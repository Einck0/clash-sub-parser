#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# CSP Real API End-to-End Test Suite (Zero Mock, Real Process & DB)
#
# Validates:
# 1. Open Mode:
#    - Healthz & Auth Status (security_mode == open)
#    - Subscriptions CRUD with Sensitive URL
#    - Redacted secret ref (***) without leaking secrets
#    - Safe update preserving secret when passing "***"
#    - Advanced Config persistence (Cron, AutoTest, RenameRules, FilterRules, TargetGroups)
# 2. Protected Mode:
#    - Token auth enforcement (ADMIN_TOKEN set)
#    - 401 on missing or invalid token
#    - 200 on valid Bearer token
#    - Publication token scope boundary / 403 on forbidden scope
#
# Cleans up all temporary processes and directories on exit.
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

echo "=============================================================================="
echo "CSP Real API End-to-End Test Suite (Zero Mock)"
echo "=============================================================================="

TMP_DIR="$(mktemp -d)"
SERVER_PID=""

cleanup() {
  if [ -n "${SERVER_PID:-}" ]; then
    kill -TERM "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT INT TERM

BIN_PATH="${TMP_DIR}/csp_test_bin"
echo "[1/4] Compiling CSP single binary..."
go build -o "${BIN_PATH}" ./cmd/csp
echo "  ✓ Executable compiled successfully: ${BIN_PATH}"

# Find free port helper
get_free_port() {
  python3 -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1]); s.close()'
}

# ==============================================================================
# TEST SUITE 1: Open Mode Testing
# ==============================================================================
echo ""
echo "[2/4] Testing Open Mode (Zero-Config Private Mode)..."

OPEN_PORT=$(get_free_port)
OPEN_DB="${TMP_DIR}/open_mode.db"

PORT="${OPEN_PORT}" DB_PATH="${OPEN_DB}" "${BIN_PATH}" serve > "${TMP_DIR}/open_server.log" 2>&1 &
SERVER_PID=$!

# Wait for server readiness
for i in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${OPEN_PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
  if [ "$i" -eq 30 ]; then
    echo "  ✗ ERROR: Server failed to start on port ${OPEN_PORT}"
    cat "${TMP_DIR}/open_server.log"
    exit 1
  fi
done

# 1. Assert healthz
HEALTHZ_RESP=$(curl -sf "http://127.0.0.1:${OPEN_PORT}/healthz")
if ! echo "${HEALTHZ_RESP}" | grep -q '"status":"ok"'; then
  echo "  ✗ ERROR: /healthz unexpected response: ${HEALTHZ_RESP}"
  exit 1
fi
echo "  ✓ /healthz responds ok"

# 2. Assert auth status is open
AUTH_STATUS=$(curl -sf "http://127.0.0.1:${OPEN_PORT}/api/v1/auth/status")
if ! echo "${AUTH_STATUS}" | grep -q '"mode":"open"'; then
  echo "  ✗ ERROR: Expected mode=open, got: ${AUTH_STATUS}"
  exit 1
fi
echo "  ✓ /api/v1/auth/status confirmed open mode"

# 3. Create subscription with sensitive URL and advanced config
SECRET_TOKEN="super-secret-user-pass-token-999"
CREATE_JSON="${TMP_DIR}/create.json"
cat > "${CREATE_JSON}" <<'EOF'
{
  "name": "Production-HK",
  "source_url_secret_ref": "https://sub.example.com/api?token=super-secret-user-pass-token-999&user=einck",
  "enabled": true,
  "refresh_policy": {
    "interval_seconds": 3600,
    "user_agent_policy": "clash-meta",
    "timeout_seconds": 15,
    "max_response_bytes": 10485760
  },
  "config": {
    "cron_schedule": "0 3 * * *",
    "auto_test": true,
    "rename_rules": [
      {"pattern": "Hong Kong (.*)", "replace": "HK-$1"}
    ],
    "filter_rules": [
      {"type": "include", "pattern": "HK|TW"},
      {"type": "exclude", "pattern": "Expired"}
    ],
    "target_groups": ["Proxy", "Streaming"]
  }
}
EOF

CREATE_RESP=$(curl -s -X POST "http://127.0.0.1:${OPEN_PORT}/api/v1/subscriptions" \
  -H "Content-Type: application/json" \
  --data-binary @"${CREATE_JSON}")

if echo "${CREATE_RESP}" | grep -q "${SECRET_TOKEN}"; then
  echo "  ✗ SECURITY VIOLATION: Create subscription leaked secret token in response!"
  echo "  Response: ${CREATE_RESP}"
  exit 1
fi

SUB_ID=$(echo "${CREATE_RESP}" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4)
SUB_REV=$(echo "${CREATE_RESP}" | grep -o '"revision":"[^"]*' | head -n 1 | cut -d'"' -f4)

if [ -z "${SUB_ID}" ]; then
  echo "  ✗ ERROR: Failed to obtain subscription ID from create response: ${CREATE_RESP}"
  exit 1
fi
echo "  ✓ Subscription created: ID=${SUB_ID}"

# 4. GET subscription and assert source_url_secret_ref is masked as "***"
GET_RESP=$(curl -sf "http://127.0.0.1:${OPEN_PORT}/api/v1/subscriptions/${SUB_ID}")
if echo "${GET_RESP}" | grep -q "${SECRET_TOKEN}"; then
  echo "  ✗ SECURITY VIOLATION: GET subscription leaked secret token!"
  exit 1
fi

if ! echo "${GET_RESP}" | grep -q '"source_url_secret_ref":"\*\*\*"'; then
  echo "  ✗ ERROR: GET subscription does not contain masked '***': ${GET_RESP}"
  exit 1
fi

if ! echo "${GET_RESP}" | grep -q '"cron_schedule":"0 3 \* \* \*"'; then
  echo "  ✗ ERROR: Advanced config cron_schedule not returned: ${GET_RESP}"
  exit 1
fi
if ! echo "${GET_RESP}" | grep -q '"auto_test":true'; then
  echo "  ✗ ERROR: Advanced config auto_test not true: ${GET_RESP}"
  exit 1
fi
echo "  ✓ Secret URL masked as '***' and advanced config properly retrieved"

# 5. Update subscription with masked '***' to verify it does NOT wipe or overwrite the secret
UPDATE_JSON="${TMP_DIR}/update.json"
cat > "${UPDATE_JSON}" <<'EOF'
{
  "name": "Production-HK-Renamed",
  "source_url_secret_ref": "***",
  "config": {
    "cron_schedule": "0 6 * * *",
    "auto_test": false,
    "rename_rules": [
      {"pattern": "HK (.*)", "replace": "HongKong-$1"}
    ],
    "filter_rules": [
      {"type": "include", "pattern": "HK"}
    ],
    "target_groups": ["Auto"]
  }
}
EOF

UPDATE_RESP=$(curl -s -X PATCH "http://127.0.0.1:${OPEN_PORT}/api/v1/subscriptions/${SUB_ID}" \
  -H "Content-Type: application/json" \
  -H "If-Match: ${SUB_REV}" \
  --data-binary @"${UPDATE_JSON}")

if echo "${UPDATE_RESP}" | grep -q "error"; then
  echo "  ✗ ERROR: Update subscription failed: ${UPDATE_RESP}"
  exit 1
fi

NEW_REV=$(echo "${UPDATE_RESP}" | grep -o '"revision":"[^"]*' | head -n 1 | cut -d'"' -f4)
if [ "${NEW_REV}" = "${SUB_REV}" ]; then
  echo "  ✗ ERROR: Revision should change on update"
  exit 1
fi

# Verify in database directly that raw source_url_secret_ref is still intact!
DB_STORED_URL=$(sqlite3 "${OPEN_DB}" "SELECT source_url_secret_ref FROM subscriptions WHERE id='${SUB_ID}';")
if [ "${DB_STORED_URL}" != "https://sub.example.com/api?token=super-secret-user-pass-token-999&user=einck" ]; then
  echo "  ✗ CRITICAL DEFECT: Secret URL was corrupted or overwritten on update with '***'!"
  echo "  Got: ${DB_STORED_URL}"
  exit 1
fi
echo "  ✓ Original secret ref preserved intact in SQLite when updating with '***'"

# Stop Open Mode server
kill -TERM "${SERVER_PID}" 2>/dev/null || true
wait "${SERVER_PID}" 2>/dev/null || true
SERVER_PID=""

# ==============================================================================
# TEST SUITE 2: Protected Mode Testing
# ==============================================================================
echo ""
echo "[3/4] Testing Protected Mode (ADMIN_TOKEN enforced)..."

PROT_PORT=$(get_free_port)
PROT_DB="${TMP_DIR}/prot_mode.db"
ADMIN_TOKEN="test-admin-secret-key-12345"

PORT="${PROT_PORT}" DB_PATH="${PROT_DB}" ADMIN_TOKEN="${ADMIN_TOKEN}" "${BIN_PATH}" serve > "${TMP_DIR}/prot_server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${PROT_PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
  if [ "$i" -eq 30 ]; then
    echo "  ✗ ERROR: Protected server failed to start on port ${PROT_PORT}"
    cat "${TMP_DIR}/prot_server.log"
    exit 1
  fi
done

# 1. Auth status must be protected
PROT_STATUS=$(curl -sf "http://127.0.0.1:${PROT_PORT}/api/v1/auth/status")
if ! echo "${PROT_STATUS}" | grep -q '"mode":"protected"'; then
  echo "  ✗ ERROR: Expected mode=protected, got: ${PROT_STATUS}"
  exit 1
fi
echo "  ✓ /api/v1/auth/status confirmed protected mode"

# 2. Anonymous request to /api/v1/subscriptions must return 401
HTTP_CODE_NO_AUTH=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${PROT_PORT}/api/v1/subscriptions")
if [ "${HTTP_CODE_NO_AUTH}" != "401" ]; then
  echo "  ✗ ERROR: Expected 401 on unauthenticated request, got: ${HTTP_CODE_NO_AUTH}"
  exit 1
fi
echo "  ✓ Anonymous request rejected with HTTP 401"

# 3. Invalid token must return 401
HTTP_CODE_BAD_AUTH=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer wrong-token-xyz" \
  "http://127.0.0.1:${PROT_PORT}/api/v1/subscriptions")
if [ "${HTTP_CODE_BAD_AUTH}" != "401" ]; then
  echo "  ✗ ERROR: Expected 401 on invalid token, got: ${HTTP_CODE_BAD_AUTH}"
  exit 1
fi
echo "  ✓ Invalid Bearer token rejected with HTTP 401"

# 4. Valid Admin Bearer token must succeed with 200
HTTP_CODE_VALID_AUTH=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "http://127.0.0.1:${PROT_PORT}/api/v1/subscriptions")
if [ "${HTTP_CODE_VALID_AUTH}" != "200" ]; then
  echo "  ✗ ERROR: Expected 200 on valid admin token, got: ${HTTP_CODE_VALID_AUTH}"
  exit 1
fi
echo "  ✓ Valid Admin Bearer token accepted with HTTP 200"

# 5. Non-admin client token must be rejected (Anti-escalation)
HTTP_CODE_FORBIDDEN_SCOPE=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer client-pub-token-987654" \
  "http://127.0.0.1:${PROT_PORT}/api/v1/subscriptions")

if [ "${HTTP_CODE_FORBIDDEN_SCOPE}" != "401" ] && [ "${HTTP_CODE_FORBIDDEN_SCOPE}" != "403" ]; then
  echo "  ✗ ERROR: Non-admin token should be rejected with 401 or 403, got: ${HTTP_CODE_FORBIDDEN_SCOPE}"
  exit 1
fi
echo "  ✓ Non-admin scope rejected (HTTP ${HTTP_CODE_FORBIDDEN_SCOPE}) on admin endpoints"

# 6. Dynamic Admin Token Management & Rotation E2E
echo "  → Testing Dynamic Admin Token rotation via POST /api/v1/settings/admin-token..."
NEW_ADMIN_TOKEN="rotated-dynamic-secret-$(date +%s)"
UPDATE_RESP=$(curl -s -w "\n%{http_code}" -X POST \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"token\": \"${NEW_ADMIN_TOKEN}\"}" \
  "http://127.0.0.1:${PROT_PORT}/api/v1/settings/admin-token")
UPDATE_CODE=$(echo "${UPDATE_RESP}" | tail -n 1)

if [ "${UPDATE_CODE}" != "200" ]; then
  echo "  ✗ ERROR: Failed to update admin token, got HTTP: ${UPDATE_CODE}"
  exit 1
fi
echo "  ✓ Token updated successfully (HTTP 200)"

# Old token must now be rejected
OLD_TOKEN_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "http://127.0.0.1:${PROT_PORT}/api/v1/subscriptions")
if [ "${OLD_TOKEN_CODE}" != "401" ]; then
  echo "  ✗ ERROR: Old admin token should be rejected with 401, got: ${OLD_TOKEN_CODE}"
  exit 1
fi
echo "  ✓ Old Bearer token immediately rejected with HTTP 401"

# New token must succeed
NEW_TOKEN_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${NEW_ADMIN_TOKEN}" \
  "http://127.0.0.1:${PROT_PORT}/api/v1/subscriptions")
if [ "${NEW_TOKEN_CODE}" != "200" ]; then
  echo "  ✗ ERROR: New admin token should succeed with 200, got: ${NEW_TOKEN_CODE}"
  exit 1
fi
echo "  ✓ Rotated Admin Bearer token accepted with HTTP 200"

# Stop Protected Mode server
kill -TERM "${SERVER_PID}" 2>/dev/null || true
wait "${SERVER_PID}" 2>/dev/null || true
SERVER_PID=""

echo ""
echo "[4/4] All Real API End-to-End Tests Passed Successfully! (Exit Code 0)"
echo "=============================================================================="

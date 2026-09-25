#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# CSP 1.0 Clean-Slate Production & Ops Smoke Test Harness
# Validates Compose configs, volume isolation, Docker non-root image,
# empty-volume startup, healthz/readyz, 410 interceptors, backup & restore.
# Zero hardcoded absolute paths: uses dynamic script directory and mktemp.
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== [1/6] Validating Docker Compose Configurations ==="
cd "${REPO_ROOT}"

# 1. Validate compose configs
docker compose config >/dev/null
echo "  ✓ docker-compose.yml config valid"

docker compose -f docker-compose.example.yml config >/dev/null
echo "  ✓ docker-compose.example.yml config valid"

# 2. Verify strict physical volume isolation (NO legacy volumes)
echo "=== [2/6] Verifying Volume Isolation (No Legacy Volumes) ==="
if grep -E "backend-data|clash-sub-parser_backend-data|clash_sub_parser\.db" docker-compose*.yml Dockerfile; then
  echo "  ✗ ERROR: Legacy volume or database reference detected in deployment manifests!"
  exit 1
fi
echo "  ✓ Zero legacy volume references in compose manifests and Dockerfile"

# Check that csp-v1-data is declared
if ! grep -q "csp-v1-data" docker-compose.yml; then
  echo "  ✗ ERROR: csp-v1-data volume missing in docker-compose.yml"
  exit 1
fi
if ! grep -q "csp-v1-data" docker-compose.example.yml; then
  echo "  ✗ ERROR: csp-v1-data volume missing in docker-compose.example.yml"
  exit 1
fi
echo "  ✓ Dedicated csp-v1-data volume declared in all compose manifests"

# 3. Verify Dockerfile non-root configuration
echo "=== [3/6] Verifying Dockerfile Non-Root Runtime Contract ==="
if ! grep -q "USER appuser" Dockerfile; then
  echo "  ✗ ERROR: USER appuser missing in Dockerfile"
  exit 1
fi
if ! grep -q "10001" Dockerfile; then
  echo "  ✗ ERROR: UID/GID 10001 specification missing in Dockerfile"
  exit 1
fi
if ! grep -q "/healthz" Dockerfile; then
  echo "  ✗ ERROR: Container healthcheck probe does not target /healthz"
  exit 1
fi
echo "  ✓ Dockerfile non-root runtime (appuser:10001) and /healthz probe verified"

# 4. Compile single binary and test empty-volume startup
echo "=== [4/6] Testing Empty-Volume Server Startup & Health Probes ==="
TMP_DIR="$(mktemp -d)"
cleanup() {
  if [ -n "${SERVER_PID:-}" ]; then
    kill -TERM "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  if [ -n "${RESTORE_PID:-}" ]; then
    kill -TERM "${RESTORE_PID}" 2>/dev/null || true
    wait "${RESTORE_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

BIN_PATH="${TMP_DIR}/csp"
go build -o "${BIN_PATH}" ./cmd/csp
echo "  ✓ Single Go executable compiled: ${BIN_PATH}"

# Pick random available port
PORT=$((20000 + RANDOM % 10000))
ADDR="127.0.0.1:${PORT}"
DB_PATH="${TMP_DIR}/data/csp-v1.db"

# Start CSP server with empty database path
"${BIN_PATH}" serve -addr "${ADDR}" -db "${DB_PATH}" &
SERVER_PID=$!

# Wait for server to start
MAX_WAIT=20
WAIT_COUNT=0
until curl -s -f "http://${ADDR}/healthz" >/dev/null 2>&1; do
  sleep 0.2
  WAIT_COUNT=$((WAIT_COUNT + 1))
  if [ "${WAIT_COUNT}" -ge "${MAX_WAIT}" ]; then
    echo "  ✗ ERROR: Timeout waiting for server startup on http://${ADDR}"
    exit 1
  fi
done

# Verify /healthz
HEALTHZ_RESP=$(curl -s "http://${ADDR}/healthz")
echo "  Response /healthz: ${HEALTHZ_RESP}"
if ! echo "${HEALTHZ_RESP}" | grep -q '"status":"ok"'; then
  echo "  ✗ ERROR: Unexpected /healthz response"
  exit 1
fi
echo "  ✓ /healthz liveness probe OK"

# Verify /readyz
READYZ_RESP=$(curl -s "http://${ADDR}/readyz")
echo "  Response /readyz: ${READYZ_RESP}"
if ! echo "${READYZ_RESP}" | grep -q '"ready":true'; then
  echo "  ✗ ERROR: Unexpected /readyz response"
  exit 1
fi
echo "  ✓ /readyz readiness probe OK (schema migrated)"

# Verify legacy 410 interceptors
YAML_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://${ADDR}/yaml")
if [ "${YAML_CODE}" != "410" ]; then
  echo "  ✗ ERROR: Expected 410 on /yaml, got ${YAML_CODE}"
  exit 1
fi
SCRIPT_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://${ADDR}/script")
if [ "${SCRIPT_CODE}" != "410" ]; then
  echo "  ✗ ERROR: Expected 410 on /script, got ${SCRIPT_CODE}"
  exit 1
fi
echo "  ✓ Legacy routes (/yaml, /script) return HTTP 410 Gone"

# 5. Database hot backup and integrity verification
echo "=== [5/6] Testing Database Hot Backup & Integrity Check ==="
BACKUP_PATH="${TMP_DIR}/csp-v1-backup.db"
sqlite3 "${DB_PATH}" ".backup '${BACKUP_PATH}'"

if [ ! -s "${BACKUP_PATH}" ]; then
  echo "  ✗ ERROR: Backup file is empty or not found: ${BACKUP_PATH}"
  exit 1
fi

INTEGRITY=$(sqlite3 "${BACKUP_PATH}" "PRAGMA integrity_check;")
if [ "${INTEGRITY}" != "ok" ]; then
  echo "  ✗ ERROR: Backup integrity check failed: ${INTEGRITY}"
  exit 1
fi

FK_CHECK=$(sqlite3 "${BACKUP_PATH}" "PRAGMA foreign_key_check;")
if [ -n "${FK_CHECK}" ]; then
  echo "  ✗ ERROR: Backup foreign key check failed: ${FK_CHECK}"
  exit 1
fi

SCHEMA_VER=$(sqlite3 "${BACKUP_PATH}" "SELECT MAX(version) FROM schema_migrations;")
echo "  Backup Schema Version: ${SCHEMA_VER}"
if [ -z "${SCHEMA_VER}" ] || [ "${SCHEMA_VER}" -lt 1 ]; then
  echo "  ✗ ERROR: Invalid schema version in backup: ${SCHEMA_VER}"
  exit 1
fi
echo "  ✓ Hot backup created, integrity_check=ok, foreign_key_check=clean, schema_version=${SCHEMA_VER}"

# 6. Disaster restore verification
echo "=== [6/6] Testing Disaster Recovery Restore ==="
RESTORE_PORT=$((30000 + RANDOM % 10000))
RESTORE_ADDR="127.0.0.1:${RESTORE_PORT}"
RESTORE_DB_PATH="${TMP_DIR}/restored_data/csp-v1.db"
mkdir -p "${TMP_DIR}/restored_data"
cp "${BACKUP_PATH}" "${RESTORE_DB_PATH}"

"${BIN_PATH}" serve -addr "${RESTORE_ADDR}" -db "${RESTORE_DB_PATH}" &
RESTORE_PID=$!

WAIT_COUNT=0
until curl -s -f "http://${RESTORE_ADDR}/readyz" >/dev/null 2>&1; do
  sleep 0.2
  WAIT_COUNT=$((WAIT_COUNT + 1))
  if [ "${WAIT_COUNT}" -ge "${MAX_WAIT}" ]; then
    echo "  ✗ ERROR: Timeout waiting for restored server startup"
    exit 1
  fi
done

RESTORE_READYZ=$(curl -s "http://${RESTORE_ADDR}/readyz")
if ! echo "${RESTORE_READYZ}" | grep -q '"ready":true'; then
  echo "  ✗ ERROR: Restored server failed readiness check"
  exit 1
fi
echo "  ✓ Restored database server launched and passed /readyz readiness check"

echo "=============================================================================="
echo "ALL SMOKE TESTS PASSED SUCCESSFULLY! (Exit Code 0)"
echo "=============================================================================="

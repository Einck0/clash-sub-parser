#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# CSP 1.0 Clean-Slate End-to-End Closed-Loop Integration Test Harness
# Validates the full lifecycle:
# 1. Integration E2E fixture (refresh -> inventory -> probe -> policy -> 5 targets -> pub -> UI)
# 2. End-to-end IP risk fake provider probe, deterministic policy & preflight interception
# 3. Whole Go backend test suite (go test ./...)
# 4. Data race detection on integration harness (go test -race)
# 5. Frontend production build (cd web && npm run build)
# 6. Web component & multi-device unit/E2E test suite (cd web && npm test)
# 7. Headless Playwright real browser multi-device verification (cd web && npm run test:playwright)
# 8. Docker Compose deployment configuration validation
# 9. Ops smoke test harness (backup/restore, non-root, volume isolation, 410s)
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

echo "=============================================================================="
echo "CSP 1.0 Clean-Slate End-to-End Closed-Loop Integration Test Harness"
echo "Root: ${REPO_ROOT}"
echo "=============================================================================="

# 1. Run full Go integration closed-loop test
echo ""
echo "=== [1/8] Running Go Integration E2E Closed-Loop Harness ==="
go test -count=1 -v ./internal/integration/...
echo "  ✓ Go integration harness passed (exit code 0)"

# 2. Run Go integration race detector
echo ""
echo "=== [2/8] Running Go Integration Race Detector ==="
go test -count=1 -race ./internal/integration/...
echo "  ✓ Go integration race detector passed with 0 races (exit code 0)"

# 3. Run full Go test suite
echo ""
echo "=== [3/8] Running Full Go Test Suite ==="
go test ./...
echo "  ✓ All Go packages passed (exit code 0)"

# 4. Build frontend bundle
echo ""
echo "=== [4/8] Building Frontend Production Distribution (web) ==="
(cd web && NO_COPY_WEBASSETS=1 npm run build)
echo "  ✓ Frontend built successfully (exit code 0)"

# 5. Run frontend Vitest suite (unit & multi-device DOM integration)
echo ""
echo "=== [5/8] Running Frontend Vitest Suite (web) ==="
(cd web && npm test)
echo "  ✓ Frontend Vitest suite passed (exit code 0)"

# 6. Run Playwright real browser tests across mobile & desktop viewports
echo ""
echo "=== [6/8] Running Playwright Real Browser Multi-Device E2E Tests ==="
(cd web && npm run test:playwright)
echo "  ✓ Playwright multi-device tests passed (exit code 0)"

# 7. Validate Docker Compose configurations
echo ""
echo "=== [7/8] Validating Docker Compose Configuration ==="
docker compose config >/dev/null
docker compose -f docker-compose.example.yml config >/dev/null
echo "  ✓ Docker Compose manifests verified (exit code 0)"

# 8. Run Production & Ops Smoke Test Harness
echo ""
echo "=== [8/8] Running Production & Ops Smoke Test Harness ==="
bash "${SCRIPT_DIR}/smoke_test.sh"
echo "  ✓ Production smoke tests passed (exit code 0)"

echo ""
echo "=============================================================================="
echo "ALL END-TO-END INTEGRATION HARNESS SUITES PASSED SUCCESSFULLY! (EXIT CODE 0)"
echo "=============================================================================="

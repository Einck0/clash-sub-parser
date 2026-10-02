#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# CSP Live Semantic Acceptance Runner
# Safely inspects production or staging CSP endpoints with semantic assertions.
# Reads credentials safely from managed environment or credential file.
# Default mode is READ-ONLY baseline audit.
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CSP_ACCEPTANCE_BIN="/tmp/csp-live-acceptance"
echo "Building csp-live-acceptance tool..."
(cd "${REPO_ROOT}" && go build -o "${CSP_ACCEPTANCE_BIN}" ./cmd/csp-live-acceptance)

exec "${CSP_ACCEPTANCE_BIN}" "$@"

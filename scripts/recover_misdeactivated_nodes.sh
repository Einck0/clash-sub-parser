#!/usr/bin/env bash
set -euo pipefail

# Script for inspecting and recovering historical mis-deactivated nodes in SQLite database.
# Default mode is dry-run inspect.
# Usage:
#   ./scripts/recover_misdeactivated_nodes.sh --db <path-to-db> [--dry-run | --apply | --rollback] [--plan <plan-file>]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CSP_RECOVER_BIN="/tmp/csp-recover-nodes"
echo "Building csp-recover-nodes tool..."
(cd "${REPO_ROOT}" && go build -o "${CSP_RECOVER_BIN}" ./cmd/csp-recover-nodes)

exec "${CSP_RECOVER_BIN}" "$@"

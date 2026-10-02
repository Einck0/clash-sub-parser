#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# CSP Live Semantic Acceptance Runner
# Safely inspects production or staging CSP endpoints with semantic assertions.
# Reads credentials safely from managed environment or credential file.
# Uses private managed scratch directory with secure umask and reliable cleanup.
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Enforce secure umask 077 for private directory and file creation
umask 077

# Determine scratch directory (managed private directory or secure mktemp)
if [[ -n "${CSP_ACCEPTANCE_SCRATCH:-}" ]]; then
    SCRATCH_DIR="${CSP_ACCEPTANCE_SCRATCH}"
    if [[ ! -d "${SCRATCH_DIR}" ]]; then
        mkdir -p "${SCRATCH_DIR}"
        chmod 0700 "${SCRATCH_DIR}"
    fi
    # Security check: verify ownership and permissions
    OWNER_UID=$(stat -c '%u' "${SCRATCH_DIR}" 2>/dev/null || stat -f '%u' "${SCRATCH_DIR}" 2>/dev/null || id -u)
    if [[ "${OWNER_UID}" != "$(id -u)" ]]; then
        echo "ERROR: Scratch directory ${SCRATCH_DIR} is not owned by current user ($(id -u))" >&2
        exit 1
    fi
    # Ensure only owner has rwx access
    chmod 0700 "${SCRATCH_DIR}"
    CLEANUP_DIR=0
else
    SCRATCH_DIR=$(mktemp -d "${TMPDIR:-/tmp}/csp-acceptance.XXXXXX")
    chmod 0700 "${SCRATCH_DIR}"
    CLEANUP_DIR=1
fi

CSP_ACCEPTANCE_BIN="${SCRATCH_DIR}/csp-live-acceptance"

cleanup() {
    local exit_code=$?
    # Delete binary temporary build artifact
    rm -f "${CSP_ACCEPTANCE_BIN}" 2>/dev/null || true
    if [[ "${CLEANUP_DIR}" -eq 1 && -d "${SCRATCH_DIR}" ]]; then
        rm -rf "${SCRATCH_DIR}" 2>/dev/null || true
    fi
    exit "${exit_code}"
}
trap cleanup EXIT INT TERM HUP

echo "Building csp-live-acceptance tool in private scratch..."
if ! (cd "${REPO_ROOT}" && go build -o "${CSP_ACCEPTANCE_BIN}" ./cmd/csp-live-acceptance); then
    echo "ERROR: Compilation of csp-live-acceptance failed" >&2
    exit 1
fi

# Do NOT use exec, so that EXIT trap reliably cleans up scratch directory and artifacts
set +e
"${CSP_ACCEPTANCE_BIN}" "$@"
RUN_EXIT_CODE=$?
set -e

exit "${RUN_EXIT_CODE}"

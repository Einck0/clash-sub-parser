#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR/backend"

if [ -x /data/py310/bin/python ]; then
  PY=/data/py310/bin/python
elif [ -n "${VIRTUAL_ENV:-}" ] && [ -x "$VIRTUAL_ENV/bin/python" ]; then
  PY="$VIRTUAL_ENV/bin/python"
elif command -v python3.10 >/dev/null 2>&1; then
  PY=python3.10
elif command -v python3 >/dev/null 2>&1; then
  PY=python3
else
  echo "[test] error: Python 3.10+ is required." >&2
  exit 1
fi

MAJOR_MINOR="$($PY - <<'PY'
import sys
print(f"{sys.version_info.major}.{sys.version_info.minor}")
PY
)"
case "$MAJOR_MINOR" in
  3.10|3.11|3.12|3.13|3.14) ;;
  *)
    echo "[test] warning: expected Python 3.10+, got $MAJOR_MINOR" >&2
    ;;
esac

echo "[test] using interpreter: $PY"
"$PY" -m pip install -r requirements.txt
"$PY" -m pytest -q

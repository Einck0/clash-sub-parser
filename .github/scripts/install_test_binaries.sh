#!/usr/bin/env bash
#
# Installs pinned official release binaries of sing-box and mihomo for CI runner tests.
# Enforces strict SHA256 checksum verification against official releases.
#
set -euo pipefail

SINGBOX_VERSION="1.14.0"
SINGBOX_SHA256="2375de6999f4f56ab46b4fc5ddf26a6aba1d3e61a0f4e7ddec2f4690457d5f63"
SINGBOX_URL="https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-amd64.tar.gz"

MIHOMO_VERSION="v1.19.3"
MIHOMO_SHA256="def3d6a2199fbbd987c68f960c1cc01842e244fa44346b3538c584cd329d57a1"
MIHOMO_URL="https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/mihomo-linux-amd64-${MIHOMO_VERSION}.gz"

BIN_DIR="${RUNNER_TEMP:-/tmp}/bin"
mkdir -p "$BIN_DIR"

SCRATCH_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$SCRATCH_DIR"
}
trap cleanup EXIT

echo "=== Installing CI Test Binaries ==="
echo "Target directory: $BIN_DIR"

# 1. sing-box
if [ -x "$BIN_DIR/sing-box" ]; then
  echo "sing-box already exists in $BIN_DIR"
else
  echo "Downloading sing-box v${SINGBOX_VERSION}..."
  SINGBOX_TAR="$SCRATCH_DIR/sing-box.tar.gz"
  curl -sSL --retry 3 --max-time 60 --fail "$SINGBOX_URL" -o "$SINGBOX_TAR"

  ACTUAL_SINGBOX_SHA="$(sha256sum "$SINGBOX_TAR" | awk '{print $1}')"
  if [ "$ACTUAL_SINGBOX_SHA" != "$SINGBOX_SHA256" ]; then
    echo "ERROR: sing-box SHA256 mismatch!" >&2
    echo "Expected: $SINGBOX_SHA256" >&2
    echo "Got:      $ACTUAL_SINGBOX_SHA" >&2
    exit 1
  fi

  tar -xzf "$SINGBOX_TAR" -C "$SCRATCH_DIR"
  cp "$SCRATCH_DIR"/sing-box-*-linux-amd64/sing-box "$BIN_DIR/sing-box"
  chmod +x "$BIN_DIR/sing-box"
  echo "sing-box v${SINGBOX_VERSION} verified and installed."
fi
"$BIN_DIR/sing-box" version

# 2. mihomo
if [ -x "$BIN_DIR/mihomo" ]; then
  echo "mihomo already exists in $BIN_DIR"
else
  echo "Downloading mihomo ${MIHOMO_VERSION}..."
  MIHOMO_GZ="$SCRATCH_DIR/mihomo.gz"
  curl -sSL --retry 3 --max-time 60 --fail "$MIHOMO_URL" -o "$MIHOMO_GZ"

  ACTUAL_MIHOMO_SHA="$(sha256sum "$MIHOMO_GZ" | awk '{print $1}')"
  if [ "$ACTUAL_MIHOMO_SHA" != "$MIHOMO_SHA256" ]; then
    echo "ERROR: mihomo SHA256 mismatch!" >&2
    echo "Expected: $MIHOMO_SHA256" >&2
    echo "Got:      $ACTUAL_MIHOMO_SHA" >&2
    exit 1
  fi

  gzip -dc "$MIHOMO_GZ" > "$BIN_DIR/mihomo"
  chmod +x "$BIN_DIR/mihomo"
  echo "mihomo ${MIHOMO_VERSION} verified and installed."
fi
"$BIN_DIR/mihomo" -v

# 3. Export PATH and environment variables for GitHub Actions runner
if [ -n "${GITHUB_PATH:-}" ]; then
  echo "$BIN_DIR" >> "$GITHUB_PATH"
fi
if [ -n "${GITHUB_ENV:-}" ]; then
  echo "SINGBOX_BIN=$BIN_DIR/sing-box" >> "$GITHUB_ENV"
  echo "MIHOMO_BIN=$BIN_DIR/mihomo" >> "$GITHUB_ENV"
fi

echo "=== CI Test Binaries Installation Complete ==="

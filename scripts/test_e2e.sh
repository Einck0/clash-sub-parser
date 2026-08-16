#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

image=${E2E_IMAGE:-clash-sub-parser:e2e}
container=${E2E_CONTAINER:-clash-sub-parser-e2e}
port=${E2E_PORT:-18082}
base_url=${E2E_BASE_URL:-http://127.0.0.1:${port}}

cleanup() {
  status=$?
  trap - EXIT
  if [ "$status" -ne 0 ] && docker ps -a --format '{{.Names}}' | grep -Fxq "$container"; then
    docker logs "$container" || true
  fi
  docker rm -f "$container" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup EXIT

build_args=()
for proxy_var in HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY; do
  proxy_value=${!proxy_var:-}
  if [ -n "$proxy_value" ]; then
    build_args+=(--build-arg "${proxy_var}=${proxy_value}")
  fi
done
if [ "${#build_args[@]}" -gt 0 ]; then
  build_args=(--network=host "${build_args[@]}")
fi

docker rm -f "$container" >/dev/null 2>&1 || true
if [ "${E2E_SKIP_BUILD:-0}" != "1" ]; then
  docker build "${build_args[@]}" -t "$image" .
fi
docker run -d --rm \
  --name "$container" \
  -p "127.0.0.1:${port}:18080" \
  -e CLASH_DATABASE_URL=sqlite+aiosqlite:////tmp/e2e.db \
  -e CLASH_AUTH_ENABLED=false \
  -e CLASH_SCHEDULER_ENABLED=false \
  "$image" >/dev/null

for _ in $(seq 1 30); do
  if curl --noproxy '*' --fail --silent "$base_url/ready" >/dev/null; then
    E2E_BASE_URL="$base_url" npm --prefix frontend run test:e2e
    exit 0
  fi
  sleep 1
done

echo "E2E container did not become ready: $base_url" >&2
exit 1

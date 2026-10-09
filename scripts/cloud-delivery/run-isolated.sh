#!/usr/bin/env bash
# scripts/cloud-delivery/run-isolated.sh
# CSP 隔离预览环境启动与健康确权脚本
# 运行在独立 Docker project、独立端口与独立全新合成卷中，带有所有权标记，严禁触碰生产环境。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PRIMARY_PORT="${CSP_ISOLATED_PORT_PRIMARY:-18081}"
SECONDARY_PORT="${CSP_ISOLATED_PORT_SECONDARY:-17001}"
IMAGE_REF="${CSP_ISOLATED_IMAGE:-${CSP_IMAGE:-}}"
PROJECT_NAME="${CSP_ISOLATED_PROJECT:-csp-isolated}"
RUN_MARKER="${CSP_ISOLATED_RUN_MARKER:-}"
CHECK_ONLY=false
WAIT_TIMEOUT=20
PREVIEW_DIR="/tmp/csp-preview"

usage() {
  cat << 'EOF'
用法:
  scripts/cloud-delivery/run-isolated.sh [选项]

选项:
  --image <ref>             待运行的完整镜像引用（必须包含 @sha256:...）
  --project <name>          隔离 Docker project 名称（默认: csp-isolated）
  --run-marker <marker>     资源所有权唯一标识（默认自动生成带随机后缀）
  --port <port>             主回环端口（默认: 18081）
  --secondary-port <port>   次级回环端口（默认: 17001）
  --timeout <sec>           健康检查等待超时（秒，默认: 20）
  --check-only              仅检查端口占用、配置合法性与本地镜像就绪状态，不启动容器
  -h, --help                显示帮助信息
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)
      IMAGE_REF="$2"
      shift 2
      ;;
    --project)
      PROJECT_NAME="$2"
      shift 2
      ;;
    --run-marker)
      RUN_MARKER="$2"
      shift 2
      ;;
    --port)
      PRIMARY_PORT="$2"
      shift 2
      ;;
    --secondary-port)
      SECONDARY_PORT="$2"
      shift 2
      ;;
    --timeout)
      WAIT_TIMEOUT="$2"
      shift 2
      ;;
    --check-only)
      CHECK_ONLY=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: 未知参数: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [ -z "${RUN_MARKER}" ]; then
  RAND_SUFFIX=$(head -c 4 /dev/urandom | xxd -p)
  RUN_MARKER="preview-${RAND_SUFFIX}"
fi

ISOLATED_VOLUME="csp-isolated-data-${RUN_MARKER}"

echo "=== [CSP Cloud Delivery Isolated Preview] 启动检查 ==="
echo "项目名称: ${PROJECT_NAME}"
echo "所有权 Marker: ${RUN_MARKER}"

# 1. 回环端口占用冲突检查 (Bind Check)
check_port() {
  local p="$1"
  if ss -tln | grep -q "127.0.0.1:${p}\b"; then
    echo "ERROR: 端口 127.0.0.1:${p} 已被占用，无法用于隔离预览！" >&2
    echo "请检查占用进程 (ss -tlnp | grep ${p}) 或通过 --port 指定其他未占用回环端口。" >&2
    exit 1
  fi
}

echo ">>> 检查回环端口可用性: ${PRIMARY_PORT}, ${SECONDARY_PORT} ..."
check_port "${PRIMARY_PORT}"
check_port "${SECONDARY_PORT}"
echo "[CHECK] 端口 127.0.0.1:${PRIMARY_PORT} 与 127.0.0.1:${SECONDARY_PORT} 均空闲可用。"

# 2. 镜像参数与本地存在性检查
if [ -z "${IMAGE_REF}" ]; then
  echo "ERROR: 未指定镜像引用。请使用 --image 选项或设置 CSP_IMAGE / CSP_ISOLATED_IMAGE。" >&2
  exit 1
fi

echo ">>> 检查本地 Docker 缓存中的镜像: ${IMAGE_REF} ..."
if ! docker image inspect "${IMAGE_REF}" >/dev/null 2>&1; then
  echo "------------------------------------------------------------------"
  echo "STATUS: BLOCKED (镜像未在本地就绪)"
  echo "镜像 ${IMAGE_REF} 不在本地 Docker 缓存中。"
  echo "由于私有镜像访问需要 Classic PAT (read:packages) 凭据，脚本严禁在此处盲目拉取。"
  echo "前置步骤: 请先运行 scripts/cloud-delivery/preflight-pull.sh 拉取并校验镜像。"
  echo "责任人: 操作员 / 主脑 (等待用户 QQ 提供凭据)"
  echo "------------------------------------------------------------------"
  if [ "${CHECK_ONLY}" = true ]; then
    echo "Check-only 检查完毕: 端口空闲，但本地镜像缺失。"
    exit 2
  fi
  exit 2
fi
echo "[CHECK] 本地已存在待测镜像 ${IMAGE_REF}。"

if [ "${CHECK_ONLY}" = true ]; then
  echo "Check-only 检查通过: 端口空闲，配置与镜像全部就绪。"
  exit 0
fi

# 3. 准备隔离凭据目录与令牌 (权限 0600)
mkdir -p "${PREVIEW_DIR}"
chmod 0700 "${PREVIEW_DIR}"

SYNTHETIC_ADMIN_TOKEN="synthetic-token-$(head -c 16 /dev/urandom | xxd -p)"
CLEANUP_TOKEN="cleanup-token-$(head -c 16 /dev/urandom | xxd -p)"

cat > "${PREVIEW_DIR}/auth-secrets.json" << EOF
{
  "adminToken": "${SYNTHETIC_ADMIN_TOKEN}",
  "createdAt": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "note": "Synthetic test credential for isolated preview only. Never use in production."
}
EOF
chmod 0600 "${PREVIEW_DIR}/auth-secrets.json"

cat > "${PREVIEW_DIR}/cleanup-token.secret" << EOF
${CLEANUP_TOKEN}
EOF
chmod 0600 "${PREVIEW_DIR}/cleanup-token.secret"

cat > "${PREVIEW_DIR}/run-state.json" << EOF
{
  "project": "${PROJECT_NAME}",
  "runMarker": "${RUN_MARKER}",
  "volumeName": "${ISOLATED_VOLUME}",
  "primaryPort": "${PRIMARY_PORT}",
  "secondaryPort": "${SECONDARY_PORT}",
  "createdAt": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
}
EOF
chmod 0600 "${PREVIEW_DIR}/run-state.json"

# 4. 使用 docker compose 在独立 project 中拉起服务 (--no-build 强制生效)
echo ">>> 在独立 project (${PROJECT_NAME}) 中启动隔离容器 (严格 --no-build) ..."
cd "${REPO_ROOT}"

CSP_ISOLATED_IMAGE="${IMAGE_REF}" \
CSP_ISOLATED_PROJECT="${PROJECT_NAME}" \
CSP_ISOLATED_RUN_MARKER="${RUN_MARKER}" \
CSP_ISOLATED_VOLUME_NAME="${ISOLATED_VOLUME}" \
CSP_ISOLATED_PORT_PRIMARY="127.0.0.1:${PRIMARY_PORT}" \
CSP_ISOLATED_PORT_SECONDARY="127.0.0.1:${SECONDARY_PORT}" \
CSP_ISOLATED_ADMIN_TOKEN="${SYNTHETIC_ADMIN_TOKEN}" \
CSP_ISOLATED_FETCH_PROXY="" \
docker compose -f docker-compose.isolated.yml -p "${PROJECT_NAME}" up -d --no-build

# 5. 健康检查轮询 (/healthz)
echo ">>> 等待隔离容器健康检查就绪 (超时: ${WAIT_TIMEOUT}s) ..."
healthy=false
for ((i=1; i<=WAIT_TIMEOUT; i++)); do
  if curl -sSf "http://127.0.0.1:${PRIMARY_PORT}/healthz" >/dev/null 2>&1; then
    healthy=true
    echo ">>> 健康检查通过 (耗时 ${i}s)"
    break
  fi
  sleep 1
done

if [ "${healthy}" = false ]; then
  echo "ERROR: 隔离容器在 ${WAIT_TIMEOUT} 秒内未通过 /healthz 健康检查！" >&2
  echo "正在收集容器日志..." >&2
  docker compose -f docker-compose.isolated.yml -p "${PROJECT_NAME}" logs --tail 30 >&2 || true
  exit 1
fi

# 6. 生成标准实例工件 instance.json
INSTANCE_ID="${PROJECT_NAME}-${RUN_MARKER}-${PRIMARY_PORT}"
INSTANCE_JSON="${PREVIEW_DIR}/instance.json"

cat > "${INSTANCE_JSON}" << EOF
{
  "instanceId": "${INSTANCE_ID}",
  "url": "http://127.0.0.1:${PRIMARY_PORT}",
  "health": "healthy",
  "allowedOrigins": [
    "http://127.0.0.1:${PRIMARY_PORT}",
    "http://127.0.0.1:${SECONDARY_PORT}"
  ],
  "sourceManifest": {
    "image": "${IMAGE_REF}",
    "project": "${PROJECT_NAME}",
    "runMarker": "${RUN_MARKER}",
    "composeFile": "docker-compose.isolated.yml"
  },
  "testDataBoundary": {
    "database": "isolated Docker volume ${ISOLATED_VOLUME}",
    "auth": "synthetic token from ${PREVIEW_DIR}/auth-secrets.json",
    "network": "isolated docker network, proxy disabled",
    "productionDataIsolation": "100% isolated, zero production DB writes"
  },
  "scenarios": [
    "smoke",
    "healthz",
    "readyz",
    "critic_browser_geometry"
  ],
  "shutdownOwner": "scripts/cloud-delivery/cleanup-isolated.sh",
  "cleanupToken": "${CLEANUP_TOKEN}"
}
EOF
chmod 0600 "${INSTANCE_JSON}"

echo "=== [CSP Cloud Delivery Isolated Preview] 隔离预览环境已就绪 ==="
echo "访问地址: http://127.0.0.1:${PRIMARY_PORT}"
echo "实例描述工件: ${INSTANCE_JSON}"
echo "清理命令: scripts/cloud-delivery/cleanup-isolated.sh --project ${PROJECT_NAME} --run-marker ${RUN_MARKER}"
exit 0

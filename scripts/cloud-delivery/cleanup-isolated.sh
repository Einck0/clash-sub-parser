#!/usr/bin/env bash
# scripts/cloud-delivery/cleanup-isolated.sh
# CSP 隔离预览环境安全清理脚本
# 仅清理明确归属当前交付拥有的隔离项目资源，具备所有权 run-marker 校验与生产容器/卷前后守护断言。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PREVIEW_DIR="/tmp/csp-preview"
STATE_FILE="${PREVIEW_DIR}/run-state.json"
PROJECT_NAME="${CSP_ISOLATED_PROJECT:-}"
RUN_MARKER="${CSP_ISOLATED_RUN_MARKER:-}"
DRY_RUN=false

usage() {
  cat << 'EOF'
用法:
  scripts/cloud-delivery/cleanup-isolated.sh [选项]

选项:
  --project <name>       指定需要清理的隔离 Docker project（若未指定则从 run-state.json 读取）
  --run-marker <marker>  指定本次交付拥有的资源所有权 Marker（必选或从 run-state.json 读取）
  --dry-run              模拟清理流程，不实际执行 docker compose down
  -h, --help             显示帮助信息
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project)
      PROJECT_NAME="$2"
      shift 2
      ;;
    --run-marker)
      RUN_MARKER="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=true
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

# 若未通过参数指定，尝试从本地运行状态工件读取
if [ -z "${PROJECT_NAME}" ] && [ -f "${STATE_FILE}" ]; then
  PROJECT_NAME=$(grep -o '"project": "[^"]*"' "${STATE_FILE}" | head -1 | cut -d'"' -f4 || true)
fi

if [ -z "${RUN_MARKER}" ] && [ -f "${STATE_FILE}" ]; then
  RUN_MARKER=$(grep -o '"runMarker": "[^"]*"' "${STATE_FILE}" | head -1 | cut -d'"' -f4 || true)
fi

echo "=== [CSP Cloud Delivery Isolated Cleanup] 开始受控清理 ==="

# 安全铁律：若缺失明确的 project 或 run-marker，拒绝盲目 down -v 以防误删他人成果
if [ -z "${PROJECT_NAME}" ] || [ -z "${RUN_MARKER}" ]; then
  echo "ERROR: 缺少明确的 project (${PROJECT_NAME:-未指定}) 或 run-marker (${RUN_MARKER:-未指定})！" >&2
  echo "安全规范: 为杜绝误删宿主机上其他并行工作区或他人容器与数据卷，清理必须显式绑定所有权标记。" >&2
  echo "请指定: --project <name> --run-marker <marker> 或保留有效的 ${STATE_FILE}。" >&2
  exit 1
fi

echo "目标隔离 Project: ${PROJECT_NAME}"
echo "验证所有权 Marker: ${RUN_MARKER}"

# 0. 生产容器与数据卷事前快照断言 (Before-Guard)
PROD_CONTAINER_BEFORE=$(docker ps --filter "name=^/clash-sub-parser$" --format '{{.ID}}' 2>/dev/null || true)
if [ -z "${PROD_CONTAINER_BEFORE}" ]; then
  PROD_CONTAINER_BEFORE=$(docker ps --filter "name=^clash-sub-parser$" --format '{{.ID}}' 2>/dev/null || true)
fi

PROD_VOLUME_BEFORE=$(docker volume ls --filter "name=^csp-v1-data$" --format '{{.Name}}' 2>/dev/null || true)

echo ">>> 生产环境现状断言: Container ID='${PROD_CONTAINER_BEFORE}', Volume='${PROD_VOLUME_BEFORE}'"

if [ "${DRY_RUN}" = true ]; then
  echo "[DRY-RUN] 模拟清理验证完毕：已校验 project 与 run-marker 参数有效，未下发真实下线指令。"
  exit 0
fi

cd "${REPO_ROOT}"

# 1. 资源所有权核验 (Ownership Label Verification)
# 检查目标 project 中现存容器，核对其 com.csp.isolated.run-marker 标签
CONTAINER_IDS=$(docker ps -a --filter "label=com.docker.compose.project=${PROJECT_NAME}" --format '{{.ID}}' 2>/dev/null || true)
if [ -n "${CONTAINER_IDS}" ]; then
  for cid in ${CONTAINER_IDS}; do
    c_marker=$(docker inspect --format '{{ index .Config.Labels "com.csp.isolated.run-marker" }}' "${cid}" 2>/dev/null || true)
    if [ -n "${c_marker}" ] && [ "${c_marker}" != "${RUN_MARKER}" ]; then
      echo "FATAL: 目标项目 ${PROJECT_NAME} 中的容器 ${cid} 标签 marker '${c_marker}' 与当前待清理 marker '${RUN_MARKER}' 不匹配！" >&2
      echo "拒绝执行 down -v，以防误删他人或旧实例资源。" >&2
      exit 1
    fi
  done
fi

# 2. 严格仅下线属于当前项目及 marker 的隔离容器与卷
echo ">>> 执行目标隔离项目受控下线: docker compose -p ${PROJECT_NAME} down -v ..."
CSP_ISOLATED_PROJECT="${PROJECT_NAME}" \
CSP_ISOLATED_RUN_MARKER="${RUN_MARKER}" \
CSP_ISOLATED_VOLUME_NAME="csp-isolated-data-${RUN_MARKER}" \
CSP_IMAGE="${CSP_IMAGE:-csp-isolated-placeholder}" \
docker compose -f docker-compose.isolated.yml -p "${PROJECT_NAME}" down -v --remove-orphans || true

# 3. 清理当前预览临时凭据与工件
if [ -d "${PREVIEW_DIR}" ]; then
  echo ">>> 清理隔离预览凭据目录: ${PREVIEW_DIR} ..."
  rm -rf "${PREVIEW_DIR}"
fi

# 4. 生产容器与数据卷事后守护核验 (After-Guard)
PROD_CONTAINER_AFTER=$(docker ps --filter "name=^/clash-sub-parser$" --format '{{.ID}}' 2>/dev/null || true)
if [ -z "${PROD_CONTAINER_AFTER}" ]; then
  PROD_CONTAINER_AFTER=$(docker ps --filter "name=^clash-sub-parser$" --format '{{.ID}}' 2>/dev/null || true)
fi

PROD_VOLUME_AFTER=$(docker volume ls --filter "name=^csp-v1-data$" --format '{{.Name}}' 2>/dev/null || true)

if [ -n "${PROD_CONTAINER_BEFORE}" ] && [ "${PROD_CONTAINER_BEFORE}" != "${PROD_CONTAINER_AFTER}" ]; then
  echo "FATAL: 生产容器 clash-sub-parser 在清理操作中发生变更或意外终止！" >&2
  exit 1
fi

if [ -n "${PROD_VOLUME_BEFORE}" ] && [ "${PROD_VOLUME_BEFORE}" != "${PROD_VOLUME_AFTER}" ]; then
  echo "FATAL: 生产数据卷 csp-v1-data 在清理操作中被意外破坏！" >&2
  exit 1
fi

echo "[ASSERT] 生产容器 clash-sub-parser 持续平稳运行中，未受任何波及。"
echo "[ASSERT] 生产卷 csp-v1-data 完好无损。"
echo "=== [CSP Cloud Delivery Isolated Cleanup] 隔离资源清理完成 ==="
exit 0

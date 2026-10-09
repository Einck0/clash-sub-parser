#!/usr/bin/env bash
# scripts/cloud-delivery/preflight-pull.sh
# CSP 日常云交付镜像拉取与安全确权前置门禁脚本
# 遵循 Fail-Closed 原则：严禁未经验证即宣称通过，严禁将历史公开包用于私有交付。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

DEFAULT_EXPECTED_PACKAGE="ghcr.io/einck0/csp-runtime-private"
LEGACY_PUBLIC_PACKAGE="ghcr.io/einck0/clash-sub-parser"
IMAGE_REF="${CSP_IMAGE:-}"
EXPECTED_REVISION="${EXPECTED_REVISION:-}"
EXPECTED_PACKAGE="${EXPECTED_PACKAGE:-${DEFAULT_EXPECTED_PACKAGE}}"
CHECK_ONLY=false
DRY_RUN=false
CONFIGURE_PAT=false

usage() {
  cat << 'EOF'
用法:
  scripts/cloud-delivery/preflight-pull.sh [选项]

选项:
  --image <ref>               指定待拉取的完整镜像引用（必须包含 @sha256:<64位16进制摘要>）
  --expected-revision <sha>   [必选] 指定期望的代码 Git Commit SHA (40位或64位十六进制)
  --expected-package <pkg>    指定期望的包命名空间（默认: ghcr.io/einck0/csp-runtime-private）
  --configure-pat             通过安全终端（stdin）配置 Classic PAT（需具有 read:packages 权限）
  --check-only                仅校验参数格式、私有仓库可达性与本地凭据，不实际拉取大镜像
  --dry-run                   模拟运行模式
  -h, --help                  显示帮助信息

环境变量:
  CSP_IMAGE                   默认镜像引用
  EXPECTED_REVISION           默认期望 Commit SHA
  EXPECTED_PACKAGE            默认期望包名
  OWNER_PAT                   用于调用官方 API 验证私有性的 Classic PAT (read:packages)
  GHCR_TOKEN                  用于调用官方 API 验证私有性的 Token (备选)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)
      IMAGE_REF="$2"
      shift 2
      ;;
    --expected-revision)
      EXPECTED_REVISION="$2"
      shift 2
      ;;
    --expected-package)
      EXPECTED_PACKAGE="$2"
      shift 2
      ;;
    --configure-pat)
      CONFIGURE_PAT=true
      shift
      ;;
    --check-only)
      CHECK_ONLY=true
      shift
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

# 交互式或 Stdin 配置 Classic PAT
if [ "$CONFIGURE_PAT" = true ]; then
  echo ">>> 配置 GHCR Classic PAT 凭据"
  echo "提示: Classic PAT 必须且仅需具备 'read:packages' 作用域。"
  echo "WARNING: Docker CLI 凭据默认以 base64 明文编码保存在 ~/.docker/config.json，非系统加密密钥环。"

  pat_token=""
  if [ -t 0 ]; then
    printf "请输入 Classic PAT (输入不回显): "
    read -s -r pat_token
    echo ""
  else
    read -r pat_token
  fi

  if [ -z "${pat_token}" ]; then
    echo "ERROR: 输入的 PAT 为空，中止凭据配置。" >&2
    exit 1
  fi

  printf "%s" "${pat_token}" | docker login ghcr.io -u einck0 --password-stdin
  
  if [ -f "${HOME}/.docker/config.json" ]; then
    chmod 0600 "${HOME}/.docker/config.json"
  fi
  echo ">>> Classic PAT 配置完成，~/.docker/config.json 权限已收敛为 0600。"
  exit 0
fi

# 1. 镜像引用与必选 Revision 检查
if [ -z "${IMAGE_REF}" ]; then
  echo "ERROR: 必须提供镜像引用。请使用 --image 选项或设置 CSP_IMAGE 环境变量。" >&2
  exit 1
fi

if [ -z "${EXPECTED_REVISION}" ]; then
  echo "ERROR: 必须提供期望的 revision SHA (--expected-revision)。Revision SHA 必选不得省略！" >&2
  exit 1
fi

# 校验 revision SHA 格式 (case-insensitive hex 40 或 64 位)
if [[ ! "${EXPECTED_REVISION}" =~ ^[0-9a-fA-F]{40}$ ]] && [[ ! "${EXPECTED_REVISION}" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "ERROR: 期望的 revision SHA 格式不合规！必须为 40 位或 64 位十六进制 Git Commit SHA。" >&2
  exit 1
fi

echo "=== [CSP Cloud Delivery Preflight] 开始镜像校验 ==="
echo "待校验镜像: ${IMAGE_REF}"
echo "期望 Revision: ${EXPECTED_REVISION}"

# 2. 严格语法与不可变 Digest 校验 (必须包含 @sha256:<64hex>)
if [[ ! "${IMAGE_REF}" =~ ^([a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*)@sha256:([a-fA-F0-9]{64})$ ]]; then
  echo "ERROR: 不合规的镜像引用格式！" >&2
  echo "规范要求: 镜像必须采用不可变 digest 锁定模式，格式为 <image_name>@sha256:<64位十六进制哈希>。" >&2
  echo "示例: ghcr.io/einck0/csp-runtime-private@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" >&2
  exit 1
fi

PKG_NAME="${BASH_REMATCH[1]}"
DIGEST_HASH="${BASH_REMATCH[3]}"

# 3. 拒绝历史公开包，强制私有命名空间
if [[ "${PKG_NAME}" == *"${LEGACY_PUBLIC_PACKAGE}"* ]]; then
  echo "ERROR: 严禁使用历史公开包 ${LEGACY_PUBLIC_PACKAGE}！" >&2
  echo "安全规范: 历史公开包已具备公开 manifest 且无法在 GHCR 单独转为私有。云交付必须使用专用私有命名空间。" >&2
  exit 1
fi

# 4. 校验目标包名匹配，绝不凭包名含 private 擅自判定通过
if [ -n "${EXPECTED_PACKAGE}" ] && [ "${PKG_NAME}" != "${EXPECTED_PACKAGE}" ]; then
  echo "ERROR: 镜像包名不匹配！期望: ${EXPECTED_PACKAGE}，实际: ${PKG_NAME}" >&2
  exit 1
fi

echo "[CHECK] 镜像包名合规: ${PKG_NAME}"
echo "[CHECK] sha256 digest 格式合规: ${DIGEST_HASH}"

# 5. 校验 GHCR 私有仓库访问与私有性证据 (Dual-Proof: 官方 API private 验证 + 完整匿名探测拦截)
EFFECTIVE_TOKEN="${OWNER_PAT:-${GHCR_TOKEN:-}}"

if [ -z "${EFFECTIVE_TOKEN}" ]; then
  echo "------------------------------------------------------------------"
  echo "STATUS: BLOCKED (缺少 GHCR 鉴权凭据)"
  echo "私有包 ${PKG_NAME} 验证与拉取需要具备 'read:packages' 作用域的 Classic PAT。"
  echo "请在环境中安全提供 OWNER_PAT 或 GHCR_TOKEN (如 export OWNER_PAT='...')"
  echo "当前无可用凭据，保持 Fail-Closed 门禁阻断 (退出码 2)。"
  echo "------------------------------------------------------------------"
  exit 2
fi

echo ">>> 执行官方 GHCR 私有发布与匿名探测门禁验证 (.github/scripts/check_ghcr_private_gate.py) ..."
GATE_SCRIPT="${REPO_ROOT}/.github/scripts/check_ghcr_private_gate.py"
if [ ! -f "${GATE_SCRIPT}" ]; then
  echo "ERROR: 未找到官方私有门禁验证脚本: ${GATE_SCRIPT}" >&2
  exit 1
fi

# 解析 owner
PKG_OWNER="einck0"
if [[ "${PKG_NAME}" =~ ghcr\.io/([^/]+)/ ]]; then
  PKG_OWNER="${BASH_REMATCH[1]}"
fi

set +e
python3 -B "${GATE_SCRIPT}" \
  --mode post-check \
  --image-name "${PKG_NAME}" \
  --owner "${PKG_OWNER}" \
  --tag "sha-${EXPECTED_REVISION}" \
  --digest "sha256:${DIGEST_HASH}"
GATE_EXIT=$?
set -e

if [ $GATE_EXIT -ne 0 ]; then
  echo "------------------------------------------------------------------"
  echo "STATUS: VERIFICATION FAILED (私有门禁校验未通过)"
  echo "官方 check_ghcr_private_gate.py 验证失败 (退出码 ${GATE_EXIT})。"
  echo "可能原因: 包不存在、未设为 private、匿名请求未被拒绝或 PAT 权限不足。"
  echo "严格 Fail-Closed：阻断后续拉取流程。"
  echo "------------------------------------------------------------------"
  exit 1
fi

echo "[CHECK] 官方私有证明验证通过: API 确认为 private 且匿名访问被严密拒绝。"

# 6. Check-only 或 Dry-run 模式直接返回
if [ "${CHECK_ONLY}" = true ]; then
  echo "[CHECK] 官方验证与参数检查全部通过，Check-only 完成。"
  exit 0
fi

if [ "${DRY_RUN}" = true ]; then
  echo "[DRY-RUN] 模拟执行完毕，无实际 pull 操作。"
  exit 0
fi

# 7. 纯拉取操作 (强制 linux/amd64，绝不直接 up 生产)
echo ">>> 执行标准 Docker 平台指定拉取: --platform linux/amd64 ..."
docker pull --platform linux/amd64 "${IMAGE_REF}"

# 8. 拉取后 RepoDigests 与元数据同一性校验
echo ">>> 校验拉取镜像元数据与 RepoDigests ..."
INSPECT_JSON=$(docker image inspect "${IMAGE_REF}")

# 校验平台架构
IMAGE_ARCH=$(echo "${INSPECT_JSON}" | grep -o '"Architecture": "[^"]*"' | head -1 | cut -d'"' -f4)
IMAGE_OS=$(echo "${INSPECT_JSON}" | grep -o '"Os": "[^"]*"' | head -1 | cut -d'"' -f4)

if [ "${IMAGE_ARCH}" != "amd64" ] || [ "${IMAGE_OS}" != "linux" ]; then
  echo "ERROR: 镜像架构不合规！期望 linux/amd64，实际: ${IMAGE_OS}/${IMAGE_ARCH}" >&2
  exit 1
fi
echo "[CHECK] 平台架构确认: ${IMAGE_OS}/${IMAGE_ARCH}"

# 校验 RepoDigests 严格匹配期望 digest
if ! echo "${INSPECT_JSON}" | grep -qi "${DIGEST_HASH}"; then
  echo "ERROR: 镜像 inspect RepoDigests 未找到指定的不可变哈希 ${DIGEST_HASH}！" >&2
  exit 1
fi
echo "[CHECK] RepoDigests 哈希严格一致。"

# 9. 校验不可变 Revision 元数据标签 (必须严格匹配)
echo ">>> 校验镜像 OCI revision 元数据 (期望 SHA: ${EXPECTED_REVISION}) ..."
LABEL_REVISION=$(docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${IMAGE_REF}" 2>/dev/null || true)
if [ -z "${LABEL_REVISION}" ] || [ "${LABEL_REVISION}" = "<no value>" ]; then
  LABEL_REVISION=$(docker inspect --format '{{ index .Config.Labels "revision" }}' "${IMAGE_REF}" 2>/dev/null || true)
fi

# 大小写不敏感校验 hex SHA
if [ "${LABEL_REVISION,,}" != "${EXPECTED_REVISION,,}" ]; then
  echo "ERROR: 镜像 revision 元数据 (${LABEL_REVISION}) 与期望 SHA (${EXPECTED_REVISION}) 不一致！" >&2
  exit 1
fi
echo "[CHECK] OCI revision 标签与源码 SHA 严格一致: ${LABEL_REVISION}"

echo "=== [CSP Cloud Delivery Preflight] 镜像拉取与确权验证全部通过 ==="
echo "注意: 本脚本仅执行预检拉取，绝不直接替换或启动生产容器。"
exit 0

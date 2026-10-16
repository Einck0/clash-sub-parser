#!/usr/bin/env bash
# Verify immutable image identity, not registry visibility. Docker manages auth.
set -euo pipefail

EXPECTED_PACKAGE="${EXPECTED_PACKAGE:-ghcr.io/einck0/csp-runtime-private}"
IMAGE_REF="${CSP_IMAGE:-}"
EXPECTED_REVISION="${EXPECTED_REVISION:-}"
CHECK_ONLY=false
DRY_RUN=false

usage() {
  cat <<'EOF'
用法: scripts/cloud-delivery/preflight-pull.sh --image <package@sha256:64hex> --expected-revision <40或64hex> [选项]
  --expected-package <pkg>  期望的 ghcr.io/einck0 包（默认 csp-runtime-private，历史名称不代表私有）
  --check-only             只验证本地镜像 identity；缺失即失败，不访问注册表
  --dry-run                仅检查参数格式，不拉取、不宣称镜像验证成功
  -h, --help               显示帮助
环境变量: CSP_IMAGE、EXPECTED_REVISION、EXPECTED_PACKAGE。
允许公开匿名拉取，或使用 Docker 正常凭据管理；不读取 config.json 或提取 token。
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image|--expected-revision|--expected-package)
      if [[ $# -lt 2 || -z "$2" ]]; then
        echo "ERROR: $1 缺少参数" >&2; exit 1
      fi
      case "$1" in
        --image) IMAGE_REF="$2" ;;
        --expected-revision) EXPECTED_REVISION="$2" ;;
        --expected-package) EXPECTED_PACKAGE="$2" ;;
      esac
      shift 2 ;;
    --check-only) CHECK_ONLY=true; shift ;;
    --dry-run) DRY_RUN=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "ERROR: 未知参数: $1" >&2; exit 1 ;;
  esac
done

if [[ -z "$IMAGE_REF" ]]; then
  echo "ERROR: 必须提供镜像引用 (--image)。" >&2; exit 1
fi
if [[ -z "$EXPECTED_REVISION" ]]; then
  echo "ERROR: Revision SHA 必选不得省略 (--expected-revision)。" >&2; exit 1
fi
if [[ ! "$EXPECTED_REVISION" =~ ^([0-9a-fA-F]{40}|[0-9a-fA-F]{64})$ ]]; then
  echo "ERROR: revision SHA 格式不合规！" >&2; exit 1
fi
if [[ ! "$EXPECTED_PACKAGE" =~ ^ghcr\.io/einck0/[a-z0-9][a-z0-9_.-]*$ ]]; then
  echo "ERROR: 期望的镜像 owner/包名不合规。" >&2; exit 1
fi
if [[ ! "$IMAGE_REF" =~ ^(ghcr\.io/[a-z0-9_.-]+/[a-z0-9_.-]+)@sha256:([a-f0-9]{64})$ ]]; then
  echo "ERROR: 不合规的镜像引用格式！必须使用完整不可变 sha256 digest，不能使用 tag。" >&2; exit 1
fi
if [[ "${BASH_REMATCH[1]}" != "$EXPECTED_PACKAGE" ]]; then
  echo "ERROR: 镜像包名不匹配！期望: $EXPECTED_PACKAGE" >&2; exit 1
fi
if [[ "$DRY_RUN" == true ]]; then
  echo "[DRY-RUN] 参数格式通过；未验证镜像、未执行 pull。"
  exit 0
fi
if [[ "$CHECK_ONLY" != true ]]; then
  docker pull --platform linux/amd64 "$IMAGE_REF"
fi
# Parse Docker's JSON schema. A digest in an unrelated label is not proof.
docker image inspect "$IMAGE_REF" | python3 -B -c '
import json, sys
ref, revision = sys.argv[1:]
try:
    images = json.load(sys.stdin)
    if not isinstance(images, list) or len(images) != 1:
        raise ValueError("expected exactly one image")
    image = images[0]
    if image.get("Os") != "linux" or image.get("Architecture") != "amd64":
        raise ValueError("expected linux/amd64 platform")
    if ref not in (image.get("RepoDigests") or []):
        raise ValueError("RepoDigests does not contain exact package@digest")
    labels = (image.get("Config") or {}).get("Labels") or {}
    if labels.get("org.opencontainers.image.revision", "").lower() != revision.lower():
        raise ValueError("OCI revision does not match expected source SHA")
except (ValueError, TypeError, AttributeError, KeyError) as error:
    sys.exit("ERROR: image identity verification failed: " + str(error))
print("[CHECK] linux/amd64, exact RepoDigests and OCI revision verified.")
' "$IMAGE_REF" "$EXPECTED_REVISION"
echo "=== 镜像确权验证通过；未启动或替换生产容器 ==="

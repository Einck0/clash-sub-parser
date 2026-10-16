## Why

CSP（Clash Sub Parser）当前处于云交付转型的关键节点。此前在分支 `task/csp-cloud-build-integration` 上的试点工作流（运行 ID `37882921681` 与容器构建 `37882921660`）虽然证实了在 GitHub Actions 云端运行 Node 22 与 Go 1.27.1 纯静态构建的可行性，但旧工作流存在多项关键缺陷：
1. CI 检查与容器构建拆分为独立并行流水线，导致 Go 二进制被重复编译两次，浪费云资源且无法保证镜像二进制与测试二进制的绝对同一性；
2. 镜像包名沿用公开包 `ghcr.io/einck0/clash-sub-parser`，其匿名 manifest 200 已公开暴露，无法通过修改标签实现私有化防护；提议的新包名 `ghcr.io/einck0/csp-runtime-private` 尚需用户确认；
3. 构建工作流包含 `continue-on-error: true` 的静默降级逻辑，发布失败时仍报告绿灯，违反安全收敛（Fail-Closed）原则；
4. 仓库中仅有开发构建配置 `docker-compose.example.yml`，未分离生产镜像只读运行（`image-only` 带确切 digest，严格 `--no-build`）与本地现场编译，生产环境存在误触发重新构建的严重风险；
5. 生产容器真实宿主路径与权限据只读探查为 `/home/service/clash-sub-parser`，且生产数据卷已具备非 root `10001:10001` (750) 权限，但当前尚未获得用户授权的写入与重启权限，必须建立严格的模板解耦与隔离验证门禁。

因此，需要建立独立的 `csp-daily-cloud-delivery` 规范，确立严格的分支保护、单次编译产物复用、私有镜像发布门禁、生产 Compose 解耦与独立隔离验证契约。

## What Changes

- **正式云端 CI 统一与分支门禁**：
  - 合并并统一官方 CI 流水线为单单一 workflow `.github/workflows/ci.yml`，彻底删除旧独立 `build-container.yml`；
  - PR（针对 `main`, `dev`, `task/csp-cloud-build-integration`）权限严格锁定为 `contents: read`，严禁包写入，仅执行全量 checks 作业；
  - 容器发布作业 `publish-runtime` 严格限制触发条件：仅在 `release/**` 分支 push、`v*` 语义版本 tag push，或 `workflow_dispatch` 手动触发且 `inputs.publish_release == 'true'` 时执行；
  - 引入 docs-only 变更过滤与基于 `concurrency` 的同分支过时任务自动取消；
  - 单一 workflow 内严格执行前端 Node 22（`npm ci --include=dev`、type-check、unit test、build）、Go 1.27.1（`go mod verify`、vet、test、静态编译）检查，保留现有 hash 锁定测试工具与 synthetic archive 诊断逻辑；
  - 纯静态编译输出 `csp-linux-amd64` 与 `sha256sum` 校验清单并上传制品。
- **构建二进制复用与轻量运行时镜像**：
  - 容器构建作业直接通过 `actions/download-artifact@v4` 获取 checks 生成的 `csp-linux-amd64` 纯静态二进制（`CGO_ENABLED=0`），建立 `needs: [test-and-build]` 硬依赖，彻底消除云端二次重复编译；
  - 引入独立的 `Dockerfile.runtime`，仅基于 `alpine:3.20` 与非 root `appuser` (10001:10001) 拷贝二进制及必要静态资产，打上 immutable SHA 标签与 OCI revision label；
  - 坚决杜绝在部署现场进行本地源码编译作为 fallback。
- **GHCR 私有包分发与 Fail-Closed 发布门禁**：
  - 确立新私有包名 `ghcr.io/einck0/csp-runtime-private`，旧公开包不删不继续 publish；
  - 仓库变量 `ENABLE_CSP_GHCR_PUBLISH == 'true'` 必须满足；
  - 引入 Secret `OWNER_PAT`（需具备 `read:packages` 最小权限），用于 GitHub Package API 验证包私有性或全分页证明包不存在；缺少 `OWNER_PAT` 时作业 FAIL-CLOSED BLOCKED，绝不降级；
  - 镜像推送使用作业环境 `packages: write` 的 `GITHUB_TOKEN`，不将外部写凭据放入代码库；
  - 匿名探测通过完整 401 challenge 获取 token 并尝试获取 manifest，若能匿名获取则立判失败；
  - ghcr 机器 pull 严格遵循 Classic PAT `read:packages` 权限契约，未获有效凭据前保持门禁阻断，不伪称私有交付通过。
- **生产 Compose 配置解耦与开发覆盖**：
  - 提供生产级 Compose 模板 `docker-compose.prod.yml`（`image-only`，必须声明完整不可变摘要 `${CSP_IMAGE:?required}`，彻底杜绝 `build` 块，执行命令加 `--no-build`）；
  - 本地现场构建完整移至显式 `docker-compose.dev.yml`；
  - 隔离运行配置 `docker-compose.isolated.yml` 采用独立项目 `csp-isolated`、独立端口 18081/17001 与独立全新数据卷 `csp-isolated-data`；
  - 保留生产环境已有的端口映射、volume 命名、环境变量与代理配置；
  - 确认生产目录真实路径为 `/home/service/clash-sub-parser`，数据卷权限为 750 (10001:10001)，本阶段仅做只读 stat 确权，严禁写入现行生产目录或重启容器。
- **Runbook、隔离预览与回滚契约**：
  - 制定基于标准 Docker/Compose 的隔离预备 Runbook：拉取指定 digest，使用隔离 project、回环端口与全新 synthetic DB 卷，严禁挂载真实生产数据与订阅凭据；
  - 成品验收遵循治理流程：经独立 Reviewer 代码审查通过且取得私有镜像后由 preview-preparer 拉起隔离预览，移交 Critic 进行黑盒验证；
  - 生产发布前要求版本影响评估与数据库热备，禁止仅做镜像版本回退而冒称数据库安全。

## Capabilities

### New Capabilities
- `daily-cloud-delivery`: 定义 CSP 云端构建、制品复用、GHCR 私有包安全发布门禁、生产与开发 Compose 配置解耦及隔离验证的完整行为规范。

### Modified Capabilities

无（`openspec/specs/` 下无对应已归档主规格，通过新增能力规格建立权威契约）。

## Impact

- **CI/CD 流水线**：
  - 合并重构 `.github/workflows/ci.yml`，承载完整的前后端检查、测试及单个二进制构建与发布；
  - 删除旧并行流水线 `.github/workflows/build-container.yml`；
  - 移除前端与后端重复测试步骤及 `continue-on-error: true` 宽松标记。
- **容器与配置**：
  - 新增 `Dockerfile.runtime` 用于纯静态运行镜像组装；
  - 新增可追踪生产模板 `docker-compose.prod.yml`（image-only）、开发配置 `docker-compose.dev.yml` 与隔离预览配置 `docker-compose.isolated.yml`；
  - 保持现行生产容器运行不受干扰，仅在模板层面完成解耦。
- **依赖与运行时**：
  - 明确 Go 运行时采用纯 Go SQLite（`modernc.org/sqlite`，`CGO_ENABLED=0`），无底层 CGO 共享库依赖；
  - 锁定 Alpine 3.20 运行时 CA 证书、时区与非 root 用户 `appuser` (10001:10001)。

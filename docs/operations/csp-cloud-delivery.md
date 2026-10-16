# CSP 云交付与隔离验证操作手册

## 1. 范围、当前权威与授权边界

当前需求入口是 [002-policy 当前交付修订](../../specs/002-policy/spec.md)、[实施计划](../../specs/002-policy/plan.md) 和 [任务](../../specs/002-policy/tasks.md)。`openspec/` 仅可作为历史材料，不是当前活动规格；不要导入远端旧规格覆盖本地 Spec Kit。

完整交付已取得有条件授权，包括通过全部门禁后的目标生产替换与重启。最新用户“没必要私有啊”明确允许公开镜像，撤销私有 visibility/API/匿名拒绝门禁，不撤销独立审查、编译质量、不可变制品验证与敏感数据保护。本轮只收敛发布源与本地证据，不修改 ignored 的 owner 生产配置 `docker-compose.yml`，不触碰生产数据库、不 refresh 真实订阅、不启停生产容器。

已确认远端 `origin/dev` 有云交付与工具链历史，本地有尚未提交的业务改动。导入正规交付文件不表示已合并分支。后续必须先完成独立审查、按明确授权提交源码，再在独立 integration tree 正常合并并保留 `origin/dev` 祖先；不在 dirty 工作树 merge/rebase，不 force push。具体提交、双亲与归档冲突处理记录在本次交付的 `integration-plan.json`，不把旧 SHA 写成未来发布版本。

### 当前验收边界

- 镜像命名全面标准化为公开包 `ghcr.io/einck0/clash-sub-parser`。先前过渡期使用的历史包名 `ghcr.io/einck0/csp-runtime-private` 及其人工私有门禁已废除。CSP 容器镜像与源码均公开，不包含任何机密或敏感数据（数据库与 Token 为外部挂载/环境变量传入），严禁擅自删除历史公开包。
- 镜像发布允许匿名拉取；发布不依赖 OWNER_PAT 或 GitHub Packages visibility API；使用既有 Actions `GITHUB_TOKEN` 实际 push/read 证明权限。先前私有门禁阻断与通知属于被新授权覆盖的历史事实，不再作为当前阻断。
- 宿主机零编译红线：宿主机严禁执行 `go build`、`go test`、`npm run build`、`npm test` 等重型编译操作。生产环境严格通过预构建容器镜像拉取部署。
- `auto_test` 尚未完整实现，不能把兼容导入或本地绿色测试称作完整产品验收。
- 本地合成预览可由当前源码构建，但不证明云镜像已发布或已上线。独立 reviewer、主脑风险核对、适用 Critic、真实云构建/push/pull/digest 和生产验收仍是独立步骤。

## 2. Same-SHA 构建链与官方入口

唯一官方 CI 为 [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)：

1. 固定 checkout `${{ github.sha }}`；Node 22 下 `web/npm ci --include=dev`、type-check、unit tests 和 build。
2. 清空 **CI runner 内** `internal/webassets/dist`，完整复制该次 `web/dist`，避免混入旧 bundle。不要在正式工作树提前刷新 embedded assets。
3. 按 `go.mod` 设置 Go，执行 mod verify、带 `with_quic with_wireguard` 的 vet/tests，安装 CI 测试所需协议二进制、准备合成 legacy archive。
4. `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags "with_quic with_wireguard"` 仅编译一次到 `bin/csp-linux-amd64`；通过 `-X clash-sub-parser/internal/platform.Commit=<sha> -X clash-sub-parser/internal/platform.BuildDate=<RFC3339>` 接入实际 CLI/platform 元数据，并检查 `--version`。
5. 校验 static ELF（readelf 失败或存在 INTERP 都失败），生成 `csp-linux-amd64.sha256`、上传 artifact。
6. publish job 下载并验算同一个 binary，再检查 static ELF 与 `--version` 的同 SHA；交给 [Dockerfile.runtime](../../Dockerfile.runtime)。此镜像不包含 Go/Node 编译过程，不重复构建前端或 Go；只 COPY 编译后二进制，不带配置、数据库、维护清单或秘密。
7. OCI `COMMIT_SHA`/`BUILD_DATE` 表示源码版本和镜像打包时间，二进制 buildTime 为上一步实际编译时间；两者不必相等。镜像绑定 `sha-<完整SHA>` 和 run 标签，交付使用 `@sha256:<digest>`，不使用可变 latest。

`dev` push 自动执行完整 Go/web 检查与编译；只有 `test-and-build` 成功，`publish-runtime` 才下载该 run 的同一个 binary/SHA256 artifact，构建并 push 公开允许的历史包、真实 pull 不可变 digest 并验证 OCI revision。随后仅 `promote-prelease` job 创建 GitHub **prerelease entity** `prelease-<tested-dev-SHA前7位>`，显示名相同、target 为完整 `${{ github.sha }}`；不创建 `prelease`/`prerelease` 分支。附加原 binary、原 SHA256 和 `release-metadata.json`（source_sha、repository、tag、artifact、binary_sha256、image完整@digest、image_digest、run_id/run_attempt/run_url）。不重编译、不自动切换生产。

同 SHA 的 dev workflow 全链串行且不取消在途发布，重跑仍执行 Go/web 检查；若已有完整 prerelease，则先验证精确 tag→commit、target SHA、资产清单、binary/SHA256 和 metadata，复用原 binary/digest/run identity，不重建或覆盖 immutable 资产。随后重新真实 pull/OCI 验证。短 SHA 冲突、孤立 tag、不完整 release、API 权限/服务错误均 fail-closed，保留现场，不能 `--clobber` 或移动 tag。完整性失败不能晋级；首次发布失败产生的残留需人工核查，流水线不自动删除重试。

分支治理与触发门禁收敛：仅 `dev` 分支的 push 与 PR（指向 `dev`）在云端触发构建与检查；`main`、`task/*`、`release/*`、`cloud-build/*` 及 `v*` tag 均已彻底退出 CI 触发入口。`workflow_dispatch` 即使从其他 ref 手动发起，`test-and-build` 入口 job 也会跳过；只有 `dev` ref 执行检查，并仅在 `publish_release='true'` 时开启镜像发布。`main` 分支的晋级通过严格的正常 Fast-Forward 将已完全通过测试的 `dev` SHA 推进，复用同一 commit 已由 GitHub Actions（App ID 15368）跑出的 `Backend & Frontend Checks` 绿标，不在 `main` 上重复编译，避免检查死锁。移除了 `paths-ignore` 过滤规则（历史上 Tag push 不应用 `paths-ignore`，现 dev CI 全量无过滤运行，确保纯文档改动亦能获得必选检查状态）。删除旧 `ENABLE_CSP_GHCR_PUBLISH` 私有流程开关，不修改远端变量。只有 publish job 具备 `packages: write`，只有实际 prerelease job 具备 `contents: write`；其余/default 为 contents read。正常 Actions `GITHUB_TOKEN` 执行 gh create/download，无本机 token/OWNER_PAT 前提。发布 concurrency 不取消在途 release。`prelease-*` 不触发循环流水线。

本地开发 [Dockerfile](../../Dockerfile) 是独立多阶段构建入口，使用官方 npm HTTPS lockfile、构建 tags 和同样的 platform.Commit/platform.BuildDate 链接参数（以 COMMIT_SHA/BUILD_DATE 构建参数传入），不经 main 别名或 init 动态覆盖；它不是生产现场编译入口。

### 开源行业标杆对照（Prior Art）

针对构建、容器设计与交付模型，对比成熟的 Go + Web 全栈开源项目（Gitea、Mihomo / Sing-box、Caddy），确立标准交付规范：

| 标杆项目 | Dockerfile 设计 | CI/CD 构建流 | 镜像命名与可见性 | 宿主机部署模型 |
| :--- | :--- | :--- | :--- | :--- |
| **Gitea** (Go + Vue) | 多阶段 `Dockerfile` (Node 构建前端 -> Go 构建二进制 -> Alpine/Rootless 运行时，固定 UID/GID)。 | GitHub Actions 编译二进制发布 Release，构建多架构容器推送至 GHCR / Docker Hub。 | 公开 `ghcr.io/go-gitea/gitea`，标签 `:latest`、`:version`、`:dev`。无 `-private` 命名。 | 服务器通过 Docker Compose 拉取预构建镜像。生产宿主绝不执行本地编译。 |
| **Mihomo / Sing-box** (Go 核心 + Web 控制台) | 独立容器用多阶段构建；CI 构建发布二进制后单阶段打包镜像以降低 Runner 负载。 | GitHub Actions 响应 `dev`/`main` 推送，跑全量测试矩阵，发布 prerelease 与标签。 | 公开 `ghcr.io/metacubex/mihomo`、`ghcr.io/sagernet/sing-box`。 | 终端与服务器拉取制品或镜像，零临时远端分支蔓延。 |
| **Caddy** (Go + 插件) | 多阶段 `builder` + 极简 `alpine` 运行时。自定义构建在本地或官方镜像中使用 `xcaddy`。 | CI 自动化构建 Release，发布带 SHA256 校验和的二进制并推送官方镜像。 | 公开 `caddy:latest`、`caddy:alpine`。 | 生产拉取预构建镜像或官方二进制。 |
| **CSP（标准化交付）** | 双 Dockerfile 架构：多阶段 `Dockerfile` 供独立复现；轻量 `Dockerfile.runtime` 供 CI 制品直装。 | 推送 `dev` 触发 CI，测试一次性静态二进制，直装镜像推送到 GHCR，发布 Prerelease。废除废弃工作流 `pages.yml`。 | 标准化公开包 `ghcr.io/einck0/clash-sub-parser`，标签 `:sha-<fullsha>`、`:prelease-<shortsha>`。 | 宿主机严格拉取预构建镜像（零本地编译）。开发者直推 `dev`，绝不在 origin 创建临时分支。 |

### 双 Dockerfile 设计原则

1. **标准多阶段 `Dockerfile`**：
   - 包含完整三阶段（`node:22-alpine` -> `golang:1.27.1-alpine` -> `alpine:3.20`）。
   - 为开发者或外部自动化构建系统提供“开箱即用”的标准容器构建能力，零外部工具链依赖。
2. **轻量 CI 制品打包 `Dockerfile.runtime`**：
   - 基于 `alpine:3.20`，直接 `COPY` CI Runner 中已经过 vet、单元测试与 static ELF 校验的 `bin/csp-linux-amd64`。
   - 避免在容器构建阶段二次执行 `npm ci && npm run build` 和 `go build`，消除 Runner 负载冗余，并保证“测试的二进制”与“容器内的二进制”100% 字节一致。
3. **一致的运行时安全约束**：
   - 两个 Dockerfile 共享相同的运行时特征：Alpine 3.20 基础镜像、非 root 用户 `appuser` (UID/GID 10001:10001)、工作目录 `/app`、数据卷 `/data` (权限 750)、服务端口 18080 以及 `/healthz` 探针。
4. **废除冗余工作流**：
   - 彻底删除历史废弃的 `.github/workflows/pages.yml`。CSP 前端控制台直接通过 Go `//go:embed` 嵌入二进制并在端口 18080 提供服务，无需且不再维护独立的静态 GitHub Pages。

### 本地验证

```bash
# 本机轻量验证（不触发真实 Go 双 build 用例）
python3 -B -m unittest discover -s tests/cloud-delivery -p 'test_cloud_delivery.py' -v
python3 -B .github/scripts/test_ci_synthetic.py
# 必须显式使用 scratch 路径；现存 archive 会被拒绝覆盖。
python3 -B .github/scripts/setup_test_archive.py "${TMPDIR:-/tmp}/csp-synthetic-archive.db"
# tests/cloud-delivery/test_build_metadata.py 仍保留，执行真实 CI/Docker Go 双 build；
# 本机按用户要求不运行。此 metadata 回归及 Go/web 编译、vet、测试均在 dev Actions 执行。
```

Synthetic CI suite 使用 Python unittest/PyYAML、Compose config 和 buildx definition check；registry/GitHub prerelease 响应全部 mock，不调用真实发布 API。覆盖 dev 成功依赖、PR/失败不发布、同 artifact/exact-SHA、不可变重跑、metadata digest 与拒绝覆盖；不把模拟权限视为云端实测。Compose config 不启动服务。真实 archive 不是测试输出路径；旧 Go archive tests 仍有历史固定路径约束，只在已具备隔离测试宿主时运行，不用本机真实备份填造夹具。

## 3. 公开允许的不可变 identity 门禁（fail-closed）

Active CI 不调用 [check_ghcr_private_gate.py](../../.github/scripts/check_ghcr_private_gate.py)。该文件及其 mock 测试仅保留历史私有工具语义，不代表当前发布策略，也不被改成无条件成功。

1. Buildx push 必须产生 `sha256:<64hex>` digest，远端 primary tag 的 manifest digest 必须与它相等。
2. [preflight-pull.sh](../../scripts/cloud-delivery/preflight-pull.sh) 约束固定 owner `einck0`、预期包、完整 source revision 与不可变引用，拒绝 mutable tag/错误 owner/短 digest。
3. 真实 `docker pull --platform linux/amd64 package@digest` 后解析 Docker inspect JSON，必须是单镜像、linux/amd64、`RepoDigests` 精确包含完整 package@digest，且 OCI `org.opencontainers.image.revision` 等于预期 source SHA。其他 label 中出现 digest 不算证明；缺 revision/拉取失败均失败。
4. PUBLIC/匿名 pull 成功不是 failure。Docker 使用匿名访问或正常 credential helper/config；脚本不读取 config 原文或提取 token，不要求私有 API/PAT。

```bash
# 显式授权的实际拉取与 identity 验证，不启动容器
scripts/cloud-delivery/preflight-pull.sh --image "$APPROVED_IMAGE_DIGEST" \
  --expected-revision "$REVIEWED_SOURCE_SHA"
# 已缓存镜像的本地 identity 验证；缺失或不一致即非零
scripts/cloud-delivery/preflight-pull.sh --image "$APPROVED_IMAGE_DIGEST" \
  --expected-revision "$REVIEWED_SOURCE_SHA" --check-only
```

`--dry-run` 只验证格式，绝不声称已验证镜像。默认包使用标准化公开名称 `ghcr.io/einck0/clash-sub-parser`，历史名称 `ghcr.io/einck0/csp-runtime-private` 已废弃，严禁擅自删除历史公开包。需要其他同 owner 包时必须显式传 `--expected-package`，不修改默认发布 scope。

严禁向用户索取明文 Token。如普通 Docker 拉取需要凭据，由操作者在安全终端自行使用 `docker login --password-stdin`（拉取最小 `read:packages`），不由本脚本配置。Docker config 的 base64 不是加密，文件应为 0600；不读取、打印或提取现存凭据，不将其写进报告。

## 4. Compose 职责与不变量

| 文件 | 职责 | 约束 |
| --- | --- | --- |
| [docker-compose.dev.yml](../../docker-compose.dev.yml) | 明确 local build | 独立 dev 容器/卷；不用于生产 rollout |
| [docker-compose.prod.yml](../../docker-compose.prod.yml) | image-only 发布模板 | `CSP_IMAGE` 必填，无 build；保留 loopback 17000/18080 → 18080、`csp-v1-data`、host proxy extra_hosts |
| [docker-compose.isolated.yml](../../docker-compose.isolated.yml) | 合成隔离预览 | 独立 project/端口/全新卷、run-marker 标签、无 restart，无 build |
| `docker-compose.yml` | ignored owner 生产配置 | 本轮不修改；模板不自动替换此文件 |

生产 owner 路径为 `/home/service/clash-sub-parser`；历史记录的非 root UID/GID 是 10001:10001、卷目录权限 750。它们不是永远有效的动态证据，正式运维前只读核对，不能据此自动 chown/chmod。生产端口 17000/18080 不供自动测试或预览使用。

隔离 Compose 默认命令包含 `--disable-subscription-scheduler --disable-probe-coordinator`，两个后台协调器均被真正禁用；internal Docker network 阻止外部连通。代理为空不等于禁网，不能只靠清空代理宣称隔离。不要导入真实 source、生产 DB 或默认启用 copy 后的订阅定时任务。

## 5. 隔离启动、交接与受控清理

仅在镜像门禁通过且获准运行容器时：

```bash
export CSP_PREVIEW_DIR="${TMPDIR:-/tmp}/csp-preview-owned-run"
scripts/cloud-delivery/run-isolated.sh --image "$APPROVED_IMAGE_DIGEST" \
  --project csp-isolated-owned-run --run-marker owned-run --check-only
# 获准启动后去掉 --check-only；脚本强制 up -d --no-build。
```

端口默认 18081/17001，必须确认未占用且不使用生产 17000/18080。脚本仅使用本地已就绪镜像，不盲 pull。`CSP_PREVIEW_DIR` 支持每次独立 scratch，避免覆盖他人预览。

浏览器交接 `instance.json` 必须包含 `instanceId/url/health/allowedOrigins/sourceManifest/testDataBoundary/scenarios/shutdownOwner/cleanupToken`。同时保留 PID 或 container identity、snapshot/binary hash、healthz/readyz 和合成数据边界证据；只交付当前 UI，不把旧 embedded assets 称作当前实现。源码私有 snapshot 可嵌入已验证的当前 web-dist，正式 embedded assets 保持不动。

准备者负责健康实例与清理移交，不扮演 Critic。Critic 需工程门禁满足后由主脑独立派发，读取多视口 PNG 并给出真实视觉结论。退出码 0 或 healthy 不等于 Critic PASSED。

容器清理由 [cleanup-isolated.sh](../../scripts/cloud-delivery/cleanup-isolated.sh) 按 project/run-marker 校验防误删；先 `--dry-run`，禁止不明目标 `down -v`。源码本地预览不使用此 Docker 清理脚本，应按交接 PID、命令和 cleanupToken 定向停止，主脑接手后负责清理。不得清理生产服务、卷或其他会话资源。

## 6. 生产部署与回滚（另获授权才执行）

本轮不部署、不备份生产。后续操作参照 [现有运维入口](../operations.md) 与 [受控部署手册](../controlled_deployment_runbook.md)，冻结 reviewed source SHA、binary SHA256、image digest、旧 digest、owner Compose 路径和兼容 schema。

SQLite WAL 不能用裸文件 copy 当在线备份：另获授权后用容器内 sqlite3 `.backup` 生成一致性快照，在**相同容器路径命名空间**检查非空、`PRAGMA schema_version;`、`PRAGMA integrity_check;` 为 ok、`PRAGMA foreign_key_check;` 为空。不要用宿主 `test -s` 检查容器内路径，也不要自动写入不存在/未挂载的备份目录。

默认回滚是将 owner 配置恢复到前一已知良好 digest 并 `up -d --no-build`，仅当 schema 向后兼容时适用。确需数据库恢复时必须先停目标服务、保存故障 DB/WAL/SHM、验证备份、按已核实挂载点恢复与权限，再启动旧 digest；不要在在线服务上直接移除 WAL/SHM。恢复后核验 healthz、readyz 与关键数据行为。目标生产写入与重启已有完整交付的有条件授权，但只有独立工程/视觉门禁、官方云构建 image/digest、owner 配置及远端历史前提均核实后才可执行；本轮尚未发布或部署，不能用本地构建替代云制品验收。

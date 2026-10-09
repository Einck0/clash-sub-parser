## Purpose

定义 CSP 日常云交付流水线、镜像构建复用、GHCR 私有包发布门禁、生产与开发 Compose 配置分离、生产目录确权及无损隔离验收规范，保障自动化交付的完整性、安全性与可回滚性。

## ADDED Requirements

### Requirement: 统一官方云端 CI 流水线与分支并发门禁
系统 SHALL 在官方 CI 工作流中实现分支接入、路径过滤与并发控制。当代码推送到受保护分支或针对受保护分支创建 Pull Request 时，系统 MUST 仅触发只读 checks 作业，禁止未授权的包写入。工作流 SHALL 配置 concurrency 取消同一分支上的过时运行，并对仅包含文档或规范变更的提交跳过重量级构建。CI checks 作业 MUST 严格包含 Node 22 前端校验与构建、Go 1.27.1 依赖验证、静态代码检查（vet）、锁定工具与测试，以及 linux/amd64 二进制纯静态编译，并上传编译二进制制品。

#### Scenario: Pull Request 触发只读 CI 校验
- **WHEN** 开发者向目标受保护分支提交 Pull Request
- **THEN** 系统自动触发 CI checks 作业，`permissions` 严格限制为 `contents: read`，执行全量前后端测试与静态检查，成功后上传 linux/amd64 二进制制品，绝不触发镜像发布

#### Scenario: 同分支多次推送并发取消
- **WHEN** 开发者在短时间内向同一分支连续推送多次提交
- **THEN** GitHub Actions concurrency 机制自动取消之前仍在执行中的旧任务，仅保留最新提交的 CI 运行

#### Scenario: 仅修改文档跳过重型测试
- **WHEN** 提交仅包含 `.md` 文档或注释修改
- **THEN** CI 路径过滤器自动判定并跳过前后端重型编译与测试作业

### Requirement: 单一二进制产物复用与轻量运行时镜像
容器镜像构建作业 SHALL 直接拉取并复用 CI checks 生成的相同 SHA 二进制产物，MUST NOT 在容器构建阶段再次执行源码重复编译。系统 SHALL 维持纯 Go 无 CGO 契约（`CGO_ENABLED=0`，基于纯 Go SQLite），并在独立于完整构建 Dockerfile 的 `Dockerfile.runtime` 中组装镜像。运行时镜像 MUST 仅拷贝通过检查的二进制文件与必要静态运行时文件，采用非 root 用户（UID 10001:appuser）和标准基础镜像（alpine:3.20），且打上不可变 SHA 标签（`sha-<commit>`）及 OCI revision 元数据标签。

#### Scenario: 容器构建复用已有构建制品
- **WHEN** CI checks 成功生成并上传 `csp-linux-amd64` 二进制制品且发布作业触发
- **THEN** 镜像构建步骤直接下载该制品并装配入 `Dockerfile.runtime`，构建日志中不得出现 `go build` 或 `npm run build` 的重新编译过程

#### Scenario: 本地现场重编译阻断
- **WHEN** 生产或云发布环境未获得经过完整 checks 验证的二进制制品
- **THEN** 系统严格阻断构建流程，SHALL NOT 降级采用本地即时重编译作为 fallback

### Requirement: GHCR 私有包发布与 Fail-Closed 门禁
系统针对正式容器发布，SHALL 提议并采用独立的私有包名（如 `ghcr.io/einck0/csp-runtime-private`），MUST NOT 向已公开且匿名可拉取（manifest 200）的历史公开包名推送。在用户未显式确认包名并授予发布凭据前，发布步骤 MUST 保持门禁阻断。正式发布作业 MUST 实施 Fail-Closed 机制：若 Docker push 失败、未生成确切 sha256 镜像 digest、私有包匿名 token/manifest challenge 探测发现可匿名拉取、未配置仓库变量 `ENABLE_CSP_GHCR_PUBLISH=true`，或缺少仓库 Secret `OWNER_PAT`（需具备 `read:packages` 作用域），系统 MUST 立即置作业为失败（exit code 1），SHALL NOT 降级或使用 `continue-on-error: true` 掩盖失败。GHCR 机器拉取契约 SHALL 明确要求 Classic PAT `read:packages`，在缺乏有效凭据时不得声称私有交付通过。

#### Scenario: 发布权限缺失或推送失败触发硬门禁
- **WHEN** 镜像推送至 GHCR 遭遇权限不足、缺少 OWNER_PAT 或网络失败
- **THEN** 工作流执行立即失败且退出码非 0，阻断后续任何发布流程，禁止报告成功或伪造绿灯

#### Scenario: 私有镜像匿名访问探测
- **WHEN** 镜像成功推送至新私有包名
- **THEN** 系统通过完整 401 challenge 验证匿名请求无法获取 manifest，且经认证的 Classic PAT（具备 `read:packages`）能够凭借确切 digest 拉取镜像

### Requirement: 生产与开发 Compose 配置解耦与无构建保障
系统在仓库模板中 SHALL 严格解耦生产运行配置与本地开发配置。生产 Compose 规范 MUST 采用 `docker-compose.prod.yml`（`image-only` 模式），必须显式引用完整不可变镜像 digest 环境变量（`${CSP_IMAGE:?required}`），禁止包含 `build` 语法块；执行拉取与启动时 MUST 使用 `--no-build` 选项，严禁在生产环境自动回退至现场编译。开发环境的源码构建配置 MUST 隔离移至显式 `docker-compose.dev.yml`。隔离预览环境 MUST 采用 `docker-compose.isolated.yml`（独立 project、端口 18081/17001、独立测试卷）。生产环境的端口映射、数据卷、环境变量及宿主代理配置 SHALL 保持与线上一致。

#### Scenario: 生产配置执行启动防止现场构建
- **WHEN** 运维使用生产 Compose 模板执行 `docker compose up -d --no-build`
- **THEN** 容器引擎仅依据声明的镜像 digest 从私有镜像库拉取并启动，任何尝试就地读取 Dockerfile 编译的行为均被禁止

#### Scenario: 本地开发需要源码调试
- **WHEN** 开发者需要在本地修改代码并即时运行
- **THEN** 开发者显式指定开发 Compose 配置文件 `docker-compose.dev.yml`，启用本地构建上下文

### Requirement: 生产环境真实宿主目录与权限确权
系统在执行任何生产替换、文件更新或服务重启前，SHALL 重新读取运行中容器的真实 labels 与宿主环境元数据，以确权实际生产目录（`/home/service/clash-sub-parser`）与容器名称，MUST NOT 盲目推测当前 git 仓库即为生产运行目录。系统 SHALL 通过只读 `stat` 验证生产数据卷具备非 root UID 10001 访问权限，MUST NOT 在生产卷上执行 `chown`。在未获得用户显式 QQ 确认及相应生产写入/重启权限前，施工活动 MUST 严格局限于隔离工作区及模板定义，SHALL NOT 写入生产目录或触发生产容器重启。

#### Scenario: 生产真实宿主探查确权
- **WHEN** 系统执行生产准备检查
- **THEN** 系统通过容器 inspect / labels 识别实际工作目录与运行实例，记录宿主路径与只读权限状态，在未获授权时将变更局限于代码仓库内模板

### Requirement: 隔离预备 Runbook 与有保障的验收回滚机制
系统 SHALL 提供标准的 Docker/Compose 隔离运行与验证 Runbook。在拉取确切 digest 镜像后，验证环境 MUST 运行在完全隔离的 Docker project、独立本地回环端口（loopback）与全新合成数据库数据卷（synthetic DB volume）中，严禁挂载真实生产数据卷或连接真实外部订阅凭据网络。清理脚本 MUST 具备事前与事后生产容器及卷名称守护断言。成品验收 MUST 遵循治理规程：仅在独立 Reviewer 代码审查通过后，由 Critic 执行黑盒验收；若无可用隔离 URL 或权限，MUST NOT 裁决为 PASSED。生产发布替换前 MUST 执行数据库热备份及版本影响评估，禁止仅凭镜像版本回滚冒称数据库与系统安全。

#### Scenario: 隔离预览环境拉起与健康检查
- **WHEN** 待测镜像通过构建并准备黑盒验证
- **THEN** 隔离运行脚本在动态本地端口拉起带有独立合成数据卷的容器实例，等待 `/healthz` 健康检查通过并输出标准实例信息，供只读验收使用

#### Scenario: 生产更新失败时执行确定性回滚
- **WHEN** 生产发布后遭遇异常需要回退
- **THEN** 运维根据 Runbook 恢复此前备份的生产数据库，并将镜像切换回上一个已知稳定的 immutable digest，确保数据与镜像状态完全一致

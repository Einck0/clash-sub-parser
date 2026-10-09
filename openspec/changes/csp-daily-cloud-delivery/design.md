## Context

CSP（Clash Sub Parser）当前运行在分支 `task/csp-cloud-build-integration`（HEAD `5e3b8fb`），相较于上游 `origin/dev`（`1c5a604`）包含 8 个独立特性提交。此前在 GitHub Actions 上的试点运行（CI 检查 `37882921681` 与容器构建 `37882921660`）虽然验证了 Node 22 和 Go 1.27.1 构建的可行性，但也暴露了架构上的关键缺陷：
1. 双重独立工作流导致相同的 Go 二进制被重复编译两次，浪费资源且存在二进制不一致风险；
2. 旧公开包 `ghcr.io/einck0/clash-sub-parser` 匿名可拉取（manifest 200），无法在 GitHub 现有机制下直接设为私有，亟需迁移至新私有包名；
3. 构建工作流包含 `continue-on-error: true` 宽松标记，发布失败时仍报告绿灯，违反安全收敛（Fail-Closed）原则；
4. 仓库中仅有包含 `build` 上下文的开发示例 Compose 配置，缺少声明确切 digest、禁止本地编译的生产专用运行配置；
5. 只读探查表明生产运行实例目录位于 `/home/service/clash-sub-parser`，生产数据卷权限经实测为 750 (UID 10001:10001)，但当前未获得用户针对该目录的写入或重启授权。

参见 `proposal.md` 了解更详细的背景与动因。

## Goals / Non-Goals

**Goals:**
- **统一构建流水线**：单单一 workflow `.github/workflows/ci.yml`，PR 触发只读 checks，push 受信事件复用生成的 `linux/amd64` 纯静态二进制制品，消除二次编译。
- **轻量独立运行时镜像**：提供 `Dockerfile.runtime`，仅基于 Alpine 3.20 拷贝校验完毕的二进制与必要资产，非 root 用户运行 (UID 10001:10001)。
- **Fail-Closed 发布门禁**：确立新私有包名 `ghcr.io/einck0/csp-runtime-private`，严格校验 push 结果与 digest，要求 `ENABLE_CSP_GHCR_PUBLISH=true` 与 Secret `OWNER_PAT`，执行完整匿名 challenge 探测，拒绝任何静默降级。
- **生产与开发 Compose 配置解耦**：生产配置 `docker-compose.prod.yml` 采用 `image-only` 模式，必须包含完整不可变摘要 `${CSP_IMAGE:?required}`，使用 `--no-build` 启动；开发现场构建移至显式 `docker-compose.dev.yml`，隔离预览使用 `docker-compose.isolated.yml`。
- **宿主目录确权与隔离验收 Runbook**：锁定真实生产目录为只读，通过独立 project、回环端口与合成数据卷提供安全的隔离验证与热备回滚流程。

**Non-Goals:**
- 本阶段不执行任何远程 GHCR 镜像推送或拉取（等待用户确认包名与提供 PAT 凭据）。
- 本阶段不触碰、不写入生产目录 `/home/service/clash-sub-parser`，不执行 chown，不重启生产容器。
- 本阶段不执行向 `origin/dev` 或 `origin/main` 的 git push 或 merge。

## Decisions

### 1. 单一流水线二进制制品复用 (Single Pipeline Artifact Reuse)
- **选择**：在单个 GitHub Actions 工作流 `.github/workflows/ci.yml` 中，`test-and-build` 作业完成全量检查并编译输出 `csp-linux-amd64` 二进制后，通过 `actions/upload-artifact@v4` 保存制品；后续的 `publish-runtime` 镜像打包作业直接通过 `actions/download-artifact@v4` 获取该二进制。删除旧独立 `build-container.yml`。
- **理由**：保证镜像中的二进制与经过完整测试验证的二进制完全逐字节一致，杜绝二次编译可能产生的环境或依赖漂移，同时减少近一半的 CI 计算耗时。
- **备选方案**：保留两个独立流水线——已被废弃，因为二次编译无法保证严格一致性。

### 2. 零 CGO 纯静态运行时契约 (Zero-CGO Pure Static Runtime)
- **选择**：严格维持 `CGO_ENABLED=0`，基于纯 Go 实现的 `modernc.org/sqlite` 进行编译。
- **理由**：后端已完成纯 Go SQLite 改造，生成纯静态二进制，不依赖宿主机或 Alpine 的 glibc/musl 动态链接库，在各类 Linux 发行版与容器环境中均可免依赖运行，消除潜在的 CGO 内存泄漏与动态库兼容风险。
- **备选方案**：启用 CGO 编译——不仅增加构建环境复杂度，还会导致跨平台移植困难，被否决。

### 3. 私有镜像包名与 Fail-Closed 发布门禁 (Private GHCR Namespace & Fail-Closed Gate)
- **选择**：提议新镜像包名 `ghcr.io/einck0/csp-runtime-private`，旧包保持现状不再推送；发布流程中移除所有 `continue-on-error: true`，校验 Docker push 退出码、提取镜像 digest；引入 `ENABLE_CSP_GHCR_PUBLISH == 'true'` 开关与 `OWNER_PAT` (read:packages) 强门禁；通过完整 401 challenge 探测拒绝匿名 manifest 获取。
- **理由**：旧公开包历史 manifest 200 无法修改为私有；新私有包名正在等待用户 QQ 确认，在未获正式凭据前发布步骤严格阻断，任何发布失败必须导致流水线置红。
- **备选方案**：尝试删除旧包或在旧包上修改设置——存在历史依赖破坏风险且不符合 GitHub 现有包管理规则，被否决。

### 4. 生产配置 Image-Only 与本地构建解耦 (Production Image-Only & Dev Build Separation)
- **选择**：生产 Compose 模板 `docker-compose.prod.yml` 严格仅包含 `image: ${CSP_IMAGE:?required}`，彻底移除 `build` 块；启动脚本增加 `--no-build` 选项；本地源码调试使用独立 `docker-compose.dev.yml`，隔离预览使用 `docker-compose.isolated.yml`。
- **理由**：防止生产环境在镜像拉取异常或网络波动时意外触发本地重新编译，污染生产环境或产生未经测试的镜像。
- **备选方案**：统一使用单个带有 `build` 的 compose 文件配合环境变量——极易因配置遗漏触发本地编译，被否决。

### 5. 生产宿主探查与零生产变更原则 (Production Host Scout & Zero Production Mutation)
- **选择**：通过只读探查确认运行中容器的真实宿主目录为 `/home/service/clash-sub-parser`，数据卷权限为 750 (10001:10001)，本任务所有操作严格局限于隔离工作区 `/tmp/csp-cloud-build-wt`，不向生产目录写入任何文件，不执行 `chown`，不执行 `docker restart`。
- **理由**：恪守治理纪律，生产变更必须具备明确授权并在独立审查与 Critic 验收通过后方可执行。

## Risks / Trade-offs

- **[Risk] GitHub Actions 制品下载在 job 间失败** → 在同一 workflow run 内部使用 `actions/upload-artifact@v4` 与 `download-artifact@v4`，并设置合理的重试与 retention-days。
- **[Risk] 用户尚未在 QQ 确认新包名或提供 Classic PAT** → 保持本地模板开发完备，将发布操作保留在门禁阻断状态，报告中明确列为外部依赖阻塞项。
- **[Risk] 生产更新后数据库结构异常无法纯镜像回退** → 在 Runbook 中强制规定发布前必须对 SQLite 数据库进行热备份，回滚时同步恢复数据库文件。

## Migration Plan

1. **第一阶段（当前）**：在隔离工作树中完成 OpenSpec 独立 change（`csp-daily-cloud-delivery`）的规约落盘、统一 CI 流水线、Compose 模板解耦、云交付脚本族与合成测试验证（退出码全 0）。
2. **第二阶段（审查）**：移交独立 Reviewer 进行代码审计。
3. **第三阶段（外部授权）**：等待用户在 QQ 确认私有包名并提供具备 `read:packages` 权限的 Classic PAT。
4. **第四阶段（验收与上线预备）**：拉取私有镜像并拉起隔离容器，移交 Critic 黑盒验收；验收通过后请示用户执行生产备份与上线。

## Open Questions

- 用户是否确认采用 `ghcr.io/einck0/csp-runtime-private` 作为正式私有镜像包名？（当前在 QQ 队列等待用户确认）
- 机器拉取私有镜像所需的 Classic PAT（含 `read:packages`）的配置时机与发放方式？（当前无可用凭据，保持门禁阻断）

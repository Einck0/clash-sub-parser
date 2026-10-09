## 1. 官方云 CI 工作流重构与分支保护门禁

- [x] 1.1 在 `.github/workflows/ci.yml` 中实现单一全量 checks 作业，包含 docs-only 变更过滤与 GitHub concurrency 自动取消过时任务机制，严格按顺序集成前端 Node 22（`npm ci --include=dev`、`npm run type-check`、`npm test`、`npm run build`、资源同步）与后端 Go 1.27.1（`go mod verify`、vet、锁定测试工具与诊断测试、静态编译），编译输出 `csp-linux-amd64` 并通过 `actions/upload-artifact@v4` 上传制品。通过本地 YAML 语法检查与配置走查验证
- [x] 1.2 在 CI 工作流中配置严格安全权限隔离：PR 触发事件权限严格锁定为 `contents: read`，严禁包写入权限；后续发布或构建作业仅对受保护分支的受信明确事件开放，且必须通过 `needs: [test-and-build]` 建立硬依赖。通过检查工作流 `permissions` 与 `needs` 声明验证

## 2. 镜像构建解耦与二进制产物复用

- [x] 2.1 编写轻量独立运行时镜像描述文件 `Dockerfile.runtime`，仅基于 Alpine 3.20 拷贝 checks 作业生成的纯静态二进制制品及必要静态资产，维持非 root 用户（UID 10001:appuser）与零 CGO 纯静态运行契约，打上不可变 SHA 标签与 OCI revision 元数据标签。通过 Dockerfile 语法与层级走查验证不包含任何重编译指令
- [x] 2.2 重构容器构建发布作业，直接通过 `actions/download-artifact@v4` 获取 CI checks 生成的 `csp-linux-amd64` 纯静态二进制并注入 `Dockerfile.runtime`，彻底消除容器构建阶段对 Go 二进制与前端代码的二次重复编译。通过工作流步骤断言验证容器构建阶段无 `go build` 或源码编译命令

## 3. GHCR 私有包发布与 Fail-Closed 门禁

- [x] 3.1 确立提议私有包名 `ghcr.io/einck0/csp-runtime-private`，并在用户 QQ 确认前保持发布门禁阻断，彻底废除向历史公开包 `ghcr.io/einck0/clash-sub-parser` 的推送。通过配置走查验证新包名规范及门禁条件
- [x] 3.2 在发布作业中移除所有 `continue-on-error: true` 宽松标记，增加 Docker push 成功断言、确切 sha256 镜像 digest 提取、私有包未授权匿名拉取拒绝（通过完整 401 challenge 探测）及 Classic PAT（`read:packages`）鉴权检查，确保发布失败时工作流立即退出且 exit code 非 0。通过发布脚本逻辑断言 Fail-Closed 行为

## 4. 生产 Compose 解耦与防现场编译保障

- [x] 4.1 提供生产专用 Compose 模板 `docker-compose.prod.yml`，采用 `image-only` 模式并显式声明不可变 digest（`${CSP_IMAGE:?required}`），坚决移除 `build` 语法块，配套启动命令强制带上 `--no-build` 选项。通过检查 Compose 文件确认无 build 字段且包含 digest 锁定
- [x] 4.2 将本地开发源码编译上下文完整隔离至显式 dev 配置文件 `docker-compose.dev.yml`，并提供隔离运行模板 `docker-compose.isolated.yml`，确保开发构建与生产运行配置彻底解耦，杜绝生产环境自动回退至现场编译。通过走查对比生产与开发 Compose 配置差异验证

## 5. 生产宿主确权、隔离 Runbook 与回滚契约

- [x] 5.1 明确生产容器真实宿主路径为 `/home/service/clash-sub-parser`，生产数据卷权限实证为 750 (UID 10001:10001)，建立权限前置门禁，在未获用户显式 QQ 确认及生产写入/重启权限前严禁修改现行生产目录或重启容器。通过权限边界审查验证
- [x] 5.2 编写隔离预备与黑盒验收 Runbook（`docs/operations/csp-cloud-delivery.md`），规范拉取指定 digest、在独立 project、独立回环端口与全新 synthetic DB 卷中拉起隔离实例，执行健康检查与 Critic 只读验收，并明确升级前 SQLite 数据库热备与回滚步骤。通过文档内容走查与回滚流程完整性验证

## 6. 真实云端流水线执行、鉴权拉取、隔离 Critic 黑盒验收与生产受控上线

- [ ] 6.1 在受保护分支或受信发布触发（`release/**` / `v*` tag / `workflow_dispatch`）上运行真实 GitHub Actions 流水线，完成云端全量测试、二进制构建与 GHCR 私有镜像首次发布校验（要求 `ENABLE_CSP_GHCR_PUBLISH=true` 且具备 `OWNER_PAT`）。（外部阻断待决：需用户 QQ 授权包名与配置凭据）
- [ ] 6.2 经由操作员配置宿主机 Classic PAT（`read:packages`），使用 `scripts/cloud-delivery/preflight-pull.sh` 执行真实私有镜像拉取与 RepoDigests 确权校验。（外部阻断待决：需真实发布产物与宿主机 PAT）
- [ ] 6.3 运行 `scripts/cloud-delivery/run-isolated.sh` 在本地 18081 回环端口拉起真实隔离预览实例，移交独立 Critic 进行全量多视口几何与黑盒功能验收并出具 `CRITIC: PASSED`。（前置条件待决：需完成 6.2 真实镜像拉取）
- [ ] 6.4 将特性分支经由标准流程合并入 `dev` / `main` 受保护分支。（前置条件待决：需通过独立审查与 Critic 验收）
- [ ] 6.5 经由用户显式请示授权，在维护宿主机执行生产 SQLite 数据库可验证热备，并使用 `docker-compose.prod.yml` 执行受控生产容器替换与上线健康检查。（前置条件待决：需用户正式上线批准）

# CSP 日常云交付、拉取式 Compose 与隔离验证操作手册 (Operations Manual)

## 1. 概述与交付架构总览

Clash Sub Parser (CSP) 正式迈入日常云端自动化交付阶段。本手册规范日常开发、云端统一 CI 构建、GHCR 私有包分发、生产环境拉取式 Compose、隔离预览验证以及数据库安全回滚的全流程操作。

### 1.1 核心架构交付链 (The Same-SHA Digest Chain)
云端交付建立在**同一 Commit SHA 与不可变 sha256 镜像摘要**的严密链条之上，杜绝现场二次编译与版本漂移：

```
[Git Commit SHA]
       │
       ▼
[GitHub Actions CI: test-and-build] (Node 22 + Go 1.27.1, CGO_ENABLED=0 单次编译)
       │  生成纯静态二进制 csp-linux-amd64 与 sha256 校验清单并上传
       ▼
[GitHub Actions CI: publish-runtime] (仅拷贝制品至 Alpine 3.20, Dockerfile.runtime, UID 10001)
       │  复用下载制品，硬依赖 needs: [test-and-build]，发布至私有 GHCR
       ▼
[GHCR Private: ghcr.io/einck0/csp-runtime-private@sha256:<digest>]
       │
       ▼
[Preflight Pull: scripts/cloud-delivery/preflight-pull.sh] (真实 API private 验证 + 架构 + 必选 revision)
       │
       ▼
[Isolated Preview: scripts/cloud-delivery/run-isolated.sh] (独立 project, 18081 端口, 所有权 run-marker)
       │
       ▼
[Reviewer 审计 + Critic 黑盒验收] (多视口截图与几何量测)
       │
       ▼ (需人工/用户显式授权)
[生产受控上线: docker-compose.prod.yml] (image-only, 显式 -f 引用, 严格 --no-build)
```

---

## 2. 日常开发 vs 生产编排独立双轨模式

仓库提供严格分离且各自完备的独立配置文件，**坚决不将开发配置设计为与生产配置隐式自动合并的 override 补丁**，杜绝现场配置污染与误触发编译：

### 2.1 日常本地开发与源码调试 (Local Dev Mode - 独立完整模式)
- **配置文件**：`docker-compose.dev.yml`
- **构建机制**：独立的自包含开发编排，显式声明 `build:` 语法块（`context: .`, `dockerfile: Dockerfile`），使用本地镜像现场编译 Node 前端与 Go 后端源码。
- **启动命令**：
  ```bash
  docker compose -f docker-compose.dev.yml up -d --build
  ```
- **数据卷与隔离**：使用独立的 `csp-v1-dev-data` 命名卷，默认端口映射 `127.0.0.1:18080:18080`，绝不污染生产或预览数据。

### 2.2 生产环境拉取式交付模板 (Production Pull-Only Template - 独立完整模式)
- **交付模板文件**：`docker-compose.prod.yml`（仓库中可版本化追踪的正式生产模板；因 `.gitignore` 保护真实 secrets 忽略 `docker-compose.yml`，生产模板固定追踪为 `docker-compose.prod.yml`，严禁修改 `.gitignore` 破坏凭据保护）。
- **构建机制**：`image-only` 纯拉取模式，坚决**不包含任何 `build` 语法块**，镜像字段强制绑定不可变摘要环境变量：
  ```yaml
  services:
    app:
      image: ${CSP_IMAGE:?required}
  ```
- **启动与应用规范**：获生产部署授权后，使用显式 `-f docker-compose.prod.yml` 或经操作员确认后安装，强制必须携带 `--no-build` 选项，杜绝任何自动 fallback 或缓存编译：
  ```bash
  CSP_IMAGE="ghcr.io/einck0/csp-runtime-private@sha256:<64位哈希>" \
  docker compose -f docker-compose.prod.yml up -d --no-build
  ```
- **生产所有权与零覆盖铁律**：生产环境真实 Compose 文件位于 `/home/service/clash-sub-parser/docker-compose.yml`。**在未获用户显式 QQ 确认及生产上线授权前，绝对保持生产 owner 目录原样只读，严禁擅自覆盖或重启生产容器**！

---

## 3. 正式分支策略与云端 CI/CD 真实接口

### 3.1 分支策略与现状声明
- **当前施工分支**：`task/csp-cloud-build-integration` (HEAD `5e3b8fb`)。
- **上游主干分支**：`origin/dev` (基于 `1c5a604`)。
- **接入状态明确声明**：
  - **当前尚未合并入 `dev` 分支**，严禁在任何报告中假称 `dev` 已完成正式接入或已上线生产。
  - 正式合并前必须满足硬门禁：全量云端 CI checks 绿灯、独立只读审查（`reviewer`）判决 PASS、Critic 黑盒验收通过，并在正式上线前获得用户的集中确认授权。

### 3.2 官方 CI 流水线真实作业与触发契约 (`.github/workflows/ci.yml`)
- **触发与过滤规范**：
  - `pull_request` (针对 `main`, `dev`, `task/csp-cloud-build-integration` 分支)：权限严格锁定为 `contents: read`，绝不触发容器发布作业；
  - 常规 `push` (针对 `main`, `dev`, `task/csp-cloud-build-integration`)：仅触发 `test-and-build` 检查，不触发容器发布；
  - 容器发布触发条件：仅对受信任的受保护事件开放：
    1. `release/**` 分支的 push 事件；
    2. `v*` 语义化版本 tag 的 push 事件；
    3. `workflow_dispatch` 手动触发且显式输入 `publish_release == 'true'`。
  - **Tag 事件路径过滤语义说明**：根据 GitHub Actions 官方规范，针对 git tag 推送事件，`paths` 与 `paths-ignore` 过滤规则天然不生效。这是 GitHub 原生设计语义，因为打 tag 本身即代表显式明确的发布意图，因此 tag 事件完整运行 checks 与发布作业完全符合预期，无需人为在单个工作流中强行拆分重复的 `on.push` 块。
  - 纯文档提交过滤：针对普通分支 push 与 PR，修改 `docs/**`、`openspec/**`、`.specify/**` 及 `*.md` 自动跳过重型作业。

- **作业 1：`test-and-build` (单一全量 Checks 作业)**：
  - Node 22 前端校验：`npm ci --include=dev`、`npm run type-check`、`npm test`、`npm run build`，产物同步至 `internal/webassets/dist`；
  - Go 1.27.1 后端校验：`go mod verify`、vet、锁定测试二进制（sing-box/mihomo）与合成 legacy archive 诊断测试；
  - 纯静态编译：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build`，生成 `bin/csp-linux-amd64` 纯静态二进制（无 CGO、无动态链接解释器）；
  - 制品上传：计算 `sha256sum` 并通过 `actions/upload-artifact@v4` 上传二进制与校验清单，供后续发布复用。

- **作业 2：`publish-runtime` (零重复编译运行时容器发布)**：
  - 硬依赖：`needs: [test-and-build]`；
  - 制品下载与校验：直接通过 `actions/download-artifact@v4` 获取上游二进制制品，核验 sha256 校验和与静态 ELF，彻底消除容器阶段二次编译；
  - 镜像描述：基于 `Dockerfile.runtime`，以 Alpine 3.20 为基底，采用非 root 用户 `appuser` (UID `10001`, GID `10001`) 运行；元数据 `BUILD_DATE` 采用规范的 ISO 8601 (RFC 3339) UTC 时间戳；
  - 发布凭据与权限：使用作业自身 `packages: write` 的 `GITHUB_TOKEN` 推送镜像，严禁将外部写凭据硬编码或放入代码库。

- **仓库变量与发布前置门禁 (`check_ghcr_private_gate.py`)**：
  - 必须配置仓库变量 `ENABLE_CSP_GHCR_PUBLISH == 'true'`；
  - **`OWNER_PAT` 硬门禁**：必须具备 GitHub Classic PAT 的 `read:packages` 最小作用域（由仓库 Secret `OWNER_PAT` 提供）。用于调用 GitHub Package API 验证包的私有可见性（`visibility == 'private'`）或全分页审计证明包不存在。**缺少 `OWNER_PAT` 时发布作业严格 FAIL-CLOSED BLOCKED，绝不降级，绝不静默忽略**；
  - **完整匿名 Challenge 探测**：探测脚本在匿名请求返回 401 challenge 后，必须使用提取的 realm/service/scope 获取 token 并尝试拉取 manifest；只有当确认无法匿名获取 manifest 时才算通过，若能获取（HTTP 200）则立判私有失效并阻断发布。

---

## 4. GHCR 私有镜像命名空间与安全发布门禁

### 4.1 私有包名提议与历史公开包处理
- **提议私有包名**：`ghcr.io/einck0/csp-runtime-private`。
- **当前审批状态**：已在 QQ 队列提交等待用户确认。
- **历史公开包政策**：历史包 `ghcr.io/einck0/clash-sub-parser` 此前已发布公开 manifest（manifest 200），GitHub Packages 机制不支持将已有公开历史的容器包直接设为私有。为保护既有下游不发生突发性破损，**严禁擅自删除历史公开包，但永久停止向该公开包推送新镜像**。

### 4.2 镜像拉取与凭据安全管理 (Classic PAT)
- **权限最小化原则**：机器本地拉取私有镜像必须且仅需具备 GitHub Classic PAT 的 `read:packages` 作用域，严禁索取或配置拥有 `repo` 或管理员权限的高危 Token。
- **凭据输入安全规程**：
  - 严禁在命令行中使用明文 `-p <token>` 参数（防止通过 `ps` 进程列表或 bash history 泄漏）；
  - 必须使用安全终端静默输入或安全管道：
    ```bash
    scripts/cloud-delivery/preflight-pull.sh --configure-pat
    ```
  - 凭据文件 `~/.docker/config.json` 权限严格限制为 `0600`。
  - **安全警示**：Docker CLI 凭据默认以 base64 编码保存在本地配置文件中，属于非系统加密密钥环存储，宿主机操作员需保护该配置文件。
  - **沟通纪律**：智能体在交互中**严禁向用户索取明文 Token，严禁擅自自动创建外部凭据**。当前状态明确记录为“QQ 队列等待用户批准包名与提供拉取权限”，不能伪称已获。

### 4.3 Fail-Closed 镜像拉取前置门禁 (`preflight-pull.sh`)
运维与脚本必须使用 `scripts/cloud-delivery/preflight-pull.sh` 进行拉取与校验：
- **必选 Revision SHA**：必须携带 `--expected-revision <sha>`（大小写不敏感 40 或 64 位十六进制），不得省略；
- **不可变摘要锁定**：严格校验 `@sha256:<64hex>` 格式，拒绝裸标签（如 `:latest`）；
- **拒绝公开与未授权包名**：包名必须与目标私有包严格一致，严禁使用历史公开包名，严禁凭包名含 `private` 擅自宣称通过；
- **可执行官方私有验证路径**：脚本直接调用官方 `.github/scripts/check_ghcr_private_gate.py`，通过环境变量 `OWNER_PAT`（需具备 `read:packages` 权限，不打印不 echo）执行真实 GitHub API visibility 验证及完整匿名 challenge 探测。无凭据时明确返回退出码 2 (BLOCKED)；验证未通过时返回退出码 1；仅在真实双重证据满足后才允许 pull；拒绝伪造 JSON 文件口头证明；
- **拉取后同一性校验**：通过 `docker image inspect` 校验 `RepoDigests` 包含期望哈希、平台架构严格为 `linux/amd64`，且 OCI revision 标签与源码 SHA 严格一致。

---

## 5. 隔离预览与 Critic 黑盒验收 Runbook

在生产环境变更前，必须拉起完全物理隔离的容器环境进行黑盒功能与视觉几何验收：

### 5.1 隔离环境技术规范
- **Docker Compose 项目名**：`csp-isolated`（通过 `docker-compose.isolated.yml` 与 `-p <project>` 管理）；
- **网络与端口**：监听本地未占用回环端口 `127.0.0.1:18081`（主端口）与 `127.0.0.1:17001`（次端口），启动前必须执行 `ss -tln` bind check；
- **持久化数据隔离与所有权**：使用全新独立的命名数据卷（带动态 run-marker，如 `csp-isolated-data-<marker>`），**严禁挂载生产路径 `/var/lib/docker/volumes/csp-v1-data/_data` 或任何生产 volume**。全新隔离卷由容器启动时自动建立所属 UID；
- **资源所有权标签**：容器与数据卷打上 `com.csp.isolated.owner=cloud-delivery` 与 `com.csp.isolated.run-marker=<marker>`，并在 `/tmp/csp-preview/run-state.json` 记录状态；
- **配置与安全边界**：
  - `CSP_ADMIN_TOKEN`：由脚本动态生成的合成测试 Token，写入 `/tmp/csp-preview/auth-secrets.json`（权限 `0600`）；
  - `CSP_FETCH_PROXY`：留空或设置为无效地址，杜绝隔离测试向外部真实代理节点发起请求；
  - 生产数据写入量：严格为 0。

### 5.2 启动与验证操作步骤
```bash
# 1. 检查端口与环境就绪状态 (Check-only 模式)
scripts/cloud-delivery/run-isolated.sh \
  --image "ghcr.io/einck0/csp-runtime-private@sha256:<digest>" \
  --check-only

# 2. 正式启动隔离实例 (需镜像已在本地缓存)
scripts/cloud-delivery/run-isolated.sh \
  --image "ghcr.io/einck0/csp-runtime-private@sha256:<digest>"

# 3. 验证健康检查
curl -fsS http://127.0.0.1:18081/healthz
# 预期返回: {"data":{"status":"ok"}}

# 4. 产出实例描述工件
cat /tmp/csp-preview/instance.json
```

### 5.3 验收后受控清理规程 (`cleanup-isolated.sh`)
严禁使用 `docker system prune` 或全局清理命令！脚本具备所有权 run-marker 校验与事前/事后生产守护断言：
```bash
scripts/cloud-delivery/cleanup-isolated.sh --project csp-isolated --run-marker <marker>
```
- **所有权防误删检查**：清理前必须提供或从 `run-state.json` 识别 `project` 与 `run-marker`；检查项目下容器标签，若不匹配当前运行拥有者标签，立即 FATAL 拒绝下线卷，严禁误删宿主机上其他并行工作区或他人容器；
- **生产守护断言 (Production ID Guards)**：清理前记录当前生产容器 ID 与 `csp-v1-data` 卷名称；执行仅针对当前 project 的 `down -v` 后，复核生产容器 ID 与卷名未发生任何变化，若有任何不匹配立即 FATAL 阻断；
- **合成测试纪律**：自动化测试套件中仅通过 `--dry-run` 模拟清理，不实际对宿主机下发 `docker compose down -v`，绝不删除任何现行 volume。

---

## 6. 生产环境确权事实与零变更铁律

### 6.1 实机生产环境探查事实
经实机 Docker labels 与系统只读探查确认：
- **生产容器**：`clash-sub-parser` (Container ID `130b96625188`，状态 healthy)；
- **生产 Compose 工作目录**：`com.docker.compose.project.working_dir = /home/service/clash-sub-parser`；
- **生产 Compose 配置文件**：`com.docker.compose.project.config_files = /home/service/clash-sub-parser/docker-compose.yml`；
- **生产端口绑定**：`127.0.0.1:17000:18080` 与 `127.0.0.1:18080:18080`；
- **生产持久化卷**：`csp-v1-data` 挂载至 `/data`；
- **生产卷权限实证 (Read-Only Stat)**：
  通过实机只读检查：`stat -c 'Access: %a, Uid: %u, Gid: %g' /var/lib/docker/volumes/csp-v1-data/_data` 输出严格为 `Access: 750, Uid: 10001, Gid: 10001`。生产数据卷已具备非 root `appuser` (10001:10001) 读写权限。**铁律：仅做只读 stat 确权，严禁在生产工作树或数据卷上执行 chown 或 chmod 操作**！

### 6.2 零生产变更治理铁律
- **本轮施工绝对只读**：严禁向 `/home/service/clash-sub-parser` 写入任何文件，严禁执行 `docker restart` 或 `docker compose up` 重启生产容器；
- **正式上线前集中请示**：仅在代码全绿、独立审查通过、拿到私有镜像后拉起隔离实例且 Critic 黑盒验收通过后，向用户呈报最终镜像 digest 并请求明确上线授权。

---

## 7. 生产 SQLite 数据库一致性热备与回滚预案

本预案严格引用既有生产运维手册（`docs/operations.md` 第 4 节与 `docs/controlled_deployment_runbook.md` 第 2 节）：

> **重要声明**：本轮任务不涉及生产部署，**因此绝不触碰生产数据库，不执行生产备份写入**。以下流程为后续获授权上线时的标准执行规范。

### 7.1 生产 SQLite 在线热备份 (Online Hot Backup)
因系统启用了 WAL 模式，直接拷贝文件可能导致损坏。上线前必须在维护宿主机执行原子备份：
```bash
BACKUP_FILE="/data/backups/csp-v1-backup-$(date +%Y%m%d_%H%M%S).db"

# 通过容器内 sqlite3 执行原子一致性快照备份
docker exec clash-sub-parser sqlite3 /data/csp-v1.db ".backup '${BACKUP_FILE}'"

# 四重完整性硬门禁校验 (任一失败必须立即终止发布)
test -s "${BACKUP_FILE}" || { echo "FATAL: backup file is empty"; exit 1; }
docker exec clash-sub-parser sqlite3 "${BACKUP_FILE}" "PRAGMA schema_version;"
INTEGRITY=$(docker exec clash-sub-parser sqlite3 "${BACKUP_FILE}" "PRAGMA integrity_check;")
[ "${INTEGRITY}" = "ok" ] || { echo "FATAL: integrity check failed"; exit 1; }
FK_ERR=$(docker exec clash-sub-parser sqlite3 "${BACKUP_FILE}" "PRAGMA foreign_key_check;")
[ -z "${FK_ERR}" ] || { echo "FATAL: foreign key check failed"; exit 1; }
```

### 7.2 双轨回滚应急预案 (Rollback Runbook)

#### 方案 A：应用逻辑纯镜像回滚（无数据损毁，默认路径）
若新版本仅存在界面展示、HTTP 路由或业务逻辑缺陷，且数据库表结构未损坏：
```bash
# 1. 将生产配置中的 CSP_IMAGE 指向上一个稳定版本的不可变 digest
# 2. 执行受控服务重启 (保持数据卷与端口映射不变)
CSP_IMAGE="<PREVIOUS_KNOWN_GOOD_DIGEST>" \
docker compose -f /home/service/clash-sub-parser/docker-compose.yml up -d --no-build app

# 3. 核验健康检查
curl -fsS http://127.0.0.1:18080/healthz
```

#### 方案 B：数据库损坏时数据原子恢复（灾难恢复路径）
若新版本触发异常 migration 导致数据结构损坏：
```bash
# 1. 停止生产服务
docker compose -f /home/service/clash-sub-parser/docker-compose.yml stop app

# 2. 隔离故障现场
mv /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db \
   /var/lib/docker/volumes/csp-v1-data/_data/csp-v1-corrupted-$(date +%Y%m%d_%H%M%S).db
rm -f /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db-wal \
      /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db-shm

# 3. 恢复经过校验的备份快照并核准权限
cp "${BACKUP_FILE}" /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db
chown 10001:10001 /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db
chmod 0600 /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db

# 4. 以先前稳定镜像启动服务
CSP_IMAGE="<PREVIOUS_KNOWN_GOOD_DIGEST>" \
docker compose -f /home/service/clash-sub-parser/docker-compose.yml up -d --no-build app

# 5. 核验健康检查与数据就绪检查
curl -fsS http://127.0.0.1:18080/healthz
curl -fsS http://127.0.0.1:18080/readyz
```

---

## 8. 当前交付现状、外部受阻点与责任人说明

截至本施工阶段收口：
1. **已就绪工程资产**：
   - 官方单 workflow CI 流水线（`.github/workflows/ci.yml`），单次编译制品复用；
   - 运行时镜像描述 `Dockerfile.runtime`（非 root UID 10001，零 CGO，零二次编译）；
   - 可追踪生产级模板 `docker-compose.prod.yml`（`image-only`，不可变 digest 校验，禁止 build）；
   - 本地开发专用独立配置 `docker-compose.dev.yml`（显式 local build）；
   - 隔离运行配置 `docker-compose.isolated.yml`（独立项目、回环端口、独立测试卷、所有权标签）；
   - 镜像预检与拉取工具 `scripts/cloud-delivery/preflight-pull.sh`；
   - 隔离预览与清理工具 `scripts/cloud-delivery/run-isolated.sh` 及 `cleanup-isolated.sh`；
   - 完整的自动化合成测试套件。
2. **当前外部阻断项 (External Blockers)**：
   - **私有包名授权**：提议的 `ghcr.io/einck0/csp-runtime-private` 仍在等待用户在 QQ 明确批准；
   - **拉取凭据**：具备 `read:packages` 权限的 Classic PAT 凭据尚未由用户配置至宿主机与仓库 Secret `OWNER_PAT`；
   - **分支合并与部署授权**：本特性分支尚未合并入 `dev`，生产环境尚未获得替换与重启授权；
   - **隔离验收状态**：当前因无可用私有镜像与权限，隔离实例处于 **NOT READY** 阻断状态，本轮不执行 Critic 验收。
3. **责任人**：
   - 外部凭据与授权：Einck（用户）；
   - 隔离实例与 Critic 验收准备：`preview-preparer`（在获得私有镜像后拉起并移交）；
   - 协调与汇报：`master-orchestrator`。

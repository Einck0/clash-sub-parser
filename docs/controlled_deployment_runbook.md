# Clash Sub Parser (CSP) 受控部署预案与服务运行手册 (Controlled Deployment Runbook)

## 一、 服务架构与受控范围基线

- **服务名称**：`clash-sub-parser` (CSP Control Plane & Compiler Engine)
- **运行形态**：单个静态编译可执行文件，内嵌 Vue 3 前端静态资产 (`internal/webassets/dist`)，容器化部署。
- **持久化数据**：SQLite 数据库，标准路径 `/data/csp-v1.db`（挂载自 Docker volume `csp-v1-data`）。
- **外部暴露端口**：宿主机 `127.0.0.1:18080`（受控反向代理）。
- **受控绝对边界与铁律**：
  1. **禁止重启宿主机 Hermes / Gateway**：CSP 容器为独立自治服务，任何维护与部署操作严禁波及或重启宿主机外部服务；
  2. **写权限与审查权绝对分离**：施工阶段**严禁**直接触碰生产环境、严禁覆盖生产数据库；
  3. **受控上线时机**：只有在代码经独立只读审查（`reviewer`）判决 **PASS** 且由主脑明确核准后，方可进入部署上线与后续 Critic 验收流程。

---

## 二、 生产数据库可验证备份流程 (Verified Backup Procedure)

在任何发布或变更前，必须对生产 SQLite 数据库执行在线可验证备份：

```bash
# 1. 创建备份目录
mkdir -p /data/backups

# 2. 使用 SQLite 官方在线备份协议（VACUUM INTO 或 .backup），杜绝读写锁竞争
BACKUP_FILE="/data/backups/csp-v1-backup-$(date +%Y%m%d_%H%M%S).db"
sqlite3 /data/csp-v1.db ".backup '${BACKUP_FILE}'"

# 3. 强校验备份副本完整性与有效性
sqlite3 "${BACKUP_FILE}" "PRAGMA integrity_check;"
# 预期输出必须严格为: ok

# 4. 校验备份文件非空与元数据
test -s "${BACKUP_FILE}" && ls -lh "${BACKUP_FILE}"
```

---

## 三、 真实镜像构建与可回滚目标方案 (Rollback Plan)

### 1. 固化当前存量镜像作为回滚目标
```bash
# 获取当前线上正在运行的镜像 ID 并打上受管回滚标签（以实际 app 镜像名称 clash-sub-parser-app 为准）
CURRENT_IMAGE_ID=$(docker inspect --format='{{.Image}}' clash-sub-parser)
docker tag "${CURRENT_IMAGE_ID}" clash-sub-parser-app:rollback-pre-03f2646-807549162ceb
echo "Rollback target pinned to image: ${CURRENT_IMAGE_ID}"
# 导出当前镜像归档到私有受管备份目录留存
docker save "${CURRENT_IMAGE_ID}" | gzip > /data/backups/csp-image-rollback.tar.gz
```

### 2. 构建新版本生产镜像与受控发布
```bash
# 基于当前已审查冻结源码树构建新镜像
docker compose -f /home/service/clash-sub-parser/docker-compose.yml build app

# 严格执行目标服务受控替换：不 down、不删卷、不重新拉取其他服务
# 保持 csp-v1-data 命名卷、双 loopback 端口 (17000/18080)、代理及现有 DB verifier 不变
docker compose -f /home/service/clash-sub-parser/docker-compose.yml up -d --no-deps --no-build --force-recreate app
```

### 3. 双向回滚应急操作流程
若上线后健康检查失败（`/healthz` 非 200）、服务异常重启或实网验收出现严重故障：

#### 方案 A：常规镜像回滚（默认无损路径）
适用于仅应用逻辑、前端资产或二进制异常，且数据库未发生有损变更：
```bash
# 1. 停止异常服务
docker compose -f /home/service/clash-sub-parser/docker-compose.yml stop app

# 2. 切换回滚镜像并拉起（严格使用 clash-sub-parser-app 镜像名，不使用旧 clash-sub-parser:latest）
docker tag clash-sub-parser-app:rollback-pre-03f2646-807549162ceb clash-sub-parser-app:latest
docker compose -f /home/service/clash-sub-parser/docker-compose.yml up -d --no-deps --no-build --force-recreate app

# 3. 验证回滚后健康状态
curl -fsS http://127.0.0.1:18080/healthz || exit 1
curl -fsS http://127.0.0.1:18080/readyz || exit 1
```
*注意：正常镜像回滚绝不覆盖仍完好的数据库，防止丢失正常的业务写入！*

#### 方案 B：数据库有损写入恢复（仅在确有数据损坏时执行）
仅当更新后数据库遭遇非法 schema 破坏或严重损坏写入时才触发：
```bash
# 1. 停止异常服务并确认无其他活跃写者
docker compose -f /home/service/clash-sub-parser/docker-compose.yml stop app

# 2. 封存损坏数据库及 WAL/SHM 现场以供审计证据留存，严禁旧 WAL 重新挂载到快照
mv /data/csp-v1.db /data/csp-v1-corrupted-$(date +%Y%m%d_%H%M%S).db
rm -f /data/csp-v1.db-wal /data/csp-v1.db-shm

# 3. 原子恢复已验证的备份副本并核验权限
cp "${BACKUP_FILE}" /data/csp-v1.db
chmod 0600 /data/csp-v1.db

# 4. 以捕获的回滚镜像重新启动
docker tag clash-sub-parser-app:rollback-pre-03f2646-807549162ceb clash-sub-parser-app:latest
docker compose -f /home/service/clash-sub-parser/docker-compose.yml up -d --no-deps --no-build --force-recreate app
```

---

## 四、 隔离环境准备与交接规范 (Preview Preparation Specification)

> **运行态就绪确权**：独立只读审查（`reviewer`）已于会话 `2026-10-01T11-02-51-323Z_8e3f0d63-d6aa6d99-bf3d93cf-87d5.jsonl` 出具明确 **REVIEW: PASS**。由 `preview-preparer` 成功拉起真实隔离预览服务，监听于本地 `127.0.0.1:18081`。

### 1. 真实运行态交接清单 (Instance Handoff Manifest)
- **实际访问 URL**：`http://127.0.0.1:18081`
- **实例 ID (Instance ID)**：`csp-isolated-preview-20261001-18081`
- **运行守护进程 PID**：见 `/tmp/csp-preview/csp-preview.pid`（通过 `setsid` 独立会话启动，子机退出后持续存活）
- **责任人 (Owner)**：`preview-preparer`
- **健康探针响应 (/healthz)**：`HTTP 200` `{"data":{"status":"ok"}}`
- **就绪探针响应 (/readyz)**：`HTTP 200` `{"data":{"ready":true,"required_tables":14,"schema_version":13}}`
- **代码版本与代码树指纹 (Source Manifest)**：
  - Git Commit: `6a008f722080e7fd2e8040a4e54cb6539baa0782`
  - Dirty Diff SHA256: `d5700687b3f7c34d1b12dbab174e72e6ded1b73acc09690074b58bace4037951`
  - Status Fingerprint: `ba5ccedd4f9715ce6ec95e515edbe0468e99e6d15bd287c102cebd3ff690a93c`
  - Web Bundle: `internal/webassets/dist` 内嵌静态资产
- **数据存储隔离边界 (Data Boundary)**：
  - 独立沙箱路径：`/tmp/csp-preview/csp-sandbox.db`（SQLite WAL 模式）
  - 生产隔离铁律：完全与生产数据库 `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db` 隔离，生产数据写入为 0
- **权限与本地密文凭据文件 (0600 File Handoff)**：
  - 管理员认证与导出 Token 凭据文件：`/tmp/csp-preview/auth-secrets.json` (权限 `0600`)
  - 清理令牌凭据文件：`/tmp/csp-preview/cleanup-token.secret` (权限 `0600`，内含高熵随机 Secret)
  - 安全原则：终端长输出与终态战报严禁明文打印 Token / 密码
- **清理命令 (Stop Command)**：`kill $(cat /tmp/csp-preview/csp-preview.pid)`（本次不停止，持续供 Critic 黑盒验收）
- **可机读实例描述符文件**：`/tmp/csp-preview/instance.json`

### 2. 预置确定性安全 Fixture 数据
1. **多订阅同 Host 不同凭据节点**：
   - Host `relay.fastnet-example.com` 挂载 3 个不同节点：
     - `node-ss-tokyo` (SS, Port 8388, Cipher aes-128-gcm, Sub: `sub-primary`)
     - `node-ss-osaka` (SS, Port 8389, Cipher aes-256-gcm, Sub: `sub-secondary`)
     - `node-vmess-hk` (VMess, Port 443, TLS, Sub: `sub-primary` & `sub-secondary`)
2. **失败刷新保留 (Failed Refresh Preservation)**：
   - 订阅 `sub-failing` 记录了最近一次失败抓取（Outcome `failed`），其关联节点 `node-preserved-sg` 依然完整保活保留，验证 Reconcile 机制的保全性；
3. **真实诊断与观测区分 (Truthful Diagnostics)**：
   - 未连接上游真实网络凭据前，所有节点健康状态严格保持 `unknown`，杜绝伪造假 `healthy`；
   - 历史观测记录（`obs-hist-tokyo`, `obs-hist-hk`）带 `connection_revision: NULL`，清晰区分最新代际与历史遗留观测；
4. **有效策略与发布数据**：
   - 策略组配置：`PROXY` (select) -> `Auto-Select` (url-test) 与 `Fallback` (fallback)；
   - 发布端点：`/publish/v1/pub-mihomo-preview` (Mihomo YAML) 与 `/publish/v1/pub-singbox-preview` (Sing-box JSON)。

---

## 五、 Critic 用户流程与几何证据验收前提 (Critic Visual Criteria)

### 1. 准入前置条件
1. 独立代码审查机（`reviewer`）已出具明确的 `REVIEW: PASS`；
2. 隔离预览环境通过健康检查（`/healthz` 与 `/readyz` 均返回 200 OK）；
3. 数据库副本已就绪且预置真实有效节点数据。

### 2. Critic 必须覆盖的核心交互流程
- **Flow 1: 登录与全局状态**：访问首页，执行登录，顶部状态徽章正确展示已认证模式；
- **Flow 2: 订阅管理与刷新**：查看已同步订阅，触发刷新，观察非破坏合并后节点列表无闪退、无空白；
- **Flow 3: 节点明文详情与抽屉交互**：点击节点打开侧边抽屉，检查明文连接参数展示，保存修改无溢出遮挡；
- **Flow 4: 策略组与分流规则**：查看分流规则，拖拽/配置规则边无阻断；
- **Flow 5: 发布与订阅导出**：切换 Mihomo / Sing-box Pills 目标，预览配置生成，复制真实订阅链接。

### 3. 多视口截图与几何证据要求
Critic 必须通过无头浏览器采集如下视口尺寸实机渲染截图并量测几何尺寸：
- **移动端视口**：`375x667` (iPhone SE) 与 `390x844` (iPhone 14)
  - 门禁要求：底部导航栏无溢出截断，侧边抽屉宽度全屏自适应，表格支持水平平滑滑动无视口破损；
- **桌面端视口**：`1280x800` 与 `1440x900`
  - 门禁要求：双列流式栅格弹性展开，弹窗居中无偏移，文字标签无省略号误截断。

---

## 六、 生产计划准备与资源/回滚预检 (Production Planning & Preflight Verification)

### 1. 生产环境服务定位与当前运行态
- **生产容器**：`clash-sub-parser` (Container ID `d8780938a0d6`，状态 `healthy`)；
- **生产镜像**：`clash-sub-parser-app:latest` (Image ID `807549162ceb`，当前稳定运行镜像，杜绝使用旧 `0ea88a8d0ee8`)；
- **生产数据卷**：Docker Volume `csp-v1-data`（宿主机物理路径 `/var/lib/docker/volumes/csp-v1-data/_data/`）；
- **生产数据库文件**：`/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db` (5.5 MB)；
- **生产外部端口**：`127.0.0.1:18080`（及 17000），严禁抢占或写入。

### 2. 宿主机可用资源与备份验证
- **磁盘可用空间**：根分区可用 13GB (占用率 81%)，满足在线 VACUUM 备份（约 6MB）与 Docker 镜像构建（约 150MB）资源预算；
- **现有可用备份验证**：
  - `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1-pre-v11-20260926_203423.db`（预置备份完好）
  - `/var/lib/docker/volumes/csp-v1-data/_data/backups/`（备份目录就绪）
- **回滚镜像固化**：当前运行态镜像已打上基线唯一标签（`clash-sub-parser-app:rollback-pre-03f2646-807549162ceb`），并在私有安全目录归档镜像 tar.gz 与校验 SHA-256，具备经严格演练的受控回滚路径，但严禁宣称无来源的“秒级原子回退”或“绝对零风险”假定。

### 3. 历史误伤失活节点恢复约束 (Node Preservation Invariant)
- **严格根据只读副本事实报告**：
  - 经对备份副本 `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1-pre-v11-20260926_203423.db` 只读校验，存量节点 963 个，失活节点为 960 个（演练计划识别出 960 个 heuristic candidates）；
  - 经对当前生产数据库 `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db` 只读校验，总节点数为 1005 个，失活节点为 983 个；杜绝机械硬写固定数值；
- **严格 Disabled 约束**：针对历史误失活节点，未来执行恢复操作时其属性必须严格保持 `active = 0`（即 disabled）；
- **严禁偷激活铁律**：严禁在未经过实机有效连通与主脑单独授权的情况下将历史节点批量激活为活跃分流节点（`active = 1`），防止脏数据冲击在线订阅。

### 5. 生产二进制与构建绑定确权事实 (Binary Identity Invariant)
- **源码与镜像可溯源绑定**：构建产生的生产镜像 `clash-sub-parser-app:v1-release-20261002-03f2646` (`sha256:7be48f7f84051aff1ff4feba34b70b95f1d8071f396af76dfb35b8914a55fafd`) 严格绑定至审查通过的 Commit `03f2646`；
- **运行二进制一致性声明**：经对镜像内可执行文件 `/app/csp` 执行 SHA-256 校验，其值为 `be2091a70e57d8f493d46ea4d14d4ba3fce1b4c35f0b6530e3908e161aa1e600`，与原稳定运行镜像 `sha256:807549162ceb` 内的二进制哈希完全一致。这表明本次镜像重建是用于将运行态确权绑定至经审查的已提交代码树（解决历史 dirty/无版本标签问题），而非引入了新的业务功能代码；本次整改的核心交付物 `cmd/csp-live-acceptance` 为独立验收工具，未打包进生产服务二进制中。
- **真实时间与状态基准事实**：
  - Fresh 热备数据库落盘与 Docker container 创建调用的文件 stat 时间差为 0.216 秒（`20:54:04.027` 至 `20:54:04.243`），此时间仅反映文件写入结束至 Docker CLI 发起调用的系统耗时，严禁表述为“写者停止窗口”或“演练时差”；
  - 最终替换前生成的生产快照（`csp-v1-fresh-before-up.db`，SHA-256 `042aa0cb8ad9fb6f1408aea8e27a81f0c3bb8eca1fe04b3f633095f73f4a932d`）已在完全隔离无挂载沙箱中完成独立复测恢复演练，确认 `integrity_check: ok`、`foreign_key_check: OK` 且全量 1005 节点、22 活跃节点、9 订阅、22 来源与身份 verifier 100% 保持；
  - 公网真实入口 `https://sub.einck.top`（由宿主机 Nginx 反代 `127.0.0.1:17000`）通过标准 TLSv1.3 校验，健康与鉴权端点均返回 200 OK。


# CSP 1.0 生产运维与安全治理手册 (Operations Runbook)

## 1. 架构定位与物理隔离契约

### 1.1 单一二进制与环境架构
- **运行时核心**：单一静态编译 Go 二进制 (`/app/csp`)，内嵌 Vue 3 + Tailwind CSS + DaisyUI 管理工作台静态资产 (`internal/webassets`)。
- **运行环境**：基于 Alpine 3.20 最小化运行时，使用非 root 专用系统账户 `appuser` (UID `10001`, GID `10001`) 运行，严禁 root 提权或特权容器。
- **网络与端口**：默认监听 `0.0.0.0:18080` (通过 `CSP_ADDR` 或 `CSP_BIND` + `CSP_PORT` 配置)，宿主机端口映射 `${CSP_PORT:-127.0.0.1:18080}:18080`。

### 1.2 独立数据卷与历史数据物理隔离铁律
- **单一数据权威**：新生产数据文件固定为 `/data/csp-v1.db`，由容器挂载的命名数据卷 `csp-v1-data` 独占。
- **物理完全隔离**：
  - 严禁在新 Compose 或运行时编排中挂载旧历史卷 `backend-data` 或 `clash-sub-parser_backend-data`。
  - 历史数据库文件 (`clash_sub_parser.db`) 仅作为只读离线归档介质 (Archive Source)，绝非新系统运行时依赖。
  - 运行时不读取任何旧数据表，不执行在线 schema 兼容适配，严禁任何形式的双写 (dual-write)。

### 1.3 数据库不变量与启动自检
- **路径**：`CSP_DB_PATH=/data/csp-v1.db`
- **Schema 迁移**：应用启动时由内嵌的顺序编号迁移 (`migrations/*.sql`) 自动创建或增量演进至最新版本。
- **运行时 PRAGMA 不变量**：
  - `PRAGMA foreign_keys = ON;` (外键级联与约束物理强制生效)
  - `PRAGMA journal_mode = WAL;` (Write-Ahead Logging 高并发读写)
  - `PRAGMA synchronous = NORMAL;` (兼顾持久性与写入吞吐)
  - `PRAGMA busy_timeout = 5000;` (5 秒锁等待超时)
- **就绪判定 (/readyz)**：
  - 检查 14 张核心业务表完整性 (`subscriptions`, `subscription_fetches`, `nodes`, `node_sources`, `probe_runs`, `probe_observations`, `node_groups`, `group_edges`, `admission_rules`, `policy_rules`, `configuration_revisions`, `publications`, `settings`, `audit_events`)。
  - 检查 `schema_migrations` 当前版本号。
  - 执行 `PRAGMA foreign_key_check;` 确认零外键破坏。

---

## 2. 容器编排与环境配置参考

### 2.1 环境变量配置清单
| 变量名 | 默认值 | 作用说明 |
|---|---|---|
| `CSP_ADDR` | `0.0.0.0:18080` | HTTP 服务监听绑定地址与端口 |
| `CSP_DB_PATH` | `/data/csp-v1.db` | SQLite 数据库文件绝对路径 |
| `CSP_ADMIN_TOKEN` | *(空)* | 管理控制面 Bearer / Cookie 认证令牌，留空则跳过或使用会话认证 |
| `TZ` | `Asia/Shanghai` | 容器时区 |

### 2.2 Docker Compose 编排规范
```yaml
services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        NPM_CONFIG_REGISTRY: https://registry.npmmirror.com
        GOPROXY: https://goproxy.cn,direct
    container_name: ${CSP_CONTAINER_NAME:-clash-sub-parser}
    environment:
      CSP_ADDR: ${CSP_ADDR:-0.0.0.0:18080}
      CSP_DB_PATH: ${CSP_DB_PATH:-/data/csp-v1.db}
      CSP_ADMIN_TOKEN: ${CSP_ADMIN_TOKEN:-}
      TZ: ${TZ:-Asia/Shanghai}
    ports:
      - "${CSP_PORT:-127.0.0.1:18080}:18080"
    volumes:
      - csp-v1-data:/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:18080/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

volumes:
  csp-v1-data:
    name: ${CSP_VOLUME_NAME:-csp-v1-data}
```

---

## 3. 历史数据离线迁移规范 (Offline Import)

离线导入必须在服务启动前或维护窗口期显式执行，严禁向 Web 服务暴露自动化导入路由。

### 步骤 1：只读源库体检 (Legacy Inspect)
```bash
# 只读分析历史库，输出指纹与字段分类报告（绝对不修改源文件，不输出敏感明文）
csp legacy-inspect --source /backup/legacy_clash_sub_parser.db --format json --output /tmp/legacy_report.json
```

### 步骤 2：空跑白名单导入预检 (Dry-Run Import)
```bash
# 模拟执行白名单转换与脱敏，输出隔离区报告 (quarantine report)
csp legacy-import \
  --source /backup/legacy_clash_sub_parser.db \
  --target /data/csp-v1.db \
  --dry-run \
  --format json \
  --report /data/quarantine_report.json
```
**安全复核要点**：
- 审查 `/data/quarantine_report.json`，确保隔离项仅记录元数据指纹与类型摘要，无密码、Token、私钥明文泄漏。
- 验证源库文件 SHA-256 校验和未发生改变。

### 步骤 3：正式原子入库 (Atomic Import)
```bash
# 写入带 draft 标记的 configuration revision，未激活前对调度器和出口不可见
csp legacy-import \
  --source /backup/legacy_clash_sub_parser.db \
  --target /data/csp-v1.db \
  --format text \
  --report /data/import_audit.txt
```

### 步骤 4：管理控制台审查与显式激活
- 管理员登录管理工作台或调用 API：`POST /api/v1/revisions/{revision_id}/review`
- 确认内容无误后，显式原子激活：`POST /api/v1/revisions/{revision_id}/activate`

---

## 4. 数据库一致性备份与恢复 Runbook

### 4.1 生产热备份规程 (Online Hot Backup)
因系统启用了 WAL 模式，直接 `cp` 正在写入的 SQLite 数据库可能产生损坏副本。必须采用 SQLite VACUUM INTO 或 sqlite3 官方备份指令保证事务快照一致性。

#### 执行热备份
```bash
BACKUP_FILE="/data/backups/csp-v1-backup-$(date +%Y%m%d%H%M%S).db"
mkdir -p /data/backups

# 方式一：容器内使用 sqlite3 CLI 进行一致性锁定备份
sqlite3 /data/csp-v1.db ".backup '${BACKUP_FILE}'"

# 方式二：通过宿主机 docker exec 执行备份
docker exec clash-sub-parser sqlite3 /data/csp-v1.db ".backup '${BACKUP_FILE}'"
```

#### 备份可用性与完整性校验（硬门禁）
备份完成后，**必须无条件执行以下四重机器校验**，任何一项报错即判定备份无效：
```bash
# 1. 验证文件非空
test -s "${BACKUP_FILE}" || { echo "FATAL: backup file is empty"; exit 1; }

# 2. 验证 Schema 与可打开性
sqlite3 "${BACKUP_FILE}" "PRAGMA schema_version;"

# 3. 验证数据库底层 B-Tree 完整性
INTEGRITY=$(sqlite3 "${BACKUP_FILE}" "PRAGMA integrity_check;")
if [ "${INTEGRITY}" != "ok" ]; then
  echo "FATAL: backup integrity check failed: ${INTEGRITY}"
  exit 1
fi

# 4. 验证外键约束一致性（输出为空代表 0 违规）
FK_ERRORS=$(sqlite3 "${BACKUP_FILE}" "PRAGMA foreign_key_check;")
if [ -n "${FK_ERRORS}" ]; then
  echo "FATAL: backup foreign key violations detected: ${FK_ERRORS}"
  exit 1
fi
```

### 4.2 灾难恢复规程 (Restore Runbook)
从经过验证的备份副本进行恢复的原子操作流程：

```bash
# 步骤 1：停止生产容器服务
docker compose stop app

# 步骤 2：对当前损坏或待恢复的现场数据库进行归档隔离
mv /data/csp-v1.db "/data/csp-v1.db.corrupted.$(date +%Y%m%d%H%M%S)"
rm -f /data/csp-v1.db-wal /data/csp-v1.db-shm

# 步骤 3：将目标备份文件恢复至生产路径
cp "${BACKUP_FILE}" /data/csp-v1.db

# 步骤 4：矫正非 root 权限（确保容器内 appuser 具备读写权限）
chown 10001:10001 /data/csp-v1.db
chmod 640 /data/csp-v1.db

# 步骤 5：启动生产容器
docker compose start app

# 步骤 6：核验健康与就绪检查
curl -sSf http://127.0.0.1:18080/healthz || { echo "Healthz probe failed"; exit 1; }
curl -sSf http://127.0.0.1:18080/readyz || { echo "Readyz probe failed"; exit 1; }
```

---

## 5. 生产切流与受控回滚 Runbook

### 5.1 生产流量切换准入检查清单 (Cutover Gate)
在调整反向代理（Nginx / FRP）或将公网流量切入新容器前，**必须由人工（Einck）明确审批授权**，并验证以下全部条件：
1. [ ] 新 Compose 使用独立的 `csp-v1-data` volume，没有挂载任何旧历史卷。
2. [ ] 容器日志无 panic、无 migration 失败记录。
3. [ ] `GET /healthz` 返回 HTTP 200 `{"data":{"status":"ok"}}`。
4. [ ] `GET /readyz` 返回 HTTP 200，`ready: true` 且 `schema_version >= 1`，0 缺失表，0 外键违规。
5. [ ] 旧接口路由 (`/yaml`, `/script`, `/api/*`) 均严格返回 HTTP 410 Gone。
6. [ ] 至少一份正式 configuration revision 已审阅激活，或具备初始可用策略配置。

### 5.2 受控秒级回滚流程 (Rollback Runbook)
若切流后发现业务严重异常、下游客户端不兼容或关键指标下跌，立即执行秒级确定性回滚：

```bash
# 步骤 1：反向代理立即切回旧服务端口
# 修改 Nginx upstream 或端口映射，恢复至旧服务端口

# 步骤 2：停止 CSP 1.0 新容器
docker compose down

# 步骤 3：启动原本处于停止状态的旧版容器（挂载原未写入的历史卷 backend-data）
docker start clash-sub-parser-legacy

# 步骤 4：验证旧版健康端点与服务恢复
curl -sSf http://127.0.0.1:18080/health

# 步骤 5：事故现场排查
# 保持 csp-v1-data 数据卷完整，收集新容器日志与异常数据用于复盘，绝不在线打盲目补丁
```

---

## 6. 自动化冒烟测试 (Smoke Test)

项目根目录提供了全自动化的冒烟测试脚本 `scripts/smoke_test.sh`：
- 自动核查 `docker compose config` 语法合规性。
- 自动验证 Compose 与 Dockerfile 绝对不含旧数据卷。
- 自动验证镜像编译产出单一 Go 二进制与 non-root (appuser) 运行权限。
- 自动验证空卷冷启动后 `/healthz`、`/readyz`、`/yaml` (410) 契约。
- 自动执行 SQLite 在线热备份、`integrity_check`、`foreign_key_check` 与测试库恢复全生命周期验证。
```bash
./scripts/smoke_test.sh
```

# CSP 生产环境有界验收与因果恢复执行计划 (Live Acceptance & Recovery Plan)

## 一、 基线与运行态确权 (Production Baseline & Runtime Verification)

- **Git Commit 基线**: `HEAD = 95ce0a9369c6cdbc8cb1231962bed3e5f7169596`
- **容器与端口状态**:
  - 容器名: `clash-sub-parser` (`d8780938a0d6`)
  - 镜像: `clash-sub-parser-app:latest` (`807549162ceb`)，健康状态: `healthy` (failing_streak = 0)
  - 监听端口: `127.0.0.1:18080` 与 `127.0.0.1:17000`
- **只读备份与回滚归档验证**:
  - 数据库基线备份: `/home/service/backups/csp-release-20261001_205016/csp-v1-backup-20261001_205016.db`
    - SHA-256: `8c9a28db4fc7c3c3c1293d3d95a5702774b0ef4643dcba6bdb1cb388246778fa` (完整性校验: OK)
    - SQLite PRAGMA integrity_check: `ok`
  - 镜像回滚归档: `/var/lib/docker/volumes/csp-v1-data/_data/backups/csp-image-rollback-0ea88a8d0ee8.tar.gz`
    - SHA-256: `440b2d769359ed7d0a3fdb518cf79c5c6badb7991c02ea4176168f18d5cd972a`
    - Gzip 完整性校验 (`gzip -t`): `exit 0`
  - 当前生产库路径: `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db`
    - SQLite PRAGMA integrity_check: `ok`

---

## 二、 节点因果分类与证据链分析 (Causal Classification & Evidence Chain)

通过对生产数据库、`subscription_fetches`、`audit_events` 及 `nodes` 进行有界因果溯源核对，当前节点台账状态如下：

| 分类 | 数量 | 判定理由与证据链 | 处置策略 |
|---|---|---|---|
| **活跃节点 (Active)** | **22** | 归属于 2 个活跃订阅（`...2e49` 拥有 7 个，`...5805` 拥有 15 个），均具备有效的 `node_sources` 来源绑定与完整配置。 | 维持正常提供服务 |
| **启发式存疑节点 (Heuristic Hold)** | **960** | 来源于 2026-09-19T12:41:38Z 的 `legacy.import`。在 2026-09-29T00:52 的单次订阅刷新中因旧版全局孤儿清理逻辑被批量置为 `active = 0`。**但该 960 个节点缺乏细粒度审计日志证明其用户保留意图，且当前 `node_sources` 中来源绑定为 0**。 | **绝对禁止全量盲目复活**（Fail-Closed 保护），保持 `active = 0` |
| **歧义节点 (Ambiguous Hold)** | **23** | 动态抓取残留节点（10 个于 2026-09-29T10:48 抓取后下线，13 个于 2026-09-30T23:36 抓取后下线）。无故障误失活因果证明，无来源有效性。 | 保持 `active = 0`，不予复活 |
| **用户明确禁用 (User Disabled)** | **0** | `audit_events` 中未发现针对单个节点的明确禁用/删除记录。 | N/A |
| **因果充分可恢复白名单 (Recoverable Allowlist)** | **0** | **当前无任何节点具备“因果充分 + 来源有效唯一 + 非用户禁用”的完整法定证明。** | **恢复白名单计数严格为 0** |

> **关键工程结论**：尊重客观事实，不可凭空将旧 983/960 当成恢复依据；在缺乏因果历史与有效来源证据时，保持受保护的未知态（Protected Unknowns），坚决杜绝为了制造“全绿假象”而违规批量放活。

---

## 三、 热备与回滚操作手册 (Hot Backup & Rollback Runbook)

在独立审查通过且获授权执行任何写操作前，必须严格按以下步骤执行热备与回滚准备：

### 1. 执行前在线事务热备 (Online Hot Backup)
```bash
BACKUP_DIR="/home/service/backups/csp-hot-backup-$(date +%Y%m%d_%H%M%S)"
mkdir -p "${BACKUP_DIR}"
# 利用 sqlite3 在线热备 API 生成一致性快照，不锁表且不中断正在运行的服务
sqlite3 /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db ".backup '${BACKUP_DIR}/csp-v1-hot.db'"
sha256sum "${BACKUP_DIR}/csp-v1-hot.db" > "${BACKUP_DIR}/backup.sha256"
echo "Hot backup created at ${BACKUP_DIR}/csp-v1-hot.db"
```

### 2. 数据库回滚指令 (Database Rollback)
若生产恢复或刷新出现意外：
```bash
docker compose -f /home/service/clash-sub-parser/docker-compose.yml stop app
cp "${BACKUP_DIR}/csp-v1-hot.db" /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db
docker compose -f /home/service/clash-sub-parser/docker-compose.yml up -d app
curl -s http://127.0.0.1:18080/healthz
```

### 3. 镜像回滚指令 (Image Rollback)
若服务二进制或镜像发生异常：
```bash
docker compose -f /home/service/clash-sub-parser/docker-compose.yml stop app
docker tag clash-sub-parser-app:rollback-target clash-sub-parser-app:latest
docker compose -f /home/service/clash-sub-parser/docker-compose.yml up -d app
curl -s http://127.0.0.1:18080/healthz
```

---

## 四、 脱敏实网验收与执行指令 (Safely Redacted Execution Commands)

本阶段交付的工具完全脱敏，不输出明文令牌，消费环境变量或受管凭据文件，并在缺少凭据时安全暂停。

### 1. 只读生产语义与基线核验 (Read-Only Baseline & Semantics)
主脑或运维随时可执行的无副作用命令：
```bash
/home/service/clash-sub-parser/scripts/run_authenticated_acceptance.sh \
  -addr http://127.0.0.1:18080 \
  -output /tmp/csp-live-acceptance-report.json
```
- 预期输出：`/healthz` (PASS, status=ok), `/readyz` (PASS, tables=14, schema=13), `/api/v1/auth/status` (PASS, mode=protected)。
- 若未提供凭据：报告 `PAUSED_CREDENTIALS_REQUIRED`，退出码 0，保护管理端不被非法调用。

### 2. 带身份管理端只读审计 (Authenticated Read-Only Management Audit)
提供合法凭据后（通过环境变量或凭据文件）：
```bash
CSP_ADMIN_TOKEN="<managed-token>" /home/service/clash-sub-parser/scripts/run_authenticated_acceptance.sh \
  -addr http://127.0.0.1:18080 \
  -token-env CSP_ADMIN_TOKEN \
  -output /tmp/csp-live-acceptance-report.json
```
或使用受管文件路径（避免命令历史泄露）：
```bash
/home/service/clash-sub-parser/scripts/run_authenticated_acceptance.sh \
  -addr http://127.0.0.1:18080 \
  -token-file /path/to/managed-token.secret \
  -output /tmp/csp-live-acceptance-report.json
```
- 预期输出：验证 `/api/v1/settings/auth`、`/api/v1/subscriptions`、`/api/v1/nodes`、`/api/v1/policies`、`/api/v1/publications` 完整响应体语义。

### 3. 有界实网验收与代表性探测 (Bounded Live Acceptance Under Budget)
在审查机 PASS 且获授权后执行：
- 冻结预算：至多 3 个既有订阅各刷新 1 次；至多 6 个代表性既有节点低并发握手；总超时 10 分钟。
- 故障库存保留：抓取失败时旧节点保持 `active = 1`，不被抹除。
```bash
CSP_ADMIN_TOKEN="<managed-token>" /home/service/clash-sub-parser/scripts/run_authenticated_acceptance.sh \
  -addr http://127.0.0.1:18080 \
  -token-env CSP_ADMIN_TOKEN \
  -live-refresh \
  -max-subs 3 \
  -live-probe \
  -max-probes 6 \
  -timeout 10m \
  -output /tmp/csp-live-acceptance-report.json
```

### 4. 节点恢复执行指令 (Node Recovery Tooling - Exact Allowlist)
当前恢复白名单为 0。若未来获得特定节点的法定授权与因果证明：
```bash
# 默认 Dry-Run 检查
/home/service/clash-sub-parser/scripts/recover_misdeactivated_nodes.sh \
  --db /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db

# 指定精准已审查白名单恢复 (仅演练沙箱环境)
/home/service/clash-sub-parser/scripts/recover_misdeactivated_nodes.sh \
  --db /tmp/csp-test-replica.db \
  --apply \
  --allowlist "node_id_1,node_id_2" \
  --rehearsal-approval="orchestrator:drill-evidence"
```

---

## 五、 外部前置依赖与安全审批选项 (External Prerequisites & Actionable Choices)

1. **管理端令牌依赖**:
   - 生产数据库当前已启用管理端保护鉴权 (`admin_auth_enabled = 1`)，数据库中存储了 bcrypt 哈希令牌（设置于 2026-09-23）。
   - 主机 `.env` 文件未配置明文令牌，遵循“不密码爆破、不绕过鉴权、不伪造临时凭据”铁律。
   - **用户可选安全决策**:
     - **决策 A**: 用户/运维在运行实机带身份验收时，通过安全环境变量 `export CSP_ADMIN_TOKEN="..."` 或凭据文件提供合法令牌；
     - **决策 B**: 保持当前基线与只读安全审计，待生产有进一步变更时一并配置统一受管 Secret。

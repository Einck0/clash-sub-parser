## Context

参见 `proposal.md`。当前 SQLite 单主库中存量 1014 个节点，其中 31 个属于有效实时订阅成员（Scope E = 31，`active = 1`），983 个处于无源失活状态（`active = 0`）。取证表明，983 个孤儿中 960 个源自 Go 重写初期 `legacy.import` 遗漏 `node_source_links`，23 个源自后续订阅刷新下线剪枝时被物理清除 `node_sources`。

## Goals / Non-Goals

**Goals:**
- 建立独立的不可变历史账本表 `node_source_history`，保存节点与来源的历史因果关系与证明证据；
- 改造订阅刷新与生命周期写链，在成员下线、订阅删除时原子保留历史快照，杜绝运行时证据丢失；
- 提供 CLI 运维工具 `csp recover-source-history`，基于经过脱敏与 Hash 确权的清单幂等回填 983 个孤儿节点的历史事实；
- 提供只读 API `GET /api/v1/nodes/{id}/source-history` 并在现有节点详情 Drawer 中以只读方式展示历史归属；
- 建立端到端资产不变量门禁，确保回填与写链变动后生产库存状态 E 绝对不变、`active` 不变、`connection_revision` 不变、无虚假 `newfetch`。

**Non-Goals:**
- **严禁向 `node_sources` 回填存量 983 个孤儿节点**：`node_sources` 仅表征实时活跃成员，历史账本严格物理隔离；
- **严禁批量放活历史失活节点**：983 个节点维持 `active = 0`，不将其引入探针调度或对外发布池；
- **严禁在数据库或 API 中持久化敏感明文**：包括密码、Token、UUID、私钥及带鉴权 Token 的原始订阅 URL；
- **不重构既有 27 张业务表结构**：不重建 `nodes` 或 `subscriptions` 表，不修改现有外键关系；
- **不开设全局历史纠偏大平台**：UI 仅在现有节点抽屉内提供轻量只读历史审计视口。

## Decisions

### D1: 独立历史账本表 (`node_source_history`) 优于扩展 `node_sources`
- **方案选择**：新增独立表 `node_source_history`。
- **设计要点**：
  - `id TEXT PRIMARY KEY`: UUIDv7 唯一标识；
  - `node_logical_id TEXT NOT NULL`: 外键引用 `nodes(logical_id)`；
  - `subscription_id TEXT`: Nullable 外键引用 `subscriptions(id) ON DELETE SET NULL`；
  - `source_identity TEXT NOT NULL`: 稳定命名空间标识（如 `legacy:src:1` 或当前 UUID），即使订阅被删仍保留历史身份；
  - `source_label TEXT NOT NULL`: 脱敏展示名称（如 `7li`, `魔戒`, `einck-qzz`）；
  - `connection_revision INTEGER`: Nullable，记录观测发生时的连接版本号；
  - `relation_state TEXT NOT NULL`: `verified | conflict | unknown`；
  - `cause TEXT NOT NULL`: `legacy_import | refresh_removed | subscription_deleted | manual_confirmed | unresolved`；
  - `first_observed_at TEXT NOT NULL`, `last_observed_at TEXT NOT NULL`: 严格基于历史实际观测时间；
  - `evidence_kind TEXT NOT NULL`: 证据类别（如 `legacy_archive_link`, `v1_backup_snapshot`）；
  - `evidence_json TEXT NOT NULL`: 脱敏证据详情（含备份 SHA-256、表主键键值、来源 URL 规范化路径，排除一切敏感凭据）；
  - 唯一索引：`UNIQUE(node_logical_id, source_identity, evidence_kind, first_observed_at)`，支持多来源与多次历史重入。
- **备选考量**：在 `node_sources` 增加 `is_historical` 字段。**否决原因**：现有业务大量查询（包括探针排队、对外发布、有效节点统计）依赖 `SELECT ... FROM node_sources`，一旦混入历史非活跃关联，将导致 Scope E 污染与灾难性穿透。

### D2: 订阅生命周期写链防断链改造
- **刷新下线剪枝 (Refresh Pruning)**：
  - 在 `internal/application/inventory/service.go` 的 `reconcile` 事务中，执行 `DELETE FROM node_sources` 前，先以 `INSERT OR IGNORE INTO node_source_history` 将即将被删除的 `(node_logical_id, subscription_id)` 关联快照持久化，带上当前订阅名称、`fetch_id` 及当前 `connection_revision`，`cause` 标记为 `refresh_removed`；
  - 随后安全删除 `node_sources`。
- **抓取失败与停用**：
  - 抓取失败时保持事务回滚或保持现状，不删除既有 `node_sources`，不产生虚假历史记录；
  - 停用订阅（`enabled = 0`）仅更新配置，不触碰 `node_sources` 亦不触碰 `node_source_history`。
- **订阅物理删除**：
  - 在删除 `subscriptions` 记录前，触发历史归档钩子，对该订阅当前持有的全部节点关系执行快照留存（`cause = 'subscription_deleted'`）；随后由外键 `ON DELETE SET NULL` 将历史表中的 `subscription_id` 置 NULL，但 `source_label` 与 `source_identity` 永久保留。
- **用户覆盖 (CoW Overrides)**：
  - 用户修改节点参数仅记录于 `node_overrides`，历史来源关系按底层节点标识稳定继承，不篡改上游来源记录。

### D3: 确定性单次回填运维命令 (`csp recover-source-history`)
- **CLI 架构**：
  - 命令：`csp recover-source-history --manifest <path> [--dry-run]`；
  - 输入：脱敏校验清单 `/tmp/csp_legacy_attribution_evidence/merged_cross_verified_manifest_v2.json`，内含清单 SHA-256、节点清单、证据来源及首选来源标记；
  - 前置检查：验证数据库可读写，统计当前 `total_nodes` (1014)、`active_nodes` (31)、`node_sources` (31)；
  - 执行事务：单事务批量写入 983 条首选历史归属记录与 15 条次要多来源证据记录；遇丢弃碰撞凭据记录仅作为丢弃证据留存，不作为有效归属挂载；
  - 后置门禁：验证回填后 `total_nodes = 1014`, `active_nodes = 31`, `node_sources = 31`，若发生任何偏差强制事务回滚（Fail-Closed）。

### D4: 规范连接比对与重复端点碰撞处理
- **首行规范比对确认**：取证已证实当前生产 960 个节点的配置与归档按 `ORDER BY id ASC` 提取的首行记录在协议、凭据（密码/UUID/私钥）及 TLS 参数上 100% 吻合；
- **首行归属已核验**：首选首行自身的 `node_source_links` 具有单一、明确且稳定的上游订阅（7li: 54, 魔戒: 31, einck-qzz: 9, Dogegg: 16, Eeox: 13, 公开节点: 79, Github: 757, WARP: 1），标记为 `verified`；
- **异凭据丢弃记录隔离**：对于 14 个存在重复端点的多行记录，13 个次行凭据相同作为次要来源保留，1 个次行凭据不同（pk 9669 from Eeox）作为丢弃冲突事实记录，严禁附着于当前节点。

### D5: 只读审计 API 与节点详情抽屉展示
- **API 契约**：
  - `GET /api/v1/nodes/{id}/source-history`
  - 响应：`{ "data": { "current_sources": [...], "history": [...], "attribution_status": "..." } }`
  - 字段全面脱敏，绝对不输出明文凭据。
- **前端适配**：
  - 复用现有节点详情 Drawer 组件，新增只读 Tab 或卡片呈现，主列表与核心路由不受影响。

## Risks / Trade-offs

- [历史归属回填误污染实时发布] → 严格物理分表，发布编译器与探针调度器仅读取 `node_sources` 与 `nodes`，不读取 `node_source_history`；
- [回填过程中并发写锁冲突] → 使用 short transaction 与 busy timeout，回填工具在受控维护窗口由 CLI 执行；
- [脱敏清单防篡改] → 清单内置自身 SHA-256 与生成元数据，CLI 执行前比对校验和；
- [订阅删除后历史丢失] → 采用 `ON DELETE SET NULL` 结合非空 `source_label` 与 `source_identity`，保证数据永久可读。

## Migration Plan

1. 执行 Migration `000016_node_source_history.sql` 创建独立账本表；
2. 构建并发布包含写链保护、只读 API 与 CLI 回填工具的应用版本；
3. 执行前在线事务热备数据库并校验 `PRAGMA integrity_check`；
4. 运行 `csp recover-source-history --dry-run` 预检脱敏清单；
5. 正式运行 `csp recover-source-history --manifest ...`，原子入库并核验前后资产状态一致性（1014 / 31）；
6. 验收只读端点 `/api/v1/nodes/{id}/source-history` 与前端 Drawer 展示。

## Why

当前 CSP 系统在节点来源归属（Provenance & Source Attribution）上存在历史断链与静默丢失问题：
1. **历史导入来源事实缺失**：在最初 Go 重写期的 `legacy.import` 批处理中，实现仅抽取了 `nodes` 与 `subscriptions` 表，遗漏了读取和持久化旧系统的 `node_source_links` 与 `source_revisions`。导致 960 个合法历史节点初始入库时 `node_sources` 关联记录即为 0，随后在 2026-09-29 的刷新中被旧全局孤儿清理逻辑误失活为 `active = 0`；
2. **动态刷新下线证据抹除**：现行订阅刷新（Reconciliation）在对比上游增量时，直接以 `DELETE FROM node_sources` 物理删除下线节点的关联关系，导致 23 个曾有真实来源的节点（如 `einck-qzz` 下线节点）彻底失去历史凭据，沦为无源孤儿；
3. **缺乏独立历史归属台账**：系统仅维护表征“当前实时订阅成员”的 `node_sources` 表（当前仅 31 个节点有效），若为了恢复历史事实而直接向 `node_sources` 回填或放活节点，将严重污染当前对外导出、发布及探针调度的作用域 E（Scope E = 31），破坏不可变业务基线；
4. **端点去重碰撞凭据风险**：旧导入基于 `(proto, server, port)` 去重，导致少量同一端点但不同凭据的记录被丢弃。若不加甄别地将丢弃记录关联至当前节点，将引入凭据污染。

必须以最小侵入模型建立独立的不可变历史来源台账（`node_source_history`），在绝对不触碰当前库存状态 E、不伪造新抓取、不修改节点连接参数的前提下，闭环固化 983 个孤儿节点的历史因果事实，修复刷新写链中的证据丢失漏洞，并提供只读审计接口与受控演练工具。

## What Changes

- **增量引入不可变历史账本表 (`node_source_history`)**：
  - 新增 Migration `000016_node_source_history.sql`，独立于实时 `node_sources`；
  - 核心字段：`id`, `node_logical_id` (FK nodes), `subscription_id` (Nullable FK subscriptions ON DELETE SET NULL), `source_identity` (稳定命名空间+历史源ID/映射), `source_label` (脱敏展示名称), `connection_revision` (Nullable), `relation_state` (`verified|conflict|unknown`), `cause` (`legacy_import|refresh_removed|subscription_deleted|manual_confirmed|unresolved`), `first_observed_at`, `last_observed_at`, `evidence_kind`, `evidence_json` (严格杜绝密码、UUID、Token 及带鉴权 query 的 raw URL)；
  - 联合唯一约束基于 `(node_logical_id, source_identity, evidence_kind, first_observed_at)` 防重，支持同一节点多次历史观测及结构化 `source_unknown` 哨兵记录。
- **修复运行时写链丢失漏洞**：
  - **刷新下线保护**：成功刷新在删除 `node_sources` 成员前，在同一事务中将既有关系的真实性、快照标签、`fetch_id` 及当前 `connection_revision` 归档至 `node_source_history`（`cause = 'refresh_removed'`）；
  - **抓取失败保护**：刷新失败时保持既有状态，绝不物理删除亦不伪造历史归档；
  - **订阅启用/停用隔离**：切换订阅状态不解绑 `node_sources`，不篡改节点 `active`，不触发历史虚假归档；
  - **订阅删除级联解耦**：删除订阅前先对现有关联建立脱敏快照（`cause = 'subscription_deleted'`, `source_label` 固化），随后物理外键级联将 `subscription_id` 置 NULL，明确区分“历史源已删除”与“未知无源”；
  - **CoW 与用户覆写隔离**：用户在节点上的字段级人工修改（`node_overrides`）仅记录节点参数变更，历史来源事实按实际节点血统继承，不将用户自定参数归咎于上游。
- **提供离线受控历史回填维护命令 (`csp recover-source-history`)**：
  - 提供单次可重入 CLI 工具，只读解析带 SHA-256 校验的脱敏清单（`recovery-manifest.json`）；
  - 严格仅向 `node_source_history` 插入不可变证据记录，绝对禁止触碰 `nodes.active`、`nodes.connection_revision` 或写入 `node_sources`；
  - 支持 `--dry-run` 预检与原子事务；前置与后置强制核验生产库存不变量（1014 总资产、31 活跃、Scope E 不变）。
- **新增只读节点来源与历史审计 API**：
  - 增加 `GET /api/v1/nodes/{id}/source-history`，返回当前源列表、历史观测事实列表与聚合的 `attribution_status` (`current|historical_verified|manual_confirmed|conflict|unknown`)；
  - 在节点详情侧栏/抽屉（Drawer）中以只读方式展示历史归属事实与证据摘要，主 UI 保持当前资产默认视图，不开设全局历史管理平台或 URL 猜测输入。

## Capabilities

### New Capabilities
- `inventory/node-source-history-ledger`: 建立独立增量表 `node_source_history`，保存节点历史来源事实与证据链，解耦实时订阅成员（Scope E）与历史因果记录。
- `inventory/source-history-write-chain`: 改造订阅刷新、下线剪枝、订阅删除与手动操作生命周期，确保来源事实在变更发生时原子快照归档，彻底消除物理误删导致的断链。
- `inventory/source-history-recovery-cli`: 实现 `csp recover-source-history` 工具，基于不可变脱敏清单幂等补齐存量 983 个历史孤儿节点证据，内置严格资产不变量门禁。
- `inventory/node-source-history-api-view`: 暴露只读历史审计端点并在前端节点抽屉中安全展示来源归属与历史状态，严格屏蔽一切凭据与敏感 URL。

### Modified Capabilities

## Impact

- **数据库层**：增量新增 Migration `000016_node_source_history.sql`，无破坏性 DDL，不影响既有 27 张表任何结构。
- **后端服务**：
  - `internal/domain/`: 增加 `NodeSourceHistory` 实体与枚举定义；
  - `internal/repository/sqlite/`: 增加 `node_source_history` 的存储与事务查询接口；
  - `internal/application/inventory/`: 在刷新剪枝与删除逻辑中嵌入历史快照归档事务；
  - `internal/transport/http/`: 增加 `/api/v1/nodes/{id}/source-history` 路由处理函数；
  - `cmd/csp/`: 增加 `recover-source-history` 子命令。
- **前端页面**：`web/src/views/` 节点详情 Drawer 增加只读来源历史选项卡/面板。
- **运行时环境**：零停机无损热更新，不影响正在运行的探针调度、发布编译及已有 31 个在线节点服务。

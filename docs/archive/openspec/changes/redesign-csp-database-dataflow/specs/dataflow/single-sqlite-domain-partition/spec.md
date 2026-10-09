## Purpose

规范 CSP 物理数据库拓扑与领域所有者边界，确立单 SQLite 主库与 WAL 运行模式，归口 Schema 迁移生命周期，全量保留现有业务表资产，并提供实测维护短窗与可执行的回滚对账规范。

## ADDED Requirements

### Requirement: 单 SQLite 主库与 WAL 运行模式
系统 SHALL 维持单一 SQLite 主业务数据库文件（`/data/csp-v1.db`），启用 WAL 日志模式（`PRAGMA journal_mode = WAL`）、普通同步等级（`PRAGMA synchronous = NORMAL`）、内置外键约束检查（`PRAGMA foreign_keys = ON`）以及 5000 毫秒忙等待锁（`PRAGMA busy_timeout = 5000`）。系统 MUST 保证写操作在同一进程内事务性串行执行，避免无意义的多库拆分与分布式复杂性。

#### Scenario: 验证主库 WAL 与外键配置生效
- **WHEN** 系统启动并打开主数据库连接
- **THEN** 系统执行 `PRAGMA journal_mode` 返回 `wal`，且 `PRAGMA foreign_keys` 返回 `1`

### Requirement: 领域所有权、27 表现状与增量关联表管理
系统 SHALL 将现有的 27 张数据表（1 张技术表与 26 张业务表，包含 7 张 IP Risk 表）严格按领域划分为 Ingestion（摄入）、Inventory（库存）、Policy Topology（策略拓扑）、Probes（探针与风险）、Publication（发布工件）与 Settings/Audit（系统配置与审计）六大界限上下文，明确读写所有权：
1. **Ingestion 域** (`subscriptions`, `subscription_fetches`, `admission_rules`, `subscription_payloads`, `subscription_entries`): 负责订阅源凭证、拉取记录与原始负载解析；
2. **Inventory 域** (`nodes`, `node_sources`, `node_connection_versions`, `node_connection_heads`, `node_overrides`): 负责稳定节点资产、多源所有权关系、连接版本、权威当前版本指针与用户显式修改；
3. **Policy Topology 域** (`node_groups`, `group_edges`, `group_node_filters`, `global_node_filters`, `policy_rules`, `configuration_revisions`): 负责策略拓扑、路由规则与版本快照；
4. **Probes 域** (`probe_schedules`, `probe_batches`, `probe_batch_runs`, `probe_runs`, `probe_observations`, `ip_risk_provider_settings`, `ip_risk_observations`, `risk_policy_revisions`, `risk_policy_providers`, `risk_policy_score_bands`, `risk_policy_trait_rules`, `risk_policy_group_bindings`): 负责轻量级与平台能力探测调度、租约控制与统一 Nullable 1:N 测量记录；
5. **Publication 域** (`publications`, `publication_payload_refs`): 负责不可变导出快照、分发清单与 Payload 物理引用保护；
6. **Settings/Audit 域** (`settings`, `audit_events`, `schema_migrations`): 负责全局安全鉴权、操作流水与数据架构版本。

#### Scenario: 业务表写入权唯一归口
- **WHEN** 探针调度器执行探测完成并生成观测数据
- **THEN** 仅允许由 Probe 领域服务写入 `probe_observations`，其他领域服务不可直接跨域修改

### Requirement: 维护短窗实测与受控回滚对账
系统在执行架构迁移时，SHALL 在隔离演练中实测真实的停写耗时，并获得用户明确确认，严禁宣称固定 30 秒或绝对零丢失；系统 SHALL 在迁移前执行 SQLite 官方 `.backup` 在线热备；发生未预期异常时，系统 MUST 提供明确的回滚路径：
1. 校验热备文件的 1014 节点、29 组、163 规则基准；
2. 若切换后产生新写入，先执行 SQL 增量导出，再执行快照覆盖回滚；
3. 向操作员呈报受影响数据窗口并执行增量数据合并，或由操作员显式确认并接受数据 Loss 窗口。

#### Scenario: 隔离演练实测维护耗时
- **WHEN** 在隔离测试环境中对 1014 节点执行 Migration 000015 演练
- **THEN** 测量并记录真实的 DDL 与指针初始化耗时，输出演练报告供上线审批

### Requirement: DDL 唯一归口 MigrationRunner
系统 SHALL 将所有数据表结构定义、索引变更与历史字段修改唯一归口于 `internal/repository/sqlite/` 中的顺序迁移脚本。系统 MUST 严禁在 Repository 构造函数或业务代码中动态执行 `CREATE TABLE`、`CREATE INDEX` 或 `ALTER TABLE`，防止隐式 DDL 竞争与架构漂移。

#### Scenario: 构造 Repository 实例时不触发运行时 DDL
- **WHEN** 应用程序初始化 `NewProbeObservationRepository(db)`
- **THEN** 构造函数仅接收只读连接句柄并返回实例，不向数据库发送任何 `CREATE INDEX` 或 `ALTER TABLE` 语句

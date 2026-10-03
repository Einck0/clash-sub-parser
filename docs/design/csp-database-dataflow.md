# CSP 数据库与上下游数据流重构架构设计方案 (Redesign Blueprint - Rev 3 Remediated)

- **设计版本**: `v1.3.0-remediated`
- **关联 Run**: `pi_run_20261003_112606_25876`
- **关联 OpenSpec Change**: `redesign-csp-database-dataflow`
- **合同血统**: `fleet-contract-v3 / redesign-csp-database-dataflow`
- **意图包校验**: SHA-256 `272752e2762f0b94177ba08252ae76f71eec50b08b4ca61f42950a4dd9657dda`
- **设计状态**: `Architecture & Remediation Complete (Implementation Awaiting Approval)`

---

## 根因定位与本轮整改声明 (Root Cause & Remediation Declaration)

上轮独立审查驳回（`FAIL`）的本质根因定位：
1. **条件机械泛化**: 将 Dogegg 特定的来源语义证据机械泛化为通用单项过滤规则，将回环地址、端口 1、流量关键词单项判定为公告伪节点，存在误杀正常 localhost/私网代理的风险；
2. **多 Provider 关联简单化**: 将 IP Risk 领域模型简单化为 1:1 双写，忽略了单次节点评估下可能并存多个情报提供商（1:N）以及历史独立风险记录无对应通用探针运行（Nullable FK）的客观事实；
3. **未证参数与虚假承诺**: 凭空引入未经实测确认的“30 秒维护短窗”与“Zero-Loss”绝对保证；对 Mihomo xhttp 渲染规则缺乏官方代码级核对，缺少未知 extra 字段安全隔离与 safe_detail_json 白名单防泄密机制；
4. **外键约束与实体身份折叠**: 误在逻辑 JSON manifest 中宣称 SQLite 外键自动约束，未设计独立的关联表进行引用完整性约束；误将 `(sub, source_key)` 设为唯一，导致同一 Payload 内多个相同连接参数的公告信息被折叠。

本轮设计严格按照主脑整改闭环要求，全面重构契约，统一全部规范、DDL、API 与测试方案。

---

## 一、 现状全量只读调查与数据资产核实 (Forensic Investigation)

### 1. 唯一主库物理状态与运行模式
- **物理路径**: `/data/csp-v1.db`，无分裂、无损坏，`PRAGMA integrity_check` 返回 `ok`，`PRAGMA foreign_key_check` 返回空；
- **运行模式**: SQLite WAL 模式（`PRAGMA journal_mode = wal`），普通同步级别（`PRAGMA synchronous = normal`），外键约束开启（`PRAGMA foreign_keys = on`），忙等待 `busy_timeout = 5000`；
- **连接模型**: 单写者串行事务，读写连接池隔离，系统无外部第二数据库，无锁争用或死锁证据。

### 2. 存量 27 张表与领域所有者准确映射 (1 技术表 + 26 业务表)

经逐字对照 `migrations/000001_initial_schema.sql` 至 `000014_probe_observation_evidence_data.sql`（特别核对 `000002_ip_risk_schema.sql`），确证存量 27 张表的准确主键、外键与读写所有权：

| 序号 | 表名 | 领域所有者 (Domain) | 真实主键 (PK) | 核心外键约束 (FK) | 现状行数 | 读写所有权与职责演进 |
|---|---|---|---|---|---|---|
| 1 | `schema_migrations` | Migration | `version` (INT) | 无 | 14 | 写: `MigrationRunner`; 读: `MigrationRunner`。严格记录版本与时间。 |
| 2 | `settings` | Settings & Auth | `id` (INT CHECK=1) | 无 | 1 | 写: `SettingsRepo` (Admin API); 读: `AuthMiddleware`, `Main`。单行配置。 |
| 3 | `subscriptions` | Ingestion | `id` (TEXT) | 无 | 9 | 写: `SubRepo` (Admin API); 读: `Inventory`, `PeriodicCoordinator`。源凭证与调度。 |
| 4 | `subscription_fetches` | Ingestion | `id` (TEXT) | `subscription_id -> subscriptions(id) CASCADE` | 18 | 写: `Inventory.Refresh`; 读: `FetchRepo`, UI。记录抓取流水与状态。 |
| 5 | `nodes` | Inventory | `logical_id` (TEXT) | 无 | 1014 | 写: `Inventory.Reconcile`; 读: `Resolver`, `ProbeRunner`, `Compiler`。保持 1014 稳定资产。 |
| 6 | `node_sources` | Inventory | `(node_logical_id, subscription_id)` | CASCADE 双向外键 | 31 | 写: `Inventory.Reconcile`; 读: `Resolver`, `Recovery`。多对多源从属关系。 |
| 7 | `node_groups` | Policy Topology | `id` (TEXT) | 无 | 29 | 写: `PolicyRepo` (Admin API); 读: `Resolver`, `Compiler`。策略组定义。 |
| 8 | `group_edges` | Policy Topology | `id` (TEXT) | CASCADE 双向外键 | 64 | 写: `PolicyRepo` (Admin API); 读: `Resolver`。组间树状/网状拓扑边。 |
| 9 | `group_node_filters` | Policy Topology | `group_id` (TEXT) | `group_id -> node_groups(id) CASCADE` | 0 | 写: `PolicyRepo`; 读: `Resolver`。组级别动态过滤规则。 |
| 10 | `global_node_filters`| Policy Topology | `id` (INT CHECK=1) | 无 | 1 | 写: `PolicyRepo`; 读: `Resolver`。全局准入/发布两级过滤规则。 |
| 11 | `policy_rules` | Policy Topology | `id` (TEXT) | `target_group_id -> node_groups(id) CASCADE` | 163 | 写: `PolicyRepo`; 读: `Resolver`, `Compiler`。分流匹配规则。 |
| 12 | `configuration_revisions`| Policy Topology| `id` (TEXT)| `parent_id -> configuration_revisions(id) SET NULL` | 1 | 写: `RevisionRepo` (Admin API Save); 读: `AuditService`。策略拓扑不可变快照。 |
| 13 | `publications` | Publication | `id` (TEXT) | 无 | 0 | 写: `PublicationService`; 读: `Export Handler`。不可变发布快照与清单。 |
| 14 | `probe_schedules` | Probes | `id` (INT CHECK=1) | 无 | 1 | 写: `PeriodicCoordinator`; 读: `PeriodicCoordinator`。探针调度器控制。 |
| 15 | `probe_batches` | Probes | `id` (TEXT) | 无 | 4219 | 写: `PeriodicCoordinator`; 读: `PeriodicCoordinator`。调度窗口与 CAS 分布式租约。 |
| 16 | `probe_batch_runs` | Probes | `(batch_id, probe_run_id)` | CASCADE 双向外键 | 476 | 写: `PeriodicCoordinator`; 读: UI。批次与运行实例关联。 |
| 17 | `probe_runs` | Probes | `id` (TEXT) | 无 | 494 | 写: `ProbeService`, `Coordinator`; 读: `ProbeRunner`, UI。探测任务生命周期。 |
| 18 | `probe_observations` | Probes | `id` (TEXT) | CASCADE 节点与 Run 外键 | 7087 | 写: `ProbeRunner`; 读: `Resolver`, `ReadModel`。单节点探针观测记录。 |
| 19 | `audit_events` | Audit | `id` (TEXT) | 无 | 42 | 写: `AuditMiddleware`; 读: UI。管理操作脱敏流水。 |
| 20 | `admission_rules` | Ingestion | `id` (TEXT) | 无 | 0 | 写: `Resolver` (Admin API); 读: `Resolver`。节点入库准入规则。 |
| 21 | `ip_risk_provider_settings`| IP Risk | `(provider, schema_version)` | 无 | 0 | 写: `IPRiskRepo` (Admin API); 读: `IPRiskProbe`。服务商查询配置。 |
| 22 | `ip_risk_observations` | IP Risk | `id` (TEXT) | `node_logical_id -> nodes(logical_id) RESTRICT`, `(provider, provider_schema_version) -> ip_risk_provider_settings` | 0 | 写: `IPRiskProbeRunner`; 读: `NodeRiskResolver`。详细威胁情报记录。 |
| 23 | `risk_policy_revisions` | IP Risk | `id` (TEXT) | 无 | 0 | 写: `RiskPolicyRepo` (Admin API); 读: `NodeRiskResolver`。风险策略版本控制。 |
| 24 | `risk_policy_providers` | IP Risk | `(revision_id, provider, schema_version)` | `revision_id -> risk_policy_revisions(id) CASCADE`, `(provider, schema_version) -> ip_risk_provider_settings` | 0 | 写: `RiskPolicyRepo`; 读: `NodeRiskResolver`。服务商优先级排序。 |
| 25 | `risk_policy_score_bands`| IP Risk | `(revision_id, min_score, max_score)` | `revision_id -> risk_policy_revisions(id) CASCADE` | 0 | 写: `RiskPolicyRepo`; 读: `NodeRiskResolver`。风险分段处置动作。 |
| 26 | `risk_policy_trait_rules`| IP Risk | `(revision_id, trait)` | `revision_id -> risk_policy_revisions(id) CASCADE` | 0 | 写: `RiskPolicyRepo`; 读: `NodeRiskResolver`。特征威胁阻断规则。 |
| 27 | `risk_policy_group_bindings`| IP Risk | `group_id` (TEXT) | `group_id -> node_groups(id) CASCADE`, `policy_revision_id -> risk_policy_revisions(id) CASCADE` | 0 | 写: `RiskPolicyRepo`; 读: `NodeRiskResolver`。风险策略与代理组绑定。 |

### 3. 节点 LogicalID 算法与存量资产保护
- **Path 1 (Base Logical ID)**: `ComputeNodeLogicalID` 提取规范协议、主机、端口及非敏感参数（排除 UUID/密码），前缀 `node_` 加 SHA-256 前 16 字节十六进制；
- **Path 2 (Scoped Candidate ID)**: 当多源出现同参数节点或需写时隔离时，拼接 `subscriptionID + "|" + baseLogicalID` 派生 scoped ID；
- **1014 节点对账**: 31 个活跃节点与 983 个历史失活节点均受严格保护。983 个失活节点为历史禁用资产，完整保留拓扑与历史记录，**严禁进行全库重新哈希或破坏性去重**。

### 4. Dogegg 上游原始响应实机对账 (/tmp/csp-evidence-20261003/)
- 原始响应包含 18 个条目：**3 个公告伪节点 + 15 条代理线路**；
- 3 个公告伪节点具有**完全相同的连接参数**（`server: 127.0.0.1`, `port: 1`, `network: ws`, `sni: localhost`, 全零占位 UUID），但携带完全不同的信息语义（`剩余流量：76.9GB`、`套餐到期：长期有效`、`新域名：https://traffic.dogeggo.us.ci`）；
- **实体属性结论**: 相同连接参数无法区分上游元数据实体。这并非密码学哈希碰撞，而是上游将连接参数用作占位符、仅靠标题传递不同公告信息的业务行为。**具体面板软件未知**（不宣称确知某特定面板品牌）。

### 5. 探针旧 Run 失败根因与在线分布客观边界
- Probe Run `01a0ff6f-fb8a-7717-bd02-ce4d9610c2de` 创建于旧版容器运行期，旧内核在 `classifyDialFailure` 中丢弃了原始底层 error 详情，导致构建失败的底层参数在数据层面已永久丢失。**当前未知实际失败根因，严禁凭空伪造定论**；
- 当前 31 个活跃节点在线探测客观分布（最新 Baseline 窗口）：`available` 15 个，`error` 16 个（9 个 `transport_error`，7 个 `timeout`），`client_build_failed` 为 0。Mihomo 副本成功解析配置仅证明配置结构有效，不代表实网连通性。

---

## 二、 目标数据模型与增量事实层 (Target ER Schema)

### 1. 架构总原则与无环外键拓扑
针对 SQLite 不支持 `ALTER TABLE ADD` 复合外键且 `nodes` 与 `versions` 易形成双向循环外键的问题，确立严格单向无环模型：
- `nodes` 表保持稳定资产标识，**不添加表级外键反向引用 versions**（避免循环依赖，无需重建 `nodes` 表）；
- `node_connection_versions` 单向外键引用 `nodes(logical_id)`；
- `node_connection_heads` 单向外键引用 `nodes(logical_id)` 并通过复合外键单向引用 `node_connection_versions(node_logical_id, connection_revision)`；
- 权威当前版本指针唯一由 `node_connection_heads.connection_revision` 表达；
- 旧 `nodes.connection_revision` 字段在过渡期仅作为只读投影视图，完成切换后严禁多事实源双写。

```
+--------------------------+       1:1       +--------------------------+
|  subscriptions           |---------------->|  subscription_payloads   | (Authoritative Raw Upstream)
+--------------------------+                 +--------------------------+
             |                                             | 1:N
             | 1:N                                         v
+--------------------------+                 +--------------------------+
|  subscription_fetches    |                 |  subscription_entries    | (Derived Entries + Proven Notice Classifier)
+--------------------------+                 +--------------------------+
             |                                             |
             +----------------------+----------------------+
                                    | 1:N
                                    v
                         +--------------------------+
                         |      node_sources        |
                         +--------------------------+
                                    | N:1
                                    v
+--------------------------+  1:N   +--------------------------+   1:1   +--------------------------+
| node_connection_versions |<-------|         nodes            |-------->|  node_connection_heads   | (Canonical Current Pointer)
+--------------------------+        +--------------------------+         +--------------------------+
             ^                                     | 1:N                              |
             |                                     v                                  | Composite FK:
             | 1:N                          +--------------------------+              | (logical_id, rev)
             |                              |      node_overrides      |              +----------------------+
+--------------------------+                +--------------------------+                                     |
| publication_payload_refs |                               | (Resolved into Effective Config)                v
+--------------------------+                               +-------------------------------------------------+
             | N:1
             v
+--------------------------+
|      publications        | (Immutable Draft / Published Snapshots)
+--------------------------+
```

### 2. 6 张新增核心表 DDL 规范与约束设计

#### (1) `subscription_payloads` (原始响应权威事实表)
```sql
CREATE TABLE subscription_payloads (
    id TEXT PRIMARY KEY, -- 格式: "payload_" + UUIDv7
    subscription_id TEXT NOT NULL,
    fetch_id TEXT NOT NULL,
    content_digest TEXT NOT NULL, -- SHA-256 校验和
    body_blob BLOB NOT NULL, -- 原始字节流 (支持 gzip 压缩存储)
    http_status INTEGER NOT NULL DEFAULT 200,
    headers_json TEXT NOT NULL DEFAULT '{}', -- 排除 Cookie / Authorization
    pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
    created_at TEXT NOT NULL,
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE,
    FOREIGN KEY (fetch_id) REFERENCES subscription_fetches(id) ON DELETE CASCADE
);
CREATE INDEX idx_sub_payloads_sub_created ON subscription_payloads(subscription_id, created_at DESC);
CREATE INDEX idx_sub_payloads_digest ON subscription_payloads(content_digest);
```

#### (2) `subscription_entries` (派生条目与来源证据表)
```sql
CREATE TABLE subscription_entries (
    id TEXT PRIMARY KEY, -- 每 payload 独立 PK (UUIDv7)
    payload_id TEXT NOT NULL,
    subscription_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL, -- Payload 内原始顺序 (0-indexed)
    source_key TEXT NOT NULL, -- 跨刷新匹配候选锚点 (非全局唯一，严禁 UNIQUE 折叠)
    raw_name TEXT NOT NULL,
    protocol TEXT NOT NULL,
    server TEXT NOT NULL DEFAULT '',
    port INTEGER NOT NULL DEFAULT 0,
    entry_kind TEXT NOT NULL CHECK (entry_kind IN ('proxy', 'notice', 'unknown')),
    classification_reason TEXT NOT NULL DEFAULT '',
    classification_version TEXT NOT NULL DEFAULT 'v2-proven-combo',
    source_provenance_json TEXT NOT NULL DEFAULT '{}', -- 包含上游上下文与语义槽证据
    user_kind_override TEXT CHECK (user_kind_override IN ('proxy', 'notice', 'unknown')),
    override_anchor TEXT NOT NULL DEFAULT '', -- 稳定匹配锚点
    override_reason TEXT NOT NULL DEFAULT '',
    override_at TEXT,
    actor_ref TEXT NOT NULL DEFAULT '', -- 既有审计操作者语义
    parsed_config_json TEXT NOT NULL DEFAULT '{}',
    parser_version TEXT NOT NULL DEFAULT '1.0.0',
    warnings_json TEXT NOT NULL DEFAULT '[]',
    node_logical_id TEXT, -- 关联的节点 ID (notice 时为 NULL)
    created_at TEXT NOT NULL,
    FOREIGN KEY (payload_id) REFERENCES subscription_payloads(id) ON DELETE CASCADE,
    FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE,
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE SET NULL,
    UNIQUE (payload_id, ordinal) -- 保证同一 Payload 内行号唯一，同参数条目独立写入
);
CREATE INDEX idx_sub_entries_source_key ON subscription_entries(subscription_id, source_key);
CREATE INDEX idx_sub_entries_node ON subscription_entries(node_logical_id);
```

**公告伪节点确定性分类规则与证据契约**:
- **严禁单项判空或通用 OR**: 严禁将回环地址、私网 IP、端口 1、占位 UUID 或标题含“流量”中的任何单项或简单 OR 作为分类依据；
- **前置来源门禁与精准组合判定 (Verified Source Rule Gate & Exact Rule Match)**:
  主脑严禁将回环/私网 + port 1 + 全零凭据 + 提示语义作为全局通用的自动 notice 规则。分类器必须遵循两级严格前置门禁：
  $$\text{verified\_source\_rule}(\text{subscription\_id}, \text{source\_evidence}, \text{rule\_version}) \land \text{exact\_rule\_match}(\text{entry}) \implies \text{notice}$$
  1. **前置来源门禁 `verified_source_rule`**: 仅对已完成实机取证、来源上下文确证且包含版本化分类规则的特定订阅源（如本轮已取证核准的 Dogegg 订阅及其 `source_provenance` 结构与 `rule_version = 'v2-dogegg-proven'`）启用该专用匹配规则。记录 `source_provenance` 仅为审计证据，不能代替针对该源的前置规则验证！未配置或未通过 `verified_source_rule` 的其他任何订阅源，严禁套用该规则；
  2. **精确条目匹配 `exact_rule_match` (全条件组合)**:
     在满足前置门禁的订阅源内，条目必须**同时满足全部四项条件**：
     a) `server IN ('127.0.0.1', 'localhost', '0.0.0.0')`；
     b) `port == 1`；
     c) 凭据 UUID 严格符合 RFC 4122 全零测试占位符（`00000000-0000-4000-8000-000000000000`）；
     d) 标题文本明确匹配来源提示语义（“剩余流量”、“套餐到期”、“新域名”、“公告”、“通知”）；
  3. **未通过前置门禁与未知处理**: 若订阅源缺乏已验证规则，即使条目满足上述四项连接与文本特征，系统也**绝对不自动归类为 notice**，而是将其保留为 `unknown` 候选状态并允许用户人工审核与显式纠偏，绝不自动推广全网；
  4. **正常代理免审批**: 正常可解析的真实代理连接（包括使用本地私网 IP 但具备有效端口与认证凭据的代理）始终归类为 `proxy`，正常入库流转，无需额外人工审批。
- **条目级人工纠偏与审计**: 即使 `node_logical_id = NULL`，用户仍可在条目级提交 `user_kind_override = 'proxy'`，携带 `override_anchor`、原因、时间戳及 `actor_ref`。
- **跨刷新匹配与去重契约**:
  - `entry_id` 是每 Payload 独立的 PK，Dogegg 的 3 个同参数公告必须**全部独立写入，严禁去重**！
  - 严禁对 `(subscription_id, source_key)` 施加 UNIQUE 约束，防止折叠同 Payload 真实数据；
  - 跨刷新继承优先级：1) 上游稳定 key；2) 版本化分类器提取的明确 `semantic_slot`（如 `traffic_remaining`, `expiry_date`，严禁使用盲目 strip-digits）；
  - 无可靠锚点时，仅在上下文与内容完全 1:1 唯一匹配时继承 override；新内容、改名或两项具有相同锚点引发歧义时，**严禁强制继承或合并**，全部保留并输出冲突诊断，由用户显式匹配，不中断正常连接。

#### (3) `node_connection_versions` (不可变连接版本表)
```sql
CREATE TABLE node_connection_versions (
    node_logical_id TEXT NOT NULL,
    connection_revision INTEGER NOT NULL,
    effective_config_json TEXT NOT NULL, -- 规范化连接参数 (含 TLS, transport, options)
    config_fingerprint TEXT NOT NULL, -- SHA-256 指纹
    source_entry_id TEXT, -- 来源于哪个 subscription_entry (nullable)
    schema_version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    PRIMARY KEY (node_logical_id, connection_revision),
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE,
    FOREIGN KEY (source_entry_id) REFERENCES subscription_entries(id) ON DELETE SET NULL
);
CREATE INDEX idx_node_conn_ver_fingerprint ON node_connection_versions(config_fingerprint);
```

#### (4) `node_connection_heads` (当前权威连接指针表)
```sql
CREATE TABLE node_connection_heads (
    logical_id TEXT PRIMARY KEY,
    connection_revision INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE,
    FOREIGN KEY (logical_id, connection_revision) REFERENCES node_connection_versions(node_logical_id, connection_revision) ON DELETE RESTRICT
);
```

**单向外键与事务插入契约**:
- 新增节点必须在单个事务中依次写入：
  1. `INSERT INTO nodes (logical_id, ...)`；
  2. `INSERT INTO node_connection_versions (node_logical_id, connection_revision, ...)` (rev=1)；
  3. `INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)` (rev=1)。
- 版本递增在单个事务中依次执行：
  1. `INSERT INTO node_connection_versions (node_logical_id, connection_revision, ...)` (rev=N+1)；
  2. `UPDATE node_connection_heads SET connection_revision = N+1, updated_at = ... WHERE logical_id = ...`。
- 若 `heads` 指向不存在的版本，SQLite 在 `RESTRICT` 约束下直接抛出外键错误，彻底杜绝悬挂指针。下游探针出站与发布编译器**唯一读取 Heads 指针指向的版本**。

#### (5) `node_overrides` (用户显式字段级修改表)
```sql
CREATE TABLE node_overrides (
    node_logical_id TEXT NOT NULL,
    field_path TEXT NOT NULL, -- 如 "transport.sni", "port", "display_name", "transport.skip_cert_verify"
    override_value_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (node_logical_id, field_path),
    FOREIGN KEY (node_logical_id) REFERENCES nodes(logical_id) ON DELETE CASCADE
);
```

#### (6) `publication_payload_refs` (发布快照与 Payload 引用关联表)
```sql
CREATE TABLE publication_payload_refs (
    publication_id TEXT NOT NULL,
    payload_id TEXT NOT NULL,
    PRIMARY KEY (publication_id, payload_id),
    FOREIGN KEY (publication_id) REFERENCES publications(id) ON DELETE CASCADE,
    FOREIGN KEY (payload_id) REFERENCES subscription_payloads(id) ON DELETE RESTRICT
);
CREATE INDEX idx_pub_payload_refs_payload ON publication_payload_refs(payload_id);
```
- **架构澄清**: 引入关联表不是引入第二数据库，而是利用 SQLite 内置外键机制对发布引用的 Payload 实施 `ON DELETE RESTRICT` 强引用保护，严禁仅在 JSON 文本中声明引用而宣称具备外键约束。

---

## 三、 IP Risk 统一测量与 Nullable 1:N 模型 (IP Risk Unification)

### 1. 现流事实 vs 目标拟归一
- **现流事实**:
  - `probe_observations` (000001 + 000007 + 000012 + 000014) 维护轻量级及平台能力拨测，包含 `kind = 'ip_risk'` 通用观测；
  - `000002_ip_risk_schema.sql` 维护 7 张表：`ip_risk_provider_settings`, `ip_risk_observations`, `risk_policy_revisions`, `risk_policy_providers`, `risk_policy_score_bands`, `risk_policy_trait_rules`, `risk_policy_group_bindings`。其中 `ip_risk_observations` 记录各独立服务商（如 ipinfo, spur 等）的细粒度特征与评分。
- **拟归一设计与严格边界**:
  - **严格对应通用 IP 风险探测**: `ip_risk_observations.probe_observation_id` **严格且仅对应 `kind = 'ip_risk'` 的通用探针观测记录**，绝对不是 Baseline 连通性测活的“一对多”（严禁混淆 Baseline 测活与 IP Risk 威胁情报，两者属于正交维度）；
  - **1:N 语义**: 一次针对节点的综合 IP 风险评估（通用 `kind = 'ip_risk'` probe observation）对应多个第三方服务商的详细情报采样（`ip_risk_observations`）；
  - **底层跨表 Kind 约束实现**: 由于标准 SQLite 无法在建表外键中直接施加跨表子查询 CHECK，系统采用**应用层事务校验 (Application Transaction Validation)** 结合标准成熟外键（`probe_observation_id REFERENCES probe_observations(id) ON DELETE SET NULL`）。在写入或更新 `ip_risk_observations` 时，事务显式校验被引用的探针观测其 `kind == 'ip_risk'`，若关联至 `kind = 'baseline'` 或其他类型则直接阻断并回滚，不凭空构造脆弱的触发器平台；
  - **Nullable 1:N 约束**: 历史独立服务商风险查询或离线导入记录，其通用观测外键**允许为 NULL**，绝不凭空伪造虚假的 `probe_runs` 或 `probe_observations`；多个 provider 采样记录完整保全不丢失；
  - **解耦 Baseline**: Baseline 连通性探测与 IP Risk 威胁情报正交解耦，严禁强绑；
  - **真实版本回填**: 历史风险记录仅在历史连接版本明确可知时如实回填，严禁凭节点当前版本伪造历史版本。

### 2. DDL 增量规范与版本摘要
```sql
-- 1. 扩展 probe_observations 元数据与统一测量标识
ALTER TABLE probe_observations ADD COLUMN measurement_id TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_observations ADD COLUMN engine_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_observations ADD COLUMN parser_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_observations ADD COLUMN mapping_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_observations ADD COLUMN stage TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_observations ADD COLUMN error_code TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_observations ADD COLUMN safe_detail_json TEXT NOT NULL DEFAULT '{}';

-- 2. 扩展 ip_risk_observations 支持 Nullable 1:N 关联
ALTER TABLE ip_risk_observations ADD COLUMN probe_observation_id TEXT REFERENCES probe_observations(id) ON DELETE SET NULL;
ALTER TABLE ip_risk_observations ADD COLUMN measurement_id TEXT NOT NULL DEFAULT '';

-- 避免重复采样误删，按作用域、服务商与时间戳施加唯一约束 (Nullable 场景忽略)
CREATE UNIQUE INDEX idx_ip_risk_obs_scope_provider_sample
    ON ip_risk_observations(probe_observation_id, provider, observed_at)
    WHERE probe_observation_id IS NOT NULL;
CREATE INDEX idx_ip_risk_obs_measurement ON ip_risk_observations(measurement_id);

-- 3. 真实利用 000002 已有的 risk_policy_revisions 增加策略规则快照摘要
ALTER TABLE risk_policy_revisions ADD COLUMN rules_digest TEXT NOT NULL DEFAULT '';
```

### 3. 策略版本锁定与读模型一致性
- `risk_policy_revisions` 在修改其子表（`risk_policy_providers`, `risk_policy_score_bands`, `risk_policy_trait_rules`）时，事务性计算子规则标准 JSON 的 SHA-256 并更新 `rules_digest`；
- 发布快照中保存 `risk_policy_snapshot { policy_revision_id, rules_digest, config_digest }`；
- `NodeRiskResolver` 与前端工作台必须读取同一次版本测量，严禁利用内存缓存篡改事实源。

---

## 四、 探针安全详情与 Allowlist 白名单设计 (Safe Detail Allowlist)

为彻底解决历史内核因丢弃 detail 导致根因丢失、以及避免未经脱敏收集引发凭据泄露的问题，系统为 `safe_detail_json` 制定严格的 **Allowlist 白名单契约**：

### 1. 允许收集字段白名单 (Strict Allowlist)
| 字段名 | 类型 | 说明与受控枚举 | 示例 |
|---|---|---|---|
| `stage` | string | 探测生命周期阶段 | `"resolve"`, `"dial"`, `"handshake"`, `"tls"`, `"http_req"`, `"read"` |
| `code` | string | 标准化错误原因代码 | `"timeout"`, `"connection_refused"`, `"unsupported_target_capability"`, `"tls_verification_failed"`, `"protocol_violation"`, `"eof"`, `"unknown"` |
| `target_core_version` | string | 拨测所依赖的底层核心版本 | `"1.19.32"` |
| `protocol` | string | 节点协议名称 | `"vless"`, `"vmess"`, `"shadowsocks"`, `"trojan"` |
| `transport` | string | 传输层网络类型 | `"xhttp"`, `"ws"`, `"tcp"`, `"grpc"` |
| `http_status` | int / null | 上下游 HTTP 状态码 | `403`, `502`, `null` |
| `timeout_ms` | int / null | 设定的超时毫秒阈值 | `5000`, `15000` |
| `reason` | string | 受控原因摘要枚举 | `"contract_matched"`, `"transport_error"`, `"timeout"` |
| `server_redacted` | string | 脱敏后的目标服务器地址 | `"127.0.0.1"`, `"example.com"` (严禁携带密码、Token 或查询串) |

### 2. 严禁收集黑名单 (Default-Deny)
- **严禁直接捕获原生 `err.Error()`**: 原生错误字串常包含携带认证参数的完整 URL；
- **严禁捕获完整 HTTP 响应体**: 响应体可能携带上游敏感上下文；
- **绝对排除凭据信息**: 严禁记录密码、UUID、Token、私钥、用户身份、Authorization 头部、Cookie 头部及 Query 参数中的敏感 Secret。
- **历史数据原则**: 历史丢弃的 detail 严格保留为 `unknown`，绝不虚假伪造；防泄密机制作为诊断防护，**绝不以此为由拒绝正常可用节点**。

---

## 五、 下游协议能力矩阵、Mihomo xhttp 官方对齐与发布生命周期

### 1. VLESS xhttp 参数规范与 Mihomo 官方代码对齐
根据 Mihomo 官方 v1.19.32 源码（GitHub: `MetaCubeX/mihomo`，分支 `v1.19.32`，包 `adapter/outbound` 中 VLESS xhttp 传输实现）：
- **内部 Canonical 模型**: 完整保留上游来源的全部字段与扩展；
- **Mihomo 渲染器白名单**: 严格仅将官方明确支持的参数渲染为 `xhttp-opts`：
  - `path`: string
  - `host`: string
  - `mode`: string (`"auto"`, `"stream-up"`, `"stream-down"`, `"packet-up"`)
  - `headers`: map[string]string
- **未知扩展隔离**: 上游若存在未知 extra 字段，完整保存在内部扩展属性中并输出诊断提示，**严禁盲目传给 Mihomo 渲染器**，防止未经验证的字段导致渲染器异常；
- **构建成功 vs 连通性区分**: 缺少特定 opts 导致构建成功仅为配置结构解析通过，不能断言实网可通；
- **sing-box 适配**: 针对 sing-box 明确返回 `unsupported_target_capability` 诊断。

### 2. Publication 清单结构 (Manifest JSON Schema)
在 `publications` 表中保存具备完整血统的 JSON 清单：
```json
{
  "manifest_version": "1.0.0",
  "publication_id": "pub_01a0ffd9...",
  "status": "draft",
  "target_engine": "mihomo",
  "engine_version": "1.19.32",
  "mapping_version": "1.0.0",
  "mode": "strict",
  "source_inventory_digest": "sha256:4f8a...",
  "configuration_revision_id": "rev_01a0b...",
  "risk_policy_snapshot": {
    "policy_revision_id": "risk_rev_01...",
    "rules_digest": "sha256:7b2c...",
    "config_digest": "sha256:1a2b..."
  },
  "payload_ids": ["payload_01a0b...", "payload_01a0c..."],
  "entry_ids": ["entry_01a0...", "entry_01a1..."],
  "version_refs": [
    { "logical_id": "node_77f0...", "connection_revision": 1 },
    { "logical_id": "node_a8f9...", "connection_revision": 1 }
  ],
  "nodes_manifest": [
    {
      "logical_id": "node_77f0...",
      "connection_revision": 1,
      "display_name": "🇺🇸【美国】线路2 | 1x",
      "protocol": "vless",
      "action": "included"
    },
    {
      "logical_id": "node_a8f9...",
      "connection_revision": 1,
      "display_name": "🇺🇸【美国】线路1 | 1x",
      "protocol": "vless",
      "action": "excluded",
      "reason": "unsupported network xhttp in sing-box",
      "diagnostic_code": "unsupported_target_capability"
    }
  ],
  "compiled_artifact": {
    "content_digest": "sha256:889d...",
    "byte_size": 28416
  }
}
```

### 3. Preview / Publish 两阶段生命周期与 Pin/Prune 机制
1. **Preview 阶段 (生成不可变 Draft 快照)**:
   - 提取指定版本的配置，计算 Manifest 与编译产物，插入 `publications`（`status = 'draft'`）；
   - 在单个事务中向 `publication_payload_refs` 插入该 Draft 引用的全部 `payload_id`；
   - 若遇到不支持节点：
     - **Strict 模式 (默认)**: 返回 HTTP 422 及结构化诊断清单，Draft 标记为 `publishable = 0`，严禁发布；
     - **Compatible 模式**: 仅在请求显式指定时启用，排除不支持节点并记录原因。重新校验策略组：**若任何策略组节点变为 0，拒绝生成并报错 `empty_group_not_allowed`，严禁自动回退到 DIRECT**。
2. **Publish 阶段 (原子激活)**:
   - 显式接收 `snapshot_id`，严格激活该快照的预编译字节，更新 `status = 'active'`；
   - 绝对不重新读取当前动态节点库，消灭时间差库存漂移；重复发布同一 snapshot 保持幂等；
   - 目标运行时版本改变必须请求新的 Preview，绝不隐式重新编译。
3. **Payload Pin 与 Prune 事务**:
   - `publication_payload_refs` 外键 `RESTRICT` 物理阻止删除正在被草稿或激活发布引用的 Payload；
   - 默认保留每个订阅最近 3 次成功抓取的 Payload（Last-Good 绝不被覆盖）；
   - Prune 清理事务：`DELETE FROM subscription_payloads WHERE id NOT IN (SELECT payload_id FROM publication_payload_refs) AND id NOT IN (最近3次成功ID)`；
   - 原始 body 永存不可无界，实施前须在隔离环境进行磁盘容量增长率实测。

### 4. 前端错误卡片治理 (`web/src/ui/ErrorStateCard.vue`)
- **根因**: `lowerMsg.includes('network')` 导致包含 `"unsupported network 'xhttp'"` 的 422 报错被误判为“网络连接异常”；
- **契约**: 优先根据 HTTP 状态码 422 或业务代码 `code === 'unsupported_target_capability'` 进行分类，展示“协议与目标能力不兼容”及具体排查引导，杜绝落入网络断开误判。

---

## 六、 维护窗口、资产对账与受控回滚 (Measured Maintenance & Reconciliation)

### 1. 清除固定时限与 Zero-loss 绝对承诺
- 坚决杜绝“固定 30 秒”与“绝对零丢失”的不实承诺；
- 真实停写维护窗口大小必须由实施阶段在隔离演练中实测（测量 DDL 执行与 1014 个节点版本指针初始化的真实耗时），并在正式割接前获得用户明确确认。

### 2. 在线一致性备份与增量迁移
1. **在线热备**: 使用 SQLite 官方 `.backup` 协议将 `/data/csp-v1.db` 热备至 `/data/backups/csp-v1-pre-migration-000015.db`，校验 `integrity_check: ok` 与 `foreign_key_check: 0`；
2. **增量迁移**:
   - 事务性创建 6 张新表；
   - 为存量 1014 个节点在 `node_connection_versions` 初始化 revision=1，并在 `node_connection_heads` 写入指针；
   - 983 个历史失活节点完整保全，标记为 legacy；
3. **读写链路切换**: 切换读写服务并验证。

### 3. 原资产对账与受控回滚可执行步骤
若切换后发生不可恢复故障：
1. **立即停机**: 停止新版容器；
2. **资产对账**: 读取热备文件校验 1014 节点、29 组、163 规则基准；
3. **回滚路径选择**:
   - 若在停写窗口内或尚未产生新写入：直接原子覆盖回滚数据库文件，切换旧容器镜像启动；
   - 若已产生新业务写入：先执行增量导出脚本提取新产生的数据流水，执行快照回滚，向操作员呈报受影响数据窗口并执行增量数据合并，或由操作员显式确认并接受数据 Loss 窗口。

---

## 七、 扩充 14 场景独立测试工程方案 (Verification Plan)

| 序号 | 验证场景 | 输入与前置条件 | 隔离验证工具与命令 | 核心断言与通过标准 | 边界与预算限制 |
|---|---|---|---|---|---|
| T1 | Heads 复合外键与级联 | 内存 SQLite 初始化核心 DDL | `python3 tests/verify_database_dataflow_design.py` | 1) 顺序插入成功; 2) 指向无效 revision 抛出外键错误; 3) 删除 node 级联删除 versions 与 head | 耗时 < 1s，纯内存沙箱 |
| T2 | 下游消费版本一致性 | 创建节点并生成 2 个连接版本 | `go test ./internal/application/... -run TestDownstreamVersionConsistency` | Probe 适配器与 Compiler 渲染输出读取的 Server/Port/TLS 完全等于 Heads 当前指向版本的有效 JSON | 零生产副作用 |
| T3 | 抓取异常保全资产 | 模拟上游返回 403 Forbidden 与 30s 超时 | `go test ./internal/application/inventory -run TestRefreshFailurePreservesNodes` | 1) `subscription_fetches` 记录失败; 2) 既有 1014 节点、31 活跃节点未被删除或标记失活 | 内存模拟 HTTP |
| T4 | 节点更名与 CoW 分支 | 共享节点更新名称 vs 改变端口凭据 | `go test ./internal/application/inventory -run TestReconcileCoWAndRename` | 1) 纯更名不产生新连接版本; 2) 凭据变更触发单源版本递增或多源 CoW 派生隔离 ID，现有组引用不中断 | 运行态 0 写入 |
| T5 | 前置来源门禁与公告识别 vs 正常私网 | 1) Dogegg已验证来源+全条件命中; 2) 未验证来源即使满足4条件保留unknown; 3) 127.0.0.1:1080 正常私网代理保持proxy; 4) 3公告独立入库与条目级override纠偏 | `python3 tests/verify_database_dataflow_design.py` | 1) 3公告均写入不去重; 2) 命中已验证来源+组合条件归类为 notice; 3) 未经验证来源保留 unknown; 4) 正常私网代理归类为 proxy; 5) 条目级 override 成功纠偏并记录 anchor 与 actor_ref | 静态逻辑断言 |
| T6 | xhttp 参数规范与导出 | 包含 path, host, mode, headers 及未知 extra 的 VLESS 节点 | `go test ./internal/compiler -run TestMihomoXHTTPExport` | 1) Mihomo 仅渲染官方支持的 4 个字段，未知 extra 保留于扩展且不盲传; 2) sing-box 目标严格返回 `unsupported_target_capability` | 内存编译，零外部网络 |
| T7 | IP Risk Nullable 1:N 统一测量与错误 kind 拦截 | 1 个通用探测 Run 与 2 个服务商风险详情，以及非 ip_risk 关联尝试 | `python3 tests/verify_database_dataflow_design.py` | 1) 两个 provider 详情外键指向同一 kind='ip_risk' probe_observation_id; 2) 尝试关联 kind='baseline' 被应用事务校验阻断; 3) 历史独立风险记录 probe_observation_id 为 NULL 成功入库且 provider 多记录不丢失; 4) rules_digest 快照一致 | 内存 SQLite |
| T8 | 发布 Manifest 与 Payload Pin/Prune | 生成 Draft 并记录 `publication_payload_refs`，执行 Prune | `python3 tests/verify_database_dataflow_design.py` | 1) 尝试删除被发布的 Payload 抛出外键 RESTRICT 错误; 2) 针对未引用的旧 Payload 清理成功; 3) 激活发布幂等 | 内存 SQLite |
| T9 | 空组防护与 DIRECT 拦截 | 过滤导致策略组有效节点为 0 | `go test ./internal/compiler -run TestEmptyGroupRejection` | 编译器抛出 `empty_group_not_allowed` 错误，绝不自动插入 `DIRECT` 节点 | 静态检查 |
| T10 | 前端 422 错误卡片呈现 | 模拟后端返回 422 `unsupported_target_capability` | `npm run test` (Vitest 测试 `web/src/ui/ErrorStateCard.test.ts`) | 卡片渲染为“协议能力不支持”，绝不包含“网络连接异常”文本 | 前端单测 |
| T11 | Safe detail JSON allowlist 与脱敏 | 包含合法字段与非法敏感凭据输入 | `python3 tests/verify_database_dataflow_design.py` | 1) 白名单字段完整保留; 2) raw err.Error() 与 password/UUID 强力剔除; 3) 历史未知详情保持 unknown | 纯函数断言 |
| T12 | 代码门禁与编译检查 | 本地工作区全量代码库 | `go build ./...` 与 `cd web && npm run type-check` | 静态编译与类型检查退出码必须严格为 0 | 本地执行 |
| T13 | 真实上游私密回放 vs Fixture | 使用 `/tmp/csp-evidence-20261003/dogegg_raw.txt` 与公开脱敏 YAML | `go test ./internal/parser -run TestDogeggRawExtraction` | 真实回放确证 18 条目（3 公告 + 15 线路），敏感凭据不入 Git 仓库 | 权限隔离 0600 |
| T14 | 停写窗口演练与回滚对账 | 隔离环境执行 DDL 与数据初始化演练 | 实施阶段专用测试脚本 | 实测停写毫秒数并输出报告；校验回滚原子性与增量对账逻辑 | 实施前隔离验证 |

---

## 八、 关键事实澄清与未定项说明

1. **客观无法恢复的历史未知项 (Permanently Unrecoverable History)**:
   - **旧 Run 失败底层原生报错**: 旧观测记录的底层细节因历史内核未捕获已永久丢失，无法回溯，当前未知真实失败根因，绝不凭空伪造；
   - **真实上游面板软件品牌**: Dogegg 虽然下发固定格式公告条目，但具体后端面板品牌未知，契约建立在经过验证的来源语义证据与规则版本上，不假设面板品牌；
2. **实施阶段完全可测项 (Measurable During Implementation)**:
   - **Payload 磁盘保留天数预算**: 原始 body 长期保留天数需实施前在隔离环境根据实测磁盘写入增长率最终确定；
   - **停写维护短窗真实时长**: 真实停写维护窗口时长需在实施阶段隔离演练实测确定，不假设固定 30 秒，并在上线前由用户确认。

---

## 九、 冻结施工写集合与可并行 API 契约 (Frozen Write Sets & Parallel API Specifications)

### 1. 完整特性包后端写集合 (Backend Write Set)
- `migrations/000015_dataflow_facts_and_versioning.sql`: 6 张新表创建、`ip_risk_observations` 与 `risk_policy_revisions` 列扩充、存量 1014 节点初始版本与 Heads 事务插入；
- `internal/repository/sqlite/probes.go`: 清除运行时硬编码 `CREATE INDEX` 与 `ALTER TABLE`；
- `internal/repository/sqlite/dataflow.go`: 新增 Payload、Entry、Versions、Heads、Overrides 与 PublicationPayloadRefs 的 SQL 仓储实现；
- `internal/application/inventory/service.go`: 摄入链路集成 Payload 原始保存、公告前置门禁分类、版本指纹比对、Overrides 合并与 CoW 分支隔离；
- `internal/compiler/mihomo.go`: 解封 VLESS `network: xhttp`，精确映射 `path`, `host`, `mode`, `headers` 为 `xhttp-opts`，未知 extra 隔离；
- `internal/compiler/singbox.go`: 对 `xhttp` 传输返回明确 `unsupported_target_capability` (422)；
- `internal/probe/mihomo/adapter.go`: 探针出站支持 xhttp 同配置拨号；
- `internal/application/publication/service.go`: 实现 Preview 不可变 Draft 生成与 Manifest 计算，Publish 接收 `snapshot_id` 原子发布，空组拦截；
- `internal/transport/http/`: 注册条目纠偏、节点覆盖与发布快照相关 HTTP 处理。

### 2. 完整特性包前端写集合 (Frontend Write Set)
- `web/src/ui/ErrorStateCard.vue`: 调整错误分类优先级，优先处理 HTTP 422 与结构化 `code`，杜绝因协议配置包含 "network" 关键词误判为网络失联；
- `web/src/views/PublishView.vue` (或发布相关页面): 支持 Preview 结果携带的 `snapshot_id` 直接提交 Publish，杜绝时间差库存漂移。

### 3. 冻结可并行 API 契约

#### (1) 条目纠偏 API: `POST /api/v1/subscriptions/{id}/entries/{entry_id}/override`
```json
// Request Body
{
  "user_kind_override": "proxy", // "proxy" | "notice" | "unknown"
  "override_anchor": "dogegg_entry_0",
  "reason": "False positive notice classification confirmed by admin"
}

// Response Body (200 OK)
{
  "code": "success",
  "data": {
    "entry_id": "entry_01923456-789a-7b12-8901-23456789abcd",
    "subscription_id": "sub_dogegg_01",
    "entry_kind": "notice",
    "user_kind_override": "proxy",
    "override_anchor": "dogegg_entry_0",
    "override_at": "2026-10-03T13:30:00Z",
    "actor_ref": "admin"
  }
}
```

#### (2) 节点连接修改/覆盖 API: `PATCH /api/v1/nodes/{logical_id}/connection`
```json
// Request Body (沿用既有 NodePatchRequest 结构，支持字段级人工覆盖)
{
  "server": "edge-hk.example.com",
  "port": 443,
  "transport": {
    "network": "xhttp",
    "sni": "edge-hk.example.com"
  }
}

// Response Body (200 OK)
{
  "code": "success",
  "data": {
    "node": {
      "logical_id": "vless_edge_hk_443",
      "display_name": "HK-Node-01",
      "protocol": "vless",
      "server": "edge-hk.example.com",
      "port": 443,
      "connection_revision": 2,
      "config_fingerprint": "sha256:abc123def...",
      "active": true
    },
    "sources": [
      { "subscription_id": "sub_01", "last_seen_fetch_id": "fetch_01" }
    ],
    "overrides": [
      { "field_path": "transport.sni", "override_value": "edge-hk.example.com", "updated_at": "2026-10-03T13:30:00Z" }
    ]
  }
}
```

#### (3) 发布预览与快照 API: `POST /api/v1/publications/preview`
```json
// Request Body
{
  "target": "mihomo", // "mihomo" | "singbox" | "surge" | "qx"
  "revision_id": "rev_01",
  "compat_mode": "strict" // "strict" (默认) | "compatible"
}

// Response Body (200 OK)
{
  "code": "success",
  "data": {
    "snapshot_id": "draft_snap_01923456-789a-7b12-8901-23456789ef01",
    "target": "mihomo",
    "snapshot_digest": "sha256:fedcba987...",
    "content_digest": "sha256:123456789...",
    "content_type": "text/yaml; charset=utf-8",
    "filename": "clash.yaml",
    "content": "proxies:\n  - name: HK-01\n    type: vless\n    xhttp-opts:\n      path: /x\n      host: edge.com\n",
    "manifest": {
      "rules_digest": "sha256:778899aa...",
      "node_count": 31,
      "excluded_count": 0,
      "payload_ids": ["payload_01923456-789a-7b12-8901-23456789abcd"],
      "version_refs": [
        { "logical_id": "vless_edge_hk_443", "connection_revision": 2 }
      ]
    },
    "diagnostics": []
  }
}
```

#### (4) 发布激活 API: `POST /api/v1/publications`
```json
// Request Body (两阶段生命周期: 接收 snapshot_id 直接激活不可变快照，杜绝漂移)
{
  "target": "mihomo",
  "snapshot_id": "draft_snap_01923456-789a-7b12-8901-23456789ef01"
}

// Response Body (201 Created)
{
  "code": "success",
  "data": {
    "id": "pub_01923456-789a-7b12-8901-23456789aaaa",
    "snapshot_id": "draft_snap_01923456-789a-7b12-8901-23456789ef01",
    "target": "mihomo",
    "content_digest": "sha256:123456789...",
    "created_at": "2026-10-03T13:35:00Z"
  }
}
```

#### (5) 统一结构化错误响应契约 (422 / 409 / 401 / 403)
```json
// 422 Unprocessable Entity - 协议能力不支持 (如 sing-box 请求 xhttp)
{
  "code": "unsupported_target_capability",
  "message": "sing-box does not support xhttp transport protocol",
  "request_id": "req_01923456-789a...",
  "target": "singbox",
  "location": "nodes[2].transport.network",
  "feature": "xhttp"
}

// 409 Conflict - 发布空组熔断
{
  "code": "publication_preflight_rejected",
  "message": "Policy group 'Proxy' contains 0 available nodes; fallback to DIRECT is forbidden",
  "request_id": "req_01923456-789b...",
  "diagnostics": [
    { "code": "empty_group_not_allowed", "feature": "Proxy", "message": "Group has no members" }
  ]
}
```

### 4. 生产环境操作 Runbook 与凭据渠道评估
- **SQLite 容器内热备**: 通过 `docker exec clash-sub-parser sqlite3 /data/csp-v1.db ".backup '/data/backups/csp-v1-backup-$(date +%Y%m%d_%H%M%S).db'"` 保持一致性快照，实测完整性与外键退出码为 0；
- **宿主挂载卷直读隔离演练**: 挂载路径 `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db` 支持零写拷贝到隔离测试沙箱（如 `/tmp/isolated_csp_test.db`）演练迁移与对账；
- **生产管理凭据现状与缺口说明**:
  - `/healthz` 与 `/readyz` 保持无鉴权开放探针；
  - 生产主库中 `settings.admin_token` 存储 60 位 bcrypt 密文散列，无明文外泄；
  - 容器环境变量未直接挂载明文 `CSP_ADMIN_TOKEN`；
  - 前台 Web 界面使用 `csp_session` cookie，当前会话在非交互子机中不直接暴露明文 secret；
  - **缺口呈报**: 若需执行全实网生产接口探测拨测（如生产 `POST /api/v1/probes/run`），需通过用户合法授权注入 Bearer token；但该缺口**不影响也不阻断**本地隔离迁移演练、DDL 执行、全量单元测试与镜像构建。

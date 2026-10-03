# Design: redesign-csp-database-dataflow

- **Change**: `redesign-csp-database-dataflow`
- **Work Package**: `pkg_database_dataflow_design`
- **Lineage**: `fleet-contract-v3 / redesign-csp-database-dataflow`
- **Architectural Scope**: CSP 数据库架构重构、上下游契约厘清与事实层补齐 (Rev 3 - Remediated)

---

## 1. 架构总原则与单库运行定式 (Architecture Baseline)

系统严格维持单一主 SQLite 数据库（`/data/csp-v1.db`），开启 WAL 日志模式、普通同步模式（`synchronous = NORMAL`）与内置外键检查（`PRAGMA foreign_keys = ON`）。单进程内通过单写者连接池与 5000ms 忙等待保证事务串行化，坚决不进行多库分拆或引入外部中间件。

存量的 27 张数据表（1 张技术表 `schema_migrations` 与 26 张业务表，包含 7 张 IP Risk 相关表）全量保全，涵盖 1014 个节点（31 活跃、983 历史失活禁用）、9 个订阅与所有策略拓扑规则。所有 Schema 变更统一归口于 `MigrationRunner`，彻底清除 Repository 构造函数中的硬编码运行时 DDL。

停写维护窗口大小由隔离演练实测确定并由用户确认，清除固定 30 秒与 Zero-Loss 的不实承诺。提供原资产对账与回滚增量合并或操作员确认 Loss 窗口的可执行步骤。

---

## 2. 目标数据模型与增量事实层 (Target ER Schema)

针对 SQLite 不支持 `ALTER TABLE ADD` 复合外键且 `nodes` 与 `versions` 容易产生双向循环外键的问题，系统确立单向解耦模型：**引入核心事实表 `node_connection_heads` 与 `publication_payload_refs`**。
- `nodes` 表保持稳定资产标识，**不添加表级外键反向引用 versions**（避免循环依赖，无需重建 `nodes` 表）；
- `node_connection_versions` 单向外键引用 `nodes(logical_id)`；
- `node_connection_heads` 单向外键引用 `nodes(logical_id)` 并通过复合外键单向引用 `node_connection_versions(node_logical_id, connection_revision)`；
- Canonical current revision pointer 唯一存在于 `node_connection_heads.connection_revision`；
- 旧 `nodes.connection_revision` 字段在过渡期仅作为只读投影视图，完成切换后严禁多事实源双写；
- 引入 `publication_payload_refs` 关联表（`publication_id`, `payload_id`），外键 `RESTRICT` 物理约束被发布引用的 Payload 不被误删。

```
+--------------------------+       1:1       +--------------------------+
|  subscriptions           |---------------->|  subscription_payloads   | (Authoritative Raw Upstream)
+--------------------------+                 +--------------------------+
             |                                             | 1:N
             | 1:N                                         v
+--------------------------+                 +--------------------------+
|  subscription_fetches    |                 |  subscription_entries    | (Derived Entries + Notice Filter)
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

### 核心新增与扩展表规范
1. **`subscription_payloads`**: 存储完整原始上游响应（BLOB，支持 gzip），排除 Cookie/Auth，内部仅供重放和对账，不向外网暴露明文凭据；
2. **`subscription_entries`**: 存储基于 parser_version 派生的结构化条目。
   - `id` 为每 Payload 独立 PK，主键加 `UNIQUE(payload_id, ordinal)`；
   - 3 个同参数公告独立写入不去重，不把连接指纹当条目身份；
   - `source_key` 仅作为跨刷新匹配候选锚点，**严禁 UNIQUE(subscription_id, source_key) 折叠条目**；
   - 内置严格来源门禁与组合全条件公告分类器：遵循 `verified_source_rule(subscription_id, source_evidence, rule_version) AND exact_rule_match(entry) => notice`，仅对已验证来源（如 Dogegg）启用规则，未验证来源即使满足回环+端口1+全零UUID+提示词四条件亦保留为 `unknown` 候选并支持纠偏；正常可解析代理始终为 `proxy`；
   - 支持条目级用户人工纠偏（`user_kind_override`、`override_anchor`、原因、时间戳、`actor_ref`），即使无对应 node 亦可修正；
3. **`node_connection_versions`**: 记录不可变的节点有效配置 `effective_config_json` 与指纹；
4. **`node_connection_heads`**: 记录节点当前生效的连接版本，单向外键约束指向版本表，杜绝悬挂指针与循环外键；
5. **`node_overrides`**: 记录用户在 Web 控制台的字段级人工修改，刷新时与源条目参数明确合并，保证人工配置不被抓取覆盖；
6. **`publication_payload_refs`**: 发布快照引用 Payload 的关联表，`ON DELETE RESTRICT` 约束 Payload 物理防删。

---

## 3. IP Risk Nullable 1:N 统一测量与发布两阶段生命周期

1. **IP Risk Nullable 1:N 统一测量模型**:
   - `probe_observations` 中严格仅 `kind = 'ip_risk'` 的通用观测记录与 `ip_risk_observations` (第三方情报商详细评分) 构成 1:N 关系，绝非与 Baseline 测活混用或一对多；
   - 跨表 kind 约束采用应用层事务校验结合标准外键 `probe_observation_id REFERENCES probe_observations(id) ON DELETE SET NULL`，关联非 ip_risk 类型直接阻断回滚，不自造触发器平台；
   - 历史独立服务商风险查询允许 `probe_observation_id = NULL`，不凭空构造虚假 `probe_runs`，多 provider 数据保全不丢失；
   - Baseline 连通性探测与 IP 风险情报解耦；历史风险记录仅在连接版本明确已知时如实回填；
   - 真实利用 `000002_ip_risk_schema.sql` 中的 `risk_policy_revisions` 增加 `rules_digest TEXT NOT NULL DEFAULT ''`，发布清单中固化 `risk_policy_snapshot`。
2. **Safe Detail Allowlist 白名单机制**:
   - `probe_observations.safe_detail_json` 仅允许收集 `stage`, `code`, `target_core_version`, `protocol`, `transport`, `http_status`, `timeout_ms`, `reason`, `server_redacted`；
   - 严禁捕获原生 `err.Error()`、完整 HTTP 响应体及任何密码、UUID、Token、私钥等凭据。历史遗留缺失 detail 保留为 `unknown`。
3. **Publication 清单与两阶段生命周期**:
   - Preview 阶段创建不可变 draft 记录并计算完整清单（Manifest JSON，含各节点 included/excluded 理由、payload_ids、version_refs、产物哈希），关联 `publication_payload_refs`；
   - Publish 阶段显式接收 `snapshot_id` 并原子发布已编译字节，绝不重新读取当前动态节点库，消灭时间差库存漂移；
   - 严格模式完整报错，显式兼容模式记录清单后安全过滤，空组拒绝自动回退至 DIRECT。
4. **协议矩阵与前端错误治理**:
   - 探针出站拨号与发布编译统一消费 `node_connection_versions.effective_config_json`；
   - Mihomo (v1.19.32) 目标仅渲染官方支持的 `path`, `host`, `mode`, `headers` 为 `xhttp-opts`，未知 extra 保留在内部扩展并不盲传；sing-box 目标严格返回 `unsupported_target_capability`；
   - 修复 `web/src/ui/ErrorStateCard.vue`，杜绝将 422 能力错误误报为网络失联。

---

## 4. 迁移演练、回滚与测试验证 (Migration & Testing)

- **迁移演练**: 在隔离环境中演练 Migration 000015，实测停写耗时并获用户确认；执行 SQLite 在线一致性热备后运行迁移，事务性初始化 1014 个节点的版本与 head 指针，保全历史失活资产。
- **受控回滚**: 准备热备文件与旧镜像，若异常发生可在短窗内原子恢复；切换后产生新数据则导出增量由操作员确认补偿。
- **测试工程方案**: 扩充至 14 个独立验证场景（详见 `docs/design/csp-database-dataflow.md` 第七章），全流程基于项目现有测试工具验证，杜绝私造框架。

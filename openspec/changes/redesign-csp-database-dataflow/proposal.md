## Why

当前 CSP 业务系统在数据流与数据库设计上存在核心架构痛点：
1. **上游原始事实丢失**：订阅抓取后未持久化原始未经篡改的 payload，解析失败或上游偶发异常（如 403 / 超时）难以回放对账，且容易误覆盖存量健康的节点资产；
2. **公告占位节点污染代理池与探针**：上游订阅面板（如 Dogegg）下发以 `127.0.0.1:1`、测试占位 UUID 和流量/到期提示为内容的通知条目（3 个同连接参数但不同内容的公告），系统将其误作为真实代理节点入库，导致探针周期性尝试拨测并持续报错，同时污染对外导出订阅；
3. **节点身份与连接版本纠缠**：节点逻辑身份与传输参数未解耦，多源共享时参数变动易引发覆盖冲突或旧历史断链，用户在 Web 端所做的字段级修改在下次抓取刷新时容易被静默覆盖；SQLite 不支持通过 `ALTER TABLE` 增加复合外键，若直接在 `nodes` 与 `versions` 间双向引用将引发循环依赖与全表重建风险；
4. **IP Risk 领域双轨分离与关联简化缺陷**：通用探测观测流（`kind = 'ip_risk'`）与详细服务商风险评估（7 张相关业务表）缺少统一测量标识关联，且上轮设计简单化为 1:1 强绑，忽略了 1:N 细粒度服务商采样及历史独立记录 Nullable FK 的客观事实；
5. **下游编译器硬编码封禁引发 422 熔断**：虽然依赖的 Mihomo 核心支持现代传输协议（如 VLESS `xhttp`），但 CSP 编译器白名单硬编码将其拦截报错，导致整个订阅无法导出持久化（`publications` 表为 0 行），前端 `web/src/ui/ErrorStateCard.vue` 还因文本匹配将协议不支持错误误报为“网络连接异常”。

用户明确要求重新设计 CSP 各个数据库与上下游契约，保持单 SQLite 主库与存量数据资产，厘清读写所有权，补齐事实层，实现确定性上下游一致流转。

## What Changes

- **单 SQLite 主库、WAL 模式与领域所有权划分**：保持 `/data/csp-v1.db` 单主库架构，存量 27 张表（1 技术表 + 26 业务表，含 7 张 IP Risk 相关表）全量保全，涵盖 1014 个节点（31 活跃、983 历史失活禁用）、9 订阅与所有策略规则。所有 Schema 变更统一归口于 `MigrationRunner`，移除 Repository 构造函数中的运行时 DDL。停写维护窗口由隔离演练实测确定，提供资产对账与回滚补偿步骤。
- **构建最少必要上游事实层**：
  - 引入 `subscription_payloads` 记录每次拉取的原始响应 BLOB（加密凭据内部重放，不向外网暴露），建立受控生命周期保留策略，抓取失败绝对不抹除既有健康库存；
  - 引入 `subscription_entries` 解耦派生条目，每 Payload 独立 PK，同参数多公告全部独立记录不去重；实现前置来源门禁分类器（`verified_source_rule(subscription_id, source_evidence, rule_version) AND exact_rule_match(entry) => notice`），仅对已验证来源（如 Dogegg）启用规则，未验证来源即使满足回环+端口1+全零UUID+提示词四条件亦保留为 `unknown` 候选并支持纠偏；正常可解析代理始终为 `proxy`；支持条目级用户人工纠偏（`user_kind_override`、`override_anchor`、原因、时间戳、`actor_ref`），并通过 `source_key` 锚点跨刷新重放。
- **建立不可变节点连接版本、权威指针与用户覆盖机制**：
  - 保持稳定 `logical_id` 资产标识；
  - 引入 `node_connection_versions` 记录不可变有效参数（`effective_config_json` 与指纹），参数变更递增 `connection_revision`；
  - 引入 `node_connection_heads` 作为权威当前版本指针，单向复合外键约束，避免与 `nodes` 双向循环外键，无需重建 `nodes` 表；
  - 引入 `node_overrides` 持久化用户字段级人工修改，刷新时与源参数明确合并，解决多源冲突与被覆盖问题。
- **统一 IP Risk Nullable 1:N 测量关联与规则快照**：
  - 建立 1:N 模型：通用测活记录（严格仅限 `kind = 'ip_risk'`）与多个细粒度服务商评分通过 `probe_observation_id` (Nullable) 与 `measurement_id` 关联，绝不与 Baseline 测活混淆；
  - 跨表 kind 约束采用应用层事务校验结合标准外键，关联非 ip_risk 类型直接阻断回滚；
  - 历史独立服务商记录保持 NULL 外键，不伪造探针运行；Baseline 与风险详情解耦；
  - 利用现有 `risk_policy_revisions` 扩展 `rules_digest` 快照锁定策略版本；
  - 规范 `safe_detail_json` Allowlist 白名单，严禁记录原生 `err.Error()` 或密码凭据。
- **打通下游统一连接消费模型、Mihomo xhttp 官方对齐与发布生命周期**：
  - 探针出站与发布编译器共用同一份 `effective_config_json`；
  - Mihomo 编译器仅将官方支持的 `path`, `host`, `mode`, `headers` 渲染为 `xhttp-opts`，未知 extra 保留于扩展且不盲传；对不支持 `xhttp` 的目标核心（如 sing-box）严格报错或兼容排除；
  - 引入 `publication_payload_refs` 关联表，物理外键 `RESTRICT` 保护被引用的 Payload 防删，制定清晰的 Prune 清理事务；
  - 实现 Preview（生成不可变 Draft 快照）与 Publish（直接发布指定 Snapshot ID，杜绝时间差库存漂移）两阶段生命周期；
  - 支持严格模式与显式兼容模式导出，具备空组安全拦截机制（绝不自动回退至 DIRECT）；
  - 修复前端 `web/src/ui/ErrorStateCard.vue`，禁止将 422 协议能力不支持误判为后端网络中断。

## Capabilities

### New Capabilities
- `dataflow/single-sqlite-domain-partition`: 保持单 SQLite 主库与 WAL 架构，按 Ingestion、Inventory、Policy Topology、Probes、Publications、Settings/Audit 明确表所有者划分，收敛 DDL 至 MigrationRunner，彻底杜绝运行时 Repo 构造函数中的重复 DDL。
- `dataflow/upstream-payload-entry-fact-layer`: 增加 `subscription_payloads` 与 `subscription_entries` 事实层，将权威原始未篡改响应（内部受控重放）与派生条目分层，实现公告伪节点与真实代理节点的确定性分离，支持 `source_key` 稳定重放与用户人工纠偏。
- `dataflow/node-connection-versioning-cow`: 引入 `node_connection_versions`、`node_connection_heads` 与 `node_overrides`，维持稳定 `logical_id` 与 1014 个存量资产，参数变更自动递增版本，用户人工修改持久化合并，多源更新写时复制 (CoW) 隔离，通过单向 Heads 外键消除循环依赖。
- `dataflow/downstream-matrix-and-publication`: 探针与编译器共用版本化连接模型，对齐 Mihomo xhttp 官方支持，统一 IP Risk Nullable 1:N 测量流与 safe_detail 白名单，严格/兼容双模式导出与清单校验，引入 publication_payload_refs 外键防删，修复前端 `web/src/ui/ErrorStateCard.vue` 422 误判为网络失联。

### Modified Capabilities

## Impact

- **核心架构**：维持单 SQLite 物理文件架构不变，通过 Migration 000015 增量新增 6 张事实与关联表，零破坏性变更。
- **内部包演进**：
  - `internal/application/inventory/`: 增加 Payload 持久化与 Entry 分类逻辑，适配 Versioning、Heads 指针与 Overrides；
  - `internal/compiler/`: 增加 `xhttp` 传输支持及发布清单生成，未知 extra 安全隔离；
  - `internal/probe/mihomo/`: 补齐 `xhttp` 出站拨号映射；
  - `internal/application/probe/`: 统一 IP Risk Nullable 1:N 测量标识与 safe_detail 白名单；
  - `web/src/ui/`: 修复错误分类逻辑。
- **数据迁移与回滚**：全量保留既有 1014 节点、29 组、163 规则、1 调度与历史探针数据，支持在线热备、实测维护窗口与原资产对账回滚。

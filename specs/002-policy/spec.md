---
feature: 002-policy
title: Policy Capability Specification (Unified Member Sources Rebuild)
status: READY
schema: speckit-native
---

# Policy Capability Specification

## Current delivery amendment (2026-10-15)

This section governs the current delivery; superseded prose below remains historical evidence, not a second authority. Current run: `pi_run_20261015_230722_1113226`, change: `change_pi_run_20261015_230722_1113226`, package: `whole_task`, epoch 1. Feature ownership stays `specs/002-policy`; related domain baselines are 001-inventory, 003-probe, 004-publication, 005-auth, 006-ui. Previous run outcomes are not current acceptance.

- **FR-DEL-01 (P1 subscription recovery)**: Diagnose every enabled production subscription by actual fetch/HTTP/parse stage; do not infer global failure or a proxy-only cause. Preserve stable identities, source/history associations on failed/partial refresh, and immutable publications. Report genuine upstream failures separately from application defects; do not fabricate successful refreshes or weaken TLS/SSRF to obtain them.
- **FR-DEL-02 (P1 settings proxy)**: Settings SHALL expose persistent subscription-fetch routing: `environment` (inherit startup configuration), `direct` (explicit bypass), `proxy` (explicit HTTP/HTTPS proxy). Default after migration is environment. Saving affects the next manual AND scheduled refresh without restart and survives restart. Existing subscription-specific override takes precedence. Each refresh captures a coherent configuration snapshot. Explicit configured proxy endpoint trust must never exempt subscription or redirect destinations from public-IP validation/pinning. Invalid saves are atomic and preserve the effective setting. Management-only GET/audit/errors must not reveal proxy credentials.
- **FR-DEL-03 (P1 diagnosability)**: Redaction SHALL hide secrets without consuming safe trailing causes. Stable sanitized error stages/codes must distinguish proxy connection, DNS, TLS, HTTP, parse, empty and partial outcomes where observed, including wrapped errors. Secrets in userinfo, query parameters, assignments and URL errors remain protected.
- **FR-DEL-04 (P1 single authority)**: Ordered sources are the sole persistent membership authority after one-time migration. No ongoing sources/edges double-write, empty-source legacy fallback, or silent loss of remove/regex/mixed order. Existing supported business actions are migrated to sources or explicitly rejected when obsolete; previews, revisions/rollback, delete and exports remain coherent. Historical HR-143 is retained for migration equivalence ONLY, not a new group-global filter. Conditions are peer source operations; sequential add/remove permits re-entry.
- **FR-DEL-05 (P1 reproducible release)**: Current source, embedded assets, local gates, independent review, applicable browser/visual acceptance, committed Git SHA, cloud checks/build/publish, image digest/OCI revision and deployed identity must form an auditable chain. Preserve remote changes and local dirty work; no forced push. Reuse existing cloud-delivery mechanism. A skipped publication job or an old successful image cannot satisfy this requirement. A package requiring private visibility must pass genuine private/anonymous-access gates before publication/deployment; do not silently publish publicly or bypass missing API authorization.
- **FR-DEL-06 (P1 production acceptance)**: Back up SQLite online with integrity/hash, record preimages/rollback point before authorized production writes. Use identifiable test objects, restore all temporary setting/object changes, preserve user assets. Test confirmed core user-visible inventory/policy/probe/publication/auth/UI actions against the actual deployment, with real business assertions and explicit pending/failed/not-applicable status. Existing coverage register is a starting inventory, not a completion score. Latest instruction removes phone testing from required scope and forbids connecting to or modifying the phone. Four browser viewports still apply; never call them real-phone evidence.

Acceptance examples: setting direct then proxy changes the actual next fetch route; invalid proxy preserves prior setting; reset to environment restores fallback; a credentialed proxy is masked on GET and causes a real proxy request without leaking; wrong route/upstream failure retains original assets; error suffix remains actionable; source add/remove/add and group_ref/group_nodes round-trip, draft no-write/conflict, over-250-node/150-group integration, and target-specific export validation pass on the final source.

Production deployment is authorized by the current complete-delivery instruction after applicable gates. This does not authorize ambiguous historical group restoration, package visibility mutation without the necessary authenticated owner capability, authentication changes, or unrelated services/framework work. The old literal `去掉(...)` has no evidenced exclusion DSL; its intended reinterpretation is unresolved and MUST NOT be invented. Copy-only historical proposals remain possible; no ambiguous permanent rewrite. Optional unsupported product enhancements are not mandatory new scope.


> **治理状态**: `READY` (All Requirements, Contracts & Packages Frozen; Sequential Add/Remove Pipeline Confirmed)
> **说明**: 本规范为策略组子系统彻底重构（Rebuild）的最终冻结规格。历史运行态曾绑定 `pi_run_20261011_190715_2256208`；当前身份见上方交付修订。用户已确认『组级条件与regex同级、按顺序加减』流水线（候选 A 顺序重入生效；候选 B 全局差集彻底废除；历史 B1/B2 彻底废除；A/B pending 消除）。
> **独立子包就绪状态**: 用户确认的库存拉取源保留与纯节点导出空组解耦修复包已冻结并具备独立契约 (`specs/002-policy/confirmed-fixes-contract.md`)，状态为 `READY`，对齐 `specs/001-inventory` (FR-INV-02, 07) 与 `specs/004-publication` (FR-PUB-02, 03)。

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的策略组与图解析器核心能力。系统必须支持灵活的代理分流拓扑，废除分裂的 `group_edges` 与 `group_node_filters` 双重真源缺陷，建立唯一权威的有序成员来源操作模型（`member_sources`，支持 `add` 与 `exclude` 增减操作，涵盖 `node`、`group_ref`、`group_nodes`、`matching` 与 `builtin` 类别）。系统废除额外全局套壳业务 filter，来源条件仅影响自身候选；严格保证成员顺序的交错保持、首次出现去重、解包递归叶节点展开、双向环路阻断、单事务原子更新与乐观并发控制；并在策略组为空时提供受控的 PASS 兜底机制与防越权隔离。

## User Scenarios & Priority

### User Story 1 - 统一有序成员来源增减混排编排 (Priority: P1)
- **GIVEN** 管理员在策略组编辑工作台中配置策略组成员
- **WHEN** 管理员按顺序混合编排了 `[+group_nodes(HK), -matching(regex:.*01.*), +node(HK-01-Backup)]`
- **THEN** 系统以单一权威有序列表 `member_sources` 持久化，图解析器严格按照编排顺序全序求值并执行首次出现去重，严禁代码硬编码“节点在前、子组在后”的分块破坏顺序。

### User Story 2 - 子组解包展开叶节点与同级条件匹配 (Priority: P1)
- **GIVEN** 管理员创建了包含子节点 N_A 与 N_B 的地区子组 G_sub，并在 G_sub 内部配置了地区匹配来源
- **WHEN** 顶级策略组 G_top 以解包模式（`group_nodes`）引用 G_sub，且该解包项自身配置了延迟或可用性匹配条件
- **THEN** 解析器首先对 G_sub 按其自身内部来源操作求值产出真实叶节点，随后平铺展开至 G_top，并由该项自带的匹配条件对展开叶节点执行过滤，最终输出真实节点列表，不包含 G_sub 组名。

### User Story 3 - 循环拓扑全方位 DAG 检测阻断 (Priority: P1)
- **GIVEN** 管理员配置策略组拓扑
- **WHEN** 产生 `group_ref` 直接成环（如 A -> B -> A）或 `group_nodes` 展开成环（如 A 展开 B，B 展开 A）
- **THEN** 系统在保存与发布预览时均立即阻断，返回 HTTP 422 `cycle_detected` 并输出精准的可视化环路路径（Cycle Path）。

### User Story 4 - 全池动态条件无截断匹配 (Priority: P1)
- **GIVEN** 系统导入了超过 200 个有效节点
- **WHEN** 管理员配置了一个按国家代码或正则匹配的动态来源项
- **THEN** 图解析器全量遍历所有活动节点，返回所有匹配项，严禁施加 50 节点的隐藏硬分页截断。

### User Story 5 - 单事务原子替换与乐观版本控制 (Priority: P2)
- **GIVEN** 管理员在 Web 端编辑并保存策略组
- **WHEN** 提交包含元数据、全量 `member_sources` 与可选约束的请求
- **THEN** 服务端在单数据库事务中原子执行校验与全量替换；若提交的 `expected_revision` 与数据库当前版本不一致，服务端返回 HTTP 409 `revision_conflict`，杜绝脏写。

### User Story 6 - 空策略组安全兜底与纯节点导出解耦 (Priority: P2)
- **GIVEN** 某个策略组由于筛选条件过严导致成员解析为 0
- **WHEN** 导出为 Mihomo 配置且启用了 `empty_fallback_pass` 时输出 `proxies: [PASS]`；未启用时输出 `REJECT`；当导出为纯节点目标（SingBox, Surge, QX）时，空策略组不阻断纯节点配置生成。

---

## Requirements

### 1. 统一有序来源与成员解析引擎

#### Requirement: 唯一权威有序来源增减操作模型 [FR-POL-01] <a id="req-pol-unified-sources"></a>
策略组成员编排 SHALL 废除 `group_edges` 与 `group_node_filters` 的双源分立，改为单一权威有序的 `member_sources` 列表。每个来源项具备明确的操作类型（`op: add | remove`）、相对位置（`position`）与类别（`kind`）。系统 MUST 支持核心类别并实行类型域严格隔离：
1. `manual_node`: 显式手工指定的稳定逻辑 ID 节点；
2. `group_ref`: 直接策略组引用（面向支持组嵌套的目标，输出组名；删除时仅匹配组名引用自身 identity，绝不移除叶节点）；
3. `group_nodes`: 解包展开子组（递归求值并平铺其最终真实叶节点；删除时仅匹配叶节点，绝不移除直接组引用）；
4. `regex` / `filter`: 条件匹配来源（支持包含 regex 正则、国家地区 geo、协议 protocol、探测 probe 的同级条件，同级条件不是全组隐式交集；`pattern` 正则仅匹配候选节点名称，严禁误匹配组 ID；条件仅影响自身候选）；
5. `typed_builtin`: 类型化内建流控项（PASS, DIRECT, REJECT），严禁伪造虚假 UUID。

系统 SHALL 彻底废除策略组上的全局套壳业务 filter（`constraints`），不搞长期双重真源（Dual Authority），避免新策略组出现旧 B1 现象。存量 filter-only 策略组平滑迁移为 `inventory_query_source`。

##### Scenario: 混合增减来源有序编排保存与读取 <a id="scenario-pol-unified-sources-save"></a>
- **GIVEN** 管理员在策略组中编排了 `[+manual_node:A, +group_ref:B, -regex:C, +group_nodes:D, +typed_builtin:PASS]`
- **WHEN** 客户端请求保存该策略组
- **THEN** 服务端持久化至 `policy_group_sources` 表，重读返回相同顺序且属性完全一致的来源操作列表

#### Requirement: 严格交错保序与首次出现去重 [FR-POL-02] <a id="req-pol-order-dedup"></a>
图解析器在求值 `member_sources` 时，MUST 严格保持各来源项产生的候选条目的相对全序，**严禁硬编码分类分块（如将节点前置、组后置）**。系统 MUST 在全局命名空间（`node:<id>`, `group:<id>`, `builtin:<name>`）下对当前有序集合实行 **First-seen wins** 去重规则，首次出现的条目保留其位置。删除操作仅剔除当前已入选条目，删除后不留下永久“墓碑”，后续追加条目可重新加入新位置。节点准入严格复用 Scope、IP Risk 与 Active 状态判定，无额外强加“必须健康”门禁，杜绝静默 drop。

##### Scenario: 相对交错顺序保持 <a id="scenario-pol-interleaved-preserve"></a>
- **GIVEN** 来源编排为 `[+manual_node:N1, +group_ref:G2, +manual_node:N3]`
- **WHEN** 图解析器输出解析快照
- **THEN** 成员列表中严格保持 N1、G2 组名、N3 的前后相对顺序

##### Scenario: 首次出现去重与无墓碑重入 <a id="scenario-pol-first-seen-dedup"></a>
- **GIVEN** 来源编排为 `[+manual_node:N1, -manual_node:N1, +manual_node:N1]`
- **WHEN** 图解析器求值
- **THEN** 最终成员列表包含 N1，N1 成功重新加入新位置，未被历史删除遗留的永久墓碑拦截

#### Requirement: 子组解包展开与同级条件匹配 [FR-POL-03] <a id="req-pol-unpack-expansion"></a>
当来源项为 `group_nodes` 时，图解析器 MUST 递归求值目标子组。求值过程遵循以下原则（用户已确认）：
1. 子组首先依据其自身内部的 `member_sources` 来源操作项完整求值，产出其真实叶节点；子组内部的原始正则来源与手工节点在同级合并，父组不能将过滤套在子组所有来源上；
2. 若该解包来源项配置了同级匹配条件，该条件仅作用于这批解包产出的真实叶节点集合；
3. 若子组内部又包含 `group_ref`，解包递归深入收集其叶节点，最终平铺输出具体节点，不输出子组名；
4. 内建项（如 PASS）依据目标编译器支持能力决定是否保留。

##### Scenario: 子组先自决求值展开 <a id="scenario-pol-unpack-filter-flow"></a>
- **GIVEN** 子组 G_child 配置了匹配来源产出合格节点 N_fast；父组 G_parent 配置了解包 G_child 并附加地区条件 `HK`
- **WHEN** G_parent 执行解析
- **THEN** G_child 先求值出 N_fast，随后 G_parent 的 `HK` 条件检验 N_fast 是否为香港节点，匹配则入选

#### Requirement: 全拓扑深度 DAG 检测与环路阻断 [FR-POL-04] <a id="req-pol-dag-cycle-block"></a>
系统 SHALL 对所有策略组拓扑执行严格的 DAG 环路检测。无论是 `group_ref` 形成的直接引用环，还是 `group_nodes` 形成的解包展开环，或两者的混合环路，在策略组保存（`SaveGroup`）与发布预览（`Preview`）阶段 MUST 予以明确阻断，返回 HTTP 422 `cycle_detected` 并输出准确的 `cycle_path`。系统设置有界深度边界（最大 16 层），杜绝栈溢出，且不进行静默截断。

##### Scenario: 混合解包成环拦截 <a id="scenario-pol-cycle-block-unpack"></a>
- **GIVEN** G1 以 `group_nodes` 展开 G2，G2 以 `group_ref` 引用 G1
- **WHEN** 管理员尝试保存 G2
- **THEN** 系统返回 422 错误且 `cycle_path` 为 `["G2", "G1", "G2"]`，保存被完全回滚

#### Requirement: 顺序重入式来源增减与条件求值规范 [FR-POL-05] <a id="req-pol-ordered-add-remove"></a>
系统废除全局套壳业务 filter，统一在有序来源列表中通过 `op: add | remove` 表达增减。求值算法严格采用用户已确认的**顺序重入流水线**：
1. **顺序执行**: 严格按照 `position` 顺序遍历执行每个条目；
2. **Remove 局部剔除**: `remove` 操作仅从**当前已经进入临时工作集**的条目中剔除匹配项；
3. **后续 Add 重入**: 后续 `add` 操作可将先前被 `remove` 的条目重新追加至新位置；
4. **去重针对当前集合**: First-seen 去重仅针对当前集合存在性，被删除条目不保留永久墓碑；
5. **类型域防污染**:
   - `remove group_ref`: 仅剔除工作集中的组名引用 identity，绝不误删展开的叶节点；
   - `remove group_nodes`: 仅剔除工作集中的叶节点，绝不误删直接组名引用；
   - `regex` / `filter`: 仅作用于候选节点名称与元数据，严禁误匹配组 ID；
6. **准入规则不绕行**: 全量候选池严格执行既有 Active、Scope、IP Risk 准入判定，无静默丢弃，无额外“必须健康”门禁。

##### Scenario: 排除仅作用于自身类型域 <a id="scenario-pol-constraints-leaves"></a>
- **GIVEN** 策略组包含 `[+manual_node:N1, +group_ref:G2, -group_ref:G2]`
- **WHEN** 解析器执行排除
- **THEN** G2 组引用被排除；N1 节点不受组排除影响，正常保留；若存在同名叶节点展开，叶节点亦不受影响

#### Requirement: 单事务原子持久化与乐观并发控制 [FR-POL-06] <a id="req-pol-atomic-optimistic"></a>
策略组保存接口（`PUT /api/v1/policies/groups/{id}`）SHALL 实行单数据库事务原子更新。更新时全量替换该组的 `policy_group_sources` 记录。请求必须携带 `expected_revision`，若版本号不匹配，服务端 MUST 拒绝写入并返回 HTTP 409 `revision_conflict`，保存成功后自增版本号并重读返回一致数据。

##### Scenario: 并发编辑版本冲突保护 <a id="scenario-pol-revision-conflict"></a>
- **GIVEN** 策略组当前数据库 revision 为 5
- **WHEN** 客户端提交 `expected_revision: 4` 的保存请求
- **THEN** 服务端立即拒绝写入，返回 409 Conflict 与当前最新 revision 5

#### Requirement: 全池动态筛选与无截断全量解析 [FR-POL-07] <a id="req-pol-full-inventory"></a>
图解析器在评估 `regex_nodes` 或复合库存查询时，MUST 全量遍历所有处于活动准入状态的节点，严禁在后端施加 50 个节点的隐藏硬分页截断。

##### Scenario: 动态匹配超过 200 个节点 <a id="scenario-pol-no-truncation"></a>
- **GIVEN** 准入节点池包含 250 个匹配某正则的活跃节点
- **WHEN** 策略组解析该 `regex_nodes` 来源
- **THEN** 解析器产出包含全部 250 个节点的候选列表，截断率为 0

#### Requirement: 空策略组 PASS 兜底与防越权隔离 [FR-POL-08] <a id="req-pol-empty-fallback"></a>
当策略组由于动态筛选无匹配导致成员解析为 0 时：
1. 启用 `empty_fallback_pass` 时，Mihomo 编译器输出 `proxies: [PASS]` 与 `empty-fallback: PASS`；
2. 未启用时，编译器输出 `REJECT`，防止非授权穿透；
3. 对于纯节点导出目标（SingBox, Surge, QX），空策略组的存在 MUST NOT 阻断纯节点配置生成。

##### Scenario: Mihomo 导出空组 PASS 兜底 <a id="scenario-pol-fallback-pass"></a>
- **GIVEN** 成员数为 0 且启用 `empty_fallback_pass` 的策略组
- **WHEN** 导出 Mihomo 配置
- **THEN** 导出的 proxy-group 包含 `proxies: [PASS]` 与 `empty-fallback: PASS`

##### Scenario: 纯节点目标不受空组阻断 <a id="scenario-pol-node-targets-unblocked"></a>
- **GIVEN** 存在成员数为 0 且未配置兜底的空策略组
- **WHEN** 导出 SingBox 或 Surge 订阅
- **THEN** 编译器正常渲染活跃节点出站配置，不因策略组空状态报错

#### Requirement: 存量数据无损迁移与审计台账 [FR-POL-09] <a id="req-pol-zero-loss-migration"></a>
系统 SHALL 提供存量数据无损迁移机制，将旧 `group_edges` 与 `group_node_filters` 转换至 `policy_group_sources`：
1. 纯边策略组按存储 position 准确转换为 `manual_node` 或 `group_ref`；
2. 纯过滤策略组转换为 `inventory_query_source`（保留完整国家/协议/正则/探测复合筛选能力）；
3. 边与过滤旧交集策略组明确迁移为来源约束条目或带谓词来源项，一次迁移绝对保持现结果，严禁偷扩增候选；
4. `policy_group_sources` 为唯一权威来源，不搞长期遗留双写；
5. 历史 3 个轮询组与“便宜”组基于归档数据库证据在隔离副本完成恢复校核并出具 receipt，生产安全写保持原授权复验，不默认永久覆盖生产配置；
6. 保持现有 groupIDs、revision、position 与 COW 不可变发布物契约不变，不原样迁移历史 bug。

##### Scenario: 迁移试运行验证 <a id="scenario-pol-migration-dryrun"></a>
- **WHEN** 管理员执行 `POST /api/v1/policies/migration/dry-run`
- **THEN** 服务端返回转换前后的组总数、来源总数及哈希对比，不产生实际数据库写入

---

### 2. 准入规则、路由管理与安全删除

#### Requirement: 策略准入规则与分流规则安全删除 <a id="req-pol-rule-safe-delete"></a>
系统 SHALL 提供 `DELETE /api/v1/policies/rules/{id}` 端点，支持按规则唯一标识符安全删除准入规则（Admission Rule）或分流规则（Policy Rule）。删除成功后 MUST 返回 204 No Content，后续查询该规则所属修订的规则列表时 MUST 不再包含已被删除的规则，且操作 MUST 记录审计事件。

##### Scenario: 删除已存在的策略规则成功返回 204 <a id="scenario-pol-rule-delete-204"></a>
- **GIVEN** 系统中存在标识为 `rule_id` 的有效分流规则
- **WHEN** 客户端向 `DELETE /api/v1/policies/rules/{rule_id}` 发送删除请求
- **THEN** 服务端返回 204 No Content，且后续 `GET /api/v1/policies/rules` 响应中不再包含该规则

##### Scenario: 删除不存在的规则返回 404 <a id="scenario-pol-rule-delete-404"></a>
- **GIVEN** 系统中不存在指定的规则标识
- **WHEN** 客户端向 `DELETE /api/v1/policies/rules/{non_existent_id}` 发送请求
- **THEN** 服务端返回 404 Not Found 且错误码为 `rule_not_found`

#### Requirement: REST 路由路径标识符安全编码 <a id="req-pol-url-encoding"></a>
所有以策略组名称或节点 ID 为路径参数的 RESTful API 端点 MUST 支持标准的 URL 安全编码（Percent-Encoding / encodeURIComponent）。系统 SHALL 正确处理包含空格、斜杠、中文及特殊符号的标识符，严禁触发路径遍历或 HTTP 400 误判。

##### Scenario: 特殊字符与中文组名 URL 编码转义 <a id="scenario-pol-url-safe-special-char"></a>
- **GIVEN** 名称为 `香港 / IPLC 专线` 的策略组
- **WHEN** 客户端请求 `GET /api/v1/policies/groups/%E9%A6%99%E6%B8%AF%20%2F%20IPLC%20%E4%B8%93%E7%BA%BF`
- **THEN** 服务端正确解码并返回该策略组详情，不发生 404 或解码崩溃

##### Scenario: 路径遍历攻击防御 <a id="scenario-pol-url-safe-path-traversal"></a>
- **GIVEN** 包含 `../` 路径穿越特征的恶意参数
- **WHEN** 客户端尝试请求非法的路径
- **THEN** 路由器安全拦截并返回 400 Bad Request，防止越权访问底层系统文件

---

## Supplemental Retained Behavioral Capabilities & Historical Clauses

### Overview
本节完整收录历史变更中属于本领域的行为规范、具体契约（包含精确状态码、传输参数、请求头校验等）与验收场景，保持行为约束无损物理留存与点对点双向追溯。

### Retained Functional Requirements

#### Requirement: 单一确定性解析快照 [HR-043] <a id="req-pol-hr-043"></a>

系统 SHALL 从指定配置修订、活动节点台账和筛选器版本产生带内容摘要的确定性解析快照。预览、导出、策略调试和发布 MUST 使用同一个解析器和同一快照语义；同一输入重算 MUST 产生相同顺序与相同内容摘要。

##### Scenario: 预览与导出一致 <a id="scenario-pol-hr-043-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者预览某个已验证配置修订后立即导出同一目标
- **THEN** 两者引用相同解析快照摘要，且节点和策略成员集合一致

#### Requirement: 五目标编译与显式能力边界 [HR-044] <a id="req-pol-hr-044"></a>

系统 SHALL 支持 Clash、Mihomo、sing-box、Surge 和 Quantumult X 作为命名发布目标。若解析快照含有目标不支持的节点、策略或规则语义，系统 MUST 在预览和发布前给出具体不兼容诊断；系统 MUST 不静默丢弃或降级为另一目标格式。

##### Scenario: 目标不支持的语义 <a id="scenario-pol-hr-044-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者导出包含某目标格式不支持的策略语义的修订
- **THEN** 系统拒绝发布并返回该目标、规则位置和不兼容原因

#### Requirement: 不可变发布物与令牌范围 [HR-045] <a id="req-pol-hr-045"></a>

系统 SHALL 为每次成功发布创建包含目标、解析快照摘要、编译器版本、创建时间、内容摘要和状态的不可变发布物。导出 URL MUST 指向明确发布物；撤销后 MUST 不再返回内容。发布令牌 MUST 被限定至单个发布物或明确最小集合。

##### Scenario: 撤销发布物 <a id="scenario-pol-hr-045-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者撤销一个发布物
- **THEN** 原导出 URL 此后返回已撤销结果，且不得自动回退到最近发布物

#### Requirement: 策略树与准入规则 [HR-114] <a id="req-pol-hr-114"></a>

系统 SHALL 允许策略组引用直接节点、其他策略组、手动包含和排除条件以及命名的准入规则。每个策略组 MUST 使用稳定逻辑 ID；解析 MUST 检测环、缺失引用与不允许的自引用，并拒绝保存或发布无效策略图。策略组可以显式关联一个已激活的 IP risk policy revision；解析 MUST 使用该 revision 的确定性决策排除 `block` 节点，并依该 policy 的未知和 review 规则处理其余节点。无关联或未激活的风险 policy MUST 不改变现有策略组成员语义。

##### Scenario: 循环策略组 <a id="scenario-pol-hr-114-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者保存包含 A → B → A 的策略组关系
- **THEN** 系统返回可定位到相关逻辑 ID 的校验错误且不发布该变更

##### Scenario: 风险 policy 拒绝成员 <a id="scenario-pol-hr-114-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 已绑定的风险 policy 将策略组成员判为 `block`
- **THEN** 解析快照排除该成员并提供脱敏 diagnostics，且不会改写节点台账

#### Requirement: 单一确定性解析快照 [HR-115] <a id="req-pol-hr-115"></a>

系统 SHALL 从指定配置修订、活动节点台账和筛选器版本产生带内容摘要的确定性解析快照。预览、导出、策略调试和发布 MUST 使用同一个解析器和同一快照语义；同一输入重算 MUST 产生相同顺序与相同内容摘要。对于绑定风险 policy 的解析，快照 MUST 绑定 policy revision、参与观察 digest 和风险决策摘要，使风险准入可重现和审计。

##### Scenario: 预览与导出一致 <a id="scenario-pol-hr-115-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者预览某个已验证配置修订后立即导出同一目标
- **THEN** 两者引用相同解析快照摘要，且节点和策略成员集合一致

##### Scenario: 风险观察未改变 <a id="scenario-pol-hr-115-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 输入节点、policy revision 和当前有效风险观察 digest 均未改变
- **THEN** 重算解析快照得到相同成员顺序、风险决策摘要和内容摘要

#### Requirement: 不可变发布物与令牌范围 [HR-116] <a id="req-pol-hr-116"></a>

系统 SHALL 为每次成功发布创建包含目标、解析快照摘要、编译器版本、创建时间、内容摘要和状态的不可变发布物。导出 URL MUST 指向明确发布物；撤销后 MUST 不再返回内容。发布令牌 MUST 被限定至单个发布物或明确最小集合。创建发布物前 MUST 执行绑定 risk policy 的 preflight；preflight 拒绝时 MUST 不创建部分 publication、不得替换现有发布物且不得静默删改节点。

##### Scenario: 撤销发布物 <a id="scenario-pol-hr-116-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者撤销一个发布物
- **THEN** 原导出 URL 此后返回已撤销结果，且不得自动回退到最近发布物

##### Scenario: 风险拦截发布 <a id="scenario-pol-hr-116-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** publication preflight 发现被绑定 policy 阻断的节点
- **THEN** 系统拒绝创建该 publication 并返回脱敏 diagnostics，既有发布物保持不变

#### Requirement: 版本化风险 policy 与显式未知处理 [HR-119] <a id="req-pol-hr-119"></a>

系统 SHALL 允许授权管理者创建、审阅、激活和停用版本化 IP risk policy。每个 policy MUST 声明 score 阈值、最低置信度、网络或匿名化类别条件、证据最大年龄和 `unknown` 处理方式。`unknown` 处理 MUST 是 `allow`、`review` 或 `block` 中之一，且默认值 MUST 为 `review`；未激活 policy 不得改变节点选择或发布语义。

##### Scenario: 缺少当前风险证据 <a id="scenario-pol-hr-119-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 已激活 policy 遇到没有未过期风险观察的节点
- **THEN** 系统应用该 policy 的显式 `unknown` 结果并记录可解释原因，而不把节点标为低风险

#### Requirement: 确定性风险决策与解释 [HR-120] <a id="req-pol-hr-120"></a>

系统 SHALL 将当前有效风险观察及已激活 policy 解释为 `allow`、`review`、`block` 或 `unknown`。相同节点、观察集合、policy revision 和解析时点 MUST 产生相同决策、原因代码和决策摘要。解释 MUST 显示使用的 policy/provider 版本、观察时间和非秘密原因代码，但不得暴露完整 IP、provider 密钥或原始证据。

##### Scenario: 高分匿名出口 <a id="scenario-pol-hr-120-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 当前观察的风险分数、置信度和匿名化类别满足已激活 policy 的 block 条件
- **THEN** 系统返回 `block` 及可解释的规则原因代码

#### Requirement: 策略组准入隔离 [HR-121] <a id="req-pol-hr-121"></a>

系统 SHALL 在策略图解析时把已启用 risk policy 的决策作为节点 admission 输入。`block` 节点 MUST 不进入受该 policy 保护的解析快照；`review` 节点 MUST 由 policy 的显式 group 行为决定纳入隔离组或排除；`allow` 节点按既有图语义继续解析。风险 policy MUST 不修改节点主记录、既有 capability verdict 或其他策略组中未引用该 policy 的成员资格。

##### Scenario: 高风险节点进入受保护策略组 <a id="scenario-pol-hr-121-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 解析一个绑定了已启用 policy 的策略组且某成员决策为 `block`
- **THEN** 解析快照排除该成员并给出其 logical ID 和脱敏原因代码

#### Requirement: 发布前风险拦截 [HR-122] <a id="req-pol-hr-122"></a>

系统 SHALL 在创建 publication 前对目标解析快照重新计算已绑定且已启用 risk policy 的决策。若任一成员为 `block`，或 policy 定义 `review` 不可发布，系统 MUST 拒绝创建 publication 并返回可定位但已脱敏的 preflight diagnostics。系统 MUST 不通过静默删除节点、替换其他发布物或降级目标格式来绕过拦截。

##### Scenario: 导出前发现被阻断节点 <a id="scenario-pol-hr-122-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者发布的快照包含一个当前决策为 `block` 的节点
- **THEN** 系统拒绝 publication，保留既有发布物不变，并返回风险 preflight 诊断

#### Requirement: 决策快照可审计 [HR-123] <a id="req-pol-hr-123"></a>

系统 SHALL 将用于 policy admission 和 publication preflight 的 policy revision、观察 ID 或 digest、决策摘要及解析快照 digest 写入脱敏审计记录。审计记录 MUST 不包含完整 IP、原始 provider 数据或 secret reference 值。

##### Scenario: 审计一次风险阻断 <a id="scenario-pol-hr-123-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** publication 被风险 preflight 拒绝
- **THEN** 审计记录可关联请求、policy revision、快照 digest 和阻断原因代码，且不含敏感原文

#### Requirement: Deterministic Membership Resolution Semantics [HR-143] <a id="req-pol-hr-143"></a>

The system SHALL adhere to the invariant membership resolution algebra: explicit node/group edges SHALL be evaluated as candidates filtered by group filter and parent filter propagation; dynamic full-pool evaluation SHALL only execute when no edges exist and a group filter is non-empty.

##### Scenario: Group with explicit edges evaluates only connected candidates <a id="scenario-pol-hr-143-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** a policy group contains one or more explicit node edges or child group edges
- **THEN** the resolver SHALL apply the group filter strictly to the connected candidates, and UI SHALL clearly indicate that group filters apply only to explicit targets rather than the global pool

##### Scenario: Dynamic group evaluates global admitted pool <a id="scenario-pol-hr-143-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** a policy group contains zero edges and has a non-empty node filter
- **THEN** the resolver SHALL dynamically scan all globally admitted nodes, and UI SHALL inform the user that this group automatically admits matching nodes from the entire pool

#### Requirement: Server-Side Pagination and Search for Policy Groups [HR-144] <a id="req-pol-hr-144"></a>

The system SHALL support server-side pagination (default 50, maximum 100) and case-insensitive keyword search for policy groups via `GET /api/v1/policies/groups`.

##### Scenario: Paginated query returns bounded groups with total count <a id="scenario-pol-hr-144-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** client requests `/api/v1/policies/groups?page=1&page_size=50`
- **THEN** the response SHALL return up to 50 policy groups alongside accurate `page`, `page_size`, and `total` metadata

##### Scenario: Keyword search filters policy groups <a id="scenario-pol-hr-144-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** client provides a search query `?search=proxy`
- **THEN** the response SHALL filter groups by name and group type, returning only matching items and the matching total count

#### Requirement: Asynchronous Candidate Node Selection Without Silent Truncation [HR-145] <a id="req-pol-hr-145"></a>

The policy editor SHALL provide asynchronous search and paginated retrieval for candidate nodes, preventing silent truncation beyond 100 nodes.

##### Scenario: Selecting nodes from a pool exceeding 100 nodes <a id="scenario-pol-hr-145-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** an operator selects nodes to attach as edges in the policy editor
- **THEN** the selector SHALL allow searching and paging through the entire active inventory, ensuring all nodes across the entire pool are discoverable

#### Requirement: Topology Graph Decoupled from Paginated Group Cards [HR-146] <a id="req-pol-hr-146"></a>

The policy management interface SHALL separate the full topology graph representation from the paginated group card list.

##### Scenario: Viewing topology when total groups exceed single page size <a id="scenario-pol-hr-146-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** total policy groups exceed single page size (e.g. > 50 groups)
- **THEN** the validation and topology view SHALL reflect the entire interconnected graph without truncating unpaged groups

#### Requirement: Baseline-Authoritative Health Evaluation [HR-147] <a id="req-pol-hr-147"></a>

The system SHALL evaluate overall node health solely based on fresh baseline probe observations matching the current connection revision, and SHALL NOT fall back auxiliary capability availability (such as AI or streaming services) to mark a node as healthy.

##### Scenario: Node with available AI observation but missing baseline probe <a id="scenario-pol-hr-147-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** a node has successful AI probe observations but lacks a fresh baseline observation for its current connection revision
- **THEN** the system SHALL classify the node health as undetermined rather than healthy, preventing dangerous routing through unverified nodes

##### Scenario: Node with stale or revision-mismatched baseline observation <a id="scenario-pol-hr-147-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** a node connection credentials have been updated or its baseline observation exceeds freshness TTL
- **THEN** the system SHALL classify the node health as undetermined, requiring re-probe before regaining healthy status

#### Requirement: Mutually Exclusive Conservation of Pool Health Counts [HR-148] <a id="req-pol-hr-148"></a>

The system SHALL maintain strict mutually exclusive conservation across all health status counters in `ProbePoolStatus`: `total_count == healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count`, where `available_count == healthy_count + degraded_count`.

##### Scenario: Node pool status aggregation with mixed probe states <a id="scenario-pol-hr-148-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** client queries `GET /api/v1/probes/pool`
- **THEN** the returned counters SHALL sum exactly to `total_count`, with `untested_count` counting only nodes with zero observations and `undetermined_count` counting nodes with inconclusive or stale observations

#### Requirement: Server-Side Health Category Filtering for Nodes [HR-149] <a id="req-pol-hr-149"></a>

The system SHALL support filtering nodes by underlying health category (`healthy`, `degraded`, `unhealthy`, `undetermined`, `untested`) at the database repository level across the full query scope.

##### Scenario: Filtering nodes by health status on node ledger <a id="scenario-pol-hr-149-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** client requests `/api/v1/nodes?health_status=undetermined&page=1&page_size=50`
- **THEN** the server SHALL query the full scope using database predicates and return accurately paginated items and matching total count without calculating counts on client-side memory

#### Requirement: Sticky Desktop Navigation Sidebar [HR-150] <a id="req-pol-hr-150"></a>

The desktop sidebar in `App.vue` SHALL remain sticky and fixed within the viewport during full-page vertical scrolling, preventing navigation controls from disappearing.

##### Scenario: User scrolls long policy or node lists on desktop <a id="scenario-pol-hr-150-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** user scrolls down a long page on a desktop viewport (e.g. 1280x800)
- **THEN** the sidebar SHALL remain anchored at `top: 0` with independent internal overflow scrolling, keeping all navigation tabs accessible at all times

#### Requirement: Body Scroll Locking on Mobile Modal Overlays [HR-151] <a id="req-pol-hr-151"></a>

The application SHALL engage body scroll lock whenever a bottom sheet, modal dialog, or full-screen drawer is active on mobile viewports.

##### Scenario: Mobile sheet opened for policy editing <a id="scenario-pol-hr-151-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** user opens `PolicyEditorSheet` or another modal on mobile viewports (e.g. 375x667, 392x872)
- **THEN** the background body element SHALL apply scroll locking (`overflow: hidden`), preventing background content from scrolling underneath the sheet

#### Requirement: Four-Viewport Responsive Validation [HR-153] <a id="req-pol-hr-153"></a>

The user interface layout SHALL be hermetically verified across four standardized viewports: Desktop (1280x800), Mobile Portrait (375x667), Mobile Large (392x872), and Mobile Landscape (667x375).

##### Scenario: Multi-device rendering inspection <a id="scenario-pol-hr-153-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** the frontend test suite runs across the four designated viewports
- **THEN** navigation elements, cards, and modal sheets SHALL render without horizontal overflow, clipping, or unclickable touch targets

### Explicitly Pending Historical Stubs

#### Requirement: 初始配置修订引导与显式创建 [HR-001] <a id="req-pol-hr-001"></a>

系统在全新数据库初始化启动时，SHALL 确保存在一个初始有效且处于激活状态（Active）的配置修订（Configuration Revision）。当客户端向 `/api/v1/policies/rules` 或 `/api/v1/admission/rules` 发送新增规则请求且未显式指定 `revision_id` 时，系统 MUST 自动绑定至当前的活跃修订，不得返回 422 `missing_revision_id` 错误。系统 SHALL 在 `/api/v1/revisions` 暴露 `POST` 接口，允许创建新的配置修订。发布预览在初始状态下若拓扑图有效，MUST NOT 抛出 409 `no_active_revision` 错误。

##### Scenario: 全新数据库首次启动后创建分流规则成功 <a id="scenario-pol-hr-001-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 客户端连接全新的空白数据库并请求 `POST /api/v1/policies/rules` 且不传 `revision_id`
- **THEN** 服务端自动将其归属于初始引导的活跃配置修订，返回 201 Created 且包含合法的 `revision_id`，不得返回 422 错误

##### Scenario: 初始活跃修订状态下发布预览成功 <a id="scenario-pol-hr-001-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 客户端在全新数据库启动后请求 `POST /api/v1/publications/preview`
- **THEN** 服务端成功生成预览响应，不得返回 409 `no_active_revision` 冲突错误

#### Requirement: 引导与显式活跃修订原子创建 [HR-002] <a id="req-pol-hr-002"></a>

当初次引导或显式创建活跃修订时，系统 SHALL 原子地创建并激活修订；激活失败 MUST 返回错误且回滚创建及任何旧活跃状态更改，不得记录成功审计。下一次启动 MUST 可在无修订时安全重试；已有用户草稿或历史规则 MUST 保留且不得自动激活。数据库查询错误 MUST 原样传递给调用方。独立 `revision.Service` 的草稿审查与显式激活语义保持不变。

##### Scenario: 首次引导激活暂时失败后重启 <a id="scenario-pol-hr-002-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 全新数据库首次启动时激活步骤失败一次，随后以同一数据库重新启动服务
- **THEN** 首次启动返回错误且不遗留草稿，第二次成功生成初始活跃修订；已有用户草稿和规则不会被隐式激活或删除

##### Scenario: 显式创建活跃修订激活失败 <a id="scenario-pol-hr-002-2"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** `policy.Service.CreateRevision(StateActive)` 的激活步骤失败
- **THEN** 调用返回失败，不存在此次请求的新草稿、假活跃状态或成功审计，原活跃版本仍保持活跃

#### Requirement: 策略树与准入规则 [HR-042] <a id="req-pol-hr-042"></a>

系统 SHALL 允许策略组引用直接节点、其他策略组、手动包含和排除条件以及命名的准入规则。每个策略组 MUST 使用稳定逻辑 ID；解析 MUST 检测环、缺失引用与不允许的自引用，并拒绝保存或发布无效策略图。

##### Scenario: 循环策略组 <a id="scenario-pol-hr-042-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** 管理者保存包含 A → B → A 的策略组关系
- **THEN** 系统返回可定位到相关逻辑 ID 的校验错误且不发布该变更

#### Requirement: Elimination of Probes Waterfall Polling [HR-152] <a id="req-pol-hr-152"></a>

The probe workbench composable `useProbes` SHALL eliminate synchronous multi-page waterfall loops (`while (page <= 20)`), fetching single paginated views and consuming backend pool metrics directly.

##### Scenario: Loading probe workbench nodes <a id="scenario-pol-hr-152-1"></a>
- **GIVEN** 策略组规则与准入树处于已加载状态
- **WHEN** user navigates to the Probes view
- **THEN** the client SHALL request bounded page data (`page_size <= 50`) and utilize `GET /api/v1/probes/pool` for overall pool metrics, avoiding sequential waterfall requests


---

## Success Criteria

### Measurable Outcomes
- **[SC-POL-01]** 循环引用 DAG 检测率达到 100%，无死循环或堆栈溢出。
- **[SC-POL-02]** 全池动态匹配无截断率达到 100%，200+ 节点全量检出。
- **[SC-POL-03]** 准入规则删除 204 与 404 响应准确率达到 100%。
- **[SC-POL-04]** 特殊字符 URL 编码路径解析成功率达到 100%。

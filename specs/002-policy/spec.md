---
feature: 002-policy
title: Policy Capability Specification
status: candidate
schema: speckit-native
---

# Policy Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的策略组与图解析器核心能力。系统必须支持灵活的代理分流拓扑，提供两种核心的成员引用模式——直接将子策略组作为节点引用（Direct Group Reference）与解包获取其中的叶节点（Unpack / Flatten Leaf Nodes），支持准入规则 REST 安全删除与 URL 编码防护，并在策略组为空时提供受控的 PASS 兜底机制与防越权隔离。

## User Scenarios & Priority

### User Story 1 - 策略组双模式嵌套引用 (Priority: P1)
- **GIVEN** 管理员创建了一个顶级策略组 G1 和一个地区子组 G2
- **WHEN** 管理员在 G1 中选择引用 G2
- **THEN** 系统必须支持直接以组名引用 G2（Mihomo 原生组嵌套），亦必须支持解包模式将 G2 内的最终叶节点直接平铺展开至 G1

### User Story 2 - 全池动态条件无截断匹配 (Priority: P1)
- **GIVEN** 系统导入了超过 200 个有效节点
- **WHEN** 管理员配置了一个按国家代码或正则匹配的动态策略组
- **THEN** 图解析器全量遍历所有活动节点，返回所有匹配项，严禁施加 50 节点的隐藏硬分页截断

### User Story 3 - 准入规则安全删除与 REST 编码防护 (Priority: P2)
- **GIVEN** 管理员需要调整分流规则或删除失效规则
- **WHEN** 发起 HTTP DELETE 请求或引用特殊字符命名的策略组
- **THEN** 系统按 REST 语义安全删除并记录审计，支持 URL 编码转义并抵御路径穿越

---

## Requirements

### 1. 策略组结构与成员引用模式

#### Requirement: 直接将其他策略组作为节点引用 [FR-POL-01] <a id="req-pol-direct-ref"></a>
图解析器与编译器 SHALL 支持将现有策略组作为目标策略组的直属成员引用（Mihomo 原生组嵌套）。在导出时直接在 proxies 数组中保留子策略组名称。包含循环引用防御。

##### Scenario: Mihomo 原生嵌套组引用 <a id="scenario-pol-direct-nesting"></a>
- **GIVEN** 顶级策略组 Proxy 与子策略组 Auto-HK
- **WHEN** 管理员在 Proxy 中添加 Auto-HK 作为直接引用
- **THEN** 编译器在 Proxy 的 proxies 成员中输出 "Auto-HK"，客户端可直接执行嵌套切换

##### Scenario: 循环引用检测与防护 <a id="scenario-pol-cycle-protect"></a>
- **GIVEN** 策略组 A 引用了策略组 B
- **WHEN** 管理员尝试在策略组 B 中添加引用策略组 A
- **THEN** 系统检测出 DAG 环路并阻断保存，返回 400 Bad Request 与明确循环依赖报错

#### Requirement: 解包获得子策略组叶节点 [FR-POL-02] [NEEDS CLARIFICATION] <a id="req-pol-unpack-leaf"></a>
系统 SHALL 支持“解包获得其中节点”模式（Unpack / Flatten Leaf Nodes）。当启用解包时，图解析器必须递归展开子组引用的所有末端叶节点，平铺合并至当前策略组中。

##### Scenario: 解包平铺叶节点至父组 <a id="scenario-pol-flatten-leaf"></a>
- **GIVEN** 子策略组 G2 包含叶节点 N1 与 N2
- **WHEN** 顶级策略组 G1 启用解包模式引用 G2
- **THEN** G1 在最终解析结果中直接包含 N1 与 N2，而不包含 G2 本身

##### Scenario: 解包展开去重保序 <a id="scenario-pol-dedup-order"></a>
- **GIVEN** 多个解包子组包含重复的叶节点
- **WHEN** 解析器执行叶节点展开
- **THEN** 依据用户在 Q1 中的决策执行稳定去重并保持相对展开顺序

#### Requirement: 成员模式数据契约与前端编辑 [FR-POL-03] [NEEDS CLARIFICATION] <a id="req-pol-member-contract"></a>
策略组配置契约与前端抽屉 SHALL 明确区分 direct 与 unpack 两种成员模式，向管理面板提供清晰的单选/勾选交互。

##### Scenario: 策略组抽屉配置双模式 <a id="scenario-pol-drawer-schema"></a>
- **GIVEN** 管理员打开策略组编辑抽屉
- **WHEN** 选择添加策略组成员
- **THEN** 界面清晰提供“直接引用子组”与“解包叶节点”选项

#### Requirement: 成员来源混合交错顺序保持 [FR-POL-04] [NEEDS CLARIFICATION] <a id="req-pol-interleaved-order"></a>
当策略组同时包含显式节点、子策略组引用及规则筛选成员时，系统 SHALL 依据用户在 Q2 中的决策，保持各来源成员在配置界面中的相对交错保序性。

##### Scenario: 相对交错顺序严格保持 <a id="scenario-pol-interleaved-preserve"></a>
- **GIVEN** 管理员编排了 [节点A, 子组B, 节点C] 的混合交错序列
- **WHEN** 解析器执行序列化输出
- **THEN** 输出序列中严格保持节点 A、子组 B 展开、节点 C 的前后相对顺序

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

##### Scenario: 删除操作记录审计事件 <a id="scenario-pol-rule-delete-audit"></a>
- **GIVEN** 管理员成功删除了某条规则
- **WHEN** 检视系统操作审计日志
- **THEN** 审计日志记录操作人、目标规则 ID、删除时间及状态，保证可追溯性

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

### 3. 图解析、安全兜底与历史兼容

#### Requirement: 空策略组 PASS 兜底与防越权隔离 [FR-POL-05] <a id="req-pol-empty-fallback"></a>
当策略组由于动态筛选无匹配、上游订阅离线等原因导致成员解析为 0 时，系统 SHALL 提供受控的兜底机制。启用 `empty_fallback_pass` 时输出 `proxies: [PASS]` 与 `empty-fallback: PASS`，未启用时输出 `REJECT`。

##### Scenario: 启用 empty_fallback_pass 输出 PASS <a id="scenario-pol-fallback-pass"></a>
- **GIVEN** 解析成员数为 0 且启用了空组兜底的策略组
- **WHEN** 编译器生成 Mihomo 导出配置
- **THEN** 该策略组包含 `proxies: [PASS]` 且 `empty-fallback: PASS`，客户端基础流量不断网

##### Scenario: 未启用空组输出 REJECT <a id="scenario-pol-fallback-reject"></a>
- **GIVEN** 解析成员数为 0 且未启用兜底的策略组
- **WHEN** 编译器生成导出配置
- **THEN** 编译器输出 REJECT 兜底或阻断无效流量

#### Requirement: 全池动态筛选与无截断全量解析 [FR-POL-06] <a id="req-pol-dynamic-match"></a>
图解析器在评估国家代码、正则匹配或标签筛选规则时，MUST 全量遍历所有处于活动状态的节点，严禁在后端施加 50 个节点的隐藏硬分页截断。

##### Scenario: 超过 200 节点全量匹配无 50 截断 <a id="scenario-pol-no-truncation"></a>
- **GIVEN** 数据库中包含 200 个匹配某国家代码的活动节点
- **WHEN** 动态策略组执行解析
- **THEN** 成员列表中全量包含 200 个节点，截断损耗为 0

#### Requirement: 历史遗留策略组数据迁移恢复 [FR-POL-07] [NEEDS CLARIFICATION] <a id="req-pol-legacy-migration"></a>
针对历史版本中存在的无显式节点且无规则的空策略组数据，系统 SHALL 依据用户在 Q4 中的裁决执行平滑迁移或归档恢复。

##### Scenario: 历史空组数据平滑过渡 <a id="scenario-pol-migrate-empty-group"></a>
- **GIVEN** 历史数据库中存在存量空策略组
- **WHEN** 启动数据版本迁移程序
- **THEN** 系统按设定规则安全保留或归档该记录，不破坏配置树完整性

---

---

---

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

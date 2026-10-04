## Purpose

定义启用订阅库存范围收敛与多视图对齐契约，将全库节点资产台账与当前启用订阅的可用库存清晰解耦，统一首页概览、节点列表、订阅卡片、探测池及发布编译的有效节点集合。

## ADDED Requirements

### Requirement: 启用订阅库存范围定义与节点列表作用域收敛
系统 SHALL 在 `GET /api/v1/nodes` 支持 `scope` 参数，控制查询的数据集合范围：
1. **默认范围 `scope=enabled_subscriptions`（范围 E）**：
   - 节点必须属于至少一个当前已启用的订阅源（`subscriptions.enabled = 1`）；
   - 关联关系基于该订阅最新一次成功或部分成功的抓取记录（`subscription_fetches.outcome IN ('success', 'partial')`，失败抓取保持上次有效成果）；
   - 节点不得为已确证的公告伪节点（`subscription_entries` 中确认为 notice 且未被用户覆盖为 proxy）；
   - **`active` 状态独立**：系统 MUST NOT 将 `nodes.active` 作为进入范围 E 的前置条件；`active_only`、`search_text`、`protocol`、`risk_*` 及 `subscription_id` 为在范围 E 之上叠加的附加过滤条件；
   - **全局去重**：主列表与总数基于 `logical_id` 全局去重；当节点同时归属多个订阅时仅展示一条；停用其中一个订阅时，只要仍属于其他已启用订阅，该节点继续保留在范围 E 中；
   - 排除无来源关联的手工历史节点，但绝不删除其底表记录；
2. **显式管理资产范围 `scope=all_assets`**：
   - 仅在管理员显式传入 `scope=all_assets` 时返回包含历史失活节点的全库台账（1014 条资产）；
3. **未知作用域报错**：
   - 传入除 `enabled_subscriptions` 与 `all_assets` 之外的未知 scope 参数时，系统 MUST 返回 HTTP 400 错误码 `invalid_node_scope`，严禁隐式回退至全库；
4. **单节点详情与覆写接口保全**：
   - `GET /api/v1/nodes/{logical_id}` 与 `PATCH /api/v1/nodes/{logical_id}/connection` 对所有合法的 `logical_id` 资产开放，允许查看与修改历史节点参数，不受列表默认作用域收敛影响。

#### Scenario: 默认查询仅返回当前启用订阅的有效节点
- **WHEN** 客户端请求 `GET /api/v1/nodes` 且未传递 `scope` 参数
- **THEN** 系统默认应用 `scope=enabled_subscriptions`，仅返回当前已启用订阅源最新成功抓取中去重后的可用节点（当前生产为 31 条），返回 HTTP 200

#### Scenario: 显式请求全库资产台账
- **WHEN** 客户端请求 `GET /api/v1/nodes?scope=all_assets`
- **THEN** 系统返回包含历史失活节点的全量资产台账（当前生产为 1014 条），返回 HTTP 200

#### Scenario: 未知 scope 参数直接返回 400
- **WHEN** 客户端请求 `GET /api/v1/nodes?scope=invalid_scope`
- **THEN** 系统返回 HTTP 400 与错误码 `invalid_node_scope`，不返回任何节点列表数据

#### Scenario: 停用订阅源不影响仍被其他启用源引用的共享节点
- **WHEN** 节点 N 同时属于订阅源 A（启用）与订阅源 B（启用），管理员将订阅源 B 停用（`enabled=0`）
- **THEN** 节点 N 依然出现在默认的 `enabled_subscriptions` 视图中；若订阅源 A 也停用，节点 N 从该视图中消失，但 `nodes` 表记录与 `active` 状态不被修改

#### Scenario: 查看与修改历史节点详情不被阻断
- **WHEN** 管理员通过 `GET /api/v1/nodes/{logical_id}` 调阅不在范围 E 中的历史节点
- **THEN** 系统返回该节点的完整详情与历史来源记录，返回 HTTP 200

### Requirement: 订阅列表归属计数与动态生效语义
系统 SHALL 在 `GET /api/v1/subscriptions` 的返回条目中包含动态计算的节点计数：
1. `node_count` (integer): 当前订阅作为已启用源对全局可用库存的去重非公告有效成员贡献。当订阅被停用（`enabled = 0`）时，`node_count` 必须为 0；
2. `source_node_count` (integer): 当前订阅在其最后一次成功或部分成功抓取中包含的去重非公告有效成员数。无论该订阅是否启用，均保留该计数值用于向管理员说明该源的历史成员规模；
3. `counts_scope` (string): 固定返回 `'enabled_subscriptions'`；
4. 若订阅从未产生过成功或部分成功的抓取，`node_count` 与 `source_node_count` 均为 0；若先前成功而最新抓取失败，系统保持上次成功抓取的成员计数；
5. 订阅启用状态切换通过既有 `PATCH /api/v1/subscriptions/{id}` 完成，即时生效，绝不修改关联节点的 `nodes.active` 属性，计数动态查询不作永久缓存。

#### Scenario: 启用状态订阅展示贡献数与源成员数相等
- **WHEN** 查询包含有效抓取的已启用订阅（如 Dogegg 拥有 16 个有效成员）
- **THEN** 返回的条目中 `node_count: 16`，`source_node_count: 16`，`counts_scope: 'enabled_subscriptions'`

#### Scenario: 停用订阅后 node_count 归零但保留 source_node_count
- **WHEN** 管理员将拥有 16 个成员的订阅停用（`enabled = 0`）并重新请求订阅列表
- **THEN** 该订阅条目中 `node_count: 0`，`source_node_count: 16`

#### Scenario: 多源共享节点导致单源累加和大于全局去重总数
- **WHEN** 订阅 A 与 订阅 B 分别贡献 10 个节点，且其中 3 个节点为二者共享
- **THEN** 订阅 A 的 `node_count` 为 10，订阅 B 的 `node_count` 为 10，但全局 `GET /api/v1/nodes` 范围 E 返回 17 个去重节点

### Requirement: 探测候选池收敛与指标对账保全
系统 SHALL 在 `GET /api/v1/probes/pool` 与探测任务派发中收敛至范围 E：
1. **指标结构扩展**:
   - `inventory_total` (integer): 范围 E 中的总节点数（包含活跃与失活）；
   - `candidate_total` (integer): 范围 E 中满足 `active = 1` 的探测候选总数；
   - `scope` (string): 固定返回 `'enabled_subscriptions'`；
   - `total_count` (integer): 保持旧字段含义不变，其数值与 `candidate_total` 一致，确保既有消费者的计量单位不发生漂移；
2. **探测候选执行门禁**:
   - 周期性探测调度（periodic coordinator）、全量探测触发（run all）以及指定节点直接派发（selected IDs）均必须严格施加 `E + active=1 + exclude_notices` 过滤；
   - 严禁通过直接传递已停用订阅源节点的 ID 绕过门禁执行探测。

#### Scenario: 探测池状态反映启用库存与候选指标
- **WHEN** 请求 `GET /api/v1/probes/pool`
- **THEN** 返回结果包含 `inventory_total`、`candidate_total`、`total_count`（等于 candidate_total）与 `scope: 'enabled_subscriptions'`

#### Scenario: 指定探测已停用订阅节点被严格排除
- **WHEN** 创建探测任务时在 `node_logical_ids` 中传入属于已停用订阅的节点 ID
- **THEN** 系统在构建任务目标时将其排除，不向该节点派发拨测

### Requirement: 发布订阅编译与既有快照不可变保全
系统 SHALL 在配置发布与预览中保持快照不可变：
1. **新发布与预览过滤**:
   - 发布解析器（`publication.Service`）在构建配置快照时，仅从范围 E 中筛选 `active = 1` 的节点；
   - 停用订阅源后，新生成的预览配置立即排除属于停用源的节点；
2. **已发布快照字节绝对不可变**:
   - 已创建并对外分发的发布快照内容（`ConfigBytes`）受只读保护，订阅源状态变化或节点状态变化严禁篡改已发布的历史快照。

#### Scenario: 停用订阅后新预览排除该源节点
- **WHEN** 管理员停用某订阅源并请求生成新的客户端配置预览
- **THEN** 预览配置中仅包含当前仍处于启用状态订阅的活跃节点

#### Scenario: 历史发布快照内容保持字节级不可变
- **WHEN** 某发布快照已生成，管理员后续停用其中部分节点所归属的订阅源
- **THEN** 通过分发 Token 请求该既有发布配置时，返回的配置文本与生成时完全一致

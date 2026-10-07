## Purpose

规范 CSP 分流规则与策略组的只读动态校验 API、空策略组阻断判定、结构化 issues 诊断与前端规则页防竞态交互机制。

## ADDED Requirements

### Requirement: 只读分流规则与动态拓扑校验 API
系统 SHALL 在 `POST /api/v1/policies/validate`（以及 `GET`）上提供当前已保存活跃配置版本（active revision）的只读校验能力。校验过程 MUST 仅基于当前已启用的订阅节点库存范围（`NodeScopeEnabledSubscriptions`），执行纯内存解析与诊断，绝不触发外部网络请求、商业出网、Mihomo 探针执行或任何数据库写入操作。返回结果 MUST 包含 `revision_id`，并保持既有 `valid` 与 `errors` 字段的向后兼容。

#### Scenario: 活跃配置版本只读校验返回结构化诊断
- **WHEN** 客户端请求 `POST /api/v1/policies/validate`
- **THEN** 系统返回当前 active revision 的校验结论，包含 `valid: boolean`、`errors: string[]`、`revision_id: string` 以及结构化 `issues: ValidationIssue[]`

### Requirement: 直接指向空策略组规则判定为致命错误
系统 SHALL 在拓扑与分流校验中，将所有被分流规则（`policy_rules`）直接引用的空策略组（可用成员数或节点数为 0）严格判定为致命错误（Severity: `error`，Code: `empty_routed_group`），无论系统是否配置了节点筛选条件（`anyFilterDefined` 为 true 或 false）。对于未被任何分流规则直接引用的普通空策略组（如作为可选子候选组），系统 MUST 保持为警告（Severity: `warning`，Code: `empty_group`），不破坏编译器严格默认安全策略。当全局有效节点库存为空时，诊断信息 MUST 明确指示当前有效库存为空背景。

#### Scenario: 分流规则直接指向无节点策略组触发错误
- **WHEN** 存在分流规则（如 `PROCESS-NAME,tr.com.kliq.app`）直接指向无可用节点的策略组（如“其他”）
- **THEN** 校验结果的 `valid` 为 false，`issues` 中包含对应规则的 `empty_routed_group` 错误项，标明 `rule_id`、`position`、`type`、`value`、`target_group_id` 及 `target_group_name`

#### Scenario: 未被规则直接指向的可选空子组仅触发警告
- **WHEN** 策略组无可用节点但未被任何分流规则直接引用（仅作为其他组的子组选项）
- **THEN** 校验结果发出 `empty_group` 警告，不因此阻断规则级拓扑校验

### Requirement: 前端规则管理与防竞态校验守卫
前端 `PolicyView` SHALL 增设“规则”Tab，按顺序展示分流规则列表（包含序号/位置、类型、匹配值及目标策略组），并提供单条规则的最小化删除操作。页面 SHALL 提供统一的校验结果诊断区域与“手动校验”按钮。在页面初始加载、刷新成功以及策略保存/删除成功后，前端 MUST 自动触发对新版本的校验；若保存操作失败，严禁显示假成功或伪造校验通过；校验请求 MUST 通过 AbortController 与请求序列守卫防止延迟响应覆盖最新版本状态。

#### Scenario: 页面加载与策略变更自动触发校验
- **WHEN** 用户进入策略页、刷新数据或成功删除/更新规则
- **THEN** 前端自动调用校验接口，若存在晚到达的过时校验响应则被序列守卫丢弃，UI 展示对应最新版本的真实校验状态

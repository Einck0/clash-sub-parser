## MODIFIED Requirements

### Requirement: 服务端分页台账
系统 SHALL 提供节点列表读取，接受 `page`、`page_size`、固定排序和已声明的协议、国家、连通性、延迟与能力筛选器。响应 MUST 返回 `items`、`page`、`page_size`、`total` 与用于详情读取的节点 `logical_id`；服务端 MUST 在筛选后分页。节点读取 SHALL 支持已声明的 IP risk decision、risk level、provider 和 freshness 筛选，并在列表和详情中仅返回当前风险的脱敏摘要。

#### Scenario: 万级节点的筛选分页
- **WHEN** 调用者以有效筛选条件请求某一页节点
- **THEN** 返回项数不超过 `page_size`，`total` 表示同一筛选集合总数，且客户端无需下载完整台账

#### Scenario: 风险筛选后的分页
- **WHEN** 调用者以有效 `risk_decision` 或 `risk_level` 筛选条件请求节点页
- **THEN** 系统先在服务端应用风险筛选再分页，并使每一项仅包含允许的风险摘要而不含完整出口 IP

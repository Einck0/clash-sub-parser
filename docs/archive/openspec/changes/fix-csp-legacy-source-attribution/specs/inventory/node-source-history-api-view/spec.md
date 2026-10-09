## Purpose

规范只读 API 端点 `GET /api/v1/nodes/{id}/source-history` 的响应结构与前端节点抽屉的展示契约，向管理员安全呈现来源血统与归属证据。

## ADDED Requirements

### Requirement: 节点来源历史只读接口
系统 SHALL 提供 `GET /api/v1/nodes/{id}/source-history` 端点，返回该节点的实时来源、历史归属列表及综合归属状态。

#### Scenario: 查询存在历史的孤儿节点
- **WHEN** 请求一个已恢复历史的孤儿节点详情时
- **THEN** 系统返回 HTTP 200，响应结构包含空的 `current_sources` 列表、按时间倒序排列的 `history` 列表，且综合状态 `attribution_status` 被准确标记为 `historical_verified`；响应内容绝对不包含密码、Token、UUID 或敏感 URL query。

#### Scenario: 查询当前有源活跃节点
- **WHEN** 请求一个当前属于活跃订阅的节点时
- **THEN** 系统返回其所属的当前订阅信息（`current_sources`），以及可能存在的离开历史；综合状态被标记为 `current`。

### Requirement: 节点详情界面只读抽屉呈现
前端界面 SHALL 在现有节点详情侧栏/抽屉（Drawer）中增设来源归属展示区域，与当前实时订阅明确区分。

#### Scenario: 查看历史来源证据
- **WHEN** 用户在管理界面打开节点详情抽屉并切换至“来源与历史”面板时
- **THEN** 界面清晰呈现当前实时归属标签与历史观测记录（含来源名称、观测时间戳、归属原因与可验证证据类型），不提供手工虚构 URL 输入框，不篡改默认列表的当前有效视图。

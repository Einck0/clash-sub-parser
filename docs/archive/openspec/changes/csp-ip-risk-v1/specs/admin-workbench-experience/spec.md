## MODIFIED Requirements

### Requirement: 服务端分页与查询状态
节点台账界面 SHALL 仅请求当前服务端页并将筛选、排序和页码反映为可恢复的 URL 查询状态。加载中、空结果、无权限和失败状态 MUST 有区别明确的界面反馈；界面 MUST 不在浏览器内获取完整节点集合后再模拟分页。风险 decision、risk level、provider 和过期状态 MUST 作为服务端节点筛选条件，并在 URL 状态中可恢复。

#### Scenario: URL 恢复筛选页面
- **WHEN** 用户复制包含有效筛选和页码的节点台账 URL 并在新窗口打开
- **THEN** 界面恢复对应筛选与页码，并请求该服务端页

#### Scenario: 恢复风险筛选
- **WHEN** 用户打开包含有效风险筛选、排序和页码的节点台账 URL
- **THEN** 界面恢复风险筛选并请求对应服务端页，而非下载完整节点集合

### Requirement: 破坏性与长时命令反馈
界面 MUST 在删除、停用、撤销发布、导入、刷新和探测等命令前后展示明确状态。破坏性命令 MUST 要求确认；长时作业 MUST 显示作业状态并允许在授权范围内取消，不能通过无提示重复点击创建重复命令。界面 SHALL 对风险 policy 造成的准入排除和 publication preflight 拒绝显示 policy revision、脱敏原因与明确的 unknown 状态，不得将 unknown 表示为低风险。

#### Scenario: 重复点击探测按钮
- **WHEN** 用户在探测提交尚未收到响应时再次点击同一按钮
- **THEN** 界面维持单一进行中状态并不会创建第二个请求

#### Scenario: 风险拦截反馈
- **WHEN** 发布预检因节点 IP risk 决策而被拒绝
- **THEN** 界面显示脱敏 diagnostics 和 policy revision，且不把该节点或未知结果渲染为通过

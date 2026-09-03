## MODIFIED Requirements

### Requirement: NodeLedger 双视图与统一查询状态
控制台 SHALL 保留 NodeLedger 的指标面板、卡片网格和紧凑表格双视图。两种视图 MUST 共享同一个查询、排序、分页/虚拟窗口和选择状态，切换视图不得清空筛选、选择或滚动语义

#### Scenario: 切换台账视图
- **WHEN** 操作者在已有组合筛选和多选节点时切换卡片/表格
- **THEN** 显示同一结果集合及选择状态，且继续使用既定分页或窗口位置

### Requirement: 多维组合筛选与指标快捷筛选
NodeLedger SHALL 支持名称/地址/端口关键字、来源、协议、探测状态、跳板状态、最低速度、媒体/AI 能力与排序组合筛选。总数、健康、高速和已挂载跳板指标 MUST 可快捷设置或重置相应过滤条件

#### Scenario: 按能力和速度筛选
- **WHEN** 操作者选择最低速度及一个媒体/AI 能力条件
- **THEN** 列表只显示匹配节点，并可解释未探测或不匹配节点的状态

### Requirement: 批量诊断与单节点详情
控制台 SHALL 保留全选、反选、批量探测、测速开关、取消 Job、清理缓存、节点详情抽屉及 sing-box/Clash 预览复制。相同命令运行中 MUST 防止重复提交并给出可恢复错误

#### Scenario: 打开节点详情后重新探测
- **WHEN** 操作者在详情抽屉发起单节点探测
- **THEN** 页面显示 Job 状态，完成后刷新该节点的延迟、出口和能力摘要，且可复制目标配置预览

### Requirement: 前端领域切片与草稿安全
NodeLedger、Subscriptions、NodeGroups、Rules、ProxyChains、Dns、Generate/QuickExport、Settings 和 ConfigHistory SHALL 按领域组件与 store 组织。远端刷新、路由切换和失败响应不得覆盖未保存的编辑草稿；危险操作需明确确认、取消后恢复焦点

#### Scenario: 编辑策略时后台刷新
- **WHEN** 操作者编辑策略组且节点查询返回新数据
- **THEN** 节点候选可更新，但当前策略草稿保持不变并提示其基于的状态
## MODIFIED Requirements

### Requirement: 订阅与手工输入完整生命周期
系统 SHALL 保留远程订阅、Base64/Clash YAML 与多行分享链接输入，并将手工节点视为与远程来源同等可追溯的输入。保存、刷新和预览 MUST 保留启用状态、主订阅、自动命名、节点前缀、精确筛选、重命名、手工节点、测速/解锁过滤、跳板和流量信息语义

#### Scenario: 导入混合协议订阅
- **WHEN** 来源包含 Base64、Clash YAML 或混合分享链接
- **THEN** 系统解析所有受支持节点并保留来源修订与节点关联

### Requirement: 全协议规范化与配置转换
系统 SHALL 支持 SS、SSR、VMess、VLESS、Trojan、Hysteria2、TUIC、WireGuard、HTTP、HTTPS 和 SOCKS5 的导入、规范化、探测输入和目标配置生成。协议专有字段 MUST 在解析、持久化与 renderer 间完整传递，不得因架构迁移静默降级

#### Scenario: 保留协议专有字段
- **WHEN** 节点包含 Reality、gRPC、port hopping、WireGuard reserved 或代理插件字段
- **THEN** 对应目标支持时生成的配置与旧基线等价，不支持时给出明确诊断而非删除字段

### Requirement: 来源修订与节点来源追溯
成功刷新 SHALL 原子保存 SourceRevision、规范化节点及 NodeSourceLink；同一节点来自多个来源时 MUST 保留所有关联。失败刷新 MUST 保留最后成功内容并写入可诊断的结果和更新时间

#### Scenario: 刷新失败
- **WHEN** 已存在成功内容的来源刷新失败
- **THEN** 系统不清空其可用节点，记录失败时间和脱敏错误，调度不会无间隔重试

### Requirement: 候选节点隔离与最终名匹配
订阅筛选候选 SHALL 仅来自该订阅的来源节点和手工节点；节点处理顺序 MUST 为筛选、前缀、重命名、去重。策略 regex 必须匹配最终节点名

#### Scenario: 编辑订阅的候选节点
- **WHEN** 操作者在一个订阅中配置筛选或重命名
- **THEN** 不会显示或选择其他订阅的节点，策略组随后按重命名后的节点名动态匹配
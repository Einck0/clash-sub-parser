## MODIFIED Requirements

### Requirement: Node Outbound Isolation and Authorized Protocol Handshake
系统 SHALL 仅为已启用、预算未耗尽的 ProbeProfile Job 中的候选节点执行受控隔离握手。节点身份使用 logical ID；运行器不得继承宿主代理环境；每次尝试前检查授权版本和预算，终态结果作为不可变观测提交

#### Scenario: 授权 Profile 的成功握手
- **WHEN** enabled Profile 的有预算任务包含有效节点
- **THEN** 系统经该节点验证连通性、记录握手/延迟并回收资源

#### Scenario: 授权在运行中撤销
- **WHEN** Profile 被禁用、取消、删除、版本变更或预算耗尽
- **THEN** 系统停止派发后续尝试、取消或收尾在途工作并给出明确终态

### Requirement: Egress Identity and Geo-location Consensus
系统 SHALL 将经被测节点取得的出口身份作为独立能力结果保存；宿主出口不得作为节点结论

#### Scenario: Multi-provider geo consensus
- **WHEN** 节点通过隔离运行环境建立出站
- **THEN** 系统按 Profile 规则保存出口共识或不一致状态

### Requirement: Streaming Media and AI Unlock Probing
系统 SHALL 仅按 Profile 的目标类别与预算执行无私有账号的流媒体/AI 能力探测，并分别保存可用、受限、限流、挑战和未知状态

#### Scenario: Netflix unlock status classification
- **WHEN** 流媒体探测经候选节点执行
- **THEN** 系统保存服务级分类，不把它转换成节点存活结论

#### Scenario: AI and platform rate limit / challenge handling
- **WHEN** AI 或平台返回挑战或 HTTP 429
- **THEN** 系统记录挑战或限流并保留基础连通性结论

### Requirement: Bounded Speed Testing and Capability Filtering
系统 SHALL 按 Profile 的字节、时间和全局并发预算测速。编译器只通过已声明的策略谓词消费观测，并解释 Profile、时效、阈值、slot 与服务状态

#### Scenario: Controlled download speed benchmark
- **WHEN** 可达节点进入测速阶段
- **THEN** 系统在预算内计算吞吐量并保存为该观测的一部分

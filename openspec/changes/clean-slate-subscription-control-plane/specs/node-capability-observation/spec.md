## Purpose

为节点真实出站能力定义经 ProbeProfile 授权、预算受限且不泄露连接秘密的不可变观测

## ADDED Requirements

### Requirement: ProbeProfile 授权与预算
每个 ProbeProfile SHALL 有 logical ID、版本、enabled 状态、目标类别、周期、并发、字节、时长和保留预算。只有已保存且 enabled 的 Profile 才能手动或周期创建 ProbeJob；执行器每次网络尝试前检查授权版本和剩余预算

#### Scenario: 禁用、取消或预算耗尽
- **WHEN** Profile 禁用/删除/变更、Job 取消或任一预算耗尽
- **THEN** 不创建或派发下一次探测；任务以确定终态结束，在途工作回收端口和资源

### Requirement: 真实代理出站
系统 SHALL 以 `trust_env=false` 和受控隔离运行环境经被测节点执行握手、出口、测速、流媒体和 AI 观测。宿主代理不得替代被测节点；观测只记录脱敏分类与必要摘要，不记录连接秘密

#### Scenario: 宿主出口不可替代
- **WHEN** 被测节点不可达而宿主机可访问探测目标
- **THEN** 观测记录节点失败，绝不将宿主结果记为节点成功

### Requirement: 分项、不可变与可解释
系统 SHALL 追加不可变终态观测，分别保存连通性、出口、测速、流媒体和 AI 结论；节点故障、地区、限流、挑战和目标故障必须可区分。策略解释 SHALL 指明 Profile、时效、阈值和 slot 选择结果

#### Scenario: 服务限流不等于节点失败
- **WHEN** 流媒体或 AI 目标返回限流或挑战
- **THEN** 保存服务级分类并保留独立的节点连通性结论

## MODIFIED Requirements

### Requirement: Node Probe Results Persistence
系统 SHALL 将每次探测终态追加为不可变观测，使用节点、Profile 和 Job logical ID 关联；不得按名称、地址或缓存覆盖历史结果，也不得保存节点连接秘密

#### Scenario: Saving probe outcome
- **WHEN** 节点探测完成并产生终态
- **THEN** 系统追加带时间、Profile 和已脱敏分类的观测记录

### Requirement: Authorized Periodic Background Node Probing
系统 SHALL 只为已保存且 enabled 的 ProbeProfile 按周期创建有预算 ProbeJob，不能直接遍历库存或覆盖缓存。每个调度/派发点必须检查 Profile 当前版本、状态与预算

#### Scenario: Running background probing
- **WHEN** enabled Profile 到达周期且预算可用
- **THEN** 系统创建可查询 Job 并在预算内处理节点

#### Scenario: Profile is disabled after scheduling
- **WHEN** Job 已排队但 Profile 被禁用、变更、删除或预算耗尽
- **THEN** Job 不执行新的网络尝试，终态指出授权撤销或预算耗尽

### Requirement: Capability-Based Filtering in Compiled Policies
系统 SHALL 依据明确的 Profile、时效、速度、服务能力和 selector slot 绑定筛选节点，并解释过期、失败、未测或不匹配原因

#### Scenario: Filtering by speed and unlock targets
- **WHEN** 策略要求指定 Profile 的近期成功观测、速度与多个服务能力
- **THEN** 编译器仅包含全部条件满足的节点，并解释每个排除原因

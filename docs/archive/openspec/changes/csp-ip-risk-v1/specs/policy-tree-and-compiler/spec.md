## MODIFIED Requirements

### Requirement: 策略树与准入规则
系统 SHALL 允许策略组引用直接节点、其他策略组、手动包含和排除条件以及命名的准入规则。每个策略组 MUST 使用稳定逻辑 ID；解析 MUST 检测环、缺失引用与不允许的自引用，并拒绝保存或发布无效策略图。策略组可以显式关联一个已激活的 IP risk policy revision；解析 MUST 使用该 revision 的确定性决策排除 `block` 节点，并依该 policy 的未知和 review 规则处理其余节点。无关联或未激活的风险 policy MUST 不改变现有策略组成员语义。

#### Scenario: 循环策略组
- **WHEN** 管理者保存包含 A → B → A 的策略组关系
- **THEN** 系统返回可定位到相关逻辑 ID 的校验错误且不发布该变更

#### Scenario: 风险 policy 拒绝成员
- **WHEN** 已绑定的风险 policy 将策略组成员判为 `block`
- **THEN** 解析快照排除该成员并提供脱敏 diagnostics，且不会改写节点台账

### Requirement: 单一确定性解析快照
系统 SHALL 从指定配置修订、活动节点台账和筛选器版本产生带内容摘要的确定性解析快照。预览、导出、策略调试和发布 MUST 使用同一个解析器和同一快照语义；同一输入重算 MUST 产生相同顺序与相同内容摘要。对于绑定风险 policy 的解析，快照 MUST 绑定 policy revision、参与观察 digest 和风险决策摘要，使风险准入可重现和审计。

#### Scenario: 预览与导出一致
- **WHEN** 管理者预览某个已验证配置修订后立即导出同一目标
- **THEN** 两者引用相同解析快照摘要，且节点和策略成员集合一致

#### Scenario: 风险观察未改变
- **WHEN** 输入节点、policy revision 和当前有效风险观察 digest 均未改变
- **THEN** 重算解析快照得到相同成员顺序、风险决策摘要和内容摘要

### Requirement: 不可变发布物与令牌范围
系统 SHALL 为每次成功发布创建包含目标、解析快照摘要、编译器版本、创建时间、内容摘要和状态的不可变发布物。导出 URL MUST 指向明确发布物；撤销后 MUST 不再返回内容。发布令牌 MUST 被限定至单个发布物或明确最小集合。创建发布物前 MUST 执行绑定 risk policy 的 preflight；preflight 拒绝时 MUST 不创建部分 publication、不得替换现有发布物且不得静默删改节点。

#### Scenario: 撤销发布物
- **WHEN** 管理者撤销一个发布物
- **THEN** 原导出 URL 此后返回已撤销结果，且不得自动回退到最近发布物

#### Scenario: 风险拦截发布
- **WHEN** publication preflight 发现被绑定 policy 阻断的节点
- **THEN** 系统拒绝创建该 publication 并返回脱敏 diagnostics，既有发布物保持不变

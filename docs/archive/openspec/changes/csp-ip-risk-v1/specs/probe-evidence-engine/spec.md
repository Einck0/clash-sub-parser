## MODIFIED Requirements

### Requirement: 证据化能力观察
系统 SHALL 为每个探测步骤保存探测配置版本、开始和完成时间、超时或网络错误类别、HTTP 状态或握手摘要、延迟、出口观察与经脱敏处理的判定证据。能力结论 MUST 采用 `available`、`restricted`、`unknown`、`error` 或 `stale`，不得将挑战页、契约漂移、认证页、DNS 失败或超时解释为 `available`。`ip_risk` MUST 是与 baseline、geo、streaming、AI 和 speed 分离的版本化探测类型；它的 provider 风险结论不得改写其他探测类型的 verdict。

#### Scenario: 平台响应不符合预期契约
- **WHEN** 平台响应无法匹配当前探测配置版本的可用性判定契约
- **THEN** 结果标为 `unknown` 或 `error` 并保存漂移证据，而非标为 `available`

#### Scenario: IP 风险 provider 契约漂移
- **WHEN** IP risk provider 响应不能匹配当前 provider schema version
- **THEN** 系统记录独立的 IP risk `unknown` 或 `error` 观察，且不改变该节点已有的 geo 或 capability verdict

### Requirement: 真正经被测节点的出口与能力请求
系统 MUST 使出口 IP、地理与平台请求经被测节点构建的内存代理管道发送；系统代理、直连连接或其他节点的连接不得充当被测节点证据。每个结果 MUST 能关联到节点 `logical_id` 和该执行的连接配置摘要。IP risk provider 请求同样 MUST 经该节点通道执行，并显式忽略宿主 `HTTP_PROXY`、`HTTPS_PROXY` 和 `ALL_PROXY`。

#### Scenario: 本机代理可用但被测节点失败
- **WHEN** 本机环境代理可访问目标而被测节点无法建立探测管道
- **THEN** 结果记录被测节点失败，且不得借用本机代理给出可用结论

#### Scenario: IP risk 请求遇到宿主代理配置
- **WHEN** 宿主进程设置了 HTTP proxy 环境变量
- **THEN** IP risk 请求仍仅使用被测节点 outbound，环境变量不会改变出口证据

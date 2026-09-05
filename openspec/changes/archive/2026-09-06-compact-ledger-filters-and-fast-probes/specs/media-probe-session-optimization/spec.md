## Purpose

在保持真实节点出站和证据级判定的前提下，减少媒体探测重复连接与 Netflix 慢路径，使探针在固定预算内更快返回可审计结果。

## ADDED Requirements

### Requirement: Shared isolated media session
对单个已通过传输握手的节点，系统 SHALL 在该节点专属本地回环代理上创建一个共享媒体 HTTP 会话，并用于该节点同次请求中的所有已启用平台。会话 SHALL 禁用宿主环境代理并固定到该节点代理；任何平台不得改用主机直连。

#### Scenario: Multiple platforms reuse one node session
- **WHEN** 一个健康节点同时探测两个或以上媒体平台
- **THEN** 所有平台请求 SHALL 使用同一节点专属 HTTP 会话和同一回环代理，同时每个平台的结果仍独立返回

#### Scenario: Transport failure avoids media session
- **WHEN** 节点传输握手失败或超时
- **THEN** 系统 SHALL 不创建媒体 HTTP 会话或发送媒体请求，并返回既有的节点失败结果

### Requirement: Bounded media-stage deadline
系统 SHALL 将已配置的媒体超时作为整个媒体阶段的壁钟预算。每个平台在共享的绝对截止时间内运行；超时、运输错误或不确定结论 SHALL 仅影响该平台结果，不得覆盖已完成的兄弟结果或将节点存活状态降级。

#### Scenario: One provider consumes remaining budget
- **WHEN** 一个媒体平台在媒体阶段截止时间前未完成而另一平台已返回确认结果
- **THEN** 前者 SHALL 返回结构化 timeout，后者结果 SHALL 被保留，节点传输状态保持由握手结果决定

#### Scenario: Media timeout configuration is effective
- **WHEN** 操作者保存新的媒体超时配置并启动下一次节点探测
- **THEN** 该探测的媒体阶段 SHALL 受该配置约束，而不是受无关默认值替代

### Requirement: Safe Netflix fast conclusion with evidence fallback
系统 SHALL 优先使用凭据无关的 Netflix CDN 快速响应判断完整区域解锁或 IP 阻断。仅当快速响应包含可解析的国家代码或明确 HTTP 403 阻断时，系统才可提前给出相应结论；其他响应、空数据或解析失败 SHALL 回退到既有的标题证据探测，并维持现有部分、受限、挑战、限流和不确定分类。

#### Scenario: Fast endpoint yields country
- **WHEN** Netflix 快速响应包含非空国家代码
- **THEN** 系统 SHALL 返回已确认完整 Netflix 解锁及该规范化国家代码，而无需发送标题回退请求

#### Scenario: Fast endpoint is inconclusive
- **WHEN** Netflix 快速响应缺失有效国家代码、返回非结论状态或无法解析
- **THEN** 系统 SHALL 执行标题证据回退，且不得仅因快速路径不确定而声明解锁、受限或阻断

#### Scenario: Fast endpoint reports IP block
- **WHEN** Netflix 快速响应为 HTTP 403
- **THEN** 系统 SHALL 返回 Netflix IP 阻断分类，并不得发送额外标题请求

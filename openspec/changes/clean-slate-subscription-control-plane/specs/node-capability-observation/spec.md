## MODIFIED Requirements

### Requirement: 真实出口与地理身份观测
系统 SHALL 保存经被测节点获得的出口 IP、ISO 国家、国旗、ASN 与组织信息，并以多 provider 结果形成共识或不一致状态。宿主出口和未绑定节点的请求 MUST 不能成为节点地理结论

#### Scenario: 多源地理结果不一致
- **WHEN** 地理 provider 对同一经节点出口给出不同结果
- **THEN** 系统记录可解释的不一致状态及各脱敏摘要，不伪造单一国家结论

### Requirement: 媒体与 AI 能力结果
系统 SHALL 将 YouTube Premium、Netflix、Disney+、Bilibili、ChatGPT、Claude、Gemini、Meta AI 等结果保存为服务级能力状态，并保留测试时间、探测配置和证据分类。能力状态 MUST 与节点存活状态分离

#### Scenario: Netflix 仅自制剧
- **WHEN** 节点能访问 Netflix 但仅返回自制剧能力
- **THEN** 系统标记 Netflix 的对应受限分类，而非标记节点为失败

### Requirement: 受控测速观测
系统 SHALL 在单节点/全局并发、字节和时长上限内进行分块下载测速，并持久化平均/峰值 Mbps 与测试状态。超时或预算限制 MUST 有明确结果而非写入误导性零速度

#### Scenario: 达到测速字节上限
- **WHEN** 下载达到配置的最大字节数
- **THEN** 系统停止下载、计算已采样吞吐并标记为受控完成

### Requirement: 观测可供台账和编译解释
每份观测 SHALL 可按节点、时间和探测配置查询，向 NodeLedger 提供公开摘要，并向策略编译提供阈值/能力判断原因，不泄露连接秘密

#### Scenario: 查看节点能力
- **WHEN** NodeLedger 请求一个节点的最新探测摘要
- **THEN** 页面获得延迟、速度、出口、旗帜和能力状态，并可追溯其探测时间
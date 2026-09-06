## Purpose

在得到用户对第三方 API 使用和出站 IP 数据披露的明确授权后，安全记录出口 IP 的风险观察，同时绝不把风险分数混入媒体能力或订阅筛选语义。

## ADDED Requirements

### Requirement: Explicitly authorized IP risk observation

系统 SHALL 默认不向第三方 IP 风险服务发送出口 IP。仅当部署已通过受部署秘密管理的已授权 API 凭据配置服务、操作者明确启用 IP Risk 且用户已确认第三方查询数据披露时，系统才可执行一次 IP Risk 观察。实现 MUST 使用供应商已授权的 API 契约；不得抓取或解析公共网页作为 API 替代。

#### Scenario: Authorization absent
- **WHEN** API 凭据、部署配置、操作者开关或用户确认中任一项缺失
- **THEN** 系统 SHALL 不发起第三方 IP Risk 请求，并以 `disabled` 或等价的未授权观察说明原因

#### Scenario: Authorized API observation
- **WHEN** 已配置获授权的 API 凭据且 IP Risk 已显式启用
- **THEN** 系统 SHALL 经待测节点获得的已验证出口 IP 向已批准服务发起受超时限制的 API 查询，并保存可审计的风险观察

### Requirement: IP risk is advisory and isolated

IP Risk 结果 SHALL 作为独立观察保存，包含规范化的 0 至 100 风险分、等级、观测时间、提供商版本或分类以及脱敏错误状态。它 MUST NOT 被表示为 IP 纯净度、欺诈事实、账户安全保证或流媒体、AI 服务完整解锁。现有媒体能力筛选、节点组过滤、订阅生成和健康统计 MUST 忽略 IP Risk，除非未来经单独规格变更明确引入该语义。

#### Scenario: Risk score exists with media capability filter
- **WHEN** 节点存在高或低 IP Risk 观察且订阅仅按媒体能力过滤
- **THEN** 订阅生成 SHALL 只使用既有媒体能力判定，不得因风险分改变节点的入选或排除结果

#### Scenario: Risk provider failure
- **WHEN** 已授权的风险 API 超时、限流、拒绝请求或返回不可识别数据
- **THEN** 系统 SHALL 保存结构化的非成功观察而不重试到无界程度、不降级节点传输健康状态，且不得回退到网页抓取

### Requirement: IP risk privacy and secret boundary

IP Risk 观察 MUST 只在身份观察已通过多提供商共识确认出口 IP 后才执行。API 凭据仅可存在于部署秘密存储，不得出现在设置读取响应、数据库观察、前端状态、日志、异常文本或诊断证据中。对外详情 MUST 不返回完整第三方响应 body、请求 URL、cookie、authorization、节点凭据或代理 URL。

#### Scenario: Observation detail is displayed
- **WHEN** 操作者查看节点的 IP Risk 详情
- **THEN** 响应 SHALL 只显示风险分、等级、观测时间、脱敏提供商状态及允许的无秘密证据字段

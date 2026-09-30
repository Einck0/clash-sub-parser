## Purpose

定义 CSP 1.0 控制面管理接口与订阅导出端点的基准鉴权协议契约，涵盖基于 Bearer Token / 会话 Cookie 的双重管理认证基准、发布专属令牌防越权强隔离门禁、以及对外标准订阅导出路径规范。

## ADDED Requirements

### Requirement: 控制面管理端受控基准认证
系统 SHALL 在开启管理保护时对所有 `/api/v1/` 控制面管理接口强制执行凭据校验，支持请求头中的 Bearer 令牌或经过验证的会话 Cookie（`csp_session`）。对于未携带有效凭据或凭据不匹配的请求，系统 MUST 拒绝该请求并返回 HTTP 401 Unauthorized 与标准错误码 `unauthorized`，且 MUST NOT 暴露底层敏感堆栈或系统内部数据。

#### Scenario: 有效管理令牌放行
- **WHEN** 客户端在 `Authorization` 头中携带匹配的管理 Bearer 令牌请求 `/api/v1/nodes` 等管理接口
- **THEN** 系统响应 200 OK 并正常返回业务数据，请求上下文被赋予管理员身份

#### Scenario: 缺失凭据请求被拦截为 401
- **WHEN** 客户端以匿名状态未携带任何凭据请求 `/api/v1/subscriptions` 等受控管理接口
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误信息 `Authentication credentials required`

#### Scenario: 错误凭据请求被拦截为 401
- **WHEN** 客户端在 `Authorization` 头中携带不匹配的令牌请求管理接口
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误信息 `Invalid authentication credentials`

### Requirement: 发布导出令牌管理越权强隔离
管理端处于受控模式时，系统 MUST 对使用发布导出令牌（Publication Export Token）访问 `/api/v1/` 管理端接口的行为实施严格的安全隔离拦截。当请求携带的令牌匹配任意有效发布导出专属令牌时，系统 MUST 拦截该请求并返回 HTTP 403 Forbidden 与稳定错误码 `invalid_token_scope`，绝不允许发布导出凭据在受控模式下越权调用控制面。管理端处于开放模式时，系统 MUST 无条件放行请求；此时管理员权限来自明确的开放配置或尚未配置主令牌，而不是发布令牌本身。导出保护开关不改变管理端的模式判定。

#### Scenario: 发布导出令牌访问管理端被 403 拒绝
- **WHEN** 管理端处于受控模式，客户端使用有效的专属发布导出令牌请求 `/api/v1/publications` 或 `/api/v1/policies/global-node-filter` 等管理接口
- **THEN** 系统返回 403 Forbidden 与错误码 `invalid_token_scope`，且不执行业务查询或状态变更

#### Scenario: 管理端开放模式无条件放行
- **WHEN** 管理端鉴权开关关闭（或没有配置主令牌），客户端匿名访问或携带任意令牌访问管理接口
- **THEN** 系统直接以开放模式管理员身份放行，不因无效令牌或发布令牌返回 401/403；导出鉴权开关保持独立

#### Scenario: 管理员令牌访问管理端正常放行
- **WHEN** 管理端受控模式下客户端使用合法的管理端令牌发起管理请求
- **THEN** 系统判定令牌作用域合法并正常进入管理业务处理链路

### Requirement: 标准订阅导出路径与不可变内容服务基准
系统 SHALL 在标准路径 `/publish/v1/{publication_id}` 对外提供客户端订阅配置拉取服务。对于已成功发布且处于激活（active）状态的配置包，系统 MUST 返回不可变二进制或纯文本配置内容，并携带响应头 `Cache-Control: no-store`、匹配的 `Content-Type` 与内容摘要 `ETag`。

#### Scenario: 标准导出路径获取订阅内容成功
- **WHEN** 客户端请求合法的 `/publish/v1/{publication_id}` 且鉴权校验通过
- **THEN** 系统返回 200 OK、不可变配置内容及对应的配置格式 Content-Type 与 ETag 摘要头

#### Scenario: 不存在的 publication_id 响应 404
- **WHEN** 客户端请求未在系统注册或已被物理清理的 publication_id
- **THEN** 系统返回 404 Not Found 与错误码 `publication_not_found`

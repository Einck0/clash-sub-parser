## Purpose

定义 CSP 1.0 控制面管理接口的鉴权双模态生命周期契约，涵盖未配置令牌时的开箱即用开放模式、配置令牌时的严格保护模式、导出令牌防越权隔离门禁，以及供前端与客户端安全探测运行模态的公共接口。

## ADDED Requirements

### Requirement: 鉴权双模态与空配置自动开放
系统 SHALL 在服务端未配置管理令牌（环境变量 `CSP_ADMIN_TOKEN` 为空或未设置）时自动运行于零配置开放模式（Zero-Config Open Mode）。在此模式下，任何未携带 Bearer 令牌或 Cookie 的管理 API 请求 MUST 自动获得管理员权限（`ActorKindAdmin`、`Subject: "admin"`、`AuthMethod: "open_mode"`、`Scopes: ["admin"]`），且 MUST 不返回 401 错误。当服务端配置了非空管理令牌时，系统 MUST 运行于受控保护模式（Token Mode），严格执行 Bearer 令牌与会话 Cookie 校验，无凭据或凭据不匹配的请求 MUST 被拦截为 401。

#### Scenario: 开放模式下匿名管理请求成功
- **WHEN** 服务端未配置 AdminToken 且客户端发起未带凭据的 `/api/v1/` 管理请求
- **THEN** 系统以 HTTP 成功状态响应并返回 `{ "data": ... }`，请求上下文被赋予管理员身份

#### Scenario: 保护模式下匿名管理请求被拦截
- **WHEN** 服务端配置了非空 AdminToken 且客户端发起未带凭据的 `/api/v1/` 管理请求
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误消息 `Authentication credentials required`

#### Scenario: 保护模式下匹配 Bearer 令牌成功放行
- **WHEN** 服务端配置了非空 AdminToken 且客户端在 `Authorization` 头携带匹配的 `Bearer <token>` 发起管理请求
- **THEN** 系统以 HTTP 成功状态响应并正常返回业务数据

#### Scenario: 保护模式下错误 Bearer 令牌被拒绝
- **WHEN** 服务端配置了非空 AdminToken 且客户端在 `Authorization` 头携带不匹配的 Token 发起管理请求
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误消息 `Invalid authentication credentials`

### Requirement: 导出发布令牌管理端越权隔离
系统 MUST 在零配置开放模式与受控保护模式下，均对携带发布导出令牌（Publication Export Token）访问 `/api/v1/` 管理端接口的行为执行强隔离拦截。当请求携带的令牌经系统判定为发布导出令牌时，系统 MUST 拒绝该请求并返回 403 Forbidden 与稳定错误码 `invalid_token_scope`，且 MUST NOT 赋予任何管理权限或泄露管理端资源存在性。

#### Scenario: 开放模式下发布导出令牌越权管理被拒绝
- **WHEN** 系统处于开放模式且调用者使用有效发布导出令牌请求 `/api/v1/nodes` 等管理接口
- **THEN** 系统返回 403 Forbidden 与错误码 `invalid_token_scope`，且不执行业务查询

#### Scenario: 保护模式下发布导出令牌越权管理被拒绝
- **WHEN** 系统处于保护模式且调用者使用有效发布导出令牌请求 `/api/v1/subscriptions` 等管理接口
- **THEN** 系统返回 403 Forbidden 与错误码 `invalid_token_scope`，且不执行业务查询

### Requirement: 公共鉴权模态查询端点
系统 SHALL 提供免认证查询端点 `GET /api/v1/auth/mode`，该端点 MUST 在鉴权中间件拦截前被放行。该端点 MUST 返回 `{ "data": { "mode": "open" | "token" } }`，用于客户端在初始化时确定系统当前安全水位与是否需要呈现凭据输入提示，且该端点 MUST NOT 泄露管理令牌的真实值或哈希。

#### Scenario: 开放模式下查询鉴权模态
- **WHEN** 客户端以匿名状态向 `GET /api/v1/auth/mode` 发起请求，且服务端未配置 AdminToken
- **THEN** 系统返回 200 OK 且响应体为 `{ "data": { "mode": "open" } }`

#### Scenario: 保护模式下查询鉴权模态
- **WHEN** 客户端以匿名状态向 `GET /api/v1/auth/mode` 发起请求，且服务端已配置 AdminToken
- **THEN** 系统返回 200 OK 且响应体为 `{ "data": { "mode": "token" } }`

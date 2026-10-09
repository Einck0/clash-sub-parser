## Purpose

定义 CSP 1.0 管理端鉴权与导出链接鉴权开关的解耦契约、统一系统令牌（System Master Token）的生命周期管理、数据库持久化字段以及 `/api/v1/settings/auth` API 交互规范。

## ADDED Requirements

### Requirement: 鉴权开关独立持久化与领域解耦
系统 SHALL 在 SQLite 数据库 `settings` 表中通过迁移 `000013_auth_settings.sql` 新增 `admin_auth_enabled INTEGER NOT NULL DEFAULT 1` 与 `export_auth_enabled INTEGER NOT NULL DEFAULT 1` 两个独立配置字段。领域层 `domain.Settings` 与仓储层 `SettingsRepository` MUST 提供独立读取与更新管理端鉴权开关和导出链接鉴权开关的能力，二者状态在存储层面与是否存在密码哈希完全解耦。

#### Scenario: 默认初始化状态为管理端与导出端均受控
- **WHEN** 数据库执行迁移并初始化默认设置或首次读取 settings 记录
- **THEN** 系统解析 `admin_auth_enabled` 为 true 且 `export_auth_enabled` 为 true，保持高安全默认水位

#### Scenario: 独立持久化并更新开关状态
- **WHEN** 管理员通过持久化接口单独将 `export_auth_enabled` 置为 false 并保留 `admin_auth_enabled` 为 true
- **THEN** 数据库精准更新对应列，后续查询返回更新后的开关状态且两者互不影响

### Requirement: 动态鉴权模型独立判定与 Zero-Config 开放受控
系统 SHALL 在 `DynamicTokenHolder` 中解耦管理端鉴权判定：管理端运行模式 `AdminMode()` MUST 优先遵循 `admin_auth_enabled` 开关。当 `admin_auth_enabled == false` 时，管理端 MUST 强制运行于 Zero-Config Open Mode，无论内存或数据库中是否存在有效令牌，匿名与任意请求均直接赋予管理员身份放行；当 `admin_auth_enabled == true` 时，系统依据是否存在有效令牌决定运行于受控保护模式（Token Protected Mode）或未配置令牌时的零配置开放模式。

#### Scenario: 管理端开关关闭时即使存在令牌仍免密放行
- **WHEN** 系统已配置有效管理令牌但管理员将 `admin_auth_enabled` 显式设置为 false
- **THEN** 客户端匿名访问、携带无效令牌或发布专属令牌访问管理接口均被直接放行并赋予管理员权限，无 401/403 拦截；导出开关状态不受影响

#### Scenario: 管理端开关开启且存在令牌时严格校验凭证
- **WHEN** 系统 `admin_auth_enabled` 为 true 且已配置有效令牌
- **THEN** 匿名管理请求被拦截为 401，携带匹配令牌或会话的请求正常放行

#### Scenario: 管理端开关开启但未配置令牌时回退至 Zero-Config 开放模式
- **WHEN** 系统 `admin_auth_enabled` 为 true 但服务端从未配置过令牌且数据库 token 为空
- **THEN** 系统自动运行于零配置开放模式，放行请求并在状态接口提示未配置凭据

### Requirement: 统一系统令牌复用与管理
系统 SHALL 支持配置单一全局统一系统令牌（System Master Token），该令牌可同时用于管理端访问认证以及导出链接下载鉴权。系统更新或清空统一令牌时，MUST 使用恒定时间对比与安全哈希（Bcrypt / SHA-256）持久化至数据库，并发安全刷新内存中 `DynamicTokenHolder` 的验证器快照，并立即使所有既有管理 Cookie 会话失效。

#### Scenario: 更新统一系统令牌成功生效
- **WHEN** 管理员提交新的统一系统令牌字符串
- **THEN** 系统完成安全持久化与快照刷新，后续管理端与开启导出鉴权时的拉取请求均可使用该新令牌通过认证

#### Scenario: 清空系统令牌安全回退
- **WHEN** 管理员提交空字符串清空统一系统令牌
- **THEN** 系统清空数据库中的令牌哈希并重置内存验证器，吊销所有当前已登录会话

### Requirement: 鉴权设置统一 API 契约与向后兼容
系统 SHALL 在 `/api/v1` 路由提供 `GET /api/v1/settings/auth` 与 `PUT /api/v1/settings/auth` 接口，用于统一查询与原子变更鉴权开关及系统令牌。该接口 MUST 兼容既有 `/api/v1/settings/admin-token`（`GET/POST`）及 `/api/v1/auth/status` 端点，且在任何响应中 MUST NOT 返回令牌明文或哈希原串。

#### Scenario: 查询全局鉴权开关与令牌配置状态
- **WHEN** 客户端以管理员身份请求 `GET /api/v1/settings/auth`
- **THEN** 系统返回 200 OK 与 JSON 数据，包含 `admin_auth_enabled`、`export_auth_enabled`、`admin_mode`、`export_mode` 以及 `token_configured` 布尔标记

#### Scenario: 原子更新鉴权开关与统一令牌
- **WHEN** 客户端向 `PUT /api/v1/settings/auth` 提交包含鉴权开关与可选新令牌的有效载荷
- **THEN** 系统原子更新数据库与内存状态，记录审计事件并返回更新后的生效状态

#### Scenario: 兼容调用旧版 admin-token 接口
- **WHEN** 旧版管理脚本向 `POST /api/v1/settings/admin-token` 发送请求
- **THEN** 系统正常更新统一系统令牌，更新后的状态同步反映至 `GET /api/v1/settings/auth`

### Requirement: 前端设置中心独立开关与凭据管理
Web 前端在 `SettingsView.vue` SHALL 分别渲染“管理端访问保护”与“订阅导出链接保护”两个独立开关组件，并提供统一系统令牌的录入、修改、测试与清除功能。界面 MUST 动态展示当前管理端生效模态（受控/免密）与导出链接保护状态（受控/公开），向管理员直观揭示系统安全水位。

#### Scenario: 前端展示独立的管理端与导出端开关
- **WHEN** 用户进入“设置中心”视图
- **THEN** 界面清晰呈现管理端鉴权开关、导出链接鉴权开关、当前有效安全模态徽章及统一令牌卡片

#### Scenario: 前端切换开关并持久化成功
- **WHEN** 用户切换任意鉴权开关并点击保存
- **THEN** 前端调用 `PUT /api/v1/settings/auth` 提交变更，收到成功响应后弹出操作提示并刷新安全徽章

## Purpose

定义 CSP 1.0 订阅发布端点的免密公开导出、专属令牌与统一系统令牌双重鉴权验证、`/p/...` 路径兼容与发布向导故障恢复交互契约。

## ADDED Requirements

### Requirement: 导出鉴权关闭时的免密公开拉取
当系统 `export_auth_enabled` 设置为 false 时，订阅发布端点 `/publish/v1/{publication_id}` MUST 无条件允许客户端拉取已激活的不可变编译配置，无需在 URL 查询参数或 `Authorization` 请求头中提供任何令牌凭据。无论客户端未携带令牌还是携带无效令牌，系统 MUST NOT 因鉴权返回 401 Unauthorized；不存在或已撤销的发布实体仍按实体状态返回相应错误。管理端鉴权开关独立生效。

#### Scenario: 导出鉴权关闭时匿名客户端直接拉取配置成功
- **WHEN** 全局配置 `export_auth_enabled` 为 false 且客户端以未带 token 的 GET 请求访问已发布的 `/publish/v1/{id}`
- **THEN** 系统响应 200 OK 并直接输出该发布对应的二进制或文本配置流

#### Scenario: 导出鉴权关闭时携带无效令牌仍提供配置
- **WHEN** 全局配置 `export_auth_enabled` 为 false，客户端在 URL 的 `token` 参数或 Bearer 请求头中携带无效令牌访问已激活发布
- **THEN** 系统忽略该凭据并响应 200 OK 提供配置，不因无效令牌返回 401

### Requirement: 导出鉴权开启时的双重令牌验证
当系统 `export_auth_enabled` 设置为 true 时，订阅导出端点 MUST 严格执行客户端身份认证。系统 MUST 依次允许以下两种有效凭据之一通过鉴权：
1. 请求携带的令牌与该 publication 专属的 `token_hash` 经恒定时间对比完全一致；
2. 请求携带的令牌与系统当前配置的全局统一系统令牌（System Master Token）完全一致。
对于已激活的发布实体，凭据缺失或既不匹配专属令牌也不匹配统一系统令牌时，系统 MUST 拒绝请求并响应 HTTP 401 Unauthorized 与错误码 `unauthorized`。此判定不依赖 `admin_auth_enabled`；不存在的发布仍返回 404。

#### Scenario: 携带 publication 专属令牌拉取成功
- **WHEN** 导出鉴权开启且客户端使用该发布生成时对应的专属 `pub_<hex>` 令牌发起导出请求
- **THEN** 系统校验专属令牌哈希成功并响应 200 OK 交付配置内容

#### Scenario: 携带系统统一主令牌拉取成功
- **WHEN** 导出鉴权开启且客户端使用管理端配置的全局统一系统令牌发起导出请求
- **THEN** 系统通过统一令牌验证器校验成功并响应 200 OK 交付配置内容，实现令牌复用

#### Scenario: 携带错误令牌或无凭据时被拦截为 401
- **WHEN** 导出鉴权开启且客户端发起未带 token 的请求或使用了无效的随机字符串
- **THEN** 系统拒绝请求并返回 401 Unauthorized 与错误码 `unauthorized`

### Requirement: 历史短路径 `/p/{publication_id}` 兼容恢复
系统路由层 MUST 识别并支持客户端对 `/p/{publication_id}` 短路径发起的订阅拉取请求，将其无缝对齐至 `/publish/v1/{publication_id}` 的处理逻辑或返回安全的 307 Temporary Redirect 重定向，杜绝因客户端使用历史简化路径而导致的无意义 HTTP 404 故障。

#### Scenario: 请求历史短路径 /p/{id} 成功获取发布内容或正确重定向
- **WHEN** 客户端向 `/p/{id}` 发起带有合规凭据（或免密模式下匿名）的 GET 请求
- **THEN** 系统成功响应对应发布内容或返回指向 `/publish/v1/{id}` 的 307 重定向并保留原有查询参数

#### Scenario: 不存在的 publication_id 在 /p/{id} 返回 404
- **WHEN** 客户端向 `/p/{non_existent_id}` 发起请求且数据库中不存在该发布记录
- **THEN** 系统返回 404 Not Found 与错误码 `publication_not_found`

### Requirement: 零发布记录诊断与工作台发布向导对齐
当数据库 `publications` 表当前有效记录数为 0 时，系统与前端 MUST 给出清晰的状态解释与指引。Web 前端在 `PublicationsView.vue` 中 MUST 区分“没有生成发布”与“系统网络故障”，向用户展示醒目的发布引导卡片，明确告知需要点击“发布当前配置”以生成首个不可变订阅发布实体与下载端点。

#### Scenario: 首次进入无发布记录时展示清晰发布向导
- **WHEN** 管理员打开发布管理视图且当前 target 没有任何已发布的记录
- **THEN** 界面展示空状态向导，提示当前尚未注册不可变发布，并提供引导点击发布操作

#### Scenario: 点击发布后即时生成新发布并呈现正确链接
- **WHEN** 管理员点击“发布当前配置”并确认
- **THEN** 系统成功创建 publication 记录并在界面呈现新的导出卡片与导出地址

### Requirement: 前端导出链接与复制行为动态适配
Web 前端在 `PublicationsView.vue` 及 `usePublications.ts` 中 MUST 根据当前生效的 `export_auth_enabled` 配置动态格式化客户端订阅地址：
- 当 `export_auth_enabled == false` 时，生成的直链 MUST 为纯净免密地址格式：`http(s)://<host>/publish/v1/{id}`；
- 当 `export_auth_enabled == true` 时，生成的直链 MUST 自动追加令牌参数：`http(s)://<host>/publish/v1/{id}?token={token}`；
- 用户点击“复制订阅链接”按钮时，复制至剪贴板的内容 MUST 与当前导出鉴权模态完全一致，并在界面提示当前链接类型（公开直链或受控令牌链接）。

#### Scenario: 免密模式下复制纯净直链
- **WHEN** 导出鉴权处于关闭状态且用户点击复制当前订阅链接
- **THEN** 写入剪贴板的 URL 不包含 `?token=` 查询参数，界面提示已复制免密公开直链

#### Scenario: 受控模式下复制包含令牌的完整订阅链接
- **WHEN** 导出鉴权处于开启状态且用户点击复制当前订阅链接
- **THEN** 写入剪贴板的 URL 包含合规的 `?token=` 查询参数，界面提示已复制包含凭据的订阅链接

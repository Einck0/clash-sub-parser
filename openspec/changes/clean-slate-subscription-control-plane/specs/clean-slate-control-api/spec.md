## Purpose

提供无版本管理与最小暴露发布契约，确保控制台、自动化和四种订阅客户端不会获得不属于自己的节点或管理秘密

## ADDED Requirements

### Requirement: 固定无版本管理入口与旧路由拒绝
系统 SHALL 只在 `/api/control` 提供管理 API，所有资源使用 logical ID。系统不得发布、重定向或翻译旧数字 ID、`/api/**` 旧 CRUD、`/api/v1/**`、`/api/v2/**`、`/yaml`、`/script`、Script.js、下载或快照路径

#### Scenario: 访问旧兼容路由
- **WHEN** 调用方请求 legacy deny manifest 中任一路径和方法
- **THEN** 返回 404、无 Location、无领域命令、无数据库/Job/网络副作用

### Requirement: 范围受限发布 endpoint
系统 SHALL 仅提供 `GET /publish/{target}`，target 只能为 `clash`、`mihomo`、`stash`、`shadowrocket`。调用方必须使用具有单 target scope、expiry、撤销状态和不可逆存储 hash 的专用 Bearer 发布令牌；令牌不得访问管理 API，原始 token 仅在创建时向操作员显示一次

#### Scenario: 有效发布请求
- **WHEN** 有效、未过期且 scope 匹配的发布令牌请求当前 revision 的目标输出
- **THEN** 服务只返回该 target 的已选节点客户端必需连接材料、revision 与语义 fingerprint

#### Scenario: 无效或越权令牌
- **WHEN** token 缺失、过期、撤销或 target scope 不匹配
- **THEN** 服务拒绝请求且不泄露活动节点、管理数据或 token 状态细节

### Requirement: 管理和发布的秘密披露边界
管理 API SHALL 只返回公开节点摘要和 `secret_present`，不得返回节点 payload、来源 URL、来源认证、管理/发布 token、Probe 原始响应或未选节点。发布响应不得含来源 URL、来源认证、管理秘密、未选节点、Bundle 或 token。日志不得记录 Authorization 值、payload 或秘密

#### Scenario: 管理与发布响应脱敏
- **WHEN** 管理员查看库存或客户端获取发布内容
- **THEN** 前者只见公开摘要，后者只见当前 target 的已选连接材料，二者均不出现来源或管理秘密

### Requirement: 输出缓存隔离
服务端可以按 revision logical ID、target 和 semantic fingerprint 缓存纯编译结果，不得用 token 原文作缓存键。响应 SHALL 设置 `Cache-Control: private, no-store`

#### Scenario: 发布响应不可共享缓存
- **WHEN** 不同令牌先后请求同一 target
- **THEN** 服务可复用纯编译结果但响应不被共享缓存，日志不记录任一令牌原文

### Requirement: 统一命令与异步错误
管理命令与 Job 查询 SHALL 返回统一封套、稳定机器码、logical ID 和已脱敏诊断。Cookie 写操作 SHALL 需要 CSRF 证明

#### Scenario: 缺少 CSRF 的写入
- **WHEN** Cookie 会话发送缺少 CSRF 证明的管理写请求
- **THEN** 服务拒绝请求且不修改配置、库存或 Job

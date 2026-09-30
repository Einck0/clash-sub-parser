## Why

当前生产环境出现订阅发布请求 404 与拉取失败：经深度排查，生产 SQLite 数据库 `publications` 表当前为 0 条记录，客户端请求历史 `/p/...` 路径报 404（CSP 标准导出路径为 `/publish/v1/{publication_id}`）；且现有导出端点硬编码强制要求专属 `pub_<hex>` 令牌，不支持免密公开拉取，也不支持使用管理端配置的统一系统令牌。同时，管理端鉴权粗暴绑定于密码字符串是否存在，缺少独立控制开关，无法满足用户关于“管理端与导出链接鉴权开关相互独立、但可复用同一个统一令牌”的核心诉求。

## What Changes

- **鉴权持久化开关扩展**：新增数据库迁移 `000013_auth_settings.sql`，在 `settings` 表中扩展 `admin_auth_enabled`（默认 1）和 `export_auth_enabled`（默认 1）字段，使管理端鉴权与导出链接鉴权在数据库层面解耦持久化。
- **领域层与仓储契约升级**：扩展 `internal/domain/settings.go` 与 `internal/repository/sqlite/settings.go`，支持 `AdminAuthEnabled` 与 `ExportAuthEnabled` 状态读取、写入与向后兼容查询。
- **鉴权动态模型解耦**：重构 `internal/transport/http/token.go` (`DynamicTokenHolder`)，维护 `adminAuthEnabled` 与 `exportAuthEnabled` 状态，提供独立的 `AdminMode()` 与 `ExportMode()` 判定，支持管理端在配置了令牌的情况下仍可通过独立开关免密开放（Zero-Config Open Mode），以及导出端点独立判定是否要求凭证。
- **发布导出双模态强化与统一令牌复用**：改造 `internal/transport/http/publications.go` 中的 `publicationClientHandler`：当 `export_auth_enabled == false` 时，无条件放行匿名客户端公开拉取；当 `export_auth_enabled == true` 时，客户端既可凭 publication 专属 TokenHash 访问，亦支持使用管理端配置的统一系统令牌（复用同一个令牌），解决生产换发或多客户端共享令牌难题。
- **路由兼容性增强**：在 `internal/transport/http/router.go` 增设 `/p/{publication_id}` 兼容路由（重定向至 `/publish/v1/{publication_id}` 或直接处理），防止旧版客户端和历史外部订阅链接失效。
- **设置管理 API 扩展**：提供标准 `GET/PUT /api/v1/settings/auth` 接口，支持统一查询与原子修改鉴权开关及系统令牌；同时保持对既有 `/api/v1/settings/admin-token` 与 `/api/v1/auth/status` 的全兼容。
- **前端设置中心独立开关与状态指示**：在 `web/src/features/settings/SettingsView.vue` 落地管理端鉴权与导出链接鉴权独立开关交互，直观显示管理端安全水位与导出链接受控状态，统一维护共享系统令牌。
- **前端发布工作台引导与免密直链动态呈现**：在 `web/src/features/publications/PublicationsView.vue` 增强零记录时的发布引导提示，并根据导出鉴权开关动态呈现免密直链（不带 Token）或受控链接（带 Token），对齐一键发布与复制交互。

## Capabilities

### New Capabilities

- `baseline-auth-and-export`: 控制面管理端与订阅发布导出端点的基准鉴权协议契约，包括 Bearer Token / Cookie 会话验证、发布令牌管理端越权强隔离与标准 HTTP 导出路径基线。
- `decoupled-auth-settings`: 管理端与导出链接鉴权独立开关（`admin_auth_enabled`、`export_auth_enabled`）解耦、统一系统令牌管理、`000013_auth_settings.sql` 存储迁移及 `/api/v1/settings/auth` API 契约。
- `publication-export-auth-recovery`: 订阅发布端点免密公开访问放行、专属发布令牌与统一系统令牌双重鉴权验证、`/p/...` 路径兼容与发布向导故障恢复交互契约。

### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。

## Impact

- **数据持久层**：新增 `migrations/000013_auth_settings.sql`，安全修改 `settings` 表追加两列；更新 `internal/domain/settings.go`、`internal/domain/ports.go` 及 `internal/repository/sqlite/settings.go`。
- **控制面运行时**：`internal/transport/http/token.go` (`DynamicTokenHolder`)、`internal/transport/http/middleware_auth.go`、`internal/transport/http/router.go`、`internal/transport/http/auth.go`、`internal/transport/http/settings.go`。
- **订阅发布端点**：`internal/transport/http/publications.go` (`publicationClientHandler`)、`internal/application/publication/service.go`。
- **Web 前端**：`web/src/features/settings/SettingsView.vue`、`web/src/features/publications/PublicationsView.vue`、`web/src/features/publications/usePublications.ts`、相关单元测试与 i18n 语言包（`web/src/locales/`）。
- **外部兼容性**：完全向后兼容现有的 HTTP API 与 Docker 部署流程，默认延续双重受控模式（`DEFAULT 1`），不会引发已有已配置凭据服务的权限降级。

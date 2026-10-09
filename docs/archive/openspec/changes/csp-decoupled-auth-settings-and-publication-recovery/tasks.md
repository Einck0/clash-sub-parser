## 1. 数据库迁移与领域仓储持久层（Database Migration & Repository Layer）

- [x] 1.1 创建数据库迁移文件 `migrations/000013_auth_settings.sql`，执行 `ALTER TABLE settings ADD COLUMN admin_auth_enabled INTEGER NOT NULL DEFAULT 1` 和 `ALTER TABLE settings ADD COLUMN export_auth_enabled INTEGER NOT NULL DEFAULT 1`，通过 `go test ./internal/repository/sqlite/...` 验证迁移成功应用且默认值为 1。
- [x] 1.2 扩展 `internal/domain/settings.go` 与 `internal/domain/ports.go`，在 `Settings` 结构体中添加 `AdminAuthEnabled` 与 `ExportAuthEnabled` 字段，并在 `DefaultSettings()` 中赋予 `true` 默认值；通过 Go 单元测试断言结构体验证与字段初始化正确。
- [x] 1.3 改造 `internal/repository/sqlite/settings.go`，在 `Get` 与 `Update` 方法中支持 `admin_auth_enabled` 与 `export_auth_enabled` 的读写持久化，通过 `go test ./internal/repository/sqlite/...` 验证存取逻辑与数据一致性。

## 2. 鉴权模型解耦与控制面受控状态（Decoupled DynamicTokenHolder & Auth Engine）

- [x] 2.1 重构 `internal/transport/http/token.go` 中的 `DynamicTokenHolder`，引入 `adminAuthEnabled` 与 `exportAuthEnabled` 状态管理，提供独立的 `AdminMode()`、`IsExportAuthEnabled()` 及 `SetAuthSwitches` 方法，通过 `go test ./internal/transport/http/...` 验证在不同开关组合下的模式判定行为。
- [x] 2.2 调整 `internal/transport/http/middleware_auth.go` 及相关上下文处理，当 `DynamicTokenHolder.AdminMode() == SecurityModeOpen`（即 `admin_auth_enabled == false` 或无令牌）时确保管理端请求无条件作为管理员放行，通过单元测试验证在配置了令牌但关闭管理端鉴权时的免密放行行为。
- [x] 2.3 扩展 `internal/transport/http/auth.go` 中的 `AuthStatusData` 与 `authStatusHandler`，使其返回详细的管理端模式与导出鉴权状态，通过测试验证状态接口输出正确性。

## 3. 订阅导出端点双模态验证与短路径兼容（Publication Export Dual-Auth & Routing Recovery）

- [x] 3.1 改造 `internal/application/publication/service.go` 与 `internal/transport/http/publications.go` 中的 `publicationClientHandler`：在 `export_auth_enabled == false` 时允许免密直接拉取已激活发布内容；在 `export_auth_enabled == true` 时，优先校验系统统一主令牌，若不匹配再校验 publication 专属 TokenHash，两者皆不匹配或未传参时返回 401；通过 `go test ./internal/transport/http/...` 覆盖免密拉取、专属令牌拉取、统一系统令牌拉取及错误令牌拦截四种场景。
- [x] 3.2 在 `internal/transport/http/router.go` 挂载 `/p/{publication_id}` 历史兼容路由，确保请求该路径时与 `/publish/v1/{publication_id}` 享受相同的导出服务或平滑 307 重定向，通过针对性 HTTP 测试验证历史客户端请求不再报 404。

## 4. 鉴权管理 API 扩展与向后兼容（Settings Auth API & Compatibility）

- [x] 4.1 在 `internal/transport/http/settings.go` 实现 `GET /api/v1/settings/auth` 与 `PUT /api/v1/settings/auth` 路由处理函数，支持原子查询与修改管理端开关、导出链接开关以及系统统一令牌，并记录审计事件；保持对既有 `/api/v1/settings/admin-token` 的兼容转发；通过 `go test ./internal/transport/http/...` 验证 API 行为。
- [x] 4.2 运行全量后端测试 `go test ./...` 与静态编译 `go build ./...`，确保整个后端的类型系统、契约接口与现有测试套件全部通过（退出码 0）。

## 5. Web 前端设置中心与发布工作台改造（Frontend Settings & Publications Workflow）

- [x] 5.1 改造 `web/src/features/settings/SettingsView.vue`，实现管理端鉴权独立开关、导出链接鉴权独立开关以及系统统一令牌维护界面，对接 `GET/PUT /api/v1/settings/auth` API，并补齐 i18n 语言文案，通过组件测试 `SettingsView.test.ts` 验证表单交互与状态回显。
- [x] 5.2 改造 `web/src/features/publications/PublicationsView.vue` 与 `web/src/features/publications/usePublications.ts`：针对数据库 0 记录场景增设醒目的发布引导空状态；根据 `export_auth_enabled` 动态生成纯净直链（免密）或带 Token 链接（受控），对齐一键复制与下载行为，通过组件测试 `publications.test.ts` 验证链接生成与向导逻辑。
- [x] 5.3 运行前端类型检查 `npm run type-check` 与前端单元测试 `npm run test`，确保退出码为 0，无任何 TypeScript 语法或构建错误。

## 6. 综合质量门禁与端到端验证（Convergence Gates & E2E Validation）

- [x] 6.1 运行全项目统一静态编译 `go build ./...`、全量后端单元测试 `go test ./...` 以及前端类型检查 `npm run type-check`，确保全部退出码为 0。
- [x] 6.2 执行端到端或集成测试，完整模拟免密拉取、统一系统令牌拉取、专属令牌拉取、管理端独立免密以及 `/p/...` 历史短路径重定向全流程，验证整体系统功能完好。

## Why

当前生产环境 CSP 1.0 单二进制控制面（`sub.einck.top` / `127.0.0.1:18080`）处于 100% 鉴权死锁状态：由于 `internal/transport/http/middleware_auth.go:29` 仅在 `cfg.AdminToken != ""` 时校验通过，且对无凭据请求无条件拦截为 401，而 `docker-compose.yml:13` 默认配置 `CSP_ADMIN_TOKEN: ${CSP_ADMIN_TOKEN:-}` 为空，导致未配置 Token 时所有控制面请求均被阻断（`HTTP 401 Unauthorized: Authentication credentials required`）。同时，前端 `web/src/api/client.ts:169` 仅被动读取 `localStorage.getItem('csp_token')`，全仓无 Token 输入弹窗或凭据录入界面，`web/src/App.vue:183` 设置页仍为占位卡片；且前期 Critic 终审仅依靠 Mock 网络路由（`web/e2e/cross-device.spec.ts:6-64`）与单测硬编码 Token 产生虚假绿灯，从未对生产容器执行真实的匿名无头浏览器全链路质检。

本变更旨在建立工业级开放/受控双模态鉴权生命周期、前端全局凭据管理与 401 拦截弹窗，并制定对抗级真机无头浏览器质检门禁，彻底解决开箱死锁并确保可用性真实闭环。

## What Changes

- **后端鉴权双模态支持**：修改 `internal/transport/http/middleware_auth.go`。当 `cfg.AdminToken == ""`（未配置环境变量 `CSP_ADMIN_TOKEN`）时，自动运行于 Zero-Config Open Mode（开放私有模式），直接放行管理请求并赋予 Admin 上下文；当 `cfg.AdminToken != ""` 时，严格执行现有 Bearer / Cookie 鉴权逻辑。
- **导出令牌安全隔离硬门禁**：在任何模式下（包括 Open Mode），携带导出令牌（Publication Export Token）访问 `/api/v1/` 管理接口必须严格拦截为 403 Forbidden（`invalid_token_scope`），杜绝越权。
- **公共鉴权状态查询端点**：新增免认证端点 `GET /api/v1/auth/mode`，返回系统当前运行模态（`open` 或 `token`），供前端初始化时动态识别服务安全状态与展示引导。
- **前端全局 401 拦截与 AuthModal**：在 `web/src/api/client.ts` 增加 401 响应事件拦截钩子；在 `web/src/ui/AuthModal.vue` 实现响应式凭据录入弹窗，支持输入 Admin Token 并持久化至 `localStorage['csp_token']`，提交后自动刷新视图数据。
- **设置视图与凭据管理落地**：在 `web/src/features/settings/SettingsView.vue` 取代 `App.vue` 中的 Phase 6.1 占位卡片，提供鉴权模态展示、Token 查看/更新/清除与连接性验证。
- **顶部状态栏鉴权指示器**：在 `web/src/App.vue` 顶部导航栏增加鉴权状态徽章（如 `Open Mode` / `Protected`），直观呈现安全水位。
- **Critic 真机对抗质检门禁**：制定铁律级 E2E 质检规范，禁止在端到端质检中使用 `page.route` 伪造网络数据；必须使用无头浏览器（Playwright/CDP）针对真实运行态服务分别验证“匿名态（未注入 Token）”与“受控态（注入 Token）”，断言控制台零 401 报错且 DOM 正常渲染。

## Capabilities

### New Capabilities

- `control-plane-auth-lifecycle`: 定义后端 Zero-Config Open Mode 与 Protected Mode 双模态鉴权、发布令牌隔离与鉴权模态探测接口契约。
- `frontend-credential-management`: 定义前端全局 401 拦截、AuthModal 凭据录入、设置中心 Token 管理与顶部安全状态指示交互契约。
- `critic-headless-adversarial-verification`: 定义禁止 Mock 路由、双态真机无头浏览器对抗验证、控制台日志与 DOM 几何断言质检门禁规范。

### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。

## Impact

- **受影响后端组件**：`internal/transport/http/middleware_auth.go`（双模态放行与发布令牌拦截）、`internal/transport/http/router.go`（注册 `/api/v1/auth/mode` 路由）。
- **受影响前端组件**：`web/src/api/client.ts`（401 响应拦截与凭据存取钩子）、`web/src/ui/AuthModal.vue`（新增凭据录入组件）、`web/src/features/settings/SettingsView.vue`（新增设置视图）、`web/src/App.vue`（接入弹窗、设置页路由与安全徽章）。
- **受影响测试与质检工具**：`web/e2e/real-server.spec.ts`（新增真实后端无 Mock E2E 测试脚本）、Critic 验收工作流规范。
- **部署与配置影响**：完全向后兼容。`docker-compose.yml` 保持 `CSP_ADMIN_TOKEN: ${CSP_ADMIN_TOKEN:-}`，未配置时即刻恢复服务可用性；配置时安全加固，不影响既有业务数据与节点台账。

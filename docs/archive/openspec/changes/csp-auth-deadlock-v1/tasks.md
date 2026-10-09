## 1. 后端鉴权双模态与探测接口施工

- [x] 1.1 [Executor: Dual-Mode Auth Middleware] 修改 `internal/transport/http/middleware_auth.go`，在 `cfg.AdminToken == ""` 时自动以 `open_mode` 放行管理请求并赋予管理员上下文，在 `cfg.AdminToken != ""` 时严格执行 Bearer 令牌与 Cookie 校验；无论何种模态均对发布导出令牌保持 403 `invalid_token_scope` 强拦截；以 Go 单元测试覆盖空配置放行、非空匹配放行、非空不匹配 401 拦截及发布令牌 403 阻断；验证命令 `go test -v ./internal/transport/http -run "TestAdminAuthMiddleware.*"`
- [x] 1.2 [Executor: Public Auth Mode Endpoint] 在 `internal/transport/http/router.go` 新增免认证端点 `GET /api/v1/auth/mode`，直接返回当前系统运行模态（`{"data":{"mode":"open"}}` 或 `{"data":{"mode":"token"}}`）；以 Go HTTP 路由测试验证该接口在匿名状态下正常返回且不产生认证失败审计；验证命令 `go test -v ./internal/transport/http -run "TestAuthModeEndpoint.*"`
- [x] 1.3 [Reviewer: Backend Auth Boundary Gate] 独立审查 1.1 与 1.2 的实现 diff，复跑所有 Go transport 单元测试与端到端测试，断言空 Token 下管理端接口不再返回 401，且发布令牌防越权隔离依然生效；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 1.1、1.2

## 2. 前端凭据拦截、AuthModal 与设置视图施工

- [x] 2.1 [Executor: API Client 401 Interceptor] 扩展 `web/src/api/client.ts`，增加 `onUnauthorized(cb: () => void)` 事件监听与去重状态锁，在捕获 401 响应时触发回调并防止重复弹窗风暴；增加 `setAuthToken(token: string | null)` 凭据管理函数；以 Vitest 单元测试覆盖 401 拦截回调触发与 Bearer 凭据动态附加；验证命令 `cd web && npm test src/api/client.test.ts`
- [x] 2.2 [Executor: AuthModal Component] 创建 `web/src/ui/AuthModal.vue` 响应式凭据录入弹窗组件，支持 Token 输入、明暗文切换、非空校验、持久化写入 `localStorage['csp_token']` 并通知当前视图刷新；以 Vitest 组件测试验证输入合法 Token 后触发保存并关闭弹窗；验证命令 `cd web && npm test src/ui/AuthModal.test.ts`
- [x] 2.3 [Executor: Settings View & Header Status Integration] 创建 `web/src/features/settings/SettingsView.vue` 并在 `web/src/App.vue` 中彻底替换 Phase 6.1 占位卡片，提供查看脱敏凭据、更新 Token、测试连接与一键清除功能；在顶部导航栏增加鉴权模态安全徽章并挂载 `AuthModal`；以 Vitest 验证设置视图与导航切换行为；验证命令 `cd web && npm test src/features/settings/SettingsView.test.ts`
- [x] 2.4 [Reviewer: Frontend Usability Gate] 独立审查 2.1–2.3 前端代码与测试，验证 401 弹窗自愈流程、设置视图完整性与无白屏崩溃；运行 `npm run build` 确保生产分发包构建全绿；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 2.1–2.3

## 3. 真机端到端对抗质检与生产上线门禁

- [x] 3.1 [Executor: Real Server Playwright Harness] 编写无 Mock 真实服务端到端测试脚本 `web/e2e/auth-real-server.spec.ts`，严禁使用 `page.route` 伪造网络数据；覆盖 Open Mode 匿名访问（访问 Subscriptions 与 Node Ledger 零 401）与 Token Mode 弹窗录入自愈两组真机用例；本地启动真实 Go 服务运行验证；验证命令 `cd web && npm run test:playwright e2e/auth-real-server.spec.ts`
- [ ] 3.2 [Reviewcommon: Dual-Track Contract Gate] 综合审查全栈代码 diff，在本地运行 `scripts/run_e2e_harness.sh` 全套测试套件，确认 Go 单元测试、Data Race 检测、前端 Build 与 E2E 均全绿，满足单轨删旧写新与零绝对路径规范；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 1.3、2.4、3.1
- [ ] 3.3 [Critic: Headless Browser Adversarial Gate] 针对运行中的 Docker 容器环境（`http://127.0.0.1:17000` 与 `https://sub.einck.top`）执行真实无头浏览器（Playwright/CDP）对抗质检：匿名访问各功能视图，深度监听控制台日志与网络错误，断言零 401 报错、零“Authentication credentials required”红字、DOM 几何布局与节点数据正常呈现；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 3.2
- [ ] 3.4 [Planner: Container Rollout & Deliverable Reconciliation] 在收到 Reviewcommon APPROVED 与 Critic PASSED 凭据后，执行 `docker compose up -d --build` 平滑重启生产容器并校验健康检查 `curl -f http://127.0.0.1:18080/healthz`；向 Einck 交付可用性整改最终闭环战报；依赖 3.3

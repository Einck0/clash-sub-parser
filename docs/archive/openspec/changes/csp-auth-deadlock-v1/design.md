## Context

当前 CSP 1.0 单二进制控制面在线上运行环境下发生 100% 鉴权死锁故障，导致 `https://sub.einck.top` 前端访问所有功能视图时均提示红色错误 `Authentication credentials required`。
深入实机配置与源码调用链审计，定位出三重叠加缺陷：
1. **后端死锁逻辑**（`internal/transport/http/middleware_auth.go:29`）：`AdminAuthMiddleware` 的 Bearer 放行条件为 `cfg.AdminToken != "" && token == cfg.AdminToken`，而针对无凭据请求则在第 70 行无条件阻断并返回 `401 Unauthorized`。
2. **生产配置现状**（`docker-compose.yml:13` 与 `cmd/csp/main.go:224`）：`CSP_ADMIN_TOKEN: ${CSP_ADMIN_TOKEN:-}` 在未显式设置时解析为空字符串 `""`。因此在空 Token 默认形态下，任何请求均无法满足 `cfg.AdminToken != ""`，导致管理端进入 100% 无法通过的绝对死锁。
3. **前端交互与凭据入口空缺**（`web/src/api/client.ts:169` 与 `web/src/App.vue:183`）：前端仅被动读取 `localStorage.getItem('csp_token')`，全仓无登录弹窗、无凭据录入表单，且设置视图（`activeTab === 'settings'`）直接回退至 Phase 6.1 静态占位卡片，用户无任何自愈输入手段。
4. **质检门禁虚假假绿失职**（`web/e2e/cross-device.spec.ts:6-64` 与 `hermes kanban log t_94d63420`）：前期 Critic 审查仅运行单测与 Playwright 脚本，而现有 Playwright 脚本使用 `page.route` 全量拦截并 Mock 了所有 API 返回（伪造 200），未连接真实 Go 后端；且 Go 集成测试全量硬编码了 Bearer Token，导致真实环境容器的匿名访问死锁被彻底漏检。

## Goals / Non-Goals

**Goals:**
- **消除开箱即用鉴权死锁**：实现开放（Open Mode）与保护（Token Mode）双模态鉴权，未配 Token 时默认开放私有访问，即刻恢复 `sub.einck.top` 生产可用性。
- **发布导出令牌硬隔离**：无论处于何种模态，严格禁止发布导出令牌（Publication Export Token）访问 `/api/v1/` 管理接口，违者拦截为 403 Forbidden。
- **轻量前端凭据闭环**：在 `api/client.ts` 引入全局 401 拦截钩子，提供响应式 `AuthModal` 弹窗与设置页凭据管理中心，消除占位卡片。
- **重塑 Critic 真机对抗门禁**：建立严禁 Mock 路由的端到端质检规范，由无头浏览器针对真实容器执行匿名态与受控态双轨对抗断言，确保控制台零 401、DOM 渲染零异常。

**Non-Goals:**
- 不引入笨重的外部 OAuth2 / OIDC 服务端或第三方认证中间件（保持 Go 单二进制自包含极简架构）。
- 不破坏现有发布接口 `/publish/v1/{publication_id}` 的匿名/Token 订阅导出逻辑。
- 不引入多用户 RBAC 权限管理或复杂用户密码数据库表（沿用单一 Admin Token 极简模型）。

## Decisions

### 1. 多源统筹推演（3+1 异构方案池）

主脑秉承系统工程思维，从四个异构维度推演鉴权架构设计：

- **来源一（工业级轻量服务鉴权标杆：Nginx Proxy Manager / subconverter / Uptime Kuma）**：
  - *思路提取*：调研开源成熟工具在未配置主密码或初始启动时的表现。当环境变量未显式声明保护 Token 时，属于零配置开放模式（Zero-Config Open Mode），允许受信任的内网/私有部署直接访问，同时在 UI 顶部展示提示；配置 Token 后无缝切换为受保护模式。
  - *可行性对账*：完美贴合 Einck 个人项目的轻量部署诉求。既免除首次安装还要到容器日志寻找临时随机密码的摩擦，又提供向受控模式平滑过渡的能力。
- **来源二（现代 SPA 凭据注入与 401 拦截标准：Axios / Fetch 统一拦截器与 Reactive Modal）**：
  - *思路提取*：现代单页应用在捕获 401 时，通过全局事件总线唤起挂载于根节点的认证模态框（AuthModal），由用户录入凭据并持久化至 `localStorage`，后续请求自动补全 `Authorization: Bearer <token>`，并重新激活当前页面查询。
  - *可行性对账*：CSP 前端基于 Vue 3 + DaisyUI，完全可在 `App.vue` 挂载轻量 `AuthModal.vue`，配合 `client.ts` 中的 `onUnauthorized` 钩子，提供丝滑的就地自愈体验，无需强制路由重定向。
- **来源三（真机对抗门禁与防测试欺骗标杆：Playwright 真实网络端到端与 CDP 深度断言）**：
  - *思路提取*：针对“测试代码硬编码 Token 或使用 `page.route` Mock 导致假绿”的失职现象，制定对抗级测试门禁：严禁在发布验收阶段 Mock 网络请求，质检必须分步执行“匿名零凭据真机访问”与“受控凭据录入恢复”两套用例，监听浏览器控制台与网络状态。
  - *可行性对账*：本地已具备 Playwright（`web/package.json:20`）与 Go 原生单二进制服务，可快速编写针对 `127.0.0.1:18080` 的真实无 Mock 端到端脚本。
- **来源四（本土现状统筹与极简融合方案：CSP 1.0 架构原生演进）**：
  - *本土融合*：在 Go 后端 `internal/transport/http/middleware_auth.go` 增补 `cfg.AdminToken == ""` 分支；新增免鉴权探测接口 `/api/v1/auth/mode`；前端 `SettingsView.vue` 彻底替换占位卡片；以最小 diff 解决全局死锁。

### 2. 后端双模态鉴权流转契约

```text
HTTP Request 到达 /api/v1/*
               │
               ▼
   [检查是否携带发布导出令牌？] ──── 是 ────► 返回 403 Forbidden (invalid_token_scope)
               │ 否
               ▼
      [cfg.AdminToken 为空？]
       ├── 是 (Open Mode) ──────► 赋予 ActorKindAdmin，AuthMethod="open_mode"，放行 (200 OK)
       └── 否 (Token Mode)
               │
               ▼
      [检查 Bearer / Cookie 凭据]
       ├── 匹配 cfg.AdminToken ─► 赋予 ActorKindAdmin，AuthMethod="bearer"，放行 (200 OK)
       └── 不匹配或无凭据 ──────► 返回 401 Unauthorized (unauthorized)
```

- **开放模式判空准则**：`strings.TrimSpace(cfg.AdminToken) == ""`。
- **安全防越权红线**：即便在 Open Mode 下，若请求携带了 `isPublicationToken` 识别出的导出令牌，依然严格阻断为 403。导出令牌绝对不能因为管理端处于开放模式而意外获取管理资源访问权！
- **鉴权探测接口**：`GET /api/v1/auth/mode` 在 `AdminAuthMiddleware` 外部或内部放行白名单中处理，直接输出 `{ "data": { "mode": "open" | "token" } }`，无副作用、无信息泄露。

### 3. 前端全局凭据管理与弹窗交互契约

- **401 拦截与去重**：`api/client.ts` 维护 `unauthorizedHandlers` 回调列表与 `isPrompting` 状态锁。当捕获 401 响应时，触发回调且防止同时触发多个弹窗。
- **AuthModal 行为**：
  - 呈现暗色半透明遮罩，居中弹出 DaisyUI `modal-box`。
  - 提供密码输入框（明暗文切换按钮）、提交按钮与关闭按钮。
  - 点击提交时进行非空校验，成功后执行 `localStorage.setItem('csp_token', token)`，并在 `api` 实例中更新当前内存 Token。
  - 关闭弹窗并自动刷新当前活动视图（如重新触发 `SubscriptionsView` 或 `NodesView` 的 fetch）。
- **设置视图落位**：新建 `web/src/features/settings/SettingsView.vue`，挂载至 `App.vue:183` 的 `activeTab === 'settings'` 分支，提供：
  1. 服务端鉴权模态展示（Open Mode / Protected）；
  2. 当前本地凭据掩码查看与清除功能；
  3. 一键修改 Token 并测试连通性；
  4. 系统运行时概况（版本、健康状态）。

### 4. Critic 真机无头浏览器质检规范契约

Reviewcommon 与 Critic 必须废除任何以 Mock API 为基础的端到端验收用例。新增 `web/e2e/auth-real-server.spec.ts` 规范：
- **测试环境**：连接本地已启动的真实 CSP 服务（`http://127.0.0.1:18080` 或 Docker 容器 `http://127.0.0.1:17000`）。
- **执行 Pass 1（Open Mode 匿名验证）**：
  - 启动无头 Chromium，设置干净上下文（无 Cookie、无 localStorage）。
  - 导航至首页，断言页面标题正常。
  - 依次点击 Subscriptions、Node Ledger 导航项。
  - 监听所有网络响应：断言没有任何 `/api/v1/` 请求返回 401/403/500。
  - 监听控制台日志：断言无 `Authentication credentials required` 报错。
  - 断言 DOM：表格结构或空数据占位符正常渲染，无全局崩溃提示。
- **执行 Pass 2（Token Mode 受控流转验证）**：
  - 重启/模拟后端带 Token 模式。
  - 匿名访问管理接口，断言 `AuthModal` 自动弹出且包含密码输入框。
  - 在输入框键入正确 Token 并点击保存，断言弹窗消失且数据立即恢复展示。

## Risks / Trade-offs

- **[Risk 1: 开放模式在公网环境下暴露管理接口]**
  - *Mitigation*: 在工作台顶部导航栏渲染醒目的黄色安全提示徽章 `Open Mode`，并在设置页明确说明“当前未配置 CSP_ADMIN_TOKEN，建议在生产暴露时配置令牌”；同时现有文档与 Docker Compose 示例提供设置指南。
- **[Risk 2: 导出令牌在 Open Mode 下被误认为合法管理请求]**
  - *Mitigation*: 鉴权中间件在逻辑上严格先执行 `isPublicationToken(...)` 判断，只要携带导出令牌，不论 Open Mode 还是 Token Mode 一律强制 403 阻断。
- **[Risk 3: 并发 401 请求引发前端事件风暴或界面死锁]**
  - *Mitigation*: 前端凭据管理器使用单例防抖锁，保证无论一秒内收到多少个 401 失败，仅有首个请求唤起弹窗，后续请求排队或在凭据录入后统一重发。

## Migration Plan

1. **零停机平滑更新**：
   - Executor 完成后端与前端代码改造并打包镜像；
   - 执行 `docker compose up -d --build` 平滑重启容器；
   - 生产环境现有 `CSP_ADMIN_TOKEN` 为空配置无需改动，服务重启后将立即无缝进入 Open Mode，死锁瞬间解除；
   - 用户如需加固安全，仅需在 `.env` 或 `docker-compose.yml` 中赋予 `CSP_ADMIN_TOKEN` 任意强密码并 `docker compose up -d` 即可秒级开启 Token Mode。
2. **回滚路径**：
   - 镜像使用本地构建，若出现异常可通过 `git checkout` 与重建恢复。由于不修改 SQLite 底层表结构与数据格式，具备秒级回滚安全性。

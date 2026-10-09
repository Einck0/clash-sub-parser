## Purpose

定义 CSP 前端单页工作台的全局凭据管理、401 自动拦截与弹窗录入、顶部安全状态回显，以及设置视图完整替换占位卡片的交互契约，确保用户在任何鉴权异常时均能以优雅、自愈的方式提供凭据，杜绝白屏与红字阻塞。

## ADDED Requirements

### Requirement: 全局 401 拦截与 AuthModal 自动唤起
前端 API 客户端 SHALL 在捕获任何服务响应状态码为 401 的请求时，自动触发全局未认证事件钩子（`onUnauthorized`），并在前端根界面唤起非阻塞的模态录入对话框（`AuthModal`）。系统 MUST NOT 因 401 响应导致界面白屏崩溃或路由重置，且在同一轮次突发多个 401 错误时 MUST 实行去重保护，仅呈现单一活动的录入弹窗。

#### Scenario: 接口 401 自动触发录入弹窗
- **WHEN** 界面请求受保护的 `/api/v1/subscriptions` 或 `/api/v1/nodes` 收到 HTTP 401
- **THEN** 界面中心自动浮起 `AuthModal` 弹窗，提示输入 Admin Token，原页面结构与侧边栏保持可见

#### Scenario: 突发多个 401 不重复弹窗
- **WHEN** 页面并行发起 3 个管理接口请求且全部返回 401
- **THEN** 界面仅渲染一个活动凭据弹窗，不会产生重叠遮罩或事件风暴

### Requirement: 凭据录入、验证与本地持久化
`AuthModal` SHALL 提供输入密码框（支持明暗文切换）与保存按钮。用户输入有效令牌并点击确认后，系统 MUST 将令牌写入浏览器的 `localStorage['csp_token']`，并在内存 API 客户端中同步更新。此后发出的所有 API 请求 MUST 在 `Authorization` 请求头中自动附加 `Bearer <token>`，并触发当前视图数据重新加载以恢复界面展示。

#### Scenario: 提交有效凭据恢复页面
- **WHEN** 用户在 `AuthModal` 中输入正确的 Admin Token 并点击保存
- **THEN** 令牌被存入 `localStorage`，弹窗自动关闭，当前视图自动刷新并成功渲染业务数据

#### Scenario: 空凭据校验拦截
- **WHEN** 用户在 `AuthModal` 输入空白字符并尝试提交
- **THEN** 弹窗在本地拦截提交并给出“令牌不能为空”的表单警告，不写入 `localStorage`

### Requirement: 顶部安全水位指示器
工作台顶部导航栏 SHALL 在健康状态探针旁呈现明确的鉴权状态徽章（Auth Badge）。当通过 `GET /api/v1/auth/mode` 探测到服务处于开放模式时，徽章 MUST 显示为“Open Mode”（带有温和提示色彩）；当探测到处于受控保护模式时，徽章 MUST 显示为“Protected”，点击该徽章可直接调起凭据管理弹窗。

#### Scenario: 开放模式状态回显
- **WHEN** 工作台加载且后端响应 `auth/mode` 为 `open`
- **THEN** 顶部栏渲染“Open Mode”安全徽章，悬停显示“当前为零配置私有开放模式”

#### Scenario: 保护模式状态回显
- **WHEN** 工作台加载且后端响应 `auth/mode` 为 `token`
- **THEN** 顶部栏渲染“Protected”安全徽章，直观表明控制面已受令牌防护

### Requirement: 设置视图凭据管理中心落地
工作台 MUST 彻底移除 `web/src/App.vue:183` 中的 Phase 6.1 占位卡片，在 `activeTab === 'settings'` 时渲染专用的 `SettingsView` 视图组件。该视图 MUST 提供独立的“安全与凭据管理”卡片，允许操作者查看当前本地存储的 Token（默认脱敏掩码显示）、更新 Token、一键测试与后端的连通性，以及清除本地 Token。

#### Scenario: 打开设置视图查看凭据状态
- **WHEN** 用户在侧边栏或底部导航栏点击“Settings”
- **THEN** 界面渲染完整的 `SettingsView` 视图，清晰呈现当前服务鉴权模态与本地凭据配置

#### Scenario: 清除本地存储凭据
- **WHEN** 用户在设置视图点击“Clear Stored Token”
- **THEN** 本地存储中的 `csp_token` 被彻底清除，API 客户端移除默认 Bearer 头，并弹出轻量 Toast 提示

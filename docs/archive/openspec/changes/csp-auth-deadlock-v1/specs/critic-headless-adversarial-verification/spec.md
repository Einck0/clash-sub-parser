## Purpose

定义审查子机（Reviewcommon）与批评家（Critic）在生产交付前必须执行的真机无头浏览器对抗质检门禁规范，严厉禁止在端到端审查中采用 Mock 网络路由欺骗假绿，确立双模态端到端测试与控制台零异常断言铁律。

## ADDED Requirements

### Requirement: 绝对禁止 Mock 网络路由的真机端到端审查
Reviewcommon 与 Critic 在执行质量门禁审查时，MUST 严格禁止在生产端到端验收脚本中使用 `page.route` 拦截或伪造 `/api/v1/` 响应。端到端质检 MUST 运行于真实的 Go 后端 HTTP 服务（或真实 Docker 容器端口 `127.0.0.1:17000` / `18080`）之上，确保所有 HTTP 状态码、请求头鉴权逻辑与响应体均由真实代码产生。

#### Scenario: 真实后端端到端通信验证
- **WHEN** 审查子机执行真机端到端验收脚本
- **THEN** 所有的前端网络请求直达真实监听的 Go 服务端端口，网络追踪中不得含有测试拦截桩或伪造的 200 响应

### Requirement: 真实无头浏览器双模态对抗断言
Critic MUST 驱动真实的无头浏览器（Playwright 或 CDP）对系统进行双重视角对抗性验收：
1. 视角一（开放模式匿名基准质检）：在容器未配置 `CSP_ADMIN_TOKEN` 的默认环境下，以完全清空 `localStorage` 的匿名浏览器会话访问系统，连续点击切换 Subscriptions、Node Ledger、Probe Engine、Policy Tree 视图，断言所有接口 HTTP 状态码为 200，页面不得出现“Authentication credentials required”红色错误横幅，核心 DOM 列表必须成功渲染。
2. 视角二（保护模式受控流转质检）：在配置了测试 Admin Token 的环境下，匿名浏览器发起管理请求时必须断言捕获 401 并自动弹出 `AuthModal`，通过脚本模拟键入正确 Token 并保存后，断言后续请求自动携带 Token 且数据恢复正常。

#### Scenario: 开放模式匿名访问真机质检通过
- **WHEN** Critic 驱动匿名无头浏览器访问处于开放模式的真实容器工作台，并依次点击各功能视图
- **THEN** 所有视图平滑加载，捕获到的所有 `/api/v1/` 请求状态码均为 200，无 401 报错，Critic 给出通过裁决

#### Scenario: 保护模式凭据唤起与自愈质检通过
- **WHEN** Critic 驱动匿名无头浏览器访问处于保护模式的真实后端，并在 401 弹出 `AuthModal` 后填入合法 Token
- **THEN** 凭据成功保存至 `localStorage`，视图重新请求并全量正常渲染，无后续 401 发生

### Requirement: 控制台错误与红字横幅硬性门禁
Critic 在端到端自动化巡检期间，MUST 监听浏览器事件 `page.on('console')`、`page.on('pageerror')` 以及页面网络失败流。一旦捕获到任何未处理的 HTTP 401 错误、控制台未捕获异常，或在页面 DOM 中检测到含有“Authentication credentials required”等鉴权阻断文本，Critic MUST 立即给出 `CRITIC_VERDICT: REJECTED` 一票否决裁决，并保存控制台完整日志与 DOM 快照作为事故证据。

#### Scenario: 页面出现鉴权错误红字横幅触发一票否决
- **WHEN** 无头浏览器在渲染 Subscriptions 或 Node Ledger 页面时捕获到红字警告“Authentication credentials required”
- **THEN** Critic 立即判定质检失败，输出 REJECTED 裁决并阻断任务上线

#### Scenario: 控制台与 DOM 几何零异常通过验收
- **WHEN** 无头浏览器完整走完双模态全功能流程，控制台零 401、网络零失败、DOM 几何布局与数据显示完整
- **THEN** Critic 输出 `CRITIC_VERDICT: PASSED` 准许发布

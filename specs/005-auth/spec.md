---
feature: 005-auth
title: Auth Capability Specification
status: candidate
schema: speckit-native
---

# Auth Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的身份认证与安全控制模型。系统必须支持管理端控制台与导出直链独立鉴权开关，统一管理系统主令牌，基于 HttpOnly Cookie 管理会话生命周期并对变更操作执行 CSRF 防护，同时确立明文敏感信息存储模型与管理面板安全风险警示。

## User Scenarios & Priority

### User Story 1 - 控制面与数据面解耦双开关鉴权 (Priority: P1)
- **GIVEN** 管理员需要保护后台管理面板但希望公开特定订阅直链
- **WHEN** 管理员开启管理端认证并关闭导出链接认证
- **THEN** 访问管理后台必须强制登录，而订阅客户端可免登录拉取配置

### User Story 2 - 会话 Cookie 安全管理与 CSRF 防御 (Priority: P1)
- **GIVEN** 管理员已成功登录控制台
- **WHEN** 执行订阅增删改等状态变更操作
- **THEN** 系统使用 HttpOnly Session Cookie 维持会话，并强制校验合法 CSRF 令牌防跨站伪造

### User Story 3 - 明文凭据存储模型与安全披露 (Priority: P2)
- **GIVEN** 节点包含服务器、端口、密码与密钥等连接参数
- **WHEN** 系统将节点入库持久化
- **THEN** 遵照用户指令在 nodes 表明文存储凭据，不引入多余加密分表，并在控制台显著警示安全边界

---

## Requirements

### 1. 鉴权架构与会话管理

#### Requirement: 管理端与导出链接独立鉴权开关 [FR-AUTH-01] <a id="req-auth-dual-switch"></a>
系统 SHALL 提供解耦的双独立鉴权配置项：管理端控制台鉴权开关 (`AUTH_ADMIN_REQUIRED`) 与订阅导出链接鉴权开关 (`AUTH_EXPORT_REQUIRED`)。两个开关互不影响、独立生效。

##### Scenario: 管理端强制登录不影响公开导出 <a id="scenario-auth-independent-switch"></a>
- **GIVEN** `AUTH_ADMIN_REQUIRED=true` 且 `AUTH_EXPORT_REQUIRED=false`
- **WHEN** 未登录访客分别访问后台管理 API 与公开导出链接
- **THEN** 管理 API 返回 401 Unauthorized，公开导出链接返回 200 OK 与配置数据

#### Requirement: 统一系统主令牌配置与共享 [FR-AUTH-02] <a id="req-auth-master-token"></a>
系统 SHALL 支持通过环境变量或系统设置配置统一主令牌（System Master Token）。主令牌可作为控制台初始登录凭证，亦可作为跨端导出的兜底超级授权令牌。

##### Scenario: 环境变量加载与单点更新 <a id="scenario-auth-env-master-token"></a>
- **GIVEN** 环境变量 `APP_MASTER_TOKEN` 已设定
- **WHEN** 系统启动初始化鉴权中间件
- **THEN** 鉴权中间件正确加载该令牌，并提供安全常量时间比对

#### Requirement: 控制面凭据生命周期与会话 Cookie [FR-AUTH-04] <a id="req-auth-session-cookie"></a>
管理端认证成功后，服务端 MUST 下发带有 `HttpOnly`、`SameSite=Lax` 属性的会话 Cookie。会话支持空闲超时自动失效，并提供显式登出端点销毁当前会话。

##### Scenario: HttpOnly 会话 Cookie 防 XSS 窃取 <a id="scenario-auth-httponly-cookie"></a>
- **GIVEN** 管理员提交正确主令牌登录成功
- **WHEN** 服务端响应 `Set-Cookie`
- **THEN** Cookie 包含 `HttpOnly; Path=/` 属性，客户端脚本无法读取该凭证

##### Scenario: 会话超时与主动登出失效 <a id="scenario-auth-session-expiry"></a>
- **GIVEN** 活跃登录会话
- **WHEN** 客户端请求 `POST /api/v1/auth/logout` 或会话达到最大闲置时长
- **THEN** 服务端销毁会话记录并清理 Cookie，后续请求返回 401

#### Requirement: 状态变更请求 CSRF 防护与验证 <a id="req-auth-csrf-protection"></a>
所有针对管理端状态变更的 HTTP 请求（POST、PUT、PATCH、DELETE）MUST 包含合法的 CSRF 令牌校验。对于缺少 CSRF 令牌或令牌不匹配的跨站请求，服务端 MUST 拒绝执行并返回 403 Forbidden。

##### Scenario: 登录后带合法 CSRF 令牌发起写请求成功 <a id="scenario-auth-csrf-token-valid"></a>
- **GIVEN** 管理员已登录并取得会话 CSRF 令牌
- **WHEN** 客户端发起带 `X-CSRF-Token` 头的创建订阅请求
- **THEN** 服务端校验通过并执行创建操作，返回 201 Created

##### Scenario: 缺少或错误 CSRF 令牌返回 403 Forbidden <a id="scenario-auth-csrf-missing-403"></a>
- **GIVEN** 未携带 `X-CSRF-Token` 头或令牌伪造的请求
- **WHEN** 发送至状态变更端点
- **THEN** 服务端立即拒绝请求并响应 403 Forbidden，不触发任何数据库变更

---

### 2. 凭据存储模型

#### Requirement: 明文敏感信息存储模型与安全警示 [FR-AUTH-03] <a id="req-auth-plaintext-storage"></a>
遵循用户明确指示，系统 SHALL 采用明文直接持久化模型，将节点连接凭据（密码、UUID、公钥等）直接存储于 `nodes` 表相应字段中，不引入外部密钥管理系统或多余加密库。管理控制台 MUST 包含清晰的安全警示信息。

##### Scenario: nodes 表明文存储节点凭据 <a id="scenario-auth-plaintext-persist"></a>
- **GIVEN** 导入包含密码的节点
- **WHEN** 节点写入 SQLite 数据库
- **THEN** `credentials` 字段直接持久化明文字符串，支持极简检索与零依赖维护

##### Scenario: 管理面板显示明文存储风险警示 <a id="scenario-auth-security-banner"></a>
- **GIVEN** 管理员访问系统安全设置面板
- **WHEN** 面板渲染完成
- **THEN** 界面显著呈现“凭据明文直存，请妥善保护数据库文件”的提示横幅

---

---

---

---

## Supplemental Retained Behavioral Capabilities & Historical Clauses

### Overview
本节完整收录历史变更中属于本领域的行为规范、具体契约（包含精确状态码、传输参数、请求头校验等）与验收场景，保持行为约束无损物理留存与点对点双向追溯。

### Retained Functional Requirements

#### Requirement: 17000 端口生产环境隔离约束 [HR-018] <a id="req-auth-hr-018"></a>

宿主机 17000 端口 SHALL 被严格认定为生产 Docker 容器映射端口别名，严禁任何自动化测试用例、预览脚本或端到端 Harness 向 `127.0.0.1:17000` 或 `127.0.0.1:18080` 发起测试写入或启动服务冲突。所有集成测试与端到端测试 MUST 使用系统随机分配的临时端口（端口 `0`）与独立的临时 SQLite 数据库。

##### Scenario: 自动化集成测试与 E2E 启动服务 <a id="scenario-auth-hr-018-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 运行 smoke_test.sh、Go 集成测试或 Playwright E2E 测试
- **THEN** 测试 Harness 分配未占用的动态端口并挂载独立临时目录，不与 17000/18080 端口或生产数据卷发生任何交互

#### Requirement: 导出发布令牌管理端越权隔离 [HR-021] <a id="req-auth-hr-021"></a>

系统 MUST 在零配置开放模式与受控保护模式下，均对携带发布导出令牌（Publication Export Token）访问 `/api/v1/` 管理端接口的行为执行强隔离拦截。当请求携带的令牌经系统判定为发布导出令牌时，系统 MUST 拒绝该请求并返回 403 Forbidden 与稳定错误码 `invalid_token_scope`，且 MUST NOT 赋予任何管理权限或泄露管理端资源存在性。

##### Scenario: 开放模式下发布导出令牌越权管理被拒绝 <a id="scenario-auth-hr-021-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 系统处于开放模式且调用者使用有效发布导出令牌请求 `/api/v1/nodes` 等管理接口
- **THEN** 系统返回 403 Forbidden 与错误码 `invalid_token_scope`，且不执行业务查询

##### Scenario: 保护模式下发布导出令牌越权管理被拒绝 <a id="scenario-auth-hr-021-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 系统处于保护模式且调用者使用有效发布导出令牌请求 `/api/v1/subscriptions` 等管理接口
- **THEN** 系统返回 403 Forbidden 与错误码 `invalid_token_scope`，且不执行业务查询

#### Requirement: 公共鉴权模态查询端点 [HR-022] <a id="req-auth-hr-022"></a>

系统 SHALL 提供免认证查询端点 `GET /api/v1/auth/mode`，该端点 MUST 在鉴权中间件拦截前被放行。该端点 MUST 返回 `{ "data": { "mode": "open" | "token" } }`，用于客户端在初始化时确定系统当前安全水位与是否需要呈现凭据输入提示，且该端点 MUST NOT 泄露管理令牌的真实值或哈希。

##### Scenario: 开放模式下查询鉴权模态 <a id="scenario-auth-hr-022-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端以匿名状态向 `GET /api/v1/auth/mode` 发起请求，且服务端未配置 AdminToken
- **THEN** 系统返回 200 OK 且响应体为 `{ "data": { "mode": "open" } }`

##### Scenario: 保护模式下查询鉴权模态 <a id="scenario-auth-hr-022-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端以匿名状态向 `GET /api/v1/auth/mode` 发起请求，且服务端已配置 AdminToken
- **THEN** 系统返回 200 OK 且响应体为 `{ "data": { "mode": "token" } }`

#### Requirement: 绝对禁止 Mock 网络路由的真机端到端审查 [HR-023] <a id="req-auth-hr-023"></a>

Reviewcommon 与 Critic 在执行质量门禁审查时，MUST 严格禁止在生产端到端验收脚本中使用 `page.route` 拦截或伪造 `/api/v1/` 响应。端到端质检 MUST 运行于真实的 Go 后端 HTTP 服务（或真实 Docker 容器端口 `127.0.0.1:17000` / `18080`）之上，确保所有 HTTP 状态码、请求头鉴权逻辑与响应体均由真实代码产生。

##### Scenario: 真实后端端到端通信验证 <a id="scenario-auth-hr-023-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 审查子机执行真机端到端验收脚本
- **THEN** 所有的前端网络请求直达真实监听的 Go 服务端端口，网络追踪中不得含有测试拦截桩或伪造的 200 响应

#### Requirement: 控制台错误与红字横幅硬性门禁 [HR-025] <a id="req-auth-hr-025"></a>

Critic 在端到端自动化巡检期间，MUST 监听浏览器事件 `page.on('console')`、`page.on('pageerror')` 以及页面网络失败流。一旦捕获到任何未处理的 HTTP 401 错误、控制台未捕获异常，或在页面 DOM 中检测到含有“Authentication credentials required”等鉴权阻断文本，Critic MUST 立即给出 `CRITIC_VERDICT: REJECTED` 一票否决裁决，并保存控制台完整日志与 DOM 快照作为事故证据。

##### Scenario: 页面出现鉴权错误红字横幅触发一票否决 <a id="scenario-auth-hr-025-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 无头浏览器在渲染 Subscriptions 或 Node Ledger 页面时捕获到红字警告“Authentication credentials required”
- **THEN** Critic 立即判定质检失败，输出 REJECTED 裁决并阻断任务上线

##### Scenario: 控制台与 DOM 几何零异常通过验收 <a id="scenario-auth-hr-025-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 无头浏览器完整走完双模态全功能流程，控制台零 401、网络零失败、DOM 几何布局与数据显示完整
- **THEN** Critic 输出 `CRITIC_VERDICT: PASSED` 准许发布

#### Requirement: 全局 401 拦截与 AuthModal 自动唤起 [HR-026] <a id="req-auth-hr-026"></a>

前端 API 客户端 SHALL 在捕获任何服务响应状态码为 401 的请求时，自动触发全局未认证事件钩子（`onUnauthorized`），并在前端根界面唤起非阻塞的模态录入对话框（`AuthModal`）。系统 MUST NOT 因 401 响应导致界面白屏崩溃或路由重置，且在同一轮次突发多个 401 错误时 MUST 实行去重保护，仅呈现单一活动的录入弹窗。

##### Scenario: 接口 401 自动触发录入弹窗 <a id="scenario-auth-hr-026-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 界面请求受保护的 `/api/v1/subscriptions` 或 `/api/v1/nodes` 收到 HTTP 401
- **THEN** 界面中心自动浮起 `AuthModal` 弹窗，提示输入 Admin Token，原页面结构与侧边栏保持可见

##### Scenario: 突发多个 401 不重复弹窗 <a id="scenario-auth-hr-026-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 页面并行发起 3 个管理接口请求且全部返回 401
- **THEN** 界面仅渲染一个活动凭据弹窗，不会产生重叠遮罩或事件风暴

#### Requirement: 凭据录入、验证与本地持久化 [HR-027] <a id="req-auth-hr-027"></a>

`AuthModal` SHALL 提供输入密码框（支持明暗文切换）与保存按钮。用户输入有效令牌并点击确认后，系统 MUST 将令牌写入浏览器的 `localStorage['csp_token']`，并在内存 API 客户端中同步更新。此后发出的所有 API 请求 MUST 在 `Authorization` 请求头中自动附加 `Bearer <token>`，并触发当前视图数据重新加载以恢复界面展示。

##### Scenario: 提交有效凭据恢复页面 <a id="scenario-auth-hr-027-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 用户在 `AuthModal` 中输入正确的 Admin Token 并点击保存
- **THEN** 令牌被存入 `localStorage`，弹窗自动关闭，当前视图自动刷新并成功渲染业务数据

##### Scenario: 空凭据校验拦截 <a id="scenario-auth-hr-027-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 用户在 `AuthModal` 输入空白字符并尝试提交
- **THEN** 弹窗在本地拦截提交并给出“令牌不能为空”的表单警告，不写入 `localStorage`

#### Requirement: 设置视图凭据管理中心落地 [HR-029] <a id="req-auth-hr-029"></a>

工作台 MUST 彻底移除 `web/src/App.vue:183` 中的 Phase 6.1 占位卡片，在 `activeTab === 'settings'` 时渲染专用的 `SettingsView` 视图组件。该视图 MUST 提供独立的“安全与凭据管理”卡片，允许操作者查看当前本地存储的 Token（默认脱敏掩码显示）、更新 Token、一键测试与后端的连通性，以及清除本地 Token。

##### Scenario: 打开设置视图查看凭据状态 <a id="scenario-auth-hr-029-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 用户在侧边栏或底部导航栏点击“Settings”
- **THEN** 界面渲染完整的 `SettingsView` 视图，清晰呈现当前服务鉴权模态与本地凭据配置

##### Scenario: 清除本地存储凭据 <a id="scenario-auth-hr-029-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 用户在设置视图点击“Clear Stored Token”
- **THEN** 本地存储中的 `csp_token` 被彻底清除，API 客户端移除默认 Bearer 头，并弹出轻量 Toast 提示

#### Requirement: 版本化控制面与统一错误 [HR-038] <a id="req-auth-hr-038"></a>

系统 SHALL 仅将 `/api/v1/` 作为管理控制面的版本化根路径。所有 JSON 成功响应 MUST 包含 `data`；所有失败响应 MUST 包含稳定机器码 `code`、安全的人类消息 `message` 与请求关联号 `request_id`，且 MUST 不含令牌、订阅 URL、节点凭据或完整生成配置。

##### Scenario: 已验证管理请求成功 <a id="scenario-auth-hr-038-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 已认证调用者请求一个存在的 `/api/v1/` 资源
- **THEN** 系统返回对应 HTTP 成功状态与 `{ "data": ... }` JSON 响应

##### Scenario: 无效输入被拒绝 <a id="scenario-auth-hr-038-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 调用者提交不满足资源约束的 JSON 请求
- **THEN** 系统返回 422 与稳定 `code`，且不写入部分资源

#### Requirement: 身份、授权与发布令牌隔离 [HR-039] <a id="req-auth-hr-039"></a>

系统 SHALL 默认拒绝未认证的管理 API 和管理界面请求。系统 MUST 将管理会话或 bearer 凭据与导出发布令牌分离存储、哈希和审计；导出令牌 MUST 仅授权其绑定发布物，不得访问管理 API。所有改变状态的 cookie 会话请求 MUST 通过 CSRF 校验。

##### Scenario: 导出令牌不能管理资源 <a id="scenario-auth-hr-039-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 调用者使用有效导出令牌请求 `/api/v1/` 管理资源
- **THEN** 系统返回 401 或 403，且不泄漏资源存在性

##### Scenario: 缺少 CSRF 的会话写入 <a id="scenario-auth-hr-039-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 已登录浏览器会话提交缺少有效 CSRF 证明的状态变更请求
- **THEN** 系统拒绝请求且不改变数据

#### Requirement: 显式拒绝旧端点 [HR-040] <a id="req-auth-hr-040"></a>

系统 MUST 对 `/yaml`、`/script` 与任何未版本化旧 `/api/` 路径返回 410，错误码为 `legacy_endpoint_removed`；系统 MUST 不提供重定向、别名、请求转换或默认兼容输出。

##### Scenario: 旧 YAML 地址被访问 <a id="scenario-auth-hr-040-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端请求 `/yaml`
- **THEN** 系统返回 410 与 `legacy_endpoint_removed`

#### Requirement: 可控资源消耗与审计 [HR-041] <a id="req-auth-hr-041"></a>

系统 SHALL 对请求体、分页大小、导出编译、订阅抓取、导入和探测提交执行明确上限。系统 MUST 为每次变更、导入、发布与探测命令记录操作主体、请求关联号、结果和脱敏摘要。

##### Scenario: 超过分页上限 <a id="scenario-auth-hr-041-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 调用者请求大于服务公布上限的 `page_size`
- **THEN** 系统返回 422 与允许的上限，且不执行无界查询

#### Requirement: 无密钥启动与节点明文持久化契约 [HR-054] <a id="req-auth-hr-054"></a>

系统启动 SHALL 不依赖 `CSP_NODE_CREDENTIAL_KEY` 或任何加密主密钥环境变量，开箱即可执行订阅拉取、节点入库、主动探测、策略解析、配置预览与发布分发。SQLite 迁移 `000011` SHALL 在 `nodes` 表新增 `server TEXT NOT NULL DEFAULT ''`、`port INTEGER NOT NULL DEFAULT 0`、`config_json TEXT NOT NULL DEFAULT '{}'` 列，删除 `normalized_config_secret_ref` 与 `credential_version` 列，并删除 `node_credentials` 表；订阅解析与 `ReconcileSubscription` SHALL 直接将完整节点配置（`Server`、`Port`、`Credentials`）明文持久化到 `nodes` 表。

##### Scenario: 无加密密钥环境下启动与订阅拉取 <a id="scenario-auth-hr-054-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 系统在未设置 `CSP_NODE_CREDENTIAL_KEY` 的环境中启动并执行订阅拉取（`ReconcileSubscription`）
- **THEN** 服务正常启动且无密钥告警，订阅中的合法节点完整明文配置（含 `server`、`port` 及协议 `config_json`）直接写入 `nodes` 表，不产生 `HTTP 422 node credentials unavailable` 错误

##### Scenario: Schema 000011 迁移与旧表清理 <a id="scenario-auth-hr-054-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 在包含 `000001` 至 `000010` 迁移的 SQLite 数据库上执行 `000011` 迁移
- **THEN** `node_credentials` 表被删除，`nodes` 表具备 `server`、`port`、`config_json` 列且不再包含 `normalized_config_secret_ref` 与 `credential_version` 列，`publications` 表具备 `content` 列且不再包含 `artifact_key_id`、`artifact_nonce`、`artifact_ciphertext`、`credential_binding_digest`、`credential_bindings_json` 列

#### Requirement: 控制面管理端受控基准认证 [HR-058] <a id="req-auth-hr-058"></a>

系统 SHALL 在开启管理保护时对所有 `/api/v1/` 控制面管理接口强制执行凭据校验，支持请求头中的 Bearer 令牌或经过验证的会话 Cookie（`csp_session`）。对于未携带有效凭据或凭据不匹配的请求，系统 MUST 拒绝该请求并返回 HTTP 401 Unauthorized 与标准错误码 `unauthorized`，且 MUST NOT 暴露底层敏感堆栈或系统内部数据。

##### Scenario: 有效管理令牌放行 <a id="scenario-auth-hr-058-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端在 `Authorization` 头中携带匹配的管理 Bearer 令牌请求 `/api/v1/nodes` 等管理接口
- **THEN** 系统响应 200 OK 并正常返回业务数据，请求上下文被赋予管理员身份

##### Scenario: 缺失凭据请求被拦截为 401 <a id="scenario-auth-hr-058-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端以匿名状态未携带任何凭据请求 `/api/v1/subscriptions` 等受控管理接口
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误信息 `Authentication credentials required`

##### Scenario: 错误凭据请求被拦截为 401 <a id="scenario-auth-hr-058-3"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端在 `Authorization` 头中携带不匹配的令牌请求管理接口
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误信息 `Invalid authentication credentials`

#### Requirement: 发布导出令牌管理越权强隔离 [HR-059] <a id="req-auth-hr-059"></a>

管理端处于受控模式时，系统 MUST 对使用发布导出令牌（Publication Export Token）访问 `/api/v1/` 管理端接口的行为实施严格的安全隔离拦截。当请求携带的令牌匹配任意有效发布导出专属令牌时，系统 MUST 拦截该请求并返回 HTTP 403 Forbidden 与稳定错误码 `invalid_token_scope`，绝不允许发布导出凭据在受控模式下越权调用控制面。管理端处于开放模式时，系统 MUST 无条件放行请求；此时管理员权限来自明确的开放配置或尚未配置主令牌，而不是发布令牌本身。导出保护开关不改变管理端的模式判定。

##### Scenario: 发布导出令牌访问管理端被 403 拒绝 <a id="scenario-auth-hr-059-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理端处于受控模式，客户端使用有效的专属发布导出令牌请求 `/api/v1/publications` 或 `/api/v1/policies/global-node-filter` 等管理接口
- **THEN** 系统返回 403 Forbidden 与错误码 `invalid_token_scope`，且不执行业务查询或状态变更

##### Scenario: 管理端开放模式无条件放行 <a id="scenario-auth-hr-059-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理端鉴权开关关闭（或没有配置主令牌），客户端匿名访问或携带任意令牌访问管理接口
- **THEN** 系统直接以开放模式管理员身份放行，不因无效令牌或发布令牌返回 401/403；导出鉴权开关保持独立

##### Scenario: 管理员令牌访问管理端正常放行 <a id="scenario-auth-hr-059-3"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理端受控模式下客户端使用合法的管理端令牌发起管理请求
- **THEN** 系统判定令牌作用域合法并正常进入管理业务处理链路

#### Requirement: 标准订阅导出路径与不可变内容服务基准 [HR-060] <a id="req-auth-hr-060"></a>

系统 SHALL 在标准路径 `/publish/v1/{publication_id}` 对外提供客户端订阅配置拉取服务。对于已成功发布且处于激活（active）状态的配置包，系统 MUST 返回不可变二进制或纯文本配置内容，并携带响应头 `Cache-Control: no-store`、匹配的 `Content-Type` 与内容摘要 `ETag`。

##### Scenario: 标准导出路径获取订阅内容成功 <a id="scenario-auth-hr-060-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端请求合法的 `/publish/v1/{publication_id}` 且鉴权校验通过
- **THEN** 系统返回 200 OK、不可变配置内容及对应的配置格式 Content-Type 与 ETag 摘要头

##### Scenario: 不存在的 publication_id 响应 404 <a id="scenario-auth-hr-060-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端请求未在系统注册或已被物理清理的 publication_id
- **THEN** 系统返回 404 Not Found 与错误码 `publication_not_found`

#### Requirement: 鉴权开关独立持久化与领域解耦 [HR-061] <a id="req-auth-hr-061"></a>

系统 SHALL 在 SQLite 数据库 `settings` 表中通过迁移 `000013_auth_settings.sql` 新增 `admin_auth_enabled INTEGER NOT NULL DEFAULT 1` 与 `export_auth_enabled INTEGER NOT NULL DEFAULT 1` 两个独立配置字段。领域层 `domain.Settings` 与仓储层 `SettingsRepository` MUST 提供独立读取与更新管理端鉴权开关和导出链接鉴权开关的能力，二者状态在存储层面与是否存在密码哈希完全解耦。

##### Scenario: 默认初始化状态为管理端与导出端均受控 <a id="scenario-auth-hr-061-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 数据库执行迁移并初始化默认设置或首次读取 settings 记录
- **THEN** 系统解析 `admin_auth_enabled` 为 true 且 `export_auth_enabled` 为 true，保持高安全默认水位

##### Scenario: 独立持久化并更新开关状态 <a id="scenario-auth-hr-061-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理员通过持久化接口单独将 `export_auth_enabled` 置为 false 并保留 `admin_auth_enabled` 为 true
- **THEN** 数据库精准更新对应列，后续查询返回更新后的开关状态且两者互不影响

#### Requirement: 动态鉴权模型独立判定与 Zero-Config 开放受控 [HR-062] <a id="req-auth-hr-062"></a>

系统 SHALL 在 `DynamicTokenHolder` 中解耦管理端鉴权判定：管理端运行模式 `AdminMode()` MUST 优先遵循 `admin_auth_enabled` 开关。当 `admin_auth_enabled == false` 时，管理端 MUST 强制运行于 Zero-Config Open Mode，无论内存或数据库中是否存在有效令牌，匿名与任意请求均直接赋予管理员身份放行；当 `admin_auth_enabled == true` 时，系统依据是否存在有效令牌决定运行于受控保护模式（Token Protected Mode）或未配置令牌时的零配置开放模式。

##### Scenario: 管理端开关关闭时即使存在令牌仍免密放行 <a id="scenario-auth-hr-062-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 系统已配置有效管理令牌但管理员将 `admin_auth_enabled` 显式设置为 false
- **THEN** 客户端匿名访问、携带无效令牌或发布专属令牌访问管理接口均被直接放行并赋予管理员权限，无 401/403 拦截；导出开关状态不受影响

##### Scenario: 管理端开关开启且存在令牌时严格校验凭证 <a id="scenario-auth-hr-062-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 系统 `admin_auth_enabled` 为 true 且已配置有效令牌
- **THEN** 匿名管理请求被拦截为 401，携带匹配令牌或会话的请求正常放行

##### Scenario: 管理端开关开启但未配置令牌时回退至 Zero-Config 开放模式 <a id="scenario-auth-hr-062-3"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 系统 `admin_auth_enabled` 为 true 但服务端从未配置过令牌且数据库 token 为空
- **THEN** 系统自动运行于零配置开放模式，放行请求并在状态接口提示未配置凭据

#### Requirement: 鉴权设置统一 API 契约与向后兼容 [HR-064] <a id="req-auth-hr-064"></a>

系统 SHALL 在 `/api/v1` 路由提供 `GET /api/v1/settings/auth` 与 `PUT /api/v1/settings/auth` 接口，用于统一查询与原子变更鉴权开关及系统令牌。该接口 MUST 兼容既有 `/api/v1/settings/admin-token`（`GET/POST`）及 `/api/v1/auth/status` 端点，且在任何响应中 MUST NOT 返回令牌明文或哈希原串。

##### Scenario: 查询全局鉴权开关与令牌配置状态 <a id="scenario-auth-hr-064-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端以管理员身份请求 `GET /api/v1/settings/auth`
- **THEN** 系统返回 200 OK 与 JSON 数据，包含 `admin_auth_enabled`、`export_auth_enabled`、`admin_mode`、`export_mode` 以及 `token_configured` 布尔标记

##### Scenario: 原子更新鉴权开关与统一令牌 <a id="scenario-auth-hr-064-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端向 `PUT /api/v1/settings/auth` 提交包含鉴权开关与可选新令牌的有效载荷
- **THEN** 系统原子更新数据库与内存状态，记录审计事件并返回更新后的生效状态

##### Scenario: 兼容调用旧版 admin-token 接口 <a id="scenario-auth-hr-064-3"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 旧版管理脚本向 `POST /api/v1/settings/admin-token` 发送请求
- **THEN** 系统正常更新统一系统令牌，更新后的状态同步反映至 `GET /api/v1/settings/auth`

#### Requirement: 前端设置中心独立开关与凭据管理 [HR-065] <a id="req-auth-hr-065"></a>

Web 前端在 `SettingsView.vue` SHALL 分别渲染“管理端访问保护”与“订阅导出链接保护”两个独立开关组件，并提供统一系统令牌的录入、修改、测试与清除功能。界面 MUST 动态展示当前管理端生效模态（受控/免密）与导出链接保护状态（受控/公开），向管理员直观揭示系统安全水位。

##### Scenario: 前端展示独立的管理端与导出端开关 <a id="scenario-auth-hr-065-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 用户进入“设置中心”视图
- **THEN** 界面清晰呈现管理端鉴权开关、导出链接鉴权开关、当前有效安全模态徽章及统一令牌卡片

##### Scenario: 前端切换开关并持久化成功 <a id="scenario-auth-hr-065-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 用户切换任意鉴权开关并点击保存
- **THEN** 前端调用 `PUT /api/v1/settings/auth` 提交变更，收到成功响应后弹出操作提示并刷新安全徽章

#### Requirement: 导出鉴权关闭时的免密公开拉取 [HR-066] <a id="req-auth-hr-066"></a>

当系统 `export_auth_enabled` 设置为 false 时，订阅发布端点 `/publish/v1/{publication_id}` MUST 无条件允许客户端拉取已激活的不可变编译配置，无需在 URL 查询参数或 `Authorization` 请求头中提供任何令牌凭据。无论客户端未携带令牌还是携带无效令牌，系统 MUST NOT 因鉴权返回 401 Unauthorized；不存在或已撤销的发布实体仍按实体状态返回相应错误。管理端鉴权开关独立生效。

##### Scenario: 导出鉴权关闭时匿名客户端直接拉取配置成功 <a id="scenario-auth-hr-066-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 全局配置 `export_auth_enabled` 为 false 且客户端以未带 token 的 GET 请求访问已发布的 `/publish/v1/{id}`
- **THEN** 系统响应 200 OK 并直接输出该发布对应的二进制或文本配置流

##### Scenario: 导出鉴权关闭时携带无效令牌仍提供配置 <a id="scenario-auth-hr-066-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 全局配置 `export_auth_enabled` 为 false，客户端在 URL 的 `token` 参数或 Bearer 请求头中携带无效令牌访问已激活发布
- **THEN** 系统忽略该凭据并响应 200 OK 提供配置，不因无效令牌返回 401

#### Requirement: 导出鉴权开启时的双重令牌验证 [HR-067] <a id="req-auth-hr-067"></a>

当系统 `export_auth_enabled` 设置为 true 时，订阅导出端点 MUST 严格执行客户端身份认证。系统 MUST 依次允许以下两种有效凭据之一通过鉴权：
1. 请求携带的令牌与该 publication 专属的 `token_hash` 经恒定时间对比完全一致；
2. 请求携带的令牌与系统当前配置的全局统一系统令牌（System Master Token）完全一致。
对于已激活的发布实体，凭据缺失或既不匹配专属令牌也不匹配统一系统令牌时，系统 MUST 拒绝请求并响应 HTTP 401 Unauthorized 与错误码 `unauthorized`。此判定不依赖 `admin_auth_enabled`；不存在的发布仍返回 404。

##### Scenario: 携带 publication 专属令牌拉取成功 <a id="scenario-auth-hr-067-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 导出鉴权开启且客户端使用该发布生成时对应的专属 `pub_<hex>` 令牌发起导出请求
- **THEN** 系统校验专属令牌哈希成功并响应 200 OK 交付配置内容

##### Scenario: 携带系统统一主令牌拉取成功 <a id="scenario-auth-hr-067-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 导出鉴权开启且客户端使用管理端配置的全局统一系统令牌发起导出请求
- **THEN** 系统通过统一令牌验证器校验成功并响应 200 OK 交付配置内容，实现令牌复用

##### Scenario: 携带错误令牌或无凭据时被拦截为 401 <a id="scenario-auth-hr-067-3"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 导出鉴权开启且客户端发起未带 token 的请求或使用了无效的随机字符串
- **THEN** 系统拒绝请求并返回 401 Unauthorized 与错误码 `unauthorized`

#### Requirement: 历史短路径 `/p/{publication_id}` 兼容恢复 [HR-068] <a id="req-auth-hr-068"></a>

系统路由层 MUST 识别并支持客户端对 `/p/{publication_id}` 短路径发起的订阅拉取请求，将其无缝对齐至 `/publish/v1/{publication_id}` 的处理逻辑或返回安全的 307 Temporary Redirect 重定向，杜绝因客户端使用历史简化路径而导致的无意义 HTTP 404 故障。

##### Scenario: 请求历史短路径 /p/{id} 成功获取发布内容或正确重定向 <a id="scenario-auth-hr-068-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端向 `/p/{id}` 发起带有合规凭据（或免密模式下匿名）的 GET 请求
- **THEN** 系统成功响应对应发布内容或返回指向 `/publish/v1/{id}` 的 307 重定向并保留原有查询参数

##### Scenario: 不存在的 publication_id 在 /p/{id} 返回 404 <a id="scenario-auth-hr-068-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 客户端向 `/p/{non_existent_id}` 发起请求且数据库中不存在该发布记录
- **THEN** 系统返回 404 Not Found 与错误码 `publication_not_found`

#### Requirement: 零发布记录诊断与工作台发布向导对齐 [HR-069] <a id="req-auth-hr-069"></a>

当数据库 `publications` 表当前有效记录数为 0 时，系统与前端 MUST 给出清晰的状态解释与指引。Web 前端在 `PublicationsView.vue` 中 MUST 区分“没有生成发布”与“系统网络故障”，向用户展示醒目的发布引导卡片，明确告知需要点击“发布当前配置”以生成首个不可变订阅发布实体与下载端点。

##### Scenario: 首次进入无发布记录时展示清晰发布向导 <a id="scenario-auth-hr-069-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理员打开发布管理视图且当前 target 没有任何已发布的记录
- **THEN** 界面展示空状态向导，提示当前尚未注册不可变发布，并提供引导点击发布操作

##### Scenario: 点击发布后即时生成新发布并呈现正确链接 <a id="scenario-auth-hr-069-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理员点击“发布当前配置”并确认
- **THEN** 系统成功创建 publication 记录并在界面呈现新的导出卡片与导出地址

#### Requirement: 前端导出链接与复制行为动态适配 [HR-070] <a id="req-auth-hr-070"></a>

Web 前端在 `PublicationsView.vue` 及 `usePublications.ts` 中 MUST 根据当前生效的 `export_auth_enabled` 配置动态格式化客户端订阅地址：
- 当 `export_auth_enabled == false` 时，生成的直链 MUST 为纯净免密地址格式：`http(s)://<host>/publish/v1/{id}`；
- 当 `export_auth_enabled == true` 时，生成的直链 MUST 自动追加令牌参数：`http(s)://<host>/publish/v1/{id}?token={token}`；
- 用户点击“复制订阅链接”按钮时，复制至剪贴板的内容 MUST 与当前导出鉴权模态完全一致，并在界面提示当前链接类型（公开直链或受控令牌链接）。

##### Scenario: 免密模式下复制纯净直链 <a id="scenario-auth-hr-070-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 导出鉴权处于关闭状态且用户点击复制当前订阅链接
- **THEN** 写入剪贴板的 URL 不包含 `?token=` 查询参数，界面提示已复制免密公开直链

##### Scenario: 受控模式下复制包含令牌的完整订阅链接 <a id="scenario-auth-hr-070-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 导出鉴权处于开启状态且用户点击复制当前订阅链接
- **THEN** 写入剪贴板的 URL 包含合规的 `?token=` 查询参数，界面提示已复制包含凭据的订阅链接

### Explicitly Pending Historical Stubs

#### Requirement: 鉴权双模态与空配置自动开放 [HR-020] <a id="req-auth-hr-020"></a>

系统 SHALL 在服务端未配置管理令牌（环境变量 `CSP_ADMIN_TOKEN` 为空或未设置）时自动运行于零配置开放模式（Zero-Config Open Mode）。在此模式下，任何未携带 Bearer 令牌或 Cookie 的管理 API 请求 MUST 自动获得管理员权限（`ActorKindAdmin`、`Subject: "admin"`、`AuthMethod: "open_mode"`、`Scopes: ["admin"]`），且 MUST 不返回 401 错误。当服务端配置了非空管理令牌时，系统 MUST 运行于受控保护模式（Token Mode），严格执行 Bearer 令牌与会话 Cookie 校验，无凭据或凭据不匹配的请求 MUST 被拦截为 401。

##### Scenario: 开放模式下匿名管理请求成功 <a id="scenario-auth-hr-020-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 服务端未配置 AdminToken 且客户端发起未带凭据的 `/api/v1/` 管理请求
- **THEN** 系统以 HTTP 成功状态响应并返回 `{ "data": ... }`，请求上下文被赋予管理员身份

##### Scenario: 保护模式下匿名管理请求被拦截 <a id="scenario-auth-hr-020-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 服务端配置了非空 AdminToken 且客户端发起未带凭据的 `/api/v1/` 管理请求
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误消息 `Authentication credentials required`

##### Scenario: 保护模式下匹配 Bearer 令牌成功放行 <a id="scenario-auth-hr-020-3"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 服务端配置了非空 AdminToken 且客户端在 `Authorization` 头携带匹配的 `Bearer <token>` 发起管理请求
- **THEN** 系统以 HTTP 成功状态响应并正常返回业务数据

##### Scenario: 保护模式下错误 Bearer 令牌被拒绝 <a id="scenario-auth-hr-020-4"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 服务端配置了非空 AdminToken 且客户端在 `Authorization` 头携带不匹配的 Token 发起管理请求
- **THEN** 系统返回 401 Unauthorized 与错误码 `unauthorized` 及错误消息 `Invalid authentication credentials`

#### Requirement: 真实无头浏览器双模态对抗断言 [HR-024] <a id="req-auth-hr-024"></a>

Critic MUST 驱动真实的无头浏览器（Playwright 或 CDP）对系统进行双重视角对抗性验收：
1. 视角一（开放模式匿名基准质检）：在容器未配置 `CSP_ADMIN_TOKEN` 的默认环境下，以完全清空 `localStorage` 的匿名浏览器会话访问系统，连续点击切换 Subscriptions、Node Ledger、Probe Engine、Policy Tree 视图，断言所有接口 HTTP 状态码为 200，页面不得出现“Authentication credentials required”红色错误横幅，核心 DOM 列表必须成功渲染。
2. 视角二（保护模式受控流转质检）：在配置了测试 Admin Token 的环境下，匿名浏览器发起管理请求时必须断言捕获 401 并自动弹出 `AuthModal`，通过脚本模拟键入正确 Token 并保存后，断言后续请求自动携带 Token 且数据恢复正常。

##### Scenario: 开放模式匿名访问真机质检通过 <a id="scenario-auth-hr-024-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** Critic 驱动匿名无头浏览器访问处于开放模式的真实容器工作台，并依次点击各功能视图
- **THEN** 所有视图平滑加载，捕获到的所有 `/api/v1/` 请求状态码均为 200，无 401 报错，Critic 给出通过裁决

##### Scenario: 保护模式凭据唤起与自愈质检通过 <a id="scenario-auth-hr-024-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** Critic 驱动匿名无头浏览器访问处于保护模式的真实后端，并在 401 弹出 `AuthModal` 后填入合法 Token
- **THEN** 凭据成功保存至 `localStorage`，视图重新请求并全量正常渲染，无后续 401 发生

#### Requirement: 顶部安全水位指示器 [HR-028] <a id="req-auth-hr-028"></a>

工作台顶部导航栏 SHALL 在健康状态探针旁呈现明确的鉴权状态徽章（Auth Badge）。当通过 `GET /api/v1/auth/mode` 探测到服务处于开放模式时，徽章 MUST 显示为“Open Mode”（带有温和提示色彩）；当探测到处于受控保护模式时，徽章 MUST 显示为“Protected”，点击该徽章可直接调起凭据管理弹窗。

##### Scenario: 开放模式状态回显 <a id="scenario-auth-hr-028-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 工作台加载且后端响应 `auth/mode` 为 `open`
- **THEN** 顶部栏渲染“Open Mode”安全徽章，悬停显示“当前为零配置私有开放模式”

##### Scenario: 保护模式状态回显 <a id="scenario-auth-hr-028-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 工作台加载且后端响应 `auth/mode` 为 `token`
- **THEN** 顶部栏渲染“Protected”安全徽章，直观表明控制面已受令牌防护

#### Requirement: 统一系统令牌复用与管理 [HR-063] <a id="req-auth-hr-063"></a>

系统 SHALL 支持配置单一全局统一系统令牌（System Master Token），该令牌可同时用于管理端访问认证以及导出链接下载鉴权。系统更新或清空统一令牌时，MUST 使用恒定时间对比与安全哈希（Bcrypt / SHA-256）持久化至数据库，并发安全刷新内存中 `DynamicTokenHolder` 的验证器快照，并立即使所有既有管理 Cookie 会话失效。

##### Scenario: 更新统一系统令牌成功生效 <a id="scenario-auth-hr-063-1"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理员提交新的统一系统令牌字符串
- **THEN** 系统完成安全持久化与快照刷新，后续管理端与开启导出鉴权时的拉取请求均可使用该新令牌通过认证

##### Scenario: 清空系统令牌安全回退 <a id="scenario-auth-hr-063-2"></a>
- **GIVEN** 会话认证与访问控制策略已启用
- **WHEN** 管理员提交空字符串清空统一系统令牌
- **THEN** 系统清空数据库中的令牌哈希并重置内存验证器，吊销所有当前已登录会话


---

## Success Criteria

### Measurable Outcomes
- **[SC-AUTH-01]** 控制台与导出双开关独立生效隔离度达到 100%。
- **[SC-AUTH-02]** 会话 Cookie HttpOnly 标志覆盖率达到 100%。
- **[SC-AUTH-03]** 针对变更请求的 CSRF 攻击防御拦截率达到 100%。
- **[SC-AUTH-04]** 主令牌常量时间比对防时序攻击安全性达标。

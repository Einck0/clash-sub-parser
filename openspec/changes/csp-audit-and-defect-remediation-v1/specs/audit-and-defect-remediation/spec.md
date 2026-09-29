## Purpose

定义 CSP 1.0 全栈经审计确证的功能缺陷修复与可靠性加固契约，包括初始修订自动引导、规则安全删除、探针并发安全与精确错误分类、观测分页准确性、订阅删除孤儿节点级联去活、前端 Cookie 会话 CSRF 守卫、局域网安全降级、性能风暴规避以及生产端口环境隔离。

## ADDED Requirements

### Requirement: 初始配置修订引导与显式创建
系统在全新数据库初始化启动时，SHALL 确保存在一个初始有效且处于激活状态（Active）的配置修订（Configuration Revision）。当客户端向 `/api/v1/policies/rules` 或 `/api/v1/admission/rules` 发送新增规则请求且未显式指定 `revision_id` 时，系统 MUST 自动绑定至当前的活跃修订，不得返回 422 `missing_revision_id` 错误。系统 SHALL 在 `/api/v1/revisions` 暴露 `POST` 接口，允许创建新的配置修订。发布预览在初始状态下若拓扑图有效，MUST NOT 抛出 409 `no_active_revision` 错误。

#### Scenario: 全新数据库首次启动后创建分流规则成功
- **WHEN** 客户端连接全新的空白数据库并请求 `POST /api/v1/policies/rules` 且不传 `revision_id`
- **THEN** 服务端自动将其归属于初始引导的活跃配置修订，返回 201 Created 且包含合法的 `revision_id`，不得返回 422 错误

#### Scenario: 初始活跃修订状态下发布预览成功
- **WHEN** 客户端在全新数据库启动后请求 `POST /api/v1/publications/preview`
- **THEN** 服务端成功生成预览响应，不得返回 409 `no_active_revision` 冲突错误

### Requirement: 引导与显式活跃修订原子创建
当初次引导或显式创建活跃修订时，系统 SHALL 原子地创建并激活修订；激活失败 MUST 返回错误且回滚创建及任何旧活跃状态更改，不得记录成功审计。下一次启动 MUST 可在无修订时安全重试；已有用户草稿或历史规则 MUST 保留且不得自动激活。数据库查询错误 MUST 原样传递给调用方。独立 `revision.Service` 的草稿审查与显式激活语义保持不变。

#### Scenario: 首次引导激活暂时失败后重启
- **WHEN** 全新数据库首次启动时激活步骤失败一次，随后以同一数据库重新启动服务
- **THEN** 首次启动返回错误且不遗留草稿，第二次成功生成初始活跃修订；已有用户草稿和规则不会被隐式激活或删除

#### Scenario: 显式创建活跃修订激活失败
- **WHEN** `policy.Service.CreateRevision(StateActive)` 的激活步骤失败
- **THEN** 调用返回失败，不存在此次请求的新草稿、假活跃状态或成功审计，原活跃版本仍保持活跃

### Requirement: 策略准入规则与分流规则安全删除
系统 SHALL 提供 `DELETE /api/v1/policies/rules/{id}` 端点，支持按规则唯一标识符安全删除准入规则（Admission Rule）或分流规则（Policy Rule）。删除成功后 MUST 返回 204 No Content，后续查询该规则所属修订的规则列表时 MUST 不再包含已被删除的规则，且操作 MUST 记录审计事件。

#### Scenario: 删除已存在的策略规则成功
- **WHEN** 客户端向 `DELETE /api/v1/policies/rules/{rule_id}` 发送删除请求
- **THEN** 服务端返回 204 No Content，且后续 `GET /api/v1/policies/rules` 响应中不再包含该规则

#### Scenario: 删除不存在的规则返回 404
- **WHEN** 客户端向 `DELETE /api/v1/policies/rules/{non_existent_id}` 发送请求
- **THEN** 服务端返回 404 Not Found 且错误码为 `rule_not_found`

### Requirement: 遗留历史路由返回 410 Gone
系统针对 Python 时代的历史路由 `GET /yaml` 与 `GET /script`，SHALL 显式响应 HTTP 410 Gone 状态码，并附带迁移指引，MUST NOT 降级匹配为 SPA 单页应用静态资源（HTTP 200）。

#### Scenario: 访问遗留 /yaml 端点
- **WHEN** 客户端请求 `GET /yaml`
- **THEN** 服务端直接响应 HTTP 410 Gone，内容说明该遗留端点已被永久移除

#### Scenario: 访问遗留 /script 端点
- **WHEN** 客户端请求 `GET /script`
- **THEN** 服务端直接响应 HTTP 410 Gone，内容说明该遗留端点已被永久移除

### Requirement: 探针连接并发关闭防死锁
`safeRuntimeConn` 在底层网络连接处于阻塞读取（Blocked `Read()`）状态时，调用方执行 `Close()` 时 MUST 能够立即中断底层连接并退出，SHALL NOT 发生死锁。系统 SHALL 在获取互斥锁之前，主动设置底层连接的读写超时截止时间，并逐层递归关闭底层传输套接字。

#### Scenario: 阻塞读取中被外部并发关闭
- **WHEN** 工作协程正在 `safeRuntimeConn.Read()` 阻塞等待数据，另一个协程调用 `safeRuntimeConn.Close()`
- **THEN** `Close()` 立即促使 `Read()` 返回超时或连接关闭错误，`Close()` 本身在毫秒级正常返回，无死锁悬挂

### Requirement: 探针拨号器错误精确分类
系统在对单节点执行拨号前检查时，若遭遇目标域名 DNS 解析失败、无公网 IP 解析记录或目标 IP 触发私网/保留地址 SSRF 拦截，SHALL 将其分类为节点级网络/准入失败（如 `ErrTargetUnresolvable`、`ErrPrivateTargetRejected`），并为该节点记录 `VerdictUnavailable` 观测证据。此类单节点目标错误 MUST NOT 判定为系统级致命凭据故障（`ErrCredentialsUnavailable`），MUST NOT 导致整个探测批次被标记为 `ProbeRunStateFailed`。

#### Scenario: 节点域名无法解析不影响全局探测批次
- **WHEN** 探测批次包含包含 1 个不可解析域名的故障节点与 5 个正常的代理节点
- **THEN** 故障节点记录为不可用（VerdictUnavailable），其余 5 个正常节点继续执行阶段 2 探测，整个探测批次最终状态为成功（Succeeded）

#### Scenario: 节点指向私有回环地址被 SSRF 拦截
- **WHEN** 某节点的服务器地址为 `127.0.0.1` 或 `10.0.0.1`
- **THEN** 安全拨号器拒绝连接并生成不可用观测记录，探测运行批次不发生中断

### Requirement: 节点探针观测记录真实分页与计数
端点 `GET /api/v1/nodes/{logical_id}/observations` SHALL 支持标准分页参数（`page` 与 `page_size`），仓储层在数据库查询时 MUST 使用 SQL 级别的 `LIMIT ? OFFSET ?`，且返回的 `total` 字段 MUST 来自符合条件的实际行数计数（`COUNT(*)`），SHALL NOT 使用内存截断列表的长度替代。

#### Scenario: 节点拥有超过当前分页大小的历史记录
- **WHEN** 数据库中某节点拥有 45 条观测记录，客户端请求 `page=1&page_size=20`
- **THEN** 服务端返回 20 条当前页记录，且响应元数据中的 `total` 精确为 45，`page` 为 1，`page_size` 为 20

### Requirement: 订阅源删除级联去活孤儿节点
当客户端调用 `DELETE /api/v1/subscriptions/{id}` 删除订阅源时，系统在清除 `subscriptions` 及 `node_sources` 关联记录后，MUST 原子化地将所有失去全部来源关联且仍处于活跃状态的节点标记为停用（`active = 0`），确保节点台账（Node Ledger）中不会遗留不受任何订阅源管辖的僵尸活跃节点。

#### Scenario: 删除唯一拥有某批节点的订阅源
- **WHEN** 订阅源 A 独占拥有节点 X 与节点 Y，客户端调用删除接口删除订阅源 A
- **THEN** 订阅源 A 被删除，节点 X 与 Y 的 `active` 状态自动变更为 `0`（非活跃），不再出现在默认活跃节点列表与探测候选集中

### Requirement: 订阅源 PATCH 空白凭据引用严格校验
当客户端向 `PATCH /api/v1/subscriptions/{id}` 提交包含空白字符串或纯空白字符的 `source_url_secret_ref` 字段时，传输层 MUST 将该字段原样传递给业务应用层，业务应用层 MUST 判定其为非法输入，并返回 422 Unprocessable Entity 错误（错误码 `invalid_source_url_secret_ref`），SHALL NOT 静默忽略。

#### Scenario: 提交纯空格凭据引用进行更新
- **WHEN** 客户端向订阅源 PATCH 请求体中传入 `{"source_url_secret_ref": "   "}`
- **THEN** 服务端返回 422 状态码与 `invalid_source_url_secret_ref` 错误响应，拒绝更新

### Requirement: 前端 Cookie 会话认证附带 CSRF 令牌
在启用 Cookie 会话的认证模式下，前端 `ApiClient` 与 `useAuth` SHALL 在用户成功登录时持久化保持服务端返回的 `csrf_token`。在随后发起的全部状态变更请求（POST、PUT、PATCH、DELETE）中，`ApiClient` MUST 自动在请求头中注入 `X-CSRF-Token`，以满足服务端 `middleware_auth.go` 的防跨站伪造校验。

#### Scenario: 登录后发起订阅创建请求
- **WHEN** 用户使用管理员密码登录成功，随后在界面创建新订阅源
- **THEN** 请求自动携带包含有效 CSRF 令牌的 `X-CSRF-Token` 请求头，服务端校验通过并返回 201 Created

### Requirement: 纯 HTTP 局域网环境下的 UUID 生成安全降级
前端在生成请求幂等键（如 `Idempotency-Key`）时，SHALL 检测当前执行上下文。若原生 `crypto.randomUUID` 在非安全上下文（纯 HTTP 局域网 IP）下为 `undefined`，系统 MUST 自动采用符合 RFC 4122 v4 规范的确定性退避伪随机算法生成合法 UUID，不得抛出未捕获的运行时异常。

#### Scenario: 局域网 HTTP 地址下触发订阅立即刷新
- **WHEN** 用户通过 `http://192.168.1.100:18080` 访问前端界面并点击刷新订阅
- **THEN** 系统顺利生成合法格式的 UUID v4 幂等凭据并完成网络请求，控制台无报错

### Requirement: 探测工作台轮询风暴消除
`ProbesView.vue` 与 `useProbes.ts` 在后台周期同步期间，SHALL NOT 在每个 2 秒心跳周期内循环发起最多 20 页的全量节点请求。当存在运行中探测任务时，系统 SHALL 仅针对活跃运行状态（Runs）、队列状态（Pool Status）与关联观测执行轻量轮询；节点列表仅在单次探测完成、任务启停或用户显式触发时进行针对性刷新。

#### Scenario: 探测运行中后台心跳轮询
- **WHEN** 探测工作台正在执行批次探测
- **THEN** 2 秒定时器仅请求运行状态与观测，不发起全量节点台账的深度分页扫描

### Requirement: 节点台账筛选状态下的无限滚动防饥饿
在 `NodesView.vue` 启用健康度客户端筛选（如筛选 degraded 或 unhealthy）时，若当前已加载的匹配条目数量不足以填满用户视口高度且后端仍有更多数据（`hasMore === true`），组件 SHALL 自动触发后续分页加载（`loadMore`），直到视口填满或数据全部加载完毕，杜绝因未达滚动阈值而导致的列表假性空白与死锁。

#### Scenario: 筛选少量异常节点触发自动补充分页
- **WHEN** 用户在拥有 500 个节点的台账中切换至“异常”筛选，第一页数据中无异常节点
- **THEN** 页面检测到视口未充满且存在后续分页，自动递归请求下一页，直至找到异常节点渲染或全部数据读取完毕

### Requirement: 订阅源配置抽屉必填校验守卫
在 `SubscriptionConfigDrawer.vue` 中，当用户点击抽屉底部的“保存”按钮时，系统 MUST 优先触发表单的完整性校验（涵盖当前激活与未激活 Tab 的必填字段，包括名称与订阅地址）。若任一必填字段为空或非法，系统 SHALL 阻止向父组件派发 `save` 事件，并向用户提示具体的校验错误。

#### Scenario: 在高级配置 Tab 中直接点击保存未填名称的订阅
- **WHEN** 用户打开新建订阅抽屉，直接切换至“高级设置”Tab 并点击底部的保存按钮
- **THEN** 表单拦截提交动作，自动聚焦或提示“基础设置”中的订阅名称与地址必填项，不发起网络请求

### Requirement: 策略组 REST 路由路径标识符安全编码
前端 `usePolicy.ts` 在拼接策略组及连接边的 REST 接口 URL（如 `/api/v1/policies/groups/{id}`、`/edges` 等）时，MUST 对传入的组标识符（`id` / `groupId`）使用 `encodeURIComponent` 进行标准 URL 编码，杜绝特殊字符破坏请求路由。

#### Scenario: 操作包含特殊字符的策略组
- **WHEN** 客户端更新或删除 ID 包含特殊符号或空格的策略组
- **THEN** 构造的 HTTP 请求路径正确编码，服务端正确匹配路由

### Requirement: 发布中心刷新后复制链接保持完整令牌
当用户成功发布订阅配置后，系统 SHALL 在客户端安全持久化（localStorage 或与发布 ID 关联的键值缓存）记录该次发布返回的包含有效 `token` 的完整导出 URL。当用户刷新页面并重新进入发布中心时，点击“复制订阅链接”按钮 MUST 复制带有 `?token=...` 的有效可访问 URL，不得复制缺少鉴权令牌的裸路径。

#### Scenario: 用户刷新浏览器后复制已发布订阅链接
- **WHEN** 用户创建了发布配置，随后刷新浏览器页面并点击“复制订阅链接”
- **THEN** 剪贴板中获取的链接为包含当前有效 token 的完整 URL，客户端以此 URL 可直接通过鉴权下载配置

### Requirement: 发布异步响应隔离当前目标
用户选中的编译目标 SHALL 不被其它目标的延迟发布或预览响应反选。成功发布结果 MUST 按请求目标保存并可在切回该目标时恢复，且只有当前选中目标的最新请求能更新可见发布信息、预览或错误。

#### Scenario: 发布 Mihomo 时切换 sing-box 后才收到成功响应
- **WHEN** Mihomo 发布请求仍未返回，用户切换至 sing-box 后收到 Mihomo 成功响应
- **THEN** 页面仍选中 sing-box 且不显示 Mihomo 发布信息，切回 Mihomo 时可恢复这条发布信息与带令牌链接

### Requirement: 17000 端口生产环境隔离约束
宿主机 17000 端口 SHALL 被严格认定为生产 Docker 容器映射端口别名，严禁任何自动化测试用例、预览脚本或端到端 Harness 向 `127.0.0.1:17000` 或 `127.0.0.1:18080` 发起测试写入或启动服务冲突。所有集成测试与端到端测试 MUST 使用系统随机分配的临时端口（端口 `0`）与独立的临时 SQLite 数据库。

#### Scenario: 自动化集成测试与 E2E 启动服务
- **WHEN** 运行 smoke_test.sh、Go 集成测试或 Playwright E2E 测试
- **THEN** 测试 Harness 分配未占用的动态端口并挂载独立临时目录，不与 17000/18080 端口或生产数据卷发生任何交互

### Requirement: 端到端测试多语言环境鲁棒性断言
`web/e2e/auth-real-server.spec.ts` 在启动真实浏览器上下文时，SHALL 显式设置固定的语言标识（通过注入 `csp_locale: 'en-US'` 或覆盖浏览器默认 Accept-Language），或采用双语/不依赖硬编码自然语言的 `data-testid` 属性进行 UI 元素断言，确保测试在默认中文环境下保持 100% 确定性通过。

#### Scenario: 在中文默认系统环境中运行真实服务端到端测试
- **WHEN** 执行 Playwright E2E 真实服务器认证对抗测试
- **THEN** 测试由于明确的语言基线配置，无语言文字不匹配失败，全部用例执行通过

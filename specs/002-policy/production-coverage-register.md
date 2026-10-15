# Production Operations & Verification Coverage Register (Stage Plan)

> **治理状态**: `STAGE_PLAN_READY_NOT_EXECUTED` (阶段计划·合同就绪·尚未实机执行·严禁宣称全量上线 PASS)
> **运行态专案**: `pi_run_20261011_190715_2256208`
> **关联变更**: `002-policy`
> **执行与报告治理声明**:
> - **阶段准备明确**: 本阶段为全量操作登记与计划编制阶段，所有 94 项操作均为 `NOT_EXECUTED`，**主脑决不采信 `whole_gate=PASS`，终态门禁保持 `terminal_gate_pass: false`**。
> - **来源与范围界定**: 本清单 94 项操作是基于全目录扫描实际已注册路由（`internal/transport/http/*.go`）与前端交互页面控件（`web/src/features/*`、`web/src/App.vue` 等）逆向推导的活动能力，**并非声称历史需求目录（如 259 项条款）已全部核验通过**；
> **输入工件真实溯源**:
> - `/tmp/csp-resume-20261011/full-production-coverage.json` (SHA-256: `ed28fee0672bdc5a4b300c678302bec207721dba05b8ece6f22f3936d4e55754`, permissions: `0600`)
> - `/tmp/csp-resume-20261011/production-coverage-preflight.json` (SHA-256: `5be37ba338442c0dd0738a320ebbda0c22f223972764d7f512b0268d051ff364`)
> - `/tmp/csp-resume-20261011/policy-scout.json` (SHA-256: `ce930f10f00d0de10b8a99a0a1c684d3be901bbcc227c0b00e22a9652a5a0485`)
> **项目规格与历史索引关联**:
> - [Spec Definition](./spec.md) | [Prior Requirements Register](./prior-requirements-register.md) | [Contracts](./contracts.md) | [Data Model](./data-model.md)
> **统计口径总览 (互斥分类计数)**:
> - 全量独立可验操作总数: **94** 项 (全量当前状态: `NOT_EXECUTED`)
> - 只读检验项 (ReadOnly): **56** 项
> - 安全受控变更项 (Safe Mutation with Preimage/Cleanup): **30** 项
> - 本地隔离补充项 (Isolated Supplement Limited Not Passed): **4** 项 (因缺生产安全前提，仅限隔离补充验证，不得视为生产全量已覆盖)
> - 需受权前提阻断项 (Blocked by Auth Live Precondition): **3** 项 (Token轮换/清空/关闭认证需安全会话与明确授权，等待合法safe步骤非自我豁免)
> - 真实移动设备通道受阻项 (Blocked by Phone Channel): **1** 项 (通道 connect refused / device alive 尚未核真实 model 与 ARM，不能与 Termux root 等价)
> - 存量资产底册: 9 个订阅源、90 个活动节点、29 个策略组、162 条分流规则、5 个发布配置、3591 次探测运行、0 条准入规则。

---

## 1. 架构原则、执行次序与安全防护红线 (Architecture, Ordering & Safety Protocol)

### 1.1 真实物理入口与内部映射关系 (Internal Ports vs Public Domain)
- **公网对外访问域名**: `https://sub.einck.top/` (由 Ubuntu Nginx 反代挂载 TLS，强制开启 Protected 模式控制面访问保护)；
- **宿主机直接访问入口**: `http://127.0.0.1:18080/` (Docker 端口直通，用于容器健康检查与内部直连)；
- **宿主机 Nginx 上游入口**: `http://127.0.0.1:17000/` (Nginx `proxy_pass` 目标，映射到容器内 18080)；
- **架构铁律**: `internal != domain`。生产真实端到端测试必须严格区分内部直连与公网反代行为，特别是在 Cookie 作用域、HTTPS 重定向与 CSRF 校验场景。

### 1.2 生产执行顺序协议 (Strict Sequential Ordering)
1. **事前热备份保底 (Backup Before Prod Write)**: 在发起任何可能引起生产状态变更的操作前，必须执行 SQLite 物理热备份：
   ```bash
   sqlite3 /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db ".backup '/home/service/backups/csp-acceptance-preimage-hot.db'"
   sqlite3 /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db "PRAGMA integrity_check;"
   ```
2. **最小合法测试源限制**: 仅使用测试专用的独立 mock 订阅源（如 `http://127.0.0.1:18080/mock-sub-acceptance`），严禁自动静默刷新既有 9 个高负荷生产订阅；若需引入新测试源，必须明确依赖由用户显式提供；
3. **前像与事务逆向清理 (Preimage & Cleanup)**: 每个测试对象严格采用 `CSP-acceptance-2209039-<slug>` 专属前缀，并在断言完成后通过 DELETE / PATCH 逆向清理，还原资产计数；
4. **事后完整性校验**: 逆向清理完成后，再次执行 `foreign_key_check` 与 `integrity_check` 确保零孤儿记录与零数据库污染。

### 1.3 真实外网流量与耗时预算提议 (External Network Budget Proposal)
- **外网响应体总流量上限**: **128 MiB**；
- **外网活动阶段总耗时上限**: **30 分钟**；
- **并发连接与测速限制**:
  - 存活可用性探测 (Alive Concurrency): `<= 4` 个并发；
  - 媒体与 AI 解锁探测 (Media Concurrency): `<= 2` 个并发；
  - 节点测速 (Speed Test Concurrency): `<= 1`（严格串行）；
  - 测速节点配额: 全量验证最多仅选取 **2 个** 代表性节点，每个节点单次下载预算不超过 **4 MiB**；
- **后台常态调度隔离**: 后端系统每 30 分钟执行的常规存活探测属于生产背景常态调度，不计入本次专项验收预算；
- **性质声明**: 本预算为执行前安全约束提议（Execution Safety Proposal），本任务阶段未向外网实际发起任何超限请求。

### 1.4 凭据安全脱敏与管理员显示设计边界 (Secret Avoidance vs UI Requirement)
- **测试证据脱敏 (Test Evidence Sanitization)**: 严禁将明文 Admin Token、订阅原 URL、节点密码、UUID 或 Session Cookie 输出至 shell stdout、测试报告文档或截图中；凭据断言必须通过哈希值（Digest）或掩码校验；
- **不修改 UI 既有设计契约**: 本约束仅为测试取证纪律，**绝非将管理员长期显示设计或前端表单明文显示需求强行变更为掩码**。UI 产品层对管理凭据的交互行为保持既有设计，不借由会话测试擅自增加不可逆的 UI 变更；
- **禁止转储会话目录**: 严禁执行 `dump /sessions` 或导出数据库明文凭据表。

### 1.5 线上故障注入与高危操作隔离 (Fault Injection & Critical Operations)
- **绝不人为制造线上事故**: 负向失败路径验证（如超时、502 网关错误、DAG 成环拦截）仅在受控隔离测试夹具中进行，严禁对生产运行态进行网络阻断或数据污染；
- **4 项隔离补充项受限说明 (ISOLATED_SUPPLEMENT_LIMITED_NOT_PASSED)**: 包含上游拉取失败容灾 (`COV-ACT-023`)、DAG 成环拦截 (`COV-ACT-056`)、修订激活演练 (`COV-ACT-084`) 与 IP 风险策略激活 (`COV-ACT-087`)。由于生产环境缺少安全故障注入前提，仅限隔离补充验证，**绝不能以本地 isolated 通过冒充生产全量通过**，待建立安全生产前置条件后补验；
- **3 项认证高危操作非豁免说明 (LIVE_PRECONDITION_BLOCKED)**: 包含 Admin Token 轮换 (`COV-ACT-079`)、Token 清空 (`COV-ACT-080`) 与管理认证关闭 (`COV-ACT-078`)。由于可能导致现有操作会话丢失或线上服务脱保，必须等待合法安全的交接步骤与已授权 Session，**此非自我豁免**。当前管理端认证默认保持开启；未获明确授权绝不尝试关闭。

### 1.6 真实移动端硬件物理边界 (Mobile Physical Boundary)
- **硬件真机标准**: 移动端验收必须在真实移动设备上执行；
- **当前通道状态**: `通道 connect refused / device alive 尚未核真实 model 与 ARM`（端口 8022 拒绝连接），不能与 Termux root 等价，硬件型号与环境未被核验；
- **严禁桌面伪造**: 坚决杜绝使用桌面浏览器缩小 Viewport 尺寸冒充真实移动端体验；该项标记为 `LIVE_PRECONDITION_BLOCKED`。

---

## 2. 全量 94 项独立可验证操作明细登记表 (Complete Operations Register)

### App Shell / Global (5 项独立操作)

#### [COV-ACT-001] Theme Switcher dropdown (7 DaisyUI themes) [Client UI]
- **所属页面/视口**: `App Shell / Global`
- **关联规格/条款**: `FR-UI-06, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/, Internal: http://127.0.0.1:17000/ (internal upstream != public domain)`
- **安全业务对象**: `CSP-acceptance-2209039-theme`
- **执行步骤**: 1. Open https://sub.einck.top/; 2. Locate theme selector dropdown in navbar; 3. Switch sequentially through light, dark, dim, cyberpunk, cupcake, dracula, nord; 4. Refresh page to verify persistence.
- **断言要求 (严禁纯200)**: HTML root element attribute data-theme updates to selected theme string; localStorage.getItem('csp_theme') matches selected theme; CSS variables update accordingly.
- **前像备份与清理方案**: Preimage: read localStorage('csp_theme'); Cleanup: restore initial theme in localStorage.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `120ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-002] i18n Language Toggle (zh-CN <-> en-US) [Client UI]
- **所属页面/视口**: `App Shell / Global`
- **关联规格/条款**: `FR-UI-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-locale`
- **执行步骤**: 1. Click language toggle button in navbar; 2. Verify all UI headings, menu items, and badges translate immediately between Simplified Chinese and English; 3. Verify localStorage persistence.
- **断言要求 (严禁纯200)**: DOM elements update text dynamically (e.g. 'Dashboard' <-> '仪表盘', 'Subscriptions' <-> '订阅源管理'); localStorage.getItem('csp_locale') updates to 'zh-CN' or 'en-US'.
- **前像备份与清理方案**: Preimage: read localStorage('csp_locale'); Cleanup: restore initial locale.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `80ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-003] Sidebar Collapse / Expand Toggle [Client UI]
- **所属页面/视口**: `App Shell / Global`
- **关联规格/条款**: `FR-UI-06, HR-150`
- **测试目标位置**: `Public URL: https://sub.einck.top/, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-sidebar`
- **执行步骤**: 1. Click sidebar toggle button (Chevron icon); 2. Observe sidebar width transition; 3. Verify localStorage persistence across reloads.
- **断言要求 (严禁纯200)**: Sidebar element toggles CSS class between w-64 and w-16; labels hide on collapse; localStorage.getItem('csp_sidebar_collapsed') toggles between 'true' and 'false'.
- **前像备份与清理方案**: Preimage: read localStorage('csp_sidebar_collapsed'); Cleanup: restore original state.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `60ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-004] Global Toast Alert Notifications [Client UI]
- **所属页面/视口**: `App Shell / Global`
- **关联规格/条款**: `FR-UI-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-toast`
- **执行步骤**: 1. Trigger an asynchronous operation resulting in toast notification (e.g. copy link or validation alert); 2. Inspect ToastContainer component.
- **断言要求 (严禁纯200)**: Toast element rendered with corresponding tone class (alert-success, alert-warning, alert-error); message matches locale string; auto-dismisses after 4000ms.
- **前像备份与清理方案**: Preimage: None; Cleanup: transient DOM element auto-dismissed.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `50ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-005] Confirmation Modal Dialog Trap & Cancel [Client UI]
- **所属页面/视口**: `App Shell / Global`
- **关联规格/条款**: `FR-UI-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-modal`
- **执行步骤**: 1. Trigger modal dialog (ConfirmModal); 2. Press Escape key or click Cancel; 3. Trigger modal and verify focus trap on action buttons.
- **断言要求 (严禁纯200)**: Modal backdrop renders with aria-modal='true'; pressing Escape or Cancel cancels promise without mutating backend state; clicking backdrop dismisses.
- **前像备份与清理方案**: Preimage: None; Cleanup: Cancel closes dialog without state modification.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `50ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

### Auth & Security Gate (7 项独立操作)

#### [COV-ACT-006] Public Auth Status Probe [GET /api/v1/auth/status]
- **所属页面/视口**: `Auth & Security Gate`
- **关联规格/条款**: `FR-AUTH-01, FR-AUTH-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/auth/status, Internal: http://127.0.0.1:18080/api/v1/auth/status`
- **安全业务对象**: `CSP-acceptance-2209039-authstatus`
- **执行步骤**: 1. Send GET request to /api/v1/auth/status without credentials.
- **断言要求 (严禁纯200)**: HTTP 200 OK; JSON body matches schema with mode=='protected', admin_mode=='protected', export_mode=='protected', authenticated==false, subject==''.
- **前像备份与清理方案**: Preimage: None; Cleanup: None (read-only query).
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `50ms / 180 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-007] Admin Session Login with Valid Token [POST /api/v1/auth/login]
- **所属页面/视口**: `Auth & Security Gate`
- **关联规格/条款**: `FR-AUTH-01, FR-AUTH-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/auth/login, Internal: http://127.0.0.1:18080/api/v1/auth/login`
- **安全业务对象**: `CSP-acceptance-2209039-login-pass`
- **执行步骤**: 1. Send POST /api/v1/auth/login with valid admin token in JSON body {token: '<valid-token>'}; 2. Check response headers and body.
- **断言要求 (严禁纯200)**: HTTP 200 OK; Set-Cookie header contains 'csp_session=...; HttpOnly; SameSite=Lax'; JSON body has authenticated==true, subject=='admin', csrf_token non-empty string. Token input must NOT be logged in plaintext. [EVIDENCE_SECRET_MASKING: 仅为测试取证与日志脱敏，绝不改写UI既有明文显示与表单设计契约]
- **前像备份与清理方案**: Preimage: Record current session cookie; Cleanup: Call POST /api/v1/auth/logout to revoke test session.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Admin Token required; Phone: No`
- **预计预算/耗时**: `150ms / 350 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-008] Admin Session Login Invalid Token Rejection [POST /api/v1/auth/login]
- **所属页面/视口**: `Auth & Security Gate`
- **关联规格/条款**: `FR-AUTH-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/auth/login, Internal: http://127.0.0.1:18080/api/v1/auth/login`
- **安全业务对象**: `CSP-acceptance-2209039-login-fail`
- **执行步骤**: 1. Send POST /api/v1/auth/login with bad token {token: 'invalid-secret-token-attempt'}; 2. Inspect response.
- **断言要求 (严禁纯200)**: HTTP 401 Unauthorized; Set-Cookie header is absent; JSON body contains error code 'unauthorized' and message 'Invalid authentication credentials'. [EVIDENCE_SECRET_MASKING: 仅为测试取证与日志脱敏，绝不改写UI既有明文显示与表单设计契约]
- **前像备份与清理方案**: Preimage: None; Cleanup: None (rejection produces no server session).
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `80ms / 220 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-009] Admin Session Logout & Cookie Revocation [POST /api/v1/auth/logout]
- **所属页面/视口**: `Auth & Security Gate`
- **关联规格/条款**: `FR-AUTH-01, FR-AUTH-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/auth/logout, Internal: http://127.0.0.1:18080/api/v1/auth/logout`
- **安全业务对象**: `CSP-acceptance-2209039-logout`
- **执行步骤**: 1. In an authenticated session with csp_session cookie, send POST /api/v1/auth/logout; 2. Attempt to reuse expired cookie on /api/v1/nodes.
- **断言要求 (严禁纯200)**: HTTP 200 OK; Set-Cookie header clears cookie with 'Max-Age=-1' and empty value; subsequent request using old cookie returns 401 Unauthorized.
- **前像备份与清理方案**: Preimage: Ephemeral test session; Cleanup: Session revoked on server.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Active Session; Phone: No`
- **预计预算/耗时**: `100ms / 280 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-010] CSRF Middleware Interception on Mutating Requests [POST /api/v1/subscriptions]
- **所属页面/视口**: `Auth & Security Gate`
- **关联规格/条款**: `FR-AUTH-03, HR-136`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions, Internal: http://127.0.0.1:18080/api/v1/subscriptions`
- **安全业务对象**: `CSP-acceptance-2209039-csrf-block`
- **执行步骤**: 1. Send POST /api/v1/subscriptions with valid session cookie but without X-CSRF-Token header; 2. Inspect response code.
- **断言要求 (严禁纯200)**: HTTP 403 Forbidden; JSON error code 'csrf_token_invalid' or 'csrf_token_missing'; database has zero mutations.
- **前像备份与清理方案**: Preimage: None; Cleanup: Request rejected before application logic.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Valid Cookie without CSRF header; Phone: No`
- **预计预算/耗时**: `60ms / 200 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-011] Bearer Token Header Authentication [GET /api/v1/nodes]
- **所属页面/视口**: `Auth & Security Gate`
- **关联规格/条款**: `FR-AUTH-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes, Internal: http://127.0.0.1:18080/api/v1/nodes`
- **安全业务对象**: `CSP-acceptance-2209039-bearer-pass`
- **执行步骤**: 1. Send GET /api/v1/nodes with header 'Authorization: Bearer <valid-token>'; 2. Check response status.
- **断言要求 (严禁纯200)**: HTTP 200 OK; JSON body returns paginated node list with items array and total count (90). [EVIDENCE_SECRET_MASKING: 仅为测试取证与日志脱敏，绝不改写UI既有明文显示与表单设计契约]
- **前像备份与清理方案**: Preimage: None; Cleanup: None (read-only query).
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Valid Bearer Token; Phone: No`
- **预计预算/耗时**: `120ms / 15 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-012] Invalid Bearer Token Header Rejection [GET /api/v1/nodes]
- **所属页面/视口**: `Auth & Security Gate`
- **关联规格/条款**: `FR-AUTH-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes, Internal: http://127.0.0.1:18080/api/v1/nodes`
- **安全业务对象**: `CSP-acceptance-2209039-bearer-fail`
- **执行步骤**: 1. Send GET /api/v1/nodes with header 'Authorization: Bearer wrong-token'; 2. Check response status.
- **断言要求 (严禁纯200)**: HTTP 401 Unauthorized; JSON body contains {code: 'unauthorized', message: '...'} envelope. [EVIDENCE_SECRET_MASKING: 仅为测试取证与日志脱敏，绝不改写UI既有明文显示与表单设计契约]
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `50ms / 190 B`
- **当前执行状态**: `NOT_EXECUTED`

### Dashboard (6 项独立操作)

#### [COV-ACT-013] Health Check Trigger & Latency Telemetry [GET /healthz]
- **所属页面/视口**: `Dashboard`
- **关联规格/条款**: `FR-UI-01, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/dashboard, Internal: http://127.0.0.1:17000/healthz`
- **安全业务对象**: `CSP-acceptance-2209039-dash-health`
- **执行步骤**: 1. Navigate to /#/dashboard; 2. Click '刷新状态' (Refresh Health) button; 3. Inspect health badge and latency indicator.
- **断言要求 (严禁纯200)**: HTTP 200 OK from /healthz; health status displays '健康' (Healthy); latency indicator displays '<10ms' or exact ms value; badge-engine and badge-storage rendered.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `60ms / 50 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-014] Aggregate Inventory Stats Loading [GET multi-endpoints]
- **所属页面/视口**: `Dashboard`
- **关联规格/条款**: `FR-UI-01, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/dashboard, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-dash-stats`
- **执行步骤**: 1. Load /#/dashboard; 2. Monitor parallel network calls to /api/v1/subscriptions, /api/v1/nodes, /api/v1/probes/runs, /api/v1/policies/groups.
- **断言要求 (严禁纯200)**: Card metrics display exact database counts: Subscriptions==9, Nodes==90, Policies==29, Probe runs >= 3591; active node subset badge rendered.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `250ms / 2.5 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-015] Quick Action Navigate to Subscriptions [Client Router]
- **所属页面/视口**: `Dashboard`
- **关联规格/条款**: `FR-UI-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/subscriptions, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-dash-nav-subs`
- **执行步骤**: 1. On Dashboard, click '添加订阅源' (Add Subscription) button or Subscriptions stat card; 2. Observe URL and view.
- **断言要求 (严禁纯200)**: Hash router transitions to /#/subscriptions; SubscriptionsView mounts and fires GET /api/v1/subscriptions.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `40ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-016] Quick Action Navigate to Probe Engine [Client Router]
- **所属页面/视口**: `Dashboard`
- **关联规格/条款**: `FR-UI-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/probes, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-dash-nav-probes`
- **执行步骤**: 1. On Dashboard, click '执行节点探测' (Run Probe) button or Probes stat card; 2. Observe URL and view.
- **断言要求 (严禁纯200)**: Hash router transitions to /#/probes; ProbesView mounts with workbench active tab and loads pool status.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `40ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-017] Quick Action Navigate to Exports [Client Router]
- **所属页面/视口**: `Dashboard`
- **关联规格/条款**: `FR-UI-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/publications, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-dash-nav-pub`
- **执行步骤**: 1. On Dashboard, click '导出配置文件' (Export Config) button; 2. Observe URL and view.
- **断言要求 (严禁纯200)**: Hash router transitions to /#/publications; PublicationsView mounts and loads default compiler target preview.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `40ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-018] Quick Action Navigate to Settings [Client Router]
- **所属页面/视口**: `Dashboard`
- **关联规格/条款**: `FR-UI-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/settings, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-dash-nav-set`
- **执行步骤**: 1. On Dashboard, click '系统偏好设置' (System Settings) button; 2. Observe URL and view.
- **断言要求 (严禁纯200)**: Hash router transitions to /#/settings; SettingsView mounts and queries auth mode status.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `40ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

### Subscriptions (10 项独立操作)

#### [COV-ACT-019] Subscription Cards Ledger Listing [GET /api/v1/subscriptions]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-01, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions, Internal: http://127.0.0.1:18080/api/v1/subscriptions`
- **安全业务对象**: `CSP-acceptance-2209039-sub-list`
- **执行步骤**: 1. Navigate to /#/subscriptions; 2. Inspect rendered cards.
- **断言要求 (严禁纯200)**: HTTP 200 OK; exactly 9 subscription cards displayed (including 7li, Mojie, etc.); each card displays name, masked source URL ref, status toggle, and entries button.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `120ms / 4 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-020] Create New Test Subscription with Auto-Naming [POST /api/v1/subscriptions]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-01, FR-INV-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions, Internal: http://127.0.0.1:18080/api/v1/subscriptions`
- **安全业务对象**: `CSP-acceptance-2209039-sub-create`
- **执行步骤**: 1. Click '新建订阅'; 2. Enter URL 'http://127.0.0.1:18080/mock-sub-acceptance'; 3. Verify auto-naming derives '18080' or secondary domain; 4. Set name to 'CSP-acceptance-2209039-sub' and save.
- **断言要求 (严禁纯200)**: HTTP 201 Created; response JSON contains new subscription ID, enabled==true; card appears in UI list without altering the 9 production subscriptions.
- **前像备份与清理方案**: Preimage: Record subscription count (9); Cleanup: Send DELETE /api/v1/subscriptions/{new_id} to restore count to 9.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `180ms / 600 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-021] Toggle Subscription Enabled / Disabled State [PATCH /api/v1/subscriptions/{id}]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions/{id}, Internal: http://127.0.0.1:18080/api/v1/subscriptions/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-sub-toggle`
- **执行步骤**: 1. On test subscription 'CSP-acceptance-2209039-sub', click enable/disable toggle button; 2. Inspect response and UI badge.
- **断言要求 (严禁纯200)**: HTTP 200 OK; subscription.enabled changes to false; status badge updates to '已停用' (Disabled); associated nodes excluded from active node scope.
- **前像备份与清理方案**: Preimage: Read current enabled flag; Cleanup: Re-toggle back to original state.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `110ms / 450 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-022] On-Demand Refresh Subscription [POST /api/v1/subscriptions/{id}/refresh]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions/{id}/refresh, Internal: http://127.0.0.1:18080/api/v1/subscriptions/{id}/refresh`
- **安全业务对象**: `CSP-acceptance-2209039-sub-refresh`
- **执行步骤**: 1. Click '刷新' on test subscription card; 2. Monitor refresh spinner and API call.
- **断言要求 (严禁纯200)**: HTTP 200 OK; last_fetched_at updated to current UTC timestamp; fetch_status records success; node counts updated.
- **前像备份与清理方案**: Preimage: Read last_fetched_at; Cleanup: Test subscription only.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `500ms / 1.2 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-023] Failsafe Node Retention on Upstream Refresh Error [POST /api/v1/subscriptions/{id}/refresh]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-02, HR-002`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions/{id}/refresh, Internal: http://127.0.0.1:18080/api/v1/subscriptions/{id}/refresh`
- **安全业务对象**: `CSP-acceptance-2209039-sub-failsafe`
- **执行步骤**: 1. Configure test subscription with unreachable mock URL (e.g. 192.0.2.1:81 timeout); 2. Trigger refresh.
- **断言要求 (严禁纯200)**: HTTP 4xx/5xx or domain error response; fetch_status records error message; existing raw_nodes remain 100% intact with zero node loss (strict failsafe invariant). [LIMITED_NOT_PASSED: 仅限本地隔离补充，缺少生产安全前提，不得作为生产全量覆盖通过，待建立安全生产前置条件后补验]
- **前像备份与清理方案**: Preimage: Check node count of test subscription; Cleanup: Delete test subscription.
- **风险与依赖**: 风险类别 `ISOLATED_SUPPLEMENT_LIMITED_NOT_PASSED` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `3200ms / 300 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-024] Update Advanced Subscription Config Drawer [PATCH /api/v1/subscriptions/{id}]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-01, FR-INV-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions/{id}, Internal: http://127.0.0.1:18080/api/v1/subscriptions/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-sub-config`
- **执行步骤**: 1. Open configuration drawer for test subscription; 2. Add rename rule (regex replace), filter rule, and custom cron schedule; 3. Save.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON config object contains rename_rules, filter_rules, and cron_schedule; raw_nodes regenerated applying rules.
- **前像备份与清理方案**: Preimage: Read original config; Cleanup: Revert config to empty default.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `160ms / 850 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-025] View Subscription Source Entries Drawer [GET /api/v1/subscriptions/{id}/entries]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-04`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions/{id}/entries, Internal: http://127.0.0.1:18080/api/v1/subscriptions/{id}/entries`
- **安全业务对象**: `CSP-acceptance-2209039-sub-entries`
- **执行步骤**: 1. Click '查看条目' (View Entries) on subscription card; 2. Inspect entries drawer categorization.
- **断言要求 (严禁纯200)**: HTTP 200 OK; entries drawer renders separating entries into notices, proxies, and unknown sections; shows parsed attributes and user_kind_override.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `150ms / 8 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-026] Override Subscription Entry Classification [POST /api/v1/subscriptions/{id}/entries/{entry_id}/override]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-04`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions/{id}/entries/{entry_id}/override, Internal: http://127.0.0.1:18080/api/v1/subscriptions/{id}/entries/{entry_id}/override`
- **安全业务对象**: `CSP-acceptance-2209039-entry-override`
- **执行步骤**: 1. In entries drawer of test subscription, locate an entry; 2. Click '标记为通知' (Override to Notice); 3. Save.
- **断言要求 (严禁纯200)**: HTTP 200 OK; entry effective_kind updates to 'notice'; entry excluded from proxies list; toast notification displays override success.
- **前像备份与清理方案**: Preimage: Read entry user_kind_override; Cleanup: Send override with null to clear.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `140ms / 400 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-027] Copy Subscription Secret Reference [Client Clipboard]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-01, FR-AUTH-04`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/subscriptions, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-sub-copyref`
- **执行步骤**: 1. Click '复制引用' in subscription card popover; 2. Check toast feedback and clipboard content.
- **断言要求 (严禁纯200)**: Toast displays '已复制到剪贴板'; clipboard text contains source_url_secret_ref or name without exposing plaintext token credentials. [EVIDENCE_SECRET_MASKING: 仅为测试取证与日志脱敏，绝不改写UI既有明文显示与表单设计契约]
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `30ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-028] Delete Test Subscription with Confirmation [DELETE /api/v1/subscriptions/{id}]
- **所属页面/视口**: `Subscriptions`
- **关联规格/条款**: `FR-INV-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/subscriptions/{id}, Internal: http://127.0.0.1:18080/api/v1/subscriptions/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-sub-del`
- **执行步骤**: 1. On test subscription 'CSP-acceptance-2209039-sub', click '删除' in popover; 2. Confirm in modal dialog.
- **断言要求 (严禁纯200)**: HTTP 200 OK; subscription deleted from database; card removed from list; associated temporary nodes cleaned up. MUST NEVER be run on the 9 production subscriptions.
- **前像备份与清理方案**: Preimage: Verified subscription ID belongs to test fixture; Cleanup: Deletion is the cleanup step.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `150ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

### Node Ledger (11 项独立操作)

#### [COV-ACT-029] Paginated Node Ledger Browsing [GET /api/v1/nodes]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03, HR-144, HR-149`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes, Internal: http://127.0.0.1:18080/api/v1/nodes`
- **安全业务对象**: `CSP-acceptance-2209039-node-page`
- **执行步骤**: 1. Navigate to /#/nodes; 2. Request page 1 with page_size=50 and scope=enabled_subscriptions; 3. Scroll to trigger virtualized list.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response contains items array of length 50, total==90; each item contains logical_id, name, protocol, server, port, and health metrics.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `180ms / 18 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-030] Filter Nodes by Protocol [GET /api/v1/nodes?protocol=...]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes, Internal: http://127.0.0.1:18080/api/v1/nodes`
- **安全业务对象**: `CSP-acceptance-2209039-node-proto`
- **执行步骤**: 1. In protocol filter dropdown, select 'vless'; 2. Verify returned list.
- **断言要求 (严禁纯200)**: HTTP 200 OK; all items in response have protocol=='vless'; protocol pill count matches array length.
- **前像备份与清理方案**: Preimage: None; Cleanup: Reset filter to 'all'.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `120ms / 6 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-031] Filter Nodes by Health Verdict [GET /api/v1/nodes?health=...]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes, Internal: http://127.0.0.1:18080/api/v1/nodes`
- **安全业务对象**: `CSP-acceptance-2209039-node-health`
- **执行步骤**: 1. Click health status pill (e.g. '健康' / 'healthy'); 2. Verify list.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returned nodes have health status matching the selected category; total items match count in badge.
- **前像备份与清理方案**: Preimage: None; Cleanup: Reset filter to 'all'.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `120ms / 5 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-032] Search Nodes by Name or Hostname [GET /api/v1/nodes?search=...]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes, Internal: http://127.0.0.1:18080/api/v1/nodes`
- **安全业务对象**: `CSP-acceptance-2209039-node-search`
- **执行步骤**: 1. Enter query 'HK' into search bar; 2. Inspect filtered nodes.
- **断言要求 (严禁纯200)**: HTTP 200 OK; every returned node contains 'HK' (case-insensitive) in either display name or server hostname.
- **前像备份与清理方案**: Preimage: None; Cleanup: Clear search input.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `100ms / 4 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-033] Deep-link Direct Node Drawer Mount [GET /#/nodes?node_id=...]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03, FR-UI-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/nodes, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-node-deeplink`
- **执行步骤**: 1. Navigate to /#/nodes?node_id=<sample-logical-id>; 2. Observe UI state on load.
- **断言要求 (严禁纯200)**: Node detail drawer opens automatically on page load without user clicking; deep-linked node details loaded and rendered.
- **前像备份与清理方案**: Preimage: None; Cleanup: Close drawer.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `150ms / 2 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-034] Node Detail Drawer Attributes View [GET /api/v1/nodes/{logical_id}]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03, FR-AUTH-04`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes/{id}, Internal: http://127.0.0.1:18080/api/v1/nodes/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-node-detail`
- **执行步骤**: 1. Click on a node card in ledger; 2. Inspect node drawer fields.
- **断言要求 (严禁纯200)**: HTTP 200 OK; drawer displays server, port, cipher/UUID (sanitized), transport protocol, TLS settings, platform tags, and IP risk badge.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `90ms / 1.5 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-035] Client Syntax Preview Snippet [Client UI]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03, FR-PUB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/nodes, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-node-snippet`
- **执行步骤**: 1. In node drawer, toggle preview target between 'Mihomo (YAML)' and 'SingBox (JSON)'; 2. Inspect code blocks.
- **断言要求 (严禁纯200)**: Mihomo tab renders valid YAML proxy block; SingBox tab renders valid JSON outbound block; credentials sanitized.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `40ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-036] Copy-on-Write Node Connection Parameter Override [PATCH /api/v1/nodes/{logical_id}/connection]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-05`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes/{id}/connection, Internal: http://127.0.0.1:18080/api/v1/nodes/{id}/connection`
- **安全业务对象**: `CSP-acceptance-2209039-node-cow`
- **执行步骤**: 1. For a designated test node, update port from 443 to 8443; 2. Save; 3. Fetch node and check override persistence.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response shows updated connection parameters; node_overrides table updated while raw_nodes remains untouched; reset restores initial port.
- **前像备份与清理方案**: Preimage: Snapshot original connection object; Cleanup: Re-submit PATCH with original port 443.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `150ms / 600 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-037] Node Source History Timeline [GET /api/v1/nodes/{logical_id}/source-history]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-INV-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes/{id}/source-history, Internal: http://127.0.0.1:18080/api/v1/nodes/{id}/source-history`
- **安全业务对象**: `CSP-acceptance-2209039-node-history`
- **执行步骤**: 1. In node drawer, click '来源历史' (Source History) tab; 2. Inspect history timeline.
- **断言要求 (严禁纯200)**: HTTP 200 OK; timeline displays events with timestamp, subscription provenance, and attribute diffs rendered in NodeSourceHistoryPanel.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `110ms / 2 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-038] Historical Node Probe Observations [GET /api/v1/nodes/{logical_id}/observations]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/nodes/{id}/observations, Internal: http://127.0.0.1:18080/api/v1/nodes/{id}/observations`
- **安全业务对象**: `CSP-acceptance-2209039-node-obs`
- **执行步骤**: 1. In node drawer, click '探测历史' (Probe Observations) tab; 2. Inspect observation records.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns array of historical observations with probe_kind, latency_ms, verdict, and timestamp.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `110ms / 3 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-039] Instant Single-Node Probe Trigger [POST /api/v1/probes/runs]
- **所属页面/视口**: `Node Ledger`
- **关联规格/条款**: `FR-PROB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/runs, Internal: http://127.0.0.1:18080/api/v1/probes/runs`
- **安全业务对象**: `CSP-acceptance-2209039-node-probe-1`
- **执行步骤**: 1. In node drawer, click '即时探测' (Probe Now); 2. Check response and run progress indicator.
- **断言要求 (严禁纯200)**: HTTP 201 Created; returns probe_run object with state=='running' targeting single logical_id; completes within 3s returning updated latency.
- **前像备份与清理方案**: Preimage: None; Cleanup: Run completes naturally.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `2500ms / 800 B`
- **当前执行状态**: `NOT_EXECUTED`

### Probe Engine - Workbench (4 项独立操作)

#### [COV-ACT-040] Probe Pool Telemetry Display [GET /api/v1/probes/pool]
- **所属页面/视口**: `Probe Engine - Workbench`
- **关联规格/条款**: `FR-PROB-01, HR-152`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/pool, Internal: http://127.0.0.1:18080/api/v1/probes/pool`
- **安全业务对象**: `CSP-acceptance-2209039-probe-pool`
- **执行步骤**: 1. Navigate to /#/probes; 2. Inspect Pool Telemetry card.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON contains active_runs, queued_runs, pool_capacity, and worker_health; rendered without waterfall looping.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `70ms / 300 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-041] Multi-Dimension Probe Launch [POST /api/v1/probes/runs]
- **所属页面/视口**: `Probe Engine - Workbench`
- **关联规格/条款**: `FR-PROB-01, FR-PROB-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/runs, Internal: http://127.0.0.1:18080/api/v1/probes/runs`
- **安全业务对象**: `CSP-acceptance-2209039-probe-dims`
- **执行步骤**: 1. Select dimension pills: baseline, streaming, ai; 2. Select 1 test node; 3. Click '发起探测' (Trigger Probe).
- **断言要求 (严禁纯200)**: HTTP 201 Created; run record created with specified dimensions; progress updates in workbench; observation records generated for each dimension.
- **前像备份与清理方案**: Preimage: None; Cleanup: Run completes naturally.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `3500ms / 1.5 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-042] Trigger Periodic Pool Enqueue [POST /api/v1/probes/schedule/trigger]
- **所属页面/视口**: `Probe Engine - Workbench`
- **关联规格/条款**: `FR-PROB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/schedule/trigger, Internal: http://127.0.0.1:18080/api/v1/probes/schedule/trigger`
- **安全业务对象**: `CSP-acceptance-2209039-probe-sched-trig`
- **执行步骤**: 1. Click '触发周期调度入队' (Trigger Schedule Enqueue); 2. Check response.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response indicates pool evaluation initiated; pending nodes queued according to schedule batch limits.
- **前像备份与清理方案**: Preimage: None; Cleanup: Queue drains naturally.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `120ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-043] Probe Evidence Sheet Inspection [Client UI / Observation]
- **所属页面/视口**: `Probe Engine - Workbench`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/#/probes, Internal: http://127.0.0.1:17000/`
- **安全业务对象**: `CSP-acceptance-2209039-probe-evidence`
- **执行步骤**: 1. In workbench node list, click '证据检视' (Inspect Evidence); 2. Inspect ProbeEvidenceSheet drawer.
- **断言要求 (严禁纯200)**: Drawer displays RTT baseline latency, streaming platform unlocks (Netflix/YouTube/Disney+), AI service availability, and IP risk score with evidence breakdown.
- **前像备份与清理方案**: Preimage: None; Cleanup: Close drawer.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `100ms / 2 KB`
- **当前执行状态**: `NOT_EXECUTED`

### Probe Engine - Runs & Batches (5 项独立操作)

#### [COV-ACT-044] Probe Runs History Ledger [GET /api/v1/probes/runs]
- **所属页面/视口**: `Probe Engine - Runs & Batches`
- **关联规格/条款**: `FR-PROB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/runs, Internal: http://127.0.0.1:18080/api/v1/probes/runs`
- **安全业务对象**: `CSP-acceptance-2209039-probe-runs-list`
- **执行步骤**: 1. Switch to '运行记录' (Runs) tab; 2. Inspect runs table.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns paginated probe runs list; each item contains run_id, state (completed, failed, cancelled), duration, node_count, and timestamp.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `140ms / 10 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-045] Probe Run Observations Inspection [GET /api/v1/probes/runs/{run_id}/observations]
- **所属页面/视口**: `Probe Engine - Runs & Batches`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/runs/{id}/observations, Internal: http://127.0.0.1:18080/api/v1/probes/runs/{id}/observations`
- **安全业务对象**: `CSP-acceptance-2209039-probe-run-obs`
- **执行步骤**: 1. Expand a completed probe run in runs table; 2. Inspect individual node observation rows.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns detailed observations for the run; includes per-node latency_ms, verdict, error_message, and test parameters.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `120ms / 5 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-046] Cancel Active Probe Run [POST /api/v1/probes/runs/{run_id}/cancel]
- **所属页面/视口**: `Probe Engine - Runs & Batches`
- **关联规格/条款**: `FR-PROB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/runs/{id}/cancel, Internal: http://127.0.0.1:18080/api/v1/probes/runs/{id}/cancel`
- **安全业务对象**: `CSP-acceptance-2209039-probe-run-cancel`
- **执行步骤**: 1. Start a multi-node probe run; 2. Immediately click '取消' (Cancel Run); 3. Verify status.
- **断言要求 (严禁纯200)**: HTTP 200 OK; run status transitions to 'cancelled'; in-flight worker probes terminated; database records cancellation timestamp.
- **前像备份与清理方案**: Preimage: None; Cleanup: Cancellation halts work.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `100ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-047] Probe Batches Ledger [GET /api/v1/probes/batches]
- **所属页面/视口**: `Probe Engine - Runs & Batches`
- **关联规格/条款**: `FR-PROB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/batches, Internal: http://127.0.0.1:18080/api/v1/probes/batches`
- **安全业务对象**: `CSP-acceptance-2209039-probe-batches`
- **执行步骤**: 1. In Runs tab, scroll to Batches section; 2. Inspect batch records.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns batches list with batch_id, total_runs, completed_runs, state, and trigger_source.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `130ms / 8 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-048] Cancel Active Probe Batch [POST /api/v1/probes/batches/{batch_id}/cancel]
- **所属页面/视口**: `Probe Engine - Runs & Batches`
- **关联规格/条款**: `FR-PROB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/batches/{id}/cancel, Internal: http://127.0.0.1:18080/api/v1/probes/batches/{id}/cancel`
- **安全业务对象**: `CSP-acceptance-2209039-batch-cancel`
- **执行步骤**: 1. For an active batch, click Cancel Batch; 2. Check batch status.
- **断言要求 (严禁纯200)**: HTTP 200 OK; batch state marked cancelled; remaining unexecuted runs in batch discarded.
- **前像备份与清理方案**: Preimage: None; Cleanup: Cancellation halts batch.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `110ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

### Probe Engine - Schedule (1 项独立操作)

#### [COV-ACT-049] Probe Scheduler Settings View & Update [GET/PUT /api/v1/probes/schedule]
- **所属页面/视口**: `Probe Engine - Schedule`
- **关联规格/条款**: `FR-PROB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/probes/schedule, Internal: http://127.0.0.1:18080/api/v1/probes/schedule`
- **安全业务对象**: `CSP-acceptance-2209039-probe-schedule`
- **执行步骤**: 1. Switch to '定时调度' (Schedule) tab; 2. Read current schedule; 3. Update batch_size and interval; 4. Save and verify persistence.
- **断言要求 (严禁纯200)**: GET: HTTP 200 OK returning interval, batch_size, concurrency, auto_trigger; PUT: HTTP 200 OK persisting changes to database.
- **前像备份与清理方案**: Preimage: Read original schedule object; Cleanup: Re-submit PUT restoring original values.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `150ms / 600 B`
- **当前执行状态**: `NOT_EXECUTED`

### Policy Tree - Groups (8 项独立操作)

#### [COV-ACT-050] Policy Groups Ledger Listing [GET /api/v1/policies/groups]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-01, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups, Internal: http://127.0.0.1:18080/api/v1/policies/groups`
- **安全业务对象**: `CSP-acceptance-2209039-pol-groups-list`
- **执行步骤**: 1. Navigate to /#/policy; 2. Inspect policy groups list.
- **断言要求 (严禁纯200)**: HTTP 200 OK; exactly 29 policy groups displayed (e.g. Proxy, Auto-HK, Select, etc.); each card displays group type, member counts, and empty fallback status.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `160ms / 12 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-051] Topology Graph DAG Validation Check [POST /api/v1/policies/validate]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-06, HR-146`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/validate, Internal: http://127.0.0.1:18080/api/v1/policies/validate`
- **安全业务对象**: `CSP-acceptance-2209039-pol-validate`
- **执行步骤**: 1. Click '校验拓扑图' (Validate Graph) button; 2. Inspect validation banner.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON contains {valid: true, issues: []}; banner displays green check badge '拓扑结构合法' (Topology Valid).
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `120ms / 400 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-052] Create New Policy Group [POST /api/v1/policies/groups]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-01, FR-POL-08`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups, Internal: http://127.0.0.1:18080/api/v1/policies/groups`
- **安全业务对象**: `CSP-acceptance-2209039-pol-create`
- **执行步骤**: 1. Click '新建策略组'; 2. Enter name 'CSP-acceptance-2209039-grp', type 'select', empty_fallback_pass=true; 3. Save.
- **断言要求 (严禁纯200)**: HTTP 201 Created; response JSON contains new group ID, revision incremented; group card appears in UI list without breaking existing 29 groups.
- **前像备份与清理方案**: Preimage: Record group count (29); Cleanup: Send DELETE /api/v1/policies/groups/{new_id} to restore count to 29.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `180ms / 700 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-053] Edit Policy Group Drawer with Mixed Sources [PUT /api/v1/policies/groups/{id}]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-01, FR-POL-02, FR-POL-05, HR-143`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups/{id}, Internal: http://127.0.0.1:18080/api/v1/policies/groups/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-pol-drawer-edit`
- **执行步骤**: 1. Open editor drawer for test group; 2. Add edges: +group (direct nest), +group_nodes (unpack leaves), -matching (regex filter); 3. Save.
- **断言要求 (严禁纯200)**: HTTP 200 OK; policy_group_sources records all edges in a single transaction; revision increments; drawer displays updated sources.
- **前像备份与清理方案**: Preimage: Read original edges; Cleanup: Re-submit PUT with empty edges array.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `190ms / 900 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-054] Group Empty Fallback PASS Toggle [PATCH /api/v1/policies/groups/{id}]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-08, HR-043`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups/{id}, Internal: http://127.0.0.1:18080/api/v1/policies/groups/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-pol-fallback-pass`
- **执行步骤**: 1. In group editor, toggle '空组兜底 PASS' (empty_fallback_pass) switch; 2. Save.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON contains empty_fallback_pass==true; in compiler output when group has 0 matches, PASS proxy is injected.
- **前像备份与清理方案**: Preimage: Read original empty_fallback_pass; Cleanup: Restore original value.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `130ms / 500 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-055] Direct Group Edge Replacement [PUT /api/v1/policies/groups/{id}/edges]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-01, FR-POL-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups/{id}/edges, Internal: http://127.0.0.1:18080/api/v1/policies/groups/{id}/edges`
- **安全业务对象**: `CSP-acceptance-2209039-pol-edges-put`
- **执行步骤**: 1. Send PUT /api/v1/policies/groups/{id}/edges with an explicit edge list; 2. Verify response.
- **断言要求 (严禁纯200)**: HTTP 200 OK; edge set completely replaced atomically; edge positions normalized.
- **前像备份与清理方案**: Preimage: Read original edges; Cleanup: Reset edges to empty.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `140ms / 600 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-056] DAG Cycle Detection & Prevention [PUT /api/v1/policies/groups/{id}]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-06, HR-001`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups/{id}, Internal: http://127.0.0.1:18080/api/v1/policies/groups/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-pol-cycle-block`
- **执行步骤**: 1. In test group A, add dependency on group B; 2. In group B, add dependency on group A; 3. Attempt to save.
- **断言要求 (严禁纯200)**: HTTP 422 Unprocessable Entity; response code 'cycle_detected' with cycle_path array e.g. ['A', 'B', 'A']; database mutation aborted. [LIMITED_NOT_PASSED: 仅限本地隔离补充，缺少生产安全前提，不得作为生产全量覆盖通过，待建立安全生产前置条件后补验]
- **前像备份与清理方案**: Preimage: None; Cleanup: Transaction rolled back automatically.
- **风险与依赖**: 风险类别 `ISOLATED_SUPPLEMENT_LIMITED_NOT_PASSED` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `110ms / 350 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-057] Delete Policy Group [DELETE /api/v1/policies/groups/{id}]
- **所属页面/视口**: `Policy Tree - Groups`
- **关联规格/条款**: `FR-POL-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups/{id}, Internal: http://127.0.0.1:18080/api/v1/policies/groups/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-pol-group-del`
- **执行步骤**: 1. Delete test group 'CSP-acceptance-2209039-grp'; 2. Confirm in modal.
- **断言要求 (严禁纯200)**: HTTP 200 OK; group removed from policy_groups table; group count returns to 29. MUST NEVER be executed on production groups.
- **前像备份与清理方案**: Preimage: Verified group ID is temporary test fixture; Cleanup: Deletion is cleanup.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `140ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

### Policy Tree - Rules (3 项独立操作)

#### [COV-ACT-058] Routing Rules Ledger Listing [GET /api/v1/policies/rules]
- **所属页面/视口**: `Policy Tree - Rules`
- **关联规格/条款**: `FR-POL-04, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/rules, Internal: http://127.0.0.1:18080/api/v1/policies/rules`
- **安全业务对象**: `CSP-acceptance-2209039-pol-rules-list`
- **执行步骤**: 1. Switch to '分流规则' (Rules) tab in Policy view; 2. Inspect rules table.
- **断言要求 (严禁纯200)**: HTTP 200 OK; exactly 162 rules rendered, sorted by position asc; shows expression, action, and target group.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `170ms / 15 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-059] Add New Routing Rule [POST /api/v1/policies/rules]
- **所属页面/视口**: `Policy Tree - Rules`
- **关联规格/条款**: `FR-POL-04`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/rules, Internal: http://127.0.0.1:18080/api/v1/policies/rules`
- **安全业务对象**: `CSP-acceptance-2209039-pol-rule-add`
- **执行步骤**: 1. Click '新建分流规则'; 2. Enter expression 'DOMAIN-SUFFIX,acceptance-test.local', target group 'Proxy', position 999; 3. Save.
- **断言要求 (严禁纯200)**: HTTP 201 Created; rule inserted at position 999; response returns new rule ID; rules table increments total count to 163.
- **前像备份与清理方案**: Preimage: Record rule count (162); Cleanup: Send DELETE /api/v1/policies/rules/{new_id} to restore count to 162.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `150ms / 500 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-060] Delete Routing Rule [DELETE /api/v1/policies/rules/{id}]
- **所属页面/视口**: `Policy Tree - Rules`
- **关联规格/条款**: `FR-POL-04`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/rules/{id}, Internal: http://127.0.0.1:18080/api/v1/policies/rules/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-pol-rule-del`
- **执行步骤**: 1. Delete temporary routing rule 'DOMAIN-SUFFIX,acceptance-test.local'; 2. Confirm in modal.
- **断言要求 (严禁纯200)**: HTTP 200 OK; rule removed; total count returns to 162; positions normalized.
- **前像备份与清理方案**: Preimage: Verified rule ID is temporary test fixture; Cleanup: Deletion is cleanup.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `130ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

### Policy Tree - Admission (2 项独立操作)

#### [COV-ACT-061] Admission Filter Rules Listing [GET /api/v1/admission/rules]
- **所属页面/视口**: `Policy Tree - Admission`
- **关联规格/条款**: `FR-POL-07`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/admission/rules, Internal: http://127.0.0.1:18080/api/v1/admission/rules`
- **安全业务对象**: `CSP-acceptance-2209039-pol-admiss-list`
- **执行步骤**: 1. Switch to '准入规则' (Admission) tab; 2. Inspect admission rules list.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns admission rules array (currently 0 baseline rules).
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `80ms / 300 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-062] Create & Delete Admission Rule [POST/DELETE /api/v1/admission/rules]
- **所属页面/视口**: `Policy Tree - Admission`
- **关联规格/条款**: `FR-POL-07`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/admission/rules, Internal: http://127.0.0.1:18080/api/v1/admission/rules`
- **安全业务对象**: `CSP-acceptance-2209039-pol-admiss-crud`
- **执行步骤**: 1. Add admission rule with expression 'GEOIP,CN', action 'DIRECT'; 2. Verify creation; 3. Delete the rule.
- **断言要求 (严禁纯200)**: POST: HTTP 201 Created returning rule ID; DELETE: HTTP 200 OK removing the rule; total returns to 0.
- **前像备份与清理方案**: Preimage: Count is 0; Cleanup: DELETE rule restores count to 0.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `200ms / 600 B`
- **当前执行状态**: `NOT_EXECUTED`

### Global Node Filter (2 项独立操作)

#### [COV-ACT-063] Get Global Node Filter Spec [GET /api/v1/policies/global-node-filter]
- **所属页面/视口**: `Global Node Filter`
- **关联规格/条款**: `FR-POL-05`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/global-node-filter, Internal: http://127.0.0.1:18080/api/v1/policies/global-node-filter`
- **安全业务对象**: `CSP-acceptance-2209039-filter-get`
- **执行步骤**: 1. Query global filter endpoint /api/v1/policies/global-node-filter.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns spec object containing conditions array; conditions define global node admission criteria.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `80ms / 400 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-064] Update Global Node Filter Spec [PUT /api/v1/policies/global-node-filter]
- **所属页面/视口**: `Global Node Filter`
- **关联规格/条款**: `FR-POL-05`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/global-node-filter, Internal: http://127.0.0.1:18080/api/v1/policies/global-node-filter`
- **安全业务对象**: `CSP-acceptance-2209039-filter-put`
- **执行步骤**: 1. Submit updated filter condition (e.g. protocol equals vless); 2. Verify compilation takes filter into account.
- **断言要求 (严禁纯200)**: HTTP 200 OK; spec updated; all subsequent policy group resolutions apply this filter globally.
- **前像备份与清理方案**: Preimage: Snapshot original filter spec; Cleanup: Re-submit PUT with original filter spec.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `150ms / 500 B`
- **当前执行状态**: `NOT_EXECUTED`

### Exports & Publications (10 项独立操作)

#### [COV-ACT-065] Mihomo Target Compiler Preview [POST /api/v1/publications/preview]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01, FR-PUB-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications/preview, Internal: http://127.0.0.1:18080/api/v1/publications/preview`
- **安全业务对象**: `CSP-acceptance-2209039-pub-prev-mihomo`
- **执行步骤**: 1. Navigate to /#/publications; 2. Select target 'Mihomo (Clash.Meta)', mode 'strict'; 3. Click '生成预览'.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response Content-Type 'text/yaml'; body contains valid YAML with 'proxies:', 'proxy-groups:', and 'rules:'; passes YAML parser without error.
- **前像备份与清理方案**: Preimage: None; Cleanup: None (ephemeral preview).
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `350ms / 35 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-066] SingBox Target Compiler Preview [POST /api/v1/publications/preview]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01, FR-PUB-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications/preview, Internal: http://127.0.0.1:18080/api/v1/publications/preview`
- **安全业务对象**: `CSP-acceptance-2209039-pub-prev-singbox`
- **执行步骤**: 1. Switch target to 'SingBox'; 2. Generate preview.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response Content-Type 'application/json'; body contains valid SingBox configuration with 'outbounds:' array and 'route.rules:' array.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `320ms / 30 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-067] Surge Target Compiler Preview [POST /api/v1/publications/preview]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01, FR-PUB-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications/preview, Internal: http://127.0.0.1:18080/api/v1/publications/preview`
- **安全业务对象**: `CSP-acceptance-2209039-pub-prev-surge`
- **执行步骤**: 1. Switch target to 'Surge'; 2. Generate preview.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response Content-Type 'text/plain'; body contains Surge sections: [General], [Proxy], [Proxy Group], [Rule].
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `300ms / 25 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-068] Quantumult-X Target Compiler Preview [POST /api/v1/publications/preview]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01, FR-PUB-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications/preview, Internal: http://127.0.0.1:18080/api/v1/publications/preview`
- **安全业务对象**: `CSP-acceptance-2209039-pub-prev-qx`
- **执行步骤**: 1. Switch target to 'Quantumult-X'; 2. Generate preview.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response Content-Type 'text/plain'; body contains QX sections: [server_remote], [server_local], [policy], [filter_remote].
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `290ms / 20 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-069] Publication Preflight Diagnostics [GET/POST /api/v1/publications/preflight]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01, FR-PUB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications/preflight, Internal: http://127.0.0.1:18080/api/v1/publications/preflight`
- **安全业务对象**: `CSP-acceptance-2209039-pub-preflight`
- **执行步骤**: 1. In Exports view, trigger Preflight Diagnostics; 2. Inspect diagnostics report.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON contains empty_groups list, unsupported_protocol_nodes list, and compiler warning indicators; zero fatal errors on baseline.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `220ms / 2.5 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-070] Create Production Publication [POST /api/v1/publications]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01, FR-AUTH-04`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications, Internal: http://127.0.0.1:18080/api/v1/publications`
- **安全业务对象**: `CSP-acceptance-2209039-pub-publish`
- **执行步骤**: 1. Click '发布新版本' (Publish); 2. Select target 'mihomo', lifespan '30d'; 3. Submit.
- **断言要求 (严禁纯200)**: HTTP 201 Created; response JSON contains publication id, target=='mihomo', state=='active', content_digest, and export_token. Token credentials must be handled securely (capturesecretavoid).
- **前像备份与清理方案**: Preimage: Record publication ID; Cleanup: Call POST /api/v1/publications/{id}/revoke to revoke publication.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `280ms / 1.2 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-071] Get Publication Status by ID [GET /api/v1/publications/{id}]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications/{id}, Internal: http://127.0.0.1:18080/api/v1/publications/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-pub-get`
- **执行步骤**: 1. Query created publication status via GET /api/v1/publications/{id}.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON contains id, target, state=='active', snapshot_digest, created_at, expires_at.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `90ms / 600 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-072] Revoke Publication [POST /api/v1/publications/{id}/revoke or DELETE]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications/{id}/revoke, Internal: http://127.0.0.1:18080/api/v1/publications/{id}/revoke`
- **安全业务对象**: `CSP-acceptance-2209039-pub-revoke`
- **执行步骤**: 1. Send POST /api/v1/publications/{id}/revoke for test publication; 2. Attempt to download via public endpoint.
- **断言要求 (严禁纯200)**: HTTP 200 OK; publication state marked 'revoked'; subsequent download request returns 403 Forbidden with code 'publication_revoked'.
- **前像备份与清理方案**: Preimage: Active publication; Cleanup: Revocation is cleanup.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `120ms / 350 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-073] Public Client Endpoint Download [GET /publish/v1/{id} & /p/{id}]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01, FR-PUB-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/publish/v1/{id}, Internal: http://127.0.0.1:18080/publish/v1/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-pub-download`
- **执行步骤**: 1. Send GET request to /publish/v1/{publication_id} (or /p/{publication_id}) with required export token; 2. Inspect headers.
- **断言要求 (严禁纯200)**: HTTP 200 OK; Content-Type is text/yaml; ETag and X-Content-Digest match digest in publication record; Content-Disposition specifies attachment filename.
- **前像备份与清理方案**: Preimage: Active publication; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Export Token if export_auth_enabled; Phone: No`
- **预计预算/耗时**: `250ms / 35 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-074] Architectural Invariant: No Export Listing Endpoint [GET /api/v1/publications]
- **所属页面/视口**: `Exports & Publications`
- **关联规格/条款**: `FR-PUB-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/publications, Internal: http://127.0.0.1:18080/api/v1/publications`
- **安全业务对象**: `CSP-acceptance-2209039-pub-no-list`
- **执行步骤**: 1. Send GET /api/v1/publications to backend router; 2. Check response status.
- **断言要求 (严禁纯200)**: HTTP 404 Not Found (or 405 Method Not Allowed); router intentionally does NOT expose an export listing endpoint; client UI manages active publication via local storage/ID lookup.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer; Phone: No`
- **预计预算/耗时**: `50ms / 150 B`
- **当前执行状态**: `NOT_EXECUTED`

### Settings & Dual Switch (6 项独立操作)

#### [COV-ACT-075] View Security Settings & Auth Dual Switches [GET /api/v1/settings/auth]
- **所属页面/视口**: `Settings & Dual Switch`
- **关联规格/条款**: `FR-AUTH-01, FR-AUTH-02, FR-UI-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/settings/auth, Internal: http://127.0.0.1:18080/api/v1/settings/auth`
- **安全业务对象**: `CSP-acceptance-2209039-set-view`
- **执行步骤**: 1. Navigate to /#/settings; 2. Inspect Security Control cards.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON contains admin_auth_enabled==true, export_auth_enabled==true, admin_mode=='protected', export_mode=='protected', token_configured==true.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `90ms / 400 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-076] Test Admin Connection Button [GET /api/v1/auth/status]
- **所属页面/视口**: `Settings & Dual Switch`
- **关联规格/条款**: `FR-AUTH-01, FR-UI-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/auth/status, Internal: http://127.0.0.1:17000/api/v1/auth/status`
- **安全业务对象**: `CSP-acceptance-2209039-set-test-conn`
- **执行步骤**: 1. On Settings view, click '测试当前连接' (Test Connection); 2. Check UI feedback.
- **断言要求 (严禁纯200)**: HTTP 200 OK; toast notification displays '控制台连接正常 (当前模式: 保护模式)' (Connection Normal); connection status indicator turns green.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `60ms / 200 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-077] Toggle Export Auth Switch with Public Warning [PUT /api/v1/settings/auth]
- **所属页面/视口**: `Settings & Dual Switch`
- **关联规格/条款**: `FR-AUTH-02`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/settings/auth, Internal: http://127.0.0.1:18080/api/v1/settings/auth`
- **安全业务对象**: `CSP-acceptance-2209039-set-export-auth`
- **执行步骤**: 1. In Settings, toggle Export Authentication switch to disabled; 2. Modal warns about public exposure; 3. Confirm; 4. Re-enable.
- **断言要求 (严禁纯200)**: HTTP 200 OK; export_auth_enabled toggles to false then back to true; when false, anonymous downloads on /p/{id} succeed without token.
- **前像备份与清理方案**: Preimage: Read original export_auth_enabled; Cleanup: Immediately re-enable export_auth_enabled=true.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `200ms / 600 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-078] Toggle Admin Auth Switch [PUT /api/v1/settings/auth] - BLOCKED ON PROD
- **所属页面/视口**: `Settings & Dual Switch`
- **关联规格/条款**: `FR-AUTH-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/settings/auth, Internal: http://127.0.0.1:18080/api/v1/settings/auth`
- **安全业务对象**: `CSP-acceptance-2209039-set-admin-auth-block`
- **执行步骤**: 1. Attempt to disable admin_auth_enabled on production.
- **断言要求 (严禁纯200)**: CRITICAL SECURITY RISK: Disabling admin auth opens the control plane without authentication. BLOCKED from live execution; needs authorized safe counterparty/session or explicit live precondition. [LIVE_PRECONDITION_BLOCKED: Token轮换/清空/关闭管理认证不能满足生产所有功能，必须等待合法安全步骤与已授权Session，不是豁免；未获明确授权绝不尝试关闭或清空生产认证]
- **前像备份与清理方案**: Preimage: admin_auth_enabled=true; Cleanup: Must NEVER remain disabled on production.
- **风险与依赖**: 风险类别 `LIVE_PRECONDITION_BLOCKED` | 认证与硬件依赖 `Auth: Explicit Admin Precondition required; Phone: No`
- **预计预算/耗时**: `0ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-079] Rotate Admin Token [PUT /api/v1/settings/auth] - BLOCKED ON PROD
- **所属页面/视口**: `Settings & Dual Switch`
- **关联规格/条款**: `FR-AUTH-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/settings/auth, Internal: http://127.0.0.1:18080/api/v1/settings/auth`
- **安全业务对象**: `CSP-acceptance-2209039-set-rotate-token-block`
- **执行步骤**: 1. Attempt to rotate admin token via PUT /api/v1/settings/auth with new secret.
- **断言要求 (严禁纯200)**: CRITICAL OPERATIONAL RISK: Token rotation invalidates all existing operator sessions and bearer tokens. BLOCKED from live execution; needs authorized safe counterparty/session or explicit live precondition. [LIVE_PRECONDITION_BLOCKED: Token轮换/清空/关闭管理认证不能满足生产所有功能，必须等待合法安全步骤与已授权Session，不是豁免；未获明确授权绝不尝试关闭或清空生产认证] [EVIDENCE_SECRET_MASKING: 仅为测试取证与日志脱敏，绝不改写UI既有明文显示与表单设计契约]
- **前像备份与清理方案**: Preimage: Current token hash; Cleanup: Cannot safely self-restore if session drops.
- **风险与依赖**: 风险类别 `LIVE_PRECONDITION_BLOCKED` | 认证与硬件依赖 `Auth: Explicit Admin Precondition required; Phone: No`
- **预计预算/耗时**: `0ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-080] Clear Admin Token [PUT /api/v1/settings/auth] - BLOCKED ON PROD
- **所属页面/视口**: `Settings & Dual Switch`
- **关联规格/条款**: `FR-AUTH-01`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/settings/auth, Internal: http://127.0.0.1:18080/api/v1/settings/auth`
- **安全业务对象**: `CSP-acceptance-2209039-set-clear-token-block`
- **执行步骤**: 1. Attempt to clear admin token via clear_token=true.
- **断言要求 (严禁纯200)**: CRITICAL RISK: Removes server authentication secret. BLOCKED from live execution; needs authorized safe counterparty/session or explicit live precondition. [LIVE_PRECONDITION_BLOCKED: Token轮换/清空/关闭管理认证不能满足生产所有功能，必须等待合法安全步骤与已授权Session，不是豁免；未获明确授权绝不尝试关闭或清空生产认证] [EVIDENCE_SECRET_MASKING: 仅为测试取证与日志脱敏，绝不改写UI既有明文显示与表单设计契约]
- **前像备份与清理方案**: Preimage: Current token; Cleanup: Must not clear production token.
- **风险与依赖**: 风险类别 `LIVE_PRECONDITION_BLOCKED` | 认证与硬件依赖 `Auth: Explicit Admin Precondition required; Phone: No`
- **预计预算/耗时**: `0ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

### Revisions & Snapshots (5 项独立操作)

#### [COV-ACT-081] List Configuration Revisions [GET /api/v1/revisions]
- **所属页面/视口**: `Revisions & Snapshots`
- **关联规格/条款**: `FR-POL-06, HR-148`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/revisions, Internal: http://127.0.0.1:18080/api/v1/revisions`
- **安全业务对象**: `CSP-acceptance-2209039-rev-list`
- **执行步骤**: 1. Send GET /api/v1/revisions; 2. Inspect revision history list.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns array of revisions with revision_id, state (draft, active, archived), created_at, activated_at. (Backend capability: not exposed as separate top-level UI tab).
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `110ms / 3 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-082] Create New Configuration Revision Draft [POST /api/v1/revisions]
- **所属页面/视口**: `Revisions & Snapshots`
- **关联规格/条款**: `FR-POL-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/revisions, Internal: http://127.0.0.1:18080/api/v1/revisions`
- **安全业务对象**: `CSP-acceptance-2209039-rev-create`
- **执行步骤**: 1. Send POST /api/v1/revisions with description 'Acceptance test revision'; 2. Verify draft created.
- **断言要求 (严禁纯200)**: HTTP 201 Created; returns new revision record with state=='draft' and unique revision ID.
- **前像备份与清理方案**: Preimage: Record current revisions; Cleanup: Draft does not affect active serving.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `140ms / 500 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-083] Get Active Configuration Revision [GET /api/v1/revisions/active]
- **所属页面/视口**: `Revisions & Snapshots`
- **关联规格/条款**: `FR-POL-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/revisions/active, Internal: http://127.0.0.1:18080/api/v1/revisions/active`
- **安全业务对象**: `CSP-acceptance-2209039-rev-active`
- **执行步骤**: 1. Query GET /api/v1/revisions/active; 2. Inspect active revision attributes.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns current active revision serving production traffic with full topology digest.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `90ms / 800 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-084] Review & Activate Configuration Revision [POST /api/v1/revisions/{id}/review & activate]
- **所属页面/视口**: `Revisions & Snapshots`
- **关联规格/条款**: `FR-POL-06`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/revisions/{id}/activate, Internal: http://127.0.0.1:18080/api/v1/revisions/{id}/activate`
- **安全业务对象**: `CSP-acceptance-2209039-rev-activate`
- **执行步骤**: 1. Post /review on draft revision; 2. Post /activate; 3. Verify state transition to active.
- **断言要求 (严禁纯200)**: HTTP 200 OK; review transitions state to 'under_review'; activate transitions state to 'active'; previous active revision transitions to 'archived'. [LIMITED_NOT_PASSED: 仅限本地隔离补充，缺少生产安全前提，不得作为生产全量覆盖通过，待建立安全生产前置条件后补验]
- **前像备份与清理方案**: Preimage: Snapshot current active revision ID; Cleanup: Re-activate original revision ID.
- **风险与依赖**: 风险类别 `ISOLATED_SUPPLEMENT_LIMITED_NOT_PASSED` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `250ms / 1.2 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-085] Configuration Snapshot Integrity & Hot Backup Verification [SQLite PRAGMA]
- **所属页面/视口**: `Revisions & Snapshots`
- **关联规格/条款**: `HR-003, HR-136`
- **测试目标位置**: `Internal: /var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db`
- **安全业务对象**: `CSP-acceptance-2209039-snap-integrity`
- **执行步骤**: 1. Execute SQLite hot backup command: sqlite3 /data/csp-v1.db ".backup '/tmp/csp-backup-test.db'"; 2. Run PRAGMA integrity_check and foreign_key_check.
- **断言要求 (严禁纯200)**: Hot backup completes with exit code 0; PRAGMA integrity_check returns 'ok'; foreign_key_check returns 0 violations.
- **前像备份与清理方案**: Preimage: None; Cleanup: Remove /tmp/csp-backup-test.db.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Host root/service permissions; Phone: No`
- **预计预算/耗时**: `120ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

### IP Risk Intelligence (5 项独立操作)

#### [COV-ACT-086] List IP Risk Policies [GET /api/v1/ip-risk/policies]
- **所属页面/视口**: `IP Risk Intelligence`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/ip-risk/policies, Internal: http://127.0.0.1:18080/api/v1/ip-risk/policies`
- **安全业务对象**: `CSP-acceptance-2209039-risk-pol-list`
- **执行步骤**: 1. Query GET /api/v1/ip-risk/policies; 2. Inspect risk policies list.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns array of risk policies with threshold scores, fraud score caps, and action rules. (Backend capability: UI reflects via node risk badge).
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer required; Phone: No`
- **预计预算/耗时**: `90ms / 1.5 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-087] Create & Activate IP Risk Policy [POST /api/v1/ip-risk/policies]
- **所属页面/视口**: `IP Risk Intelligence`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/ip-risk/policies, Internal: http://127.0.0.1:18080/api/v1/ip-risk/policies`
- **安全业务对象**: `CSP-acceptance-2209039-risk-pol-create`
- **执行步骤**: 1. Create risk policy 'CSP-acceptance-2209039-risk' with max_fraud_score=75; 2. Activate via /activate endpoint.
- **断言要求 (严禁纯200)**: HTTP 201 Created on create; HTTP 200 OK on activate; policy becomes active reference for node risk scoring. [LIMITED_NOT_PASSED: 仅限本地隔离补充，缺少生产安全前提，不得作为生产全量覆盖通过，待建立安全生产前置条件后补验]
- **前像备份与清理方案**: Preimage: Record active risk policy ID; Cleanup: Deactivate or restore original active policy.
- **风险与依赖**: 风险类别 `ISOLATED_SUPPLEMENT_LIMITED_NOT_PASSED` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `180ms / 800 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-088] Manage Group Risk Policy Bindings [GET/PUT/DELETE /api/v1/ip-risk/bindings/{group_id}]
- **所属页面/视口**: `IP Risk Intelligence`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/ip-risk/bindings/{id}, Internal: http://127.0.0.1:18080/api/v1/ip-risk/bindings/{id}`
- **安全业务对象**: `CSP-acceptance-2209039-risk-binding`
- **执行步骤**: 1. Bind test risk policy to a test policy group; 2. Verify binding query; 3. Delete binding.
- **断言要求 (严禁纯200)**: PUT: HTTP 200 OK creating binding; GET: HTTP 200 OK returning bound policy_id; DELETE: HTTP 200 OK removing binding.
- **前像备份与清理方案**: Preimage: Read original binding if any; Cleanup: DELETE binding.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `220ms / 700 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-089] Group Members Risk Evaluation [POST /api/v1/ip-risk/groups/{group_id}/evaluate]
- **所属页面/视口**: `IP Risk Intelligence`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/ip-risk/groups/{id}/evaluate, Internal: http://127.0.0.1:18080/api/v1/ip-risk/groups/{id}/evaluate`
- **安全业务对象**: `CSP-acceptance-2209039-risk-eval`
- **执行步骤**: 1. Send POST /api/v1/ip-risk/groups/{group_id}/evaluate for a group; 2. Inspect filtered nodes.
- **断言要求 (严禁纯200)**: HTTP 200 OK; returns evaluation summary with eligible_nodes, disqualified_nodes, and individual fraud score justifications.
- **前像备份与清理方案**: Preimage: None; Cleanup: None (evaluation is computation without mutation).
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `140ms / 2 KB`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-090] Group Scoped Risk Policy Binding Endpoint [GET/PUT/DELETE /api/v1/policies/groups/{id}/risk-policy]
- **所属页面/视口**: `IP Risk Intelligence`
- **关联规格/条款**: `FR-PROB-03`
- **测试目标位置**: `Public URL: https://sub.einck.top/api/v1/policies/groups/{id}/risk-policy, Internal: http://127.0.0.1:18080/api/v1/policies/groups/{id}/risk-policy`
- **安全业务对象**: `CSP-acceptance-2209039-risk-group-alias`
- **执行步骤**: 1. Access group-scoped endpoint /api/v1/policies/groups/{id}/risk-policy; 2. Verify alias routing to ip-risk service.
- **断言要求 (严禁纯200)**: HTTP 200 OK; successfully maps to risk binding handler; returns consistent binding state.
- **前像备份与清理方案**: Preimage: None; Cleanup: Delete test binding.
- **风险与依赖**: 风险类别 `SAFE_MUTATION` | 认证与硬件依赖 `Auth: Session/Bearer + CSRF; Phone: No`
- **预计预算/耗时**: `110ms / 500 B`
- **当前执行状态**: `NOT_EXECUTED`

### System Interceptors (3 项独立操作)

#### [COV-ACT-091] Legacy Python /yaml Hard 410 Gone Interception [GET /yaml & /yaml/*]
- **所属页面/视口**: `System Interceptors`
- **关联规格/条款**: `HR-148, CSP-V1-ARCH`
- **测试目标位置**: `Public URL: https://sub.einck.top/yaml, Internal: http://127.0.0.1:18080/yaml`
- **安全业务对象**: `CSP-acceptance-2209039-sys-410-yaml`
- **执行步骤**: 1. Send GET /yaml and GET /yaml/any-sub-path; 2. Inspect status code and error payload.
- **断言要求 (严禁纯200)**: HTTP 410 Gone; Location header deleted; JSON body contains {code: 'legacy_endpoint_removed', message: 'This endpoint has been permanently removed in CSP 1.0. Please use the versioned API at /api/v1/.'}.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `50ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-092] Legacy Python /script Hard 410 Gone Interception [GET /script & /script/*]
- **所属页面/视口**: `System Interceptors`
- **关联规格/条款**: `HR-148, CSP-V1-ARCH`
- **测试目标位置**: `Public URL: https://sub.einck.top/script, Internal: http://127.0.0.1:18080/script`
- **安全业务对象**: `CSP-acceptance-2209039-sys-410-script`
- **执行步骤**: 1. Send GET /script and GET /script/eval; 2. Inspect status code and error payload.
- **断言要求 (严禁纯200)**: HTTP 410 Gone; JSON body contains {code: 'legacy_endpoint_removed', message: '...'}.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `50ms / 250 B`
- **当前执行状态**: `NOT_EXECUTED`

#### [COV-ACT-093] System Readiness Healthcheck [GET /readyz]
- **所属页面/视口**: `System Interceptors`
- **关联规格/条款**: `CSP-V1-ARCH`
- **测试目标位置**: `Public URL: https://sub.einck.top/readyz, Internal: http://127.0.0.1:18080/readyz`
- **安全业务对象**: `CSP-acceptance-2209039-sys-readyz`
- **执行步骤**: 1. Send GET /readyz; 2. Check health status of database and token provider.
- **断言要求 (严禁纯200)**: HTTP 200 OK; response JSON contains {status: 'ok', checks: {database: 'up', token_provider: 'up'}}.
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `READONLY` | 认证与硬件依赖 `Auth: None; Phone: No`
- **预计预算/耗时**: `40ms / 150 B`
- **当前执行状态**: `NOT_EXECUTED`

### Mobile Physical Boundary (1 项独立操作)

#### [COV-ACT-094] Edge Device Channel Connection Check [Port 8022] (device alive / connect refused / unverified model & ARM)
- **所属页面/视口**: `Mobile Physical Boundary`
- **关联规格/条款**: `HR-150, SPEC-006 (Router & Device Catalog)`
- **测试目标位置**: `LAN Edge Device: port 8022`
- **安全业务对象**: `CSP-acceptance-2209039-mob-channel`
- **执行步骤**: 1. Probe TCP socket connection to LAN edge device on port 8022; 2. Inspect socket state and device response.
- **断言要求 (严禁纯200)**: Socket connection returns Connection Refused (通道connect refused/devicealive尚未核真实model/ARM); 设备虽然在线但通道连接被拒，硬件型号与ARM环境未核验，不能与Termux root等价。严禁通过桌面Viewport缩放假充真机Passed。
- **前像备份与清理方案**: Preimage: None; Cleanup: None.
- **风险与依赖**: 风险类别 `LIVE_PRECONDITION_BLOCKED` | 认证与硬件依赖 `Auth: SSH key/channel; Phone: YES (通道connect refused/devicealive尚未核真实model/ARM)`
- **预计预算/耗时**: `1000ms / 0 B`
- **当前执行状态**: `NOT_EXECUTED`

---

## 3. 统计汇总与闭环确认 (Summary & Invariant Verification)

| 统计分类指标 | 项数 | 说明 |
|---|---|---|
| **总登记独立操作数** | **94** | 完整覆盖 7 大页面、底层全部注册路由与后端未暴露能力 |
| **只读检验操作 (ReadOnly)** | **56** | 零状态变更，包含预览、查询、断言、过滤、搜索与前端切换 |
| **安全受控变更 (Safe Mutation)** | **30** | 配备 Preimage 与逆向 Cleanup 方案，单事务回滚保障 |
| **本地隔离补充 (Limited Not Passed)** | **4** | 缺少生产安全前提，仅限隔离补充，不得作为生产全量通过 |
| **受权前提阻断 (Auth Precondition)** | **3** | Token 轮换/清空/关闭认证需安全会话与明确授权，非豁免 |
| **移动设备通道受阻 (Phone Channel)** | **1** | 通道 connect refused / device alive 尚未核真实 model 与 ARM |
| **本次实际执行数** | **0** | 本任务全量状态保持 `NOT_EXECUTED`，阶段计划非生产生效 |

### 3.1 规格与路由覆盖自洽性复核 (Self-Consistency Checklist)
- [x] `internal/transport/http/router.go` 全部挂载路由均已显式映射为独立操作（包含 `/healthz`, `/readyz`, `/yaml`, `/script`, `/publish/v1/`, `/p/`, `/api/v1/...`）；
- [x] 明确指出公网 `/publish/v1/{id}` 无批量 GET 列表路由 (`COV-ACT-074`)，不凭空捏造不存在的导出列表功能；
- [x] 涵盖后端控制面 Revisions (`COV-ACT-081` ~ `084`)、IP Risk Intelligence (`COV-ACT-086` ~ `090`) 及配置快照恢复能力 (`COV-ACT-085`)，并准确标明 UI 暴露差异；
- [x] 涵盖前端 Vue 全部真实组件：7 套主题切换、i18n 多语言切换、侧边栏折叠、全局 Toast、ConfirmModal 焦点陷阱、抽屉卡片、虚拟滚动与各种过滤；
- [x] 严格执行 `capturesecretavoid` 取证脱敏，同时明确不更改 UI 既有管理员明文显示契约；
- [x] 关联工件 `/tmp/csp-resume-20261011/full-production-coverage.json` 权限严格设为 `0600`，SHA-256 校验自洽。

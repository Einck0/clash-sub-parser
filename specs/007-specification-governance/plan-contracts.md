# CSP Technical Plan Contracts & Authoritative Constraint Annex (specs/007/plan-contracts.md)

- **Feature Directory**: `specs/007-specification-governance/` (canonical: `specs/007/plan-contracts.md`)
- **Capability Domain**: `governance`
- **Status**: AUTHORITATIVE NATIVE PLAN CONTRACTS
- **Baseline**: HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **Scope**: Authoritative retention of 261 still-applicable system invariants and architectural constraints plus 3 active architectural decisions across all 6 business capability domains (`inventory`, `policy`, `probe`, `publication`, `auth`, `ui`).

## 1. Technical Plan Index & Authority Link <a id="plan-contracts-index"></a>

This authoritative annex provides individual retention and cryptographic content integrity for all active design invariants and technical constraints derived from the 26 historical OpenSpec change proposals and design documents.
- Authoritative Native Plan Index: Referenced directly by [`specs/007-specification-governance/plan.md`](plan.md) with link authority.
- Total Active Plan Contracts: 261 Still-Applicable Constraints (DOC-0001 ~ DOC-0354 subset) + 3 Active Architectural Decisions (DEC-0807 ~ DEC-0809).
- Historical Inactive Archives: 58 Superseded Constraints + 35 Undetermined Constraints + 452 Historical Tasks retained as archived reference evidence in `docs/migration/historical-contract-register.md`.

## 2. Authoritative Plan Contracts (DOC-0001 ~ DOC-0354)

### Plan Contract DOC-0001 <a id="plan-doc-0001"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/proposal.md:1`
- **Source Digest**: `070ab4619584a278fd6d887b4eaef971f16cfd4a675a91a3aaed068c600bc9af`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

在针对 CSP 1.0（分支 `dev` 基线 `fe4654b`）的独立前后端代码与运行时走查中，发现并证实了多项影响系统可用性、并发安全性、生命周期一致性与环境安全契约的真实功能缺陷：
1. **配置修订引导缺失**：全新数据库启动时无任何配置修订版本，导致添加规则时报 422 `missing_revision_id`、发布中心报 409 `no_active_revision` 且前端无从引导恢复；
2. **探针运行时并发死锁**：`safeRuntimeConn` 的 `readCloseMu` 在底层 `Read()` 处于阻塞状态时执行 `Close()` 发生互斥死锁，导致探测连接取消与超时无法及时中断；
3. **拨号器错误归类过宽**：单节点 DNS 解析失败或私网目标被 SSRF 策略拒绝时，`safe_dialer.go` 统一包裹 `ErrCredentialsUnavailable`，导致 runner 将其判定为系统级致命错误而使整个探测批次全部置为失败；
4. **订阅删除残留活跃孤儿节点**：删除订阅源时仅级联删除 `node_sources`，未对失去所有来源的活跃节点执行状态停用，造成台账残留无效僵尸节点；
5. **观测历史分页与计数偏差**：节点观测历史接口采用 `page*size` LIMIT 且以内存结果长度作为 `total`，导致前端分页组件计算失真；
6. **空白凭据引用更新静默穿透**：PATCH 订阅源时若传递空白字符串 `source_url_secret_ref`，传输层修剪导致命令层接收 `nil` 而完全绕过必填校验；
7. **策略准入规则缺乏删除端点**：策略规则支持新增却未开放删除 REST 路由与 Composable 方法，已建规则无法清理；
8. **前端鉴权 Cookie 缺少 CSRF 令牌投递**：在 Cookie 会话模式下，`ApiClient` 未提取和投递 `X-CSRF-Token`，导致状态变更请求被 `middleware_auth.go` 拦截（403）；
9. **局域网 HTTP 下 UUID 生成崩溃**：`crypto.randomUUID()` 在非安全上下文（纯 HTTP 局域网）中未定义，导致订阅刷新报错；
10. **探测工作台高频分页轮询风暴**：`ProbesView.vue` 每 2 秒循环拉取最多 20 页全量节点，产生极高无谓请求开销；
11. **节点台账筛选状态下无限滚动饥饿**：客户端健康度筛选后若视口未被填满且无法触发滚动事件，已请求数据无法自动递增分页；
12. **订阅抽屉必填校验跳过**：保存按钮直接调用提交逻辑，未通过表单有效性断言即可跳过必填字段保存；
13. **策略组 URL 路径未执行安全编码**：策略组相关 REST 请求未对标识符使用 `encodeURIComponent`；
14. **发布中心刷新后复制链接缺失令牌**：单次发布后重新加载页面，导出 URL 缺失 `token` 参数导致复制无效订阅地址；
15. **Playwright E2E 语言断言冲突**：真实服务端到端测试硬编码英文文案断言，与默认中文语言环境不符；
16. **历史路由 SPA 降级误响应**：`/yaml` 与 `/script` 遗留端点未正确拦截为 410 Gone，误降级为 SPA 200；
17. **环境示例残留 Python 时代遗留变量**：`.env.example` 中包含 `CLASH_AUTH_TOKEN` 等无效变量，误导运维配置导致认证失效；
18. **17000 端口生产环境混淆**：宿主机 17000 为线上容器映射端口，非测试预览环境，需确立严格隔离准则。
本项目内 OpenSpec 变更将上述经确证的缺陷组织为五个高内聚、零重叠写集合的特性级工作包，建立确定性修复与门禁验证契约。
```

### Plan Contract DOC-0002 <a id="plan-doc-0002"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/proposal.md:25`
- **Source Digest**: `070ab4619584a278fd6d887b4eaef971f16cfd4a675a91a3aaed068c600bc9af`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **配置修订生命周期与规则管理完善**：
- 服务端启动时在空数据库上自动引导初始有效活跃配置修订（Active Revision），消灭全新数据库首次添加规则报 422 `missing_revision_id` 与发布中心 409 `no_active_revision`。
- 新增 `POST /api/v1/revisions` 显式创建/升级修订端点。
- 补充准入规则与策略规则的删除 REST 路由（`DELETE /api/v1/policies/rules/{id}`）及底层仓储能力。
- 明确遗留端点拦截：`GET /yaml` 与 `GET /script` 显式返回 HTTP 410 Gone 及指引，禁止 SPA 200 降级。
- **探针运行时并发死锁消除与错误分类精确化**：
- 重构 `safeRuntimeConn` 的关闭与超时机制：在执行互斥加锁前递归中断底层套接字读写超时并安全释放底层连接，杜绝 `Read()` 阻塞导致 `Close()` 死锁。
- 拨号器错误解耦：将 DNS 解析失败与私网/SSRF 目标拦截归类为节点级目标错误（`ErrTargetUnresolvable` / `ErrPrivateTargetRejected`），在生成 `VerdictUnavailable` 观测证据后正常结束任务，不再设置 `hasFatalDialErr`，杜绝单节点故障拖垮全局探测。
- 修正节点观测历史接口 `/api/v1/nodes/{logical_id}/observations`：仓储层采用真实 `LIMIT ? OFFSET ?` 并配合 `COUNT(*)` 计算真实 `total`。
- **订阅生命周期数据一致性与变更准入加固**：
- 订阅删除事务原子级联清理孤儿节点：删除订阅源时，原子执行无所属来源活跃节点的去活操作（`SET active = 0`），杜绝无效孤儿僵尸节点。
- 修复 PATCH 订阅空白引用绕过缺陷：传输层保持空字符串指针传递，使 `invalid_source_url_secret_ref` 校验逻辑正常生效。
- **前端通信安全、设备兼容与状态交互自愈**：
- `ApiClient` 与 `useAuth` 支持 CSRF 令牌管理：提取登录响应中的 `csrf_token` 并在状态变更请求头中附带 `X-CSRF-Token`。
- 健壮的 UUID 生成降级：在非安全上下文（纯 HTTP 局域网）提供符合 RFC4122 v4 的退避算法，避免 `crypto.randomUUID()` 未定义异常。
- 消除探测工作台高频分页风暴：移除每 2 秒轮询 20 页全量节点的激进循环，仅在任务或节点池变动时按需同步状态。
- 解决节点台账筛选状态下的无限滚动饥饿：当视口未充满且存在更多数据时自动补充请求，提供用户可恢复的加载机制。
- 订阅配置抽屉必填校验守卫：在保存前显式验证表单输入，阻止空白必填字段提交。
- 策略组 REST 请求 URL 编码：对所有动态路径参数使用 `encodeURIComponent`。
- 发布链接令牌持久化：在客户端安全会话存储中维护发布令牌，确保页面刷新后“复制订阅链接”依然完整携带 `?token=...`。
- **运维配置统一与自动化测试鲁棒性**：
- 同步 `.env.example` 与 `docker-compose.example.yml`，彻底清理 Python 时代 `CLASH_*` 遗留变量，统一采用 `CSP_*` 规范。
- 明晰 17000 端口生产宿主属性：严禁向 17000 或 18080 端口发送测试写入，所有本地与 E2E 测试均运行于动态临时端口（`0`）及独立临时数据库。
- 调整 Playwright E2E 语言环境初始化：通过显式注入 `en-US` 语言或断言本地化标识符，消除默认中文语言导致的字符串断言失败。
```

### Plan Contract DOC-0003 <a id="plan-doc-0003"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/proposal.md:54`
- **Source Digest**: `070ab4619584a278fd6d887b4eaef971f16cfd4a675a91a3aaed068c600bc9af`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `audit-and-defect-remediation`: 针对 CSP 1.0 全栈经审计确证的功能缺陷提供确定性修复，涵盖修订引导、规则增删、探针死锁与错误分类、观测分页、订阅孤儿去活、CSRF 请求头注入、局域网 UUID 退避、前端轮询风暴优化与视口防饥饿。
```

### Plan Contract DOC-0004 <a id="plan-doc-0004"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/proposal.md:58`
- **Source Digest**: `070ab4619584a278fd6d887b4eaef971f16cfd4a675a91a3aaed068c600bc9af`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（`openspec/specs/` 下无对应已归档主规格，本次通过新增能力规格建立权威契约）。
```

### Plan Contract DOC-0005 <a id="plan-doc-0005"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/proposal.md:62`
- **Source Digest**: `070ab4619584a278fd6d887b4eaef971f16cfd4a675a91a3aaed068c600bc9af`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **后端代码**：
- `cmd/csp/main.go`
- `internal/application/revision/service.go`
- `internal/application/policy/service.go`
- `internal/application/policy/types.go`
- `internal/application/probe/safe_dialer.go`
- `internal/application/probe/runner.go`
- `internal/application/subscription/service.go`
- `internal/domain/ports.go`
- `internal/domain/probe.go`
- `internal/probe/singbox/runtime.go`
- `internal/repository/sqlite/policy.go`
- `internal/repository/sqlite/probes.go`
- `internal/repository/sqlite/subscriptions.go`
- `internal/transport/http/policies.go`
- `internal/transport/http/probes.go`
- `internal/transport/http/revisions.go`
- `internal/transport/http/router.go`
- `internal/transport/http/subscriptions.go`
- **前端代码**：
- `web/src/api/client.ts`
- `web/src/features/auth/useAuth.ts`
- `web/src/features/nodes/NodesView.vue`
- `web/src/features/policy/usePolicy.ts`
- `web/src/features/probes/ProbesView.vue`
- `web/src/features/probes/useProbes.ts`
- `web/src/features/publications/PublicationsView.vue`
- `web/src/features/publications/usePublications.ts`
- `web/src/features/subscriptions/SubscriptionConfigDrawer.vue`
- `web/src/features/subscriptions/useSubscriptions.ts`
- **配置与测试**：
- `.env.example`
- `web/e2e/auth-real-server.spec.ts`
- 后端单测与集成测试用例补充
```

### Plan Contract DOC-0006 <a id="plan-doc-0006"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/proposal.md:1`
- **Source Digest**: `ec6d3fecf6cd09621a7ca07617051cac5f0d09320fe3994cc67669145699e6aa`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前生产环境 CSP 1.0 单二进制控制面（`sub.einck.top` / `127.0.0.1:18080`）处于 100% 鉴权死锁状态：由于 `internal/transport/http/middleware_auth.go:29` 仅在 `cfg.AdminToken != ""` 时校验通过，且对无凭据请求无条件拦截为 401，而 `docker-compose.yml:13` 默认配置 `CSP_ADMIN_TOKEN: ${CSP_ADMIN_TOKEN:-}` 为空，导致未配置 Token 时所有控制面请求均被阻断（`HTTP 401 Unauthorized: Authentication credentials required`）。同时，前端 `web/src/api/client.ts:169` 仅被动读取 `localStorage.getItem('csp_token')`，全仓无 Token 输入弹窗或凭据录入界面，`web/src/App.vue:183` 设置页仍为占位卡片；且前期 Critic 终审仅依靠 Mock 网络路由（`web/e2e/cross-device.spec.ts:6-64`）与单测硬编码 Token 产生虚假绿灯，从未对生产容器执行真实的匿名无头浏览器全链路质检。
本变更旨在建立工业级开放/受控双模态鉴权生命周期、前端全局凭据管理与 401 拦截弹窗，并制定对抗级真机无头浏览器质检门禁，彻底解决开箱死锁并确保可用性真实闭环。
```

### Plan Contract DOC-0007 <a id="plan-doc-0007"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/proposal.md:7`
- **Source Digest**: `ec6d3fecf6cd09621a7ca07617051cac5f0d09320fe3994cc67669145699e6aa`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **后端鉴权双模态支持**：修改 `internal/transport/http/middleware_auth.go`。当 `cfg.AdminToken == ""`（未配置环境变量 `CSP_ADMIN_TOKEN`）时，自动运行于 Zero-Config Open Mode（开放私有模式），直接放行管理请求并赋予 Admin 上下文；当 `cfg.AdminToken != ""` 时，严格执行现有 Bearer / Cookie 鉴权逻辑。
- **导出令牌安全隔离硬门禁**：在任何模式下（包括 Open Mode），携带导出令牌（Publication Export Token）访问 `/api/v1/` 管理接口必须严格拦截为 403 Forbidden（`invalid_token_scope`），杜绝越权。
- **公共鉴权状态查询端点**：新增免认证端点 `GET /api/v1/auth/mode`，返回系统当前运行模态（`open` 或 `token`），供前端初始化时动态识别服务安全状态与展示引导。
- **前端全局 401 拦截与 AuthModal**：在 `web/src/api/client.ts` 增加 401 响应事件拦截钩子；在 `web/src/ui/AuthModal.vue` 实现响应式凭据录入弹窗，支持输入 Admin Token 并持久化至 `localStorage['csp_token']`，提交后自动刷新视图数据。
- **设置视图与凭据管理落地**：在 `web/src/features/settings/SettingsView.vue` 取代 `App.vue` 中的 Phase 6.1 占位卡片，提供鉴权模态展示、Token 查看/更新/清除与连接性验证。
- **顶部状态栏鉴权指示器**：在 `web/src/App.vue` 顶部导航栏增加鉴权状态徽章（如 `Open Mode` / `Protected`），直观呈现安全水位。
- **Critic 真机对抗质检门禁**：制定铁律级 E2E 质检规范，禁止在端到端质检中使用 `page.route` 伪造网络数据；必须使用无头浏览器（Playwright/CDP）针对真实运行态服务分别验证“匿名态（未注入 Token）”与“受控态（注入 Token）”，断言控制台零 401 报错且 DOM 正常渲染。
```

### Plan Contract DOC-0008 <a id="plan-doc-0008"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/proposal.md:19`
- **Source Digest**: `ec6d3fecf6cd09621a7ca07617051cac5f0d09320fe3994cc67669145699e6aa`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `control-plane-auth-lifecycle`: 定义后端 Zero-Config Open Mode 与 Protected Mode 双模态鉴权、发布令牌隔离与鉴权模态探测接口契约。
- `frontend-credential-management`: 定义前端全局 401 拦截、AuthModal 凭据录入、设置中心 Token 管理与顶部安全状态指示交互契约。
- `critic-headless-adversarial-verification`: 定义禁止 Mock 路由、双态真机无头浏览器对抗验证、控制台日志与 DOM 几何断言质检门禁规范。
```

### Plan Contract DOC-0009 <a id="plan-doc-0009"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/proposal.md:25`
- **Source Digest**: `ec6d3fecf6cd09621a7ca07617051cac5f0d09320fe3994cc67669145699e6aa`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。
```

### Plan Contract DOC-0010 <a id="plan-doc-0010"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/proposal.md:29`
- **Source Digest**: `ec6d3fecf6cd09621a7ca07617051cac5f0d09320fe3994cc67669145699e6aa`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **受影响后端组件**：`internal/transport/http/middleware_auth.go`（双模态放行与发布令牌拦截）、`internal/transport/http/router.go`（注册 `/api/v1/auth/mode` 路由）。
- **受影响前端组件**：`web/src/api/client.ts`（401 响应拦截与凭据存取钩子）、`web/src/ui/AuthModal.vue`（新增凭据录入组件）、`web/src/features/settings/SettingsView.vue`（新增设置视图）、`web/src/App.vue`（接入弹窗、设置页路由与安全徽章）。
- **受影响测试与质检工具**：`web/e2e/real-server.spec.ts`（新增真实后端无 Mock E2E 测试脚本）、Critic 验收工作流规范。
- **部署与配置影响**：完全向后兼容。`docker-compose.yml` 保持 `CSP_ADMIN_TOKEN: ${CSP_ADMIN_TOKEN:-}`，未配置时即刻恢复服务可用性；配置时安全加固，不影响既有业务数据与节点台账。
```

### Plan Contract DOC-0018 <a id="plan-doc-0018"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:15`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `debloat`: 移除节点凭据与发布产物 AEAD 加密、身份绑定、CAS 版本号及前端掩码，改为 SQLite 明文直存、直编、直发与明文可编辑管理契约。
```

### Plan Contract DOC-0019 <a id="plan-doc-0019"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:19`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `debloat` 能力规格建立完整减法契约）。
```

### Plan Contract DOC-0020 <a id="plan-doc-0020"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:23`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`migrations/000011_*.sql`、`internal/domain/`、`internal/parser/`、`internal/resolver/`、`internal/compiler/`、`internal/probe/singbox/`、`internal/repository/sqlite/`、`internal/application/{inventory,publication,probe,subscription}/`、`internal/transport/http/`、`internal/integration/`、`cmd/csp/`、`docker-compose.yml` 及 `web/src/features/{nodes,publications,subscriptions}/`。本阶段仅在隔离环境完成实现与验收，不触碰生产容器与线上数据库，生产库迁移通过备份保障并在升级后触发 `ReconcileSubscription` 自动重填明文节点配置。
```

### Plan Contract DOC-0021 <a id="plan-doc-0021"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/proposal.md:1`
- **Source Digest**: `3fd54e6cae0d69fcfcc9306db4d2870358f5c1d55105e4b0571752069b68d767`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前生产环境出现订阅发布请求 404 与拉取失败：经深度排查，生产 SQLite 数据库 `publications` 表当前为 0 条记录，客户端请求历史 `/p/...` 路径报 404（CSP 标准导出路径为 `/publish/v1/{publication_id}`）；且现有导出端点硬编码强制要求专属 `pub_<hex>` 令牌，不支持免密公开拉取，也不支持使用管理端配置的统一系统令牌。同时，管理端鉴权粗暴绑定于密码字符串是否存在，缺少独立控制开关，无法满足用户关于“管理端与导出链接鉴权开关相互独立、但可复用同一个统一令牌”的核心诉求。
```

### Plan Contract DOC-0022 <a id="plan-doc-0022"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/proposal.md:5`
- **Source Digest**: `3fd54e6cae0d69fcfcc9306db4d2870358f5c1d55105e4b0571752069b68d767`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **鉴权持久化开关扩展**：新增数据库迁移 `000013_auth_settings.sql`，在 `settings` 表中扩展 `admin_auth_enabled`（默认 1）和 `export_auth_enabled`（默认 1）字段，使管理端鉴权与导出链接鉴权在数据库层面解耦持久化。
- **领域层与仓储契约升级**：扩展 `internal/domain/settings.go` 与 `internal/repository/sqlite/settings.go`，支持 `AdminAuthEnabled` 与 `ExportAuthEnabled` 状态读取、写入与向后兼容查询。
- **鉴权动态模型解耦**：重构 `internal/transport/http/token.go` (`DynamicTokenHolder`)，维护 `adminAuthEnabled` 与 `exportAuthEnabled` 状态，提供独立的 `AdminMode()` 与 `ExportMode()` 判定，支持管理端在配置了令牌的情况下仍可通过独立开关免密开放（Zero-Config Open Mode），以及导出端点独立判定是否要求凭证。
- **发布导出双模态强化与统一令牌复用**：改造 `internal/transport/http/publications.go` 中的 `publicationClientHandler`：当 `export_auth_enabled == false` 时，无条件放行匿名客户端公开拉取；当 `export_auth_enabled == true` 时，客户端既可凭 publication 专属 TokenHash 访问，亦支持使用管理端配置的统一系统令牌（复用同一个令牌），解决生产换发或多客户端共享令牌难题。
- **路由兼容性增强**：在 `internal/transport/http/router.go` 增设 `/p/{publication_id}` 兼容路由（重定向至 `/publish/v1/{publication_id}` 或直接处理），防止旧版客户端和历史外部订阅链接失效。
- **设置管理 API 扩展**：提供标准 `GET/PUT /api/v1/settings/auth` 接口，支持统一查询与原子修改鉴权开关及系统令牌；同时保持对既有 `/api/v1/settings/admin-token` 与 `/api/v1/auth/status` 的全兼容。
- **前端设置中心独立开关与状态指示**：在 `web/src/features/settings/SettingsView.vue` 落地管理端鉴权与导出链接鉴权独立开关交互，直观显示管理端安全水位与导出链接受控状态，统一维护共享系统令牌。
- **前端发布工作台引导与免密直链动态呈现**：在 `web/src/features/publications/PublicationsView.vue` 增强零记录时的发布引导提示，并根据导出鉴权开关动态呈现免密直链（不带 Token）或受控链接（带 Token），对齐一键发布与复制交互。
```

### Plan Contract DOC-0023 <a id="plan-doc-0023"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/proposal.md:18`
- **Source Digest**: `3fd54e6cae0d69fcfcc9306db4d2870358f5c1d55105e4b0571752069b68d767`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `baseline-auth-and-export`: 控制面管理端与订阅发布导出端点的基准鉴权协议契约，包括 Bearer Token / Cookie 会话验证、发布令牌管理端越权强隔离与标准 HTTP 导出路径基线。
- `decoupled-auth-settings`: 管理端与导出链接鉴权独立开关（`admin_auth_enabled`、`export_auth_enabled`）解耦、统一系统令牌管理、`000013_auth_settings.sql` 存储迁移及 `/api/v1/settings/auth` API 契约。
- `publication-export-auth-recovery`: 订阅发布端点免密公开访问放行、专属发布令牌与统一系统令牌双重鉴权验证、`/p/...` 路径兼容与发布向导故障恢复交互契约。
```

### Plan Contract DOC-0024 <a id="plan-doc-0024"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/proposal.md:24`
- **Source Digest**: `3fd54e6cae0d69fcfcc9306db4d2870358f5c1d55105e4b0571752069b68d767`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。
```

### Plan Contract DOC-0025 <a id="plan-doc-0025"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/proposal.md:28`
- **Source Digest**: `3fd54e6cae0d69fcfcc9306db4d2870358f5c1d55105e4b0571752069b68d767`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **数据持久层**：新增 `migrations/000013_auth_settings.sql`，安全修改 `settings` 表追加两列；更新 `internal/domain/settings.go`、`internal/domain/ports.go` 及 `internal/repository/sqlite/settings.go`。
- **控制面运行时**：`internal/transport/http/token.go` (`DynamicTokenHolder`)、`internal/transport/http/middleware_auth.go`、`internal/transport/http/router.go`、`internal/transport/http/auth.go`、`internal/transport/http/settings.go`。
- **订阅发布端点**：`internal/transport/http/publications.go` (`publicationClientHandler`)、`internal/application/publication/service.go`。
- **Web 前端**：`web/src/features/settings/SettingsView.vue`、`web/src/features/publications/PublicationsView.vue`、`web/src/features/publications/usePublications.ts`、相关单元测试与 i18n 语言包（`web/src/locales/`）。
- **外部兼容性**：完全向后兼容现有的 HTTP API 与 Docker 部署流程，默认延续双重受控模式（`DEFAULT 1`），不会引发已有已配置凭据服务的权限降级。
```

### Plan Contract DOC-0026 <a id="plan-doc-0026"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/proposal.md:1`
- **Source Digest**: `3e5329b834a2dfd2dce3e5973ce288df919c84c8039f4c441f5a3d9e87640eb6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

本 Change 已完成库存清空、两源 Token 修复、协议身份扩展、维护 CLI、规则管理及历史生产切换，现有 A/B 实现必须保留。2026-10-08 最新只读预检证明新比较尚未公平覆盖 ALL90、stage-first 执行器及跨启动预算未闭环、最后源码没有全量成功门禁；本次按用户已接受的分阶段实验修订同一 Change，不重做历史操作、不以标题计数相等替代缺陷解释。
```

### Plan Contract DOC-0027 <a id="plan-doc-0027"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/proposal.md:5`
- **Source Digest**: `3e5329b834a2dfd2dce3e5973ce288df919c84c8039f4c441f5a3d9e87640eb6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **保留特性 A 与既有 B**：显式空组 PASS 默认严格、非 DIRECT、真实节点计数不变；PCRE 兼容与规则校验/UI 语义保持原合同。只补最终版本回归和必要整改，不 blanket-enable 生产用户 group flag，不 reset/import、重复修 Token 或删除规则。
- **完成一个 stage-first 比较整改包**：`tools/probe-comparison/run.py` 先执行 CSP 与独立标记的 same-engine Mihomo v1.19.32 comparator 的 ALL90 baseline AB/BA 四侧，然后在四侧 baseline 交集上执行平台阶段。共同资格之外的 discordant 节点保留逐节点诊断，不把未执行、取消或短时失败改写为永久 dead。
- **native 与归一比较分离**：真实 upstream 固定 commit `3c320fd58aff5235e16218c050ec5b8ce587e233`，native v1.19.31 `Check` 方法保持真实原生配置与默认差异，测速关闭。same-engine comparator 的版本、modfile/传递依赖 diff 与 build provenance 单列。默认只跑主比较四侧；仅在首次请求前冻结且不损害主覆盖的同一总盘子中允许额外两侧 native baseline-only 控制，不强制六侧。
- **新批次无测速且硬限额**：alive8/media2/speed0；没有 speed URL 或测速请求。实际 baseline runner 每 node×side 响应 body 上限 64 KiB，四侧最坏预留 22.5 MiB；所有 launches 共用原子 reservation/refund ledger、同一起点与截止，整批应用响应 body ≤128 MiB、整批含清理 ≤30 分钟。平台剩余额度四侧公平相等、逐节点参与有界；不保证原生全部 media 能在上限内完成，准确列出未尝试阶段，不漏算非 collector 平台调用，不增加重试、512 MiB 或自动扩额。
- **真实终态顺序与现存授权**：离线全部代码自测（含 native/same-engine/reference fixtures）→独立最终 Reviewer→已批准的实网 staged batch→最终源码绑定的刷新隔离预览与 Critic→正确 COPY flag 同步嵌入资产→仅授权 CSP Git commit→一致性热备/rollback 演练/app-only runbook 部署与健康验证。7200 秒观察独立于 rollout 完成，不能以健康轮询冒充。发布依据解释清楚的缺陷与适用门禁，不要求 headline 数字一致。
```

### Plan Contract DOC-0028 <a id="plan-doc-0028"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/proposal.md:15`
- **Source Digest**: `3e5329b834a2dfd2dce3e5973ce288df919c84c8039f4c441f5a3d9e87640eb6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- `policy/rules-validation`：保留显式空组 PASS、过滤保真、统一校验输入及 UI 刷新/编辑合同；新 UI 改动仍须最终独立审查和成品验收。
- `maintenance/reset-node-inventory-cli`：补齐 ALL90 优先、四侧 same-engine/native 分离、无测速、整批跨启动预算/时间、固定 manifest 和逐节点阶段归因。
- `inventory/clean-slate-node-reset`、`subscription/source-token-repair`：保留已完成合同，不新增重复执行授权。
```

### Plan Contract DOC-0029 <a id="plan-doc-0029"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/proposal.md:20`
- **Source Digest**: `3e5329b834a2dfd2dce3e5973ce288df919c84c8039f4c441f5a3d9e87640eb6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

本次规划只修改官方 CLI 解析出的同一 Change 的既有 proposal/design/tasks 和 maintenance delta spec，不写业务、测试、生产配置，不派工、不执行实验、构建、部署、提交或 QQ 通知。后续整改写集合及拟定精确 CLI 接口见 design/tasks；已有 A/B 工作树、失败历史与私有证据均保留。
最新证据来源为指定预检 session `2026-10-08T05-50-14-467Z_879111db-a2c3b028-3b1a3e40-48b9.jsonl` 的最终报告 `rep_exec_csp_continuation_preflight_1`。其报告当前 run 为 `pi_run_20261008_134941_3738865`；本规划会话环境无不同的 run_id，不杜撰新 run。READY 只代表此修订计划完整、严格校验通过，不代表施工或发布门禁 PASS，也不重复索取已存在的实施/发布授权。
```

### Plan Contract DOC-0030 <a id="plan-doc-0030"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:1`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

用户发现部分操作菜单、弹窗在实际视口中被裁切或被其他弹层遮住，同时设置页缺少以探针状态为主的全局导出筛选入口。现有筛选后端已经存在，但发布解析在读取异常时可能静默回退为不过滤，产生不符合管理员意图的导出。
```

### Plan Contract DOC-0031 <a id="plan-doc-0031"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:5`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- 在设置页复用现有全局节点筛选存储与条件语言，提供以探针类型、判定和观测有效期为主的编辑入口；清楚解释空条件透传、未测/超龄观测排除、保存后只影响新预览及新发布，且不把队列瞬态或凭据版本匹配描述为已有筛选能力。
- 让四类导出目标的预览、新发布消费相同筛选解析结果；读取筛选或筛选依赖的观测/来源失败时禁止扩大导出，配置了筛选但候选节点为零时返回可操作的错误并拒绝意外发布。保留历史不可变发布直链与旧配置无筛选时的既有行为。
- 修复订阅卡片菜单被裁切、确认框被原生对话框遮挡，以及短视口弹层标题、正文与操作按钮不可用的问题；兼顾定位、键盘焦点和视口适配。
- 用服务层/组件测试与四视口加短高度浏览器验收覆盖关键路径；本 Change 不包含实施、部署或生产数据修改。
```

### Plan Contract DOC-0032 <a id="plan-doc-0032"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:14`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `global-probe-export-filter`: 设置页探针优先的全局筛选、安全解析、预览/发布及零结果行为。
- `viewport-safe-overlays`: 菜单、对话框、确认框及抽屉在可用视口内的层叠、可读与可操作行为。
```

### Plan Contract DOC-0033 <a id="plan-doc-0033"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:19`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（`openspec/specs/` 当前无已发布能力规格；此 Change 新增完整行为契约，不修改其他 Change 的历史文档）。
```

### Plan Contract DOC-0034 <a id="plan-doc-0034"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:23`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`internal/application/publication/**`，`web/src/features/settings/**`，`web/src/features/policy/PolicyView.vue`，`web/src/ui/{Popover,ModalDialog,ConfirmModal,Drawer}.vue` 及相关 subscriptions/publications/policy/probes 组件与测试。沿用 `GET/PUT /api/v1/policies/global-node-filter`、现有 `NodeFilterSpec`/resolver、Vue 现有 Teleport/原生 `<dialog>`/浏览器 top layer 与已有自适应 CSS token；不新建第二套筛选存储或引入额外浮层框架。四目标为 Mihomo、sing-box、Surge、Quantumult X；已发布直链继续提供创建时冻结的内容。上线另需授权。
```

### Plan Contract DOC-0035 <a id="plan-doc-0035"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/proposal.md:1`
- **Source Digest**: `6cd62a5ff3de70227de7c8914e60fd88fd273bc066c06cbc31080eecdcf34943`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

用户反馈：在开启定时检测后，由于系统原先将整个探测周期（如 1 小时或 2 小时）作为单一粗粒度定时器，在未到达完整周期前系统完全不执行检测；且一旦时间到达，又将全量节点的所有探测类别同时并发倾泻入池，造成瞬时冲击与长周期等待。用户期望系统具备常态化轻量巡检能力（默认每 10 分钟或合适频次巡检一次数据库中的节点观测记录），仅将已过期或尚无观测记录的 (node, kind) 增量送检，而非全量一次性突发检测。
```

### Plan Contract DOC-0037 <a id="plan-doc-0037"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/proposal.md:17`
- **Source Digest**: `6cd62a5ff3de70227de7c8914e60fd88fd273bc066c06cbc31080eecdcf34943`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `incremental-expiry-probe-sweep`: 提供基于固定轻量巡检时钟与逐节点/类别观测有效期的增量过期探测调度、有界配额分批入池、平滑升级校正与前端保底巡检感知。
```

### Plan Contract DOC-0038 <a id="plan-doc-0038"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/proposal.md:22`
- **Source Digest**: `6cd62a5ff3de70227de7c8914e60fd88fd273bc066c06cbc31080eecdcf34943`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- 后端调度与应用层：`internal/application/probe/periodic.go`, `internal/application/probe/service.go`, `cmd/csp/main.go`。
- 依赖注入：`PeriodicCoordinator` 注入 `ProbeObservationRepository` 查询最新观测。
- 契约语义：`ProbeSchedule.NextDueAt` 统一表达下次巡检扫描时间；`IntervalSeconds` 表达节点级观测过期阈值。
- 前端视图与状态管理：`web/src/features/probes/ProbesView.vue`, `web/src/features/probes/useProbes.ts`。
```

### Plan Contract DOC-0041 <a id="plan-doc-0041"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/proposal.md:19`
- **Source Digest**: `8d8a51d33ceaa7f0e99e60d9a492b9f1184057f2f28fd993ad75c47928c91a93`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `ip-risk-evidence`: 定义经被测节点出口取得、脱敏存储、过期和可解释读取的 IP 风险证据合同
- `risk-policy-admission-and-publication`: 定义风险分级、节点筛选、策略组准入和导出前硬拦截合同
- `ip-risk-management-workbench`: 定义节点台账风险回显、筛选、高风险提示和未知状态展示合同
```

### Plan Contract DOC-0045 <a id="plan-doc-0045"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/proposal.md:10`
- **Source Digest**: `0707deb099f9f2dcac158a0e6b38ea2886bb60c43923843a71bfba221fcf4d14`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

1. **建立 6 大业务领域主干能力规范 (Living Specs)**：
- 在 `openspec/specs/` 下建立 `inventory`、`policy`、`probe`、`publication`、`auth`、`ui` 规范体系；
- 吸收 GitHub Spec-Kit 结构化范式（优先级场景 User Scenarios、稳定需求编号 FR-ID、可度量标准 SC-ID），适配 OpenSpec 严格语法并通过 `openspec validate --specs --strict` 校验。
2. **导入官方模板参考存根与架构决策记录**：
- 在 `.specify/templates/` 存放 Spec-Kit 官方模板副本及 provenance 哈希记录，声明保留 repo-local `openspec` CLI 作为执行框架；
- 在 `docs/decisions/proposed/architecture/` 编写 CSP 架构决策记录，客观记录 `proposed` 状态与被否决方案（`.agents/notes/README.md` 仅保留跳转索引）；
3. **输出多维度工程审计文档与待决问题清单**：
- `docs/specification/source-register.md`：建立四级来源分级体系，严格甄别用户原话；
- `docs/specification/conformance-matrix.md`：建立 37 项功能需求的五态符合度审计矩阵，附带精准实机代码行与测试退出码；
- `docs/specification/clarification-questions.md`：汇总 6 组核心业务待决问题，列出分项选项，绝不自行预设默认值；
- `docs/specification/upstream-provenance-and-coverage.md`：逐一映射 26 个既有变更集与 63 份历史规格，确保契约零遗失；
- `docs/specification/maintenance-guide.md`：建立长期规范维护与演进指南。
```

### Plan Contract DOC-0046 <a id="plan-doc-0046"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/proposal.md:27`
- **Source Digest**: `0707deb099f9f2dcac158a0e6b38ea2886bb60c43923843a71bfba221fcf4d14`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `living-spec-consolidation`: 建立项目持续主干能力规范管理体系，包含 6 大业务领域 Living Spec、四级来源确权、实机符合度审计矩阵及跨变更覆盖追溯能力。
```

### Plan Contract DOC-0047 <a id="plan-doc-0047"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/proposal.md:31`
- **Source Digest**: `0707deb099f9f2dcac158a0e6b38ea2886bb60c43923843a71bfba221fcf4d14`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本变更集保留全部既有 26 个变更集，不作归档或删除。
```

### Plan Contract DOC-0048 <a id="plan-doc-0048"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/proposal.md:35`
- **Source Digest**: `0707deb099f9f2dcac158a0e6b38ea2886bb60c43923843a71bfba221fcf4d14`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **代码与运行时**：零修改。本阶段不改动任何 Go 后端代码、Vue 前端代码、SQL 迁移脚本或 Docker 部署配置，不触碰正在健康运行的生产容器。
- **文档与治理**：新增 `openspec/specs/` 主干规范、`.specify/templates/` 存根、`docs/decisions/` 决策记录及 `docs/specification/` 系列审计文档。
- **流程状态**：本变更完成文档编制与本地自测后，流转独立审查，业务代码落地需待用户对 6 组问题裁决后另行实施。
```

### Plan Contract DOC-0049 <a id="plan-doc-0049"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/proposal.md:1`
- **Source Digest**: `674c4490d44e3254aac0b13e6ed37ddac12bf58d62ceb5db7a571cc71f43fb1c`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

根据用户的明确需求指令：
> “还记得我们之前说的节点池的概念，现在好像没有这个概念，重新实现当定时测速的时候，添加节点池，如果节点池已经有了，就不重复添加，手动检测插在队列最前面，然后检测状态显示，当前队列中的节点数，未测数，总数，不可用数，可用数，设计一个好官，合适恰当前端，节点状态添加检测中”
经过对当前前后端探针引擎与节点台账的实机取证，定位到以下核心缺失与体验痛点：
1. **缺乏统一的“节点池（Node Probe Pool）”概念与去重/插队调度机制**：
- 当前 `internal/probe/queue/scheduler.go` 仅按 `(RunID, Task)` 做简单轮询调度，且当同一节点与探测维度在短时间或并发批次中再次提交时直接报错 `ErrNodeConflict`（导致定时测速与手动测速重叠时任务直接失败）；
- **定时测速未做节点池去重**：定时测速触发时未能检查节点是否已在节点池中，缺少“若节点已在池中（排队等待或正在检测）则静默跳过、不重复入池”的机制；
- **手动检测无法插队到最前面**：用户手动点击单节点重测、已选节点测速或全量测速时，无法插在节点池队列最前面优先执行，也无法将已在池中排队的节点提升（Promote）到队首。
2. **节点状态缺失“检测中（`probing`）”实时态与节点池 5 大统计指标 API**：
- 后端 `NodeView`（`GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}`）仅根据历史持久化的 `probe_observations` 计算静态 `health_status`（`healthy` / `degraded` / `unhealthy` / `unknown`），完全不知道某个节点此刻是否正在节点池中处于 **`probing`（检测中）** 或 **`queued`（队列等待中）**；
- 缺少统一的节点池状态接口 `GET /api/v1/probes/pool` 来实时汇总并返回用户明确要求的 5 大核心指标：**当前队列中的节点数**、**未测数**、**总数**、**不可用数**、**可用数**。
3. **前端顶部看板与节点状态展示未体现节点池与“检测中”直观反馈**：
- `ProbesView.vue` 顶部 KPI 栏未能醒目、直观地呈现节点池 5 大核心检测状态指标（当前队列中的节点数、未测数、总数、不可用数、可用数）；
- 状态筛选器、状态图例、`ProbesView.vue` 节点表格与 `NodesView.vue` 节点卡片/详情抽屉缺少动态的 **「检测中」**（`probing`）与 **「队列中」**（`queued`）状态徽章及插队反馈。
```

### Plan Contract DOC-0051 <a id="plan-doc-0051"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/proposal.md:41`
- **Source Digest**: `674c4490d44e3254aac0b13e6ed37ddac12bf58d62ceb5db7a571cc71f43fb1c`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `node-probe-pool`: 统一节点池双端优先级队列（定时测速去重入池、手动检测插队最前）、节点“检测中（`probing`）”实时状态透传、`GET /api/v1/probes/pool` 5 大核心统计指标契约，以及前端节点池状态看板与“检测中”全链路视觉交互规格。
```

### Plan Contract DOC-0052 <a id="plan-doc-0052"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/proposal.md:45`
- **Source Digest**: `674c4490d44e3254aac0b13e6ed37ddac12bf58d62ceb5db7a571cc71f43fb1c`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `node-probe-pool` 能力规格建立完整契约）。
```

### Plan Contract DOC-0053 <a id="plan-doc-0053"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/proposal.md:49`
- **Source Digest**: `674c4490d44e3254aac0b13e6ed37ddac12bf58d62ceb5db7a571cc71f43fb1c`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- 后端：`internal/probe/queue/**`、`internal/application/probe/**`、`internal/application/inventory/**`、`internal/domain/**`、`internal/transport/http/{probes,nodes,router}*.go`、`cmd/csp/main.go`
- 前端：`web/src/features/probes/**`、`web/src/features/nodes/**`、`web/src/locales/**`
- 验证边界：仅在本地单测与隔离预览环境（临时 SQLite 库、独立非生产端口与临时构建目录）执行自测、代码审查与真浏览器多视口验收，严禁触碰生产环境。
```

### Plan Contract DOC-0054 <a id="plan-doc-0054"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:1`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前探测主要通过手动 `/probes/runs` 提交，缺少面向仓库激活节点的可配置、可恢复周期任务；策略组仅保存显式成员引用，无法按节点属性和探测结果筛选，也没有先于策略组筛选的全局筛选。用户另报告“csp出错”，但未提供当次请求、时间与错误日志；已核实历史 `target mihomo cannot render PROCESS-NAME` 修复位于 `7289b4c`，不得将该旧错误冒充本次根因。
```

### Plan Contract DOC-0055 <a id="plan-doc-0055"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:5`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- 增加持久化、默认关闭的周期探测计划：仅面向执行时仍激活且有可用凭据的节点，复用现有安全探测 runner / 有界队列；显示执行批次、失败原因、最新观测与取消/停用状态，并支持重启恢复与受控关闭。
- 增加可持久化的两级节点筛选：**全局筛选 → 按策略组筛选**；支持名称、协议、真实来源、探测状态/延迟与观测新鲜度，明确定义未知/过期、显式引用与空组处理；策略预览与发布共用同一解析结果。
- 增加管理 API 与界面，用于调整探测计划、全局筛选和分组筛选、预览诊断及复核执行证据；无筛选条件保留既有配置效果。
- 扩展 SQLite 增量迁移及自动化验收，保留历史手动探测与旧策略配置；不包含生产部署和任何未复现错误的推测性“修复”。
```

### Plan Contract DOC-0056 <a id="plan-doc-0056"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:14`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `periodic-active-probes`: 周期探测计划、执行证据、恢复与生命周期。
- `global-group-node-filters`: 全局先于组级的节点筛选及预览/发布一致性。
```

### Plan Contract DOC-0057 <a id="plan-doc-0057"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:19`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 没有已发布的同名主规格；此次以新增能力建立完整契约）。
```

### Plan Contract DOC-0058 <a id="plan-doc-0058"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:23`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`internal/domain/{probe,node,policy,ports}.go`、`internal/application/{probe,policy,publication}`、`internal/probe/queue`、`internal/resolver`、`internal/repository/sqlite`、`internal/transport/http`、`cmd/csp/main.go`、`migrations`、`web/src/features/{probes,policy}` 与 API 类型/测试。沿用 Go 标准 `context`/`time`、现有 `modernc.org/sqlite` 事务和索引、现有 sing-box runner 与有界公平队列，不引入第二套调度或表达式解析体系。上线、数据库备份和流量接入另需生产授权。
```

### Plan Contract DOC-0059 <a id="plan-doc-0059"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/proposal.md:1`
- **Source Digest**: `4591c9e00baae13d740df41aa0ef9819e9a462229cddd8f8eb699d11f3a3d9cd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前 CSP 策略树和探针引擎存在四大核心缺陷：
1. **全量节点截断导致匹配丢失**：`publication/service.go:812` 与 `policy/service.go:932` 调用 `nodeRepo.List` 时未指定分页参数，触发 `sqlite/nodes.go:114` 默认 `pageSize=50` 截断。在 90 个活跃节点的真实节点池中，台湾（排序 70-72）与加拿大（排序 78-80）节点被硬截断，导致策略组匹配为空，用户误以为是旧快照或规则失效；
2. **连接边与节点筛选条件语义混淆**：用户无法直观理解「显式连接边筛选」与「动态全池筛选」的关系，误以为规则冲突或应合并；
3. **探针健康状态失真与未检测混淆**：`BaselineUnknown` 叠加已有 AI/流媒体等辅助观测时被 `UntestedCount` 与前端 `unprobed` 混淆；纯辅助 `available` 危险回退为 `healthy` 破坏了只有新鲜 baseline 才能评判健康的铁律；缺乏 `undetermined` 状态导致已测辅助能力的节点被误报为「未检测」；
4. **前端分页缺失、瀑布流开销与侧栏布局滚动逃逸**：`usePolicy` 仅加载第 1 页 100 条组，`PolicyEditorSheet` 传 `page_size=200` 被后端截断至 100 漏掉后续节点，`useProbes` 存在 20 页瀑布流轮询，`App.vue` 侧栏随整页滚动消失，移动端遮罩缺乏体滚动锁定。
```

### Plan Contract DOC-0060 <a id="plan-doc-0060"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/proposal.md:9`
- **Source Digest**: `4591c9e00baae13d740df41aa0ef9819e9a462229cddd8f8eb699d11f3a3d9cd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **全量节点读取（ListAll）消除截断**：在 `NodeRepository` 引入显式全量查询接口 `ListAll`，供 `policy/service.go`（校验）与 `publication/service.go`（导出）使用，绕过 UI 分页截断，完整保留订阅 Scope、ActiveOnly、ExcludeNotices 与 IP 风险准入。严格保持 HTTP API 的默认 50/最大 100 分页限制，不盲改全局默认值；
- **澄清组连接边与筛选条件原生语义**：在 UI 统一编辑交互中明确区分「显式连接边（若存在边则候选边被筛选及父筛选下传）」与「动态全池规则（仅无任何边且非空 filter 时动态遍历全池）」，不改变底层代数运算，严禁自动静默删除生产连接边或筛选条件；
- **探针健康状态互斥守恒与 Undetermined 细分**：
- 废除纯辅助能力（如 AI/Streaming）`available` 回退为整体节点 `healthy` 的危险逻辑；整体健康必须且仅可信新鲜的当前 revision baseline；
- 区分真正的「全无观测未检测 (Untested)」与「观测不确定/过期/版本过时/待复核 (Undetermined)」；
- 后端 `ProbePoolStatus` 引入 `undetermined_count`，满足互斥守恒：`total_count = healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count`；
- 后端 `/api/v1/nodes` 支持全 Scope 的数据库级健康状态查询（`health_status`），禁止在前 100 条前端内存中统计全池；
- **服务端分页、候选异步搜索与拓扑解耦**：
- 策略组列表实现服务端标准分页（默认 50，上限 100）与搜索；
- 策略编辑器提供完整的异步搜索/分页候选节点与子组选择，坚决杜绝静默漏掉第 100 个以后的节点；
- 拓扑全图从独立接口或全量元数据解析渲染，与卡片列表分页明确解耦，拓扑全图不可被分页截断；
- 移除 `useProbes` 的 while-20 全量瀑布流，改为标准分页和直接使用后端 Pool 统计指标；
- **桌面端粘性侧栏与移动端体滚动锁**：
- 重构 `App.vue` 布局：Desktop 侧栏采用 `sticky top-0 h-screen`（或 `h-dvh`），在窗口滚动时永久固定；
- 移动端 Bottom Dock 与 Drawer/Sheet 弹出时启用 `overflow: hidden` 体滚动锁定，防穿透滚动；
- 经由 4 视口（Desktop 1280x800, Landscape 667x375, Mobile 375x667, Mobile 392x872）严格验证。
```

### Plan Contract DOC-0061 <a id="plan-doc-0061"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/proposal.md:30`
- **Source Digest**: `4591c9e00baae13d740df41aa0ef9819e9a462229cddd8f8eb699d11f3a3d9cd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `policy/full-inventory-resolution`: 后端全量节点读取（ListAll）消除策略树与导出截断，支持 250+ 节点完整解析与准入保留。
- `policy/membership-semantics-and-pagination`: 策略组成员筛选规则与连接边原语义澄清，策略组列表服务端分页搜索，候选成员异步完整检索与拓扑全图解耦。
- `probe/health-state-conservation-and-pagination`: 探针健康状态互斥守恒与 undetermined 计数分类，删除危险的辅助可用回退，全局节点分页健康筛选。
- `ui/navigation-layout-and-viewport-resilience`: Desktop 侧栏粘性固定布局，Mobile 导航与 Overlay 遮罩体滚动锁定，多视口适配保障。
```

### Plan Contract DOC-0062 <a id="plan-doc-0062"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/proposal.md:38`
- **Source Digest**: `4591c9e00baae13d740df41aa0ef9819e9a462229cddd8f8eb699d11f3a3d9cd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **后端持久层与服务层**：`internal/domain/ports.go`（新增 `ListAll`）、`internal/repository/sqlite/nodes.go`、`internal/application/policy/service.go`、`internal/application/publication/service.go`、`internal/application/probe/service.go`、`internal/transport/http/nodes.go`、`internal/transport/http/policies.go`；
- **前端模型与视图层**：`web/src/features/nodes/nodeView.ts`、`web/src/features/probes/useProbes.ts`、`web/src/features/probes/ProbesView.vue`、`web/src/features/policy/usePolicy.ts`、`web/src/features/policy/PolicyView.vue`、`web/src/features/policy/PolicyEditorSheet.vue`、`web/src/App.vue`；
- **API 契约**：`/api/v1/policies/groups` 明确分页与搜索参数，`/api/v1/probes/pool` 响应增加 `undetermined_count`，`/api/v1/nodes` 扩展 `health_status` 查询过滤；
- **隔离性与无副作用**：所有改动纯本地离线测试，无外网依赖，不执行任何外部生产调用。
```

### Plan Contract DOC-0063 <a id="plan-doc-0063"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/proposal.md:1`
- **Source Digest**: `e7d7285a8bbcdbb69ed3dd831cc2bd335840e611c7f656ea50b9c8570ab50569`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前生产环境存在三大核心业务与探针缺陷：
1. 订阅刷新机制具有破坏性：当外部订阅源拉取失败、解析异常、返回空节点或暂时缺失时，系统过早中断并在仓储层执行全局孤儿失活 SQL（`UPDATE nodes SET active = 0 WHERE ... NOT EXISTS node_sources`），导致历史上千节点（含 2026-09-29 被误伤的大量节点）被不当失活，且缺乏同一来源内的节点身份与连接版本（`connection_revision`）安全稳定递增机制；
2. 探针构建器存在协议降级与虚假错误掩盖：`builder.go` 将 transport 名直接填入 Outbound Network 强制设为 TCP，破坏了 UDP 协议能力；Reality 公钥缺少标准 32 字节校验，ShortID（sid）处理缺乏奇数长度拦截；`safe_dialer.go` 在构建失败时丢弃原始原因返回泛化错误，且未能严格分离“本地配置构建成功”与“远端网络握手健康”；
3. 接口缺乏端到端真实业务与语义门禁：过往仅依赖编译通过或空列表 HTTP 200，缺乏对全部前台路由与前端调用点（覆盖 45+ 端点）的副作用、权限、幂等、并发、取消及业务数据闭环验收。
```

### Plan Contract DOC-0064 <a id="plan-doc-0064"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/proposal.md:8`
- **Source Digest**: `e7d7285a8bbcdbb69ed3dd831cc2bd335840e611c7f656ea50b9c8570ab50569`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **订阅节点非破坏性合并与来源保护**：
- 重构 `internal/application/inventory/service.go` 的 Reconcile 流程：订阅网络失败、HTTP 异常、解析报错、返回空列表或局部拒绝（rejected）时，默认保留既有已导入节点与策略边，标记来源为 stale/刷新失败，严禁自动下线或删除既有节点；
- 彻底废除全局无来源节点失活 SQL（`deactivateOrphansSQL`），来源 prune 仅限于当前受管订阅作用域内的归属关系，禁止波及手工导入节点或其他订阅来源的节点；
- 在同一订阅来源作用域内实现安全的节点身份匹配：优先使用源提供的稳定标识，无标识时仅在同一来源内基于名称/协议等唯一无歧义条件匹配更新；存在歧义或重名时保守保留并新增，严禁跨来源按名称合并凭据；
- 维护节点逻辑身份与策略边稳定，连接信息变化时严格单调递增真实的 `connection_revision`；单纯显示名或 JSON 键排序变化不得提升 revision；真连接变化时旧观测保留历史但当前状态置为 unknown/pending，杜绝沿用失效 revision 的虚假 healthy 结论；
- 制定并执行幂等、可回滚、只针对误伤数据的恢复方案与可验证演练脚本，禁止一键粗暴全量激活，不得覆盖用户手工禁用或真实删除状态。
- **探针构建器能力解耦与脱敏真实诊断**：
- 重构 `internal/probe/singbox/builder.go`：严格解耦 L4 网络层（`tcp` / `udp`）与传输层（`ws` / `grpc` / `http` 等），遵循 Sing-box 实际库协议能力，支持 UDP 出站，不再强制单一 TCP；
- 规范 Reality 协议参数校验：对 Reality `pbk`（Public Key）实施严格的 32 字节解码校验与标准 Base64 规范化；对 `sid`（Short ID）严格遵循协议规范，奇数长度或非法十六进制字符直接拒绝并返回明确诊断，严禁静默补零或截断篡改身份；
- 测证底层 Sing-box 库对 `xhttp` 等协议特性的实际构建支持，不凭空假设或报告认定；
- 改造 `internal/application/probe/safe_dialer.go`：捕获底层客户端构建错误并分类，输出脱敏的结构化诊断信息，严禁在日志或响应中泄露密码、UUID、URL query、公钥或私钥等敏感凭据；
- 严格保持 SSRF 防护、私网 IP 拦截及 TLS 证书安全校验边界；将“本地客户端配置构建成功”与“远程链路握手成功”作为独立状态分别判定与上报。
- **全接口业务语义测试与交付门禁**：
- 建立覆盖全部前端调用点与后端路由（包含 auth、settings、subscriptions、nodes、probes、policies、publications、health/ready 等 45+ 路由端点）的端到端语义验收矩阵；
- 实施隔离环境下的全接口语义测试，严格校验请求入参、响应字段、数据库副作用、权限拦截（401/403）、幂等处理、并发控制与撤销操作，杜绝空泛 HTTP 200 判定；
- 确立完整施工门禁与只针对 CSP 受管服务的受控部署预案，包括备份验证、副本演练与独立 Reviewer/Critic 验收闭环。
```

### Plan Contract DOC-0065 <a id="plan-doc-0065"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/proposal.md:29`
- **Source Digest**: `e7d7285a8bbcdbb69ed3dd831cc2bd335840e611c7f656ea50b9c8570ab50569`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `subscription-inventory-preservation`: 订阅节点非破坏性合并、来源作用域隔离、身份与连接版本单调递增管理，以及历史误伤节点可证据化、幂等、可回滚的数据恢复规程。
- `probe-construction-and-truthful-diagnostics`: 探针网络层与传输层协议解耦、UDP 真实支持、Reality 凭证严格校验、客户端构建错误脱敏诊断，以及构建成功与握手健康的精确分离。
- `api-semantic-verification-matrix`: 覆盖全台 45+ 路由与前端交互端点的端到端语义隔离测试、副作用闭环及业务真实验收矩阵。
```

### Plan Contract DOC-0066 <a id="plan-doc-0066"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/proposal.md:35`
- **Source Digest**: `e7d7285a8bbcdbb69ed3dd831cc2bd335840e611c7f656ea50b9c8570ab50569`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。
```

### Plan Contract DOC-0067 <a id="plan-doc-0067"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/proposal.md:39`
- **Source Digest**: `e7d7285a8bbcdbb69ed3dd831cc2bd335840e611c7f656ea50b9c8570ab50569`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **领域与应用服务层**：`internal/application/inventory/service.go`、`internal/domain/id.go`、`internal/application/probe/safe_dialer.go`、`internal/probe/singbox/builder.go`、`internal/probe/singbox/`。
- **持久化与数据迁移**：SQLite 节点与来源关联更新逻辑、历史误伤数据修复迁移/运维脚本。
- **传输层与全接口测试**：`internal/transport/http/` 全部路由处理程序、端到端测试套件 `test/` 与 API 集成测试。
- **Web 前端与黑盒验收**：前端各模块与后端的交互联动、独立审查后 Critic 的真实视口及有数据场景截屏验收。
```

### Plan Contract DOC-0068 <a id="plan-doc-0068"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/proposal.md:1`
- **Source Digest**: `d40fc8557e37d1604e0dedf47a4b8ad9b05e7e8a652337c0663b4df15b2651c5`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

根据用户在实机使用中的明确反馈——**“探针引擎那块的前端需要完全设计，完全不符合人类直觉”**，结合前后端代码与运行态取证，定位到当前探针引擎存在两大类根本问题：
1. **前端信息架构与交互背离人类直觉**：
- 当前 `web/src/features/probes/ProbesView.vue` 将后台异步队列任务对象（`ProbeRun` UUID、`actor_scope`、`config_revision`、`lease_until`、`generation`、`sha256` 证据摘要）直接当成主界面卡片展示，却完全看不到**哪些节点可用、每个节点延迟多少毫秒、流媒体/AI 是否解锁**；
- 发起测速弹窗强迫用户填写内部概念 `config_revision` 与截止分钟数，不支持一键全量测速，也不支持按节点勾选或按筛选结果定点测速；
- 定时测速配置要求用户手填原始秒数（60~604800 秒）并暴露分布式租约代数/Owner 等底层实现噪音；
- 证据抽屉 `ProbeEvidenceSheet.vue` 直接渲染机器日志字符串 `profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=42` 与原始 `node_logical_id` 哈希，没有节点名称与结构化人类可读卡片。
2. **后端节点读模型与探针执行器存在断线与硬编码缺陷**：
- `internal/application/inventory/service.go` 虽然已注入 `s.probeObsRepo`（`cmd/csp/main.go:190`）与 `s.sources`，但在 `ListNodesReadModel` 与 `GetNodeDetailWithRisk`（以及 `internal/transport/http/nodes.go` 的 `get`/`patchConnection`）构建 `NodeView` 时从未调用 `s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil)` 与 `s.sources.ListByNodes(ctx, nodeIDs)`，导致前端获取到的节点列表永远缺失 `latency_ms`、`last_probed_at`、`health_status`、`probe_missing`、`probe_stale`、`capabilities` 与 `sources`，在节点台账与探针工作台均显示为“未探测”；
- `internal/application/probe/runner.go` 在 `nodeIDs` 为空（全量测速）时调用 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true})` 未分页拉取，仅拿到第一页默认 50 个节点；同时 `DefaultRunBudget.MaxTasks = 512` 导致超过 85 个节点的多维测速直接报错 `probe run task budget exceeded` 并将整个 Run 置为 `failed`；且 `executeTaskWithVerdict` 对 `domain.ProbeKindSpeed` 硬编码短路返回 `ErrSpeedProbeOptInRequired`，即便用户显式勾选 `speed` 也无法执行真实测速。
```

### Plan Contract DOC-0069 <a id="plan-doc-0069"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/proposal.md:15`
- **Source Digest**: `d40fc8557e37d1604e0dedf47a4b8ad9b05e7e8a652337c0663b4df15b2651c5`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **打通后端 `inventory.Service` 节点最新探测状态与订阅源读模型（`internal/application/inventory/service.go`、`internal/transport/http/nodes.go`）**：
- 在 `ListNodesReadModel` 与 `GetNodeDetailWithRisk` 构建 `NodeView` 时，批量调用 `s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil)` 与 `s.sources.ListByNodes(ctx, nodeIDs)`；
- 在 `NodeView` 上完整填充 `latency_ms`（优先取最新 `baseline` 观测延迟，若无则取最新有效观测延迟）、`last_probed_at`（最新探测时间）、`health_status`（`healthy` / `degraded` / `unhealthy` / `unknown`）、`probe_missing`、`probe_stale`（超过保鲜阈值 1 小时为 `true`）、`capabilities`（各 `ProbeKind` 的 `{ verdict, latency_ms, observed_at, summary, stale }`）以及 `sources`（节点所属订阅源列表）。
- **修复后端手动探测全量分页、任务预算限制与 `speed` 显式勾选执行（`internal/application/probe/runner.go`）**：
- 当 `nodeIDs` 为空（全量测速）时，按页循环拉取全部活跃节点（而非仅取第一页默认 50 个）；
- 当 `run.Kinds` 中显式包含 `domain.ProbeKindSpeed` 时，视为用户已显式 Opt-In 带宽测速，通过节点代理拨号请求测速端点并传入 `Result{OptIn: true, ...}` 完成带宽评估，不再硬编码短路报错 `ErrSpeedProbeOptInRequired`；
- 放宽单次探测任务预算并支持按并发信号量流式/分批调度全量活跃节点的多维探测任务，不再因 `taskCount > 512` 直接将整个 Run 标为 `failed`。
- **彻底重构探针引擎前端（`web/src/features/probes/**`），打造以“人类直觉”为核心的节点测速与可用性工作台**：
- **第一视觉层级（全景节点健康与延迟统计看板 KPI Summary Bar）**：直观展示「在线可用率」、「平均响应延迟」、「流媒体与 AI 解锁统计」、「定时自动测速状态（含一键开关与策略调整入口）」4 张核心指标卡片；
- **第二视觉层级（一键测速与多维筛选控制栏 Action & Filter Toolbar）**：提供「⚡ 一键全量测速」（无需弹窗填 `config_revision`，顶部展示实时测速进度条）、「🎯 测速已选节点 (N)」、直观可点击切换的「探测维度胶囊开关（连通与延迟必选、流媒体解锁、AI 服务可用性、IP 风险评估、落地地区识别、带宽测速）」，以及关键词、协议、订阅源、健康状态筛选与延迟排序；
- **第三视觉层级（以“节点”为主体的实时测速结果工作台表格 Node Probe Workbench Table）**：展示复选框、节点名称（`display_name`）+ 协议标签 + 服务器地址（`server:port`）、带色彩语义的延迟徽章（绿色 `<100ms`、黄色 `100-250ms`、橙红 `>250ms` 或 `不可达/超时`、灰色 `未测速`）、能力矩阵徽章（流媒体、AI、地区、IP 风险）、人性化最后测速时间，以及行级「⚡ 立即重测」与「查看详情」快捷按钮；
- **人性化定时自动测速配置与后台任务历史折叠区**：定时策略采用预设周期按钮（每 15 分钟 / 30 分钟 / 1 小时 / 6 小时 / 12 小时 / 每天 + 自定义分钟数）与探测项目勾选；后台任务与自动批次历史收敛为可切换次级视图/折叠区，使用人类可读标题（「手动测速任务 / 自动定时批次 · HH:mm:ss」），彻底隐藏底层 UUID、Actor Scope、`config_revision`、Lease 租约与 SHA256 指纹噪音；
- **结构化人类可读的测速详情抽屉（`ProbeEvidenceSheet.vue`）**：顶部展示节点名称、协议、服务器地址与当前综合状态，将 `profile=... verdict=... reason=... status=... latency_ms=...` 解析转换为结构化中文详情卡片，并支持抽屉内一键「重新测速此节点」。
- **节点台账页（`web/src/features/nodes/**`）联动增强**：
- `nodeView.ts` 兼容后端结构化 `capabilities` 对象与顶层 `latency_ms`/`last_probed_at`/`health_status`/`sources`，`NodesView.vue` 直接展示真实最新探测状态、延迟毫秒数，并在节点详情抽屉提供单节点立即测速联动能力。
```

### Plan Contract DOC-0070 <a id="plan-doc-0070"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/proposal.md:35`
- **Source Digest**: `d40fc8557e37d1604e0dedf47a4b8ad9b05e7e8a652337c0663b4df15b2651c5`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `probe-engine-ux`: 探针引擎后端节点探测读模型打通、全量测速与带宽测速执行器修复、以及前端以“节点与人类直觉”为核心的测速与可用性工作台全量重构契约。
```

### Plan Contract DOC-0071 <a id="plan-doc-0071"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/proposal.md:39`
- **Source Digest**: `d40fc8557e37d1604e0dedf47a4b8ad9b05e7e8a652337c0663b4df15b2651c5`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `probe-engine-ux` 能力规格建立完整契约）。
```

### Plan Contract DOC-0072 <a id="plan-doc-0072"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/proposal.md:43`
- **Source Digest**: `d40fc8557e37d1604e0dedf47a4b8ad9b05e7e8a652337c0663b4df15b2651c5`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- 后端：`internal/application/inventory/**`、`internal/application/probe/**`、`internal/transport/http/{nodes,probes}*.go`
- 前端：`web/src/features/probes/**`、`web/src/features/nodes/**`、`web/src/locales/**`
- 验证边界：仅在本地单测与隔离预览环境（临时 SQLite 库、独立端口与临时构建目录）验证，严禁触碰生产容器。
```

### Plan Contract DOC-0073 <a id="plan-doc-0073"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/proposal.md:1`
- **Source Digest**: `b4b1dd6f21cb5c5c37acf09fd40c7019621b1df4c3269cbe9f311affc6f61e2a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

用户反馈节点“全部异常”，并要求探针每分钟更新状态：有状态变化才检测，手动刷新状态时该检测的也要检测。现有观测能证明的是“最近 21 个启用节点 baseline 均记录 error”，而非 21 条线路均不可用：其中 18 条记录为 `credentials_unavailable`（实际有配置凭据；6 条 hysteria2 因不安全 TLS 被安全拒绝，12 条 vless 含 UUID/vision/reality，需校准构建配置），1 条 `private_target_rejected`，另 2 条 `transport_error` 还缺足以断言根因的证据。必须先区分探针能力/安全策略、未知与线路真实失败，再调整巡检，避免把节点误染全红或全绿。
```

### Plan Contract DOC-0074 <a id="plan-doc-0074"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/proposal.md:5`
- **Source Digest**: `b4b1dd6f21cb5c5c37acf09fd40c7019621b1df4c3269cbe9f311affc6f61e2a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- 校准运行时 sing-box VLESS/vision 等探测配置与现有导出语义；安全拒绝、缺失配置、构建不支持及真实传输失败分开归因，保留脱敏证据，不放宽严格 TLS 和公网目标限制。
- 冻结共享状态口径：仅新鲜、可判定的 baseline 给出整体线路健康；baseline 缺失、过期或无法检测应为未知，能力类别结果只表示对应能力，不代替 baseline；节点视图和 pool 计数一致。
- 服务端每分钟**扫描状态**而非全量测速；仅观测缺失/到期、有效连接参数实质改变或活跃切换使对应类别待测；保留现有单轮 512 任务预算、失败 5 分钟冷却、租约和队列去重，防止由观测写入自触发。
- 手动“刷新状态”先重读最新状态，走同一增量候选判定并反馈排队/无待测状态；与已有“全量测速”分开，手动刷新不得推迟服务端正常分钟扫描。
- 不声称 2 条 SS 节点已证实故障；本 Change 不执行生产探针、迁移或部署。是否真正异常待实现后通过受控、获授权的实机复测判定。
```

### Plan Contract DOC-0075 <a id="plan-doc-0075"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/proposal.md:15`
- **Source Digest**: `b4b1dd6f21cb5c5c37acf09fd40c7019621b1df4c3269cbe9f311affc6f61e2a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `probe-evidence-classification`: 安全约束下配置编译一致性与探测失败归因。
- `baseline-health-read-model`: 节点整体健康与探测池聚合的共享基线口径。
- `minute-incremental-state-sweep`: 一分钟服务端状态扫描、配置变化失效及手动状态刷新。
```

### Plan Contract DOC-0076 <a id="plan-doc-0076"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/proposal.md:20`
- **Source Digest**: `b4b1dd6f21cb5c5c37acf09fd40c7019621b1df4c3269cbe9f311affc6f61e2a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无已归档的 `openspec/specs/` 能力；本 Change 与历史未归档的 `csp-incremental-expiry-probe-sweep` 契约的 10 分钟默认值及控制台文字存在明确后继覆盖关系，不修改/复用其历史 Change。
```

### Plan Contract DOC-0081 <a id="plan-doc-0081"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/proposal.md:29`
- **Source Digest**: `30357f95d591623d060274b4f4e7235934230682c86ace3a94f6de0662859c05`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `probe-truth-and-efficiency` 建立完整能力规格契约）。
```

### Plan Contract DOC-0082 <a id="plan-doc-0082"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/proposal.md:32`
- **Source Digest**: `30357f95d591623d060274b4f4e7235934230682c86ace3a94f6de0662859c05`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **特性级正交写集合拆分（互不交叉）**：
- **工作包 1（后端 Probe Runner 与 Profiles 契约及客户端复用）**：`internal/application/probe/**`、`internal/probe/profiles/**`
- **工作包 2（SQLite 最新观测精确查询、复合索引保障与离线基准测试）**：`internal/repository/sqlite/**`
- **不触碰边界**：不修改外部 HTTP API 路由或 JSON 字段结构，不修改前端代码，不删除或弱化 DNS Pinning / SSRF / Redirect / 证据摘要签名字段，仅在本地离线测试夹具中验证，不提交、不部署、不触碰生产环境。
```

### Plan Contract DOC-0083 <a id="plan-doc-0083"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:1`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

本 Change 不能结案。最新独立审计 `VERDICT: REJECTED`：先前 reviewer 在主仓执行 `npm --prefix web run build`，`web/vite.config.ts` 的 `closeBundle` 默认 `fs.cpSync(web/dist, internal/webassets/dist, {recursive:true})`，写入用户未跟踪资产。主仓资产聚合 SHA-256 从已记录的 `bac5a0f424b263f8ad2f538368863200045978f58a0c9d8f9d89f1701866a655` 变为 `6ec421559324992339314b05071a0e44f3fc5c91ee8b3339852ea10953ddf0f6`；`/tmp/csp-critic-stage/original_dist` 有 18 个文件，审计报告原有 17 个资产与备份逐字节相同。当前目录额外包含 `index-CuRBRMN8.js`、`index-CY02MDnG.css`（均不在备份中）；它们可能是其他合法产物，**不能凭文件名直接删除或覆盖**。本次只规划取证与受控恢复，由有授权的施工者执行后再独立核验。
此前 critic 的 37 张原始 PNG 曾证明 Publications 按钮/targets 裁切、Policy/Subscriptions 溢出、Dashboard badge 刺边、Settings `Active Active`、dock 缺 Dashboard；先前 `CRITIC_FLOW_VERDICT: PASSED`、`CRITIC_VISUAL_VERDICT: FAILED`。即使后来有 reviewer 工程 PASS 和 critic FLOW/VISUAL PASSED，其发生时序也不能替代本 Change 要求的“所有特性包完成→同一最终 diff 全量门禁→独立 reviewer→同源隔离 critic”。`/tmp/csp-preview-refresh-20260924/instance.json` 声称隔离快照前后 SHA 一致及构建时主仓资产聚合 SHA 为 `6ec...`，只能证明**该快照在那次构建过程内**一致且未再次触碰主仓，不能证明现在的源代码、嵌入资产、服务进程、截图和最后审查 diff 同源。
原有六协议真实安全探针/race/负例、进程停服、移动页面可用性仍按原规格完成，不得以旧的局部测试、旧视觉 PASS 或 OpenSpec 规划完成替代实施验收。另一 Pi 治理 Change `remediate-phone-test-visual-verdict-gate` 的 reviewer PASS 不等于其 3.1–3.3 完成；用户已说明真机不可用，仅能走有证据的本地替代，本 Change 不修改其任务、不伪造物理验收。
```

### Plan Contract DOC-0084 <a id="plan-doc-0084"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:9`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- 先在授权边界内建立不可变证据：备份、当前未跟踪 `internal/webassets/dist`、构建产物、`index.html` 引用图及 Git/worktree 的路径、哈希、时间和归属。先判定额外两个文件的来源和保留需要（既有用户资产、引用链、构建历史），再提出逐文件恢复/隔离方案；禁止盲删、`reset/restore/clean`、全目录覆盖或操作生产卷。恢复后逐文件比对原有 18 文件（含 index）和任何应保留的新产物，并独立复核工作树未受损。无法确认归属时保留原状并向主脑升级证据/授权，不以 BLOCKED 勾结单。
- 固化安全构建规范：在非主仓隔离 workspace 或显式 `NO_COPY_WEBASSETS=1` 下构建，复验前后主仓资产集合+内容指纹相等；reviewer/critic 必须只读，审查禁止在主仓执行有副作用的 build。若要更改构建钩子，仅在确定性回归夹具证明保护用户产物且不破坏正常嵌入资产时由施工者实现；不为局部问题自造构建框架。
- 将当前业务实现状态拆分为“已有通过记录但最终版无证明”、“确实待实现”、“必须重跑”、“必须依序重新审查/验收”。原 tasks 1.3、2.1/2.1a–c、3.1–3.4、4.1、4.3、4.4 保持未完成；先前 4.2 的旧隔离预览不满足新同源门禁，撤销其完成标记，重新构建独立实例。保留有证据的局部 2.2/2.3 成果，但仍受最终 4.1 重跑与 reviewer/critic 的汇聚门禁约束。
- 复用仓库现有 Go 标准测试、Vue/Vitest、Playwright、Vite 环境开关、sing-box 安全客户端；每一验收记录固定最终源码指纹、源码所生资产、嵌入二进制、服务文件和本轮原图/trace 的可验证关联。禁止从污染前快照推断最新版通过。
```

### Plan Contract DOC-0085 <a id="plan-doc-0085"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:18`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `publication-theme-probe-recovery`: 发布恢复、移动与桌面可用视觉、主题、真实安全探针及停服生命周期、同源隔离预览与独立黑盒验收。
```

### Plan Contract DOC-0086 <a id="plan-doc-0086"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:22`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无。主规格未同步；OpenSpec 严格校验仅表明规划工件有效。
```

### Plan Contract DOC-0087 <a id="plan-doc-0087"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:26`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

规划与验收涉及 `web/vite.config.ts`、`web/dist`、`internal/webassets/dist`、`internal/webassets/embed.go`（如适用）、隔离工作区/预览 manifest、Go/web 测试及前端相关视图与测试。主仓 `internal/webassets/dist` 当前为未跟踪用户资产，施工/审查者必须保护原样直到来源取证和明确授权；不碰生产 17000/18080、生产卷、不部署、不提交。此次策略机只写同一 Change 的现有 OpenSpec 工件。
```

### Plan Contract DOC-0088 <a id="plan-doc-0088"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/proposal.md:1`
- **Source Digest**: `0b7a3f2008751c66cb95f1576cc0d91cee3bfc0efc6de16e6bd1ca9ad6cc3b60`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

生产环境中 9 个订阅源（4 个启用）最近 6 次拉取记录全部未成功（4 次 failed：2 次 TLS handshake timeout、1 次 fetch 30s timeout、1 次 HTTP 404；2 次 partial），且所有订阅的 `fetch_proxy_ref` 与 `user_agent_policy` 均为空，运行容器亦未配置代理环境变量。虽然 HTTP 404 可能是特定服务端对默认 UA 的拒绝（假说，非实测确证），但节点订阅服务器多位于境外，缺乏出站代理是导致握手与拉取超时的关键根因。
同时，用户明确要求采用 Clash Verge Rev（官方 v2.5.6 `src-tauri/src/utils/network.rs:246-255`）的订阅请求头标准（默认 UA `clash-verge/v2.5.6`，`Accept: */*`，`Accept-Encoding: gzip, deflate`）。系统需要建立严谨的全局与订阅级出站代理通道，并在保持严格 SSRF 防护的前提下，实现安全可信的出站代理接入。
```

### Plan Contract DOC-0089 <a id="plan-doc-0089"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/proposal.md:7`
- **Source Digest**: `0b7a3f2008751c66cb95f1576cc0d91cee3bfc0efc6de16e6bd1ca9ad6cc3b60`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **Verge 订阅请求头标准化（Verge Headers & User-Agent Alignment）**：
- 将系统默认抓取 UA 从 `clash-sub-parser/1.0` 调整为官方精确版本 `clash-verge/v2.5.6`。
- 请求头补齐 `Accept: */*`，保持 `Accept-Encoding: gzip, deflate`。
- 保留单订阅级自定义 `user_agent_policy` 优先覆盖机制。
- 维持现有 URL Basic Auth 鉴权及有界安全解压限制，不可回退。
- **可控的全局出站订阅代理机制（Controlled Global & Subscription Outbound Proxy）**：
- 新增全局代理配置入口（环境变量 `CSP_FETCH_PROXY` / 命令行参数 `-fetch-proxy`），通过 `cmd/csp` 注入 `inventory.Service` 与 `fetch.Client`。
- 确定代理选择优先级：单订阅级 `fetch_proxy_ref` 优先 > 全局 `CSP_FETCH_PROXY` > 未配置则直连。
- 更新版本受控的 `docker-compose.example.yml`，增加 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts` 映射。真实 `docker-compose.yml` 维持 `.gitignore` 忽略，通过投产备份核验落地。
- **严格的分离式 SSRF 代理安全模型与应用层目标 IP 强制绑定（Strict Fail-Closed Proxy & Application-Layer Target IP Pinning）**：
- 目标端点与每次重定向保持严格的私网/回环/元数据（169.254.169.254）拦截，目标域名解析后遇私有 IP 立即失败闭合。
- **应用层强制绑定经过公网校验的目标 IP 给代理（禁止依赖上游代理/mihomo ACL）**：
- 经实证，生产环境 mihomo（`/root/clashctl/resources/runtime.yaml`）对私网与回环网段配置为 `DIRECT`，经 7890 代理可穿透访问宿主机 `127.0.0.1:18080`，故**绝对不得依赖 mihomo ACL 作为 SSRF 防线**。
- 客户端在每一跳（初始请求及每次 301/302/303/307/308 重定向）连接代理前，必须在本地完成目标域名 DNS 解析并对全部解析 IP 执行严格公网 SSRF 校验（`ValidateIP`）。
- **HTTP 正向代理目标绑定**：将经过公网校验的 `pinnedIP:port` 写入发送给代理的请求行绝对 URL（`GET http://<pinnedIP>:<port>/path`），同时在 `Host` 请求头中保留原始目标域名（`Host: <original-domain>`），确保代理仅按已验证的公网 IP 建立连接，严禁代理对原始域名进行二次 DNS 解析。
- **HTTPS 代理 CONNECT 目标绑定与 TLS 原域名校验**：向 HTTP 代理发送 `CONNECT <pinnedIP>:<port> HTTP/1.1`（`Host: <pinnedIP>:<port>`），使代理仅见到已验证的公网 IP；隧道建立后，在客户端 TLS 握手中显式设置 `ServerName: <original-domain>`（SNI）并严格校验远端证书与原始域名匹配（严禁开启 `InsecureSkipVerify`）。
- **重定向每跳重验与重绑**：禁用标准库自动隐式跳转，按每跳显式循环重新解析重定向目标、重新执行 SSRF 校验并重新绑定新跳的公网 IP 给代理。
- **代理端点显式 HTTP 协议限定与精确白名单规范化**：
- 先限定仅支持显式 `http://` 代理协议，不支持的 scheme（如 `socks5`、`https`、`ftp` 或省略 scheme）一律失败闭合（`invalid_proxy_url`）。
- `AddAllowedProxy`、`IsProxyHostAllowed` 与 `ValidateProxy` 严格按规范化的完整 `scheme://hostname:port`（即 `http://<canonical-host>:<canonical-port>`）精确匹配，严禁丢弃 scheme 或优先按裸 `host:port` 匹配导致同端口的 `socks5`/`https` 代理获豁免。
- 明确规范化与拒绝规则：拒绝端口省略、拒绝端口前导零（如 `:07890`）、拒绝代理主机名尾随点（如 `host.docker.internal.` 或 `127.0.0.1.`）、拒绝 userinfo 凭据、拒绝 IPv6 Zone ID（`%`）、拒绝混淆 IP（八进制/十六进制/整型/短 IP/IPv4-mapped IPv6），IPv6 地址统一按 RFC 5952 规范化为带方括号格式 `http://[<canonical-ipv6>]:<port>`。
- 任何未在白名单内的自定义 `fetch_proxy_ref` 端点绝不豁免，必须通过完整公网 SSRF 校验并同样绑定校验后的代理公网 IP，严禁指向私网、回环、网关私有 IP 或云元数据。
- **如实说明路由影响与 TOCTOU 边界**：
- 由于向代理传递的是已校验公网 IP（CONNECT authority 与 HTTP 请求行均为 IP），若上游代理配置了依赖请求行/CONNECT 域名的路由分流规则（而非基于 IP/GeoIP 或 TLS SNI 嗅探），其分流路线可能改变（例如走 IP/FINAL 规则而非 DOMAIN 规则）；系统在规格、设计与测试中如实说明并验证该行为，绝不以此为由关闭证书校验或跳过应用层 IP 绑定。
- 应用层 IP 绑定彻底消除了代理侧二次 DNS 解析导致的代理侧 DNS Rebinding 与内网穿透，但系统不虚假宣称消灭互联网公网 IP 层面的极端路由重分配 TOCTOU。
- **调度一致性与可观测性（Scheduler Consistency & Failure Audit）**：
- 手动刷新与后台定时调度统一走相同的代理选择与安全判定流水线。
- 代理连接失败、超时或安全拒绝统一进行结构化分类与安全脱敏审计。
```

### Plan Contract DOC-0090 <a id="plan-doc-0090"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/proposal.md:40`
- **Source Digest**: `0b7a3f2008751c66cb95f1576cc0d91cee3bfc0efc6de16e6bd1ca9ad6cc3b60`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `subscription-proxy-and-verge-headers`: Verge 订阅请求头标准化、全局与订阅级出站代理分级解析、受控宿主代理接入、以及严格的分离式代理 SSRF 安全模型。
```

### Plan Contract DOC-0091 <a id="plan-doc-0091"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/proposal.md:44`
- **Source Digest**: `0b7a3f2008751c66cb95f1576cc0d91cee3bfc0efc6de16e6bd1ca9ad6cc3b60`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（`openspec/specs/` 下无对应已归档主规格，本次通过新增能力规格建立权威契约）。
```

### Plan Contract DOC-0092 <a id="plan-doc-0092"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/proposal.md:48`
- **Source Digest**: `0b7a3f2008751c66cb95f1576cc0d91cee3bfc0efc6de16e6bd1ca9ad6cc3b60`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- `internal/fetch/`（`fetch.go`, `policy.go`, `fetch_test.go`）：默认 UA、请求头、严格代理准入校验规则、混淆 IP 防御与安全策略。
- `internal/application/inventory/`（`service.go`, `service_test.go`）：支持默认代理注入与优先级解析。
- `cmd/csp/`（`main.go`）：解析 `CSP_FETCH_PROXY` 与 `-fetch-proxy`，装配 `fetchPolicy`（仅注入规范化 `host:port`）、`fetchClient` 与 `inventory.Service`。
- `docker-compose.example.yml`：增加 `CSP_FETCH_PROXY` 默认环境变量示例与 `extra_hosts`。
- 本地自动化测试（单测与 httptest 隔离夹具）：覆盖请求头、代理优先级、SSRF 拦截、端口与混淆 IP 过滤、重定向安全及失败审计。
```

### Plan Contract DOC-0096 <a id="plan-doc-0096"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/proposal.md:27`
- **Source Digest**: `6bec759645a452410b81a76e94b4830d05fe72a32f46096273eca1af7bcecfaf`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `topology-i18n-export` 能力规格建立完整契约）。
```

### Plan Contract DOC-0097 <a id="plan-doc-0097"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/proposal.md:31`
- **Source Digest**: `6bec759645a452410b81a76e94b4830d05fe72a32f46096273eca1af7bcecfaf`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`internal/transport/http/policies*.go`、`internal/compiler/{compiler,mihomo,singbox,surge,qx}*.go`、`internal/compiler/testdata/golden/*.golden`、`internal/application/publication/{service,preflight_test,service_test}.go`、`internal/integration/*.go` 及 `web/src/**`。仅在隔离验证环境构建与测试，不直接操作生产运行容器。
```

### Plan Contract DOC-0098 <a id="plan-doc-0098"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:1`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前用户明确要求实际修复 CSP 管理界面故障，而旧合同将 ADB/USB 真机连接设为前置门槛，已被废除。已报告 `POST /api/v1/publications/preview` 返回 500、模板生成边界不清、`web/src/locales/messages.ts` 缺失 130+ 中英文键、异常状态缺乏可操作重试以及 PolicyView 拓扑与抽屉不联动。需要保留已经完成的隔离取证工作，立即把剩余施工和复验转向这些可验证缺陷。
```

### Plan Contract DOC-0099 <a id="plan-doc-0099"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:5`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- 修复发布预览 500 并明确模板生成的合法输入、无效输入及错误响应边界；失败不可伪装为成功，服务端与前端按同一接口契约联调。
- 补齐管理界面实际使用的 i18n 键的中英文翻译，七个入口不显示裸键；异常/空状态给出明确恢复或重试动作。
- 修复 PolicyView 拓扑选中、抽屉编辑和数据刷新之间的状态联动，触控下保持可操作。
- 使用唯一授权的设备执行路径 `ssh xiaomi-phone "/data/local/einck/script/ubuntu_chroot.sh run '<cmd>'"`，在 Ubuntu chroot ARM64 的 `/root/.cache/ms-playwright` Chromium Playwright 做浏览器验收；headless 证据只称设备上 headless Chromium，不称实屏或物理触控验收。覆盖移动 392×872、375×667 与桌面 1280×800。
- 保护已有 726 项工作区变更；仅隔离实例/临时数据可用于写测试，生产 18080/17000 和数据卷禁止写入；不授权部署、重启或清场。
```

### Plan Contract DOC-0100 <a id="plan-doc-0100"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:15`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `admin-ui-device-quality`: CSP 管理界面的发布预览、国际化、失败恢复、策略联动及跨视口浏览器验收契约。
```

### Plan Contract DOC-0101 <a id="plan-doc-0101"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:19`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。与其他进行中 Change 的 API/鉴权契约冲突时先核对归属，不擅自更改其工件。
```

### Plan Contract DOC-0102 <a id="plan-doc-0102"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:23`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- 预期后端发布预览/模板生成路径及相关契约测试、`web/src/locales/messages.ts`、异常状态组件与 PolicyView，以及定向 Playwright/单元或集成回归。具体文件归属由施工者先查现有实现确定，重叠接口串行处理。
- 已完成的基线、隔离服务与历史故障复核四项保留；它们不是当前修复已经完成的证明。
- 不对共享服务、数据卷或现有构建产物进行测试写入；非本次授权的实屏、ADB、USB 或设备唤屏操作不纳入任务。
```

### Plan Contract DOC-0105 <a id="plan-doc-0105"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:14`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `admin-ui-adaptive-layout`: 界面响应式排布、语义交互可达性、设计原语复用及跨视口实证验收。
```

### Plan Contract DOC-0106 <a id="plan-doc-0106"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:17`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。`openspec/specs/` 目前没有已有主规格；进行中的旧 Change 是另一个尚未合并的 delta，不当作主规格直接改写。
```

### Plan Contract DOC-0107 <a id="plan-doc-0107"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:20`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

主要涉及 `web/src/style.css`、`web/tailwind.config.js`、`web/src/ui/ResponsiveGrid.vue` 与弹层组件、`web/src/App.vue`、引用方和定向 UI/Playwright 测试。无业务 API、服务端算法或数据迁移；不改部署与生产数据。工作区基线 HEAD `7f827e2`，初始记录 `git status --short` 726 条；`web/src/` 当前是未跟踪目录，实施时必须保留其中所有用户既有工作。规划不授权实施、生产写入或设备实屏操作。
```

### Plan Contract DOC-0108 <a id="plan-doc-0108"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/proposal.md:1`
- **Source Digest**: `bf00aaa136aad994071b0ffc5fc166c1dfddb9ae26767a969fdcf7400ce6185a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

CSP 受控更新及合法带身份的真实业务验收仍未交付。2026-10-02 本轮只读取证确认 HEAD=`e0431767845ffe3cd2bff8a7e415abf085814ac7`、起始工作树 clean，运行容器 healthy，但健康 200、历史 fixture PASS 和无凭据安全暂停均不能代替正文审计、订阅刷新和真实握手。旧 tasks 3.1 的完成勾选不符合已执行事实，必须撤销；本轮只规划，不实施、构建、部署、刷新、恢复或提交。
历史部署报告把当前镜像绑定到 `6a008f7-dirty` 及31文件 manifest，运行二进制具有后来库存保全提交的 SQL 特征。`95ce0a9..e043176` 的服务业务源码、前端及迁移未变化，仅恢复 CLI 所用库变化；不能声称订阅失败保护尚未上线。需要的是可核验源码/产物归属的受控发布和真实验收，而非重新实现既有保护。
现有验收 CLI 存在确定性的假 PASS 风险：正文断言未汇聚，错误/空探针可最终 PASS，探针请求字段与观察端点错配，分页计数不完整、库存保护只比计数且网络错误直接假定保留，报告写入错误被忽略。runner 写固定公共 `/tmp` 路径。必须先整改和独立审查再使用于生产。
```

### Plan Contract DOC-0109 <a id="plan-doc-0109"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/proposal.md:9`
- **Source Digest**: `bf00aaa136aad994071b0ffc5fc166c1dfddb9ae26767a969fdcf7400ce6185a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- 继续同一 repo-local Change，冻结完整特性包 `pkg_csp_release_and_acceptance`，串行完成施工自测→独立审查→授权 app 发布验证→合法真实验收→独立证据复验。
- 复用 Go 标准 HTTP/context/json/testing/httptest、已有 SQLite/领域类型/脱敏函数及 e2e harness；修复验收工具的真实协议、fail-closed 结论、身份/分页/库存集合核验、私有 scratch 和报告错误处理，不另造控制框架。
- fresh 一致性备份 DB/库存/身份配置、当前镜像及 Compose/环境；验证可恢复性后只替换目标 app。保留 `csp-v1-data`、双 loopback 端口、代理及现有 DB verifier，不改 gateway、其他服务、Hermes 内核或模型路由。
- 合法凭据只来自已存在受管来源；缺凭据时真实验收保持未完成，可继续本地整改，不改哈希、不降级鉴权、不聊天索要秘密。
- 真实验收仅既有订阅/节点，限定预算，失败保护库存；历史恢复仅因果充分且独立审查的精确白名单，允许真实计数0但不掩盖待证明事项。
- 使用已冻结的本轮原生 run 合同和真实子会话报告闭环；保持历史 UNVERIFIED 原样，不倒灌合同。不为现有已修复的报告识别再扩大跨根源码修改。
```

### Plan Contract DOC-0110 <a id="plan-doc-0110"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/proposal.md:20`
- **Source Digest**: `bf00aaa136aad994071b0ffc5fc166c1dfddb9ae26767a969fdcf7400ce6185a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `live-acceptance-and-qualified-recovery`: 真实带身份正文验收、有界实网、因果充分恢复及可回滚发布。
```

### Plan Contract DOC-0111 <a id="plan-doc-0111"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/proposal.md:27`
- **Source Digest**: `bf00aaa136aad994071b0ffc5fc166c1dfddb9ae26767a969fdcf7400ce6185a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

限定 CSP 的 `cmd/csp-live-acceptance/main.go`、runner、对应 CLI/e2e 测试和两份运行手册；恢复实现只在新证据揭示具体缺口时定点修复。生产只涉及已授权目标 app。Pi `/home/service/pi` 及外置 bridge 本轮只读取证，不写；CLI 确认 bridge 所属 OpenSpec 根是 `/home/service/hermes/hermes-home`，并非独立子目录根。本轮无 UI 变更，不派无 URL critic。规划 READY 不等于业务 PASS。
```

### Plan Contract DOC-0112 <a id="plan-doc-0112"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/proposal.md:1`
- **Source Digest**: `bd5b9c2fe370c76ed49e19cfa779d564cc02de63868233738edfc70c47fa5562`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前 CSP 系统在节点来源归属（Provenance & Source Attribution）上存在历史断链与静默丢失问题：
1. **历史导入来源事实缺失**：在最初 Go 重写期的 `legacy.import` 批处理中，实现仅抽取了 `nodes` 与 `subscriptions` 表，遗漏了读取和持久化旧系统的 `node_source_links` 与 `source_revisions`。导致 960 个合法历史节点初始入库时 `node_sources` 关联记录即为 0，随后在 2026-09-29 的刷新中被旧全局孤儿清理逻辑误失活为 `active = 0`；
2. **动态刷新下线证据抹除**：现行订阅刷新（Reconciliation）在对比上游增量时，直接以 `DELETE FROM node_sources` 物理删除下线节点的关联关系，导致 23 个曾有真实来源的节点（如 `einck-qzz` 下线节点）彻底失去历史凭据，沦为无源孤儿；
3. **缺乏独立历史归属台账**：系统仅维护表征“当前实时订阅成员”的 `node_sources` 表（当前仅 31 个节点有效），若为了恢复历史事实而直接向 `node_sources` 回填或放活节点，将严重污染当前对外导出、发布及探针调度的作用域 E（Scope E = 31），破坏不可变业务基线；
4. **端点去重碰撞凭据风险**：旧导入基于 `(proto, server, port)` 去重，导致少量同一端点但不同凭据的记录被丢弃。若不加甄别地将丢弃记录关联至当前节点，将引入凭据污染。
必须以最小侵入模型建立独立的不可变历史来源台账（`node_source_history`），在绝对不触碰当前库存状态 E、不伪造新抓取、不修改节点连接参数的前提下，闭环固化 983 个孤儿节点的历史因果事实，修复刷新写链中的证据丢失漏洞，并提供只读审计接口与受控演练工具。
```

### Plan Contract DOC-0113 <a id="plan-doc-0113"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/proposal.md:11`
- **Source Digest**: `bd5b9c2fe370c76ed49e19cfa779d564cc02de63868233738edfc70c47fa5562`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **增量引入不可变历史账本表 (`node_source_history`)**：
- 新增 Migration `000016_node_source_history.sql`，独立于实时 `node_sources`；
- 核心字段：`id`, `node_logical_id` (FK nodes), `subscription_id` (Nullable FK subscriptions ON DELETE SET NULL), `source_identity` (稳定命名空间+历史源ID/映射), `source_label` (脱敏展示名称), `connection_revision` (Nullable), `relation_state` (`verified|conflict|unknown`), `cause` (`legacy_import|refresh_removed|subscription_deleted|manual_confirmed|unresolved`), `first_observed_at`, `last_observed_at`, `evidence_kind`, `evidence_json` (严格杜绝密码、UUID、Token 及带鉴权 query 的 raw URL)；
- 联合唯一约束基于 `(node_logical_id, source_identity, evidence_kind, first_observed_at)` 防重，支持同一节点多次历史观测及结构化 `source_unknown` 哨兵记录。
- **修复运行时写链丢失漏洞**：
- **刷新下线保护**：成功刷新在删除 `node_sources` 成员前，在同一事务中将既有关系的真实性、快照标签、`fetch_id` 及当前 `connection_revision` 归档至 `node_source_history`（`cause = 'refresh_removed'`）；
- **抓取失败保护**：刷新失败时保持既有状态，绝不物理删除亦不伪造历史归档；
- **订阅启用/停用隔离**：切换订阅状态不解绑 `node_sources`，不篡改节点 `active`，不触发历史虚假归档；
- **订阅删除级联解耦**：删除订阅前先对现有关联建立脱敏快照（`cause = 'subscription_deleted'`, `source_label` 固化），随后物理外键级联将 `subscription_id` 置 NULL，明确区分“历史源已删除”与“未知无源”；
- **CoW 与用户覆写隔离**：用户在节点上的字段级人工修改（`node_overrides`）仅记录节点参数变更，历史来源事实按实际节点血统继承，不将用户自定参数归咎于上游。
- **提供离线受控历史回填维护命令 (`csp recover-source-history`)**：
- 提供单次可重入 CLI 工具，只读解析带 SHA-256 校验的脱敏清单（`recovery-manifest.json`）；
- 严格仅向 `node_source_history` 插入不可变证据记录，绝对禁止触碰 `nodes.active`、`nodes.connection_revision` 或写入 `node_sources`；
- 支持 `--dry-run` 预检与原子事务；前置与后置强制核验生产库存不变量（1014 总资产、31 活跃、Scope E 不变）。
- **新增只读节点来源与历史审计 API**：
- 增加 `GET /api/v1/nodes/{id}/source-history`，返回当前源列表、历史观测事实列表与聚合的 `attribution_status` (`current|historical_verified|manual_confirmed|conflict|unknown`)；
- 在节点详情侧栏/抽屉（Drawer）中以只读方式展示历史归属事实与证据摘要，主 UI 保持当前资产默认视图，不开设全局历史管理平台或 URL 猜测输入。
```

### Plan Contract DOC-0114 <a id="plan-doc-0114"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/proposal.md:33`
- **Source Digest**: `bd5b9c2fe370c76ed49e19cfa779d564cc02de63868233738edfc70c47fa5562`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `inventory/node-source-history-ledger`: 建立独立增量表 `node_source_history`，保存节点历史来源事实与证据链，解耦实时订阅成员（Scope E）与历史因果记录。
- `inventory/source-history-write-chain`: 改造订阅刷新、下线剪枝、订阅删除与手动操作生命周期，确保来源事实在变更发生时原子快照归档，彻底消除物理误删导致的断链。
- `inventory/source-history-recovery-cli`: 实现 `csp recover-source-history` 工具，基于不可变脱敏清单幂等补齐存量 983 个历史孤儿节点证据，内置严格资产不变量门禁。
- `inventory/node-source-history-api-view`: 暴露只读历史审计端点并在前端节点抽屉中安全展示来源归属与历史状态，严格屏蔽一切凭据与敏感 URL。
```

### Plan Contract DOC-0115 <a id="plan-doc-0115"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/proposal.md:41`
- **Source Digest**: `bd5b9c2fe370c76ed49e19cfa779d564cc02de63868233738edfc70c47fa5562`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **数据库层**：增量新增 Migration `000016_node_source_history.sql`，无破坏性 DDL，不影响既有 27 张表任何结构。
- **后端服务**：
- `internal/domain/`: 增加 `NodeSourceHistory` 实体与枚举定义；
- `internal/repository/sqlite/`: 增加 `node_source_history` 的存储与事务查询接口；
- `internal/application/inventory/`: 在刷新剪枝与删除逻辑中嵌入历史快照归档事务；
- `internal/transport/http/`: 增加 `/api/v1/nodes/{id}/source-history` 路由处理函数；
- `cmd/csp/`: 增加 `recover-source-history` 子命令。
- **前端页面**：`web/src/views/` 节点详情 Drawer 增加只读来源历史选项卡/面板。
- **运行时环境**：零停机无损热更新，不影响正在运行的探针调度、发布编译及已有 31 个在线节点服务。
```

### Plan Contract DOC-0126 <a id="plan-doc-0126"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/proposal.md:1`
- **Source Digest**: `21a3be9b7b38b4e16f5747924fc2d04a420351f1c49a7af64a229af24d88123f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前 CSP 节点探针子系统存在严重的人为过度设计与臆造阻断（如无条件拦截自签名证书 `skip-cert-verify`、强行禁止 Hysteria2 端口跳跃与 TUIC `disable_sni`、将流媒体和 AI 探测硬编码为 `contract_drift` 假存活、将并发硬编码限制在 16~32）。用户明确要求彻底重做节点探针，全面对齐工业界成熟开源实践 `beck-8/subs-check`（主基准）与 `sinspired/subs-check-pro`（辅助调度与生命周期参考），移除主观安全门禁，恢复真实节点质量探测能力。
```

### Plan Contract DOC-0127 <a id="plan-doc-0127"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/proposal.md:5`
- **Source Digest**: `21a3be9b7b38b4e16f5747924fc2d04a420351f1c49a7af64a229af24d88123f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### What Changes

- **核心运行时替换**：将探针底层拨号从被替代的 Sing-box 内存包装与 SafeDialer 切换为成熟的进程内 Mihomo `adapter.ParseProxy` 方案，以内核真实语义支持全量协议，完整保留连接参数，正常支持 `skip-cert-verify`、Hysteria2 `ports`、TUIC `disable_sni`，消除私网/本地 DNS 截断阻断，目标域名交由远端代理代理解析，严格设置 `Proxy: nil` 杜绝宿主代理泄漏。
- **三阶段流水线重构**：实施 Alive (测活) -> Media/AI (流媒体与 AI) -> Speed (可选受控测速) 阶段漏斗；第一阶段失败立即终止，不再无差别执行昂贵探测；支持阶段性动态并发池与 `watchdog` / `cancel` 生命周期可靠回收。
- **真实能力检测落地**：废除 `contract_drift` 占位，全面移植 beck-8/subs-check 真实平台检测算法（OpenAI 网页/移动端双轨 + 地区、Netflix 全解锁/自制剧/封禁 + 地区、YouTube 地区码与送中过滤、Disney+ 解锁/封禁/上线中、Claude trace 地区与封禁过滤、Scamalytics IP 风险分与纯净度、Bounded Speedtest 带宽测速）。
- **数据库、API 与 WebUI 结果透传**：扩展/适配观测记录与节点 DTO，将各平台的细粒度状态（解锁/部分可用/封禁/不支持/超时/网络错误）如实存储并经 `/probes/*` 与 `/nodes/*` API 输出到 WebUI 监控工作台，消除“未检测”视差与虚假全绿。
- **移除孤立旧代码**：清理已被完全替代的探针旧拨号与过度规则，不影响其他依然使用 Singbox 的导出编译业务，保留管理鉴权、任务审计、定时触发与订阅库存。
```

### Plan Contract DOC-0129 <a id="plan-doc-0129"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/proposal.md:23`
- **Source Digest**: `21a3be9b7b38b4e16f5747924fc2d04a420351f1c49a7af64a229af24d88123f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

- **核心依赖**：引入 `github.com/metacubex/mihomo v1.19.31` 用于探针出站拨号；探针运行时不再依赖 sing-box 拨号逻辑（原有订阅编译与导出功能继续保留 sing-box）。
- **内部包**：重构 `internal/application/probe/` 与 `internal/probe/` 目录，移除 SafeDialer 和过重调度限制。
- **API 与数据模型**：扩展 `internal/domain/probe.go` 观测结果与 DTO，补充平台结构化结果（如 `openai`, `netflix`, `youtube`, `disney`, `claude`, `speed` 等）。
- **前端工作台**：更新 `web/src/features/probes/` 与 `web/src/features/nodes/`，以直观标签和状态指示器展示各平台真实检测状态。
- **协议与许可**：参考实现源自 GPL-3.0 项目，严格保留原作者版权署名并在项目中维护对应 NOTICE。
```

### Plan Contract DOC-0130 <a id="plan-doc-0130"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:1`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md` 与 `specs/audit-and-defect-remediation/spec.md`。
在当前 CSP 1.0 代码库（分支 `dev` 基线 `fe4654b`）中，系统由 Go 1.24 后端与 Vue 3 / Vite 前端构成，静态资源通过 Go `embed` 打包为单一可执行文件。
本次变更源于三轮独立代码走查和运行时审计，重点解决影响正常生命周期使用、并发稳定性与端到端质检的真实缺陷。
特别约束：
1. 宿主机 17000 端口为线上 Docker 服务的映射端口别名（生产端口为 18080 与 17000），任何测试与预览严禁向该端口发起网络请求或写入测试数据；
2. 保持严格安全不变量：SSR 保护、公网 IP 校验、私网隔离拦截、发布 Token 鉴权以及敏感凭据脱敏均不可降级；
3. 不进行未经论证的无端重构或投机性抽象（如非必要的 N+1 复杂缓存层、无调用的多余库等）；
4. 排除追踪的已生成静态产物（`internal/webassets/dist`）的非必要修改，直至最终门禁统一打包。
```

### Plan Contract DOC-0131 <a id="plan-doc-0131"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:12`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 在后端服务启动时自动引导初始活跃配置修订（Configuration Revision），彻底消除冷启动下添加规则报 422 `missing_revision_id` 与发布预览报 409 `no_active_revision` 的死锁问题；
- 开放规则删除 REST 端点及前端集成，补齐策略管理生命周期闭环；
- 解决 `safeRuntimeConn` 在阻塞 Read 时执行 Close 出现的互斥锁死锁；
- 精准化拨号器错误归类：DNS 失败与私网目标拒绝仅作为单节点不可用证据，不再使整个探测批次标记为失败；
- 修复节点观测历史接口的分页 SQL 逻辑与真实总数计数；
- 订阅删除时原子级联停用无所属来源的孤儿节点；
- 修复 PATCH 订阅空白凭据引用的传输层静默绕过缺陷；
- 前端补齐 Cookie 会话下的 CSRF 请求头投递、局域网 HTTP 下的 UUID 退避生成算法；
- 消除前端 Probes 视图高频扫描风暴与 Nodes 视图无限滚动饥饿；
- 修复抽屉校验绕过、策略路径编码与发布链接刷新丢失 Token 缺陷；
- 修正 `.env.example` 遗留 Python 变量，对齐 Go CSP 1.0 环境变量；
- 修复 Playwright E2E 真实服务器测试中文环境语言断言冲突，巩固 17000 生产端口隔离纪律。
**Non-Goals:**
- 不重写或颠覆既有的策略图拓扑校验引擎与求解算法；
- 不引入重型外部 Redis 或分布式队列，维持单进程本地 SQLite 存储；
- 不进行无关视图的激进 UI/UX 重设计；
- 不放宽私网地址拦截规则或绕过 SSRF 防御体系。
```

### Plan Contract DOC-0132 <a id="plan-doc-0132"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:36`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 配置修订引导与规则自动归属方案

- **决策**：
- 在 `cmd/csp/main.go` 启动流水线中，初始化 `policyService` 与 `revisionService` 后，调用 `policyService.EnsureActiveRevision(startupCtx)`。若当前数据库无活跃修订且修订总数为 0，则自动以当前有效（默认空白）策略图状态计算 digest 并创建激活一条初始修订；
- 在 `internal/application/policy/service.go` 的 `CreateAdmissionRule` 与 `CreatePolicyRule` 中，若客户端未传递 `revision_id`，优先尝试 `GetActive`；若此时仍无活跃修订，则在同一事务锁内自动引导并绑定该初始修订；
- 在 `internal/transport/http/revisions.go` 注册 `POST /api/v1/revisions`，支持显式生成新草稿/活跃修订。
- **替代方案考虑**：
- *仅在前端强制要求用户手动点击“新建版本”*：用户体验极差，新手在首次部署后看到的是莫名其妙的 422 报错，违背零配置可用原则；
- *运行时彻底丢弃 revision 表*：违背 CSP 1.0 配置幂等性与不可变版本快照设计初衷。
```

### Plan Contract DOC-0133 <a id="plan-doc-0133"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:45`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. `safeRuntimeConn` 关闭防死锁设计

- **决策**：
- 传统 `net.Conn` 在另一个协程阻塞于 `Read()` 时调用 `Close()`，正常情况下底层系统调用套接字关闭会使阻塞的 `Read()` 立即返回 `net.ErrClosed`。但在 sing-box 的分层封装结构中，`safeRuntimeConn` 内部使用 `readCloseMu` 将 `Read` 与 `Close` 强行互斥：
```go
func (c *safeRuntimeConn) Read(p []byte) (int, error) {
c.readCloseMu.Lock()
defer c.readCloseMu.Unlock()
return c.Conn.Read(p)
}
```
当 `c.Conn.Read()` 阻塞等待网络数据时，`readCloseMu` 被长期持有，此时外界调用 `Close()` 试图获取 `readCloseMu.Lock()` 即产生死锁。
- **解决方案**：
- 在 `Close()` 执行加锁之前，先主动调用 `c.Conn.SetDeadline(time.Now())`（以及递归遍历所有 `upstreamProvider` 收集的底座网络连接并设置立即超时），强制唤醒并退出阻塞中的 `Read()` 调用栈；
- 遍历并优先关闭最底层真实物理套接字，使应用层解密/解协议 reader 迅速退出并释放 `readCloseMu`；
- 若连接未嵌套额外协议层（`len(upstreams) == 0`），则直接调用底层 `c.Conn.Close()`，无需在 `readCloseMu` 上无谓排队。
- **替代方案考虑**：
- *完全移除互斥锁*：部分 sing-box AEAD 协议栈在并发 Read 与 Close 时可能存在 data race；使用“先超时解阻塞 + 再加锁释放协议资源”能兼顾无死锁与无竞态。
```

### Plan Contract DOC-0134 <a id="plan-doc-0134"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:63`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0135 <a id="plan-doc-0135"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:71`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 订阅删除时孤儿节点去活机制

- **决策**：
- 在 `internal/repository/sqlite/subscriptions.go` 的 `Delete` 方法中，使用原子事务：
1. `DELETE FROM subscriptions WHERE id = ?;`（外键级联清理 `node_sources`）；
2. `UPDATE nodes SET active = 0, updated_at = ? WHERE active = 1 AND NOT EXISTS (SELECT 1 FROM node_sources WHERE node_sources.node_logical_id = nodes.logical_id);`
- 确保失去全部订阅来源的孤儿节点立即标记为 `active = 0`，与 `inventory.Service.Reconcile` 的孤儿节点去活语义保持 100% 一致。
- **替代方案考虑**：
- *物理硬删除节点*：可能破坏已有关联的历史观测外键记录或审计痕迹；采用软去活（`active = 0`）最安全且保留审计证据链。
```

### Plan Contract DOC-0136 <a id="plan-doc-0136"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:80`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 前端 CSRF 守卫、UUID 兼容与视口防饥饿设计

- **决策**：
- **CSRF 注入**：`ApiClient` 维护内存中的 `csrfToken` 状态，`useAuth.login` 在成功获取响应时提取并调用 `api.setCsrfToken(res.csrf_token)`，在发起 POST/PUT/PATCH/DELETE 等请求时统一注入 `X-CSRF-Token` 请求头；
- **UUID 退避**：封装通用 `generateUUID()` 工具函数，优先使用 `crypto.randomUUID()`，若检测到未定义（纯 HTTP 局域网）则平滑退避至符合 RFC 4122 v4 标准的伪随机算法；
- **轮询风暴规避**：`ProbesView.vue` 轮询定时器仅对活跃状态（`runs`、`poolStatus`、`observations`）做必要刷新；全量节点拉取仅在首次挂载、任务状态由 running 变为结束或手动点击刷新时执行；
- **视口无限滚动防饥饿**：在 `NodesView.vue` 中，当应用了客户端健康度过滤且 `filteredItems.length` 不足填满可视区域高度（如低于 10 个且 `hasMore.value` 为真）时，通过侦听或加载回调自动触发下一次 `loadMore()`，直到视口被有效填满或已无更多后台数据。
- **替代方案考虑**：
- *完全依赖服务端过滤健康度*：健康度是基于探针观测计算的动态视图，重写服务端节点接口过滤改动面过大；客户端视口防饥饿逻辑简单高内聚。
```

### Plan Contract DOC-0137 <a id="plan-doc-0137"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:89`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 6. 运维配置与端口隔离纪律

- **决策**：
- 更新 `.env.example`，将过时的 `CLASH_*` 全面重写为 `CSP_*`，并提供 `CSP_ADMIN_TOKEN`、`CSP_PORT`、`CSP_FETCH_PROXY` 等官方支持字段的详细注释；
- 在工程规范与测试用例中明确：`17000` 端口为宿主机生产映射别名，测试一律使用系统动态端口（如 `127.0.0.1:0`）及独立隔离临时数据库。
- `web/e2e/auth-real-server.spec.ts` 在页面加载前注入 `localStorage.setItem('csp_locale', 'en-US')`，保证英文断言环境一致性。
```

### Plan Contract DOC-0138 <a id="plan-doc-0138"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:95`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 7. 复审整改：发布异步隔离与修订原子激活

- **发布响应**：用户选择目标是唯一选择来源。网络异步结果按请求目标持久化；仅当请求目标仍是当前目标时才改变可见发布/预览、错误状态。不在成功回调中反向改变选择，切回时读取该目标持久记录；请求代际防止同目标旧响应盖过新结果。
- **修订创建**：`policy.Service` 的首次引导和显式 `StateActive` 共用 `RevisionRepository.CreateActive` 原子接口。SQLite 仓储在同一个事务中插入草稿、归档旧活跃版本并激活新版本；任一步出错整体回滚（包含旧活跃版本）。启动仅在无任何修订时引导，用户已有草稿不自动激活，原有规则数据不做删除/重建；`revision.Service` 显式生命周期保持不变。用 SQLite 临时触发器在激活 SQL 注入单次故障，重建 service 后去除故障并重试，检验无残留草稿、规则未丢失与活跃版本正确。
```

### Plan Contract DOC-0139 <a id="plan-doc-0139"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:99`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk] 启动自动引导初始修订是否会影响既有包含数据的环境？**
- → 缓释措施：仅当数据库查询活跃修订为空且修订总记录为 0 时才触发初始引导。已有数据或已有历史修订的环境绝不重复触发，保证纯净幂等。
- **[Risk] safeRuntimeConn 递归关闭底层套接字是否会导致多次关闭报错？**
- → 缓释措施：使用 `sync.Once` 保护各底层连接的释放，各层关闭错误仅记录不向外扩散致命 panic。
- **[Risk] 前端自动连续加载分页是否会导致超大数据库卡顿？**
- → 缓释措施：设置防饥饿连续加载的最大保护阈值（如单次连续请求最多自动加载 3 页），若仍无匹配数据则显示“继续加载更多”按钮，保障主线程响应速度。
```

### Plan Contract DOC-0140 <a id="plan-doc-0140"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:1`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

当前 CSP 1.0 单二进制控制面在线上运行环境下发生 100% 鉴权死锁故障，导致 `https://sub.einck.top` 前端访问所有功能视图时均提示红色错误 `Authentication credentials required`。
深入实机配置与源码调用链审计，定位出三重叠加缺陷：
1. **后端死锁逻辑**（`internal/transport/http/middleware_auth.go:29`）：`AdminAuthMiddleware` 的 Bearer 放行条件为 `cfg.AdminToken != "" && token == cfg.AdminToken`，而针对无凭据请求则在第 70 行无条件阻断并返回 `401 Unauthorized`。
2. **生产配置现状**（`docker-compose.yml:13` 与 `cmd/csp/main.go:224`）：`CSP_ADMIN_TOKEN: ${CSP_ADMIN_TOKEN:-}` 在未显式设置时解析为空字符串 `""`。因此在空 Token 默认形态下，任何请求均无法满足 `cfg.AdminToken != ""`，导致管理端进入 100% 无法通过的绝对死锁。
3. **前端交互与凭据入口空缺**（`web/src/api/client.ts:169` 与 `web/src/App.vue:183`）：前端仅被动读取 `localStorage.getItem('csp_token')`，全仓无登录弹窗、无凭据录入表单，且设置视图（`activeTab === 'settings'`）直接回退至 Phase 6.1 静态占位卡片，用户无任何自愈输入手段。
4. **质检门禁虚假假绿失职**（`web/e2e/cross-device.spec.ts:6-64` 与 `hermes kanban log t_94d63420`）：前期 Critic 审查仅运行单测与 Playwright 脚本，而现有 Playwright 脚本使用 `page.route` 全量拦截并 Mock 了所有 API 返回（伪造 200），未连接真实 Go 后端；且 Go 集成测试全量硬编码了 Bearer Token，导致真实环境容器的匿名访问死锁被彻底漏检。
```

### Plan Contract DOC-0141 <a id="plan-doc-0141"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:10`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- **消除开箱即用鉴权死锁**：实现开放（Open Mode）与保护（Token Mode）双模态鉴权，未配 Token 时默认开放私有访问，即刻恢复 `sub.einck.top` 生产可用性。
- **发布导出令牌硬隔离**：无论处于何种模态，严格禁止发布导出令牌（Publication Export Token）访问 `/api/v1/` 管理接口，违者拦截为 403 Forbidden。
- **轻量前端凭据闭环**：在 `api/client.ts` 引入全局 401 拦截钩子，提供响应式 `AuthModal` 弹窗与设置页凭据管理中心，消除占位卡片。
- **重塑 Critic 真机对抗门禁**：建立严禁 Mock 路由的端到端质检规范，由无头浏览器针对真实容器执行匿名态与受控态双轨对抗断言，确保控制台零 401、DOM 渲染零异常。
**Non-Goals:**
- 不引入笨重的外部 OAuth2 / OIDC 服务端或第三方认证中间件（保持 Go 单二进制自包含极简架构）。
- 不破坏现有发布接口 `/publish/v1/{publication_id}` 的匿名/Token 订阅导出逻辑。
- 不引入多用户 RBAC 权限管理或复杂用户密码数据库表（沿用单一 Admin Token 极简模型）。
```

### Plan Contract DOC-0142 <a id="plan-doc-0142"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:25`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
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
```

### Plan Contract DOC-0143 <a id="plan-doc-0143"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:41`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
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
```

### Plan Contract DOC-0144 <a id="plan-doc-0144"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:64`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0145 <a id="plan-doc-0145"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:78`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
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
```

### Plan Contract DOC-0146 <a id="plan-doc-0146"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:94`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk 1: 开放模式在公网环境下暴露管理接口]**
- *Mitigation*: 在工作台顶部导航栏渲染醒目的黄色安全提示徽章 `Open Mode`，并在设置页明确说明“当前未配置 CSP_ADMIN_TOKEN，建议在生产暴露时配置令牌”；同时现有文档与 Docker Compose 示例提供设置指南。
- **[Risk 2: 导出令牌在 Open Mode 下被误认为合法管理请求]**
- *Mitigation*: 鉴权中间件在逻辑上严格先执行 `isPublicationToken(...)` 判断，只要携带导出令牌，不论 Open Mode 还是 Token Mode 一律强制 403 阻断。
- **[Risk 3: 并发 401 请求引发前端事件风暴或界面死锁]**
- *Mitigation*: 前端凭据管理器使用单例防抖锁，保证无论一秒内收到多少个 401 失败，仅有首个请求唤起弹窗，后续请求排队或在凭据录入后统一重发。
```

### Plan Contract DOC-0147 <a id="plan-doc-0147"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:103`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. **零停机平滑更新**：
- Executor 完成后端与前端代码改造并打包镜像；
- 执行 `docker compose up -d --build` 平滑重启容器；
- 生产环境现有 `CSP_ADMIN_TOKEN` 为空配置无需改动，服务重启后将立即无缝进入 Open Mode，死锁瞬间解除；
- 用户如需加固安全，仅需在 `.env` 或 `docker-compose.yml` 中赋予 `CSP_ADMIN_TOKEN` 任意强密码并 `docker compose up -d` 即可秒级开启 Token Mode。
2. **回滚路径**：
- 镜像使用本地构建，若出现异常可通过 `git checkout` 与重建恢复。由于不修改 SQLite 底层表结构与数据格式，具备秒级回滚安全性。
```

### Plan Contract DOC-0170 <a id="plan-doc-0170"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:1`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

当前生产环境出现订阅发布请求 404 与拉取鉴权故障。深入排查表明：
1. 生产环境数据库 `publications` 表当前有效记录数为 0 条，且请求使用了历史短路径 `/p/...`，而系统标准服务路径为 `/publish/v1/{publication_id}`，导致 Chi 路由返回 404 Not Found；
2. 现有导出端点 `publicationClientHandler` 在代码中硬编码强制要求提供令牌，且底层 `svc.ResolveAndServe` 仅校验 publication 自身的专属 `pub_<hex>` 令牌，既不支持免密公开拉取，也不支持使用管理端配置的全局统一系统令牌；
3. 管理端鉴权逻辑完全绑定于密码字符串是否存在（`admin_token != ""` 即 protected，反之 open），缺乏由用户独立控制的开关；用户明确要求“管理端与导出链接鉴权开关独立受控，但能复用同一个统一令牌”。
```

### Plan Contract DOC-0171 <a id="plan-doc-0171"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:8`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 实现数据库迁移 `000013_auth_settings.sql`，在 `settings` 表中解耦持久化 `admin_auth_enabled` 和 `export_auth_enabled`；
- 在领域层与仓储层实现开关状态的原子持久化与读取，确保默认值为 1（全受控）；
- 解耦 `DynamicTokenHolder`，提供由开关独立控制的 `AdminMode()` 与 `IsExportAuthEnabled()`；
- 改造 `publicationClientHandler` 导出端点，在关闭导出鉴权时免密放行，开启时支持专属发布令牌与全局统一系统令牌双重验证；
- 路由层支持 `/p/{publication_id}` 历史短路径，避免客户端 404 故障；
- 提供标准 `GET/PUT /api/v1/settings/auth` API，并保持旧有 API 向后兼容；
- 前端 `SettingsView.vue` 落地双开关与统一令牌维护；`PublicationsView.vue` 落地空状态向导与免密/受控动态直链呈现。
**Non-Goals:**
- 不构建多租户/多角色复杂 RBAC 鉴权体系，保持单管理员控制面极简架构；
- 不修改底层密码哈希体系（沿用 Bcrypt 与 SHA-256 预哈希）；
- 不改动不可变配置编译引擎的核心产出与签名摘要算法；
- 不在 OpenSpec 规划阶段直接触碰或修改业务代码。
```

### Plan Contract DOC-0172 <a id="plan-doc-0172"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:27`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 鉴权开关持久化与向后兼容迁移

- **设计选择**：编写 SQLite 增量迁移脚本 `migrations/000013_auth_settings.sql`：
```sql
ALTER TABLE settings ADD COLUMN admin_auth_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE settings ADD COLUMN export_auth_enabled INTEGER NOT NULL DEFAULT 1;
```
- **技术理由**：使用 `DEFAULT 1` 保证存量配置升级后默认处于受控状态，杜绝任何未预期的安全降级。`internal/domain/settings.go` 中 `Settings` 结构体增加 `AdminAuthEnabled bool` 与 `ExportAuthEnabled bool`；`internal/repository/sqlite/settings.go` 在 `Get` 与 `Update` 方法中无缝映射。
```

### Plan Contract DOC-0173 <a id="plan-doc-0173"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:35`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 内存鉴权模型 DynamicTokenHolder 解耦

- **设计选择**：在 `internal/transport/http/token.go` 的 `DynamicTokenHolder` 内部增设 `adminAuthEnabled` 与 `exportAuthEnabled` 状态变量及对应读写互斥锁：
- `AdminMode() SecurityMode`：当 `adminAuthEnabled == false` 时，强制返回 `SecurityModeOpen`，管理端完全免密放行；当 `adminAuthEnabled == true` 且存在有效令牌时，返回 `SecurityModeProtected`；当开启但未配置令牌时，保持 Zero-Config 开放特性。
- `IsExportAuthEnabled() bool`：直接反映导出端点是否受控。
- `Verify(candidate string) bool`：恒定时间比对候选令牌与系统主令牌哈希。
```

### Plan Contract DOC-0174 <a id="plan-doc-0174"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:41`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0175 <a id="plan-doc-0175"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:47`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 路由层兼容性修复与 `/p/...` 故障消除

- **设计选择**：在 `internal/transport/http/router.go` 中除保留 `/publish/v1/{publication_id}` 之外，同时挂载 `/p/{publication_id}` 路由，直接由相同处理函数接管或执行 307 Temporary Redirect（携带原始 QueryString），无缝化解历史客户端与外部订阅软件请求 `/p/...` 导致的 404 故障。
```

### Plan Contract DOC-0176 <a id="plan-doc-0176"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:50`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 鉴权管理 API 扩展与平滑过渡

- **设计选择**：
- 新增 `GET /api/v1/settings/auth`：返回当前管理端与导出链接的开关状态、生效安全模态以及是否存在主令牌；
- 新增 `PUT /api/v1/settings/auth`：支持原子更新开关状态与可选的主令牌；
- 保留旧版 `POST/GET /api/v1/settings/admin-token`，底层无缝桥接至统一令牌更新逻辑；
- 维护公共探测端点 `GET /api/v1/auth/status`，准确反映管理端当前运行模态（Open 或 Protected）。
```

### Plan Contract DOC-0177 <a id="plan-doc-0177"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:57`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 6. 前端设置中心与发布工作台交互对齐

- **设计选择**：
- `SettingsView.vue`：拆分为管理端保护开关、导出链接保护开关与统一系统令牌维护卡片，并明确标注“统一系统令牌可同时用于管理端登录与受控导出下载”；
- `PublicationsView.vue`：当数据库 0 记录时，呈现醒目的未发布引导空状态卡片，提示用户点击发布；根据导出开关动态决定展示与复制的链接格式（免密直链为纯净 URL，受控链接携带 Token），并更新使用说明文案。
```

### Plan Contract DOC-0178 <a id="plan-doc-0178"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:62`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[风险 1：导出免密后节点配置暴露]** → 对策：数据库迁移默认值强制为 1（受控）；前端设置界面在关闭导出鉴权时弹出显式警告提示，说明此操作将使任何知晓发布 ID 的人均可拉取节点配置。
- **[风险 2：统一系统主令牌越权风险]** → 对策：管理端受控时，专属发布令牌 `pub_<hex>` 访问控制面返回 403 Forbidden `invalid_token_scope`；管理端显式开放或未设置主令牌时所有请求均以开放模式放行，不能宣称发布令牌在此模式下具备权限隔离。开启导出保护时，仅匹配该发布专属令牌或系统统一主令牌的请求可拉取配置。
- **[风险 3：既有自动化脚本与旧接口兼容性]** → 对策：既有 `/api/v1/settings/admin-token` 保持完整保留与向后兼容，所有旧接口均通过单元测试与回归测试保障功能不变。
```

### Plan Contract DOC-0179 <a id="plan-doc-0179"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:1`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

同一 repo-local Change `csp-fresh-inventory-full-validation`，schema `spec-driven`；CLI 返回 changeRoot `/home/service/clash-sub-parser/openspec/changes/csp-fresh-inventory-full-validation`、allowedEditRoots `/home/service/clash-sub-parser`。本轮只修订既有规划工件，不派工、不构建、不网络实验、不改业务/测试/生产配置、不提交、不 QQ（父编排负责里程碑）。用户明确批准本修订和后续既有发布授权，不新增一般审批门槛。
```

### Plan Contract DOC-0180 <a id="plan-doc-0180"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:5`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 最新证据与授权身份

指定预检 session `2026-10-08T05-50-14-467Z_879111db-a2c3b028-3b1a3e40-48b9.jsonl` 最终报告 `rep_exec_csp_continuation_preflight_1`：当前 run `pi_run_20261008_134941_3738865`；本规划会话环境没有不同 run_id，PI_SESSION_ID 为 `eace1c63-0de6-484b-8124-155fd0219b9b`，仅用于会话身份，不能当 run_id 或伪造子会话 ID。父编排继续真实 run 的合同/epoch，不自动继承旧 run `pi_run_20261008_080906_3465602`。
预检 HEAD `8b26f079e852d707c4cc2f74bbac6fb202c7ada4`，59 条状态、47 个 tracked 文件变更、暂存为空，旧相关施工进程定点检查不在。最后 runner/test 修改后验证链 45s 超时，build-final.log 空，未形成最终报告；此前 go test ./... exit1（共享缓存 linker 对象缺失），全量 race 120s 超时无成功退出码；定点 race 和较早编译成功不覆盖最终源码。Reviewer `rep_rev_csp_preview_code_review_2` 时间早于后续 05:40–05:48Z 修改，不能当最终 PASS。
旧实网 `/tmp/csp-real-comparison-20261008T045932Z/`：90 唯一 LogicalID，输入 SHA256 `f3d99066914c49e43019187560bbdd27a36941c971fd057ece7e15ec8842a2e5`，107537726 body bytes、900.027s、1620 终态；reference 两轮 baseline 仅 34/90，CSP 90/90，属于 PARTIAL，不覆盖/删除。run.py 只有计划；reference stage 串行，alive8/media2 未生效；samecore binary 是 v1.19.32 但 engine 标签硬编码 v1.19.31、modfile 改动了传递依赖；平台错误及非 collector 路径未完整导出/计量；实际 CSP baseline 默认 16MiB+探测字节并非 helper 的 64KiB。
预检仅 6.4GiB 可用磁盘。旧 preview `http://127.0.0.1:19080` 健康但不绑定最终源码，instance JSON 含 token 且0644；不能移用旧 Critic PASS。当前生产仍旧镜像，本次未上线。根 `.exp.md` 已完整读取，旧 Python/pytest/backend-data 经验应由后续文档责任人仅加 obsolete 前缀，不作 Go 部署指南；预检相关目录无额外经验，本 change 目录 `.exp.md` 不存在。
```

### Plan Contract DOC-0181 <a id="plan-doc-0181"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:15`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:** 保留 A/B 已有实现，补齐最新源码自测；ALL90 baseline 优先的公平四侧 comparison；native 与 same-engine 方法分开；硬限制整批 body/时间；解释逐节点差异和未执行项；最终独立 review、刷新预览、critic、授权 commit/rollout 与真实长期观察分阶段交付。
**Non-Goals:** 不 reset/import、再次修 Token/删除 kliq，不 blanket-enable 用户 group flag；不替换原生 Check 为 CSP 算法，不复制 GPL 探针进入 MIT 产品，不构建私有测试框架；新批不测速、无 speed URL/请求、无额外重试/自适应扩额/512MiB；不触碰 Pi/gateway/CPA/PDD 或无关服务。
```

### Plan Contract DOC-0182 <a id="plan-doc-0182"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:23`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 保留 A：字段/API/空组 PASS 与独立守卫

字段为 domain `NodeGroup.EmptyFallbackPass bool`、SQLite/JSON/API `empty_fallback_pass`，现有 migration `000017_empty_fallback_pass.sql` 保留，不重建/重复创建。旧行和创建默认 false；Get/List/Create/Update roundtrip，PATCH optional bool 区分 omission/false/true；filter/flag 验证在 metadata/revision 更新前，失败事务原子。现有 UI checkbox/文案明确继续规则匹配、非直连。
空定义沿用完成 admission/global/group/source/probe/risk 组装后的静态解析：Members=0 OR AllNodeLogicalIDs=0。configured 与 effective PASS 分开，只有明确配置的静态空组才输出 `proxies: [PASS]`、`empty-fallback: PASS`。非空勾选组不得有 PASS 成员或 runtime PASS fallback；原生非空 fallback 如需显式设置用拒绝，不允许 COMPATIBLE→DIRECT。未许可空组仍 required_nonempty/empty_routed_group；父组/derived projection 不继承许可，真实节点计数不变，PASS 非 LogicalID。
resolver/digest/compiler/capability/prune/publication compatible/preflight/frozen-content 一致，有效 PASS 不被 optional prune 消失；循环/悬挂/risk/source/probe/global filtered_nodes_empty 仍阻断；非 Mihomo 不能假映射。复用官方 Mihomo v1.19.32 离线规则 fixture：第一条 PASS 后真实落到后续非 DIRECT 控制规则，字符串包含 PASS 不构成证明。digest 随配置变化、同输入稳定，历史 publication 字节不改。
```

### Plan Contract DOC-0183 <a id="plan-doc-0183"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:31`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 保留 A：PCRE、校验上下文和 UI

优先 RE2，已知不兼容模式复用现有 regexp2 v1.12.0，有限正 MatchTimeout 和 pattern/input/cache 上限；Validate/Match 同引擎诊断，not_regex 不把错误/超时取反成通过。归档稳定身份恢复缺失“便宜”配置，保留已有用户非空 filter，事务幂等。原式 `去掉(流媒体|x(?:[0-5](?:\.[0-9]+)?)(?![\d.])|便宜|free )`、Eeox、einck 的 OR 语义不猜改；黄金语料 x5/x5.9/x6/x50/x5.9.1、literal 前缀、大小写、中文、恶意回溯，fixture 自建归档而非私有 DB skip。
policy ValidateGraph/publication 用相同只读 nodes/filters/NodeSources/LatestObservations/AsOf/risk 来源；仓储错误 incomplete，不假空/valid，无网络/写库。既有 regex/not_regex UI、warnings/manual validate、AbortController+seq+revision 保留；初始/保存/reload/retry 成功后最新 revision validate，错误不半保存、晚响应不覆盖。此前 PASS 后新 UI/locale/test 改动仍需最终独立 reviewer，不沿用旧 PASS。
```

### Plan Contract DOC-0184 <a id="plan-doc-0184"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:37`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0185 <a id="plan-doc-0185"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:49`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 不变 manifest 与比较条件

首次外网前冻结唯一 manifest SHA256，涵盖所有四侧和可选 native 两侧、ALL90 identity/config摘要、source/build/engine、AB/BA次序、实际target及子请求清单、timeout、TLS verification/SNI、redirect/HTTP2/UA/keepalive/selection、平台顺序、alive8/media2/speed0、逐node×side×stage body/time allocations、ledger绝对路径、原始总起点/截止、责任人与cleanup。秘密输入仅私有0700目录/0600文件；公开摘要无token/raw URL；凭据不经命令行明文。
comparative CSP/same-engine baseline统一target、15s请求超时（既有维护值）、TLS/redirect/HTTP2；平台每个实际子target/transport等先对齐再计为可比。不能仅统一核心版本。保留native默认差异表；不可对齐维度标 configuration/method mismatch，不混入公平成绩。具体endpoint/TLS值取最终真实输入冻结，规划不虚构未证实地址；execute不得自动补endpoint、改并发/TLS、重试或改变参数，manifest不匹配即fail-closed且无网络。
```

### Plan Contract DOC-0186 <a id="plan-doc-0186"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:55`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 共享跨启动 budget/deadline 与公平参与

总应用响应 body cap =134217728 bytes；整批首次运行开始到结束/cleanup ≤1800s，不是每launch/每侧各30min。（注：历史批次与旧合同保留 128MiB/1800s 硬限额；2026-10-08 用户及主脑新 reissue 授权仅对新绑定 manifest 生效，放宽至上限 1073741824 字节(1GiB)与 7200 秒一次性绝对期限，保持 baseline 四侧全量与既定 intersection 共同资格覆盖逻辑，不改动全项目旧默认与历史事实）。baseline实际node×side所有响应body合计≤65536 bytes，实际CSP runner必须接到该值，不能只改helper/事后摘要；不能用extra probe byte突破上限。四侧最坏 `90×4×65536=23592960 bytes=22.5MiB`，默认platform余量 `110624768 bytes=105.5MiB`，四侧各 `27656192 bytes=26.375MiB`。可选两native baseline最多额外11.25MiB，必须从首次冻结的同一128MiB分配中扣除；不另开预算/时间。
平台额度四主侧完全相等，stage/node可预先冻结quotas算法，eligibility结算后将确定的参与表及SHA记为manifest派生工件，算法/次序不得自适应改动。每个common eligible node获得相同各侧额度和尝试机会：采用固定stage次序、同轮node顺序、逐node×stage×side子额度，不让前节点用完全盘；无额度/时间则skip精确列出。无跨side/stage临时借贷/隐藏重试。平台可能含Netflix/YouTube/Disney/OpenAI/Claude/Gemini/IPRisk，但只尝试manifest列明的能力，不承诺128MiB内全部原生media覆盖。
跨Python/Go子进程及重启复用同一个 ledger（优先stdlib SQLite事务，不依赖每进程重置的计数器）：保存run/manifest绑定、start/deadline、各side/stage/node预留/消费/退款及attempt身份；BEGIN IMMEDIATE/条件更新等成熟原子事务实现read-before-reserve、实际读取结算、未用退款。单个response limiter/lifecycle接到真实reader，所有成功/失败响应及重定向/内嵌client/非collector平台调用都进入同一账本。仅collector结果bytes不是完整计量；做不到全路径覆盖即保持network入口拒绝并整改。
timeout/cancel/deadline停止排队和新请求，取消在途且cleanup留在总截止内；崩溃未结算reservation不得乐观退还为可用额度，恢复只准保守核对/结束，不自动重试。body实际消费、预留和可用余额不越界，NIC/header/TLS/transport另列且有测量局限，不冒称硬NIC全流量上限。阶段预算截断返回预算终态，不直接改写存活策略或永久dead。
```

### Plan Contract DOC-0187 <a id="plan-doc-0187"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:65`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 6. 逐节点执行与缺陷解释

node×side×stage×attempt恰一终态，保留queued/not_scheduled/executed、原始verdict/status/typed errors、配置和source/profile/target/engine/commit、时间/body/reservation/refund、取消/复验关联。dependency_skipped保留先前失败根因，不由后来的deadline/budget合成覆盖；restricted/unknown/fail/cancelled/budget_not_executed/stale/configuration_mismatch分账。errors.Is/As区分net/x509/context等，不猜DNS/TLS，不漏非collector的native平台返回错误及内部请求。primary discordant清单单独解释，诊断不授权额外网络尝试。
所有原始失败/skip不可覆盖成success；旧partial证据保留。比对结论按已执行且条件对齐的阶段/节点，partial不声称全量parity。发布决策看解释清楚的缺陷及是否影响正确性/安全，不要求headline counts一致；真实缺陷先整改/retest/review，方法差异和不执行项如实记录，不能以partial为假阻断或假PASS。
```

### Plan Contract DOC-0188 <a id="plan-doc-0188"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:71`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 7. 离线确定性门禁与资源边界

使用成熟 Go testing/t.TempDir/httptest、Python unittest黄金分配/调度测试、Vitest/Playwright及官方Mihomo；禁止新测试框架。自测无外网/商业源/生产DB：`GOPROXY=off GOTOOLCHAIN=local`、私有GOCACHE/GOTMPDIR，并将fixture网络限定loopback（禁外部egress）；空订阅Check、无测速、budget漏路与默认network拒绝以fail-on-network fixture验证，不把env变量当OS网络沙箱。
需要最终源码的 `go build ./...`、`go test ./...`、`go test -race ./...` 全量命令证据；不能用targeted race文本替代full。若full race真实受复现资源/工具链限制，保留非0/超时、确切命令/版本/资源日志/复现步骤，相关checkbox不勾且报告unchecked；由父编排明确裁定该环境局限，不伪造全部门禁通过。full build/test失败继续本地整改，不能只写环境结论。
reference native和same-engine分别离线build/test/fixture（含Check、transport、无speed、metadata/pin diff与计量所有路径）；Python golden覆盖四侧/可选两native预算、交集、ABBA stage顺序/公平participation、cancel/deadline、manifest篡改默认拒绝、跨launch共享ledger并发/退款/崩溃保守；Go覆盖实际64KiB runner而非helper、2xx/status、typed原因、超额/取消/race。Web type-check/tests/开发build禁COPY，最新UI变更再review。
磁盘预检仅6.4GiB，executor先定点资源复核、限制单写build与GOMAXPROCS/test -p等实际资源参数、复用已核实离线模块；不删除共享cache、不盲装/下载、不漫游查环境。每个长命令合理timeout和稳定日志，用工具执行输出/进度记录观察，不高频轮询机械重试。私有cache/tmp/log预算在可用空间内留余量，全部artifact先记录owner后cleanup；缺cache或真实外部硬边界交回父编排，不扩授权。
```

### Plan Contract DOC-0189 <a id="plan-doc-0189"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:81`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 8. 拟定确切命令接口（待施工实现，不冒称已经存在）

在repo root：
```
python3 tools/probe-comparison/run.py plan --inventory <private-all90.json> --output <private-plan.json> --response-body-budget-bytes 134217728 --deadline-seconds 1800 --baseline-body-limit-bytes 65536 --alive-concurrency 8 --media-concurrency 2 --speed-concurrency 0 --native-baseline-sides 0
python3 tools/probe-comparison/run.py freeze --plan <private-plan.json> --manifest <private-manifest.json> --ledger <private-ledger.db>
python3 tools/probe-comparison/run.py execute --manifest <private-manifest.json> --ledger <private-ledger.db> --output <private-results-dir> --allow-network
python3 tools/probe-comparison/run.py report --manifest <private-manifest.json> --ledger <private-ledger.db> --results <private-results-dir> --output <sanitized-report.json>
python3 -m unittest discover -s tools/probe-comparison -p 'test_*.py'
GOPROXY=off GOTOOLCHAIN=local GOCACHE=<private-cache> GOTMPDIR=<private-tmp> go build ./...
GOPROXY=off GOTOOLCHAIN=local GOCACHE=<private-cache> GOTMPDIR=<private-tmp> go test ./...
GOPROXY=off GOTOOLCHAIN=local GOCACHE=<private-cache> GOTMPDIR=<private-tmp> go test -race ./...
```
`--native-baseline-sides` 只允许0或2；2必须plan-time可行性验证通过，否则拒绝。plan/freeze/report绝不网络；execute缺--allow-network、pin不符、manifest不完整/变更、预算超过对应 manifest 冻结限额（旧批次 128MiB/1800s，新 reissue 批次上限 1GiB/7200s）、speed>0/URL存在、deadline超时、ledger绑定不符或自测/独立review未通过全部拒绝且不创建请求。显式allow-network也不绕过门禁。freeze输出完整实际child argv/schema（包括native Check/native config vs normalized stages）、二进制摘要，execute只接受已冻结参数；不能按新flag私改命令或自动探测endpoint。Go adapter基于既有入口扩展这一数据契约，需README列明真实参数与fixtures；本规划不猜未取证的入口名。网络执行仅父编排在最终review后启动，同一批准总盘子，没有计划外probe。
Web在web目录执行 `npm run type-check`、`npm test`、`NO_COPY_WEBASSETS=1 COPY_WEBASSETS=0 npm run build`；private native snapshot 用 `GOPROXY=off GOTOOLCHAIN=local go build ./...` / `go test ./...`，same-engine同样命令加 `-modfile=<frozen-samecore.mod>`，先记录必要`-mod=mod`产生的diff再冻结，禁止执行阶段更新modfile或联网。全部命令记录真实exit_code和日志/source指纹。
```

### Plan Contract DOC-0190 <a id="plan-doc-0190"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:100`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 9. DAG、合同与终态

一个完整B纠偏包保留A/B源码，包含必要A最终回归、Python scheduler、真实reference+same-engine+native fixtures及全量自测；不按函数/checkbox派微任务。单写模块/schema/ledger和generated证据，不伪造并发安全。完成→独立最终Reviewer→已批准实网staged batch→最终源码冻结刷新preview→Critic→assets/commit→backup/app-only rollout→独立7200s观察；实网引出源码变化须回full self-test/review，再刷新preview。
父编排运行合同version1：真实run_id（当前报告值如上，以实际运行确权）、同一change/changeRoot与repo root分开、既有A/B全部work_packages及epochs、自测/reviewer必选、critic applicable=true（policy UI/发布真实体验）。本规划不改run metadata、不宣布PASS。派工必须包含run_id/change/work_package/epoch/reverification_of；五要素executor报告与结构化commands真实退出码、独立review、critic真实证据，历史失败引用保留。
preview准备者建立private DB/端口、禁止自动refresh/probe/periodic外网，收紧含凭据JSON为0600（对外实例不含token），最终binary/frontend SHA与sourceManifest绑定最终diff。旧19080健康不能替代刷新实例。instanceId/url/health/allowedOrigins/testDataBoundary/scenarios/shutdownOwner/cleanupToken/sourceManifest完整后才交Critic。Critic只读Playwright+read PNG，既有375/390/1440视口无遮/溢出、PASS持久化/非空/关闭、regex编辑、reload/retry/竞态/最新issues、发布preview与probe状态解释；fixture非商业解锁证据。
Reviewer与Critic后准备者以 `COPY_WEBASSETS=1 NO_COPY_WEBASSETS=0` 同步最终嵌入资产，核对与预览内容绑定且无额外源码变更；根编排仅git_commit授权CSP路径，不夹带无关/用户staged变更。`.exp.md`旧Python段落仅加obsolete标识并限定不指导当前Go部署，不无关重写。
部署严格依 `docs/controlled_deployment_runbook.md`/operations：SQLite官方online .backup，size/schema/integrity/FK/SHA/0600/owner、新schema兼容isolated rollback演练与当前image rollback tag；app-only/no-deps/no-build，不down/delete volume/reset/import/回灌旧库存/代替用户开启flag。验证双loopback healthz/readyz、401合法鉴权、schema/assets/FK0与合法管理/发布操作。rollout健康只是短时上线证据；9.2必须另交真实起止≥7200s周期日志，不能靠health poll或隔离加速冒充完成，无限轮询禁止。未观察完如实partial，不撤销已有发布授权或伪勾9.2。
```

### Plan Contract DOC-0191 <a id="plan-doc-0191"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:112`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- 128MiB是应用body总额而非NIC；native平台多子请求可能无法全部覆盖，明确unattempted而不自动扩额。
- comparative归一化不能冒称原生方法；native与same-engine依赖/默认差异分开，不凭headline计数判release。
- 崩溃reservation保守占额可能减少剩余覆盖，但避免跨launch超额与隐形重试。
- full race环境限制必须可复现且unchecked；旧targeted PASS、空日志和超时不是成功。
- PCRE兼容非完全等价；指定语义/黄金数据验证，超时fail-closed。
- 本轮READY仅规划修订完成，实施/review/实网/critic/commit/部署/长观察仍须真实证据。
```

### Plan Contract DOC-0192 <a id="plan-doc-0192"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/design.md:1`
- **Source Digest**: `aebac2fba78d6e0a4764f7794b3bedd31c7665bebee95876fcf3086af8a27ca4`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

当前 `GET/PUT /api/v1/policies/global-node-filter` 已保存 `{spec:{conditions:[...]}}`；`internal/domain/node_filter.go` 与 resolver 已有 AND 条件、probe_kind、判定、24h 默认/可选有效期及不匹配原因。设置页无入口，策略页已有表单但声称凭据版本匹配，实际 `MatchesCondition` 只检查时间和 verdict。`publication.Service.resolveSnapshot` 在过滤定义、来源、观测读取失败时容忍失败；无路由组时仅靠既有空路由组预检无法发现零节点。`Popover` 的 absolute 菜单位于 `SubscriptionsView` 的 overflow-hidden 卡片内，`ModalDialog` 使用 `showModal()` 而 `ConfirmModal` 是普通 z-index div，后者无法超过浏览器 top layer。`web/src/style.css` 已提供 `adaptive-surface-dialog/sheet` 等视口 token。
```

### Plan Contract DOC-0193 <a id="plan-doc-0193"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/design.md:5`
- **Source Digest**: `aebac2fba78d6e0a4764f7794b3bedd31c7665bebee95876fcf3086af8a27ca4`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

- Goals: 设置页可编辑唯一全局筛选，以探针状态为主；同一解析结果驱动四目标新预览/发布，读取失败和配置导致零候选不得扩大或产生意外发布；跨视口浮层无遮挡、可滚动、键盘可操作。
- Non-Goals: 新筛选数据库迁移、第二套筛选 DSL、自动重写旧直链、声称凭据版本核验、把 queued/probing 存为 verdict、改造探针调度、上线生产。
```

### Plan Contract DOC-0194 <a id="plan-doc-0194"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/design.md:10`
- **Source Digest**: `aebac2fba78d6e0a4764f7794b3bedd31c7665bebee95876fcf3086af8a27ca4`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Decisions

1. **一个权威配置，多个管理入口。** 设置页使用现有 API 与 `policyTypes.ts` 的领域类型/校验（按需复用已有 `usePolicy` 的读写方法或抽出已有读写逻辑，不能复制一套存储）。以 verdict 类型/可选 latency 条件优先展示，并允许已有名称/协议/来源条件原样回显与编辑；判定选项只限已有 verdict。保持最多 32 个条件、服务端校验为最终权威；前端提示 24h 默认有效期及已发布内容冻结。策略页保留同一入口、纠正凭据版本匹配的虚假声明，并对状态说明与设置页一致。缺失记录/空条件透传，但传输、数据库读取错误不可用空条件替代。
2. **安全失败发生在共有解析边界。** 在 `internal/application/publication/**` 的 `resolveSnapshot` 及相邻 preview/preflight/publish 共用入口处理筛选定义读取错误；含条件时从字段识别所需依赖（probe 对应观测、source_subscription_ids 对应来源，必要时考虑组级条件），依赖未注入或读取失败即带安全错误返回，不能以空 map 代替。沿用现有 repository/resolver 和 Go 错误/领域错误，避免仅在 UI 或单一 target 特判。考虑 group filters 的读取失败，同样不得静默丢弃配置以扩大导出；未配置条件正常透传。观察到无匹配时使用 snapshot 的现有节点/FilterCounts 和诊断，在编译/发布持久化前统一拒绝或给出明确诊断。测试覆盖无路由组，不能仅以 empty_routed_group 为防护；区分完全没有库存且未配置筛选的既有行为。旧直链只提供已冻结 artifact，不受当下筛选状态和读取失败回溯影响。
3. **复用浏览器层叠和现有布局。** `Popover.vue` 用 Vue `Teleport` 至 body 摆脱卡片裁切，以触发器 DOMRect 和浮层实际尺寸在打开、滚动、resize 时计算可视区域内位置（可复用项目现有 `@vueuse/core` 能力，避免自制通用定位框架；若已有可靠定位依赖，优先采用成熟 API）；保留 trigger/slot API，兼顾外部点击和焦点恢复。`ConfirmModal.vue` 采用原生 `<dialog>.showModal()` 加入 top layer，而不是继续叠加 z-index；沿用既有 `update:modelValue`/`confirm`/`cancel`，Escape/backdrop/loading 语义需验证。针对已打开的 `ModalDialog`，确保原生对话框同一 top layer 后打开者可交互。`ModalDialog`、`ConfirmModal`、`Drawer` 的标题/正文/操作沿用项目自适应 token、flex shrink 与可滚动区域，短高度时允许整体/正文按实际空间滚动，操作与关闭仍可见。此包同时完成 subscriptions/publications/policy/probes 的必要调用点适配与回归。
4. **并行写集合严格分离。** A 仅 `internal/application/publication/**`；B `web/src/features/settings/**`、`web/src/features/policy/PolicyView.vue`（只改文案，如必须共享新代码也由 B 在其拥有文件内完成，避免触碰 C）；C `web/src/ui/{Popover,ModalDialog,ConfirmModal,Drawer}.vue` 与 subscriptions/publications/probes 组件及其测试，不写 PolicyView.vue 或 settings。跨包交互仅使用现有 API/props/slots，不并行修改 `web/src/style.css`；新类型尽量复用已有类型，不扩大写集合。若实现必须触碰其他公共文件，先协调写集合冻结，不能私自双写。
```

### Plan Contract DOC-0195 <a id="plan-doc-0195"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/design.md:17`
- **Source Digest**: `aebac2fba78d6e0a4764f7794b3bedd31c7665bebee95876fcf3086af8a27ca4`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- 所有解析入口在筛选库故障下安全拒绝，会使配置错误时新预览/发布不可用；这是优先防止额外节点泄露的明确取舍，已发布冻结链接不变。服务层依赖未配置与历史无筛选的兼容须由夹具确定性区分。
- 浮层放到 body 后，外部点击、关闭时焦点恢复、菜单滚动跟随及 top layer 中定位必须在真实浏览器验证；不要仅用 jsdom 的静态矩形断言宣布无裁切。
- 可用的视口空间随软键盘、安全区及浏览器缩放改变，应采用现有响应式 token 和浏览器实际视口，不把凭空固定像素当硬验收标准。
```

### Plan Contract DOC-0196 <a id="plan-doc-0196"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/design.md:23`
- **Source Digest**: `aebac2fba78d6e0a4764f7794b3bedd31c7665bebee95876fcf3086af8a27ca4`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Verification

- A：仓库 stub 注入筛选/组筛选/观测/来源读取错误、依赖缺失，断言所有新入口安全失败且无创建记录；配置 probe 条件含缺失/过期观测、零候选且无路由组、四 target 共同筛选；确认旧 artifact 保持冻结。
- B：设置页 GET/PUT payload、刷新回显、空条件、非法字段/判定/有效期以及失败不覆盖，策略页文案不得再宣称凭据版本核对或持久 queued/probing。
- C：组件层菜单脱离裁切祖先、Escape/外部点击/焦点；叠层确认框事件与键盘；真实浏览器四代表视口与额外短高度检查订阅、发布、策略和探针页面长内容可滚至底，交互按钮不被覆盖；测试可运行于隔离本地预览，不触碰生产。最终统一执行 `go build ./...`、`go test ./...`、`npm run type-check` 及相关前端测试；独立审查与黑盒验收不得预先勾选。
```

### Plan Contract DOC-0198 <a id="plan-doc-0198"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:11`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 将轻量扫描周期（默认 10 分钟）与用户配置的节点观测新鲜度周期（`interval_seconds`）彻底解耦；
- 计划启用或配置变更时即刻唤醒巡检，不空等整段周期；
- 基于 `ListLatestByNodes` 增量比对过期：仅当 (node, kind) 无历史观测或观测时间已过有效期（距今 >= `interval_seconds`）时入池；
- 对失败观测施加合理冷却期（默认 5 分钟），避免失败节点每轮重复震荡；
- 实施单轮任务派发上限配额（默认 512 任务），并按观测时间升序（从未观测优先、最老观测次之）排序，防止单批突发冲击并保证防饥饿公平调度；
- 启动与恢复自愈：自动将历史遗留的远期 `next_due_at` 校正为当前即刻巡检；
- 前端控制台文案明示巡检时钟与观测有效期，修复空闲时不轮询的缺陷（引入 15 秒低频保底刷新），并在立即触发后维持追踪。
**Non-Goals:**
- 不修改底层 SQLite 数据库模式（不增加新表或破坏现有迁移历史）；
- 不变更 `domain.ProbeSchedule` API 的基础传输结构（保留 `interval_seconds`, `next_due_at` 字段语义，向后兼容）；
- 不引入外部复杂调度框架（如 cron 或独立调度器服务），保持单进程纯 Go 协程自愈架构。
```

### Plan Contract DOC-0199 <a id="plan-doc-0199"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:29`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 时钟模型解耦与 `next_due_at` 语义

- **决策**：巡检时钟固定为每 10 分钟一次（`defaultSweepInterval = 10 * time.Minute`，支持测试 Option 覆盖）。`ProbeSchedule.NextDueAt` 保存下一次轻量巡检的时间戳（即 `now.Add(sweepInterval)`），而非整段 `interval_seconds`。
- **配置变更时处理**：当管理员调用 `UpdateSchedule` 启用或变更配置时，将 `NextDueAt` 设为 `now` 并调用 `coord.Wake()`，使巡检协程立刻启动评估。
- **替代方案考虑**：为每个节点单独维护定时器。被否决，因为 960+ 节点的定时器过多且无法有效合并网络探测连接与出站会话。
```

### Plan Contract DOC-0200 <a id="plan-doc-0200"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:34`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 增量过期与失败冷却算法

- **决策**：
1. 获取全部激活节点；
2. 批量调用 `ListLatestByNodes` 获取各节点各类别的最新观测 `obs`；
3. 对每对 `(node, kind)` 进行判断：
- 若 `obs` 不存在：判定为到期；
- 若 `obs.Verdict` 为失败（`Failed` / `Timeout`）：检查冷却时间 `now.Sub(obs.ObservedAt) < failureCooldown`（5 分钟），若在冷却期内则跳过，超过冷却期且超过 `interval_seconds` 则判定为到期；
- 若 `obs.Verdict` 为成功（`Available` / `Degraded` 等）：若 `now.Sub(obs.ObservedAt) >= interval_seconds` 则判定为到期，否则未过期。
4. 仅收集到期的 `(node, kind)`。
```

### Plan Contract DOC-0201 <a id="plan-doc-0201"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:44`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0202 <a id="plan-doc-0202"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:52`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 启动与 Recover 自愈与租约解耦

- **决策**：
- 在 `PeriodicCoordinator.Recover` 中，若已启用的计划其 `NextDueAt` 为空、在过去（已过期），或处于未来大于一个巡检周期（例如老逻辑写下的 2 小时后），统一将其校正为 `now`，让服务在启动后即刻开启首次巡检。
- 租约生命周期与批次状态正交管理：`UpdateBatch`（无论是 SQLite 还是内存 Mock）仅持久化批次执行状态（`state`, `counts`, `run_ids`, `redacted_error`, `updated_at`），禁止重写或覆盖租约列（`owner`, `lease_until`）。租约归属于 `AcquireLease`、`HeartbeatLease` 与 `ReleaseLease` 专责维护，杜绝多实例执行期间多次 `UpdateBatch` 清空租约导致其他实例重复夺约并并发派发。
- 执行过程中通过 `leaseLost` 渠道严格熔断，一旦心跳失败或租约丢失立即停止后续批次写回，防止覆盖新主状态。
```

### Plan Contract DOC-0203 <a id="plan-doc-0203"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:58`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 前端空闲保底刷新与触发后跟踪

- **决策**：在 `ProbesView.vue` 中，当页面处于空闲状态（无活跃 run 且队列为 0）时，维持每 15 秒一次的低频保底刷新，定时获取最新 `batches` 与 `runs`；点击“立即触发”时，设置活跃追踪状态并在触发后 10 秒内保持高频轮询（每 2 秒一次），确保即时捕获批次与运行状态迁移。
```

### Plan Contract DOC-0204 <a id="plan-doc-0204"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:61`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk] 节点数量极大时（如数万节点）单次扫描开销** → 采用分批获取激活节点与批量 `ListLatestByNodes`（分批 100 个节点查询），内存占用极低且耗时在数十毫秒内完成。
- **[Risk] 探测失败节点在冷却后频繁重试** → 失败节点仍需满足同时超过冷却期（5 分钟）与检测周期（`interval_seconds`），不会比正常节点探测更频繁。
- **[Risk] 多实例并发抢占** → 继承已有的 CAS Batch Lease 机制，同一巡检窗口只有一个实例获得执行权。
```

### Plan Contract DOC-0205 <a id="plan-doc-0205"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:67`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. 在 `internal/application/probe/periodic.go` 中重构巡检与过期判定逻辑，注入 `ProbeObservationRepository`；
2. 在 `cmd/csp/main.go` 中为 `NewPeriodicCoordinator` 传入 `probeObsRepo`；
3. 更新 `service.go` 的 `UpdateSchedule` 逻辑：启用或修改配置时设定即刻巡检；
4. 更新前端视图文案与空闲刷新逻辑；
5. 运行完整单元测试与端到端模拟测试。
```

### Plan Contract DOC-0206 <a id="plan-doc-0206"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:1`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

见 `proposal.md`。该仓库正在进行 Clean-Slate Phase 4–7；本 change 只定义新的独立能力，不重建或修改现有 change。工作树当前将旧 Python、前端和既有 Go 包标记为删除，并新增 Go 基座；因此任何设计不得依赖旧路径或旧 SQLite schema。`internal/domain/probe.go:60-70` 的 `ProbeObservation` 已是不可变、最小化的公共证据骨架，`internal/application/observation/service.go:52-105` 已按 SHA-256 digest 和 bounded redacted summary 持久化结果，适合作为风险证据的安全基线。
出口路径已有关键硬事实：`internal/probe/singbox/runtime.go:148-175` 经该 runtime 的 outbound 拨号；`internal/probe/singbox/runtime.go:178-208` 构造的 HTTP transport 以 `Proxy: nil` 禁止读取 `HTTP_PROXY`、`HTTPS_PROXY` 或 `ALL_PROXY`。风险查询必须复用该隔离边界，不能增加系统代理或网页抓取的旁路。`profiles.Profile` 当前只有 baseline、geo、streaming、AI、speed 五类版本化契约（`internal/probe/profiles/profiles.go:12-18,47-103,150-153`），新类型需要通过 domain/profiles 的显式扩展实现。
风险供应商提供的语义并非可以无损互换：Scamalytics 面向 IP fraud score、国家、operator、proxy 与 Tor 状态。[1] IPQS 将 >=75 描述为 suspicious/可能 proxy、VPN 或 Tor，而非必然欺诈，>=90 才为 high-risk。[2] MaxMind 说明 residential proxy 更难检测，并将其与其他 anonymizer trait 分开。[3] 因此本设计禁止将任意 provider 的原始分数直接作为通用的准入阈值，必须先保留来源版本、归一化语义与 policy revision。
```

### Plan Contract DOC-0207 <a id="plan-doc-0207"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:9`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 形成一条唯一、受限的证据流：被测节点 outbound → exit identity → 官方 provider API → 脱敏不可变 observation → versioned risk policy → resolver/publication/UI read model。
- 将第三方风险事实、CSP 本地风险决策、节点 capability verdict 三者物理分离，并让未知与契约漂移安全可见。
- 允许不同 provider 的可追溯并存，但以一个 policy 的明确选源、冲突和 unknown 规则得出确定性结果。
- 为节点管理筛选、受保护策略组和发布前阻断提供同一决策快照，而不将风险 state 回写为节点固有属性。
**Non-Goals:**
- 不默认启用任何商业 API，不购买配额，不在本 change 写入 provider 密钥或触发真实公网风险查询。
- 不爬取或解析 Scamalytics、IPQS、MaxMind 的网页；不以 HTML、浏览器会话或反爬规避作为数据路径。
- 不以风险分数推断节点真实性、地理位置、流媒体或 AI capability；不覆盖 `ProbeObservation` 的既有 verdict。
- 不建兼容 adapter、旧数据迁移、双写、全局风险 JSON blob 或客户端全量筛选。
```

### Plan Contract DOC-0209 <a id="plan-doc-0209"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:44`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D2 单节点通道、官方 API adapter 与 provider registry

引入 application port `IPRiskProvider`，其输入是受控的 `ExitIdentity` 和 request context，输出是内存中的 provider-specific normalized candidate。每个 adapter 被 registry 以 `provider` 和 `schema_version` 注册，只有 deployment settings 中 `enabled=true` 且 secret reference 可解析时可被调度。adapter 不拥有 HTTP client、secret store、SQLite connection 或 policy；probe orchestrator 注入由 `singbox.Runtime.HTTPClient` 取得的 client 和 request-scoped credential resolver。
执行固定序列：
```text
node config → sing-box Runtime → exit identity profile → exit_identity_digest
→ provider budget/admission → provider API via same Runtime HTTP client
→ normalize + contract validate → IPRiskObservation → repository append
```
每个 provider 有 `max_concurrency`、token bucket `requests_per_minute`、`daily_request_budget`、`per_request_timeout` 与最大 response bytes。网络 call 永远在 DB transaction 外；budget reservation、observation append 和 audit 使用短事务。429、预算耗尽、缺密钥、非版本化 response、deadline、DNS/network error 各产生不同 reason code，绝不回退到另一 provider、网页端或系统代理。
现有 runtime 的 `Proxy:nil` 和 custom `DialContext` 说明这种注入是可行的（`internal/probe/singbox/runtime.go:178-208`）。sing-box 的路由规则可按条件匹配流量，[5] 但本专项不增加额外 route-based fallback，避免同一 observation 的真实出口不可证明。
替代方案：直接 `net/http` 调 provider、使用 host proxy、或把 provider 调用放在前端。前两者违反出站证据；后者泄露密钥并不可审计；拒绝。
```

### Plan Contract DOC-0210 <a id="plan-doc-0210"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:62`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D3 新鲜度、缓存和多源融合

风险缓存 key 固定为 `(node_logical_id, exit_identity_digest, provider, provider_schema_version)`。仅 `status=available`、`observed_at < expires_at`、provider 未被停用且 key 完全相同的 observation 可复用。对 exit identity 不能确认、出口 hash 改变或 TTL 到期时，风险输入为 `unknown` 并重新排队（受 budget 限制），不得重放旧风险结论。
每个 `RiskPolicy` 明确选择以下融合之一，禁止 implicit averaging：
- `single_provider`: 只使用声明 provider 的最新有效 observation
- `all_must_allow`: 每个 required provider 都必须给出 allow；任何 block 为 block，未知按 `unknown_action`
- `highest_risk`: 只在 policy 列出可比的 `(provider,schema_version)` 映射时，采用最高 band；否则 `unknown`
score bands 冻结为 `0..100` 的不交叠完整区间，结合 minimum confidence 和 trait predicate 决定 `allow|review|block`。`unknown_action` 默认为 `review`，可配置 `allow|review|block`；`conflict_action` 默认为 `review`。这避免把 IPQS 等 provider 的阈值误称为跨厂商硬真相。[2]
替代方案：把多 provider 分数求平均或缺失值当 0。会掩盖语义差异与缺数风险；拒绝。
```

### Plan Contract DOC-0211 <a id="plan-doc-0211"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:76`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D4 Resolver 与 publication 的消费边界

`RiskDecisionService` 只接受 active inventory watermark、risk policy revision、固定 `evaluated_at` 和 observation repository，生成排序稳定的 decisions。`ResolvedPolicySnapshot` 增加 `risk_policy_revision?`、`risk_evaluated_at?`、`risk_decision_digest` 与 admitted/excluded node diagnostics；不会储存原始 risk data。现有设计要求 resolver 是 preview、下载前校验与 publication 的唯一输入（`csp-clean-slate-v1/design.md:88-94`），本 change 延续该单一路径。
- 未绑定 policy 的 group 维持原语义
- `block` 无条件从绑定 group 的 snapshot 排除
- `review` 是否隔离或排除由 policy 的 `review_action` 决定
- publication preflight 对将要发布的 snapshot 重新计算绑定 policy；若结果含不可发布的 `block` 或 `review`，返回 409-style diagnostic，且不创建 partial publication
- 后续 observation 在既有 immutable publication 后发生变化时不得改变其内容；只有下一次 preview/publication 使用新 decision snapshot
Envoy 的 RBAC 模型允许 per-route 覆盖，[4] 提供了“policy scope 显式绑定而非全局隐式污染”的参考；本地设计把该原则用于 group/publication scope，而不是引入 Envoy。
```

### Plan Contract DOC-0213 <a id="plan-doc-0213"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:96`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D6 3+1 方案池和裁决

| 方案 | 来源与启发 | 价值 | 致命问题 | 裁决 |
| --- | --- | --- | --- | --- |
| A 供应商原始分数直通 | Scamalytics/IPQS/MaxMind 风险和匿名化输出[1][2][3] | 获得成熟 IP intelligence | 分数语义与阈值不可直接等同，可能泄露原始数据 | 仅保留为 versioned normalized evidence |
| B 网页探针或 scraper | 现成非官方开源抓取实现 [inference] | 无 API 接入表面成本低 | 403/反爬、契约漂移、条款与可用性不可保证，且难以安全审计 | 明确拒绝 |
| C 代理网关的 metadata/RBAC | Envoy 显式 policy scope 与 metadata 机制[4] | 强调策略绑定和解释性 | 引入 proxy sidecar 与跨系统控制面过度设计 | 借鉴 scoped policy，不引入 Envoy |
| D 本地融合 | CSP existing sing-box outbound + immutable observation + resolver snapshot | 能证明出口、隔离配额与把事实同决策分离 | 需要新增 schema/ports 和严格 provider 运营配置 | 采用 |
```

### Plan Contract DOC-0214 <a id="plan-doc-0214"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:105`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [商业 provider API 费用、配额或凭据不可用] → 默认禁用、预留预算、secret reference、明确 `unknown` 与不可无界重试
- [出口频繁变化导致缓存失效与请求放大] → exit identity digest 绑定、TTL、单节点排他、provider rate/budget；超过预算转 `unknown`
- [provider schema 或风险语义变更] → 每个 adapter schema version + fixture；contract drift 不发布 allow
- [完整 IP 或原始 payload 泄露到库、日志、UI] → contract 中根除字段、digest-only evidence、redaction tests、API contract negative tests
- [高风险误判造成意外节点排除] → policy 显式启用、默认 unknown=review、preview/preflight diagnostics、未绑定 group 不受影响
- [低风险漏判导致订阅发布] → 可启用 policy 的 `unknown=block`；发布前强制 preflight；但该更严格模式由操作者显式决定
- [SQLite read query 影响大台账分页] → index/filter plan、限页、按 current observation 的物化或 join contract 由 executor 基准验证；不在浏览器过滤
```

### Plan Contract DOC-0215 <a id="plan-doc-0215"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:115`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. 先在全新 SQLite fixture 增加 append-only risk evidence、provider settings/policy revision 和必要索引；验证 migration 不引用旧卷或旧表
2. 实现 fake provider 与 fake sing-box outbound fixture，验证环境代理不能替代 node outbound、API secret 不出任何层、TTL/cache/budget reason 可重放
3. 部署时保留所有 provider disabled；启动 readiness 只验证 schema 和配置结构，不因缺商业 key 失败
4. 授权操作者以 deployment secret reference 启用一个 provider、低预算与 `unknown=review` policy；以预览审阅结果后才把 policy 绑定到非生产 group
5. 经单独发布授权后再绑定 production group 或选用 `unknown=block`，并先运行 preflight
6. 回滚仅停用/解绑 risk policy 和 provider，不删 observation；既有 immutable publications 保持不变，重新发布前恢复既有未绑定 policy 的 snapshot
```

### Plan Contract DOC-0216 <a id="plan-doc-0216"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:124`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Open Questions

- [inference] 首个真实 provider 的账号、API edition、区域可用性与合同字段尚未取证；这些影响 adapter 的具体 credential header、endpoint 与 mapping fixture，但不改变本 change 的 provider port、脱敏、预算或 policy 结构。实施前必须由 executor 依据已授权 provider 的官方 API 文档补充 provider-specific contract，不能从网页页面猜测。
```

### Plan Contract DOC-0217 <a id="plan-doc-0217"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:128`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Sources

[1] https://scamalytics.com/ip
[2] https://www.ipqualityscore.com/documentation/proxy-detection-api/response-parameters
[3] https://support.maxmind.com/knowledge-base/articles/ip-anonymizer-risk-data-minfraud
[4] https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rbac_filter
[5] https://sing-box.sagernet.org/configuration/route/rule
```

### Plan Contract DOC-0218 <a id="plan-doc-0218"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/design.md:1`
- **Source Digest**: `e2e68390407d378eaf92613288be29bfed79db80bf40170e007ec4bd4609cfb9`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context & Architectural Intent

本项目为解决历史变更集众多但缺乏持续主干规范的问题，严格执行用户关于“重写SPC、审计符合度、汇总模糊项待我确认再落地”的最高指示。设计重点在于实现规范治理的双层解耦：将全局持久的 Living Specification（存放在 `openspec/specs/`）与局部的增量变更集（存放在 `openspec/changes/`）有效协同，同时融合业界 Spec-Kit 优先级场景范式与 DeepSeek 决策记录规范。
```

### Plan Contract DOC-0219 <a id="plan-doc-0219"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/design.md:7`
- **Source Digest**: `e2e68390407d378eaf92613288be29bfed79db80bf40170e007ec4bd4609cfb9`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 结构与格式融合设计

- **Spec-Kit 融入 OpenSpec**：Spec-Kit 具有清晰的 `User Scenarios & Testing (Priority: P1/P2/P3)`、`Functional Requirements` 与 `Success Criteria` 结构。我们将其完全吸纳进 OpenSpec 的 `## Requirements`、`### Requirement: ... [FR-ID]` 与 `#### Scenario: ...` 严格语法树中。
- **全局唯一 FR-ID 与 SC-ID**：为 6 大领域分配了 37 个稳定的功能需求编号（`FR-INV-xx`、`FR-POL-xx`、`FR-PROB-xx`、`FR-PUB-xx`、`FR-AUTH-xx`、`FR-UI-xx`），消除历史模糊引用的歧义。
```

### Plan Contract DOC-0220 <a id="plan-doc-0220"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/design.md:11`
- **Source Digest**: `e2e68390407d378eaf92613288be29bfed79db80bf40170e007ec4bd4609cfb9`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 证据分级与审计矩阵设计

- 严格贯彻 `pi-governance` 事实来源铁律，建立四级确权：
- `TIER-1: USER-VERBATIM`（最高）
- `TIER-2: HISTORICAL-CONTRACT`（高）
- `TIER-3: IMPLEMENTATION`（中）
- `TIER-4: EXPERIENCE`（参考）
- 审计矩阵覆盖 37 项稳定需求，清晰定性为 `符合` (26)、`不符合` (3)、`部分符合` (6)、`未验证` (0)、`待确认` (2)，每项均附带文件行号、实机测试命令（附真实通过用例数与收据哈希）与真实物理局限分析。
```

### Plan Contract DOC-0223 <a id="plan-doc-0223"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/design.md:55`
- **Source Digest**: `e2e68390407d378eaf92613288be29bfed79db80bf40170e007ec4bd4609cfb9`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 阶段与角色边界防线

- **本阶段严格受限**：仅完成文档工件编写与文档内部一致性自测。
- **审查与交付边界**：保留审查任务为未完成，供独立审查机（Reviewer）进行只读比对，审查全绿后呈报用户裁决。
```

### Plan Contract DOC-0224 <a id="plan-doc-0224"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/design.md:1`
- **Source Digest**: `22ccc93b242323f291aff2784eaf502ed8b3e2d35610b02bb69661cff9853865`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

1. **当前探针调度器缺乏“节点池（Node Probe Pool）”概念与去重/插队机制（`internal/probe/queue/scheduler.go`、`internal/application/probe/{runner,periodic,service}.go`）**：
- `internal/probe/queue/scheduler.go` 目前仅维护按 `RunID` 分桶的 `queues map[string][]Task` 与 `activeNodes map[string]activeRecord`（键为 `logicalID#kind`）。当同一节点与同一探测维度在活跃期或 `NodeTTL`（30 秒）内再次提交时，`s.submit` 直接返回 `ErrNodeConflict`。
- `internal/application/probe/periodic.go` 在 `executeBatch` 中查询全部活跃节点后，未检查节点是否已经在节点池中排队或正在检测，直接全部提交给 `DefaultRunner.Run`；若此时恰好有手动测速或上一批任务尚未结束，就会触发 `ErrNodeConflict` 导致批次/任务报错。
- 用户手动触发的单节点重测、已选节点测速或一键全量测速（`POST /api/v1/probes/runs`）与后台定时任务平权追加在 `runOrder` 队尾，既不能插到队列最前面优先执行，也无法将已在池中排队的节点提升（Promote）至队首。
2. **节点状态缺失“检测中（`probing`）”实时态与节点池 5 大统计指标接口（`internal/application/inventory/service.go`、`internal/transport/http/{nodes,probes}.go`）**：
- `inventory.NodeView`（`internal/application/inventory/service.go:96`）目前仅有静态 `health_status`（`healthy` / `degraded` / `unhealthy` / `unknown`），未关联节点池实时状态；当节点正在池中执行检测（`probing`）或排队等待（`queued`）时，`GET /api/v1/nodes` 与 `GET /api/v1/nodes/{logical_id}` 无法向前端透传 `probe_state` 与 `"probing"`（检测中）状态。
- 后端缺少统一的 `GET /api/v1/probes/pool` 接口来实时聚合用户要求的 5 项核心指标：**当前队列中的节点数**（`queue_nodes_count`）、**未测数**（`untested_count`）、**总数**（`total_count`）、**不可用数**（`unavailable_count`）、**可用数**（`available_count`）。
3. **前端探针引擎看板与节点状态展示需围绕“节点池 + 5 大核心指标 + 检测中状态”升级（`web/src/features/probes/**`、`web/src/features/nodes/**`）**：
- `web/src/features/probes/ProbesView.vue` 第一视觉层级需升级为直观、大气的**节点池与全网检测状态看板**，清晰展示「当前队列中的节点数」、「未测数」、「总数」、「不可用数」、「可用数」5 大指标及定时自动入池控制；
- `web/src/features/nodes/nodeView.ts`、`ProbesView.vue` 与 `NodesView.vue` 需支持 `probing`（检测中）与 `queued`（队列中）状态徽章、状态筛选及手动插队优先检测反馈。
```

### Plan Contract DOC-0227 <a id="plan-doc-0227"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/design.md:57`
- **Source Digest**: `22ccc93b242323f291aff2784eaf502ed8b3e2d35610b02bb69661cff9853865`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 节点“检测中（`probing`）”状态注入与 `GET /api/v1/probes/pool` 契约

1. **`inventory.NodeView` 扩展（`internal/application/inventory/service.go`）**：
- 新增 `NodePoolStateProvider` 接口（由 `*queue.Scheduler` 或 `*probe.Service` 实现）：
```go
type NodePoolStateProvider interface {
GetNodePoolState(logicalID string) string // returns "probing", "queued", or "idle"
}
```
- 通过 `inventory.WithNodePoolStateProvider(provider)` 注入 `inventory.Service`（在 `cmd/csp/main.go` 中将 `probeScheduler` 注入 `invService`）；
- 在 `NodeView` 与 `NodeDetail` 新增字段：
- `ProbeState string`（JSON: `"probe_state"`，取值 `"probing"` | `"queued"` | `"idle"`）；
- 在 `enrichNodeViews` 中：
- 默认设置 `views[i].ProbeState = "idle"`；
- 若 `s.poolProvider != nil`，查询 `state := s.poolProvider.GetNodePoolState(views[i].LogicalID)`，设置 `views[i].ProbeState = state`；
- 当 `state == "probing"` 时，将 `views[i].HealthStatus` 设为 `"probing"`（同时保留 `LatencyMS`、`LastProbedAt`、`Capabilities` 历史最新观测值，方便前端在展示「检测中」动态徽章的同时展示上次延迟参考）。
2. **节点池与全网检测状态统计 API：`GET /api/v1/probes/pool`（`internal/application/probe/service.go` & `internal/transport/http/probes.go`）**：
- 在 `probe.Service` 注入 `nodes domain.NodeRepository`、`observations domain.ProbeObservationRepository` 与 `scheduler *queue.Scheduler`；
- 新增 `probe.Service.GetPoolStatus(ctx context.Context) (*domain.ProbePoolStatus, error)`，计算并返回：
```json
{
"queue_nodes_count": 12,
"probing_count": 4,
"queued_waiting_count": 8,
"untested_count": 15,
"total_count": 120,
"unavailable_count": 18,
"available_count": 87,
"healthy_count": 75,
"degraded_count": 12,
"probing_node_ids": ["node_01", "node_02", "node_03", "node_04"],
"queued_node_ids": ["node_05", "node_06", "..."],
"updated_at": "2026-09-27T03:00:00Z"
}
```
- **5 大核心指标口径定义（全网活跃节点维度 + 实时节点池维度）**：
- `queue_nodes_count`：**当前队列中的节点数**（当前在节点池中的去重节点总数 = `probing_count` + `queued_waiting_count`）；
- `total_count`：**总数**（当前 `active = true` 的活跃节点总数）；
- `untested_count`：**未测数**（活跃节点中尚无任何 `probe_observations` 记录的节点数）；
- `unavailable_count`：**不可用数**（活跃节点中已探测且综合健康结论为 `unhealthy` 的节点数）；
- `available_count`：**可用数**（活跃节点中已探测且综合健康结论为 `healthy` 或 `degraded` 的节点数，满足 `available_count + unavailable_count + untested_count == total_count` 恒等式！）。
- 新增 `POST /api/v1/probes/schedule/trigger`：支持管理员或前端一键触发一次定时入池巡检（将当前不在池中的活跃节点按定时去重规则加入节点池队尾），并返回最新 `ProbePoolStatus`。
- 在 `POST /api/v1/probes/runs`（手动检测）中：当请求返回前，先把目标节点（或全量活跃节点）预注册/插队到节点池最前面，使紧随其后的 `GET /api/v1/probes/pool` 与 `GET /api/v1/nodes` 立即反映 `queue_nodes_count` 与 `probe_state = "probing" / "queued"`。
```

### Plan Contract DOC-0228 <a id="plan-doc-0228"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/design.md:102`
- **Source Digest**: `22ccc93b242323f291aff2784eaf502ed8b3e2d35610b02bb69661cff9853865`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0230 <a id="plan-doc-0230"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:1`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 proposal.md。只读取证：`domain.Node.Active` 是激活定义；`NodeFilter.ActiveOnly` 可查激活节点，`NodeSourceRepository.ListByNode` 存真实来源；`ProbeRun`/`ProbeObservation` 和 `/probes/runs` 已持久化，runner 支持空 nodeIDs 取激活节点、基线默认种类、`DefaultRunBudget.MaxTasks=512`、现有安全出站与队列；`cmd/csp/main.go` 负责探测队列 Drain/DB Close；`resolver.Resolve` 先做激活/准入/风险，再按显式边生成组；`publication.resolveSnapshot` 与 policy 预览调用关系需施工核对并统一。`matchesAdmissionExpression` 使用宽松字符串/正则回退及无效字段按名称匹配，不适合未经严格校验直接复用于新配置。历史 `PROCESS-NAME` 修复在提交 `7289b4c`；本次错误无请求日志，无法断言其根因。项目主规格目录当前为空，故本 Change 建立两个新能力规格。
```

### Plan Contract DOC-0231 <a id="plan-doc-0231"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:5`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:** 可恢复周期探测、跨实例单到期互斥、低成本可观察证据、全局先于组级的确定性筛选、老配置保持兼容。
**Non-Goals:** 自动访问生产环境、改写历史手动运行的权限、把第三方目标站可达性作为本地测试门禁、推断未提供的 CSP 错误。
```

### Plan Contract DOC-0232 <a id="plan-doc-0232"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:13`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 冻结公共契约和计划归属

在 `internal/domain` 新增 `ProbeSchedule{Enabled,IntervalSeconds,Kinds,NextDueAt,Generation,UpdatedAt}`、`ProbeBatch{ID,WindowAt,Generation,Owner,LeaseUntil,State,RunIDs,Counts,RedactedError,CreatedAt,UpdatedAt}` 与专用 `ProbeScheduleRepository`（原子 Compare-and-Swap 领取、心跳续约、完成、列表）；将全局 `NodeFilterSpec` 作为独立单例设置，将组 `NodeFilterSpec` 作为按 group_id 绑定持久化；其结构为 `Conditions []FilterCondition{Field,Op,Value,ProbeKind,FreshnessSeconds}`，仅 AND，同一字段多值可重复 OR 不提供，NOT 用专有受限操作。`ProbeObservation` 添加产生时节点 credential_version 元数据，旧行标识版本未知；判断版本匹配不通过未知行。选 SQLite 增量表而非新的配置中心/另一套调度依赖；仓库已有 `modernc.org/sqlite` 与显式事务，单一 DB 可用受条件 UPDATE + 唯一 `(generation,window_at)` 协调进程。多独立 DB 的跨主机 exactly-once 不保证，部署边界为共享同一受控 SQLite 库。
```

### Plan Contract DOC-0233 <a id="plan-doc-0233"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:17`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 周期执行算法与安全

默认关闭；管理员配置的周期和种类经上/下界和 runner 预算验证（产品初始建议 15 分钟至 24 小时；如用户明确不同周期另调整）。`cmd/csp/main.go` 启动从 DB 加载配置，用 `time.Timer` 与 `context` 等待最近到期；不引入私有 cron 或第二队列。原子占位先推进 `next_due_at`（停机后采用 now+interval 跳过过去窗口，防止补发风暴），批次唯一；租约有过期恢复，续约与领用必须在同事务按 generation/owner fencing，旧 owner 丢租约后不得再写状态或创建运行。运行 actor_scope 固定 `system:periodic-probe`，幂等键源于 generation/window/chunk，使用现有 `ProbeRun`/`DefaultRunner`/`queue.Scheduler`，不经 HTTP 模拟手动调用；批次与运行关联在派发前持久化，每次从当前 `ActiveOnly` 完整分页或显式无分页查询取快照，逻辑 ID 去重排序，执行前再校验 active/credential_version 与解密是否可用，绝不在日志或 API 中打印密钥；设置运行期限与节点×种类任务预算，分片均衡以覆盖大库存，不允许静默截断。手动任务和周期任务共享现有有界 queue 公平机制；周期不得占满全部待执行容量，队列压力时让位/延后并记载未覆盖数。检测到无有效凭据时每节点记录脱敏跳过，禁止安全出站退化；计划关闭取消后续批次并根据 UI 的“取消本批次”显式取消运行。启动先恢复遗留 queued/running 运行与批次：截至 deadline/失租约终结为 expired/cancelled，禁止不完整运行数据无条件重放；尚未到期且可安全重放的片段只凭关联运行 ID 幂等恢复。停服顺序：停止 timer/续约与新派发 → 取消/等待周期运行 → 共用 Scheduler.Drain → DB Close；Drain 失败保持现有不关闭 DB 安全门禁。比较方案：单进程锁不足多实例，cron 调度器仍需分布式锁与恢复，故选择现有 timer+SQLite 原子条件抢占。
```

### Plan Contract DOC-0234 <a id="plan-doc-0234"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:21`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0235 <a id="plan-doc-0235"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:25`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 解析边、快照与发布

读取同一次解析所需节点、来源、观测、全局/组级筛选及 group/rules：事务一致读或稳定快照并证明无竞态，注入 `ResolveInput{AsOf,GlobalFilter,GroupFilters,NodeSources,LatestObservations}`；已有风险拒绝和准入先执行，全局过滤建立唯一 admitted 集，组在此基础上按自身条件筛选显式成员；只有设置非空组条件且没有节点边的组会从全局集合自动匹配，按 `DisplayName ASC,LogicalID ASC` 附加，组边排序保持原序；子组边保持拓扑，在父组展开时再应用父组条件，防止子组绕过。`AllNodeLogicalIDs` 与编译所用 `Members` 必须对应：若父组过滤会使直接 child-group 边暗含被拒绝节点，采用在解析快照中为该父组展开的受限子组成员/可编译投影，或拒绝该种无法无损表达的嵌套配置；不得只改变统计 ID 而保留可绕过的子组边。推荐对父/子存在不同筛选的场景直接拒绝并给出 `unrepresentable_nested_filter` 字段级诊断，直到真正实现可编译投影；规格要求有效嵌套筛选必须可以表达，故交付前必须完成投影方案，不得长期以拒绝替代。决策：解析器形成不可变的 per-group 展开投影，由 compiler 只引用投影中的节点；子组如果被多个父组引用且父过滤不同，生成稳定的父上下文派生子组名称/ID（摘要来源于父组 ID、子组 ID、过滤条件）并保证各目标渲染均支持，UI 同时保留原策略图引用。预览、预检、发布共用单一 `resolveSnapshot` 装载函数/输入摘要，涵盖筛选定义版本、来源集合、观测 ID/版本/时间及 AsOf 新鲜度区间；快照摘要不纳入无关的当前墙钟时间，但跨越 freshness 边界必须变化。过滤为空的老组仍保持已有编译行为；新筛选导致被路由引用组空集必须 fail-closed 且有目标组与过滤层诊断。
```

### Plan Contract DOC-0236 <a id="plan-doc-0236"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:29`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. API/UI 与失败根因取证

探测计划：`GET/PUT /api/v1/probes/schedule`、`GET /api/v1/probes/batches`、`POST /api/v1/probes/batches/{id}/cancel`；全局筛选：`GET/PUT /api/v1/policies/global-node-filter`（在 router 的已认证 admin 范围内，实际公共路由前缀以 router 注册为准）；现有组 CRUD 追加可选 `node_filter`。UI 在探测页呈现启用/周期/种类/批次/状态，在策略页呈现全局/组级条件及预览数量、排除原因和空组阻断，不在浏览器实现另一套筛选语义。对于用户报告的“csp出错”：先只读复核当前编译目标能力与提交 `7289b4c`；需从用户获取本次 URL/时间、请求 ID、脱敏错误正文与版本/容器日志才可诊断新错误；不从线上 DB/密钥/日志取证。既有 `PROCESS-NAME` 未支持假说已由提交排除，手动探测不存在假说由 `http/probes.go` 排除，周期配置已存在假说由现有 `settings.go` 排除；其余当前错误根因未知，不将未知错误纳入伪修复任务。
```

### Plan Contract DOC-0237 <a id="plan-doc-0237"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:33`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [SQLite 多实例租约无法保护不同数据库实例] → 明确共享库前提，CAS/唯一索引/fencing 与双连接竞争测试。
- [观测缺版本导致误筛] → 迁移旧观测为 unknown；新写入记录版本，缺失 fail-closed。
- [自动探测出站负载或队列饥饿] → 默认关、界限配置、切片预算、队列共用公平性、压测与取消。
- [跨父上下文派生子组使目标名称冲突/编译不一致] → compiler 全目标测试名称唯一及端到端结果；若不可实现须先向用户升级设计，不能降级跳过嵌套需求。
- [探测状态随时间变而快照 digest 陈旧] → 截止时间边界纳入 digest；预览/发布选取同一冻结输入再解析。
- [现有 Vite closeBundle 写 `internal/webassets/dist`] → 原仓不运行默认 `npm build`；只用 no-copy 隔离副本或事先可证不会拷贝的构建配置，并核对 dist 基线。
```

### Plan Contract DOC-0238 <a id="plan-doc-0238"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:42`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. 本地隔离 DB 备份后应用新增 SQLite 顺序迁移：计划/批次/运行关联/全局组筛选存储及索引，旧 plan 默认 disabled，旧 filter 默认恒真，旧观测 credential_version=unknown；验证旧 DB 升级与全新 DB 同步。
2. 启动顺序先 migration 后装载计划；上线前复验隔离副本里的迁移、重启恢复及旧配置编译输出，生产部署必须另行授权并先备份数据库。
3. 回滚应用可停用周期计划和清空过滤条件恢复旧行为；若需要回滚二进制，则先排空计划并恢复迁移前备份（现有迁移器不支持向下回滚），禁止在旧二进制仍运行时擅自改写已迁移生产数据库。
```

### Plan Contract DOC-0239 <a id="plan-doc-0239"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:1`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参考 `proposal.md`。
在当前 CSP 架构中：
1. **策略解析与发布截断**：`publication/service.go:812` 与 `policy/service.go:932` 在构造 Resolver 输入时均调用 `s.nodeRepo.List(ctx, domain.NodeFilter{ Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, ExcludeNotices: true })`。由于未传入 `Pagination` 结构，SQLite 实现 `internal/repository/sqlite/nodes.go:114` 命中默认逻辑 `pageSize = 50`。当活跃节点池达到 90 个节点时，加拿大（第 78-80 位）与台湾（第 70-72 位）节点被物理截断，导致策略组计算出空组，用户误以为是旧快照；
2. **成员判定语义**：Resolver 原生算法根据是否有边严格分流：若存在显式边，则以边为候选并叠加组筛选；仅在边集合完全为空且非空 `node_filter` 时动态扫描全池。但前端界面未提供模式清晰引导，致使操作者困惑筛选与边是否冲突；
3. **探针健康状态与互斥守恒**：`nodeView.ts` 存在将非 baseline（如 AI）的 `available` 判定危险回退为整体 `healthy` 的逻辑；同时 `BaselineUnknown` 被简单归为 `unprobed` / `UntestedCount`，导致「AI 测过可用却显示未检测」；缺乏 `undetermined` 状态，池统计未保持互斥守恒；
4. **前端分页缺失与视图布局缺陷**：`usePolicy` 仅加载单页 100 条组；`PolicyEditorSheet` 传递 `page_size=200` 被后端硬截断至 100；`useProbes` 采用 `while (page <= 20)` 瀑布流并发拖慢性能；`App.vue` 侧边栏未设粘性定位，导致页面滚动时导航消失。
```

### Plan Contract DOC-0240 <a id="plan-doc-0240"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:10`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- **消除全量截断**：`NodeRepository` 新增专用全量读取接口 `ListAll`，在 `policy` 校验与 `publication` 导出中显式全量拉取，保留全部过滤准入条件与 IP 风险控制；测试用例覆盖 250+ 节点（目标节点排在 100 之后）；
- **保留原有代数语义**：澄清「显式连接边 + 条件过滤」与「零边全池动态规则」的双轨模式，不破坏组合运算，不自动删除生产数据中的任何边或 filter；
- **探针健康权威与守恒**：删除辅助能力回退 `healthy` 的危险逻辑；整体健康必须且仅依赖新鲜当前 revision baseline；新增 `undetermined_count` 与 `undetermined` 分类；保持 `total_count = healthy + degraded + unavailable + undetermined + untested` 守恒；后端提供全 Scope 数据库级健康筛选；
- **前端标准分页与异步检索**：策略组列表支持服务端分页与搜索；候选成员选择支持异步检索与分页；`useProbes` 废除 20 页瀑布流；拓扑全图解耦独立展现；
- **导航粘性与移动端体验**：桌面侧栏 `sticky top-0 h-screen` 固定；移动端 Modal/Drawer 弹出时锁定 `body` 滚动；经 4 视口回归验证；
- **独立工程写集合**：Backend 包（Go/migrations）与 Frontend 包（web）写集合严格物理正交。
**Non-Goals:**
- 不修改 `/api/v1/nodes` 针对 UI 列表的默认分页上限（保持默认 50，最大 100），防止无界内存膨胀；
- “批量”严格限定为分批加载与分页（chunked loading / pagination），不增加节点或策略组的业务批量启停、批量增删操作；
- 避免全量外网探测依赖，单测与集成测试均采用本地合成 Fixture。
```

### Plan Contract DOC-0241 <a id="plan-doc-0241"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:27`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 全量节点查询接口 (`ListAll`) vs 盲改 UI 分页上限

- **决策**：在 `internal/domain/ports.go` 中为 `NodeRepository` 显式增加 `ListAll(ctx context.Context, filter NodeFilter) ([]Node, error)`，在 `sqlite.nodeRepository` 中实现专用的无分页上限读取通道。`publication/service.go` 与 `policy/service.go` 改用 `ListAll`；
- **理由**：UI 列表查询必须有上限（最大 100）以保护网络传输与浏览器内存；而编译器/校验器属于单次全量内存计算，必须获得全量已准入活跃节点；
- **替代方案考虑**：若直接将 `nodeRepo.List` 的 `MaxPageSize` 改大（例如 10000），会导致前端请求不规范时可能拉爆服务器内存，破坏 API 保护边界。
```

### Plan Contract DOC-0242 <a id="plan-doc-0242"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:32`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 策略组成员判定原语义保留与界面交互重梳

- **决策**：严格保留 `internal/resolver/graph.go` 的代数运算规则。在 `PolicyEditorSheet.vue` 中重构交互：
- 显示模式指示徽章：
- 当配置了连接边时：标为「显式连接边模式（组筛选仅作为入选边之二次过滤）」；
- 当未配置连接边且配置了筛选条件时：标为「全池动态匹配模式（自动遍历全局活跃节点）」；
- 当连接边包含子策略组时：标为「子策略组级联模式（父级筛选条件向下级递归继承）」；
- 增加清晰的警示说明，不执行任何自动静默删除边或筛选条件的破坏性操作。
```

### Plan Contract DOC-0243 <a id="plan-doc-0243"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:40`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0244 <a id="plan-doc-0244"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:51`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4.1 策略组列表 (`GET /api/v1/policies/groups`)

- **请求参数**：
- `page`: 整数，默认 1
- `page_size`: 整数，默认 50，最大 100
- `search`: 字符串，可选，针对 `name` 与 `group_type` 进行不区分大小写匹配
- **响应体**：标准 Paginated Envelope：
```json
{
"code": "success",
"data": {
"items": [ ...GroupView ],
"page": 1,
"page_size": 50,
"total": 42
}
}
```
```

### Plan Contract DOC-0245 <a id="plan-doc-0245"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:69`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4.2 节点列表 (`GET /api/v1/nodes`)

- **请求参数**：
- `page`: 整数，默认 1
- `page_size`: 整数，默认 50，最大 100
- `scope`: `enabled_subscriptions` | `active_subscriptions` | `all`，默认 `enabled_subscriptions`
- `active_only`: 布尔值，可选
- `search`: 字符串，可选（匹配名称/服务器）
- `health_status`: 字符串，可选，逗号分隔支持 `healthy,degraded,unhealthy,undetermined,untested`
- `protocol`: 字符串，可选
- **响应体**：标准 Paginated Envelope。
```

### Plan Contract DOC-0246 <a id="plan-doc-0246"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:80`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4.3 探针池指标 (`GET /api/v1/probes/pool`)

- **响应体**：
```json
{
"code": "success",
"data": {
"total_count": 90,
"available_count": 45,
"healthy_count": 40,
"degraded_count": 5,
"unavailable_count": 15,
"undetermined_count": 20,
"untested_count": 10,
"queue_nodes_count": 2,
"probing_nodes_count": 1
}
}
```
- **守恒校验公式**：
`total_count === healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count`
`available_count === healthy_count + degraded_count`
```

### Plan Contract DOC-0247 <a id="plan-doc-0247"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:102`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 前端导航固定与多视口滚动隔离

- **桌面端**：`App.vue` 中的 `<aside>` 容器设为 `sticky top-0 h-screen shrink-0 overflow-y-auto`，无论主内容区域如何纵向滚动，侧栏永久吸顶可见；
- **移动端**：在弹出 Bottom Sheet（如 `PolicyEditorSheet`）、ModalDialog 或 Drawer 时，通过 Vue composition hook (`useBodyScrollLock`) 为 `document.body` 添加 `overflow: hidden; touch-action: none;`，关闭时释放，彻底消除双重滚动穿透；
- **四视口适配测试**：在 `web/e2e` 中配置 Desktop 1280x800、Mobile 375x667、Mobile Large 392x872、Landscape 667x375 自动化回归校验。
```

### Plan Contract DOC-0248 <a id="plan-doc-0248"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:107`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk] 250+ 节点全量读取可能轻微增加一次性内存消耗** → **Mitigation**: 限制于 `publication` 与 `policy` 单次计算，只拉取当前启用的活跃节点，不持久持有，垃圾回收可立即回收；
- **[Risk] `undetermined_count` 引入打破旧版前端仅识别 3 种状态的假设** → **Mitigation**: 在 `nodeView.ts` 与 `probesView.vue` 中同步更新 KPI 卡片、进度条和过滤器，保证守恒公式总和为 100%；
- **[Risk] 异步候选节点搜索可能因网络抖动引起多次请求** → **Mitigation**: 在候选搜索输入框接入防抖（debounce 300ms）并保留当前选中项的状态不被覆盖。
```

### Plan Contract DOC-0249 <a id="plan-doc-0249"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:1`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md` - Why 与 What Changes。
当前系统在生产运行中暴露了三个层面的实现缺陷：
1. **订阅合并与节点生命周期**：`internal/application/inventory/service.go:297-348` 在拉取或解析错误时提前返回并中止事务，且在 `service.go:459-476` 中使用了破坏性的全局孤儿失活 SQL：
```sql
UPDATE nodes SET active = 0, updated_at = ?
WHERE active = 1 AND NOT EXISTS (SELECT 1 FROM node_sources WHERE node_sources.node_logical_id = nodes.logical_id);
```
该语句不仅在外部订阅拉取失败或返回空时直接使全部节点失活，而且抹杀了未绑定 node_sources 的手工导入节点；同时 `internal/domain/id.go:82-120` 生成的 `logical_id` 与连接版本管理缺乏在订阅作用域内的安全匹配机制，导致连接配置发生真实变更时未能正确自增 `connection_revision` 或虚假沿用历史 healthy 状态。
2. **探针出站构建器与安全拨号器**：`internal/probe/singbox/builder.go:149/164/199/262/289` 直接调用 `networkName(config)`，将传输层名称（如 `ws`、`grpc`）当作 L4 网络协议写入 `Outbound.Network`，并强制默认为单一 `tcp`，导致 UDP 探测能力失效；Reality 参数处理未对 `pbk` 实施严格的 32 字节校验，对 `sid`（Short ID）存在奇数位未拒绝的潜在隐患；`internal/application/probe/safe_dialer.go:179-183` 在捕获底层客户端构建失败时直接丢弃了根因并返回泛化的 `ErrClientBuildFailed`，且未严格解耦本地构建成功与远端网络握手。
3. **接口业务语义与交付门禁**：控制面现有 45+ 个路由端点缺乏端到端副作用闭环测试，部分测试仅做 HTTP 200 断言，未能验证数据库持久化、幂等性、并发冲突及权限边界。
```

### Plan Contract DOC-0250 <a id="plan-doc-0250"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:14`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 在 `internal/application/inventory/service.go` 中建立非破坏性 Reconcile 状态机：拉取/解析失败、空列表、部分拒绝时全量保留既有活跃节点，标记来源状态为 stale/failed；
- 彻底废除全局孤儿节点批量失活 SQL，将来源 prune 严格限定在当前操作的 `subscription_id` 范围内，保护手工节点与其他来源节点不受波及；
- 在同一订阅来源作用域内实现稳定节点身份匹配与连接版本（`connection_revision`）单调递增管理，重名/歧义保守保留并新增，严禁跨来源合并凭据；单纯显示名/JSON 键排序变更不升 revision；连接真实变更时重置当前观测为 unknown/pending；
- 制定并演练针对 2026-09-29 等历史误伤节点的证据化、幂等、可回滚的数据恢复脚本，严禁全量粗暴激活，保护手工禁用状态；
- 在 `internal/probe/singbox/builder.go` 中严格解耦 L4 协议（`tcp`/`udp`）与传输层（`ws`/`grpc`/`http`），完整支持 UDP 协议出站；
- 对 Reality `pbk` 实现 32 字节解码校验与 Base64 标准化；对 `sid` 实施奇数位与非法字符严格拒绝；测证实际 Sing-box 库对 `xhttp` 的构建能力；
- 改造 `safe_dialer.go`：捕获底层构建错误并提供脱敏结构化诊断，严禁凭据/URL query/公钥私钥泄露，维持 SSRF/私网 IP/TLS 安全拦截；解耦本地构建通过与远端握手健康；
- 建立前后端双向核对清单，新增隔离全接口语义测试套件，全面覆盖 45+ 路由的数据库副作用、幂等性、并发及权限拦截。
**Non-Goals:**
- 不修改底层 Mihomo 编译器规则匹配核心或 PDD 引擎；
- 不重启或修改宿主机 Hermes 内核与外部网关服务；
- 不在 OpenSpec 规划与设计阶段直接编写或修改生产业务代码；
- 不改动外部订阅凭据本身失效或无权限的客观事实；
- 不在未经过 DB 备份与测试副本演练前直接对生产数据库执行数据变更。
```

### Plan Contract DOC-0251 <a id="plan-doc-0251"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:35`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 非破坏性订阅协调状态机（Non-Destructive Reconcile Engine）

- **设计选择**：改造 `internal/application/inventory/service.go` 中的 `ReconcileSubscription`：
- **失败安全分支**：当 `fetcher.Fetch` 失败或 `parser.ExtractWithCredentials` 报错时，记录 `domain.SubscriptionFetch` 审计事件并将订阅状态置为 `FetchOutcomeFailed`，但**立即终止后续 prune 操作**，保留该订阅关联的所有活跃节点与策略边；
- **来源修剪严格局部化**：废除全局 `deactivateOrphansSQL`。来源修剪语句改造为：
```sql
DELETE FROM node_sources WHERE subscription_id = ? AND last_seen_fetch_id != ?;
```
对于因本订阅更新而在当前订阅中不再出现的节点，系统仅解除该节点与当前订阅在 `node_sources` 中的绑定关系；只有当某个节点在 `node_sources` 中已无任何订阅绑定、且该节点被明确标识为由订阅自动生成（而非手工创建）时，才将其局部标记为失活，严禁跨表全量更新无关联节点；
- **空提取保护**：若订阅提取成功但有效节点数为 0，系统将其视为异常或源端清空保护场景，记录警告审计，默认不清理既有节点，除非用户在 API 中显式指定清空选项。
```

### Plan Contract DOC-0252 <a id="plan-doc-0252"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:45`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 来源作用域内节点身份稳定匹配与连接版本控制

- **设计选择**：
- **匹配优先级**：在同一 `subscription_id` 范围内，若节点提取项包含源端唯一标识（如源端提供的 ID/UUID），以该标识优先匹配；若无标识，则以 `(normalized_name, protocol, server, port, transport_summary)` 作为唯一性键匹配；
- **歧义处理**：若同一来源内存在重名或参数冲突，采取保守策略：保留既有节点不变，为新节点分配独立逻辑身份，严禁覆盖；绝对严禁跨不同订阅按显示名合并节点；
- **连接版本（`connection_revision`）递增逻辑**：
- 对比新旧配置的真实连接要素（`server`、`port`、`protocol`、关键传输密钥、加密方式、混淆密码）；
- 若连接要素实质变更：递增 `connection_revision`（`revision = revision + 1`），更新节点配置，并将该节点的当前探测健康态标记为 `unknown/pending`，不复用旧版本的探测结论；
- 若仅为 `display_name` 变更或 JSON 字段格式化/排序变化：仅更新显示元数据，`connection_revision` 保持不变，既有健康状态继续保留。
```

### Plan Contract DOC-0253 <a id="plan-doc-0253"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:54`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0254 <a id="plan-doc-0254"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:63`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 探针出站构建器 L4 协议解耦与 Reality 参数规范化

- **设计选择**：
- **L4 与 Transport 解耦**：重构 `builder.go` 中的 `networkName` 及各个 `build*Outbound` 函数：
- `Outbound.Network` 仅包含合法 L4 协议（`tcp`、`udp` 或 `badoption.Listable[string]{"tcp", "udp"}`）；
- Transport 层（如 `ws`、`grpc`、`http`）配置在对应的 `V2RayTransportOptions` 中，不得将 transport 字符串写入 Network 字段；
- 对于 Shadowsocks、VMess、VLESS、Trojan 等协议，若节点配置中声明了 UDP 转发或传输支持，正确生成支持 UDP 的出站配置；对于 Hysteria2、TUIC 等基于 UDP 的协议，确保 Network 包含 `udp`；
- **Reality 参数校验**：
- `RealityPublicKey`：先验证非空，尝试标准 Base64 解码，若失败则尝试 URL-Safe Base64 解码；解码后字节数必须严格等于 32 字节，否则返回 `domain.NewValidationError("invalid_reality_public_key", "Reality public key must decode to exactly 32 bytes")`；最后统一转为 Sing-box 期望的编码格式；
- `RealityShortID`：验证其为纯十六进制字符串（`[0-9a-fA-F]*`），且长度必须为偶数（`len % 2 == 0`）且不超过 16 字节（32 个十六进制字符）。若包含奇数长度或非法字符，直接返回 `domain.NewValidationError("invalid_reality_short_id", "Reality short ID must be even-length hex string")`，严禁截断或补零；
- **Sing-box 库能力真实测证**：针对底层核心库实际支持的协议选项编写独立单元测试（`builder_singbox_test.go`），对 `xhttp` 等特性以实际构建是否报错为依据判定支持情况。
```

### Plan Contract DOC-0255 <a id="plan-doc-0255"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:74`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 安全拨号器脱敏结构化诊断与握手健康分离

- **设计选择**：
- **错误分类捕获**：在 `safe_dialer.go` 中，调用 `clientFactory(ctx, cfg, httpOpts)` 时捕获底层返回的原始错误，利用类型判断与模式匹配将其分类为：`ConfigParseError`、`TLSConfigError`、`TransportUnsupportedError`、`InternalEngineError` 等；
- **脱敏处理**：将原始错误字符串输入 `domain.RedactSensitiveInfo` 进行敏感信息脱敏（剥除密码、UUID、Query 参数、密钥），包装为可读的结构化错误返回，确保日志与 API 输出既具备可诊断性又绝对不泄露机密；
- **安全边界维持**：SSRF 校验、私网 IP 过滤（`cleanHost` 校验与私网地址拦截）以及证书校验逻辑保持最严水位，不得为了连通而放宽；
- **状态分离**：在探测观测模型中，将“本地配置构建完成”与“远程网络探测成功”明确解耦，本地构建成功后方进入网络阶段，网络阶段超时或重定向错误绝不伪造为 healthy。
```

### Plan Contract DOC-0256 <a id="plan-doc-0256"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:81`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 6. 前后端路由双向核对与全接口语义测试矩阵

- **设计选择**：
- **双向清单核对**：编写自动化测试 `test/e2e/route_inventory_test.go`，静态扫描前端代码中的所有 API 请求调用路径与后端 Chi 路由树，建立动态对齐清单，覆盖全部 45+ 路由；
- **全接口语义测试矩阵**：在 `test/e2e/api_semantic_matrix_test.go` 中针对每个路由组建立端到端测试用例：
1. **Auth 组**：`/api/v1/auth/status`、`/login`、`/logout`，校验 Session 与 Cookie 状态；
2. **Settings 组**：`/api/v1/settings/auth`、`/settings/admin-token`，校验双开关状态持久化与密码验证；
3. **Subscriptions 组**：增删改查、`/refresh`，校验数据库新增与变更、非破坏性合并；
4. **Nodes 组**：查询列表、详情、连接补丁，校验 `connection_revision` 变更；
5. **Probes 组**：`/runs`、`/pool`、`/schedule`、`/cancel`、`/batches`，校验状态机与幂等重放；
6. **Policies 组**：全局节点过滤规则查询与变更，校验审计事件；
7. **Publications 组**：发布创建、预检、预览、导出端点（`/publish/v1/{id}` 与 `/p/{id}`）、撤销；
8. **System 组**：`/healthz`、`/readyz`、历史路径 `/yaml` 与 `/script` 410 拦截。
- **断言标准**：每个用例不仅验证 HTTP 状态码，还直接查询 SQLite 验证数据库副作用与数据完整性。
```

### Plan Contract DOC-0257 <a id="plan-doc-0257"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:95`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk 1: 外部订阅源永久失效导致陈旧节点长期积压]** → 缓解措施：在订阅元数据中明确标识 `stale: true` 与最后成功刷新时间戳，前端直观标红提示，并提供显式“清空该订阅节点”的管理操作，由管理员自主决策是否清除。
- **[Risk 2: 误伤数据恢复脚本误恢复了用户主动停用的节点]** → 缓解措施：严格依据审计日志排查是否由用户手动修改，恢复脚本在临时测试数据库上演练输出 diff 报告，且恢复事务具备反向回滚 SQL 脚本。
- **[Risk 3: Reality 参数严格校验导致部分历史不规范订阅节点在构建阶段被拒]** → 缓解措施：在脱敏诊断中明确返回 `invalid_reality_short_id` 或 `invalid_reality_public_key`，便于排查源端配置，符合协议规范与安全边界。
- **[Risk 4: 45+ 接口全量语义测试执行耗时增加]** → 缓解措施：测试采用内存 SQLite（`:memory:`）或独立隔离临时文件库，用例间数据完全隔离且执行速度控制在秒级。
```

### Plan Contract DOC-0258 <a id="plan-doc-0258"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:102`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. **备份与环境确认**：执行当前 SQLite 数据库文件与配置文件的时间戳快照备份，验证备份文件大小与完整性；
2. **测试副本演练**：在独立的测试数据库副本上运行历史误伤节点精准恢复脚本，核对恢复节点数量与状态字段；
3. **本地静态编译与单元测试门禁**：后端执行 `go build ./...` 与 `go test ./...`，前端执行 `npm run type-check`，全部确保退出码为 0；
4. **独立 Reviewer 审查与 Critic 验收**：代码审查 PASS 后，启动隔离容器验证健康端点与全接口业务，Critic 执行带数据界面的无头浏览器截屏验收；
5. **受控上线与可回滚目标**：保留当前运行版本镜像作为回滚目标，仅当所有前置门禁全部通过后由主脑执行受控上线。
```

### Plan Contract DOC-0259 <a id="plan-doc-0259"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:1`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

1. **后端 `NodeView` 探测状态与订阅源读模型断线根因（`internal/application/inventory/service.go` & `internal/transport/http/nodes.go`）**：
- 在 `cmd/csp/main.go:188-191` 中，`inventory.NewService` 已经注入了 `inventory.WithProbeObservationRepository(probeObsRepo)` 与 `nodeSourceRepo`；
- 且 `internal/repository/sqlite/probes.go` 已实现 `ListLatestByNodes(ctx, nodeLogicalIDs, kinds)`，`internal/repository/sqlite/nodes.go` 已实现 `ListByNodes(ctx, logicalIDs)`；
- 但 `internal/application/inventory/service.go` 的 `NodeView` 结构体（第 39-50 行）仅包含基础字段与 `IPRiskSummary`，`ListNodesReadModel`（第 386-392 行）和 `GetNodeDetailWithRisk`（第 352-378 行）完全没有调用 `s.probeObsRepo.ListLatestByNodes` 与 `s.sources.ListByNodes`；
- 结果导致 `GET /api/v1/nodes` 与 `GET /api/v1/nodes/{logical_id}` 返回的节点 JSON 永远不包含任何探针结果（无延迟、无健康状态、无流媒体/AI/地区能力矩阵、列表中也无订阅源归属），前端拿到后只能将全部节点显示为“未探测”。
2. **后端手动探测全量截断、512 预算报错与 `speed` 硬编码报错根因（`internal/application/probe/runner.go`）**：
- **全量测速仅取第一页 50 个节点**：`runner.go:191` 在 `len(nodeIDs) == 0` 时直接调用 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true})`，而 `sqlite.nodeRepository.List` 在未传 `Pagination` 时默认 `page=1, pageSize=50`（区别于 `periodic.go:414-431` 按页循环拉取 100 条直至 `total`）；
- **512 任务预算直接打崩多维全量测速**：`runner.go:52` 定义 `DefaultRunBudget = RunBudget{MaxTasks: 512, ...}`，`runner.go:214-219` 在 `len(targetNodes) * len(validKinds) > r.budget.MaxTasks` 时直接将 `ProbeRun` 置为 `failed` 并返回 `probe run task budget exceeded`。当用户对 100+ 活跃节点发起 5~6 个维度的全量测速时必现失败；
- **显式勾选 `speed` 仍报错**：`runner.go:555-573` 在 `kind == domain.ProbeKindSpeed` 时不经过 `r.dialer` 发起请求，而是直接构造零值 `profiles.Result{}`（`OptIn: false`）并固定返回 `ErrSpeedProbeOptInRequired`，导致用户勾选「带宽测速」后永远得到 `unknown (speed_opt_in_required)`。
3. **前端探针引擎反人类交互根因（`web/src/features/probes/**`）**：
- `ProbesView.vue` 以 `ProbeRun` 列表作为唯一主视图，卡片标题是 UUID，副标题是 `作用域：default · 修订：活跃版本`，用户无法在探针页面直接看到任何一个节点的名称、延迟毫秒数或解锁状态；
- 点击“发起探测”弹出模态框要求填写 `配置修订版本（可选）` 和 `截止超时（分钟）`，违背直觉；
- `ProbeEvidenceSheet.vue` 原样输出 `obs.redacted_summary`（如 `profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=42`）和内部 `node_logical_id` 哈希，没有关联节点显示名（`display_name`），也没有结构化人类可读展示。
```

### Plan Contract DOC-0260 <a id="plan-doc-0260"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:19`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 在 `inventory.Service` 与 `transport/http/nodes.go` 中完整打通 `NodeView` 的最新探测状态（`latency_ms`、`last_probed_at`、`health_status`、`probe_missing`、`probe_stale`、`capabilities`）与订阅来源（`sources`）；
- 在 `probe.DefaultRunner` 中支持全量活跃节点自动分页拉取、用户显式包含 `speed` 时开启 `OptIn: true` 执行真实测速、以及放宽默认任务预算（支持大批量节点按并发信号量受控执行，同时保留 `WithRunBudget` 显式注入自定义预算时的单测可测性）；
- 将 `web/src/features/probes/**` 彻底重构为以“人类直觉”和“节点”为核心的测速与可用性工作台（KPI 看板 + 一键测速与探测项胶囊工具栏 + 节点实时测速表格 + 预设周期定时配置与折叠历史 + 结构化人类可读测速详情抽屉）；
- 在 `web/src/features/nodes/**` 中同步联动展示节点真实延迟、健康度、流媒体/AI 解锁状态与单节点快速测速入口。
**Non-Goals:**
- 不改动底层 SQLite 表结构（复用现有 `probe_observations`、`probe_runs`、`probe_schedules`、`probe_batches`、`node_sources` 表与既有 Repository 接口）；
- 不触碰生产运行容器，所有验证均在本地测试与隔离预览实例中完成。
```

### Plan Contract DOC-0261 <a id="plan-doc-0261"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:33`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 后端 `NodeView` 探测状态与订阅源聚合设计（`internal/application/inventory/service.go` & `internal/transport/http/nodes.go`）

- **数据结构定义**：
- 在 `internal/application/inventory/service.go` 中定义节点单维能力状态结构体 `CapabilityStatus`：
```go
type CapabilityStatus struct {
Verdict    domain.ProbeVerdict `json:"verdict"`
LatencyMS  int64               `json:"latency_ms"`
ObservedAt time.Time           `json:"observed_at"`
Summary    string              `json:"summary,omitempty"`
Stale      bool                `json:"stale"`
}
```
- 在 `NodeView` 上新增字段：
* `LatencyMS *int64` (`json:"latency_ms,omitempty"`)
* `LastProbedAt *time.Time` (`json:"last_probed_at,omitempty"`)
* `HealthStatus string` (`json:"health_status,omitempty"`)
* `ProbeMissing bool` (`json:"probe_missing"`)
* `ProbeStale bool` (`json:"probe_stale"`)
* `Capabilities map[string]CapabilityStatus` (`json:"capabilities,omitempty"`)
* `Sources []domain.NodeSource` (`json:"sources,omitempty"`)
- 在 `NodeDetail` 上同步包含填充好探测状态与订阅源的 `NodeView`（或由 `Service.EnrichNodeView` / `ListNodesReadModel` / `GetNodeDetailWithRisk` 统一装配），并在 `internal/transport/http/nodes.go` 的 `get` 与 `patchConnection` 中直接使用已装配探测状态与 `sources` 的 `NodeView`，避免 `inventory.ToNodeView(detail.Node)` 丢失探测字段。
- **聚合与派生规则（保鲜阈值 `defaultProbeFreshnessTTL = 1 * time.Hour`）**：
- 批量提取当前页/详情节点的 `nodeIDs`；
- 当 `s.sources != nil` 时，调用 `s.sources.ListByNodes(ctx, nodeIDs)` 填充每个 `NodeView.Sources`（无记录时初始化为空切片 `[]domain.NodeSource{}`）；
- 当 `s.probeObsRepo != nil` 时，调用 `s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil)` 获取每个节点各 `ProbeKind` 的最新 `ProbeObservation`：
* **无任何观测记录**：`probe_missing = true`，`probe_stale = false`，`health_status = "unknown"`，`capabilities = map[string]CapabilityStatus{}`；
* **有观测记录**：
- 遍历该节点各 `ProbeKind` 的最新观测，计算 `stale := now.Sub(obs.ObservedAt) > time.Hour || obs.Verdict == domain.VerdictStale`，写入 `capabilities[string(kind)] = CapabilityStatus{Verdict: obs.Verdict, LatencyMS: obs.LatencyMS, ObservedAt: obs.ObservedAt, Summary: obs.RedactedSummary, Stale: stale}`；
- `last_probed_at` 取所有 `ProbeKind` 中最新的 `ObservedAt`；
- `probe_stale` 为 `now.Sub(*last_probed_at) > time.Hour`（或任一核心观测已过期）；
- `latency_ms` 优先取最新 `baseline` 观测的 `LatencyMS`（当 `LatencyMS > 0` 或 `Verdict == available/restricted`），若无 `baseline` 观测则取最新有效观测的 `LatencyMS`；
- `health_status` 优先基于最新 `baseline` 观测判定：
* `Verdict == domain.VerdictAvailable` -> `"healthy"`（若该观测 `stale` 则为 `"degraded"` 或保留 `"healthy"` 并标记 `probe_stale: true`，而 `Verdict == domain.VerdictRestricted` / `VerdictStale` -> `"degraded"`）；
* `Verdict == domain.VerdictError` -> `"unhealthy"`；
* 若尚无 `baseline` 观测但存在其他维度观测：存在 `error` 且无 `available` -> `"unhealthy"`；存在 `restricted` -> `"degraded"`；存在 `available` -> `"healthy"`；其余 -> `"unknown"`。
```

### Plan Contract DOC-0262 <a id="plan-doc-0262"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:69`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 后端 `probe.DefaultRunner` 全量分页、预算放宽与 `speed` Opt-In 修复（`internal/application/probe/runner.go`）

- **全量活跃节点分页拉取**：
- 当 `len(nodeIDs) == 0` 时，参照 `periodic.go:414-431` 循环分页调用 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true, Pagination: domain.Pagination{Page: page, PageSize: 100}})` 直至 `len(targetNodes) >= total || len(chunk) == 0`，确保全量测速覆盖全部活跃节点而非仅前 50 个。
- **显式 `speed` 探测 Opt-In 执行**：
- 当调用方传入的 `validKinds` 中包含 `domain.ProbeKindSpeed` 时，表示用户/调度器已显式勾选 `speed` 测速；
- 在 `executeTaskWithVerdict` 中移除原先 `if kind == domain.ProbeKindSpeed` 的无条件短路报错分支；对 `domain.ProbeKindSpeed` 正常通过 `r.dialer(ctx, node)` 建立代理连接并请求 `probeURLForKind(domain.ProbeKindSpeed)`（应用 `prof.SpeedBudget.Deadline` 与 `prof.SpeedBudget.MaxBytesPerRequest` 限流读取），将 `result.OptIn = true` 传给 `prof.Evaluate(result)`，并在 `RedactedSummary` 中记录 `bytes_read` 与 `throughput_kbps` 等摘要信息。
- 同步更新 `internal/application/probe/stage_gate_test.go` 中旧的 `TestStageGateSpeedRejectedBeforeDialOrHTTP`，验证显式请求 `ProbeKindSpeed` 时正常拨号、携带 `OptIn: true` 完成测速观测并受 `SpeedBudget` 约束。
- **单次运行任务预算放宽**：
- 将 `DefaultRunBudget.MaxTasks` 从 `512` 提升至 `65536`（底层实际并发度已由 `queue.Scheduler` 的 `DefaultConcurrency` 信号量严格限制，不会产生无界 goroutine 膨胀），从而支持上千活跃节点 × 6 维度的全量一键测速直接完成，同时保留 `WithRunBudget` 注入小预算时的边界测试行为。
```

### Plan Contract DOC-0263 <a id="plan-doc-0263"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:79`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0264 <a id="plan-doc-0264"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:113`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 节点台账页（`web/src/features/nodes/**`）联动增强

- `web/src/features/nodes/nodeView.ts`：
- 扩展 `NodeRecord` 与 `NormalizedNode` 支持 `latency_ms?: number | null`、`last_probed_at?: string | null`、`health_status?: 'healthy' | 'degraded' | 'unhealthy' | 'unknown' | 'missing'`，以及 `capabilities` 既支持旧的字符串枚举（兼容现有单测）又支持后端新返回的结构化对象 `{ verdict, latency_ms, observed_at, summary, stale }`；
- 新增 `formatNodeLatency(node)`、`formatRelativeTime(iso)` 等辅助函数；
- `web/src/features/nodes/NodesView.vue`：
- 在节点列表与详情抽屉中直接展示后端返回的真实健康状态徽章、延迟毫秒数（如 `42 ms`）、流媒体与 AI 解锁状态，并在节点详情抽屉底部或头部提供「⚡ 立即测速」按钮（直接调用 `/api/v1/probes/runs` 针对该节点发起测速并刷新节点详情）。
```

### Plan Contract DOC-0265 <a id="plan-doc-0265"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:120`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **`nodeView.ts` 前向与后向兼容**：
- 现有 `web/src/features/nodes/nodeView.test.ts` 中存在直接传入 `{ capabilities: { streaming: 'available', ai: 'restricted' } }` 的单测用例。`nodeView.ts` 在 `normalizeNode` 与 `nodeCapabilityLabel` 中同时兼容字符串值与 `{ verdict, latency_ms, observed_at, summary, stale }` 对象值，确保旧测试与新后端响应双双零报错通过。
- **`stage_gate_test.go` 带宽测速单测语义更新**：
- 原 `TestStageGateSpeedRejectedBeforeDialOrHTTP` 测试的是旧版硬编码拒绝 `speed` 的行为。在修复为“当 `run.Kinds` 显式包含 `domain.ProbeKindSpeed` 时即视为用户已 Opt-In”后，需同步更新 `stage_gate_test.go` 验证显式传入 `ProbeKindSpeed` 时正常执行拨号与 `OptIn: true` 评估，而 `profiles.Speed().Evaluate(profiles.Result{OptIn: false})` 单元测试继续验证未 Opt-In 时的契约保护。
```

### Plan Contract DOC-0266 <a id="plan-doc-0266"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:1`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md`。规划基于 dev HEAD `23b8af8` 的只读证据，不是生产网络复验：992 节点中 21 启用，最近 21 条 baseline 为 error；18 条 `credentials_unavailable` 不能据此判线路坏（含 6 条不安全 TLS 拒绝、12 条有 UUID 的 VLESS），1 条私有目标拒绝，2 条 `transport_error` 原因未确权。`safe_dialer.go` 把安全拒绝和客户端构建错误包成缺凭据；`builder.go` 的 VLESS 构建路径缺 Flow，而 `compiler_export.go` 有 Flow。`inventory/service.go` 与 `probe/service.go` 在 baseline 缺失/未知时从其他能力推整体状态，也缺乏可靠的旧观测与当前连接参数关联。`periodic.go` 已有批量观测查询、512 预算、5 分钟失败冷却、CAS 租约/队列去重，但扫描默认 10 分钟，手动 `TriggerImmediate` 会更新 `next_due_at`；前端刷新按钮只做 GET。
```

### Plan Contract DOC-0267 <a id="plan-doc-0267"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:5`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:** 冻结共享健康语义和出站安全边界；消除探针自身配置/归因误报；分钟状态巡检只派发增量待测；手动刷新复用同一路径且不重置自动时钟；测试使用假时钟/离线网络夹具。
**Non-Goals:** 不自动把当前 21 条历史 error 改写成成功；不宣称 2 条 SS 已证实故障；不因测试诉求放开 `skip_cert_verify` / 内网出站；不每分钟全量测速；本规划不做生产改动或部署。
```

### Plan Contract DOC-0268 <a id="plan-doc-0268"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:13`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 分类在写观测处，健康在共享读模型处

复用 `domain.ProbeObservation`、`RedactedSummary` 与现有 API 可选字段；在安全拨号器添加有类型的缺配置/安全拒绝/不支持与客户端构建失败错误，保持 `errors.Is` 分类；runner 在出站前和 HTTP 执行阶段分别写稳定的脱敏 reason（安全拒绝、构建失败、DNS、超时、传输、profile 不匹配）。安全拒绝继续零出站；不要把 `clientFactory` 的原始错误或服务器地址/密钥拼进摘要。`VerdictError` 对不可检测仍可用于原始观测，但健康分类必须按 reason 区分，不靠 error verdict 推论网络坏。兼容历史 `credentials_unavailable` 等模糊原因：缺乏可证实传输失败细节时 fail-unknown；`transport_error` 若无足够阶段证据同样未知，后续新观测应提供可核验分类。VLESS 运行时从 `NodeConfig` 正确传入支持的 vision Flow 并对照现有 `compiler_export`，保留 TLS/Reality/SNI pinning；不引入独立代理引擎。
新增单一纯函数健康判定包/模块（domain 或已有 application 公共层中无循环依赖位置，由施工确认）接收 baseline 观测、新鲜度、当前配置版本返回 `healthy` / `degraded` / `unhealthy` / `unknown` 与延迟是否可信；`applyProbeObservationsToView` 与 `GetPoolStatus` 都调用它。以现有 `defaultProbeFreshnessTTL`（目前 1 小时）作为一致的 baseline 展示新鲜度上限；周期计划的 `interval_seconds` 是调度到期阈值，不替代读模型新鲜度。被旧配置版本覆盖的结果即使年龄新也不参与健康判断。能力观测继续逐类显示，但不能回填整体延迟/健康；`untested_count` 暂作为已存在的未知/无法检测桶，UI 标签必须解释“未知/未确认”而不误称全部从未测。已有 API 字段、分组与订阅过滤仍保持兼容；不无意更改订阅导出条件。
**备选**：仅改前端红绿文字，不解决聚合读模型、误报原因和旧观测可信度，拒绝。
```

### Plan Contract DOC-0269 <a id="plan-doc-0269"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:21`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 连接变更采用持久的单调版本，观测关联生成版本

已核对 `Node` 的 `UpdatedAt`、`ProbeObservation` 和 `UpsertBatch`：抓取/改名可刷新 updated_at，而观测无连接版本。现有 run `ConfigRevision` 是运行级自由字符串，不能替代逐节点版本。故在新 SQLite migration 增加 `nodes.connection_revision`（初始值）及 `probe_observations.connection_revision`（历史可空）。`UpsertBatch`/`UpdateNode` 原子比较 protocol、server、port、规范化 credentials JSON/transport，仅连接字段实变才递增；显示名、来源元数据、观测写入不递增。停用→激活须唤醒增量评估（可维护激活版本或在再激活时提升连接版本）；激活→停用从活跃候选中移除。SQLite 版本单调防止 A→B→A 的签名回退失效；比较/递增与节点更新同一事务，写入观测时关联实际执行所读取的节点版本；API/传输字段若有必要添加仅可选兼容字段，秘密配置值不得在状态接口泄露。旧观测 NULL 版本只作历史能力证据，新版本节点不能将它误判为新鲜健康，首次计划扫描为其安排增量复测；初次接入测试版本差异在持久化快照、重启、多实例下成立。调度按观测关联版本与当前节点版本比较，与有效期共同决定待测；旧版本即使时间新也必须在 UI 标注未知，并继续受冷却/配额限制。
**备选**：纯内存签名丢失于重启/多实例；`updated_at`/最新观测作为变化信号会使每次抓取或测量反复自旋；无版本时不能对旧结果做可信度标注，均拒绝。迁移仅新增字段并兼容历史只读数据，回滚到旧二进制前保留数据库备份，禁止覆盖或删除历史观测。
```

### Plan Contract DOC-0270 <a id="plan-doc-0270"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:27`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0271 <a id="plan-doc-0271"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:33`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. DAG 与文件所有权

先冻结共享状态函数的签名、观测原因枚举、连接版本读写契约（特性包 A 的先决子阶段，非独立微派单）；A 实施完整探测编译/错误分类及版本记录（`internal/probe/singbox/**`、`internal/application/probe/{safe_dialer,runner}*`、`internal/domain/`、`internal/repository/sqlite/` 与迁移、对应离线测试）。A 完成后，B 整体健康视图/pool 统一（`internal/application/inventory/**`、`internal/application/probe/service.go` 的统计、共用状态规则，后端离线测试）；C 分钟扫描/手动后端（`internal/application/probe/periodic.go`、`service.go` 的 trigger、HTTP handler、配套测试）。B、C 都需要共享 `probe/service.go`，**必须串行**；D 在 C 后实施前端刷新/文字/交互与测试（`web/src/features/probes/**` 及有必要的节点台账/i18n）；B/C 自测完成后才做全量汇聚审查。不得以划分并行任务为由交叉改写共享文件；每个包包含实现 + 自测，不独立派微任务。
```

### Plan Contract DOC-0272 <a id="plan-doc-0272"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:37`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [Risk] 老观测缺少版本，部分节点短时显示未知 → 保留证据与脱敏原因，增量扫描安排复测；绝不全染红/绿。
- [Risk] 1 分钟扫描与手动并发多实例争抢，或预算被同一节点多 kind 超限 → CAS/在池去重，精确任务预算、假时钟与并发夹具验收；确保手动不推进 next_due。
- [Risk] 更细分类泄露地址/凭据 → 错误枚举与固定脱敏模板，不输出原始 `clientFactory` 错误/credentials/私钥；安全拒绝零出站测。
- [Risk] 改迁移或版本写路径影响订阅导入与历史数据 → SQLite 临时库迁移、双入口 Upsert/Update、重启/再次导入和 UI 旧观测回归；迁移非破坏性，部署前另需授权和备份。
```

### Plan Contract DOC-0273 <a id="plan-doc-0273"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:44`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. 在隔离 SQLite 测试库验证新增列迁移、旧数据 NULL 的未知语义、元数据更新不改变版本、真实连接变更版本递增、重启及双写路径；不在本规划运行生产数据库。
2. 实施 A→B→C→D 的完整特性与各自测试；门禁 `go build ./...`、针对性单测、`go test ./...`、前端类型检查和相关测试；独立 Reviewer 复验最终 diff 与契约。
3. 仅在另行获得预览授权与隔离 URL 后，由准备者隔离启动并提供 Critic 只读黑盒验收：一分钟扫描/增量与手动刷新、pool/台账口径、移动端交互；Critic 不承担构建/部署。上线、生产节点复测和最终真假故障结论均不属于本规划施工授权。
```

### Plan Contract DOC-0276 <a id="plan-doc-0276"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:32`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 在 `internal/probe/profiles` 与 `internal/application/probe` 中建立基于真实证据的各维度判定契约，彻底消除 `HTTP 200/204` 一刀切带来的伪阳性。
- 修复 `speed` 探针完整响应体传输计时，设置最小有效测速负载门槛（`MinValidSpeedBytes = 1024`），并保留超限预算 `speed_budget_exceeded` 错误行为。
- 在 `DefaultRunner.Run` 内实现同 `ProbeRun` 同节点的客户端单次创建、跨阶段/跨维度复用及全部任务回调完成后的确定性单次关闭。
- 在 `internal/repository/sqlite/probes.go` 中实现数据库内按 `(node_logical_id, kind)` 分区选最新（含 `observed_at DESC, id DESC` 决胜）与复合索引加速，并配套离线基准与回归测试。
- 将全部改动拆分为两个写集合完全正交的特性级工作包（工作包 1：`internal/application/probe/**` + `internal/probe/profiles/**`；工作包 2：`internal/repository/sqlite/**`）。
**Non-Goals:**
- **严禁编写未经证实的 Netflix / OpenAI 私有解锁规则，严禁照搬 `subs-check-pro v3.2.1` 的误判实现**。
- **严禁删除或弱化** `NewSafeNodeDialer` 的 Server DNS Pinning、SSRF 公网 IP 校验、TLS 证书校验、HTTP 重定向禁止（`http.ErrUseLastResponse`）以及 `domain.ComputeProbeEvidenceDigest` 签名摘要计算逻辑与既有公开函数签名。
- 不修改 HTTP 路由或前端代码，不执行 `git commit`、容器构建或触碰生产环境。
```

### Plan Contract DOC-0279 <a id="plan-doc-0279"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:95`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Decision 3: SQLite `ListLatestByNodes` 数据库内窗口函数精确选取与复合索引

- **方案设计**：
1. **复合索引保障（`internal/repository/sqlite/probes.go` 或 `db.go`）**：
- 在 `NewProbeObservationRepository`（或连接初始化时）执行幂等索引创建：
```sql
CREATE INDEX IF NOT EXISTS idx_probe_obs_node_kind_observed_id
ON probe_observations(node_logical_id, kind, observed_at DESC, id DESC);
```
- 该索引与分区排序键 `(node_logical_id, kind, observed_at DESC, id DESC)` 完全对齐，且修改范围严格收敛在 `internal/repository/sqlite/**` 写集合内。
2. **窗口函数 `ROW_NUMBER() OVER (PARTITION BY node_logical_id, kind ORDER BY observed_at DESC, id DESC)` 查询**：
- 在 `probeObservationRepository.ListLatestByNodes` 中（保留每批 `chunkSize = 100` 的参数分片与去重），将查询重写为：
```sql
WITH ranked_observations AS (
SELECT id, probe_run_id, node_logical_id, kind, verdict,
evidence_digest, observed_at, latency_ms, redacted_summary,
ROW_NUMBER() OVER (
PARTITION BY node_logical_id, kind
ORDER BY observed_at DESC, id DESC
) AS rn
FROM probe_observations
WHERE node_logical_id IN (%s) [AND kind IN (%s)]
)
SELECT id, probe_run_id, node_logical_id, kind, verdict,
evidence_digest, observed_at, latency_ms, redacted_summary
FROM ranked_observations
WHERE rn = 1;
```
- SQLite 在数据库引擎内部按 `(node_logical_id, kind)` 分区并根据 `observed_at DESC, id DESC` 选出 `rn = 1` 的唯一最新记录，传输给 `database/sql` 的行数最多为 `len(chunk) × len(kinds)`。
- **备选方案对比**：
- *备选方案*：`GROUP BY node_logical_id, kind` 配合 `MAX(observed_at)`。
- *放弃理由*：SQLite 的 `MAX(observed_at)` 裸聚合在 `observed_at` 秒级时间戳相同时无法保证按 `id DESC` 确定性决胜（tie-break），而 `ROW_NUMBER() OVER (PARTITION BY node_logical_id, kind ORDER BY observed_at DESC, id DESC)` 与仓库内 `internal/repository/sqlite/node_risk.go:580` 的成熟范式完全一致且具备严格的 `(observed_at DESC, id DESC)` 确定性决胜语义。
```

### Plan Contract DOC-0281 <a id="plan-doc-0281"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:137`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. 本次变更不涉及外部 API 破坏性变更或持久化表结构破坏，幂等索引 `idx_probe_obs_node_kind_observed_id` 在仓储初始化时自动通过 `CREATE INDEX IF NOT EXISTS` 创建。
2. 两个正交特性工作包（工作包 1：`internal/application/probe/**` + `internal/probe/profiles/**`；工作包 2：`internal/repository/sqlite/**`）可并发独立实施并通过各自单测与静态编译门禁，最后在汇聚门禁运行全量回归测试。
```

### Plan Contract DOC-0282 <a id="plan-doc-0282"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/design.md:1`
- **Source Digest**: `02eb8a19dbdf7bacd3df9c8d2be1554ee16423847c213dd43b6c8395e7f63dbd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

本整改承接原 probe、生命周期及跨视口需求，不降低任何既有门禁。最新审计 `REJECTED` 的新增根因是审查在**共享主仓**执行了会写盘的 `npm --prefix web run build`：`web/vite.config.ts` 插件 `closeBundle` 在 `NO_COPY_WEBASSETS !== '1'` 时直接递归 `cpSync(web/dist, ../internal/webassets/dist)`；它不会先清空目标，却会覆盖同名文件并留下旧 hash 资产。未跟踪资产聚合哈希 `bac5...`→`6ec...`。当前备份 18 文件；主仓 22 文件：相对备份新增四个（`index-BLIeF39G.css`、`index-DiuVQvQe.js`、`index-CuRBRMN8.js`、`index-CY02MDnG.css`），其中前两者在当前 `index.html` 引用，且匹配预览 isolated workspace manifest；另两者既不在备份亦未由当前 index 引用，但**不等于可删除**。备份 `index.html` 引用原 `index-w-J7iDPQ.js`、`index-CEihJNxU.css`；主仓 index 改为引用预览的 `DiuVQvQe/BLIeF39G`。审计称旧 17 assets 字节匹配，须复算原始文件逐字节和历史基线哈希；哈希变化至少包含 index 替换、新增资产。不要将 `bac5...` 或外部备份未经认证即当成覆盖主仓的绝对真值。
`/tmp/csp-preview-refresh-20260924/instance.json` 的 `sourceTreeSha256Before/After=9f359...`、binary SHA `75b7...` 和 served index/JS/CSS hash 表明**预览快照在构建当时**一致；其记录主仓资产在预览构建前后均 `6ec...`，未证明该值来自预览脚本，也未证明现时源码与旧快照一致。预览 snapshot 构建于此次审计前，旧 reviewer PASS 与 critic 双 PASS 无法补全最终汇聚次序。`graphify-out/graph.json` 在当前范围不存在，不从知识图谱推断调用链。之前已排除“所有协议都不能连”（非 race 局部 204）、“仅用外层 overflow hidden 即可修复”（旧 PNG 裁操作）、“仅有截图就代表视觉通过”（旧视觉明确 FAILED）、“原仓额外资产未被 index 引用就可擅删”；仍待证的是额外两个 hash 文件来源和最终代码是否与预览一致。
```

### Plan Contract DOC-0283 <a id="plan-doc-0283"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/design.md:7`
- **Source Digest**: `02eb8a19dbdf7bacd3df9c8d2be1554ee16423847c213dd43b6c8395e7f63dbd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:** 保护并恢复可核验的原仓用户资产、消除审查构建污染路径；既有六协议与安全负例及生命周期真实闭环；七路由、409→恢复、主题和移动可用；最终单一源码/嵌入产物/隔离 URL/原图身份链；最终 Go/web 全量门禁→独立只读 reviewer APPROVED→独立 critic FLOW/VISUAL 双 PASSED。
**Non-Goals:** 策略机不删改业务文件或原仓资产，不运行测试或构建，不触生产端口/卷，不给另一个 Pi 治理 Change 的真机验收代签，不凭无引用证明资产是垃圾。若归属待定，保护原状、列出决策点并寻求授权，绝不通过勾选任务假结案。
```

### Plan Contract DOC-0284 <a id="plan-doc-0284"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/design.md:13`
- **Source Digest**: `02eb8a19dbdf7bacd3df9c8d2be1554ee16423847c213dd43b6c8395e7f63dbd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Decisions

1. **取证先于资产修复，按文件和引用链判归属。** 由有授权施工者以 Go `sha256sum`/`find`/`diff` 或 Node 标准库对备份、主仓、当前 `web/dist` 与预览 workspace 列相对路径、字节 SHA、mtime 和 index 引用；保全所有待处理字节到带清单的只读隔离副本（须在授权范围），计算备份完整聚合 SHA 是否重现 `bac5...`，复核 17 原资产逐文件一致。沿 web/dist、已有证据 manifest、相关构建时间与任何其他工作树引用追溯 `CuRBRMN8/CY02MDnG`；明确**谁拥有原仓未跟踪文件、哪些要保留以及预期恢复目标**后才允许局部恢复。若最终需移走/删未跟踪资产，须主脑取得用户明确授权；未经授权不做，不能使用 `git clean/reset/restore` 或覆盖目录。恢复仅针对经确认的文件并防覆盖；验收含目录清单、每文件 SHA、index 所有引用可解析、原有 18 文件（含 index）完整、合法新产物按批准方案保留、原仓其他 status 不变。不可确认归属则保全证据并升级，不将复原算 PASS。
2. **构建钩子是全链路写权限边界。** 前端回归优先在 `mktemp` 隔离工作区复制必需源码/依赖、独立 output 与 embed，使用现有 Vite `NO_COPY_WEBASSETS=1` 构建纯 `web/dist`；若后端必须嵌入，则只在隔离副本把新 dist 安装进该副本 `internal/webassets/dist` 后运行 `go build ./...`。在隔离环境检查 Vite 钩子行为及其覆盖风险；生产是否需改 `vite.config.ts`（例如将资产复制作为显式 opt-in 且无默认写主仓），由施工者基于现有调用点/兼容性和测试证据决定，不能因此破坏正式嵌入构建。针对带 sentinel 的模拟目标目录运行“默认不写原仓、显式写只限隔离输出、并存旧 hash 时给确定结果”的测试；基线主仓路径集合及每文件 SHA 与构建后完全一致，且主仓 `git status --porcelain` 不增减业务文件。reviewer 使用只读代码审计与已有测试报告；如确需独立执行构建，必须在自有隔离副本、显式关复制，并验证主仓前后快照。`npm --prefix web run build` 在主仓默认模式不再是许可验证命令。
3. **保留完整网络 probe 和停服原门禁。** `internal/probe/singbox/protocol_handshake_matrix_test.go` 用标准 `sync/atomic` 或锁保护 `targetHits/observedHost`；取得旧 race SHA `263766475...` 原始双栈以区分计数器、HTTP2/gRPC late-reader、VLESS/httpupgrade EOF；优先经兼容验证的官方 sing-box 修复，必要时固定 upstream commit 做最小 patch，写端不等待读端 Setup。服务端等 body 才答复的 `httptest.Server` 证明不存在读写死锁；现有 `SafeNodeDialer`、`fetch.DefaultPolicy`、测试专属 TLS CA/SAN/SNI 和受控 registry/loopback mapper 构建合法公网 IP pin→真实六协议 inbound→HTTP 204、12 transport 与错误凭据/Host/SNI/私网/重绑等负例，逐行记录 socket/证书/Host。`cmd/csp` 使用 `os.Pipe`/`Cmd.ExtraFiles` 测试装配同步 Bearer+Idempotency-Key POST→service→Runner→Scheduler/RunID→callback 持久化→Drain→真实 DB.Close→clean→exit0，以及超时无提前 Close/clean 并自然非零退出；不能拿直接 hook Submit/GET 代替。2.2/2.3 有局部通过记录而最终版本仍须再跑。
4. **移动页面实际收缩与导航交互，不靠全局裁剪。** Publications header 三按钮组允许换行，五 target 优先 `flex-wrap`，若局部滚动须滚动提示/键盘/末项可见；Policy action row/长 rule 单元、Subscriptions grid/secret_ref 逐层 `min-w-0`，Popover 复用既有定位或 `@vueuse/core` 的能力而非自制通用定位框架；Dashboard badge 内容适配、Settings 状态避免重复、App 从同一 `navItems` 给 Dashboard 与其余路由可辨入口。用现有 Tailwind/DaisyUI/Vue Router/Vitest/Playwright。真实 375×667、392×872、1280×800 下逐元素测 main 宽、控件及菜单 rect、badge scroll/clientHeight、目标末项可达与 dock 间距；jsdom 仅验证状态和点击，不测几何。长文本、409→修订→预览、401/403/500/网络、主题/语言均在隔离受控 fixture 验证。
5. **确定性同源验收链按先后不可逆汇聚。** 各完整特性施工及自测完毕，最终 diff 冻结为源清单（包含未跟踪实现源码/资产、每文件 SHA 和 Git status，不单靠 commit SHA）；先跑 `go test ./...`、指定 race `-count=3`、`go build ./...`，web `vue-tsc --noEmit`、Vitest、隔离构建和 Playwright；记录命令、退出码、版本及原仓无写指纹。若有改动，全部门禁重新计数。准备者在隔离 tmp 工作区**由该冻结源码**重新构建前端并更新隔离 `internal/webassets/dist`，然后构建 go:embed 二进制、临时 SQLite/密钥/fixture 并绑定独立 loopback 端口；记录源清单 SHA→前端文件 SHA/index 引用→embed 目录 SHA→binary SHA→health/auth/实际 HTTP served SHA→此实例的 DOM trace/原始 PNG SHA、截图时间及 URL/进程身份。启服务后若做可变 fixture，保存场景状态与时间戳/重置证据；对最终二进制与服务资源做二次核验。**不可复用** `/tmp/csp-preview-refresh-20260924` 的旧快照身份，即使旧 health 可达且源快照构建前后一致；可复用脚本思想/fixture 实现，但必须给新唯一 instanceId 和最新快照。回归测试与截图必须对应同一冻结源清单；如果审查要求修代码，重新从汇聚门禁开始。
6. **reviewer 与 critic 分权顺序。** 新全量门禁成功并验证原仓资产/源码保护后，仅由只读 reviewer 对同一冻结 diff、Change、证据清单和构建安全审查，给 APPROVED；不得在主仓执行 npm build、改 dist 或勾任务。随后准备者提供已证明隔离、可达、认证与原始 PNG 的新实例 JSON 给 critic；critic 用只读 Playwright/像素/DOM 与 `read` 直接看原 PNG，分别出具 FLOW、VISUAL PASSED，涵盖七路由、五 targets、409 恢复、菜单、badge、Settings、探针安全负例对应门禁及三视口。审查或验收 REJECTED/FAILED/BLOCKED/PRECONDITION_FAILED 均回根因整改，不能保留先前终态签名；真机不可用不准借本地替代冒充物理验收。
```

### Plan Contract DOC-0285 <a id="plan-doc-0285"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/design.md:27`
- **Source Digest**: `02eb8a19dbdf7bacd3df9c8d2be1554ee16423847c213dd43b6c8395e7f63dbd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [未跟踪资产来源不清/外部备份不可信] → 先多来源逐文件比对与归属审批，歧义留原状并升级；不盲删两文件。
- [Vite 编译顺手覆写目标] → 主仓构建禁行、隔离 workspace+`NO_COPY_WEBASSETS=1`、前后每文件指纹；reviewer 独立运行任何有副作用命令同样遵守。
- [旧隔离预览被误作新版本] → 新 instanceId、冻结源清单、前端→embed→binary→served→PNG 哈希链，旧实例只用于失败取证。
- [矩阵只修测试 race 而漏 late-reader/EOF] → 原始双栈归因、204 与取消/先写后读夹具、每行独立真实 socket；失败不勾。
- [隐藏溢出假修复] → 实测每个控件边界、点击与原图；父级 `overflow-x-hidden` 不等于可达。
- [评审次序逆转] → 冻结源码、全量门禁、reviewer、critic 顺序写入证据账本；任何源改动强制重跑。
```

### Plan Contract DOC-0286 <a id="plan-doc-0286"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/design.md:36`
- **Source Digest**: `02eb8a19dbdf7bacd3df9c8d2be1554ee16423847c213dd43b6c8395e7f63dbd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

本 Change 仅规划；实施由获授权施工者按任务执行。先保全并判明主仓资产，分离构建写边界；Go probe/停服与前端可用性可在公共契约冻结后按无重叠写集合并行，各自测试闭环；汇聚后在隔离环境建同源新实例，全量复验、只读独立 reviewer、再 critic。对另一 Pi 治理 Change 只报告其 3.1–3.3 尚未完成及真机不可用，不改其工件。无原仓资产来源或删除授权时不得清理未跟踪文件；实施是否可继续由主脑按证据决策，不以暂停冒充完成。
```

### Plan Contract DOC-0287 <a id="plan-doc-0287"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:1`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md`。生产环境 9 个订阅源中 4 个处于启用状态，最近 6 条拉取审计记录显示 4 条 failed（TLS 握手超时、拉取超时与 HTTP 404）及 2 条 partial，且所有订阅的 `fetch_proxy_ref` 与 UA 均为空。容器内当前无代理环境变量。Clash Verge Rev 官方 v2.5.6（`src-tauri/src/utils/network.rs:246-255`）使用 `clash-verge/v2.5.6` 默认 UA 与 `reqwest` 标准请求头（`Accept: */*`, `Accept-Encoding: gzip, deflate`）。宿主机运行中 mihomo 监听于 `0.0.0.0:7890`，实测容器通过 `host.docker.internal:7890` 连通正常。
```

### Plan Contract DOC-0288 <a id="plan-doc-0288"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:5`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 将系统默认抓取请求头对齐 Clash Verge Rev 官方标准，包括默认 UA、`Accept: */*`、`Accept-Encoding: gzip, deflate`。
- 支持单订阅级自定义 `user_agent_policy` 优先覆盖。
- 设计明确配置的全局出站订阅代理机制（环境变量 `CSP_FETCH_PROXY` / 命令行参数 `-fetch-proxy`），在 Compose 中安全对接 `host.docker.internal:7890`。
- 实现分级代理选择：订阅级 `fetch_proxy_ref` > 全局 `CSP_FETCH_PROXY` > 直连。
- 保证手动刷新与定时调度统一走相同的代理选择管道。
- 建立分离式 SSRF 安全防护模型与应用层强制目标 IP 绑定：绝不依赖上游代理（如 mihomo）的 ACL；在每一跳连接前解析并校验目标公网 IP，将已验证的公网 IP 强制绑定传递给代理（HTTP 传递 IP 绝对 URL 且保留原 `Host`，HTTPS 向已验证公网 IP 发起 `CONNECT` 并在 TLS 中校验原域名证书与 SNI），302 每跳重新校验与绑定。
- 严格限定显式 `http://` 代理协议，白名单精确匹配 `http://<canonical-host>:<canonical-port>`，明确端口前导零、尾随点与 IPv6 规范化规则，自定义 `fetch_proxy_ref` 私网端点严禁绕过。
- 如实陈述运行态代理域名路由影响与 TOCTOU 边界，严谨执行本地测试与错误分类脱敏审计，严禁真实批量刷新生产订阅。
**Non-Goals:**
- 盲目照搬桌面端 Clash Verge 的系统代理嗅探或跳过 TLS 证书校验。
- 盲目放开内网或回环访问权限，或向白名单写入裸主机名导致同主机其他危险端口被放行。
- 在构建与验证过程中直接触发生产环境真实拉取。
```

### Plan Contract DOC-0289 <a id="plan-doc-0289"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:24`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. Verge 订阅头规范化

- **决策**：在 `internal/fetch/fetch.go` 中将 `DefaultUserAgent` 常量修改为 `"clash-verge/v2.5.6"`，并在构建 HTTP 请求时显式注入 `Accept: */*` 和 `Accept-Encoding: gzip, deflate`。保留原有基于 `opts.UserAgent` 的单订阅覆盖逻辑。
- **备选方案**：保留 `clash-sub-parser/1.0` 并在订阅中逐个手动配置 UA。被否决，因为大多数订阅提供商（如各类面板）只识别 Clash/Verge 客户端 UA 才能返回对应配置，且生产中现有所有订阅 UA 均为空。
```

### Plan Contract DOC-0290 <a id="plan-doc-0290"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:28`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 全局出站代理与优先级分发

- **决策**：
1. 在 `cmd/csp/main.go` 中解析 `CSP_FETCH_PROXY` 与命令行 `-fetch-proxy`。
2. 严格校验与规范化代理端点：仅允许显式 `http://` 协议（`socks5`、`https` 等其他协议一律失败闭合），必须具备显式合法端口（无前导零）、无 userinfo、无尾随点、无混淆 IP，且将规范化的完整 `http://<canonical-host>:<canonical-port>` 注入 `fetch.Policy` 的 `AllowedProxyHosts`，严禁丢弃 scheme 或写入裸 `host:port` / 裸主机名。
3. 在 `inventory.Service` 中引入 `defaultFetchProxy` 字段及 `inventory.WithDefaultFetchProxy(...)` 选项。
4. `ReconcileSubscription` 中判定：若 `sub.RefreshPolicy.FetchProxyRef != ""`，使用订阅级代理；否则若 `s.defaultFetchProxy != ""`，使用全局默认代理；否则使用直连。
- **备选方案**：使用标准库默认从 `HTTP_PROXY` / `HTTPS_PROXY` 自动继承。被否决，因为这会导致容器内无关请求（如探针、健康检查或本地通信）被意外劫持，不符合受控安全边界契约。
```

### Plan Contract DOC-0291 <a id="plan-doc-0291"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:36`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0292 <a id="plan-doc-0292"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:57`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 宿主 mihomo 7890 代理对接与 Compose 投产规范

- **决策**：
1. 更新版本受控的样例文件 `docker-compose.example.yml`，增加 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts: ["host.docker.internal:host-gateway"]`。
2. 宿主机实际运行的 `docker-compose.yml` 处于 `.gitignore` 中，保证提交仅包含 `.example`。
3. 上线投产规程：上线部署时，先对宿主机既有 `docker-compose.yml` 执行带时间戳备份，比对 `docker-compose.example.yml` 变更，同步合并 `CSP_FETCH_PROXY` 环境变量与 `extra_hosts` 配置，运行 `docker compose config` 核验语法，再执行重启投产。
```

### Plan Contract DOC-0293 <a id="plan-doc-0293"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:63`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [Risk: 上游代理（如 mihomo）对私网配置 `DIRECT` 且存在代理侧 DNS 投毒/重绑定] → Mitigation: 应用层强制将经过本地 SSRF 校验的公网 IP 绑定给代理（HTTP 绝对 URL 与 HTTPS CONNECT authority 均仅含公网 IP），代理不执行目标域名解析，无法将流量导向内网或宿主机回环。
- [Risk: 应用层向代理传递 IP 导致上游代理纯域名分流规则未命中] → Mitigation: HTTPS 隧道内仍保留完整原域名 TLS SNI（支持代理侧 sniffer），HTTP 请求仍保留原 `Host` 头；在设计、规格与测试中如实说明该路由差异。
- [Risk: 用户配置了不可达的代理端口导致拉取挂起] → Mitigation: `fetch.Client` 具有确定性的连接超时（`Dialer.Timeout`）与请求整体超时（默认 30s），超时后统一返回 `fetch_timeout` 失败闭合，并在审计日志中记录。
- [Risk: 目标订阅通过 302 重定向到内部私网服务或跨域名跳转] → Mitigation: 显式每跳重定向循环对每一跳独立执行 DNS 解析、SSRF 公网校验、目标 IP 重新绑定与 TLS `ServerName` 更新，遇到私网 IP 立即阻断。
- [Risk: 协议混淆、端口前导零、尾随点或混淆 IP 绕过代理校验] → Mitigation: 仅允许 `http://`，白名单精确匹配 `http://hostname:port`，严格拦截端口前导零、尾随点、IPv6 Zone、八进制/十六进制/整型/短 IP 及 IPv4-mapped IPv6。
```

### Plan Contract DOC-0294 <a id="plan-doc-0294"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:71`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration / Production Deployment Plan

1. 代码变更提交并在隔离环境中编译自测（仅跟踪 `docker-compose.example.yml`）。
2. 宿主机上线流程：
- 步骤 1：备份当前宿主机配置 `cp docker-compose.yml docker-compose.yml.bak.$(date +%Y%m%d%H%M%S)`。
- 步骤 2：参照 `docker-compose.example.yml` 在 `docker-compose.yml` 中补充 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts`。
- 步骤 3：执行 `docker compose config` 确保语法无误。
- 步骤 4：执行镜像更新与容器受控重建，验证健康检查端点。
3. 保持数据库向后兼容，不引入任何破坏性模式迁移。
```

### Plan Contract DOC-0297 <a id="plan-doc-0297"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/design.md:31`
- **Source Digest**: `a61cab822be5453efb60c4a3cf290a025964e405ba239e62efbece7ce32673c8`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 策略拓扑校验去冗余参数化（`internal/transport/http/policies.go` & `web/src/features/policy/usePolicy.ts`）

- **后端 `policyHandler.validate`**：
- 所有策略组、边和规则均已持久化在后端数据库中，彻底移除 `validateGraphRequest` 请求体解析分支，直接调用 `h.service.ValidateGraph(r.Context())` 从数据库加载当前策略组、边与活动修订规则进行校验。
- **前端 `usePolicy.ts`（`validateGraph`）**：
- 直接无参调用 `api.post<ValidationResult>('/api/v1/policies/validate')`，不再从前端传 `groups`、`edges` 或 `rules`。
```

### Plan Contract DOC-0301 <a id="plan-doc-0301"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/design.md:1`
- **Source Digest**: `fe90310a0b1947e45272ee0cc4a8a8979e9563d6a049a81bb967bb22a43a9afc`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 proposal.md 与 specs/admin-ui-device-quality/spec.md。当前工作区已有 726 项变更，`web/` 未跟踪；已完成的四项隔离基线/服务/测试数据/历史问题复核保持原有勾选。`web/e2e/auth-real-server.spec.ts` 的构建可能覆盖根目录 `csp`，`web/vite.config.ts` 构建后可能覆盖 `internal/webassets/dist`；现网 18080/17000 和数据卷禁止写操作。旧 ADB/USB 门槛和由此派生的真机认定一律废止。交互验收与 `csp-ui-hardcode-governance-v1` 对齐，不设全控件固定尺寸门禁。
```

### Plan Contract DOC-0302 <a id="plan-doc-0302"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/design.md:5`
- **Source Digest**: `fe90310a0b1947e45272ee0cc4a8a8979e9563d6a049a81bb967bb22a43a9afc`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:** 修复发布预览 500 与模板边界、中英文缺失键、异常状态恢复及 PolicyView 拓扑/抽屉联动；以隔离实例和设备 Ubuntu chroot 的 Chromium Playwright 复验全路由及移动/桌面布局与实际操作可达性。
**Non-Goals:** 不经 ADB、USB 或唤醒屏幕；不声称 headless 等于实屏；不以统一像素阈值代替交互实测；不在现网服务与卷上写测试数据；不部署、重启、清理脏工作区或扩张到无关重构。
```

### Plan Contract DOC-0303 <a id="plan-doc-0303"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/design.md:11`
- **Source Digest**: `fe90310a0b1947e45272ee0cc4a8a8979e9563d6a049a81bb967bb22a43a9afc`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Decisions

1. **预览契约先于交叉施工冻结。** 后端负责查明 `POST /api/v1/publications/preview` 的 500 根因，明确模板生成的合法输入、边界值、无效输入及对应成功/失败响应；前端按同一契约呈现真实结果与可重试失败。若该路径涉及其他 Change，先核对归属并串行处理公共接口及共用测试。
2. **隔离取证与写集合保护。** 延用已完成的临时服务/测试 DB 基线；对每个施工包先核对目标文件的已有变更，禁止覆盖用户改动。构建/E2E 仅在安全副本或经证实不覆盖根目录 `csp`、`internal/webassets/dist` 的路径执行；测试写请求仅发往独立临时实例，不向 18080/17000 或数据卷写入。
3. **可并行但不共享写目标。** 预览后端及契约测试是一包；`web/src/locales/messages.ts` 和裸键核查是一包；异常恢复和 PolicyView 联动若共用视图/组件则串行，由施工者确认分界；统一 E2E 与审查等待接口契约和前端合并，任何同文件编辑不得并行。弹层结构与样式归 `csp-ui-hardcode-governance-v1` 的共享原语包时，Policy 编辑不得同时修改同一文件。
4. **唯一设备通道与证据分级。** 设备命令仅通过 `ssh xiaomi-phone "/data/local/einck/script/ubuntu_chroot.sh run '<cmd>'"`；chroot 为 ARM64 Ubuntu，Chromium Playwright 缓存在 `/root/.cache/ms-playwright`。使用该浏览器在 392×872、375×667、1280×800 验证七路由、异常请求、语言、PolicyView 抽屉、底 Dock 避让及横向溢出；记录 viewport、构建标识、浏览器模式和脱敏控制台/网络证据。headless 只能证明浏览器视口下行为，不能证明实屏触控、软键盘或系统安全区域；未经额外授权不得添加物理屏幕操作。
5. **可观察验收替代固定尺寸门禁。** 在指定视口检查关键控件的可访问名称、焦点可见、实际可激活范围与相邻控件分离；滚动末项与 Dock 不重叠，长弹层可读且关闭/提交可达，无不可恢复横向溢出；所有路由无未处理错误或裸 i18n 键，失败状态可理解且可重试，不伪报保存/预览成功。目标矩形可作诊断但单个宽高数字或字面量扫描不能充当通过条件。先运行针对性测试，再在合并后的最终状态进行独立审查与隔离浏览器复验。
```

### Plan Contract DOC-0305 <a id="plan-doc-0305"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/design.md:26`
- **Source Digest**: `fe90310a0b1947e45272ee0cc4a8a8979e9563d6a049a81bb967bb22a43a9afc`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

无数据 schema 迁移或部署。施工完成只交付经独立审查与隔离复验的代码和证据；发布、重启、生产数据更改需另行授权。
```

### Plan Contract DOC-0307 <a id="plan-doc-0307"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/design.md:7`
- **Source Digest**: `ddff2ed1b6cfcb1100f1c6d06961ebb102b356964fcee450e96e1d865301fe40`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:** 用已有 DaisyUI/Tailwind 语义尺寸及少量有名称的布局原语统一重复规则，保证导航占位、内容流动和弹层可滚动；按真实交互情境验证，不以单个数字定义体验；每处保留的例外说明理由与复验路径。
**Non-Goals:** 不把所有 `px`/`vh` 或 Tailwind 间距一律删掉；不修改业务分页/超时、虚拟滚动测量或 API；不新增 CSS-in-JS/私有 token 框架；不重新设计全部七页视觉，不更改生产构建产物。
```

### Plan Contract DOC-0309 <a id="plan-doc-0309"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/design.md:22`
- **Source Digest**: `ddff2ed1b6cfcb1100f1c6d06961ebb102b356964fcee450e96e1d865301fe40`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [取消全局补丁暴露小控件问题] → 组件逐处审视可访问名称、label 和目标邻距，先编排回归再清理。
- [safe-area/浏览器动态工具栏影响实际可用高度] → 共享边界维护约束、以滚动与遮挡实测，分别记录 headless 不足和待授权实屏验证。
- [同时编辑 `App.vue` 或共享 CSS 与旧治理任务冲突] → 同文件串行单写者，合并后独立审查。
- [726 条脏改动且 `web/src` 未跟踪] → 施工前逐文件拍照/哈希并比对，禁止清场/覆盖，必要时停工请主脑处理归属。
```

### Plan Contract DOC-0310 <a id="plan-doc-0310"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/design.md:29`
- **Source Digest**: `ddff2ed1b6cfcb1100f1c6d06961ebb102b356964fcee450e96e1d865301fe40`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

无数据迁移；先核对旧 Change 已协调的合同，再冻结文件归属，按共享弹层容器先行、页面调用后迁移的依赖序列分批施工，独立复验合并态。若回归失败，仅撤销本任务可追溯的局部编辑，不使用 reset/restore/checkout/clean，也不影响既有脏文件。发布/重启需单独授权。
```

### Plan Contract DOC-0311 <a id="plan-doc-0311"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:3`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 本轮授权与边界

意图包 `verbatim_current_commands` 为“按你的建议做”“现在看看下一步推荐我做什么”“把你的建议先来吧，其他的也继续”；包内明确解释为 CSP 必要 app 短暂替换及真实验收，不是全服务器更新。本规划子阶段只允许有界只读取证及现有 OpenSpec 规划工件更新。禁止源码/测试实现、构建、部署、业务接口调用、刷新/恢复/提交/QQ。本轮未派子机。
CSP 起始 Git clean、HEAD e0431767845ffe3cd2bff8a7e415abf085814ac7；Pi 和 bridge 不是 Git 工作树，不能宣称 clean 或借嵌套 Git 提交。CLI allowedEditRoots 本 Change 仅 `/home/service/clash-sub-parser`；跨根不写。无 graphify-out/graph.json。
```

### Plan Contract DOC-0312 <a id="plan-doc-0312"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:9`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2026-10-02 只读现场

- Compose labels：project clash-sub-parser，workdir `/home/service/clash-sub-parser`，docker-compose.yml，service app。
- 容器 d8780938a0d6ae9e21590e5cb8d81a02cf74ac035947b559d6660a723a88511b，running/healthy，StartedAt 2026-10-01T12:57:31.010279174Z；镜像 sha256:807549162cebecfcddee509629a69d322fd98eef8a9b60ae9a05b89de5a10542，tag latest 与 v1-release-20261001-6a008f7-dirty，无 VCS build revision。命令 /app/csp serve，user appuser。
- 内存读取容器二进制（docker cp tar stream，未落盘/未执行）：SHA256 be2091a70e57d8f493d46ea4d14d4ba3fce1b4c35f0b6530e3908e161aa1e600，46854304 bytes，Go1.27.1/trimpath，本地 csp 与其不相等（不是源码变化证明）。历史部署报告 `/home/service/backups/csp-release-20261001_205016/deployment-report.json` 记录当前 image/container，dirty diff d5700687b3f7c34d1b12dbab174e72e6ded1b73acc09690074b58bace4037951；对应 commit-manifest 31文件包含库存及探针保护。
- 运行二进制包含95ce0a9/e043176库存新 upsert SQL（SHA b552f091362edc4f8fa93c36d1bb14e6888a993a1568dc9975107496a4a6f888），不含6a008f7旧 prune SQL（SHA a119e10a1be90800507caa883185b358b00234b254eae673fa1e812e110fe7c4）。此为代码特征证据而非完整 byte-for-byte 源码归属证明；旧 orphan SQL 仍可为其他调用存在，不能凭单个字符串推行为。95ce0a9..HEAD 的 app相关目录差异仅 internal/recovery（调用者仅恢复CLI，非cmd/csp）；库存/探针服务、web、migrations无新增差异。因此不推断保护未上线，重建用途是绑定经审查的当前树与镜像，后续先核验构建 manifest。
- 保持 named volume csp-v1-data→/data；127.0.0.1:17000/18080→18080；现有 CSP_FETCH_PROXY 与 host-gateway；DB `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db`。只读 SQL：schema13，1005节点（22 active/983 inactive），22 source，9订阅（4 enabled/5 disabled）。这不是恢复分类证明。
- 当前 fetch timeout30s、enabled订阅timeout30s、probe TTL300s、最大并发16（只读配置，不修改）。公开 healthz/readyz/auth-status 200，ready=true/tables14/schema13，admin/export均protected，authenticated=false。
- 已读取 `.env`（0600，仅CLASH_REQUEST_USER_AGENT）、容器及本进程 CSP_ADMIN_TOKEN 均空；已知受管 env.bak（0600）也无token。settings现存bcrypt verifier且两个auth开关启用，不输出hash。没有已确认生产明文凭据来源；历史隔离预览凭据不能用于生产。不要扩大到主目录盲搜；仅持有人安全投递现有匹配token到0600受管文件或受控进程env。启动代码以DB verifier为真，不允许注入任意新token覆盖。
- 历史DB备份及旧0ea88镜像归档仍存在，但本轮没做完整性/恢复测试，不作为fresh可回滚证明，旧镜像不是当前回滚目标。
```

### Plan Contract DOC-0313 <a id="plan-doc-0313"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:20`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

交付可溯源受控app更新、库存/身份保持、真实鉴权正文与代表性实网证据、可证误下线恢复。不是要求全节点在线；不造生产故障订阅、不新增陌生目标、不动UI/gateway/Hermes内核/模型/Gemini，不假全绿。
```

### Plan Contract DOC-0314 <a id="plan-doc-0314"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:26`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 冻结工作包及原生运行关联

唯一完整特性包 `pkg_csp_release_and_acceptance`，全部实现、自测同executor完成；审核/生产动作有依赖，不为并行拆微任务。change=`finish-csp-live-validation-and-recovery`；changeRoot=`/home/service/clash-sub-parser/openspec/changes/finish-csp-live-validation-and-recovery`，schema=spec-driven。
真实当前run `pi_run_20261002_194438_3727280`，metadata=`/home/service/hermes/workspace/tmp/pi_runs/pi_run_20261002_194438_3727280.json`。官方status本轮 RUNNING/IN_PROGRESS，不能说EXITED或PASS。parent session为环境指向的 `2026-10-02T11-44-41-696Z_sess_20261002_194438_3727280.jsonl`；本规划子会话不是executor/reviewer，不提供伪造 gate PASS。
metadata已冻结version1合同：change_root字段是项目工作区 `/home/service/clash-sub-parser`（区分CLI changeRoot），work_packages仅本包，epoch1；executor_self_test=true、reviewer=true、critic.applicable=false，非空reason为无新增UI的CLI/发布治理。不得覆盖这个不可变合同，不注册历史合同。
- 原生CLI `/usr/local/hermes-util/pi-dispatch`，help确认 dispatch / register-contract / status。新运行使用 `dispatch @<approved-task> --workdir /home/service/clash-sub-parser --change finish-csp-live-validation-and-recovery --change-root /home/service/clash-sub-parser --work-packages pkg_csp_release_and_acceptance --critic-reason '既有app发布与CLI实网验收，无新增UI'`（本轮不执行、不改model）；采用其新回执ID。已有当前合同无需register；仅active且缺合同时才register-contract，已存在冲突拒绝，历史/finished拒绝。
- 每次派工显式 Task Execution Contract：真实run_id/change/work_package/epoch/reverification_of=null或真实failed report_id。报告用json:pi-governance-report或官方HTML标记，含report_id/run_id/change/work_package/epoch/role/gate/verdict/final；executor列实际build/test命令exit_code0；reviewer必须真实独立子会话，失败后复验final:true及reverification_of。生产证据纳入最终独立复验，不能前置代码PASS抹除未做业务。
- 官方终态 `pi-dispatch status <actual-run> --json`，全部包门禁及真实业务完成后检查退出状态、报告、实际命令与独立session，无活跃失败。旧110336 run仍EXITED/UNVERIFIED/MISSING_COMPLETION_CONTRACT；独立112126 run仍EXITED/PASS、无children，原样保持，不追溯洗绿。
- contract_lineage=`fleet-contract-v3 / csp-release-and-remaining`。可读取原始意图包 `/home/service/hermes/workspace/tmp/intent_package_csp_release_and_remaining_20261002.md`（8617 bytes）；其原始字节SHA256，即contract_revision_hash=`cbca3c542a87438759c2c9d8fab80f205558a2ce066dd22ea06fa5aeebb47baa`。不是用户原话单独SHA，不改原始包。patch_hash实施后对明确文件集合实算，本轮业务patch尚不存在不编造。
```

### Plan Contract DOC-0315 <a id="plan-doc-0315"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:39`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 根因级整改及完整施工规格

写集合仅CSP：`cmd/csp-live-acceptance/main.go`、该目录标准*_test.go、`scripts/run_authenticated_acceptance.sh`及必要脚本fixture、`test/e2e/live_acceptance_and_recovery_fixture_test.go`、`docs/production_live_acceptance_and_recovery_plan.md`、`docs/controlled_deployment_runbook.md`。不改服务业务接口来迎合错CLI；沿用成熟标准API和既有领域类型/脱敏函数。
- 将main拆为可测run入口返回exit，统一汇聚必选断言：HTTP/JSON/schema/业务失败、超时、缺结果、库存变化、报告保存失败均不允许PASS。missing credential明确PARTIAL/BLOCKED；零退出安全暂停不能作为executor业务完成。日志/报告只脱敏白名单摘要，去原始URL/query/userinfo、token、节点配置/凭据、原始上游err；文件0600、目录0700，write errors必须返回。
- 节点Active实际为领域布尔（以NodeView为准），不能int解析。分页按现有pagination协议及100上限正确迭代；总数/页数/重复ID检查，全库概况与页内数分开。选取至少存在的active节点，不让首页无active成为成功零探针。
- `/nodes` handler不支持subscription_id筛选，现getActiveNodesForSub不能当订阅库存。使用既有节点detail Sources在有界全量清单中还原归属，或由运维读取SQL关联生成只读安全manifest（不新增后门）。集合为logical_id、active、connection_revision及来源关系的稳定摘要，失败前后集合比较；无法取到任何快照是FAIL不是0。失败响应500为顶层error envelope，不靠data.outcome空值判成功；client timeout/传输断开后要确认服务器结束与库存现态，无法确认标未知/失败，不自动重试刷新。
- POST探针准确字段 `node_logical_ids`、`kinds:["baseline"]`、显式deadline，仅选审查列表；不能node_ids空字段导致潜在扩大扫描。GET run终态是succeeded/failed/cancelled/expired，观察在 `/probes/runs/{id}/observations` paginated envelope，不是run内嵌observations。用domain.ProbeObservation（node_logical_id、kind/verdict、evidence_digest、observed_at、connection_revision、latency_ms、redacted_summary），校验每个预选节点的新观察与run/当前revision绑定。阶段从实际summary记录，不虚构字段；握手失败如实分层，缺观察不绿。等待是针对本业务job、deadline控制的有限事件进展读取，不无目标机械轮询；预算到期最多取消本工具创建的run，不取消其他任务，不改全局schedule。
- runner用传入受管scratch（核验owner/非公共可写）或mktemp -d、umask077，唯一bin、trap清理；不exec丢失EXIT cleanup。失败不执行旧binary。保留报告但删构建临时物，禁止固定公共/tmp路径。
```

### Plan Contract DOC-0316 <a id="plan-doc-0316"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:49`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
#

### Plan Contract DOC-0318 <a id="plan-doc-0318"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:63`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. fresh热备与恢复验证（后续门禁通过才执行）

1. 紧邻替换前再读Git/容器/DB/权限与变更计划；保存当前精确image ID，不以旧0ea88归档回滚。创建私有fresh备份目录（实际后续时间/run确定，0700/umask077），DB在线SQLite backup API/.backup（禁止直接cp正在写的DB），busy_timeout并确认成功。SQL只在同一snapshot统计库存/source/订阅/策略/配置revision/schema及auth verifier的受管对比摘要，不输出hash/secret。备份整个DB含身份和库存；复制Compose/.env及受管身份来源配置0600，docker inspect只保存脱敏选项，secret副本单独受控。
2. 在备份副本用immutable只读运行integrity_check与foreign_key_check；用第二个隔离可写副本恢复演练、逐项数/稳定身份指纹匹配，禁网络/后台refresh，不启动指向生产源的服务。确认owner UID/GID（运行image appuser为10001，执行前再核验）、权限与WAL模式处理。记录SHA/字节/检查exit与恢复演练证据，成功才可替换。
3. 将当前image ID固定为唯一 `clash-sub-parser-app:rollback-<actual-run>`，docker save导出当前镜像到私有备份并SHA/tar可读校验；docker image inspect标签必须指向相同ID。无需加载旧镜像覆盖当前。
```

### Plan Contract DOC-0319 <a id="plan-doc-0319"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:69`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 6. 构建/发布顺序及精确回滚

独立reviewer PASS→构建已冻结源码树及manifest（Git HEAD、定向diff SHA、构建输入/内嵌资产摘要、image ID/可选OCI revision，不以HEAD掩盖dirty）→验证新镜像基础命令/非root/DB兼容与隔离临时DB行为→fresh备份及镜像pin→只替换app。
后续命令以CSP workdir运行：`docker compose -f docker-compose.yml build app`；核对实际Compose service image（当前默认clash-sub-parser-app）及新ID；`docker compose -f docker-compose.yml up -d --no-deps --no-build --force-recreate app`。不down、不删卷、不pull或重启其他服务；Compose变量/端口/volume/代理逐项同基线。若已证明业务二进制无差异，重建只作为用户授权的可追溯发布，不假称新增保护。
更新后建议120秒就绪预算（来源本次部署控制建议，Compose health30秒/start10秒而非接口常量），公共health/ready与双loopback、鉴权模式、容器稳定无重启、真实已知入口（执行前从现有配置确定，不猜域名）、库存/身份基线一致；再执行合法真实验收。无凭据仍允许本地/发布可验证部分但整体PARTIAL，不要求先造凭据。
回滚触发：超120秒未ready/持续crash、双端口/入口失效、DB schema不兼容、库存删除/错误失活、身份/安全配置变化或泄密。普通上游网络失败本身不回滚，但停止进一步业务写并记录。
- 镜像回滚：仅stop app；`docker tag <captured-current-image-id> clash-sub-parser-app:latest`（执行前确认Compose确用此image）；`docker compose -f docker-compose.yml up -d --no-deps --no-build --force-recreate app`。检查实际image ID为captured值，健康/ready/双端口/身份恢复。禁止旧手册误tag clash-sub-parser:latest而app用另名。
- DB回滚仅在有损DB写/迁移且需要时：先stop app，确认目标无其他写者；封存失败DB以及-WAL/-SHM以留证，绝不让旧WAL应用到恢复快照；将已验证backup恢复至原卷内临时文件，chown/chmod与原值匹配，原子替换csp-v1.db，隔离旧sidecar后以captured镜像no-build启动。正常镜像回滚不覆盖仍完好的DB。记录恢复前后snapshot/身份/完整性，避免丢失不相关正常业务写；若出现未预期并发写需停止并由责任人决定精确恢复，不私自覆盖。
```

### Plan Contract DOC-0320 <a id="plan-doc-0320"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:82`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 7. 恢复及跨根报告遗漏

恢复白名单历史为0，本轮不重新认定全部失活原因；源、唯一身份、审计、用户禁用/删除、时间线充分才准入，fresh只读候选manifest→独立reviewer→精确precondition/transaction/backup→实际计数。缺证据的node维持现态并分层待决，0恢复不宣称全部正确或永久关闭需求。
报告角色识别/合同注册/测试出口已有实现及独立112126 PASS，本轮未运行桥接测试，不重复宣称新验证。bridge两个Change仍有未勾选的官方终态/交付与独立审查项：fix-pi-governance-report-role-attribution 2.2/3.1、finish-dispatch-isolation-and-attribution 2.1；Pi finish-pi-test-isolation有真实reviewer记录且勾选。它们属于各自根，不把CSP包PASS当别包PASS，不在本Change跨根勾选。后续母体对既有独立证据另行核对/精确收口，当前CSP合同无需扩改。
```

### Plan Contract DOC-0321 <a id="plan-doc-0321"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:88`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

凭据外部持有人尚未安全投递；不能从bcrypt恢复明文。镜像缺revision使完整来源一致性需后续构建manifest，而非仅比时间。真实节点可能失败、历史归因可能缺失，必须保留限定结果。备份恢复涉及WAL与并发写，须先演练再放行。CLI当前协议错配可能扩大探测，整改前禁用于production live。
```

### Plan Contract DOC-0322 <a id="plan-doc-0322"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:92`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

本轮更新规划并strict至READY，所有后续业务未执行保持unchecked。后续一个完整executor包自测→独立reviewer→已授权受控发布与证据收集→合法有界真实验收/充分证据恢复→独立终态复验→母体核验官方run与必要授权提交/通知。凭据阻断仅暂停受限目标；本地整改继续，绝不伪造业务完成。
```

### Plan Contract DOC-0323 <a id="plan-doc-0323"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:1`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md`。当前 SQLite 单主库中存量 1014 个节点，其中 31 个属于有效实时订阅成员（Scope E = 31，`active = 1`），983 个处于无源失活状态（`active = 0`）。取证表明，983 个孤儿中 960 个源自 Go 重写初期 `legacy.import` 遗漏 `node_source_links`，23 个源自后续订阅刷新下线剪枝时被物理清除 `node_sources`。
```

### Plan Contract DOC-0324 <a id="plan-doc-0324"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:5`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 建立独立的不可变历史账本表 `node_source_history`，保存节点与来源的历史因果关系与证明证据；
- 改造订阅刷新与生命周期写链，在成员下线、订阅删除时原子保留历史快照，杜绝运行时证据丢失；
- 提供 CLI 运维工具 `csp recover-source-history`，基于经过脱敏与 Hash 确权的清单幂等回填 983 个孤儿节点的历史事实；
- 提供只读 API `GET /api/v1/nodes/{id}/source-history` 并在现有节点详情 Drawer 中以只读方式展示历史归属；
- 建立端到端资产不变量门禁，确保回填与写链变动后生产库存状态 E 绝对不变、`active` 不变、`connection_revision` 不变、无虚假 `newfetch`。
**Non-Goals:**
- **严禁向 `node_sources` 回填存量 983 个孤儿节点**：`node_sources` 仅表征实时活跃成员，历史账本严格物理隔离；
- **严禁批量放活历史失活节点**：983 个节点维持 `active = 0`，不将其引入探针调度或对外发布池；
- **严禁在数据库或 API 中持久化敏感明文**：包括密码、Token、UUID、私钥及带鉴权 Token 的原始订阅 URL；
- **不重构既有 27 张业务表结构**：不重建 `nodes` 或 `subscriptions` 表，不修改现有外键关系；
- **不开设全局历史纠偏大平台**：UI 仅在现有节点抽屉内提供轻量只读历史审计视口。
```

### Plan Contract DOC-0325 <a id="plan-doc-0325"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:23`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D1: 独立历史账本表 (`node_source_history`) 优于扩展 `node_sources`

- **方案选择**：新增独立表 `node_source_history`。
- **设计要点**：
- `id TEXT PRIMARY KEY`: UUIDv7 唯一标识；
- `node_logical_id TEXT NOT NULL`: 外键引用 `nodes(logical_id)`；
- `subscription_id TEXT`: Nullable 外键引用 `subscriptions(id) ON DELETE SET NULL`；
- `source_identity TEXT NOT NULL`: 稳定命名空间标识（如 `legacy:src:1` 或当前 UUID），即使订阅被删仍保留历史身份；
- `source_label TEXT NOT NULL`: 脱敏展示名称（如 `7li`, `魔戒`, `einck-qzz`）；
- `connection_revision INTEGER`: Nullable，记录观测发生时的连接版本号；
- `relation_state TEXT NOT NULL`: `verified | conflict | unknown`；
- `cause TEXT NOT NULL`: `legacy_import | refresh_removed | subscription_deleted | manual_confirmed | unresolved`；
- `first_observed_at TEXT NOT NULL`, `last_observed_at TEXT NOT NULL`: 严格基于历史实际观测时间；
- `evidence_kind TEXT NOT NULL`: 证据类别（如 `legacy_archive_link`, `v1_backup_snapshot`）；
- `evidence_json TEXT NOT NULL`: 脱敏证据详情（含备份 SHA-256、表主键键值、来源 URL 规范化路径，排除一切敏感凭据）；
- 唯一索引：`UNIQUE(node_logical_id, source_identity, evidence_kind, first_observed_at)`，支持多来源与多次历史重入。
- **备选考量**：在 `node_sources` 增加 `is_historical` 字段。**否决原因**：现有业务大量查询（包括探针排队、对外发布、有效节点统计）依赖 `SELECT ... FROM node_sources`，一旦混入历史非活跃关联，将导致 Scope E 污染与灾难性穿透。
```

### Plan Contract DOC-0326 <a id="plan-doc-0326"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:40`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D2: 订阅生命周期写链防断链改造

- **刷新下线剪枝 (Refresh Pruning)**：
- 在 `internal/application/inventory/service.go` 的 `reconcile` 事务中，执行 `DELETE FROM node_sources` 前，先以 `INSERT OR IGNORE INTO node_source_history` 将即将被删除的 `(node_logical_id, subscription_id)` 关联快照持久化，带上当前订阅名称、`fetch_id` 及当前 `connection_revision`，`cause` 标记为 `refresh_removed`；
- 随后安全删除 `node_sources`。
- **抓取失败与停用**：
- 抓取失败时保持事务回滚或保持现状，不删除既有 `node_sources`，不产生虚假历史记录；
- 停用订阅（`enabled = 0`）仅更新配置，不触碰 `node_sources` 亦不触碰 `node_source_history`。
- **订阅物理删除**：
- 在删除 `subscriptions` 记录前，触发历史归档钩子，对该订阅当前持有的全部节点关系执行快照留存（`cause = 'subscription_deleted'`）；随后由外键 `ON DELETE SET NULL` 将历史表中的 `subscription_id` 置 NULL，但 `source_label` 与 `source_identity` 永久保留。
- **用户覆盖 (CoW Overrides)**：
- 用户修改节点参数仅记录于 `node_overrides`，历史来源关系按底层节点标识稳定继承，不篡改上游来源记录。
```

### Plan Contract DOC-0327 <a id="plan-doc-0327"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:52`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D3: 确定性单次回填运维命令 (`csp recover-source-history`)

- **CLI 架构**：
- 命令：`csp recover-source-history --manifest <path> [--dry-run]`；
- 输入：脱敏校验清单 `/tmp/csp_legacy_attribution_evidence/merged_cross_verified_manifest_v2.json`，内含清单 SHA-256、节点清单、证据来源及首选来源标记；
- 前置检查：验证数据库可读写，统计当前 `total_nodes` (1014)、`active_nodes` (31)、`node_sources` (31)；
- 执行事务：单事务批量写入 983 条首选历史归属记录与 15 条次要多来源证据记录；遇丢弃碰撞凭据记录仅作为丢弃证据留存，不作为有效归属挂载；
- 后置门禁：验证回填后 `total_nodes = 1014`, `active_nodes = 31`, `node_sources = 31`，若发生任何偏差强制事务回滚（Fail-Closed）。
```

### Plan Contract DOC-0328 <a id="plan-doc-0328"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:60`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D4: 规范连接比对与重复端点碰撞处理

- **首行规范比对确认**：取证已证实当前生产 960 个节点的配置与归档按 `ORDER BY id ASC` 提取的首行记录在协议、凭据（密码/UUID/私钥）及 TLS 参数上 100% 吻合；
- **首行归属已核验**：首选首行自身的 `node_source_links` 具有单一、明确且稳定的上游订阅（7li: 54, 魔戒: 31, einck-qzz: 9, Dogegg: 16, Eeox: 13, 公开节点: 79, Github: 757, WARP: 1），标记为 `verified`；
- **异凭据丢弃记录隔离**：对于 14 个存在重复端点的多行记录，13 个次行凭据相同作为次要来源保留，1 个次行凭据不同（pk 9669 from Eeox）作为丢弃冲突事实记录，严禁附着于当前节点。
```

### Plan Contract DOC-0329 <a id="plan-doc-0329"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:65`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### D5: 只读审计 API 与节点详情抽屉展示

- **API 契约**：
- `GET /api/v1/nodes/{id}/source-history`
- 响应：`{ "data": { "current_sources": [...], "history": [...], "attribution_status": "..." } }`
- 字段全面脱敏，绝对不输出明文凭据。
- **前端适配**：
- 复用现有节点详情 Drawer 组件，新增只读 Tab 或卡片呈现，主列表与核心路由不受影响。
```

### Plan Contract DOC-0330 <a id="plan-doc-0330"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:73`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [历史归属回填误污染实时发布] → 严格物理分表，发布编译器与探针调度器仅读取 `node_sources` 与 `nodes`，不读取 `node_source_history`；
- [回填过程中并发写锁冲突] → 使用 short transaction 与 busy timeout，回填工具在受控维护窗口由 CLI 执行；
- [脱敏清单防篡改] → 清单内置自身 SHA-256 与生成元数据，CLI 执行前比对校验和；
- [订阅删除后历史丢失] → 采用 `ON DELETE SET NULL` 结合非空 `source_label` 与 `source_identity`，保证数据永久可读。
```

### Plan Contract DOC-0331 <a id="plan-doc-0331"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:80`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. 执行 Migration `000016_node_source_history.sql` 创建独立账本表；
2. 构建并发布包含写链保护、只读 API 与 CLI 回填工具的应用版本；
3. 执行前在线事务热备数据库并校验 `PRAGMA integrity_check`；
4. 运行 `csp recover-source-history --dry-run` 预检脱敏清单；
5. 正式运行 `csp recover-source-history --manifest ...`，原子入库并核验前后资产状态一致性（1014 / 31）；
6. 验收只读端点 `/api/v1/nodes/{id}/source-history` 与前端 Drawer 展示。
```

### Plan Contract DOC-0345 <a id="plan-doc-0345"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:1`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

详见 `proposal.md`。当前 CSP 节点探针在内部使用 Sing-box 内存实例与自定义 `SafeDialer` 进行拨号，存在大量人为主观限制（拦截自签证书、阻断 Hysteria2 端口范围和 TUIC `disable_sni`、强行使用本地 DNS 解析验证远端 IP 并实施私网断言）。流媒体与 AI 探针契约被硬编码为 `contract_drift` 假存活。此外，调度器硬限制并发为 16~32，无法承载大节点库的快速初筛。
本设计对齐工业界成熟开源项目：
- **权威主基准**：`https://github.com/beck-8/subs-check` @ `3c320fd58aff5235e16218c050ec5b8ce587e233`（本地缓存 `/tmp/subs-check-beck`，GPL-3.0 许可）；
- **辅助调度与连接复用参考**：`https://github.com/sinspired/subs-check-pro` @ `e768ab277457ad3edc87c1cea6738a312192686e`（本地缓存 `/tmp/subs-check-pro-src`，GPL-3.0 许可）；
- **历史溯源分支**：`https://github.com/sinspired/subs-check` @ `8055505bf95631bf4dc6beabaa4956567b1c7f4e`。
```

### Plan Contract DOC-0346 <a id="plan-doc-0346"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:10`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- **进程内 Mihomo 适配出站**：基于 `github.com/metacubex/mihomo` (`v1.19.31`) 的 `adapter.ParseProxy` 构造单节点代理出站，完整支持所有协议参数，支持 `skip-cert-verify: true`、Hysteria2 `ports` 与 TUIC `disable_sni`。
- **目标域名直传远端与宿主代理隔离**：设置 `Proxy: nil` 彻底切断宿主 `HTTP_PROXY`/`HTTPS_PROXY` 穿透；目标主机域名直接作为 `constant.Metadata` 传入隧道，远端代理解析，防止本地 DNS 泄漏。
- **三阶段漏斗调度 (Funnel Pipeline)**：Alive (测活初筛) -> Media/AI (流媒体与 AI 深度检测) -> Speed (可选带宽测速)，存活失败立即终止，不再无差别执行重型检测。
- **端到端多平台细粒度结果**：全面移植 beck-8 的真实商业检测算法（OpenAI 网页/客户端双轨、Netflix 全解锁/自制剧/封禁、YouTube 地区码及送中过滤、Disney+、Claude、Scamalytics IP 风险分），彻底删除 `contract_drift` 硬编码。
- **SQLite -> API -> WebUI 统一数据链路**：扩展结构化观测结果并直通 WebUI，展现清晰的平台徽章与状态。
- **完整向后兼容与鉴权保全**：保全既有订阅库存、节点台账、定时调度、管理 API 鉴权机制与 `connection_revision` 校验。
**Non-Goals:**
- 不触碰非探针业务：继续保留 Sing-box 用于客户端配置导出与订阅转换，不修改 Gateway、Hermes 内核或模型路由。
- 不恢复无关专项：不恢复无关的 Gemini 模型异常专项（仅作为通用能力探针检测）。
- 不自造私有轮子：严禁手搓脆弱代理实现或过度工程化审批平台。
```

### Plan Contract DOC-0348 <a id="plan-doc-0348"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:34`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 三阶段流水线调度与并发控制

- **决策**：
- Stage 1 (Alive)：测活初筛阶段（默认并发 50），极短超时（2~3 秒），探测 `http://cp.cloudflare.com/generate_204` 或 CDN Trace，快速淘汰坏死节点；未通过测活的节点严禁进入 Stage 2 与 Stage 3；
- Stage 2 (Media/AI)：流媒体与 AI 深度检测阶段（默认并发 20），对 Stage 1 成功的节点并行或按需探测 OpenAI、Netflix、YouTube、Disney+、Claude、IPRisk；必须等 Stage 2 全量执行完毕后方可启动 Stage 3；
- Stage 3 (Speed)：可选测速阶段（默认并发 8；注：核实上游 subs-check 默认配置为 Alive 50 / Media 20 / Speed 20，本项目采用 Speed 8 系为了降低带宽争用与并发压力的本地差异化配置，非上游默认），完全接入 `platform.CheckSpeed`，使用 `NetworkLimitedReader` 设定 5MB 默认应用读取预算（作用于 `resp.Body` 读取截断，TCP 缓冲区存在网络层 read-ahead，非 NIC 网卡硬上限），单调截止超时（10 秒），测速结果以 `float64 KB/s (1024 B/s)` 计入 `throughput`。
- **生命周期**：每个待测节点在进入阶段时创建独立 `ProxyClient` 并在各阶段之间复用该客户端实例（复用上层 adapter 与 Transport 连接池，不称物理网络只拨号一次），探测完成后统一执行 `Close()` 释放底层连接；支持用户单选或混选探针类型；支持外部上下文 Cancel 优雅取消。
```

### Plan Contract DOC-0350 <a id="plan-doc-0350"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:63`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 消除所有人为伪阻断

- 废除针对 `skip-cert-verify: true` 的报错拦截。
- 废除针对 Hysteria2 `ports`、TUIC `disable_sni` 的拒绝。
- 废除针对私有 IP 与本地 DNS 解析的预检拦截，由远端代理决定出站路由。
```

### Plan Contract DOC-0351 <a id="plan-doc-0351"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:68`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[引入 Mihomo 依赖体积增大]** → Mitigation: Mihomo 与 Sing-box 共享许多基础网络依赖，仅在编译目标中略微增加二进制大小（约 15~20MB），换取完全合规的工业级协议支持与稳定性。
- **[部分商业流媒体接口风控加剧]** → Mitigation: 严格遵循真实浏览器 User-Agent 与常见客户端 Headers，避免高频并发击穿同一目标（采用随机乱序与合理并发）。
- **[网络测速流量消耗]** → Mitigation: `NetworkLimitedReader` 在应用层响应体 (`resp.Body`) 设定 5MB 流量读取预算，到达上限立即返回 EOF 截断并按实际读取字节与耗时精确计算带宽；明确标明为应用层读取预算（因 TCP 缓冲存在 read-ahead，非 NIC 网卡硬上限），不为字节预算增加脆弱复杂的 OS 层控制。
```

### Plan Contract DOC-0352 <a id="plan-doc-0352"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:74`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

1. **依赖升级**：在 `go.mod` 中引入 `github.com/metacubex/mihomo v1.19.31`。
2. **代码重构**：
- 在 `internal/probe/mihomo/` 下实现进程内代理客户端与看门狗；
- 在 `internal/probe/platform/` 下实现 OpenAI, Netflix, YouTube, Disney, Claude, IPRisk, Speed 平台检测器；
- 在 `internal/application/probe/` 落地三阶段流水线调度。
3. **数据模型与契约升级**：平滑扩展 `probe_observations` 的 JSON 格式与 `internal/application/inventory/` 中的 `CapabilityStatus`，旧数据零破坏。
4. **前端适配**：在 `web/src/features/probes/` 与 `nodes/` 接入细粒度平台标签。
5. **验证与上线**：本地编译与单元测试 100% 通过后，按受控部署手册执行 SQLite 热备与镜像构建。
```

### Plan Contract DOC-0353 <a id="plan-doc-0353"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:87`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 运行态谱系与旧 Run 关联澄清 (Run Lineage & Historical Integrity)

- **历史旧 Run (`pi_run_20261002_194438_3727280`) 与过渡 Run (`pi_run_20261003_004041_3864824`) 事实确权**：
- 历史旧 Run `pi_run_20261002_194438_3727280` 终态为 `EXITED / BLOCKED`，根因系缺乏可用的生产明文管理 Token 导致实网真实节点验收受阻；
- 紧随的过渡旧 Run `pi_run_20261003_004041_3864824`（对应会话 `sess_20261003_004041_3864824`）因超时终态为 `EXITED / FAILED / TIMEOUT`，此前文档部分字样机械称其为“另一个 BLOCKED”系误记，特此严谨纠正为真实事实；上述历史失败与超时事实严格保留，严禁覆盖、改写或篡改历史记录。
- **当前新 Run (`pi_run_20261003_083633_4084296`)**：
- 承接用户关于全面重做节点探针、对齐 subs-check、移除不合理门禁的明确授权；
- 全流程完成 Mihomo 进程内出站、三阶段调度漏斗、真实商业平台判定移植、前端 WebAssets 重新编译同步；
- 汇聚门禁独立代码审查已通过（`REVIEW: PASS`）；
- Critic 隔离多视口截图黑盒验收已通过（`CRITIC: PASSED`）；
- 主脑完成主线代码提交：`commit 9b43415 feat(probes): rebuild node probing from subs-check with mihomo pipeline`；
- 生产发布受控上线与双端口健康检查验证完成（`RELEASE: PASS`），旧 Run 历史记录不篡改。
```

### Plan Contract DOC-0354 <a id="plan-doc-0354"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:99`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. Critic 隔离多视口黑盒验收事实 (Critic Visual Acceptance Facts)

- **验收环境**：本地独立沙箱预览服务 (`http://127.0.0.1:18081`)，基于独立 SQLite 数据库 (`/tmp/csp-preview/csp-sandbox.db`)，生产数据 0 写入；
- **验收证据**：`/tmp/pi-critic-workspace/screenshots/01..14`（共 15 张图片，覆盖 `01_desktop_dirty_token_gate.png` 至 `14_mobile_probes_evidence_drawer.png`，含 `11b`）；
- **视口与量测**：覆盖桌面端 `1440x900` 与移动端 `392x872`；
- **核心判定结论**：
- 浏览器全流程 0 控制台报错 (console error: 0)、0 网络故障 (neterror: 0)；
- 细粒度平台分级真实呈现：OpenAI `Full (GPT⁺) [US]` / `Web (GPT) [SG]` / `App Only [HK]`，Netflix `Full [US]` / `Originals Only [HK]` / `Banned (Fast 403)`，YouTube 送中识别 `CN` 与正常解锁 `US/HK`，Disney+ `Soon` 与 `Banned`，测速吞吐 `51.2 MB/s`、`15.0 MB/s`、`4.0 MB/s`；
- 探针手动触发与取消流程实测正常；
- **测试数据边界声明**：所有展示节点均为明确标记为 `[Test-Fixture]` 的受控测试数据，绝不声称或冒充真实商业节点网络解锁；
- **总裁决**：`CRITIC: PASSED`。
```

---

## 3. Active Architecture Decisions (DEC-0807 ~ DEC-0809)

### Plan Decision DEC-0807 <a id="plan-dec-0807"></a>

- **Source**: `docs/decisions/README.md:1`
- **Source Digest**: `adaff746d88a47647e517e5bfa0494dd3cf0e6741d12b8417cec5f67c420c8c5`
- **Classification**: `active architectural decision / policy guideline`
- **Status**: Canonical architecture decision record per docs/decisions/.
- **Original Source Text**:
```text
# CSP 架构决策记录 (Architecture Decision Records)

本目录为 `clash-sub-parser` 项目专属的技术决策记录（Architecture Decision Records / Agent Notes）唯一权威事实源（Canonical Single Source of Truth）。

> **规范声明**：
> 1. 本目录借鉴 DeepSeek Agent Notes 的生命周期分类思想（`proposed/`、`implemented/`、`rejected/`、`archived/`），但**本项目未安装外部 DeepSeek 工具链或运行时框架**，完全依赖 Git 与 Markdown 原生维护；
> 2. 项目的主干能力规范由 `openspec/specs/` 承载，变更工作流由 repo-local `openspec` CLI 驱动；本目录仅记录“架构为什么如此设计”以及“否决了哪些备选方案”。

## 目录结构与生命周期规范

路径编码规范：`{lifecycle}/{class}/yyyy-mm-dd-topic-title.zh.md`

- **生命周期 (Lifecycle)**：
  - `proposed/`：已提议或已接受但**尚未在业务代码中完全交付落地**的提案记录（当前文档阶段的新增决策均置于此）。
  - `implemented/`：业务代码已完整交付且处于生产运行态的决策记录。**严禁在仅完成文档编写而业务代码未上线时将记录置于 implemented**。
  - `rejected/`：经评审后被否决的提案，保留其拒绝理由以避免未来重复犯错。
  - `archived/`：已落地且不再指导日常开发的历史决策。

- **分类 (Classification)**：
  - `architecture`：核心架构与领域模型设计。
  - `feature`：关键业务特性与能力设计。
  - `process`：工程流程与规范演进。
  - `bug-fix`：重大缺陷与架构对齐修复。

## 治理与一事实归属准则

1. **唯一真实事实源**：项目的形式化规格基线由 `openspec/specs/` 承载，可执行工作流由 `openspec/changes/` 承载；本目录仅记录“为什么如此设计”以及“否决了哪些备选方案”。
2. **状态客观性**：文档状态严格区分 `proposed` / `accepted` 与 `implemented`，绝不虚假宣称未落地的代码已实现。
```

### Plan Decision DEC-0808 <a id="plan-dec-0808"></a>

- **Source**: `docs/decisions/proposed/architecture/2026-10-09-living-spec-consolidation.zh.md:1`
- **Source Digest**: `27b443ea5305eb50cb5652c2090eb31f75b76b7005e93fcbeb90d1da954eb1ea`
- **Classification**: `active architectural decision / policy guideline`
- **Status**: Canonical architecture decision record per docs/decisions/.
- **Original Source Text**:
```text
# Agent Note: Living Specification 架构整合与规范分层

- Status: proposed (Phase: Documents & Specification Consolidation)
- Date: 2026-10-09
- Author: executor
- Scope: clash-sub-parser specification architecture and governance harmonization

## 背景与上下文

`clash-sub-parser`（CSP）项目在历史迭代中积累了 26 个 OpenSpec 变更集（Change）与 63 份分项规格，但根目录 `openspec/specs/` 一直为空，缺乏统一的持续有效主干能力契约（Living Specification）。各变更集之间的规格分散，部分历史假设、临时修复与实际业务代码之间存在偏差。

用户在本轮会话中明确指示：
> “按你的建议来，重写当前的SPC，再根据规格看当前是否符合，模糊整合询问我，然后再落地”

本决策记录旨在梳理本次规范整合的架构设计、设计依据及否决的备选方案。

## 决策内容

1. **采用 SpecKit 规范结构融入 repo-local OpenSpec 契约**：
   - 在项目内建立 6 大核心业务领域的主干能力规范（`inventory`、`policy`、`probe`、`publication`、`auth`、`ui`），存放于 `openspec/specs/<domain>/spec.md`。
   - 吸收 GitHub Spec-Kit 核心结构（优先级用户场景 User Story P1~P3、稳定功能需求编号 FR-ID、可度量成功标准 SC-ID），并严格适配 OpenSpec 机器可校验的 Markdown 结构（`## Requirements`、`### Requirement: ...`、`#### Scenario: ...`）。
   - 保留官方 OpenSpec CLI 校验能力，执行 `openspec validate --specs --strict` 确保零语法错误。

2. **保留 OpenSpec 作为全局执行治理框架，参考模板仅作局部副本**：
   - 在 `.specify/templates/` 保存 Spec-Kit 官方模板副本（锁定 commit `93989802b1401c99cc7de6e4ed76572f9cb662e8`）及 provenance 哈希记录。
   - 明确声明因工程既有 `pi-governance` 契约要求，保留 repo-local `openspec` CLI 作为变更驱动引擎，不安装或替换为外部 SpecKit 运行时。

3. **双模式策略组核心能力冻结与模糊语义显式标记**：
   - 将用户明确要求的“直接将其他策略组作为节点引用”（Direct Group Reference）与“解包获得其中节点”（Leaf Node Unpack/Flatten）确立为双核心功能需求。
   - 针对当前代码尚未实现的解包逻辑、混合交错顺序丢失、历史空组数据恢复、非 Mihomo 目标空组语义等未决事项，在规范中明确标注为 `[NEEDS CLARIFICATION]`，并通过结构化问题清单汇总呈报用户，严禁施工机擅自脑补默认逻辑。

4. **严格的文档生命周期与交付边界
```

### Plan Decision DEC-0809 <a id="plan-dec-0809"></a>

- **Source**: `docs/decisions/proposed/architecture/2026-10-09-speckit-version-pin-and-migration.zh.md:1`
- **Source Digest**: `697099d4a82d76c9afc90e446f0a22dae7df24789e828faf17303856d6f17bf9`
- **Classification**: `active architectural decision / policy guideline`
- **Status**: Canonical architecture decision record per docs/decisions/.
- **Original Source Text**:
```text
# Agent Note: GitHub Spec Kit 官方版本锁定与原生规范迁移决策

- Status: proposed (Phase: Pilot Migration & Schema Harmonization)
- Date: 2026-10-09
- Author: executor
- Scope: clash-sub-parser specification architecture and multi-root Spec Kit migration

## 背景与上下文

在上一轮规范整合中，项目引入了 GitHub Spec Kit 的参考模板（位于 `.specify/templates/`），并记录了 provenance 哈希，锁定了临时参考提交 `93989802b1401c99cc7de6e4ed76572f9cb662e8`。

用户在本轮会话中明确全量授权了跨 5 大根目录的正式 Spec Kit 迁移。为保证迁移的确定性与工程复现能力，主脑与施工机对官方 Spec Kit 进行了严格选型与环境验证。

## 决策内容

1. **版本锁定与升级变更说明 (Version Pinning & Provenance Transition)**：
   - **历史存根保留**：保留 `.specify/templates/provenance.json` 中对 `93989802b1401c99cc7de6e4ed76572f9cb662e8` 的历史记录，作为阶段演进客观凭证。
   - **全新官方锁定**：正式将工具链升级并锁定至官方正式发布版本 **`v1.1.2`**（发布日期 `2026-10-07T22:27:43Z`，commit **`959e866caa3618bf3dc290d5dca33394365af9c6`**）。
   - **变更理由**：`v1.1.2` 是 upstream 的最新稳定 tag，包含完备的 `PiIntegration`（支持 `.pi/prompts/` 原生斜杠命令）、`GenericIntegration`（支持任意技能与命令目录），且已封板发布，避免依赖 main 分支浮动提交。

2. **原生规范目录演进 (`specs/` vs `openspec/specs/`)**：
   - 遵照 Spec Kit 原生标准，在项目根建立编号能力规范目录：
     - `specs/001-inventory/`
     - `specs/002-policy/`
     - `specs/003-probe/`
     - `specs/004-publication/`
     - `specs/005-auth/`
     - `specs/006-ui/`
   - 每个能力目录下包含 `spec.md`（原生规格）、`plan.md`（技术方案与已知/未知事实）、`tasks.md`（分离迁移任务与业务待办）。
   - 现存 `openspec/specs/` 与 28 个历史变更集保持只读存在，严禁擅自删除或重命名，直到用户终态审查并确认切换。

3. **台账式全量追溯与未决问题保持 (Traceability Ledger & Retained Blockers)**：
   - 建立机器可读的 `docs/specification/migration-ledger.json`，全量覆盖 259 条历史需求、463 个场景
```

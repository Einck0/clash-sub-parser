# CSP 历史补充契约与设计台账注册表 (Historical Contract Register)

- **基线版本**: HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **编制日期**: 2026-10-09 (Epoch 4 Targeted Remediation)
- **编制角色**: Executor（施工机 · 治理与全量补充契约归档）
- **数据来源**: 26 个历史变更集之 proposal.md, design.md, tasks.md 及架构决策记录 ADR
- **治理原则**: 杜绝虚构运行态规范需求；逐条登记 809 项历史约束、设计提案、已完成历史任务与架构决策，赋予物理独立锚点与真实内容指纹，明确历史淘汰/现行规划/待审状态。

---

## 1. 统计摘要 (Census & Metrics)

| 统计类别 | 条目总数 | 归档处置 | 现行状态 |
|---|---|---|---|
| **设计提案与约束 (Constraints)** | 354 | `archived_reference` | 58 历史淘汰 / 35 待审未决 / 261 现行规划 |
| **历史已完成任务 (Historical Tasks)** | 452 | `archived_reference` | 452 项历史完成履约证据（非新开发任务） |
| **架构决策记录 (ADR / Notes)** | 3 | `archived_reference` / `active_decision` | 3 项主干规范与版本锁定决策 |
| **补充契约证据总量 (Total Supplemental)** | **809** | **100.0%** | **全量具备独立物理锚点与内容指纹** |

---

## 2. 约束与设计提案登记 (Design Proposals & Constraints - DOC-0001 ~ DOC-0354)

### Constraint DOC-0001 <a id="hcr-doc-0001"></a>

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

### Constraint DOC-0002 <a id="hcr-doc-0002"></a>

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

### Constraint DOC-0003 <a id="hcr-doc-0003"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/proposal.md:54`
- **Source Digest**: `070ab4619584a278fd6d887b4eaef971f16cfd4a675a91a3aaed068c600bc9af`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `audit-and-defect-remediation`: 针对 CSP 1.0 全栈经审计确证的功能缺陷提供确定性修复，涵盖修订引导、规则增删、探针死锁与错误分类、观测分页、订阅孤儿去活、CSRF 请求头注入、局域网 UUID 退避、前端轮询风暴优化与视口防饥饿。
```

### Constraint DOC-0004 <a id="hcr-doc-0004"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/proposal.md:58`
- **Source Digest**: `070ab4619584a278fd6d887b4eaef971f16cfd4a675a91a3aaed068c600bc9af`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（`openspec/specs/` 下无对应已归档主规格，本次通过新增能力规格建立权威契约）。
```

### Constraint DOC-0005 <a id="hcr-doc-0005"></a>

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

### Constraint DOC-0006 <a id="hcr-doc-0006"></a>

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

### Constraint DOC-0007 <a id="hcr-doc-0007"></a>

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

### Constraint DOC-0008 <a id="hcr-doc-0008"></a>

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

### Constraint DOC-0009 <a id="hcr-doc-0009"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/proposal.md:25`
- **Source Digest**: `ec6d3fecf6cd09621a7ca07617051cac5f0d09320fe3994cc67669145699e6aa`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。
```

### Constraint DOC-0010 <a id="hcr-doc-0010"></a>

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

### Constraint DOC-0011 <a id="hcr-doc-0011"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/proposal.md:1`
- **Source Digest**: `e300e72a069f772dddd392a308ce408b0bfd5ad0a69fe00722d7f23f62c6d5ae`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Why

旧 CSP 已被物理清空，且旧 Python、旧前端与旧数据模型均不再是可维护基线。保留的生产 SQLite 含有用户业务内容，但将其作为新运行时数据库或兼容旧表会把历史架构重新引入产品，因此需要在明确数据边界下重建单一、可验证的控制平面。
```

### Constraint DOC-0012 <a id="hcr-doc-0012"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/proposal.md:5`
- **Source Digest**: `e300e72a069f772dddd392a308ce408b0bfd5ad0a69fe00722d7f23f62c6d5ae`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### What Changes

- **BREAKING** 新建 CSP 1.0 产品边界：Go 1.22+ 单二进制、内嵌管理界面、新 SQLite schema 与新的 HTTP API；不保留旧应用代码、路由、序列化形状、页面状态或运行时适配层
- **BREAKING** 弃用旧的 `/yaml`、`/script`、旧 `/api/*` 和旧输出形状。新发布仅支持带版本前缀的受控导出契约；旧 URL 必须返回明确的已弃用错误，不能悄然映射
- 新增订阅采集、规范化节点台账、可取消且受配额限制的 sing-box 内存探测、证据化能力观察、服务端分页查询和递归策略树解析
- 新增以一个规范化解析结果为唯一输入的多客户端配置编译器，支持 Clash、Mihomo、sing-box、Surge、Quantumult X 五种目标
- 新增采用成熟开源标杆 Zephyruso/zashboard 体系的管理界面：基于 Vue 3 + Vite + Tailwind CSS + DaisyUI，原生多主题平滑切换，TanStack Virtual 虚拟滚动，移动端采用 Zashboard 经过实战检验的流畅弹性交互，不搞死板教条限制
- 新增独立、离线、一次性的 legacy SQLite 导入工具：仅导入经 allowlist 和校验通过的非秘密逻辑内容到新 schema 的待审草稿；不自动激活、不双写、不影子读、不原位改写历史库
- 新增部署、备份、恢复、切流与回滚契约：新服务使用独立新卷，旧停止的容器及历史卷只用于人工批准的回滚或离线导入
```

### Constraint DOC-0013 <a id="hcr-doc-0013"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/proposal.md:17`
- **Source Digest**: `e300e72a069f772dddd392a308ce408b0bfd5ad0a69fe00722d7f23f62c6d5ae`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### New Capabilities

- `control-plane-boundary`: 新 CSP 1.0 的版本化管理 API、鉴权、错误和废弃端点拒绝行为
- `subscription-inventory`: 订阅采集、节点逻辑身份、差集收敛、服务端分页节点台账与筛选
- `probe-evidence-engine`: 可取消、限流、幂等的 sing-box 内存探测作业及可审计结果证据
- `policy-tree-and-compiler`: 递归策略树单一解析器及五目标配置编译与发布
- `clean-slate-import-and-operations`: 新 schema、离线历史导入隔离、备份恢复、切流回滚及秘密边界
- `admin-workbench-experience`: 双模态、无障碍、服务端分页和移动端可用的管理工作台
```

### Constraint DOC-0014 <a id="hcr-doc-0014"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/proposal.md:25`
- **Source Digest**: `e300e72a069f772dddd392a308ce408b0bfd5ad0a69fe00722d7f23f62c6d5ae`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本仓库的旧 OpenSpec 主规格随旧源码被移除；本变更只定义 CSP 1.0 新能力。
```

### Constraint DOC-0015 <a id="hcr-doc-0015"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/proposal.md:28`
- **Source Digest**: `e300e72a069f772dddd392a308ce408b0bfd5ad0a69fe00722d7f23f62c6d5ae`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Impact

- 规划中的实现范围为新建 `cmd/`、`internal/`、`web/`、`migrations/`、`test/` 和部署工件；不得恢复或引用已删除的 Python/FastAPI、旧 Vue 代码或旧表模型
- 运行时依赖为 Go 1.22+、`modernc.org/sqlite`、sing-box Go 模块、Vue 3、Vite、Tailwind、shadcn-vue；Go `embed` 用于同一二进制交付静态界面
- 现有 `clash-sub-parser_backend-data` 卷和其 17 MB 历史数据库不挂载给新服务作数据目录；历史源仅由显式操作的离线导入器只读打开
- 端口 18080 与 17000 的最终绑定、反向代理切流和旧容器销毁属于受控发布操作，不由本计划自动执行
```

### Constraint DOC-0016 <a id="hcr-doc-0016"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:1`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### Why

线上环境因未配置 `CSP_NODE_CREDENTIAL_KEY` 导致 `vault=nil`，在订阅拉取、节点详情、安全探测与预览发布全链路抛出 `HTTP 422 node credentials unavailable: vault or credential repository not configured`。用户明确指出“这个K是什么？是我登录的key吗”、“话说为什么要加密”并指令**“去掉过度工程化设计，给系统做一次减法”**：个人/自建订阅解析器根本不需要也不理解 `NodeCredentialVault` AES-256-GCM 加密、版本化 CAS 轮换、`VerifiedNodeIdentity` 端点身份绑定、发布产物二次 AEAD 加密与前端 `***` 掩码体系。
```

### Constraint DOC-0017 <a id="hcr-doc-0017"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:5`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### What Changes

- **BREAKING**：彻底移除 `NodeCredentialVault` AES-256-GCM 加密体系、`CSP_NODE_CREDENTIAL_KEY` 环境变量依赖、`node_credentials` 表、`VerifiedNodeIdentity` 端点身份 HMAC/绑定、`SafeNodeConnection` Write-Only 掩码投影以及 `CredentialVersion` CAS 版本控制；系统启动无需配置任何加密密钥即可完整运行。
- **BREAKING**：新增 SQLite 迁移 `000011_plaintext_nodes_and_publications.sql`，在 `nodes` 表直接存储明文 `server`、`port`、`config_json`（删除 `normalized_config_secret_ref` 与 `credential_version` 列），在 `publications` 表直接存储明文 `content`（删除 `artifact_key_id`、`artifact_nonce`、`artifact_ciphertext`、`credential_binding_digest`、`credential_bindings_json` 列），并 `DROP TABLE IF EXISTS node_credentials`。
- 精简 `domain`、`parser`、`resolver`、`compiler` 共享契约：`domain.Node` 与 `resolver.ResolvedNode` 直接持有 `Server`、`Port`、`Credentials`（`InboundProtocolCredential`）；`compiler.Compile` 直接从 `snapshot.Nodes[i]` 读取节点明文配置，删除 `WithCredentials` / `WithCredentialInputs` 与身份交叉核验，仅校验各协议必填字段非空。
- 精简 `repository/sqlite`、`inventory`、`publication`、`probe` 与 `transport/http`：订阅 `ReconcileSubscription` 直接将解析结果写入 `nodes` 表；`Publish` 直接持久化明文 `Content` 至 `publications` 表，`ResolveAndServe` 校验 Token 与未撤销状态后直接返回明文 `Content`；`SafeNodeDialer` 直接从 `domain.Node` 读取明文配置且无条件挂载，周期探测不再按 `CredentialVersion <= 0` 跳过；节点详情与更新接口（`GET /api/v1/nodes/{id}`、`PATCH /api/v1/nodes/{id}`）直接返回与更新明文字段，订阅接口直接返回真实 URL。
- 精简前端 `web/src/**`：节点详情页移除 `***` 掩码、Write-Only 轮换输入、CAS 版本号、Vault AEAD 徽章与 Unavailable 横幅，直接展示并编辑明文配置；发布预览页移除 `redactPreviewSecrets` 直接展示真实配置文本；订阅编辑抽屉移除 `***` 占位直接回显真实订阅 URL。
```

### Constraint DOC-0018 <a id="hcr-doc-0018"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:15`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `debloat`: 移除节点凭据与发布产物 AEAD 加密、身份绑定、CAS 版本号及前端掩码，改为 SQLite 明文直存、直编、直发与明文可编辑管理契约。
```

### Constraint DOC-0019 <a id="hcr-doc-0019"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:19`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `debloat` 能力规格建立完整减法契约）。
```

### Constraint DOC-0020 <a id="hcr-doc-0020"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/proposal.md:23`
- **Source Digest**: `17a3a943f58f3ddd82339612955737d6d3fcb0db145242dc6a29221b3a4f45f1`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`migrations/000011_*.sql`、`internal/domain/`、`internal/parser/`、`internal/resolver/`、`internal/compiler/`、`internal/probe/singbox/`、`internal/repository/sqlite/`、`internal/application/{inventory,publication,probe,subscription}/`、`internal/transport/http/`、`internal/integration/`、`cmd/csp/`、`docker-compose.yml` 及 `web/src/features/{nodes,publications,subscriptions}/`。本阶段仅在隔离环境完成实现与验收，不触碰生产容器与线上数据库，生产库迁移通过备份保障并在升级后触发 `ReconcileSubscription` 自动重填明文节点配置。
```

### Constraint DOC-0021 <a id="hcr-doc-0021"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/proposal.md:1`
- **Source Digest**: `3fd54e6cae0d69fcfcc9306db4d2870358f5c1d55105e4b0571752069b68d767`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前生产环境出现订阅发布请求 404 与拉取失败：经深度排查，生产 SQLite 数据库 `publications` 表当前为 0 条记录，客户端请求历史 `/p/...` 路径报 404（CSP 标准导出路径为 `/publish/v1/{publication_id}`）；且现有导出端点硬编码强制要求专属 `pub_<hex>` 令牌，不支持免密公开拉取，也不支持使用管理端配置的统一系统令牌。同时，管理端鉴权粗暴绑定于密码字符串是否存在，缺少独立控制开关，无法满足用户关于“管理端与导出链接鉴权开关相互独立、但可复用同一个统一令牌”的核心诉求。
```

### Constraint DOC-0022 <a id="hcr-doc-0022"></a>

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

### Constraint DOC-0023 <a id="hcr-doc-0023"></a>

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

### Constraint DOC-0024 <a id="hcr-doc-0024"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/proposal.md:24`
- **Source Digest**: `3fd54e6cae0d69fcfcc9306db4d2870358f5c1d55105e4b0571752069b68d767`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。
```

### Constraint DOC-0025 <a id="hcr-doc-0025"></a>

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

### Constraint DOC-0026 <a id="hcr-doc-0026"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/proposal.md:1`
- **Source Digest**: `3e5329b834a2dfd2dce3e5973ce288df919c84c8039f4c441f5a3d9e87640eb6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

本 Change 已完成库存清空、两源 Token 修复、协议身份扩展、维护 CLI、规则管理及历史生产切换，现有 A/B 实现必须保留。2026-10-08 最新只读预检证明新比较尚未公平覆盖 ALL90、stage-first 执行器及跨启动预算未闭环、最后源码没有全量成功门禁；本次按用户已接受的分阶段实验修订同一 Change，不重做历史操作、不以标题计数相等替代缺陷解释。
```

### Constraint DOC-0027 <a id="hcr-doc-0027"></a>

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

### Constraint DOC-0028 <a id="hcr-doc-0028"></a>

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

### Constraint DOC-0029 <a id="hcr-doc-0029"></a>

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

### Constraint DOC-0030 <a id="hcr-doc-0030"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:1`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

用户发现部分操作菜单、弹窗在实际视口中被裁切或被其他弹层遮住，同时设置页缺少以探针状态为主的全局导出筛选入口。现有筛选后端已经存在，但发布解析在读取异常时可能静默回退为不过滤，产生不符合管理员意图的导出。
```

### Constraint DOC-0031 <a id="hcr-doc-0031"></a>

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

### Constraint DOC-0032 <a id="hcr-doc-0032"></a>

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

### Constraint DOC-0033 <a id="hcr-doc-0033"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:19`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（`openspec/specs/` 当前无已发布能力规格；此 Change 新增完整行为契约，不修改其他 Change 的历史文档）。
```

### Constraint DOC-0034 <a id="hcr-doc-0034"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/proposal.md:23`
- **Source Digest**: `981f54f65f011e5fd6dee64c8ed083c5357bfeee127aded9bcf59f963c4a216b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`internal/application/publication/**`，`web/src/features/settings/**`，`web/src/features/policy/PolicyView.vue`，`web/src/ui/{Popover,ModalDialog,ConfirmModal,Drawer}.vue` 及相关 subscriptions/publications/policy/probes 组件与测试。沿用 `GET/PUT /api/v1/policies/global-node-filter`、现有 `NodeFilterSpec`/resolver、Vue 现有 Teleport/原生 `<dialog>`/浏览器 top layer 与已有自适应 CSS token；不新建第二套筛选存储或引入额外浮层框架。四目标为 Mihomo、sing-box、Surge、Quantumult X；已发布直链继续提供创建时冻结的内容。上线另需授权。
```

### Constraint DOC-0035 <a id="hcr-doc-0035"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/proposal.md:1`
- **Source Digest**: `6cd62a5ff3de70227de7c8914e60fd88fd273bc066c06cbc31080eecdcf34943`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

用户反馈：在开启定时检测后，由于系统原先将整个探测周期（如 1 小时或 2 小时）作为单一粗粒度定时器，在未到达完整周期前系统完全不执行检测；且一旦时间到达，又将全量节点的所有探测类别同时并发倾泻入池，造成瞬时冲击与长周期等待。用户期望系统具备常态化轻量巡检能力（默认每 10 分钟或合适频次巡检一次数据库中的节点观测记录），仅将已过期或尚无观测记录的 (node, kind) 增量送检，而非全量一次性突发检测。
```

### Constraint DOC-0036 <a id="hcr-doc-0036"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/proposal.md:5`
- **Source Digest**: `6cd62a5ff3de70227de7c8914e60fd88fd273bc066c06cbc31080eecdcf34943`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### What Changes

1. **时钟解耦与轻量巡检**：解耦“固定 10 分钟轻量扫描时钟 (sweep interval)”与“用户配置的 `interval_seconds` 节点观测有效期”。`next_due_at` 统一表达下一次轻量扫描执行时间（当前时间 + 10 分钟），而非遥远的整段周期。
2. **启用与配置即刻巡检**：管理员开启计划或修改配置时，立即唤醒协调器执行巡检，不等待整段 interval。
3. **基于最新观测的增量过期判定**：巡检时比对每个激活节点在各配置 probe kind 下的最新观测完成时间 `ObservedAt`。若无观测记录，或距今已超过 `interval_seconds`，判定为到期；对失败观测引入合理冷却时间，避免失败节点每轮立即重试。
4. **单轮有界配额与公平覆盖**：设置单轮巡检入池任务上限（配额），优先调度从未观测的节点和观测时间最久远的节点，防止初始或突发情况（如 960 节点 * 5 kind）打爆探测队列；多轮巡检平滑收敛，杜绝饥饿。
5. **池内去重与防自旋**：复用现有 `EnqueuePeriodicDedupe` 队列模式与池内检查，已在探测池或执行中的节点/种类不重复入队；单批探测进行中或刚完成时不自旋重复派发。
6. **启动与迁移校正**：启动或 Recover 时，若历史数据中存留远在未来数小时的旧 `next_due_at`，自动校正为当前即刻巡检，平滑升级且不破坏迁移历史。
7. **前端交互与状态透传**：在 Web 探测控制台明示“每 10 分钟巡检过期节点”与“每节点/类别有效期 = 配置周期”，完整显示下次扫描时间；修复空闲时完全停止轮询问题（引入低频保底刷新）；手动立即触发后持续跟踪批次与运行状态，避免 50ms 快照丢失后续更新。
```

### Constraint DOC-0037 <a id="hcr-doc-0037"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/proposal.md:17`
- **Source Digest**: `6cd62a5ff3de70227de7c8914e60fd88fd273bc066c06cbc31080eecdcf34943`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `incremental-expiry-probe-sweep`: 提供基于固定轻量巡检时钟与逐节点/类别观测有效期的增量过期探测调度、有界配额分批入池、平滑升级校正与前端保底巡检感知。
```

### Constraint DOC-0038 <a id="hcr-doc-0038"></a>

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

### Constraint DOC-0039 <a id="hcr-doc-0039"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/proposal.md:1`
- **Source Digest**: `8d8a51d33ceaa7f0e99e60d9a492b9f1184057f2f28fd993ad75c47928c91a93`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### Why

当前 CSP Clean-Slate 已具备探测运行时、版本化 `ProbeObservation` 与节点台账的基础，但尚无独立、可审计的出口 IP 风险证据和消费该证据的准入、管理筛选与导出阻断合同。把风险分数直接写入现有 geo 或 capability verdict 会混淆“出口属性”“第三方信誉”和“本地准入决策”，并会使第三方配额、脱敏和失败降级无法单独治理。
本变更建立 CSP+IP Risk 的最小独立能力：以经被测节点出口的受控风险 enrichment 形成不可变、可过期的证据，再由确定性风险 policy 解释为管理筛选和发布准入结果。它不重建正在进行的 CSP Clean-Slate 施工 DAG，也不把网页抓取或第三方原始响应引入运行时。
```

### Constraint DOC-0040 <a id="hcr-doc-0040"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/proposal.md:7`
- **Source Digest**: `8d8a51d33ceaa7f0e99e60d9a492b9f1184057f2f28fd993ad75c47928c91a93`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### What Changes

- 新增版本化 `ip_risk` ProbeProfile、provider adapter 合同、按 provider/node 的配额与缓存边界，以及“无 API 凭据或 provider 不可用即 unknown”的安全降级。
- 新增不可变 `IPRiskObservation` 证据模型：保存 SHA-256 证据摘要、规范化分类、来源版本、观察时间和有限脱敏摘要；不保存完整 IP、第三方原始 payload、API key、cookie 或网页 HTML。
- 新增独立的风险 policy：将 observation 的 freshness、confidence、score、匿名化和网络属性确定性映射为 `allow`、`review`、`block` 或 `unknown`，且不覆盖原有 capability verdict。
- 扩展节点台账读取合同，使管理端可按风险决策和风险等级筛选，并以风险卡片和高风险提示回显可解释但已脱敏的结果。
- 新增策略组 admission 与 publication preflight：仅已启用的、版本化的风险 policy 可排除或隔离节点；无有效风险证据时按照显式 policy 处理，默认不把未知伪装为低风险。
- 明确禁止浏览器抓取 Scamalytics 等网页端；仅允许有凭据、可配置的官方 API adapter，且 provider 凭据只作为部署端 secret reference 使用。
- **BREAKING**：当操作者启用一个以 `block` 为结果的 IP risk admission 或 publication policy 时，原来会被选入该策略组或导出的节点 SHALL 被拒绝或隔离。未启用 policy 时既有选择和发布语义不改变。
```

### Constraint DOC-0041 <a id="hcr-doc-0041"></a>

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

### Constraint DOC-0042 <a id="hcr-doc-0042"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/proposal.md:25`
- **Source Digest**: `8d8a51d33ceaa7f0e99e60d9a492b9f1184057f2f28fd993ad75c47928c91a93`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### Modified Capabilities

- `probe-evidence-engine`: 扩展 probe kind、运行调度和证据读取，以承载独立的 `ip_risk` profile 而不改变既有 profile verdict 语义
- `subscription-inventory`: 扩展节点列表和详情读模型，使其能够提供已脱敏的当前 IP risk 摘要与服务端风险筛选
- `policy-tree-and-compiler`: 扩展 resolver 和 publication preflight，使已启用 risk policy 的 admission 决策能确定性地影响解析快照和发布
- `admin-workbench-experience`: 扩展管理工作台的节点风险筛选与可解释回显要求
```

### Constraint DOC-0043 <a id="hcr-doc-0043"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/proposal.md:32`
- **Source Digest**: `8d8a51d33ceaa7f0e99e60d9a492b9f1184057f2f28fd993ad75c47928c91a93`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### Impact

- 计划实现范围：`internal/domain/`、`internal/application/observation/`、`internal/application/probe/`、`internal/probe/profiles/`、新增 IP risk provider adapter、`internal/repository/sqlite/`、`migrations/`、`internal/transport/http/`、`internal/application/policy/`、`internal/resolver/`、`internal/application/publication/` 与未来 `web/src/features/`。
- 当前可核验基础：`internal/domain/probe.go:60-70` 已有不可变 observation 骨架；`internal/application/observation/service.go:52-105` 已以 SHA-256 digest 和脱敏 bounded summary 持久化；`internal/probe/singbox/runtime.go:148-208` 已将 dial/HTTP client 绑定到节点 outbound 并显式禁用宿主代理；`internal/domain/policy.go:70-87` 已有 admission rule 值对象。
- 新增 provider 可能产生 API 费用与配额消耗；默认关闭，必须由部署端配置 secret reference、预算和显式启用。不会新增网页抓取、浏览器自动化、旧数据库依赖、兼容层、双写或运行时数据迁移。
- 本变更只交付规划工件；不构建镜像、不启动容器、不写运行数据库、不切换流量或发布订阅。
```

### Constraint DOC-0044 <a id="hcr-doc-0044"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/proposal.md:1`
- **Source Digest**: `0707deb099f9f2dcac158a0e6b38ea2886bb60c43923843a71bfba221fcf4d14`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): policy group leaf node unpacking behavior pending user decision on Clarification Q1.
- **Original Source Text**:
```text
### Why

当前 `clash-sub-parser`（CSP）项目在演进中积累了 26 个历史 OpenSpec 变更集和 63 份分项规格，但根目录持续有效的主干能力规范（`openspec/specs/`）长期为空，导致全系统缺乏单一的权威规范事实源（Single Source of Truth）。历史提案中的问题分析有时被误当成用户原话，且部分业务行为（如策略组解包展开、成员混合交错顺序、历史空组数据恢复、非 Mihomo 目标空组处理等）存在实现缺口或未决歧义。
用户在本轮会话中明确指示：
> “按你的建议来，重写当前的SPC，再根据规格看当前是否符合，模糊整合询问我，然后再落地”
因此，本变更集作为**纯文档与规范特性包（Documentation Only）**，负责整合并重写全系统 Living Specification，完成客观代码符合度审计，编制待决业务问题集供用户裁决，为后续业务代码落地筑牢契约根基。
```

### Constraint DOC-0045 <a id="hcr-doc-0045"></a>

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

### Constraint DOC-0046 <a id="hcr-doc-0046"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/proposal.md:27`
- **Source Digest**: `0707deb099f9f2dcac158a0e6b38ea2886bb60c43923843a71bfba221fcf4d14`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `living-spec-consolidation`: 建立项目持续主干能力规范管理体系，包含 6 大业务领域 Living Spec、四级来源确权、实机符合度审计矩阵及跨变更覆盖追溯能力。
```

### Constraint DOC-0047 <a id="hcr-doc-0047"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/proposal.md:31`
- **Source Digest**: `0707deb099f9f2dcac158a0e6b38ea2886bb60c43923843a71bfba221fcf4d14`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本变更集保留全部既有 26 个变更集，不作归档或删除。
```

### Constraint DOC-0048 <a id="hcr-doc-0048"></a>

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

### Constraint DOC-0049 <a id="hcr-doc-0049"></a>

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

### Constraint DOC-0050 <a id="hcr-doc-0050"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/proposal.md:18`
- **Source Digest**: `674c4490d44e3254aac0b13e6ed37ddac12bf58d62ceb5db7a571cc71f43fb1c`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### What Changes

- **后端实现统一节点池双端优先级队列（Node Probe Pool / Priority Deque）与去重/插队机制（`internal/probe/queue/**`、`internal/application/probe/**`）**：
- 在调度器与探针执行层构建线程安全的**节点池（Node Probe Pool）**状态机，跟踪每个入池节点的实时状态（`queued` 排队等待 / `probing` 检测中）、待执行探测维度集合与关联的等待方；
- **定时测速去重入池（Tail Enqueue with Deduplication）**：当定时测速（`PeriodicCoordinator` / `ActorScope == "system:periodic-probe"`）添加节点到节点池时，按顺序推入队列尾部；**若目标节点已经在节点池中（无论处于 `queued` 还是 `probing`），直接静默去重跳过，绝不重复添加，亦不触发 `ErrNodeConflict` 报错**；
- **手动检测插在队列最前面（Front-of-Queue Priority Preemption & Promotion）**：当手动发起检测（单节点重测、批量已选节点测速、手动一键全量测速）时，以高优先级推入节点池**队列最前面（队首）**优先调度；若某目标节点已在节点池等待队列中，立即将其**提升（Promote）至队列最前面**并合并待测维度 `kinds`；若该节点此刻正处于 `probing`（检测中），则复用/合并当前检测或紧接队首补测新增维度，绝不报错中断。
- **后端新增节点“检测中（`probing`）”实时状态注入与 `GET /api/v1/probes/pool` 5 大指标接口（`internal/application/inventory/**`、`internal/application/probe/**`、`internal/transport/http/**`、`cmd/csp/main.go`）**：
- 在 `inventory.NodeView`（及 `NodeDetail`）中新增 `probe_state` 字段（`"probing"` | `"queued"` | `"idle"`），并在节点正处于节点池检测中时将实时状态反映到 `NodeView`（当节点处于检测中时 `probe_state = "probing"`，`health_status` 支持返回 `"probing"` 或保留历史观测同时由 `probe_state` 驱动前端优先展示「检测中」）；
- 新增 `GET /api/v1/probes/pool`（并在 `POST /api/v1/probes/runs` 与新增的 `POST /api/v1/probes/schedule/trigger` 响应中同步返回最新池快照），实时返回节点池与全网活跃节点 5 大核心统计指标：
1. `queue_nodes_count`：**当前队列中的节点数**（节点池内节点总数 = `probing_count` 正在检测数 + `queued_waiting_count` 排队等待数）；
2. `untested_count`：**未测数**（无任何探测观测记录的活跃节点数）；
3. `total_count`：**总数**（全网活跃节点总数）；
4. `unavailable_count`：**不可用数**（最新连通结论为异常/不可达 `unhealthy` 的活跃节点数）；
5. `available_count`：**可用数**（最新连通结论为正常 `healthy` 或降级可用 `degraded` 的活跃节点数，同时返回 `healthy_count` 与 `degraded_count` 明细）；
6. `probing_node_ids`（当前正在检测中的节点 LogicalID 列表）与 `queued_node_ids`（按当前节点池队首到队尾顺序排列的等待节点 LogicalID 列表）。
- **前端重设计美观恰当的“节点池与全网检测状态看板”及“检测中”状态体系（`web/src/features/probes/**`、`web/src/features/nodes/**`、`web/src/locales/**`）**：
- 在 `ProbesView.vue` 第一视觉层级打造直观、高辨识度的**节点池实时检测状态看板（Node Pool Status Dashboard）**，醒目展示用户指定的 5 项核心指标：**当前队列中的节点数**（含检测中/排队等待细分与队列消化进度条）、**未测数**、**总数**、**不可用数**、**可用数**（含可用率与正常/降级细分），并配套定时自动入池策略卡（含「立即入池巡检」按钮与一键启停）；
- 点击看板上的「当前队列中 / 检测中 / 可用 / 不可用 / 未测 / 总数」指标卡片可直接联动下方工作台表格快速筛选对应状态的节点；
- 状态筛选器下拉框、状态徽章组件（`nodeView.ts` / `ProbesView.vue` / `NodesView.vue` / `ProbeEvidenceSheet.vue`）全面新增 **「检测中」**（`probing`，带脉冲/旋转动效）与 **「队列中」**（`queued`）状态展示；
- 手动触发单节点重测或批量测速时，节点立即切换为「检测中 / 队列最前」状态并提示“已插队至节点池最前面优先检测”，轮询实时刷新节点池计数与节点最新延迟。
```

### Constraint DOC-0051 <a id="hcr-doc-0051"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/proposal.md:41`
- **Source Digest**: `674c4490d44e3254aac0b13e6ed37ddac12bf58d62ceb5db7a571cc71f43fb1c`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `node-probe-pool`: 统一节点池双端优先级队列（定时测速去重入池、手动检测插队最前）、节点“检测中（`probing`）”实时状态透传、`GET /api/v1/probes/pool` 5 大核心统计指标契约，以及前端节点池状态看板与“检测中”全链路视觉交互规格。
```

### Constraint DOC-0052 <a id="hcr-doc-0052"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/proposal.md:45`
- **Source Digest**: `674c4490d44e3254aac0b13e6ed37ddac12bf58d62ceb5db7a571cc71f43fb1c`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `node-probe-pool` 能力规格建立完整契约）。
```

### Constraint DOC-0053 <a id="hcr-doc-0053"></a>

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

### Constraint DOC-0054 <a id="hcr-doc-0054"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:1`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前探测主要通过手动 `/probes/runs` 提交，缺少面向仓库激活节点的可配置、可恢复周期任务；策略组仅保存显式成员引用，无法按节点属性和探测结果筛选，也没有先于策略组筛选的全局筛选。用户另报告“csp出错”，但未提供当次请求、时间与错误日志；已核实历史 `target mihomo cannot render PROCESS-NAME` 修复位于 `7289b4c`，不得将该旧错误冒充本次根因。
```

### Constraint DOC-0055 <a id="hcr-doc-0055"></a>

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

### Constraint DOC-0056 <a id="hcr-doc-0056"></a>

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

### Constraint DOC-0057 <a id="hcr-doc-0057"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:19`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 没有已发布的同名主规格；此次以新增能力建立完整契约）。
```

### Constraint DOC-0058 <a id="hcr-doc-0058"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/proposal.md:23`
- **Source Digest**: `d0b0ccbf91a00de827c626af1477ec9d9eb7f3015f1d81c5ec95b5e7f3ee3786`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`internal/domain/{probe,node,policy,ports}.go`、`internal/application/{probe,policy,publication}`、`internal/probe/queue`、`internal/resolver`、`internal/repository/sqlite`、`internal/transport/http`、`cmd/csp/main.go`、`migrations`、`web/src/features/{probes,policy}` 与 API 类型/测试。沿用 Go 标准 `context`/`time`、现有 `modernc.org/sqlite` 事务和索引、现有 sing-box runner 与有界公平队列，不引入第二套调度或表达式解析体系。上线、数据库备份和流量接入另需生产授权。
```

### Constraint DOC-0059 <a id="hcr-doc-0059"></a>

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

### Constraint DOC-0060 <a id="hcr-doc-0060"></a>

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

### Constraint DOC-0061 <a id="hcr-doc-0061"></a>

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

### Constraint DOC-0062 <a id="hcr-doc-0062"></a>

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

### Constraint DOC-0063 <a id="hcr-doc-0063"></a>

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

### Constraint DOC-0064 <a id="hcr-doc-0064"></a>

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

### Constraint DOC-0065 <a id="hcr-doc-0065"></a>

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

### Constraint DOC-0066 <a id="hcr-doc-0066"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/proposal.md:35`
- **Source Digest**: `e7d7285a8bbcdbb69ed3dd831cc2bd335840e611c7f656ea50b9c8570ab50569`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。
```

### Constraint DOC-0067 <a id="hcr-doc-0067"></a>

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

### Constraint DOC-0068 <a id="hcr-doc-0068"></a>

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

### Constraint DOC-0069 <a id="hcr-doc-0069"></a>

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

### Constraint DOC-0070 <a id="hcr-doc-0070"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/proposal.md:35`
- **Source Digest**: `d40fc8557e37d1604e0dedf47a4b8ad9b05e7e8a652337c0663b4df15b2651c5`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `probe-engine-ux`: 探针引擎后端节点探测读模型打通、全量测速与带宽测速执行器修复、以及前端以“节点与人类直觉”为核心的测速与可用性工作台全量重构契约。
```

### Constraint DOC-0071 <a id="hcr-doc-0071"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/proposal.md:39`
- **Source Digest**: `d40fc8557e37d1604e0dedf47a4b8ad9b05e7e8a652337c0663b4df15b2651c5`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `probe-engine-ux` 能力规格建立完整契约）。
```

### Constraint DOC-0072 <a id="hcr-doc-0072"></a>

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

### Constraint DOC-0073 <a id="hcr-doc-0073"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/proposal.md:1`
- **Source Digest**: `b4b1dd6f21cb5c5c37acf09fd40c7019621b1df4c3269cbe9f311affc6f61e2a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

用户反馈节点“全部异常”，并要求探针每分钟更新状态：有状态变化才检测，手动刷新状态时该检测的也要检测。现有观测能证明的是“最近 21 个启用节点 baseline 均记录 error”，而非 21 条线路均不可用：其中 18 条记录为 `credentials_unavailable`（实际有配置凭据；6 条 hysteria2 因不安全 TLS 被安全拒绝，12 条 vless 含 UUID/vision/reality，需校准构建配置），1 条 `private_target_rejected`，另 2 条 `transport_error` 还缺足以断言根因的证据。必须先区分探针能力/安全策略、未知与线路真实失败，再调整巡检，避免把节点误染全红或全绿。
```

### Constraint DOC-0074 <a id="hcr-doc-0074"></a>

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

### Constraint DOC-0075 <a id="hcr-doc-0075"></a>

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

### Constraint DOC-0076 <a id="hcr-doc-0076"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/proposal.md:20`
- **Source Digest**: `b4b1dd6f21cb5c5c37acf09fd40c7019621b1df4c3269cbe9f311affc6f61e2a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无已归档的 `openspec/specs/` 能力；本 Change 与历史未归档的 `csp-incremental-expiry-probe-sweep` 契约的 10 分钟默认值及控制台文字存在明确后继覆盖关系，不修改/复用其历史 Change。
```

### Constraint DOC-0077 <a id="hcr-doc-0077"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/proposal.md:24`
- **Source Digest**: `b4b1dd6f21cb5c5c37acf09fd40c7019621b1df4c3269cbe9f311affc6f61e2a`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### Impact

后端 `internal/probe/singbox/`、`internal/application/probe/`、`internal/application/inventory/`、必要的 `internal/domain/`、SQLite 仓储及显式装配调用点；Web 探针控制台与节点台账状态展示、必要 HTTP 接口。优先复用现有 `ListLatestByNodes`、CAS 租约、`EnqueuePeriodicDedupe`、安全拨号器和既有状态字段；若需要存储连接参数签名，仅添加有版本且向后兼容的持久化迁移，不把 `nodes.updated_at`、抓取时间或观测时间当作连接变化证明。不增加破坏性字段或升级用户观测为伪造成功。
```

### Constraint DOC-0078 <a id="hcr-doc-0078"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/proposal.md:1`
- **Source Digest**: `30357f95d591623d060274b4f4e7235934230682c86ace3a94f6de0662859c05`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### Why

在完成探针准确性与执行效率专项研究后，代码取证确认当前探针执行器（`internal/application/probe/runner.go`、`internal/probe/profiles/profiles.go`）与 SQLite 观测仓储（`internal/repository/sqlite/probes.go`）存在四项影响探测真实性与运行效率的核心缺陷：（1）`runner.go` 对所有 `ProbeKind` 只要收到 `HTTP 200/204` 即无条件标记 `ContractMatched = true`，导致 `baseline` 误将 `HTTP 200` 伪装页判为可用、`geo` 不校验出口 IP 与国家代码、`streaming`/`ai` 仅凭首页 `200` 误报解锁、`ip_risk` 仅凭 Cloudflare `trace` 出口身份误标风险可用；（2）`speed` 在读取响应体 `readBoundedResponse` 之前就结束 `latency` 计时，却用完整响应体字节数计算 `throughput_kbps`，且未拒绝空响应或极短伪响应；（3）同一 `ProbeRun` 内每个 `(node, kind)` 任务各自独立调用 `r.dialer` 与 `cleanup`，导致同一节点在多维探测中重复创建和销毁内存 `sing-box` 实例；（4）`sqlite/probes.go:380-465` 的 `ListLatestByNodes` 从数据库拉取匹配节点的全部历史观测行到 Go 内存再去重。现需建立严格契约，一次性闭环探针判定真实性、同 Run 节点客户端复用与 SQLite 最新观测查询效率。
```

### Constraint DOC-0079 <a id="hcr-doc-0079"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/proposal.md:5`
- **Source Digest**: `30357f95d591623d060274b4f4e7235934230682c86ace3a94f6de0662859c05`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### What Changes

- **收紧各 `ProbeKind` 真实性评估契约，杜绝伪阳性误判（`internal/probe/profiles/profiles.go`、`internal/application/probe/runner.go`）**：
- **`baseline` 严格 204 空响应契约**：仅当 `StatusCode == 204 (http.StatusNoContent)` 且响应体为空（`len(Body) == 0 && BytesRead == 0`）时才满足契约并判定为 `available`（`reason=contract_matched`）；遇到明确限制信号（HTTP `401`/`403` 或挑战/登录拦截标记）判定为 `restricted`（`reason=access_restricted`）；任何 `HTTP 200`（无论正文为空、`"OK"` 或未命中拦截词的网页）及非空 `204` 一律判定为 `unknown`（`reason=contract_drift`）；
- **`geo` 出口 IP + 国家代码双重验证契约**：复用既有 `internal/probe/identity.ExtractCandidate` 解析响应负载，仅当 `StatusCode == 200` 且同时提取出合法 IP 地址（`net.ParseIP != nil`）与 2 位 ISO 3166-1 alpha-2 国家代码时才判定为 `available`；空响应、纯文本 `"OK"`、缺失 IP 或非法国家代码一律判定为 `unknown`（`reason=contract_drift`）；
- **`streaming` 与 `ai` 保守真实性契约（严禁伪解锁与私有猜测规则）**：在未有可证明的权威业务能力契约时，**绝不判定为 `available`**；明确限制信号（HTTP `401`/`403`/`451` 或 `hasRestrictionMarker` 挑战/登录标记）判定为 `restricted`（`reason=access_restricted`），传输/DNS/超时故障判定为 `error`（`reason=transport_error`），其余响应（含首页 `HTTP 200`、`204` 空响应或普通非空响应）一律判定为 `unknown`（`reason=contract_drift`）；**严禁编写未经证实的 Netflix/OpenAI 私有规则，严禁照搬 `subs-check-pro v3.2.1` 的误判实现**；
- **`ip_risk` 出口身份与风险评分边界分离契约**：明确出口身份（`cloudflare.com/cdn-cgi/trace` 解析出的 IP + 国家代码）仅用于证明出口身份存在（`ExitIdentityMissing = false`），绝不等于 IP 风险评分；当响应无法提取有效出口身份时标记 `ExitIdentityMissing = true` 并返回 `unknown`（`reason=missing_exit_identity`）；在未接入并验证真实风险情报源（`iprisk.Provider`）产出有效风险观测时，`ip_risk` 探测结果必须保持 `ContractMatched = false` 并返回 `unknown`（`reason=contract_drift`），绝不因 `trace` 返回 `200` 而误标为 `available`；
- **`speed` 完整传输计时、最小有效负载门槛与超限预算保持契约**：将 `speed` 耗时计算移至 `readBoundedResponse` 完整读取响应体之后，确保 `latency_ms` 与 `throughput_kbps = (BytesRead * 8) / latency_ms` 真实反映完整负载传输耗时；要求 `StatusCode == 200` 且实际读取字节数达到最小有效测速负载门槛（`BytesRead >= MinValidSpeedBytes`，拒绝 `204`、空响应及 `< 1024B` 的极短伪响应，不满足时判为 `unknown` / `reason=contract_drift`）；完整保留超过 `MaxBytesPerRequest` / `MaxBytesPerRun` 时返回 `error`（`reason=speed_budget_exceeded`）的既有超限预算错误契约。
- **实现同 `ProbeRun` 同节点客户端生命周期单次创建与安全复用（`internal/application/probe/runner.go`）**：
- 在单个 `ProbeRun` 内按 `node.LogicalID` 管理节点探测客户端生命周期：同一 Run 内同一节点无论执行单阶段多维任务还是两阶段（Phase 1 `baseline` -> Phase 2 扩展维度）任务，最多调用一次 `r.dialer` 创建 `*http.Client` 与底层内存 `sing-box` 实例；
- 节点客户端的 `cleanup()` 必须且只能在该节点于当前 Run 的全部已调度任务及其 `OnComplete` 回调执行完毕后调用一次（`sync.Once`）；若节点在 Phase 1 `baseline` 未通过（不进入 Phase 2），在 Phase 1 同步点后立即释放；在 Run 取消（`context.Canceled`）、超时（`context.DeadlineExceeded`）、任务提交失败或致命拨号错误等所有退出路径上，等待在途任务回调收敛后确定性执行全部已创建客户端的 `cleanup()`，杜绝协程或套接字泄漏；
- 保持既有队列手动插队优先（`queue.EnqueueManualPreemptFront`）与定时巡检去重入池（`queue.EnqueuePeriodicDedupe`）语义不变。
- **重构 SQLite `ListLatestByNodes` 为数据库内按 `(node_logical_id, kind)` 精确选最新并补充离线基准与回归测试（`internal/repository/sqlite/probes.go`、`internal/repository/sqlite/*_test.go`）**：
- 在 `internal/repository/sqlite/probes.go` 中将 `ListLatestByNodes` 从“拉取全量历史观测并在 Go 内存去重”重构为 SQLite 数据库内使用窗口函数 `ROW_NUMBER() OVER (PARTITION BY node_logical_id, kind ORDER BY observed_at DESC, id DESC) AS rn` 且仅篩选 `WHERE rn = 1`（并确保 `(node_logical_id, kind, observed_at DESC, id DESC)` 复合索引就绪），使返回给 Go 层的行数严格受限于 `<= len(nodeLogicalIDs) × len(kinds)`；
- 严格保证相同 `(node_logical_id, kind)` 在相同 `observed_at` 时间戳下按 `id DESC` 执行确定性决胜（tie-break）；
- 新增离线基准测试（Benchmark）与多历史版本、同秒级时间戳 tie-break、分批（`> 100` 节点 chunking）及 `kinds` 过滤回归单元测试。
- **严守安全与接口签名不变量**：
- 完整保留 `NewSafeNodeDialer` 的 Server DNS Pinning（域名前置解析校验并固定 IP）、SSRF 公网地址校验、HTTP 重定向禁止（`http.ErrUseLastResponse`）以及 `domain.ComputeProbeEvidenceDigest` 证据签名计算字段与公开接口签名。
```

### Constraint DOC-0080 <a id="hcr-doc-0080"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/proposal.md:26`
- **Source Digest**: `30357f95d591623d060274b4f4e7235934230682c86ace3a94f6de0662859c05`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### New Capabilities

- `probe-truth-and-efficiency`: 探针各维度（`baseline`、`geo`、`streaming`、`ai`、`ip_risk`、`speed`）真实性判定契约、完整传输计时、同 `ProbeRun` 同节点客户端单次拨号生命周期复用，以及 SQLite `ListLatestByNodes` 数据库内精确选最新与离线基准测试规格。
```

### Constraint DOC-0081 <a id="hcr-doc-0081"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/proposal.md:29`
- **Source Digest**: `30357f95d591623d060274b4f4e7235934230682c86ace3a94f6de0662859c05`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `probe-truth-and-efficiency` 建立完整能力规格契约）。
```

### Constraint DOC-0082 <a id="hcr-doc-0082"></a>

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

### Constraint DOC-0083 <a id="hcr-doc-0083"></a>

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

### Constraint DOC-0084 <a id="hcr-doc-0084"></a>

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

### Constraint DOC-0085 <a id="hcr-doc-0085"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:18`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `publication-theme-probe-recovery`: 发布恢复、移动与桌面可用视觉、主题、真实安全探针及停服生命周期、同源隔离预览与独立黑盒验收。
```

### Constraint DOC-0086 <a id="hcr-doc-0086"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:22`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无。主规格未同步；OpenSpec 严格校验仅表明规划工件有效。
```

### Constraint DOC-0087 <a id="hcr-doc-0087"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/proposal.md:26`
- **Source Digest**: `248014f5ea9b95431d84c3a7242c1a0c4abc8b57007a4919e334920934f90a3f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

规划与验收涉及 `web/vite.config.ts`、`web/dist`、`internal/webassets/dist`、`internal/webassets/embed.go`（如适用）、隔离工作区/预览 manifest、Go/web 测试及前端相关视图与测试。主仓 `internal/webassets/dist` 当前为未跟踪用户资产，施工/审查者必须保护原样直到来源取证和明确授权；不碰生产 17000/18080、生产卷、不部署、不提交。此次策略机只写同一 Change 的现有 OpenSpec 工件。
```

### Constraint DOC-0088 <a id="hcr-doc-0088"></a>

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

### Constraint DOC-0089 <a id="hcr-doc-0089"></a>

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

### Constraint DOC-0090 <a id="hcr-doc-0090"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/proposal.md:40`
- **Source Digest**: `0b7a3f2008751c66cb95f1576cc0d91cee3bfc0efc6de16e6bd1ca9ad6cc3b60`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `subscription-proxy-and-verge-headers`: Verge 订阅请求头标准化、全局与订阅级出站代理分级解析、受控宿主代理接入、以及严格的分离式代理 SSRF 安全模型。
```

### Constraint DOC-0091 <a id="hcr-doc-0091"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/proposal.md:44`
- **Source Digest**: `0b7a3f2008751c66cb95f1576cc0d91cee3bfc0efc6de16e6bd1ca9ad6cc3b60`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（`openspec/specs/` 下无对应已归档主规格，本次通过新增能力规格建立权威契约）。
```

### Constraint DOC-0092 <a id="hcr-doc-0092"></a>

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

### Constraint DOC-0093 <a id="hcr-doc-0093"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/proposal.md:1`
- **Source Digest**: `6bec759645a452410b81a76e94b4830d05fe72a32f46096273eca1af7bcecfaf`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): pure-node export empty-group validation bypass pending Clarification Q4.
- **Original Source Text**:
```text
### Why

根据用户在实机使用中的三项明确指令与取证结果：
1. **“校验拓扑图，报错”**：策略树页面点击“校验拓扑图”时，前端 `usePolicy.ts` 直接将含内嵌 `edges` 及视图字段的 `groups.value` 发送给 `POST /api/v1/policies/validate`；后端 `policies.go` 的 `validate` 处理器不仅因 `domain.NodeGroup` 不含 `edges` 字段且 `decodeJSON` 启用 `DisallowUnknownFields()` 触发 HTTP 422 `invalid_json`，还因为 `_ = decodeJSON(w, r, &body)` 吞掉错误未 `return` 导致向同一 `http.ResponseWriter` 二次写入响应。
2. **“订阅发布只导出mihomo，其他是导出节点格式，忽略规则”**：当前编译器对 `singbox`、`surge`、`qx` 强行校验并渲染策略组（`Groups`）与路由规则（`Rules`），一旦策略树使用了 `fallback`/`loadbalance` 策略组或 `GEOSITE`/`PROCESS-NAME`/`RULE-SET` 等规则，非 Mihomo 目标即报 422 拒绝导出。用户明确要求仅 `mihomo` 导出含策略组与规则的完整配置，`singbox`、`surge`、`qx` 仅作为纯节点格式导出并彻底忽略策略组与规则。
3. **“系统语言很多没有中文化”**：前端 `web/src/locales/index.ts` 默认语言仍为 `'en-US'`，且 `ErrorStateCard.vue`、`PolicyView.vue`、`GroupCard.vue`、`NodesView.vue`、`ProbesView.vue`、`ProbeEvidenceSheet.vue`、`PublicationsView.vue`、`SettingsView.vue` 等大量组件与 TS 辅助模块存在硬编码英文、裸露后端枚举与英文错误提示。
```

### Constraint DOC-0094 <a id="hcr-doc-0094"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/proposal.md:8`
- **Source Digest**: `6bec759645a452410b81a76e94b4830d05fe72a32f46096273eca1af7bcecfaf`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): pure-node export empty-group validation bypass pending Clarification Q4.
- **Original Source Text**:
```text
### What Changes

- **修复策略拓扑图校验接口与前端请求（Topology Validation Zero-Payload Simplification）**：
- 后端 `internal/transport/http/policies.go`（`validate`）：移除冗余的 `validateGraphRequest` 请求体解析逻辑，直接调用 `h.service.ValidateGraph(r.Context())` 从后端数据库加载完整策略组、边与活动修订规则执行权威拓扑校验。
- 前端 `web/src/features/policy/usePolicy.ts`（`validateGraph`）：直接无参调用 `api.post<ValidationResult>('/api/v1/policies/validate')`，不再将前端内存中的边和点重复传给后端。
- **落实“仅 Mihomo 导出完整配置，其他目标仅导出纯节点格式并忽略规则”（Mihomo Full Config vs Node-Only Export Targets）**：
- 后端 `internal/compiler/{compiler,mihomo,singbox,surge,qx}.go`：仅 `mihomo` 校验并渲染完整客户端配置（`Nodes` + `Groups` + `Rules`）；当 `target != domain.TargetMihomo`（即 `singbox`、`surge`、`qx`）时，`compiler.validate` 完全忽略 `snapshot.Groups` 与 `snapshot.Rules`（不对策略组类型、成员引用或路由规则做任何能力校验）；`renderSingBox` 仅输出纯节点 JSON（`{"outbounds": [...], "endpoints": [...]}`，不含策略组 outbound 与 `route`），`renderSurge` 仅输出纯节点行（不含 `[General]`/`[Proxy]`/`[Proxy Group]`/`[Rule]` 段落，并补齐 Surge 5 原生支持的 `hysteria2`、`tuic`、`wireguard` 节点行渲染），`renderQuantumultX` 仅输出纯节点行（不含 `[general]`/`[server_local]`/`[policy]`/`[filter_local]` 段落）。
- 后端 `internal/application/publication/service.go`（`evaluatePreflight`）：预检与发布感知 `target`，对非 `mihomo` 目标忽略 `empty_routed_group` 等策略组/规则级阻断诊断，仅校验导出节点本身。
- 前端 `web/src/features/publications/` 与 `web/src/features/policy/`：同步更新目标能力边界说明与文案，明确标识 `Mihomo` 为完整配置（含策略组与规则），`sing-box`、`Surge`、`Quantumult X` 为纯节点格式导出（忽略策略组与规则）。
- **全站默认简体中文与全面中文化（Comprehensive `zh-CN` Localization）**：
- `web/src/locales/index.ts` 将默认语言与回退语言统一设为 `'zh-CN'`；在 `web/src/locales/messages.ts`（及 `zh-CN.ts` / `en-US.ts` 模块导出）补齐全部缺失词条。
- 全面改造 `App.vue`、`ErrorStateCard.vue`、`DashboardView.vue`、`PolicyView.vue`、`PolicyEditorSheet.vue`、`GroupCard.vue`、`usePolicy.ts`、`policyTypes.ts`、`NodesView.vue`、`nodeView.ts`、`useNodes.ts`、`ProbesView.vue`、`ProbeRunCard.vue`、`ProbeEvidenceSheet.vue`、`useProbes.ts`、`probeTypes.ts`、`PublicationsView.vue`、`usePublications.ts`、`publicationTypes.ts`、`SubscriptionsView.vue`、`SubscriptionConfigDrawer.vue`、`useSubscriptions.ts`、`SettingsView.vue` 中的硬编码英文，统一将协议、策略组类型（手动选择/自动测速/故障转移/负载均衡）、规则动作、探针类型/状态/结论、风险等级与常见 API 错误码映射为地道清晰的中文展示。
```

### Constraint DOC-0095 <a id="hcr-doc-0095"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/proposal.md:23`
- **Source Digest**: `6bec759645a452410b81a76e94b4830d05fe72a32f46096273eca1af7bcecfaf`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): pure-node export empty-group validation bypass pending Clarification Q4.
- **Original Source Text**:
```text
### New Capabilities

- `topology-i18n-export`: 策略拓扑图校验修复、Mihomo 完整配置与非 Mihomo 目标纯节点格式导出（忽略策略组与规则）、以及前端默认 `zh-CN` 全站中文化契约。
```

### Constraint DOC-0096 <a id="hcr-doc-0096"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/proposal.md:27`
- **Source Digest**: `6bec759645a452410b81a76e94b4830d05fe72a32f46096273eca1af7bcecfaf`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `topology-i18n-export` 能力规格建立完整契约）。
```

### Constraint DOC-0097 <a id="hcr-doc-0097"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/proposal.md:31`
- **Source Digest**: `6bec759645a452410b81a76e94b4830d05fe72a32f46096273eca1af7bcecfaf`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

`internal/transport/http/policies*.go`、`internal/compiler/{compiler,mihomo,singbox,surge,qx}*.go`、`internal/compiler/testdata/golden/*.golden`、`internal/application/publication/{service,preflight_test,service_test}.go`、`internal/integration/*.go` 及 `web/src/**`。仅在隔离验证环境构建与测试，不直接操作生产运行容器。
```

### Constraint DOC-0098 <a id="hcr-doc-0098"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:1`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前用户明确要求实际修复 CSP 管理界面故障，而旧合同将 ADB/USB 真机连接设为前置门槛，已被废除。已报告 `POST /api/v1/publications/preview` 返回 500、模板生成边界不清、`web/src/locales/messages.ts` 缺失 130+ 中英文键、异常状态缺乏可操作重试以及 PolicyView 拓扑与抽屉不联动。需要保留已经完成的隔离取证工作，立即把剩余施工和复验转向这些可验证缺陷。
```

### Constraint DOC-0099 <a id="hcr-doc-0099"></a>

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

### Constraint DOC-0100 <a id="hcr-doc-0100"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:15`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `admin-ui-device-quality`: CSP 管理界面的发布预览、国际化、失败恢复、策略联动及跨视口浏览器验收契约。
```

### Constraint DOC-0101 <a id="hcr-doc-0101"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/proposal.md:19`
- **Source Digest**: `2122c6ecd2c97bf863af0b3938ad69d6d7026cc3bf3b16121f239a5e16b8419b`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。与其他进行中 Change 的 API/鉴权契约冲突时先核对归属，不擅自更改其工件。
```

### Constraint DOC-0102 <a id="hcr-doc-0102"></a>

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

### Constraint DOC-0103 <a id="hcr-doc-0103"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:1`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: fixed 44px touch target replaced by fluid design tokens and responsive CSS rules per FR-UI-02 (no automatic old 44px revived active).
- **Original Source Text**:
```text
### Why

当前管理界面既有 DaisyUI/Tailwind 语义层，又在全局 CSS、Dock、弹层和卡片布局中散布固定像素、视口百分比与 `!important` 补丁。统一把目标强制设为固定 44px 已被用户否决：需要由顶层设计契约治理重复的呈现硬编码，以真实可用性而非机械尺寸作为验收。
```

### Constraint DOC-0104 <a id="hcr-doc-0104"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:5`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: fixed 44px touch target replaced by fluid design tokens and responsive CSS rules per FR-UI-02 (no automatic old 44px revived active).
- **Original Source Text**:
```text
### What Changes

- 建立流式布局、语义交互区、设计原语与例外登记的跨页面契约；优先复用 DaisyUI/Tailwind 与现有组件，不造全套新框架。
- 对全局小屏强制 44px 规则、Dock、抽屉/弹层、卡片最小宽和预览高度等已确认重复布局常量做有界治理；不得简单替换成另一批固定数值。
- 加入按实际交互/滚动/字体缩放验证的定向回归与例外说明，不引入所有像素字面量必然失败的机械门禁。
- 与进行中的 `csp-ui-device-governance-v1` 共享视口质量目标；旧 Change 的统一 44px 门禁已协调为语义交互可达、无遮挡及实测体验的验收要求。两套合同仍须在最终合并态共同复验。
```

### Constraint DOC-0105 <a id="hcr-doc-0105"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:14`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `admin-ui-adaptive-layout`: 界面响应式排布、语义交互可达性、设计原语复用及跨视口实证验收。
```

### Constraint DOC-0106 <a id="hcr-doc-0106"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:17`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Modified Capabilities

- 无。`openspec/specs/` 目前没有已有主规格；进行中的旧 Change 是另一个尚未合并的 delta，不当作主规格直接改写。
```

### Constraint DOC-0107 <a id="hcr-doc-0107"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/proposal.md:20`
- **Source Digest**: `cb109bdf9a58f3d1557055a19ae1518e0e58bbcd62c3c55f46603ed794e61bdb`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

主要涉及 `web/src/style.css`、`web/tailwind.config.js`、`web/src/ui/ResponsiveGrid.vue` 与弹层组件、`web/src/App.vue`、引用方和定向 UI/Playwright 测试。无业务 API、服务端算法或数据迁移；不改部署与生产数据。工作区基线 HEAD `7f827e2`，初始记录 `git status --short` 726 条；`web/src/` 当前是未跟踪目录，实施时必须保留其中所有用户既有工作。规划不授权实施、生产写入或设备实屏操作。
```

### Constraint DOC-0108 <a id="hcr-doc-0108"></a>

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

### Constraint DOC-0109 <a id="hcr-doc-0109"></a>

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

### Constraint DOC-0110 <a id="hcr-doc-0110"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/proposal.md:20`
- **Source Digest**: `bf00aaa136aad994071b0ffc5fc166c1dfddb9ae26767a969fdcf7400ce6185a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### New Capabilities

- `live-acceptance-and-qualified-recovery`: 真实带身份正文验收、有界实网、因果充分恢复及可回滚发布。
```

### Constraint DOC-0111 <a id="hcr-doc-0111"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/proposal.md:27`
- **Source Digest**: `bf00aaa136aad994071b0ffc5fc166c1dfddb9ae26767a969fdcf7400ce6185a`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Impact

限定 CSP 的 `cmd/csp-live-acceptance/main.go`、runner、对应 CLI/e2e 测试和两份运行手册；恢复实现只在新证据揭示具体缺口时定点修复。生产只涉及已授权目标 app。Pi `/home/service/pi` 及外置 bridge 本轮只读取证，不写；CLI 确认 bridge 所属 OpenSpec 根是 `/home/service/hermes/hermes-home`，并非独立子目录根。本轮无 UI 变更，不派无 URL critic。规划 READY 不等于业务 PASS。
```

### Constraint DOC-0112 <a id="hcr-doc-0112"></a>

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

### Constraint DOC-0113 <a id="hcr-doc-0113"></a>

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

### Constraint DOC-0114 <a id="hcr-doc-0114"></a>

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

### Constraint DOC-0115 <a id="hcr-doc-0115"></a>

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

### Constraint DOC-0116 <a id="hcr-doc-0116"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/proposal.md:1`
- **Source Digest**: `50d889afdac5ac7fb97d00fa34d09c7c045cd99524a54783b2023e29b1e5b610`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Why

Phase 2 原作者勾选 10/12 后独立 reviewer REJECTED。实读发现五类未闭环行为：凭据端点身份缺少可信绑定，节点 UI 展示/保存虚构资料，RULE-SET 假资源 URL，发布绑定依赖进程 map/审计文本，风险 SQL 多 provider 计数与历史扫描不可靠。既有勾选和旧测试不能代替验收，撤销相关勾选并在同一 Change 根因整改；不把所有审查意见未经核验当成事实。
```

### Constraint DOC-0117 <a id="hcr-doc-0117"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/proposal.md:5`
- **Source Digest**: `50d889afdac5ac7fb97d00fa34d09c7c045cd99524a54783b2023e29b1e5b610`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### What Changes

- **BREAKING**：维持四目标 Mihomo、sing-box、Surge、Quantumult X 及既定 legacy 清场；旧数据不自动迁移/修改。补足可认证的节点端点与凭据一致性：独立于逻辑 ID 的可信绑定；URL/YAML 敏感 transport 与版本不得因简单反算逻辑 ID 而误拒。
- 提供来自受保护源的真实节点非秘密元信息与有审计、鉴权、CSRF、并发版本保护的编辑/原地轮换接口；订阅来源在后续 Reconcile 的覆盖语义明确，拒绝在 UI 伪造保存或泄露私钥、PSK。
- 发布时原子持久化内容摘要、凭据版本/绑定与不可变加密工件或可验证重建依据；重启后严格验证，不从进程 map 或审计字符串恢复身份；历史缺失绑定记录 fail closed。
- Mihomo RULE-SET 仅接受可验证的真实资源 URL/已配置 provider，缺失资源即拒绝；风险读模型保证多 provider 下每节点一条最新观测、count 与分页一致且索引有界。
- 使用隔离本地数据库迁移、注入式确定性安全回归与真实前后端流程复验，再执行独立 reviewer 与具备隔离 URL 的 critic；不触生产、不提交。
```

### Constraint DOC-0118 <a id="hcr-doc-0118"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/proposal.md:15`
- **Source Digest**: `50d889afdac5ac7fb97d00fa34d09c7c045cd99524a54783b2023e29b1e5b610`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### New Capabilities

- `native-mihomo-publications`: 四目标真实导出、节点身份/编辑、持久发布、风险读模型与安全门禁。
```

### Constraint DOC-0119 <a id="hcr-doc-0119"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/proposal.md:19`
- **Source Digest**: `50d889afdac5ac7fb97d00fa34d09c7c045cd99524a54783b2023e29b1e5b610`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Modified Capabilities

无；更新该 Change 已存在的 delta spec。
```

### Constraint DOC-0120 <a id="hcr-doc-0120"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/proposal.md:23`
- **Source Digest**: `50d889afdac5ac7fb97d00fa34d09c7c045cd99524a54783b2023e29b1e5b610`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Impact

`internal/domain/`、`internal/parser/`、`internal/resolver/`、`internal/application/inventory/`、`internal/transport/http/nodes.go`、`web/src/features/nodes/`、`internal/application/publication/`、`internal/repository/sqlite/` 与本地 schema migration、`internal/compiler/mihomo.go` 及相应测试。其他已实现四目标/legacy 清场仅对回归负责。规划授权不等于施工、生产迁移或部署授权。
```

### Constraint DOC-0121 <a id="hcr-doc-0121"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/proposal.md:1`
- **Source Digest**: `b98ef7d7cf4c811262010d11fb2a7e2ef5db1f9490e468d8483e2932ee17d5d6`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### Why

当前 CSP 业务系统在数据流与数据库设计上存在核心架构痛点：
1. **上游原始事实丢失**：订阅抓取后未持久化原始未经篡改的 payload，解析失败或上游偶发异常（如 403 / 超时）难以回放对账，且容易误覆盖存量健康的节点资产；
2. **公告占位节点污染代理池与探针**：上游订阅面板（如 Dogegg）下发以 `127.0.0.1:1`、测试占位 UUID 和流量/到期提示为内容的通知条目（3 个同连接参数但不同内容的公告），系统将其误作为真实代理节点入库，导致探针周期性尝试拨测并持续报错，同时污染对外导出订阅；
3. **节点身份与连接版本纠缠**：节点逻辑身份与传输参数未解耦，多源共享时参数变动易引发覆盖冲突或旧历史断链，用户在 Web 端所做的字段级修改在下次抓取刷新时容易被静默覆盖；SQLite 不支持通过 `ALTER TABLE` 增加复合外键，若直接在 `nodes` 与 `versions` 间双向引用将引发循环依赖与全表重建风险；
4. **IP Risk 领域双轨分离与关联简化缺陷**：通用探测观测流（`kind = 'ip_risk'`）与详细服务商风险评估（7 张相关业务表）缺少统一测量标识关联，且上轮设计简单化为 1:1 强绑，忽略了 1:N 细粒度服务商采样及历史独立记录 Nullable FK 的客观事实；
5. **下游编译器硬编码封禁引发 422 熔断**：虽然依赖的 Mihomo 核心支持现代传输协议（如 VLESS `xhttp`），但 CSP 编译器白名单硬编码将其拦截报错，导致整个订阅无法导出持久化（`publications` 表为 0 行），前端 `web/src/ui/ErrorStateCard.vue` 还因文本匹配将协议不支持错误误报为“网络连接异常”。
用户明确要求重新设计 CSP 各个数据库与上下游契约，保持单 SQLite 主库与存量数据资产，厘清读写所有权，补齐事实层，实现确定性上下游一致流转。
```

### Constraint DOC-0122 <a id="hcr-doc-0122"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/proposal.md:12`
- **Source Digest**: `b98ef7d7cf4c811262010d11fb2a7e2ef5db1f9490e468d8483e2932ee17d5d6`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### What Changes

- **单 SQLite 主库、WAL 模式与领域所有权划分**：保持 `/data/csp-v1.db` 单主库架构，存量 27 张表（1 技术表 + 26 业务表，含 7 张 IP Risk 相关表）全量保全，涵盖 1014 个节点（31 活跃、983 历史失活禁用）、9 订阅与所有策略规则。所有 Schema 变更统一归口于 `MigrationRunner`，移除 Repository 构造函数中的运行时 DDL。停写维护窗口由隔离演练实测确定，提供资产对账与回滚补偿步骤。
- **构建最少必要上游事实层**：
- 引入 `subscription_payloads` 记录每次拉取的原始响应 BLOB（加密凭据内部重放，不向外网暴露），建立受控生命周期保留策略，抓取失败绝对不抹除既有健康库存；
- 引入 `subscription_entries` 解耦派生条目，每 Payload 独立 PK，同参数多公告全部独立记录不去重；实现前置来源门禁分类器（`verified_source_rule(subscription_id, source_evidence, rule_version) AND exact_rule_match(entry) => notice`），仅对已验证来源（如 Dogegg）启用规则，未验证来源即使满足回环+端口1+全零UUID+提示词四条件亦保留为 `unknown` 候选并支持纠偏；正常可解析代理始终为 `proxy`；支持条目级用户人工纠偏（`user_kind_override`、`override_anchor`、原因、时间戳、`actor_ref`），并通过 `source_key` 锚点跨刷新重放。
- **建立不可变节点连接版本、权威指针与用户覆盖机制**：
- 保持稳定 `logical_id` 资产标识；
- 引入 `node_connection_versions` 记录不可变有效参数（`effective_config_json` 与指纹），参数变更递增 `connection_revision`；
- 引入 `node_connection_heads` 作为权威当前版本指针，单向复合外键约束，避免与 `nodes` 双向循环外键，无需重建 `nodes` 表；
- 引入 `node_overrides` 持久化用户字段级人工修改，刷新时与源参数明确合并，解决多源冲突与被覆盖问题。
- **统一 IP Risk Nullable 1:N 测量关联与规则快照**：
- 建立 1:N 模型：通用测活记录（严格仅限 `kind = 'ip_risk'`）与多个细粒度服务商评分通过 `probe_observation_id` (Nullable) 与 `measurement_id` 关联，绝不与 Baseline 测活混淆；
- 跨表 kind 约束采用应用层事务校验结合标准外键，关联非 ip_risk 类型直接阻断回滚；
- 历史独立服务商记录保持 NULL 外键，不伪造探针运行；Baseline 与风险详情解耦；
- 利用现有 `risk_policy_revisions` 扩展 `rules_digest` 快照锁定策略版本；
- 规范 `safe_detail_json` Allowlist 白名单，严禁记录原生 `err.Error()` 或密码凭据。
- **打通下游统一连接消费模型、Mihomo xhttp 官方对齐与发布生命周期**：
- 探针出站与发布编译器共用同一份 `effective_config_json`；
- Mihomo 编译器仅将官方支持的 `path`, `host`, `mode`, `headers` 渲染为 `xhttp-opts`，未知 extra 保留于扩展且不盲传；对不支持 `xhttp` 的目标核心（如 sing-box）严格报错或兼容排除；
- 引入 `publication_payload_refs` 关联表，物理外键 `RESTRICT` 保护被引用的 Payload 防删，制定清晰的 Prune 清理事务；
- 实现 Preview（生成不可变 Draft 快照）与 Publish（直接发布指定 Snapshot ID，杜绝时间差库存漂移）两阶段生命周期；
- 支持严格模式与显式兼容模式导出，具备空组安全拦截机制（绝不自动回退至 DIRECT）；
- 修复前端 `web/src/ui/ErrorStateCard.vue`，禁止将 422 协议能力不支持误判为后端网络中断。
```

### Constraint DOC-0123 <a id="hcr-doc-0123"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/proposal.md:39`
- **Source Digest**: `b98ef7d7cf4c811262010d11fb2a7e2ef5db1f9490e468d8483e2932ee17d5d6`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### New Capabilities

- `dataflow/single-sqlite-domain-partition`: 保持单 SQLite 主库与 WAL 架构，按 Ingestion、Inventory、Policy Topology、Probes、Publications、Settings/Audit 明确表所有者划分，收敛 DDL 至 MigrationRunner，彻底杜绝运行时 Repo 构造函数中的重复 DDL。
- `dataflow/upstream-payload-entry-fact-layer`: 增加 `subscription_payloads` 与 `subscription_entries` 事实层，将权威原始未篡改响应（内部受控重放）与派生条目分层，实现公告伪节点与真实代理节点的确定性分离，支持 `source_key` 稳定重放与用户人工纠偏。
- `dataflow/node-connection-versioning-cow`: 引入 `node_connection_versions`、`node_connection_heads` 与 `node_overrides`，维持稳定 `logical_id` 与 1014 个存量资产，参数变更自动递增版本，用户人工修改持久化合并，多源更新写时复制 (CoW) 隔离，通过单向 Heads 外键消除循环依赖。
- `dataflow/downstream-matrix-and-publication`: 探针与编译器共用版本化连接模型，对齐 Mihomo xhttp 官方支持，统一 IP Risk Nullable 1:N 测量流与 safe_detail 白名单，严格/兼容双模式导出与清单校验，引入 publication_payload_refs 外键防删，修复前端 `web/src/ui/ErrorStateCard.vue` 422 误判为网络失联。
```

### Constraint DOC-0124 <a id="hcr-doc-0124"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/proposal.md:47`
- **Source Digest**: `b98ef7d7cf4c811262010d11fb2a7e2ef5db1f9490e468d8483e2932ee17d5d6`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### Impact

- **核心架构**：维持单 SQLite 物理文件架构不变，通过 Migration 000015 增量新增 6 张事实与关联表，零破坏性变更。
- **内部包演进**：
- `internal/application/inventory/`: 增加 Payload 持久化与 Entry 分类逻辑，适配 Versioning、Heads 指针与 Overrides；
- `internal/compiler/`: 增加 `xhttp` 传输支持及发布清单生成，未知 extra 安全隔离；
- `internal/probe/mihomo/`: 补齐 `xhttp` 出站拨号映射；
- `internal/application/probe/`: 统一 IP Risk Nullable 1:N 测量标识与 safe_detail 白名单；
- `web/src/ui/`: 修复错误分类逻辑。
- **数据迁移与回滚**：全量保留既有 1014 节点、29 组、163 规则、1 调度与历史探针数据，支持在线热备、实测维护窗口与原资产对账回滚。
```

### Constraint DOC-0125 <a id="hcr-doc-0125"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/proposal.md:58`
- **Source Digest**: `b98ef7d7cf4c811262010d11fb2a7e2ef5db1f9490e468d8483e2932ee17d5d6`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 增量演进：启用订阅库存范围收敛与端到端指标对齐 (Enabled Inventory Scoping)

- **业务背景与痛点**：当前控制台首页概览与节点列表请求 `/api/v1/nodes` 时未附带订阅范围过滤，直接返回全库 1014 个节点资产（包含 983 个历史失活节点）；而探测池与发布导出内部均已过滤活跃非公告节点（31 个），造成端到端数值不一致与“仍显示 1000 多个节点”的混淆。
- **核心对齐规则与冻结 API 契约**：
1. **全库资产 vs 可用库存分离**：底表保留全量 1014 个资产作为保护数据，全局主视图与首页概览默认应用 `scope=enabled_subscriptions`（范围 E），聚焦于“至少属于一个当前已启用订阅源且为其最后成功抓取条目且非确证公告”的可用节点（31 个）；`nodes.active` 保持独立，非 E 前置条件；显式 `scope=all_assets` 支持全库台账查询；未知 scope 返回 HTTP 400 `invalid_node_scope`；
2. **多订阅去重与各卡计数**：`GET /api/v1/subscriptions` 增加 `node_count`（启用有效贡献数）、`source_node_count`（源最后成功成员数）与 `counts_scope: 'enabled_subscriptions'`；主视图按 `logical_id` 全局去重；多源共享节点时呈现 `Sum(Per-Sub) >= Global Dedup` 拓扑事实；
3. **探测池与发布快照契约**：`GET /api/v1/probes/pool` 增加 `inventory_total`（范围 E 总数）、`candidate_total`（范围 E 活跃数）、`scope: 'enabled_subscriptions'`，保持 `total_count`（旧候选计数）含义不变；探测派发与发布解析基于范围 E 门禁；历史已发布快照字节绝对不可变；
4. **非破坏性与编辑接口保全**：用户手动无订阅节点保持安全持久化；`unknown` 节点不盲目封杀；节点详情与修改接口 (`GET/PATCH /api/v1/nodes/{logical_id}`) 保持全库资产访问权限，杜绝删历史。
```

### Constraint DOC-0126 <a id="hcr-doc-0126"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/proposal.md:1`
- **Source Digest**: `21a3be9b7b38b4e16f5747924fc2d04a420351f1c49a7af64a229af24d88123f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Why

当前 CSP 节点探针子系统存在严重的人为过度设计与臆造阻断（如无条件拦截自签名证书 `skip-cert-verify`、强行禁止 Hysteria2 端口跳跃与 TUIC `disable_sni`、将流媒体和 AI 探测硬编码为 `contract_drift` 假存活、将并发硬编码限制在 16~32）。用户明确要求彻底重做节点探针，全面对齐工业界成熟开源实践 `beck-8/subs-check`（主基准）与 `sinspired/subs-check-pro`（辅助调度与生命周期参考），移除主观安全门禁，恢复真实节点质量探测能力。
```

### Constraint DOC-0127 <a id="hcr-doc-0127"></a>

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

### Constraint DOC-0128 <a id="hcr-doc-0128"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/proposal.md:15`
- **Source Digest**: `21a3be9b7b38b4e16f5747924fc2d04a420351f1c49a7af64a229af24d88123f`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### New Capabilities

- `node-probes/mihomo-subcheck-runtime`: 采用进程内 Mihomo 适配器构建单节点独立 HTTP Client，完全遵循节点原生连接配置（支持 skip-cert-verify、hy2 ports、tuic disable_sni），远端域名解析，无环境变量代理穿透。
- `node-probes/pipeline-funnel`: 三阶段漏斗调度管道（Alive -> Media -> Speed），支持高并发轻量初筛与低并发深度检测，结合每节点 Client 复用、主动 Close 与挂起看门狗。
- `node-probes/platform-capability-matrix`: 工业级真实流媒体与 AI 能力检测矩阵（OpenAI, Netflix, YouTube, Disney, Claude, IPRisk, Geo），彻底消除伪造代码与硬编码拒绝。
- `node-probes/result-contract-and-ui`: 结构化多平台能力结果存储、统一 DTO 透传与 WebUI 流式工作台展示，区分封禁、不可达与真实解锁。
```

### Constraint DOC-0129 <a id="hcr-doc-0129"></a>

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

### Constraint DOC-0130 <a id="hcr-doc-0130"></a>

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

### Constraint DOC-0131 <a id="hcr-doc-0131"></a>

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

### Constraint DOC-0132 <a id="hcr-doc-0132"></a>

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

### Constraint DOC-0133 <a id="hcr-doc-0133"></a>

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

### Constraint DOC-0134 <a id="hcr-doc-0134"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/design.md:63`
- **Source Digest**: `b276d5b74dee0baeb48a7ebdb4d052aa39abe66859030f7f7dfe0edf2766a006`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 安全拨号器错误解耦与节点级隔离

- **决策**：
- 在 `internal/application/probe/safe_dialer.go` 中，定义明确的独立错误：`ErrTargetUnresolvable`（域名 DNS 解析失败/无公网 IP）与 `ErrPrivateTargetRejected`（目标被 SSRF 拦截），不再对这两种情况包裹 `ErrCredentialsUnavailable`；
- 仅在节点真正缺失认证凭据（如空 password/uuid）或 dialer 未配置时包裹 `ErrCredentialsUnavailable` / `ErrProbeDialingNotConfigured`；
- 在 `internal/application/probe/runner.go` 中，`isFatalDialError` 仅对真正的系统级凭据配置缺失生效；对于 DNS 与私网错误，单节点记录为不可用（`VerdictUnavailable`），不设置 `hasFatalDialErr`，允许批次其余健康节点正常完成 Phase 2 探测。
- **替代方案考虑**：
- *修改 runner 忽略所有 dial error*：会导致测试凭据配置完全失效时探测依然静默跑完，降低了系统配置错误的可发现性。
```

### Constraint DOC-0135 <a id="hcr-doc-0135"></a>

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

### Constraint DOC-0136 <a id="hcr-doc-0136"></a>

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

### Constraint DOC-0137 <a id="hcr-doc-0137"></a>

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

### Constraint DOC-0138 <a id="hcr-doc-0138"></a>

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

### Constraint DOC-0139 <a id="hcr-doc-0139"></a>

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

### Constraint DOC-0140 <a id="hcr-doc-0140"></a>

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

### Constraint DOC-0141 <a id="hcr-doc-0141"></a>

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

### Constraint DOC-0142 <a id="hcr-doc-0142"></a>

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

### Constraint DOC-0143 <a id="hcr-doc-0143"></a>

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

### Constraint DOC-0144 <a id="hcr-doc-0144"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/design.md:64`
- **Source Digest**: `663aa1398d387d660b3820bee8cbbed5093dabc0a97c3a45a844de805688f79f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
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
```

### Constraint DOC-0145 <a id="hcr-doc-0145"></a>

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

### Constraint DOC-0146 <a id="hcr-doc-0146"></a>

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

### Constraint DOC-0147 <a id="hcr-doc-0147"></a>

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

### Constraint DOC-0148 <a id="hcr-doc-0148"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:1`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Context

见 `proposal.md` 的动机。实机证据表明工作树 `dev` 已将旧 Python、旧 Go、旧前端和既有 OpenSpec 工件标记为删除；仍存在停止状态的 `clash-sub-parser` 容器，且保留卷中的 `/var/lib/docker/volumes/clash-sub-parser_backend-data/_data/clash_sub_parser.db` 为 17,121,280 bytes。`DESIGN_REQUIREMENTS.md` 已将该库明确定位为仅供后续独立离线迁移的数据源，并要求新模型不继承旧表。
新运行时是 Linux Docker 单容器，已有目标端口为 18080 与 17000。代码与历史运行卷不能共享数据权威。Go `embed` 可将构建后的 Web 静态资源嵌入单一二进制。[1] `modernc.org/sqlite` 提供纯 Go SQLite 驱动，适合不引入 CGO 的单二进制部署。[2]
```

### Constraint DOC-0149 <a id="hcr-doc-0149"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:7`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 用新 Go 模块、纯领域模型、新 schema 和新 REST 契约建立唯一 CSP 1.0 数据权威
- 以 sing-box 内存实例承载每个被测节点的实际流量路径；探测调度只管理有界任务，不持久化或复用代理运行时
- 让策略预览、导出和发布从同一不可变解析快照派生
- 建立不接触秘密、不原地改写历史库的离线、审查后激活导入流程
- 将前端限定为成熟 Vite + Vue 3 + Tailwind + shadcn-vue 组合；官方安装文档支持该 Vite 组合。[3]
**Non-Goals:**
- 不重新实现、调用或包装旧 Python/FastAPI、旧 Vue、旧 Go 包、旧 OpenSpec 或旧 API
- 不将旧 SQLite 原样挂载为新服务数据库，不做自动在线迁移、表适配、影子读写、双写或兼容 URL
- 不把订阅密钥、节点凭据、私钥、导出令牌、完整配置或探测原始响应放入前端、日志、审计摘要或 Git
- 不在本计划中实施、构建镜像、启动容器、写数据库、切换反代流量或删除历史卷
```

### Constraint DOC-0150 <a id="hcr-doc-0150"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:26`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D1 产品根、交付单元和模块边界

建立新根目录：`cmd/csp/` 是唯一进程入口；`internal/` 只能被本模块引用；`web/` 为独立 Vue 构建输入；`web/dist/` 仅为构建时中间物，不提交；`internal/webassets` 使用 `embed.FS` 提供 SPA。运行时健康端点为 `GET /healthz`，就绪端点为 `GET /readyz`，控制面为 `/api/v1/`，发布物为 `/publish/v1/{publication_id}`。
`internal/` 的所有权按以下方向单向依赖：
```text
transport/http ──> application ──> domain <── repository/sqlite
│             ↑
├─────────────┤
├── probe/singbox
├── compiler
└── import/legacy (CLI-only)
```
`domain` 不导入 HTTP、SQLite、sing-box 或 Vue 类型。`application` 持有命令用例、事务边界、幂等和授权策略。`repository/sqlite` 是新 schema 唯一持久化实现。`probe/singbox` 只接受已规范化节点配置并在内存中构建执行环境，不得到数据库连接。所有入站取消必须进入 `context.Context`；Go `database/sql` 在 BeginTx context 取消时会回滚事务。[6]
替代方案：继续拼接旧双后端或把 sing-box 子进程作为探测器。前者违反 clean-slate，后者增加进程、端口、配置文件和资源回收边界；均拒绝。
```

### Constraint DOC-0151 <a id="hcr-doc-0151"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:45`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D2 新 SQLite 数据权威与命名冻结

新运行时数据库文件固定为 `/data/csp-v1.db`，Compose 命名卷固定为 `csp-v1-data`；**不得**挂载 `clash-sub-parser_backend-data` 为此路径。由应用内嵌、顺序编号的 schema migration 创建；每次启动在 ready 前校验 schema version、`PRAGMA foreign_keys=ON` 和只读健康查询。
冻结领域 ID 规则：实体公开 ID 一律使用 UUIDv7 字符串；节点用稳定 `logical_id`（由规范化、非秘密的传输标识 hash 编码）用于跨刷新关联；数据库行号仅内部使用，不能出现在 API、URL、导出或关系字段中。公开 API 使用 `snake_case` JSON；时间为 RFC 3339 UTC；枚举为小写 ASCII；资源 revision 使用不可变 UUIDv7 与 content SHA-256。
冻结实体与数据 ownership：
| 实体 | 不可变或可变 | 关键字段与约束 | 唯一 owner |
| --- | --- | --- | --- |
| `subscriptions` | 可变配置 | `id`, `name`, `source_url_secret_ref`, `enabled`, `refresh_policy`, `revision` | Subscription application service |
| `subscription_fetches` | 不可变审计 | `id`, `subscription_id`, `started_at`, `outcome`, `content_digest`, `redacted_error` | Fetch command |
| `nodes` | 当前规范化状态 | `logical_id`, `protocol`, `display_name`, `normalized_config_secret_ref`, `active` | Inventory reconciler |
| `node_sources` | 可变关联 | `node_logical_id`, `subscription_id`, `last_seen_fetch_id`，复合唯一 | Inventory reconciler |
| `probe_runs` | 状态机 | `id`, `idempotency_key`, `config_revision`, `state`, `deadline_at` | Probe command service |
| `probe_observations` | 不可变证据 | `id`, `probe_run_id`, `node_logical_id`, `kind`, `verdict`, `evidence_digest` | Probe result recorder |
| `node_groups` / `group_edges` | 可变策略图 | UUIDv7 `id`，有序边，不能自环 | Policy service |
| `admission_rules` / `policy_rules` | 版本化配置 | `id`, `revision_id`, `expression`, `position` | Policy service |
| `configuration_revisions` | 不可变 | `id`, `parent_id`, `content_digest`, `state` | Revision service |
| `publications` | 不可变后可撤销 | `id`, `target`, `snapshot_digest`, `compiler_version`, `state` | Publication service |
| `settings` | 可变小集合 | 公开安全默认值与限制，不存秘密明文 | Settings service |
| `audit_events` | 只追加 | `id`, `actor_kind`, `request_id`, `action`, `result`, `redacted_summary` | Audit writer |
秘密须经部署机上的 secret provider 或加密 keystore 引用保存，API 只显示掩码或是否已配置。SQLite 写入使用显式短事务；大型读取有上限和索引；外键检查使用 SQLite 的 `foreign_key_check` 能力。[7]
替代方案：将整份节点配置、策略树和 probe 原文塞进 JSON blob 或用旧表适配。两者破坏约束、可查询性和单一数据权威；拒绝。
```

### Constraint DOC-0152 <a id="hcr-doc-0152"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:72`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D3 订阅到节点的事务边界

刷新命令的步骤是：取订阅快照与 revision → 受 SSRF policy 约束地获取 → 解析和规范化于内存 → 在单个新库事务内写入 fetch、upsert nodes、更新 `node_sources`、收敛最后来源已消失节点 → 提交。网络获取和解析绝不持有数据库写事务。失败仅追加失败 fetch，不改变最后有效台账。
SSRF policy：仅 `http/https`；解析后逐地址拒绝 loopback、private、link-local、multicast、unspecified、reserved，代理 URL 同样验证；每次重定向重新验证；禁止继承宿主环境代理；只允许显式 secret reference 的 `fetch_proxy`。设置 `timeout`、`max_redirects`、`max_response_bytes` 和解压缩后大小上限。
```

### Constraint DOC-0153 <a id="hcr-doc-0153"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:78`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D4 探测管道、资源模型和失败语义

`ProbeRun` 是持久命令记录；内存队列按 run 实例化。调度器为每类探测维护固定大小滑动窗口，默认全局 16，合法范围 10–20，硬上限 32；另有每订阅和每节点的 semaphore。run 的 idempotency 唯一键为 `(actor_scope, idempotency_key)`，TTL 至少 24 小时。所有工作 goroutine 都由 run context 监管，并在 run terminal state 时完成回收。
单个 node task 构建临时 sing-box 实例或等价官方 in-memory pipeline；所有 TCP、TLS、HTTP、IP 和平台请求必须使用该 task 的 outbound。探测结果不能复用系统 `HTTP_PROXY` 或直接 `net/http` transport。步骤顺序：config validation → handshake/connectivity → controlled HTTP latency → exit-IP/geo/ASN → provider-specific capability tests → optional bounded throughput。吞吐量必须为 opt-in、有字节预算、deadline 和 per-run 总预算。
`available` 是正向、版本化且满足语义断言的证据；`restricted` 是明确受限证据；超时、challenge、登录页、验证码、DNS/网络错误、协议错误和服务契约漂移分别是 `unknown` 或 `error`。绝不把“HTTP 200”单独当作可用。每条 observation 留存脱敏 normalized summary 和内容 digest，不保留 cookie、令牌、完整 HTML 或完整 IP（必要时记录受保护的原始证据引用）。资源上限同时解决 API 未受控资源消耗风险。[4]
替代方案：每节点启动子进程、全局节点共享 outbounds、仅测试 TCP 或普通网页。前两者使隔离及回收不可验证，后者会产生假阳性；拒绝。
```

### Constraint DOC-0154 <a id="hcr-doc-0154"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:88`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D5 策略图、解析快照和多目标编译

策略输入是已激活 `configuration_revision`、指定 inventory watermark、admission rule revision 与固定 compiler version。Resolver 以深度优先显式栈处理 `group_edges`，维护 visit colors 识别循环，应用 include/exclude 和 MATCH 置底，生成排序稳定的 `ResolvedPolicySnapshot`。它包含节点 logical IDs、解析过的 group tree、rule list、DNS 设置、input digest 和 resolution digest。
仅 Resolver 产生 snapshot；预览、下载前校验、五个 renderers 与 publication 服务只消费 snapshot。每 renderer 必须声明能力矩阵，遇到不支持特征报位置化 diagnostics。禁止“尽力丢字段”或以别的格式替代。发布前新建不可变 publication；令牌只能读取绑定 `publication_id`，撤销后永远不重定向到别的发布物。
替代方案：每个 renderer 自行查询并解析策略。会造成同输入多种语义与 preview/export 分叉；拒绝。
```

### Constraint DOC-0155 <a id="hcr-doc-0155"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:96`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D6 历史数据处理与恢复

保留卷的历史 SQLite 是 archive source，不是新系统依赖。`csp legacy-import --source <absolute-readonly-path> --report <path>` 是单独 CLI，不由 HTTP 服务调用、不在容器常规 entrypoint 执行。执行流程：
1. 以 read-only 模式打开源，生成 schema fingerprint、源文件 digest 和只读备份或快照引用
2. 使用明确列 allowlist 读取历史逻辑内容，逐记录 validate、normalize、去秘密化、转换为新 schema 的 `draft` configuration revision
3. 将未知字段、无法映射或无效记录写入只含类型、hash 和原因的 quarantine report
4. 在新库短事务中写入 drafts、import audit event 和总数；任何目标写入失败时整个本批次回滚
5. 输出报告并要求授权管理者 review/activate；未激活内容禁止刷新、探测、发布
导入允许逻辑名称、无凭据协议属性、分组、规则与 DNS 配置；明确排除 subscription URL 密码与 query token、代理 username/password/UUID/private key、auth token/cookie、完整 configs、原始 probe body/header 及未知 blob。该模型满足 clean-slate：内容可移入，但运行时没有旧 schema contract。SQLite Online Backup API 支持把源一致复制到新文件，可作为操作人员在源可能恢复运行时的快照方式。[8]
```

### Constraint DOC-0156 <a id="hcr-doc-0156"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:108`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D7 前端 UX 契约

采用 Vue 3 + Vite + TypeScript + Tailwind + shadcn-vue。组件只走生成的 shadcn primitives，页面不再维护私有“仿 shadcn”组件库。页面边界：Overview、Subscriptions、Nodes、Probe Runs、Policy、Revisions & Publications、Settings、Audit。路由状态管理 URL query；远端状态由 query client 统一 cache/invalidate；绝不复制成第二份全局节点账本。
主题是 `light | dark | system` 三态，服务端渲染不存在时仍用首屏前 `data-theme` bootstrap 避免 flash；偏好只存 UI 主题，不存认证 token。Node page 只请求服务端分页，具有 loading、empty、error 和 retry 状态；详情在 desktop 用 drawer，mobile 用 bottom sheet。断点验收为 375、768、1440 CSS px；主控件可触达靶心至少 44×44，禁止横向 overflow；dialog/drawer 有 `aria-*`、focus trap、Escape 和回焦。
```

### Constraint DOC-0157 <a id="hcr-doc-0157"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:114`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### D8 HTTP 命令/读取合同

所有管理列表返回：
```json
{"data":{"items":[],"page":1,"page_size":50,"total":0}}
```
`page >= 1`，默认 `page_size=50`，最大 `100`。状态变更走 POST/PATCH/DELETE，JSON 请求体以 schema 校验。创建 run、refresh、import 预检等可重试命令要求 `Idempotency-Key`；资源更新可选 `If-Match` revision，冲突为 409。HTTP 语义与条件请求遵循 RFC 9110 的 idempotence 与 opaque validators 框架。[5] 每个 response 带 request ID；日志和 audit event 共用它。
```

### Constraint DOC-0158 <a id="hcr-doc-0158"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:124`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### 3+1 异构方案池及仲裁

| 方案 | 取材 | 优点 | 关键缺陷 | 裁决 |
| --- | --- | --- | --- | --- |
| A 原生 sing-box 运行时嵌入 | sing-box 配置模型与 Go runtime | 流量真经被测节点，协议覆盖广 | 有生命周期和内存压力 | 采用为 probe adapter，受 run context 与 budget 约束 |
| B 纯 Go SQLite 单二进制 | Go `embed`、modernc SQLite | 容器轻、无 CGO、部署面小 | 需严格 migration 和 SQLite 写竞争纪律 | 采用，独立新卷与短事务 |
| C 成熟 Vue Admin 构件 | shadcn-vue Vite 官方配置 | 可访问 primitive、主题与响应式有成熟基座 | 需避免组件再次私有分叉 | 采用，禁止手搓基础 UI |
| D 本地融合：不可变 snapshot + CLI-only import | 当前清空代码和保留历史库事实 | 完全 clean-slate 又可保留经审查的逻辑内容 | 需要显式审核成本 | 采用，拒绝线上 adapter / dual-write |
```

### Constraint DOC-0159 <a id="hcr-doc-0159"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:133`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [sing-box API 或上游探测契约变动] → 在 `ProbeProfile` 加 version 和契约 fixture；漂移返回 `unknown/error`，不自动判可用
- [SQLite 单写者吞吐与长事务] → 网络/编译均在事务外；写侧短事务、busy timeout、WAL 评估和并发写测试
- [历史字段无法无损映射] → “无损”定义为 allowlisted 非秘密逻辑内容的记录级可追溯导入，而非复制旧 schema 或秘密；未知项进 quarantine，不伪造映射
- [公网管理/SSRF/凭据泄露] → 默认认证、CSRF、出口令牌分离、URL 逐跳验证、脱敏 telemetry 和最小暴露端口
- [五目标格式语义不对等] → renderer capability matrix 和发布前 hard-fail，不静默降级
- [前端大型表性能与移动可用性冲突] → 服务端分页为强制边界；详情按需加载；以浏览器自动化在三种视口验收
```

### Constraint DOC-0160 <a id="hcr-doc-0160"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:142`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Migration Plan

1. 构建与单测在全新数据库 fixture 上完成，且确认新 Compose 使用 `csp-v1-data`，没有旧 volume mount
2. 构建镜像但不启动旧服务替代物；以空卷启动并验证 `/healthz`、`/readyz`、schema version、认证拒绝、旧路由 410、五目标 contract fixtures
3. 由操作人使用只读历史副本运行 `csp legacy-import --dry-run`，人工审阅 counts、quarantine 和 secret-exclusion report
4. 经授权以新卷运行实际离线导入；审阅 drafts 并显式 activate；以脱敏集成数据验证探测与发布
5. 人工批准后才修改反向代理或端口流量；切换前保存并验证新库 backup，确认旧容器依旧停止、旧卷未写
6. 发布后观察 readiness、错误率、队列深度与审计；不达标时切回旧停止容器和原卷，或从已验证新 backup 恢复。禁止借助双写、旧表 adapter 或自动回灌回滚
```

### Constraint DOC-0161 <a id="hcr-doc-0161"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:151`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Open Questions

无会改变范围、公开契约、数据处理或任务拆分的未决问题。历史表的实际字段仅影响离线 importer 的 mapping matrix，该任务明确要求先做只读 schema inventory 与 fixture；若发现某类数据无法落入 allowlist，必须 quarantine 而不是改变本设计。
```

### Constraint DOC-0162 <a id="hcr-doc-0162"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/design.md:155`
- **Source Digest**: `716f45da8f62850cc3e7ecd3269a4110127b852074fb60eadcd8135d727d74e2`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Sources

[1] https://pkg.go.dev/embed
[2] https://pkg.go.dev/modernc.org/sqlite
[3] https://www.shadcn-vue.com/docs/installation/vite
[4] https://owasp.org/API-Security/editions/2023/en/0xa4-unrestricted-resource-consumption/
[5] https://datatracker.ietf.org/doc/html/rfc9110
[6] https://go.dev/doc/database/execute-transactions
[7] https://sqlite.org/pragma.html
[8] https://www.sqlite.org/backup.html
```

### Constraint DOC-0163 <a id="hcr-doc-0163"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/design.md:1`
- **Source Digest**: `0b0202be0d0766c63885038a3bf684b72fb338d908b85330be9ede091fd1a26a`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### Context

当前代码库围绕 `NodeCredentialVault`（AES-256-GCM）构建了繁重的加密与防篡改体系：`node_credentials` 独立密文表、`nodes.credential_version` CAS 版本号、`VerifiedNodeIdentity` 端点身份 HMAC/绑定、`SafeNodeConnection` Write-Only 脱敏投影、`publications` 表二次 AEAD 加密（`artifact_ciphertext` + `credential_bindings_json`）、`probe_observations.credential_version` 过滤门禁以及前端 `***` 掩码与 `redactPreviewSecrets`。线上因未配置 `CSP_NODE_CREDENTIAL_KEY` 导致 `vault=nil`，触发 `HTTP 422 node credentials unavailable: vault or credential repository not configured`。遵循用户“去掉过度工程化设计，给系统做一次减法”的明确指令（参见 `proposal.md`），本设计将节点配置与发布产物彻底回归 SQLite 明文直存，消除全部密钥、密文表与身份版本绑定开销。
```

### Constraint DOC-0164 <a id="hcr-doc-0164"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/design.md:5`
- **Source Digest**: `0b0202be0d0766c63885038a3bf684b72fb338d908b85330be9ede091fd1a26a`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 彻底移除 `NodeCredentialVault`、`node_credentials` 表、`VerifiedNodeIdentity`、`SafeNodeConnection`、`CredentialVersion` CAS 与 `CSP_NODE_CREDENTIAL_KEY`，服务零密钥启动即可完整工作。
- `nodes` 表直接持久化 `server`、`port`、`config_json`（序列化 `InboundProtocolCredential`）；`publications` 表直接持久化 `content`（编译后的明文配置字节）。
- 冻结精简后的 `domain` → `parser` → `resolver` → `compiler` 共享契约，使后续 Repository/Inventory、Publication、Probe、HTTP Transport、Web 前端与进程入口 6 个正交特性包可按不重叠写集合并发施工。
- 保留管理员鉴权（Admin Token / Session / CSRF）、导出 Token 鉴权与撤销、以及 `SafeNodeDialer` 的 SSRF 防护（公网 IP 校验、DNS 解析 IP 绑定、禁止重定向与严格 TLS 证书校验）。
**Non-Goals:**
- 不改变四目标编译器（Mihomo、sing-box、Surge、Quantumult X）的协议渲染格式、策略图解析拓扑或 IP 风险评估算法。
- 不在本 Change 执行期间触碰正在运行的生产容器或线上生产数据库（生产迁移留待交工后按备份与 `ReconcileSubscription` 流程执行）。
```

### Constraint DOC-0165 <a id="hcr-doc-0165"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/design.md:19`
- **Source Digest**: `0b0202be0d0766c63885038a3bf684b72fb338d908b85330be9ede091fd1a26a`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### 1. SQLite Schema `000011` 明文化减法迁移

- **决策**：新增 `migrations/000011_plaintext_nodes_and_publications.sql`（复用 `modernc.org/sqlite` 原生支持的 `ALTER TABLE ... ADD COLUMN` / `DROP COLUMN`）：
1. `nodes` 表：`ADD COLUMN server TEXT NOT NULL DEFAULT ''`、`ADD COLUMN port INTEGER NOT NULL DEFAULT 0`、`ADD COLUMN config_json TEXT NOT NULL DEFAULT '{}'`；`DROP COLUMN normalized_config_secret_ref`、`DROP COLUMN credential_version`。
2. `publications` 表：`ADD COLUMN content BLOB NOT NULL DEFAULT X''`；`DROP COLUMN artifact_key_id`、`DROP COLUMN artifact_nonce`、`DROP COLUMN artifact_ciphertext`、`DROP COLUMN credential_binding_digest`、`DROP COLUMN credential_bindings_json`。
3. 执行 `DROP TABLE IF EXISTS node_credentials;`。
- **备选方案**：新建 `nodes_v2` / `publications_v2` 重建表并拷贝数据。因现有索引均不引用被删列，直接使用 `ALTER TABLE` 原地增删列更简洁且不破坏外键级联。
```

### Constraint DOC-0166 <a id="hcr-doc-0166"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/design.md:26`
- **Source Digest**: `0b0202be0d0766c63885038a3bf684b72fb338d908b85330be9ede091fd1a26a`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### 2. Domain / Parser / Resolver / Compiler 共享契约重建（Phase 1 串行基座）

- **决策**：
- **Domain**：
- 删除 `internal/domain/node_identity.go`、`node_identity_test.go`、`node_connection.go`（含相关测试）。
- 精简 `internal/domain/node_credential.go`：删除 `DefaultKeyID`、`EnvNodeCredentialMasterKey`、`NodeCredentialVault`、`NodeCredentialRecord`、`NewNodeCredentialVault`、`NewNodeCredentialVaultFromEnv`、`Encrypt`、`Decrypt`；保留 `InboundProtocolCredential` 及其辅助函数（`EffectivePreSharedKey`、`IsTruthy`、`HasInsecureTransport`、`ExtractHy2Ports`、`HasTUICDisableSNI`）。
- 精简 `internal/domain/publication.go`：删除 `PublicationCredentialBinding`、`ComputeCredentialBindingDigest`、`ParseAndVerifyCredentialBindings`、`PublicationArtifactAAD`、`EncryptPublicationArtifact`、`DecryptPublicationArtifact`；`domain.Publication` 直接持有 `Content []byte`，删除加密与绑定相关字段。
- 精简 `domain.Node`：直接持有 `Server string`、`Port int`、`Credentials InboundProtocolCredential`；删除 `NormalizedConfigSecretRef`、`CredentialVersion`、`Identity`。
- 精简 `internal/domain/ports.go`：删除 `NodeCredentialRepository` 接口。
- 精简 `internal/domain/probe.go` 与 `node_filter.go`：删除 `HasValidCredentialVersion` 及 `MatchesCondition` 中基于 `CredentialVersion` 的过滤检查。
- **Parser**：删除 `parser.go` 中的 `opaqueSecretRef`、`yamlSecrets`、`urlSecrets`；`Parse` / `ExtractWithCredentials` 直接产出完整 `domain.Node`（填充 `Server`、`Port`、`Credentials`）。
- **Resolver**：`resolver.ResolvedNode` 直接包含 `Server string`、`Port int`、`Credentials domain.InboundProtocolCredential`，删除 `CredentialVersion` 与 `Identity`；`digest.go` 移除观测 `CredentialVersion` 摘要耦合。
- **Compiler**：删除 `compiler.WithCredentials`、`compiler.WithCredentialInputs`、`CompileOptions.Credentials`、`NodeCredentialInput`；`Compile(ctx, snapshot, target)` 直接从 `snapshot.Nodes[i]` 读取明文节点配置；`validateCredentialEnvelope` 仅校验 `Server`/`Port` 合法及各协议必填字段非空（如 SS 的 cipher+password、WG 的 private_key+public_key+local_address CIDR、TUIC 的 uuid+password），删除全部 `VerifiedNodeIdentity` 交叉核验；`CompileMihomo(ctx, snapshot)` 保留为无需 `credentials` 参数的快捷入口（同步简化 `internal/probe/singbox` 的导出/配置构建入参）。
```

### Constraint DOC-0167 <a id="hcr-doc-0167"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/design.md:39`
- **Source Digest**: `0b0202be0d0766c63885038a3bf684b72fb338d908b85330be9ede091fd1a26a`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### 3. Repository、Inventory、Publication、Probe 与 Transport/Web 并发减法（Phase 2 互斥写集）

- **决策**：在 Phase 1 共享契约编译通过后，按互不重叠的写集合切分为 6 个特性包并发推进：
1. **Repository & Inventory**（`internal/repository/sqlite/{nodes,node_credentials,revisions,node_risk,probes,*}`、`internal/application/inventory/**`）：删除 `sqlite/node_credentials.go` 及加密产物/版本专属测试；`sqlite/nodes.go`（及 `node_risk.go` 中的节点列扫描）直接读写 `server`、`port`、`config_json`；`sqlite/revisions.go` 直接读写 `publications.content`；`inventory/service.go` 的 `ReconcileSubscription` 直接将解析出的 `domain.Node` 写入 `nodes` 表（约 30 行替换原 ~250 行加解密事务），删除 `WithCredentialVault`、`credentialTransactionRepository`、`projectNodeConnection`、`verifyCredentialIdentity`，将节点更新简化为直接修改 `nodes` 表明文字段，`GetNodeDetailWithRisk` 直接返回自带完整配置的 `domain.Node`。
2. **Publication Service**（`internal/application/publication/**`）：删除 `vault`/`credRepo` 依赖注入、`resolveCredentials`（~100 行）与 `verifyCredentialBindings`（~95 行）；`Publish` 编译后直接将明文 `Content` 写入 `publications` 表；`ResolveAndServe` 校验 Token 与未撤销后直接返回 `pub.Content`（若历史记录 `Content` 为空则实时编译）。
3. **Probe Service**（`internal/application/probe/**`）：`NewSafeNodeDialer(opts ...SafeNodeDialerOptions)` 直接从 `domain.Node` 读取明文配置，删除查库与解密逻辑（~60 行）；`periodic.go` 删除 `CredentialVersion <= 0` 跳过逻辑；`runner.go` 无条件使用 `SafeNodeDialer`。
4. **HTTP Transport**（`internal/transport/http/{nodes,publications,subscriptions}*.go`）：`GET /api/v1/nodes/{id}` 直接返回完整节点信息；`PATCH /api/v1/nodes/{id}`（同时兼容 `/nodes/{id}/connection` 或统一至 `/nodes/{id}`）直接接收并更新明文字段；`publications.go` 删除 `ErrIntegrityCheckFailed` 映射；`subscriptions.go` 移除 `***` 脱敏保留判断，直接读写明文 URL。
5. **Web Frontend**（`web/src/**`）：节点详情删除 `***` 掩码、Write-Only 轮换、CAS 版本号、Vault AEAD 徽章与 Unavailable 横幅，直接展示并编辑明文字段；发布预览删除 `redactPreviewSecrets`，直接展示真实配置文本；订阅编辑直接回显真实 URL，去掉 `***` 占位。
6. **Entrypoint & Compose**（`cmd/csp/main*.go`、`docker-compose.yml`）：删除 `CSP_NODE_CREDENTIAL_KEY` 读取與告警、删除 `NodeCredentialRepository` 与 `WithCredentialVault` 初始化、无条件挂载 `SafeNodeDialer`。
```

### Constraint DOC-0168 <a id="hcr-doc-0168"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/design.md:48`
- **Source Digest**: `0b0202be0d0766c63885038a3bf684b72fb338d908b85330be9ede091fd1a26a`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk] 管理端接口直接返回节点密码/私钥与订阅 URL** → **Mitigation**：`/api/v1/*` 管理接口受 Admin Token / Session 鉴权与写操作 CSRF 保护，且个人自建控制面管理员本就是配置拥有者；客户端分发接口 `/publish/v1/{id}` 本就输出完整代理配置。
- **[Risk] 既有测试文件（如 `internal/integration/`、`sqlite/`、`http/`）大量引用 `NodeCredentialVault` 与 `CredentialVersion`** → **Mitigation**：在对应特性包与汇聚阶段同步清理测试中的 Vault/密文构造辅助函数，直接构造含 `Server`、`Port`、`Credentials` 的 `domain.Node`。
- **[Risk] 前端构建产物意外污染 `internal/webassets/dist`** → **Mitigation**：前端验证与隔离预览严格设置 `NO_COPY_WEBASSETS=1` 并使用临时 `--outDir`，校验 `internal/webassets/dist` 文件哈希前后完全一致。
```

### Constraint DOC-0169 <a id="hcr-doc-0169"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/design.md:54`
- **Source Digest**: `0b0202be0d0766c63885038a3bf684b72fb338d908b85330be9ede091fd1a26a`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### Migration Plan

1. **隔离开发与验证**：本 Change 全程在本地工作区与隔离临时 SQLite 数据库上验证 `000011` 迁移、单元测试、竞态测试与隔离预览实例，严禁操作线上容器或 `/data/csp-v1.db`。
2. **生产数据迁移说明（仅记录，不在本 Change 中执行）**：
- 生产库当前 `node_credentials` 仅 3 行，963 个节点绝大部分无加密凭据；升级包含 `000011` 迁移的新版本后，无需解密旧表，直接触发所有订阅重新拉取（`ReconcileSubscription`）即可自动重填 `nodes` 表的 `server`、`port`、`config_json` 明文字段。
- 生产库完整热备份已保存在 `/home/service/backups/csp-v1-backup-20260926_152446.db`。
- 含完整明文节点配置的冷归档备份位于 `/home/service/backups/csp-legacy-cold-archive-20260919.db`，可随时作为回退或离线核对基准。
```

### Constraint DOC-0170 <a id="hcr-doc-0170"></a>

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

### Constraint DOC-0171 <a id="hcr-doc-0171"></a>

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

### Constraint DOC-0172 <a id="hcr-doc-0172"></a>

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

### Constraint DOC-0173 <a id="hcr-doc-0173"></a>

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

### Constraint DOC-0174 <a id="hcr-doc-0174"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:41`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 订阅导出端点双模态验证与统一令牌复用

- **设计选择**：改造 `internal/transport/http/publications.go` 中的 `publicationClientHandler`：
- 服务层先查询发布实体并检查撤销状态，再通过鉴权回调决定是否交付相同的不可变编译产物；未知 ID 保持 404。
- 当 `!tokenHolder.IsExportAuthRequired()` 时：已激活的发布内容直接放行，无论请求没有令牌还是携带无效 query/Bearer 令牌，均不因鉴权返回 401。
- 当 `tokenHolder.IsExportAuthRequired()` 时：提取 query `token` 或 Bearer 令牌，校验 publication 专属 SHA-256 摘要或系统统一令牌 bcrypt 验证器；两者均未命中（含令牌缺失）则返回 401。`admin_auth_enabled` 不改变该结果。
```

### Constraint DOC-0175 <a id="hcr-doc-0175"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/design.md:47`
- **Source Digest**: `0456225a99b927d97e38ae84bbf7fb72f4bfdb35c4a55353c71305a5c3405b50`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 路由层兼容性修复与 `/p/...` 故障消除

- **设计选择**：在 `internal/transport/http/router.go` 中除保留 `/publish/v1/{publication_id}` 之外，同时挂载 `/p/{publication_id}` 路由，直接由相同处理函数接管或执行 307 Temporary Redirect（携带原始 QueryString），无缝化解历史客户端与外部订阅软件请求 `/p/...` 导致的 404 故障。
```

### Constraint DOC-0176 <a id="hcr-doc-0176"></a>

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

### Constraint DOC-0177 <a id="hcr-doc-0177"></a>

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

### Constraint DOC-0178 <a id="hcr-doc-0178"></a>

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

### Constraint DOC-0179 <a id="hcr-doc-0179"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:1`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

同一 repo-local Change `csp-fresh-inventory-full-validation`，schema `spec-driven`；CLI 返回 changeRoot `/home/service/clash-sub-parser/openspec/changes/csp-fresh-inventory-full-validation`、allowedEditRoots `/home/service/clash-sub-parser`。本轮只修订既有规划工件，不派工、不构建、不网络实验、不改业务/测试/生产配置、不提交、不 QQ（父编排负责里程碑）。用户明确批准本修订和后续既有发布授权，不新增一般审批门槛。
```

### Constraint DOC-0180 <a id="hcr-doc-0180"></a>

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

### Constraint DOC-0181 <a id="hcr-doc-0181"></a>

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

### Constraint DOC-0182 <a id="hcr-doc-0182"></a>

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

### Constraint DOC-0183 <a id="hcr-doc-0183"></a>

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

### Constraint DOC-0184 <a id="hcr-doc-0184"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/design.md:37`
- **Source Digest**: `0a1e3dad4ec43b55e1fdeec4b4d5a06a3f590cf646dcec7b0e228b9fec1822c6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 一个完整整改包：四侧 stage-first 与三种方法身份

入口 `tools/probe-comparison/run.py` 实现 plan/freeze/execute/report，复用 Python stdlib argparse/sqlite3、现有 Go testing/adapter/client，不新建调度服务或探针引擎。default network fail-closed。默认主比较四侧顺序 baseline A1(CSP)→B1(same-engine)→B2(same-engine)→A2(CSP)，每侧相同不可变 ALL90、90 唯一 LogicalID、connection_revision/full config fingerprint，不因前侧失败减少后侧 baseline。stage-first 保证全部四侧 baseline 终态后才计算 intersection `{node | 四侧 baseline 均成功且完整配置可比}`；空交集真实报告，不补入 discordant 节点。
- **CSP normalized stage control**：真实 runner/profile 必须使用相同任意2xx成功判定；非空 body 不自动失败，受硬预算；保存原204+empty策略版本、原始status/body和原因。
- **same-engine normalized comparator**：真实 upstream commit `3c320fd58aff5235e16218c050ec5b8ce587e233` 的独立干净私有 snapshot/薄 stage adapter，真实 CreateClient/platform；独立 modfile pin Mihomo v1.19.32，逐行保存相对 native 的 go.mod/go.sum、传递依赖 diff、go version -m/build flags/source SHA，修复硬编码 engine；不得宣称其为 unchanged native Check。
- **native Check control**：同一真实 upstream commit、原生 Mihomo v1.19.31 与 authentic native 配置，通过真实 `check.Check` 保留方法/默认行为。关闭 speed（空/禁用 SpeedTestUrl 并以 fixture 证明不发请求），冻结 GlobalProxies、空 SubUrls/SubUrlsRemote、KeepDays=0、SuccessLimit=0、空 Filter，关闭 Web/cron/save/upload/callback。baseline-only 原生配置允许，不能用 normalized stage control 冒充 native Check 或静默统一原生 defaults。
默认 manifest native_network_sides=[]，但 native build/Check 离线 fixtures 必验。可选 N1/N2 native baseline-only 两侧仅在首次外网请求前写入 manifest，验证时间/配额仍在同一总盘子且不损害主 baseline/platform 计划；否则不加，不能强制六侧。native eligibility 单列，绝不混入主四侧 intersection。native 全 media 实网不是新批必跑项，全部未尝试 native media 明确列出，不能宣称全原生 parity。
reference stage adapter 真正以有界 worker/semaphore 实现 alive concurrency8、media2，离线 blocking fixture 同时证明上限和确实可并行；声明配置但串行循环不通过。新批 speed concurrency0、stage清单无speed/risk中的speed请求，配置/日志/fixture均不得出现测速URL或请求。IPRisk 如被选为平台能力须完整计量并预先冻结，禁止借此附加新网络套件。
```

### Constraint DOC-0185 <a id="hcr-doc-0185"></a>

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

### Constraint DOC-0186 <a id="hcr-doc-0186"></a>

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

### Constraint DOC-0187 <a id="hcr-doc-0187"></a>

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

### Constraint DOC-0188 <a id="hcr-doc-0188"></a>

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

### Constraint DOC-0189 <a id="hcr-doc-0189"></a>

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

### Constraint DOC-0190 <a id="hcr-doc-0190"></a>

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

### Constraint DOC-0191 <a id="hcr-doc-0191"></a>

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

### Constraint DOC-0192 <a id="hcr-doc-0192"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/design.md:1`
- **Source Digest**: `aebac2fba78d6e0a4764f7794b3bedd31c7665bebee95876fcf3086af8a27ca4`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

当前 `GET/PUT /api/v1/policies/global-node-filter` 已保存 `{spec:{conditions:[...]}}`；`internal/domain/node_filter.go` 与 resolver 已有 AND 条件、probe_kind、判定、24h 默认/可选有效期及不匹配原因。设置页无入口，策略页已有表单但声称凭据版本匹配，实际 `MatchesCondition` 只检查时间和 verdict。`publication.Service.resolveSnapshot` 在过滤定义、来源、观测读取失败时容忍失败；无路由组时仅靠既有空路由组预检无法发现零节点。`Popover` 的 absolute 菜单位于 `SubscriptionsView` 的 overflow-hidden 卡片内，`ModalDialog` 使用 `showModal()` 而 `ConfirmModal` 是普通 z-index div，后者无法超过浏览器 top layer。`web/src/style.css` 已提供 `adaptive-surface-dialog/sheet` 等视口 token。
```

### Constraint DOC-0193 <a id="hcr-doc-0193"></a>

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

### Constraint DOC-0194 <a id="hcr-doc-0194"></a>

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

### Constraint DOC-0195 <a id="hcr-doc-0195"></a>

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

### Constraint DOC-0196 <a id="hcr-doc-0196"></a>

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

### Constraint DOC-0197 <a id="hcr-doc-0197"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:1`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### Context

当前系统的定时探测逻辑将用户配置的 `interval_seconds` 直接作为轮询触发定时器，并在启用或变更配置时将 `next_due_at` 设定为未来整段周期之后（如 2 小时后），导致管理员配置后迟迟看不到任何探测动作。并且当周期到达时，系统无差别将所有激活节点的全部类别（如 960 节点 * 5 kind）一次性全部入池，造成瞬时网络与并发拥塞。
现有系统已具备以下关键设施：
1. `probeObservationRepository.ListLatestByNodes(ctx, nodeIDs, kinds)`：高效分块查询节点在各类别下的最新观测；
2. `probe.queue.Scheduler` 的 `EnqueuePeriodicDedupe`：队列去重，如果已有相同 node 和 kind 在等待或执行中，则不重复提交；
3. `PeriodicCoordinator` 的 CAS 分布式租约与故障恢复机制；
4. `Runner` 与 `probePool`：支持节点池在池状态检测与安全出站探测。
```

### Constraint DOC-0198 <a id="hcr-doc-0198"></a>

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

### Constraint DOC-0199 <a id="hcr-doc-0199"></a>

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

### Constraint DOC-0200 <a id="hcr-doc-0200"></a>

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

### Constraint DOC-0201 <a id="hcr-doc-0201"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:44`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 单轮有界配额与公平覆盖策略

- **决策**：
- 单轮入池最大任务数为 `maxTasksPerRun`（默认 512）。
- 若到期任务数超过配额，对候选节点进行排序：
- 未被观测过的节点权重最高（观测时间视为 0）；
- 拥有最老观测时间的节点次之；
- 截取配额上限内的任务执行。未被选中的节点将在 10 分钟后的下一轮巡检中自然被选中，保证多轮之内全覆盖且无饥饿。
```

### Constraint DOC-0202 <a id="hcr-doc-0202"></a>

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

### Constraint DOC-0203 <a id="hcr-doc-0203"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/design.md:58`
- **Source Digest**: `d8464c0645eaf640d9ef70265ee226e7cf08b4a2cef6376c695446f984203570`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. 前端空闲保底刷新与触发后跟踪

- **决策**：在 `ProbesView.vue` 中，当页面处于空闲状态（无活跃 run 且队列为 0）时，维持每 15 秒一次的低频保底刷新，定时获取最新 `batches` 与 `runs`；点击“立即触发”时，设置活跃追踪状态并在触发后 10 秒内保持高频轮询（每 2 秒一次），确保即时捕获批次与运行状态迁移。
```

### Constraint DOC-0204 <a id="hcr-doc-0204"></a>

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

### Constraint DOC-0205 <a id="hcr-doc-0205"></a>

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

### Constraint DOC-0206 <a id="hcr-doc-0206"></a>

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

### Constraint DOC-0207 <a id="hcr-doc-0207"></a>

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

### Constraint DOC-0208 <a id="hcr-doc-0208"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:27`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### D1 三层数据边界与冻结数据合同

新增四种 domain 概念，并冻结它们的用途：
| 概念 | 责任 | 必须字段 | 禁止字段 |
| --- | --- | --- | --- |
| `ExitIdentity` | 一次 runtime 中被测节点的出口证明 | `node_logical_id`, `observed_at`, `identity_digest`, `country_code?`, `asn?` | 完整 IP 的持久字段 |
| `IPRiskObservation` | provider 返回的不可变、最小化事实 | `id`, `node_logical_id`, `exit_identity_digest`, `provider`, `provider_schema_version`, `observed_at`, `expires_at`, `status`, `score?`, `confidence?`, `network_class`, `anonymizer_traits`, `evidence_digest`, `redacted_summary` | 原始 JSON、完整 IP、API key、URL query、cookie |
| `RiskPolicy` | 本地业务决定 | `revision_id`, `provider_selection`, `max_observation_age`, `minimum_confidence`, `score_bands`, `trait_rules`, `unknown_action`, `conflict_action`, `review_action` | provider secret 或原始 payload |
| `RiskDecision` | 解析时派生 read model | `node_logical_id`, `policy_revision_id`, `decision`, `reason_code`, `observation_digest_set`, `evaluated_at` | 可逆 IP、密钥、原始响应 |
`score` 统一为可空 `0..100` 整数，含义由 `(provider, provider_schema_version)` 限定；没有有效 score 不得填 0。`confidence` 统一为可空 `0..100` 整数；provider 无该概念时为空而非伪造 100。`network_class` 为 `residential|datacenter|mobile|business|unknown`，`anonymizer_traits` 为受限的集合（如 `proxy|vpn|tor|residential_proxy|hosting|unknown`）。`status` 为 `available|unknown|error|stale`，刻意不同于 capability verdict 的语义域。
`EvidenceDigest = sha256(provider + schema_version + exit_identity_digest + canonical_normalized_response + observed_at)`。canonical response 只在内存中用于摘要计算后立即丢弃；持久化前应用同样的 `domain.RedactSensitiveInfo` 规则与长度上限。这继承现有 observation recorder 的安全边界，而不复用其类型字段。[inference] 当前 schema 尚无 IP risk 表，迁移名和 SQL 字段须由 executor 以现有 migration 编号实机核验后确定。
替代方案：把 risk 字段塞入 `domain.Node` 或 `ProbeObservation.RedactedSummary`。前者会把易过期的第三方判断误作节点本体状态；后者无法按 provider、TTL 与 policy 可查询地审计；均拒绝。
```

### Constraint DOC-0209 <a id="hcr-doc-0209"></a>

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

### Constraint DOC-0210 <a id="hcr-doc-0210"></a>

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

### Constraint DOC-0211 <a id="hcr-doc-0211"></a>

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

### Constraint DOC-0212 <a id="hcr-doc-0212"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:88`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### D5 管理读取与 HTTP 契约

管理 API 仅暴露 `IPRiskSummary`：`decision`、`risk_band`、`provider`、`provider_schema_version`、`observed_at`、`expires_at`、`status`、`reason_code`、`policy_revision_id?`。`risk_band` 为 `low|medium|high|critical|unknown`，由 active policy 映射，不是 provider 原始 label。
节点列表新增可重复 query：`risk_decision`、`risk_band`、`risk_provider`、`risk_status`、`risk_policy_revision`。未知或不支持的 filter 返回 validation error；所有 filter 先在 SQL/app query 层完成再排序分页，保持 `internal/transport/http/nodes.go:33-98` 当前 server-side pagination 边界。节点详情可以包含 `ip_risk_summary` 和最近脱敏 observations page；不能回传完整 IP。
风险命令沿用 `Idempotency-Key`，必须要求管理授权和 CSRF。创建 provider settings、policy revision、risk run 与 publication preflight 都写 audit，但 audit 仅有 digest、reason code、policy revision 和 request ID。前端只能消费上述 API-safe summary，路由 query 可恢复筛选，禁止 localStorage token 或 raw response cache。
```

### Constraint DOC-0213 <a id="hcr-doc-0213"></a>

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

### Constraint DOC-0214 <a id="hcr-doc-0214"></a>

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

### Constraint DOC-0215 <a id="hcr-doc-0215"></a>

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

### Constraint DOC-0216 <a id="hcr-doc-0216"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/design.md:124`
- **Source Digest**: `07d3e524395fe4ad26f18870ffcfc9e1610abf598194696f0494532e5708b810`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Open Questions

- [inference] 首个真实 provider 的账号、API edition、区域可用性与合同字段尚未取证；这些影响 adapter 的具体 credential header、endpoint 与 mapping fixture，但不改变本 change 的 provider port、脱敏、预算或 policy 结构。实施前必须由 executor 依据已授权 provider 的官方 API 文档补充 provider-specific contract，不能从网页页面猜测。
```

### Constraint DOC-0217 <a id="hcr-doc-0217"></a>

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

### Constraint DOC-0218 <a id="hcr-doc-0218"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/design.md:1`
- **Source Digest**: `e2e68390407d378eaf92613288be29bfed79db80bf40170e007ec4bd4609cfb9`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context & Architectural Intent

本项目为解决历史变更集众多但缺乏持续主干规范的问题，严格执行用户关于“重写SPC、审计符合度、汇总模糊项待我确认再落地”的最高指示。设计重点在于实现规范治理的双层解耦：将全局持久的 Living Specification（存放在 `openspec/specs/`）与局部的增量变更集（存放在 `openspec/changes/`）有效协同，同时融合业界 Spec-Kit 优先级场景范式与 DeepSeek 决策记录规范。
```

### Constraint DOC-0219 <a id="hcr-doc-0219"></a>

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

### Constraint DOC-0220 <a id="hcr-doc-0220"></a>

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

### Constraint DOC-0221 <a id="hcr-doc-0221"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/design.md:19`
- **Source Digest**: `e2e68390407d378eaf92613288be29bfed79db80bf40170e007ec4bd4609cfb9`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): policy group leaf node unpacking behavior pending user decision on Clarification Q1.
- **Original Source Text**:
```text
### 3. 六组核心待决业务问题解耦

针对审计暴露出的重大分歧，坚持“做减法、严禁自行做主”的原则，集中形成 6 组独立决策包：
1. 策略组解包叶节点的去重、递归深度、过滤传播与失效节点处理；
2. 成员来源节点与子组的混合交错顺序策略；
3. 历史 3 个轮询组遗留 `group_nodes` 的数据修复范围；
4. 非 Mihomo 纯节点目标遇到空策略组校验时的行为一致性；
5. 订阅刷新部分成功（Partial）时的来源修剪与隐藏作用域保护；
6. 前端节点健康状态筛选与服务端分页联动的交互模式。
```

### Constraint DOC-0222 <a id="hcr-doc-0222"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/design.md:28`
- **Source Digest**: `e2e68390407d378eaf92613288be29bfed79db80bf40170e007ec4bd4609cfb9`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### 4. 前两审 REJECTED 根因定位与 Epoch 3 整改架构 (Remediation Root Cause & Architecture)

经过两轮独立审查机驳回（Reviewer REJECTED，审计收据 `3cb8a9188068e45477b661fde2f31c2f0d311efc763a94f48e2175b00be0a6ab` 与 `391d6ff8146e6a76ca92f6bb118183525d4d5c5aa9cfe8b0183c288d7cd0892e`），本阶段深度定位并闭环解决以下四大根因：
1. **根因一：命令散落 Markdown 表格导致的正则转义失效与 0-case 伪证 (P0-01)**：
- *问题表现*：表格中直接书写正则管道符导致必须转义为 `\|`，被 Go 正则引擎识别为字面量竖线而匹配 0 个用例；同时 `FR-PROB-07` 的测试命令 `^TestBuildOptions$` 因尾锚点 `$` 导致实际匹配 0 个用例（receipt 中 `cases_run: 0`），而矩阵表格虚标 `16 passed`。
- *整改方案*：
- 彻底解耦验证命令与矩阵表格，建立专门的命令目录工件 [`docs/specification/verification-commands.md`](../../../docs/specification/verification-commands.md)，使用独立 Markdown fenced code blocks 编号 `CV-001` ~ `CV-041` 逐条存储与执行，严禁表格内包含管道正则与 HTML 实体；
- 将 `FR-PROB-07` 替换为真实测试 `TestBuildOptionsRejectsUnsafeProtocolOptions`（实际执行 1 top-level + 6 subtests，全绿通过）；
- 重新全量实机执行 41 条命令，收集每条命令的标准 argv JSON、退出码（全为 0）、用例名、测试包、子测试、源码 SHA-256 与日志存根；
- 重新精确统计执行次数（178 次）与独立唯一测试数（137 个，含 91 个 top-level + 46 个 subtests），明确披露跨 FR 复用事实，杜绝人工虚填。
2. **根因二：问题清单 Q1.4 选项越过既有激活与风险准入底线 (P1-01)**：
- *问题表现*：原 Q1.4 备选项提出“不论健康与激活状态完整展开”，违反了系统既有的 `active=false` 与 `NodeScopeEnabledSubscriptions` 准入底线。
- *整改方案*：彻底删除该越界选项。明确未激活与风险拦截节点在准入阶段已被坚决剔除，Q1.4 严格聚焦于已准入节点中探针健康状态（`healthy` / `degraded` / `undetermined`）如何受父策略组显式筛选约束，且不默认剔除 `undetermined`。同时确立图环路拒绝为既有架构强约束。
3. **根因三：问题清单 Q6 预设跨页全选为强需求 (P1-02)**：
- *问题表现*：原 Q6 将跨页全选预设为既有需求，迫使用户在本地分页与全局全选间抉择。
- *整改方案*：确立服务端全量分页与筛选为既有架构基线（不向用户提问是否保留本地 3 步翻页）；将跨页批量全选明确标记为“如果用户需要才进入落地（待确定），默认单页选择”，未获确认不新增范围。
4. **根因四：粗粒度 63 份规范映射掩盖历史需求细节与冲突**：
- *问题表现*：先前仅以 26 个 Change 的领域大类宣称覆盖，未逐条核对 63 份规范中具体的 Requirements 与 Scenarios，且掩盖了历史加密/Vault 与主干明文直存等显式冲突。
- *整改方案*：
- 建立逐条登记工件 [`docs/specification/historical-requirement-register.md`](../../../docs/specification/historical-requirement-register.md)，从全部 63 份规范中 100% 提取 259 项 Requirement 与 463 个 Scenario，分配稳定 `HR-001` ~ `HR-259` 编号并记录精确文件行号锚点；
- 严格按真实可证状态标注（按注册表明细表条目统计：232 项已映射待复验、3 项历史冲突待裁定、24 项未验证存根 UNRESOLVED；深入剖析 6 组架构演进冲突主题，包括凭据明文 vs Vault、探针周期 1m vs 10m、解包能力缺失、混合顺序缺陷等）；
- 确立主干 37 项 FR 矩阵与 259 项 HR 库存的双层治理体系，后续变更持续更新状态，严禁删除历史存根或宣称全部已验。
```

### Constraint DOC-0223 <a id="hcr-doc-0223"></a>

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

### Constraint DOC-0224 <a id="hcr-doc-0224"></a>

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

### Constraint DOC-0225 <a id="hcr-doc-0225"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/design.md:16`
- **Source Digest**: `22ccc93b242323f291aff2784eaf502ed8b3e2d35610b02bb69661cff9853865`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:**
- 在 `internal/probe/queue` 与 `internal/application/probe` 实现统一的**节点池双端优先级队列（Node Probe Pool / Priority Deque）**：
1. **定时测速去重入池（Tail Enqueue with Deduplication）**：定时测速触发时将活跃节点加入节点池队尾；若节点已在节点池中（处于 `queued` 等待或 `probing` 检测中），**静默跳过、不重复添加**，彻底消除 `ErrNodeConflict` 冲突报错；
2. **手动检测插在队列最前面（Front-of-Queue Priority Preemption & Promotion）**：手动发起检测时，目标节点直接插在节点池队列最前面优先调度；若目标节点已在节点池队列后方等待，将其**提升（Promote）至队列最前面**并合并探测维度；若目标节点正在检测中（`probing`），复用/合并待测维度且不报冲突错误；
3. 保持底层 `Scheduler.Submit` 默认模式（`EnqueueStrictConflict = 0`）对既有单元测试（如 `TestSchedulerBoundsConcurrencyAndRejectsNodeConflict`）的完全向后兼容。
- 在 `NodeView` 与前端节点状态体系中新增 **「检测中」（`probing`）** 状态（及 `probe_state: "probing" | "queued" | "idle"`），在 `GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}` 与 `GET /api/v1/probes/pool` 实时透传。
- 新增 `GET /api/v1/probes/pool`（以及手动触发定时入池 `POST /api/v1/probes/schedule/trigger`），精准返回用户要求的 5 大统计指标：**当前队列中的节点数**、**未测数**、**总数**、**不可用数**、**可用数**。
- 在 `ProbesView.vue` 与 `NodesView.vue` 完成美观恰当的节点池状态看板、“检测中”动态徽章、点击指标卡联动筛选及实时池状态轮询。
**Non-Goals:**
- 不引入 Redis 或外部消息队列中间件，继续使用单进程内存双端优先级队列 + SQLite 持久化观测记录；
- 不改变 sing-box 代理拨号安全边界与 SSRF 防护逻辑；
- 不触碰生产容器或生产数据库。
```

### Constraint DOC-0226 <a id="hcr-doc-0226"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/design.md:34`
- **Source Digest**: `22ccc93b242323f291aff2784eaf502ed8b3e2d35610b02bb69661cff9853865`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### 1. 统一节点池双端优先级队列（`internal/probe/queue/scheduler.go` & `internal/application/probe`）

1. **任务入池模式（`EnqueueMode`）与双端队列调度**：
- 在 `queue.Task` 新增字段 `Mode EnqueueMode`：
- `EnqueueStrictConflict EnqueueMode = 0`（默认零值）：保持原有严格冲突检查与 `NodeTTL` 行为，确保既有底层单元测试 100% 兼容；
- `EnqueuePeriodicDedupe EnqueueMode = 1`（定时测速去重入池）：
- 检查该节点 `LogicalID`（或 `(LogicalID, Kind)`）是否已处于节点池中（`probing` 正在执行或 `queued` 正在排队）；
- **若已在节点池中**：不报错、不重复入队，立即触发 `task.OnComplete(nil)` 并返回 `nil`（同时在 `Scheduler` 记录去重跳过计数）；
- **若不在节点池中**：将任务追加至普通/定时队列尾部（Tail），等待工作协程按序消化。
- `EnqueueManualPreemptFront EnqueueMode = 2`（手动检测插在队列最前面）：
- 忽略 `NodeTTL` 冷却期限制，永不返回 `ErrNodeConflict`；
- **若该节点已有定时/低优排队任务在节点池中等待**：将原排队任务出队并完成其回调（或合并关联回调），将本次手动检测任务**插在节点池队列最前面（Head / `runOrder[0]` 且 `queues[runID]` 队首）**；
- **若该节点此刻正处于 `probing`（检测中）**：允许手动任务排在队首紧随当前正在执行的动作（或复用结果），绝不因 `ErrNodeConflict` 失败；
- 在 `nextEligibleLocked()` 调度时，拥有 `EnqueueManualPreemptFront` 任务的手动 Run 始终排在 `runOrder` 最前面，并优先获得空闲 Worker 槽位。
2. **节点池实时状态追踪（Node Pool State Introspection）**：
- `queue.Scheduler` 维护节点级池状态快照方法 `PoolQueueSnapshot()` 与 `GetNodePoolState(logicalID string) string`：
- 当某个 `LogicalID` 有至少一个任务正在 `processTask` 执行中时，其池状态为 `"probing"`（检测中）；
- 当某个 `LogicalID` 没有正在执行的任务、但在 `queues` 中有等待调度的任务时，其池状态为 `"queued"`（队列等待中），且按队首到队尾顺序记录在 `QueuedNodeIDs` 中；
- 其他情况为 `"idle"`；
- **当前队列中的节点数（`queue_nodes_count`）**：定义为当前节点池内的去重节点总数（`len(ProbingNodeIDs) + len(QueuedNodeIDs)`，同时在响应中提供 `probing_count` 正在检测节点数与 `queued_waiting_count` 排队等待节点数，满足对“队列中/池中节点数”的直观理解与细分查看）。
- 在 `PeriodicCoordinator.executeBatch` 中，定时批次拉取活跃节点后，亦可先通过 `scheduler.FilterNotInPool(nodes)` 或通过 `DefaultRunner` 的 `EnqueuePeriodicDedupe` 自动跳过已在池中的节点，并将跳过数量计入 `batch.Counts.SkippedNodes`。
```

### Constraint DOC-0227 <a id="hcr-doc-0227"></a>

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

### Constraint DOC-0228 <a id="hcr-doc-0228"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/design.md:102`
- **Source Digest**: `22ccc93b242323f291aff2784eaf502ed8b3e2d35610b02bb69661cff9853865`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 前端节点池状态看板与“检测中”全链路设计（`web/src/features/probes/**` & `web/src/features/nodes/**`）

1. **第一视觉层级：节点池与全网检测状态看板（`ProbesView.vue`）**：
- 设计高辨识度的 **5+1 节点池全景状态看板**（支持桌面端网格与移动端自适应流式布局，点击前 5 张指标卡可直接切换工作台状态筛选）：
1. **当前队列中的节点数**（`data-testid="pool-metric-queue"`）：醒目展示 `queue_nodes_count`，副标题拆分展示「⚡ 检测中 `probing_count` · ⏳ 排队等待 `queued_waiting_count`」，当队列非空时展示动态脉冲指示器与实时消化进度条；
2. **总数**（`data-testid="pool-metric-total"`）：展示全网活跃节点总数 `total_count` 与已覆盖订阅源数；
3. **可用数**（`data-testid="pool-metric-available"`）：绿色语义展示 `available_count`、在线可用率百分比进度条，以及「正常 `healthy_count` / 降级 `degraded_count`」细分徽章；
4. **不可用数**（`data-testid="pool-metric-unavailable"`）：红色语义展示 `unavailable_count`（连接超时/握手失败）与占比，提供一键「重测不可用节点（插队最前）」快捷入口；
5. **未测数**（`data-testid="pool-metric-untested"`）：中性/琥珀色语义展示 `untested_count`，提供一键「检测未测节点（插队最前）」快捷入口；
6. **节点池定时巡检与去重策略卡**（`data-testid="pool-schedule-card"`）：展示定时自动入池开关、周期、去重规则说明（「已在池中节点不重复添加 · 手动检测自动插队最前」）、一键「立即入池巡检」与「策略设置」按钮。
2. **节点状态新增「检测中」（`probing`）与「队列中」（`queued`）**：
- 在 `web/src/features/nodes/nodeView.ts` 中：
- `NodeHealthStatus` 扩展支持 `'probing' | 'queued' | 'healthy' | 'degraded' | 'unhealthy' | 'unknown' | 'missing'`；
- `NodeRecord` 与 `NormalizedNode` 新增 `probe_state?: 'probing' | 'queued' | 'idle'` / `probeState: 'probing' | 'queued' | 'idle'`；
- `nodeHealthBadge(node, probingIds?, queuedIds?)`：若 `node.probeState === 'probing'` 或 `node.healthStatus === 'probing'` 或在实时 `probingNodeIds` 集合中，返回 `{ label: '检测中', tone: 'info', pulsing: true }`；若 `node.probeState === 'queued'` 或在实时 `queuedNodeIds` 集合中，返回 `{ label: '队列中', tone: 'warning', pulsing: true }`；否则返回原有的 `正常` / `降级` / `异常` / `未探测`；
- 在 `ProbesView.vue` 状态筛选器 `healthFilter` 中新增选项：
- `all`（全部状态）
- `probing`（检测中 / 队列中）
- `healthy`（可用 · 正常）
- `degraded`（可用 · 降级）
- `unhealthy`（不可用 · 异常）
- `unprobed`（未测）
- 在 `ProbesView.vue` 节点表格行与 `NodesView.vue` 节点卡片/详情抽屉中：
- 当节点处于 `检测中` 时，渲染带旋转/呼吸动效的蓝色/主色「⚡ 检测中」徽章；处于 `队列中` 时渲染「⏳ 队列中」徽章；
- 手动点击单节点「立即重测」或顶部「一键全量测速 / 测速已选节点」时，立即乐观标记目标节点为 `probing` / 队首并展示清晰提示：「已插入节点池队列最前面优先检测」，同时启动高频轮询（1.5s~2s）拉取 `/api/v1/probes/pool` 与 `/api/v1/nodes` 直至节点池队列消化完毕。
```

### Constraint DOC-0229 <a id="hcr-doc-0229"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/design.md:129`
- **Source Digest**: `22ccc93b242323f291aff2784eaf502ed8b3e2d35610b02bb69661cff9853865`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk] 快速探测完成导致前端轮询错过瞬时 `probing` 状态** → **Mitigation**：
1. 在手动发起测速（`createRun` / `triggerQuickProbe`）时，后端在 `POST /api/v1/probes/runs` 同步阶段即将目标节点登记入池（置为 `probing`/`queued`），前端也在请求发起瞬间将目标节点加入本地 `probingNodeIds` 集合并在 `GET /api/v1/probes/pool` 返回后同步服务端 `probing_node_ids` / `queued_node_ids`；
2. 单元测试与隔离预览测试可同时覆盖“探测进行中（`probing` / `queued`）”与“探测完成后转为 `可用 / 不可用`”完整状态流转。
- **[Risk] 既有调度器单元测试对 `Submit` 返回 `ErrNodeConflict` 有断言** → **Mitigation**：
1. `Task.Mode` 默认零值为 `EnqueueStrictConflict`，保留 `Scheduler.Submit` 原有冲突校验语义，确保 `internal/probe/queue/*_test.go` 既有测试零破坏；
2. 新增 `EnqueuePeriodicDedupe` 与 `EnqueueManualPreemptFront` 模式以及 `Scheduler.EnqueueNodePool` / `PoolQueueSnapshot` 专用于节点池定时去重与手动插队，并编写专门的节点池并发单测。
```

### Constraint DOC-0230 <a id="hcr-doc-0230"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:1`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 proposal.md。只读取证：`domain.Node.Active` 是激活定义；`NodeFilter.ActiveOnly` 可查激活节点，`NodeSourceRepository.ListByNode` 存真实来源；`ProbeRun`/`ProbeObservation` 和 `/probes/runs` 已持久化，runner 支持空 nodeIDs 取激活节点、基线默认种类、`DefaultRunBudget.MaxTasks=512`、现有安全出站与队列；`cmd/csp/main.go` 负责探测队列 Drain/DB Close；`resolver.Resolve` 先做激活/准入/风险，再按显式边生成组；`publication.resolveSnapshot` 与 policy 预览调用关系需施工核对并统一。`matchesAdmissionExpression` 使用宽松字符串/正则回退及无效字段按名称匹配，不适合未经严格校验直接复用于新配置。历史 `PROCESS-NAME` 修复在提交 `7289b4c`；本次错误无请求日志，无法断言其根因。项目主规格目录当前为空，故本 Change 建立两个新能力规格。
```

### Constraint DOC-0231 <a id="hcr-doc-0231"></a>

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

### Constraint DOC-0232 <a id="hcr-doc-0232"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:13`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 1. 冻结公共契约和计划归属

在 `internal/domain` 新增 `ProbeSchedule{Enabled,IntervalSeconds,Kinds,NextDueAt,Generation,UpdatedAt}`、`ProbeBatch{ID,WindowAt,Generation,Owner,LeaseUntil,State,RunIDs,Counts,RedactedError,CreatedAt,UpdatedAt}` 与专用 `ProbeScheduleRepository`（原子 Compare-and-Swap 领取、心跳续约、完成、列表）；将全局 `NodeFilterSpec` 作为独立单例设置，将组 `NodeFilterSpec` 作为按 group_id 绑定持久化；其结构为 `Conditions []FilterCondition{Field,Op,Value,ProbeKind,FreshnessSeconds}`，仅 AND，同一字段多值可重复 OR 不提供，NOT 用专有受限操作。`ProbeObservation` 添加产生时节点 credential_version 元数据，旧行标识版本未知；判断版本匹配不通过未知行。选 SQLite 增量表而非新的配置中心/另一套调度依赖；仓库已有 `modernc.org/sqlite` 与显式事务，单一 DB 可用受条件 UPDATE + 唯一 `(generation,window_at)` 协调进程。多独立 DB 的跨主机 exactly-once 不保证，部署边界为共享同一受控 SQLite 库。
```

### Constraint DOC-0233 <a id="hcr-doc-0233"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:17`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 2. 周期执行算法与安全

默认关闭；管理员配置的周期和种类经上/下界和 runner 预算验证（产品初始建议 15 分钟至 24 小时；如用户明确不同周期另调整）。`cmd/csp/main.go` 启动从 DB 加载配置，用 `time.Timer` 与 `context` 等待最近到期；不引入私有 cron 或第二队列。原子占位先推进 `next_due_at`（停机后采用 now+interval 跳过过去窗口，防止补发风暴），批次唯一；租约有过期恢复，续约与领用必须在同事务按 generation/owner fencing，旧 owner 丢租约后不得再写状态或创建运行。运行 actor_scope 固定 `system:periodic-probe`，幂等键源于 generation/window/chunk，使用现有 `ProbeRun`/`DefaultRunner`/`queue.Scheduler`，不经 HTTP 模拟手动调用；批次与运行关联在派发前持久化，每次从当前 `ActiveOnly` 完整分页或显式无分页查询取快照，逻辑 ID 去重排序，执行前再校验 active/credential_version 与解密是否可用，绝不在日志或 API 中打印密钥；设置运行期限与节点×种类任务预算，分片均衡以覆盖大库存，不允许静默截断。手动任务和周期任务共享现有有界 queue 公平机制；周期不得占满全部待执行容量，队列压力时让位/延后并记载未覆盖数。检测到无有效凭据时每节点记录脱敏跳过，禁止安全出站退化；计划关闭取消后续批次并根据 UI 的“取消本批次”显式取消运行。启动先恢复遗留 queued/running 运行与批次：截至 deadline/失租约终结为 expired/cancelled，禁止不完整运行数据无条件重放；尚未到期且可安全重放的片段只凭关联运行 ID 幂等恢复。停服顺序：停止 timer/续约与新派发 → 取消/等待周期运行 → 共用 Scheduler.Drain → DB Close；Drain 失败保持现有不关闭 DB 安全门禁。比较方案：单进程锁不足多实例，cron 调度器仍需分布式锁与恢复，故选择现有 timer+SQLite 原子条件抢占。
```

### Constraint DOC-0234 <a id="hcr-doc-0234"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:21`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 新筛选结构、语义与安全

沿用 `resolver` 的排序/图展开和诊断设施，但**不复用**宽松 `matchesAdmissionExpression` 处理新字段；使用标准库 `strings`、`time` 和枚举类型作严格有界校验；不用 CEL/自定义脚本/正则解析器。`display_name` 包含/不包含大小写折叠，`protocol`、`source_subscription_ids` 使用枚举/精确成员关系；probe kind/verdict 取单条“最新有效”观测（`observed_at DESC,id DESC`），需对应当前节点 credential_version、且新鲜度有界，以接收本次解析的固定 `AsOf` 时间判断；质量 `latency_ms <= value` 仅对 `available` 且非负结果有效。缺失、失败、未知、过期或版本不符时，所有 probe 条件（含否定条件）为 false，拒绝未知绕过。显式来源仓库 `NodeSourceRepository` 必须按节点批量查询新增有界方法，禁止每节点 N+1；观测仓库追加批量 latest 方法和索引 `(node_logical_id,kind,observed_at DESC,id DESC)`；读取上限受输入节点/条件限制。全局配置由 admin GET/PUT 管理并持久化版本、审计；组级筛选随 `POST/PATCH/GET /policies/groups` 的可选 `node_filter` 读写，省略 PATCH 表示不修改，显式空对象清除。`NodeFilterSpec` 用 `null/[]` 表示恒真；不更改老 `AdmissionRule` 的优先级与行为。所有保存路径对字段、操作、值和总条件数校验并返回字段级错误。
```

### Constraint DOC-0235 <a id="hcr-doc-0235"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:25`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. 解析边、快照与发布

读取同一次解析所需节点、来源、观测、全局/组级筛选及 group/rules：事务一致读或稳定快照并证明无竞态，注入 `ResolveInput{AsOf,GlobalFilter,GroupFilters,NodeSources,LatestObservations}`；已有风险拒绝和准入先执行，全局过滤建立唯一 admitted 集，组在此基础上按自身条件筛选显式成员；只有设置非空组条件且没有节点边的组会从全局集合自动匹配，按 `DisplayName ASC,LogicalID ASC` 附加，组边排序保持原序；子组边保持拓扑，在父组展开时再应用父组条件，防止子组绕过。`AllNodeLogicalIDs` 与编译所用 `Members` 必须对应：若父组过滤会使直接 child-group 边暗含被拒绝节点，采用在解析快照中为该父组展开的受限子组成员/可编译投影，或拒绝该种无法无损表达的嵌套配置；不得只改变统计 ID 而保留可绕过的子组边。推荐对父/子存在不同筛选的场景直接拒绝并给出 `unrepresentable_nested_filter` 字段级诊断，直到真正实现可编译投影；规格要求有效嵌套筛选必须可以表达，故交付前必须完成投影方案，不得长期以拒绝替代。决策：解析器形成不可变的 per-group 展开投影，由 compiler 只引用投影中的节点；子组如果被多个父组引用且父过滤不同，生成稳定的父上下文派生子组名称/ID（摘要来源于父组 ID、子组 ID、过滤条件）并保证各目标渲染均支持，UI 同时保留原策略图引用。预览、预检、发布共用单一 `resolveSnapshot` 装载函数/输入摘要，涵盖筛选定义版本、来源集合、观测 ID/版本/时间及 AsOf 新鲜度区间；快照摘要不纳入无关的当前墙钟时间，但跨越 freshness 边界必须变化。过滤为空的老组仍保持已有编译行为；新筛选导致被路由引用组空集必须 fail-closed 且有目标组与过滤层诊断。
```

### Constraint DOC-0236 <a id="hcr-doc-0236"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/design.md:29`
- **Source Digest**: `254d3f0d4fa6391fb1258f53a74b13e4356b5c2de49a451bc2369e2d9ecdccef`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 5. API/UI 与失败根因取证

探测计划：`GET/PUT /api/v1/probes/schedule`、`GET /api/v1/probes/batches`、`POST /api/v1/probes/batches/{id}/cancel`；全局筛选：`GET/PUT /api/v1/policies/global-node-filter`（在 router 的已认证 admin 范围内，实际公共路由前缀以 router 注册为准）；现有组 CRUD 追加可选 `node_filter`。UI 在探测页呈现启用/周期/种类/批次/状态，在策略页呈现全局/组级条件及预览数量、排除原因和空组阻断，不在浏览器实现另一套筛选语义。对于用户报告的“csp出错”：先只读复核当前编译目标能力与提交 `7289b4c`；需从用户获取本次 URL/时间、请求 ID、脱敏错误正文与版本/容器日志才可诊断新错误；不从线上 DB/密钥/日志取证。既有 `PROCESS-NAME` 未支持假说已由提交排除，手动探测不存在假说由 `http/probes.go` 排除，周期配置已存在假说由现有 `settings.go` 排除；其余当前错误根因未知，不将未知错误纳入伪修复任务。
```

### Constraint DOC-0237 <a id="hcr-doc-0237"></a>

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

### Constraint DOC-0238 <a id="hcr-doc-0238"></a>

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

### Constraint DOC-0239 <a id="hcr-doc-0239"></a>

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

### Constraint DOC-0240 <a id="hcr-doc-0240"></a>

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

### Constraint DOC-0241 <a id="hcr-doc-0241"></a>

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

### Constraint DOC-0242 <a id="hcr-doc-0242"></a>

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

### Constraint DOC-0243 <a id="hcr-doc-0243"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/design.md:40`
- **Source Digest**: `fd9c6d3a09d4f21a4d4641cf303297abec9a55eecf73977ee661099f3e222cd3`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 探针健康权威性修正与 `undetermined` 状态分流

- **决策**：
1. 在 `web/src/features/nodes/nodeView.ts` 中删除 `if (values.includes('available')) return 'healthy'`；
2. 节点整体健康状态仅在拥有当前 `connection_revision` 且处于新鲜期内的 baseline 观测时才可判定为 `healthy` / `degraded` / `unhealthy`；
3. 当基线不存在、过期、版本不匹配，但存在其他有效探测（如 AI、IPRisk 等）或探测结果待确认时，分类为 `undetermined`（待复核）；
4. 仅在节点没有任何探测观测时，才标记为 `untested`（未检测）；
5. 后端 `domain.ProbePoolStatus` 结构体新增 `UndeterminedCount int json:"undetermined_count"`；
6. 后端 `/api/v1/nodes` 支持 `health_status` 查询参数，在数据库层面通过 `buildNodeFilterPredicates` 进行全 Scope 过滤，禁止依赖前端内存计算全量。
```

### Constraint DOC-0244 <a id="hcr-doc-0244"></a>

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

### Constraint DOC-0245 <a id="hcr-doc-0245"></a>

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

### Constraint DOC-0246 <a id="hcr-doc-0246"></a>

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

### Constraint DOC-0247 <a id="hcr-doc-0247"></a>

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

### Constraint DOC-0248 <a id="hcr-doc-0248"></a>

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

### Constraint DOC-0249 <a id="hcr-doc-0249"></a>

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

### Constraint DOC-0250 <a id="hcr-doc-0250"></a>

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

### Constraint DOC-0251 <a id="hcr-doc-0251"></a>

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

### Constraint DOC-0252 <a id="hcr-doc-0252"></a>

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

### Constraint DOC-0253 <a id="hcr-doc-0253"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/design.md:54`
- **Source Digest**: `7bec13d5ca29598309c4469e4913586d904cf9c014eafe207e5713db2a18957f`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 历史误伤节点证据化精准修复程序

- **设计选择**：
- **误伤数据特征归因**：2026-09-29T00:52 发生的批量失活系由当时的全局孤儿 SQL 触发。修复脚本通过排查以下条件锁定误伤节点集合：
1. `active = 0` 且 `updated_at` 处于该批次误伤时间窗口；
2. 节点的规范配置 `normalized_config_secret_ref` 完好且协议参数合法；
3. 未存在用户手工在控制面点击禁用的审计记录。
- **证据记录与幂等性**：恢复脚本编写为独立工具/脚本，先生成包含待修复节点 ID、名称、协议的 JSON 审计清单，在测试数据库副本上全流程演练无误后方可由授权运维应用；
- **安全约束**：严禁执行全量 `SET active = 1 WHERE active = 0`；对于无法确定归因的存量节点保留在 `pending_review` 列表中供人工确认。
```

### Constraint DOC-0254 <a id="hcr-doc-0254"></a>

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

### Constraint DOC-0255 <a id="hcr-doc-0255"></a>

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

### Constraint DOC-0256 <a id="hcr-doc-0256"></a>

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

### Constraint DOC-0257 <a id="hcr-doc-0257"></a>

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

### Constraint DOC-0258 <a id="hcr-doc-0258"></a>

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

### Constraint DOC-0259 <a id="hcr-doc-0259"></a>

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

### Constraint DOC-0260 <a id="hcr-doc-0260"></a>

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

### Constraint DOC-0261 <a id="hcr-doc-0261"></a>

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

### Constraint DOC-0262 <a id="hcr-doc-0262"></a>

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

### Constraint DOC-0263 <a id="hcr-doc-0263"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/design.md:79`
- **Source Digest**: `35dd882a04c76d0877aee207e2aeef24aa989e207e41c3484a5f321184c8fd8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 前端探针工作台三层视觉与交互架构（`web/src/features/probes/**`）

- **状态管理增强（`useProbes.ts` & `probeTypes.ts`）**：
- `useProbes.ts` 组合管理节点测速列表数据（通过 `GET /api/v1/nodes?page=...&page_size=100&active_only=true` 拉取全量活跃节点及其 `capabilities`、`latency_ms`、`health_status`、`sources`）、订阅源列表（`GET /api/v1/subscriptions` 用于显示订阅源名称与按订阅源筛选）、实时测速进度（跟踪当前活动的 `ProbeRun` 及已完成节点观测数 / 目标节点总数）、单节点/多节点/全量测速触发、节点级历史观测查询（`GET /api/v1/nodes/{logical_id}/observations`）以及任务级观测查询（`GET /api/v1/probes/runs/{run_id}/observations`）。
- 在 `probeTypes.ts` 中提供 `parseRedactedSummary(summary: string)` 辅助函数，将 `profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=42` 解析为结构化字段 `{ profile, version, verdict, reason, reasonLabel, statusCode, latencyMs, error, raw }`，将机器码（如 `contract_matched` -> `协议握手与响应校验通过`、`transport_error` -> `网络连接或握手失败`、`access_restricted` -> `目标服务访问受限 / 触发验证`、`missing_exit_identity` -> `未能识别出口 IP 身份`）转换为清晰中文说明。
- **第一视觉层级：全景节点健康与延迟统计看板（KPI Summary Bar）**：
- 顶部 4 张响应式统计卡片：
1. **在线可用率**：`可用节点数 / 总活跃节点数` + 百分比进度条 + 正常/降级/异常/未测分布徽章；
2. **平均响应延迟**：全网可用节点平均 `ms` + 极速（`<100ms`）/ 良好（`100-250ms`）/ 较慢（`>250ms`）节点数分布；
3. **流媒体与 AI 解锁**：已解锁流媒体节点数 / 已支持 AI 节点数 / 低风险 IP 节点数；
4. **定时自动测速状态**：当前开关状态（支持一键切换启用/停用）、易读定时间隔（如“每 1 小时”）、下次自动测速时间、以及「调整定时策略」快捷按钮。
- **第二视觉层级：直观的一键测速与多维筛选控制栏（Action & Filter Toolbar）**：
- **主操作按钮**：
* **「⚡ 一键全量测速」**：无需弹窗填写 `config_revision`，直接按当前开启的探测维度胶囊对全部活跃节点发起测速，并在工具栏下方展示实时测速进度横幅（显示正在测速状态、已完成观测进度条与「取消测速」按钮）；
* **「🎯 测速已选节点 (N)」**：当用户在表格勾选了节点（或一键全选当前筛选结果）时，直接对选中节点发起定点测速；
- **探测维度直观胶囊开关（Probe Dimension Pills）**：
* 工具栏直接排列可点击开启/关闭的胶囊按钮：`⚡ 连通与延迟 (必选)`、`🎬 流媒体解锁`、`🤖 AI 服务可用性`、`🛡️ IP 风险评估`、`🌍 落地地区识别`、`🚀 带宽测速`；
- **多维节点筛选与排序**：
* 关键词搜索（匹配节点名 `display_name` / 服务器 `server`）、协议筛选（全部 / SS / VMess / VLESS / Trojan / Hysteria2 / WireGuard / TUIC）、订阅源筛选、健康状态筛选（全部 / 正常 / 降级 / 异常 / 未测速）、排序方式（按延迟从快到慢 / 从慢到快 / 按名称）。
- **第三视觉层级：以“节点”为主体的实时测速结果表格/卡片列表（Node Probe Workbench Table）**：
- 桌面端使用清晰的响应式表格，移动端自适应为紧凑节点测速卡片；
- 每行展示：
* 复选框（支持全选当前筛选列表 / 多选定点测速）；
* **节点名称 (`display_name`)** + 协议标签 + 服务器地址（`server:port`）+ 所属订阅源标签；
* **连通状态与延迟 (`latency_ms`)**：语义化颜色毫秒徽章（绿色 `<100ms`、黄色 `100-250ms`、橙色 `>250ms`、红色 `超时/不可达`、灰色 `未测速`）；
* **能力矩阵徽章**：`流媒体`（解锁/受限/未测）、`AI`（可用/受限/未测）、`地区`（可用/未测）、`IP 风险`（低风险/中风险/高风险/未测）；
* **最后测速时间**：人性化相对时间（如 `刚刚`、`3 分钟前`）或本地时间；
* **行级快捷操作**：「⚡ 立即重测」（单击直接对该节点发起单节点测速并展示行级 loading 状态）+ 「查看详情」（打开该节点的测速详情抽屉）。
- **人性化定时自动测速弹窗与后台任务历史折叠区（`ProbeRunCard.vue` 重构）**：
- 定时自动测速配置弹窗：提供预设周期快捷按钮（`每 15 分钟`、`每 30 分钟`、`每 1 小时`、`每 6 小时`、`每 12 小时`、`每天`）+ 自定义分钟数输入 + 探测维度勾选 + 启用开关；
- 后台任务执行历史（手动测速任务 & 自动定时批次）：收敛在工作台底部的可展开/折叠面板（或次级视图切换），`ProbeRunCard.vue` 标题改为**「手动测速任务 · HH:mm:ss」**（自动批次为**「自动定时批次 · HH:mm:ss」**），展示运行状态、耗时与取消/查看结果按钮，彻底隐藏 UUID、`actor_scope`、`config_revision`、`lease_until`、`generation`、`owner` 等底层机器字段。
- **人类可读的测速详情抽屉（`ProbeEvidenceSheet.vue` 重构）**：
- 支持两种打开模式：**按节点查看历史测速详情**（主模式，展示节点名称 `display_name`、协议、`server:port`、综合健康度与最新延迟，并提供「⚡ 重新测速此节点」按钮）与**按任务查看批次测速结果**（自动映射 `node_logical_id -> display_name`）；
- 将每条观测记录的 `redacted_summary` 解析为结构化中文卡片：展示「检测项目（如：基础连通与延迟 / 流媒体解锁 / AI 服务可用性）」、「结论徽章（可用 / 受限 / 不可达）」、「响应延迟（`42 ms`）」、「状态说明（如：HTTP 204 连通正常 · 协议校验通过）」、「检测时间（本地格式化时间）」，不再向用户直接裸露 `profile=... version=...` 原始机器串或冗长 SHA256 指纹。
```

### Constraint DOC-0264 <a id="hcr-doc-0264"></a>

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

### Constraint DOC-0265 <a id="hcr-doc-0265"></a>

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

### Constraint DOC-0266 <a id="hcr-doc-0266"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:1`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md`。规划基于 dev HEAD `23b8af8` 的只读证据，不是生产网络复验：992 节点中 21 启用，最近 21 条 baseline 为 error；18 条 `credentials_unavailable` 不能据此判线路坏（含 6 条不安全 TLS 拒绝、12 条有 UUID 的 VLESS），1 条私有目标拒绝，2 条 `transport_error` 原因未确权。`safe_dialer.go` 把安全拒绝和客户端构建错误包成缺凭据；`builder.go` 的 VLESS 构建路径缺 Flow，而 `compiler_export.go` 有 Flow。`inventory/service.go` 与 `probe/service.go` 在 baseline 缺失/未知时从其他能力推整体状态，也缺乏可靠的旧观测与当前连接参数关联。`periodic.go` 已有批量观测查询、512 预算、5 分钟失败冷却、CAS 租约/队列去重，但扫描默认 10 分钟，手动 `TriggerImmediate` 会更新 `next_due_at`；前端刷新按钮只做 GET。
```

### Constraint DOC-0267 <a id="hcr-doc-0267"></a>

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

### Constraint DOC-0268 <a id="hcr-doc-0268"></a>

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

### Constraint DOC-0269 <a id="hcr-doc-0269"></a>

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

### Constraint DOC-0270 <a id="hcr-doc-0270"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:27`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 一分钟定时器与增量候选分离；手动复用扫描器

在 `PeriodicCoordinator` 将 `defaultSweepInterval` 改为 1 分钟并同步 `Recover` 对遗留远期 `next_due_at` 的校正，保留 `IntervalSeconds`、`ListLatestByNodes` 和失败冷却逻辑。筛选规则：无/到期观测或连接版本不匹配；当前仍在池跳过，配额 512 **按实际 (node,kind) 任务数计算**，不因单节点多类别越限，最老/从未测优先且下一分钟继续；失败冷却与配置变化同时发生亦遵守保护窗口并后续重试。过期是否重测基于最新观测时间，不基于读取发生时间；每次轮询页面或写入观测不能触发额外批次。保持无待测批次的可查询状态及 CAS 租约；如果 `TriggerImmediate` 已有推进 next_due 副作用，将手动路径与自动窗口推进分离：手动只触发一次同样的有界增量扫描，绝不修改计划的下次到期时间，仍受跨实例协调与队列去重。接口优先复用已有 `POST` schedule trigger（不是 GET，更不是全量测速），响应保留池字段并提供兼容的可选入队/无待测反馈；需确保失败不会静默报成功。页面先刷新状态后发该 POST，跟踪批次/节点状态直至完成；原“全量测速”独立保留，空闲 GET 轮询不能偷偷发 POST。
**备选**：每分钟固定跑全部协议将压垮队列；把手动刷新设成 `TriggerRun` 会突破增量语义；手动复位 `next_due_at` 会令连续点击延后后台时钟，均拒绝。
```

### Constraint DOC-0271 <a id="hcr-doc-0271"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/design.md:33`
- **Source Digest**: `6e5dff1ad66759672291199bd88b2cef02d587dcc1466d7f60079f89d702af83`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 4. DAG 与文件所有权

先冻结共享状态函数的签名、观测原因枚举、连接版本读写契约（特性包 A 的先决子阶段，非独立微派单）；A 实施完整探测编译/错误分类及版本记录（`internal/probe/singbox/**`、`internal/application/probe/{safe_dialer,runner}*`、`internal/domain/`、`internal/repository/sqlite/` 与迁移、对应离线测试）。A 完成后，B 整体健康视图/pool 统一（`internal/application/inventory/**`、`internal/application/probe/service.go` 的统计、共用状态规则，后端离线测试）；C 分钟扫描/手动后端（`internal/application/probe/periodic.go`、`service.go` 的 trigger、HTTP handler、配套测试）。B、C 都需要共享 `probe/service.go`，**必须串行**；D 在 C 后实施前端刷新/文字/交互与测试（`web/src/features/probes/**` 及有必要的节点台账/i18n）；B/C 自测完成后才做全量汇聚审查。不得以划分并行任务为由交叉改写共享文件；每个包包含实现 + 自测，不独立派微任务。
```

### Constraint DOC-0272 <a id="hcr-doc-0272"></a>

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

### Constraint DOC-0273 <a id="hcr-doc-0273"></a>

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

### Constraint DOC-0274 <a id="hcr-doc-0274"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:1`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### Context

当前 `dev` 分支（HEAD `5d75644`）的探针执行器与 SQLite 观测仓储存在以下已确认源码级根因（背景动机详见 `proposal.md`）：
1. **`internal/application/probe/runner.go:676-698` 无差别状态码匹配导致全维度伪阳性**：
- `executeTaskWithVerdict` 对所有 `ProbeKind` 均执行 `result.ContractMatched = resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent` 且 `result.ContractVersion = prof.Contract`；
- `baseline`（目标 `http://cp.cloudflare.com/generate_204`）收到 `HTTP 200`（如 Captive Portal 页面或任意 `"OK"` 响应）也被标为 `available`，进而错误放行节点进入 Phase 2；
- `geo`（目标 `https://api.ip.sb/geoip`）未解析响应体即把空响应或任意 `200/204` 标为 `available`，未使用项目已有的 `internal/probe/identity.ExtractCandidate` 校验出口 IP 与 2 位国家代码；
- `streaming`（`https://www.netflix.com`）与 `ai`（`https://api.openai.com`）仅请求根路径首页，只要返回 `200/204` 且不含通用挑戰词即被判为 `available`（“已解锁”）；
- `ip_risk`（`https://cloudflare.com/cdn-cgi/trace`）访问的仅是 Cloudflare 出口身份追踪端点而非风险评分源，`runner.go` 从未设置 `ExitIdentityMissing` 或调用真实风险源，却直接把 `200` 判为 `available`。
2. **`internal/application/probe/runner.go:650-693, 700-706` 带宽测速（`speed`）计时错位与伪负载放行**：
- `latency = r.clock().Sub(reqStart).Milliseconds()` 在 `client.Do(req)` 返回响应头后立即计算，**早于** `readBoundedResponse(resp.Body, readLimit)` 读取 1 MiB 负载；随后却用 `readBoundedResponse` 读出的 `result.BytesRead` 除以响应头延迟计算 `throughput_kbps = (result.BytesRead * 8) / latency`，导致测速耗时遗漏主体传输时间、吞吐率严重虚高；
- 同时 `speed` 对 `204`、空响应体或几字节的 `"OK"` 伪响应也判定 `ContractMatched = true`。
3. **`internal/application/probe/runner.go:630-633` 与 `internal/probe/singbox/runtime.go:108` 节点客户端重复创建及上下文生命周期陷阱**：
- `executeTaskWithVerdict` 在每个 `(node, kind)` 任务内独立调用 `r.dialer(ctx, node)` 并 `defer cleanup()`；当单次 Run 对单节点探测 `K` 个维度时，重复执行 `K` 次 DNS 解析校验、配置构建与内存 `sing-box` 实例启停；
- 特别注意：`singbox.NewWithBoxConfig(ctx, ...)`（`internal/probe/singbox/runtime.go:108`）会注册 `context.AfterFunc(ctx, func() { _ = rt.Close() })`。若复用客户端时将首个任务的短生命周期 `taskCtx`（单任务结束即 `taskCancel()`）传给 `r.dialer`，首个任务一结束就会触发 `rt.Close()` 导致同节点后续维度任务全部因 `ErrRuntimeClosed` 失败。
4. **`internal/repository/sqlite/probes.go:380-465` 最新观测查询全表历史拉取**：
- `ListLatestByNodes` 执行 `SELECT ... FROM probe_observations WHERE node_logical_id IN (...) [AND kind IN (...)] ORDER BY observed_at DESC, id DESC;`，将匹配节点的**全部历史观测行**传输到 Go 进程内存再通过 `if _, exists := nodeObs[obs.Kind]; !exists` 丢弃旧行。
```

### Constraint DOC-0275 <a id="hcr-doc-0275"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:20`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### 基线可测指标（Baseline vs. Target Metrics）

| 指标维度 | 当前基线（HEAD `5d75644`） | 目标契约指标 | 验证方式 |
|---|---|---|---|
| `baseline` 收到 `HTTP 200` 或非空 `204` 的判定 | `available`（伪阳性） | `unknown` (`reason=contract_drift`)，仅 `204` + `0` 字节正文为 `available` | `profiles_test.go` & `runner_test.go` 表驱动单测 |
| `geo` 收到无合法 IP/2位国家代码的 `200/204` | `available`（伪阳性） | `unknown` (`reason=contract_drift`)，仅 `200` + 合法 IP + 2位 ISO 国家码为 `available` | `profiles_test.go` & `runner_test.go` 单测 |
| `streaming` / `ai` 收到首页 `200` / 空响应 / 普通非空正文 | `available`（伪解锁） | 无明确限制信号时恒为 `unknown` (`reason=contract_drift`)；有限制信号时为 `restricted` | `profiles_test.go` & `runner_test.go` 单测 |
| `ip_risk` 仅收到 `cdn-cgi/trace` 或无真实风险源 | `available`（伪风险通过） | 无出口身份时 `unknown` (`missing_exit_identity`)；仅有出口身份无真实风险源时 `unknown` (`contract_drift`) | `profiles_test.go` & `runner_test.go` 单测 |
| `speed` 计时区间与极短伪响应（`< 1024B` 或 `204`） | 仅计 `client.Do` 响应头耗时；极短响应判 `available` | 计时覆盖 `client.Do` + `readBoundedResponse` 全过程；`< 1024B` 或非 `200` 判 `unknown`；超预算仍返 `error` (`speed_budget_exceeded`) | 慢速流式 Reader 计时单测 + 边界字节单测 |
| 单次 `ProbeRun` 内单节点探测 `K` 个可用维度的 `dialer` / `cleanup` 次数 | `K` 次 `dialer`、`K` 次 `cleanup` | `1` 次 `dialer`、在该节点全部任务 `OnComplete` 完成后 `1` 次 `cleanup`（取消/失败 `0` 泄漏） | 原子计数器并发/两阶段/取消/失败单测 |
| `ListLatestByNodes` 查询 `N` 节点 × `K` 维度 × 每组合 `H` 条历史记录时 SQLite 返回行数 | `N × K × H` 行（随历史深度线性膨胀） | DB 内窗口函数筛选 `rn = 1`，返回 `<= N × K` 行（与历史深度 `H` 解耦） | `sqlite` 回归测试 + 离线 `BenchmarkListLatestByNodes` |
```

### Constraint DOC-0276 <a id="hcr-doc-0276"></a>

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

### Constraint DOC-0277 <a id="hcr-doc-0277"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:48`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### Decision 1: 各 `ProbeKind` 响应真实性校验与 `profiles.Evaluate` 协同契约

- **方案设计**：
1. **保留已有 `internal/probe/identity.ExtractCandidate` 轮子（Prior Art First）**：`internal/probe/identity/identity.go` 已实现成熟的 JSON（`ip`/`query` + `country_code`/`country`/`countryCode`）与 Cloudflare Trace（`ip=...` + `loc=...`）双格式解析及 `net.ParseIP` + 2 位 ISO 国家代码校验。`runner.go`（或 `profiles` 辅助验证函数）直接复用 `identity.ExtractCandidate(body)` 验证 `geo` 与 `ip_risk` 的出口身份负载，绝不重复造轮子。
2. **按维度精确定义契约匹配条件（替代 `resp.StatusCode == 200 || resp.StatusCode == 204`）**：
- **`ProbeKindBaseline`**：要求 `StatusCode == http.StatusNoContent (204)` 且 `len(Body) == 0 && BytesRead == 0`。若 `StatusCode == 200` 或响应体非空（且未命中 `hasRestrictionMarker` / `401` / `403`），则 `ContractMatched = false`，`Evaluate` 返回 `VerdictUnknown`（`reason=contract_drift`）。同时在 `profiles.Profile.Evaluate` 中加固：当 `p.Kind == domain.ProbeKindBaseline` 时，若 `result.StatusCode != http.StatusNoContent || len(result.Body) > 0 || result.BytesRead > 0`，即便调用方传入 `ContractMatched: true` 也返回 `VerdictUnknown`（`reason=contract_drift`）。
- **`ProbeKindGeo`**：要求 `StatusCode == http.StatusOK (200)` 且 `identity.ExtractCandidate(body)` 成功返回合法 IP 与 2 位国家代码。若状态码非 `200` 或解析失败，则 `ContractMatched = false`，返回 `VerdictUnknown`（`reason=contract_drift`）。当解析成功时，可在 `RedactedSummary` 中安全附加脱敏的 `country=<CC>`（绝不写入完整出口 IP）。
- **`ProbeKindStreaming` 与 `ProbeKindAI`**：
- 优先检查限制信号：当 `StatusCode` 为 `401`、`403`、`451` 或 `hasRestrictionMarker(body)` 为真（或包含明确地区/访问封锁标记）时，返回 `VerdictRestricted`（`reason=access_restricted`）；
- 检查传输/超时错误：`DNSError || NetworkError || DeadlineExceeded` 返回 `VerdictError`（`reason=transport_error`）；
- 在 `DefaultRunner` 中，由于仅请求 `https://www.netflix.com` 或 `https://api.openai.com` 根路径不具备可验证的业务解锁契约，`runner` 对 `streaming` 与 `ai` 恒设置 `result.ContractMatched = false`，使所有未受限的 `200`/`204` 响应（无论空响应、`"OK"` 或普通首页 HTML）均由 `Evaluate` 归类为 `VerdictUnknown`（`reason=contract_drift`），绝不产生虚假的 `VerdictAvailable`。
- **`ProbeKindIPRisk`**：
- `runner` 请求 `https://cloudflare.com/cdn-cgi/trace` 后，使用 `identity.ExtractCandidate(body)` 检验出口身份：若 `StatusCode != http.StatusOK` 或无法提取合法出口 IP + 2 位国家代码，则设置 `result.ExitIdentityMissing = true`，由 `Evaluate` 返回 `VerdictUnknown`（`reason=missing_exit_identity`）；
- 若成功提取出口身份（`ExitIdentityMissing = false`），由于 `cdn-cgi/trace` 仅证明出口身份而非真实 IP 风险情报源（`iprisk.Provider`）的风险评分结果，`runner` 保持 `result.ContractMatched = false`，由 `Evaluate` 返回 `VerdictUnknown`（`reason=contract_drift`），绝不把出口身份当成风险评分 `available`。
- **`ProbeKindSpeed`**：
- 在 `profiles` 包定义常量 `MinValidSpeedBytes int64 = 1024`（1 KiB）；
- 耗时测量：在 `runner.go` 中将 `latency = r.clock().Sub(reqStart).Milliseconds()` 移至 `readBoundedResponse(resp.Body, readLimit)` 完整读取（并关闭 `resp.Body`）之后执行，使 `latency` 包含连接建立、请求发送、响应头接收与完整响应体传输的全部时间；
- 预算超限优先判定：当 `readBoundedResponse` 返回 `errResponseTooLarge`（超出 `prof.SpeedBudget.MaxBytesPerRequest`）时，保持 `result.BytesRead = prof.SpeedBudget.MaxBytesPerRequest + 1`，`Evaluate` 优先命中 `bytesRead > p.SpeedBudget.MaxBytesPerRequest || bytesRead > p.SpeedBudget.MaxBytesPerRun` 并返回 `VerdictError`（`reason=speed_budget_exceeded`）；
- 最小有效负载校验：在未超预算且无网络错误时，仅当 `resp.StatusCode == http.StatusOK (200)` 且 `result.BytesRead >= profiles.MinValidSpeedBytes (1024)` 时才设置 `result.ContractMatched = true`（并在 `Profile.Evaluate` 中同步校验 `result.StatusCode == http.StatusOK && bytesRead >= MinValidSpeedBytes`），从而拒绝 `204`、空响应（`0B`）或 `"OK"`（`2B`）等极短伪响应（返回 `VerdictUnknown` / `reason=contract_drift`）。
- **备选方案对比**：
- *备选方案*：在 `streaming` / `ai` 中通过正则匹配页面零散关键词（如 `subs-check-pro v3.2.1` 的做法）猜测解锁状态。
- *放弃理由*：此类私有规则缺乏协议级稳定性，极易将 CDN 静态页、区域重定向页或未登录门户误判为 `available`，直接违反本次治理契约。
```

### Constraint DOC-0278 <a id="hcr-doc-0278"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:71`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): policy member interleaved ordering pending user decision on Clarification Q2.
- **Original Source Text**:
```text
### Decision 2: 同 `ProbeRun` 同节点客户端生命周期复用管理器（`runNodeSession`）

- **方案设计**：
1. **Run 级节点客户端池（`nodeSessionPool`）**：
- 在 `DefaultRunner.Run` 内部创建局部的 `nodeSessionPool`（按 `node.LogicalID` 索引 `*nodeSession`）。
- 每个 `nodeSession` 持有：`once sync.Once`、`client *http.Client`、`cleanup func() error`、`dialErr error`、`sessionCtx context.Context`、`sessionCancel context.CancelFunc`、`closeOnce sync.Once` 以及当前阶段剩余任务引用计数 `pendingTasks int32`（配合互斥锁保护）。
2. **解决 `singbox.Runtime` 绑定 `ctx` 提前关闭问题**：
- `nodeSession` 初始化时基于 `runCtx`（而非单个任务的 `taskCtx`）派生 `sessionCtx, sessionCancel := context.WithCancel(runCtx)`，并在首次拨号时将受拨号超时约束但生命周期绑定至 `sessionCtx`（或直接传入 `sessionCtx`，若自定义 `dialer` 需要）的上下文传给 `r.dialer(dialCtx, node)`。
- 这样底层 `singbox.NewWithBoxConfig` 注册的 `context.AfterFunc` 绑定在 `sessionCtx` 上，不会在第一个任务（如 Phase 1 `baseline`）结束 `taskCancel()` 时被误关停；同时每个 HTTP 请求仍通过 `req = req.WithContext(taskCtx)` 严格执行单任务 `TaskTimeout` / `SpeedBudget.Deadline`。
- 若返回的 `client != nil && client.CheckRedirect == nil`，在 `once.Do` 初始化期间一次性克隆并挂载 `CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }`，避免多任务并发读写 `client.CheckRedirect` 产生数据竞争。
3. **精确引用计数与任务回调完成后关闭**：
- **单阶段模式（`!isMixedTwoPhase`）**：
- 在为某节点提交任务前，按该节点即将提交的任务数初始化/递增 `pendingTasks`；若某个任务的 `SubmitWithContext` 失败，立即递减 `pendingTasks`（若归零则触发 `session.close()`）；
- 每个任务的 `OnComplete` 回调在调用 `wg.Done()` 前（或同时）递减该节点的 `pendingTasks`，当某节点的 `pendingTasks` 归零时（即该节点在本 Run 的全部任务及回调均已完成），立即通过 `closeOnce.Do` 调用 `sessionCancel()` 与 `cleanup()`。
- **两阶段模式（`isMixedTwoPhase`：Phase 1 `baseline` -> Phase 2 `expensiveKinds`）**：
- Phase 1 期间每个节点仅执行 `baseline` 任务，Phase 1 的 `OnComplete` 不立即关闭客户端，以便通过 `baseline`（`VerdictAvailable`）的节点在 Phase 2 直接复用已建立的客户端；
- 在 `stage1WG.Wait()` 完成后：对于**未进入** `stage2Nodes`（即 `baseline` 非 `available` 或 Phase 1 提前终止）的所有节点，立即调用 `session.close()` 释放其客户端；
- 对于进入 `stage2Nodes` 的节点，按其在 Phase 2 的任务数设置 `pendingTasks`，在每个 Phase 2 任务的 `OnComplete` 回调中递减，当该节点在 Phase 2 的全部任务回调完成（`pendingTasks == 0`）或提交失败归零时，立即调用 `session.close()`。
- **全局退出兜底（Cancel / Deadline / Error 零泄漏保障）**：
- 在 `Run` 的所有退出路径（含 `handleCancelOrDeadline` 在 `<-done` 收敛之后、Phase 1/Phase 2 提前返回之后）统一通过 `defer pool.closeAll()` 确保在所有工作协程和 `OnComplete` 回调结束之后，对任何尚未关闭的 `nodeSession` 执行 `closeOnce.Do`，保证 `cleanup()` 100% 精确执行 `1` 次且绝无资源泄漏。
- **备选方案对比**：
- *备选方案*：跨不同 `ProbeRun` 全局长连接缓存 `sing-box` 实例。
- *放弃理由*：跨 Run 全局驻留内存 `sing-box` 实例会在空闲期持续占用套接字与内存，且当节点配置更新时引发陈旧凭据/DNS 固定状态不一致；按单次 `ProbeRun` 内同节点复用即可将多维探测的拨号开销从 `O(N × K)` 降至 `O(N)`，同时保证 Run 结束后内存 100% 归零释放。
```

### Constraint DOC-0279 <a id="hcr-doc-0279"></a>

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

### Constraint DOC-0280 <a id="hcr-doc-0280"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/design.md:128`
- **Source Digest**: `35166f4e2a5e9a16c6aba6219a6f752cdab0c3bc8846e3fa3729a2b2b7a95820`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **[Risk] 既有测试夹具中部分 `baseline` / 全量多维测试使用 `HTTP 200 "OK"` 桩响应** → **Mitigation**：
- 在工作包 1（写集合 `internal/application/probe/**`、`internal/probe/profiles/**`）中，同步更新 `runner_test.go`、`stage_gate_test.go` 中以“验证 `baseline` 可用通过”为前提的测试夹具使其对 `baseline` 返回 `204 No Content` 空响应（对 `geo` 返回合法 IP+国家代码 JSON、对 `speed` 返回 `>= 1024` 字节负载），同时保留并新增专门针对 `baseline` 返回 `200 "OK"` 必须被拒为 `unknown` 的回归测试，以及更新 `profiles_test.go` 中 `TestIPRiskMatchedContractProducesAvailable` 等单测以对齐收紧后的契约。
- **[Risk] 单阶段并发多维探测时同一节点的多个任务同时首次触发 `getOrCreateClient`** → **Mitigation**：
- 每个 `nodeSession` 使用 `sync.Once` 串行化该节点的单次 `r.dialer` 调用，并发到达的同节点任务等待 `sync.Once` 完成后共享同一个 `*http.Client`，且 `CheckRedirect` 在 `sync.Once` 内一次性配置完成，彻底消除并发初始化竞争。
- **[Risk] `EnqueuePeriodicDedupe` 静默跳过任务时会在 `SubmitWithContext` 内同步调用 `task.OnComplete(nil)`** → **Mitigation**：
- 引用计数在调用 `SubmitWithContext` **之前**预先递增，在 `OnComplete` 回调内递减，若 `SubmitWithContext` 返回非 `nil` 错误（此时调度器未调用 `OnComplete`）则在错误处理分支递减，确保无论任务正常执行、被周期去重同步调用 `OnComplete(nil)`、被 `CancelRun` 取消回调还是提交报错，引用计数均严格守恒。
```

### Constraint DOC-0281 <a id="hcr-doc-0281"></a>

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

### Constraint DOC-0282 <a id="hcr-doc-0282"></a>

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

### Constraint DOC-0283 <a id="hcr-doc-0283"></a>

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

### Constraint DOC-0284 <a id="hcr-doc-0284"></a>

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

### Constraint DOC-0285 <a id="hcr-doc-0285"></a>

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

### Constraint DOC-0286 <a id="hcr-doc-0286"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/design.md:36`
- **Source Digest**: `02eb8a19dbdf7bacd3df9c8d2be1554ee16423847c213dd43b6c8395e7f63dbd`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

本 Change 仅规划；实施由获授权施工者按任务执行。先保全并判明主仓资产，分离构建写边界；Go probe/停服与前端可用性可在公共契约冻结后按无重叠写集合并行，各自测试闭环；汇聚后在隔离环境建同源新实例，全量复验、只读独立 reviewer、再 critic。对另一 Pi 治理 Change 只报告其 3.1–3.3 尚未完成及真机不可用，不改其工件。无原仓资产来源或删除授权时不得清理未跟踪文件；实施是否可继续由主脑按证据决策，不以暂停冒充完成。
```

### Constraint DOC-0287 <a id="hcr-doc-0287"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:1`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md`。生产环境 9 个订阅源中 4 个处于启用状态，最近 6 条拉取审计记录显示 4 条 failed（TLS 握手超时、拉取超时与 HTTP 404）及 2 条 partial，且所有订阅的 `fetch_proxy_ref` 与 UA 均为空。容器内当前无代理环境变量。Clash Verge Rev 官方 v2.5.6（`src-tauri/src/utils/network.rs:246-255`）使用 `clash-verge/v2.5.6` 默认 UA 与 `reqwest` 标准请求头（`Accept: */*`, `Accept-Encoding: gzip, deflate`）。宿主机运行中 mihomo 监听于 `0.0.0.0:7890`，实测容器通过 `host.docker.internal:7890` 连通正常。
```

### Constraint DOC-0288 <a id="hcr-doc-0288"></a>

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

### Constraint DOC-0289 <a id="hcr-doc-0289"></a>

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

### Constraint DOC-0290 <a id="hcr-doc-0290"></a>

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

### Constraint DOC-0291 <a id="hcr-doc-0291"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/design.md:36`
- **Source Digest**: `4a5c8410547479d645ca6bbd37f5299ba66897a44f21f2b639b37c8a69c7d5d0`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. Go `net/http` 代理与 TLS 模型审计及应用层目标 IP 强制绑定

- **背景与实证**：
- 审查实测确认，生产环境宿主机 mihomo（`/root/clashctl/resources/runtime.yaml`）中私网与回环地址规则为 `DIRECT`，通过 `7890` 代理实际可穿透访问宿主机 `127.0.0.1:18080`。因此，**绝对不能依赖上游 mihomo ACL 防御代理端 SSRF 或 DNS Rebinding**。
- 审计 Go 标准库 `net/http` 源码确认：
1. 对于 HTTP 正向代理（`net/http/request.go` `Request.WriteProxy`）：当 `usingProxy` 为真且 `r.URL.Opaque == ""` 时，标准库用 `r.Host`（若非空）拼接 `ruri = r.URL.Scheme + "://" + host + ruri`，导致请求行仍发送原始域名；而当显式设置 `r.URL.Opaque = "//" + pinnedIPPort + escapedPath` 且 `r.Host = origHostHeader` 时，`WriteProxy` 发送的请求行固定为 `GET http://<pinnedIP>:<port><path>?<query> HTTP/1.1`，同时 `Host` 头保持 `Host: <origHostHeader>`。
2. 对于 HTTPS 代理隧道（`net/http/transport.go` `dialConn` & `addTLS`）：标准库使用 `cm.targetAddr = canonicalAddr(treq.URL)` 构造 `CONNECT <cm.targetAddr> HTTP/1.1`；随后调用 `addTLS(ctx, cm.tlsHost(), trace)`，其中 `if cfg.ServerName == "" { cfg.ServerName = name }`。因此，当每跳将 `treq.URL.Host` 设为已校验的 `pinnedIP:port`、将 `treq.Host` 设为 `origHostHeader`，并在 `Transport.TLSClientConfig` 中显式设置 `ServerName: tlsServerName`（原始目标域名）且保持 `InsecureSkipVerify: false` 时，代理收到的 `CONNECT` 权威目标永远是已验证公网 IP（不见原始域名），而隧道内 TLS 握手依然发送原域名 SNI 并对原域名执行严格的 X.509 证书校验。
- **决策**：
1. **显式每跳重验与目标 IP 强制绑定（Per-Hop Resolution & Forced IP Pinning）**：
- 禁用 `http.Client` 的隐式重定向跟随（`CheckRedirect` 返回 `http.ErrUseLastResponse`），在 `Client.Fetch` 中实现显式有界跳转循环（最多 `MaxRedirects` 跳）。
- 每一跳（含初始请求与每次 `301/302/303/307/308` 重定向）在发起网络请求前，先在本地通过 `policy.Resolver` 解析目标主机名，并对全部解析出的 IP 逐一执行 `policy.ValidateIP` 校验（遇到任何私网、回环、链路本地/云元数据地址立即返回 `ssrf_blocked`）。
- **HTTP 代理转发**：使用校验通过的公网 `pinnedIP:port` 构造 `req.URL.Host = pinnedAddr` 与 `req.URL.Opaque = "//" + pinnedAddr + escapedPath`，并设置 `req.Host = origHostHeader`，使代理仅看到公网 IP 目标且源站收到原 `Host` 头。
- **HTTPS 代理 CONNECT**：使用校验通过的公网 `pinnedIP:port` 构造 `req.URL.Host = pinnedAddr`，`req.Host = origHostHeader`，配置 `TLSClientConfig: &tls.Config{ServerName: origHostname, RootCAs: opts.RootCAs, MinVersion: tls.VersionTLS12, InsecureSkipVerify: false}`，使代理在 `CONNECT` 阶段仅看到 `pinnedIP:port`，而客户端在隧道内完成原域名 SNI 发送与严格证书验证。
2. **代理协议限定与白名单精确规范化（Strict `http://` Scheme & Canonical Whitelist）**：
- 仅允许显式 `http://` 代理协议；`socks5`、`https`、`ftp` 或无 scheme 输入一律失败闭合返回 `invalid_proxy_url`。
- 规范化规则：端口必须显式指定（1..65535）且严禁包含前导零（如 `:07890` 拒绝）；代理主机名严禁包含尾随点（如 `host.docker.internal.` 或 `127.0.0.1.` 拒绝）；严禁携带 userinfo、路径、查询参数或片段；IPv6 必须使用标准方括号格式并规范化为 RFC 5952 形式（严禁携带 `%` Zone ID 或 IPv4-mapped IPv6 `::ffff:...`）。
- `AddAllowedProxy` 与 `IsProxyHostAllowed` 严格按完整 `http://<canonical-host>:<canonical-port>` 精确匹配，彻底杜绝丢弃 scheme 或同端口跨协议豁免。
- 对于未命中白名单的自定义 `fetch_proxy_ref`：必须通过完整公网 SSRF 校验并解析绑定代理自身的公网 IP 进行建连，严禁绕过访问私网/回环/元数据。
3. **路由行为说明与 TOCTOU 边界如实披露**：
- **代理域名路由影响**：由于应用层强制向代理传递已验证的公网 IP（HTTP 请求行与 HTTPS CONNECT authority 均为 IP），如果上游代理依赖请求行/CONNECT 域名匹配分流规则（如仅配置 `DOMAIN-SUFFIX` 而未开启 TLS SNI 嗅探或对应 IP/FINAL 规则），流量在代理内部将按 IP 规则或默认出口路由。系统如实记录并测试该特性，绝不为此牺牲 SSRF 防护或关闭 TLS 证书校验。
- **TOCTOU 边界**：应用层强制将本地预解析并校验通过的公网 IP 传给代理，彻底消除了“客户端解析为公网 IP、代理端二次 DNS 解析重绑定为内网 IP”的代理侧 DNS Rebinding 攻击面；但对于公网 IP 在极短时间窗内的互联网 BGP/云弹性 IP 归属变更，系统不作不切实际的绝对消除宣称。
```

### Constraint DOC-0292 <a id="hcr-doc-0292"></a>

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

### Constraint DOC-0293 <a id="hcr-doc-0293"></a>

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

### Constraint DOC-0294 <a id="hcr-doc-0294"></a>

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

### Constraint DOC-0295 <a id="hcr-doc-0295"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/design.md:1`
- **Source Digest**: `a61cab822be5453efb60c4a3cf290a025964e405ba239e62efbece7ce32673c8`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): pure-node export empty-group validation bypass pending Clarification Q4.
- **Original Source Text**:
```text
### Context

1. **拓扑图校验报错根因（`POST /api/v1/policies/validate`）**：
- 前端 `web/src/features/policy/usePolicy.ts:206-212` 在调用 `POST /api/v1/policies/validate` 时直接传 `{ groups: groups.value, admission_rules: admissionRules.value, policy_rules: policyRules.value }`。其中 `groups.value` 的元素为 `PolicyGroup`（含 `edges: GroupEdge[]` 字段）。
- 后端 `internal/transport/http/policies.go:54-59` 定义的 `validateGraphRequest.Groups` 类型为 `[]domain.NodeGroup`，而 `domain.NodeGroup` 结构体不含 `edges` 字段；同时 `decodeJSON`（`internal/transport/http/probes.go:260`）启用了 `decoder.DisallowUnknownFields()`。
- 当请求到达 `policyHandler.validate`（`policies.go:352`）时，`decodeJSON(w, r, &body)` 因未知字段 `"edges"` 立即调用 `WriteDomainError` 写入 HTTP 422 `invalid_json` 响应并返回非 nil 错误；但 `policies.go:355` 写了 `_ = decodeJSON(w, r, &body)` 忽略了错误未 `return`，继续向下执行 `h.service.ValidateGraph` 或 `WriteSuccess`，导致响应被二次写入且前端始终收到 422 报错。
- 此外，即使 `groups` 不含未知字段，前端原先未在顶层构造 `edges: map[string][]GroupEdge`，导致 `body.Edges` 为空，无法校验画布中的边连接关系。
2. **订阅发布目标边界重构（仅 Mihomo 导出完整配置，其他目标仅导出节点格式并忽略规则）**：
- 现有 `internal/compiler/` 对 `mihomo`、`singbox`、`surge`、`qx` 四个目标均执行完整的 `Nodes + Groups + Rules` 校验与渲染。当用户的策略树包含 Mihomo 常用的 `fallback`、`loadbalance` 策略组或 `GEOSITE`、`PROCESS-NAME`、`RULE-SET` 规则时，`singbox`、`surge`、`qx` 在 `compiler.validate` 或渲染阶段即抛出 `CapabilityError`（HTTP 422 `unsupported_target_capability`）；同时 `publication.evaluatePreflight` 对所有目标均拦截 `empty_routed_group`。
- 用户明确指定业务契约：**“订阅发布只导出mihomo，其他是导出节点格式，忽略规则”**。即：
* `mihomo`：唯一导出完整客户端配置的目标（包含 `proxies`、`proxy-groups`、`rules`、`rule-providers`、`dns`）。
* `singbox`、`surge`、`qx`：纯节点格式导出目标，彻底忽略 `snapshot.Groups` 与 `snapshot.Rules`，不输出任何策略组与分流规则段落，也不因策略组或规则触发编译或预检错误。
3. **全站中文化缺口（Comprehensive `zh-CN` Localization）**：
- `web/src/locales/index.ts:12` 默认回退返回 `'en-US'`，`createI18n` 的 `fallbackLocale` 亦为 `'en-US'`，导致未手动切换语言的新会话默认显示英文。
- `web/src/locales/messages.ts` 虽已有基础字典，但大量视图组件（`App.vue`、`ErrorStateCard.vue`、`PolicyView.vue`、`PolicyEditorSheet.vue`、`GroupCard.vue`、`NodesView.vue`、`ProbesView.vue`、`ProbeRunCard.vue`、`ProbeEvidenceSheet.vue`、`PublicationsView.vue`、`SubscriptionsView.vue`、`SubscriptionConfigDrawer.vue`、`SettingsView.vue`）及 TS 模块（`policyTypes.ts`、`nodeView.ts`、`probeTypes.ts`、`publicationTypes.ts`、`use*.ts`）仍存在大量硬编码英文文案、未翻译表单校验提示与裸露枚举值。
```

### Constraint DOC-0296 <a id="hcr-doc-0296"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/design.md:19`
- **Source Digest**: `a61cab822be5453efb60c4a3cf290a025964e405ba239e62efbece7ce32673c8`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): pure-node export empty-group validation bypass pending Clarification Q4.
- **Original Source Text**:
```text
### Goals / Non-Goals

- **Goals**：
1. 彻底修复 `POST /api/v1/policies/validate`：无论客户端发送空请求体/`{}`、带内嵌 `edges` 的 `groups` 数组，还是规范化的 `{ groups, edges, rules, policy_rules, admission_rules }`，均能正确解析并返回拓扑校验结果；非法 JSON 请求体在 `decodeJSON` 失败后立即终止返回 422，绝不二次写响应。
2. 将 `singbox`、`surge`、`qx` 改造为纯节点导出目标：在 `compiler.validate`、各目标渲染器及 `publication` 预检中完全忽略 `Groups` 与 `Rules`，仅校验并输出节点列表格式；`mihomo` 保持完整配置编译与校验。
3. 将前端默认语言切换为 `'zh-CN'`，补齐 `zh-CN` 与 `en-US` 词条，消除全站视图组件与 TS 辅助模块中的硬编码英文及裸露枚举。
- **Non-Goals**：
- 不改变 SQLite 数据库表结构或迁移文件。
- 不修改生产环境运行中的容器状态，仅在隔离测试与预览环境中验证。
```

### Constraint DOC-0297 <a id="hcr-doc-0297"></a>

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

### Constraint DOC-0298 <a id="hcr-doc-0298"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/design.md:38`
- **Source Digest**: `a61cab822be5453efb60c4a3cf290a025964e405ba239e62efbece7ce32673c8`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): pure-node export empty-group validation bypass pending Clarification Q4.
- **Original Source Text**:
```text
### 2. 编译器与预检：Mihomo 完整配置 vs 非 Mihomo 纯节点格式导出（`internal/compiler/**` & `internal/application/publication/**`）

- **能力矩阵与校验逻辑（`internal/compiler/compiler.go`）**：
- `mihomoCapability()` 保持不变：支持全部 7 种协议、4 种策略组类型（`select`, `urltest`, `fallback`, `loadbalance`）与 14 种规则类型。
- 非 Mihomo 目标（`singbox`, `surge`, `qx`）：
* `Capability.GroupTypes` 与 `Capability.RuleKinds` 均设为空集合（`groupSet()`, `ruleSet()`），表明其仅作为节点格式导出，不承载策略组与路由规则。
* 在 `compiler.validate(snapshot, target, capability)` 中：先对 `snapshot.Nodes` 执行协议能力与节点字段校验；若 `target != domain.TargetMihomo`，**直接 `return nil`**，完全跳过后续对 `snapshot.Groups` 与 `snapshot.Rules` 的一切校验！
- **各目标渲染器输出格式**：
- `mihomo`（`internal/compiler/mihomo.go`）：保持完整 YAML 配置输出（`proxies`, `proxy-groups`, `rule-providers`, `rules`, `dns`）。
- `singbox`（`internal/compiler/singbox.go`）：
* `singBoxCapability()` 支持全部 7 种协议（`ss`, `vmess`, `vless`, `trojan`, `hysteria2`, `wireguard`, `tuic`）。
* `renderSingBox(snapshot)` 仅遍历 `snapshot.Nodes` 构建节点 `outbounds` 与 WireGuard `endpoints`，序列化为 `{"outbounds": [...], "endpoints": [...]}`（当无 endpoint 时仅 `{"outbounds": [...]}`），不生成任何策略组 selector/urltest outbound、内置 direct/block outbound 或 `route` 字段；生成的 JSON 仍完全通过官方 `sing-box check -c` 校验。
- `surge`（`internal/compiler/surge.go`）：
* `surgeCapability()` 支持 `ss`, `vmess`, `trojan`, `hysteria2`, `tuic`, `wireguard`（补齐 Surge 5 原生支持的 `hysteria2`、`tuic`、`wireguard` 节点行渲染；对 Surge 不支持的 `vless` 仍保持节点级 `CapabilityError` 拒绝）。
* `renderSurge(snapshot)` 仅输出纯节点行（每行一个 `<name> = <protocol>, <server>, <port>, ...`），不再输出 `[General]`、`[Proxy]`、`[Proxy Group]`、`[Rule]` 段落。
- `qx`（`internal/compiler/qx.go`）：
* `quantumultXCapability()` 支持 `ss`, `vmess`, `trojan`（对不支持的协议保持节点级 `CapabilityError` 拒绝）。
* `renderQuantumultX(snapshot)` 仅输出纯节点行（每行一个 `<protocol> = <server>:<port>, ..., tag=<name>`），不再输出 `[general]`、`[server_local]`、`[policy]`、`[filter_local]` 段落。
- **发布预检同步（`internal/application/publication/service.go`）**：
- `evaluatePreflight(ctx, snapshot, target)` 接收 `target domain.CompilerTarget` 参数：
* 当 `target != domain.TargetMihomo` 时，忽略 `snapshot.Diagnostics` 中的 `empty_routed_group`（以及任何针对策略组/规则的阻断项），仅对节点风险（`risk_blocked` / `risk_review` / `risk_unknown`）进行预检拦截；
* 在实时 IP 风险重算阶段，当 `target != domain.TargetMihomo` 时仅收集 `snapshot.Nodes` 的 `LogicalID`，不遍历 `snapshot.Groups`。
```

### Constraint DOC-0299 <a id="hcr-doc-0299"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/design.md:61`
- **Source Digest**: `a61cab822be5453efb60c4a3cf290a025964e405ba239e62efbece7ce32673c8`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### 3. 全站默认 `zh-CN` 与完整中文化设计（`web/src/**`）

- **默认语言与字典架构**：
- `web/src/locales/index.ts`：`getInitialLocale()` 在 `localStorage` 未存储有效值时默认返回 `'zh-CN'`；`createI18n` 的 `fallbackLocale` 设为 `'zh-CN'`。
- 提供清晰的 `web/src/locales/zh-CN.ts` 与 `web/src/locales/en-US.ts`（并由 `messages.ts` 聚合导出，保持 `zhCN`、`enUS`、`Locale` 导出兼容），集中管理所有模块的完整中英文词条。
- **统一术语与枚举中文化映射**：
- 策略组类型：`select` → `手动选择`、`urltest` → `自动测速`、`fallback` → `故障转移`、`loadbalance` → `负载均衡`。
- 准入规则动作：`allow` → `允许`、`reject` → `拒绝`、`quarantine` → `隔离`。
- 筛选字段与操作符：`display_name` → `节点名称`、`protocol` → `协议类型`、`source_subscription_ids` → `来源订阅`、`probe_verdict` → `探针结论`、`probe_latency_ms` → `探针延迟 (ms)`；`contains` → `包含`、`not_contains` → `不包含`、`equals` → `等于`、`not_equals` → `不等于`、`lte` → `小于等于 (≤)`。
- 探针类型与状态：`baseline` → `基础连通`、`geo` → `地理与出口`、`streaming` → `流媒体解锁`、`ai` → `AI 服务`、`speed` → `速度与带宽`、`ip_risk` → `IP 风险评估`；运行状态 `queued` → `排队中`、`running` → `运行中`、`succeeded` → `已成功`、`failed` → `已失败`、`cancelled` → `已取消`、`expired` → `已过期`；探测结论 `available` → `可用`、`restricted` → `受限`、`error` → `异常`、`unknown` → `未知`、`stale` → `已过期`、`missing` → `未探测`。
- 错误卡片与 API 错误码（`ErrorStateCard.vue`）：将鉴权失效、访问受限、状态冲突、网络异常、服务异常、重试与前往设置按钮、以及常见错误码（如 `no_active_revision`、`unsupported_target_capability`、`cycle_detected`、`self_loop_forbidden`、`publication_preflight_rejected` 等）全部映射为清晰的中文说明。
```

### Constraint DOC-0300 <a id="hcr-doc-0300"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/design.md:73`
- **Source Digest**: `a61cab822be5453efb60c4a3cf290a025964e405ba239e62efbece7ce32673c8`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): pure-node export empty-group validation bypass pending Clarification Q4.
- **Original Source Text**:
```text
### Risks / Trade-offs

- **Golden Fixture 与现有测试契约变更**：
- `singbox.golden`、`surge.golden`、`qx.golden`、`quantumult-x.golden` 以及 `compiler_test.go`、`singbox_test.go`、`surge_test.go`、`qx_test.go`、`preflight_test.go`、`feature_convergence_test.go` 原先断言非 Mihomo 目标包含策略组和路由规则，需同步更新为验证“仅 Mihomo 包含策略组与规则，非 Mihomo 目标仅输出纯节点且忽略策略组与规则”。
- **前端单测默认语言切换影响**：
- `web/src/**/*.test.ts` 中部分测试原先假定初始 locale 为 `'en-US'` 或断言英文按钮/校验文本，需在 Task 1.2 中同步更新测试断言以匹配默认 `'zh-CN'` 及中文化文案（并保留对切换 `'en-US'` 的测试覆盖）。
```

### Constraint DOC-0301 <a id="hcr-doc-0301"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/design.md:1`
- **Source Digest**: `fe90310a0b1947e45272ee0cc4a8a8979e9563d6a049a81bb967bb22a43a9afc`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 proposal.md 与 specs/admin-ui-device-quality/spec.md。当前工作区已有 726 项变更，`web/` 未跟踪；已完成的四项隔离基线/服务/测试数据/历史问题复核保持原有勾选。`web/e2e/auth-real-server.spec.ts` 的构建可能覆盖根目录 `csp`，`web/vite.config.ts` 构建后可能覆盖 `internal/webassets/dist`；现网 18080/17000 和数据卷禁止写操作。旧 ADB/USB 门槛和由此派生的真机认定一律废止。交互验收与 `csp-ui-hardcode-governance-v1` 对齐，不设全控件固定尺寸门禁。
```

### Constraint DOC-0302 <a id="hcr-doc-0302"></a>

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

### Constraint DOC-0303 <a id="hcr-doc-0303"></a>

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

### Constraint DOC-0304 <a id="hcr-doc-0304"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/design.md:19`
- **Source Digest**: `fe90310a0b1947e45272ee0cc4a8a8979e9563d6a049a81bb967bb22a43a9afc`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): policy member interleaved ordering pending user decision on Clarification Q2.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [预览 API 与前端共享合同交错] → 先冻结输入/响应与错误语义，再联调和集成复验；禁止两个工作包同时改同一接口或测试文件。
- [726 项已有变更且 `web/` 未跟踪] → 小范围编辑与文件归属核查，禁止清场、重写既有文件或在当前树执行有覆盖副作用的构建。
- [headless 不具备实屏信息] → 报告严格限定结论；未获授权的实屏场景不列为阻断这次 headless 验收的前置门槛，也不谎称已经完成。
- [凭据/订阅信息泄露] → 使用临时数据、证据脱敏，不传真实密钥、不向生产端口写入。
```

### Constraint DOC-0305 <a id="hcr-doc-0305"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/design.md:26`
- **Source Digest**: `fe90310a0b1947e45272ee0cc4a8a8979e9563d6a049a81bb967bb22a43a9afc`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

无数据 schema 迁移或部署。施工完成只交付经独立审查与隔离复验的代码和证据；发布、重启、生产数据更改需另行授权。
```

### Constraint DOC-0306 <a id="hcr-doc-0306"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/design.md:1`
- **Source Digest**: `ddff2ed1b6cfcb1100f1c6d06961ebb102b356964fcee450e96e1d865301fe40`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: fixed 44px touch target replaced by fluid design tokens and responsive CSS rules per FR-UI-02 (no automatic old 44px revived active).
- **Original Source Text**:
```text
### Context

`web/tailwind.config.js` 已启用 DaisyUI 七主题，但 `theme.extend` 为空；`web/src/theme/index.ts` 管主题名/存储而非空间尺寸。原始清单中 `web/src/style.css` 用小屏全局强制 44px 按钮/输入和 24px 复选框，Dock 及响应式卡片另有固定尺寸；当前只读核查显示全局强制规则已被修改，包 A/B 已勾选，但共享 `Drawer.vue`、`ModalDialog.vue`、`ConfirmModal.vue` 仍使用重复 `min(...,calc(...))` 视口表达式，`PolicyEditorSheet.vue`、`ProbeEvidenceSheet.vue` 与 `PublicationsView.vue` 也仍各自写高度公式。已勾选任务不等于合并验收通过。`NodesView.vue` 虚拟列表的动态像素高度是算法输出，不属于呈现硬编码。`openspec/specs/` 为空；旧设备治理 Change 的统一 44px 门禁已在其规划中改为语义交互可达、无遮挡与实测，不改变设备路径或其他任务。
HEAD 基线 `7f827e2`、起始记录 726 项脏条目，`web/src` 是未跟踪目录；既有 `npm test` 17 files/114 tests 成功是继承的报告，规划角色没有重新执行。禁止使用 git 清理/重置覆盖旧文件；无授权生产写入、部署、重启、实屏/ADB/USB。
```

### Constraint DOC-0307 <a id="hcr-doc-0307"></a>

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

### Constraint DOC-0308 <a id="hcr-doc-0308"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/design.md:13`
- **Source Digest**: `ddff2ed1b6cfcb1100f1c6d06961ebb102b356964fcee450e96e1d865301fe40`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: fixed 44px touch target replaced by fluid design tokens and responsive CSS rules per FR-UI-02 (no automatic old 44px revived active).
- **Original Source Text**:
```text
### Decisions

1. **复用现有原语而非再造主题系统。** 主题颜色继续由 DaisyUI `data-theme` 管理；空间优先 Tailwind 既有 scale、流式 flex/grid、`min-w-0`/`max-w-full`、可用视口高度与安全区。只有跨组件重复且有语义的间距/停靠避让才提炼少量共享 CSS 变量或组件接口；不得将散装 44px 替换成单个 44px token 假装问题解决，也不得用更多逐页 `min/calc/vh` 表达式包装旧常数。备选“一次性替换所有字面量”会掩盖不同控件语义，拒绝。
2. **把交互区域同视觉标记分开。** 移除/收敛按断点通杀 DaisyUI 尺寸的全局强制覆盖，由组件按操作优先级使用语义按钮、明确焦点/名称及必要的容器间距；小型复选框可通过关联 label 扩大有效激活范围，不强迫图标本体与容器相等。先量测目标矩形、遮挡和邻近间距，再决定例外。备选“所有控件至少 44px 并添加像素 lint”与用户要求相反。
3. **共享弹层原语契约：布局职责在容器而非各页面。** 优先沿用现有 Drawer/ModalDialog/ConfirmModal 的交互语义和 DaisyUI 对话框能力；若现有 API 不足，由共享容器集中提供可用视口/安全区约束、窄屏及桌面定位、溢出防护、头部与底部操作区稳定可见、正文区域内部滚动、焦点/关闭行为。PolicyEditorSheet 和 ProbeEvidenceSheet 选择同一既有容器接口或可复用的共享弹层结构，而非各自继续拼视口算式；PublicationsView 的预览作为容器内长内容滚动，能扩展填充剩余空间，不能再用固定视口比例限制内容。页面只传内容、动作与确有理由的宽度或密度变体。可用高度/安全区仅在共享边界求值一次；若平台/浏览器差异必须保留局部尺寸，登记意图、唯一作用域和验证场景，不把多组近似数字迁移到新 token。变更抽象以前先检查已完成包 A/B 的实际产物，不能把勾选当作原语已达标。
4. **导航与卡片由可用空间决定。** Dock 以实际导航占用与 safe-area 避让主内容；响应式卡片阈值如需保留，作为局部可解释值或在响应式组件中封装，并验证狭窄容器下无溢出。`NodesView` 动态虚拟列表高度保留。
5. **旧 Change 已完成规划协调，施工仍须核实写集合。** `csp-ui-device-governance-v1` 的 spec/design/tasks 取消统一像素门禁，保留三视口、SSH→Ubuntu chroot ARM64 Chromium Playwright 和隔离服务边界。新旧 Change 的 Policy 编辑、共享弹层及预览路径只能有单写者并按接口先后串行；不能因规划协调就把未完成的业务任务勾选。
6. **实测而不是简单数字门禁。** 定向组件/浏览器测试检查窄屏 375×667/392×872、桌面 1280×800、长文本与缩放、导航末项、弹层与交互控件焦点/可点击性；多个主题抽样。无头验收仅限获授权隔离实例和 SSH→Ubuntu chroot ARM64 Chromium Playwright，记录命令、退出码、视口、构建、模式与脱敏证据，不声称实屏通过。不在当前树运行会覆盖 `csp`/`internal/webassets/dist` 的构建。
```

### Constraint DOC-0309 <a id="hcr-doc-0309"></a>

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

### Constraint DOC-0310 <a id="hcr-doc-0310"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/design.md:29`
- **Source Digest**: `ddff2ed1b6cfcb1100f1c6d06961ebb102b356964fcee450e96e1d865301fe40`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

无数据迁移；先核对旧 Change 已协调的合同，再冻结文件归属，按共享弹层容器先行、页面调用后迁移的依赖序列分批施工，独立复验合并态。若回归失败，仅撤销本任务可追溯的局部编辑，不使用 reset/restore/checkout/clean，也不影响既有脏文件。发布/重启需单独授权。
```

### Constraint DOC-0311 <a id="hcr-doc-0311"></a>

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

### Constraint DOC-0312 <a id="hcr-doc-0312"></a>

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

### Constraint DOC-0313 <a id="hcr-doc-0313"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:20`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Goals / Non-Goals

交付可溯源受控app更新、库存/身份保持、真实鉴权正文与代表性实网证据、可证误下线恢复。不是要求全节点在线；不造生产故障订阅、不新增陌生目标、不动UI/gateway/Hermes内核/模型/Gemini，不假全绿。
```

### Constraint DOC-0314 <a id="hcr-doc-0314"></a>

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

### Constraint DOC-0315 <a id="hcr-doc-0315"></a>

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

### Constraint DOC-0316 <a id="hcr-doc-0316"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:49`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### 3. 自测、独立审查与隔离预算

成熟testing/httptest/t.TempDir/标准断言及既有e2e SQLite，不手造私有框架。完整测试矩阵：合法/错token/无凭据；公开基线失败；200错误JSON/schema；单条断言失败；多页与首100无active；刷新500/超时/误清空及等计数替换ID；未知状态/错误node_ids反例；真实协议succeeded和独立observations、错run/revision、空观察、deadline/cancel；redaction金丝雀；写文件失败；公共tmp预置symlink不触及、并发scratch不冲突、build失败与清理。
本地只loopback httptest/临时SQLite/fake现有probe依赖；测试不能用生产DB、真实订阅/节点/凭据、真实模型或QQ。CLI subprocess fixture清空token/代理/外部通知env，禁止生产出口。命令：`go build ./...`、`go test ./...`、`bash -n scripts/run_authenticated_acceptance.sh`、针对CLI包及现有e2e test可复验命令、OpenSpec strict，全exit0。建议本地完整包预算40分钟，耗时有新证据继续、无进展caller_ping保留成果，不机械重试。无UI源码变化无需新前端门禁/critic；Docker既有web构建仍应成功。reviewer20分钟建议预算，独立重跑Go/build/shell与故障fixture，审核发布/备份恢复步骤及真实合同，不介入施工中间步。
```

### Constraint DOC-0317 <a id="hcr-doc-0317"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:55`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical 10-minute single timer probe sweep superseded by tiered probe scheduler (1m sweep, 5m cooldown, 1h cycle) per FR-PROB-02.
- **Original Source Text**:
```text
### 4. 真实验收预算与标准

本次建议（操作上限，不是协议常量）：至多3个enabled既有订阅、每个一次，按现有来源/协议分布选而非随意前3；每次HTTP上限45秒（已知服务fetch30秒），不得修改订阅配置来扩大timeout；代表性至多6个active既有节点，按来源/协议覆盖、明确baseline，分至多2节点的小批串行，本轮总deadline10分钟。probe现有TTL300秒，deadline与剩余预算一致，不将TTL强改成30秒。公开/正文GET超时10秒，probe创建10秒、确认预算从剩余deadline扣减。安全事件中止不重试生产刷新。
合法auth正文：status authenticated=true、settings开关/verifier配置保持、subscriptions分页/9类计数、nodes全量/active/source、policies/pubs真实shape与计数；已有publication才核验导出解析，无publication不凭空创建，标不适用。API数据变化用fresh实际基线，不硬编码1005或22为未来结果。记录脱敏请求类别/状态码/body schema摘要、数量、ID HMAC/受管摘要、时间/run/observation digest/阶段/延迟，保存0700/0600，不上传原始配置或订阅URL。
成功刷新允许新增合法库存，不应删除/失活既有有效节点；失败刷新完整旧节点/来源/active集合保持。任何丢库、安全限制绕过、身份开关/DB verifier变化立即回滚。外部订阅不可达/节点握手失败如实报告，不等同部署需回滚，但真实成功握手不足不可说“握手成功”；若全失败报告PARTIAL及具体外部层原因。缺合法凭据不发受限请求、保留任务未完成；继续本地闭环不借暂停exit0洗成PASS。
```

### Constraint DOC-0318 <a id="hcr-doc-0318"></a>

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

### Constraint DOC-0319 <a id="hcr-doc-0319"></a>

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

### Constraint DOC-0320 <a id="hcr-doc-0320"></a>

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

### Constraint DOC-0321 <a id="hcr-doc-0321"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:88`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Risks / Trade-offs

凭据外部持有人尚未安全投递；不能从bcrypt恢复明文。镜像缺revision使完整来源一致性需后续构建manifest，而非仅比时间。真实节点可能失败、历史归因可能缺失，必须保留限定结果。备份恢复涉及WAL与并发写，须先演练再放行。CLI当前协议错配可能扩大探测，整改前禁用于production live。
```

### Constraint DOC-0322 <a id="hcr-doc-0322"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/design.md:92`
- **Source Digest**: `88afb745cd41b59add98dea896c0cf4e21b8bf47598507cc4db02230b7bc0b8d`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Migration Plan

本轮更新规划并strict至READY，所有后续业务未执行保持unchecked。后续一个完整executor包自测→独立reviewer→已授权受控发布与证据收集→合法有界真实验收/充分证据恢复→独立终态复验→母体核验官方run与必要授权提交/通知。凭据阻断仅暂停受限目标；本地整改继续，绝不伪造业务完成。
```

### Constraint DOC-0323 <a id="hcr-doc-0323"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/design.md:1`
- **Source Digest**: `41ece28cc73226f359634b1d28a962768b1045b11e1b49f6c284d7d09a8061a6`
- **Classification**: `still-applicable-plan`
- **Witness / Status**: Still-Applicable Plan: architectural invariant active in system design.
- **Original Source Text**:
```text
### Context

参见 `proposal.md`。当前 SQLite 单主库中存量 1014 个节点，其中 31 个属于有效实时订阅成员（Scope E = 31，`active = 1`），983 个处于无源失活状态（`active = 0`）。取证表明，983 个孤儿中 960 个源自 Go 重写初期 `legacy.import` 遗漏 `node_source_links`，23 个源自后续订阅刷新下线剪枝时被物理清除 `node_sources`。
```

### Constraint DOC-0324 <a id="hcr-doc-0324"></a>

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

### Constraint DOC-0325 <a id="hcr-doc-0325"></a>

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

### Constraint DOC-0326 <a id="hcr-doc-0326"></a>

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

### Constraint DOC-0327 <a id="hcr-doc-0327"></a>

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

### Constraint DOC-0328 <a id="hcr-doc-0328"></a>

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

### Constraint DOC-0329 <a id="hcr-doc-0329"></a>

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

### Constraint DOC-0330 <a id="hcr-doc-0330"></a>

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

### Constraint DOC-0331 <a id="hcr-doc-0331"></a>

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

### Constraint DOC-0332 <a id="hcr-doc-0332"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/design.md:1`
- **Source Digest**: `4704bf000b043a2f427c023c0a201cb84cbf6e18cc5fa448951aea78c966a517`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Context

参见 proposal.md。审查的五项不能一概归咎于编译器：`compiler/compiler.go:280-338` 与 `publication/service.go:880-955` 只检查 payload logicalID/protocol/version 与端点形状；`domain.Node`、`resolver.ResolvedNode` 不带 server/port。`domain/id.go` 的 ComputeNodeLogicalID 对 protocol/server/port/非秘密 transport 做归一哈希，排除 uuid/password/key 等敏感字段；`parser/parser.go` 用归一 transport；不能仅凭不完整 payload 反算或以订阅内容摘要（`inventory/service.go` 的 payload.Digest）证明可信端点。已排除“检查非空即安全”和“简单从凭据反算 logicalID 即足够”：凭据可能缺少参与 ID 的非秘密 transport，且版本/别名变化会误拒真实 URL。`http/nodes.go` 只有 GET，`inventory.NodeView` 不含端点；`nodeView.ts` 注入 `.edge.internal`/WG 地址、公钥、UUID，`useNodes.ts` 的 update 只改内存。`mihomo.go:974-1011` 在只有 provider 名称时生成 `ruleset.invalid`。`publication/service.go:955-1036` 全局 map 与前 100 条审计 RedactedSummary 不能提供可信、完整、可重启的绑定；现有 snapshot digest 等同于凭据版本绑定的假说亦被排除。`node_risk.go:560-645` 多 provider join 同时间戳可能倍增，`latestObservationsForNodes:792-835` 一页节点扫描全部历史。不要把现有其他协议能力断言为失败；其原勾选保留，最终回归仍待完成。
```

### Constraint DOC-0333 <a id="hcr-doc-0333"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/design.md:5`
- **Source Digest**: `4704bf000b043a2f427c023c0a201cb84cbf6e18cc5fa448951aea78c966a517`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Goals / Non-Goals

**Goals:** 确定安全绑定与编辑语义，重启可验证不可变发布，规则资源 fail closed，风险列表稳定；本地确定性验证与独立复验。
**Non-Goals:** 不部署、不触生产卷或密钥、不提供向未认证用户展示明文凭据的 API；不从旧审计日志猜测出版绑定或自动补历史信任。
```

### Constraint DOC-0334 <a id="hcr-doc-0334"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/design.md:11`
- **Source Digest**: `4704bf000b043a2f427c023c0a201cb84cbf6e18cc5fa448951aea78c966a517`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### Decisions

1. **身份模型与端点来源（工作包 A，先冻结合同）。** `domain.NodeCredentialPayload` 保留现有 server/port/version，扩展独立的加密认证绑定字段（如 identityBinding：规范化 protocol/server/port、节点 logicalID、凭据 version、非秘密 transport 身份材料的版本化摘要；敏感 transport 不写明文索引）。在 `parser/extract.go` 与 `parser/parser.go` 生成同一 canonical identity，`inventory.Service.ReconcileSubscription` 在单事务内将来源计算的绑定写入受保护 vault AEAD 载荷/凭据记录，并与节点版本一致；若需独立索引，SQLite 只存带密钥 HMAC-SHA256 的版本化绑定而非可自称真实的 payload 摘要。读取时必须以可信的、不可由待验证 payload 单独构造的节点来源绑定/版本和 AEAD AAD 核对 payload 端点；`publication.resolveCredentials` 与 `compiler.validateCredentialEnvelope` 通过冻结的 typed verified credentials 输入共享这一核验，直接不可信 `WithCredentials` 调用不能绕过：要么验证可信节点绑定，要么 fail closed。若纯编译器输入原本支持未持久化快照，需显式携带由可信解析器计算且不可由攻击者同一调用伪造的身份来源；不可用则拒绝。保留原始 normalized transport 身份版本或经过认证的 canonical identity 材料，正确处理 URI/YAML 等价、敏感 transport 排除、大小写/默认值与旧记录无绑定 fail closed；不能从 payload 自证或把源码 fetch digest 当端点证明。`domain/id.go` 的现有逻辑 ID 算法不暗改；如确须身份算法版本化，明确旧版读取约束并在本地 fixtures 覆盖，绝不将所有 URL 节点误判为篡改。篡改测试需显式使用正常 logicalID + version + protocol 与恶意不同 server/port，证明鉴别有效。
2. **真实元信息与编辑（A 冻结 HTTP 合同，前端包 E 后行）。** 管理员鉴权的 `GET /api/v1/nodes/{logical_id}` 响应 `node.connection` 由 vault 中已认证记录投影：server/port、WG localAddress/publicKey/reserved/mtu/dns、TUIC uuid/alpn/sni/congestionControl/udpRelayMode 等可展示字段；WG 公钥、TUIC UUID 虽非私钥，仍限管理员详情；密码、WG privateKey/PSK、token、任意原始 transport 秘密不序列化，只有来自真实 payload 的 `hasPrivateKey/hasPreSharedKey/hasPassword` 布尔值。失败/缺绑定显示 unavailable，不做默认值或猜测。冻结 `PATCH /api/v1/nodes/{logical_id}/connection`，body 带 `expected_credential_version`、可更改非秘密字段及 write-only `private_key_input/pre_shared_key_input/password_input`；空值表示保留既有秘密，明确轮换动作才更改，绝不回显；输入校验/字段白名单、管理员认证、CSRF、无缓存与审计脱敏。logicalID 由端点和非秘密 transport 派生：此类变更不能在旧 ID 原地改造、须拒绝并要求修改源订阅后 Reconcile 生成新 ID；同 ID 仅允许不变身份字段的凭据轮换与安全元信息更新，CAS version + 节点/凭据事务，成功递增版本并使旧 probe/发布重建路径失效。若节点有来源订阅，后续 Reconcile 是源的权威值，覆盖本地轮换并重新 version（或冲突拒绝并审计）；不宣传持久覆盖，详情显示来源与下次同步影响。服务内存更新禁止被称为保存；失败保持旧 UI 状态。编辑接口的 token/CSRF 复用现有 chi router 中间件，不私造认证。
3. **发布持久绑定（B）。** 在 `domain.Publication` 与 `domain.PublicationRepository` 增加不可变 `content_digest`、`credential_binding_digest`、版本化绑定与工件元数据；SQLite 新 migration 仅对隔离临时 DB 运行验证，增加出版元数据/密文工件表或列。`Publish` 在同一事务原子写 publication、token hash、绑定、按 vault AES-GCM 加密的内容和 content type/filename；若现有 repository 不支持事务，扩展事务端口避免先写可用 token 后存工件失败。凭据绑定使用有分隔/规范编码（Go encoding/json 标准编码结构，禁字符串拼接歧义），覆盖排序 logicalID、version、protocol、可信端点/身份摘要及编译版本；内容 digest SHA-256 对实际输出字节，服务读密文解密并先校验 token/撤销/目标/绑定与字节摘要，再返回；不调用审核日志作为数据库，不依赖进程 map。可验证重编仅在持久化完整绑定、固定 compiler version 与相同内容 digest 全部成立时允许；默认密文不可变工件消除活订阅漂移。旧出版行缺摘要/工件不回填审计文本，安全拒绝且不自动改写历史数据；风险/认证门禁优先。迁移用 `schema_migrations` 的现有 runner、事务回滚/升级测试验证旧行和新行、重启读密钥错误与篡改拒绝；生产数据迁移/备份/回滚待另行授权。
4. **规则资源（C）。** `compiler/mihomo.go` 的 RULE-SET 必须引用调用方配置的合法 provider 或显式 HTTPS URL，经现有 URL parser 和 provider identifier 校验、定义与引用双向校验；单纯 `RULE-SET,name,GROUP` 无已定义 provider 时位置化 capability error，不注入 `.invalid` 或任何猜测 URL。网络抓取非编译时默认动作；本地 fixtures 使用受控 HTTPS/已声明 provider，不为绕过验证引入公网请求；官方 mihomo 配置检查与定向负例校验。
5. **风险单行最新观测（D）。** 在 `internal/repository/sqlite/node_risk.go` 用 SQLite `ROW_NUMBER() OVER (PARTITION BY node_logical_id ORDER BY observed_at DESC,id DESC)` 的白名单 provider 集（`EXISTS` 而非多对多 join），`rn=1` 才进入 `node_eval`，count 与 page 使用相同 CTE 和过滤/排序语义；`latestObservationsForNodes` 以分页 ID + 允许 provider 为约束的同一排名查询，仅返回每 ID 最新一条。针对 `ip_risk_observations(node_logical_id,observed_at DESC,id DESC)` 与 provider/schema 过滤建立符合 EXPLAIN QUERY PLAN 的索引，先分页节点再取观测，避免读全历史；多 provider 同时间戳/同 ID tie break、失效/未知、过滤 count/page 与万条历史隔离 fixture 对照。索引 migration 只在临时 DB 验证，不碰生产。
6. **DAG 与验收。** 先由 A 冻结身份和 GET/PATCH JSON、可信凭据 typed 接口，不允许 A/B 同时写 publication 或 compiler：A 写 domain identity、parser、resolver、inventory、HTTP nodes、A 专属测试；B 写 publication service、domain publication/repository port、SQLite publication repo/migration、B 专属测试；C 写 compiler/mihomo.go 及专属测试（若需改共享 compiler.go，先由 A 冻结接口并交接后串行）；D 写 sqlite/node_risk.go、风险专属 migration/测试（与 B 的 migration 版本号、共享 migrate.go 需预先协调，绝不并发写同文件）。E 仅在 A 的详情 API 合同冻结后写 `web/src/features/nodes/**`，不与 A 写同一路径。各施工包代码+定向自测+`go build ./...` exit 0；全分支汇聚再执行全量 Go race/count/build、Vitest/vue-tsc、隔离 web 构建、官方 CLI fixture、httptest 与独立 reviewer。reviewer/critic 阻断允许最多一次定点返工复验；若仍失败，回到本 Change 根因级整改，未真正过的任务维持未勾，不能假结单。
```

### Constraint DOC-0335 <a id="hcr-doc-0335"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/design.md:25`
- **Source Digest**: `4704bf000b043a2f427c023c0a201cb84cbf6e18cc5fa448951aea78c966a517`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: historical AES-GCM credentials vault superseded by plaintext direct storage in nodes table per FR-AUTH-03 / FR-INV-06.
- **Original Source Text**:
```text
### Risks / Trade-offs

- [凭据自身自证身份、逻辑 ID transport 不完整] → 信任锚来自解析/节点身份版本，不由待验 payload 构造；双格式合法 URL 与同 ID 换端点负例同时锁定。
- [订阅刷新覆盖本地轮换] → API 明示来源权威与覆盖语义、记录审计；身份变化只能通过源 Reconcile。
- [发布工件含明文秘密] → vault AEAD 加密存储、独立密钥/权限、无日志、token 门禁与摘要校验；无密钥拒绝。
- [迁移共享版本或生产数据意外更动] → 迁移编号预留、只用本地临时 DB 演练；生产升级另需授权。
- [风险全局排名 CTE 仍扫描历史] → 分页先行、索引与 EXPLAIN/有界 fixture 作为验收，不以仅结果正确放行。
```

### Constraint DOC-0336 <a id="hcr-doc-0336"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/design.md:33`
- **Source Digest**: `4704bf000b043a2f427c023c0a201cb84cbf6e18cc5fa448951aea78c966a517`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: one-off clean-slate migration scripts superseded by living spec baseline and repo-local migrations.
- **Original Source Text**:
```text
### Migration Plan

本次 Change 仅在临时数据库验证 schema 升级/回滚方案、历史行只读和缺绑定拒绝；不操作生产库。将来部署前另行审批备份、密钥可用性与灰度回退；旧记录不自动假恢复。
```

### Constraint DOC-0337 <a id="hcr-doc-0337"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:1`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### Design: redesign-csp-database-dataflow

- **Change**: `redesign-csp-database-dataflow`
- **Work Package**: `pkg_database_dataflow_design`
- **Lineage**: `fleet-contract-v3 / redesign-csp-database-dataflow`
- **Architectural Scope**: CSP 数据库架构重构、上下游契约厘清与事实层补齐 (Rev 3 - Remediated)
---
```

### Constraint DOC-0338 <a id="hcr-doc-0338"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:10`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 1. 架构总原则与单库运行定式 (Architecture Baseline)

系统严格维持单一主 SQLite 数据库（`/data/csp-v1.db`），开启 WAL 日志模式、普通同步模式（`synchronous = NORMAL`）与内置外键检查（`PRAGMA foreign_keys = ON`）。单进程内通过单写者连接池与 5000ms 忙等待保证事务串行化，坚决不进行多库分拆或引入外部中间件。
存量的 27 张数据表（1 张技术表 `schema_migrations` 与 26 张业务表，包含 7 张 IP Risk 相关表）全量保全，涵盖 1014 个节点（31 活跃、983 历史失活禁用）、9 个订阅与所有策略拓扑规则。所有 Schema 变更统一归口于 `MigrationRunner`，彻底清除 Repository 构造函数中的硬编码运行时 DDL。
停写维护窗口大小由隔离演练实测确定并由用户确认，清除固定 30 秒与 Zero-Loss 的不实承诺。提供原资产对账与回滚增量合并或操作员确认 Loss 窗口的可执行步骤。
---
```

### Constraint DOC-0339 <a id="hcr-doc-0339"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:20`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 2. 目标数据模型与增量事实层 (Target ER Schema)

针对 SQLite 不支持 `ALTER TABLE ADD` 复合外键且 `nodes` 与 `versions` 容易产生双向循环外键的问题，系统确立单向解耦模型：**引入核心事实表 `node_connection_heads` 与 `publication_payload_refs`**。
- `nodes` 表保持稳定资产标识，**不添加表级外键反向引用 versions**（避免循环依赖，无需重建 `nodes` 表）；
- `node_connection_versions` 单向外键引用 `nodes(logical_id)`；
- `node_connection_heads` 单向外键引用 `nodes(logical_id)` 并通过复合外键单向引用 `node_connection_versions(node_logical_id, connection_revision)`；
- Canonical current revision pointer 唯一存在于 `node_connection_heads.connection_revision`；
- 旧 `nodes.connection_revision` 字段在过渡期仅作为只读投影视图，完成切换后严禁多事实源双写；
- 引入 `publication_payload_refs` 关联表（`publication_id`, `payload_id`），外键 `RESTRICT` 物理约束被发布引用的 Payload 不被误删。
```
+--------------------------+       1:1       +--------------------------+
|  subscriptions           |---------------->|  subscription_payloads   | (Authoritative Raw Upstream)
+--------------------------+                 +--------------------------+
|                                             | 1:N
| 1:N                                         v
+--------------------------+                 +--------------------------+
|  subscription_fetches    |                 |  subscription_entries    | (Derived Entries + Notice Filter)
+--------------------------+                 +--------------------------+
|                                             |
+----------------------+----------------------+
| 1:N
v
+--------------------------+
|      node_sources        |
+--------------------------+
| N:1
v
+--------------------------+  1:N   +--------------------------+   1:1   +--------------------------+
| node_connection_versions |<-------|         nodes            |-------->|  node_connection_heads   | (Canonical Current Pointer)
+--------------------------+        +--------------------------+         +--------------------------+
^                                     | 1:N                              |
|                                     v                                  | Composite FK:
| 1:N                          +--------------------------+              | (logical_id, rev)
|                              |      node_overrides      |              +----------------------+
+--------------------------+                +--------------------------+                                     |
| publication_payload_refs |                               | (Resolved into Effective Config)                v
+--------------------------+                               +-------------------------------------------------+
| N:1
v
+--------------------------+
|      publications        | (Immutable Draft / Published Snapshots)
+--------------------------+
```
```

### Constraint DOC-0340 <a id="hcr-doc-0340"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:65`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 核心新增与扩展表规范

1. **`subscription_payloads`**: 存储完整原始上游响应（BLOB，支持 gzip），排除 Cookie/Auth，内部仅供重放和对账，不向外网暴露明文凭据；
2. **`subscription_entries`**: 存储基于 parser_version 派生的结构化条目。
- `id` 为每 Payload 独立 PK，主键加 `UNIQUE(payload_id, ordinal)`；
- 3 个同参数公告独立写入不去重，不把连接指纹当条目身份；
- `source_key` 仅作为跨刷新匹配候选锚点，**严禁 UNIQUE(subscription_id, source_key) 折叠条目**；
- 内置严格来源门禁与组合全条件公告分类器：遵循 `verified_source_rule(subscription_id, source_evidence, rule_version) AND exact_rule_match(entry) => notice`，仅对已验证来源（如 Dogegg）启用规则，未验证来源即使满足回环+端口1+全零UUID+提示词四条件亦保留为 `unknown` 候选并支持纠偏；正常可解析代理始终为 `proxy`；
- 支持条目级用户人工纠偏（`user_kind_override`、`override_anchor`、原因、时间戳、`actor_ref`），即使无对应 node 亦可修正；
3. **`node_connection_versions`**: 记录不可变的节点有效配置 `effective_config_json` 与指纹；
4. **`node_connection_heads`**: 记录节点当前生效的连接版本，单向外键约束指向版本表，杜绝悬挂指针与循环外键；
5. **`node_overrides`**: 记录用户在 Web 控制台的字段级人工修改，刷新时与源条目参数明确合并，保证人工配置不被抓取覆盖；
6. **`publication_payload_refs`**: 发布快照引用 Payload 的关联表，`ON DELETE RESTRICT` 约束 Payload 物理防删。
---
```

### Constraint DOC-0341 <a id="hcr-doc-0341"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:80`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 3. IP Risk Nullable 1:N 统一测量与发布两阶段生命周期

1. **IP Risk Nullable 1:N 统一测量模型**:
- `probe_observations` 中严格仅 `kind = 'ip_risk'` 的通用观测记录与 `ip_risk_observations` (第三方情报商详细评分) 构成 1:N 关系，绝非与 Baseline 测活混用或一对多；
- 跨表 kind 约束采用应用层事务校验结合标准外键 `probe_observation_id REFERENCES probe_observations(id) ON DELETE SET NULL`，关联非 ip_risk 类型直接阻断回滚，不自造触发器平台；
- 历史独立服务商风险查询允许 `probe_observation_id = NULL`，不凭空构造虚假 `probe_runs`，多 provider 数据保全不丢失；
- Baseline 连通性探测与 IP 风险情报解耦；历史风险记录仅在连接版本明确已知时如实回填；
- 真实利用 `000002_ip_risk_schema.sql` 中的 `risk_policy_revisions` 增加 `rules_digest TEXT NOT NULL DEFAULT ''`，发布清单中固化 `risk_policy_snapshot`。
2. **Safe Detail Allowlist 白名单机制**:
- `probe_observations.safe_detail_json` 仅允许收集 `stage`, `code`, `target_core_version`, `protocol`, `transport`, `http_status`, `timeout_ms`, `reason`, `server_redacted`；
- 严禁捕获原生 `err.Error()`、完整 HTTP 响应体及任何密码、UUID、Token、私钥等凭据。历史遗留缺失 detail 保留为 `unknown`。
3. **Publication 清单与两阶段生命周期**:
- Preview 阶段创建不可变 draft 记录并计算完整清单（Manifest JSON，含各节点 included/excluded 理由、payload_ids、version_refs、产物哈希），关联 `publication_payload_refs`；
- Publish 阶段显式接收 `snapshot_id` 并原子发布已编译字节，绝不重新读取当前动态节点库，消灭时间差库存漂移；
- 严格模式完整报错，显式兼容模式记录清单后安全过滤，空组拒绝自动回退至 DIRECT。
4. **协议矩阵与前端错误治理**:
- 探针出站拨号与发布编译统一消费 `node_connection_versions.effective_config_json`；
- Mihomo (v1.19.32) 目标仅渲染官方支持的 `path`, `host`, `mode`, `headers` 为 `xhttp-opts`，未知 extra 保留在内部扩展并不盲传；sing-box 目标严格返回 `unsupported_target_capability`；
- 修复 `web/src/ui/ErrorStateCard.vue`，杜绝将 422 能力错误误报为网络失联。
---
```

### Constraint DOC-0342 <a id="hcr-doc-0342"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:102`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 4. 迁移演练、回滚与测试验证 (Migration & Testing)

- **迁移演练**: 在隔离环境中演练 Migration 000015，实测停写耗时并获用户确认；执行 SQLite 在线一致性热备后运行迁移，事务性初始化 1014 个节点的版本与 head 指针，保全历史失活资产。
- **受控回滚**: 准备热备文件与旧镜像，若异常发生可在短窗内原子恢复；切换后产生新数据则导出增量由操作员确认补偿。
- **测试工程方案**: 扩充至 14 个独立验证场景（详见 `docs/design/csp-database-dataflow.md` 第七章），全流程基于项目现有测试工具验证，杜绝私造框架。
---
```

### Constraint DOC-0343 <a id="hcr-doc-0343"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:112`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 5.1 数据集合边界与视图语义定义

1. **全库节点资产 (Total Asset Ledger)**: `nodes` 表全量记录（1014 个节点），包含 983 个失活历史节点与 31 个当前活跃节点。全量资产用于审计、不可变版本链追溯与合法单节点管理编辑。
2. **当前生效订阅库存范围 E (Enabled Subscription Inventory Scope E)**: 满足以下全部条件的节点子集（当前生产对账为 31 个）：
- 存在关联记录 `node_sources ns JOIN subscriptions s ON ns.subscription_id = s.id`，且 `s.enabled = 1`；
- `ns.last_seen_fetch_id` 等于该订阅最新一次成功/部分成功（`outcome IN ('success', 'partial')`）的抓取记录 ID（失败抓取保持上次成功状态）；
- 条目类型非确认公告（`subscription_entries` 中无未覆盖为 proxy 的 notice 记录）；
- **`nodes.active` 独立性**：`active` 绝不作为范围 E 的前置条件；`active_only`、搜索、协议、风险等过滤条件为在范围 E 之上叠加的附加过滤器。
3. **多源去重与各订阅计数**:
- 全局主视图与首页概览统计均基于 `logical_id` 全局去重；
- 单订阅详情/卡片展示该订阅私有的匹配条目数（Per-Sub）：`node_count`（启用时的去重非公告有效成员贡献，停用时为 0）与 `source_node_count`（最后成功成员数，停用时保留说明历史成员）；
- 多源共享节点时，各订阅累加计数可大于全局去重总数，系统真实呈现该拓扑事实，不进行暴力强扣。
4. **单节点详情与覆盖权限保全**:
- `GET /api/v1/nodes/{logical_id}` 与 `PATCH /api/v1/nodes/{logical_id}/connection` 对所有合法的 `logical_id` 资产开放，允许管理员查看与覆写历史节点参数，不受列表主视图范围收敛影响。
```

### Constraint DOC-0344 <a id="hcr-doc-0344"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/design.md:126`
- **Source Digest**: `7626afa05896cc5260f4d86f6892e6790d2a5ff157b9d3cb950a46d5c6725eb7`
- **Classification**: `historical superseded`
- **Witness / Status**: Superseded: legacy database migration intermediate schema proposals superseded by final unified schema.
- **Original Source Text**:
```text
### 5.2 最小 API 冻结规范与查询语义

- **`GET /api/v1/nodes` 扩展参数**:
- `scope` (可选 string): `enabled_subscriptions` (默认，仅返回当前启用订阅的生效可用节点，当前共 31 条) 或 `all_assets` (返回包含历史失活节点的全库台账，当前共 1014 条)；未知 scope 返回 HTTP 400 `invalid_node_scope`；
- 兼容参数: `active_only=true`、`search`、`protocol`、`risk_*`、`subscription_id` 保持一致并在范围 E 之上叠加。
- **`GET /api/v1/subscriptions` 扩展属性**:
- `node_count` (int): 本源当前启用且有效成员去重非公告贡献（禁用为 0）；
- `source_node_count` (int): 本源最后成功成员去重非公告数（禁用时保留用于说明历史规模）；
- `counts_scope` (string): 固定返回 `'enabled_subscriptions'`。
- **`GET /api/v1/probes/pool` 扩展属性与候选门禁**:
- `inventory_total` (int): 范围 E 节点总数；
- `candidate_total` (int): 范围 E 中满足 `active = 1` 的探测候选总数；
- `total_count` (int): 保持旧字段含义作为候选计数（与 `candidate_total` 一致）；
- `scope` (string): 固定返回 `'enabled_subscriptions'`；
- 任务派发（selected IDs、run all、periodic）严格施加 `E + active=1 + exclude_notices` 门禁。
- **仓储层与服务层统一谓词实现方案**:
- `NodeFilter` 中增加 `Scope: NodeScope` 与 `SubscriptionID: string`；
- `nodeRepository.List` 与 `nodeRepository.ListReadModel` 以及对应的 `Count` 查询共享统一底层谓词构建器 `buildNodeFilterPredicates`，彻底消除条件不一致与 substring SQL 隐患。
```

### Constraint DOC-0345 <a id="hcr-doc-0345"></a>

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

### Constraint DOC-0346 <a id="hcr-doc-0346"></a>

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

### Constraint DOC-0347 <a id="hcr-doc-0347"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:27`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): probe subcheck deduplication rules pending Clarification Q5.
- **Original Source Text**:
```text
### 1. 采用进程内 Mihomo 替代探针专用 Singbox 内存运行时

- **决策**：使用 `adapter.ParseProxy(mapping)` 直接在 Go 进程中解析节点并获得 `constant.Proxy` 实例。
- **替代方案考虑**：
- *继续修补 Singbox 内存实例*：Singbox 对 Hysteria2 端口跳跃、TUIC 部分混淆选项存在复杂配置抽象，且原实现中包含过度安全门禁，修补成本高且难以与 subcheck 生态保持一致；
- *拉起外部子进程*：生成配置文件并拉起外部进程开销过大，容易造成进程风暴与 I/O 抖动。
- **结论**：进程内 `adapter.ParseProxy` 具备高性能、轻量级、连接复用友好等显著优势，为业界主流实现。
```

### Constraint DOC-0348 <a id="hcr-doc-0348"></a>

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

### Constraint DOC-0349 <a id="hcr-doc-0349"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/design.md:41`
- **Source Digest**: `af0ace8321965031a4b48ea789a3ca7aaac0ebc313a7dc079bfcef8b4bc4f35a`
- **Classification**: `undetermined`
- **Witness / Status**: Undetermined (Pending Review): IP risk probe integration and external API rate limit policy pending Clarification Q3.
- **Original Source Text**:
```text
### 3. 多平台检测算法与数据契约（冻结共享API）

- **OpenAI**：双轨探测（Cookies 网页端 + iOS 客户端网关），全通判定 `Full (GPT⁺)`（`sub_tier: 'full'`），单通判定：Cookies 通为 `Web (GPT)`（`sub_tier: 'web'`），iOS 通为 `App`（`sub_tier: 'app'`，不硬归 web）；两项均不通判受限 `restricted`（`sub_tier: 'banned'`）；提取 Trace 国家码写入 `region`。
- **Netflix**：优先 Fast.com CDN 测速 API（403 判封禁 `sub_tier: 'banned'`，有效地区判全解锁 `sub_tier: 'full'` 提取 `region`）；回退至双 Title（81280792 与 70143836）状态断言（区分全解锁 `full`、仅自制剧 `originals`、明确封禁 `banned`）。
- **YouTube**：请求 `https://www.youtube.com/premium`，提取 `INNERTUBE_CONTEXT_GL` 区域标签；命中 `www.google.cn` 判定送中 `CN`；命中未开通文案判定不支持；包含正向会员或广告关键词判定解锁 `unlocked`，提取区域码。
- **Disney+**：通过 BAMGrid 认证断言接口判断：已解锁 `unlocked`、即将上线 `soon`（符合上游未开放语义，判定受限 `restricted`，绝不虚报为已解锁）、封禁 `banned`。
- **Claude**：请求 `https://claude.ai/cdn-cgi/trace`，正则提取 CDN 边缘国家码启发式并核对受制裁国家黑名单（AF, BY, CN, CU, HK, IR, KP, MO, RU, SY），若受限判定 `restricted`。
- **Gemini**：请求真实页面提取 JSON/HTML 中的三字国家码（非 trace），判定解锁或封禁。
- **IP 风险**：请求 Scamalytics 提取 IP Fraud Risk 评分，填入 `risk_score`。
- **测速**：`CheckSpeed` 返回带宽（float64 KB/s = 1024 bytes/s），记录为 `throughput`。
- **能力契约映射**：
- `ProbeKind`: `baseline`, `geo`, `streaming`, `ai`, `speed`, `ip_risk`
- `ProbeVerdict`: `available`, `restricted`, `unknown`, `error`, `stale`
- `CapabilityStatus`（即 `NodeCapabilityView`）：保留原有字段 `verdict`、`latency_ms`、`observed_at`、`summary`、`stale`，新增可选：
- `region?: string`
- `sub_tier?: 'full' | 'web' | 'app' | 'originals' | 'banned' | 'unlocked' | 'soon'`
- `throughput?: number` (KB/s = 1024 bytes/s)
- `risk_score?: string`
- `platforms?: Record<string, PlatformCapability>`（用于 `streaming` 与 `ai`，平台键至少包含 `openai`, `claude`, `netflix`, `youtube`, `disney`，支持 `gemini`）
- `PlatformCapability`: `{verdict, latency_ms?: number, observed_at?: RFC3339, summary?: string, region?: string, sub_tier?: 'full'|'web'|'app'|'originals'|'banned'|'unlocked'|'soon', throughput?: number, risk_score?: string, reason?: string}`
- 组级 `verdict` 汇聚规则：任一实际可用为 `available`，皆明确受限为 `restricted`，全部错误为 `error`，否则 `unknown`。细分以 `platforms` 为准，无嵌套 platforms。未知地区省略，禁止为 AI 伪造 geo 地区，latency 缺测为 -1。
- 数据持久化：`probe_observations` 扩展最小必要持久化字段（`evidence_data` JSON 结构），贯通 `/nodes` 与 `/node` 详情以及探针运行记录全链路。
```

### Constraint DOC-0350 <a id="hcr-doc-0350"></a>

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

### Constraint DOC-0351 <a id="hcr-doc-0351"></a>

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

### Constraint DOC-0352 <a id="hcr-doc-0352"></a>

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

### Constraint DOC-0353 <a id="hcr-doc-0353"></a>

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

### Constraint DOC-0354 <a id="hcr-doc-0354"></a>

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

## 3. 历史任务履约登记 (Historical Tasks - TSK-0355 ~ TSK-0806)

### Task TSK-0355 <a id="hcr-tsk-0355"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:3`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 在 `internal/application/revision/service.go` 与 `internal/application/policy/service.go` 中实现初始修订引导与自动归属逻辑，在 `cmd/csp/main.go` 启动时调用 `EnsureActiveRevision`，并在 `internal/transport/http/revisions.go` 暴露 `POST /api/v1/revisions`。通过单元测试 `go test ./internal/transport/http -run TestRevisions` 与 `go test ./internal/application/policy -run TestPolicy` 验证无活跃修订时添加规则成功返回 201 且不报 422
```

### Task TSK-0356 <a id="hcr-tsk-0356"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:4`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 在 `internal/domain/ports.go`、`internal/repository/sqlite/policy.go`、`internal/application/policy/service.go` 与 `internal/transport/http/policies.go` 中实现准入规则与分流规则的删除能力（`DELETE /api/v1/policies/rules/{id}`）。通过 `go test ./internal/transport/http -run TestPolicyRulesDelete` 验证成功删除返回 204 且后续列表不再包含该规则，不存在规则返回 404
```

### Task TSK-0357 <a id="hcr-tsk-0357"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:5`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 在 `internal/transport/http/router.go` 中为遗留路由 `GET /yaml` 与 `GET /script` 注册显式 410 Gone 处理函数。通过 `go test ./internal/transport/http -run TestLegacyGoneEndpoints` 与 curl 验证响应状态码为 410 且不降级为 SPA 200
```

### Task TSK-0358 <a id="hcr-tsk-0358"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:9`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 重构 `internal/probe/singbox/runtime.go` 中 `safeRuntimeConn` 的 `Close()` 逻辑，在互斥锁前遍历底层套接字并设置立即读写截止时间，确保阻塞读取状态下并发关闭立即返回。通过新增并发单元测试 `go test -race ./internal/probe/singbox -run TestSafeRuntimeConn_ConcurrentReadClose` 验证 100 次循环零死锁
```

### Task TSK-0359 <a id="hcr-tsk-0359"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:10`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 在 `internal/application/probe/safe_dialer.go` 中解耦错误类型，对 DNS 解析失败与私网/SSRF 拦截返回非致命的 `ErrTargetUnresolvable` 与 `ErrPrivateTargetRejected`，在 `runner.go` 中将其作为单节点 `VerdictUnavailable` 处理而不设置全局 `hasFatalDialErr`。通过 `go test ./internal/application/probe -run TestSafeNodeDialer` 与 `TestRunner_TargetResolutionFailureDoesNotFailRun` 验证单节点 DNS 失败时其余节点正常完成且批次成功
```

### Task TSK-0360 <a id="hcr-tsk-0360"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:11`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 修复 `internal/repository/sqlite/probes.go` 与 `internal/transport/http/probes.go` 中的 `nodeObservations` 分页逻辑，采用 SQL 真实 `LIMIT ? OFFSET ?` 并使用 `COUNT(*)` 计算真实 `total`。通过 `go test ./internal/transport/http -run TestNodeObservationsPagination` 验证多页数据时 `total` 与实际记录数一致
```

### Task TSK-0361 <a id="hcr-tsk-0361"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:15`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 在 `internal/repository/sqlite/subscriptions.go` 的 `Delete` 事务中补充孤儿节点原子去活更新（`UPDATE nodes SET active = 0 WHERE active = 1 AND NOT EXISTS (SELECT 1 FROM node_sources ...)`）。通过 `go test ./internal/repository/sqlite -run TestSubscriptionDeleteDeactivatesOrphanNodes` 验证删除订阅后独占节点变为非活跃（active=0）
```

### Task TSK-0362 <a id="hcr-tsk-0362"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:16`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 修复 `internal/transport/http/subscriptions.go` 中 PATCH 订阅的空白引用传递逻辑，使空白字符串正常送达业务层校验。通过 `go test ./internal/transport/http -run TestSubscriptionPatch_BlankSecretRefRejected` 验证提交纯空格引用时返回 422 `invalid_source_url_secret_ref`
```

### Task TSK-0363 <a id="hcr-tsk-0363"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:20`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 在 `web/src/api/client.ts` 与 `web/src/features/auth/useAuth.ts` 中实现 CSRF 令牌管理，在用户登录后提取并在 POST/PUT/PATCH/DELETE 请求中自动注入 `X-CSRF-Token` 请求头。通过前端测试验证在 Cookie 模式下发起变更请求附带有效 CSRF 头
```

### Task TSK-0364 <a id="hcr-tsk-0364"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:21`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 在 `web/src/features/subscriptions/useSubscriptions.ts` 及通用工具中增加局域网纯 HTTP 环境下的 UUID v4 安全退避实现。通过前端单元测试断言当 `window.crypto.randomUUID` 为空时仍能生成合规 UUID 格式字符串
```

### Task TSK-0365 <a id="hcr-tsk-0365"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:22`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 优化 `web/src/features/probes/ProbesView.vue` 与 `useProbes.ts` 的心跳轮询机制，移除每 2 秒遍历 20 页全量节点的循环，仅在任务或节点池变化时按需同步状态。通过模拟定时器验证心跳期间不会频繁调用 `/api/v1/nodes` 深度分页
```

### Task TSK-0366 <a id="hcr-tsk-0366"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:23`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.4 在 `web/src/features/nodes/NodesView.vue` 中实现视口无限滚动防饥饿机制，当客户端筛选后结果不足填满视口且 `hasMore` 为真时自动加载后续分页。通过单元测试与组件测试验证筛选状态下自动加载下一页
```

### Task TSK-0367 <a id="hcr-tsk-0367"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:24`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.5 修复 `web/src/features/subscriptions/SubscriptionConfigDrawer.vue` 的保存校验拦截，在非基础 Tab 或点击底部保存时强制验证必填字段。通过测试验证空白名称或空白地址无法触发 `save` 派发
```

### Task TSK-0368 <a id="hcr-tsk-0368"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:25`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.6 在 `web/src/features/policy/usePolicy.ts` 中对所有策略组及连接边的 REST 接口动态 ID 参数应用 `encodeURIComponent`，并补充删除规则的前端 Composable 方法
```

### Task TSK-0369 <a id="hcr-tsk-0369"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:26`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.7 在 `web/src/features/publications/usePublications.ts` 与 `PublicationsView.vue` 中引入客户端发布令牌持久化恢复逻辑，确保刷新页面后“复制订阅链接”依然携带有效 `token`。通过单元测试验证刷新状态下导出的完整 URL 包含令牌
```

### Task TSK-0370 <a id="hcr-tsk-0370"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:30`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 更新根目录 `.env.example`，清理所有 Python 时代的 `CLASH_*` 遗留变量，全面对齐 `docker-compose.example.yml` 与 Go 后端 `CSP_*` 规范（包括 `CSP_ADDR`、`CSP_ADMIN_TOKEN`、`CSP_PORT`、`CSP_FETCH_PROXY` 等）
```

### Task TSK-0371 <a id="hcr-tsk-0371"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:31`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 在 `scripts/smoke_test.sh` 及相关测试运行规程中明确 17000 端口为生产映射别名契约，确认所有冒烟与集成测试严格绑定动态临时端口（`0`）和隔离临时数据库，绝不向 17000/18080 发起请求。运行 `bash scripts/smoke_test.sh` 验证 6 项测试全部通过（退出码 0）
```

### Task TSK-0372 <a id="hcr-tsk-0372"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:32`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 修复 `web/e2e/auth-real-server.spec.ts` 中的多语言冲突，在启动测试时显式注入 `en-US` 语言或断言本地化标识符，杜绝默认中文环境下的字符串断言失败。在隔离环境执行 `npm run type-check` 验证前端代码类型检查通过
```

### Task TSK-0373 <a id="hcr-tsk-0373"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:36`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.1 修复发布异步响应跨目标写入：仅当前选中目标可更新可见发布状态、预览、选择与错误；延迟的 Mihomo 发布在切换 sing-box 后不得反选 Mihomo，成功结果仍按 Mihomo 目标存储以供切回恢复。通过延迟响应与切回的 Vitest 回归断言；继续保证刷新后令牌链接可复制。
```

### Task TSK-0374 <a id="hcr-tsk-0374"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:37`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.2 在 `policy.Service` 的首次引导与显式创建活跃修订共用 SQLite 仓储原子创建+激活路径：若激活失败，事务回滚不留下引导草稿或假活跃版本；下一次启动可安全重试，不激活用户既有草稿或丢失既有规则；数据库查询与激活错误均向调用方传播。注入一次激活失败后重新构造服务并启动验证恢复，同时覆盖用户草稿和规则数据保留。
```

### Task TSK-0375 <a id="hcr-tsk-0375"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:38`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.3 `policy.Service.CreateRevision(StateActive)` 在激活失败时返回错误且绝不记录成功审计，使用 6.2 的原子路径避免遗留草稿；补充注入失败与数据不变的测试，保持 `revision.Service` 原有显式生命周期不变。
```

### Task TSK-0376 <a id="hcr-tsk-0376"></a>

- **Source**: `openspec/changes/csp-audit-and-defect-remediation-v1/tasks.md:39`
- **Source Digest**: `09bf15f390afcb5581e54282e4e2d23c7009e9af67f47e741111d82372ab3a4d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.4 所有整改完成后运行 `go build ./...`、`go test ./...`、前端 type-check/test/build、隔离 smoke/E2E、`openspec validate csp-audit-and-defect-remediation-v1 --strict`、`git diff --check`；仅全部退出码为 0 时勾选本节及 1.1/4.7。
```

### Task TSK-0377 <a id="hcr-tsk-0377"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:3`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 [Executor: Dual-Mode Auth Middleware] 修改 `internal/transport/http/middleware_auth.go`，在 `cfg.AdminToken == ""` 时自动以 `open_mode` 放行管理请求并赋予管理员上下文，在 `cfg.AdminToken != ""` 时严格执行 Bearer 令牌与 Cookie 校验；无论何种模态均对发布导出令牌保持 403 `invalid_token_scope` 强拦截；以 Go 单元测试覆盖空配置放行、非空匹配放行、非空不匹配 401 拦截及发布令牌 403 阻断；验证命令 `go test -v ./internal/transport/http -run "TestAdminAuthMiddleware.*"`
```

### Task TSK-0378 <a id="hcr-tsk-0378"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:4`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 [Executor: Public Auth Mode Endpoint] 在 `internal/transport/http/router.go` 新增免认证端点 `GET /api/v1/auth/mode`，直接返回当前系统运行模态（`{"data":{"mode":"open"}}` 或 `{"data":{"mode":"token"}}`）；以 Go HTTP 路由测试验证该接口在匿名状态下正常返回且不产生认证失败审计；验证命令 `go test -v ./internal/transport/http -run "TestAuthModeEndpoint.*"`
```

### Task TSK-0379 <a id="hcr-tsk-0379"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:5`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 [Reviewer: Backend Auth Boundary Gate] 独立审查 1.1 与 1.2 的实现 diff，复跑所有 Go transport 单元测试与端到端测试，断言空 Token 下管理端接口不再返回 401，且发布令牌防越权隔离依然生效；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 1.1、1.2
```

### Task TSK-0380 <a id="hcr-tsk-0380"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:9`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 [Executor: API Client 401 Interceptor] 扩展 `web/src/api/client.ts`，增加 `onUnauthorized(cb: () => void)` 事件监听与去重状态锁，在捕获 401 响应时触发回调并防止重复弹窗风暴；增加 `setAuthToken(token: string | null)` 凭据管理函数；以 Vitest 单元测试覆盖 401 拦截回调触发与 Bearer 凭据动态附加；验证命令 `cd web && npm test src/api/client.test.ts`
```

### Task TSK-0381 <a id="hcr-tsk-0381"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:10`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 [Executor: AuthModal Component] 创建 `web/src/ui/AuthModal.vue` 响应式凭据录入弹窗组件，支持 Token 输入、明暗文切换、非空校验、持久化写入 `localStorage['csp_token']` 并通知当前视图刷新；以 Vitest 组件测试验证输入合法 Token 后触发保存并关闭弹窗；验证命令 `cd web && npm test src/ui/AuthModal.test.ts`
```

### Task TSK-0382 <a id="hcr-tsk-0382"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:11`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 [Executor: Settings View & Header Status Integration] 创建 `web/src/features/settings/SettingsView.vue` 并在 `web/src/App.vue` 中彻底替换 Phase 6.1 占位卡片，提供查看脱敏凭据、更新 Token、测试连接与一键清除功能；在顶部导航栏增加鉴权模态安全徽章并挂载 `AuthModal`；以 Vitest 验证设置视图与导航切换行为；验证命令 `cd web && npm test src/features/settings/SettingsView.test.ts`
```

### Task TSK-0383 <a id="hcr-tsk-0383"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:12`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 [Reviewer: Frontend Usability Gate] 独立审查 2.1–2.3 前端代码与测试，验证 401 弹窗自愈流程、设置视图完整性与无白屏崩溃；运行 `npm run build` 确保生产分发包构建全绿；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 2.1–2.3
```

### Task TSK-0384 <a id="hcr-tsk-0384"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:16`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 [Executor: Real Server Playwright Harness] 编写无 Mock 真实服务端到端测试脚本 `web/e2e/auth-real-server.spec.ts`，严禁使用 `page.route` 伪造网络数据；覆盖 Open Mode 匿名访问（访问 Subscriptions 与 Node Ledger 零 401）与 Token Mode 弹窗录入自愈两组真机用例；本地启动真实 Go 服务运行验证；验证命令 `cd web && npm run test:playwright e2e/auth-real-server.spec.ts`
```

### Task TSK-0385 <a id="hcr-tsk-0385"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:17`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.2 [Reviewcommon: Dual-Track Contract Gate] 综合审查全栈代码 diff，在本地运行 `scripts/run_e2e_harness.sh` 全套测试套件，确认 Go 单元测试、Data Race 检测、前端 Build 与 E2E 均全绿，满足单轨删旧写新与零绝对路径规范；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 1.3、2.4、3.1
```

### Task TSK-0386 <a id="hcr-tsk-0386"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:18`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.3 [Critic: Headless Browser Adversarial Gate] 针对运行中的 Docker 容器环境（`http://127.0.0.1:17000` 与 `https://sub.einck.top`）执行真实无头浏览器（Playwright/CDP）对抗质检：匿名访问各功能视图，深度监听控制台日志与网络错误，断言零 401 报错、零“Authentication credentials required”红字、DOM 几何布局与节点数据正常呈现；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 3.2
```

### Task TSK-0387 <a id="hcr-tsk-0387"></a>

- **Source**: `openspec/changes/csp-auth-deadlock-v1/tasks.md:19`
- **Source Digest**: `932981dd9b32b7c4c674adeec01b4376d7f68c9f28d02912e285e9c103e0b36f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.4 [Planner: Container Rollout & Deliverable Reconciliation] 在收到 Reviewcommon APPROVED 与 Critic PASSED 凭据后，执行 `docker compose up -d --build` 平滑重启生产容器并校验健康检查 `curl -f http://127.0.0.1:18080/healthz`；向 Einck 交付可用性整改最终闭环战报；依赖 3.3
```

### Task TSK-0388 <a id="hcr-tsk-0388"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:7`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 [Executor: Go Foundation] 创建 `go.mod`、`cmd/csp/`、`internal/domain/`、`internal/application/`、`internal/platform/` 的空白可编译新模块，并以 `go test ./...` 验证不恢复任何旧包路径；此卡独占 `go.mod` 与 `go.sum`
```

### Task TSK-0389 <a id="hcr-tsk-0389"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:8`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 [Executor: Domain Contract] 在 `internal/domain/` 定义 UUIDv7、`logical_id`、时间、枚举、领域错误、实体值对象与端口接口，并以 domain 单元测试验证 ID 不泄漏数据库 row ID；依赖 1.1，独占 `internal/domain/`
```

### Task TSK-0390 <a id="hcr-tsk-0390"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:9`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 [Executor: Schema Foundation] 在 `internal/repository/sqlite/` 与 `migrations/` 实现 CSP 1.0 新 schema、schema version、外键、索引、事务 helper 和空库 readiness 验证，并以临时 SQLite 测试验证 required tables、`foreign_key_check` 与 rollback；依赖 1.1、1.2，独占 `migrations/` 和 `internal/repository/sqlite/`
```

### Task TSK-0391 <a id="hcr-tsk-0391"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:10`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 [Executor: HTTP Foundation] 在 `internal/transport/http/` 实现 request ID、统一 `{data}` 与 `{code,message,request_id}` response、认证边界、CSRF、`/healthz`、`/readyz`、`/api/v1/` 路由骨架，以及旧 `/yaml`、`/script`、未版本化 `/api/` 的 410；以 HTTP contract tests 验证；依赖 1.2、1.3，独占 `internal/transport/http/`
```

### Task TSK-0392 <a id="hcr-tsk-0392"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:11`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 1.5 [Reviewer: Foundation Gate] 独立复跑 `go test ./...`，审查模块依赖方向、无旧 import、错误脱敏、健康与 410 契约；对 `go.mod` 变更、schema 基线和 HTTP 根路径给出 APPROVED、REJECTED 或 REPLAN_REQUIRED；依赖 1.1–1.4
```

### Task TSK-0393 <a id="hcr-tsk-0393"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:15`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 [Executor: Subscription CRUD] 实现 subscriptions 的版本化管理命令、授权、CSRF、secret reference 掩码和 audit events，并以 API integration tests 验证无认证拒绝、无 CSRF 拒绝与令牌不出响应；依赖 1.3、1.4，独占 `internal/application/subscription/` 和对应 HTTP handlers
```

### Task TSK-0394 <a id="hcr-tsk-0394"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:16`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 [Executor: Secure Fetch] 实现明确代理引用、URL scheme、每跳 DNS/IP SSRF 防护、timeout、redirect、compressed/decompressed body limits 与脱敏失败记录，并以本地 httptest fixtures 验证私网和重定向拒绝且不改变已有台账；依赖 2.1，独占 `internal/fetch/`
```

### Task TSK-0395 <a id="hcr-tsk-0395"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:17`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 [Executor: Parser and Identity] 实现 YAML、Base64 和 Shadowsocks、VMess、VLESS、Trojan、Hysteria2、WireGuard、TUIC 的规范化 parser 与不含秘密的稳定 `logical_id`，以测试夹具验证同传输身份跨名称和来源去重；依赖 1.2，独占 `internal/parser/`
```

### Task TSK-0396 <a id="hcr-tsk-0396"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:18`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 [Executor: Inventory Reconciler] 实现 fetch 结果的短事务 upsert、`node_sources` 关联、最后来源差集收敛和失败不覆盖规则，并以 repository integration tests 验证双来源合并、最后来源删除和失败保留；依赖 2.2、2.3，独占 `internal/application/inventory/`
```

### Task TSK-0397 <a id="hcr-tsk-0397"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:19`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.5 [Executor: Node Ledger API] 实现节点详情和带固定排序、过滤、`page/page_size/total` 的服务端分页 API，并以 10,000 节点 SQLite fixture 验证不全量传输、max page size 与稳定 totals；依赖 2.4，独占 `internal/transport/http/nodes.go`
```

### Task TSK-0398 <a id="hcr-tsk-0398"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:20`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.6 [Reviewer: Inventory Gate] 独立验证 SSRF 重定向、秘密 redaction、逻辑 ID、刷新事务和分页性能合同；审查不能通过旧表或全量客户端分页绕过；依赖 2.1–2.5
```

### Task TSK-0399 <a id="hcr-tsk-0399"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:24`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.1 [Executor: Probe State and Queue] 实现 `ProbeRun` 状态机、24 小时幂等键、取消、deadline、bounded sliding window、per-node TTL 排他和容量错误，并用 fake clock/worker tests 验证无重复执行和无 goroutine 泄漏；依赖 1.2、1.3，独占 `internal/probe/queue/` 与 `internal/application/probe/`
```

### Task TSK-0400 <a id="hcr-tsk-0400"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:25`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.2 [Executor: Sing-box Probe Adapter] 在 `internal/probe/singbox/` 实现被测节点内存运行时 adapter，确保 task 的出口 IP、HTTP、TLS 与平台请求全经该 outbound；用可控 proxy fixture 验证本机环境代理不能替代被测节点；依赖 2.3、3.1，独占 `internal/probe/singbox/`
```

### Task TSK-0401 <a id="hcr-tsk-0401"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:26`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.3 [Executor: Observation Profiles] 实现带版本的 baseline、geo、streaming、AI、测速 probe profile，定义 `available/restricted/unknown/error/stale` 判定、漂移与挑战页证据并脱敏持久化；用 fixtures 验证 HTTP 200、challenge、login、timeout 和 contract drift 不会成为 available；依赖 3.2，独占 `internal/probe/profiles/` 与 `internal/application/observation/`
```

### Task TSK-0402 <a id="hcr-tsk-0402"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:27`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.4 [Executor: Probe API] 实现 run 创建、状态读取、取消与证据详情 API，以及作业审计；以 HTTP + repository tests 验证 Idempotency-Key、取消传播、token separation 与 pagination limits；依赖 3.1、3.3，独占 `internal/transport/http/probes.go`
```

### Task TSK-0403 <a id="hcr-tsk-0403"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:28`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.5 [Reviewer: Probe Security Gate] 独立复跑 race-enabled probe tests，验证有界并发、实际被测节点路径、取消清理、错误分类和证据脱敏；发现上游 API 假设不成立时标 REPLAN_REQUIRED；依赖 3.1–3.4
```

### Task TSK-0404 <a id="hcr-tsk-0404"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:32`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 [Executor: Policy Graph] 实现策略组、边、准入规则、config revision 和 validation API，检测自环、环、缺失引用和无效 MATCH 排序；以 property/unit tests 验证非法图不保存；依赖 1.2、1.3、1.4，独占 `internal/application/policy/` 与对应 handlers
```

### Task TSK-0405 <a id="hcr-tsk-0405"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:33`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 [Executor: Deterministic Resolver] 实现唯一 `ResolvedPolicySnapshot` 与 stable digest，供预览、diagnostics 和发布共同消费；以相同输入多次解析测试验证排序和 digest 完全一致；依赖 2.4、4.1，独占 `internal/resolver/`
```

### Task TSK-0406 <a id="hcr-tsk-0406"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:34`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 [Executor: Renderer Set] 在 `internal/compiler/` 分别实现 Clash、Mihomo、sing-box、Surge、Quantumult X renderer 和显式 capability matrix；为每个目标建立 golden fixtures，并验证不支持语义 hard-fail 而非丢弃；依赖 4.2，独占 `internal/compiler/`
```

### Task TSK-0407 <a id="hcr-tsk-0407"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:35`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.4 [Executor: Publication Boundary] 实现不可变 publications、目标绑定令牌、内容摘要、撤销和 `/publish/v1/{publication_id}` 读取合同；以 integration tests 验证 token 不能读管理 API、撤销不回退、预览/导出共用 snapshot；依赖 4.3、1.4，独占 `internal/application/publication/` 与 `internal/transport/http/publications.go`
```

### Task TSK-0408 <a id="hcr-tsk-0408"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:36`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.5 [Reviewer: Compiler Gate] 独立生成五目标 goldens，审查 resolver 是唯一读取路径、能力不兼容不会静默降级、token 范围正确且撤销生效；依赖 4.1–4.4
```

### Task TSK-0409 <a id="hcr-tsk-0409"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:40`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 [Executor: Read-only Legacy Inventory] 创建 `cmd/csp legacy-inspect`，只读打印历史 schema fingerprint、按表计数和已识别字段类别的脱敏报告；以复制的测试 SQLite fixture 验证源不变且不输出值；依赖 1.1、1.3，独占 `internal/import/legacy/inspect/`
```

### Task TSK-0410 <a id="hcr-tsk-0410"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:41`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 [Executor: Allowlist Importer] 创建 `cmd/csp legacy-import --dry-run` 与实际导入逻辑，使用显式字段 allowlist、secret exclusion、quarantine report、批次事务和 `draft` configuration revision；以 legacy fixture 验证目标可追溯导入、秘密永不落库、未知字段被隔离；依赖 5.1、2.3、4.1，独占 `internal/import/legacy/`
```

### Task TSK-0411 <a id="hcr-tsk-0411"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:42`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 [Executor: Draft Review and Activation] 实现导入 draft 的审阅、激活和 audit API；以 integration tests 验证未激活的导入不参与 scheduler、probe、resolver 或 publication；依赖 5.2、2.4、3.1、4.2，独占 `internal/application/revision/` 与对应 handlers
```

### Task TSK-0412 <a id="hcr-tsk-0412"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:43`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.4 [Executor: Ops Contract] 编写新 `docker-compose.example.yml`、Dockerfile、non-root runtime、独立 `csp-v1-data` volume、backup/restore runbook 和 smoke scripts；以 `docker compose config`、镜像 build、空卷 `/healthz` `/readyz` 和 no-legacy-volume-mount 检查验证；依赖 1.4、5.3，独占 `Dockerfile`、`docker-compose*.yml`、`docs/operations.md`
```

### Task TSK-0413 <a id="hcr-tsk-0413"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:44`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 5.5 [Reviewer: Import and Operations Gate] 独立检查导入过程源库只读、quarantine 无秘密、新旧 volume 物理分离、备份 restore smoke 与未授权不切流；依赖 5.1–5.4
```

### Task TSK-0414 <a id="hcr-tsk-0414"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:48`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 6.1 [Executor: Web Foundation] 在 `web/` 基于 Zashboard（Zephyruso/zashboard）开源底座初始化 Vue 3 + Vite + TypeScript + Tailwind CSS + DaisyUI + Heroicons，建立响应式应用外壳、API client 与 Go embed 打包；以 `npm run build` 与 embed smoke test 验证；依赖 1.1、1.4，独占 `web/package.json`、`web/vite.config.ts`、`web/src/main.ts` 和 `internal/webassets/`
```

### Task TSK-0415 <a id="hcr-tsk-0415"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:49`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 6.2 [Executor: Theme and DaisyUI Components] 采用 DaisyUI 官方主题体系（支持 light, dark, dim, cyberpunk 等自由切换）与成熟组件库，实现通知 Toast 与弹性布局，不搞死板尺寸断言；依赖 6.1，独占 `web/src/ui/` 和 `web/src/theme/`
```

### Task TSK-0416 <a id="hcr-tsk-0416"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:50`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.3 [Executor: Subscription and Node Views] 采用 Zashboard 风格的高性能节点卡片与 TanStack Virtual 虚拟滚动列表，实现订阅管理、节点台账、流媒体与 AI 状态回显，支持移动端流畅滚动；依赖 2.1、2.5、6.2，独占 `web/src/features/subscriptions/` 和 `web/src/features/nodes/`
```

### Task TSK-0417 <a id="hcr-tsk-0417"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:51`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.4 [Executor: Mobile Experience and Policy Views] 按照 Zashboard 移动端交互设计：卡片轻触生长展开动画（平滑 200ms ease-out 动效 + 40% 柔和毛玻璃遮罩）、底部抽屉操作，不搞死板几何限制；实现策略图编辑与导出配置预览；依赖 3.4、4.4、6.2，独占 `web/src/features/probes/`、`web/src/features/policy/`、`web/src/features/publications/`
```

### Task TSK-0418 <a id="hcr-tsk-0418"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:52`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.5 [Executor: Multi-Device Verification] 在移动端与桌面端视口验证流畅手感、无异常白屏、真实可用性，验证主题切换与路由持久化；依赖 6.3、6.4，独占 `web/e2e/`
```

### Task TSK-0419 <a id="hcr-tsk-0419"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:53`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 6.6 [Critic: UX and Interaction Gate] 独立体验审查：对照 Zashboard 真实手感验证移动端流畅度、节点与探测回显、主题与操作顺手度，拒绝以死板教条阻碍顺滑体验；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 6.5
```

### Task TSK-0420 <a id="hcr-tsk-0420"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:57`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 7.1 [Executor: Integration Harness] 建立无秘密的端到端 fixture：订阅 refresh → inventory → probe fake provider → policy resolve → 五目标 compile → publication → UI；以 `go test ./...`、`go test -race ./...`、`npm ci && npm run build`、Playwright 和 `docker compose config` 形成可重复报告；依赖 2.6、3.5、4.5、5.5、6.5
```

### Task TSK-0421 <a id="hcr-tsk-0421"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:58`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 7.2 [Reviewcommon: Final Engineering Gate] 从干净 checkout 独立执行 7.1 的所有命令，逐项对照六份 specs、检查旧路径/旧 volume/旧 API 无兼容复活、验证不泄密与回滚文档；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 7.1
```

### Task TSK-0422 <a id="hcr-tsk-0422"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:59`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 7.3 [Critic: Final Adversarial Gate] 对零订阅、节点大量消失、探测队列饱和、provider 契约漂移、启动 schema 失败、发布撤销、导入隔离和前端移动失败进行对抗复核；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 7.1
```

### Task TSK-0423 <a id="hcr-tsk-0423"></a>

- **Source**: `openspec/changes/csp-clean-slate-v1/tasks.md:60`
- **Source Digest**: `5ac3239897009e56e399651fbdbc6f072f864171843cde54afa29d1b5264c54d`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 7.4 [Planner: Release Decision] 汇聚 7.2 APPROVED、7.3 PASSED、测试报告、backup restore 结果与 runbook，向 Einck 请求独立的新容器启动和流量切换授权；未经明确授权不得启动、切流、写入历史卷或删除旧容器/卷；依赖 7.2、7.3
```

### Task TSK-0424 <a id="hcr-tsk-0424"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:3`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 新增数据库 Schema Migration `migrations/000011_plaintext_nodes_and_publications.sql`：`nodes` 表新增 `server TEXT NOT NULL DEFAULT ''`、`port INTEGER NOT NULL DEFAULT 0`、`config_json TEXT NOT NULL DEFAULT '{}'` 并删除 `normalized_config_secret_ref` 和 `credential_version` 列；`publications` 表新增 `content BLOB NOT NULL DEFAULT X''` 并删除 `artifact_key_id`、`artifact_nonce`、`artifact_ciphertext`、`credential_binding_digest`、`credential_bindings_json` 列；执行 `DROP TABLE IF EXISTS node_credentials`。生产迁移说明（仅记录，不在本 Change 中对生产库执行）：生产库 `node_credentials` 仅 3 行、963 个节点大部分无加密凭据，升级 schema 后通过触发所有订阅重新拉取（`ReconcileSubscription`）自动重填 `server/port/config_json`；完整备份位于 `/home/service/backups/csp-v1-backup-20260926_152446.db`，含完整明文节点配置的冷归档备份位于 `/home/service/backups/csp-legacy-cold-archive-20260919.db`。在隔离测试库运行迁移验证表结构变更无误。
```

### Task TSK-0425 <a id="hcr-tsk-0425"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:4`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 精简 `internal/domain/`：删除整个 `internal/domain/node_identity.go` 和 `node_identity_test.go`；删除 `node_credential.go` 中的 `NodeCredentialVault`、`NodeCredentialRecord`、`NewNodeCredentialVault`、`NewNodeCredentialVaultFromEnv`、`Encrypt`、`Decrypt`、`DefaultKeyID`、`EnvNodeCredentialMasterKey`（保留 `InboundProtocolCredential` 及其辅助函数）；删除 `node_connection.go` 整个文件及相关测试（Write-Only、CAS、`SafeNodeConnection`、`UnavailableNodeConnection` 全部移除）；删除 `publication.go` 中的 `PublicationCredentialBinding`、`ComputeCredentialBindingDigest`、`ParseAndVerifyCredentialBindings`、`PublicationArtifactAAD`、`EncryptPublicationArtifact`、`DecryptPublicationArtifact`；简化 `domain.Node` 直接持有 `Server string`、`Port int`、`Credentials InboundProtocolCredential` 并删除 `NormalizedConfigSecretRef`、`CredentialVersion`、`Identity`；简化 `domain.Publication` 直接持有 `Content []byte` 并删除加密与绑定相关字段；删除 `domain/ports.go` 中的 `NodeCredentialRepository` 接口；删除 `domain/probe.go` 中的 `HasValidCredentialVersion` 和 `node_filter.go` 中基于 `CredentialVersion` 的过滤。运行 `go test ./internal/domain/...` 验证通过且退出码为 0。
```

### Task TSK-0426 <a id="hcr-tsk-0426"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:5`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 精简 `internal/parser/`：删除 `parser.go` 中的 `opaqueSecretRef`、`yamlSecrets`、`urlSecrets` 函数；让 `Parse` / `ExtractWithCredentials` 直接产出完整 `domain.Node`（含 `Server`、`Port`、`Credentials`）。运行 `go test ./internal/parser/...` 验证通过且退出码为 0。
```

### Task TSK-0427 <a id="hcr-tsk-0427"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:6`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 精简 `internal/resolver/`：`resolver.ResolvedNode` 直接包含 `Server`、`Port`、`Credentials`，删除 `CredentialVersion` 和 `Identity`（同步清理 `digest.go` 与 `resolver_test.go` 中的 `CredentialVersion`/`Identity` 引用）。运行 `go test ./internal/resolver/...` 验证通过且退出码为 0。
```

### Task TSK-0428 <a id="hcr-tsk-0428"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:7`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.5 精简 `internal/compiler/`（及 `internal/probe/singbox/` 导出辅助）：删除 `compiler.WithCredentials`、`compiler.WithCredentialInputs`、`CompileOptions.Credentials`、`NodeCredentialInput`；`Compile(ctx, snapshot, target)` 直接从 `snapshot.Nodes[i]` 读取节点明文配置；`validateCredentialEnvelope` 简化为仅校验协议必填字段非空（如 SS 的 cipher+password、WG 的 key+address、TUIC 的 uuid+password），删除全部 `VerifiedNodeIdentity` 交叉核验；保留 `CompileMihomo` 作为快捷入口但无需接收 `credentials` 参数。运行 `go test ./internal/compiler/... ./internal/probe/singbox/...` 验证通过且退出码为 0。
```

### Task TSK-0429 <a id="hcr-tsk-0429"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:11`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 重写 Repository 与 Inventory（写集合：`internal/repository/sqlite/{nodes,node_credentials,revisions,node_risk,probes,publication_artifacts_test,node_credentials_test,nodes_credential_version_test,shared_foundation_test,sqlite_test,node_risk_test,node_filters_batch_test,iprisk_test,iprisk_schema_test,iprisk_security_fixture_test}.go`、`internal/application/inventory/**`）：删除 `sqlite/node_credentials.go` 及 `node_credentials_test.go`、`nodes_credential_version_test.go`、`publication_artifacts_test.go`；`sqlite/nodes.go`（及 `node_risk.go`）直接读写 `server`、`port`、`config_json` 明文列；`sqlite/revisions.go` 直接读写 `content` 明文列；`inventory/service.go` 的 `ReconcileSubscription` 直接将解析结果写入 `nodes` 表（约 30 行替换 ~250 行加解密逻辑）；删除 `WithCredentialVault`、`credentialTransactionRepository`、`projectNodeConnection`、`verifyCredentialIdentity` 及复杂的 `UpdateNodeConnection`，简化为直接更新 `nodes` 表明文字段；`GetNodeDetailWithRisk` 直接返回自带完整配置的 `domain.Node`。运行 `go test ./internal/repository/sqlite/... ./internal/application/inventory/...` 验证通过且退出码为 0。
```

### Task TSK-0430 <a id="hcr-tsk-0430"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:12`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 精简 Publication Service（写集合：`internal/application/publication/**`）：删除 `vault`/`credRepo` 依赖注入、`resolveCredentials`（~100 行）与 `verifyCredentialBindings`（~95 行）；`Publish` 编译后直接存储明文 `Content` 到 `publications` 表；`ResolveAndServe` 校验 Token 与未撤销后直接返回 `pub.Content`（若旧记录无 `Content` 则实时编译）。运行 `go test ./internal/application/publication/...` 验证通过且退出码为 0。
```

### Task TSK-0431 <a id="hcr-tsk-0431"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:13`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 精简 Probe Service（写集合：`internal/application/probe/**`）：`safe_dialer.go` 直接从 `domain.Node` 读取明文配置，删除查库与解密逻辑（~60 行）；`periodic.go` 删除 `CredentialVersion <= 0` 跳过逻辑；`runner.go` 无条件使用 `SafeNodeDialer`。运行 `go test ./internal/application/probe/...` 验证通过且退出码为 0。
```

### Task TSK-0432 <a id="hcr-tsk-0432"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:14`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 精简 HTTP Transport（写集合：`internal/transport/http/{nodes,publications,subscriptions}*.go`）：`nodes.go` 中 `GET /api/v1/nodes/{id}` 直接返回节点完整明文信息（管理员接口已有 Token 保护），`PATCH /api/v1/nodes/{id}`（含 `/connection` 路由兼容或统一）简化为直接接收并更新明文字段；`publications.go` 删除 `ErrIntegrityCheckFailed` 映射与 Vault 相关错误码；`subscriptions.go` 直接返回并更新真实订阅 URL，移除 `***` 占位保留判断。运行 `go test ./internal/transport/http/...` 验证通过且退出码为 0。
```

### Task TSK-0433 <a id="hcr-tsk-0433"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:15`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.5 精简前端（写集合：`web/src/**`）：节点详情删除 `***` 掩码、Write-Only 轮换、CAS 版本号、Vault AEAD 徽章与 Unavailable 横幅，直接展示和编辑明文字段；发布预览删除 `redactPreviewSecrets`，直接展示真实配置文本；订阅编辑去掉 `***` 占位，直接展示与提交真实 URL。运行前端 Vitest 单测与 `npx vue-tsc --noEmit` 验证通过且退出码为 0。
```

### Task TSK-0434 <a id="hcr-tsk-0434"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:16`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.6 清理 `cmd/csp` 与 `docker-compose.yml`（写集合：`cmd/csp/main*.go`、`docker-compose.yml`）：删除 `CSP_NODE_CREDENTIAL_KEY` 环境变量读取、告警与 Compose 环境变量项；删除 `NodeCredentialRepository` 与 `WithCredentialVault` 初始化；`SafeNodeDialer` 无条件挂载。运行 `go test ./cmd/csp/...` 与 `go build ./...` 验证通过且退出码为 0。
```

### Task TSK-0435 <a id="hcr-tsk-0435"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:20`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 同步更新 `internal/integration/**` 测试以适配无 Vault 明文契约；按 `go list ./...` 分 3 组执行全量 `go test -race -count=3`、执行 `go test -count=1 ./...` 与 `go build ./...` 全部 exit 0；执行 `web` Vitest 单测与 `npx vue-tsc --noEmit` exit 0；使用 `NO_COPY_WEBASSETS=1` 在隔离目录执行 `vite build` 并校验原仓 `internal/webassets/dist` 文件 hash 不变；执行 `openspec validate csp-debloat-over-engineering --strict` exit 0。
```

### Task TSK-0436 <a id="hcr-tsk-0436"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:21`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 独立 `reviewer` 对照 `csp-debloat-over-engineering` 规格与最终合流 diff 完成只读代码与安全审查并给出 `PASS` 裁决。
```

### Task TSK-0437 <a id="hcr-tsk-0437"></a>

- **Source**: `openspec/changes/csp-debloat-over-engineering/tasks.md:22`
- **Source Digest**: `6672d2f000a3250d3f918dbe381b03b7994a595878e8b60b60043f0c41b84cb2`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 准备者在隔离环境（临时 SQLite 库、无 `CSP_NODE_CREDENTIAL_KEY`、非生产端口）启动预览实例并通过健康检查，移交独立 `critic` 执行订阅拉取、节点明文详情与编辑、配置明文预览、发布、下载与撤销全链路 `FLOW` + 多视口 `VISUAL` 验收并取得 `PASSED`。
```

### Task TSK-0438 <a id="hcr-tsk-0438"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:3`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 创建数据库迁移文件 `migrations/000013_auth_settings.sql`，执行 `ALTER TABLE settings ADD COLUMN admin_auth_enabled INTEGER NOT NULL DEFAULT 1` 和 `ALTER TABLE settings ADD COLUMN export_auth_enabled INTEGER NOT NULL DEFAULT 1`，通过 `go test ./internal/repository/sqlite/...` 验证迁移成功应用且默认值为 1。
```

### Task TSK-0439 <a id="hcr-tsk-0439"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:4`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 扩展 `internal/domain/settings.go` 与 `internal/domain/ports.go`，在 `Settings` 结构体中添加 `AdminAuthEnabled` 与 `ExportAuthEnabled` 字段，并在 `DefaultSettings()` 中赋予 `true` 默认值；通过 Go 单元测试断言结构体验证与字段初始化正确。
```

### Task TSK-0440 <a id="hcr-tsk-0440"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:5`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 改造 `internal/repository/sqlite/settings.go`，在 `Get` 与 `Update` 方法中支持 `admin_auth_enabled` 与 `export_auth_enabled` 的读写持久化，通过 `go test ./internal/repository/sqlite/...` 验证存取逻辑与数据一致性。
```

### Task TSK-0441 <a id="hcr-tsk-0441"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:9`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 重构 `internal/transport/http/token.go` 中的 `DynamicTokenHolder`，引入 `adminAuthEnabled` 与 `exportAuthEnabled` 状态管理，提供独立的 `AdminMode()`、`IsExportAuthEnabled()` 及 `SetAuthSwitches` 方法，通过 `go test ./internal/transport/http/...` 验证在不同开关组合下的模式判定行为。
```

### Task TSK-0442 <a id="hcr-tsk-0442"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:10`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 调整 `internal/transport/http/middleware_auth.go` 及相关上下文处理，当 `DynamicTokenHolder.AdminMode() == SecurityModeOpen`（即 `admin_auth_enabled == false` 或无令牌）时确保管理端请求无条件作为管理员放行，通过单元测试验证在配置了令牌但关闭管理端鉴权时的免密放行行为。
```

### Task TSK-0443 <a id="hcr-tsk-0443"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:11`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 扩展 `internal/transport/http/auth.go` 中的 `AuthStatusData` 与 `authStatusHandler`，使其返回详细的管理端模式与导出鉴权状态，通过测试验证状态接口输出正确性。
```

### Task TSK-0444 <a id="hcr-tsk-0444"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:15`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 改造 `internal/application/publication/service.go` 与 `internal/transport/http/publications.go` 中的 `publicationClientHandler`：在 `export_auth_enabled == false` 时允许免密直接拉取已激活发布内容；在 `export_auth_enabled == true` 时，优先校验系统统一主令牌，若不匹配再校验 publication 专属 TokenHash，两者皆不匹配或未传参时返回 401；通过 `go test ./internal/transport/http/...` 覆盖免密拉取、专属令牌拉取、统一系统令牌拉取及错误令牌拦截四种场景。
```

### Task TSK-0445 <a id="hcr-tsk-0445"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:16`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 在 `internal/transport/http/router.go` 挂载 `/p/{publication_id}` 历史兼容路由，确保请求该路径时与 `/publish/v1/{publication_id}` 享受相同的导出服务或平滑 307 重定向，通过针对性 HTTP 测试验证历史客户端请求不再报 404。
```

### Task TSK-0446 <a id="hcr-tsk-0446"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:20`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 在 `internal/transport/http/settings.go` 实现 `GET /api/v1/settings/auth` 与 `PUT /api/v1/settings/auth` 路由处理函数，支持原子查询与修改管理端开关、导出链接开关以及系统统一令牌，并记录审计事件；保持对既有 `/api/v1/settings/admin-token` 的兼容转发；通过 `go test ./internal/transport/http/...` 验证 API 行为。
```

### Task TSK-0447 <a id="hcr-tsk-0447"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:21`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 运行全量后端测试 `go test ./...` 与静态编译 `go build ./...`，确保整个后端的类型系统、契约接口与现有测试套件全部通过（退出码 0）。
```

### Task TSK-0448 <a id="hcr-tsk-0448"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:25`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 改造 `web/src/features/settings/SettingsView.vue`，实现管理端鉴权独立开关、导出链接鉴权独立开关以及系统统一令牌维护界面，对接 `GET/PUT /api/v1/settings/auth` API，并补齐 i18n 语言文案，通过组件测试 `SettingsView.test.ts` 验证表单交互与状态回显。
```

### Task TSK-0449 <a id="hcr-tsk-0449"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:26`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 改造 `web/src/features/publications/PublicationsView.vue` 与 `web/src/features/publications/usePublications.ts`：针对数据库 0 记录场景增设醒目的发布引导空状态；根据 `export_auth_enabled` 动态生成纯净直链（免密）或带 Token 链接（受控），对齐一键复制与下载行为，通过组件测试 `publications.test.ts` 验证链接生成与向导逻辑。
```

### Task TSK-0450 <a id="hcr-tsk-0450"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:27`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 运行前端类型检查 `npm run type-check` 与前端单元测试 `npm run test`，确保退出码为 0，无任何 TypeScript 语法或构建错误。
```

### Task TSK-0451 <a id="hcr-tsk-0451"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:31`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.1 运行全项目统一静态编译 `go build ./...`、全量后端单元测试 `go test ./...` 以及前端类型检查 `npm run type-check`，确保全部退出码为 0。
```

### Task TSK-0452 <a id="hcr-tsk-0452"></a>

- **Source**: `openspec/changes/csp-decoupled-auth-settings-and-publication-recovery/tasks.md:32`
- **Source Digest**: `294408a71d493c13b8fdaec947c42c0df5d700524a30fcfca2ba69d3fca84ff5`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.2 执行端到端或集成测试，完整模拟免密拉取、统一系统令牌拉取、专属令牌拉取、管理端独立免密以及 `/p/...` 历史短路径重定向全流程，验证整体系统功能完好。
```

### Task TSK-0453 <a id="hcr-tsk-0453"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:11`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 在 SQLite 仓储层实现原子节点重置闭包、受影响绑定捕获与前置/后置外键完整性校验，并通过单元测试验证
```

### Task TSK-0454 <a id="hcr-tsk-0454"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:12`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 在 inventory 应用服务层实现 ResetNodeInventory 业务逻辑、不可变发布解耦与旧抓取指针切断，保持抓取审计事实，并通过服务测试验证
```

### Task TSK-0455 <a id="hcr-tsk-0455"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:16`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 实现基于冷归档稳定身份证据的 7li 与魔戒 URL 精准恢复、幂等性跳过与脱敏审计，并通过单元测试验证
```

### Task TSK-0456 <a id="hcr-tsk-0456"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:17`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 验证其余 7 个订阅源状态与配置严格不变，且日志与输出完全脱敏（包含路径 Token 掩码）
```

### Task TSK-0457 <a id="hcr-tsk-0457"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:21`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 在 cmd/csp 中新增 reset-node-inventory 维护子命令，支持 --db, --dry-run, --apply, --confirm-backup, --backup-file, --restore-source-tokens，并通过 CLI 测试验证
```

### Task TSK-0458 <a id="hcr-tsk-0458"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:22`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 验证 dry-run 模式只读不写与 apply 模式的备份硬确认、原子提交与结构化 JSON 报告输出
```

### Task TSK-0459 <a id="hcr-tsk-0459"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:26`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 实现 HTTP, SOCKS5, VLESS, AnyTLS 协议扩展与 Mihomo 适配支持
```

### Task TSK-0460 <a id="hcr-tsk-0460"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:27`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 实现 domain.ComputeConnectionLogicalID 区分同端点多凭据节点，通过 fresh_private_test.go 门禁与单元测试验证
```

### Task TSK-0461 <a id="hcr-tsk-0461"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:31`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 实现 same-service 隔离刷新工作流测试（涵盖 success/empty/fail/notice/manual/multi-credentials 与 last-good 保持）
```

### Task TSK-0462 <a id="hcr-tsk-0462"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:32`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 运行 go build ./...、全量测试（含 -race）与前端 type-check/tests，确保退出码均为 0
```

### Task TSK-0463 <a id="hcr-tsk-0463"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:36`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.1 对 4 个已启用订阅源（7li、魔戒、Dogegg、einck-qzz）执行全新抓取，验证节点解析入库（抓取 95 个候选节点，去重入库 90 个全新有效节点，全部标记 active=1，5 个禁用源严格未触碰）
```

### Task TSK-0464 <a id="hcr-tsk-0464"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:37`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.2 针对任何抓取失败或异常响应输出详尽根因归因分析与诊断记录（7li 返回空内容，归因诊断为订阅内容为空，如实记录失败而未伪造旧节点；魔戒 44、Dogegg 18、einck-qzz 33 成功入库）
```

### Task TSK-0465 <a id="hcr-tsk-0465"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:41`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 7.1 在隔离环境中对全部新节点执行公平 Benchmark 探针连通性与延迟评测（全量 90 节点完成 540 项有界任务：69 存活，385 成功，50 失败，105 跳过，耗时 5m54s，下载 54.5MiB 严格控制在 128MiB 预算内）
```

### Task TSK-0466 <a id="hcr-tsk-0466"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:42`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 7.2 依据策略规则与准入规则筛选 publication candidates 候选节点集（候选节点按规则流向汇入香港、台湾、日本、新加坡、美国、加拿大及其他组）
```

### Task TSK-0467 <a id="hcr-tsk-0467"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:46`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 8.1 最终自测/14.1 review及14.2 staged batch完成后冻结源码，准备者刷新独立DB/端口preview，禁自动外网refresh/probe；不重用旧19080未绑定源码实例作为凭据。交付instanceId/url/health/allowedOrigins/testDataBoundary/scenarios/shutdownOwner/cleanupToken/sourceManifest及binary/frontend SHA；含token私有JSON0600，交付公共合同不带秘密；只在指定fixture组opt-in，生产用户flags不变。
```

### Task TSK-0468 <a id="hcr-tsk-0468"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:47`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 8.2 URL-ready且最终Reviewer后，Critic只读Playwright+read PNG验证既有375/390/1440视口无溢出遮挡、checkbox保存重载/空非空/uncheck、regex编辑、retry/manual/晚响应与最新issues、PASS非DIRECT发布预览及probe状态解释，交真实截图/几何证据/CRITIC_VISUAL_VERDICT。无URL/隔离/责任人不派工；旧Critic不替代最终源码验收。
```

### Task TSK-0469 <a id="hcr-tsk-0469"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:51`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 9.1 执行生产环境服务发布与节点重置切换（完成旧 1014 节点清空、两源 Token 修复、15 策略组正则过滤恢复、1 条 kliq 规则精准删除，生产容器已成功滚动至 414a6b7 最新镜像并恢复健康双端口服务）
```

### Task TSK-0470 <a id="hcr-tsk-0470"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:52`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 9.2 15.2 rollout健康后另交7200s真实生产调度/起止时间与有界观察记录，关联已解释配对结果。health poll/隔离加速不能冒充7200s完成；未满周期保留unchecked并区分“已上线/观察未完成”，不无限轮询。真实外部auth/设备边界交父编排，不绕鉴权、不在聊天索要明文凭据。
```

### Task TSK-0471 <a id="hcr-tsk-0471"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:56`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 10.1 后端 Policy 校验与 Resolver 增强：在 resolver 与 policy 服务中将直接被分流规则指向的空组标记为 error（无论是否有 filter），输出结构化 issues（包含 rule_id, position, type, value, target_group_id, target_group_name, severity, message）并返回 revision_id，保持既有 valid/errors 字段兼容
```

### Task TSK-0472 <a id="hcr-tsk-0472"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:57`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 10.2 规则精准删除与事务安全测试：针对规则 01a0b9af-c118-72f9-9949-d94b49fa6ec2（指向“其他”组）编写精准删除与验证测试，确保 1->0 规则扣减、新 revision 生成、策略组保留与历史不可变发布快照隔离
```

### Task TSK-0473 <a id="hcr-tsk-0473"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:58`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 10.3 前端 PolicyView 增设分流规则 Tab、手动校验按钮、自动校验触发（初始/刷新/修改后）及防竞态守卫（AbortController + 请求序列），并通过类型与构建验证
```

### Task TSK-0474 <a id="hcr-tsk-0474"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:62`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 11.1 父编排确权当前真实run并冻结version1 completion_contract、同一change/changeRoot/schema、A/B全work_packages/epochs、executor/reviewer必选及critic applicable=true/UI理由，派工显式run_id/change/work_package/epoch/reverification_of。本轮规划不写run metadata。确定单写公共类型/schema/模块/ledger/generated ownership、design §8真实CLI接口和network fail-closed；保留旧报告和失败引用。不开新项目/Change/历史run，不把preflight当完成。
```

### Task TSK-0475 <a id="hcr-tsk-0475"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:68`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 12.1 实现 empty_fallback_pass 默认 false 的 domain/SQLite 增量 migration/仓储/Create-Update API/UI checkbox 全链路，旧 DB、创建默认、Get/List 与 PATCH omission/false/true roundtrip；失败更新不改 metadata/flag/filter/revision，生产原 flags 不 blanket-enable。
```

### Task TSK-0476 <a id="hcr-tsk-0476"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:69`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 12.2 在 resolver/digest/compiler/prune/publication 所有门禁共享显式有效 PASS 语义：只有明确配置的静态空组可用 PASS，非空无 PASS、未勾选仍 required_nonempty、真实节点计数不变；父组/derived 不自动继承，graph/risk/capability/source/probe/global filter 守卫保留，兼容模式无 DIRECT/COMPATIBLE 偷渡，Mihomo 离线真实规则 fixture 证明 PASS 命中后续非 DIRECT 控制规则。
```

### Task TSK-0477 <a id="hcr-tsk-0477"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:70`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 12.3 复用 RE2 与 regexp2 有限 MatchTimeout/有界 cache-input，Validate/Match 同语义、not_regex 超时不放行；用自建归档 fixture 和原 PCRE 黄金语料保真恢复缺失“便宜”过滤器（稳定 ID/幂等/保留现有配置），不猜 literal“去掉”含义、不删 lookahead；UI 支持 regex/not_regex 创建编辑保真。
```

### Task TSK-0478 <a id="hcr-tsk-0478"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:71`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 12.4 修复 reload/retry→最新 revision→validate，沿用现有 warnings/manual/Abort-seq 守卫；校验与 publication 同一只读 NodeSources/LatestObservations/risk/filter 装配，repository 故障显式 incomplete；验证 stale、失败保存、错误重试与每条规则真实 issues。
```

### Task TSK-0479 <a id="hcr-tsk-0479"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:72`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 12.5 最新源码重新完成A回归与Web type-check/tests/NO_COPY开发build，保留原生PASS与PCRE/事务/仓储失败fixture。go build/test FULL成功及full race真实凭据由13.4统一取得；targeted race不是full PASS。full race资源限制必须确切可复现/日志与unchecked，不以文字宣称完成；旧PASS之后的新UI改动包含在14.1最终独立review。
```

### Task TSK-0480 <a id="hcr-tsk-0480"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:78`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 13.1 完成真实runner/profile任意2xx normalized策略、实际node×side baseline≤65536 body bytes；固定upstream commit3c320fd58aff5235e16218c050ec5b8ce587e233，独立标记native Mihomo1.19.31 Check与same-engine1.19.32 stage comparator，保存samecore modfile/全部传递依赖diff、真实build metadata/pin，修硬编码engine；原生Check保持authentic config/defaults且speed disabled，baseline-only native config允许，无订阅重抓/Web/cron/upload/save/callback。
```

### Task TSK-0481 <a id="hcr-tsk-0481"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:79`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 13.2 完成跨launch shared aggregate ledger、原始start/deadline、原子read-before-reserve/settle/refund、崩溃reservation保守恢复；全部成功/失败body和native非collector平台/redirect/内嵌client进入同账本，body/NIC分开。整批134217728 bytes/1800s含cleanup，实际reader无extra-byte超限；无法覆盖路径network fail-closed先整改，不用collector事后汇总冒充硬限额。
```

### Task TSK-0482 <a id="hcr-tsk-0482"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:80`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 13.3 全node×side×stage×attempt queued/not_scheduled/executed、verdict/reason/status/profile/target/engine/commit/revision/config指纹/时戳/body/复验关联恰一终态；dependency优先保留根因、typed net/x509/context错误及native平台原始返回、非collector完整导出；unknown/restricted/skip/cancel/budget/stale/mismatch不混fail/永久dead。secret脱敏、cleanup与失败历史保真。
```

### Task TSK-0483 <a id="hcr-tsk-0483"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:81`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 13.4 correction后私有有界cache离线完成 `go build ./...`、`go test ./...` FULL和 `go test -race ./...` FULL、Python unittest golden/scheduler/ledger、native与same-engine reference各自build/test/Check与stage fixtures、最终Web门禁，五要素报告+实际exit/log/source指纹。预检可用磁盘6.4GiB，资源先核对、限制并发/留余量、不删共享cache/联网补依赖，合理长命令timeout和可见进度不机械轮询。full race若真实环境限制只报告可复现unchecked并保持本项未勾，由父编排裁定适用性，不伪造全通过。
```

### Task TSK-0484 <a id="hcr-tsk-0484"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:85`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 14B.1 保存真实旧 AB/BA partial：90节点、1620终态记录、107537726 body bytes（102.56MiB），900.027s；不覆盖原记录、不宣称parity，逐节点私有证据句柄进入 docs/evidence-final-validation.md。
```

### Task TSK-0485 <a id="hcr-tsk-0485"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:86`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 14B.2 实现 `tools/probe-comparison/run.py` stage-first执行器：主四侧ALL90 baseline A1→B1→B2→A2全覆盖终态后，固定四侧baseline intersection作为平台共同eligibility；同stage公平相等side allocation与per-node participation，discordant另列诊断。新批speed0/无speed URL和请求；alive8/media2 reference worker真正生效并由blocking offline fixture验证非串行/不超上限。默认4侧22.5MiB最坏baseline，platform105.5MiB四侧均分；native可选0/2两侧baseline-only在首次冻结同一128MiB/30min总盘子内可行且不伤主覆盖才加，不强制6。
```

### Task TSK-0486 <a id="hcr-tsk-0486"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:87`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 14B.3 冻结所有side/stage/node manifest/identity/SHA与派生参与表算法、exact child argv/target子请求、deadline/timeout/TLS/redirect/HTTP2/selection/UA；comparative CSP/same-engine统一条件，native defaults差异单列。execute禁止自适应endpoint/并发/TLS修改/未记载重试或扩额；平台未尝试项/native未选media精确清单，离线证明所有平台调用预算路径（含非collector）与取消/cleanup。
```

### Task TSK-0487 <a id="hcr-tsk-0487"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:88`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 14B.4 按design §8实现明确plan/freeze/execute/report参数：plan/freeze/report无网络，execute必须allow-network且审核通过/manifest匹配/ledger绑定/speed0/body≤128MiB/整批≤1800s，非法输入发请求前拒绝；Python黄金allocation/stage顺序/交集/side/node公平/可选native扣额、Go实际64KiB与跨launch并发reservation/refund/cancel/崩溃、native Check真实baseline-only/禁speed、samecore diff与真实metadata fixtures均离线验证。
```

### Task TSK-0488 <a id="hcr-tsk-0488"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:89`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 14B.5 与12.5/13.4统一完成最新源码全量静态/功能/race或诚实unchecked环境证据、reference/Python/Web；网络仍默认关闭直到14.1独立review通过。不存在512MiB REQUESTED计划或新测速suite；新批授权仅同一128MiB/30min staged方案。
```

### Task TSK-0489 <a id="hcr-tsk-0489"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:93`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 14.1 全部施工自测完成后独立reviewer对同一Change最终diff/API/migration/原生PASS/PCRE/校验输入/UI新增变更/stage-first/actual64KiB/跨launchledger/reference并发/所有noncollector计量/native与samecore来源/no-network safety复验，真实verdict/commands exit/log；旧rep_rev_csp_preview_code_review_2不覆盖新源码。历史失败用reverification_of/final=true明确消除，环境unchecked不得冒称full门禁通过；失败主脑根因整改后再施工/独立review。
```

### Task TSK-0490 <a id="hcr-tsk-0490"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:94`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 14.2 Reviewer后执行已批准stage-first batch：前置冻结exact ALL90/所有sides manifest与ledger/start/deadline，默认CSP vs same-engine ABBA四baseline，native两侧仅可选固定总盘子；之后common intersection平台，alive8/media2/speed0，无测速请求/重试/512MiB/自动扩额，无publish/生产stored-state mutation/reset/import/商业源刷新。整批含cleanup≤1800s/body≤134217728，公平side/node配额；输出实际executed/failed/skipped/cancelled/unattempted、discordant与缺陷解释/条件不可比、资源/queue/请求/body/NIC/成功分位和删失样本。不能保证全部native媒体完成，不声称全量parity；release按解释清楚缺陷而非headline计数。产生代码变更回full自测/review并刷新preview。
```

### Task TSK-0491 <a id="hcr-tsk-0491"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:98`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 15.1 最终源码/preview/Reviewer/Critic真实通过后准备者明确 `COPY_WEBASSETS=1 NO_COPY_WEBASSETS=0` 同步嵌入assets，与最终preview/build指纹绑定、无额外源码；根编排只git_commit授权CSP路径、核对staged不夹无关改动、记录hash。经验维护责任人仅给 `.exp.md`旧Python/pytest/backend-data段落加obsolete前缀与当前Go部署不适用说明，不无关重写；父编排按授权核对真实经验变更。源码追加变化回完整门禁。
```

### Task TSK-0492 <a id="hcr-tsk-0492"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:99`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 15.2 按真实controlled_deployment_runbook/operations做SQLite一致性online hot .backup，size/schema/integrity/FK/SHA/0600/owner核验、现image rollback tag与schema17兼容隔离演练；app-only/no-deps/no-build上线及双loopback healthz/readyz/401/schema/assets/FK0和合法管理/发布核对。保持用户group flags、源启停与业务配置，无reset/import/down/删卷/回灌/重复删规则；只CSP，不触碰无关服务。记录rollout完成，7200s观察归9.2另报。
```

### Task TSK-0493 <a id="hcr-tsk-0493"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:105`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-1 run.py / ledger.py 预算、截止时间与 Change 参数化支持（保留旧默认 1800s/128MiB，支持新授权 7200s/1GiB，绑定 manifest，取消写死 change 名但保留 review-report 严格校验与 hash 绑定）
```

### Task TSK-0494 <a id="hcr-tsk-0494"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:106`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-2 子进程 invoke 增加 <=10 秒心跳机制（side/stage/elapsed/consumed/reserved/finished/deadline）、安全 stdout/stderr 双通道并发消费防死锁及内存/日志敏感信息脱敏
```

### Task TSK-0495 <a id="hcr-tsk-0495"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:107`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-3 独立守护进程启动与监督脚本/包装（daemon mode / wrapper，PID/启动时间/收据/progress.log/runner-exit.json 原子写盘，总 deadline/budget 进程内强制执行，崩溃不自动退款/刷新，stage 级失败归因）
```

### Task TSK-0496 <a id="hcr-tsk-0496"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:108`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-4 结果与报告原子落盘（terminal/eligibility/report 原子写入，全 90*4*4 阶段全量终态，平台级分层 supported/not_supported/error/unexecuted，无伪造 latency）
```

### Task TSK-0497 <a id="hcr-tsk-0497"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:109`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-5 完善脱敏报告生成器与指标汇总（新身份 hash、两端版本、各侧 baseline/交集、平台一致性、匿名归因、实际 vs 保守未结算 bytes、耗时与故障分类，无凭据泄漏）
```

### Task TSK-0498 <a id="hcr-tsk-0498"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:110`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-6 csp 与 reference runner 适配新限额（csp/main.go 与 reference_entry.go.txt 解除 30m / 128MiB 写死上限，支持至 4h / 1GiB）
```

### Task TSK-0499 <a id="hcr-tsk-0499"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:111`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-7 完整离线单元测试与 Go 编译门禁（Python py_compile/unittest 全绿、Go build/test 全绿，包含参数化、二次 start 拒绝、不退款、心跳/崩溃终态/原子写/资源限制 mock 测试）
```

### Task TSK-0500 <a id="hcr-tsk-0500"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:112`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-8 离线测试 plan 与 freeze，生成新 manifest 与 new ledger（不发网络，验证 sha256 绑定与状态）
```

### Task TSK-0501 <a id="hcr-tsk-0501"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:119`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-10 修复 launch/execute 目录冲突与控制目录隔离：execute 保持拒绝已有结果目录，launch-daemon 控制文件移入 `control/<launch_attempt_id>` 独立目录，实现 append-only 历史与同 ledger 存活进程互斥检测
```

### Task TSK-0502 <a id="hcr-tsk-0502"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:120`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-11 编排源码 hash（run.py, ledger.py）加入 manifest 源码绑定，离线 plan/freeze 生成 v2 隔离目录及新 manifest 与 ledger
```

### Task TSK-0503 <a id="hcr-tsk-0503"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:121`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-12 真实 subprocess launch→execute 集成测试 fixture（验证结果目录预置拒绝、首次创建成功、二次启动拒绝、failed log 不覆盖、子进程退出证据）
```

### Task TSK-0504 <a id="hcr-tsk-0504"></a>

- **Source**: `openspec/changes/csp-fresh-inventory-full-validation/tasks.md:122`
- **Source Digest**: `a45471bd53d3e77abbd994019417f5aa31b98cc8d74bec8ed45f7068e7597f22`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] reissue-9 经新授权与独立审查批准后启动并完成真实外网全量批次（PID 4092797 正常收口 exit 0，16 主阶段全量终态，脱敏报告及对比分析生成于私有目录 /tmp/csp-reissue-20261008T192722Z-v2/）
```

### Task TSK-0505 <a id="hcr-tsk-0505"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/tasks.md:3`
- **Source Digest**: `84e04cbe98f047bba038e8f55ea414b97f5988e9e19dbe7e3b8afce0dbf57688`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 仅在 `internal/application/publication/**` 实现并自测新预览/预检/新发布的安全共用解析：筛选/组筛选读取错误不透传；已配置 probe/source 条件时观测/来源读取错误或必要依赖缺失一律阻断；无条件透传、旧直链冻结不变；用注入失败仓库的定点 Go 测试验证三入口及四目标不会产生额外节点或意外发布，运行 `go test ./internal/application/publication/...` 和 `go build ./...` 退出码均为 0。
```

### Task TSK-0506 <a id="hcr-tsk-0506"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/tasks.md:4`
- **Source Digest**: `84e04cbe98f047bba038e8f55ea414b97f5988e9e19dbe7e3b8afce0dbf57688`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 在同一 A 包内完成已配置条件筛出零节点的可识别诊断与新发布阻断，覆盖无路由组、含路由组、未测/过期、四目标及旧直链的测试；验证 `go test ./internal/application/publication/...` 和 `go build ./...` 退出码均为 0。A 包自行完成 1.1、1.2 的代码及自测后才交工。
```

### Task TSK-0507 <a id="hcr-tsk-0507"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/tasks.md:8`
- **Source Digest**: `84e04cbe98f047bba038e8f55ea414b97f5988e9e19dbe7e3b8afce0dbf57688`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 仅修改 `web/src/features/settings/**` 及必要 `web/src/features/policy/PolicyView.vue` 文案/共享已有类型：设置页复用同一 GET/PUT 全局筛选，允许现有全部合法字段回显与编辑、探针 kind/verdict/有效期、AND、清空及错误处理；准确说明未测/过期、24h 默认、新发布才生效及 queued/probing 非持久状态，纠正策略页凭据版本匹配的误导文案；增补设置页组件测试验证读写 payload、刷新回显、非法输入/读写失败不覆盖、双入口文案，运行前端针对性测试及 `npm run type-check` 退出码均为 0，B 包实现与自测一并交工。
```

### Task TSK-0508 <a id="hcr-tsk-0508"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/tasks.md:12`
- **Source Digest**: `84e04cbe98f047bba038e8f55ea414b97f5988e9e19dbe7e3b8afce0dbf57688`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 仅修改 `web/src/ui/{Popover,ModalDialog,ConfirmModal,Drawer}.vue`、必要的 `web/src/features/{subscriptions,publications,probes}/**` 调用点及对应测试（不写 PolicyView.vue、SettingsView.vue、共享样式）：订阅菜单脱离裁切祖先并按可用视口定位、Escape/外部点击/键盘焦点可用；确认框进入与原生 dialog 兼容的 top layer；短高度标题/正文可滚、操作可见，确保不会因普通 z-index 遮挡；用组件测试覆盖事件/焦点/叠层，运行前端针对性测试及 `npm run type-check` 退出码均为 0，C 包实现与自测一并交工。
```

### Task TSK-0509 <a id="hcr-tsk-0509"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/tasks.md:16`
- **Source Digest**: `84e04cbe98f047bba038e8f55ea414b97f5988e9e19dbe7e3b8afce0dbf57688`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 在三包全部完成后的最终工作树运行 `go test ./...`、`go build ./...`、前端完整测试和 `npm run type-check`，各退出码 0；跨层核对四目标从新预览到新发布的筛选/零结果诊断一致性，且不存在共享文件双写或新旧发布内容混淆。
```

### Task TSK-0510 <a id="hcr-tsk-0510"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/tasks.md:17`
- **Source Digest**: `84e04cbe98f047bba038e8f55ea414b97f5988e9e19dbe7e3b8afce0dbf57688`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 独立只读 reviewer 对同一 Change 全部规格、最终 diff、失败关闭分支、旧直链冻结与真实测试证据复验并出具明确 PASS；不以施工者自测替代审查。
```

### Task TSK-0511 <a id="hcr-tsk-0511"></a>

- **Source**: `openspec/changes/csp-global-probe-export-filter-and-overlays/tasks.md:18`
- **Source Digest**: `84e04cbe98f047bba038e8f55ea414b97f5988e9e19dbe7e3b8afce0dbf57688`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 工程审查通过后由准备者提供隔离预览 URL、健康检查与标准实例 JSON，独立 critic 对订阅/发布/策略/探针页在窄手机、宽手机、平板、桌面及短高度视口操作菜单、嵌套确认框、长内容表单/抽屉，结合真实浏览器截图、几何与键盘交互给出 `CRITIC_VISUAL_VERDICT: PASSED`；无隔离 URL 或可核验证据不得勾选本条。
```

### Task TSK-0512 <a id="hcr-tsk-0512"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:3`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 在 `PeriodicCoordinator` 中注入 `ProbeObservationRepository` 与 `sweepInterval` 配置项，更新构造函数与选项函数，修改 `cmd/csp/main.go` 完成依赖装配并通过编译
```

### Task TSK-0513 <a id="hcr-tsk-0513"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:4`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 重构 `PeriodicCoordinator.triggerWindow` 与 `executeBatch`：实现基于最新观测时间的逐 (node, kind) 增量过期判定、失败观测冷却期与未观测节点优先调度
```

### Task TSK-0514 <a id="hcr-tsk-0514"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:5`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 实现单轮巡检有界任务配额（512 上限）、公平分页覆盖与基于 `EnqueuePeriodicDedupe` 和在池检测的防重入机制，避免自旋派发
```

### Task TSK-0515 <a id="hcr-tsk-0515"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:6`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 修复 `PeriodicCoordinator.Recover`：对 `NextDueAt` 为空或已在过去的启用计划，校正为 `now` 确保启动即刻巡检，消除非预期的 10 分钟推迟；更新 `executeBatch` 严密处理 `leaseLost` 熔断与防止并发覆盖
```

### Task TSK-0516 <a id="hcr-tsk-0516"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:7`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.5 修复 SQLite Repository 与内存测试 Repo 的 `UpdateBatch`：移除对 `owner` 和 `lease_until` 租约列的盲写覆盖，使租约与批次状态正交管理，保障多实例执行期间多次 `UpdateBatch` 不破坏已有租约
```

### Task TSK-0517 <a id="hcr-tsk-0517"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:11`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 修改 `web/src/features/probes/ProbesView.vue` 与 `probeTypes.ts`：文案明示“每 10 分钟巡检过期节点”及“每节点/类别有效期 = 配置周期”，展示完整的下次巡检时间
```

### Task TSK-0518 <a id="hcr-tsk-0518"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:12`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 在 `ProbesView.vue` 中修复空闲时不轮询问题，引入 15 秒低频保底轮询，并在 `useProbes.ts` 中增强立即触发后的批次与运行状态追踪
```

### Task TSK-0519 <a id="hcr-tsk-0519"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:13`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 补充前端单元测试，验证空闲低频轮询与立即触发跟踪行为，执行 `npm --prefix web test -- --run` 与 `npx vue-tsc --noEmit` 通过
```

### Task TSK-0520 <a id="hcr-tsk-0520"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:17`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 编写 `internal/application/probe/periodic_test.go` 针对性测试：覆盖启用即扫、周期缩短、有效/过期/未观测节点多类别增量判定
```

### Task TSK-0521 <a id="hcr-tsk-0521"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:18`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 完善假时钟启动恢复单测，验证过去过期与空 `NextDueAt` 启动即刻校正为 `now`；新增多实例租约在多次 `UpdateBatch`、续约与租约丢失下的回归测试
```

### Task TSK-0522 <a id="hcr-tsk-0522"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:19`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 运行全量后端门禁：`gofmt`, `go build ./...`, `go test -count=1 -timeout 120s ./...` 以及 `go test -race -count=10 -timeout 180s ./internal/application/probe -run TestPeriodicCoordinator_MultiInstanceCASContention` 与相关 probe/sqlite/http race 分包通过
```

### Task TSK-0523 <a id="hcr-tsk-0523"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:23`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 独立 Reviewer 代码审查通过（门禁保留未勾选）
```

### Task TSK-0524 <a id="hcr-tsk-0524"></a>

- **Source**: `openspec/changes/csp-incremental-expiry-probe-sweep/tasks.md:24`
- **Source Digest**: `a19410776168bb71a91ecea7b4c6cc88ec6d30a19dde28e688a6f3ae5165b349`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 独立 Critic 黑盒成品验收通过（门禁保留未勾选）
```

### Task TSK-0525 <a id="hcr-tsk-0525"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:3`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 1.1 [Executor: Domain Contract] 在 `internal/domain/` 增加 `ExitIdentity`、`IPRiskObservation`、`RiskPolicy`、`RiskDecision`、风险枚举和验证规则，明确 score/confidence 取值、禁止字段与 `unknown` 默认语义；以 domain unit tests 验证完整 IP、原始 payload 和 secret 字段没有公开 JSON contract，且无效 band/trait/policy 被拒绝；依赖既有 domain foundation，独占新增 IP risk domain 文件
```

### Task TSK-0526 <a id="hcr-tsk-0526"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:4`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 [Executor: Repository Schema] 按现有迁移编号在 `migrations/` 创建 append-only IP risk observations、provider settings、risk policy revisions 和必要分页查询索引；扩展 `internal/domain/ports.go` 与 `internal/repository/sqlite/` 的专属 IP risk repository；以临时 SQLite migration test 验证外键、索引、回滚、append-only 约束和 `foreign_key_check`，且 schema 不出现旧表或完整 IP 列；依赖 1.1，独占新迁移与 IP risk repository 文件
```

### Task TSK-0527 <a id="hcr-tsk-0527"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:5`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 1.3 [Executor: Security Contract Tests] 为 domain、repository 与 API-safe DTO 建立负向 redaction fixtures，覆盖 API key、cookie、URL query、完整 IPv4/IPv6、原始 JSON 和 error text；以 `go test ./internal/domain ./internal/repository/sqlite ...` 验证它们均不可持久化或出现在公开结果；依赖 1.1、1.2，独占 IP risk security test fixtures
```

### Task TSK-0528 <a id="hcr-tsk-0528"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:6`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 1.4 [Reviewer: Data Boundary Gate] 独立审查 1.1–1.3，复跑相关 Go tests，确认第三方事实、节点 capability verdict 与 local risk decision 三层未混用，且 schema/API 无敏感字段；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 1.1–1.3
```

### Task TSK-0529 <a id="hcr-tsk-0529"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:10`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.1 [Executor: Exit Identity Profile] 在 `internal/probe/profiles/` 与相应 application orchestration 中新增版本化 exit identity/IP risk profile 入口，复用被测节点 runtime 而不改变 baseline、geo、streaming、AI、speed 的 verdict 语义；以 fixture 验证 `ip_risk` contract drift、challenge、超时和 exit identity 缺失只产生 `unknown` 或 `error`；依赖 1.1，独占 `internal/probe/profiles/` 的 IP risk additions
```

### Task TSK-0530 <a id="hcr-tsk-0530"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:11`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.2 [Executor: Provider Port and Fake Adapter] 在专属 `internal/probe/iprisk/` 包定义 provider registry、versioned adapter port、响应 normalizer 和 deterministic fake provider，不写真实供应商密钥或网页 scraper；以 contract tests 验证缺 score、低 confidence、未知 trait、schema mismatch 与 provider conflict 不会被归为低风险；依赖 1.1、2.1，独占 `internal/probe/iprisk/`
```

### Task TSK-0531 <a id="hcr-tsk-0531"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:12`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.3 [Executor: Outbound-bound Client] 将 IP risk provider call 注入 `internal/probe/singbox/` 已有 runtime HTTP client，保证所有 exit identity 与 provider 请求走同一 outbound 并 `Proxy:nil`；以可控 proxy fixture 和环境变量测试验证宿主 `HTTP_PROXY`、直连及另一节点出口不会替代被测节点；依赖 2.1、2.2，独占 `internal/probe/singbox/` 的 IP risk additions
```

### Task TSK-0532 <a id="hcr-tsk-0532"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:13`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.4 [Executor: Budget, Cache and Observation Recorder] 实现 provider 的 explicit enablement、secret reference lookup、per-provider concurrency/rate/daily budget、deadline 和 `(logical_id, exit_identity_digest, provider, schema_version)` TTL cache；通过 observation application service 将仅摘要的结果 append 到库；以 fake clock tests 验证预算前拒绝、429、缺密钥、TTL、出口变更与取消清理，且网络请求不包在 SQLite transaction 内；依赖 1.2、2.2、2.3，独占 `internal/application/iprisk/`
```

### Task TSK-0533 <a id="hcr-tsk-0533"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:14`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.5 [Executor: Probe Run Integration] 把已启用的 IP risk task 接入 bounded probe queue、run lifecycle、idempotency 与审计，不让它覆盖既有 ProbeObservation；以 integration tests 验证单节点同类型排他、run cancellation、provider 未启用不发网、证据与 node logical ID 关联；依赖 2.4，独占 `internal/application/probe/` 的 IP risk integration files
```

### Task TSK-0534 <a id="hcr-tsk-0534"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:15`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.6 [Reviewer: Evidence and Egress Gate] 独立以 fake provider/proxy fixture 复跑 2.1–2.5，验证 outbound 实证、bounded resource use、cache key、cancel cleanup、provider drift 和脱敏；真实 provider 无授权时不得以网页抓取替代；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 2.1–2.5
```

### Task TSK-0535 <a id="hcr-tsk-0535"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:19`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 [Executor: Risk Decision Service] 在 `internal/application/iprisk/` 或专属 policy risk 子包实现 versioned policy 的 single-provider、all-must-allow、mapped-highest-risk 融合和 `allow|review|block|unknown` 决策，固定默认 `unknown=review`；以 table/fake-clock tests 验证 score bands 不重叠、缺失/冲突数据不变 low-risk、输入相同则 digest 和 reason code 稳定；依赖 1.1、1.2、2.4，独占 risk decision service 文件
```

### Task TSK-0536 <a id="hcr-tsk-0536"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:20`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 [Executor: Policy Binding API] 扩展风险 policy revision 的管理、审阅、激活、停用及策略组绑定 API，要求认证、CSRF、Idempotency-Key 和脱敏 audit；以 HTTP contract tests 验证未激活 policy 不影响成员、无效 filter/policy 受拒绝、secret reference 不出响应；依赖 3.1，独占 `internal/transport/http/` 中新增 `ip_risk*.go` handler
```

### Task TSK-0537 <a id="hcr-tsk-0537"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:21`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 [Executor: Resolver Admission Integration] 在 `internal/resolver/` 建立唯一的 risk-aware resolved snapshot，加入 policy revision、evaluated_at、decision digest 和 admitted/excluded node diagnostics；以 deterministic/property tests 验证 block 排除、review 的显式行为、未绑定 group 原语义及相同输入下 snapshot digest 稳定；依赖 3.1、3.2，独占 `internal/resolver/`
```

### Task TSK-0538 <a id="hcr-tsk-0538"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:22`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.4 [Executor: Publication Preflight] 在 `internal/application/publication/` 与专属 publication HTTP handler 实施 snapshot 风险重算和 preflight diagnostic；以 integration tests 验证 block 或不可发布 review 返回冲突、不会创建 partial publication、不会替换既有 immutable publication，且既有 publication 不因后续观察改变；依赖 3.3，独占 `internal/application/publication/` 与对应新增 handler 文件
```

### Task TSK-0539 <a id="hcr-tsk-0539"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:23`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.5 [Executor: Node Risk Read Model] 扩展 inventory/repository node read model，使列表和详情仅返回 `IPRiskSummary`，支持 risk decision、band、provider、status 和 policy revision 的服务端筛选再稳定排序分页；以 10,000 节点 SQLite fixture 验证 total、一页上限、filter SQL path 和 API response 不含完整 IP；依赖 1.2、3.1，独占 `internal/application/inventory/` 和 `internal/repository/sqlite/` 的 IP risk read-model files
```

### Task TSK-0540 <a id="hcr-tsk-0540"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:24`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.6 [Reviewer: Admission and Publication Gate] 从空库及风险 fixture 独立复跑 3.1–3.5，检查 resolver 唯一路径、preflight 原子性、未知默认策略、SQL server-side filtering、audit 和敏感字段；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 3.1–3.5
```

### Task TSK-0541 <a id="hcr-tsk-0541"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:28`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.1 [Executor: Risk Workbench Feature] 在未来 `web/src/features/` 的 nodes、policy 与 publications scope 实现风险摘要、unknown/stale/error 区分、可恢复 URL 筛选、高风险准入和 preflight diagnostics；只消费 API-safe summary；以前端 unit tests 验证 unknown 绝不显示低风险、筛选触发服务端 query、raw provider data 不进入 store；依赖 3.2、3.4、3.5，独占 `web/src/features/` 的 IP risk feature directories
```

### Task TSK-0542 <a id="hcr-tsk-0542"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:29`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.2 [Executor: Cross-layer Harness] 增加无秘密 end-to-end fixture，走 fake outbound → fake provider → risk observation → policy decision → resolver → publication preflight → node API/UI；以 `go test ./...`、`go test -race ./...` 与前端 build/test 命令验证，报告明确标出真实商业 provider 未调用；依赖 2.6、3.6、4.1，独占专属 IP risk integration fixtures
```

### Task TSK-0543 <a id="hcr-tsk-0543"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:30`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.3 [Reviewcommon: Final Engineering Gate] 在干净、只读 checkout 独立执行 4.2 的全部验证，逐项对照七份 delta specs、最小 diff、旧 CSP DAG 隔离、无 scraper/host-proxy fallback、无 secret/raw-IP leakage；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 4.2
```

### Task TSK-0544 <a id="hcr-tsk-0544"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:31`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.4 [Critic: Risk Failure-mode Gate] 对 provider 403/429、无 key、配额耗尽、出口突变、节点大量消失、风险结果冲突、policy 默认 unknown、publication 回滚和移动端筛选进行对抗复核；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 4.2
```

### Task TSK-0545 <a id="hcr-tsk-0545"></a>

- **Source**: `openspec/changes/csp-ip-risk-v1/tasks.md:32`
- **Source Digest**: `ea847b04acd8155603079e6dd33c98572148fd67de7053db3d5ad1f6deaf3ea9`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.5 [Planner: Controlled Enablement Decision] 汇聚 4.3 APPROVED、4.4 PASSED、测试报告、部署 secret reference 与预算配置审阅结果，请求 Einck 独立批准真实 provider 启用、production policy binding 或发布；未经明确授权不得发起真实 provider 查询、写入真实密钥、启动容器、切换流量或发布订阅；依赖 4.3、4.4
```

### Task TSK-0546 <a id="hcr-tsk-0546"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:3`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 核验官方 SpecKit 锁定版本 (`93989802b1401c99cc7de6e4ed76572f9cb662e8`) 与本地模板原始哈希，在 `.specify/templates/` 建立参考副本及 `provenance.json` 存根，明确声明保留 OpenSpec CLI。
```

### Task TSK-0547 <a id="hcr-tsk-0547"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:4`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 调研 DeepSeek Agent Notes 架构决策模式，统一收敛单事实源至 `docs/decisions/`（清理 `.agents/notes/` 重复镜像），明确记录 proposed 状态及否决备选方案。
```

### Task TSK-0548 <a id="hcr-tsk-0548"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:8`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 编写 `openspec/specs/inventory/spec.md`，规范化受控订阅、CoW 覆写、节点身份哈希及数据修剪边界 (`FR-INV-01` ~ `FR-INV-07`)。
```

### Task TSK-0549 <a id="hcr-tsk-0549"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:9`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 编写 `openspec/specs/policy/spec.md`，确立直接组引用与解包叶节点双核心能力，定义空组 PASS 兜底与动态全池解析 (`FR-POL-01` ~ `FR-POL-07`)。
```

### Task TSK-0550 <a id="hcr-tsk-0550"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:10`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 编写 `openspec/specs/probe/spec.md`，规范原生 Mihomo 执行引擎、多层周期解耦、单节点插队与健康基线四态判定 (`FR-PROB-01` ~ `FR-PROB-07`)。
```

### Task TSK-0551 <a id="hcr-tsk-0551"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:11`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 编写 `openspec/specs/publication/spec.md`，定义 Mihomo 拓扑导出、非 Mihomo 纯节点边界、免密公开导出与双令牌验证 (`FR-PUB-01` ~ `FR-PUB-06`)。
```

### Task TSK-0552 <a id="hcr-tsk-0552"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:12`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.5 编写 `openspec/specs/auth/spec.md`，规范管理端与导出链接解耦双开关、统一系统主令牌共享与明文敏感配置模型 (`FR-AUTH-01` ~ `FR-AUTH-04`)。
```

### Task TSK-0553 <a id="hcr-tsk-0553"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:13`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.6 编写 `openspec/specs/ui/spec.md`，定义紧凑自适应视口排版、健康筛选与分页交互、编辑抽屉与双语国际化 (`FR-UI-01` ~ `FR-UI-06`)。
```

### Task TSK-0554 <a id="hcr-tsk-0554"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:14`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.7 编写 `openspec/specs/README.md` 目录索引，通过 `openspec validate --specs --strict` 验证所有 6 份主干规范零错误通过。
```

### Task TSK-0555 <a id="hcr-tsk-0555"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:18`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 编写 `docs/specification/source-register.md`，建立四级证据确权体系，严格甄别用户原话，纠偏提案问题描述为历史变更分析。
```

### Task TSK-0556 <a id="hcr-tsk-0556"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:19`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 编制 `docs/specification/verification-commands.md`（编号 CV-001 ~ CV-041）与 `docs/specification/conformance-matrix.md`，替换 FR-PROB-07 伪 0-case 命令为真实用例，解耦命令与表格避免转义破坏正则，重新实机执行 41 条命令并生成收据 `/tmp/csp-conformance-real-tests-receipt.json`（178 次执行 / 137 个唯一用例，零 0-case 运行）。
```

### Task TSK-0557 <a id="hcr-tsk-0557"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:20`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 编制 `docs/specification/historical-requirement-register.md`，逐条提取 26 个 Change 63 份历史规范中的 259 项需求（HR-001 ~ HR-259）与 463 个场景，明确显式冲突与未验证存根；同步更新 `upstream-provenance-and-coverage.md`。
```

### Task TSK-0558 <a id="hcr-tsk-0558"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:21`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.4 编写 `docs/specification/maintenance-guide.md`，确立 37 项主干 FR 矩阵与 259 项 HR 库存的双层治理体系，制定长期演进与更新指南。
```

### Task TSK-0559 <a id="hcr-tsk-0559"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:25`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 编写 `docs/specification/clarification-questions.md`，删除 Q1.4 越过激活/风险准入的选项，纠偏 Q6 跨页全选为可选候选，规范 Q3、Q2、Q5，精准呈现 6 组核心业务待决选项。
```

### Task TSK-0560 <a id="hcr-tsk-0560"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:29`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 运行 OpenSpec 全量严格门禁检查（`openspec validate --specs --strict` 与 `openspec validate --changes --strict`），保持零错误退出码。
```

### Task TSK-0561 <a id="hcr-tsk-0561"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:30`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 提交完整文档工作树供独立只读审查机（Reviewer）进行交叉比对与审计复核（独立Reviewer最终PASS，凭证 `/tmp/csp-living-spec-review-accepted-20261009.json`，SHA-256 `838a270e824f82a093df1987ca807fab19fa19b70aa3a59f90d05d62520e05e9`）。
```

### Task TSK-0562 <a id="hcr-tsk-0562"></a>

- **Source**: `openspec/changes/csp-living-spec-consolidation/tasks.md:34`
- **Source Digest**: `929088c9f4120531bfd2a1d3a7e5d6cd8d43a8bba6aae0e92dd0fdb5b5d94908`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 6.1 向用户呈报 6 组核心业务待决问题清单，等待用户明确裁决后再启动业务代码落地（本任务待用户决策后开展，本阶段不勾选）。
```

### Task TSK-0563 <a id="hcr-tsk-0563"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/tasks.md:3`
- **Source Digest**: `6e6dd98d789edd7a5c1d9845dc289746d03c80aeb1047f0141193a241fafabad`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 **Task 1.1（后端：实现统一节点池双端优先级队列、定时去重入池、手动插队最前、`probing` 节点状态注入与 `GET /api/v1/probes/pool` 5 大统计指标接口）**（写集合：`internal/**`, `cmd/csp/**`）：
```

### Task TSK-0564 <a id="hcr-tsk-0564"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/tasks.md:17`
- **Source Digest**: `6e6dd98d789edd7a5c1d9845dc289746d03c80aeb1047f0141193a241fafabad`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 **Task 1.2（前端：重设计节点池检测状态看板展示 5 大核心指标、节点“检测中”动态状态徽章与筛选、实时池状态轮询联动）**（写集合：`web/src/**`）：
```

### Task TSK-0565 <a id="hcr-tsk-0565"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/tasks.md:35`
- **Source Digest**: `6e6dd98d789edd7a5c1d9845dc289746d03c80aeb1047f0141193a241fafabad`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 **Task 2.1（全仓汇聚自测与硬门禁）**：执行后端 `go test -count=1 ./...`、`go build ./...` 与前端 `npm test -- --run`、`npx vue-tsc --noEmit` 全部 exit 0；验证 `openspec validate csp-node-probe-pool-priority-queue --strict` exit 0。
```

### Task TSK-0566 <a id="hcr-tsk-0566"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/tasks.md:36`
- **Source Digest**: `6e6dd98d789edd7a5c1d9845dc289746d03c80aeb1047f0141193a241fafabad`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 **Task 2.2（独立 `reviewer` 终态审查）**：独立 `reviewer` 对照 `csp-node-probe-pool-priority-queue` 规格与最终 diff 完成只读代码与安全审计并出具 `PASS` 裁决。
```

### Task TSK-0567 <a id="hcr-tsk-0567"></a>

- **Source**: `openspec/changes/csp-node-probe-pool-priority-queue/tasks.md:37`
- **Source Digest**: `6e6dd98d789edd7a5c1d9845dc289746d03c80aeb1047f0141193a241fafabad`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 **Task 2.3（隔离预览实例准备与独立 `critic` 多视口真浏览器验收）**：在隔离环境（临时 SQLite 数据库、种子节点与探测队列状态、隔离构建产物、非生产端口）启动预览实例并通过健康检查，移交独立 `critic` 在桌面与移动多视口下对「节点池 5 大指标看板」、「节点检测中动态状态与筛选」、「手动检测插队最前与定时测速去重入池」执行真浏览器交互与视觉验收并取得 `PASSED`。
```

### Task TSK-0568 <a id="hcr-tsk-0568"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:3`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 冻结 `domain` 中 ProbeSchedule/ProbeBatch、NodeFilterSpec、FilterCondition、credential_version 与仓库接口，记录 API JSON 示例和有效/无效条件矩阵；以 domain 表驱动测试和 JSON 往返测试验证兼容。
```

### Task TSK-0569 <a id="hcr-tsk-0569"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:4`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 在隔离测试 DB 核验既有手动探测、库存和旧策略输出；对 `7289b4c` 的 `PROCESS-NAME` 回归用例做只读代码核验，若用户提供本次脱敏请求/日志则建立可重复的错误复现用例，否则将未知错误独立标记为待证据，不猜测修复；以基线/复现记录核对结论。
```

### Task TSK-0570 <a id="hcr-tsk-0570"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:8`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 在新顺序迁移和 `repository/sqlite` 增加计划、批次、运行关联、CAS 租约/唯一窗口、credential_version 观测字段与索引；以新旧隔离 DB 迁移/回滚备份恢复、双连接同窗口争抢、fencing、版本不符测试验证。
```

### Task TSK-0571 <a id="hcr-tsk-0571"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:9`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 在 `application/probe` 与 `cmd/csp/main.go` 接入 `time.Timer`、现有 runner/有界公平队列，完成当前激活库存全量分片、凭据 fail-closed、过期恢复/失租约中止、手动任务公平和停服 Drain；以 fake clock、双实例共享测试 DB、可控本地探测目标和生命周期测试验证“无重复批次/无漏报/无秘密/DB 关闭顺序”。
```

### Task TSK-0572 <a id="hcr-tsk-0572"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:10`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 在 `transport/http/probes.go` 接入计划查询/修改、批次分页/取消与脱敏审计，管理端授权和手动 API 保持兼容；以 HTTP 契约测试（未授权、非法预算、关闭计划、空仓库及有/无凭据）验证。
```

### Task TSK-0573 <a id="hcr-tsk-0573"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:14`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 在独立迁移（与包 A 的迁移编号/写集合先协商冻结，避免冲突）及 `repository/sqlite` 实现全局与组筛选存储、真实来源及最新观测批量加载与索引；验证旧配置默认恒真、无 N+1、版本未知旧观测失效和独立数据库事务一致读取。
```

### Task TSK-0574 <a id="hcr-tsk-0574"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:15`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 在 `application/policy`、`resolver`、`compiler`、`application/publication` 实现严格字段校验、激活/风险/准入→全局→组条件、动态/显式节点、父子上下文可编译投影、摘要与同源预览/预检/发布；以多父嵌套组、跨目标配置、不同过滤顺序、过期/未知/错误观测、空路由组阻断和旧策略字节级/语义回归用例验证。
```

### Task TSK-0575 <a id="hcr-tsk-0575"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:16`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 在 `transport/http/policies.go` 与全局筛选 admin 路由添加可选 `node_filter` 和全局筛选 GET/PUT，PATCH 省略/清空区别、错误字段/认证与预览诊断；以 HTTP 契约测试和完整配置生成核验不泄漏秘密、不引用不存在节点。
```

### Task TSK-0576 <a id="hcr-tsk-0576"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:20`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 对照 `internal/transport/http/probes.go`、`internal/domain/probe_schedule.go` 核实 `web/src/features/probes/{probeTypes,useProbes,ProbesView}.vue/ts`、`web/src/api/client.ts`：计划 GET/PUT、批次列表/详情/取消、`run_ids` 关联、失败/跳过及脱敏证据、启停和无库存/过期反馈；`ApiError` 的 401 走既有认证回调，其他 4xx/5xx 和网络错误在计划/批次读取处必须可见（现有 `loadSchedule`/`loadBatches` 吞错误且 `cancelBatch` 乐观写死 cancelled，应据服务端实际状态修正）。补足 Vitest 的真实 Vue 组件交互 + API mock（启用/停用、取消成功/失败、过期、空库存、401/403、无密钥泄漏），与现有 composable 测试一起退出 0；完成源码、接口、组件和集成自测前不勾。
```

### Task TSK-0577 <a id="hcr-tsk-0577"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:21`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 对照 `internal/transport/http/policies.go`、`internal/application/policy/types.go`、`internal/resolver/types.go` 核实 `web/src/features/policy/{policyTypes,usePolicy,PolicyView,PolicyEditorSheet,GroupCard}.vue/ts` 和发布预检页：全局及组级筛选 GET/PUT/POST/PATCH 的省略与显式清空语义、字段级错误、预览各层计数/排除原因/空路由组阻断；`loadGlobalFilter` 不得把 401/403/5xx 当作空筛选，UI 仅消费服务端诊断，不另造筛选引擎。补 Vitest 的 Vue 组件/API mock、键盘交互测试，并在隔离可访问预览中核实小屏实际可操作、无溢出/遮挡及编辑-预览-预检一致；所有适用测试退出 0 后勾，黑盒独立裁决仍归 5.2。
```

### Task TSK-0578 <a id="hcr-tsk-0578"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:25`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 保持 Go 每包 race 检查 `-race -count=3` 覆盖不变，勿把同一条全量 race 静默 337 秒遭外层运行时中断误判为 race 检出或成功：按 `go list ./...` 的**完整包清单**确定性拆成连续三组（各 9 包；当前 27 包），逐组串行执行 `go test -race -count=3 <组内所有包>`；每组将 stdout/stderr 和退出码保存在隔离临时日志/状态记录，标明开始与结束时间、包清单并显示阶段进度；必须在调用环境单次运行时预算内执行并获取真实子进程退出码。若某组超预算，使用系统已有 `timeout` 给组设置可见期限（其退出码 124/137 不代表 race 失败或 PASS），在未减少 `-race -count=3` 和未遗漏任何包的前提下把该组进一步拆小、串行重跑并记录原组超时及子组真实退出码；若整个受控运行环境无法提供足够时间，移交可持续的独立执行环境并保留未完成状态，绝不静默视为通过。核对组并集等于 `go list ./...` 且无重复无漏，三组及必要子组均真实 exit 0；对检出的 race/失败定点修复后重测。随后在隔离临时 SQLite DB 和受控本地探测目标验证周期/手动竞争、重启、全局→组筛选预览/预检/五目标编译一致；`go build ./...`、`go test ./...`、前端 `npx vue-tsc --noEmit`（项目当前无 `npm run type-check` 脚本）、`npm test -- --run` 或项目已有等价 Vitest 命令各真实 exit 0，保留命令/退出码/失败复现与修复证据。本项是施工自测门禁，不代替独立审查。
```

### Task TSK-0579 <a id="hcr-tsk-0579"></a>

- **Source**: `openspec/changes/csp-periodic-active-probes-global-group-filters/tasks.md:26`
- **Source Digest**: `1632c9263cffef966e3f0820e3fa7fc042e6927cde309c5b3adc2ce9fca54e59`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 先在严格无副作用隔离工作副本完成前端构建/预览：核对 `web/vite.config.ts` 的 `closeBundle` 仅当 `COPY_WEBASSETS=1` 才拷贝，显式 `NO_COPY_WEBASSETS=1`、使用隔离输出/副本，并比对原仓 `internal/webassets/dist` 前后基线；不得在原仓默认构建、不得连生产 17000/18080/生产卷。待 4.1/4.2/5.1 真实完成，独立 reviewer 对最终合流 diff、迁移、鉴权、凭据脱敏、出站、前后端 JSON 契约、全量测试证据作安全/代码复验并 PASS；准备者提供健康检查通过的**隔离 URL**，critic 针对周期探测启停/批次取消/失败反馈、全局→组筛选/空组阻断在实际桌面与小屏视口做只读 FLOW 和 VISUAL 双 PASS（截图与交互/几何证据）。任何 REJECTED/FAILED/BLOCKED/PRECONDITION_FAILED 或缺少隔离 URL 均保持本项 `[ ]`，按根因整改复验，不能以文档勾选替代验收。生产备份、部署、重启、端口及线上数据、Git 提交均不在本 Change 本地验收授权内。
```

### Task TSK-0580 <a id="hcr-tsk-0580"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:3`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 In `internal/domain/ports.go` and `internal/repository/sqlite/nodes.go`, implement `ListAll(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, error)` with unpaginated query execution, preserving subscription scope, active-only constraints, notice exclusions, and IP risk decision admissions. Verify with sqlite node repository tests.
```

### Task TSK-0581 <a id="hcr-tsk-0581"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:4`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 In `internal/application/publication/service.go:812` and `internal/application/policy/service.go:932`, replace `nodeRepo.List` with `nodeRepo.ListAll` in resolver snapshot provider and export compiler. Verify publication preflight and policy service tests pass.
```

### Task TSK-0582 <a id="hcr-tsk-0582"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:5`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 Create unit and integration fixtures with 250 active nodes where target nodes (e.g. Taiwan at 70-72, Canada at 78-80, and extra targets at > 100) are resolved. Verify that both export artifact compilation and policy graph validation resolve all target groups without empty group errors.
```

### Task TSK-0583 <a id="hcr-tsk-0583"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:9`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 In `internal/domain/probe.go` and `internal/domain/probe_health.go`, add `UndeterminedCount` to `ProbePoolStatus`, and establish strict mutual exclusion conservation: `total_count == healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count` and `available_count == healthy_count + degraded_count`. Verify domain health tests pass.
```

### Task TSK-0584 <a id="hcr-tsk-0584"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:10`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 In `internal/application/probe/service.go`, update pool status calculation to separate nodes with 0 observations (`UntestedCount`) from nodes with stale, revision-mismatched, or non-baseline-only observations (`UndeterminedCount`). Verify probe service tests pass.
```

### Task TSK-0585 <a id="hcr-tsk-0585"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:11`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 In `internal/repository/sqlite/nodes.go`, `internal/repository/sqlite/node_predicates.go`, and `internal/transport/http/nodes.go`, add full-scope server-side `health_status` filtering to `buildNodeFilterPredicates` and query parameter parsing, returning accurate paginated subsets. Verify HTTP nodes test suite passes.
```

### Task TSK-0586 <a id="hcr-tsk-0586"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:15`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 In `web/src/features/policy/PolicyEditorSheet.vue` and `web/src/features/policy/GroupCard.vue`, clarify explicit edges mode vs dynamic pool filter mode with visual badges and guidance, without altering the underlying algebra or deleting existing edges/filters. Verify policy component tests pass.
```

### Task TSK-0587 <a id="hcr-tsk-0587"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:16`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 In `web/src/features/policy/usePolicy.ts` and `web/src/features/policy/PolicyView.vue`, implement server-side pagination (default 50, max 100) and keyword search for policy groups, with pagination navigation controls. Verify `policy.test.ts` passes.
```

### Task TSK-0588 <a id="hcr-tsk-0588"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:17`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 In `web/src/features/policy/PolicyEditorSheet.vue`, implement asynchronous debounced search and paginated loading for candidate node selection, preventing silent truncation beyond 100 nodes. Verify editor sheet tests pass.
```

### Task TSK-0589 <a id="hcr-tsk-0589"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:18`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.4 In `web/src/features/policy/PolicyView.vue`, separate the full topology graph rendering from the paginated group cards list, ensuring topology validation reflects all groups regardless of the current card page. Verify `PolicyView.test.ts` passes.
```

### Task TSK-0590 <a id="hcr-tsk-0590"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:22`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 In `web/src/features/nodes/nodeView.ts`, remove the dangerous fallback `if (values.includes('available')) return 'healthy'`, enforce baseline-authoritative health evaluation, and introduce the `undetermined` category. Verify `nodeView.test.ts` passes.
```

### Task TSK-0591 <a id="hcr-tsk-0591"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:23`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 In `web/src/features/probes/useProbes.ts` and `web/src/features/probes/ProbesView.vue`, remove the `while (page <= 20)` waterfall loop, consume server pool status metrics, update KPI cards, and add the undetermined category filter. Verify `probes.test.ts` passes.
```

### Task TSK-0592 <a id="hcr-tsk-0592"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:24`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 In `web/src/App.vue`, update the desktop `<aside>` element with `sticky top-0 h-screen shrink-0 overflow-y-auto` so it remains anchored during page scrolling. Verify layout in desktop view.
```

### Task TSK-0593 <a id="hcr-tsk-0593"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:25`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.4 In `web/src/App.vue` and `web/src/features/policy/PolicyEditorSheet.vue`, implement body scroll locking (`overflow: hidden`) when mobile modal sheets or drawers are open. Verify mobile interaction in tests.
```

### Task TSK-0594 <a id="hcr-tsk-0594"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:26`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.5 In `web/e2e/multidevice.test.ts`, run multi-viewport tests across Desktop (1280x800), Landscape (667x375), Mobile (375x667), and Mobile Large (392x872), ensuring zero horizontal overflow, visible navigation, and accessible targets.
```

### Task TSK-0595 <a id="hcr-tsk-0595"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:30`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 Run full repository deterministic compile and test gates: `go test -v ./internal/...` (exit code 0) and `cd web && npm run type-check && npm test` (exit code 0).
```

### Task TSK-0596 <a id="hcr-tsk-0596"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:31`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 Execute independent read-only reviewer verification against the final Git diff and OpenSpec change contract.
```

### Task TSK-0597 <a id="hcr-tsk-0597"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:32`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 Deploy isolated preview build, run health check, and execute Critic visual inspection across the four standard viewports.
```

### Task TSK-0598 <a id="hcr-tsk-0598"></a>

- **Source**: `openspec/changes/csp-policy-membership-probe-state-pagination/tasks.md:33`
- **Source Digest**: `ea00920107cd4762e4e05cc40ccc8aeec90cb7c862004923ba30ae5aed19e27b`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 5.4 Synchronize web assets with `COPY` flag preservation, execute database backup, perform authorized `git_commit`, and verify zero-downtime service launch.
```

### Task TSK-0599 <a id="hcr-tsk-0599"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:3`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 重构 `internal/application/inventory/service.go` 的 Reconcile 流程：在外部订阅拉取失败、HTTP 异常、内容解析错误或提取结果为空时，记录审计事件并将状态标记为 stale/failed，全量保留该订阅既有的活跃节点与策略边，通过单元测试 `go test -v -run TestReconcile_FetchOrParseFailure_PreservesNodes ./internal/application/inventory/...` 验证节点不被删除或失活。
```

### Task TSK-0600 <a id="hcr-tsk-0600"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:4`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 废除 `internal/application/inventory/service.go` 中的全局孤儿节点批量失活 SQL（`deactivateOrphansSQL`），将来源清理严格限定在当前 `subscription_id` 授权作用域内，通过单元测试 `go test -v -run TestReconcile_PruneScoped_ProtectsManualAndOtherSourceNodes ./internal/application/inventory/...` 验证来源修剪不波及独立手工节点与其他订阅节点。
```

### Task TSK-0601 <a id="hcr-tsk-0601"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:5`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 在同一订阅来源作用域内实现稳定节点身份匹配：优先匹配源端稳定标识，无标识时基于同一来源内名称与协议唯一组合匹配；存在重名或歧义时保守保留并新增，严禁跨来源按名称合并凭据，通过单元测试 `go test -v -run TestReconcile_IdentityMatching_DisambiguationAndIsolation ./internal/application/inventory/...` 验证跨来源隔离与同一来源更新行为。
```

### Task TSK-0602 <a id="hcr-tsk-0602"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:6`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3.1 根因整改：调用 `computeScopedLogicalID` 隔离新来源/歧义身份，同 host/port 跨来源不同 UUID/password 绝对禁止 upsert 覆盖；多源共享节点连接变更时执行 copy-on-write 隔离并保守保留政策边，补充同 host+port+协议但 UUID/password 不同的 A/B 刷新与 revision 独立自测试。
```

### Task TSK-0603 <a id="hcr-tsk-0603"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:7`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 实现严格的连接版本控制：当节点的真实连接要素发生变更时单调递增 `connection_revision` 并将当前健康探测状态置为 unknown/pending；若仅为显示名称或 JSON 键排序变化则保持版本不变，通过单元测试 `go test -v -run TestReconcile_ConnectionRevision_IncrementOnRealChange ./internal/application/inventory/...` 验证版本自增与历史观测失效逻辑。
```

### Task TSK-0604 <a id="hcr-tsk-0604"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:11`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 编写历史误伤节点排查与恢复脚本工具，基于归因特征（2026-09-29T00:52 批次全局孤儿失活、规范配置合法、未被用户手工禁用）精准定位待恢复节点，通过单元测试与 dry-run 验证其排查逻辑不包含用户主动禁用的节点。
```

### Task TSK-0605 <a id="hcr-tsk-0605"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:12`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 在 SQLite 备份副本上完整演练恢复脚本，验证其幂等性、可回滚性与受影响行数输出，确保重复执行受影响行数为 0 且回滚脚本能无损恢复，记录结构化演练审计凭据。
```

### Task TSK-0606 <a id="hcr-tsk-0606"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:13`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2.1 根因整改：使用 CLI 对新创建沙箱 DB 副本执行真实演练，生成 output-plan/output-report 并持久化至 logs/recovery（含 plan hash、审批模拟标识、首次 apply、重复 apply 0 行、rollback 报告与 sha256），归因 960 节点标记为 heuristic ambiguous 不可批准生产。
```

### Task TSK-0607 <a id="hcr-tsk-0607"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:17`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 重构 `internal/probe/singbox/builder.go`：将底层的 L4 网络协议（`tcp`/`udp`）与传输层包装（`ws`/`grpc` 等）严格解耦，出站配置中的 `Network` 字段正确填入 L4 协议且支持 UDP，通过单元测试 `go test -v -run TestSingboxBuilder_L4TransportDecoupling_UDP ./internal/probe/singbox/...` 验证 Shadowsocks、VLESS、VMess、Trojan、Hysteria2 等协议出站网络参数的正确性。
```

### Task TSK-0608 <a id="hcr-tsk-0608"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:18`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 规范 Reality 协议参数校验：对 `RealityPublicKey`（`pbk`）执行标准及 URL-Safe Base64 解码与 32 字节严格长度校验，对 `RealityShortID`（`sid`）执行偶数长度与纯十六进制校验（奇数长度或非法字符明确拒绝，严禁截断补零），通过单元测试 `go test -v -run TestSingboxBuilder_RealityParametersValidation ./internal/probe/singbox/...` 验证边界情况拦截。
```

### Task TSK-0609 <a id="hcr-tsk-0609"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:19`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 编写探针实际构建能力测证测试，针对底层使用的 Sing-box 核心库真实测证其对 `xhttp` 等协议特性的构建支持情况，通过 `go test -v -run TestSingboxLibrary_ActualFeatureSupport ./internal/probe/singbox/...` 确立实际支持基线。
```

### Task TSK-0610 <a id="hcr-tsk-0610"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:23`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 改造 `internal/application/probe/safe_dialer.go`：捕获底层客户端构建失败原始错误并进行分类归因，输出脱敏的结构化诊断信息，严格剥除密码、UUID、URL query 及公私密钥，通过单元测试 `go test -v -run TestSafeDialer_RedactedDiagnostics ./internal/application/probe/...` 验证敏感凭据未泄露。
```

### Task TSK-0611 <a id="hcr-tsk-0611"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:24`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 维持 SSRF 严格防护、私网 IP 拦截与 TLS 证书严格校验安全边界，通过单元测试 `go test -v -run TestSafeDialer_SSRFAndPrivateNetProtection ./internal/application/probe/...` 验证针对回环地址与私有网段的防御不被放宽。
```

### Task TSK-0612 <a id="hcr-tsk-0612"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:25`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 在探针观测状态机中将“本地配置构建成功”与“远程网络握手健康”严格分离，本地构建成功但远端网络超时或失败时真实记录网络失败状态，通过单元测试 `go test -v -run TestProbe_BuildSuccessVsHandshakeFailureSeparation ./internal/application/probe/...` 验证健康态不被虚假上报。
```

### Task TSK-0613 <a id="hcr-tsk-0613"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:29`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 编写前后端路由双向核对测试，静态比对前端 API 客户端调用点与后端 Chi 路由注册表，确保覆盖 45+ 路由端点无遗漏，通过测试 `go test -v -run TestRouteInventory_BilateralCompleteness ./test/e2e/...` 验证双向映射完整。
```

### Task TSK-0614 <a id="hcr-tsk-0614"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:30`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 编写覆盖 Auth 与 Settings 模块的隔离全接口语义测试，通过 `go test -v -run TestAPISemantic_AuthAndSettings ./test/e2e/...` 验证登录登出会话、双鉴权独立开关持久化及权限拦截。
```

### Task TSK-0615 <a id="hcr-tsk-0615"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:31`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 编写覆盖 Subscriptions、Nodes、Policies 模块的隔离全接口语义测试，通过 `go test -v -run TestAPISemantic_SubscriptionsNodesPolicies ./test/e2e/...` 验证数据库持久化副作用、来源隔离、幂等性与节点连接补丁版本递增。
```

### Task TSK-0616 <a id="hcr-tsk-0616"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:32`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3.1 根因整改：补全真实 HTTP 及业务副作用测试：PATCH /api/v1/nodes/{id}、GET/PATCH/PUT /api/v1/policies/groups/{id}、POST /api/v1/policies/groups/{id}/edges、GET /api/v1/revisions、GET /api/v1/revisions/{id}，以及 policy groups risk-policy GET/PUT/DELETE 与 /api/v1/policy/* 别名。
```

### Task TSK-0617 <a id="hcr-tsk-0617"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:33`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.4 编写覆盖 Probes 与 Publications 模块的隔离全接口语义测试，通过 `go test -v -run TestAPISemantic_ProbesAndPublications ./test/e2e/...` 验证探针任务状态机取消、幂等创建、发布创建、预检、不可变导出（`/publish/v1/{id}` 与 `/p/{id}`）及发布撤销。
```

### Task TSK-0618 <a id="hcr-tsk-0618"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:34`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.4.1 根因整改：补全 DELETE /api/v1/publications/{id} 真实测试；补全 admission rules GET/POST/DELETE、ip-risk groups evaluate/members、ip-risk policies GET/deactivate/review 等端点实测，实测 /yaml 及 /script 真实 410 错误包络，重新盘点 chi 真实路由与 docs/api_semantic_matrix.md 计数。
```

### Task TSK-0619 <a id="hcr-tsk-0619"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:38`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.1 运行全项目统一静态编译 `go build ./...`、全量后端单元测试 `go test ./...` 以及前端类型检查 `npm run type-check`，验证全部门禁退出码为 0，无编译与类型错误。
```

### Task TSK-0620 <a id="hcr-tsk-0620"></a>

- **Source**: `openspec/changes/csp-preserve-inventory-and-verify-business-results/tasks.md:39`
- **Source Digest**: `1acc81db3dbe0c8bccdf4b73ed8e9bb46f3920e5658d65b0d0e81e7ae4ec17b4`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.2 整理受控部署预案文档与脚本，明确数据库备份验证、测试副本演练、健康检查端点（`/healthz`、`/readyz`）验证、可用回滚镜像构建以及禁止重启 Hermes/Gateway 约束，由主脑最终核准。
```

### Task TSK-0621 <a id="hcr-tsk-0621"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/tasks.md:3`
- **Source Digest**: `633ffe517a14d51c0b5760440587ee1d378138814030885e3da569b0fb1d8f2f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 **Task 1.1（后端：打通 `NodeView` 最新探测状态与订阅源读模型 + 修复全量探测分页截断、任务预算与 Speed 探测开关）**（写集合：`internal/application/inventory/**`, `internal/application/probe/**`, `internal/transport/http/{nodes,probes}*.go`）：
```

### Task TSK-0622 <a id="hcr-tsk-0622"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/tasks.md:11`
- **Source Digest**: `633ffe517a14d51c0b5760440587ee1d378138814030885e3da569b0fb1d8f2f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 **Task 1.2（前端：彻底重构探针引擎为以节点为核心的人类直觉测速工作台 + 节点台账探测状态联动）**（写集合：`web/src/features/probes/**`, `web/src/features/nodes/**`, `web/src/locales/**`）：
```

### Task TSK-0623 <a id="hcr-tsk-0623"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/tasks.md:23`
- **Source Digest**: `633ffe517a14d51c0b5760440587ee1d378138814030885e3da569b0fb1d8f2f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 **Task 2.1（全仓汇聚自测与硬门禁）**：执行后端 `go test -count=1 ./...`、`go build ./...` 与前端 `npm test -- --run`、`npx vue-tsc --noEmit` 全部 exit 0；在隔离临时目录验证前端生产构建并确保受保护现场干净；执行 `openspec validate csp-probe-engine-human-centric-redesign --strict` exit 0。
```

### Task TSK-0624 <a id="hcr-tsk-0624"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/tasks.md:24`
- **Source Digest**: `633ffe517a14d51c0b5760440587ee1d378138814030885e3da569b0fb1d8f2f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 **Task 2.2（独立 `reviewer` 终态审查）**：独立 `reviewer` 对照 `csp-probe-engine-human-centric-redesign` 规格与最终 diff 完成只读代码与安全审计并出具 `PASS` 裁决。
```

### Task TSK-0625 <a id="hcr-tsk-0625"></a>

- **Source**: `openspec/changes/csp-probe-engine-human-centric-redesign/tasks.md:25`
- **Source Digest**: `633ffe517a14d51c0b5760440587ee1d378138814030885e3da569b0fb1d8f2f`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 **Task 2.3（隔离预览实例准备与独立 `critic` 多视口真浏览器体验与视觉验收）**：在隔离环境（临时 SQLite 数据库、种子订阅与真实探测观测数据、隔离构建产物、非生产端口）启动预览实例并通过健康检查，移交独立 `critic` 在桌面与移动多视口下对重构后的「探针引擎工作台」与「节点台账联动」执行真浏览器交互、一键全量/单节点测速、筛选排序、测速详情抽屉及视觉可用性验收并取得 `PASSED`。
```

### Task TSK-0626 <a id="hcr-tsk-0626"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:3`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 冻结并实现稳定脱敏原因分类及共享健康判定输入契约，修复 sing-box VLESS vision/Reality 运行时配置与导出一致性，保持严格 TLS/私网拒绝；以现有 sing-box 配置校验、离线握手/拒绝零出站测试验证有效配置不再误报缺凭据。
```

### Task TSK-0627 <a id="hcr-tsk-0627"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:4`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 实现安全拨号器/runner 对缺配置、安全拒绝、构建错误、DNS/超时/传输错误的可识别脱敏记录；以本地 HTTP/TLS 回环夹具和分类回归测试验证所有类别有凭据不泄漏、无证据 SS `transport_error` 不被臆断。
```

### Task TSK-0628 <a id="hcr-tsk-0628"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:5`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 用 SQLite 非破坏性 migration 和节点/观测仓储实现逐节点单调连接版本及观测关联版本；以隔离临时 SQLite 验证 UpsertBatch/UpdateNode、停启切换、重启、多实例读取、旧 NULL 观测和显示名/来源/观测更新不触发版本变化，禁止接触生产库。
```

### Task TSK-0629 <a id="hcr-tsk-0629"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:6`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 完成 A 的针对性离线测试、格式化和 `go build ./...`（退出码 0）；保持类别原因和版本契约供后续 B/C 使用。
```

### Task TSK-0630 <a id="hcr-tsk-0630"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:10`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 复用 A 的共享状态函数，将 inventory 的节点/台账视图与 probe/service.go pool 统计统一为仅新鲜、当前版本、可判定 baseline 的整体线路判断；能力类别独立展示、旧/阻断/模糊观测 fail-unknown、未知计入兼容的未确认桶；以固定假时钟、多 kind 混合观测和两入口一致性测试验证总数及各互斥桶。
```

### Task TSK-0631 <a id="hcr-tsk-0631"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:11`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 校准现有 node/pool UI 类型与状态文字所依赖的读模型语义（不改 C 的触发路径），用旧 API payload 和有效/过期/阻断 baseline 回归测试确认无隐式必填字段或订阅过滤变更；完成 B 的 `go build ./...` 与相关后端测试（退出码 0）。
```

### Task TSK-0632 <a id="hcr-tsk-0632"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:15`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 将定时轻量状态扫描改为 1 分钟、纠正启动/遗留 next_due，按照缺失/到期或节点连接版本变化（含重新激活）决定逐节点/类别待测；保持 5 分钟失败冷却、严格 512 任务预算、公平排序、CAS 租约及队列去重；以假时钟+临时 SQLite、多实例并发和前后版本对照测试验证无网络全量/观测自旋。
```

### Task TSK-0633 <a id="hcr-tsk-0633"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:16`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 调整现有 trigger API/service：手动状态刷新重读并执行相同增量判定且不推进 next_due，与全量测速独立，提供兼容且可识别的排队/无待测结果；用 HTTP handler 测试验证无任务、并发、错误传播、连续点击不会延迟自动扫描；完成 C 的 `go build ./...` 和相关后端测试（退出码 0）。
```

### Task TSK-0634 <a id="hcr-tsk-0634"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:20`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 更新探针控制台一分钟状态扫描文案、未知/无法检测标识及手动“刷新状态”先重读再 POST 增量触发并持续跟踪的交互，保持独立“全量测速”按钮；用 Vitest（现有 runner，必要时使用已装 MSW/fixture 而非真实网络）验证有待测/无需检测/失败、后台 GET 不自动 POST、连续刷新不重置正常扫描。
```

### Task TSK-0635 <a id="hcr-tsk-0635"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:21`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 检查台账与探针控制台对同一节点的视觉/文字状态及窄视口无溢出；运行项目已有前端测试与类型检查（例如 `npm --prefix web run type-check` 若脚本存在，或 `npx --prefix web vue-tsc --noEmit`），退出码 0。
```

### Task TSK-0636 <a id="hcr-tsk-0636"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:25`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 对最终合并状态执行 `go build ./...`、`go test ./...`、前端类型检查及相关 Vitest（全部退出码 0），并运行 `openspec validate csp-probe-health-minute-state-sweep --strict`；核实仅本 Change 工件和获授权实现被修改。
```

### Task TSK-0637 <a id="hcr-tsk-0637"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:26`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 由独立 Reviewer 只读审查最终 diff：安全零出站、VLESS 参数、新旧观测归因、pool/node 同口径、迁移后连接版本与 1 分钟扫描预算/冷却/租约、手动与自动时钟分离；取得明确 PASS 裁决后才勾选本项。
```

### Task TSK-0638 <a id="hcr-tsk-0638"></a>

- **Source**: `openspec/changes/csp-probe-health-minute-state-sweep/tasks.md:27`
- **Source Digest**: `80bbe0b01db99658f4fc33f7df6aff321e58459b47ee24bd90b850174844cb14`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 如有已获授权的隔离预览且准备者交付健康 URL/实例 JSON，由独立 Critic 黑盒验收分钟增量、手动刷新与全量区别、状态文字与视口交互并取得 PASSED；无隔离 URL 不派 Critic，不勾选本项或宣称成品验收完成。
```

### Task TSK-0639 <a id="hcr-tsk-0639"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/tasks.md:3`
- **Source Digest**: `683dbfc2f095e47792f5d2d4fb4875772904c50890b771ba986eb3b75355ebc1`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 **Task 1.1（后端探针契约真实性收紧、Speed 完整传输计时与同 Run 同节点客户端安全复用）**（写集合：`internal/application/probe/**`, `internal/probe/profiles/**`）：
```

### Task TSK-0640 <a id="hcr-tsk-0640"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/tasks.md:17`
- **Source Digest**: `683dbfc2f095e47792f5d2d4fb4875772904c50890b771ba986eb3b75355ebc1`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 **Task 1.2（SQLite 最新探测观测 DB 内按 `node+kind` 精确选最新、复合索引保障与离线基准/回归测试）**（写集合：`internal/repository/sqlite/**`）：
```

### Task TSK-0641 <a id="hcr-tsk-0641"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/tasks.md:26`
- **Source Digest**: `683dbfc2f095e47792f5d2d4fb4875772904c50890b771ba986eb3b75355ebc1`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 **Task 2.1（全仓汇聚自测与静态编译硬门禁）**：运行 `go build ./...`、`go test -count=1 ./...`、`go test -run=^$ -bench=BenchmarkListLatestByNodes -benchmem ./internal/repository/sqlite/...` 及 `openspec validate csp-probe-truth-and-efficiency --strict`，确认全部 exit 0 且安全不变量（Server DNS Pinning、SSRF、Redirect 禁止、EvidenceDigest 签名字段）回归测试全绿。
```

### Task TSK-0642 <a id="hcr-tsk-0642"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/tasks.md:27`
- **Source Digest**: `683dbfc2f095e47792f5d2d4fb4875772904c50890b771ba986eb3b75355ebc1`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 **Task 2.2（独立 `reviewer` 终态代码与安全审计）**：由独立 `reviewer` 对照 `csp-probe-truth-and-efficiency` 规格契约与最终 diff 执行只读代码与安全审查，确认无伪阳性遗留、无客户端泄漏、无全表历史拉取且未引入未经证实的私有规则，出具 `PASS` 裁决。
```

### Task TSK-0643 <a id="hcr-tsk-0643"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/tasks.md:28`
- **Source Digest**: `683dbfc2f095e47792f5d2d4fb4875772904c50890b771ba986eb3b75355ebc1`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 **Task 2.3（整改施工：探针节点表格/卡片宽度边界与真实视口证据）**：修复桌面表格操作列右端裁切：详情与重测按钮 bounding rect 必须完整落在可见表格/card 范围内；长无空格节点名和来源 badge 不得撑宽桌面表格或移动卡片。补充组件布局约束与操作回归测试。隔离 Chromium 在 375×667、392×872、667×375、1280×800 验证每个操作 rect 包含于可用可见区域及 card/table bounds，row `scrollWidth <= clientWidth` 或明确验证可访问滚动方案；测量长节点名及长来源 badge 的 bbox 和容器边界，按钮可见可点击。详情真实点击；重测仅路由拦截模拟并断言无公网 POST。复用健康隔离预览，仅在私有副本构建，保全合成 SQLite 与 token；报告、脚本/命令、geometry.json、原始 PNG 及 SHA256 存入权限 0700 的 `/tmp/csp-probe-truth-remediation-evidence-20260928`。禁止修改主仓 embedded dist、生产服务/数据。运行针对性前端测试、`go build ./...`、`go test -count=1 -timeout 120s ./...`、前端测试/type-check、隔离构建与 OpenSpec strict，全 exit 0 且几何通过后方可勾选。
```

### Task TSK-0644 <a id="hcr-tsk-0644"></a>

- **Source**: `openspec/changes/csp-probe-truth-and-efficiency/tasks.md:29`
- **Source Digest**: `683dbfc2f095e47792f5d2d4fb4875772904c50890b771ba986eb3b75355ebc1`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 **Task 2.4（独立终态复验与真实视口验收）**：独立 reviewer 对照同一 Change、最终 diff、整改报告/原始截图及几何 JSON 复核代码与安全边界；critic 在隔离预览复验四种视口及详情/重测触达，无水平溢出后分别出具裁决。不得将施工自测替代独立审查或验收。
```

### Task TSK-0645 <a id="hcr-tsk-0645"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:3`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 保留先前既有工作树约 731–734 项规模记录；本轮不能用旧计数推导文件归属。禁止 reset/restore/clean、生产端口 17000/18080、生产卷、未经授权删除/提交。
```

### Task TSK-0646 <a id="hcr-tsk-0646"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:4`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 已有响应字节 limit+1 局部测试记录；不代表真实探针或最终版本通过。
```

### Task TSK-0647 <a id="hcr-tsk-0647"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:5`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 【旧证据不构成前置汇聚】撤销旧发布/主题/视觉综合完成声明。先前 37 PNG 的视觉 FAILED 是问题证据；其后 reviewer 工程 PASS/critic FLOW+VISUAL PASSED 的时序不能补足此 Change 的全量门禁→reviewer→critic。建立本轮同一最终代码版本、七路由、网络 fixture 与分阶段签名的唯一身份台账，旧证据仅作对照。
```

### Task TSK-0648 <a id="hcr-tsk-0648"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:6`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 【原仓资产取证与恢复，先判归属再操作】以只读清单/逐文件 SHA 复核 `/tmp/csp-critic-stage/original_dist` 18 文件、其中 17 原有 assets 与主仓逐字节相同及完整聚合基线 `bac5a0f424b263f8ad2f538368863200045978f58a0c9d8f9d89f1701866a655`；复核当前 22 文件/聚合 `6ec421559324992339314b05071a0e44f3fc5c91ee8b3339852ea10953ddf0f6`、两版 index 引用、当前/旧 web/dist、mtime/来源。为四个新增文件尤其 `index-CuRBRMN8.js/index-CY02MDnG.css` 确认使用/归属和保留理由；**未获授权不删/移任何未跟踪资产**。取证后由主脑确认逐文件恢复和合法额外资产安置范围；获授权施工者保全字节后定点恢复，复算旧 18 文件（含 index）SHA 与合法新文件清单、所有 index 引用可解析、原仓其他文件/status 未变化；歧义不勾、递交决策证据。
```

### Task TSK-0649 <a id="hcr-tsk-0649"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:7`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.5 【阻断再污染的构建门禁】审查 `web/vite.config.ts` `closeBundle` 默认复制与所有调用点，约定/必要时施工实现安全 opt-in 或隔离输出，不破坏正式 go:embed；在带 sentinel 的独立临时工作区测试默认构建/显式嵌入的边界。所有前端构建及 reviewer 复验只在隔离副本或显式 `NO_COPY_WEBASSETS=1` 下进行，前后检查主仓 `internal/webassets/dist` 全量路径+每文件 SHA、Git status 与业务源码未改变；`npm --prefix web run build` 在主仓默认环境禁止。记录测试命令/退出码 0、无原仓写入证据；若决定不改钩子也必须以确定性测试证明操作规范可执行且 reviewer 无副作用。
```

### Task TSK-0650 <a id="hcr-tsk-0650"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:11`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 【真实网络总门禁，确实待实现/证明】只有 2.1a–c 真实六协议/12 transport 逐行 inbound→目标 204、合法 TCP/UDP socket/证书/Host、负例、race `-count=3`、`go build ./...` 全部通过后才勾；旧 Trojan 局部和非 race 204 不替代。
```

### Task TSK-0651 <a id="hcr-tsk-0651"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:12`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1a 【已有局部代码但最终版缺证明】保全已有 Trojan/TLS/TCP `SafeNodeDialer` + `fetch.DefaultPolicy` 公网 IP pin→测试专属 sing-box inbound→`httptest.Server` 204、可信临时 CA/SAN/SNI/密码和双方 socket 证据；最终源码复验 `go test ./internal/application/probe ./internal/probe/singbox -run 'Test.*Trojan.*(Handshake|204)' -count=1`、`go build ./...` exit 0；fixture 不入生产。
```

### Task TSK-0652 <a id="hcr-tsk-0652"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:13`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1b 【待根因归因/消除】取得 SHA 前缀 `263766475...` 原 race 日志完整路径/SHA/双栈，区分 fixture 普通计数器 race、sing-box v1.14.0 HTTP2/gRPC late-reader 与 VLESS/httpupgrade EOF/端口释放。用 `sync/atomic` 或锁同步 targetHits/observedHost；优先官方兼容修复，必要时固定 upstream commit 最小 late Read create patch，Write 不等 RoundTrip Setup；服务端先等 body 的确定性夹具证实先写后读/取消有界。六协议 + Trojan/VLESS/VMess × ws/http/httpupgrade/grpc 逐行真实 204、Host/SNI/授权 socket，`go test -race -count=3 ./internal/probe/singbox -run 'Test(ProtocolMatrixRealHandshake204|TransportMatrixRealHandshake204|TrojanRealTLSHandshakeAnd204)'` 与 `go build ./...` exit 0；不跳行、不重试到绿。
```

### Task TSK-0653 <a id="hcr-tsk-0653"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:14`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1c 【负例确实待实现】从真实 `SafeNodeDialer` 授权凭据版本/受控 resolver/public IP pin 到测试专属 registry/loopback mapper，表驱动公私 DNS、重绑、metadata/RFC6598/IPv4-mapped、SS/Trojan/VLESS/VMess 错密码、SAN/CA/SNI、ws/http/httpupgrade Host、grpc 不可表达的独立 Host 拒绝、Hy2/TUIC UDP、目标私网/redirect、过期授权/取消；负例无目标到达/非法 socket/available/泄密，合法正例 204/Host/SNI/IP。`go test -race -count=3 ./internal/application/probe ./internal/probe/singbox` + `go build ./...` exit 0，保留每例证据；不以 BuildOptions 静态断言代替。
```

### Task TSK-0654 <a id="hcr-tsk-0654"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:15`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 【已有局部通过记录，非最终合并签名】进程级真实 Bearer + Idempotency-Key POST `/api/v1/probes/runs`→service→Runner→queue→callback/RunID，`os.Pipe`/`Cmd.ExtraFiles` 同步 SIGTERM 成功分支 callback 持久化→Drain→真实 DB.Close→clean→exit0 和超时无提前 Close/clean→自然非零退出；旧记录已通过，不因本轮规划撤销历史事实，但最终 diff 后必须在 4.1 重跑。
```

### Task TSK-0655 <a id="hcr-tsk-0655"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:16`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 【已有局部通过记录，非最终合并签名】queue Wait/Drain 局部证据保留，race/回调在 4.1 随最终变更重跑。
```

### Task TSK-0656 <a id="hcr-tsk-0656"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:20`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 【Publications 待证】`PublicationsView.vue` header Create/Download/Refresh 可换行且 375×667、392×872 真实 viewport 均可操作；五 targets 包括 Quantumult X 在本地可见/可滚及键盘可达，main 不被 746px 内容增宽；409 长文/预览只在局部单元缩放。Vitest 409→修订→预览/五目标/错误分类 + Playwright 实际 rect、点击和原图。
```

### Task TSK-0657 <a id="hcr-tsk-0657"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:21`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 【其余页面待证】Policy action row/Admission Rules 长 ID，Subscriptions grid/secret_ref/Popover 面板，Dashboard 两 badge 内容高度，Settings 有/无 token 中英状态一次显示、主题下拉；用既有 Vue/Tailwind/DaisyUI 与成熟定位能力修正，Vitest+Playwright 验证 375/392 边界、点击/键盘、菜单和 badge，无父级 hidden 假修复。
```

### Task TSK-0658 <a id="hcr-tsk-0658"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:22`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 【七路由待证】`App.vue` 从 navItems 给 Dashboard + 其他六路由清楚可辨的移动入口，点击/键盘全可达，dock 不压末尾动作，桌面导航/主题不回退；组件测试 + 375×667/392×872 真浏览器逐入口 rect、路由和间距。
```

### Task TSK-0659 <a id="hcr-tsk-0659"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:23`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.4 【自测须重跑】最终版在隔离工作区执行 `./node_modules/.bin/vue-tsc --noEmit -p tsconfig.json`、`npm test -- --run` 或既有 Vitest、在受控复制开关/隔离路径 `npm run build`、Playwright 三视口交互与几何，全部 exit 0；主仓资产前后路径/哈希相等。任何 CSS 不能在 jsdom 假量，编译红灯施工机原地修复。
```

### Task TSK-0660 <a id="hcr-tsk-0660"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:27`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 【冻结最终 diff 后必须重跑】1.4/1.5、2.x、3.x 施工和各自自测完成，再记录 Git status + 未跟踪源码/资产的逐文件 SHA 源清单、变更范围和依赖版本；同一冻结源执行 `go test ./...`、`go test -race -count=3 ./internal/probe/queue ./internal/application/probe ./internal/probe/singbox`、`go build ./...` 与 web vue-tsc/Vitest/隔离 build/Playwright，所有 exit 0，保存协议逐行 204/安全负例、停服事件、构建前后原仓资产不变。后续任何源变更重跑此项，旧 PASS 不可继承。
```

### Task TSK-0661 <a id="hcr-tsk-0661"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:28`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 【旧 4.2 勾选撤销：污染前旧快照不证明最新同源】不可仅沿用 `/tmp/csp-preview-refresh-20260924` 的 `sourceTreeSha256Before/After=9f359...`、binary `75b7...` 或旧 served hash。准备者用 4.1 冻结源码在新的唯一 `/tmp` 隔离工作区构建 frontend dist→隔离副本 embed dist→go:embed binary，启动独立 loopback URL/临时 DB/凭据/受控 fixture；记录源清单 SHA、三段产物 SHA、index 引用、实际 HTTP 响应 SHA、process/URL/health/auth、fixture 状态与截图/trace SHA。375×667、392×872、1280×800 对 Publications 409→修订→预览/五 targets、Policy/Subscriptions 菜单、Dashboard badge、Settings/主题、七路由及 401/403/500/网络分类实测 main/rect/scrollHeight/局部滚动末项，采本实例原始 PNG/DOM trace/几何 JSON。证明主仓原有资产/生产端口/卷不变；旧实例及旧 PNG 仅作为历史对照。健康/认证/同源或新原图缺一不得派 critic。
```

### Task TSK-0662 <a id="hcr-tsk-0662"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:29`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 【次序门禁须重新执行】只在 4.1 完成、4.2 同源新实例健康且冻结 diff 未变后，由独立只读 reviewer 审 Change/最终 diff/资产安全/协议/停服/页面并给 APPROVED，不在主仓运行有副作用 build；其后准备者将隔离实例 JSON、URL、原 PNG/trace 路径和 SHA 交独立 critic，用 `read` 直读 PNG + 只读 Playwright/像素/DOM，对同一实例出具 `CRITIC_FLOW_VERDICT: PASSED`、`CRITIC_VISUAL_VERDICT: PASSED`。此前 reviewer PASS 与 critic 双 PASS 时序不符合此门禁，不复用；任何 REJECTED/FAILED/BLOCKED/PRECONDITION_FAILED 回根因整改且源变后回 4.1。
```

### Task TSK-0663 <a id="hcr-tsk-0663"></a>

- **Source**: `openspec/changes/csp-publication-theme-probe-recovery-v1/tasks.md:30`
- **Source Digest**: `967f75cfed34c891c8e1ba3c6354f9158782a5d2dd0e061b3fc9781a0fb0caab`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.4 【终态仍未完成】核对 1.3–1.5、2.x、3.x、4.1–4.3 全部有本版证据，按隔离实例 owner 受控回收，确认原仓资产和生产无侵扰；否则只报告 PARTIAL/待授权事实，不勾总任务、不同步/归档/提交。另一 Pi 治理 Change 的真机 3.1–3.3 继续未完成，本地替代只记录其真实范围，绝不假勾物理验收。
```

### Task TSK-0664 <a id="hcr-tsk-0664"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:3`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 更新 `internal/fetch/fetch.go` 中的默认 UA 为 `clash-verge/v2.5.6`，显式设置 `Accept: */*`，保留 `Accept-Encoding: gzip, deflate` 与单订阅自定义 UA 优先覆盖，并通过针对性单元测试验证
```

### Task TSK-0665 <a id="hcr-tsk-0665"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:4`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 编写 `internal/fetch/fetch_test.go` 针对 Verge 请求头的自动化验证用例（默认 UA、自定义 UA 覆盖、Accept 头验证），运行并通过测试
```

### Task TSK-0666 <a id="hcr-tsk-0666"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:8`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 审计 Go `net/http` Proxy `CONNECT`、HTTP 正向代理与 TLS `Host`/SNI 模型；在 `internal/fetch/policy.go` 中限定显式 `http://` 代理协议（`socks5`、`https` 等不支持的 scheme 一律失败闭合），白名单 `AllowedProxyHosts` 与 `AddAllowedProxy` / `IsProxyHostAllowed` / `ValidateProxy` 严格按规范化的 `http://<canonical-host>:<canonical-port>` 精确匹配，明确拦截端口省略、端口前导零（如 `:07890`）、代理主机名尾随点（如 `host.docker.internal.` / `127.0.0.1.`）、恶意 userinfo、IPv6 Zone ID 及混淆 IP（八进制/十六进制/整型/短 IP/IPv4-mapped IPv6），彻底废除对上游 mihomo ACL 的安全依赖
```

### Task TSK-0667 <a id="hcr-tsk-0667"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:9`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 在 `internal/fetch/fetch.go` 中实现应用层强制绑定经过公网校验的目标 IP 给代理：每一跳本地预解析并校验全部目标 IP；HTTP 正向代理传递已校验公网 `pinnedIP:port` 绝对 URL 并保留原 `Host` 头；HTTPS 代理 `CONNECT` 到已验证的公网 `pinnedIP:port` 并在 TLS 握手中验证原域名证书与设置原域名 SNI（严禁关闭证书校验）；`301/302/303/307/308` 每跳重新解析、重新校验公网 IP 并重新绑定给代理；如实说明并测试依赖代理域名路由场景的路线变化与 TOCTOU 边界
```

### Task TSK-0668 <a id="hcr-tsk-0668"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:10`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 在 `internal/fetch/fetch_test.go` 中编写离线恶意 HTTP 代理与代理侧 DNS 投毒防御测试：验证代理捕获的 HTTP 绝对 URL 和 HTTPS `CONNECT` authority 永远是已校验公网 IP（绝不见未校验域名），源站收到的 `Host` 与 TLS SNI 仍为原域名且证书严格校验，302 重定向至内网被拒绝、公网多跳重定向每跳重绑 IP，本地 `httptest` 模拟代理侧 DNS 投毒指向 `127.0.0.1` 不得访问内网
```

### Task TSK-0669 <a id="hcr-tsk-0669"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:11`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 在 `internal/fetch/fetch_test.go` 中补充协议混淆豁免阻断（`socks5://`、`https://` 同端口 7890 拒绝）、端口前导零、尾随点、IPv6 规范化、混淆 IP、userinfo 及自定义 `fetch_proxy_ref` 私网端点严禁绕过的全链路测试
```

### Task TSK-0670 <a id="hcr-tsk-0670"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:15`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 在 `internal/application/inventory/service.go` 中增加 `defaultFetchProxy` 字段与 `WithDefaultFetchProxy` 选项，并在 `ReconcileSubscription` 中实现单订阅级 > 全局受控代理 > 直连的分级优先级选择
```

### Task TSK-0671 <a id="hcr-tsk-0671"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:16`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 在 `cmd/csp/main.go` 中增加 `CSP_FETCH_PROXY` 环境变量与 `-fetch-proxy` 命令行参数解析，将受控代理配置注入 `fetch.Client` 与 `inventory.Service`
```

### Task TSK-0672 <a id="hcr-tsk-0672"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:17`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 在 `internal/application/inventory/service_test.go` 中增加关于全局代理注入与分级选择优先级的自动化测试，验证成功与失败情况下的审计脱敏记录
```

### Task TSK-0673 <a id="hcr-tsk-0673"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:21`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 修正 Compose 配置跟踪契约：更新版本受控的 `docker-compose.example.yml` 增加 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts`；在任务与设计中明确宿主机被忽略的 `docker-compose.yml` 需遵循核验、备份与投产规范，严禁尝试跟踪或强制提交已忽略文件
```

### Task TSK-0674 <a id="hcr-tsk-0674"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:22`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 执行本地端对端与 httptest 隔离模拟测试，验证代理故障分类、审计记录安全性，且不触发生产真实拉取、不触碰生产 mihomo 配置与宿主机 Compose
```

### Task TSK-0675 <a id="hcr-tsk-0675"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:23`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 运行全量静态编译与测试门禁：`gofmt`、`go build ./...`、`go test -count=1 -timeout=120s ./...`、变更包 `-race` 测试、前端 186 项测试 + `vue-tsc` 类型检查及 `openspec validate --strict` 严格校验
```

### Task TSK-0676 <a id="hcr-tsk-0676"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:27`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 独立代码与安全审查（Reviewer 复验应用层目标 IP 强制绑定、精确 scheme+host+port 白名单、测试覆盖与门禁凭据）
```

### Task TSK-0677 <a id="hcr-tsk-0677"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:28`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 独立成品视觉与黑盒验收（Critic 针对隔离预览实例完成 FLOW 与 VISUAL 双验收，出具 PASSED 裁决与多视口度量报告）
```

### Task TSK-0678 <a id="hcr-tsk-0678"></a>

- **Source**: `openspec/changes/csp-subscription-proxy-and-verge-headers/tasks.md:29`
- **Source Digest**: `952644bf6820bb33fcca00933149eb249ca8171dfdcf2258066a40923c186975`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 生产环境受控投产与健康检查（待审查与验收归档后，在明确授权下按部署手册执行生产 Compose 更新与验证）
```

### Task TSK-0679 <a id="hcr-tsk-0679"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/tasks.md:3`
- **Source Digest**: `ccf99c9630fb914e8c5da84b78af41f09aa436a89bef8609d4a1bfc242e86414`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 **Task 1.1（后端：拓扑校验修复 + 编译器非 Mihomo 纯节点格式导出忽略规则 + 预检同步）**（写集合：`internal/**`）：
```

### Task TSK-0680 <a id="hcr-tsk-0680"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/tasks.md:8`
- **Source Digest**: `ccf99c9630fb914e8c5da84b78af41f09aa436a89bef8609d4a1bfc242e86414`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 **Task 1.2（前端：拓扑校验请求规范化 + 全站完整中文化 + 发布目标节点格式文案更新）**（写集合：`web/src/**`）：
```

### Task TSK-0681 <a id="hcr-tsk-0681"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/tasks.md:16`
- **Source Digest**: `ccf99c9630fb914e8c5da84b78af41f09aa436a89bef8609d4a1bfc242e86414`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 **Task 2.1（全仓汇聚自测与硬门禁）**：执行 `go test -count=1 ./...`、`go build ./...`、前端 `npm test -- --run`、`npx vue-tsc --noEmit` 全部 exit 0；使用隔离输出目录验证前端生产构建并确保受保护现场干净；执行 `openspec validate csp-topology-i18n-and-node-targets --strict` exit 0。
```

### Task TSK-0682 <a id="hcr-tsk-0682"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/tasks.md:17`
- **Source Digest**: `ccf99c9630fb914e8c5da84b78af41f09aa436a89bef8609d4a1bfc242e86414`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 **Task 2.2（独立 `reviewer` 终态审查）**：独立 `reviewer` 对照 `csp-topology-i18n-and-node-targets` 规格与最终 diff 完成只读代码与安全审计并出具 `PASS` 裁决。
```

### Task TSK-0683 <a id="hcr-tsk-0683"></a>

- **Source**: `openspec/changes/csp-topology-i18n-and-node-targets/tasks.md:18`
- **Source Digest**: `ccf99c9630fb914e8c5da84b78af41f09aa436a89bef8609d4a1bfc242e86414`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 **Task 2.3（隔离预览实例准备与独立 `critic` 多视口真浏览器验收）**：在隔离环境（临时 SQLite 库、隔离构建产物、非生产端口）启动预览实例并通过健康检查，移交独立 `critic` 执行全站中文 UI、策略树拓扑校验、Mihomo 完整配置导出及 sing-box/Surge/Quantumult X 纯节点格式导出（忽略规则）的真浏览器全链路与多视口视觉验收并取得 `PASSED`。
```

### Task TSK-0684 <a id="hcr-tsk-0684"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:3`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 记录 HEAD、`git status --short` 分类、`web/`/`internal/webassets/`/`csp` 关键哈希、Docker 服务与卷归属，并与三个进行中 Change 的代码归属对照；验证产出只读基线清单、允许修改文件集合和冲突停工规则。
```

### Task TSK-0685 <a id="hcr-tsk-0685"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:4`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 审核 `web/vite.config.ts` 与 `web/e2e/auth-real-server.spec.ts` 的构建副作用，在不会写入当前 `csp` 或 `internal/webassets/dist` 的独立测试副本构建并启动动态端口+临时 DB 的 Go 服务；验证副本 `/healthz` 返回 200、工作树与运行中的容器/卷哈希及状态未变。
```

### Task TSK-0686 <a id="hcr-tsk-0686"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:5`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 在独立服务中准备不含真实凭据的 open/token 双模态测试数据与可控失败响应；验证测试账号可用、脱敏流程有效且任何写请求均未发往共享服务端口 18080/17000。
```

### Task TSK-0687 <a id="hcr-tsk-0687"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:6`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 用隔离服务复核历史报告的 settings 底部间距、probes 标题、订阅菜单及认证状态回显，核对当前预期而非旧测试文案；验证输出故障台账，每项标注 P0/P1/P2、确定复现/未复现/测试过期、归因线索及复验优先级。
```

### Task TSK-0688 <a id="hcr-tsk-0688"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:10`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.1 核对现有 Change、预览接口调用和测试文件归属，固定 `POST /api/v1/publications/preview` 与模板生成的合法/边界/无效输入、成功与非成功响应以及写集合；验证前后端对同一契约达成一致，冲突文件明确串行归属，且已有 726 项改动不被覆盖。
```

### Task TSK-0689 <a id="hcr-tsk-0689"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:11`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.2 【包 A；在 2.1 后，可与不重叠包并行】修复预览 500 与模板生成边界的后端实现及定向契约测试，避免可预期输入错误成为未说明 500；验证合法/边界/无效输入、实际失败响应和隔离临时服务上的预览结果，生产 18080/17000 与卷无写入。
```

### Task TSK-0690 <a id="hcr-tsk-0690"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:12`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.3 【包 B；与 A 不同文件时可并行】补齐 `web/src/locales/messages.ts` 中实际使用但缺失的 130+ i18n 键的中英双语，核对七入口、抽屉和错误文案；验证键集合无遗漏、中文/英文切换均不显示裸 key，记录检查命令及退出码。
```

### Task TSK-0691 <a id="hcr-tsk-0691"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:13`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.4 【包 C；与共用视图/组件编辑串行】修复异常/空状态引导、重试与鉴权恢复，前端预览的错误展示须在 2.1 后与包 A 契约一致；验证 401/403/409/500/网络失败各有正确恢复路径，重试按真实响应更新且失败不显示成功。
```

### Task TSK-0692 <a id="hcr-tsk-0692"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:14`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 2.5 【包 D；与 C 共用文件则串行】修复 PolicyView 拓扑选中与详情/编辑抽屉、保存/取消和刷新联动；验证成功/失败/取消流程在隔离实例中的 UI 与服务端状态一致，抽屉可关闭且不遗留错误选择。
```

### Task TSK-0693 <a id="hcr-tsk-0693"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:18`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.1 在包 A–D 合并后增补/执行定向测试和隔离服务下的七路由回归，仅使用 `ssh xiaomi-phone "/data/local/einck/script/ubuntu_chroot.sh run '<cmd>'"` 进入设备 Ubuntu chroot ARM64，调用 `/root/.cache/ms-playwright` 的 Chromium Playwright；验证 392×872、375×667、1280×800 下无未处理错误/裸 key，关键交互具备可访问名称与可见焦点、点击范围可靠且邻近控件可区分，实际滚动末项不被底 Dock 遮挡，长抽屉关闭及提交可到达且无不可恢复横向溢出；记录操作/遮挡证据、命令、退出码、构建和 headless 模式。不得用统一像素阈值代替交互复验；不得使用 ADB、USB 或点亮屏幕；不得在当前树运行会覆盖 `csp`/`internal/webassets/dist` 的构建。
```

### Task TSK-0694 <a id="hcr-tsk-0694"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:19`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.2 独立审查最终 diff、契约一致性、原有工作区保护、隔离/脱敏与 A–D 的测试证据；验证出具 APPROVED 或 REJECTED 裁决、独立复验命令及退出码，驳回项限范围返工。
```

### Task TSK-0695 <a id="hcr-tsk-0695"></a>

- **Source**: `openspec/changes/csp-ui-device-governance-v1/tasks.md:20`
- **Source Digest**: `7ab32c94b25884a86d19a6f2405e1b40d4d05dd0869c286ce6a04177bbf67f86`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.3 汇总已解决问题、剩余风险及 headless 验收边界；验证报告不声称实屏、软键盘或物理触控通过，生产容器/数据卷未变；部署或重启仍须单独授权。
```

### Task TSK-0696 <a id="hcr-tsk-0696"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:3`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 由有权策略角色协调旧 `csp-ui-device-governance-v1` 的 spec/design/tasks 三处统一 44px 门禁冲突，改为语义点击区域、无遮挡及可观测操作验收，保留设备通道和三视口；旧 Change 独立严格校验通过后才算完成。本项只代表规划协调，不代表实现或浏览器验收。
```

### Task TSK-0697 <a id="hcr-tsk-0697"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:4`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 实施机对拟编辑的每个文件记录哈希、既有内容和归属，复核 HEAD `7f827e2`/起始 726 项脏基线并限定写集合；验证 `git status --short` 与局部 diff 不包含既有工作丢失，不执行 reset/restore/checkout/clean。当前规划只核查了有限文件，未获得覆盖每个目标的施工基线证据，不可勾选。
```

### Task TSK-0698 <a id="hcr-tsk-0698"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:5`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 对上述硬编码清单逐项标注“可移除重复布局值/有理由的局部呈现值/必须保留的功能或测量值”，确定共享弹层容器职责及例外清单；验证清单与代码引用对应，`NodesView` 动态虚拟高度及业务常量不进入清理范围。当前只读抽查仍见共享组件与三处页面的重复高度表达式，尚无逐项分类/例外清单，不可勾选。
```

### Task TSK-0699 <a id="hcr-tsk-0699"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:9`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 【包 A，独占 `web/src/style.css`、`web/tailwind.config.js` 和 `web/src/App.vue`】将全局 44px/24px 的强制规则改为明确语义且不依赖统一尺寸的交互与布局策略，修复 Dock 对内容的动态避让和 safe-area；验证窄屏/桌面 Dock 末项可读可点、控件焦点可见、无全局固定触控尺寸门禁且其他主题无回退。保留既有勾选事实；最终验收仍待 3.1–3.3。若需编辑包 B/C 文件，暂停并串行协调。
```

### Task TSK-0700 <a id="hcr-tsk-0700"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:10`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 【包 B，独占 `web/src/ui/ResponsiveGrid.vue`、`Drawer.vue`、`DrawerCard.vue`、`ModalDialog.vue`、`ConfirmModal.vue`】对卡片与弹层布局复用流式原语，使长文本/缩放/窄屏下头、内容、脚可滚动可操作；验证代表性组件测试或浏览器场景没有横向溢出、关闭与提交按钮可达，不把现有固定值仅换成另一个固定值。保留既有勾选事实；只读核查仍见共享弹层中的重复视口公式，须按 1.3/2.3 及 3.1–3.3 复核，不将勾选误认为设计已完成。
```

### Task TSK-0701 <a id="hcr-tsk-0701"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:11`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 【包 C，独占 `web/src/features/policy/PolicyEditorSheet.vue`、`web/src/features/probes/ProbeEvidenceSheet.vue`、`web/src/features/publications/PublicationsView.vue`，共享弹层 API 更改须先与包 B 的单写者串行协调】按已冻结的共享弹层容器契约迁移页面调用：容器集中处理可用视口、安全区、定位、头/内容/脚滚动与关闭；页面传内容/动作及有理由的局部变体，预览长内容随容器滚动，不再另起多组 `min/calc/dvh/vh` 高度公式。登记确需保留的局部尺寸理由；验证三处长内容可滚动、主操作可达、不破坏真实服务响应展示。当前三处仍有独立高度公式，未经代码整改与复验不得勾选。与旧设备治理 Change 冲突的 Policy 编辑和 Publications 预览文件须先确认单写者，否则包 C 延后。
```

### Task TSK-0702 <a id="hcr-tsk-0702"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:15`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 合并 A–C 后增补或更新定向回归，执行 `web` 现有测试及不覆盖当前 `csp`/`internal/webassets/dist` 的受控构建；验证失败和退出码有记录、原 17 files/114 tests 基线不退步，拒绝所有控件同一像素阈值或字面量扫描即通过/失败的伪门禁。
```

### Task TSK-0703 <a id="hcr-tsk-0703"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:16`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 在明确授权的隔离服务与 SSH→Ubuntu chroot ARM64 Chromium Playwright 中完成七入口的 375×667、392×872、1280×800、长文案/缩放、主题抽样及 Dock/抽屉/表单交互回归；验证报告含命令/退出码、构建、视口、headless 模式、遮挡/溢出与焦点/点击证据，不向生产 18080/17000 或数据卷写入，不用 ADB/USB/实屏。
```

### Task TSK-0704 <a id="hcr-tsk-0704"></a>

- **Source**: `openspec/changes/csp-ui-hardcode-governance-v1/tasks.md:17`
- **Source Digest**: `cce20fe39f31e75da119180f240add45b2609971d1645c1f7397080b4c2d2e33`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 由独立审查角色核对新旧 Change 一致性、最终 diff、原有 726 项脏工作、例外清单和测试证据，出具 APPROVED 或 REJECTED 及独立复验退出码；未通过不得宣布完成或归档，部署与重启仍须另获授权。
```

### Task TSK-0705 <a id="hcr-tsk-0705"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:7`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 读取同一Change apply contextFiles；重读Git/运行归属/实际构建输入及合法凭据可用性，保持既有改动；冻结本包run/epoch、精确文件集合、脱敏基线/验收选择、备份恢复及发布计划。不改变原始意图包或历史run。
```

### Task TSK-0706 <a id="hcr-tsk-0706"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:8`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 在cmd/csp-live-acceptance/main.go与runner完整整改：汇聚fail-closed正文/JSON/HTTP断言，准确bool/pagination/订阅source集合核验（含来源集合排序精确保全），刷新错误与未知库存不绿，node_logical_ids+baseline+deadline及独立observations/真实终态协议（严格<=2分批串行与目标/revision/healthy强校验），凭据暂停不冒充业务通过，token-file/report文件安全校验（禁symlink/非regular/权限超标），脱敏/private scratch/report写入错误与清理。同步两份运行手册，保留既有业务保护，不改生产鉴权或服务接口迎合错CLI。
```

### Task TSK-0707 <a id="hcr-tsk-0707"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:9`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 同一施工会话用testing/httptest/t.TempDir及现有e2e SQLite完成design完整正负fixture矩阵（CLI真实入口而非仅API fixture），验证错字段不会扩大探针、空结果不PASS、等计数换ID也能发现库存损失、同ID丢失来源反例、探针<=2分批串行全覆盖、外来节点/错revision/unreachable失败反例、token-file/report安全反例、secret金丝雀无泄漏、私有scratch与故障清理。所有fixture禁生产/外部订阅/真实模型/QQ/密钥出口。
```

### Task TSK-0708 <a id="hcr-tsk-0708"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:10`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 执行go build ./...、go test ./...、bash -n scripts/run_authenticated_acceptance.sh、CLI/e2e定点命令与本Change strict，均exit0；输出绑定真实run/pkg/epoch的executor final结构化报告及commands。编译报错原地按行号修复，无进展caller_ping；不带错误交工。
```

### Task TSK-0709 <a id="hcr-tsk-0709"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:14`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 未参与施工的reviewer只读对照同一Change/最终diff，独立重跑静态/全量测试和关键故障fixture，审核fail-closed协议、redaction、安全备份恢复/回滚/预算及本包真实合同；输出独立final报告（报告ID: `rep_rev_pkg_csp_release_and_acceptance_1_r2`，裁决: `PASS`，exit 0）。未PASS不得执行生产副作用。
```

### Task TSK-0710 <a id="hcr-tsk-0710"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:18`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 以已审查冻结源码树构建并记录真实image/input manifest，隔离临时DB验证；紧邻替换前在线fresh备份DB/库存/身份配置与Compose/env（私有权限0700/0600，落地耗时0.216s），验证完整性及基于最终快照的独立隔离受管scratch恢复演练（`managed_scratch_final_snapshot_drill_report.json`），pin/save当前807549镜像为基线标签`clash-sub-parser-app:rollback-pre-03f2646-807549162ceb`并离线归档（`csp-image-rollback-807549162ceb.tar.gz`）；记录精确回滚步骤，保持卷/双loopback/代理/DB verifier，仅no-deps/no-build替换目标app并验证就绪/双端口/公网TLS入口（`https://sub.einck.top` 200 OK）/库存/身份（镜像重建绑定03f2646，运行二进制be2091与原运行态相同，CLI整改为独立工具）。
```

### Task TSK-0711 <a id="hcr-tsk-0711"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:19`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.2 仅有合法受管且匹配现有verifier的凭据后，执行带身份正文语义、至多3现有enabled订阅各1次刷新及至多6既有active节点baseline真实握手/探针；design建议10分钟总预算、GET10秒/刷新45秒、deadline遵守现有TTL。记录实际run/observations/库存集合及脱敏摘要；当前因缺少用户合法投递令牌，Acceptance Runner输出exit 2（PAUSED_CREDENTIALS_REQUIRED，报告: `csp-live-acceptance-post-replacement-report.json`），明确保持未勾选，PARTIAL/BLOCKED绝不伪造PASS。
```

### Task TSK-0712 <a id="hcr-tsk-0712"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:20`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 3.3 按fresh实际历史证据重新分类；仅因果充分、唯一身份、有效来源且排除手动禁用/删除的独立审查白名单允许事务恢复，记录实际数量（可0）及受保护歧义/外部待决项。当前证据链下因果充分恢复白名单严格为0，保持983个未知节点Fail-Closed安全保护，未执行任何节点复活，保持未勾选，不把未完成恢复假勾选。
```

### Task TSK-0713 <a id="hcr-tsk-0713"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:24`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.1 独立reviewer复验实际发布/业务/安全证据、必要恢复结果及全部尚未完成事项；确认构建/运行image、fresh备份可恢复、库存身份保持、实网成功和失败均真实、无秘密，输出本包最终报告。前置代码审查或公共健康不可代替此项。
```

### Task TSK-0714 <a id="hcr-tsk-0714"></a>

- **Source**: `openspec/changes/finish-csp-live-validation-and-recovery/tasks.md:25`
- **Source Digest**: `3e1ba13a60c898bcf657d3e6eccb215644e57a4c7ea2130f5a3ac1d3c6cdacfc`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 4.2 母体核验真实当前run合同、全部包报告及官方status；保留旧UNVERIFIED，实际业务满足才报告PASS。核算实施文件patch_hash及contract_revision_hash，按授权做必要Git提交/终态通知；bridge遗留Change须在其各自根独立收口，不跨根补勾选。本规划阶段不执行提交/QQ/归档。
```

### Task TSK-0715 <a id="hcr-tsk-0715"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:3`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 新增 Migration `000016_node_source_history.sql`，建立独立不可变历史账本表 `node_source_history`，定义联合唯一索引与 `ON DELETE SET NULL` 外键级联；运行单测验证 migration runner 顺利应用且具有幂等性。
```

### Task TSK-0716 <a id="hcr-tsk-0716"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:4`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 在 `internal/domain/` 中新增 `NodeSourceHistory` 实体、`RelationState`、`AttributionCause` 与 `AttributionStatus` 类型及常量；以单元测试验证实体字段校验与 JSON 脱敏规范。
```

### Task TSK-0717 <a id="hcr-tsk-0717"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:5`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 在 `internal/repository/sqlite/` 中实现 `NodeSourceHistoryRepository`，包含单事务批量插入、按节点查询历史及按订阅查询历史方法；编写集成单测验证外键置空与观测时间倒序排序。
```

### Task TSK-0718 <a id="hcr-tsk-0718"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:9`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 改造 `internal/application/inventory/service.go` 的订阅刷新剪枝逻辑，在 `DELETE FROM node_sources` 物理移除前原子写入快照至 `node_source_history`（`cause = 'refresh_removed'`）；编写针对性单测验证节点下线时历史事实被固化且节点 `active` 保持不变。
```

### Task TSK-0719 <a id="hcr-tsk-0719"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:10`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 完善抓取失败、启用/停用与用户覆盖生命周期隔离：验证抓取失败时不删除实时成员亦不伪造历史；订阅状态切换不触发冗余历史；用户覆写（`node_overrides`）不污染上游来源血统；编写覆盖单测。
```

### Task TSK-0720 <a id="hcr-tsk-0720"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:11`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 改造订阅删除生命周期处理：在删除订阅前快照留存该订阅下全部节点的关联记录至 `node_source_history`（`cause = 'subscription_deleted'`）；编写针对性测试验证订阅删除后历史记录保留脱敏标签但 `subscription_id` 为 NULL。
```

### Task TSK-0721 <a id="hcr-tsk-0721"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:15`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 在 `cmd/csp/` 与 `internal/application/inventory/` 中实现 `csp recover-source-history --manifest <path> [--dry-run]`，只读解析带 SHA-256 校验的脱敏清单并批量原子写入 `node_source_history`；编写单测验证 dry-run 0 写入与正式回填的幂等性。
```

### Task TSK-0722 <a id="hcr-tsk-0722"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:16`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 在恢复工具中实现严格的资产不变量前置与后置断言门禁（总节点 1014、活跃节点 31、`node_sources` 31、Scope E 不变）；编写单测验证任何状态漂移或凭据篡改时立即触发 Fail-Closed 事务回滚。
```

### Task TSK-0723 <a id="hcr-tsk-0723"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:17`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 实施丢弃碰撞凭据记录的隔离逻辑，确保首选记录写入 verified，被丢弃异凭据记录仅作为丢弃证据留存不附着当前节点；以单测验证碰撞隔离。
```

### Task TSK-0724 <a id="hcr-tsk-0724"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:21`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 在 `internal/transport/http/` 中实现 `GET /api/v1/nodes/{id}/source-history` 端点，返回当前所属源与历史归属列表及综合 `attribution_status`；编写 HTTP Handler 单测验证鉴权、脱敏及响应结构。
```

### Task TSK-0725 <a id="hcr-tsk-0725"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:22`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 在 `web/src/` 的节点详情 Drawer 中增设“来源归属与历史”只读面板，展示当前源与历史观测事实（含证据类型与观测时间），并确保不暴露敏感凭据与原始 URL；运行前端 `npm run type-check` 与组件测试。
```

### Task TSK-0726 <a id="hcr-tsk-0726"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:26`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 编写全链路端到端集成测试，使用沙箱数据库验证脱敏清单回填、刷新剪枝归档、订阅删除级联以及 API 审计完整闭环；验证前后库存 Scope E 严格为 31。
```

### Task TSK-0727 <a id="hcr-tsk-0727"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:27`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 运行全量后端测试 `go test ./...` 与静态编译门禁 `go build ./...`，确保零报错退出码严格为 0。
```

### Task TSK-0728 <a id="hcr-tsk-0728"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:31`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.1 来源严格完整 URL 规范化与名称回退废除：改造 `CanonicalSourceURL` 仅规范化 scheme/hostname 与默认端口，严格保留 query (token/signature)、userinfo 与 path 大小写；废除现代订阅映射对名称的任何 fallback，URL 未匹配一律保留为 unmapped 历史记录。
```

### Task TSK-0729 <a id="hcr-tsk-0729"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:32`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.2 订阅删除与未关联 DTO 语义解耦：修复 `inventory/service.go` 中 `subscription_id == nil` 误标 `source_deleted`，新增 `source_unmapped: boolean`；前端节点抽屉对历史未关联订阅展示“未关联当前订阅”黄色标签，仅对真实删除事件或外键级联清空展示“已删除”；全量前端单测与类型检查通过。
```

### Task TSK-0730 <a id="hcr-tsk-0730"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:33`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.3 幂等性预查与不可变证据防伪计数：改造 `recover_history.go` 历史插入逻辑，采用同事务前置存在性预查与 `INSERT ... ON CONFLICT DO NOTHING`，准确上报 `newly_inserted`、`existing_records`、`total_records`，杜绝假增；历史已存在证据若内容篡改触发 Fail-Closed 回滚；实机副本验证首轮回填 998，次轮回填 0 增量、998 已存。
```

### Task TSK-0731 <a id="hcr-tsk-0731"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:34`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.4 独立审查机复验（Reviewer Reverification）：由独立 Reviewer 会话对照同一 Change 与整改 diff 审查代码安全性与架构规范。（已于独立 Reviewer 会话出具最终代码与安全审计 PASS）
```

### Task TSK-0732 <a id="hcr-tsk-0732"></a>

- **Source**: `openspec/changes/fix-csp-legacy-source-attribution/tasks.md:35`
- **Source Digest**: `ffc7d39cc7e34d156b7f48ee31b441f0ac15b356285aab066a04d55ce5722819`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.5 成品黑盒验收（Critic Verification）、上线投产与回滚副本演练闭环：在 18081 隔离环境完成多视口视觉与深链交互黑盒验收（Critic PASSED）；获得用户授权后严格执行受控停机冷替换（实测停机 70s）、正式迁移 Migration 16、生产历史账本幂等回填（998 条）及生产双端口就绪自验闭环；在独立沙箱完成原 rollback-pre-8302c51 镜像与停写 Schema 15 备份回滚演练（1348ms readyz，Schema 15 保持，表数 33，发布引用数 0，零改动原始备份，已安全清理容器释放 18082 端口）。
```

### Task TSK-0733 <a id="hcr-tsk-0733"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:3`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 统一 WG/TUIC/VLESS/Hysteria2 凭据提取及基础字段验证；既有实现保留但新增身份安全回归归 1.4，不能以该勾选证明恶意端点已拒绝。
```

### Task TSK-0734 <a id="hcr-tsk-0734"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:4`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 解析入口、图规则/拓扑与四目标编译接口基础收敛；可信节点身份与版本化合同仍由 1.4 冻结。
```

### Task TSK-0735 <a id="hcr-tsk-0735"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:5`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 复核 legacy 清场及 `sqlite/node_risk.go` 原勾选范围：多 provider 最新观测 count/page 必须一节点一行，分页节点后按索引取最新、不扫描全历史；以独立多 provider 同时间戳、不同 ID、海量旧行和 EXPLAIN QUERY PLAN fixture 验证，历史 DB 不自动写改，Go 定向测试与 build exit 0。（原任务包括风险模块，审查证明该部分未完，撤销勾选。）
```

### Task TSK-0736 <a id="hcr-tsk-0736"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:6`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 A 先冻结 `VerifiedNodeCredential`/版本化 identity binding（可信来源 vs payload、规范化 server/port/protocol/transport、敏感字段排除）和 `GET /api/v1/nodes/{logical_id}` 的 `node.connection`、`PATCH /api/v1/nodes/{logical_id}/connection` JSON/错误合同，文档对应 design.md 决策 1/2；不得纯由 payload 自证或简单反算 ID。证明 YAML/URI 双格式、相同 ID/protocol/version 恶意替换端点被拒绝、旧缺绑定 fail closed，合法 URL 不误拒。冻结 B 使用的接口与 E 使用的详情接口后才扇出。
```

### Task TSK-0737 <a id="hcr-tsk-0737"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:7`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.5 冻结 B/D 两个 SQLite migration 的不同版本号、仓库端口事务接口与迁移测试边界，两个分支不得同时写 `migrate.go` 或同一 migration 文件；如需共享编辑，先串行完成交接。
```

### Task TSK-0738 <a id="hcr-tsk-0738"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:11`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 C Mihomo（`internal/compiler/mihomo.go` + 独立测试）：RULE-SET 仅接受真实已配置 provider 或显式合法 URL，缺失即定点拒绝；所有四组及十四规则正反 fixtures、官方 `mihomo -t -f`、`go build ./...` exit 0；不与 A 同时修改 `compiler/compiler.go`。
```

### Task TSK-0739 <a id="hcr-tsk-0739"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:12`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 sing-box 官方选项编译的既有特性包；最终门禁 3.1 仍需重验。
```

### Task TSK-0740 <a id="hcr-tsk-0740"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:13`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 Surge 可表达子集编译的既有特性包；最终门禁 3.1 仍需重验。
```

### Task TSK-0741 <a id="hcr-tsk-0741"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:14`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 Quantumult X 可表达子集编译的既有特性包；最终门禁 3.1 仍需重验。
```

### Task TSK-0742 <a id="hcr-tsk-0742"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:15`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.5 B Publication+repo migration（`internal/application/publication/**`、`domain/publication.go`、`domain/ports.go` 出版接口、SQLite publication repo + 专用 migration、`transport/http/publications.go`/cmd 仅必要接线及专属测试）：A 交付可信凭据合同后，全目标的预览/发布/分发核验身份；原子持久化加密工件、内容 digest、凭据版本绑定/编译版本，删除全局 map 与从 auditRepo.List(PageSize100) 文本恢复；临时 DB 升级、重启、源变更、损坏、错误 token、撤销和历史无绑定 fail closed；`go build ./...` exit 0。无同一物理文件并发写入。
```

### Task TSK-0743 <a id="hcr-tsk-0743"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:16`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.6 E 前端（`web/src/features/nodes/**` 及专属测试）：必须在 A 冻结详情/编辑 API 后施工；删除 `.edge.internal`、假 WG 地址/公钥/UUID/secret 状态，未返回字段显示 unavailable；真实 GET 详情、PATCH 保存/轮换、失败回滚 UI、来源覆盖提示，不在 state/DOM 缓存秘密；Vitest 与 `vue-tsc --noEmit` exit 0。
```

### Task TSK-0744 <a id="hcr-tsk-0744"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:17`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.7 A 端点身份与节点编辑（`domain` 身份字段、`parser`、`resolver`、`inventory`、`http/nodes.go` 与 A 专属测试）：按 1.4 冻结合约，存可信绑定/非秘密详情投影、管理员 CSRF/CAS PATCH、轮换及订阅 Reconcile 覆盖/新身份节点语义；覆盖直接 compiler WithCredentials 绕过、恶意同 ID 换端点、合法 URL/YAML、无私钥/PSK 外泄、审计/并发；定向测试及 `go build ./...` exit 0。完成 1.4 后方与 B/C/D 并发独立文件施工，A 与 E 不写同文件。
```

### Task TSK-0745 <a id="hcr-tsk-0745"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:18`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.8 D SQLite risk（`internal/repository/sqlite/node_risk.go`、独立风险 migration + 专属测试）：单节点唯一最新风险 CTE，count/page 同过滤；分页 ID 后有界取最新、索引和 EXPLAIN；多 provider/大历史/unknown 与边界负例定向测试及 `go build ./...` exit 0。与 B 避免同 migration 文件/版本号及共享测试文件。
```

### Task TSK-0746 <a id="hcr-tsk-0746"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:22`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 最终合并工作树执行 `go test -race -count=3` 分组覆盖全部包、`go test -count=1 ./...`、`go build ./...`、web Vitest、`npx vue-tsc --noEmit` 全部 exit 0；隔离 vite build 前后 webassets/dist hash 不变；官方 mihomo/sing-box fixture 校验、端点伪造与本地 migration/重启/风险查询计划 fixtures、`openspec validate native-mihomo-export-clean-slate --strict` exit 0；保存退出码/版本/摘要。原先勾选早于独立审查阻断，撤销。
```

### Task TSK-0747 <a id="hcr-tsk-0747"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:23`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 独立 reviewer 对照本 Change、最终 diff、身份锚/HTTP 编辑/发布持久化与风险计数证据复验给出 PASS。阻断最多一次定点返工与复验；仍失败直接根因级修订本 Change，未通过项不勾选。
```

### Task TSK-0748 <a id="hcr-tsk-0748"></a>

- **Source**: `openspec/changes/native-mihomo-export-clean-slate/tasks.md:24`
- **Source Digest**: `5c5899a979a66926dbc4f1f12171a22d9d1d2c97b8cae42e8a7c87de9156f5b0`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 准备者提供隔离 URL、临时密钥/库、健康检查及多视口截图；critic 真实节点→编辑→预览→发布→下载→撤销 FLOW + VISUAL PASS 后才勾选，不触生产。
```

### Task TSK-0749 <a id="hcr-tsk-0749"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:36`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 单数据库与 27 张表领域职责全量取证：核准 `/data/csp-v1.db` 为唯一主库，确证 1 张技术表与 26 张业务表的完整主键、核心外键、资产行数与领域读写所有者
```

### Task TSK-0750 <a id="hcr-tsk-0750"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:37`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 节点 LogicalID 算法双路径与刷新事实核对：提取 Base ID 与 Scoped ID 实际计算路径，核准 983 个失活节点为历史保护资产，排除 ID 漂移缺陷
```

### Task TSK-0751 <a id="hcr-tsk-0751"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:38`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 Dogegg 上游原始响应实机最小 GET 凭据对账：通过受控只读 GET 取得上游原始响应并脱敏保存，确证 `127.0.0.1:1` 流量与域名条目系上游面板公告伪节点
```

### Task TSK-0752 <a id="hcr-tsk-0752"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:39`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 旧 Run 失败根因与在线探测分布重核：澄清旧 Run 构建失败细节未持久化之事实，统计最新 31 活跃节点的可用与失败客观分布
```

### Task TSK-0753 <a id="hcr-tsk-0753"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:40`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.5 Publications 状态与 SQLite 连接并发排查：核实当前持久化发布工件数为 0，核准 SQLite 单物理连接无锁争用或并发冲突证据
```

### Task TSK-0754 <a id="hcr-tsk-0754"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:44`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 数据库 ER 目标设计与增量表 DDL 规范：制定包含 6 张新增事实与关联表（`subscription_payloads`, `subscription_entries`, `node_connection_versions`, `node_connection_heads`, `node_overrides`, `publication_payload_refs`）的标准 DDL 规范与 SQLite 单向外键验证，核实存量 27 表主外键准确性
```

### Task TSK-0755 <a id="hcr-tsk-0755"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:45`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 上游事实层与公告伪节点分类器设计：设计原始响应 BLOB 受控持久化规则；3 同参数公告独立写入不去重；无 `UNIQUE(sub, source_key)` 折叠；设计两级前置来源门禁公告分类算法；设计条目级用户人工纠偏与审计；澄清上游面板品牌未知
```

### Task TSK-0756 <a id="hcr-tsk-0756"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:46`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 节点连接版本化、权威 Heads 指针与 CoW 隔离设计：设计解耦逻辑 ID 与连接参数的不可变版本模型；设计 `node_connection_heads` 单向复合外键指针；消费端统一读取 Head 指针；设计用户字段级修改合并及多源更新写时复制隔离规则
```

### Task TSK-0757 <a id="hcr-tsk-0757"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:47`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 IP Risk Nullable 1:N、Safe Detail 白名单、Mihomo xhttp 官方代码对齐与发布两阶段设计：建立 IP Risk Nullable 1:N 统一测量流（严格仅关联 kind='ip_risk' 通用探针观测，应用层事务校验阻断非 ip_risk 类型）；为 `risk_policy_revisions` 添加 `rules_digest` 快照；建立 `safe_detail_json` 严格白名单防泄密机制；严格依据 Mihomo v1.19.32 官方代码渲染 `path`, `host`, `mode`, `headers`，未知 extra 隔离不盲传；设计 `publication_payload_refs` 外键防删、Preview Draft -> Publish Snapshot 两阶段生命周期与空组防护
```

### Task TSK-0758 <a id="hcr-tsk-0758"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:48`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.5 扩充 14 场景独立测试方案与实测维护回滚对账方案编写：编制包含 14 个具体场景的独立测试工程方案（无私造框架）；清除固定 30 秒与 Zero-Loss 绝对承诺，建立实测停写短窗演练与原资产对账回滚补偿规范；完成内存 SQLite 23 项核心断言自验证
```

### Task TSK-0759 <a id="hcr-tsk-0759"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:52`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 编写并执行 Migration 000015：在保持既有 1014 节点、29 组、163 规则完全无损的前提下增量创建 6 张新表，对 `ip_risk_observations` 增量添加 `probe_observation_id`，对 `risk_policy_revisions` 添加 `rules_digest`，并事务性初始化存量节点连接版本与当前 head 指针 (实施已授权)
```

### Task TSK-0760 <a id="hcr-tsk-0760"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:53`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 移除 Repository 构造函数中的硬编码运行时 DDL：清理 `internal/repository/sqlite/probes.go` 中残存的动态 `CREATE INDEX` 与 `ALTER TABLE`，收敛至迁移框架 (实施已授权)
```

### Task TSK-0761 <a id="hcr-tsk-0761"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:54`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 实现新增数据表的 SQLite Repository 接口及单测：包括 Payload 压缩存取、Entry 查询、版本读写、Heads 指针更新、Payload 引用锁定及 Overrides 合并仓储方法 (实施已授权)
```

### Task TSK-0762 <a id="hcr-tsk-0762"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:58`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 在 Inventory 抓取服务中集成 Payload 原始持久化：成功抓取落盘完整 BLOB 与关键响应头，抓取失败时保留错误流水并严格保全既有健康节点库存 (实施已授权)
```

### Task TSK-0763 <a id="hcr-tsk-0763"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:59`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 落地严格组合公告伪节点分类器与用户纠偏：在解析环节将组合全条件匹配项归类为 `notice`，同参数多公告全部独立记录不去重，保留于条目表供展示，不入库 `nodes` 且不参与拨测与发布；支持条目级纠偏重放 (实施已授权)
```

### Task TSK-0764 <a id="hcr-tsk-0764"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:63`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 实现节点连接版本演进与不可变版本派生：参数实质变动自增 `connection_revision`，原子更新 `node_connection_heads` 并使旧观测标记为 stale，纯名称更新仅修改元数据 (实施已授权)
```

### Task TSK-0765 <a id="hcr-tsk-0765"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:64`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 实现用户显式覆盖（Overrides）持久化与合并：支持用户在 Web 控制台修改参数并在后续抓取刷新中持续继承生效 (实施已授权)
```

### Task TSK-0766 <a id="hcr-tsk-0766"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:65`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 实现多源更新时的写时复制（CoW）隔离：当多订阅共享节点发生参数分化时，自动分支至 scoped ID，保持未变动订阅及现有策略拓扑引用完整 (实施已授权)
```

### Task TSK-0767 <a id="hcr-tsk-0767"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:69`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.1 统一 IP Risk Nullable 1:N 测量关联与 Safe Detail 白名单：在 `probe_observations` 与 `ip_risk_observations` 写入时绑定同一 `probe_observation_id` 与 `measurement_id`，落盘 safe_detail 白名单，为策略版本添加 `rules_digest` 快照 (实施已授权)
```

### Task TSK-0768 <a id="hcr-tsk-0768"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:70`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.2 在 Mihomo 编译器与探针适配器中解封 xhttp：按官方代码规范渲染 `path`, `host`, `mode`, `headers` 为 `xhttp-opts`，未知 extra 安全隔离，使 VLESS xhttp 节点能够顺利导出与建立出站代理；对 sing-box 返回清晰的 `unsupported_target_capability` (实施已授权)
```

### Task TSK-0769 <a id="hcr-tsk-0769"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:71`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.3 实现发布两阶段快照生命周期与空组保护：Preview 创建不可变 Draft 并计算 Manifest，插入 `publication_payload_refs` 保护 Payload；Publish 严格激活指定快照（杜绝时间差库存漂移）；空组拒绝自动回退至 DIRECT (实施已授权)
```

### Task TSK-0770 <a id="hcr-tsk-0770"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:72`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.4 修复前端 `web/src/ui/ErrorStateCard.vue` 错误分类逻辑：优先依据 HTTP 状态码 422 与业务 code 进行能力不支持展示，杜绝误判为网络失联 (实施已授权)
```

### Task TSK-0771 <a id="hcr-tsk-0771"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:76`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 7.1 本地全量单元测试与编译门禁：运行 `go build ./...`、全量 Go 单元测试与前端类型检查，确保退出码为 0 (实施已授权)
```

### Task TSK-0772 <a id="hcr-tsk-0772"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:77`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 7.2 生产受控单容器停机冷替换与可验证备份：执行 SQLite 在线一致性备份 (sha256:a5b25dfe...) 校验完整性与外键 (FK 0)，新镜像 (sha256:62ee52ebbfaf) 替换旧容器至 5aaefb45714c，实测停机命令至就绪耗时 1.941s，双端口验证 `/healthz` 与 `/readyz` (schema 15) 200 OK，存量资产与冷备布尔全等比对相等 (已闭环完成)
```

### Task TSK-0773 <a id="hcr-tsk-0773"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:78`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 7.3 合法生产管理凭据真实节点实网验收：用户已授权实施上线，Pi无法调用前台已登录vault安全渠道；生产带身份管理刷新/探针/预览发布未验 (工程 Reviewer PASS 与 Critic PASSED 基于隔离 fixture 验证，不冒充生产管理验收)
```

### Task TSK-0774 <a id="hcr-tsk-0774"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:82`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 8.1 后端领域与仓储层支持启用订阅与统一底层谓词：在 `domain.NodeFilter` 中增加 `Scope: NodeScope`（`enabled_subscriptions`、`all_assets`）与 `SubscriptionID: string`，在 `domain.Subscription` 与 `domain.ProbePoolStatus` 扩展对账字段，实现 `buildNodeFilterPredicates` 统一谓词并更新 `List`、`ListReadModel` 与 `Count` 查询
```

### Task TSK-0775 <a id="hcr-tsk-0775"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:83`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 8.2 后端 HTTP 路由参数扩展与接口契约落地：在 `GET /api/v1/nodes` 增加 `scope`（默认 `enabled_subscriptions`，显式 `all_assets`，未知 400 `invalid_node_scope`），在 `GET /api/v1/subscriptions` 增加 `node_count`、`source_node_count` 与 `counts_scope`，在 `GET /api/v1/probes/pool` 增加 `inventory_total`、`candidate_total` 与 `scope` 并保持 `total_count` 含义，单节点详情与覆写接口保持全库权限
```

### Task TSK-0776 <a id="hcr-tsk-0776"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:84`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 8.3 探测调度、发布编译与消费者候选门禁：在 probe runner（selected IDs 与 run all）、periodic coordinator 与 publication resolve 中收敛至范围 E 活跃非公告候选，保持历史发布快照字节绝对不可变
```

### Task TSK-0777 <a id="hcr-tsk-0777"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:85`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 8.4 隔离环境全场景单元与集成自测：编写覆盖启用/停用、共享节点、失活节点、手工无源节点、公告排除、抓取失败保留旧成果、刷新删除成员、10+页分页与 risk/q/sub 过滤一致性的全套测试，确保 `go test -count=1 ./...` 与静态编译退出码为 0
```

### Task TSK-0778 <a id="hcr-tsk-0778"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:86`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 8.5 前端节点台账、首页概览与订阅卡片生效库存展示：更新 `web/src/features/nodes/useNodes.ts`、`NodesView.vue`、`DashboardView.vue` 与 `SubscriptionsView.vue`（由前端独立施工机完成，本包不修改 webassets 共享产物）
```

### Task TSK-0779 <a id="hcr-tsk-0779"></a>

- **Source**: `openspec/changes/redesign-csp-database-dataflow/tasks.md:87`
- **Source Digest**: `529f776e579ba02ebc02b466ceafeeb9fc0ee4b30860f37dce349064a294c865`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 8.6 生产受控单容器停机冷替换与生效库存发布自验：执行时点一致性 SQLite 备份快照 (sha256:65dd54ce...) 校验完整性与外键 (FK 0)，新镜像 (sha256:4925c70d...) 替换旧容器至 73cd5633a319，实测停机至首个 healthz 耗时 2.296s (/readyz 随后单独校验)，生产只读 SQL 对账生效库存 31、全量资产 1014、候选 31，静态业务数据对停写备份保持全等，隔离预览 PID 827371 释放
```

### Task TSK-0780 <a id="hcr-tsk-0780"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:3`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.1 引入 `github.com/metacubex/mihomo v1.19.31` 依赖并运行 `go mod tidy`，验证编译通过且依赖无冲突
```

### Task TSK-0781 <a id="hcr-tsk-0781"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:4`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.2 实现 `internal/probe/mihomo/adapter.go`，将 `domain.Node` 映射为 Mihomo proxy 配置并使用 `adapter.ParseProxy` 构造进程内出站；添加单元测试验证支持 `skip-cert-verify`、Hysteria2 `ports` 与 TUIC `disable_sni`
```

### Task TSK-0782 <a id="hcr-tsk-0782"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:5`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.3 实现 `internal/probe/mihomo/client.go`，构建 `ProxyClient` 绑定单节点出站，显式配置 `Proxy: nil`，将目标域名直接送入远端 Metadata，并实现 `Close()` 与超时 Watchdog 机制；添加单元测试验证宿主系统代理不生效且远端域名直传
```

### Task TSK-0783 <a id="hcr-tsk-0783"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:6`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 1.4 清理已被完全替代的 `internal/application/probe/safe_dialer.go` 与 `internal/probe/singbox/runtime.go` 等旧拨号代码，确保其他使用 Singbox 的导出和转换逻辑不受影响
```

### Task TSK-0784 <a id="hcr-tsk-0784"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:10`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.1 移植 `internal/probe/platform/alive.go`，基于 Cloudflare 204 及快速 CDN Trace 接口实现轻量级 Stage 1 测活；编写单测验证有效 204 与非 2xx 响应处理
```

### Task TSK-0785 <a id="hcr-tsk-0785"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:11`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.2 移植 `internal/probe/platform/openai.go`，实现双轨（Cookies + iOS 客户端）检测与 Trace 国家码提取；编写单测覆盖 `Full (GPT⁺)`、`Web (GPT)` 与不可用场景
```

### Task TSK-0786 <a id="hcr-tsk-0786"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:12`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.3 移植 `internal/probe/platform/netflix.go`，实现 Fast.com CDN 测速接口检测与双 Title 回退断言；编写单测覆盖 `Full`、`OriginalsOnly`、`Banned` 三种状态
```

### Task TSK-0787 <a id="hcr-tsk-0787"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:13`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.4 移植 `internal/probe/platform/youtube.go`，请求 Premium 页面并提取 `INNERTUBE_CONTEXT_GL` 区域标签，过滤 `www.google.cn` 送中特征；编写单测覆盖解锁与送中场景
```

### Task TSK-0788 <a id="hcr-tsk-0788"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:14`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.5 移植 `internal/probe/platform/disney.go` 与 `internal/probe/platform/claude.go`，接入 BAMGrid 与 Trace 地区黑名单断言；编写单测验证判定逻辑
```

### Task TSK-0789 <a id="hcr-tsk-0789"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:15`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.6 实现 `internal/probe/platform/speed.go`，包装 `NetworkLimitedReader` 设定 5MB 应用读取预算上限（作用于 `resp.Body`，TCP 缓冲存在网络层预读，非 NIC 网卡硬上限）与单调截止超时；编写单测验证达到字节上限时主动返回 EOF 截断并正确计算带宽 (float64 KB/s, 1024B/s)
```

### Task TSK-0790 <a id="hcr-tsk-0790"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:16`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 2.7 在代码库根目录或相应模块维护原作者版权与 GPL-3.0 许可证 NOTICE 说明，记录上游出处
```

### Task TSK-0791 <a id="hcr-tsk-0791"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:20`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.1 重构 `internal/probe/queue/` 与 `internal/application/probe/runner.go`，实现 Alive -> Media/AI -> Speed 三阶段漏斗调度，废除 16~32 硬编码并发限制，采用阶段并发（Alive 50 / Media 20；测速阶段采用 Speed 8 作为降低带宽争用的本地差异设置，上游默认则为 Speed 20），保证 Stage 2 结束才启动 Stage 3，支持用户单选/混选 kind，复用每节点 client 实例（上层复用 adapter 实例，非物理网络只拨号一次）并一次性优雅 close；编写调度器漏斗单测验证 Stage 1 失败节点绝不进入后续阶段
```

### Task TSK-0792 <a id="hcr-tsk-0792"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:21`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.2 真正接入 `platform.CheckSpeed` 生产调用，删除 `runner.go` 中被替代的 profiles.Speed 旧 1MB HTTP 分支，彻底消除新旧双轨，将各平台真实检测结构体统一序列化为 JSON 观测记录；编写单测验证所有探针 Kind 均有可达的真实判定
```

### Task TSK-0793 <a id="hcr-tsk-0793"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:22`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.3 扩展 `internal/domain/probe.go` 与 `internal/application/inventory/service.go` 的 `CapabilityStatus` 数据结构，透传细粒度平台结果（`region`、`sub_tier` 等）；编写单测验证各平台状态在 `ToNodeView` 中无损输出
```

### Task TSK-0794 <a id="hcr-tsk-0794"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:23`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 3.4 保持现有 `/api/v1/probes/*` 与 `/api/v1/nodes/*` 路由定义，确保现有管理鉴权、任务取消、批量触发与定时轮询逻辑兼容运作
```

### Task TSK-0795 <a id="hcr-tsk-0795"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:27`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.1 更新 `web/src/features/probes/probeTypes.ts` 与 `web/src/features/nodes/nodeView.ts`，扩展多平台解锁结果数据模型（如 OpenAI、Netflix、YouTube、Disney、Claude 等细粒度状态）
```

### Task TSK-0796 <a id="hcr-tsk-0796"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:28`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.2 更新 `web/src/features/probes/ProbeRunCard.vue` 与 `ProbeEvidenceSheet.vue`，展示各平台状态徽标（如 `GPT⁺ (US)`、`NF (HK)`、`YT (SG)`）及测速带宽；运行 `npm --prefix web run type-check` 验证类型安全
```

### Task TSK-0797 <a id="hcr-tsk-0797"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:29`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 4.3 更新中英双语国际化文案字典 `web/src/locales/zh-CN.ts` 与 `en-US.ts`，补充各平台状态描述与封禁提示；运行 `npm --prefix web test` 确保前端测试全部通过
```

### Task TSK-0798 <a id="hcr-tsk-0798"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:33`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.1 运行确定性静态编译与类型门禁：执行 `go build ./...` 与 `npm --prefix web run type-check`，验证退出码严格为 0
```

### Task TSK-0799 <a id="hcr-tsk-0799"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:34`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.2 编写与运行受控代理/fixture集成测试覆盖实际 Runner 行为：alive 失败不测媒体与速度、阶段 2 结束才阶段 3、阶段并发 (alive 50/media 20/本地 speed 8)、用户单选/混选/取消/复用/close、速度 float64 KB/s 及预算、SQLite 到 Run/node 接口；执行 `go test ./...` 退出码严格为 0
```

### Task TSK-0800 <a id="hcr-tsk-0800"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:35`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 5.3 在本地隔离沙箱环境启动预览实例，验证管理凭据登录、节点列表及探针工作台真实渲染正常，准备就绪实例交付 Critic
```

### Task TSK-0801 <a id="hcr-tsk-0801"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:39`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.1 汇聚门禁独立代码审查（Reviewer Gate）：完成对全量 Diff、三阶段调度架构、Mihomo 出站与数据模型的独立代码审查，出具真实裁决 `REVIEW: PASS`
```

### Task TSK-0802 <a id="hcr-tsk-0802"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:40`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.2 成品隔离黑盒验收（Critic Gate）：基于本地隔离预览实例 (`127.0.0.1:18081`)，完成桌面端 (1440x900) 与移动端 (392x872) 15 视口截图采集与无控制台/网络报错验证，真实验证各平台细粒度分级 (GPT⁺/Web/App, NF Full/Originals/Banned, YT 送中 CN, Disney+ Soon) 与取消/流转流程，出具真实裁决 `CRITIC: PASSED`（全部基于 Test-Fixture，不声称真实商业解锁）
```

### Task TSK-0803 <a id="hcr-tsk-0803"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:41`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.3 生产发布在线一致性热备（Production Preflight Backup）：部署前使用官方 SQLite `.backup` 协议对生产数据库 `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db` 执行在线热备并校验完整性与节点库存
```

### Task TSK-0804 <a id="hcr-tsk-0804"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:42`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.4 生产发布与上线健康门禁（Controlled Deployment & Healthz）：构建并受控替换生产容器镜像，维持现有数据卷/双端口/代理/鉴权配置不变，验证生产 `/healthz` 与 `/readyz` 200 OK
```

### Task TSK-0805 <a id="hcr-tsk-0805"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:43`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [ ] 6.5 合法生产管理身份真实节点实网验收（Production Acceptance with Legal Credential）：在用户提供或配置合法生产管理凭据后，针对生产活跃节点执行受控小范围真实节点探针抽测
```

### Task TSK-0806 <a id="hcr-tsk-0806"></a>

- **Source**: `openspec/changes/redo-node-probes-subcheck/tasks.md:44`
- **Source Digest**: `402d67cffcaa62eb5e94fa15b71696e282d84a9abf54e03aedb52e0a754ab979`
- **Classification**: `completed status source only, not fresh work`
- **Status**: Historical task completion record preserved from source change; not fresh runtime work.
- **Original Source Text**:
```text
- [x] 6.6 冻结写集合与代码版本提交（Git Commit）：经独立审查与验收通过后，由主脑调用 `git_commit` 正式提交代码变更 (HEAD 9b43415)
```

---

## 4. 架构决策记录登记 (Architecture Decision Records - DEC-0807 ~ DEC-0809)

### Decision DEC-0807 <a id="hcr-dec-0807"></a>

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

### Decision DEC-0808 <a id="hcr-dec-0808"></a>

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

### Decision DEC-0809 <a id="hcr-dec-0809"></a>

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

## Context

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

## Goals / Non-Goals

- **Goals**：
  1. 彻底修复 `POST /api/v1/policies/validate`：无论客户端发送空请求体/`{}`、带内嵌 `edges` 的 `groups` 数组，还是规范化的 `{ groups, edges, rules, policy_rules, admission_rules }`，均能正确解析并返回拓扑校验结果；非法 JSON 请求体在 `decodeJSON` 失败后立即终止返回 422，绝不二次写响应。
  2. 将 `singbox`、`surge`、`qx` 改造为纯节点导出目标：在 `compiler.validate`、各目标渲染器及 `publication` 预检中完全忽略 `Groups` 与 `Rules`，仅校验并输出节点列表格式；`mihomo` 保持完整配置编译与校验。
  3. 将前端默认语言切换为 `'zh-CN'`，补齐 `zh-CN` 与 `en-US` 词条，消除全站视图组件与 TS 辅助模块中的硬编码英文及裸露枚举。
- **Non-Goals**：
  - 不改变 SQLite 数据库表结构或迁移文件。
  - 不修改生产环境运行中的容器状态，仅在隔离测试与预览环境中验证。

## Decisions

### 1. 策略拓扑校验去冗余参数化（`internal/transport/http/policies.go` & `web/src/features/policy/usePolicy.ts`）

- **后端 `policyHandler.validate`**：
  - 所有策略组、边和规则均已持久化在后端数据库中，彻底移除 `validateGraphRequest` 请求体解析分支，直接调用 `h.service.ValidateGraph(r.Context())` 从数据库加载当前策略组、边与活动修订规则进行校验。
- **前端 `usePolicy.ts`（`validateGraph`）**：
  - 直接无参调用 `api.post<ValidationResult>('/api/v1/policies/validate')`，不再从前端传 `groups`、`edges` 或 `rules`。

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

## Risks / Trade-offs

- **Golden Fixture 与现有测试契约变更**：
  - `singbox.golden`、`surge.golden`、`qx.golden`、`quantumult-x.golden` 以及 `compiler_test.go`、`singbox_test.go`、`surge_test.go`、`qx_test.go`、`preflight_test.go`、`feature_convergence_test.go` 原先断言非 Mihomo 目标包含策略组和路由规则，需同步更新为验证“仅 Mihomo 包含策略组与规则，非 Mihomo 目标仅输出纯节点且忽略策略组与规则”。
- **前端单测默认语言切换影响**：
  - `web/src/**/*.test.ts` 中部分测试原先假定初始 locale 为 `'en-US'` 或断言英文按钮/校验文本，需在 Task 1.2 中同步更新测试断言以匹配默认 `'zh-CN'` 及中文化文案（并保留对切换 `'en-US'` 的测试覆盖）。

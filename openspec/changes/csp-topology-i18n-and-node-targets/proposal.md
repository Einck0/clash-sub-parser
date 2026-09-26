## Why

根据用户在实机使用中的三项明确指令与取证结果：
1. **“校验拓扑图，报错”**：策略树页面点击“校验拓扑图”时，前端 `usePolicy.ts` 直接将含内嵌 `edges` 及视图字段的 `groups.value` 发送给 `POST /api/v1/policies/validate`；后端 `policies.go` 的 `validate` 处理器不仅因 `domain.NodeGroup` 不含 `edges` 字段且 `decodeJSON` 启用 `DisallowUnknownFields()` 触发 HTTP 422 `invalid_json`，还因为 `_ = decodeJSON(w, r, &body)` 吞掉错误未 `return` 导致向同一 `http.ResponseWriter` 二次写入响应。
2. **“订阅发布只导出mihomo，其他是导出节点格式，忽略规则”**：当前编译器对 `singbox`、`surge`、`qx` 强行校验并渲染策略组（`Groups`）与路由规则（`Rules`），一旦策略树使用了 `fallback`/`loadbalance` 策略组或 `GEOSITE`/`PROCESS-NAME`/`RULE-SET` 等规则，非 Mihomo 目标即报 422 拒绝导出。用户明确要求仅 `mihomo` 导出含策略组与规则的完整配置，`singbox`、`surge`、`qx` 仅作为纯节点格式导出并彻底忽略策略组与规则。
3. **“系统语言很多没有中文化”**：前端 `web/src/locales/index.ts` 默认语言仍为 `'en-US'`，且 `ErrorStateCard.vue`、`PolicyView.vue`、`GroupCard.vue`、`NodesView.vue`、`ProbesView.vue`、`ProbeEvidenceSheet.vue`、`PublicationsView.vue`、`SettingsView.vue` 等大量组件与 TS 辅助模块存在硬编码英文、裸露后端枚举与英文错误提示。

## What Changes

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

## Capabilities

### New Capabilities

- `topology-i18n-export`: 策略拓扑图校验修复、Mihomo 完整配置与非 Mihomo 目标纯节点格式导出（忽略策略组与规则）、以及前端默认 `zh-CN` 全站中文化契约。

### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `topology-i18n-export` 能力规格建立完整契约）。

## Impact

`internal/transport/http/policies*.go`、`internal/compiler/{compiler,mihomo,singbox,surge,qx}*.go`、`internal/compiler/testdata/golden/*.golden`、`internal/application/publication/{service,preflight_test,service_test}.go`、`internal/integration/*.go` 及 `web/src/**`。仅在隔离验证环境构建与测试，不直接操作生产运行容器。

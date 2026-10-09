## 1. Phase 1 — 可并发扇出前后端特性包（写集合不重叠）

- [x] 1.1 **Task 1.1（后端：拓扑校验修复 + 编译器非 Mihomo 纯节点格式导出忽略规则 + 预检同步）**（写集合：`internal/**`）：
  - 修复 `internal/transport/http/policies.go`（`validate`）：移除冗余的 `validateGraphRequest` 请求体解析逻辑，直接调用 `h.service.ValidateGraph(r.Context())` 校验后端数据库持久化状态，并更新 `internal/transport/http/policies_test.go`。
  - 改造 `internal/compiler/{compiler,mihomo,singbox,surge,qx}.go` 及 golden/测试文件：仅 `mihomo` 校验并渲染完整配置（`Nodes` + `Groups` + `Rules`）；当 `target != domain.TargetMihomo` 时，`compiler.validate` 完全忽略 `snapshot.Groups` 与 `snapshot.Rules`；`renderSingBox` 仅输出节点 `{"outbounds": [...], "endpoints": [...]}` JSON（不含策略组 outbound 与 `route`）；`renderSurge` 仅输出纯节点行（不含 `[General]`/`[Proxy]`/`[Proxy Group]`/`[Rule]` 段落，并补齐 `hysteria2`、`tuic`、`wireguard` 节点行渲染）；`renderQuantumultX` 仅输出纯节点行（不含 `[general]`/`[server_local]`/`[policy]`/`[filter_local]` 段落）；同步更新 `internal/compiler/testdata/golden/*.golden` 与 `internal/compiler/*_test.go`。
  - 同步更新 `internal/application/publication/service.go`（`evaluatePreflight`）及 `preflight_test.go`、`service_test.go`、`internal/integration/feature_convergence_test.go`：预检与发布对非 `mihomo` 目标忽略 `empty_routed_group` 等策略组/规则阻断诊断。运行 `go test ./internal/...` 与 `go build ./...` 验证全部通过（exit 0）。

- [x] 1.2 **Task 1.2（前端：拓扑校验请求规范化 + 全站完整中文化 + 发布目标节点格式文案更新）**（写集合：`web/src/**`）：
  - 修复 `web/src/features/policy/usePolicy.ts`（`validateGraph`）：直接无参调用 `api.post<ValidationResult>('/api/v1/policies/validate')`，不再从前端重复传边和点。
  - 更新 `web/src/locales/index.ts` 将默认语言与 `fallbackLocale` 设为 `'zh-CN'`；新增/完善 `web/src/locales/zh-CN.ts`、`web/src/locales/en-US.ts` 与 `web/src/locales/messages.ts` 完整词条。
  - 全面改造 `web/src/**` 所有含硬编码英文或裸露枚举的组件与 TS 模块（`App.vue`、`ErrorStateCard.vue`、`DashboardView.vue`、`PolicyView.vue`、`PolicyEditorSheet.vue`、`GroupCard.vue`、`usePolicy.ts`、`policyTypes.ts`、`NodesView.vue`、`nodeView.ts`、`useNodes.ts`、`ProbesView.vue`、`ProbeRunCard.vue`、`ProbeEvidenceSheet.vue`、`useProbes.ts`、`probeTypes.ts`、`PublicationsView.vue`、`usePublications.ts`、`publicationTypes.ts`、`SubscriptionsView.vue`、`SubscriptionConfigDrawer.vue`、`useSubscriptions.ts`、`SettingsView.vue`），将协议名、策略组类型（手动选择/自动测速/故障转移/负载均衡）、规则动作、探针状态/结论、健康度、风险等级、预检诊断及常见后端 API 错误码全部映射为地道中文，并更新发布目标说明文案（明确标识 `Mihomo` 为完整配置含策略组与规则，`sing-box` / `Surge` / `Quantumult X` 为仅导出节点格式、忽略规则）。
  - 同步更新 `web/src/**/*.test.ts` 单测并运行 `npm test -- --run` 与 `npx vue-tsc --noEmit` 验证通过（exit 0）。

## 2. Phase 2 — 全仓汇聚门禁与独立验收（Phase 1 全部完成后执行）

- [x] 2.1 **Task 2.1（全仓汇聚自测与硬门禁）**：执行 `go test -count=1 ./...`、`go build ./...`、前端 `npm test -- --run`、`npx vue-tsc --noEmit` 全部 exit 0；使用隔离输出目录验证前端生产构建并确保受保护现场干净；执行 `openspec validate csp-topology-i18n-and-node-targets --strict` exit 0。
- [x] 2.2 **Task 2.2（独立 `reviewer` 终态审查）**：独立 `reviewer` 对照 `csp-topology-i18n-and-node-targets` 规格与最终 diff 完成只读代码与安全审计并出具 `PASS` 裁决。
- [x] 2.3 **Task 2.3（隔离预览实例准备与独立 `critic` 多视口真浏览器验收）**：在隔离环境（临时 SQLite 库、隔离构建产物、非生产端口）启动预览实例并通过健康检查，移交独立 `critic` 执行全站中文 UI、策略树拓扑校验、Mihomo 完整配置导出及 sing-box/Surge/Quantumult X 纯节点格式导出（忽略规则）的真浏览器全链路与多视口视觉验收并取得 `PASSED`。

## Purpose

修复策略树拓扑图校验报错，落实“订阅发布仅 Mihomo 导出含策略组与规则的完整配置、其他目标（sing-box / Surge / Quantumult X）仅导出纯节点格式并忽略策略组与规则”，并将前端默认语言切换为简体中文（`zh-CN`）完成全站无死角中文化。

## ADDED Requirements

### Requirement: 策略拓扑图校验接口无参直接校验后端持久化数据契约
后端 `POST /api/v1/policies/validate`（及 `/api/v1/policy/validate`）与前端 `validateGraph()` SHALL 直接基于后端数据库持久化状态执行拓扑校验，无需前端重复回传节点组、边或规则载荷：
1. 后端 `policyHandler.validate` SHALL 直接调用 `h.service.ValidateGraph(r.Context())` 从 SQLite 数据库加载全量策略组、边连接关系及当前活动修订的分流与准入规则执行完整拓扑校验，移除冗余的 `validateGraphRequest` 请求体解析分支。
2. 前端 `web/src/features/policy/usePolicy.ts` 的 `validateGraph()` SHALL 直接以无请求体方式调用 `api.post<ValidationResult>('/api/v1/policies/validate')`，并在校验通过或发现环路/自环/非法引用时准确更新 `validationResult`。

#### Scenario: 前端点击校验拓扑图无参触发后端数据库权威拓扑校验
- **WHEN** 用户在策略树页面点击“校验拓扑图”，前端向 `POST /api/v1/policies/validate` 发起无参请求，且后端数据库中的策略组、边与规则无环路或非法引用
- **THEN** 后端直接从数据库加载完整拓扑图与规则完成校验，返回 HTTP 200 及 `{"data":{"valid":true}}`

#### Scenario: 后端数据库存在拓扑环路时返回结构化校验错误
- **WHEN** 客户端调用 `POST /api/v1/policies/validate` 且后端持久化的策略组边关系存在环路或非法引用
- **THEN** 后端返回单个 HTTP 422 领域校验错误响应（如 `cycle_detected`）

### Requirement: 仅 Mihomo 导出完整配置、其他目标仅导出纯节点格式并忽略规则
编译器与发布服务 SHALL 严格区分完整配置目标（`mihomo`）与纯节点格式导出目标（`singbox`、`surge`、`qx`）：
1. **Mihomo 完整配置目标（`target == "mihomo"`）**：编译器 SHALL 校验并渲染完整的节点（`proxies`）、策略组（`proxy-groups`）、规则集提供者（`rule-providers`）、分流规则（`rules`）与 `dns` 配置；当路由指向空策略组（`empty_routed_group`）或策略组/规则不合法时，编译器与发布预检 SHALL 正常拦截。
2. **非 Mihomo 纯节点导出目标（`target` 为 `singbox`、`surge` 或 `qx`）**：
   - `compiler.validate` SHALL 仅校验 `snapshot.Nodes` 的节点协议支持度与凭据完整性，SHALL 完全忽略 `snapshot.Groups` 与 `snapshot.Rules`（即使快照中包含 `fallback`/`loadbalance` 策略组、空策略组或 `GEOSITE`/`PROCESS-NAME`/`RULE-SET` 等任意路由规则，也绝不报错）。
   - `renderSingBox` SHALL 仅输出纯节点 JSON 配置（仅包含节点 `outbounds` 与 WireGuard `endpoints`，不含策略组 selector/urltest outbound，不含 `route` 路由段落），且输出 JSON 必须通过官方 `sing-box check` 校验。
   - `renderSurge` SHALL 仅输出纯节点行（每行一个 `<name> = <protocol>, <server>, <port>, ...`，不含 `[General]`、`[Proxy]`、`[Proxy Group]`、`[Rule]` 段落），并支持渲染 `ss`、`vmess`、`trojan`、`hysteria2`、`tuic`、`wireguard` 节点行（对 Surge 不支持的 `vless` 协议保持节点级 `CapabilityError`）。
   - `renderQuantumultX` SHALL 仅输出纯节点行（每行一个 `<protocol> = <server>:<port>, ..., tag=<name>`，不含 `[general]`、`[server_local]`、`[policy]`、`[filter_local]` 段落），支持渲染 `ss`、`vmess`、`trojan` 节点行。
   - 发布预检 `evaluatePreflight` 在 `target != "mihomo"` 时 SHALL 忽略 `empty_routed_group` 等策略组/规则诊断，不因策略组或规则阻断预览或发布。
3. 前端发布页（`web/src/features/publications/`）与策略树页（`web/src/features/policy/`）SHALL 明确标识 `Mihomo` 为完整配置导出（含策略组与分流规则），`sing-box`、`Surge`、`Quantumult X` 为纯节点格式导出（忽略策略组与规则）。

#### Scenario: 快照含复杂策略组与 Mihomo 专属规则时导出非 Mihomo 纯节点格式
- **WHEN** 当前策略快照包含 `fallback`、`loadbalance` 策略组以及 `GEOSITE`、`PROCESS-NAME`、`RULE-SET` 路由规则，管理员选择 `singbox`、`surge`（协议受支持）或 `qx`（协议受支持）执行预览或创建发布
- **THEN** 预览与发布均返回 HTTP 200 成功，不报策略组或规则不支持错误；`singbox` 仅输出节点 `outbounds`/`endpoints` JSON，`surge` 与 `qx` 仅输出纯节点文本行，不含任何策略组或路由规则段落

#### Scenario: 空路由策略组仅阻断 Mihomo 而不阻断纯节点导出目标
- **WHEN** 快照存在路由规则指向 0 节点的空策略组（产生 `empty_routed_group` 诊断）
- **THEN** `mihomo` 目标的发布预检与发布被阻断（HTTP 409 `publication_preflight_rejected`），而 `singbox`、`surge`、`qx` 纯节点导出目标的预检与发布正常通过

#### Scenario: Mihomo 目标继续导出含策略组与规则的完整配置
- **WHEN** 管理员选择 `mihomo` 目标执行预览或发布下载
- **THEN** 导出内容包含完整的 `proxies`、`proxy-groups`、`rules`（及必要的 `rule-providers` 与 `dns`）段落

### Requirement: 前端默认简体中文（`zh-CN`）与全站完整中文化
前端国际化体系 SHALL 将默认语言设置为简体中文（`'zh-CN'`），并完成全站所有界面组件、状态徽章、枚举标签、表单校验与错误提示的中文化：
1. `web/src/locales/index.ts` 在本地存储未显式设置语言时 SHALL 默认使用 `'zh-CN'`，且 `fallbackLocale` SHALL 为 `'zh-CN'`。
2. `web/src/locales/`（含 `zh-CN.ts`、`en-US.ts` 及 `messages.ts`）SHALL 完整覆盖导航栏、控制台、订阅源、节点账本、探针引擎、策略树、订阅发布、全局设置、鉴权门禁及通用错误卡片的全部词条。
3. `App.vue`、`ErrorStateCard.vue`、`DashboardView.vue`、`PolicyView.vue`、`PolicyEditorSheet.vue`、`GroupCard.vue`、`usePolicy.ts`、`policyTypes.ts`、`NodesView.vue`、`nodeView.ts`、`useNodes.ts`、`ProbesView.vue`、`ProbeRunCard.vue`、`ProbeEvidenceSheet.vue`、`useProbes.ts`、`probeTypes.ts`、`PublicationsView.vue`、`usePublications.ts`、`publicationTypes.ts`、`SubscriptionsView.vue`、`SubscriptionConfigDrawer.vue`、`useSubscriptions.ts`、`SettingsView.vue` SHALL 移除硬编码英文文案，在 `'zh-CN'` 模式下将策略组类型（手动选择/自动测速/故障转移/负载均衡）、规则动作（允许/拒绝/隔离）、探针类型与状态、探测结论、风险等级、预检诊断与常见 API 错误码统一展示为清晰地道的中文。

#### Scenario: 首次访问默认展示全中文界面
- **WHEN** 用户在未设置 `localStorage.csp_locale` 的全新浏览器环境中打开系统各页面（控制台、订阅源、节点账本、探针引擎、策略树、订阅发布、全局配置）
- **THEN** 页面标题、导航菜单、按钮、抽屉弹窗、筛选器、空状态、徽章与提示信息全部以简体中文展示，无残留硬编码英文段落或裸露英文枚举

#### Scenario: 错误状态卡片与拓扑校验反馈中文展示
- **WHEN** 接口返回鉴权错误、冲突错误、校验错误或在策略树触发拓扑校验反馈
- **THEN** `ErrorStateCard.vue` 与策略树提示横幅展示中文标题、中文错误说明及中文操作按钮（“重试”、“前往设置 / 鉴权”）

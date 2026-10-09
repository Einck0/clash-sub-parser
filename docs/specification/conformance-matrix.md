# CSP 规格符合度审计矩阵 (Living Conformance Matrix)

- **基线版本**：HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **审计日期**：2026-10-09 (Epoch 3 Remediation)
- **审计角色**：Executor（施工机 · 规格整合特性包）
- **测试收据凭证**：`/tmp/csp-conformance-real-tests-receipt.json`
- **收据 SHA-256**：`14a67e565050ab68180bf31d3ce1f1bafc58e0cb55008190217098f7de0608fb`
- **实机自测执行统计**：收录 41 条标准测试命令，共计执行 **178 次**测试用例（125 top-level + 53 subtests），包含 **137 个独立唯一测试**（91 top-level + 46 subtests），退出码全部为 0，零 0-case 运行。

> **架构治理与审计边界披露 (Scope & Multi-Layer Disclosure)**：
> 1. **双层治理体系**：本矩阵覆盖持续主干能力体系的 **37 项全局功能需求 (FR-INV/POL/PROB/PUB/AUTH/UI)**；历史 26 个 Change 积累的 259 项需求和 463 个场景由独立工件 [`docs/specification/historical-requirement-register.md`](historical-requirement-register.md) 完整登记。**本矩阵中『符合』仅代表在 HEAD 实机代码上通过本地单元/契约测试，绝不代表已全量解决 63 份历史规格的所有存根需求**。
> 2. **命令目录与表格解耦**：为消除 Markdown 表格转义导致正则管道符 `|` 被逃逸为字面量 `\|` 的问题，所有测试命令统一收敛至 [`docs/specification/verification-commands.md`](verification-commands.md)（编号 `CV-001` ~ `CV-041`），矩阵表格仅保留 CV 链接与通过用例数。
> 3. **复用说明**：部分跨领域共享逻辑复用同一测试命令或测试套件（如 PolicyView 13 个用例同时覆盖 FR-POL-03 与 FR-UI-03；TestPublicationClientExportAuthModes 8 个用例同时覆盖 FR-PUB-04 与 FR-PUB-05）。此类复用严格登记为复用，绝不虚报为独立新用例。

---

## 汇总统计 (Summary of Conformance)

| 领域分类 | 规格数 | 符合 | 不符合 | 部分符合 | 未验证 | 待确认 |
|---|---|---|---|---|---|---|
| **Inventory (订阅与节点资产)** | 7 | 6 | 0 | 0 | 0 | 1 |
| **Policy (策略组与图解析器)** | 7 | 2 | 2 | 2 | 0 | 1 |
| **Probe (主动探测引擎)** | 7 | 5 | 1 | 1 | 0 | 0 |
| **Publication (发布与编译器)** | 6 | 5 | 0 | 1 | 0 | 0 |
| **Auth (鉴权与安全模型)** | 4 | 4 | 0 | 0 | 0 | 0 |
| **UI (控制台界面交互)** | 6 | 4 | 0 | 2 | 0 | 0 |
| **总计** | **37** | **26** | **3** | **6** | **0** | **2** |

---

## 领域分项审计详情

### 1. 订阅与节点资产领域 (Domain: Inventory)

| 功能需求 ID | 需求名称 | 来源级别 | 符合度 | 实机代码证据 (文件与行号) | 验证命令与结果 (附真实用例数) | 现实局限与技术细节说明 |
|---|---|---|---|---|---|---|
| **FR-INV-01** | 受控订阅生命周期与非敏感存储 | TIER-2 | **符合** | `internal/domain/subscription.go:20-60`<br>`internal/repository/sqlite/subscription.go:80-140` | [CV-001](verification-commands.md#cv-001) (exit 0, 1 passed)<br>[CV-002](verification-commands.md#cv-002) (exit 0, 1 passed) | 订阅创建、更新、禁用生命周期完整，秘密字段不暴露在公开接口；未核验线上生产订阅源。 |
| **FR-INV-02** | 抓取失败与零有效节点资产保全 | TIER-1 | **符合** | `internal/application/inventory/service.go:400-500` | [CV-003](verification-commands.md#cv-003) (exit 0, 4 passed: 1 top + 3 sub) | 当抓取失败或解析零节点时，不进入写入事务，存量节点与来源关联完整保留。 |
| **FR-INV-03** | 规范化传输身份与稳定 Logical ID | TIER-2 | **符合** | `internal/domain/node.go:50-95`<br>`internal/parser/subparser.go:120-180` | [CV-004](verification-commands.md#cv-004) (exit 0, 2 passed) | 跨刷新使用一致的传输参数哈希生成 `logical_id`，覆盖主流 7 种协议；名称变更不改变 ID。 |
| **FR-INV-04** | 多来源关联与历史版本台账 | TIER-2 | **符合** | `internal/repository/sqlite/node_sources.go:30-85`<br>`migrations/000010_node_source_history.sql:1-40` | [CV-005](verification-commands.md#cv-005) (exit 0, 1 passed)<br>[CV-006](verification-commands.md#cv-006) (exit 0, 1 passed) | 支持同一节点归属于多个订阅，提供历史变更记录与 source ledger 台账。 |
| **FR-INV-05** | 启用订阅作用域隔离查询 | TIER-2 | **符合** | `internal/repository/sqlite/node_predicates.go:41`<br>`internal/repository/sqlite/nodes.go:110-145` | [CV-007](verification-commands.md#cv-007) (exit 0, 1 passed) | 禁用订阅后，其专有节点在 `enabled_subscriptions` 作用域下被安全隔离，不进入导出候选集。 |
| **FR-INV-06** | 节点连接参数写时复制与人工覆写 | TIER-1 | **符合** | `internal/domain/node_connection.go:20-75`<br>`migrations/000008_node_connection_cow.sql:1-35` | [CV-008](verification-commands.md#cv-008) (exit 0, 1 passed) | 用户在前端修改节点连接参数后，存入独立 overrides 表，后续订阅刷新不覆盖人工改动。 |
| **FR-INV-07** | 订阅刷新数据修剪与重试策略 | TIER-3 | **待确认** | `internal/application/inventory/service.go:350-410, 1245-1250` | 源码审计、未独立执行自动化测试 | 当前代码无自动重试循环；抓取部分成功（Partial）时的来源修剪与隐藏作用域保护待用户裁决（问题组 5）。 |

---

### 2. 策略组与图解析器领域 (Domain: Policy & Resolver)

| 功能需求 ID | 需求名称 | 来源级别 | 符合度 | 实机代码证据 (文件与行号) | 验证命令与结果 (附真实用例数) | 现实局限与技术细节说明 |
|---|---|---|---|---|---|---|
| **FR-POL-01** | 直接将其他策略组作为节点引用 | TIER-1 | **部分符合** | `internal/domain/policy.go:31-77`<br>`internal/resolver/graph.go:167-177`<br>`internal/compiler/mihomo.go:991-1001` | [CV-009](verification-commands.md#cv-009) (exit 0, 1 passed) | Mihomo 目标支持直接组引用渲染；但非 Mihomo 目标由于纯节点协议忽略策略组，且混合顺序破坏，未能全平台统一支持。 |
| **FR-POL-02** | 解包获得子策略组叶节点 | TIER-1 | **不符合** | `internal/domain/policy.go:31-41`<br>`internal/resolver/graph.go:315-345` | [CV-010](verification-commands.md#cv-010) (exit 0, 1 passed) | **核心缺口**：Go 实现完全缺乏解包叶节点语义。`AllNodeLogicalIDs` 仅用于空组判断与诊断元数据，未参与出站代理成员展开（问题组 1）。 |
| **FR-POL-03** | 成员模式数据契约与前端编辑 | TIER-1 | **不符合** | `internal/transport/http/policies.go:25-45`<br>`web/src/features/policy/policyTypes.ts:210-225`<br>`web/src/features/policy/PolicyEditorSheet.vue:360-385` | [CV-011](verification-commands.md#cv-011) (exit 0, 13 passed) | 数据契约 `EdgeInput` 缺少 `mode`（`direct_ref` vs `unpack_nodes`），前端仅有单选按钮，无模式切换控制。 |
| **FR-POL-04** | 成员来源混合交错顺序保持 | TIER-2 | **部分符合** | `internal/resolver/graph.go:140-185` | [CV-012](verification-commands.md#cv-012) (exit 0, 1 passed) | 同类型边按 `Position` 保序；但解析器硬编码先遍历显式节点、后遍历子策略组，节点与子组间的交错相对顺序丢失，属已知解析器实现缺陷（问题组 2）。 |
| **FR-POL-05** | 空策略组 PASS 兜底与防越权隔离 | TIER-2 | **符合** | `internal/domain/policy.go:19`<br>`internal/resolver/graph.go:343-345`<br>`internal/compiler/mihomo.go:1002-1015` | [CV-013](verification-commands.md#cv-013) (exit 0, 2 passed) | Mihomo 对标记 `empty_fallback_pass` 的组输出 `proxies: [PASS]` 与 `empty-fallback: PASS`；未配置的空组输出 `REJECT`。 |
| **FR-POL-06** | 全池动态筛选与无上限全量解析 | TIER-2 | **符合** | `internal/repository/sqlite/policy.go:210-260`<br>`internal/resolver/graph.go:215-270` | [CV-014](verification-commands.md#cv-014) (exit 0, 1 passed)<br>[CV-015](verification-commands.md#cv-015) (exit 0, 1 passed) | 历史 50 节点截断已被 `ListAll` 取代，测试验证 250+ 节点场景可完整命中。 |
| **FR-POL-07** | 历史遗留策略组数据迁移恢复 | TIER-3 | **待确认** | `migrations/000001_initial_schema.sql:114-129`<br>`/tmp/csp-empty-groups-audit-20261009/evidence.json` | 源码审计、未独立执行自动化测试 | 历史 3 个轮询组因迁移淘汰 `group_nodes` 导致当前为 0 条边。恢复策略需用户在问题清单中定夺（问题组 3）。 |

---

### 3. 主动探测引擎领域 (Domain: Probe)

| 功能需求 ID | 需求名称 | 来源级别 | 符合度 | 实机代码证据 (文件与行号) | 验证命令与结果 (附真实用例数) | 现实局限与技术细节说明 |
|---|---|---|---|---|---|---|
| **FR-PROB-01** | 原生 Mihomo 探针执行引擎 | TIER-2 | **符合** | `internal/application/probe/runner.go:196`<br>`internal/probe/mihomo/adapter.go:100,150,215,252` | [CV-016](verification-commands.md#cv-016) (exit 0, 6 passed: 1 top + 5 sub) | 生产 runner 使用 Mihomo dialer 并完整支持 `skip-cert-verify`。历史 `safe_dialer.go` 已被安全移除。 |
| **FR-PROB-02** | 多层级探测周期解耦架构 | TIER-2 | **符合** | `internal/application/probe/periodic.go:18-19`<br>`internal/domain/probe_schedule.go:23`<br>`internal/domain/node_filter.go:353` | [CV-017](verification-commands.md#cv-017) (exit 0, 1 passed)<br>[CV-018](verification-commands.md#cv-018) (exit 0, 1 passed) | 1m 扫描协调、5m 失败冷却、1h 调度周期、24h 有效期各司其职，无内在冲突。注释中的 10m 为过时历史说明。 |
| **FR-PROB-03** | 单节点插队与优先队列调度 | TIER-2 | **符合** | `internal/probe/queue/queue.go:210-250`<br>`internal/transport/http/probes.go:140-180` | [CV-019](verification-commands.md#cv-019) (exit 0, 1 passed) | 前端点击测试单节点时，请求通过 `EnqueueManualPreemptFront` 插队至探测池顶端优先调度。 |
| **FR-PROB-04** | 探测观测证据与健康基线判定 | TIER-2 | **符合** | `internal/domain/probe_health.go:15-80`<br>`internal/application/probe/runner.go:1540-1560` | [CV-020](verification-commands.md#cv-020) (exit 0, 2 passed) | 结合 RTT 延迟、错误码及观测时效生成 healthy / degraded / unhealthy / undetermined 状态。 |
| **FR-PROB-05** | 拨号失败原因分类归因精确性 | TIER-2 | **部分符合** | `internal/application/probe/runner.go:965-970, 1139` | [CV-021](verification-commands.md#cv-021) (exit 0, 9 passed: 1 top + 8 sub) | 已知错误分类完整，但未知错误与 client 为 nil 时默认归因 `client_build_failed`，存在归因偏差。 |
| **FR-PROB-06** | 观测数据防未来时间穿越校验 | TIER-2 | **不符合** | `internal/domain/node_filter.go:359`<br>`internal/domain/probe_health.go:20` | [CV-022](verification-commands.md#cv-022) (exit 0, 6 passed: 1 top + 5 sub) | `probe_health.go` 有 `obs.ObservedAt.After(now)` 保护，但 `node_filter.go` 缺少未来时间守卫，负时间差导致未来时间绕过新鲜度截断。 |
| **FR-PROB-07** | 辅助 Sing-box 探针构造器隔离 | TIER-2 | **符合** | `internal/probe/singbox/builder.go:23-28` | [CV-023](verification-commands.md#cv-023) (exit 0, 7 passed: 1 top + 6 sub) | Sing-box builder 显式拦截非安全传输并返回 `insecure_tls`，其属于离线/辅助构造器，不阻断主线 Mihomo runner。已纠正先前 0-case 伪命令，采用真实 `TestBuildOptionsRejectsUnsafeProtocolOptions` 闭环验证。 |

---

### 4. 发布与编译器领域 (Domain: Publication & Compilers)

| 功能需求 ID | 需求名称 | 来源级别 | 符合度 | 实机代码证据 (文件与行号) | 验证命令与结果 (附真实用例数) | 现实局限与技术细节说明 |
|---|---|---|---|---|---|---|
| **FR-PUB-01** | 原生 Mihomo 完整拓扑发布导出 | TIER-2 | **符合** | `internal/compiler/mihomo.go:60-150, 950-1030` | [CV-024](verification-commands.md#cv-024) (exit 0, 1 passed) | 完整生成 proxies、proxy-groups、rules，支持子策略组嵌套与规则拓扑映射。 |
| **FR-PUB-02** | 非 Mihomo 纯节点导出协议边界 | TIER-1 | **符合** | `internal/compiler/singbox.go:40-115`<br>`internal/compiler/surge.go:60-80`<br>`internal/compiler/qx.go:55-80` | [CV-025](verification-commands.md#cv-025) (exit 0, 3 passed) | Sing-box、Surge、Quantumult X 严格遵循仅导出已准入节点协议，不导出策略组或规则。 |
| **FR-PUB-03** | 非 Mihomo 遇到空策略组校验一致性 | TIER-2 | **部分符合** | `internal/compiler/compiler.go:160-168` | [CV-026](verification-commands.md#cv-026) (exit 0, 5 passed: 1 top + 4 sub) | 编译器 `validate()` 在渲染前一刀切拦截所有含 `UsesEmptyPass` 组的非 Mihomo 目标，但由于后续实际只渲染节点，存在语义冲突（问题组 4）。 |
| **FR-PUB-04** | 发布端点免密公开访问与专属 Token | TIER-1 | **符合** | `internal/transport/http/publications.go:180-230`<br>`internal/application/publication/service.go:110-160` | [CV-027](verification-commands.md#cv-027) (exit 0, 8 passed: 1 top + 7 sub) | `export_auth_enabled == false` 时允许匿名拉取；为 true 时凭专属发布 Token 校验通过。 |
| **FR-PUB-05** | 导出端点统一系统主令牌复用 | TIER-1 | **符合** | `internal/transport/http/publications.go:210-225`<br>`internal/transport/http/token.go:70-95` | [CV-028](verification-commands.md#cv-028) (exit 0, 8 passed: 1 top + 7 sub) | 当导出需要鉴权时，既支持专属 Token，也支持输入管理端配置的统一系统令牌。 |
| **FR-PUB-06** | 历史 `/p/{id}` 路径兼容路由 | TIER-2 | **符合** | `internal/transport/http/router.go:95-105` | [CV-029](verification-commands.md#cv-029) (exit 0, 7 passed: 1 top + 6 sub) | 挂载 `/p/{publication_id}` 兼容路由，重定向或对齐 `/publish/v1/{publication_id}`，防止旧客户端失效。 |

---

### 5. 鉴权与安全模型领域 (Domain: Auth & Security)

| 功能需求 ID | 需求名称 | 来源级别 | 符合度 | 实机代码证据 (文件与行号) | 验证命令与结果 (附真实用例数) | 现实局限与技术细节说明 |
|---|---|---|---|---|---|---|
| **FR-AUTH-01** | 管理端与导出链接独立鉴权开关 | TIER-1 | **符合** | `migrations/000013_auth_settings.sql:1-10`<br>`internal/domain/settings.go:30-45`<br>`internal/transport/http/token.go:40-65` | [CV-030](verification-commands.md#cv-030) (exit 0, 1 passed)<br>[CV-031](verification-commands.md#cv-031) (exit 0, 1 passed) | 数据库支持 `admin_auth_enabled` 与 `export_auth_enabled`，双开关物理独立解耦。 |
| **FR-AUTH-02** | 统一系统主令牌配置与共享 | TIER-1 | **符合** | `internal/transport/http/settings.go:85-120`<br>`internal/transport/http/token.go:70-85` | [CV-032](verification-commands.md#cv-032) (exit 0, 1 passed)<br>[CV-033](verification-commands.md#cv-033) (exit 0, 1 passed) | 系统统一配置单一密码字符串，管理端登录与导出拉取共用该令牌校验。 |
| **FR-AUTH-03** | 明文敏感信息存储模型与安全警示 | TIER-1 | **符合** | `internal/repository/sqlite/node_connection.go:45-80`<br>`internal/repository/sqlite/subscription.go:50-75` | [CV-034](verification-commands.md#cv-034) (exit 0, 1 passed) | 遵照用户要求直接明文存储。系统在安全规范中明确保密责任与生产网络隔离边界。 |
| **FR-AUTH-04** | 控制面凭据生命周期与会话 Cookie | TIER-2 | **符合** | `internal/transport/http/middleware_auth.go:35-85`<br>`internal/transport/http/auth.go:40-90` | [CV-035](verification-commands.md#cv-035) (exit 0, 4 passed: 2 top + 2 sub) | 管理端支持 Authorization Bearer Header 与 HTTP-Only Session Cookie 双模式校验，登出注销。 |

---

### 6. 控制台界面交互领域 (Domain: Admin UI)

| 功能需求 ID | 需求名称 | 来源级别 | 符合度 | 实机代码证据 (文件与行号) | 验证命令与结果 (附真实用例数) | 现实局限与技术细节说明 |
|---|---|---|---|---|---|---|
| **FR-UI-01** | 节点列表紧凑布局与真实视口适配 | TIER-1 | **符合** | `web/src/features/nodes/NodesView.vue:520-580` | [CV-036](verification-commands.md#cv-036) (exit 0, 12 passed) | 采用弹性网格与紧凑列表，移除机械 44px 限制，真实视口下无溢出或遮挡。 |
| **FR-UI-02** | 节点健康状态筛选与分页交互 | TIER-2 | **部分符合** | `web/src/features/nodes/useNodes.ts:133-140`<br>`web/src/features/nodes/NodesView.vue:154-180, 412-435` | [CV-037](verification-commands.md#cv-037) (exit 0, 8 passed) | `useNodes.ts` 支持 `params.health_status`；但 `NodesView.vue` 切换筛选时客户端过滤并通过 `checkViewportStarvation` 最多翻 3 页补足，属已知实现缺口，待重构为服务端分页联动（问题组 6）。 |
| **FR-UI-03** | 策略组抽屉式编辑工作台 | TIER-2 | **部分符合** | `web/src/features/policy/PolicyEditorSheet.vue:1-400` | [CV-038](verification-commands.md#cv-038) (exit 0, 13 passed) | 抽屉式侧边栏编辑，支持动态条件表单与成员选择，但缺乏解包模式切换。 |
| **FR-UI-04** | 订阅发布管理与复制直链向导 | TIER-2 | **符合** | `web/src/features/publications/PublicationsView.vue:120-200` | [CV-039](verification-commands.md#cv-039) (exit 0, 27 passed) | 零记录时展示引导，根据导出鉴权开关动态呈现免密纯净链接或带 Token 受控链接。 |
| **FR-UI-05** | 节点连接参数人工覆写详情抽屉 | TIER-1 | **符合** | `web/src/features/nodes/NodesView.vue:200-240, 950-1050` | [CV-040](verification-commands.md#cv-040) (exit 0, 8 passed) | 详情抽屉中支持直接编辑协议字段（端口、密码、UUID 等），提交 PATCH 保存为 overrides。 |
| **FR-UI-06** | 国际化双语支持 (zh-CN / en-US) | TIER-2 | **符合** | `web/src/locales/zh-CN.json`<br>`web/src/locales/en-US.json` | [CV-041](verification-commands.md#cv-041) (exit 0, 4 passed) | 补齐管理端各视图关键术语双语对照，无遗漏缺失键报错。 |

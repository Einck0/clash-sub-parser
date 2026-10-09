## Why

根据用户在实机使用中的明确反馈——**“探针引擎那块的前端需要完全设计，完全不符合人类直觉”**，结合前后端代码与运行态取证，定位到当前探针引擎存在两大类根本问题：

1. **前端信息架构与交互背离人类直觉**：
   - 当前 `web/src/features/probes/ProbesView.vue` 将后台异步队列任务对象（`ProbeRun` UUID、`actor_scope`、`config_revision`、`lease_until`、`generation`、`sha256` 证据摘要）直接当成主界面卡片展示，却完全看不到**哪些节点可用、每个节点延迟多少毫秒、流媒体/AI 是否解锁**；
   - 发起测速弹窗强迫用户填写内部概念 `config_revision` 与截止分钟数，不支持一键全量测速，也不支持按节点勾选或按筛选结果定点测速；
   - 定时测速配置要求用户手填原始秒数（60~604800 秒）并暴露分布式租约代数/Owner 等底层实现噪音；
   - 证据抽屉 `ProbeEvidenceSheet.vue` 直接渲染机器日志字符串 `profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=42` 与原始 `node_logical_id` 哈希，没有节点名称与结构化人类可读卡片。

2. **后端节点读模型与探针执行器存在断线与硬编码缺陷**：
   - `internal/application/inventory/service.go` 虽然已注入 `s.probeObsRepo`（`cmd/csp/main.go:190`）与 `s.sources`，但在 `ListNodesReadModel` 与 `GetNodeDetailWithRisk`（以及 `internal/transport/http/nodes.go` 的 `get`/`patchConnection`）构建 `NodeView` 时从未调用 `s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil)` 与 `s.sources.ListByNodes(ctx, nodeIDs)`，导致前端获取到的节点列表永远缺失 `latency_ms`、`last_probed_at`、`health_status`、`probe_missing`、`probe_stale`、`capabilities` 与 `sources`，在节点台账与探针工作台均显示为“未探测”；
   - `internal/application/probe/runner.go` 在 `nodeIDs` 为空（全量测速）时调用 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true})` 未分页拉取，仅拿到第一页默认 50 个节点；同时 `DefaultRunBudget.MaxTasks = 512` 导致超过 85 个节点的多维测速直接报错 `probe run task budget exceeded` 并将整个 Run 置为 `failed`；且 `executeTaskWithVerdict` 对 `domain.ProbeKindSpeed` 硬编码短路返回 `ErrSpeedProbeOptInRequired`，即便用户显式勾选 `speed` 也无法执行真实测速。

## What Changes

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

## Capabilities

### New Capabilities

- `probe-engine-ux`: 探针引擎后端节点探测读模型打通、全量测速与带宽测速执行器修复、以及前端以“节点与人类直觉”为核心的测速与可用性工作台全量重构契约。

### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `probe-engine-ux` 能力规格建立完整契约）。

## Impact

- 后端：`internal/application/inventory/**`、`internal/application/probe/**`、`internal/transport/http/{nodes,probes}*.go`
- 前端：`web/src/features/probes/**`、`web/src/features/nodes/**`、`web/src/locales/**`
- 验证边界：仅在本地单测与隔离预览环境（临时 SQLite 库、独立端口与临时构建目录）验证，严禁触碰生产容器。

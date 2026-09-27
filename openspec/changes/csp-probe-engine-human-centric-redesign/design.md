## Context

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

## Goals / Non-Goals

**Goals:**
- 在 `inventory.Service` 与 `transport/http/nodes.go` 中完整打通 `NodeView` 的最新探测状态（`latency_ms`、`last_probed_at`、`health_status`、`probe_missing`、`probe_stale`、`capabilities`）与订阅来源（`sources`）；
- 在 `probe.DefaultRunner` 中支持全量活跃节点自动分页拉取、用户显式包含 `speed` 时开启 `OptIn: true` 执行真实测速、以及放宽默认任务预算（支持大批量节点按并发信号量受控执行，同时保留 `WithRunBudget` 显式注入自定义预算时的单测可测性）；
- 将 `web/src/features/probes/**` 彻底重构为以“人类直觉”和“节点”为核心的测速与可用性工作台（KPI 看板 + 一键测速与探测项胶囊工具栏 + 节点实时测速表格 + 预设周期定时配置与折叠历史 + 结构化人类可读测速详情抽屉）；
- 在 `web/src/features/nodes/**` 中同步联动展示节点真实延迟、健康度、流媒体/AI 解锁状态与单节点快速测速入口。

**Non-Goals:**
- 不改动底层 SQLite 表结构（复用现有 `probe_observations`、`probe_runs`、`probe_schedules`、`probe_batches`、`node_sources` 表与既有 Repository 接口）；
- 不触碰生产运行容器，所有验证均在本地测试与隔离预览实例中完成。

## Decisions

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

### 2. 后端 `probe.DefaultRunner` 全量分页、预算放宽与 `speed` Opt-In 修复（`internal/application/probe/runner.go`）
- **全量活跃节点分页拉取**：
  - 当 `len(nodeIDs) == 0` 时，参照 `periodic.go:414-431` 循环分页调用 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true, Pagination: domain.Pagination{Page: page, PageSize: 100}})` 直至 `len(targetNodes) >= total || len(chunk) == 0`，确保全量测速覆盖全部活跃节点而非仅前 50 个。
- **显式 `speed` 探测 Opt-In 执行**：
  - 当调用方传入的 `validKinds` 中包含 `domain.ProbeKindSpeed` 时，表示用户/调度器已显式勾选 `speed` 测速；
  - 在 `executeTaskWithVerdict` 中移除原先 `if kind == domain.ProbeKindSpeed` 的无条件短路报错分支；对 `domain.ProbeKindSpeed` 正常通过 `r.dialer(ctx, node)` 建立代理连接并请求 `probeURLForKind(domain.ProbeKindSpeed)`（应用 `prof.SpeedBudget.Deadline` 与 `prof.SpeedBudget.MaxBytesPerRequest` 限流读取），将 `result.OptIn = true` 传给 `prof.Evaluate(result)`，并在 `RedactedSummary` 中记录 `bytes_read` 与 `throughput_kbps` 等摘要信息。
  - 同步更新 `internal/application/probe/stage_gate_test.go` 中旧的 `TestStageGateSpeedRejectedBeforeDialOrHTTP`，验证显式请求 `ProbeKindSpeed` 时正常拨号、携带 `OptIn: true` 完成测速观测并受 `SpeedBudget` 约束。
- **单次运行任务预算放宽**：
  - 将 `DefaultRunBudget.MaxTasks` 从 `512` 提升至 `65536`（底层实际并发度已由 `queue.Scheduler` 的 `DefaultConcurrency` 信号量严格限制，不会产生无界 goroutine 膨胀），从而支持上千活跃节点 × 6 维度的全量一键测速直接完成，同时保留 `WithRunBudget` 注入小预算时的边界测试行为。

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

### 4. 节点台账页（`web/src/features/nodes/**`）联动增强
- `web/src/features/nodes/nodeView.ts`：
  - 扩展 `NodeRecord` 与 `NormalizedNode` 支持 `latency_ms?: number | null`、`last_probed_at?: string | null`、`health_status?: 'healthy' | 'degraded' | 'unhealthy' | 'unknown' | 'missing'`，以及 `capabilities` 既支持旧的字符串枚举（兼容现有单测）又支持后端新返回的结构化对象 `{ verdict, latency_ms, observed_at, summary, stale }`；
  - 新增 `formatNodeLatency(node)`、`formatRelativeTime(iso)` 等辅助函数；
- `web/src/features/nodes/NodesView.vue`：
  - 在节点列表与详情抽屉中直接展示后端返回的真实健康状态徽章、延迟毫秒数（如 `42 ms`）、流媒体与 AI 解锁状态，并在节点详情抽屉底部或头部提供「⚡ 立即测速」按钮（直接调用 `/api/v1/probes/runs` 针对该节点发起测速并刷新节点详情）。

## Risks / Trade-offs

- **`nodeView.ts` 前向与后向兼容**：
  - 现有 `web/src/features/nodes/nodeView.test.ts` 中存在直接传入 `{ capabilities: { streaming: 'available', ai: 'restricted' } }` 的单测用例。`nodeView.ts` 在 `normalizeNode` 与 `nodeCapabilityLabel` 中同时兼容字符串值与 `{ verdict, latency_ms, observed_at, summary, stale }` 对象值，确保旧测试与新后端响应双双零报错通过。
- **`stage_gate_test.go` 带宽测速单测语义更新**：
  - 原 `TestStageGateSpeedRejectedBeforeDialOrHTTP` 测试的是旧版硬编码拒绝 `speed` 的行为。在修复为“当 `run.Kinds` 显式包含 `domain.ProbeKindSpeed` 时即视为用户已 Opt-In”后，需同步更新 `stage_gate_test.go` 验证显式传入 `ProbeKindSpeed` 时正常执行拨号与 `OptIn: true` 评估，而 `profiles.Speed().Evaluate(profiles.Result{OptIn: false})` 单元测试继续验证未 Opt-In 时的契约保护。

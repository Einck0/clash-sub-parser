## Purpose

打通后端 `NodeView` 的最新探测状态与订阅源读模型，修复手动全量测速截断、512 任务预算报错与带宽测速（`speed`）显式勾选报错，并将探针引擎前端（`web/src/features/probes/**`）彻底重构为以“人类直觉”和“节点”为核心的实时测速与可用性工作台，同时实现与节点台账页（`web/src/features/nodes/**`）的探测状态联动。

## ADDED Requirements

### Requirement: 后端节点读模型（`NodeView`）完整填充最新探测状态与订阅来源
后端 `inventory.Service` 在 `ListNodesReadModel` 与 `GetNodeDetailWithRisk`（以及 `internal/transport/http/nodes.go` 的 `GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}`、`PATCH /api/v1/nodes/{logical_id}/connection`）构建 `NodeView` 时 SHALL 调用 `s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil)` 与 `s.sources.ListByNodes(ctx, nodeIDs)`，并在 `NodeView` 上完整填充以下字段：
1. `latency_ms` (`*int64`)：优先取该节点最新 `baseline` 观测的 `LatencyMS`，若无 `baseline` 观测则取最新有效观测延迟；无任何观测时为 `nil`。
2. `last_probed_at` (`*time.Time`)：取该节点所有探测维度中最新一次观测的 `ObservedAt`；无任何观测时为 `nil`。
3. `health_status` (`string`)：基于最新 `baseline`（或综合）观测派生：`available` -> `"healthy"`，`restricted`/`stale` -> `"degraded"`，`error` -> `"unhealthy"`，无任何观测记录 -> `"unknown"`。
4. `probe_missing` (`bool`)：该节点无任何探测观测记录时 SHALL 为 `true`，否则为 `false`。
5. `probe_stale` (`bool`)：当存在探测记录且距离 `last_probed_at` 超过保鲜阈值（1 小时）时 SHALL 为 `true`，否则为 `false`。
6. `capabilities` (`map[string]CapabilityStatus`)：按 `ProbeKind` 填充结构化状态 `{ verdict, latency_ms, observed_at, summary, stale }`。
7. `sources` (`[]domain.NodeSource`)：填充该节点关联的订阅源列表，供前端展示节点来源及按订阅源筛选测速。

#### Scenario: 节点存在最新探测观测与订阅源时查询节点列表
- **WHEN** 某节点已关联订阅源 `sub-01`，且已完成 `baseline`（`available`, `42ms`）与 `streaming`（`available`, `85ms`）探测观测，客户端请求 `GET /api/v1/nodes` 或 `GET /api/v1/nodes/{logical_id}`
- **THEN** 返回的 `NodeView` 中 `latency_ms` 为 `42`、`health_status` 为 `"healthy"`、`probe_missing` 为 `false`、`probe_stale` 为 `false`、`capabilities.baseline.verdict` 为 `"available"`、`capabilities.streaming.verdict` 为 `"available"`，且 `sources` 包含 `sub-01`

#### Scenario: 节点尚无任何探测记录时查询节点列表
- **WHEN** 新入库节点尚无任何 `probe_observations` 记录，客户端请求 `GET /api/v1/nodes`
- **THEN** 返回的 `NodeView` 中 `probe_missing` 为 `true`、`health_status` 为 `"unknown"`、`latency_ms` 与 `last_probed_at` 为空

### Requirement: 手动探测支持全量活跃节点分页拉取、大批量任务预算与显式 `speed` 带宽测速
后端 `probe.DefaultRunner`（`internal/application/probe/runner.go`）在执行手动或周期探测任务时 SHALL 满足：
1. **全量活跃节点完整拉取**：当 `nodeIDs` 为空（全量测速）时，`Runner.Run` SHALL 按页循环查询 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true, Pagination: ...})` 拉取全部活跃节点，不得仅拉取默认第一页（50 个节点）。
2. **显式勾选 `speed` 即视为 Opt-In**：当 `run.Kinds` 中显式包含 `domain.ProbeKindSpeed` 时，`executeTaskWithVerdict` SHALL 视为用户已显式 Opt-In 带宽测速，通过 `r.dialer` 建立节点代理连接并发起测速请求，以 `Result{OptIn: true, ...}` 评估结果并持久化 `ProbeObservation`，不得硬编码返回 `ErrSpeedProbeOptInRequired`。
3. **支持全量节点多维并发受控测速**：默认运行预算 `DefaultRunBudget.MaxTasks` SHALL 放宽以支持全量活跃节点的多维探测按调度器并发信号量依次执行，不再因 `len(targetNodes) * len(validKinds) > 512` 直接将整个 `ProbeRun` 标为 `failed`。

#### Scenario: 活跃节点超过 50 个且多维任务数超过 512 时发起一键全量测速
- **WHEN** 系统中存在 120 个活跃节点，用户发起 `node_logical_ids: []` 且包含 5 个探测维度（共 600 个子任务）的全量测速
- **THEN** `DefaultRunner` 分页拉取全部 120 个活跃节点并按并发信号量完成探测，Run 状态正常流转为 `succeeded` 而非因 50 条分页截断或 512 预算上限失败

#### Scenario: 用户显式勾选带宽测速（`speed`）发起探测
- **WHEN** 用户在 `kinds` 中显式包含 `"speed"` 发起探测任务且节点代理连通正常
- **THEN** `DefaultRunner` 通过节点代理请求测速端点并以 `OptIn: true` 完成评估，生成 `verdict: "available"`（或实际网络结论）与非零延迟的 `speed` 观测记录

### Requirement: 探针引擎前端重构为以“人类直觉”和“节点”为核心的测速与可用性工作台
前端 `web/src/features/probes/**` SHALL 彻底重构为三层视觉架构的节点测速工作台，并消除面向机器的底层实现黑话：
1. **第一视觉层级：全景节点健康与延迟统计看板（KPI Summary Bar）**：
   - 页面顶部 SHALL 展示 4 张核心人类指标卡片：
     * **在线可用率**：`可用节点数 / 总活跃节点数`（含百分比进度条与正常/降级/异常/未测分布）；
     * **平均响应延迟**：全网可用节点平均 `ms`（含极速 `<100ms`、良好 `100-250ms`、较慢 `>250ms` 分布）；
     * **流媒体与 AI 解锁**：已解锁流媒体节点数 / 已支持 AI 节点数；
     * **定时自动测速状态**：当前开关状态（支持一键切换开/关）、易读定时间隔（如“每 1 小时”）、下次自动测速时间、以及「调整定时策略」快捷入口。
2. **第二视觉层级：直观的一键测速与多维筛选控制栏（Action & Filter Toolbar）**：
   - SHALL 提供主操作按钮 **「⚡ 一键全量测速」**（无需弹窗填写 `config_revision`，直接按当前开启的探测维度对全部活跃节点发起测速，并在顶部显示实时测速进度条与取消按钮）与 **「🎯 测速已选节点 (N)」**（对表格勾选节点或当前筛选结果发起定点测速）；
   - SHALL 在工具栏直接提供可点击开启/关闭的**探测维度胶囊开关（Probe Dimension Pills）**：`⚡ 连通与延迟 (必选)`、`🎬 流媒体解锁`、`🤖 AI 服务可用性`、`🛡️ IP 风险评估`、`🌍 落地地区识别`、`🚀 带宽测速`；
   - SHALL 支持按**关键词（节点名/服务器）**、**协议（SS/VMess/VLESS/Trojan/Hysteria2/WireGuard/TUIC）**、**订阅源**、**健康状态（全部 / 正常 / 降级 / 异常 / 未测速）**过滤，以及**按延迟排序（从快到慢 / 从慢到快）**。
3. **第三视觉层级：以“节点”为主体的实时测速结果表格/卡片列表（Node Probe Workbench Table）**：
   - 页面主体 SHALL 展示节点测速结果列表，每行包含：复选框（支持全选/多选定点测速）、**节点名称 (`display_name`)** + 协议标签 + 服务器地址（`server:port`）、**带颜色语义的连通状态与延迟 (`latency_ms`) 徽章**、**能力矩阵徽章（流媒体 / AI / 地区 / IP 风险）**、**人性化最后测速时间**，以及行级 **「⚡ 立即重测」** 与 **「查看详情」** 操作按钮。
4. **人性化定时自动测速配置与后台任务历史折叠区**：
   - 定时测速配置 SHALL 提供预设周期选择按钮（`每 15 分钟`、`每 30 分钟`、`每 1 小时`、`每 6 小时`、`每 12 小时`、`每天` 及自定义分钟数）+ 探测项目勾选 + 一键启用/停用；
   - 后台任务执行历史（Runs / Batches）SHALL 收敛为可切换次级视图或折叠面板，卡片标题采用人类可读的 **「手动测速任务 / 自动定时批次 · HH:mm:ss」**，彻底隐藏内部 UUID、Actor Scope、`config_revision`、Lease 租约、SHA256 证据指纹等底层噪音。
5. **结构化人类可读的测速详情抽屉（`ProbeEvidenceSheet.vue`）**：
   - 点击节点的「查看详情」或任务的「查看结果」时，抽屉顶部 SHALL 展示**节点名称 (`display_name`)**、协议、服务器地址与当前综合状态，将原始 `profile=... version=... verdict=... reason=... status=... latency_ms=...` 解析转换为**结构化人类可读卡片**（检测项目、结论、响应延迟、中文状态说明、检测时间），并支持在抽屉内一键「重新测速此节点」。

#### Scenario: 用户在探针工作台直观查看节点延迟并一键发起全量或单节点测速
- **WHEN** 用户进入“探针引擎”页面
- **THEN** 页面顶部展示 4 张核心 KPI 统计卡片，中部工具栏展示「⚡ 一键全量测速」与 6 个探测维度胶囊开关，主体区域以节点列表展示每个节点的名称、协议、`server:port`、彩色延迟毫秒徽章及流媒体/AI/IP 风险能力状态；点击「⚡ 一键全量测速」或某行的「⚡ 立即重测」即可无需填写 `config_revision` 直接启动测速并实时刷新进度与节点延迟

#### Scenario: 用户打开节点测速详情抽屉查看人类可读报告
- **WHEN** 用户点击任意节点行的「查看详情」按钮打开 `ProbeEvidenceSheet.vue`
- **THEN** 抽屉顶部显示该节点的 `display_name`、协议与服务器地址，各探测维度的观测记录以结构化中文卡片展示（包含检测项目名称、结论徽章、延迟 `ms`、中文状态说明如“HTTP 204 连通正常”及检测时间），不裸露 `profile=baseline version=baseline-v1` 机器串或内部 SHA256 指纹，且可点击「重新测速此节点」立即重测

### Requirement: 节点台账页（`NodesView.vue`）探测状态与延迟联动
前端节点台账页（`web/src/features/nodes/{NodesView.vue,nodeView.ts,useNodes.ts}`）SHALL 与后端返回的最新节点探测读模型实时联动：
1. `nodeView.ts` SHALL 兼容解析后端 `NodeView` 的 `latency_ms`、`last_probed_at`、`health_status`、`probe_missing`、`probe_stale` 及结构化 `capabilities` 对象（同时向后兼容字符串格式 `capabilities` 单测夹具）。
2. `NodesView.vue` 节点列表与详情抽屉 SHALL 直接展示真实的节点健康状态、延迟毫秒数、流媒体与 AI 解锁徽章，并在节点详情抽屉中提供一键触发单节点测速或跳转查看测速详情的能力。

#### Scenario: 节点台账展示真实探测延迟并在详情抽屉触发单节点测速
- **WHEN** 用户打开“节点台账”页面查看已完成探测的节点，或打开节点详情抽屉点击单节点测速
- **THEN** 节点列表直接显示真实的健康状态（正常/降级/异常）、延迟毫秒数及流媒体/AI 解锁徽章，抽屉内可一键对该节点触发测速并刷新状态

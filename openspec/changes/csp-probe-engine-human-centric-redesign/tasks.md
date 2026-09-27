## 1. Phase 1 — 可并发扇出前后端特性包（写集合互不重叠）

- [x] 1.1 **Task 1.1（后端：打通 `NodeView` 最新探测状态与订阅源读模型 + 修复全量探测分页截断、任务预算与 Speed 探测开关）**（写集合：`internal/application/inventory/**`, `internal/application/probe/**`, `internal/transport/http/{nodes,probes}*.go`）：
  - 在 `internal/application/inventory/service.go`（及 `internal/transport/http/nodes.go`）中扩展 `NodeView` 与 `CapabilityStatus`：在 `ListNodesReadModel` 与 `GetNodeDetailWithRisk` 构建 `NodeView` 时调用已注入的 `s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil)` 与 `s.sources.ListByNodes(ctx, nodeIDs)`，完整填充 `latency_ms`（优先取最新 `baseline` 观测 `LatencyMS`，若无则取最新有效观测延迟）、`last_probed_at`（最新探测时间）、`health_status`（`healthy` / `degraded` / `unhealthy` / `unknown`）、`probe_missing`、`probe_stale`（超过 1 小时保鲜阈值为 `true`）、`capabilities`（各 `ProbeKind` 的 `{ verdict, latency_ms, observed_at, summary, stale }`）与 `sources`（`[]domain.NodeSource`）；确保 `GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}` 与 `PATCH /api/v1/nodes/{logical_id}/connection` 均返回填充完整的 `NodeView`。
  - 在 `internal/application/probe/runner.go` 中修复三项探针执行缺陷：
    1. 当 `nodeIDs` 为空（全量测速）时，按页循环调用 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true, Pagination: ...})` 拉取全部活跃节点（而非只取第一页默认 50 个）；
    2. 当 `run.Kinds` 中显式包含 `domain.ProbeKindSpeed` 时，视为用户已显式 Opt-In 带宽测速，通过 `r.dialer` 建立代理连接并以 `Result{OptIn: true, ...}` 执行真实带宽测速评估，不再硬编码 `OptIn: false` 返回 `ErrSpeedProbeOptInRequired`；
    3. 放宽 `DefaultRunBudget.MaxTasks`（允许全量活跃节点的多维探测按调度器并发信号量依次执行，不再因超过 512 直接将整个 Run 标为 `failed`）。
  - 补充并更新 `internal/application/inventory/read_model_test.go`、`internal/application/probe/{runner_test,stage_gate_test}.go`、`internal/transport/http/{nodes_test,probes_test}.go` 单元测试，运行 `go test ./internal/application/inventory/... ./internal/application/probe/... ./internal/transport/http/...` 与 `go build ./...` 全部通过（exit 0）。

- [x] 1.2 **Task 1.2（前端：彻底重构探针引擎为以节点为核心的人类直觉测速工作台 + 节点台账探测状态联动）**（写集合：`web/src/features/probes/**`, `web/src/features/nodes/**`, `web/src/locales/**`）：
  - 彻底推平重写 `web/src/features/probes/{ProbesView.vue,ProbeRunCard.vue,ProbeEvidenceSheet.vue,useProbes.ts,probeTypes.ts}` 并更新 `web/src/locales/{zh-CN.ts,en-US.ts,messages.ts}`：
    1. **第一视觉层级（全景节点健康与延迟统计看板 KPI Summary Bar）**：展示「在线可用率（可用/总活跃节点数 + 百分比进度条 + 正常/降级/异常/未测分布）」、「平均响应延迟（平均 ms + 极速 `<100ms` / 良好 `100-250ms` / 较慢 `>250ms` 分布）」、「流媒体与 AI 解锁统计」、「定时自动测速状态（一键开关、定时间隔、下次测速时间、调整定时策略入口）」4 张核心指标卡片；
    2. **第二视觉层级（直观的一键测速与多维筛选控制栏 Action & Filter Toolbar）**：提供「⚡ 一键全量测速」（无需弹窗填 `config_revision`，一键按当前勾选探测维度对全部活跃节点测速并在顶部展示实时测速进度条与取消按钮）、「🎯 测速已选节点 (N)」、直观可点击切换的「探测维度胶囊开关（`⚡ 连通与延迟 (必选)`、`🎬 流媒体解锁`、`🤖 AI 服务可用性`、`🛡️ IP 风险评估`、`🌍 落地地区识别`、`🚀 带宽测速`）」，以及按关键词（节点名/服务器）、协议、订阅源、健康状态过滤与按延迟排序（从快到慢 / 从慢到快）；
    3. **第三视觉层级（以“节点”为主体的实时测速结果表格/卡片列表 Node Probe Workbench Table）**：每行展示复选框（全选/多选定点测速）、节点名称 (`display_name`) + 协议标签 + 服务器地址 (`server:port`)、带颜色语义的连通状态与延迟 (`latency_ms`) 徽章、能力矩阵徽章（流媒体 / AI / 地区 / IP 风险）、人性化最后测速时间，以及行级「⚡ 立即重测」与「查看详情」按钮；
    4. **人性化定时自动测速配置与后台任务历史折叠区**：定时策略提供预设周期选择按钮（每 15 分钟 / 30 分钟 / 1 小时 / 6 小时 / 12 小时 / 每天 + 自定义分钟数）与探测项目勾选；任务执行历史（Runs / Batches）收敛为次级视图或折叠面板，标题改为「手动测速任务 / 自动定时批次 · HH:mm:ss」+ 状态 + 耗时 + 取消按钮，彻底隐藏内部 UUID、Actor Scope、`config_revision`、Lease 租约与 SHA256 指纹噪音；
    5. **人类可读的测速详情抽屉（`ProbeEvidenceSheet.vue`）**：顶部展示节点名称 (`display_name`)、协议、服务器地址与当前综合状态，将原始 `profile=baseline version=baseline-v1 verdict=available reason=contract_matched status=204 latency_ms=42` 解析转换为结构化人类可读卡片（检测项目、结论、响应延迟、中文状态说明、检测时间），并支持抽屉内一键「重新测速此节点」。
  - 联动增强 `web/src/features/nodes/{NodesView.vue,nodeView.ts,useNodes.ts}`：兼容解析后端结构化 `capabilities` 与 `latency_ms`/`last_probed_at`/`health_status`/`sources`，在节点列表与详情抽屉直接展示真实探测状态与延迟毫秒数，并提供单节点立即测速联动按钮。
  - 更新并补充 `web/src/features/probes/probes.test.ts` 与 `web/src/features/nodes/nodeView.test.ts` 单测，运行 `npm test -- --run` 与 `npx vue-tsc --noEmit` 全部通过（exit 0）。

## 2. Phase 2 — 全仓汇聚门禁与独立验收（Phase 1 全部完成后执行）

- [x] 2.1 **Task 2.1（全仓汇聚自测与硬门禁）**：执行后端 `go test -count=1 ./...`、`go build ./...` 与前端 `npm test -- --run`、`npx vue-tsc --noEmit` 全部 exit 0；在隔离临时目录验证前端生产构建并确保受保护现场干净；执行 `openspec validate csp-probe-engine-human-centric-redesign --strict` exit 0。
- [x] 2.2 **Task 2.2（独立 `reviewer` 终态审查）**：独立 `reviewer` 对照 `csp-probe-engine-human-centric-redesign` 规格与最终 diff 完成只读代码与安全审计并出具 `PASS` 裁决。
- [x] 2.3 **Task 2.3（隔离预览实例准备与独立 `critic` 多视口真浏览器体验与视觉验收）**：在隔离环境（临时 SQLite 数据库、种子订阅与真实探测观测数据、隔离构建产物、非生产端口）启动预览实例并通过健康检查，移交独立 `critic` 在桌面与移动多视口下对重构后的「探针引擎工作台」与「节点台账联动」执行真浏览器交互、一键全量/单节点测速、筛选排序、测速详情抽屉及视觉可用性验收并取得 `PASSED`。

## 1. Phase 1 — 可并发扇出前后端特性包（写集合互不重叠）

- [x] 1.1 **Task 1.1（后端：实现统一节点池双端优先级队列、定时去重入池、手动插队最前、`probing` 节点状态注入与 `GET /api/v1/probes/pool` 5 大统计指标接口）**（写集合：`internal/**`, `cmd/csp/**`）：
  - 在 `internal/probe/queue/scheduler.go` 与 `internal/application/probe/{runner,periodic,service}.go` 实现统一**节点池双端优先级队列（Node Probe Pool / Priority Deque）**：
    1. 扩展 `queue.Task` 支持 `EnqueueMode`（保留默认 `EnqueueStrictConflict = 0` 兼容既有底层单测；新增 `EnqueuePeriodicDedupe` 定时去重队尾入池与 `EnqueueManualPreemptFront` 手动插队队首优先调度）；
    2. **定时测速去重入池**：当 `PeriodicCoordinator`（或 `run.ActorScope == "system:periodic-probe"`）提交节点入池时，若节点已在节点池中（处于 `probing` 正在检测或 `queued` 排队等待），直接静默去重跳过（计入 `SkippedNodes` 且不报 `ErrNodeConflict`），仅将不在池中的节点加入队列尾部；
    3. **手动检测插在队列最前面**：当手动发起检测（单节点、多选节点或全量测速）时，将目标节点插在节点池队列最前面优先调度执行；若目标节点已在节点池队列后方等待，将其提升（Promote）至队列最前面并合并探测维度；忽略 `NodeTTL` 冷却期，永不因 `ErrNodeConflict` 失败；
    4. 实现节点池实时状态自省方法 `GetNodePoolState(logicalID string) string`（返回 `"probing"` | `"queued"` | `"idle"`）与 `PoolQueueSnapshot()`（返回 `ProbingNodeIDs`、`QueuedNodeIDs`、`QueueNodesCount`、`ProbingCount`、`QueuedWaitingCount`）。
  - 在 `internal/application/inventory/service.go`（及 `cmd/csp/main.go`）中打通节点“检测中（`probing`）”实时状态：
    1. 新增 `inventory.WithNodePoolStateProvider` 并注入 `probeScheduler`；
    2. 在 `NodeView` 与 `NodeDetail` 新增 `probe_state`（`"probing"` | `"queued"` | `"idle"`），当节点处于检测中时将 `probe_state` 设为 `"probing"`、`health_status` 设为 `"probing"`（同时保留历史 `latency_ms` / `capabilities` 供前端参考）。
  - 在 `internal/domain/probe.go`、`internal/application/probe/service.go` 与 `internal/transport/http/{probes,router}.go` 新增 `GET /api/v1/probes/pool` 与 `POST /api/v1/probes/schedule/trigger`：
    1. `GET /api/v1/probes/pool` 实时返回 5 大核心统计指标：`queue_nodes_count`（当前队列中的节点数）、`untested_count`（未测数）、`total_count`（总数）、`unavailable_count`（不可用数）、`available_count`（可用数），以及 `probing_count`、`queued_waiting_count`、`healthy_count`、`degraded_count`、`probing_node_ids`、`queued_node_ids`；
    2. `POST /api/v1/probes/schedule/trigger` 支持立即触发一次定时去重入池并返回最新池状态。
  - 编写并更新 `internal/probe/queue/queue_test.go`、`internal/application/probe/{runner_test,periodic_test,service_test}.go`、`internal/application/inventory/read_model_test.go`、`internal/transport/http/probes_test.go` 单元测试，运行 `go test ./internal/probe/... ./internal/application/probe/... ./internal/application/inventory/... ./internal/transport/http/...` 与 `go build ./...` 全部 exit 0。

- [x] 1.2 **Task 1.2（前端：重设计节点池检测状态看板展示 5 大核心指标、节点“检测中”动态状态徽章与筛选、实时池状态轮询联动）**（写集合：`web/src/**`）：
  - 在 `web/src/features/probes/{probeTypes.ts,useProbes.ts,ProbesView.vue,ProbeEvidenceSheet.vue}` 与 `web/src/features/nodes/{nodeView.ts,useNodes.ts,NodesView.vue}`、`web/src/locales/{zh-CN.ts,en-US.ts}` 实现节点池看板与“检测中”全链路交互：
    1. 在 `probeTypes.ts` 与 `useProbes.ts` 新增 `ProbePoolStatus` 类型、`poolStatus` 响应式状态、`loadPoolStatus()`（对接 `GET /api/v1/probes/pool`，并在后端未返回时优雅回退本地节点实时聚合）与 `triggerScheduleNow()`（对接 `POST /api/v1/probes/schedule/trigger`）；
    2. 在 `ProbesView.vue` 顶部第一视觉层级重设计 **「节点池与全网检测状态看板」**（`data-testid="probe-kpi-bar"`），醒目、美观地展示用户要求的 5 大核心指标卡片：
       - **当前队列中的节点数**（`data-testid="pool-metric-queue"`）：主数字展示 `queue_nodes_count`，副标签展示「⚡ 检测中 `probing_count` · ⏳ 排队 `queued_waiting_count`」及实时队列消化进度条；
       - **总数**（`data-testid="pool-metric-total"`）：展示活跃节点总数 `total_count` 与平均延迟摘要；
       - **可用数**（`data-testid="pool-metric-available"`）：展示 `available_count`、可用率百分比进度条及「正常 / 降级」细分；
       - **不可用数**（`data-testid="pool-metric-unavailable"`）：展示 `unavailable_count` 及一键「插队重测不可用」快捷操作；
       - **未测数**（`data-testid="pool-metric-untested"`）：展示 `untested_count` 及一键「插队检测未测」快捷操作；
       - 配套 **定时巡检入池控制卡**（展示去重规则提示、一键「立即入池」、启停与策略设置），且点击 5 大指标卡可直接联动下方表格按对应状态筛选；
    3. 在 `nodeView.ts`、`ProbesView.vue` 与 `NodesView.vue` 新增 **「检测中」**（`probing`）与 **「队列中」**（`queued`）节点状态：
       - `nodeHealthBadge` 支持识别 `probe_state === 'probing'` / `health_status === 'probing'` / `probingNodeIds` 并返回 `{ label: '检测中', tone: 'info' }`（以及 `queued` 返回 `{ label: '队列中', tone: 'warning' }`）；
       - `ProbesView.vue` 状态筛选器 `healthFilter` 新增 `probing`（「检测中 / 队列中」）筛选项；
       - 在 `ProbesView.vue` 节点表格、`ProbeEvidenceSheet.vue` 与 `NodesView.vue` 节点卡片/详情抽屉中，对处于检测中的节点展示带旋转/脉冲动效的「检测中」徽章，手动点击测速时即时展示“已插队至节点池最前面优先检测”反馈并联动高频轮询刷新。
  - 补充并更新 `web/src/features/probes/probes.test.ts` 与 `web/src/features/nodes/nodeView.test.ts` 单测，运行 `npm test -- --run` 与 `npx vue-tsc --noEmit` 全部 exit 0。

## 2. Phase 2 — 全仓汇聚门禁与独立验收（Phase 1 全部完成后执行）

- [x] 2.1 **Task 2.1（全仓汇聚自测与硬门禁）**：执行后端 `go test -count=1 ./...`、`go build ./...` 与前端 `npm test -- --run`、`npx vue-tsc --noEmit` 全部 exit 0；验证 `openspec validate csp-node-probe-pool-priority-queue --strict` exit 0。
- [x] 2.2 **Task 2.2（独立 `reviewer` 终态审查）**：独立 `reviewer` 对照 `csp-node-probe-pool-priority-queue` 规格与最终 diff 完成只读代码与安全审计并出具 `PASS` 裁决。
- [x] 2.3 **Task 2.3（隔离预览实例准备与独立 `critic` 多视口真浏览器验收）**：在隔离环境（临时 SQLite 数据库、种子节点与探测队列状态、隔离构建产物、非生产端口）启动预览实例并通过健康检查，移交独立 `critic` 在桌面与移动多视口下对「节点池 5 大指标看板」、「节点检测中动态状态与筛选」、「手动检测插队最前与定时测速去重入池」执行真浏览器交互与视觉验收并取得 `PASSED`。

## Why

根据用户的明确需求指令：
> “还记得我们之前说的节点池的概念，现在好像没有这个概念，重新实现当定时测速的时候，添加节点池，如果节点池已经有了，就不重复添加，手动检测插在队列最前面，然后检测状态显示，当前队列中的节点数，未测数，总数，不可用数，可用数，设计一个好官，合适恰当前端，节点状态添加检测中”

经过对当前前后端探针引擎与节点台账的实机取证，定位到以下核心缺失与体验痛点：
1. **缺乏统一的“节点池（Node Probe Pool）”概念与去重/插队调度机制**：
   - 当前 `internal/probe/queue/scheduler.go` 仅按 `(RunID, Task)` 做简单轮询调度，且当同一节点与探测维度在短时间或并发批次中再次提交时直接报错 `ErrNodeConflict`（导致定时测速与手动测速重叠时任务直接失败）；
   - **定时测速未做节点池去重**：定时测速触发时未能检查节点是否已在节点池中，缺少“若节点已在池中（排队等待或正在检测）则静默跳过、不重复入池”的机制；
   - **手动检测无法插队到最前面**：用户手动点击单节点重测、已选节点测速或全量测速时，无法插在节点池队列最前面优先执行，也无法将已在池中排队的节点提升（Promote）到队首。
2. **节点状态缺失“检测中（`probing`）”实时态与节点池 5 大统计指标 API**：
   - 后端 `NodeView`（`GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}`）仅根据历史持久化的 `probe_observations` 计算静态 `health_status`（`healthy` / `degraded` / `unhealthy` / `unknown`），完全不知道某个节点此刻是否正在节点池中处于 **`probing`（检测中）** 或 **`queued`（队列等待中）**；
   - 缺少统一的节点池状态接口 `GET /api/v1/probes/pool` 来实时汇总并返回用户明确要求的 5 大核心指标：**当前队列中的节点数**、**未测数**、**总数**、**不可用数**、**可用数**。
3. **前端顶部看板与节点状态展示未体现节点池与“检测中”直观反馈**：
   - `ProbesView.vue` 顶部 KPI 栏未能醒目、直观地呈现节点池 5 大核心检测状态指标（当前队列中的节点数、未测数、总数、不可用数、可用数）；
   - 状态筛选器、状态图例、`ProbesView.vue` 节点表格与 `NodesView.vue` 节点卡片/详情抽屉缺少动态的 **「检测中」**（`probing`）与 **「队列中」**（`queued`）状态徽章及插队反馈。

## What Changes

- **后端实现统一节点池双端优先级队列（Node Probe Pool / Priority Deque）与去重/插队机制（`internal/probe/queue/**`、`internal/application/probe/**`）**：
  - 在调度器与探针执行层构建线程安全的**节点池（Node Probe Pool）**状态机，跟踪每个入池节点的实时状态（`queued` 排队等待 / `probing` 检测中）、待执行探测维度集合与关联的等待方；
  - **定时测速去重入池（Tail Enqueue with Deduplication）**：当定时测速（`PeriodicCoordinator` / `ActorScope == "system:periodic-probe"`）添加节点到节点池时，按顺序推入队列尾部；**若目标节点已经在节点池中（无论处于 `queued` 还是 `probing`），直接静默去重跳过，绝不重复添加，亦不触发 `ErrNodeConflict` 报错**；
  - **手动检测插在队列最前面（Front-of-Queue Priority Preemption & Promotion）**：当手动发起检测（单节点重测、批量已选节点测速、手动一键全量测速）时，以高优先级推入节点池**队列最前面（队首）**优先调度；若某目标节点已在节点池等待队列中，立即将其**提升（Promote）至队列最前面**并合并待测维度 `kinds`；若该节点此刻正处于 `probing`（检测中），则复用/合并当前检测或紧接队首补测新增维度，绝不报错中断。
- **后端新增节点“检测中（`probing`）”实时状态注入与 `GET /api/v1/probes/pool` 5 大指标接口（`internal/application/inventory/**`、`internal/application/probe/**`、`internal/transport/http/**`、`cmd/csp/main.go`）**：
  - 在 `inventory.NodeView`（及 `NodeDetail`）中新增 `probe_state` 字段（`"probing"` | `"queued"` | `"idle"`），并在节点正处于节点池检测中时将实时状态反映到 `NodeView`（当节点处于检测中时 `probe_state = "probing"`，`health_status` 支持返回 `"probing"` 或保留历史观测同时由 `probe_state` 驱动前端优先展示「检测中」）；
  - 新增 `GET /api/v1/probes/pool`（并在 `POST /api/v1/probes/runs` 与新增的 `POST /api/v1/probes/schedule/trigger` 响应中同步返回最新池快照），实时返回节点池与全网活跃节点 5 大核心统计指标：
    1. `queue_nodes_count`：**当前队列中的节点数**（节点池内节点总数 = `probing_count` 正在检测数 + `queued_waiting_count` 排队等待数）；
    2. `untested_count`：**未测数**（无任何探测观测记录的活跃节点数）；
    3. `total_count`：**总数**（全网活跃节点总数）；
    4. `unavailable_count`：**不可用数**（最新连通结论为异常/不可达 `unhealthy` 的活跃节点数）；
    5. `available_count`：**可用数**（最新连通结论为正常 `healthy` 或降级可用 `degraded` 的活跃节点数，同时返回 `healthy_count` 与 `degraded_count` 明细）；
    6. `probing_node_ids`（当前正在检测中的节点 LogicalID 列表）与 `queued_node_ids`（按当前节点池队首到队尾顺序排列的等待节点 LogicalID 列表）。
- **前端重设计美观恰当的“节点池与全网检测状态看板”及“检测中”状态体系（`web/src/features/probes/**`、`web/src/features/nodes/**`、`web/src/locales/**`）**：
  - 在 `ProbesView.vue` 第一视觉层级打造直观、高辨识度的**节点池实时检测状态看板（Node Pool Status Dashboard）**，醒目展示用户指定的 5 项核心指标：**当前队列中的节点数**（含检测中/排队等待细分与队列消化进度条）、**未测数**、**总数**、**不可用数**、**可用数**（含可用率与正常/降级细分），并配套定时自动入池策略卡（含「立即入池巡检」按钮与一键启停）；
  - 点击看板上的「当前队列中 / 检测中 / 可用 / 不可用 / 未测 / 总数」指标卡片可直接联动下方工作台表格快速筛选对应状态的节点；
  - 状态筛选器下拉框、状态徽章组件（`nodeView.ts` / `ProbesView.vue` / `NodesView.vue` / `ProbeEvidenceSheet.vue`）全面新增 **「检测中」**（`probing`，带脉冲/旋转动效）与 **「队列中」**（`queued`）状态展示；
  - 手动触发单节点重测或批量测速时，节点立即切换为「检测中 / 队列最前」状态并提示“已插队至节点池最前面优先检测”，轮询实时刷新节点池计数与节点最新延迟。

## Capabilities

### New Capabilities

- `node-probe-pool`: 统一节点池双端优先级队列（定时测速去重入池、手动检测插队最前）、节点“检测中（`probing`）”实时状态透传、`GET /api/v1/probes/pool` 5 大核心统计指标契约，以及前端节点池状态看板与“检测中”全链路视觉交互规格。

### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `node-probe-pool` 能力规格建立完整契约）。

## Impact

- 后端：`internal/probe/queue/**`、`internal/application/probe/**`、`internal/application/inventory/**`、`internal/domain/**`、`internal/transport/http/{probes,nodes,router}*.go`、`cmd/csp/main.go`
- 前端：`web/src/features/probes/**`、`web/src/features/nodes/**`、`web/src/locales/**`
- 验证边界：仅在本地单测与隔离预览环境（临时 SQLite 库、独立非生产端口与临时构建目录）执行自测、代码审查与真浏览器多视口验收，严禁触碰生产环境。

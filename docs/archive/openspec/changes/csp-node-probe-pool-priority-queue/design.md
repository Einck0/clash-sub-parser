## Context

1. **当前探针调度器缺乏“节点池（Node Probe Pool）”概念与去重/插队机制（`internal/probe/queue/scheduler.go`、`internal/application/probe/{runner,periodic,service}.go`）**：
   - `internal/probe/queue/scheduler.go` 目前仅维护按 `RunID` 分桶的 `queues map[string][]Task` 与 `activeNodes map[string]activeRecord`（键为 `logicalID#kind`）。当同一节点与同一探测维度在活跃期或 `NodeTTL`（30 秒）内再次提交时，`s.submit` 直接返回 `ErrNodeConflict`。
   - `internal/application/probe/periodic.go` 在 `executeBatch` 中查询全部活跃节点后，未检查节点是否已经在节点池中排队或正在检测，直接全部提交给 `DefaultRunner.Run`；若此时恰好有手动测速或上一批任务尚未结束，就会触发 `ErrNodeConflict` 导致批次/任务报错。
   - 用户手动触发的单节点重测、已选节点测速或一键全量测速（`POST /api/v1/probes/runs`）与后台定时任务平权追加在 `runOrder` 队尾，既不能插到队列最前面优先执行，也无法将已在池中排队的节点提升（Promote）至队首。

2. **节点状态缺失“检测中（`probing`）”实时态与节点池 5 大统计指标接口（`internal/application/inventory/service.go`、`internal/transport/http/{nodes,probes}.go`）**：
   - `inventory.NodeView`（`internal/application/inventory/service.go:96`）目前仅有静态 `health_status`（`healthy` / `degraded` / `unhealthy` / `unknown`），未关联节点池实时状态；当节点正在池中执行检测（`probing`）或排队等待（`queued`）时，`GET /api/v1/nodes` 与 `GET /api/v1/nodes/{logical_id}` 无法向前端透传 `probe_state` 与 `"probing"`（检测中）状态。
   - 后端缺少统一的 `GET /api/v1/probes/pool` 接口来实时聚合用户要求的 5 项核心指标：**当前队列中的节点数**（`queue_nodes_count`）、**未测数**（`untested_count`）、**总数**（`total_count`）、**不可用数**（`unavailable_count`）、**可用数**（`available_count`）。

3. **前端探针引擎看板与节点状态展示需围绕“节点池 + 5 大核心指标 + 检测中状态”升级（`web/src/features/probes/**`、`web/src/features/nodes/**`）**：
   - `web/src/features/probes/ProbesView.vue` 第一视觉层级需升级为直观、大气的**节点池与全网检测状态看板**，清晰展示「当前队列中的节点数」、「未测数」、「总数」、「不可用数」、「可用数」5 大指标及定时自动入池控制；
   - `web/src/features/nodes/nodeView.ts`、`ProbesView.vue` 与 `NodesView.vue` 需支持 `probing`（检测中）与 `queued`（队列中）状态徽章、状态筛选及手动插队优先检测反馈。

## Goals / Non-Goals

**Goals:**
- 在 `internal/probe/queue` 与 `internal/application/probe` 实现统一的**节点池双端优先级队列（Node Probe Pool / Priority Deque）**：
  1. **定时测速去重入池（Tail Enqueue with Deduplication）**：定时测速触发时将活跃节点加入节点池队尾；若节点已在节点池中（处于 `queued` 等待或 `probing` 检测中），**静默跳过、不重复添加**，彻底消除 `ErrNodeConflict` 冲突报错；
  2. **手动检测插在队列最前面（Front-of-Queue Priority Preemption & Promotion）**：手动发起检测时，目标节点直接插在节点池队列最前面优先调度；若目标节点已在节点池队列后方等待，将其**提升（Promote）至队列最前面**并合并探测维度；若目标节点正在检测中（`probing`），复用/合并待测维度且不报冲突错误；
  3. 保持底层 `Scheduler.Submit` 默认模式（`EnqueueStrictConflict = 0`）对既有单元测试（如 `TestSchedulerBoundsConcurrencyAndRejectsNodeConflict`）的完全向后兼容。
- 在 `NodeView` 与前端节点状态体系中新增 **「检测中」（`probing`）** 状态（及 `probe_state: "probing" | "queued" | "idle"`），在 `GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}` 与 `GET /api/v1/probes/pool` 实时透传。
- 新增 `GET /api/v1/probes/pool`（以及手动触发定时入池 `POST /api/v1/probes/schedule/trigger`），精准返回用户要求的 5 大统计指标：**当前队列中的节点数**、**未测数**、**总数**、**不可用数**、**可用数**。
- 在 `ProbesView.vue` 与 `NodesView.vue` 完成美观恰当的节点池状态看板、“检测中”动态徽章、点击指标卡联动筛选及实时池状态轮询。

**Non-Goals:**
- 不引入 Redis 或外部消息队列中间件，继续使用单进程内存双端优先级队列 + SQLite 持久化观测记录；
- 不改变 sing-box 代理拨号安全边界与 SSRF 防护逻辑；
- 不触碰生产容器或生产数据库。

## Decisions

### 1. 统一节点池双端优先级队列（`internal/probe/queue/scheduler.go` & `internal/application/probe`）

1. **任务入池模式（`EnqueueMode`）与双端队列调度**：
   - 在 `queue.Task` 新增字段 `Mode EnqueueMode`：
     - `EnqueueStrictConflict EnqueueMode = 0`（默认零值）：保持原有严格冲突检查与 `NodeTTL` 行为，确保既有底层单元测试 100% 兼容；
     - `EnqueuePeriodicDedupe EnqueueMode = 1`（定时测速去重入池）：
       - 检查该节点 `LogicalID`（或 `(LogicalID, Kind)`）是否已处于节点池中（`probing` 正在执行或 `queued` 正在排队）；
       - **若已在节点池中**：不报错、不重复入队，立即触发 `task.OnComplete(nil)` 并返回 `nil`（同时在 `Scheduler` 记录去重跳过计数）；
       - **若不在节点池中**：将任务追加至普通/定时队列尾部（Tail），等待工作协程按序消化。
     - `EnqueueManualPreemptFront EnqueueMode = 2`（手动检测插在队列最前面）：
       - 忽略 `NodeTTL` 冷却期限制，永不返回 `ErrNodeConflict`；
       - **若该节点已有定时/低优排队任务在节点池中等待**：将原排队任务出队并完成其回调（或合并关联回调），将本次手动检测任务**插在节点池队列最前面（Head / `runOrder[0]` 且 `queues[runID]` 队首）**；
       - **若该节点此刻正处于 `probing`（检测中）**：允许手动任务排在队首紧随当前正在执行的动作（或复用结果），绝不因 `ErrNodeConflict` 失败；
       - 在 `nextEligibleLocked()` 调度时，拥有 `EnqueueManualPreemptFront` 任务的手动 Run 始终排在 `runOrder` 最前面，并优先获得空闲 Worker 槽位。

2. **节点池实时状态追踪（Node Pool State Introspection）**：
   - `queue.Scheduler` 维护节点级池状态快照方法 `PoolQueueSnapshot()` 与 `GetNodePoolState(logicalID string) string`：
     - 当某个 `LogicalID` 有至少一个任务正在 `processTask` 执行中时，其池状态为 `"probing"`（检测中）；
     - 当某个 `LogicalID` 没有正在执行的任务、但在 `queues` 中有等待调度的任务时，其池状态为 `"queued"`（队列等待中），且按队首到队尾顺序记录在 `QueuedNodeIDs` 中；
     - 其他情况为 `"idle"`；
     - **当前队列中的节点数（`queue_nodes_count`）**：定义为当前节点池内的去重节点总数（`len(ProbingNodeIDs) + len(QueuedNodeIDs)`，同时在响应中提供 `probing_count` 正在检测节点数与 `queued_waiting_count` 排队等待节点数，满足对“队列中/池中节点数”的直观理解与细分查看）。
   - 在 `PeriodicCoordinator.executeBatch` 中，定时批次拉取活跃节点后，亦可先通过 `scheduler.FilterNotInPool(nodes)` 或通过 `DefaultRunner` 的 `EnqueuePeriodicDedupe` 自动跳过已在池中的节点，并将跳过数量计入 `batch.Counts.SkippedNodes`。

### 2. 节点“检测中（`probing`）”状态注入与 `GET /api/v1/probes/pool` 契约

1. **`inventory.NodeView` 扩展（`internal/application/inventory/service.go`）**：
   - 新增 `NodePoolStateProvider` 接口（由 `*queue.Scheduler` 或 `*probe.Service` 实现）：
     ```go
     type NodePoolStateProvider interface {
         GetNodePoolState(logicalID string) string // returns "probing", "queued", or "idle"
     }
     ```
   - 通过 `inventory.WithNodePoolStateProvider(provider)` 注入 `inventory.Service`（在 `cmd/csp/main.go` 中将 `probeScheduler` 注入 `invService`）；
   - 在 `NodeView` 与 `NodeDetail` 新增字段：
     - `ProbeState string`（JSON: `"probe_state"`，取值 `"probing"` | `"queued"` | `"idle"`）；
   - 在 `enrichNodeViews` 中：
     - 默认设置 `views[i].ProbeState = "idle"`；
     - 若 `s.poolProvider != nil`，查询 `state := s.poolProvider.GetNodePoolState(views[i].LogicalID)`，设置 `views[i].ProbeState = state`；
     - 当 `state == "probing"` 时，将 `views[i].HealthStatus` 设为 `"probing"`（同时保留 `LatencyMS`、`LastProbedAt`、`Capabilities` 历史最新观测值，方便前端在展示「检测中」动态徽章的同时展示上次延迟参考）。

2. **节点池与全网检测状态统计 API：`GET /api/v1/probes/pool`（`internal/application/probe/service.go` & `internal/transport/http/probes.go`）**：
   - 在 `probe.Service` 注入 `nodes domain.NodeRepository`、`observations domain.ProbeObservationRepository` 与 `scheduler *queue.Scheduler`；
   - 新增 `probe.Service.GetPoolStatus(ctx context.Context) (*domain.ProbePoolStatus, error)`，计算并返回：
     ```json
     {
       "queue_nodes_count": 12,
       "probing_count": 4,
       "queued_waiting_count": 8,
       "untested_count": 15,
       "total_count": 120,
       "unavailable_count": 18,
       "available_count": 87,
       "healthy_count": 75,
       "degraded_count": 12,
       "probing_node_ids": ["node_01", "node_02", "node_03", "node_04"],
       "queued_node_ids": ["node_05", "node_06", "..."],
       "updated_at": "2026-09-27T03:00:00Z"
     }
     ```
   - **5 大核心指标口径定义（全网活跃节点维度 + 实时节点池维度）**：
     - `queue_nodes_count`：**当前队列中的节点数**（当前在节点池中的去重节点总数 = `probing_count` + `queued_waiting_count`）；
     - `total_count`：**总数**（当前 `active = true` 的活跃节点总数）；
     - `untested_count`：**未测数**（活跃节点中尚无任何 `probe_observations` 记录的节点数）；
     - `unavailable_count`：**不可用数**（活跃节点中已探测且综合健康结论为 `unhealthy` 的节点数）；
     - `available_count`：**可用数**（活跃节点中已探测且综合健康结论为 `healthy` 或 `degraded` 的节点数，满足 `available_count + unavailable_count + untested_count == total_count` 恒等式！）。
   - 新增 `POST /api/v1/probes/schedule/trigger`：支持管理员或前端一键触发一次定时入池巡检（将当前不在池中的活跃节点按定时去重规则加入节点池队尾），并返回最新 `ProbePoolStatus`。
   - 在 `POST /api/v1/probes/runs`（手动检测）中：当请求返回前，先把目标节点（或全量活跃节点）预注册/插队到节点池最前面，使紧随其后的 `GET /api/v1/probes/pool` 与 `GET /api/v1/nodes` 立即反映 `queue_nodes_count` 与 `probe_state = "probing" / "queued"`。

### 3. 前端节点池状态看板与“检测中”全链路设计（`web/src/features/probes/**` & `web/src/features/nodes/**`）

1. **第一视觉层级：节点池与全网检测状态看板（`ProbesView.vue`）**：
   - 设计高辨识度的 **5+1 节点池全景状态看板**（支持桌面端网格与移动端自适应流式布局，点击前 5 张指标卡可直接切换工作台状态筛选）：
     1. **当前队列中的节点数**（`data-testid="pool-metric-queue"`）：醒目展示 `queue_nodes_count`，副标题拆分展示「⚡ 检测中 `probing_count` · ⏳ 排队等待 `queued_waiting_count`」，当队列非空时展示动态脉冲指示器与实时消化进度条；
     2. **总数**（`data-testid="pool-metric-total"`）：展示全网活跃节点总数 `total_count` 与已覆盖订阅源数；
     3. **可用数**（`data-testid="pool-metric-available"`）：绿色语义展示 `available_count`、在线可用率百分比进度条，以及「正常 `healthy_count` / 降级 `degraded_count`」细分徽章；
     4. **不可用数**（`data-testid="pool-metric-unavailable"`）：红色语义展示 `unavailable_count`（连接超时/握手失败）与占比，提供一键「重测不可用节点（插队最前）」快捷入口；
     5. **未测数**（`data-testid="pool-metric-untested"`）：中性/琥珀色语义展示 `untested_count`，提供一键「检测未测节点（插队最前）」快捷入口；
     6. **节点池定时巡检与去重策略卡**（`data-testid="pool-schedule-card"`）：展示定时自动入池开关、周期、去重规则说明（「已在池中节点不重复添加 · 手动检测自动插队最前」）、一键「立即入池巡检」与「策略设置」按钮。

2. **节点状态新增「检测中」（`probing`）与「队列中」（`queued`）**：
   - 在 `web/src/features/nodes/nodeView.ts` 中：
     - `NodeHealthStatus` 扩展支持 `'probing' | 'queued' | 'healthy' | 'degraded' | 'unhealthy' | 'unknown' | 'missing'`；
     - `NodeRecord` 与 `NormalizedNode` 新增 `probe_state?: 'probing' | 'queued' | 'idle'` / `probeState: 'probing' | 'queued' | 'idle'`；
     - `nodeHealthBadge(node, probingIds?, queuedIds?)`：若 `node.probeState === 'probing'` 或 `node.healthStatus === 'probing'` 或在实时 `probingNodeIds` 集合中，返回 `{ label: '检测中', tone: 'info', pulsing: true }`；若 `node.probeState === 'queued'` 或在实时 `queuedNodeIds` 集合中，返回 `{ label: '队列中', tone: 'warning', pulsing: true }`；否则返回原有的 `正常` / `降级` / `异常` / `未探测`；
   - 在 `ProbesView.vue` 状态筛选器 `healthFilter` 中新增选项：
     - `all`（全部状态）
     - `probing`（检测中 / 队列中）
     - `healthy`（可用 · 正常）
     - `degraded`（可用 · 降级）
     - `unhealthy`（不可用 · 异常）
     - `unprobed`（未测）
   - 在 `ProbesView.vue` 节点表格行与 `NodesView.vue` 节点卡片/详情抽屉中：
     - 当节点处于 `检测中` 时，渲染带旋转/呼吸动效的蓝色/主色「⚡ 检测中」徽章；处于 `队列中` 时渲染「⏳ 队列中」徽章；
     - 手动点击单节点「立即重测」或顶部「一键全量测速 / 测速已选节点」时，立即乐观标记目标节点为 `probing` / 队首并展示清晰提示：「已插入节点池队列最前面优先检测」，同时启动高频轮询（1.5s~2s）拉取 `/api/v1/probes/pool` 与 `/api/v1/nodes` 直至节点池队列消化完毕。

## Risks / Trade-offs

- **[Risk] 快速探测完成导致前端轮询错过瞬时 `probing` 状态** → **Mitigation**：
  1. 在手动发起测速（`createRun` / `triggerQuickProbe`）时，后端在 `POST /api/v1/probes/runs` 同步阶段即将目标节点登记入池（置为 `probing`/`queued`），前端也在请求发起瞬间将目标节点加入本地 `probingNodeIds` 集合并在 `GET /api/v1/probes/pool` 返回后同步服务端 `probing_node_ids` / `queued_node_ids`；
  2. 单元测试与隔离预览测试可同时覆盖“探测进行中（`probing` / `queued`）”与“探测完成后转为 `可用 / 不可用`”完整状态流转。
- **[Risk] 既有调度器单元测试对 `Submit` 返回 `ErrNodeConflict` 有断言** → **Mitigation**：
  1. `Task.Mode` 默认零值为 `EnqueueStrictConflict`，保留 `Scheduler.Submit` 原有冲突校验语义，确保 `internal/probe/queue/*_test.go` 既有测试零破坏；
  2. 新增 `EnqueuePeriodicDedupe` 与 `EnqueueManualPreemptFront` 模式以及 `Scheduler.EnqueueNodePool` / `PoolQueueSnapshot` 专用于节点池定时去重与手动插队，并编写专门的节点池并发单测。

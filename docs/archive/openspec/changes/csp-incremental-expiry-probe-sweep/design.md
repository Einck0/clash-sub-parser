## Context

当前系统的定时探测逻辑将用户配置的 `interval_seconds` 直接作为轮询触发定时器，并在启用或变更配置时将 `next_due_at` 设定为未来整段周期之后（如 2 小时后），导致管理员配置后迟迟看不到任何探测动作。并且当周期到达时，系统无差别将所有激活节点的全部类别（如 960 节点 * 5 kind）一次性全部入池，造成瞬时网络与并发拥塞。

现有系统已具备以下关键设施：
1. `probeObservationRepository.ListLatestByNodes(ctx, nodeIDs, kinds)`：高效分块查询节点在各类别下的最新观测；
2. `probe.queue.Scheduler` 的 `EnqueuePeriodicDedupe`：队列去重，如果已有相同 node 和 kind 在等待或执行中，则不重复提交；
3. `PeriodicCoordinator` 的 CAS 分布式租约与故障恢复机制；
4. `Runner` 与 `probePool`：支持节点池在池状态检测与安全出站探测。

## Goals / Non-Goals

**Goals:**
- 将轻量扫描周期（默认 10 分钟）与用户配置的节点观测新鲜度周期（`interval_seconds`）彻底解耦；
- 计划启用或配置变更时即刻唤醒巡检，不空等整段周期；
- 基于 `ListLatestByNodes` 增量比对过期：仅当 (node, kind) 无历史观测或观测时间已过有效期（距今 >= `interval_seconds`）时入池；
- 对失败观测施加合理冷却期（默认 5 分钟），避免失败节点每轮重复震荡；
- 实施单轮任务派发上限配额（默认 512 任务），并按观测时间升序（从未观测优先、最老观测次之）排序，防止单批突发冲击并保证防饥饿公平调度；
- 启动与恢复自愈：自动将历史遗留的远期 `next_due_at` 校正为当前即刻巡检；
- 前端控制台文案明示巡检时钟与观测有效期，修复空闲时不轮询的缺陷（引入 15 秒低频保底刷新），并在立即触发后维持追踪。

**Non-Goals:**
- 不修改底层 SQLite 数据库模式（不增加新表或破坏现有迁移历史）；
- 不变更 `domain.ProbeSchedule` API 的基础传输结构（保留 `interval_seconds`, `next_due_at` 字段语义，向后兼容）；
- 不引入外部复杂调度框架（如 cron 或独立调度器服务），保持单进程纯 Go 协程自愈架构。

## Decisions

### 1. 时钟模型解耦与 `next_due_at` 语义
- **决策**：巡检时钟固定为每 10 分钟一次（`defaultSweepInterval = 10 * time.Minute`，支持测试 Option 覆盖）。`ProbeSchedule.NextDueAt` 保存下一次轻量巡检的时间戳（即 `now.Add(sweepInterval)`），而非整段 `interval_seconds`。
- **配置变更时处理**：当管理员调用 `UpdateSchedule` 启用或变更配置时，将 `NextDueAt` 设为 `now` 并调用 `coord.Wake()`，使巡检协程立刻启动评估。
- **替代方案考虑**：为每个节点单独维护定时器。被否决，因为 960+ 节点的定时器过多且无法有效合并网络探测连接与出站会话。

### 2. 增量过期与失败冷却算法
- **决策**：
  1. 获取全部激活节点；
  2. 批量调用 `ListLatestByNodes` 获取各节点各类别的最新观测 `obs`；
  3. 对每对 `(node, kind)` 进行判断：
     - 若 `obs` 不存在：判定为到期；
     - 若 `obs.Verdict` 为失败（`Failed` / `Timeout`）：检查冷却时间 `now.Sub(obs.ObservedAt) < failureCooldown`（5 分钟），若在冷却期内则跳过，超过冷却期且超过 `interval_seconds` 则判定为到期；
     - 若 `obs.Verdict` 为成功（`Available` / `Degraded` 等）：若 `now.Sub(obs.ObservedAt) >= interval_seconds` 则判定为到期，否则未过期。
  4. 仅收集到期的 `(node, kind)`。

### 3. 单轮有界配额与公平覆盖策略
- **决策**：
  - 单轮入池最大任务数为 `maxTasksPerRun`（默认 512）。
  - 若到期任务数超过配额，对候选节点进行排序：
    - 未被观测过的节点权重最高（观测时间视为 0）；
    - 拥有最老观测时间的节点次之；
  - 截取配额上限内的任务执行。未被选中的节点将在 10 分钟后的下一轮巡检中自然被选中，保证多轮之内全覆盖且无饥饿。

### 4. 启动与 Recover 自愈与租约解耦
- **决策**：
  - 在 `PeriodicCoordinator.Recover` 中，若已启用的计划其 `NextDueAt` 为空、在过去（已过期），或处于未来大于一个巡检周期（例如老逻辑写下的 2 小时后），统一将其校正为 `now`，让服务在启动后即刻开启首次巡检。
  - 租约生命周期与批次状态正交管理：`UpdateBatch`（无论是 SQLite 还是内存 Mock）仅持久化批次执行状态（`state`, `counts`, `run_ids`, `redacted_error`, `updated_at`），禁止重写或覆盖租约列（`owner`, `lease_until`）。租约归属于 `AcquireLease`、`HeartbeatLease` 与 `ReleaseLease` 专责维护，杜绝多实例执行期间多次 `UpdateBatch` 清空租约导致其他实例重复夺约并并发派发。
  - 执行过程中通过 `leaseLost` 渠道严格熔断，一旦心跳失败或租约丢失立即停止后续批次写回，防止覆盖新主状态。

### 5. 前端空闲保底刷新与触发后跟踪
- **决策**：在 `ProbesView.vue` 中，当页面处于空闲状态（无活跃 run 且队列为 0）时，维持每 15 秒一次的低频保底刷新，定时获取最新 `batches` 与 `runs`；点击“立即触发”时，设置活跃追踪状态并在触发后 10 秒内保持高频轮询（每 2 秒一次），确保即时捕获批次与运行状态迁移。

## Risks / Trade-offs

- **[Risk] 节点数量极大时（如数万节点）单次扫描开销** → 采用分批获取激活节点与批量 `ListLatestByNodes`（分批 100 个节点查询），内存占用极低且耗时在数十毫秒内完成。
- **[Risk] 探测失败节点在冷却后频繁重试** → 失败节点仍需满足同时超过冷却期（5 分钟）与检测周期（`interval_seconds`），不会比正常节点探测更频繁。
- **[Risk] 多实例并发抢占** → 继承已有的 CAS Batch Lease 机制，同一巡检窗口只有一个实例获得执行权。

## Migration Plan

1. 在 `internal/application/probe/periodic.go` 中重构巡检与过期判定逻辑，注入 `ProbeObservationRepository`；
2. 在 `cmd/csp/main.go` 中为 `NewPeriodicCoordinator` 传入 `probeObsRepo`；
3. 更新 `service.go` 的 `UpdateSchedule` 逻辑：启用或修改配置时设定即刻巡检；
4. 更新前端视图文案与空闲刷新逻辑；
5. 运行完整单元测试与端到端模拟测试。

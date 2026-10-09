## Why

用户反馈：在开启定时检测后，由于系统原先将整个探测周期（如 1 小时或 2 小时）作为单一粗粒度定时器，在未到达完整周期前系统完全不执行检测；且一旦时间到达，又将全量节点的所有探测类别同时并发倾泻入池，造成瞬时冲击与长周期等待。用户期望系统具备常态化轻量巡检能力（默认每 10 分钟或合适频次巡检一次数据库中的节点观测记录），仅将已过期或尚无观测记录的 (node, kind) 增量送检，而非全量一次性突发检测。

## What Changes

1. **时钟解耦与轻量巡检**：解耦“固定 10 分钟轻量扫描时钟 (sweep interval)”与“用户配置的 `interval_seconds` 节点观测有效期”。`next_due_at` 统一表达下一次轻量扫描执行时间（当前时间 + 10 分钟），而非遥远的整段周期。
2. **启用与配置即刻巡检**：管理员开启计划或修改配置时，立即唤醒协调器执行巡检，不等待整段 interval。
3. **基于最新观测的增量过期判定**：巡检时比对每个激活节点在各配置 probe kind 下的最新观测完成时间 `ObservedAt`。若无观测记录，或距今已超过 `interval_seconds`，判定为到期；对失败观测引入合理冷却时间，避免失败节点每轮立即重试。
4. **单轮有界配额与公平覆盖**：设置单轮巡检入池任务上限（配额），优先调度从未观测的节点和观测时间最久远的节点，防止初始或突发情况（如 960 节点 * 5 kind）打爆探测队列；多轮巡检平滑收敛，杜绝饥饿。
5. **池内去重与防自旋**：复用现有 `EnqueuePeriodicDedupe` 队列模式与池内检查，已在探测池或执行中的节点/种类不重复入队；单批探测进行中或刚完成时不自旋重复派发。
6. **启动与迁移校正**：启动或 Recover 时，若历史数据中存留远在未来数小时的旧 `next_due_at`，自动校正为当前即刻巡检，平滑升级且不破坏迁移历史。
7. **前端交互与状态透传**：在 Web 探测控制台明示“每 10 分钟巡检过期节点”与“每节点/类别有效期 = 配置周期”，完整显示下次扫描时间；修复空闲时完全停止轮询问题（引入低频保底刷新）；手动立即触发后持续跟踪批次与运行状态，避免 50ms 快照丢失后续更新。

## Capabilities

### New Capabilities
- `incremental-expiry-probe-sweep`: 提供基于固定轻量巡检时钟与逐节点/类别观测有效期的增量过期探测调度、有界配额分批入池、平滑升级校正与前端保底巡检感知。

### Modified Capabilities

## Impact
- 后端调度与应用层：`internal/application/probe/periodic.go`, `internal/application/probe/service.go`, `cmd/csp/main.go`。
- 依赖注入：`PeriodicCoordinator` 注入 `ProbeObservationRepository` 查询最新观测。
- 契约语义：`ProbeSchedule.NextDueAt` 统一表达下次巡检扫描时间；`IntervalSeconds` 表达节点级观测过期阈值。
- 前端视图与状态管理：`web/src/features/probes/ProbesView.vue`, `web/src/features/probes/useProbes.ts`。

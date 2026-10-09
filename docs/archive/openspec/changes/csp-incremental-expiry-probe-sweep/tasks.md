## 1. 周期巡检应用层重构与依赖注入

- [x] 1.1 在 `PeriodicCoordinator` 中注入 `ProbeObservationRepository` 与 `sweepInterval` 配置项，更新构造函数与选项函数，修改 `cmd/csp/main.go` 完成依赖装配并通过编译
- [x] 1.2 重构 `PeriodicCoordinator.triggerWindow` 与 `executeBatch`：实现基于最新观测时间的逐 (node, kind) 增量过期判定、失败观测冷却期与未观测节点优先调度
- [x] 1.3 实现单轮巡检有界任务配额（512 上限）、公平分页覆盖与基于 `EnqueuePeriodicDedupe` 和在池检测的防重入机制，避免自旋派发
- [x] 1.4 修复 `PeriodicCoordinator.Recover`：对 `NextDueAt` 为空或已在过去的启用计划，校正为 `now` 确保启动即刻巡检，消除非预期的 10 分钟推迟；更新 `executeBatch` 严密处理 `leaseLost` 熔断与防止并发覆盖
- [x] 1.5 修复 SQLite Repository 与内存测试 Repo 的 `UpdateBatch`：移除对 `owner` 和 `lease_until` 租约列的盲写覆盖，使租约与批次状态正交管理，保障多实例执行期间多次 `UpdateBatch` 不破坏已有租约

## 2. 前端巡检语义明示与低频保底刷新

- [x] 2.1 修改 `web/src/features/probes/ProbesView.vue` 与 `probeTypes.ts`：文案明示“每 10 分钟巡检过期节点”及“每节点/类别有效期 = 配置周期”，展示完整的下次巡检时间
- [x] 2.2 在 `ProbesView.vue` 中修复空闲时不轮询问题，引入 15 秒低频保底轮询，并在 `useProbes.ts` 中增强立即触发后的批次与运行状态追踪
- [x] 2.3 补充前端单元测试，验证空闲低频轮询与立即触发跟踪行为，执行 `npm --prefix web test -- --run` 与 `npx vue-tsc --noEmit` 通过

## 3. 后端确定性假时钟与仓库回归测试

- [x] 3.1 编写 `internal/application/probe/periodic_test.go` 针对性测试：覆盖启用即扫、周期缩短、有效/过期/未观测节点多类别增量判定
- [x] 3.2 完善假时钟启动恢复单测，验证过去过期与空 `NextDueAt` 启动即刻校正为 `now`；新增多实例租约在多次 `UpdateBatch`、续约与租约丢失下的回归测试
- [x] 3.3 运行全量后端门禁：`gofmt`, `go build ./...`, `go test -count=1 -timeout 120s ./...` 以及 `go test -race -count=10 -timeout 180s ./internal/application/probe -run TestPeriodicCoordinator_MultiInstanceCASContention` 与相关 probe/sqlite/http race 分包通过

## 4. 独立审查与成品验收门禁（待独立角色复验）

- [x] 4.1 独立 Reviewer 代码审查通过（门禁保留未勾选）
- [x] 4.2 独立 Critic 黑盒成品验收通过（门禁保留未勾选）

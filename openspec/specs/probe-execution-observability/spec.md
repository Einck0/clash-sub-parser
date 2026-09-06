# probe-execution-observability Specification

## Purpose
让节点深度质检以真实可观察的并发执行和定时运行状态呈现给操作者，同时保持已有服务端并发上限、真实出站和结果持久化边界。

## Requirements

### Requirement: Bounded sliding manual probe dispatch

NodeLedger SHALL 从当前已保存的探测设置读取 `probe_concurrency`，并在一次手动综合质检中最多发起该数量的活跃节点探测请求。任一请求完成后，只要尚有未开始节点且操作者未停止，页面 SHALL 立即发起下一个节点请求；页面不得按固定大小分组等待一组节点全部返回。服务端仍 SHALL 把持久化配置作为每个请求的最大允许并发，客户端不得以请求参数提高该上限。

#### Scenario: Slow node does not stall a free slot
- **WHEN** 操作者以并发上限 10 对超过 10 个节点启动综合质检，且其中一个前序节点比其他九个更慢
- **THEN** 其余任一已完成请求的槽位 SHALL 立即开始尚未执行的下一个节点，而不得等待该慢节点完成

#### Scenario: Progressive result and progress update
- **WHEN** 任一节点探测请求返回结果
- **THEN** NodeLedger SHALL 立即合并该节点结果并把完成、正常或异常计数更新为已完成请求的实际数量

#### Scenario: Stop prevents new requests but does not falsify cancellation
- **WHEN** 操作者在手动综合质检期间选择停止
- **THEN** 页面 SHALL 停止派发尚未开始的节点请求，标记进行中的浏览器请求为已取消，并将已经返回的结果保留
- **THEN** 页面不得声称服务端已取消一个已被接收的节点探测，除非服务端明确返回取消确认

### Requirement: Periodic probe runtime status API

系统 SHALL 提供一个受既有 API 认证保护的只读状态响应，供 NodeLedger 查询当前单实例后台质检的配置与运行状态。响应 MUST 包含服务器当前 Unix 时间、总开关、定时开关、周期分钟数、运行状态、最近一次开始和完成时间、最近一次结果摘要或经过脱敏的失败分类，以及当前实例可计算的下次预计触发时间。

#### Scenario: Enabled scheduler awaiting next interval
- **WHEN** 后台调度器已经建立周期基准线，节点质检总开关和定时开关均开启，且没有任务正在运行
- **THEN** 状态响应 SHALL 返回 `waiting`、服务器时间和基于当前实例基准线计算的 `next_expected_at`

#### Scenario: Runtime state resets after process restart
- **WHEN** 后端进程重启且没有共享调度状态存储
- **THEN** 状态响应 SHALL 表示当前实例尚未建立周期基准线或正在等待新基准线，不得把重启前内存中的最近完成或下次时间伪装成当前事实

#### Scenario: Disabled periodic probe
- **WHEN** 节点质检总开关或定时开关关闭
- **THEN** 状态响应 SHALL 返回禁用原因并将 `next_expected_at` 设为 null

### Requirement: Ledger periodic-probe awareness

NodeLedger SHALL 定期读取后台质检状态，并在台账顶部显示定时质检是否启用、当前状态、上次完成或失败摘要及下一次预计触发的倒计时。页面 SHALL 使用响应的服务器时间而不是浏览器系统时间计算倒计时，并在离开页面时清除刷新计时器。

#### Scenario: Server-time-based countdown
- **WHEN** API 返回服务器时间和未来的 `next_expected_at`
- **THEN** 页面 SHALL 以二者的差值开始倒计时，并在后续状态刷新后以新的服务器时间重新校正

#### Scenario: Scheduler running
- **WHEN** 后台质检正在运行
- **THEN** NodeLedger SHALL 显示运行中及已知进度摘要，并且不得同时显示一个保证会发生的下次倒计时

### Requirement: Periodic Background Node Probing

The system SHALL support periodic automated node probing on a configurable interval. The scheduler SHALL wait one full configured interval after this process establishes its initial baseline, then probe all active subscription nodes regardless of any subscription export filtering, persist the results, and prevent overlapping runs within that process. Its current runtime status and estimated next trigger SHALL be queryable through the protected probe status API. This single-process coordination SHALL NOT be represented as a distributed lock or globally accurate schedule when the application runs with multiple workers or replicas.

#### Scenario: Running background probing
- GIVEN `probe_cron_enabled` is true and `probe_cron_interval_minutes` is configured to a positive integer $M$
- WHEN $M$ minutes have elapsed since the current process established its previous background-probe baseline and no periodic probe is running
- THEN the background scheduler triggers probing for all active subscription nodes, including nodes excluded only from generated subscription output, persists the results, and exposes a running status

#### Scenario: Overlap prevention
- WHEN a periodic probe remains active when a later scheduler tick occurs
- THEN the later tick SHALL start no second run and the status API SHALL continue to represent the active run rather than report a fabricated completion

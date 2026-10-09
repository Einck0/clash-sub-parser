## Why

用户反馈节点“全部异常”，并要求探针每分钟更新状态：有状态变化才检测，手动刷新状态时该检测的也要检测。现有观测能证明的是“最近 21 个启用节点 baseline 均记录 error”，而非 21 条线路均不可用：其中 18 条记录为 `credentials_unavailable`（实际有配置凭据；6 条 hysteria2 因不安全 TLS 被安全拒绝，12 条 vless 含 UUID/vision/reality，需校准构建配置），1 条 `private_target_rejected`，另 2 条 `transport_error` 还缺足以断言根因的证据。必须先区分探针能力/安全策略、未知与线路真实失败，再调整巡检，避免把节点误染全红或全绿。

## What Changes

- 校准运行时 sing-box VLESS/vision 等探测配置与现有导出语义；安全拒绝、缺失配置、构建不支持及真实传输失败分开归因，保留脱敏证据，不放宽严格 TLS 和公网目标限制。
- 冻结共享状态口径：仅新鲜、可判定的 baseline 给出整体线路健康；baseline 缺失、过期或无法检测应为未知，能力类别结果只表示对应能力，不代替 baseline；节点视图和 pool 计数一致。
- 服务端每分钟**扫描状态**而非全量测速；仅观测缺失/到期、有效连接参数实质改变或活跃切换使对应类别待测；保留现有单轮 512 任务预算、失败 5 分钟冷却、租约和队列去重，防止由观测写入自触发。
- 手动“刷新状态”先重读最新状态，走同一增量候选判定并反馈排队/无待测状态；与已有“全量测速”分开，手动刷新不得推迟服务端正常分钟扫描。
- 不声称 2 条 SS 节点已证实故障；本 Change 不执行生产探针、迁移或部署。是否真正异常待实现后通过受控、获授权的实机复测判定。

## Capabilities

### New Capabilities
- `probe-evidence-classification`: 安全约束下配置编译一致性与探测失败归因。
- `baseline-health-read-model`: 节点整体健康与探测池聚合的共享基线口径。
- `minute-incremental-state-sweep`: 一分钟服务端状态扫描、配置变化失效及手动状态刷新。

### Modified Capabilities

无已归档的 `openspec/specs/` 能力；本 Change 与历史未归档的 `csp-incremental-expiry-probe-sweep` 契约的 10 分钟默认值及控制台文字存在明确后继覆盖关系，不修改/复用其历史 Change。

## Impact

后端 `internal/probe/singbox/`、`internal/application/probe/`、`internal/application/inventory/`、必要的 `internal/domain/`、SQLite 仓储及显式装配调用点；Web 探针控制台与节点台账状态展示、必要 HTTP 接口。优先复用现有 `ListLatestByNodes`、CAS 租约、`EnqueuePeriodicDedupe`、安全拨号器和既有状态字段；若需要存储连接参数签名，仅添加有版本且向后兼容的持久化迁移，不把 `nodes.updated_at`、抓取时间或观测时间当作连接变化证明。不增加破坏性字段或升级用户观测为伪造成功。

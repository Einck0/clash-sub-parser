## Why

CSP 的 Workbench 虽已有部分窄屏样式和抽屉原语，但移动端仍存在小于 44px 的导航触点、NodeLedger 表格式密集交互、全局横向溢出风险，以及 Rules 仍使用旧视觉与不适合手机的卡片网格。与此同时，探测配置把手动质检启用、后台调度周期和单节点总超时混为一组：默认并发仍为 5，服务探针未获得独立 deadline，且“周期为 0”同时充当调度开关。

本变更以既有 Workbench 与真实 sing-box 探测链路为基线，补齐可验证的移动端交互、每个服务探针的独立 2 秒超时、默认 10 并发和可热更新的定时质检设置，不删除任何现有非 SCRIPT 能力或历史数据。

## What Changes

- 将 Workbench 壳体的移动导航规范化：在小于 768px 的屏幕使用 44px 汉堡入口和受控左侧导航抽屉；主内容、页面工具栏与全局层禁止产生页面级横向滚动。
- 将所有共享 Drawer/Modal 的窄屏展示统一为可关闭的 Bottom Sheet，保留焦点陷阱、Escape、背景滚动锁、明确关闭按钮和 iOS 安全区；为可选的向下拖拽关闭定义阈值与不误触表单控件的边界。
- 为 NodeLedger 补充移动专用紧凑列表渲染器，保留同一筛选、选择、批量质检、单节点质检、详情及跳板能力；手机端不依赖虚拟桌面行的多列布局，也不让操作控件溢出。
- 将 Subscriptions、NodeGroups、ProxyChains、Rules 和 Settings 的移动端布局统一为单列、可折行的工具栏、满宽或等宽操作区及安全的表单栅格；Rules 迁入现有语义 token 与共享 UI 原语，不保留旧页面的独立配色体系。
- 将探测配置扩展为 `probe_concurrency` 默认 10、`probe_service_timeout_ms` 默认 2000，以及独立的 `probe_cron_enabled` 默认 true、`probe_cron_interval_minutes` 默认 60；保留旧 `probe_timeout_ms` 作为单个节点端到端总预算，避免把新服务级超时误解为整批或整节点 deadline。
- 让 transport、出口地理、每个流媒体或 AI provider、受控测速和 sing-box 启动等待各自受 `probe_service_timeout_ms` 约束。服务超时必须记录结构化 `timeout` 结果；它不得取消同节点的其他独立服务探针，也不得阻塞 semaphore 中的其他节点。
- 将批量质检并发控制收束为由设置默认值驱动的 semaphore，允许 API 调低但不允许绕过服务端配置上限；前端不再硬编码批量并发为 5。
- 将 APScheduler 的每分钟检查保留为调度 tick，但改为读取新的 `probe_cron_enabled` 和 `probe_cron_interval_minutes`。Settings PATCH 成功提交后，下一 tick 必须使用新配置，无需容器重启；运行中的批次使用启动时冻结的配置快照，避免半批改变行为。
- 为 SQLite/Alembic、配置导入导出白名单、Settings API、Settings 表单、后端并发和超时、调度热更新及 375/768/1024px 关键交互补充回归测试。

## Capabilities

### New Capabilities

- `mobile-workbench-responsiveness`: Workbench 壳体、移动导航、Bottom Sheet、核心管理视图和 NodeLedger 的响应式与可访问交互契约。
- `probe-execution-orchestration`: 批量探测的配置快照、服务级 deadline、并发上限、超时结果和调度热更新契约。

### Modified Capabilities

- `node-capability-probe`: 将单节点能力探测调整为服务级超时和可配置的最大并发，同时保持真实经被测节点出站与现有平台能力语义。
- `node-probe-persistence-and-filtering`: 将后台自动质检开关和间隔从旧的 `probe_interval_minutes` 语义迁移为独立且可热更新的 cron 配置。

## Impact

- 后端受影响范围包括 `ProbeConfig`、Alembic 迁移桥接与配置导入导出、probe schemas/router/service/providers/runner、APScheduler 和 probe 测试；不迁移或删除现有 `node_probe_results` 数据。
- 前端受影响范围包括 App/Workbench Header/Sidebar、共享 BaseDrawer、NodeLedger、Subscriptions、NodeGroups、ProxyChains、Rules、Settings、主题样式、API client 和响应式测试。现有路由、Quick Export 和桌面大列表虚拟化保持。
- 运行期需要在同一个 `backend-data` SQLite volume 上执行可逆 Alembic schema upgrade；无新第三方运行时依赖。部署仅在 executor 实现、reviewcommon APPROVED 和完整验证之后，由后续部署卡执行 `docker compose up -d --build` 与健康检查。

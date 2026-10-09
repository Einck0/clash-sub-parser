## Why

当前 CSP 策略树和探针引擎存在四大核心缺陷：
1. **全量节点截断导致匹配丢失**：`publication/service.go:812` 与 `policy/service.go:932` 调用 `nodeRepo.List` 时未指定分页参数，触发 `sqlite/nodes.go:114` 默认 `pageSize=50` 截断。在 90 个活跃节点的真实节点池中，台湾（排序 70-72）与加拿大（排序 78-80）节点被硬截断，导致策略组匹配为空，用户误以为是旧快照或规则失效；
2. **连接边与节点筛选条件语义混淆**：用户无法直观理解「显式连接边筛选」与「动态全池筛选」的关系，误以为规则冲突或应合并；
3. **探针健康状态失真与未检测混淆**：`BaselineUnknown` 叠加已有 AI/流媒体等辅助观测时被 `UntestedCount` 与前端 `unprobed` 混淆；纯辅助 `available` 危险回退为 `healthy` 破坏了只有新鲜 baseline 才能评判健康的铁律；缺乏 `undetermined` 状态导致已测辅助能力的节点被误报为「未检测」；
4. **前端分页缺失、瀑布流开销与侧栏布局滚动逃逸**：`usePolicy` 仅加载第 1 页 100 条组，`PolicyEditorSheet` 传 `page_size=200` 被后端截断至 100 漏掉后续节点，`useProbes` 存在 20 页瀑布流轮询，`App.vue` 侧栏随整页滚动消失，移动端遮罩缺乏体滚动锁定。

## What Changes

- **全量节点读取（ListAll）消除截断**：在 `NodeRepository` 引入显式全量查询接口 `ListAll`，供 `policy/service.go`（校验）与 `publication/service.go`（导出）使用，绕过 UI 分页截断，完整保留订阅 Scope、ActiveOnly、ExcludeNotices 与 IP 风险准入。严格保持 HTTP API 的默认 50/最大 100 分页限制，不盲改全局默认值；
- **澄清组连接边与筛选条件原生语义**：在 UI 统一编辑交互中明确区分「显式连接边（若存在边则候选边被筛选及父筛选下传）」与「动态全池规则（仅无任何边且非空 filter 时动态遍历全池）」，不改变底层代数运算，严禁自动静默删除生产连接边或筛选条件；
- **探针健康状态互斥守恒与 Undetermined 细分**：
  - 废除纯辅助能力（如 AI/Streaming）`available` 回退为整体节点 `healthy` 的危险逻辑；整体健康必须且仅可信新鲜的当前 revision baseline；
  - 区分真正的「全无观测未检测 (Untested)」与「观测不确定/过期/版本过时/待复核 (Undetermined)」；
  - 后端 `ProbePoolStatus` 引入 `undetermined_count`，满足互斥守恒：`total_count = healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count`；
  - 后端 `/api/v1/nodes` 支持全 Scope 的数据库级健康状态查询（`health_status`），禁止在前 100 条前端内存中统计全池；
- **服务端分页、候选异步搜索与拓扑解耦**：
  - 策略组列表实现服务端标准分页（默认 50，上限 100）与搜索；
  - 策略编辑器提供完整的异步搜索/分页候选节点与子组选择，坚决杜绝静默漏掉第 100 个以后的节点；
  - 拓扑全图从独立接口或全量元数据解析渲染，与卡片列表分页明确解耦，拓扑全图不可被分页截断；
  - 移除 `useProbes` 的 while-20 全量瀑布流，改为标准分页和直接使用后端 Pool 统计指标；
- **桌面端粘性侧栏与移动端体滚动锁**：
  - 重构 `App.vue` 布局：Desktop 侧栏采用 `sticky top-0 h-screen`（或 `h-dvh`），在窗口滚动时永久固定；
  - 移动端 Bottom Dock 与 Drawer/Sheet 弹出时启用 `overflow: hidden` 体滚动锁定，防穿透滚动；
  - 经由 4 视口（Desktop 1280x800, Landscape 667x375, Mobile 375x667, Mobile 392x872）严格验证。

## Capabilities

### New Capabilities
- `policy/full-inventory-resolution`: 后端全量节点读取（ListAll）消除策略树与导出截断，支持 250+ 节点完整解析与准入保留。
- `policy/membership-semantics-and-pagination`: 策略组成员筛选规则与连接边原语义澄清，策略组列表服务端分页搜索，候选成员异步完整检索与拓扑全图解耦。
- `probe/health-state-conservation-and-pagination`: 探针健康状态互斥守恒与 undetermined 计数分类，删除危险的辅助可用回退，全局节点分页健康筛选。
- `ui/navigation-layout-and-viewport-resilience`: Desktop 侧栏粘性固定布局，Mobile 导航与 Overlay 遮罩体滚动锁定，多视口适配保障。

### Modified Capabilities

## Impact

- **后端持久层与服务层**：`internal/domain/ports.go`（新增 `ListAll`）、`internal/repository/sqlite/nodes.go`、`internal/application/policy/service.go`、`internal/application/publication/service.go`、`internal/application/probe/service.go`、`internal/transport/http/nodes.go`、`internal/transport/http/policies.go`；
- **前端模型与视图层**：`web/src/features/nodes/nodeView.ts`、`web/src/features/probes/useProbes.ts`、`web/src/features/probes/ProbesView.vue`、`web/src/features/policy/usePolicy.ts`、`web/src/features/policy/PolicyView.vue`、`web/src/features/policy/PolicyEditorSheet.vue`、`web/src/App.vue`；
- **API 契约**：`/api/v1/policies/groups` 明确分页与搜索参数，`/api/v1/probes/pool` 响应增加 `undetermined_count`，`/api/v1/nodes` 扩展 `health_status` 查询过滤；
- **隔离性与无副作用**：所有改动纯本地离线测试，无外网依赖，不执行任何外部生产调用。

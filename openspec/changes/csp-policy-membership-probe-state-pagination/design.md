## Context

参考 `proposal.md`。
在当前 CSP 架构中：
1. **策略解析与发布截断**：`publication/service.go:812` 与 `policy/service.go:932` 在构造 Resolver 输入时均调用 `s.nodeRepo.List(ctx, domain.NodeFilter{ Scope: domain.NodeScopeEnabledSubscriptions, ActiveOnly: true, ExcludeNotices: true })`。由于未传入 `Pagination` 结构，SQLite 实现 `internal/repository/sqlite/nodes.go:114` 命中默认逻辑 `pageSize = 50`。当活跃节点池达到 90 个节点时，加拿大（第 78-80 位）与台湾（第 70-72 位）节点被物理截断，导致策略组计算出空组，用户误以为是旧快照；
2. **成员判定语义**：Resolver 原生算法根据是否有边严格分流：若存在显式边，则以边为候选并叠加组筛选；仅在边集合完全为空且非空 `node_filter` 时动态扫描全池。但前端界面未提供模式清晰引导，致使操作者困惑筛选与边是否冲突；
3. **探针健康状态与互斥守恒**：`nodeView.ts` 存在将非 baseline（如 AI）的 `available` 判定危险回退为整体 `healthy` 的逻辑；同时 `BaselineUnknown` 被简单归为 `unprobed` / `UntestedCount`，导致「AI 测过可用却显示未检测」；缺乏 `undetermined` 状态，池统计未保持互斥守恒；
4. **前端分页缺失与视图布局缺陷**：`usePolicy` 仅加载单页 100 条组；`PolicyEditorSheet` 传递 `page_size=200` 被后端硬截断至 100；`useProbes` 采用 `while (page <= 20)` 瀑布流并发拖慢性能；`App.vue` 侧边栏未设粘性定位，导致页面滚动时导航消失。

## Goals / Non-Goals

**Goals:**
- **消除全量截断**：`NodeRepository` 新增专用全量读取接口 `ListAll`，在 `policy` 校验与 `publication` 导出中显式全量拉取，保留全部过滤准入条件与 IP 风险控制；测试用例覆盖 250+ 节点（目标节点排在 100 之后）；
- **保留原有代数语义**：澄清「显式连接边 + 条件过滤」与「零边全池动态规则」的双轨模式，不破坏组合运算，不自动删除生产数据中的任何边或 filter；
- **探针健康权威与守恒**：删除辅助能力回退 `healthy` 的危险逻辑；整体健康必须且仅依赖新鲜当前 revision baseline；新增 `undetermined_count` 与 `undetermined` 分类；保持 `total_count = healthy + degraded + unavailable + undetermined + untested` 守恒；后端提供全 Scope 数据库级健康筛选；
- **前端标准分页与异步检索**：策略组列表支持服务端分页与搜索；候选成员选择支持异步检索与分页；`useProbes` 废除 20 页瀑布流；拓扑全图解耦独立展现；
- **导航粘性与移动端体验**：桌面侧栏 `sticky top-0 h-screen` 固定；移动端 Modal/Drawer 弹出时锁定 `body` 滚动；经 4 视口回归验证；
- **独立工程写集合**：Backend 包（Go/migrations）与 Frontend 包（web）写集合严格物理正交。

**Non-Goals:**
- 不修改 `/api/v1/nodes` 针对 UI 列表的默认分页上限（保持默认 50，最大 100），防止无界内存膨胀；
- “批量”严格限定为分批加载与分页（chunked loading / pagination），不增加节点或策略组的业务批量启停、批量增删操作；
- 避免全量外网探测依赖，单测与集成测试均采用本地合成 Fixture。

## Decisions

### 1. 全量节点查询接口 (`ListAll`) vs 盲改 UI 分页上限
- **决策**：在 `internal/domain/ports.go` 中为 `NodeRepository` 显式增加 `ListAll(ctx context.Context, filter NodeFilter) ([]Node, error)`，在 `sqlite.nodeRepository` 中实现专用的无分页上限读取通道。`publication/service.go` 与 `policy/service.go` 改用 `ListAll`；
- **理由**：UI 列表查询必须有上限（最大 100）以保护网络传输与浏览器内存；而编译器/校验器属于单次全量内存计算，必须获得全量已准入活跃节点；
- **替代方案考虑**：若直接将 `nodeRepo.List` 的 `MaxPageSize` 改大（例如 10000），会导致前端请求不规范时可能拉爆服务器内存，破坏 API 保护边界。

### 2. 策略组成员判定原语义保留与界面交互重梳
- **决策**：严格保留 `internal/resolver/graph.go` 的代数运算规则。在 `PolicyEditorSheet.vue` 中重构交互：
  - 显示模式指示徽章：
    - 当配置了连接边时：标为「显式连接边模式（组筛选仅作为入选边之二次过滤）」；
    - 当未配置连接边且配置了筛选条件时：标为「全池动态匹配模式（自动遍历全局活跃节点）」；
    - 当连接边包含子策略组时：标为「子策略组级联模式（父级筛选条件向下级递归继承）」；
  - 增加清晰的警示说明，不执行任何自动静默删除边或筛选条件的破坏性操作。

### 3. 探针健康权威性修正与 `undetermined` 状态分流
- **决策**：
  1. 在 `web/src/features/nodes/nodeView.ts` 中删除 `if (values.includes('available')) return 'healthy'`；
  2. 节点整体健康状态仅在拥有当前 `connection_revision` 且处于新鲜期内的 baseline 观测时才可判定为 `healthy` / `degraded` / `unhealthy`；
  3. 当基线不存在、过期、版本不匹配，但存在其他有效探测（如 AI、IPRisk 等）或探测结果待确认时，分类为 `undetermined`（待复核）；
  4. 仅在节点没有任何探测观测时，才标记为 `untested`（未检测）；
  5. 后端 `domain.ProbePoolStatus` 结构体新增 `UndeterminedCount int json:"undetermined_count"`；
  6. 后端 `/api/v1/nodes` 支持 `health_status` 查询参数，在数据库层面通过 `buildNodeFilterPredicates` 进行全 Scope 过滤，禁止依赖前端内存计算全量。

### 4. 冻结后端 ↔ 前端 API 准确契约

#### 4.1 策略组列表 (`GET /api/v1/policies/groups`)
- **请求参数**：
  - `page`: 整数，默认 1
  - `page_size`: 整数，默认 50，最大 100
  - `search`: 字符串，可选，针对 `name` 与 `group_type` 进行不区分大小写匹配
- **响应体**：标准 Paginated Envelope：
  ```json
  {
    "code": "success",
    "data": {
      "items": [ ...GroupView ],
      "page": 1,
      "page_size": 50,
      "total": 42
    }
  }
  ```

#### 4.2 节点列表 (`GET /api/v1/nodes`)
- **请求参数**：
  - `page`: 整数，默认 1
  - `page_size`: 整数，默认 50，最大 100
  - `scope`: `enabled_subscriptions` | `active_subscriptions` | `all`，默认 `enabled_subscriptions`
  - `active_only`: 布尔值，可选
  - `search`: 字符串，可选（匹配名称/服务器）
  - `health_status`: 字符串，可选，逗号分隔支持 `healthy,degraded,unhealthy,undetermined,untested`
  - `protocol`: 字符串，可选
- **响应体**：标准 Paginated Envelope。

#### 4.3 探针池指标 (`GET /api/v1/probes/pool`)
- **响应体**：
  ```json
  {
    "code": "success",
    "data": {
      "total_count": 90,
      "available_count": 45,
      "healthy_count": 40,
      "degraded_count": 5,
      "unavailable_count": 15,
      "undetermined_count": 20,
      "untested_count": 10,
      "queue_nodes_count": 2,
      "probing_nodes_count": 1
    }
  }
  ```
- **守恒校验公式**：
  `total_count === healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count`
  `available_count === healthy_count + degraded_count`

### 5. 前端导航固定与多视口滚动隔离
- **桌面端**：`App.vue` 中的 `<aside>` 容器设为 `sticky top-0 h-screen shrink-0 overflow-y-auto`，无论主内容区域如何纵向滚动，侧栏永久吸顶可见；
- **移动端**：在弹出 Bottom Sheet（如 `PolicyEditorSheet`）、ModalDialog 或 Drawer 时，通过 Vue composition hook (`useBodyScrollLock`) 为 `document.body` 添加 `overflow: hidden; touch-action: none;`，关闭时释放，彻底消除双重滚动穿透；
- **四视口适配测试**：在 `web/e2e` 中配置 Desktop 1280x800、Mobile 375x667、Mobile Large 392x872、Landscape 667x375 自动化回归校验。

## Risks / Trade-offs

- **[Risk] 250+ 节点全量读取可能轻微增加一次性内存消耗** → **Mitigation**: 限制于 `publication` 与 `policy` 单次计算，只拉取当前启用的活跃节点，不持久持有，垃圾回收可立即回收；
- **[Risk] `undetermined_count` 引入打破旧版前端仅识别 3 种状态的假设** → **Mitigation**: 在 `nodeView.ts` 与 `probesView.vue` 中同步更新 KPI 卡片、进度条和过滤器，保证守恒公式总和为 100%；
- **[Risk] 异步候选节点搜索可能因网络抖动引起多次请求** → **Mitigation**: 在候选搜索输入框接入防抖（debounce 300ms）并保留当前选中项的状态不被覆盖。

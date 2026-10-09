# Clash Sub Parser (CSP) - 启用订阅库存与台账节点范围取证审计报告 (Sanitized)

- **取证执行环境**:
  - `Parent Session`: `sess_20261003_004041_3864824`
  - `Subagent Intent Artifact`: `/root/.pi/history/clash-sub-parser/artifacts/sess_20261003_004041_3864824/context/csp-inventory-scope-2026-10-04T02-47-38.md`
  - `Intent SHA256`: `f65ff49aef428d522a157eb17f2e332d479a4ba2e985bee223a7275ef18c29af`
  - `Git Baseline`: Commit `5f2f941` (`docs(deployment): record verified dataflow release and benchmark boundaries`), clean working tree.
  - `运行态容器`: `5aaefb45714c` (`clash-sub-parser-app`, ports 18080/17000, schema 15, healthy).
  - `数据库`: `/data/csp-v1.db` (单 SQLite 主库, WAL 模式).

---

## 一、 端到端调用链路与 1000+ 节点显示定位 (Trace: API -> Service -> SQL -> UI)

| 业务视图 / 组件 | 前端调用代码与参数 | 后端 HTTP 处理入口 | 领域服务与持久层实现 | 实际执行 SQL 逻辑 | 生产实测数值 |
|---|---|---|---|---|---|
| **首页概览 Dashboard** | `DashboardView.vue:59`<br>`api.get('/api/v1/nodes', { params: { page: 1, page_size: 1 } })` | `transport/http/nodes.go:37`<br>`h.list` | `inventory/service.go:1205`<br>`ListNodesReadModel` | `repository/sqlite/node_risk.go:119`<br>`SELECT COUNT(*) FROM nodes;` (无过滤条件) | **1014** (全库全量资产数) |
| **节点列表 NodesView** | `useNodes.ts:117` & `NodesView.vue:414`<br>`api.get('/api/v1/nodes', { params })`<br>(未传递 `active_only` 亦无订阅范围参数) | `transport/http/nodes.go:37`<br>`h.list` (默认返回全库) | `inventory/service.go:1205`<br>`ListNodesReadModel` | `repository/sqlite/node_risk.go:138`<br>`SELECT COUNT(*) FROM nodes;`<br>`SELECT ... FROM node_eval LIMIT ? OFFSET ?;` | **1014** (列表展示全部 1014 个节点，含 983 失活节点) |
| **订阅卡片 Subscriptions** | `SubscriptionsView.vue:240-315`<br>`useSubscriptions.ts:180`<br>`api.get('/api/v1/subscriptions')` | `transport/http/subscriptions.go:180`<br>`h.list` | `subscription/service.go:74`<br>`SubscriptionView` (未含 `node_count` 字段) | `repository/sqlite/subscriptions.go`<br>`SELECT ... FROM subscriptions;` | 卡片未显示单个订阅节点数；刷新摘要返回 `nodes_valid` |
| **探测池当前状态 ProbePool** | `useProbes.ts` / `ProbesView.vue`<br>`api.get('/api/v1/probes/pool')` | `transport/http/probes.go`<br>`h.getPoolStatus` | `probe/service.go:760-820`<br>`GetPoolStatus`<br>使用 `ActiveOnly: true, ExcludeNotices: true` | `repository/sqlite/nodes.go:130`<br>`SELECT ... FROM nodes WHERE active = 1;` | **31** (仅统计活跃节点) |
| **周期性探测调度 Periodic** | 后台调度引擎 (Runner / Scheduler) | 无 (内部服务) | `probe/periodic.go:621`<br>使用 `ActiveOnly: true, ExcludeNotices: true` | `repository/sqlite/nodes.go:130`<br>`SELECT ... FROM nodes WHERE active = 1;` | **31** (仅对 31 个活跃节点分片拨测) |
| **发布与订阅导出 Publication** | `POST /api/v1/publications/{id}/preview`<br>`GET /publish/v1/{id}` | `transport/http/publications.go` | `publication/service.go:775`<br>使用 `ActiveOnly: true, ExcludeNotices: true` | `repository/sqlite/nodes.go:130`<br>`SELECT ... FROM nodes WHERE active = 1;` | **31** (最终导出 31 个节点) |

---

## 二、 生产 SQLite 数据库全量资产与集合对账矩阵 (Reconciled Sets)

基于生产环境 `/data/csp-v1.db` 只读查询（完全无写入、无订阅状态变更）：

| 指标 / 集合项 | 统计定义与 SQL 约束 | 集合计数值 | 说明与业务含义 |
|---|---|---|---|
| **全库节点资产总数** | `SELECT count(*) FROM nodes;` | **1014** | 包含存量所有历史保护资产与当前抓取资产 |
| ↳ **活跃节点子集 (`active=1`)** | `SELECT count(*) FROM nodes WHERE active = 1;` | **31** | 当前可用并参与拨测/导出的节点集合 |
| ↳ **失活节点子集 (`active=0`)** | `SELECT count(*) FROM nodes WHERE active = 0;` | **983** | 历史导入/旧刷新留存的非活跃受保护资产 |
| **有 Source 关联的节点总数** | `SELECT count(DISTINCT node_logical_id) FROM node_sources;` | **31** | 仅 31 个节点在 `node_sources` 表中有归属记录 |
| **无 Source 关联的历史孤立节点** | `SELECT count(*) FROM nodes WHERE logical_id NOT IN (SELECT node_logical_id FROM node_sources);` | **983** | 存量 983 个历史节点未挂载任何现存订阅源 |
| **订阅源总数** | `SELECT count(*) FROM subscriptions;` | **9** | 系统内配置的 9 个订阅配置 |
| ↳ **已启用订阅源 (`enabled=1`)** | `SELECT count(*) FROM subscriptions WHERE enabled = 1;` | **4** | Dogegg (16 节点), einck-qzz (15 节点), 7li (0 节点，上次失败), 魔戒 (0 节点，上次失败) |
| ↳ **已禁用订阅源 (`enabled=0`)** | `SELECT count(*) FROM subscriptions WHERE enabled = 0;` | **5** | 7li7li-便宜, WARP, Eeox, 公开节点, Githubusercontent (均无拉取记录) |
| **挂载至已启用订阅的 Distinct 节点** | `JOIN subscriptions s ON s.id = ns.subscription_id WHERE s.enabled = 1` | **31** | 16 (Dogegg) + 15 (einck-qzz) = 31，当前两者无重合节点 |
| **挂载至已禁用订阅的 Distinct 节点** | `JOIN subscriptions s ON s.id = ns.subscription_id WHERE s.enabled = 0` | **0** | 当前已禁用的 5 个订阅在 `node_sources` 中无任何关联节点 |
| **多源共享节点 (同时属于启用与禁用)** | `node_logical_id IN (enabled_sources) AND IN (disabled_sources)` | **0** | 当前生产数据中无跨启用/禁用的多源重叠节点 |
| **当前最新成功 Fetch 条目数** | `ns.last_seen_fetch_id = latest_successful_fetch.id` | **31** | 全部 31 个节点均精确匹配各自订阅最后一次成功/部分成功 Fetch |
| **条目表公告与分类记录 (`subscription_entries`)** | `SELECT count(*) FROM subscription_entries;` | **0** | Migration 15 上线后生产尚未执行刷新重放，条目表尚无新流水 |

---

## 三、 根因分析：已确证根因 (Proven Root Cause) vs 排除的假说

### 1. 已确证根本原因 (Proven Root Cause)
1. **视图查询语义与数据模型脱节**：
   - 首页 `DashboardView.vue` 与节点台账 `NodesView.vue` 前端直接请求未加过滤的 `/api/v1/nodes`；
   - 后端 `ListReadModel` 与 `List` 默认对 `SELECT COUNT(*) FROM nodes` 全表扫描计件，返回包括 983 个失活历史节点在内的 1014 个节点资产；
   - 探针模块 (`ProbePoolStatus`、`PeriodicScheduler`) 和发布模块 (`PublicationService`) 在底层均已硬编码 `ActiveOnly: true, ExcludeNotices: true`，实际使用 31 个节点；
   - 两侧范围未统一，导致用户在首页与台账看到 1014 个节点，而在探针与发布处看到 31 个节点，产生“节点数完全不统一且多出 1000 多个节点”的直接体验缺陷。
2. **`Node.active` 与 `Subscription.enabled` 缺乏联动约束**：
   - `Subscription.enabled` 仅控制订阅是否参与定时拉取，修改订阅开关并不会联动更新 `nodes.active` 字段；
   - 现存 API 缺少“按当前启用订阅及其最新有效抓取轮次过滤”的服务层与仓储层支持。

### 2. 已排除的假说 (Disproven Hypotheses)
- **假说 1：Logical ID 漂移导致节点重复冗余**（排除）：Base ID 与 Scoped ID 稳定，无重复哈希节点。
- **假说 2：禁用订阅下的节点仍在 `node_sources` 中活跃残留**（排除）：对账显示当前 5 个禁用订阅在 `node_sources` 中关联数为 0。
- **假说 3：多订阅共享节点导致全局计数虚高**（排除）：当前 31 个节点在 Dogegg 与 einck-qzz 间完全互斥，交集数为 0。

---

## 四、 节点生命周期语义与业务规则建议

1. **生命周期语义澄清 (Lifecycle Invariants)**:
   - **刷新失败 (Failed Refresh)**: 严格保全上次良好库存（Non-destructive invariant），不得解绑 `node_sources` 或清空 `nodes`。
   - **刷新成功但条目消失**: `node_sources` 中该节点记录不被物理删除，但其 `last_seen_fetch_id` 停留在旧 fetch，可通过比对最新成功 fetch 区分“当前有效成员”与“历史脱落条目”。
   - **多订阅归属 (Per-Sub vs Global Dedup)**: 单个订阅卡片展示的节点数（Per-Sub）在未来可能存在多源共享；因此 `Sum(Per-Sub) >= Global Deduplicated Count` 属于合理多对多拓扑事实，台账主视图必须按 `logical_id` 去重。
2. **全局主视图语义规则 (Global Main View Policy)**:
   - **主视图默认展示范围 (Active Enabled Inventory)**: 仅展示至少属于一个**当前启用 (`enabled=1`)** 订阅源、且属于该订阅**最后一次成功抓取 (`last_seen_fetch_id = latest_success_fetch_id`)** 的**有效非公告 (`active=1 AND (entry_kind IS NULL OR entry_kind != 'notice')`)** 代理节点；
   - **用户手动添加无订阅节点 (Manual Nodes)**: 保持在资产台账底表中持久化（不可变版本与 heads 完整保全），默认主视图若强调“订阅库存”可通过视图切换器（`全部资产台账 (1014)` vs `当前订阅生效节点 (31)`）进行安全切换，绝不物理删除历史资产；
   - **可解析未知节点 (`unknown`)**: 保持作为候选代理保留，严禁盲目封杀；
   - **详情与编辑接口 (`GET/PATCH /api/v1/nodes/{logical_id}`)**: 保持全局资产范围权限，允许管理员按 `logical_id` 调阅与覆盖任何历史节点参数，不受列表主视图范围限制。

---

## 五、 工具与认证渠道缺口分析 (Vault & Credential Tool Gap)

- **当前工具边界**: 当前执行机具备 `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`, `caller_ping` 等本地文件与进程工具，**未装配任何浏览器端会话桥接工具（如 `playwright` 驱动）**，无法调用前台用户已登录的浏览器 `csp_session` cookie。
- **安全规范遵守**: 执行机严格遵守安全契约，严禁提取会话私钥、严禁关闭生产鉴权或修改生产密码。
- **前台管理验收最小 GET / 操作清单**:
  1. `GET /api/v1/auth/status` (验证当前 Session 是否有效)
  2. `GET /api/v1/nodes?scope=active_enabled` (获取当前启用订阅可用节点，预期 31 条)
  3. `GET /api/v1/subscriptions` (获取 9 个订阅及其状态)
  4. `GET /api/v1/probes/pool` (获取当前拨测池状态，预期总数 31)
  5. `GET /api/v1/publications` (获取当前发布工件)
- **隔离测试替代方案**: 所有逻辑改动与集合对账完全可通过 SQLite 官方 `.backup` 至受控私密临时目录，配合本地测试用例以零凭据、自包含方式 100% 闭环复验。

---

## 六、 施工写集合与并行拆分建议 (Write Sets & Parallelism)

后端服务层与前端 UI 层的修改范围完全正交，写集合不重叠，无共享生成物：

### 1. 后端工作包 (`pkg_backend_inventory_scope`)
- `internal/domain/ports.go`: 扩展 `NodeFilter`，增加 `Scope` / `EnabledSourcesOnly` 与 `ExcludeStaleFetch` 字段；
- `internal/repository/sqlite/node_risk.go`: 在 `ListReadModel` 及 `Count` 中增加对 `node_sources` 与 `subscriptions.enabled = 1` 的关联查询；
- `internal/transport/http/nodes.go`: 路由与参数解析支持 `scope=active_enabled`（或默认启用订阅范围）并保留 `scope=all` 用于资产审计；
- `internal/application/inventory/service.go`: 强化对账与读取模型。

### 2. 前端工作包 (`pkg_frontend_inventory_views`)
- `web/src/features/nodes/useNodes.ts`: 默认请求 `scope=active_enabled`；
- `web/src/features/nodes/NodesView.vue`: 头部显示当前生效节点数（31），提供查看全量台账资产的选项；
- `web/src/features/dashboard/DashboardView.vue`: 概览卡片使用生效节点数（31）或呈现双指标；
- `web/src/features/subscriptions/SubscriptionsView.vue`: 在卡片上呈现各订阅归属的节点数。

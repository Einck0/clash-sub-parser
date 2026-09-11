## Why

节点质检状态目前以两套不完整的身份视图连接：`GET /api/probe/results` 的摘要 map 以 `node_key` 为键，但摘要值不带身份；`GET /api/proxy-chains/meta/node-ledger` 又不带 `node_key`。Node Ledger 的桌面模板仍以显示名索引摘要，因而分页摘要重新加载后可把真实 `fail`、`timeout` 或 `unknown` 错误误呈为“未测”。同时，`node_probe_results` 不随当前有效订阅节点集合收敛，历史记录会占用摘要分页游标并延迟当前节点的可见状态。

前端已有 Vue 3、Tailwind v4、Lucide、共享 Button、Modal/Drawer 和局部 token 化基础，但尚未采用 shadcn-vue 的可审计组件分发结构；且当前 shadcn-vue 最新体系以 Reka UI v2（Radix Vue 的后续名称）为无样式可访问原语。此次变更将以该当前体系重构，而不是引入过期的 Radix Vue 包或并存的私有组件层。

## What Changes

- **BREAKING (probe summary projection):** `GET /api/probe/results` 继续只以 `node_key` 为结果 map 键和游标排序，但每个摘要值增加受限身份回显 `node_key` 与 `name`；其余仅允许状态、延迟、速度、国家、IP 和布尔媒体矩阵。它仍不得返回 server、port、type、ASN、组织、错误详情、时间戳、身份证据、原始响应或任何凭据。
- **BREAKING (ledger projection):** `GET /api/proxy-chains/meta/node-ledger` 的每个有效最终节点必须包含与探测持久化使用同一算法得到的非空 `node_key`。后端只保留一个可复用的纯身份构造器；前端不得自行再实现字符串拼接或以显示名猜测身份。
- 定义前端唯一匹配规则：`node_key` 是唯一主键；`name` 只可用于单次探测的临时兼容回填，且仅当同名候选唯一时使用。批量摘要、分页合并、过滤、统计、抽屉详情和所有桌面/卡片/移动状态渲染都通过 `getProbeForNode` 取记录，绝不直接以显示名读取 map。
- 定义显式状态模型：无记录才是 `untested`；任何已收到但不在已知枚举中的状态均归一为 `unknown` 并显示“状态未知”，不得回退为“未测”。`fail` 与 `timeout` 保持各自可见的失败语义。
- 增加当前有效节点集合与持久化探测结果的收敛机制：在成功刷新、手动节点重算、会改变最终节点集合的订阅更新或删除后，清理不再属于当前有效最终节点集合的探测结果和进程缓存。摘要读取仍以当前集合过滤，防止在并发探测期间被迟到写入的孤儿记录重新出现在分页结果。清理不产生影子账本、双写或保留旧分页兼容层。
- 在现有 Vue 3 + Vite + Tailwind v4 应用中引入当前 shadcn-vue 分发约定及 Reka UI v2 原语，建立 `components.json`、共享 `cn` 工具、语义 CSS 变量和项目内可审计组件源；禁止运行时黑箱主题或平行旧组件库。
- 用 shadcn-vue 风格的 Button、Badge、Input、Select、Tooltip、Dialog、Sheet、DropdownMenu 和受控 Drawer 原语逐步替换工作台共享控件与 Node Ledger 交互外壳。保留既有路由、API 调用、访问性生命周期、375–1440px 视口、桌面 36px 台账行密度和移动端 44px 可触达区。
- 将深色主题冻结为 Void `#08090a`、panel `#0d0f12`、card `#121417` 的工业控制台层级；不得用视觉重构改变订阅、探测、生成、鉴权、导出、运行配置或部署行为。

## Capabilities

### New Capabilities

- `node-ledger-probe-identity`: 当前节点台账、探测摘要和前端状态展示之间的主键、状态归一化、收敛与并发安全行为。
- `shadcn-vue-workbench-foundation`: CSP 使用 shadcn-vue 分发结构、Reka UI 无样式原语、语义 token 与可访问工作台组件的边界。

### Modified Capabilities

- `node-probe-persistence-and-filtering`: 持久化探测结果必须按当前有效节点集合收敛，且 Node Ledger 以规范身份和明确状态语义进行筛选与显示。

## Impact

- Backend: 新的共享节点身份纯函数；`backend/app/services/proxy_chain_service.py`、`backend/app/services/probe/service.py`、订阅生命周期服务、相关路由 schema 和 targeted tests。数据库表结构不变；运行时清理是可重复的行删除，受当前有效节点集合严格限定。
- Frontend foundation: `frontend/package.json`、lockfile、Vite alias/config、`components.json`、`src/lib`、主题 tokens、共享 UI 原语及其测试。
- Frontend migration: Node Ledger 的域 helper、主视图、桌面/卡片/移动渲染、筛选器和详情抽屉；其它工作台覆盖层在共享原语稳定后按不重叠文件分批迁移。
- APIs: 两个 owned read projections 均增加身份字段；没有外部旧映射兼容层。所有调用者必须采用 `node_key`，并在同一发布中更新 CSP 自有前端。
- No deployment action is authorized by this plan. 依赖安装、代码实现、数据库清理触发和最终生产发布必须在实施阶段各自获得既有门禁与验证证据。

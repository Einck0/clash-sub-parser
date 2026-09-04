## Context

见 proposal.md。当前生产 SQLite 及 WAL 已有完整在线备份；Evidence Dossier 已确认其中含 2,080 个节点、5,609 条节点来源关联、8 条订阅、29 个策略组、470 条规则、362 条探测结果和 50 个快照。旧分支/标签是功能和回退基线，不是应被排除的历史包袱。

现有重构 WIP 已创建 `control-plane/` 和 PostgreSQL 容器，但其“空库、拒绝旧实现、拒绝兼容行为”的设计不能作为上线条件。本设计只定义未来实现边界，不授权覆盖或启动该 WIP。

## Goals / Non-Goals

**Goals:** 用可测试的分层架构承接完整旧能力；保留业务语义和真实历史数据；将大型页面和耦合服务拆成可维护模块；让每一阶段可由功能矩阵、迁移审计和回退演练验证。

**Non-Goals:** 不重做 SCRIPT；不在计划阶段改动运行系统；不以长期双写掩盖迁移问题；不在缺少功能等价证据时切换生产入口；不把数据库或协议细节暴露给浏览器。

## Decisions

### D1. 以行为基线驱动的渐进替换

实现以 legacy snapshot 的测试、路由行为和 UI 交互为验收来源，建立 `feature-parity` 矩阵：每项功能标注 legacy 证据、目标模块、自动化测试、迁移字段、切换门和回退点。默认轨道是将边界明确的新服务实现为目标模块，并用该矩阵逐项验收。若解析、sing-box 探测、配置编译或 NodeLedger 任一领域无法在计划阶段证明等价，则启用保底轨道：迁入对应旧核心模块及原有测试，先包裹在目标接口后再拆分。两轨共享同一领域契约，不能出现第二套长期业务逻辑。

选择渐进替换而非空白重写，是因为已验证功能含大量协议细节、外部服务分类和用户交互；选择有限迁入而非永久保留单体，是为了最终消除跨领域耦合。

### D2. 后端边界、领域模型与数据流

目标后端按以下层次组织：

`api/` 只做认证、请求校验、响应封套和 Job/下载流；`application/` 执行用例与事务边界；`domain/` 保存协议无关实体和规则；`repositories/` 隔离 SQL/ORM；`adapters/` 承载订阅 HTTP、协议 parser、sing-box、地理/媒体/AI provider、配置渲染器；`workers/` 消费刷新和探测 Job。API 不直接访问 ORM，渲染器和探针不直接写请求响应对象。

核心实体及责任：

- `Source`、`SourceRevision`、`Subscription`：URL/手工输入、刷新版本、启用状态、过滤规则、前缀、重命名、userinfo 和来源追溯
- `Node`、`NodeSourceLink`、`NodeSecret`：公开规范化字段、协议载荷指纹、来源关联与受保护连接材料；协议 adapter 覆盖 SS、SSR、VMess、VLESS、Trojan、Hysteria2、TUIC、WireGuard、HTTP/HTTPS/SOCKS5
- `ProbeJob`、`ProbeResult`、`ProbeProfile`：运行状态、取消、握手/延迟、出口 IP/国家/ASN/旗帜、媒体/AI 结果和峰值/平均测速
- `NodeGroup`、`GroupEntry`、`RuleCategory`、`Rule`、`ProxyChainBinding`：select/url-test/fallback/load-balance、动态 regex、静态节点/嵌套组/排除项、规则顺序和跳板链
- `ConfigurationRevision`、`ConfigSnapshot`、`DnsConfig`、`GenerateConfig`、`ReleaseToken`：可预检的发布输入、历史回滚、目标导出与快捷分发

数据流固定为：刷新输入或手工节点 -> parser/normalizer -> 来源修订与节点关联 -> 可取消 probe Job -> ProbeResult -> 策略解析/能力过滤 -> canonical config graph -> target renderer -> 发布文本、Scheme、二维码。每个箭头均有可测试的输入输出契约；探针请求必须经被测节点并显式禁用环境代理继承。

### D3. API 契约与过渡方式

现有 `/api` 资源按领域收束为 subscriptions、sources、nodes、probes、node-groups、proxy-chains、rule-categories、rules、dns、generate、settings、snapshots 和 downloads。每个资源保留旧客户端所依赖的字段与操作语义，新增 API 在同一领域 router 内以明确 DTO 演进，避免另一套 `/api/v2` 平行产品。

探测和刷新使用可查询、可取消的 Job 资源；列表查询支持原有 NodeLedger 的 keyword、source、protocol、probe、chain、speed、capability、排序和分页/窗口参数。配置预览/下载使用目标 `clash`、`mihomo`、`stash`、`shadowrocket`、`sing-box`，而不是让前端拼接配置。SCRIPT 路由和导出类型在第一阶段删除并以 404/不可选项明确终结。

### D4. Workbench 前端架构、视觉系统与状态边界

前端保持 Vue 3 + Vite，升级为“应用壳 + 领域切片 + 共享原语”的 Workbench，而不是把旧页面换一层样式。`AppShell` 由紧凑顶栏、主导航、命令区、路由工作区及可选上下文检查器组成：桌面宽度保留可见导航；窄屏将导航折叠为受控抽屉，数据表退化为卡片或可横滚的语义表格，抽屉/对话框全屏化。现有 `/`、`/nodes`、`/node-groups`、`/proxy-chains`、`/rules`、`/dns`、`/generate`、`/settings`、`/history` 路由和 Quick Export 入口维持，不建立第二套控制台。

样式使用 Tailwind v4 与 Vite 集成，但业务组件只消费语义化原语，不能散落任意颜色、尺寸或状态判断。Design Token 的基线为：画布 `#090D16`，面板阶梯 `#0F172A/#1E293B`，主色 `#3B82F6`，成功 `#10B981`，测速 `#06B6D4`，告警 `#F59E0B`，失败 `#EF4444`，主/次/弱文本 `#F8FAFC/#94A3B8/#64748B`，边框 `rgba(255,255,255,.08)`；数字、IP、端口、延迟和速度使用 JetBrains Mono 或 Fira Code。令牌必须同时提供暗色默认与等价浅色主题，状态色不能是唯一传达信息的途径。密集表格行高 36px、节点卡 110px、常规交互过渡 150ms ease-out；禁止未经过令牌的全局大面积玻璃效果，避免可读性和滚动性能退化。

共享 `ui/` 仅承载 Button、IconButton、Field、Combobox、Tabs、Switch、Badge、Status、Tooltip、Menu、Dialog/Drawer、Confirm、Toast、Empty/Loading 和数据表壳。Headless UI 负责 Dialog、Menu、Listbox/Combobox、Tabs、Switch 等焦点与键盘语义，Vue 组件负责 CSP 领域组合；图标统一来自 lucide-vue-next。任何对话框和抽屉均须有可感知标题、焦点圈、焦点陷阱、Escape/取消、关闭后焦点恢复和不可点击的背景层。可删除/清缓存/恢复等命令必须经统一 Confirm 原语，运行中命令在原始按钮和重复入口同时禁用。

按 `features/<domain>/` 组织 `api/`、`stores/`、`components/`、`composables/` 和 `views/`：领域包括 subscriptions、nodes、probes、node-groups、rules、proxy-chains、dns、generate、settings、history。`core/api` 统一认证、CSRF、响应错误和请求取消；领域 store 只保存远端缓存、查询和命令状态；表单草稿、未保存标记和校验状态属于编辑器组件/草稿 composable，不能被轮询或列表刷新覆盖。路由 query 保存可分享且可恢复的筛选、排序、视图与窗口锚点；本地偏好只保存主题、密度等非业务 UI 设置。组件不得复制协议解析、能力判定、策略展开、导出编译或敏感字段处理。

`NodeLedgerView` 由 `LedgerMetrics`、`LedgerFilterBar`、`LedgerToolbar`、`LedgerSelectionBar`、`LedgerCardGrid`、`LedgerCompactTable`、`LedgerVirtualWindow`、`NodeDetailDrawer`、`ProbeDialog` 与 `ProxyChainDialog` 组合。它们共享不可变 query、稳定的 `node_id` 选择集和当前窗口锚点，卡片/表格切换只改变渲染器而不清空任何一种状态；选择模型必须同时支持当前窗口全选/反选、显式 `node_id` 集合，以及“当前筛选的全部结果”。后者以 `query_snapshot_id + query_fingerprint + excluded_node_ids` 表达，工具栏显示实际匹配计数和排除数，并在批量探测前确认。创建 Job 时后端将该 snapshot 范围解析为不可变目标集合并持久化，之后的筛选变化、节点刷新和窗口切换不得改变正在运行的 Job；绝不用可变节点名称作身份。列表读取由服务端执行搜索、组合筛选、排序、统计及 facets，响应返回稳定 snapshot/cursor 和最小公开摘要；前端以 TanStack Virtual 的固定高度窗口渲染表格和卡片，overscan 为 10。筛选、排序或数据 snapshot 变化时取消旧请求、重置到首窗口；同一 snapshot 内视图切换以可见 node_id 为锚点恢复语义位置。这样既保留原“未显式选择即批量探测全部筛选结果”的操作语义，也避免一次取得节点、探测和跳板全量后在模板内重复筛选/排序。

实时探测/刷新呈现为可恢复 Job 状态而不是前端遍历节点：命令返回 Job 标识，store 以可取消的查询更新进度和已完成摘要，详情抽屉仅刷新受影响 node_id；失败、取消、部分完成和超时都有不同的可读状态及重试入口。QuickExport 继续是共享全局命令，保留单订阅/合并订阅、Clash、Mihomo、Stash、Shadowrocket、Sing-box、Scheme 与二维码；其 UI 仅消费服务端已编译的发布结果。

选择 Tailwind + Headless UI + TanStack Virtual，是为了分别解决可维护样式、可访问行为和大列表渲染，三者不承担领域状态。拒绝继续扩展 2461 行单文件与 scoped CSS，因为其查询、选择、探测和视觉职责已相互耦合；也拒绝把筛选/导出规则搬进 Pinia 或浏览器，以免与 canonical compiler 产生第二个真相来源。

### D5. SQLite 到目标库的有限、可核验迁移

迁移工具以 SQLite 在线备份生成的一致副本为只读输入，写入新的临时目标数据库/namespace，绝不挂载或写入生产旧卷。它分阶段导入：

1. 记录源备份 hash、SQLite `user_version`、行数和时间窗，创建迁移 run
2. 先导入全局设置、来源/修订、订阅及其 JSON 配置；保留原业务 UUID/逻辑标识并建立旧 ID 映射表
3. 导入规范化节点、原始/规范化 payload、指纹、节点来源链接、手工节点及订阅筛选/改名/跳板引用
4. 导入策略组、include_entries/regex、嵌套组、规则分类/排序/规则、DNS 和生成配置
5. 导入历史 ProbeResult、ConfigSnapshot、安全设置及 userinfo；不能解析的字段进入逐项隔离报告，禁止静默丢弃
6. 在事务内验证 referential integrity、数量、指纹和配置编译；产出 source/target count、hash、缺失/隔离清单

验收基线至少为 sources 4、subscriptions 8、nodes 2,080、node-source links 5,609、groups 29、rules 470、probe results 362、snapshots 50；实际运行以迁移时源快照审计数为准。导入失败回滚整个目标事务。正式切换前在副本演练，短暂冻结旧写入，完成最终增量/一致性校验；失败则旧入口和旧 SQLite 不变，丢弃或隔离新目标库后从快照重演。

### D6. 编译、探测与安全边界

唯一 canonical compiler 先将订阅候选、策略组动态成员、跳板绑定、规则和 DNS 解析为无目标偏差的图，再交给五个 renderer。regex 是虚拟成员，始终在编译/预览时动态展开；不得冻结成当时节点列表。编译器输出选择解释，以便 NodeLedger 与预览复用。

Probe runner 复用经验证的 sing-box 1.14+ 构造、异步端口池和优雅子进程生命周期，针对每个节点建立隔离本地代理。所有检测 HTTP client 使用 `trust_env=False` 并指定该代理；连接失败、服务挑战/429、地区限制和测速限制分别存储，不将宿主出口或服务失败误判为节点成功/失败。并发、时长、字节和周期由 ProbeProfile 限制，取消不启动下一节点。

节点连接材料和上游认证继续遵循最小披露：管理 API、日志、快照报告、二维码元信息和迁移审计不得回显；只有目标配置下载可包含被选节点所必需的材料。

## Risks / Trade-offs

- [全新实现遗漏协议字段或边缘交互] -> 以矩阵和 golden fixtures 对照；触发保底轨道迁入旧核心模块
- [SQLite JSON 历史字段无法映射] -> 先保留原始受保护载荷并生成隔离报告；禁止“默认值覆盖”
- [迁移期间数据变化] -> 一致性快照、最终冻结窗口、计数/hash 审计和旧入口回退
- [探测外部平台变化] -> 结果保存为服务级分类，provider adapter 可独立更新，不改节点存活语义
- [拆分 NodeLedger 产生状态漂移] -> 单筛选 query/选择集、端到端双视图和批量操作回归测试

## Migration Plan

先完成只读证据和 parity matrix，再实现目标领域并以 fixtures/旧测试对照。随后对生产 SQLite 的在线备份副本执行可重复迁移演练，记录完整审计。只有后端、前端、五目标 golden、真实经节点探测、迁移审计和回退演练全部通过后，才由用户确认生产冻结与入口切换。切换后保留旧容器、旧卷、legacy tags 和备份为只读回退资产；旧实例仅在稳定观察窗口及用户确认后退役。
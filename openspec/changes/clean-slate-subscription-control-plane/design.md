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

### D4. NodeLedger 与控制台组件树

前端保留 Vue 3 + Vite。领域 store 仅维护其查询、缓存和命令状态；表单草稿与远端查询状态分离。`NodeLedgerView` 组合 `LedgerMetrics`、`LedgerFilterBar`、`LedgerToolbar`、`LedgerCardGrid`、`LedgerCompactTable`、`LedgerPager/VirtualWindow`、`NodeDetailDrawer`、`ProbeDialog`、`ProxyChainDialog`。它们共同使用同一个筛选 query 与选择集，切换视图不得改变筛选或选择。

其余视图分别组织为 Subscription、NodeGroup、Rule/RuleCategory、ProxyChains、Dns、Generate/QuickExport、Settings、ConfigHistory。QuickExport 保留单订阅/合并订阅切换、五目标选择、客户端 Scheme 和二维码。组件只能调用 API client/领域 store，不能复制解析、筛选或编译规则。

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
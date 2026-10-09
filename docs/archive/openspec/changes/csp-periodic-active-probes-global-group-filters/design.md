## Context

参见 proposal.md。只读取证：`domain.Node.Active` 是激活定义；`NodeFilter.ActiveOnly` 可查激活节点，`NodeSourceRepository.ListByNode` 存真实来源；`ProbeRun`/`ProbeObservation` 和 `/probes/runs` 已持久化，runner 支持空 nodeIDs 取激活节点、基线默认种类、`DefaultRunBudget.MaxTasks=512`、现有安全出站与队列；`cmd/csp/main.go` 负责探测队列 Drain/DB Close；`resolver.Resolve` 先做激活/准入/风险，再按显式边生成组；`publication.resolveSnapshot` 与 policy 预览调用关系需施工核对并统一。`matchesAdmissionExpression` 使用宽松字符串/正则回退及无效字段按名称匹配，不适合未经严格校验直接复用于新配置。历史 `PROCESS-NAME` 修复在提交 `7289b4c`；本次错误无请求日志，无法断言其根因。项目主规格目录当前为空，故本 Change 建立两个新能力规格。

## Goals / Non-Goals

**Goals:** 可恢复周期探测、跨实例单到期互斥、低成本可观察证据、全局先于组级的确定性筛选、老配置保持兼容。

**Non-Goals:** 自动访问生产环境、改写历史手动运行的权限、把第三方目标站可达性作为本地测试门禁、推断未提供的 CSP 错误。

## Decisions

### 1. 冻结公共契约和计划归属

在 `internal/domain` 新增 `ProbeSchedule{Enabled,IntervalSeconds,Kinds,NextDueAt,Generation,UpdatedAt}`、`ProbeBatch{ID,WindowAt,Generation,Owner,LeaseUntil,State,RunIDs,Counts,RedactedError,CreatedAt,UpdatedAt}` 与专用 `ProbeScheduleRepository`（原子 Compare-and-Swap 领取、心跳续约、完成、列表）；将全局 `NodeFilterSpec` 作为独立单例设置，将组 `NodeFilterSpec` 作为按 group_id 绑定持久化；其结构为 `Conditions []FilterCondition{Field,Op,Value,ProbeKind,FreshnessSeconds}`，仅 AND，同一字段多值可重复 OR 不提供，NOT 用专有受限操作。`ProbeObservation` 添加产生时节点 credential_version 元数据，旧行标识版本未知；判断版本匹配不通过未知行。选 SQLite 增量表而非新的配置中心/另一套调度依赖；仓库已有 `modernc.org/sqlite` 与显式事务，单一 DB 可用受条件 UPDATE + 唯一 `(generation,window_at)` 协调进程。多独立 DB 的跨主机 exactly-once 不保证，部署边界为共享同一受控 SQLite 库。

### 2. 周期执行算法与安全

默认关闭；管理员配置的周期和种类经上/下界和 runner 预算验证（产品初始建议 15 分钟至 24 小时；如用户明确不同周期另调整）。`cmd/csp/main.go` 启动从 DB 加载配置，用 `time.Timer` 与 `context` 等待最近到期；不引入私有 cron 或第二队列。原子占位先推进 `next_due_at`（停机后采用 now+interval 跳过过去窗口，防止补发风暴），批次唯一；租约有过期恢复，续约与领用必须在同事务按 generation/owner fencing，旧 owner 丢租约后不得再写状态或创建运行。运行 actor_scope 固定 `system:periodic-probe`，幂等键源于 generation/window/chunk，使用现有 `ProbeRun`/`DefaultRunner`/`queue.Scheduler`，不经 HTTP 模拟手动调用；批次与运行关联在派发前持久化，每次从当前 `ActiveOnly` 完整分页或显式无分页查询取快照，逻辑 ID 去重排序，执行前再校验 active/credential_version 与解密是否可用，绝不在日志或 API 中打印密钥；设置运行期限与节点×种类任务预算，分片均衡以覆盖大库存，不允许静默截断。手动任务和周期任务共享现有有界 queue 公平机制；周期不得占满全部待执行容量，队列压力时让位/延后并记载未覆盖数。检测到无有效凭据时每节点记录脱敏跳过，禁止安全出站退化；计划关闭取消后续批次并根据 UI 的“取消本批次”显式取消运行。启动先恢复遗留 queued/running 运行与批次：截至 deadline/失租约终结为 expired/cancelled，禁止不完整运行数据无条件重放；尚未到期且可安全重放的片段只凭关联运行 ID 幂等恢复。停服顺序：停止 timer/续约与新派发 → 取消/等待周期运行 → 共用 Scheduler.Drain → DB Close；Drain 失败保持现有不关闭 DB 安全门禁。比较方案：单进程锁不足多实例，cron 调度器仍需分布式锁与恢复，故选择现有 timer+SQLite 原子条件抢占。

### 3. 新筛选结构、语义与安全

沿用 `resolver` 的排序/图展开和诊断设施，但**不复用**宽松 `matchesAdmissionExpression` 处理新字段；使用标准库 `strings`、`time` 和枚举类型作严格有界校验；不用 CEL/自定义脚本/正则解析器。`display_name` 包含/不包含大小写折叠，`protocol`、`source_subscription_ids` 使用枚举/精确成员关系；probe kind/verdict 取单条“最新有效”观测（`observed_at DESC,id DESC`），需对应当前节点 credential_version、且新鲜度有界，以接收本次解析的固定 `AsOf` 时间判断；质量 `latency_ms <= value` 仅对 `available` 且非负结果有效。缺失、失败、未知、过期或版本不符时，所有 probe 条件（含否定条件）为 false，拒绝未知绕过。显式来源仓库 `NodeSourceRepository` 必须按节点批量查询新增有界方法，禁止每节点 N+1；观测仓库追加批量 latest 方法和索引 `(node_logical_id,kind,observed_at DESC,id DESC)`；读取上限受输入节点/条件限制。全局配置由 admin GET/PUT 管理并持久化版本、审计；组级筛选随 `POST/PATCH/GET /policies/groups` 的可选 `node_filter` 读写，省略 PATCH 表示不修改，显式空对象清除。`NodeFilterSpec` 用 `null/[]` 表示恒真；不更改老 `AdmissionRule` 的优先级与行为。所有保存路径对字段、操作、值和总条件数校验并返回字段级错误。

### 4. 解析边、快照与发布

读取同一次解析所需节点、来源、观测、全局/组级筛选及 group/rules：事务一致读或稳定快照并证明无竞态，注入 `ResolveInput{AsOf,GlobalFilter,GroupFilters,NodeSources,LatestObservations}`；已有风险拒绝和准入先执行，全局过滤建立唯一 admitted 集，组在此基础上按自身条件筛选显式成员；只有设置非空组条件且没有节点边的组会从全局集合自动匹配，按 `DisplayName ASC,LogicalID ASC` 附加，组边排序保持原序；子组边保持拓扑，在父组展开时再应用父组条件，防止子组绕过。`AllNodeLogicalIDs` 与编译所用 `Members` 必须对应：若父组过滤会使直接 child-group 边暗含被拒绝节点，采用在解析快照中为该父组展开的受限子组成员/可编译投影，或拒绝该种无法无损表达的嵌套配置；不得只改变统计 ID 而保留可绕过的子组边。推荐对父/子存在不同筛选的场景直接拒绝并给出 `unrepresentable_nested_filter` 字段级诊断，直到真正实现可编译投影；规格要求有效嵌套筛选必须可以表达，故交付前必须完成投影方案，不得长期以拒绝替代。决策：解析器形成不可变的 per-group 展开投影，由 compiler 只引用投影中的节点；子组如果被多个父组引用且父过滤不同，生成稳定的父上下文派生子组名称/ID（摘要来源于父组 ID、子组 ID、过滤条件）并保证各目标渲染均支持，UI 同时保留原策略图引用。预览、预检、发布共用单一 `resolveSnapshot` 装载函数/输入摘要，涵盖筛选定义版本、来源集合、观测 ID/版本/时间及 AsOf 新鲜度区间；快照摘要不纳入无关的当前墙钟时间，但跨越 freshness 边界必须变化。过滤为空的老组仍保持已有编译行为；新筛选导致被路由引用组空集必须 fail-closed 且有目标组与过滤层诊断。

### 5. API/UI 与失败根因取证

探测计划：`GET/PUT /api/v1/probes/schedule`、`GET /api/v1/probes/batches`、`POST /api/v1/probes/batches/{id}/cancel`；全局筛选：`GET/PUT /api/v1/policies/global-node-filter`（在 router 的已认证 admin 范围内，实际公共路由前缀以 router 注册为准）；现有组 CRUD 追加可选 `node_filter`。UI 在探测页呈现启用/周期/种类/批次/状态，在策略页呈现全局/组级条件及预览数量、排除原因和空组阻断，不在浏览器实现另一套筛选语义。对于用户报告的“csp出错”：先只读复核当前编译目标能力与提交 `7289b4c`；需从用户获取本次 URL/时间、请求 ID、脱敏错误正文与版本/容器日志才可诊断新错误；不从线上 DB/密钥/日志取证。既有 `PROCESS-NAME` 未支持假说已由提交排除，手动探测不存在假说由 `http/probes.go` 排除，周期配置已存在假说由现有 `settings.go` 排除；其余当前错误根因未知，不将未知错误纳入伪修复任务。

## Risks / Trade-offs

- [SQLite 多实例租约无法保护不同数据库实例] → 明确共享库前提，CAS/唯一索引/fencing 与双连接竞争测试。
- [观测缺版本导致误筛] → 迁移旧观测为 unknown；新写入记录版本，缺失 fail-closed。
- [自动探测出站负载或队列饥饿] → 默认关、界限配置、切片预算、队列共用公平性、压测与取消。
- [跨父上下文派生子组使目标名称冲突/编译不一致] → compiler 全目标测试名称唯一及端到端结果；若不可实现须先向用户升级设计，不能降级跳过嵌套需求。
- [探测状态随时间变而快照 digest 陈旧] → 截止时间边界纳入 digest；预览/发布选取同一冻结输入再解析。
- [现有 Vite closeBundle 写 `internal/webassets/dist`] → 原仓不运行默认 `npm build`；只用 no-copy 隔离副本或事先可证不会拷贝的构建配置，并核对 dist 基线。

## Migration Plan

1. 本地隔离 DB 备份后应用新增 SQLite 顺序迁移：计划/批次/运行关联/全局组筛选存储及索引，旧 plan 默认 disabled，旧 filter 默认恒真，旧观测 credential_version=unknown；验证旧 DB 升级与全新 DB 同步。
2. 启动顺序先 migration 后装载计划；上线前复验隔离副本里的迁移、重启恢复及旧配置编译输出，生产部署必须另行授权并先备份数据库。
3. 回滚应用可停用周期计划和清空过滤条件恢复旧行为；若需要回滚二进制，则先排空计划并恢复迁移前备份（现有迁移器不支持向下回滚），禁止在旧二进制仍运行时擅自改写已迁移生产数据库。

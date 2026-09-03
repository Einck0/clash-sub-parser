## Context

本设计落实 proposal.md 所述的领域重构。当前运行形态是单容器 FastAPI + Vue + SQLite 默认 / PostgreSQL 可选；它仍可健康运行，且现有后端测试、前端类型检查和构建已在审计时通过。问题不在某一个端点，而在持久化与运行时边界：订阅、原始节点、手工节点、组匹配规则、探测结果和历史快照各自保存并各自解释；生产启动同时执行 ORM `create_all` 与运行时 DDL，而 Alembic 历史不完整，现场 SQLite 数据库也未记录 `alembic_version`。

约束：保留当前单容器部署和既有真实数据；不暴露订阅/节点凭据；现有 `/yaml`、`/script` 和管理 API 需要兼容窗口；当前未提交的探测功能属于必须保留并重新验证的输入，不作为已接受的架构事实。详细外部行为见本 change 的 specs。

## Goals / Non-Goals

**Goals:**

- 将节点身份、节点来源、探测结果、组解析和生成输出收敛到可追踪的版本化领域模型
- 为所有输出目标提供一个确定性编译入口和可解释诊断
- 将数据库演进、配置导入/恢复、部署升级做成可预检、可验证、可恢复的流程
- 让控制台按领域拆分状态，确保异步刷新和编辑不会相互覆盖
- 以可重放 fixture 与迁移/输出 parity 门禁取代“测试全绿即可发布”的假安全感

**Non-Goals:**

- 首个重构阶段不要求 PostgreSQL、Redis、消息队列或多容器 worker
- 不改变订阅协议语义、也不引入对外账号系统或云同步
- 不将历史探测结果无限保留；保留策略和容量上限在实现前由配置确定
- 不一次性删除旧 API；兼容适配层在 parity 验证后再按版本移除
- 不把 node payload 的协议细节重新实现为自定义代理内核，继续通过受控运行器验证协议出站

## Decisions

### D1. 把“节点”建成库存实体，而不是订阅 JSON 字段

建立 `Source`、`SourceRevision`、`Node`、`NodeSourceLink` 四层概念：Source 表示可刷新来源；SourceRevision 表示一次成功发布的不可变刷新代；Node 是稳定、可引用的规范化节点；NodeSourceLink 保存节点在某一代来源中的出现、原始显示信息和 payload 指纹。手工节点走逻辑 Source，但不依赖远端拉取。

稳定身份采用内部 UUID/ULID，匹配以规范化协议 payload 的安全指纹为基础；显示名称、行 ID、`name|type|server:port` 都只能是属性或辅助诊断。不同来源中 payload 相同的节点默认仍保留独立来源 link，编译时由明确去重策略决定是否归并，避免悄悄丢掉来源可追溯性。

备选方案是继续扩展 `subscriptions.raw_nodes/source_nodes/manual_nodes` JSON 列，并新增更多索引字段。拒绝原因是它无法表达刷新代、稳定引用、孤儿节点、探测历史和可移植配置引用，只会继续把业务规则压进 Python list/dict 操作。

### D2. 以逻辑 ID 和表达式 AST 保存配置图

组、规则、策略、DNS、生成设置及其引用使用稳定逻辑 ID；组成员保存为有序的 expression 列表（node selector、literal target、group reference、group-members expansion、exclude、policy predicate），而不是既保存 `include_entries` 又镜像到 `regex_rules/include_group_ids/...`。在写入时做结构验证，编译时进行循环检测和求值。

备选方案是保留当前 JSON 数组和数据库整数 ID，只补强 validator。拒绝原因是导入后的行 ID 不可移植、镜像字段天然可漂移，并且当前 `include_entries` 与 legacy 字段已经需要双向兼容分支。

### D3. 编译器是唯一真源，渲染器只是 target adapter

定义纯领域输入 `CompilationInput(configRevision, inventoryGeneration, policyRevision, target)` 和输出 `CompilationResult(graph, diagnostics, fingerprint, provenance)`。解析顺序固定为：选活动配置修订 → 装载库存代 → 解析表达式图 → 评估策略/探测资格 → 有序去重 → 目标适配。预览、Node Ledger、YAML、Script.js、短订阅均读取同一 result，只有最终 target serializer 不同。

为成本控制，结果按输入 fingerprint 缓存；任何配置、库存或策略版本变化自然失效。编译器不得在请求路径写库，也不得隐式触发刷新或探测。

备选方案是继续在各 generate/preview/export endpoint 调用共有 util。拒绝原因是“共有 util”仍允许每个调用方决定节点集合和开关，无法给出 parity 保证或解释一次输出到底基于哪批数据。

### D4. 探测用 observation + job，而不是可覆盖缓存

`ProbeProfile` 定义平台集合、超时、带宽上限与兼容性；`ProbeJob` 表示一次手动或计划批任务；`ProbeObservation` 是对单节点、单 profile 的不可变 terminal 结果。策略查询通过“最新且满足 profile/时效/status 的 observation”视图得出资格，保留 stale/missing/failed 的区别。

单节点运行器维持 loopback 隔离和 `trust_env=False` 的代理不继承原则。job orchestrator 负责全局 semaphore、每 job 限额、取消清理、端口池和结果落盘；provider probe 只返回分类结果。计划任务只创建 job，不直接在 scheduler 里遍历并写单个结果。

备选方案是保留 name-key upsert 表并向其加列。拒绝原因是名称可变/重复，无法回答“什么 profile、何时、什么输入得出的结论”，且上次运行会破坏历史与回归调试证据。

### D5. Alembic 成为唯一 schema authority，启动只检查不修复

将完整目标 schema 纳入 Alembic 链，并提供从现场无 `alembic_version` 的 bootstrap 数据库迁移的受控入口：`preflight` 只读扫描现有表、列和数据质量；`backup` 生成或验证恢复点；`upgrade` 执行分阶段迁移；`verify` 对行数、逻辑引用、密码保留和 fixture 输出做验证。应用 startup 只设置 SQLite 连接 pragma、检查 revision 和初始化必需运行资源，禁止 `create_all`、`ALTER TABLE` 或补列。

因 SQLite 不能在任意 DDL 上可靠回滚，shape-changing migration 使用 shadow tables / copy-validate-swap 策略，swap 前保持原表和备份；PostgreSQL 采用同一逻辑契约但可使用事务 DDL。任何无法映射数据写入 quarantine 表/恢复 bundle，不静默删除。

备选方案是补齐当前 bootstrap DDL 和追加 Alembic revision。拒绝原因是两个 schema authority 仍会竞争，未来镜像/多实例/降级会继续不可预测。

### D6. Bundle 与历史是逻辑配置修订，不是物理表 dump

`ConfigurationRevision` 保存 schema/version、逻辑配置 bundle、校验摘要、编译验证元数据和来源；导入先 decode → migrate bundle schema → validate references → compile verify，之后只在一个 transaction 内创建并激活修订。secret manager 只保存本机认证/来源私密字段，bundle 中用 source logical ID 与脱敏标签引用。

保留 revision 数量和 observation 留存量是配置化的；清理只能删除未被活动/回滚保护的对象。恢复是导入已验证 bundle，不是直接 truncate/reinsert 表。

备选方案是扩展当前 `ConfigSnapshot.snapshot_data` 与导出表清单。拒绝原因是两套 table maps 已经漂移（snapshot 未覆盖 security/probe 等），并且物理列序列化无法承诺跨 schema 版本可移植。

### D7. 控制台按 feature slice 重建，不引入全局万能 store

前端采用 feature folder：`features/subscriptions`、`inventory`、`groups`、`policies`、`generation`、`history`、`security`，每个拥有 API contract、query composable、mutation composable、view/component。共享层只包含 transport、auth/CSRF、typed error、toast/dialog、route guards 与 design tokens。

请求由 query key + AbortController/request sequence 管理；mutation 有 entity-level pending key，成功后精确失效依赖 query，失败不清空 draft。`NodeLedger.vue` 被拆为 inventory filter/table/detail、probe history、policy explanation 和 shared virtualized list。返回体通过 OpenAPI/contract generation 或由 Pydantic schema 导出的稳定类型源生成，禁止手抄任意对象继续漂移。

备选方案是把现有所有状态挪进一个 Pinia store。拒绝原因是单一 global store 只会把巨型页面的问题换成巨型 store，且无法隔离编辑草稿与远端 query。

### D8. 发布采用 feature parity 双轨与 kill switch

实施阶段保留 legacy read adapters 与新 compiler 并行，只在受控 feature flag 下对同一 fixture/只读配置进行 shadow compile；正常流量仍走 legacy，直到 semantic parity、API contract 和迁移验证全部通过。切换顺序是数据镜像可读 → 只读 shadow → 新 read path → 新 write path → 停止 legacy writes → 移除 adapter。

`/yaml`、`/script` 的 URL 语义、认证 query token 和 content-type 在兼容窗口保持；管理 API 用 v1 adapter 映射新领域 DTO，新增 revision/diagnostic 字段以可选字段出现。重大不兼容只在 v2 或弃用期后发生。

## Risks / Trade-offs

- [历史 SQLite 不具备 Alembic 版本且列组合未知] → 先执行只读 preflight，按照 schema fingerprint 选择升级路径；未知组合止步并导出红色报告，不在 startup 猜测修复
- [JSON 节点的 payload 规范化可能把语义不同的节点误归并] → 指纹格式版本化，保留原 payload 密封副本和 source link；默认只作为 identity 证据，不自动跨来源删除
- [双轨期间新旧输出不一致] → fixture + shadow compile + normalized semantic comparator；未批准差异阻断 read cutover
- [探测带宽/端口消耗影响宿主] → 全局 semaphore、job 预算、端口 lease、取消清理和每 profile 上限；不引入未受控的 parallel `gather`
- [配置 revision 增加存储成本] → bundle 去重、bounded retention、受引用保护的清理；先收集真实体积后定默认上限
- [前端重写影响用户正在编辑的表单] → feature-by-feature replacement，双路由或 feature flag，draft persistence 只在已定义语义的特性中启用
- [单容器 job 执行在多副本部署时竞争] → 第一阶段显式声明单 active scheduler；未来多副本前必须增加数据库 lease/队列，不假设内存锁跨进程有效

## Migration Plan

1. **冻结与基线**：记录当前 commit、未提交探测改动、容器健康、数据库 schema fingerprint；生成脱敏 backup manifest，不写业务数据
2. **合同与 fixtures**：从代表性订阅/组/规则/输出创建已脱敏 golden bundles，冻结 legacy normalized output 与 API contract，补齐 migration start-state fixtures
3. **Schema groundwork**：建立 Alembic baseline/bridge revision 和 schema preflight CLI；将现有 bootstrap DDL 的每一个字段变更映射到命名 migration，先在临时副本上演练
4. **Inventory read model**：加入新库存表及 legacy importer，在不改变线上输出的前提下填充 source revisions、nodes、links；跑计数、来源和 payload fingerprint 校验
5. **Compiler shadow mode**：实现配置表达式导入、纯 compiler、diagnostics 与 target adapters；对 fixtures 和受控现有配置执行 shadow parity，修复任何未批准差异
6. **Observation/job model**：双写或从现有 probe records 导入历史，接入 bounded job runner；先让策略解释只读显示，再切新 policy evaluation
7. **Bundle/history cutover**：实现 preflighted logical bundle、revision restore 和 secret preservation；验证导入/回滚以及数据库行 ID 改变的跨实例恢复
8. **Frontend slice replacement**：按 subscriptions → inventory/probe → groups/policies → generation/history 的顺序替换，每个 slice 完成 API contract、交互测试与无障碍检查后切换
9. **Production cutover**：维护窗口内备份→preflight→migration→verify→新 read flag→新 write flag；ready/parity/核心生成任一失败立即使用 backup 和旧镜像恢复
10. **Decommission**：在至少一个明确发布周期的兼容窗口和观测期后，停止 legacy writes、删除 adapter/运行时 DDL/重复 JSON 镜像；保留只读 migration audit 和恢复 bundle

## Open Questions

- 默认 observation 与 configuration revision 的保留数量和磁盘预算需要从真实库体积测量后确定；它不改变本设计的有界留存模型
- 现有真实配置中是否存在依赖“同名跨来源自动合并”的隐性习惯，需要在 golden fixtures 抽样后决定默认 dedup policy；在此之前默认保守保留来源区分
- PostgreSQL 目前是否存在生产实例需要与 SQLite 同一发布窗口迁移；若存在，只影响 migration runbook 的执行分支，不改变领域模型或契约

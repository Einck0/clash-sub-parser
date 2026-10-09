# Tasks: redesign-csp-database-dataflow

- **设计审查状态**: `Independent Reviewer PASS` (架构与整改方案已通过独立审查闭环)
- **交付终态**: `STATUS: PASS (Production Released)` / `Commit 2ffd20d deployed to production container 73cd5633a319 (18080/17000) with measured 2.296s downtime (script stop to healthz)`
- **内存约束验证**: `/tmp/verify_database_dataflow_design.py` 完成 23 项 DDL、外键、前置门禁、IP Risk 统一关联及防泄密约束验证
- **实施大包状态**: 任务 3.1 至 7.2 及 8.1 至 8.6 实施、自测与生产受控替换全部完成；任务 7.3 保持未勾选 (`[ ]`)，用户已授权实施上线，Pi 无法调用前台已登录 vault 安全渠道，生产带身份管理刷新/探针/预览发布未验；工程 Reviewer PASS 与 Critic PASSED 基于隔离 fixture 验证，不冒充生产管理验收

## 独立审查复审阻断与根因整改闭环说明 (Remediation Loop - Rev 4: 队列稳定性与多Run公平)

> **第二轮审查阻断证据 (Reviewer Evidence)**:
> 独立复审执行 `go test -race ./internal/probe/queue/...` 失败于 `fairness_test.go:242-298`（`bulk active peak 7 exceeded fair share 5` 或 2s 等待超时）。
>
> **根因分析与主脑裁定**:
> 1. **代码真实缺陷**: `scheduler.go` 原 `len(queues) <= 1` 仅依据等待队列数判定单 Run 闲置，忽略了正在执行中（`activeByRun > 0`）但队列暂空的 Run。当 Run A 队列刚排空但仍占槽执行时，新来的 Run B 会被误判为单 Run 闲置从而无限吞槽突破公平上限。
> 2. **测试时序设计缺陷与主脑不采纳理由**:
>    - 原测试在已启动的调度器上先循环提交 12 个 bulk 任务，后循环提交 other 任务。在 other 尚未提交的间隙，bulk 属于合法空闲单 Run，按 Work-Conserving 原则理应占满 10 个槽位；
>    - 网络请求在上下文中属非抢占式，新 Run 中途到来**绝不得回溯要求历史峰值 `peakBulk <= 5`，亦不得强行打断已在途请求**；
>    - **主脑严禁恢复旧半数限制**（闲置时强制限流至 5）伪装假过；**严禁使用 sleep 掩盖时序问题**。
>
> **本轮采纳与落地工程方案**:
> 1. **`activeCompetingRunsLocked` 综合判定**: 调度器以 `queued ∪ activeByRun` 集合计算实际竞争 Run 数，队列暂空但有活动任务的 Run 严格计入，彻底消除单 Run 误判；
> 2. **测试专用确定性同步 (Test-Only Synchronization)**: 不在生产暴露 `StartPaused` 或 `Pause/Resume` 状态机；测试在 `_test.go` 中直接使用内部锁与同步原语完成双 Run 原子入队，保持生产控制面零侵入；
> 3. **晚到收敛与非抢占时序断言**: 在 Late-Arrival 场景中，允许 bulk 先占满 10 槽（历史 peak = 10），断言在途任务完成释放槽位后，后续派发立即且优先流向 other，逐步收敛至公平份额，无饥饿；
> 4. **六大独立场景测试闭环**:
>    - `TestSchedulerIdleWorkConservingAndMultiRunFairness` (单 Run 满 10 槽 + 同时多 Run 限 5 槽)；
>    - `TestSchedulerLateArrivalConvergence` (10 槽在途晚到收敛)；
>    - `TestSchedulerQueuedEmptyButActiveCountsAsCompetingRun` (队列空但活跃算竞争 Run)；
>    - `TestSchedulerExplicitRunConcurrencyRespected` (显式配置上限严格遵守)；
>    - `TestSchedulerCancelAndCloseRecovery` (取消与关闭无死锁/无资源泄漏)；
> 5. **Race 证据与断言澄清**: 经核查无 Go 运行时 `WARNING: DATA RACE` 警告，原失败系断言时序设计不当与空闲判定缺陷导致的逻辑阻断。

---

## 1. 现状全量只读调查与数据资产核实 (Forensic Investigation - Complete)

- [x] 1.1 单数据库与 27 张表领域职责全量取证：核准 `/data/csp-v1.db` 为唯一主库，确证 1 张技术表与 26 张业务表的完整主键、核心外键、资产行数与领域读写所有者
- [x] 1.2 节点 LogicalID 算法双路径与刷新事实核对：提取 Base ID 与 Scoped ID 实际计算路径，核准 983 个失活节点为历史保护资产，排除 ID 漂移缺陷
- [x] 1.3 Dogegg 上游原始响应实机最小 GET 凭据对账：通过受控只读 GET 取得上游原始响应并脱敏保存，确证 `127.0.0.1:1` 流量与域名条目系上游面板公告伪节点
- [x] 1.4 旧 Run 失败根因与在线探测分布重核：澄清旧 Run 构建失败细节未持久化之事实，统计最新 31 活跃节点的可用与失败客观分布
- [x] 1.5 Publications 状态与 SQLite 连接并发排查：核实当前持久化发布工件数为 0，核准 SQLite 单物理连接无锁争用或并发冲突证据

## 2. 架构蓝图与重构方案设计 (Architecture & Blueprint Specification - Rev 3 Remediated)

- [x] 2.1 数据库 ER 目标设计与增量表 DDL 规范：制定包含 6 张新增事实与关联表（`subscription_payloads`, `subscription_entries`, `node_connection_versions`, `node_connection_heads`, `node_overrides`, `publication_payload_refs`）的标准 DDL 规范与 SQLite 单向外键验证，核实存量 27 表主外键准确性
- [x] 2.2 上游事实层与公告伪节点分类器设计：设计原始响应 BLOB 受控持久化规则；3 同参数公告独立写入不去重；无 `UNIQUE(sub, source_key)` 折叠；设计两级前置来源门禁公告分类算法；设计条目级用户人工纠偏与审计；澄清上游面板品牌未知
- [x] 2.3 节点连接版本化、权威 Heads 指针与 CoW 隔离设计：设计解耦逻辑 ID 与连接参数的不可变版本模型；设计 `node_connection_heads` 单向复合外键指针；消费端统一读取 Head 指针；设计用户字段级修改合并及多源更新写时复制隔离规则
- [x] 2.4 IP Risk Nullable 1:N、Safe Detail 白名单、Mihomo xhttp 官方代码对齐与发布两阶段设计：建立 IP Risk Nullable 1:N 统一测量流（严格仅关联 kind='ip_risk' 通用探针观测，应用层事务校验阻断非 ip_risk 类型）；为 `risk_policy_revisions` 添加 `rules_digest` 快照；建立 `safe_detail_json` 严格白名单防泄密机制；严格依据 Mihomo v1.19.32 官方代码渲染 `path`, `host`, `mode`, `headers`，未知 extra 隔离不盲传；设计 `publication_payload_refs` 外键防删、Preview Draft -> Publish Snapshot 两阶段生命周期与空组防护
- [x] 2.5 扩充 14 场景独立测试方案与实测维护回滚对账方案编写：编制包含 14 个具体场景的独立测试工程方案（无私造框架）；清除固定 30 秒与 Zero-Loss 绝对承诺，建立实测停写短窗演练与原资产对账回滚补偿规范；完成内存 SQLite 23 项核心断言自验证

## 3. 实施大包一：数据持久层与架构迁移 (Persistence & Migration - 实施已授权)

- [x] 3.1 编写并执行 Migration 000015：在保持既有 1014 节点、29 组、163 规则完全无损的前提下增量创建 6 张新表，对 `ip_risk_observations` 增量添加 `probe_observation_id`，对 `risk_policy_revisions` 添加 `rules_digest`，并事务性初始化存量节点连接版本与当前 head 指针 (实施已授权)
- [x] 3.2 移除 Repository 构造函数中的硬编码运行时 DDL：清理 `internal/repository/sqlite/probes.go` 中残存的动态 `CREATE INDEX` 与 `ALTER TABLE`，收敛至迁移框架 (实施已授权)
- [x] 3.3 实现新增数据表的 SQLite Repository 接口及单测：包括 Payload 压缩存取、Entry 查询、版本读写、Heads 指针更新、Payload 引用锁定及 Overrides 合并仓储方法 (实施已授权)

## 4. 实施大包二：上游摄入层与公告解耦 (Ingestion & Notice Classifier - 实施已授权)

- [x] 4.1 在 Inventory 抓取服务中集成 Payload 原始持久化：成功抓取落盘完整 BLOB 与关键响应头，抓取失败时保留错误流水并严格保全既有健康节点库存 (实施已授权)
- [x] 4.2 落地严格组合公告伪节点分类器与用户纠偏：在解析环节将组合全条件匹配项归类为 `notice`，同参数多公告全部独立记录不去重，保留于条目表供展示，不入库 `nodes` 且不参与拨测与发布；支持条目级纠偏重放 (实施已授权)

## 5. 实施大包三：版本化连接、Heads 指针与多源 CoW (Versioning, Heads & CoW - 实施已授权)

- [x] 5.1 实现节点连接版本演进与不可变版本派生：参数实质变动自增 `connection_revision`，原子更新 `node_connection_heads` 并使旧观测标记为 stale，纯名称更新仅修改元数据 (实施已授权)
- [x] 5.2 实现用户显式覆盖（Overrides）持久化与合并：支持用户在 Web 控制台修改参数并在后续抓取刷新中持续继承生效 (实施已授权)
- [x] 5.3 实现多源更新时的写时复制（CoW）隔离：当多订阅共享节点发生参数分化时，自动分支至 scoped ID，保持未变动订阅及现有策略拓扑引用完整 (实施已授权)

## 6. 实施大包四：IP Risk 统一、下游矩阵对齐与发布修复 (Downstream Matrix & Publication - 实施已授权)

- [x] 6.1 统一 IP Risk Nullable 1:N 测量关联与 Safe Detail 白名单：在 `probe_observations` 与 `ip_risk_observations` 写入时绑定同一 `probe_observation_id` 与 `measurement_id`，落盘 safe_detail 白名单，为策略版本添加 `rules_digest` 快照 (实施已授权)
- [x] 6.2 在 Mihomo 编译器与探针适配器中解封 xhttp：按官方代码规范渲染 `path`, `host`, `mode`, `headers` 为 `xhttp-opts`，未知 extra 安全隔离，使 VLESS xhttp 节点能够顺利导出与建立出站代理；对 sing-box 返回清晰的 `unsupported_target_capability` (实施已授权)
- [x] 6.3 实现发布两阶段快照生命周期与空组保护：Preview 创建不可变 Draft 并计算 Manifest，插入 `publication_payload_refs` 保护 Payload；Publish 严格激活指定快照（杜绝时间差库存漂移）；空组拒绝自动回退至 DIRECT (实施已授权)
- [x] 6.4 修复前端 `web/src/ui/ErrorStateCard.vue` 错误分类逻辑：优先依据 HTTP 状态码 422 与业务 code 进行能力不支持展示，杜绝误判为网络失联 (实施已授权)

## 7. 实施大包五：全量自测与生产受控上线 (Verification & Deployment - 实施已授权)

- [x] 7.1 本地全量单元测试与编译门禁：运行 `go build ./...`、全量 Go 单元测试与前端类型检查，确保退出码为 0 (实施已授权)
- [x] 7.2 生产受控单容器停机冷替换与可验证备份：执行 SQLite 在线一致性备份 (sha256:a5b25dfe...) 校验完整性与外键 (FK 0)，新镜像 (sha256:62ee52ebbfaf) 替换旧容器至 5aaefb45714c，实测停机命令至就绪耗时 1.941s，双端口验证 `/healthz` 与 `/readyz` (schema 15) 200 OK，存量资产与冷备布尔全等比对相等 (已闭环完成)
- [ ] 7.3 合法生产管理凭据真实节点实网验收：用户已授权实施上线，Pi无法调用前台已登录vault安全渠道；生产带身份管理刷新/探针/预览发布未验 (工程 Reviewer PASS 与 Critic PASSED 基于隔离 fixture 验证，不冒充生产管理验收)

## 8. 实施大包六：启用订阅可用库存收敛与多视图对齐 (Enabled Inventory Scoping - 施工完成)

- [x] 8.1 后端领域与仓储层支持启用订阅与统一底层谓词：在 `domain.NodeFilter` 中增加 `Scope: NodeScope`（`enabled_subscriptions`、`all_assets`）与 `SubscriptionID: string`，在 `domain.Subscription` 与 `domain.ProbePoolStatus` 扩展对账字段，实现 `buildNodeFilterPredicates` 统一谓词并更新 `List`、`ListReadModel` 与 `Count` 查询
- [x] 8.2 后端 HTTP 路由参数扩展与接口契约落地：在 `GET /api/v1/nodes` 增加 `scope`（默认 `enabled_subscriptions`，显式 `all_assets`，未知 400 `invalid_node_scope`），在 `GET /api/v1/subscriptions` 增加 `node_count`、`source_node_count` 与 `counts_scope`，在 `GET /api/v1/probes/pool` 增加 `inventory_total`、`candidate_total` 与 `scope` 并保持 `total_count` 含义，单节点详情与覆写接口保持全库权限
- [x] 8.3 探测调度、发布编译与消费者候选门禁：在 probe runner（selected IDs 与 run all）、periodic coordinator 与 publication resolve 中收敛至范围 E 活跃非公告候选，保持历史发布快照字节绝对不可变
- [x] 8.4 隔离环境全场景单元与集成自测：编写覆盖启用/停用、共享节点、失活节点、手工无源节点、公告排除、抓取失败保留旧成果、刷新删除成员、10+页分页与 risk/q/sub 过滤一致性的全套测试，确保 `go test -count=1 ./...` 与静态编译退出码为 0
- [x] 8.5 前端节点台账、首页概览与订阅卡片生效库存展示：更新 `web/src/features/nodes/useNodes.ts`、`NodesView.vue`、`DashboardView.vue` 与 `SubscriptionsView.vue`（由前端独立施工机完成，本包不修改 webassets 共享产物）
- [x] 8.6 生产受控单容器停机冷替换与生效库存发布自验：执行时点一致性 SQLite 备份快照 (sha256:65dd54ce...) 校验完整性与外键 (FK 0)，新镜像 (sha256:4925c70d...) 替换旧容器至 73cd5633a319，实测停机至首个 healthz 耗时 2.296s (/readyz 随后单独校验)，生产只读 SQL 对账生效库存 31、全量资产 1014、候选 31，静态业务数据对停写备份保持全等，隔离预览 PID 827371 释放


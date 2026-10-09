## Context

参见 `proposal.md`。规划基于 dev HEAD `23b8af8` 的只读证据，不是生产网络复验：992 节点中 21 启用，最近 21 条 baseline 为 error；18 条 `credentials_unavailable` 不能据此判线路坏（含 6 条不安全 TLS 拒绝、12 条有 UUID 的 VLESS），1 条私有目标拒绝，2 条 `transport_error` 原因未确权。`safe_dialer.go` 把安全拒绝和客户端构建错误包成缺凭据；`builder.go` 的 VLESS 构建路径缺 Flow，而 `compiler_export.go` 有 Flow。`inventory/service.go` 与 `probe/service.go` 在 baseline 缺失/未知时从其他能力推整体状态，也缺乏可靠的旧观测与当前连接参数关联。`periodic.go` 已有批量观测查询、512 预算、5 分钟失败冷却、CAS 租约/队列去重，但扫描默认 10 分钟，手动 `TriggerImmediate` 会更新 `next_due_at`；前端刷新按钮只做 GET。

## Goals / Non-Goals

**Goals:** 冻结共享健康语义和出站安全边界；消除探针自身配置/归因误报；分钟状态巡检只派发增量待测；手动刷新复用同一路径且不重置自动时钟；测试使用假时钟/离线网络夹具。

**Non-Goals:** 不自动把当前 21 条历史 error 改写成成功；不宣称 2 条 SS 已证实故障；不因测试诉求放开 `skip_cert_verify` / 内网出站；不每分钟全量测速；本规划不做生产改动或部署。

## Decisions

### 1. 分类在写观测处，健康在共享读模型处

复用 `domain.ProbeObservation`、`RedactedSummary` 与现有 API 可选字段；在安全拨号器添加有类型的缺配置/安全拒绝/不支持与客户端构建失败错误，保持 `errors.Is` 分类；runner 在出站前和 HTTP 执行阶段分别写稳定的脱敏 reason（安全拒绝、构建失败、DNS、超时、传输、profile 不匹配）。安全拒绝继续零出站；不要把 `clientFactory` 的原始错误或服务器地址/密钥拼进摘要。`VerdictError` 对不可检测仍可用于原始观测，但健康分类必须按 reason 区分，不靠 error verdict 推论网络坏。兼容历史 `credentials_unavailable` 等模糊原因：缺乏可证实传输失败细节时 fail-unknown；`transport_error` 若无足够阶段证据同样未知，后续新观测应提供可核验分类。VLESS 运行时从 `NodeConfig` 正确传入支持的 vision Flow 并对照现有 `compiler_export`，保留 TLS/Reality/SNI pinning；不引入独立代理引擎。

新增单一纯函数健康判定包/模块（domain 或已有 application 公共层中无循环依赖位置，由施工确认）接收 baseline 观测、新鲜度、当前配置版本返回 `healthy` / `degraded` / `unhealthy` / `unknown` 与延迟是否可信；`applyProbeObservationsToView` 与 `GetPoolStatus` 都调用它。以现有 `defaultProbeFreshnessTTL`（目前 1 小时）作为一致的 baseline 展示新鲜度上限；周期计划的 `interval_seconds` 是调度到期阈值，不替代读模型新鲜度。被旧配置版本覆盖的结果即使年龄新也不参与健康判断。能力观测继续逐类显示，但不能回填整体延迟/健康；`untested_count` 暂作为已存在的未知/无法检测桶，UI 标签必须解释“未知/未确认”而不误称全部从未测。已有 API 字段、分组与订阅过滤仍保持兼容；不无意更改订阅导出条件。

**备选**：仅改前端红绿文字，不解决聚合读模型、误报原因和旧观测可信度，拒绝。

### 2. 连接变更采用持久的单调版本，观测关联生成版本

已核对 `Node` 的 `UpdatedAt`、`ProbeObservation` 和 `UpsertBatch`：抓取/改名可刷新 updated_at，而观测无连接版本。现有 run `ConfigRevision` 是运行级自由字符串，不能替代逐节点版本。故在新 SQLite migration 增加 `nodes.connection_revision`（初始值）及 `probe_observations.connection_revision`（历史可空）。`UpsertBatch`/`UpdateNode` 原子比较 protocol、server、port、规范化 credentials JSON/transport，仅连接字段实变才递增；显示名、来源元数据、观测写入不递增。停用→激活须唤醒增量评估（可维护激活版本或在再激活时提升连接版本）；激活→停用从活跃候选中移除。SQLite 版本单调防止 A→B→A 的签名回退失效；比较/递增与节点更新同一事务，写入观测时关联实际执行所读取的节点版本；API/传输字段若有必要添加仅可选兼容字段，秘密配置值不得在状态接口泄露。旧观测 NULL 版本只作历史能力证据，新版本节点不能将它误判为新鲜健康，首次计划扫描为其安排增量复测；初次接入测试版本差异在持久化快照、重启、多实例下成立。调度按观测关联版本与当前节点版本比较，与有效期共同决定待测；旧版本即使时间新也必须在 UI 标注未知，并继续受冷却/配额限制。

**备选**：纯内存签名丢失于重启/多实例；`updated_at`/最新观测作为变化信号会使每次抓取或测量反复自旋；无版本时不能对旧结果做可信度标注，均拒绝。迁移仅新增字段并兼容历史只读数据，回滚到旧二进制前保留数据库备份，禁止覆盖或删除历史观测。

### 3. 一分钟定时器与增量候选分离；手动复用扫描器

在 `PeriodicCoordinator` 将 `defaultSweepInterval` 改为 1 分钟并同步 `Recover` 对遗留远期 `next_due_at` 的校正，保留 `IntervalSeconds`、`ListLatestByNodes` 和失败冷却逻辑。筛选规则：无/到期观测或连接版本不匹配；当前仍在池跳过，配额 512 **按实际 (node,kind) 任务数计算**，不因单节点多类别越限，最老/从未测优先且下一分钟继续；失败冷却与配置变化同时发生亦遵守保护窗口并后续重试。过期是否重测基于最新观测时间，不基于读取发生时间；每次轮询页面或写入观测不能触发额外批次。保持无待测批次的可查询状态及 CAS 租约；如果 `TriggerImmediate` 已有推进 next_due 副作用，将手动路径与自动窗口推进分离：手动只触发一次同样的有界增量扫描，绝不修改计划的下次到期时间，仍受跨实例协调与队列去重。接口优先复用已有 `POST` schedule trigger（不是 GET，更不是全量测速），响应保留池字段并提供兼容的可选入队/无待测反馈；需确保失败不会静默报成功。页面先刷新状态后发该 POST，跟踪批次/节点状态直至完成；原“全量测速”独立保留，空闲 GET 轮询不能偷偷发 POST。

**备选**：每分钟固定跑全部协议将压垮队列；把手动刷新设成 `TriggerRun` 会突破增量语义；手动复位 `next_due_at` 会令连续点击延后后台时钟，均拒绝。

### 4. DAG 与文件所有权

先冻结共享状态函数的签名、观测原因枚举、连接版本读写契约（特性包 A 的先决子阶段，非独立微派单）；A 实施完整探测编译/错误分类及版本记录（`internal/probe/singbox/**`、`internal/application/probe/{safe_dialer,runner}*`、`internal/domain/`、`internal/repository/sqlite/` 与迁移、对应离线测试）。A 完成后，B 整体健康视图/pool 统一（`internal/application/inventory/**`、`internal/application/probe/service.go` 的统计、共用状态规则，后端离线测试）；C 分钟扫描/手动后端（`internal/application/probe/periodic.go`、`service.go` 的 trigger、HTTP handler、配套测试）。B、C 都需要共享 `probe/service.go`，**必须串行**；D 在 C 后实施前端刷新/文字/交互与测试（`web/src/features/probes/**` 及有必要的节点台账/i18n）；B/C 自测完成后才做全量汇聚审查。不得以划分并行任务为由交叉改写共享文件；每个包包含实现 + 自测，不独立派微任务。

## Risks / Trade-offs

- [Risk] 老观测缺少版本，部分节点短时显示未知 → 保留证据与脱敏原因，增量扫描安排复测；绝不全染红/绿。
- [Risk] 1 分钟扫描与手动并发多实例争抢，或预算被同一节点多 kind 超限 → CAS/在池去重，精确任务预算、假时钟与并发夹具验收；确保手动不推进 next_due。
- [Risk] 更细分类泄露地址/凭据 → 错误枚举与固定脱敏模板，不输出原始 `clientFactory` 错误/credentials/私钥；安全拒绝零出站测。
- [Risk] 改迁移或版本写路径影响订阅导入与历史数据 → SQLite 临时库迁移、双入口 Upsert/Update、重启/再次导入和 UI 旧观测回归；迁移非破坏性，部署前另需授权和备份。

## Migration Plan

1. 在隔离 SQLite 测试库验证新增列迁移、旧数据 NULL 的未知语义、元数据更新不改变版本、真实连接变更版本递增、重启及双写路径；不在本规划运行生产数据库。
2. 实施 A→B→C→D 的完整特性与各自测试；门禁 `go build ./...`、针对性单测、`go test ./...`、前端类型检查和相关测试；独立 Reviewer 复验最终 diff 与契约。
3. 仅在另行获得预览授权与隔离 URL 后，由准备者隔离启动并提供 Critic 只读黑盒验收：一分钟扫描/增量与手动刷新、pool/台账口径、移动端交互；Critic 不承担构建/部署。上线、生产节点复测和最终真假故障结论均不属于本规划施工授权。

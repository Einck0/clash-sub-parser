## Context

见 `proposal.md` 的动机。实机证据表明工作树 `dev` 已将旧 Python、旧 Go、旧前端和既有 OpenSpec 工件标记为删除；仍存在停止状态的 `clash-sub-parser` 容器，且保留卷中的 `/var/lib/docker/volumes/clash-sub-parser_backend-data/_data/clash_sub_parser.db` 为 17,121,280 bytes。`DESIGN_REQUIREMENTS.md` 已将该库明确定位为仅供后续独立离线迁移的数据源，并要求新模型不继承旧表。

新运行时是 Linux Docker 单容器，已有目标端口为 18080 与 17000。代码与历史运行卷不能共享数据权威。Go `embed` 可将构建后的 Web 静态资源嵌入单一二进制。[1] `modernc.org/sqlite` 提供纯 Go SQLite 驱动，适合不引入 CGO 的单二进制部署。[2]

## Goals / Non-Goals

**Goals:**

- 用新 Go 模块、纯领域模型、新 schema 和新 REST 契约建立唯一 CSP 1.0 数据权威
- 以 sing-box 内存实例承载每个被测节点的实际流量路径；探测调度只管理有界任务，不持久化或复用代理运行时
- 让策略预览、导出和发布从同一不可变解析快照派生
- 建立不接触秘密、不原地改写历史库的离线、审查后激活导入流程
- 将前端限定为成熟 Vite + Vue 3 + Tailwind + shadcn-vue 组合；官方安装文档支持该 Vite 组合。[3]

**Non-Goals:**

- 不重新实现、调用或包装旧 Python/FastAPI、旧 Vue、旧 Go 包、旧 OpenSpec 或旧 API
- 不将旧 SQLite 原样挂载为新服务数据库，不做自动在线迁移、表适配、影子读写、双写或兼容 URL
- 不把订阅密钥、节点凭据、私钥、导出令牌、完整配置或探测原始响应放入前端、日志、审计摘要或 Git
- 不在本计划中实施、构建镜像、启动容器、写数据库、切换反代流量或删除历史卷

## Decisions

### D1 产品根、交付单元和模块边界

建立新根目录：`cmd/csp/` 是唯一进程入口；`internal/` 只能被本模块引用；`web/` 为独立 Vue 构建输入；`web/dist/` 仅为构建时中间物，不提交；`internal/webassets` 使用 `embed.FS` 提供 SPA。运行时健康端点为 `GET /healthz`，就绪端点为 `GET /readyz`，控制面为 `/api/v1/`，发布物为 `/publish/v1/{publication_id}`。

`internal/` 的所有权按以下方向单向依赖：

```text
transport/http ──> application ──> domain <── repository/sqlite
                         │             ↑
                         ├─────────────┤
                         ├── probe/singbox
                         ├── compiler
                         └── import/legacy (CLI-only)
```

`domain` 不导入 HTTP、SQLite、sing-box 或 Vue 类型。`application` 持有命令用例、事务边界、幂等和授权策略。`repository/sqlite` 是新 schema 唯一持久化实现。`probe/singbox` 只接受已规范化节点配置并在内存中构建执行环境，不得到数据库连接。所有入站取消必须进入 `context.Context`；Go `database/sql` 在 BeginTx context 取消时会回滚事务。[6]

替代方案：继续拼接旧双后端或把 sing-box 子进程作为探测器。前者违反 clean-slate，后者增加进程、端口、配置文件和资源回收边界；均拒绝。

### D2 新 SQLite 数据权威与命名冻结

新运行时数据库文件固定为 `/data/csp-v1.db`，Compose 命名卷固定为 `csp-v1-data`；**不得**挂载 `clash-sub-parser_backend-data` 为此路径。由应用内嵌、顺序编号的 schema migration 创建；每次启动在 ready 前校验 schema version、`PRAGMA foreign_keys=ON` 和只读健康查询。

冻结领域 ID 规则：实体公开 ID 一律使用 UUIDv7 字符串；节点用稳定 `logical_id`（由规范化、非秘密的传输标识 hash 编码）用于跨刷新关联；数据库行号仅内部使用，不能出现在 API、URL、导出或关系字段中。公开 API 使用 `snake_case` JSON；时间为 RFC 3339 UTC；枚举为小写 ASCII；资源 revision 使用不可变 UUIDv7 与 content SHA-256。

冻结实体与数据 ownership：

| 实体 | 不可变或可变 | 关键字段与约束 | 唯一 owner |
| --- | --- | --- | --- |
| `subscriptions` | 可变配置 | `id`, `name`, `source_url_secret_ref`, `enabled`, `refresh_policy`, `revision` | Subscription application service |
| `subscription_fetches` | 不可变审计 | `id`, `subscription_id`, `started_at`, `outcome`, `content_digest`, `redacted_error` | Fetch command |
| `nodes` | 当前规范化状态 | `logical_id`, `protocol`, `display_name`, `normalized_config_secret_ref`, `active` | Inventory reconciler |
| `node_sources` | 可变关联 | `node_logical_id`, `subscription_id`, `last_seen_fetch_id`，复合唯一 | Inventory reconciler |
| `probe_runs` | 状态机 | `id`, `idempotency_key`, `config_revision`, `state`, `deadline_at` | Probe command service |
| `probe_observations` | 不可变证据 | `id`, `probe_run_id`, `node_logical_id`, `kind`, `verdict`, `evidence_digest` | Probe result recorder |
| `node_groups` / `group_edges` | 可变策略图 | UUIDv7 `id`，有序边，不能自环 | Policy service |
| `admission_rules` / `policy_rules` | 版本化配置 | `id`, `revision_id`, `expression`, `position` | Policy service |
| `configuration_revisions` | 不可变 | `id`, `parent_id`, `content_digest`, `state` | Revision service |
| `publications` | 不可变后可撤销 | `id`, `target`, `snapshot_digest`, `compiler_version`, `state` | Publication service |
| `settings` | 可变小集合 | 公开安全默认值与限制，不存秘密明文 | Settings service |
| `audit_events` | 只追加 | `id`, `actor_kind`, `request_id`, `action`, `result`, `redacted_summary` | Audit writer |

秘密须经部署机上的 secret provider 或加密 keystore 引用保存，API 只显示掩码或是否已配置。SQLite 写入使用显式短事务；大型读取有上限和索引；外键检查使用 SQLite 的 `foreign_key_check` 能力。[7]

替代方案：将整份节点配置、策略树和 probe 原文塞进 JSON blob 或用旧表适配。两者破坏约束、可查询性和单一数据权威；拒绝。

### D3 订阅到节点的事务边界

刷新命令的步骤是：取订阅快照与 revision → 受 SSRF policy 约束地获取 → 解析和规范化于内存 → 在单个新库事务内写入 fetch、upsert nodes、更新 `node_sources`、收敛最后来源已消失节点 → 提交。网络获取和解析绝不持有数据库写事务。失败仅追加失败 fetch，不改变最后有效台账。

SSRF policy：仅 `http/https`；解析后逐地址拒绝 loopback、private、link-local、multicast、unspecified、reserved，代理 URL 同样验证；每次重定向重新验证；禁止继承宿主环境代理；只允许显式 secret reference 的 `fetch_proxy`。设置 `timeout`、`max_redirects`、`max_response_bytes` 和解压缩后大小上限。

### D4 探测管道、资源模型和失败语义

`ProbeRun` 是持久命令记录；内存队列按 run 实例化。调度器为每类探测维护固定大小滑动窗口，默认全局 16，合法范围 10–20，硬上限 32；另有每订阅和每节点的 semaphore。run 的 idempotency 唯一键为 `(actor_scope, idempotency_key)`，TTL 至少 24 小时。所有工作 goroutine 都由 run context 监管，并在 run terminal state 时完成回收。

单个 node task 构建临时 sing-box 实例或等价官方 in-memory pipeline；所有 TCP、TLS、HTTP、IP 和平台请求必须使用该 task 的 outbound。探测结果不能复用系统 `HTTP_PROXY` 或直接 `net/http` transport。步骤顺序：config validation → handshake/connectivity → controlled HTTP latency → exit-IP/geo/ASN → provider-specific capability tests → optional bounded throughput。吞吐量必须为 opt-in、有字节预算、deadline 和 per-run 总预算。

`available` 是正向、版本化且满足语义断言的证据；`restricted` 是明确受限证据；超时、challenge、登录页、验证码、DNS/网络错误、协议错误和服务契约漂移分别是 `unknown` 或 `error`。绝不把“HTTP 200”单独当作可用。每条 observation 留存脱敏 normalized summary 和内容 digest，不保留 cookie、令牌、完整 HTML 或完整 IP（必要时记录受保护的原始证据引用）。资源上限同时解决 API 未受控资源消耗风险。[4]

替代方案：每节点启动子进程、全局节点共享 outbounds、仅测试 TCP 或普通网页。前两者使隔离及回收不可验证，后者会产生假阳性；拒绝。

### D5 策略图、解析快照和多目标编译

策略输入是已激活 `configuration_revision`、指定 inventory watermark、admission rule revision 与固定 compiler version。Resolver 以深度优先显式栈处理 `group_edges`，维护 visit colors 识别循环，应用 include/exclude 和 MATCH 置底，生成排序稳定的 `ResolvedPolicySnapshot`。它包含节点 logical IDs、解析过的 group tree、rule list、DNS 设置、input digest 和 resolution digest。

仅 Resolver 产生 snapshot；预览、下载前校验、五个 renderers 与 publication 服务只消费 snapshot。每 renderer 必须声明能力矩阵，遇到不支持特征报位置化 diagnostics。禁止“尽力丢字段”或以别的格式替代。发布前新建不可变 publication；令牌只能读取绑定 `publication_id`，撤销后永远不重定向到别的发布物。

替代方案：每个 renderer 自行查询并解析策略。会造成同输入多种语义与 preview/export 分叉；拒绝。

### D6 历史数据处理与恢复

保留卷的历史 SQLite 是 archive source，不是新系统依赖。`csp legacy-import --source <absolute-readonly-path> --report <path>` 是单独 CLI，不由 HTTP 服务调用、不在容器常规 entrypoint 执行。执行流程：

1. 以 read-only 模式打开源，生成 schema fingerprint、源文件 digest 和只读备份或快照引用
2. 使用明确列 allowlist 读取历史逻辑内容，逐记录 validate、normalize、去秘密化、转换为新 schema 的 `draft` configuration revision
3. 将未知字段、无法映射或无效记录写入只含类型、hash 和原因的 quarantine report
4. 在新库短事务中写入 drafts、import audit event 和总数；任何目标写入失败时整个本批次回滚
5. 输出报告并要求授权管理者 review/activate；未激活内容禁止刷新、探测、发布

导入允许逻辑名称、无凭据协议属性、分组、规则与 DNS 配置；明确排除 subscription URL 密码与 query token、代理 username/password/UUID/private key、auth token/cookie、完整 configs、原始 probe body/header 及未知 blob。该模型满足 clean-slate：内容可移入，但运行时没有旧 schema contract。SQLite Online Backup API 支持把源一致复制到新文件，可作为操作人员在源可能恢复运行时的快照方式。[8]

### D7 前端 UX 契约

采用 Vue 3 + Vite + TypeScript + Tailwind + shadcn-vue。组件只走生成的 shadcn primitives，页面不再维护私有“仿 shadcn”组件库。页面边界：Overview、Subscriptions、Nodes、Probe Runs、Policy、Revisions & Publications、Settings、Audit。路由状态管理 URL query；远端状态由 query client 统一 cache/invalidate；绝不复制成第二份全局节点账本。

主题是 `light | dark | system` 三态，服务端渲染不存在时仍用首屏前 `data-theme` bootstrap 避免 flash；偏好只存 UI 主题，不存认证 token。Node page 只请求服务端分页，具有 loading、empty、error 和 retry 状态；详情在 desktop 用 drawer，mobile 用 bottom sheet。断点验收为 375、768、1440 CSS px；主控件可触达靶心至少 44×44，禁止横向 overflow；dialog/drawer 有 `aria-*`、focus trap、Escape 和回焦。

### D8 HTTP 命令/读取合同

所有管理列表返回：

```json
{"data":{"items":[],"page":1,"page_size":50,"total":0}}
```

`page >= 1`，默认 `page_size=50`，最大 `100`。状态变更走 POST/PATCH/DELETE，JSON 请求体以 schema 校验。创建 run、refresh、import 预检等可重试命令要求 `Idempotency-Key`；资源更新可选 `If-Match` revision，冲突为 409。HTTP 语义与条件请求遵循 RFC 9110 的 idempotence 与 opaque validators 框架。[5] 每个 response 带 request ID；日志和 audit event 共用它。

## 3+1 异构方案池及仲裁

| 方案 | 取材 | 优点 | 关键缺陷 | 裁决 |
| --- | --- | --- | --- | --- |
| A 原生 sing-box 运行时嵌入 | sing-box 配置模型与 Go runtime | 流量真经被测节点，协议覆盖广 | 有生命周期和内存压力 | 采用为 probe adapter，受 run context 与 budget 约束 |
| B 纯 Go SQLite 单二进制 | Go `embed`、modernc SQLite | 容器轻、无 CGO、部署面小 | 需严格 migration 和 SQLite 写竞争纪律 | 采用，独立新卷与短事务 |
| C 成熟 Vue Admin 构件 | shadcn-vue Vite 官方配置 | 可访问 primitive、主题与响应式有成熟基座 | 需避免组件再次私有分叉 | 采用，禁止手搓基础 UI |
| D 本地融合：不可变 snapshot + CLI-only import | 当前清空代码和保留历史库事实 | 完全 clean-slate 又可保留经审查的逻辑内容 | 需要显式审核成本 | 采用，拒绝线上 adapter / dual-write |

## Risks / Trade-offs

- [sing-box API 或上游探测契约变动] → 在 `ProbeProfile` 加 version 和契约 fixture；漂移返回 `unknown/error`，不自动判可用
- [SQLite 单写者吞吐与长事务] → 网络/编译均在事务外；写侧短事务、busy timeout、WAL 评估和并发写测试
- [历史字段无法无损映射] → “无损”定义为 allowlisted 非秘密逻辑内容的记录级可追溯导入，而非复制旧 schema 或秘密；未知项进 quarantine，不伪造映射
- [公网管理/SSRF/凭据泄露] → 默认认证、CSRF、出口令牌分离、URL 逐跳验证、脱敏 telemetry 和最小暴露端口
- [五目标格式语义不对等] → renderer capability matrix 和发布前 hard-fail，不静默降级
- [前端大型表性能与移动可用性冲突] → 服务端分页为强制边界；详情按需加载；以浏览器自动化在三种视口验收

## Migration Plan

1. 构建与单测在全新数据库 fixture 上完成，且确认新 Compose 使用 `csp-v1-data`，没有旧 volume mount
2. 构建镜像但不启动旧服务替代物；以空卷启动并验证 `/healthz`、`/readyz`、schema version、认证拒绝、旧路由 410、五目标 contract fixtures
3. 由操作人使用只读历史副本运行 `csp legacy-import --dry-run`，人工审阅 counts、quarantine 和 secret-exclusion report
4. 经授权以新卷运行实际离线导入；审阅 drafts 并显式 activate；以脱敏集成数据验证探测与发布
5. 人工批准后才修改反向代理或端口流量；切换前保存并验证新库 backup，确认旧容器依旧停止、旧卷未写
6. 发布后观察 readiness、错误率、队列深度与审计；不达标时切回旧停止容器和原卷，或从已验证新 backup 恢复。禁止借助双写、旧表 adapter 或自动回灌回滚

## Open Questions

无会改变范围、公开契约、数据处理或任务拆分的未决问题。历史表的实际字段仅影响离线 importer 的 mapping matrix，该任务明确要求先做只读 schema inventory 与 fixture；若发现某类数据无法落入 allowlist，必须 quarantine 而不是改变本设计。

## Sources

[1] https://pkg.go.dev/embed
[2] https://pkg.go.dev/modernc.org/sqlite
[3] https://www.shadcn-vue.com/docs/installation/vite
[4] https://owasp.org/API-Security/editions/2023/en/0xa4-unrestricted-resource-consumption/
[5] https://datatracker.ietf.org/doc/html/rfc9110
[6] https://go.dev/doc/database/execute-transactions
[7] https://sqlite.org/pragma.html
[8] https://www.sqlite.org/backup.html

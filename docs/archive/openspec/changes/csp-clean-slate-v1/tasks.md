# CSP 1.0 实施 DAG 与原子任务合同

本清单在 `design.md` 的冻结名称、数据 schema、HTTP 契约与文件 ownership 下执行。禁止执行者从旧源码、旧数据库模型、旧路由或旧 OpenSpec 复制兼容实现。除本 change 工件外，任何实现卡不得同时写 `go.mod`、`web/package.json`、`Dockerfile` 或 `docker-compose.yml`；这些 hot file 只由指定集成卡写入。

## 1. 基线与共享契约

- [x] 1.1 [Executor: Go Foundation] 创建 `go.mod`、`cmd/csp/`、`internal/domain/`、`internal/application/`、`internal/platform/` 的空白可编译新模块，并以 `go test ./...` 验证不恢复任何旧包路径；此卡独占 `go.mod` 与 `go.sum`
- [x] 1.2 [Executor: Domain Contract] 在 `internal/domain/` 定义 UUIDv7、`logical_id`、时间、枚举、领域错误、实体值对象与端口接口，并以 domain 单元测试验证 ID 不泄漏数据库 row ID；依赖 1.1，独占 `internal/domain/`
- [x] 1.3 [Executor: Schema Foundation] 在 `internal/repository/sqlite/` 与 `migrations/` 实现 CSP 1.0 新 schema、schema version、外键、索引、事务 helper 和空库 readiness 验证，并以临时 SQLite 测试验证 required tables、`foreign_key_check` 与 rollback；依赖 1.1、1.2，独占 `migrations/` 和 `internal/repository/sqlite/`
- [x] 1.4 [Executor: HTTP Foundation] 在 `internal/transport/http/` 实现 request ID、统一 `{data}` 与 `{code,message,request_id}` response、认证边界、CSRF、`/healthz`、`/readyz`、`/api/v1/` 路由骨架，以及旧 `/yaml`、`/script`、未版本化 `/api/` 的 410；以 HTTP contract tests 验证；依赖 1.2、1.3，独占 `internal/transport/http/`
- [ ] 1.5 [Reviewer: Foundation Gate] 独立复跑 `go test ./...`，审查模块依赖方向、无旧 import、错误脱敏、健康与 410 契约；对 `go.mod` 变更、schema 基线和 HTTP 根路径给出 APPROVED、REJECTED 或 REPLAN_REQUIRED；依赖 1.1–1.4

## 2. 订阅与规范化节点台账

- [x] 2.1 [Executor: Subscription CRUD] 实现 subscriptions 的版本化管理命令、授权、CSRF、secret reference 掩码和 audit events，并以 API integration tests 验证无认证拒绝、无 CSRF 拒绝与令牌不出响应；依赖 1.3、1.4，独占 `internal/application/subscription/` 和对应 HTTP handlers
- [x] 2.2 [Executor: Secure Fetch] 实现明确代理引用、URL scheme、每跳 DNS/IP SSRF 防护、timeout、redirect、compressed/decompressed body limits 与脱敏失败记录，并以本地 httptest fixtures 验证私网和重定向拒绝且不改变已有台账；依赖 2.1，独占 `internal/fetch/`
- [x] 2.3 [Executor: Parser and Identity] 实现 YAML、Base64 和 Shadowsocks、VMess、VLESS、Trojan、Hysteria2、WireGuard、TUIC 的规范化 parser 与不含秘密的稳定 `logical_id`，以测试夹具验证同传输身份跨名称和来源去重；依赖 1.2，独占 `internal/parser/`
- [x] 2.4 [Executor: Inventory Reconciler] 实现 fetch 结果的短事务 upsert、`node_sources` 关联、最后来源差集收敛和失败不覆盖规则，并以 repository integration tests 验证双来源合并、最后来源删除和失败保留；依赖 2.2、2.3，独占 `internal/application/inventory/`
- [x] 2.5 [Executor: Node Ledger API] 实现节点详情和带固定排序、过滤、`page/page_size/total` 的服务端分页 API，并以 10,000 节点 SQLite fixture 验证不全量传输、max page size 与稳定 totals；依赖 2.4，独占 `internal/transport/http/nodes.go`
- [ ] 2.6 [Reviewer: Inventory Gate] 独立验证 SSRF 重定向、秘密 redaction、逻辑 ID、刷新事务和分页性能合同；审查不能通过旧表或全量客户端分页绕过；依赖 2.1–2.5

## 3. 探测引擎与证据模型

- [ ] 3.1 [Executor: Probe State and Queue] 实现 `ProbeRun` 状态机、24 小时幂等键、取消、deadline、bounded sliding window、per-node TTL 排他和容量错误，并用 fake clock/worker tests 验证无重复执行和无 goroutine 泄漏；依赖 1.2、1.3，独占 `internal/probe/queue/` 与 `internal/application/probe/`
- [ ] 3.2 [Executor: Sing-box Probe Adapter] 在 `internal/probe/singbox/` 实现被测节点内存运行时 adapter，确保 task 的出口 IP、HTTP、TLS 与平台请求全经该 outbound；用可控 proxy fixture 验证本机环境代理不能替代被测节点；依赖 2.3、3.1，独占 `internal/probe/singbox/`
- [ ] 3.3 [Executor: Observation Profiles] 实现带版本的 baseline、geo、streaming、AI、测速 probe profile，定义 `available/restricted/unknown/error/stale` 判定、漂移与挑战页证据并脱敏持久化；用 fixtures 验证 HTTP 200、challenge、login、timeout 和 contract drift 不会成为 available；依赖 3.2，独占 `internal/probe/profiles/` 与 `internal/application/observation/`
- [ ] 3.4 [Executor: Probe API] 实现 run 创建、状态读取、取消与证据详情 API，以及作业审计；以 HTTP + repository tests 验证 Idempotency-Key、取消传播、token separation 与 pagination limits；依赖 3.1、3.3，独占 `internal/transport/http/probes.go`
- [ ] 3.5 [Reviewer: Probe Security Gate] 独立复跑 race-enabled probe tests，验证有界并发、实际被测节点路径、取消清理、错误分类和证据脱敏；发现上游 API 假设不成立时标 REPLAN_REQUIRED；依赖 3.1–3.4

## 4. 策略解析、五目标编译和发布

- [x] 4.1 [Executor: Policy Graph] 实现策略组、边、准入规则、config revision 和 validation API，检测自环、环、缺失引用和无效 MATCH 排序；以 property/unit tests 验证非法图不保存；依赖 1.2、1.3、1.4，独占 `internal/application/policy/` 与对应 handlers
- [x] 4.2 [Executor: Deterministic Resolver] 实现唯一 `ResolvedPolicySnapshot` 与 stable digest，供预览、diagnostics 和发布共同消费；以相同输入多次解析测试验证排序和 digest 完全一致；依赖 2.4、4.1，独占 `internal/resolver/`
- [x] 4.3 [Executor: Renderer Set] 在 `internal/compiler/` 分别实现 Clash、Mihomo、sing-box、Surge、Quantumult X renderer 和显式 capability matrix；为每个目标建立 golden fixtures，并验证不支持语义 hard-fail 而非丢弃；依赖 4.2，独占 `internal/compiler/`
- [x] 4.4 [Executor: Publication Boundary] 实现不可变 publications、目标绑定令牌、内容摘要、撤销和 `/publish/v1/{publication_id}` 读取合同；以 integration tests 验证 token 不能读管理 API、撤销不回退、预览/导出共用 snapshot；依赖 4.3、1.4，独占 `internal/application/publication/` 与 `internal/transport/http/publications.go`
- [ ] 4.5 [Reviewer: Compiler Gate] 独立生成五目标 goldens，审查 resolver 是唯一读取路径、能力不兼容不会静默降级、token 范围正确且撤销生效；依赖 4.1–4.4

## 5. 历史离线导入与运行安全

- [x] 5.1 [Executor: Read-only Legacy Inventory] 创建 `cmd/csp legacy-inspect`，只读打印历史 schema fingerprint、按表计数和已识别字段类别的脱敏报告；以复制的测试 SQLite fixture 验证源不变且不输出值；依赖 1.1、1.3，独占 `internal/import/legacy/inspect/`
- [x] 5.2 [Executor: Allowlist Importer] 创建 `cmd/csp legacy-import --dry-run` 与实际导入逻辑，使用显式字段 allowlist、secret exclusion、quarantine report、批次事务和 `draft` configuration revision；以 legacy fixture 验证目标可追溯导入、秘密永不落库、未知字段被隔离；依赖 5.1、2.3、4.1，独占 `internal/import/legacy/`
- [x] 5.3 [Executor: Draft Review and Activation] 实现导入 draft 的审阅、激活和 audit API；以 integration tests 验证未激活的导入不参与 scheduler、probe、resolver 或 publication；依赖 5.2、2.4、3.1、4.2，独占 `internal/application/revision/` 与对应 handlers
- [x] 5.4 [Executor: Ops Contract] 编写新 `docker-compose.example.yml`、Dockerfile、non-root runtime、独立 `csp-v1-data` volume、backup/restore runbook 和 smoke scripts；以 `docker compose config`、镜像 build、空卷 `/healthz` `/readyz` 和 no-legacy-volume-mount 检查验证；依赖 1.4、5.3，独占 `Dockerfile`、`docker-compose*.yml`、`docs/operations.md`
- [ ] 5.5 [Reviewer: Import and Operations Gate] 独立检查导入过程源库只读、quarantine 无秘密、新旧 volume 物理分离、备份 restore smoke 与未授权不切流；依赖 5.1–5.4

## 6. 管理工作台（对标并采用 Zashboard 体系）

- [ ] 6.1 [Executor: Web Foundation] 在 `web/` 基于 Zashboard（Zephyruso/zashboard）开源底座初始化 Vue 3 + Vite + TypeScript + Tailwind CSS + DaisyUI + Heroicons，建立响应式应用外壳、API client 与 Go embed 打包；以 `npm run build` 与 embed smoke test 验证；依赖 1.1、1.4，独占 `web/package.json`、`web/vite.config.ts`、`web/src/main.ts` 和 `internal/webassets/`
- [ ] 6.2 [Executor: Theme and DaisyUI Components] 采用 DaisyUI 官方主题体系（支持 light, dark, dim, cyberpunk 等自由切换）与成熟组件库，实现通知 Toast 与弹性布局，不搞死板尺寸断言；依赖 6.1，独占 `web/src/ui/` 和 `web/src/theme/`
- [x] 6.3 [Executor: Subscription and Node Views] 采用 Zashboard 风格的高性能节点卡片与 TanStack Virtual 虚拟滚动列表，实现订阅管理、节点台账、流媒体与 AI 状态回显，支持移动端流畅滚动；依赖 2.1、2.5、6.2，独占 `web/src/features/subscriptions/` 和 `web/src/features/nodes/`
- [x] 6.4 [Executor: Mobile Experience and Policy Views] 按照 Zashboard 移动端交互设计：卡片轻触生长展开动画（平滑 200ms ease-out 动效 + 40% 柔和毛玻璃遮罩）、底部抽屉操作，不搞死板几何限制；实现策略图编辑与导出配置预览；依赖 3.4、4.4、6.2，独占 `web/src/features/probes/`、`web/src/features/policy/`、`web/src/features/publications/`
- [x] 6.5 [Executor: Multi-Device Verification] 在移动端与桌面端视口验证流畅手感、无异常白屏、真实可用性，验证主题切换与路由持久化；依赖 6.3、6.4，独占 `web/e2e/`
- [ ] 6.6 [Critic: UX and Interaction Gate] 独立体验审查：对照 Zashboard 真实手感验证移动端流畅度、节点与探测回显、主题与操作顺手度，拒绝以死板教条阻碍顺滑体验；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 6.5

## 7. 集成、独立审查与受控发布准备

- [x] 7.1 [Executor: Integration Harness] 建立无秘密的端到端 fixture：订阅 refresh → inventory → probe fake provider → policy resolve → 五目标 compile → publication → UI；以 `go test ./...`、`go test -race ./...`、`npm ci && npm run build`、Playwright 和 `docker compose config` 形成可重复报告；依赖 2.6、3.5、4.5、5.5、6.5
- [ ] 7.2 [Reviewcommon: Final Engineering Gate] 从干净 checkout 独立执行 7.1 的所有命令，逐项对照六份 specs、检查旧路径/旧 volume/旧 API 无兼容复活、验证不泄密与回滚文档；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 7.1
- [x] 7.3 [Critic: Final Adversarial Gate] 对零订阅、节点大量消失、探测队列饱和、provider 契约漂移、启动 schema 失败、发布撤销、导入隔离和前端移动失败进行对抗复核；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 7.1
- [ ] 7.4 [Planner: Release Decision] 汇聚 7.2 APPROVED、7.3 PASSED、测试报告、backup restore 结果与 runbook，向 Einck 请求独立的新容器启动和流量切换授权；未经明确授权不得启动、切流、写入历史卷或删除旧容器/卷；依赖 7.2、7.3

## 调度约束

- 1.1、1.2、1.3、1.4 顺序执行，1.5 为第一硬门
- 2.x、3.x、4.x、5.1 可在 1.5 后按声明依赖并行；`go.mod`、`migrations/`、HTTP hot files 不得并发写
- 6.1 可在 1.4 后并行，6.3 与 6.4 依赖相应后端 API；web 基座 hot files 只由 6.1 写
- 5.4 在 5.3 完成后拥有 Dockerfile/Compose 唯一写权，任何其他卡不得碰这些文件
- 7.2 和 7.3 必须由互相独立的 reviewer/critic profile 在共享只读 workspace 执行；Planner 是唯一能建立执行 DAG 和发起外部汇报、发布决策的角色

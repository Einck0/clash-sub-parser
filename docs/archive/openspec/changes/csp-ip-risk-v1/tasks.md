## 1. 冻结契约与持久化基础

- [ ] 1.1 [Executor: Domain Contract] 在 `internal/domain/` 增加 `ExitIdentity`、`IPRiskObservation`、`RiskPolicy`、`RiskDecision`、风险枚举和验证规则，明确 score/confidence 取值、禁止字段与 `unknown` 默认语义；以 domain unit tests 验证完整 IP、原始 payload 和 secret 字段没有公开 JSON contract，且无效 band/trait/policy 被拒绝；依赖既有 domain foundation，独占新增 IP risk domain 文件
- [x] 1.2 [Executor: Repository Schema] 按现有迁移编号在 `migrations/` 创建 append-only IP risk observations、provider settings、risk policy revisions 和必要分页查询索引；扩展 `internal/domain/ports.go` 与 `internal/repository/sqlite/` 的专属 IP risk repository；以临时 SQLite migration test 验证外键、索引、回滚、append-only 约束和 `foreign_key_check`，且 schema 不出现旧表或完整 IP 列；依赖 1.1，独占新迁移与 IP risk repository 文件
- [ ] 1.3 [Executor: Security Contract Tests] 为 domain、repository 与 API-safe DTO 建立负向 redaction fixtures，覆盖 API key、cookie、URL query、完整 IPv4/IPv6、原始 JSON 和 error text；以 `go test ./internal/domain ./internal/repository/sqlite ...` 验证它们均不可持久化或出现在公开结果；依赖 1.1、1.2，独占 IP risk security test fixtures
- [ ] 1.4 [Reviewer: Data Boundary Gate] 独立审查 1.1–1.3，复跑相关 Go tests，确认第三方事实、节点 capability verdict 与 local risk decision 三层未混用，且 schema/API 无敏感字段；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 1.1–1.3

## 2. 被测节点出口与 provider 证据管道

- [ ] 2.1 [Executor: Exit Identity Profile] 在 `internal/probe/profiles/` 与相应 application orchestration 中新增版本化 exit identity/IP risk profile 入口，复用被测节点 runtime 而不改变 baseline、geo、streaming、AI、speed 的 verdict 语义；以 fixture 验证 `ip_risk` contract drift、challenge、超时和 exit identity 缺失只产生 `unknown` 或 `error`；依赖 1.1，独占 `internal/probe/profiles/` 的 IP risk additions
- [ ] 2.2 [Executor: Provider Port and Fake Adapter] 在专属 `internal/probe/iprisk/` 包定义 provider registry、versioned adapter port、响应 normalizer 和 deterministic fake provider，不写真实供应商密钥或网页 scraper；以 contract tests 验证缺 score、低 confidence、未知 trait、schema mismatch 与 provider conflict 不会被归为低风险；依赖 1.1、2.1，独占 `internal/probe/iprisk/`
- [ ] 2.3 [Executor: Outbound-bound Client] 将 IP risk provider call 注入 `internal/probe/singbox/` 已有 runtime HTTP client，保证所有 exit identity 与 provider 请求走同一 outbound 并 `Proxy:nil`；以可控 proxy fixture 和环境变量测试验证宿主 `HTTP_PROXY`、直连及另一节点出口不会替代被测节点；依赖 2.1、2.2，独占 `internal/probe/singbox/` 的 IP risk additions
- [ ] 2.4 [Executor: Budget, Cache and Observation Recorder] 实现 provider 的 explicit enablement、secret reference lookup、per-provider concurrency/rate/daily budget、deadline 和 `(logical_id, exit_identity_digest, provider, schema_version)` TTL cache；通过 observation application service 将仅摘要的结果 append 到库；以 fake clock tests 验证预算前拒绝、429、缺密钥、TTL、出口变更与取消清理，且网络请求不包在 SQLite transaction 内；依赖 1.2、2.2、2.3，独占 `internal/application/iprisk/`
- [ ] 2.5 [Executor: Probe Run Integration] 把已启用的 IP risk task 接入 bounded probe queue、run lifecycle、idempotency 与审计，不让它覆盖既有 ProbeObservation；以 integration tests 验证单节点同类型排他、run cancellation、provider 未启用不发网、证据与 node logical ID 关联；依赖 2.4，独占 `internal/application/probe/` 的 IP risk integration files
- [ ] 2.6 [Reviewer: Evidence and Egress Gate] 独立以 fake provider/proxy fixture 复跑 2.1–2.5，验证 outbound 实证、bounded resource use、cache key、cancel cleanup、provider drift 和脱敏；真实 provider 无授权时不得以网页抓取替代；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 2.1–2.5

## 3. 风险 policy、节点台账与发布准入

- [x] 3.1 [Executor: Risk Decision Service] 在 `internal/application/iprisk/` 或专属 policy risk 子包实现 versioned policy 的 single-provider、all-must-allow、mapped-highest-risk 融合和 `allow|review|block|unknown` 决策，固定默认 `unknown=review`；以 table/fake-clock tests 验证 score bands 不重叠、缺失/冲突数据不变 low-risk、输入相同则 digest 和 reason code 稳定；依赖 1.1、1.2、2.4，独占 risk decision service 文件
- [x] 3.2 [Executor: Policy Binding API] 扩展风险 policy revision 的管理、审阅、激活、停用及策略组绑定 API，要求认证、CSRF、Idempotency-Key 和脱敏 audit；以 HTTP contract tests 验证未激活 policy 不影响成员、无效 filter/policy 受拒绝、secret reference 不出响应；依赖 3.1，独占 `internal/transport/http/` 中新增 `ip_risk*.go` handler
- [x] 3.3 [Executor: Resolver Admission Integration] 在 `internal/resolver/` 建立唯一的 risk-aware resolved snapshot，加入 policy revision、evaluated_at、decision digest 和 admitted/excluded node diagnostics；以 deterministic/property tests 验证 block 排除、review 的显式行为、未绑定 group 原语义及相同输入下 snapshot digest 稳定；依赖 3.1、3.2，独占 `internal/resolver/`
- [x] 3.4 [Executor: Publication Preflight] 在 `internal/application/publication/` 与专属 publication HTTP handler 实施 snapshot 风险重算和 preflight diagnostic；以 integration tests 验证 block 或不可发布 review 返回冲突、不会创建 partial publication、不会替换既有 immutable publication，且既有 publication 不因后续观察改变；依赖 3.3，独占 `internal/application/publication/` 与对应新增 handler 文件
- [ ] 3.5 [Executor: Node Risk Read Model] 扩展 inventory/repository node read model，使列表和详情仅返回 `IPRiskSummary`，支持 risk decision、band、provider、status 和 policy revision 的服务端筛选再稳定排序分页；以 10,000 节点 SQLite fixture 验证 total、一页上限、filter SQL path 和 API response 不含完整 IP；依赖 1.2、3.1，独占 `internal/application/inventory/` 和 `internal/repository/sqlite/` 的 IP risk read-model files
- [ ] 3.6 [Reviewer: Admission and Publication Gate] 从空库及风险 fixture 独立复跑 3.1–3.5，检查 resolver 唯一路径、preflight 原子性、未知默认策略、SQL server-side filtering、audit 和敏感字段；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 3.1–3.5

## 4. 管理工作台与端到端门禁

- [ ] 4.1 [Executor: Risk Workbench Feature] 在未来 `web/src/features/` 的 nodes、policy 与 publications scope 实现风险摘要、unknown/stale/error 区分、可恢复 URL 筛选、高风险准入和 preflight diagnostics；只消费 API-safe summary；以前端 unit tests 验证 unknown 绝不显示低风险、筛选触发服务端 query、raw provider data 不进入 store；依赖 3.2、3.4、3.5，独占 `web/src/features/` 的 IP risk feature directories
- [ ] 4.2 [Executor: Cross-layer Harness] 增加无秘密 end-to-end fixture，走 fake outbound → fake provider → risk observation → policy decision → resolver → publication preflight → node API/UI；以 `go test ./...`、`go test -race ./...` 与前端 build/test 命令验证，报告明确标出真实商业 provider 未调用；依赖 2.6、3.6、4.1，独占专属 IP risk integration fixtures
- [ ] 4.3 [Reviewcommon: Final Engineering Gate] 在干净、只读 checkout 独立执行 4.2 的全部验证，逐项对照七份 delta specs、最小 diff、旧 CSP DAG 隔离、无 scraper/host-proxy fallback、无 secret/raw-IP leakage；输出 `verdict: APPROVED | REJECTED | REPLAN_REQUIRED`；依赖 4.2
- [ ] 4.4 [Critic: Risk Failure-mode Gate] 对 provider 403/429、无 key、配额耗尽、出口突变、节点大量消失、风险结果冲突、policy 默认 unknown、publication 回滚和移动端筛选进行对抗复核；输出 `CRITIC_VERDICT: PASSED | REJECTED`；依赖 4.2
- [ ] 4.5 [Planner: Controlled Enablement Decision] 汇聚 4.3 APPROVED、4.4 PASSED、测试报告、部署 secret reference 与预算配置审阅结果，请求 Einck 独立批准真实 provider 启用、production policy binding 或发布；未经明确授权不得发起真实 provider 查询、写入真实密钥、启动容器、切换流量或发布订阅；依赖 4.3、4.4

## 调度约束

- 1.x 为数据和安全硬门，2.x、3.x 只能在 1.4 APPROVED 后开始
- 2.1–2.3 可在声明依赖满足时并行；2.4 汇聚它们，2.6 为真实出口与证据硬门
- 3.1 与 3.5 可在 2.4 后并行，3.2 依赖 3.1，3.3/3.4 为严格顺序；3.6 收敛后端风险准入
- 4.1 只能消费已冻结 API-safe contract；4.3 与 4.4 由独立只读 profile 并行执行
- `migrations/`、`internal/domain/ports.go`、现有 HTTP hot files、`internal/resolver/`、`internal/application/publication/` 与 `web/package.json` 不得被并发卡片共同写入；Planner 必须按上述 ownership 串行登记

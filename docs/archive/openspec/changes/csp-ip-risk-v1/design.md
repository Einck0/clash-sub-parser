## Context

见 `proposal.md`。该仓库正在进行 Clean-Slate Phase 4–7；本 change 只定义新的独立能力，不重建或修改现有 change。工作树当前将旧 Python、前端和既有 Go 包标记为删除，并新增 Go 基座；因此任何设计不得依赖旧路径或旧 SQLite schema。`internal/domain/probe.go:60-70` 的 `ProbeObservation` 已是不可变、最小化的公共证据骨架，`internal/application/observation/service.go:52-105` 已按 SHA-256 digest 和 bounded redacted summary 持久化结果，适合作为风险证据的安全基线。

出口路径已有关键硬事实：`internal/probe/singbox/runtime.go:148-175` 经该 runtime 的 outbound 拨号；`internal/probe/singbox/runtime.go:178-208` 构造的 HTTP transport 以 `Proxy: nil` 禁止读取 `HTTP_PROXY`、`HTTPS_PROXY` 或 `ALL_PROXY`。风险查询必须复用该隔离边界，不能增加系统代理或网页抓取的旁路。`profiles.Profile` 当前只有 baseline、geo、streaming、AI、speed 五类版本化契约（`internal/probe/profiles/profiles.go:12-18,47-103,150-153`），新类型需要通过 domain/profiles 的显式扩展实现。

风险供应商提供的语义并非可以无损互换：Scamalytics 面向 IP fraud score、国家、operator、proxy 与 Tor 状态。[1] IPQS 将 >=75 描述为 suspicious/可能 proxy、VPN 或 Tor，而非必然欺诈，>=90 才为 high-risk。[2] MaxMind 说明 residential proxy 更难检测，并将其与其他 anonymizer trait 分开。[3] 因此本设计禁止将任意 provider 的原始分数直接作为通用的准入阈值，必须先保留来源版本、归一化语义与 policy revision。

## Goals / Non-Goals

**Goals:**

- 形成一条唯一、受限的证据流：被测节点 outbound → exit identity → 官方 provider API → 脱敏不可变 observation → versioned risk policy → resolver/publication/UI read model。
- 将第三方风险事实、CSP 本地风险决策、节点 capability verdict 三者物理分离，并让未知与契约漂移安全可见。
- 允许不同 provider 的可追溯并存，但以一个 policy 的明确选源、冲突和 unknown 规则得出确定性结果。
- 为节点管理筛选、受保护策略组和发布前阻断提供同一决策快照，而不将风险 state 回写为节点固有属性。

**Non-Goals:**

- 不默认启用任何商业 API，不购买配额，不在本 change 写入 provider 密钥或触发真实公网风险查询。
- 不爬取或解析 Scamalytics、IPQS、MaxMind 的网页；不以 HTML、浏览器会话或反爬规避作为数据路径。
- 不以风险分数推断节点真实性、地理位置、流媒体或 AI capability；不覆盖 `ProbeObservation` 的既有 verdict。
- 不建兼容 adapter、旧数据迁移、双写、全局风险 JSON blob 或客户端全量筛选。

## Decisions

### D1 三层数据边界与冻结数据合同

新增四种 domain 概念，并冻结它们的用途：

| 概念 | 责任 | 必须字段 | 禁止字段 |
| --- | --- | --- | --- |
| `ExitIdentity` | 一次 runtime 中被测节点的出口证明 | `node_logical_id`, `observed_at`, `identity_digest`, `country_code?`, `asn?` | 完整 IP 的持久字段 |
| `IPRiskObservation` | provider 返回的不可变、最小化事实 | `id`, `node_logical_id`, `exit_identity_digest`, `provider`, `provider_schema_version`, `observed_at`, `expires_at`, `status`, `score?`, `confidence?`, `network_class`, `anonymizer_traits`, `evidence_digest`, `redacted_summary` | 原始 JSON、完整 IP、API key、URL query、cookie |
| `RiskPolicy` | 本地业务决定 | `revision_id`, `provider_selection`, `max_observation_age`, `minimum_confidence`, `score_bands`, `trait_rules`, `unknown_action`, `conflict_action`, `review_action` | provider secret 或原始 payload |
| `RiskDecision` | 解析时派生 read model | `node_logical_id`, `policy_revision_id`, `decision`, `reason_code`, `observation_digest_set`, `evaluated_at` | 可逆 IP、密钥、原始响应 |

`score` 统一为可空 `0..100` 整数，含义由 `(provider, provider_schema_version)` 限定；没有有效 score 不得填 0。`confidence` 统一为可空 `0..100` 整数；provider 无该概念时为空而非伪造 100。`network_class` 为 `residential|datacenter|mobile|business|unknown`，`anonymizer_traits` 为受限的集合（如 `proxy|vpn|tor|residential_proxy|hosting|unknown`）。`status` 为 `available|unknown|error|stale`，刻意不同于 capability verdict 的语义域。

`EvidenceDigest = sha256(provider + schema_version + exit_identity_digest + canonical_normalized_response + observed_at)`。canonical response 只在内存中用于摘要计算后立即丢弃；持久化前应用同样的 `domain.RedactSensitiveInfo` 规则与长度上限。这继承现有 observation recorder 的安全边界，而不复用其类型字段。[inference] 当前 schema 尚无 IP risk 表，迁移名和 SQL 字段须由 executor 以现有 migration 编号实机核验后确定。

替代方案：把 risk 字段塞入 `domain.Node` 或 `ProbeObservation.RedactedSummary`。前者会把易过期的第三方判断误作节点本体状态；后者无法按 provider、TTL 与 policy 可查询地审计；均拒绝。

### D2 单节点通道、官方 API adapter 与 provider registry

引入 application port `IPRiskProvider`，其输入是受控的 `ExitIdentity` 和 request context，输出是内存中的 provider-specific normalized candidate。每个 adapter 被 registry 以 `provider` 和 `schema_version` 注册，只有 deployment settings 中 `enabled=true` 且 secret reference 可解析时可被调度。adapter 不拥有 HTTP client、secret store、SQLite connection 或 policy；probe orchestrator 注入由 `singbox.Runtime.HTTPClient` 取得的 client 和 request-scoped credential resolver。

执行固定序列：

```text
node config → sing-box Runtime → exit identity profile → exit_identity_digest
   → provider budget/admission → provider API via same Runtime HTTP client
   → normalize + contract validate → IPRiskObservation → repository append
```

每个 provider 有 `max_concurrency`、token bucket `requests_per_minute`、`daily_request_budget`、`per_request_timeout` 与最大 response bytes。网络 call 永远在 DB transaction 外；budget reservation、observation append 和 audit 使用短事务。429、预算耗尽、缺密钥、非版本化 response、deadline、DNS/network error 各产生不同 reason code，绝不回退到另一 provider、网页端或系统代理。

现有 runtime 的 `Proxy:nil` 和 custom `DialContext` 说明这种注入是可行的（`internal/probe/singbox/runtime.go:178-208`）。sing-box 的路由规则可按条件匹配流量，[5] 但本专项不增加额外 route-based fallback，避免同一 observation 的真实出口不可证明。

替代方案：直接 `net/http` 调 provider、使用 host proxy、或把 provider 调用放在前端。前两者违反出站证据；后者泄露密钥并不可审计；拒绝。

### D3 新鲜度、缓存和多源融合

风险缓存 key 固定为 `(node_logical_id, exit_identity_digest, provider, provider_schema_version)`。仅 `status=available`、`observed_at < expires_at`、provider 未被停用且 key 完全相同的 observation 可复用。对 exit identity 不能确认、出口 hash 改变或 TTL 到期时，风险输入为 `unknown` 并重新排队（受 budget 限制），不得重放旧风险结论。

每个 `RiskPolicy` 明确选择以下融合之一，禁止 implicit averaging：

- `single_provider`: 只使用声明 provider 的最新有效 observation
- `all_must_allow`: 每个 required provider 都必须给出 allow；任何 block 为 block，未知按 `unknown_action`
- `highest_risk`: 只在 policy 列出可比的 `(provider,schema_version)` 映射时，采用最高 band；否则 `unknown`

score bands 冻结为 `0..100` 的不交叠完整区间，结合 minimum confidence 和 trait predicate 决定 `allow|review|block`。`unknown_action` 默认为 `review`，可配置 `allow|review|block`；`conflict_action` 默认为 `review`。这避免把 IPQS 等 provider 的阈值误称为跨厂商硬真相。[2]

替代方案：把多 provider 分数求平均或缺失值当 0。会掩盖语义差异与缺数风险；拒绝。

### D4 Resolver 与 publication 的消费边界

`RiskDecisionService` 只接受 active inventory watermark、risk policy revision、固定 `evaluated_at` 和 observation repository，生成排序稳定的 decisions。`ResolvedPolicySnapshot` 增加 `risk_policy_revision?`、`risk_evaluated_at?`、`risk_decision_digest` 与 admitted/excluded node diagnostics；不会储存原始 risk data。现有设计要求 resolver 是 preview、下载前校验与 publication 的唯一输入（`csp-clean-slate-v1/design.md:88-94`），本 change 延续该单一路径。

- 未绑定 policy 的 group 维持原语义
- `block` 无条件从绑定 group 的 snapshot 排除
- `review` 是否隔离或排除由 policy 的 `review_action` 决定
- publication preflight 对将要发布的 snapshot 重新计算绑定 policy；若结果含不可发布的 `block` 或 `review`，返回 409-style diagnostic，且不创建 partial publication
- 后续 observation 在既有 immutable publication 后发生变化时不得改变其内容；只有下一次 preview/publication 使用新 decision snapshot

Envoy 的 RBAC 模型允许 per-route 覆盖，[4] 提供了“policy scope 显式绑定而非全局隐式污染”的参考；本地设计把该原则用于 group/publication scope，而不是引入 Envoy。

### D5 管理读取与 HTTP 契约

管理 API 仅暴露 `IPRiskSummary`：`decision`、`risk_band`、`provider`、`provider_schema_version`、`observed_at`、`expires_at`、`status`、`reason_code`、`policy_revision_id?`。`risk_band` 为 `low|medium|high|critical|unknown`，由 active policy 映射，不是 provider 原始 label。

节点列表新增可重复 query：`risk_decision`、`risk_band`、`risk_provider`、`risk_status`、`risk_policy_revision`。未知或不支持的 filter 返回 validation error；所有 filter 先在 SQL/app query 层完成再排序分页，保持 `internal/transport/http/nodes.go:33-98` 当前 server-side pagination 边界。节点详情可以包含 `ip_risk_summary` 和最近脱敏 observations page；不能回传完整 IP。

风险命令沿用 `Idempotency-Key`，必须要求管理授权和 CSRF。创建 provider settings、policy revision、risk run 与 publication preflight 都写 audit，但 audit 仅有 digest、reason code、policy revision 和 request ID。前端只能消费上述 API-safe summary，路由 query 可恢复筛选，禁止 localStorage token 或 raw response cache。

### D6 3+1 方案池和裁决

| 方案 | 来源与启发 | 价值 | 致命问题 | 裁决 |
| --- | --- | --- | --- | --- |
| A 供应商原始分数直通 | Scamalytics/IPQS/MaxMind 风险和匿名化输出[1][2][3] | 获得成熟 IP intelligence | 分数语义与阈值不可直接等同，可能泄露原始数据 | 仅保留为 versioned normalized evidence |
| B 网页探针或 scraper | 现成非官方开源抓取实现 [inference] | 无 API 接入表面成本低 | 403/反爬、契约漂移、条款与可用性不可保证，且难以安全审计 | 明确拒绝 |
| C 代理网关的 metadata/RBAC | Envoy 显式 policy scope 与 metadata 机制[4] | 强调策略绑定和解释性 | 引入 proxy sidecar 与跨系统控制面过度设计 | 借鉴 scoped policy，不引入 Envoy |
| D 本地融合 | CSP existing sing-box outbound + immutable observation + resolver snapshot | 能证明出口、隔离配额与把事实同决策分离 | 需要新增 schema/ports 和严格 provider 运营配置 | 采用 |

## Risks / Trade-offs

- [商业 provider API 费用、配额或凭据不可用] → 默认禁用、预留预算、secret reference、明确 `unknown` 与不可无界重试
- [出口频繁变化导致缓存失效与请求放大] → exit identity digest 绑定、TTL、单节点排他、provider rate/budget；超过预算转 `unknown`
- [provider schema 或风险语义变更] → 每个 adapter schema version + fixture；contract drift 不发布 allow
- [完整 IP 或原始 payload 泄露到库、日志、UI] → contract 中根除字段、digest-only evidence、redaction tests、API contract negative tests
- [高风险误判造成意外节点排除] → policy 显式启用、默认 unknown=review、preview/preflight diagnostics、未绑定 group 不受影响
- [低风险漏判导致订阅发布] → 可启用 policy 的 `unknown=block`；发布前强制 preflight；但该更严格模式由操作者显式决定
- [SQLite read query 影响大台账分页] → index/filter plan、限页、按 current observation 的物化或 join contract 由 executor 基准验证；不在浏览器过滤

## Migration Plan

1. 先在全新 SQLite fixture 增加 append-only risk evidence、provider settings/policy revision 和必要索引；验证 migration 不引用旧卷或旧表
2. 实现 fake provider 与 fake sing-box outbound fixture，验证环境代理不能替代 node outbound、API secret 不出任何层、TTL/cache/budget reason 可重放
3. 部署时保留所有 provider disabled；启动 readiness 只验证 schema 和配置结构，不因缺商业 key 失败
4. 授权操作者以 deployment secret reference 启用一个 provider、低预算与 `unknown=review` policy；以预览审阅结果后才把 policy 绑定到非生产 group
5. 经单独发布授权后再绑定 production group 或选用 `unknown=block`，并先运行 preflight
6. 回滚仅停用/解绑 risk policy 和 provider，不删 observation；既有 immutable publications 保持不变，重新发布前恢复既有未绑定 policy 的 snapshot

## Open Questions

- [inference] 首个真实 provider 的账号、API edition、区域可用性与合同字段尚未取证；这些影响 adapter 的具体 credential header、endpoint 与 mapping fixture，但不改变本 change 的 provider port、脱敏、预算或 policy 结构。实施前必须由 executor 依据已授权 provider 的官方 API 文档补充 provider-specific contract，不能从网页页面猜测。

## Sources

[1] https://scamalytics.com/ip
[2] https://www.ipqualityscore.com/documentation/proxy-detection-api/response-parameters
[3] https://support.maxmind.com/knowledge-base/articles/ip-anonymizer-risk-data-minfraud
[4] https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rbac_filter
[5] https://sing-box.sagernet.org/configuration/route/rule

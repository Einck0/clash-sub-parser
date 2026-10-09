## Context

参见 proposal.md。审查的五项不能一概归咎于编译器：`compiler/compiler.go:280-338` 与 `publication/service.go:880-955` 只检查 payload logicalID/protocol/version 与端点形状；`domain.Node`、`resolver.ResolvedNode` 不带 server/port。`domain/id.go` 的 ComputeNodeLogicalID 对 protocol/server/port/非秘密 transport 做归一哈希，排除 uuid/password/key 等敏感字段；`parser/parser.go` 用归一 transport；不能仅凭不完整 payload 反算或以订阅内容摘要（`inventory/service.go` 的 payload.Digest）证明可信端点。已排除“检查非空即安全”和“简单从凭据反算 logicalID 即足够”：凭据可能缺少参与 ID 的非秘密 transport，且版本/别名变化会误拒真实 URL。`http/nodes.go` 只有 GET，`inventory.NodeView` 不含端点；`nodeView.ts` 注入 `.edge.internal`/WG 地址、公钥、UUID，`useNodes.ts` 的 update 只改内存。`mihomo.go:974-1011` 在只有 provider 名称时生成 `ruleset.invalid`。`publication/service.go:955-1036` 全局 map 与前 100 条审计 RedactedSummary 不能提供可信、完整、可重启的绑定；现有 snapshot digest 等同于凭据版本绑定的假说亦被排除。`node_risk.go:560-645` 多 provider join 同时间戳可能倍增，`latestObservationsForNodes:792-835` 一页节点扫描全部历史。不要把现有其他协议能力断言为失败；其原勾选保留，最终回归仍待完成。

## Goals / Non-Goals

**Goals:** 确定安全绑定与编辑语义，重启可验证不可变发布，规则资源 fail closed，风险列表稳定；本地确定性验证与独立复验。

**Non-Goals:** 不部署、不触生产卷或密钥、不提供向未认证用户展示明文凭据的 API；不从旧审计日志猜测出版绑定或自动补历史信任。

## Decisions

1. **身份模型与端点来源（工作包 A，先冻结合同）。** `domain.NodeCredentialPayload` 保留现有 server/port/version，扩展独立的加密认证绑定字段（如 identityBinding：规范化 protocol/server/port、节点 logicalID、凭据 version、非秘密 transport 身份材料的版本化摘要；敏感 transport 不写明文索引）。在 `parser/extract.go` 与 `parser/parser.go` 生成同一 canonical identity，`inventory.Service.ReconcileSubscription` 在单事务内将来源计算的绑定写入受保护 vault AEAD 载荷/凭据记录，并与节点版本一致；若需独立索引，SQLite 只存带密钥 HMAC-SHA256 的版本化绑定而非可自称真实的 payload 摘要。读取时必须以可信的、不可由待验证 payload 单独构造的节点来源绑定/版本和 AEAD AAD 核对 payload 端点；`publication.resolveCredentials` 与 `compiler.validateCredentialEnvelope` 通过冻结的 typed verified credentials 输入共享这一核验，直接不可信 `WithCredentials` 调用不能绕过：要么验证可信节点绑定，要么 fail closed。若纯编译器输入原本支持未持久化快照，需显式携带由可信解析器计算且不可由攻击者同一调用伪造的身份来源；不可用则拒绝。保留原始 normalized transport 身份版本或经过认证的 canonical identity 材料，正确处理 URI/YAML 等价、敏感 transport 排除、大小写/默认值与旧记录无绑定 fail closed；不能从 payload 自证或把源码 fetch digest 当端点证明。`domain/id.go` 的现有逻辑 ID 算法不暗改；如确须身份算法版本化，明确旧版读取约束并在本地 fixtures 覆盖，绝不将所有 URL 节点误判为篡改。篡改测试需显式使用正常 logicalID + version + protocol 与恶意不同 server/port，证明鉴别有效。

2. **真实元信息与编辑（A 冻结 HTTP 合同，前端包 E 后行）。** 管理员鉴权的 `GET /api/v1/nodes/{logical_id}` 响应 `node.connection` 由 vault 中已认证记录投影：server/port、WG localAddress/publicKey/reserved/mtu/dns、TUIC uuid/alpn/sni/congestionControl/udpRelayMode 等可展示字段；WG 公钥、TUIC UUID 虽非私钥，仍限管理员详情；密码、WG privateKey/PSK、token、任意原始 transport 秘密不序列化，只有来自真实 payload 的 `hasPrivateKey/hasPreSharedKey/hasPassword` 布尔值。失败/缺绑定显示 unavailable，不做默认值或猜测。冻结 `PATCH /api/v1/nodes/{logical_id}/connection`，body 带 `expected_credential_version`、可更改非秘密字段及 write-only `private_key_input/pre_shared_key_input/password_input`；空值表示保留既有秘密，明确轮换动作才更改，绝不回显；输入校验/字段白名单、管理员认证、CSRF、无缓存与审计脱敏。logicalID 由端点和非秘密 transport 派生：此类变更不能在旧 ID 原地改造、须拒绝并要求修改源订阅后 Reconcile 生成新 ID；同 ID 仅允许不变身份字段的凭据轮换与安全元信息更新，CAS version + 节点/凭据事务，成功递增版本并使旧 probe/发布重建路径失效。若节点有来源订阅，后续 Reconcile 是源的权威值，覆盖本地轮换并重新 version（或冲突拒绝并审计）；不宣传持久覆盖，详情显示来源与下次同步影响。服务内存更新禁止被称为保存；失败保持旧 UI 状态。编辑接口的 token/CSRF 复用现有 chi router 中间件，不私造认证。

3. **发布持久绑定（B）。** 在 `domain.Publication` 与 `domain.PublicationRepository` 增加不可变 `content_digest`、`credential_binding_digest`、版本化绑定与工件元数据；SQLite 新 migration 仅对隔离临时 DB 运行验证，增加出版元数据/密文工件表或列。`Publish` 在同一事务原子写 publication、token hash、绑定、按 vault AES-GCM 加密的内容和 content type/filename；若现有 repository 不支持事务，扩展事务端口避免先写可用 token 后存工件失败。凭据绑定使用有分隔/规范编码（Go encoding/json 标准编码结构，禁字符串拼接歧义），覆盖排序 logicalID、version、protocol、可信端点/身份摘要及编译版本；内容 digest SHA-256 对实际输出字节，服务读密文解密并先校验 token/撤销/目标/绑定与字节摘要，再返回；不调用审核日志作为数据库，不依赖进程 map。可验证重编仅在持久化完整绑定、固定 compiler version 与相同内容 digest 全部成立时允许；默认密文不可变工件消除活订阅漂移。旧出版行缺摘要/工件不回填审计文本，安全拒绝且不自动改写历史数据；风险/认证门禁优先。迁移用 `schema_migrations` 的现有 runner、事务回滚/升级测试验证旧行和新行、重启读密钥错误与篡改拒绝；生产数据迁移/备份/回滚待另行授权。

4. **规则资源（C）。** `compiler/mihomo.go` 的 RULE-SET 必须引用调用方配置的合法 provider 或显式 HTTPS URL，经现有 URL parser 和 provider identifier 校验、定义与引用双向校验；单纯 `RULE-SET,name,GROUP` 无已定义 provider 时位置化 capability error，不注入 `.invalid` 或任何猜测 URL。网络抓取非编译时默认动作；本地 fixtures 使用受控 HTTPS/已声明 provider，不为绕过验证引入公网请求；官方 mihomo 配置检查与定向负例校验。

5. **风险单行最新观测（D）。** 在 `internal/repository/sqlite/node_risk.go` 用 SQLite `ROW_NUMBER() OVER (PARTITION BY node_logical_id ORDER BY observed_at DESC,id DESC)` 的白名单 provider 集（`EXISTS` 而非多对多 join），`rn=1` 才进入 `node_eval`，count 与 page 使用相同 CTE 和过滤/排序语义；`latestObservationsForNodes` 以分页 ID + 允许 provider 为约束的同一排名查询，仅返回每 ID 最新一条。针对 `ip_risk_observations(node_logical_id,observed_at DESC,id DESC)` 与 provider/schema 过滤建立符合 EXPLAIN QUERY PLAN 的索引，先分页节点再取观测，避免读全历史；多 provider 同时间戳/同 ID tie break、失效/未知、过滤 count/page 与万条历史隔离 fixture 对照。索引 migration 只在临时 DB 验证，不碰生产。

6. **DAG 与验收。** 先由 A 冻结身份和 GET/PATCH JSON、可信凭据 typed 接口，不允许 A/B 同时写 publication 或 compiler：A 写 domain identity、parser、resolver、inventory、HTTP nodes、A 专属测试；B 写 publication service、domain publication/repository port、SQLite publication repo/migration、B 专属测试；C 写 compiler/mihomo.go 及专属测试（若需改共享 compiler.go，先由 A 冻结接口并交接后串行）；D 写 sqlite/node_risk.go、风险专属 migration/测试（与 B 的 migration 版本号、共享 migrate.go 需预先协调，绝不并发写同文件）。E 仅在 A 的详情 API 合同冻结后写 `web/src/features/nodes/**`，不与 A 写同一路径。各施工包代码+定向自测+`go build ./...` exit 0；全分支汇聚再执行全量 Go race/count/build、Vitest/vue-tsc、隔离 web 构建、官方 CLI fixture、httptest 与独立 reviewer。reviewer/critic 阻断允许最多一次定点返工复验；若仍失败，回到本 Change 根因级整改，未真正过的任务维持未勾，不能假结单。

## Risks / Trade-offs

- [凭据自身自证身份、逻辑 ID transport 不完整] → 信任锚来自解析/节点身份版本，不由待验 payload 构造；双格式合法 URL 与同 ID 换端点负例同时锁定。
- [订阅刷新覆盖本地轮换] → API 明示来源权威与覆盖语义、记录审计；身份变化只能通过源 Reconcile。
- [发布工件含明文秘密] → vault AEAD 加密存储、独立密钥/权限、无日志、token 门禁与摘要校验；无密钥拒绝。
- [迁移共享版本或生产数据意外更动] → 迁移编号预留、只用本地临时 DB 演练；生产升级另需授权。
- [风险全局排名 CTE 仍扫描历史] → 分页先行、索引与 EXPLAIN/有界 fixture 作为验收，不以仅结果正确放行。

## Migration Plan

本次 Change 仅在临时数据库验证 schema 升级/回滚方案、历史行只读和缺绑定拒绝；不操作生产库。将来部署前另行审批备份、密钥可用性与灰度回退；旧记录不自动假恢复。

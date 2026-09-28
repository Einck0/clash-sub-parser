## Purpose

规范探针引擎各探测维度（`baseline`、`geo`、`streaming`、`ai`、`ip_risk`、`speed`）的真实性判定契约、完整负载传输计时、同一 `ProbeRun` 内同节点客户端的单次拨号生命周期安全复用，以及 SQLite `ListLatestByNodes` 在数据库内按 `(node_logical_id, kind)` 精确选取最新观测记录与离线基准验证要求。

## ADDED Requirements

### Requirement: Baseline connectivity probe requires strict HTTP 204 empty response
系统在执行 `baseline`（连通性与延迟）探测时，MUST 仅在收到 `HTTP 204 No Content` 且响应体为空（0 字节）时判定为 `available`（`reason=contract_matched`）。若响应包含明确访问限制信号（HTTP `401`、`403` 或挑战/登录拦截标记），系统 MUST 判定为 `restricted`（`reason=access_restricted`）；若响应为 `HTTP 200`（如 Captive Portal 伪装页、通用 `"OK"` 文本）或带非空响应体的 `HTTP 204`，系统 MUST 判定为 `unknown`（`reason=contract_drift`），绝不因状态码为 `200` 而误判节点连通可用。

#### Scenario: HTTP 204 空响应判定为 available
- **WHEN** `baseline` 探针请求 `http://cp.cloudflare.com/generate_204` 收到状态码 `204` 且响应体长度为 `0`
- **THEN** 评估结果 MUST 为 `verdict=available`、`reason=contract_matched`，并记录对应的 `latency_ms` 与 `evidence_digest`

#### Scenario: HTTP 200 或非空 204 响应判定为 unknown
- **WHEN** `baseline` 探针收到状态码 `200`（正文为 `"OK"`、空白或不含限制词的 HTML）或收到带非空正文的 `204`
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，且在两阶段探测中不得放行该节点进入第二阶段高开销探测

#### Scenario: 挑战或登录拦截响应判定为 restricted
- **WHEN** `baseline` 探针收到状态码 `401`/`403` 或包含挑战/登录标记（如 `cf-chl`、`turnstile`、`captcha`、`/login`）的响应体
- **THEN** 评估结果 MUST 为 `verdict=restricted`、`reason=access_restricted`

### Requirement: Geo probe validates exit IP and ISO country code
系统在执行 `geo`（落地地区识别）探测时，MUST 校验响应状态码为 `HTTP 200` 且响应体能够解析出合法出口 IP 地址（IPv4 或 IPv6）与 2 位 ISO 3166-1 alpha-2 国家代码后，才可判定为 `available`（`reason=contract_matched`）。

#### Scenario: 包含合法出口 IP 与两位国家代码的响应判定为 available
- **WHEN** `geo` 探针收到 `HTTP 200` 且响应体包含可解析的合法 IP（如 `"203.0.113.10"`）与 2 位字母国家代码（如 `"US"` 或 `"SG"`）
- **THEN** 评估结果 MUST 为 `verdict=available`、`reason=contract_matched`

#### Scenario: 缺失出口 IP 或国家代码的 200/204 响应判定为 unknown
- **WHEN** `geo` 探针收到 `HTTP 204` 空响应、`HTTP 200` 纯文本 `"OK"`、缺失 IP 字段或国家代码非 2 位字母的 JSON/Trace 响应
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁判定为 `available`

### Requirement: Streaming and AI probes never report available without a verifiable capability contract
系统在执行 `streaming` 与 `ai` 探测时，在未配置或未验证可证明的权威业务能力契约前，MUST 绝不因为目标站点根路径/首页返回 `HTTP 200`、`HTTP 204` 空响应或任意非空正文而判定为 `available`，且 MUST 严禁内置未经证实的 Netflix/OpenAI 私有探测规则或照搬 `subs-check-pro v3.2.1` 的误判逻辑。

#### Scenario: 明确限制信号判定为 restricted
- **WHEN** `streaming` 或 `ai` 探针收到状态码 `401`/`403`/`451` 或响应体包含挑战/登录/人机验证限制标记
- **THEN** 评估结果 MUST 为 `verdict=restricted`、`reason=access_restricted`

#### Scenario: 首页 200、空响应或普通非空正文在无业务契约证明时判定为 unknown
- **WHEN** `streaming` 或 `ai` 探针请求默认端点收到 `HTTP 200`（无论正文为空、`"OK"` 或普通网页正文）或 `HTTP 204` 且无明确限制标记、亦无已证实的业务能力契约证明
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁返回 `available`

#### Scenario: 传输或超时故障判定为 error
- **WHEN** `streaming` 或 `ai` 探针发生连接失败、DNS 解析错误或任务超时（`DeadlineExceeded`）
- **THEN** 评估结果 MUST 为 `verdict=error`、`reason=transport_error`

### Requirement: IP risk probe distinguishes exit identity from risk intelligence
系统在执行 `ip_risk` 探测时，MUST 严格区分“出口身份（Exit Identity）”与“IP 风险评分（IP Risk Intelligence）”：从 `cloudflare.com/cdn-cgi/trace` 等身份源提取到的出口 IP 与国家代码仅能证明出口身份存在（`ExitIdentityMissing = false`），绝不构成 IP 风险评分。在未获得真实 IP 风险情报源（`iprisk.Provider`）验证通过的风险评估结果时，系统 MUST 绝不将 `ip_risk` 判定为 `available`。

#### Scenario: 无法提取出口身份时判定为 missing_exit_identity
- **WHEN** `ip_risk` 探针收到 `200`/`204` 响应但无法从中解析出合法出口 IP 与 2 位国家代码
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=missing_exit_identity`

#### Scenario: 仅具备出口身份而无真实风险源结果时判定为 unknown
- **WHEN** `ip_risk` 探针从 `cdn-cgi/trace` 成功解析出出口 IP 与国家代码，但未由真实风险情报源产出有效风险评估契约结果
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁将出口身份直接判定为 `available`

### Requirement: Speed probe measures full payload transfer duration, rejects tiny stub responses, and enforces byte budgets
系统在执行显式 Opt-In 的 `speed`（带宽测速）探测时，MUST 在完整读取有界响应体（`readBoundedResponse`）完成后再截止计时计算 `latency_ms` 与 `throughput_kbps = (BytesRead * 8) / latency_ms`。`speed` 探针 MUST 仅在收到 `HTTP 200` 且有效读取字节数满足最小负载门槛（`BytesRead >= 1024` 字节）且未超出预算上限（`<= MaxBytesPerRequest` 且 `<= MaxBytesPerRun`）时判定为 `available`；对空响应、`HTTP 204` 或 `< 1024` 字节的极短伪响应 MUST 判定为 `unknown`（`reason=contract_drift`）；对超出请求或运行字节预算的响应 MUST 保留返回 `error`（`reason=speed_budget_exceeded`）。

#### Scenario: 完整读取有效测速负载后计算耗时与吞吐率
- **WHEN** `speed` 探针收到 `HTTP 200` 且响应体流式传输 `>= 1024` 字节（且 `<= MaxBytesPerRequest`），并在读取响应体过程中消耗可测时间
- **THEN** 记录的 `latency_ms` MUST 包含完整响应体读取耗时，`RedactedSummary` 中的 `bytes_read` 与 `throughput_kbps` 基于完整传输耗时计算，且评估结果为 `verdict=available`、`reason=contract_matched`

#### Scenario: 拒绝空响应、204 或极短伪响应
- **WHEN** `speed` 探针收到 `HTTP 204`、`HTTP 200` 空响应（`0` 字节）或极短伪响应（如 `"OK"` 等 `< 1024` 字节负载）
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁判定为 `available`

#### Scenario: 超出单次请求字节预算时返回 speed_budget_exceeded 错误
- **WHEN** `speed` 探针读取的响应体超过 `SpeedBudget.MaxBytesPerRequest`（1 MiB）
- **THEN** 评估结果 MUST 为 `verdict=error`、`reason=speed_budget_exceeded`

### Requirement: Same-run per-node probe client reuse and leak-free lifecycle cleanup
系统在单次 `ProbeRun` 执行期间，针对同一节点（`node.LogicalID`）跨多个探测维度（含单阶段并发提交与两阶段 Phase 1 `baseline` -> Phase 2 扩展维度）MUST 最多调用一次 `NodeDialer` 创建客户端实例并复用，且 MUST 在该节点于本次 Run 的全部已调度任务及其 `OnComplete` 回调完成后精确调用一次 `cleanup()` 释放资源。无论 Run 正常完成、Phase 1 淘汰、上下文取消、截止时间超时、任务提交失败或发生致命拨号错误，系统 MUST 保证所有已创建的节点客户端均被关闭且不发生泄漏，同时完整保持手动探测插队队首（`EnqueueManualPreemptFront`）与定时探测去重入池（`EnqueuePeriodicDedupe`）的队列语义。

#### Scenario: 同 Run 多探测维度单次创建并在全部回调完成后关闭客户端
- **WHEN** 单次 `ProbeRun` 对同一节点执行多个探测维度（如 `baseline` + `geo` + `streaming` + `ai` + `speed`，或不含 `baseline` 的多维组合）
- **THEN** 系统对该节点仅调用 `1` 次 `NodeDialer`，所有探测维度共享该客户端完成请求，且 `cleanup()` 在该节点最后一个任务的 `OnComplete` 回调完成后精确执行 `1` 次

#### Scenario: 两阶段探测中 baseline 未通过的节点不进入第二阶段且立即安全关闭客户端
- **WHEN** 两阶段 `ProbeRun` 中某节点的 Phase 1 `baseline` 结论非 `available`（如 `unknown`、`restricted` 或 `error`）
- **THEN** 系统跳过该节点的 Phase 2 维度提交，并在该节点 Phase 1 任务回调收敛后精确调用 `1` 次 `cleanup()`

#### Scenario: Run 取消、超时或异常退出时不泄漏节点客户端
- **WHEN** `ProbeRun` 在执行过程中遭遇 `context.Canceled`、`context.DeadlineExceeded`、队列提交错误或致命拨号错误
- **THEN** 系统等待在途任务与 `OnComplete` 回调收敛后，对本次 Run 中已创建的全部节点客户端各精确执行 `1` 次 `cleanup()`

#### Scenario: 保持队列手动插队与定时去重语义不变
- **WHEN** 手动触发探测或由 `system:periodic-probe` 触发定时探测
- **THEN** 任务仍分别以 `EnqueueManualPreemptFront` 与 `EnqueuePeriodicDedupe` 模式提交至调度器，手动插队优先与定时池内去重跳过行为保持不变

### Requirement: Security and signature invariants are preserved during probing
系统在收紧探针契约与实现客户端复用时，MUST 完整保留 `NewSafeNodeDialer` 的目标服务器 DNS 解析校验与 IP 固定（Server DNS Pinning）、SSRF 非公网 IP 拦截、TLS 安全校验、HTTP 重定向禁止（`http.ErrUseLastResponse`），以及 `domain.ComputeProbeEvidenceDigest` 的证据摘要签名计算规则与既有接口签名。

#### Scenario: Server DNS Pinning、SSRF 拦截、禁止重定向与证据签名保持生效
- **WHEN** 使用 `NewSafeNodeDialer` 与 `DefaultRunner` 执行节点探测
- **THEN** 域名节点仍预先解析校验公网 IP 并固定 `cfg.Server`（同时保留 TLS `SNI`），私网/环回地址仍被拒绝，HTTP `3xx` 响应仍返回 `http.ErrUseLastResponse` 不自动跟随，且生成的 `ProbeObservation.EvidenceDigest` 仍严格匹配 `ComputeProbeEvidenceDigest(runID, nodeLogicalID, profileVersion, verdict, statusCode, reason)`

### Requirement: SQLite latest probe observation query selects the exact latest record per node and kind inside the database
SQLite 观测仓储的 `ListLatestByNodes(ctx, nodeLogicalIDs, kinds)` 方法 MUST 在 SQLite 数据库内部按 `(node_logical_id, kind)` 分区并按 `observed_at DESC, id DESC` 排序，仅筛选并向 Go 层返回每个 `(node_logical_id, kind)` 的唯一最新观测记录（`rn = 1`），严禁将匹配节点的全部历史观测行拉取到 Go 内存后再去重。

#### Scenario: 多轮历史观测下仅在数据库内选出每个节点与维度的最新记录
- **WHEN** 数据库中多个节点在各个 `ProbeKind` 下分别存有多条不同 `observed_at` 的历史观测记录，调用 `ListLatestByNodes` 查询指定节点与维度集合
- **THEN** 数据库查询仅返回每个 `(node_logical_id, kind)` 排序最前的单条最新记录，返回结果集与全量历史行内存去重结果完全一致

#### Scenario: 相同 observed_at 秒级时间戳下按 id DESC 确定性决胜
- **WHEN** 同一 `(node_logical_id, kind)` 存在多条 `observed_at` 时间戳完全相同的观测记录
- **THEN** `ListLatestByNodes` MUST 按 `ORDER BY observed_at DESC, id DESC` 稳定返回 `id` 字典序最大的那条观测记录

#### Scenario: 离线基准与回归测试验证大历史数据下查询开销受控
- **WHEN** 在离线 SQLite 测试数据库中注入多节点、多维度、深历史观测数据并运行回归单测与 `go test -bench` 基准测试
- **THEN** 所有功能回归单测与基准测试全部离线通过（exit 0），且 `ListLatestByNodes` 扫描返回给 Go 层的行数恒等于有效 `(node_logical_id, kind)` 组合数而非历史总行数

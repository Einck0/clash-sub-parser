## Why

在完成探针准确性与执行效率专项研究后，代码取证确认当前探针执行器（`internal/application/probe/runner.go`、`internal/probe/profiles/profiles.go`）与 SQLite 观测仓储（`internal/repository/sqlite/probes.go`）存在四项影响探测真实性与运行效率的核心缺陷：（1）`runner.go` 对所有 `ProbeKind` 只要收到 `HTTP 200/204` 即无条件标记 `ContractMatched = true`，导致 `baseline` 误将 `HTTP 200` 伪装页判为可用、`geo` 不校验出口 IP 与国家代码、`streaming`/`ai` 仅凭首页 `200` 误报解锁、`ip_risk` 仅凭 Cloudflare `trace` 出口身份误标风险可用；（2）`speed` 在读取响应体 `readBoundedResponse` 之前就结束 `latency` 计时，却用完整响应体字节数计算 `throughput_kbps`，且未拒绝空响应或极短伪响应；（3）同一 `ProbeRun` 内每个 `(node, kind)` 任务各自独立调用 `r.dialer` 与 `cleanup`，导致同一节点在多维探测中重复创建和销毁内存 `sing-box` 实例；（4）`sqlite/probes.go:380-465` 的 `ListLatestByNodes` 从数据库拉取匹配节点的全部历史观测行到 Go 内存再去重。现需建立严格契约，一次性闭环探针判定真实性、同 Run 节点客户端复用与 SQLite 最新观测查询效率。

## What Changes

- **收紧各 `ProbeKind` 真实性评估契约，杜绝伪阳性误判（`internal/probe/profiles/profiles.go`、`internal/application/probe/runner.go`）**：
  - **`baseline` 严格 204 空响应契约**：仅当 `StatusCode == 204 (http.StatusNoContent)` 且响应体为空（`len(Body) == 0 && BytesRead == 0`）时才满足契约并判定为 `available`（`reason=contract_matched`）；遇到明确限制信号（HTTP `401`/`403` 或挑战/登录拦截标记）判定为 `restricted`（`reason=access_restricted`）；任何 `HTTP 200`（无论正文为空、`"OK"` 或未命中拦截词的网页）及非空 `204` 一律判定为 `unknown`（`reason=contract_drift`）；
  - **`geo` 出口 IP + 国家代码双重验证契约**：复用既有 `internal/probe/identity.ExtractCandidate` 解析响应负载，仅当 `StatusCode == 200` 且同时提取出合法 IP 地址（`net.ParseIP != nil`）与 2 位 ISO 3166-1 alpha-2 国家代码时才判定为 `available`；空响应、纯文本 `"OK"`、缺失 IP 或非法国家代码一律判定为 `unknown`（`reason=contract_drift`）；
  - **`streaming` 与 `ai` 保守真实性契约（严禁伪解锁与私有猜测规则）**：在未有可证明的权威业务能力契约时，**绝不判定为 `available`**；明确限制信号（HTTP `401`/`403`/`451` 或 `hasRestrictionMarker` 挑战/登录标记）判定为 `restricted`（`reason=access_restricted`），传输/DNS/超时故障判定为 `error`（`reason=transport_error`），其余响应（含首页 `HTTP 200`、`204` 空响应或普通非空响应）一律判定为 `unknown`（`reason=contract_drift`）；**严禁编写未经证实的 Netflix/OpenAI 私有规则，严禁照搬 `subs-check-pro v3.2.1` 的误判实现**；
  - **`ip_risk` 出口身份与风险评分边界分离契约**：明确出口身份（`cloudflare.com/cdn-cgi/trace` 解析出的 IP + 国家代码）仅用于证明出口身份存在（`ExitIdentityMissing = false`），绝不等于 IP 风险评分；当响应无法提取有效出口身份时标记 `ExitIdentityMissing = true` 并返回 `unknown`（`reason=missing_exit_identity`）；在未接入并验证真实风险情报源（`iprisk.Provider`）产出有效风险观测时，`ip_risk` 探测结果必须保持 `ContractMatched = false` 并返回 `unknown`（`reason=contract_drift`），绝不因 `trace` 返回 `200` 而误标为 `available`；
  - **`speed` 完整传输计时、最小有效负载门槛与超限预算保持契约**：将 `speed` 耗时计算移至 `readBoundedResponse` 完整读取响应体之后，确保 `latency_ms` 与 `throughput_kbps = (BytesRead * 8) / latency_ms` 真实反映完整负载传输耗时；要求 `StatusCode == 200` 且实际读取字节数达到最小有效测速负载门槛（`BytesRead >= MinValidSpeedBytes`，拒绝 `204`、空响应及 `< 1024B` 的极短伪响应，不满足时判为 `unknown` / `reason=contract_drift`）；完整保留超过 `MaxBytesPerRequest` / `MaxBytesPerRun` 时返回 `error`（`reason=speed_budget_exceeded`）的既有超限预算错误契约。
- **实现同 `ProbeRun` 同节点客户端生命周期单次创建与安全复用（`internal/application/probe/runner.go`）**：
  - 在单个 `ProbeRun` 内按 `node.LogicalID` 管理节点探测客户端生命周期：同一 Run 内同一节点无论执行单阶段多维任务还是两阶段（Phase 1 `baseline` -> Phase 2 扩展维度）任务，最多调用一次 `r.dialer` 创建 `*http.Client` 与底层内存 `sing-box` 实例；
  - 节点客户端的 `cleanup()` 必须且只能在该节点于当前 Run 的全部已调度任务及其 `OnComplete` 回调执行完毕后调用一次（`sync.Once`）；若节点在 Phase 1 `baseline` 未通过（不进入 Phase 2），在 Phase 1 同步点后立即释放；在 Run 取消（`context.Canceled`）、超时（`context.DeadlineExceeded`）、任务提交失败或致命拨号错误等所有退出路径上，等待在途任务回调收敛后确定性执行全部已创建客户端的 `cleanup()`，杜绝协程或套接字泄漏；
  - 保持既有队列手动插队优先（`queue.EnqueueManualPreemptFront`）与定时巡检去重入池（`queue.EnqueuePeriodicDedupe`）语义不变。
- **重构 SQLite `ListLatestByNodes` 为数据库内按 `(node_logical_id, kind)` 精确选最新并补充离线基准与回归测试（`internal/repository/sqlite/probes.go`、`internal/repository/sqlite/*_test.go`）**：
  - 在 `internal/repository/sqlite/probes.go` 中将 `ListLatestByNodes` 从“拉取全量历史观测并在 Go 内存去重”重构为 SQLite 数据库内使用窗口函数 `ROW_NUMBER() OVER (PARTITION BY node_logical_id, kind ORDER BY observed_at DESC, id DESC) AS rn` 且仅篩选 `WHERE rn = 1`（并确保 `(node_logical_id, kind, observed_at DESC, id DESC)` 复合索引就绪），使返回给 Go 层的行数严格受限于 `<= len(nodeLogicalIDs) × len(kinds)`；
  - 严格保证相同 `(node_logical_id, kind)` 在相同 `observed_at` 时间戳下按 `id DESC` 执行确定性决胜（tie-break）；
  - 新增离线基准测试（Benchmark）与多历史版本、同秒级时间戳 tie-break、分批（`> 100` 节点 chunking）及 `kinds` 过滤回归单元测试。
- **严守安全与接口签名不变量**：
  - 完整保留 `NewSafeNodeDialer` 的 Server DNS Pinning（域名前置解析校验并固定 IP）、SSRF 公网地址校验、HTTP 重定向禁止（`http.ErrUseLastResponse`）以及 `domain.ComputeProbeEvidenceDigest` 证据签名计算字段与公开接口签名。

## Capabilities

### New Capabilities
- `probe-truth-and-efficiency`: 探针各维度（`baseline`、`geo`、`streaming`、`ai`、`ip_risk`、`speed`）真实性判定契约、完整传输计时、同 `ProbeRun` 同节点客户端单次拨号生命周期复用，以及 SQLite `ListLatestByNodes` 数据库内精确选最新与离线基准测试规格。

### Modified Capabilities
无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `probe-truth-and-efficiency` 建立完整能力规格契约）。

## Impact

- **特性级正交写集合拆分（互不交叉）**：
  - **工作包 1（后端 Probe Runner 与 Profiles 契约及客户端复用）**：`internal/application/probe/**`、`internal/probe/profiles/**`
  - **工作包 2（SQLite 最新观测精确查询、复合索引保障与离线基准测试）**：`internal/repository/sqlite/**`
- **不触碰边界**：不修改外部 HTTP API 路由或 JSON 字段结构，不修改前端代码，不删除或弱化 DNS Pinning / SSRF / Redirect / 证据摘要签名字段，仅在本地离线测试夹具中验证，不提交、不部署、不触碰生产环境。

## Context

当前 `dev` 分支（HEAD `5d75644`）的探针执行器与 SQLite 观测仓储存在以下已确认源码级根因（背景动机详见 `proposal.md`）：

1. **`internal/application/probe/runner.go:676-698` 无差别状态码匹配导致全维度伪阳性**：
   - `executeTaskWithVerdict` 对所有 `ProbeKind` 均执行 `result.ContractMatched = resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent` 且 `result.ContractVersion = prof.Contract`；
   - `baseline`（目标 `http://cp.cloudflare.com/generate_204`）收到 `HTTP 200`（如 Captive Portal 页面或任意 `"OK"` 响应）也被标为 `available`，进而错误放行节点进入 Phase 2；
   - `geo`（目标 `https://api.ip.sb/geoip`）未解析响应体即把空响应或任意 `200/204` 标为 `available`，未使用项目已有的 `internal/probe/identity.ExtractCandidate` 校验出口 IP 与 2 位国家代码；
   - `streaming`（`https://www.netflix.com`）与 `ai`（`https://api.openai.com`）仅请求根路径首页，只要返回 `200/204` 且不含通用挑戰词即被判为 `available`（“已解锁”）；
   - `ip_risk`（`https://cloudflare.com/cdn-cgi/trace`）访问的仅是 Cloudflare 出口身份追踪端点而非风险评分源，`runner.go` 从未设置 `ExitIdentityMissing` 或调用真实风险源，却直接把 `200` 判为 `available`。
2. **`internal/application/probe/runner.go:650-693, 700-706` 带宽测速（`speed`）计时错位与伪负载放行**：
   - `latency = r.clock().Sub(reqStart).Milliseconds()` 在 `client.Do(req)` 返回响应头后立即计算，**早于** `readBoundedResponse(resp.Body, readLimit)` 读取 1 MiB 负载；随后却用 `readBoundedResponse` 读出的 `result.BytesRead` 除以响应头延迟计算 `throughput_kbps = (result.BytesRead * 8) / latency`，导致测速耗时遗漏主体传输时间、吞吐率严重虚高；
   - 同时 `speed` 对 `204`、空响应体或几字节的 `"OK"` 伪响应也判定 `ContractMatched = true`。
3. **`internal/application/probe/runner.go:630-633` 与 `internal/probe/singbox/runtime.go:108` 节点客户端重复创建及上下文生命周期陷阱**：
   - `executeTaskWithVerdict` 在每个 `(node, kind)` 任务内独立调用 `r.dialer(ctx, node)` 并 `defer cleanup()`；当单次 Run 对单节点探测 `K` 个维度时，重复执行 `K` 次 DNS 解析校验、配置构建与内存 `sing-box` 实例启停；
   - 特别注意：`singbox.NewWithBoxConfig(ctx, ...)`（`internal/probe/singbox/runtime.go:108`）会注册 `context.AfterFunc(ctx, func() { _ = rt.Close() })`。若复用客户端时将首个任务的短生命周期 `taskCtx`（单任务结束即 `taskCancel()`）传给 `r.dialer`，首个任务一结束就会触发 `rt.Close()` 导致同节点后续维度任务全部因 `ErrRuntimeClosed` 失败。
4. **`internal/repository/sqlite/probes.go:380-465` 最新观测查询全表历史拉取**：
   - `ListLatestByNodes` 执行 `SELECT ... FROM probe_observations WHERE node_logical_id IN (...) [AND kind IN (...)] ORDER BY observed_at DESC, id DESC;`，将匹配节点的**全部历史观测行**传输到 Go 进程内存再通过 `if _, exists := nodeObs[obs.Kind]; !exists` 丢弃旧行。

### 基线可测指标（Baseline vs. Target Metrics）

| 指标维度 | 当前基线（HEAD `5d75644`） | 目标契约指标 | 验证方式 |
|---|---|---|---|
| `baseline` 收到 `HTTP 200` 或非空 `204` 的判定 | `available`（伪阳性） | `unknown` (`reason=contract_drift`)，仅 `204` + `0` 字节正文为 `available` | `profiles_test.go` & `runner_test.go` 表驱动单测 |
| `geo` 收到无合法 IP/2位国家代码的 `200/204` | `available`（伪阳性） | `unknown` (`reason=contract_drift`)，仅 `200` + 合法 IP + 2位 ISO 国家码为 `available` | `profiles_test.go` & `runner_test.go` 单测 |
| `streaming` / `ai` 收到首页 `200` / 空响应 / 普通非空正文 | `available`（伪解锁） | 无明确限制信号时恒为 `unknown` (`reason=contract_drift`)；有限制信号时为 `restricted` | `profiles_test.go` & `runner_test.go` 单测 |
| `ip_risk` 仅收到 `cdn-cgi/trace` 或无真实风险源 | `available`（伪风险通过） | 无出口身份时 `unknown` (`missing_exit_identity`)；仅有出口身份无真实风险源时 `unknown` (`contract_drift`) | `profiles_test.go` & `runner_test.go` 单测 |
| `speed` 计时区间与极短伪响应（`< 1024B` 或 `204`） | 仅计 `client.Do` 响应头耗时；极短响应判 `available` | 计时覆盖 `client.Do` + `readBoundedResponse` 全过程；`< 1024B` 或非 `200` 判 `unknown`；超预算仍返 `error` (`speed_budget_exceeded`) | 慢速流式 Reader 计时单测 + 边界字节单测 |
| 单次 `ProbeRun` 内单节点探测 `K` 个可用维度的 `dialer` / `cleanup` 次数 | `K` 次 `dialer`、`K` 次 `cleanup` | `1` 次 `dialer`、在该节点全部任务 `OnComplete` 完成后 `1` 次 `cleanup`（取消/失败 `0` 泄漏） | 原子计数器并发/两阶段/取消/失败单测 |
| `ListLatestByNodes` 查询 `N` 节点 × `K` 维度 × 每组合 `H` 条历史记录时 SQLite 返回行数 | `N × K × H` 行（随历史深度线性膨胀） | DB 内窗口函数筛选 `rn = 1`，返回 `<= N × K` 行（与历史深度 `H` 解耦） | `sqlite` 回归测试 + 离线 `BenchmarkListLatestByNodes` |

## Goals / Non-Goals

**Goals:**
- 在 `internal/probe/profiles` 与 `internal/application/probe` 中建立基于真实证据的各维度判定契约，彻底消除 `HTTP 200/204` 一刀切带来的伪阳性。
- 修复 `speed` 探针完整响应体传输计时，设置最小有效测速负载门槛（`MinValidSpeedBytes = 1024`），并保留超限预算 `speed_budget_exceeded` 错误行为。
- 在 `DefaultRunner.Run` 内实现同 `ProbeRun` 同节点的客户端单次创建、跨阶段/跨维度复用及全部任务回调完成后的确定性单次关闭。
- 在 `internal/repository/sqlite/probes.go` 中实现数据库内按 `(node_logical_id, kind)` 分区选最新（含 `observed_at DESC, id DESC` 决胜）与复合索引加速，并配套离线基准与回归测试。
- 将全部改动拆分为两个写集合完全正交的特性级工作包（工作包 1：`internal/application/probe/**` + `internal/probe/profiles/**`；工作包 2：`internal/repository/sqlite/**`）。

**Non-Goals:**
- **严禁编写未经证实的 Netflix / OpenAI 私有解锁规则，严禁照搬 `subs-check-pro v3.2.1` 的误判实现**。
- **严禁删除或弱化** `NewSafeNodeDialer` 的 Server DNS Pinning、SSRF 公网 IP 校验、TLS 证书校验、HTTP 重定向禁止（`http.ErrUseLastResponse`）以及 `domain.ComputeProbeEvidenceDigest` 签名摘要计算逻辑与既有公开函数签名。
- 不修改 HTTP 路由或前端代码，不执行 `git commit`、容器构建或触碰生产环境。

## Decisions

### Decision 1: 各 `ProbeKind` 响应真实性校验与 `profiles.Evaluate` 协同契约

- **方案设计**：
  1. **保留已有 `internal/probe/identity.ExtractCandidate` 轮子（Prior Art First）**：`internal/probe/identity/identity.go` 已实现成熟的 JSON（`ip`/`query` + `country_code`/`country`/`countryCode`）与 Cloudflare Trace（`ip=...` + `loc=...`）双格式解析及 `net.ParseIP` + 2 位 ISO 国家代码校验。`runner.go`（或 `profiles` 辅助验证函数）直接复用 `identity.ExtractCandidate(body)` 验证 `geo` 与 `ip_risk` 的出口身份负载，绝不重复造轮子。
  2. **按维度精确定义契约匹配条件（替代 `resp.StatusCode == 200 || resp.StatusCode == 204`）**：
     - **`ProbeKindBaseline`**：要求 `StatusCode == http.StatusNoContent (204)` 且 `len(Body) == 0 && BytesRead == 0`。若 `StatusCode == 200` 或响应体非空（且未命中 `hasRestrictionMarker` / `401` / `403`），则 `ContractMatched = false`，`Evaluate` 返回 `VerdictUnknown`（`reason=contract_drift`）。同时在 `profiles.Profile.Evaluate` 中加固：当 `p.Kind == domain.ProbeKindBaseline` 时，若 `result.StatusCode != http.StatusNoContent || len(result.Body) > 0 || result.BytesRead > 0`，即便调用方传入 `ContractMatched: true` 也返回 `VerdictUnknown`（`reason=contract_drift`）。
     - **`ProbeKindGeo`**：要求 `StatusCode == http.StatusOK (200)` 且 `identity.ExtractCandidate(body)` 成功返回合法 IP 与 2 位国家代码。若状态码非 `200` 或解析失败，则 `ContractMatched = false`，返回 `VerdictUnknown`（`reason=contract_drift`）。当解析成功时，可在 `RedactedSummary` 中安全附加脱敏的 `country=<CC>`（绝不写入完整出口 IP）。
     - **`ProbeKindStreaming` 与 `ProbeKindAI`**：
       - 优先检查限制信号：当 `StatusCode` 为 `401`、`403`、`451` 或 `hasRestrictionMarker(body)` 为真（或包含明确地区/访问封锁标记）时，返回 `VerdictRestricted`（`reason=access_restricted`）；
       - 检查传输/超时错误：`DNSError || NetworkError || DeadlineExceeded` 返回 `VerdictError`（`reason=transport_error`）；
       - 在 `DefaultRunner` 中，由于仅请求 `https://www.netflix.com` 或 `https://api.openai.com` 根路径不具备可验证的业务解锁契约，`runner` 对 `streaming` 与 `ai` 恒设置 `result.ContractMatched = false`，使所有未受限的 `200`/`204` 响应（无论空响应、`"OK"` 或普通首页 HTML）均由 `Evaluate` 归类为 `VerdictUnknown`（`reason=contract_drift`），绝不产生虚假的 `VerdictAvailable`。
     - **`ProbeKindIPRisk`**：
       - `runner` 请求 `https://cloudflare.com/cdn-cgi/trace` 后，使用 `identity.ExtractCandidate(body)` 检验出口身份：若 `StatusCode != http.StatusOK` 或无法提取合法出口 IP + 2 位国家代码，则设置 `result.ExitIdentityMissing = true`，由 `Evaluate` 返回 `VerdictUnknown`（`reason=missing_exit_identity`）；
       - 若成功提取出口身份（`ExitIdentityMissing = false`），由于 `cdn-cgi/trace` 仅证明出口身份而非真实 IP 风险情报源（`iprisk.Provider`）的风险评分结果，`runner` 保持 `result.ContractMatched = false`，由 `Evaluate` 返回 `VerdictUnknown`（`reason=contract_drift`），绝不把出口身份当成风险评分 `available`。
     - **`ProbeKindSpeed`**：
       - 在 `profiles` 包定义常量 `MinValidSpeedBytes int64 = 1024`（1 KiB）；
       - 耗时测量：在 `runner.go` 中将 `latency = r.clock().Sub(reqStart).Milliseconds()` 移至 `readBoundedResponse(resp.Body, readLimit)` 完整读取（并关闭 `resp.Body`）之后执行，使 `latency` 包含连接建立、请求发送、响应头接收与完整响应体传输的全部时间；
       - 预算超限优先判定：当 `readBoundedResponse` 返回 `errResponseTooLarge`（超出 `prof.SpeedBudget.MaxBytesPerRequest`）时，保持 `result.BytesRead = prof.SpeedBudget.MaxBytesPerRequest + 1`，`Evaluate` 优先命中 `bytesRead > p.SpeedBudget.MaxBytesPerRequest || bytesRead > p.SpeedBudget.MaxBytesPerRun` 并返回 `VerdictError`（`reason=speed_budget_exceeded`）；
       - 最小有效负载校验：在未超预算且无网络错误时，仅当 `resp.StatusCode == http.StatusOK (200)` 且 `result.BytesRead >= profiles.MinValidSpeedBytes (1024)` 时才设置 `result.ContractMatched = true`（并在 `Profile.Evaluate` 中同步校验 `result.StatusCode == http.StatusOK && bytesRead >= MinValidSpeedBytes`），从而拒绝 `204`、空响应（`0B`）或 `"OK"`（`2B`）等极短伪响应（返回 `VerdictUnknown` / `reason=contract_drift`）。
- **备选方案对比**：
  - *备选方案*：在 `streaming` / `ai` 中通过正则匹配页面零散关键词（如 `subs-check-pro v3.2.1` 的做法）猜测解锁状态。
  - *放弃理由*：此类私有规则缺乏协议级稳定性，极易将 CDN 静态页、区域重定向页或未登录门户误判为 `available`，直接违反本次治理契约。

### Decision 2: 同 `ProbeRun` 同节点客户端生命周期复用管理器（`runNodeSession`）

- **方案设计**：
  1. **Run 级节点客户端池（`nodeSessionPool`）**：
     - 在 `DefaultRunner.Run` 内部创建局部的 `nodeSessionPool`（按 `node.LogicalID` 索引 `*nodeSession`）。
     - 每个 `nodeSession` 持有：`once sync.Once`、`client *http.Client`、`cleanup func() error`、`dialErr error`、`sessionCtx context.Context`、`sessionCancel context.CancelFunc`、`closeOnce sync.Once` 以及当前阶段剩余任务引用计数 `pendingTasks int32`（配合互斥锁保护）。
  2. **解决 `singbox.Runtime` 绑定 `ctx` 提前关闭问题**：
     - `nodeSession` 初始化时基于 `runCtx`（而非单个任务的 `taskCtx`）派生 `sessionCtx, sessionCancel := context.WithCancel(runCtx)`，并在首次拨号时将受拨号超时约束但生命周期绑定至 `sessionCtx`（或直接传入 `sessionCtx`，若自定义 `dialer` 需要）的上下文传给 `r.dialer(dialCtx, node)`。
     - 这样底层 `singbox.NewWithBoxConfig` 注册的 `context.AfterFunc` 绑定在 `sessionCtx` 上，不会在第一个任务（如 Phase 1 `baseline`）结束 `taskCancel()` 时被误关停；同时每个 HTTP 请求仍通过 `req = req.WithContext(taskCtx)` 严格执行单任务 `TaskTimeout` / `SpeedBudget.Deadline`。
     - 若返回的 `client != nil && client.CheckRedirect == nil`，在 `once.Do` 初始化期间一次性克隆并挂载 `CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }`，避免多任务并发读写 `client.CheckRedirect` 产生数据竞争。
  3. **精确引用计数与任务回调完成后关闭**：
     - **单阶段模式（`!isMixedTwoPhase`）**：
       - 在为某节点提交任务前，按该节点即将提交的任务数初始化/递增 `pendingTasks`；若某个任务的 `SubmitWithContext` 失败，立即递减 `pendingTasks`（若归零则触发 `session.close()`）；
       - 每个任务的 `OnComplete` 回调在调用 `wg.Done()` 前（或同时）递减该节点的 `pendingTasks`，当某节点的 `pendingTasks` 归零时（即该节点在本 Run 的全部任务及回调均已完成），立即通过 `closeOnce.Do` 调用 `sessionCancel()` 与 `cleanup()`。
     - **两阶段模式（`isMixedTwoPhase`：Phase 1 `baseline` -> Phase 2 `expensiveKinds`）**：
       - Phase 1 期间每个节点仅执行 `baseline` 任务，Phase 1 的 `OnComplete` 不立即关闭客户端，以便通过 `baseline`（`VerdictAvailable`）的节点在 Phase 2 直接复用已建立的客户端；
       - 在 `stage1WG.Wait()` 完成后：对于**未进入** `stage2Nodes`（即 `baseline` 非 `available` 或 Phase 1 提前终止）的所有节点，立即调用 `session.close()` 释放其客户端；
       - 对于进入 `stage2Nodes` 的节点，按其在 Phase 2 的任务数设置 `pendingTasks`，在每个 Phase 2 任务的 `OnComplete` 回调中递减，当该节点在 Phase 2 的全部任务回调完成（`pendingTasks == 0`）或提交失败归零时，立即调用 `session.close()`。
     - **全局退出兜底（Cancel / Deadline / Error 零泄漏保障）**：
       - 在 `Run` 的所有退出路径（含 `handleCancelOrDeadline` 在 `<-done` 收敛之后、Phase 1/Phase 2 提前返回之后）统一通过 `defer pool.closeAll()` 确保在所有工作协程和 `OnComplete` 回调结束之后，对任何尚未关闭的 `nodeSession` 执行 `closeOnce.Do`，保证 `cleanup()` 100% 精确执行 `1` 次且绝无资源泄漏。
- **备选方案对比**：
  - *备选方案*：跨不同 `ProbeRun` 全局长连接缓存 `sing-box` 实例。
  - *放弃理由*：跨 Run 全局驻留内存 `sing-box` 实例会在空闲期持续占用套接字与内存，且当节点配置更新时引发陈旧凭据/DNS 固定状态不一致；按单次 `ProbeRun` 内同节点复用即可将多维探测的拨号开销从 `O(N × K)` 降至 `O(N)`，同时保证 Run 结束后内存 100% 归零释放。

### Decision 3: SQLite `ListLatestByNodes` 数据库内窗口函数精确选取与复合索引

- **方案设计**：
  1. **复合索引保障（`internal/repository/sqlite/probes.go` 或 `db.go`）**：
     - 在 `NewProbeObservationRepository`（或连接初始化时）执行幂等索引创建：
       ```sql
       CREATE INDEX IF NOT EXISTS idx_probe_obs_node_kind_observed_id
           ON probe_observations(node_logical_id, kind, observed_at DESC, id DESC);
       ```
     - 该索引与分区排序键 `(node_logical_id, kind, observed_at DESC, id DESC)` 完全对齐，且修改范围严格收敛在 `internal/repository/sqlite/**` 写集合内。
  2. **窗口函数 `ROW_NUMBER() OVER (PARTITION BY node_logical_id, kind ORDER BY observed_at DESC, id DESC)` 查询**：
     - 在 `probeObservationRepository.ListLatestByNodes` 中（保留每批 `chunkSize = 100` 的参数分片与去重），将查询重写为：
       ```sql
       WITH ranked_observations AS (
           SELECT id, probe_run_id, node_logical_id, kind, verdict,
                  evidence_digest, observed_at, latency_ms, redacted_summary,
                  ROW_NUMBER() OVER (
                      PARTITION BY node_logical_id, kind
                      ORDER BY observed_at DESC, id DESC
                  ) AS rn
           FROM probe_observations
           WHERE node_logical_id IN (%s) [AND kind IN (%s)]
       )
       SELECT id, probe_run_id, node_logical_id, kind, verdict,
              evidence_digest, observed_at, latency_ms, redacted_summary
       FROM ranked_observations
       WHERE rn = 1;
       ```
     - SQLite 在数据库引擎内部按 `(node_logical_id, kind)` 分区并根据 `observed_at DESC, id DESC` 选出 `rn = 1` 的唯一最新记录，传输给 `database/sql` 的行数最多为 `len(chunk) × len(kinds)`。
- **备选方案对比**：
  - *备选方案*：`GROUP BY node_logical_id, kind` 配合 `MAX(observed_at)`。
  - *放弃理由*：SQLite 的 `MAX(observed_at)` 裸聚合在 `observed_at` 秒级时间戳相同时无法保证按 `id DESC` 确定性决胜（tie-break），而 `ROW_NUMBER() OVER (PARTITION BY node_logical_id, kind ORDER BY observed_at DESC, id DESC)` 与仓库内 `internal/repository/sqlite/node_risk.go:580` 的成熟范式完全一致且具备严格的 `(observed_at DESC, id DESC)` 确定性决胜语义。

## Risks / Trade-offs

- **[Risk] 既有测试夹具中部分 `baseline` / 全量多维测试使用 `HTTP 200 "OK"` 桩响应** → **Mitigation**：
  - 在工作包 1（写集合 `internal/application/probe/**`、`internal/probe/profiles/**`）中，同步更新 `runner_test.go`、`stage_gate_test.go` 中以“验证 `baseline` 可用通过”为前提的测试夹具使其对 `baseline` 返回 `204 No Content` 空响应（对 `geo` 返回合法 IP+国家代码 JSON、对 `speed` 返回 `>= 1024` 字节负载），同时保留并新增专门针对 `baseline` 返回 `200 "OK"` 必须被拒为 `unknown` 的回归测试，以及更新 `profiles_test.go` 中 `TestIPRiskMatchedContractProducesAvailable` 等单测以对齐收紧后的契约。
- **[Risk] 单阶段并发多维探测时同一节点的多个任务同时首次触发 `getOrCreateClient`** → **Mitigation**：
  - 每个 `nodeSession` 使用 `sync.Once` 串行化该节点的单次 `r.dialer` 调用，并发到达的同节点任务等待 `sync.Once` 完成后共享同一个 `*http.Client`，且 `CheckRedirect` 在 `sync.Once` 内一次性配置完成，彻底消除并发初始化竞争。
- **[Risk] `EnqueuePeriodicDedupe` 静默跳过任务时会在 `SubmitWithContext` 内同步调用 `task.OnComplete(nil)`** → **Mitigation**：
  - 引用计数在调用 `SubmitWithContext` **之前**预先递增，在 `OnComplete` 回调内递减，若 `SubmitWithContext` 返回非 `nil` 错误（此时调度器未调用 `OnComplete`）则在错误处理分支递减，确保无论任务正常执行、被周期去重同步调用 `OnComplete(nil)`、被 `CancelRun` 取消回调还是提交报错，引用计数均严格守恒。

## Migration Plan

1. 本次变更不涉及外部 API 破坏性变更或持久化表结构破坏，幂等索引 `idx_probe_obs_node_kind_observed_id` 在仓储初始化时自动通过 `CREATE INDEX IF NOT EXISTS` 创建。
2. 两个正交特性工作包（工作包 1：`internal/application/probe/**` + `internal/probe/profiles/**`；工作包 2：`internal/repository/sqlite/**`）可并发独立实施并通过各自单测与静态编译门禁，最后在汇聚门禁运行全量回归测试。

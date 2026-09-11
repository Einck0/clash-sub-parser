## Purpose

Implements an ultra-high concurrency in-memory proxy node probing engine capable of 100 to 500 concurrent goroutines with direct memory dialing and physical egress isolation.

## ADDED Requirements

### Requirement: 纯内存滑动窗口高并发探测
系统 SHALL 采用参考 `subs-check-pro` 的 Goroutine 滑动窗口工作池（并发范围 100~500），通过纯内存 Dialer 建立被测节点代理通道，绝不拉起外部 sing-box 子进程，绝不在磁盘生成临时 JSON 文件，绝不占用本地 TCP 回环端口池。

#### Scenario: 100 并发批量探测大节点清单
- **WHEN** 客户端向 `/api/probe/batch` 提交包含 500 个节点的批量探测请求，设置并发为 100
- **THEN** 系统在工作池内并发派发探测任务，所有节点代理握手均在内存中完成，宿主端口不发生递增占用，并在指定超时时间内完成全量结果汇总

### Requirement: 物理出口绝对隔离与环境代理禁用
探测引擎在建立节点连接时，MUST 严格保证该连接仅通过被测节点指定的代理配置流出，且 HTTP Client 必须禁用系统环境变量代理（`HTTP_PROXY`、`ALL_PROXY`），杜绝因宿主网络连通或代理泄露导致假阳性。

#### Scenario: 被测节点失效而宿主可正常联网
- **WHEN** 被测节点的远端服务器离线或证书无效，而运行宿主机拥有正常公网连接与环境代理
- **THEN** 该节点的探测结果严格记录为 `fail` 或 `timeout`，绝不记录为 `ok`，且出口 IP 绝不得体现宿主机的公网 IP

### Requirement: 多阶段细粒度能力证据链
探测引擎 SHALL 依次执行基础传输延迟（204）、出口 IP 与地理位置（IP/ASN/国家）、流媒体（YouTube, Netflix, Disney+, Bilibili）及 AI 服务（ChatGPT, Claude, Gemini, Meta AI）解锁检测，各阶段证据独立记录且互不污染。

#### Scenario: 基础网络可达但流媒体受到封锁
- **WHEN** 节点的 TCP 握手正常且出口 IP 识别成功，但 Netflix 返回地域不可用（404/403/重定向）
- **THEN** 节点的 `status` 标记为 `ok`，延迟有效，而流媒体结果中的 `netflix` 明确标记为未解锁或限制，保存结构化证据

### Requirement: 运行时软内存防线与动态背压限速
探测引擎 SHALL 在启动时配置运行时软内存上限（默认 200MB），并在活跃检测任务中监测内存压力；当检测到堆内存使用超过阈值时，自动对 Goroutine 工作池执行滑动限速与降并发保护。

#### Scenario: 大并发批量检测遭遇内存高压
- **WHEN** 在 500 并发压测下系统总内存占用接近 200MB 上限
- **THEN** 探测引擎动态将新任务分发并发度平滑下调至安全区间，保障进程不发生 OOM 崩溃

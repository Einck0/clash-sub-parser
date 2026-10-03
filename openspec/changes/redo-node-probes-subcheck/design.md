## Context

详见 `proposal.md`。当前 CSP 节点探针在内部使用 Sing-box 内存实例与自定义 `SafeDialer` 进行拨号，存在大量人为主观限制（拦截自签证书、阻断 Hysteria2 端口范围和 TUIC `disable_sni`、强行使用本地 DNS 解析验证远端 IP 并实施私网断言）。流媒体与 AI 探针契约被硬编码为 `contract_drift` 假存活。此外，调度器硬限制并发为 16~32，无法承载大节点库的快速初筛。

本设计对齐工业界成熟开源项目：
- **权威主基准**：`https://github.com/beck-8/subs-check` @ `3c320fd58aff5235e16218c050ec5b8ce587e233`（本地缓存 `/tmp/subs-check-beck`，GPL-3.0 许可）；
- **辅助调度与连接复用参考**：`https://github.com/sinspired/subs-check-pro` @ `e768ab277457ad3edc87c1cea6738a312192686e`（本地缓存 `/tmp/subs-check-pro-src`，GPL-3.0 许可）；
- **历史溯源分支**：`https://github.com/sinspired/subs-check` @ `8055505bf95631bf4dc6beabaa4956567b1c7f4e`。

## Goals / Non-Goals

**Goals:**
- **进程内 Mihomo 适配出站**：基于 `github.com/metacubex/mihomo` (`v1.19.31`) 的 `adapter.ParseProxy` 构造单节点代理出站，完整支持所有协议参数，支持 `skip-cert-verify: true`、Hysteria2 `ports` 与 TUIC `disable_sni`。
- **目标域名直传远端与宿主代理隔离**：设置 `Proxy: nil` 彻底切断宿主 `HTTP_PROXY`/`HTTPS_PROXY` 穿透；目标主机域名直接作为 `constant.Metadata` 传入隧道，远端代理解析，防止本地 DNS 泄漏。
- **三阶段漏斗调度 (Funnel Pipeline)**：Alive (测活初筛) -> Media/AI (流媒体与 AI 深度检测) -> Speed (可选带宽测速)，存活失败立即终止，不再无差别执行重型检测。
- **端到端多平台细粒度结果**：全面移植 beck-8 的真实商业检测算法（OpenAI 网页/客户端双轨、Netflix 全解锁/自制剧/封禁、YouTube 地区码及送中过滤、Disney+、Claude、Scamalytics IP 风险分），彻底删除 `contract_drift` 硬编码。
- **SQLite -> API -> WebUI 统一数据链路**：扩展结构化观测结果并直通 WebUI，展现清晰的平台徽章与状态。
- **完整向后兼容与鉴权保全**：保全既有订阅库存、节点台账、定时调度、管理 API 鉴权机制与 `connection_revision` 校验。

**Non-Goals:**
- 不触碰非探针业务：继续保留 Sing-box 用于客户端配置导出与订阅转换，不修改 Gateway、Hermes 内核或模型路由。
- 不恢复无关专项：不恢复无关的 Gemini 模型异常专项（仅作为通用能力探针检测）。
- 不自造私有轮子：严禁手搓脆弱代理实现或过度工程化审批平台。

## Decisions

### 1. 采用进程内 Mihomo 替代探针专用 Singbox 内存运行时
- **决策**：使用 `adapter.ParseProxy(mapping)` 直接在 Go 进程中解析节点并获得 `constant.Proxy` 实例。
- **替代方案考虑**：
  - *继续修补 Singbox 内存实例*：Singbox 对 Hysteria2 端口跳跃、TUIC 部分混淆选项存在复杂配置抽象，且原实现中包含过度安全门禁，修补成本高且难以与 subcheck 生态保持一致；
  - *拉起外部子进程*：生成配置文件并拉起外部进程开销过大，容易造成进程风暴与 I/O 抖动。
- **结论**：进程内 `adapter.ParseProxy` 具备高性能、轻量级、连接复用友好等显著优势，为业界主流实现。

### 2. 三阶段流水线调度与并发控制
- **决策**：
  - Stage 1 (Alive)：测活初筛阶段（默认并发 50），极短超时（2~3 秒），探测 `http://cp.cloudflare.com/generate_204` 或 CDN Trace，快速淘汰坏死节点；未通过测活的节点严禁进入 Stage 2 与 Stage 3；
  - Stage 2 (Media/AI)：流媒体与 AI 深度检测阶段（默认并发 20），对 Stage 1 成功的节点并行或按需探测 OpenAI、Netflix、YouTube、Disney+、Claude、IPRisk；必须等 Stage 2 全量执行完毕后方可启动 Stage 3；
  - Stage 3 (Speed)：可选测速阶段（默认并发 8；注：核实上游 subs-check 默认配置为 Alive 50 / Media 20 / Speed 20，本项目采用 Speed 8 系为了降低带宽争用与并发压力的本地差异化配置，非上游默认），完全接入 `platform.CheckSpeed`，使用 `NetworkLimitedReader` 设定 5MB 默认应用读取预算（作用于 `resp.Body` 读取截断，TCP 缓冲区存在网络层 read-ahead，非 NIC 网卡硬上限），单调截止超时（10 秒），测速结果以 `float64 KB/s (1024 B/s)` 计入 `throughput`。
- **生命周期**：每个待测节点在进入阶段时创建独立 `ProxyClient` 并在各阶段之间复用该客户端实例（复用上层 adapter 与 Transport 连接池，不称物理网络只拨号一次），探测完成后统一执行 `Close()` 释放底层连接；支持用户单选或混选探针类型；支持外部上下文 Cancel 优雅取消。

### 3. 多平台检测算法与数据契约（冻结共享API）
- **OpenAI**：双轨探测（Cookies 网页端 + iOS 客户端网关），全通判定 `Full (GPT⁺)`（`sub_tier: 'full'`），单通判定：Cookies 通为 `Web (GPT)`（`sub_tier: 'web'`），iOS 通为 `App`（`sub_tier: 'app'`，不硬归 web）；两项均不通判受限 `restricted`（`sub_tier: 'banned'`）；提取 Trace 国家码写入 `region`。
- **Netflix**：优先 Fast.com CDN 测速 API（403 判封禁 `sub_tier: 'banned'`，有效地区判全解锁 `sub_tier: 'full'` 提取 `region`）；回退至双 Title（81280792 与 70143836）状态断言（区分全解锁 `full`、仅自制剧 `originals`、明确封禁 `banned`）。
- **YouTube**：请求 `https://www.youtube.com/premium`，提取 `INNERTUBE_CONTEXT_GL` 区域标签；命中 `www.google.cn` 判定送中 `CN`；命中未开通文案判定不支持；包含正向会员或广告关键词判定解锁 `unlocked`，提取区域码。
- **Disney+**：通过 BAMGrid 认证断言接口判断：已解锁 `unlocked`、即将上线 `soon`（符合上游未开放语义，判定受限 `restricted`，绝不虚报为已解锁）、封禁 `banned`。
- **Claude**：请求 `https://claude.ai/cdn-cgi/trace`，正则提取 CDN 边缘国家码启发式并核对受制裁国家黑名单（AF, BY, CN, CU, HK, IR, KP, MO, RU, SY），若受限判定 `restricted`。
- **Gemini**：请求真实页面提取 JSON/HTML 中的三字国家码（非 trace），判定解锁或封禁。
- **IP 风险**：请求 Scamalytics 提取 IP Fraud Risk 评分，填入 `risk_score`。
- **测速**：`CheckSpeed` 返回带宽（float64 KB/s = 1024 bytes/s），记录为 `throughput`。
- **能力契约映射**：
  - `ProbeKind`: `baseline`, `geo`, `streaming`, `ai`, `speed`, `ip_risk`
  - `ProbeVerdict`: `available`, `restricted`, `unknown`, `error`, `stale`
  - `CapabilityStatus`（即 `NodeCapabilityView`）：保留原有字段 `verdict`、`latency_ms`、`observed_at`、`summary`、`stale`，新增可选：
    - `region?: string`
    - `sub_tier?: 'full' | 'web' | 'app' | 'originals' | 'banned' | 'unlocked' | 'soon'`
    - `throughput?: number` (KB/s = 1024 bytes/s)
    - `risk_score?: string`
    - `platforms?: Record<string, PlatformCapability>`（用于 `streaming` 与 `ai`，平台键至少包含 `openai`, `claude`, `netflix`, `youtube`, `disney`，支持 `gemini`）
  - `PlatformCapability`: `{verdict, latency_ms?: number, observed_at?: RFC3339, summary?: string, region?: string, sub_tier?: 'full'|'web'|'app'|'originals'|'banned'|'unlocked'|'soon', throughput?: number, risk_score?: string, reason?: string}`
  - 组级 `verdict` 汇聚规则：任一实际可用为 `available`，皆明确受限为 `restricted`，全部错误为 `error`，否则 `unknown`。细分以 `platforms` 为准，无嵌套 platforms。未知地区省略，禁止为 AI 伪造 geo 地区，latency 缺测为 -1。
  - 数据持久化：`probe_observations` 扩展最小必要持久化字段（`evidence_data` JSON 结构），贯通 `/nodes` 与 `/node` 详情以及探针运行记录全链路。

### 4. 消除所有人为伪阻断
- 废除针对 `skip-cert-verify: true` 的报错拦截。
- 废除针对 Hysteria2 `ports`、TUIC `disable_sni` 的拒绝。
- 废除针对私有 IP 与本地 DNS 解析的预检拦截，由远端代理决定出站路由。

## Risks / Trade-offs

- **[引入 Mihomo 依赖体积增大]** → Mitigation: Mihomo 与 Sing-box 共享许多基础网络依赖，仅在编译目标中略微增加二进制大小（约 15~20MB），换取完全合规的工业级协议支持与稳定性。
- **[部分商业流媒体接口风控加剧]** → Mitigation: 严格遵循真实浏览器 User-Agent 与常见客户端 Headers，避免高频并发击穿同一目标（采用随机乱序与合理并发）。
- **[网络测速流量消耗]** → Mitigation: `NetworkLimitedReader` 在应用层响应体 (`resp.Body`) 设定 5MB 流量读取预算，到达上限立即返回 EOF 截断并按实际读取字节与耗时精确计算带宽；明确标明为应用层读取预算（因 TCP 缓冲存在 read-ahead，非 NIC 网卡硬上限），不为字节预算增加脆弱复杂的 OS 层控制。

## Migration Plan

1. **依赖升级**：在 `go.mod` 中引入 `github.com/metacubex/mihomo v1.19.31`。
2. **代码重构**：
   - 在 `internal/probe/mihomo/` 下实现进程内代理客户端与看门狗；
   - 在 `internal/probe/platform/` 下实现 OpenAI, Netflix, YouTube, Disney, Claude, IPRisk, Speed 平台检测器；
   - 在 `internal/application/probe/` 落地三阶段流水线调度。
3. **数据模型与契约升级**：平滑扩展 `probe_observations` 的 JSON 格式与 `internal/application/inventory/` 中的 `CapabilityStatus`，旧数据零破坏。
4. **前端适配**：在 `web/src/features/probes/` 与 `nodes/` 接入细粒度平台标签。
5. **验证与上线**：本地编译与单元测试 100% 通过后，按受控部署手册执行 SQLite 热备与镜像构建。

## Verification, Run Lineage & Evidence Clarification

### 1. 运行态谱系与旧 Run 关联澄清 (Run Lineage & Historical Integrity)
- **历史旧 Run (`pi_run_20261002_194438_3727280`)**：
  - 历史状态为 `EXITED` / `BLOCKED`，根因系缺乏可用的生产明文管理 Token 导致实网真实节点验收无法推进；
  - 该历史失败事实严格保留，严禁覆盖、改写或篡改历史失败记录。
- **当前新 Run (`pi_run_20261003_083633_4084296`)**：
  - 承接用户关于全面重做节点探针、对齐 subs-check、移除不合理门禁的明确授权；
  - 全流程完成 Mihomo 进程内出站、三阶段调度漏斗、真实商业平台判定移植、前端 WebAssets 重新编译同步；
  - 汇聚门禁独立代码审查已通过（`REVIEW: PASS`）；
  - 施工上一报告曾引用既有会话句柄 `sess_20261003_004041_3864824`，特在此确权澄清其属于新 Run `pi_run_20261003_083633_4084296` 之成功施工成果，旧 Run 历史 BLOCKED 绝不篡改。

### 2. Critic 隔离多视口黑盒验收事实 (Critic Visual Acceptance Facts)
- **验收环境**：本地独立沙箱预览服务 (`http://127.0.0.1:18081`)，基于独立 SQLite 数据库 (`/tmp/csp-preview/csp-sandbox.db`)，生产数据 0 写入；
- **验收证据**：`/tmp/pi-critic-workspace/screenshots/01..14`（共 15 张图片，覆盖 `01_desktop_dirty_token_gate.png` 至 `14_mobile_probes_evidence_drawer.png`，含 `11b`）；
- **视口与量测**：覆盖桌面端 `1440x900` 与移动端 `392x872`；
- **核心判定结论**：
  - 浏览器全流程 0 控制台报错 (console error: 0)、0 网络故障 (neterror: 0)；
  - 细粒度平台分级真实呈现：OpenAI `Full (GPT⁺) [US]` / `Web (GPT) [SG]` / `App Only [HK]`，Netflix `Full [US]` / `Originals Only [HK]` / `Banned (Fast 403)`，YouTube 送中识别 `CN` 与正常解锁 `US/HK`，Disney+ `Soon` 与 `Banned`，测速吞吐 `51.2 MB/s`、`15.0 MB/s`、`4.0 MB/s`；
  - 探针手动触发与取消流程实测正常；
  - **测试数据边界声明**：所有展示节点均为明确标记为 `[Test-Fixture]` 的受控测试数据，绝不声称或冒充真实商业节点网络解锁；
  - **总裁决**：`CRITIC: PASSED`。


## Why

当前 CSP 节点探针子系统存在严重的人为过度设计与臆造阻断（如无条件拦截自签名证书 `skip-cert-verify`、强行禁止 Hysteria2 端口跳跃与 TUIC `disable_sni`、将流媒体和 AI 探测硬编码为 `contract_drift` 假存活、将并发硬编码限制在 16~32）。用户明确要求彻底重做节点探针，全面对齐工业界成熟开源实践 `beck-8/subs-check`（主基准）与 `sinspired/subs-check-pro`（辅助调度与生命周期参考），移除主观安全门禁，恢复真实节点质量探测能力。

## What Changes

- **核心运行时替换**：将探针底层拨号从被替代的 Sing-box 内存包装与 SafeDialer 切换为成熟的进程内 Mihomo `adapter.ParseProxy` 方案，以内核真实语义支持全量协议，完整保留连接参数，正常支持 `skip-cert-verify`、Hysteria2 `ports`、TUIC `disable_sni`，消除私网/本地 DNS 截断阻断，目标域名交由远端代理代理解析，严格设置 `Proxy: nil` 杜绝宿主代理泄漏。
- **三阶段流水线重构**：实施 Alive (测活) -> Media/AI (流媒体与 AI) -> Speed (可选受控测速) 阶段漏斗；第一阶段失败立即终止，不再无差别执行昂贵探测；支持阶段性动态并发池与 `watchdog` / `cancel` 生命周期可靠回收。
- **真实能力检测落地**：废除 `contract_drift` 占位，全面移植 beck-8/subs-check 真实平台检测算法（OpenAI 网页/移动端双轨 + 地区、Netflix 全解锁/自制剧/封禁 + 地区、YouTube 地区码与送中过滤、Disney+ 解锁/封禁/上线中、Claude trace 地区与封禁过滤、Scamalytics IP 风险分与纯净度、Bounded Speedtest 带宽测速）。
- **数据库、API 与 WebUI 结果透传**：扩展/适配观测记录与节点 DTO，将各平台的细粒度状态（解锁/部分可用/封禁/不支持/超时/网络错误）如实存储并经 `/probes/*` 与 `/nodes/*` API 输出到 WebUI 监控工作台，消除“未检测”视差与虚假全绿。
- **移除孤立旧代码**：清理已被完全替代的探针旧拨号与过度规则，不影响其他依然使用 Singbox 的导出编译业务，保留管理鉴权、任务审计、定时触发与订阅库存。

## Capabilities

### New Capabilities
- `node-probes/mihomo-subcheck-runtime`: 采用进程内 Mihomo 适配器构建单节点独立 HTTP Client，完全遵循节点原生连接配置（支持 skip-cert-verify、hy2 ports、tuic disable_sni），远端域名解析，无环境变量代理穿透。
- `node-probes/pipeline-funnel`: 三阶段漏斗调度管道（Alive -> Media -> Speed），支持高并发轻量初筛与低并发深度检测，结合每节点 Client 复用、主动 Close 与挂起看门狗。
- `node-probes/platform-capability-matrix`: 工业级真实流媒体与 AI 能力检测矩阵（OpenAI, Netflix, YouTube, Disney, Claude, IPRisk, Geo），彻底消除伪造代码与硬编码拒绝。
- `node-probes/result-contract-and-ui`: 结构化多平台能力结果存储、统一 DTO 透传与 WebUI 流式工作台展示，区分封禁、不可达与真实解锁。

### Modified Capabilities

## Impact

- **核心依赖**：引入 `github.com/metacubex/mihomo v1.19.31` 用于探针出站拨号；探针运行时不再依赖 sing-box 拨号逻辑（原有订阅编译与导出功能继续保留 sing-box）。
- **内部包**：重构 `internal/application/probe/` 与 `internal/probe/` 目录，移除 SafeDialer 和过重调度限制。
- **API 与数据模型**：扩展 `internal/domain/probe.go` 观测结果与 DTO，补充平台结构化结果（如 `openai`, `netflix`, `youtube`, `disney`, `claude`, `speed` 等）。
- **前端工作台**：更新 `web/src/features/probes/` 与 `web/src/features/nodes/`，以直观标签和状态指示器展示各平台真实检测状态。
- **协议与许可**：参考实现源自 GPL-3.0 项目，严格保留原作者版权署名并在项目中维护对应 NOTICE。

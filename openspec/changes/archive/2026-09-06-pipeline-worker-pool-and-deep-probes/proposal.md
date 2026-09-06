## Why

NodeLedger 的手动深度质检把节点固定切成十个一批，并在每一批 HTTP 响应完整返回前才提交下一批，导致慢节点让可用槽位闲置。定时质检已具备开关和 60 分钟默认周期，但控制台没有运行状态或下一次触发信息。现有平台探针已建立“真实节点出站、共享会话、限时和脱敏证据”的边界，新增能力必须沿用该边界，不能以网页抓取或主机直连破坏它。

## What Changes

- NodeLedger 手动综合质检改为使用已保存的 `probe_concurrency` 上限的滑动客户端 worker pool；每个已完成槽位立刻领取一个尚未开始的节点，逐项更新结果与进度，并允许停止尚未开始的工作。
- 增加受鉴权保护的定时质检状态读取契约，并在 NodeLedger 顶部显示开关状态、运行状态、上次完成或失败信息、服务器时间和下一次预计触发时间的倒计时。它反映单实例调度器状态，不把“预计”伪装成保证执行时间。
- 新增 Claude 区域可达性观察，但不把 Cloudflare trace 的 `loc` 或静态国家黑名单误报为 Claude 产品解锁；结果必须明确区分“区域信号可用”和“服务能力未验证”。
- 强化 ChatGPT 观察为 Web 可达与 iOS App 请求形态的分级结果；只有各自可识别的正向响应才可声明对应级别，挑战、429、契约漂移和不可识别响应均不得升格为解锁。
- 保留现有有界测速读取和每节点共享隔离 HTTP 会话；不重写或复制测速器。
- 为 IP Risk 预留独立、非能力过滤的风险观察契约。只有在 Einck 确认可使用已授权的 Scamalytics API 并以部署秘密提供凭据后才实施。禁止无授权抓取 `scamalytics.com/ip/{ip}` 页面、禁止把出口 IP 交给未批准的第三方、禁止将风险分写入媒体“完整解锁”语义。[1][2]
- 定时任务状态由单实例运行时状态提供。当前 Dockerfile 使用单个 uvicorn worker；若以后扩展成多 worker 或多副本，本变更的状态与互斥语义必须先迁移到共享存储或独立 job owner，不能继续声称全局准确。

## Capabilities

### New Capabilities
- `probe-execution-observability`: 让控制台和 API 可观察定时深度质检的配置、运行、结果和预计下次触发时间，并提供受控的手动滑动池交互。
- `ai-access-tier-observations`: 为 Claude 区域信号和 ChatGPT Web、App 分级定义证据级、非凭据的观察结果，保证不会将不确定信号作为完整解锁。
- `ip-risk-observation`: 在获得授权 API 凭据与用户部署确认后，记录与能力筛选隔离、可脱敏的出口 IP 风险观察。

### Modified Capabilities
- `node-capability-probe`: 手动与定时批量质检的并发、取消、平台观察和结果返回行为扩展为有界滑动执行，同时保留节点出站隔离与测速上限。
- `node-probe-persistence-and-filtering`: 定期探测的调度状态可查询；新增的风险观察不得影响现有媒体能力筛选或订阅生成。

## Impact

- 前端：`frontend/src/views/NodeLedger.vue`、API 封装与类型、`nodeLedgerDomain.ts` 及 NodeLedger 纯函数测试；Settings 仅作为现有配置来源，不再复制并发或调度配置。
- 后端：`backend/app/routers/probe.py`、`services/scheduler.py`、`services/probe/service.py`、`providers.py`、`catalogue.py`、观察模型和探针契约测试。
- 数据：默认实现不增加数据库列；调度运行状态是单进程瞬态。已持久化 `media` JSON 仅可增加版本化、脱敏的保留键，不能写入 API 密钥、原始 HTTP body、cookie、代理 URL 或节点凭据。
- 运行：无需立即迁移或重启。IP Risk 落地前需要单独得到用户对 API 授权、部署秘密写入及出站第三方披露的确认。

## Sources

[1] https://scamalytics.com/ip/api/pricing
[2] https://scamalytics.com/products

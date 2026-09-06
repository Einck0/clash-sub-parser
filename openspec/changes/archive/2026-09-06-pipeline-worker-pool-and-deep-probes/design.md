## Context

See `proposal.md` for motivation. 现有手动质检位于 `NodeLedger.vue`：它按常数 10 切片，并串行等待 `POST /api/probe/batch` 的整批响应。后端的 `probe_batch_nodes` 已在单一请求内用 semaphore 限制 1 至 20 个节点；定时任务则在进程内 `AsyncIOScheduler` 的每分钟 tick 上依据 `_last_probe_run_at`、`_probe_lock` 和持久化 `probe_cron_*` 配置运行。生产 Dockerfile 启动的是单个 uvicorn worker，因此该锁和内存状态只对当前进程有效。

探针已具备三个不应被破坏的边界：节点专属 loopback 代理和 `trust_env=False`，每节点单次媒体阶段的共享 HTTP 会话与绝对截止时间，以及只返回 allowlist 证据字段的脱敏链路。现有 `media` JSON 同时承载各平台结果；以 `_` 开头的键不会进入轻量摘要的媒体能力矩阵。`ip_risk` 没有现有授权 API 凭据或可验证的供应商 API 集成，不能把网页 URL 当作可靠 API。

## Goals / Non-Goals

**Goals:**

- 消除前端固定十节点栅栏：并发槽位完成即消费下一个节点，并以已保存的 `probe_concurrency` 为发起上限
- 给操作者可信的定时任务感知，并明确它是当前单实例的状态和预计时间
- 冻结 Claude 与 ChatGPT 的分级、证据和完整解锁语义，防止“HTTP 200 或国家代码等于产品解锁”
- 保持测速 5 MB 等既有有界读取、真实节点出站、媒体共享会话和能力过滤语义
- 将 IP Risk 设计为单独、默认关闭、需确认的观察，绝不作为伪造的纯净度证明或能力筛选输入

**Non-Goals:**

- 不将后台调度器改成分布式作业系统、队列、SSE 或持久化 job ledger
- 不改变 `probe_concurrency` 的 1 至 20 服务端上限，不以手动探测绕过它
- 不重写 `check_download_speed` 或新增重复测速 outbox
- 不改变订阅导出、节点重命名或向名称注入 `0% | NF | GPT⁺` 等标签
- 不使用无授权网页抓取、账户登录、cookie、私有 token 或账号凭据判定任何第三方服务

## Decisions

### D1: 前端使用 singleton 请求的固定宽度滑动池，而不修改已有 batch 响应为流协议

`NodeLedger` 在开始手动批次时读取一次 `GET /api/probe/settings` 快照，取 `probe_concurrency` 为 `workerLimit`，创建最多 `workerLimit` 个异步 worker。每个 worker 从共享索引取得一个节点，调用现有单节点探测端点，合并返回结果，再继续取下一个节点；所有 worker 退出后才显示总结果。这样任何结束的 worker 立即释放槽位，不受缓慢同批节点影响。

`probeNode`/底层 API POST 增加可选 `AbortSignal` 参数。停止时先置 `dispatchStopped=true`，不再领新节点，再 abort 所有活跃浏览器请求；进度只统计已获得成功或失败响应的节点。浏览器中断是“停止等待响应”，不是服务端作业取消协议，因此 UI 固定文案为“已停止后续探测”，不伪称已终止后端已经开始的 sing-box runner。

替代方案：单一 `/batch` 增加 SSE 或 NDJSON 逐节点事件。拒绝原因是现有 API 是完整 JSON 批量结果，SSE 会引入连接恢复、授权、缓冲、断线语义和服务端 job ownership；当前问题可由有限 singleton 请求在不新增后台作业模型的情况下解决。

### D2: `GET /api/probe/status` 是单实例、瞬态且受现有鉴权保护的状态投影

`scheduler.py` 拥有唯一 `ProbeScheduleRuntime` 进程内状态，字段冻结如下：

```text
state: disabled | initializing | waiting | running | failed
server_now: integer Unix seconds
interval_minutes: integer | null
next_expected_at: integer | null
last_started_at: integer | null
last_finished_at: integer | null
last_summary: { total, ok, fail, timeout, skipped } | null
last_error_code: string | null
```

运行时状态不保存原始异常、节点名称、节点 URL 或秘密。Scheduler 在首次有效 tick 设立基准线后报告 `waiting`；真正启动前报告 `running` 并记录开始时间；完成后保存由 `probe_batch_nodes` 返回的 summary；异常后记录稳定分类（例如 `probe_run_failed`）而不是 `str(exc)`；关闭总开关或 cron 开关时报告 `disabled` 和 `next_expected_at=null`。`server_now` 一律由状态 API 返回，页面以它校正倒计时，每 15 秒读取状态、每秒本地 tick 显示、卸载清理计时器。

当前进程重启后状态重置为 `initializing`，直到建立本次进程基准线。Dockerfile 的单 uvicorn 启动模式使这一表述正确；未来水平扩展前必须将锁和状态迁移到共享 job owner 或事务性存储。替代方案是从 APScheduler 的 `next_run_time` 直接推断；拒绝，因为现有实际调度间隔在函数内受动态 DB 配置、锁和首次基准线控制，未等同于静态 trigger 时间。

### D3: 定时探测节点集合不使用订阅导出的能力筛选

当前 `collect_all_subscription_nodes` 会对订阅级速度和媒体筛选生效，因此它不满足“检测所有节点”的用户语义。Scheduler 使用一个独立的 inventory collector：读取所有 enabled subscription 的 `raw_nodes`，合并去重但不调用 `is_node_capability_qualified`。手动 NodeLedger 仍遵循其已选择或筛选的节点集；只有后台“所有节点”改为未过滤库存。

替代方案是修改共用 `collect_all_subscription_nodes` 删除筛选。拒绝，因为该函数还服务其他调用链，可能破坏原有导出或编译预期。

### D4: AI 结果使用冻结的 `observation_kind`、`tier` 与 `subobservations` 扩展，不改变完整解锁谓词

平台结果仍保留现有顶层字段 `status`、`verdict`、`unlocked`、`confidence`、`evidence`。追加的公开、安全字段冻结为：

```text
observation_kind: capability | region_signal
tier: none | web | app
subobservations: {
  web?: ProviderResult,
  app?: ProviderResult
}
```

Claude 保存为 `media["claude"]`，`observation_kind="region_signal"`。即使 trace 有可解析 `loc`，顶层结果固定 `verdict="unknown"`、`unlocked=false`，使现有 `is_media_full_unlocked`、订阅筛选和 UI 成功 badge 一律拒绝。地区值只是观察证据，不是官方支持国家清单或账户可用证明。

ChatGPT 的子观察固定为 `web`（现有公开 Web 状态契约）和 `app`（明确带 `X-Requested-With: com.openai.chatgpt` 的 iOS 请求形态）。只有两个子观察皆为 confirmed available 时，顶层为 `status="verified"`、`verdict="available"`、`unlocked=true`、`tier="app"`；仅 Web 确认时顶层为 `status="partial"`、`verdict="unknown"`、`unlocked=false`、`tier="web"`；其余为对应失败/不确定且 `tier="none"`。这避免将 Web 成功假装成 App 能力，也不把 App HTTP 响应假装成已登录用户可使用产品。

将 `claude` 纳入允许配置平台和前端展示矩阵；ChatGPT 保留一个平台卡，通过 `tier` 显示 GPT 或 GPT⁺，而非重复两个能力键。所有结果继续走 `sanitize_probe_evidence`；`subobservations` 内每条 evidence 也只允许现有 six-key allowlist。

### D5: IP Risk 使用保留的 `_ip_risk` 观察命名空间，且落实任务在确认前不派发

为避免无理由 schema migration，已持久化的 `node_probe_results.media` JSON 使用保留键 `media["_ip_risk"]`：

```text
provider: "scamalytics"
status: disabled | verified | rate_limited | timeout | transport_error | inconclusive
evidence_version: string
score: integer 0..100 | null
level: low | medium | high | unknown
checked_at: integer
error_code: string | null
```

该物理存储位置不会进入 `STANDARD_MEDIA_PLATFORMS` 或动态媒体摘要，因为键以 `_` 开头；过滤器、编译器、统计和前端完整解锁判定均只消费真实平台键。风险观察仅在 identity consensus 产出 verified IP 后、显式设置启用、部署有 secret provider API key、以及 Einck 确认外发该出口 IP 到 Scamalytics 后执行。实现调用供应商正式 API，不解析公共页面；API key 只通过部署环境秘密提供，绝不出现在 settings GET/PATCH、数据库 JSON、日志、异常、前端或测试 fixture。

替代方案：立即抓取 `/ip/{ip}` HTML。拒绝，因为它不是已授权的稳定 API，易受反爬和页面漂移影响，也会在未经同意时披露出口 IP。另一个替代方案是把分数作为 `media["ip_risk"]` 以供筛选。拒绝，因为会让风险评分错误地进入媒体能力语言与导出规则。

## Risks / Trade-offs

- [多标签页同时手动发起] → 每个页面自身严格限宽，但没有新的跨请求全局手动 job lock；维持现有后端请求内保护，文档和 UI 不宣称全局 worker pool
- [浏览器 abort 后服务端仍继续] → 明示取消范围，保留服务端持久化的任何已完成结果，下次刷新重新获取事实
- [单实例 scheduler 内存状态丢失] → `initializing` 明确报告并在扩容前禁止全局准确承诺
- [Claude 或 OpenAI 端点契约漂移] → `inconclusive`、版本化 evidence、fixture contract tests；绝不自动升级成“解锁”
- [IP Risk 第三方 API / 隐私 / 成本] → 硬性确认门、secret-only key、单次节点观察超时、无网页回退、独立 review
- [后台“全部节点”扩大负载] → 保留并发、节点/服务/媒体阶段/下载字节上限；延迟首轮一个完整 interval，禁止启动补跑冲击

## Migration Plan

1. 先部署不含 IP Risk 的后端和前端改动；不需要数据库迁移。`_ip_risk` 仅是未来可选 JSON 键，旧记录不受影响。
2. 在单 worker 运行态先做内存状态、后台全库存、滑动池和 AI fixture 验收；保存设置后通过 status API 验证开关、间隔、重启初始化与无重叠。
3. 用户确认 IP Risk 的 API 授权、密钥配置及外发出口 IP 后，才部署 gated IP 任务；默认仍关闭，先以 synthetic fixture 验证。
4. 回滚：前端可回退到不显示状态并停止前端池；后端可移除 status 和新平台 dispatcher 而不迁移数据。已存在的 `claude` 或 `_ip_risk` JSON 键保持为不可消费的历史观察；绝不双写、影子 ledger 或兼容 outbox。若 IP Risk 已启用，先关闭开关并撤销部署 secret，再回滚代码。

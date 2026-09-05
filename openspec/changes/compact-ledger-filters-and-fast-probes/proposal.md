## Why

节点台账已把地区与解锁能力作为一长排 Chip 平铺，节点量与平台数量增长时会挤占操作空间，无法按维度清晰组合筛选。流媒体探针虽有节点级并发与单平台并发，但每个平台重复建立 HTTP 客户端，并且现有 Netflix 走两次完整 HTML 标题页；已持久化却未实际生效的 `media_timeout_s` 也让操作员无法独立控制媒体探测预算。

这次变更要在不降低真实节点出站、证据分级、历史台账和多选 AND 语义的前提下，收纳台账筛选并缩短媒体探测关键路径。

## What Changes

- 将节点台账筛选改为按“地区、协议、健康、链路、解锁能力”维度收纳的可展开多选控件，显示已选条件摘要和可清空动作；不再平铺所有地区或平台 Chip。
- 将筛选状态从单值 `country`、`protocol` 与 `status` 演进为同维度 OR、跨维度 AND 的多选集合；保留关键词、订阅、链路、最低速率和排序语义。
- 引入显式的媒体探针会话边界：每个已通过传输握手的节点只创建一个 `httpx.AsyncClient`，所有选定平台共享其连接池、代理固定和请求头。
- 将 `media_timeout_s` 作为媒体阶段总预算，而非沿用通用 service timeout；各平台必须从共同绝对截止时间计算剩余时间，任何平台失败或超时不丢弃同批兄弟结果。
- 为 Netflix 增加凭据无关的 Fast.com CDN 快速路径。仅在返回可验证国家或 HTTP 403 IP 阻断时提前结论；空、非预期或解析失败必须回退至既有双标题证据路径，不能把快速路径失败误判为解锁或受限。
- 维持所有对外 API 请求形状、平台键、`ProviderResult` 分类、`NodeProbeResult.media` JSON 持久化和历史兼容筛选；不新增数据库表、不做数据迁移、不引入账户、Cookie、令牌或浏览器自动化。

## Capabilities

### New Capabilities

- `ledger-facet-filtering`: 节点台账面向地区、协议、健康、链路与解锁能力的可访问、可组合、分组多选筛选体验。
- `media-probe-session-optimization`: 在已验证节点的本地回环代理上复用一个受时间预算约束的媒体 HTTP 会话，并为 Netflix 使用可审计的快速结论和安全回退。

### Modified Capabilities

- `node-capability-probe`: 媒体探测阶段采用独立总预算、共享 HTTP 会话和 Netflix 快速路径，同时保持真正的节点出站、结果分类和故障隔离。
- `node-probe-persistence-and-filtering`: 台账及下游能力过滤支持同维度多选 OR、跨维度 AND，且继续只接受已确认全解锁能力。

## Impact

- Frontend: `frontend/src/components/ledger/LedgerSearchFilter.vue`、`frontend/src/views/NodeLedger.vue`、`frontend/src/views/nodeLedgerDomain.ts` 及对应 Node 单元、组件和浏览器测试。
- Backend: `backend/app/services/probe/providers.py`、`catalogue.py`、`service.py`、`schemas/probe.py`、`probe_settings_service.py` 与探针单元和编排测试。
- 接口和存储：`GET/PATCH /probe/settings` 保持现有字段，`POST /probe/node` 与 `POST /probe/batch` 请求和结果形状保持兼容；仅让已存在的 `media_timeout_s` 成为实际生效的媒体阶段预算。
- 运行安全：不改变 sing-box runner、节点级 semaphore 上限、全局节点预算或数据库模式；部署回滚仍是应用镜像回退，新增媒体结果是既有 JSON 内的加性字段。
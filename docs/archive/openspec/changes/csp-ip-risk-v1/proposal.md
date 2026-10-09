## Why

当前 CSP Clean-Slate 已具备探测运行时、版本化 `ProbeObservation` 与节点台账的基础，但尚无独立、可审计的出口 IP 风险证据和消费该证据的准入、管理筛选与导出阻断合同。把风险分数直接写入现有 geo 或 capability verdict 会混淆“出口属性”“第三方信誉”和“本地准入决策”，并会使第三方配额、脱敏和失败降级无法单独治理。

本变更建立 CSP+IP Risk 的最小独立能力：以经被测节点出口的受控风险 enrichment 形成不可变、可过期的证据，再由确定性风险 policy 解释为管理筛选和发布准入结果。它不重建正在进行的 CSP Clean-Slate 施工 DAG，也不把网页抓取或第三方原始响应引入运行时。

## What Changes

- 新增版本化 `ip_risk` ProbeProfile、provider adapter 合同、按 provider/node 的配额与缓存边界，以及“无 API 凭据或 provider 不可用即 unknown”的安全降级。
- 新增不可变 `IPRiskObservation` 证据模型：保存 SHA-256 证据摘要、规范化分类、来源版本、观察时间和有限脱敏摘要；不保存完整 IP、第三方原始 payload、API key、cookie 或网页 HTML。
- 新增独立的风险 policy：将 observation 的 freshness、confidence、score、匿名化和网络属性确定性映射为 `allow`、`review`、`block` 或 `unknown`，且不覆盖原有 capability verdict。
- 扩展节点台账读取合同，使管理端可按风险决策和风险等级筛选，并以风险卡片和高风险提示回显可解释但已脱敏的结果。
- 新增策略组 admission 与 publication preflight：仅已启用的、版本化的风险 policy 可排除或隔离节点；无有效风险证据时按照显式 policy 处理，默认不把未知伪装为低风险。
- 明确禁止浏览器抓取 Scamalytics 等网页端；仅允许有凭据、可配置的官方 API adapter，且 provider 凭据只作为部署端 secret reference 使用。
- **BREAKING**：当操作者启用一个以 `block` 为结果的 IP risk admission 或 publication policy 时，原来会被选入该策略组或导出的节点 SHALL 被拒绝或隔离。未启用 policy 时既有选择和发布语义不改变。

## Capabilities

### New Capabilities

- `ip-risk-evidence`: 定义经被测节点出口取得、脱敏存储、过期和可解释读取的 IP 风险证据合同
- `risk-policy-admission-and-publication`: 定义风险分级、节点筛选、策略组准入和导出前硬拦截合同
- `ip-risk-management-workbench`: 定义节点台账风险回显、筛选、高风险提示和未知状态展示合同

### Modified Capabilities

- `probe-evidence-engine`: 扩展 probe kind、运行调度和证据读取，以承载独立的 `ip_risk` profile 而不改变既有 profile verdict 语义
- `subscription-inventory`: 扩展节点列表和详情读模型，使其能够提供已脱敏的当前 IP risk 摘要与服务端风险筛选
- `policy-tree-and-compiler`: 扩展 resolver 和 publication preflight，使已启用 risk policy 的 admission 决策能确定性地影响解析快照和发布
- `admin-workbench-experience`: 扩展管理工作台的节点风险筛选与可解释回显要求

## Impact

- 计划实现范围：`internal/domain/`、`internal/application/observation/`、`internal/application/probe/`、`internal/probe/profiles/`、新增 IP risk provider adapter、`internal/repository/sqlite/`、`migrations/`、`internal/transport/http/`、`internal/application/policy/`、`internal/resolver/`、`internal/application/publication/` 与未来 `web/src/features/`。
- 当前可核验基础：`internal/domain/probe.go:60-70` 已有不可变 observation 骨架；`internal/application/observation/service.go:52-105` 已以 SHA-256 digest 和脱敏 bounded summary 持久化；`internal/probe/singbox/runtime.go:148-208` 已将 dial/HTTP client 绑定到节点 outbound 并显式禁用宿主代理；`internal/domain/policy.go:70-87` 已有 admission rule 值对象。
- 新增 provider 可能产生 API 费用与配额消耗；默认关闭，必须由部署端配置 secret reference、预算和显式启用。不会新增网页抓取、浏览器自动化、旧数据库依赖、兼容层、双写或运行时数据迁移。
- 本变更只交付规划工件；不构建镜像、不启动容器、不写运行数据库、不切换流量或发布订阅。

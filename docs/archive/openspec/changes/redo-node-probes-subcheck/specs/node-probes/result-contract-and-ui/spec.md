## Purpose

规范节点探针多平台细粒度结果在数据持久化、管理 API 与前端 WebUI 工作台之间的完整数据流与展示契约，冻结共享数据结构，消除多平台认知断层与假未知。

## ADDED Requirements

### Requirement: 冻结共享能力模型与多平台字典契约
系统 SHALL 在节点能力状态 `CapabilityStatus`（即 `NodeCapabilityView`）中维持核心枚举与字段，并扩展标准化细粒度多平台契约：
- 现有 `ProbeKind` 保持 `baseline` | `geo` | `streaming` | `ai` | `speed` | `ip_risk`；
- 现有 `verdict` 保持 `available` | `restricted` | `unknown` | `error` | `stale`；
- `CapabilityStatus` 保留现有字段 `verdict`、`latency_ms`、`observed_at`、`summary`、`stale`，并新增可选扩展字段：
  - `region` (string): 出口地区 ISO 二字码；
  - `sub_tier` (string): 细分层级，严格限定为 `'full'` | `'web'` | `'app'` | `'originals'` | `'banned'` | `'unlocked'` | `'soon'`；
  - `throughput` (number): 测速吞吐带宽，单位 KB/s（1024 bytes/s）；
  - `risk_score` (string): IP 风险分及等级描述；
  - `platforms` (Record<string, PlatformCapability>): 面向 `streaming` 与 `ai` 维度的真实平台字典，键至少包含 `openai`、`claude`、`netflix`、`youtube`、`disney`（支持 `gemini`），无嵌套 platforms。
- `PlatformCapability` 结构定义为：
  - `verdict`: `available` | `restricted` | `unknown` | `error` | `stale`；
  - `latency_ms` (number): 缺测为 -1，不将旧观测带入新结果；
  - `observed_at` (RFC3339 string);
  - `summary` (string);
  - `region` (string, 未知地区省略，严禁为 AI 探测伪造地理位置);
  - `sub_tier` (string, 枚举值同上；OpenAI 仅 App 通过时为 `'app'`，不得硬归为 `'web'`；Disney `'soon'` 符合上游未开放语义，不得当作已解锁);
  - `throughput` (number);
  - `risk_score` (string);
  - `reason` (string).
- 组级 `verdict` 聚合裁决规则：
  - 组内任一平台实际可用（`available`）时，组 verdict 判定为 `available`；
  - 组内所有平台皆明确受限（`restricted`）时，组 verdict 判定为 `restricted`；
  - 组内所有平台全部网络或协议错误（`error`）时，组 verdict 判定为 `error`；
  - 否则判定为 `unknown`；前端与业务细分必须以 `platforms` 字典为准。

#### Scenario: AI 平台多轨结果与组判决汇聚
- **WHEN** 节点的 `openai` 为 full (US) 可用，而 `claude` 受限封锁
- **THEN** 节点的 `ai` 能力项包含 `platforms.openai` 与 `platforms.claude`，且组 `verdict` 为 `available`

#### Scenario: Disney 即将上线状态语义保留
- **WHEN** 节点的 Disney+ 探测返回地区即将上线 (`soon`)
- **THEN** 平台的 `sub_tier` 记录为 `soon`，`verdict` 记录为 `restricted`，不得虚报为 `available`

### Requirement: 结构化多平台能力结果持久化存储
系统 SHALL 在 `probe_observations` 中以 JSON 结构（`evidence_data`）序列化存储多平台细粒度能力检测结果（包括但不限于 `openai`、`netflix`、`youtube`、`disney`、`claude`、`gemini`、`ip_risk`、`speed`），并在记录中保留探测发生时的 `connection_revision`、真实网络延迟与可审计证据摘要，实施最小必要迁移，打通从 SQLite 存储到 API 响应与节点详情的全调用链路。

#### Scenario: 完整结构化结果落库
- **WHEN** 节点的 Stage 2 媒体检测完成
- **THEN** 系统将包含细粒度平台判定与区域码的观测结果原子写入数据库，不遗失字段

### Requirement: 节点详情与工作台 API 统一透传
管理 API（包括 `/api/v1/nodes` 列表、`/api/v1/nodes/{id}` 详情及 `/api/v1/probes/runs/*`）SHALL 在响应中统一透传节点的 `capabilities` 字典，每一项能力均包含标准判决（`verdict`）、延迟（`latency_ms`）、时间戳（`observed_at`）以及平台扩展详情（如 `region`、`sub_tier`、`throughput`、`risk_score` 与 `platforms` 字典），杜绝前后端认知割裂。

#### Scenario: 查询节点详情包含多维度解锁数据
- **WHEN** 客户端请求 `GET /api/v1/nodes/{logical_id}`
- **THEN** 返回的 JSON 包含完整的 `capabilities` 字典，呈现各平台细粒度状态

### Requirement: 前端 WebUI 实时状态与平台徽标展示
前端工作台 SHALL 更新探针运行卡片与节点台账列表，以语义化的彩色徽章（Badge）和平台图标展示真实检测结果（如 `GPT⁺ (US)`、`NF (HK)`、`YT (SG)`、`Claude (JP)` 等），对封禁、不可用及未测速状态给予明确视觉区分，杜绝假未检测或虚假占位。

#### Scenario: 前端如实展示 GPT 满血徽标与地区
- **WHEN** 节点的 `openai` 能力返回 `Full` 且国家为 `US`
- **THEN** 前端在节点卡片上渲染绿色的 `GPT⁺ (US)` 标签

## MODIFIED Requirements

### Requirement: Capability-Based Filtering in Subscriptions and Node Groups
系统 SHALL 基于速度阈值与多选流媒体 AI 完整解锁能力过滤订阅和节点组。多项必需媒体能力 SHALL 同时满足；confirmed full unlock 之外的 partial、restricted、challenged、rate-limited、timeout、transport-error 和 inconclusive 均不得满足能力要求。节点台账 SHALL 以同一判定语义提供地区、协议、健康和解锁能力的多选筛选：同一维度内按 OR 匹配，不同维度以及速度、链路、订阅和关键词约束按 AND 组合。

#### Scenario: Filtering by speed and unlock targets
- **GIVEN** 一个节点组配置 `filter_min_speed_mbps = 10.0` 和 `filter_media_unlock = ['chatgpt', 'gemini']`
- **WHEN** 系统解析该节点组
- **THEN** 系统 SHALL 仅包含最新节点探测状态为 ok、速度至少 10.0 Mbps 且同时确认解锁 ChatGPT 与 Gemini 的节点

#### Scenario: Ledger facet selection combines values correctly
- **GIVEN** 操作者在节点台账中选择多个地区、多个协议和多个解锁能力
- **WHEN** 系统计算可见节点
- **THEN** 系统 SHALL 接受每个维度内任一已选地区或协议，并要求所有已选解锁能力和其他跨维度条件同时满足

#### Scenario: Historical result compatibility
- **WHEN** 历史探测记录只包含既有的 full、ok 或布尔媒体值
- **THEN** 系统 SHALL 保持已建立的完整解锁过滤兼容语义，直到该平台结果被新的结构化观测替换

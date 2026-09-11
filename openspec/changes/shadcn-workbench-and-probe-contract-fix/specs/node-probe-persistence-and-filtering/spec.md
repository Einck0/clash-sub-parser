## MODIFIED Requirements

### Requirement: Node Probe Results Persistence

The system SHALL store the outcome of all node capability and speed probes in a persistent relational table. A persisted result SHALL be keyed by the canonical current-node identity `name|type|server:port`, where string components are trimmed and a missing port is serialized as an empty segment.

The system SHALL converge persisted probe results and in-memory result cache entries to the current enabled final-node identity set after every successful operation that changes that set, including successful remote refresh, manual-node recomputation, selection-affecting subscription update, or subscription deletion. It SHALL remove only entries absent from that newly computed key set, shall be idempotent, and shall not create a second ledger, compatibility mirror, or shadow persistence path. A failed refresh SHALL leave the currently active node set and its matching persisted records untouched.

#### Scenario: Saving probe outcome

- **GIVEN** a node capability probe finishes
- **WHEN** the result contains latency, geo ip, speed, or streaming unlock status
- **THEN** the system stores or updates the record keyed by the canonical `name|type|server:port` identity in `node_probe_results`.

#### Scenario: Successful current-node set change removes orphaned results

- **GIVEN** persisted results include a key that is no longer represented by any enabled final node
- **WHEN** a successful source or subscription operation publishes the new current node set
- **THEN** the system removes that orphaned persisted result and cache entry while retaining each record whose key remains in the new set.

#### Scenario: Failed refresh preserves active-node probe results

- **GIVEN** a remote refresh fails before publishing a replacement node set
- **WHEN** the system records the refresh failure
- **THEN** it does not remove probe records belonging to the existing active nodes.

### Requirement: Capability-Based Filtering in Subscriptions and Node Groups

系统 SHALL 基于速度阈值与多选流媒体 AI 完整解锁能力过滤订阅和节点组。多项必需媒体能力 SHALL 同时满足；confirmed full unlock 之外的 partial、restricted、challenged、rate-limited、timeout、transport-error 和 inconclusive 均不得满足能力要求。节点台账 SHALL 以同一判定语义提供地区、协议、健康和解锁能力的多选筛选：同一维度内按 OR 匹配，不同维度以及速度、链路、订阅和关键词约束按 AND 组合。

节点台账 SHALL 以规范 `node_key` 将当前节点与探测结果关联。显示名只能作为单次即时探测响应的临时回填，并且仅当当前节点集合中该名称唯一时允许使用；持久化和分页摘要不得依赖显示名关联。无匹配结果 SHALL 表示尚未探测；`fail`、`timeout`、`skipped` 和收到的未知状态 SHALL 保持各自独立语义，未知状态不得按未测处理。

#### Scenario: Filtering by speed and unlock targets

- **GIVEN** 一个节点组配置 `filter_min_speed_mbps = 10.0` 和 `filter_media_unlock = ['chatgpt', 'gemini']`
- **WHEN** 系统解析该节点组
- **THEN** 系统 SHALL 仅包含最新节点探测状态为 ok、速度至少 10.0 Mbps 且同时确认解锁 ChatGPT 与 Gemini 的节点

#### Scenario: Ledger facet selection combines values correctly

- **GIVEN** 操作者在节点台账中选择多个地区、多个协议和多个解锁能力
- **WHEN** 系统计算可见节点
- **THEN** 系统 SHALL 接受每个维度内任一已选地区或协议，并要求所有已选解锁能力和其他跨维度条件同时满足

#### Scenario: Persisted timeout is not presented as untested

- **GIVEN** 当前节点的规范 key 关联到状态为 `timeout` 的持久化探测结果
- **WHEN** 操作者加载、刷新或筛选节点台账
- **THEN** 台账 SHALL 保留超时语义而不是将该节点分类、计数或显示为未测

#### Scenario: Historical result compatibility

- **WHEN** 历史探测记录只包含既有的 full、ok 或布尔媒体值
- **THEN** 系统 SHALL 保持已建立的完整解锁过滤兼容语义，直到该平台结果被新的结构化观测替换

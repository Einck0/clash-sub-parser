## Purpose

规范订阅刷新、下线剪枝、抓取失败与订阅删除各阶段写操作对历史来源记录的原子快照保全行为，彻底消除运行态证据丢失漏洞。

## ADDED Requirements

### Requirement: 刷新剪枝原子归档
当订阅刷新成功且部分现有成员不再包含在上游内容中时，系统 SHALL 在物理删除 `node_sources` 前将既有真实关联归档至 `node_source_history`。

#### Scenario: 成员下线快照持久化
- **WHEN** 订阅成功刷新导致某个节点自该订阅中移除时
- **THEN** 系统在同一事务中向 `node_source_history` 写入归档快照（`cause = 'refresh_removed'`），记录当时的 `connection_revision` 与 `fetch_id`，随后从 `node_sources` 中移除实时成员，且当前节点的 `active` 与配置参数保持不变。

### Requirement: 异常与状态变更保护
在刷新失败或订阅启用/停用状态变更时，系统 SHALL 严格隔离，禁止伪造历史归档或清除实时成员。

#### Scenario: 抓取失败状态保持
- **WHEN** 远程订阅抓取发生网络或解析失败时
- **THEN** 系统保持现存 `node_sources` 关联与健康节点状态，不向 `node_source_history` 插入虚假失败归档，亦不删除实时成员。

#### Scenario: 订阅停用与启用
- **WHEN** 管理员将某个订阅置为禁用或启用时
- **THEN** 系统更新订阅的 `enabled` 状态，不解绑 `node_sources` 关系，不篡改关联节点的 `active` 属性，亦不向历史台账新增冗余条目。

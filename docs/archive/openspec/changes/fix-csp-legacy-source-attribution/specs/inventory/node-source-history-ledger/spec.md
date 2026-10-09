## Purpose

定义独立的不可变历史来源台账表 `node_source_history` 的数据契约与约束规范，在不影响实时订阅成员（Scope E）的前提下记录节点的历史来源血统。

## ADDED Requirements

### Requirement: 不可变历史来源台账存储
系统 SHALL 提供 `node_source_history` 表，持久化存储节点的历史来源关系、观测时间戳、归属原因与可验证证据。

#### Scenario: 存储历史归属记录
- **WHEN** 记录被插入 `node_source_history` 时
- **THEN** 系统持久化存储 `node_logical_id`、可选的 `subscription_id`、脱敏展示名称 `source_label`、`source_identity`、`relation_state`、`cause`、`first_observed_at`、`evidence_kind` 与 `evidence_json`，且严格禁止记录明文密码、密钥、Token 与带敏感参数的原始 URL。

### Requirement: 证据去重与外键安全
系统 SHALL 通过复合唯一键确保历史证据的幂等性，并保证当订阅被物理删除时历史外键安全置空。

#### Scenario: 重复证据幂等忽略
- **WHEN** 尝试插入具有相同 `(node_logical_id, source_identity, evidence_kind, first_observed_at)` 的历史记录时
- **THEN** 系统幂等忽略或更新最近观测时间，不引发主键冲突，亦不覆盖不同时间点的多次观测事实。

#### Scenario: 订阅删除级联置空
- **WHEN** 关联的 `subscriptions` 记录被删除时
- **THEN** 数据库物理外键约束 `ON DELETE SET NULL` 自动将 `subscription_id` 置为 NULL，且保留已固化的 `source_label` 与 `source_identity`，不物理删除历史台账行。

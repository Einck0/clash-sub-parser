# config-transfer 规格

## Purpose

配置导出/导入/重置的领域逻辑契约。保证配置迁移的完整性（全表有序导出、失败整体回滚）与安全性（敏感字段不随导出泄漏、导入不覆盖认证凭据）。

## ADDED Requirements

### Requirement: 完整导出

导出 SHALL 按外键安全顺序序列化全部业务表，输出单个 JSON 文档，包含 schema 版本标记。

#### Scenario: 全量导出
- **WHEN** 调用导出接口且数据库含订阅、节点组、规则、DNS 等数据
- **THEN** 返回的 JSON 含全部表的全部行，字段与 ORM 模型一致，可独立用于迁移

### Requirement: 导入原子性

导入 SHALL 在单事务内完成；任一步骤失败时整体回滚到导入前状态，不留部分写入。

#### Scenario: 中途失败整体回滚
- **WHEN** 导入文档中某行违反约束导致写入失败
- **THEN** 数据库状态与导入前完全一致，响应返回明确的失败原因

### Requirement: 认证凭据保留

导入 SHALL NOT 覆盖 security_settings 中的 token_hash 等认证凭据字段。

#### Scenario: 导入不含凭据覆盖
- **WHEN** 导入文档包含 security_settings 数据
- **THEN** 当前系统的登录凭据保持不变，导入后原 token 仍可登录

### Requirement: 敏感字段过滤

导出 SHALL NOT 包含 token 原文等明文凭据；token 相关字段仅以哈希形态存在或被排除。

#### Scenario: 导出无明文 token
- **WHEN** 导出配置并检查内容
- **THEN** 文档中不存在任何可用于直接登录的凭据原文

### Requirement: 重置确定性

重置 SHALL 按 fixed 顺序清空业务表并恢复初始状态，顺序保证外键约束不被违反。

#### Scenario: 重置后系统可用
- **WHEN** 执行重置操作
- **THEN** 业务表清空、security_settings 保留、系统无需重启即可继续正常服务

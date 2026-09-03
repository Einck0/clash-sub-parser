## MODIFIED Requirements

### Requirement: 安全逻辑 Bundle
系统 SHALL 只导出 schema version、策略图、规则、DNS、发布选项和无节点载荷的 `selector_slot`。Bundle 不得包含来源/InventoryNode logical ID、订阅 URL、节点 payload、UUID、密码、私钥、认证或发布 token、数据库行 ID、运行时观测或旧快照

#### Scenario: 全量导出
- **WHEN** 操作者导出活动配置
- **THEN** 文档只表达可移植逻辑语义和 slot，不存在可用于连接上游或代理的材料

### Requirement: 跨实例导入与显式 rebind
系统 SHALL 把 Bundle 导入原子地保存为 `UNBOUND` 未发布草稿。导入先验证 schema、禁止字段、结构和循环；它不得创建节点、来源秘密或部分绑定。操作者必须在目的实例创建本地 `RebindDraft`，把每个 slot 绑定至当地库存选择器；绑定不得回写 Bundle

#### Scenario: 空实例导入
- **WHEN** 空白新实例导入有效 Bundle
- **THEN** 它得到不含节点和秘密的 UNBOUND 草稿，活动修订不变

#### Scenario: 未绑定发布
- **WHEN** 操作者预检仍含未绑定 slot 的导入草稿
- **THEN** 预检返回 slot 级诊断且不得创建或激活修订

#### Scenario: rebind 后发布
- **WHEN** 每个 slot 都绑定到存在的本地库存、预检成功且编译节点集合非空
- **THEN** 草稿可按通常发布流程发布，输出仅使用目的实例被绑定的节点

### Requirement: 秘密边界
系统 SHALL 将来源连接秘密、管理认证和节点秘密限制在部署本地加密存储。Bundle、响应、预检诊断、审计报告和日志不得读取、覆盖、恢复或回显这些材料

#### Scenario: 导入携带秘密字段
- **WHEN** Bundle 含来源认证、节点秘密或 token 字段
- **THEN** 导入拒绝该文档并提供不回显值的安全诊断

### Requirement: 重置确定性
系统 SHALL 通过预检并激活空白或历史逻辑修订完成重置，不得按固定表顺序清空后导入

#### Scenario: 重置后系统可用
- **WHEN** 操作者确认并激活已预检的空白或历史修订
- **THEN** 活动修订原子切换，历史、认证设置和来源秘密边界保持不变

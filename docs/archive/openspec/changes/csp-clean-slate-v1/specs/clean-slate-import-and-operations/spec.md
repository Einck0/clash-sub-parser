## Purpose

定义新 CSP SQLite 数据库、离线历史内容导入和受控运维的不可混淆边界，确保历史数据库被保留但不成为新系统的运行时依赖，也不使秘密数据进入诊断或前端。

## ADDED Requirements

### Requirement: 新 schema 与单一数据权威
系统 SHALL 使用由 CSP 1.0 自身管理的独立 SQLite 文件与版本化 schema。核心实体 MUST 至少包括 subscriptions、nodes、node_sources、probe_runs、probe_observations、node_groups、group_memberships、admission_rules、policy_rules、configuration_revisions、publications、settings 和 audit_events；所有跨实体引用 MUST 使用稳定逻辑 ID 或受外键约束的内部键，且运行时不得读取旧数据库表。

#### Scenario: 新运行时启动
- **WHEN** 新 CSP 1.0 以空的新数据卷启动
- **THEN** 系统创建或验证 CSP 1.0 schema 且不要求旧数据库文件存在或可访问

### Requirement: 历史导入为离线、一次性、待审草稿
系统 SHALL 只提供显式启动的离线导入命令，不向服务路由暴露自动导入。导入器 MUST 只读打开历史 SQLite 源，并将 allowlist 中的订阅元数据、非秘密节点逻辑、分组、规则和 DNS 逻辑内容转换为目标 schema 的待审草稿。导入内容 MUST 在授权管理者逐项或整体审查激活前不参与刷新、探测或导出。

#### Scenario: 导入完成但未审查
- **WHEN** 离线导入成功完成
- **THEN** 新系统显示带审计报告的待审草稿，且调度器和导出接口不使用这些草稿

### Requirement: 导入秘密排除与隔离报告
导入器 MUST 排除订阅 URL 中凭据、导出令牌、代理凭据、私钥、cookie、完整历史探测响应和未知 JSON 载荷。遇到无法按 allowlist 映射的字段或不合规记录时，导入器 MUST 将其放入隔离报告并继续处理其他独立记录；报告只包含字段类别、历史主键摘要和脱敏错误，不含秘密值。

#### Scenario: 历史记录含私钥
- **WHEN** 历史节点记录包含私钥字段
- **THEN** 私钥不写入新数据库或报告，记录以需要审查或隔离状态出现

### Requirement: 备份、恢复、切流和回滚
系统 SHALL 提供一致性备份与恢复验证步骤，备份后 MUST 验证可打开性和外键完整性。新服务 MUST 使用独立数据卷；生产切流 MUST 由人工批准，先验证新服务健康和发布物，再修改流量。回滚 MUST 仅恢复旧的已停止容器与原历史卷或恢复一份已验证的新备份，且不得通过双写或在线 schema 适配实现。

#### Scenario: 新服务健康检查失败
- **WHEN** 发布前新服务的 schema、健康端点或只读验证失败
- **THEN** 不执行流量切换，原历史卷保持未写入状态

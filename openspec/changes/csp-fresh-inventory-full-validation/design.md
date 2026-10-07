## Context

参见 `proposal.md`。目标数据库当前包含 1014 个节点（983 个历史失活节点，31 个活跃节点），9 个订阅源（4 enabled，5 disabled）。由于历史 `SanitizeSubscriptionURL` 缺陷，`7li` 与 `魔戒` 两个订阅的 URL 缺失 token 参数导致抓取返回 403。本设计定义节点库存与派生数据的原子清空闭包、源 Token URL 精准恢复以及维护 CLI 命令。

## Goals / Non-Goals

**Goals:**
- 实现严格事务性的节点清空闭包，使 `nodes` 及相关派生表记录数归 0，外键约束 0 违规。
- 保证不可变发布快照（`publications`）与发布所引用的载荷数据（`publication_payload_refs`）安全保留，并将关联条目的 `node_logical_id` 脱钩为 NULL。
- 保证所有非节点类业务资产（订阅配置、启停、分流分组、策略规则、风险策略、定时调度、设置）完整保留，哈希与结构不变。
- 仅通过冷归档稳定导入 ID 证据恢复 `7li` 与 `魔戒` 的完整 Token URL，保持其余 7 个订阅源及 enabled/disabled 状态不变。
- 在 `cmd/csp` 中提供 `reset-node-inventory` 维护子命令，复用生产 service 与 repository wiring，支持 dry-run 与 apply。
- 在只读隔离 SQLite 副本上完成自测演练，本轮不直接对生产环境数据执行破坏性修改或部署重启。

**Non-Goals:**
- 不修改 Web 前端界面，不新增免密 Web API 或旁路 HTTP 桥接。
- 不修改平台（platform）、队列（queue）、探针引擎执行器（probe runner/mihomo）底层行为（由其他专业机处理）。
- 不执行生产数据库的直接线上物理修改或重启上线。

## Decisions

### 决策 1: 外键敏感的逆序原子删除事务闭包
- **选择**: 在启用 `PRAGMA foreign_keys = ON` 的事务中，按照严格的拓扑逆序清空节点衍生表：
  1. `probe_observations`, `probe_batch_runs`, `probe_runs`, `probe_batches`
  2. `ip_risk_observations`（其对外键为 RESTRICT）
  3. `node_source_history`, `node_sources`, `node_overrides`
  4. `node_connection_heads`, `node_connection_versions`（heads 对 versions 外键为 RESTRICT）
  5. `group_edges` 中 `node_logical_id` 非空的边（清理或解除绑定）
  6. 解绑发布载荷条目：`UPDATE subscription_entries SET node_logical_id = NULL WHERE payload_id IN (SELECT payload_id FROM publication_payload_refs);`
  7. 清理未被发布引用的旧原始条目与载荷：`DELETE FROM subscription_entries WHERE payload_id NOT IN (SELECT payload_id FROM publication_payload_refs);`，`DELETE FROM subscription_payloads WHERE id NOT IN (SELECT payload_id FROM publication_payload_refs);`
  8. 清理未引用抓取或断开成员指针：`DELETE FROM subscription_fetches WHERE id NOT IN (SELECT fetch_id FROM subscription_payloads);`，留存的 fetches 严格保留原始历史审计事实（`nodes_parsed` 与 `nodes_valid` 计数保持不变，严禁置 0 伪造审计），通过清空 `node_sources` 切断历史 last-good 指针
  9. 删除 `nodes` 表全部记录：`DELETE FROM nodes;`
  10. 事务内执行 `PRAGMA foreign_key_check` 校验，若存在违规立即回滚；在删除前提取所有指向节点的 `group_edges`，构建基于 `ComputeConnectionLogicalID` 的精确重绑计划，持久化到 0600 私密维护清单。
- **备选方案**:
  - 关闭外键检查强制级联：违背严谨工程原则，易造成隐藏孤儿数据。
  - 标记 tombstone 保留旧节点：不满足用户彻底清空库存的要求。

### 决策 2: 基于冷归档稳定身份证据的双源精准恢复与幂等性
- **选择**: 读取冷归档数据库中的 `subscriptions` 与 `sources` 表，仅针对 `enabled = 1`、基础 URL (scheme+host+path) 与非 token 查询参数完全一致的源进行唯一无歧义匹配。仅针对已证实丢失 Token 的裸 URL 恢复 `source_url_secret_ref`。若数据库中当前 URL 已包含有效 Token 或与归档一致，则直接跳过，具备严格幂等性，不产生虚假 revision 递增或重复审计。
- **脱敏原则**: 在日志、控制台输出和对外 JSON 报告中，仅输出 `restored: true` 与脱敏后域名路径，包含路径参数中的敏感 Token 亦自动掩码，绝不打印包含 Token 的明文字符串。

### 决策 3: 一次性维护 CLI (`csp reset-node-inventory`) 与备份硬约束
- **选择**: 扩展 `cmd/csp/main.go`，增加 `reset-node-inventory` 命令。
  - 支持参数：`--db`, `--dry-run`, `--apply`, `--confirm-backup`, `--backup-file`, `--restore-source-tokens`, `--archive-db`。
  - `--apply` 模式强制校验备份确认：必须显式传入 `--confirm-backup` 或提供真实可读且 header 为 SQLite 格式的 `--backup-file`。
  - 任何执行模式（dry-run 或 apply）均执行前置外键完整性预检，前置脏外键直接拒绝。
  - 内部直接复用现有 SQLite 仓库与服务，不新建旁路 HTTP 接口。

## Risks / Trade-offs

- **[Risk] 发布 payload 引用旧条目导致的外键阻断**
  → **Mitigation**: 显式将已发布 payload 条目的 `node_logical_id` 置 NULL，保持载荷内容不变的同时避免指向已删除节点的外键冲突。
- **[Risk] 误修改其他订阅配置或误激活未启用订阅**
  → **Mitigation**: 严格限定仅对 `7li` 与 `魔戒` 两个特定 ID 执行更新，前置与后置断言其余 7 个源的 `enabled` 状态与配置不变。
- **[Risk] 敏感 Token 泄露至日志或输出**
  → **Mitigation**: 采用 `domain.RedactSensitiveInfo` 和布尔型报告结构，任何打印与返回数据均脱敏。

## Purpose

规范 CSP 节点库存及所有历史衍生表数据的彻底原子清空闭包规程，确保在执行节点库存重置时，物理节点与历史观测记录彻底归零，同时完整保护不可变发布快照与未变更业务配置资产，保证外键约束完整无挂起。

## ADDED Requirements

### Requirement: 节点与派生数据原子全量清空
系统 SHALL 提供严格事务性的节点库存彻底清空闭包。当触发节点重置时，系统 MUST 在单一原子事务中将 nodes 表以及所有依赖于 nodes 的衍生表清空（行数严格归 0），包括 `nodes`、`node_sources`、`node_source_history`、`node_connection_heads`、`node_connection_versions`、`node_overrides`、`probe_observations`、`probe_runs`、`probe_batches`、`probe_batch_runs` 和 `ip_risk_observations`。重置操作 MUST 彻底物理清除历史遗留节点，严禁采用保留墓碑状态或伪置无效而实际保留节点的方案。

#### Scenario: 执行重置后节点与衍生表计数清零
- **WHEN** 在目标数据库上执行节点库存重置事务并成功提交
- **THEN** `nodes`、`node_sources`、`node_source_history`、`node_connection_heads`、`node_connection_versions` 及所有探针、IP风险观测表的行数均为 0

#### Scenario: 事务异常时完整回滚不造成部分数据丢失
- **WHEN** 重置事务在执行过程中由于锁超时或受控注入错误而发生失败
- **THEN** 数据库事务完全回滚，既有节点与历史记录行数与内容维持不变

### Requirement: 不可变发布快照与已发布 payload 保护
系统 SHALL 严格保护既有不可变发布快照（`publications`）与发布所引用的原始载荷（`publication_payload_refs`）。在重置过程中，系统 MUST 保留已被 `publication_payload_refs` 引用的 `subscription_payloads` 记录及其原始内容；对于这些已发布载荷下的 `subscription_entries`，系统 MUST 将其关联的 `node_logical_id` 脱钩设置为 NULL，确保不持有指向已删除节点的悬挂外键。对于未被任何发布引用的旧原始载荷与条目，系统 MUST 予以物理删除。

#### Scenario: 存在已发布载荷时保留载荷内容并解绑节点外键
- **WHEN** 数据库包含已被 publications 引用锁定的 subscription_payloads 记录并触发节点重置
- **THEN** 系统保留该 payload 记录，将其下 subscription_entries 的 node_logical_id 设置为 NULL，且外键校验无违规

#### Scenario: 未被发布的历史载荷与条目彻底清除
- **WHEN** 存在未关联任何 publication 的旧 subscription_payloads 和 subscription_entries
- **THEN** 重置事务将其全量删除，不留无用旧载荷数据

### Requirement: 断开旧抓取记录成员指针与历史审计事实保持
系统 SHALL 在重置事务中清理未绑定发布的无用旧抓取记录，并通过清空 `node_sources` 表彻底切断旧节点集合与上游抓取的成员关联指针。对于留存的不可变抓取记录（`subscription_fetches`），系统 MUST 严格保持其不可变的历史审计事实（`nodes_parsed` 与 `nodes_valid` 原始计数保持不变，严禁清零伪造审计）。当重置后后续抓取失败时，系统 MUST NOT 回退或重新激活任何旧节点库存。重置操作 MUST 具备幂等性，连续多次执行必须均能安全完成且结果一致。

#### Scenario: 留存抓取记录保持原始审计计数不变
- **WHEN** 存在与已发布载荷关联的留存 subscription_fetches 记录并触发节点重置
- **THEN** 该抓取记录的 nodes_parsed 与 nodes_valid 维持其历史抓取时的真实数值不变

#### Scenario: 连续多次执行重置具备幂等性
- **WHEN** 在已重置为空的数据库上再次执行节点库存重置
- **THEN** 操作成功完成，相关节点表仍为 0，外键检查无任何错误

### Requirement: 策略分组节点边解绑与受保护私密重绑计划
系统 SHALL 在重置事务中识别并清理显式指向节点的 `group_edges`。在删除前，系统 MUST 提取所有受影响的节点绑定配置（协议、端点、传输参数），生成严格基于连接身份（`ComputeConnectionLogicalID`）的重绑计划（Rebind Plan）与诊断信息，严禁仅按节点名称或端点模糊自动重绑，亦严禁未经授权自动回退到 DIRECT。在 apply 执行模式下，若存在受影响绑定，系统 MUST 将其持久化至文件系统受保护的私有维护清单（目录权限 0700，文件权限 0600）。

#### Scenario: 显式节点边解绑并生成精确重绑计划
- **WHEN** 存在指向具体 node_logical_id 的 group_edges 并执行重置
- **THEN** 对应 group_edges 被安全清理避免悬挂外键，报告包含 impacted_bindings_count 与 exact_connection_id 映射计划，并在 apply 时输出 0600 权限的私有维护清单文件路径

#### Scenario: 策略分组与业务配置资产完全保留
- **WHEN** 执行节点库存重置
- **THEN** `subscriptions`、`node_groups`、`group_edges`（组间关系）、`policy_rules`、`admission_rules`、`global_node_filters`、`settings` 等业务配置行数与配置保持完全一致

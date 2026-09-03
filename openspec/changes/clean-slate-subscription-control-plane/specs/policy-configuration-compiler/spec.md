## Purpose

定义可移植逻辑配置、本地 rebind 和确定性编译，使 Bundle 不引用空实例不存在的库存节点

## ADDED Requirements

### Requirement: 版本化逻辑配置和 selector slot
系统 SHALL 将策略、规则、DNS、发布选项和有序 `selector_slot` 保存为逻辑配置。Bundle 中 slot 只表达选择语义与顺序，所有 Bundle 引用使用逻辑 ID，但不得引用 Source 或 InventoryNode logical ID

#### Scenario: Bundle 无库存引用
- **WHEN** 操作者检查导出的 Bundle
- **THEN** selector slot 只含可移植选择语义，不含任何来源或 InventoryNode ID

### Requirement: 本地 rebind 预检
导入 Bundle SHALL 创建 UNBOUND 草稿；目的实例的 RebindDraft 必须为每个 slot 显式给出本地库存选择器。预检 SHALL 拒绝未绑定 slot、缺失本地目标、循环、无效表达式和空编译结果，且不影响活动修订

#### Scenario: 跨新实例导入
- **WHEN** 目的实例没有任何 InventoryNode 时导入 Bundle
- **THEN** 导入成功为 UNBOUND 草稿，不生成未解析节点引用，也不允许发布

#### Scenario: 本地绑定后编译
- **WHEN** 操作员完成本地 rebind 并通过预检
- **THEN** 编译只使用目的实例库存，且解释列出 slot、选择器和包含/排除原因

### Requirement: 确定性编译与四目标渲染
系统 SHALL 根据已发布修订、明确库存世代、观测规则和目标生成确定性语义图。Clash、Mihomo、Stash、Shadowrocket SHALL 仅渲染同一语义图；不得提供 Script.js 或旧格式适配器

#### Scenario: 相同输入重复编译
- **WHEN** 两次编译使用相同修订、库存世代、观测规则和 target
- **THEN** 节点顺序与语义 fingerprint 完全相同

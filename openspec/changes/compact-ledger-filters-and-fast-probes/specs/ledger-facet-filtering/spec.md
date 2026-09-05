## Purpose

为节点台账提供紧凑、可访问的分面多选筛选，使操作者按业务维度组合条件而不被持续平铺的标签占用工作空间。

## ADDED Requirements

### Requirement: Collapsible facet selection
节点台账 SHALL 将地区、协议、健康状态、链路和解锁能力呈现为独立的可展开筛选分面，而不是持续显示每个候选项的 Chip。每个分面 SHALL 显示其标题、已选数量和可访问的展开状态，且在窄屏上不得引入文档水平滚动。

#### Scenario: Closed filter bar preserves workspace
- **WHEN** 操作者首次打开节点台账且未展开任何分面
- **THEN** 筛选栏仅显示各维度入口、已选条件摘要与现有搜索和操作控件，不显示完整地区或平台候选列表

#### Scenario: Keyboard accessible facet control
- **WHEN** 操作者通过键盘聚焦某个分面入口并使用 Enter、Space 或 Escape
- **THEN** 控件 SHALL 分别展开、切换或关闭该分面，并保持可见焦点和正确的 aria-expanded 状态

### Requirement: Composable facet multi-selection
节点台账 SHALL 支持地区、协议、健康状态和解锁能力各自多选。同一维度内任一选中值匹配即可通过；不同维度、订阅、链路、速度门槛和关键词之间 SHALL 同时满足。解锁能力仍 SHALL 仅接受已确认的完整解锁结果。

#### Scenario: Two countries and two media capabilities
- **WHEN** 操作者选中香港和日本，并选中 Netflix 和 ChatGPT
- **THEN** 结果 SHALL 仅包含出口国家为香港或日本且同时完整解锁 Netflix 与 ChatGPT 的节点

#### Scenario: Empty facet means no constraint
- **WHEN** 某个分面没有选中任何候选值
- **THEN** 该分面 SHALL 不排除任何节点

### Requirement: Active-filter summary and reset
节点台账 SHALL 在不展开候选列表的情况下显示当前各分面的已选值或数量，并允许操作者逐项移除已选值或一次性清空所有筛选条件。清空 SHALL 保留现有的默认排序和未选中状态语义。

#### Scenario: Clear an individual facet value
- **WHEN** 操作者从活动筛选摘要移除一个已选协议
- **THEN** 仅该协议从协议集合中移除，其余分面与筛选条件保持不变

#### Scenario: Reset all filters
- **WHEN** 操作者执行清空所有筛选条件
- **THEN** 所有多选集合、关键词、订阅、链路和速度门槛恢复为空或默认值，筛选结果恢复为未约束台账

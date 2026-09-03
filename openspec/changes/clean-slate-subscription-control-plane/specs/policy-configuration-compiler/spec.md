## MODIFIED Requirements

### Requirement: 策略组与动态成员语义
系统 SHALL 保留 select、url-test、fallback 与 load-balance 策略组。组成员 MUST 支持显式节点、嵌套组、组节点展开、排除项及 regex 虚拟条目；regex 在预览和每次编译时动态解析，不得冻结为静态节点列表

#### Scenario: 新节点匹配既有 regex
- **WHEN** 来源刷新产生名称匹配策略组 regex 的新节点
- **THEN** 无需编辑该策略组，新节点在下一次预览/编译中按规则出现

### Requirement: 策略质量条件与跳板绑定
策略组 SHALL 支持最低速度和服务能力条件；ProxyChainBinding SHALL 可将节点或策略组解析为前置 Dialer Proxy，并拒绝循环或无效目标。预览与发布 MUST 使用同一组成员解析规则

#### Scenario: 绑定策略组跳板
- **WHEN** 操作者为策略组保存一个有效跳板绑定
- **THEN** 编译输出对该组解析出的叶子节点应用前置链，且台账显示绑定状态

### Requirement: 规则集与分类顺序
系统 SHALL 保留规则分类、拖拽/显式排序、启用状态、目标策略组和 DOMAIN、DOMAIN-SUFFIX、DOMAIN-KEYWORD、IP-CIDR、GEOIP、PROCESS-NAME、MATCH 规则类型。编译器 MUST 按已保存分类和规则顺序生成目标配置

#### Scenario: 调整分类规则顺序
- **WHEN** 操作者改变规则分类或条目的顺序并保存
- **THEN** 预览与五个目标输出以新顺序生成，而未启用项不进入输出

### Requirement: 唯一 canonical graph 与五目标渲染
系统 SHALL 从同一 canonical graph 渲染 Clash、Mihomo、Stash、Shadowrocket 和 Sing-box。每个 renderer 可处理目标格式差异，但节点/策略/规则选择必须一致，且不得提供 Script.js renderer

#### Scenario: 比较多目标导出
- **WHEN** 同一已预检配置分别编译五个支持目标
- **THEN** 各输出具有目标格式所需差异，但使用同一选中节点、策略组、规则和 DNS 语义
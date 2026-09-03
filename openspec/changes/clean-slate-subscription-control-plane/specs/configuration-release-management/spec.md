## MODIFIED Requirements

### Requirement: 配置修订、预检与快照回滚
系统 SHALL 保留配置预检、活动配置、ConfigSnapshot 和历史恢复能力。预检 MUST 在激活前验证节点选择、策略组循环、regex、跳板、规则引用及五目标编译；失败不得改变当前活动配置

#### Scenario: 预检失败
- **WHEN** 配置含循环策略组或无法解析的节点选择
- **THEN** 系统返回领域级诊断，活动配置和已有快照保持不变

### Requirement: 五目标发布和快捷导入
系统 SHALL 发布 Clash、Mihomo、Stash、Shadowrocket 与 Sing-box 配置，并保留单订阅/合并订阅链接、客户端 URL scheme 和二维码。每个输出只包含当前配置选中的节点和对应目标所需字段

#### Scenario: 扫码导入 Stash
- **WHEN** 操作者在 Quick Export 选择 Stash 和合并订阅
- **THEN** 页面生成正确的订阅 URL、Stash Scheme 和二维码，下载内容与预览使用同一修订

### Requirement: 发布可追溯性与回滚
每次生成/下载 SHALL 关联配置修订和语义指纹，快照恢复 SHALL 作为新的可审计活动变更而非覆写历史。发布或恢复期间失败不得破坏上一个可用配置

#### Scenario: 回滚配置快照
- **WHEN** 操作者选择已保存快照并通过预检恢复
- **THEN** 系统激活可追溯版本，五目标下载切换到该版本且旧快照仍可查看

### Requirement: SCRIPT 不属于发布目标
发布目标列表和历史输出 MUST 不包含 Script.js。移除脚本能力不得删除任一 YAML、订阅 URL、Scheme 或二维码分发功能

#### Scenario: 查看导出目标
- **WHEN** 操作者打开 Quick Export
- **THEN** 仅显示五个支持目标及其分发方式，不显示 Script.js 入口
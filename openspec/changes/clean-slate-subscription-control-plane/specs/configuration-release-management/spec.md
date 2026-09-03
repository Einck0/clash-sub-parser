## Purpose

提供草稿到预检、发布、回滚和四目标安全发布的完整生命周期

## ADDED Requirements

### Requirement: 预检优先发布
系统 SHALL 在发布前验证 Bundle 禁止字段、selector slot 的完整 rebind、结构、引用、循环、非空编译与秘密泄露。失败不得创建或激活修订，也不得回显秘密

#### Scenario: 未绑定 slot 的预检
- **WHEN** 草稿仍含 UNBOUND selector slot
- **THEN** 预检返回 slot 级诊断且活动修订不变

### Requirement: 原子激活与回滚
系统 SHALL 原子激活一个新修订或已验证历史修订。任意时刻只存在一个活动修订；回滚记录操作者与时间但不改写原修订

#### Scenario: 回滚已验证修订
- **WHEN** 操作者选择一个已验证历史修订回滚
- **THEN** 服务原子激活该修订，且不会改写其内容

### Requirement: 有界修订留存
系统 SHALL 仅清理未激活且未保护的修订；活动、保护和回滚目标不得自动删除

#### Scenario: 达到留存上限
- **WHEN** 发布使历史数量超过保留上限
- **THEN** 仅清理最旧的不受保护非活动修订

### Requirement: 安全目标发布
每份四目标输出 SHALL 标识 revision 和 semantic fingerprint，并只经受限发布 token 暴露当前已选节点的客户端必需连接材料。来源/管理秘密、未选节点、Bundle、token、观测原始响应不得出现在输出、缓存、日志或管理响应

#### Scenario: 发布内容最小化
- **WHEN** 有效客户端请求任一受支持 target
- **THEN** 返回内容仅足以配置该 target 的已选节点，不授予管理权限或枚举其他库存

## MODIFIED Requirements

### Requirement: 已授权来源的受限拉取
订阅拉取 SHALL 仅由已保存且 enabled 的 Source 在自身授权版本和预算内发起。每个请求和重定向前必须检查 Source 当前状态、协议、DNS/地址安全、重定向深度、超时和剩余字节；禁用、配置变更、取消或预算耗尽不得派发下一次请求

#### Scenario: 授权被撤销
- **WHEN** Source refresh Job 在排队或运行时被禁用、变更、取消或耗尽预算
- **THEN** 终止后续请求，保留此前成功修订，不创建新修订

### Requirement: 重定向逐跳校验
订阅拉取 SHALL 在每一次重定向前执行相同 URL、DNS 和地址安全校验。失败不得发布新来源修订

#### Scenario: 重定向到内网地址被拒绝
- **WHEN** 重定向目标为环回、私网或禁止地址
- **THEN** 系统停止请求、记录已脱敏安全拒绝并保持上次成功修订

#### Scenario: 重定向缺失 Location
- **WHEN** 上游返回没有 Location 的重定向响应
- **THEN** 系统终止刷新并返回结构化上游错误

### Requirement: 响应字节限长
系统 SHALL 在流式读取时强制 Source 字节上限。超限内容不得成为来源修订、库存或后续请求可消费的部分文件

#### Scenario: 超大文件截断
- **WHEN** 响应超过允许字节上限
- **THEN** 系统中止读取、记录失败且不改变当前修订

### Requirement: 重定向深度上限
系统 SHALL 限制重定向深度，并将超限记录为已脱敏刷新失败

#### Scenario: 重定向循环
- **WHEN** 上游构造超过深度上限的循环
- **THEN** 系统结束请求并返回重定向超限错误

## REMOVED Requirements

### Requirement: 双路径行为一致
**Reason**: 新系统仅将授权受限拉取作为来源输入，不保留旧资产下载兼容路径

**Migration**: 旧资产下载调用方不会迁移到新控制平面

## Purpose

定义来源和手工来源如何在授权、预算与本地秘密边界内产生不可变输入修订和可追溯库存

## ADDED Requirements

### Requirement: 来源授权与受限刷新
每个远程 Source SHALL 有版本化的 `enabled` 状态、允许操作、HTTP/DNS/重定向/并发/字节/时长预算和审计版本。只有已保存且 enabled 的 Source 可刷新；每次网络尝试前 SHALL 检查当前授权与剩余预算。保存或启用是产品内持久授权，不是生产确认

#### Scenario: 授权撤销或预算耗尽
- **WHEN** Source 被禁用、删除、配置版本改变、Job 取消或预算耗尽
- **THEN** 不派发后续请求；排队/执行 Job 以 `authorization_revoked` 或 `budget_exhausted` 终态结束，在途请求仅取消或收尾且不再尝试

### Requirement: 来源与不可变修订
系统 SHALL 将远程订阅或手工节点集合保存为 Source；完整读取、解析、规范化和预算检查成功后才原子创建 SourceRevision。失败不得替换最后成功修订

#### Scenario: 成功和失败刷新
- **WHEN** 刷新分别完成或失败
- **THEN** 成功时创建新不可变修订，失败时保留此前成功修订

### Requirement: 协议规范化和秘密存储
协议 adapter SHALL 声明 public 与 secret 字段。public 字段采用版本化 canonical JSON；完整连接材料只存在解析内存和每记录随机 nonce、key id 的 AEAD `EncryptedSecret`。库存 dedup SHALL 使用覆盖 public 和 secret canonical 字段的版本化 HMAC，不得保存或回显原文

#### Scenario: 指纹边界
- **WHEN** 两个 payload 仅字段顺序不同
- **THEN** 它们稳定归并并保留全部来源关联

#### Scenario: 同端点凭据或协议不同
- **WHEN** 节点地址相同但协议、密码、UUID 或私钥不同
- **THEN** 它们绝不归并，且 API、日志、Bundle 和诊断不出现差异秘密

### Requirement: 统一来源可追溯性
系统 SHALL 保留 SourceRevisionNode 关联和历史出现记录，手工节点也必须成为手工 SourceRevision，不能绕过库存、授权或秘密契约

#### Scenario: 两个来源含相同节点
- **WHEN** 两个来源修订产生同一规范化节点
- **THEN** 系统保留一个库存节点和两个来源关联，且不丢失历史出现记录

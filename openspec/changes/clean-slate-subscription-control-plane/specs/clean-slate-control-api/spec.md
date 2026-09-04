## MODIFIED Requirements

### Requirement: 保留领域管理 API 与渐进演进
系统 SHALL 继续提供订阅、来源、节点、探测、策略组、跳板链、规则分类、规则、DNS、生成、设置、快照和下载的既有业务操作。重构后的 router SHALL 将请求映射至领域服务而非直接访问存储，并保持现有客户端已使用的资源字段、筛选语义和操作结果；新增 DTO 不得删除尚未迁移客户端依赖的功能

#### Scenario: 现有管理流程请求
- **WHEN** 现有控制台执行订阅刷新、节点筛选、策略保存或配置预览
- **THEN** API 返回与既有业务语义等价的结果，且由对应领域服务完成操作

### Requirement: 可查询且可取消的异步命令
刷新与探测 SHALL 创建含状态、进度、错误分类和取消语义的 Job。系统 MUST 防止同一目标的重复运行，并在取消后不派发下一节点或下一网络尝试

#### Scenario: 取消批量探测
- **WHEN** 操作者取消仍在运行的批量探测 Job
- **THEN** 系统报告确定终态、回收运行资源并保留已经完成的结果

### Requirement: SCRIPT 功能终止
系统 MUST 不再生成、执行、下载或在管理界面展示 Script.js；`/script` 及其下载变体 SHALL 返回 404，且不触发编译、存储写入或网络操作

#### Scenario: 请求已移除脚本
- **WHEN** 调用方访问旧 SCRIPT 路径或在导出参数中选择 script
- **THEN** 系统返回不可用结果，其他配置导出能力不受影响

### Requirement: 管理响应秘密最小化
管理列表、详情、Job 和迁移诊断 SHALL 不回显上游认证、节点私钥/密码、管理 token 或完整敏感载荷；配置下载只可暴露被选节点在对应客户端运行所必需的材料

#### Scenario: 查看节点详情
- **WHEN** 操作者在 NodeLedger 打开节点详情
- **THEN** 可查看协议公开摘要、探测和可复制的目标配置预览，但管理响应不泄露来源认证或无关节点秘密

## ADDED Requirements

### Requirement: NodeLedger 服务端查询窗口
系统 SHALL 为 NodeLedger 提供基于稳定逻辑 `node_id` 的查询窗口，支持关键字、来源、协议、探测状态、跳板状态、最低速度、能力条件和排序的组合筛选。每个响应 MUST 返回公开节点/探测摘要、当前总数、指标统计、可用筛选 facets、稳定 snapshot 标识及可继续读取的 cursor 或等价窗口信息；不得要求控制台为获得这些结果而下载全量节点、探测和跳板记录

#### Scenario: 带组合筛选的连续窗口读取
- **WHEN** 操作者按能力和速度筛选节点并继续浏览下一虚拟窗口
- **THEN** 返回集合、总数和 facets 与同一 snapshot 的排序一致，已选择的 `node_id` 在返回窗口变化后仍可用于后续批量命令

#### Scenario: 查询 snapshot 过期
- **WHEN** 控制台使用已失效的 cursor 或其查询期间节点集合已发生影响排序的变化
- **THEN** API 返回可识别的过期结果或新的 snapshot 信息，控制台可从首窗口安全重载而不将旧响应覆盖新查询

### Requirement: Snapshot 范围批量探测命令
系统 SHALL 接受显式 `node_id` 集合、当前窗口集合或查询 snapshot 加排除集合三种探测目标表达。对于查询 snapshot，系统 MUST 在创建 Job 时验证 query fingerprint、解析并持久化不可变目标集合，返回目标计数和 Job 标识；后续节点刷新、游标读取或前端筛选变化不得扩大、缩小或替换该 Job 的目标集合

#### Scenario: 冻结全筛选探测范围
- **WHEN** 控制台确认针对一个查询 snapshot 的全部匹配结果执行探测，并排除若干 `node_id`
- **THEN** Job 只探测创建时解析出的匹配结果减排除集，进度、取消和结果均以该不可变集合计数

### Requirement: QuickExport 发布结果契约
系统 SHALL 为单订阅与合并订阅提供关联配置修订的 Clash、Mihomo、Stash、Shadowrocket、Sing-box 发布结果。每个结果 MUST 提供对应订阅 URL、适用的客户端 Scheme 和可编码为二维码的内容，且不得返回 SCRIPT 选项或未编译的节点秘密

#### Scenario: 请求客户端快捷导入结果
- **WHEN** 已认证控制台为指定发布范围请求 Stash、Shadowrocket 或其他支持目标的 QuickExport 结果
- **THEN** API 返回与该配置修订一致的 URL、Scheme 和二维码负载，目标切换不改变已选订阅范围

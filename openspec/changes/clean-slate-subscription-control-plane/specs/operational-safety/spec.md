## MODIFIED Requirements

### Requirement: 不破坏旧生产资产的实施边界
实现、测试和迁移演练 SHALL 不修改旧 SQLite 生产卷、legacy snapshot/tag 或正在服务的旧入口。新实现可以读取经批准的快照副本和 fixture；任意生产切换、容器停止、卷写入或入口变更必须在实现验收后单独确认

#### Scenario: 演练迁移
- **WHEN** 工程人员运行 SQLite 到目标库的迁移演练
- **THEN** 工具只访问副本并在新目标数据库/namespace 写入，旧容器和生产卷无变化

### Requirement: 功能等价和双轨门禁
系统 SHALL 维护完整 feature-parity matrix，覆盖所有除 SCRIPT 外的遗留功能、legacy 证据、目标实现、测试和回退路径。新实现不能证明某高风险领域等价时 MUST 迁入该领域的已验证 legacy 核心模块及测试，而非裁剪功能

#### Scenario: 协议探测等价未通过
- **WHEN** 全新探针实现未通过一个协议或真实出口回归
- **THEN** 发布门禁阻止切换，团队采用保底轨道迁入旧探针并重新验证

### Requirement: 可演练切换与回退
上线前 SHALL 在独立环境完成目标库恢复、五目标导出、真实经节点探测、前端构建和旧入口回退演练。正式切换必须先完成最终一致性审计；失败时 MUST 恢复旧入口且不写回旧 SQLite

#### Scenario: 切换健康检查失败
- **WHEN** 新入口在切换后健康检查、数据抽样或关键功能验证失败
- **THEN** 流量恢复至旧服务，目标库保留为排查副本，旧数据库和 legacy 快照不被篡改

### Requirement: 上线验收清单
生产切换前 MUST 通过后端测试、前端类型/构建、迁移审计、协议/编译 golden、NodeLedger 关键 E2E、真实出口探测与独立代码审查。SCRIPT 移除验证必须通过，但不得作为其他能力缺失的豁免

#### Scenario: 验收存在未覆盖功能
- **WHEN** feature-parity matrix 有任何未验收的非 SCRIPT 条目
- **THEN** 系统不得申请入口切换或旧服务退役
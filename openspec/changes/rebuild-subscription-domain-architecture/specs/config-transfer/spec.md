## MODIFIED Requirements

### Requirement: 完整导出
导出 SHALL 生成 versioned-configuration-bundles 约定的单个 JSON 配置包，包含 schema 版本、逻辑标识、配置修订元数据以及重建支持输出所需的全部非敏感配置。导出 SHALL NOT 依赖数据库行 ID，且 SHALL NOT 包含 token_hash、认证 token、订阅 URL 中的凭据或节点私密参数。

#### Scenario: 全量导出
- **WHEN** 调用导出接口且数据库含订阅、节点组、规则、DNS、策略与配置修订数据
- **THEN** 返回的 JSON 包含完整的逻辑配置图、版本标记与安全元数据，可在另一实例上经预检后导入并重建等价配置

### Requirement: 导入原子性
导入 SHALL 先执行不写库的版本、引用、字段和编译预检；只有预检通过后才以单个配置修订转换完成持久化。任一步骤失败时 SHALL 整体回滚到导入前的活动修订，不留部分写入。

#### Scenario: 中途失败整体回滚
- **WHEN** 导入包中某逻辑引用无效、字段不受支持或编译验证失败
- **THEN** 数据库活动配置与导入前完全一致，响应返回明确的失败原因与安全诊断

### Requirement: 认证凭据保留
导入 SHALL NOT 覆盖 security_settings 中的 token_hash、认证 token、订阅凭据或本机专属秘密。此类值 SHALL 只通过独立的秘密管理操作变更。

#### Scenario: 导入不含凭据覆盖
- **WHEN** 导入一个有效配置包到已启用认证的实例
- **THEN** 当前系统的登录凭据保持不变，导入后原 token 仍可登录

### Requirement: 重置确定性
重置 SHALL 创建并激活一个明确的空初始配置修订，而非按物理表固定顺序直接清空数据。重置 SHALL 保留本机认证秘密并允许系统无需重启继续服务。

#### Scenario: 重置后系统可用
- **WHEN** 执行重置操作并确认预检
- **THEN** 新活动修订只含初始配置、security_settings 中的本机认证秘密保持有效、系统可继续生成和管理配置
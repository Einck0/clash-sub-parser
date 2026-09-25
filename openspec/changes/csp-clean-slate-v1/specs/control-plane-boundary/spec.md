## Purpose

定义 CSP 1.0 唯一的外部控制面边界，使管理、发布和诊断请求具有稳定的版本、身份、错误语义与资源保护，同时显式拒绝已经废弃的旧接口。

## ADDED Requirements

### Requirement: 版本化控制面与统一错误
系统 SHALL 仅将 `/api/v1/` 作为管理控制面的版本化根路径。所有 JSON 成功响应 MUST 包含 `data`；所有失败响应 MUST 包含稳定机器码 `code`、安全的人类消息 `message` 与请求关联号 `request_id`，且 MUST 不含令牌、订阅 URL、节点凭据或完整生成配置。

#### Scenario: 已验证管理请求成功
- **WHEN** 已认证调用者请求一个存在的 `/api/v1/` 资源
- **THEN** 系统返回对应 HTTP 成功状态与 `{ "data": ... }` JSON 响应

#### Scenario: 无效输入被拒绝
- **WHEN** 调用者提交不满足资源约束的 JSON 请求
- **THEN** 系统返回 422 与稳定 `code`，且不写入部分资源

### Requirement: 身份、授权与发布令牌隔离
系统 SHALL 默认拒绝未认证的管理 API 和管理界面请求。系统 MUST 将管理会话或 bearer 凭据与导出发布令牌分离存储、哈希和审计；导出令牌 MUST 仅授权其绑定发布物，不得访问管理 API。所有改变状态的 cookie 会话请求 MUST 通过 CSRF 校验。

#### Scenario: 导出令牌不能管理资源
- **WHEN** 调用者使用有效导出令牌请求 `/api/v1/` 管理资源
- **THEN** 系统返回 401 或 403，且不泄漏资源存在性

#### Scenario: 缺少 CSRF 的会话写入
- **WHEN** 已登录浏览器会话提交缺少有效 CSRF 证明的状态变更请求
- **THEN** 系统拒绝请求且不改变数据

### Requirement: 显式拒绝旧端点
系统 MUST 对 `/yaml`、`/script` 与任何未版本化旧 `/api/` 路径返回 410，错误码为 `legacy_endpoint_removed`；系统 MUST 不提供重定向、别名、请求转换或默认兼容输出。

#### Scenario: 旧 YAML 地址被访问
- **WHEN** 客户端请求 `/yaml`
- **THEN** 系统返回 410 与 `legacy_endpoint_removed`

### Requirement: 可控资源消耗与审计
系统 SHALL 对请求体、分页大小、导出编译、订阅抓取、导入和探测提交执行明确上限。系统 MUST 为每次变更、导入、发布与探测命令记录操作主体、请求关联号、结果和脱敏摘要。

#### Scenario: 超过分页上限
- **WHEN** 调用者请求大于服务公布上限的 `page_size`
- **THEN** 系统返回 422 与允许的上限，且不执行无界查询

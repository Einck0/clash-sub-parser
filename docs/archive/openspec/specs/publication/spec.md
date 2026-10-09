# Publication Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的发布导出与客户端编译器能力。系统必须支持面向不同客户端生态的多目标编译：为原生 Mihomo 输出完整的代理、策略组拓扑与分流规则配置；为 Sing-box、Surge 及 Quantumult X 严格输出纯节点出站配置；并提供受控的发布端点，支持免密公开访问、专属发布令牌与统一主令牌双模态鉴权，以及历史短链接兼容。

## User Scenarios & Priority

### User Story 1 - 原生 Mihomo 完整分流拓扑导出 (Priority: P1)
- **GIVEN** 管理员配置了订阅、分流规则及具有直接引用的多层级策略组
- **WHEN** 客户端请求 Mihomo 格式的发布链接
- **THEN** 编译器生成包含 proxies、proxy-groups（含完整组嵌套）与 rules 的合法 YAML 配置

### User Story 2 - 非 Mihomo 客户端纯节点导出 (Priority: P1)
- **GIVEN** 管理员创建了针对 Sing-box 或 Surge 的发布项
- **WHEN** 客户端请求该发布端点
- **THEN** 编译器仅提取已准入节点并格式化为目标规范，完全忽略策略组与规则，不输出空壳结构

### User Story 3 - 发布端点免密与双令牌灵活拉取 (Priority: P1)
- **GIVEN** 管理员配置了发布项且系统启用了统一管理令牌
- **WHEN** 导出开关设为免密公开时，匿名客户端直接获取；设为受控时，客户端既可凭专属 TokenHash 亦可凭统一令牌拉取
- **THEN** 发布端点准确校验凭据并交付编译配置

---

## Requirements

### Requirement: 原生 Mihomo 完整拓扑发布导出 [FR-PUB-01]
系统 SHALL 支持原生 Mihomo（Clash.Meta）编译目标。编译器 MUST 正确渲染基础全局配置、proxies 节点列表、proxy-groups 策略组拓扑（包括 fallback、loadbalance、select 及子组直接嵌套）与分流 rules。

#### Scenario: 复杂策略组拓扑导出
- **WHEN** 请求 target=mihomo 的发布端点
- **THEN** 输出标准 YAML 文本，且 proxies 与 proxy-groups 语法完全通过 Mihomo 客户端加载校验

### Requirement: 非 Mihomo 纯节点导出协议边界 [FR-PUB-02]
系统 SHALL 支持 Sing-box、Surge 与 Quantumult X 编译目标。这些目标 MUST 严格遵循纯节点导出协议边界，仅将已准入节点转换为对应的 JSON 或行文本格式，不输出任何策略组或分流规则。

#### Scenario: Sing-box 纯节点导出
- **WHEN** 请求 target=singbox 的发布端点
- **THEN** 输出包含 outbounds/endpoints 代理节点定义的 JSON 文本，且不包含任何 route 规则或空组

### Requirement: 非 Mihomo 遇到空策略组校验一致性 [FR-PUB-03] [NEEDS CLARIFICATION]
在处理纯节点导出请求（Sing-box / Surge / Quantumult X）时，针对数据库中存在未使用的空策略组校验逻辑，系统 SHALL 依据用户在 Q4 中的决策，确定是完全忽略策略组直接导出有效节点，还是保持严格全系统一致性拦截报错。

#### Scenario: 含有空组时导出纯节点
- **WHEN** 数据库中存在一个空策略组，且客户端请求 Sing-box 纯节点配置
- **THEN** 编译器依据已定策略执行校验拦截或跳过策略组安全导出节点

### Requirement: 发布端点免密公开访问与专属 Token [FR-PUB-04]
系统 SHALL 支持发布端点的鉴权控制。当 `export_auth_enabled == false` 时，服务端 MUST 允许匿名客户端公开免密拉取；当为 true 时，客户端 MUST 提供有效凭据，且专属发布 Token 必须通过校验。

#### Scenario: 免密模式下无凭据访问
- **WHEN** 系统关闭了导出鉴权开关，外部客户端不带任何 Token 请求 `/publish/v1/{id}`
- **THEN** 服务端返回 HTTP 200 并下发完整的发布内容

### Requirement: 导出端点统一系统主令牌复用 [FR-PUB-05]
当导出端点处于受控模式（`export_auth_enabled == true`）时，服务端 MUST 同时支持凭统一系统主令牌（System Token）进行鉴权。若请求携带的 Bearer Token 或 Query Token 匹配主令牌，系统 MUST 予以放行。

#### Scenario: 统一令牌拉取受控发布
- **WHEN** 客户端使用管理端配置的统一系统令牌请求某个受控发布链接
- **THEN** 服务端校验主令牌成功，返回该发布对应的配置内容

### Requirement: 历史 `/p/{id}` 路径兼容路由 [FR-PUB-06]
系统 SHALL 在 HTTP 路由层挂载 `/p/{publication_id}` 兼容端点，确保其行为与标准端点 `/publish/v1/{publication_id}` 完全一致或进行安全重定向，防止存量客户端由于路径迁移发生 404。

#### Scenario: 历史客户端请求短路径
- **WHEN** 历史客户端向 `/p/pub-12345` 发送 GET 请求
- **THEN** 服务端正确处理并返回订阅配置，不返回 404 错误

---

## Success Criteria

### Measurable Outcomes
- **[SC-PUB-01]** Mihomo 编译输出在官方客户端解析下零语法报错，拓扑还原度 100%。
- **[SC-PUB-02]** Sing-box、Surge、QX 编译输出中非节点结构（规则/组）污染率为 0。
- **[SC-PUB-03]** 统一系统令牌在发布端点的兼容鉴权通过率达到 100%。
- **[SC-PUB-04]** 历史 `/p/...` 路径在合法发布项下的请求成功率达到 100%。

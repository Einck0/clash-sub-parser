## Purpose

Compiles canonical subscription graphs and dynamic routing rules into valid configuration formats for Clash Meta, Sing-box, Surge, Loon, and Quantumult X clients.

## ADDED Requirements

### Requirement: 五大核心客户端配置编译与原生序列化
系统 SHALL 完整支持 Clash Meta (Mihomo)、Sing-box、Surge、Loon 与 Quantumult X (QX) 五大核心目标配置的编译与分发导出，使用强类型 Go 适配器和原生序列化（YAML/JSON/INI）保证与既有导出配置 100% 格式与字段兼容。

#### Scenario: 导出 Mihomo 完整配置
- **WHEN** 客户端访问 `GET /mihomo` 或 `GET /api/generate?target=mihomo`
- **THEN** 系统动态根据当前启用的订阅、正则策略组过滤规则、跳板链配置与 DNS 选项，生成合法、无语法错误的 YAML 格式配置文本并直接返回

### Requirement: 正则表达式动态展开与跳板链解析
系统在编译节点分组与路由规则时，SHALL 动态执行正则表达式匹配展开当前有效节点清单，绝不将静态节点列表固化保存；且能够正确解析跳板链（Proxy Chain）顺序并拒绝自环或循环嵌套。

#### Scenario: 包含循环引用的跳板链配置
- **WHEN** 用户配置的跳板链存在 A -> B -> A 循环嵌套
- **THEN** 编译器在预检阶段即刻捕获循环依赖并返回 HTTP 400 明确错误信息，避免配置导出导致客户端无限死循环

### Requirement: 废弃端点与不兼容格式显式拒绝
系统 MUST 显式拒绝已废弃的 SCRIPT 格式和非法目标请求，返回明确的 HTTP 404 或 400 错误。

#### Scenario: 请求已废弃的 script 格式
- **WHEN** 客户端访问 `GET /script`
- **THEN** 系统立即返回 HTTP 404 Not Found，绝不返回未定义格式文本

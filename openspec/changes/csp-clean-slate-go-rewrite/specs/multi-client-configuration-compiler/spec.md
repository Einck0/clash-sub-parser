# Capability: Multi-Client Configuration Compiler

## ADDED Requirements

### Requirement: 五大核心客户端配置编译与渲染
系统 SHALL 完整支持 Clash Meta (Mihomo)、Sing-box、Surge、Loon 与 Quantumult X (QX) 五大核心目标配置的编译与分发导出，使用 `flosch/pongo2` 保证与既有 Jinja2 模板 100% 语法兼容。

#### Scenario: 导出 Mihomo 完整配置
- **WHEN** 客户端访问 `GET /mihomo` 或 `GET /api/generate?target=mihomo`
- **THEN** 系统动态根据当前启用的订阅、正则策略组过滤规则、跳板链配置与 DNS 选项，生成合法、无语法错误的 YAML 格式配置文本并直接返回

### Requirement: 正则表达式动态展开与跳板链解析
系统在编译节点分组与路由规则时，SHALL 动态执行正则表达式匹配展开当前有效节点清单，绝不将静态节点列表固化保存；且能够正确解析跳板链（Proxy Chain）顺序并拒绝自环或循环嵌套。

#### Scenario: 包含循环引用的跳板链配置
- **WHEN** 用户配置的跳板链存在 A -> B -> A 循环嵌套
- **THEN** 编译器在预检阶段即刻捕获循环依赖并返回 HTTP 400 明确错误信息，避免配置导出导致客户端无限死循环

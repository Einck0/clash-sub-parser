## Why

Phase 2 原作者勾选 10/12 后独立 reviewer REJECTED。实读发现五类未闭环行为：凭据端点身份缺少可信绑定，节点 UI 展示/保存虚构资料，RULE-SET 假资源 URL，发布绑定依赖进程 map/审计文本，风险 SQL 多 provider 计数与历史扫描不可靠。既有勾选和旧测试不能代替验收，撤销相关勾选并在同一 Change 根因整改；不把所有审查意见未经核验当成事实。

## What Changes

- **BREAKING**：维持四目标 Mihomo、sing-box、Surge、Quantumult X 及既定 legacy 清场；旧数据不自动迁移/修改。补足可认证的节点端点与凭据一致性：独立于逻辑 ID 的可信绑定；URL/YAML 敏感 transport 与版本不得因简单反算逻辑 ID 而误拒。
- 提供来自受保护源的真实节点非秘密元信息与有审计、鉴权、CSRF、并发版本保护的编辑/原地轮换接口；订阅来源在后续 Reconcile 的覆盖语义明确，拒绝在 UI 伪造保存或泄露私钥、PSK。
- 发布时原子持久化内容摘要、凭据版本/绑定与不可变加密工件或可验证重建依据；重启后严格验证，不从进程 map 或审计字符串恢复身份；历史缺失绑定记录 fail closed。
- Mihomo RULE-SET 仅接受可验证的真实资源 URL/已配置 provider，缺失资源即拒绝；风险读模型保证多 provider 下每节点一条最新观测、count 与分页一致且索引有界。
- 使用隔离本地数据库迁移、注入式确定性安全回归与真实前后端流程复验，再执行独立 reviewer 与具备隔离 URL 的 critic；不触生产、不提交。

## Capabilities

### New Capabilities

- `native-mihomo-publications`: 四目标真实导出、节点身份/编辑、持久发布、风险读模型与安全门禁。

### Modified Capabilities

无；更新该 Change 已存在的 delta spec。

## Impact

`internal/domain/`、`internal/parser/`、`internal/resolver/`、`internal/application/inventory/`、`internal/transport/http/nodes.go`、`web/src/features/nodes/`、`internal/application/publication/`、`internal/repository/sqlite/` 与本地 schema migration、`internal/compiler/mihomo.go` 及相应测试。其他已实现四目标/legacy 清场仅对回归负责。规划授权不等于施工、生产迁移或部署授权。

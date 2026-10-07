## Why

当前 CSP 实例包含大量历史遗留节点数据（1014 节点，其中 983 历史失活/孤儿节点，31 活跃节点）与累积探针历史。根据用户最新明确授权，系统需要进行彻底的节点数据清空（Clean-slate Reset）与从当前启用订阅源的全新全量拉取及验证，彻底告别旧节点与历史派生关联。同时，前期排查发现历史上 `SanitizeSubscriptionURL` 的 `q.Del("token")` 缺陷导致 `7li` 与 `魔戒` 两个订阅源持久化了丢失 Token 的裸 URL，导致抓取返回 403。

本项目需要在不破坏任何业务配置资产（订阅配置、启停状态、策略分组、规则、风险策略、定时调度、管理设置及不可变发布快照）的前提下：
1. 实现完整、原子且满足严格外键约束的节点重置闭包（彻底将 nodes 及全部派生表归零，断开旧 last-good 指针，保留业务资产与不可变发布快照，生成受保护的私有重绑计划）；
2. 基于冷归档稳定身份证据，精准修复 `7li` 与 `魔戒` 两个源的完整 URL（包含 token，不污染日志与其余 7 个源，具备幂等性与安全脱敏）；
3. 拓展协议解析层对全量协议（HTTP, SOCKS5, VLESS, AnyTLS）的支持，基于 `ComputeConnectionLogicalID` 保证同端点不同凭据的正确区分与去重，不泄露凭据摘要；
4. 提供正式的维护 CLI 子命令（`csp reset-node-inventory`），支持 `--confirm-backup` 与 `--backup-file` 校验，复用生产 application/repository wiring，在隔离副本上完成全量演练自测；
5. 实施全新订阅源全量刷新与失败根因归因分析，并在隔离环境下执行公平 Benchmark 与预览发布验证，最终平滑交付生产稳定观察。

## What Changes

- **节点派生数据彻底清空与不可变发布保护 (Clean-slate Node Reset)**：
  - 新增原子重置事务能力，清除所有旧 `nodes` (1014 -> 0) 以及全部依赖派生表：`node_sources`, `node_source_history`, `node_connection_heads`, `node_connection_versions`, `node_overrides`, `probe_observations`, `probe_runs`, `probe_batches`, `probe_batch_runs`, `ip_risk_observations`；
  - 检查并清理显式指向节点的 `group_edges`，在重置前生成受保护的私有重绑计划（Rebind Plan），记录原 group/key 关系与连接配置，避免悬挂外键引用，在 apply 模式下持久化至 0600 权限的私有维护清单；
  - 保护不可变发布资产：保留 `publications` 与 `publication_payload_refs` 引用的 payloads 及其 body/config 字节与头信息不变，对其关联的 `subscription_entries` 执行 `node_logical_id = NULL` 解耦；清理未被发布引用的旧 raw payloads 与 entries；
  - 保留留存抓取记录的不可变历史审计事实（`nodes_parsed` 与 `nodes_valid` 原始数值严禁清零伪造审计），通过清空 `node_sources` 切断历史 last-good 成员指针；
  - 完整保留 9 个订阅配置、启停、分流组（29 个 node_groups 及组间边）、分流规则（163 条 policy_rules）、准入规则、全局过滤器、风险策略与定时调度；
  - 重置前后执行严格 `PRAGMA foreign_key_check`，前置脏外键提前拒绝，支持失败事务回滚与幂等重复执行。
- **已证实损坏订阅源配置修复 (Source Token URL Repair)**：
  - 针对历史 `SanitizeSubscriptionURL` 导致的 token 剥离缺陷，以冷归档 `/home/service/backups/csp-legacy-cold-archive-20260919.db` 中的稳定导入 ID 与归档身份为唯一证据；
  - 仅修复 `7li` 与 `魔戒` 两个源的持久化完整 URL（恢复 `?token=...`），严格不改动其他 7 个订阅的配置、启停状态（保持 4 enabled / 5 disabled）；
  - 具备严格幂等性，已恢复或已有 Token 时跳过且不累增 revision 与审计事件；日志与对外报告严禁暴露 raw token 及敏感路径片段。
- **协议解析与同端点多凭据区分 (Protocol & Connection Identity)**：
  - 扩展完整 HTTP, SOCKS5, VLESS, AnyTLS 协议解析与 Mihomo 适配；
  - 实现基于 `domain.ComputeConnectionLogicalID` 的确定性唯一身份计算，将完整认证与传输配置融入不透明哈希，使同端点不同账号节点具备独立 Logical ID，不泄露凭据摘要。
- **正式维护 CLI 命令与服务复用 (`csp reset-node-inventory`)**：
  - 在 `cmd/csp` 下新增 `reset-node-inventory` 子命令，提供 `--db`, `--dry-run`, `--apply`, `--confirm-backup`, `--backup-file`, `--restore-source-tokens` 等参数；
  - 内部直接复用现有 SQLite 仓库与 inventory/subscription application service wiring，不新增额外免密 Web 接口或旁路 HTTP 桥接；
  - 输出结构化 JSON 报告，包含各表前后计数、外键校验结论、受影响绑定统计与源修复状态。
- **全新订阅源抓取、归因分析、公平基准与发布验证 (Fresh Fetch, Bench & Preview)**：
  - 对 4 个已启用的订阅源执行全新全量抓取，对任何失败执行根本原因归因分析；
  - 实施公平 Benchmark 探针评测与候选发布节点筛选；
  - 构建预览发布快照，执行前端界面与黑盒验收验证；
  - 生产发布后执行有界观察窗口，确保服务运行平稳与性能达标。

## Capabilities

### New Capabilities
- `inventory/clean-slate-node-reset`: 规范节点库存及所有衍生表（连接版本、探针历史、观测数据、历史账本）的完整原子清空闭包，维护不可变发布快照与未受影响业务配置资产，断开旧 last-good 关联。
- `subscription/source-token-repair`: 规范基于冷归档证据对 `7li` 和 `魔戒` 订阅源完整 URL 的精准恢复规范与脱敏审计机制。
- `maintenance/reset-node-inventory-cli`: 规范一次性维护 CLI 工具 `csp reset-node-inventory` 的参数设计、dry-run 预检、原子 apply 事务及复用生产 service wiring 规范。
- `policy/rules-validation`: 规范分流规则只读动态校验 API、空策略组致命错误判定、结构化 issues 诊断与前端规则页防竞态交互。

### Modified Capabilities

## Impact

- **数据存储层 (`internal/repository/sqlite/`)**：
  - 在 inventory/node repository 中提供原子清空闭包方法，确保严格的外键执行顺序与事务边界；
  - 确保外键校验无悬挂；
- **应用逻辑层 (`internal/application/inventory/`, `internal/application/subscription/`)**：
  - 提供 `ResetNodeInventory` 核心编排服务与结果结构；
  - 提供源 Token URL 精准修复函数与安全审计；
- **命令行工具 (`cmd/csp/`)**：
  - 增加 `csp reset-node-inventory` 子命令；
- **生产隔离性**：
  - 本轮施工与自测完全在 SQLite 隔离副本及测试用例中执行，绝不直接修改生产容器数据库或生产环境数据。

## Why

当前生产环境存在三大核心业务与探针缺陷：
1. 订阅刷新机制具有破坏性：当外部订阅源拉取失败、解析异常、返回空节点或暂时缺失时，系统过早中断并在仓储层执行全局孤儿失活 SQL（`UPDATE nodes SET active = 0 WHERE ... NOT EXISTS node_sources`），导致历史上千节点（含 2026-09-29 被误伤的大量节点）被不当失活，且缺乏同一来源内的节点身份与连接版本（`connection_revision`）安全稳定递增机制；
2. 探针构建器存在协议降级与虚假错误掩盖：`builder.go` 将 transport 名直接填入 Outbound Network 强制设为 TCP，破坏了 UDP 协议能力；Reality 公钥缺少标准 32 字节校验，ShortID（sid）处理缺乏奇数长度拦截；`safe_dialer.go` 在构建失败时丢弃原始原因返回泛化错误，且未能严格分离“本地配置构建成功”与“远端网络握手健康”；
3. 接口缺乏端到端真实业务与语义门禁：过往仅依赖编译通过或空列表 HTTP 200，缺乏对全部前台路由与前端调用点（覆盖 45+ 端点）的副作用、权限、幂等、并发、取消及业务数据闭环验收。

## What Changes

- **订阅节点非破坏性合并与来源保护**：
  - 重构 `internal/application/inventory/service.go` 的 Reconcile 流程：订阅网络失败、HTTP 异常、解析报错、返回空列表或局部拒绝（rejected）时，默认保留既有已导入节点与策略边，标记来源为 stale/刷新失败，严禁自动下线或删除既有节点；
  - 彻底废除全局无来源节点失活 SQL（`deactivateOrphansSQL`），来源 prune 仅限于当前受管订阅作用域内的归属关系，禁止波及手工导入节点或其他订阅来源的节点；
  - 在同一订阅来源作用域内实现安全的节点身份匹配：优先使用源提供的稳定标识，无标识时仅在同一来源内基于名称/协议等唯一无歧义条件匹配更新；存在歧义或重名时保守保留并新增，严禁跨来源按名称合并凭据；
  - 维护节点逻辑身份与策略边稳定，连接信息变化时严格单调递增真实的 `connection_revision`；单纯显示名或 JSON 键排序变化不得提升 revision；真连接变化时旧观测保留历史但当前状态置为 unknown/pending，杜绝沿用失效 revision 的虚假 healthy 结论；
  - 制定并执行幂等、可回滚、只针对误伤数据的恢复方案与可验证演练脚本，禁止一键粗暴全量激活，不得覆盖用户手工禁用或真实删除状态。
- **探针构建器能力解耦与脱敏真实诊断**：
  - 重构 `internal/probe/singbox/builder.go`：严格解耦 L4 网络层（`tcp` / `udp`）与传输层（`ws` / `grpc` / `http` 等），遵循 Sing-box 实际库协议能力，支持 UDP 出站，不再强制单一 TCP；
  - 规范 Reality 协议参数校验：对 Reality `pbk`（Public Key）实施严格的 32 字节解码校验与标准 Base64 规范化；对 `sid`（Short ID）严格遵循协议规范，奇数长度或非法十六进制字符直接拒绝并返回明确诊断，严禁静默补零或截断篡改身份；
  - 测证底层 Sing-box 库对 `xhttp` 等协议特性的实际构建支持，不凭空假设或报告认定；
  - 改造 `internal/application/probe/safe_dialer.go`：捕获底层客户端构建错误并分类，输出脱敏的结构化诊断信息，严禁在日志或响应中泄露密码、UUID、URL query、公钥或私钥等敏感凭据；
  - 严格保持 SSRF 防护、私网 IP 拦截及 TLS 证书安全校验边界；将“本地客户端配置构建成功”与“远程链路握手成功”作为独立状态分别判定与上报。
- **全接口业务语义测试与交付门禁**：
  - 建立覆盖全部前端调用点与后端路由（包含 auth、settings、subscriptions、nodes、probes、policies、publications、health/ready 等 45+ 路由端点）的端到端语义验收矩阵；
  - 实施隔离环境下的全接口语义测试，严格校验请求入参、响应字段、数据库副作用、权限拦截（401/403）、幂等处理、并发控制与撤销操作，杜绝空泛 HTTP 200 判定；
  - 确立完整施工门禁与只针对 CSP 受管服务的受控部署预案，包括备份验证、副本演练与独立 Reviewer/Critic 验收闭环。

## Capabilities

### New Capabilities

- `subscription-inventory-preservation`: 订阅节点非破坏性合并、来源作用域隔离、身份与连接版本单调递增管理，以及历史误伤节点可证据化、幂等、可回滚的数据恢复规程。
- `probe-construction-and-truthful-diagnostics`: 探针网络层与传输层协议解耦、UDP 真实支持、Reality 凭证严格校验、客户端构建错误脱敏诊断，以及构建成功与握手健康的精确分离。
- `api-semantic-verification-matrix`: 覆盖全台 45+ 路由与前端交互端点的端到端语义隔离测试、副作用闭环及业务真实验收矩阵。

### Modified Capabilities

- 无。本仓库规格体系聚焦当前变更增量，未修改既有已归档主规格。

## Impact

- **领域与应用服务层**：`internal/application/inventory/service.go`、`internal/domain/id.go`、`internal/application/probe/safe_dialer.go`、`internal/probe/singbox/builder.go`、`internal/probe/singbox/`。
- **持久化与数据迁移**：SQLite 节点与来源关联更新逻辑、历史误伤数据修复迁移/运维脚本。
- **传输层与全接口测试**：`internal/transport/http/` 全部路由处理程序、端到端测试套件 `test/` 与 API 集成测试。
- **Web 前端与黑盒验收**：前端各模块与后端的交互联动、独立审查后 Critic 的真实视口及有数据场景截屏验收。

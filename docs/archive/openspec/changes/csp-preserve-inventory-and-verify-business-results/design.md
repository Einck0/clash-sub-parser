## Context

参见 `proposal.md` - Why 与 What Changes。
当前系统在生产运行中暴露了三个层面的实现缺陷：
1. **订阅合并与节点生命周期**：`internal/application/inventory/service.go:297-348` 在拉取或解析错误时提前返回并中止事务，且在 `service.go:459-476` 中使用了破坏性的全局孤儿失活 SQL：
   ```sql
   UPDATE nodes SET active = 0, updated_at = ?
   WHERE active = 1 AND NOT EXISTS (SELECT 1 FROM node_sources WHERE node_sources.node_logical_id = nodes.logical_id);
   ```
   该语句不仅在外部订阅拉取失败或返回空时直接使全部节点失活，而且抹杀了未绑定 node_sources 的手工导入节点；同时 `internal/domain/id.go:82-120` 生成的 `logical_id` 与连接版本管理缺乏在订阅作用域内的安全匹配机制，导致连接配置发生真实变更时未能正确自增 `connection_revision` 或虚假沿用历史 healthy 状态。
2. **探针出站构建器与安全拨号器**：`internal/probe/singbox/builder.go:149/164/199/262/289` 直接调用 `networkName(config)`，将传输层名称（如 `ws`、`grpc`）当作 L4 网络协议写入 `Outbound.Network`，并强制默认为单一 `tcp`，导致 UDP 探测能力失效；Reality 参数处理未对 `pbk` 实施严格的 32 字节校验，对 `sid`（Short ID）存在奇数位未拒绝的潜在隐患；`internal/application/probe/safe_dialer.go:179-183` 在捕获底层客户端构建失败时直接丢弃了根因并返回泛化的 `ErrClientBuildFailed`，且未严格解耦本地构建成功与远端网络握手。
3. **接口业务语义与交付门禁**：控制面现有 45+ 个路由端点缺乏端到端副作用闭环测试，部分测试仅做 HTTP 200 断言，未能验证数据库持久化、幂等性、并发冲突及权限边界。

## Goals / Non-Goals

**Goals:**
- 在 `internal/application/inventory/service.go` 中建立非破坏性 Reconcile 状态机：拉取/解析失败、空列表、部分拒绝时全量保留既有活跃节点，标记来源状态为 stale/failed；
- 彻底废除全局孤儿节点批量失活 SQL，将来源 prune 严格限定在当前操作的 `subscription_id` 范围内，保护手工节点与其他来源节点不受波及；
- 在同一订阅来源作用域内实现稳定节点身份匹配与连接版本（`connection_revision`）单调递增管理，重名/歧义保守保留并新增，严禁跨来源合并凭据；单纯显示名/JSON 键排序变更不升 revision；连接真实变更时重置当前观测为 unknown/pending；
- 制定并演练针对 2026-09-29 等历史误伤节点的证据化、幂等、可回滚的数据恢复脚本，严禁全量粗暴激活，保护手工禁用状态；
- 在 `internal/probe/singbox/builder.go` 中严格解耦 L4 协议（`tcp`/`udp`）与传输层（`ws`/`grpc`/`http`），完整支持 UDP 协议出站；
- 对 Reality `pbk` 实现 32 字节解码校验与 Base64 标准化；对 `sid` 实施奇数位与非法字符严格拒绝；测证实际 Sing-box 库对 `xhttp` 的构建能力；
- 改造 `safe_dialer.go`：捕获底层构建错误并提供脱敏结构化诊断，严禁凭据/URL query/公钥私钥泄露，维持 SSRF/私网 IP/TLS 安全拦截；解耦本地构建通过与远端握手健康；
- 建立前后端双向核对清单，新增隔离全接口语义测试套件，全面覆盖 45+ 路由的数据库副作用、幂等性、并发及权限拦截。

**Non-Goals:**
- 不修改底层 Mihomo 编译器规则匹配核心或 PDD 引擎；
- 不重启或修改宿主机 Hermes 内核与外部网关服务；
- 不在 OpenSpec 规划与设计阶段直接编写或修改生产业务代码；
- 不改动外部订阅凭据本身失效或无权限的客观事实；
- 不在未经过 DB 备份与测试副本演练前直接对生产数据库执行数据变更。

## Decisions

### 1. 非破坏性订阅协调状态机（Non-Destructive Reconcile Engine）
- **设计选择**：改造 `internal/application/inventory/service.go` 中的 `ReconcileSubscription`：
  - **失败安全分支**：当 `fetcher.Fetch` 失败或 `parser.ExtractWithCredentials` 报错时，记录 `domain.SubscriptionFetch` 审计事件并将订阅状态置为 `FetchOutcomeFailed`，但**立即终止后续 prune 操作**，保留该订阅关联的所有活跃节点与策略边；
  - **来源修剪严格局部化**：废除全局 `deactivateOrphansSQL`。来源修剪语句改造为：
    ```sql
    DELETE FROM node_sources WHERE subscription_id = ? AND last_seen_fetch_id != ?;
    ```
    对于因本订阅更新而在当前订阅中不再出现的节点，系统仅解除该节点与当前订阅在 `node_sources` 中的绑定关系；只有当某个节点在 `node_sources` 中已无任何订阅绑定、且该节点被明确标识为由订阅自动生成（而非手工创建）时，才将其局部标记为失活，严禁跨表全量更新无关联节点；
  - **空提取保护**：若订阅提取成功但有效节点数为 0，系统将其视为异常或源端清空保护场景，记录警告审计，默认不清理既有节点，除非用户在 API 中显式指定清空选项。

### 2. 来源作用域内节点身份稳定匹配与连接版本控制
- **设计选择**：
  - **匹配优先级**：在同一 `subscription_id` 范围内，若节点提取项包含源端唯一标识（如源端提供的 ID/UUID），以该标识优先匹配；若无标识，则以 `(normalized_name, protocol, server, port, transport_summary)` 作为唯一性键匹配；
  - **歧义处理**：若同一来源内存在重名或参数冲突，采取保守策略：保留既有节点不变，为新节点分配独立逻辑身份，严禁覆盖；绝对严禁跨不同订阅按显示名合并节点；
  - **连接版本（`connection_revision`）递增逻辑**：
    - 对比新旧配置的真实连接要素（`server`、`port`、`protocol`、关键传输密钥、加密方式、混淆密码）；
    - 若连接要素实质变更：递增 `connection_revision`（`revision = revision + 1`），更新节点配置，并将该节点的当前探测健康态标记为 `unknown/pending`，不复用旧版本的探测结论；
    - 若仅为 `display_name` 变更或 JSON 字段格式化/排序变化：仅更新显示元数据，`connection_revision` 保持不变，既有健康状态继续保留。

### 3. 历史误伤节点证据化精准修复程序
- **设计选择**：
  - **误伤数据特征归因**：2026-09-29T00:52 发生的批量失活系由当时的全局孤儿 SQL 触发。修复脚本通过排查以下条件锁定误伤节点集合：
    1. `active = 0` 且 `updated_at` 处于该批次误伤时间窗口；
    2. 节点的规范配置 `normalized_config_secret_ref` 完好且协议参数合法；
    3. 未存在用户手工在控制面点击禁用的审计记录。
  - **证据记录与幂等性**：恢复脚本编写为独立工具/脚本，先生成包含待修复节点 ID、名称、协议的 JSON 审计清单，在测试数据库副本上全流程演练无误后方可由授权运维应用；
  - **安全约束**：严禁执行全量 `SET active = 1 WHERE active = 0`；对于无法确定归因的存量节点保留在 `pending_review` 列表中供人工确认。

### 4. 探针出站构建器 L4 协议解耦与 Reality 参数规范化
- **设计选择**：
  - **L4 与 Transport 解耦**：重构 `builder.go` 中的 `networkName` 及各个 `build*Outbound` 函数：
    - `Outbound.Network` 仅包含合法 L4 协议（`tcp`、`udp` 或 `badoption.Listable[string]{"tcp", "udp"}`）；
    - Transport 层（如 `ws`、`grpc`、`http`）配置在对应的 `V2RayTransportOptions` 中，不得将 transport 字符串写入 Network 字段；
    - 对于 Shadowsocks、VMess、VLESS、Trojan 等协议，若节点配置中声明了 UDP 转发或传输支持，正确生成支持 UDP 的出站配置；对于 Hysteria2、TUIC 等基于 UDP 的协议，确保 Network 包含 `udp`；
  - **Reality 参数校验**：
    - `RealityPublicKey`：先验证非空，尝试标准 Base64 解码，若失败则尝试 URL-Safe Base64 解码；解码后字节数必须严格等于 32 字节，否则返回 `domain.NewValidationError("invalid_reality_public_key", "Reality public key must decode to exactly 32 bytes")`；最后统一转为 Sing-box 期望的编码格式；
    - `RealityShortID`：验证其为纯十六进制字符串（`[0-9a-fA-F]*`），且长度必须为偶数（`len % 2 == 0`）且不超过 16 字节（32 个十六进制字符）。若包含奇数长度或非法字符，直接返回 `domain.NewValidationError("invalid_reality_short_id", "Reality short ID must be even-length hex string")`，严禁截断或补零；
  - **Sing-box 库能力真实测证**：针对底层核心库实际支持的协议选项编写独立单元测试（`builder_singbox_test.go`），对 `xhttp` 等特性以实际构建是否报错为依据判定支持情况。

### 5. 安全拨号器脱敏结构化诊断与握手健康分离
- **设计选择**：
  - **错误分类捕获**：在 `safe_dialer.go` 中，调用 `clientFactory(ctx, cfg, httpOpts)` 时捕获底层返回的原始错误，利用类型判断与模式匹配将其分类为：`ConfigParseError`、`TLSConfigError`、`TransportUnsupportedError`、`InternalEngineError` 等；
  - **脱敏处理**：将原始错误字符串输入 `domain.RedactSensitiveInfo` 进行敏感信息脱敏（剥除密码、UUID、Query 参数、密钥），包装为可读的结构化错误返回，确保日志与 API 输出既具备可诊断性又绝对不泄露机密；
  - **安全边界维持**：SSRF 校验、私网 IP 过滤（`cleanHost` 校验与私网地址拦截）以及证书校验逻辑保持最严水位，不得为了连通而放宽；
  - **状态分离**：在探测观测模型中，将“本地配置构建完成”与“远程网络探测成功”明确解耦，本地构建成功后方进入网络阶段，网络阶段超时或重定向错误绝不伪造为 healthy。

### 6. 前后端路由双向核对与全接口语义测试矩阵
- **设计选择**：
  - **双向清单核对**：编写自动化测试 `test/e2e/route_inventory_test.go`，静态扫描前端代码中的所有 API 请求调用路径与后端 Chi 路由树，建立动态对齐清单，覆盖全部 45+ 路由；
  - **全接口语义测试矩阵**：在 `test/e2e/api_semantic_matrix_test.go` 中针对每个路由组建立端到端测试用例：
    1. **Auth 组**：`/api/v1/auth/status`、`/login`、`/logout`，校验 Session 与 Cookie 状态；
    2. **Settings 组**：`/api/v1/settings/auth`、`/settings/admin-token`，校验双开关状态持久化与密码验证；
    3. **Subscriptions 组**：增删改查、`/refresh`，校验数据库新增与变更、非破坏性合并；
    4. **Nodes 组**：查询列表、详情、连接补丁，校验 `connection_revision` 变更；
    5. **Probes 组**：`/runs`、`/pool`、`/schedule`、`/cancel`、`/batches`，校验状态机与幂等重放；
    6. **Policies 组**：全局节点过滤规则查询与变更，校验审计事件；
    7. **Publications 组**：发布创建、预检、预览、导出端点（`/publish/v1/{id}` 与 `/p/{id}`）、撤销；
    8. **System 组**：`/healthz`、`/readyz`、历史路径 `/yaml` 与 `/script` 410 拦截。
  - **断言标准**：每个用例不仅验证 HTTP 状态码，还直接查询 SQLite 验证数据库副作用与数据完整性。

## Risks / Trade-offs

- **[Risk 1: 外部订阅源永久失效导致陈旧节点长期积压]** → 缓解措施：在订阅元数据中明确标识 `stale: true` 与最后成功刷新时间戳，前端直观标红提示，并提供显式“清空该订阅节点”的管理操作，由管理员自主决策是否清除。
- **[Risk 2: 误伤数据恢复脚本误恢复了用户主动停用的节点]** → 缓解措施：严格依据审计日志排查是否由用户手动修改，恢复脚本在临时测试数据库上演练输出 diff 报告，且恢复事务具备反向回滚 SQL 脚本。
- **[Risk 3: Reality 参数严格校验导致部分历史不规范订阅节点在构建阶段被拒]** → 缓解措施：在脱敏诊断中明确返回 `invalid_reality_short_id` 或 `invalid_reality_public_key`，便于排查源端配置，符合协议规范与安全边界。
- **[Risk 4: 45+ 接口全量语义测试执行耗时增加]** → 缓解措施：测试采用内存 SQLite（`:memory:`）或独立隔离临时文件库，用例间数据完全隔离且执行速度控制在秒级。

## Migration Plan

1. **备份与环境确认**：执行当前 SQLite 数据库文件与配置文件的时间戳快照备份，验证备份文件大小与完整性；
2. **测试副本演练**：在独立的测试数据库副本上运行历史误伤节点精准恢复脚本，核对恢复节点数量与状态字段；
3. **本地静态编译与单元测试门禁**：后端执行 `go build ./...` 与 `go test ./...`，前端执行 `npm run type-check`，全部确保退出码为 0；
4. **独立 Reviewer 审查与 Critic 验收**：代码审查 PASS 后，启动隔离容器验证健康端点与全接口业务，Critic 执行带数据界面的无头浏览器截屏验收；
5. **受控上线与可回滚目标**：保留当前运行版本镜像作为回滚目标，仅当所有前置门禁全部通过后由主脑执行受控上线。

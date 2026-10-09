## Purpose

定义探针拨测与发布编译器的统一消费模型、版本化传输协议能力矩阵、IP Risk Nullable 1:N 统一测量关联、Safe Detail 白名单防泄密机制、发布两阶段生命周期（Preview Draft -> Publish Snapshot）与 Payload 物理防删引用，以及前端错误分类治理。

## ADDED Requirements

### Requirement: 统一消费模型与 Mihomo xhttp 官方代码对齐
系统中的探针拨测模块（`internal/probe/mihomo`）与发布编译器模块（`internal/compiler`）MUST 严格基于同一份 `effective_config_json` 进行下游出站渲染与转换。针对支持现代传输协议（如 VLESS `xhttp` / Splithttp）的底层运行时（Mihomo v1.19.32+），系统 MUST 解除编译白名单限制，并严格依照官方支持参数进行渲染：
1. 仅将官方支持的字段（`path`, `host`, `mode`, `headers`）渲染为 `xhttp-opts`；
2. 未知 extra 字段完整保存在内部扩展属性中并输出诊断提示，**严禁盲传给 Mihomo 渲染器**；
3. 对于不支持 `xhttp` 的目标运行时（如 sing-box），系统如实返回 `unsupported_target_capability` 诊断，严禁伪造假协议转换。

#### Scenario: VLESS xhttp 节点在 Mihomo 目标下成功导出
- **WHEN** 包含 `network: "xhttp"` 的 VLESS 节点请求 Mihomo 格式预览
- **THEN** 编译器仅渲染官方支持的 `path`, `host`, `mode`, `headers` 节点块，未知 extra 不盲传，无 422 报错

### Requirement: IP Risk Nullable 1:N 统一测量与策略规则快照
系统 SHALL 建立 IP Risk 的 1:N 关联模型：
1. 细粒度服务商评分记录（`ip_risk_observations`）的外键 `probe_observation_id` **严格且仅对应 `kind = 'ip_risk'` 的通用探针观测记录**，绝对不是 Baseline 连通性测活的“一对多”（严禁混淆 Baseline 测活与 IP Risk 威胁情报，两者属于正交维度）；
2. 跨表 kind 约束采用应用层事务校验（Application Transaction Validation）结合标准 SQLite 外键（`probe_observation_id REFERENCES probe_observations(id) ON DELETE SET NULL`）。在写入或更新 `ip_risk_observations` 时，事务显式校验被引用的探针观测其 `kind == 'ip_risk'`，若关联至 `kind = 'baseline'` 或其他非 ip_risk 类型则直接阻断并报错 `invalid_probe_kind_for_ip_risk`，不自造复杂触发器平台；
3. 历史独立服务商风险记录外键 `probe_observation_id` 允许为 `NULL`，严禁凭空构造伪造的 `probe_runs` 或观测记录，多个 provider 记录完整保全不丢失；
4. Baseline 测活与 IP Risk 详情解耦；历史风险记录仅在连接版本明确已知时如实回填；
5. 利用现有 `risk_policy_revisions` 表的 `rules_digest` 字段锁定策略版本快照，避免多表版本膨胀。

#### Scenario: IP Risk Nullable 1:N 统一关联生效
- **WHEN** 探针调度器完成某节点的 IP 风险查询并获取 2 个服务商情报
- **THEN** 系统写入 1 条通用 `kind = 'ip_risk'` 的 `probe_observations` 记录与 2 条 `ip_risk_observations` 记录，后两者外键均指向该通用观测 ID

#### Scenario: 关联错误探针 kind 被应用层事务校验阻断
- **WHEN** 尝试将 `ip_risk_observations.probe_observation_id` 指向一条 `kind = 'baseline'` 的观测记录
- **THEN** 系统应用层事务校验抛出 `invalid_probe_kind_for_ip_risk` 错误并回滚事务，拒绝非 ip_risk 类型关联

#### Scenario: 历史独立风险记录保持 NULL 关联
- **WHEN** 录入或迁移无对应通用探针运行的历史独立服务商风险评分
- **THEN** `probe_observation_id` 为 NULL 成功持久化，不伪造 probe_run，多 provider 数据无损保全

### Requirement: Safe Detail JSON 白名单防泄密机制
系统在记录探针详细失败原因时，SHALL 在 `safe_detail_json` 中实施严格的 Allowlist 白名单机制：
1. 仅允许收集 `stage`, `code`, `target_core_version`, `protocol`, `transport`, `http_status`, `timeout_ms`, `reason`, `server_redacted`；
2. 严禁捕获原生 `err.Error()` 或完整 HTTP 响应体；
3. 绝对排除密码、UUID、Token、私钥、Authorization 头部、Cookie 头部及 Query 参数中的敏感 Secret；
4. 历史遗留缺失 detail 保留为 `unknown`，不伪造定论。

#### Scenario: 探针失败记录自动过滤敏感凭据
- **WHEN** 探针执行发生认证失败，底层错误包含包含密钥的 URL
- **THEN** `safe_detail_json` 仅记录 stage 与 code，将 server 脱敏，绝不输出密码与私密参数

### Requirement: Publication 两阶段生命周期、清单与 Payload 物理防删引用
系统在执行发布编译与导出（`/api/v1/publications/*`）时，SHALL 实施两阶段快照生命周期与物理引用保护：
1. **Preview 阶段**: 系统创建不可变 draft 记录并计算完整清单（Manifest JSON，含各节点 included/excluded 理由、payload_ids、version_refs、产物哈希），向 `publication_payload_refs` 插入关联记录；
2. **Publish 阶段**: 发布请求 MUST 显式传递 `snapshot_id`，系统严格直接激活该快照已编译的字节，**绝不重新读取当前动态节点库**，杜绝时间差库存漂移；
3. **Payload 物理防删**: `publication_payload_refs` 外键 `ON DELETE RESTRICT` 物理阻止删除正在被草稿或激活发布引用的 Payload；
4. **Strict 模式 (默认)**: 遇到不支持节点返回 HTTP 422 及完整诊断清单，Draft 标记为不可发布（`publishable = 0`）；
5. **Compatible 模式**: 系统根据规则安全过滤不支持节点并在清单中详实记录排除原因。重新校验策略组引用：若任何策略组的成员节点数变为 0，系统拒绝生成并明确报错，**严禁自动回退到 DIRECT**，杜绝用户真实流量非预期泄露。

#### Scenario: 显式兼容模式安全排除不兼容节点且防止 DIRECT 穿透
- **WHEN** 导出目标为 sing-box 且请求显式携带 `mode=compatible`，某策略组过滤后剩余节点为 0
- **THEN** 编译器拒绝发布并报错 `empty_group_not_allowed`，不回退为 DIRECT

### Requirement: 探针观测元数据扩展与陈旧状态派生
系统 SHALL 为 `probe_observations` 扩展 `measurement_id`、`engine_version`、`parser_version`、`mapping_version`、`stage`、`error_code` 与 `safe_detail_json` 字段。探针记录采用纯追加模式（Append-Only），严禁原地覆写历史观测的时间戳或裁决。读模型在查询时，若观测记录的 `connection_revision` 不等于 `node_connection_heads.connection_revision` 或观测时间超过 TTL，动态将其派生标为 `stale`。

#### Scenario: 连接参数更新后历史观测动态派生为 stale
- **WHEN** 某节点的连接版本从 1 递增至 2，客户端请求该节点最新状态
- **THEN** 读模型发现版本不一致，将基于版本 1 的观测记录动态标记为 stale，不展示虚假可用

### Requirement: 前端错误卡片纠偏治理 (Path: `web/src/ui/ErrorStateCard.vue`)
Web 前端（`web/src/ui/ErrorStateCard.vue`）SHALL 优先基于 HTTP 状态码与结构化业务 `code` 进行错误分类展示。当且仅当底层发生真实网络中断或网关超时时，方可分类为“网络失联”；对于 HTTP 422 或 `code === 'unsupported_target_capability'` 错误，前端 MUST 准确展示为“协议与目标能力不兼容”，严禁因错误消息中包含 `"network"` 字样而落入 `isNetwork` 误判为网络异常。

#### Scenario: 422 协议不支持准确展示业务卡片
- **WHEN** 前端收到后端返回的 HTTP 422 `unsupported_target_capability` 响应
- **THEN** 界面展示具体的协议能力诊断与节点定位，不出现“网络连接异常，请检查服务可用性”的误导提示

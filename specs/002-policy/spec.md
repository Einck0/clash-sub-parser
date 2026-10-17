---
feature: 002-policy
title: Policy Capability Specification (Topology Hygiene, Inherited Filter & Probe State Governance)
status: IMPLEMENTING
schema: speckit-native
---

# Policy Capability Specification (去过度设计与健全性治理)

## Purpose

本规范定义 CSP 1.0 的策略拓扑校验去噪、策略组全局过滤继承控制、节点探针健康状态极净化、探测超时可配置化以及探针历史运行记录治理。
彻底消除系统中的过度工程化与推卸责任设计：
1. **策略拓扑校验去噪**：被全局可用性过滤器排除的节点属于正常分流数据流转，严禁作为策略拓扑配置警告或阻断 Issue 输出；
2. **策略组「继承全局过滤」开关**：`node_groups` 表支持 `inherit_global_filter`（默认开启 1），允许特定策略组（如 `WARP`）关闭继承（0），直接从全量已接入节点中匹配成员，确保专属策略组不被全局健康过滤抽空；
3. **消除「观测已过期」主状态**：移除节点超过 1 小时即强制标记为 `probe_stale` 的抖动设计；在下一次探测产生新数据前，前一次有效观测判定持续生效；
4. **收敛消除「待复核」，恢复真实「不可用」判定**：彻底消除“不可用为0、待复核几十个”的推卸责任设计；探测失败（超时、连接拒绝、解析失败、传输错误）一律明确判定为「不可用（unhealthy）」，状态体系极简化为：可用（healthy）、不可用（unhealthy）、未测试（untested）；
5. **探测最大超时时间动态配置**：在全局设置（Settings）中支持配置 `probe_timeout_seconds`（范围 1~30 秒，默认推荐 5 秒），控制 Mihomo/Sing-box 出站握手与 HTTP 请求超时；
6. **探针历史记录治理与自动滚动上限**：限制最大保留 100 个批次/运行记录，自动清理老旧 `probe_runs` 与 `probe_batches`，彻底治理生产环境中累积的 14,000+ 运行记录与 80,000+ 冗余观测事实；
7. **纠正批次跳过文案**：修复将无需重复重测的有效节点张冠李戴误标为“凭据缺失或无效节点”的荒谬文案。

---

## User Scenarios & Priority

### User Story 1 - 全局可用性过滤与拓扑校验去噪 (Priority: P1)
- **GIVEN** 管理员开启了全局节点可用性过滤（如 `baseline == available`）
- **WHEN** 进入策略管理界面或调用 `GET /api/v1/policy/validate`
- **THEN** 被全局过滤排除的节点不产生任何黄色警告 Issue，拓扑校验绿色通过；仅当策略组经解析后无任何节点且未配置 `empty_fallback_pass` 时才产生阻断性错误。

### User Story 2 - 策略组继承全局过滤控制 (Priority: P1)
- **GIVEN** 管理员配置了 `WARP` 策略组（匹配 WireGuard WARP 节点）
- **WHEN** 全局开启了可用性过滤（而 WARP 节点尚未测速或独立组网）
- **THEN** `WARP` 策略组因设置了 `inherit_global_filter = 0`，依然能够顺利从全量节点中匹配到 `WARP-83d9` 节点，导出的配置中 WARP 策略组与 WARP 节点均完整呈现，不发生空组阻断。

### User Story 3 - 节点健康状态真实与极简化 (Priority: P1)
- **GIVEN** 节点探针执行完毕
- **WHEN** 节点测通（HTTP 200）则显示「可用」，节点超时或报错则明确显示「不可用」，未测节点显示「未测试」
- **THEN** 界面彻底消除令人困惑的「观测已过期」与「待复核」，不可用计数真实反映故障节点，不再出现“不可用为0、待复核几十个”的虚假现象。

### User Story 4 - 探测超时可配置 (Priority: P2)
- **GIVEN** 管理员在系统设置中调整「探针超时时间」为 5 秒
- **WHEN** 触发手动或周期探测
- **THEN** 探针引擎严格在 5 秒内超时熔断，杜绝单节点长挂 15~30 秒。

### User Story 5 - 探针记录自动精简与数据轻量化 (Priority: P2)
- **GIVEN** 系统长期运行周期探测
- **WHEN** 产生新的探测批次
- **THEN** 数据库自动保留最近 100 次运行批次，老旧批次自动滚动修剪，数据库体积保持轻巧紧凑。

### User Story 6 - 纠正批次跳过文案 (Priority: P2)
- **GIVEN** 周期探测调度器跳过了仍在有效期内的节点
- **WHEN** 管理员查看探针批次详情
- **THEN** 界面准确显示“本次跳过 X 个状态最新/无需重测的节点”，不再虚假提示“凭据缺失或无效”。

---

## Requirements

### 1. 策略拓扑校验去噪 [FR-POL-CLEAN-01]
1. `internal/application/policy/service.go` 的 `Validate` 方法 SHALL 仅将 `Severity == DiagnosticSeverityError` 或针对策略组空置（`empty_routed_group`、`empty_group`）的诊断列为 `ValidationIssue`；
2. 属于普通节点过滤行为的 `global_filter_excluded` 等信息级（Info）诊断，STRICTLY SHALL NOT 生成 `ValidationIssue`；
3. 前端 `PolicyView.vue` 仅在存在 `severity: 'error'` 或 `severity: 'warning'` 时呈现警告或阻断横幅；普通正常过滤状态显示绿色校验通过。

### 2. 策略组继承全局过滤开关 [FR-POL-CLEAN-02]
1. 数据库 `node_groups` 表新增 `inherit_global_filter INTEGER NOT NULL DEFAULT 1`；
2. 存量 `WARP` 策略组（`UPPER(name) = 'WARP'` 或 `id = 'WARP'`）默认将 `inherit_global_filter` 更新为 `0`；
3. `domain.NodeGroup`、`GroupView`、`CreateGroupCommand` 与 `UpdateGroupCommand` 新增 `InheritGlobalFilter bool`；
4. `internal/resolver/resolver.go` 与 `graph.go`：
   - 当策略组 `InheritGlobalFilter == true`（默认）：仅从通过全局过滤的节点池（`globalAdmittedNodes`）匹配成员；
   - 当策略组 `InheritGlobalFilter == false`：从全局准入的全量活跃节点池（`admissionRes.AdmittedNodes`）匹配成员；
   - 最终输出的 `snapshot.Nodes` 包含通过全局过滤的节点池与所有非继承组引用的节点并集，保证编译器导出的 `proxies:` 与策略组引用 100% 闭环合法；
5. 前端 `PolicyEditorSheet.vue` 提供「继承全局过滤」Switch 开关，`GroupCard.vue` 在关闭时展示徽标。

### 3. 消除「观测已过期」主状态 [FR-POL-CLEAN-03]
1. 节点健康度评估不再因自然时间超过 1 小时（`defaultProbeFreshnessTTL`）而将其置为 `stale` / `unknown` / `undetermined`；
2. 只要节点连接凭据未发生版本更新（`connection_revision` 一致），前次探测结论持续有效，直至下一次探测产生新数据覆盖；
3. 前端移除「观测已过期」主状态标签，仅以相对时间（如“35分钟前探测”）作为次要时间戳展示。

### 4. 收敛「待复核」，恢复真实「不可用」判定 [FR-POL-CLEAN-04]
1. `web/src/features/nodes/nodeView.ts` 消除只有 `node_connect_failed` 才是 `unhealthy` 的错误规则；
2. 凡探针基线报错（包含 `timeout`, `dial_timeout`, `transport_error`, `target_unresolvable`, `tls_handshake_failed` 等），在无重试或重试耗尽时一律归类为 `unhealthy`（不可用）；
3. 状态体系收敛为三态：`healthy`（可用）、`unhealthy`（不可用）、`untested`（未测试）；
4. 节点列表过滤与统计卡片同步精简。

### 5. 探针超时时间动态配置 [FR-POL-CLEAN-05]
1. `settings` 表新增 `probe_timeout_seconds INTEGER NOT NULL DEFAULT 5`；
2. `domain.Settings` 增加 `ProbeTimeoutSeconds int`（合法范围 1~30 秒，默认 5 秒）；
3. 探针执行引擎（Mihomo 出站客户端、Singbox 运行参数及 Probe Runner TaskTimeout）动态读取该超时设置；
4. 前端设置页 `SettingsView.vue` 暴露该配置项。

### 6. 探针历史数据治理与自动滚动上限 [FR-POL-CLEAN-06]
1. 数据库迁移对现存 14,000+ 条 `probe_runs` 与 `probe_batches` 执行清理，仅保留最新 100 批次记录；
2. `probe_observations` 仅保留最新 100 批次相关的记录，或每个节点各 probe kind 的最新 1 条记录，清理其余孤儿历史记录；
3. `internal/application/probe` 在每次批次/运行完成时，异步执行修剪：删除 ID 不在最新 100 批次范围内的老旧记录。

### 7. 纠正批次跳过文案 [FR-POL-CLEAN-07]
1. `web/src/features/probes/ProbesView.vue` 将 “已跳过 {{ batch.counts.skipped_nodes }} 个凭据缺失或无效的节点” 修正为：“本次跳过 {{ batch.counts.skipped_nodes }} 个状态最新/无需重测的节点”。

---

## Acceptance Criteria

- **AC-POL-01 (去噪通过)**: 开启全局可用性过滤后，`GET /api/v1/policy/validate` 不再包含 `global_filter_excluded` 作为 warning/error，前端拓扑校验无报警；
- **AC-POL-02 (WARP 豁免)**: `WARP` 策略组关闭继承全局过滤后，在全局过滤开启时仍能选出 WARP-83d9 节点，Clash 导出中 WARP 策略组与节点均存在；
- **AC-POL-03 (真实不可用)**: 探针超时或网络失败的节点，前端统计和卡片真实归类为「不可用」，不再归入「待复核」，「待复核」计数归零或彻底移除；
- **AC-POL-04 (永不过期)**: 距离上次探测超过 1 小时后，节点仍保持可用状态，不自动变更为“观测已过期”；
- **AC-POL-05 (超时生效)**: 设置探针超时为 3 秒后，探针在 3 秒后熔断退出；
- **AC-POL-06 (历史收敛)**: 数据库中 `probe_runs` 和 `probe_batches` 数量被限制在 100 条以内；
- **AC-POL-07 (文案准确)**: 周期探测跳过处于保鲜期节点的提示文案准确清晰。

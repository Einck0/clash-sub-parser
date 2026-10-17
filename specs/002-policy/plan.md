# Technical Plan: Policy Topology Hygiene, Inherited Filter & Probe State Governance

- **Feature**: `specs/002-policy/`
- **Run ID**: `pi_run_20261017_203412_2919528`
- **Status**: IMPLEMENTING

---

## 1. 架构定位与设计要点

### 1.1 策略拓扑校验去噪
- **问题**：`resolver.Resolve` 在应用全局过滤时产生 `DiagnosticSeverityInfo` 级别的 `global_filter_excluded` 诊断。`application/policy/service.go` 的 `Validate` 将所有非 `empty_routed_group`/`empty_group` 诊断不加区分地全量转成 `ValidationIssue`；前端只要 `issues.length > 0` 立即将校验框标黄为“存在配置警告”，把原本正常的分流节点排除误当成策略拓扑配置错误。
- **方案**：
  1. `policy.Service.Validate`: 过滤掉 `DiagnosticSeverityInfo`（尤其是 `global_filter_excluded`），仅保留 `DiagnosticSeverityError` 及显式策略组空置警告（`empty_group`）；
  2. `PolicyView.vue`: 检查有效警告，仅在存在 `severity: 'warning' | 'error'` 时呈现黄色警告框。

### 1.2 策略组「继承全局过滤」开关 (Inherit Global Filter)
- **问题**：全局过滤器（如仅保留可用节点）作用于全系统，导致未探活或特殊节点（如 Cloudflare WARP WireGuard 节点）被从候选池中剔除，进而使专属策略组（如 WARP）由于无可用节点而清空并阻断导出。
- **方案**：
  1. 数据库迁移 `000024_inherit_global_filter_and_probe_governance.sql`：
     ```sql
     ALTER TABLE node_groups ADD COLUMN inherit_global_filter INTEGER NOT NULL DEFAULT 1;
     UPDATE node_groups SET inherit_global_filter = 0 WHERE UPPER(name) = 'WARP' OR id = 'WARP';
     ```
  2. 领域层 `domain.NodeGroup` 增加 `InheritGlobalFilter bool`；
  3. `internal/resolver/graph.go`:
     - 策略组在计算成员候选节点时，检查 `group.InheritGlobalFilter`；
     - 若为 `true`：从 `globalAdmittedNodes` 中挑选；
     - 若为 `false`：从 `admissionRes.AdmittedNodes` 中挑选；
  4. `internal/resolver/resolver.go`:
     - 确保 `snapshot.Nodes` 包含所有被任何策略组引用的节点，保证编译器 `proxies:` 完整包含目标组引用的所有节点；
  5. 前后端接口与界面同步支持该字段。

### 1.3 节点探针健康状态极净化 (收敛消除「已过期」与「待复核」)
- **问题**：
  - `defaultProbeFreshnessTTL = 1 * time.Hour`：节点超过 1 小时未被探测就会被置为 `stale` / `undetermined`，导致客户端订阅节点随时间流逝突然丢失；
  - `nodeView.ts` 中只有 `node_connect_failed` 算 `unhealthy`，超时、连接拒绝、DNS 解析失败等所有其他错误都被强行推脱成“待复核（undetermined）”，造成“不可用为0、待复核几十个”的虚假指标。
- **方案**：
  1. 移除自然时间超过 1 小时强制变 `stale` 的逻辑：只要节点的连接凭据版本（`connection_revision`）一致，上一轮的探测结果持续有效，直到新一轮探测覆盖它；
  2. `nodeView.ts`: 凡 baseline 探测报错（`verdict === 'error'`），均归为 `unhealthy`（不可用），彻底废弃将 timeout / transport_error 归入 `undetermined` 的多余分支；
  3. 节点卡片清晰展示三种核心状态：可用（绿）、不可用（红，附具体原因如“超时”）、未测试（灰）。

### 1.4 探针探测超时动态可配置
- **问题**：Mihomo 出站客户端、Sing-box 运行参数及 Probe Runner 均硬编码 15s 或 30s 超时，无法根据用户需求灵活调整。
- **方案**：
  1. `settings` 表增加 `probe_timeout_seconds INTEGER NOT NULL DEFAULT 5`；
  2. `domain.Settings` 增加 `ProbeTimeoutSeconds int`；
  3. 探针拨号与任务超时使用该配置；
  4. 前端设置页 `SettingsView.vue` 支持配置。

### 1.5 探针历史记录治理与滚动上限
- **问题**：现存 14,000+ 条 `probe_runs`、80,000+ 条 `probe_observations` 无清理机制，数据库冗余庞大。
- **方案**：
  1. 迁移中修剪历史：
     ```sql
     -- 保留最新 100 条 probe_runs，删除多余记录
     DELETE FROM probe_runs WHERE id NOT IN (SELECT id FROM probe_runs ORDER BY started_at DESC LIMIT 100);
     DELETE FROM probe_batches WHERE id NOT IN (SELECT id FROM probe_batches ORDER BY started_at DESC LIMIT 100);
     -- 保留最新 100 条 run 对应的 observations，以及每个节点每个 kind 最新的 1 条 observation
     DELETE FROM probe_observations 
     WHERE run_id NOT IN (SELECT id FROM probe_runs)
       AND id NOT IN (
         SELECT po.id FROM probe_observations po
         JOIN (
           SELECT node_logical_id, probe_kind, MAX(observed_at) AS max_at
           FROM probe_observations GROUP BY node_logical_id, probe_kind
         ) latest ON po.node_logical_id = latest.node_logical_id AND po.probe_kind = latest.probe_kind AND po.observed_at = latest.max_at
       );
     ```
  2. 运行时修剪：`internal/application/probe` 在每次 run 完成后触发异步滚动修剪，限制最多保留 100 批次。

### 1.6 纠正批次跳过文案
- **方案**：在 `web/src/features/probes/ProbesView.vue:2018` 将“已跳过 X 个凭据缺失或无效的节点”改为“本次跳过 X 个状态最新/无需重测的节点”。

---

## 2. 数据库迁移方案 (Migration 000024)
- `migrations/000024_inherit_global_filter_and_probe_governance.sql`:
  1. `ALTER TABLE node_groups ADD COLUMN inherit_global_filter INTEGER NOT NULL DEFAULT 1;`
  2. `UPDATE node_groups SET inherit_global_filter = 0 WHERE UPPER(name) = 'WARP' OR id = 'WARP';`
  3. `ALTER TABLE settings ADD COLUMN probe_timeout_seconds INTEGER NOT NULL DEFAULT 5;`
  4. 滚动修剪历史冗余 `probe_runs` / `probe_batches` / `probe_observations`。

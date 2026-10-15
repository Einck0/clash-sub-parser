# Capability Contract: Confirmed Fixes Subset (pkg_confirmed_fixes)

- **Feature Directory**: `specs/002-policy/`
- **Work Package**: `pkg_confirmed_fixes`
- **Status**: `READY` (Ready for Implementation & Self-Test)
- **Bound Run ID**: `pi_run_20261011_190715_2256208`
- **Change**: `002-policy`
- **Whole Feature Policy Status**: `DRAFT_AWAITING_CORE_DIRECTION_AND_CORRECT_CONTRACT` (NOT READY for whole 002; Candidate A vs Candidate B pending user decision via QQ)

---

## 1. 契约背景与对齐说明 (Provenance & Alignment)

在策略组核心重构中，用户核心排除算法已正式确认采纳『候选 A 顺序重入流水线』，候选 B 全局一票否决彻底废除。消除所有 A/B pending 阻塞项。
与此同时，用户已明确采纳并确认两项独立于政策排除算法的关键工程修复需求：
1. **库存拉取资产保全与源关系留存**：拉取失败或部分成功（Partial）时保全 `node_sources` 与历史节点；完全成功刷新更新 sources（修剪未见关联并归档历史），但不删除存量 `nodes` 资产；多订阅刷新异常不产生全局失活；
2. **纯节点导出与无关空策略组解耦**：SingBox、Surge、QuantumultX 纯节点导出仅验证实际节点准入（协议、凭据、IP 风险、活跃状态、节点过滤器），不受无关空策略组阻断；Mihomo 保持完整策略组与空组强校验；保留节点过滤器 Fail-Closed 机制，严禁滥用 PASS 兜底。

本契约作为 `002-policy` 特性中可独立推进的最小原生能力工作包（`pkg_confirmed_fixes`），严格对齐上游已核准规范：
- 对齐 `specs/001-inventory/spec.md` 中 **FR-INV-02** (抓取失败与零有效节点资产保全) 与 **FR-INV-07** (订阅刷新数据修剪与重试策略)；
- 对齐 `specs/004-publication/spec.md` 中 **FR-PUB-02** (非 Mihomo 纯节点导出协议边界) 与 **FR-PUB-03** (非 Mihomo 遇到空策略组校验一致性)。

---

## 2. 详细技术需求与代码缺陷基线 (Requirements & Deficiency Baseline)

### 2.1 需求 1: 库存拉取失败与部分成功时的源保留 (Inventory Source & Node Preservation)
- **上游规范对齐**: `specs/001-inventory/spec.md#req-inv-zero-loss`, `#req-inv-prune-retry`
- **当前实机代码缺陷**:
  在 `internal/application/inventory/service.go` 的 `ReconcileSubscription` 事务中，当前代码在拉取状态为 `domain.FetchOutcomePartial`（部分节点解析错误或被拒）时，仍然无条件执行了：
  ```sql
  DELETE FROM node_sources WHERE subscription_id = ? AND last_seen_fetch_id != ?;
  ```
  这导致在部分拉取异常或远端下发部分残缺配置时，先前合法的节点源关系被非预期修剪。
- **目标契约与行为不变式**:
  1. **失败与部分拉取源保留 (Failed & Partial Retention)**：当刷新结果为 `FetchOutcomeFailed` 或 `FetchOutcomePartial` 时，**严禁修剪 `node_sources`**。先前已绑定的存量源关联与 `node_source_history` 记录 100% 完整保留。
  2. **完全成功差异更新 (Successful Diff Updates)**：仅当刷新结果严格为 `domain.FetchOutcomeSuccess` 时，才允许执行 `node_sources` 差异修剪（针对最新快照中未出现的节点，先写入 `node_source_history` 归档证据，再从 `node_sources` 移除当前订阅关联）。
  3. **存量节点物理资产零删除 (Non-destructive Node Invariant)**：无论刷新成功、部分成功或失败，`nodes` 表中的节点记录本身绝对不得物理删除或级联失活（`node.active` 保持原状）。
  4. **多订阅刷新互不干扰 (Multi-sub Failure Isolation)**：订阅 A 的抓取失败或返回 0 节点，绝对不得导致订阅 B 的节点源关系发生变化或全局失活。

### 2.2 需求 2: 非 Mihomo 纯节点导出解耦与空组穿透 (Publication Pure-Node Export Decoupling)
- **上游规范对齐**: `specs/004-publication/spec.md#req-pub-non-mihomo-export`, `#req-pub-empty-group-valid`
- **当前实机代码缺陷**:
  1. 在 `internal/application/publication/service.go` 的 `PreviewCompatible`（兼容预览）中，对所有目标遍历策略组，若策略组成员为 0 且未启用 PASS，直接返回 `domain.NewValidationError("empty_group_not_allowed", ...)`，导致纯节点导出目标在兼容预览时被策略组空状态意外阻断；
  2. 编译器 `internal/compiler/compiler.go` 的 `validate` 与发布服务四动作（`Validate`, `RawPreview`, `Publish`, `Serve`）缺乏对纯节点目标（SingBox, Surge, QuantumultX）与拓扑级目标（Mihomo）的一致性解耦。
- **目标契约与行为不变式**:
  1. **纯节点四动作空组穿透**：针对 `domain.TargetSingBox`、`domain.TargetSurge`、`domain.TargetQuantumultX` 三大纯节点目标，在 **validate (预校验)**、**rawpreview (实时预览)**、**publish (发布创建)**、**serve (端点导出)** 四大动作中，仅校验实际节点准入合规性（有效协议、凭据信封、IP 风险决策、活跃状态与节点过滤器匹配结果），**绝不因策略组为空（0 members）或策略组缺失而阻断**。
  2. **节点过滤器 Fail-Closed 保留**：若节点过滤器本身过滤后导致可用节点数为 0，系统依然严格触发 `filtered_nodes_empty` 或对应 Fail-Closed 门禁，防止导出空节点配置造成客户端连接故障。
  3. **Mihomo 拓扑强校验保全**：Mihomo 内核必须严格保持策略组非空或显式 `EmptyFallbackPass` 校验，未配置 PASS 兜底的空策略组依然严格拒绝发布。
  4. **严禁滥用 PASS 兜底 (No Synthetic PASS Catch-all)**：不得向非 Mihomo 目标注入伪造的 PASS 代理或出站规则，保持各目标原生格式纯洁性。

---

## 3. 写集合与串行化规则 (Write Sets & Serialization Discipline)

- **本工作包独占写集合**:
  - `internal/application/inventory/service.go`
  - `internal/application/inventory/reconcile_preservation_test.go`
  - `internal/application/publication/service.go`
  - `internal/application/publication/service_test.go`
  - `internal/compiler/compiler.go`
  - `internal/compiler/xhttp_and_empty_group_test.go`
  - `internal/compiler/compiler_test.go`
- **共享依赖串行化铁律**:
  `publication/service.go` 与 `compiler.go` 在后续策略组后端大包（`pkg_policy_backend`）中亦有修改。因此，当前工作包必须作为**独立原子包率先完整实现与自测闭环**；未来策略组后端开发必须在明确读取本包改动后的稳定基线上串行推进，严禁并发踩踏。

---

## 4. 验收测试目标与执行规范 (Verification Targets)

施工机在推进 `pkg_confirmed_fixes` 施工时，必须编写并执行以下针对性回归测试用例：

1. **测试用例 1: 部分成功刷新保留全部存量源关系与历史**
   - 测试标识: `TestReconcile_PartialRejected_RetainsSourcesAndHistory`
   - 目标文件: `internal/application/inventory/reconcile_preservation_test.go`
   - 验证点: 初始订阅 2 个节点；二次拉取 1 个新节点与 1 个残缺非法节点（结果为 `Partial`）；断言旧节点的 `node_sources` 未被删除，新节点正常写入，存量节点均处于 active 状态。
2. **测试用例 2: 完全成功刷新执行差量修剪且存量节点资产不删除**
   - 测试标识: `TestReconcile_SuccessfulDiffUpdates_RetainsNodeAssets`
   - 目标文件: `internal/application/inventory/reconcile_preservation_test.go`
   - 验证点: 初始订阅包含节点 A 与 B；二次完全成功拉取仅包含节点 A；断言节点 B 从 `node_sources` 修剪并写入 `node_source_history` 归档记录，但节点 B 在 `nodes` 表中依然存在且未被删除。
3. **测试用例 3: 多订阅拉取异常互不干扰隔离验证**
   - 测试标识: `TestReconcile_MultiSubscriptionFailure_NoGlobalDeactivation`
   - 目标文件: `internal/application/inventory/reconcile_preservation_test.go`
   - 验证点: 订阅 1 包含 10 个有效节点；订阅 2 拉取失败（返回 HTTP 500）或返回 0 节点；断言订阅 1 的所有节点依然有效且未被全局失活。
4. **测试用例 4: 纯节点目标四动作空策略组穿透**
   - 测试标识: `TestPublication_PureNodeTargets_IgnoreEmptyGroups_AcrossAllActions`
   - 目标文件: `internal/application/publication/service_test.go`, `internal/compiler/xhttp_and_empty_group_test.go`
   - 验证点: 构建包含 0 成员策略组但包含有效节点的快照；针对 SingBox, Surge, QuantumultX 执行 validate, preview/rawpreview, publish, serve；断言全部成功返回合法纯节点配置，无 `empty_group_not_allowed` 错误。
5. **测试用例 5: 纯节点目标保留节点过滤器 Fail-Closed**
   - 测试标识: `TestPublication_PureNodeTargets_RetainNodeFilterFailClosed`
   - 目标文件: `internal/application/publication/service_test.go`
   - 验证点: 节点过滤器导致 0 节点匹配；针对四大目标断言依然触发 `filtered_nodes_empty`，安全拦截。
6. **测试用例 6: Mihomo 保持严格空策略组校验**
   - 测试标识: `TestPublication_Mihomo_RetainsEmptyGroupValidation`
   - 目标文件: `internal/application/publication/service_test.go`
   - 验证点: 包含空策略组且无 PASS 时，Mihomo 发布与预检严格失败。

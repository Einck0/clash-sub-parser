# Implementation Tasks: Policy Topology Hygiene, Inherited Filter & Probe State Governance

- **Feature Directory**: `specs/002-policy/`
- **Run ID**: `pi_run_20261017_203412_2919528`
- **Change**: `002-policy`
- **Work Package**: `default`
- **Epoch**: 1
- **Status**: IMPLEMENTING

---

## 任务执行序列

- [x] **T01 [MIGRATION & STORAGE]** 数据库迁移与历史数据治理：
  - 编写 `migrations/000024_inherit_global_filter_and_probe_governance.sql`：
    - `node_groups` 表新增 `inherit_global_filter INTEGER NOT NULL DEFAULT 1`；
    - 执行 `UPDATE node_groups SET inherit_global_filter = 0 WHERE UPPER(name) = 'WARP' OR id = 'WARP';`；
    - `settings` 表新增 `probe_timeout_seconds INTEGER NOT NULL DEFAULT 5`；
    - 清理超出最新 100 批次的历史 `probe_runs`、`probe_batches` 与非最新的历史 `probe_observations`；
  - 更新 `domain.NodeGroup` 与 `domain.Settings` 模型字段；
  - 更新仓储层 `internal/repository/sqlite/policy.go`、`consistent_facts.go`、`settings.go` 与 `revision_snapshots.go` 支持读写新字段。

- [x] **T02 [RESOLVER & TOPOLOGY]** 策略拓扑校验去噪与独立组过滤逻辑：
  - `internal/application/policy/service.go`: `Validate` 方法过滤掉 `DiagnosticSeverityInfo`（尤其是 `global_filter_excluded`），仅保留 `DiagnosticSeverityError` 及显式空组风险；
  - `internal/resolver/graph.go`: `resolveGroups` 根据策略组的 `InheritGlobalFilter` 决定从 `globalAdmittedNodes` 还是全量 `AdmittedNodes` 挑选候选节点；
  - `internal/resolver/resolver.go`: 确保 `snapshot.Nodes` 包含被任何策略组引用的所有节点，保证编译器 `proxies:` 与策略组引用 100% 闭环；
  - 完善单元测试。

- [x] **T03 [PROBE ENGINE & TIMEOUT]** 探针状态健全化与超时配置生效：
  - `internal/application/inventory/service.go`: 移除自然时间超过 1 小时强制将观测置为 `stale` 的设计，保留有效探测结果直至被新探测覆盖；
  - 探针运行器 (`internal/application/probe/runner.go` 与 `mihomo/client.go`): 读取 `ProbeTimeoutSeconds` 配置作为单节点探测与出站握手的最大超时时间；
  - 探针运行器增加运行时历史自动修剪：每次批次完成时保留最新 100 批次记录。

- [x] **T04 [FRONTEND UI]** 前端页面极简化与文案纠偏：
  - `PolicyView.vue`: 拓扑校验横幅仅在存在 error 或 warning 时标黄/标红，普通过滤节点不再触发警告；
  - `PolicyEditorSheet.vue`: 增加「继承全局过滤」开关及说明；`GroupCard.vue` 在关闭时展示徽标；
  - `nodeView.ts`: 凡 baseline 探测报错（包含 timeout 等网络错误）一律归类为 `unhealthy`（不可用），彻底废除将 timeout 归入 `undetermined`（待复核）的分支；
  - `ProbesView.vue`: 修正跳过文案为“本次跳过 {{ batch.counts.skipped_nodes }} 个状态最新/无需重测的节点”；
  - `SettingsView.vue`: 增加「探针探测超时 (秒)」设置项；
  - 更新相关的前端单元测试并保证 `vue-tsc` 检查通过。

- [x] **T05 [SELF-TEST & PREVIEW]** 施工子机自测与隔离容器成品预览准备：
  - 在 Docker 容器内执行 Go 单元测试与前端 Vitest 回归；
  - 启动隔离端口（28081）预览实例，验证 WARP 策略组在全局过滤下依然正常解析出节点，验证拓扑校验不再报全局过滤警告，验证探针超时与节点健康状态分类；
  - 交付包含标准 `instanceId`、`url`、`testDataBoundary`、`cleanupCommand` 的预览清单。

- [x] **T06 [REVIEWER]** 独立 Reviewer 只读审查：
  - 对照用户指令核查：去噪彻底性、WARP 独立组继承逻辑、待复核消除与真实不可用判定、探针超时动态配置、历史数据清理与文案纠偏；
  - 出具独立审查结论与治理报告。

- [x] **T07 [CRITIC]** Critic 黑盒与多模态视觉验收：
  - 在隔离预览环境中黑盒验证桌面与移动端 UI 体验；
  - 查看策略组视图、节点视图卡片状态、设置页与探针批次页面截图；
  - 出具 `CRITIC_VISUAL_VERDICT: PASSED` 与治理报告。

- [ ] **T08 [DEPLOY]** 提交源码、推送 dev、官方 Actions 云构建与平滑更新生产：
  - 提交确证源码，推送至 `dev`；
  - 跟踪 GitHub Actions 完成构建与不可变镜像发布；
  - 生产 SQLite 数据库一致性热备（并验证历史数据清理与 VACUUM）；
  - 平滑更新生产容器，在线验证 `https://sub.einck.top`。

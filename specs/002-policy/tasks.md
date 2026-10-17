# Implementation Tasks: Policy Routing Rules Modernization & Legacy 470 Rules Restoration

- **Feature Directory**: `specs/002-policy/`
- **Run ID**: `pi_run_20261018_022832_3123769`
- **Change**: `002-policy`
- **Work Package**: `default`
- **Epoch**: 3
- **Status**: IMPLEMENTED

---

## 任务执行序列

- [x] **T01 [MIGRATION & SEEDING]** 数据库结构重构与 470 条规则完整回填：
  - 编写 `migrations/000025_rule_categories_and_builtin_targets.sql`；
  - 创建 `rule_categories` 表（`id`, `name`, `sort_order`, `created_at`, `updated_at`）；
  - 重建 `policy_rules` 表，解除外键硬绑定并新增 `category_id`, `category`, `name`, `builtin_target`, `enabled` 字段及合法性 CHECK 约束；
  - 完整迁移回填 `/data/clash_sub_parser.db` 中的 14 个分类与全部 470 条分流规则（DIRECT 292条、REJECT 15条、策略组163条），确保总数与顺序分毫不差。

- [x] **T02 [DOMAIN & REPOSITORY]** 领域实体扩展与仓储分类保序：
  - 更新 `internal/domain/policy.go`：新增 `RuleCategory`，扩展 `PolicyRule`（支持内置动作及分类关联），优化 `ValidatePolicyRules` 支持内置目标放行与 MATCH 严格置底；
  - 更新 `internal/domain/ports.go`：增加分类 CRUD 与规则更新/重排序接口；
  - 更新 `internal/repository/sqlite/policy.go` 与 `consistent_facts.go`：支持分类与规则的完整读写与联合保序查询；
  - 更新 `internal/resolver/graph.go`：`resolveRules` 支持将 `builtin_target` 映射为 `TargetGroupName`，并保证 MATCH 终结规则绝对置底。

- [x] **T03 [SERVICE & HTTP API]** 服务层与控制面 API 适配：
  - 更新 `internal/application/policy/service.go`：
    - `Validate` 方法针对内置动作（DIRECT/REJECT）跳过策略组存在性与空组校验；
    - 新增 `ListRuleCategories`、`CreateRuleCategory`、`UpdateRuleCategory`、`DeleteRuleCategory`、`ReorderRuleCategories`；
    - 升级 `CreatePolicyRule`、`UpdatePolicyRule`、`ReorderPolicyRules`；
  - 更新 `internal/transport/http/policies.go` 与 `response.go`：暴露分类与规则的 RESTful 端点，覆盖分类列表、增删改查、排序以及规则筛选与更新，提供 `ParsePaginationWithLimit` 支持大容量规则与策略组单页拉取。

- [x] **T04 [FRONTEND RULE DESK]** 顶级规则管理工作台 UI 实现：
  - 注册顶级导航：更新 `web/src/navigation.ts`、`web/src/router/index.ts` 与 `web/src/App.vue` 侧边栏，恢复顶级独立菜单「规则管理」（`/#/rules`）；
  - 实现 `web/src/features/rules/RulesView.vue`、`useRules.ts` 与规则/分类对话框；
  - 支持 14 个分类分组展示、分类手风琴折叠展开、分类权重排序、组内规则上下移动保序；
  - 移动端 392px 视口无截断自然换行，底部增加 `pb-24` 避让悬浮 Dock；
  - 支持内置动作（DIRECT/REJECT/PASS）与策略组直观选择与彩色徽章区分；
  - 完善中英文双语字典（`web/src/locales/messages.ts`）。

- [x] **T05 [SELF-TEST & PREVIEW]** 施工子机自测与隔离容器成品预览准备：
  - 在 Docker 容器内运行 Go 单元测试与前端 Vitest 回归，并通过 `vue-tsc` 类型检查；
  - 启动隔离端口（28081）预览实例，验证 470 条规则及 14 个分类在界面上的渲染与交互；
  - 测试直链导出 `/clash`、`/singbox`、`/surge`、`/qx`，验证各客户端格式中 470 条规则与内置动作正确导出且 MATCH 绝对置底；
  - 交付标准预览清单（`instanceId`, `url`, `testDataBoundary`, `cleanupCommand`）。

- [x] **T06 [REVIEWER]** 独立 Reviewer 只读审查：
  - 对照 Spec Kit Feature Spec 验证：470 条规则完整性、DIRECT/REJECT 内置目标支持、分类权重与组内保序、MATCH 绝对置底、API 与 UI 规范；
  - 出具独立审查结论与治理报告（Epoch 3 VERDICT: PASS）。

- [x] **T07 [CRITIC]** Critic 黑盒与多模态视觉验收：
  - 在隔离预览环境中黑盒验证桌面端 (1440x900) 与移动端 (392x872) 规则管理 UI 体验（手风琴折叠、分类权重、新建/编辑规则弹窗、搜索过滤、悬浮 Dock 避让）；
  - 验证 Clash 导出 470 条规则且 MATCH 绝对置底；
  - 出具 `CRITIC_VISUAL_VERDICT: PASSED` 与治理报告（Epoch 3 VERDICT: PASS）。

- [x] **T08 [DEPLOY]** 提交源码、推送 dev、官方 Actions 云构建与平滑更新生产：
  - 提交确证源码，推送至 `dev` 分支；
  - 跟踪 GitHub Actions 完成镜像云端构建；
  - 生产 SQLite 数据库一致性热备（`.backup`）；
  - 平滑更新生产容器，在线验证 `https://sub.einck.top` 与 `/clash` 导出；
  - 向随身 QQ 终端发送终态战报。

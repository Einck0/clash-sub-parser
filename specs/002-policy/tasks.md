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


## 当前增量任务：节点/探针/策略筛选重做（前述 T01..T08 为历史）

当前 run `pi_run_20261020_015412_446000`，change `002-policy`，work_package `default`，epoch 1；R02–R10 已完成；业务提交 `778b579`，隔离预览已清理，最终报告随本规范记录交付。此前规划 BLOCKED 与上一实施轮 TIMEOUT 均保留在 redesign-execution-status.md；本轮不沿用历史发布状态。

- [x] R01 主脑完成有界证据与业务收敛，spec/plan/tasks 增量一致性分析；契约 probe-ledger-redesign.md。
- [x] R02 [US-R3] domain/subscription.go、application/subscription、repository/sqlite、transport/http 贯通 config.probe_policy；缺省继承、部分更新、非法周期/模式测试。
- [x] R03 [US-R2/3] application/probe/periodic.go 与 wiring 实现多来源最短有效周期、停测/停源/手动/版本/冷却/恢复/去重；容器定向测试。不引入每源定时器。
- [x] R04 [US-R1/2/3] web/src/features/nodes、settings、subscriptions 重做统一节点操作工作台、全局探针配置及来源周期表单；导航/router/dashboard 去除第二入口、旧地址直达；测试选择/分页/异常/卸载清理。
- [x] R05 [US-R4] web/src/features/policy 的全局/组内筛选共享条件编辑，候选范围/添加/排除/实时结果；保留有序高级来源和动态正则，防抖/旧请求隔离；同步 locales/测试。
- [x] R06 [US-R4] internal/resolver/graph.go 与 application/policy/service.go 统一候选范围与事实提供，预览/校验/导出一致且失败不静默；WARP/子组/来源变动/规则MATCH回归。
- [x] R07 容器 Go/Vitest/typecheck/build 全量自测，更新相关稳定使用说明但保护用户改动；交付独立预览、合成数据场景及 URL-ready 元数据、源码 hash。
- [x] R08 独立 reviewer 审查最终整合状态；失败项保留历史并整改复验。
- [x] R09 工程门禁放行后 critic 桌面/移动视口真实流程与截图：节点检测操作、全局配置/源周期保存、策略动态筛选、错误及脏 token；本轮合同明确 critic=true，记录真实桌面/移动视口验收。
- [x] R10 全部适用门禁通过后逐文件提交，预览清理、QQ 交付；不部署生产，不沿用前序上线授权。

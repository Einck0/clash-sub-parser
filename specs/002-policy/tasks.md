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

## 2026-10-20 授权生产发布（独立于此前实施闭环）

Run `pi_run_20261020_083521_627930` / change `002-policy` / package `default` / epoch 1。用户现已授权上线；旧“不推送/不部署”仅约束旧轮。本轮只发布已审查提交 `6586b008ccda3ace18a29c43fbbff23f5756a902`，不重做业务，不处理由前台另行执行的磁盘清理和自动清理脚本。

- [x] D01 推送既有 dev 提交并取得同 SHA 云端检查、镜像发布与 prerelease 凭据，不在宿主编译。
- [x] D02 拉取不可变 digest，核验源码/二进制版本；一致性热备、回滚镜像和凭据/卷保全；新云镜像隔离运行验证与发布差量独立审查。
- [x] D03 按真实 owner Compose image-only 替换 app；核验镜像/revision、健康就绪、核心业务数量、鉴权与公开导出，不以首页 200 代替成功。失败按 schema 兼容性回滚，不自动覆盖业务数据库。
- [x] D04 最终独立只读核验、清理仅本轮隔离资源、记录真实发布证据与 QQ 交付。

发布路径沿用现有 plan 阶段 C 与云交付手册，源码门禁复用原实施有效证据，本轮仅验证云制品及运行变化。当前无新增 schema 迁移；若发现部署链路需要源码/公共接口变更，先回报，不临时改业务或 CI 绕门禁。

发布结果：同 SHA 云任务 39349217297 三 job success，生产 image digest `211f442205b26656e00cda8a14f610dd61c6341de182ea7555c3aab3031600ff` 已上线，Schema25、Clash470/MATCH末位、资源指纹与数据保全经独立核验；隔离资源及敏感副本已回收。其他客户端遇 HTTP 协议节点严格422为已知限制，不宣称全目标正常。详见 [release-20261020.md](release-20261020.md)。磁盘与自动清理脚本不在本发布执行范围。

## 2026-10-20 当前实施：筛选、DNS、待复核（旧R/D勾选不代表本轮）

Run `pi_run_20261020_111741_708234` / change `002-policy` / work_package `default` / epoch 1。详细边界见 filter-dns-redesign.md。该run为历史规划断点；当前续办验收见 [continuation-1346392.md](continuation-1346392.md)。

- [x] S01 主脑定位待复核/区域条件/DNS消失真实调用链与生产只读概要，完成详细业务/实现方案并Spec Kit一致性分析。
- [x] S02 [FR-S1] domain/probe_health.go、application/{inventory,probe}、sqlite/node_predicates.go三态单一事实，去掉健康/筛选时效与旧待复核；真实失败/版本/操作态/SQL守恒回归。
- [x] S03 [FR-S2] domain/node_filter.go与facts依赖加载、policy/resolver：health_status/exit_region/platform_verdict、baseline延迟及legacy probe条件，零时效/真实地区/同事实预览导出测试。
- [x] S04 [FR-S4] 新migration/DNS领域与repo、Settings GET/PUT及409、snapshot与Mihomo编译贯通；默认不导入旧库，复杂Raw无损与安全输入/持久化/导出测试。
- [x] S05 [FR-S3] web policy编辑共享组件/types改单草稿一次保存；真实来源名称、独立维度、普通/高级有序来源无损，计数原因解释；行为测试。
- [x] S06 [FR-S1/4] web节点/详情去待复核时效与操作态覆盖；恢复DNS页面/nav/router/Settings跳转、Raw/可视化/并发保存/错误回读及响应式测试。
- [x] S07 最终合并Docker Go/Vitest/typecheck/build，合成独立DB与无出网真实预览、源码指纹/健康/回收合同。
- [x] S08 独立Reviewer对FR-S1..4及旧资产/WARP/MATCH/失败路径审查；缺陷整改保留失败复验链。
- [x] S09 主脑亲自真实业务流程及Critic桌面/移动视口验收：已有香港组改节点状态和出口地区单次保存、DNS持久化与导出、无待复核和时效回潮；不宣称实屏。
- [x] S10 适用门禁全部通过后逐文件本地提交，清理预览，完整QQ交付三项目标；不push/部署，不影响清盘任务。

## 2026-10-21 当前实施：DNS数据与策略去重

当前run pi_run_20261021_160307_1574174/default/epoch1，详细冻结业务见dns-policy-cleanup-1574174.md；旧任务勾选仅历史。

- [x] C01 主脑核实唯一已确认旧DNS、生产准入0、重复页签与额外拦截调用链，冻结FR-C1..3模块/接口/场景。
- [x] C02 [FR-C1] 单DNS基线与在线backup、候选validate、CAS恢复17键；GET及实际/clash深等价与静态数据保全，记录恢复来源，不整库恢复。
- [x] C03 [P] [FR-C1/2] web DNS去header/defaultbanner；policy视图及usePolicy去重复路由/准入/模拟CRUD，保留组编辑与唯一Rules导航，更新行为tests。
- [x] C04 [P] [FR-C3] resolver/admission.go去旧DSL保inactive/riskexcluded；历史输入一次warning，更新WARP/globalsoft/hard边界tests。
- [x] C05 [P] [FR-C3] application/policy与HTTP所有admission写422、无revision副作用、GET历史保留、删除不误删同IDpolicy；修分类新规则validation顺序并保MATCH唯一/末位，更新service/HTTP/revisions相关tests。
- [x] C06 主脑核对源diff/secret、逐文件开发commit并pushdev；Actions同SHA前后端必需jobs全成功，云失败保留/修复后新提交，不本地编译。
- [x] C07 同SHA不可变云镜像+revision核对、合成隔离DB/禁调度/无出网预览，executor云自测凭据，独立reviewer核源码/当前Actions/生产DNS恢复；按发现针对整改复验。
- [x] C08 真实desktop/mobilebrowser：DNS无横幅且编辑工作；policy无重复tabs而Rules创建DIRECT/REJECT/分类/MATCH正常；组filter/WARP/legacyadmission422与硬约束不回归，Critic亲读PNG，主脑实际风险复验。
- [x] C09 回收专属预览/敏感副本保真实backup，记录开发commit/cloudreview/验收/生产DNS来源与上线未授权状态，QQ交付；程序部署如需另请示。

终态与当前SHA/云测、独审、桌面及移动浏览器证据见 [cleanup-completion-1574174.md](cleanup-completion-1574174.md)。C09交付对应当前会话及QQ完工报告，程序部署仍未授权、未执行。旧S系列的Docker/不push限制只记历史，不适用于本轮。

## 2026-10-25 DNS 前端重设计（当前实施）

身份 pi_run_20261025_123522_590576 / change_pi_run_20261025_123522_590576 / whole_task / epoch 1。本轮所有任务按 [dns-ui-redesign.md](dns-ui-redesign.md)；旧勾选不代表本轮门禁通过。

- [x] D25-00 主脑核对入口dirty/AGENTS、官方Feature解析、规格/模块契约及speckit-analyze一致性，保存规划原基线。
- [x] D25-01 [P] [DU2/DU3/DU4] 草稿模块 useDnsDraft.ts/dnsTypes.ts/useDnsDraft.test.ts：键删除、对象校验、预览失效、异步防护及相应回归；不本地运行测试编译，待同SHA云测收敛。
- [x] D25-02 [P] [DU1/DU2/DU3/DU5] 呈现模块 DnsView.vue/DnsView.test.ts 与必要专属字段组件：主路径、高级分组、三态/重置、busy锁定、可访问响应式及中英文，消费冻结A接口。
- [x] D25-03 主脑逐文件核对→开发提交推dev→同SHA Actions checks/runtime必需成功（dev发布job skipped预期）；施工机提供当前源码云测结果与自测报告，禁止本地编译。
- [x] D25-04 同SHA云镜像digest/OCI revision、合成DB隔离预览与API/导出验收、独立reviewer后实际UI critic桌面/移动视口与截图；失败保留并定向整改复验。
- [x] D25-05 图工具可用性如实报告，核对任务现场/回收专属预览、逐文件最终提交、QQ交付完成/未做，禁止生产部署。

当前D25终态证据、失败整改链、当前业务SHA、云测/镜像/独审/视觉复验及隔离清理见 [dns-ui-completion-590576.md](dns-ui-completion-590576.md)。程序和生产DNS配置未上线/未写入。

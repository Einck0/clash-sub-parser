---
feature: 002-policy
title: Policy Capability Specification (Topology Hygiene, Inherited Filter, Rule Desk & 470 Rules Restoration)
status: IMPLEMENTING
schema: speckit-native
---

# Policy Capability Specification (去过度设计、健全性治理与规则管理现代化)

## Purpose

本规范定义 CSP 1.0 的策略与分流管理体系。在完成策略拓扑校验去噪、策略组全局过滤继承控制（WARP 直通）、探针状态极简化与历史滚动的生产落地后，**本阶段聚焦于全面恢复规则管理工作台，彻底根除规则丢失缺陷，并演进为现代分流规则业务形态**：
1. **彻底解除外键硬绑定，全面支持内置动作**：解除 `policy_rules.target_group_id` 对 `node_groups` 的硬外键限制，原生支持 `DIRECT`、`REJECT`、`REJECT-DROP`、`PASS` 等标准内置动作（Built-in Policy Targets），使规则能自由流向直连、阻断或自定义策略组；
2. **完整无损回填 14 个分类与 470 条分流规则**：基于历史基准 `/data/clash_sub_parser.db`，一次性完整回填 14 个 `rule_categories` 与全部 470 条生产分流规则（含 292 条 DIRECT 与 15 条 REJECT 规则），重现完整的规则资产库；
3. **分层权重与组内/组间保序排布**：
   - **宏观组间排序**：按 `rule_categories.sort_order`（0 到 130）划分流量匹配优先级（如：广告拦截 -> 国内日常 -> 应用进程 -> 游戏/流媒体/AI -> 负载均衡 -> 国内直连 -> 常规代理 -> 兜底）；
   - **微观组内排序**：在分类内部依据规则 `position`（`sort_order`）严格保序；
   - **绝对兜底铁律**：无论分类或组内如何调整，`MATCH` 兜底规则在拓扑与编译器中必须绝对置底；
4. **左侧导航恢复顶级独立 `/#/rules` 规则管理工作台**：
   - 在侧边栏恢复独立顶级菜单「规则管理」（`/#/rules`）；
   - 提供直观的分类手风琴/卡片折叠、全局关键词快捷检索、分类权重调整、组内上下移动排布；
   - 提供结构化新建/编辑弹窗，无缝切换内置动作（DIRECT/REJECT）与策略组，并支持 `no-resolve` 等标准选项；
5. **全量多客户端编译器原生保序与导出验证**：
   - Mihomo、Surge、Sing-box、Quantumult X 全平台原生兼容分类保序与内置动作规则，直链导出实时生效。

---

## User Scenarios & Priority

### User Story 1 - 独立规则管理工作台查看与分类折叠 (Priority: P1)
- **GIVEN** 管理员登录 CSP 控制台
- **WHEN** 点击左侧导航栏顶级「规则管理」（`/#/rules`）
- **THEN** 界面清晰呈现 14 个规则分类（如广告/拦截、国内日常服务、AI、常规代理等），各分类显示规则数量徽标与权重排序；支持一键展开/折叠分类与全局实时关键字检索。

### User Story 2 - 内置动作与策略组自由流向配置 (Priority: P1)
- **GIVEN** 管理员需要新增或修改一条规则（如屏蔽广告域名或将内网网段直连）
- **WHEN** 在弹窗中选择规则类型为 `DOMAIN-SUFFIX` 或 `IP-CIDR`，目标类型切换至「内置动作」并选择 `REJECT` 或 `DIRECT`
- **THEN** 规则成功保存，列表呈现鲜明的红/绿状态徽章，编译器正确渲染 `REJECT` / `DIRECT`，拓扑校验绝不产生“目标策略组不存在”的误报。

### User Story 3 - 470 条历史规则无损恢复与保序导出 (Priority: P1)
- **GIVEN** 生产数据库执行最新迁移升级
- **WHEN** 客户端请求 `/clash`, `/singbox`, `/surge`, `/qx` 订阅导出
- **THEN** 全部 470 条规则完整呈现，严格按照「分类权重 -> 组内顺序」排列，终结兜底 `MATCH` 严格置于最末尾，无任何规则丢失或乱序。

### User Story 4 - 分类层级与组内规则移动调序 (Priority: P2)
- **GIVEN** 管理员需要调整某一类规则（如将游戏规则上提至日常服务前）或调整组内优先匹配顺序
- **WHEN** 点击分类的“上移/下移”或规则行的“上移/下移”
- **THEN** 数据库实时保存最新次序，导出订阅立即应用最新分流优先级。

---

## Requirements

### 1. 数据模型与 470 条规则回填 [FR-RULE-DATA-01]
1. 数据库创建 `rule_categories` 表：
   - `id TEXT PRIMARY KEY` (UUIDv7)
   - `name TEXT NOT NULL UNIQUE` (如 `广告/拦截`)
   - `sort_order INTEGER NOT NULL DEFAULT 0`
   - `created_at TIMESTAMP`, `updated_at TIMESTAMP`
2. `policy_rules` 表结构升级重构：
   - `id TEXT PRIMARY KEY` (UUIDv7)
   - `revision_id TEXT NOT NULL`
   - `category_id TEXT REFERENCES rule_categories(id) ON DELETE SET NULL`
   - `category TEXT NOT NULL DEFAULT 'default'`
   - `name TEXT NOT NULL DEFAULT ''` (规则描述/标签，如 `appRules:应用商店`)
   - `target_group_id TEXT REFERENCES node_groups(id) ON DELETE CASCADE` (可空)
   - `builtin_target TEXT CHECK (builtin_target IS NULL OR builtin_target IN ('DIRECT', 'REJECT', 'REJECT-DROP', 'PASS'))` (可空)
   - `expression TEXT NOT NULL`
   - `position INTEGER NOT NULL DEFAULT 0`
   - `enabled INTEGER NOT NULL DEFAULT 1`
   - 约束：`CHECK (target_group_id IS NOT NULL OR builtin_target IS NOT NULL)`
3. 迁移逻辑从 `/data/clash_sub_parser.db` 一次性回填全部 14 个分类与 470 条规则至当前活动 Revision，确保 292 条 DIRECT 规则与 15 条 REJECT 规则 100% 恢复。

### 2. 领域层、仓储层与解析保序 [FR-RULE-DOMAIN-02]
1. `domain.PolicyRule` 扩展 `CategoryID`, `Category`, `Name`, `BuiltinTarget`, `Enabled` 字段；
2. `domain.RuleCategory` 实体支持 CRUD 与排序；
3. `internal/domain/policy.go` `ValidatePolicyRules`：
   - 规则若设置了 `BuiltinTarget`，校验其合法性，不要求存在对应的 `node_groups`；
   - 规则若设置了 `TargetGroupID`，校验其必须在 `groupSet` 中存在；
   - 绝对置底校验：MATCH/FINAL 规则必须作为整套规则集的最后一条；
4. `internal/resolver/graph.go` `resolveRules`：
   - 输入按分类权重 `sort_order ASC` + 组内 `position ASC` 排序；
   - 若 `TargetGroupID` 为空且 `BuiltinTarget != ""`，`TargetGroupName` 赋值为 `BuiltinTarget`；
   - `MATCH` 规则强制切片至最底端。

### 3. API 控制面路由与能力支持 [FR-RULE-API-03]
1. 分类管理 API：
   - `GET /api/v1/policies/categories`：列出所有分类（按 `sort_order ASC`）；
   - `POST /api/v1/policies/categories`：新建分类；
   - `PUT /api/v1/policies/categories/{id}`：修改分类名称与权重；
   - `DELETE /api/v1/policies/categories/{id}`：删除分类；
   - `PUT /api/v1/policies/categories/reorder`：批量重排分类权重；
2. 规则管理 API：
   - `GET /api/v1/policies/rules`：支持 `category_id` / `category` / `keyword` 查询过滤；
   - `POST /api/v1/policies/rules`：支持新建内置动作规则与策略组规则；
   - `PUT /api/v1/policies/rules/{id}`：支持全量更新规则表达式、目标（内置/策略组）、所属分类、名称与启用状态；
   - `DELETE /api/v1/policies/rules/{id}`：删除规则；
   - `PUT /api/v1/policies/rules/reorder`：批量调整组内次序。

### 4. 独立规则工作台前端 [FR-RULE-UI-04]
1. `web/src/App.vue` 侧边栏在「策略编排（Policy Tree）」下方增设顶级项目「规则管理（Rule Desk）」，路由 `/#/rules`；
2. `web/src/router/index.ts` 与 `web/src/navigation.ts` 正式注册 `rules` 为合法顶级视图；
3. `web/src/features/rules/RulesView.vue`：
   - 顶部统揽：显示规则总量（470）、分类总量（14）、搜索过滤框、新建规则按钮、分类管理按钮、规则拓扑校验入口；
   - 分类卡片/手风琴：按分类权重升序排列，展示分类名、规则数、权重、上移/下移调权操作；
   - 规则表格：
     - 类型徽章（DOMAIN-SUFFIX / IP-CIDR / GEOSITE 等不同色系区分）；
     - 匹配表达式与名称说明；
     - 目标标识：DIRECT 绿色徽章、REJECT 红色徽章、策略组 蓝色微标带芯片图标；
     - 启用/停用 Switch 开关；
     - 上移/下移与编辑/删除操作；
   - 规则编辑弹窗：
     - 所属分类下拉；
     - 描述标签；
     - 规则类型下拉；
     - 匹配值输入框；
     - 目标类型选择（内置动作 / 策略组切换）；
     - 内置动作（DIRECT / REJECT / PASS）或策略组列表；
     - no-resolve 复选框；
4. 国际化：中英文完整覆盖。

---

## Non-Functional Requirements

- **数据完整性 (Data Integrity)**：迁移前后验证规则总数严格等于 470，分类总数严格等于 14，无任何旧规则遗漏；
- **零胶水原则 (Zero Glue)**：不搞临时映射表，`policy_rules` 原生容纳内置目标与策略组目标；
- **编译性能 (Compiler Throughput)**：470 条规则在各类客户端编译器中毫秒级产出（< 15ms），内存零额外开销。


## 当前增量：2026-10-19 节点/探针与策略筛选重做

最新授权、业务决策与 FR-R1..R4 以 [probe-ledger-redesign.md](probe-ledger-redesign.md) 为本规范组成部分。原有规则工作台要求继续有效；本次实施不包含生产发布。

## 当前授权增量：2026-10-20 筛选业务重做、DNS恢复与待复核治理

本轮需求/业务定义/接口与验收以 [filter-dns-redesign.md](filter-dns-redesign.md) FR-S1..S4为组成部分；其对前轮探针时效、普通编辑动线的纠正优先。旧实施/发布状态仅历史；当时“实施验证本地提交、不生产发布”的阶段边界不作为后续开发流程。本项目当前开发提交、云端验证及部署授权以根目录 [AGENTS.md](../../AGENTS.md) 为准：先推送 `dev` 触发云测，不在服务器或其本地 Docker 测试编译，生产部署另按本轮授权。此流程修订不授权自动导入历史DNS。

2026-10-21 实施与验收终态见 [continuation-1346392.md](continuation-1346392.md)，其中“不推送/不生产发布”仅描述旧 run 当时的授权与事实，不约束后续开发提交、云测及另外获授权的部署；旧失败历史保持不变。

## 2026-10-21 DNS数据恢复与策略功能去重

当前用户明确恢复原DNS数据(不回退现编辑器)、删除DNS页内标题横幅，直接处理重复规则/准入冲突。FR-C1..C3、真实证据、模块接口、兼容及验收以 [dns-policy-cleanup-1574174.md](dns-policy-cleanup-1574174.md) 为组成部分；该增量覆盖旧“不导历史DNS”的阶段限制及旧准入DSL要求。保留分流规则、节点启用/风险硬约束、全局filter、动态来源。程序仅开发push/云测/验收，生产DNS写单项已授权，程序上线另确认。

## 2026-10-25 DNS 页面独立重设计

最新用户目标、FR-DU1..DU5、授权与验收以 [dns-ui-redesign.md](dns-ui-redesign.md) 为当前组成部分；本轮允许重新设计编辑体验，但仍不恢复重复页内大标题。旧生产 DNS 写授权不继承，当前仅开发推 dev、云测与隔离验收，无生产部署/配置写入。

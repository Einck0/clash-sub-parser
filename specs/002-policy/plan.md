# Technical Plan: Policy Routing Rules Modernization & Legacy 470 Rules Restoration

- **Feature Directory**: `specs/002-policy/`
- **Run ID**: `pi_run_20261018_022832_3123769`
- **Status**: IMPLEMENTING

---

## 1. 架构定位与设计要点

### 1.1 规则丢失根因诊断与外键解耦
- **历史根因**：原 `policy_rules` 表定义了 `target_group_id TEXT NOT NULL REFERENCES node_groups(id)`，在系统初始化迁移时，无法存储目标为 `DIRECT`（292 条）与 `REJECT`（15 条）的规则，导致 308 条关键分流规则被粗暴丢弃，生产环境仅残留 162 条规则。
- **解耦设计**：
  - 允许 `target_group_id` 为 NULL，增设 `builtin_target TEXT CHECK (builtin_target IS NULL OR builtin_target IN ('DIRECT', 'REJECT', 'REJECT-DROP', 'PASS'))`；
  - 约束保证合法性：`CHECK (target_group_id IS NOT NULL OR builtin_target IS NOT NULL)`；
  - 增设 `category_id` 与冗余 `category` 字段，建立与 `rule_categories` 的关联。

### 1.2 历史数据无损回填方案
- **数据源**：历史稳定基准 SQLite `/data/clash_sub_parser.db`：
  - 14 个 `rule_categories`：广告/拦截 (0), 国内日常服务 (10), 移动应用/系统工具 (20), 桌面应用进程 (30), 游戏 (40), 流媒体 (50), 下载 (60), AI (70), 个人 (80), GEOSITE/平台 (90), 负载均衡 (100), 国内/直连 (110), 常规代理 (120), 兜底 (130)；
  - 470 条 `rules`：DIRECT 292 条，REJECT 15 条，策略组规则 163 条（包括常规代理 94 条、AI 22 条等）；
- **回填算法**：
  - 提取当前活跃 revision_id；
  - 插入 14 个 `rule_categories`（使用确定性或 UUIDv7 ID）；
  - 查询 `node_groups` 构建 `name -> id` 字典；
  - 遍历 470 条规则：
    - 若 proxy 为 `DIRECT` 或 `REJECT`：`builtin_target = proxy`，`target_group_id = NULL`；
    - 若 proxy 为策略组名：从字典查找对应 group_id 填入 `target_group_id`，`builtin_target = NULL`；
    - 若 options 包含 `no-resolve`：拼接至 expression；
    - 组内 position 从 0 递增赋值；
    - 验证迁移后规则总数恰好等于 470 条。

### 1.3 排序与终结兜底保序
- **宏观与微观联合排序**：
  - SQL 查询通过 `LEFT JOIN rule_categories rc ON pr.category_id = rc.id ORDER BY COALESCE(rc.sort_order, 999) ASC, pr.position ASC, pr.id ASC` 导出；
- **终结规则置底**：
  - `resolveRules` 遍历识别 `domain.IsMatchRule(expr)`；
  - 普通规则在前，MATCH 规则强制压栈至最底端。

### 1.4 API 控制面扩展
- 分类管理：`GET/POST/PUT/DELETE /api/v1/policies/categories` 与 `PUT /api/v1/policies/categories/reorder`；
- 规则管理：
  - `GET /api/v1/policies/rules` 支持 `category_id` / `category` 筛选；
  - `POST /api/v1/policies/rules` 原生接收 `builtin_target` 与 `category_id`；
  - `PUT /api/v1/policies/rules/{id}` 支持就地修改目标、表达式与分类；
  - `PUT /api/v1/policies/rules/reorder` 支持组内规则位置批量保存。

### 1.5 现代化独立规则工作台前端
- 恢复顶级路由：在 `router/index.ts` 中注册 `/rules`，在 `navigation.ts` 加入 `rules`，在 `App.vue` 侧边栏呈现「规则管理」；
- 界面架构：
  - 头部统揽与搜索栏（即时模糊匹配域名/IP/标签）；
  - 分类折叠面板（DaisyUI collapse 或自定义 card），显示分类名称、权重 badge、规则数 badge；
  - 分类内规则表格：支持上下移动、就地开关启用、编辑与删除；
  - 友好新建/编辑对话框：内置动作（DIRECT/REJECT）与策略组分立单选，直观便捷。

---

## 2. 变更文件清单

- `migrations/000025_rule_categories_and_builtin_targets.sql`：数据库表结构与 470 条规则回填迁移
- `internal/domain/policy.go`：领域实体与规则校验
- `internal/domain/ports.go`：仓储接口定义
- `internal/repository/sqlite/policy.go`：分类与规则 CRUD
- `internal/repository/sqlite/consistent_facts.go`：事实加载排序与分类关联
- `internal/resolver/graph.go`：内置目标名称解析与置底保序
- `internal/application/policy/service.go`：服务层分类与规则业务逻辑
- `internal/application/policy/types.go`：API 请求与响应结构体
- `internal/transport/http/policies.go`：HTTP 路由与处理函数
- `web/src/navigation.ts`：导航 Tab 注册
- `web/src/router/index.ts`：路由定义
- `web/src/App.vue`：左侧导航栏图标与菜单
- `web/src/features/rules/RulesView.vue`：独立规则工作台主页面
- `web/src/features/rules/useRules.ts`：规则与分类响应式状态与 API 交互
- `web/src/features/rules/RuleDialog.vue`：规则新建/编辑对话框
- `web/src/locales/messages.ts`：中英文双语翻译字典


## 2026-10-19 增量实施计划

Run `pi_run_20261019_115729_4186512` / change `change_pi_run_20261019_115729_4186512` / package `whole_task` / epoch 1。
基线 f00d49537f9788057c97232e531666c7587a5800；前序章节为历史已完成方案，当前施工仅以下增量及 probe-ledger-redesign.md。

### 技术上下文与宪章核验
Go 1.27.1、Chi、SQLite、Vue3/TS、DaisyUI；构建工具已在 Docker 中可用，宿主禁止重型测试/构建。graphify-out 不存在且 CLI 未安装，不能声称图更新成功。保留全量初始 status 到 /tmp/csp-4186512-initial-status.txt。既有修改跨文档及归档，完全不清理、不混入提交。
宪章：失败保全资产、稳定 logical_id、六域接口、极简直接存储、活规范均 PASS；无豁免。无 extensions.yml，前后 hooks 不适用。用户本次未授权生产发布，严禁主动部署/外部订阅探测。

### 阶段 A：探针业务与数据
在 domain/subscription.go 的 SubscriptionConfig 中加入可验证 probe_policy JSON 字段；在 subscription 服务/HTTP/SQLite 路径透传且部分更新保留旧配置。协调器 periodic.go 注入最小来源读取依赖，结合 node_sources 计算每节点最短有效周期，复用现有 schedule/recovery/lease/cooldown/pool；不建立每源计时器。先写可控 clock/runner 的单元与 SQLite/HTTP 集成测试，再实现。API 沿用现有 settings、schedule、subscription 入口，无额外控制面。

### 阶段 B：业务前端与筛选
NodesView/useNodes 接入运行及批次结果，分离出必要小组件，不把旧 ProbesView 直接套入 tab；SettingsView 统一持久配置；SubscriptionConfigDrawer 提供继承/关闭/自定义。路由、导航、dashboard、本地 tab 同步更新，旧 probes 只重定向。
策略编辑按范围/选入/排除/结果重组，复用结构化条件组件，保留存量有序来源的完整编辑。resolver/graph.go 修正 remove-filter 候选范围；PreviewGroup 和 ValidateGraph 事实组装与发布同源，拒绝加载失败的静默降级。维持动态来源、PCRE 能力、风险准入、子组投影和 WARP；不修改规则工作台。全局配置接口及过滤结构保持一份事实。

### 阶段 C：整体门禁
同一施工工作包顺序实施 A+B，避免共享 domain/ports、locale、service 等冲突。施工完成容器全量 Go/Vitest/typecheck/build，交付真实预览（loopback 临时端口，合成 DB，禁外联）及完整命令退出码、源码指纹、URL/清理合同。主脑核对后派独立 reviewer，整改后复验；工程 PASS 才派 critic 桌面及移动视口实际交互。critic 不代表物理手机验证。
全部通过才逐文件 git_commit，不自动推送/发布。结束清理本次预览并 QQ 汇报完成/遗留。graphify 若仍未安装明示限制，不为此私自安装工具。

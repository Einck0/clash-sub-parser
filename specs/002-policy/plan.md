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

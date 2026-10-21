# Policy Editor UI & Filter Source Integration Contract

## 1. 运行与验证身份 (Run & Governance Identity)
- **Run ID**: `pi_run_20261020_204735_990606`
- **Change**: `002-policy`
- **Work Package**: `default`
- **Reverification Of**: `b29aee57-1056-4ccd-a043-d232de732e99`
- **Task Scope**: 政策父编辑闭环（Policy Editor Sheet & Shared Structured Filter Inline Editing）

---

## 2. 架构契约与关键实现 (Key Architectural Decisions & Implementation)

### 2.1 行内属性过滤编辑闭环 (Direct Inline Filter Source Editing)
- **消除旧二次草稿与孤立引用**：
  - 彻底清理原父级 `PolicyEditorSheet.vue` 中遗留但已删除函数的引用（如 template 中的 `@click="editFilterSource(idx)"`）。
  - 删除旧有条件二次 draft 流程及“更新来源”中间状态机。
  - 在 Stage 3 执行流水线列表（Sources Pipeline）中，对于 `source.kind === 'filter'` 的条目，直接在流水线行内（`[data-testid="inline-filter-editor"]`）渲染 `StructuredFilterEditor`。
  - 打开策略组抽屉时，已有属性过滤条件开箱即见，严格保持原来源项流水线顺序。
  - 非 filter 来源的高级草稿（例如正则 pattern、组引用、手工节点等）保留原有输入与明确放弃/应用逻辑。

### 2.2 聚合无效性校验阻断 (Invalid Aggregate Protection)
- **状态追踪**：`invalidFilterSources` 维护存在校验失败条件的来源项行索引集合。
- **聚合校验与拦截**：
  - 计算属性 `hasInvalidFilterSources` 综合评估 `invalidFilterSources` 集合及是否存在空条件过滤来源（`filter_spec.conditions.length === 0`）。
  - 当存在任何未完成或无效条件时，同时禁用并阻断：
    1. 表单保存按钮与提交处理（`handleSave`），提示明确校验文案。
    2. 实时草稿快照预览请求（`requestLivePreview`）及刷新按钮，防止无效草稿污染后端解析器。
  - 校验通过后允许一键保存（一次 save），无需繁琐的“先更新来源项再保存策略组”。

### 2.3 预览防抖与世代复用 (Preview Debounce & Generation Reuse)
- 防抖时间设置为 250ms。
- 采用世代编号（Generation Counter）复用与校验机制：
  - 在防抖计时器调度时分配预期世代 `gen = ++previewGeneration.value`。
  - 异步请求发起与返回后严格比对当前最新世代，自动丢弃乱序返回的旧请求响应。
  - 组件关闭或取消时主动递增世代并清空定时器，防止组件卸载后 late response 导致状态漂移。

### 2.4 统计与排除展示细化 (Metrics Cards & Exclusion Breakdown)
- **候选节点数 (`candidate-count`)**：
  `candidate_count = snapshotPreview.candidate_count ?? (node_count + excluded_count)`，确保候选总数精确表达为“入选节点 + 净未选入节点”。
- **生效成员数 (`member-count`)**：
  另列独立统计卡片，区分策略组成员实体（包含内建项及展开组）与最终叶子节点数。
- **排除原因明细 ("无细因别说无排除")**：
  当存在排除节点（`excluded_count > 0`）但后端未返回细因分类字典时，明确显示 `已排除 X 个节点（未提供细因明细）`，不再误报“当前草稿无排除节点”。

### 2.5 共享组件字段集与直接行内化 (`StructuredFilterEditor.vue`)
- **字段集扩充与探针去新增**：
  - 新增现代策略字段：`health_status` (节点可用性)、`exit_region` (ISO 3166-1 alpha-2 地区代码)、`platform_verdict` (服务解锁与平台判定)。
  - `probe_verdict` (旧版探针判定) 默认移出新增下拉列表，仅在历史旧数据行中保留显示与继续维护。
- **直接行内化**：
  - 标准模式与行内模式统一采用行内即时编辑机制，彻底废弃底部旧二次草稿面板。
  - 字段切换与初始半填状态（如输入框为空）保持草稿可编辑性，同时正确触发 `@invalid` 事件。
  - 全局具体校验错误（如延迟格式、地区代码规则）优先于通用 invalid 提示。

---

## 3. 验证日志与凭据 (Verification Evidence)

### 3.1 单元与集成测试套件
- **命令**：
  `docker run --rm --network none -v "$PWD/web:/app" -w /app node:22 npm test -- policy GlobalNodeFilterSettings`
- **退出码**：`0`
- **日志 SHA-256**：`790b92337f7a7174121da1137dce1c109c86cd4f4662acffbebafcfef95456c2`
- **执行结果摘要**：
  ```text
  ✓ src/features/policy/PolicyView.test.ts (15 tests)
  ✓ src/features/policy/policySources.test.ts (34 tests)
  ✓ src/features/settings/GlobalNodeFilterSettings.test.ts (8 tests)
  ✓ src/features/subscriptions/SubscriptionProbePolicy.test.ts (4 tests)
  ✓ src/features/policy/policy.test.ts (29 tests)

  Test Files  5 passed (5)
       Tests  90 passed (90)
    Duration  11.01s
  ```

### 3.2 静态类型检查 (Type Check)
- **命令**：
  `docker run --rm --network none -v "$PWD/web:/app" -w /app node:22 npx vue-tsc --noEmit`
- **本模块结果**：
  `web/src/features/policy` 与 `web/src/features/settings` 范围内 **0 错误**（已完全修复原有 `Property editFilterSource does not exist` 与 `NodeFilterSpec` 缺失问题）。

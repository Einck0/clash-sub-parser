## Why

CSP 的 Workbench 表面上已引入 Tailwind token、`BaseDrawer` 与视觉审计，但 `style.css` 仍保留一套旧变量、旧 `.modal/.modal-backdrop` 和组件私有尺寸规则；Quick Export 与规则、节点组弹窗因而采用不同的 DOM、无障碍和响应式行为。现有静态审计也不能证明浏览器在真实 375/768/1024/1440px 视口下没有溢出或错误的覆盖层几何。

现在必须把设计原语、覆盖层尺寸和真实浏览器门禁冻结为可执行契约，并将同一契约同步给 Eindash，避免两个控制台再次各自演化为不兼容的样式系统。

## What Changes

- 新增 Workbench 设计治理能力：语义 token 成为唯一视觉来源，移除 CSP 旧 token 与旧全局弹窗体系，禁止领域组件重新定义覆盖层尺寸或行为。
- 新增共享 `AppModal` 与 `AppDrawer` 契约，定义 `sm|md|lg` Modal 尺度、桌面 Drawer 尺度、遮罩、层级、焦点生命周期和移动端 Bottom Sheet 行为。
- 将 Quick Export、Rule Import、Rule Presets、Node Group，以及遗留的手动节点编辑覆盖层迁移到共享原语；保留所有现有非 SCRIPT 功能、路由、表单语义与五目标导出能力。
- 新增真实 Playwright 多视口渲染门禁，针对所有覆盖层、Drawer、表格和核心路由验证 DOM 几何、局部滚动与文档横向溢出；违反尺寸契约或产生页面溢出即失败。
- 将同一 Design Primitives 以 Eindash 的 React/OpenSpec 适配条款冻结，后续 Eindash CPA 控制台不得引入第二套 Modal、Drawer 或移动覆盖层语义。

## Capabilities

### New Capabilities

- `workbench-design-governance`: 定义 CSP 与 Eindash 都必须遵守的语义 token、覆盖层尺寸、响应式交互和真实浏览器验收契约。

### Modified Capabilities

- None.

## Impact

- CSP 受影响范围：`frontend/src/assets/theme.css`、`frontend/src/style.css`、共享 `components/ui/` 原语、所有旧 Modal 消费者、相关路由，以及 frontend unit/Playwright/visual-audit 脚本。
- Eindash 受影响范围：现有 `refactor-eindash-cpa-quota-console` 的设计与任务工件，及后续 React 覆盖层实现；本变更不授权修改 Eindash 业务代码、运行配置或部署。
- 不新增运行时依赖，不修改后端 API、数据库、真实订阅、节点数据或部署服务。
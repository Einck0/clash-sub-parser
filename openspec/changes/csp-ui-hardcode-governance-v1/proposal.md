## Why

当前管理界面既有 DaisyUI/Tailwind 语义层，又在全局 CSS、Dock、弹层和卡片布局中散布固定像素、视口百分比与 `!important` 补丁。统一把目标强制设为固定 44px 已被用户否决：需要由顶层设计契约治理重复的呈现硬编码，以真实可用性而非机械尺寸作为验收。

## What Changes

- 建立流式布局、语义交互区、设计原语与例外登记的跨页面契约；优先复用 DaisyUI/Tailwind 与现有组件，不造全套新框架。
- 对全局小屏强制 44px 规则、Dock、抽屉/弹层、卡片最小宽和预览高度等已确认重复布局常量做有界治理；不得简单替换成另一批固定数值。
- 加入按实际交互/滚动/字体缩放验证的定向回归与例外说明，不引入所有像素字面量必然失败的机械门禁。
- 与进行中的 `csp-ui-device-governance-v1` 共享视口质量目标；旧 Change 的统一 44px 门禁已协调为语义交互可达、无遮挡及实测体验的验收要求。两套合同仍须在最终合并态共同复验。

## Capabilities

### New Capabilities
- `admin-ui-adaptive-layout`: 界面响应式排布、语义交互可达性、设计原语复用及跨视口实证验收。

### Modified Capabilities
- 无。`openspec/specs/` 目前没有已有主规格；进行中的旧 Change 是另一个尚未合并的 delta，不当作主规格直接改写。

## Impact

主要涉及 `web/src/style.css`、`web/tailwind.config.js`、`web/src/ui/ResponsiveGrid.vue` 与弹层组件、`web/src/App.vue`、引用方和定向 UI/Playwright 测试。无业务 API、服务端算法或数据迁移；不改部署与生产数据。工作区基线 HEAD `7f827e2`，初始记录 `git status --short` 726 条；`web/src/` 当前是未跟踪目录，实施时必须保留其中所有用户既有工作。规划不授权实施、生产写入或设备实屏操作。

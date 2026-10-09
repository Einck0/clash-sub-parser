## Context

`web/tailwind.config.js` 已启用 DaisyUI 七主题，但 `theme.extend` 为空；`web/src/theme/index.ts` 管主题名/存储而非空间尺寸。原始清单中 `web/src/style.css` 用小屏全局强制 44px 按钮/输入和 24px 复选框，Dock 及响应式卡片另有固定尺寸；当前只读核查显示全局强制规则已被修改，包 A/B 已勾选，但共享 `Drawer.vue`、`ModalDialog.vue`、`ConfirmModal.vue` 仍使用重复 `min(...,calc(...))` 视口表达式，`PolicyEditorSheet.vue`、`ProbeEvidenceSheet.vue` 与 `PublicationsView.vue` 也仍各自写高度公式。已勾选任务不等于合并验收通过。`NodesView.vue` 虚拟列表的动态像素高度是算法输出，不属于呈现硬编码。`openspec/specs/` 为空；旧设备治理 Change 的统一 44px 门禁已在其规划中改为语义交互可达、无遮挡与实测，不改变设备路径或其他任务。

HEAD 基线 `7f827e2`、起始记录 726 项脏条目，`web/src` 是未跟踪目录；既有 `npm test` 17 files/114 tests 成功是继承的报告，规划角色没有重新执行。禁止使用 git 清理/重置覆盖旧文件；无授权生产写入、部署、重启、实屏/ADB/USB。

## Goals / Non-Goals

**Goals:** 用已有 DaisyUI/Tailwind 语义尺寸及少量有名称的布局原语统一重复规则，保证导航占位、内容流动和弹层可滚动；按真实交互情境验证，不以单个数字定义体验；每处保留的例外说明理由与复验路径。

**Non-Goals:** 不把所有 `px`/`vh` 或 Tailwind 间距一律删掉；不修改业务分页/超时、虚拟滚动测量或 API；不新增 CSS-in-JS/私有 token 框架；不重新设计全部七页视觉，不更改生产构建产物。

## Decisions

1. **复用现有原语而非再造主题系统。** 主题颜色继续由 DaisyUI `data-theme` 管理；空间优先 Tailwind 既有 scale、流式 flex/grid、`min-w-0`/`max-w-full`、可用视口高度与安全区。只有跨组件重复且有语义的间距/停靠避让才提炼少量共享 CSS 变量或组件接口；不得将散装 44px 替换成单个 44px token 假装问题解决，也不得用更多逐页 `min/calc/vh` 表达式包装旧常数。备选“一次性替换所有字面量”会掩盖不同控件语义，拒绝。
2. **把交互区域同视觉标记分开。** 移除/收敛按断点通杀 DaisyUI 尺寸的全局强制覆盖，由组件按操作优先级使用语义按钮、明确焦点/名称及必要的容器间距；小型复选框可通过关联 label 扩大有效激活范围，不强迫图标本体与容器相等。先量测目标矩形、遮挡和邻近间距，再决定例外。备选“所有控件至少 44px 并添加像素 lint”与用户要求相反。
3. **共享弹层原语契约：布局职责在容器而非各页面。** 优先沿用现有 Drawer/ModalDialog/ConfirmModal 的交互语义和 DaisyUI 对话框能力；若现有 API 不足，由共享容器集中提供可用视口/安全区约束、窄屏及桌面定位、溢出防护、头部与底部操作区稳定可见、正文区域内部滚动、焦点/关闭行为。PolicyEditorSheet 和 ProbeEvidenceSheet 选择同一既有容器接口或可复用的共享弹层结构，而非各自继续拼视口算式；PublicationsView 的预览作为容器内长内容滚动，能扩展填充剩余空间，不能再用固定视口比例限制内容。页面只传内容、动作与确有理由的宽度或密度变体。可用高度/安全区仅在共享边界求值一次；若平台/浏览器差异必须保留局部尺寸，登记意图、唯一作用域和验证场景，不把多组近似数字迁移到新 token。变更抽象以前先检查已完成包 A/B 的实际产物，不能把勾选当作原语已达标。
4. **导航与卡片由可用空间决定。** Dock 以实际导航占用与 safe-area 避让主内容；响应式卡片阈值如需保留，作为局部可解释值或在响应式组件中封装，并验证狭窄容器下无溢出。`NodesView` 动态虚拟列表高度保留。
5. **旧 Change 已完成规划协调，施工仍须核实写集合。** `csp-ui-device-governance-v1` 的 spec/design/tasks 取消统一像素门禁，保留三视口、SSH→Ubuntu chroot ARM64 Chromium Playwright 和隔离服务边界。新旧 Change 的 Policy 编辑、共享弹层及预览路径只能有单写者并按接口先后串行；不能因规划协调就把未完成的业务任务勾选。
6. **实测而不是简单数字门禁。** 定向组件/浏览器测试检查窄屏 375×667/392×872、桌面 1280×800、长文本与缩放、导航末项、弹层与交互控件焦点/可点击性；多个主题抽样。无头验收仅限获授权隔离实例和 SSH→Ubuntu chroot ARM64 Chromium Playwright，记录命令、退出码、视口、构建、模式与脱敏证据，不声称实屏通过。不在当前树运行会覆盖 `csp`/`internal/webassets/dist` 的构建。

## Risks / Trade-offs

- [取消全局补丁暴露小控件问题] → 组件逐处审视可访问名称、label 和目标邻距，先编排回归再清理。
- [safe-area/浏览器动态工具栏影响实际可用高度] → 共享边界维护约束、以滚动与遮挡实测，分别记录 headless 不足和待授权实屏验证。
- [同时编辑 `App.vue` 或共享 CSS 与旧治理任务冲突] → 同文件串行单写者，合并后独立审查。
- [726 条脏改动且 `web/src` 未跟踪] → 施工前逐文件拍照/哈希并比对，禁止清场/覆盖，必要时停工请主脑处理归属。

## Migration Plan

无数据迁移；先核对旧 Change 已协调的合同，再冻结文件归属，按共享弹层容器先行、页面调用后迁移的依赖序列分批施工，独立复验合并态。若回归失败，仅撤销本任务可追溯的局部编辑，不使用 reset/restore/checkout/clean，也不影响既有脏文件。发布/重启需单独授权。

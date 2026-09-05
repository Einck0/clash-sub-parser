## Why

CSP 已具备 Node Ledger、全协议质检、流媒体与 AI 解锁证据、规则分流和多目标导出等完整工作流，但当前 Slate 蓝灰色底、分散的旧 CSS 表现规则和过于功能化的筛选/台账层级，未能把这些高价值数据呈现为一个精密、可扫描的现代控制台。现在需要在不触及任何非 SCRIPT 业务语义的前提下，将已治理的 Modal/Drawer 原语落实为 Linear 的冷静层级与 Raycast 的细腻实体感。

## What Changes

- 重新定义 CSP 深色语义色阶：以 `#08090a` void canvas、`#0d0f12` panel 和 `#121417` card surface 建立自然 Surface Elevation，并为内高光、发丝边框、状态色、焦点、阴影和动效提供唯一 token 来源。
- 将 Node Ledger 的筛选、地区、流媒体/AI 矩阵、握手延迟、台账行和详情抽屉改造成高集成胶囊控制区与高密度可扫描数据面；保留所有筛选、批量、探测、详情、导出与保存行为及其 API 契约。
- 将共享按钮、卡片、输入、状态点、Modal 和 Drawer 的 hover、focus、进入/退出与 `prefers-reduced-motion` 表现统一到 token 化微动效；共享遮罩允许仅在受治理的 AppModal/AppDrawer 上使用受限的柔和 backdrop blur。
- 移除与新 token 层级冲突的遗留 Slate 色值、组件私有 elevation/overlay 外观和笼统全局按钮规则；不将此次视觉升级扩展为 Tailwind 全量重写或领域重构。
- 扩展视觉与浏览器门禁：除既有 375/768/1024/1440px 几何、可访问性、键盘和溢出检查外，验证 void/surface 层级、token 所有权、受限 blur 边界、交互状态及 Node Ledger 核心真实工作流未回归。

## Capabilities

### New Capabilities

- `linear-raycast-workbench-visual-system`: 定义 CSP 控制台的深色表面层级、交互质感、台账与筛选视觉语义，以及可测量的视觉治理边界。

### Modified Capabilities

- `workbench-design-governance`: 将既有跨覆盖层治理要求扩展为新的 CSP 深色 token/elevation、受限 backdrop blur、控件交互状态与浏览器视觉门禁契约，同时保留统一 Modal/Drawer 尺度与可访问性约束。

## Impact

- 仅影响 CSP 前端表现层：`frontend/src/assets/theme.css`、兼容样式、共享 UI 原语、Workbench/Node Ledger 组件、视觉审计及 Playwright/前端测试。
- 不修改后端 API、数据库、订阅源、真实节点与探测数据、规则/分流、Clash/Mihomo/Stash/Shadowrocket/Sing-box 导出语义、认证、依赖、运行配置或部署。
- 已存在的 `unified-workbench-design-governance` 仍是覆盖层尺寸与可访问性的基线；本变更仅对其受影响要求追加 delta，不复制或推翻其非视觉业务约束。

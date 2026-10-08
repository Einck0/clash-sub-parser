## Purpose

构建稳定一致的自适应视口导航布局，固定桌面端侧边栏防止滚动逃逸，实施移动端遮罩体滚动锁定，消除探针瀑布流拉取开销，确保跨四种标准视口的人机工效与可用性。

## ADDED Requirements

### Requirement: Sticky Desktop Navigation Sidebar
The desktop sidebar in `App.vue` SHALL remain sticky and fixed within the viewport during full-page vertical scrolling, preventing navigation controls from disappearing.

#### Scenario: User scrolls long policy or node lists on desktop
- **WHEN** user scrolls down a long page on a desktop viewport (e.g. 1280x800)
- **THEN** the sidebar SHALL remain anchored at `top: 0` with independent internal overflow scrolling, keeping all navigation tabs accessible at all times

### Requirement: Body Scroll Locking on Mobile Modal Overlays
The application SHALL engage body scroll lock whenever a bottom sheet, modal dialog, or full-screen drawer is active on mobile viewports.

#### Scenario: Mobile sheet opened for policy editing
- **WHEN** user opens `PolicyEditorSheet` or another modal on mobile viewports (e.g. 375x667, 392x872)
- **THEN** the background body element SHALL apply scroll locking (`overflow: hidden`), preventing background content from scrolling underneath the sheet

### Requirement: Elimination of Probes Waterfall Polling
The probe workbench composable `useProbes` SHALL eliminate synchronous multi-page waterfall loops (`while (page <= 20)`), fetching single paginated views and consuming backend pool metrics directly.

#### Scenario: Loading probe workbench nodes
- **WHEN** user navigates to the Probes view
- **THEN** the client SHALL request bounded page data (`page_size <= 50`) and utilize `GET /api/v1/probes/pool` for overall pool metrics, avoiding sequential waterfall requests

### Requirement: Four-Viewport Responsive Validation
The user interface layout SHALL be hermetically verified across four standardized viewports: Desktop (1280x800), Mobile Portrait (375x667), Mobile Large (392x872), and Mobile Landscape (667x375).

#### Scenario: Multi-device rendering inspection
- **WHEN** the frontend test suite runs across the four designated viewports
- **THEN** navigation elements, cards, and modal sheets SHALL render without horizontal overflow, clipping, or unclickable touch targets

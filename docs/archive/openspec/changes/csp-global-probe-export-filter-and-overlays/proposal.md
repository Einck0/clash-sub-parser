## Why

用户发现部分操作菜单、弹窗在实际视口中被裁切或被其他弹层遮住，同时设置页缺少以探针状态为主的全局导出筛选入口。现有筛选后端已经存在，但发布解析在读取异常时可能静默回退为不过滤，产生不符合管理员意图的导出。

## What Changes

- 在设置页复用现有全局节点筛选存储与条件语言，提供以探针类型、判定和观测有效期为主的编辑入口；清楚解释空条件透传、未测/超龄观测排除、保存后只影响新预览及新发布，且不把队列瞬态或凭据版本匹配描述为已有筛选能力。
- 让四类导出目标的预览、新发布消费相同筛选解析结果；读取筛选或筛选依赖的观测/来源失败时禁止扩大导出，配置了筛选但候选节点为零时返回可操作的错误并拒绝意外发布。保留历史不可变发布直链与旧配置无筛选时的既有行为。
- 修复订阅卡片菜单被裁切、确认框被原生对话框遮挡，以及短视口弹层标题、正文与操作按钮不可用的问题；兼顾定位、键盘焦点和视口适配。
- 用服务层/组件测试与四视口加短高度浏览器验收覆盖关键路径；本 Change 不包含实施、部署或生产数据修改。

## Capabilities

### New Capabilities

- `global-probe-export-filter`: 设置页探针优先的全局筛选、安全解析、预览/发布及零结果行为。
- `viewport-safe-overlays`: 菜单、对话框、确认框及抽屉在可用视口内的层叠、可读与可操作行为。

### Modified Capabilities

无（`openspec/specs/` 当前无已发布能力规格；此 Change 新增完整行为契约，不修改其他 Change 的历史文档）。

## Impact

`internal/application/publication/**`，`web/src/features/settings/**`，`web/src/features/policy/PolicyView.vue`，`web/src/ui/{Popover,ModalDialog,ConfirmModal,Drawer}.vue` 及相关 subscriptions/publications/policy/probes 组件与测试。沿用 `GET/PUT /api/v1/policies/global-node-filter`、现有 `NodeFilterSpec`/resolver、Vue 现有 Teleport/原生 `<dialog>`/浏览器 top layer 与已有自适应 CSS token；不新建第二套筛选存储或引入额外浮层框架。四目标为 Mihomo、sing-box、Surge、Quantumult X；已发布直链继续提供创建时冻结的内容。上线另需授权。

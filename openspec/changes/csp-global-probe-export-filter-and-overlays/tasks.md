## 1. A · 后端安全筛选与零结果（完整特性包；可与 2、3 并行）

- [x] 1.1 仅在 `internal/application/publication/**` 实现并自测新预览/预检/新发布的安全共用解析：筛选/组筛选读取错误不透传；已配置 probe/source 条件时观测/来源读取错误或必要依赖缺失一律阻断；无条件透传、旧直链冻结不变；用注入失败仓库的定点 Go 测试验证三入口及四目标不会产生额外节点或意外发布，运行 `go test ./internal/application/publication/...` 和 `go build ./...` 退出码均为 0。
- [x] 1.2 在同一 A 包内完成已配置条件筛出零节点的可识别诊断与新发布阻断，覆盖无路由组、含路由组、未测/过期、四目标及旧直链的测试；验证 `go test ./internal/application/publication/...` 和 `go build ./...` 退出码均为 0。A 包自行完成 1.1、1.2 的代码及自测后才交工。

## 2. B · 设置页探针优先的全局筛选（完整特性包；可与 1、3 并行）

- [x] 2.1 仅修改 `web/src/features/settings/**` 及必要 `web/src/features/policy/PolicyView.vue` 文案/共享已有类型：设置页复用同一 GET/PUT 全局筛选，允许现有全部合法字段回显与编辑、探针 kind/verdict/有效期、AND、清空及错误处理；准确说明未测/过期、24h 默认、新发布才生效及 queued/probing 非持久状态，纠正策略页凭据版本匹配的误导文案；增补设置页组件测试验证读写 payload、刷新回显、非法输入/读写失败不覆盖、双入口文案，运行前端针对性测试及 `npm run type-check` 退出码均为 0，B 包实现与自测一并交工。

## 3. C · 脱裁切与层叠安全浮层（完整特性包；可与 1、2 并行）

- [x] 3.1 仅修改 `web/src/ui/{Popover,ModalDialog,ConfirmModal,Drawer}.vue`、必要的 `web/src/features/{subscriptions,publications,probes}/**` 调用点及对应测试（不写 PolicyView.vue、SettingsView.vue、共享样式）：订阅菜单脱离裁切祖先并按可用视口定位、Escape/外部点击/键盘焦点可用；确认框进入与原生 dialog 兼容的 top layer；短高度标题/正文可滚、操作可见，确保不会因普通 z-index 遮挡；用组件测试覆盖事件/焦点/叠层，运行前端针对性测试及 `npm run type-check` 退出码均为 0，C 包实现与自测一并交工。

## 4. 汇聚门禁（依赖 1、2、3 全部施工及各自自测结束）

- [x] 4.1 在三包全部完成后的最终工作树运行 `go test ./...`、`go build ./...`、前端完整测试和 `npm run type-check`，各退出码 0；跨层核对四目标从新预览到新发布的筛选/零结果诊断一致性，且不存在共享文件双写或新旧发布内容混淆。
- [x] 4.2 独立只读 reviewer 对同一 Change 全部规格、最终 diff、失败关闭分支、旧直链冻结与真实测试证据复验并出具明确 PASS；不以施工者自测替代审查。
- [x] 4.3 工程审查通过后由准备者提供隔离预览 URL、健康检查与标准实例 JSON，独立 critic 对订阅/发布/策略/探针页在窄手机、宽手机、平板、桌面及短高度视口操作菜单、嵌套确认框、长内容表单/抽屉，结合真实浏览器截图、几何与键盘交互给出 `CRITIC_VISUAL_VERDICT: PASSED`；无隔离 URL 或可核验证据不得勾选本条。

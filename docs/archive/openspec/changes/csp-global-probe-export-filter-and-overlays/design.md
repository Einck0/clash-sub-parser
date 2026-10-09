## Context

当前 `GET/PUT /api/v1/policies/global-node-filter` 已保存 `{spec:{conditions:[...]}}`；`internal/domain/node_filter.go` 与 resolver 已有 AND 条件、probe_kind、判定、24h 默认/可选有效期及不匹配原因。设置页无入口，策略页已有表单但声称凭据版本匹配，实际 `MatchesCondition` 只检查时间和 verdict。`publication.Service.resolveSnapshot` 在过滤定义、来源、观测读取失败时容忍失败；无路由组时仅靠既有空路由组预检无法发现零节点。`Popover` 的 absolute 菜单位于 `SubscriptionsView` 的 overflow-hidden 卡片内，`ModalDialog` 使用 `showModal()` 而 `ConfirmModal` 是普通 z-index div，后者无法超过浏览器 top layer。`web/src/style.css` 已提供 `adaptive-surface-dialog/sheet` 等视口 token。

## Goals / Non-Goals

- Goals: 设置页可编辑唯一全局筛选，以探针状态为主；同一解析结果驱动四目标新预览/发布，读取失败和配置导致零候选不得扩大或产生意外发布；跨视口浮层无遮挡、可滚动、键盘可操作。
- Non-Goals: 新筛选数据库迁移、第二套筛选 DSL、自动重写旧直链、声称凭据版本核验、把 queued/probing 存为 verdict、改造探针调度、上线生产。

## Decisions

1. **一个权威配置，多个管理入口。** 设置页使用现有 API 与 `policyTypes.ts` 的领域类型/校验（按需复用已有 `usePolicy` 的读写方法或抽出已有读写逻辑，不能复制一套存储）。以 verdict 类型/可选 latency 条件优先展示，并允许已有名称/协议/来源条件原样回显与编辑；判定选项只限已有 verdict。保持最多 32 个条件、服务端校验为最终权威；前端提示 24h 默认有效期及已发布内容冻结。策略页保留同一入口、纠正凭据版本匹配的虚假声明，并对状态说明与设置页一致。缺失记录/空条件透传，但传输、数据库读取错误不可用空条件替代。
2. **安全失败发生在共有解析边界。** 在 `internal/application/publication/**` 的 `resolveSnapshot` 及相邻 preview/preflight/publish 共用入口处理筛选定义读取错误；含条件时从字段识别所需依赖（probe 对应观测、source_subscription_ids 对应来源，必要时考虑组级条件），依赖未注入或读取失败即带安全错误返回，不能以空 map 代替。沿用现有 repository/resolver 和 Go 错误/领域错误，避免仅在 UI 或单一 target 特判。考虑 group filters 的读取失败，同样不得静默丢弃配置以扩大导出；未配置条件正常透传。观察到无匹配时使用 snapshot 的现有节点/FilterCounts 和诊断，在编译/发布持久化前统一拒绝或给出明确诊断。测试覆盖无路由组，不能仅以 empty_routed_group 为防护；区分完全没有库存且未配置筛选的既有行为。旧直链只提供已冻结 artifact，不受当下筛选状态和读取失败回溯影响。
3. **复用浏览器层叠和现有布局。** `Popover.vue` 用 Vue `Teleport` 至 body 摆脱卡片裁切，以触发器 DOMRect 和浮层实际尺寸在打开、滚动、resize 时计算可视区域内位置（可复用项目现有 `@vueuse/core` 能力，避免自制通用定位框架；若已有可靠定位依赖，优先采用成熟 API）；保留 trigger/slot API，兼顾外部点击和焦点恢复。`ConfirmModal.vue` 采用原生 `<dialog>.showModal()` 加入 top layer，而不是继续叠加 z-index；沿用既有 `update:modelValue`/`confirm`/`cancel`，Escape/backdrop/loading 语义需验证。针对已打开的 `ModalDialog`，确保原生对话框同一 top layer 后打开者可交互。`ModalDialog`、`ConfirmModal`、`Drawer` 的标题/正文/操作沿用项目自适应 token、flex shrink 与可滚动区域，短高度时允许整体/正文按实际空间滚动，操作与关闭仍可见。此包同时完成 subscriptions/publications/policy/probes 的必要调用点适配与回归。
4. **并行写集合严格分离。** A 仅 `internal/application/publication/**`；B `web/src/features/settings/**`、`web/src/features/policy/PolicyView.vue`（只改文案，如必须共享新代码也由 B 在其拥有文件内完成，避免触碰 C）；C `web/src/ui/{Popover,ModalDialog,ConfirmModal,Drawer}.vue` 与 subscriptions/publications/probes 组件及其测试，不写 PolicyView.vue 或 settings。跨包交互仅使用现有 API/props/slots，不并行修改 `web/src/style.css`；新类型尽量复用已有类型，不扩大写集合。若实现必须触碰其他公共文件，先协调写集合冻结，不能私自双写。

## Risks / Trade-offs

- 所有解析入口在筛选库故障下安全拒绝，会使配置错误时新预览/发布不可用；这是优先防止额外节点泄露的明确取舍，已发布冻结链接不变。服务层依赖未配置与历史无筛选的兼容须由夹具确定性区分。
- 浮层放到 body 后，外部点击、关闭时焦点恢复、菜单滚动跟随及 top layer 中定位必须在真实浏览器验证；不要仅用 jsdom 的静态矩形断言宣布无裁切。
- 可用的视口空间随软键盘、安全区及浏览器缩放改变，应采用现有响应式 token 和浏览器实际视口，不把凭空固定像素当硬验收标准。

## Verification

- A：仓库 stub 注入筛选/组筛选/观测/来源读取错误、依赖缺失，断言所有新入口安全失败且无创建记录；配置 probe 条件含缺失/过期观测、零候选且无路由组、四 target 共同筛选；确认旧 artifact 保持冻结。
- B：设置页 GET/PUT payload、刷新回显、空条件、非法字段/判定/有效期以及失败不覆盖，策略页文案不得再宣称凭据版本核对或持久 queued/probing。
- C：组件层菜单脱离裁切祖先、Escape/外部点击/焦点；叠层确认框事件与键盘；真实浏览器四代表视口与额外短高度检查订阅、发布、策略和探针页面长内容可滚至底，交互按钮不被覆盖；测试可运行于隔离本地预览，不触碰生产。最终统一执行 `go build ./...`、`go test ./...`、`npm run type-check` 及相关前端测试；独立审查与黑盒验收不得预先勾选。

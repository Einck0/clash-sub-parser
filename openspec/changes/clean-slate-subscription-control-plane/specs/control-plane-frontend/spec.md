## MODIFIED Requirements

### Requirement: NodeLedger 双视图与统一查询状态
控制台 SHALL 保留 NodeLedger 的指标面板、卡片网格和紧凑表格双视图。两种视图 MUST 共享同一个可恢复查询、稳定 `node_id` 选择集和语义窗口锚点；切换视图不得清空筛选、选择、排序或当前可见节点语义。列表 MUST 消费服务端返回的查询窗口、统计和 facets，而不是在浏览器中合并全量节点、探测和跳板后重复筛选或排序

#### Scenario: 切换台账视图
- **WHEN** 操作者在已有组合筛选和多选节点时切换卡片/表格
- **THEN** 显示同一查询结果及选择状态，并以当前可见 `node_id` 恢复既定窗口语义

#### Scenario: 连续浏览大量节点
- **WHEN** 匹配结果超过当前视口并且操作者滚动卡片或紧凑表格
- **THEN** 页面只渲染当前窗口及必要缓冲项，继续读取不会造成已选择节点、过滤条件或可见位置丢失

### Requirement: 多维组合筛选与指标快捷筛选
NodeLedger SHALL 支持名称/地址/端口关键字、来源、协议、探测状态、跳板状态、最低速度、媒体/AI 能力与排序组合筛选。总数、健康、高速和已挂载跳板指标 MUST 可快捷设置或重置相应过滤条件，并在数据未探测、部分探测或不匹配时解释状态

#### Scenario: 按能力和速度筛选
- **WHEN** 操作者选择最低速度及一个媒体/AI 能力条件
- **THEN** 列表只显示匹配节点，并可解释未探测或不匹配节点的状态

### Requirement: 批量诊断与单节点详情
控制台 SHALL 保留当前窗口全选、反选、显式多选以及“当前筛选全部结果”选择，保留批量探测、测速开关、取消 Job、清理缓存、节点详情抽屉及 sing-box/Clash 预览复制。全筛选选择 MUST 显示匹配数与排除数，并在命令前确认其固定 snapshot 范围；相同命令运行中 MUST 防止重复提交并给出可恢复错误；取消、失败和部分完成 MUST 与成功状态明确区分

#### Scenario: 打开节点详情后重新探测
- **WHEN** 操作者在详情抽屉发起单节点探测
- **THEN** 页面显示 Job 状态，完成后只刷新该节点的延迟、出口和能力摘要，且可复制目标配置预览

#### Scenario: 批量探测部分完成后取消
- **WHEN** 操作者取消一个已有部分节点完成的批量探测 Job
- **THEN** 页面保留已完成节点的最新结果、显示取消终态且允许对明确选择集再次发起探测

#### Scenario: 对全部筛选结果发起批量探测
- **WHEN** 操作者未显式选择节点并确认对当前组合筛选的全部结果执行探测
- **THEN** 页面展示 snapshot 的匹配数与排除数，Job 固定使用该范围，即使后续滚动窗口、筛选或节点刷新发生变化也不改变其目标集合

### Requirement: QuickExport 统一分发入口
控制台 SHALL 提供全局 QuickExport 入口，保留单订阅与合并订阅切换、Clash、Mihomo、Stash、Shadowrocket、Sing-box 五个目标、客户端 Scheme 与二维码。界面 MUST 只展示服务端已编译且关联配置修订的发布结果，且不得提供 SCRIPT 目标

#### Scenario: 从工作台快速导出
- **WHEN** 操作者从任一已认证工作台页面打开 QuickExport，选择一个目标和单订阅或合并订阅
- **THEN** 页面展示与该修订一致的订阅 URL、适用客户端 Scheme 和二维码，且切换目标不会丢失已选发布范围

### Requirement: Workbench 视觉一致性、密度与可访问交互
控制台 SHALL 以一致的 Workbench 壳承载订阅、策略组、NodeLedger、跳板链、规则、DNS、导出、设置和快照入口，并保留既有路由与 Quick Export 入口。它 SHALL 以 token 驱动的暗色默认和等价浅色主题呈现，采用不透明表面、1px hairline 分区、8px 栅格、明确文本层级和等宽数据轨；数字、IP、端口、延迟、速度、流量和日期 MUST 使用 tabular figures。密集数据表 MUST 保持 36px 行高、可辨识列标题、焦点和状态，节点卡 MUST 是有界的小屏或替代视图而不是默认库存容器。

工作台 MUST 禁止 Card Soup、glassmorphism、紫色或荧光渐变、居中 Hero、无意义留白、装饰性 emoji 图标、过大圆角与阴影堆叠。领域模板和 scoped styles 不得私自定义颜色、阴影、圆角、渐变或像素密度；它们 MUST 消费共享语义 token/原语。状态色不能是唯一传达信息的途径，图标 MUST 来自统一图标集并附带文字、title 或可访问名称。

所有键盘可操作元素 MUST 有可见 `:focus-visible` 焦点样式；页面 MUST 提供 skip link。Dialog、Drawer、Menu 和 Confirm MUST 保持可感知标题、可访问名称、焦点陷阱、Escape/取消、背景不可交互、初始焦点和关闭后的焦点恢复。窄屏交互目标至少为 44px、底部 sheet 适配 safe area；数据表必须降级为可读卡片或可横滚的语义表格。动画 MUST 使用统一短时缓动和 reduced-motion 回退，且不得使用 `transition: all`、宽度动画或持续闪烁作为状态表达

#### Scenario: 键盘操作对话框
- **WHEN** 操作者用键盘打开详情、跳板配置、导出或危险操作确认界面
- **THEN** 焦点进入带可感知标题的界面，可用键盘完成控件操作或 Escape/取消关闭，并在关闭后回到原始触发控件

#### Scenario: 窄屏访问数据表
- **WHEN** 操作者在窄屏设备访问 NodeLedger 或其他高密度管理视图
- **THEN** 页面提供可读的卡片或可横向浏览的语义表格，不隐藏操作、不把状态仅用颜色表达，也不使焦点落入不可见区域

#### Scenario: 高密度视觉门禁
- **WHEN** CI 扫描领域视图和共享组件
- **THEN** 扫描拒绝未映射的颜色、渐变、模糊背景、emoji 交互图标、`transition: all`、宽度动画和不受 token 控制的 Card Soup 样式，同时允许主题入口和共享原语中经记录的 token 映射

### Requirement: 前端领域切片、草稿安全与请求一致性
NodeLedger、Subscriptions、NodeGroups、Rules、ProxyChains、Dns、Generate/QuickExport、Settings 和 ConfigHistory SHALL 按领域组件、查询状态和编辑草稿组织。远端刷新、路由切换和失败响应不得覆盖未保存的编辑草稿；危险操作需明确确认、取消后恢复焦点。领域组件只能经统一 API client 与领域状态读取服务端结果，不得在浏览器复制协议解析、节点能力判定、策略动态展开或配置编译规则。统一 API client MUST 支持 AbortSignal/请求身份，以保证晚到的旧查询不能覆盖更新筛选后的结果

#### Scenario: 编辑策略时后台刷新
- **WHEN** 操作者编辑策略组且节点查询返回新数据
- **THEN** 节点候选可更新，但当前策略草稿保持不变并提示其基于的状态

#### Scenario: 旧请求晚于新筛选返回
- **WHEN** 操作者快速变更筛选或排序，较早的查询响应在较新的查询之后返回
- **THEN** 控制台忽略或取消旧响应，只显示最新查询对应的节点窗口、统计与选择语义

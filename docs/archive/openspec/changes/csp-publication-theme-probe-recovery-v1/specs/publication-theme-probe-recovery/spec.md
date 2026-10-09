## Purpose

本能力定义发布中心恢复、跨视口可用性、真实安全探针、生命周期与隔离成品验收的可独立验证合同；静态配置断言不等于代理握手或成品可用性。

## ADDED Requirements

### Requirement: 发布预览缺少活跃修订时可理解并可恢复
发布中心 SHALL 保留服务端 `409 no_active_revision`、提供配置入口和真实重试；真实预览成功前 MUST 禁用发布、下载与复制。401/403/500 与网络错误 MUST 区分，不得假装成功。

#### Scenario: 无活跃修订后恢复
- **WHEN** 隔离实例无活跃修订，用户打开发布中心、建立有效修订并重试
- **THEN** 初始请求返回 409 且显示前置条件与入口；修订后仅真实预览成功才开放发布操作

#### Scenario: 鉴权或服务器失败
- **WHEN** 预览遇 401、403、500 或网络故障
- **THEN** 显示对应恢复反馈，不误报缺少修订

### Requirement: 主题偏好菜单不被布局裁剪
主题选择 SHALL 在移动及桌面视口可见、可聚焦、可经指针与键盘操作；展开时 MUST 不被遮挡，切换保留当前路由与持久化偏好。

#### Scenario: 本地多视口展开与选择
- **WHEN** 在隔离浏览器 392×872、375×667、1280×800 打开主题菜单并选择 light/dark/system
- **THEN** 所有选项可触达、无不可恢复横向溢出，主题生效、路由不变且重载后偏好仍有效

### Requirement: 移动工作区页面和导航可用且不靠裁剪掩盖
Publications、Dashboard、Policy Admission Rules、Subscriptions 和 Settings SHALL 在 375×667、392×872 与 1280×800 真实浏览器视口中保持主内容、操作和浮层可用；关键操作 MUST 完整位于 viewport 之内、无互相遮挡，主区域 MUST 无横向内容溢出。仅允许有明确内容边界且末项确实可达的局部滚动；设置主容器 `overflow-x-hidden` 却裁掉控件 MUST NOT 算合格。导航 SHALL 在移动提供 Dashboard 及其余全部现有七条业务路由的可辨、可点击路径，dock 项之间有明显可辨间隔且末项不被遮住；Settings token 状态每次只表达一次状态，不得出现重复 `Active Active`。

#### Scenario: Publications 头部操作与全部 target
- **WHEN** 在 409 状态和预览成功状态下于 375×667、392×872 分别打开 Publications，遍历 `COMPILER_TARGETS` 的五个项目并操作下载/创建
- **THEN** Create Publication 和恢复按钮完整可见可点击、禁用/成功状态不变；包括 Quantumult X 在内每个项目可由触摸/键盘发现并到达，选择后真实预览或明确 409，main 不因目标行总宽而增宽；任一局部滚动可把末项完全滚入视口

#### Scenario: Dashboard badges、Policy rules 与 Subscriptions 菜单
- **WHEN** 渲染 Health Telemetry 的长徽章、Admission Rules 的长表达式/ID 和带状态徽章/动作菜单的 Subscription，再打开菜单并执行操作
- **THEN** 两 badge 文字在自身边界内且不穿刺边框、规则与 subscription 卡片不强迫 main 变宽、操作入口和菜单完整在 viewport 内，可点击/键盘可操作，长内容仅在受控内容区截断/滚动而不丢操作

#### Scenario: 七路由移动可达、dock 分隔与状态文案
- **WHEN** 在 375×667/392×872 的移动 dock 从任意路由返回 Dashboard，再访问其余六路由，分别加载有/无 token 的 Settings 且切换支持语言
- **THEN** 首页及每一路由有明确可操作入口，dock 可见目标不粘连、导航不盖住页面末尾控件，设置状态单次准确表示且无 `Active Active`，主题菜单及当前路由保持原有语义

### Requirement: 无可用凭据时安全失败而不误报成功
探针 SHALL 仅凭经确认完整节点配置经该节点隔离 outbound 请求受控目标；MUST NOT 使用占位服务器/口令、宿主直连、系统代理或其它节点出口。仅有旧摘要、缺密钥、无可执行目标时 MUST 同步拒绝或到达明确失败终态，不得生成 available 或永久 queued/running。运行结果与节点可用性 SHALL 区分。

#### Scenario: 旧摘要、无节点或缺密钥
- **WHEN** 选择缺少可解析凭据、缺密钥或有效目标为空的节点
- **THEN** 无占位/宿主直连出站，返回脱敏安全失败或同步拒绝，无 available

#### Scenario: 部分提交失败、取消与截止时间
- **WHEN** 队列容量不足、取消、服务关闭或期限到达
- **THEN** 已接纳任务有界回收并得到 failed/cancelled/expired 终态，不被晚到 succeed 覆盖

### Requirement: 真实探针凭据与库存版本一致且保密
系统 SHALL 仅通过获授权源输入获得完整连接参数；持久化认证密文 SHALL 受独立受管密钥保护，并原子绑定节点当前 credential_version、精确 (logical_id,version,protocol) 密文及有效来源。版本仅随规范化完整连接配置变化而单调增长；源全文摘要、顺序或显示名单独变化 MUST NOT 升级版本；撤销后版本不得重用。授权来源冲突 MUST fail-closed。旧摘要和无版本旧行 MUST NOT 视为可用密文；API/日志/审计不得泄漏口令、源 URL token、密钥或响应正文。

#### Scenario: 幂等刷新、变更、旧行及冲突
- **WHEN** 用隔离密钥导入授权 fixture 源、调整排序/显示名称、修改连接配置、撤销重导或遇来源冲突
- **THEN** 相同配置版本不变、不同配置递增且不重用；旧行/冲突不得拨号，事务失败不半更新，数据库无明文

### Requirement: 接纳的运行冻结精确授权快照
每个获接纳 run SHALL 固定节点标识、protocol、credential_version 及来源有效性非秘密快照；执行时 MUST 精确取此版本并核验配置身份/AAD，不得升级 latest。拨号前及在途 MUST 检查版本和授权仍有效；撤销、重配、取消停止新连接并有界关闭在途；重启无法恢复快照的旧任务 SHALL expired。

#### Scenario: 排队后重配或撤销
- **WHEN** 排队后节点更新/失去来源或运行中撤销
- **THEN** worker 不用新版本替代、不使旧版本结果成为当前 available，资源及时关闭并脱敏失败

### Requirement: 节点入口与目标实际拨号均阻断 SSRF 并保留域名身份
生产探针 SHALL 仅请求服务端允许的 scheme/host/port，不接收客户端动态 URL，不随意跳转。节点和目标每次实际拨号 IP MUST 按现有策略禁止内网、环回、metadata、RFC6598、IPv4 映射 IPv6 等；节点域名 MUST 在实际连接处固定为已解析验证公网 IP，不能校验后再次解析连接。`ss`、`trojan`、`vless`、`vmess`、`hysteria2`、`tuic` 六协议域名入口 MUST 经真实协议 inbound→目标 HTTP 204 实测成功才声称完成；开启 TLS 者 MUST 依显式 SNI 或原域名校验证书；ws/http/httpupgrade/grpc 适用组合 MUST 保持显式 Host/authority 或原域名且路由正确。SS 验证加密代理握手但不要求 TLS；Hy2/TUIC 实测 QUIC/TLS/UDP。测试 CA 和映射 MUST NOT 进入生产默认配置；未通过协议/传输 MUST fail-closed 且不得勾完成。

#### Scenario: 不安全入口、双解析改绑及目标重定向
- **WHEN** 节点/目标为 RFC6598/metadata/私网、首次公网后重绑私网或目标跳内网
- **THEN** 拒绝不安全连接，不发凭据，无 available

#### Scenario: 首个最小真实协议闭环
- **WHEN** 受控 resolver 返回验证公网节点与目标 IP，Trojan/TLS/TCP 凭据通过测试专属映射到 sing-box inbound 经代理请求本地 HTTP 204
- **THEN** 可观察公网 IP、TCP accept、受测试 CA 信任的 SAN/TLS/SNI、正确密码握手、目标命中和 204；配置快照不算通过

#### Scenario: 六协议及传输矩阵完成
- **WHEN** 同 fixture 分别提供 ss、trojan、vless、vmess、hysteria2、tuic 域名凭据并测试声明支持的 ws/http/httpupgrade/grpc 组合
- **THEN** 每协议真实鉴权/握手、验证节点 IP 的 TCP/UDP socket、目标 204 和逐项 TLS/SNI/Host 证据；不支持组合安全拒绝、不伪称支持，任一失败行不汇总 PASS

#### Scenario: 并发矩阵不以竞态或偶发 EOF 宣告完成
- **WHEN** 六协议与适用传输在 race 检测重复运行，包含双向 HTTP2/gRPC 及 HTTPUpgrade
- **THEN** 每行真实 204/身份/授权 socket 且无 data race/偶发 EOF；服务端等请求 body 后回复时客户端 MUST 先写再读、不死锁；失败保持未完成，不跳过或重试到绿

#### Scenario: 应用层阻断与协议鉴权负例
- **WHEN** 应用层解析遇混合公私/重绑/非法节点或目标/内网重定向/失效凭据/取消/错协议密码/TLS 错 SAN 或 CA/不支持独立 gRPC Host/QUIC UDP 端口异常
- **THEN** 拒绝路径无未授权 socket/available/凭据泄漏；合法节点鉴权失败不访问目标，成功显式 SNI/Host 保持真实域名身份；测试专属映射不得进入生产

#### Scenario: 显式身份与错误证书
- **WHEN** 显式指定 SNI/Host 或 inbound 给错证书/密钥/私网目标
- **THEN** 显式身份不被默认覆盖，错证书/凭据/目标失败，不以 SkipCertVerify 或私网豁免求通过

### Requirement: 探针预算与跨运行公平
默认 SHALL 低流量测活，昂贵阶段按需选择；MUST 限制节点×profile 总任务、队列、全局和每 RunID 并发、连接/运行时间、响应/测速字节；超额拒绝并释放容量。持续提交的大 run MUST NOT 使已接纳小 run 饥饿。缓存仅可用于未过期且匹配版本/授权来源/profile 的成功结果。

#### Scenario: 两个竞争 run 的执行进展与取消
- **WHEN** 大 run 持续入队且部分阻塞，小 run 接纳后取消大 run
- **THEN** 小 run 在大 run 全结束前前进，峰值受限，取消有界释放，无 FIFO 队头饥饿

#### Scenario: 超预算、禁用昂贵阶段与缓存失效
- **WHEN** 超额、未选昂贵阶段或版本/来源失效
- **THEN** 明确失败、无 available，昂贵阶段不出站，旧缓存不作为新证据

### Requirement: 关闭与回调完成可观察且不虚报干净退出
队列 SHALL 原子拒绝关闭后新接纳，已接纳任务精确调用一次 completion callback；Close SHALL 能在 Execute/OnComplete 中调用且不自等待。兼容 Wait 不证明 callback 完成；外部协调者 SHALL 停接纳并有界 drain 等已接纳 callback/执行/调度器退出。DB 释放前 MUST 证明写入结束；超时 MUST 报未完成而非 clean。隔离 HTTP 服务真实接收任务后停服的成功/超时双分支不可被只 GET、hook Submit、queue helper 替代。clean MUST 晚于真实 DB.Close 成功；Close 失败非零退出且无 clean；超时不得提前 Close，进程退出后能重开 DB 不代替关闭路径观察。

#### Scenario: 最后一个回调晚于 Wait 返回
- **WHEN** Execute 结束 OnComplete 阻塞，协调者 Wait 后调用有界 drain
- **THEN** Wait 可先返但不宣告完成；放行后 drain 才成功且精确一次 completion

#### Scenario: 关停、提交、取消与重入交错
- **WHEN** Submit/Close、CancelRun/Close 竞态、回调中重入或执行不响应取消
- **THEN** 明确线性化点、无重复/遗漏/自等待死锁；截止未完成可诊断失败，不假报 clean

#### Scenario: 认证 HTTP route 真实提交且不得有测试后门
- **WHEN** 隔离子进程以真实 Bearer + 唯一 Idempotency-Key POST `/api/v1/probes/runs`，另试未经授权及重复 key
- **THEN** 201 run_id 与 SQLite 和 Runner/queue 同一 task 匹配，拒绝未认证/重复；仅测试内部装配 Runner，不从生产 HTTP/flag/env 开测试后门、不直接 hook Submit

#### Scenario: 带在途 callback 的进程级正常停服
- **WHEN** 认证 POST 创建任务，同步屏障确认 callback 阻塞，SIGTERM/drain 后放行
- **THEN** 同步事件+DB 读回证明 callback 持久化并返回→Drain 成功→真实 DB.Close 成功→clean→cmd.Wait exit0，不用 sleep/轮询代替因果

#### Scenario: 带在途 callback 的进程级超时停服
- **WHEN** 同路径不合作 callback 阻塞至自然 drain 截止
- **THEN** timeout/close-skipped/失败事件及实际非零 ExitCode/pipe EOF，事件中无 DB.Close attempt、持久化或 clean；外部 watchdog Kill 不算合格

### Requirement: 本地隔离成品交互与多模态视觉验收可复现
终态验收 SHALL 在本轮重新构建的非生产隔离 URL、临时 DB 上检验 health/auth、发布恢复、七路由、多视口主题及布局和探针路径。375×667、392×872、1280×800 浏览器 SHALL 提供主区宽度、关键 action/menu 边界、badges 内容高度、全部 targets/移动入口到达、截断与局部滚动的真实几何及行为证据；原始 PNG MUST 由具备 read 能力的独立 critic 直接检视，结合 bash 只读浏览器/DOM 量测。完整交付 MUST 同时取得 `CRITIC_FLOW_VERDICT: PASSED` 与 `CRITIC_VISUAL_VERDICT: PASSED`，旧 37 PNG 的 VISUAL FAILED、截图清单或只做 jsdom class 检查不得替代。

#### Scenario: 最新隔离预览重审
- **WHEN** 准备者提供最新构建隔离 URL/health/auth、脱敏 trace/几何 JSON、三视口全页及菜单/targets/badge/dock 原始 PNG 与网络 fixture，critic 直读原图并操作/量测
- **THEN** `main.scrollWidth <= main.clientWidth`、关键动作/菜单 rect 均在 viewport、badge.scrollHeight <= clientHeight、五 targets 包括末项实际可达、dock 路由可辨且 Settings 不重复状态，并取得新版本 FLOW/VISUAL 双 PASSED；不触生产端口/卷

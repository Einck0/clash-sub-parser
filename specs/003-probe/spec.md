---
feature: 003-probe
title: Probe Capability Specification
status: candidate
schema: speckit-native
---

# Probe Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的主动探测引擎能力。系统必须通过原生 Mihomo 内核在沙箱环境中对节点执行真实拨号探测，提供多层级解耦的探测调度体系，支持并发关闭防死锁、拨号错误精确分类，并建立四态健康基线与防未来时间穿越校验。

## User Scenarios & Priority

### User Story 1 - 原生 Mihomo 隔离探针执行 (Priority: P1)
- **GIVEN** 系统需要验证一批节点的网络可用性与延迟
- **WHEN** 协调器触发探测作业
- **THEN** 系统为每个探测实例分配独立的临时工作目录，执行原生 Mihomo 拨号并采集确定性证据

### User Story 2 - 并发关闭防死锁与错误精确归因 (Priority: P1)
- **GIVEN** 探针正在执行网络读取或遭遇网络异常
- **WHEN** 发生外部并发关闭或拨号失败
- **THEN** 引擎安全退出不发生死锁，并将错误精确分类为 DNS 故障、超时、TCP Reset、TLS 握手或 SSRF 拦截

### User Story 3 - 多层级解耦调度与单节点插队 (Priority: P2)
- **GIVEN** 系统中有大量存量节点和新发现节点
- **WHEN** 协调器调度探测或管理员在界面手动触发单个节点测试
- **THEN** 1 分钟协调器扫描未探测节点，失败节点进入 5 分钟冷却，单节点请求支持即时插队

---

## Requirements

### 1. 探针执行核心与并发安全

#### Requirement: 原生 Mihomo 探针执行引擎 [FR-PROB-01] <a id="req-prob-mihomo-engine"></a>
系统 SHALL 使用原生 Mihomo 二进制或核心库作为唯一主力探测引擎。每个探测批次 MUST 在隔离的临时目录下启动，生成独立的临时配置与套接字，探针完成后受控销毁。

##### Scenario: 临时目录运行独立探针实例 <a id="scenario-prob-isolated-instance"></a>
- **GIVEN** 需要执行探测作业
- **WHEN** 探针引擎启动测试
- **THEN** 为作业创建专有临时目录，进程退出后临时目录完整清理，零资源泄漏

#### Requirement: 探针连接并发关闭防死锁 <a id="req-prob-concurrent-close"></a>
探针连接包装器（如 `safeRuntimeConn`）MUST 支持安全的并发 `Close()` 调用。当工作协程处于阻塞读取或写入状态时，外部并发调用 `Close()` MUST 立即解除阻塞，且 `Close()` 调用本身不得发生死锁。

##### Scenario: 阻塞读取中并发关闭立即促使返回 <a id="scenario-prob-concurrent-close-timeout"></a>
- **GIVEN** 工作协程正在 `safeRuntimeConn.Read()` 中阻塞等待网络数据
- **WHEN** 另一个协程并发调用 `safeRuntimeConn.Close()`
- **THEN** `Read()` 立即返回超时或连接关闭错误，`Close()` 本身在 100ms 内安全返回

##### Scenario: 重复关闭不阻塞且幂等 <a id="scenario-prob-close-idempotent"></a>
- **GIVEN** 连接已被关闭
- **WHEN** 再次调用 `Close()`
- **THEN** 第二次调用立即返回 nil，不发生 panic 亦不发生阻塞

#### Requirement: 探针拨号器错误精确分类 [FR-PROB-05] <a id="req-prob-dial-err-class"></a>
探针拨号器 MUST 对失败原因进行结构化精确分类，明确区分 DNS 解析失败、TCP 连接超时、TCP Reset、TLS 握手失败、SSRF 安全拦截及协议不支持。

##### Scenario: 域名无法解析分类为 DNS 故障 <a id="scenario-prob-dns-failure"></a>
- **GIVEN** 节点服务器域名不存在或 DNS 污染
- **WHEN** 探针发起拨号
- **THEN** 失败原因记录为 `dns_resolution_failed`，不影响该批次其他节点的探测

##### Scenario: 指向回环地址被 SSRF 拦截 <a id="scenario-prob-ssrf-intercept"></a>
- **GIVEN** 节点服务器地址指向 `127.0.0.1` 或私有内网段
- **WHEN** SSRF 拦截器进行校验
- **THEN** 拨号立即阻断，失败原因记录为 `ssrf_blocked`

##### Scenario: 握手超时精确归因 <a id="scenario-prob-timeout-classify"></a>
- **GIVEN** 远端服务器未在限定时间内完成 TLS 握手
- **WHEN** 握手超时定时器触发
- **THEN** 失败原因记录为 `tls_handshake_timeout`

---

### 2. 多层级探测调度与健康判定

#### Requirement: 多层级探测周期解耦架构 [FR-PROB-02] <a id="req-prob-multi-tier-sched"></a>
探测调度器 SHALL 采用分层解耦模型：1 分钟协调器定时扫描未探测节点；失败节点进入 5 分钟冷却期；常规节点执行 1 小时调度周期；观测记录具备 24 小时有效窗口。历史 10 分钟单一定时器规范已被本模型替代。

##### Scenario: 1 分钟协调器扫描未探测节点 <a id="scenario-prob-coordinator-sweep"></a>
- **GIVEN** 系统刚导入了一批新节点（状态为未探测）
- **WHEN** 1 分钟协调器定时器触发
- **THEN** 协调器立即捞取这批未探测节点并分发给工作协程执行测试

##### Scenario: 失败节点 5 分钟冷却 <a id="scenario-prob-failure-cooldown"></a>
- **GIVEN** 某个节点刚发生探测失败
- **WHEN** 调度器在 5 分钟冷却期内评估该节点
- **THEN** 调度器跳过该节点，防止频繁对失效节点无效重试造成资源浪费

#### Requirement: 单节点插队与优先队列调度 [FR-PROB-03] <a id="req-prob-priority-queue"></a>
系统 SHALL 支持优先队列调度。当用户在管理面板手动触发某个节点的“即时测试”时，该作业 MUST 插队至队列头部优先执行。

##### Scenario: 单节点即时插队探测 <a id="scenario-prob-jump-queue"></a>
- **GIVEN** 队列中排队等待了 50 个常规探测任务
- **WHEN** 管理员点击节点 N 的手动测试按钮
- **THEN** 节点 N 插入高优先队列，在下一个可用工作协程中立即开始执行

#### Requirement: 探测观测证据与健康基线判定 [FR-PROB-04] <a id="req-prob-health-baseline"></a>
系统 SHALL 提供四态健康判定基线（ALIVE 可用、TIMEOUT 超时、FAILED 失败、UNCHECKED 未探测）。每次判定 MUST 伴随结构化的观测证据记录（包括 RTT 延迟、丢包率、测试时间戳与错误摘要）。

##### Scenario: 四态健康判定基线 <a id="scenario-prob-health-four-state"></a>
- **GIVEN** 完成探测的节点观测结果
- **WHEN** 健康评估器计算节点状态
- **THEN** 准确输出四态之一，并在观测记录中持久化 RTT 与证据指纹

#### Requirement: 观测数据防未来时间穿越校验 [FR-PROB-06] <a id="req-prob-timestamp-sanity"></a>
系统在接收并写入观测数据时，MUST 对时间戳执行合理性校验。记录的时间戳不得晚于当前服务器时间（允许 5 秒时钟抖动容限），防止未来时间穿越污染健康度排序。

##### Scenario: 拒绝未来时间戳记录 <a id="scenario-prob-no-future-timestamp"></a>
- **GIVEN** 客户端或上报数据包含晚于当前时间 1 小时的时间戳
- **WHEN** 写入服务进行校验
- **THEN** 系统拒绝该异常时间戳，使用当前服务器时间修正或抛出数据异常警告

#### Requirement: 辅助 Sing-box 探针构造器隔离 [FR-PROB-07] <a id="req-prob-singbox-isolation"></a>
针对 Sing-box 辅助探针构造器，系统 SHALL 保持其独立性与协议边界隔离，仅在特定辅助场景使用，不得越权替换主 Mihomo 探针引擎。

##### Scenario: Sing-box 仅作为辅助不越权 <a id="scenario-prob-singbox-fallback"></a>
- **GIVEN** 常规节点探测作业
- **WHEN** 默认探测流水线启动
- **THEN** 严格使用 Mihomo 探针引擎执行，Sing-box 构造器保持待命不越权介入

---

---

---

---

## Supplemental Retained Behavioral Capabilities & Historical Clauses

### Overview
本节完整收录历史变更中属于本领域的行为规范、具体契约（包含精确状态码、传输参数、请求头校验等）与验收场景，保持行为约束无损物理留存与点对点双向追溯。

### Retained Functional Requirements

#### Requirement: 探测作业状态机与幂等提交 [HR-046] <a id="req-prob-hr-046"></a>

系统 SHALL 将探测建模为 `queued`、`running`、`succeeded`、`failed`、`cancelled` 和 `expired` 状态。提交同一节点集合、探测配置版本与幂等键的重复请求 MUST 返回原作业而不启动重复探测。作业取消 MUST 阻止尚未开始的任务并传播截止时间到执行中的任务。

##### Scenario: 网络重试重复提交 <a id="scenario-prob-hr-046-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 调用者以相同幂等键重复提交仍有效的探测请求
- **THEN** 系统返回同一作业标识且不新增第二组探测任务

##### Scenario: 取消运行中作业 <a id="scenario-prob-hr-046-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 授权调用者取消一个运行中探测作业
- **THEN** 未开始任务转为取消，运行任务在其截止时间或取消信号后结束，作业终态为 `cancelled`

#### Requirement: 有界并发与单节点排他 [HR-047] <a id="req-prob-hr-047"></a>

系统 SHALL 按配置执行有限滑动窗口并发，默认值必须位于 10 至 20 之间且设置上限。任一 `logical_id` 在同一探测类型和 TTL 内 MUST 至多有一个活动执行；超额提交 MUST 排队或返回明确的容量错误，不能无界创建 goroutine、网络连接或测速流量。

##### Scenario: 同一节点的并发深探测 <a id="scenario-prob-hr-047-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 同一节点已有未终止的同类型深探测
- **THEN** 后续请求复用、排队或收到明确冲突结果，且不会并行建立第二条同类型探测通道

#### Requirement: 证据化能力观察 [HR-048] <a id="req-prob-hr-048"></a>

系统 SHALL 为每个探测步骤保存探测配置版本、开始和完成时间、超时或网络错误类别、HTTP 状态或握手摘要、延迟、出口观察与经脱敏处理的判定证据。能力结论 MUST 采用 `available`、`restricted`、`unknown`、`error` 或 `stale`，不得将挑战页、契约漂移、认证页、DNS 失败或超时解释为 `available`。

##### Scenario: 平台响应不符合预期契约 <a id="scenario-prob-hr-048-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 平台响应无法匹配当前探测配置版本的可用性判定契约
- **THEN** 结果标为 `unknown` 或 `error` 并保存漂移证据，而非标为 `available`

#### Requirement: 真正经被测节点的出口与能力请求 [HR-049] <a id="req-prob-hr-049"></a>

系统 MUST 使出口 IP、地理与平台请求经被测节点构建的内存代理管道发送；系统代理、直连连接或其他节点的连接不得充当被测节点证据。每个结果 MUST 能关联到节点 `logical_id` 和该执行的连接配置摘要。

##### Scenario: 本机代理可用但被测节点失败 <a id="scenario-prob-hr-049-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 本机环境代理可访问目标而被测节点无法建立探测管道
- **THEN** 结果记录被测节点失败，且不得借用本机代理给出可用结论

#### Requirement: 设置页编辑唯一全局筛选 [HR-091] <a id="req-prob-hr-091"></a>

系统 SHALL 在设置页展示、编辑并保存已有全局节点筛选定义，不另建独立持久化配置；探针判定条件 SHALL 明确指定探针类型、合法判定值（available/restricted/unknown/error/stale）及可选观测有效期，并支持现有合法的非探针条件及删除条件。多条件 SHALL 按 AND 语义执行；保存无条件 SHALL 恢复默认透传。读取或保存失败 SHALL 显示错误而非假称保存成功或默默用空条件覆盖已有配置。

##### Scenario: 从设置页保存并回显 <a id="scenario-prob-hr-091-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 管理员在设置页设置 baseline 探针判定为 available 且有效期为合法范围内的数值并保存，然后重新打开设置页或策略页
- **THEN** 两入口从同一全局定义读回该条件，下一次新预览和新发布只选择通过该条件的节点

##### Scenario: 无条件与校验失败 <a id="scenario-prob-hr-091-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 管理员清空所有条件保存，或输入不支持的探针类型/判定/有效期
- **THEN** 空条件对所有原有合格节点透传；无效条件被拒绝且已保存定义不变

#### Requirement: 探针结果及生效范围说明准确 [HR-092] <a id="req-prob-hr-092"></a>

设置页与策略页 SHALL 区分持久观测的 verdict、有效期以及探针任务 queued/probing 等非持久瞬态；未测、超出有效期的观测 SHALL 不匹配任何探针条件（包括否定条件），默认有效期 SHALL 与服务端一致。界面 SHALL 不声称现有筛选核对节点当前凭据版本；SHALL 解释新预览/新发布按当前条件生成，而已经创建的直链内容保持不变，须重新发布才更新。

##### Scenario: 没有可信的当前观测 <a id="scenario-prob-hr-092-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点未测或最近观测超出所设置的有效期，而条件为 verdict 等于或不等于某状态
- **THEN** 节点均不匹配，界面解释其排除原因，不将 queued/probing 当成可选的持久 verdict

##### Scenario: 修改条件后查看旧直链 <a id="scenario-prob-hr-092-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 管理员保存新的全局条件但没有重新发布
- **THEN** 新预览采用新条件，既有发布直链仍返回创建时冻结的原内容，界面提示需要新发布

#### Requirement: 四目标共用筛选且读取异常安全闭合 [HR-093] <a id="req-prob-hr-093"></a>

Mihomo、sing-box、Surge、Quantumult X 的新预览、预检及发布 SHALL 从同一已解析候选集合编译；筛选定义读取失败 SHALL 拒绝本次新解析，而不是将其解释成空条件；已配置条件依赖的观测或来源读取失败、依赖未接线 SHALL 拒绝本次新解析，不得放行全部节点。无条件的正常输入 SHALL 保持已有导出效果；既有已发布冻结直链 SHALL 不被重新解析或被新条件影响。错误 SHALL 明示且不泄漏敏感节点凭据。

##### Scenario: 读取筛选定义失败 <a id="scenario-prob-hr-093-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 新预览或新发布遇到全局筛选存储读取失败
- **THEN** 请求失败且不会产生新发布或返回意外扩大节点的配置

##### Scenario: 已配置探针筛选但观测读取失败 <a id="scenario-prob-hr-093-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 全局或组级筛选包含探针条件且观测读取失败/必要依赖缺失
- **THEN** 预览、预检和新发布均阻断，不允许无观测集合引发错误的全量放行

##### Scenario: 各目标相同候选节点 <a id="scenario-prob-hr-093-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 相同有效输入下分别请求四个目标的预览和新发布
- **THEN** 每个目标以同一筛选结果为基础，再按其已有的协议与格式能力进行编译；不因目标不同绕过全局条件

#### Requirement: 配置筛选导致零候选时防止意外发布 [HR-094] <a id="req-prob-hr-094"></a>

当已经配置的全局或组级条件使解析后候选节点为零，系统 SHALL 在新预览、预检和新发布给出可识别的空筛选结果或阻断诊断，新发布 SHALL 在持久化前被拒绝；不得将零节点当作成功导出、也不得因为不存在路由组就跳过防护。未配置条件时 SHALL 保留原有空组预检约束和旧发布直链行为。

##### Scenario: 没有策略组且筛选出零节点 <a id="scenario-prob-hr-094-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 存在激活节点且配置了探针筛选，但因未测或超龄导致候选为零，且拓扑不含路由组
- **THEN** 四个目标的新预览与新发布均明确提示筛选后无节点，新发布不创建记录；不能只靠空路由组预检拦截

##### Scenario: 有路由组筛选出零节点 <a id="scenario-prob-hr-094-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 已配置的筛选使路由所指组无可用成员
- **THEN** 原有空路由组诊断与零候选诊断保持可辨识，并拒绝无效新发布

#### Requirement: 确认框不被其他模态遮盖 [HR-096] <a id="req-prob-hr-096"></a>

确认框 SHALL 在已有原生模态对话框上方显示（同属浏览器 top layer 或先关闭父对话框再打开），不得仅靠增加普通容器的 `z-index` 试图盖过 `showModal()`；确认、取消及关闭 SHALL 保持原有事件语义与焦点可达性。

##### Scenario: 发布页打开模态框后撤销 <a id="scenario-prob-hr-096-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 在已有原生 `ModalDialog` 打开期间触发 `ConfirmModal`
- **THEN** 确认框标题、正文和操作按钮位于可交互最上层，不受父 dialog 的遮挡或焦点陷阱影响

#### Requirement: 短高度视口保持正文与操作可用 [HR-097] <a id="req-prob-hr-097"></a>

`Popover`、`ModalDialog`、`ConfirmModal`、`Drawer` 的布局 SHALL 使用项目已有自适应视口约束、安全区和流式 flex/scroll 能力，使较长标题、正文与操作区在短高度视口及窄屏下仍可浏览、滚动和点击；在真实四类代表视口及短高度场景 SHALL 无视口外操作按钮或强制水平溢出。关闭操作与背景交互 SHALL 仅影响当前最上层弹层。

##### Scenario: 长文案与短高度 <a id="scenario-prob-hr-097-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 对话框或抽屉包含长标题、多段正文及操作按钮，视口高度缩短
- **THEN** 正文可滚动到末尾，操作按钮与关闭入口仍可见且可用，安全区内无裁切或遮挡

##### Scenario: 四视口回归 <a id="scenario-prob-hr-097-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 在窄手机、宽手机、平板及桌面视口操作订阅、发布、策略与探针页面浮层
- **THEN** 菜单项、表单项和确认框操作可完整访问，不发生剪裁、失焦或对话框之间的遮挡

#### Requirement: 逐节点类别增量过期判定与失败冷却 [HR-099] <a id="req-prob-hr-099"></a>

系统 SHALL 在每次巡检中评估所有激活节点在计划所选类别（ProbeKinds）下的最新观测状态；若某一节点和类别不存在历史观测记录，或最新观测记录的 `ObservedAt` 距今已达到或超过用户配置的 `interval_seconds`，系统 SHALL 判定该 (node, kind) 到期并待测；对于最近失败或超时的观测记录，系统 SHALL 引入不低于 5 分钟的合理冷却窗口，禁止在每轮巡检中无间隔盲目重试。

##### Scenario: 未观测与过期节点精准识别 <a id="scenario-prob-hr-099-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 巡检扫描发现部分节点从未观测，部分节点观测时间超过配置周期，其余节点观测仍处于有效期内
- **THEN** 系统仅将从未观测和已过期的 (node, kind) 纳入待派发集合，处于有效期内的节点不产生探测任务

##### Scenario: 探测失败节点的冷却保护 <a id="scenario-prob-hr-099-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 某一节点在上一次巡检中探测失败且距今不足冷却时间
- **THEN** 本轮巡检跳过该失败节点，避免高频自旋消耗网络与并发资源

#### Requirement: 队列去重与单实例租约防重 [HR-101] <a id="req-prob-hr-101"></a>

系统 SHALL 在派发前校验当前探测池与执行状态，复用 `EnqueuePeriodicDedupe` 队列模式，对已在队列或正在执行的节点/类别进行去重，严禁重复入队；巡检执行批次 SHALL 依托 CAS 租约保持跨多实例的单执行者语义；在不足巡检周期的单批探测执行中或完成后，系统 SHALL 严禁自旋重复派发。

##### Scenario: 重复巡检去重抑制 <a id="scenario-prob-hr-101-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 巡检触发时部分节点仍在上一批次执行中或在队列等待
- **THEN** 系统跳过这些节点或仅对空闲类别入队，不产生重复并发任务

#### Requirement: 前端巡检语义明示与低频保底刷新 [HR-102] <a id="req-prob-hr-102"></a>

前端控制台 SHALL 清晰展示“每 10 分钟巡检过期节点”及“每节点/类别有效期 = 配置周期”，完整显示下次轻量扫描时间；当界面处于空闲状态（无活跃探测任务）时，前端 SHALL 保持低频保底刷新（如每 15 秒至 30 秒轮询一次），确保后台周期巡检产生的新批次在无需用户手动刷新的情况下自动呈现；在用户点击“立即触发”后，系统 SHALL 持续跟踪批次与运行状态，直至状态转为终态。

##### Scenario: 空闲页面自动呈现新巡检批次 <a id="scenario-prob-hr-102-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户停留在探测面板且当前无活跃任务，后台到达 10 分钟时钟触发了新批次
- **THEN** 前端在低频保底刷新周期内自动感知并展示新生成的批次与运行记录

##### Scenario: 立即触发后的持续状态追踪 <a id="scenario-prob-hr-102-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户在控制台点击“立即触发巡检”
- **THEN** 前端立即激活高频轮询跟踪该次派发的批次与运行，直到全部完成或进入稳态

#### Requirement: Provider 启用、凭据与配额边界 [HR-106] <a id="req-prob-hr-106"></a>

系统 SHALL 仅调用已由部署端显式启用且具有有效 secret reference 的官方 API provider。provider 凭据 MUST 不出现在 API 响应、审计、日志、证据摘要、导出物或前端状态。系统 MUST 对每个 provider 实施配置化的最大并发、请求速率、每日预算和 deadline；预算耗尽、缺少凭据、429 或 provider 不可用 MUST 产生可区分的 `unknown` 或 `error` 观察，且不得重试到无界。

##### Scenario: Provider 预算耗尽 <a id="scenario-prob-hr-106-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 某个已启用 provider 的请求将超过其配置的预算
- **THEN** 系统不发送该请求，记录明确的预算原因，并将节点风险状态保留为 `unknown` 而非低风险

#### Requirement: 不可变、最小化的风险观察 [HR-107] <a id="req-prob-hr-107"></a>

系统 SHALL 为每个成功、受限或失败的 IP risk 查询创建不可变观察，包含节点 `logical_id`、provider 标识及 schema 版本、观察时间、TTL、规范化风险 score 或缺失原因、置信度、匿名化和网络类别、判定状态、SHA-256 evidence digest 与有限的脱敏摘要。系统 MUST 不持久化完整出口 IP、完整第三方响应、API key、cookie、请求 URL query 或网页 body。

##### Scenario: 成功查询被持久化 <a id="scenario-prob-hr-107-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 已启用 provider 返回符合其版本化响应合同的风险数据
- **THEN** 系统保存可关联节点和 provider 版本的不可变观察及摘要，不保存完整 IP 或原始 payload

#### Requirement: 风险证据新鲜度与缓存隔离 [HR-108] <a id="req-prob-hr-108"></a>

系统 SHALL 将风险观察限定为其 provider、出口身份摘要和 TTL。缓存命中 MUST 仅用于具有同一 node `logical_id`、同一 provider/schema version、未过期且出口身份摘要未变化的查询。过期、出口身份变化、provider 合同不匹配或无法验证出口身份的结果 MUST 不被视为当前有效风险证据。

##### Scenario: 出口变更后的查询 <a id="scenario-prob-hr-108-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点新的出口身份摘要与最近风险观察的摘要不同
- **THEN** 系统不复用旧观察，并在新查询完成前向风险 policy 提供 `unknown`

#### Requirement: 多 provider 归一化不伪造确定性 [HR-109] <a id="req-prob-hr-109"></a>

系统 SHALL 将 provider 的原始风险语义归一化为明确的 score 范围、置信度、网络类别和匿名化特征，并保留 provider 标识与版本。没有可比映射、provider 间冲突或置信度低于 policy 最低值时 MUST 标为 `unknown` 或交由 policy 的显式冲突策略；系统不得将缺失或冲突自动折算为低风险。

##### Scenario: Provider 数据不完整 <a id="scenario-prob-hr-109-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** provider 响应不包含 policy 所需的 score 或网络类别
- **THEN** 系统记录合同缺失并产出 `unknown`，不推导 `allow`

#### Requirement: 证据化能力观察 [HR-117] <a id="req-prob-hr-117"></a>

系统 SHALL 为每个探测步骤保存探测配置版本、开始和完成时间、超时或网络错误类别、HTTP 状态或握手摘要、延迟、出口观察与经脱敏处理的判定证据。能力结论 MUST 采用 `available`、`restricted`、`unknown`、`error` 或 `stale`，不得将挑战页、契约漂移、认证页、DNS 失败或超时解释为 `available`。`ip_risk` MUST 是与 baseline、geo、streaming、AI 和 speed 分离的版本化探测类型；它的 provider 风险结论不得改写其他探测类型的 verdict。

##### Scenario: 平台响应不符合预期契约 <a id="scenario-prob-hr-117-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 平台响应无法匹配当前探测配置版本的可用性判定契约
- **THEN** 结果标为 `unknown` 或 `error` 并保存漂移证据，而非标为 `available`

##### Scenario: IP 风险 provider 契约漂移 <a id="scenario-prob-hr-117-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** IP risk provider 响应不能匹配当前 provider schema version
- **THEN** 系统记录独立的 IP risk `unknown` 或 `error` 观察，且不改变该节点已有的 geo 或 capability verdict

#### Requirement: 真正经被测节点的出口与能力请求 [HR-118] <a id="req-prob-hr-118"></a>

系统 MUST 使出口 IP、地理与平台请求经被测节点构建的内存代理管道发送；系统代理、直连连接或其他节点的连接不得充当被测节点证据。每个结果 MUST 能关联到节点 `logical_id` 和该执行的连接配置摘要。IP risk provider 请求同样 MUST 经该节点通道执行，并显式忽略宿主 `HTTP_PROXY`、`HTTPS_PROXY` 和 `ALL_PROXY`。

##### Scenario: 本机代理可用但被测节点失败 <a id="scenario-prob-hr-118-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 本机环境代理可访问目标而被测节点无法建立探测管道
- **THEN** 结果记录被测节点失败，且不得借用本机代理给出可用结论

##### Scenario: IP risk 请求遇到宿主代理配置 <a id="scenario-prob-hr-118-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 宿主进程设置了 HTTP proxy 环境变量
- **THEN** IP risk 请求仍仅使用被测节点 outbound，环境变量不会改变出口证据

#### Requirement: 定时测速节点池去重入池（Periodic Probe Node Pool Deduplication） [HR-129] <a id="req-prob-hr-129"></a>

当定时测速（`PeriodicCoordinator` 或 `POST /api/v1/probes/schedule/trigger`）触发时，系统 MUST 将当前活跃节点按序加入节点池队列尾部；如果某个节点已经在节点池中（无论处于排队等待 `queued` 还是正在检测 `probing`），系统 MUST 静默跳过该节点，绝不重复添加入池，且 MUST NOT 因 `ErrNodeConflict` 导致定时批次或探测任务报错失败。

##### Scenario: 定时测速遇到已在节点池中排队或正在检测的节点时静默去重跳过 <a id="scenario-prob-hr-129-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点 `node-hk-01` 已在节点池中（处于 `probing` 正在检测或 `queued` 排队等待），此时定时测速触发并对活跃节点 `["node-hk-01", "node-sg-02"]` 入池
- **THEN** 系统 MUST 跳过已在池中的 `node-hk-01`（不重复入队、不抛出 `ErrNodeConflict`），仅将不在池中的 `node-sg-02` 加入节点池队列尾部，并正常完成批次与观测持久化

#### Requirement: 手动检测插在节点池队列最前面（Manual Probe Front-of-Queue Preemption & Promotion） [HR-130] <a id="req-prob-hr-130"></a>

当用户手动触发检测（单节点重测、测速已选节点或一键全量测速）时，系统 MUST 将目标节点插在节点池队列最前面（队首）优先调度执行；若目标节点原本已在节点池等待队列后方（如定时测速排队中），系统 MUST 将其提升（Promote）至队列最前面并合并所需探测维度；若目标节点当前正处于 `probing`（检测中），系统 MUST 允许复用或紧接队首完成检测，绝不返回冲突错误。

##### Scenario: 定时测速队列排队期间手动触发单节点或批量检测插队到最前面 <a id="scenario-prob-hr-130-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点池队列中有定时测速任务 `["node-01", "node-02", ..., "node-20"]` 正在排队等待，用户手动触发对 `"node-18"` 与 `"node-99"` 的检测
- **THEN** 系统 MUST 将 `"node-18"`（从原队列位置提升）与 `"node-99"` 插在节点池队列最前面，使下一个空闲工作协程优先执行 `"node-18"` 与 `"node-99"` 的检测，再去执行其余定时排队节点

#### Requirement: 节点状态支持“检测中（`probing`）”与节点池 5 大核心统计指标 API [HR-131] <a id="req-prob-hr-131"></a>

后端 `GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}` 与 `PATCH /api/v1/nodes/{logical_id}/connection` 返回的 `NodeView` MUST 包含 `probe_state`（`"probing"` | `"queued"` | `"idle"`），且当节点正在节点池中执行检测时将 `probe_state` 置为 `"probing"`、`health_status` 置为 `"probing"`。后端 MUST 提供 `GET /api/v1/probes/pool` 接口，实时返回节点池与全网活跃节点的 5 大核心统计指标：`queue_nodes_count`（当前队列中的节点数）、`untested_count`（未测数）、`total_count`（总数）、`unavailable_count`（不可用数）、`available_count`（可用数），以及 `probing_count`、`queued_waiting_count`、`healthy_count`、`degraded_count`、`probing_node_ids` 与 `queued_node_ids`。

##### Scenario: 查询节点池统计接口与节点列表实时反映 5 大指标及“检测中”状态 <a id="scenario-prob-hr-131-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 仓库共有 10 个活跃节点（其中 5 个可用、2 个不可用、3 个未测），当前有 2 个节点正在执行检测（`probing`）、3 个节点在池中排队等待（`queued`），管理员调用 `GET /api/v1/probes/pool` 与 `GET /api/v1/nodes`
- **THEN** `GET /api/v1/probes/pool` MUST 返回 `queue_nodes_count=5`（`probing_count=2`、`queued_waiting_count=3`）、`untested_count=3`、`total_count=10`、`unavailable_count=2`、`available_count=5`；且 `GET /api/v1/nodes` 中对应检测中节点的 `probe_state` 与 `health_status` MUST 为 `"probing"`

#### Requirement: 前端节点池状态看板展示 5 大核心指标与节点“检测中”动态状态设计 [HR-132] <a id="req-prob-hr-132"></a>

探针引擎前端（`web/src/features/probes/ProbesView.vue`）顶部 MUST 设计直观、美观的节点池检测状态看板，醒目显示「当前队列中的节点数」、「未测数」、「总数」、「不可用数」、「可用数」5 项核心指标（并支持点击指标卡快速筛选对应状态节点）以及定时自动入池控制卡片。`ProbesView.vue` 与 `NodesView.vue` 的节点状态徽章、状态筛选器与详情抽屉 MUST 支持 **「检测中」**（`probing`，带动态呼吸/旋转指示）与 **「队列中」**（`queued`）状态展示，并在手动测速时即时反馈插队优先状态。

##### Scenario: 用户在探针工作台查看节点池 5 大指标看板并触发手动插队检测观察“检测中”状态 <a id="scenario-prob-hr-132-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户打开探针引擎页面查看顶部看板，并点击某节点的「立即重测」或顶部「一键全量测速 / 测速已选节点」
- **THEN** 顶部看板清晰展示「当前队列中的节点数、未测数、总数、不可用数、可用数」5 大指标；被触发节点立即显示「检测中」（或「队列中」）动态徽章及插队提示，状态筛选器可按「检测中」筛选，检测完成后自动刷新为最新可用/不可用状态与延迟毫秒数

#### Requirement: 全局先于策略组的两级筛选 [HR-133] <a id="req-prob-hr-133"></a>

系统 SHALL 保存全局节点筛选与各策略组筛选；解析顺序 SHALL 为已有激活/准入/风险硬拒绝 → 全局条件 → 策略组条件。组级条件 SHALL 只在全局留下的集合内筛选，不能让被全局拒绝的节点重新进入策略组。无条件 SHALL 对现有配置表现为恒真，未使用筛选的老策略继续生效。

##### Scenario: 全局先筛 <a id="scenario-prob-hr-133-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 全局只允许协议 A，而组级允许名称含 X 的节点
- **THEN** 组仅包含同时满足协议 A 和名称 X 的节点，协议 B 即使名称 X 也被排除

##### Scenario: 旧配置 <a id="scenario-prob-hr-133-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 升级前的策略组没有筛选定义
- **THEN** 原有节点边、组排序和发布目标在相同仓库输入下保持不变

#### Requirement: 明确可校验的筛选语言 [HR-134] <a id="req-prob-hr-134"></a>

系统 SHALL 支持结构化 AND 条件与显式取反、枚举合法比较，不执行用户代码或无限复杂正则；可筛字段为 display_name 包含/不包含、protocol 等于/不等于、source_subscription_ids 包含/不包含、指定 probe kind 的 verdict 与 latency_ms 上限及 freshness_seconds。条件数、文本长度和新鲜度 SHALL 有界；未知字段、非法操作、负延迟、不合法种类 SHALL 在保存时返回字段级错误。

##### Scenario: 来源与探测属性 <a id="scenario-prob-hr-134-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点关联指定订阅且最近 baseline 观测 verdict=available、延迟低于所设阈值且未超过新鲜度
- **THEN** 该节点通过对应 AND 条件；来源必须取真实关联而不是节点名称推断

##### Scenario: 无有效观测 <a id="scenario-prob-hr-134-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 设有 probe 条件但观测缺失、超龄、结果未知或凭据版本与当前节点不匹配
- **THEN** probe 条件视为不匹配，包括否定条件；返回可辨识的诊断，不把旧结果当作当前证据

#### Requirement: 显式成员与组级作用域 [HR-135] <a id="req-prob-hr-135"></a>

系统 SHALL 对筛选生效后的显式节点成员执行组级筛选；没有显式节点边且设置了组级筛选的组 SHALL 从全局候选集合动态选择匹配节点。子组边保持原有拓扑语义，父组筛选 SHALL 限制其所有最终展开节点，且不得使子组节点绕过父组/全局限制；结果去重且稳定排序。未设置组筛选的无节点边组不应被隐式填满。

##### Scenario: 显式节点被拒绝 <a id="scenario-prob-hr-135-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 显式成员已被全局或本组条件排除
- **THEN** 编译输出不包含此成员，预览显示排除原因，持久化边本身不被删除

##### Scenario: 动态组与嵌套组 <a id="scenario-prob-hr-135-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 空节点边组含组级条件，或父组包含子组并设置筛选
- **THEN** 动态组稳定取匹配候选；父组展开结果不含本组拒绝的子组节点，且原拓扑无环验证保持有效

#### Requirement: 预览与发布共同的快照及空组防护 [HR-136] <a id="req-prob-hr-136"></a>

系统 SHALL 对预览、预检、五类客户端编译和发布使用同一输入水印与规范化解析；筛选定义和观测选择信息参与输入摘要，预览 SHALL 展示每层数量、排除理由和空组。若目标路由所指组过滤后无有效节点或子组，发布/下载 SHALL 明确拒绝并返回脱敏诊断，不输出空策略或引用不存在的节点；旧配置中的空组仍遵循既有预检行为。

##### Scenario: 一致性与空集 <a id="scenario-prob-hr-136-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 同一冻结的库存、关联来源、策略和探测快照被预览和发布消费，且一个被路由引用的组筛成空集
- **THEN** 两入口返回同一筛选诊断与摘要，发布拒绝生成无效配置；未筛成空集时各目标引用有效

##### Scenario: 输入变化 <a id="scenario-prob-hr-136-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 保存后探测结果更新或节点激活/来源发生变化
- **THEN** 新预览与发布采用新的共同输入快照和摘要，不复用过时的筛选结果

#### Requirement: 可管理周期计划 [HR-137] <a id="req-prob-hr-137"></a>

系统 SHALL 允许管理员查询和更新唯一的周期探测计划，包含 enabled、interval_seconds、kinds、下次到期时间及最近执行摘要；初始默认关闭。更新 SHALL 验证周期范围、合法探测种类及执行预算，记录非敏感审计事件；非管理员 SHALL 无法修改。

##### Scenario: 默认与更新 <a id="scenario-prob-hr-137-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 新库迁移后查询计划，随后管理员配置合法周期、探测种类并启用
- **THEN** 起始状态为关闭；启用配置及下次到期时间持久化，重启后保持一致

##### Scenario: 无效配置 <a id="scenario-prob-hr-137-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 管理员提交零/过短周期、非法种类或超预算配置
- **THEN** 请求被拒绝，原计划与已有执行保持不变

#### Requirement: 批次互斥、持久化恢复与停服 [HR-139] <a id="req-prob-hr-139"></a>

系统 SHALL 持久化计划、到期批次与运行关联；同一计划同一到期窗口在共享库的多个实例中最多一个实例取得执行权，租约失效后可确定性接管，但不得并行重复探测。重启 SHALL 恢复/终结遗留任务并继续未来周期，不补发无限历史批次；停服 SHALL 停止新批次并按现有队列 Drain 后关闭数据库。

##### Scenario: 双实例到期竞争 <a id="scenario-prob-hr-139-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 两个实例同时处理同一到期窗口
- **THEN** 至多一个实例创建该窗口的批次及运行，另一个实例不触发重复任务

##### Scenario: 崩溃与重启 <a id="scenario-prob-hr-139-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 执行中进程崩溃或长期停机后重启
- **THEN** 遗留任务按超时/取消/接管规则终结，不重复写入观测；仅安排一个未来到期窗口

##### Scenario: 停服排空 <a id="scenario-prob-hr-139-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 收到停服信号且已有周期批次进行中
- **THEN** 不再派发新批次，现有任务获取消或在界限内完成，并在安全排空后关闭数据库

#### Requirement: 可检查的执行证据 [HR-140] <a id="req-prob-hr-140"></a>

系统 SHALL 提供管理员可见的计划、批次、关联运行和已有观测的分页查询、失败/跳过摘要及取消/停用操作；响应 SHALL 不包含凭据明文、原始出站响应或未授权地址。

##### Scenario: 查看与停用 <a id="scenario-prob-hr-140-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 管理员停用计划并查看最近批次和关联运行
- **THEN** 后续窗口不再触发且历史脱敏证据仍可查询，运行状态与批次状态一致

#### Requirement: 手动探测支持全量活跃节点分页拉取、大批量任务预算与显式 `speed` 带宽测速 [HR-167] <a id="req-prob-hr-167"></a>

后端 `probe.DefaultRunner`（`internal/application/probe/runner.go`）在执行手动或周期探测任务时 SHALL 满足：
1. **全量活跃节点完整拉取**：当 `nodeIDs` 为空（全量测速）时，`Runner.Run` SHALL 按页循环查询 `r.nodes.List(ctx, domain.NodeFilter{ActiveOnly: true, Pagination: ...})` 拉取全部活跃节点，不得仅拉取默认第一页（50 个节点）。
2. **显式勾选 `speed` 即视为 Opt-In**：当 `run.Kinds` 中显式包含 `domain.ProbeKindSpeed` 时，`executeTaskWithVerdict` SHALL 视为用户已显式 Opt-In 带宽测速，通过 `r.dialer` 建立节点代理连接并发起测速请求，以 `Result{OptIn: true, ...}` 评估结果并持久化 `ProbeObservation`，不得硬编码返回 `ErrSpeedProbeOptInRequired`。
3. **支持全量节点多维并发受控测速**：默认运行预算 `DefaultRunBudget.MaxTasks` SHALL 放宽以支持全量活跃节点的多维探测按调度器并发信号量依次执行，不再因 `len(targetNodes) * len(validKinds) > 512` 直接将整个 `ProbeRun` 标为 `failed`。

##### Scenario: 活跃节点超过 50 个且多维任务数超过 512 时发起一键全量测速 <a id="scenario-prob-hr-167-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 系统中存在 120 个活跃节点，用户发起 `node_logical_ids: []` 且包含 5 个探测维度（共 600 个子任务）的全量测速
- **THEN** `DefaultRunner` 分页拉取全部 120 个活跃节点并按并发信号量完成探测，Run 状态正常流转为 `succeeded` 而非因 50 条分页截断或 512 预算上限失败

##### Scenario: 用户显式勾选带宽测速（`speed`）发起探测 <a id="scenario-prob-hr-167-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户在 `kinds` 中显式包含 `"speed"` 发起探测任务且节点代理连通正常
- **THEN** `DefaultRunner` 通过节点代理请求测速端点并以 `OptIn: true` 完成评估，生成 `verdict: "available"`（或实际网络结论）与非零延迟的 `speed` 观测记录

#### Requirement: 探针引擎前端重构为以“人类直觉”和“节点”为核心的测速与可用性工作台 [HR-168] <a id="req-prob-hr-168"></a>

前端 `web/src/features/probes/**` SHALL 彻底重构为三层视觉架构的节点测速工作台，并消除面向机器的底层实现黑话：
1. **第一视觉层级：全景节点健康与延迟统计看板（KPI Summary Bar）**：
   - 页面顶部 SHALL 展示 4 张核心人类指标卡片：
     * **在线可用率**：`可用节点数 / 总活跃节点数`（含百分比进度条与正常/降级/异常/未测分布）；
     * **平均响应延迟**：全网可用节点平均 `ms`（含极速 `<100ms`、良好 `100-250ms`、较慢 `>250ms` 分布）；
     * **流媒体与 AI 解锁**：已解锁流媒体节点数 / 已支持 AI 节点数；
     * **定时自动测速状态**：当前开关状态（支持一键切换开/关）、易读定时间隔（如“每 1 小时”）、下次自动测速时间、以及「调整定时策略」快捷入口。
2. **第二视觉层级：直观的一键测速与多维筛选控制栏（Action & Filter Toolbar）**：
   - SHALL 提供主操作按钮 **「⚡ 一键全量测速」**（无需弹窗填写 `config_revision`，直接按当前开启的探测维度对全部活跃节点发起测速，并在顶部显示实时测速进度条与取消按钮）与 **「🎯 测速已选节点 (N)」**（对表格勾选节点或当前筛选结果发起定点测速）；
   - SHALL 在工具栏直接提供可点击开启/关闭的**探测维度胶囊开关（Probe Dimension Pills）**：`⚡ 连通与延迟 (必选)`、`🎬 流媒体解锁`、`🤖 AI 服务可用性`、`🛡️ IP 风险评估`、`🌍 落地地区识别`、`🚀 带宽测速`；
   - SHALL 支持按**关键词（节点名/服务器）**、**协议（SS/VMess/VLESS/Trojan/Hysteria2/WireGuard/TUIC）**、**订阅源**、**健康状态（全部 / 正常 / 降级 / 异常 / 未测速）**过滤，以及**按延迟排序（从快到慢 / 从慢到快）**。
3. **第三视觉层级：以“节点”为主体的实时测速结果表格/卡片列表（Node Probe Workbench Table）**：
   - 页面主体 SHALL 展示节点测速结果列表，每行包含：复选框（支持全选/多选定点测速）、**节点名称 (`display_name`)** + 协议标签 + 服务器地址（`server:port`）、**带颜色语义的连通状态与延迟 (`latency_ms`) 徽章**、**能力矩阵徽章（流媒体 / AI / 地区 / IP 风险）**、**人性化最后测速时间**，以及行级 **「⚡ 立即重测」** 与 **「查看详情」** 操作按钮。
4. **人性化定时自动测速配置与后台任务历史折叠区**：
   - 定时测速配置 SHALL 提供预设周期选择按钮（`每 15 分钟`、`每 30 分钟`、`每 1 小时`、`每 6 小时`、`每 12 小时`、`每天` 及自定义分钟数）+ 探测项目勾选 + 一键启用/停用；
   - 后台任务执行历史（Runs / Batches）SHALL 收敛为可切换次级视图或折叠面板，卡片标题采用人类可读的 **「手动测速任务 / 自动定时批次 · HH:mm:ss」**，彻底隐藏内部 UUID、Actor Scope、`config_revision`、Lease 租约、SHA256 证据指纹等底层噪音。
5. **结构化人类可读的测速详情抽屉（`ProbeEvidenceSheet.vue`）**：
   - 点击节点的「查看详情」或任务的「查看结果」时，抽屉顶部 SHALL 展示**节点名称 (`display_name`)**、协议、服务器地址与当前综合状态，将原始 `profile=... version=... verdict=... reason=... status=... latency_ms=...` 解析转换为**结构化人类可读卡片**（检测项目、结论、响应延迟、中文状态说明、检测时间），并支持在抽屉内一键「重新测速此节点」。

##### Scenario: 用户在探针工作台直观查看节点延迟并一键发起全量或单节点测速 <a id="scenario-prob-hr-168-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户进入“探针引擎”页面
- **THEN** 页面顶部展示 4 张核心 KPI 统计卡片，中部工具栏展示「⚡ 一键全量测速」与 6 个探测维度胶囊开关，主体区域以节点列表展示每个节点的名称、协议、`server:port`、彩色延迟毫秒徽章及流媒体/AI/IP 风险能力状态；点击「⚡ 一键全量测速」或某行的「⚡ 立即重测」即可无需填写 `config_revision` 直接启动测速并实时刷新进度与节点延迟

##### Scenario: 用户打开节点测速详情抽屉查看人类可读报告 <a id="scenario-prob-hr-168-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户点击任意节点行的「查看详情」按钮打开 `ProbeEvidenceSheet.vue`
- **THEN** 抽屉顶部显示该节点的 `display_name`、协议与服务器地址，各探测维度的观测记录以结构化中文卡片展示（包含检测项目名称、结论徽章、延迟 `ms`、中文状态说明如“HTTP 204 连通正常”及检测时间），不裸露 `profile=baseline version=baseline-v1` 机器串或内部 SHA256 指纹，且可点击「重新测速此节点」立即重测

#### Requirement: 节点台账页（`NodesView.vue`）探测状态与延迟联动 [HR-169] <a id="req-prob-hr-169"></a>

前端节点台账页（`web/src/features/nodes/{NodesView.vue,nodeView.ts,useNodes.ts}`）SHALL 与后端返回的最新节点探测读模型实时联动：
1. `nodeView.ts` SHALL 兼容解析后端 `NodeView` 的 `latency_ms`、`last_probed_at`、`health_status`、`probe_missing`、`probe_stale` 及结构化 `capabilities` 对象（同时向后兼容字符串格式 `capabilities` 单测夹具）。
2. `NodesView.vue` 节点列表与详情抽屉 SHALL 直接展示真实的节点健康状态、延迟毫秒数、流媒体与 AI 解锁徽章，并在节点详情抽屉中提供一键触发单节点测速或跳转查看测速详情的能力。

##### Scenario: 节点台账展示真实探测延迟并在详情抽屉触发单节点测速 <a id="scenario-prob-hr-169-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户打开“节点台账”页面查看已完成探测的节点，或打开节点详情抽屉点击单节点测速
- **THEN** 节点列表直接显示真实的健康状态（正常/降级/异常）、延迟毫秒数及流媒体/AI 解锁徽章，抽屉内可一键对该节点触发测速并刷新状态

#### Requirement: 整体健康以新鲜 baseline 为唯一依据 [HR-170] <a id="req-prob-hr-170"></a>

系统 SHALL 仅使用新鲜且可判定的 baseline 结果判定节点整体健康；健康/可用需要 baseline 实际通过，线路不可用需要 baseline 真实连通性/传输失败的有效证据。baseline 缺失、过期、unknown/stale，或因安全策略、配置与探针能力阻断而无法完成检测时，整体状态 SHALL 为未知/无法检测，不得以其他类别的失败或成功代替，也不得把阻断计为“已证线路不可用”。

##### Scenario: 非 baseline 成功或失败但 baseline 缺失 <a id="scenario-prob-hr-170-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 流媒体或 AI 类别有成功或失败，baseline 未有有效观测
- **THEN** 整体健康未知；各能力结论仅在自身类别呈现

##### Scenario: baseline 过期而能力类别新鲜 <a id="scenario-prob-hr-170-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** baseline 观测已过新鲜度阈值，但其他能力观测新鲜
- **THEN** 整体健康未知、标识 baseline 过期，不显示来自其他能力的整体线路延迟

##### Scenario: baseline 安全拒绝或配置失败 <a id="scenario-prob-hr-170-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** baseline 由于不安全 TLS、私有目标或探针构建错误失败
- **THEN** 节点显示未知/无法检测及脱敏原因，既不视为健康也不视为已证异常

##### Scenario: baseline 可判定的连通结果 <a id="scenario-prob-hr-170-4"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 新鲜 baseline 观测证明成功或有真实连通性失败证据
- **THEN** 整体健康相应为健康/异常，线路延迟只在成功 baseline 有有效耗时数据时显示；其他能力错误不覆盖此判断

#### Requirement: pool 与节点读模型同口径且兼容 [HR-171] <a id="req-prob-hr-171"></a>

系统 SHALL 使用同一健康判定规则产出节点视图和探测池统计，所有激活节点的互斥计数和 SHALL 等于总数；未知/无法检测 SHALL 计入现有未确认/未测类别而非可用或异常类别，并可从现有能力摘要解释原因；不得悄然增加不兼容的必填响应字段或改变现有订阅过滤语义。

##### Scenario: 多种观测混合聚合 <a id="scenario-prob-hr-171-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 激活节点同时包含健康 baseline、真实失败 baseline、阻断 baseline、过期 baseline、以及无 baseline
- **THEN** pool 的健康、降级、异常和未知计数与逐节点视图的相应整体状态一致且互斥；受阻节点不被统计为已证坏线路

#### Requirement: 一分钟服务端轻量状态扫描 [HR-172] <a id="req-prob-hr-172"></a>

启用周期探测时系统 SHALL 约每 1 分钟由服务端进行一次状态巡检，而不是每分钟对所有节点运行全部探测；计划的 `next_due_at` SHALL 表示下次状态扫描时间，`interval_seconds` 仍表示逐节点/类别的观测有效期。启用、重配及服务恢复 SHALL 在无需等待旧 10 分钟或整段观测有效期的前提下及时扫描。

##### Scenario: 状态未变化且观测仍新鲜 <a id="scenario-prob-hr-172-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 本轮活跃节点和有效连接参数未变化，最新观测均未到期
- **THEN** 系统不新增网络探测任务，保留下一次分钟扫描

##### Scenario: 缺失、到期或有效连接配置变化 <a id="scenario-prob-hr-172-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 某激活节点类别没有观测、已达 `interval_seconds`，或该节点的连接相关参数实质发生变化
- **THEN** 仅将相应到期类别或因连接变更失效的配置类别列为增量待测，受预算与冷却约束；显示旧观测时不得将其错误标为当前配置的有效线路结论

##### Scenario: 元数据与观测写入不自旋 <a id="scenario-prob-hr-172-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 仅显示名、来源抓取时间、普通 `updated_at` 或最新探测观测写入发生改变而连接参数未变
- **THEN** 不因该变化本身重复入队或形成反馈自旋

##### Scenario: 活跃状态切换 <a id="scenario-prob-hr-172-4"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点重新激活或从活跃变成停用
- **THEN** 重新激活的节点进入有效期/配置变化增量评估，停用节点不产生新任务且不计入活跃池

#### Requirement: 预算、去重与冷却保留 [HR-173] <a id="req-prob-hr-173"></a>

系统 SHALL 继续遵守每轮最多 512 个任务预算、失败后 5 分钟冷却、CAS 租约与在池/队列去重；被预算延后的候选 SHALL 在后续扫描中公平获得机会，手动刷新 SHALL 遵守同一边界，不得重置正常分钟时钟或暗中走全量测速。

##### Scenario: 大量到期节点并发触发 <a id="scenario-prob-hr-173-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 自动巡检与手动刷新同时到达且待测任务超过单轮预算
- **THEN** 每轮有界入池，已排队节点不重复派发，租约保持单执行者，剩余待测在后续扫描中有机会入池

#### Requirement: 手动刷新状态与全量测速明确区分 [HR-174] <a id="req-prob-hr-174"></a>

控制台 SHALL 提供可见的手动“刷新状态”操作：先重读池、节点与计划状态，再请求服务端执行与分钟巡检相同的增量判定；反馈已排队或无需检测，并继续跟踪结果。原有“全量测速” SHALL 保留且独立标识，刷新 GET 不得冒充探测，手动刷新不得改变计划 `next_due_at`。

##### Scenario: 手动刷新且有到期类别 <a id="scenario-prob-hr-174-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户点击“刷新状态”，至少一个激活节点类别到期
- **THEN** 页面重读状态并发起有界增量巡检，展示排队/测试进展，后续更新节点与池状态

##### Scenario: 手动刷新而无待测类别 <a id="scenario-prob-hr-174-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户点击“刷新状态”，所有观测对当前配置均新鲜
- **THEN** 页面明确显示无需检测，不创建全量测速或伪造测试结果，正常后台扫描仍按原计划进行

#### Requirement: 安全约束下的协议参数一致性 [HR-175] <a id="req-prob-hr-175"></a>

系统 SHALL 在探测支持的协议上使用有效的连接参数；VLESS vision/Reality 的适用参数 SHALL 与该节点既有导出连接语义一致。系统 MUST 继续拒绝不安全 TLS 校验及私有目标，不得因修复误报而放行安全边界。

##### Scenario: 有效 VLESS vision/Reality 配置 <a id="scenario-prob-hr-175-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点含有效 UUID、vision flow、Reality 身份参数和公网目标
- **THEN** 探针构建路径保留适用的 flow、TLS/Reality 身份参数，离线配置校验不因遗漏参数报凭据缺失

##### Scenario: 不安全 TLS 或私有目标 <a id="scenario-prob-hr-175-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点要求跳过证书校验，或目标 IP/解析结果触及私网策略
- **THEN** 探针拒绝连接并明确标识对应安全拒绝原因，不得将其归因为线路已坏，也不得实际向该目标出站

#### Requirement: 故障归因有证据且不泄密 [HR-176] <a id="req-prob-hr-176"></a>

系统 SHALL 将配置确实缺失、协议不支持/配置构建失败、安全策略拒绝、解析失败、超时和传输失败区分记录；传输失败 SHALL 保留可用的脱敏阶段/原因信息，不得把缺乏进一步证据的传输错误直接断定为服务端线路故障。现有观测读取方 SHALL 能依据结构化或稳定的脱敏原因识别检测能力阻断，且不能暴露凭据、私钥或敏感地址。

##### Scenario: 凭据存在但客户端构建失败 <a id="scenario-prob-hr-176-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点具有凭据但探针客户端无法创建
- **THEN** 观测标识配置/构建类失败而不是 `credentials_unavailable` 或已经确认的网络故障

##### Scenario: 真实网络传输失败 <a id="scenario-prob-hr-176-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 已通过安全与配置校验后发生传输失败或超时
- **THEN** 观测保留相应类别与安全脱敏证据，不将其伪装为凭据缺失，也不制造成功结果

#### Requirement: Baseline connectivity probe requires strict HTTP 204 empty response [HR-177] <a id="req-prob-hr-177"></a>

系统在执行 `baseline`（连通性与延迟）探测时，MUST 仅在收到 `HTTP 204 No Content` 且响应体为空（0 字节）时判定为 `available`（`reason=contract_matched`）。若响应包含明确访问限制信号（HTTP `401`、`403` 或挑战/登录拦截标记），系统 MUST 判定为 `restricted`（`reason=access_restricted`）；若响应为 `HTTP 200`（如 Captive Portal 伪装页、通用 `"OK"` 文本）或带非空响应体的 `HTTP 204`，系统 MUST 判定为 `unknown`（`reason=contract_drift`），绝不因状态码为 `200` 而误判节点连通可用。

##### Scenario: HTTP 204 空响应判定为 available <a id="scenario-prob-hr-177-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `baseline` 探针请求 `http://cp.cloudflare.com/generate_204` 收到状态码 `204` 且响应体长度为 `0`
- **THEN** 评估结果 MUST 为 `verdict=available`、`reason=contract_matched`，并记录对应的 `latency_ms` 与 `evidence_digest`

##### Scenario: HTTP 200 或非空 204 响应判定为 unknown <a id="scenario-prob-hr-177-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `baseline` 探针收到状态码 `200`（正文为 `"OK"`、空白或不含限制词的 HTML）或收到带非空正文的 `204`
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，且在两阶段探测中不得放行该节点进入第二阶段高开销探测

##### Scenario: 挑战或登录拦截响应判定为 restricted <a id="scenario-prob-hr-177-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `baseline` 探针收到状态码 `401`/`403` 或包含挑战/登录标记（如 `cf-chl`、`turnstile`、`captcha`、`/login`）的响应体
- **THEN** 评估结果 MUST 为 `verdict=restricted`、`reason=access_restricted`

#### Requirement: Geo probe validates exit IP and ISO country code [HR-178] <a id="req-prob-hr-178"></a>

系统在执行 `geo`（落地地区识别）探测时，MUST 校验响应状态码为 `HTTP 200` 且响应体能够解析出合法出口 IP 地址（IPv4 或 IPv6）与 2 位 ISO 3166-1 alpha-2 国家代码后，才可判定为 `available`（`reason=contract_matched`）。

##### Scenario: 包含合法出口 IP 与两位国家代码的响应判定为 available <a id="scenario-prob-hr-178-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `geo` 探针收到 `HTTP 200` 且响应体包含可解析的合法 IP（如 `"203.0.113.10"`）与 2 位字母国家代码（如 `"US"` 或 `"SG"`）
- **THEN** 评估结果 MUST 为 `verdict=available`、`reason=contract_matched`

##### Scenario: 缺失出口 IP 或国家代码的 200/204 响应判定为 unknown <a id="scenario-prob-hr-178-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `geo` 探针收到 `HTTP 204` 空响应、`HTTP 200` 纯文本 `"OK"`、缺失 IP 字段或国家代码非 2 位字母的 JSON/Trace 响应
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁判定为 `available`

#### Requirement: Streaming and AI probes never report available without a verifiable capability contract [HR-179] <a id="req-prob-hr-179"></a>

系统在执行 `streaming` 与 `ai` 探测时，在未配置或未验证可证明的权威业务能力契约前，MUST 绝不因为目标站点根路径/首页返回 `HTTP 200`、`HTTP 204` 空响应或任意非空正文而判定为 `available`，且 MUST 严禁内置未经证实的 Netflix/OpenAI 私有探测规则或照搬 `subs-check-pro v3.2.1` 的误判逻辑。

##### Scenario: 明确限制信号判定为 restricted <a id="scenario-prob-hr-179-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `streaming` 或 `ai` 探针收到状态码 `401`/`403`/`451` 或响应体包含挑战/登录/人机验证限制标记
- **THEN** 评估结果 MUST 为 `verdict=restricted`、`reason=access_restricted`

##### Scenario: 首页 200、空响应或普通非空正文在无业务契约证明时判定为 unknown <a id="scenario-prob-hr-179-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `streaming` 或 `ai` 探针请求默认端点收到 `HTTP 200`（无论正文为空、`"OK"` 或普通网页正文）或 `HTTP 204` 且无明确限制标记、亦无已证实的业务能力契约证明
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁返回 `available`

##### Scenario: 传输或超时故障判定为 error <a id="scenario-prob-hr-179-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `streaming` 或 `ai` 探针发生连接失败、DNS 解析错误或任务超时（`DeadlineExceeded`）
- **THEN** 评估结果 MUST 为 `verdict=error`、`reason=transport_error`

#### Requirement: Speed probe measures full payload transfer duration, rejects tiny stub responses, and enforces byte budgets [HR-181] <a id="req-prob-hr-181"></a>

系统在执行显式 Opt-In 的 `speed`（带宽测速）探测时，MUST 在完整读取有界响应体（`readBoundedResponse`）完成后再截止计时计算 `latency_ms` 与 `throughput_kbps = (BytesRead * 8) / latency_ms`。`speed` 探针 MUST 仅在收到 `HTTP 200` 且有效读取字节数满足最小负载门槛（`BytesRead >= 1024` 字节）且未超出预算上限（`<= MaxBytesPerRequest` 且 `<= MaxBytesPerRun`）时判定为 `available`；对空响应、`HTTP 204` 或 `< 1024` 字节的极短伪响应 MUST 判定为 `unknown`（`reason=contract_drift`）；对超出请求或运行字节预算的响应 MUST 保留返回 `error`（`reason=speed_budget_exceeded`）。

##### Scenario: 完整读取有效测速负载后计算耗时与吞吐率 <a id="scenario-prob-hr-181-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `speed` 探针收到 `HTTP 200` 且响应体流式传输 `>= 1024` 字节（且 `<= MaxBytesPerRequest`），并在读取响应体过程中消耗可测时间
- **THEN** 记录的 `latency_ms` MUST 包含完整响应体读取耗时，`RedactedSummary` 中的 `bytes_read` 与 `throughput_kbps` 基于完整传输耗时计算，且评估结果为 `verdict=available`、`reason=contract_matched`

##### Scenario: 拒绝空响应、204 或极短伪响应 <a id="scenario-prob-hr-181-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `speed` 探针收到 `HTTP 204`、`HTTP 200` 空响应（`0` 字节）或极短伪响应（如 `"OK"` 等 `< 1024` 字节负载）
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁判定为 `available`

##### Scenario: 超出单次请求字节预算时返回 speed_budget_exceeded 错误 <a id="scenario-prob-hr-181-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `speed` 探针读取的响应体超过 `SpeedBudget.MaxBytesPerRequest`（1 MiB）
- **THEN** 评估结果 MUST 为 `verdict=error`、`reason=speed_budget_exceeded`

#### Requirement: Same-run per-node probe client reuse and leak-free lifecycle cleanup [HR-182] <a id="req-prob-hr-182"></a>

系统在单次 `ProbeRun` 执行期间，针对同一节点（`node.LogicalID`）跨多个探测维度（含单阶段并发提交与两阶段 Phase 1 `baseline` -> Phase 2 扩展维度）MUST 最多调用一次 `NodeDialer` 创建客户端实例并复用，且 MUST 在该节点于本次 Run 的全部已调度任务及其 `OnComplete` 回调完成后精确调用一次 `cleanup()` 释放资源。无论 Run 正常完成、Phase 1 淘汰、上下文取消、截止时间超时、任务提交失败或发生致命拨号错误，系统 MUST 保证所有已创建的节点客户端均被关闭且不发生泄漏，同时完整保持手动探测插队队首（`EnqueueManualPreemptFront`）与定时探测去重入池（`EnqueuePeriodicDedupe`）的队列语义。

##### Scenario: 同 Run 多探测维度单次创建并在全部回调完成后关闭客户端 <a id="scenario-prob-hr-182-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 单次 `ProbeRun` 对同一节点执行多个探测维度（如 `baseline` + `geo` + `streaming` + `ai` + `speed`，或不含 `baseline` 的多维组合）
- **THEN** 系统对该节点仅调用 `1` 次 `NodeDialer`，所有探测维度共享该客户端完成请求，且 `cleanup()` 在该节点最后一个任务的 `OnComplete` 回调完成后精确执行 `1` 次

##### Scenario: 两阶段探测中 baseline 未通过的节点不进入第二阶段且立即安全关闭客户端 <a id="scenario-prob-hr-182-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 两阶段 `ProbeRun` 中某节点的 Phase 1 `baseline` 结论非 `available`（如 `unknown`、`restricted` 或 `error`）
- **THEN** 系统跳过该节点的 Phase 2 维度提交，并在该节点 Phase 1 任务回调收敛后精确调用 `1` 次 `cleanup()`

##### Scenario: Run 取消、超时或异常退出时不泄漏节点客户端 <a id="scenario-prob-hr-182-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `ProbeRun` 在执行过程中遭遇 `context.Canceled`、`context.DeadlineExceeded`、队列提交错误或致命拨号错误
- **THEN** 系统等待在途任务与 `OnComplete` 回调收敛后，对本次 Run 中已创建的全部节点客户端各精确执行 `1` 次 `cleanup()`

##### Scenario: 保持队列手动插队与定时去重语义不变 <a id="scenario-prob-hr-182-4"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 手动触发探测或由 `system:periodic-probe` 触发定时探测
- **THEN** 任务仍分别以 `EnqueueManualPreemptFront` 与 `EnqueuePeriodicDedupe` 模式提交至调度器，手动插队优先与定时池内去重跳过行为保持不变

#### Requirement: Security and signature invariants are preserved during probing [HR-183] <a id="req-prob-hr-183"></a>

系统在收紧探针契约与实现客户端复用时，MUST 完整保留 `NewSafeNodeDialer` 的目标服务器 DNS 解析校验与 IP 固定（Server DNS Pinning）、SSRF 非公网 IP 拦截、TLS 安全校验、HTTP 重定向禁止（`http.ErrUseLastResponse`），以及 `domain.ComputeProbeEvidenceDigest` 的证据摘要签名计算规则与既有接口签名。

##### Scenario: Server DNS Pinning、SSRF 拦截、禁止重定向与证据签名保持生效 <a id="scenario-prob-hr-183-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 使用 `NewSafeNodeDialer` 与 `DefaultRunner` 执行节点探测
- **THEN** 域名节点仍预先解析校验公网 IP 并固定 `cfg.Server`（同时保留 TLS `SNI`），私网/环回地址仍被拒绝，HTTP `3xx` 响应仍返回 `http.ErrUseLastResponse` 不自动跟随，且生成的 `ProbeObservation.EvidenceDigest` 仍严格匹配 `ComputeProbeEvidenceDigest(runID, nodeLogicalID, profileVersion, verdict, statusCode, reason)`

#### Requirement: SQLite latest probe observation query selects the exact latest record per node and kind inside the database [HR-184] <a id="req-prob-hr-184"></a>

SQLite 观测仓储的 `ListLatestByNodes(ctx, nodeLogicalIDs, kinds)` 方法 MUST 在 SQLite 数据库内部按 `(node_logical_id, kind)` 分区并按 `observed_at DESC, id DESC` 排序，仅筛选并向 Go 层返回每个 `(node_logical_id, kind)` 的唯一最新观测记录（`rn = 1`），严禁将匹配节点的全部历史观测行拉取到 Go 内存后再去重。

##### Scenario: 多轮历史观测下仅在数据库内选出每个节点与维度的最新记录 <a id="scenario-prob-hr-184-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 数据库中多个节点在各个 `ProbeKind` 下分别存有多条不同 `observed_at` 的历史观测记录，调用 `ListLatestByNodes` 查询指定节点与维度集合
- **THEN** 数据库查询仅返回每个 `(node_logical_id, kind)` 排序最前的单条最新记录，返回结果集与全量历史行内存去重结果完全一致

##### Scenario: 相同 observed_at 秒级时间戳下按 id DESC 确定性决胜 <a id="scenario-prob-hr-184-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 同一 `(node_logical_id, kind)` 存在多条 `observed_at` 时间戳完全相同的观测记录
- **THEN** `ListLatestByNodes` MUST 按 `ORDER BY observed_at DESC, id DESC` 稳定返回 `id` 字典序最大的那条观测记录

##### Scenario: 离线基准与回归测试验证大历史数据下查询开销受控 <a id="scenario-prob-hr-184-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 在离线 SQLite 测试数据库中注入多节点、多维度、深历史观测数据并运行回归单测与 `go test -bench` 基准测试
- **THEN** 所有功能回归单测与基准测试全部离线通过（exit 0），且 `ListLatestByNodes` 扫描返回给 Go 层的行数恒等于有效 `(node_logical_id, kind)` 组合数而非历史总行数

#### Requirement: 发布预览缺少活跃修订时可理解并可恢复 [HR-185] <a id="req-prob-hr-185"></a>

发布中心 SHALL 保留服务端 `409 no_active_revision`、提供配置入口和真实重试；真实预览成功前 MUST 禁用发布、下载与复制。401/403/500 与网络错误 MUST 区分，不得假装成功。

##### Scenario: 无活跃修订后恢复 <a id="scenario-prob-hr-185-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 隔离实例无活跃修订，用户打开发布中心、建立有效修订并重试
- **THEN** 初始请求返回 409 且显示前置条件与入口；修订后仅真实预览成功才开放发布操作

##### Scenario: 鉴权或服务器失败 <a id="scenario-prob-hr-185-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 预览遇 401、403、500 或网络故障
- **THEN** 显示对应恢复反馈，不误报缺少修订

#### Requirement: 主题偏好菜单不被布局裁剪 [HR-186] <a id="req-prob-hr-186"></a>

主题选择 SHALL 在移动及桌面视口可见、可聚焦、可经指针与键盘操作；展开时 MUST 不被遮挡，切换保留当前路由与持久化偏好。

##### Scenario: 本地多视口展开与选择 <a id="scenario-prob-hr-186-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 在隔离浏览器 392×872、375×667、1280×800 打开主题菜单并选择 light/dark/system
- **THEN** 所有选项可触达、无不可恢复横向溢出，主题生效、路由不变且重载后偏好仍有效

#### Requirement: 移动工作区页面和导航可用且不靠裁剪掩盖 [HR-187] <a id="req-prob-hr-187"></a>

Publications、Dashboard、Policy Admission Rules、Subscriptions 和 Settings SHALL 在 375×667、392×872 与 1280×800 真实浏览器视口中保持主内容、操作和浮层可用；关键操作 MUST 完整位于 viewport 之内、无互相遮挡，主区域 MUST 无横向内容溢出。仅允许有明确内容边界且末项确实可达的局部滚动；设置主容器 `overflow-x-hidden` 却裁掉控件 MUST NOT 算合格。导航 SHALL 在移动提供 Dashboard 及其余全部现有七条业务路由的可辨、可点击路径，dock 项之间有明显可辨间隔且末项不被遮住；Settings token 状态每次只表达一次状态，不得出现重复 `Active Active`。

##### Scenario: Publications 头部操作与全部 target <a id="scenario-prob-hr-187-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 在 409 状态和预览成功状态下于 375×667、392×872 分别打开 Publications，遍历 `COMPILER_TARGETS` 的五个项目并操作下载/创建
- **THEN** Create Publication 和恢复按钮完整可见可点击、禁用/成功状态不变；包括 Quantumult X 在内每个项目可由触摸/键盘发现并到达，选择后真实预览或明确 409，main 不因目标行总宽而增宽；任一局部滚动可把末项完全滚入视口

##### Scenario: Dashboard badges、Policy rules 与 Subscriptions 菜单 <a id="scenario-prob-hr-187-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 渲染 Health Telemetry 的长徽章、Admission Rules 的长表达式/ID 和带状态徽章/动作菜单的 Subscription，再打开菜单并执行操作
- **THEN** 两 badge 文字在自身边界内且不穿刺边框、规则与 subscription 卡片不强迫 main 变宽、操作入口和菜单完整在 viewport 内，可点击/键盘可操作，长内容仅在受控内容区截断/滚动而不丢操作

##### Scenario: 七路由移动可达、dock 分隔与状态文案 <a id="scenario-prob-hr-187-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 在 375×667/392×872 的移动 dock 从任意路由返回 Dashboard，再访问其余六路由，分别加载有/无 token 的 Settings 且切换支持语言
- **THEN** 首页及每一路由有明确可操作入口，dock 可见目标不粘连、导航不盖住页面末尾控件，设置状态单次准确表示且无 `Active Active`，主题菜单及当前路由保持原有语义

#### Requirement: 无可用凭据时安全失败而不误报成功 [HR-188] <a id="req-prob-hr-188"></a>

探针 SHALL 仅凭经确认完整节点配置经该节点隔离 outbound 请求受控目标；MUST NOT 使用占位服务器/口令、宿主直连、系统代理或其它节点出口。仅有旧摘要、缺密钥、无可执行目标时 MUST 同步拒绝或到达明确失败终态，不得生成 available 或永久 queued/running。运行结果与节点可用性 SHALL 区分。

##### Scenario: 旧摘要、无节点或缺密钥 <a id="scenario-prob-hr-188-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 选择缺少可解析凭据、缺密钥或有效目标为空的节点
- **THEN** 无占位/宿主直连出站，返回脱敏安全失败或同步拒绝，无 available

##### Scenario: 部分提交失败、取消与截止时间 <a id="scenario-prob-hr-188-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 队列容量不足、取消、服务关闭或期限到达
- **THEN** 已接纳任务有界回收并得到 failed/cancelled/expired 终态，不被晚到 succeed 覆盖

#### Requirement: 真实探针凭据与库存版本一致且保密 [HR-189] <a id="req-prob-hr-189"></a>

系统 SHALL 仅通过获授权源输入获得完整连接参数；持久化认证密文 SHALL 受独立受管密钥保护，并原子绑定节点当前 credential_version、精确 (logical_id,version,protocol) 密文及有效来源。版本仅随规范化完整连接配置变化而单调增长；源全文摘要、顺序或显示名单独变化 MUST NOT 升级版本；撤销后版本不得重用。授权来源冲突 MUST fail-closed。旧摘要和无版本旧行 MUST NOT 视为可用密文；API/日志/审计不得泄漏口令、源 URL token、密钥或响应正文。

##### Scenario: 幂等刷新、变更、旧行及冲突 <a id="scenario-prob-hr-189-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用隔离密钥导入授权 fixture 源、调整排序/显示名称、修改连接配置、撤销重导或遇来源冲突
- **THEN** 相同配置版本不变、不同配置递增且不重用；旧行/冲突不得拨号，事务失败不半更新，数据库无明文

#### Requirement: 接纳的运行冻结精确授权快照 [HR-190] <a id="req-prob-hr-190"></a>

每个获接纳 run SHALL 固定节点标识、protocol、credential_version 及来源有效性非秘密快照；执行时 MUST 精确取此版本并核验配置身份/AAD，不得升级 latest。拨号前及在途 MUST 检查版本和授权仍有效；撤销、重配、取消停止新连接并有界关闭在途；重启无法恢复快照的旧任务 SHALL expired。

##### Scenario: 排队后重配或撤销 <a id="scenario-prob-hr-190-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 排队后节点更新/失去来源或运行中撤销
- **THEN** worker 不用新版本替代、不使旧版本结果成为当前 available，资源及时关闭并脱敏失败

#### Requirement: 节点入口与目标实际拨号均阻断 SSRF 并保留域名身份 [HR-191] <a id="req-prob-hr-191"></a>

生产探针 SHALL 仅请求服务端允许的 scheme/host/port，不接收客户端动态 URL，不随意跳转。节点和目标每次实际拨号 IP MUST 按现有策略禁止内网、环回、metadata、RFC6598、IPv4 映射 IPv6 等；节点域名 MUST 在实际连接处固定为已解析验证公网 IP，不能校验后再次解析连接。`ss`、`trojan`、`vless`、`vmess`、`hysteria2`、`tuic` 六协议域名入口 MUST 经真实协议 inbound→目标 HTTP 204 实测成功才声称完成；开启 TLS 者 MUST 依显式 SNI 或原域名校验证书；ws/http/httpupgrade/grpc 适用组合 MUST 保持显式 Host/authority 或原域名且路由正确。SS 验证加密代理握手但不要求 TLS；Hy2/TUIC 实测 QUIC/TLS/UDP。测试 CA 和映射 MUST NOT 进入生产默认配置；未通过协议/传输 MUST fail-closed 且不得勾完成。

##### Scenario: 不安全入口、双解析改绑及目标重定向 <a id="scenario-prob-hr-191-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点/目标为 RFC6598/metadata/私网、首次公网后重绑私网或目标跳内网
- **THEN** 拒绝不安全连接，不发凭据，无 available

##### Scenario: 首个最小真实协议闭环 <a id="scenario-prob-hr-191-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 受控 resolver 返回验证公网节点与目标 IP，Trojan/TLS/TCP 凭据通过测试专属映射到 sing-box inbound 经代理请求本地 HTTP 204
- **THEN** 可观察公网 IP、TCP accept、受测试 CA 信任的 SAN/TLS/SNI、正确密码握手、目标命中和 204；配置快照不算通过

##### Scenario: 六协议及传输矩阵完成 <a id="scenario-prob-hr-191-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 同 fixture 分别提供 ss、trojan、vless、vmess、hysteria2、tuic 域名凭据并测试声明支持的 ws/http/httpupgrade/grpc 组合
- **THEN** 每协议真实鉴权/握手、验证节点 IP 的 TCP/UDP socket、目标 204 和逐项 TLS/SNI/Host 证据；不支持组合安全拒绝、不伪称支持，任一失败行不汇总 PASS

##### Scenario: 并发矩阵不以竞态或偶发 EOF 宣告完成 <a id="scenario-prob-hr-191-4"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 六协议与适用传输在 race 检测重复运行，包含双向 HTTP2/gRPC 及 HTTPUpgrade
- **THEN** 每行真实 204/身份/授权 socket 且无 data race/偶发 EOF；服务端等请求 body 后回复时客户端 MUST 先写再读、不死锁；失败保持未完成，不跳过或重试到绿

##### Scenario: 应用层阻断与协议鉴权负例 <a id="scenario-prob-hr-191-5"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 应用层解析遇混合公私/重绑/非法节点或目标/内网重定向/失效凭据/取消/错协议密码/TLS 错 SAN 或 CA/不支持独立 gRPC Host/QUIC UDP 端口异常
- **THEN** 拒绝路径无未授权 socket/available/凭据泄漏；合法节点鉴权失败不访问目标，成功显式 SNI/Host 保持真实域名身份；测试专属映射不得进入生产

##### Scenario: 显式身份与错误证书 <a id="scenario-prob-hr-191-6"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 显式指定 SNI/Host 或 inbound 给错证书/密钥/私网目标
- **THEN** 显式身份不被默认覆盖，错证书/凭据/目标失败，不以 SkipCertVerify 或私网豁免求通过

#### Requirement: 探针预算与跨运行公平 [HR-192] <a id="req-prob-hr-192"></a>

默认 SHALL 低流量测活，昂贵阶段按需选择；MUST 限制节点×profile 总任务、队列、全局和每 RunID 并发、连接/运行时间、响应/测速字节；超额拒绝并释放容量。持续提交的大 run MUST NOT 使已接纳小 run 饥饿。缓存仅可用于未过期且匹配版本/授权来源/profile 的成功结果。

##### Scenario: 两个竞争 run 的执行进展与取消 <a id="scenario-prob-hr-192-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 大 run 持续入队且部分阻塞，小 run 接纳后取消大 run
- **THEN** 小 run 在大 run 全结束前前进，峰值受限，取消有界释放，无 FIFO 队头饥饿

##### Scenario: 超预算、禁用昂贵阶段与缓存失效 <a id="scenario-prob-hr-192-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 超额、未选昂贵阶段或版本/来源失效
- **THEN** 明确失败、无 available，昂贵阶段不出站，旧缓存不作为新证据

#### Requirement: 关闭与回调完成可观察且不虚报干净退出 [HR-193] <a id="req-prob-hr-193"></a>

队列 SHALL 原子拒绝关闭后新接纳，已接纳任务精确调用一次 completion callback；Close SHALL 能在 Execute/OnComplete 中调用且不自等待。兼容 Wait 不证明 callback 完成；外部协调者 SHALL 停接纳并有界 drain 等已接纳 callback/执行/调度器退出。DB 释放前 MUST 证明写入结束；超时 MUST 报未完成而非 clean。隔离 HTTP 服务真实接收任务后停服的成功/超时双分支不可被只 GET、hook Submit、queue helper 替代。clean MUST 晚于真实 DB.Close 成功；Close 失败非零退出且无 clean；超时不得提前 Close，进程退出后能重开 DB 不代替关闭路径观察。

##### Scenario: 最后一个回调晚于 Wait 返回 <a id="scenario-prob-hr-193-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** Execute 结束 OnComplete 阻塞，协调者 Wait 后调用有界 drain
- **THEN** Wait 可先返但不宣告完成；放行后 drain 才成功且精确一次 completion

##### Scenario: 关停、提交、取消与重入交错 <a id="scenario-prob-hr-193-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** Submit/Close、CancelRun/Close 竞态、回调中重入或执行不响应取消
- **THEN** 明确线性化点、无重复/遗漏/自等待死锁；截止未完成可诊断失败，不假报 clean

##### Scenario: 认证 HTTP route 真实提交且不得有测试后门 <a id="scenario-prob-hr-193-3"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 隔离子进程以真实 Bearer + 唯一 Idempotency-Key POST `/api/v1/probes/runs`，另试未经授权及重复 key
- **THEN** 201 run_id 与 SQLite 和 Runner/queue 同一 task 匹配，拒绝未认证/重复；仅测试内部装配 Runner，不从生产 HTTP/flag/env 开测试后门、不直接 hook Submit

##### Scenario: 带在途 callback 的进程级正常停服 <a id="scenario-prob-hr-193-4"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 认证 POST 创建任务，同步屏障确认 callback 阻塞，SIGTERM/drain 后放行
- **THEN** 同步事件+DB 读回证明 callback 持久化并返回→Drain 成功→真实 DB.Close 成功→clean→cmd.Wait exit0，不用 sleep/轮询代替因果

##### Scenario: 带在途 callback 的进程级超时停服 <a id="scenario-prob-hr-193-5"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 同路径不合作 callback 阻塞至自然 drain 截止
- **THEN** timeout/close-skipped/失败事件及实际非零 ExitCode/pipe EOF，事件中无 DB.Close attempt、持久化或 clean；外部 watchdog Kill 不算合格

#### Requirement: 本地隔离成品交互与多模态视觉验收可复现 [HR-194] <a id="req-prob-hr-194"></a>

终态验收 SHALL 在本轮重新构建的非生产隔离 URL、临时 DB 上检验 health/auth、发布恢复、七路由、多视口主题及布局和探针路径。375×667、392×872、1280×800 浏览器 SHALL 提供主区宽度、关键 action/menu 边界、badges 内容高度、全部 targets/移动入口到达、截断与局部滚动的真实几何及行为证据；原始 PNG MUST 由具备 read 能力的独立 critic 直接检视，结合 bash 只读浏览器/DOM 量测。完整交付 MUST 同时取得 `CRITIC_FLOW_VERDICT: PASSED` 与 `CRITIC_VISUAL_VERDICT: PASSED`，旧 37 PNG 的 VISUAL FAILED、截图清单或只做 jsdom class 检查不得替代。

##### Scenario: 最新隔离预览重审 <a id="scenario-prob-hr-194-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 准备者提供最新构建隔离 URL/health/auth、脱敏 trace/几何 JSON、三视口全页及菜单/targets/badge/dock 原始 PNG 与网络 fixture，critic 直读原图并操作/量测
- **THEN** `main.scrollWidth <= main.clientWidth`、关键动作/菜单 rect 均在 viewport、badge.scrollHeight <= clientHeight、五 targets 包括末项实际可达、dock 路由可辨且 Settings 不重复状态，并取得新版本 FLOW/VISUAL 双 PASSED；不触生产端口/卷

#### Requirement: 进程内代理适配与连接参数完整保留 [HR-250] <a id="req-prob-hr-250"></a>

系统 SHALL 在探针出站层使用进程内 Mihomo `adapter.ParseProxy` 构造单节点代理出站，严格遵循节点原生连接配置，正常支持 `skip-cert-verify`（跳过 TLS 证书验证）、Hysteria2 `ports`（端口跳跃）以及 TUIC `disable_sni` 等常用协议参数。系统 MUST 废除针对自签名证书或特定协议选项的人为主观拦截，以代理内核真实协商语义为准。

##### Scenario: 包含 skip-cert-verify 的节点正常建立出站 <a id="scenario-prob-hr-250-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 探针接收到标记有 `skip-cert-verify: true` 的 Trojan 或 VMess 节点
- **THEN** 系统构建对应出站代理实例并在 TLS 握手中遵循该设置，不抛出 `unsafe_tls_rejected` 异常

##### Scenario: 包含端口跳跃配置的 Hysteria2 节点正常拨号 <a id="scenario-prob-hr-250-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 探针调度包含 `ports` 端口跳跃范围参数的 Hysteria2 节点
- **THEN** 代理适配器如实将端口范围透传至底层协议栈，不拒绝创建客户端

#### Requirement: 宿主代理严格隔离与远端域名代理解析 [HR-251] <a id="req-prob-hr-251"></a>

系统构建的每个节点独立 HTTP Client MUST 显式将底层 Transport 的 `Proxy` 设置为 `nil`，绝不允许使用或继承宿主机运行环境中的 `HTTP_PROXY`、`HTTPS_PROXY` 或 `ALL_PROXY` 环境变量。探针发起 HTTP 请求时，系统 MUST 将目标主机名直接装载入目标 Metadata (`Host`) 中并通过节点隧道向远端发起拨号，严禁在本地宿主机执行直接 DNS 解析，防止本地 DNS 污染及真实出口偏差。

##### Scenario: 宿主机配置系统代理时探测请求不被穿透 <a id="scenario-prob-hr-251-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 宿主操作系统环境中存在 `HTTP_PROXY=http://127.0.0.1:7890` 环境变量
- **THEN** 探针发出的所有探测流量严格仅通过被测节点出站隧道建立，不经过宿主机代理

##### Scenario: 目标域名由远端节点代理解析 <a id="scenario-prob-hr-251-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 探针对 `https://api.openai.com` 或 `https://www.netflix.com` 发起探测
- **THEN** 本地操作系统不执行针对目标域名的 A/AAAA 记录解析，域名直接由代理链路远端协商解析

#### Requirement: 独立客户端生命周期管理与挂起看门狗 [HR-252] <a id="req-prob-hr-252"></a>

系统 SHALL 为每个待测节点建立独立的客户端生命周期上下文。当节点生命周期结束或 Run 取消时，系统 MUST 触发 `Close()` 并释放相关 Transport 空闲连接与套接字句柄。系统 MUST 配备看门狗保护机制，防止底层网络握手无限期挂起导致的协程与句柄泄漏。

##### Scenario: 探测超时触发看门狗强制释放 <a id="scenario-prob-hr-252-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 某个节点在传输层握手陷入非响应阻塞超过预设生命周期阈值
- **THEN** 看门狗协程主动触发底层代理 Close 并释放连接，主调度循环不被死锁

#### Requirement: 三阶段自适应漏斗执行管线 [HR-253] <a id="req-prob-hr-253"></a>

系统探针执行引擎 SHALL 按 `Alive (测活)` -> `Media/AI (深度能力)` -> `Speed (可选带宽测速)` 的三阶段管线执行探测任务。第一阶段（Stage 1: Alive）使用轻量级请求（如 Cloudflare 204 或快速 Trace 接口）在短超时（默认 2~3 秒）内进行存活初筛；若 Stage 1 判定节点不可用（网络失败、超时或握手拒绝），系统 MUST 立即记录失败结论，终止该节点的后续流程，严禁为坏死节点分配 Stage 2 与 Stage 3 的检测资源。

##### Scenario: 离线节点在第一阶段秒级淘汰 <a id="scenario-prob-hr-253-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 某个节点在 Stage 1 拨号或 HTTP 204 探针超时
- **THEN** 系统立即落库失败观测记录，该节点不进入流媒体与测速阶段

##### Scenario: 存活节点按序晋级至第二阶段 <a id="scenario-prob-hr-253-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点在 Stage 1 成功完成握手并获取有效 204/200 响应
- **THEN** 系统将该节点移交至 Stage 2 媒体与 AI 任务队列，复用该节点既有 Client 发起深度能力探测

#### Requirement: 分阶段独立并发控制与调度保护 [HR-254] <a id="req-prob-hr-254"></a>

系统 SHALL 为三阶段设定独立可配置的并发度参数（如测活阶段支持 50~100 并发，媒体检测支持 20~50 并发，测速阶段支持 4~16 并发）。系统 MUST 废除原有的全局 16~32 硬编码限制，使用非阻塞的 Worker 协程池在阶段之间通过管道流转任务。

##### Scenario: 测活阶段高并发吞吐 <a id="scenario-prob-hr-254-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 批量调度 200 个待测节点
- **THEN** 调度器以测活阶段设定的高并发通道快速消费节点，不阻塞其他业务任务

#### Requirement: 流量硬上限截断网络测速 [HR-255] <a id="req-prob-hr-255"></a>

当启用带宽测速阶段时，系统 MUST 在底层读取流上包装流量限制器（如 `networkLimitedReader`），设定单节点样本读取硬上限（默认不超过 5MB~10MB）与严格超时（默认不超过 10 秒）。一旦读取字节数达到上限，读取器 MUST 立即返回 EOF 并关闭连接，以实际消耗字节与耗时精确计算吞吐带宽，严禁因测速造成节点流量过度消耗。

##### Scenario: 达到流量上限时立即截断并产出带宽 <a id="scenario-prob-hr-255-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 测速节点高速下载文件且累计传输达到预设的 5MB 上限
- **THEN** 读取器主动终止网络连接并按传输字节数与毫秒耗时计算 Mbps 带宽，不再继续下载

#### Requirement: OpenAI 双轨分级与地区识别 [HR-256] <a id="req-prob-hr-256"></a>

系统探针对 OpenAI / ChatGPT 的可用性探测 SHALL 采用双轨机制：轨 1 请求 Web 端 `https://api.openai.com/compliance/cookie_requirements`（判断响应是否排除 `unsupported_country`）；轨 2 携带移动端请求头请求 `https://ios.chat.openai.com`（判断响应是否排除 `unsupported_country` 及 `vpn` 封禁信号）。
- 双轨全部通过时，系统判定为全平台可用（`Full = true`，标记为 `GPT⁺`）；
- 仅单轨通过时，系统判定为仅网页端可用（`Web = true`，标记为 `GPT`）；
- 两轨均失败时，系统判定为不可用。
系统 SHALL 进一步通过 Trace 提取落地 ISO 国家代码（如 `US`, `JP`）。

##### Scenario: 客户端网关通过判定为满血可用 <a id="scenario-prob-hr-256-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点的双轨请求均未返回地域不支持与封禁信号
- **THEN** 探测结果输出 `openai` 能力状态为 `Full` 并附带检测到的国家代码

##### Scenario: 网页端通过但移动客户端拦截 <a id="scenario-prob-hr-256-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点的轨 1 正常通过，但轨 2 返回 vpn 拦截关键字
- **THEN** 探测结果输出 `openai` 为 `Web` 可用，标记为 `GPT` 降级可用

#### Requirement: Netflix 细粒度解锁分级与封禁识别 [HR-257] <a id="req-prob-hr-257"></a>

系统对 Netflix 的探测 SHALL 区分全解锁、仅自制剧与 IP 封禁三类细粒度状态。系统 SHALL 首先请求 Fast.com CDN 测速接口；若 Fast 接口返回 403，直接判定为 `Banned = true`（被 Netflix 全局风控封锁）；若返回有效国家，判定为全解锁并提取区域。在回退模式下，系统结合非自制剧 Title (81280792) 与自制剧 Title (70143836) 进行状态码断言：
- 非自制剧返回 200/301：判定为全解锁（`Full = true`）；
- 非自制剧 404 但自制剧 200/301：判定为仅自制剧（`OriginalsOnly = true`）；
- 任一 Title 收到 403 明确封锁：判定为封禁（`Banned = true`）。
系统 MUST 坚决杜绝把单一 403 或通用 200 简单笼统等同于“解锁”或“错误”。

##### Scenario: 仅自制剧解锁正确分类 <a id="scenario-prob-hr-257-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 探测非自制剧返回 404 但自制剧返回 200
- **THEN** 探测结果输出 `netflix` 细粒度分类为 `OriginalsOnly`，不虚报全解锁

##### Scenario: Netflix 明确 403 封锁识别 <a id="scenario-prob-hr-257-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** Fast 接口或 Title 请求返回 403 Forbidden
- **THEN** 探测结果输出 `netflix` 细粒度分类为 `Banned`

#### Requirement: YouTube 地区码提取与送中严格识别 [HR-258] <a id="req-prob-hr-258"></a>

系统对 YouTube 的探测 SHALL 请求 `https://www.youtube.com/premium` 页面并提取页面中的 `INNERTUBE_CONTEXT_GL` 区域标签。若响应中包含 `www.google.cn` 或重定向送中特征，系统 MUST 明确输出 `CN` 送中状态；若响应包含 `Premium is not available in your country`，系统 MUST 判定为 Premium 不可用；只有在 HTTP 状态码为 2xx 且包含正向会员或广告关键词时，系统方可认定为解锁并输出大写二字国家码。

##### Scenario: 识别被重定向送中 <a id="scenario-prob-hr-258-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 页面返回内容包含 `www.google.cn`
- **THEN** 结果标记为送中 `CN`

##### Scenario: 正常商业解锁提取国家 <a id="scenario-prob-hr-258-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 状态码为 200 且匹配正向会员标识及区域码 `SG`
- **THEN** 结果标记为 YouTube 解锁，区域为 `SG`

#### Requirement: Disney+、Claude 与 IP 风险真实检测 [HR-259] <a id="req-prob-hr-259"></a>

系统 SHALL 支持 Disney+（通过 BAMGrid 认证断言接口判断 `Unlocked`、`Soon` 或 `Banned`）、Claude（通过 Trace 提取地区码并核对受制裁国家黑名单）以及 Scamalytics IP 欺诈风险评分检测，所有支持的平台均执行真实 HTTP 探测并输出客观结论，严禁以 `contract_drift` 进行内部假报错拦截。

##### Scenario: Claude 封禁地区识别 <a id="scenario-prob-hr-259-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点 Trace 显示出口地区为 `CN`、`HK` 或 `RU`
- **THEN** Claude 检测判定为不可用

##### Scenario: Scamalytics 风险分提取 <a id="scenario-prob-hr-259-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 请求 Scamalytics 查询接口返回 200 且包含欺诈分析数值
- **THEN** 系统提取分数百分比并写入观测摘要

#### Requirement: 冻结共享能力模型与多平台字典契约 [HR-260] <a id="req-prob-hr-260"></a>

系统 SHALL 在节点能力状态 `CapabilityStatus`（即 `NodeCapabilityView`）中维持核心枚举与字段，并扩展标准化细粒度多平台契约：
- 现有 `ProbeKind` 保持 `baseline` | `geo` | `streaming` | `ai` | `speed` | `ip_risk`；
- 现有 `verdict` 保持 `available` | `restricted` | `unknown` | `error` | `stale`；
- `CapabilityStatus` 保留现有字段 `verdict`、`latency_ms`、`observed_at`、`summary`、`stale`，并新增可选扩展字段：
  - `region` (string): 出口地区 ISO 二字码；
  - `sub_tier` (string): 细分层级，严格限定为 `'full'` | `'web'` | `'app'` | `'originals'` | `'banned'` | `'unlocked'` | `'soon'`；
  - `throughput` (number): 测速吞吐带宽，单位 KB/s（1024 bytes/s）；
  - `risk_score` (string): IP 风险分及等级描述；
  - `platforms` (Record<string, PlatformCapability>): 面向 `streaming` 与 `ai` 维度的真实平台字典，键至少包含 `openai`、`claude`、`netflix`、`youtube`、`disney`（支持 `gemini`），无嵌套 platforms。
- `PlatformCapability` 结构定义为：
  - `verdict`: `available` | `restricted` | `unknown` | `error` | `stale`；
  - `latency_ms` (number): 缺测为 -1，不将旧观测带入新结果；
  - `observed_at` (RFC3339 string);
  - `summary` (string);
  - `region` (string, 未知地区省略，严禁为 AI 探测伪造地理位置);
  - `sub_tier` (string, 枚举值同上；OpenAI 仅 App 通过时为 `'app'`，不得硬归为 `'web'`；Disney `'soon'` 符合上游未开放语义，不得当作已解锁);
  - `throughput` (number);
  - `risk_score` (string);
  - `reason` (string).
- 组级 `verdict` 聚合裁决规则：
  - 组内任一平台实际可用（`available`）时，组 verdict 判定为 `available`；
  - 组内所有平台皆明确受限（`restricted`）时，组 verdict 判定为 `restricted`；
  - 组内所有平台全部网络或协议错误（`error`）时，组 verdict 判定为 `error`；
  - 否则判定为 `unknown`；前端与业务细分必须以 `platforms` 字典为准。

##### Scenario: AI 平台多轨结果与组判决汇聚 <a id="scenario-prob-hr-260-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点的 `openai` 为 full (US) 可用，而 `claude` 受限封锁
- **THEN** 节点的 `ai` 能力项包含 `platforms.openai` 与 `platforms.claude`，且组 `verdict` 为 `available`

##### Scenario: Disney 即将上线状态语义保留 <a id="scenario-prob-hr-260-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点的 Disney+ 探测返回地区即将上线 (`soon`)
- **THEN** 平台的 `sub_tier` 记录为 `soon`，`verdict` 记录为 `restricted`，不得虚报为 `available`

#### Requirement: 结构化多平台能力结果持久化存储 [HR-261] <a id="req-prob-hr-261"></a>

系统 SHALL 在 `probe_observations` 中以 JSON 结构（`evidence_data`）序列化存储多平台细粒度能力检测结果（包括但不限于 `openai`、`netflix`、`youtube`、`disney`、`claude`、`gemini`、`ip_risk`、`speed`），并在记录中保留探测发生时的 `connection_revision`、真实网络延迟与可审计证据摘要，实施最小必要迁移，打通从 SQLite 存储到 API 响应与节点详情的全调用链路。

##### Scenario: 完整结构化结果落库 <a id="scenario-prob-hr-261-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点的 Stage 2 媒体检测完成
- **THEN** 系统将包含细粒度平台判定与区域码的观测结果原子写入数据库，不遗失字段

#### Requirement: 节点详情与工作台 API 统一透传 [HR-262] <a id="req-prob-hr-262"></a>

管理 API（包括 `/api/v1/nodes` 列表、`/api/v1/nodes/{id}` 详情及 `/api/v1/probes/runs/*`）SHALL 在响应中统一透传节点的 `capabilities` 字典，每一项能力均包含标准判决（`verdict`）、延迟（`latency_ms`）、时间戳（`observed_at`）以及平台扩展详情（如 `region`、`sub_tier`、`throughput`、`risk_score` 与 `platforms` 字典），杜绝前后端认知割裂。

##### Scenario: 查询节点详情包含多维度解锁数据 <a id="scenario-prob-hr-262-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 客户端请求 `GET /api/v1/nodes/{logical_id}`
- **THEN** 返回的 JSON 包含完整的 `capabilities` 字典，呈现各平台细粒度状态

#### Requirement: 前端 WebUI 实时状态与平台徽标展示 [HR-263] <a id="req-prob-hr-263"></a>

前端工作台 SHALL 更新探针运行卡片与节点台账列表，以语义化的彩色徽章（Badge）和平台图标展示真实检测结果（如 `GPT⁺ (US)`、`NF (HK)`、`YT (SG)`、`Claude (JP)` 等），对封禁、不可用及未测速状态给予明确视觉区分，杜绝假未检测或虚假占位。

##### Scenario: 前端如实展示 GPT 满血徽标与地区 <a id="scenario-prob-hr-263-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点的 `openai` 能力返回 `Full` 且国家为 `US`
- **THEN** 前端在节点卡片上渲染绿色的 `GPT⁺ (US)` 标签

### Explicitly Pending Historical Stubs

#### Requirement: 无凭据版本门禁的安全探测与两级节点筛选 [HR-056] <a id="req-prob-hr-056"></a>

安全节点拨号器 `SafeNodeDialer` SHALL 在应用启动时无条件挂载，并直接从 `domain.Node` 读取明文 `Server`、`Port` 与 `Credentials` 建立临时 sing-box 探测客户端，继续强制公网 IP 校验、域名解析公网 IP 绑定、禁止 HTTP 重定向与严格 TLS 证书校验，SHALL NOT 查询凭据仓库或执行 AEAD 解密。周期探测调度与全局/策略组两级节点筛选 SHALL 直接依据节点激活状态与探测观测结论/延迟/新鲜度运行，SHALL NOT 因 `CredentialVersion <= 0` 跳过节点或因观测凭据版本不匹配而过滤观测记录。

##### Scenario: 无 Vault 环境下执行手动与周期探测 <a id="scenario-prob-hr-056-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 系统在无 Vault 依赖的情况下触发手动探测或周期探测任务
- **THEN** 激活节点不再因缺少 `CredentialVersion` 被跳过，`SafeNodeDialer` 直接使用 `domain.Node` 明文配置完成安全校验与拨号探测

##### Scenario: 探测观测直接参与节点两级筛选 <a id="scenario-prob-hr-056-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 全局筛选或策略组筛选配置了 `probe_verdict` 或 `probe_latency_ms` 条件且节点存在新鲜观测记录
- **THEN** 筛选器直接按观测新鲜度、结论与延迟判定节点是否匹配，不再检查 `HasValidCredentialVersion`

#### Requirement: 菜单脱离裁切容器并留在视口内 [HR-095] <a id="req-prob-hr-095"></a>

操作菜单 SHALL 不受父卡片 `overflow-hidden` 等裁切，且 SHALL 在视口边缘向可用方向打开或调整位置，不能覆盖到不可点击区域；关闭时 SHALL 不残留可聚焦的离屏元素。按触发器打开、点击外部或按 Escape 关闭 SHALL 保持正确，键盘打开后焦点 SHALL 可到达菜单项，关闭后可返回触发器。

##### Scenario: 订阅卡片末行靠近视口边缘 <a id="scenario-prob-hr-095-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 用户在卡片右下角或移动设备底端打开订阅源操作菜单
- **THEN** 全部可用菜单项可见且可点击，不受卡片边框裁切，必要时向上展开或滚动菜单

##### Scenario: 键盘操作 <a id="scenario-prob-hr-095-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 使用 Tab/Enter 打开菜单、导航项目并按 Escape 关闭
- **THEN** 菜单项目可依序到达且焦点回到触发器，不会因为定位至 body 而失去键盘操作

#### Requirement: 受控的出口 IP 风险探测 [HR-105] <a id="req-prob-hr-105"></a>

系统 SHALL 将 IP 风险查询建模为独立的 `ip_risk` 探测类型。每次查询 MUST 在已建立的被测节点探测通道内取得出口身份并经同一节点通道访问已配置 provider；宿主环境代理、直连连接或另一节点的出口不得作为该节点的风险证据。系统 MUST 禁止通过浏览器或 HTML 抓取 provider 网页端。

##### Scenario: 宿主代理与被测节点出口不同 <a id="scenario-prob-hr-105-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 宿主环境代理可以访问 provider 而被测节点探测通道失败
- **THEN** 系统记录该节点的 `error` 或 `unknown` 风险观察，且不会用宿主代理结果替代

#### Requirement: 到期探测仅取当前可探测激活节点 [HR-138] <a id="req-prob-hr-138"></a>

系统 SHALL 在每次到期批次创建时重新读取仓库激活节点，按稳定顺序分片至现有运行任务上限，不探测停用节点；无节点、凭据不可用或版本不匹配时 SHALL 记录明确且脱敏的跳过或失败证据，不得回退到直连或泄漏秘密；手动探测行为保持兼容。

##### Scenario: 激活范围变化与缺失凭据 <a id="scenario-prob-hr-138-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 节点在两个周期之间停用或缺少匹配版本的凭据
- **THEN** 下个批次不对停用节点建立探测；凭据异常节点不会绕过安全出站限制，页面和 API 可查看脱敏原因

##### Scenario: 大规模库存 <a id="scenario-prob-hr-138-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 激活节点数超过单运行任务预算
- **THEN** 系统在整体期限和队列上限内分批覆盖所有可探测目标或明确记录未覆盖数，不发生隐式截断和无限制并发

#### Requirement: 后端节点读模型（`NodeView`）完整填充最新探测状态与订阅来源 [HR-166] <a id="req-prob-hr-166"></a>

后端 `inventory.Service` 在 `ListNodesReadModel` 与 `GetNodeDetailWithRisk`（以及 `internal/transport/http/nodes.go` 的 `GET /api/v1/nodes`、`GET /api/v1/nodes/{logical_id}`、`PATCH /api/v1/nodes/{logical_id}/connection`）构建 `NodeView` 时 SHALL 调用 `s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil)` 与 `s.sources.ListByNodes(ctx, nodeIDs)`，并在 `NodeView` 上完整填充以下字段：
1. `latency_ms` (`*int64`)：优先取该节点最新 `baseline` 观测的 `LatencyMS`，若无 `baseline` 观测则取最新有效观测延迟；无任何观测时为 `nil`。
2. `last_probed_at` (`*time.Time`)：取该节点所有探测维度中最新一次观测的 `ObservedAt`；无任何观测时为 `nil`。
3. `health_status` (`string`)：基于最新 `baseline`（或综合）观测派生：`available` -> `"healthy"`，`restricted`/`stale` -> `"degraded"`，`error` -> `"unhealthy"`，无任何观测记录 -> `"unknown"`。
4. `probe_missing` (`bool`)：该节点无任何探测观测记录时 SHALL 为 `true`，否则为 `false`。
5. `probe_stale` (`bool`)：当存在探测记录且距离 `last_probed_at` 超过保鲜阈值（1 小时）时 SHALL 为 `true`，否则为 `false`。
6. `capabilities` (`map[string]CapabilityStatus`)：按 `ProbeKind` 填充结构化状态 `{ verdict, latency_ms, observed_at, summary, stale }`。
7. `sources` (`[]domain.NodeSource`)：填充该节点关联的订阅源列表，供前端展示节点来源及按订阅源筛选测速。

##### Scenario: 节点存在最新探测观测与订阅源时查询节点列表 <a id="scenario-prob-hr-166-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 某节点已关联订阅源 `sub-01`，且已完成 `baseline`（`available`, `42ms`）与 `streaming`（`available`, `85ms`）探测观测，客户端请求 `GET /api/v1/nodes` 或 `GET /api/v1/nodes/{logical_id}`
- **THEN** 返回的 `NodeView` 中 `latency_ms` 为 `42`、`health_status` 为 `"healthy"`、`probe_missing` 为 `false`、`probe_stale` 为 `false`、`capabilities.baseline.verdict` 为 `"available"`、`capabilities.streaming.verdict` 为 `"available"`，且 `sources` 包含 `sub-01`

##### Scenario: 节点尚无任何探测记录时查询节点列表 <a id="scenario-prob-hr-166-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 新入库节点尚无任何 `probe_observations` 记录，客户端请求 `GET /api/v1/nodes`
- **THEN** 返回的 `NodeView` 中 `probe_missing` 为 `true`、`health_status` 为 `"unknown"`、`latency_ms` 与 `last_probed_at` 为空

#### Requirement: IP risk probe distinguishes exit identity from risk intelligence [HR-180] <a id="req-prob-hr-180"></a>

系统在执行 `ip_risk` 探测时，MUST 严格区分“出口身份（Exit Identity）”与“IP 风险评分（IP Risk Intelligence）”：从 `cloudflare.com/cdn-cgi/trace` 等身份源提取到的出口 IP 与国家代码仅能证明出口身份存在（`ExitIdentityMissing = false`），绝不构成 IP 风险评分。在未获得真实 IP 风险情报源（`iprisk.Provider`）验证通过的风险评估结果时，系统 MUST 绝不将 `ip_risk` 判定为 `available`。

##### Scenario: 无法提取出口身份时判定为 missing_exit_identity <a id="scenario-prob-hr-180-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `ip_risk` 探针收到 `200`/`204` 响应但无法从中解析出合法出口 IP 与 2 位国家代码
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=missing_exit_identity`

##### Scenario: 仅具备出口身份而无真实风险源结果时判定为 unknown <a id="scenario-prob-hr-180-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** `ip_risk` 探针从 `cdn-cgi/trace` 成功解析出出口 IP 与国家代码，但未由真实风险情报源产出有效风险评估契约结果
- **THEN** 评估结果 MUST 为 `verdict=unknown`、`reason=contract_drift`，严禁将出口身份直接判定为 `available`

### Explicitly Pending Legacy Clauses & Conflicts

#### Requirement: 固定轻量巡检时钟与即刻巡检 [HR-098] <a id="req-prob-hr-098"></a>

系统 SHALL 运行固定周期的轻量扫描时钟（默认 10 分钟），用于定期巡检数据库中节点在各配置探测类别下的最新观测记录；当管理员启用计划或更新配置时，系统 SHALL 立即唤醒巡检流程而不等待整段检测周期；`next_due_at` 字段 SHALL 明确指示下一次巡检执行的时间戳，而非粗粒度的整段过期周期。

##### Scenario: 启用或修改配置即刻触发巡检 <a id="scenario-prob-hr-098-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 管理员启用定时检测或变更周期配置
- **THEN** 系统立即触发一次巡检评估，并将下次巡检时间设定为当前时间加固定巡检间隔（10分钟）

##### Scenario: 历史陈旧或遥远到期时间启动自愈 <a id="scenario-prob-hr-098-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 系统启动或执行恢复流程且数据库中已存留为空、过去已过期或远在数小时后的 `next_due_at`
- **THEN** 系统自动将其校正为当前立即巡检（`now`），消除历史陈旧延迟并按新轻量时钟推进

#### Requirement: 有界单轮配额与防饥饿公平调度 [HR-100] <a id="req-prob-hr-100"></a>

系统 SHALL 在单轮巡检中施加有界派发配额（默认单轮最多调度 512 个任务或对应节点上限），防止大规模节点（如 960 节点 * 5 kind）在初次启动或长假唤醒时瞬间压垮探测队列；在配额受限时，系统 SHALL 依据观测时间升序排序（从未观测节点排在最前，最陈旧观测次之），并在跨轮巡检中分页推进，确保所有过期节点均能在有限轮次内得到探测且无饥饿。

##### Scenario: 大规模节点首轮受控入池 <a id="scenario-prob-hr-100-1"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 系统中存在 960 个从未观测的激活节点且配置了多种探测类别
- **THEN** 单轮巡检仅截取配额内的任务数入池，剩余节点在下个轻量巡检周期按时间戳继续推进，不引起队列拥塞

##### Scenario: 老旧观测优先覆盖防饥饿 <a id="scenario-prob-hr-100-2"></a>
- **GIVEN** 探测引擎与待测节点就绪
- **WHEN** 待测过期节点数大于单轮配额
- **THEN** 观测记录最久远的节点优先入池，确保各节点刷新间隔分布均衡


---

## Success Criteria

### Measurable Outcomes
- **[SC-PROB-01]** 并发关闭死锁发生率为 0%，在 100 次高并发关闭测试中无挂起。
- **[SC-PROB-02]** 错误分类准确率达到 100%，无未捕获的通用未知崩溃。
- **[SC-PROB-03]** 单节点插队延迟小于 500ms（空闲协程状态下）。
- **[SC-PROB-04]** 未来时间戳写入拦截率达到 100%。

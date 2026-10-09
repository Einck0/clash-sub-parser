---
feature: 006-ui
title: UI Capability Specification
status: candidate
schema: speckit-native
---

# UI Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的管理控制台人机交互与视觉工效能力。管理界面必须采用紧凑自适应的现代流式排版，适配桌面与移动端视口，杜绝无来源硬编码尺寸，提供节点状态组合筛选与服务端分页联动、策略组抽屉式工作台、快速复制直链向导，并支持中英双语国际化。

## User Scenarios & Priority

### User Story 1 - 紧凑自适应布局与关键视口可用性 (Priority: P1)
- **GIVEN** 管理员在不同设备（桌面、笔记本、平板或移动端）访问控制台
- **WHEN** 浏览节点列表或管理大批量代理资产
- **THEN** 界面保持紧凑排版，文字无遮挡、无横向异常溢出，操作靶心与信息密度达成良好工效

### User Story 2 - 健康状态组合筛选与分页联动 (Priority: P1)
- **GIVEN** 节点列表中存在成百上千个不同探测状态的节点
- **WHEN** 管理员筛选存活、超时或未探测状态并切换分页
- **THEN** 服务端全量分页与状态多选筛选精准联动，筛选变更时自动重置至第 1 页

### User Story 3 - 抽屉式工作台与双语国际化 (Priority: P2)
- **GIVEN** 管理员需要细粒度配置策略组或修改节点连接参数
- **WHEN** 打开编辑抽屉并切换中英文语言
- **THEN** 抽屉提供所见即所得的配置体验，界面语言即时切换且不丢失当前表单输入

---

## Requirements

### 1. 布局与视口工效

#### Requirement: 节点列表紧凑布局与真实视口适配 [FR-UI-01] <a id="req-ui-compact-layout"></a>
管理控制台表格与面板 MUST 依据项目既有 Design Tokens 与流式响应式布局构建，坚决杜绝无来源假设的固定像素硬编码尺寸。界面在各类视口基准下必须保证内容无截断、无遮挡、无水平溢出滚动条。

##### Scenario: 桌面与移动视口无横向溢出无遮挡 <a id="scenario-ui-no-overflow"></a>
- **GIVEN** 常见的桌面与移动端视口分辨率
- **WHEN** 页面加载并渲染节点表格
- **THEN** 表格主体自适应容器宽度，内容完整可读，无视口级别水平异常滚动条

##### Scenario: 流式响应式布局适配 <a id="scenario-ui-streaming-layout"></a>
- **GIVEN** 用户调整浏览器窗口大小
- **WHEN** 视口尺寸动态改变
- **THEN** 栅格与弹性盒子自动重排，操作按钮始终处于可点击区域

---

### 2. 数据交互与工作台

#### Requirement: 节点健康状态筛选与全量服务端分页联动 [FR-UI-02] [NEEDS CLARIFICATION] <a id="req-ui-health-filter-pagination"></a>
节点管理面板 SHALL 支持按 ALIVE、TIMEOUT、FAILED、UNCHECKED 四态健康状态进行单选或组合筛选，并与服务端全量分页机制联动。依据用户在 Q6 中的决策落实筛选条件变更时的分页重置与跨页选择行为。

##### Scenario: 改变健康筛选状态重置为第 1 页 <a id="scenario-ui-pagination-reset"></a>
- **GIVEN** 用户正处于第 5 页数据视图
- **WHEN** 用户点击筛选仅查看“超时”节点
- **THEN** 分页器自动将当前页码重置为第 1 页，并向后端请求符合条件的最新数据

##### Scenario: 多选健康状态组合筛选 <a id="scenario-ui-multi-state-filter"></a>
- **GIVEN** 用户需要同时查看可用与未探测节点
- **WHEN** 勾选 ALIVE 与 UNCHECKED 组合过滤
- **THEN** 列表精准呈现符合任一选中状态的节点集合

#### Requirement: 策略组抽屉式编辑工作台 [FR-UI-03] <a id="req-ui-policy-drawer"></a>
系统 SHALL 提供侧边抽屉式（Drawer）策略组编辑工作台。工作台内清晰提供名称、类型（select/url-test/fallback/load-balance）、直属成员引用以及解包叶节点配置项，支持拖拽排序与实时预览。

##### Scenario: 抽屉中配置直接组引用与解包选项 <a id="scenario-ui-policy-drawer-edit"></a>
- **GIVEN** 打开策略组详情抽屉
- **WHEN** 管理员编排子策略组引用并选择解包开关
- **THEN** 界面清晰呈现子组嵌套关系与展开状态，表单提交后即时生效

#### Requirement: 订阅发布管理与复制直链向导 [FR-UI-04] <a id="req-ui-publication-wizard"></a>
发布管理面板 SHALL 集中展示所有活动发布端点，提供独立专属 Token 开关状态、格式选择（Mihomo / Clash / 纯节点）以及一键复制订阅直链向导弹窗。

##### Scenario: 快速复制订阅导出直链 <a id="scenario-ui-copy-link-wizard"></a>
- **GIVEN** 某个已配置的发布项目
- **WHEN** 用户点击“复制直链”按钮
- **THEN** 弹窗展示不同格式的订阅 URL，并自动将完整直链写入剪贴板

#### Requirement: 节点连接参数人工覆写详情抽屉 [FR-UI-05] <a id="req-ui-node-override-drawer"></a>
系统 SHALL 提供节点详情抽屉，支持用户对当前节点的服务器地址、端口、传输参数等进行手工覆写（CoW），并提供重置为订阅原始值的快捷操作。

##### Scenario: 抽屉保存人工连接覆写 <a id="scenario-ui-override-save"></a>
- **GIVEN** 节点详情抽屉处于编辑状态
- **WHEN** 用户修改端口并点击保存
- **THEN** 界面即时显示覆写高亮标记，后台持久化至 overrides 事实表

#### Requirement: 国际化双语支持 (zh-CN / en-US) [FR-UI-06] <a id="req-ui-bilingual-i18n"></a>
控制台界面 MUST 提供完善的国际化支持，涵盖简体中文（zh-CN）与英语（en-US）。语言切换时即时热替换文本，无需刷新整个页面且不丢失当前表单暂存状态。

##### Scenario: 界面语言即时切换不丢状态 <a id="scenario-ui-i18n-switch"></a>
- **GIVEN** 用户正在策略组抽屉中填写表单
- **WHEN** 点击语言切换选择 English
- **THEN** 抽屉内所有文案即时切换为英文，已填写的表单内容完整保留

---

---

---

---

## Supplemental Retained Behavioral Capabilities & Historical Clauses

### Overview
本节完整收录历史变更中属于本领域的行为规范、具体契约（包含精确状态码、传输参数、请求头校验等）与验收场景，保持行为约束无损物理留存与点对点双向追溯。

### Retained Functional Requirements

#### Requirement: 纯 HTTP 局域网环境下的 UUID 生成安全降级 [HR-011] <a id="req-ui-hr-011"></a>

前端在生成请求幂等键（如 `Idempotency-Key`）时，SHALL 检测当前执行上下文。若原生 `crypto.randomUUID` 在非安全上下文（纯 HTTP 局域网 IP）下为 `undefined`，系统 MUST 自动采用符合 RFC 4122 v4 规范的确定性退避伪随机算法生成合法 UUID，不得抛出未捕获的运行时异常。

##### Scenario: 局域网 HTTP 地址下触发订阅立即刷新 <a id="scenario-ui-hr-011-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户通过 `http://192.168.1.100:18080` 访问前端界面并点击刷新订阅
- **THEN** 系统顺利生成合法格式的 UUID v4 幂等凭据并完成网络请求，控制台无报错

#### Requirement: 探测工作台轮询风暴消除 [HR-012] <a id="req-ui-hr-012"></a>

`ProbesView.vue` 与 `useProbes.ts` 在后台周期同步期间，SHALL NOT 在每个 2 秒心跳周期内循环发起最多 20 页的全量节点请求。当存在运行中探测任务时，系统 SHALL 仅针对活跃运行状态（Runs）、队列状态（Pool Status）与关联观测执行轻量轮询；节点列表仅在单次探测完成、任务启停或用户显式触发时进行针对性刷新。

##### Scenario: 探测运行中后台心跳轮询 <a id="scenario-ui-hr-012-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 探测工作台正在执行批次探测
- **THEN** 2 秒定时器仅请求运行状态与观测，不发起全量节点台账的深度分页扫描

#### Requirement: 订阅源配置抽屉必填校验守卫 [HR-014] <a id="req-ui-hr-014"></a>

在 `SubscriptionConfigDrawer.vue` 中，当用户点击抽屉底部的“保存”按钮时，系统 MUST 优先触发表单的完整性校验（涵盖当前激活与未激活 Tab 的必填字段，包括名称与订阅地址）。若任一必填字段为空或非法，系统 SHALL 阻止向父组件派发 `save` 事件，并向用户提示具体的校验错误。

##### Scenario: 在高级配置 Tab 中直接点击保存未填名称的订阅 <a id="scenario-ui-hr-014-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户打开新建订阅抽屉，直接切换至“高级设置”Tab 并点击底部的保存按钮
- **THEN** 表单拦截提交动作，自动聚焦或提示“基础设置”中的订阅名称与地址必填项，不发起网络请求

#### Requirement: 端到端测试多语言环境鲁棒性断言 [HR-019] <a id="req-ui-hr-019"></a>

`web/e2e/auth-real-server.spec.ts` 在启动真实浏览器上下文时，SHALL 显式设置固定的语言标识（通过注入 `csp_locale: 'en-US'` 或覆盖浏览器默认 Accept-Language），或采用双语/不依赖硬编码自然语言的 `data-testid` 属性进行 UI 元素断言，确保测试在默认中文环境下保持 100% 确定性通过。

##### Scenario: 在中文默认系统环境中运行真实服务端到端测试 <a id="scenario-ui-hr-019-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 执行 Playwright E2E 真实服务器认证对抗测试
- **THEN** 测试由于明确的语言基线配置，无语言文字不匹配失败，全部用例执行通过

#### Requirement: 三态主题和无闪烁首屏 [HR-030] <a id="req-ui-hr-030"></a>

管理界面 SHALL 提供 `light`、`dark` 和 `system` 主题偏好，并在首屏可交互前将有效主题应用到根元素。用户显式偏好 MUST 被本地保存；`system` MUST 随操作系统偏好变化。主题切换 MUST 不改变当前路由、表单草稿或服务端数据。

##### Scenario: 暗色偏好首次访问 <a id="scenario-ui-hr-030-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户保存 dark 偏好后重新加载管理界面
- **THEN** 首个可见界面以 dark 主题呈现，且当前路由保持不变

#### Requirement: 服务端分页与查询状态 [HR-031] <a id="req-ui-hr-031"></a>

节点台账界面 SHALL 仅请求当前服务端页并将筛选、排序和页码反映为可恢复的 URL 查询状态。加载中、空结果、无权限和失败状态 MUST 有区别明确的界面反馈；界面 MUST 不在浏览器内获取完整节点集合后再模拟分页。

##### Scenario: URL 恢复筛选页面 <a id="scenario-ui-hr-031-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户复制包含有效筛选和页码的节点台账 URL 并在新窗口打开
- **THEN** 界面恢复对应筛选与页码，并请求该服务端页

#### Requirement: 移动端触控与抽屉行为 [HR-032] <a id="req-ui-hr-032"></a>

在宽度 375 CSS px 的视口上，主要可点击控制项的可见或可触达靶心 MUST 不小于 44 × 44 CSS px，页面 MUST 不产生横向滚动。节点详情 MUST 以可关闭的 bottom sheet 或等效可访问抽屉呈现，并支持键盘焦点约束、Escape 关闭和返回焦点。

##### Scenario: 移动端节点详情 <a id="scenario-ui-hr-032-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户在 375 CSS px 宽视口选择一个节点
- **THEN** 详情以可访问 bottom sheet 或抽屉打开，页面没有横向溢出，关闭后焦点回到原触发项

#### Requirement: 破坏性与长时命令反馈 [HR-033] <a id="req-ui-hr-033"></a>

界面 MUST 在删除、停用、撤销发布、导入、刷新和探测等命令前后展示明确状态。破坏性命令 MUST 要求确认；长时作业 MUST 显示作业状态并允许在授权范围内取消，不能通过无提示重复点击创建重复命令。

##### Scenario: 重复点击探测按钮 <a id="scenario-ui-hr-033-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户在探测提交尚未收到响应时再次点击同一按钮
- **THEN** 界面维持单一进行中状态并不会创建第二个请求

#### Requirement: 管理端节点、预览与订阅明文直读直编 [HR-057] <a id="req-ui-hr-057"></a>

管理端节点详情接口 `GET /api/v1/nodes/{logical_id}` SHALL 直接返回完整 `domain.Node` 明文信息（含 `server`、`port`、`credentials`）；节点更新接口 `PATCH /api/v1/nodes/{logical_id}` SHALL 直接接收并更新 `nodes` 表的明文字段（`display_name`、`server`、`port`、`credentials`），SHALL NOT 要求 `expected_credential_version` CAS 版本号、Write-Only 输入字段或拦截 `server`/`port` 修改。前端节点详情视图 SHALL 直接展示并编辑明文字段，移除 `***` 掩码、Write-Only 轮换输入、CAS 版本号、Vault AEAD 徽章与 Unavailable 横幅；发布预览视图 SHALL 直接展示真实配置文本而不调用 `redactPreviewSecrets`；订阅接口与前端订阅抽屉 SHALL 直接返回并回显真实订阅 URL 而不使用 `***` 占位。

##### Scenario: 管理端查看与直接编辑节点明文配置 <a id="scenario-ui-hr-057-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 管理员打开节点详情抽屉查看配置并修改 `display_name`、`server`、`port` 或协议凭据字段后保存
- **THEN** `GET /api/v1/nodes/{logical_id}` 直接回显明文配置，`PATCH /api/v1/nodes/{logical_id}` 直接更新 `nodes` 表并返回更新后的完整节点明文信息，界面无 `***` 掩码或 CAS 版本冲突阻断

##### Scenario: 发布预览与订阅编辑展示真实明文内容 <a id="scenario-ui-hr-057-2"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 管理员在发布页预览编译配置或打开已有订阅的编辑抽屉
- **THEN** 发布预览直接显示包含真实密码/私钥的完整客户端配置文本，订阅编辑抽屉直接显示真实 `source_url_secret_ref` URL

#### Requirement: 服务端分页与查询状态 [HR-103] <a id="req-ui-hr-103"></a>

节点台账界面 SHALL 仅请求当前服务端页并将筛选、排序和页码反映为可恢复的 URL 查询状态。加载中、空结果、无权限和失败状态 MUST 有区别明确的界面反馈；界面 MUST 不在浏览器内获取完整节点集合后再模拟分页。风险 decision、risk level、provider 和过期状态 MUST 作为服务端节点筛选条件，并在 URL 状态中可恢复。

##### Scenario: URL 恢复筛选页面 <a id="scenario-ui-hr-103-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户复制包含有效筛选和页码的节点台账 URL 并在新窗口打开
- **THEN** 界面恢复对应筛选与页码，并请求该服务端页

##### Scenario: 恢复风险筛选 <a id="scenario-ui-hr-103-2"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户打开包含有效风险筛选、排序和页码的节点台账 URL
- **THEN** 界面恢复风险筛选并请求对应服务端页，而非下载完整节点集合

#### Requirement: 破坏性与长时命令反馈 [HR-104] <a id="req-ui-hr-104"></a>

界面 MUST 在删除、停用、撤销发布、导入、刷新和探测等命令前后展示明确状态。破坏性命令 MUST 要求确认；长时作业 MUST 显示作业状态并允许在授权范围内取消，不能通过无提示重复点击创建重复命令。界面 SHALL 对风险 policy 造成的准入排除和 publication preflight 拒绝显示 policy revision、脱敏原因与明确的 unknown 状态，不得将 unknown 表示为低风险。

##### Scenario: 重复点击探测按钮 <a id="scenario-ui-hr-104-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户在探测提交尚未收到响应时再次点击同一按钮
- **THEN** 界面维持单一进行中状态并不会创建第二个请求

##### Scenario: 风险拦截反馈 <a id="scenario-ui-hr-104-2"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 发布预检因节点 IP risk 决策而被拒绝
- **THEN** 界面显示脱敏 diagnostics 和 policy revision，且不把该节点或未知结果渲染为通过

#### Requirement: 节点风险摘要与未知状态回显 [HR-110] <a id="req-ui-hr-110"></a>

工作台 SHALL 在节点台账和详情中显示当前风险决策、风险等级或 score 区间、provider 标识、观察时间、过期状态和脱敏原因。工作台 MUST 明确区分 `allow`、`review`、`block`、`unknown`、`stale` 和 provider error，且不得将 `unknown`、`stale` 或 error 渲染为低风险或绿色通过。

##### Scenario: 未配置风险 provider <a id="scenario-ui-hr-110-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 节点没有可用的 IP risk 观察，因为没有启用 provider
- **THEN** 工作台显示 `unknown` 与配置缺失原因，不显示低风险 score

#### Requirement: 服务端风险筛选与稳定分页 [HR-111] <a id="req-ui-hr-111"></a>

工作台 SHALL 将风险决策、风险等级、provider、过期状态和策略组风险状态作为可恢复 URL query 的服务端筛选条件。节点列表 MUST 在风险筛选后进行固定排序和分页，并返回与该筛选集合一致的 `total`；浏览器不得下载完整台账后再执行风险筛选。

##### Scenario: 筛选被阻断节点 <a id="scenario-ui-hr-111-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 操作者在节点台账选择 `risk_decision=block`
- **THEN** 工作台请求对应服务端页并只显示当前筛选集合中的节点及一致 total

#### Requirement: 无敏感风险数据的客户端边界 [HR-113] <a id="req-ui-hr-113"></a>

工作台 MUST 不接收、缓存、记录或显示完整出口 IP、provider API key、原始 provider response、cookie 或包含凭据的查询 URL。风险展示 API 仅返回规范化摘要与必要的版本和时间元数据。

##### Scenario: 查看节点风险详情 <a id="scenario-ui-hr-113-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 操作者打开节点风险详情
- **THEN** 响应和界面仅包含允许的规范化摘要，不包含完整 IP 或 provider 原始 payload

#### Requirement: 发布预览和模板生成可预测 [HR-202] <a id="req-ui-hr-202"></a>

管理界面 SHALL 在合法输入下获得 `POST /api/v1/publications/preview` 的有效预览结果；模板生成 SHALL 明确区分合法输入、边界输入与无效输入，无效输入或后端失败 MUST NOT 变为未说明的 500 或虚假的成功状态。前后端 SHALL 按一致的错误语义展示可理解反馈。

##### Scenario: 合法与边界输入 <a id="scenario-ui-hr-202-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户在隔离实例上提交合法发布预览或模板生成输入（包括明确支持的边界值）
- **THEN** 服务 SHALL 返回契约定义的有效结果，界面 SHALL 展示对应预览且不报告失败

##### Scenario: 无效模板或预览输入 <a id="scenario-ui-hr-202-2"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户提交缺失、格式错误或不受支持的模板生成/预览输入
- **THEN** 服务 SHALL 返回可解释的非成功结果，界面 SHALL 展示对应错误与恢复动作，MUST NOT 因可预期输入错误返回未说明的 500

#### Requirement: 全路由完整双语与失败恢复 [HR-203] <a id="req-ui-hr-203"></a>

管理界面 SHALL 在 dashboard、subscriptions、nodes、probes、policy、publications、settings 七个入口提供完整中英文用户文案，MUST NOT 显示裸 i18n key。可恢复的请求异常或空状态 SHALL 呈现与状态相符的引导和可操作的重试或返回路径，重试后 SHALL 使用真实服务端结果，MUST NOT 伪报成功。

##### Scenario: 切换中英文 <a id="scenario-ui-hr-203-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户分别以中文和英文浏览七个入口、异常提示及抽屉
- **THEN** 页面 SHALL 显示自然语言文本而非裸 key，且不出现未处理的运行时错误

##### Scenario: 失败后重试 <a id="scenario-ui-hr-203-2"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 视图请求返回 401、403、409、500 或网络错误，随后用户触发可用的恢复/重试动作
- **THEN** 页面 SHALL 呈现可理解的失败说明与适用的下一步，并按新的真实响应更新状态；鉴权失败 SHALL 引导鉴权恢复，失败的写操作 MUST NOT 显示成功

#### Requirement: PolicyView 拓扑与抽屉一致 [HR-204] <a id="req-ui-hr-204"></a>

PolicyView SHALL 将拓扑节点的选择与相应详情/编辑抽屉联动；抽屉保存或取消后拓扑、详情与服务端确认的状态 SHALL 保持一致，失败 MUST NOT 静默丢失用户可见状态。

##### Scenario: 选中拓扑节点并编辑 <a id="scenario-ui-hr-204-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户在策略拓扑中选择节点、打开抽屉并保存有效更改或取消编辑
- **THEN** 抽屉 SHALL 对应选中节点；保存成功后拓扑和详情 SHALL 呈现已确认的新状态，取消 SHALL 不修改已保存状态

#### Requirement: 视口可触达与证据边界 [HR-205] <a id="req-ui-hr-205"></a>

界面 SHALL 在 392×872、375×667 和 1280×800 视口保证关键操作可辨识且可通过正常滚动、键盘与指针到达；交互区域 MUST 有可访问名称、可见焦点及可可靠激活的范围，相邻操作 MUST 可区分而不发生误触重叠，不以统一像素尺寸作为充分或必要门禁。底部 Dock MUST NOT 遮挡最后的关键内容或操作，抽屉 MUST NOT 产生不可恢复的横向溢出，其关闭及提交操作 MUST 可到达。自动化验收 SHALL 经唯一授权的 SSH→Ubuntu chroot ARM64 Chromium Playwright 路径执行，并 MUST 明确区分 headless 视口证据与实屏触控证据；设备 headless 结果 MUST NOT 声称实屏通过。生产 18080/17000 与数据卷 MUST NOT 接收测试写入。

##### Scenario: 跨视口浏览与抽屉 <a id="scenario-ui-hr-205-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 浏览器在上述三个视口依次打开七入口、滚动到底部并打开 PolicyView 或其他编辑抽屉，尝试焦点导航与关键操作
- **THEN** 关键控件 SHALL 能辨识、聚焦并可靠激活，邻近目标不会重叠误触，底 Dock 不遮挡末项，抽屉关闭与主操作可达、无不可恢复的横向溢出，主路由无裸 key 或未处理错误；验收 SHALL 记录实际可见性、遮挡、滚动及操作结果，而非只断言统一尺寸

##### Scenario: headless 取证报告 <a id="scenario-ui-hr-205-2"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 使用设备 Ubuntu chroot 的 Chromium Playwright 完成无实屏的回归
- **THEN** 报告 SHALL 标明 headless 模式、视口、构建与脱敏错误证据，MUST NOT 将其描述为实屏触控、软键盘或系统安全区域验收

#### Requirement: 流式内容与弹层可达 [HR-206] <a id="req-ui-hr-206"></a>

管理界面 SHALL 随可用视口及内容变化自动适配主导航、卡片、抽屉和对话框；关键内容及操作 MUST 可通过正常滚动、键盘和指针访问，固定导航 MUST NOT 遮挡末项，弹层 MUST NOT 在可用视口发生不可恢复的横向溢出。

##### Scenario: 小屏长内容 <a id="scenario-ui-hr-206-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户在窄视口打开七入口之一并滚动长列表至末项，再打开长表单抽屉
- **THEN** 用户 SHALL 能读完末项并操作提交或关闭按钮，内容不会被底部导航遮住，抽屉在视口内可滚动且不产生不可恢复的横向溢出

##### Scenario: 桌面及文本放大 <a id="scenario-ui-hr-206-2"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 浏览器切换桌面视口或放大文字、使用更长的中英文文案
- **THEN** 导航、卡片和主要操作 SHALL 保持可识别、可到达，允许自适应换行或滚动，MUST NOT 以裁剪关键标签代替布局适配

#### Requirement: 语义交互面积而非统一硬尺寸 [HR-207] <a id="req-ui-hr-207"></a>

界面 SHALL 将主要操作呈现为可明确辨识并可可靠激活的语义交互区域；视觉图标大小 MUST NOT 被误认为实际点击区域。界面 MUST NOT 仅凭统一固定像素宽高或扫描字面量判断触控可用性；仅有明确上下文理由的局部尺寸例外 SHALL 被记录并进行交互复验。

##### Scenario: 密集图标和表单控件 <a id="scenario-ui-hr-207-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户使用移动视口激活 Dock 图标、抽屉关闭按钮、复选框及相邻小控件
- **THEN** 对应意图 SHALL 有独立可识别的交互区域，邻近目标不会无意重叠或难以区分，键盘焦点与可访问名称 SHALL 可发现；不要求每个视觉图标等于同一固定尺寸

#### Requirement: 跨主题一致与验收证据 [HR-208] <a id="req-ui-hr-208"></a>

界面 SHALL 在现有主题中保持可辨识的控件状态和布局；验收 SHALL 覆盖代表性窄屏、桌面、文本放大和长内容的关键用户路径，记录实际可见性、滚动及点击证据。设备无头浏览器结果 MUST 明确标记为 headless 视口证据，不得声称通过实屏、软键盘或物理触控。

##### Scenario: 隔离浏览器回归 <a id="scenario-ui-hr-208-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 在隔离实例上执行不同视口、主题和文字长度的浏览器回归
- **THEN** 报告 SHALL 包含运行命令/退出码、视口和模式、被测路径、失败截图或可复验日志，MUST NOT 仅以一组尺寸断言或 CSS 字面量扫描宣称体验合格

### Explicitly Pending Historical Stubs

#### Requirement: 高风险准入和发布提示 [HR-112] <a id="req-ui-hr-112"></a>

工作台 SHALL 在策略组配置和 publication preflight 中显示风险 policy 是否启用、其 revision、未知处理方式和已阻断或待审节点数。对于会使节点被排除或使 publication 被拒绝的操作，工作台 MUST 在提交前显示后果并在收到诊断后保留可读的失败状态；界面不得自动关闭、隐藏或擅自修改 policy 以绕过拦截。

##### Scenario: publication 预检被拒绝 <a id="scenario-ui-hr-112-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 发布请求因风险 policy 被拒绝
- **THEN** 工作台显示脱敏节点标识、原因代码和 policy revision，并保持既有 publication 状态不变

### Explicitly Pending Legacy Clauses & Conflicts

#### Requirement: 节点台账筛选状态下的无限滚动防饥饿 [HR-013] <a id="req-ui-hr-013"></a>

在 `NodesView.vue` 启用健康度客户端筛选（如筛选 degraded 或 unhealthy）时，若当前已加载的匹配条目数量不足以填满用户视口高度且后端仍有更多数据（`hasMore === true`），组件 SHALL 自动触发后续分页加载（`loadMore`），直到视口填满或数据全部加载完毕，杜绝因未达滚动阈值而导致的列表假性空白与死锁。

##### Scenario: 筛选少量异常节点触发自动补充分页 <a id="scenario-ui-hr-013-1"></a>
- **GIVEN** 管理控制台处于当前视口与活动页面
- **WHEN** 用户在拥有 500 个节点的台账中切换至“异常”筛选，第一页数据中无异常节点
- **THEN** 页面检测到视口未充满且存在后续分页，自动递归请求下一页，直至找到异常节点渲染或全部数据读取完毕


---

## Success Criteria

### Measurable Outcomes
- **[SC-UI-01]** 常见移动与桌面视口横向溢出发生率为 0%。
- **[SC-UI-02]** 筛选条件变更时分页重置成功率达到 100%。
- **[SC-UI-03]** 抽屉表单提交与 CoW 覆写生效成功率达到 100%。
- **[SC-UI-04]** 国际化中英双语词条覆盖率达到 100%，无未翻译键名泄漏。

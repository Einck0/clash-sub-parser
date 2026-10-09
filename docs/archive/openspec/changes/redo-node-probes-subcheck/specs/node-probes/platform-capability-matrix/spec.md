## Purpose

规范基于 beck-8/subs-check 的真实商业流媒体与主流 AI 服务能力判定契约，消除虚假占位与硬编码拒绝。

## ADDED Requirements

### Requirement: OpenAI 双轨分级与地区识别
系统探针对 OpenAI / ChatGPT 的可用性探测 SHALL 采用双轨机制：轨 1 请求 Web 端 `https://api.openai.com/compliance/cookie_requirements`（判断响应是否排除 `unsupported_country`）；轨 2 携带移动端请求头请求 `https://ios.chat.openai.com`（判断响应是否排除 `unsupported_country` 及 `vpn` 封禁信号）。
- 双轨全部通过时，系统判定为全平台可用（`Full = true`，标记为 `GPT⁺`）；
- 仅单轨通过时，系统判定为仅网页端可用（`Web = true`，标记为 `GPT`）；
- 两轨均失败时，系统判定为不可用。
系统 SHALL 进一步通过 Trace 提取落地 ISO 国家代码（如 `US`, `JP`）。

#### Scenario: 客户端网关通过判定为满血可用
- **WHEN** 节点的双轨请求均未返回地域不支持与封禁信号
- **THEN** 探测结果输出 `openai` 能力状态为 `Full` 并附带检测到的国家代码

#### Scenario: 网页端通过但移动客户端拦截
- **WHEN** 节点的轨 1 正常通过，但轨 2 返回 vpn 拦截关键字
- **THEN** 探测结果输出 `openai` 为 `Web` 可用，标记为 `GPT` 降级可用

### Requirement: Netflix 细粒度解锁分级与封禁识别
系统对 Netflix 的探测 SHALL 区分全解锁、仅自制剧与 IP 封禁三类细粒度状态。系统 SHALL 首先请求 Fast.com CDN 测速接口；若 Fast 接口返回 403，直接判定为 `Banned = true`（被 Netflix 全局风控封锁）；若返回有效国家，判定为全解锁并提取区域。在回退模式下，系统结合非自制剧 Title (81280792) 与自制剧 Title (70143836) 进行状态码断言：
- 非自制剧返回 200/301：判定为全解锁（`Full = true`）；
- 非自制剧 404 但自制剧 200/301：判定为仅自制剧（`OriginalsOnly = true`）；
- 任一 Title 收到 403 明确封锁：判定为封禁（`Banned = true`）。
系统 MUST 坚决杜绝把单一 403 或通用 200 简单笼统等同于“解锁”或“错误”。

#### Scenario: 仅自制剧解锁正确分类
- **WHEN** 探测非自制剧返回 404 但自制剧返回 200
- **THEN** 探测结果输出 `netflix` 细粒度分类为 `OriginalsOnly`，不虚报全解锁

#### Scenario: Netflix 明确 403 封锁识别
- **WHEN** Fast 接口或 Title 请求返回 403 Forbidden
- **THEN** 探测结果输出 `netflix` 细粒度分类为 `Banned`

### Requirement: YouTube 地区码提取与送中严格识别
系统对 YouTube 的探测 SHALL 请求 `https://www.youtube.com/premium` 页面并提取页面中的 `INNERTUBE_CONTEXT_GL` 区域标签。若响应中包含 `www.google.cn` 或重定向送中特征，系统 MUST 明确输出 `CN` 送中状态；若响应包含 `Premium is not available in your country`，系统 MUST 判定为 Premium 不可用；只有在 HTTP 状态码为 2xx 且包含正向会员或广告关键词时，系统方可认定为解锁并输出大写二字国家码。

#### Scenario: 识别被重定向送中
- **WHEN** 页面返回内容包含 `www.google.cn`
- **THEN** 结果标记为送中 `CN`

#### Scenario: 正常商业解锁提取国家
- **WHEN** 状态码为 200 且匹配正向会员标识及区域码 `SG`
- **THEN** 结果标记为 YouTube 解锁，区域为 `SG`

### Requirement: Disney+、Claude 与 IP 风险真实检测
系统 SHALL 支持 Disney+（通过 BAMGrid 认证断言接口判断 `Unlocked`、`Soon` 或 `Banned`）、Claude（通过 Trace 提取地区码并核对受制裁国家黑名单）以及 Scamalytics IP 欺诈风险评分检测，所有支持的平台均执行真实 HTTP 探测并输出客观结论，严禁以 `contract_drift` 进行内部假报错拦截。

#### Scenario: Claude 封禁地区识别
- **WHEN** 节点 Trace 显示出口地区为 `CN`、`HK` 或 `RU`
- **THEN** Claude 检测判定为不可用

#### Scenario: Scamalytics 风险分提取
- **WHEN** 请求 Scamalytics 查询接口返回 200 且包含欺诈分析数值
- **THEN** 系统提取分数百分比并写入观测摘要

---
feature: 004-publication
title: Publication Capability Specification (Clean Slate Minimal Client Subscriptions)
status: implementing # eliminating over-engineering, unified settings token, clean routes, primary sub userinfo & group ordering
schema: speckit-native
---

# Publication Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（CSP 1.0）的客户端订阅导出与配置编译能力。
CSP 的核心定位是**个人私有代理订阅解析与规则编排控制平面**。本规范彻底消除将个人工具异化为商业 SaaS 发卡系统的过度设计（Over-Engineering）：
1. 废除多余的 `subscription_links` 实体表、UUIDv7 路径、专属随机高熵 Token 状态机、轮换/吊销及公网基准地址等冗余胶水；
2. 建立直观、人类可读的极简客户端导出直链（如 `/clash`、`/singbox`、`/surge`、`/mihomo`、`/qx`，及 `/sub/{target}`）；
3. 统一使用系统现有 Settings Token 进行单点鉴权：Open Mode 下直接免密直链导出；Protected Mode 下统一使用系统设置中的 Token 进行保护（支持 `?token=...` 与 `Authorization: Bearer ...`）；
4. 订阅导出完全在客户端发起 HTTP 请求时**现场动态编译（Live on-request compilation）**，基于当前已提交的节点、策略组与分流规则最新一致性事实实时输出；
5. 支持设置主力订阅并透传流量与过期信息（`Subscription-Userinfo`）到导出的客户端订阅响应头中；
6. 恢复并保全策略组自定义排序（`position`），确保导出的策略组与用户在控制台编排的顺序 100% 一致。

## 本轮状态、范围与规则优先级（2026-10-17 架构重塑与体验修复）

依据 Einck 当前原话最高指令：
“现在有几个问题，没有设置主订阅的功能，主订阅主要是将该订阅的信息返回给当前的订阅，就是一些剩余时间啊、剩余流量什么的，策略组顺序和之前不一样，是因为策略组现在没有顺序了吗”
“那就修复”

- **主订阅与流量透传边界**：
  - 用户可在订阅源列表中设置一个启用的“主订阅”；
  - 拉取上游订阅时解析上游返回的 `Subscription-Userinfo` 头部并持久化；
  - 导出直链时，若存在启用的主订阅且有 userinfo，在 HTTP 响应头注入 `Subscription-Userinfo: <userinfo>`；
  - 前端订阅源卡片直观展示已用流量、剩余流量与到期时间；主订阅只在列表卡片操作（`设为主订阅`），不在表单里硬塞；
- **策略组排序边界**：
  - 数据库 `node_groups` 表支持 `position` 字段；
  - 策略组查询与编译器输出按 `position ASC, created_at ASC, id ASC` 严格保序；
  - 前端支持上移/下移或调整排序，彻底解决之前按拼音字母无序散乱排布的问题；
- **核心能力边界**：
  - Mihomo / Clash 导出原生完整拓扑配置（proxies, proxy-groups, rules, dns）；
  - Sing-box、Surge、Quantumult X 导出符合目标客户端协议规范的合法节点配置；
  - 业务筛选核心算法（`cheap = all - expensive - other`）与节点台账完整保全，绝不破坏存量业务数据；
- **历史兼容性**：既有历史冻结发布（`/publish/v1/{id}`、`/p/{id}`）保持只读兼容；废弃端点 `/yaml`、`/script` 维持 HTTP 410 Gone。

---

## User Scenarios & Priority

### User Story 1 - 极简人类可读直链与现场动态编译 (Priority: P1)
- **GIVEN** 管理员在 CSP 中配置了订阅源、策略组和分流规则
- **WHEN** 客户端请求直观后缀的订阅端点（如 `GET /clash`、`GET /singbox` 或 `GET /sub/mihomo`）
- **THEN** 系统在收到请求时现场从 SQLite 读取当前一致性事实，实时动态编译并返回对应格式配置，完全无需先执行“发布”或“创建订阅链接”操作

### User Story 2 - 统一使用 Settings Token 鉴权 (Priority: P1)
- **GIVEN** 系统在设置（Settings）中配置了统一管理员安全 Token（Protected Mode）
- **WHEN** 客户端请求带有 `?token=<admin_token>` 或 Header `Authorization: Bearer <admin_token>`
- **THEN** 系统通过统一 Token 鉴权放行并输出配置；若未提供或 Token 错误，返回 401 Unauthorized

### User Story 3 - 开放模式零门槛直出 (Priority: P2)
- **GIVEN** 系统未配置 Token（Open Mode）或设置中关闭了导出保护开关
- **WHEN** 客户端以无参方式直接访问 `GET /clash` 或 `GET /singbox`
- **THEN** 服务端直接免密下发配置，无任何多余弹窗与鉴权阻断

### User Story 4 - 极简管理体验与直观复制 (Priority: P2)
- **GIVEN** 管理员在控制台打开“客户端订阅”页面
- **WHEN** 查看订阅入口
- **THEN** 界面直接呈现 4 种主流客户端直链（Mihomo/Clash、Sing-box、Surge、Quantumult X），一键复制即可导入客户端；完全不展示任何 UUID、状态机、轮换、吊销、掩码等过度设计元素

### User Story 5 - 主订阅与流量/时间透传 (Priority: P1)
- **GIVEN** 管理员在订阅列表中将某个主力机场订阅标记为“主订阅”
- **WHEN** 客户端拉取导出直链（如 `/clash`、`/singbox`、`/surge`、`/qx`）
- **THEN** 响应中包含 `Subscription-Userinfo` 头部，客户端（如 Clash Verge 等）能够直接在界面上显示该主订阅的剩余流量与过期时间

### User Story 6 - 策略组自定义顺序保序 (Priority: P1)
- **GIVEN** 管理员在策略组管理界面编排策略组顺序
- **WHEN** 客户端拉取 Mihomo/Clash 等客户端配置
- **THEN** 配置中的 `proxy-groups` 严格按照管理员编排的顺序呈现，不再按字母无序散乱排序

---

## Requirements

### 1. 直观客户端直链与现场动态编译

#### Requirement: 极简人类可读导出路由 [FR-PUB-CLEAN-01]
系统 SHALL 在 HTTP 控制平面直接暴露直观的客户端类型导出端点：
- `/clash` 与 `/mihomo`：映射到 `TargetMihomo`（完整拓扑 YAML 配置）；
- `/singbox`：映射到 `TargetSingBox`（JSON 配置）；
- `/surge`：映射到 `TargetSurge`（Conf 文本配置）；
- `/qx` 与 `/quantumult-x`：映射到 `TargetQuantumultX`（Conf 文本配置）；
同时支持 `/sub/{target}` 通用前缀路由（`{target}` 为 mihomo, clash, singbox, surge, qx）。客户端请求不受复杂的 UUIDv7 限制。

#### Requirement: 现场动态编译契约 [FR-PUB-CLEAN-02]
当客户端请求合法导出路由时，服务端 MUST 现场从当前存储中加载最新一致性快照（`ConsistentFacts`），并在内存中实时执行策略解析与目标编译器渲染：
1. 每次请求均反映最新的节点增删、启用状态、前缀更新与策略分流变更；
2. 支持标准 HTTP 条件请求（`ETag` 与 `If-None-Match`），内容未发生变化时直接响应 `304 Not Modified`，降低客户端与服务端带宽开销；
3. 严格设置 `Cache-Control: no-store` 与 `Referrer-Policy: no-referrer`。

---

### 2. 统一 Settings Token 鉴权

#### Requirement: 复用系统 Settings Token 统一鉴权 [FR-PUB-CLEAN-03]
订阅导出端点 SHALL 直接集成系统 `DynamicTokenHolder`（或 Settings Token）进行鉴权，废除独立的订阅链接 Token 数据库表与哈希校验：
1. **开放模式（Open Mode / `!exportAuthEnabled`）**：直接允许匿名访问，不强制要求 `token` 参数；
2. **保护模式（Protected Mode / `exportAuthEnabled`）**：
   - 客户端可通过 Query 参数 `?token=<secret>` 或 Header `Authorization: Bearer <secret>` 传递凭据；
   - 系统直接使用 `TokenHolder.Verify(candidate)` 校验；
   - 校验通过则放行；未提供或校验失败统一返回 `401 Unauthorized`；
   - 严格禁止在 URL 中同时混用不一致的 query token 与 Bearer header。

#### Requirement: 消除多余配置与中间状态 [FR-PUB-CLEAN-04]
1. 彻底移除前端对 `subscription_links` 独立管理接口（CRUD、Rotate、Revoke、PublicBaseURL 强制要求等）的依赖；
2. 客户端直链构造完全依赖当前访问入口（`window.location.origin`）或已配置的对外反代域名；
3. 未配置公网基准地址时，直接基于当前主机地址输出，绝不抛出 422 阻断复制；
4. 彻底消除伪状态机（如“默认链接状态”、“未初始化”、“已撤销”、“Token 掩码”等）。

---

### 3. 前端极简体验

#### Requirement: 四行直链卡片与一键复制 [FR-PUB-CLEAN-05]
前端“客户端订阅”页面 SHALL 采用极简设计：
1. 四个目标清晰并列：Mihomo / Clash、Sing-box、Surge、Quantumult X；
2. 清晰展示直链地址（如 `https://sub.einck.top/clash?token=xxx` 或 `http://127.0.0.1:18080/clash`）；
3. 如果系统启用了保护模式，且当前管理端处于已认证状态，前端自动从本地认证会话（`localStorage.getItem('csp_token')`）提取当前全局 Token 拼入展示直链；
4. 提供“复制订阅链接”与“下载配置文件”核心操作；
5. 剪贴板不可用时优雅弹出只读全选复制框。

---

### 4. 资产保全与历史兼容

#### Requirement: 核心数据零丢失与历史兼容 [FR-PUB-CLEAN-06]
1. 存量节点数据、订阅源、策略组、分流规则 100% 保持完好，不执行破坏性截断；
2. 历史冻结快照接口（`/publish/v1/{publication_id}`、`/p/{publication_id}`）作为只读存档继续有效；
3. 废弃路由 `/yaml`、`/script` 依然维持 HTTP 410 Gone。

---

### 5. 策略组顺序与主订阅透传

#### Requirement: 策略组顺序维护与导出保序 [FR-PUB-CLEAN-07]
1. `node_groups` 增加 `position INTEGER NOT NULL DEFAULT 0`；
2. 策略组查询、一致性快照与解析器（Resolver）按 `position ASC, created_at ASC, id ASC` 严格保序，导出的 Mihomo/Clash `proxy-groups:` 顺序与用户在控制台编排的顺序完全一致；
3. 策略组 API 支持调整顺序，前端策略组卡片支持上下移动与序号维护。

#### Requirement: 主订阅标识与流量透传 [FR-PUB-CLEAN-08]
1. `subscriptions` 增加 `is_primary INTEGER NOT NULL DEFAULT 0` 与 `userinfo TEXT NOT NULL DEFAULT ''`；
2. 上游拉取时解析 `Subscription-Userinfo` 头部并持久化入库；
3. 导出所有直链时，若存在启用的主订阅（`is_primary = 1 AND enabled = 1`）且其 `userinfo` 非空，响应头注入 `Subscription-Userinfo: <userinfo>`；
4. 订阅管理列表页支持单键“设为主订阅”（同一时间至多一个主订阅），卡片直观显示流量与到期信息；遵循 `.exp.md`，不在添加/编辑弹窗内塞主订阅开关。

---

## Acceptance Criteria (SC-PUB-MINIMAL)

- **SC-MIN-01 (极简直链)**: 客户端通过 `GET /clash`、`GET /singbox`、`GET /surge`、`GET /qx` 能够直接获取对应目标格式配置；
- **SC-MIN-02 (Settings Token 鉴权)**: 在系统启用 Token 时，带正确 `?token=admin_token` 返回 200，错误或缺失返回 401；在 Open Mode 下无参请求直接返回 200；
- **SC-MIN-03 (现场动态)**: 当修改节点前缀或禁用某订阅后，下一次请求 `/clash` 立即反映更新结果，无需重新“发布”；
- **SC-MIN-04 (条件缓存)**: 相同内容的二次请求带 `If-None-Match` 返回 304 Not Modified；
- **SC-MIN-05 (前端纯净)**: 前端页面没有任何 UUID 列表、Token 轮换、吊销弹窗或基址配置卡片，四行直链一目了然一键复制；
- **SC-MIN-06 (生产替换)**: 全部门禁通过后直接部署上线，公网域名 `https://sub.einck.top` 验证通过；
- **SC-MIN-07 (策略组保序)**: 用户编排策略组顺序后，导出的配置中 `proxy-groups` 严格按该顺序排列，不再按字母无序散乱；
- **SC-MIN-08 (主订阅流量透传)**: 设置主订阅后，拉取直链响应头包含主订阅的 `Subscription-Userinfo`，Clash Verge 等客户端可正确显示流量与到期时间。

---
feature: 004-publication
title: Publication Capability Specification
status: candidate
schema: speckit-native
---

# Publication Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的订阅发布与配置编译器能力。系统必须支持原生 Mihomo 完整拓扑导出与非 Mihomo 纯节点导出，提供专属 Token 与系统主令牌双重访问控制，并支持历史 `/p/{id}` 路径兼容路由，同时对遗留的 `/yaml` 与 `/script` 端点明确返回 410 Gone。

## User Scenarios & Priority

### User Story 1 - 原生 Mihomo 完整拓扑配置发布 (Priority: P1)
- **GIVEN** 管理员配置了多订阅节点、自定义策略组及分流规则
- **WHEN** 客户端使用 Mihomo User-Agent 请求发布端点
- **THEN** 编译器输出包含 proxies、proxy-groups、rule-providers 与 rules 的完整合法 YAML 配置

### User Story 2 - 非 Mihomo 纯节点安全导出 (Priority: P1)
- **GIVEN** 客户端使用传统 Clash 或通用代理工具请求订阅
- **WHEN** 目标格式为非 Mihomo 格式
- **THEN** 编译器仅导出经过启用法定作用域过滤的合法纯节点列表，不导出复杂嵌套组

### User Story 3 - 发布端点访问控制与遗留路由兼容 (Priority: P2)
- **GIVEN** 用户配置了公开分享直链或使用系统主令牌
- **WHEN** 访问包含专属 Token 或访问历史遗留端点
- **THEN** 系统正确鉴权并响应配置，对已废弃的遗留端点响应 410 Gone 明确告知客户端

---

## Requirements

### 1. 拓扑与纯节点导出编译

#### Requirement: 原生 Mihomo 完整拓扑发布导出 [FR-PUB-01] <a id="req-pub-mihomo-export"></a>
系统 SHALL 提供针对 Mihomo 内核的完整拓扑配置发布能力。编译器 MUST 输出合法的 YAML 格式，完整包含 `proxies`、`proxy-groups`、`rules` 及可选的 `rule-providers`。

##### Scenario: 完整导出 proxies, proxy-groups, rules <a id="scenario-pub-full-topology"></a>
- **GIVEN** 存在有效节点和嵌套策略组
- **WHEN** 客户端请求 Mihomo 发布端点
- **THEN** 响应包含格式完备的 YAML，经 Mihomo 语法校验退出码为 0

#### Requirement: 非 Mihomo 纯节点导出协议边界 [FR-PUB-02] <a id="req-pub-non-mihomo-export"></a>
当导出目标为非 Mihomo 格式（如 Base64 订阅或传统轻量客户端）时，编译器 SHALL 仅导出通过启用订阅作用域过滤的合法纯节点配置，严禁输出非标准扩展字段。

##### Scenario: 非 Mihomo 格式仅导出合法纯节点 <a id="scenario-pub-pure-nodes"></a>
- **GIVEN** 系统中存在复杂策略组与多协议节点
- **WHEN** 客户端以非 Mihomo 目标请求导出
- **THEN** 响应仅包含纯节点列表，协议符合目标客户端规范

#### Requirement: 非 Mihomo 遇到空策略组校验一致性 [FR-PUB-03] [NEEDS CLARIFICATION] <a id="req-pub-empty-group-valid"></a>
针对非 Mihomo 纯节点导出在处理引用了空策略组的规则时的行为边界，系统 SHALL 依据用户在 Q3 中的裁决落实一致性校验或安全忽略。

##### Scenario: 引用空组时的过滤或阻断 <a id="scenario-pub-empty-group-validation"></a>
- **GIVEN** 某些分流规则引用了成员为空的策略组
- **WHEN** 编译纯节点或简单规则配置
- **THEN** 系统按既定策略安全过滤该规则或发出编译警告，防止客户端解析崩溃

---

### 2. 发布端点安全与历史路由兼容

#### Requirement: 发布端点免密公开访问与专属 Token [FR-PUB-04] <a id="req-pub-auth-access"></a>
系统 SHALL 支持为每个发布配置生成独立的专属访问 Token。当导出端点鉴权开启且请求携带合法专属 Token 时，系统 MUST 允许免登录直接获取订阅配置。

##### Scenario: 专属 Token 访问有效配置 <a id="scenario-pub-token-verify"></a>
- **GIVEN** 发布配置拥有专属 Token `sec_abc123`
- **WHEN** 外部客户端请求 `/api/v1/publications/export?token=sec_abc123`
- **THEN** 服务端返回 200 OK 并下发配置内容，无需管理端 Cookie

#### Requirement: 导出端点统一系统主令牌复用 [FR-PUB-05] <a id="req-pub-master-token"></a>
当用户使用全局统一系统主令牌（System Master Token）访问任意有效发布端点时，鉴权中间件 MUST 判定授权通过，支持管理员便捷调试与多客户端聚合接入。

##### Scenario: 系统主令牌授权导出 <a id="scenario-pub-master-token-auth"></a>
- **GIVEN** 系统配置了主令牌 `MASTER_KEY_SECRET`
- **WHEN** 客户端请求带有 `token=MASTER_KEY_SECRET`
- **THEN** 无论该发布是否设置专属 Token，系统均允许访问并返回配置

#### Requirement: 历史 `/p/{id}` 路径兼容路由 [FR-PUB-06] <a id="req-pub-compat-routes"></a>
系统 SHALL 保留对历史短链接路由 `/p/{id}` 的兼容支持，能够正确路由或重定向至最新版本的发布导出处理器，不破坏既有客户端订阅链接。

##### Scenario: 历史短链接 `/p/{id}` 正常响应 <a id="scenario-pub-compat-p-route"></a>
- **GIVEN** 客户端使用历史短链 `/p/my-sub`
- **WHEN** 发起 HTTP GET 请求
- **THEN** 服务端正确匹配发布标识并输出对应配置

#### Requirement: 遗留历史路由返回 410 Gone <a id="req-pub-legacy-410"></a>
对于历史早期废弃的临时端点（如 `/yaml` 与 `/script`），系统 MUST 直接响应 HTTP 410 Gone，并在响应体中明确说明该遗留端点已被永久移除，防止旧脚本无限重试。

##### Scenario: 访问遗留 `/yaml` 返回 410 Gone <a id="scenario-pub-legacy-yaml-410"></a>
- **GIVEN** 历史客户端尝试访问已移除的旧端点
- **WHEN** 客户端请求 `GET /yaml`
- **THEN** 服务端直接响应 HTTP 410 Gone，告知端点已被永久废弃

##### Scenario: 访问遗留 `/script` 返回 410 Gone <a id="scenario-pub-legacy-script-410"></a>
- **GIVEN** 历史客户端尝试访问旧脚本端点
- **WHEN** 客户端请求 `GET /script`
- **THEN** 服务端直接响应 HTTP 410 Gone，内容说明该遗留端点已被永久移除

---

---

---

---

## Supplemental Retained Behavioral Capabilities & Historical Clauses

### Overview
本节完整收录历史变更中属于本领域的行为规范、具体契约（包含精确状态码、传输参数、请求头校验等）与验收场景，保持行为约束无损物理留存与点对点双向追溯。

### Retained Functional Requirements

#### Requirement: 发布中心刷新后复制链接保持完整令牌 [HR-016] <a id="req-pub-hr-016"></a>

当用户成功发布订阅配置后，系统 SHALL 在客户端安全持久化（localStorage 或与发布 ID 关联的键值缓存）记录该次发布返回的包含有效 `token` 的完整导出 URL。当用户刷新页面并重新进入发布中心时，点击“复制订阅链接”按钮 MUST 复制带有 `?token=...` 的有效可访问 URL，不得复制缺少鉴权令牌的裸路径。

##### Scenario: 用户刷新浏览器后复制已发布订阅链接 <a id="scenario-pub-hr-016-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 用户创建了发布配置，随后刷新浏览器页面并点击“复制订阅链接”
- **THEN** 剪贴板中获取的链接为包含当前有效 token 的完整 URL，客户端以此 URL 可直接通过鉴权下载配置

#### Requirement: 发布异步响应隔离当前目标 [HR-017] <a id="req-pub-hr-017"></a>

用户选中的编译目标 SHALL 不被其它目标的延迟发布或预览响应反选。成功发布结果 MUST 按请求目标保存并可在切回该目标时恢复，且只有当前选中目标的最新请求能更新可见发布信息、预览或错误。

##### Scenario: 发布 Mihomo 时切换 sing-box 后才收到成功响应 <a id="scenario-pub-hr-017-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** Mihomo 发布请求仍未返回，用户切换至 sing-box 后收到 Mihomo 成功响应
- **THEN** 页面仍选中 sing-box 且不显示 Mihomo 发布信息，切回 Mihomo 时可恢复这条发布信息与带令牌链接

#### Requirement: 直接基于快照节点的四目标编译与明文发布 [HR-055] <a id="req-pub-hr-055"></a>

策略解析生成的 `ResolvedNode` SHALL 直接包含节点的 `Server`、`Port` 与 `Credentials` 明文配置。四目标编译器（Mihomo、sing-box、Surge、Quantumult X）调用 `Compile(ctx, snapshot, target)` 时 SHALL 直接从 `snapshot.Nodes[i]` 读取明文配置，仅校验协议必填字段非空与格式合法（如 Shadowsocks 的 cipher 与 password、VMess/VLESS 的 uuid、Trojan/Hysteria2 的 password、WireGuard 的 private_key/public_key/local_address CIDR、TUIC 的 uuid 与 password），SHALL NOT 要求外部注入凭据映射或执行 `VerifiedNodeIdentity` 交叉核验。发布服务在 `Publish` 时 SHALL 将编译生成的明文 `Content` 直接存入 `publications` 表；`ResolveAndServe` 在校验导出 Token 与未撤销状态后 SHALL 直接返回 `pub.Content`（若旧记录 `Content` 为空则实时编译返回）。

##### Scenario: 直接从快照编译与明文发布下载 <a id="scenario-pub-hr-055-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 管理员针对包含合法明文节点的策略快照请求预览、创建发布并通过有效导出 Token 下载配置
- **THEN** 编译器直接从快照节点渲染目标配置，`publications` 表直接保存明文 `content`，客户端下载无需解密或凭据绑定校验即可获得完整配置文本

##### Scenario: 协议必填字段缺失被编译拒绝 <a id="scenario-pub-hr-055-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 快照中的节点缺少对应协议必填字段（如 Shadowsocks 缺少 cipher 或 password、WireGuard 缺少 private_key/public_key/local_address）并请求编译
- **THEN** 编译器返回明确的 `CapabilityError` 拒绝导出，不生成残缺配置

#### Requirement: 全局与订阅级出站代理分级解析与调度一致性 [HR-196] <a id="req-pub-hr-196"></a>

系统 SHALL 支持通过全局受控配置与单订阅独立配置指定显式 HTTP 出站代理（`http://`），并确保手动刷新与定时调度走完全一致的代理选择逻辑：
1. 优先级顺序 SHALL 严格遵循：单订阅级 `refresh_policy.fetch_proxy_ref` 优先 > 全局受控配置（`CSP_FETCH_PROXY` / 命令行参数 `-fetch-proxy`）> 未配置时使用直连。
2. 调度执行器与手动刷新入口在调用 `inventory.ReconcileSubscription` 时，SHALL 统一应用上述代理选择结果。
3. 在版本受控的 `docker-compose.example.yml` 中，系统 SHALL 提供 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts` 样例，宿主机环境的 `.gitignore` 忽略文件 `docker-compose.yml` 遵循核验与投产备份规程。

##### Scenario: 订阅未指定代理时自动继承全局受控代理 <a id="scenario-pub-hr-196-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 全局配置了 `CSP_FETCH_PROXY=http://host.docker.internal:7890` 且订阅未设置 `fetch_proxy_ref`
- **THEN** 订阅拉取操作自动经由 `http://host.docker.internal:7890` 代理执行请求

##### Scenario: 订阅配置了特定代理时覆盖全局代理 <a id="scenario-pub-hr-196-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 全局配置了 `CSP_FETCH_PROXY` 但订阅明确指定了合法的 `fetch_proxy_ref=http://custom-proxy.example.com:8080`
- **THEN** 订阅拉取操作优先经由 `http://custom-proxy.example.com:8080` 执行

##### Scenario: 未配置任何代理时保持安全直连 <a id="scenario-pub-hr-196-3"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 全局 `CSP_FETCH_PROXY` 为空且订阅 `fetch_proxy_ref` 亦为空
- **THEN** 订阅拉取操作通过直连模式发起，不继承宿主机环境变量中的不可控代理

#### Requirement: 严格分离式 SSRF 目标防御、代理端点精确准入与应用层目标 IP 强制绑定 [HR-197] <a id="req-pub-hr-197"></a>

系统 SHALL 对订阅目标 URL 与出站代理端点执行严格的分离式安全准入、失败闭合控制及应用层目标 IP 强制绑定，**SHALL NOT 依赖上游代理（如 mihomo）的 ACL 阻断私网**：
1. **订阅目标端点与每跳重定向预解析校验**：
   - 初始目标 URL 及任何重定向跳转（`301/302/303/307/308`）的目标地址，SHALL 在发起建连或向代理发送请求前，由客户端本地解析 DNS 并逐一校验全部解析出的 IP。
   - 若目标解析到任何私有网段（RFC 1918）、回环地址（`127.0.0.0/8`、`::1`）、链路本地地址（`169.254.0.0/16`、`fe80::/10`，含云厂商元数据）、共享地址空间（`100.64.0.0/10`）、多播、文档测试网段或未指定地址，SHALL 立即终止请求并返回 `ssrf_blocked` 安全错误。
   - 当目标域名 DNS 解析失败或无可用记录时，SHALL 立即失败闭合返回 `dns_resolution_failed`，严禁在解析失败时将未验证域名转发给代理。
2. **应用层强制绑定经过公网校验的目标 IP 给代理**：
   - **HTTP 正向代理目标绑定**：当通过 HTTP 代理抓取 `http://` 目标时，客户端发送给代理的请求行绝对 URL SHALL 强制替换为已验证的公网 `pinnedIP:port`（即 `GET http://<pinnedIP>:<port>/path`），同时 `Host` 请求头 SHALL 保留原始目标域名（`Host: <original-domain>`）。
   - **HTTPS 代理 CONNECT 目标绑定与 TLS 原域名验证**：当通过 HTTP 代理抓取 `https://` 目标时，客户端发送给代理的 `CONNECT` 请求权威目标与 `Host` 头 SHALL 强制使用已验证的公网 `pinnedIP:port`（即 `CONNECT <pinnedIP>:<port> HTTP/1.1`），严禁向代理发送未验证的原始域名；代理隧道建立后，客户端在 TLS 握手中 SHALL 设置 `ServerName`（SNI）为原始目标域名，并严格校验服务端证书与原始目标域名匹配（SHALL NOT 开启 `InsecureSkipVerify`）。
   - **重定向每跳重新校验与重新绑定**：每次 `301/302/303/307/308` 重定向跳转 SHALL 独立重新执行本地 DNS 解析、公网 SSRF 校验、目标 IP 绑定及 TLS `ServerName` 更新，禁止任何一跳将未校验域名交给代理二次解析。
   - **路由行为与 TOCTOU 边界明示**：因代理在 HTTP 请求行与 HTTPS `CONNECT` 权威目标中仅接收已验证的公网 IP，若上游代理配置了仅依赖请求行/`CONNECT` 域名匹配的路由规则（未开启 TLS SNI 嗅探），其分流路线将按 IP/默认规则匹配；系统如实说明并测试该行为，且不虚假宣称完全消除互联网公网 IP 路由重分配层面的极端 TOCTOU。
3. **代理端点独立准入校验与白名单精确规范化**：
   - 代理端点协议 SHALL 严格限定为显式 `http://`；任何其他协议（如 `socks5`、`https`、`ftp`）或省略协议头 SHALL 立即失败闭合返回 `invalid_proxy_url`。
   - 受信任代理白名单（`AllowedProxyHosts`）SHALL 仅存储并匹配规范化的完整 `http://<canonical-host>:<canonical-port>`，严禁丢弃 scheme 或按裸 `host:port` 匹配导致同端口的 `socks5`/`https` 代理获豁免。
   - 代理 URL 规范化与拒绝规则：
     - 端口 SHALL 显式指定（1..65535）且严禁包含前导零（如 `:07890` 返回 `invalid_proxy_url`）；
     - 代理主机名严禁包含尾随点（如 `host.docker.internal.` 返回 `invalid_proxy_url`，IP 尾随点如 `127.0.0.1.` 返回 `proxy_ssrf_blocked`）；
     - 严禁包含 userinfo 凭据、URL 路径、查询字符串、片段或 IPv6 Zone ID（`%`）；
     - 严禁混淆 IP 格式（八进制、十六进制、整型、短 IP、IPv4-mapped IPv6 `::ffff:...`）；标准 IPv6 地址 SHALL 规范化为 RFC 5952 方括号格式 `http://[<canonical-ipv6>]:<port>`。
   - 受信任代理连接阶段 SHALL 仅豁免精确匹配 `http://<canonical-host>:<canonical-port>` 的端点，严禁将白名单扩大至同主机的其他端口（如 `:2375`、`:6379`、`:22`）。
   - 单订阅中任意用户输入的 `fetch_proxy_ref` 绝不能扩展白名单；其指向私网、回环、云元数据地址或网关私有 IP 等价地址时，SHALL 立即返回 `proxy_ssrf_blocked` 拒绝执行；合法的公网自定义代理同样绑定其校验后的公网代理 IP 进行建连。

##### Scenario: 离线恶意 HTTP 代理仅见已校验公网 IP 且不见未校验域名 <a id="scenario-pub-hr-197-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 客户端通过受信任代理抓取 `http://sub.example.com/feed` 与 `https://sub.example.com/feed`（本地 DNS 解析为公网 IP `93.184.216.34`）
- **THEN** 代理捕获到的 HTTP 请求行绝对 URL 为 `http://93.184.216.34:80/feed`、HTTPS `CONNECT` 权威目标为 `93.184.216.34:443`（均不见 `sub.example.com`），而源站收到的 `Host` 头与 TLS SNI 仍为 `sub.example.com` 且完成严格证书校验

##### Scenario: 代理侧 DNS 投毒指向宿主机回环时无法访问内网 <a id="scenario-pub-hr-197-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 上游代理自身 DNS 被投毒（将 `sub.example.com` 解析至 `127.0.0.1:18080` 内网服务）且私网规则为 `DIRECT`，客户端通过该代理拉取 `http://sub.example.com/feed` 或 `https://sub.example.com/feed`
- **THEN** 客户端向代理强制传递预校验的公网 IP 而非域名，代理不会触发针对 `sub.example.com` 的投毒 DNS 解析，内网 `127.0.0.1:18080` 零命中

##### Scenario: 目标 URL 指向回环或私网时即使配置了代理仍被拦截 <a id="scenario-pub-hr-197-3"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 客户端配置了全局代理 `http://host.docker.internal:7890`，但拉取目标 URL 为 `http://127.0.0.1:8080/feed` 或 `http://169.254.169.254/latest/meta-data`
- **THEN** 系统在预检阶段即刻阻断请求，返回 `ssrf_blocked` 领域安全错误，不向代理发出任何网络连接

##### Scenario: 重定向跳转每跳重新绑定公网 IP 且跳转内网被即时阻断 <a id="scenario-pub-hr-197-4"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 订阅目标返回 302 重定向至另一公网域名或重定向至 `http://10.0.0.1:8080/internal`
- **THEN** 公网重定向跳重新解析并向代理绑定新跳的公网 IP 及更新 TLS `ServerName`；指向内网的重定向在本地预检阶段即刻返回 `ssrf_blocked` 阻断，不向代理发起连接

##### Scenario: 同端口非 HTTP 协议代理与畸形端口/尾随点被严格拒绝 <a id="scenario-pub-hr-197-5"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 全局受信任代理为 `http://host.docker.internal:7890`，请求指定 `socks5://host.docker.internal:7890`、`https://host.docker.internal:7890`、`http://host.docker.internal:07890` 或 `http://host.docker.internal.:7890`
- **THEN** 系统判定协议不支持或端点格式违规，立即返回 `invalid_proxy_url` 失败闭合，严禁因同端口而豁免

##### Scenario: 受信任精确代理端点获准但危险端口被严格拒绝 <a id="scenario-pub-hr-197-6"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 全局受信任代理为 `http://host.docker.internal:7890`，用户或订阅请求指定 `http://host.docker.internal:2375`、`:6379` 或 `:22`
- **THEN** 系统判定端口不在受信任精确白名单内，回退至 SSRF 校验并检测到内网网关 IP，立即返回 `proxy_ssrf_blocked` 阻断

##### Scenario: 网关私有 IP 等价绕过被拦截 <a id="scenario-pub-hr-197-7"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 用户尝试在订阅级代理中提供 `host.docker.internal` 解析后的网关私有 IP（例如 `http://172.17.0.1:7890`）
- **THEN** 系统检测到该 IP 属于私有 RFC 1918 范围且未处于显式白名单，返回 `proxy_ssrf_blocked` 阻断

##### Scenario: 用户自定义代理指向内网元数据或回环被拒绝 <a id="scenario-pub-hr-197-8"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 订阅设置了 `fetch_proxy_ref=http://169.254.169.254:80` 或 `http://127.0.0.1:7890`
- **THEN** 系统判定代理端点违规，返回 `proxy_ssrf_blocked` 错误，拒绝建立代理连接

##### Scenario: 端口省略、userinfo 与混淆 IP 被严格拒绝 <a id="scenario-pub-hr-197-9"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 订阅设置代理为 `http://host.docker.internal`、`http://user:pass@host.docker.internal:7890` 或 `http://0177.0.0.1:7890`
- **THEN** 系统拒绝该代理端点并返回校验或安全错误，失败闭合

#### Requirement: 代理故障分类、安全脱敏与审计记录 [HR-198] <a id="req-pub-hr-198"></a>

当代理连接失败、超时或遇到网络异常时，系统 SHALL 生成清晰结构化错误并在持久化审计中进行脱敏记录：
1. 代理网络超时或连接拒绝等底层异常 SHALL 正确分类为 `fetch_failed` 或 `fetch_timeout` 等领域错误。
2. 订阅拉取审计表 `subscription_fetches` 中记录的 `redacted_error` SHALL 经过敏感凭据与 Token 脱敏，严禁泄露 URL 中的 Token、Secret 或内部认证参数。
3. 单次刷新失败 SHALL 正常记录 `outcome = failed`，不得因报错导致脏状态遗留或未写入完成状态。

##### Scenario: 代理端口无法连接时分类为失败并记录脱敏审计 <a id="scenario-pub-hr-198-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 全局代理配置了不可达的端口导致连接拒绝
- **THEN** `ReconcileSubscription` 捕获该错误并返回失败摘要，在 `subscription_fetches` 中写入 `outcome = failed` 且错误信息已剔除敏感凭据

#### Requirement: 策略拓扑图校验接口无参直接校验后端持久化数据契约 [HR-199] <a id="req-pub-hr-199"></a>

后端 `POST /api/v1/policies/validate`（及 `/api/v1/policy/validate`）与前端 `validateGraph()` SHALL 直接基于后端数据库持久化状态执行拓扑校验，无需前端重复回传节点组、边或规则载荷：
1. 后端 `policyHandler.validate` SHALL 直接调用 `h.service.ValidateGraph(r.Context())` 从 SQLite 数据库加载全量策略组、边连接关系及当前活动修订的分流与准入规则执行完整拓扑校验，移除冗余的 `validateGraphRequest` 请求体解析分支。
2. 前端 `web/src/features/policy/usePolicy.ts` 的 `validateGraph()` SHALL 直接以无请求体方式调用 `api.post<ValidationResult>('/api/v1/policies/validate')`，并在校验通过或发现环路/自环/非法引用时准确更新 `validationResult`。

##### Scenario: 前端点击校验拓扑图无参触发后端数据库权威拓扑校验 <a id="scenario-pub-hr-199-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 用户在策略树页面点击“校验拓扑图”，前端向 `POST /api/v1/policies/validate` 发起无参请求，且后端数据库中的策略组、边与规则无环路或非法引用
- **THEN** 后端直接从数据库加载完整拓扑图与规则完成校验，返回 HTTP 200 及 `{"data":{"valid":true}}`

##### Scenario: 后端数据库存在拓扑环路时返回结构化校验错误 <a id="scenario-pub-hr-199-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 客户端调用 `POST /api/v1/policies/validate` 且后端持久化的策略组边关系存在环路或非法引用
- **THEN** 后端返回单个 HTTP 422 领域校验错误响应（如 `cycle_detected`）

#### Requirement: 仅 Mihomo 导出完整配置、其他目标仅导出纯节点格式并忽略规则 [HR-200] <a id="req-pub-hr-200"></a>

编译器与发布服务 SHALL 严格区分完整配置目标（`mihomo`）与纯节点格式导出目标（`singbox`、`surge`、`qx`）：
1. **Mihomo 完整配置目标（`target == "mihomo"`）**：编译器 SHALL 校验并渲染完整的节点（`proxies`）、策略组（`proxy-groups`）、规则集提供者（`rule-providers`）、分流规则（`rules`）与 `dns` 配置；当路由指向空策略组（`empty_routed_group`）或策略组/规则不合法时，编译器与发布预检 SHALL 正常拦截。
2. **非 Mihomo 纯节点导出目标（`target` 为 `singbox`、`surge` 或 `qx`）**：
   - `compiler.validate` SHALL 仅校验 `snapshot.Nodes` 的节点协议支持度与凭据完整性，SHALL 完全忽略 `snapshot.Groups` 与 `snapshot.Rules`（即使快照中包含 `fallback`/`loadbalance` 策略组、空策略组或 `GEOSITE`/`PROCESS-NAME`/`RULE-SET` 等任意路由规则，也绝不报错）。
   - `renderSingBox` SHALL 仅输出纯节点 JSON 配置（仅包含节点 `outbounds` 与 WireGuard `endpoints`，不含策略组 selector/urltest outbound，不含 `route` 路由段落），且输出 JSON 必须通过官方 `sing-box check` 校验。
   - `renderSurge` SHALL 仅输出纯节点行（每行一个 `<name> = <protocol>, <server>, <port>, ...`，不含 `[General]`、`[Proxy]`、`[Proxy Group]`、`[Rule]` 段落），并支持渲染 `ss`、`vmess`、`trojan`、`hysteria2`、`tuic`、`wireguard` 节点行（对 Surge 不支持的 `vless` 协议保持节点级 `CapabilityError`）。
   - `renderQuantumultX` SHALL 仅输出纯节点行（每行一个 `<protocol> = <server>:<port>, ..., tag=<name>`，不含 `[general]`、`[server_local]`、`[policy]`、`[filter_local]` 段落），支持渲染 `ss`、`vmess`、`trojan` 节点行。
   - 发布预检 `evaluatePreflight` 在 `target != "mihomo"` 时 SHALL 忽略 `empty_routed_group` 等策略组/规则诊断，不因策略组或规则阻断预览或发布。
3. 前端发布页（`web/src/features/publications/`）与策略树页（`web/src/features/policy/`）SHALL 明确标识 `Mihomo` 为完整配置导出（含策略组与分流规则），`sing-box`、`Surge`、`Quantumult X` 为纯节点格式导出（忽略策略组与规则）。

##### Scenario: 快照含复杂策略组与 Mihomo 专属规则时导出非 Mihomo 纯节点格式 <a id="scenario-pub-hr-200-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 当前策略快照包含 `fallback`、`loadbalance` 策略组以及 `GEOSITE`、`PROCESS-NAME`、`RULE-SET` 路由规则，管理员选择 `singbox`、`surge`（协议受支持）或 `qx`（协议受支持）执行预览或创建发布
- **THEN** 预览与发布均返回 HTTP 200 成功，不报策略组或规则不支持错误；`singbox` 仅输出节点 `outbounds`/`endpoints` JSON，`surge` 与 `qx` 仅输出纯节点文本行，不含任何策略组或路由规则段落

##### Scenario: 空路由策略组仅阻断 Mihomo 而不阻断纯节点导出目标 <a id="scenario-pub-hr-200-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 快照存在路由规则指向 0 节点的空策略组（产生 `empty_routed_group` 诊断）
- **THEN** `mihomo` 目标的发布预检与发布被阻断（HTTP 409 `publication_preflight_rejected`），而 `singbox`、`surge`、`qx` 纯节点导出目标的预检与发布正常通过

##### Scenario: Mihomo 目标继续导出含策略组与规则的完整配置 <a id="scenario-pub-hr-200-3"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 管理员选择 `mihomo` 目标执行预览或发布下载
- **THEN** 导出内容包含完整的 `proxies`、`proxy-groups`、`rules`（及必要的 `rule-providers` 与 `dns`）段落

#### Requirement: 前端默认简体中文（`zh-CN`）与全站完整中文化 [HR-201] <a id="req-pub-hr-201"></a>

前端国际化体系 SHALL 将默认语言设置为简体中文（`'zh-CN'`），并完成全站所有界面组件、状态徽章、枚举标签、表单校验与错误提示的中文化：
1. `web/src/locales/index.ts` 在本地存储未显式设置语言时 SHALL 默认使用 `'zh-CN'`，且 `fallbackLocale` SHALL 为 `'zh-CN'`。
2. `web/src/locales/`（含 `zh-CN.ts`、`en-US.ts` 及 `messages.ts`）SHALL 完整覆盖导航栏、控制台、订阅源、节点账本、探针引擎、策略树、订阅发布、全局设置、鉴权门禁及通用错误卡片的全部词条。
3. `App.vue`、`ErrorStateCard.vue`、`DashboardView.vue`、`PolicyView.vue`、`PolicyEditorSheet.vue`、`GroupCard.vue`、`usePolicy.ts`、`policyTypes.ts`、`NodesView.vue`、`nodeView.ts`、`useNodes.ts`、`ProbesView.vue`、`ProbeRunCard.vue`、`ProbeEvidenceSheet.vue`、`useProbes.ts`、`probeTypes.ts`、`PublicationsView.vue`、`usePublications.ts`、`publicationTypes.ts`、`SubscriptionsView.vue`、`SubscriptionConfigDrawer.vue`、`useSubscriptions.ts`、`SettingsView.vue` SHALL 移除硬编码英文文案，在 `'zh-CN'` 模式下将策略组类型（手动选择/自动测速/故障转移/负载均衡）、规则动作（允许/拒绝/隔离）、探针类型与状态、探测结论、风险等级、预检诊断与常见 API 错误码统一展示为清晰地道的中文。

##### Scenario: 首次访问默认展示全中文界面 <a id="scenario-pub-hr-201-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 用户在未设置 `localStorage.csp_locale` 的全新浏览器环境中打开系统各页面（控制台、订阅源、节点账本、探针引擎、策略树、订阅发布、全局配置）
- **THEN** 页面标题、导航菜单、按钮、抽屉弹窗、筛选器、空状态、徽章与提示信息全部以简体中文展示，无残留硬编码英文段落或裸露英文枚举

##### Scenario: 错误状态卡片与拓扑校验反馈中文展示 <a id="scenario-pub-hr-201-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 接口返回鉴权错误、冲突错误、校验错误或在策略树触发拓扑校验反馈
- **THEN** `ErrorStateCard.vue` 与策略树提示横幅展示中文标题、中文错误说明及中文操作按钮（“重试”、“前往设置 / 鉴权”）

#### Requirement: 统一节点凭据与不可伪造端点绑定 [HR-223] <a id="req-pub-hr-223"></a>

系统 SHALL 保留 WireGuard 的 local_address/private_key/public_key/pre_shared_key/reserved/mtu/dns、TUIC 的 uuid/password/congestion_control/udp_relay_mode/alpn/sni/disable_sni、VLESS Reality 的 pbk/sid/fp/flow 与 Hysteria2 的 up/down/obfs/obfs-password；WG 缺 private_key/public_key/local_address 或 TUIC 缺 uuid/password SHALL 拒绝。YAML/URI SHALL 通过单一语义提取。编译和发布 SHALL 使用可信解析/持久节点身份与加密凭据的版本化绑定核对 protocol、logicalID、server、port 和身份 transport；不得仅凭待验 payload 自证身份，亦不得简单反算不完整 transport 导致真实 URL 被误拒。认证失败 SHALL 在任何目标导出前拒绝。

##### Scenario: 双格式合法输入及恶意端点 <a id="scenario-pub-hr-223-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 受支持 YAML/URI 的真实节点经过解析、保存、编译，与另一个保持相同 logicalID、protocol、version 却替换 payload server/port 的节点分别请求预览/发布
- **THEN** 合法节点均使用真实凭据导出，恶意载荷在预览/发布/重启分发均被拒绝，错误不暴露秘密

##### Scenario: 不完整必需字段或旧身份资料 <a id="scenario-pub-hr-223-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** WG/TUIC 缺少必需字段，或旧凭据行缺少可验证身份绑定
- **THEN** 不伪造字段或根据密文自身补信任，明确拒绝并保持旧行原状

#### Requirement: 四目标能力与规则资源 fail closed [HR-224] <a id="req-pub-hr-224"></a>

系统 SHALL 提供 Mihomo、sing-box、Surge、Quantumult X。Mihomo SHALL 以真实参数渲染 SS/VMess/VLESS/Trojan/Hysteria2/WireGuard/TUIC、select/url-test/fallback/load-balance 与十四种受支持规则；sing-box SHALL 使用官方选项格式，Surge/QX SHALL 仅渲染各自可表达子集。不支持的节点/组/规则 SHALL 定点拒绝而非使用 logicalID、固定端口或假参数。Mihomo RULE-SET SHALL 仅引用显式有效 URL 或已配置的真实 provider，不得生成占位资源。

##### Scenario: 合法四目标与不支持组合 <a id="scenario-pub-hr-224-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 完整凭据和有效规则分别编译各目标或请求目标不支持组合
- **THEN** 可支持输出含真实连接/引用且接受相应官方校验，不可支持组合返回定位错误且不输出部分配置

##### Scenario: 缺失 RULE-SET provider <a id="scenario-pub-hr-224-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** Mihomo 规则只声明 RULE-SET 名字而不存在显式 URL 或已配置 provider
- **THEN** 编译拒绝并指出规则位置，输出绝不含 `ruleset.invalid` 或其他猜测 URL

#### Requirement: 四目标受保护发布与不可变持久恢复 [HR-226] <a id="req-pub-hr-226"></a>

系统 SHALL 在 Preview、Publish、ResolveAndServe 核对可信节点身份和凭据；保留 token/撤销/CSRF/风险预检。发布 SHALL 原子持久保存工件加密字节、内容摘要、目标、编译版本及版本化凭据绑定，重启下载 SHALL 核对认证和字节摘要，不以进程 map、前百条审计文本、当前订阅内容猜测原工件。缺绑定的历史发布 SHALL 拒绝分发而非假恢复。

##### Scenario: 重启、漂移、撤销与历史行 <a id="scenario-pub-hr-226-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 发布后重启、刷新源凭据或发生工件篡改，再访问有效 token/错误 token/撤销 token/旧无绑定记录
- **THEN** 仅真实保存且验证过的不可变工件可下载，内容摘要一致；错误凭据/被撤销/损坏/历史无绑定一律拒绝且不泄密

#### Requirement: 风险读模型最新观测正确且有界 [HR-227] <a id="req-pub-hr-227"></a>

系统 SHALL 对策略允许的多个 provider 每节点选取按 observed_at DESC、id DESC 确定的唯一最新观测；分页总数 SHALL 是匹配的节点数而非观测 join 行数，风险过滤/状态与详情一致；分页查询 SHALL 不扫描每节点全部历史后在 Go 中选最大。

##### Scenario: 多 provider 同时刻与历史增长 <a id="scenario-pub-hr-227-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 一个节点有多个 provider 同时间戳记录，且其他节点有大量旧观测，按风险条件分页
- **THEN** 每节点只出现一次、count 等于实际匹配节点数、排序稳定且最新记录按 id 打破平局；查询计划使用限定节点/索引的最新观测路径

#### Requirement: 删除旧兼容并完成隔离验收 [HR-228] <a id="req-pub-hr-228"></a>

系统 SHALL 不提供 legacy 导入/查看、旧 `/yaml/*`、`/script/*`、`/api/*` 专用 410、Clash/retired 重发入口、旧过滤别名或 skeleton 假回退；历史行不自动改写。端到端验收 SHALL 采用临时数据、官方客户端/规范验证及真实隔离实例，包含节点详情/编辑、预览、发布、下载、撤销和多视口交互；不得触生产。

##### Scenario: 清场与真实交互 <a id="scenario-pub-hr-228-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 客户端调用旧路径/目标或管理员在本地实例操作 WG/TUIC 编辑与四目标出版
- **THEN** 旧兼容不提供假成功，新流程仅真实可保存、可校验、可撤销；独立审查及黑盒验收通过前不宣称交付

#### Requirement: 统一消费模型与 Mihomo xhttp 官方代码对齐 [HR-229] <a id="req-pub-hr-229"></a>

系统中的探针拨测模块（`internal/probe/mihomo`）与发布编译器模块（`internal/compiler`）MUST 严格基于同一份 `effective_config_json` 进行下游出站渲染与转换。针对支持现代传输协议（如 VLESS `xhttp` / Splithttp）的底层运行时（Mihomo v1.19.32+），系统 MUST 解除编译白名单限制，并严格依照官方支持参数进行渲染：
1. 仅将官方支持的字段（`path`, `host`, `mode`, `headers`）渲染为 `xhttp-opts`；
2. 未知 extra 字段完整保存在内部扩展属性中并输出诊断提示，**严禁盲传给 Mihomo 渲染器**；
3. 对于不支持 `xhttp` 的目标运行时（如 sing-box），系统如实返回 `unsupported_target_capability` 诊断，严禁伪造假协议转换。

##### Scenario: VLESS xhttp 节点在 Mihomo 目标下成功导出 <a id="scenario-pub-hr-229-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 包含 `network: "xhttp"` 的 VLESS 节点请求 Mihomo 格式预览
- **THEN** 编译器仅渲染官方支持的 `path`, `host`, `mode`, `headers` 节点块，未知 extra 不盲传，无 422 报错

#### Requirement: IP Risk Nullable 1:N 统一测量与策略规则快照 [HR-230] <a id="req-pub-hr-230"></a>

系统 SHALL 建立 IP Risk 的 1:N 关联模型：
1. 细粒度服务商评分记录（`ip_risk_observations`）的外键 `probe_observation_id` **严格且仅对应 `kind = 'ip_risk'` 的通用探针观测记录**，绝对不是 Baseline 连通性测活的“一对多”（严禁混淆 Baseline 测活与 IP Risk 威胁情报，两者属于正交维度）；
2. 跨表 kind 约束采用应用层事务校验（Application Transaction Validation）结合标准 SQLite 外键（`probe_observation_id REFERENCES probe_observations(id) ON DELETE SET NULL`）。在写入或更新 `ip_risk_observations` 时，事务显式校验被引用的探针观测其 `kind == 'ip_risk'`，若关联至 `kind = 'baseline'` 或其他非 ip_risk 类型则直接阻断并报错 `invalid_probe_kind_for_ip_risk`，不自造复杂触发器平台；
3. 历史独立服务商风险记录外键 `probe_observation_id` 允许为 `NULL`，严禁凭空构造伪造的 `probe_runs` 或观测记录，多个 provider 记录完整保全不丢失；
4. Baseline 测活与 IP Risk 详情解耦；历史风险记录仅在连接版本明确已知时如实回填；
5. 利用现有 `risk_policy_revisions` 表的 `rules_digest` 字段锁定策略版本快照，避免多表版本膨胀。

##### Scenario: IP Risk Nullable 1:N 统一关联生效 <a id="scenario-pub-hr-230-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 探针调度器完成某节点的 IP 风险查询并获取 2 个服务商情报
- **THEN** 系统写入 1 条通用 `kind = 'ip_risk'` 的 `probe_observations` 记录与 2 条 `ip_risk_observations` 记录，后两者外键均指向该通用观测 ID

##### Scenario: 关联错误探针 kind 被应用层事务校验阻断 <a id="scenario-pub-hr-230-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 尝试将 `ip_risk_observations.probe_observation_id` 指向一条 `kind = 'baseline'` 的观测记录
- **THEN** 系统应用层事务校验抛出 `invalid_probe_kind_for_ip_risk` 错误并回滚事务，拒绝非 ip_risk 类型关联

##### Scenario: 历史独立风险记录保持 NULL 关联 <a id="scenario-pub-hr-230-3"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 录入或迁移无对应通用探针运行的历史独立服务商风险评分
- **THEN** `probe_observation_id` 为 NULL 成功持久化，不伪造 probe_run，多 provider 数据无损保全

#### Requirement: Safe Detail JSON 白名单防泄密机制 [HR-231] <a id="req-pub-hr-231"></a>

系统在记录探针详细失败原因时，SHALL 在 `safe_detail_json` 中实施严格的 Allowlist 白名单机制：
1. 仅允许收集 `stage`, `code`, `target_core_version`, `protocol`, `transport`, `http_status`, `timeout_ms`, `reason`, `server_redacted`；
2. 严禁捕获原生 `err.Error()` 或完整 HTTP 响应体；
3. 绝对排除密码、UUID、Token、私钥、Authorization 头部、Cookie 头部及 Query 参数中的敏感 Secret；
4. 历史遗留缺失 detail 保留为 `unknown`，不伪造定论。

##### Scenario: 探针失败记录自动过滤敏感凭据 <a id="scenario-pub-hr-231-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 探针执行发生认证失败，底层错误包含包含密钥的 URL
- **THEN** `safe_detail_json` 仅记录 stage 与 code，将 server 脱敏，绝不输出密码与私密参数

#### Requirement: Publication 两阶段生命周期、清单与 Payload 物理防删引用 [HR-232] <a id="req-pub-hr-232"></a>

系统在执行发布编译与导出（`/api/v1/publications/*`）时，SHALL 实施两阶段快照生命周期与物理引用保护：
1. **Preview 阶段**: 系统创建不可变 draft 记录并计算完整清单（Manifest JSON，含各节点 included/excluded 理由、payload_ids、version_refs、产物哈希），向 `publication_payload_refs` 插入关联记录；
2. **Publish 阶段**: 发布请求 MUST 显式传递 `snapshot_id`，系统严格直接激活该快照已编译的字节，**绝不重新读取当前动态节点库**，杜绝时间差库存漂移；
3. **Payload 物理防删**: `publication_payload_refs` 外键 `ON DELETE RESTRICT` 物理阻止删除正在被草稿或激活发布引用的 Payload；
4. **Strict 模式 (默认)**: 遇到不支持节点返回 HTTP 422 及完整诊断清单，Draft 标记为不可发布（`publishable = 0`）；
5. **Compatible 模式**: 系统根据规则安全过滤不支持节点并在清单中详实记录排除原因。重新校验策略组引用：若任何策略组的成员节点数变为 0，系统拒绝生成并明确报错，**严禁自动回退到 DIRECT**，杜绝用户真实流量非预期泄露。

##### Scenario: 显式兼容模式安全排除不兼容节点且防止 DIRECT 穿透 <a id="scenario-pub-hr-232-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 导出目标为 sing-box 且请求显式携带 `mode=compatible`，某策略组过滤后剩余节点为 0
- **THEN** 编译器拒绝发布并报错 `empty_group_not_allowed`，不回退为 DIRECT

#### Requirement: 探针观测元数据扩展与陈旧状态派生 [HR-233] <a id="req-pub-hr-233"></a>

系统 SHALL 为 `probe_observations` 扩展 `measurement_id`、`engine_version`、`parser_version`、`mapping_version`、`stage`、`error_code` 与 `safe_detail_json` 字段。探针记录采用纯追加模式（Append-Only），严禁原地覆写历史观测的时间戳或裁决。读模型在查询时，若观测记录的 `connection_revision` 不等于 `node_connection_heads.connection_revision` 或观测时间超过 TTL，动态将其派生标为 `stale`。

##### Scenario: 连接参数更新后历史观测动态派生为 stale <a id="scenario-pub-hr-233-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 某节点的连接版本从 1 递增至 2，客户端请求该节点最新状态
- **THEN** 读模型发现版本不一致，将基于版本 1 的观测记录动态标记为 stale，不展示虚假可用

#### Requirement: 前端错误卡片纠偏治理 (Path: `web/src/ui/ErrorStateCard.vue`) [HR-234] <a id="req-pub-hr-234"></a>

Web 前端（`web/src/ui/ErrorStateCard.vue`）SHALL 优先基于 HTTP 状态码与结构化业务 `code` 进行错误分类展示。当且仅当底层发生真实网络中断或网关超时时，方可分类为“网络失联”；对于 HTTP 422 或 `code === 'unsupported_target_capability'` 错误，前端 MUST 准确展示为“协议与目标能力不兼容”，严禁因错误消息中包含 `"network"` 字样而落入 `isNetwork` 误判为网络异常。

##### Scenario: 422 协议不支持准确展示业务卡片 <a id="scenario-pub-hr-234-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 前端收到后端返回的 HTTP 422 `unsupported_target_capability` 响应
- **THEN** 界面展示具体的协议能力诊断与节点定位，不出现“网络连接异常，请检查服务可用性”的误导提示

### Explicitly Pending Historical Stubs

#### Requirement: Verge 订阅请求头标准化与单订阅自定义覆盖 [HR-195] <a id="req-pub-hr-195"></a>

抓取客户端在执行远程订阅拉取时，SHALL 采用对齐 Clash Verge Rev 官方规范的默认请求头，并保留单订阅自定义覆盖优先级与安全边界：
1. 当订阅未配置自定义 `user_agent_policy` 时，抓取客户端 SHALL 使用默认 User-Agent `clash-verge/v2.5.6`。
2. 当订阅显式配置了自定义 `user_agent_policy` 时，抓取客户端 SHALL 优先使用该自定义 User-Agent。
3. 抓取请求头 SHALL 显式包含 `Accept: */*` 与 `Accept-Encoding: gzip, deflate`。
4. 抓取客户端 SHALL 维持现有 URL Basic Auth 凭据解析、最大响应体积截断、解压缩安全边界与 SHA-256 摘要计算，禁止出现安全回退。

##### Scenario: 未指定自定义 UA 时默认发送 Verge 规范请求头 <a id="scenario-pub-hr-195-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 客户端抓取一个未设置自定义 UserAgent 的订阅源
- **THEN** 发送的 HTTP 请求包含 `User-Agent: clash-verge/v2.5.6`、`Accept: */*` 与 `Accept-Encoding: gzip, deflate`

##### Scenario: 指定自定义 UA 时优先使用订阅配置 <a id="scenario-pub-hr-195-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 客户端抓取一个 `user_agent_policy` 设置为 `my-custom-client/2.0` 的订阅源
- **THEN** 发送的 HTTP 请求包含 `User-Agent: my-custom-client/2.0`，且包含 `Accept: */*`

#### Requirement: 真实节点详情与有意义的编辑 [HR-225] <a id="req-pub-hr-225"></a>

系统 SHALL 在管理员详情接口从受认证凭据源提供真实非秘密连接字段和准确 secret-presence 布尔值；未认证列表/日志/错误/快照 SHALL 不暴露秘密，详情亦 SHALL 不返回私钥、PSK、密码。管理员编辑 SHALL 使用鉴权、CSRF、期望凭据版本 CAS、事务和脱敏审计；空秘密输入代表保留，明确轮换输入仅单向写入、不回显。改变 ID 所依赖的端点/transport SHALL 不得在旧 ID 下静默修改；源订阅 Reconcile 对同 ID 本地修改的覆盖语义 SHALL 明示并可验证；前端 SHALL 只有服务端确认后才显示已保存。

##### Scenario: 详情数据与失败状态 <a id="scenario-pub-hr-225-1"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** WG/TUIC 详情有真实凭据或凭据缺失/校验失败
- **THEN** 只显示真正存在的可展示值及真实 secret-presence 状态；缺失显示 unavailable，不产生默认地址、公钥、UUID 或虚构 secret 状态

##### Scenario: 编辑、轮换、刷新与冲突 <a id="scenario-pub-hr-225-2"></a>
- **GIVEN** 发布目标配置与导出令牌就绪
- **WHEN** 管理员持有效 CSRF 与 expected_credential_version 轮换秘密、尝试改身份端点、并在之后触发源订阅刷新；或两名管理员并发编辑
- **THEN** 轮换原子持久化且版本增加，旧版本冲突拒绝；改身份端点要求改订阅源并 Reconcile 为新 ID；刷新覆盖/冲突行为可见且审计脱敏，明文秘密不出现在返回、DOM 或日志


---

## Success Criteria

### Measurable Outcomes
- **[SC-PUB-01]** Mihomo 导出配置语法有效率达到 100%。
- **[SC-PUB-02]** 专属 Token 与主令牌鉴权通过率达到 100%。
- **[SC-PUB-03]** 历史 `/p/{id}` 路由向后兼容率达到 100%。
- **[SC-PUB-04]** 废弃端点 410 Gone 响应准确率达到 100%。

## Purpose

定义订阅拉取的请求头标准（对齐 Clash Verge Rev 官方规范）、全局与订阅级出站代理的分级解析机制，以及针对目标地址、重定向与代理端点的严格分离式 SSRF 安全防护模型。

## ADDED Requirements

### Requirement: Verge 订阅请求头标准化与单订阅自定义覆盖
抓取客户端在执行远程订阅拉取时，SHALL 采用对齐 Clash Verge Rev 官方规范的默认请求头，并保留单订阅自定义覆盖优先级与安全边界：
1. 当订阅未配置自定义 `user_agent_policy` 时，抓取客户端 SHALL 使用默认 User-Agent `clash-verge/v2.5.6`。
2. 当订阅显式配置了自定义 `user_agent_policy` 时，抓取客户端 SHALL 优先使用该自定义 User-Agent。
3. 抓取请求头 SHALL 显式包含 `Accept: */*` 与 `Accept-Encoding: gzip, deflate`。
4. 抓取客户端 SHALL 维持现有 URL Basic Auth 凭据解析、最大响应体积截断、解压缩安全边界与 SHA-256 摘要计算，禁止出现安全回退。

#### Scenario: 未指定自定义 UA 时默认发送 Verge 规范请求头
- **WHEN** 客户端抓取一个未设置自定义 UserAgent 的订阅源
- **THEN** 发送的 HTTP 请求包含 `User-Agent: clash-verge/v2.5.6`、`Accept: */*` 与 `Accept-Encoding: gzip, deflate`

#### Scenario: 指定自定义 UA 时优先使用订阅配置
- **WHEN** 客户端抓取一个 `user_agent_policy` 设置为 `my-custom-client/2.0` 的订阅源
- **THEN** 发送的 HTTP 请求包含 `User-Agent: my-custom-client/2.0`，且包含 `Accept: */*`

### Requirement: 全局与订阅级出站代理分级解析与调度一致性
系统 SHALL 支持通过全局受控配置与单订阅独立配置指定显式 HTTP 出站代理（`http://`），并确保手动刷新与定时调度走完全一致的代理选择逻辑：
1. 优先级顺序 SHALL 严格遵循：单订阅级 `refresh_policy.fetch_proxy_ref` 优先 > 全局受控配置（`CSP_FETCH_PROXY` / 命令行参数 `-fetch-proxy`）> 未配置时使用直连。
2. 调度执行器与手动刷新入口在调用 `inventory.ReconcileSubscription` 时，SHALL 统一应用上述代理选择结果。
3. 在版本受控的 `docker-compose.example.yml` 中，系统 SHALL 提供 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts` 样例，宿主机环境的 `.gitignore` 忽略文件 `docker-compose.yml` 遵循核验与投产备份规程。

#### Scenario: 订阅未指定代理时自动继承全局受控代理
- **WHEN** 全局配置了 `CSP_FETCH_PROXY=http://host.docker.internal:7890` 且订阅未设置 `fetch_proxy_ref`
- **THEN** 订阅拉取操作自动经由 `http://host.docker.internal:7890` 代理执行请求

#### Scenario: 订阅配置了特定代理时覆盖全局代理
- **WHEN** 全局配置了 `CSP_FETCH_PROXY` 但订阅明确指定了合法的 `fetch_proxy_ref=http://custom-proxy.example.com:8080`
- **THEN** 订阅拉取操作优先经由 `http://custom-proxy.example.com:8080` 执行

#### Scenario: 未配置任何代理时保持安全直连
- **WHEN** 全局 `CSP_FETCH_PROXY` 为空且订阅 `fetch_proxy_ref` 亦为空
- **THEN** 订阅拉取操作通过直连模式发起，不继承宿主机环境变量中的不可控代理

### Requirement: 严格分离式 SSRF 目标防御、代理端点精确准入与应用层目标 IP 强制绑定
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

#### Scenario: 离线恶意 HTTP 代理仅见已校验公网 IP 且不见未校验域名
- **WHEN** 客户端通过受信任代理抓取 `http://sub.example.com/feed` 与 `https://sub.example.com/feed`（本地 DNS 解析为公网 IP `93.184.216.34`）
- **THEN** 代理捕获到的 HTTP 请求行绝对 URL 为 `http://93.184.216.34:80/feed`、HTTPS `CONNECT` 权威目标为 `93.184.216.34:443`（均不见 `sub.example.com`），而源站收到的 `Host` 头与 TLS SNI 仍为 `sub.example.com` 且完成严格证书校验

#### Scenario: 代理侧 DNS 投毒指向宿主机回环时无法访问内网
- **WHEN** 上游代理自身 DNS 被投毒（将 `sub.example.com` 解析至 `127.0.0.1:18080` 内网服务）且私网规则为 `DIRECT`，客户端通过该代理拉取 `http://sub.example.com/feed` 或 `https://sub.example.com/feed`
- **THEN** 客户端向代理强制传递预校验的公网 IP 而非域名，代理不会触发针对 `sub.example.com` 的投毒 DNS 解析，内网 `127.0.0.1:18080` 零命中

#### Scenario: 目标 URL 指向回环或私网时即使配置了代理仍被拦截
- **WHEN** 客户端配置了全局代理 `http://host.docker.internal:7890`，但拉取目标 URL 为 `http://127.0.0.1:8080/feed` 或 `http://169.254.169.254/latest/meta-data`
- **THEN** 系统在预检阶段即刻阻断请求，返回 `ssrf_blocked` 领域安全错误，不向代理发出任何网络连接

#### Scenario: 重定向跳转每跳重新绑定公网 IP 且跳转内网被即时阻断
- **WHEN** 订阅目标返回 302 重定向至另一公网域名或重定向至 `http://10.0.0.1:8080/internal`
- **THEN** 公网重定向跳重新解析并向代理绑定新跳的公网 IP 及更新 TLS `ServerName`；指向内网的重定向在本地预检阶段即刻返回 `ssrf_blocked` 阻断，不向代理发起连接

#### Scenario: 同端口非 HTTP 协议代理与畸形端口/尾随点被严格拒绝
- **WHEN** 全局受信任代理为 `http://host.docker.internal:7890`，请求指定 `socks5://host.docker.internal:7890`、`https://host.docker.internal:7890`、`http://host.docker.internal:07890` 或 `http://host.docker.internal.:7890`
- **THEN** 系统判定协议不支持或端点格式违规，立即返回 `invalid_proxy_url` 失败闭合，严禁因同端口而豁免

#### Scenario: 受信任精确代理端点获准但危险端口被严格拒绝
- **WHEN** 全局受信任代理为 `http://host.docker.internal:7890`，用户或订阅请求指定 `http://host.docker.internal:2375`、`:6379` 或 `:22`
- **THEN** 系统判定端口不在受信任精确白名单内，回退至 SSRF 校验并检测到内网网关 IP，立即返回 `proxy_ssrf_blocked` 阻断

#### Scenario: 网关私有 IP 等价绕过被拦截
- **WHEN** 用户尝试在订阅级代理中提供 `host.docker.internal` 解析后的网关私有 IP（例如 `http://172.17.0.1:7890`）
- **THEN** 系统检测到该 IP 属于私有 RFC 1918 范围且未处于显式白名单，返回 `proxy_ssrf_blocked` 阻断

#### Scenario: 用户自定义代理指向内网元数据或回环被拒绝
- **WHEN** 订阅设置了 `fetch_proxy_ref=http://169.254.169.254:80` 或 `http://127.0.0.1:7890`
- **THEN** 系统判定代理端点违规，返回 `proxy_ssrf_blocked` 错误，拒绝建立代理连接

#### Scenario: 端口省略、userinfo 与混淆 IP 被严格拒绝
- **WHEN** 订阅设置代理为 `http://host.docker.internal`、`http://user:pass@host.docker.internal:7890` 或 `http://0177.0.0.1:7890`
- **THEN** 系统拒绝该代理端点并返回校验或安全错误，失败闭合

### Requirement: 代理故障分类、安全脱敏与审计记录
当代理连接失败、超时或遇到网络异常时，系统 SHALL 生成清晰结构化错误并在持久化审计中进行脱敏记录：
1. 代理网络超时或连接拒绝等底层异常 SHALL 正确分类为 `fetch_failed` 或 `fetch_timeout` 等领域错误。
2. 订阅拉取审计表 `subscription_fetches` 中记录的 `redacted_error` SHALL 经过敏感凭据与 Token 脱敏，严禁泄露 URL 中的 Token、Secret 或内部认证参数。
3. 单次刷新失败 SHALL 正常记录 `outcome = failed`，不得因报错导致脏状态遗留或未写入完成状态。

#### Scenario: 代理端口无法连接时分类为失败并记录脱敏审计
- **WHEN** 全局代理配置了不可达的端口导致连接拒绝
- **THEN** `ReconcileSubscription` 捕获该错误并返回失败摘要，在 `subscription_fetches` 中写入 `outcome = failed` 且错误信息已剔除敏感凭据

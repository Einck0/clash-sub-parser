## Context

参见 `proposal.md`。生产环境 9 个订阅源中 4 个处于启用状态，最近 6 条拉取审计记录显示 4 条 failed（TLS 握手超时、拉取超时与 HTTP 404）及 2 条 partial，且所有订阅的 `fetch_proxy_ref` 与 UA 均为空。容器内当前无代理环境变量。Clash Verge Rev 官方 v2.5.6（`src-tauri/src/utils/network.rs:246-255`）使用 `clash-verge/v2.5.6` 默认 UA 与 `reqwest` 标准请求头（`Accept: */*`, `Accept-Encoding: gzip, deflate`）。宿主机运行中 mihomo 监听于 `0.0.0.0:7890`，实测容器通过 `host.docker.internal:7890` 连通正常。

## Goals / Non-Goals

**Goals:**
- 将系统默认抓取请求头对齐 Clash Verge Rev 官方标准，包括默认 UA、`Accept: */*`、`Accept-Encoding: gzip, deflate`。
- 支持单订阅级自定义 `user_agent_policy` 优先覆盖。
- 设计明确配置的全局出站订阅代理机制（环境变量 `CSP_FETCH_PROXY` / 命令行参数 `-fetch-proxy`），在 Compose 中安全对接 `host.docker.internal:7890`。
- 实现分级代理选择：订阅级 `fetch_proxy_ref` > 全局 `CSP_FETCH_PROXY` > 直连。
- 保证手动刷新与定时调度统一走相同的代理选择管道。
- 建立分离式 SSRF 安全防护模型与应用层强制目标 IP 绑定：绝不依赖上游代理（如 mihomo）的 ACL；在每一跳连接前解析并校验目标公网 IP，将已验证的公网 IP 强制绑定传递给代理（HTTP 传递 IP 绝对 URL 且保留原 `Host`，HTTPS 向已验证公网 IP 发起 `CONNECT` 并在 TLS 中校验原域名证书与 SNI），302 每跳重新校验与绑定。
- 严格限定显式 `http://` 代理协议，白名单精确匹配 `http://<canonical-host>:<canonical-port>`，明确端口前导零、尾随点与 IPv6 规范化规则，自定义 `fetch_proxy_ref` 私网端点严禁绕过。
- 如实陈述运行态代理域名路由影响与 TOCTOU 边界，严谨执行本地测试与错误分类脱敏审计，严禁真实批量刷新生产订阅。

**Non-Goals:**
- 盲目照搬桌面端 Clash Verge 的系统代理嗅探或跳过 TLS 证书校验。
- 盲目放开内网或回环访问权限，或向白名单写入裸主机名导致同主机其他危险端口被放行。
- 在构建与验证过程中直接触发生产环境真实拉取。

## Decisions

### 1. Verge 订阅头规范化
- **决策**：在 `internal/fetch/fetch.go` 中将 `DefaultUserAgent` 常量修改为 `"clash-verge/v2.5.6"`，并在构建 HTTP 请求时显式注入 `Accept: */*` 和 `Accept-Encoding: gzip, deflate`。保留原有基于 `opts.UserAgent` 的单订阅覆盖逻辑。
- **备选方案**：保留 `clash-sub-parser/1.0` 并在订阅中逐个手动配置 UA。被否决，因为大多数订阅提供商（如各类面板）只识别 Clash/Verge 客户端 UA 才能返回对应配置，且生产中现有所有订阅 UA 均为空。

### 2. 全局出站代理与优先级分发
- **决策**：
  1. 在 `cmd/csp/main.go` 中解析 `CSP_FETCH_PROXY` 与命令行 `-fetch-proxy`。
  2. 严格校验与规范化代理端点：仅允许显式 `http://` 协议（`socks5`、`https` 等其他协议一律失败闭合），必须具备显式合法端口（无前导零）、无 userinfo、无尾随点、无混淆 IP，且将规范化的完整 `http://<canonical-host>:<canonical-port>` 注入 `fetch.Policy` 的 `AllowedProxyHosts`，严禁丢弃 scheme 或写入裸 `host:port` / 裸主机名。
  3. 在 `inventory.Service` 中引入 `defaultFetchProxy` 字段及 `inventory.WithDefaultFetchProxy(...)` 选项。
  4. `ReconcileSubscription` 中判定：若 `sub.RefreshPolicy.FetchProxyRef != ""`，使用订阅级代理；否则若 `s.defaultFetchProxy != ""`，使用全局默认代理；否则使用直连。
- **备选方案**：使用标准库默认从 `HTTP_PROXY` / `HTTPS_PROXY` 自动继承。被否决，因为这会导致容器内无关请求（如探针、健康检查或本地通信）被意外劫持，不符合受控安全边界契约。

### 3. Go `net/http` 代理与 TLS 模型审计及应用层目标 IP 强制绑定
- **背景与实证**：
  - 审查实测确认，生产环境宿主机 mihomo（`/root/clashctl/resources/runtime.yaml`）中私网与回环地址规则为 `DIRECT`，通过 `7890` 代理实际可穿透访问宿主机 `127.0.0.1:18080`。因此，**绝对不能依赖上游 mihomo ACL 防御代理端 SSRF 或 DNS Rebinding**。
  - 审计 Go 标准库 `net/http` 源码确认：
    1. 对于 HTTP 正向代理（`net/http/request.go` `Request.WriteProxy`）：当 `usingProxy` 为真且 `r.URL.Opaque == ""` 时，标准库用 `r.Host`（若非空）拼接 `ruri = r.URL.Scheme + "://" + host + ruri`，导致请求行仍发送原始域名；而当显式设置 `r.URL.Opaque = "//" + pinnedIPPort + escapedPath` 且 `r.Host = origHostHeader` 时，`WriteProxy` 发送的请求行固定为 `GET http://<pinnedIP>:<port><path>?<query> HTTP/1.1`，同时 `Host` 头保持 `Host: <origHostHeader>`。
    2. 对于 HTTPS 代理隧道（`net/http/transport.go` `dialConn` & `addTLS`）：标准库使用 `cm.targetAddr = canonicalAddr(treq.URL)` 构造 `CONNECT <cm.targetAddr> HTTP/1.1`；随后调用 `addTLS(ctx, cm.tlsHost(), trace)`，其中 `if cfg.ServerName == "" { cfg.ServerName = name }`。因此，当每跳将 `treq.URL.Host` 设为已校验的 `pinnedIP:port`、将 `treq.Host` 设为 `origHostHeader`，并在 `Transport.TLSClientConfig` 中显式设置 `ServerName: tlsServerName`（原始目标域名）且保持 `InsecureSkipVerify: false` 时，代理收到的 `CONNECT` 权威目标永远是已验证公网 IP（不见原始域名），而隧道内 TLS 握手依然发送原域名 SNI 并对原域名执行严格的 X.509 证书校验。
- **决策**：
  1. **显式每跳重验与目标 IP 强制绑定（Per-Hop Resolution & Forced IP Pinning）**：
     - 禁用 `http.Client` 的隐式重定向跟随（`CheckRedirect` 返回 `http.ErrUseLastResponse`），在 `Client.Fetch` 中实现显式有界跳转循环（最多 `MaxRedirects` 跳）。
     - 每一跳（含初始请求与每次 `301/302/303/307/308` 重定向）在发起网络请求前，先在本地通过 `policy.Resolver` 解析目标主机名，并对全部解析出的 IP 逐一执行 `policy.ValidateIP` 校验（遇到任何私网、回环、链路本地/云元数据地址立即返回 `ssrf_blocked`）。
     - **HTTP 代理转发**：使用校验通过的公网 `pinnedIP:port` 构造 `req.URL.Host = pinnedAddr` 与 `req.URL.Opaque = "//" + pinnedAddr + escapedPath`，并设置 `req.Host = origHostHeader`，使代理仅看到公网 IP 目标且源站收到原 `Host` 头。
     - **HTTPS 代理 CONNECT**：使用校验通过的公网 `pinnedIP:port` 构造 `req.URL.Host = pinnedAddr`，`req.Host = origHostHeader`，配置 `TLSClientConfig: &tls.Config{ServerName: origHostname, RootCAs: opts.RootCAs, MinVersion: tls.VersionTLS12, InsecureSkipVerify: false}`，使代理在 `CONNECT` 阶段仅看到 `pinnedIP:port`，而客户端在隧道内完成原域名 SNI 发送与严格证书验证。
  2. **代理协议限定与白名单精确规范化（Strict `http://` Scheme & Canonical Whitelist）**：
     - 仅允许显式 `http://` 代理协议；`socks5`、`https`、`ftp` 或无 scheme 输入一律失败闭合返回 `invalid_proxy_url`。
     - 规范化规则：端口必须显式指定（1..65535）且严禁包含前导零（如 `:07890` 拒绝）；代理主机名严禁包含尾随点（如 `host.docker.internal.` 或 `127.0.0.1.` 拒绝）；严禁携带 userinfo、路径、查询参数或片段；IPv6 必须使用标准方括号格式并规范化为 RFC 5952 形式（严禁携带 `%` Zone ID 或 IPv4-mapped IPv6 `::ffff:...`）。
     - `AddAllowedProxy` 与 `IsProxyHostAllowed` 严格按完整 `http://<canonical-host>:<canonical-port>` 精确匹配，彻底杜绝丢弃 scheme 或同端口跨协议豁免。
     - 对于未命中白名单的自定义 `fetch_proxy_ref`：必须通过完整公网 SSRF 校验并解析绑定代理自身的公网 IP 进行建连，严禁绕过访问私网/回环/元数据。
  3. **路由行为说明与 TOCTOU 边界如实披露**：
     - **代理域名路由影响**：由于应用层强制向代理传递已验证的公网 IP（HTTP 请求行与 HTTPS CONNECT authority 均为 IP），如果上游代理依赖请求行/CONNECT 域名匹配分流规则（如仅配置 `DOMAIN-SUFFIX` 而未开启 TLS SNI 嗅探或对应 IP/FINAL 规则），流量在代理内部将按 IP 规则或默认出口路由。系统如实记录并测试该特性，绝不为此牺牲 SSRF 防护或关闭 TLS 证书校验。
     - **TOCTOU 边界**：应用层强制将本地预解析并校验通过的公网 IP 传给代理，彻底消除了“客户端解析为公网 IP、代理端二次 DNS 解析重绑定为内网 IP”的代理侧 DNS Rebinding 攻击面；但对于公网 IP 在极短时间窗内的互联网 BGP/云弹性 IP 归属变更，系统不作不切实际的绝对消除宣称。

### 4. 宿主 mihomo 7890 代理对接与 Compose 投产规范
- **决策**：
  1. 更新版本受控的样例文件 `docker-compose.example.yml`，增加 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts: ["host.docker.internal:host-gateway"]`。
  2. 宿主机实际运行的 `docker-compose.yml` 处于 `.gitignore` 中，保证提交仅包含 `.example`。
  3. 上线投产规程：上线部署时，先对宿主机既有 `docker-compose.yml` 执行带时间戳备份，比对 `docker-compose.example.yml` 变更，同步合并 `CSP_FETCH_PROXY` 环境变量与 `extra_hosts` 配置，运行 `docker compose config` 核验语法，再执行重启投产。

## Risks / Trade-offs

- [Risk: 上游代理（如 mihomo）对私网配置 `DIRECT` 且存在代理侧 DNS 投毒/重绑定] → Mitigation: 应用层强制将经过本地 SSRF 校验的公网 IP 绑定给代理（HTTP 绝对 URL 与 HTTPS CONNECT authority 均仅含公网 IP），代理不执行目标域名解析，无法将流量导向内网或宿主机回环。
- [Risk: 应用层向代理传递 IP 导致上游代理纯域名分流规则未命中] → Mitigation: HTTPS 隧道内仍保留完整原域名 TLS SNI（支持代理侧 sniffer），HTTP 请求仍保留原 `Host` 头；在设计、规格与测试中如实说明该路由差异。
- [Risk: 用户配置了不可达的代理端口导致拉取挂起] → Mitigation: `fetch.Client` 具有确定性的连接超时（`Dialer.Timeout`）与请求整体超时（默认 30s），超时后统一返回 `fetch_timeout` 失败闭合，并在审计日志中记录。
- [Risk: 目标订阅通过 302 重定向到内部私网服务或跨域名跳转] → Mitigation: 显式每跳重定向循环对每一跳独立执行 DNS 解析、SSRF 公网校验、目标 IP 重新绑定与 TLS `ServerName` 更新，遇到私网 IP 立即阻断。
- [Risk: 协议混淆、端口前导零、尾随点或混淆 IP 绕过代理校验] → Mitigation: 仅允许 `http://`，白名单精确匹配 `http://hostname:port`，严格拦截端口前导零、尾随点、IPv6 Zone、八进制/十六进制/整型/短 IP 及 IPv4-mapped IPv6。

## Migration / Production Deployment Plan

1. 代码变更提交并在隔离环境中编译自测（仅跟踪 `docker-compose.example.yml`）。
2. 宿主机上线流程：
   - 步骤 1：备份当前宿主机配置 `cp docker-compose.yml docker-compose.yml.bak.$(date +%Y%m%d%H%M%S)`。
   - 步骤 2：参照 `docker-compose.example.yml` 在 `docker-compose.yml` 中补充 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts`。
   - 步骤 3：执行 `docker compose config` 确保语法无误。
   - 步骤 4：执行镜像更新与容器受控重建，验证健康检查端点。
3. 保持数据库向后兼容，不引入任何破坏性模式迁移。

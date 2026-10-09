## Why

生产环境中 9 个订阅源（4 个启用）最近 6 次拉取记录全部未成功（4 次 failed：2 次 TLS handshake timeout、1 次 fetch 30s timeout、1 次 HTTP 404；2 次 partial），且所有订阅的 `fetch_proxy_ref` 与 `user_agent_policy` 均为空，运行容器亦未配置代理环境变量。虽然 HTTP 404 可能是特定服务端对默认 UA 的拒绝（假说，非实测确证），但节点订阅服务器多位于境外，缺乏出站代理是导致握手与拉取超时的关键根因。

同时，用户明确要求采用 Clash Verge Rev（官方 v2.5.6 `src-tauri/src/utils/network.rs:246-255`）的订阅请求头标准（默认 UA `clash-verge/v2.5.6`，`Accept: */*`，`Accept-Encoding: gzip, deflate`）。系统需要建立严谨的全局与订阅级出站代理通道，并在保持严格 SSRF 防护的前提下，实现安全可信的出站代理接入。

## What Changes

- **Verge 订阅请求头标准化（Verge Headers & User-Agent Alignment）**：
  - 将系统默认抓取 UA 从 `clash-sub-parser/1.0` 调整为官方精确版本 `clash-verge/v2.5.6`。
  - 请求头补齐 `Accept: */*`，保持 `Accept-Encoding: gzip, deflate`。
  - 保留单订阅级自定义 `user_agent_policy` 优先覆盖机制。
  - 维持现有 URL Basic Auth 鉴权及有界安全解压限制，不可回退。
- **可控的全局出站订阅代理机制（Controlled Global & Subscription Outbound Proxy）**：
  - 新增全局代理配置入口（环境变量 `CSP_FETCH_PROXY` / 命令行参数 `-fetch-proxy`），通过 `cmd/csp` 注入 `inventory.Service` 与 `fetch.Client`。
  - 确定代理选择优先级：单订阅级 `fetch_proxy_ref` 优先 > 全局 `CSP_FETCH_PROXY` > 未配置则直连。
  - 更新版本受控的 `docker-compose.example.yml`，增加 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts` 映射。真实 `docker-compose.yml` 维持 `.gitignore` 忽略，通过投产备份核验落地。
- **严格的分离式 SSRF 代理安全模型与应用层目标 IP 强制绑定（Strict Fail-Closed Proxy & Application-Layer Target IP Pinning）**：
  - 目标端点与每次重定向保持严格的私网/回环/元数据（169.254.169.254）拦截，目标域名解析后遇私有 IP 立即失败闭合。
  - **应用层强制绑定经过公网校验的目标 IP 给代理（禁止依赖上游代理/mihomo ACL）**：
    - 经实证，生产环境 mihomo（`/root/clashctl/resources/runtime.yaml`）对私网与回环网段配置为 `DIRECT`，经 7890 代理可穿透访问宿主机 `127.0.0.1:18080`，故**绝对不得依赖 mihomo ACL 作为 SSRF 防线**。
    - 客户端在每一跳（初始请求及每次 301/302/303/307/308 重定向）连接代理前，必须在本地完成目标域名 DNS 解析并对全部解析 IP 执行严格公网 SSRF 校验（`ValidateIP`）。
    - **HTTP 正向代理目标绑定**：将经过公网校验的 `pinnedIP:port` 写入发送给代理的请求行绝对 URL（`GET http://<pinnedIP>:<port>/path`），同时在 `Host` 请求头中保留原始目标域名（`Host: <original-domain>`），确保代理仅按已验证的公网 IP 建立连接，严禁代理对原始域名进行二次 DNS 解析。
    - **HTTPS 代理 CONNECT 目标绑定与 TLS 原域名校验**：向 HTTP 代理发送 `CONNECT <pinnedIP>:<port> HTTP/1.1`（`Host: <pinnedIP>:<port>`），使代理仅见到已验证的公网 IP；隧道建立后，在客户端 TLS 握手中显式设置 `ServerName: <original-domain>`（SNI）并严格校验远端证书与原始域名匹配（严禁开启 `InsecureSkipVerify`）。
    - **重定向每跳重验与重绑**：禁用标准库自动隐式跳转，按每跳显式循环重新解析重定向目标、重新执行 SSRF 校验并重新绑定新跳的公网 IP 给代理。
  - **代理端点显式 HTTP 协议限定与精确白名单规范化**：
    - 先限定仅支持显式 `http://` 代理协议，不支持的 scheme（如 `socks5`、`https`、`ftp` 或省略 scheme）一律失败闭合（`invalid_proxy_url`）。
    - `AddAllowedProxy`、`IsProxyHostAllowed` 与 `ValidateProxy` 严格按规范化的完整 `scheme://hostname:port`（即 `http://<canonical-host>:<canonical-port>`）精确匹配，严禁丢弃 scheme 或优先按裸 `host:port` 匹配导致同端口的 `socks5`/`https` 代理获豁免。
    - 明确规范化与拒绝规则：拒绝端口省略、拒绝端口前导零（如 `:07890`）、拒绝代理主机名尾随点（如 `host.docker.internal.` 或 `127.0.0.1.`）、拒绝 userinfo 凭据、拒绝 IPv6 Zone ID（`%`）、拒绝混淆 IP（八进制/十六进制/整型/短 IP/IPv4-mapped IPv6），IPv6 地址统一按 RFC 5952 规范化为带方括号格式 `http://[<canonical-ipv6>]:<port>`。
    - 任何未在白名单内的自定义 `fetch_proxy_ref` 端点绝不豁免，必须通过完整公网 SSRF 校验并同样绑定校验后的代理公网 IP，严禁指向私网、回环、网关私有 IP 或云元数据。
  - **如实说明路由影响与 TOCTOU 边界**：
    - 由于向代理传递的是已校验公网 IP（CONNECT authority 与 HTTP 请求行均为 IP），若上游代理配置了依赖请求行/CONNECT 域名的路由分流规则（而非基于 IP/GeoIP 或 TLS SNI 嗅探），其分流路线可能改变（例如走 IP/FINAL 规则而非 DOMAIN 规则）；系统在规格、设计与测试中如实说明并验证该行为，绝不以此为由关闭证书校验或跳过应用层 IP 绑定。
    - 应用层 IP 绑定彻底消除了代理侧二次 DNS 解析导致的代理侧 DNS Rebinding 与内网穿透，但系统不虚假宣称消灭互联网公网 IP 层面的极端路由重分配 TOCTOU。
- **调度一致性与可观测性（Scheduler Consistency & Failure Audit）**：
  - 手动刷新与后台定时调度统一走相同的代理选择与安全判定流水线。
  - 代理连接失败、超时或安全拒绝统一进行结构化分类与安全脱敏审计。

## Capabilities

### New Capabilities

- `subscription-proxy-and-verge-headers`: Verge 订阅请求头标准化、全局与订阅级出站代理分级解析、受控宿主代理接入、以及严格的分离式代理 SSRF 安全模型。

### Modified Capabilities

无（`openspec/specs/` 下无对应已归档主规格，本次通过新增能力规格建立权威契约）。

## Impact

- `internal/fetch/`（`fetch.go`, `policy.go`, `fetch_test.go`）：默认 UA、请求头、严格代理准入校验规则、混淆 IP 防御与安全策略。
- `internal/application/inventory/`（`service.go`, `service_test.go`）：支持默认代理注入与优先级解析。
- `cmd/csp/`（`main.go`）：解析 `CSP_FETCH_PROXY` 与 `-fetch-proxy`，装配 `fetchPolicy`（仅注入规范化 `host:port`）、`fetchClient` 与 `inventory.Service`。
- `docker-compose.example.yml`：增加 `CSP_FETCH_PROXY` 默认环境变量示例与 `extra_hosts`。
- 本地自动化测试（单测与 httptest 隔离夹具）：覆盖请求头、代理优先级、SSRF 拦截、端口与混淆 IP 过滤、重定向安全及失败审计。

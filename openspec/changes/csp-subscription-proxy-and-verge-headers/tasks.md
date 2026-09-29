## 1. 订阅抓取客户端请求头规范化

- [x] 1.1 更新 `internal/fetch/fetch.go` 中的默认 UA 为 `clash-verge/v2.5.6`，显式设置 `Accept: */*`，保留 `Accept-Encoding: gzip, deflate` 与单订阅自定义 UA 优先覆盖，并通过针对性单元测试验证
- [x] 1.2 编写 `internal/fetch/fetch_test.go` 针对 Verge 请求头的自动化验证用例（默认 UA、自定义 UA 覆盖、Accept 头验证），运行并通过测试

## 2. 严格分离式 SSRF 策略、精确 HTTP 代理准入与应用层目标 IP 强制绑定

- [x] 2.1 审计 Go `net/http` Proxy `CONNECT`、HTTP 正向代理与 TLS `Host`/SNI 模型；在 `internal/fetch/policy.go` 中限定显式 `http://` 代理协议（`socks5`、`https` 等不支持的 scheme 一律失败闭合），白名单 `AllowedProxyHosts` 与 `AddAllowedProxy` / `IsProxyHostAllowed` / `ValidateProxy` 严格按规范化的 `http://<canonical-host>:<canonical-port>` 精确匹配，明确拦截端口省略、端口前导零（如 `:07890`）、代理主机名尾随点（如 `host.docker.internal.` / `127.0.0.1.`）、恶意 userinfo、IPv6 Zone ID 及混淆 IP（八进制/十六进制/整型/短 IP/IPv4-mapped IPv6），彻底废除对上游 mihomo ACL 的安全依赖
- [x] 2.2 在 `internal/fetch/fetch.go` 中实现应用层强制绑定经过公网校验的目标 IP 给代理：每一跳本地预解析并校验全部目标 IP；HTTP 正向代理传递已校验公网 `pinnedIP:port` 绝对 URL 并保留原 `Host` 头；HTTPS 代理 `CONNECT` 到已验证的公网 `pinnedIP:port` 并在 TLS 握手中验证原域名证书与设置原域名 SNI（严禁关闭证书校验）；`301/302/303/307/308` 每跳重新解析、重新校验公网 IP 并重新绑定给代理；如实说明并测试依赖代理域名路由场景的路线变化与 TOCTOU 边界
- [x] 2.3 在 `internal/fetch/fetch_test.go` 中编写离线恶意 HTTP 代理与代理侧 DNS 投毒防御测试：验证代理捕获的 HTTP 绝对 URL 和 HTTPS `CONNECT` authority 永远是已校验公网 IP（绝不见未校验域名），源站收到的 `Host` 与 TLS SNI 仍为原域名且证书严格校验，302 重定向至内网被拒绝、公网多跳重定向每跳重绑 IP，本地 `httptest` 模拟代理侧 DNS 投毒指向 `127.0.0.1` 不得访问内网
- [x] 2.4 在 `internal/fetch/fetch_test.go` 中补充协议混淆豁免阻断（`socks5://`、`https://` 同端口 7890 拒绝）、端口前导零、尾随点、IPv6 规范化、混淆 IP、userinfo 及自定义 `fetch_proxy_ref` 私网端点严禁绕过的全链路测试

## 3. 全局出站代理与调度分级解析集成

- [x] 3.1 在 `internal/application/inventory/service.go` 中增加 `defaultFetchProxy` 字段与 `WithDefaultFetchProxy` 选项，并在 `ReconcileSubscription` 中实现单订阅级 > 全局受控代理 > 直连的分级优先级选择
- [x] 3.2 在 `cmd/csp/main.go` 中增加 `CSP_FETCH_PROXY` 环境变量与 `-fetch-proxy` 命令行参数解析，将受控代理配置注入 `fetch.Client` 与 `inventory.Service`
- [x] 3.3 在 `internal/application/inventory/service_test.go` 中增加关于全局代理注入与分级选择优先级的自动化测试，验证成功与失败情况下的审计脱敏记录

## 4. Compose 配置与全量质量门禁闭环

- [x] 4.1 修正 Compose 配置跟踪契约：更新版本受控的 `docker-compose.example.yml` 增加 `CSP_FETCH_PROXY: ${CSP_FETCH_PROXY:-http://host.docker.internal:7890}` 与 `extra_hosts`；在任务与设计中明确宿主机被忽略的 `docker-compose.yml` 需遵循核验、备份与投产规范，严禁尝试跟踪或强制提交已忽略文件
- [x] 4.2 执行本地端对端与 httptest 隔离模拟测试，验证代理故障分类、审计记录安全性，且不触发生产真实拉取、不触碰生产 mihomo 配置与宿主机 Compose
- [x] 4.3 运行全量静态编译与测试门禁：`gofmt`、`go build ./...`、`go test -count=1 -timeout=120s ./...`、变更包 `-race` 测试、前端 186 项测试 + `vue-tsc` 类型检查及 `openspec validate --strict` 严格校验

## 5. 独立审查与验收门禁（由 Reviewer / Critic 执行）

- [x] 5.1 独立代码与安全审查（Reviewer 复验应用层目标 IP 强制绑定、精确 scheme+host+port 白名单、测试覆盖与门禁凭据）
- [x] 5.2 独立成品视觉与黑盒验收（Critic 针对隔离预览实例完成 FLOW 与 VISUAL 双验收，出具 PASSED 裁决与多视口度量报告）
- [ ] 5.3 生产环境受控投产与健康检查（待审查与验收归档后，在明确授权下按部署手册执行生产 Compose 更新与验证）

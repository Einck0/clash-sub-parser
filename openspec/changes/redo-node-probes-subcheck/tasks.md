## 1. 后端依赖与进程内 Mihomo 代理出站 (Backend Runtime)

- [x] 1.1 引入 `github.com/metacubex/mihomo v1.19.31` 依赖并运行 `go mod tidy`，验证编译通过且依赖无冲突
- [x] 1.2 实现 `internal/probe/mihomo/adapter.go`，将 `domain.Node` 映射为 Mihomo proxy 配置并使用 `adapter.ParseProxy` 构造进程内出站；添加单元测试验证支持 `skip-cert-verify`、Hysteria2 `ports` 与 TUIC `disable_sni`
- [x] 1.3 实现 `internal/probe/mihomo/client.go`，构建 `ProxyClient` 绑定单节点出站，显式配置 `Proxy: nil`，将目标域名直接送入远端 Metadata，并实现 `Close()` 与超时 Watchdog 机制；添加单元测试验证宿主系统代理不生效且远端域名直传
- [x] 1.4 清理已被完全替代的 `internal/application/probe/safe_dialer.go` 与 `internal/probe/singbox/runtime.go` 等旧拨号代码，确保其他使用 Singbox 的导出和转换逻辑不受影响

## 2. 真实多平台能力检测与测速引擎 (Platform Capability Engines)

- [x] 2.1 移植 `internal/probe/platform/alive.go`，基于 Cloudflare 204 及快速 CDN Trace 接口实现轻量级 Stage 1 测活；编写单测验证有效 204 与非 2xx 响应处理
- [x] 2.2 移植 `internal/probe/platform/openai.go`，实现双轨（Cookies + iOS 客户端）检测与 Trace 国家码提取；编写单测覆盖 `Full (GPT⁺)`、`Web (GPT)` 与不可用场景
- [x] 2.3 移植 `internal/probe/platform/netflix.go`，实现 Fast.com CDN 测速接口检测与双 Title 回退断言；编写单测覆盖 `Full`、`OriginalsOnly`、`Banned` 三种状态
- [x] 2.4 移植 `internal/probe/platform/youtube.go`，请求 Premium 页面并提取 `INNERTUBE_CONTEXT_GL` 区域标签，过滤 `www.google.cn` 送中特征；编写单测覆盖解锁与送中场景
- [x] 2.5 移植 `internal/probe/platform/disney.go` 与 `internal/probe/platform/claude.go`，接入 BAMGrid 与 Trace 地区黑名单断言；编写单测验证判定逻辑
- [x] 2.6 实现 `internal/probe/platform/speed.go`，包装 `NetworkLimitedReader` 设定 5MB 应用读取预算上限（作用于 `resp.Body`，TCP 缓冲存在网络层预读，非 NIC 网卡硬上限）与单调截止超时；编写单测验证达到字节上限时主动返回 EOF 截断并正确计算带宽 (float64 KB/s, 1024B/s)
- [x] 2.7 在代码库根目录或相应模块维护原作者版权与 GPL-3.0 许可证 NOTICE 说明，记录上游出处

## 3. 三阶段自适应漏斗调度与契约透传 (Pipeline Funnel & Application Service)

- [x] 3.1 重构 `internal/probe/queue/` 与 `internal/application/probe/runner.go`，实现 Alive -> Media/AI -> Speed 三阶段漏斗调度，废除 16~32 硬编码并发限制，采用阶段并发（Alive 50 / Media 20；测速阶段采用 Speed 8 作为降低带宽争用的本地差异设置，上游默认则为 Speed 20），保证 Stage 2 结束才启动 Stage 3，支持用户单选/混选 kind，复用每节点 client 实例（上层复用 adapter 实例，非物理网络只拨号一次）并一次性优雅 close；编写调度器漏斗单测验证 Stage 1 失败节点绝不进入后续阶段
- [x] 3.2 真正接入 `platform.CheckSpeed` 生产调用，删除 `runner.go` 中被替代的 profiles.Speed 旧 1MB HTTP 分支，彻底消除新旧双轨，将各平台真实检测结构体统一序列化为 JSON 观测记录；编写单测验证所有探针 Kind 均有可达的真实判定
- [x] 3.3 扩展 `internal/domain/probe.go` 与 `internal/application/inventory/service.go` 的 `CapabilityStatus` 数据结构，透传细粒度平台结果（`region`、`sub_tier` 等）；编写单测验证各平台状态在 `ToNodeView` 中无损输出
- [x] 3.4 保持现有 `/api/v1/probes/*` 与 `/api/v1/nodes/*` 路由定义，确保现有管理鉴权、任务取消、批量触发与定时轮询逻辑兼容运作

## 4. 前端 WebUI 监控工作台与多平台状态徽标 (Frontend UI & i18n)

- [x] 4.1 更新 `web/src/features/probes/probeTypes.ts` 与 `web/src/features/nodes/nodeView.ts`，扩展多平台解锁结果数据模型（如 OpenAI、Netflix、YouTube、Disney、Claude 等细粒度状态）
- [x] 4.2 更新 `web/src/features/probes/ProbeRunCard.vue` 与 `ProbeEvidenceSheet.vue`，展示各平台状态徽标（如 `GPT⁺ (US)`、`NF (HK)`、`YT (SG)`）及测速带宽；运行 `npm --prefix web run type-check` 验证类型安全
- [x] 4.3 更新中英双语国际化文案字典 `web/src/locales/zh-CN.ts` 与 `en-US.ts`，补充各平台状态描述与封禁提示；运行 `npm --prefix web test` 确保前端测试全部通过

## 5. 全链路自动化测试与隔离验证 (Verification & Convergence Gate)

- [x] 5.1 运行确定性静态编译与类型门禁：执行 `go build ./...` 与 `npm --prefix web run type-check`，验证退出码严格为 0
- [x] 5.2 编写与运行受控代理/fixture集成测试覆盖实际 Runner 行为：alive 失败不测媒体与速度、阶段 2 结束才阶段 3、阶段并发 (alive 50/media 20/本地 speed 8)、用户单选/混选/取消/复用/close、速度 float64 KB/s 及预算、SQLite 到 Run/node 接口；执行 `go test ./...` 退出码严格为 0
- [x] 5.3 在本地隔离沙箱环境启动预览实例，验证管理凭据登录、节点列表及探针工作台真实渲染正常，准备就绪实例交付 Critic

## 6. 治理汇聚、多视口验收与发布准备 (Governance, Acceptance & Release Preparation)

- [x] 6.1 汇聚门禁独立代码审查（Reviewer Gate）：完成对全量 Diff、三阶段调度架构、Mihomo 出站与数据模型的独立代码审查，出具真实裁决 `REVIEW: PASS`
- [x] 6.2 成品隔离黑盒验收（Critic Gate）：基于本地隔离预览实例 (`127.0.0.1:18081`)，完成桌面端 (1440x900) 与移动端 (392x872) 15 视口截图采集与无控制台/网络报错验证，真实验证各平台细粒度分级 (GPT⁺/Web/App, NF Full/Originals/Banned, YT 送中 CN, Disney+ Soon) 与取消/流转流程，出具真实裁决 `CRITIC: PASSED`（全部基于 Test-Fixture，不声称真实商业解锁）
- [ ] 6.3 生产发布在线一致性热备（Production Preflight Backup）：部署前使用官方 SQLite `.backup` 协议对生产数据库 `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db` 执行在线热备并校验完整性与节点库存
- [ ] 6.4 生产发布与上线健康门禁（Controlled Deployment & Healthz）：构建并受控替换生产容器镜像，维持现有数据卷/双端口/代理/鉴权配置不变，验证生产 `/healthz` 与 `/readyz` 200 OK
- [ ] 6.5 合法生产管理身份真实节点实网验收（Production Acceptance with Legal Credential）：在用户提供或配置合法生产管理凭据后，针对生产活跃节点执行受控小范围真实节点探针抽测
- [ ] 6.6 冻结写集合与代码版本提交（Git Commit）：经独立审查与验收通过后，由主脑调用 `git_commit` 正式提交代码变更


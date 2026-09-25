## 1. 证据冻结、资产修复与构建隔离（本轮先决）

- [x] 1.1 保留先前既有工作树约 731–734 项规模记录；本轮不能用旧计数推导文件归属。禁止 reset/restore/clean、生产端口 17000/18080、生产卷、未经授权删除/提交。
- [x] 1.2 已有响应字节 limit+1 局部测试记录；不代表真实探针或最终版本通过。
- [x] 1.3 【旧证据不构成前置汇聚】撤销旧发布/主题/视觉综合完成声明。先前 37 PNG 的视觉 FAILED 是问题证据；其后 reviewer 工程 PASS/critic FLOW+VISUAL PASSED 的时序不能补足此 Change 的全量门禁→reviewer→critic。建立本轮同一最终代码版本、七路由、网络 fixture 与分阶段签名的唯一身份台账，旧证据仅作对照。
- [x] 1.4 【原仓资产取证与恢复，先判归属再操作】以只读清单/逐文件 SHA 复核 `/tmp/csp-critic-stage/original_dist` 18 文件、其中 17 原有 assets 与主仓逐字节相同及完整聚合基线 `bac5a0f424b263f8ad2f538368863200045978f58a0c9d8f9d89f1701866a655`；复核当前 22 文件/聚合 `6ec421559324992339314b05071a0e44f3fc5c91ee8b3339852ea10953ddf0f6`、两版 index 引用、当前/旧 web/dist、mtime/来源。为四个新增文件尤其 `index-CuRBRMN8.js/index-CY02MDnG.css` 确认使用/归属和保留理由；**未获授权不删/移任何未跟踪资产**。取证后由主脑确认逐文件恢复和合法额外资产安置范围；获授权施工者保全字节后定点恢复，复算旧 18 文件（含 index）SHA 与合法新文件清单、所有 index 引用可解析、原仓其他文件/status 未变化；歧义不勾、递交决策证据。
- [x] 1.5 【阻断再污染的构建门禁】审查 `web/vite.config.ts` `closeBundle` 默认复制与所有调用点，约定/必要时施工实现安全 opt-in 或隔离输出，不破坏正式 go:embed；在带 sentinel 的独立临时工作区测试默认构建/显式嵌入的边界。所有前端构建及 reviewer 复验只在隔离副本或显式 `NO_COPY_WEBASSETS=1` 下进行，前后检查主仓 `internal/webassets/dist` 全量路径+每文件 SHA、Git status 与业务源码未改变；`npm --prefix web run build` 在主仓默认环境禁止。记录测试命令/退出码 0、无原仓写入证据；若决定不改钩子也必须以确定性测试证明操作规范可执行且 reviewer 无副作用。

## 2. 单一完整特性包：真实安全探针与负载停服

- [x] 2.1 【真实网络总门禁，确实待实现/证明】只有 2.1a–c 真实六协议/12 transport 逐行 inbound→目标 204、合法 TCP/UDP socket/证书/Host、负例、race `-count=3`、`go build ./...` 全部通过后才勾；旧 Trojan 局部和非 race 204 不替代。
- [x] 2.1a 【已有局部代码但最终版缺证明】保全已有 Trojan/TLS/TCP `SafeNodeDialer` + `fetch.DefaultPolicy` 公网 IP pin→测试专属 sing-box inbound→`httptest.Server` 204、可信临时 CA/SAN/SNI/密码和双方 socket 证据；最终源码复验 `go test ./internal/application/probe ./internal/probe/singbox -run 'Test.*Trojan.*(Handshake|204)' -count=1`、`go build ./...` exit 0；fixture 不入生产。
- [x] 2.1b 【待根因归因/消除】取得 SHA 前缀 `263766475...` 原 race 日志完整路径/SHA/双栈，区分 fixture 普通计数器 race、sing-box v1.14.0 HTTP2/gRPC late-reader 与 VLESS/httpupgrade EOF/端口释放。用 `sync/atomic` 或锁同步 targetHits/observedHost；优先官方兼容修复，必要时固定 upstream commit 最小 late Read create patch，Write 不等 RoundTrip Setup；服务端先等 body 的确定性夹具证实先写后读/取消有界。六协议 + Trojan/VLESS/VMess × ws/http/httpupgrade/grpc 逐行真实 204、Host/SNI/授权 socket，`go test -race -count=3 ./internal/probe/singbox -run 'Test(ProtocolMatrixRealHandshake204|TransportMatrixRealHandshake204|TrojanRealTLSHandshakeAnd204)'` 与 `go build ./...` exit 0；不跳行、不重试到绿。
- [x] 2.1c 【负例确实待实现】从真实 `SafeNodeDialer` 授权凭据版本/受控 resolver/public IP pin 到测试专属 registry/loopback mapper，表驱动公私 DNS、重绑、metadata/RFC6598/IPv4-mapped、SS/Trojan/VLESS/VMess 错密码、SAN/CA/SNI、ws/http/httpupgrade Host、grpc 不可表达的独立 Host 拒绝、Hy2/TUIC UDP、目标私网/redirect、过期授权/取消；负例无目标到达/非法 socket/available/泄密，合法正例 204/Host/SNI/IP。`go test -race -count=3 ./internal/application/probe ./internal/probe/singbox` + `go build ./...` exit 0，保留每例证据；不以 BuildOptions 静态断言代替。
- [x] 2.2 【已有局部通过记录，非最终合并签名】进程级真实 Bearer + Idempotency-Key POST `/api/v1/probes/runs`→service→Runner→queue→callback/RunID，`os.Pipe`/`Cmd.ExtraFiles` 同步 SIGTERM 成功分支 callback 持久化→Drain→真实 DB.Close→clean→exit0 和超时无提前 Close/clean→自然非零退出；旧记录已通过，不因本轮规划撤销历史事实，但最终 diff 后必须在 4.1 重跑。
- [x] 2.3 【已有局部通过记录，非最终合并签名】queue Wait/Drain 局部证据保留，race/回调在 4.1 随最终变更重跑。

## 3. 完整响应式/视觉整改特性包（已有旧反馈，不等于最终版通过）

- [x] 3.1 【Publications 待证】`PublicationsView.vue` header Create/Download/Refresh 可换行且 375×667、392×872 真实 viewport 均可操作；五 targets 包括 Quantumult X 在本地可见/可滚及键盘可达，main 不被 746px 内容增宽；409 长文/预览只在局部单元缩放。Vitest 409→修订→预览/五目标/错误分类 + Playwright 实际 rect、点击和原图。
- [x] 3.2 【其余页面待证】Policy action row/Admission Rules 长 ID，Subscriptions grid/secret_ref/Popover 面板，Dashboard 两 badge 内容高度，Settings 有/无 token 中英状态一次显示、主题下拉；用既有 Vue/Tailwind/DaisyUI 与成熟定位能力修正，Vitest+Playwright 验证 375/392 边界、点击/键盘、菜单和 badge，无父级 hidden 假修复。
- [x] 3.3 【七路由待证】`App.vue` 从 navItems 给 Dashboard + 其他六路由清楚可辨的移动入口，点击/键盘全可达，dock 不压末尾动作，桌面导航/主题不回退；组件测试 + 375×667/392×872 真浏览器逐入口 rect、路由和间距。
- [x] 3.4 【自测须重跑】最终版在隔离工作区执行 `./node_modules/.bin/vue-tsc --noEmit -p tsconfig.json`、`npm test -- --run` 或既有 Vitest、在受控复制开关/隔离路径 `npm run build`、Playwright 三视口交互与几何，全部 exit 0；主仓资产前后路径/哈希相等。任何 CSS 不能在 jsdom 假量，编译红灯施工机原地修复。

## 4. 汇聚、最新同源隔离预览与终态独立验收（严格顺序）

- [x] 4.1 【冻结最终 diff 后必须重跑】1.4/1.5、2.x、3.x 施工和各自自测完成，再记录 Git status + 未跟踪源码/资产的逐文件 SHA 源清单、变更范围和依赖版本；同一冻结源执行 `go test ./...`、`go test -race -count=3 ./internal/probe/queue ./internal/application/probe ./internal/probe/singbox`、`go build ./...` 与 web vue-tsc/Vitest/隔离 build/Playwright，所有 exit 0，保存协议逐行 204/安全负例、停服事件、构建前后原仓资产不变。后续任何源变更重跑此项，旧 PASS 不可继承。
- [x] 4.2 【旧 4.2 勾选撤销：污染前旧快照不证明最新同源】不可仅沿用 `/tmp/csp-preview-refresh-20260924` 的 `sourceTreeSha256Before/After=9f359...`、binary `75b7...` 或旧 served hash。准备者用 4.1 冻结源码在新的唯一 `/tmp` 隔离工作区构建 frontend dist→隔离副本 embed dist→go:embed binary，启动独立 loopback URL/临时 DB/凭据/受控 fixture；记录源清单 SHA、三段产物 SHA、index 引用、实际 HTTP 响应 SHA、process/URL/health/auth、fixture 状态与截图/trace SHA。375×667、392×872、1280×800 对 Publications 409→修订→预览/五 targets、Policy/Subscriptions 菜单、Dashboard badge、Settings/主题、七路由及 401/403/500/网络分类实测 main/rect/scrollHeight/局部滚动末项，采本实例原始 PNG/DOM trace/几何 JSON。证明主仓原有资产/生产端口/卷不变；旧实例及旧 PNG 仅作为历史对照。健康/认证/同源或新原图缺一不得派 critic。
- [x] 4.3 【次序门禁须重新执行】只在 4.1 完成、4.2 同源新实例健康且冻结 diff 未变后，由独立只读 reviewer 审 Change/最终 diff/资产安全/协议/停服/页面并给 APPROVED，不在主仓运行有副作用 build；其后准备者将隔离实例 JSON、URL、原 PNG/trace 路径和 SHA 交独立 critic，用 `read` 直读 PNG + 只读 Playwright/像素/DOM，对同一实例出具 `CRITIC_FLOW_VERDICT: PASSED`、`CRITIC_VISUAL_VERDICT: PASSED`。此前 reviewer PASS 与 critic 双 PASS 时序不符合此门禁，不复用；任何 REJECTED/FAILED/BLOCKED/PRECONDITION_FAILED 回根因整改且源变后回 4.1。
- [x] 4.4 【终态仍未完成】核对 1.3–1.5、2.x、3.x、4.1–4.3 全部有本版证据，按隔离实例 owner 受控回收，确认原仓资产和生产无侵扰；否则只报告 PARTIAL/待授权事实，不勾总任务、不同步/归档/提交。另一 Pi 治理 Change 的真机 3.1–3.3 继续未完成，本地替代只记录其真实范围，绝不假勾物理验收。

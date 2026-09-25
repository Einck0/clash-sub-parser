## Context

本整改承接原 probe、生命周期及跨视口需求，不降低任何既有门禁。最新审计 `REJECTED` 的新增根因是审查在**共享主仓**执行了会写盘的 `npm --prefix web run build`：`web/vite.config.ts` 插件 `closeBundle` 在 `NO_COPY_WEBASSETS !== '1'` 时直接递归 `cpSync(web/dist, ../internal/webassets/dist)`；它不会先清空目标，却会覆盖同名文件并留下旧 hash 资产。未跟踪资产聚合哈希 `bac5...`→`6ec...`。当前备份 18 文件；主仓 22 文件：相对备份新增四个（`index-BLIeF39G.css`、`index-DiuVQvQe.js`、`index-CuRBRMN8.js`、`index-CY02MDnG.css`），其中前两者在当前 `index.html` 引用，且匹配预览 isolated workspace manifest；另两者既不在备份亦未由当前 index 引用，但**不等于可删除**。备份 `index.html` 引用原 `index-w-J7iDPQ.js`、`index-CEihJNxU.css`；主仓 index 改为引用预览的 `DiuVQvQe/BLIeF39G`。审计称旧 17 assets 字节匹配，须复算原始文件逐字节和历史基线哈希；哈希变化至少包含 index 替换、新增资产。不要将 `bac5...` 或外部备份未经认证即当成覆盖主仓的绝对真值。

`/tmp/csp-preview-refresh-20260924/instance.json` 的 `sourceTreeSha256Before/After=9f359...`、binary SHA `75b7...` 和 served index/JS/CSS hash 表明**预览快照在构建当时**一致；其记录主仓资产在预览构建前后均 `6ec...`，未证明该值来自预览脚本，也未证明现时源码与旧快照一致。预览 snapshot 构建于此次审计前，旧 reviewer PASS 与 critic 双 PASS 无法补全最终汇聚次序。`graphify-out/graph.json` 在当前范围不存在，不从知识图谱推断调用链。之前已排除“所有协议都不能连”（非 race 局部 204）、“仅用外层 overflow hidden 即可修复”（旧 PNG 裁操作）、“仅有截图就代表视觉通过”（旧视觉明确 FAILED）、“原仓额外资产未被 index 引用就可擅删”；仍待证的是额外两个 hash 文件来源和最终代码是否与预览一致。

## Goals / Non-Goals

**Goals:** 保护并恢复可核验的原仓用户资产、消除审查构建污染路径；既有六协议与安全负例及生命周期真实闭环；七路由、409→恢复、主题和移动可用；最终单一源码/嵌入产物/隔离 URL/原图身份链；最终 Go/web 全量门禁→独立只读 reviewer APPROVED→独立 critic FLOW/VISUAL 双 PASSED。

**Non-Goals:** 策略机不删改业务文件或原仓资产，不运行测试或构建，不触生产端口/卷，不给另一个 Pi 治理 Change 的真机验收代签，不凭无引用证明资产是垃圾。若归属待定，保护原状、列出决策点并寻求授权，绝不通过勾选任务假结案。

## Decisions

1. **取证先于资产修复，按文件和引用链判归属。** 由有授权施工者以 Go `sha256sum`/`find`/`diff` 或 Node 标准库对备份、主仓、当前 `web/dist` 与预览 workspace 列相对路径、字节 SHA、mtime 和 index 引用；保全所有待处理字节到带清单的只读隔离副本（须在授权范围），计算备份完整聚合 SHA 是否重现 `bac5...`，复核 17 原资产逐文件一致。沿 web/dist、已有证据 manifest、相关构建时间与任何其他工作树引用追溯 `CuRBRMN8/CY02MDnG`；明确**谁拥有原仓未跟踪文件、哪些要保留以及预期恢复目标**后才允许局部恢复。若最终需移走/删未跟踪资产，须主脑取得用户明确授权；未经授权不做，不能使用 `git clean/reset/restore` 或覆盖目录。恢复仅针对经确认的文件并防覆盖；验收含目录清单、每文件 SHA、index 所有引用可解析、原有 18 文件（含 index）完整、合法新产物按批准方案保留、原仓其他 status 不变。不可确认归属则保全证据并升级，不将复原算 PASS。

2. **构建钩子是全链路写权限边界。** 前端回归优先在 `mktemp` 隔离工作区复制必需源码/依赖、独立 output 与 embed，使用现有 Vite `NO_COPY_WEBASSETS=1` 构建纯 `web/dist`；若后端必须嵌入，则只在隔离副本把新 dist 安装进该副本 `internal/webassets/dist` 后运行 `go build ./...`。在隔离环境检查 Vite 钩子行为及其覆盖风险；生产是否需改 `vite.config.ts`（例如将资产复制作为显式 opt-in 且无默认写主仓），由施工者基于现有调用点/兼容性和测试证据决定，不能因此破坏正式嵌入构建。针对带 sentinel 的模拟目标目录运行“默认不写原仓、显式写只限隔离输出、并存旧 hash 时给确定结果”的测试；基线主仓路径集合及每文件 SHA 与构建后完全一致，且主仓 `git status --porcelain` 不增减业务文件。reviewer 使用只读代码审计与已有测试报告；如确需独立执行构建，必须在自有隔离副本、显式关复制，并验证主仓前后快照。`npm --prefix web run build` 在主仓默认模式不再是许可验证命令。

3. **保留完整网络 probe 和停服原门禁。** `internal/probe/singbox/protocol_handshake_matrix_test.go` 用标准 `sync/atomic` 或锁保护 `targetHits/observedHost`；取得旧 race SHA `263766475...` 原始双栈以区分计数器、HTTP2/gRPC late-reader、VLESS/httpupgrade EOF；优先经兼容验证的官方 sing-box 修复，必要时固定 upstream commit 做最小 patch，写端不等待读端 Setup。服务端等 body 才答复的 `httptest.Server` 证明不存在读写死锁；现有 `SafeNodeDialer`、`fetch.DefaultPolicy`、测试专属 TLS CA/SAN/SNI 和受控 registry/loopback mapper 构建合法公网 IP pin→真实六协议 inbound→HTTP 204、12 transport 与错误凭据/Host/SNI/私网/重绑等负例，逐行记录 socket/证书/Host。`cmd/csp` 使用 `os.Pipe`/`Cmd.ExtraFiles` 测试装配同步 Bearer+Idempotency-Key POST→service→Runner→Scheduler/RunID→callback 持久化→Drain→真实 DB.Close→clean→exit0，以及超时无提前 Close/clean 并自然非零退出；不能拿直接 hook Submit/GET 代替。2.2/2.3 有局部通过记录而最终版本仍须再跑。

4. **移动页面实际收缩与导航交互，不靠全局裁剪。** Publications header 三按钮组允许换行，五 target 优先 `flex-wrap`，若局部滚动须滚动提示/键盘/末项可见；Policy action row/长 rule 单元、Subscriptions grid/secret_ref 逐层 `min-w-0`，Popover 复用既有定位或 `@vueuse/core` 的能力而非自制通用定位框架；Dashboard badge 内容适配、Settings 状态避免重复、App 从同一 `navItems` 给 Dashboard 与其余路由可辨入口。用现有 Tailwind/DaisyUI/Vue Router/Vitest/Playwright。真实 375×667、392×872、1280×800 下逐元素测 main 宽、控件及菜单 rect、badge scroll/clientHeight、目标末项可达与 dock 间距；jsdom 仅验证状态和点击，不测几何。长文本、409→修订→预览、401/403/500/网络、主题/语言均在隔离受控 fixture 验证。

5. **确定性同源验收链按先后不可逆汇聚。** 各完整特性施工及自测完毕，最终 diff 冻结为源清单（包含未跟踪实现源码/资产、每文件 SHA 和 Git status，不单靠 commit SHA）；先跑 `go test ./...`、指定 race `-count=3`、`go build ./...`，web `vue-tsc --noEmit`、Vitest、隔离构建和 Playwright；记录命令、退出码、版本及原仓无写指纹。若有改动，全部门禁重新计数。准备者在隔离 tmp 工作区**由该冻结源码**重新构建前端并更新隔离 `internal/webassets/dist`，然后构建 go:embed 二进制、临时 SQLite/密钥/fixture 并绑定独立 loopback 端口；记录源清单 SHA→前端文件 SHA/index 引用→embed 目录 SHA→binary SHA→health/auth/实际 HTTP served SHA→此实例的 DOM trace/原始 PNG SHA、截图时间及 URL/进程身份。启服务后若做可变 fixture，保存场景状态与时间戳/重置证据；对最终二进制与服务资源做二次核验。**不可复用** `/tmp/csp-preview-refresh-20260924` 的旧快照身份，即使旧 health 可达且源快照构建前后一致；可复用脚本思想/fixture 实现，但必须给新唯一 instanceId 和最新快照。回归测试与截图必须对应同一冻结源清单；如果审查要求修代码，重新从汇聚门禁开始。

6. **reviewer 与 critic 分权顺序。** 新全量门禁成功并验证原仓资产/源码保护后，仅由只读 reviewer 对同一冻结 diff、Change、证据清单和构建安全审查，给 APPROVED；不得在主仓执行 npm build、改 dist 或勾任务。随后准备者提供已证明隔离、可达、认证与原始 PNG 的新实例 JSON 给 critic；critic 用只读 Playwright/像素/DOM 与 `read` 直接看原 PNG，分别出具 FLOW、VISUAL PASSED，涵盖七路由、五 targets、409 恢复、菜单、badge、Settings、探针安全负例对应门禁及三视口。审查或验收 REJECTED/FAILED/BLOCKED/PRECONDITION_FAILED 均回根因整改，不能保留先前终态签名；真机不可用不准借本地替代冒充物理验收。

## Risks / Trade-offs

- [未跟踪资产来源不清/外部备份不可信] → 先多来源逐文件比对与归属审批，歧义留原状并升级；不盲删两文件。
- [Vite 编译顺手覆写目标] → 主仓构建禁行、隔离 workspace+`NO_COPY_WEBASSETS=1`、前后每文件指纹；reviewer 独立运行任何有副作用命令同样遵守。
- [旧隔离预览被误作新版本] → 新 instanceId、冻结源清单、前端→embed→binary→served→PNG 哈希链，旧实例只用于失败取证。
- [矩阵只修测试 race 而漏 late-reader/EOF] → 原始双栈归因、204 与取消/先写后读夹具、每行独立真实 socket；失败不勾。
- [隐藏溢出假修复] → 实测每个控件边界、点击与原图；父级 `overflow-x-hidden` 不等于可达。
- [评审次序逆转] → 冻结源码、全量门禁、reviewer、critic 顺序写入证据账本；任何源改动强制重跑。

## Migration Plan

本 Change 仅规划；实施由获授权施工者按任务执行。先保全并判明主仓资产，分离构建写边界；Go probe/停服与前端可用性可在公共契约冻结后按无重叠写集合并行，各自测试闭环；汇聚后在隔离环境建同源新实例，全量复验、只读独立 reviewer、再 critic。对另一 Pi 治理 Change 只报告其 3.1–3.3 尚未完成及真机不可用，不改其工件。无原仓资产来源或删除授权时不得清理未跟踪文件；实施是否可继续由主脑按证据决策，不以暂停冒充完成。

## Context

同一 repo-local Change `csp-fresh-inventory-full-validation`，schema `spec-driven`；CLI 返回 changeRoot `/home/service/clash-sub-parser/openspec/changes/csp-fresh-inventory-full-validation`、allowedEditRoots `/home/service/clash-sub-parser`。本轮只修订既有规划工件，不派工、不构建、不网络实验、不改业务/测试/生产配置、不提交、不 QQ（父编排负责里程碑）。用户明确批准本修订和后续既有发布授权，不新增一般审批门槛。

### 最新证据与授权身份

指定预检 session `2026-10-08T05-50-14-467Z_879111db-a2c3b028-3b1a3e40-48b9.jsonl` 最终报告 `rep_exec_csp_continuation_preflight_1`：当前 run `pi_run_20261008_134941_3738865`；本规划会话环境没有不同 run_id，PI_SESSION_ID 为 `eace1c63-0de6-484b-8124-155fd0219b9b`，仅用于会话身份，不能当 run_id 或伪造子会话 ID。父编排继续真实 run 的合同/epoch，不自动继承旧 run `pi_run_20261008_080906_3465602`。

预检 HEAD `8b26f079e852d707c4cc2f74bbac6fb202c7ada4`，59 条状态、47 个 tracked 文件变更、暂存为空，旧相关施工进程定点检查不在。最后 runner/test 修改后验证链 45s 超时，build-final.log 空，未形成最终报告；此前 go test ./... exit1（共享缓存 linker 对象缺失），全量 race 120s 超时无成功退出码；定点 race 和较早编译成功不覆盖最终源码。Reviewer `rep_rev_csp_preview_code_review_2` 时间早于后续 05:40–05:48Z 修改，不能当最终 PASS。

旧实网 `/tmp/csp-real-comparison-20261008T045932Z/`：90 唯一 LogicalID，输入 SHA256 `f3d99066914c49e43019187560bbdd27a36941c971fd057ece7e15ec8842a2e5`，107537726 body bytes、900.027s、1620 终态；reference 两轮 baseline 仅 34/90，CSP 90/90，属于 PARTIAL，不覆盖/删除。run.py 只有计划；reference stage 串行，alive8/media2 未生效；samecore binary 是 v1.19.32 但 engine 标签硬编码 v1.19.31、modfile 改动了传递依赖；平台错误及非 collector 路径未完整导出/计量；实际 CSP baseline 默认 16MiB+探测字节并非 helper 的 64KiB。

预检仅 6.4GiB 可用磁盘。旧 preview `http://127.0.0.1:19080` 健康但不绑定最终源码，instance JSON 含 token 且0644；不能移用旧 Critic PASS。当前生产仍旧镜像，本次未上线。根 `.exp.md` 已完整读取，旧 Python/pytest/backend-data 经验应由后续文档责任人仅加 obsolete 前缀，不作 Go 部署指南；预检相关目录无额外经验，本 change 目录 `.exp.md` 不存在。

## Goals / Non-Goals

**Goals:** 保留 A/B 已有实现，补齐最新源码自测；ALL90 baseline 优先的公平四侧 comparison；native 与 same-engine 方法分开；硬限制整批 body/时间；解释逐节点差异和未执行项；最终独立 review、刷新预览、critic、授权 commit/rollout 与真实长期观察分阶段交付。

**Non-Goals:** 不 reset/import、再次修 Token/删除 kliq，不 blanket-enable 用户 group flag；不替换原生 Check 为 CSP 算法，不复制 GPL 探针进入 MIT 产品，不构建私有测试框架；新批不测速、无 speed URL/请求、无额外重试/自适应扩额/512MiB；不触碰 Pi/gateway/CPA/PDD 或无关服务。

## Decisions

### 1. 保留 A：字段/API/空组 PASS 与独立守卫

字段为 domain `NodeGroup.EmptyFallbackPass bool`、SQLite/JSON/API `empty_fallback_pass`，现有 migration `000017_empty_fallback_pass.sql` 保留，不重建/重复创建。旧行和创建默认 false；Get/List/Create/Update roundtrip，PATCH optional bool 区分 omission/false/true；filter/flag 验证在 metadata/revision 更新前，失败事务原子。现有 UI checkbox/文案明确继续规则匹配、非直连。

空定义沿用完成 admission/global/group/source/probe/risk 组装后的静态解析：Members=0 OR AllNodeLogicalIDs=0。configured 与 effective PASS 分开，只有明确配置的静态空组才输出 `proxies: [PASS]`、`empty-fallback: PASS`。非空勾选组不得有 PASS 成员或 runtime PASS fallback；原生非空 fallback 如需显式设置用拒绝，不允许 COMPATIBLE→DIRECT。未许可空组仍 required_nonempty/empty_routed_group；父组/derived projection 不继承许可，真实节点计数不变，PASS 非 LogicalID。

resolver/digest/compiler/capability/prune/publication compatible/preflight/frozen-content 一致，有效 PASS 不被 optional prune 消失；循环/悬挂/risk/source/probe/global filtered_nodes_empty 仍阻断；非 Mihomo 不能假映射。复用官方 Mihomo v1.19.32 离线规则 fixture：第一条 PASS 后真实落到后续非 DIRECT 控制规则，字符串包含 PASS 不构成证明。digest 随配置变化、同输入稳定，历史 publication 字节不改。

### 2. 保留 A：PCRE、校验上下文和 UI

优先 RE2，已知不兼容模式复用现有 regexp2 v1.12.0，有限正 MatchTimeout 和 pattern/input/cache 上限；Validate/Match 同引擎诊断，not_regex 不把错误/超时取反成通过。归档稳定身份恢复缺失“便宜”配置，保留已有用户非空 filter，事务幂等。原式 `去掉(流媒体|x(?:[0-5](?:\.[0-9]+)?)(?![\d.])|便宜|free )`、Eeox、einck 的 OR 语义不猜改；黄金语料 x5/x5.9/x6/x50/x5.9.1、literal 前缀、大小写、中文、恶意回溯，fixture 自建归档而非私有 DB skip。

policy ValidateGraph/publication 用相同只读 nodes/filters/NodeSources/LatestObservations/AsOf/risk 来源；仓储错误 incomplete，不假空/valid，无网络/写库。既有 regex/not_regex UI、warnings/manual validate、AbortController+seq+revision 保留；初始/保存/reload/retry 成功后最新 revision validate，错误不半保存、晚响应不覆盖。此前 PASS 后新 UI/locale/test 改动仍需最终独立 reviewer，不沿用旧 PASS。

### 3. 一个完整整改包：四侧 stage-first 与三种方法身份

入口 `tools/probe-comparison/run.py` 实现 plan/freeze/execute/report，复用 Python stdlib argparse/sqlite3、现有 Go testing/adapter/client，不新建调度服务或探针引擎。default network fail-closed。默认主比较四侧顺序 baseline A1(CSP)→B1(same-engine)→B2(same-engine)→A2(CSP)，每侧相同不可变 ALL90、90 唯一 LogicalID、connection_revision/full config fingerprint，不因前侧失败减少后侧 baseline。stage-first 保证全部四侧 baseline 终态后才计算 intersection `{node | 四侧 baseline 均成功且完整配置可比}`；空交集真实报告，不补入 discordant 节点。

- **CSP normalized stage control**：真实 runner/profile 必须使用相同任意2xx成功判定；非空 body 不自动失败，受硬预算；保存原204+empty策略版本、原始status/body和原因。
- **same-engine normalized comparator**：真实 upstream commit `3c320fd58aff5235e16218c050ec5b8ce587e233` 的独立干净私有 snapshot/薄 stage adapter，真实 CreateClient/platform；独立 modfile pin Mihomo v1.19.32，逐行保存相对 native 的 go.mod/go.sum、传递依赖 diff、go version -m/build flags/source SHA，修复硬编码 engine；不得宣称其为 unchanged native Check。
- **native Check control**：同一真实 upstream commit、原生 Mihomo v1.19.31 与 authentic native 配置，通过真实 `check.Check` 保留方法/默认行为。关闭 speed（空/禁用 SpeedTestUrl 并以 fixture 证明不发请求），冻结 GlobalProxies、空 SubUrls/SubUrlsRemote、KeepDays=0、SuccessLimit=0、空 Filter，关闭 Web/cron/save/upload/callback。baseline-only 原生配置允许，不能用 normalized stage control 冒充 native Check 或静默统一原生 defaults。

默认 manifest native_network_sides=[]，但 native build/Check 离线 fixtures 必验。可选 N1/N2 native baseline-only 两侧仅在首次外网请求前写入 manifest，验证时间/配额仍在同一总盘子且不损害主 baseline/platform 计划；否则不加，不能强制六侧。native eligibility 单列，绝不混入主四侧 intersection。native 全 media 实网不是新批必跑项，全部未尝试 native media 明确列出，不能宣称全原生 parity。

reference stage adapter 真正以有界 worker/semaphore 实现 alive concurrency8、media2，离线 blocking fixture 同时证明上限和确实可并行；声明配置但串行循环不通过。新批 speed concurrency0、stage清单无speed/risk中的speed请求，配置/日志/fixture均不得出现测速URL或请求。IPRisk 如被选为平台能力须完整计量并预先冻结，禁止借此附加新网络套件。

### 4. 不变 manifest 与比较条件

首次外网前冻结唯一 manifest SHA256，涵盖所有四侧和可选 native 两侧、ALL90 identity/config摘要、source/build/engine、AB/BA次序、实际target及子请求清单、timeout、TLS verification/SNI、redirect/HTTP2/UA/keepalive/selection、平台顺序、alive8/media2/speed0、逐node×side×stage body/time allocations、ledger绝对路径、原始总起点/截止、责任人与cleanup。秘密输入仅私有0700目录/0600文件；公开摘要无token/raw URL；凭据不经命令行明文。

comparative CSP/same-engine baseline统一target、15s请求超时（既有维护值）、TLS/redirect/HTTP2；平台每个实际子target/transport等先对齐再计为可比。不能仅统一核心版本。保留native默认差异表；不可对齐维度标 configuration/method mismatch，不混入公平成绩。具体endpoint/TLS值取最终真实输入冻结，规划不虚构未证实地址；execute不得自动补endpoint、改并发/TLS、重试或改变参数，manifest不匹配即fail-closed且无网络。

### 5. 共享跨启动 budget/deadline 与公平参与

总应用响应 body cap =134217728 bytes；整批首次运行开始到结束/cleanup ≤1800s，不是每launch/每侧各30min。baseline实际node×side所有响应body合计≤65536 bytes，实际CSP runner必须接到该值，不能只改helper/事后摘要；不能用extra probe byte突破上限。四侧最坏 `90×4×65536=23592960 bytes=22.5MiB`，默认platform余量 `110624768 bytes=105.5MiB`，四侧各 `27656192 bytes=26.375MiB`。可选两native baseline最多额外11.25MiB，必须从首次冻结的同一128MiB分配中扣除；不另开预算/时间。

平台额度四主侧完全相等，stage/node可预先冻结quotas算法，eligibility结算后将确定的参与表及SHA记为manifest派生工件，算法/次序不得自适应改动。每个common eligible node获得相同各侧额度和尝试机会：采用固定stage次序、同轮node顺序、逐node×stage×side子额度，不让前节点用完全盘；无额度/时间则skip精确列出。无跨side/stage临时借贷/隐藏重试。平台可能含Netflix/YouTube/Disney/OpenAI/Claude/Gemini/IPRisk，但只尝试manifest列明的能力，不承诺128MiB内全部原生media覆盖。

跨Python/Go子进程及重启复用同一个 ledger（优先stdlib SQLite事务，不依赖每进程重置的计数器）：保存run/manifest绑定、start/deadline、各side/stage/node预留/消费/退款及attempt身份；BEGIN IMMEDIATE/条件更新等成熟原子事务实现read-before-reserve、实际读取结算、未用退款。单个response limiter/lifecycle接到真实reader，所有成功/失败响应及重定向/内嵌client/非collector平台调用都进入同一账本。仅collector结果bytes不是完整计量；做不到全路径覆盖即保持network入口拒绝并整改。

timeout/cancel/deadline停止排队和新请求，取消在途且cleanup留在总截止内；崩溃未结算reservation不得乐观退还为可用额度，恢复只准保守核对/结束，不自动重试。body实际消费、预留和可用余额不越界，NIC/header/TLS/transport另列且有测量局限，不冒称硬NIC全流量上限。阶段预算截断返回预算终态，不直接改写存活策略或永久dead。

### 6. 逐节点执行与缺陷解释

node×side×stage×attempt恰一终态，保留queued/not_scheduled/executed、原始verdict/status/typed errors、配置和source/profile/target/engine/commit、时间/body/reservation/refund、取消/复验关联。dependency_skipped保留先前失败根因，不由后来的deadline/budget合成覆盖；restricted/unknown/fail/cancelled/budget_not_executed/stale/configuration_mismatch分账。errors.Is/As区分net/x509/context等，不猜DNS/TLS，不漏非collector的native平台返回错误及内部请求。primary discordant清单单独解释，诊断不授权额外网络尝试。

所有原始失败/skip不可覆盖成success；旧partial证据保留。比对结论按已执行且条件对齐的阶段/节点，partial不声称全量parity。发布决策看解释清楚的缺陷及是否影响正确性/安全，不要求headline counts一致；真实缺陷先整改/retest/review，方法差异和不执行项如实记录，不能以partial为假阻断或假PASS。

### 7. 离线确定性门禁与资源边界

使用成熟 Go testing/t.TempDir/httptest、Python unittest黄金分配/调度测试、Vitest/Playwright及官方Mihomo；禁止新测试框架。自测无外网/商业源/生产DB：`GOPROXY=off GOTOOLCHAIN=local`、私有GOCACHE/GOTMPDIR，并将fixture网络限定loopback（禁外部egress）；空订阅Check、无测速、budget漏路与默认network拒绝以fail-on-network fixture验证，不把env变量当OS网络沙箱。

需要最终源码的 `go build ./...`、`go test ./...`、`go test -race ./...` 全量命令证据；不能用targeted race文本替代full。若full race真实受复现资源/工具链限制，保留非0/超时、确切命令/版本/资源日志/复现步骤，相关checkbox不勾且报告unchecked；由父编排明确裁定该环境局限，不伪造全部门禁通过。full build/test失败继续本地整改，不能只写环境结论。

reference native和same-engine分别离线build/test/fixture（含Check、transport、无speed、metadata/pin diff与计量所有路径）；Python golden覆盖四侧/可选两native预算、交集、ABBA stage顺序/公平participation、cancel/deadline、manifest篡改默认拒绝、跨launch共享ledger并发/退款/崩溃保守；Go覆盖实际64KiB runner而非helper、2xx/status、typed原因、超额/取消/race。Web type-check/tests/开发build禁COPY，最新UI变更再review。

磁盘预检仅6.4GiB，executor先定点资源复核、限制单写build与GOMAXPROCS/test -p等实际资源参数、复用已核实离线模块；不删除共享cache、不盲装/下载、不漫游查环境。每个长命令合理timeout和稳定日志，用工具执行输出/进度记录观察，不高频轮询机械重试。私有cache/tmp/log预算在可用空间内留余量，全部artifact先记录owner后cleanup；缺cache或真实外部硬边界交回父编排，不扩授权。

### 8. 拟定确切命令接口（待施工实现，不冒称已经存在）

在repo root：

```
python3 tools/probe-comparison/run.py plan --inventory <private-all90.json> --output <private-plan.json> --response-body-budget-bytes 134217728 --deadline-seconds 1800 --baseline-body-limit-bytes 65536 --alive-concurrency 8 --media-concurrency 2 --speed-concurrency 0 --native-baseline-sides 0
python3 tools/probe-comparison/run.py freeze --plan <private-plan.json> --manifest <private-manifest.json> --ledger <private-ledger.db>
python3 tools/probe-comparison/run.py execute --manifest <private-manifest.json> --ledger <private-ledger.db> --output <private-results-dir> --allow-network
python3 tools/probe-comparison/run.py report --manifest <private-manifest.json> --ledger <private-ledger.db> --results <private-results-dir> --output <sanitized-report.json>
python3 -m unittest discover -s tools/probe-comparison -p 'test_*.py'
GOPROXY=off GOTOOLCHAIN=local GOCACHE=<private-cache> GOTMPDIR=<private-tmp> go build ./...
GOPROXY=off GOTOOLCHAIN=local GOCACHE=<private-cache> GOTMPDIR=<private-tmp> go test ./...
GOPROXY=off GOTOOLCHAIN=local GOCACHE=<private-cache> GOTMPDIR=<private-tmp> go test -race ./...
```

`--native-baseline-sides` 只允许0或2；2必须plan-time可行性验证通过，否则拒绝。plan/freeze/report绝不网络；execute缺--allow-network、pin不符、manifest不完整/变更、预算超128MiB、speed>0/URL存在、deadline>1800、ledger绑定不符或自测/独立review未通过全部拒绝且不创建请求。显式allow-network也不绕过门禁。freeze输出完整实际child argv/schema（包括native Check/native config vs normalized stages）、二进制摘要，execute只接受已冻结参数；不能按新flag私改命令或自动探测endpoint。Go adapter基于既有入口扩展这一数据契约，需README列明真实参数与fixtures；本规划不猜未取证的入口名。网络执行仅父编排在最终review后启动，同一批准总盘子，没有计划外probe。

Web在web目录执行 `npm run type-check`、`npm test`、`NO_COPY_WEBASSETS=1 COPY_WEBASSETS=0 npm run build`；private native snapshot 用 `GOPROXY=off GOTOOLCHAIN=local go build ./...` / `go test ./...`，same-engine同样命令加 `-modfile=<frozen-samecore.mod>`，先记录必要`-mod=mod`产生的diff再冻结，禁止执行阶段更新modfile或联网。全部命令记录真实exit_code和日志/source指纹。

### 9. DAG、合同与终态

一个完整B纠偏包保留A/B源码，包含必要A最终回归、Python scheduler、真实reference+same-engine+native fixtures及全量自测；不按函数/checkbox派微任务。单写模块/schema/ledger和generated证据，不伪造并发安全。完成→独立最终Reviewer→已批准实网staged batch→最终源码冻结刷新preview→Critic→assets/commit→backup/app-only rollout→独立7200s观察；实网引出源码变化须回full self-test/review，再刷新preview。

父编排运行合同version1：真实run_id（当前报告值如上，以实际运行确权）、同一change/changeRoot与repo root分开、既有A/B全部work_packages及epochs、自测/reviewer必选、critic applicable=true（policy UI/发布真实体验）。本规划不改run metadata、不宣布PASS。派工必须包含run_id/change/work_package/epoch/reverification_of；五要素executor报告与结构化commands真实退出码、独立review、critic真实证据，历史失败引用保留。

preview准备者建立private DB/端口、禁止自动refresh/probe/periodic外网，收紧含凭据JSON为0600（对外实例不含token），最终binary/frontend SHA与sourceManifest绑定最终diff。旧19080健康不能替代刷新实例。instanceId/url/health/allowedOrigins/testDataBoundary/scenarios/shutdownOwner/cleanupToken/sourceManifest完整后才交Critic。Critic只读Playwright+read PNG，既有375/390/1440视口无遮/溢出、PASS持久化/非空/关闭、regex编辑、reload/retry/竞态/最新issues、发布preview与probe状态解释；fixture非商业解锁证据。

Reviewer与Critic后准备者以 `COPY_WEBASSETS=1 NO_COPY_WEBASSETS=0` 同步最终嵌入资产，核对与预览内容绑定且无额外源码变更；根编排仅git_commit授权CSP路径，不夹带无关/用户staged变更。`.exp.md`旧Python段落仅加obsolete标识并限定不指导当前Go部署，不无关重写。

部署严格依 `docs/controlled_deployment_runbook.md`/operations：SQLite官方online .backup，size/schema/integrity/FK/SHA/0600/owner、新schema兼容isolated rollback演练与当前image rollback tag；app-only/no-deps/no-build，不down/delete volume/reset/import/回灌旧库存/代替用户开启flag。验证双loopback healthz/readyz、401合法鉴权、schema/assets/FK0与合法管理/发布操作。rollout健康只是短时上线证据；9.2必须另交真实起止≥7200s周期日志，不能靠health poll或隔离加速冒充完成，无限轮询禁止。未观察完如实partial，不撤销已有发布授权或伪勾9.2。

## Risks / Trade-offs

- 128MiB是应用body总额而非NIC；native平台多子请求可能无法全部覆盖，明确unattempted而不自动扩额。
- comparative归一化不能冒称原生方法；native与same-engine依赖/默认差异分开，不凭headline计数判release。
- 崩溃reservation保守占额可能减少剩余覆盖，但避免跨launch超额与隐形重试。
- full race环境限制必须可复现且unchecked；旧targeted PASS、空日志和超时不是成功。
- PCRE兼容非完全等价；指定语义/黄金数据验证，超时fail-closed。
- 本轮READY仅规划修订完成，实施/review/实网/critic/commit/部署/长观察仍须真实证据。

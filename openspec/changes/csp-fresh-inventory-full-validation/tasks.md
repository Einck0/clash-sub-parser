## 执行范围、最新授权与证据状态

2026-10-08 用户已接受：ALL90 baseline先行，四侧CSP/same-engine AB/BA后在交集做platform，discordant单列解释，新批无speed；整批全部launch/side body≤128MiB、含清理≤30min，不512MiB/额外重试/自动扩额。原实施与发布授权保留，按已解释缺陷而非headline计数判断release。只修订同一Change，不新项目；本轮不派工、不实施/测试/实网/preview/提交/部署/QQ。

保留现有A/B、所有历史失败/partial、用户worktree。以下18个历史[x]及12.1–12.4的已有实现记录保留，不证明最后源码自测或最终review/critic。7.1旧“严格控制”统计不等于跨launch硬限额；9.1旧部署不是本次上线。最新预检 `rep_exec_csp_continuation_preflight_1` / session `2026-10-08T05-50-14-467Z_879111db-a2c3b028-3b1a3e40-48b9.jsonl` 报告run `pi_run_20261008_134941_3738865`，本轮环境无不同run_id；不虚构。预检只读成功，不是executor完成报告。

13.1–13.3撤销勾选：实际stage-first executor不存在、reference并发未生效、64KiB未接实际runner、same-engine标签/pin diff不完整、非collector平台错误/计量未全覆盖、跨launchledger未证明。12.5/13.4/14.1仍未完成：最后源码验证45s超时，较早full functional exit1（共享cache linker缺对象）、full race120s超时；较早targeted PASS不是full/final PASS。14B.1旧partial保留，不是新批通过。

## 1. 节点库存清空闭包核心与仓储实现

- [x] 1.1 在 SQLite 仓储层实现原子节点重置闭包、受影响绑定捕获与前置/后置外键完整性校验，并通过单元测试验证
- [x] 1.2 在 inventory 应用服务层实现 ResetNodeInventory 业务逻辑、不可变发布解耦与旧抓取指针切断，保持抓取审计事实，并通过服务测试验证

## 2. 订阅源 Token URL 精准修复

- [x] 2.1 实现基于冷归档稳定身份证据的 7li 与魔戒 URL 精准恢复、幂等性跳过与脱敏审计，并通过单元测试验证
- [x] 2.2 验证其余 7 个订阅源状态与配置严格不变，且日志与输出完全脱敏（包含路径 Token 掩码）

## 3. 运维 CLI 命令与服务复用

- [x] 3.1 在 cmd/csp 中新增 reset-node-inventory 维护子命令，支持 --db, --dry-run, --apply, --confirm-backup, --backup-file, --restore-source-tokens，并通过 CLI 测试验证
- [x] 3.2 验证 dry-run 模式只读不写与 apply 模式的备份硬确认、原子提交与结构化 JSON 报告输出

## 4. 协议扩展与多凭据连接身份区分

- [x] 4.1 实现 HTTP, SOCKS5, VLESS, AnyTLS 协议扩展与 Mihomo 适配支持
- [x] 4.2 实现 domain.ComputeConnectionLogicalID 区分同端点多凭据节点，通过 fresh_private_test.go 门禁与单元测试验证

## 5. 隔离应用工作流自测与门禁闭环

- [x] 5.1 实现 same-service 隔离刷新工作流测试（涵盖 success/empty/fail/notice/manual/multi-credentials 与 last-good 保持）
- [x] 5.2 运行 go build ./...、全量测试（含 -race）与前端 type-check/tests，确保退出码均为 0

## 6. 全新订阅源全量拉取与失败根因归因分析

- [x] 6.1 对 4 个已启用订阅源（7li、魔戒、Dogegg、einck-qzz）执行全新抓取，验证节点解析入库（抓取 95 个候选节点，去重入库 90 个全新有效节点，全部标记 active=1，5 个禁用源严格未触碰）
- [x] 6.2 针对任何抓取失败或异常响应输出详尽根因归因分析与诊断记录（7li 返回空内容，归因诊断为订阅内容为空，如实记录失败而未伪造旧节点；魔戒 44、Dogegg 18、einck-qzz 33 成功入库）

## 7. 公平探针评测与发布候选筛选

- [x] 7.1 在隔离环境中对全部新节点执行公平 Benchmark 探针连通性与延迟评测（全量 90 节点完成 540 项有界任务：69 存活，385 成功，50 失败，105 跳过，耗时 5m54s，下载 54.5MiB 严格控制在 128MiB 预算内）
- [x] 7.2 依据策略规则与准入规则筛选 publication candidates 候选节点集（候选节点按规则流向汇入香港、台湾、日本、新加坡、美国、加拿大及其他组）

## 8. 最终源码绑定的隔离预览与成品验收

- [ ] 8.1 最终自测/14.1 review及14.2 staged batch完成后冻结源码，准备者刷新独立DB/端口preview，禁自动外网refresh/probe；不重用旧19080未绑定源码实例作为凭据。交付instanceId/url/health/allowedOrigins/testDataBoundary/scenarios/shutdownOwner/cleanupToken/sourceManifest及binary/frontend SHA；含token私有JSON0600，交付公共合同不带秘密；只在指定fixture组opt-in，生产用户flags不变。
- [ ] 8.2 URL-ready且最终Reviewer后，Critic只读Playwright+read PNG验证既有375/390/1440视口无溢出遮挡、checkbox保存重载/空非空/uncheck、regex编辑、retry/manual/晚响应与最新issues、PASS非DIRECT发布预览及probe状态解释，交真实截图/几何证据/CRITIC_VISUAL_VERDICT。无URL/隔离/责任人不派工；旧Critic不替代最终源码验收。

## 9. 生产发布与长观察分开

- [x] 9.1 执行生产环境服务发布与节点重置切换（完成旧 1014 节点清空、两源 Token 修复、15 策略组正则过滤恢复、1 条 kliq 规则精准删除，生产容器已成功滚动至 414a6b7 最新镜像并恢复健康双端口服务）
- [ ] 9.2 15.2 rollout健康后另交7200s真实生产调度/起止时间与有界观察记录，关联已解释配对结果。health poll/隔离加速不能冒充7200s完成；未满周期保留unchecked并区分“已上线/观察未完成”，不无限轮询。真实外部auth/设备边界交父编排，不绕鉴权、不在聊天索要明文凭据。

## 10. 已实现规则只读校验与管理

- [x] 10.1 后端 Policy 校验与 Resolver 增强：在 resolver 与 policy 服务中将直接被分流规则指向的空组标记为 error（无论是否有 filter），输出结构化 issues（包含 rule_id, position, type, value, target_group_id, target_group_name, severity, message）并返回 revision_id，保持既有 valid/errors 字段兼容
- [x] 10.2 规则精准删除与事务安全测试：针对规则 01a0b9af-c118-72f9-9949-d94b49fa6ec2（指向“其他”组）编写精准删除与验证测试，确保 1->0 规则扣减、新 revision 生成、策略组保留与历史不可变发布快照隔离
- [x] 10.3 前端 PolicyView 增设分流规则 Tab、手动校验按钮、自动校验触发（初始/刷新/修改后）及防竞态守卫（AbortController + 请求序列），并通过类型与构建验证

## 11. 当前run合同与完整工作包冻结

- [ ] 11.1 父编排确权当前真实run并冻结version1 completion_contract、同一change/changeRoot/schema、A/B全work_packages/epochs、executor/reviewer必选及critic applicable=true/UI理由，派工显式run_id/change/work_package/epoch/reverification_of。本轮规划不写run metadata。确定单写公共类型/schema/模块/ledger/generated ownership、design §8真实CLI接口和network fail-closed；保留旧报告和失败引用。不开新项目/Change/历史run，不把preflight当完成。

## 12. 保留特性 A：policy-empty-pass-and-filter-fidelity

以下为已有完整特性，不重写；必要整改和最终回归纳入本次完整纠偏包，不按checkbox拆派。

- [x] 12.1 实现 empty_fallback_pass 默认 false 的 domain/SQLite 增量 migration/仓储/Create-Update API/UI checkbox 全链路，旧 DB、创建默认、Get/List 与 PATCH omission/false/true roundtrip；失败更新不改 metadata/flag/filter/revision，生产原 flags 不 blanket-enable。
- [x] 12.2 在 resolver/digest/compiler/prune/publication 所有门禁共享显式有效 PASS 语义：只有明确配置的静态空组可用 PASS，非空无 PASS、未勾选仍 required_nonempty、真实节点计数不变；父组/derived 不自动继承，graph/risk/capability/source/probe/global filter 守卫保留，兼容模式无 DIRECT/COMPATIBLE 偷渡，Mihomo 离线真实规则 fixture 证明 PASS 命中后续非 DIRECT 控制规则。
- [x] 12.3 复用 RE2 与 regexp2 有限 MatchTimeout/有界 cache-input，Validate/Match 同语义、not_regex 超时不放行；用自建归档 fixture 和原 PCRE 黄金语料保真恢复缺失“便宜”过滤器（稳定 ID/幂等/保留现有配置），不猜 literal“去掉”含义、不删 lookahead；UI 支持 regex/not_regex 创建编辑保真。
- [x] 12.4 修复 reload/retry→最新 revision→validate，沿用现有 warnings/manual/Abort-seq 守卫；校验与 publication 同一只读 NodeSources/LatestObservations/risk/filter 装配，repository 故障显式 incomplete；验证 stale、失败保存、错误重试与每条规则真实 issues。
- [ ] 12.5 最新源码重新完成A回归与Web type-check/tests/NO_COPY开发build，保留原生PASS与PCRE/事务/仓储失败fixture。go build/test FULL成功及full race真实凭据由13.4统一取得；targeted race不是full PASS。full race资源限制必须确切可复现/日志与unchecked，不以文字宣称完成；旧PASS之后的新UI改动包含在14.1最终独立review。

## 13. 完整特性 B 纠偏包：probe-normalized-reference-and-attribution

13与14B是同一个feature-level executor工作包内实现/自测事项，不拆成微子机，不重置A/B。共同契约冻结后单写推进；共享源码/ledger/generated无独立并行支路。

- [ ] 13.1 完成真实runner/profile任意2xx normalized策略、实际node×side baseline≤65536 body bytes；固定upstream commit3c320fd58aff5235e16218c050ec5b8ce587e233，独立标记native Mihomo1.19.31 Check与same-engine1.19.32 stage comparator，保存samecore modfile/全部传递依赖diff、真实build metadata/pin，修硬编码engine；原生Check保持authentic config/defaults且speed disabled，baseline-only native config允许，无订阅重抓/Web/cron/upload/save/callback。
- [ ] 13.2 完成跨launch shared aggregate ledger、原始start/deadline、原子read-before-reserve/settle/refund、崩溃reservation保守恢复；全部成功/失败body和native非collector平台/redirect/内嵌client进入同账本，body/NIC分开。整批134217728 bytes/1800s含cleanup，实际reader无extra-byte超限；无法覆盖路径network fail-closed先整改，不用collector事后汇总冒充硬限额。
- [ ] 13.3 全node×side×stage×attempt queued/not_scheduled/executed、verdict/reason/status/profile/target/engine/commit/revision/config指纹/时戳/body/复验关联恰一终态；dependency优先保留根因、typed net/x509/context错误及native平台原始返回、非collector完整导出；unknown/restricted/skip/cancel/budget/stale/mismatch不混fail/永久dead。secret脱敏、cleanup与失败历史保真。
- [ ] 13.4 correction后私有有界cache离线完成 `go build ./...`、`go test ./...` FULL和 `go test -race ./...` FULL、Python unittest golden/scheduler/ledger、native与same-engine reference各自build/test/Check与stage fixtures、最终Web门禁，五要素报告+实际exit/log/source指纹。预检可用磁盘6.4GiB，资源先核对、限制并发/留余量、不删共享cache/联网补依赖，合理长命令timeout和可见进度不机械轮询。full race若真实环境限制只报告可复现unchecked并保持本项未勾，由父编排裁定适用性，不伪造全通过。

## 14B. 同包根因整改设计与验证（旧证据保真）

- [x] 14B.1 保存真实旧 AB/BA partial：90节点、1620终态记录、107537726 body bytes（102.56MiB），900.027s；不覆盖原记录、不宣称parity，逐节点私有证据句柄进入 docs/evidence-final-validation.md。
- [ ] 14B.2 实现 `tools/probe-comparison/run.py` stage-first执行器：主四侧ALL90 baseline A1→B1→B2→A2全覆盖终态后，固定四侧baseline intersection作为平台共同eligibility；同stage公平相等side allocation与per-node participation，discordant另列诊断。新批speed0/无speed URL和请求；alive8/media2 reference worker真正生效并由blocking offline fixture验证非串行/不超上限。默认4侧22.5MiB最坏baseline，platform105.5MiB四侧均分；native可选0/2两侧baseline-only在首次冻结同一128MiB/30min总盘子内可行且不伤主覆盖才加，不强制6。
- [ ] 14B.3 冻结所有side/stage/node manifest/identity/SHA与派生参与表算法、exact child argv/target子请求、deadline/timeout/TLS/redirect/HTTP2/selection/UA；comparative CSP/same-engine统一条件，native defaults差异单列。execute禁止自适应endpoint/并发/TLS修改/未记载重试或扩额；平台未尝试项/native未选media精确清单，离线证明所有平台调用预算路径（含非collector）与取消/cleanup。
- [ ] 14B.4 按design §8实现明确plan/freeze/execute/report参数：plan/freeze/report无网络，execute必须allow-network且审核通过/manifest匹配/ledger绑定/speed0/body≤128MiB/整批≤1800s，非法输入发请求前拒绝；Python黄金allocation/stage顺序/交集/side/node公平/可选native扣额、Go实际64KiB与跨launch并发reservation/refund/cancel/崩溃、native Check真实baseline-only/禁speed、samecore diff与真实metadata fixtures均离线验证。
- [ ] 14B.5 与12.5/13.4统一完成最新源码全量静态/功能/race或诚实unchecked环境证据、reference/Python/Web；网络仍默认关闭直到14.1独立review通过。不存在512MiB REQUESTED计划或新测速suite；新批授权仅同一128MiB/30min staged方案。

## 14. 汇聚最终独立审查与已批准实网分阶段batch

- [ ] 14.1 全部施工自测完成后独立reviewer对同一Change最终diff/API/migration/原生PASS/PCRE/校验输入/UI新增变更/stage-first/actual64KiB/跨launchledger/reference并发/所有noncollector计量/native与samecore来源/no-network safety复验，真实verdict/commands exit/log；旧rep_rev_csp_preview_code_review_2不覆盖新源码。历史失败用reverification_of/final=true明确消除，环境unchecked不得冒称full门禁通过；失败主脑根因整改后再施工/独立review。
- [ ] 14.2 Reviewer后执行已批准stage-first batch：前置冻结exact ALL90/所有sides manifest与ledger/start/deadline，默认CSP vs same-engine ABBA四baseline，native两侧仅可选固定总盘子；之后common intersection平台，alive8/media2/speed0，无测速请求/重试/512MiB/自动扩额，无publish/生产stored-state mutation/reset/import/商业源刷新。整批含cleanup≤1800s/body≤134217728，公平side/node配额；输出实际executed/failed/skipped/cancelled/unattempted、discordant与缺陷解释/条件不可比、资源/queue/请求/body/NIC/成功分位和删失样本。不能保证全部native媒体完成，不声称全量parity；release按解释清楚缺陷而非headline计数。产生代码变更回full自测/review并刷新preview。

## 15. 既有授权终态（最终review/critic后才动作）

- [ ] 15.1 最终源码/preview/Reviewer/Critic真实通过后准备者明确 `COPY_WEBASSETS=1 NO_COPY_WEBASSETS=0` 同步嵌入assets，与最终preview/build指纹绑定、无额外源码；根编排只git_commit授权CSP路径、核对staged不夹无关改动、记录hash。经验维护责任人仅给 `.exp.md`旧Python/pytest/backend-data段落加obsolete前缀与当前Go部署不适用说明，不无关重写；父编排按授权核对真实经验变更。源码追加变化回完整门禁。
- [ ] 15.2 按真实controlled_deployment_runbook/operations做SQLite一致性online hot .backup，size/schema/integrity/FK/SHA/0600/owner核验、现image rollback tag与schema17兼容隔离演练；app-only/no-deps/no-build上线及双loopback healthz/readyz/401/schema/assets/FK0和合法管理/发布核对。保持用户group flags、源启停与业务配置，无reset/import/down/删卷/回灌/重复删规则；只CSP，不触碰无关服务。记录rollout完成，7200s观察归9.2另报。

## 写集合、验证与单写责任

保留原授权A候选文件：`internal/domain/{policy,node_filter}.go`、migration000017、`internal/repository/sqlite/policy.go`、`internal/application/policy/{types,service}.go`、`internal/transport/http/policies.go`、resolver types/graph/digest、compiler compiler/mihomo/prune、publication service、inventory filter_recovery与colocated tests；Web policyTypes/usePolicy/PolicyEditorSheet/PolicyView/GroupCard及tests、zh-CN/en-US locale与tests。现有go.mod/go.sum无目标升级，不复制GPL算法；新公共schema先11.1确认单写。

B候选：`internal/application/probe/runner.go`与tests、`internal/probe/platform/{alive,pool,speed,claude,disney,gemini,iprisk,netflix,openai,youtube}.go`与tests、profiles、Mihomo client、domain probe、inventory maintenance、cmd/csp maintain与tests；`tools/probe-comparison/run.py`、现有adapter/README/fixtures与Python test_*.py（精确已有入口由executor定点确认，不私造引擎）。private native/same-engine source/test adapter、modfile diff/ledger/result不复制进产品；原 `/tmp/subs-check-beck`缓存不覆盖。需要新增公共类型/仓储/schema须先确认11.1边界，不能盲扩大。

汇聚单写：最终证据/runbook（当前Go正确说明）、`.exp.md`局部obsolete标记、`internal/webassets/dist/**`按15.1阶段，不并发生成。exec只勾真正已完成项，不代勾review/critic/实网/commit/deploy/长期观察。

**验证入口与默认无网络**：design §8明确plan/freeze/execute/report及byte/concurrency/timeout参数为待实现接口，不能在旧run.py冒称已支持。离线 `python3 -m unittest discover -s tools/probe-comparison -p 'test_*.py'`、GOPROXY=off/GOTOOLCHAIN=local/private cache的Go FULL build/test/race、native与samecore各自build/tests；Web type-check/test/NO_COPY build。Loopback fixture允许，不访问外部egress/商业源/生产DB。每命令真实exit+稳定log/source SHA，cleanup责任和预算由完整包承担。

**DAG**：11.1→同一完整纠偏包（保留A/B，12.5+13+14B自测）→14.1独立final reviewer→14.2已批准staged batch→最终源码冻结8.1刷新preview→8.2 critic→15.1正确assets/授权CSP commit→15.2热备rollback/app-only rollout→9.2真实7200s观察。READY是本轮规划完整，不是任何执行gate PASS。

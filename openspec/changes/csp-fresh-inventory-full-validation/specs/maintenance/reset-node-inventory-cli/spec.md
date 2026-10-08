## Purpose

规范已实现库存维护CLI与本续作隔离比较。历史reset/import/规则删除已完成，不重复执行；保留现有A/B，新增ALL90先行stage-first、真实native与same-engine方法分离、无测速、整批跨launch预算和可审计归因。

## ADDED Requirements

### Requirement: 维护 CLI 命令参数与安全预检
系统 SHALL 在 `cmd/csp` 提供 `reset-node-inventory`，支持 `--db`、`--dry-run`、`--apply`、`--confirm-backup`、`--backup-file`、`--restore-source-tokens` 及 `--archive-db`。未指定apply或dry-run MUST 报错；apply无有效一致性SQLite backup与确认 MUST 拒绝；所有模式 MUST 前置外键检查，脏外键阻断。本续作 MUST NOT 重复reset/import/Token修复或kliq删除来解决策略/探针问题。

#### Scenario: Dry-run 不修改数据
- **WHEN** 执行 `csp reset-node-inventory --db <isolated-path> --dry-run`
- **THEN** 只读输出变更计数与外键结果，数据库内容不变

#### Scenario: Apply 缺备份拒绝
- **WHEN** apply无confirm-backup或有效backup-file
- **THEN** 退出1并说明备份确认要求

#### Scenario: Apply 原子报告
- **WHEN** 历史维护apply具有有效确认/backup
- **THEN** 单事务成功后报告前后计数/FK0，失败完整回滚；此能力不授权本续作再次执行

### Requirement: 隔离验证与成熟 Wiring
CLI SHALL 复用现有SQLite/application、Go testing/httptest、Python unittest/sqlite3、Vitest/Playwright与官方Mihomo，不新建旁路控制面、私有测试框架/探针引擎。验证 MUST 使用独立SQLite/端口，开发build MUST 禁止嵌入资产copy，最终review/critic后才显式COPY_WEBASSETS=1同步；生产用户flags与既有库存 MUST 保持。

#### Scenario: 隔离fixture与产物责任
- **WHEN** 执行离线自测或刷新最终preview
- **THEN** 无生产/无关服务变更，无商业源/外部egress，仅loopback fixture；私有0700目录/0600秘密文件，owner/cleanup明确

### Requirement: 三种真实方法与source/engine身份分开
比较 SHALL 固定真实upstream commit `3c320fd58aff5235e16218c050ec5b8ce587e233`，薄adapter调用真实Check/CreateClient/platform，不复制算法。native Check MUST 使用原生Mihomo v1.19.31与authentic native配置、保持原方法/defaults差异且speed disabled；允许真实baseline-only native配置。same-engine v1.19.32 normalized stage comparator MUST 单独标记，保存native对比modfile/go.sum/传递依赖diff、真实engine/build/source metadata，不硬编码1.19.31。CSP实际runner/profile SHALL 任意2xx normalized alive且受实际body limit，保留原204+empty来源、status/body/typed错误；native Check与normalized stage控制 MUST NOT 混称。

#### Scenario: 原生Check无副作用/无speed
- **WHEN** 离线fixture冻结GlobalProxies、空SubUrls/SubUrlsRemote、KeepDays=0、SuccessLimit=0、空Filter、关闭保存/上传/callback/Web/cron与speed
- **THEN** 真正Check不重复fetch、不测速且保留native方法；baseline-only控制不假称full-media结果

#### Scenario: same-engine pin与实际引擎
- **WHEN** 构建v1.19.32比较器并运行离线fixture
- **THEN** 真实metadata和source/modfile diff可复验，报告distinct same-engine标签，宿主CLI1.19.30不得冒充任一in-process引擎

#### Scenario: 200非空与204
- **WHEN** comparative客户端收到200非空、204空、非2xx或预算截断
- **THEN** 2xx遵循统一normalized成功策略并保留body/status，预算截断单列预算终态，不静默改变native方法或以一次失败宣判永久dead

### Requirement: ALL90 baseline四侧优先与真实reference并发
新batch SHALL 先运行相同ALL90完整输入的四主侧baseline A1(CSP)→B1(same-engine)→B2(same-engine)→A2(CSP)，不能因前侧失败减少后侧baseline。四侧终态后 SHALL 用四baseline成功且配置可比的交集作为平台共同eligibility，discordant另列解释。actual runner node×side baseline body MUST ≤65536 bytes；alive concurrency8/media2 MUST 实际执行，reference不允许只声明并发但串行循环。新batch speed concurrency MUST 为0、无speed stage/URL/请求。

#### Scenario: 全baseline后公平平台
- **WHEN** 四侧baseline完成或形成准确取消/未执行终态
- **THEN** 先交付ALL90各侧覆盖，交集上的各节点得到同等各side预算/参与机会；非交集保留差异诊断，不自适应增加网络诊断

#### Scenario: reference并发实际生效
- **WHEN** blocking离线fixture检验reference alive/media调度
- **THEN** 能实际并行且分别不超过8/2，queue/cancel/cleanup保真；声明但未使用配置不通过

#### Scenario: 可选两native baseline侧
- **WHEN** plan首次冻结前提出native0或2侧baseline-only
- **THEN** 默认0；2仅在同一128MiB/1800s总盘子内且不损害主四侧覆盖的可行性证据成立才冻结，不强制6侧，native结果/eligibility与主比较分开

### Requirement: 首次请求前冻结全部manifest与可比条件
运行 SHALL 冻结所有side/node/stage/config/revision/source/engine/次序、真实target及子请求、timeout/TLS/SNI/redirect/HTTP2/UA/selection、8/2/0并发、全部quota、ledger原始start/deadline与exact child argv/hash。comparative客户端 MUST 统一target/timeout/TLS/redirect/HTTP2，native defaults差异 MUST 明示。execute MUST 验证manifest/pin/ledger绑定，不允许未记载的endpoint/并发/TLS变化、额外重试或自动扩额。

#### Scenario: 默认无网络与manifest变更
- **WHEN** plan/freeze/report或execute缺allow-network、manifest/pin不符、speed>0/测速URL、body>128MiB、deadline>1800s或review门禁未满足
- **THEN** 发请求前fail-closed，命令默认无外部网络，allow-network不能绕过门禁

#### Scenario: 归一化与原生差异
- **WHEN** comparative无法对齐某配置或native默认不同
- **THEN** 标不可比/方法差异并分账，不能静默把native Check改为normalized stage或凭headline计数判缺陷

### Requirement: 整批跨launch原子响应body与deadline硬限制
全部launch、全部side、成功/失败/redirect/内嵌client与非collector平台调用 SHALL 共享同一个aggregate ledger、原始start/deadline。整批body MUST ≤134217728 bytes、含cleanup MUST ≤1800s；不按每进程/每side重置，不额外512MiB。read前原子reservation、实际settle、未用refund和cancel MUST race-safe；崩溃未结算预留 MUST 保守占额不自动重试。NIC/header/TLS/transport MUST 单独标注，body cap不冒称NIC上限。

#### Scenario: 四侧最坏baseline与平台公平配额
- **WHEN** 冻结默认90×4×65536 baseline allocation
- **THEN** 最坏22.5MiB，platform剩105.5MiB四侧各26.375MiB；逐node×stage公平参与算法先冻结，不能前节点吞全盘/借贷/隐形扩额；可选native两侧额外最多11.25MiB在同一盘内扣除

#### Scenario: 并发耗尽/取消/跨launch恢复
- **WHEN** 并发reader、第二launch、deadline/cancel或崩溃恢复
- **THEN** atomic reserve/settle/refund不越body总额，不重置start/额度，取消在途/停止新任务，未执行与退款/保守预留准确记录

#### Scenario: 原生noncollector调用无法完整计量
- **WHEN** adapter只收collector结果而遗漏native平台内部HTTP调用body/error
- **THEN** 禁止实网并整改计量seam，不用结果bytes事后汇总宣称上限或通过，不承诺128MiB内全部native媒体完成

### Requirement: 逐节点真实终态与缺陷解释
系统 SHALL 保留每node×side×stage×attempt queued/not_scheduled/executed、原始verdict/typed reason/status、source/revision/config/target/profile/engine/commit、时间/body/预留/退款/复验关联且恰一终态。dependency_skipped、cancelled、budget_not_executed、restricted、unknown、fail、stale/config mismatch MUST 分账；根因不得由后deadline合成覆盖，日志/报告无raw token/password/private URL。未尝试平台/native media MUST 精确报告，双方失败 MUST NOT 解释为永久dead。

#### Scenario: discordant与budget未覆盖
- **WHEN** 四侧baseline不一致或平台因quota/dependency/deadline未覆盖
- **THEN** 保留逐节点解释、实际executed/skipped/cancelled和unattempted清单，只对可比已执行范围结论，partial不宣称full parity，不扩额/重复试探

#### Scenario: 发布判断与完整原始证据
- **WHEN** counts不同但阶段方法/配置/限制已解释或发现真实实现缺陷
- **THEN** 发布依据适用门禁与缺陷解释，不强制headline匹配；真实缺陷整改/自测/review后重验，不擦失败历史或假勾完成

### Requirement: 最终源码离线门禁与后续终态顺序
executor SHALL 在有界private caches与无外网环境完成Python golden/stage scheduler/ledger、native与same-engine各自build/test/Check/fixtures/pin diff、Go FULL build/test与FULL race、Web type-check/tests/NO_COPY build。full race若真实受资源/工具链限制 MUST 给确切命令/非0或超时/可复现日志与unchecked，不能用targeted PASS或文字冒充全量通过。旧Python经验 MUST 仅局部obsolete标识，不作当前Go部署指南。后续 SHALL 按最终独立Reviewer→批准staged batch→源码冻结刷新preview+Critic→正确COPY assets→授权CSP commit→一致性热备/rollback/app-only runbook rollout/health→独立7200s观察推进。

#### Scenario: 最新源码未测与资源限制
- **WHEN** 最后修改后build日志空/验证超时或full race限制
- **THEN** 保留未完成gate、稳定日志与复现限制，不声明PASS；private缓存在预检6.4GiB可用盘内有界留余量，不删除共享cache/联网盲补依赖

#### Scenario: preview与长期观察凭据
- **WHEN** 旧preview健康但不绑定最终源码，或rollout只有短健康检查
- **THEN** 刷新独立preview并获得最终review/critic；rollout完成和7200s观察完成分开，不将health poll/隔离加速当长观察

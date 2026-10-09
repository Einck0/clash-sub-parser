## Context

### 本轮授权与边界

意图包 `verbatim_current_commands` 为“按你的建议做”“现在看看下一步推荐我做什么”“把你的建议先来吧，其他的也继续”；包内明确解释为 CSP 必要 app 短暂替换及真实验收，不是全服务器更新。本规划子阶段只允许有界只读取证及现有 OpenSpec 规划工件更新。禁止源码/测试实现、构建、部署、业务接口调用、刷新/恢复/提交/QQ。本轮未派子机。

CSP 起始 Git clean、HEAD e0431767845ffe3cd2bff8a7e415abf085814ac7；Pi 和 bridge 不是 Git 工作树，不能宣称 clean 或借嵌套 Git 提交。CLI allowedEditRoots 本 Change 仅 `/home/service/clash-sub-parser`；跨根不写。无 graphify-out/graph.json。

### 2026-10-02 只读现场

- Compose labels：project clash-sub-parser，workdir `/home/service/clash-sub-parser`，docker-compose.yml，service app。
- 容器 d8780938a0d6ae9e21590e5cb8d81a02cf74ac035947b559d6660a723a88511b，running/healthy，StartedAt 2026-10-01T12:57:31.010279174Z；镜像 sha256:807549162cebecfcddee509629a69d322fd98eef8a9b60ae9a05b89de5a10542，tag latest 与 v1-release-20261001-6a008f7-dirty，无 VCS build revision。命令 /app/csp serve，user appuser。
- 内存读取容器二进制（docker cp tar stream，未落盘/未执行）：SHA256 be2091a70e57d8f493d46ea4d14d4ba3fce1b4c35f0b6530e3908e161aa1e600，46854304 bytes，Go1.27.1/trimpath，本地 csp 与其不相等（不是源码变化证明）。历史部署报告 `/home/service/backups/csp-release-20261001_205016/deployment-report.json` 记录当前 image/container，dirty diff d5700687b3f7c34d1b12dbab174e72e6ded1b73acc09690074b58bace4037951；对应 commit-manifest 31文件包含库存及探针保护。
- 运行二进制包含95ce0a9/e043176库存新 upsert SQL（SHA b552f091362edc4f8fa93c36d1bb14e6888a993a1568dc9975107496a4a6f888），不含6a008f7旧 prune SQL（SHA a119e10a1be90800507caa883185b358b00234b254eae673fa1e812e110fe7c4）。此为代码特征证据而非完整 byte-for-byte 源码归属证明；旧 orphan SQL 仍可为其他调用存在，不能凭单个字符串推行为。95ce0a9..HEAD 的 app相关目录差异仅 internal/recovery（调用者仅恢复CLI，非cmd/csp）；库存/探针服务、web、migrations无新增差异。因此不推断保护未上线，重建用途是绑定经审查的当前树与镜像，后续先核验构建 manifest。
- 保持 named volume csp-v1-data→/data；127.0.0.1:17000/18080→18080；现有 CSP_FETCH_PROXY 与 host-gateway；DB `/var/lib/docker/volumes/csp-v1-data/_data/csp-v1.db`。只读 SQL：schema13，1005节点（22 active/983 inactive），22 source，9订阅（4 enabled/5 disabled）。这不是恢复分类证明。
- 当前 fetch timeout30s、enabled订阅timeout30s、probe TTL300s、最大并发16（只读配置，不修改）。公开 healthz/readyz/auth-status 200，ready=true/tables14/schema13，admin/export均protected，authenticated=false。
- 已读取 `.env`（0600，仅CLASH_REQUEST_USER_AGENT）、容器及本进程 CSP_ADMIN_TOKEN 均空；已知受管 env.bak（0600）也无token。settings现存bcrypt verifier且两个auth开关启用，不输出hash。没有已确认生产明文凭据来源；历史隔离预览凭据不能用于生产。不要扩大到主目录盲搜；仅持有人安全投递现有匹配token到0600受管文件或受控进程env。启动代码以DB verifier为真，不允许注入任意新token覆盖。
- 历史DB备份及旧0ea88镜像归档仍存在，但本轮没做完整性/恢复测试，不作为fresh可回滚证明，旧镜像不是当前回滚目标。

## Goals / Non-Goals

交付可溯源受控app更新、库存/身份保持、真实鉴权正文与代表性实网证据、可证误下线恢复。不是要求全节点在线；不造生产故障订阅、不新增陌生目标、不动UI/gateway/Hermes内核/模型/Gemini，不假全绿。

## Decisions

### 1. 冻结工作包及原生运行关联

唯一完整特性包 `pkg_csp_release_and_acceptance`，全部实现、自测同executor完成；审核/生产动作有依赖，不为并行拆微任务。change=`finish-csp-live-validation-and-recovery`；changeRoot=`/home/service/clash-sub-parser/openspec/changes/finish-csp-live-validation-and-recovery`，schema=spec-driven。

真实当前run `pi_run_20261002_194438_3727280`，metadata=`/home/service/hermes/workspace/tmp/pi_runs/pi_run_20261002_194438_3727280.json`。官方status本轮 RUNNING/IN_PROGRESS，不能说EXITED或PASS。parent session为环境指向的 `2026-10-02T11-44-41-696Z_sess_20261002_194438_3727280.jsonl`；本规划子会话不是executor/reviewer，不提供伪造 gate PASS。

metadata已冻结version1合同：change_root字段是项目工作区 `/home/service/clash-sub-parser`（区分CLI changeRoot），work_packages仅本包，epoch1；executor_self_test=true、reviewer=true、critic.applicable=false，非空reason为无新增UI的CLI/发布治理。不得覆盖这个不可变合同，不注册历史合同。

- 原生CLI `/usr/local/hermes-util/pi-dispatch`，help确认 dispatch / register-contract / status。新运行使用 `dispatch @<approved-task> --workdir /home/service/clash-sub-parser --change finish-csp-live-validation-and-recovery --change-root /home/service/clash-sub-parser --work-packages pkg_csp_release_and_acceptance --critic-reason '既有app发布与CLI实网验收，无新增UI'`（本轮不执行、不改model）；采用其新回执ID。已有当前合同无需register；仅active且缺合同时才register-contract，已存在冲突拒绝，历史/finished拒绝。
- 每次派工显式 Task Execution Contract：真实run_id/change/work_package/epoch/reverification_of=null或真实failed report_id。报告用json:pi-governance-report或官方HTML标记，含report_id/run_id/change/work_package/epoch/role/gate/verdict/final；executor列实际build/test命令exit_code0；reviewer必须真实独立子会话，失败后复验final:true及reverification_of。生产证据纳入最终独立复验，不能前置代码PASS抹除未做业务。
- 官方终态 `pi-dispatch status <actual-run> --json`，全部包门禁及真实业务完成后检查退出状态、报告、实际命令与独立session，无活跃失败。旧110336 run仍EXITED/UNVERIFIED/MISSING_COMPLETION_CONTRACT；独立112126 run仍EXITED/PASS、无children，原样保持，不追溯洗绿。
- contract_lineage=`fleet-contract-v3 / csp-release-and-remaining`。可读取原始意图包 `/home/service/hermes/workspace/tmp/intent_package_csp_release_and_remaining_20261002.md`（8617 bytes）；其原始字节SHA256，即contract_revision_hash=`cbca3c542a87438759c2c9d8fab80f205558a2ce066dd22ea06fa5aeebb47baa`。不是用户原话单独SHA，不改原始包。patch_hash实施后对明确文件集合实算，本轮业务patch尚不存在不编造。

### 2. 根因级整改及完整施工规格

写集合仅CSP：`cmd/csp-live-acceptance/main.go`、该目录标准*_test.go、`scripts/run_authenticated_acceptance.sh`及必要脚本fixture、`test/e2e/live_acceptance_and_recovery_fixture_test.go`、`docs/production_live_acceptance_and_recovery_plan.md`、`docs/controlled_deployment_runbook.md`。不改服务业务接口来迎合错CLI；沿用成熟标准API和既有领域类型/脱敏函数。

- 将main拆为可测run入口返回exit，统一汇聚必选断言：HTTP/JSON/schema/业务失败、超时、缺结果、库存变化、报告保存失败均不允许PASS。missing credential明确PARTIAL/BLOCKED；零退出安全暂停不能作为executor业务完成。日志/报告只脱敏白名单摘要，去原始URL/query/userinfo、token、节点配置/凭据、原始上游err；文件0600、目录0700，write errors必须返回。
- 节点Active实际为领域布尔（以NodeView为准），不能int解析。分页按现有pagination协议及100上限正确迭代；总数/页数/重复ID检查，全库概况与页内数分开。选取至少存在的active节点，不让首页无active成为成功零探针。
- `/nodes` handler不支持subscription_id筛选，现getActiveNodesForSub不能当订阅库存。使用既有节点detail Sources在有界全量清单中还原归属，或由运维读取SQL关联生成只读安全manifest（不新增后门）。集合为logical_id、active、connection_revision及来源关系的稳定摘要，失败前后集合比较；无法取到任何快照是FAIL不是0。失败响应500为顶层error envelope，不靠data.outcome空值判成功；client timeout/传输断开后要确认服务器结束与库存现态，无法确认标未知/失败，不自动重试刷新。
- POST探针准确字段 `node_logical_ids`、`kinds:["baseline"]`、显式deadline，仅选审查列表；不能node_ids空字段导致潜在扩大扫描。GET run终态是succeeded/failed/cancelled/expired，观察在 `/probes/runs/{id}/observations` paginated envelope，不是run内嵌observations。用domain.ProbeObservation（node_logical_id、kind/verdict、evidence_digest、observed_at、connection_revision、latency_ms、redacted_summary），校验每个预选节点的新观察与run/当前revision绑定。阶段从实际summary记录，不虚构字段；握手失败如实分层，缺观察不绿。等待是针对本业务job、deadline控制的有限事件进展读取，不无目标机械轮询；预算到期最多取消本工具创建的run，不取消其他任务，不改全局schedule。
- runner用传入受管scratch（核验owner/非公共可写）或mktemp -d、umask077，唯一bin、trap清理；不exec丢失EXIT cleanup。失败不执行旧binary。保留报告但删构建临时物，禁止固定公共/tmp路径。

### 3. 自测、独立审查与隔离预算

成熟testing/httptest/t.TempDir/标准断言及既有e2e SQLite，不手造私有框架。完整测试矩阵：合法/错token/无凭据；公开基线失败；200错误JSON/schema；单条断言失败；多页与首100无active；刷新500/超时/误清空及等计数替换ID；未知状态/错误node_ids反例；真实协议succeeded和独立observations、错run/revision、空观察、deadline/cancel；redaction金丝雀；写文件失败；公共tmp预置symlink不触及、并发scratch不冲突、build失败与清理。

本地只loopback httptest/临时SQLite/fake现有probe依赖；测试不能用生产DB、真实订阅/节点/凭据、真实模型或QQ。CLI subprocess fixture清空token/代理/外部通知env，禁止生产出口。命令：`go build ./...`、`go test ./...`、`bash -n scripts/run_authenticated_acceptance.sh`、针对CLI包及现有e2e test可复验命令、OpenSpec strict，全exit0。建议本地完整包预算40分钟，耗时有新证据继续、无进展caller_ping保留成果，不机械重试。无UI源码变化无需新前端门禁/critic；Docker既有web构建仍应成功。reviewer20分钟建议预算，独立重跑Go/build/shell与故障fixture，审核发布/备份恢复步骤及真实合同，不介入施工中间步。

### 4. 真实验收预算与标准

本次建议（操作上限，不是协议常量）：至多3个enabled既有订阅、每个一次，按现有来源/协议分布选而非随意前3；每次HTTP上限45秒（已知服务fetch30秒），不得修改订阅配置来扩大timeout；代表性至多6个active既有节点，按来源/协议覆盖、明确baseline，分至多2节点的小批串行，本轮总deadline10分钟。probe现有TTL300秒，deadline与剩余预算一致，不将TTL强改成30秒。公开/正文GET超时10秒，probe创建10秒、确认预算从剩余deadline扣减。安全事件中止不重试生产刷新。

合法auth正文：status authenticated=true、settings开关/verifier配置保持、subscriptions分页/9类计数、nodes全量/active/source、policies/pubs真实shape与计数；已有publication才核验导出解析，无publication不凭空创建，标不适用。API数据变化用fresh实际基线，不硬编码1005或22为未来结果。记录脱敏请求类别/状态码/body schema摘要、数量、ID HMAC/受管摘要、时间/run/observation digest/阶段/延迟，保存0700/0600，不上传原始配置或订阅URL。

成功刷新允许新增合法库存，不应删除/失活既有有效节点；失败刷新完整旧节点/来源/active集合保持。任何丢库、安全限制绕过、身份开关/DB verifier变化立即回滚。外部订阅不可达/节点握手失败如实报告，不等同部署需回滚，但真实成功握手不足不可说“握手成功”；若全失败报告PARTIAL及具体外部层原因。缺合法凭据不发受限请求、保留任务未完成；继续本地闭环不借暂停exit0洗成PASS。

### 5. fresh热备与恢复验证（后续门禁通过才执行）

1. 紧邻替换前再读Git/容器/DB/权限与变更计划；保存当前精确image ID，不以旧0ea88归档回滚。创建私有fresh备份目录（实际后续时间/run确定，0700/umask077），DB在线SQLite backup API/.backup（禁止直接cp正在写的DB），busy_timeout并确认成功。SQL只在同一snapshot统计库存/source/订阅/策略/配置revision/schema及auth verifier的受管对比摘要，不输出hash/secret。备份整个DB含身份和库存；复制Compose/.env及受管身份来源配置0600，docker inspect只保存脱敏选项，secret副本单独受控。
2. 在备份副本用immutable只读运行integrity_check与foreign_key_check；用第二个隔离可写副本恢复演练、逐项数/稳定身份指纹匹配，禁网络/后台refresh，不启动指向生产源的服务。确认owner UID/GID（运行image appuser为10001，执行前再核验）、权限与WAL模式处理。记录SHA/字节/检查exit与恢复演练证据，成功才可替换。
3. 将当前image ID固定为唯一 `clash-sub-parser-app:rollback-<actual-run>`，docker save导出当前镜像到私有备份并SHA/tar可读校验；docker image inspect标签必须指向相同ID。无需加载旧镜像覆盖当前。

### 6. 构建/发布顺序及精确回滚

独立reviewer PASS→构建已冻结源码树及manifest（Git HEAD、定向diff SHA、构建输入/内嵌资产摘要、image ID/可选OCI revision，不以HEAD掩盖dirty）→验证新镜像基础命令/非root/DB兼容与隔离临时DB行为→fresh备份及镜像pin→只替换app。

后续命令以CSP workdir运行：`docker compose -f docker-compose.yml build app`；核对实际Compose service image（当前默认clash-sub-parser-app）及新ID；`docker compose -f docker-compose.yml up -d --no-deps --no-build --force-recreate app`。不down、不删卷、不pull或重启其他服务；Compose变量/端口/volume/代理逐项同基线。若已证明业务二进制无差异，重建只作为用户授权的可追溯发布，不假称新增保护。

更新后建议120秒就绪预算（来源本次部署控制建议，Compose health30秒/start10秒而非接口常量），公共health/ready与双loopback、鉴权模式、容器稳定无重启、真实已知入口（执行前从现有配置确定，不猜域名）、库存/身份基线一致；再执行合法真实验收。无凭据仍允许本地/发布可验证部分但整体PARTIAL，不要求先造凭据。

回滚触发：超120秒未ready/持续crash、双端口/入口失效、DB schema不兼容、库存删除/错误失活、身份/安全配置变化或泄密。普通上游网络失败本身不回滚，但停止进一步业务写并记录。

- 镜像回滚：仅stop app；`docker tag <captured-current-image-id> clash-sub-parser-app:latest`（执行前确认Compose确用此image）；`docker compose -f docker-compose.yml up -d --no-deps --no-build --force-recreate app`。检查实际image ID为captured值，健康/ready/双端口/身份恢复。禁止旧手册误tag clash-sub-parser:latest而app用另名。
- DB回滚仅在有损DB写/迁移且需要时：先stop app，确认目标无其他写者；封存失败DB以及-WAL/-SHM以留证，绝不让旧WAL应用到恢复快照；将已验证backup恢复至原卷内临时文件，chown/chmod与原值匹配，原子替换csp-v1.db，隔离旧sidecar后以captured镜像no-build启动。正常镜像回滚不覆盖仍完好的DB。记录恢复前后snapshot/身份/完整性，避免丢失不相关正常业务写；若出现未预期并发写需停止并由责任人决定精确恢复，不私自覆盖。

### 7. 恢复及跨根报告遗漏

恢复白名单历史为0，本轮不重新认定全部失活原因；源、唯一身份、审计、用户禁用/删除、时间线充分才准入，fresh只读候选manifest→独立reviewer→精确precondition/transaction/backup→实际计数。缺证据的node维持现态并分层待决，0恢复不宣称全部正确或永久关闭需求。

报告角色识别/合同注册/测试出口已有实现及独立112126 PASS，本轮未运行桥接测试，不重复宣称新验证。bridge两个Change仍有未勾选的官方终态/交付与独立审查项：fix-pi-governance-report-role-attribution 2.2/3.1、finish-dispatch-isolation-and-attribution 2.1；Pi finish-pi-test-isolation有真实reviewer记录且勾选。它们属于各自根，不把CSP包PASS当别包PASS，不在本Change跨根勾选。后续母体对既有独立证据另行核对/精确收口，当前CSP合同无需扩改。

## Risks / Trade-offs

凭据外部持有人尚未安全投递；不能从bcrypt恢复明文。镜像缺revision使完整来源一致性需后续构建manifest，而非仅比时间。真实节点可能失败、历史归因可能缺失，必须保留限定结果。备份恢复涉及WAL与并发写，须先演练再放行。CLI当前协议错配可能扩大探测，整改前禁用于production live。

## Migration Plan

本轮更新规划并strict至READY，所有后续业务未执行保持unchecked。后续一个完整executor包自测→独立reviewer→已授权受控发布与证据收集→合法有界真实验收/充分证据恢复→独立终态复验→母体核验官方run与必要授权提交/通知。凭据阻断仅暂停受限目标；本地整改继续，绝不伪造业务完成。

# DNS 数据恢复与策略功能去重（2026-10-21）

身份 pi_run_20261021_160307_1574174 / 002-policy / default / epoch1。沿当前 Feature 增量实施，不新建分支或重生成既有规划。入口HEAD79ff03a42b546c54f08767a6842cb816a78dd731，初始dirty清单/tmp/csp-1574174-initial-status.txt；调查机报告中HEAD79ff03a后缀不一致不能作Git证据，以主脑实际rev-parse为准。

## 授权与结果

用户澄清“恢复数据，去除标题，界面就用这种”，并要求直接处理策略组规则/准入规则冗余冲突。确认恢复DNS数据、删除页内重复标题横幅、保留现有编辑器；不回退整页、不整库回滚。生产DNS配置写入已明确授权；程序生产部署没有在本轮明确授权，开发提交推dev为既定日常授权，云测与预构建镜像隔离验收后交付，不沿用上一轮上线许可。必要时仅请示新程序上线，不把请示当施工替代。禁止本地宿主及本地Docker任何Go/Node测试编译，复用现Actions。

## 真实问题与证据

- 当前DNS source=default/revision0，表无配置行。历史宿主/data/clash_sub_parser.db仅一行有效自定义DNS，enabled=true、821字符、17顶级键，raw SHA57fcbd1056e78ba6db554cd5c37b459cf569c4477913c884a2045b92245d004d；匿名旧残留空配置不是恢复源。纯validate200，规范化SHA8c994f438f9c7f8c84dd530bfb01a4b74043bac89344c8760353d3b3d047f9bb，17键语义保持。详/tmp/csp-1574174/dns/evidence.md。调查机扩大到一个已识别匿名卷属取证边界偏差，后续禁止扩搜；不以“唯一全系统”夸大为全盘已证明，只称已确认同源历史记录唯一有效。
- PolicyView分流/准入页签重复RulesView；分流页点新建实际CreateAdmissionRule，DIRECT/REJECT目标不能正确显示。RulesView才有分类、排序、内置动作和MATCH处理，现470条/14分类。生产admission_rules为0、group_sources92。
- resolver admission伪DSL不支持提示的in/&&，country从名称猜；在全局软筛选之前硬排除，绕过组inherit_global_filter=false的预期软豁免。真正inactive/显式riskexcluded和风险决策是另一条硬约束，不可删除。题意不是要求删除流量分流规则或安全校验。

## FR-C1：DNS恢复和去标题

恢复流程：先读在线当前DNS并保存单配置JSON、在线SQLitebackup基线(0600/10001属主)，核实旧源sha与enabled和17键；原生validate无保存；用现PUT settings/dns expected_revision=current仅写DNS；发生409不覆盖重试，重读确认冲突再主脑决定。回读source=saved、revision+1、enabled相同，规范化mapping与候选逐键deep-equal；实际/clash的dns完整map等价。规则/订阅/组等静态数据指纹不变，动态probe结果不以全库hash硬比。留可恢复的单DNS基线，回滚若需要走CAS恢复原配置，不直接还原整库或强删WAL；不能恢复时冒默认成功。

界面删除DnsView页内header的DNS小标/大标题/重复scope段及default-source迁移提示横幅；应用导航仍有DNS名称，section可用aria-label，不删除错误/成功/409反馈。可视/Raw/editor/outerenabled保持。范围说明不是新banner，可放紧凑的现有help(确有必要再用)，不为布局改造扩范围。

## FR-C2：用户操作唯一入口

策略树仅组拓扑与成员编排、全局软筛选/校验。删除分流/准入tabs、列表、新建准入modal和无用state/composable请求/模拟工具；不保留另一个重复CRUD。流量规则全部在现/#/rules，允许一个简洁导航链接而不是第二套数据列表。保持组创建、编辑、顺序、删除确认、全局filter CAS、validate和单草稿预览不变；移除重复后reload不再取admission/routing两套列表。

## FR-C3：准入伪DSL退役，保留硬约束

层次明确：订阅入库过滤决定资产来源，节点enabled及显式风险硬约束决定可用候选，全局filter为可按组豁免的软约束，sources动态有序add/remove决定组成员，route规则决定流量去向。删除额外用户admission表达式对候选的影响，不引入第二过滤DSL或自动迁移country猜测规则。

resolver.evaluateNodeAdmission保留签名作历史调用兼容，但仅执行inactive与excludedNodeIDs硬约束，完全移除matchesAdmissionExpression及字段猜测解析。历史AdmissionRules输入不再参与实时筛选，若非空输出一条明确legacy_admission_rules_ignored警告(不逐节点重复，不含表达式原文)，便于旧库知晓规则已退役。旧持久数据不删除、不伪迁移到真实地区条件，不改历史不可变发布artifact；production0规则因此不改变当前有效候选。computeInputDigest中历史字段是否影响hash可保留兼容，不为了本任务改摘要格式；内容输出与预览一致才是验收核心。

控制面CreateAdmissionRule从service入口稳定返回ValidationError admission_rules_retired(422)，在任何修订/审计外部副作用之前拒写。所有POST别名及kind=admission/旧action推断都调用同一拒绝路径，不悄悄当policy创建。历史GET仍可只读返回旧记录，明确read-only(通过响应提示若不破坏兼容)，不要求用户管理；admission DELETE同422，保留历史。generic DeleteRule无kind只表示policy，移除policy-notfound→admission隐式回退；显式kind=policy与RulesView原CRUD不变。/admission/rules/{id}必须明确绑定admission拒绝而不是generic误删同ID路由规则。

调查后补充决定：独立RulesView也调用同一CreatePolicyRule，已有MATCH时追加普通分类规则确实被原验证顺序误拒。仅在已有分类模式对候选验证序列将MATCH置末，再走原验证；不改变无分类显式position语义、目标校验或MATCH唯一约束。服务/HTTP回归及真实UI创建471条再删除恢复470条已通过，不借删除重复入口掩盖核心写入失败。

## 场景与通过条件

1. 已有香港组编辑→健康/地区自动预览→一次保存；拓扑无准入/分流重复页签；Rules页仍470/14、DIRECT/REJECT/组、排序、MATCH末位；新建流量规则须成功且MATCH仍last(调查追加问题需专项核实)。
2. 设计夹具：active vless节点，旧admission reject protocol vless、globalfilter不匹配、WARP不继承→WARP仍可引用节点；同节点inactive或riskexcluded则仍不可用，普通组仍受globalfilter。country伪字符串不再影响真实geo/platform，历史GET不丢记录；POST/DELETE admission422且revision/table无变化，generic无kind delete不碰同IDadmission。
3. DNS旧17键恢复→currentGET/实际/clash完整相同；UI页内无大标题横幅，Raw/可视/错误冲突原行为可用。生产仅配置恢复不等于代码上线。

## 模块契约/顺序

A 前端：web/src/features/policy/**中的PolicyView,usePolicy及其tests；web/src/features/dns/DnsView与tests。不能改PolicyEditorSheet/StructuredFilterEditor业务无关布局、locale/router/App全局文件；若必要报主脑。读取RulesView/useRules仅主脑给定调用事实无需跨改。
B resolver：internal/resolver/admission.go及相关admission/resolver测试；不改risk_admission/globalfilter/graph/domainmodels。只读domain policy/node与resolver邻接契约。
C 控制面：internal/application/policy/service.go及相应tests，internal/transport/http/policies.go及相应tests；专属负责API别名和拒写契约。不改resolver/UI/repo底层表/migrations。
D 生产DNS恢复：仅专属/tmp证据与在线backup、现DNS API一次CAS，不改源码或生产程序。D读/写集合不依赖A/B/C，可并行；程序改动全部汇聚主脑逐文件开发提交/push，任何子机不提交。

A/B/C独立无共享生成物可并行。主脑冻结接口与范围，module外未知caller_ping。先只源码与测试代码编辑，不运行本地tests。主脑diff核对→开发提交/push dev→同SHA Actions全部成功和云image→只用合成DB隔离预览→独立review、真实桌面/移动browsercritic(虽冻结合同critic默认false，本UI任务追加实际验收，不伪造合同修改)。云失败保留日志针对修复新开发提交，不本地跑补测。graphify不可用如实记录不安装。

## 最终验收

实施、云失败复验链、提示文案补漏、最新SHA及证据与清理见 [cleanup-completion-1574174.md](cleanup-completion-1574174.md)。程序生产部署未执行；DNS数据恢复已在生产生效。

# 2026-10-20 策略筛选、探针语义、DNS 与待复核治理详细契约

Run `pi_run_20261020_111741_708234` / change `002-policy` / work_package `default` / epoch 1。基线 `2969d537215e8eeabe5d9e0dc978093e0127b0e0`。

## 1. 用户要求、授权与优先级

> 让csp重新做策略组筛选，特别是探针筛选相关逻辑，区域等于可用性是什么鬼好好想想要怎么做，，主脑方案设计必须详细，指明业务逻辑和实现方案，然后让主脑恢复dns设置页面，现在的dns怎么没有了，
>
> 另外现在又有一堆待复核是什么鬼，看看到底是什么情况

实施、恢复、隔离验证、独立审查、逐文件本地提交获授权；**不包含本轮生产部署、真实探针/源刷新、批量改生产状态或自动导入历史 DNS**。不干扰 ops-tasks 清盘，保护初始 dirty。沿用当前 Feature，相关 Probe/UI/Publication 的旧规范出现冲突时以本文本次条款为准，不重建项目/constitution。前轮报告只是历史，不是本轮通过凭据。

## 2. 已核实根因与证据边界

### 2.1 筛选与探针
- 当前高级过滤只有名称/协议/来源订阅/probe_verdict/probe_latency_ms。探针种类 geo 与 verdict available 组合含义是“地域查询成功”，**不是出口处于某区域**，UI 却把所有种类用同一判定组合呈现。
- 真正出口国家/地区已在 geo 观测 `evidence_data.region` 持久化，值如 HK/JP/SG/US；成功 geo 在 runner.go 中从独立出口查询提取国家代码，不能从名称猜测。平台观测另有 platforms map，Region 可能是平台解锁区域，不能混作出口国家。
- node_filter.go 在 probe 条件中未填期限默认86400秒，SQL节点健康筛选 node_predicates.go 还有1小时判断，pool/status service.go 又用 DefaultProbeFreshnessTTL，而 Inventory read model 以0期限评估。并非一份健康事实。
- EvaluateBaselineHealth 仅 `error reason=node_connect_failed` 被归不可用，当前 runner 经常写 transport_error / timeout，于是归 unknown。前端 undetermined/baseline_missing 显示待复核，并有旧“未知·配置待核”等分支。
- 本轮只读运行态快照69可见节点：healthy26、unknown43；unknown中28无有效baseline摘要、11 transport_error、4 timeout。数据随自动巡检变化，不能拿这个快照当固定总数或将所有unknown等同失败。原始聚合 `/tmp/csp-708234/live-health.json`。
- 后续SQL反例确认同样error有匹配/跨连接版本之分（启用来源引用集合：当前版本transport_error4/timeout4，跨版本transport_error7/timeout1，另53成功；非同一时刻API集合）。所以摘要中的15失败不能整体重判当前不可用：先版本匹配再分类，跨版本仍未测试；报告数字不可混用。API未暴露revision不能据null显示断言实际DB为NULL。
- 当前有序source持久化无需废除，但普通用户编辑被迫操作两层草稿/流水线，来源要求UUID，继承重复控件；计数算术闭合但“排除43”下说无排除节点。

### 2.2 DNS
- Go rewrite后新web无DNS页/导航，HTTP settings无DNS接口；settings表无DNS列或独立表；Publication给resolver硬编码 nameservers，而 Mihomo配置struct没有DNS字段，因此只加菜单不能恢复功能。
- 历史 `16fe0e7:frontend/src/views/DnsSettings.vue` 和 backend dns_config 定义可查。旧库 `/data/clash_sub_parser.db` 中单例 DNS enabled=true/raw_yaml 821字符仍存在，含服务器、fake-ip、policy、fallback等字段；本轮仅查shape/长度，未输出含secret值/未写入。
- 当前真实生产DB `/data/csp-v1.db` 没有DNS持久配置。恢复页面不能假称已找回并启用了旧库配置；旧归档仅显式用户导入时才读取，程序不在启动时偷偷迁移旧库。

## 3. FR-S1：探针事实与节点三态

### 3.1 对用户只展示三种主态
- 可用：当前 connection_revision 的最新 baseline 结果 verdict available；历史 restricted baseline若确实连通，映射可用但保留受限细节，不让受限成为第四主态。
- 不可用：当前版本最新 baseline 已完成尝试的 error（包括transport_error/timeout/DNS解析/握手失败）、restricted以外明确失败。**不可用表示此次检测没有成功，不断言物理线路已坏。** 原因详细区分网络失败/安全拒绝/配置缺失/构建失败，不能隐藏失败或伪造成待复核；安全策略不放开。
- 未测试：没有当前版本有效 baseline、连接版本变更、旧NULL revision/future/stale历史无法证明当前连接；可显示“当前连接未测试”“只有辅助能力记录”原因，但不再发明待复核/待核验/过期主态。旧历史原样保留，不UPDATE数据库抹观测。
- queued/running只为操作徽章，**不覆盖健康主态**。取消未执行不写假成功/假失败；正在刷新时显示最近一次真实结果，完成新失败必须替换旧成功（不存在“取最近成功”掩盖失败）。

### 3.2 单一评估语义与调度
- domain/probe_health.go 定义统一分类，read model、pool status与SQL分页predicate严格等价，固定 clock/同revision生成守恒测试。内部可保留旧响应字段用于读取兼容，但 canonical状态只healthy/unhealthy/untested，旧degraded/undetermined计数为0或规范化映射，不做平行分类器。
- 去除 **用于健康/筛选的自然时间TTL**：不因1小时/24小时自动降unknown。观测时间只显示上次检测和供统一协调器判断到期；周期沿已实现global/per-source policy自动更新；失败是最新失败而不是待复核。连接版本隔离仍必须保留。
- 用户在已有来源设置disabled/关闭全局自动时不越权重启检测；没有baseline时仍未测试，必要手动检测明确IDs，不能静默全库探测。
- 遗留 freshness_seconds 接收后必须明确废弃并在下一次保存规范化移除；**不再影响结果**。旧持久source读取无损，包括未知属性，不能启动时批量改生产数据。旧stale观测属于历史不可认证记录，非自然过期判断；不新生成stale verdict，不把stale列为可筛选业务状态。

## 4. FR-S2：真实业务筛选维度和契约

沿现有 NodeFilterSpec.conditions 原生增加必要字段，不新增第二套DSL、表或编辑持久化配置。普通编辑器只提供以下有明确含义的字段：

| 字段 | 条件/值 | 证据与缺失语义 |
|---|---|---|
| 显示名称 display_name | contains/not_contains/regex/not_regex | 当前名称，保留大小写不敏感PCRE及有界超时，错误不可静默改写 |
| 协议 protocol | equals/not_equals | 原协议枚举 |
| 来源订阅 source_subscription_ids | contains/not_contains | 名称多选/单选逐条件，值仍是稳定订阅ID；所有来源取事实，不能依赖当前页节点猜来源 |
| 节点可用性 health_status（新增） | equals/not_equals：healthy/unhealthy/untested | 统一baseline三态；最新失败=unhealthy，未测不能匹配healthy，也不得被“排除不可用”偷渡成可用 |
| 基础延迟 probe_latency_ms | lte，非负整数ms，固定baseline | 必须当前baseline可用且真实延迟>0，缺失不当0，不能选geo当线路延迟 |
| 出口地区 exit_region（新增） | equals/not_equals：ISO3166 alpha2如HK/JP/US | **仅最新当前revision geo成功且Region非空**；从region标准化大写，候选可按常用地区及代码搜索；unknown/失败两种比较都不匹配，不从名称/AI地域回填 |
| 服务可用性 platform_verdict（新增） | equals/not_equals，platform明确具体服务，available/restricted/unknown/error | 当前kind ai/streaming的最新platform观测。平台来自实际支持集合，不允许用户把geo当服务；缺记录不等于unknown verdict，两个比较都不匹配。服务判定不改变baseline健康 |

- 旧 probe_verdict 的kind+verdict依旧可在高级来源中原样显示/编辑，说明其是“某检测是否成功”，不能变成出口地区业务选择。旧geo.available不自动翻译成某国家，不批量改原用户意图。普通入口不再提供“区域=available”组合。
- 新维度 validation在domain统一；前端只做值类型/基础范围快速提示，PCRE交服务端；非法码/平台/运算符422且带可定位条件索引。
- ConsistentFacts的依赖检查包含新增health/exit/platform/baseline_latency；最新对应kind按当前revision读取。preview/validate/live export共享同事实与resolver，不仅前端过滤。缺数据库观测不静默当全部通过。
- 真正过期/时效概念从普通界面、详情当前结果与筛选撤下；历史记录保留原文，不重写失败历史。

## 5. FR-S3：策略组编辑的实际动线

### 5.1 默认普通编辑
- 卡片打开已有组时先显示 **组名/类型紧凑行、当前动态选入条件、明确添加成员/排除项、实时成员结果**；已有filter直接展开当前值，不先占满手选节点。排序/空组行为/客户端能力说明为折叠高级设置；继承全局软过滤只有一套控件，说明WARP豁免不越过硬准入。
- 普通动态筛选的一组conditions是AND；来源选择器显示真实订阅名称而非UUID输入。地区名/ISO值明确分离；“节点可用性”“出口地区”“服务可用性”分别为独立字段和控件。
- **一份草稿、一次保存**：条件修改、增加、删除直接更新父草稿，250ms防抖对完整当前草稿预览；不再要求“添加条件→添加来源→保存”。半填条件显示具体错误/未完成且阻止保存，不用旧结果冒充新预览。预览代次保护与卸载清理保留。
- 前端仅投影已有sources，不保存preview ID列表为手动节点。简单source可直接映射编辑；没有条件的全匹配source仍保持动态。手选节点是用户主动选择的补充，不默认喧宾夺主。

### 5.2 复杂存量与高级编排
- 对有序add/remove/重入/子组引用/展开/builtin，默认显示可理解的逐条中文摘要与当前条件，提供明确“高级有序编排”展开入口；**不强制把所有排除搬到末尾**，不自动转换复杂组为普通模式。编辑任一filter原位直接修改、同一保存按钮，不产生第二份来源草稿。
- 已有sources不丢未知属性/ID/位置；移动/增删仅修改用户选中的项，不清空其他动态条件。子组引用 vs 展开节点语义明确，合法builtin和MATCH置底保持。

### 5.3 结果解释
- 候选池、匹配节点、未选入节点分别命名，不能将所有未匹配者笼统说成显式排除。candidate = kept + not_selected使用去重node集合，members含builtin/子组另列。
- 每一候选给一个最终归属：入选/不满足选入条件/被全局筛选/显式排除（后续重入则归入选）；历史流水线事件仅在高级诊断中显示，不能用重复事件总量冒充节点数。
- 原接口candidate_count/node_count/excluded_count可保留兼容，UI说明excluded是净未选入，摘要缺失显示“尚无详细原因”而不谎称“无排除”。最少需按明确集合/阶段给汇总，不要求为未知原因猜结论。
- 编辑错误、加载订阅失败/候选失败、预览失败分别可见；不catch后静默保留旧数据当成功。

## 6. FR-S4：恢复DNS页面、存储与真实导出

### 6.1 产品边界
- 顶级 `/#/dns` 页面 + 桌面/移动导航及历史tab识别；Settings提供跳转不复制第二份DNS表单。用途是 **导出客户端(Mihomo/Clash) DNS配置**，不是给服务器改系统DNS，也不改变探针引擎/宿主解析或WireGuard节点自身dns参数。
- 常用可视化：启用、ipv6、enhanced-mode、nameserver/default-nameserver/fallback、proxy-server-nameserver/direct-nameserver、fake-ip范围与过滤、nameserver-policy/fallback-filter；其余高级键用Raw YAML完整编辑。可分组/折叠，不机械搬旧CSS。
- 保存前统一校验，保存后读回；Raw与可视化双向同步且 **未知键无损**；解析失败不能以空对象替换、不能保存覆盖原值。重新加载有未保存内容应明确提示；一次保存，错误可见。

### 6.2 最小原生持久化与API
- 当前无可复用DNS持久表，新增下一可用migration（现场核对为26，施工若已存在应报而不是覆盖）：单例 dns_config(id=1,enabled,raw_yaml,revision,updated_at)。单一事实，raw_yaml最多64KiB。
- domain提供DNS配置值对象与验证/解析（复用gopkg.in/yaml.v3）；只接受单个mapping或完整dns: wrapper，拒绝多文档/重复键/递归alias过度展开及非mapping；限制输入大小/深度与常用字段类型，保持未知合法字段，不任意替换自定义值。不得主动DNS解析或出站验证。
- 默认种子遵守当前resolver已硬编码的启用与nameserver1.1.1.1/8.8.8.8，**不擅自套国内模板或自动加载旧库**。GET返回配置来源default/saved，页面清楚说明首次恢复尚未导入历史Raw。若用户已将来导入原配置，所有未知键/值保存保持。
- GET/PUT `/api/v1/settings/dns`复用现有Settings鉴权；PUT完整enabled/raw_yaml + expected_revision，原子乐观锁，冲突409并保留草稿，非法422不写；GET从不含其他Settings Token。附revision便于双客户端不互盖。
- 若未来需恢复历史原配置，用户在页面显式粘贴原Raw并保存；本轮隔离测试以历史shape的合成值验证无损，不在生产导入/重置DNS。交付须明确旧配置尚在归档，不假装自动恢复全部历史值。

### 6.3 一致导出
- ConsistentFacts在同一read transaction加载dns_config，Publication所有实际snapshot组装不再硬编码覆盖；resolver.DNSConfig升级为可携完整DNS map（可兼容既有Enabled/Nameservers/Fallback测试构造，但不得两套事实同时生效），snapshot digest纳入所有有效DNS字段。
- Mihomo compiler.mihomoConfig加入dns对象，启用时保留合法未知键并生成effective enable，禁用时输出enable:false而不伪造enable:true；原规则/节点/组不变。
- `/clash` live on-request compilation与preview采用同一DNS事实；独立历史artifact不可变无需改写。其他目标不支持完整DNS配置必须清楚提示范围，不谎称已给singbox/surge/qx恢复DNS。

## 7. 工程实现分界与验证安排

### 工作包 A（先）：领域/后端完整闭环
修改 domain/{probe_health,node_filter,DNS新值对象}、probe service/pool、inventory read model、sqlite/node_predicates/consistent_facts及依赖检测、DNS repository/migration、Settings HTTP与wiring、policy DTO/resolver统计、publication/compiler。输出明确新增字段/API契约和可运行合成集成用例，文档引用本文。先行为红绿再兼容回归，不在业务之外放平行adapter/DSL。节点状态SQL与read model/池守恒是硬门禁。

### 工作包 B（后）：同一事实前端与隔离预览
修改PolicyEditorSheet/共享StructuredFilterEditor/types/usePolicy/必要GroupCard，NodesView/nodeView/ProbeEvidenceSheet同步三态，新增DNS页/API/composable/nav/router/locales。重用已有组件/样式，不造新设计体系。前后端契约以工作包A已证实输出为准，不能猜字段。

### 测试矩阵与门禁
1. 固定clock的baseline成功→最新失败(transport/timeout/安全拒绝/配置错误)、旧成功超过1h/24h但版本相同、版本变化/仅辅助/无记录/历史NULL revision/取消/并发检测；API/SQL筛选/pool/导出一致，所有主态总数守恒，无自然过期/待复核。
2. 区域HK与geo.available分别验证；成功但缺Region、最新失败盖旧地区、版本不符、来源名字选取ID稳定、AI/流媒体某platform结果不得替baseline；neq未知不能误入；PCRE/lookaround有界。
3. 简单真实组改一个条件自动预览一次保存，复杂sources有序add/remove/重入/子组/builtin/WARP无损；候选净结果解释闭合且不存在“排除43却无排除”。
4. DNS默认/自定义/禁用/未知键/视觉Raw切换/非法YAML/多文档/过深输入/409冲突/重启持久化/preview与clash导出/hash变更；保护settings其他字段及源节点规则。API层权限沿既有Settings。
5. 重型Go/Vitest/typecheck/build只Docker或云，不宿主；前台已清Go build cache，不假定仍在，不清共享缓存。隔离SQLite、合成来源/节点/观测、Docker internal禁外联、临时loopback，不碰生产/真实订阅/真实探针。
6. 最终合并源码独立Reviewer通过且主脑核对实际疑点后，追加真实URL-ready桌面/392px移动视口Critic，不能用无横向溢出代业务可用。contract初始critic=false不免除任务UI实际验收。主脑需亲自走‘已有香港组改可用性/出口地区→预览→一次保存→回读’及DNS，不能仅转发PASS。
7. 所有通过后逐文件本地提交，清理本轮预览、QQ交付。**不push/部署**；生产待复核仍需用户后续批准发布新版本后才变化。

## 8. 成功判据与取舍

- 不再存在可配置“区域等于available”作为地区筛选；节点可用性/出口地区/服务结果有独立业务含义并作用真实resolver。
- 用户打开已有筛选就看到已有条件，可在同一草稿完成更改并一次保存，复杂存量定义不丢失。
- 待复核根因解释可回放，网络失败归真实失败、未测试归未测试，时间推移不制造状态。
- DNS页面可访问、配置可读改保存、复杂键保留且导出真正含DNS，存量数据与归档原DNS不被擅动。
- 本次优先完整业务闭环而非恢复旧页面所有装饰/模板/默认自动套值；不新增地理数据库、不新增外联API，不将名称地域当实测出口。

## 9. 当前执行状态与规则追加

已按用户追加完整读取最新主脑/共享治理，不修改Pi框架；主脑已按正常操作路径、状态含义、场景和实现映射自检，文档完整不代表功能正确。上述revision反例已修正调查解释，业务条件原本的版本隔离保持。施工两上下文均未产生业务diff，执行被停/准备报告BLOCKED；见 [filter-dns-execution-status.md](filter-dns-execution-status.md)。S02–10尚未完成，不得以本设计或历史门禁宣称本轮交付。后续恢复/审查须读取最新正文及本补充，不原样盲重派。

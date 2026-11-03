# Implementation Plan: Settings and chain restoration

## Technical context and scope
既有 Go/SQLite + Vue3/TypeScript/Tailwind4/DaisyUI5，不升级底座。历史 Python 链 UI/表/算法在重写中未移植，当前不是仅入口缺失；恢复为 Go 单轨，不引旧运行时/名称匹配兼容。CSP 指现 settings，DNS 指现 /dns。沿 /settings、/dns，新增 /chains；同一设置领域导航在三个页面可达，无额外管理对象。

## Constitution check
资产、logical_id、领域解耦、极简存储、规则不破坏、规范追溯均保持。新表仅存绑定，不复写 nodes/groups/rules；不自动导入旧 Python 数据、生产数据或恢复备份。没有违宪例外。

## Module design / contracts
详 contracts/chain-api.md 与 data-model.md。各模块不得重定公共字段：

A 链核心（domain + resolver）：domain/proxy_chain.go 新模型与 repo port；resolver/types.go 添加输入 ChainBindings 和节点 DialerProxy/ChainBindingID/ChainSource、snapshot ChainEffects。resolver/chain.go 唯一展开算法，由 Resolve 在现节点/组解析后、snapshot digest 前调用；digest.go 把绑定按 ID 排序纳入 input digest，snapshot 已含最终节点字段和 effects。先按 enabled 过滤，三类目标展开为现 snapshot 输出节点交集；source 为 NodeSources 的 subscription_id。节点目标与跳板稳定 ID；策略组来源复用 ResolvedGroup.AllNodeLogicalIDs。节点优先于组优先于订阅，同级 sort_order 小值优先、再 ID 字典序。部分目标属于 dialer 组叶子或等于 node dialer 时跳过并给原因；若全部非空目标重合或直接同组则错误。空目标可保存并预览0；跳板节点须存在且可导出，跳板组须非空且存在；失效启用绑定不静默忽略。

所有启用绑定完成优先级选择后，构建节点 dialer 依赖+策略组到递归叶子的依赖图，DFS 检测节点/组/绑定之间的传递环；不只检测自指。动态状态变坏时失败而非直连。disabled 绑定不参与效果、环检或引用存在性检查（字段枚举仍合法）。不自动将被筛除跳板加入导出，需修正筛选配置。

B 持久化/应用/HTTP/装配：新 migration27、sqlite/proxy_chain.go、application/proxychain、http/proxy_chains.go，配 router config 与 cmd wiring、publication 输入加载。应用服务拥有事务级全局 CAS（以所有绑定配置的稳定版本摘要防并发跨绑定成环）以及行 revision；repo port 与 API 冻结见合同。应用通过发布 AssembleResolveInput 取得同一来源/事实/组信息，在 candidate 替换完整绑定集合后调用同一 resolver，不复制环检算法；持久化在配置摘要比较的事务内 CAS，失败409不写。发表 service WithProxyChainRepository 加载每次现绑定；enabled subscription 绑定需要 NodeSources 即便无全局 source filter。任何启用链存在则纯节点 target 在 stripping group graph 前明确拒绝 unsupported_target_capability（本轮不实现 singbox detour），disabled 不影响原无链纯节点路径。

C 编译：只消费 snapshot 节点 DialerProxy，在所有 Mihomo proxy 协议 emit dialer-proxy，无链不输出。其他三格式对有 DialerProxy 的 snapshot 也防御性 CapabilityError，不能静默丢弃。不加对象/dict双轨；必要通过现 typed proxy 内嵌公共结构实现，不添加 wrapper 转发生产算法。保规则 PROCESS-NAME/global order/MATCH 与 DNS 内容。preview/publish/direct 都走现 snapshot，immutable 已保存 publication 内容不重写。

D CSP 设置与导航：重构 SettingsView 为用途清晰的标题、section 导航与内容；服务连接/订阅 URL和fetch、检测schedule/timeout、全局筛选、安全token/admin/export、外观本地theme/locale。沿既有独立保存端点，不全页大save，页面/分类切换未保存草稿不丢。安全函数拆可读 composable/组件，保现 API/AuthSettings、cookie/CSRF/rotate行为。管理认证不能在验收中改生产。SettingsSections.vue 在 settings/dns/chains 复用（三 links），settings 模块独占写；路由/App/nav changes 后续链UI独占，D不碰。

E DNS：保持 useDnsDraft 单权威与现 API，改 DnsView/FieldControl/GroupCard/fields 的常用区和高级区。主备/引导DNS常显，逐项列表和多行粘贴；布尔保未指定状态但更直白。映射字段改 YAML 编辑（可 typed mapping 基础），invalidYAML 原文保留、错误不能被切模式抹掉；unknown nested 用原对象保留。完整raw YAML与visual同一draft，outerenabled/CAS/reloadcancel/useUnsavedChanges保。规范化预览明确本次草稿，过时预览标记而非冒充当前；不新增危险默认模板覆盖用户配置。

F 链设置 UI：features/chains Vue components/composable/types 与导航/router/App 本轮归属。调用合同 GET options + GET list，真实ID选项/显示名分离、失效已选项不被空控件清除；编辑drawer单draft，预览250ms/代次/错误，计数/样本/skip原因和优先级说明。真实 CRUD/rowbusy/confirmdelete/CAS409保草稿，搜索URL仅公开筛选。节点链台账在本页面效果面板提供节点→跳板/来源，不修改全库Node详情接口。设置区与独立路由 /chains 均到达。

## Ownership, parallelism and integration
接口冻结后 A/B/D/E 四包无交叉写并行，B可根据已冻结新增 types 编码、不要求 A 完成才能写。A负责 domain/proxy_chain.go（含repo port）、resolver/types/chain/digest/resolver与tests；B不能写这些，只读契约。B负责 migration/sqlite/app/http/router/cmd/publication 和相关测试。D负责 settings 和共享 SettingsSections；E负责 dns 目录并可静态引用冻结共享SettingsSections接口。C/F 下一波并行（分别 compiler 与 chains/router/App/nav），只读 A/B 已完成接口。根 locale 不并发写，新增文案局部 uiText；README 更新由最终集成包独占。

## Verification and release boundary
Git index/commit/push 由主脑串行，保护原224dirty（入口web/internal/cmd/migrations无dirty）。每修订精确files开发提交推dev→现 CI 全 checks/runtime，promote skipped正确；本机不Go/node/Python项目test/type/build，也不本地Docker编译。可gofmt/静态diff。每模块新增行为测试交云端，不拿静态PASS当自测。最后独review读当前SHAjobs与artifact、风险核，拉同SHA云镜像 internalnet/合成DB/禁scheduler/loopback预览，Critic真实保存/readback/export/409/环路与desktop/mobilelightdark PNG。无生产部署、真实刷新/probe/改现银行规则、prelease推进或历史Release清理授权。

## Decisions
恢复历史三类目标/两类跳板与层级优先级，纠正旧名称作为身份及同级模糊覆盖。增加配置集合CAS用于防并发环，这个内部版本不成为用户需维护的实体。Mihomo链语义为本轮完整目标，其他格式真实拒绝避免假绿导出；无链原行为不变。可读性优先，不以压行或迎合旧测试引入兼容。

## Recovery correction: live export, repair and mandatory versions
首个全绿候选00a1dce仅证明现有测试通过，不能替代消费链审查。定点复核确认：输入装配对旧持久化订阅引用的前置校验会阻塞候选修复；API版本可省略；直接订阅走ConsistentFacts而未接入链。采纳这些实质问题，递归栈等无实际规模证据的优化延期。修复范围为以下两个不交叉模块，不引兼容双轨：

1. 发布和事实模块负责 `internal/domain/facts.go`、`internal/repository/sqlite/consistent_facts*`、`internal/application/publication/*chain*`/`service.go`、`cmd/csp/chain_wiring_test.go` 及其必要模块内测试。`ConsistentFacts` 增加 `ProxyChainBindings []ProxyChainBinding` 和 `EnabledSubscriptionIDs map[string]bool`，在同一事务加载完整绑定及启用订阅身份；启用订阅目标需要来源时强制加载NodeSources。BuildCurrentArtifact将这些绑定传入唯一resolver，在图剥离前拒绝非Mihomo启用链；订阅引用验证使用同快照的启用ID集合，不在纯编译期间额外查库。输入装配只装配，不在候选替换前拒绝旧绑定；实际发布/resolve入口仍校验当前启用订阅引用。候选管理服务自己验证候选，故旧订阅删除/停用后能停用、改目标或删除该绑定，未修复的实际导出必须失败。所有direct/link/check入口均经同一BuildCurrentArtifact，旧immutable发布不改写。
2. 链管理模块负责 `internal/application/proxychain/*`、`internal/transport/http/proxy_chains*`、`internal/repository/sqlite/proxy_chain*`：所有preview/create/update/delete必须非空config_version；update/delete必须正expected_revision。缺失/空/0/负数422，格式有效但过时409且零写入；DELETE仅合同JSON body，不保留query/EOF垫片。仓储Replace同样不可空版本绕过。同步更新被废弃默认值绑定的测试，不能生产兼容旧测试。已有事务CAS仍使用实际读版本防并发安全，API必填是额外用户冲突保障，不能把原代码夸大为一定能并发造环。

关键复验场景：绑定有效订阅后停用/删除订阅→导出失败→通过API停用/改目标/删除成功；真实链保存后GET/clash/sub/mihomo及活链接输出同一dialer；开启链后三种非Mihomo明确422，停用后恢复；不带版本/修订号写请求不能改变集合。所有新测试仅云端运行。

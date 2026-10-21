# 当前run模块级执行与接口协调

Run pi_run_20261020_144514_808995 / 002-policy / default / epoch1。依据用户module-scope-20261020-1845追加，主脑完整读取最新角色与共享治理后收敛。旧run/旧blocked只是历史；本轮已有真实S02实现与四包容器自测，非整体完成。用户原授权不含push/deploy/真实探针/源刷新/生产DNS导入。

## 当前成果与证据

S02 domain分类、Inventory操作态解耦、pool计数与SQL同义已实施。/tmp/csp-808995/health/s02_suite.log sha233f835a6f10efea40fc0ae02fc2b4d7afd728b8af61534e9190bf298c6814cb，domain/inventory/probe/sqlite容器测试exit0，旧失败日志保留。仅S02阶段自测，无独审；S03 domain字段/匹配已落盘，domain单测绿但加载/保存全链尚未完成。9b12...报告机器verdict PARTIAL不在允许枚举，不能接受其为PASS，后续须BLOCKED或真正阶段完成报告。

## 模块划分与并行

按业务内聚能力限定到明确文件集，而不是允许子机全仓按调用链漫游；同目录不同独立文件不得互相覆盖。所有普通依赖外信息由主脑提供或收到问题后定向读取协调；共享生成物无并发运行。

1. 策略筛选事实模块：domain/node_filter.go及其测试、policy_sources.go(必要规范化)、sqlite/consistent_facts.go及相关filter测试、application/policy/service.go和publication/service.go的filterDependencies/保存部分及直接filter测试。只对新条件事实加载/保存规范化闭环，不动DNS/其它publication逻辑。DNS后续整合触同文件，必须待此模块结束才派。必要元数据主脑已核实：三处依赖检查只识别probe_verdict/probe_latency_ms，新三维度须加入；Preview无provider须拒绝新证据条件。来源保存入口SaveGroup/EditLegacySources需移除freshness且不破坏sources顺序。
2. DNS领域与持久化模块：仅domain/dns.go,dns_test.go、domain/dns_ports.go、新sqlite/dns.go,dns_test.go、migration000026_dns_config.sql。接口冻结见下；不能读现有其它域以猜实现，已核实yaml.v3/SQLite/db repository模式由主脑交接。与筛选模块文件不重合。
3. 策略编辑前端模块：web/src/features/policy/**及web/src/features/settings/GlobalNodeFilterSettings.vue(共享组件调用必要)，其余web模块只主脑协调。不改全局locales/nav/router/package/build输出。完整字段契约主脑交接，不依赖后端最终metadata工件再开工。一次草稿/实时预览/一次保存、普通与复杂原序保全；订阅名API由主脑给端点。前端只定向测试，统一build由集成模块串行做，避免共享生成物。

三模块可并行：写文件互不相交、无共享生成物、不依赖另外模块新代码。后续模块：DNS应用/HTTP/编译集成(待1/2结束)、DNS页面+节点前端(待3结束以免types/locale冲突)、最终验证与预览。每次派工必须列清唯一允许读写模块与外部输入，不允许自行扩大到直接依赖；必要接口/读取清单由主脑决定。

## DNS外部接口（主脑冻结）

domain.DNSSettings {Enabled bool json enabled; RawYAML string json raw_yaml; Revision int64 json revision; UpdatedAt time.Time json updated_at; Source string json source}；domain.DefaultDNSSettings()返回default/enabled true/raw含enable:true及nameserver1.1.1.1/8.8.8.8，revision0；domain.ParseDNSSettings(enabled,raw)返回规范DNS map与error(具体函数名由模块保持明确交接即可)，只单mapping/dns-wrapper，64KiB，duplicate/multidoc/deep/alias安全；外enabled唯一有效，unknown合法键保留。domain.DNSSettingsRepository Get(ctx)(*DNSSettings,error),Update(ctx,*DNSSettings,expectedRevision int64)error；NewDNSSettingsRepository(db *sql.DB)返回接口。默认行不预置或revision0可读default；首次保存expect0原子新增revision1，之后CAS递增，冲突用现domain conflict错误约定由主脑交接。禁自动加载旧库。规范化保持Raw未知值，解析失败不能返回空对象当成功。

## 前端新条件契约

health_status equals/not_equals value healthy/unhealthy/untested；exit_region equals/not_equals ISOalpha2；platform_verdict equals/not_equals value available/restricted/unknown/error,probe_kind ai/streaming,platform openai/claude/gemini/netflix/disney/youtube；probe_latency_ms lte value数字字符，probe_kind省略或baseline。旧probe_verdict仅高级原样，不提供普通区域=available组合，不再展示freshness/stale新条件。source_subscription_ids contains/not_contains仍订阅ID，选项应来自真实订阅列表名字。无记录地区/平台neq不匹配；健康三态neq按三态比较，未测不是可用，需要可用时明确healthy条件(不得前后端另造排除未测规则)。

## 适用门禁

所有测试构建Docker/云；Go只读repo/modcache，GOPROXYoff/network none、本runbuildcache；前端定向vitest不与全局build重叠。最终合并全量自测→独立Reviewer→主脑风险核对→健康隔离URL Critic/主脑真实流程→逐文件本地commit/清理/QQ。禁止以domain绿或模块报告替整体门禁。

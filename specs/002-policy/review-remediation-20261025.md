# 2026-10-25 整体 Review 整改设计与验收契约

- Run: pi_run_20261025_165716_739681 / csp-review-remediation-20261025 / default / epoch 1。
- 当前 feature 来自 `.specify/feature.json`：002-policy；本文件为 spec/plan/tasks 的共同组成部分，不创建平行 feature。仅更新受影响章节，不重新执行覆盖 setup。
- 基线 HEAD c8c4dc3c6e4bbabf0ab64ebe37d2549579bbd880；原规划副本及既有 dirty 补丁先保存于 `/home/service/hermes/workspace/tmp/csp-remediation-739681/baseline/`。
- 来源：完整原整改意图包及 resume 包、review/report.md；当前授权实施、开发提交推 dev、Actions、同 SHA 隔离验收；生产部署/设置/DB清理/重启未授权。旧 analysis PASS 不作当前门禁。

## 结果、场景与明确范围

正常路径：用户管理规则→保存→重启后仍是原规则；探测收到合法风险结果→显示可信评分；用户看节点/选组/删订阅，服务失败时看到真实错误并能重试，不把失败伪装成成功或404。

### FR-RM1 规则生命周期

已有 revision 的规则，无论0、1、469、470或更多条，启动不得自动恢复默认；修改、顺序、启用状态和草稿保持原样。新建且原先完全没有 revision 的数据库仍自动初始化默认规则一次。初始化失败不能形成被误认作已完成初始化的部分状态；真实仓储错误要报告失败，不把查询失败当新库。不新增自动清理/迁移已有用户规则。

复用 EnsureActiveRevision 的现有边界（仅无active且total=0创建初始revision），删除 main 中 `<470` 自动替换逻辑。默认seed仅属于首次初始revision创建，须使初始revision+默认规则可原子提交或等价安全失败；现有显式 backfill/恢复默认操作保留，不更改其API语义，不自动触发。

验收：新库初始化470且二次启动不变；正常删除一条并修改另一条后重启469条内容不变；有意清空保持0；少量自定义保持；已有草稿无active不误建；初始化失败回滚、仓储读取失败不seed。

### FR-RM2 风险证据

Scamalytics score在判available前必须完整解析成0..100整数，正常0/100、现有有效格式保留；缺失、非数字、越界、尾随垃圾、响应结构漂移归unknown/contract_drift而非available。runner持久化防御一致：available必须有合法score，异常不能写available+nil/80；失败/未知不伪造固定可信度。协议与HTTP返回不新增配置。

验收：有效中间值与0/100；负数/101/小数/空/文本/后缀垃圾/缺失；runner观察状态、score、confidence一致。使用HTTP隔离fixture，不真实探测节点/外部提供商。

### FR-RM3 节点错误反馈

详情失败区分真实404、网络、500；列表缓存可保留但必须显示未刷新及重试，不静默假装最新；深链接失败不能一概404。过时请求失败不能污染新选中节点，不引入未处理Promise拒绝；探测后刷新等既有调用方保留安全行为。不存在的节点依然明确不存在。复用ApiError、现有抽屉/提示和请求代次，不造全站状态框架。

### FR-RM5 拓扑完整性

全量拓扑请求成功才发布完整集合，不用当前分页结果冒充完整。loading/error/ready可见；任何页失败保留已编辑来源与引用、不丢用户草稿；保留旧成功集合可用作展示但标记过期，不允许基于不完整数据创建/覆盖组引用或保存来源。提供原位重试，成功后恢复。最新请求胜出，旧失败不得覆盖新成功；列表搜索/分页不改变全量语义。现有编辑字段可继续显示，保存需等完整数据就绪以避免误写。

### FR-RM6 删除重试

订阅删除API失败显示错误，确认框及待删对象保留，按钮解锁可重试；仅DELETE成功关闭。删除成功但随后列表刷新失败不得诱导再次DELETE，区分mutation和reload结果。复用toast，不加二次审批。

### FR-RM4 留存策略（本轮有据暂缓）

未确认保留期限与回溯依赖，现有prune未接入的问题本轮不通过随意设期限处理。不启用后台删除、不变生产数据；最终报告明确遗留与最小决策：先确定原始载荷/历史需要保留多久以及固定/发布引用是否永久保留，再实施dry-run与有界维护。此项作为明确待决项，不假勾实现完成。

条件建议：宪法原则4明确明文直存，故不恢复vault/加密迁移；HTTP资源边界非本轮确认缺陷，不顺带修改超时/登录策略，以免破坏长任务；留下依据与后续建议。

## 模块契约、接口与归属

A 初始化：cmd/csp/main.go、app_services.go；internal/application/policy的bootstrap；internal/repository/sqlite中revision/seed事务及相关测试。负责首次创建与已有数据保护；不得读取/修改probe/web。仅必要时调整domain端口并先报主脑；优先在现有构造/仓储事务内完成，不新增用户配置/表。

B 探测证据：internal/probe/platform/iprisk.go及fixture测试；internal/application/probe/runner.go及风险相关测试。输入provider响应，输出合法verdict/score与持久化观察。可在本分区共享严格数值解析，避免两个正则不一致；不改全域实体/API、不碰A/Web。

C 前端失败状态：web/src/features/nodes、policy、subscriptions的hooks/views/components/tests。只改失败路径及必要反馈/重试，正常编辑/保存/API保持；三feature属本次错误反馈模块，其他共享api/ui只读，需修改先报。R3、R5、R6不要求服务端接口变化。无新依赖。

A/B/C写集合不相交、无共享生成物，设计冻结后并行。主脑拥有spec、Git index/commit/push、CI协调与外部证据，施工不得提交/推送。最终必须同一合并SHA审查与成品验收，不拼凑旧SHA通过。

## 技术约束与验证

Go模块/ Vue TS、SQLite。宿主在/home/service/clash-sub-parser；python3/node/gh/docker可用；graphify图与命令均不存在，记录缺失，不本轮生成全仓图。禁止宿主及本地Docker go test/build、前端单测/typecheck/build。可gofmt、差异检查、源码检查；真正自测在既有.github/workflows/ci.yml的远端Actions。施工先实现与测试源码，静态交接PENDING云测，不虚报PASS；主脑开发提交推dev后，施工复核同SHA云测/工件并报自测。

远端必需Backend & Frontend Checks、Build & Publish Runtime Container成功；dev prelease job skipped正确。核对镜像digest及OCI revision，同SHA预构建镜像用scripts/cloud-delivery隔离volume/loopback端口、关闭subscription/probe调度，零生产挂载。独立reviewer在准备critic前确认代码、CI和核心风险；critic在healthy URL上走真实正常流程+浏览器故障注入（500/断网/多页失败/DELETE拒绝为明确夹具），直接看截图。桌面及窄屏，不强制物理手机（未获需求），不冒称ARM/Android真机。

只预览测试DB可以创建/编辑/删除合成数据，最终准备者按marker回收容器/卷/网络并核实，不触碰生产。所有开发提交仅显式本轮文件，不夹带既有224项dirty；最终独审与critic通过后再持久化最终交付工件并提交。若云端门禁失败，保留失败，按日志修复推dev复验，不退回本机编译。

## 取舍依据

- 一次性历史470恢复不能当持续规则数不变量，否则与用户删除/自定义冲突；FR-RM1优先于旧规划对恢复完整470的历史描述，仅覆盖运行时自动补齐语义。
- 不新增DB retention配置/后台清理，因期限与证据保留未确认；不是不修确认缺陷，而是不可逆优化的授权边界。
- 明文静态存储遵守现有宪法，不根据review条件建议反转产品决定。

## Spec Kit analyze（实施前）

沿现有feature增量spec/plan/tasks检查：FR-RM1→RM25-01，FR-RM2→02，FR-RM3/5/6→03，FR-RM4→04明确待决；SC-RM1..3→05..08。无未映射实施任务，当前8项任务与6项FR覆盖100%（RM4为评估交代非删除实现），无关键接口未知，无新增宪法冲突。旧470恢复要求被明确收窄为首次初始化/显式恢复，避免与本轮数据保护冲突。验证位置与push/部署授权按最新AGENTS修正，不继承旧仅本地验收或不push历史条款。Critical/High=0，READY实施；后续失败在同tasks追加整改不改写历史。

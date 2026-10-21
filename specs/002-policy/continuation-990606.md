# 失败断点续办 pi_run_20261020_204735_990606

当前唯一身份：run pi_run_20261020_204735_990606 / change002-policy / default / epoch1。用户“CSP失败了，继续完成”。不重规划，不继承旧报告身份或PASS。HEAD2969d537，入口/tmp/csp-990606-initial-status.txt，保护304项既有dirty及旧失败。沿filter-dns-redesign.md FR-S1..4与module-execution-808995.md接口/模块边界，后者run/结果仅历史。

已核查真实源码：S02三态实现与测试已在工作树；S03新字段/依赖加载/保存去TTL已在。最新Publication测试不是简单‘都缺revision’：export_filter_test.go current obs ObservedAt=time.Now()，需核对service固定asOf导致观测落未来，以及断言仍要求净保留1且排除US却文案写接受旧观测，存在夹具时间与断言冲突。必须以真实调用链明确归因，修测试不放宽生产revision/future门槛，保留immutable artifact/缺观测拒绝保护。

模块：A 策略筛选事实(domain/node_filter*,policy_sources、sqlite/consistent_facts相关filter测试、application/policy+publication筛选代码/相关tests)，修最后失败并补新增字段全链测试。B DNS原生领域和持久化(只新domain/dns*,sqlite/dns*,migration26)，接口按原模块契约。C 策略前端(policy/**与settings/GlobalNodeFilterSettings.vue)，单草稿+真实独立维度+订阅名。A/B/C写集独立，无生成物并行。DNS后端消费集成须待A/B完成(共享Publication/facts)，DNS页面/节点前端待C完成再接全局nav/locales。

新进程实际批量派工验证而非假称热加载；扩展修复不在本任务范围。普通模块外读取/修改交主脑，明确只读例外。Go重型Docker--network none、repo ro、/root/go/pkg/mod ro、GOPROXYoff、Go1.27、本轮cache可复用旧/tmp/csp-808995/go-build；Node镜像22与现node_modules，不hosttest/build，不清sharedcache，不production17000/18080/DB/真实备份fixture/探针/刷新/旧DNS导入。无push/部署。

规划一致性复核：FR-S1→S02/S06、S2→S03/S05、S3→S05、S4→S04/S06，S07–09最终自测/独审/真实UI全部必需、S10仅门禁全过才逐文件本地提交。check-prerequisites解析现Feature exit0，图工具与graph缺失不安装不报更新；未改constitution，无新需求未覆盖，旧S02–10未勾不等于无实现，按新证据验收后增量更新。当前是实施续办，不以规划工件或阶段自测宣称完成。

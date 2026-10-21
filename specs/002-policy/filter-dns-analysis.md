# Spec Kit Analyze — 2026-10-20 当前修订

Run pi_run_20261020_111741_708234 / 002-policy / default / epoch1。

官方 check-prerequisites --json --require-spec --require-tasks --include-tasks exit0，当前feature specs/002-policy；无extensions hooks。本轮按官方specify/plan/tasks/analyze流程增量更新，不重跑初始化覆盖既有工件，宪章未变化。

| 需求 | spec定义 | plan实现 | tasks/验收 |
|---|---|---|---|
| FR-S1 待复核根因与三态/无时效 | filter-dns-redesign §2,3 | 后端domain统一+SQL/read model/pool，前端同步 | S02,S06–09 |
| FR-S2 探针筛选真实维度 | §4 | conditions原生扩展与facts依赖 | S03,S05,S07–09 |
| FR-S3 普通筛选单草稿/复杂sources无损 | §5 | 投影已有sources，不改原有序语义 | S05,S07–09 |
| FR-S4 DNS页面/存储/导出 | §6 | singleton/Settings API/事务facts/compiler/UI | S04,S06–09 |
| 保护资产/无生产发布/无外联 | §1,7 | 原领域/稳定ID/隔离合成数据 | S07–10 |

4/4需求均有业务、数据/API、实现和可执行验收映射，无孤立任务。关键易漏项已冻结：geo.available不等于区域；最新失败覆盖旧成功、无baseline不伪造可用；旧TTL字段不再影响筛选；SQL分页与池状态同义；Raw未知键保全、DNS真正输出、历史配置不自动导入；复杂add/remove不强行重排。没有只写调查问题让施工猜业务。

设计取舍：DNS原Go版本无持久能力必须增加最小表/API，不能假装只恢复菜单。后端先行，前端引用实际契约，避免并发改共享文件。输入限制与乐观锁覆盖失败路径；物理手机/外联测试不在范围。原R/D完成记录均标历史，现S02–10未勾，初始critic=false不冒充UI免验。

未发现未解决CRITICAL/HIGH设计冲突。两项有意兼容边界：旧高级probe_verdict仅原样保留而不自动改写为地区；默认DNS沿已硬编码值，归档配置不自动生效。均在规格明确，不以澄清空白推给施工。

结论：READY FOR IMPLEMENTATION，不是功能完成/自测PASS/生产上线。本轮施工仍需真实实施及独立验收。

执行后补充：当前仅设计READY，实施BLOCKED；已用跨revision失败反例修正调查解释但保留先版本再结果不变量。数字来源不混用，API未提供revision与DBNULL不等价。现阶段无任何功能门禁PASS，状态见filter-dns-execution-status.md。

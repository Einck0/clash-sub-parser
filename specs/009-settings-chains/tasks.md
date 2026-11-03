# Tasks: settings and chains

## Planning
- [x] T001 主脑核当前实现与旧链能力，冻结 spec.md/plan.md/data-model.md/research.md/contracts/chain-api.md/quickstart.md，保护入口dirty并明确云执行位置。

## US1 链功能恢复（A/B并行，完成后C/F并行）
- [ ] T002 [P] [US1] A：internal/domain/proxy_chain.go 模型/repo port；internal/resolver/{types,chain,resolver,digest}.go 和定向测试，优先级/overlap/完整环/失效/指纹算法单轨。
- [ ] T003 [P] [US1] B：migrations/000027_proxy_chain_bindings.sql、internal/repository/sqlite/proxy_chain.go及测试、internal/application/proxychain、internal/transport/http/proxy_chains.go/router.go、cmd/csp/{app_services,main}.go、internal/application/publication/service.go与相关测试；CAS/校验/装配/来源加载/unsupportedtarget。
- [ ] T004 [P] [US1] C：internal/compiler/{compiler,mihomo}.go 与定向测试；Mihomo全协议dialer-proxy、其他target显式拒绝，不破坏无链输出。
- [ ] T005 [P] [US1] F：web/src/features/chains 领域UI/types/composable/tests；web/src/{App,navigation,router/index} route导航，复用SettingsSections，实际预览/CRUD/CAS/效果台账。

## US2 CSP设置 / US3 DNS（与A/B并行）
- [ ] T006 [P] [US2] D：web/src/features/settings 的页面/组件/composable与tests；新增SettingsSections.vue冻结接口，重排用途/独立save/draft保留、安全不回归。
- [ ] T007 [P] [US3] E：web/src/features/dns 的view/controls/fields/draft和定向tests；常用项、逐项列表、YAML映射、17键unknown/outer/CAS无损。

## Integration / gates（FR-008/009及SC-001..004）
- [ ] T008 模块静态自查后主脑精确文件开发commitpushdev；现Actions全量checks/runtime，失败保留并定向修订云复验；不得本机test/build。
- [ ] T009 最终集成包维护根README使用导航与必要跨域测试；冻结消费接口/预览sameSHA镜像身份，合成DB/internalnet/禁scheduler/loopback，保PROCESS-NAME与规则hash/order。
- [ ] T010 未参与实现的reviewer独立审当前SHA源码、云jobs/artifact、数据不破坏与CAS/环检/输出风险，主脑裁决发现并定向复验。
- [ ] T011 ready URL交Critic，真实CSP/DNS/chain save/readback/export/409/环路径、desktop/mobile/lightdark与PNGread；不实体手机或生产。
- [ ] T012 owner受控回收预览及临时凭据，Spec Kit analyze一致性/任务状态和逐文件规范提交，QQ纯文本去重交付，明确生产未部署。

Dependencies: T002/T003/T006/T007 写集不交叉并行；T004/T005 等 A/B公共实现完成后并行。T005 独占App/router/navigation；T006独占SettingsSections，E/F仅消费。共享Git/index/提交push由主脑串行，最终README单owner。不按函数拆微任务，不跨模块自由探索。

# Tasks: settings and chains

## Planning
- [x] T001 主脑核当前实现与旧链能力，冻结 spec.md/plan.md/data-model.md/research.md/contracts/chain-api.md/quickstart.md，保护入口dirty并明确云执行位置。

## US1 链功能恢复（A/B并行，完成后C/F并行）
- [x] T002 [P] [US1] A：internal/domain/proxy_chain.go 模型/repo port；internal/resolver/{types,chain,resolver,digest}.go 和定向测试，优先级/overlap/完整环/失效/指纹算法单轨。
- [x] T003 [P] [US1] B：migrations/000027_proxy_chain_bindings.sql、internal/repository/sqlite/proxy_chain.go及测试、internal/application/proxychain、internal/transport/http/proxy_chains.go/router.go、cmd/csp/{app_services,main}.go、internal/application/publication/service.go与相关测试；CAS/校验/装配/来源加载/unsupportedtarget。
- [x] T004 [P] [US1] C：internal/compiler/{compiler,mihomo}.go 与定向测试；Mihomo全协议dialer-proxy、其他target显式拒绝，不破坏无链输出。
- [x] T005 [P] [US1] F：web/src/features/chains 领域UI/types/composable/tests；web/src/{App,navigation,router/index} route导航，复用SettingsSections，实际预览/CRUD/CAS/效果台账。

## US2 CSP设置 / US3 DNS（与A/B并行）
- [x] T006 [P] [US2] D：web/src/features/settings 的页面/组件/composable与tests；新增SettingsSections.vue冻结接口，重排用途/独立save/draft保留、安全不回归。
- [x] T007 [P] [US3] E：web/src/features/dns 的view/controls/fields/draft和定向tests；常用项、逐项列表、YAML映射、17键unknown/outer/CAS无损。

## Integration / gates（FR-008/009及SC-001..004）
- [x] T008 模块静态自查后主脑精确文件开发commitpushdev；现Actions全量checks/runtime，失败保留并定向修订云复验；不得本机test/build。
- [x] T009 最终集成包维护根README使用导航与必要跨域测试；冻结消费接口/预览sameSHA镜像身份，合成DB/internalnet/禁scheduler/loopback，保PROCESS-NAME与规则hash/order。
- [x] T010 未参与实现的reviewer独立审当前SHA源码、云jobs/artifact、数据不破坏与CAS/环检/输出风险，主脑裁决发现并定向复验。
- [x] T011 ready URL交Critic，真实CSP/DNS/chain save/readback/export/409/环路径、desktop/mobile/lightdark与PNGread；不实体手机或生产。
- [x] T012 owner受控回收预览及临时凭据，Spec Kit analyze一致性/任务状态和逐文件规范提交，QQ纯文本去重交付，明确生产未部署。

## Recovery corrections（保留失败事实，不以首个全绿代替完成）
- [x] T013 修复生命周期及新增真实Resolver夹具：合法logical ID/UUIDv7、完整SS参数、成功抓取来源、独立锚定库存；不放宽生产校验。
- [x] T014 ConsistentFacts同事务加载链与启用订阅身份，必要来源不依赖过滤器；BuildCurrentArtifact覆盖直接订阅/活链接/检查。候选修复不被旧绑定前置阻塞，实际失效导出仍失败。云端真实装配回归验证retarget/disable/delete。
- [x] T015 所有链preview/create/update/delete强制集合版本，update/delete强制正修订号，DELETE仅JSON body；应用/HTTP/仓储同步验证并补422/409无写入回归。

Dependencies: T002/T003/T006/T007 写集不交叉并行；T004/T005 等 A/B公共实现完成后并行。T005 独占App/router/navigation；T006独占SettingsSections，E/F仅消费。共享Git/index/提交push由主脑串行，最终README单owner。不按函数拆微任务，不跨模块自由探索。

## Verification closeout
已验收代码提交：`47951bd108c62e1b8a51783c498308c63bcd1a04`，Actions `41705292068` 的checks/runtime均成功，dev发布job跳过符合契约；前端61文件566测试、类型检查/构建、Go vet/测试、交付契约通过。镜像digest `sha256:8274fc3418a4cd393eb8b7e11f9a2edbbd9ea853e2ab592922c15e3506964886`，OCI revision匹配上述提交。

独立工程审查修复复核通过；Critic主流程52项与定点补充27项断言通过，直接检视桌面/移动视口与深浅主题PNG。覆盖真实CSP保存/分类草稿、DNS列表/映射保存回读与导出、真实409/非法输入、订阅/组/节点链优先级与部分重合、混合传递环、启停删除及实际Mihomo输出。移动视口不冒充物理手机。

隔离预览已由owner回收，清理前恢复空链基线；合成规则472条及PROCESS-NAME顺序、terminal MATCH和导出哈希恢复一致；容器/网络/数据卷/relay已移除、端口拒绝连接、凭据已清除。没有生产部署、真实订阅刷新、探测或历史Python数据自动导入。

本次收尾为规划状态与最终简报，不改变已验收源码；文档提交不冒称为另一个已经UI验收的镜像。

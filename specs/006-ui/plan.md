# Technical plan: modern frontend and GitHub delivery increment

Feature specs/006-ui。实现、云测、产品验收与新 prelease 发布完成；整体受旧 Release 删除 HTTP 403 权限阻断，非全部完成。已有工作区改动受保护；任务状态见 tasks.md。

## Outcome and design

全站八域现代化详modern-frontend-contract.md；新增GitHub/发布/部署/内部资料设计详github-delivery-contract.md。两合同均为spec组成部分，最新追加对dev自动Release/不得prelease长期分支旧结论优先。前端不只首页，不fork参考的代理运行时控制业务。

Frontend维持View→domain composable→ApiClient→Go API。服务负责事实/解析/导出/CAS；前端单draft、响应式展示与状态。Vue3/TS/Vite6/Router4/Vitest2保留，Tailwind4.3.3+Vite插件4.3.3/DaisyUI5.7.50精确版本；CSS-first去旧链，保7themes。官方peer支持Vite6/CI Node22，不盲升版本。不新增BFF/schema/global万能store。

Delivery维持现GitHub Actions→GHCR→image-only Compose，不造新系统。dev checks+sha镜像供隔离验收；真正prelease分支完整checks+image后固定渠道Release/tag/prelease镜像alias，只有一个滚动预发布；main检查不自动stable发布。release-source metadata/binarysha/digest/OCI用于核验，固定tag为可变channel不是不可变版本。移除Release-basedbinaryreuse；同SHA重跑以commit timestamp保证可重现时间元数据，对已有sha镜像校验再用而非盲覆盖。云端runner当前构建时间另有Actions日志，不把source时间冒称实际墙钟。生产Compose保原ports/vol/env/auth、强制pull策略；普通远程pull与受控pull后使用缓存两个路径解释清楚。

Privacy以精确文件停止追踪+gitignore/dockerignore为当前止损，磁盘内容及dirty保全；main独立限定后代tree不合前端、不强推。旧Release先离线恢复资产和镜像依赖再收敛UI；不删GHCR/historytags，不称当前止损等于清历史；真credential另报轮换，历史重写另批。

## Ownership and dependency

前端A基础/Bnodes/Cpolicy/Drules已按冻结公共接口第一波并行，Esubscriptions/FDNS/Gpublication-settings/Hdashboard第二波并行；共享dependency/lock/locale由指定owner负责。后续cloudfail按真实domain定向改，不重探索全仓。

追加发布模块独占.githubworkflow/promotion/synthetic/contracttests；部署文档模块独占Compose/公开手册/README/local AGENTS；privacy独占ignore与精确manifest，index和GitHub写操作主脑串行。shared tests/cloud-delivery归发布模块所有，部署包交断言事实而不并写。main树止损与旧Release收敛须单独独审计划，操作仅授权delta，不触生产。

## Constitution and validation

保稳定ID/失败资产/真实健康/有序来源/CAS/DNS损耗为零等，鉴权/CSRF/API不弱化。Constitution原则不变无需改；Spec Kit按SPECIFY_FEATURE_DIRECTORY=specs/006-ui、SPECIFY_FEATURE_NO_PERSIST=1。旧historical HRQ6/千万级不冒称本轮验收。

全部项目测试/类型/编译包括新增Python契约仅云端现工作流。本机仅静态差异/语法/metadata，以及同SHA预构建镜像实际验收。任何本机测试结果均不替代云端门禁。

explicitfiles开发commitpushdev→当前SHA云checks+image→无真实数据/无出网/禁scheduler preview→独review与关键风险核→Critic desktop/mobile/lightdark真实场景→必要修复同SHA链复验→prelease发布真实凭据与历史收敛/main止损独审操作→ownedcleanup/工件/QQ四部分交付。各分支条件分别判断，dev promote skipped是正确、不算已经发布；生产无当前授权。

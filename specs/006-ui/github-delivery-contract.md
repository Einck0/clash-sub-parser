# 2026-10-22 追加：GitHub、发布、部署与内部资料

追加不撤销全站前端；本文件覆盖现代前端合同和旧项目手册中 dev 自动创建 prelease-<sha>、禁止 prelease 长期分支等冲突条款。旧失败记录不重写。

## 目标与官方事实

用户明确要求：真正的 prelease 分支发布，公开只保留一个预发布；查清远程镜像部署；本地经验等内部资料停止外泄。不是新增一个分支名标签，不是再搭发布平台。依据 GitHub 官方 Actions 事件/权限、Release API/编辑资产、GHCR，与 Docker Compose pull_policy/pull/up 文档；证据存受限本机工件，不将内部路径和原始数据再提交公开仓库。

已查明当前 dev 每成功提交创建一个 prelease-<sha> Release，共17个，prelease 分支不存在；发布与日常云测耦合。当前生产和回滚使用不可变镜像digest，Release UI不是镜像存储。前次部署有真实远程pull成功，然后--pull never使用已校验缓存；不能称从未拉取。公开Compose无build但未显式pull_policy，默认missing可复用缓存。

.exp.md在远端dev/main及历史有追踪。本轮检查尚未发现已确认真实密钥，不等于全历史无密钥或从未外泄。维护manifest含真实节点端点等不适合公开；内部运行回执与经验本地保留。停止追踪不能抹除历史、旧提交下载、fork或缓存。改写公开分支历史、强推代码分支、凭据轮换需另行批准。

## 需求与契约

- FR-G01 开发与发布分离：dev push继续现云端完整type/test/build/Go检查和sha镜像，**不创建Release、不改发布tag**。PR只检查；main的PR/push也检查，不自动稳定发布。prelease push与选中prelease的手动运行通过同一工作流完整检查后才发布。取消易绕过分支限制的publish_release输入，job与发布脚本均校验refs/heads/prelease和非PR事件，失败不发布。
- FR-G02 唯一预发布：真实创建/快进远端prelease分支，不强推代码分支。固定tag和Release名prelease、prerelease=true/latest=false，目标是完整测试SHA。固定tag是明确可移动的渠道指针，不称不可变；元数据记录source_sha/run/binarysha/image_digest。sha镜像保持不可变，不删除GHCR现生产/回滚digest或其他稳定资产。发布串行、失败不谎报完成；不从Release下载二进制绕过本轮编译验证，删旧reuse模式。定向重复发布同SHA可验证重用，替换资产之后必须完整下载校验；单Release的更新不是跨资产原子事务，消费者用元数据哈希验证，过渡态不得部署。
- FR-G03 历史收敛：先本地保存17个旧Release的metadata和3资产并核checksum，重点保现生产062955f与回滚4b1fbcb的离线恢复材料；新prelease验证通过后只清理确属本项目自动生成且prerelease=true的prelease-<7hex> Release对象。不删历史tag/commit，不删任何GHCR镜像，不擅自清稳定Release。Release页最终只一个prelease。权限不足保留真实阻断，不能用新建更多Release掩盖。
- FR-G04 远程镜像部署：生产Compose继续必须CSP_IMAGE、无build、显式pull_policy: always；文档以公开GHCR远程预构建镜像为普通用户路径，推荐digest固定版本。受控部署先pull并校验digest/OCIrevision/binary，对已拉取不可变digest用up --no-build --pull never是有解释的受控路径，不作为普通读者默认省略pull。远程拉取失败终止，不能自动fallback至未知缓存；保现有端口/卷/环境/auth/健康检查，本轮只改文件不执行生产替换。不在部署服务器编译。
- FR-G05 内部资料当前止损：.exp.md、本机agent规则、含真实维护端点的manifest、纯内部运行/部署回执从index定向移出，**保留本地文件逐个hash**；gitignore/dockerignore阻止未来加入Git/构建上下文。正常公开业务spec/API文档不整棵删除。公开交付手册与README去私有主机路径、个人域名、内部回执引用，用example.invalid/公开镜像/可配置路径。对main使用快进的新提交移出相同当前tree文件，不把未验前端合main、不重写历史。已发布源码仍可能含旧文件，最终明确历史残留与另授权清史方案。
- FR-G06 验证与防回归：现CI内测试分支/事件真值表、发布脚本guard/固定Release元数据/失败路径、Compose无build/pullpolicy及已有隔离安全规则、公开Git树与Docker上下文排除。所有CSP测试与编译仅GitHub Actions；本机仅静态检查和预构建镜像验收。独立reviewer核source/SHA/jobs、当前远端树、Release数/资产hash与保留的镜像依赖，Critic仍覆盖前端。

## 模块与写集（主脑串行Git）

1. 发布模块：.github/workflows/ci.yml、.github/scripts/promote_prelease.py、tests/cloud-delivery/test_cloud_delivery.py中workflow tests及新增test_promote_prelease.py；保持其余Compose tests由部署模块另时更新或明确交接。只读scripts/cloud-delivery/preflight-pull.sh、Dockerfile.runtime、现官方证据。不触碰前端、ignore或文档。
2. 部署/公开说明：docker-compose.prod.yml、docker-compose.example.yml（存在才维护）、docs/operations/csp-cloud-delivery.md、README.md/README.zh-CN.md相关使用与交付段、AGENTS.md冲突项目段（保已有dirty差量），tests/cloud-delivery/test_cloud_delivery.py的Compose断言由主脑在发布模块完工后单独协调，避免并写。
3. 当前资料止损：.gitignore/.dockerignore、逐文件index移出由主脑协调；施工机只准备精确manifest/hash和ignore编辑，禁止自行提交/push、历史清理/rotate。内部receipt清单取自核实的受限工件，公开spec只放类别不放真实端点。
4. GitHub操作：主脑在源差异核实后开发提交pushdev；全绿与独立review后快进prelease，再取同SHA发布凭据与旧Release收敛；main止损用独立临时index/限定新tree descendant提交，不checkout/reset/覆盖dirty现场，动作前独立核对差异。

## 场景与验收

1. 修前端推dev：完整checks/image成功、promote job skipped属于**正确**；没有新Release或tag，供同SHA隔离验收。
2. PR/手动dev/main：不发布；仅prelease push或手动prelease在完整checks/image成功才更新唯一Release，脚本直接误调dev也拒绝。
3. 发布失败：保失败日志，不清旧Release，不部署；重试不产生第二个Release，最终下载binary/checksum/metadata三者与source/image一致。
4. 普通Compose使用固定digest：远程pull/策略明确；断网/registry失败无启动新版本；受控已pull验签路径不再被误解释为从未远程拉取。
5. 移出.exp后：本地仍存在且hash一致、dev/main最新tree无目标内部文件、ignore命中、历史git show依旧可见须明确披露；发现真credential立即通知需轮换而非宣称已消除。

## 决策记录

保留GitHub Actions/GHCR/Release和现小发布脚本，不造平台。固定prelease Release是用户明确单一滚动预发布的必要实体；旧immutable-per-dev Release与复杂复用废除。dev云镜像属于测试工件而非面向用户的Release，区分并在交付中说明。历史Release收敛不能以生产依赖为由永远保留多个UI条目：先验证镜像与离线资产独立可恢复，再删除旧Release对象，不删除镜像。未经当前授权不部署生产、不强推代码分支历史、不自动轮换凭据。

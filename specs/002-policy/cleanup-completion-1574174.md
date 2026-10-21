# DNS 数据恢复与策略功能去重：验收终态

身份：`pi_run_20261021_160307_1574174 / 002-policy / default / epoch 1`。

## 完成状态与授权边界

已完成 DNS 生产数据恢复，以及 DNS 标题去除、策略页去重和准入伪 DSL 退役的实现、开发推送、云测、隔离验收与独立审查。**新程序没有部署生产**：生产仍是 `4b1fbcb` / 镜像 `3ed17542…`；本轮仅明确授权生产 DNS 数据恢复，不继承上一轮程序部署许可。用户会在旧生产程序上立即获得恢复的 DNS 数据；页面与策略程序变更待另获上线授权后生效。

没有整库回滚、没有删除流量规则或历史准入记录，没有执行真实探针/刷新、没有本机或本机 Docker Go/Node 测试编译。没有修改 CI 或框架；既有非任务 dirty 保留。graphify 命令和 graph.json 不存在，未安装或冒称更新。

## 业务结果

- DNS 保留当前可视/Raw 编辑器及启用、校验、保存、错误与409反馈；去掉页内重复标题及默认来源迁移横幅，应用导航标题保留。
- 策略页只管理策略组、成员来源与全局软筛选；去掉重复分流/准入页签、列表、新建准入弹窗及多余请求。流量规则唯一入口为 `/#/rules`。
- 退役按节点名字猜国家的额外准入 DSL；实时解析只保留启用状态与显式风险排除硬约束。历史记录仍可读，存在时给一次 `legacy_admission_rules_ignored` 警告，不泄露表达式。准入新建/删除统一422 `admission_rules_retired`，不生成修订；准入专用删除不再误删同ID流量规则，通用删除不再隐式回退到准入记录。
- 保留全局软筛选、WARP软豁免、有序动态来源与安全校验。三个消费入口的帮助文字同步纠正，不再宣称已退役用户准入规则始终生效。
- 实际发现分类模式已有MATCH时普通新规则被验证顺序误拒；只调整分类模式候选验证顺序，保留重复MATCH拒绝与非分类显式位置约束。

## DNS 生产恢复证据

已确认历史源为宿主 `/data/clash_sub_parser.db` 的有效单条 DNS 记录，enabled=true，821字符、17顶级键。原始 SHA256 `57fcbd1056e78ba6db554cd5c37b459cf569c4477913c884a2045b92245d004d`；规范化 SHA256 `8c994f438f9c7f8c84dd530bfb01a4b74043bac89344c8760353d3b3d047f9bb`。该数据库没有历史更新时间字段，不虚构时间来源。

恢复前在线备份 `/data/backups/csp-v1-pre-dns-restore-20261021-1574174.db`，280449024字节，0600 / 10001:10001，SHA256 `fda82fa0a5ecbc704dde8e098f7238dbc566d5c1934faf696ace5be679966a82`，integrity/FK通过。原生验证后，仅PUT DNS expected_revision=0，成功到revision1、source=saved；GET与实际 `/clash` 的完整DNS映射和旧候选深等价。9订阅、28组、92组来源、14分类、470路由规则指纹保持，终结规则仍 `MATCH,规则外代理模式`。

单项独审 `/tmp/csp-1574174/dns/review/dns-restore-review.json` verdict PASS。原配置基线 `current-dns-before-restore.json` 以0600保留，真实在线备份保留；临时重复 YAML/候选副本已删除。若将来要恢复默认内容，用当前revision CAS保存基线；这不会把source伪装成default，也不授权整库覆盖。

## 开发与云端门禁

所有开发修订已推 `dev`。最终业务 SHA：`062955fbc81579f49d92487aaa449930e1ac0bd5`。

- Actions：[39585967910](https://github.com/Einck0/clash-sub-parser/actions/runs/39585967910)
- Checks `119174813593`：Web类型检查、42文件403单测、前端构建、Go依赖/vet/全套测试、静态二进制与元数据校验成功。
- Image `119176835618`、Prerelease `119177097650` 均success，`prelease-062955f`。
- 镜像 `ghcr.io/einck0/clash-sub-parser@sha256:a3f4796a50bc6f4b675f947d55318db3c478a5ced5687756ab75be27172a56c9`，OCI revision与最终SHA一致。
- 二进制 SHA256 `e441999ed38044064f1dccab522cf3583653ce0bd1faeb34b825f934fe1e240b`。

保留失败历史：39572158043残留TS导出；39573190149 Go未定义符号；39574196045测试变量作用域（Go vet阶段失败，不是运行断言）；39575127699旧e2e准入201断言与不合法id字段夹具。均通过开发修订推送后云复验，没有转本地测试或改API迎合错误夹具。16d9ec5的39576994813首次全绿，后续062955f纠正帮助文字并重新全绿。

## 独审与实际页面验收

最终工程独审 `rep_rev_default_943ff0b7-4be6-4140-8ffa-ffd5020f1014` PASS，复验首轮 `rep_rev_default_2fddf576-a4e7-48eb-94c1-c52ae2ad2ea2`；记录 `/tmp/csp-1574174/review/guidance-review.md`。

Critic原报告 `rep_crit_default_1_20261021_183000` 在旧SHA通过后，主脑实际读PNG发现帮助文字矛盾，拒绝将该项视为已覆盖，安排上述三处文字修复与回归。最终补验 `rep_crit_default_1_guidance_20261021_190500` PASSED，明确链接首轮。不是用旧PASS代替最新成品验收。

隔离预览只用同SHA预构建镜像、纯合成数据、内部网络无默认出口、禁订阅/探针调度；桌面1440×900与移动392×872浏览器验收，非物理手机/触摸屏验证。

- DNS实际保存/回读/导出17键保留，外层启用优先，非法YAML不保存，真实CAS409保留草稿。测试库revision增长不代表生产revision；生产恢复的独审值为1。
- 策略页无重复入口；组防抖预览、实际一次保存、GET与重开修订3→4，预览2节点与保存状态一致，Global-Auto三项动态有序来源保持，未通过修改empty_fallback_pass换取导出成功。
- 独立规则页真实UI新增普通分类规则201，导出470→471且MATCH末位；删除后恢复470。准入API422/硬软边界/同ID保护由云回归及工程独审覆盖。
- 实际读取桌面/移动PNG，确认控件可操作、无横向溢出；主脑另亲读最新移动全局筛选及DNS截图。长帮助文字仍可读，非新增标题横幅；不将视觉审查夸大为全产品无缺陷。

最终测试源未提交差异检查退出0。关键审计工件当前SHA256：

| 工件 | SHA256 |
|---|---|
| `/tmp/csp-1574174/dns/review/dns-restore-review.json` | `aadec7b5e87bf4b93b502b3b04da2d64f5c5b31755e7f6c02c9609ecd56a7fac` |
| `/tmp/csp-1574174/critic/guidance/guidance-audit-results.json` | `94654cde1846e5e3df98edd0e971ff35371613f120647df65ed02bae1767e59c` |
| `/tmp/csp-1574174/critic/guidance/guidance-global-filter-modal-mobile.png` | `138df656834a0eea488cfe6adbdfbea8f4f940ab5f8c017eb4da6b89e25bec7e` |
| `/tmp/csp-1574174/critic/guidance/layout-dns-clean.png` | `b8e031f07d5d0265ee5eb27b9137606796a415bb4006091a7cc9ece21eb9ab86` |
| `/tmp/csp-1574174/cleanup.json` | `58a2a80b550bf8dada60a723da44c2e19418e1e950c4c008a227879570b3bf4d` |

以上为主脑最终读取值；辅助机较早输出的工件哈希不能替代更新后文件的当前哈希。

## 回收与限制

专属预览容器61411b094380…、数据卷csp-isolated-data-1574174-ep1、网络csp-isolated-net、socat1620116与端口39741已逐项归属核验后回收。旧实例JSON追加closed状态，不再作为在线可用URL。审计日志/PNG保留，生产在线备份与单配置基线保留，无生产程序切换。

未做：生产程序部署、Surge/QX旧能力限制整改、真实手机验证、CI新增远程浏览器系统。它们不被本轮通过结果冒充完成。

## Spec Kit 增量一致性分析

官方check-prerequisites --json --require-spec --require-tasks --include-tasks解析Feature为本目录；没有extensions.yml附加钩子。按speckit-analyze复核当前受影响增量，不重生成历史文档：FR-C1映射C02/C03/C08；FR-C2映射C03/C05/C08；FR-C3映射C04/C05/C07/C08；C06云门禁与C09回收/交付为横切任务。覆盖3/3需求，无未映射当前需求、无无主实现任务，当前增量无CRITICAL/HIGH阻断。分类新增MATCH决定已合并原契约，旧Docker/不push/不导历史DNS仅为明确标注的历史阶段，不覆盖最新AGENTS和本轮授权。宪章资产保全、稳定身份、领域解耦、单一存储与活规范均保持；不修订constitution。当前实现验收完毕，生产程序上线仍需另获授权。

## 0. 本轮规划与历史事实纠正

2026-10-02：本轮仅取证/规划，未执行任何源码修改、构建、部署、受限API、刷新、恢复或提交。旧1.1/1.2/2.1表示历史准备/施工/审查记录，不是本轮新整改已过门禁；重写为以下待执行任务。旧3.1带身份正文/刷新/握手只有公开健康及PAUSED_CREDENTIALS_REQUIRED，明确撤销[x]。旧3.2仅历史0白名单审计不能证明恢复目标完成，旧3.3不能证明本轮发布与最终实网独立复验，均撤销。规划READY不勾选业务。

## 1. 完整特性包 pkg_csp_release_and_acceptance（一个executor实现与自测闭环）

- [x] 1.1 读取同一Change apply contextFiles；重读Git/运行归属/实际构建输入及合法凭据可用性，保持既有改动；冻结本包run/epoch、精确文件集合、脱敏基线/验收选择、备份恢复及发布计划。不改变原始意图包或历史run。
- [x] 1.2 在cmd/csp-live-acceptance/main.go与runner完整整改：汇聚fail-closed正文/JSON/HTTP断言，准确bool/pagination/订阅source集合核验（含来源集合排序精确保全），刷新错误与未知库存不绿，node_logical_ids+baseline+deadline及独立observations/真实终态协议（严格<=2分批串行与目标/revision/healthy强校验），凭据暂停不冒充业务通过，token-file/report文件安全校验（禁symlink/非regular/权限超标），脱敏/private scratch/report写入错误与清理。同步两份运行手册，保留既有业务保护，不改生产鉴权或服务接口迎合错CLI。
- [x] 1.3 同一施工会话用testing/httptest/t.TempDir及现有e2e SQLite完成design完整正负fixture矩阵（CLI真实入口而非仅API fixture），验证错字段不会扩大探针、空结果不PASS、等计数换ID也能发现库存损失、同ID丢失来源反例、探针<=2分批串行全覆盖、外来节点/错revision/unreachable失败反例、token-file/report安全反例、secret金丝雀无泄漏、私有scratch与故障清理。所有fixture禁生产/外部订阅/真实模型/QQ/密钥出口。
- [x] 1.4 执行go build ./...、go test ./...、bash -n scripts/run_authenticated_acceptance.sh、CLI/e2e定点命令与本Change strict，均exit0；输出绑定真实run/pkg/epoch的executor final结构化报告及commands。编译报错原地按行号修复，无进展caller_ping；不带错误交工。

## 2. 独立生产准入（依赖完整施工包自测通过）

- [ ] 2.1 未参与施工的reviewer只读对照同一Change/最终diff，独立重跑静态/全量测试和关键故障fixture，审核fail-closed协议、redaction、安全备份恢复/回滚/预算及本包真实合同；输出独立final报告，若复验历史失败引用实际reverification_of。未PASS不得执行生产副作用。

## 3. 已授权受控发布与真实验收（依赖2.1）

- [ ] 3.1 以已审查冻结源码树构建并记录真实image/input manifest，隔离临时DB验证；紧邻替换前在线fresh备份DB/库存/身份配置与Compose/env（私有权限），验证完整性及隔离恢复，pin/save当前807549镜像（不是旧0ea88）；记录精确回滚步骤，保持卷/双loopback/代理/DB verifier，仅no-deps/no-build替换目标app并验证就绪/入口/库存/身份。
- [ ] 3.2 仅有合法受管且匹配现有verifier的凭据后，执行带身份正文语义、至多3现有enabled订阅各1次刷新及至多6既有active节点baseline真实握手/探针；design建议10分钟总预算、GET10秒/刷新45秒、deadline遵守现有TTL。记录实际run/observations/库存集合及脱敏摘要；缺凭据或缺实网结果保持未完成，PARTIAL/BLOCKED不能伪造PASS。
- [ ] 3.3 按fresh实际历史证据重新分类；仅因果充分、唯一身份、有效来源且排除手动禁用/删除的独立审查白名单允许事务恢复，记录实际数量（可0）及受保护歧义/外部待决项。没有充分证据不写，不把历史983/960当批准，不把未完成恢复假勾选。

## 4. 独立终态与母体汇聚（依赖真实目标完成）

- [ ] 4.1 独立reviewer复验实际发布/业务/安全证据、必要恢复结果及全部尚未完成事项；确认构建/运行image、fresh备份可恢复、库存身份保持、实网成功和失败均真实、无秘密，输出本包最终报告。前置代码审查或公共健康不可代替此项。
- [ ] 4.2 母体核验真实当前run合同、全部包报告及官方status；保留旧UNVERIFIED，实际业务满足才报告PASS。核算实施文件patch_hash及contract_revision_hash，按授权做必要Git提交/终态通知；bridge遗留Change须在其各自根独立收口，不跨根补勾选。本规划阶段不执行提交/QQ/归档。

---
feature: 007-specification-governance
title: Specification Governance & Provenance Capability
status: candidate
schema: speckit-native
---

# Specification Governance & Provenance Capability

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的规范治理、源头追溯、用户原话确权、未决决策收敛与生产安全上线交付能力。确立 Specification-Driven Development (SDD) 原生契约，保障历史 26 个变更集、70 份规范文件与 154 份合同文档的双向无损追溯，严守用户原话最高授权原则，严格隔离生产环境写副作用。

## User Scenarios & Priority

### User Story 1 - 历史变更追溯与双向规范契约映射 (Priority: P1)
- **GIVEN** 项目历经多次迭代演进，存在大量历史变更与分项规格
- **WHEN** 开发者或治理工具检索任一历史业务需求与场景
- **THEN** 系统提供机器可读的双向追溯台账，精确定位至对应能力规范锚点与可验证场景

### User Story 2 - 证据来源分级与用户原话确权 (Priority: P1)
- **GIVEN** 存在历史推测、中间讨论与用户明确指令冲突的场景
- **WHEN** 评估业务行为与规范有效性
- **THEN** 用户原话与已核实机器事实作为最高判据，任何未经用户确认的猜测不得升级为硬约束

### User Story 3 - 未决问题收敛与决策日志归档 (Priority: P1)
- **GIVEN** 架构演化过程中存在待澄清业务分支或历史遗留冲突
- **WHEN** 推进规范基线冻结
- **THEN** 显式收敛为待决决策议题并归档至决策记录，严禁模糊合并或带病假结单

---

## Requirements

### 1. 核心治理规范 (Core Living Governance Requirements)

#### Requirement: 领域主干能力规范集合管理 [FR-GOV-01] <a id="req-gov-consolidation-integrity"></a>

系统 SHALL 在 `openspec/specs/` 目录下集中维护持续有效的领域主干能力规范，包括 `inventory`、`policy`、`probe`、`publication`、`auth`、`ui` 规范体系，并适配 RFC 2119 规范语法与 RFC 场景格式。

##### Scenario: 自动化严格语法校验 <a id="scenario-gov-consolidation-integrity-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 运行 openspec validate --specs --strict 命令
- **THEN** 所有主干能力规范均通过校验且退出码为 0，零语法或格式警告

#### Requirement: 证据来源分级与用户原话确权 [FR-GOV-02] <a id="req-gov-user-intent-attribution"></a>

系统 SHALL 建立严格的四级证据来源分级体系（TIER-1 用户明确原话、TIER-2 历史变更契约、TIER-3 实机实现事实、TIER-4 开发者经验），在规范编制中严格区分客观事实与推测分析。

##### Scenario: 甄别变更提案分析 <a id="scenario-gov-user-intent-attribution-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 变更提案 proposal.md 中存在问题分析描述
- **THEN** 注册表将其准确归类为 TIER-2 历史契约分析，严禁冒充 TIER-1 用户原话

#### Requirement: 历史变更追溯与双层规范契约映射 [FR-GOV-03] <a id="req-gov-traceability-mapping"></a>

系统 SHALL 对历史积累的 26 个既有变更集和 63 份分项规范建立领域级追溯映射矩阵，明确历史规范的主干合流领域与尚未消解需求存根，确立『主干规范 + 历史存根』双层契约，确保历史契约不被无意删除。

##### Scenario: 追溯历史分流规则规范 <a id="scenario-gov-traceability-mapping-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 检索历史变更 csp-fresh-inventory-full-validation 中的 policy/rules-validation 规范
- **THEN** 追溯矩阵明确指出其合流至 policy 领域的 FR-POL-05 与 publication 领域的 FR-PUB-01，未合并细分条款保留为历史存根

#### Requirement: 结构化业务未决问题收敛 [FR-GOV-04] <a id="req-gov-conflict-convergence"></a>

对于实机实现中存在的语义断层与未决需求，系统 SHALL 收敛形成不超过 6 组的业务问题清单，提供清晰的分项业务选项，且严禁施工机自行预设默认值。

##### Scenario: 呈现策略组解包未决选项 <a id="scenario-gov-conflict-convergence-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 检视 clarification-questions.md 中的策略组解包问题
- **THEN** 清单清晰列出去重规则、递归深度与过滤传播的具体选项供用户决策

---

## Supplemental Retained Behavioral Capabilities & Historical Clauses

### Retained Governance Clauses

#### Requirement: Authenticated bounded semantic acceptance [HR-209] <a id="req-gov-hr-209"></a>

生产验收 SHALL 仅使用合法受管且匹配现有身份的凭据和已有订阅/节点；SHALL 验证完整正文语义而非仅HTTP状态。预算 SHALL 在审查前明确，失败刷新 SHALL 保留旧节点身份、active状态及来源关系，不以计数相等代替集合保持。

##### Scenario: Subscription is unreachable <a id="scenario-gov-hr-209-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 有界刷新返回fetch/parse失败、顶层错误或传输超时
- **THEN** 工具核实服务器现态及失败前后库存集合，未能核实则报告未知/失败，不假定库存保留或写PASS

##### Scenario: Real node probe <a id="scenario-gov-hr-209-2"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 使用审查过的代表性既有active节点执行baseline探针
- **THEN** 请求使用真实node_logical_ids/kinds/deadline协议，并读取真实run终态与独立observations；证据绑定node、run、当前connection_revision及时间，记录实际握手阶段、可达性/失败与延迟，不声称全部节点必须在线

##### Scenario: Inventory spans multiple pages <a id="scenario-gov-hr-209-3"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 节点或订阅清单超过首页
- **THEN** 工具依照实际分页协议获取有界完整清单并验证总数/重复，分别报告全量与页内数字，不将首页无active判为成功零探针

#### Requirement: Fail-closed acceptance reports [HR-210] <a id="req-gov-hr-210"></a>

工具 SHALL 汇聚全部必选HTTP、JSON/schema、正文、库存、刷新及探针证据；任何必选断言失败、结果缺失、预算到期或报告保存失败 SHALL 禁止PASS。工具 SHALL 使用Go标准测试及既有e2e fixture覆盖真实CLI入口的正负例，而非只测服务API。

##### Scenario: An assertion fails but the process continues <a id="scenario-gov-hr-210-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 任一必选正文断言失败、上游错误或创建探针后无合法观察
- **THEN** 最终结论不是PASS，退出与结构化报告明确错误/未完成原因，不由后续成功断言覆盖失败

##### Scenario: Private scratch and redacted persistence <a id="scenario-gov-hr-210-2"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** runner构建验收工具或持久化证据
- **THEN** 使用受管私有scratch或唯一mktemp目录，目录0700/报告0600并可靠清理，不复用固定公共/tmp binary；输出只含脱敏摘要，写入错误必须显式失败

#### Requirement: Evidence-qualified restoration [HR-211] <a id="req-gov-hr-211"></a>

历史节点恢复 SHALL 要求充分因果证据、唯一身份、当前有效来源且排除手动禁用/删除。歧义候选 SHALL 保持现态并逐类报告；历史启发式数字或0白名单 SHALL 不等于恢复需求全部完成。

##### Scenario: Historical heuristic candidate <a id="scenario-gov-hr-211-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 节点仅符合旧失败刷新启发式或历史数量
- **THEN** 缺因果及用户意图证据时不恢复，并保留明确待证明类别

#### Requirement: Reviewed production write safety [HR-212] <a id="req-gov-hr-212"></a>

生产写入 SHALL 遵循独立审查、可恢复的fresh一致性热备及精确precondition/allowlist；app更新 SHALL 保持已有卷、端口、代理和DB身份配置，只涉及授权目标服务。回滚 SHALL 绑定替换前实际image ID，不使用不相关历史旧镜像。

##### Scenario: Stale recovery plan <a id="scenario-gov-hr-212-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 现态身份或来源与审查白名单不符
- **THEN** 事务恢复中止，不批量放活

##### Scenario: Controlled application release <a id="scenario-gov-hr-212-2"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 审查和隔离自测通过后执行授权app替换
- **THEN** 留存源码/镜像manifest，在线备份DB及身份库存并验证隔离恢复，仅替换app，保持csp-v1-data与既有loopback双端口/代理/鉴权

##### Scenario: Rollback is required <a id="scenario-gov-hr-212-3"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 发布造成持续不可用、库存有损变化或身份/安全退化
- **THEN** 使用捕获的当前镜像进行no-build目标回滚；仅有损DB写需要时在停止目标写者后恢复验证过的快照，隔离旧WAL/SHM并保留失败现场，不以正常镜像回滚无条件覆盖DB

#### Requirement: External prerequisites stay explicit [HR-213] <a id="req-gov-hr-213"></a>

缺合法凭据、真实握手成功证据或因果历史 SHALL 以限定PARTIAL/BLOCKED及可操作安全决定报告，不绕过或伪装为完成；本地可闭环整改 SHALL 继续，未执行业务任务 SHALL 保持unchecked。

##### Scenario: No legal credential available <a id="scenario-gov-hr-213-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 已存在且明确受管来源无匹配生产凭据
- **THEN** 不调用受限接口、不更改verifier、不聊天索要秘密；由持有人安全投递现有凭据到受控env/0600文件，真实验收仍未完成

#### Requirement: Native current-run completion provenance [HR-214] <a id="req-gov-hr-214"></a>

本包 SHALL 使用真实本轮原生dispatch合同和独立子会话报告，明确run/change/package/epoch及失败复验关联。历史无合同运行 SHALL 保持原记录，当前或后续合同 SHALL 不倒灌历史。规划READY SHALL 不代表实现、部署或业务PASS。

##### Scenario: Historical run has no completion contract <a id="scenario-gov-hr-214-1"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 旧run为EXITED/UNVERIFIED且缺completion_contract
- **THEN** 保留旧记录，新run由原生CLI冻结合同并分别核验实际全部门禁，不补旧合同洗绿

##### Scenario: Final evidence is incomplete <a id="scenario-gov-hr-214-2"></a>
- **GIVEN** 规范治理与变更证据链处于审计状态
- **WHEN** 代码自测/审查通过但发布或带身份实网证据未完成
- **THEN** 本包不能宣告业务PASS，也不能以公共健康、fixture或无凭据退出0抹除未完成任务

---

## Success Criteria

### Measurable Outcomes
- **[SC-GOV-01]** 规范条目双向追溯覆盖率达到 100%。
- **[SC-GOV-02]** 来源分级违规与越权假结单拦截率达到 100%。
- **[SC-GOV-03]** 未决问题与显式冲突收敛归档闭环率达到 100%。

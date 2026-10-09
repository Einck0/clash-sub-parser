## ADDED Requirements

### Requirement: Authenticated bounded semantic acceptance

生产验收 SHALL 仅使用合法受管且匹配现有身份的凭据和已有订阅/节点；SHALL 验证完整正文语义而非仅HTTP状态。预算 SHALL 在审查前明确，失败刷新 SHALL 保留旧节点身份、active状态及来源关系，不以计数相等代替集合保持。

#### Scenario: Subscription is unreachable
- **WHEN** 有界刷新返回fetch/parse失败、顶层错误或传输超时
- **THEN** 工具核实服务器现态及失败前后库存集合，未能核实则报告未知/失败，不假定库存保留或写PASS

#### Scenario: Real node probe
- **WHEN** 使用审查过的代表性既有active节点执行baseline探针
- **THEN** 请求使用真实node_logical_ids/kinds/deadline协议，并读取真实run终态与独立observations；证据绑定node、run、当前connection_revision及时间，记录实际握手阶段、可达性/失败与延迟，不声称全部节点必须在线

#### Scenario: Inventory spans multiple pages
- **WHEN** 节点或订阅清单超过首页
- **THEN** 工具依照实际分页协议获取有界完整清单并验证总数/重复，分别报告全量与页内数字，不将首页无active判为成功零探针

### Requirement: Fail-closed acceptance reports

工具 SHALL 汇聚全部必选HTTP、JSON/schema、正文、库存、刷新及探针证据；任何必选断言失败、结果缺失、预算到期或报告保存失败 SHALL 禁止PASS。工具 SHALL 使用Go标准测试及既有e2e fixture覆盖真实CLI入口的正负例，而非只测服务API。

#### Scenario: An assertion fails but the process continues
- **WHEN** 任一必选正文断言失败、上游错误或创建探针后无合法观察
- **THEN** 最终结论不是PASS，退出与结构化报告明确错误/未完成原因，不由后续成功断言覆盖失败

#### Scenario: Private scratch and redacted persistence
- **WHEN** runner构建验收工具或持久化证据
- **THEN** 使用受管私有scratch或唯一mktemp目录，目录0700/报告0600并可靠清理，不复用固定公共/tmp binary；输出只含脱敏摘要，写入错误必须显式失败

### Requirement: Evidence-qualified restoration

历史节点恢复 SHALL 要求充分因果证据、唯一身份、当前有效来源且排除手动禁用/删除。歧义候选 SHALL 保持现态并逐类报告；历史启发式数字或0白名单 SHALL 不等于恢复需求全部完成。

#### Scenario: Historical heuristic candidate
- **WHEN** 节点仅符合旧失败刷新启发式或历史数量
- **THEN** 缺因果及用户意图证据时不恢复，并保留明确待证明类别

### Requirement: Reviewed production write safety

生产写入 SHALL 遵循独立审查、可恢复的fresh一致性热备及精确precondition/allowlist；app更新 SHALL 保持已有卷、端口、代理和DB身份配置，只涉及授权目标服务。回滚 SHALL 绑定替换前实际image ID，不使用不相关历史旧镜像。

#### Scenario: Stale recovery plan
- **WHEN** 现态身份或来源与审查白名单不符
- **THEN** 事务恢复中止，不批量放活

#### Scenario: Controlled application release
- **WHEN** 审查和隔离自测通过后执行授权app替换
- **THEN** 留存源码/镜像manifest，在线备份DB及身份库存并验证隔离恢复，仅替换app，保持csp-v1-data与既有loopback双端口/代理/鉴权

#### Scenario: Rollback is required
- **WHEN** 发布造成持续不可用、库存有损变化或身份/安全退化
- **THEN** 使用捕获的当前镜像进行no-build目标回滚；仅有损DB写需要时在停止目标写者后恢复验证过的快照，隔离旧WAL/SHM并保留失败现场，不以正常镜像回滚无条件覆盖DB

### Requirement: External prerequisites stay explicit

缺合法凭据、真实握手成功证据或因果历史 SHALL 以限定PARTIAL/BLOCKED及可操作安全决定报告，不绕过或伪装为完成；本地可闭环整改 SHALL 继续，未执行业务任务 SHALL 保持unchecked。

#### Scenario: No legal credential available
- **WHEN** 已存在且明确受管来源无匹配生产凭据
- **THEN** 不调用受限接口、不更改verifier、不聊天索要秘密；由持有人安全投递现有凭据到受控env/0600文件，真实验收仍未完成

### Requirement: Native current-run completion provenance

本包 SHALL 使用真实本轮原生dispatch合同和独立子会话报告，明确run/change/package/epoch及失败复验关联。历史无合同运行 SHALL 保持原记录，当前或后续合同 SHALL 不倒灌历史。规划READY SHALL 不代表实现、部署或业务PASS。

#### Scenario: Historical run has no completion contract
- **WHEN** 旧run为EXITED/UNVERIFIED且缺completion_contract
- **THEN** 保留旧记录，新run由原生CLI冻结合同并分别核验实际全部门禁，不补旧合同洗绿

#### Scenario: Final evidence is incomplete
- **WHEN** 代码自测/审查通过但发布或带身份实网证据未完成
- **THEN** 本包不能宣告业务PASS，也不能以公共健康、fixture或无凭据退出0抹除未完成任务

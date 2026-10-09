## Purpose

规范 CSP 分流规则与策略组的只读动态校验、显式空组 PASS、过滤语义保真与前端规则页防竞态交互。历史规则管理已完成，本续作只补新增配置与验证缺口，不重复删除既有规则。

## ADDED Requirements

### Requirement: 只读分流规则与动态拓扑校验 API
系统 SHALL 在 `POST /api/v1/policies/validate`（以及 `GET`）上校验当前已保存 active revision，保持 `valid`、`errors`、`revision_id` 与结构化 issues 兼容。系统 MUST 使用与 publication 一致的启用源节点范围、filters、NodeSources、LatestObservations、AsOf 和风险上下文，纯内存解析，不触发网络、探针执行或数据库写入。依赖仓储错误 MUST 返回明确 incomplete/错误，不得伪造空库存或 valid。

#### Scenario: 同一活跃版本返回结构化诊断
- **WHEN** 客户端请求当前 active revision 的 validate
- **THEN** 返回对应 revision_id、valid、errors 和包含规则定位信息的 issues，使用与发布相同的来源/探针过滤输入，且无网络与写入副作用

#### Scenario: 仓储依赖读取失败
- **WHEN** rules、nodes、filters、sources 或 observations 读取失败
- **THEN** 返回明确不完整/错误诊断，不把失败当作空数组或校验成功

### Requirement: 默认严格与显式空组 PASS
系统 SHALL 增加 `empty_fallback_pass` 布尔配置，SQLite 历史行和创建默认 false；domain、repository、Create/Get/List/Update API、UI 与 resolver/compiler/publication MUST 一致保真，PATCH omission 保持旧值而显式 false 关闭。系统 MUST 仅为该 flag=true 且静态解析为空的配置组允许 PASS；未勾选空组继续 required_nonempty，直接被规则引用的未许可空组返回 error/empty_routed_group，未直接引用的普通空组保持既有 empty_group warning 与编译安全策略。

#### Scenario: 旧数据库迁移和 API 三态
- **WHEN** 旧库升级且客户端创建或 PATCH 一个策略组
- **THEN** 历史与新建默认 false，Get/List/回包返回真实值，PATCH 未提供保持原值、false 关闭、true 显式开启，失败校验不留下部分修改或新 revision

#### Scenario: 未勾选的被路由空组
- **WHEN** 分流规则直接引用静态无有效普通成员或无递归真实节点的组，且该组 flag=false
- **THEN** valid=false，返回 error/empty_routed_group 及 rule_id、position、type、value、target_group_id/name，编译/发布仍以 required_nonempty 阻断

#### Scenario: 勾选空组与非空组区别
- **WHEN** flag=true 的配置组为空或非空
- **THEN** 只有为空的组获得有效 PASS 语义；非空组不得附加 PASS 成员或运行时 PASS fallback，实际节点 ID 集合和计数均不变

### Requirement: PASS 继续规则匹配且无隐藏直连
Mihomo 编译器 SHALL 为明确配置且有效为空的组输出 `proxies: [PASS]` 和原生 `empty-fallback: PASS`，MUST 通过官方 Mihomo 离线规则匹配 fixture 证明其继续后续规则而非 DIRECT。该特性 MUST NOT 通过 DIRECT 或缺省 COMPATIBLE 的 Direct fallback 隐藏绕行。非空勾选组不得被扩大为运行时探针失败后自动 PASS；如固定其 native empty-fallback，MUST 使用原生拒绝而非 DIRECT/COMPATIBLE。不支持 PASS 路由的目标 MUST 明确 capability 诊断，保持原 nodes-only 行为，不伪造映射。

#### Scenario: 第一条命中 PASS 后匹配后续控制规则
- **WHEN** 无网络内核 fixture 第一条规则命中有效 PASS 空组，下一控制规则为非 DIRECT 终点
- **THEN** 实际规则执行落在后续控制规则，而不是提前 Direct 出口，仅有 YAML 字符串不构成语义证明

#### Scenario: 必需 fallback 分支与兼容模式发布
- **WHEN** 明确配置的空组被必需分支或规则引用并参与正常/compatible 发布
- **THEN** resolver、policy 校验、capability、renderer、preflight 和冻结内容校验一致认可有效 PASS，不被 optional prune 删除，不通过 hidden COMPATIBLE/Direct 绕过

### Requirement: PASS 不伪造节点或放宽独立守卫
系统 SHALL 分离配置 flag 与有效 PASS 状态，将其纳入 input/snapshot digest。父组及 derived/filter projection MUST NOT 自动继承子组或原组许可，父组仅含 PASS 子组而无真实节点时仍按既有空定义处理。PASS MUST NOT 成为伪造 NodeLogicalID，不豁免循环、悬挂引用、risk、来源/探针条件、能力或全局过滤库存守卫，不覆盖生产用户 flags。

#### Scenario: 子组许可不自动传播
- **WHEN** 某子组获 PASS 许可但父组未配置，或出现过滤后为空的 derived projection
- **THEN** 父组/投影不会因许可自动转为 PASS；仍执行真实空组/过滤诊断和拓扑安全校验

#### Scenario: 重复编译与切换配置
- **WHEN** 相同输入重复编译或单独切换 empty_fallback_pass
- **THEN** 相同输入 digest/输出确定，切换配置改变语义 digest，历史 publication bytes 与真实节点数不受改写

### Requirement: 有界 PCRE 兼容与过滤恢复保真
系统 SHALL 优先复用 RE2，并对已知 RE2 不兼容模式使用现有 regexp2 依赖的有限正 MatchTimeout 和有界 pattern/input/cache。Validate 与 Match MUST 使用一致引擎选择与诊断，not_regex MUST NOT 将超时/错误取反成匹配通过。缺失“便宜”过滤器恢复 MUST 基于稳定归档身份、保持原表达式负向 lookahead 与已知 OR 语义、事务幂等且不覆盖用户非空配置，不能凭名称猜排除或删断言。

#### Scenario: 原表达式黄金匹配保真
- **WHEN** 对原正则与 x5、x5.9、x6、x50、x5.9.1、literal 前缀、Eeox/einck、大小写及中文黄金语料校验匹配
- **THEN** Validate 与 Match 结果一致且符合原完整表达式语义，自建 fixture 不依赖私有机器数据库或被跳过的恢复测试

#### Scenario: 回溯超时与已有配置
- **WHEN** 输入触发 bounded timeout 或恢复目标已有用户 filter
- **THEN** 超时报错误且 not_regex 不放行；已有配置不被覆盖，重复恢复不新增虚假修改

### Requirement: 前端规则管理、正则编辑与刷新校验
前端 SHALL 保留既有规则 Tab、warnings、规则定位、手动校验和 AbortController/seq/revision 守卫，增加默认未勾选的空组 PASS checkbox 和 regex/not_regex 创建编辑支持。UI MUST 明确 PASS 继续规则匹配不是直连；初始/保存/reload/retry 成功后基于一致最新 revision 自动 validate，失败加载/保存不得伪造通过，晚响应不得覆盖新状态。

#### Scenario: 保存重载与正则编辑
- **WHEN** 用户创建/编辑 group、checkbox 或 regex/not_regex 并保存后重载
- **THEN** UI 展示持久化真实值与原正则，错误不产生半保存，文案明确 PASS 非 DIRECT

#### Scenario: 重试加载与晚校验响应
- **WHEN** 用户点击错误卡 retry 或 reload，旧 validate 响应晚到
- **THEN** 成功加载后对最新 revision 触发 validate，旧响应被丢弃，失败保留真实错误与每条规则 issues

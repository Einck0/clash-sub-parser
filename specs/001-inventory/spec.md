---
feature: 001-inventory
title: Inventory Capability Specification
status: candidate
schema: speckit-native
---

# Inventory Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的订阅输入、节点规范化传输身份、写时复制（CoW）连接参数覆写、多数据源版本台账以及作用域查询生命周期。系统必须保证节点在跨订阅刷新时拥有全局稳定的逻辑身份，在抓取异常或返回零有效节点时保全既有资产，并对上层策略与发布模块提供确定、高效的服务端数据读取契约。

## User Scenarios & Priority

### User Story 1 - 订阅导入与稳定资产管理 (Priority: P1)
- **GIVEN** 管理员配置了一个包含多个节点的远程订阅 URL
- **WHEN** 触发手动刷新或周期调度拉取成功
- **THEN** 系统解析并规范化各协议节点，按传输哈希分配稳定 logical_id，并记录来源归属版本

### User Story 2 - 抓取异常时的资产零丢失保全 (Priority: P1)
- **GIVEN** 数据库中存在由订阅 A 导入的 100 个存量有效节点
- **WHEN** 订阅 A 发生网络超时、证书错误或返回 0 个有效节点
- **THEN** 系统记录刷新失败审计日志，完全保全这 100 个存量节点与来源关联，禁止执行物理清空

### User Story 3 - 人工连接覆写与写时复制保护 (Priority: P2)
- **GIVEN** 节点已被导入且存在默认连接参数（如端口、UUID、伪装域名）
- **WHEN** 管理员在控制台详情面板手动修改连接参数并保存
- **THEN** 系统在独立 overrides 表中持久化修改，后续该订阅重新拉取时人工覆写参数优先保留

---

## Requirements

### 1. 订阅输入与生命周期管理

#### Requirement: 受控订阅生命周期与非敏感存储 [FR-INV-01] <a id="req-inv-lifecycle"></a>
系统 SHALL 允许授权管理者创建、修改、启用、停用和手动刷新订阅。每个订阅 MUST 包含稳定逻辑 ID、显示名称、来源 URL、可选抓取代理、User-Agent 策略与刷新间隔；敏感凭据与 Token 不得暴露在公开只读接口。更新订阅时对空白凭据引用进行严格校验。

##### Scenario: 禁用订阅停止调度 <a id="scenario-inv-disable-schedule"></a>
- **GIVEN** 某个订阅处于活动调度状态
- **WHEN** 管理员将其设置为禁用状态 (`active = false`)
- **THEN** 周期调度器立即停止该订阅的自动抓取作业，手动抓取时给出明确警告

##### Scenario: 订阅 PATCH 空白凭据引用严格校验 <a id="scenario-inv-patch-whitespace"></a>
- **GIVEN** 已经存在的订阅记录
- **WHEN** 客户端向 `PATCH /api/v1/subscriptions/{id}` 提交纯空格构成的凭据引用
- **THEN** 服务端拒绝更新并返回 400 Bad Request，不得将空白引用持久化为有效外键

#### Requirement: 抓取失败与零有效节点资产保全 [FR-INV-02] <a id="req-inv-zero-loss"></a>
当远程订阅抓取发生网络错误、HTTP 非 200 响应、解析异常或返回有效节点数为 0 时，系统 MUST 终止写入事务并记录失败日志，严禁删除先前已存在的有效节点或来源关联。

##### Scenario: 远端返回 500 时存量节点保全 <a id="scenario-inv-500-zero-loss"></a>
- **GIVEN** 数据库中包含由订阅 A 导入的 50 个有效节点
- **WHEN** 订阅 A 在拉取时上游服务器返回 HTTP 500 错误
- **THEN** 写入事务中断，存量 50 个节点及其连接参数在数据库中完好保留，节点损耗率为 0%

##### Scenario: 远端返回 0 节点时存量资产保全 <a id="scenario-inv-zero-nodes-preserve"></a>
- **GIVEN** 数据库中包含由订阅 B 导入的存量节点
- **WHEN** 订阅 B 拉取返回内容为空或解析后得到 0 个有效节点
- **THEN** 系统记录解析警告审计日志，现有节点资产与关联保持不变，不执行级联清空

#### Requirement: 订阅刷新数据修剪与重试策略 [FR-INV-07] [NEEDS CLARIFICATION] <a id="req-inv-prune-retry"></a>
在订阅成功抓取后，系统 SHALL 对未在新快照中出现的来源执行修剪。关于完全成功（Success）与部分成功（Partial）场景下新节点缺失时的来源修剪与隐藏作用域保护策略，系统 SHALL 依据用户在 Q5 中的决策分别落实。

##### Scenario: 抓取部分成功时的修剪边界 <a id="scenario-inv-partial-prune"></a>
- **GIVEN** 某个订阅在刷新过程中部分节点解析失败
- **WHEN** 解析器成功提取了部分新节点
- **THEN** 系统根据既定策略执行来源修剪或保守保留历史关联，并记录审计差异

---

### 2. 规范化节点传输身份与覆写

#### Requirement: 规范化传输身份与稳定 Logical ID [FR-INV-03] <a id="req-inv-logical-id"></a>
系统 SHALL 支持 Shadowsocks、VMess、VLESS、Trojan、Hysteria2、TUIC、WireGuard 协议，并根据协议、服务器、端口及核心传输特征计算规范化哈希，赋予全局稳定的 `logical_id`。重复节点 MUST 合并存储。支持节点列表与观测记录的分页查询与真实计数。

##### Scenario: 跨刷新节点身份恒定 <a id="scenario-inv-hash-stability"></a>
- **GIVEN** 已导入的节点集合
- **WHEN** 节点在后续订阅刷新中名称发生变更但核心传输参数未变
- **THEN** 该节点复用原有的 `logical_id`，策略组的绑定关系不发生脱保

##### Scenario: 节点列表与观测记录真实分页与计数 <a id="scenario-inv-pagination"></a>
- **GIVEN** 数据库中包含超过单页限制的节点观测记录
- **WHEN** 客户端请求分页查询并带 `page` 与 `page_size`
- **THEN** 服务端返回当前页节点数据，同时返回准确的 `total` 真实总数计数，不施加硬截断

#### Requirement: 多来源关联与历史版本台账 [FR-INV-04] <a id="req-inv-multi-source"></a>
系统 SHALL 支持同一个规范化节点同时关联至多个订阅源，在 `node_sources` 中维护订阅与节点的关联关系，并在 `node_source_history` 中记录抓取版本快照与变更历史。删除唯一拥有节点的订阅时，级联将孤儿节点设置为去活状态。

##### Scenario: 节点同时出现在两个订阅 <a id="scenario-inv-multi-src-map"></a>
- **GIVEN** 订阅 A 与订阅 B
- **WHEN** 订阅 A 与订阅 B 包含相同传输参数的节点
- **THEN** 节点台账仅生成一条节点记录，并在来源关联表中记录两个关联关系

##### Scenario: 订阅源删除级联去活孤儿节点 <a id="scenario-inv-cascade-orphan"></a>
- **GIVEN** 某个节点仅由订阅 A 独家提供
- **WHEN** 管理员执行删除订阅 A
- **THEN** 系统安全删除订阅与关联关系，并将该孤儿节点状态置为 inactive，避免残留悬空

#### Requirement: 启用订阅作用域隔离查询 [FR-INV-05] <a id="req-inv-scoped-query"></a>
系统 SHALL 提供针对活动订阅的隔离作用域（`enabled_subscriptions`）。当某个订阅被禁用时，仅属于该订阅的节点 MUST 自动从活动查询、策略组动态全池和发布导出中屏蔽。

##### Scenario: 禁用订阅隔离节点可见性 <a id="scenario-inv-disable-isolate"></a>
- **GIVEN** 订阅 A 被设置为禁用状态
- **WHEN** 客户端执行启用作用域查询或客户端导出
- **THEN** 仅由订阅 A 提供的专有节点在查询结果中被安全过滤，不进入导出候选集

#### Requirement: 节点连接参数写时复制与人工覆写 [FR-INV-06] <a id="req-inv-cow-overrides"></a>
系统 SHALL 支持针对节点属性的独立人工覆写（Overrides）。当用户修改节点连接属性时，系统 MUST 将差异持久化至覆写事实层，并在数据组装时按高优先级合并应用。

##### Scenario: 刷新不冲掉人工覆写 <a id="scenario-inv-cow-persist"></a>
- **GIVEN** 用户手动将节点的连接端口覆写为 443
- **WHEN** 上游订阅重新拉取且下发端口为 8080
- **THEN** 节点组装结果中端口依然保持为 443，人工覆写参数守恒率为 100%

---

---

---

---

## Supplemental Retained Behavioral Capabilities & Historical Clauses

### Overview
本节完整收录历史变更中属于本领域的行为规范、具体契约（包含精确状态码、传输参数、请求头校验等）与验收场景，保持行为约束无损物理留存与点对点双向追溯。

### Retained Functional Requirements

#### Requirement: 订阅源删除级联去活孤儿节点 [HR-008] <a id="req-inv-hr-008"></a>

当客户端调用 `DELETE /api/v1/subscriptions/{id}` 删除订阅源时，系统在清除 `subscriptions` 及 `node_sources` 关联记录后，MUST 原子化地将所有失去全部来源关联且仍处于活跃状态的节点标记为停用（`active = 0`），确保节点台账（Node Ledger）中不会遗留不受任何订阅源管辖的僵尸活跃节点。

##### Scenario: 删除唯一拥有某批节点的订阅源 <a id="scenario-inv-hr-008-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅源 A 独占拥有节点 X 与节点 Y，客户端调用删除接口删除订阅源 A
- **THEN** 订阅源 A 被删除，节点 X 与 Y 的 `active` 状态自动变更为 `0`（非活跃），不再出现在默认活跃节点列表与探测候选集中

#### Requirement: 订阅源 PATCH 空白凭据引用严格校验 [HR-009] <a id="req-inv-hr-009"></a>

当客户端向 `PATCH /api/v1/subscriptions/{id}` 提交包含空白字符串或纯空白字符的 `source_url_secret_ref` 字段时，传输层 MUST 将该字段原样传递给业务应用层，业务应用层 MUST 判定其为非法输入，并返回 422 Unprocessable Entity 错误（错误码 `invalid_source_url_secret_ref`），SHALL NOT 静默忽略。

##### Scenario: 提交纯空格凭据引用进行更新 <a id="scenario-inv-hr-009-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 客户端向订阅源 PATCH 请求体中传入 `{"source_url_secret_ref": "   "}`
- **THEN** 服务端返回 422 状态码与 `invalid_source_url_secret_ref` 错误响应，拒绝更新

#### Requirement: 新 schema 与单一数据权威 [HR-034] <a id="req-inv-hr-034"></a>

系统 SHALL 使用由 CSP 1.0 自身管理的独立 SQLite 文件与版本化 schema。核心实体 MUST 至少包括 subscriptions、nodes、node_sources、probe_runs、probe_observations、node_groups、group_memberships、admission_rules、policy_rules、configuration_revisions、publications、settings 和 audit_events；所有跨实体引用 MUST 使用稳定逻辑 ID 或受外键约束的内部键，且运行时不得读取旧数据库表。

##### Scenario: 新运行时启动 <a id="scenario-inv-hr-034-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 新 CSP 1.0 以空的新数据卷启动
- **THEN** 系统创建或验证 CSP 1.0 schema 且不要求旧数据库文件存在或可访问

#### Requirement: 导入秘密排除与隔离报告 [HR-036] <a id="req-inv-hr-036"></a>

导入器 MUST 排除订阅 URL 中凭据、导出令牌、代理凭据、私钥、cookie、完整历史探测响应和未知 JSON 载荷。遇到无法按 allowlist 映射的字段或不合规记录时，导入器 MUST 将其放入隔离报告并继续处理其他独立记录；报告只包含字段类别、历史主键摘要和脱敏错误，不含秘密值。

##### Scenario: 历史记录含私钥 <a id="scenario-inv-hr-036-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 历史节点记录包含私钥字段
- **THEN** 私钥不写入新数据库或报告，记录以需要审查或隔离状态出现

#### Requirement: 备份、恢复、切流和回滚 [HR-037] <a id="req-inv-hr-037"></a>

系统 SHALL 提供一致性备份与恢复验证步骤，备份后 MUST 验证可打开性和外键完整性。新服务 MUST 使用独立数据卷；生产切流 MUST 由人工批准，先验证新服务健康和发布物，再修改流量。回滚 MUST 仅恢复旧的已停止容器与原历史卷或恢复一份已验证的新备份，且不得通过双写或在线 schema 适配实现。

##### Scenario: 新服务健康检查失败 <a id="scenario-inv-hr-037-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 发布前新服务的 schema、健康端点或只读验证失败
- **THEN** 不执行流量切换，原历史卷保持未写入状态

#### Requirement: 安全受限的获取与解析 [HR-051] <a id="req-inv-hr-051"></a>

系统 MUST 仅允许明确批准的 URL scheme，并在连接前和每次重定向后执行 DNS 解析与目标地址策略校验。系统 SHALL 执行超时、最大响应字节数、最大重定向次数与内容解析限制；失败 MUST 产生脱敏作业错误而不覆盖先前有效节点集。

##### Scenario: 重定向到被禁止的私有地址 <a id="scenario-inv-hr-051-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 已批准的订阅 URL 重定向到策略禁止的地址
- **THEN** 刷新失败并记录原因，且不向该目标发送请求

#### Requirement: 规范化节点与稳定逻辑身份 [HR-052] <a id="req-inv-hr-052"></a>

系统 SHALL 将支持协议解析成一个规范化节点表示，并为同一规范化传输身份计算稳定 `logical_id`。节点的数据库主键 MUST 不作为跨资源引用或导出名称依据。重复节点 MUST 合并来源关联；刷新完成后，已失去全部来源关联的节点 MUST 按事务性差集规则移除或标记过期。

##### Scenario: 同节点来自两个订阅 <a id="scenario-inv-hr-052-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 两个有效订阅解析出具有相同规范化传输身份的节点
- **THEN** 台账仅包含一个 `logical_id` 节点并记录两个来源关联

##### Scenario: 节点从最后来源消失 <a id="scenario-inv-hr-052-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 节点不再出现在其最后一个已成功刷新的订阅版本中
- **THEN** 刷新事务后该节点不再进入活动台账或导出候选集

#### Requirement: 服务端分页台账 [HR-053] <a id="req-inv-hr-053"></a>

系统 SHALL 提供节点列表读取，接受 `page`、`page_size`、固定排序和已声明的协议、国家、连通性、延迟与能力筛选器。响应 MUST 返回 `items`、`page`、`page_size`、`total` 与用于详情读取的节点 `logical_id`；服务端 MUST 在筛选后分页。

##### Scenario: 万级节点的筛选分页 <a id="scenario-inv-hr-053-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 调用者以有效筛选条件请求某一页节点
- **THEN** 返回项数不超过 `page_size`，`total` 表示同一筛选集合总数，且客户端无需下载完整台账

#### Requirement: 不可变发布快照与已发布 payload 保护 [HR-072] <a id="req-inv-hr-072"></a>

系统 SHALL 严格保护既有不可变发布快照（`publications`）与发布所引用的原始载荷（`publication_payload_refs`）。在重置过程中，系统 MUST 保留已被 `publication_payload_refs` 引用的 `subscription_payloads` 记录及其原始内容；对于这些已发布载荷下的 `subscription_entries`，系统 MUST 将其关联的 `node_logical_id` 脱钩设置为 NULL，确保不持有指向已删除节点的悬挂外键。对于未被任何发布引用的旧原始载荷与条目，系统 MUST 予以物理删除。

##### Scenario: 存在已发布载荷时保留载荷内容并解绑节点外键 <a id="scenario-inv-hr-072-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 数据库包含已被 publications 引用锁定的 subscription_payloads 记录并触发节点重置
- **THEN** 系统保留该 payload 记录，将其下 subscription_entries 的 node_logical_id 设置为 NULL，且外键校验无违规

##### Scenario: 未被发布的历史载荷与条目彻底清除 <a id="scenario-inv-hr-072-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 存在未关联任何 publication 的旧 subscription_payloads 和 subscription_entries
- **THEN** 重置事务将其全量删除，不留无用旧载荷数据

#### Requirement: 断开旧抓取记录成员指针与历史审计事实保持 [HR-073] <a id="req-inv-hr-073"></a>

系统 SHALL 在重置事务中清理未绑定发布的无用旧抓取记录，并通过清空 `node_sources` 表彻底切断旧节点集合与上游抓取的成员关联指针。对于留存的不可变抓取记录（`subscription_fetches`），系统 MUST 严格保持其不可变的历史审计事实（`nodes_parsed` 与 `nodes_valid` 原始计数保持不变，严禁清零伪造审计）。当重置后后续抓取失败时，系统 MUST NOT 回退或重新激活任何旧节点库存。重置操作 MUST 具备幂等性，连续多次执行必须均能安全完成且结果一致。

##### Scenario: 留存抓取记录保持原始审计计数不变 <a id="scenario-inv-hr-073-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 存在与已发布载荷关联的留存 subscription_fetches 记录并触发节点重置
- **THEN** 该抓取记录的 nodes_parsed 与 nodes_valid 维持其历史抓取时的真实数值不变

##### Scenario: 连续多次执行重置具备幂等性 <a id="scenario-inv-hr-073-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 在已重置为空的数据库上再次执行节点库存重置
- **THEN** 操作成功完成，相关节点表仍为 0，外键检查无任何错误

#### Requirement: 策略分组节点边解绑与受保护私密重绑计划 [HR-074] <a id="req-inv-hr-074"></a>

系统 SHALL 在重置事务中识别并清理显式指向节点的 `group_edges`。在删除前，系统 MUST 提取所有受影响的节点绑定配置（协议、端点、传输参数），生成严格基于连接身份（`ComputeConnectionLogicalID`）的重绑计划（Rebind Plan）与诊断信息，严禁仅按节点名称或端点模糊自动重绑，亦严禁未经授权自动回退到 DIRECT。在 apply 执行模式下，若存在受影响绑定，系统 MUST 将其持久化至文件系统受保护的私有维护清单（目录权限 0700，文件权限 0600）。

##### Scenario: 显式节点边解绑并生成精确重绑计划 <a id="scenario-inv-hr-074-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 存在指向具体 node_logical_id 的 group_edges 并执行重置
- **THEN** 对应 group_edges 被安全清理避免悬挂外键，报告包含 impacted_bindings_count 与 exact_connection_id 映射计划，并在 apply 时输出 0600 权限的私有维护清单文件路径

##### Scenario: 策略分组与业务配置资产完全保留 <a id="scenario-inv-hr-074-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行节点库存重置
- **THEN** `subscriptions`、`node_groups`、`group_edges`（组间关系）、`policy_rules`、`admission_rules`、`global_node_filters`、`settings` 等业务配置行数与配置保持完全一致

#### Requirement: 维护 CLI 命令参数与安全预检 [HR-075] <a id="req-inv-hr-075"></a>

系统 SHALL 在 `cmd/csp` 提供 `reset-node-inventory`，支持 `--db`、`--dry-run`、`--apply`、`--confirm-backup`、`--backup-file`、`--restore-source-tokens` 及 `--archive-db`。未指定apply或dry-run MUST 报错；apply无有效一致性SQLite backup与确认 MUST 拒绝；所有模式 MUST 前置外键检查，脏外键阻断。本续作 MUST NOT 重复reset/import/Token修复或kliq删除来解决策略/探针问题。

##### Scenario: Dry-run 不修改数据 <a id="scenario-inv-hr-075-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行 `csp reset-node-inventory --db <isolated-path> --dry-run`
- **THEN** 只读输出变更计数与外键结果，数据库内容不变

##### Scenario: Apply 缺备份拒绝 <a id="scenario-inv-hr-075-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** apply无confirm-backup或有效backup-file
- **THEN** 退出1并说明备份确认要求

##### Scenario: Apply 原子报告 <a id="scenario-inv-hr-075-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 历史维护apply具有有效确认/backup
- **THEN** 单事务成功后报告前后计数/FK0，失败完整回滚；此能力不授权本续作再次执行

#### Requirement: 隔离验证与成熟 Wiring [HR-076] <a id="req-inv-hr-076"></a>

CLI SHALL 复用现有SQLite/application、Go testing/httptest、Python unittest/sqlite3、Vitest/Playwright与官方Mihomo，不新建旁路控制面、私有测试框架/探针引擎。验证 MUST 使用独立SQLite/端口，开发build MUST 禁止嵌入资产copy，最终review/critic后才显式COPY_WEBASSETS=1同步；生产用户flags与既有库存 MUST 保持。

##### Scenario: 隔离fixture与产物责任 <a id="scenario-inv-hr-076-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行离线自测或刷新最终preview
- **THEN** 无生产/无关服务变更，无商业源/外部egress，仅loopback fixture；私有0700目录/0600秘密文件，owner/cleanup明确

#### Requirement: 三种真实方法与source/engine身份分开 [HR-077] <a id="req-inv-hr-077"></a>

比较 SHALL 固定真实upstream commit `3c320fd58aff5235e16218c050ec5b8ce587e233`，薄adapter调用真实Check/CreateClient/platform，不复制算法。native Check MUST 使用原生Mihomo v1.19.31与authentic native配置、保持原方法/defaults差异且speed disabled；允许真实baseline-only native配置。same-engine v1.19.32 normalized stage comparator MUST 单独标记，保存native对比modfile/go.sum/传递依赖diff、真实engine/build/source metadata，不硬编码1.19.31。CSP实际runner/profile SHALL 任意2xx normalized alive且受实际body limit，保留原204+empty来源、status/body/typed错误；native Check与normalized stage控制 MUST NOT 混称。

##### Scenario: 原生Check无副作用/无speed <a id="scenario-inv-hr-077-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 离线fixture冻结GlobalProxies、空SubUrls/SubUrlsRemote、KeepDays=0、SuccessLimit=0、空Filter、关闭保存/上传/callback/Web/cron与speed
- **THEN** 真正Check不重复fetch、不测速且保留native方法；baseline-only控制不假称full-media结果

##### Scenario: same-engine pin与实际引擎 <a id="scenario-inv-hr-077-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 构建v1.19.32比较器并运行离线fixture
- **THEN** 真实metadata和source/modfile diff可复验，报告distinct same-engine标签，宿主CLI1.19.30不得冒充任一in-process引擎

##### Scenario: 200非空与204 <a id="scenario-inv-hr-077-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** comparative客户端收到200非空、204空、非2xx或预算截断
- **THEN** 2xx遵循统一normalized成功策略并保留body/status，预算截断单列预算终态，不静默改变native方法或以一次失败宣判永久dead

#### Requirement: ALL90 baseline四侧优先与真实reference并发 [HR-078] <a id="req-inv-hr-078"></a>

新batch SHALL 先运行相同ALL90完整输入的四主侧baseline A1(CSP)→B1(same-engine)→B2(same-engine)→A2(CSP)，不能因前侧失败减少后侧baseline。四侧终态后 SHALL 用四baseline成功且配置可比的交集作为平台共同eligibility，discordant另列解释。actual runner node×side baseline body MUST ≤65536 bytes；alive concurrency8/media2 MUST 实际执行，reference不允许只声明并发但串行循环。新batch speed concurrency MUST 为0、无speed stage/URL/请求。

##### Scenario: 全baseline后公平平台 <a id="scenario-inv-hr-078-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 四侧baseline完成或形成准确取消/未执行终态
- **THEN** 先交付ALL90各侧覆盖，交集上的各节点得到同等各side预算/参与机会；非交集保留差异诊断，不自适应增加网络诊断

##### Scenario: reference并发实际生效 <a id="scenario-inv-hr-078-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** blocking离线fixture检验reference alive/media调度
- **THEN** 能实际并行且分别不超过8/2，queue/cancel/cleanup保真；声明但未使用配置不通过

##### Scenario: 可选两native baseline侧 <a id="scenario-inv-hr-078-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** plan首次冻结前提出native0或2侧baseline-only
- **THEN** 默认0；2仅在同一128MiB/1800s总盘子内且不损害主四侧覆盖的可行性证据成立才冻结，不强制6侧，native结果/eligibility与主比较分开

#### Requirement: 整批跨launch原子响应body与deadline硬限制 [HR-080] <a id="req-inv-hr-080"></a>

全部launch、全部side、成功/失败/redirect/内嵌client与非collector平台调用 SHALL 共享同一个aggregate ledger、原始start/deadline。整批body MUST ≤对应manifest冻结上限（旧批次134217728 bytes、新reissue授权1073741824 bytes）、含cleanup MUST ≤冻结期限（旧批次1800s、新reissue授权7200s）；保持历史旧批次事实与128MiB/1800s记录不变，新限额仅限新manifest；不按每进程/每side重置，不额外512MiB。read前原子reservation、实际settle、未用refund和cancel MUST race-safe；崩溃未结算预留 MUST 保守占额不自动重试。NIC/header/TLS/transport MUST 单独标注，body cap不冒称NIC上限。

##### Scenario: 四侧最坏baseline与平台公平配额 <a id="scenario-inv-hr-080-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 冻结baseline allocation
- **THEN** 旧批次128MiB按最坏22.5MiB/平台105.5MiB；新reissue授权1GiB同样保持baseline四侧全覆盖与四侧交集共同资格逻辑，平台额度四侧均分；逐node×stage公平参与算法先冻结，不能前节点吞全盘/借贷/隐形扩额；可选native两侧额外最多11.25MiB在同一盘内扣除

##### Scenario: 并发耗尽/取消/跨launch恢复 <a id="scenario-inv-hr-080-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 并发reader、第二launch、deadline/cancel或崩溃恢复
- **THEN** atomic reserve/settle/refund不越body总额，不重置start/额度，取消在途/停止新任务，未执行与退款/保守预留准确记录

##### Scenario: 原生noncollector调用无法完整计量 <a id="scenario-inv-hr-080-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** adapter只收collector结果而遗漏native平台内部HTTP调用body/error
- **THEN** 禁止实网并整改计量seam，不用结果bytes事后汇总宣称上限或通过，不承诺128MiB内全部native媒体完成

#### Requirement: 逐节点真实终态与缺陷解释 [HR-081] <a id="req-inv-hr-081"></a>

系统 SHALL 保留每node×side×stage×attempt queued/not_scheduled/executed、原始verdict/typed reason/status、source/revision/config/target/profile/engine/commit、时间/body/预留/退款/复验关联且恰一终态。dependency_skipped、cancelled、budget_not_executed、restricted、unknown、fail、stale/config mismatch MUST 分账；根因不得由后deadline合成覆盖，日志/报告无raw token/password/private URL。未尝试平台/native media MUST 精确报告，双方失败 MUST NOT 解释为永久dead。

##### Scenario: discordant与budget未覆盖 <a id="scenario-inv-hr-081-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 四侧baseline不一致或平台因quota/dependency/deadline未覆盖
- **THEN** 保留逐节点解释、实际executed/skipped/cancelled和unattempted清单，只对可比已执行范围结论，partial不宣称full parity，不扩额/重复试探

##### Scenario: 发布判断与完整原始证据 <a id="scenario-inv-hr-081-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** counts不同但阶段方法/配置/限制已解释或发现真实实现缺陷
- **THEN** 发布依据适用门禁与缺陷解释，不强制headline匹配；真实缺陷整改/自测/review后重验，不擦失败历史或假勾完成

#### Requirement: 最终源码离线门禁与后续终态顺序 [HR-082] <a id="req-inv-hr-082"></a>

executor SHALL 在有界private caches与无外网环境完成Python golden/stage scheduler/ledger、native与same-engine各自build/test/Check/fixtures/pin diff、Go FULL build/test与FULL race、Web type-check/tests/NO_COPY build。full race若真实受资源/工具链限制 MUST 给确切命令/非0或超时/可复现日志与unchecked，不能用targeted PASS或文字冒充全量通过。旧Python经验 MUST 仅局部obsolete标识，不作当前Go部署指南。后续 SHALL 按最终独立Reviewer→批准staged batch→源码冻结刷新preview+Critic→正确COPY assets→授权CSP commit→一致性热备/rollback/app-only runbook rollout/health→独立7200s观察推进。

##### Scenario: 最新源码未测与资源限制 <a id="scenario-inv-hr-082-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 最后修改后build日志空/验证超时或full race限制
- **THEN** 保留未完成gate、稳定日志与复现限制，不声明PASS；private缓存在预检6.4GiB可用盘内有界留余量，不删除共享cache/联网盲补依赖

##### Scenario: preview与长期观察凭据 <a id="scenario-inv-hr-082-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 旧preview健康但不绑定最终源码，或rollout只有短健康检查
- **THEN** 刷新独立preview并获得最终review/critic；rollout完成和7200s观察完成分开，不将health poll/隔离加速当长观察

#### Requirement: 只读分流规则与动态拓扑校验 API [HR-083] <a id="req-inv-hr-083"></a>

系统 SHALL 在 `POST /api/v1/policies/validate`（以及 `GET`）上校验当前已保存 active revision，保持 `valid`、`errors`、`revision_id` 与结构化 issues 兼容。系统 MUST 使用与 publication 一致的启用源节点范围、filters、NodeSources、LatestObservations、AsOf 和风险上下文，纯内存解析，不触发网络、探针执行或数据库写入。依赖仓储错误 MUST 返回明确 incomplete/错误，不得伪造空库存或 valid。

##### Scenario: 同一活跃版本返回结构化诊断 <a id="scenario-inv-hr-083-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 客户端请求当前 active revision 的 validate
- **THEN** 返回对应 revision_id、valid、errors 和包含规则定位信息的 issues，使用与发布相同的来源/探针过滤输入，且无网络与写入副作用

##### Scenario: 仓储依赖读取失败 <a id="scenario-inv-hr-083-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** rules、nodes、filters、sources 或 observations 读取失败
- **THEN** 返回明确不完整/错误诊断，不把失败当作空数组或校验成功

#### Requirement: 默认严格与显式空组 PASS [HR-084] <a id="req-inv-hr-084"></a>

系统 SHALL 增加 `empty_fallback_pass` 布尔配置，SQLite 历史行和创建默认 false；domain、repository、Create/Get/List/Update API、UI 与 resolver/compiler/publication MUST 一致保真，PATCH omission 保持旧值而显式 false 关闭。系统 MUST 仅为该 flag=true 且静态解析为空的配置组允许 PASS；未勾选空组继续 required_nonempty，直接被规则引用的未许可空组返回 error/empty_routed_group，未直接引用的普通空组保持既有 empty_group warning 与编译安全策略。

##### Scenario: 旧数据库迁移和 API 三态 <a id="scenario-inv-hr-084-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 旧库升级且客户端创建或 PATCH 一个策略组
- **THEN** 历史与新建默认 false，Get/List/回包返回真实值，PATCH 未提供保持原值、false 关闭、true 显式开启，失败校验不留下部分修改或新 revision

##### Scenario: 未勾选的被路由空组 <a id="scenario-inv-hr-084-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 分流规则直接引用静态无有效普通成员或无递归真实节点的组，且该组 flag=false
- **THEN** valid=false，返回 error/empty_routed_group 及 rule_id、position、type、value、target_group_id/name，编译/发布仍以 required_nonempty 阻断

##### Scenario: 勾选空组与非空组区别 <a id="scenario-inv-hr-084-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** flag=true 的配置组为空或非空
- **THEN** 只有为空的组获得有效 PASS 语义；非空组不得附加 PASS 成员或运行时 PASS fallback，实际节点 ID 集合和计数均不变

#### Requirement: PASS 继续规则匹配且无隐藏直连 [HR-085] <a id="req-inv-hr-085"></a>

Mihomo 编译器 SHALL 为明确配置且有效为空的组输出 `proxies: [PASS]` 和原生 `empty-fallback: PASS`，MUST 通过官方 Mihomo 离线规则匹配 fixture 证明其继续后续规则而非 DIRECT。该特性 MUST NOT 通过 DIRECT 或缺省 COMPATIBLE 的 Direct fallback 隐藏绕行。非空勾选组不得被扩大为运行时探针失败后自动 PASS；如固定其 native empty-fallback，MUST 使用原生拒绝而非 DIRECT/COMPATIBLE。不支持 PASS 路由的目标 MUST 明确 capability 诊断，保持原 nodes-only 行为，不伪造映射。

##### Scenario: 第一条命中 PASS 后匹配后续控制规则 <a id="scenario-inv-hr-085-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 无网络内核 fixture 第一条规则命中有效 PASS 空组，下一控制规则为非 DIRECT 终点
- **THEN** 实际规则执行落在后续控制规则，而不是提前 Direct 出口，仅有 YAML 字符串不构成语义证明

##### Scenario: 必需 fallback 分支与兼容模式发布 <a id="scenario-inv-hr-085-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 明确配置的空组被必需分支或规则引用并参与正常/compatible 发布
- **THEN** resolver、policy 校验、capability、renderer、preflight 和冻结内容校验一致认可有效 PASS，不被 optional prune 删除，不通过 hidden COMPATIBLE/Direct 绕过

#### Requirement: PASS 不伪造节点或放宽独立守卫 [HR-086] <a id="req-inv-hr-086"></a>

系统 SHALL 分离配置 flag 与有效 PASS 状态，将其纳入 input/snapshot digest。父组及 derived/filter projection MUST NOT 自动继承子组或原组许可，父组仅含 PASS 子组而无真实节点时仍按既有空定义处理。PASS MUST NOT 成为伪造 NodeLogicalID，不豁免循环、悬挂引用、risk、来源/探针条件、能力或全局过滤库存守卫，不覆盖生产用户 flags。

##### Scenario: 子组许可不自动传播 <a id="scenario-inv-hr-086-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 某子组获 PASS 许可但父组未配置，或出现过滤后为空的 derived projection
- **THEN** 父组/投影不会因许可自动转为 PASS；仍执行真实空组/过滤诊断和拓扑安全校验

##### Scenario: 重复编译与切换配置 <a id="scenario-inv-hr-086-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 相同输入重复编译或单独切换 empty_fallback_pass
- **THEN** 相同输入 digest/输出确定，切换配置改变语义 digest，历史 publication bytes 与真实节点数不受改写

#### Requirement: 有界 PCRE 兼容与过滤恢复保真 [HR-087] <a id="req-inv-hr-087"></a>

系统 SHALL 优先复用 RE2，并对已知 RE2 不兼容模式使用现有 regexp2 依赖的有限正 MatchTimeout 和有界 pattern/input/cache。Validate 与 Match MUST 使用一致引擎选择与诊断，not_regex MUST NOT 将超时/错误取反成匹配通过。缺失“便宜”过滤器恢复 MUST 基于稳定归档身份、保持原表达式负向 lookahead 与已知 OR 语义、事务幂等且不覆盖用户非空配置，不能凭名称猜排除或删断言。

##### Scenario: 原表达式黄金匹配保真 <a id="scenario-inv-hr-087-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 对原正则与 x5、x5.9、x6、x50、x5.9.1、literal 前缀、Eeox/einck、大小写及中文黄金语料校验匹配
- **THEN** Validate 与 Match 结果一致且符合原完整表达式语义，自建 fixture 不依赖私有机器数据库或被跳过的恢复测试

##### Scenario: 回溯超时与已有配置 <a id="scenario-inv-hr-087-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 输入触发 bounded timeout 或恢复目标已有用户 filter
- **THEN** 超时报错误且 not_regex 不放行；已有配置不被覆盖，重复恢复不新增虚假修改

#### Requirement: 基于冷归档稳定身份证据精准恢复 Token URL [HR-089] <a id="req-inv-hr-089"></a>

系统 SHALL 仅针对已证实因历史缺陷导致持久化裸 URL 的订阅源（`7li` 与 `魔戒`），基于冷归档数据库（`/home/service/backups/csp-legacy-cold-archive-20260919.db`）中的稳定导入 ID 与完整归档身份进行精确匹配与恢复。系统 MUST 严格校验证据一致性，严禁依据源名称进行模糊猜测。系统 MUST 仅更新受影响的 2 个订阅源的 `source_url_secret_ref` 字段，严禁篡改其他 7 个订阅源的配置、URL 或启停状态（保持 4 enabled / 5 disabled）。

##### Scenario: 成功恢复 7li 与魔戒完整 URL <a id="scenario-inv-hr-089-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 匹配冷归档中的稳定导入 ID 与无参数基准 URL 成功
- **THEN** 系统将目标数据库中 7li 与魔戒的 source_url_secret_ref 恢复为包含有效 token 的完整 URL，并递增 subscription revision

##### Scenario: 其余订阅源状态与配置绝对不被修改 <a id="scenario-inv-hr-089-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行源 Token 恢复操作
- **THEN** 除 7li 与魔戒外的其他 7 个订阅源（包括 5 个 disabled 源及 Dogegg、einck-qzz 等）配置及 enabled 状态保持不变

##### Scenario: 重复执行具备幂等性不产生虚假版本累增 <a id="scenario-inv-hr-089-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 针对已经成功恢复或已包含有效 Token 的数据库再次执行恢复操作
- **THEN** 系统跳过更新，不递增订阅 revision，不写入重复的 audit_events

#### Requirement: 恢复操作日志与对外报告敏感凭据完全脱敏 [HR-090] <a id="req-inv-hr-090"></a>

系统 SHALL 在恢复订阅源完整 URL 时严格执行敏感信息脱敏与审计记录。系统 MUST 将修复事实记录为审计事件（AuditEvent），说明源 URL 已恢复。系统 MUST 严禁在控制台标准输出、日志输出、审计记录摘要或对外 JSON 报告中打印包含 raw token、password 或完整敏感参数的明文 URL，且对 URL 路径中的秘密 Token 片段（如 /sub/<secret>/...）亦必须脱敏掩码，仅输出布尔标记（`restored: true`）与脱敏后的基准地址。

##### Scenario: 恢复执行输出不包含明文 Token <a id="scenario-inv-hr-090-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行源 Token 修复操作并输出执行结果
- **THEN** 输出中仅包含脱敏后的地址与布尔状态，未出现包含 ?token= 敏感参数或敏感路径片段的明文字符串

#### Requirement: 服务端分页台账 [HR-124] <a id="req-inv-hr-124"></a>

系统 SHALL 提供节点列表读取，接受 `page`、`page_size`、固定排序和已声明的协议、国家、连通性、延迟与能力筛选器。响应 MUST 返回 `items`、`page`、`page_size`、`total` 与用于详情读取的节点 `logical_id`；服务端 MUST 在筛选后分页。节点读取 SHALL 支持已声明的 IP risk decision、risk level、provider 和 freshness 筛选，并在列表和详情中仅返回当前风险的脱敏摘要。

##### Scenario: 万级节点的筛选分页 <a id="scenario-inv-hr-124-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 调用者以有效筛选条件请求某一页节点
- **THEN** 返回项数不超过 `page_size`，`total` 表示同一筛选集合总数，且客户端无需下载完整台账

##### Scenario: 风险筛选后的分页 <a id="scenario-inv-hr-124-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 调用者以有效 `risk_decision` 或 `risk_level` 筛选条件请求节点页
- **THEN** 系统先在服务端应用风险筛选再分页，并使每一项仅包含允许的风险摘要而不含完整出口 IP

#### Requirement: Full Inventory Resolution for Validation and Publication [HR-141] <a id="req-inv-hr-141"></a>

The system SHALL retrieve the complete active node set within enabled subscriptions without pagination truncation during policy validation and publication artifact compilation.

##### Scenario: Validation resolves all active nodes beyond default page size <a id="scenario-inv-hr-141-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** the active node inventory contains 90 or more nodes (such as Taiwan nodes at index 70-72 and Canada nodes at index 78-80, or a test fixture with 250 nodes)
- **THEN** policy validation and resolver snapshot provider SHALL load all 90+ nodes without applying the 50/100 limit, resolving matching groups without premature empty group diagnostics

##### Scenario: Publication export includes all active nodes beyond default page size <a id="scenario-inv-hr-141-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** client requests subscription publication export for Mihomo or other target formats
- **THEN** the publication pipeline SHALL compile artifacts against the complete admitted node inventory, ensuring all matching nodes past index 50 appear in the compiled configuration

#### Requirement: Preservation of Admission Scope and Global Filters [HR-142] <a id="req-inv-hr-142"></a>

The system SHALL preserve subscription scope, active-only constraints, notice exclusions, and IP risk policy admissions when fetching unpaginated node inventory.

##### Scenario: Excluded notices and inactive nodes remain filtered <a id="scenario-inv-hr-142-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** unpaginated node retrieval is executed for publication or policy validation
- **THEN** the system SHALL strictly filter out notice-type nodes, inactive nodes, and nodes outside enabled subscriptions, passing only fully admitted candidates to the resolver

#### Requirement: 前后端路由端点双向核对与完整性覆盖 [HR-154] <a id="req-inv-hr-154"></a>

系统 SHALL 建立并维护前端调用点（Web Client API）与后端 Chi 路由注册表的双向核对清单，覆盖认证生命周期（auth）、全局与鉴权配置（settings）、订阅源管理与刷新（subscriptions）、节点查询与连接更新（nodes）、探针运行调度与观测（probes）、策略树与全局过滤（policies）、不可变发布与导出（publications）、系统健康与就绪检查（health/ready）等全量端点（基准 45+ 路由端点），系统 MUST 保证双向核对清单具备动态可扩展性，不将路由数量硬编码写死，且所有对外的 API 路由均须纳入测试覆盖与语义校验。

##### Scenario: 所有已注册路由端点均具备双向映射 <a id="scenario-inv-hr-154-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 运行路由双向校验测试对比前端 API 客户端调用点与后端路由树
- **THEN** 系统确认所有前端调用点均能正确定位到后端已挂载端点，后端注册的控制面端点均具备语义测试用例覆盖

##### Scenario: 动态新增路由端点自动纳入核对矩阵 <a id="scenario-inv-hr-154-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 控制面根据业务扩展新增特定的 API 端点
- **THEN** 验收矩阵自动检查该端点的方法、路由路径、鉴权门禁及返回包络，不因硬编码固定条目数而漏检

#### Requirement: 隔离全接口语义测试与副作用状态验证 [HR-155] <a id="req-inv-hr-155"></a>

系统 SHALL 针对核对清单中的每个端点实施隔离的全接口语义测试。测试断言 MUST 严格基于端点的输入参数/fixture、预期返回字段结构、数据库状态机副作用与实际执行结果，严禁以单一的 HTTP 200 或空响应体代替真实业务成功判定。对于具备状态机转换的接口（如订阅刷新、探针调度、发布撤销、配置覆写），测试 MUST 验证底层 SQLite 数据表记录的实际变更、版本号自增与关联外键状态；对于异常与边界输入，测试 MUST 验证 HTTP 400/401/403/404/409/422 等精准状态码与统一标准错误包络。

##### Scenario: 订阅创建与刷新测试验证真实持久化副作用 <a id="scenario-inv-hr-155-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 调用 `POST /api/v1/subscriptions` 创建订阅并触发 `POST /api/v1/subscriptions/{id}/refresh`
- **THEN** 测试不仅断言 HTTP 200/201，且直接查询数据库验证 `subscriptions` 表新增记录、`subscription_fetches` 表新增审计项，以及节点关联表状态正确更新

##### Scenario: 探针任务取消测试验证状态机流转与中断 <a id="scenario-inv-hr-155-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 对处于运行中或等待中的探针任务发起 `POST /api/v1/probes/runs/{run_id}/cancel`
- **THEN** 测试断言返回状态码且查询底层数据库确认任务状态已变更为 canceled，同时活跃协程被受控释放

#### Requirement: 幂等性、并发控制与权限隔离闭环门禁 [HR-156] <a id="req-inv-hr-156"></a>

系统 SHALL 在全接口语义验收矩阵中针对关键写操作和敏感端点实施幂等性（Idempotency）、并发安全与权限边界（Auth Boundary）校验。对于携带 `Idempotency-Key` 的操作（如探针任务创建、策略变更、发布生成），系统 MUST 在重放相同请求时返回相同的业务凭证且不产生重复数据副作用；对于并发修改操作，系统 MUST 依靠版本号校验（如 `If-Match` 机制或事务锁）防止并发脏写；对于受控管理端点，系统 MUST 在匿名状态、携带无效令牌或携带仅限发布导出的专属令牌时，严格返回 401 Unauthorized 或 403 Forbidden，不发生越权访问。

##### Scenario: 携带相同幂等键的请求不产生重复副作用 <a id="scenario-inv-hr-156-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 客户端使用相同的 Idempotency-Key 重复发送两次探针运行创建请求
- **THEN** 第二次请求返回与第一次相同的已创建 run_id 与结果，数据库中探针运行记录数保持为 1

##### Scenario: 发布专属令牌无法越权访问管理端接口 <a id="scenario-inv-hr-156-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 使用合法的发布导出令牌访问 `/api/v1/nodes` 或 `/api/v1/subscriptions`
- **THEN** 系统响应 403 Forbidden 与 `invalid_token_scope`，控制面不执行任何数据查询或返回

#### Requirement: 独立审查后的黑盒端到端视觉与数据验收 [HR-157] <a id="req-inv-hr-157"></a>

在独立代码审查（Reviewer）给出 PASS 裁决后，系统 SHALL 移交成品环境供 Critic 角色执行包含真实视口几何量测、交互行为与有数据场景截图的黑盒端到端验收。Critic 验收 MUST 在隔离预览环境部署就绪后执行，验证包含真实节点数据与发布链接的完整界面流，确认无布局遮挡、无文本截断、交互顺畅；未通过独立代码审查或前置健康检查失败时，系统 MUST 严格禁止提前进入黑盒验收阶段。

##### Scenario: 独立代码审查通过后触发包含真实数据的黑盒验收 <a id="scenario-inv-hr-157-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 施工与本地自测全部通过，且 Reviewer 出具独立的 PASS 审查报告
- **THEN** 准备隔离预览服务实例，Critic 执行无头浏览器黑盒交互与有数据场景截图核对，出具包含几何量测凭证的最终验收裁决

#### Requirement: 探针出站构建器 L4 网络层与传输层严格解耦 [HR-158] <a id="req-inv-hr-158"></a>

系统 SHALL 在 Sing-box 出站配置构建器中将底层的 L4 网络协议（如 `tcp`、`udp`）与传输层流包装协议（如 `ws`、`grpc`、`http`、`quic` 等）严格解耦。构建器 MUST 遵循底层 Sing-box 核心库的实际协议能力，出站的 `Network` 字段仅填入合法的 L4 网络协议列表（如 `tcp` 或 `udp` 或 `["tcp", "udp"]`），严禁将传输层名称（如 `ws`、`grpc`）作为 L4 网络类型注入。对于支持 UDP 的协议（如 Shadowsocks、Trojan、VLESS、VMess、Hysteria2、TUIC），系统 MUST 尊重配置中的真实网络需求，严禁无条件强制锁定为纯 TCP，确保节点的 UDP 探测与数据报能力真实可用。

##### Scenario: 包含 WebSocket 传输的 VLESS 节点正确区分 L4 与 Transport <a id="scenario-inv-hr-158-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 构建一个网络类型为 `ws`、传输层承载于 TCP 上的 VLESS 出站配置
- **THEN** 系统生成的出站配置中 Network 设置为 `tcp`，并在 Transport 字段配置完整的 WebSocket 传输选项，而不是在 Network 中填入 `ws`

##### Scenario: 声明 UDP 传输或双栈协议的节点正确保留 UDP 能力 <a id="scenario-inv-hr-158-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 构建一个声明支持 UDP 转发或基于 QUIC/UDP（如 Hysteria2、TUIC）的节点配置
- **THEN** 系统在出站配置的 Network 字段中正确包含 `udp`，不被强制降级或改写为单 `tcp`

#### Requirement: Reality 协议公钥标准化与 Short ID 严格格式校验 [HR-159] <a id="req-inv-hr-159"></a>

系统 SHALL 对 Reality 协议的相关出站参数实施严格的规范化与合规性校验。对于公钥参数（`RealityPublicKey` / `pbk`），系统 MUST 验证其经 Base64 或 URL-Safe Base64 解码后为恰好 32 字节的合法公钥，并将其标准化为 Sing-box 库所接受的统一编码格式；若公钥缺失、长度非法或字符集损坏，系统 MUST 明确拒绝构建并抛出可诊断的参数校验错误。对于 Short ID（`RealityShortID` / `sid`），系统 MUST 严格遵循 Reality 协议规范，要求为偶数长度的十六进制字符（0 到 16 字节十六进制串）；若传入奇数长度十六进制串、非法非十六进制字符，系统 MUST 明确拦截并返回规范错误，严禁通过剥离非法字符、向后静默补零或自动补位来篡改节点身份特征。

##### Scenario: 合法 32 字节 Reality 公钥正常标准化并构建 <a id="scenario-inv-hr-159-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 传入合法标准 Base64 或 URL-Safe Base64 编码且解码后长度为 32 字节的 Reality 公钥
- **THEN** 系统成功通过校验，标准化编码并注入 OutboundRealityOptions 完成出站构建

##### Scenario: 奇数长度或非法字符的 Short ID 被拒绝 <a id="scenario-inv-hr-159-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 传入长度为 7 位（奇数长度）或包含字母 `g-z` 的 Reality Short ID
- **THEN** 系统构建器拒绝该配置，返回明确的校验错误 `invalid_reality_short_id`，不静默补零或吞并错误

#### Requirement: 实际库能力测证与真实构建错误脱敏诊断 [HR-160] <a id="req-inv-hr-160"></a>

系统 SHALL 对底层使用的 Sing-box 核心库真实协议支持能力（包括 `xhttp` 等前沿传输模式）以真实的本地构建测试进行验证，严禁仅凭文档或静态声明假设其支持。在安全拨号器与客户端构建流程中（如 `safe_dialer.go`），当底层核心库构建失败时，系统 MUST 捕获底层原始错误，按照错误类别（如协议配置无效、TLS 证书冲突、网络类型不支持、传输层参数不匹配等）进行结构化归因与分类诊断，并向调用方返回具备可诊断性的错误上下文。系统在输出错误诊断与日志时，MUST 严格脱敏，严禁包含任何明文密码、UUID、Token、URL query 参数、私钥或公钥等敏感凭据。系统 MUST 维持既有的 SSRF 防护、私网 IP 拦截与 TLS 证书严格校验安全边界，绝不得为了使探测通过而放宽安全拦截。

##### Scenario: 捕获构建异常并脱敏输出分类诊断 <a id="scenario-inv-hr-160-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 传入格式损坏的传输层参数导致 Sing-box 客户端创建失败
- **THEN** 安全拨号器捕获底层错误，返回包含错误归类与脱敏原因的错误，诊断文本中不暴露 URL 敏感 query 与私钥等凭据

##### Scenario: 安全防护与私网拦截边界不放宽 <a id="scenario-inv-hr-160-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 探测目标指向 RFC1918 私有地址空间或回环地址（如 `127.0.0.1`、`192.168.1.1`）
- **THEN** 安全拨号器在预检阶段严格拦截并返回 SSRF 拒绝错误，不进入底层拨号流程

#### Requirement: 本地配置构建成功与远端网络握手健康严格分离 [HR-161] <a id="req-inv-hr-161"></a>

系统 MUST 在探测状态机中将“本地 Sing-box 客户端配置成功构建”与“远端链路网络握手真实成功”作为两个独立阶段进行判定和证据记录。本地配置构建成功仅代表该节点的配置与语法符合协议要求（build succeeded），绝不能等同于远程节点连接可用或探测健康；只有在经过真实的网络层握手、协议协商且成功获取预期的探测响应（或无重定向成功响应）后，系统方可将该探测观测结果裁决为健康（healthy）。任何本地语法通过但远端连接超时、拒绝连接或 TLS 握手失败的情形，系统 MUST 将其记录为远端网络探测失败（failed/unhealthy），绝不允许伪造虚假健康态。

##### Scenario: 本地构建通过但远程网络超时判定为探测失败 <a id="scenario-inv-hr-161-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 节点配置完全合法通过本地构建器生成了客户端，但向目标地址发起连接时遭遇超时（i/o timeout）
- **THEN** 系统记录构建状态为成功，但该次探测的最终观测结论裁决为超时失败（failed），不将其标记为 healthy

##### Scenario: 本地构建与远程握手均成功方可判定健康 <a id="scenario-inv-hr-161-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 本地构建通过且与远端测试目标成功完成握手并取得状态码 200/204
- **THEN** 系统将该节点在该 connection_revision 下的观测记录更新为 healthy 并记录真实延迟

#### Requirement: 订阅刷新非破坏性原子合并与来源保护 [HR-162] <a id="req-inv-hr-162"></a>

系统 SHALL 将订阅拉取与刷新操作定义为默认非破坏性合并（Non-destructive Merge）语义。当订阅发生网络超时、HTTP 状态码错误、解析失败、提取结果为空、局部条目拒绝（partial/rejected）或新一轮拉取暂时缺失部分节点时，系统 MUST 保留既有已归属于该来源的活跃节点与关联策略边，严禁将其自动下线（active 设为 0）或级联删除。系统 MUST 将订阅状态准确标记为失败或陈旧（stale），但 MUST NOT 宣称该来源名下的物理节点连接失效。系统 MUST 禁止仅依靠“节点数量骤降阈值”来拦截失活，而 MUST 在刷新控制流中全场景提供完整的既有节点状态保留。

##### Scenario: 外部订阅网络或 HTTP 请求失败时全量保留节点 <a id="scenario-inv-hr-162-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 触发订阅刷新但远程服务器返回 5xx 错误、连接超时或 DNS 解析失败
- **THEN** 系统记录该次刷新为失败（outcome 为 failed），保留该订阅历史已同步的所有活跃节点及策略边，活跃节点状态与数量维持不变

##### Scenario: 订阅内容解析失败或提取为空时保留节点 <a id="scenario-inv-hr-162-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 远程服务器返回无法解析的空文本、乱码或格式错误导致提取器返回错误
- **THEN** 系统记录错误摘要并完成审计，该订阅下已有的所有节点保持原有 active=1 状态与连接信息不变，不执行下线或删除

##### Scenario: 局部节点拒绝时合并有效项并保留其余节点 <a id="scenario-inv-hr-162-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 远程订阅内容部分解析成功（outcome 为 partial），且有若干历史节点在当前提取清单中未出现
- **THEN** 系统原子写入/更新有效的新节点与匹配节点，新一轮中暂时缺失的既有节点保持 active=1 且策略边完好，不被删除或失活

#### Requirement: 废除全局孤儿失活与严格来源作用域隔离 [HR-163] <a id="req-inv-hr-163"></a>

系统 MUST 彻底废除任何跨越所有订阅或作用于全局节点表的孤儿批量失活逻辑（如 `UPDATE nodes SET active = 0 WHERE NOT EXISTS node_sources`）。来源清理与解绑操作 MUST 严格受限于当前操作所明确授权的订阅作用域（Subscription Scope）。对于未与任何订阅绑定的独立手工导入节点（Manual Nodes），以及归属于其他订阅源的节点，任何单一订阅的刷新、失效或修剪操作 MUST NOT 对其产生任何附带影响或状态变更。只有当用户对特定订阅或节点执行显式删除（Delete）或显式禁用操作时，系统方可在当前授权实体边界内执行状态更新。

##### Scenario: 订阅来源修剪不波及全局手工导入节点 <a id="scenario-inv-hr-163-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 系统执行特定订阅的来源修剪与对账事务
- **THEN** 事务内仅清理归属于该特定 subscription_id 的关联边，表中未关联 node_sources 的手工节点或其他订阅的节点状态 active 保持为 1，不被附带失活

##### Scenario: 显式删除订阅仅作用于授权范围 <a id="scenario-inv-hr-163-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 用户通过 API 显式删除指定订阅实体
- **THEN** 系统仅级联清理该订阅的配置与归属记录，不影响共享节点在其他订阅或全局手工池中的独立存活

#### Requirement: 来源作用域内安全稳定身份识别与连接版本管理 [HR-164] <a id="req-inv-hr-164"></a>

系统 SHALL 在订阅来源的作用域边界内实现稳定、安全的节点身份识别与连接版本（`connection_revision`）递增追踪。在处理订阅提取项时，系统 MUST 优先匹配源提供的稳定唯一标识符；若源未提供标识符，系统 MUST 仅在同一订阅来源范围内，基于名称与协议等无歧义且唯一的组合条件执行同一节点匹配。若同一来源内存在重名或字段歧义，系统 MUST 采取保守策略保留原节点并新增独立节点，严禁跨不同订阅来源依据相同名称合并不同账户或不同服务的敏感凭据。当同一节点的真实连接参数（服务器地址、端口、传输参数、关键密钥）发生真实变更时，系统 MUST 保持该节点的稳定 `logical_id` 与既有策略边，并严格单调递增真实的 `connection_revision`；单纯显示名（display_name）变更或 JSON 配置键的格式化/排序变化 MUST NOT 递增连接版本。发生真连接变更后，节点先前的历史探测观测记录可保留在审计记录中，但其当前有效观测结论 MUST 立即置为 unknown/pending，严禁沿用已失效版本得出虚假的 healthy 诊断。

##### Scenario: 同一来源内连接参数变更正确递增版本并重置健康结论 <a id="scenario-inv-hr-164-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 同一订阅来源在刷新中提供相同稳定身份标识但更新了服务器端口与传输密钥
- **THEN** 系统更新节点配置，保持 logical_id 与策略边不变，将 connection_revision 自增 1，将当前探测健康态标记为 unknown/pending，不复用旧版本的 healthy 状态

##### Scenario: 仅修改显示名或键排序不递增连接版本 <a id="scenario-inv-hr-164-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅源仅调整了节点的显示名称，或提取的配置字典键顺序发生变化而连接语义完全等价
- **THEN** 系统更新显示名，但 connection_revision 保持不变，既有健康探测结论继续有效

##### Scenario: 跨来源同名节点禁止合并凭据 <a id="scenario-inv-hr-164-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 不同的订阅源 A 与订阅源 B 分别包含名称同为“香港 01”的节点
- **THEN** 系统为两个来源分别分配或计算独立的逻辑身份并独立维护凭据，绝不跨越来源边界合并配置

#### Requirement: 历史误伤失活节点可证据化幂等恢复规程 [HR-165] <a id="req-inv-hr-165"></a>

系统 SHALL 为历史上因全局孤儿失活 SQL 误伤而导致 `active = 0` 的节点（如 2026-09-29 导入被批量失活的存量节点），制定并支持可证据化、可审计、幂等且可安全回滚的精准数据恢复规程。数据恢复操作 MUST 仅针对明确具备误伤归因证据（例如具备历史合法导入记录、且未被用户显式标记为手工禁用或物理删除）的节点记录生效。系统 MUST 严禁通过无条件执行 `UPDATE nodes SET active = 1 WHERE active = 0` 的一键粗暴全量激活手段处理；系统 MUST 确保恢复操作不得覆盖用户的手工禁用状态，对无法确定归因的存量节点 MUST 导出为待确认审计清单；恢复措施 MUST 优先在数据库备份副本上完成演练验证，并明确保持外部订阅凭据是否有效的客观事实。

##### Scenario: 证据化恢复历史误伤节点且不覆盖手工禁用 <a id="scenario-inv-hr-165-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行误伤节点精准修复程序时，数据库中存在因全局失活缺陷被标记 active=0 但具有合法有效配置与导入证据的节点，且存在用户主动禁用的节点
- **THEN** 系统仅将证据确凿的误伤节点恢复为 active=1，用户显式禁用的节点维持 active=0，输出受影响行数与变更证据清单

##### Scenario: 恢复操作具备完全幂等性与可回滚性 <a id="scenario-inv-hr-165-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 对已经恢复的目标数据库重复执行相同的恢复脚本
- **THEN** 系统检测到目标状态已满足，受影响行数为 0，不产生二次修改，且备份回滚脚本可完整恢复至执行前快照

#### Requirement: 节点来源历史只读接口 [HR-215] <a id="req-inv-hr-215"></a>

系统 SHALL 提供 `GET /api/v1/nodes/{id}/source-history` 端点，返回该节点的实时来源、历史归属列表及综合归属状态。

##### Scenario: 查询存在历史的孤儿节点 <a id="scenario-inv-hr-215-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 请求一个已恢复历史的孤儿节点详情时
- **THEN** 系统返回 HTTP 200，响应结构包含空的 `current_sources` 列表、按时间倒序排列的 `history` 列表，且综合状态 `attribution_status` 被准确标记为 `historical_verified`；响应内容绝对不包含密码、Token、UUID 或敏感 URL query。

##### Scenario: 查询当前有源活跃节点 <a id="scenario-inv-hr-215-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 请求一个当前属于活跃订阅的节点时
- **THEN** 系统返回其所属的当前订阅信息（`current_sources`），以及可能存在的离开历史；综合状态被标记为 `current`。

#### Requirement: 节点详情界面只读抽屉呈现 [HR-216] <a id="req-inv-hr-216"></a>

前端界面 SHALL 在现有节点详情侧栏/抽屉（Drawer）中增设来源归属展示区域，与当前实时订阅明确区分。

##### Scenario: 查看历史来源证据 <a id="scenario-inv-hr-216-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 用户在管理界面打开节点详情抽屉并切换至“来源与历史”面板时
- **THEN** 界面清晰呈现当前实时归属标签与历史观测记录（含来源名称、观测时间戳、归属原因与可验证证据类型），不提供手工虚构 URL 输入框，不篡改默认列表的当前有效视图。

#### Requirement: 不可变历史来源台账存储 [HR-217] <a id="req-inv-hr-217"></a>

系统 SHALL 提供 `node_source_history` 表，持久化存储节点的历史来源关系、观测时间戳、归属原因与可验证证据。

##### Scenario: 存储历史归属记录 <a id="scenario-inv-hr-217-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 记录被插入 `node_source_history` 时
- **THEN** 系统持久化存储 `node_logical_id`、可选的 `subscription_id`、脱敏展示名称 `source_label`、`source_identity`、`relation_state`、`cause`、`first_observed_at`、`evidence_kind` 与 `evidence_json`，且严格禁止记录明文密码、密钥、Token 与带敏感参数的原始 URL。

#### Requirement: 证据去重与外键安全 [HR-218] <a id="req-inv-hr-218"></a>

系统 SHALL 通过复合唯一键确保历史证据的幂等性，并保证当订阅被物理删除时历史外键安全置空。

##### Scenario: 重复证据幂等忽略 <a id="scenario-inv-hr-218-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 尝试插入具有相同 `(node_logical_id, source_identity, evidence_kind, first_observed_at)` 的历史记录时
- **THEN** 系统幂等忽略或更新最近观测时间，不引发主键冲突，亦不覆盖不同时间点的多次观测事实。

##### Scenario: 订阅删除级联置空 <a id="scenario-inv-hr-218-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 关联的 `subscriptions` 记录被删除时
- **THEN** 数据库物理外键约束 `ON DELETE SET NULL` 自动将 `subscription_id` 置为 NULL，且保留已固化的 `source_label` 与 `source_identity`，不物理删除历史台账行。

#### Requirement: 基于脱敏清单的幂等回填 [HR-219] <a id="req-inv-hr-219"></a>

管理命令行工具 `csp recover-source-history` SHALL 读取指定的脱敏历史归属清单，并将其原子写入 `node_source_history`。

##### Scenario: 演练与空跑模式 <a id="scenario-inv-hr-219-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行带有 `--dry-run` 选项的恢复命令时
- **THEN** 系统解析清单并验证 SHA-256 校验和，输出拟回填记录摘要与冲突报告，且对数据库做 0 字节写入。

##### Scenario: 原子回填与资产不变量约束 <a id="scenario-inv-hr-219-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 执行正式恢复写入时
- **THEN** 系统在单一数据库事务内完成历史记录插入；在执行前后强制校验资产不变量（总节点数 1014、活跃节点数 31、Scope E 节点集合不变）；若前后数量或状态发生任何漂移，事务立即回滚并以非零退出码失败。

#### Requirement: 丢弃碰撞凭据隔离 [HR-220] <a id="req-inv-hr-220"></a>

系统 SHALL 区分首选导入记录的可靠来源与后续相同端点碰撞被丢弃凭据的记录，严禁将不同凭据行的来源关联附着至当前节点。

##### Scenario: 碰撞凭据不予挂载 <a id="scenario-inv-hr-220-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 清单中包含相同端点但不同凭据的被丢弃记录时
- **THEN** 系统仅将该记录作为冲突或丢弃说明留存证据，绝对禁止将其作为当前节点的有效归属进行回填。

#### Requirement: 刷新剪枝原子归档 [HR-221] <a id="req-inv-hr-221"></a>

当订阅刷新成功且部分现有成员不再包含在上游内容中时，系统 SHALL 在物理删除 `node_sources` 前将既有真实关联归档至 `node_source_history`。

##### Scenario: 成员下线快照持久化 <a id="scenario-inv-hr-221-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅成功刷新导致某个节点自该订阅中移除时
- **THEN** 系统在同一事务中向 `node_source_history` 写入归档快照（`cause = 'refresh_removed'`），记录当时的 `connection_revision` 与 `fetch_id`，随后从 `node_sources` 中移除实时成员，且当前节点的 `active` 与配置参数保持不变。

#### Requirement: 异常与状态变更保护 [HR-222] <a id="req-inv-hr-222"></a>

在刷新失败或订阅启用/停用状态变更时，系统 SHALL 严格隔离，禁止伪造历史归档或清除实时成员。

##### Scenario: 抓取失败状态保持 <a id="scenario-inv-hr-222-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 远程订阅抓取发生网络或解析失败时
- **THEN** 系统保持现存 `node_sources` 关联与健康节点状态，不向 `node_source_history` 插入虚假失败归档，亦不删除实时成员。

##### Scenario: 订阅停用与启用 <a id="scenario-inv-hr-222-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 管理员将某个订阅置为禁用或启用时
- **THEN** 系统更新订阅的 `enabled` 状态，不解绑 `node_sources` 关系，不篡改关联节点的 `active` 属性，亦不向历史台账新增冗余条目。

#### Requirement: 启用订阅库存范围定义与节点列表作用域收敛 [HR-235] <a id="req-inv-hr-235"></a>

系统 SHALL 在 `GET /api/v1/nodes` 支持 `scope` 参数，控制查询的数据集合范围：
1. **默认范围 `scope=enabled_subscriptions`（范围 E）**：
   - 节点必须属于至少一个当前已启用的订阅源（`subscriptions.enabled = 1`）；
   - 关联关系基于该订阅最新一次成功或部分成功的抓取记录（`subscription_fetches.outcome IN ('success', 'partial')`，失败抓取保持上次有效成果）；
   - 节点不得为已确证的公告伪节点（`subscription_entries` 中确认为 notice 且未被用户覆盖为 proxy）；
   - **`active` 状态独立**：系统 MUST NOT 将 `nodes.active` 作为进入范围 E 的前置条件；`active_only`、`search_text`、`protocol`、`risk_*` 及 `subscription_id` 为在范围 E 之上叠加的附加过滤条件；
   - **全局去重**：主列表与总数基于 `logical_id` 全局去重；当节点同时归属多个订阅时仅展示一条；停用其中一个订阅时，只要仍属于其他已启用订阅，该节点继续保留在范围 E 中；
   - 排除无来源关联的手工历史节点，但绝不删除其底表记录；
2. **显式管理资产范围 `scope=all_assets`**：
   - 仅在管理员显式传入 `scope=all_assets` 时返回包含历史失活节点的全库台账（1014 条资产）；
3. **未知作用域报错**：
   - 传入除 `enabled_subscriptions` 与 `all_assets` 之外的未知 scope 参数时，系统 MUST 返回 HTTP 400 错误码 `invalid_node_scope`，严禁隐式回退至全库；
4. **单节点详情与覆写接口保全**：
   - `GET /api/v1/nodes/{logical_id}` 与 `PATCH /api/v1/nodes/{logical_id}/connection` 对所有合法的 `logical_id` 资产开放，允许查看与修改历史节点参数，不受列表默认作用域收敛影响。

##### Scenario: 默认查询仅返回当前启用订阅的有效节点 <a id="scenario-inv-hr-235-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 客户端请求 `GET /api/v1/nodes` 且未传递 `scope` 参数
- **THEN** 系统默认应用 `scope=enabled_subscriptions`，仅返回当前已启用订阅源最新成功抓取中去重后的可用节点（当前生产为 31 条），返回 HTTP 200

##### Scenario: 显式请求全库资产台账 <a id="scenario-inv-hr-235-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 客户端请求 `GET /api/v1/nodes?scope=all_assets`
- **THEN** 系统返回包含历史失活节点的全量资产台账（当前生产为 1014 条），返回 HTTP 200

##### Scenario: 未知 scope 参数直接返回 400 <a id="scenario-inv-hr-235-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 客户端请求 `GET /api/v1/nodes?scope=invalid_scope`
- **THEN** 系统返回 HTTP 400 与错误码 `invalid_node_scope`，不返回任何节点列表数据

##### Scenario: 停用订阅源不影响仍被其他启用源引用的共享节点 <a id="scenario-inv-hr-235-4"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 节点 N 同时属于订阅源 A（启用）与订阅源 B（启用），管理员将订阅源 B 停用（`enabled=0`）
- **THEN** 节点 N 依然出现在默认的 `enabled_subscriptions` 视图中；若订阅源 A 也停用，节点 N 从该视图中消失，但 `nodes` 表记录与 `active` 状态不被修改

##### Scenario: 查看与修改历史节点详情不被阻断 <a id="scenario-inv-hr-235-5"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 管理员通过 `GET /api/v1/nodes/{logical_id}` 调阅不在范围 E 中的历史节点
- **THEN** 系统返回该节点的完整详情与历史来源记录，返回 HTTP 200

#### Requirement: 订阅列表归属计数与动态生效语义 [HR-236] <a id="req-inv-hr-236"></a>

系统 SHALL 在 `GET /api/v1/subscriptions` 的返回条目中包含动态计算的节点计数：
1. `node_count` (integer): 当前订阅作为已启用源对全局可用库存的去重非公告有效成员贡献。当订阅被停用（`enabled = 0`）时，`node_count` 必须为 0；
2. `source_node_count` (integer): 当前订阅在其最后一次成功或部分成功抓取中包含的去重非公告有效成员数。无论该订阅是否启用，均保留该计数值用于向管理员说明该源的历史成员规模；
3. `counts_scope` (string): 固定返回 `'enabled_subscriptions'`；
4. 若订阅从未产生过成功或部分成功的抓取，`node_count` 与 `source_node_count` 均为 0；若先前成功而最新抓取失败，系统保持上次成功抓取的成员计数；
5. 订阅启用状态切换通过既有 `PATCH /api/v1/subscriptions/{id}` 完成，即时生效，绝不修改关联节点的 `nodes.active` 属性，计数动态查询不作永久缓存。

##### Scenario: 启用状态订阅展示贡献数与源成员数相等 <a id="scenario-inv-hr-236-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 查询包含有效抓取的已启用订阅（如 Dogegg 拥有 16 个有效成员）
- **THEN** 返回的条目中 `node_count: 16`，`source_node_count: 16`，`counts_scope: 'enabled_subscriptions'`

##### Scenario: 停用订阅后 node_count 归零但保留 source_node_count <a id="scenario-inv-hr-236-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 管理员将拥有 16 个成员的订阅停用（`enabled = 0`）并重新请求订阅列表
- **THEN** 该订阅条目中 `node_count: 0`，`source_node_count: 16`

##### Scenario: 多源共享节点导致单源累加和大于全局去重总数 <a id="scenario-inv-hr-236-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅 A 与 订阅 B 分别贡献 10 个节点，且其中 3 个节点为二者共享
- **THEN** 订阅 A 的 `node_count` 为 10，订阅 B 的 `node_count` 为 10，但全局 `GET /api/v1/nodes` 范围 E 返回 17 个去重节点

#### Requirement: 探测候选池收敛与指标对账保全 [HR-237] <a id="req-inv-hr-237"></a>

系统 SHALL 在 `GET /api/v1/probes/pool` 与探测任务派发中收敛至范围 E：
1. **指标结构扩展**:
   - `inventory_total` (integer): 范围 E 中的总节点数（包含活跃与失活）；
   - `candidate_total` (integer): 范围 E 中满足 `active = 1` 的探测候选总数；
   - `scope` (string): 固定返回 `'enabled_subscriptions'`；
   - `total_count` (integer): 保持旧字段含义不变，其数值与 `candidate_total` 一致，确保既有消费者的计量单位不发生漂移；
2. **探测候选执行门禁**:
   - 周期性探测调度（periodic coordinator）、全量探测触发（run all）以及指定节点直接派发（selected IDs）均必须严格施加 `E + active=1 + exclude_notices` 过滤；
   - 严禁通过直接传递已停用订阅源节点的 ID 绕过门禁执行探测。

##### Scenario: 探测池状态反映启用库存与候选指标 <a id="scenario-inv-hr-237-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 请求 `GET /api/v1/probes/pool`
- **THEN** 返回结果包含 `inventory_total`、`candidate_total`、`total_count`（等于 candidate_total）与 `scope: 'enabled_subscriptions'`

##### Scenario: 指定探测已停用订阅节点被严格排除 <a id="scenario-inv-hr-237-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 创建探测任务时在 `node_logical_ids` 中传入属于已停用订阅的节点 ID
- **THEN** 系统在构建任务目标时将其排除，不向该节点派发拨测

#### Requirement: 发布订阅编译与既有快照不可变保全 [HR-238] <a id="req-inv-hr-238"></a>

系统 SHALL 在配置发布与预览中保持快照不可变：
1. **新发布与预览过滤**:
   - 发布解析器（`publication.Service`）在构建配置快照时，仅从范围 E 中筛选 `active = 1` 的节点；
   - 停用订阅源后，新生成的预览配置立即排除属于停用源的节点；
2. **已发布快照字节绝对不可变**:
   - 已创建并对外分发的发布快照内容（`ConfigBytes`）受只读保护，订阅源状态变化或节点状态变化严禁篡改已发布的历史快照。

##### Scenario: 停用订阅后新预览排除该源节点 <a id="scenario-inv-hr-238-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 管理员停用某订阅源并请求生成新的客户端配置预览
- **THEN** 预览配置中仅包含当前仍处于启用状态订阅的活跃节点

##### Scenario: 历史发布快照内容保持字节级不可变 <a id="scenario-inv-hr-238-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 某发布快照已生成，管理员后续停用其中部分节点所归属的订阅源
- **THEN** 通过分发 Token 请求该既有发布配置时，返回的配置文本与生成时完全一致

#### Requirement: 节点身份稳定与不可变连接版本解耦 [HR-239] <a id="req-inv-hr-239"></a>

系统 SHALL 保持既有 1014 个节点的 `logical_id` 不变，并引入 `node_connection_versions` 记录不可变连接版本。节点配置的有效参数以 `effective_config_json` 存储，并通过 `(node_logical_id, connection_revision)` 唯一主键约束。当订阅刷新导致节点连接参数（服务器地址、端口、凭据或传输参数）发生实质改变时，系统 MUST 递增 `connection_revision` 并插入新版本记录，历史版本的探针观测记录自动标记为陈旧状态（stale），绝不原地覆盖破坏历史时序。

##### Scenario: 节点仅更新名称不改变连接版本 <a id="scenario-inv-hr-239-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅刷新拉取到同名协议节点，其服务器与证书参数完全一致，仅别名有微小变动
- **THEN** 系统更新 `nodes.display_name` 与 `updated_at`，保持 `connection_revision` 不变，不触发新版本生成

##### Scenario: 节点连接参数变更自动派生新版本 <a id="scenario-inv-hr-239-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅上游将某 VLESS 节点的端口从 443 修改为 8443
- **THEN** 系统在 `node_connection_versions` 中新增对应 `connection_revision = 2` 的不可变记录

#### Requirement: 权威当前版本指针表与单向无环外键 [HR-240] <a id="req-inv-hr-240"></a>

为了避免 SQLite 不支持 `ALTER TABLE ADD` 复合外键以及 `nodes` 与 `versions` 产生循环外键依赖，系统 SHALL 引入 `node_connection_heads` 作为当前权威版本指针：
1. `node_connection_heads` 以 `logical_id` 为主键，外键单向引用 `nodes(logical_id)`，并通过 `(logical_id, connection_revision)` 复合外键单向引用 `node_connection_versions(node_logical_id, connection_revision)`；
2. `nodes` 表不添加表级外键反向引用 versions 表；
3. 新增节点时，系统 MUST 在单个事务中严格按 `nodes` -> `node_connection_versions` -> `node_connection_heads` 顺序写入；
4. 现有 `nodes.connection_revision` 在过渡期仅作为只读投影视图，完成切换后严禁双写；所有下游消费者唯一读取 Heads 指向的版本。

##### Scenario: 指向不存在版本的外键违规直接阻断 <a id="scenario-inv-hr-240-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 事务尝试将 `node_connection_heads.connection_revision` 指向尚未在 `node_connection_versions` 插入的版本号
- **THEN** SQLite 外键机制抛出 `FOREIGN KEY constraint failed` 并回滚事务

#### Requirement: 用户显式字段级修改（Overrides）持久化与合并 [HR-241] <a id="req-inv-hr-241"></a>

系统 SHALL 提供 `node_overrides` 表记录用户在 Web 端对特定节点显式修改的字段（如自定义 SNI、跳过证书验证标志、自定义标签等）。每次订阅刷新重新计算节点参数时，系统 MUST 显式提取上游基础配置并叠加上述 overrides 派生 `effective_config_json`，保证用户的人工修改在订阅刷新后绝不被静默抹除。

##### Scenario: 用户自定义 SNI 跨订阅刷新保持生效 <a id="scenario-inv-hr-241-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 用户在控制台将某节点 SNI 手动设置为 `custom.domain.com`，随后该节点所属订阅执行刷新
- **THEN** 系统在计算该节点新版本参数时自动叠加该覆盖项，生成的 effective_config 中 SNI 依然为 `custom.domain.com`

#### Requirement: 多源共享节点写时复制（CoW）隔离 [HR-242] <a id="req-inv-hr-242"></a>

系统 SHALL 维持 `node_sources` 表记录节点与订阅源的多对多从属关系。当多个订阅源共享同一逻辑节点且其中一个订阅更新了节点参数时，系统 MUST 执行写时复制隔离：为发生变更的订阅派生隔离的 scoped logical ID，同时保持未变动订阅对原节点的引用与策略拓扑 group_edges 关联完整，防止跨源配置相互污染。

##### Scenario: 单个订阅更新共享节点触发分支隔离 <a id="scenario-inv-hr-242-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅 A 与 订阅 B 共享节点 X，订阅 A 下发了新的凭据参数
- **THEN** 订阅 A 的关联条目分支至 `node_<scoped_hash>`，节点 X 继续保持归属于订阅 B，两者的策略分组引用均不受破坏

#### Requirement: 单 SQLite 主库与 WAL 运行模式 [HR-243] <a id="req-inv-hr-243"></a>

系统 SHALL 维持单一 SQLite 主业务数据库文件（`/data/csp-v1.db`），启用 WAL 日志模式（`PRAGMA journal_mode = WAL`）、普通同步等级（`PRAGMA synchronous = NORMAL`）、内置外键约束检查（`PRAGMA foreign_keys = ON`）以及 5000 毫秒忙等待锁（`PRAGMA busy_timeout = 5000`）。系统 MUST 保证写操作在同一进程内事务性串行执行，避免无意义的多库拆分与分布式复杂性。

##### Scenario: 验证主库 WAL 与外键配置生效 <a id="scenario-inv-hr-243-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 系统启动并打开主数据库连接
- **THEN** 系统执行 `PRAGMA journal_mode` 返回 `wal`，且 `PRAGMA foreign_keys` 返回 `1`

#### Requirement: 领域所有权、27 表现状与增量关联表管理 [HR-244] <a id="req-inv-hr-244"></a>

系统 SHALL 将现有的 27 张数据表（1 张技术表与 26 张业务表，包含 7 张 IP Risk 表）严格按领域划分为 Ingestion（摄入）、Inventory（库存）、Policy Topology（策略拓扑）、Probes（探针与风险）、Publication（发布工件）与 Settings/Audit（系统配置与审计）六大界限上下文，明确读写所有权：
1. **Ingestion 域** (`subscriptions`, `subscription_fetches`, `admission_rules`, `subscription_payloads`, `subscription_entries`): 负责订阅源凭证、拉取记录与原始负载解析；
2. **Inventory 域** (`nodes`, `node_sources`, `node_connection_versions`, `node_connection_heads`, `node_overrides`): 负责稳定节点资产、多源所有权关系、连接版本、权威当前版本指针与用户显式修改；
3. **Policy Topology 域** (`node_groups`, `group_edges`, `group_node_filters`, `global_node_filters`, `policy_rules`, `configuration_revisions`): 负责策略拓扑、路由规则与版本快照；
4. **Probes 域** (`probe_schedules`, `probe_batches`, `probe_batch_runs`, `probe_runs`, `probe_observations`, `ip_risk_provider_settings`, `ip_risk_observations`, `risk_policy_revisions`, `risk_policy_providers`, `risk_policy_score_bands`, `risk_policy_trait_rules`, `risk_policy_group_bindings`): 负责轻量级与平台能力探测调度、租约控制与统一 Nullable 1:N 测量记录；
5. **Publication 域** (`publications`, `publication_payload_refs`): 负责不可变导出快照、分发清单与 Payload 物理引用保护；
6. **Settings/Audit 域** (`settings`, `audit_events`, `schema_migrations`): 负责全局安全鉴权、操作流水与数据架构版本。

##### Scenario: 业务表写入权唯一归口 <a id="scenario-inv-hr-244-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 探针调度器执行探测完成并生成观测数据
- **THEN** 仅允许由 Probe 领域服务写入 `probe_observations`，其他领域服务不可直接跨域修改

#### Requirement: 维护短窗实测与受控回滚对账 [HR-245] <a id="req-inv-hr-245"></a>

系统在执行架构迁移时，SHALL 在隔离演练中实测真实的停写耗时，并获得用户明确确认，严禁宣称固定 30 秒或绝对零丢失；系统 SHALL 在迁移前执行 SQLite 官方 `.backup` 在线热备；发生未预期异常时，系统 MUST 提供明确的回滚路径：
1. 校验热备文件的 1014 节点、29 组、163 规则基准；
2. 若切换后产生新写入，先执行 SQL 增量导出，再执行快照覆盖回滚；
3. 向操作员呈报受影响数据窗口并执行增量数据合并，或由操作员显式确认并接受数据 Loss 窗口。

##### Scenario: 隔离演练实测维护耗时 <a id="scenario-inv-hr-245-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 在隔离测试环境中对 1014 节点执行 Migration 000015 演练
- **THEN** 测量并记录真实的 DDL 与指针初始化耗时，输出演练报告供上线审批

#### Requirement: DDL 唯一归口 MigrationRunner [HR-246] <a id="req-inv-hr-246"></a>

系统 SHALL 将所有数据表结构定义、索引变更与历史字段修改唯一归口于 `internal/repository/sqlite/` 中的顺序迁移脚本。系统 MUST 严禁在 Repository 构造函数或业务代码中动态执行 `CREATE TABLE`、`CREATE INDEX` 或 `ALTER TABLE`，防止隐式 DDL 竞争与架构漂移。

##### Scenario: 构造 Repository 实例时不触发运行时 DDL <a id="scenario-inv-hr-246-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 应用程序初始化 `NewProbeObservationRepository(db)`
- **THEN** 构造函数仅接收只读连接句柄并返回实例，不向数据库发送任何 `CREATE INDEX` 或 `ALTER TABLE` 语句

#### Requirement: 上游原始负载（Payload）权威事实持久化 [HR-247] <a id="req-inv-hr-247"></a>

系统 SHALL 在每次订阅抓取成功后，在 `subscription_payloads` 表中持久化保存完整的未经修改的上游原始响应体（BLOB 格式，支持 gzip 压缩存储），并记录内容哈希、HTTP 状态码以及关键协商响应头（ETag、Last-Modified、Content-Type、Profile-Update-Interval）。系统 MUST 排除响应头中的敏感 Cookie 或 Authorization 信息。该原始响应仅供内部安全回放与审计对账，严禁对外网管理 API 暴露明文凭据。

##### Scenario: 抓取成功完整保存原始负载 <a id="scenario-inv-hr-247-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅服务成功抓取到上游下发的响应内容
- **THEN** 系统在单个事务中插入 `subscription_fetches` 与对应的 `subscription_payloads`，记录正确的 content_digest 与 body BLOB

##### Scenario: 抓取异常保留存量资产 <a id="scenario-inv-hr-247-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 订阅源返回 HTTP 403 Forbidden 或连接超时
- **THEN** 系统在 `subscription_fetches` 中记录错误状态与失败原因，绝不清除或删除该订阅已持有的健康节点库存，保持历史 last-good 状态有效

#### Requirement: 派生条目（Entries）、独立身份与严格公告伪节点分类器 [HR-248] <a id="req-inv-hr-248"></a>

系统 SHALL 在解析 Payload 时，生成对应的 `subscription_entries` 派生记录：
1. 每个条目拥有独立的 `id`（UUIDv7）作为主键，施加 `UNIQUE (payload_id, ordinal)` 约束，确保同一 Payload 内原始顺序记录；
2. 即使上游存在相同连接参数的多个条目（如 Dogegg 的 3 个不同内容公告），系统 MUST 全部独立写入入库，**严禁将连接指纹作为条目身份进行折叠去重**；
3. `source_key` 仅作为跨刷新匹配的候选锚点，**严禁施加 `UNIQUE (subscription_id, source_key)` 唯一约束**；
4. 系统 MUST 运行基于来源语义证据与规则版本的前置门禁分类器，严禁使用单项条件或全局无来源前提的判定：
   - **判定逻辑**: `verified_source_rule(subscription_id, source_evidence, rule_version) AND exact_rule_match(entry) => notice`；
   - **前置来源门禁 `verified_source_rule`**: 仅对已完成实机取证、来源上下文确证且具备版本化规则的特定订阅源（如本轮核准的 Dogegg 订阅及其 `source_provenance` 与 `rule_version`）启用该专用匹配规则。记录来源上下文是必要凭据但非足够门禁，未通过已验证来源规则的其他订阅源严禁套用；
   - **精确匹配 `exact_rule_match` (全条件组合)**: 在满足前置门禁的订阅中，条目必须**同时满足全部四项条件**：
     - `server` 属于 `127.0.0.1`、`localhost` 或 `0.0.0.0`；
     - `port == 1`；
     - 凭据 UUID 严格符合 RFC 4122 全零测试占位符（`00000000-0000-4000-8000-000000000000`）；
     - 标题文本明确匹配提示语义（“剩余流量”、“套餐到期”、“新域名”、“公告”、“通知”）；
   - 满足前置门禁且命中全部四项条件者归类为 `notice`，`node_logical_id = NULL`，不插入 `nodes` 表，不派发探针拨测，不进入发布配置；
   - 若订阅源缺乏已验证来源规则，即使条目满足上述四项特征也**绝对不自动归类为 notice**，而是保留为 `unknown` 候选状态并允许用户人工审核与显式纠偏，不自动推广全网；
   - 正常可解析的真实代理连接（包括使用本地私网 IP 但具备有效端口与凭据者）始终归类为 `proxy`，正常入库流转，无需额外人工审批。

##### Scenario: 同连接参数的多公告全部独立持久化 <a id="scenario-inv-hr-248-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 解析包含 3 个相同 `127.0.0.1:1` 连接参数但分别展示流量、到期、域名的条目
- **THEN** 系统在 `subscription_entries` 成功插入 3 条独立记录，均持有不同的 `id` 与 `ordinal`，无数据折叠

##### Scenario: 提示性流量伪节点满足已验证来源与全条件被归类为 notice <a id="scenario-inv-hr-248-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 解析已验证订阅源（如 Dogegg）中包含 `Dogegg-剩余流量：81.9GB`（server: 127.0.0.1, port: 1, RFC 全零占位 UUID）的条目
- **THEN** 该条目在 `subscription_entries` 中被归类为 `notice`，不被插入 `nodes` 表，不派发探针拨测，且不出现在编译导出列表中

##### Scenario: 未验证来源即使满足四条件亦保留为 unknown 候选 <a id="scenario-inv-hr-248-3"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 解析尚未配置已验证来源规则的普通订阅中包含相同回环与全零 UUID 的提示性条目
- **THEN** 系统将其归类为 `unknown` 并保留，不自动标记为 notice，供用户显式纠偏

##### Scenario: 正常本地代理节点绝不被误判 <a id="scenario-inv-hr-248-4"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 解析 server 为 `127.0.0.1` 但 port 为 `1080` 且带有有效密码的 SOCKS5 节点
- **THEN** 该节点因未满足全套判定条件，被归类为 `proxy` 并正常入库无需额外审批

#### Requirement: 条目级人工纠偏与跨刷新稳定继承 [HR-249] <a id="req-inv-hr-249"></a>

系统 SHALL 在 `subscription_entries` 支持条目级用户人工纠偏：
1. 包含 `user_kind_override`（`proxy`, `notice`, `unknown`）、`override_anchor`、`override_reason`、`override_at` 与 `actor_ref`；
2. 即使条目被归类为 `notice` 且 `node_logical_id = NULL`，用户仍可在条目级执行人工纠偏；
3. 跨刷新匹配优先级：1) 上游显式稳定 key；2) 有版本的分类器提取的明确 `semantic_slot`（如 `traffic_remaining`, `expiry_date`）；3) 上下文与内容 1:1 唯一精确匹配；
4. 若出现改名、新内容或同锚点歧义，系统 MUST NOT 强制继承或合并，必须保留全部条目并提供冲突诊断，不影响正常连接。

##### Scenario: 用户人工将公告条目纠偏为 proxy <a id="scenario-inv-hr-249-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 用户对某特殊格式条目提交 `user_kind_override = 'proxy'`，随后该订阅触发刷新
- **THEN** 系统在新一轮条目派生时依据匹配规则自动继承该覆盖，将其作为代理节点入库

### Explicitly Pending Historical Stubs

#### Requirement: 历史导入为离线、一次性、待审草稿 [HR-035] <a id="req-inv-hr-035"></a>

系统 SHALL 只提供显式启动的离线导入命令，不向服务路由暴露自动导入。导入器 MUST 只读打开历史 SQLite 源，并将 allowlist 中的订阅元数据、非秘密节点逻辑、分组、规则和 DNS 逻辑内容转换为目标 schema 的待审草稿。导入内容 MUST 在授权管理者逐项或整体审查激活前不参与刷新、探测或导出。

##### Scenario: 导入完成但未审查 <a id="scenario-inv-hr-035-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 离线导入成功完成
- **THEN** 新系统显示带审计报告的待审草稿，且调度器和导出接口不使用这些草稿

#### Requirement: 受控订阅生命周期 [HR-050] <a id="req-inv-hr-050"></a>

系统 SHALL 允许授权管理者创建、修改、启用、停用和手动刷新订阅。每个订阅 MUST 包含稳定逻辑 ID、显示名称、来源 URL、可选 fetch proxy 引用、User-Agent 策略、刷新间隔和启用状态；秘密部分 MUST 不出现在列表、日志或错误中。

##### Scenario: 禁用订阅不被调度 <a id="scenario-inv-hr-050-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 一个订阅被停用
- **THEN** 周期调度不再创建该订阅的刷新作业，手动刷新必须被明确拒绝或先要求重新启用

#### Requirement: 节点与派生数据原子全量清空 [HR-071] <a id="req-inv-hr-071"></a>

系统 SHALL 提供严格事务性的节点库存彻底清空闭包。当触发节点重置时，系统 MUST 在单一原子事务中将 nodes 表以及所有依赖于 nodes 的衍生表清空（行数严格归 0），包括 `nodes`、`node_sources`、`node_source_history`、`node_connection_heads`、`node_connection_versions`、`node_overrides`、`probe_observations`、`probe_runs`、`probe_batches`、`probe_batch_runs` 和 `ip_risk_observations`。重置操作 MUST 彻底物理清除历史遗留节点，严禁采用保留墓碑状态或伪置无效而实际保留节点的方案。

##### Scenario: 执行重置后节点与衍生表计数清零 <a id="scenario-inv-hr-071-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 在目标数据库上执行节点库存重置事务并成功提交
- **THEN** `nodes`、`node_sources`、`node_source_history`、`node_connection_heads`、`node_connection_versions` 及所有探针、IP风险观测表的行数均为 0

##### Scenario: 事务异常时完整回滚不造成部分数据丢失 <a id="scenario-inv-hr-071-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 重置事务在执行过程中由于锁超时或受控注入错误而发生失败
- **THEN** 数据库事务完全回滚，既有节点与历史记录行数与内容维持不变

#### Requirement: 首次请求前冻结全部manifest与可比条件 [HR-079] <a id="req-inv-hr-079"></a>

运行 SHALL 冻结所有side/node/stage/config/revision/source/engine/次序、真实target及子请求、timeout/TLS/SNI/redirect/HTTP2/UA/selection、8/2/0并发、全部quota、ledger原始start/deadline与exact child argv/hash。comparative客户端 MUST 统一target/timeout/TLS/redirect/HTTP2，native defaults差异 MUST 明示。execute MUST 验证manifest/pin/ledger绑定，不允许未记载的endpoint/并发/TLS变化、额外重试或自动扩额。

##### Scenario: 默认无网络与manifest变更 <a id="scenario-inv-hr-079-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** plan/freeze/report或execute缺allow-network、manifest/pin不符、speed>0/测速URL、body超出对应manifest冻结限额（旧批次128MiB、新reissue授权1GiB）、deadline超时（旧批次1800s、新reissue授权7200s）或review门禁未满足
- **THEN** 发请求前fail-closed，命令默认无外部网络，allow-network不能绕过门禁

##### Scenario: 归一化与原生差异 <a id="scenario-inv-hr-079-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** comparative无法对齐某配置或native默认不同
- **THEN** 标不可比/方法差异并分账，不能静默把native Check改为normalized stage或凭headline计数判缺陷

#### Requirement: 前端规则管理、正则编辑与刷新校验 [HR-088] <a id="req-inv-hr-088"></a>

前端 SHALL 保留既有规则 Tab、warnings、规则定位、手动校验和 AbortController/seq/revision 守卫，增加默认未勾选的空组 PASS checkbox 和 regex/not_regex 创建编辑支持。UI MUST 明确 PASS 继续规则匹配不是直连；初始/保存/reload/retry 成功后基于一致最新 revision 自动 validate，失败加载/保存不得伪造通过，晚响应不得覆盖新状态。

##### Scenario: 保存重载与正则编辑 <a id="scenario-inv-hr-088-1"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 用户创建/编辑 group、checkbox 或 regex/not_regex 并保存后重载
- **THEN** UI 展示持久化真实值与原正则，错误不产生半保存，文案明确 PASS 非 DIRECT

##### Scenario: 重试加载与晚校验响应 <a id="scenario-inv-hr-088-2"></a>
- **GIVEN** 订阅源与节点资产处于有效配置状态
- **WHEN** 用户点击错误卡 retry 或 reload，旧 validate 响应晚到
- **THEN** 成功加载后对最新 revision 触发 validate，旧响应被丢弃，失败保留真实错误与每条规则 issues


---

## Success Criteria

### Measurable Outcomes
- **[SC-INV-01]** 订阅拉取失败率 100% 情况下存量节点资产损耗率为 0%。
- **[SC-INV-02]** 重复节点跨刷新 `logical_id` 守恒率达到 100%。
- **[SC-INV-03]** 节点人工覆写参数在 10 次以上重复拉取中保持率达到 100%。
- **[SC-INV-04]** 启用作用域下未启用订阅专有节点的导出泄漏率为 0。


## 原始载荷整改（2026-10-25 当前有效补充）

用户明确无需保存订阅响应原文，保留每次所属节点的已解析信息与主订阅信息。停止新增原文持久化，历史原文经显式离线维护清除，节点凭据、来源、分类纠偏、订阅配置及历史发布保持可用。完整需求 FR-RAW-01..06、场景与成功条件见 [payload-retirement-20261025.md](payload-retirement-20261025.md)。旧 R4 的保留期待定不再是当前阻断。本轮不包含生产部署或生产数据清理。

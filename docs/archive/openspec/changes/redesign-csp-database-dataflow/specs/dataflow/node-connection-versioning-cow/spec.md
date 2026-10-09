## Purpose

规范稳定节点资产身份、不可变连接版本管理、权威当前版本指针（Heads）、用户显式覆盖机制以及跨多订阅源的写时复制（Copy-on-Write）隔离。

## ADDED Requirements

### Requirement: 节点身份稳定与不可变连接版本解耦
系统 SHALL 保持既有 1014 个节点的 `logical_id` 不变，并引入 `node_connection_versions` 记录不可变连接版本。节点配置的有效参数以 `effective_config_json` 存储，并通过 `(node_logical_id, connection_revision)` 唯一主键约束。当订阅刷新导致节点连接参数（服务器地址、端口、凭据或传输参数）发生实质改变时，系统 MUST 递增 `connection_revision` 并插入新版本记录，历史版本的探针观测记录自动标记为陈旧状态（stale），绝不原地覆盖破坏历史时序。

#### Scenario: 节点仅更新名称不改变连接版本
- **WHEN** 订阅刷新拉取到同名协议节点，其服务器与证书参数完全一致，仅别名有微小变动
- **THEN** 系统更新 `nodes.display_name` 与 `updated_at`，保持 `connection_revision` 不变，不触发新版本生成

#### Scenario: 节点连接参数变更自动派生新版本
- **WHEN** 订阅上游将某 VLESS 节点的端口从 443 修改为 8443
- **THEN** 系统在 `node_connection_versions` 中新增对应 `connection_revision = 2` 的不可变记录

### Requirement: 权威当前版本指针表与单向无环外键
为了避免 SQLite 不支持 `ALTER TABLE ADD` 复合外键以及 `nodes` 与 `versions` 产生循环外键依赖，系统 SHALL 引入 `node_connection_heads` 作为当前权威版本指针：
1. `node_connection_heads` 以 `logical_id` 为主键，外键单向引用 `nodes(logical_id)`，并通过 `(logical_id, connection_revision)` 复合外键单向引用 `node_connection_versions(node_logical_id, connection_revision)`；
2. `nodes` 表不添加表级外键反向引用 versions 表；
3. 新增节点时，系统 MUST 在单个事务中严格按 `nodes` -> `node_connection_versions` -> `node_connection_heads` 顺序写入；
4. 现有 `nodes.connection_revision` 在过渡期仅作为只读投影视图，完成切换后严禁双写；所有下游消费者唯一读取 Heads 指向的版本。

#### Scenario: 指向不存在版本的外键违规直接阻断
- **WHEN** 事务尝试将 `node_connection_heads.connection_revision` 指向尚未在 `node_connection_versions` 插入的版本号
- **THEN** SQLite 外键机制抛出 `FOREIGN KEY constraint failed` 并回滚事务

### Requirement: 用户显式字段级修改（Overrides）持久化与合并
系统 SHALL 提供 `node_overrides` 表记录用户在 Web 端对特定节点显式修改的字段（如自定义 SNI、跳过证书验证标志、自定义标签等）。每次订阅刷新重新计算节点参数时，系统 MUST 显式提取上游基础配置并叠加上述 overrides 派生 `effective_config_json`，保证用户的人工修改在订阅刷新后绝不被静默抹除。

#### Scenario: 用户自定义 SNI 跨订阅刷新保持生效
- **WHEN** 用户在控制台将某节点 SNI 手动设置为 `custom.domain.com`，随后该节点所属订阅执行刷新
- **THEN** 系统在计算该节点新版本参数时自动叠加该覆盖项，生成的 effective_config 中 SNI 依然为 `custom.domain.com`

### Requirement: 多源共享节点写时复制（CoW）隔离
系统 SHALL 维持 `node_sources` 表记录节点与订阅源的多对多从属关系。当多个订阅源共享同一逻辑节点且其中一个订阅更新了节点参数时，系统 MUST 执行写时复制隔离：为发生变更的订阅派生隔离的 scoped logical ID，同时保持未变动订阅对原节点的引用与策略拓扑 group_edges 关联完整，防止跨源配置相互污染。

#### Scenario: 单个订阅更新共享节点触发分支隔离
- **WHEN** 订阅 A 与 订阅 B 共享节点 X，订阅 A 下发了新的凭据参数
- **THEN** 订阅 A 的关联条目分支至 `node_<scoped_hash>`，节点 X 继续保持归属于订阅 B，两者的策略分组引用均不受破坏

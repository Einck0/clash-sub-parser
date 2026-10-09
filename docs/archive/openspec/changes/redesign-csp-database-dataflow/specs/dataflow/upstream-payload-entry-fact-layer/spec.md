## Purpose

定义订阅抓取原始负载持久化与派生条目解析规范，实现权威事实保留与公告伪节点（Notice）与真实代理节点（Proxy）的物理级解耦。

## ADDED Requirements

### Requirement: 上游原始负载（Payload）权威事实持久化
系统 SHALL 在每次订阅抓取成功后，在 `subscription_payloads` 表中持久化保存完整的未经修改的上游原始响应体（BLOB 格式，支持 gzip 压缩存储），并记录内容哈希、HTTP 状态码以及关键协商响应头（ETag、Last-Modified、Content-Type、Profile-Update-Interval）。系统 MUST 排除响应头中的敏感 Cookie 或 Authorization 信息。该原始响应仅供内部安全回放与审计对账，严禁对外网管理 API 暴露明文凭据。

#### Scenario: 抓取成功完整保存原始负载
- **WHEN** 订阅服务成功抓取到上游下发的响应内容
- **THEN** 系统在单个事务中插入 `subscription_fetches` 与对应的 `subscription_payloads`，记录正确的 content_digest 与 body BLOB

#### Scenario: 抓取异常保留存量资产
- **WHEN** 订阅源返回 HTTP 403 Forbidden 或连接超时
- **THEN** 系统在 `subscription_fetches` 中记录错误状态与失败原因，绝不清除或删除该订阅已持有的健康节点库存，保持历史 last-good 状态有效

### Requirement: 派生条目（Entries）、独立身份与严格公告伪节点分类器
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

#### Scenario: 同连接参数的多公告全部独立持久化
- **WHEN** 解析包含 3 个相同 `127.0.0.1:1` 连接参数但分别展示流量、到期、域名的条目
- **THEN** 系统在 `subscription_entries` 成功插入 3 条独立记录，均持有不同的 `id` 与 `ordinal`，无数据折叠

#### Scenario: 提示性流量伪节点满足已验证来源与全条件被归类为 notice
- **WHEN** 解析已验证订阅源（如 Dogegg）中包含 `Dogegg-剩余流量：81.9GB`（server: 127.0.0.1, port: 1, RFC 全零占位 UUID）的条目
- **THEN** 该条目在 `subscription_entries` 中被归类为 `notice`，不被插入 `nodes` 表，不派发探针拨测，且不出现在编译导出列表中

#### Scenario: 未验证来源即使满足四条件亦保留为 unknown 候选
- **WHEN** 解析尚未配置已验证来源规则的普通订阅中包含相同回环与全零 UUID 的提示性条目
- **THEN** 系统将其归类为 `unknown` 并保留，不自动标记为 notice，供用户显式纠偏

#### Scenario: 正常本地代理节点绝不被误判
- **WHEN** 解析 server 为 `127.0.0.1` 但 port 为 `1080` 且带有有效密码的 SOCKS5 节点
- **THEN** 该节点因未满足全套判定条件，被归类为 `proxy` 并正常入库无需额外审批

### Requirement: 条目级人工纠偏与跨刷新稳定继承
系统 SHALL 在 `subscription_entries` 支持条目级用户人工纠偏：
1. 包含 `user_kind_override`（`proxy`, `notice`, `unknown`）、`override_anchor`、`override_reason`、`override_at` 与 `actor_ref`；
2. 即使条目被归类为 `notice` 且 `node_logical_id = NULL`，用户仍可在条目级执行人工纠偏；
3. 跨刷新匹配优先级：1) 上游显式稳定 key；2) 有版本的分类器提取的明确 `semantic_slot`（如 `traffic_remaining`, `expiry_date`）；3) 上下文与内容 1:1 唯一精确匹配；
4. 若出现改名、新内容或同锚点歧义，系统 MUST NOT 强制继承或合并，必须保留全部条目并提供冲突诊断，不影响正常连接。

#### Scenario: 用户人工将公告条目纠偏为 proxy
- **WHEN** 用户对某特殊格式条目提交 `user_kind_override = 'proxy'`，随后该订阅触发刷新
- **THEN** 系统在新一轮条目派生时依据匹配规则自动继承该覆盖，将其作为代理节点入库

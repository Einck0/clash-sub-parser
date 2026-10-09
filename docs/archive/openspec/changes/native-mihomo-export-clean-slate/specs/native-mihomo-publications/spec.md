## Purpose

规定四目标真实发布、节点身份与可用编辑、安全重启和风险读模型，不以占位配置或虚假 UI 保存冒充成功。

## ADDED Requirements

### Requirement: 统一节点凭据与不可伪造端点绑定
系统 SHALL 保留 WireGuard 的 local_address/private_key/public_key/pre_shared_key/reserved/mtu/dns、TUIC 的 uuid/password/congestion_control/udp_relay_mode/alpn/sni/disable_sni、VLESS Reality 的 pbk/sid/fp/flow 与 Hysteria2 的 up/down/obfs/obfs-password；WG 缺 private_key/public_key/local_address 或 TUIC 缺 uuid/password SHALL 拒绝。YAML/URI SHALL 通过单一语义提取。编译和发布 SHALL 使用可信解析/持久节点身份与加密凭据的版本化绑定核对 protocol、logicalID、server、port 和身份 transport；不得仅凭待验 payload 自证身份，亦不得简单反算不完整 transport 导致真实 URL 被误拒。认证失败 SHALL 在任何目标导出前拒绝。

#### Scenario: 双格式合法输入及恶意端点
- **WHEN** 受支持 YAML/URI 的真实节点经过解析、保存、编译，与另一个保持相同 logicalID、protocol、version 却替换 payload server/port 的节点分别请求预览/发布
- **THEN** 合法节点均使用真实凭据导出，恶意载荷在预览/发布/重启分发均被拒绝，错误不暴露秘密

#### Scenario: 不完整必需字段或旧身份资料
- **WHEN** WG/TUIC 缺少必需字段，或旧凭据行缺少可验证身份绑定
- **THEN** 不伪造字段或根据密文自身补信任，明确拒绝并保持旧行原状

### Requirement: 四目标能力与规则资源 fail closed
系统 SHALL 提供 Mihomo、sing-box、Surge、Quantumult X。Mihomo SHALL 以真实参数渲染 SS/VMess/VLESS/Trojan/Hysteria2/WireGuard/TUIC、select/url-test/fallback/load-balance 与十四种受支持规则；sing-box SHALL 使用官方选项格式，Surge/QX SHALL 仅渲染各自可表达子集。不支持的节点/组/规则 SHALL 定点拒绝而非使用 logicalID、固定端口或假参数。Mihomo RULE-SET SHALL 仅引用显式有效 URL 或已配置的真实 provider，不得生成占位资源。

#### Scenario: 合法四目标与不支持组合
- **WHEN** 完整凭据和有效规则分别编译各目标或请求目标不支持组合
- **THEN** 可支持输出含真实连接/引用且接受相应官方校验，不可支持组合返回定位错误且不输出部分配置

#### Scenario: 缺失 RULE-SET provider
- **WHEN** Mihomo 规则只声明 RULE-SET 名字而不存在显式 URL 或已配置 provider
- **THEN** 编译拒绝并指出规则位置，输出绝不含 `ruleset.invalid` 或其他猜测 URL

### Requirement: 真实节点详情与有意义的编辑
系统 SHALL 在管理员详情接口从受认证凭据源提供真实非秘密连接字段和准确 secret-presence 布尔值；未认证列表/日志/错误/快照 SHALL 不暴露秘密，详情亦 SHALL 不返回私钥、PSK、密码。管理员编辑 SHALL 使用鉴权、CSRF、期望凭据版本 CAS、事务和脱敏审计；空秘密输入代表保留，明确轮换输入仅单向写入、不回显。改变 ID 所依赖的端点/transport SHALL 不得在旧 ID 下静默修改；源订阅 Reconcile 对同 ID 本地修改的覆盖语义 SHALL 明示并可验证；前端 SHALL 只有服务端确认后才显示已保存。

#### Scenario: 详情数据与失败状态
- **WHEN** WG/TUIC 详情有真实凭据或凭据缺失/校验失败
- **THEN** 只显示真正存在的可展示值及真实 secret-presence 状态；缺失显示 unavailable，不产生默认地址、公钥、UUID 或虚构 secret 状态

#### Scenario: 编辑、轮换、刷新与冲突
- **WHEN** 管理员持有效 CSRF 与 expected_credential_version 轮换秘密、尝试改身份端点、并在之后触发源订阅刷新；或两名管理员并发编辑
- **THEN** 轮换原子持久化且版本增加，旧版本冲突拒绝；改身份端点要求改订阅源并 Reconcile 为新 ID；刷新覆盖/冲突行为可见且审计脱敏，明文秘密不出现在返回、DOM 或日志

### Requirement: 四目标受保护发布与不可变持久恢复
系统 SHALL 在 Preview、Publish、ResolveAndServe 核对可信节点身份和凭据；保留 token/撤销/CSRF/风险预检。发布 SHALL 原子持久保存工件加密字节、内容摘要、目标、编译版本及版本化凭据绑定，重启下载 SHALL 核对认证和字节摘要，不以进程 map、前百条审计文本、当前订阅内容猜测原工件。缺绑定的历史发布 SHALL 拒绝分发而非假恢复。

#### Scenario: 重启、漂移、撤销与历史行
- **WHEN** 发布后重启、刷新源凭据或发生工件篡改，再访问有效 token/错误 token/撤销 token/旧无绑定记录
- **THEN** 仅真实保存且验证过的不可变工件可下载，内容摘要一致；错误凭据/被撤销/损坏/历史无绑定一律拒绝且不泄密

### Requirement: 风险读模型最新观测正确且有界
系统 SHALL 对策略允许的多个 provider 每节点选取按 observed_at DESC、id DESC 确定的唯一最新观测；分页总数 SHALL 是匹配的节点数而非观测 join 行数，风险过滤/状态与详情一致；分页查询 SHALL 不扫描每节点全部历史后在 Go 中选最大。

#### Scenario: 多 provider 同时刻与历史增长
- **WHEN** 一个节点有多个 provider 同时间戳记录，且其他节点有大量旧观测，按风险条件分页
- **THEN** 每节点只出现一次、count 等于实际匹配节点数、排序稳定且最新记录按 id 打破平局；查询计划使用限定节点/索引的最新观测路径

### Requirement: 删除旧兼容并完成隔离验收
系统 SHALL 不提供 legacy 导入/查看、旧 `/yaml/*`、`/script/*`、`/api/*` 专用 410、Clash/retired 重发入口、旧过滤别名或 skeleton 假回退；历史行不自动改写。端到端验收 SHALL 采用临时数据、官方客户端/规范验证及真实隔离实例，包含节点详情/编辑、预览、发布、下载、撤销和多视口交互；不得触生产。

#### Scenario: 清场与真实交互
- **WHEN** 客户端调用旧路径/目标或管理员在本地实例操作 WG/TUIC 编辑与四目标出版
- **THEN** 旧兼容不提供假成功，新流程仅真实可保存、可校验、可撤销；独立审查及黑盒验收通过前不宣称交付

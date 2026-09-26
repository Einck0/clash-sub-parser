## Purpose

彻底移除节点凭据与发布产物的 AEAD 加密、端点身份绑定、CAS 版本控制及前端掩码等过度工程化设计，使系统无需配置任何加密密钥即可直接以明文完成节点存储、探测、编译、预览、发布与管理编辑。

## ADDED Requirements

### Requirement: 无密钥启动与节点明文持久化契约
系统启动 SHALL 不依赖 `CSP_NODE_CREDENTIAL_KEY` 或任何加密主密钥环境变量，开箱即可执行订阅拉取、节点入库、主动探测、策略解析、配置预览与发布分发。SQLite 迁移 `000011` SHALL 在 `nodes` 表新增 `server TEXT NOT NULL DEFAULT ''`、`port INTEGER NOT NULL DEFAULT 0`、`config_json TEXT NOT NULL DEFAULT '{}'` 列，删除 `normalized_config_secret_ref` 与 `credential_version` 列，并删除 `node_credentials` 表；订阅解析与 `ReconcileSubscription` SHALL 直接将完整节点配置（`Server`、`Port`、`Credentials`）明文持久化到 `nodes` 表。

#### Scenario: 无加密密钥环境下启动与订阅拉取
- **WHEN** 系统在未设置 `CSP_NODE_CREDENTIAL_KEY` 的环境中启动并执行订阅拉取（`ReconcileSubscription`）
- **THEN** 服务正常启动且无密钥告警，订阅中的合法节点完整明文配置（含 `server`、`port` 及协议 `config_json`）直接写入 `nodes` 表，不产生 `HTTP 422 node credentials unavailable` 错误

#### Scenario: Schema 000011 迁移与旧表清理
- **WHEN** 在包含 `000001` 至 `000010` 迁移的 SQLite 数据库上执行 `000011` 迁移
- **THEN** `node_credentials` 表被删除，`nodes` 表具备 `server`、`port`、`config_json` 列且不再包含 `normalized_config_secret_ref` 与 `credential_version` 列，`publications` 表具备 `content` 列且不再包含 `artifact_key_id`、`artifact_nonce`、`artifact_ciphertext`、`credential_binding_digest`、`credential_bindings_json` 列

### Requirement: 直接基于快照节点的四目标编译与明文发布
策略解析生成的 `ResolvedNode` SHALL 直接包含节点的 `Server`、`Port` 与 `Credentials` 明文配置。四目标编译器（Mihomo、sing-box、Surge、Quantumult X）调用 `Compile(ctx, snapshot, target)` 时 SHALL 直接从 `snapshot.Nodes[i]` 读取明文配置，仅校验协议必填字段非空与格式合法（如 Shadowsocks 的 cipher 与 password、VMess/VLESS 的 uuid、Trojan/Hysteria2 的 password、WireGuard 的 private_key/public_key/local_address CIDR、TUIC 的 uuid 与 password），SHALL NOT 要求外部注入凭据映射或执行 `VerifiedNodeIdentity` 交叉核验。发布服务在 `Publish` 时 SHALL 将编译生成的明文 `Content` 直接存入 `publications` 表；`ResolveAndServe` 在校验导出 Token 与未撤销状态后 SHALL 直接返回 `pub.Content`（若旧记录 `Content` 为空则实时编译返回）。

#### Scenario: 直接从快照编译与明文发布下载
- **WHEN** 管理员针对包含合法明文节点的策略快照请求预览、创建发布并通过有效导出 Token 下载配置
- **THEN** 编译器直接从快照节点渲染目标配置，`publications` 表直接保存明文 `content`，客户端下载无需解密或凭据绑定校验即可获得完整配置文本

#### Scenario: 协议必填字段缺失被编译拒绝
- **WHEN** 快照中的节点缺少对应协议必填字段（如 Shadowsocks 缺少 cipher 或 password、WireGuard 缺少 private_key/public_key/local_address）并请求编译
- **THEN** 编译器返回明确的 `CapabilityError` 拒绝导出，不生成残缺配置

### Requirement: 无凭据版本门禁的安全探测与两级节点筛选
安全节点拨号器 `SafeNodeDialer` SHALL 在应用启动时无条件挂载，并直接从 `domain.Node` 读取明文 `Server`、`Port` 与 `Credentials` 建立临时 sing-box 探测客户端，继续强制公网 IP 校验、域名解析公网 IP 绑定、禁止 HTTP 重定向与严格 TLS 证书校验，SHALL NOT 查询凭据仓库或执行 AEAD 解密。周期探测调度与全局/策略组两级节点筛选 SHALL 直接依据节点激活状态与探测观测结论/延迟/新鲜度运行，SHALL NOT 因 `CredentialVersion <= 0` 跳过节点或因观测凭据版本不匹配而过滤观测记录。

#### Scenario: 无 Vault 环境下执行手动与周期探测
- **WHEN** 系统在无 Vault 依赖的情况下触发手动探测或周期探测任务
- **THEN** 激活节点不再因缺少 `CredentialVersion` 被跳过，`SafeNodeDialer` 直接使用 `domain.Node` 明文配置完成安全校验与拨号探测

#### Scenario: 探测观测直接参与节点两级筛选
- **WHEN** 全局筛选或策略组筛选配置了 `probe_verdict` 或 `probe_latency_ms` 条件且节点存在新鲜观测记录
- **THEN** 筛选器直接按观测新鲜度、结论与延迟判定节点是否匹配，不再检查 `HasValidCredentialVersion`

### Requirement: 管理端节点、预览与订阅明文直读直编
管理端节点详情接口 `GET /api/v1/nodes/{logical_id}` SHALL 直接返回完整 `domain.Node` 明文信息（含 `server`、`port`、`credentials`）；节点更新接口 `PATCH /api/v1/nodes/{logical_id}` SHALL 直接接收并更新 `nodes` 表的明文字段（`display_name`、`server`、`port`、`credentials`），SHALL NOT 要求 `expected_credential_version` CAS 版本号、Write-Only 输入字段或拦截 `server`/`port` 修改。前端节点详情视图 SHALL 直接展示并编辑明文字段，移除 `***` 掩码、Write-Only 轮换输入、CAS 版本号、Vault AEAD 徽章与 Unavailable 横幅；发布预览视图 SHALL 直接展示真实配置文本而不调用 `redactPreviewSecrets`；订阅接口与前端订阅抽屉 SHALL 直接返回并回显真实订阅 URL 而不使用 `***` 占位。

#### Scenario: 管理端查看与直接编辑节点明文配置
- **WHEN** 管理员打开节点详情抽屉查看配置并修改 `display_name`、`server`、`port` 或协议凭据字段后保存
- **THEN** `GET /api/v1/nodes/{logical_id}` 直接回显明文配置，`PATCH /api/v1/nodes/{logical_id}` 直接更新 `nodes` 表并返回更新后的完整节点明文信息，界面无 `***` 掩码或 CAS 版本冲突阻断

#### Scenario: 发布预览与订阅编辑展示真实明文内容
- **WHEN** 管理员在发布页预览编译配置或打开已有订阅的编辑抽屉
- **THEN** 发布预览直接显示包含真实密码/私钥的完整客户端配置文本，订阅编辑抽屉直接显示真实 `source_url_secret_ref` URL

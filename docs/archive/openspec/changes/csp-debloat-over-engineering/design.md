## Context

当前代码库围绕 `NodeCredentialVault`（AES-256-GCM）构建了繁重的加密与防篡改体系：`node_credentials` 独立密文表、`nodes.credential_version` CAS 版本号、`VerifiedNodeIdentity` 端点身份 HMAC/绑定、`SafeNodeConnection` Write-Only 脱敏投影、`publications` 表二次 AEAD 加密（`artifact_ciphertext` + `credential_bindings_json`）、`probe_observations.credential_version` 过滤门禁以及前端 `***` 掩码与 `redactPreviewSecrets`。线上因未配置 `CSP_NODE_CREDENTIAL_KEY` 导致 `vault=nil`，触发 `HTTP 422 node credentials unavailable: vault or credential repository not configured`。遵循用户“去掉过度工程化设计，给系统做一次减法”的明确指令（参见 `proposal.md`），本设计将节点配置与发布产物彻底回归 SQLite 明文直存，消除全部密钥、密文表与身份版本绑定开销。

## Goals / Non-Goals

**Goals:**
- 彻底移除 `NodeCredentialVault`、`node_credentials` 表、`VerifiedNodeIdentity`、`SafeNodeConnection`、`CredentialVersion` CAS 与 `CSP_NODE_CREDENTIAL_KEY`，服务零密钥启动即可完整工作。
- `nodes` 表直接持久化 `server`、`port`、`config_json`（序列化 `InboundProtocolCredential`）；`publications` 表直接持久化 `content`（编译后的明文配置字节）。
- 冻结精简后的 `domain` → `parser` → `resolver` → `compiler` 共享契约，使后续 Repository/Inventory、Publication、Probe、HTTP Transport、Web 前端与进程入口 6 个正交特性包可按不重叠写集合并发施工。
- 保留管理员鉴权（Admin Token / Session / CSRF）、导出 Token 鉴权与撤销、以及 `SafeNodeDialer` 的 SSRF 防护（公网 IP 校验、DNS 解析 IP 绑定、禁止重定向与严格 TLS 证书校验）。

**Non-Goals:**
- 不改变四目标编译器（Mihomo、sing-box、Surge、Quantumult X）的协议渲染格式、策略图解析拓扑或 IP 风险评估算法。
- 不在本 Change 执行期间触碰正在运行的生产容器或线上生产数据库（生产迁移留待交工后按备份与 `ReconcileSubscription` 流程执行）。

## Decisions

### 1. SQLite Schema `000011` 明文化减法迁移
- **决策**：新增 `migrations/000011_plaintext_nodes_and_publications.sql`（复用 `modernc.org/sqlite` 原生支持的 `ALTER TABLE ... ADD COLUMN` / `DROP COLUMN`）：
  1. `nodes` 表：`ADD COLUMN server TEXT NOT NULL DEFAULT ''`、`ADD COLUMN port INTEGER NOT NULL DEFAULT 0`、`ADD COLUMN config_json TEXT NOT NULL DEFAULT '{}'`；`DROP COLUMN normalized_config_secret_ref`、`DROP COLUMN credential_version`。
  2. `publications` 表：`ADD COLUMN content BLOB NOT NULL DEFAULT X''`；`DROP COLUMN artifact_key_id`、`DROP COLUMN artifact_nonce`、`DROP COLUMN artifact_ciphertext`、`DROP COLUMN credential_binding_digest`、`DROP COLUMN credential_bindings_json`。
  3. 执行 `DROP TABLE IF EXISTS node_credentials;`。
- **备选方案**：新建 `nodes_v2` / `publications_v2` 重建表并拷贝数据。因现有索引均不引用被删列，直接使用 `ALTER TABLE` 原地增删列更简洁且不破坏外键级联。

### 2. Domain / Parser / Resolver / Compiler 共享契约重建（Phase 1 串行基座）
- **决策**：
  - **Domain**：
    - 删除 `internal/domain/node_identity.go`、`node_identity_test.go`、`node_connection.go`（含相关测试）。
    - 精简 `internal/domain/node_credential.go`：删除 `DefaultKeyID`、`EnvNodeCredentialMasterKey`、`NodeCredentialVault`、`NodeCredentialRecord`、`NewNodeCredentialVault`、`NewNodeCredentialVaultFromEnv`、`Encrypt`、`Decrypt`；保留 `InboundProtocolCredential` 及其辅助函数（`EffectivePreSharedKey`、`IsTruthy`、`HasInsecureTransport`、`ExtractHy2Ports`、`HasTUICDisableSNI`）。
    - 精简 `internal/domain/publication.go`：删除 `PublicationCredentialBinding`、`ComputeCredentialBindingDigest`、`ParseAndVerifyCredentialBindings`、`PublicationArtifactAAD`、`EncryptPublicationArtifact`、`DecryptPublicationArtifact`；`domain.Publication` 直接持有 `Content []byte`，删除加密与绑定相关字段。
    - 精简 `domain.Node`：直接持有 `Server string`、`Port int`、`Credentials InboundProtocolCredential`；删除 `NormalizedConfigSecretRef`、`CredentialVersion`、`Identity`。
    - 精简 `internal/domain/ports.go`：删除 `NodeCredentialRepository` 接口。
    - 精简 `internal/domain/probe.go` 与 `node_filter.go`：删除 `HasValidCredentialVersion` 及 `MatchesCondition` 中基于 `CredentialVersion` 的过滤检查。
  - **Parser**：删除 `parser.go` 中的 `opaqueSecretRef`、`yamlSecrets`、`urlSecrets`；`Parse` / `ExtractWithCredentials` 直接产出完整 `domain.Node`（填充 `Server`、`Port`、`Credentials`）。
  - **Resolver**：`resolver.ResolvedNode` 直接包含 `Server string`、`Port int`、`Credentials domain.InboundProtocolCredential`，删除 `CredentialVersion` 与 `Identity`；`digest.go` 移除观测 `CredentialVersion` 摘要耦合。
  - **Compiler**：删除 `compiler.WithCredentials`、`compiler.WithCredentialInputs`、`CompileOptions.Credentials`、`NodeCredentialInput`；`Compile(ctx, snapshot, target)` 直接从 `snapshot.Nodes[i]` 读取明文节点配置；`validateCredentialEnvelope` 仅校验 `Server`/`Port` 合法及各协议必填字段非空（如 SS 的 cipher+password、WG 的 private_key+public_key+local_address CIDR、TUIC 的 uuid+password），删除全部 `VerifiedNodeIdentity` 交叉核验；`CompileMihomo(ctx, snapshot)` 保留为无需 `credentials` 参数的快捷入口（同步简化 `internal/probe/singbox` 的导出/配置构建入参）。

### 3. Repository、Inventory、Publication、Probe 与 Transport/Web 并发减法（Phase 2 互斥写集）
- **决策**：在 Phase 1 共享契约编译通过后，按互不重叠的写集合切分为 6 个特性包并发推进：
  1. **Repository & Inventory**（`internal/repository/sqlite/{nodes,node_credentials,revisions,node_risk,probes,*}`、`internal/application/inventory/**`）：删除 `sqlite/node_credentials.go` 及加密产物/版本专属测试；`sqlite/nodes.go`（及 `node_risk.go` 中的节点列扫描）直接读写 `server`、`port`、`config_json`；`sqlite/revisions.go` 直接读写 `publications.content`；`inventory/service.go` 的 `ReconcileSubscription` 直接将解析出的 `domain.Node` 写入 `nodes` 表（约 30 行替换原 ~250 行加解密事务），删除 `WithCredentialVault`、`credentialTransactionRepository`、`projectNodeConnection`、`verifyCredentialIdentity`，将节点更新简化为直接修改 `nodes` 表明文字段，`GetNodeDetailWithRisk` 直接返回自带完整配置的 `domain.Node`。
  2. **Publication Service**（`internal/application/publication/**`）：删除 `vault`/`credRepo` 依赖注入、`resolveCredentials`（~100 行）与 `verifyCredentialBindings`（~95 行）；`Publish` 编译后直接将明文 `Content` 写入 `publications` 表；`ResolveAndServe` 校验 Token 与未撤销后直接返回 `pub.Content`（若历史记录 `Content` 为空则实时编译）。
  3. **Probe Service**（`internal/application/probe/**`）：`NewSafeNodeDialer(opts ...SafeNodeDialerOptions)` 直接从 `domain.Node` 读取明文配置，删除查库与解密逻辑（~60 行）；`periodic.go` 删除 `CredentialVersion <= 0` 跳过逻辑；`runner.go` 无条件使用 `SafeNodeDialer`。
  4. **HTTP Transport**（`internal/transport/http/{nodes,publications,subscriptions}*.go`）：`GET /api/v1/nodes/{id}` 直接返回完整节点信息；`PATCH /api/v1/nodes/{id}`（同时兼容 `/nodes/{id}/connection` 或统一至 `/nodes/{id}`）直接接收并更新明文字段；`publications.go` 删除 `ErrIntegrityCheckFailed` 映射；`subscriptions.go` 移除 `***` 脱敏保留判断，直接读写明文 URL。
  5. **Web Frontend**（`web/src/**`）：节点详情删除 `***` 掩码、Write-Only 轮换、CAS 版本号、Vault AEAD 徽章与 Unavailable 横幅，直接展示并编辑明文字段；发布预览删除 `redactPreviewSecrets`，直接展示真实配置文本；订阅编辑直接回显真实 URL，去掉 `***` 占位。
  6. **Entrypoint & Compose**（`cmd/csp/main*.go`、`docker-compose.yml`）：删除 `CSP_NODE_CREDENTIAL_KEY` 读取與告警、删除 `NodeCredentialRepository` 与 `WithCredentialVault` 初始化、无条件挂载 `SafeNodeDialer`。

## Risks / Trade-offs

- **[Risk] 管理端接口直接返回节点密码/私钥与订阅 URL** → **Mitigation**：`/api/v1/*` 管理接口受 Admin Token / Session 鉴权与写操作 CSRF 保护，且个人自建控制面管理员本就是配置拥有者；客户端分发接口 `/publish/v1/{id}` 本就输出完整代理配置。
- **[Risk] 既有测试文件（如 `internal/integration/`、`sqlite/`、`http/`）大量引用 `NodeCredentialVault` 与 `CredentialVersion`** → **Mitigation**：在对应特性包与汇聚阶段同步清理测试中的 Vault/密文构造辅助函数，直接构造含 `Server`、`Port`、`Credentials` 的 `domain.Node`。
- **[Risk] 前端构建产物意外污染 `internal/webassets/dist`** → **Mitigation**：前端验证与隔离预览严格设置 `NO_COPY_WEBASSETS=1` 并使用临时 `--outDir`，校验 `internal/webassets/dist` 文件哈希前后完全一致。

## Migration Plan

1. **隔离开发与验证**：本 Change 全程在本地工作区与隔离临时 SQLite 数据库上验证 `000011` 迁移、单元测试、竞态测试与隔离预览实例，严禁操作线上容器或 `/data/csp-v1.db`。
2. **生产数据迁移说明（仅记录，不在本 Change 中执行）**：
   - 生产库当前 `node_credentials` 仅 3 行，963 个节点绝大部分无加密凭据；升级包含 `000011` 迁移的新版本后，无需解密旧表，直接触发所有订阅重新拉取（`ReconcileSubscription`）即可自动重填 `nodes` 表的 `server`、`port`、`config_json` 明文字段。
   - 生产库完整热备份已保存在 `/home/service/backups/csp-v1-backup-20260926_152446.db`。
   - 含完整明文节点配置的冷归档备份位于 `/home/service/backups/csp-legacy-cold-archive-20260919.db`，可随时作为回退或离线核对基准。

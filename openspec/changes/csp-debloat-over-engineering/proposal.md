## Why

线上环境因未配置 `CSP_NODE_CREDENTIAL_KEY` 导致 `vault=nil`，在订阅拉取、节点详情、安全探测与预览发布全链路抛出 `HTTP 422 node credentials unavailable: vault or credential repository not configured`。用户明确指出“这个K是什么？是我登录的key吗”、“话说为什么要加密”并指令**“去掉过度工程化设计，给系统做一次减法”**：个人/自建订阅解析器根本不需要也不理解 `NodeCredentialVault` AES-256-GCM 加密、版本化 CAS 轮换、`VerifiedNodeIdentity` 端点身份绑定、发布产物二次 AEAD 加密与前端 `***` 掩码体系。

## What Changes

- **BREAKING**：彻底移除 `NodeCredentialVault` AES-256-GCM 加密体系、`CSP_NODE_CREDENTIAL_KEY` 环境变量依赖、`node_credentials` 表、`VerifiedNodeIdentity` 端点身份 HMAC/绑定、`SafeNodeConnection` Write-Only 掩码投影以及 `CredentialVersion` CAS 版本控制；系统启动无需配置任何加密密钥即可完整运行。
- **BREAKING**：新增 SQLite 迁移 `000011_plaintext_nodes_and_publications.sql`，在 `nodes` 表直接存储明文 `server`、`port`、`config_json`（删除 `normalized_config_secret_ref` 与 `credential_version` 列），在 `publications` 表直接存储明文 `content`（删除 `artifact_key_id`、`artifact_nonce`、`artifact_ciphertext`、`credential_binding_digest`、`credential_bindings_json` 列），并 `DROP TABLE IF EXISTS node_credentials`。
- 精简 `domain`、`parser`、`resolver`、`compiler` 共享契约：`domain.Node` 与 `resolver.ResolvedNode` 直接持有 `Server`、`Port`、`Credentials`（`InboundProtocolCredential`）；`compiler.Compile` 直接从 `snapshot.Nodes[i]` 读取节点明文配置，删除 `WithCredentials` / `WithCredentialInputs` 与身份交叉核验，仅校验各协议必填字段非空。
- 精简 `repository/sqlite`、`inventory`、`publication`、`probe` 与 `transport/http`：订阅 `ReconcileSubscription` 直接将解析结果写入 `nodes` 表；`Publish` 直接持久化明文 `Content` 至 `publications` 表，`ResolveAndServe` 校验 Token 与未撤销状态后直接返回明文 `Content`；`SafeNodeDialer` 直接从 `domain.Node` 读取明文配置且无条件挂载，周期探测不再按 `CredentialVersion <= 0` 跳过；节点详情与更新接口（`GET /api/v1/nodes/{id}`、`PATCH /api/v1/nodes/{id}`）直接返回与更新明文字段，订阅接口直接返回真实 URL。
- 精简前端 `web/src/**`：节点详情页移除 `***` 掩码、Write-Only 轮换输入、CAS 版本号、Vault AEAD 徽章与 Unavailable 横幅，直接展示并编辑明文配置；发布预览页移除 `redactPreviewSecrets` 直接展示真实配置文本；订阅编辑抽屉移除 `***` 占位直接回显真实订阅 URL。

## Capabilities

### New Capabilities

- `debloat`: 移除节点凭据与发布产物 AEAD 加密、身份绑定、CAS 版本号及前端掩码，改为 SQLite 明文直存、直编、直发与明文可编辑管理契约。

### Modified Capabilities

无（当前 `openspec/specs/` 下无已归档主规格，本次通过 `debloat` 能力规格建立完整减法契约）。

## Impact

`migrations/000011_*.sql`、`internal/domain/`、`internal/parser/`、`internal/resolver/`、`internal/compiler/`、`internal/probe/singbox/`、`internal/repository/sqlite/`、`internal/application/{inventory,publication,probe,subscription}/`、`internal/transport/http/`、`internal/integration/`、`cmd/csp/`、`docker-compose.yml` 及 `web/src/features/{nodes,publications,subscriptions}/`。本阶段仅在隔离环境完成实现与验收，不触碰生产容器与线上数据库，生产库迁移通过备份保障并在升级后触发 `ReconcileSubscription` 自动重填明文节点配置。

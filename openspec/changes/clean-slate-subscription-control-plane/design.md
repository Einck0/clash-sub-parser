## Context

旧 Compose 定义单一 `app` 服务、`backend-data` 卷和 SQLite `/data` 数据库；`backend/app/main.py` 注册 `/api` 下的 11 组 CRUD Router、`/api/v2`、`/yaml`、`/script` 以及把未知路径回退到旧 SPA 的 catch-all。新方案必须证明它不接触这些边界。当前未提交的重构文件是 WIP，只可作为已脱敏行为参考。

## Goals / Non-Goals

**Goals:** 见 proposal；本设计将物理隔离、运行期授权、可移植 Bundle、秘密边界、发布和确认门冻结成可实施契约

**Non-Goals:** 不复用旧代码或部署；不做双写、影子读、影子编译或请求翻译；不把产品内来源/Profile 的只读网络授权解释为 Hermes 的自主出站许可

## Decisions

### D1. 固定物理隔离拓扑

新根为 `control-plane/`。生产形态固定使用两个新 Compose 服务：`csp-control-plane` 与 `csp-control-plane-db`；数据库为新 PostgreSQL 实例、数据库名 `csp_control_plane`、专用数据库角色、命名卷 `csp-control-plane-db-data`、私有网络 `csp-control-plane-net`。应用只连接该数据库服务；数据库不发布宿主端口，旧服务不加入该网络。

新镜像仅从 `control-plane/` 构建上下文复制文件，允许基础镜像和新根依赖；禁止旧 Dockerfile、`backend/`、`frontend/`、`backend-data`、旧 SQLite 路径、旧容器网络别名、旧环境前缀和任何旧镜像层。新凭据仅由部署期 secret 注入，不能在镜像、构建参数、卷、日志或诊断中出现。实现验证同时做静态构建上下文清单、镜像层、Inspect 挂载/网络/环境名和运行时数据库连接审计；任一发现旧对象即失败。独立 schema 被拒绝，因为旧单服务 SQLite 卷说明 schema 级隔离不足以证明物理切断。

### D2. 双层网络授权

产品层只允许两类预期只读外部操作：已保存且 `enabled` 的 Source 可按自身 fetch budget 刷新，已保存且 `enabled` 的 ProbeProfile 可按自身 probe budget 创建或周期调度 Job。授权记录必须有 logical ID、配置 revision、允许操作、状态、目标类别、并发/字节/时长/周期上限与审计版本；Job 在派发每个网络尝试前重新检查当前授权和剩余预算。

禁用、删除、取消、授权版本变更或预算耗尽立即阻止新尝试，排队 Job 终态为 `authorization_revoked` 或 `budget_exhausted`；在途请求仅可收尾/取消，不得继续下一次尝试或产生新修订。Source 刷新只在完整解析和预算未耗尽后原子发布；Profile 调度不得绕过 Job 和预算。来源保存、Profile 保存或启用只是操作员对产品行为的持久授权，不是生产确认门。

Hermes 层保持更严边界：本 change 的规划、实现和验证仅使用 fixtures、独立环境或已获用户确认的必要操作；不得因产品契约自行发起非必要出站或生产外部写入。生产/外部写入仍走 D6 确认门。

### D3. Portable Bundle 与本地 rebind

Bundle 只序列化 schema 版本、策略图、规则、DNS、发布选项和带顺序/能力语义的 `selector_slot`。slot 不含 InventoryNode logical ID、来源 logical ID、URL、节点 payload 或任何凭据。导入在事务内创建 `UNBOUND` 草稿；语法、循环和禁止字段可立即检查，但不得把未知 inventory 当成可发布配置。

目的实例的操作者必须创建本地 `RebindDraft`，将每个 slot 显式绑定为该实例的可审计库存选择器（本地 InventoryNode logical ID 集合或本地库存查询）；绑定不进入 Bundle。预检要求全部 slot 已绑定、绑定目标存在、编译成功且所需节点集合非空；否则返回 slot 级诊断、不创建部分绑定、不改变活动修订。跨空实例导入、审查 rebind、预检、发布的验收必须证明：导入零节点/零秘密，未 rebind 不能发布，重绑后只有目的实例库存进入输出。

### D4. 节点规范化、指纹与本地秘密边界

每个协议 adapter 明确列出可公开规范化字段（协议、规范化主机/IP、端口、传输与 TLS 公共选项）和 secret 字段（认证 UUID/密码/PSK、私钥、短 ID、Authorization/订阅认证值及协议声明的其他机密字段）。公开字段采用版本化、字段排序的 canonical JSON；主机小写、IP 标准化、端口数值化、无语义顺序集合排序。未知会影响连接的字段拒绝，不静默忽略。

完整规范化连接材料只在解析内存及本地 `EncryptedSecret` 中出现。`EncryptedSecret` 使用每记录随机 nonce 的 AEAD、key id 和部署期注入的密钥；密钥、明文、密文和 nonce 不进入 API、日志、Bundle、审计详情或诊断。库存去重键为 `HMAC-SHA-256(fingerprint-version || protocol || canonical-public-fields || canonical-secret-fields)`；所以同一语义 payload 稳定归并，而同一地址不同凭据或不同协议绝不归并。只保存指纹版本和不可逆摘要。fixtures 必须覆盖等价重排、协议差异、同端点不同密码/UUID/私钥和脱敏响应。

### D5. 发布契约与秘密最小披露

发布 URL 固定为 `GET /publish/{target}`，其中 target 仅为 `clash`、`mihomo`、`stash`、`shadowrocket`。调用者通过专用 Bearer 发布令牌认证；令牌以不可逆 hash 存储，带单目标 scope、expiry、撤销状态和 logical ID，原始 token 只在创建时向操作员显示一次。发布令牌不能访问 `/api/control`。

服务端纯编译缓存键为活动 revision logical ID、target 和语义 fingerprint，绝不以 token 原文作键，响应固定 `Cache-Control: private, no-store`。成功输出只含该 target 当前已发布 revision 所选节点的客户端必需连接材料及 revision/fingerprint 元数据；不得含未选节点、来源 URL、来源认证、管理认证、Probe 原始数据、Bundle 或管理/发布 token。管理 API 仅返回节点公开摘要与 `secret_present`，日志仅记录方法、路径模板、target、status、request id 和 token logical ID 摘要。旧路径、未知 target 与 Script.js 一律 404、无 Location、无编译、无 DB/Job 副作用。

### D6. 可消费确认门

所有生产初始化、离线导入、入口切换、旧容器停服及外部写入由执行器在动作前验证 `ExecutionConfirmation`。记录绑定 operator subject、environment=`production`、action enum、不可变 target descriptor、target digest、issued/expiry（15 分钟）、状态、消费时间和审计 correlation id。确认只能由操作员针对预览的精确 action/target 创建；一个确认只能成功消费一次。

执行器在任何副作用前原子比较 action、environment、target digest、expiry 和未消费状态。缺失、错误目标、过期、已消费或拒绝均返回 `confirmation_required` 且零外部/生产副作用；成功时原子消费并记录不含秘密的审计事件。每种受门动作都有正向、缺失、错误目标、过期和重复消费测试。产品内只读 fetch/probe 由 D2 管理，不伪装为确认门绕过。

### D7. 可再生 legacy deny manifest

实现开始时从干净的旧基线以 FastAPI 路由表、`main.py` include 顺序、每个 router decorator、生成/下载/快照路由和 `frontend/src/router/index.ts` 生成版本化 `legacy-deny-manifest.json`；生成脚本自身在新根且只读取受控 legacy-reference fixture，不让新服务导入旧应用。manifest 保存 `(method,path)` 与 SPA path，不保存请求体、响应或秘密，并由测试固定 hash 防漏项。

本次已知 API 前缀为 `/api`，router 根必须完整覆盖 `/subscriptions`、`/node-groups`、`/probe`、`/proxy-chains`、`/rule-categories`、`/rules`、`/dns`、`/generate`、`/downloads`、`/settings`、`/snapshots`，以及全部 `/api/v2/**`；根级 `/yaml`、`/script`、`/generate/**` 的 yaml/script/current/download、所有 `/downloads/**`、`/snapshots/**` 均在清单。旧 SPA 路径为 `/`、`/node-groups`、`/proxy-chains`、`/nodes`、`/rules`、`/rules/category/:name`、`/dns`、`/generate`、`/settings`、`/history`。新服务对清单中每个方法/路径和 SPA path 均断言 404、无 Location、无兼容生成、无数据库写入、无 Job、无网络调用；每次旧基线 router 或前端路由变更必须重新生成 manifest，hash 差异使 CI 失败直到人工审查。

### D8. 迁移与回滚

先在独立空数据库和私有网络完成依赖、镜像、挂载、网络、Alembic、恢复、授权、rebind、发布与 deny-list 演练。生产初始化、可选离线导入、入口切换、停旧容器分别要求 D6 独立确认。入口切换失败保持或恢复旧流量；新库保留仅供排查，不读写旧库。旧容器只在确认后停止，旧恢复备份只读且永不挂载给新服务。

## Risks / Trade-offs

- [完全不兼容] → 默认空库、portable Bundle 只做草稿、显式 rebind
- [新 PostgreSQL 服务增加运维成本] → 换取可检查的数据库/卷/网络切断
- [节点输出必然携带客户端连接材料] → 仅限授权发布 endpoint 和已选目标节点；其余接口严格脱敏
- [定期网络任务失控] → D2 授权版本、预算、逐尝试检查与取消语义
- [legacy 路由后续新增导致漏拦] → 生成 manifest hash 成为 CI 门禁

## Open Questions

无

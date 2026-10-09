## Context

参见 `proposal.md` 与 `specs/audit-and-defect-remediation/spec.md`。
在当前 CSP 1.0 代码库（分支 `dev` 基线 `fe4654b`）中，系统由 Go 1.24 后端与 Vue 3 / Vite 前端构成，静态资源通过 Go `embed` 打包为单一可执行文件。
本次变更源于三轮独立代码走查和运行时审计，重点解决影响正常生命周期使用、并发稳定性与端到端质检的真实缺陷。
特别约束：
1. 宿主机 17000 端口为线上 Docker 服务的映射端口别名（生产端口为 18080 与 17000），任何测试与预览严禁向该端口发起网络请求或写入测试数据；
2. 保持严格安全不变量：SSR 保护、公网 IP 校验、私网隔离拦截、发布 Token 鉴权以及敏感凭据脱敏均不可降级；
3. 不进行未经论证的无端重构或投机性抽象（如非必要的 N+1 复杂缓存层、无调用的多余库等）；
4. 排除追踪的已生成静态产物（`internal/webassets/dist`）的非必要修改，直至最终门禁统一打包。

## Goals / Non-Goals

**Goals:**
- 在后端服务启动时自动引导初始活跃配置修订（Configuration Revision），彻底消除冷启动下添加规则报 422 `missing_revision_id` 与发布预览报 409 `no_active_revision` 的死锁问题；
- 开放规则删除 REST 端点及前端集成，补齐策略管理生命周期闭环；
- 解决 `safeRuntimeConn` 在阻塞 Read 时执行 Close 出现的互斥锁死锁；
- 精准化拨号器错误归类：DNS 失败与私网目标拒绝仅作为单节点不可用证据，不再使整个探测批次标记为失败；
- 修复节点观测历史接口的分页 SQL 逻辑与真实总数计数；
- 订阅删除时原子级联停用无所属来源的孤儿节点；
- 修复 PATCH 订阅空白凭据引用的传输层静默绕过缺陷；
- 前端补齐 Cookie 会话下的 CSRF 请求头投递、局域网 HTTP 下的 UUID 退避生成算法；
- 消除前端 Probes 视图高频扫描风暴与 Nodes 视图无限滚动饥饿；
- 修复抽屉校验绕过、策略路径编码与发布链接刷新丢失 Token 缺陷；
- 修正 `.env.example` 遗留 Python 变量，对齐 Go CSP 1.0 环境变量；
- 修复 Playwright E2E 真实服务器测试中文环境语言断言冲突，巩固 17000 生产端口隔离纪律。

**Non-Goals:**
- 不重写或颠覆既有的策略图拓扑校验引擎与求解算法；
- 不引入重型外部 Redis 或分布式队列，维持单进程本地 SQLite 存储；
- 不进行无关视图的激进 UI/UX 重设计；
- 不放宽私网地址拦截规则或绕过 SSRF 防御体系。

## Decisions

### 1. 配置修订引导与规则自动归属方案
- **决策**：
  - 在 `cmd/csp/main.go` 启动流水线中，初始化 `policyService` 与 `revisionService` 后，调用 `policyService.EnsureActiveRevision(startupCtx)`。若当前数据库无活跃修订且修订总数为 0，则自动以当前有效（默认空白）策略图状态计算 digest 并创建激活一条初始修订；
  - 在 `internal/application/policy/service.go` 的 `CreateAdmissionRule` 与 `CreatePolicyRule` 中，若客户端未传递 `revision_id`，优先尝试 `GetActive`；若此时仍无活跃修订，则在同一事务锁内自动引导并绑定该初始修订；
  - 在 `internal/transport/http/revisions.go` 注册 `POST /api/v1/revisions`，支持显式生成新草稿/活跃修订。
- **替代方案考虑**：
  - *仅在前端强制要求用户手动点击“新建版本”*：用户体验极差，新手在首次部署后看到的是莫名其妙的 422 报错，违背零配置可用原则；
  - *运行时彻底丢弃 revision 表*：违背 CSP 1.0 配置幂等性与不可变版本快照设计初衷。

### 2. `safeRuntimeConn` 关闭防死锁设计
- **决策**：
  - 传统 `net.Conn` 在另一个协程阻塞于 `Read()` 时调用 `Close()`，正常情况下底层系统调用套接字关闭会使阻塞的 `Read()` 立即返回 `net.ErrClosed`。但在 sing-box 的分层封装结构中，`safeRuntimeConn` 内部使用 `readCloseMu` 将 `Read` 与 `Close` 强行互斥：
    ```go
    func (c *safeRuntimeConn) Read(p []byte) (int, error) {
        c.readCloseMu.Lock()
        defer c.readCloseMu.Unlock()
        return c.Conn.Read(p)
    }
    ```
    当 `c.Conn.Read()` 阻塞等待网络数据时，`readCloseMu` 被长期持有，此时外界调用 `Close()` 试图获取 `readCloseMu.Lock()` 即产生死锁。
  - **解决方案**：
    - 在 `Close()` 执行加锁之前，先主动调用 `c.Conn.SetDeadline(time.Now())`（以及递归遍历所有 `upstreamProvider` 收集的底座网络连接并设置立即超时），强制唤醒并退出阻塞中的 `Read()` 调用栈；
    - 遍历并优先关闭最底层真实物理套接字，使应用层解密/解协议 reader 迅速退出并释放 `readCloseMu`；
    - 若连接未嵌套额外协议层（`len(upstreams) == 0`），则直接调用底层 `c.Conn.Close()`，无需在 `readCloseMu` 上无谓排队。
- **替代方案考虑**：
  - *完全移除互斥锁*：部分 sing-box AEAD 协议栈在并发 Read 与 Close 时可能存在 data race；使用“先超时解阻塞 + 再加锁释放协议资源”能兼顾无死锁与无竞态。

### 3. 安全拨号器错误解耦与节点级隔离
- **决策**：
  - 在 `internal/application/probe/safe_dialer.go` 中，定义明确的独立错误：`ErrTargetUnresolvable`（域名 DNS 解析失败/无公网 IP）与 `ErrPrivateTargetRejected`（目标被 SSRF 拦截），不再对这两种情况包裹 `ErrCredentialsUnavailable`；
  - 仅在节点真正缺失认证凭据（如空 password/uuid）或 dialer 未配置时包裹 `ErrCredentialsUnavailable` / `ErrProbeDialingNotConfigured`；
  - 在 `internal/application/probe/runner.go` 中，`isFatalDialError` 仅对真正的系统级凭据配置缺失生效；对于 DNS 与私网错误，单节点记录为不可用（`VerdictUnavailable`），不设置 `hasFatalDialErr`，允许批次其余健康节点正常完成 Phase 2 探测。
- **替代方案考虑**：
  - *修改 runner 忽略所有 dial error*：会导致测试凭据配置完全失效时探测依然静默跑完，降低了系统配置错误的可发现性。

### 4. 订阅删除时孤儿节点去活机制
- **决策**：
  - 在 `internal/repository/sqlite/subscriptions.go` 的 `Delete` 方法中，使用原子事务：
    1. `DELETE FROM subscriptions WHERE id = ?;`（外键级联清理 `node_sources`）；
    2. `UPDATE nodes SET active = 0, updated_at = ? WHERE active = 1 AND NOT EXISTS (SELECT 1 FROM node_sources WHERE node_sources.node_logical_id = nodes.logical_id);`
  - 确保失去全部订阅来源的孤儿节点立即标记为 `active = 0`，与 `inventory.Service.Reconcile` 的孤儿节点去活语义保持 100% 一致。
- **替代方案考虑**：
  - *物理硬删除节点*：可能破坏已有关联的历史观测外键记录或审计痕迹；采用软去活（`active = 0`）最安全且保留审计证据链。

### 5. 前端 CSRF 守卫、UUID 兼容与视口防饥饿设计
- **决策**：
  - **CSRF 注入**：`ApiClient` 维护内存中的 `csrfToken` 状态，`useAuth.login` 在成功获取响应时提取并调用 `api.setCsrfToken(res.csrf_token)`，在发起 POST/PUT/PATCH/DELETE 等请求时统一注入 `X-CSRF-Token` 请求头；
  - **UUID 退避**：封装通用 `generateUUID()` 工具函数，优先使用 `crypto.randomUUID()`，若检测到未定义（纯 HTTP 局域网）则平滑退避至符合 RFC 4122 v4 标准的伪随机算法；
  - **轮询风暴规避**：`ProbesView.vue` 轮询定时器仅对活跃状态（`runs`、`poolStatus`、`observations`）做必要刷新；全量节点拉取仅在首次挂载、任务状态由 running 变为结束或手动点击刷新时执行；
  - **视口无限滚动防饥饿**：在 `NodesView.vue` 中，当应用了客户端健康度过滤且 `filteredItems.length` 不足填满可视区域高度（如低于 10 个且 `hasMore.value` 为真）时，通过侦听或加载回调自动触发下一次 `loadMore()`，直到视口被有效填满或已无更多后台数据。
- **替代方案考虑**：
  - *完全依赖服务端过滤健康度*：健康度是基于探针观测计算的动态视图，重写服务端节点接口过滤改动面过大；客户端视口防饥饿逻辑简单高内聚。

### 6. 运维配置与端口隔离纪律
- **决策**：
  - 更新 `.env.example`，将过时的 `CLASH_*` 全面重写为 `CSP_*`，并提供 `CSP_ADMIN_TOKEN`、`CSP_PORT`、`CSP_FETCH_PROXY` 等官方支持字段的详细注释；
  - 在工程规范与测试用例中明确：`17000` 端口为宿主机生产映射别名，测试一律使用系统动态端口（如 `127.0.0.1:0`）及独立隔离临时数据库。
  - `web/e2e/auth-real-server.spec.ts` 在页面加载前注入 `localStorage.setItem('csp_locale', 'en-US')`，保证英文断言环境一致性。

### 7. 复审整改：发布异步隔离与修订原子激活
- **发布响应**：用户选择目标是唯一选择来源。网络异步结果按请求目标持久化；仅当请求目标仍是当前目标时才改变可见发布/预览、错误状态。不在成功回调中反向改变选择，切回时读取该目标持久记录；请求代际防止同目标旧响应盖过新结果。
- **修订创建**：`policy.Service` 的首次引导和显式 `StateActive` 共用 `RevisionRepository.CreateActive` 原子接口。SQLite 仓储在同一个事务中插入草稿、归档旧活跃版本并激活新版本；任一步出错整体回滚（包含旧活跃版本）。启动仅在无任何修订时引导，用户已有草稿不自动激活，原有规则数据不做删除/重建；`revision.Service` 显式生命周期保持不变。用 SQLite 临时触发器在激活 SQL 注入单次故障，重建 service 后去除故障并重试，检验无残留草稿、规则未丢失与活跃版本正确。

## Risks / Trade-offs

- **[Risk] 启动自动引导初始修订是否会影响既有包含数据的环境？**
  - → 缓释措施：仅当数据库查询活跃修订为空且修订总记录为 0 时才触发初始引导。已有数据或已有历史修订的环境绝不重复触发，保证纯净幂等。
- **[Risk] safeRuntimeConn 递归关闭底层套接字是否会导致多次关闭报错？**
  - → 缓释措施：使用 `sync.Once` 保护各底层连接的释放，各层关闭错误仅记录不向外扩散致命 panic。
- **[Risk] 前端自动连续加载分页是否会导致超大数据库卡顿？**
  - → 缓释措施：设置防饥饿连续加载的最大保护阈值（如单次连续请求最多自动加载 3 页），若仍无匹配数据则显示“继续加载更多”按钮，保障主线程响应速度。

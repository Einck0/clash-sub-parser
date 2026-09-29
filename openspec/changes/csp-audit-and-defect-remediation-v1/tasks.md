## 1. 后端运行时核心、配置修订与策略规则闭环 (Package 1)

- [x] 1.1 在 `internal/application/revision/service.go` 与 `internal/application/policy/service.go` 中实现初始修订引导与自动归属逻辑，在 `cmd/csp/main.go` 启动时调用 `EnsureActiveRevision`，并在 `internal/transport/http/revisions.go` 暴露 `POST /api/v1/revisions`。通过单元测试 `go test ./internal/transport/http -run TestRevisions` 与 `go test ./internal/application/policy -run TestPolicy` 验证无活跃修订时添加规则成功返回 201 且不报 422
- [x] 1.2 在 `internal/domain/ports.go`、`internal/repository/sqlite/policy.go`、`internal/application/policy/service.go` 与 `internal/transport/http/policies.go` 中实现准入规则与分流规则的删除能力（`DELETE /api/v1/policies/rules/{id}`）。通过 `go test ./internal/transport/http -run TestPolicyRulesDelete` 验证成功删除返回 204 且后续列表不再包含该规则，不存在规则返回 404
- [x] 1.3 在 `internal/transport/http/router.go` 中为遗留路由 `GET /yaml` 与 `GET /script` 注册显式 410 Gone 处理函数。通过 `go test ./internal/transport/http -run TestLegacyGoneEndpoints` 与 curl 验证响应状态码为 410 且不降级为 SPA 200

## 2. 探针并发安全、拨号错误解耦与观测分页修复 (Package 2)

- [x] 2.1 重构 `internal/probe/singbox/runtime.go` 中 `safeRuntimeConn` 的 `Close()` 逻辑，在互斥锁前遍历底层套接字并设置立即读写截止时间，确保阻塞读取状态下并发关闭立即返回。通过新增并发单元测试 `go test -race ./internal/probe/singbox -run TestSafeRuntimeConn_ConcurrentReadClose` 验证 100 次循环零死锁
- [x] 2.2 在 `internal/application/probe/safe_dialer.go` 中解耦错误类型，对 DNS 解析失败与私网/SSRF 拦截返回非致命的 `ErrTargetUnresolvable` 与 `ErrPrivateTargetRejected`，在 `runner.go` 中将其作为单节点 `VerdictUnavailable` 处理而不设置全局 `hasFatalDialErr`。通过 `go test ./internal/application/probe -run TestSafeNodeDialer` 与 `TestRunner_TargetResolutionFailureDoesNotFailRun` 验证单节点 DNS 失败时其余节点正常完成且批次成功
- [x] 2.3 修复 `internal/repository/sqlite/probes.go` 与 `internal/transport/http/probes.go` 中的 `nodeObservations` 分页逻辑，采用 SQL 真实 `LIMIT ? OFFSET ?` 并使用 `COUNT(*)` 计算真实 `total`。通过 `go test ./internal/transport/http -run TestNodeObservationsPagination` 验证多页数据时 `total` 与实际记录数一致

## 3. 订阅生命周期数据一致性与输入准入加固 (Package 3)

- [x] 3.1 在 `internal/repository/sqlite/subscriptions.go` 的 `Delete` 事务中补充孤儿节点原子去活更新（`UPDATE nodes SET active = 0 WHERE active = 1 AND NOT EXISTS (SELECT 1 FROM node_sources ...)`）。通过 `go test ./internal/repository/sqlite -run TestSubscriptionDeleteDeactivatesOrphanNodes` 验证删除订阅后独占节点变为非活跃（active=0）
- [x] 3.2 修复 `internal/transport/http/subscriptions.go` 中 PATCH 订阅的空白引用传递逻辑，使空白字符串正常送达业务层校验。通过 `go test ./internal/transport/http -run TestSubscriptionPatch_BlankSecretRefRejected` 验证提交纯空格引用时返回 422 `invalid_source_url_secret_ref`

## 4. 前端通信安全、设备兼容与交互自愈 (Package 4)

- [x] 4.1 在 `web/src/api/client.ts` 与 `web/src/features/auth/useAuth.ts` 中实现 CSRF 令牌管理，在用户登录后提取并在 POST/PUT/PATCH/DELETE 请求中自动注入 `X-CSRF-Token` 请求头。通过前端测试验证在 Cookie 模式下发起变更请求附带有效 CSRF 头
- [x] 4.2 在 `web/src/features/subscriptions/useSubscriptions.ts` 及通用工具中增加局域网纯 HTTP 环境下的 UUID v4 安全退避实现。通过前端单元测试断言当 `window.crypto.randomUUID` 为空时仍能生成合规 UUID 格式字符串
- [x] 4.3 优化 `web/src/features/probes/ProbesView.vue` 与 `useProbes.ts` 的心跳轮询机制，移除每 2 秒遍历 20 页全量节点的循环，仅在任务或节点池变化时按需同步状态。通过模拟定时器验证心跳期间不会频繁调用 `/api/v1/nodes` 深度分页
- [x] 4.4 在 `web/src/features/nodes/NodesView.vue` 中实现视口无限滚动防饥饿机制，当客户端筛选后结果不足填满视口且 `hasMore` 为真时自动加载后续分页。通过单元测试与组件测试验证筛选状态下自动加载下一页
- [x] 4.5 修复 `web/src/features/subscriptions/SubscriptionConfigDrawer.vue` 的保存校验拦截，在非基础 Tab 或点击底部保存时强制验证必填字段。通过测试验证空白名称或空白地址无法触发 `save` 派发
- [x] 4.6 在 `web/src/features/policy/usePolicy.ts` 中对所有策略组及连接边的 REST 接口动态 ID 参数应用 `encodeURIComponent`，并补充删除规则的前端 Composable 方法
- [x] 4.7 在 `web/src/features/publications/usePublications.ts` 与 `PublicationsView.vue` 中引入客户端发布令牌持久化恢复逻辑，确保刷新页面后“复制订阅链接”依然携带有效 `token`。通过单元测试验证刷新状态下导出的完整 URL 包含令牌

## 5. 运维环境配置与自动化测试治理 (Package 5)

- [x] 5.1 更新根目录 `.env.example`，清理所有 Python 时代的 `CLASH_*` 遗留变量，全面对齐 `docker-compose.example.yml` 与 Go 后端 `CSP_*` 规范（包括 `CSP_ADDR`、`CSP_ADMIN_TOKEN`、`CSP_PORT`、`CSP_FETCH_PROXY` 等）
- [x] 5.2 在 `scripts/smoke_test.sh` 及相关测试运行规程中明确 17000 端口为生产映射别名契约，确认所有冒烟与集成测试严格绑定动态临时端口（`0`）和隔离临时数据库，绝不向 17000/18080 发起请求。运行 `bash scripts/smoke_test.sh` 验证 6 项测试全部通过（退出码 0）
- [x] 5.3 修复 `web/e2e/auth-real-server.spec.ts` 中的多语言冲突，在启动测试时显式注入 `en-US` 语言或断言本地化标识符，杜绝默认中文环境下的字符串断言失败。在隔离环境执行 `npm run type-check` 验证前端代码类型检查通过

## 6. 独立复审驳回的根因整改 (Remediation)

- [x] 6.1 修复发布异步响应跨目标写入：仅当前选中目标可更新可见发布状态、预览、选择与错误；延迟的 Mihomo 发布在切换 sing-box 后不得反选 Mihomo，成功结果仍按 Mihomo 目标存储以供切回恢复。通过延迟响应与切回的 Vitest 回归断言；继续保证刷新后令牌链接可复制。
- [x] 6.2 在 `policy.Service` 的首次引导与显式创建活跃修订共用 SQLite 仓储原子创建+激活路径：若激活失败，事务回滚不留下引导草稿或假活跃版本；下一次启动可安全重试，不激活用户既有草稿或丢失既有规则；数据库查询与激活错误均向调用方传播。注入一次激活失败后重新构造服务并启动验证恢复，同时覆盖用户草稿和规则数据保留。
- [x] 6.3 `policy.Service.CreateRevision(StateActive)` 在激活失败时返回错误且绝不记录成功审计，使用 6.2 的原子路径避免遗留草稿；补充注入失败与数据不变的测试，保持 `revision.Service` 原有显式生命周期不变。
- [x] 6.4 所有整改完成后运行 `go build ./...`、`go test ./...`、前端 type-check/test/build、隔离 smoke/E2E、`openspec validate csp-audit-and-defect-remediation-v1 --strict`、`git diff --check`；仅全部退出码为 0 时勾选本节及 1.1/4.7。

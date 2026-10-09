## Why

在针对 CSP 1.0（分支 `dev` 基线 `fe4654b`）的独立前后端代码与运行时走查中，发现并证实了多项影响系统可用性、并发安全性、生命周期一致性与环境安全契约的真实功能缺陷：
1. **配置修订引导缺失**：全新数据库启动时无任何配置修订版本，导致添加规则时报 422 `missing_revision_id`、发布中心报 409 `no_active_revision` 且前端无从引导恢复；
2. **探针运行时并发死锁**：`safeRuntimeConn` 的 `readCloseMu` 在底层 `Read()` 处于阻塞状态时执行 `Close()` 发生互斥死锁，导致探测连接取消与超时无法及时中断；
3. **拨号器错误归类过宽**：单节点 DNS 解析失败或私网目标被 SSRF 策略拒绝时，`safe_dialer.go` 统一包裹 `ErrCredentialsUnavailable`，导致 runner 将其判定为系统级致命错误而使整个探测批次全部置为失败；
4. **订阅删除残留活跃孤儿节点**：删除订阅源时仅级联删除 `node_sources`，未对失去所有来源的活跃节点执行状态停用，造成台账残留无效僵尸节点；
5. **观测历史分页与计数偏差**：节点观测历史接口采用 `page*size` LIMIT 且以内存结果长度作为 `total`，导致前端分页组件计算失真；
6. **空白凭据引用更新静默穿透**：PATCH 订阅源时若传递空白字符串 `source_url_secret_ref`，传输层修剪导致命令层接收 `nil` 而完全绕过必填校验；
7. **策略准入规则缺乏删除端点**：策略规则支持新增却未开放删除 REST 路由与 Composable 方法，已建规则无法清理；
8. **前端鉴权 Cookie 缺少 CSRF 令牌投递**：在 Cookie 会话模式下，`ApiClient` 未提取和投递 `X-CSRF-Token`，导致状态变更请求被 `middleware_auth.go` 拦截（403）；
9. **局域网 HTTP 下 UUID 生成崩溃**：`crypto.randomUUID()` 在非安全上下文（纯 HTTP 局域网）中未定义，导致订阅刷新报错；
10. **探测工作台高频分页轮询风暴**：`ProbesView.vue` 每 2 秒循环拉取最多 20 页全量节点，产生极高无谓请求开销；
11. **节点台账筛选状态下无限滚动饥饿**：客户端健康度筛选后若视口未被填满且无法触发滚动事件，已请求数据无法自动递增分页；
12. **订阅抽屉必填校验跳过**：保存按钮直接调用提交逻辑，未通过表单有效性断言即可跳过必填字段保存；
13. **策略组 URL 路径未执行安全编码**：策略组相关 REST 请求未对标识符使用 `encodeURIComponent`；
14. **发布中心刷新后复制链接缺失令牌**：单次发布后重新加载页面，导出 URL 缺失 `token` 参数导致复制无效订阅地址；
15. **Playwright E2E 语言断言冲突**：真实服务端到端测试硬编码英文文案断言，与默认中文语言环境不符；
16. **历史路由 SPA 降级误响应**：`/yaml` 与 `/script` 遗留端点未正确拦截为 410 Gone，误降级为 SPA 200；
17. **环境示例残留 Python 时代遗留变量**：`.env.example` 中包含 `CLASH_AUTH_TOKEN` 等无效变量，误导运维配置导致认证失效；
18. **17000 端口生产环境混淆**：宿主机 17000 为线上容器映射端口，非测试预览环境，需确立严格隔离准则。

本项目内 OpenSpec 变更将上述经确证的缺陷组织为五个高内聚、零重叠写集合的特性级工作包，建立确定性修复与门禁验证契约。

## What Changes

- **配置修订生命周期与规则管理完善**：
  - 服务端启动时在空数据库上自动引导初始有效活跃配置修订（Active Revision），消灭全新数据库首次添加规则报 422 `missing_revision_id` 与发布中心 409 `no_active_revision`。
  - 新增 `POST /api/v1/revisions` 显式创建/升级修订端点。
  - 补充准入规则与策略规则的删除 REST 路由（`DELETE /api/v1/policies/rules/{id}`）及底层仓储能力。
  - 明确遗留端点拦截：`GET /yaml` 与 `GET /script` 显式返回 HTTP 410 Gone 及指引，禁止 SPA 200 降级。
- **探针运行时并发死锁消除与错误分类精确化**：
  - 重构 `safeRuntimeConn` 的关闭与超时机制：在执行互斥加锁前递归中断底层套接字读写超时并安全释放底层连接，杜绝 `Read()` 阻塞导致 `Close()` 死锁。
  - 拨号器错误解耦：将 DNS 解析失败与私网/SSRF 目标拦截归类为节点级目标错误（`ErrTargetUnresolvable` / `ErrPrivateTargetRejected`），在生成 `VerdictUnavailable` 观测证据后正常结束任务，不再设置 `hasFatalDialErr`，杜绝单节点故障拖垮全局探测。
  - 修正节点观测历史接口 `/api/v1/nodes/{logical_id}/observations`：仓储层采用真实 `LIMIT ? OFFSET ?` 并配合 `COUNT(*)` 计算真实 `total`。
- **订阅生命周期数据一致性与变更准入加固**：
  - 订阅删除事务原子级联清理孤儿节点：删除订阅源时，原子执行无所属来源活跃节点的去活操作（`SET active = 0`），杜绝无效孤儿僵尸节点。
  - 修复 PATCH 订阅空白引用绕过缺陷：传输层保持空字符串指针传递，使 `invalid_source_url_secret_ref` 校验逻辑正常生效。
- **前端通信安全、设备兼容与状态交互自愈**：
  - `ApiClient` 与 `useAuth` 支持 CSRF 令牌管理：提取登录响应中的 `csrf_token` 并在状态变更请求头中附带 `X-CSRF-Token`。
  - 健壮的 UUID 生成降级：在非安全上下文（纯 HTTP 局域网）提供符合 RFC4122 v4 的退避算法，避免 `crypto.randomUUID()` 未定义异常。
  - 消除探测工作台高频分页风暴：移除每 2 秒轮询 20 页全量节点的激进循环，仅在任务或节点池变动时按需同步状态。
  - 解决节点台账筛选状态下的无限滚动饥饿：当视口未充满且存在更多数据时自动补充请求，提供用户可恢复的加载机制。
  - 订阅配置抽屉必填校验守卫：在保存前显式验证表单输入，阻止空白必填字段提交。
  - 策略组 REST 请求 URL 编码：对所有动态路径参数使用 `encodeURIComponent`。
  - 发布链接令牌持久化：在客户端安全会话存储中维护发布令牌，确保页面刷新后“复制订阅链接”依然完整携带 `?token=...`。
- **运维配置统一与自动化测试鲁棒性**：
  - 同步 `.env.example` 与 `docker-compose.example.yml`，彻底清理 Python 时代 `CLASH_*` 遗留变量，统一采用 `CSP_*` 规范。
  - 明晰 17000 端口生产宿主属性：严禁向 17000 或 18080 端口发送测试写入，所有本地与 E2E 测试均运行于动态临时端口（`0`）及独立临时数据库。
  - 调整 Playwright E2E 语言环境初始化：通过显式注入 `en-US` 语言或断言本地化标识符，消除默认中文语言导致的字符串断言失败。

## Capabilities

### New Capabilities

- `audit-and-defect-remediation`: 针对 CSP 1.0 全栈经审计确证的功能缺陷提供确定性修复，涵盖修订引导、规则增删、探针死锁与错误分类、观测分页、订阅孤儿去活、CSRF 请求头注入、局域网 UUID 退避、前端轮询风暴优化与视口防饥饿。

### Modified Capabilities

无（`openspec/specs/` 下无对应已归档主规格，本次通过新增能力规格建立权威契约）。

## Impact

- **后端代码**：
  - `cmd/csp/main.go`
  - `internal/application/revision/service.go`
  - `internal/application/policy/service.go`
  - `internal/application/policy/types.go`
  - `internal/application/probe/safe_dialer.go`
  - `internal/application/probe/runner.go`
  - `internal/application/subscription/service.go`
  - `internal/domain/ports.go`
  - `internal/domain/probe.go`
  - `internal/probe/singbox/runtime.go`
  - `internal/repository/sqlite/policy.go`
  - `internal/repository/sqlite/probes.go`
  - `internal/repository/sqlite/subscriptions.go`
  - `internal/transport/http/policies.go`
  - `internal/transport/http/probes.go`
  - `internal/transport/http/revisions.go`
  - `internal/transport/http/router.go`
  - `internal/transport/http/subscriptions.go`
- **前端代码**：
  - `web/src/api/client.ts`
  - `web/src/features/auth/useAuth.ts`
  - `web/src/features/nodes/NodesView.vue`
  - `web/src/features/policy/usePolicy.ts`
  - `web/src/features/probes/ProbesView.vue`
  - `web/src/features/probes/useProbes.ts`
  - `web/src/features/publications/PublicationsView.vue`
  - `web/src/features/publications/usePublications.ts`
  - `web/src/features/subscriptions/SubscriptionConfigDrawer.vue`
  - `web/src/features/subscriptions/useSubscriptions.ts`
- **配置与测试**：
  - `.env.example`
  - `web/e2e/auth-real-server.spec.ts`
  - 后端单测与集成测试用例补充

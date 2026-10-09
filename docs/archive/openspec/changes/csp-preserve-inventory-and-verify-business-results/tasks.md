## 1. 订阅节点非破坏性合并与来源生命周期（Subscription Inventory Preservation & Merging）

- [x] 1.1 重构 `internal/application/inventory/service.go` 的 Reconcile 流程：在外部订阅拉取失败、HTTP 异常、内容解析错误或提取结果为空时，记录审计事件并将状态标记为 stale/failed，全量保留该订阅既有的活跃节点与策略边，通过单元测试 `go test -v -run TestReconcile_FetchOrParseFailure_PreservesNodes ./internal/application/inventory/...` 验证节点不被删除或失活。
- [x] 1.2 废除 `internal/application/inventory/service.go` 中的全局孤儿节点批量失活 SQL（`deactivateOrphansSQL`），将来源清理严格限定在当前 `subscription_id` 授权作用域内，通过单元测试 `go test -v -run TestReconcile_PruneScoped_ProtectsManualAndOtherSourceNodes ./internal/application/inventory/...` 验证来源修剪不波及独立手工节点与其他订阅节点。
- [x] 1.3 在同一订阅来源作用域内实现稳定节点身份匹配：优先匹配源端稳定标识，无标识时基于同一来源内名称与协议唯一组合匹配；存在重名或歧义时保守保留并新增，严禁跨来源按名称合并凭据，通过单元测试 `go test -v -run TestReconcile_IdentityMatching_DisambiguationAndIsolation ./internal/application/inventory/...` 验证跨来源隔离与同一来源更新行为。
  - [x] 1.3.1 根因整改：调用 `computeScopedLogicalID` 隔离新来源/歧义身份，同 host/port 跨来源不同 UUID/password 绝对禁止 upsert 覆盖；多源共享节点连接变更时执行 copy-on-write 隔离并保守保留政策边，补充同 host+port+协议但 UUID/password 不同的 A/B 刷新与 revision 独立自测试。
- [x] 1.4 实现严格的连接版本控制：当节点的真实连接要素发生变更时单调递增 `connection_revision` 并将当前健康探测状态置为 unknown/pending；若仅为显示名称或 JSON 键排序变化则保持版本不变，通过单元测试 `go test -v -run TestReconcile_ConnectionRevision_IncrementOnRealChange ./internal/application/inventory/...` 验证版本自增与历史观测失效逻辑。

## 2. 历史误伤失活节点证据化恢复（Historical Mis-deactivated Node Precision Recovery）

- [x] 2.1 编写历史误伤节点排查与恢复脚本工具，基于归因特征（2026-09-29T00:52 批次全局孤儿失活、规范配置合法、未被用户手工禁用）精准定位待恢复节点，通过单元测试与 dry-run 验证其排查逻辑不包含用户主动禁用的节点。
- [x] 2.2 在 SQLite 备份副本上完整演练恢复脚本，验证其幂等性、可回滚性与受影响行数输出，确保重复执行受影响行数为 0 且回滚脚本能无损恢复，记录结构化演练审计凭据。
  - [x] 2.2.1 根因整改：使用 CLI 对新创建沙箱 DB 副本执行真实演练，生成 output-plan/output-report 并持久化至 logs/recovery（含 plan hash、审批模拟标识、首次 apply、重复 apply 0 行、rollback 报告与 sha256），归因 960 节点标记为 heuristic ambiguous 不可批准生产。

## 3. 探针出站构建器解耦与 Reality 协议严格校验（Probe Construction & Protocol Decoupling）

- [x] 3.1 重构 `internal/probe/singbox/builder.go`：将底层的 L4 网络协议（`tcp`/`udp`）与传输层包装（`ws`/`grpc` 等）严格解耦，出站配置中的 `Network` 字段正确填入 L4 协议且支持 UDP，通过单元测试 `go test -v -run TestSingboxBuilder_L4TransportDecoupling_UDP ./internal/probe/singbox/...` 验证 Shadowsocks、VLESS、VMess、Trojan、Hysteria2 等协议出站网络参数的正确性。
- [x] 3.2 规范 Reality 协议参数校验：对 `RealityPublicKey`（`pbk`）执行标准及 URL-Safe Base64 解码与 32 字节严格长度校验，对 `RealityShortID`（`sid`）执行偶数长度与纯十六进制校验（奇数长度或非法字符明确拒绝，严禁截断补零），通过单元测试 `go test -v -run TestSingboxBuilder_RealityParametersValidation ./internal/probe/singbox/...` 验证边界情况拦截。
- [x] 3.3 编写探针实际构建能力测证测试，针对底层使用的 Sing-box 核心库真实测证其对 `xhttp` 等协议特性的构建支持情况，通过 `go test -v -run TestSingboxLibrary_ActualFeatureSupport ./internal/probe/singbox/...` 确立实际支持基线。

## 4. 安全拨号器脱敏结构化诊断与状态解耦（Safe Dialer Diagnostics & Decoupled State）

- [x] 4.1 改造 `internal/application/probe/safe_dialer.go`：捕获底层客户端构建失败原始错误并进行分类归因，输出脱敏的结构化诊断信息，严格剥除密码、UUID、URL query 及公私密钥，通过单元测试 `go test -v -run TestSafeDialer_RedactedDiagnostics ./internal/application/probe/...` 验证敏感凭据未泄露。
- [x] 4.2 维持 SSRF 严格防护、私网 IP 拦截与 TLS 证书严格校验安全边界，通过单元测试 `go test -v -run TestSafeDialer_SSRFAndPrivateNetProtection ./internal/application/probe/...` 验证针对回环地址与私有网段的防御不被放宽。
- [x] 4.3 在探针观测状态机中将“本地配置构建成功”与“远程网络握手健康”严格分离，本地构建成功但远端网络超时或失败时真实记录网络失败状态，通过单元测试 `go test -v -run TestProbe_BuildSuccessVsHandshakeFailureSeparation ./internal/application/probe/...` 验证健康态不被虚假上报。

## 5. 全接口业务语义与双向核对测试矩阵（Comprehensive API Semantic Matrix & End-to-End Tests）

- [x] 5.1 编写前后端路由双向核对测试，静态比对前端 API 客户端调用点与后端 Chi 路由注册表，确保覆盖 45+ 路由端点无遗漏，通过测试 `go test -v -run TestRouteInventory_BilateralCompleteness ./test/e2e/...` 验证双向映射完整。
- [x] 5.2 编写覆盖 Auth 与 Settings 模块的隔离全接口语义测试，通过 `go test -v -run TestAPISemantic_AuthAndSettings ./test/e2e/...` 验证登录登出会话、双鉴权独立开关持久化及权限拦截。
- [x] 5.3 编写覆盖 Subscriptions、Nodes、Policies 模块的隔离全接口语义测试，通过 `go test -v -run TestAPISemantic_SubscriptionsNodesPolicies ./test/e2e/...` 验证数据库持久化副作用、来源隔离、幂等性与节点连接补丁版本递增。
  - [x] 5.3.1 根因整改：补全真实 HTTP 及业务副作用测试：PATCH /api/v1/nodes/{id}、GET/PATCH/PUT /api/v1/policies/groups/{id}、POST /api/v1/policies/groups/{id}/edges、GET /api/v1/revisions、GET /api/v1/revisions/{id}，以及 policy groups risk-policy GET/PUT/DELETE 与 /api/v1/policy/* 别名。
- [x] 5.4 编写覆盖 Probes 与 Publications 模块的隔离全接口语义测试，通过 `go test -v -run TestAPISemantic_ProbesAndPublications ./test/e2e/...` 验证探针任务状态机取消、幂等创建、发布创建、预检、不可变导出（`/publish/v1/{id}` 与 `/p/{id}`）及发布撤销。
  - [x] 5.4.1 根因整改：补全 DELETE /api/v1/publications/{id} 真实测试；补全 admission rules GET/POST/DELETE、ip-risk groups evaluate/members、ip-risk policies GET/deactivate/review 等端点实测，实测 /yaml 及 /script 真实 410 错误包络，重新盘点 chi 真实路由与 docs/api_semantic_matrix.md 计数。

## 6. 质量门禁验证与受控部署预案（Quality Gates & Controlled Deployment Runbook）

- [x] 6.1 运行全项目统一静态编译 `go build ./...`、全量后端单元测试 `go test ./...` 以及前端类型检查 `npm run type-check`，验证全部门禁退出码为 0，无编译与类型错误。
- [x] 6.2 整理受控部署预案文档与脚本，明确数据库备份验证、测试副本演练、健康检查端点（`/healthz`、`/readyz`）验证、可用回滚镜像构建以及禁止重启 Hermes/Gateway 约束，由主脑最终核准。

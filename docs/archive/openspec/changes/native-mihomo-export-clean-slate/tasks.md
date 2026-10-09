## 1. Phase 1 — 已实现基础与新增合同门禁

- [x] 1.1 统一 WG/TUIC/VLESS/Hysteria2 凭据提取及基础字段验证；既有实现保留但新增身份安全回归归 1.4，不能以该勾选证明恶意端点已拒绝。
- [x] 1.2 解析入口、图规则/拓扑与四目标编译接口基础收敛；可信节点身份与版本化合同仍由 1.4 冻结。
- [x] 1.3 复核 legacy 清场及 `sqlite/node_risk.go` 原勾选范围：多 provider 最新观测 count/page 必须一节点一行，分页节点后按索引取最新、不扫描全历史；以独立多 provider 同时间戳、不同 ID、海量旧行和 EXPLAIN QUERY PLAN fixture 验证，历史 DB 不自动写改，Go 定向测试与 build exit 0。（原任务包括风险模块，审查证明该部分未完，撤销勾选。）
- [x] 1.4 A 先冻结 `VerifiedNodeCredential`/版本化 identity binding（可信来源 vs payload、规范化 server/port/protocol/transport、敏感字段排除）和 `GET /api/v1/nodes/{logical_id}` 的 `node.connection`、`PATCH /api/v1/nodes/{logical_id}/connection` JSON/错误合同，文档对应 design.md 决策 1/2；不得纯由 payload 自证或简单反算 ID。证明 YAML/URI 双格式、相同 ID/protocol/version 恶意替换端点被拒绝、旧缺绑定 fail closed，合法 URL 不误拒。冻结 B 使用的接口与 E 使用的详情接口后才扇出。
- [x] 1.5 冻结 B/D 两个 SQLite migration 的不同版本号、仓库端口事务接口与迁移测试边界，两个分支不得同时写 `migrate.go` 或同一 migration 文件；如需共享编辑，先串行完成交接。

## 2. Phase 2 — 特性级施工与本地自测（A 合同后只对互斥写集并发）

- [x] 2.1 C Mihomo（`internal/compiler/mihomo.go` + 独立测试）：RULE-SET 仅接受真实已配置 provider 或显式合法 URL，缺失即定点拒绝；所有四组及十四规则正反 fixtures、官方 `mihomo -t -f`、`go build ./...` exit 0；不与 A 同时修改 `compiler/compiler.go`。
- [x] 2.2 sing-box 官方选项编译的既有特性包；最终门禁 3.1 仍需重验。
- [x] 2.3 Surge 可表达子集编译的既有特性包；最终门禁 3.1 仍需重验。
- [x] 2.4 Quantumult X 可表达子集编译的既有特性包；最终门禁 3.1 仍需重验。
- [x] 2.5 B Publication+repo migration（`internal/application/publication/**`、`domain/publication.go`、`domain/ports.go` 出版接口、SQLite publication repo + 专用 migration、`transport/http/publications.go`/cmd 仅必要接线及专属测试）：A 交付可信凭据合同后，全目标的预览/发布/分发核验身份；原子持久化加密工件、内容 digest、凭据版本绑定/编译版本，删除全局 map 与从 auditRepo.List(PageSize100) 文本恢复；临时 DB 升级、重启、源变更、损坏、错误 token、撤销和历史无绑定 fail closed；`go build ./...` exit 0。无同一物理文件并发写入。
- [x] 2.6 E 前端（`web/src/features/nodes/**` 及专属测试）：必须在 A 冻结详情/编辑 API 后施工；删除 `.edge.internal`、假 WG 地址/公钥/UUID/secret 状态，未返回字段显示 unavailable；真实 GET 详情、PATCH 保存/轮换、失败回滚 UI、来源覆盖提示，不在 state/DOM 缓存秘密；Vitest 与 `vue-tsc --noEmit` exit 0。
- [x] 2.7 A 端点身份与节点编辑（`domain` 身份字段、`parser`、`resolver`、`inventory`、`http/nodes.go` 与 A 专属测试）：按 1.4 冻结合约，存可信绑定/非秘密详情投影、管理员 CSRF/CAS PATCH、轮换及订阅 Reconcile 覆盖/新身份节点语义；覆盖直接 compiler WithCredentials 绕过、恶意同 ID 换端点、合法 URL/YAML、无私钥/PSK 外泄、审计/并发；定向测试及 `go build ./...` exit 0。完成 1.4 后方与 B/C/D 并发独立文件施工，A 与 E 不写同文件。
- [x] 2.8 D SQLite risk（`internal/repository/sqlite/node_risk.go`、独立风险 migration + 专属测试）：单节点唯一最新风险 CTE，count/page 同过滤；分页 ID 后有界取最新、索引和 EXPLAIN；多 provider/大历史/unknown 与边界负例定向测试及 `go build ./...` exit 0。与 B 避免同 migration 文件/版本号及共享测试文件。

## 3. Phase 3 — 扇入终态门禁（所有 Phase 2 完工后）

- [x] 3.1 最终合并工作树执行 `go test -race -count=3` 分组覆盖全部包、`go test -count=1 ./...`、`go build ./...`、web Vitest、`npx vue-tsc --noEmit` 全部 exit 0；隔离 vite build 前后 webassets/dist hash 不变；官方 mihomo/sing-box fixture 校验、端点伪造与本地 migration/重启/风险查询计划 fixtures、`openspec validate native-mihomo-export-clean-slate --strict` exit 0；保存退出码/版本/摘要。原先勾选早于独立审查阻断，撤销。
- [x] 3.2 独立 reviewer 对照本 Change、最终 diff、身份锚/HTTP 编辑/发布持久化与风险计数证据复验给出 PASS。阻断最多一次定点返工与复验；仍失败直接根因级修订本 Change，未通过项不勾选。
- [x] 3.3 准备者提供隔离 URL、临时密钥/库、健康检查及多视口截图；critic 真实节点→编辑→预览→发布→下载→撤销 FLOW + VISUAL PASS 后才勾选，不触生产。

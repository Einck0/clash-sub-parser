## Why

CSP 受控更新及合法带身份的真实业务验收仍未交付。2026-10-02 本轮只读取证确认 HEAD=`e0431767845ffe3cd2bff8a7e415abf085814ac7`、起始工作树 clean，运行容器 healthy，但健康 200、历史 fixture PASS 和无凭据安全暂停均不能代替正文审计、订阅刷新和真实握手。旧 tasks 3.1 的完成勾选不符合已执行事实，必须撤销；本轮只规划，不实施、构建、部署、刷新、恢复或提交。

历史部署报告把当前镜像绑定到 `6a008f7-dirty` 及31文件 manifest，运行二进制具有后来库存保全提交的 SQL 特征。`95ce0a9..e043176` 的服务业务源码、前端及迁移未变化，仅恢复 CLI 所用库变化；不能声称订阅失败保护尚未上线。需要的是可核验源码/产物归属的受控发布和真实验收，而非重新实现既有保护。

现有验收 CLI 存在确定性的假 PASS 风险：正文断言未汇聚，错误/空探针可最终 PASS，探针请求字段与观察端点错配，分页计数不完整、库存保护只比计数且网络错误直接假定保留，报告写入错误被忽略。runner 写固定公共 `/tmp` 路径。必须先整改和独立审查再使用于生产。

## What Changes

- 继续同一 repo-local Change，冻结完整特性包 `pkg_csp_release_and_acceptance`，串行完成施工自测→独立审查→授权 app 发布验证→合法真实验收→独立证据复验。
- 复用 Go 标准 HTTP/context/json/testing/httptest、已有 SQLite/领域类型/脱敏函数及 e2e harness；修复验收工具的真实协议、fail-closed 结论、身份/分页/库存集合核验、私有 scratch 和报告错误处理，不另造控制框架。
- fresh 一致性备份 DB/库存/身份配置、当前镜像及 Compose/环境；验证可恢复性后只替换目标 app。保留 `csp-v1-data`、双 loopback 端口、代理及现有 DB verifier，不改 gateway、其他服务、Hermes 内核或模型路由。
- 合法凭据只来自已存在受管来源；缺凭据时真实验收保持未完成，可继续本地整改，不改哈希、不降级鉴权、不聊天索要秘密。
- 真实验收仅既有订阅/节点，限定预算，失败保护库存；历史恢复仅因果充分且独立审查的精确白名单，允许真实计数0但不掩盖待证明事项。
- 使用已冻结的本轮原生 run 合同和真实子会话报告闭环；保持历史 UNVERIFIED 原样，不倒灌合同。不为现有已修复的报告识别再扩大跨根源码修改。

## Capabilities

### New Capabilities
- `live-acceptance-and-qualified-recovery`: 真实带身份正文验收、有界实网、因果充分恢复及可回滚发布。

### Modified Capabilities

无。

## Impact

限定 CSP 的 `cmd/csp-live-acceptance/main.go`、runner、对应 CLI/e2e 测试和两份运行手册；恢复实现只在新证据揭示具体缺口时定点修复。生产只涉及已授权目标 app。Pi `/home/service/pi` 及外置 bridge 本轮只读取证，不写；CLI 确认 bridge 所属 OpenSpec 根是 `/home/service/hermes/hermes-home`，并非独立子目录根。本轮无 UI 变更，不派无 URL critic。规划 READY 不等于业务 PASS。

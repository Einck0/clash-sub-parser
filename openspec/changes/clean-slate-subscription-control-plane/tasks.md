## 1. 独立根与物理切断

- [x] 1.1 创建独立 `control-plane/` 根、依赖清单、后端、前端、Alembic 与测试工程，验证构建入口不导入旧 `backend/app` 或旧前端
- [x] 1.2 定义 `csp-control-plane`、`csp-control-plane-db`、`csp_control_plane`、`csp-control-plane-db-data` 和 `csp-control-plane-net`，验证 Inspect、镜像层、构建上下文、卷、连接与运行网络均不接触旧 `app`、`backend-data`、SQLite 或旧网络
- [x] 1.3 实现新镜像/secret 注入 allowlist，验证构建参数、层、环境、日志、诊断与镜像历史不含旧路径或真实凭据
- [x] 1.4 建立静态依赖、镜像层、挂载、网络和连接审计，验证任一旧边界引用阻断构建/部署

## 2. 新库、身份与确认门

- [x] 2.1 建立独立 PostgreSQL 的 Alembic schema，覆盖 logical configuration、source、inventory、job、observation、令牌、确认与审计，验证空库从无版本升级至 head
- [x] 2.2 实现只读 readiness，验证未迁移库返回不可用且启动、调度和请求均不执行 DDL
- [x] 2.3 实现 `ExecutionConfirmation` 的 action、target digest、operator、生产环境、15 分钟 expiry、一次性消费与无秘密审计，验证缺失/错目标/过期/重复消费均零副作用
- [x] 2.4 让生产初始化、离线导入、入口切换、旧容器停服和外部写入均在副作用前消费对应确认，验证每种 action 的成功审计和拒绝路径
- [x] 2.5 实现新库备份 manifest 与恢复演练，验证恢复只连接新数据库且可回到指定已发布修订

## 3. 来源授权、库存和秘密

- [x] 3.1 实现带版本、状态、操作类型和预算的 Source 授权，验证未启用或授权变更后不派发刷新
- [x] 3.2 实现带预算的 ProbeProfile 授权和逐尝试复查，验证取消、禁用、删除、版本变更和预算耗尽停止后续网络尝试并给出确定终态
- [x] 3.3 实现 Source、SourceRevision、InventoryNode 和来源关联，验证成功刷新原子产生修订、失败保留上次成功输入且无 N+1 查询
- [x] 3.4 实现 protocol adapter 的字段清单、canonical JSON、AEAD `EncryptedSecret` 和版本化 HMAC 指纹，验证等价 payload 归并、协议/凭据差异不归并以及 API/日志/诊断脱敏
- [x] 3.5 实现手工来源和安全 HTTP 刷新，验证逐跳地址校验、字节/时长/并发预算及失败原子性

## 4. 探测与不可变观测

- [x] 4.1 实现 ProbeJob、租约和全局/任务级预算，验证取消、超时、崩溃恢复和授权撤销回收资源
- [x] 4.2 实现 `trust_env=false` 的隔离运行器，验证不可用节点不能借宿主出口成功
- [x] 4.3 实现握手、出口、测速、流媒体和 AI 分项不可变观测，验证限流、挑战、地区和目标服务故障与节点故障分离
- [x] 4.4 实现周期调度只从已启用 Profile 创建有预算 Job，验证周期任务不绕过授权或预算

## 5. 可移植配置和唯一编译器

- [x] 5.1 定义 Bundle 与 `selector_slot` schema，验证 Bundle 不含来源/库存 logical ID、节点 payload、URL、UUID、密码、私钥或 token
- [x] 5.2 实现跨实例导入为 `UNBOUND` 草稿和本地 `RebindDraft`，验证空实例导入不创建节点或秘密且未 rebind 不能发布
- [x] 5.3 实现 slot 本地绑定、完整性预检、非空编译与原子失败，验证绑定不存在库存、部分绑定和循环均不改变活动修订
- [x] 5.4 实现逻辑配置、观测谓词、唯一确定性编译器和纯缓存，验证相同输入结果稳定且缓存不触发刷新、探测或数据库写入
- [x] 5.5 实现预览和节点解释，验证可解释选择与发布使用同一编译结果

## 6. 修订、发布和 API

- [x] 6.1 实现不可变 ConfigurationRevision、唯一活动修订、预检、保留和回滚，验证失败预检零状态变更
- [x] 6.2 实现 Clash、Mihomo、Stash、Shadowrocket 四个渲染器，验证同一语义图、节点集合、策略集合、revision 和 fingerprint
- [x] 6.3 实现哈希存储、单 target scope、expiry、撤销和一次性显示的发布令牌，验证令牌无法访问管理 API
- [x] 6.4 实现 `GET /publish/{target}`、服务器纯编译缓存与 `private, no-store`，验证输出只含已选节点的客户端必需连接材料且不含来源/管理秘密、未选节点或 token
- [x] 6.5 实现 `/api/control` 统一封套、logical ID、Cookie/CSRF/Bearer 边界和脱敏异步 Job，验证未认证、缺 CSRF 和越权均零变更

## 7. Legacy deny-list 与新控制台

- [x] 7.1 从受控 legacy-reference fixture 的 FastAPI 路由表、main include、全部 router、生成/下载/快照和旧前端路由生成版本化 `legacy-deny-manifest.json`，验证 manifest hash 变化在 CI 中需人工审查
- [x] 7.2 对 manifest 的每个 method/path、`/api/v2/**`、`/yaml`、`/script`、Script.js、下载/快照和旧 SPA path 断言 404、无 Location、无翻译/生成、无 DB/Job/网络副作用
- [x] 7.3 建立独立控制台的 sources、inventory、probes、policies、releases、settings 切片，验证不导入旧 API client、store、页面或组件
- [x] 7.4 为危险操作、异步状态和草稿隔离实现可访问交互，验证取消、焦点恢复、重复提交和网络失败不损失草稿

## 8. 演练、确认与质量门

- [x] 8.1 在独立空库演练 Alembic、授权刷新、探测、Bundle rebind、四目标发布、恢复和 physical-boundary 检查，验证零旧运行时接触
- [x] 8.2 在脱敏旧库副本演练一次性离线导入，验证仅生成来源名称、策略、规则草稿并隔离 URL、节点载荷、UUID、token 与凭据
- [x] 8.3 仅在对应 `ExecutionConfirmation` 已创建后执行生产初始化、离线导入、入口切换或停旧容器；验证未确认时保持旧入口和旧容器不变
- [x] 8.4 执行后端静态检查、全量测试、前端类型/单元/构建、秘密扫描、资源预算、legacy deny-list、四目标 golden 与独立审查，验证全部通过
- [x] 8.5 运行 `openspec validate clean-slate-subscription-control-plane --strict`，验证 proposal、design、specs 和 tasks 一致

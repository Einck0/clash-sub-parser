## 1. 公共契约冻结与本地基线

- [x] 1.1 冻结 `domain` 中 ProbeSchedule/ProbeBatch、NodeFilterSpec、FilterCondition、credential_version 与仓库接口，记录 API JSON 示例和有效/无效条件矩阵；以 domain 表驱动测试和 JSON 往返测试验证兼容。
- [x] 1.2 在隔离测试 DB 核验既有手动探测、库存和旧策略输出；对 `7289b4c` 的 `PROCESS-NAME` 回归用例做只读代码核验，若用户提供本次脱敏请求/日志则建立可重复的错误复现用例，否则将未知错误独立标记为待证据，不猜测修复；以基线/复现记录核对结论。

## 2. 特性包 A：周期探测端到端（公共契约冻结后与包 B、C 可并行）

- [x] 2.1 在新顺序迁移和 `repository/sqlite` 增加计划、批次、运行关联、CAS 租约/唯一窗口、credential_version 观测字段与索引；以新旧隔离 DB 迁移/回滚备份恢复、双连接同窗口争抢、fencing、版本不符测试验证。
- [x] 2.2 在 `application/probe` 与 `cmd/csp/main.go` 接入 `time.Timer`、现有 runner/有界公平队列，完成当前激活库存全量分片、凭据 fail-closed、过期恢复/失租约中止、手动任务公平和停服 Drain；以 fake clock、双实例共享测试 DB、可控本地探测目标和生命周期测试验证“无重复批次/无漏报/无秘密/DB 关闭顺序”。
- [x] 2.3 在 `transport/http/probes.go` 接入计划查询/修改、批次分页/取消与脱敏审计，管理端授权和手动 API 保持兼容；以 HTTP 契约测试（未授权、非法预算、关闭计划、空仓库及有/无凭据）验证。

## 3. 特性包 B：两级筛选解析/发布端到端（公共契约冻结后与包 A、C 可并行）

- [x] 3.1 在独立迁移（与包 A 的迁移编号/写集合先协商冻结，避免冲突）及 `repository/sqlite` 实现全局与组筛选存储、真实来源及最新观测批量加载与索引；验证旧配置默认恒真、无 N+1、版本未知旧观测失效和独立数据库事务一致读取。
- [x] 3.2 在 `application/policy`、`resolver`、`compiler`、`application/publication` 实现严格字段校验、激活/风险/准入→全局→组条件、动态/显式节点、父子上下文可编译投影、摘要与同源预览/预检/发布；以多父嵌套组、跨目标配置、不同过滤顺序、过期/未知/错误观测、空路由组阻断和旧策略字节级/语义回归用例验证。
- [x] 3.3 在 `transport/http/policies.go` 与全局筛选 admin 路由添加可选 `node_filter` 和全局筛选 GET/PUT，PATCH 省略/清空区别、错误字段/认证与预览诊断；以 HTTP 契约测试和完整配置生成核验不泄漏秘密、不引用不存在节点。

## 4. 特性包 C：前端管理体验（公共 API 契约已冻结；前端源码与后端剩余修复写集合不交叉时可并行施工，按完整特性包自测交付）

- [x] 4.1 对照 `internal/transport/http/probes.go`、`internal/domain/probe_schedule.go` 核实 `web/src/features/probes/{probeTypes,useProbes,ProbesView}.vue/ts`、`web/src/api/client.ts`：计划 GET/PUT、批次列表/详情/取消、`run_ids` 关联、失败/跳过及脱敏证据、启停和无库存/过期反馈；`ApiError` 的 401 走既有认证回调，其他 4xx/5xx 和网络错误在计划/批次读取处必须可见（现有 `loadSchedule`/`loadBatches` 吞错误且 `cancelBatch` 乐观写死 cancelled，应据服务端实际状态修正）。补足 Vitest 的真实 Vue 组件交互 + API mock（启用/停用、取消成功/失败、过期、空库存、401/403、无密钥泄漏），与现有 composable 测试一起退出 0；完成源码、接口、组件和集成自测前不勾。
- [x] 4.2 对照 `internal/transport/http/policies.go`、`internal/application/policy/types.go`、`internal/resolver/types.go` 核实 `web/src/features/policy/{policyTypes,usePolicy,PolicyView,PolicyEditorSheet,GroupCard}.vue/ts` 和发布预检页：全局及组级筛选 GET/PUT/POST/PATCH 的省略与显式清空语义、字段级错误、预览各层计数/排除原因/空路由组阻断；`loadGlobalFilter` 不得把 401/403/5xx 当作空筛选，UI 仅消费服务端诊断，不另造筛选引擎。补 Vitest 的 Vue 组件/API mock、键盘交互测试，并在隔离可访问预览中核实小屏实际可操作、无溢出/遮挡及编辑-预览-预检一致；所有适用测试退出 0 后勾，黑盒独立裁决仍归 5.2。

## 5. 汇聚验收与安全发布边界（4.1/4.2 与任何后端定点修复自测结束后执行）

- [x] 5.1 保持 Go 每包 race 检查 `-race -count=3` 覆盖不变，勿把同一条全量 race 静默 337 秒遭外层运行时中断误判为 race 检出或成功：按 `go list ./...` 的**完整包清单**确定性拆成连续三组（各 9 包；当前 27 包），逐组串行执行 `go test -race -count=3 <组内所有包>`；每组将 stdout/stderr 和退出码保存在隔离临时日志/状态记录，标明开始与结束时间、包清单并显示阶段进度；必须在调用环境单次运行时预算内执行并获取真实子进程退出码。若某组超预算，使用系统已有 `timeout` 给组设置可见期限（其退出码 124/137 不代表 race 失败或 PASS），在未减少 `-race -count=3` 和未遗漏任何包的前提下把该组进一步拆小、串行重跑并记录原组超时及子组真实退出码；若整个受控运行环境无法提供足够时间，移交可持续的独立执行环境并保留未完成状态，绝不静默视为通过。核对组并集等于 `go list ./...` 且无重复无漏，三组及必要子组均真实 exit 0；对检出的 race/失败定点修复后重测。随后在隔离临时 SQLite DB 和受控本地探测目标验证周期/手动竞争、重启、全局→组筛选预览/预检/五目标编译一致；`go build ./...`、`go test ./...`、前端 `npx vue-tsc --noEmit`（项目当前无 `npm run type-check` 脚本）、`npm test -- --run` 或项目已有等价 Vitest 命令各真实 exit 0，保留命令/退出码/失败复现与修复证据。本项是施工自测门禁，不代替独立审查。
- [x] 5.2 先在严格无副作用隔离工作副本完成前端构建/预览：核对 `web/vite.config.ts` 的 `closeBundle` 仅当 `COPY_WEBASSETS=1` 才拷贝，显式 `NO_COPY_WEBASSETS=1`、使用隔离输出/副本，并比对原仓 `internal/webassets/dist` 前后基线；不得在原仓默认构建、不得连生产 17000/18080/生产卷。待 4.1/4.2/5.1 真实完成，独立 reviewer 对最终合流 diff、迁移、鉴权、凭据脱敏、出站、前后端 JSON 契约、全量测试证据作安全/代码复验并 PASS；准备者提供健康检查通过的**隔离 URL**，critic 针对周期探测启停/批次取消/失败反馈、全局→组筛选/空组阻断在实际桌面与小屏视口做只读 FLOW 和 VISUAL 双 PASS（截图与交互/几何证据）。任何 REJECTED/FAILED/BLOCKED/PRECONDITION_FAILED 或缺少隔离 URL 均保持本项 `[ ]`，按根因整改复验，不能以文档勾选替代验收。生产备份、部署、重启、端口及线上数据、Git 提交均不在本 Change 本地验收授权内。

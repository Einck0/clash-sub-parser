# Tasks: CSP Clean-Slate Go Rewrite 1.0

## 1. 安全快照、基线封存与只读演练准备

- [x] 1.1 `[ic_csgo_1_1]` 创建 Git 安全快照分支 `backup/pre-go-rewrite-dev` 与 Tag `pre-go-rewrite-snapshot` 封存当前 11 个提交与 44 个修改文件，并将生产 SQLite 数据库冷备至 `/home/service/clash-sub-parser/backups/clash_sub_parser_pre_go_rewrite_20260911.db`，验证备份文件存在且大小与生产库一致（约 44.38 MB）
- [x] 1.2 `[ic_csgo_1_2]` 编写独立只读数据校验工具（基于 `modernc.org/sqlite`），核验生产库 7,236 节点、8 订阅、29 策略组、470 规则与 7,297 探测记录，验证行数及关键配置 SHA-256 校验和 100% 匹配
- [x] 1.3 `[ic_csgo_1_3]` 建立 Critic RFC 架构对抗审查门禁，完成高并发拨号内存膨胀、CGO-free SQLite 驱动稳定性与物理出口隔离的威胁模型质检，输出 Critic 裁决记录

## 2. 领域数据层与存储引擎 (internal/domain & internal/repository)

- [x] 2.1 `[ic_csgo_2_1]` 初始化 Go 模块（`go.mod` 声明 Go 1.25+，包划分遵循 `cmd/server/`, `internal/domain/`, `internal/repository/`, `internal/probe/`, `internal/compiler/`），验证 `go vet ./...` 检查通过
- [x] 2.2 `[ic_csgo_2_2]` 基于 `modernc.org/sqlite` 实现无 CGO 纯 Go 数据库仓储层（WAL 模式、busy_timeout 30s、连接池读写隔离），实现 `Subscription`, `Node`, `NodeGroup`, `Rule`, `NodeProbeResult` 的读取与写入接口，运行单元测试全绿通过
- [x] 2.3 `[ic_csgo_2_3]` 冻结与生产 SQLite 数据库的双模式字段映射契约（包含 `node_probe_results` 增量写入与 `probe_observations` 统一存储），通过只读回放测试验证 7,297 条历史探测记录完整反序列化

## 3. 内存级高并发探测引擎 (internal/probe)

- [x] 3.1 `[ic_csgo_3_1]` 参考 `sinspired/subs-check-pro` 实现 Goroutine 滑动窗口工作池（支持 100~500 并发），支持 Context 取消、任务超时控制与软内存动态限速（`debug.SetMemoryLimit` 150MB~200MB），运行并发限流单测全绿通过
- [x] 3.2 `[ic_csgo_3_2]` 深度集成 `github.com/sagernet/sing-box` 官方库（v1.14.0），实现纯内存 Dialer 管道，覆盖 VLESS/Reality, VMess AEAD, Hysteria2, Trojan, Shadowsocks 协议出站，验证 0 子进程、0 磁盘临时配置、0 本地端口占用
- [x] 3.3 `[ic_csgo_3_3]` 实现分级探测管线（204 延迟、出口 IP/地理识别、流媒体与 AI 解锁），显式禁用系统环境代理（`HTTP_PROXY`/`ALL_PROXY`），运行离线 Mock 测试验证物理出口隔离与断网假阳性拦截

## 4. 五大客户端配置编译器 (internal/compiler)

- [x] 4.1 `[ic_csgo_4_1]` 实现 CanonicalGraph 中间图模型与正则表达式动态展开引擎，支持 29 个节点分组、470 条分流规则与跳板链（Proxy Chain）无环解析，运行循环引用死锁探测单测通过
- [x] 4.2 `[ic_csgo_4_2]` 实现五大客户端原生序列化适配器（Clash/Mihomo/Stash YAML, Sing-box JSON, Surge/Loon/QX INI/Text），运行 Golden Fixtures 测试验证与 Python 旧版输出逐行一致且管理敏感字段（tokens/passwords）完全脱敏
- [x] 4.3 `[ic_csgo_4_3]` 建立配置编译接口安全门禁，确保非法或已废弃的 target（如 `/script`）严格返回 HTTP 404，通过单元测试验证

## 5. Web 控制面、前端内嵌与容器单二进制交付 (cmd/server)

- [x] 5.1 `[ic_csgo_5_1]` 基于 `go-chi/chi` 构建轻量 HTTP 路由引擎，实现完整 `/api/probe/*`, `/api/subscriptions/*`, `/api/rules/*`, `/api/generate/*` 路由并保留原有 URL 路径契约，运行 HTTP API 集成测试通过
- [x] 5.2 `[ic_csgo_5_2]` 利用 Go 标准库 `embed.FS` 将现有的现代前端编译产物（`frontend/dist/`）内嵌进 Go 二进制，配置 SPA fallback 路由并实现单二进制独立启动与健康检查（`GET /health` 返回 200）
- [x] 5.3 `[ic_csgo_5_3]` 编写多阶段 Dockerfile（构建镜像 `golang:1.26-alpine`，运行镜像最小化 Alpine），编译生成单一二进制文件（约 25~35MB），在测试容器内验证单二进制内存占用稳定在 20~35MB
- [x] 5.4 `[ic_csgo_5_4]` 运行生产切换演练（Migration Dry-run），确认生产 7,236 节点与 7,297 探测数据无损挂载，完成 Reviewcommon 双轨合规审查与 Critic 终态体验复核，最终由 Planner 执行受控切换

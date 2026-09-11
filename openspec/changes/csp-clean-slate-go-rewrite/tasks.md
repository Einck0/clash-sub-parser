# Tasks: CSP Clean-Slate Go Rewrite 1.0

## 1. 安全快照、对标基准与只读迁移演练准备

- [ ] 1.1 对当前工作区创建安全 Git 保护分支与 Tag，封存当前的 11 个提交与 44 个未提交前端工作台成果；对生产 SQLite 卷执行冷备份至 `/home/service/clash-sub-parser/backups/clash_sub_parser_pre_go_rewrite.db`
- [ ] 1.2 编写只读数据迁移验证工具契约（基于 `modernc.org/sqlite`），比对生产源库 7,236 节点、8 订阅、29 策略组、470 规则及 7,297 条历史探测结果，输出字段级一致性校验报告
- [ ] 1.3 确立以 `sinspired/subs-check-pro` 为并发探测对标、`tindy2013/subconverter` 为协议解析对标、`flosch/pongo2` 为模板渲染对标的技术实现规范，编写子机执行任务指引

## 2. 领域数据层与存储引擎 (cmd/ & internal/)

- [ ] 2.1 初始化标准 Go 模块结构（`go.mod` 声明 Go 1.25+，包划分遵循 `cmd/server/`, `internal/domain/`, `internal/repository/`, `internal/service/`, `internal/probe/`）
- [ ] 2.2 基于 `modernc.org/sqlite` 实现无 CGO 纯 Go 数据库连接池与仓储层（WAL 模式、busy_timeout 30s），支持 `Subscription`, `Node`, `NodeGroup`, `Rule`, `NodeProbeResult` 读写
- [ ] 2.3 集成 `flosch/pongo2` 模板渲染引擎，导入并验证现有的 Clash、Mihomo、Stash、Shadowrocket 配置模板，确保语法零修改通过

## 3. 内存级高并发探测引擎 (internal/probe/)

- [ ] 3.1 参考 `subs-check-pro` 实现 Goroutine 滑动窗口工作池（支持 100~500 并发），实现任务派发、完成通知与整体 Context 取消
- [ ] 3.2 深度集成 `github.com/sagernet/sing-box` 官方库（v1.14.0），实现内存级 Dialer 管道，支持 VLESS/Reality、VMess AEAD、Hysteria2、Trojan、Shadowsocks 等协议出站
- [ ] 3.3 实现传输延迟（204）、真实出口 IP/地理识别、多流媒体与 AI 解锁检测 pipeline，严格禁用系统环境代理，保持物理出口绝对隔离
- [ ] 3.4 设立软内存防线（`debug.SetMemoryLimit` 150MB），实现内存压力下的工作池动态降速，避免 OOM 崩溃

## 4. API 服务、5-Target 导出与前端内嵌 (internal/server/)

- [ ] 4.1 基于 `go-chi/chi` 构建轻量 Web 路由，实现完整 `/api/probe/*`, `/api/subscriptions/*`, `/api/rules/*`, `/api/generate/*` 接口
- [ ] 4.2 实现 `/clash`, `/mihomo`, `/stash`, `/shadowrocket`, `/sing-box` 五大核心导出端点与客户端 Scheme / 二维码生成
- [ ] 4.3 使用 Go 标准库 `embed.FS` 将编译后的前端工作台产物（`frontend/dist/`）内嵌至二进制文件，实现零外部依赖的单文件 SPA 托管

## 5. 容器构建、全链路回归与无损切线

- [ ] 5.1 编写多阶段 Dockerfile（构建阶段采用 `golang:1.26-alpine`，运行阶段采用最小化 Alpine 镜像），并配置健康检查与平滑重启
- [ ] 5.2 运行端到端测试与 Golden Fixtures 对比，验证五大客户端导出配置与 Python 旧版逐行一致
- [ ] 5.3 在临时副本上完成只读全量迁移演练；经用户确认后执行正式容器镜像切换，保留旧 Python 容器为秒级回退资产
- [ ] 5.4 运行 `openspec validate csp-clean-slate-go-rewrite --strict`，完成全量交付物校验

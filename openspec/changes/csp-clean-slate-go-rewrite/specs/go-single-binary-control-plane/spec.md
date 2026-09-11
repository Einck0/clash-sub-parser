## Purpose

Delivers a unified Go single-binary control plane that embeds modernized Vue 3 web assets, serves high-performance HTTP APIs, and manages application lifecycle with graceful signal shutdown.

## ADDED Requirements

### Requirement: Go 单二进制独立运行与静态资源内嵌
系统 SHALL 编译为单一的静态 Go 二进制文件，利用 `embed.FS` 将前端现代化工作台（Vue 3 + Tailwind CSS + shadcn-vue）内嵌，无需外部 Node.js 或 Python 运行时即可独立启动 HTTP 服务。

#### Scenario: 独立单二进制启动与健康检查
- **WHEN** 在空白 Linux 容器环境中直接执行 `/app/clash-sub-parser`
- **THEN** 服务成功监听指定端口（默认 18080），`GET /health` 立即返回 HTTP 200 `{"status": "ok"}`，且访问根路径 `GET /` 正确返回内嵌的前端 `index.html`

### Requirement: 优雅启停与资源生命周期治理
系统 SHALL 监听 `SIGINT` 和 `SIGTERM` 信号，在收到退出信号时优雅中断正在执行的高并发探测任务并释放数据库连接与网络套接字，且支持最大 10 秒优雅关机超时。

#### Scenario: 收到终止信号时的并发任务优雅回收
- **WHEN** 正在执行批量 100 并发节点探测时接收到 `SIGTERM`
- **THEN** 系统立即关闭任务调度通道，等待已发起的在途 HTTP 握手退出或超时，完成最后的数据库事务提交后正常退出，退出码为 0

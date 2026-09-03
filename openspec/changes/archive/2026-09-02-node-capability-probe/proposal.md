## Why

当前 `clash-sub-parser` 仅具备简单的 TCP 端口探测（`tcp_probe_service.py`），无法验证节点的实际代理协议握手、真实出口 IP/国家、流媒体/AI 解锁状态（如 Netflix、YouTube Premium、Disney+、ChatGPT）以及真实下载速度。为了实现与 `subs-check` / `subs-check-pro` 类似的高级节点质检与自动清洗能力，需要在保持项目轻量化、控制面与数据面解耦的前提下，引入系统化的节点能力探测与测速架构。

## What Changes

- 新增基于 `sing-box` 轻量 runner 的多协议（Shadowsocks, VMess, VLESS/Reality, Trojan, Hysteria2 等）单节点隔离出站探测管线。
- 新增真实出口识别与地理位置共识模块（出口 IP、国家代码、ASN 共识校验）。
- 新增分级流媒体与 AI 解锁探测模块（YouTube Premium 地区、Netflix 鉴权/全解锁、Disney+、ChatGPT 连通性）。
- 新增受控单线程下载测速模块（小样本流量测速、带宽估算、严格并发控制与熔断保护）。
- 扩展后端数据库与 API，持久化存储节点健康状态、延迟、测速结果与解锁能力标签，并支持按能力标签（如 Netflix 解锁、低延迟）智能生成订阅。
- 前端节点列表与生成页面支持直观展示测速与流媒体解锁状态徽标。

## Capabilities

### New Capabilities
- `node-capability-probe`: 节点全协议真实代理握手、出口地理位置识别、流媒体与 AI 解锁探测以及受控测速能力。

### Modified Capabilities
- 无（现有订阅转换与分组核心逻辑保持兼容）。

## Impact

- **Backend**: 新增 `app/services/probe/` 模块（含 runner、providers、speedtest），扩展 SQLAlchemy 模型存储节点能力与历史检测结果，新增 `/api/probe/*` 路由。
- **Runtime/Docker**: `Dockerfile` 引入对应架构的静态 `sing-box` 二进制文件作为节点执行器。
- **Frontend**: `frontend/src/` 扩展节点列表的标签展示与探测触发交互。

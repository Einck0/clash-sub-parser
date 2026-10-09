## Purpose

定义基于进程内 Mihomo 适配器的节点探针出站拨号与 HTTP 客户端契约，确保真实协议参数完整透传、远端域名代理解析并杜绝宿主代理泄漏。

## ADDED Requirements

### Requirement: 进程内代理适配与连接参数完整保留
系统 SHALL 在探针出站层使用进程内 Mihomo `adapter.ParseProxy` 构造单节点代理出站，严格遵循节点原生连接配置，正常支持 `skip-cert-verify`（跳过 TLS 证书验证）、Hysteria2 `ports`（端口跳跃）以及 TUIC `disable_sni` 等常用协议参数。系统 MUST 废除针对自签名证书或特定协议选项的人为主观拦截，以代理内核真实协商语义为准。

#### Scenario: 包含 skip-cert-verify 的节点正常建立出站
- **WHEN** 探针接收到标记有 `skip-cert-verify: true` 的 Trojan 或 VMess 节点
- **THEN** 系统构建对应出站代理实例并在 TLS 握手中遵循该设置，不抛出 `unsafe_tls_rejected` 异常

#### Scenario: 包含端口跳跃配置的 Hysteria2 节点正常拨号
- **WHEN** 探针调度包含 `ports` 端口跳跃范围参数的 Hysteria2 节点
- **THEN** 代理适配器如实将端口范围透传至底层协议栈，不拒绝创建客户端

### Requirement: 宿主代理严格隔离与远端域名代理解析
系统构建的每个节点独立 HTTP Client MUST 显式将底层 Transport 的 `Proxy` 设置为 `nil`，绝不允许使用或继承宿主机运行环境中的 `HTTP_PROXY`、`HTTPS_PROXY` 或 `ALL_PROXY` 环境变量。探针发起 HTTP 请求时，系统 MUST 将目标主机名直接装载入目标 Metadata (`Host`) 中并通过节点隧道向远端发起拨号，严禁在本地宿主机执行直接 DNS 解析，防止本地 DNS 污染及真实出口偏差。

#### Scenario: 宿主机配置系统代理时探测请求不被穿透
- **WHEN** 宿主操作系统环境中存在 `HTTP_PROXY=http://127.0.0.1:7890` 环境变量
- **THEN** 探针发出的所有探测流量严格仅通过被测节点出站隧道建立，不经过宿主机代理

#### Scenario: 目标域名由远端节点代理解析
- **WHEN** 探针对 `https://api.openai.com` 或 `https://www.netflix.com` 发起探测
- **THEN** 本地操作系统不执行针对目标域名的 A/AAAA 记录解析，域名直接由代理链路远端协商解析

### Requirement: 独立客户端生命周期管理与挂起看门狗
系统 SHALL 为每个待测节点建立独立的客户端生命周期上下文。当节点生命周期结束或 Run 取消时，系统 MUST 触发 `Close()` 并释放相关 Transport 空闲连接与套接字句柄。系统 MUST 配备看门狗保护机制，防止底层网络握手无限期挂起导致的协程与句柄泄漏。

#### Scenario: 探测超时触发看门狗强制释放
- **WHEN** 某个节点在传输层握手陷入非响应阻塞超过预设生命周期阈值
- **THEN** 看门狗协程主动触发底层代理 Close 并释放连接，主调度循环不被死锁

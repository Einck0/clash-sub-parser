## Purpose

规范探针出站构建器网络层与传输层解耦、UDP 真实能力保障、Reality 凭据校验、脱敏构建诊断与握手健康分离契约，确保探测引擎真实反映协议状态。

## ADDED Requirements

### Requirement: 探针出站构建器 L4 网络层与传输层严格解耦
系统 SHALL 在 Sing-box 出站配置构建器中将底层的 L4 网络协议（如 `tcp`、`udp`）与传输层流包装协议（如 `ws`、`grpc`、`http`、`quic` 等）严格解耦。构建器 MUST 遵循底层 Sing-box 核心库的实际协议能力，出站的 `Network` 字段仅填入合法的 L4 网络协议列表（如 `tcp` 或 `udp` 或 `["tcp", "udp"]`），严禁将传输层名称（如 `ws`、`grpc`）作为 L4 网络类型注入。对于支持 UDP 的协议（如 Shadowsocks、Trojan、VLESS、VMess、Hysteria2、TUIC），系统 MUST 尊重配置中的真实网络需求，严禁无条件强制锁定为纯 TCP，确保节点的 UDP 探测与数据报能力真实可用。

#### Scenario: 包含 WebSocket 传输的 VLESS 节点正确区分 L4 与 Transport
- **WHEN** 构建一个网络类型为 `ws`、传输层承载于 TCP 上的 VLESS 出站配置
- **THEN** 系统生成的出站配置中 Network 设置为 `tcp`，并在 Transport 字段配置完整的 WebSocket 传输选项，而不是在 Network 中填入 `ws`

#### Scenario: 声明 UDP 传输或双栈协议的节点正确保留 UDP 能力
- **WHEN** 构建一个声明支持 UDP 转发或基于 QUIC/UDP（如 Hysteria2、TUIC）的节点配置
- **THEN** 系统在出站配置的 Network 字段中正确包含 `udp`，不被强制降级或改写为单 `tcp`

### Requirement: Reality 协议公钥标准化与 Short ID 严格格式校验
系统 SHALL 对 Reality 协议的相关出站参数实施严格的规范化与合规性校验。对于公钥参数（`RealityPublicKey` / `pbk`），系统 MUST 验证其经 Base64 或 URL-Safe Base64 解码后为恰好 32 字节的合法公钥，并将其标准化为 Sing-box 库所接受的统一编码格式；若公钥缺失、长度非法或字符集损坏，系统 MUST 明确拒绝构建并抛出可诊断的参数校验错误。对于 Short ID（`RealityShortID` / `sid`），系统 MUST 严格遵循 Reality 协议规范，要求为偶数长度的十六进制字符（0 到 16 字节十六进制串）；若传入奇数长度十六进制串、非法非十六进制字符，系统 MUST 明确拦截并返回规范错误，严禁通过剥离非法字符、向后静默补零或自动补位来篡改节点身份特征。

#### Scenario: 合法 32 字节 Reality 公钥正常标准化并构建
- **WHEN** 传入合法标准 Base64 或 URL-Safe Base64 编码且解码后长度为 32 字节的 Reality 公钥
- **THEN** 系统成功通过校验，标准化编码并注入 OutboundRealityOptions 完成出站构建

#### Scenario: 奇数长度或非法字符的 Short ID 被拒绝
- **WHEN** 传入长度为 7 位（奇数长度）或包含字母 `g-z` 的 Reality Short ID
- **THEN** 系统构建器拒绝该配置，返回明确的校验错误 `invalid_reality_short_id`，不静默补零或吞并错误

### Requirement: 实际库能力测证与真实构建错误脱敏诊断
系统 SHALL 对底层使用的 Sing-box 核心库真实协议支持能力（包括 `xhttp` 等前沿传输模式）以真实的本地构建测试进行验证，严禁仅凭文档或静态声明假设其支持。在安全拨号器与客户端构建流程中（如 `safe_dialer.go`），当底层核心库构建失败时，系统 MUST 捕获底层原始错误，按照错误类别（如协议配置无效、TLS 证书冲突、网络类型不支持、传输层参数不匹配等）进行结构化归因与分类诊断，并向调用方返回具备可诊断性的错误上下文。系统在输出错误诊断与日志时，MUST 严格脱敏，严禁包含任何明文密码、UUID、Token、URL query 参数、私钥或公钥等敏感凭据。系统 MUST 维持既有的 SSRF 防护、私网 IP 拦截与 TLS 证书严格校验安全边界，绝不得为了使探测通过而放宽安全拦截。

#### Scenario: 捕获构建异常并脱敏输出分类诊断
- **WHEN** 传入格式损坏的传输层参数导致 Sing-box 客户端创建失败
- **THEN** 安全拨号器捕获底层错误，返回包含错误归类与脱敏原因的错误，诊断文本中不暴露 URL 敏感 query 与私钥等凭据

#### Scenario: 安全防护与私网拦截边界不放宽
- **WHEN** 探测目标指向 RFC1918 私有地址空间或回环地址（如 `127.0.0.1`、`192.168.1.1`）
- **THEN** 安全拨号器在预检阶段严格拦截并返回 SSRF 拒绝错误，不进入底层拨号流程

### Requirement: 本地配置构建成功与远端网络握手健康严格分离
系统 MUST 在探测状态机中将“本地 Sing-box 客户端配置成功构建”与“远端链路网络握手真实成功”作为两个独立阶段进行判定和证据记录。本地配置构建成功仅代表该节点的配置与语法符合协议要求（build succeeded），绝不能等同于远程节点连接可用或探测健康；只有在经过真实的网络层握手、协议协商且成功获取预期的探测响应（或无重定向成功响应）后，系统方可将该探测观测结果裁决为健康（healthy）。任何本地语法通过但远端连接超时、拒绝连接或 TLS 握手失败的情形，系统 MUST 将其记录为远端网络探测失败（failed/unhealthy），绝不允许伪造虚假健康态。

#### Scenario: 本地构建通过但远程网络超时判定为探测失败
- **WHEN** 节点配置完全合法通过本地构建器生成了客户端，但向目标地址发起连接时遭遇超时（i/o timeout）
- **THEN** 系统记录构建状态为成功，但该次探测的最终观测结论裁决为超时失败（failed），不将其标记为 healthy

#### Scenario: 本地构建与远程握手均成功方可判定健康
- **WHEN** 本地构建通过且与远端测试目标成功完成握手并取得状态码 200/204
- **THEN** 系统将该节点在该 connection_revision 下的观测记录更新为 healthy 并记录真实延迟

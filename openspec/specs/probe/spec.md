# Probe Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的主动探测引擎（Active Probing Engine）与健康证据链体系。系统必须以生产级 Mihomo 原生拨号器为核心执行网络连通性与 RTT 延迟检测，支持多层级周期解耦调度与单节点优先插队机制，建立科学的健康基线判定模型，并确保观测数据的时效性防篡改与精准归因。

## User Scenarios & Priority

### User Story 1 - 周期性轻量化健康巡检 (Priority: P1)
- **GIVEN** 节点池中存在大量存量代理节点
- **WHEN** 周期调度器运行时
- **THEN** 系统以 1 分钟轻量级 tick 扫描过期节点，应用 5 分钟失败冷却，按 1 小时基线周期增量送检，并保证 24 小时内的有效观测作为健康依据

### User Story 2 - 管理员控制台单节点测速插队 (Priority: P1)
- **GIVEN** 探测队列中正在排队执行批量后台任务
- **WHEN** 管理员在前端节点列表点击某个特定节点的“即时测速”按钮
- **THEN** 探测引擎优先队列立即将该节点置顶插队，并在秒级内返回真实探测结果与时延

### User Story 3 - 探测失败诚实归因与时效防穿越 (Priority: P2)
- **GIVEN** 某个节点发生真实网络故障或证书错误
- **WHEN** 探测引擎执行失败归因时
- **THEN** 系统准确归类为网络超时、握手拒绝或鉴权失败，而非泛化兜底为构建失败；同时时间戳超出当前时间的未来观测不得通过新鲜度校验

---

## Requirements

### Requirement: 原生 Mihomo 探针执行引擎 [FR-PROB-01]
系统 SHALL 使用原生 Mihomo 拨号器（`mihomo.NewNodeDialer`）作为生产探测运行时的核心实现，支持常用代理协议与 TLS 参数，并在节点配置了非安全传输或跳过证书时正确映射 `skip-cert-verify: true`。

#### Scenario: 自签名证书节点探测
- **WHEN** 节点的传输配置声明了 SkipCertVerify 或 Insecure
- **THEN** Mihomo 适配层在构建拨号配置时开启 skip-cert-verify，成功完成 TLS 握手测试

### Requirement: 多层级探测周期解耦架构 [FR-PROB-02]
系统 SHALL 分层管理探测生命周期：
1. 增量协调器扫描周期（Sweep Tick）为 1 分钟；
2. 连续失败冷却窗口（Failure Cooldown）为 5 分钟；
3. 单节点默认观测调度基准周期为 1 小时；
4. 节点观测有效性与时效截断（Freshness & Expiry）为 24 小时。
各层参数职责解耦，不得相互混淆。

#### Scenario: 失败节点触发冷却保护
- **WHEN** 节点在本次探测中发生连接超时失败
- **THEN** 协调器在随后的 5 分钟冷却期内跳过该节点，防止对异常远端造成无效高频冲击

### Requirement: 单节点插队与优先队列调度 [FR-PROB-03]
探测池 SHALL 维护支持抢占式插队的优先队列。当接收到管理端针对指定 `logical_id` 的手动单检请求时，调度器 MUST 将该任务提升至队列头部优先派发。

#### Scenario: 后台大任务期间插队单测
- **WHEN** 探测池中有 50 个节点正在排队，用户触发了单节点 N 的探测
- **THEN** 节点 N 立即跨过 50 个排队节点进入下一个可用并发槽位执行

### Requirement: 探测观测证据与健康基线判定 [FR-PROB-04]
系统 SHALL 在 SQLite 中持久化探测观测记录（`probe_observations`）。健康评估模型 MUST 结合 RTT 往返时延与失败特征，输出 healthy、degraded、unhealthy、undetermined 四态基线。

#### Scenario: 高延迟节点判定为降级
- **WHEN** 节点的探测 RTT 持续超过系统设定的性能阈值但未发生丢包
- **THEN** 系统将该节点健康状态标记为 degraded（降级），供上层策略参考

### Requirement: 拨号失败原因分类归因精确性 [FR-PROB-05]
系统在捕获拨号异常时，SHALL 依据具体的网络错误、DNS 解析失败、TLS 握手超时等底层错误类型进行精确分类，不得在未发生客户端内部构造崩溃时将故障统一泛化为 `client_build_failed`。

#### Scenario: 网络超时精确归因
- **WHEN** 节点目标端口无响应导致 15 秒 context deadline exceeded
- **THEN** 诊断结果准确记录为 i/o timeout，而非客户端构建异常

### Requirement: 观测数据防未来时间穿越校验 [FR-PROB-06]
所有依赖探测观测时间戳的计算模块（包括 `probe_health` 与 `node_filter`）MUST 显式校验 `obs.ObservedAt.After(asOf)`。对于观测时间处于未来的异常数据，系统 MUST 拒绝将其判定为新鲜有效记录。

#### Scenario: 异常未来时间戳被拒绝
- **WHEN** 某个观测记录由于时钟不同步具有晚于当前系统时间的 ObservedAt
- **THEN** 节点过滤器与健康判定模块均拒绝该记录，返回状态为未测或未知

### Requirement: 辅助 Sing-box 探针构造器隔离 [FR-PROB-07]
系统中的 Sing-box 探针构造器（`internal/probe/singbox/builder.go`）作为辅助工具，SHALL 保持其严格的配置校验规范。其对不安全 TLS 的显式拒绝行为 MUST 严格隔离在自身组件内，不影响主线 Mihomo 探针的运行。

#### Scenario: 辅助构建器独立报错
- **WHEN** 调用 Sing-box 辅助构造器构建包含 SkipCertVerify 的选项
- **THEN** 构造器返回明确的 insecure_tls 验证错误，生产探测服务正常运行 Mihomo

---

## Success Criteria

### Measurable Outcomes
- **[SC-PROB-01]** 单节点手动插队端到端平均响应延迟低于 3 秒（受远端网络超时上限约束）。
- **[SC-PROB-02]** 多层周期解耦调度下，1000 节点常态后台 CPU 占用率稳定在轻量级水位。
- **[SC-PROB-03]** 探测健康基线四态判定在 100% 测试用例中无状态漂移或内存泄漏。
- **[SC-PROB-04]** 未来时间戳穿透漏洞修复后，非法时间戳新鲜度误判率为 0。

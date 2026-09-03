## MODIFIED Requirements

### Requirement: sing-box 隔离节点运行器
系统 SHALL 使用 sing-box 1.14+ 为被测节点生成隔离出站配置，通过并发安全回环端口池启动、监控并优雅终止子进程。每次探测 HTTP 请求 MUST 显式绑定该隔离代理并禁用环境代理继承

#### Scenario: 被测节点不可达而宿主可达
- **WHEN** 节点无法建立出站但宿主机可访问检测目标
- **THEN** 探测记录节点失败，绝不以宿主或系统代理出口作为节点成功结果

### Requirement: 分阶段真实能力探测
ProbeJob SHALL 按配置执行握手/TCP/TLS 延迟、出口 IP/地理、流媒体、AI 服务及受控测速。各阶段 MUST 独立分类，Netflix、YouTube Premium、Disney+、ChatGPT、Claude、Gemini 与 Meta AI 的受限、挑战或限流不得覆盖基础连通性结论

#### Scenario: 平台挑战响应
- **WHEN** 某 AI 或流媒体端点返回挑战或 429
- **THEN** 系统保存该服务的 challenge/rate-limited 状态，并保留独立的握手和出口结论

### Requirement: 受控并发、预算与取消
ProbeProfile SHALL 定义节点范围、阶段、并发、超时、下载字节和周期。执行器 MUST 在取消、禁用或预算耗尽后停止后续派发，回收端口与子进程，不遗留孤儿运行器

#### Scenario: 取消受控测速
- **WHEN** 操作者取消包含测速的批量 Job
- **THEN** 在途下载被终止或收尾，后续节点不再测速，已完成结果可查询

### Requirement: 旧核心的等价迁入保障
若新 Runner 无法通过协议、生命周期或真实出口回归，系统 SHALL 允许将 legacy snapshot 中已验证的 Runner、端口池和测试迁入 adapter 边界；迁入后对外 Job 契约保持不变

#### Scenario: 新运行器出现覆盖缺口
- **WHEN** parity matrix 发现某协议的握手或出站结论不等价
- **THEN** 该领域切换至已验证核心迁入轨道，不删除原能力且继续以同一验收用例验证
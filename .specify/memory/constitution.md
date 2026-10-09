# Clash Sub Parser (CSP) Project Constitution

- **Adoption Date**: 2026-10-09
- **Scope**: 全系统规范演进、架构设计与行为准则

## Core Principles

### Principle 1 - 资产零丢失铁律 (Zero Asset Loss on Fetch Failure)
订阅拉取异常（如网络超时、证书失效、上游返回 0 节点）时，系统必须保全存量有效节点与来源台账，绝对禁止对存量数据库执行级联物理清空或破坏性截断。

### Principle 2 - 节点逻辑身份全局稳定 (Stable Transmission Hash Identity)
节点规范化传输身份基于核心传输参数（协议、服务器、端口、认证密钥/UUID等）哈希分配全局唯一的 `logical_id`。跨订阅刷新、去重匹配与策略组引用必须保持该逻辑身份的恒定不变。

### Principle 3 - 六大领域清晰解耦 (Six Decoupled Capability Boundaries)
系统划分为 Inventory（资产台账）、Policy（策略编排与图解析）、Probe（节点健康探测）、Publication（多格式订阅发布）、Auth（会话安全与设置）、UI（人机交互与管理面板）六大正交能力。跨领域通信仅通过清晰的数据模型与接口，禁止深层隐式耦合。

### Principle 4 - 明文直存与极简工程 (Plaintext Direct Storage & Pragmatism)
遵循用户明确指示，节点凭据与连接参数在 `nodes` 表明文直接持久化，严格避免未经推演的过度加密分表与冗余中间件封装，做满足业务目标的最小完整实现。

### Principle 5 - 双向全量可追溯性 (Bidirectional Contract Traceability)
所有主干规范、计划与任务必须向后可追溯到原始用户需求或历史 OpenSpec 变更，向前可映射到实机代码行与自动化测试。严禁静默抹除未决分歧，待决事项必须透明记录于台账供用户裁定。

### Principle 6 - 活规范持续演化 (Living Specification Maintenance)
`specs/` 下的规范为系统行为的活契约（Living Contract）。当代码或需求变更时，规范必须优先或同步更新，避免文档与实机现实产生漂移。

## 1. 功能基线与迁移准备

- [ ] 1.1 从 `legacy-full-working-snapshot-20260903` 生成 feature-parity 矩阵，覆盖协议、订阅、探测、NodeLedger、策略/规则、跳板、五目标导出和除 SCRIPT 外的全部路由；验证每行有 legacy 证据、目标模块和验收用例
- [ ] 1.2 建立不可变 SQLite 在线备份副本、schema 清单和迁移 run manifest；验证源 hash、SQLite 版本、表行数和读取权限被记录且源卷未写入
- [ ] 1.3 为高风险 parser、sing-box probe、compiler 与 NodeLedger 确定默认/保底轨道；验证任一未达等价的领域有可执行旧核心迁入方案和回退点

## 2. 领域分层与数据模型

- [ ] 2.1 建立 API、application、domain、repositories、adapters、workers 边界并禁止跨层 ORM/HTTP 访问；验证架构依赖检查通过
- [ ] 2.2 实现 Source/Subscription/Node/NodeSourceLink/NodeSecret 与来源修订模型；验证 11 种输入协议、重命名、筛选、手工节点和去重/追溯 fixtures 通过
- [ ] 2.3 实现 NodeGroup/GroupEntry/RuleCategory/Rule/ProxyChainBinding 模型；验证 regex 动态展开、嵌套、排除、排序和跳板循环检测通过
- [ ] 2.4 实现 ProbeProfile/ProbeJob/ProbeResult、ConfigurationRevision/Snapshot 和设置模型；验证关联完整性、取消状态与历史不可变性通过

## 3. 订阅输入与真实节点探测

- [ ] 3.1 将旧订阅拉取和协议解析迁入 adapter 契约或以等价新实现替换；验证 SS、SSR、VMess、VLESS、Trojan、Hysteria2、TUIC、WireGuard、HTTP/HTTPS/SOCKS5 的 legacy golden fixtures 完全通过
- [ ] 3.2 实现安全 fetch、刷新失败语义和来源修订发布；验证重定向、SSRF、字节/超时限制、失败更新时间和删除竞态回归通过
- [ ] 3.3 复用或迁入 sing-box 1.14+ Runner、端口池和进程回收；验证 `trust_env=False`、节点失败不借宿主出口、取消无孤儿进程
- [ ] 3.4 实现出口/地理/国旗、流媒体、AI 和受控测速 provider；验证 Netflix、YouTube Premium、Disney+、ChatGPT、Claude、Gemini、Meta 的分类、429/挑战与节点连通性分离

## 4. 策略编译、发布与管理 API

- [ ] 4.1 实现唯一 canonical config graph 与节点选择解释；验证订阅候选、regex、探测门槛、跳板和规则在预览/发布中结果一致
- [ ] 4.2 实现 Clash、Mihomo、Stash、Shadowrocket、Sing-box renderer；验证五目标 golden 输出、节点集合、规则组、DNS 和敏感字段边界通过
- [ ] 4.3 实现 subscriptions、nodes、probes、groups、rules、chains、settings、snapshots 和 downloads 的领域 router；验证现有客户端关键请求、异步 Job 查询/取消和 SCRIPT 404 回归通过
- [ ] 4.4 保留 Quick Export 的单订阅/合并链接、客户端 Scheme 与二维码；验证每个支持目标可从 UI 和 API 获取正确分发内容

## 5. 前端重构与交互回归

- [ ] 5.1 按设计拆分 NodeLedgerView 与子组件，保持一个筛选 query/选择集；验证卡片/紧凑表格切换、组合筛选、分页/虚拟窗口和排序结果一致
- [ ] 5.2 完成批量/单节点探测、详情预览、复制、取消、清缓存与链路绑定 UI；验证并发按钮防重、错误反馈和抽屉草稿不丢失
- [ ] 5.3 重组订阅、策略组、规则、DNS、导出、设置和快照视图；验证订阅自动命名、候选节点隔离、动态 regex、快照恢复和危险操作确认回归通过

## 6. SQLite 迁移、演练与发布门禁

- [ ] 6.1 实现可重复的 SQLite 到目标库迁移命令和旧 ID 映射；验证事务失败全回滚、未知字段隔离报告和零静默丢失
- [ ] 6.2 在生产在线备份副本演练全量迁移；验证 source/target 审计至少匹配 4 sources、8 subscriptions、2,080 nodes、5,609 links、29 groups、470 rules、362 probe results、50 snapshots 或演练源的实际审计数
- [ ] 6.3 执行后端单元/集成、迁移、协议/编译 golden、探针隔离和 API 回归；验证 `pytest` 全绿且功能矩阵无未验收项
- [ ] 6.4 执行前端单元、类型检查和生产构建；验证 `vue-tsc --noEmit` 与 `npm run build` 通过，关键 NodeLedger/QuickExport E2E 通过
- [ ] 6.5 在独立环境完成目标库恢复、旧入口回退和容器/网络审计演练；验证任何失败不写旧 SQLite 且能恢复旧服务
- [ ] 6.6 用户确认后执行生产冻结、最终一致性校验、入口切换和观察；验证外网 API、五目标发布、真实节点探测和历史数据抽样均通过后才申请旧服务退役
- [ ] 6.7 运行 `openspec validate clean-slate-subscription-control-plane --strict`；验证所有规划工件保持一致
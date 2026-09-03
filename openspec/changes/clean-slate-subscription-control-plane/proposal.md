## Why

上一版计划将 CSP 作为空白、与旧系统物理切断的新产品，且主动拒绝旧代码、旧 API、旧 SQLite 数据和既有交互。这与 CSP 的实际价值及用户要求冲突：旧架构需要重构，但除 SCRIPT 外的成熟业务能力、历史数据和可回退资产必须完整保留。

本变更把重构重新定义为“以经过验证的旧行为为契约的分阶段架构演进”。它先建立功能与数据的可验证基线，再以模块化服务、规范化模型和可回滚迁移承接生产资产，而不是用空库和功能缩减冒充完成。

## What Changes

- **BREAKING（仅 SCRIPT）**：移除 `/script`、Script.js 注入/生成/下载和前端 Script.js 入口；不存在兼容替代。
- 保留全部节点输入、协议解析、sing-box 深度探测、NodeLedger、跳板链、规则/策略组、订阅筛选重命名、历史快照以及 Clash/Mihomo、Stash、Shadowrocket、Sing-box 分发能力；重构不得以“暂不支持”替代其中任一能力。
- 将实现收束为 `API Router -> 应用服务 -> 仓储 -> 存储/外部适配器`：旧稳定解析器、探针和配置编译逻辑先以有测试的核心模块迁入，再在明确契约内拆分，不以重写为前提。
- 以旧 SQLite 的一致性快照作为迁移输入，导入并校验来源、订阅、节点、节点-来源关联、策略组、规则、探测结果、DNS/生成/探测/安全设置及配置快照；迁移完成前旧实例继续可回退。
- 将长页面 NodeLedger 分解为复用组件与领域 store，保留卡片/紧凑表格、组合筛选、分页/虚拟滚动、批量探测、详情预览与链路绑定的现有操作语义。
- 以唯一的规范化中间模型驱动五个目标渲染器；客户端 Scheme 与二维码继续属于分发流程。
- 建立双轨实施门：优先以新模块实现并由功能矩阵验收；任一高风险领域出现覆盖缺口时，将旧已验证核心模块连同其测试迁入新边界，再逐步结构化，而不删除该能力。

## Capabilities

### Modified Capabilities

- `clean-slate-control-api`: 以保留旧功能的渐进 API 契约、迁移可观测性和 SCRIPT 移除替代旧路由 deny-list。
- `subscription-source-lifecycle`: 保留远程与手工订阅、筛选、重命名、来源修订和全协议输入。
- `node-capability-observation`: 保留真实节点出站、地理、流媒体、AI、测速和历史探测观测。
- `node-capability-probe`: 保留 sing-box 隔离运行、端口池、取消与受控并发探测。
- `node-probe-persistence-and-filtering`: 保留探测缓存、周期任务、阈值和能力筛选。
- `policy-configuration-compiler`: 保留动态 regex 策略组、规则、跳板链与五目标确定性编译。
- `configuration-release-management`: 保留发布、快照、预检、回滚、快捷导入和二维码分发。
- `control-plane-frontend`: 保留 NodeLedger 双视图及所有领域管理视图，按功能切片重组。
- `http-fetch-pipeline`: 保留安全订阅拉取、重定向处理和失败刷新语义。
- `config-transfer`: 保留受控配置导入/导出与恢复，同时迁移真实历史配置资产。
- `operational-safety`: 定义 SQLite 到目标存储的可校验、可演练、可回退迁移与上线门禁。

## Impact

- 规划阶段只修改本 OpenSpec change；不修改业务代码、测试、容器、生产数据库、入口或当前未提交 `control-plane/` 工作。
- 实现期将以 `legacy-full-working-snapshot-20260903` 和 `backup/legacy-full-working-state-20260903` 为行为基线；这些快照持续保留，不能被重写或作为迁移源写入。
- 目标架构可承接现有 PostgreSQL 控制平面，但只有在迁移审计通过后才能切换；旧 SQLite 不是可丢弃的启动样例，而是 2,080 节点、8 订阅、29 策略组、470 规则及历史探测的权威迁移输入。
- 生产初始化、数据迁移演练、入口切换和旧服务退役均属于独立的运行操作，须在实施完成和用户确认后执行。
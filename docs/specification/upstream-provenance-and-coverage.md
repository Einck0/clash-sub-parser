# CSP 上游变更追溯与主干规格继承契约 (Upstream Provenance & Inheritance Contract)

- **基线版本**：HEAD `1c5a604f9d4317dd7458710e30618dba13e26724`
- **编制日期**：2026-10-09 (Epoch 3 Remediation)
- **编制角色**：Executor（施工机 · 规格整合特性包）
- **规范治理模式**：『持续主干能力规范 (Living Specs, 37 项全局 FR) + 历史需求逐条登记与契约覆盖表 (Historical Requirement Register, 259 项 HR)』双层契约。
- **逐条登记清单索引**：请参阅 [`docs/specification/historical-requirement-register.md`](historical-requirement-register.md) 获取 300 项业务需求与 504 个源场景（目标映射 519 个独立场景，522 个场景链接引用，含 15 个显式新增迁移测试场景，源绝非 522）的逐条行锚登记明细；
- **全量补充契约归档索引**：请参阅 [`specs/007/plan-contracts.md`](../../specs/007-specification-governance/plan-contracts.md) 与 [`docs/migration/historical-contract-register.md`](../migration/historical-contract-register.md) 获取 809 项历史设计约束 (DOC-0001 ~ DOC-0354)、已完成任务履约凭证 (TSK-0355 ~ TSK-0806) 与架构决策 (DEC-0807 ~ DEC-0809) 的权威留存及独立物理锚点与 SHA-256 内容指纹归档。其中 261 项现行规划约束与 3 项架构决策权威留存于 `specs/007/plan-contracts.md`，58 项历史淘汰约束、35 项待审未决约束与 452 项历史任务作为不可执行归档参考留存于 `docs/migration/historical-contract-register.md`；
- **任务与业务闭环独立性**：452 项历史任务作为历史档案凭证留存（`verified=false`），不作为新验证凭证；业务待决问题与未决存根保持 open，Spec Kit 规范迁移独立于业务实现闭环。

---

## 1. 规范继承边界与免责说明 (Inheritance Boundary & Risk Disclosure)

1. **双层治理与真实逐条确权**：
   - 本文第 2 节呈现 26 个既有变更集（Change）与 63 份分项规范的**架构领域级追溯索引**；
   - 针对 63 份历史规范中包含的 **259 项需求 (Requirements)** 与 **463 个场景 (Scenarios)**，系统已通过独立工件 [`docs/specification/historical-requirement-register.md`](historical-requirement-register.md) 完成 100% 逐条行锚提取与编号（`HR-001` ~ `HR-259`）；
   - **严禁宣称“所有历史需求已 100% 整合实现”**：经实机审计，259 项历史需求中，按注册表明细表（Table Entries）真实精确统计：**232 项**收敛至主干能力已映射待复验，**3 项**属于显式历史冲突待裁定（HR-013, HR-098, HR-100），**24 项**属于特定历史未验证存根（UNRESOLVED）；第 2 节与注册表另归纳 6 组架构演进冲突主题。所有历史存根在对应变更集中永久保留，严禁擅自抹除或伪造全部已验证。
2. **多版本历史冲突显式暴露 (Surfaced Version Conflicts)**：
   历史变更在演进过程中存在多处设计演进与语义断层，本规范予以显式披露，严禁静默偏向当前代码事实：
   - **冲突 1：Clash 历史目标 vs 现代纯节点目标**：早期规范（如 `csp-clean-slate-v1`）曾允许为传统 Clash 导出策略组与规则；而在 `native-mihomo-export-clean-slate` 与 `csp-topology-i18n-and-node-targets` 中，系统确立了“仅 Mihomo 支持完整策略组规则，Sing-box/Surge/QX 严格仅导出纯节点”的协议边界；
   - **冲突 2：旧版 safe_dialer 冗余层废弃**：`csp-clean-slate-v1` 中设计的 `safe_dialer.go` 与 `client_factory` 已在后续的 `csp-debloat-over-engineering` 中被彻底删除，生产环境已收敛至原生 Mihomo 拨号器；
   - **冲突 3：探测周期 10m 说明 vs 1m 实现**：早期文档注释曾提及 10 分钟协调周期，当前代码常数为 1 分钟（`defaultSweepInterval`），属注释过时与历史假设偏差；
   - **冲突 4：历史 group_nodes 嵌套意图 vs 现行 group_edges 缺失**：历史 Python 与早期架构使用 `group_nodes` 表达“解包叶节点”，迁移至 Go 版（`000001_initial_schema.sql`）时仅保留 `include_group_ids`，导致解包叶节点能力出现物理断层。

---

## 2. 历史 26 变更集与 63 份分项规范领域追溯表 (Domain Provenance Index)

| 序号 | 变更集名称 (Change Name) | 内部分项规范路径 (Spec Path) | 主干领域与关联 FRs (Domain & Mapped FRs) | 追溯说明与历史需求存根状态 |
|---|---|---|---|---|
| 1 | `csp-audit-and-defect-remediation-v1` | `specs/audit-and-defect-remediation/spec.md` | `inventory` (`FR-INV-01`, `02`), `auth` (`FR-AUTH-01`) | 审计基线映射至资产保全与鉴权；其细分审计指标作为历史存根保留。 |
| 2 | `csp-auth-deadlock-v1` | `specs/control-plane-auth-lifecycle/spec.md` | `auth` (`FR-AUTH-04`) | 会话生命周期映射至 FR-AUTH-04；Cookie 续期历史条款保留。 |
| 2 | `csp-auth-deadlock-v1` | `specs/critic-headless-adversarial-verification/spec.md` | `auth` (`FR-AUTH-04`), `ui` (`FR-UI-01`) | 无头验收要求映射至会话与视口；对抗验证用例作为历史存根保留。 |
| 2 | `csp-auth-deadlock-v1` | `specs/frontend-credential-management/spec.md` | `auth` (`FR-AUTH-02`, `04`) | 前端凭据拦截映射至主令牌与会话；未逐条覆验每个拦截器分支。 |
| 3 | `csp-clean-slate-v1` | `specs/admin-workbench-experience/spec.md` | `ui` (`FR-UI-01`, `04`) | 基础工作台布局映射至 UI；历史组件细则保留于原变更中。 |
| 3 | `csp-clean-slate-v1` | `specs/clean-slate-import-and-operations/spec.md` | `inventory` (`FR-INV-01`, `03`) | 纯净导入映射至受控订阅与身份哈希；初始化命令行操作保留。 |
| 3 | `csp-clean-slate-v1` | `specs/control-plane-boundary/spec.md` | `auth` (`FR-AUTH-01`, `04`) | 控制面安全隔离映射至鉴权；端口硬绑定历史要求保留。 |
| 3 | `csp-clean-slate-v1` | `specs/policy-tree-and-compiler/spec.md` | `policy` (`FR-POL-01`, `05`), `publication` (`FR-PUB-01`) | 策略树与编译拓扑映射；历史 Clash 兼容条款已标注冲突。 |
| 3 | `csp-clean-slate-v1` | `specs/probe-evidence-engine/spec.md` | `probe` (`FR-PROB-01`, `04`) | 证据收集基础架构映射；其 safe_dialer 假设已被后续变更废弃。 |
| 3 | `csp-clean-slate-v1` | `specs/subscription-inventory/spec.md` | `inventory` (`FR-INV-01`, `03`, `05`) | 订阅台账映射；DNS 重定向校验等历史细节作为未消解存根保留。 |
| 4 | `csp-debloat-over-engineering` | `specs/debloat/spec.md` | `probe` (`FR-PROB-01`) | 冗余层精简规范；明确废弃 safe_dialer 与 client_factory。 |
| 5 | `csp-decoupled-auth-settings-and-publication-recovery` | `specs/baseline-auth-and-export/spec.md` | `auth` (`FR-AUTH-01`), `publication` (`FR-PUB-04`) | 鉴权基线映射；公开拉取与管理端越权隔离已合流。 |
| 5 | `csp-decoupled-auth-settings-and-publication-recovery` | `specs/decoupled-auth-settings/spec.md` | `auth` (`FR-AUTH-01`, `02`) | 双开关解耦与统一主令牌，用户核心诉求已全面合流。 |
| 5 | `csp-decoupled-auth-settings-and-publication-recovery` | `specs/publication-export-auth-recovery/spec.md` | `publication` (`FR-PUB-04`, `05`, `06`) | 双令牌验证与短路径映射；零发布向导历史细节保留。 |
| 6 | `csp-fresh-inventory-full-validation` | `specs/inventory/clean-slate-node-reset/spec.md` | `inventory` (`FR-INV-02`, `03`) | 节点重置映射至资产保全与身份守恒；历史快照回滚保留。 |
| 6 | `csp-fresh-inventory-full-validation` | `specs/maintenance/reset-node-inventory-cli/spec.md` | `inventory` (`FR-INV-03`) | 重置 CLI 命令映射至节点台账；CLI 参数未逐一纳入主干。 |
| 6 | `csp-fresh-inventory-full-validation` | `specs/policy/rules-validation/spec.md` | `policy` (`FR-POL-05`), `publication` (`FR-PUB-01`) | 规则校验与空组 PASS 语义映射；规则语法白名单保留。 |
| 6 | `csp-fresh-inventory-full-validation` | `specs/subscription/source-token-repair/spec.md` | `inventory` (`FR-INV-01`, `04`) | 来源 Token 修复映射至持久化存储；历史修复工具保留。 |
| 7 | `csp-global-probe-export-filter-and-overlays` | `specs/global-probe-export-filter/spec.md` | `probe` (`FR-PROB-04`), `publication` (`FR-PUB-01`) | 全局过滤映射至探测与发布；动态覆盖层参数作为存根保留。 |
| 7 | `csp-global-probe-export-filter-and-overlays` | `specs/viewport-safe-overlays/spec.md` | `ui` (`FR-UI-01`, `03`, `05`) | 视口遮罩与弹窗适配映射至 UI；无来源像素指标已被纠偏。 |
| 8 | `csp-incremental-expiry-probe-sweep` | `specs/incremental-expiry-probe-sweep/spec.md` | `probe` (`FR-PROB-02`, `04`) | 增量过期扫描映射至解耦周期；后台 worker 数量细节保留。 |
| 9 | `csp-ip-risk-v1` | `specs/admin-workbench-experience/spec.md` | `ui` (`FR-UI-01`) | 风险标签交互呈现映射至 UI；风险颜色等级细节作为存根保留。 |
| 9 | `csp-ip-risk-v1` | `specs/ip-risk-evidence/spec.md` | `probe` (`FR-PROB-04`) | IP 风险证据结构映射至观测证据；第三方提供商配置保留。 |
| 9 | `csp-ip-risk-v1` | `specs/ip-risk-management-workbench/spec.md` | `ui` (`FR-UI-01`, `05`) | IP 风险管理抽屉映射至 UI；历史批量操作细节保留。 |
| 9 | `csp-ip-risk-v1` | `specs/policy-tree-and-compiler/spec.md` | `policy` (`FR-POL-01`), `publication` (`FR-PUB-01`) | 风险过滤在策略树上的应用映射至策略与发布。 |
| 9 | `csp-ip-risk-v1` | `specs/probe-evidence-engine/spec.md` | `probe` (`FR-PROB-04`) | 风险检测管道映射至探测引擎；评分阈值作为存根保留。 |
| 9 | `csp-ip-risk-v1` | `specs/risk-policy-admission-and-publication/spec.md` | `publication` (`FR-PUB-01`, `02`) | 高风险节点准入与剔除映射至发布协议；严格阈值存根保留。 |
| 9 | `csp-ip-risk-v1` | `specs/subscription-inventory/spec.md` | `inventory` (`FR-INV-03`) | 风险标记与节点规范化映射；异步打标流水线细节保留。 |
| 10 | `csp-node-probe-pool-priority-queue` | `specs/node-probe-pool/spec.md` | `probe` (`FR-PROB-03`) | 优先队列与插队调度已合流；并发租约回收细节作为存根保留。 |
| 11 | `csp-periodic-active-probes-global-group-filters` | `specs/global-group-node-filters/spec.md` | `policy` (`FR-POL-06`), `probe` (`FR-PROB-04`) | 全局节点过滤器映射至动态解析；多组继承细节保留。 |
| 11 | `csp-periodic-active-probes-global-group-filters` | `specs/periodic-active-probes/spec.md` | `probe` (`FR-PROB-02`) | 周期性主动探测基准映射至解耦周期；心跳租约算法保留。 |
| 12 | `csp-policy-membership-probe-state-pagination` | `specs/policy/full-inventory-resolution/spec.md` | `policy` (`FR-POL-06`) | 全量节点动态解析映射至 FR-POL-06；破除 50 截断已证实。 |
| 12 | `csp-policy-membership-probe-state-pagination` | `specs/policy/membership-semantics-and-pagination/spec.md` | `policy` (`FR-POL-01`, `04`) | 成员连接边与交错保序映射；交错顺序断层已显式披露。 |
| 12 | `csp-policy-membership-probe-state-pagination` | `specs/probe/health-state-conservation-and-pagination/spec.md` | `probe` (`FR-PROB-04`), `ui` (`FR-UI-02`) | 探测健康守恒与分页联动映射；客户端 3 步翻页缺口已披露。 |
| 12 | `csp-policy-membership-probe-state-pagination` | `specs/ui/navigation-layout-and-viewport-resilience/spec.md` | `ui` (`FR-UI-01`) | 视口弹性展示映射至 UI；历史抽屉宽度细节保留。 |
| 13 | `csp-preserve-inventory-and-verify-business-results` | `specs/api-semantic-verification-matrix/spec.md` | `conformance-matrix.md` (全域) | API 语义矩阵基准吸收进符合度矩阵；历史测试用例集存根保留。 |
| 13 | `csp-preserve-inventory-and-verify-business-results` | `specs/probe-construction-and-truthful-diagnostics/spec.md` | `probe` (`FR-PROB-05`) | 真实探测构造与诚实诊断映射；默认归因缺口已显式披露。 |
| 13 | `csp-preserve-inventory-and-verify-business-results` | `specs/subscription-inventory-preservation/spec.md` | `inventory` (`FR-INV-02`) | 异常资产保全核心诉求映射至 FR-INV-02。 |
| 14 | `csp-probe-engine-human-centric-redesign` | `specs/probe-engine-ux/spec.md` | `ui` (`FR-UI-02`), `probe` (`FR-PROB-03`) | 探测人机交互改进映射至 UI 与优先队列；按钮微交互保留。 |
| 15 | `csp-probe-health-minute-state-sweep` | `specs/baseline-health-read-model/spec.md` | `probe` (`FR-PROB-04`) | 基线健康读取模型映射至观测证据；窗口排名细节存根保留。 |
| 15 | `csp-probe-health-minute-state-sweep` | `specs/minute-incremental-state-sweep/spec.md` | `probe` (`FR-PROB-02`) | 1 分钟轻量级扫描调度映射至 FR-PROB-02。 |
| 15 | `csp-probe-health-minute-state-sweep` | `specs/probe-evidence-classification/spec.md` | `probe` (`FR-PROB-04`, `05`) | 证据分类与延迟度量映射至探测；错误码枚举历史存根保留。 |
| 16 | `csp-probe-truth-and-efficiency` | `specs/probe-truth-and-efficiency/spec.md` | `probe` (`FR-PROB-01`, `03`) | 探测真实性与并发防雪崩调度映射至 Mihomo 引擎与队列。 |
| 17 | `csp-publication-theme-probe-recovery-v1` | `specs/publication-theme-probe-recovery/spec.md` | `ui` (`FR-UI-04`), `publication` (`FR-PUB-04`) | 发布页面暗黑适配与直链恢复映射至 UI 与发布。 |
| 18 | `csp-subscription-proxy-and-verge-headers` | `specs/subscription-proxy-and-verge-headers/spec.md` | `inventory` (`FR-INV-01`), `publication` (`FR-PUB-01`) | 抓取代理与客户端头部识别映射；特定客户端标识存根保留。 |
| 19 | `csp-topology-i18n-and-node-targets` | `specs/topology-i18n-export/spec.md` | `publication` (`FR-PUB-02`), `ui` (`FR-UI-06`) | 纯节点格式导出（Sing-box等）与国际化映射至发布与 UI。 |
| 20 | `csp-ui-device-governance-v1` | `specs/admin-ui-device-quality/spec.md` | `ui` (`FR-UI-01`, `06`) | 废除真机限制，修复 i18n 缺失键与抽屉联动映射至 UI。 |
| 21 | `csp-ui-hardcode-governance-v1` | `specs/admin-ui-adaptive-layout/spec.md` | `ui` (`FR-UI-01`) | 废除固定 44px 机械门禁，流式弹性布局映射至 UI。 |
| 22 | `finish-csp-live-validation-and-recovery` | `specs/live-acceptance-and-qualified-recovery/spec.md` | `conformance-matrix.md` (全域) | 生产实机验收契约映射至审计矩阵；停机窗口操作规范保留。 |
| 23 | `fix-csp-legacy-source-attribution` | `specs/inventory/node-source-history-api-view/spec.md` | `inventory` (`FR-INV-04`), `ui` (`FR-UI-05`) | 来源历史 API 与抽屉视图映射至资产台账与 UI 抽屉。 |
| 23 | `fix-csp-legacy-source-attribution` | `specs/inventory/node-source-history-ledger/spec.md` | `inventory` (`FR-INV-04`) | 来源变更台账（node_source_history）映射至 FR-INV-04。 |
| 23 | `fix-csp-legacy-source-attribution` | `specs/inventory/source-history-recovery-cli/spec.md` | `inventory` (`FR-INV-04`) | 历史数据修复 CLI 规范；CLI 参数作为运维存根保留。 |
| 23 | `fix-csp-legacy-source-attribution` | `specs/inventory/source-history-write-chain/spec.md` | `inventory` (`FR-INV-04`) | 来源变更事务写入链映射至 FR-INV-04。 |
| 24 | `native-mihomo-export-clean-slate` | `specs/native-mihomo-publications/spec.md` | `publication` (`FR-PUB-01`) | 原生 Mihomo 纯净导出管道映射至 FR-PUB-01。 |
| 25 | `redesign-csp-database-dataflow` | `specs/dataflow/downstream-matrix-and-publication/spec.md` | `publication` (`FR-PUB-01`, `02`) | 下游矩阵与多目标导出映射；格式转换矩阵存根保留。 |
| 25 | `redesign-csp-database-dataflow` | `specs/dataflow/enabled-subscription-inventory/spec.md` | `inventory` (`FR-INV-05`) | 启用订阅资产作用域隔离映射至 FR-INV-05。 |
| 25 | `redesign-csp-database-dataflow` | `specs/dataflow/node-connection-versioning-cow/spec.md` | `inventory` (`FR-INV-06`) | 节点连接参数写时复制与覆写映射至 FR-INV-06。 |
| 25 | `redesign-csp-database-dataflow` | `specs/dataflow/single-sqlite-domain-partition/spec.md` | `inventory` (`FR-INV-01`), 领域架构 | 单 SQLite 库领域分区架构映射；数据库索引细节存根保留。 |
| 25 | `redesign-csp-database-dataflow` | `specs/dataflow/upstream-payload-entry-fact-layer/spec.md` | `inventory` (`FR-INV-03`, `04`) | 上游原始载荷事实层映射至节点身份哈希与来源台账。 |
| 26 | `redo-node-probes-subcheck` | `specs/node-probes/mihomo-subcheck-runtime/spec.md` | `probe` (`FR-PROB-01`) | Mihomo 子检查运行时映射至 FR-PROB-01。 |
| 26 | `redo-node-probes-subcheck` | `specs/node-probes/pipeline-funnel/spec.md` | `probe` (`FR-PROB-03`, `04`) | 探测漏斗管道架构映射至优先队列与健康证据。 |
| 26 | `redo-node-probes-subcheck` | `specs/node-probes/platform-capability-matrix/spec.md` | `probe` (`FR-PROB-01`, `07`) | 平台探测能力矩阵映射至探针与辅助构建器隔离。 |
| 26 | `redo-node-probes-subcheck` | `specs/node-probes/result-contract-and-ui/spec.md` | `probe` (`FR-PROB-04`), `ui` (`FR-UI-02`) | 探测结果契约与前端展示映射至健康判定与 UI。 |

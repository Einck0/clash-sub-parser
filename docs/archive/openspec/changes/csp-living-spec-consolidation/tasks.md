## 1. 规范资产调研与官方参考模板导入 (Phase 1: Research & Reference Templates)

- [x] 1.1 核验官方 SpecKit 锁定版本 (`93989802b1401c99cc7de6e4ed76572f9cb662e8`) 与本地模板原始哈希，在 `.specify/templates/` 建立参考副本及 `provenance.json` 存根，明确声明保留 OpenSpec CLI。
- [x] 1.2 调研 DeepSeek Agent Notes 架构决策模式，统一收敛单事实源至 `docs/decisions/`（清理 `.agents/notes/` 重复镜像），明确记录 proposed 状态及否决备选方案。

## 2. 六大领域主干持续能力规范编制 (Phase 2: Canonical Living Specs Authoring)

- [x] 2.1 编写 `openspec/specs/inventory/spec.md`，规范化受控订阅、CoW 覆写、节点身份哈希及数据修剪边界 (`FR-INV-01` ~ `FR-INV-07`)。
- [x] 2.2 编写 `openspec/specs/policy/spec.md`，确立直接组引用与解包叶节点双核心能力，定义空组 PASS 兜底与动态全池解析 (`FR-POL-01` ~ `FR-POL-07`)。
- [x] 2.3 编写 `openspec/specs/probe/spec.md`，规范原生 Mihomo 执行引擎、多层周期解耦、单节点插队与健康基线四态判定 (`FR-PROB-01` ~ `FR-PROB-07`)。
- [x] 2.4 编写 `openspec/specs/publication/spec.md`，定义 Mihomo 拓扑导出、非 Mihomo 纯节点边界、免密公开导出与双令牌验证 (`FR-PUB-01` ~ `FR-PUB-06`)。
- [x] 2.5 编写 `openspec/specs/auth/spec.md`，规范管理端与导出链接解耦双开关、统一系统主令牌共享与明文敏感配置模型 (`FR-AUTH-01` ~ `FR-AUTH-04`)。
- [x] 2.6 编写 `openspec/specs/ui/spec.md`，定义紧凑自适应视口排版、健康筛选与分页交互、编辑抽屉与双语国际化 (`FR-UI-01` ~ `FR-UI-06`)。
- [x] 2.7 编写 `openspec/specs/README.md` 目录索引，通过 `openspec validate --specs --strict` 验证所有 6 份主干规范零错误通过。

## 3. 证据溯源、实机符合度审计与全量覆盖矩阵 (Phase 3: Source Tier, Audit Matrix & Coverage)

- [x] 3.1 编写 `docs/specification/source-register.md`，建立四级证据确权体系，严格甄别用户原话，纠偏提案问题描述为历史变更分析。
- [x] 3.2 编制 `docs/specification/verification-commands.md`（编号 CV-001 ~ CV-041）与 `docs/specification/conformance-matrix.md`，替换 FR-PROB-07 伪 0-case 命令为真实用例，解耦命令与表格避免转义破坏正则，重新实机执行 41 条命令并生成收据 `/tmp/csp-conformance-real-tests-receipt.json`（178 次执行 / 137 个唯一用例，零 0-case 运行）。
- [x] 3.3 编制 `docs/specification/historical-requirement-register.md`，逐条提取 26 个 Change 63 份历史规范中的 259 项需求（HR-001 ~ HR-259）与 463 个场景，明确显式冲突与未验证存根；同步更新 `upstream-provenance-and-coverage.md`。
- [x] 3.4 编写 `docs/specification/maintenance-guide.md`，确立 37 项主干 FR 矩阵与 259 项 HR 库存的双层治理体系，制定长期演进与更新指南。

## 4. 业务未决问题清单梳理与选项编制 (Phase 4: Ambiguity Consolidation & Options Formulation)

- [x] 4.1 编写 `docs/specification/clarification-questions.md`，删除 Q1.4 越过激活/风险准入的选项，纠偏 Q6 跨页全选为可选候选，规范 Q3、Q2、Q5，精准呈现 6 组核心业务待决选项。

## 5. 独立只读审查准备与门禁复验 (Phase 5: Independent Review Readiness)

- [x] 5.1 运行 OpenSpec 全量严格门禁检查（`openspec validate --specs --strict` 与 `openspec validate --changes --strict`），保持零错误退出码。
- [x] 5.2 提交完整文档工作树供独立只读审查机（Reviewer）进行交叉比对与审计复核（独立Reviewer最终PASS，凭证 `/tmp/csp-living-spec-review-accepted-20261009.json`，SHA-256 `838a270e824f82a093df1987ca807fab19fa19b70aa3a59f90d05d62520e05e9`）。

## 6. 用户呈报与业务方案待决交付 (Phase 6: User Reporting & Awaiting Decision)

- [ ] 6.1 向用户呈报 6 组核心业务待决问题清单，等待用户明确裁决后再启动业务代码落地（本任务待用户决策后开展，本阶段不勾选）。

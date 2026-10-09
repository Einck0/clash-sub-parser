## Context & Architectural Intent

本项目为解决历史变更集众多但缺乏持续主干规范的问题，严格执行用户关于“重写SPC、审计符合度、汇总模糊项待我确认再落地”的最高指示。设计重点在于实现规范治理的双层解耦：将全局持久的 Living Specification（存放在 `openspec/specs/`）与局部的增量变更集（存放在 `openspec/changes/`）有效协同，同时融合业界 Spec-Kit 优先级场景范式与 DeepSeek 决策记录规范。

## Architectural Decisions

### 1. 结构与格式融合设计
- **Spec-Kit 融入 OpenSpec**：Spec-Kit 具有清晰的 `User Scenarios & Testing (Priority: P1/P2/P3)`、`Functional Requirements` 与 `Success Criteria` 结构。我们将其完全吸纳进 OpenSpec 的 `## Requirements`、`### Requirement: ... [FR-ID]` 与 `#### Scenario: ...` 严格语法树中。
- **全局唯一 FR-ID 与 SC-ID**：为 6 大领域分配了 37 个稳定的功能需求编号（`FR-INV-xx`、`FR-POL-xx`、`FR-PROB-xx`、`FR-PUB-xx`、`FR-AUTH-xx`、`FR-UI-xx`），消除历史模糊引用的歧义。

### 2. 证据分级与审计矩阵设计
- 严格贯彻 `pi-governance` 事实来源铁律，建立四级确权：
  - `TIER-1: USER-VERBATIM`（最高）
  - `TIER-2: HISTORICAL-CONTRACT`（高）
  - `TIER-3: IMPLEMENTATION`（中）
  - `TIER-4: EXPERIENCE`（参考）
- 审计矩阵覆盖 37 项稳定需求，清晰定性为 `符合` (26)、`不符合` (3)、`部分符合` (6)、`未验证` (0)、`待确认` (2)，每项均附带文件行号、实机测试命令（附真实通过用例数与收据哈希）与真实物理局限分析。

### 3. 六组核心待决业务问题解耦
针对审计暴露出的重大分歧，坚持“做减法、严禁自行做主”的原则，集中形成 6 组独立决策包：
1. 策略组解包叶节点的去重、递归深度、过滤传播与失效节点处理；
2. 成员来源节点与子组的混合交错顺序策略；
3. 历史 3 个轮询组遗留 `group_nodes` 的数据修复范围；
4. 非 Mihomo 纯节点目标遇到空策略组校验时的行为一致性；
5. 订阅刷新部分成功（Partial）时的来源修剪与隐藏作用域保护；
6. 前端节点健康状态筛选与服务端分页联动的交互模式。

### 4. 前两审 REJECTED 根因定位与 Epoch 3 整改架构 (Remediation Root Cause & Architecture)

经过两轮独立审查机驳回（Reviewer REJECTED，审计收据 `3cb8a9188068e45477b661fde2f31c2f0d311efc763a94f48e2175b00be0a6ab` 与 `391d6ff8146e6a76ca92f6bb118183525d4d5c5aa9cfe8b0183c288d7cd0892e`），本阶段深度定位并闭环解决以下四大根因：

1. **根因一：命令散落 Markdown 表格导致的正则转义失效与 0-case 伪证 (P0-01)**：
   - *问题表现*：表格中直接书写正则管道符导致必须转义为 `\|`，被 Go 正则引擎识别为字面量竖线而匹配 0 个用例；同时 `FR-PROB-07` 的测试命令 `^TestBuildOptions$` 因尾锚点 `$` 导致实际匹配 0 个用例（receipt 中 `cases_run: 0`），而矩阵表格虚标 `16 passed`。
   - *整改方案*：
     - 彻底解耦验证命令与矩阵表格，建立专门的命令目录工件 [`docs/specification/verification-commands.md`](../../../docs/specification/verification-commands.md)，使用独立 Markdown fenced code blocks 编号 `CV-001` ~ `CV-041` 逐条存储与执行，严禁表格内包含管道正则与 HTML 实体；
     - 将 `FR-PROB-07` 替换为真实测试 `TestBuildOptionsRejectsUnsafeProtocolOptions`（实际执行 1 top-level + 6 subtests，全绿通过）；
     - 重新全量实机执行 41 条命令，收集每条命令的标准 argv JSON、退出码（全为 0）、用例名、测试包、子测试、源码 SHA-256 与日志存根；
     - 重新精确统计执行次数（178 次）与独立唯一测试数（137 个，含 91 个 top-level + 46 个 subtests），明确披露跨 FR 复用事实，杜绝人工虚填。

2. **根因二：问题清单 Q1.4 选项越过既有激活与风险准入底线 (P1-01)**：
   - *问题表现*：原 Q1.4 备选项提出“不论健康与激活状态完整展开”，违反了系统既有的 `active=false` 与 `NodeScopeEnabledSubscriptions` 准入底线。
   - *整改方案*：彻底删除该越界选项。明确未激活与风险拦截节点在准入阶段已被坚决剔除，Q1.4 严格聚焦于已准入节点中探针健康状态（`healthy` / `degraded` / `undetermined`）如何受父策略组显式筛选约束，且不默认剔除 `undetermined`。同时确立图环路拒绝为既有架构强约束。

3. **根因三：问题清单 Q6 预设跨页全选为强需求 (P1-02)**：
   - *问题表现*：原 Q6 将跨页全选预设为既有需求，迫使用户在本地分页与全局全选间抉择。
   - *整改方案*：确立服务端全量分页与筛选为既有架构基线（不向用户提问是否保留本地 3 步翻页）；将跨页批量全选明确标记为“如果用户需要才进入落地（待确定），默认单页选择”，未获确认不新增范围。

4. **根因四：粗粒度 63 份规范映射掩盖历史需求细节与冲突**：
   - *问题表现*：先前仅以 26 个 Change 的领域大类宣称覆盖，未逐条核对 63 份规范中具体的 Requirements 与 Scenarios，且掩盖了历史加密/Vault 与主干明文直存等显式冲突。
   - *整改方案*：
     - 建立逐条登记工件 [`docs/specification/historical-requirement-register.md`](../../../docs/specification/historical-requirement-register.md)，从全部 63 份规范中 100% 提取 259 项 Requirement 与 463 个 Scenario，分配稳定 `HR-001` ~ `HR-259` 编号并记录精确文件行号锚点；
     - 严格按真实可证状态标注（按注册表明细表条目统计：232 项已映射待复验、3 项历史冲突待裁定、24 项未验证存根 UNRESOLVED；深入剖析 6 组架构演进冲突主题，包括凭据明文 vs Vault、探针周期 1m vs 10m、解包能力缺失、混合顺序缺陷等）；
     - 确立主干 37 项 FR 矩阵与 259 项 HR 库存的双层治理体系，后续变更持续更新状态，严禁删除历史存根或宣称全部已验。

### 5. 阶段与角色边界防线
- **本阶段严格受限**：仅完成文档工件编写与文档内部一致性自测。
- **审查与交付边界**：保留审查任务为未完成，供独立审查机（Reviewer）进行只读比对，审查全绿后呈报用户裁决。

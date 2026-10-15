# Policy 子系统彻底重构提案 (Rebuild Proposal)

> **状态标示**: `READY` (Sequential Add/Remove Pipeline Confirmed)  
> **治理说明**: 本文档为架构重构提案。当前运行态已绑定真实专案 `pi_run_20261011_190715_2256208` 与变更 `002-policy`。用户已确认『组级条件与regex同级、按顺序加减』流水线（候选 A 顺序重入流水线生效，候选 B 彻底废除；B1/B2 废除；A/B pending 消除）。在独立大包通过自测、审查与黑盒验收前，绝不推进任何业务代码修改或生产部署。

---

## 1. 架构演进背景与核心问题诊断

### 1.1 历史演进与“双重真源”设计缺陷
在系统的历史演进中，策略组成员编排经历了从旧 Python 系统的 `include_entries` 动态列表，到 Go 重构初期的 `group_edges`（静态显式拓扑关系）与 `group_node_filters`（组级过滤规格）分立结构。这种分立导致了严重的架构缺陷：
1. **权威真源分裂（Split Authority）**：
   - 策略组同时拥有 `group_edges`（存储手动节点引用与直接组引用）和 `group_node_filters`（存储动态正则/地区/协议规则）。
   - 在图解析器（`internal/resolver/graph.go`）中，两者关系被隐式处理为“交集筛选”或“分块覆盖”，导致用户在界面上保存节点组时，显式勾选的节点与动态规则相互冲突。
2. **混合交错保序被破坏（Interleaving Violation）**：
   - `internal/resolver/graph.go` 硬编码为：先处理所有 `explicitNodeEdges`，再处理 `childGroupEdges`。用户在 UI 中编排的 `[NodeA, ChildGroupB, NodeC]` 相对顺序被彻底打碎。
3. **叶节点解包完全缺失（Unpack / Flatten Missing）**：
   - 旧系统支持 `group_nodes` 类型（展开子组已解析的最终叶节点），但在 Go 重构后仅保留了直接组引用（`group_ref`），导致需要将子组叶节点合并作为轮询/回退池的场景彻底失效，并产生了多达 3 个历史空轮询组（日本、新加坡、包含香港）。
4. **布尔补丁与打补丁式修补（Patch Fatigue）**：
   - 此前曾探讨通过在边或组上增加 `unpack_leaf_nodes` 等布尔标志进行局部修补。但此举无法解决多源交错、统一去重、内建项处理及事务原子性等根本问题。
   - **用户原话明确要求**：**坚决废除旧模式打补丁，彻底重构策略组编排模型**！以一份权威有序的 `member_sources` 作为唯一真源。

---

## 2. 新架构：统一有序来源模型 (Unified Ordered Member Sources)

### 2.1 唯一权威列表 `member_sources`
策略组成员编排废除 `group_edges` 与 `group_node_filters` 的双源分立，改为单一有序的 `member_sources` 列表。每个成员来源项具备唯一的位置（Position）和明确的类型。

用户可见的 **四大来源类别 (Four User-Visible Kinds)**：
1. **`regex_nodes`**：正则匹配全准入节点库存。按确定的显示名与逻辑 ID 全序匹配，产出 0 到 N 个活动节点候选。
   - *兼容扩展*：支持 `inventory_query` 子类型（包含国家代码、协议、标签等完整 `NodeFilterSpec` 表达能力），确保旧系统的复合筛选能力无损。
2. **`node`**：显式指定的手工节点。绑定稳定的节点 `logical_id`，产出 1 个节点候选。
3. **`group_ref`**：直接策略组引用。将另一个策略组本身插入当前组中（面向支持组嵌套的目标，如 Mihomo）。产出 1 个策略组候选，**绝不伪装为节点**。
4. **`group_nodes`**：解包展开子组。递归求值目标子组并将其最终真实叶节点平铺展开插入当前组。产出 0 到 N 个叶节点候选。

**内建项 (Typed Builtin Targets)**：
- 对于 `PASS`、`DIRECT`、`REJECT`、`REJECT-DROP` 等特殊流控目标，系统以显式类型化的 `builtin` 对象或枚举表示，**严禁伪造虚假 UUID**，亦不得凭名字盲猜规则。

### 2.2 求解语义：并集追加与首次出现去重 (Union & First-Occurrence Dedup)
- **候选生成**：每个来源项按其在 `member_sources` 中的绝对位置顺序独立求值，生成候选序列。所有来源项的求值结果为**顺序并集（Union）**。
- **命名空间隔离 (Identity Namespace)**：
  - `node:<logical_id>`
  - `group:<group_id>`
  - `builtin:<name>`
- **全序首次去重 (First-Occurrence Wins)**：
  - 在遍历展开的候选流中，依据上述全局命名空间执行去重。首个出现的身份保留在当前位置，后续重复出现者静默丢弃，严格保持全序。

### 2.3 组级可选业务约束 (Group-Level Constraints)
- 组级可选约束（`constraints` / `NodeFilterSpec`）与成员来源（`member_sources`）在概念语义上**严格分离**：
  - `member_sources` 负责“将谁拉入候选池”；
  - `constraints` 负责“对拉入的叶节点候选执行额外的健康、延迟、地区或排除过滤”。
- 在管理端交互中，两者统一在成员编排区域呈现，消除认知割裂。

### 2.4 保存事务原子性与并发控制
- **单事务原子替换**：保存策略组时，通过单数据库事务原子写入策略组元数据、全量替换 `member_sources` 并在需要时更新组级约束，杜绝中间不一致状态。
- **乐观版本并发控制 (Optimistic Concurrency Control)**：
  - 请求必须携带 `revision` / `version` 标识；
  - 若并发写入导致版本过期，服务端直接返回 409 `revision_conflict`，要求客户端拉取最新状态后重试，禁止静默覆盖。
- **一致性快照与校验**：
  - 保存后立即重读验证（Save -> Reload Consistency）；
  - 编辑预览（Preview）与最终导出（Export）必须基于相同修订的不可变快照，调用相同的图解析器（Resolver）。
  - *说明*：不同导出目标（Mihomo YAML vs SingBox JSON vs Surge conf）语法与格式不同，因此**不可声称全目标产物哈希一致**；系统固定输出 `selection_digest`（拓扑选择摘要）与各目标独立的 `target_render_content`。

---

## 3. 用户已明确采纳的决策矩阵 (Confirmed Items)

以下六项核心原则已获用户明确确认（**CONFIRMED**），作为本次重构的不可动摇基石：

| 序号 | 决策项 | 状态 | 详细执行规则 |
|---|---|---|---|
| 1 | **子组自决与父过滤二次生效** | **CONFIRMED** | `group_nodes` 展开时，子组先按照自身的来源与自身过滤条件求值产生叶节点；父组配置的过滤条件随后对展开后的所有真实叶节点执行二次过滤。 |
| 2 | **首次出现保留去重** | **CONFIRMED** | 同一节点或同一组引用在同一策略组的成员列表中多次出现时，严格实行 **First-seen wins**，保留首个位置，消除后续冗余。 |
| 3 | **无额外强加健康限制** | **CONFIRMED** | 系统不强加额外的“必须健康（must-healthy）”隐式门禁，严格继承既有的全局准入、活动状态（Active）、范围（Scope）与风险控制（Risk Admission）规则。 |
| 4 | **混合交错严格保序** | **CONFIRMED** | 节点、子组引用、正则展开项与解包叶节点的相对顺序完全按照用户编排全序排列，**严禁**代码中硬编码“节点在前、子组在后”的分类分块逻辑。 |
| 5 | **全引用环与展开环拒绝** | **CONFIRMED** | 对所有策略组拓扑引用进行严格 DAG 检测。无论是 `group_ref` 形成的直接引用环，还是 `group_nodes` 形成的解包展开环，在保存与预览时**均予以明确拒绝**，并输出完整的成环路径（Cycle Path）。 |
| 6 | **深度与资源边界防护** | **CONFIRMED** | 系统设定有界资源预算（如最大递归深度 16 层、最大成员预算 5000），超出时报错阻断；不向用户机械询问“要 3 层还是 5 层”，亦绝不进行静默截断。`group_ref` 始终输出组名，`group_nodes` 递归平铺为真实叶节点。内建项（PASS 等）按目标编译器能力真实保留。 |

### 废弃原则 (SUPERSEDED)
- **旧方案废止**：此前设计的 `unpack_leaf_nodes` 单一边布尔开关、双真源交集计算机制以及隐式派生组方案全部标记为 **SUPERSEDED**。

---

## 4. 存量数据无缝迁移设计 (Zero-Loss Migration Design)

本次迁移不是运行时的兼容垫片（Runtime Legacy Shim），而是数据库持久化层面的一次性无损转换：
1. **纯边策略组 (Edges-only)**：
   - 现有的 `group_edges` 依据其存储的 `position` 准确转换为对应类型的 `node` 或 `group_ref` 项，绝不猜测伪造原始交织顺序。
2. **纯过滤策略组 (Filter-only / Dynamic)**：
   - 现有的 `group_node_filters` 转换为 `regex_nodes`（若为纯正则）或 `inventory_query` 来源项，完整保留其 AI、Geo、协议及标签筛选能力，在用户可见的四大类别下不损表达力。
3. **边与过滤混合策略组 (Edges + Filter)**：
   - 旧系统的“边 + 过滤”交集语义，准确转换为“成员来源（来源边）+ 组级可选约束（原过滤）”，**绝不偷偷将其转为并集导致成员意外扩增**。
4. **历史空组与缺失配置**：
   - 缺失配置的策略组保持为空，并在界面明确提示“缺少来源”；原配置的 `empty_fallback_pass` 正常生效（输出 PASS），不以虚假数据冒充恢复。
5. **不可变标识与防漂移保证**：
   - 现有所有 `group_id`、分流规则引用（`policy_rules.target_group_id`）及已发布订阅引用保持绝对不变，绝不重新生成 UUID 导致客户端订阅失效。
6. **迁移安全防护**：
   - 提供完全可逆的 `dry-run` 报告，输出迁移前后的 Revision、内容哈希对比；
   - 历史旧表归档保留用于审计，不搞长期双写，严禁删除任何归档冷数据库。

---

## 5. 历史归档取证与定点纠偏 (Archival Forensics & Corrections)

针对前期调查中的疑问与推测，本次只读取证（Read-Only Investigation）确认了以下关键实机事实：

### 5.1 历史冷归档数据库确权
- 冷归档数据库路径：`/home/service/backups/csp-legacy-cold-archive-20260919.db`
- 文件确权 SHA-256：`bf768408578663c355baa17af9259955540fda54a73330bd921d8807b9b53630`
- 确权结论：该归档文件完好且哈希完全匹配，数据库中记录为正常的 2026 年业务操作与隔离保留（Quarantine Records），不存在所谓的“2016 年虚假日期”或物理删除。

### 5.2 历史 3 个轮询组与“便宜”组真实结构证明
在冷归档的 `node_groups` 表中，真实数据如下（证实为真实的 JSON Array of Dict，而非简写）：
- **组 11「轮询(日本)」**：`[{"type": "group_nodes", "value": 6}]`
- **组 12「轮询(新加坡)」**：`[{"type": "group_nodes", "value": 7}, {"type": "node", "value": "PASS"}]`
- **组 13「轮询(包含香港)」**：`[{"type": "group_nodes", "value": 4}, {"type": "group_nodes", "value": 3}, {"type": "group_nodes", "value": 5}, {"type": "group_nodes", "value": 6}, {"type": "group_nodes", "value": 7}]`
- **组 2「便宜」**：
  ```json
  [
    {"type": "node", "value": "PASS"},
    {"type": "regex", "value": "去掉(流媒体|x(?:[0-5](?:\\.[0-9]+)?)(?![\\d.])|便宜|free )", "name": "便宜"},
    {"type": "regex", "value": "Eeox", "name": "eeox"},
    {"type": "regex", "value": "einck", "name": "einck"}
  ]
  ```

### 5.3 历史 ID 映射与当前数据库对照表
通过对照当前数据库备份（`/home/service/backups/csp-before-policy-membership-20261008_194145.db`），历史组 ID 到现代 UUID 的映射得到 100% 确定性证实：

| 历史 ID | 组名称 | 现代稳定 UUID (`node_groups.id`) | 历史展开引用 (Refs) | 当前边数量 | 恢复方案说明 |
|---|---|---|---|---|---|
| 1 | 低延迟 | `01a0b9af-c116-783d-903e-6e0c59f066ae` | PASS + 正则 | 正常 | 保持现状 |
| 2 | 便宜 | `01a0b9af-c116-7479-bcf8-338e825502ad` | PASS + 3条正则 (含'去掉'DSL) | 0 边 0 过滤 | **待核验副本恢复** |
| 3 | 美国 | `01a0b9af-c116-7803-89e1-593bb58b23e8` | 正则 | 正常 | 保持现状 |
| 4 | 香港 | `01a0b9af-c116-7c5f-8a2e-c5363cc72b4d` | 正则 | 正常 | 保持现状 |
| 5 | 台湾 | `01a0b9af-c116-7997-bf6b-b96edd2310a3` | 正则 | 正常 | 保持现状 |
| 6 | 日本 | `01a0b9af-c116-7cca-8ee5-47d928361ec1` | 正则 | 正常 | 保持现状 |
| 7 | 新加坡 | `01a0b9af-c116-70d1-a027-dd08a0e3b9b1` | 正则 | 正常 | 保持现状 |
| 11 | 轮询(日本) | `01a0b9af-c116-79e1-9ce2-f6eb09df7b63` | `group_nodes: 6` (日本) | 0 边 | **待核验副本恢复** |
| 12 | 轮询(新加坡) | `01a0b9af-c116-770f-aeb9-68db80511fc7` | `group_nodes: 7` (新加坡) + PASS | 0 边 | **待核验副本恢复** |
| 13 | 轮询(包含香港) | `01a0b9af-c116-7bda-ab27-c6e4685f8de5` | `group_nodes: 4, 3, 5, 6, 7` | 0 边 | **待核验副本恢复** |

> **关键恢复原则**：本轮**绝不实际向数据库恢复任何数据**。所有恢复脚本与映射表仅作为设计方案呈现，待用户确认后再行推进。原顺序必须严格基于归档证据恢复，不可依据现 DB 猜测。

### 5.4 旧 DSL `去掉(...)` 语义定点纠偏
- 在旧 Python 代码（`commit 8b6f8278~1:backend/app/utils/group_utils.py`）中，`re.compile` 直接编译了该字符串。
- 业务实际中，“去掉”属于用户在旧前端/配置中书写的排除意图（Exclusion）。但由于历史代码中未能证明其精确 handler 逻辑，**本草案严禁擅自直接将其生成为现代 RE2/PCRE 正则映射**，亦绝不擅自引入未经批准的现代专有字段或简单以 PASS 兜底掩盖。
- **处理规范**：原式保留在归档记录中，数据迁移与恢复留待后续副本离线人工对账后再行确认转换规则。

---

## 6. 编译器目标实机分支审计 (Compiler Reality Verification)

通过对 `internal/compiler/` 源码的深入审计，核实了各编译目标的真实行为：
1. **Mihomo 目标 (`renderMihomo`)**：
   - 唯一完整渲染节点（`proxies`）、策略组（`proxy-groups`）和分流规则（`rules`）的目标；
   - 严格保留策略组拓扑结构验证，支持组嵌套与 `empty-fallback: PASS`。
2. **纯节点三目标 (`renderSingBox`, `renderSurge`, `renderQuantumultX`)**：
   - `renderSingBox` 仅输出 `endpoints` 与 `outbounds`；
   - `renderSurge` 仅输出 `[Proxy]` 节点行；
   - `renderQuantumultX` 仅输出节点服务器配置行。
   - **现有缺陷与修复**：当前代码在 `validate()` 中，当 `group.UsesEmptyPass()` 为 true 且目标不为 Mihomo 时抛出 `empty_fallback_pass: target does not support explicit empty-group PASS routing` 错误，导致纯节点导出被无关的空组阻断。用户已确认：纯节点目标不受无关空组阻断，节点全准入与全局导出过滤保持不变。

### 6.1 策略组引用与父约束边界 (Group Reference Constraint Invariant)
- 当策略组包含 `group_ref`（直接策略组引用）时，父策略组上配置的额外组级约束**绝不隐式修改或过滤该子策略组本身的成员结果**。
- 若未来需要让父约束作用于被引用的组，必须依赖用户显式确认的投影组设计，**严禁在未授权情况下偷换为隐藏派生组或强行隐式投影实现**。系统严格保持：直接组引用仅插入子组名，不改子组既有求值结果。

---

## 7. 外部凭据与环境现状说明 (Credentials & Environment Note)

- **说明**：GitHub 网页 Actions secret 与服务器受控 `readpackages` 登录凭据属于远端 CI/CD 配置，**并非本 Policy 子系统设计的前置依赖**。
- **实机命令现状**：经检查，服务器环境变量中未安装 `gh` CLI（`which gh` 退出码为 1）。系统绝不伪造已安装状态，亦不发出不必要的外部网络请求。

---

## 8. 最新用户决策与排除求值算法关键未知 (Latest User Decisions & Unknown Divergence)

### 8.1 用户最新确认共识：取代旧 B1/B2 的统一有序操作模型
- **最新确认核心**: 用户已明确指示**『组级条件与 regex 同级、有加有减』**，彻底取代旧有的 B1/B2 争论。
- **架构定案**:
  1. 废除策略组上外挂的全局套壳业务 filter（`constraints`）。新架构下不搞长期双重真源（Dual Authority），避免新策略组出现旧 B1 现象；
  2. 统一使用唯一有序的 `member_sources`，每个条目显式标注操作（`operation: add | exclude`）与类别（`kind: node | group_ref | group_nodes | matching | builtin`）；
  3. `matching` 来源条件（支持 regex、geo 国家、protocol 协议、probe 探测）仅对该项自身候选生效，不隐式全局过滤其他来源；
  4. 存量既有边又有过滤的组通过明确迁移机制保全其交集语义，严禁以粗暴并集偷偷扩增候选；
  5. `group_ref` 与 `group_nodes` 保持类型域隔离：排除 `group_ref` 仅匹配组名引用，不剔除叶节点；排除 `group_nodes` 仅匹配叶节点，不剔除直接组引用；
  6. 子组在解包时先依据自身内部 sources 计算出叶节点，再平铺展开并接受该解包条目的匹配条件过滤；
  7. 全局命名空间保持混合全序与首次出现去重（First-seen wins），复用既有准入判定，无额外强加健康检查。

### 8.2 真正关键未知：排除求值算法双候选分析 (The Critical Divergence)
用户原话“有加有减”明确了按顺序加减的双向操作。用户已确认采纳**候选 A (顺序重入流水线)**，彻底废除候选 B (全局一票否决) 与历史 B1/B2 设问，消除所有 A/B pending 阻塞项：

1. **候选 A：顺序重入流水线 (Sequential Re-entrant Pipeline)**
   - 严格按列表顺序逐项执行。`add` 追加候选，`exclude` 仅从当前已入选列表中剔除匹配项。
   - **特点**：支持顺序重入。若后续再次 `add` 先前被排除的节点，该节点可被成功重新加回。
   - *案例表现*：`[+group_nodes(HK), -regex(.*01.*), +node(HK-01-Backup)]` → HK-01-Backup 最终**入选**。

2. **候选 B：全局一票否决差集 (Global Veto / Set Difference - 对齐旧 Python 系统)**
   - 分别收集全部 `add` 候选与全部 `exclude` 规则。
   - **特点**：全局差集过滤。只要命中任一 `exclude` 规则即被一票否决，后续任何 `add` 无法复活。
   - *案例表现*：`[+group_nodes(HK), -regex(.*01.*), +node(HK-01-Backup)]` → HK-01-Backup 最终**被否决排除**。

### 8.3 治理应对与冻结原则
- 方案数据结构设计统一参数 `exclusion_mode: "sequential_pipeline" | "global_difference_set"`；
- 保持文档处于草案挂起状态，**绝不擅自二选一宣称 READY**，亦**绝不提前编写业务代码**。

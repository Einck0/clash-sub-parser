# Research & Baseline Audit: Policy Capability

> **治理状态**: `READY` (Sequential Add/Remove Pipeline Confirmed)  
> **关联工件**: `specs/002-policy/rebuild-proposal.md`, `specs/002-policy/data-model.md`  
> **运行态绑定**: `run_id`: `pi_run_20261011_190715_2256208`, `change`: `002-policy`

---

## 1. 历史系统与提交基线审计 (Baseline Audit)

### 1.1 历史演进关键锚点
- **`commit 16fe0e7`**: `snapshot(python): freeze pre-go-rewrite baseline with shadcn workbench and backend changes`
  - 冻结了 Python 版本的旧实现与 Shadcn 工作台。
  - 在 `.exp.md` 中记录了 `include_entries` 支持 `regex` 类型、只存正则字符串、保存 payload 仅传 `include_entries` 的关键历史决策。
- **`commit 8b6f8278`**: Python 到 Go 重写前夕的参考实现。
  - 关键文件 `backend/app/utils/group_utils.py`：包含 `resolve_entries` 与 `resolve_group_members` 函数。
  - 支持 `node`（静态节点名）、`group`（直接按 ID 引用策略组）、`group_nodes`（展开子组叶节点）、`regex`（动态正则匹配）与 `exclude_group_nodes`（展开并排除）。
  - 处理逻辑明确证实：`group_nodes` 是递归调用求值后 `selected.extend(...)`；`dedup_names(...)` 实行保持顺序的首次出现去重。

### 1.2 历史冷归档数据库取证
- **归档路径**: `/home/service/backups/csp-legacy-cold-archive-20260919.db`
- **SHA-256 校验和**: `bf768408578663c355baa17af9259955540fda54a73330bd921d8807b9b53630`
- **数据真实性**:
  - `node_groups` 表中 `include_entries` 为标准的 JSON 数组对象（Array of Dict）。
  - 例如组 13「轮询(包含香港)」：`[{"type": "group_nodes", "value": 4}, {"type": "group_nodes", "value": 3}, {"type": "group_nodes", "value": 5}, {"type": "group_nodes", "value": 6}, {"type": "group_nodes", "value": 7}]`，证实了其为结构化的展开引用，而非简写。
  - 组 2「便宜」包含 `去掉(...)` 模式字符串，证实了历史 DSL 的存在。
  - `quarantine_records` 证实被替换节点是作为隔离记录保留，而非物理硬删除，操作时间为 2026 年（无虚假 2016 日期）。

### 1.3 历史排除语义实机源码取证与「去掉」真伪辨析
通过对 `policy-scout.json` 与历史 Python 源码的深入审计，排除了以下技术假说：
1. **「去掉(...)」非特殊 DSL 包装器**：在 `backend/app/utils/group_utils.py` 中，并未实现任何对中文“去掉”的前缀截取、反向解析或 AST 语法糖。历史代码直接将包含“去掉”的完整字符串交由 `re.compile` 编译。由于上游节点名称中均不含“去掉”二字，该项在历史运行中实际上匹配到 0 个节点，属于历史未完成的排除意图表达。
2. **历史排除的单向全局差集（Global Set Difference）机制**：在历史 Python 实现中，排除能力主要由 `exclude_group_nodes` 以及策略组上的 `exclude_nodes` / `exclude_group_ids` 字段承载。在算法流水线上，历史系统先全量收集所有的 `selected` 候选，再全量构建 `excluded` 集合，最后通过列表推导式 `[x for x in selected if x not in excluded]` 执行单向过滤。这证实了历史系统的排除具有“全局一票否决”特性，而非在特定序列点上的瞬态剔除。
3. **排除粒度仅限叶节点**：历史实现仅支持排除叶节点（`exclude_group_nodes`），从未支持过排除组引用（`exclude_group_ref`）。

---

## 2. 现代 Go 代码库当前实现缺陷分析 (Current Codebase Analysis)

### 2.1 拓扑存储：`group_edges` 与 `group_node_filters`
- `internal/domain/policy.go`:
  - `NodeGroup`: 仅包含 `EmptyFallbackPass`, `ID`, `Name`, `GroupType`, `NodeFilter`。
  - `GroupEdge`: 包含 `ParentGroupID`, `ChildGroupID`, `NodeLogicalID`, `Position`。
- `internal/repository/sqlite/policy.go`:
  - `SaveGroup`: 接受 `edges *[]GroupEdge` 与 `filter *NodeFilterSpec`，两表独立存储。
- **缺陷**:
  1. 无法存储 `group_nodes`（子组叶节点展开）语义。
  2. 无法在同一有序列表中混排正则规则项。
  3. `NodeFilter` 独立于边，造成语义混淆（是用来筛选边，还是独立产生动态节点？）。
  4. 缺少定向操作（`add` / `exclude`）的表达能力，无法支持用户在策略组中有加有减的编排需求。

### 2.2 解析引擎：`internal/resolver/graph.go`
- 审查 `resolveGroups`：
  ```go
  var explicitNodeEdges []domain.GroupEdge
  var childGroupEdges []domain.GroupEdge
  for _, e := range sortedEdges {
      if e.NodeLogicalID != nil && *e.NodeLogicalID != "" {
          explicitNodeEdges = append(explicitNodeEdges, e)
      } else if e.ChildGroupID != nil && *e.ChildGroupID != "" {
          childGroupEdges = append(childGroupEdges, e)
      }
  }
  ```
  先处理所有 `explicitNodeEdges`，再处理 `childGroupEdges`。这彻底破坏了用户的自定义交错顺序。
- 缺少递归展开子组为真实叶节点（Flatten / Unpack）的逻辑，且未处理解包场景下的 DAG 成环检测。

### 2.3 编译器适配：`internal/compiler/`
- 经代码审查确认：
  - `TargetMihomo`: 完整渲染 `proxies`, `proxy-groups`, `rules`。
  - `TargetSingBox`: 仅渲染 `endpoints` 和 `outbounds`（来自 `snapshot.Nodes`）。
  - `TargetSurge`: 仅渲染 `[Proxy]` 行。
  - `TargetQuantumultX`: 仅渲染节点服务器行。
- 在 `compiler.go` 中，`validate` 函数对非 Mihomo 目标检查了 `empty_fallback_pass`：
  ```go
  if target != domain.TargetMihomo {
      for i, group := range snapshot.Groups {
          if group.UsesEmptyPass() {
              return &CapabilityError{...}
          }
      }
      return nil
  }
  ```
  这导致纯节点导出目标因无关空策略组的存在而被错误拦截。新架构下纯节点导出应仅校验活跃节点池，不被策略组状态阻断。

---

## 3. 最新用户确认核心规范与排除未知分歧 (User Decisions & Unknown Divergence)

### 3.1 最新已确认核心共识 (Confirmed User Decisions)
1. **组级条件与 regex 同级，有加有减**：彻底废除旧的 B1/B2 讨论！在策略组中不再额外设置全局套壳的业务 filter（`constraints`）。所有的条件匹配项（`matching` / `condition`，可包含 regex、国家地区 geo、协议 protocol、探测结果 probe 等）与手工节点、组引用同级存在于统一有序的 `sources` 列表中，且每个来源操作项显式声明操作类型：增加（`add`）或排除（`exclude`）。
2. **来源条件仅影响自身候选**：每个 `matching` 来源操作项仅对其自身的候选生成或过滤生效，不作为全局外层套壳去隐式二次过滤其他非同级项。
3. **存量数据无损迁移语义保全**：对于旧数据库中既有边又有过滤的策略组，迁移时必须通过显式迁移逻辑精准还原其交集语义，严禁粗暴做并集导致候选节点偷偷扩增。
4. **子组解包展开与同级过滤**：当来源项为 `group_nodes` 时，目标子组首先根据其自身内部的来源项与操作完整求值产出最终叶节点；若该解包项附带条件，则该条件仅作用于这批解包产出的叶节点。
5. **保持混排顺序与首次出现去重**：全局命名空间（`node:<id>`, `group:<id>`, `builtin:<name>`）严格按编排顺序求值并执行 First-seen wins 去重。
6. **准入标准无额外门禁**：代理节点严格复用既有的 Scope、IP Risk 与 Active 状态判定，不追加任何未经授权的“必须健康”硬门禁。

### 3.2 真正关键未知：排除求值算法双候选分析 (The Critical Algorithmic Divergence)
用户明确指示了“有加有减”，但从当前原话中无法断定多步增删存在冲突时的精确求值算法。当前存在两大严谨的算法候选：

### 3.2 排除求值算法用户最终裁决: 候选 A (顺序重入流水线) 确认采纳

用户已做出明确指示：**『按顺序加减』**。系统正式采纳**算法候选 A (顺序重入流水线)**，彻底废除候选 B (全局差集) 以及历史 B1/B2 设问，消除所有 A/B pending 阻塞项。

#### 采纳方案：顺序重入式流水线 (Sequential Re-entrant Pipeline)
- **计算逻辑**：严格按照 `sources` 的 `position` 升序全序遍历执行。
  - `add` 操作：将新候选条目追加至当前临时结果集中（保持首次去重）；
  - `remove` 操作：仅从**当前已经进入临时结果集**的条目中剔除匹配项；
  - **顺序重入特性**：若在后续位置再次出现 `add` 操作，该操作**可以重新将之前被删除的条目加回新位置**；
  - **无墓碑**：First-seen 去重仅针对当前结果集，删除条目后不留下永久“墓碑”阻止重加。
- **业务案例表现**：编排序列为 `[+group_nodes(All HK), -regex(.*HK 01.*), +manual_node(HK 01-Backup)]`。
  - 第一步加入全量香港节点（包含 HK 01-Backup）；
  - 第二步正则排除所有匹配 HK 01 的节点（HK 01-Backup 被移出）；
  - 第三步显式添加手动节点 HK 01-Backup；
  - **最终结果**：HK 01-Backup **成功重新入选**。

#### 核心已确认共识六大铁律 (Confirmed Invariants - 不再重问)
1. **交错顺序保持**: 来源项混合排序（mixed order）在最终结果中保留用户编排相对顺序；
2. **子组优先与条件局部化**: 子组展开时先按子组自身来源计算，再由父项条件过滤展开叶节点（child filter before parent item filter）；
3. **当前集去重与无墓碑**: First-seen dedup 针对当前集合，删除后不留墓碑允许后续重加；
4. **无额外健康门禁**: 严格遵循系统准入与风险策略，无静默 drop，不强加必须 healthy 限制；
5. **纯节点目标解耦**: SingBox/Surge/QX 导出纯节点，空策略组不产生阻断；
6. **故障隔离与资产保护**: 订阅部分失败时保留源关系与历史节点资产不删除；跨页选择与过滤各自独立操作。

### 3.3 排除类型域严格隔离定义 (Type Domain Invariant)
为防止类型混淆破坏拓扑完整性，系统做出以下严谨的定义说明：
1. **删除 `group_ref` 的域隔离**：当执行 `- group_ref(G_sub)` 时，仅匹配并剔除最终成员列表中的策略组引用标识（组名），**绝不得移除任何由其他来源展开的叶节点**；
2. **删除 `group_nodes` 的域隔离**：当执行 `- group_nodes(G_sub)` 时，递归计算 G_sub 的叶节点并从成员中剔除这些叶节点，**绝不得移除列表中同名的 `group_ref` 直接组引用**；
3. **正则仅匹配节点名称**：`regex` / `filter` 来源中的 `pattern` 正则仅作用于候选节点的 `name`，**严禁使用正则误匹配策略组 ID 或组名称**。

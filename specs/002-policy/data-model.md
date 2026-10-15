# Data Model Specification: Policy Capability (Unified Member Source Operations)

> **治理状态**: `READY` (Sequential Add/Remove Pipeline Confirmed)  
> **关联工件**: `specs/002-policy/research.md`, `specs/002-policy/contracts.md`  
> **运行态绑定**: `run_id`: `pi_run_20261011_190715_2256208`, `change`: `002-policy`

---

## 1. 领域模型结构定义 (Domain Entities)

### 1.1 成员来源操作与类别定义 (`MemberSource`)

```go
package domain

import "time"

// SourceOperation 定义对候选成员的增减操作。
type SourceOperation string

const (
	OperationAdd    SourceOperation = "add"    // 候选增加 (+)
	OperationRemove SourceOperation = "remove" // 候选删除/扣减 (-)
	// 兼容别名
	OperationExclude SourceOperation = "exclude" // 兼容旧草案 exclude，规范等价于 remove
)

// MemberSourceKind 定义成员来源的核心类别。
type MemberSourceKind string

const (
	SourceKindManualNode  MemberSourceKind = "manual_node"   // 显式手工指定的单个节点 (逻辑 ID，别名 node)
	SourceKindGroupRef    MemberSourceKind = "group_ref"     // 直接引用策略组本身 (输出组名)
	SourceKindGroupNodes  MemberSourceKind = "group_nodes"   // 解包展开子组 (递归求值输出真实叶节点)
	SourceKindRegex       MemberSourceKind = "regex"         // 正则匹配来源 (仅匹配候选节点名称)
	SourceKindFilter      MemberSourceKind = "filter"        // 复合属性过滤 (Geo, Protocol, Probe 等同级条件)
	SourceKindMatching    MemberSourceKind = "matching"      // 综合匹配兼容别名
	SourceKindTypedBuiltin MemberSourceKind = "typed_builtin" // 类型化内建流控项 (PASS, DIRECT, REJECT)
)

// MemberSource 代表策略组中一个权威有序的成员操作项。
type MemberSource struct {
	ID        string           `json:"id"`        // 来源项唯一标识 (UUIDv7)
	GroupID   string           `json:"group_id"`  // 所属策略组 ID
	Operation SourceOperation  `json:"operation"` // "add" 或 "remove"
	Kind      MemberSourceKind `json:"kind"`      // "manual_node", "group_ref", "group_nodes", "regex", "filter", "typed_builtin"
	Position  int              `json:"position"`  // 排序位置 (0-indexed 全序)

	// 当 Kind == manual_node / node 时有效：
	NodeLogicalID *string `json:"node_logical_id,omitempty"` // 节点稳定逻辑 ID

	// 当 Kind == group_ref 或 group_nodes 时有效：
	TargetGroupID *string `json:"target_group_id,omitempty"` // 目标策略组 ID

	// 当 Kind == regex / filter / matching 时有效（亦可作为 group_nodes 的附加匹配条件）：
	Pattern    *string         `json:"pattern,omitempty"`     // 候选节点名称匹配正则 (严禁误匹配组 ID)
	FilterSpec *NodeFilterSpec `json:"filter_spec,omitempty"` // 复合匹配条件 (Geo, Protocol, Probe, Tags 等，同级非全组隐式交集)

	// 当 Kind == typed_builtin / builtin 时有效：
	BuiltinName *string `json:"builtin_name,omitempty"` // "PASS", "DIRECT", "REJECT", "REJECT-DROP"

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PolicyGroup 策略组核心实体（已废除 group.Constraints 全局套壳 filter，统一由 sources 表达增减与条件）。
type PolicyGroup struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	GroupType         GroupType      `json:"group_type"`          // select, urltest, fallback, loadbalance
	EmptyFallbackPass bool           `json:"empty_fallback_pass"` // 空组 PASS 兜底标记
	Sources           []MemberSource `json:"sources"`             // 唯一权威的有序来源操作列表
	Revision          int64          `json:"revision"`            // 乐观并发控制版本号
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}
```

### 1.2 解析快照模型 (`ResolvedGroupMember` & `ResolvedGroup`)

```go
package resolver

import "clash-sub-parser/internal/domain"

// MemberKind 标识解析出的最终成员类别。
type MemberKind string

const (
	MemberKindNode    MemberKind = "node"    // 真实代理节点
	MemberKindGroup   MemberKind = "group"   // 嵌套策略组名
	MemberKindBuiltin MemberKind = "builtin" // 内建流控项 (PASS/DIRECT/REJECT)
)

// ResolvedGroupMember 标识最终编排输出的一项成员。
type ResolvedGroupMember struct {
	Kind        MemberKind `json:"kind"`
	TargetID    string     `json:"target_id"`    // 节点 logical_id、策略组 ID 或 builtin 名称
	DisplayName string     `json:"display_name"` // 最终渲染在配置中的名称
	Position    int        `json:"position"`     // 稳定相对位置
}

// ResolvedGroup 包含完整求值结果的策略组快照。
type ResolvedGroup struct {
	ID                 string                `json:"id"`
	Name               string                `json:"name"`
	GroupType          domain.GroupType      `json:"group_type"`
	EmptyFallbackPass  bool                  `json:"empty_fallback_pass"`
	EffectiveEmptyPass bool                  `json:"effective_empty_pass"`
	Members            []ResolvedGroupMember `json:"members"`
	ChildGroupIDs      []string              `json:"child_group_ids"`   // 直接引用的策略组 ID
	ExpandedNodeIDs    []string              `json:"expanded_node_ids"` // 该组所有叶节点 logical_id
	Position           int                   `json:"position"`
}
```

---

## 2. 数据库持久化 Schema (SQLite DDL)

### 2.1 结构化 `policy_group_sources` 表设计

```sql
CREATE TABLE IF NOT EXISTS policy_group_sources (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES node_groups(id) ON DELETE CASCADE,
    operation TEXT NOT NULL DEFAULT 'add',
    kind TEXT NOT NULL,
    position INTEGER NOT NULL,
    node_logical_id TEXT,
    target_group_id TEXT REFERENCES node_groups(id) ON DELETE RESTRICT,
    pattern TEXT,
    filter_spec_json TEXT,
    builtin_name TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_source_operation CHECK (operation IN ('add', 'remove', 'exclude')),
    CONSTRAINT chk_source_kind CHECK (kind IN ('manual_node', 'node', 'group_ref', 'group_nodes', 'regex', 'filter', 'matching', 'typed_builtin', 'builtin')),
    CONSTRAINT chk_builtin_name CHECK (builtin_name IS NULL OR builtin_name IN ('PASS', 'DIRECT', 'REJECT', 'REJECT-DROP'))
);

CREATE INDEX IF NOT EXISTS idx_group_sources_group_pos ON policy_group_sources(group_id, position);
CREATE INDEX IF NOT EXISTS idx_group_sources_target_group ON policy_group_sources(target_group_id);
CREATE INDEX IF NOT EXISTS idx_group_sources_node_logical_id ON policy_group_sources(node_logical_id);
```

### 2.2 `node_groups` 表兼容性微调
为支持乐观并发控制与平滑迁移审计，在 `node_groups` 增加 `revision` 字段：
```sql
ALTER TABLE node_groups ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
```
> **注意**: 历史 `group_edges` 与 `group_node_filters` 表在迁移期保持只读保留，不执行物理删除，确保历史对账可追溯。

---

## 3. 实体关系与生命周期 (Entity Lifecycle & Invariants)

### 3.1 实体关系图 (ERD)

```
┌────────────────────────┐
│      node_groups       │
├────────────────────────┤
│ id (PK)                │
│ name                   │
│ group_type             │
│ empty_fallback_pass    │
│ revision               │
└───────────┬────────────┘
            │ 1
            │
            │ N (Cascade Delete, Ordered by position)
            ▼
┌────────────────────────┐
│  policy_group_sources  │
├────────────────────────┤
│ id (PK)                │
│ group_id (FK)          │
│ operation ('add'|'ex') │
│ kind                   │
│ position               │
│ node_logical_id (FK)   │──────► [nodes (logical_id)]
│ target_group_id (FK)   │──────► [node_groups (id)]
│ pattern                │
│ filter_spec_json       │
│ builtin_name           │
└────────────────────────┘
```

### 3.2 领域不变式 (Domain Invariants)
1. **单一事实源不变式**: 策略组的所有成员产生、删除与排序完全由 `policy_group_sources` 唯一决定，废除任何分离的外部 filter 字段与双重真源。存量 filter-only 组迁移为 `inventory_query_source`。
2. **位置连续与全序执行不变式**: 同一 `group_id` 内的 `position` 从 0 开始严格递增，解析时按此全序进行顺序重入式执行。
3. **删除与去重无墓碑不变式**:
   - `remove` 操作仅删除当前临时结果集中命中的对应 identity；
   - 后续 `add` 项可重新将先前删除的 identity 追加至新位置；
   - First-seen wins 去重仅针对当前结果集，删除后不保留永久“墓碑”阻碍后续重新加入。
4. **类型域严格隔离不变式**:
   - `kind IN ('manual_node', 'node')` 时，`node_logical_id` 必须非空；
   - `kind IN ('group_ref', 'group_nodes')` 时，`target_group_id` 必须非空；
   - `kind IN ('regex', 'filter', 'matching')` 时，`pattern` 或 `filter_spec_json` 至少一项非空；
   - `kind IN ('typed_builtin', 'builtin')` 时，`builtin_name` 必须非空且属于受限枚举；
   - `operation IN ('remove', 'exclude')` 且 `kind == 'group_ref'` 时，仅剔除当前组引用 identity，严禁误删解包叶节点；
   - `operation IN ('remove', 'exclude')` 且 `kind == 'group_nodes'` 时，仅剔除当前展开叶节点，严禁误删直接组引用；
   - `pattern` 正则与 filter 条件仅作用于候选节点名称与元数据，绝不误匹配策略组 ID 或组名。
5. **子组优先求值与条件局部化不变式**:
   - 当来源项为 `group_nodes` 时，目标子组首先根据其自身内部来源自决求值产生真实叶节点；子组内部的正则来源与手工节点同级合并，不能全组筛 node；
   - 父组该解包项所附带的同级条件仅用于过滤这批解包产出的叶节点。
6. **循环依赖明报零容忍不变式**: 无论通过 `group_ref` 还是 `group_nodes`，全图不允许存在有向环；一旦成环必须显式报错 `cycle_detected` 并输出路径，严禁静默丢弃。
7. **系统准入与零额外健康限制不变式**:
   - 节点准入继承既有的 Active、Scope、IP Risk 等系统安全规则，不被增减编排绕行；
   - 严禁擅自追加“必须健康”或静默 drop 门禁。

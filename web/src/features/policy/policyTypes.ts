export type GroupType = 'select' | 'urltest' | 'fallback' | 'loadbalance'

export type RuleAction = 'allow' | 'reject' | 'quarantine'

export interface GroupEdge {
  id?: string
  parent_group_id?: string
  child_group_id?: string
  node_logical_id?: string
  position: number
}

export interface PolicyGroup {
  id: string
  name: string
  group_type: GroupType
  edges: GroupEdge[]
  node_filter?: NodeFilterSpec | null
  created_at?: string
  updated_at?: string
}

export interface AdmissionRule {
  id: string
  revision_id: string
  name: string
  expression: string
  action: RuleAction
  position: number
}

export interface PolicyRule {
  id: string
  revision_id: string
  target_group_id: string
  expression: string
  position: number
}

export interface ValidationResult {
  valid: boolean
  errors?: string[]
}

export const ALL_GROUP_TYPES: Array<{ type: GroupType; label: string; desc: string }> = [
  { type: 'select', label: 'Select', desc: 'Manual user selection of node or child group' },
  { type: 'urltest', label: 'URL Test', desc: 'Automatic lowest-latency benchmarking group' },
  { type: 'fallback', label: 'Fallback', desc: 'Automatic failover to next available node in order' },
  { type: 'loadbalance', label: 'Load Balance', desc: 'Round-robin or hash-based distribution' },
]

export function groupTypeLabel(type: GroupType): string {
  switch (type) {
    case 'select':
      return 'Select'
    case 'urltest':
      return 'URL Test'
    case 'fallback':
      return 'Fallback'
    case 'loadbalance':
      return 'Load Balance'
    default:
      return type
  }
}

export function groupTypeTone(type: GroupType): 'primary' | 'secondary' | 'accent' | 'info' {
  switch (type) {
    case 'select':
      return 'primary'
    case 'urltest':
      return 'secondary'
    case 'fallback':
      return 'accent'
    case 'loadbalance':
      return 'info'
    default:
      return 'primary'
  }
}

export function ruleActionTone(action: RuleAction): 'success' | 'error' | 'warning' {
  switch (action) {
    case 'allow':
      return 'success'
    case 'reject':
      return 'error'
    case 'quarantine':
      return 'warning'
    default:
      return 'warning'
  }
}

export function validateEdgeInput(edge: Partial<GroupEdge>, parentId: string): string | null {
  const hasChild = Boolean(edge.child_group_id && edge.child_group_id.trim())
  const hasNode = Boolean(edge.node_logical_id && edge.node_logical_id.trim())

  if (!hasChild && !hasNode) {
    return 'Must specify either child group or node logical ID'
  }

  if (hasChild && hasNode) {
    return 'Cannot specify both child group and node logical ID'
  }

  if (hasChild && edge.child_group_id === parentId) {
    return 'Self-loop forbidden: parent group cannot reference itself'
  }

  if (edge.position !== undefined && edge.position < 0) {
    return 'Edge position must be non-negative'
  }

  return null
}

export type FilterField =
  | 'display_name'
  | 'protocol'
  | 'source_subscription_ids'
  | 'probe_verdict'
  | 'probe_latency_ms'

export type FilterOp =
  | 'contains'
  | 'not_contains'
  | 'equals'
  | 'not_equals'
  | 'lte'

export interface FilterCondition {
  field: FilterField
  op: FilterOp
  value?: string
  probe_kind?: 'baseline' | 'geo' | 'streaming' | 'ai' | 'speed' | 'ip_risk'
  freshness_seconds?: number
}

export interface NodeFilterSpec {
  conditions: FilterCondition[]
}

export interface GlobalNodeFilter {
  spec: NodeFilterSpec
  updated_at: string
}

export const SUPPORTED_FILTER_FIELDS: Array<{ field: FilterField; label: string }> = [
  { field: 'display_name', label: 'Display Name' },
  { field: 'protocol', label: 'Protocol' },
  { field: 'source_subscription_ids', label: 'Source Subscription' },
  { field: 'probe_verdict', label: 'Probe Verdict' },
  { field: 'probe_latency_ms', label: 'Probe Latency (ms)' },
]

export function validateConditionInput(cond: Partial<FilterCondition>): string | null {
  if (!cond.field) return 'Field is required'
  if (!cond.op) return 'Operator is required'

  switch (cond.field) {
    case 'display_name':
      if (cond.op !== 'contains' && cond.op !== 'not_contains') {
        return 'Display name only supports "contains" or "not_contains"'
      }
      if (!cond.value || !cond.value.trim()) {
        return 'Display name value cannot be empty'
      }
      if (cond.value.trim().length > 255) {
        return 'Display name value cannot exceed 255 characters'
      }
      break

    case 'protocol':
      if (cond.op !== 'equals' && cond.op !== 'not_equals') {
        return 'Protocol only supports "equals" or "not_equals"'
      }
      if (!cond.value || !cond.value.trim()) {
        return 'Protocol value cannot be empty'
      }
      break

    case 'source_subscription_ids':
      if (cond.op !== 'contains' && cond.op !== 'not_contains') {
        return 'Source subscription only supports "contains" or "not_contains"'
      }
      if (!cond.value || !cond.value.trim()) {
        return 'Subscription ID cannot be empty'
      }
      break

    case 'probe_verdict':
      if (cond.op !== 'equals' && cond.op !== 'not_equals') {
        return 'Probe verdict only supports "equals" or "not_equals"'
      }
      if (!cond.probe_kind) {
        return 'Probe kind is required for probe verdict condition'
      }
      if (!cond.value || !cond.value.trim()) {
        return 'Verdict value is required'
      }
      if (cond.freshness_seconds !== undefined && (cond.freshness_seconds < 1 || cond.freshness_seconds > 604800)) {
        return 'Freshness must be between 1s and 604800s (7 days)'
      }
      break

    case 'probe_latency_ms':
      if (cond.op !== 'lte') {
        return 'Probe latency only supports "<=" (lte)'
      }
      if (!cond.probe_kind) {
        return 'Probe kind is required for probe latency condition'
      }
      if (!cond.value || !cond.value.trim()) {
        return 'Latency threshold (ms) is required'
      }
      const lat = Number(cond.value)
      if (isNaN(lat) || lat < 0 || lat > 60000) {
        return 'Latency threshold must be a number between 0 and 60000 ms'
      }
      if (cond.freshness_seconds !== undefined && (cond.freshness_seconds < 1 || cond.freshness_seconds > 604800)) {
        return 'Freshness must be between 1s and 604800s (7 days)'
      }
      break

    default:
      return `Unsupported field: ${cond.field}`
  }

  return null
}


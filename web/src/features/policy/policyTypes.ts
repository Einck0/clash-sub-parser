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

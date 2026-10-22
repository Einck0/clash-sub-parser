export interface RuleCategory {
  id: string
  name: string
  sort_order: number
  created_at?: string
  updated_at?: string
}

export type BuiltinTargetAction = 'DIRECT' | 'REJECT' | 'PASS'

export interface PolicyRuleView {
  id: string
  revision_id: string
  category_id?: string
  category?: string
  name?: string
  target_group_id?: string
  builtin_target?: string
  expression: string
  position: number
  enabled: boolean
}

export interface ListRulesResponse {
  revision_id: string
  policy_rules: PolicyRuleView[]
  admission_rules?: unknown[]
  page?: number
  page_size?: number
  total: number
}

export interface CreateRulePayload {
  revision_id?: string
  category_id?: string
  category?: string
  name?: string
  target_group_id?: string
  builtin_target?: string
  expression: string
  position?: number
  enabled?: boolean
}

export interface UpdateRulePayload {
  category_id?: string
  category?: string
  name?: string
  target_group_id?: string
  builtin_target?: string
  expression?: string
  position?: number
  enabled?: boolean
}

export interface CreateCategoryPayload {
  name: string
  sort_order: number
}

export interface UpdateCategoryPayload {
  name: string
  sort_order: number
}

export interface RuleValidationIssue {
  code: string
  severity: 'error' | 'warning'
  rule_id?: string
  position?: number
  type?: string
  value?: string
  target_group_id?: string
  target_group_name?: string
  message: string
}

export interface RuleValidationResult {
  valid: boolean
  errors?: string[]
  revision_id?: string
  issues?: RuleValidationIssue[]
}

export const COMMON_RULE_TYPES = [
  'DOMAIN-SUFFIX',
  'DOMAIN-KEYWORD',
  'DOMAIN',
  'IP-CIDR',
  'IP-CIDR6',
  'GEOSITE',
  'GEOIP',
  'PROCESS-NAME',
  'RULE-SET',
  'MATCH',
] as const

export type CommonRuleType = (typeof COMMON_RULE_TYPES)[number]

export interface ParsedRuleExpression {
  type: string
  value: string
  noResolve: boolean
  isTerminal: boolean
}

export function parseRuleExpression(expr: string): ParsedRuleExpression {
  const parts = (expr || '').split(',').map((p) => p.trim())
  const type = (parts[0] || 'MATCH').toUpperCase()
  const isTerminal = type === 'MATCH' || type === 'FINAL'
  const value = parts.slice(1).filter((p) => p.toLowerCase() !== 'no-resolve').join(',')
  const noResolve = parts.some((p) => p.toLowerCase() === 'no-resolve')
  return { type, value, noResolve, isTerminal }
}

export function ruleTypeBadgeClass(type: string): string {
  const upper = (type || '').toUpperCase()
  if (upper.startsWith('DOMAIN')) {
    return 'badge-primary text-primary-content'
  }
  if (upper.startsWith('IP-CIDR')) {
    return 'badge-secondary text-secondary-content'
  }
  if (upper.startsWith('GEO')) {
    return 'badge-accent text-accent-content'
  }
  if (upper === 'MATCH' || upper === 'FINAL') {
    return 'badge-neutral text-neutral-content'
  }
  if (upper.startsWith('PROCESS')) {
    return 'badge-info text-info-content'
  }
  return 'badge-ghost'
}

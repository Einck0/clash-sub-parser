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

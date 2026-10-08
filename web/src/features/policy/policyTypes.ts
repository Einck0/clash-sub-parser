import type { CompilerTarget } from '../publications/publicationTypes'

export type GroupType = 'select' | 'urltest' | 'fallback' | 'loadbalance'

export type RuleAction = 'allow' | 'reject' | 'quarantine'

export type SupportedProtocol =
  | 'ss'
  | 'vmess'
  | 'vless'
  | 'trojan'
  | 'hysteria2'
  | 'wireguard'
  | 'tuic'

export const SUPPORTED_PROTOCOLS: readonly SupportedProtocol[] = [
  'ss',
  'vmess',
  'vless',
  'trojan',
  'hysteria2',
  'wireguard',
  'tuic',
] as const

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
  empty_fallback_pass?: boolean
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

export interface ValidationIssue {
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

export interface ValidationResult {
  valid: boolean
  errors?: string[]
  revision_id?: string
  issues?: ValidationIssue[]
}

export interface GroupTypeCapabilityInfo {
  type: GroupType
  label: string
  desc: string
  supportedTargets: readonly CompilerTarget[]
  capabilityNote: string
}

export const ALL_GROUP_TYPES: GroupTypeCapabilityInfo[] = [
  {
    type: 'select',
    label: '手动选择 (select)',
    desc: '用户手动选择使用的节点或子策略组',
    supportedTargets: ['mihomo'],
    capabilityNote: '仅 Mihomo 支持策略组（其他目标仅导出节点格式，忽略策略组与规则）',
  },
  {
    type: 'urltest',
    label: '自动测速 (url-test)',
    desc: '自动选择延迟最低的节点',
    supportedTargets: ['mihomo'],
    capabilityNote: '仅 Mihomo 支持策略组（其他目标仅导出节点格式，忽略策略组与规则）',
  },
  {
    type: 'fallback',
    label: '故障转移 (fallback)',
    desc: '按优先顺序自动切换到下一个可用节点',
    supportedTargets: ['mihomo'],
    capabilityNote: '仅 Mihomo 支持策略组（其他目标仅导出节点格式，忽略策略组与规则）',
  },
  {
    type: 'loadbalance',
    label: '负载均衡 (load-balance)',
    desc: '在多个节点间轮询或哈希分配流量',
    supportedTargets: ['mihomo'],
    capabilityNote: '仅 Mihomo 支持策略组（其他目标仅导出节点格式，忽略策略组与规则）',
  },
]

export interface RuleCapabilityBand {
  category: string
  ruleKinds: readonly string[]
  supportedTargets: readonly CompilerTarget[]
  note: string
}

export const MODERN_RULE_CAPABILITY_MATRIX: RuleCapabilityBand[] = [
  {
    category: '通用路由子集（7 类规则）',
    ruleKinds: ['DOMAIN', 'DOMAIN-SUFFIX', 'DOMAIN-KEYWORD', 'IP-CIDR', 'IP-CIDR6', 'GEOIP', 'MATCH'],
    supportedTargets: ['mihomo'],
    note: '仅 Mihomo 支持分流规则（sing-box / Surge / Quantumult X 仅导出节点格式，忽略规则）',
  },
  {
    category: '规则集与进程子集（2 类规则）',
    ruleKinds: ['RULE-SET', 'PROCESS-NAME'],
    supportedTargets: ['mihomo'],
    note: '仅 Mihomo 支持分流规则',
  },
  {
    category: '现代 Geosite 与端口/源 IP 子集（5 类规则）',
    ruleKinds: ['GEOSITE', 'SRC-IP-CIDR', 'SRC-PORT', 'DST-PORT', 'PORT'],
    supportedTargets: ['mihomo'],
    note: '仅 Mihomo 支持分流规则',
  },
]

export function groupTypeSupportedTargets(type: GroupType): readonly CompilerTarget[] {
  const found = ALL_GROUP_TYPES.find((g) => g.type === type)
  return found ? found.supportedTargets : ['mihomo']
}

export function groupTypeLabel(type: GroupType): string {
  switch (type) {
    case 'select':
      return '手动选择 (select)'
    case 'urltest':
      return '自动测速 (url-test)'
    case 'fallback':
      return '故障转移 (fallback)'
    case 'loadbalance':
      return '负载均衡 (load-balance)'
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

export function ruleActionLabel(action: RuleAction): string {
  switch (action) {
    case 'allow':
      return '允许'
    case 'reject':
      return '拒绝'
    case 'quarantine':
      return '隔离'
    default:
      return action
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
    return '必须指定子策略组或节点逻辑 ID'
  }

  if (hasChild && hasNode) {
    return '不能同时指定子策略组和节点逻辑 ID'
  }

  if (hasChild && edge.child_group_id === parentId) {
    return '禁止自环：父策略组不能引用自身'
  }

  if (edge.position !== undefined && edge.position < 0) {
    return '边位置序号必须为非负数'
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
  | 'regex'
  | 'not_regex'
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
  { field: 'display_name', label: '显示名称' },
  { field: 'protocol', label: '协议类型' },
  { field: 'source_subscription_ids', label: '来源订阅' },
  { field: 'probe_verdict', label: '探针判定' },
  { field: 'probe_latency_ms', label: '探针延迟 (ms)' },
]

export function validateConditionInput(cond: Partial<FilterCondition>): string | null {
  if (!cond.field) return '必须选择字段'
  if (!cond.op) return '必须选择运算符'

  switch (cond.field) {
    case 'display_name':
      if (!['contains', 'not_contains', 'regex', 'not_regex'].includes(cond.op)) {
        return '显示名称仅支持包含、不包含、正则或不匹配正则'
      }
      if (!cond.value || !cond.value.trim()) {
        return '显示名称值不能为空'
      }
      const maxBytes = cond.op === 'regex' || cond.op === 'not_regex' ? 1024 : 255
      if (new TextEncoder().encode(cond.value.trim()).length > maxBytes) {
        return `显示名称值长度不能超过 ${maxBytes} 个字符（${maxBytes} 字节上限）`
      }
      // PCRE lookaround is validated by the server, never rewritten as JS/RE2.
      break

    case 'protocol': {
      if (cond.op !== 'equals' && cond.op !== 'not_equals') {
        return '协议类型仅支持"等于"或"不等于"运算符'
      }
      if (!cond.value || !cond.value.trim()) {
        return '协议值不能为空'
      }
      const normProto = cond.value.trim().toLowerCase()
      if (!(SUPPORTED_PROTOCOLS as readonly string[]).includes(normProto)) {
        return `不支持的协议"${cond.value.trim()}"（有效协议：${SUPPORTED_PROTOCOLS.join(', ')}）`
      }
      break
    }

    case 'source_subscription_ids':
      if (cond.op !== 'contains' && cond.op !== 'not_contains') {
        return '来源订阅仅支持"包含"或"不包含"运算符'
      }
      if (!cond.value || !cond.value.trim()) {
        return '订阅 ID 不能为空'
      }
      break

    case 'probe_verdict':
      if (cond.op !== 'equals' && cond.op !== 'not_equals') {
        return '探针判定仅支持"等于"或"不等于"运算符'
      }
      if (!cond.probe_kind) {
        return '探针判定条件必须指定探针类型'
      }
      if (!cond.value || !cond.value.trim()) {
        return '判定结果值不能为空'
      }
      if (cond.freshness_seconds !== undefined && (cond.freshness_seconds < 1 || cond.freshness_seconds > 604800)) {
        return '有效期必须在 1 秒到 604800 秒（7 天）之间'
      }
      break

    case 'probe_latency_ms': {
      if (cond.op !== 'lte') {
        return '探针延迟仅支持"小于等于"运算符'
      }
      if (!cond.probe_kind) {
        return '探针延迟条件必须指定探针类型'
      }
      if (!cond.value || !cond.value.trim()) {
        return '延迟阈值 (ms) 不能为空'
      }
      const lat = Number(cond.value)
      if (isNaN(lat) || lat < 0 || lat > 60000) {
        return '延迟阈值必须在 0 到 60000 ms 之间'
      }
      if (cond.freshness_seconds !== undefined && (cond.freshness_seconds < 1 || cond.freshness_seconds > 604800)) {
        return '有效期必须在 1 秒到 604800 秒（7 天）之间'
      }
      break
    }

    default:
      return `不支持的字段：${cond.field}`
  }

  return null
}

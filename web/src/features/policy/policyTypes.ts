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

export type SourceOperation = 'add' | 'remove'

export type MemberSourceKind =
  | 'manual_node'
  | 'group_ref'
  | 'group_nodes'
  | 'regex'
  | 'filter'
  | 'typed_builtin'

export type BuiltinName = 'PASS' | 'DIRECT' | 'REJECT' | 'REJECT-DROP'

export const VALID_BUILTIN_NAMES: readonly BuiltinName[] = [
  'PASS',
  'DIRECT',
  'REJECT',
  'REJECT-DROP',
] as const

export interface MemberSource {
  id?: string
  group_id?: string
  operation: SourceOperation
  kind: MemberSourceKind
  position: number
  node_logical_id?: string
  target_group_id?: string
  pattern?: string
  filter_spec?: NodeFilterSpec
  builtin_name?: BuiltinName
  created_at?: string
  updated_at?: string
}

export interface PolicyGroup {
  id: string
  name: string
  group_type: GroupType
  position?: number
  empty_fallback_pass?: boolean
  inherit_global_filter?: boolean
  revision?: number
  sources?: MemberSource[]
  edges?: GroupEdge[]
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

export const SUPPORTED_PROBE_KINDS = [
  'baseline',
  'geo',
  'streaming',
  'ai',
  'speed',
  'ip_risk',
] as const

export type ProbeKind = typeof SUPPORTED_PROBE_KINDS[number]

export const SUPPORTED_PROBE_VERDICTS = [
  'available',
  'restricted',
  'unknown',
  'error',
  'stale',
] as const

export type ProbeVerdict = typeof SUPPORTED_PROBE_VERDICTS[number]

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
  probe_kind?: ProbeKind
  freshness_seconds?: number
}

export function filterFieldOperators(field: FilterField): FilterOp[] {
  switch (field) {
    case 'display_name':
      return ['contains', 'not_contains', 'regex', 'not_regex']
    case 'protocol':
      return ['equals', 'not_equals']
    case 'source_subscription_ids':
      return ['contains', 'not_contains']
    case 'probe_verdict':
      return ['equals', 'not_equals']
    case 'probe_latency_ms':
      return ['lte']
    default:
      return ['equals']
  }
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

export function sourceKindLabel(kind: MemberSourceKind | string): string {
  switch (kind) {
    case 'manual_node':
    case 'node':
      return '手工节点'
    case 'group_ref':
    case 'group':
      return '策略组引用'
    case 'group_nodes':
      return '解包策略组'
    case 'regex':
      return '节点名称正则'
    case 'filter':
    case 'matching':
      return '同级属性过滤'
    case 'typed_builtin':
    case 'builtin':
      return '内建流控项'
    default:
      return kind
  }
}

export function sourceOperationLabel(op: SourceOperation | string): string {
  switch (op) {
    case 'add':
      return '加入 (+)'
    case 'remove':
    case 'exclude':
      return '排除 (-)'
    default:
      return op
  }
}

export function normalizeGroupSources(group?: Partial<PolicyGroup> | null): MemberSource[] {
  if (!group) return []
  if (Array.isArray(group.sources)) {
    return group.sources.map((s, idx) => {
      const opStr = (s.operation as string) || 'add'
      const kindStr = (s.kind as string) || 'manual_node'
      const operation: SourceOperation = opStr === 'exclude' ? 'remove' : (opStr === 'remove' ? 'remove' : 'add')
      const kind: MemberSourceKind =
        kindStr === 'node'
          ? 'manual_node'
          : kindStr === 'builtin'
            ? 'typed_builtin'
            : kindStr === 'matching'
              ? 'filter'
              : (s.kind as MemberSourceKind) || 'manual_node'
      return {
        ...s,
        position: s.position !== undefined ? s.position : idx,
        operation,
        kind,
      }
    })
  }

  const result: MemberSource[] = []
  let pos = 0

  if (Array.isArray(group.edges) && group.edges.length > 0) {
    const sortedEdges = [...group.edges].sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
    for (const edge of sortedEdges) {
      if (edge.node_logical_id) {
        result.push({
          operation: 'add',
          kind: 'manual_node',
          position: pos++,
          node_logical_id: edge.node_logical_id,
        })
      } else if (edge.child_group_id) {
        result.push({
          operation: 'add',
          kind: 'group_ref',
          position: pos++,
          target_group_id: edge.child_group_id,
        })
      }
    }
  }

  if (group.node_filter?.conditions && group.node_filter.conditions.length > 0) {
    result.push({
      operation: 'add',
      kind: 'filter',
      position: pos++,
      filter_spec: {
        conditions: [...group.node_filter.conditions],
      },
    })
  }

  return result
}

export function validateRegexPattern(pattern: string): string | null {
  if (!pattern || !pattern.trim()) {
    return '节点匹配正则表达式不能为空'
  }
  const trimmed = pattern.trim()
  const maxBytes = 1024
  if (new TextEncoder().encode(trimmed).length > maxBytes) {
    return `正则表达式长度不能超过 ${maxBytes} 字节`
  }
  // Try JS RegExp validation, stripping PCRE inline flags like (?i) for JS compatibility
  const jsCompatible = trimmed.replace(/^\(\?[imsUx-]+\)/, '')
  try {
    new RegExp(jsCompatible)
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    return `正则表达式语法无效: ${msg}`
  }
  return null
}

export function validateSourceInput(
  source: Partial<MemberSource>,
  parentGroupId?: string
): string | null {
  if (!source.operation) {
    return '必须指定操作类型（加入或排除）'
  }
  const opStr = source.operation as string
  const op = opStr === 'exclude' ? 'remove' : source.operation
  if (op !== 'add' && op !== 'remove') {
    return '操作类型必须为"add"或"remove"'
  }

  if (!source.kind) {
    return '必须指定来源类别'
  }

  const kindStr = source.kind as string
  const kind = kindStr === 'node' ? 'manual_node' : kindStr === 'builtin' ? 'typed_builtin' : kindStr === 'matching' ? 'filter' : source.kind

  switch (kind) {
    case 'manual_node': {
      if (!source.node_logical_id || !source.node_logical_id.trim()) {
        return '手工节点必须指定节点逻辑 ID'
      }
      break
    }

    case 'group_ref': {
      if (!source.target_group_id || !source.target_group_id.trim()) {
        return '策略组引用必须指定目标策略组 ID'
      }
      if (parentGroupId && source.target_group_id.trim() === parentGroupId.trim()) {
        return '禁止自环：父策略组不能引用自身'
      }
      break
    }

    case 'group_nodes': {
      if (!source.target_group_id || !source.target_group_id.trim()) {
        return '解包策略组必须指定目标策略组 ID'
      }
      if (parentGroupId && source.target_group_id.trim() === parentGroupId.trim()) {
        return '禁止自环：父策略组不能展开自身'
      }
      break
    }

    case 'regex': {
      const err = validateRegexPattern(source.pattern || '')
      if (err) return err
      break
    }

    case 'filter': {
      if (
        !source.filter_spec?.conditions ||
        source.filter_spec.conditions.length === 0
      ) {
        return '同级属性过滤来源至少需要包含一个有效条件'
      }
      if (source.filter_spec.conditions.length > 32) {
        return '单个来源项最多允许添加 32 条过滤条件'
      }
      for (const cond of source.filter_spec.conditions) {
        const err = validateConditionInput(cond)
        if (err) return err
      }
      break
    }

    case 'typed_builtin': {
      if (!source.builtin_name) {
        return '必须选择内建流控项名称'
      }
      if (!VALID_BUILTIN_NAMES.includes(source.builtin_name)) {
        return `不支持的内建项"${source.builtin_name}"（有效项：${VALID_BUILTIN_NAMES.join(', ')}）`
      }
      break
    }

    default:
      return `不支持的来源类别：${source.kind}`
  }

  if (source.position !== undefined && source.position < 0) {
    return '位置序号必须为非负数'
  }

  return null
}

export interface GroupDraftPreviewPayload {
  group_id?: string
  expected_revision?: number
  name?: string
  group_type?: GroupType
  empty_fallback_pass?: boolean
  inherit_global_filter?: boolean
  sources: MemberSource[]
  target?: string
}

export type ResolvedMemberKind = 'node' | 'group' | 'builtin' | MemberSourceKind

export interface ResolvedGroupMember {
  kind: ResolvedMemberKind
  display_name?: string
  position?: number
  node_logical_id?: string
  group_id?: string
  builtin_name?: string
}

export interface GroupFilterCount {
  candidate: number
  kept: number
  excluded: number
  exclusion_summary?: Record<string, number>
}

export interface ResolverDiagnostic {
  severity: 'info' | 'warning' | 'error' | string
  code: string
  message: string
  target?: string
  reason?: string
  excluded_count?: number
}

export interface GroupDraftPreviewResponse {
  group_id?: string
  revision?: number
  selection_digest: string
  content_digest?: string
  content?: string
  members: ResolvedGroupMember[]
  member_count?: number
  node_count: number
  candidate_count?: number
  excluded_count?: number
  exclusion_summary?: Record<string, number>
  excluded_reasons?: string[]
  filter_counts?: GroupFilterCount
  diagnostics?: Array<ResolverDiagnostic | string>
}

export interface SourceSemanticInfo {
  source: MemberSource
  position: number
  isReEntry: boolean
  isDuplicate: boolean
  semanticText: string
}

export function computeSourceSemantics(sources: MemberSource[]): SourceSemanticInfo[] {
  const seenTargets = new Map<string, { lastOp: SourceOperation; firstPos: number }>()

  return sources.map((s, idx) => {
    const opStr = s.operation as string
    const op: SourceOperation = opStr === 'exclude' ? 'remove' : s.operation
    let targetKey = ''
    const isDynamic = s.kind === 'regex' || s.kind === 'filter' || s.kind === 'group_nodes'

    switch (s.kind) {
      case 'manual_node':
        targetKey = `node:${s.node_logical_id || ''}`
        break
      case 'group_ref':
        targetKey = `group_ref:${s.target_group_id || ''}`
        break
      case 'group_nodes':
        targetKey = `group_nodes:${s.target_group_id || ''}`
        break
      case 'regex':
        targetKey = `regex:${s.pattern || ''}`
        break
      case 'filter':
        targetKey = `filter:${JSON.stringify(s.filter_spec?.conditions || [])}`
        break
      case 'typed_builtin':
        targetKey = `builtin:${s.builtin_name || ''}`
        break
      default:
        targetKey = `${s.kind}:${s.position}`
    }

    const previous = seenTargets.get(targetKey)
    let isReEntry = false
    let isDuplicate = false
    let semanticText = ''

    if (isDynamic) {
      // Dynamic pattern/filter/expansion: actual resolved members are determined by backend resolver
      if (op === 'remove') {
        semanticText = `动态排除匹配项（第 #${idx + 1} 步，实际成员由后端解析管道计算）`
      } else {
        if (previous && previous.lastOp === 'add') {
          isDuplicate = true
          semanticText = `重复定义匹配规则（首次定义于 #${previous.firstPos + 1} 步）`
        } else {
          semanticText = `动态加入匹配项（第 #${idx + 1} 步，实际成员由后端解析管道计算）`
        }
      }
    } else {
      // Static discrete entities: manual_node, group_ref, typed_builtin
      if (op === 'remove') {
        semanticText = '排除仅当前成员，后续可加入'
      } else {
        if (previous) {
          if (previous.lastOp === 'remove') {
            isReEntry = true
            semanticText = `重入入选（位置 #${idx + 1}，恢复前置排除）`
          } else {
            isDuplicate = true
            semanticText = `重复项（首次出现于 #${previous.firstPos + 1}）`
          }
        } else {
          semanticText = '首次出现，加入候选'
        }
      }
    }

    seenTargets.set(targetKey, {
      lastOp: op,
      firstPos: previous ? previous.firstPos : idx,
    })

    return {
      source: s,
      position: idx,
      isReEntry,
      isDuplicate,
      semanticText,
    }
  })
}

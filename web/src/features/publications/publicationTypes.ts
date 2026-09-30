export type CompilerTarget = 'mihomo' | 'singbox' | 'surge' | 'qx'

export const DEFAULT_COMPILER_TARGET: CompilerTarget = 'mihomo'

export const VALID_COMPILER_TARGETS: readonly CompilerTarget[] = [
  'mihomo',
  'singbox',
  'surge',
  'qx',
] as const

export interface TargetMetadata {
  target: CompilerTarget
  label: string
  ext: string
  mimeType: string
  desc: string
  protocols: readonly string[]
  groupTypes: readonly string[]
  ruleKinds: readonly string[]
  ruleSummary: string
  unsupportedSummary: string
}

export const COMPILER_TARGETS: TargetMetadata[] = [
  {
    target: 'mihomo',
    label: 'Mihomo',
    ext: 'yaml',
    mimeType: 'application/x-yaml',
    desc: '导出完整 Mihomo 配置（含节点、策略组与分流规则）',
    protocols: ['ss', 'vmess', 'vless', 'trojan', 'hysteria2', 'wireguard', 'tuic'],
    groupTypes: ['select', 'urltest', 'fallback', 'loadbalance'],
    ruleKinds: [
      'DOMAIN',
      'DOMAIN-SUFFIX',
      'DOMAIN-KEYWORD',
      'IP-CIDR',
      'IP-CIDR6',
      'GEOIP',
      'GEOSITE',
      'RULE-SET',
      'SRC-IP-CIDR',
      'SRC-PORT',
      'DST-PORT',
      'PORT',
      'PROCESS-NAME',
      'MATCH',
    ],
    ruleSummary: '导出完整配置（含 14 类分流规则：DOMAIN*、IP-CIDR*、GEOIP、GEOSITE、RULE-SET、SRC-IP-CIDR、SRC/DST-PORT、PORT、PROCESS-NAME、MATCH）',
    unsupportedSummary: '拒绝凭据不完整的 WireGuard/TUIC 节点、空路由策略组或无法解析的规则提供者',
  },
  {
    target: 'singbox',
    label: 'sing-box',
    ext: 'json',
    mimeType: 'application/json',
    desc: '仅导出 sing-box 节点格式（全 7 协议 outbounds/endpoints，忽略策略组与规则）',
    protocols: ['ss', 'vmess', 'vless', 'trojan', 'hysteria2', 'wireguard', 'tuic'],
    groupTypes: [],
    ruleKinds: [],
    ruleSummary: '仅导出 sing-box 节点格式（全 7 协议 outbounds/endpoints，忽略策略组与规则）',
    unsupportedSummary: '仅校验节点协议与连接凭据完整性，忽略策略组与分流规则',
  },
  {
    target: 'surge',
    label: 'Surge',
    ext: 'conf',
    mimeType: 'text/plain',
    desc: '仅导出 Surge 节点格式（支持 SS/VMess/Trojan/Hysteria2/TUIC/WireGuard，忽略策略组与规则）',
    protocols: ['ss', 'vmess', 'trojan', 'hysteria2', 'tuic', 'wireguard'],
    groupTypes: [],
    ruleKinds: [],
    ruleSummary: '仅导出 Surge 节点格式（支持 SS/VMess/Trojan/Hysteria2/TUIC/WireGuard，忽略策略组与规则）',
    unsupportedSummary: '拒绝 VLESS 协议节点；忽略策略组与分流规则',
  },
  {
    target: 'qx',
    label: 'Quantumult X',
    ext: 'conf',
    mimeType: 'text/plain',
    desc: '仅导出 Quantumult X 节点格式（支持 SS/VMess/Trojan，忽略策略组与规则）',
    protocols: ['ss', 'vmess', 'trojan'],
    groupTypes: [],
    ruleKinds: [],
    ruleSummary: '仅导出 Quantumult X 节点格式（支持 SS/VMess/Trojan，忽略策略组与规则）',
    unsupportedSummary: '拒绝 VLESS/WireGuard/TUIC/Hysteria2 协议节点；忽略策略组与分流规则',
  },
]

export interface Diagnostic {
  code?: string
  target?: string
  message: string
  severity: string
  excluded_count?: number
  reason?: string
}

export interface FilterLayerCounts {
  raw_total?: number
  admitted_total?: number
  global_filtered_total?: number
  group_filtered_total?: number
  group_counts?: Record<string, { candidate: number; kept: number; excluded: number }>
}

export interface PreviewResult {
  target: CompilerTarget
  snapshot_digest: string
  content_digest: string
  content: string
  content_type: string
  filename: string
  diagnostics?: Diagnostic[]
  filter_counts?: FilterLayerCounts
}

export interface PublicationDetail {
  id: string
  target: CompilerTarget
  state?: 'active' | 'revoked'
  snapshot_digest: string
  content_digest?: string
  content_type?: string
  filename?: string
  export_url?: string
  revoked_at?: string
  created_at: string
}

export function isValidCompilerTarget(target: unknown): target is CompilerTarget {
  return typeof target === 'string' && (VALID_COMPILER_TARGETS as readonly string[]).includes(target)
}

export function getTargetMetadata(target: CompilerTarget): TargetMetadata {
  return COMPILER_TARGETS.find((t) => t.target === target) ?? COMPILER_TARGETS[0]
}

export function targetLabel(target: CompilerTarget): string {
  const meta = COMPILER_TARGETS.find((t) => t.target === target)
  return meta ? meta.label : target
}

export function targetFileExt(target: CompilerTarget): string {
  const meta = COMPILER_TARGETS.find((t) => t.target === target)
  return meta ? meta.ext : 'txt'
}

export function formatDigest(digest: string): string {
  if (!digest || digest.length <= 16) return digest || '--'
  return `${digest.slice(0, 8)}...${digest.slice(-5)}`
}

const GROUP_TYPE_LABELS: Record<string, string> = {
  select: '手动选择 (select)',
  urltest: '自动测速 (urltest)',
  fallback: '故障转移 (fallback)',
  loadbalance: '负载均衡 (loadbalance)',
}

export function groupTypeLabel(groupType: string): string {
  return GROUP_TYPE_LABELS[groupType.toLowerCase()] ?? groupType
}

export function publicationStateLabel(state?: string, revokedAt?: string): string {
  if (revokedAt || state === 'revoked') {
    return '已撤销'
  }
  return '已生效'
}

const PREFLIGHT_CHECK_LABELS: Record<string, string> = {
  empty_routed_group: '空路由策略组检查',
  filtered_nodes_empty: '筛选后无可导出节点',
  risk_blocked: '高风险节点拦截检查',
  risk_review: '中风险节点复核检查',
  risk_unknown: '未探测风险节点检查',
  unsupported_target_capability: '目标能力兼容性检查',
}

export function preflightCheckLabel(code?: string): string {
  if (!code) return '预检检查项'
  return PREFLIGHT_CHECK_LABELS[code] ?? code
}

const AUDIT_EVENT_LABELS: Record<string, string> = {
  'publication.create': '创建订阅发布',
  'publication.revoke': '撤销订阅发布',
  'publication.preflight': '发布前置预检',
  'publication.export': '客户端拉取订阅',
}

export function auditEventLabel(event?: string): string {
  if (!event) return '审计事件'
  return AUDIT_EVENT_LABELS[event] ?? event
}



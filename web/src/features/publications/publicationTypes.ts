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
    desc: 'Mihomo native YAML subscription profile (full 7 protocols, 4 group types, 14 modern rules)',
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
    ruleSummary: '14 modern rules (DOMAIN*, IP-CIDR*, GEOIP, GEOSITE, RULE-SET, SRC-IP-CIDR, SRC/DST-PORT, PORT, PROCESS-NAME, MATCH)',
    unsupportedSummary: 'Rejects incomplete WG/TUIC credentials or unresolvable rule-providers',
  },
  {
    target: 'singbox',
    label: 'sing-box',
    ext: 'json',
    mimeType: 'application/json',
    desc: 'sing-box official JSON options profile (7 protocols with native WG endpoint, select/urltest groups, 14 rules)',
    protocols: ['ss', 'vmess', 'vless', 'trojan', 'hysteria2', 'wireguard', 'tuic'],
    groupTypes: ['select', 'urltest'],
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
    ruleSummary: '14 modern rules mapped to official sing-box route & rule_set options',
    unsupportedSummary: 'Rejects fallback and loadbalance policy group types',
  },
  {
    target: 'surge',
    label: 'Surge',
    ext: 'conf',
    mimeType: 'text/plain',
    desc: 'Surge 5 INI configuration subset ([Proxy], [Proxy Group], [Rule])',
    protocols: ['ss', 'vmess', 'trojan'],
    groupTypes: ['select', 'urltest', 'fallback'],
    ruleKinds: [
      'DOMAIN',
      'DOMAIN-SUFFIX',
      'DOMAIN-KEYWORD',
      'IP-CIDR',
      'IP-CIDR6',
      'GEOIP',
      'PROCESS-NAME',
      'SRC-IP',
      'DEST-PORT',
      'IN-PORT',
      'RULE-SET',
      'USER-AGENT',
      'URL-REGEX',
      'MATCH',
      'FINAL',
    ],
    ruleSummary: 'Surge 5 [Rule] subset (DOMAIN*, IP-CIDR*, GEOIP, PROCESS-NAME, RULE-SET, SRC-IP, DEST/IN-PORT, MATCH/FINAL)',
    unsupportedSummary: 'Rejects VLESS/WireGuard/TUIC/Hysteria2 protocols, loadbalance groups, and GEOSITE rules',
  },
  {
    target: 'qx',
    label: 'Quantumult X',
    ext: 'conf',
    mimeType: 'text/plain',
    desc: 'Quantumult X INI subset ([server_local], [policy], [filter_local])',
    protocols: ['ss', 'vmess', 'trojan'],
    groupTypes: ['select'],
    ruleKinds: [
      'DOMAIN',
      'DOMAIN-SUFFIX',
      'DOMAIN-KEYWORD',
      'IP-CIDR',
      'IP-CIDR6',
      'GEOIP',
      'MATCH',
    ],
    ruleSummary: 'Quantumult X [filter_local] subset (DOMAIN, DOMAIN-SUFFIX, DOMAIN-KEYWORD, IP-CIDR, IP-CIDR6, GEOIP, MATCH)',
    unsupportedSummary: 'Rejects VLESS/WireGuard/TUIC/Hysteria2, urltest/fallback/loadbalance groups, and GEOSITE/RULE-SET/PROCESS-NAME rules',
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


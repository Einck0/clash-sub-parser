export type CompilerTarget = 'clash' | 'mihomo' | 'singbox' | 'surge' | 'qx'

export interface TargetMetadata {
  target: CompilerTarget
  label: string
  ext: string
  mimeType: string
  desc: string
}

export const COMPILER_TARGETS: TargetMetadata[] = [
  {
    target: 'clash',
    label: 'Clash',
    ext: 'yaml',
    mimeType: 'application/x-yaml',
    desc: 'Standard Clash YAML subscription profile',
  },
  {
    target: 'mihomo',
    label: 'Mihomo',
    ext: 'yaml',
    mimeType: 'application/x-yaml',
    desc: 'Clash.Meta / Mihomo enhanced client profile',
  },
  {
    target: 'singbox',
    label: 'sing-box',
    ext: 'json',
    mimeType: 'application/json',
    desc: 'sing-box modern rule-set & outbound profile',
  },
  {
    target: 'surge',
    label: 'Surge',
    ext: 'conf',
    mimeType: 'text/plain',
    desc: 'Surge iOS / macOS configuration format',
  },
  {
    target: 'qx',
    label: 'Quantumult X',
    ext: 'conf',
    mimeType: 'text/plain',
    desc: 'Quantumult X server & filter subscription list',
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
  snapshot_digest: string
  content_digest?: string
  export_url?: string
  revoked_at?: string
  created_at: string
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

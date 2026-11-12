export type ProbeKind = 'baseline' | 'geo' | 'streaming' | 'ai' | 'speed' | 'ip_risk'

export type ProbeRunState =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'expired'

export type ProbeVerdict =
  | 'available'
  | 'abnormal'
  | 'restricted'
  | 'unknown'
  | 'error'
  | 'skipped'
  | 'stale'

export type PlatformSubTier =
  | 'full'
  | 'web'
  | 'app'
  | 'originals'
  | 'banned'
  | 'unlocked'
  | 'soon'
  | 'reachable'

export type RegionSource = 'service' | 'cdn_trace' | 'cdn_target' | string

export type EvidenceScope = 'network_entry' | 'web_region' | 'cdn_trace' | string

export interface PlatformCapability {
  verdict: ProbeVerdict
  latency_ms?: number
  observed_at?: string
  summary?: string
  region?: string
  sub_tier?: PlatformSubTier
  throughput?: number
  risk_score?: string
  reason?: string
  exit_region?: string
  region_source?: RegionSource
  evidence_scope?: EvidenceScope
}

export interface PlatformBadge {
  platform: string
  verdict: ProbeVerdict
  label: string
  tone: 'success' | 'warning' | 'error' | 'info'
  region?: string
  subTier?: PlatformSubTier
  tooltip?: string
  throughput?: number
  riskScore?: string
  exitRegion?: string
  regionSource?: string
  evidenceScope?: string
}

export interface ProbeRun {
  id: string
  idempotency_key: string
  actor_scope: string
  config_revision: string
  state: ProbeRunState
  deadline_at: string
  created_at: string
  updated_at: string
}

export interface ProbeAttemptEvidence {
  executed: boolean
  category: string
  reason: string
  raw_verdict: ProbeVerdict
  status?: number
  body_bytes?: number
  config_fingerprint?: string
  config_revision?: string
  profile?: string
  contract?: string
  contract_source?: string
  legacy_contract?: string
  engine?: string
  source?: string
  attempt_id?: string
  reverification_of?: string
  started_at?: string
  finished_at?: string
  budget_limit?: number
  budget_used?: number
  responses?: unknown
}

export interface ProbeObservation {
  id: string
  probe_run_id: string
  node_logical_id: string
  kind: ProbeKind
  verdict: ProbeVerdict
  evidence_digest: string
  observed_at: string
  latency_ms: number
  redacted_summary: string
  connection_revision?: number | null
  region?: string
  sub_tier?: PlatformSubTier
  throughput?: number
  risk_score?: string
  platforms?: Record<string, PlatformCapability>
  attempt?: ProbeAttemptEvidence
}

export interface CreateProbeRunInput {
  kinds?: ProbeKind[]
  node_logical_ids?: string[]
  config_revision?: string
  deadline_minutes?: number
}

export interface CreateProbeRunResponse {
  run_id: string
  state: ProbeRunState
  deadline_at: string
}

export const ALL_PROBE_KINDS: Array<{ kind: ProbeKind; label: string; desc: string; emoji: string }> = [
  { kind: 'baseline', label: '连通与延迟', desc: 'TCP、TLS 握手与基础连通性检测', emoji: '⚡' },
  { kind: 'streaming', label: '流媒体解锁', desc: 'Netflix、YouTube 与哔哩哔哩解锁检测', emoji: '🎬' },
  { kind: 'ai', label: 'AI 可用性', desc: 'OpenAI、Gemini 与 Claude 可访问性检测', emoji: '🤖' },
  { kind: 'ip_risk', label: 'IP 风险', desc: 'Scamalytics、IPQS 与 IP-API 风险评分', emoji: '🛡️' },
  { kind: 'geo', label: '落地地区', desc: '出口 IP、ISO 国家/地区代码与 ASN 识别', emoji: '🌍' },
  { kind: 'speed', label: '带宽测速', desc: '延迟与吞吐带宽基准测试', emoji: '🚀' },
]

export function probeKindLabel(kind: ProbeKind | string): string {
  switch (kind) {
    case 'baseline':
      return '基础连通性'
    case 'geo':
      return '地域与出口 IP'
    case 'streaming':
      return '流媒体解锁'
    case 'ai':
      return 'AI 服务'
    case 'speed':
      return '带宽测速'
    case 'ip_risk':
      return 'IP 风险'
    default:
      return kind
  }
}

export function probeKindEmoji(kind: ProbeKind | string): string {
  const found = ALL_PROBE_KINDS.find((k) => k.kind === kind)
  return found?.emoji ?? '🔍'
}

export function probeStateLabel(state: ProbeRunState | ProbeBatchState | string): string {
  switch (state) {
    case 'pending':
      return '等待中'
    case 'queued':
      return '排队中'
    case 'running':
      return '运行中'
    case 'succeeded':
      return '已完成'
    case 'failed':
      return '已失败'
    case 'cancelled':
      return '已取消'
    case 'expired':
      return '已过期'
    default:
      return state
  }
}

export function probeVerdictLabel(verdict: ProbeVerdict | string): string {
  switch (verdict) {
    case 'available':
      return '可用'
    case 'abnormal':
      return '异常'
    case 'restricted':
      return '降级/受限'
    case 'unknown':
      return '未知'
    case 'error':
      return '检测失败'
    case 'skipped':
      return '未执行'
    case 'stale':
      return '已过期'
    default:
      return verdict
  }
}

export function probeStateTone(state: ProbeRunState): 'primary' | 'success' | 'warning' | 'error' | 'neutral' {
  switch (state) {
    case 'queued':
      return 'warning'
    case 'running':
      return 'primary'
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'error'
    case 'cancelled':
    case 'expired':
    default:
      return 'neutral'
  }
}

export function probeVerdictTone(verdict: ProbeVerdict): 'success' | 'warning' | 'error' | 'info' | 'neutral' {
  switch (verdict) {
    case 'available':
      return 'success'
    case 'abnormal':
      return 'warning'
    case 'restricted':
      return 'warning'
    case 'error':
      return 'error'
    case 'skipped':
      return 'info'
    case 'unknown':
      return 'info'
    case 'stale':
    default:
      return 'neutral'
  }
}

export function formatLatency(ms: number): string {
  if (ms < 0) return '--'
  return `${ms} ms`
}

/** Format speed throughput into reasonable KB/s or MB/s */
export function formatSpeed(kbPerSec?: number | null): string {
  if (kbPerSec == null || kbPerSec < 0 || Number.isNaN(kbPerSec)) return '--'
  if (kbPerSec < 1024) {
    return `${Math.round(kbPerSec)} KB/s`
  }
  const mb = kbPerSec / 1024
  return `${mb.toFixed(1)} MB/s`
}

/** Compute overall group verdict from platform dictionary adhering to frozen domain precedence */
export function computeGroupVerdict(
  platforms?: Record<string, PlatformCapability>
): ProbeVerdict {
  if (!platforms) return 'skipped'
  const values = Object.values(platforms)
  if (values.length === 0) return 'skipped'

  const VALID_VERDICTS = new Set<string>([
    'available',
    'abnormal',
    'restricted',
    'unknown',
    'error',
    'skipped',
    'stale',
  ])

  let hasAbnormal = false
  let hasAvailable = false
  let hasUnknownOrStale = false
  let hasRestricted = false
  let hasError = false
  let skippedCount = 0

  for (const p of values) {
    const v = p?.verdict
    if (!v || !VALID_VERDICTS.has(v)) {
      hasUnknownOrStale = true
      continue
    }

    switch (v) {
      case 'abnormal':
        hasAbnormal = true
        break
      case 'available':
        hasAvailable = true
        break
      case 'unknown':
      case 'stale':
        hasUnknownOrStale = true
        break
      case 'restricted':
        hasRestricted = true
        break
      case 'error':
        hasError = true
        break
      case 'skipped':
        skippedCount++
        break
    }
  }

  if (hasAbnormal) return 'abnormal'
  if (hasAvailable) return 'available'
  if (hasUnknownOrStale) return 'unknown'
  if (hasRestricted) return 'restricted'
  if (hasError) return 'error'
  if (skippedCount === values.length) return 'skipped'
  return 'skipped'
}

/** Normalize raw platform dictionary from snake_case or camelCase */
export function normalizePlatforms(
  raw?: Record<string, any>
): Record<string, PlatformCapability> | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const result: Record<string, PlatformCapability> = {}
  for (const [k, v] of Object.entries(raw)) {
    if (!v || typeof v !== 'object') continue
    let verdict: ProbeVerdict = 'unknown'
    if (v.verdict && typeof v.verdict === 'string') {
      const normV = v.verdict.toLowerCase().trim() as ProbeVerdict
      if (['available', 'abnormal', 'restricted', 'unknown', 'error', 'skipped', 'stale'].includes(normV)) {
        verdict = normV
      }
    }
    const latency =
      typeof v.latency_ms === 'number'
        ? v.latency_ms
        : typeof v.latencyMs === 'number'
        ? v.latencyMs
        : undefined
    const observedAt = v.observed_at || v.observedAt
    const subTier = v.sub_tier || v.subTier
    const throughput = typeof v.throughput === 'number' ? v.throughput : undefined
    const riskScore =
      v.risk_score != null ? String(v.risk_score) : v.riskScore != null ? String(v.riskScore) : undefined
    const rawRegion = typeof v.region === 'string' ? v.region.trim() : undefined
    const region = rawRegion && rawRegion.toLowerCase() !== 'unknown' ? rawRegion : undefined

    const rawExitRegion =
      typeof v.exit_region === 'string'
        ? v.exit_region.trim()
        : typeof v.exitRegion === 'string'
        ? v.exitRegion.trim()
        : undefined
    const exitRegion = rawExitRegion && rawExitRegion.toLowerCase() !== 'unknown' ? rawExitRegion : undefined

    const rawRegionSource =
      typeof v.region_source === 'string'
        ? v.region_source.trim()
        : typeof v.regionSource === 'string'
        ? v.regionSource.trim()
        : undefined
    const regionSource = rawRegionSource || undefined

    const rawEvidenceScope =
      typeof v.evidence_scope === 'string'
        ? v.evidence_scope.trim()
        : typeof v.evidenceScope === 'string'
        ? v.evidenceScope.trim()
        : undefined
    const evidenceScope = rawEvidenceScope || undefined

    result[k.toLowerCase()] = {
      verdict,
      latency_ms: latency,
      observed_at: observedAt,
      summary: v.summary,
      region,
      sub_tier: subTier,
      throughput,
      risk_score: riskScore,
      reason: v.reason,
      exit_region: exitRegion,
      region_source: regionSource,
      evidence_scope: evidenceScope,
    }
  }
  return Object.keys(result).length > 0 ? result : undefined
}

export function buildPlatformTooltip(platform: string, cap: PlatformCapability): string {
  const normPlat = platform.trim().toLowerCase()
  const parts: string[] = []

  // 1. Platform-specific headline or summary
  if (normPlat === 'antigravity') {
    const entryReachable = cap.verdict === 'available' && cap.sub_tier === 'reachable' &&
      cap.evidence_scope === 'network_entry' && cap.reason === 'antigravity_entry_reachable'
    const scope = entryReachable ? '官网入口可达' : '检测范围：官网入口连通性'
    parts.push(`${scope}；个人账号所在国家/地区、年龄(18+)及模型资格未核验（依据官方 FAQ）`)
  } else if (normPlat === 'gemini') {
    parts.push('网页端区域支持检测；个人账号登录、年龄及模型权限未核验')
  } else if (normPlat === 'openai') {
    if (cap.sub_tier === 'full') parts.push('OpenAI 网页 + App 双通')
    else if (cap.sub_tier === 'web') parts.push('OpenAI 仅网页端可用')
    else if (cap.sub_tier === 'app') parts.push('OpenAI 仅客户端可用')
    else if (cap.sub_tier === 'banned') parts.push('OpenAI 封禁')
  } else if (normPlat === 'netflix') {
    if (cap.sub_tier === 'full') parts.push('Netflix 全量剧集解锁')
    else if (cap.sub_tier === 'originals') parts.push('Netflix 仅自制剧')
    else if (cap.sub_tier === 'banned') parts.push('Netflix 已封禁')
  } else if (normPlat === 'disney') {
    if (cap.sub_tier === 'unlocked') parts.push('Disney+ 已解锁')
    else if (cap.sub_tier === 'soon') parts.push('Disney+ 尚未开放')
    else if (cap.sub_tier === 'banned') parts.push('Disney+ 已封禁')
  }

  // 2. Reason label / summary
  const reasonText = cap.reason ? formatObservationReasonLabel(cap.reason) : ''
  if (reasonText && !parts.some((p) => p.includes(reasonText))) {
    parts.push(reasonText)
  }
  if (cap.summary && cap.summary !== reasonText && !parts.some((p) => p.includes(cap.summary!))) {
    parts.push(cap.summary)
  }

  // 3. Observed service country vs exit/geo reference
  const srvRegion = cap.region && cap.region.toLowerCase() !== 'unknown' ? cap.region.toUpperCase() : undefined
  const exitRegion =
    cap.exit_region && cap.exit_region.toLowerCase() !== 'unknown' ? cap.exit_region.toUpperCase() : undefined

  if (srvRegion && exitRegion) {
    if (srvRegion !== exitRegion) {
      parts.push(`服务观测地区: ${srvRegion}，出口参考地区: ${exitRegion}（地区存在差异，非物理路由绝对证明）`)
    } else {
      parts.push(`服务观测地区: ${srvRegion}，出口参考地区: ${exitRegion}`)
    }
  } else if (srvRegion) {
    parts.push(`服务观测地区: ${srvRegion}`)
  } else if (exitRegion) {
    parts.push(`出口参考地区: ${exitRegion}`)
  }

  // 4. Region Source display
  if (cap.region_source) {
    const srcMap: Record<string, string> = {
      service: '服务直测 (service)',
      cdn_trace: 'CDN 追踪 (cdn_trace)',
      cdn_target: 'CDN 目标节点 (cdn_target)',
    }
    const srcLabel = srcMap[cap.region_source] || cap.region_source
    parts.push(`地区来源: ${srcLabel}`)
  }

  // 5. Evidence Scope display
  if (cap.evidence_scope) {
    const scopeMap: Record<string, string> = {
      network_entry: '观测范围: 网站入口连通性 (network_entry)',
      web_region: '观测范围: 网页端区域支持 (web_region)',
      cdn_trace: '观测范围: CDN 节点出口 (cdn_trace)',
    }
    const scopeLabel = scopeMap[cap.evidence_scope] || `观测范围: ${cap.evidence_scope}`
    parts.push(scopeLabel)
  }

  return parts.join(' · ')
}

/** Format platform status badge adhering to beck-8 and subcheck semantics */
export function formatPlatformBadge(platform: string, cap: PlatformCapability): PlatformBadge {
  const normPlat = platform.trim().toLowerCase()
  const rawRegion = typeof cap.region === 'string' ? cap.region.trim() : undefined
  const region = rawRegion && rawRegion.toLowerCase() !== 'unknown' ? rawRegion : undefined
  const subTier = cap.sub_tier
  const verdict = cap.verdict || 'unknown'
  const isStale = verdict === 'stale'

  let label = ''
  let tone: 'success' | 'warning' | 'error' | 'info' = 'info'
  const tooltip: string | undefined = buildPlatformTooltip(normPlat, cap) || undefined

  switch (normPlat) {
    case 'openai':
      if (subTier === 'banned') {
        label = region ? `GPT 封禁 (${region})` : 'GPT 封禁'
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `GPT 异常 (${region})` : 'GPT 异常'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'GPT 未执行'
        tone = 'info'
      } else if (subTier === 'full') {
        label = region ? `GPT⁺ (${region})` : 'GPT⁺'
        tone = 'success'
      } else if (subTier === 'web') {
        label = region ? `GPT (${region})` : 'GPT'
        tone = 'warning'
      } else if (subTier === 'app') {
        label = region ? `GPT App (${region})` : 'GPT App'
        tone = 'warning'
      } else if (verdict === 'available') {
        label = region ? `GPT⁺ (${region})` : 'GPT⁺'
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `GPT 受限 (${region})` : 'GPT 受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `GPT 失败 (${region})` : 'GPT 失败'
        tone = 'error'
      } else {
        label = region ? `GPT (${region})` : 'GPT'
        tone = 'info'
      }
      break

    case 'netflix':
      if (subTier === 'banned') {
        label = region ? `NF 封禁 (${region})` : 'NF 封禁'
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `NF 异常 (${region})` : 'NF 异常'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'NF 未执行'
        tone = 'info'
      } else if (subTier === 'full') {
        label = region ? `NF (${region})` : 'NF'
        tone = 'success'
      } else if (subTier === 'originals') {
        label = region ? `NF 仅自制 (${region})` : 'NF 仅自制'
        tone = 'warning'
      } else if (verdict === 'available') {
        label = region ? `NF (${region})` : 'NF'
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `NF 受限 (${region})` : 'NF 受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `NF 失败 (${region})` : 'NF 失败'
        tone = 'error'
      } else {
        label = region ? `NF (${region})` : 'NF'
        tone = 'info'
      }
      break

    case 'disney':
      if (subTier === 'banned') {
        label = region ? `D+ 封禁 (${region})` : 'D+ 封禁'
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `D+ 异常 (${region})` : 'D+ 异常'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'D+ 未执行'
        tone = 'info'
      } else if (subTier === 'unlocked') {
        label = region ? `D+ (${region})` : 'D+'
        tone = 'success'
      } else if (subTier === 'soon') {
        label = region ? `D+ 尚未开放 (${region})` : 'D+ 尚未开放'
        tone = 'warning'
      } else if (verdict === 'available') {
        label = region ? `D+ (${region})` : 'D+'
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `D+ 受限 (${region})` : 'D+ 受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `D+ 失败 (${region})` : 'D+ 失败'
        tone = 'error'
      } else {
        label = region ? `D+ (${region})` : 'D+'
        tone = 'info'
      }
      break

    case 'youtube':
      if (subTier === 'banned') {
        label = region ? `YT 封禁 (${region})` : 'YT 封禁'
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `YT 异常 (${region})` : 'YT 异常'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'YT 未执行'
        tone = 'info'
      } else if (verdict === 'available') {
        label = region ? `YT (${region})` : 'YT'
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `YT 受限 (${region})` : 'YT 受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `YT 失败 (${region})` : 'YT 失败'
        tone = 'error'
      } else {
        label = region ? `YT (${region})` : 'YT'
        tone = 'info'
      }
      break

    case 'claude':
      if (subTier === 'banned') {
        label = region ? `Claude 封禁 (${region})` : 'Claude 封禁'
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `Claude 异常 (${region})` : 'Claude 异常'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'Claude 未执行'
        tone = 'info'
      } else if (verdict === 'available') {
        label = region ? `Claude (${region})` : 'Claude'
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `Claude 受限 (${region})` : 'Claude 受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `Claude 失败 (${region})` : 'Claude 失败'
        tone = 'error'
      } else {
        label = region ? `Claude (${region})` : 'Claude'
        tone = 'info'
      }
      break

    case 'gemini':
      if (subTier === 'banned') {
        label = region ? `Gemini 封禁 (${region})` : 'Gemini 封禁'
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `Gemini 异常 (${region})` : 'Gemini 异常'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'Gemini 未执行'
        tone = 'info'
      } else if (verdict === 'available') {
        label = region ? `Gemini (${region})` : 'Gemini'
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `Gemini 受限 (${region})` : 'Gemini 受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `Gemini 失败 (${region})` : 'Gemini 失败'
        tone = 'error'
      } else {
        label = region ? `Gemini (${region})` : 'Gemini'
        tone = 'info'
      }
      break

    case 'antigravity':
      if (subTier === 'banned') {
        label = region ? `Antigravity 封禁 (${region})` : 'Antigravity 封禁'
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `Antigravity 异常 (${region})` : 'Antigravity 异常'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'Antigravity 未执行'
        tone = 'info'
      } else if (verdict === 'available') {
        label = region ? `Antigravity 入口可达 (${region})` : 'Antigravity 入口可达'
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `Antigravity 受限 (${region})` : 'Antigravity 受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `Antigravity 失败 (${region})` : 'Antigravity 失败'
        tone = 'error'
      } else {
        label = region ? `Antigravity (${region})` : 'Antigravity'
        tone = 'info'
      }
      break

    default: {
      const platName = platform.toUpperCase()
      if (subTier === 'banned') {
        label = region ? `${platName} 封禁 (${region})` : `${platName} 封禁`
        tone = 'error'
      } else if (verdict === 'abnormal') {
        label = region ? `${platName} 异常 (${region})` : `${platName} 异常`
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = `${platName} 未执行`
        tone = 'info'
      } else if (verdict === 'available') {
        label = region ? `${platName} (${region})` : platName
        tone = 'success'
      } else if (verdict === 'restricted') {
        label = region ? `${platName} 受限 (${region})` : `${platName} 受限`
        tone = 'warning'
      } else if (verdict === 'error') {
        label = region ? `${platName} 失败 (${region})` : `${platName} 失败`
        tone = 'error'
      } else {
        label = region ? `${platName} (${region})` : platName
        tone = 'info'
      }
      break
    }
  }

  // Stale handling: append status and set tone to warning
  if (isStale) {
    label = `${label} · 已过期`
    tone = 'warning'
  }

  // Non-available states must never be success/green
  if (
    (verdict === 'unknown' ||
      verdict === 'error' ||
      verdict === 'abnormal' ||
      verdict === 'restricted' ||
      verdict === 'skipped') &&
    tone === 'success'
  ) {
    tone = verdict === 'abnormal' || verdict === 'restricted' ? 'warning' : verdict === 'error' ? 'error' : 'info'
  }

  return {
    platform: normPlat,
    verdict,
    label,
    tone,
    region,
    subTier,
    tooltip,
    throughput: cap.throughput,
    riskScore: cap.risk_score,
  }
}

/** Returns semantic latency class for styling */
export function latencyTone(ms: number | null | undefined): 'success' | 'warning' | 'error' | 'neutral' {
  if (ms == null || ms < 0) return 'neutral'
  if (ms < 100) return 'success'
  if (ms <= 250) return 'warning'
  return 'error'
}

/** Human-readable latency label */
export function latencyLabel(ms: number | null | undefined): string {
  if (ms == null || ms < 0) return '未测速'
  if (ms === 0) return '0 ms'
  return `${ms} ms`
}

export interface ProbeSchedule {
  enabled: boolean
  interval_seconds: number
  kinds: ProbeKind[]
  next_due_at?: string | null
  generation: number
  updated_at: string
}

export type ProbeBatchState = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'expired'

export interface ProbeBatchCounts {
  total_nodes: number
  dispatched_runs: number
  completed_runs: number
  skipped_nodes: number
}

export interface ProbeBatch {
  id: string
  window_at: string
  generation: number
  owner: string
  lease_until?: string | null
  state: ProbeBatchState
  run_ids: string[]
  counts: ProbeBatchCounts
  redacted_error?: string
  created_at: string
  updated_at: string
}

export function probeBatchStateTone(state: ProbeBatchState): 'info' | 'success' | 'warning' | 'error' {
  switch (state) {
    case 'pending':
      return 'warning'
    case 'running':
      return 'info'
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'error'
    case 'cancelled':
    case 'expired':
    default:
      return 'info'
  }
}

export interface ProbePoolStatus {
  queue_nodes_count: number
  probing_count: number
  queued_waiting_count: number
  untested_count: number
  undetermined_count: number
  total_count: number
  unavailable_count: number
  available_count: number
  healthy_count: number
  degraded_count: number
  probing_node_ids: string[]
  queued_node_ids: string[]
  updated_at: string
  batch_id?: string
  batch_state?: ProbeBatchState
  dispatched_runs?: number
  scheduled_nodes?: number
  scheduled_tasks?: number
  skipped_nodes?: number
  no_due_tasks?: boolean
  run_ids?: string[]
  inventory_total?: number
  candidate_total?: number
  scope?: string
}

export type ScheduleTriggerResult = ProbePoolStatus

export function generateIdempotencyKey(): string {
  const entropy = Math.random().toString(36).slice(2, 10)
  return `probe-run-${Date.now().toString(36)}-${entropy}`
}

/** Human-readable interval label */
export function intervalLabel(seconds: number): string {
  if (seconds < 60) return `${seconds} 秒`
  const mins = Math.round(seconds / 60)
  if (mins < 60) return `每 ${mins} 分钟`
  const hrs = Math.round(mins / 60)
  if (hrs < 24) return `每 ${hrs} 小时`
  const days = Math.round(hrs / 24)
  return `每 ${days} 天`
}

/** Parse redacted_summary key=value into structured object */
export interface ParsedSummary {
  profile?: string
  version?: string
  verdict?: string
  reason?: string
  reasonLabel: string
  statusCode?: number
  latencyMs?: number
  error?: string
  raw: string
}

const REASON_LABELS: Record<string, string> = {
  contract_matched: '协议握手与响应校验通过',
  region_mismatch: '服务观测地区与出口参考地区不一致（地区异常）',
  support_unverified: '地区支持状态待核验（未在已知官方支持列表确认）',
  antigravity_entry_reachable: '官网入口可达（个人账号及模型资格未核验）',
  not_executed: '未执行出站探测',
  no_platforms_selected: '未选择探测平台',
  evidence_inconclusive: '探测证据不足 / 结果未确定',
  partial_or_abnormal: '存在部分或异常观测结果',
  detection_failed: '探测执行失败',
  rate_limited: '触发请求频度限制 (429)',
  overflow: '响应体积超出探测安全上限',
  cancelled: '探测任务已取消',
  budget_not_executed: '超出运行预算未执行',
  skipped: '跳过执行',
  control_skip: '控制跳过',
  unsafe_tls_rejected: '安全策略拒绝：禁用跳过证书校验 (skip_cert_verify)',
  unsafe_option_rejected: '安全策略拒绝：包含不安全传输或协议选项',
  private_target_rejected: '安全策略拒绝：私有或保留目标地址',
  target_unresolvable: '节点入口域名无法解析为公网 IP',
  credentials_unavailable: '探针凭据或必要连接参数缺失（未执行出站检测）',
  probe_dialing_not_configured: '探针拨号服务未配置',
  client_build_failed: '探针客户端构建失败（协议参数暂不兼容，未执行出站检测）',
  request_build_failed: '探测请求构建失败',
  node_connect_failed: '节点连接或代理握手失败（已证实不可达）',
  transport_error: '网络连接或握手异常（阶段未确权，待核验）',
  timeout: '探测连接或响应超时',
  dial_timeout: '连接超时',
  dns_error: 'DNS 解析失败',
  dns_resolution_failed: 'DNS 解析失败',
  tls_handshake_failed: 'TLS 握手失败',
  access_restricted: '目标服务访问受限 / 触发验证',
  missing_exit_identity: '未能识别出口 IP 身份',
  speed_opt_in_required: '带宽测速需要显式勾选',
  speed_budget_exceeded: '超出单次测速流量上限',
  unexpected_status: '探测响应状态码不符合预期',
  contract_drift: '探测响应内容不符合契约',
}

const POLICY_OR_CONFIG_BLOCKED_REASONS = new Set<string>([
  'unsafe_tls_rejected',
  'unsafe_option_rejected',
  'private_target_rejected',
  'credentials_unavailable',
  'probe_dialing_not_configured',
  'client_build_failed',
  'request_build_failed',
])

export function isPolicyOrConfigBlockedReason(reason?: string): boolean {
  if (!reason) return false
  return POLICY_OR_CONFIG_BLOCKED_REASONS.has(reason.trim())
}

export function formatObservationReasonLabel(reason?: string): string {
  if (!reason) return ''
  return REASON_LABELS[reason] || reason
}

export function formatAttemptCategory(category?: string): string {
  if (!category) return ''
  const catMap: Record<string, string> = {
    control_skip: '控制跳过',
    budget_exceeded: '超出预算',
    policy_rejected: '策略拒绝',
    network_error: '网络故障',
    protocol_error: '协议错误',
    detection_failed: '检测失败',
    contract_mismatch: '契约不符',
  }
  return catMap[category] || category
}

export function formatAttemptReason(reason?: string): string {
  return formatObservationReasonLabel(reason)
}

export function parseRedactedSummary(summary: string): ParsedSummary {
  const result: ParsedSummary = { reasonLabel: '', raw: summary }
  if (!summary) return result
  const pairs = summary.match(/(\w+)=(\S+)/g) || []
  for (const pair of pairs) {
    const eqIdx = pair.indexOf('=')
    const key = pair.substring(0, eqIdx)
    const val = pair.substring(eqIdx + 1)
    switch (key) {
      case 'profile':
        result.profile = val
        break
      case 'version':
        result.version = val
        break
      case 'verdict':
        result.verdict = val
        break
      case 'reason':
        result.reason = val
        result.reasonLabel = REASON_LABELS[val] || val
        break
      case 'status':
        result.statusCode = parseInt(val, 10) || undefined
        break
      case 'latency_ms':
        result.latencyMs = parseInt(val, 10) || undefined
        break
      case 'error':
        result.error = val
        break
    }
  }
  if (!result.reasonLabel && result.reason) {
    result.reasonLabel = result.reason
  }
  return result
}

/** Format relative time from ISO string */
export function formatRelativeTime(iso: string | null | undefined): string {
  if (!iso) return '未测速'
  try {
    const d = new Date(iso)
    const now = Date.now()
    const diffMs = now - d.getTime()
    if (diffMs < 0) return '即将'
    if (diffMs < 60_000) return '刚刚'
    const mins = Math.floor(diffMs / 60_000)
    if (mins < 60) return `${mins} 分钟前`
    const hrs = Math.floor(mins / 60)
    if (hrs < 24) return `${hrs} 小时前`
    const days = Math.floor(hrs / 24)
    return `${days} 天前`
  } catch {
    return iso
  }
}

export const SCHEDULE_PRESETS = [
  { label: '每 15 分钟', seconds: 900 },
  { label: '每 30 分钟', seconds: 1800 },
  { label: '每 1 小时', seconds: 3600 },
  { label: '每 6 小时', seconds: 21600 },
  { label: '每 12 小时', seconds: 43200 },
  { label: '每 24 小时', seconds: 86400 },
]

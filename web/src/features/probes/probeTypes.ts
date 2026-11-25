import { uiText } from '../../locales'

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

export type EvidenceScope = 'network_entry' | 'web_region' | 'cdn_trace' | 'anonymous_preflight' | string

export interface PlatformCheck {
  verdict: ProbeVerdict
  reason?: string
  status_code?: number
  evidence_scope?: EvidenceScope
}

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
  checks?: Record<string, PlatformCheck>
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
  manual_speed?: boolean
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
    case 'baseline': return uiText('基础连通性', 'Connectivity')
    case 'geo': return uiText('地域与出口 IP', 'Region and exit IP')
    case 'streaming': return uiText('流媒体解锁', 'Streaming access')
    case 'ai': return uiText('AI 服务', 'AI services')
    case 'speed': return uiText('带宽测速', 'Bandwidth test')
    case 'ip_risk': return uiText('IP 风险', 'IP risk')
    default: return kind
  }
}

export function probeKindEmoji(kind: ProbeKind | string): string {
  const found = ALL_PROBE_KINDS.find((k) => k.kind === kind)
  return found?.emoji ?? '🔍'
}

export function probeStateLabel(state: ProbeRunState | ProbeBatchState | string): string {
  switch (state) {
    case 'pending': return uiText('等待中', 'Pending')
    case 'queued': return uiText('排队中', 'Queued')
    case 'running': return uiText('运行中', 'Running')
    case 'succeeded': return uiText('已完成', 'Completed')
    case 'failed': return uiText('已失败', 'Failed')
    case 'cancelled': return uiText('已取消', 'Cancelled')
    case 'expired': return uiText('已过期', 'Expired')
    default: return state
  }
}

export function probeVerdictLabel(verdict: ProbeVerdict | string): string {
  switch (verdict) {
    case 'available': return uiText('可用', 'Available')
    case 'abnormal': return uiText('异常', 'Conflicting evidence')
    case 'restricted': return uiText('降级/受限', 'Restricted')
    case 'unknown': return uiText('未知', 'Unknown')
    case 'error': return uiText('检测失败', 'Probe failed')
    case 'skipped': return uiText('未执行', 'Not executed')
    case 'stale': return uiText('已过期', 'Stale')
    default: return verdict
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
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
    const rawChecks = isRecord(v.checks) ? v.checks : undefined
    const validCheckVerdicts: ProbeVerdict[] = [
      'available', 'abnormal', 'restricted', 'unknown', 'error', 'skipped', 'stale',
    ]
    const checks = rawChecks
      ? Object.entries(rawChecks).reduce<Record<string, PlatformCheck>>((normalized, [checkName, rawCheck]) => {
          if (!isRecord(rawCheck)) return normalized
          const check = rawCheck

          const rawVerdict = typeof check.verdict === 'string' ? check.verdict.toLowerCase().trim() : ''
          const verdict = validCheckVerdicts.find((candidate) => candidate === rawVerdict) ?? 'unknown'
          const statusCode = check.status_code ?? check.statusCode
          const evidenceScope = check.evidence_scope ?? check.evidenceScope

          normalized[checkName] = {
            verdict,
            reason: typeof check.reason === 'string' ? check.reason : undefined,
            status_code: typeof statusCode === 'number' ? statusCode : undefined,
            evidence_scope: typeof evidenceScope === 'string' ? evidenceScope.trim() || undefined : undefined,
          }
          return normalized
        }, {})
      : undefined

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
      checks,
    }
  }
  return Object.keys(result).length > 0 ? result : undefined
}

export function buildPlatformTooltip(platform: string, cap: PlatformCapability): string {
  const name = platform.trim().toLowerCase()
  const parts: string[] = []
  if (name === 'antigravity') {
    const reachable = cap.verdict === 'available' && cap.sub_tier === 'reachable' &&
      cap.evidence_scope === 'network_entry' && cap.reason === 'antigravity_entry_reachable'
    parts.push(reachable ? uiText('官网入口可达', 'Official site entry reachable') : uiText('检测范围：官网入口连通性', 'Scope: official site entry connectivity'))
    parts.push(uiText('个人账号所在国家/地区、年龄(18+)及模型资格未核验（依据官方 FAQ）', 'Account country/region, age (18+), and model eligibility are not verified (official FAQ)'))
  } else if (name === 'gemini') {
    parts.push(uiText('网页端区域支持检测；个人账号登录、年龄及模型权限未核验', 'Web region support only; account login, age, and model permissions are not verified'))
  } else if (name === 'openai') {
    parts.push(uiText('匿名公开入口预检；账号登录、付费模型、Codex 能力及 App 能力未核验；API 鉴权、API 调用和模型推理未检测', 'Anonymous public-entry preflight only; account login, paid models, Codex and App access are unverified; API authentication, API calls and model inference are not tested'))
    if (['full', 'web', 'app'].includes(cap.sub_tier ?? '')) {
      parts.push(uiText('旧版入口分级仅作历史记录，不代表 ChatGPT/API/Codex 能力', 'Legacy entry tiers are historical only and do not establish ChatGPT/API/Codex access'))
    }
    if (cap.sub_tier === 'banned') parts.push(uiText('旧版封禁标签不代表账号封禁', 'Legacy blocked labels do not establish an account ban'))
  } else if (name === 'netflix') {
    if (cap.sub_tier === 'full') parts.push(uiText('Netflix 全量剧集解锁', 'Netflix full catalog observed'))
    if (cap.sub_tier === 'originals') parts.push(uiText('Netflix 仅自制剧', 'Netflix originals only'))
    if (cap.sub_tier === 'banned') parts.push(uiText('Netflix 已封禁', 'Netflix entry blocked'))
  } else if (name === 'disney') {
    if (cap.sub_tier === 'unlocked') parts.push(uiText('Disney+ 已解锁', 'Disney+ access observed'))
    if (cap.sub_tier === 'soon') parts.push(uiText('Disney+ 尚未开放', 'Disney+ not yet available'))
    if (cap.sub_tier === 'banned') parts.push(uiText('Disney+ 已封禁', 'Disney+ entry blocked'))
  }

  // 每个平台的子检查都进入可见正文，不把证据限制留在悬浮提示中
  for (const [key, check] of Object.entries(cap.checks ?? {})) {
    const checkName = key === 'cookie_requirements' ? uiText('合规入口', 'Compliance entry')
      : key === 'chatgpt_web' ? uiText('网页入口', 'Web entry') : key
    const checkVerdict = check.verdict === 'available' ? uiText('已确认', 'Confirmed') : probeVerdictLabel(check.verdict)
    const values = [`${checkName}: ${checkVerdict}`]
    if (check.status_code !== undefined) values.push(`HTTP ${check.status_code}`)
    if (check.reason) values.push(formatObservationReasonLabel(check.reason))
    if (check.evidence_scope) values.push(`${uiText('范围', 'Scope')} ${check.evidence_scope}`)
    parts.push(values.join(' · '))
  }
  const reason = formatObservationReasonLabel(cap.reason)
  if (reason && !parts.some((part) => part.includes(reason))) parts.push(reason)
  if (cap.summary && cap.summary !== reason && !parts.some((part) => part.includes(cap.summary!))) parts.push(cap.summary)

  const serviceRegion = cap.region && cap.region.toLowerCase() !== 'unknown' ? cap.region.toUpperCase() : undefined
  const exitRegion = cap.exit_region && cap.exit_region.toLowerCase() !== 'unknown' ? cap.exit_region.toUpperCase() : undefined
  if (serviceRegion) parts.push(`${uiText('服务观测地区', 'Observed service region')}: ${serviceRegion}`)
  if (exitRegion) parts.push(`${uiText('出口参考地区', 'Reference exit region')}: ${exitRegion}`)
  if (serviceRegion && exitRegion && serviceRegion !== exitRegion) {
    parts.push(uiText('地区存在差异，非物理路由绝对证明', 'Regions differ; this is not definitive proof of physical routing'))
  }
  if (cap.region_source) {
    const sourceLabels: Record<string, string> = {
      service: uiText('服务直测 (service)', 'Direct service observation (service)'),
      cdn_trace: uiText('CDN 追踪 (cdn_trace)', 'CDN trace (cdn_trace)'),
      cdn_target: uiText('CDN 目标节点 (cdn_target)', 'CDN target (cdn_target)'),
    }
    parts.push(`${uiText('地区来源', 'Region source')}: ${sourceLabels[cap.region_source] || cap.region_source}`)
  }
  if (cap.evidence_scope) {
    const scopeLabels: Record<string, string> = {
      network_entry: uiText('网站入口连通性 (network_entry)', 'Site entry connectivity (network_entry)'),
      web_region: uiText('网页端区域支持 (web_region)', 'Web region support (web_region)'),
      cdn_trace: uiText('CDN 节点出口 (cdn_trace)', 'CDN exit (cdn_trace)'),
      anonymous_preflight: uiText('匿名公开入口预检 (anonymous_preflight)', 'Anonymous entry preflight (anonymous_preflight)'),
    }
    parts.push(`${uiText('观测范围', 'Scope')}: ${scopeLabels[cap.evidence_scope] || cap.evidence_scope}`)
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
      if (isStale) {
        label = 'OpenAI 结果过期'
        tone = 'warning'
      } else if (verdict === 'skipped') {
        label = 'OpenAI 未执行'
        tone = 'info'
      } else if (verdict === 'abnormal') {
        label = 'OpenAI 预检证据冲突'
        tone = 'warning'
      } else if (verdict === 'restricted' && cap.reason === 'rate_limited') {
        label = 'OpenAI 入口限流'
        tone = 'info'
      } else if (verdict === 'restricted') {
        label = 'OpenAI 入口受限'
        tone = 'warning'
      } else if (verdict === 'error') {
        label = 'OpenAI 检测失败'
        tone = 'error'
      } else if (verdict === 'available' && cap.evidence_scope === 'anonymous_preflight') {
        label = 'OpenAI 预检通过'
        tone = 'success'
      } else if (verdict === 'available') {
        label = '旧版探测结果，需重新检测'
        tone = 'info'
      } else if (verdict === 'unknown' && cap.reason === 'partial_preflight') {
        label = 'OpenAI 预检部分证据'
        tone = 'info'
      } else if (verdict === 'unknown' && cap.reason === 'endpoint_reachable_capability_unverified') {
        label = 'OpenAI 入口响应（能力未核验）'
        tone = 'info'
      } else {
        label = 'OpenAI 能力未核验'
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

  const openAILabels: Record<string, string> = {
    'OpenAI 结果过期': 'OpenAI results stale',
    'OpenAI 未执行': 'OpenAI not executed',
    'OpenAI 预检证据冲突': 'OpenAI preflight evidence conflicts',
    'OpenAI 入口限流': 'OpenAI entry rate limited',
    'OpenAI 入口受限': 'OpenAI entry restricted',
    'OpenAI 检测失败': 'OpenAI probe failed',
    'OpenAI 预检通过': 'OpenAI preflight passed',
    '旧版探测结果，需重新检测': 'Legacy probe result; test again',
    'OpenAI 预检部分证据': 'OpenAI partial preflight evidence',
    'OpenAI 入口响应（能力未核验）': 'OpenAI entry responded (capability unverified)',
    'OpenAI 能力未核验': 'OpenAI capability unverified',
  }
  if (normPlat === 'openai') {
    label = uiText(label, openAILabels[label] ?? label)
  } else {
    const name = ({ netflix: 'NF', disney: 'D+', youtube: 'YT', claude: 'Claude', gemini: 'Gemini', antigravity: 'Antigravity' } as Record<string, string>)[normPlat] ?? platform.toUpperCase()
    let status = ''
    if (subTier === 'banned') status = 'blocked'
    else if (verdict === 'abnormal') status = 'conflicting evidence'
    else if (verdict === 'skipped') status = 'not executed'
    else if (normPlat === 'netflix' && subTier === 'originals') status = 'originals only'
    else if (normPlat === 'disney' && subTier === 'soon') status = 'not yet available'
    else if (verdict === 'restricted') status = 'restricted'
    else if (verdict === 'error') status = 'failed'
    else if (normPlat === 'antigravity' && verdict === 'available') status = 'entry reachable'
    label = uiText(label, `${name}${status ? ` ${status}` : ''}${region ? ` (${region})` : ''}`)
  }

  // Stale handling: append status and set tone to warning
  if (isStale) {
    label = `${label} · ${uiText('已过期', 'Stale')}`
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
  endpoint_reachable_capability_unverified: '入口有响应，但服务能力未核验',
}

const EN_REASON_LABELS: Record<string, string> = {
  contract_matched: 'Protocol handshake and response verified',
  region_mismatch: 'Observed service region differs from the reference exit region',
  support_unverified: 'Region support not verified against the known official list',
  antigravity_entry_reachable: 'Official site entry reachable; account and model eligibility unverified',
  not_executed: 'No outbound probe executed',
  no_platforms_selected: 'No probe platforms selected',
  evidence_inconclusive: 'Insufficient or inconclusive evidence',
  partial_or_abnormal: 'Partial or conflicting observations',
  detection_failed: 'Probe execution failed',
  rate_limited: 'Rate limited (429)',
  overflow: 'Response exceeds the probe safety limit',
  cancelled: 'Probe run cancelled',
  budget_not_executed: 'Not executed: run budget exceeded',
  skipped: 'Execution skipped',
  control_skip: 'Skipped by control policy',
  unsafe_tls_rejected: 'Security policy rejected skip_cert_verify',
  unsafe_option_rejected: 'Security policy rejected unsafe transport or protocol options',
  private_target_rejected: 'Security policy rejected a private or reserved target address',
  target_unresolvable: 'Node entry domain could not resolve to a public IP',
  credentials_unavailable: 'Missing probe credentials or connection parameters; no outbound attempt',
  probe_dialing_not_configured: 'Probe dialing service is not configured',
  client_build_failed: 'Probe client could not be created; no outbound attempt',
  request_build_failed: 'Could not construct the probe request',
  node_connect_failed: 'Node connection or proxy handshake failed; confirmed unreachable',
  transport_error: 'Connection or handshake error; failure stage unverified',
  timeout: 'Probe connection or response timed out',
  dial_timeout: 'Connection timed out',
  dns_error: 'DNS resolution failed',
  dns_resolution_failed: 'DNS resolution failed',
  tls_handshake_failed: 'TLS handshake failed',
  access_restricted: 'Service access restricted or verification required',
  missing_exit_identity: 'Exit IP identity could not be established',
  speed_opt_in_required: 'Bandwidth testing requires explicit opt-in',
  speed_budget_exceeded: 'Per-test bandwidth budget exceeded',
  unexpected_status: 'Unexpected probe response status',
  contract_drift: 'Response content does not match the probe contract',
  endpoint_reachable_capability_unverified: 'Entry responded; service capability unverified',
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
  return uiText(REASON_LABELS[reason] || reason, EN_REASON_LABELS[reason] || reason)
}

export function formatAttemptCategory(category?: string): string {
  if (!category) return ''
  const catMap: Record<string, string> = {
    control_skip: uiText('控制跳过', 'Control skip'),
    budget_exceeded: uiText('超出预算', 'Budget exceeded'),
    policy_rejected: uiText('策略拒绝', 'Policy rejected'),
    network_error: uiText('网络故障', 'Network error'),
    protocol_error: uiText('协议错误', 'Protocol error'),
    detection_failed: uiText('检测失败', 'Probe failed'),
    contract_mismatch: uiText('契约不符', 'Contract mismatch'),
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
        result.reasonLabel = formatObservationReasonLabel(val)
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
  if (!iso) return uiText('未检测', 'Not probed')
  try {
    const d = new Date(iso)
    const now = Date.now()
    const diffMs = now - d.getTime()
    if (diffMs < 0) return uiText('即将', 'Soon')
    if (diffMs < 60_000) return uiText('刚刚', 'Just now')
    const mins = Math.floor(diffMs / 60_000)
    if (mins < 60) return uiText(`${mins} 分钟前`, `${mins} min ago`)
    const hrs = Math.floor(mins / 60)
    if (hrs < 24) return uiText(`${hrs} 小时前`, `${hrs} h ago`)
    const days = Math.floor(hrs / 24)
    return uiText(`${days} 天前`, `${days} d ago`)
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

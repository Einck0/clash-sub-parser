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
  | 'restricted'
  | 'unknown'
  | 'error'
  | 'stale'

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
    case 'restricted':
      return '降级/受限'
    case 'unknown':
      return '未知'
    case 'error':
      return '不可达'
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
    case 'restricted':
      return 'warning'
    case 'error':
      return 'error'
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
  transport_error: '网络连接或握手失败',
  access_restricted: '目标服务访问受限 / 触发验证',
  missing_exit_identity: '未能识别出口 IP 身份',
  dial_timeout: '连接超时',
  speed_opt_in_required: '带宽测速需要显式勾选',
  tls_handshake_failed: 'TLS 握手失败',
  dns_resolution_failed: 'DNS 解析失败',
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

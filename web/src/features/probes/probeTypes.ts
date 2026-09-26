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

export const ALL_PROBE_KINDS: Array<{ kind: ProbeKind; label: string; desc: string }> = [
  { kind: 'baseline', label: '基础连通性 (Baseline)', desc: 'TCP、TLS 握手与基础连通性检测' },
  { kind: 'geo', label: '地域与出口 IP (Geo)', desc: '出口 IP、ISO 国家/地区代码与 ASN 识别' },
  { kind: 'streaming', label: '流媒体解锁 (Streaming)', desc: 'Netflix、YouTube 与哔哩哔哩解锁检测' },
  { kind: 'ai', label: 'AI 服务可用性 (AI)', desc: 'OpenAI、Gemini 与 Claude 可访问性检测' },
  { kind: 'speed', label: '速度与带宽 (Speed)', desc: '延迟与吞吐带宽基准测试' },
  { kind: 'ip_risk', label: 'IP 风险与欺诈分 (IP Risk)', desc: 'Scamalytics、IPQS 与 IP-API 风险评分' },
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

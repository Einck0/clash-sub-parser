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
  { kind: 'baseline', label: 'Baseline', desc: 'TCP, TLS handshakes and connectivity' },
  { kind: 'geo', label: 'Geo & Outbound IP', desc: 'Exit IP, ISO country code & ASN' },
  { kind: 'streaming', label: 'Streaming', desc: 'Netflix, YouTube & Bilibili unblocking' },
  { kind: 'ai', label: 'AI Services', desc: 'OpenAI, Gemini & Claude accessibility' },
  { kind: 'speed', label: 'Speed & Bandwidth', desc: 'Latency and throughput benchmarks' },
  { kind: 'ip_risk', label: 'IP Risk & Fraud', desc: 'Scamalytics, IPQS & IP-API risk score' },
]

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

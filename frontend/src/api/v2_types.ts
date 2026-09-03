export interface NodeItemV2 {
  logical_id: string
  name: string
  protocol: string
  server: string
  port: number
  lifecycle_state: string
  payload_fingerprint?: string
  normalized_payload?: Record<string, any>
  created_at?: string
  updated_at?: string
}

export interface SourceItemV2 {
  logical_id: string
  name: string
  kind: 'subscription' | 'manual'
  url?: string
  update_interval?: number
  enabled: boolean
  last_fetched_at?: string
  fetch_status?: string
  fetch_error?: string
  node_count?: number
}

export interface InventoryNodesResponse {
  total: number
  limit: number
  offset: number
  items: NodeItemV2[]
}

export interface MembershipEntryV2 {
  entry_type: 'source' | 'node' | 'group' | 'regex' | 'probe_filter' | 'all'
  value?: string
  action?: 'include' | 'exclude'
  filter_params?: Record<string, any>
}

export interface LogicalGroupConfigV2 {
  logical_id: string
  name: string
  group_type: string
  use_node_names?: string[]
  include_entries?: MembershipEntryV2[]
  exclude_entries?: MembershipEntryV2[]
  url?: string
  interval?: number
  tolerance?: number
  lazy?: boolean
  disable_udp?: boolean
  strategy?: string
}

export interface LogicalConfigurationBundleV2 {
  schema_version: number
  logical_id?: string
  name?: string
  groups: LogicalGroupConfigV2[]
  rule_categories?: any[]
  rules?: any[]
  dns?: Record<string, any>
  generate_settings?: Record<string, any>
}

export interface PreflightDiagnosticV2 {
  rule_id: string
  severity: 'error' | 'warning' | 'info'
  message: string
  entity_type?: string
  entity_id?: string
}

export interface BundlePreflightResultV2 {
  valid: boolean
  schema_valid: boolean
  secret_leak_free: boolean
  graph_valid: boolean
  blockers: PreflightDiagnosticV2[]
  warnings: PreflightDiagnosticV2[]
  summary?: Record<string, any>
}

export interface RevisionItemV2 {
  logical_id: string
  parent_revision_id?: string | null
  bundle_checksum: string
  author: string
  change_summary: string
  is_active: boolean
  created_at: string
}

export interface CompilationResultV2 {
  success: boolean
  semantic_fingerprint?: string
  diagnostics: PreflightDiagnosticV2[]
  bundle_summary?: Record<string, any>
}

export interface ProbeProfileV2 {
  logical_id: string
  name: string
  probe_type: string
  target_url?: string
  timeout_ms: number
  concurrency: number
  interval_seconds?: number
  config_payload?: Record<string, any>
  is_default: boolean
}

export interface ProbeJobV2 {
  logical_id: string
  profile_id?: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  node_count: number
  completed_count: number
  error_message?: string | null
  started_at?: string | null
  completed_at?: string | null
}

export interface ProbeObservationV2 {
  logical_id: string
  node_id: string
  job_id?: string
  probe_type: string
  status: 'ok' | 'fail' | 'timeout' | 'skipped' | 'unknown'
  latency_ms?: number | null
  speed_mbps?: number | null
  ip?: string | null
  country?: string | null
  asn?: number | null
  organization?: string | null
  media_unlock?: Record<string, any>
  diagnostics_redacted?: string | null
  observed_at: string
}

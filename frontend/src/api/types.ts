export interface SubscriptionItem {
  id: number
  name: string
  url?: string
  enabled: boolean
  is_primary?: boolean
  update_interval?: number
  last_fetched_at?: string
  fetch_status?: string
  fetch_error?: string
  fetch_failed_count?: number
  raw_nodes?: any[]
  selected_nodes?: any[]
  filter_regex?: string[]
  filter_min_speed_mbps?: number | null
  filter_media_unlock?: string[]
  include_node_names?: string[]
  exclude_node_names?: string[]
  node_renames?: Record<string, string>
}

export interface NodeGroupItem {
  id: number
  name: string
  type: string
  proxies?: string[]
  filter_regex?: string[]
  filter_min_speed_mbps?: number | null
  filter_media_unlock?: string[]
  use?: string[]
  url?: string
  interval?: number
  tolerance?: number
  lazy?: boolean
  disable_udp?: boolean
}

export interface RuleItem {
  id: number
  category_id?: number
  type: string
  payload: string
  proxy: string
  no_resolve?: boolean
  priority?: number
}

export interface RuleCategoryItem {
  id: number
  name: string
  description?: string
  priority?: number
  rules?: RuleItem[]
}

export interface ProxyChainItem {
  id: number
  name: string
  enabled: boolean
  underlying_proxy: string
  target_proxy: string
}

export interface GenerateConfig {
  enabled: boolean
  subscriptions: boolean
  node_groups: boolean
  rules: boolean
  dns: boolean
  exclude_node_proxies: boolean
}

export interface ProbeSettings {
  probe_enabled: boolean
  probe_interval_minutes: number
  speedtest_enabled: boolean
  speedtest_url: string
  speedtest_timeout_s: number
  speedtest_max_bytes: number
  speedtest_min_speed_mbps: number
  media_check_enabled: boolean
  media_platforms: string[]
  media_timeout_s: number
  probe_concurrency: number
  probe_timeout_ms: number
}

export interface ProbeResult {
  name: string
  server?: string
  port?: number
  type?: string
  status: 'ok' | 'fail' | 'timeout' | 'skipped' | 'unknown'
  latency_ms?: number | null
  speed_mbps?: number | null
  ip?: string | null
  country?: string | null
  asn?: number | null
  organization?: string | null
  media?: Record<string, any>
  error?: string | null
  checked_at?: number
}

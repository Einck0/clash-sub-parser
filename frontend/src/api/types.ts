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

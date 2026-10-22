export interface DnsSettings {
  enabled: boolean
  raw_yaml: string
  revision: number
  updated_at: string | null
  source: 'default' | 'saved'
  config: Record<string, unknown>
}

export interface ValidatedDns {
  raw_yaml: string
  config: Record<string, unknown>
}

/**
 * 17 well-known DNS configuration keys supported by Clash / Mihomo:
 * 1.  enable (outer & inner)
 * 2.  ipv6
 * 3.  enhanced-mode
 * 4.  listen
 * 5.  fake-ip-range
 * 6.  cache-algorithm
 * 7.  use-hosts
 * 8.  use-system-hosts
 * 9.  respect-rules
 * 10. nameserver
 * 11. default-nameserver
 * 12. fallback
 * 13. proxy-server-nameserver
 * 14. direct-nameserver
 * 15. fake-ip-filter
 * 16. nameserver-policy
 * 17. fallback-filter
 */
export interface DnsConfigKnownFields {
  enable?: boolean
  ipv6?: boolean
  'enhanced-mode'?: string
  listen?: string
  'fake-ip-range'?: string
  'cache-algorithm'?: string
  'use-hosts'?: boolean
  'use-system-hosts'?: boolean
  'respect-rules'?: boolean

  nameserver?: string[]
  'default-nameserver'?: string[]
  fallback?: string[]
  'proxy-server-nameserver'?: string[]
  'direct-nameserver'?: string[]
  'fake-ip-filter'?: string[]

  'nameserver-policy'?: Record<string, unknown>
  'fallback-filter'?: Record<string, unknown>
}

export type DnsConfig = DnsConfigKnownFields & Record<string, unknown>

export const DNS_LIST_KEYS = [
  'nameserver',
  'default-nameserver',
  'fallback',
  'proxy-server-nameserver',
  'direct-nameserver',
  'fake-ip-filter',
] as const

export type DnsListKey = (typeof DNS_LIST_KEYS)[number]

export const DNS_JSON_KEYS = [
  'nameserver-policy',
  'fallback-filter',
] as const

export type DnsJsonKey = (typeof DNS_JSON_KEYS)[number]

export const DNS_BASIC_SCALAR_KEYS = [
  'enhanced-mode',
  'listen',
  'fake-ip-range',
  'cache-algorithm',
] as const

export const DNS_BASIC_BOOLEAN_KEYS = [
  'ipv6',
  'use-hosts',
  'use-system-hosts',
  'respect-rules',
] as const

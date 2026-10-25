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
 * 20 well-known DNS configuration keys supported by Clash / Mihomo:
 * 1.  enable (outer & inner)
 * 2.  ipv6
 * 3.  enhanced-mode
 * 4.  listen
 * 5.  fake-ip-range
 * 6.  cache-algorithm
 * 7.  use-hosts
 * 8.  use-system-hosts
 * 9.  respect-rules
 * 10. prefer-h3
 * 11. direct-nameserver-follow-policy
 * 12. nameserver
 * 13. default-nameserver
 * 14. fallback
 * 15. proxy-server-nameserver
 * 16. direct-nameserver
 * 17. fake-ip-filter
 * 18. nameserver-policy
 * 19. fallback-filter
 * 20. proxy-server-nameserver-policy
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
  'prefer-h3'?: boolean
  'direct-nameserver-follow-policy'?: boolean

  nameserver?: string[]
  'default-nameserver'?: string[]
  fallback?: string[]
  'proxy-server-nameserver'?: string[]
  'direct-nameserver'?: string[]
  'fake-ip-filter'?: string[]

  'nameserver-policy'?: Record<string, unknown>
  'fallback-filter'?: Record<string, unknown>
  'proxy-server-nameserver-policy'?: Record<string, unknown>
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
  'proxy-server-nameserver-policy',
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

export const DNS_ADVANCED_BOOLEAN_KEYS = [
  'prefer-h3',
  'direct-nameserver-follow-policy',
] as const

export const DNS_BOOLEAN_KEYS = [
  'ipv6',
  'use-hosts',
  'use-system-hosts',
  'respect-rules',
  'prefer-h3',
  'direct-nameserver-follow-policy',
] as const

export const DNS_KNOWN_KEYS = [
  'enable',
  'ipv6',
  'enhanced-mode',
  'listen',
  'fake-ip-range',
  'cache-algorithm',
  'use-hosts',
  'use-system-hosts',
  'respect-rules',
  'prefer-h3',
  'direct-nameserver-follow-policy',
  'nameserver',
  'default-nameserver',
  'fallback',
  'proxy-server-nameserver',
  'direct-nameserver',
  'fake-ip-filter',
  'nameserver-policy',
  'fallback-filter',
  'proxy-server-nameserver-policy',
] as const

export type DnsKnownKey = (typeof DNS_KNOWN_KEYS)[number]

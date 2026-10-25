// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import {
  DNS_BASIC_FIELDS,
  DNS_GROUPS,
  ALL_KNOWN_KEYS,
  getUnknownKeys,
  getBoolDisplayState,
  isFieldConfigured,
  countGroupConfigured,
} from './dnsFields'

describe('dnsFields configuration and helpers', () => {
  it('covers all 19 functional DNS keys plus enable in ALL_KNOWN_KEYS', () => {
    expect(ALL_KNOWN_KEYS).toContain('enable')
    expect(ALL_KNOWN_KEYS).toContain('nameserver')
    expect(ALL_KNOWN_KEYS).toContain('enhanced-mode')
    expect(ALL_KNOWN_KEYS).toContain('ipv6')
    expect(ALL_KNOWN_KEYS).toContain('default-nameserver')
    expect(ALL_KNOWN_KEYS).toContain('proxy-server-nameserver')
    expect(ALL_KNOWN_KEYS).toContain('direct-nameserver')
    expect(ALL_KNOWN_KEYS).toContain('listen')
    expect(ALL_KNOWN_KEYS).toContain('respect-rules')
    expect(ALL_KNOWN_KEYS).toContain('direct-nameserver-follow-policy')
    expect(ALL_KNOWN_KEYS).toContain('prefer-h3')
    expect(ALL_KNOWN_KEYS).toContain('fake-ip-range')
    expect(ALL_KNOWN_KEYS).toContain('fake-ip-filter')
    expect(ALL_KNOWN_KEYS).toContain('cache-algorithm')
    expect(ALL_KNOWN_KEYS).toContain('fallback')
    expect(ALL_KNOWN_KEYS).toContain('nameserver-policy')
    expect(ALL_KNOWN_KEYS).toContain('fallback-filter')
    expect(ALL_KNOWN_KEYS).toContain('proxy-server-nameserver-policy')
    expect(ALL_KNOWN_KEYS).toContain('use-hosts')
    expect(ALL_KNOWN_KEYS).toContain('use-system-hosts')
    expect(ALL_KNOWN_KEYS.length).toBe(20)
  })

  it('correctly partitions basic fields and 4 advanced groups', () => {
    expect(DNS_BASIC_FIELDS.map((f) => f.key)).toEqual(['nameserver', 'enhanced-mode', 'ipv6'])
    expect(DNS_GROUPS.map((g) => g.id)).toEqual([
      'connection-bootstrap',
      'fakeip-cache',
      'fallback-policy',
      'hosts',
    ])
  })

  it('detects unknown / custom fields without discarding them', () => {
    const draft: Record<string, unknown> = {
      nameserver: ['1.1.1.1'],
      'custom-upstream': 'https://custom.dns/query',
      'x-extra-profile': { enabled: true },
    }
    const unknown = getUnknownKeys(draft)
    expect(unknown).toEqual(['custom-upstream', 'x-extra-profile'])
  })

  it('tri-state getBoolDisplayState distinguishes unspecified from false and true', () => {
    expect(getBoolDisplayState({}, 'ipv6')).toBe('unspecified')
    expect(getBoolDisplayState({ ipv6: undefined }, 'ipv6')).toBe('unspecified')
    expect(getBoolDisplayState({ ipv6: null }, 'ipv6')).toBe('unspecified')
    expect(getBoolDisplayState({ ipv6: true }, 'ipv6')).toBe('true')
    expect(getBoolDisplayState({ ipv6: false }, 'ipv6')).toBe('false')
    expect(getBoolDisplayState({ ipv6: 'unexpected-string' }, 'ipv6')).toBe('custom')
  })

  it('accurately counts configured fields in a group', () => {
    const bootstrapGroup = DNS_GROUPS.find((g) => g.id === 'connection-bootstrap')!
    expect(countGroupConfigured({}, bootstrapGroup)).toBe(0)

    const draft: Record<string, unknown> = {
      'default-nameserver': ['114.114.114.114'],
      listen: '0.0.0.0:1053',
      'prefer-h3': true,
      'direct-nameserver': [], // empty list not counted
    }
    expect(countGroupConfigured(draft, bootstrapGroup)).toBe(3)
  })

  it('handles field configuration presence checks for various types', () => {
    expect(isFieldConfigured({ nameserver: ['1.1.1.1'] }, 'nameserver', 'list')).toBe(true)
    expect(isFieldConfigured({ nameserver: [] }, 'nameserver', 'list')).toBe(false)
    expect(isFieldConfigured({ listen: '0.0.0.0:1053' }, 'listen', 'scalar-text')).toBe(true)
    expect(isFieldConfigured({ listen: '   ' }, 'listen', 'scalar-text')).toBe(false)
    expect(isFieldConfigured({ 'cache-algorithm': 'arc' }, 'cache-algorithm', 'scalar-select')).toBe(true)
    expect(isFieldConfigured({ 'cache-algorithm': '' }, 'cache-algorithm', 'scalar-select')).toBe(false)
    expect(isFieldConfigured({ ipv6: false }, 'ipv6', 'bool')).toBe(true)
    expect(isFieldConfigured({}, 'ipv6', 'bool')).toBe(false)
    expect(isFieldConfigured({ 'nameserver-policy': { 'geosite:cn': '223.5.5.5' } }, 'nameserver-policy', 'json')).toBe(true)
    expect(isFieldConfigured({ 'nameserver-policy': {} }, 'nameserver-policy', 'json')).toBe(false)
  })
})

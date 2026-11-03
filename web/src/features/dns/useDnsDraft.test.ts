// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import yaml from 'js-yaml'
import { api } from '../../api/client'
import { useDnsDraft, serializeDraftToYaml } from './useDnsDraft'
import type { DnsSettings } from './dnsTypes'

const createMockSettings = (overrides?: Partial<DnsSettings>): DnsSettings => ({
  enabled: true,
  raw_yaml: 'dns:\n  enable: true\n  nameserver:\n    - 1.1.1.1\n',
  revision: 5,
  updated_at: '2026-10-22T05:00:00Z',
  source: 'saved',
  config: {
    enable: true,
    ipv6: false,
    'enhanced-mode': 'fake-ip',
    listen: '0.0.0.0:1053',
    'fake-ip-range': '198.18.0.1/16',
    'cache-algorithm': 'arc',
    'use-hosts': true,
    'use-system-hosts': false,
    'respect-rules': true,
    'prefer-h3': false,
    'direct-nameserver-follow-policy': false,
    nameserver: ['1.1.1.1', '8.8.8.8'],
    'default-nameserver': ['114.114.114.114'],
    fallback: ['8.8.4.4'],
    'proxy-server-nameserver': ['1.0.0.1'],
    'direct-nameserver': ['223.5.5.5'],
    'fake-ip-filter': ['*.lan', '+.local'],
    'nameserver-policy': { 'geosite:cn': '223.5.5.5' },
    'fallback-filter': { geoip: true, ipcidr: ['240.0.0.0/4'] },
    'proxy-server-nameserver-policy': { 'domain:proxy.lan': '1.0.0.1' },
    // Custom unknown nested keys that must be preserved with zero loss
    custom_extension: 'preserve-me',
    extra_nested: { deep: { key: 42 }, arr: [1, 2, 3] },
  },
  ...overrides,
})

describe('useDnsDraft', () => {
  let mockSettings: DnsSettings

  beforeEach(() => {
    mockSettings = createMockSettings()
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/settings/dns') {
        return { ...mockSettings }
      }
      throw new Error(`Unexpected GET: ${path}`)
    })
    vi.spyOn(api, 'post').mockImplementation(async (path: string, body: any) => {
      if (path === '/api/v1/settings/dns/validate') {
        let parsed: Record<string, unknown> = {}
        try {
          parsed = (yaml.load(body.raw_yaml) as Record<string, unknown>) || {}
        } catch {
          parsed = { enable: body.enabled }
        }
        return {
          raw_yaml: String(body.raw_yaml),
          config: parsed,
        }
      }
      throw new Error(`Unexpected POST: ${path}`)
    })
    vi.spyOn(api, 'put').mockImplementation(async (path: string, body: any) => {
      if (path === '/api/v1/settings/dns') {
        mockSettings = {
          ...mockSettings,
          enabled: body.enabled !== undefined ? body.enabled : mockSettings.enabled,
          raw_yaml: body.raw_yaml,
          revision: mockSettings.revision + 1,
        }
        return { ...mockSettings }
      }
      throw new Error(`Unexpected PUT: ${path}`)
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('initializes and loads settings from server, initially clean', async () => {
    const draft = useDnsDraft()
    expect(draft.settings.value).toBeNull()
    expect(draft.dirty.value).toBe(false)

    await draft.load(true)

    expect(draft.settings.value).not.toBeNull()
    expect(draft.settings.value?.revision).toBe(5)
    expect(draft.enabled.value).toBe(true)
    expect(draft.dirty.value).toBe(false)
    expect(draft.getBool('ipv6')).toBe(false)
    expect(draft.getString('enhanced-mode')).toBe('fake-ip')
    expect(draft.getString('cache-algorithm')).toBe('arc')
    expect(draft.getBool('prefer-h3')).toBe(false)
    expect(draft.getBool('direct-nameserver-follow-policy')).toBe(false)
  })

  it('supports list item operations: add individual, remove individual, paste multiline, and setListText', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // Initial state: ['1.1.1.1', '8.8.8.8']
    expect(draft.getList('nameserver')).toEqual(['1.1.1.1', '8.8.8.8'])

    // 1. Add single item
    draft.addListItem('nameserver', '9.9.9.9')
    expect(draft.getList('nameserver')).toEqual(['1.1.1.1', '8.8.8.8', '9.9.9.9'])
    expect(draft.dirty.value).toBe(true)

    // 2. Remove individual item by index (e.g. index 1: '8.8.8.8')
    draft.removeListItem('nameserver', 1)
    expect(draft.getList('nameserver')).toEqual(['1.1.1.1', '9.9.9.9'])

    // 3. Paste multiline text into list
    draft.pasteListItems('nameserver', '1.0.0.1\n\n  223.5.5.5  \n')
    expect(draft.getList('nameserver')).toEqual(['1.1.1.1', '9.9.9.9', '1.0.0.1', '223.5.5.5'])

    // 4. Batch edit via setListText
    draft.setListText('nameserver', '114.114.114.114\n119.29.29.29')
    expect(draft.getList('nameserver')).toEqual(['114.114.114.114', '119.29.29.29'])
    expect(draft.getListText('nameserver')).toBe('114.114.114.114\n119.29.29.29')

    // Reset back to baseline
    draft.setList('nameserver', ['1.1.1.1', '8.8.8.8'])
    expect(draft.dirty.value).toBe(false)
  })

  it('tracks dirty states across scalar, list, YAML mapping, raw and outer enabled toggles', async () => {
    const draft = useDnsDraft()
    await draft.load(true)
    expect(draft.dirty.value).toBe(false)

    // 1. Scalar change
    draft.setString('listen', '127.0.0.1:5353')
    expect(draft.dirty.value).toBe(true)
    draft.setString('listen', '0.0.0.0:1053')
    expect(draft.dirty.value).toBe(false)

    // 2. Boolean change
    draft.setBool('ipv6', true)
    expect(draft.dirty.value).toBe(true)
    draft.setBool('ipv6', false)
    expect(draft.dirty.value).toBe(false)

    // 3. List change
    draft.setListText('nameserver', '9.9.9.9')
    expect(draft.dirty.value).toBe(true)
    draft.setListText('nameserver', '1.1.1.1\n8.8.8.8')
    expect(draft.dirty.value).toBe(false)

    // 4. YAML mapping change
    draft.setYamlMappingText('nameserver-policy', 'geosite:cn: 119.29.29.29')
    expect(draft.dirty.value).toBe(true)
    draft.setYamlMappingText('nameserver-policy', 'geosite:cn: 223.5.5.5')
    expect(draft.dirty.value).toBe(false)

    // 5. Outer enabled toggle
    draft.enabled.value = false
    expect(draft.dirty.value).toBe(true)
    draft.enabled.value = true
    expect(draft.dirty.value).toBe(false)

    // 6. Raw mode change
    await draft.switchMode('raw')
    draft.onRawInput('different raw yaml')
    expect(draft.dirty.value).toBe(true)
  })

  it('preserves all known keys and custom unknown nested keys in draft with zero loss', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // Verify presence of unknown nested keys and new advanced keys in initial draft
    expect(draft.draft.value['custom_extension']).toBe('preserve-me')
    expect((draft.draft.value['extra_nested'] as any)?.deep?.key).toBe(42)
    expect(draft.draft.value['prefer-h3']).toBe(false)
    expect(draft.draft.value['direct-nameserver-follow-policy']).toBe(false)
    expect(draft.draft.value['proxy-server-nameserver-policy']).toEqual({ 'domain:proxy.lan': '1.0.0.1' })

    // Mutate one of the standard keys
    draft.setListText('nameserver', '1.1.1.1\n1.0.0.1')

    // Confirm custom keys and all other fields were not discarded
    expect(draft.draft.value['custom_extension']).toBe('preserve-me')
    expect((draft.draft.value['extra_nested'] as any)?.deep?.key).toBe(42)
    expect((draft.draft.value['extra_nested'] as any)?.arr).toEqual([1, 2, 3])
    expect(draft.draft.value['cache-algorithm']).toBe('arc')
    expect(draft.draft.value['use-hosts']).toBe(true)
    expect(draft.draft.value['proxy-server-nameserver-policy']).toEqual({ 'domain:proxy.lan': '1.0.0.1' })

    // Serialization to YAML retains custom nested keys
    const serialized = serializeDraftToYaml(draft.draft.value, draft.enabled.value)
    expect(serialized).toContain('custom_extension: preserve-me')
    expect(serialized).toContain('extra_nested:')
  })

  it('handles natural YAML mapping in advanced fields: preserves text, blocks save/switch/preview on invalid YAML', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // 1. Invalid YAML syntax
    const ok = draft.setYamlMappingText('nameserver-policy', 'invalid: [ yaml unclosed')
    expect(ok).toBe(false)
    expect(draft.fieldErrors.value['nameserver-policy']).toContain('YAML 格式无效')
    expect(draft.hasValidationErrors.value).toBe(true)
    expect(draft.getYamlMappingText('nameserver-policy')).toBe('invalid: [ yaml unclosed')
    // draft object itself should NOT contain the invalid input
    expect(draft.draft.value['nameserver-policy']).toEqual({ 'geosite:cn': '223.5.5.5' })

    // Save should be blocked
    await draft.save()
    expect(draft.error.value).toContain('请先修正字段格式错误')
    expect(api.put).not.toHaveBeenCalled()

    // Mode switch should be blocked
    await draft.switchMode('raw')
    expect(draft.error.value).toContain('请先修正字段格式错误')
    expect(draft.mode.value).toBe('visual')

    // Preview should be blocked
    await draft.preview()
    expect(draft.error.value).toContain('请先修正字段格式错误')
    expect(draft.previewYaml.value).toBe('')

    // 2. Duplicate keys rejection in YAML mapping
    const dupOk = draft.setYamlMappingText('nameserver-policy', 'key: 1\nkey: 2')
    expect(dupOk).toBe(false)
    expect(draft.fieldErrors.value['nameserver-policy']).toBeDefined()

    // 3. Fix YAML with natural mapping
    const fixed = draft.setYamlMappingText('nameserver-policy', 'geosite:google:\n  - 8.8.8.8\n  - 8.8.4.4')
    expect(fixed).toBe(true)
    expect(draft.fieldErrors.value['nameserver-policy']).toBeUndefined()
    expect(draft.hasValidationErrors.value).toBe(false)
    expect(draft.draft.value['nameserver-policy']).toEqual({
      'geosite:google': ['8.8.8.8', '8.8.4.4'],
    })
  })

  it('rejects YAML non-mapping inputs: array, null, scalar strings/numbers', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // 1. empty array []
    expect(draft.setYamlMappingText('nameserver-policy', '[]')).toBe(false)
    expect(draft.fieldErrors.value['nameserver-policy']).toContain('必须是有效 YAML 键值映射')
    expect(draft.getYamlMappingText('nameserver-policy')).toBe('[]')

    // 2. YAML list sequence
    expect(draft.setYamlMappingText('nameserver-policy', '- item1\n- item2')).toBe(false)
    expect(draft.fieldErrors.value['nameserver-policy']).toContain('必须是有效 YAML 键值映射')

    // 3. scalar number
    expect(draft.setYamlMappingText('nameserver-policy', '12345')).toBe(false)

    // 4. scalar string
    expect(draft.setYamlMappingText('nameserver-policy', 'plain-scalar-string')).toBe(false)

    // 5. valid natural mapping recovers
    expect(draft.setYamlMappingText('nameserver-policy', 'domain:example.com: 1.1.1.1')).toBe(true)
    expect(draft.hasValidationErrors.value).toBe(false)
    expect(draft.draft.value['nameserver-policy']).toEqual({ 'domain:example.com': '1.1.1.1' })
  })

  it('supports explicit removeKey(key) for scalars, booleans, lists, and YAML mappings', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // Scalar deletion
    draft.removeKey('enhanced-mode')
    expect(draft.hasKey('enhanced-mode')).toBe(false)
    expect(draft.getString('enhanced-mode')).toBe('')

    // Boolean deletion (allows distinguishing undefined vs false)
    draft.removeKey('ipv6')
    expect(draft.hasKey('ipv6')).toBe(false)
    expect(draft.getBool('ipv6', false)).toBe(false)

    // List deletion
    draft.removeKey('nameserver')
    expect(draft.hasKey('nameserver')).toBe(false)
    expect('nameserver' in draft.draft.value).toBe(false)

    // YAML mapping deletion
    draft.removeKey('proxy-server-nameserver-policy')
    expect(draft.hasKey('proxy-server-nameserver-policy')).toBe(false)
    expect(draft.getYamlMappingText('proxy-server-nameserver-policy')).toBe('')

    // Custom extension deletion
    draft.removeKey('custom_extension')
    expect(draft.hasKey('custom_extension')).toBe(false)

    expect(draft.dirty.value).toBe(true)
  })

  it('clears invalid YAML errors when removeKey is invoked on an erroneous field', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    draft.setYamlMappingText('fallback-filter', 'invalid: [ unclosed')
    expect(draft.hasValidationErrors.value).toBe(true)
    expect(draft.fieldErrors.value['fallback-filter']).toBeDefined()

    draft.removeKey('fallback-filter')
    expect(draft.hasValidationErrors.value).toBe(false)
    expect(draft.fieldErrors.value['fallback-filter']).toBeUndefined()
    expect(draft.getYamlMappingText('fallback-filter')).toBe('')
    expect('fallback-filter' in draft.draft.value).toBe(false)
  })

  it('tracks preview freshness: current when generated, stale when draft is modified', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    expect(draft.previewYaml.value).toBe('')
    expect(draft.isPreviewStale.value).toBe(false)

    // Generate preview
    await draft.preview()
    expect(draft.previewYaml.value).toContain('1.1.1.1')
    expect(draft.isPreviewStale.value).toBe(false)

    // Modify a field -> preview becomes stale (NOT immediately deleted, labeled stale)
    draft.setString('listen', '0.0.0.0:5353')
    expect(draft.isPreviewStale.value).toBe(true)
    expect(draft.previewYaml.value).toContain('1.1.1.1')

    // Regenerate preview -> fresh again
    await draft.preview()
    expect(draft.isPreviewStale.value).toBe(false)

    // Toggle enabled -> preview becomes stale
    draft.enabled.value = false
    expect(draft.isPreviewStale.value).toBe(true)
  })

  it('discards stale preview response when draft changes while request is in flight', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    let resolveValidate: (val: any) => void
    vi.mocked(api.post).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveValidate = resolve
        })
    )

    const previewPromise = draft.preview()
    expect(draft.previewing.value).toBe(true)

    // User modifies draft while validate request is still in flight
    draft.setString('listen', '127.0.0.1:53')

    resolveValidate!({
      raw_yaml: 'stale_preview_yaml: true',
      config: { listen: 'stale' },
    })
    await previewPromise

    // The stale response must NOT overwrite previewYaml
    expect(draft.previewYaml.value).toBe('')
    expect(draft.previewing.value).toBe(false)
  })

  it('performs atomic save: validates, sends expected_revision PUT, then GET confirms and resets dirty', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    draft.setString('listen', '0.0.0.0:5353')
    expect(draft.dirty.value).toBe(true)

    await draft.save()

    expect(api.post).toHaveBeenCalledWith('/api/v1/settings/dns/validate', expect.any(Object))
    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({
        enabled: true,
        expected_revision: 5,
        raw_yaml: expect.stringContaining('0.0.0.0:5353'),
      })
    )
    expect(api.get).toHaveBeenCalledTimes(2)
    expect(draft.settings.value?.revision).toBe(6)
    expect(draft.dirty.value).toBe(false)
    expect(draft.notice.value).toContain('已保存')
  })

  it('retains dirty draft and sets error on 409 revision conflict', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    draft.setString('listen', '0.0.0.0:8053')
    vi.mocked(api.put).mockRejectedValueOnce(
      Object.assign(new Error('revision mismatch'), { status: 409, name: 'ApiError' })
    )

    await draft.save()

    expect(draft.error.value).toContain('revision 冲突')
    expect(draft.error.value).toContain('草稿已保留')
    expect(draft.getString('listen')).toBe('0.0.0.0:8053')
    expect(draft.dirty.value).toBe(true)
  })

  it('retains dirty draft and sets error on 422 validation failure', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    draft.setString('listen', 'invalid:port:spec')
    vi.mocked(api.put).mockRejectedValueOnce(
      Object.assign(new Error('port out of range'), { status: 422, name: 'ApiError' })
    )

    await draft.save()

    expect(draft.error.value).toContain('port out of range')
    expect(draft.getString('listen')).toBe('invalid:port:spec')
    expect(draft.dirty.value).toBe(true)
  })

  it('outer enabled toggle is authoritative: passing enabled=false overrides inner enable:true in PUT', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // Draft currently has enable: true
    expect(draft.enabled.value).toBe(true)

    // Toggle outer enabled to false
    draft.enabled.value = false
    expect(draft.dirty.value).toBe(true)

    await draft.save()

    expect(api.post).toHaveBeenCalledWith(
      '/api/v1/settings/dns/validate',
      expect.objectContaining({ enabled: false })
    )
    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({ enabled: false })
    )
  })

  it('guards against discarding dirty draft during reload if user cancels', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    draft.setString('listen', '127.0.0.1:9053')
    expect(draft.dirty.value).toBe(true)

    const confirmSpy = vi.spyOn(window, 'confirm')
    confirmSpy.mockReturnValueOnce(false)

    await draft.load(false)

    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining('放弃未保存的 DNS 草稿'))
    expect(api.get).toHaveBeenCalledTimes(1)
    expect(draft.getString('listen')).toBe('127.0.0.1:9053')

    // When user confirms, load proceeds
    confirmSpy.mockReturnValueOnce(true)
    await draft.load(false)
    expect(api.get).toHaveBeenCalledTimes(2)
    expect(draft.getString('listen')).toBe('0.0.0.0:1053')
    expect(draft.dirty.value).toBe(false)
  })
})

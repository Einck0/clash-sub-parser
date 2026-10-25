// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../../api/client'
import { useDnsDraft } from './useDnsDraft'
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
    // Custom unknown nested keys that must be preserved
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
          parsed = JSON.parse(body.raw_yaml)
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

  it('tracks dirty states across scalar, list, json, raw and outer enabled toggles', async () => {
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

    // 4. Outer enabled toggle
    draft.enabled.value = false
    expect(draft.dirty.value).toBe(true)
    draft.enabled.value = true
    expect(draft.dirty.value).toBe(false)

    // 5. Raw mode change
    await draft.switchMode('raw')
    draft.onRawInput('different raw yaml')
    expect(draft.dirty.value).toBe(true)
  })

  it('preserves all known keys and custom unknown nested keys in draft', async () => {
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
  })

  it('handles invalid JSON in advanced fields: preserves text, does not corrupt draft, blocks save, switch and preview', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    const ok = draft.setJsonText('nameserver-policy', '{ malformed json: ')
    expect(ok).toBe(false)
    expect(draft.fieldErrors.value['nameserver-policy']).toContain('必须是有效 JSON')
    expect(draft.hasValidationErrors.value).toBe(true)
    expect(draft.getJsonText('nameserver-policy')).toBe('{ malformed json: ')
    // draft object itself should NOT contain the invalid string
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

    // Fix JSON
    const fixed = draft.setJsonText('nameserver-policy', '{"geosite:google": "8.8.8.8"}')
    expect(fixed).toBe(true)
    expect(draft.fieldErrors.value['nameserver-policy']).toBeUndefined()
    expect(draft.hasValidationErrors.value).toBe(false)
  })

  it('rejects JSON non-plain-mapping inputs: array, null, scalar strings/numbers', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // 1. null
    expect(draft.setJsonText('nameserver-policy', 'null')).toBe(false)
    expect(draft.fieldErrors.value['nameserver-policy']).toContain('必须是有效 JSON')
    expect(draft.getJsonText('nameserver-policy')).toBe('null')
    expect(draft.draft.value['nameserver-policy']).toEqual({ 'geosite:cn': '223.5.5.5' })

    // 2. empty array []
    expect(draft.setJsonText('nameserver-policy', '[]')).toBe(false)
    expect(draft.fieldErrors.value['nameserver-policy']).toContain('必须是有效 JSON')
    expect(draft.getJsonText('nameserver-policy')).toBe('[]')

    // 3. array with elements [1, 2]
    expect(draft.setJsonText('nameserver-policy', '[1, 2]')).toBe(false)
    expect(draft.getJsonText('nameserver-policy')).toBe('[1, 2]')

    // 4. scalar number
    expect(draft.setJsonText('nameserver-policy', '12345')).toBe(false)
    expect(draft.getJsonText('nameserver-policy')).toBe('12345')

    // 5. scalar string
    expect(draft.setJsonText('nameserver-policy', '"scalar-string"')).toBe(false)
    expect(draft.getJsonText('nameserver-policy')).toBe('"scalar-string"')

    // 6. valid plain mapping recovers
    expect(draft.setJsonText('nameserver-policy', '{"domain:example.com": "1.1.1.1"}')).toBe(true)
    expect(draft.hasValidationErrors.value).toBe(false)
    expect(draft.draft.value['nameserver-policy']).toEqual({ 'domain:example.com': '1.1.1.1' })
  })

  it('removes keys when empty scalar or empty JSON is provided', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    expect(draft.hasKey('listen')).toBe(true)
    draft.setString('listen', '')
    expect(draft.hasKey('listen')).toBe(false)
    expect(draft.getString('listen')).toBe('')
    expect('listen' in draft.draft.value).toBe(false)
    expect(draft.dirty.value).toBe(true)

    expect(draft.hasKey('nameserver-policy')).toBe(true)
    draft.setJsonText('nameserver-policy', '   ')
    expect(draft.hasKey('nameserver-policy')).toBe(false)
    expect('nameserver-policy' in draft.draft.value).toBe(false)
    expect(draft.getJsonText('nameserver-policy')).toBe('')

    // List clear retains explicit [] per contract
    draft.setListText('nameserver', '')
    expect(draft.draft.value['nameserver']).toEqual([])
  })

  it('supports explicit removeKey(key) for scalars, booleans, lists, JSON mappings and custom fields', async () => {
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

    // List deletion (differs from setting empty [])
    draft.removeKey('nameserver')
    expect(draft.hasKey('nameserver')).toBe(false)
    expect('nameserver' in draft.draft.value).toBe(false)

    // JSON deletion
    draft.removeKey('proxy-server-nameserver-policy')
    expect(draft.hasKey('proxy-server-nameserver-policy')).toBe(false)
    expect(draft.getJsonText('proxy-server-nameserver-policy')).toBe('')

    // Custom extension deletion
    draft.removeKey('custom_extension')
    expect(draft.hasKey('custom_extension')).toBe(false)

    expect(draft.dirty.value).toBe(true)
  })

  it('clears invalid JSON errors when removeKey is invoked on an erroneous field', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    draft.setJsonText('fallback-filter', 'invalid: { not json')
    expect(draft.hasValidationErrors.value).toBe(true)
    expect(draft.fieldErrors.value['fallback-filter']).toBeDefined()

    draft.removeKey('fallback-filter')
    expect(draft.hasValidationErrors.value).toBe(false)
    expect(draft.fieldErrors.value['fallback-filter']).toBeUndefined()
    expect(draft.getJsonText('fallback-filter')).toBe('')
    expect('fallback-filter' in draft.draft.value).toBe(false)
  })

  it('replaces known invalid JSON fields sequentially and recovers clean draft validation state', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // Set invalid JSON on both known JSON fields
    expect(draft.setJsonText('nameserver-policy', '{ bad-json-1')).toBe(false)
    expect(draft.setJsonText('fallback-filter', '{ bad-json-2')).toBe(false)
    expect(draft.hasValidationErrors.value).toBe(true)
    expect(Object.keys(draft.fieldErrors.value).length).toBe(2)

    // Repairing one field keeps the remaining error and continues blocking save and switch
    expect(draft.setJsonText('nameserver-policy', '{"geosite:cn": "223.5.5.5"}')).toBe(true)
    expect(draft.fieldErrors.value['nameserver-policy']).toBeUndefined()
    expect(draft.fieldErrors.value['fallback-filter']).toContain('fallback-filter 必须是有效 JSON')
    expect(draft.hasValidationErrors.value).toBe(true)

    await draft.switchMode('raw')
    expect(draft.mode.value).toBe('visual')

    // Repairing the second field clears all validation errors
    expect(draft.setJsonText('fallback-filter', '{"geoip": true}')).toBe(true)
    expect(draft.fieldErrors.value['fallback-filter']).toBeUndefined()
    expect(draft.hasValidationErrors.value).toBe(false)
    expect(draft.error.value).toBe('')

    // Switch mode now succeeds
    await draft.switchMode('raw')
    expect(draft.mode.value).toBe('raw')
    expect(draft.raw.value).toContain('223.5.5.5')
    expect(draft.raw.value).toContain('geoip')
  })

  it('generates preview YAML without persisting to server, sets previewing and busy', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    expect(draft.previewYaml.value).toBe('')
    expect(draft.previewing.value).toBe(false)

    const previewPromise = draft.preview()
    expect(draft.previewing.value).toBe(true)
    expect(draft.busy.value).toBe(true)

    await previewPromise
    expect(draft.previewing.value).toBe(false)
    expect(draft.busy.value).toBe(false)
    expect(draft.previewYaml.value).toContain('1.1.1.1')
    expect(api.put).not.toHaveBeenCalled()
  })

  it('invalidates preview on any draft modification including enabled toggle', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    await draft.preview()
    expect(draft.previewYaml.value).not.toBe('')

    // 1. setString invalidates
    draft.setString('listen', '0.0.0.0:5353')
    expect(draft.previewYaml.value).toBe('')

    await draft.preview()
    expect(draft.previewYaml.value).not.toBe('')

    // 2. setBool invalidates
    draft.setBool('prefer-h3', true)
    expect(draft.previewYaml.value).toBe('')

    await draft.preview()
    expect(draft.previewYaml.value).not.toBe('')

    // 3. setListText invalidates
    draft.setListText('nameserver', '9.9.9.9')
    expect(draft.previewYaml.value).toBe('')

    await draft.preview()
    expect(draft.previewYaml.value).not.toBe('')

    // 4. setJsonText invalidates
    draft.setJsonText('nameserver-policy', '{"geosite:cn": "119.29.29.29"}')
    expect(draft.previewYaml.value).toBe('')

    await draft.preview()
    expect(draft.previewYaml.value).not.toBe('')

    // 5. removeKey invalidates
    draft.removeKey('nameserver-policy')
    expect(draft.previewYaml.value).toBe('')

    await draft.preview()
    expect(draft.previewYaml.value).not.toBe('')

    // 6. Direct enabled v-model change invalidates
    const initialRev = draft.draftRevision.value
    draft.enabled.value = false
    expect(draft.draftRevision.value).toBeGreaterThan(initialRev)
    expect(draft.previewYaml.value).toBe('')
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
    // Confirmatory GET
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

  it('discards stale responses when generation or revision changes during mode switch', async () => {
    const draft = useDnsDraft()
    await draft.load(true)

    // User is in visual mode, initiates switch to raw
    let resolveValidate: (val: any) => void
    vi.mocked(api.post).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveValidate = resolve
        })
    )

    const switchPromise = draft.switchMode('raw')
    expect(draft.validating.value).toBe(true)

    // While validating, user triggers another change which bumps draftRevision and generation
    draft.setString('listen', '1.2.3.4:53')

    // Now complete the earlier validate call
    resolveValidate!({
      raw_yaml: 'dns:\n  listen: stale:53\n',
      config: { listen: 'stale:53' },
    })
    await switchPromise

    // The stale response should NOT overwrite the user's latest draft
    expect(draft.getString('listen')).toBe('1.2.3.4:53')
    expect(draft.raw.value).not.toContain('stale:53')
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
    // api.get was called once initially, not called again because user cancelled
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

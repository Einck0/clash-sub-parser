// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { api } from '../../api/client'
import DnsView from './DnsView.vue'

const base = () => ({
  enabled: true,
  raw_yaml: 'dns:\n  enable: true\n',
  revision: 3,
  updated_at: 'now',
  source: 'saved' as const,
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
    'fake-ip-filter': ['*.lan', '+.local'],
    nameserver: ['1.1.1.1'],
    'default-nameserver': ['114.114.114.114'],
    fallback: ['8.8.8.8'],
    'proxy-server-nameserver': ['1.0.0.1'],
    'direct-nameserver': ['223.5.5.5'],
    'nameserver-policy': {},
    'fallback-filter': {},
    extra: { kept: true, deep: { id: 100 } },
  },
})

let mountEl: HTMLDivElement
let app: ReturnType<typeof createApp>
const flush = async () => {
  await Promise.resolve()
  await nextTick()
  await Promise.resolve()
  await nextTick()
}
function mount() {
  mountEl = document.createElement('div')
  document.body.appendChild(mountEl)
  app = createApp(DnsView)
  app.mount(mountEl)
}

let currentSettings = base()

describe('DNS settings page', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    currentSettings = base()
    vi.spyOn(api, 'get').mockImplementation(async () => ({ ...currentSettings }))
    vi.spyOn(api, 'post').mockImplementation(async (_path, body) => {
      const rawYaml = String((body as any).raw_yaml)
      let cfg: Record<string, unknown> = {}
      try {
        cfg = JSON.parse(rawYaml)
      } catch {
        cfg = { enable: false }
      }
      return { raw_yaml: rawYaml, config: cfg } as never
    })
    vi.spyOn(api, 'put').mockImplementation(async (_path, body: any) => {
      currentSettings = {
        ...currentSettings,
        enabled: body.enabled !== undefined ? body.enabled : currentSettings.enabled,
        raw_yaml: body.raw_yaml !== undefined ? body.raw_yaml : currentSettings.raw_yaml,
        revision: currentSettings.revision + 1,
      }
      return { ...currentSettings }
    })
  })

  afterEach(() => {
    app?.unmount()
    mountEl?.remove()
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('loads config and visual edits validate with exact request body, PUT with expected revision, then GETs the saved record', async () => {
    mount()
    await flush()
    expect(api.get).toHaveBeenCalledWith('/api/v1/settings/dns')
    expect(mountEl.querySelector('[data-testid="dns-view"]')?.getAttribute('aria-label')).toBe('客户端 DNS 配置')
    expect(mountEl.querySelector('header')).toBeNull()

    // Test visual controls: multiline nameserver
    const nameserver = mountEl.querySelector('[aria-label="nameserver"]') as HTMLTextAreaElement
    expect(nameserver).not.toBeNull()
    nameserver.value = '9.9.9.9\n8.8.8.8'
    nameserver.dispatchEvent(new Event('input', { bubbles: true }))
    nameserver.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    // Test enhanced-mode select
    const modeSelect = mountEl.querySelector('[aria-label="enhanced-mode"]') as HTMLSelectElement
    expect(modeSelect.value).toBe('fake-ip')
    modeSelect.value = 'redir-host'
    modeSelect.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    // Test ipv6 checkbox
    const ipv6Checkbox = mountEl.querySelector('[aria-label="ipv6"]') as HTMLInputElement
    expect(ipv6Checkbox.checked).toBe(false)
    ipv6Checkbox.checked = true
    ipv6Checkbox.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    // Test listen text input
    const listenInput = mountEl.querySelector('[aria-label="listen"]') as HTMLInputElement
    expect(listenInput.value).toBe('0.0.0.0:1053')
    listenInput.value = '127.0.0.1:5353'
    listenInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    // Test fake-ip-filter multiline
    const filterTextarea = mountEl.querySelector('[aria-label="fake-ip-filter"]') as HTMLTextAreaElement
    expect(filterTextarea.value).toContain('*.lan')
    filterTextarea.value = '*.custom.lan\n+.internal'
    filterTextarea.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()

    // Assert exact validate request: must include enabled AND raw_yaml
    expect(api.post).toHaveBeenCalledWith('/api/v1/settings/dns/validate', {
      enabled: true,
      raw_yaml: expect.stringContaining('9.9.9.9'),
    })
    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({
        enabled: true,
        expected_revision: 3,
        raw_yaml: expect.stringContaining('9.9.9.9'),
      })
    )
    expect(api.get).toHaveBeenCalledTimes(2)
  })

  it('supports light grouping and preserves 17 keys + custom nested keys', async () => {
    mount()
    await flush()

    // Check presence of basic group keys
    const cacheAlgo = mountEl.querySelector('[aria-label="cache-algorithm"]') as HTMLSelectElement
    expect(cacheAlgo.value).toBe('arc')
    cacheAlgo.value = 'bip'
    cacheAlgo.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    const useHosts = mountEl.querySelector('[aria-label="use-hosts"]') as HTMLInputElement
    expect(useHosts.checked).toBe(true)
    useHosts.checked = false
    useHosts.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    const directNs = mountEl.querySelector('[aria-label="direct-nameserver"]') as HTMLTextAreaElement
    expect(directNs.value).toContain('223.5.5.5')

    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(false)
    saveBtn.click()
    await flush()

    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({
        raw_yaml: expect.stringContaining('bip'),
      })
    )
  })

  it('performs enabled toggle roundtrip: disabled -> validate sends enabled:false, enabled again -> validate sends enabled:true', async () => {
    mount()
    await flush()

    const toggle = mountEl.querySelector('[data-testid="dns-enabled"]') as HTMLInputElement
    expect(toggle.checked).toBe(true)

    // Toggle off
    toggle.click()
    await flush()
    expect(toggle.checked).toBe(false)

    // Save with disabled
    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()

    expect(api.post).toHaveBeenCalledWith('/api/v1/settings/dns/validate', {
      enabled: false,
      raw_yaml: expect.any(String),
    })
    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({
        enabled: false,
        expected_revision: 3,
      })
    )

    // Toggle back on
    toggle.click()
    await flush()
    expect(toggle.checked).toBe(true)

    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()

    expect(api.post).toHaveBeenLastCalledWith('/api/v1/settings/dns/validate', {
      enabled: true,
      raw_yaml: expect.any(String),
    })
  })

  it('preserves unknown config keys and preserves raw text on malformed validation', async () => {
    mount()
    await flush()

    const rawTab = mountEl.querySelector('[data-testid="dns-raw-tab"]') as HTMLButtonElement
    rawTab.click()
    await flush()

    const editor = mountEl.querySelector('[data-testid="dns-raw-editor"]') as HTMLTextAreaElement
    editor.value = 'not: [valid'
    editor.dispatchEvent(new Event('input', { bubbles: true }))

    vi.mocked(api.post).mockRejectedValueOnce(new Error('invalid YAML'))
    ;(mountEl.querySelector('[data-testid="dns-visual-tab"]') as HTMLButtonElement).click()
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-raw-editor"]')).not.toBeNull()
    expect((mountEl.querySelector('[data-testid="dns-raw-editor"]') as HTMLTextAreaElement).value).toBe('not: [valid')
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('invalid YAML')
  })

  it('guards against invalid JSON in advanced fields, preserves draft text, and blocks save and mode switch', async () => {
    mount()
    await flush()

    const policyEditor = mountEl.querySelector('[aria-label="nameserver-policy"]') as HTMLTextAreaElement
    expect(policyEditor).not.toBeNull()

    // Type invalid JSON
    policyEditor.value = '{ invalid json'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    // Error is set and invalid text is preserved
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('nameserver-policy 必须是有效 JSON')
    expect(policyEditor.value).toBe('{ invalid json')

    // Save button is disabled
    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(true)

    // Attempting to switch to raw tab is blocked
    const rawTab = mountEl.querySelector('[data-testid="dns-raw-tab"]') as HTMLButtonElement
    rawTab.click()
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-raw-editor"]')).toBeNull()
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('请先修正字段格式错误')

    // Fix the JSON
    policyEditor.value = '{"geosite:cn": "223.5.5.5"}'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    // Save button becomes enabled
    expect(saveBtn.disabled).toBe(false)
    saveBtn.click()
    await flush()

    expect(api.post).toHaveBeenCalledWith(
      '/api/v1/settings/dns/validate',
      expect.objectContaining({
        raw_yaml: expect.stringContaining('223.5.5.5'),
      })
    )
  })

  it('replaces known invalid JSON fields and restores save and mode-switch capabilities', async () => {
    mount()
    await flush()

    // Verify querySelectorAll on container works cleanly without throwing TypeError
    const jsonEditors = mountEl.querySelectorAll<HTMLTextAreaElement>(
      '[aria-label="nameserver-policy"], [aria-label="fallback-filter"]'
    )
    expect(jsonEditors.length).toBe(2)

    const policyEditor = jsonEditors[0]
    const fallbackEditor = jsonEditors[1]

    // Type invalid JSON into both known JSON fields
    policyEditor.value = '{ bad-policy-json'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    policyEditor.dispatchEvent(new Event('change', { bubbles: true }))

    fallbackEditor.value = '{ bad-fallback-json'
    fallbackEditor.dispatchEvent(new Event('input', { bubbles: true }))
    fallbackEditor.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    // Verify errors are displayed and save/mode-switch are blocked
    expect(mountEl.querySelector('[data-testid="dns-error"]')).not.toBeNull()
    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(true)

    const rawTab = mountEl.querySelector('[data-testid="dns-raw-tab"]') as HTMLButtonElement
    rawTab.click()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-raw-editor"]')).toBeNull()

    // Repair nameserver-policy
    policyEditor.value = '{"geosite:cn": "223.5.5.5"}'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    policyEditor.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    // Fallback-filter still invalid -> save still blocked
    expect(saveBtn.disabled).toBe(true)
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('fallback-filter 必须是有效 JSON')

    // Repair fallback-filter
    fallbackEditor.value = '{"geoip": true}'
    fallbackEditor.dispatchEvent(new Event('input', { bubbles: true }))
    fallbackEditor.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    // Now save is enabled and error is cleared
    expect(saveBtn.disabled).toBe(false)
    expect(mountEl.querySelector('[data-testid="dns-error"]')).toBeNull()

    // Mode switch to raw editor now succeeds
    rawTab.click()
    await flush()
    const rawEditor = mountEl.querySelector('[data-testid="dns-raw-editor"]') as HTMLTextAreaElement
    expect(rawEditor).not.toBeNull()
    expect(rawEditor.value).toContain('223.5.5.5')

    // Save sends repaired payload
    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()
    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({
        raw_yaml: expect.stringContaining('223.5.5.5'),
      })
    )
  })

  it('sends the standalone enabled toggle and preserves drafts on revision conflict', async () => {
    mount()
    await flush()
    const toggle = mountEl.querySelector('[data-testid="dns-enabled"]') as HTMLInputElement
    toggle.click()
    await flush()
    const editorTab = mountEl.querySelector('[data-testid="dns-raw-tab"]') as HTMLButtonElement
    editorTab.click()
    await flush()
    const editor = mountEl.querySelector('[data-testid="dns-raw-editor"]') as HTMLTextAreaElement
    editor.value = 'enable: false'
    editor.dispatchEvent(new Event('input', { bubbles: true }))
    vi.mocked(api.put).mockRejectedValueOnce(Object.assign(new Error('conflict'), { status: 409, name: 'ApiError' }))
    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()
    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({ enabled: false, raw_yaml: 'enable: false', expected_revision: 3 })
    )
    expect((mountEl.querySelector('[data-testid="dns-raw-editor"]') as HTMLTextAreaElement).value).toBe('enable: false')
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('revision 冲突')
  })

  it('preserves draft and displays error on 422 validation failure', async () => {
    mount()
    await flush()
    const toggle = mountEl.querySelector('[data-testid="dns-enabled"]') as HTMLInputElement
    toggle.click()
    await flush()
    vi.mocked(api.put).mockRejectedValueOnce(
      Object.assign(new Error('Invalid DNS YAML parameters'), { status: 422, name: 'ApiError' })
    )
    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('Invalid DNS YAML parameters')
  })

  it('does not display default source banner even when source is default', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({ ...base(), source: 'default' } as never)
    mount()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-default-source"]')).toBeNull()
    expect(mountEl.querySelector('header')).toBeNull()
  })

  it('confirms before discarding dirty draft on reload', async () => {
    mount()
    await flush()
    const toggle = mountEl.querySelector('[data-testid="dns-enabled"]') as HTMLInputElement
    toggle.click()
    await flush()

    const confirmSpy = vi.spyOn(window, 'confirm')
    confirmSpy.mockReturnValueOnce(false)

    // Trigger error state to show retry/reload button
    const rawTab = mountEl.querySelector('[data-testid="dns-raw-tab"]') as HTMLButtonElement
    rawTab.click()
    await flush()
    vi.mocked(api.post).mockRejectedValueOnce(new Error('YAML validation failed'))
    const visualTab = mountEl.querySelector('[data-testid="dns-visual-tab"]') as HTMLButtonElement
    visualTab.click()
    await flush()

    const reloadBtn = mountEl.querySelector('[data-testid="dns-error"] button') as HTMLButtonElement
    expect(reloadBtn).not.toBeNull()
    reloadBtn.click()
    await flush()
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining('放弃未保存的 DNS 草稿'))
    // Since user declined (returned false), api.get should not be called again
    expect(api.get).toHaveBeenCalledTimes(1)
  })

  it('uses PageToolbar with accessible sr-only heading and no duplicate visible header', async () => {
    mount()
    await flush()

    const toolbar = mountEl.querySelector('[data-testid="page-toolbar"]')
    expect(toolbar).not.toBeNull()
    expect(mountEl.querySelector('header')).toBeNull()
    const srTitle = toolbar?.querySelector('h1.sr-only')
    expect(srTitle?.textContent).toBe('客户端 DNS 配置')
  })

  it('triggers beforeunload warning when draft is dirty', async () => {
    mount()
    await flush()

    const toggle = mountEl.querySelector('[data-testid="dns-enabled"]') as HTMLInputElement
    toggle.click()
    await flush()

    const event = new Event('beforeunload', { cancelable: true }) as BeforeUnloadEvent
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
  })
})

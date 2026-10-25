// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { api } from '../../api/client'
import type { DnsSettings, ValidatedDns } from './dnsTypes'
import DnsView from './DnsView.vue'

const base = (overrides?: Partial<DnsSettings>): DnsSettings => ({
  enabled: true,
  raw_yaml: 'dns:\n  enable: true\n',
  revision: 3,
  updated_at: '2026-10-25T10:00:00Z',
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
    'fake-ip-filter': ['*.lan', '+.local'],
    nameserver: ['1.1.1.1'],
    'default-nameserver': ['114.114.114.114'],
    fallback: ['8.8.8.8'],
    'proxy-server-nameserver': ['1.0.0.1'],
    'direct-nameserver': ['223.5.5.5'],
    'nameserver-policy': { 'geosite:cn': '223.5.5.5' },
    'fallback-filter': { geoip: true },
    'proxy-server-nameserver-policy': { 'geosite:netflix': '1.0.0.1' },
    'direct-nameserver-follow-policy': true,
    'prefer-h3': false,
    extra: { kept: true, deep: { id: 100 } },
  },
  ...overrides,
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

let currentSettings: DnsSettings = base()

describe('DNS settings page (DnsView)', () => {
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
      return { raw_yaml: rawYaml, config: cfg } as ValidatedDns
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

  it('loads config and visual edits validate with exact request body, PUT with expected revision, then GETs saved record', async () => {
    mount()
    await flush()
    expect(api.get).toHaveBeenCalledWith('/api/v1/settings/dns')
    expect(mountEl.querySelector('[data-testid="dns-view"]')?.getAttribute('aria-label')).toBe('客户端 DNS 配置')
    expect(mountEl.querySelector('header')).toBeNull()

    // Test primary nameserver multiline textarea
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

    // Test ipv6 tri-state select
    const ipv6Select = mountEl.querySelector('[aria-label="ipv6"]') as HTMLSelectElement
    expect(ipv6Select.value).toBe('false')
    ipv6Select.value = 'true'
    ipv6Select.dispatchEvent(new Event('change', { bubbles: true }))
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

  it('displays actual saved custom enum in scalar-select without overwriting (regression)', async () => {
    currentSettings = base({
      config: {
        ...base().config,
        'cache-algorithm': 'future-cache',
      },
    })
    mount()
    await flush()

    // Open fakeip-cache group
    const cacheToggle = mountEl.querySelector('[data-testid="dns-group-toggle-fakeip-cache"]') as HTMLButtonElement
    cacheToggle.click()
    await flush()

    const cacheSelect = mountEl.querySelector('[aria-label="cache-algorithm"]') as HTMLSelectElement
    expect(cacheSelect.value).toBe('future-cache')

    const customOption = mountEl.querySelector('[data-testid="dns-select-custom-option"]') as HTMLOptionElement
    expect(customOption).not.toBeNull()
    expect(customOption.value).toBe('future-cache')
    expect(customOption.textContent).toContain('future-cache')

    // Clean save does not overwrite or drop the unexpected enum
    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(true) // clean, nothing edited
  })

  it('accurately tracks dirty indicator badges near toolbar', async () => {
    mount()
    await flush()

    // Initially clean (source = saved)
    expect(mountEl.querySelector('[data-testid="dns-clean-badge"]')?.textContent).toContain('已保存')
    expect(mountEl.querySelector('[data-testid="dns-clean-badge"]')?.textContent).not.toContain('默认配置')
    expect(mountEl.querySelector('[data-testid="dns-dirty-badge"]')).toBeNull()

    // Modify a field -> becomes dirty
    const nameserver = mountEl.querySelector('[aria-label="nameserver"]') as HTMLTextAreaElement
    nameserver.value = '1.0.0.1'
    nameserver.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-dirty-badge"]')?.textContent).toContain('未保存')
    expect(mountEl.querySelector('[data-testid="dns-clean-badge"]')).toBeNull()

    // Save -> becomes clean
    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-clean-badge"]')?.textContent).toContain('已保存')
    expect(mountEl.querySelector('[data-testid="dns-dirty-badge"]')).toBeNull()
  })

  it('distinguishes unsaved draft DNS off vs confirmed saved off', async () => {
    mount()
    await flush()

    const toggle = mountEl.querySelector('[data-testid="dns-enabled"]') as HTMLInputElement
    expect(toggle.checked).toBe(true)
    expect(mountEl.querySelector('[data-testid="dns-disabled-notice"]')).toBeNull()

    // 1. User toggles off in draft -> displays UNSAVED draft off notice
    toggle.click()
    await flush()
    const unsavedNotice = mountEl.querySelector('[data-testid="dns-disabled-notice"]')
    expect(unsavedNotice).not.toBeNull()
    expect(unsavedNotice?.textContent).toContain('草稿中已关闭客户端 DNS（未保存）')

    // 2. Click save -> saved on server becomes off
    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()

    // 3. Now it displays CONFIRMED saved off notice
    const savedNotice = mountEl.querySelector('[data-testid="dns-disabled-notice"]')
    expect(savedNotice).not.toBeNull()
    expect(savedNotice?.textContent).toContain('客户端 DNS 当前已停用')
    expect(savedNotice?.textContent).not.toContain('未保存')
  })

  it('supports progressive disclosure groups and preserves all standard + new fields + custom nested keys', async () => {
    mount()
    await flush()

    // Check presence of group toggle buttons
    const bootstrapToggle = mountEl.querySelector('[data-testid="dns-group-toggle-connection-bootstrap"]') as HTMLButtonElement
    expect(bootstrapToggle).not.toBeNull()
    expect(bootstrapToggle.getAttribute('aria-expanded')).toBe('false')

    // Open connection-bootstrap accordion
    bootstrapToggle.click()
    await flush()
    expect(bootstrapToggle.getAttribute('aria-expanded')).toBe('true')

    // Test new boolean field: prefer-h3
    const preferH3 = mountEl.querySelector('[aria-label="prefer-h3"]') as HTMLSelectElement
    expect(preferH3.value).toBe('false')
    preferH3.value = 'true'
    preferH3.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    // Test new boolean field: direct-nameserver-follow-policy
    const followPolicy = mountEl.querySelector('[aria-label="direct-nameserver-follow-policy"]') as HTMLSelectElement
    expect(followPolicy.value).toBe('true')

    // Check direct-nameserver
    const directNs = mountEl.querySelector('[aria-label="direct-nameserver"]') as HTMLTextAreaElement
    expect(directNs.value).toContain('223.5.5.5')

    // Check preserved custom keys display
    expect(mountEl.querySelector('[data-testid="dns-unknown-keys"]')?.textContent).toContain('extra')

    // Save and verify payload includes new fields
    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(false)
    saveBtn.click()
    await flush()

    expect(api.put).toHaveBeenCalledWith(
      '/api/v1/settings/dns',
      expect.objectContaining({
        raw_yaml: expect.stringContaining('prefer-h3'),
      })
    )
  })

  it('displays accurate advisories for respect-rules and prefer-h3 conflicts', async () => {
    mount()
    await flush()

    // Open connection-bootstrap group
    const bootstrapToggle = mountEl.querySelector('[data-testid="dns-group-toggle-connection-bootstrap"]') as HTMLButtonElement
    bootstrapToggle.click()
    await flush()

    // In base config, respect-rules is true -> advisory is visible
    expect(mountEl.querySelector('[data-testid="respect-rules-advisory"]')).not.toBeNull()
    expect(mountEl.querySelector('[data-testid="respect-rules-advisory"]')?.textContent).toContain('proxy-server-nameserver')

    // Now set prefer-h3 to true as well -> conflict advisory appears
    const preferH3 = mountEl.querySelector('[aria-label="prefer-h3"]') as HTMLSelectElement
    preferH3.value = 'true'
    preferH3.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="prefer-h3-advisory"]')).not.toBeNull()
    expect(mountEl.querySelector('[data-testid="prefer-h3-advisory"]')?.textContent).toContain('冲突')
  })

  it('distinguishes unspecified from false/true in default configuration (Scene 1)', async () => {
    // Default revision 0 configuration where only enable and nameserver are present
    currentSettings = {
      enabled: true,
      raw_yaml: 'dns:\n  enable: true\n  nameserver:\n    - 1.1.1.1\n',
      revision: 0,
      updated_at: null,
      source: 'default',
      config: {
        enable: true,
        nameserver: ['1.1.1.1'],
      },
    }
    mount()
    await flush()

    // Primary nameserver is visible and editable
    const nameserver = mountEl.querySelector('[aria-label="nameserver"]') as HTMLTextAreaElement
    expect(nameserver.value).toBe('1.1.1.1')

    // Clean badge reflects default config source instead of '已保存'
    const cleanBadge = mountEl.querySelector('[data-testid="dns-clean-badge"]')
    expect(cleanBadge?.textContent).toContain('默认配置')
    expect(cleanBadge?.textContent).not.toContain('已保存')

    // enhanced-mode displays unspecified (''), NOT forced fake-ip
    const modeSelect = mountEl.querySelector('[aria-label="enhanced-mode"]') as HTMLSelectElement
    expect(modeSelect.value).toBe('')

    // ipv6 displays unspecified (''), NOT forced false
    const ipv6Select = mountEl.querySelector('[aria-label="ipv6"]') as HTMLSelectElement
    expect(ipv6Select.value).toBe('')

    // Reset button for ipv6 is not rendered because key is not configured
    expect(mountEl.querySelector('[data-testid="dns-reset-ipv6"]')).toBeNull()

    // Set ipv6 to true -> reset button appears
    ipv6Select.value = 'true'
    ipv6Select.dispatchEvent(new Event('change', { bubbles: true }))
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-reset-ipv6"]')).not.toBeNull()

    // Click reset button -> resets back to unspecified and deletes key
    const resetBtn = mountEl.querySelector('[data-testid="dns-reset-ipv6"]') as HTMLButtonElement
    resetBtn.click()
    await flush()
    expect(ipv6Select.value).toBe('')
  })

  it('deletes keys on explicit reset and clearing (FR-DU3)', async () => {
    mount()
    await flush()

    // 1. Reset scalar field: listen
    const listenReset = mountEl.querySelector('[data-testid="dns-reset-listen"]') as HTMLButtonElement
    expect(listenReset).not.toBeNull()
    listenReset.click()
    await flush()

    const listenInput = mountEl.querySelector('[aria-label="listen"]') as HTMLInputElement
    expect(listenInput.value).toBe('')

    // 2. Clearing scalar text removes the key
    const fakeIpRangeInput = mountEl.querySelector('[aria-label="fake-ip-range"]') as HTMLInputElement
    fakeIpRangeInput.value = ''
    fakeIpRangeInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    // 3. Reset select field: cache-algorithm
    const cacheReset = mountEl.querySelector('[data-testid="dns-reset-cache-algorithm"]') as HTMLButtonElement
    cacheReset.click()
    await flush()
    const cacheSelect = mountEl.querySelector('[aria-label="cache-algorithm"]') as HTMLSelectElement
    expect(cacheSelect.value).toBe('')

    // Save and assert cleared keys are absent from validated YAML
    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()

    expect(api.post).toHaveBeenCalledWith(
      '/api/v1/settings/dns/validate',
      expect.objectContaining({
        raw_yaml: expect.not.stringContaining('198.18.0.1/16'),
      })
    )
  })

  it('generates server-normalized YAML preview on demand, supports accessible labels, and invalidates on edit (FR-DU4)', async () => {
    mount()
    await flush()

    // Preview section is initially absent
    expect(mountEl.querySelector('[data-testid="dns-preview-section"]')).toBeNull()

    const previewBtn = mountEl.querySelector('[data-testid="dns-preview-btn"]') as HTMLButtonElement
    expect(previewBtn).not.toBeNull()
    expect(previewBtn.disabled).toBe(false)

    // Click preview
    previewBtn.click()
    await flush()

    expect(api.post).toHaveBeenCalledWith(
      '/api/v1/settings/dns/validate',
      expect.objectContaining({
        enabled: true,
        raw_yaml: expect.any(String),
      })
    )

    // Read-only preview area is displayed with accessible label
    const previewArea = mountEl.querySelector('[data-testid="dns-preview-yaml"]') as HTMLTextAreaElement
    expect(previewArea).not.toBeNull()
    expect(previewArea.readOnly).toBe(true)
    expect(previewArea.getAttribute('aria-label')).toBe('当前草稿规范化 DNS 配置块')

    // Close preview via close button with accessible aria-label
    const closeBtn = mountEl.querySelector('[data-testid="dns-close-preview"]') as HTMLButtonElement
    expect(closeBtn.getAttribute('aria-label')).toBe('关闭预览')
    closeBtn.click()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-preview-section"]')).toBeNull()

    // Regenerate preview
    previewBtn.click()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-preview-section"]')).not.toBeNull()

    // Edit nameserver -> preview is immediately invalidated and cleared
    const nameserver = mountEl.querySelector('[aria-label="nameserver"]') as HTMLTextAreaElement
    nameserver.value = '10.0.0.1'
    nameserver.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-preview-section"]')).toBeNull()
  })

  it('guards against invalid JSON, preserves draft text, and blocks save, switch, and preview', async () => {
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

    // Save button is disabled (has validation errors)
    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(true)

    // Preview button is disabled
    const previewBtn = mountEl.querySelector('[data-testid="dns-preview-btn"]') as HTMLButtonElement
    expect(previewBtn.disabled).toBe(true)

    // Attempting to switch to raw tab is blocked
    const rawTab = mountEl.querySelector('[data-testid="dns-raw-tab"]') as HTMLButtonElement
    rawTab.click()
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-raw-editor"]')).toBeNull()
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('请先修正字段格式错误')

    // Scenario A: Restore to baseline JSON {"geosite:cn": "223.5.5.5"} -> error cleared, preview available, but save remains disabled because unchanged (!dirty)
    policyEditor.value = '{"geosite:cn": "223.5.5.5"}'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-error"]')).toBeNull()
    expect(previewBtn.disabled).toBe(false)
    expect(mountEl.querySelector('[data-testid="dns-clean-badge"]')?.textContent).toContain('已保存')
    expect(saveBtn.disabled).toBe(true) // properly disabled because draft is clean

    // Scenario B: Make a distinct meaningful valid edit -> save becomes enabled
    policyEditor.value = '{"geosite:cn": "119.29.29.29"}'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-dirty-badge"]')?.textContent).toContain('未保存')
    expect(saveBtn.disabled).toBe(false)
    expect(previewBtn.disabled).toBe(false)

    saveBtn.click()
    await flush()

    expect(api.post).toHaveBeenCalledWith(
      '/api/v1/settings/dns/validate',
      expect.objectContaining({
        raw_yaml: expect.stringContaining('119.29.29.29'),
      })
    )
  })

  it('retains drafts and errors across accordion fold/unfold toggles', async () => {
    mount()
    await flush()

    // Open fallback-policy group
    const fallbackToggle = mountEl.querySelector('[data-testid="dns-group-toggle-fallback-policy"]') as HTMLButtonElement
    fallbackToggle.click()
    await flush()

    const fallbackEditor = mountEl.querySelector('[aria-label="fallback-filter"]') as HTMLTextAreaElement
    fallbackEditor.value = '{ malformed-filter'
    fallbackEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('fallback-filter 必须是有效 JSON')

    // Close accordion
    fallbackToggle.click()
    await flush()
    expect(fallbackToggle.getAttribute('aria-expanded')).toBe('false')

    // Input element is still in DOM (via v-show) with invalid text preserved
    expect(fallbackEditor.value).toBe('{ malformed-filter')
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('fallback-filter 必须是有效 JSON')

    // Reopen accordion
    fallbackToggle.click()
    await flush()
    expect(fallbackToggle.getAttribute('aria-expanded')).toBe('true')
    expect(fallbackEditor.value).toBe('{ malformed-filter')
  })

  it('replaces multiple invalid JSON fields sequentially and restores capabilities', async () => {
    mount()
    await flush()

    const policyEditor = mountEl.querySelector('[aria-label="nameserver-policy"]') as HTMLTextAreaElement
    const fallbackEditor = mountEl.querySelector('[aria-label="fallback-filter"]') as HTMLTextAreaElement
    const previewBtn = mountEl.querySelector('[data-testid="dns-preview-btn"]') as HTMLButtonElement
    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement

    // Type invalid JSON into both known JSON fields
    policyEditor.value = '{ bad-policy-json'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))

    fallbackEditor.value = '{ bad-fallback-json'
    fallbackEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(saveBtn.disabled).toBe(true)
    expect(previewBtn.disabled).toBe(true)

    // Repair nameserver-policy
    policyEditor.value = '{"geosite:cn": "223.5.5.5"}'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    // Fallback-filter still invalid -> save and preview still blocked
    expect(saveBtn.disabled).toBe(true)
    expect(previewBtn.disabled).toBe(true)
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('fallback-filter 必须是有效 JSON')

    // Repair fallback-filter back to baseline -> error is cleared, preview becomes available, draft is clean (!dirty)
    fallbackEditor.value = '{"geoip": true}'
    fallbackEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-error"]')).toBeNull()
    expect(previewBtn.disabled).toBe(false)
    expect(saveBtn.disabled).toBe(true) // identical to baseline, not dirty

    // Make a genuine distinct change to nameserver-policy -> save becomes enabled
    policyEditor.value = '{"geosite:google": "8.8.8.8"}'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(saveBtn.disabled).toBe(false)
    expect(mountEl.querySelector('[data-testid="dns-dirty-badge"]')?.textContent).toContain('未保存')

    saveBtn.click()
    await flush()

    expect(api.post).toHaveBeenCalledWith(
      '/api/v1/settings/dns/validate',
      expect.objectContaining({
        raw_yaml: expect.stringContaining('8.8.8.8'),
      })
    )
  })

  it('sends the standalone enabled toggle and preserves drafts on revision conflict (409)', async () => {
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
    vi.mocked(api.get).mockResolvedValueOnce(base({ source: 'default' }))
    mount()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-default-source"]')).toBeNull()
    expect(mountEl.querySelector('header')).toBeNull()
  })

  it('confirms before discarding dirty draft on reload and disables reload during busy', async () => {
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
    expect(reloadBtn.disabled).toBe(false)
    reloadBtn.click()
    await flush()
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringContaining('放弃未保存的 DNS 草稿'))
    expect(api.get).toHaveBeenCalledTimes(1)
  })

  it('uses PageToolbar with accessible sr-only heading, tab semantics, and accessible raw editor', async () => {
    mount()
    await flush()

    const toolbar = mountEl.querySelector('[data-testid="page-toolbar"]')
    expect(toolbar).not.toBeNull()
    expect(mountEl.querySelector('header')).toBeNull()
    const srTitle = toolbar?.querySelector('h1.sr-only')
    expect(srTitle?.textContent).toBe('客户端 DNS 配置')

    // Tab semantics
    const visualTab = mountEl.querySelector('[data-testid="dns-visual-tab"]') as HTMLButtonElement
    const rawTab = mountEl.querySelector('[data-testid="dns-raw-tab"]') as HTMLButtonElement
    expect(visualTab.getAttribute('role')).toBe('tab')
    expect(visualTab.getAttribute('aria-selected')).toBe('true')
    expect(rawTab.getAttribute('role')).toBe('tab')
    expect(rawTab.getAttribute('aria-selected')).toBe('false')

    // Switch to raw tab
    rawTab.click()
    await flush()
    expect(visualTab.getAttribute('aria-selected')).toBe('false')
    expect(rawTab.getAttribute('aria-selected')).toBe('true')

    const rawEditor = mountEl.querySelector('[data-testid="dns-raw-editor"]') as HTMLTextAreaElement
    expect(rawEditor.getAttribute('aria-label')).toBe('完整 YAML 配置草稿')
  })

  it('supports mobile-responsive toolbar wrapping and non-squeezed enable label', async () => {
    mount()
    await flush()

    const toolbar = mountEl.querySelector('[data-testid="page-toolbar"]')
    expect(toolbar).not.toBeNull()
    expect(toolbar?.className).toContain('flex-col')
    expect(toolbar?.className).toContain('sm:flex-row')

    const enableLabel = mountEl.querySelector('[data-testid="dns-enabled-label"]')
    expect(enableLabel).not.toBeNull()
    expect(enableLabel?.className).toContain('shrink-0')

    const enableSpan = enableLabel?.querySelector('span')
    expect(enableSpan?.className).toContain('whitespace-nowrap')

    const actions = mountEl.querySelector('[data-testid="toolbar-actions"]')
    expect(actions).not.toBeNull()
    expect(actions?.className).toContain('w-full')
    expect(actions?.className).toContain('sm:w-auto')
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

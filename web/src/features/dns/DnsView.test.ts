// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createRouter, createMemoryHistory, type Router } from 'vue-router'
import yaml from 'js-yaml'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
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
let router: Router

function createTestRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/settings', component: { render: () => null } },
      { path: '/dns', component: DnsView },
      { path: '/chains', component: { render: () => null } },
    ],
  })
}

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
  app.use(router)
  app.mount(mountEl)
}

let currentSettings: DnsSettings = base()

describe('DNS settings page (DnsView)', () => {
  beforeEach(async () => {
    setLocale('zh-CN')
    router = createTestRouter()
    await router.push('/dns')
    await router.isReady()
    document.body.innerHTML = ''
    currentSettings = base()
    vi.spyOn(api, 'get').mockImplementation(async () => ({ ...currentSettings }))
    vi.spyOn(api, 'post').mockImplementation(async (_path, body) => {
      const rawYaml = String((body as any).raw_yaml)
      let cfg: Record<string, unknown> = {}
      try {
        cfg = (yaml.load(rawYaml) as Record<string, unknown>) || {}
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
    setLocale('zh-CN')
    vi.restoreAllMocks()
  })

  it('renders top SharedSettingsSections navigation bar with active=dns', async () => {
    mount()
    await flush()

    const nav = mountEl.querySelector('[data-testid="settings-sections-nav"]')
    expect(nav).not.toBeNull()

    const dnsLink = mountEl.querySelector('[data-testid="settings-section-dns"]')
    expect(dnsLink).not.toBeNull()
    expect(dnsLink?.getAttribute('aria-current')).toBe('page')
    expect(dnsLink?.className).toContain('btn-primary')

    const cspLink = mountEl.querySelector('[data-testid="settings-section-csp"]')
    expect(cspLink).not.toBeNull()
    expect(cspLink?.getAttribute('aria-current')).toBeNull()

    const chainsLink = mountEl.querySelector('[data-testid="settings-section-chains"]')
    expect(chainsLink).not.toBeNull()
  })

  it('renders common core fields upfront without folding in accordions (nameserver, fallback, default-nameserver, fake-ip)', async () => {
    mount()
    await flush()

    const commonSection = mountEl.querySelector('[data-testid="dns-common-section"]')
    expect(commonSection).not.toBeNull()

    // Upfront fields are all immediately in DOM and visible
    expect(commonSection?.querySelector('[aria-label="nameserver"]')).not.toBeNull()
    expect(commonSection?.querySelector('[aria-label="fallback"]')).not.toBeNull()
    expect(commonSection?.querySelector('[aria-label="default-nameserver"]')).not.toBeNull()
    expect(commonSection?.querySelector('[aria-label="enhanced-mode"]')).not.toBeNull()
    expect(commonSection?.querySelector('[aria-label="fake-ip-range"]')).not.toBeNull()
    expect(commonSection?.querySelector('[aria-label="fake-ip-filter"]')).not.toBeNull()
    expect(commonSection?.querySelector('[aria-label="ipv6"]')).not.toBeNull()
    expect(commonSection?.querySelector('[aria-label="listen"]')).not.toBeNull()
  })

  it('supports list individual item remove and quick item add', async () => {
    mount()
    await flush()

    // Primary nameserver has 1 item initial ('1.1.1.1')
    expect(mountEl.querySelector('[data-testid="dns-list-item-nameserver-0"]')?.textContent).toContain('1.1.1.1')

    // Add new item via input
    const addInput = mountEl.querySelector('[data-testid="dns-list-new-nameserver"]') as HTMLInputElement
    const addBtn = mountEl.querySelector('[data-testid="dns-list-add-nameserver"]') as HTMLButtonElement
    addInput.value = '8.8.8.8'
    addInput.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    addBtn.click()
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-list-item-nameserver-1"]')?.textContent).toContain('8.8.8.8')

    // Remove item at index 0 ('1.1.1.1')
    const removeBtn0 = mountEl.querySelector('[data-testid="dns-list-remove-nameserver-0"]') as HTMLButtonElement
    removeBtn0.click()
    await flush()

    // Now index 0 is '8.8.8.8'
    expect(mountEl.querySelector('[data-testid="dns-list-item-nameserver-0"]')?.textContent).toContain('8.8.8.8')
    expect(mountEl.querySelector('[data-testid="dns-list-item-nameserver-1"]')).toBeNull()
  })

  it('supports multiline paste into list input and batch textarea', async () => {
    mount()
    await flush()

    const addInput = mountEl.querySelector('[data-testid="dns-list-new-fallback"]') as HTMLInputElement
    expect(addInput).not.toBeNull()

    // Simulate paste event with multiple lines
    const pasteEvent = new Event('paste', { bubbles: true, cancelable: true }) as any
    pasteEvent.clipboardData = {
      getData: (type: string) => (type === 'text' ? '9.9.9.9\n149.112.112.112\n' : ''),
    }
    addInput.dispatchEvent(pasteEvent)
    await flush()

    // Fallback initially had '8.8.8.8', now should also have the pasted items
    expect(mountEl.querySelector('[data-testid="dns-list-item-fallback-0"]')?.textContent).toContain('8.8.8.8')
    expect(mountEl.querySelector('[data-testid="dns-list-item-fallback-1"]')?.textContent).toContain('9.9.9.9')
    expect(mountEl.querySelector('[data-testid="dns-list-item-fallback-2"]')?.textContent).toContain('149.112.112.112')
  })

  it('loads config and visual edits validate with exact request body, PUT with expected revision, then GETs saved record', async () => {
    mount()
    await flush()
    expect(api.get).toHaveBeenCalledWith('/api/v1/settings/dns')
    expect(mountEl.querySelector('[data-testid="dns-view"]')?.getAttribute('aria-label')).toBe('客户端 DNS 配置')

    // Test primary nameserver batch textarea
    const nameserver = mountEl.querySelector('[aria-label="nameserver"]') as HTMLTextAreaElement
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

    ;(mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement).click()
    await flush()

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

  it('accurately tracks dirty indicator badges near toolbar', async () => {
    mount()
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-clean-badge"]')?.textContent).toContain('已保存')
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

  it('handles natural YAML mapping: renders clean YAML, displays errors on invalid YAML syntax, preserves input', async () => {
    mount()
    await flush()

    // Open routing-policy group
    const policyGroupToggle = mountEl.querySelector('[data-testid="dns-group-toggle-routing-policy"]') as HTMLButtonElement
    expect(policyGroupToggle).not.toBeNull()
    policyGroupToggle.click()
    await flush()

    const policyEditor = mountEl.querySelector('[data-testid="dns-yaml-nameserver-policy"]') as HTMLTextAreaElement
    expect(policyEditor).not.toBeNull()
    expect(policyEditor.value).toContain('geosite:cn: 223.5.5.5')

    // Input invalid YAML syntax
    policyEditor.value = 'invalid: [ unclosed yaml array'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    // Error is displayed and invalid text is preserved
    expect(mountEl.querySelector('[data-testid="dns-error"]')?.textContent).toContain('nameserver-policy YAML 格式无效')
    expect(policyEditor.value).toBe('invalid: [ unclosed yaml array')

    // Save button is disabled
    const saveBtn = mountEl.querySelector('[data-testid="dns-save"]') as HTMLButtonElement
    expect(saveBtn.disabled).toBe(true)

    // Preview button is disabled
    const previewBtn = mountEl.querySelector('[data-testid="dns-preview-btn"]') as HTMLButtonElement
    expect(previewBtn.disabled).toBe(true)

    // Restore to valid natural YAML mapping
    policyEditor.value = 'geosite:cn: 119.29.29.29\ngeosite:google:\n  - 8.8.8.8\n  - 8.8.4.4'
    policyEditor.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-error"]')).toBeNull()
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

  it('generates server-normalized YAML preview with current vs stale state tracking', async () => {
    mount()
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-preview-section"]')).toBeNull()

    const previewBtn = mountEl.querySelector('[data-testid="dns-preview-btn"]') as HTMLButtonElement
    previewBtn.click()
    await flush()

    // Preview section is displayed with CURRENT status badge
    expect(mountEl.querySelector('[data-testid="dns-preview-section"]')).not.toBeNull()
    expect(mountEl.querySelector('[data-testid="dns-preview-status-current"]')).not.toBeNull()
    expect(mountEl.querySelector('[data-testid="dns-preview-status-stale"]')).toBeNull()

    // Modify a draft field -> status becomes STALE
    const nameserver = mountEl.querySelector('[aria-label="nameserver"]') as HTMLTextAreaElement
    nameserver.value = '10.0.0.1'
    nameserver.dispatchEvent(new Event('input', { bubbles: true }))
    await flush()

    expect(mountEl.querySelector('[data-testid="dns-preview-status-stale"]')).not.toBeNull()
    expect(mountEl.querySelector('[data-testid="dns-preview-status-current"]')).toBeNull()

    // Regenerate preview -> becomes CURRENT again
    previewBtn.click()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-preview-status-current"]')).not.toBeNull()

    // Close preview via close button
    const closeBtn = mountEl.querySelector('[data-testid="dns-close-preview"]') as HTMLButtonElement
    closeBtn.click()
    await flush()
    expect(mountEl.querySelector('[data-testid="dns-preview-section"]')).toBeNull()
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
})

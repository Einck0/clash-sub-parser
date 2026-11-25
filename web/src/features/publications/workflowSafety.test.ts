// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { api } from '../../api/client'
import { setLocale } from '../../locales'
import PublicationsView from './PublicationsView.vue'
import { usePublications } from './usePublications'
import type { PreviewResult } from './publicationTypes'

const result: PreviewResult = {
  target: 'mihomo', snapshot_id: 'snapshot-current', snapshot_digest: 'snapshot-digest',
  content_digest: 'content-digest', content: 'proxies: []', content_type: 'application/yaml', filename: 'mihomo.yaml',
  manifest: { node_count: 1, excluded_count: 2, excluded: [
    { node_id: 'node-legacy', display_name: 'Tokyo Legacy', reason: 'Unsupported transport' },
    { node_id: 'safe-node-id', reason: 'Missing required capability' },
  ] },
}

describe('Export workflow safety', () => {
  let app: ReturnType<typeof createApp> | null = null
  let container: HTMLDivElement
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    setLocale('zh-CN')
    vi.spyOn(api, 'get').mockResolvedValue({ export_mode: 'open' })
    container = document.createElement('div')
    document.body.appendChild(container)
  })
  afterEach(() => {
    app?.unmount()
    app = null
    container.remove()
    localStorage.clear()
    sessionStorage.clear()
    vi.restoreAllMocks()
  })
  async function flush() {
    await Promise.resolve()
    await nextTick()
    await Promise.resolve()
    await nextTick()
  }

  it('has one snapshot download action, named exclusions, ID fallback and search', async () => {
    vi.spyOn(api, 'post').mockResolvedValue(result)
    app = createApp({ render: () => h(PublicationsView) })
    app.mount(container)
    ;(container.querySelector('[data-testid="generate-preview-btn"]') as HTMLButtonElement).click()
    await flush()
    const exclusions = container.querySelector('[data-testid="preview-exclusions"]')!
    expect(exclusions.textContent).toContain('Tokyo Legacy')
    expect(exclusions.textContent).toContain('safe-node-id')
    expect(exclusions.textContent).not.toContain('node-legacy')
    expect(container.querySelectorAll('[data-testid="download-preview-btn"]')).toHaveLength(1)
    expect(container.querySelectorAll('[data-testid="download-client-subscription-btn"]')).toHaveLength(0)
    const search = container.querySelector('[data-testid="exclusion-search"]') as HTMLInputElement
    search.value = 'capability'
    search.dispatchEvent(new Event('input'))
    await nextTick()
    expect(exclusions.textContent).not.toContain('Tokyo Legacy')
    expect(exclusions.textContent).toContain('safe-node-id')
    const select = container.querySelector('[data-testid="preview-target-select"]') as HTMLSelectElement
    select.value = 'surge'
    select.dispatchEvent(new Event('change'))
    await nextTick()
    expect(container.querySelector('[data-testid="preview-actions"]')).toBeNull()
    expect(container.querySelector('[data-testid="preview-exclusions"]')).toBeNull()
  })

  it('invalidates the old preview while refreshing and preserves real rejection', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValueOnce(result)
    const state = usePublications('mihomo')
    await state.fetchPreview('mihomo')
    let reject!: (reason: Error) => void
    post.mockImplementationOnce(() => new Promise((_resolve, fail) => { reject = fail }))
    const refresh = state.fetchPreview('mihomo')
    expect(state.preview.value).toBeNull()
    expect(state.loadingPreview.value).toBe(true)
    reject(new Error('Preview service unavailable'))
    await refresh
    expect(state.preview.value).toBeNull()
    expect(state.error.value).toBe('Preview service unavailable')
    expect(state.loadingPreview.value).toBe(false)
  })

  it('does not silently report a failed browser download as successful', () => {
    const state = usePublications('mihomo')
    vi.stubGlobal('URL', class {
      static createObjectURL() { throw new Error('Browser download unavailable') }
    })
    try {
      expect(() => state.downloadFile('test.yaml', 'config', 'text/plain')).toThrow('Browser download unavailable')
    } finally { vi.unstubAllGlobals() }
  })
})

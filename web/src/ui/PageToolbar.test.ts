// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import PageToolbar from './PageToolbar.vue'

describe('PageToolbar Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    document.body.innerHTML = ''
  })

  it('renders default slot controls and actions slot without redundant visible h1', async () => {
    app = createApp({
      render() {
        return h(
          PageToolbar,
          {
            title: 'Node Ledger',
            description: 'Overview of all node probes and protocols',
          },
          {
            default: () => h('input', { 'data-testid': 'search-input', placeholder: 'Filter nodes...' }),
            actions: () => h('button', { 'data-testid': 'refresh-btn' }, 'Probe All'),
          }
        )
      },
    })

    app.mount(container)
    await nextTick()

    const toolbar = container.querySelector('[data-testid="page-toolbar"]')
    expect(toolbar).toBeTruthy()
    expect(toolbar?.classList.contains('flex-wrap')).toBe(true)

    // Verify title is rendered as h2 (not duplicate visible h1)
    const h1 = container.querySelector('h1')
    expect(h1).toBeNull()

    const h2 = container.querySelector('[data-testid="toolbar-title"]')
    expect(h2).toBeTruthy()
    expect(h2?.textContent?.trim()).toBe('Node Ledger')

    const desc = container.querySelector('[data-testid="toolbar-description"]')
    expect(desc?.textContent?.trim()).toBe('Overview of all node probes and protocols')

    // Verify slots
    expect(container.querySelector('[data-testid="search-input"]')).toBeTruthy()
    expect(container.querySelector('[data-testid="refresh-btn"]')).toBeTruthy()
  })

  it('renders sr-only h1 when srOnlyTitle is set for screen-reader heading hierarchy', async () => {
    app = createApp({
      render() {
        return h(PageToolbar, {
          title: 'DNS Rules',
          srOnlyTitle: true,
        })
      },
    })

    app.mount(container)
    await nextTick()

    const h1 = container.querySelector('h1.sr-only')
    expect(h1).toBeTruthy()
    expect(h1?.textContent?.trim()).toBe('DNS Rules')

    // No visible h2
    const h2 = container.querySelector('h2')
    expect(h2).toBeNull()
  })
})

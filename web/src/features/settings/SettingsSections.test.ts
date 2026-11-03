// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { createApp, h } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import SettingsSections, { type SettingsSectionId } from './SettingsSections.vue'
import { setLocale } from '../../locales'

describe('SettingsSections Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  const router = createRouter({
    history: createWebHashHistory(),
    routes: [
      { path: '/settings', component: { render: () => h('div', 'Settings') } },
      { path: '/dns', component: { render: () => h('div', 'DNS') } },
      { path: '/chains', component: { render: () => h('div', 'Chains') } },
    ],
  })

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    setLocale('zh-CN')
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    setLocale('zh-CN')
  })

  function mountSections(active: SettingsSectionId = 'csp') {
    app = createApp({
      render() {
        return h(SettingsSections, { active })
      },
    })
    app.use(router)
    app.mount(container)

    return {
      getCspLink: () => container.querySelector('[data-testid="settings-section-csp"]') as HTMLAnchorElement,
      getDnsLink: () => container.querySelector('[data-testid="settings-section-dns"]') as HTMLAnchorElement,
      getChainsLink: () => container.querySelector('[data-testid="settings-section-chains"]') as HTMLAnchorElement,
      getNav: () => container.querySelector('[data-testid="settings-sections-nav"]') as HTMLElement,
    }
  }

  it('renders all three navigation links with accessible labels and correct routes', () => {
    const { getCspLink, getDnsLink, getChainsLink } = mountSections('csp')

    const csp = getCspLink()
    const dns = getDnsLink()
    const chains = getChainsLink()

    expect(csp).toBeTruthy()
    expect(dns).toBeTruthy()
    expect(chains).toBeTruthy()

    expect(csp.textContent).toContain('CSP 设置')
    expect(dns.textContent).toContain('DNS 设置')
    expect(chains.textContent).toContain('链式代理')

    expect(csp.getAttribute('href')).toContain('/settings')
    expect(dns.getAttribute('href')).toContain('/dns')
    expect(chains.getAttribute('href')).toContain('/chains')
  })

  it('marks csp active when active="csp"', () => {
    const { getCspLink, getDnsLink, getChainsLink } = mountSections('csp')

    expect(getCspLink().getAttribute('aria-current')).toBe('page')
    expect(getCspLink().className).toContain('btn-primary')

    expect(getDnsLink().getAttribute('aria-current')).toBeNull()
    expect(getChainsLink().getAttribute('aria-current')).toBeNull()
  })

  it('marks dns active when active="dns"', () => {
    const { getCspLink, getDnsLink, getChainsLink } = mountSections('dns')

    expect(getCspLink().getAttribute('aria-current')).toBeNull()
    expect(getDnsLink().getAttribute('aria-current')).toBe('page')
    expect(getDnsLink().className).toContain('btn-primary')
    expect(getChainsLink().getAttribute('aria-current')).toBeNull()
  })

  it('marks chains active when active="chains"', () => {
    const { getCspLink, getDnsLink, getChainsLink } = mountSections('chains')

    expect(getCspLink().getAttribute('aria-current')).toBeNull()
    expect(getDnsLink().getAttribute('aria-current')).toBeNull()
    expect(getChainsLink().getAttribute('aria-current')).toBe('page')
    expect(getChainsLink().className).toContain('btn-primary')
  })

  it('supports English localized labels when locale is en-US', () => {
    setLocale('en-US')
    const { getCspLink, getDnsLink, getChainsLink } = mountSections('csp')

    expect(getCspLink().textContent).toContain('CSP Settings')
    expect(getDnsLink().textContent).toContain('DNS Settings')
    expect(getChainsLink().textContent).toContain('Proxy Chains')
  })

  it('has non-overflow responsive flex-wrap layout', () => {
    const { getNav } = mountSections('csp')
    expect(getNav().className).toContain('flex-wrap')
  })
})

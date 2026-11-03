// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { router, routes } from './index'
import { ROUTE_STORAGE_KEY } from '../navigation'

describe('router/index.ts route registry and redirection', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('registers /chains route record targeting ChainsView', () => {
    const chainRoute = routes.find((r) => r.path === '/chains')
    expect(chainRoute).toBeDefined()
    expect(chainRoute?.name).toBe('chains')
    expect(chainRoute?.meta?.title).toBe('Proxy Chains')
  })

  it('navigates to /chains and synchronizes localStorage active tab', async () => {
    await router.push('/chains')
    await router.isReady()
    expect(router.currentRoute.value.path).toBe('/chains')
    expect(window.localStorage.getItem(ROUTE_STORAGE_KEY)).toBe('chains')
  })

  it('redirects unknown or legacy hash pathMatch correctly', async () => {
    await router.push('/probes')
    expect(router.currentRoute.value.path).toBe('/nodes')

    await router.push('/unknown-nonexistent-path')
    expect(router.currentRoute.value.path).toBe('/dashboard')
  })
})

// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import {
  VALID_TABS,
  MOBILE_PRIMARY_TABS,
  MOBILE_MORE_TABS,
  ROUTE_STORAGE_KEY,
  isValidTab,
  isMobileMoreTab,
  isLegacyHash,
  getRouteTab,
  setRouteTab,
} from './navigation'

describe('navigation.ts core router & dock utilities', () => {
  beforeEach(() => {
    window.location.hash = ''
    window.localStorage.clear()
  })

  it('declares all 9 valid navigation tabs including chains', () => {
    expect(VALID_TABS).toContain('dashboard')
    expect(VALID_TABS).toContain('subscriptions')
    expect(VALID_TABS).toContain('nodes')
    expect(VALID_TABS).toContain('dns')
    expect(VALID_TABS).toContain('chains')
    expect(VALID_TABS).toContain('policy')
    expect(VALID_TABS).toContain('rules')
    expect(VALID_TABS).toContain('publications')
    expect(VALID_TABS).toContain('settings')
    expect(VALID_TABS.length).toBe(9)
  })

  it('includes chains in MOBILE_MORE_TABS and leaves primary tabs intact', () => {
    expect(MOBILE_PRIMARY_TABS).toEqual(['nodes', 'policy', 'subscriptions'])
    expect(MOBILE_MORE_TABS).toContain('chains')
    expect(isMobileMoreTab('chains')).toBe(true)
    expect(isMobileMoreTab('nodes')).toBe(false)
    expect(isMobileMoreTab('unknown')).toBe(false)
  })

  it('identifies valid tabs accurately', () => {
    expect(isValidTab('chains')).toBe(true)
    expect(isValidTab('dns')).toBe(true)
    expect(isValidTab('settings')).toBe(true)
    expect(isValidTab('probes')).toBe(false)
    expect(isValidTab(null)).toBe(false)
    expect(isValidTab(undefined)).toBe(false)
  })

  it('detects legacy non-canonical hashes', () => {
    expect(isLegacyHash('#chains')).toBe(true)
    expect(isLegacyHash('#nodes')).toBe(true)
    expect(isLegacyHash('#/chains')).toBe(false)
    expect(isLegacyHash('#/')).toBe(false)
    expect(isLegacyHash('')).toBe(false)
    expect(isLegacyHash(null)).toBe(false)
  })

  it('resolves active tab from window.location.hash or hashProvider', () => {
    expect(getRouteTab('#/chains')).toBe('chains')
    expect(getRouteTab('#chains')).toBe('chains')
    expect(getRouteTab('#/probes')).toBe('nodes')
    expect(getRouteTab('#nodes/probes')).toBe('nodes')
    expect(getRouteTab('#/invalid-tab')).toBe('dashboard')
  })

  it('falls back to storage provider when hash is missing or empty', () => {
    window.location.hash = ''
    const storage: Storage = {
      length: 1,
      clear: () => {},
      getItem: (key: string) => (key === ROUTE_STORAGE_KEY ? 'chains' : null),
      key: () => null,
      removeItem: () => {},
      setItem: () => {},
    }
    expect(getRouteTab('', storage)).toBe('chains')
  })

  it('sets route tab synchronizing canonical hash and localStorage', () => {
    setRouteTab('chains')
    expect(window.location.hash).toBe('#/chains')
    expect(window.localStorage.getItem(ROUTE_STORAGE_KEY)).toBe('chains')
  })
})

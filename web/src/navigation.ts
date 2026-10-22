export const VALID_TABS = [
  'dashboard',
  'subscriptions',
  'nodes',
  'dns',
  'policy',
  'rules',
  'publications',
  'settings',
] as const

export type NavTab = (typeof VALID_TABS)[number]

export const ROUTE_STORAGE_KEY = 'csp_active_tab'

export const MOBILE_PRIMARY_TABS = ['nodes', 'policy', 'subscriptions'] as const
export type MobilePrimaryTab = (typeof MOBILE_PRIMARY_TABS)[number]

export const MOBILE_MORE_TABS = [
  'dashboard',
  'dns',
  'rules',
  'publications',
  'settings',
] as const
export type MobileMoreTab = (typeof MOBILE_MORE_TABS)[number]

export function isValidTab(tab: string | null | undefined): tab is NavTab {
  return tab !== null && tab !== undefined && (VALID_TABS as readonly string[]).includes(tab)
}

export function isMobileMoreTab(tab: string | null | undefined): tab is MobileMoreTab {
  return tab !== null && tab !== undefined && (MOBILE_MORE_TABS as readonly string[]).includes(tab)
}

/**
 * Detects whether a hash is a non-canonical legacy hash (e.g. `#nodes`, `#subscriptions`).
 * Canonical hashes start with `#/` (e.g. `#/nodes`, `#/settings`, `#/`) or are empty.
 */
export function isLegacyHash(hash: string | null | undefined): boolean {
  if (!hash) return false
  const trimmed = hash.trim()
  return trimmed.startsWith('#') && !trimmed.startsWith('#/') && trimmed.length > 1
}

export function getRouteTab(
  hashProvider?: string,
  storageProvider?: Pick<Storage, 'getItem'>
): NavTab {
  if (typeof window !== 'undefined' || hashProvider !== undefined) {
    const rawHash = hashProvider !== undefined ? hashProvider : (window.location?.hash || '')
    const cleanHash = rawHash.replace(/^#\/?/, '').trim().toLowerCase()
    if (cleanHash === 'probes' || cleanHash === 'nodes/probes') {
      return 'nodes'
    }
    if (isValidTab(cleanHash)) {
      return cleanHash
    }
  }

  try {
    const storage = storageProvider ?? (typeof window !== 'undefined' ? window.localStorage : undefined)
    const stored = storage?.getItem(ROUTE_STORAGE_KEY)
    if (stored === 'probes' || stored === 'nodes/probes') {
      return 'nodes'
    }
    if (isValidTab(stored)) {
      return stored
    }
  } catch {
    // Storage access may be blocked in private mode
  }

  return 'dashboard'
}

export function setRouteTab(
  tab: NavTab,
  storageProvider?: Pick<Storage, 'setItem'>
): void {
  if (typeof window !== 'undefined') {
    const canonicalHash = `#/${tab}`
    if (window.location?.hash !== canonicalHash && window.location?.hash !== `#${tab}`) {
      window.location.hash = canonicalHash
    }
  }
  try {
    const storage = storageProvider ?? (typeof window !== 'undefined' ? window.localStorage : undefined)
    storage?.setItem(ROUTE_STORAGE_KEY, tab)
  } catch {
    // Storage access may be blocked in private mode
  }
}

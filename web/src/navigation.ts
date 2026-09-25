export const VALID_TABS = [
  'dashboard',
  'subscriptions',
  'nodes',
  'probes',
  'policy',
  'publications',
  'settings',
] as const

export type NavTab = (typeof VALID_TABS)[number]

export const ROUTE_STORAGE_KEY = 'csp_active_tab'

export function isValidTab(tab: string | null | undefined): tab is NavTab {
  return tab !== null && tab !== undefined && (VALID_TABS as readonly string[]).includes(tab)
}

export function getRouteTab(
  hashProvider?: string,
  storageProvider?: Pick<Storage, 'getItem'>
): NavTab {
  if (typeof window !== 'undefined' || hashProvider !== undefined) {
    const rawHash = hashProvider !== undefined ? hashProvider : (window.location?.hash || '')
    const cleanHash = rawHash.replace(/^#\/?/, '').trim().toLowerCase()
    if (isValidTab(cleanHash)) {
      return cleanHash
    }
  }

  try {
    const storage = storageProvider ?? (typeof window !== 'undefined' ? window.localStorage : undefined)
    const stored = storage?.getItem(ROUTE_STORAGE_KEY)
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
    if (window.location?.hash !== `#${tab}`) {
      window.location.hash = `#${tab}`
    }
  }
  try {
    const storage = storageProvider ?? (typeof window !== 'undefined' ? window.localStorage : undefined)
    storage?.setItem(ROUTE_STORAGE_KEY, tab)
  } catch {
    // Storage access may be blocked in private mode
  }
}

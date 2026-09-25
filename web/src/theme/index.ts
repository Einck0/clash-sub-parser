export const THEME_STORAGE_KEY = 'csp_theme'
export const THEME_NAMES = ['light', 'dark', 'dim', 'cyberpunk', 'cupcake', 'dracula', 'nord'] as const
export type ThemeName = (typeof THEME_NAMES)[number]
export const DEFAULT_THEME: ThemeName = 'dark'

type StorageLike = Pick<Storage, 'getItem' | 'setItem'>
type ThemeStorage = Pick<Storage, 'getItem'>
type DocumentLike = { documentElement: Pick<HTMLElement, 'setAttribute'> }

const browserStorage = (): StorageLike | undefined => {
  if (typeof window === 'undefined') return undefined
  try {
    return window.localStorage
  } catch {
    return undefined
  }
}

export function isThemeName(value: string | null | undefined): value is ThemeName {
  return value !== null && value !== undefined && (THEME_NAMES as readonly string[]).includes(value)
}

export function getStoredTheme(storage: ThemeStorage | undefined = browserStorage()): ThemeName {
  try {
    const stored = storage?.getItem(THEME_STORAGE_KEY)
    return isThemeName(stored) ? stored : DEFAULT_THEME
  } catch {
    return DEFAULT_THEME
  }
}

export function applyTheme(
  theme: ThemeName,
  documentLike: DocumentLike | undefined = typeof document === 'undefined' ? undefined : document,
  storage: Pick<Storage, 'setItem'> | undefined = browserStorage(),
): void {
  documentLike?.documentElement.setAttribute('data-theme', theme)
  try {
    storage?.setItem(THEME_STORAGE_KEY, theme)
  } catch {
    // Browser privacy settings can disable persistence without disabling themes
  }
}

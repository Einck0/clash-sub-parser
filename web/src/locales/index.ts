import { ref, computed } from 'vue'
import { createI18n } from 'vue-i18n'
import { zhCN, enUS, type Locale } from './messages'

export const LOCALE_STORAGE_KEY = 'csp_locale'

function getInitialLocale(): Locale {
  try {
    const stored = typeof window !== 'undefined' ? window.localStorage.getItem(LOCALE_STORAGE_KEY) : null
    if (stored === 'zh-CN' || stored === 'en-US') return stored
  } catch {
    // Storage may be unavailable without preventing an in-memory preference.
  }
  return 'zh-CN'
}

const activeLocaleRef = ref<Locale>(getInitialLocale())
if (typeof document !== 'undefined') document.documentElement.lang = activeLocaleRef.value

export const i18n = createI18n({
  legacy: false,
  locale: activeLocaleRef.value,
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})

export function setLocale(locale: Locale): void {
  activeLocaleRef.value = locale
  ;(i18n.global.locale as any).value = locale
  if (typeof document !== 'undefined') document.documentElement.lang = locale
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, locale)
    } catch {
      // ignore
    }
  }
}

export function getLocale(): Locale {
  return activeLocaleRef.value
}

export const currentLocale = computed(() => activeLocaleRef.value)

export function toggleLocale(): void {
  setLocale(activeLocaleRef.value === 'zh-CN' ? 'en-US' : 'zh-CN')
}

export function t(key: string, named?: Record<string, unknown>): string {
  try {
    return (i18n.global.t as any)(key, named)
  } catch {
    return key
  }
}

export { zhCN, enUS }
export { uiText } from './redesign'
export type { Locale }

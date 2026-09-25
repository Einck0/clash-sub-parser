import { ref, computed } from 'vue'
import { createI18n } from 'vue-i18n'
import { zhCN, enUS, type Locale } from './messages'

export const LOCALE_STORAGE_KEY = 'csp_locale'

function getInitialLocale(): Locale {
  if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
    const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY)
    if (stored === 'zh-CN' || stored === 'en-US') {
      return stored
    }
  }
  return 'en-US'
}

const activeLocaleRef = ref<Locale>(getInitialLocale())

export const i18n = createI18n({
  legacy: false,
  locale: activeLocaleRef.value,
  fallbackLocale: 'en-US',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})

export function setLocale(locale: Locale): void {
  activeLocaleRef.value = locale
  ;(i18n.global.locale as any).value = locale
  if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
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
export type { Locale }

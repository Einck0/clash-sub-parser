import { currentLocale } from './index'

/**
 * Returns either the Chinese or English string based on the active reactive locale.
 * Calling this inside Vue templates or computed properties ensures automatic
 * re-rendering upon language switching without editing global translation dictionaries.
 *
 * @param zh Chinese string
 * @param en English string
 */
export function uiText(zh: string, en: string): string {
  return currentLocale.value === 'en-US' ? en : zh
}

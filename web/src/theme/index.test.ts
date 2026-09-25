import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  DEFAULT_THEME,
  THEME_STORAGE_KEY,
  THEME_NAMES,
  applyTheme,
  getStoredTheme,
  isThemeName,
} from './index'

describe('theme contract', () => {
  afterEach(() => vi.restoreAllMocks())

  it('accepts DaisyUI themes and rejects arbitrary persisted values', () => {
    expect(THEME_NAMES).toEqual(expect.arrayContaining(['light', 'dark', 'dim', 'cyberpunk']))
    expect(isThemeName('cyberpunk')).toBe(true)
    expect(isThemeName('not-a-daisy-theme')).toBe(false)
    expect(getStoredTheme({ getItem: () => 'not-a-daisy-theme' })).toBe(DEFAULT_THEME)
    expect(getStoredTheme({ getItem: () => 'dim' })).toBe('dim')
  })

  it('applies and persists a selected theme through the document root', () => {
    const setAttribute = vi.fn()
    const storage = { setItem: vi.fn() }

    applyTheme('dark', { documentElement: { setAttribute } } as unknown as Document, storage)

    expect(setAttribute).toHaveBeenCalledWith('data-theme', 'dark')
    expect(storage.setItem).toHaveBeenCalledWith(THEME_STORAGE_KEY, 'dark')
  })

  it('falls back safely when browser persistence is unavailable', () => {
    const storage = { getItem: () => { throw new Error('blocked') } }
    expect(getStoredTheme(storage)).toBe(DEFAULT_THEME)
  })
})

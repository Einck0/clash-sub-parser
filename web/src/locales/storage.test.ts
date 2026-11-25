// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'

afterEach(() => { vi.restoreAllMocks(); vi.resetModules() })

describe('locale storage boundaries', () => {
  it('initializes and changes language in memory when the storage property is denied', async () => {
    vi.resetModules()
    vi.spyOn(window, 'localStorage', 'get').mockImplementation(() => { throw new DOMException('Denied', 'SecurityError') })
    const locale = await import('./index')
    expect(locale.getLocale()).toBe('zh-CN')
    expect(() => locale.setLocale('en-US')).not.toThrow()
    expect(locale.getLocale()).toBe('en-US')
    expect(document.documentElement.lang).toBe('en-US')
    expect(locale.uiText('中文', 'English')).toBe('English')
  })

  it('survives denied storage methods and still updates the document language', async () => {
    vi.resetModules()
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('Unavailable') })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Unavailable') })
    const locale = await import('./index')
    locale.setLocale('en-US')
    locale.toggleLocale()
    expect(locale.getLocale()).toBe('zh-CN')
    expect(document.documentElement.lang).toBe('zh-CN')
  })
})

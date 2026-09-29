import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { generateUUID } from './uuid'

const UUID_V4_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

describe('generateUUID RFC 4122 v4 generator with insecure HTTP fallbacks', () => {
  const originalCrypto = globalThis.crypto

  afterEach(() => {
    Object.defineProperty(globalThis, 'crypto', {
      value: originalCrypto,
      writable: true,
      configurable: true,
    })
    vi.restoreAllMocks()
  })

  it('generates compliant UUID v4 using native crypto.randomUUID when available', () => {
    const uuid = generateUUID()
    expect(uuid).toMatch(UUID_V4_REGEX)
    expect(uuid.charAt(14)).toBe('4')
    expect(['8', '9', 'a', 'b', 'A', 'B']).toContain(uuid.charAt(19))
  })

  it('generates compliant UUID v4 when crypto.randomUUID is undefined (HTTP LAN fallback via getRandomValues)', () => {
    // Simulate non-secure context where randomUUID is not exposed
    const mockCrypto = {
      getRandomValues: (arr: Uint8Array) => {
        for (let i = 0; i < arr.length; i++) {
          arr[i] = Math.floor(Math.random() * 256)
        }
        return arr
      },
      randomUUID: undefined,
    }

    Object.defineProperty(globalThis, 'crypto', {
      value: mockCrypto,
      writable: true,
      configurable: true,
    })

    for (let i = 0; i < 50; i++) {
      const uuid = generateUUID()
      expect(uuid).toMatch(UUID_V4_REGEX)
      expect(uuid.charAt(14)).toBe('4')
      expect(['8', '9', 'a', 'b']).toContain(uuid.charAt(19).toLowerCase())
    }
  })

  it('generates compliant UUID v4 when window.crypto is entirely absent or throws', () => {
    // Simulate legacy/restricted engine where globalThis.crypto is undefined
    Object.defineProperty(globalThis, 'crypto', {
      value: undefined,
      writable: true,
      configurable: true,
    })

    for (let i = 0; i < 50; i++) {
      const uuid = generateUUID()
      expect(uuid).toMatch(UUID_V4_REGEX)
      expect(uuid.charAt(14)).toBe('4')
      expect(['8', '9', 'a', 'b']).toContain(uuid.charAt(19).toLowerCase())
    }
  })

  it('generates distinct IDs on consecutive invocations across fallbacks', () => {
    const set = new Set<string>()
    for (let i = 0; i < 100; i++) {
      set.add(generateUUID())
    }
    expect(set.size).toBe(100)
  })
})

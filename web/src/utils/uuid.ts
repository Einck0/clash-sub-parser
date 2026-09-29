/**
 * Generates an RFC 4122 version 4 UUID.
 *
 * In secure contexts (HTTPS / localhost), it leverages native `crypto.randomUUID()`.
 * In insecure contexts (e.g. plain HTTP LAN access like http://192.168.1.100:18080),
 * `crypto.randomUUID` is undefined in modern browsers.
 * This function provides a robust, compliant fallback using `crypto.getRandomValues`
 * or pseudo-random entropy, ensuring valid UUID v4 output without unhandled runtime exceptions.
 */
export function generateUUID(): string {
  // 1. Native crypto.randomUUID (available in secure contexts)
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    try {
      return crypto.randomUUID()
    } catch {
      // fallback if invocation fails
    }
  }

  // 2. crypto.getRandomValues fallback (often available even in HTTP contexts in some engines)
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    try {
      const bytes = new Uint8Array(16)
      crypto.getRandomValues(bytes)
      // Set version to 0100 (v4)
      bytes[6] = (bytes[6] & 0x0f) | 0x40
      // Set variant to 10xx (RFC 4122)
      bytes[8] = (bytes[8] & 0x3f) | 0x80
      const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
      return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20, 32)}`
    } catch {
      // fallback if getRandomValues fails
    }
  }

  // 3. Mathematical pseudo-random entropy fallback (strictly RFC 4122 v4 formatted)
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

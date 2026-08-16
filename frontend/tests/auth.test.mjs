import assert from 'node:assert/strict'
import test from 'node:test'


test('auth session keeps the raw token in memory only', async () => {
  const writes = []
  globalThis.localStorage = {
    getItem() {
      throw new Error('localStorage must not be read for auth tokens')
    },
    setItem(...args) {
      writes.push(args)
    },
    removeItem(...args) {
      writes.push(args)
    },
  }

  const auth = await import(new URL('../src/auth.ts?auth-regression', import.meta.url))
  auth.setAuthToken('secret-token')

  assert.equal(auth.getAuthToken(), 'secret-token')
  assert.deepEqual(auth.authHeaders(), { 'X-Clash-Token': 'secret-token' })
  assert.deepEqual(writes, [])

  auth.setAuthToken('')
  assert.equal(auth.getAuthToken(), '')
  assert.deepEqual(writes, [])
})

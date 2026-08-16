import assert from 'node:assert/strict'
import test from 'node:test'

const { buildErrorMessage, parseResponse } = await import('../src/api/response.ts?api-response-tests')


test('keeps malformed JSON response bodies available to callers', async () => {
  const response = new Response('<html>upstream failure</html>', {
    status: 502,
    headers: { 'content-type': 'application/json' },
  })

  assert.equal(await parseResponse(response), '<html>upstream failure</html>')
})


test('formats structured API error details without object coercion', () => {
  assert.equal(
    buildErrorMessage({ detail: { code: 'invalid_target' } }, 'fallback'),
    '{"code":"invalid_target"}',
  )
})

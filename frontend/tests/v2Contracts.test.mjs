import assert from 'node:assert/strict'
import test from 'node:test'

test('v2 types structure test', () => {
  const mockNode = {
    logical_id: 'node-001',
    name: 'JP Node',
    protocol: 'shadowsocks',
    server: 'jp.example.com',
    port: 8388,
    lifecycle_state: 'active',
  }
  assert.equal(mockNode.logical_id, 'node-001')
  assert.equal(mockNode.lifecycle_state, 'active')
})

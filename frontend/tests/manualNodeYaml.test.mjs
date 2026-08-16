import assert from 'node:assert/strict'
import test from 'node:test'

import { parseManualNodeYaml, serializeManualNode } from '../src/utils/manualNodeYaml.js'

test('edits a manual node without dropping nested protocol fields', () => {
  const original = {
    name: '香港入口',
    type: 'vless',
    server: 'old.example.com',
    port: 443,
    uuid: '11111111-1111-1111-1111-111111111111',
    'reality-opts': {
      'public-key': 'public-key',
      'short-id': 'a1b2c3d4',
    },
    'ws-opts': {
      path: '/socket',
      headers: { Host: 'cdn.example.com' },
    },
  }

  const edited = parseManualNodeYaml(
    serializeManualNode(original).replace('server: old.example.com', 'server: new.example.com'),
  )

  assert.equal(edited.server, 'new.example.com')
  assert.deepEqual(edited['reality-opts'], original['reality-opts'])
  assert.deepEqual(edited['ws-opts'], original['ws-opts'])
})

test('rejects a manual node document without a name', () => {
  assert.throws(
    () => parseManualNodeYaml('type: ss\nserver: example.com\nport: 443\n'),
    /节点名称/,
  )
})

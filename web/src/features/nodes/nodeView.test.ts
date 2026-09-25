import { describe, expect, it } from 'vitest'
import {
  maskSecretReference,
  normalizeNode,
  nodeCapabilityLabel,
  type NodeRecord,
} from './nodeView'
import { subscriptionPatchPayload } from '../subscriptions/useSubscriptions'

describe('node view helpers', () => {
  it('masks secret references without changing a safe node display', () => {
    expect(maskSecretReference('secret://nodes/abc?token=hidden')).toBe('***')
    expect(maskSecretReference('')).toBe('***')

    const node = normalizeNode({
      logical_id: 'node-1',
      protocol: 'vless',
      display_name: ' Tokyo 01 ',
      active: true,
    })
    expect(node).toMatchObject({
      logicalId: 'node-1',
      displayName: 'Tokyo 01',
      protocol: 'vless',
      active: true,
    })
  })

  it('renders probe status as a stable capability label', () => {
    const node: NodeRecord = {
      logical_id: 'node-2',
      protocol: 'ss',
      display_name: 'Seoul',
      active: false,
      capabilities: { streaming: 'available', ai: 'restricted' },
    }
    expect(nodeCapabilityLabel(node, 'streaming')).toEqual({ label: 'Available', tone: 'success' })
    expect(nodeCapabilityLabel(node, 'ai')).toEqual({ label: 'Restricted', tone: 'warning' })
    expect(nodeCapabilityLabel(node, 'geo')).toEqual({ label: 'Unknown', tone: 'info' })
  })

  it('omits a masked source reference from an unchanged edit', () => {
    expect(subscriptionPatchPayload({
      name: 'Renamed',
      source_url_secret_ref: '***',
      enabled: true,
      config: {},
      refresh_policy: {
        interval_seconds: 86400,
        user_agent_policy: 'default',
        timeout_seconds: 30,
        max_response_bytes: 10485760,
      },
    })).not.toHaveProperty('source_url_secret_ref')
  })
})

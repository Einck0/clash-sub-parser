// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  SUPPORTED_NODE_PROTOCOLS,
  maskSecretReference,
  normalizeNode,
  nodeCapabilityLabel,
  protocolSupportedTargets,
  renderSafeNodePreview,
  validateNodeConnectionProfile,
  type NodeRecord,
} from './nodeView'
import { subscriptionPatchPayload } from '../subscriptions/useSubscriptions'
import { api } from '../../api/client'

describe('node view helpers & WireGuard / TUIC credential safety', () => {
  it('supports all 7 modern protocols and maps target compatibility accurately', () => {
    expect(SUPPORTED_NODE_PROTOCOLS).toEqual([
      'ss',
      'vmess',
      'vless',
      'trojan',
      'hysteria2',
      'wireguard',
      'tuic',
    ])
    expect(protocolSupportedTargets('wireguard')).toEqual(['mihomo', 'singbox'])
    expect(protocolSupportedTargets('tuic')).toEqual(['mihomo', 'singbox'])
    expect(protocolSupportedTargets('vless')).toEqual(['mihomo', 'singbox'])
    expect(protocolSupportedTargets('hysteria2')).toEqual(['mihomo', 'singbox'])
    expect(protocolSupportedTargets('ss')).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
    expect(protocolSupportedTargets('vmess')).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
    expect(protocolSupportedTargets('trojan')).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
  })

  it('masks secret references and returns unavailable connection when node.connection is absent', () => {
    expect(maskSecretReference('secret://nodes/abc?token=hidden')).toBe('***')
    expect(maskSecretReference('')).toBe('***')

    const node = normalizeNode({
      logical_id: 'node-1',
      protocol: 'wireguard',
      display_name: ' Tokyo 01 ',
      active: true,
    })
    expect(node).toMatchObject({
      logicalId: 'node-1',
      displayName: 'Tokyo 01',
      protocol: 'wireguard',
      active: true,
    })
    expect(node.connection.available).toBe(false)
    expect(node.connection.unavailableReason).toBe('credential_unavailable')
    expect(node.connection.server).toBe('')
    expect(node.connection.port).toBe(0)
    expect(node.connection.localAddress).toEqual([])
    expect(node.connection.publicKey).toBe('')
    expect(node.connection.hasPrivateKey).toBe(false)
    expect(node.connection.hasPreSharedKey).toBe(false)
    expect(node.connection.hasPassword).toBe(false)
    expect(JSON.stringify(node)).not.toContain('.edge.internal')
    expect(renderSafeNodePreview(node, 'mihomo')).toContain(
      '# Connection details unavailable (credential_unavailable)'
    )
  })

  it('maps API node.connection projection and sanitizes credentials without retaining plaintext secrets', () => {
    const wgNode = normalizeNode({
      logical_id: 'node-wg-jp',
      protocol: 'wireguard',
      display_name: 'JP WireGuard 01',
      active: true,
      credential_version: 2,
      connection: {
        available: true,
        server: '198.51.100.10',
        port: 51820,
        local_address: ['10.0.0.2/32', 'fd00::2/128'],
        public_key: 'peer-pub-key-base64',
        mtu: 1400,
        dns: ['1.1.1.1', '8.8.8.8'],
        reserved: [1, 2, 3],
        has_private_key: true,
        has_pre_shared_key: true,
        has_password: false,
      },
      credentials: {
        private_key: 'NEVER-EXPOSE-WG-PRIVATE-KEY',
        pre_shared_key: 'NEVER-EXPOSE-WG-PSK',
      },
    })

    expect(wgNode.connection.available).toBe(true)
    expect(wgNode.connection.localAddress).toEqual(['10.0.0.2/32', 'fd00::2/128'])
    expect(wgNode.connection.publicKey).toBe('peer-pub-key-base64')
    expect(wgNode.connection.mtu).toBe(1400)
    expect(wgNode.connection.dns).toEqual(['1.1.1.1', '8.8.8.8'])
    expect(wgNode.connection.reserved).toEqual([1, 2, 3])
    expect(wgNode.connection.hasPrivateKey).toBe(true)
    expect(wgNode.connection.hasPreSharedKey).toBe(true)
    expect(wgNode.connection.privateKeyMasked).toBe('***')
    expect(wgNode.connection.preSharedKeyMasked).toBe('***')
    expect(JSON.stringify(wgNode)).not.toContain('NEVER-EXPOSE-WG-PRIVATE-KEY')
    expect(JSON.stringify(wgNode)).not.toContain('NEVER-EXPOSE-WG-PSK')

    const tuicNode = normalizeNode({
      logical_id: 'node-tuic-sg',
      protocol: 'tuic',
      display_name: 'SG TUIC 01',
      active: true,
      credential_version: 1,
      connection: {
        available: true,
        server: '198.51.100.11',
        port: 8443,
        uuid: '11111111-2222-4333-8444-555555555555',
        congestion_control: 'bbr',
        udp_relay_mode: 'quic',
        alpn: ['h3'],
        sni: 'tuic.sg.example.com',
        disable_sni: false,
        has_private_key: false,
        has_pre_shared_key: false,
        has_password: true,
      },
      credentials: {
        password: 'NEVER-EXPOSE-TUIC-PASSWORD',
      },
    })

    expect(tuicNode.connection.available).toBe(true)
    expect(tuicNode.connection.uuid).toBe('11111111-2222-4333-8444-555555555555')
    expect(tuicNode.connection.congestionControl).toBe('bbr')
    expect(tuicNode.connection.udpRelayMode).toBe('quic')
    expect(tuicNode.connection.alpn).toEqual(['h3'])
    expect(tuicNode.connection.sni).toBe('tuic.sg.example.com')
    expect(tuicNode.connection.hasPassword).toBe(true)
    expect(tuicNode.connection.passwordMasked).toBe('***')
    expect(JSON.stringify(tuicNode)).not.toContain('NEVER-EXPOSE-TUIC-PASSWORD')

    // Verify Mihomo YAML and sing-box JSON previews redact secrets while showing non-secret fields
    const wgYaml = renderSafeNodePreview(wgNode, 'mihomo')
    expect(wgYaml).toContain('type: wireguard')
    expect(wgYaml).toContain('ip: 10.0.0.2/32')
    expect(wgYaml).toContain('ipv6: fd00::2/128')
    expect(wgYaml).toContain('public-key: peer-pub-key-base64')
    expect(wgYaml).toContain('private-key: ***')
    expect(wgYaml).toContain('pre-shared-key: ***')
    expect(wgYaml).not.toContain('NEVER-EXPOSE')

    const tuicJson = renderSafeNodePreview(tuicNode, 'singbox')
    expect(tuicJson).toContain('"type": "tuic"')
    expect(tuicJson).toContain('"uuid": "11111111-2222-4333-8444-555555555555"')
    expect(tuicJson).toContain('"congestion_control": "bbr"')
    expect(tuicJson).toContain('"udp_relay_mode": "quic"')
    expect(tuicJson).toContain('"password": "***"')
    expect(tuicJson).not.toContain('NEVER-EXPOSE')
  })

  it('validates WireGuard and TUIC required connection fields', () => {
    expect(
      validateNodeConnectionProfile('wireguard', {
        server: '198.51.100.10',
        port: 51820,
        localAddress: [],
        publicKey: 'pub-key',
        hasPrivateKey: true,
      })
    ).toContain('local_address')

    expect(
      validateNodeConnectionProfile('wireguard', {
        server: '198.51.100.10',
        port: 51820,
        localAddress: ['10.0.0.2/32'],
        publicKey: '',
        hasPrivateKey: true,
      })
    ).toContain('public_key')

    expect(
      validateNodeConnectionProfile('wireguard', {
        server: '198.51.100.10',
        port: 51820,
        localAddress: ['10.0.0.2/32'],
        publicKey: 'pub-key',
        hasPrivateKey: false,
      })
    ).toContain('private_key')

    expect(
      validateNodeConnectionProfile('tuic', {
        server: '198.51.100.11',
        port: 8443,
        uuid: '',
        hasPassword: true,
      })
    ).toContain('uuid')

    expect(
      validateNodeConnectionProfile('tuic', {
        server: '198.51.100.11',
        port: 8443,
        uuid: '11111111-2222-4333-8444-555555555555',
        hasPassword: false,
      })
    ).toContain('password')
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
    expect(
      subscriptionPatchPayload({
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
      })
    ).not.toHaveProperty('source_url_secret_ref')
  })
})

describe('NodesView real API detail, CAS PATCH edit/rotation, failure draft preservation, and unavailable state', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    if (typeof globalThis.ResizeObserver === 'undefined') {
      globalThis.ResizeObserver = class {
        observe() {}
        unobserve() {}
        disconnect() {}
      } as any
    }
  })

  it('loads real node.connection from GET detail, sends CAS PATCH with expected_credential_version, rotates write-only secrets, and preserves draft on failure', async () => {
    const { default: NodesView } = await import('./NodesView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const mockItems: NodeRecord[] = [
      {
        logical_id: 'node-wg-1',
        protocol: 'wireguard',
        display_name: 'WG Tokyo Edge',
        active: true,
        credential_version: 2,
      },
      {
        logical_id: 'node-tuic-1',
        protocol: 'tuic',
        display_name: 'TUIC Seoul Edge',
        active: true,
        credential_version: 4,
      },
      {
        logical_id: 'node-unavail-1',
        protocol: 'wireguard',
        display_name: 'WG Missing Creds',
        active: true,
        credential_version: 1,
      },
    ]

    const wgDetailNode: NodeRecord = {
      logical_id: 'node-wg-1',
      protocol: 'wireguard',
      display_name: 'WG Tokyo Edge',
      active: true,
      credential_version: 2,
      connection: {
        available: true,
        server: '198.51.100.10',
        port: 51820,
        local_address: ['10.0.0.2/32'],
        public_key: 'wg-peer-public-key-initial',
        mtu: 1420,
        dns: ['1.1.1.1'],
        reserved: [0, 0, 0],
        has_private_key: true,
        has_pre_shared_key: true,
        has_password: false,
      },
    }

    const tuicDetailNode: NodeRecord = {
      logical_id: 'node-tuic-1',
      protocol: 'tuic',
      display_name: 'TUIC Seoul Edge',
      active: true,
      credential_version: 4,
      connection: {
        available: true,
        server: '198.51.100.11',
        port: 8443,
        uuid: '00000000-0000-4000-8000-000000000077',
        congestion_control: 'bbr',
        udp_relay_mode: 'native',
        alpn: ['h3'],
        sni: 'tuic.kr.example.com',
        disable_sni: false,
        has_private_key: false,
        has_pre_shared_key: false,
        has_password: true,
      },
    }

    const unavailDetailNode: NodeRecord = {
      logical_id: 'node-unavail-1',
      protocol: 'wireguard',
      display_name: 'WG Missing Creds',
      active: true,
      credential_version: 1,
      connection: {
        available: false,
        unavailable_reason: 'missing_verified_identity',
      },
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return {
          items: mockItems,
          page: 1,
          page_size: 100,
          total: 3,
        }
      }
      if (path === '/api/v1/nodes/node-wg-1') {
        return {
          node: wgDetailNode,
          sources: [
            {
              node_logical_id: 'node-wg-1',
              subscription_id: 'sub-upstream-tokyo',
              last_seen_fetch_id: 'fetch-01',
            },
          ],
        }
      }
      if (path === '/api/v1/nodes/node-tuic-1') {
        return {
          node: tuicDetailNode,
          sources: [
            {
              node_logical_id: 'node-tuic-1',
              subscription_id: 'sub-upstream-seoul',
              last_seen_fetch_id: 'fetch-02',
            },
          ],
        }
      }
      if (path === '/api/v1/nodes/node-unavail-1') {
        return {
          node: unavailDetailNode,
          sources: [],
        }
      }
      return {}
    })

    let shouldFailPatch = false
    const patchSpy = vi.spyOn(api, 'patch').mockImplementation(async (path: string, body?: any) => {
      if (shouldFailPatch) {
        throw new Error('409 Conflict: identity_mutation_forbidden')
      }
      if (path === '/api/v1/nodes/node-wg-1/connection') {
        return {
          node: {
            ...wgDetailNode,
            display_name: body.display_name ?? wgDetailNode.display_name,
            credential_version: 3,
            connection: {
              ...wgDetailNode.connection,
              available: true,
              local_address: body.local_address ?? wgDetailNode.connection?.local_address,
              mtu: body.mtu ?? wgDetailNode.connection?.mtu,
              has_private_key: true,
              has_pre_shared_key: true,
            },
          },
          sources: [
            {
              node_logical_id: 'node-wg-1',
              subscription_id: 'sub-upstream-tokyo',
              last_seen_fetch_id: 'fetch-01',
            },
          ],
        }
      }
      throw new Error(`Unexpected PATCH path: ${path}`)
    })

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({
      render() {
        return h(NodesView)
      },
    })
    app.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const cards = mountEl.querySelectorAll('[data-testid="node-card"]')
    expect(cards.length).toBe(3)

    // 1. Inspect & edit WireGuard node via real API detail + CAS PATCH
    ;(cards[0] as HTMLElement).click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')
    expect(drawer).not.toBeNull()
    expect(drawer?.textContent).toContain('WireGuard Endpoint & Peer Configuration')
    expect(drawer?.querySelector('[data-testid="node-reconcile-overwrite-notice"]')?.textContent).toContain(
      'Reconcile'
    )
    expect(drawer?.querySelector('[data-testid="node-provenance-sources"]')?.textContent).toContain(
      'sub-upstream-tokyo'
    )
    expect(drawer?.querySelector('[data-testid="wg-private-key-masked"]')?.textContent).toContain('***')
    expect(drawer?.querySelector('[data-testid="wg-psk-masked"]')?.textContent).toContain('***')

    const wgAddrInput = drawer?.querySelector('[data-testid="wg-local-address-input"]') as HTMLInputElement | null
    const wgMtuInput = drawer?.querySelector('[data-testid="wg-mtu-input"]') as HTMLInputElement | null
    const wgPrivInput = drawer?.querySelector('[data-testid="wg-private-key-input"]') as HTMLInputElement | null
    const wgPskInput = drawer?.querySelector('[data-testid="wg-psk-input"]') as HTMLInputElement | null
    expect(wgAddrInput?.value).toBe('10.0.0.2/32')
    expect(wgPrivInput?.value).toBe('')

    if (wgAddrInput && wgMtuInput && wgPrivInput && wgPskInput) {
      wgAddrInput.value = '10.0.0.9/32, fd00::9/128'
      wgAddrInput.dispatchEvent(new Event('input'))
      wgMtuInput.value = '1380'
      wgMtuInput.dispatchEvent(new Event('input'))
      wgPrivInput.value = 'ROTATED-WG-PRIVATE-KEY-SECRET'
      wgPrivInput.dispatchEvent(new Event('input'))
      wgPskInput.value = 'ROTATED-WG-PSK-SECRET'
      wgPskInput.dispatchEvent(new Event('input'))
    }

    const saveBtn = drawer?.querySelector('[data-testid="node-save-connection-btn"]') as HTMLButtonElement | null
    saveBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    expect(patchSpy).toHaveBeenCalledTimes(1)
    expect(patchSpy).toHaveBeenCalledWith(
      '/api/v1/nodes/node-wg-1/connection',
      expect.objectContaining({
        expected_credential_version: 2,
        local_address: ['10.0.0.9/32', 'fd00::9/128'],
        mtu: 1380,
        private_key_input: 'ROTATED-WG-PRIVATE-KEY-SECRET',
        pre_shared_key_input: 'ROTATED-WG-PSK-SECRET',
      })
    )

    // Write-only inputs are cleared on success and never appear in DOM text
    expect(wgPrivInput?.value).toBe('')
    expect(wgPskInput?.value).toBe('')
    expect(document.body.textContent).not.toContain('ROTATED-WG-PRIVATE-KEY-SECRET')
    expect(document.body.textContent).not.toContain('ROTATED-WG-PSK-SECRET')
    expect(drawer?.querySelector('[data-testid="node-connection-saved"]')?.textContent).toContain('v3')

    const previewEl = drawer?.querySelector('[data-testid="node-config-preview"]')
    expect(previewEl?.textContent).toContain('ip: 10.0.0.9/32')
    expect(previewEl?.textContent).toContain('ipv6: fd00::9/128')
    expect(previewEl?.textContent).toContain('mtu: 1380')
    expect(previewEl?.textContent).toContain('private-key: ***')

    // Switch preview to sing-box JSON
    const sbBtn = drawer?.querySelector('[data-testid="node-preview-target-singbox"]') as HTMLButtonElement | null
    sbBtn?.click()
    await nextTick()
    expect(previewEl?.textContent).toContain('"type": "wireguard"')
    expect(previewEl?.textContent).toContain('"private_key": "***"')

    // 2. Inspect TUIC node and verify PATCH failure preserves uncommitted draft without fake in-memory save
    ;(cards[1] as HTMLElement).click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    expect(drawer?.textContent).toContain('TUIC v5 Connection & QUIC Transport Parameters')
    expect(drawer?.querySelector('[data-testid="tuic-password-masked"]')?.textContent).toContain('***')
    const tuicSniInput = drawer?.querySelector('[data-testid="tuic-sni-input"]') as HTMLInputElement | null
    expect(tuicSniInput?.value).toBe('tuic.kr.example.com')

    shouldFailPatch = true
    if (tuicSniInput) {
      tuicSniInput.value = 'mutated-sni.kr.example.com'
      tuicSniInput.dispatchEvent(new Event('input'))
    }
    saveBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    // Error banner shown, saved indicator hidden, uncommitted draft retained in input, preview still shows committed server state
    expect(drawer?.querySelector('[data-testid="node-connection-error"]')?.textContent).toContain(
      'identity_mutation_forbidden'
    )
    expect(drawer?.querySelector('[data-testid="node-connection-saved"]')).toBeNull()
    expect(tuicSniInput?.value).toBe('mutated-sni.kr.example.com')
    expect(previewEl?.textContent).toContain('tuic.kr.example.com')
    expect(previewEl?.textContent).not.toContain('mutated-sni.kr.example.com')

    // 3. Inspect node with unavailable credentials — no fabricated defaults, editing disabled
    ;(cards[2] as HTMLElement).click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    expect(drawer?.querySelector('[data-testid="node-connection-unavailable"]')?.textContent).toContain(
      'missing_verified_identity'
    )
    expect(saveBtn?.disabled).toBe(true)
    expect(previewEl?.textContent).toContain(
      '# Connection details unavailable (missing_verified_identity)'
    )
    expect(document.body.textContent).not.toContain('.edge.internal')

    app.unmount()
    mountEl.remove()
  })
})

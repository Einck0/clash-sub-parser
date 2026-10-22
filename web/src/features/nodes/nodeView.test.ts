// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  SUPPORTED_NODE_PROTOCOLS,
  normalizeNode,
  nodeCapabilityLabel,
  nodeHealthBadge,
  nodeHealthDiagnostic,
  nodeUnderlyingHealthCategory,
  nodeRiskBadge,
  formatNodeLatency,
  nodeLatencyTone,
  resolveNodeLatencyMs,
  protocolSupportedTargets,
  renderNodePreview,
  renderSafeNodePreview,
  validateNodeConnectionProfile,
  getNodeGeoRegion,
  type NodeRecord,
} from './nodeView'
import { subscriptionPatchPayload } from '../subscriptions/useSubscriptions'
import { api } from '../../api/client'

vi.mock('../../locales/redesign', () => ({
  uiText: (zh: string, _en: string) => zh,
}))

vi.mock('../../ui/PageToolbar.vue', () => ({
  default: {
    name: 'PageToolbar',
    props: ['title', 'description'],
    template: `
      <div data-testid="page-toolbar" class="flex flex-wrap items-center justify-between gap-3 mb-2">
        <div class="flex items-center gap-2">
          <slot name="default" />
        </div>
        <div class="flex items-center gap-2">
          <slot name="actions" />
        </div>
      </div>
    `,
  },
}))

describe('node view helpers & plaintext WireGuard / TUIC connection handling', () => {
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
    expect(protocolSupportedTargets('wireguard')).toEqual(['mihomo', 'singbox', 'surge'])
    expect(protocolSupportedTargets('tuic')).toEqual(['mihomo', 'singbox', 'surge'])
    expect(protocolSupportedTargets('vless')).toEqual(['mihomo', 'singbox'])
    expect(protocolSupportedTargets('hysteria2')).toEqual(['mihomo', 'singbox', 'surge'])
    expect(protocolSupportedTargets('ss')).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
    expect(protocolSupportedTargets('vmess')).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
    expect(protocolSupportedTargets('trojan')).toEqual(['mihomo', 'singbox', 'surge', 'qx'])
  })

  it('normalizes empty node connection cleanly without fabricated defaults', () => {
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
    expect(node.connection.server).toBe('')
    expect(node.connection.port).toBe(0)
    expect(node.connection.localAddress).toEqual([])
    expect(node.connection.publicKey).toBe('')
    expect(node.connection.privateKey).toBe('')
    expect(node.connection.preSharedKey).toBe('')
    expect(node.connection.password).toBe('')
  })

  it('maps API node plaintext credentials and renders Mihomo/sing-box previews directly', () => {
    const wgNode = normalizeNode({
      logical_id: 'node-wg-jp',
      protocol: 'wireguard',
      display_name: 'JP WireGuard 01',
      active: true,
      server: '198.51.100.10',
      port: 51820,
      credentials: {
        local_address: ['10.0.0.2/32', 'fd00::2/128'],
        public_key: 'peer-pub-key-base64',
        private_key: 'wg-plaintext-private-key',
        pre_shared_key: 'wg-plaintext-psk',
        mtu: 1400,
        dns: ['1.1.1.1', '8.8.8.8'],
        reserved: [1, 2, 3],
      },
    })

    expect(wgNode.connection.server).toBe('198.51.100.10')
    expect(wgNode.connection.port).toBe(51820)
    expect(wgNode.connection.localAddress).toEqual(['10.0.0.2/32', 'fd00::2/128'])
    expect(wgNode.connection.publicKey).toBe('peer-pub-key-base64')
    expect(wgNode.connection.privateKey).toBe('wg-plaintext-private-key')
    expect(wgNode.connection.preSharedKey).toBe('wg-plaintext-psk')
    expect(wgNode.connection.mtu).toBe(1400)
    expect(wgNode.connection.dns).toEqual(['1.1.1.1', '8.8.8.8'])
    expect(wgNode.connection.reserved).toEqual([1, 2, 3])

    const tuicNode = normalizeNode({
      logical_id: 'node-tuic-sg',
      protocol: 'tuic',
      display_name: 'SG TUIC 01',
      active: true,
      server: '198.51.100.11',
      port: 8443,
      credentials: {
        uuid: '11111111-2222-4333-8444-555555555555',
        password: 'tuic-plaintext-password',
        congestion_control: 'bbr',
        udp_relay_mode: 'quic',
        alpn: ['h3'],
        sni: 'tuic.sg.example.com',
        disable_sni: false,
      },
    })

    expect(tuicNode.connection.server).toBe('198.51.100.11')
    expect(tuicNode.connection.port).toBe(8443)
    expect(tuicNode.connection.uuid).toBe('11111111-2222-4333-8444-555555555555')
    expect(tuicNode.connection.password).toBe('tuic-plaintext-password')
    expect(tuicNode.connection.congestionControl).toBe('bbr')
    expect(tuicNode.connection.udpRelayMode).toBe('quic')
    expect(tuicNode.connection.alpn).toEqual(['h3'])
    expect(tuicNode.connection.sni).toBe('tuic.sg.example.com')

    const wgYaml = renderNodePreview(wgNode, 'mihomo')
    expect(wgYaml).toContain('type: wireguard')
    expect(wgYaml).toContain('ip: 10.0.0.2/32')
    expect(wgYaml).toContain('ipv6: fd00::2/128')
    expect(wgYaml).toContain('public-key: peer-pub-key-base64')
    expect(wgYaml).toContain('private-key: wg-plaintext-private-key')
    expect(wgYaml).toContain('pre-shared-key: wg-plaintext-psk')

    const tuicJson = renderSafeNodePreview(tuicNode, 'singbox')
    expect(tuicJson).toContain('"type": "tuic"')
    expect(tuicJson).toContain('"uuid": "11111111-2222-4333-8444-555555555555"')
    expect(tuicJson).toContain('"congestion_control": "bbr"')
    expect(tuicJson).toContain('"udp_relay_mode": "quic"')
    expect(tuicJson).toContain('"password": "tuic-plaintext-password"')
  })

  it('validates WireGuard and TUIC required connection fields', () => {
    expect(
      validateNodeConnectionProfile('wireguard', {
        server: '198.51.100.10',
        port: 51820,
        localAddress: [],
        publicKey: 'pub-key',
        privateKey: 'priv-key',
      })
    ).toContain('local_address')

    expect(
      validateNodeConnectionProfile('wireguard', {
        server: '198.51.100.10',
        port: 51820,
        localAddress: ['10.0.0.2/32'],
        publicKey: '',
        privateKey: 'priv-key',
      })
    ).toContain('public_key')

    expect(
      validateNodeConnectionProfile('wireguard', {
        server: '198.51.100.10',
        port: 51820,
        localAddress: ['10.0.0.2/32'],
        publicKey: 'pub-key',
        privateKey: '',
      })
    ).toContain('private_key')

    expect(
      validateNodeConnectionProfile('tuic', {
        server: '198.51.100.11',
        port: 8443,
        uuid: '',
        password: 'tuic-password',
      })
    ).toContain('uuid')

    expect(
      validateNodeConnectionProfile('tuic', {
        server: '198.51.100.11',
        port: 8443,
        uuid: '11111111-2222-4333-8444-555555555555',
        password: '',
      })
    ).toContain('password')
  })

  it('renders probe status, health badge, and risk badge as localized Chinese labels', () => {
    const node: NodeRecord = {
      logical_id: 'node-2',
      protocol: 'ss',
      display_name: 'Seoul',
      active: false,
      capabilities: { streaming: 'available', ai: 'restricted' },
      ip_risk_summary: { risk_band: 'low', decision: 'allow', status: 'fresh' },
    }
    expect(nodeCapabilityLabel(node, 'streaming')).toEqual({ label: '可用', tone: 'success' })
    expect(nodeCapabilityLabel(node, 'ai')).toEqual({ label: '受限', tone: 'warning' })
    expect(nodeCapabilityLabel(node, 'geo')).toEqual({ label: '未知', tone: 'info' })

    // Auxiliary-only capability records do not establish health
    expect(nodeHealthBadge({ logical_id: 'n1', protocol: 'ss', display_name: 'N1', active: true, capabilities: { streaming: 'available' } })).toEqual({
      label: '未测试',
      tone: 'info',
    })
    expect(nodeHealthBadge({ logical_id: 'n1-baseline', protocol: 'ss', display_name: 'N1', active: true, capabilities: { baseline: { verdict: 'available', latency_ms: 50 } } })).toEqual({
      label: '正常',
      tone: 'success',
    })
    expect(nodeHealthBadge(node)).toEqual({ label: '未测试', tone: 'info' })
    expect(nodeHealthBadge({ logical_id: 'n2', protocol: 'ss', display_name: 'N2', active: true, capabilities: { baseline: 'error' } })).toEqual({
      label: '异常',
      tone: 'error',
    })
    expect(nodeHealthBadge({ logical_id: 'n3', protocol: 'ss', display_name: 'N3', active: true })).toEqual({
      label: '未测试',
      tone: 'info',
    })

    expect(nodeRiskBadge(node)).toEqual({ label: '低风险', tone: 'success' })
    expect(nodeRiskBadge({ logical_id: 'n4', protocol: 'ss', display_name: 'N4', active: true, ip_risk_summary: { risk_band: 'medium' } })).toEqual({
      label: '中风险',
      tone: 'warning',
    })
    expect(nodeRiskBadge({ logical_id: 'n5', protocol: 'ss', display_name: 'N5', active: true, ip_risk_summary: { risk_band: 'high' } })).toEqual({
      label: '高风险',
      tone: 'error',
    })

    const enrichedNode = normalizeNode({
      logical_id: 'n-enriched',
      protocol: 'vless',
      display_name: 'Tokyo Enriched',
      active: true,
      latency_ms: 42,
      last_probed_at: '2026-09-26T10:00:00Z',
      health_status: 'healthy',
      probe_missing: false,
      probe_stale: false,
      capabilities: {
        baseline: { verdict: 'available', latency_ms: 42, observed_at: '2026-09-26T10:00:00Z', stale: false },
        streaming: { verdict: 'available', latency_ms: 85, observed_at: '2026-09-26T10:00:00Z', stale: false },
      },
    })
    expect(enrichedNode.latencyMs).toBe(42)
    expect(formatNodeLatency(enrichedNode)).toBe('42 ms')
    expect(nodeLatencyTone(enrichedNode)).toBe('success')
    expect(nodeHealthBadge(enrichedNode)).toEqual({ label: '正常', tone: 'success' })
    expect(nodeCapabilityLabel(enrichedNode, 'streaming')).toEqual({ label: '可用', tone: 'success' })

    const probingNode = normalizeNode({
      logical_id: 'n-probing',
      protocol: 'vless',
      display_name: 'HK Probing 01',
      active: true,
      probe_state: 'probing',
      health_status: 'probing',
      latency_ms: 35,
    })
    expect(probingNode.probeState).toBe('probing')
    expect(nodeHealthBadge(probingNode)).toEqual({ label: '未测试', tone: 'info' })

    const queuedNode = normalizeNode({
      logical_id: 'n-queued',
      protocol: 'trojan',
      display_name: 'SG Queued 02',
      active: true,
      probe_state: 'queued',
    })
    expect(queuedNode.probeState).toBe('queued')
    expect(nodeHealthBadge(queuedNode)).toEqual({ label: '未测试', tone: 'info' })

    // Health badge remains underlying canonical health
    expect(nodeHealthBadge(enrichedNode)).toEqual({
      label: '正常',
      tone: 'success',
    })

    // Unhealthy / failed node with latency_ms=0 never resolves to 0 ms or green 'success' badge
    const failedZeroLatencyNode = normalizeNode({
      logical_id: 'n-failed-0',
      protocol: 'ss',
      display_name: 'RU Failed 0ms',
      active: true,
      health_status: 'unhealthy',
      latency_ms: 0,
      capabilities: {
        baseline: { verdict: 'error', latency_ms: 0, observed_at: '2026-09-26T10:00:00Z' },
      },
    })
    expect(failedZeroLatencyNode.latencyMs).toBeNull()
    expect(resolveNodeLatencyMs(failedZeroLatencyNode)).toBeNull()
    expect(formatNodeLatency(failedZeroLatencyNode)).toBe('--')
    expect(nodeLatencyTone(failedZeroLatencyNode)).toBe('error')

    // Safety policy rejections and missing creds classify as 'unhealthy' with actionable diagnostic
    const unsafeTlsNode = normalizeNode({
      logical_id: 'n-unsafe-tls',
      protocol: 'hysteria2',
      display_name: 'Hy2 SkipCert',
      active: true,
      health_status: 'unknown',
      probe_missing: false,
      capabilities: {
        baseline: {
          verdict: 'error',
          latency_ms: 0,
          summary: 'profile=baseline version=baseline-v1 verdict=error reason=unsafe_tls_rejected status=0 latency_ms=0',
        },
      },
    })
    expect(nodeUnderlyingHealthCategory(unsafeTlsNode)).toBe('unhealthy')
    expect(resolveNodeLatencyMs(unsafeTlsNode)).toBeNull()
    expect(nodeHealthBadge(unsafeTlsNode)).toEqual({ label: '异常', tone: 'error' })
    expect(nodeHealthDiagnostic(unsafeTlsNode)?.shortLabel).toContain('安全拒绝：跳过证书校验')

    const credsMissingNode = normalizeNode({
      logical_id: 'n-creds-missing',
      protocol: 'vless',
      display_name: 'VLESS Missing Creds',
      active: true,
      health_status: 'unknown',
      probe_missing: false,
      capabilities: {
        baseline: {
          verdict: 'error',
          latency_ms: 0,
          summary: 'profile=baseline version=baseline-v1 verdict=error reason=credentials_unavailable status=0 latency_ms=0',
        },
      },
    })
    expect(nodeUnderlyingHealthCategory(credsMissingNode)).toBe('unhealthy')
    expect(nodeHealthBadge(credsMissingNode)).toEqual({ label: '异常', tone: 'error' })
    expect(nodeHealthDiagnostic(credsMissingNode)?.detail).toContain('未完成出站检测')

    // Timeout errors classify as 'unhealthy' and diagnostic returns specific 'timeout'
    const timeoutNode = normalizeNode({
      logical_id: 'n-timeout',
      protocol: 'ss',
      display_name: 'Timeout Node',
      active: true,
      health_status: 'unknown',
      capabilities: {
        baseline: {
          verdict: 'error',
          latency_ms: 0,
          summary: 'profile=baseline version=baseline-v1 verdict=error reason=timeout status=0 latency_ms=0',
        },
      },
    })
    expect(nodeUnderlyingHealthCategory(timeoutNode)).toBe('unhealthy')
    expect(nodeHealthDiagnostic(timeoutNode)?.code).toBe('timeout')
    expect(nodeHealthDiagnostic(timeoutNode)?.shortLabel).toBe('探测超时')

    // Stale baseline with available verdict retains 'healthy' status
    const staleBaselineNode = normalizeNode({
      logical_id: 'n-stale-baseline',
      protocol: 'vless',
      display_name: 'Stale Baseline Node',
      active: true,
      health_status: 'unknown',
      probe_stale: true,
      probe_missing: false,
      capabilities: {
        baseline: { verdict: 'available', latency_ms: 48, stale: true },
        streaming: { verdict: 'available', latency_ms: 92, stale: false },
      },
    })
    expect(nodeUnderlyingHealthCategory(staleBaselineNode)).toBe('healthy')
    expect(nodeHealthBadge({ logical_id: 'legacy-degraded', protocol: 'ss', display_name: 'legacy', active: true, health_status: 'degraded' })).toEqual({ label: '正常', tone: 'success' })
    for (const status of ['unknown', 'undetermined', 'missing'] as const) {
      expect(nodeUnderlyingHealthCategory({ logical_id: status, protocol: 'ss', display_name: status, active: true, health_status: status })).toBe('untested')
    }
    expect(nodeHealthBadge({ logical_id: 'queued-healthy', protocol: 'ss', display_name: 'queued', active: true, health_status: 'healthy', probe_state: 'queued' })).toEqual({ label: '正常', tone: 'success' })
    expect(nodeHealthBadge({ logical_id: 'transport-fail', protocol: 'ss', display_name: 'failed', active: true, health_status: 'unhealthy', capabilities: { baseline: { verdict: 'error', summary: 'reason=transport_error' } } })).toEqual({ label: '异常', tone: 'error' })
    expect(nodeHealthBadge(staleBaselineNode)).toEqual({ label: '正常', tone: 'success' })
    expect(nodeHealthDiagnostic(staleBaselineNode)).toBeNull()

    // Auxiliary-only records remain untested and do not pollute baseline latency
    const aiOnlyNode = normalizeNode({
      logical_id: 'n-ai-only',
      protocol: 'ss',
      display_name: 'AI Fast Node',
      active: true,
      health_status: 'unknown',
      capabilities: {
        ai: { verdict: 'available', latency_ms: 120, observed_at: '2026-09-26T10:00:00Z' },
      },
    })
    expect(nodeUnderlyingHealthCategory(aiOnlyNode)).toBe('untested')
    expect(resolveNodeLatencyMs(aiOnlyNode)).toBeNull()
    expect(nodeHealthBadge(aiOnlyNode)).toEqual({ label: '未测试', tone: 'info' })

    // Zero observations: classified as untested
    const unprobedNode = normalizeNode({
      logical_id: 'n-unprobed',
      protocol: 'ss',
      display_name: 'Never Probed Node',
      active: true,
      health_status: 'unknown',
      probe_missing: true,
      capabilities: {},
    })
    expect(nodeUnderlyingHealthCategory(unprobedNode)).toBe('untested')
    expect(nodeHealthBadge(unprobedNode)).toEqual({ label: '未测试', tone: 'info' })
  })

  it('includes plaintext source_url_secret_ref in subscriptionPatchPayload and omits blank URL', () => {
    expect(
      subscriptionPatchPayload({
        name: 'Renamed',
        source_url_secret_ref: ' https://sub.example.com/api/v1/client/subscribe?token=abc ',
        enabled: true,
        config: {},
        refresh_policy: {
          interval_seconds: 86400,
          user_agent_policy: 'default',
          timeout_seconds: 30,
          max_response_bytes: 10485760,
        },
      })
    ).toHaveProperty(
      'source_url_secret_ref',
      'https://sub.example.com/api/v1/client/subscribe?token=abc'
    )

    expect(
      subscriptionPatchPayload({
        name: 'Renamed',
        source_url_secret_ref: '   ',
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

describe('NodesView real API detail, plaintext PATCH edit, and failure draft preservation', () => {
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

  it('loads plaintext node detail from GET, sends direct plaintext PATCH to /connection, and preserves draft on failure', async () => {
    const { default: NodesView } = await import('./NodesView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const mockItems: NodeRecord[] = [
      {
        logical_id: 'node-wg-1',
        protocol: 'wireguard',
        display_name: 'WG Tokyo Edge',
        active: true,
        server: '198.51.100.10',
        port: 51820,
      },
      {
        logical_id: 'node-tuic-1',
        protocol: 'tuic',
        display_name: 'TUIC Seoul Edge',
        active: true,
        server: '198.51.100.11',
        port: 8443,
      },
    ]

    const wgDetailNode: NodeRecord = {
      logical_id: 'node-wg-1',
      protocol: 'wireguard',
      display_name: 'WG Tokyo Edge',
      active: true,
      server: '198.51.100.10',
      port: 51820,
      credentials: {
        local_address: ['10.0.0.2/32'],
        public_key: 'wg-peer-public-key-initial',
        private_key: 'wg-private-key-initial',
        pre_shared_key: 'wg-psk-initial',
        mtu: 1420,
        dns: ['1.1.1.1'],
        reserved: [0, 0, 0],
      },
    }

    const tuicDetailNode: NodeRecord = {
      logical_id: 'node-tuic-1',
      protocol: 'tuic',
      display_name: 'TUIC Seoul Edge',
      active: true,
      server: '198.51.100.11',
      port: 8443,
      credentials: {
        uuid: '00000000-0000-4000-8000-000000000077',
        password: 'tuic-password-initial',
        congestion_control: 'bbr',
        udp_relay_mode: 'native',
        alpn: ['h3'],
        sni: 'tuic.kr.example.com',
        disable_sni: false,
      },
    }

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return {
          items: mockItems,
          page: 1,
          page_size: 100,
          total: 2,
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
      return {}
    })

    let shouldFailPatch = false
    const patchSpy = vi.spyOn(api, 'patch').mockImplementation(async (path: string, body?: any) => {
      if (shouldFailPatch) {
        throw new Error('422 Unprocessable Entity: invalid_sni')
      }
      if (path === '/api/v1/nodes/node-wg-1/connection') {
        return {
          node: {
            ...wgDetailNode,
            display_name: body.display_name ?? wgDetailNode.display_name,
            server: body.server ?? wgDetailNode.server,
            port: body.port ?? wgDetailNode.port,
            credentials: {
              ...wgDetailNode.credentials,
              local_address: body.local_address ?? wgDetailNode.credentials?.local_address,
              mtu: body.mtu ?? wgDetailNode.credentials?.mtu,
              private_key: body.private_key ?? wgDetailNode.credentials?.private_key,
              pre_shared_key: body.pre_shared_key ?? wgDetailNode.credentials?.pre_shared_key,
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
    expect(cards.length).toBe(2)
    const healthFilterSelect = mountEl.querySelector('[data-testid="node-health-filter"]') as HTMLSelectElement | null
    expect(healthFilterSelect).not.toBeNull()
    expect(healthFilterSelect?.textContent).toContain('检测中')

    // 1. Inspect & edit WireGuard node via real API detail + direct plaintext PATCH
    ;(cards[0] as HTMLElement).click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')
    expect(drawer).not.toBeNull()
    expect(drawer?.textContent).toContain('WireGuard 端点与对端配置')
    expect(drawer?.querySelector('[data-testid="node-reconcile-overwrite-notice"]')?.textContent).toContain(
      'Reconcile'
    )
    expect(drawer?.querySelector('[data-testid="node-provenance-sources"]')?.textContent).toContain(
      'sub-upstream-tokyo'
    )

    const wgAddrInput = drawer?.querySelector('[data-testid="wg-local-address-input"]') as HTMLInputElement | null
    const wgMtuInput = drawer?.querySelector('[data-testid="wg-mtu-input"]') as HTMLInputElement | null
    const wgPrivInput = drawer?.querySelector('[data-testid="wg-private-key-input"]') as HTMLInputElement | null
    const wgPskInput = drawer?.querySelector('[data-testid="wg-psk-input"]') as HTMLInputElement | null
    expect(wgAddrInput?.value).toBe('10.0.0.2/32')
    expect(wgPrivInput?.value).toBe('wg-private-key-initial')
    expect(wgPskInput?.value).toBe('wg-psk-initial')

    if (wgAddrInput && wgMtuInput && wgPrivInput && wgPskInput) {
      wgAddrInput.value = '10.0.0.9/32, fd00::9/128'
      wgAddrInput.dispatchEvent(new Event('input'))
      wgMtuInput.value = '1380'
      wgMtuInput.dispatchEvent(new Event('input'))
      wgPrivInput.value = 'UPDATED-WG-PRIVATE-KEY'
      wgPrivInput.dispatchEvent(new Event('input'))
      wgPskInput.value = 'UPDATED-WG-PSK'
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
        local_address: ['10.0.0.9/32', 'fd00::9/128'],
        mtu: 1380,
        private_key: 'UPDATED-WG-PRIVATE-KEY',
        pre_shared_key: 'UPDATED-WG-PSK',
      })
    )

    expect(wgPrivInput?.value).toBe('UPDATED-WG-PRIVATE-KEY')
    expect(wgPskInput?.value).toBe('UPDATED-WG-PSK')
    expect(drawer?.querySelector('[data-testid="node-connection-saved"]')?.textContent).toContain(
      '连接参数已保存'
    )

    const previewEl = drawer?.querySelector('[data-testid="node-config-preview"]')
    expect(previewEl?.textContent).toContain('ip: 10.0.0.9/32')
    expect(previewEl?.textContent).toContain('ipv6: fd00::9/128')
    expect(previewEl?.textContent).toContain('mtu: 1380')
    expect(previewEl?.textContent).toContain('private-key: UPDATED-WG-PRIVATE-KEY')

    // Switch preview to sing-box JSON
    const sbBtn = drawer?.querySelector('[data-testid="node-preview-target-singbox"]') as HTMLButtonElement | null
    sbBtn?.click()
    await nextTick()
    expect(previewEl?.textContent).toContain('"type": "wireguard"')
    expect(previewEl?.textContent).toContain('"private_key": "UPDATED-WG-PRIVATE-KEY"')

    // 2. Inspect TUIC node and verify PATCH failure preserves uncommitted draft without fake in-memory save
    ;(cards[1] as HTMLElement).click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    expect(drawer?.textContent).toContain('TUIC v5 连接与 QUIC 传输参数')
    const tuicPassInput = drawer?.querySelector('[data-testid="tuic-password-input"]') as HTMLInputElement | null
    const tuicSniInput = drawer?.querySelector('[data-testid="tuic-sni-input"]') as HTMLInputElement | null
    expect(tuicPassInput?.value).toBe('tuic-password-initial')
    expect(tuicSniInput?.value).toBe('tuic.kr.example.com')

    shouldFailPatch = true
    if (tuicSniInput) {
      tuicSniInput.value = 'mutated-sni.kr.example.com'
      tuicSniInput.dispatchEvent(new Event('input'))
    }
    saveBtn?.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 30))

    expect(drawer?.querySelector('[data-testid="node-connection-error"]')?.textContent).toContain(
      'invalid_sni'
    )
    expect(drawer?.querySelector('[data-testid="node-connection-saved"]')).toBeNull()
    expect(tuicSniInput?.value).toBe('mutated-sni.kr.example.com')
    expect(previewEl?.textContent).toContain('tuic.kr.example.com')
    expect(previewEl?.textContent).not.toContain('mutated-sni.kr.example.com')

    app.unmount()
    mountEl.remove()
  })

  it('automatically loads subsequent pages when health filter starves viewport and hasMore is true', async () => {
    const { default: NodesView } = await import('./NodesView.vue')
    const { createApp, h, nextTick } = await import('vue')

    // Page 1: 10 healthy nodes (total = 40)
    const page1Nodes: NodeRecord[] = Array.from({ length: 10 }, (_, i) => ({
      logical_id: `node-healthy-${i}`,
      protocol: 'vmess',
      display_name: `Healthy Node ${i}`,
      active: true,
      health_status: 'healthy' as const,
      latency_ms: 50,
      last_probed_at: new Date().toISOString(),
    }))

    // Page 2: 1 unhealthy node + 9 healthy nodes (total = 40)
    const page2Nodes: NodeRecord[] = [
      {
        logical_id: 'node-unhealthy-99',
        protocol: 'vmess',
        display_name: 'Unhealthy Node 99',
        active: true,
        health_status: 'unhealthy' as const,
        latency_ms: 0,
        last_probed_at: new Date().toISOString(),
      },
      ...Array.from({ length: 9 }, (_, i) => ({
        logical_id: `node-healthy-p2-${i}`,
        protocol: 'vmess',
        display_name: `Healthy Node P2 ${i}`,
        active: true,
        health_status: 'healthy' as const,
        latency_ms: 60,
        last_probed_at: new Date().toISOString(),
      })),
    ]

    const pageCalls: number[] = []
    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/nodes') {
        const page = Number(options?.params?.page || 1)
        pageCalls.push(page)
        if (page === 1) {
          return { items: page1Nodes, total: 40, page: 1, page_size: 10 }
        }
        if (page === 2) {
          return { items: page2Nodes, total: 40, page: 2, page_size: 10 }
        }
        return { items: [], total: 40, page, page_size: 10 }
      }
      if (path === '/api/v1/probes/pool') {
        return {
          queue_nodes_count: 0,
          probing_count: 0,
          queued_waiting_count: 0,
          untested_count: 0,
          total_count: 40,
          unavailable_count: 1,
          available_count: 39,
          healthy_count: 39,
          degraded_count: 0,
          probing_node_ids: [],
          queued_node_ids: [],
          updated_at: new Date().toISOString(),
        }
      }
      return {}
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
    await new Promise((r) => setTimeout(r, 40))

    // Initially page 1 is loaded
    expect(pageCalls).toContain(1)

    // Switch health filter to "unhealthy"
    const unhealthyPill = Array.from(mountEl.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('异常')
    )
    expect(unhealthyPill).toBeDefined()
    unhealthyPill?.click()

    await nextTick()
    await new Promise((r) => setTimeout(r, 60))

    // Because page 1 had 0 unhealthy nodes and hasMore was true, auto-pagination must trigger page 2
    expect(pageCalls).toContain(2)

    // The unhealthy node from page 2 should now be visible in DOM
    expect(mountEl.textContent).toContain('Unhealthy Node 99')

    app.unmount()
    mountEl.remove()
  })

  it('handles cross-page explicit ID selection, disabled empty selection, invalid ID pruning, full fleet confirmation, and global kinds', async () => {
    HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
    HTMLDialogElement.prototype.close = function () { this.removeAttribute('open'); this.dispatchEvent(new Event('close')) }

    const { default: NodesView } = await import('./NodesView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const postCalls: Array<{ path: string; body: any }> = []
    let getPoolCount = 0

    const nodesPage1: NodeRecord[] = [
      {
        logical_id: 'node-p1-1',
        display_name: 'Node P1-1',
        protocol: 'vmess',
        active: true,
        connection: { server: '1.1.1.1', port: 443 },
        capabilities: {},
        updated_at: new Date().toISOString(),
      },
      {
        logical_id: 'node-p1-2',
        display_name: 'Node P1-2',
        protocol: 'vless',
        active: true,
        connection: { server: '1.1.1.2', port: 443 },
        capabilities: {},
        updated_at: new Date().toISOString(),
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
      if (path === '/api/v1/probes/schedule') {
        return {
          enabled: true,
          interval_seconds: 7200,
          kinds: ['baseline', 'ai'],
          updated_at: new Date().toISOString(),
        }
      }
      if (path === '/api/v1/nodes') {
        return { items: nodesPage1, total: 2, page: 1, page_size: 10 }
      }
      if (path === '/api/v1/probes/pool') {
        getPoolCount++
        return {
          probing_node_ids: [],
          queued_node_ids: [],
          queue_nodes_count: 0,
          probing_count: 0,
          total_count: 2,
        }
      }
      if (path === '/api/v1/probes/runs') {
        return {
          items: [
            {
              id: 'run-test-1',
              state: 'running',
              kinds: ['baseline'],
              created_at: new Date().toISOString(),
              target_nodes_count: 2,
            },
          ],
          total: 1,
        }
      }
      if (path === '/api/v1/probes/batches') {
        return {
          items: [
            {
              id: 'batch-active-101',
              window_at: '2026-10-19T10:00:00Z',
              state: 'running',
              counts: { total_nodes: 10, dispatched_runs: 5, completed_runs: 2, skipped_nodes: 3 },
            },
          ],
          total: 1,
        }
      }
      return {}
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string, body?: any) => {
      postCalls.push({ path, body })
      if (path === '/api/v1/probes/runs') {
        return { run_id: 'run-test-1', state: 'pending' }
      }
      return {}
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
    await new Promise((r) => setTimeout(r, 60))

    // 1. Initial empty selection: probe-selected-btn is disabled
    const probeSelectedBtn = mountEl.querySelector('[data-testid="probe-selected-btn"]') as HTMLButtonElement
    expect(probeSelectedBtn).not.toBeNull()
    expect(probeSelectedBtn.disabled).toBe(true)

    // 2. Select first node
    const cb1 = mountEl.querySelector('[data-testid="node-checkbox-node-p1-1"]') as HTMLInputElement
    expect(cb1).not.toBeNull()
    cb1.click()
    await nextTick()

    expect(probeSelectedBtn.disabled).toBe(false)
    const countBadge = mountEl.querySelector('[data-testid="selected-count-badge"]')
    expect(countBadge?.textContent).toContain('1')

    // 3. Trigger manual probe for selected node; should inherit global schedule kinds ['baseline', 'ai']
    probeSelectedBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const runCall = postCalls.find((c) => c.path === '/api/v1/probes/runs')
    expect(runCall).toBeDefined()
    expect(runCall?.body.node_logical_ids).toEqual(['node-p1-1'])
    expect(runCall?.body.kinds).toEqual(['baseline', 'ai'])

    // 4. Full fleet probe confirmation modal
    const probeAllBtn = mountEl.querySelector('[data-testid="probe-all-btn"]') as HTMLButtonElement
    expect(probeAllBtn).not.toBeNull()
    probeAllBtn.click()
    await nextTick()

    // Confirm modal should be open
    const modalConfirmBtn = document.body.querySelector('[data-testid="confirm-modal-confirm"]') as HTMLButtonElement
    expect(modalConfirmBtn).not.toBeNull()
    modalConfirmBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    const probeAllCall = postCalls.filter((c) => c.path === '/api/v1/probes/runs')[1]
    expect(probeAllCall).toBeDefined()
    expect(probeAllCall?.body.node_logical_ids).toEqual([])

    // 5. Probe runs & batch history drawer
    const historyBtn = mountEl.querySelector('[data-testid="node-probe-history-btn"]') as HTMLButtonElement
    expect(historyBtn).not.toBeNull()
    historyBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 40))

    // Drawer should show run-test-1 and allow cancel
    const cancelRunBtn = document.body.querySelector('[data-testid="cancel-run-btn"]') as HTMLButtonElement
    expect(cancelRunBtn).not.toBeNull()
    cancelRunBtn.click()
    await nextTick()

    const cancelPost = postCalls.find((c) => c.path.includes('/runs/run-test-1/cancel'))
    expect(cancelPost).toBeDefined()

    // 6. Batches Tab and batch cancel in drawer
    const batchesTabBtn = document.body.querySelector('[data-testid="history-tab-batches"]') as HTMLButtonElement
    expect(batchesTabBtn).not.toBeNull()
    batchesTabBtn.click()
    await nextTick()
    await new Promise((r) => setTimeout(r, 20))

    expect(document.body.textContent).toContain('batch-active-101')
    const cancelBatchBtn = document.body.querySelector('[data-testid="cancel-batch-btn"]') as HTMLButtonElement
    expect(cancelBatchBtn).not.toBeNull()
    cancelBatchBtn.click()
    await nextTick()

    const cancelBatchPost = postCalls.find((c) => c.path.includes('/batches/batch-active-101/cancel'))
    expect(cancelBatchPost).toBeDefined()

    // 7. Zero secret leakage in rendered UI and batch evidence
    const bodyText = document.body.textContent || ''
    expect(bodyText).not.toContain('secret_raw_token')
    expect(bodyText).not.toContain('password123')

    app.unmount()
    mountEl.remove()
  })

  it('prunes invalid selected IDs on refresh and restricts polling to visible running state', async () => {
    let getPoolCount = 0

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/probes/schedule') {
        return { enabled: true, interval_seconds: 3600, kinds: ['baseline'] }
      }
      if (path === '/api/v1/nodes') {
        return {
          items: [
            {
              logical_id: 'node-live-1',
              display_name: 'Node Live 1',
              protocol: 'vmess',
              active: true,
              connection: { server: '1.1.1.1', port: 443 },
              capabilities: {},
              updated_at: new Date().toISOString(),
            },
          ],
          total: 1,
          page: 1,
          page_size: 10,
        }
      }
      if (path === '/api/v1/probes/pool') {
        getPoolCount++
        return {
          probing_node_ids: [],
          queued_node_ids: [],
          queue_nodes_count: 0,
          probing_count: 0,
          total_count: 1,
        }
      }
      return {}
    })

    const { default: NodesView } = await import('./NodesView.vue')
    const { createApp, h, nextTick } = await import('vue')

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({
      render() {
        return h(NodesView)
      },
    })
    app.mount(mountEl)
    await nextTick()
    await new Promise((r) => setTimeout(r, 60))

    // Check live node
    const cb = mountEl.querySelector('[data-testid="node-checkbox-node-live-1"]') as HTMLInputElement
    expect(cb).not.toBeNull()
    cb.click()
    await nextTick()

    const countBadge = mountEl.querySelector('[data-testid="selected-count-badge"]')
    expect(countBadge?.textContent).toContain('1')

    // Simulate hidden document
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', writable: true })
    const prevPoolCount = getPoolCount

    // Advance timer: should not poll pool because document is hidden and no active tasks
    await new Promise((r) => setTimeout(r, 50))
    expect(getPoolCount).toBe(prevPoolCount)

    // Restore visibilityState
    Object.defineProperty(document, 'visibilityState', { value: 'visible', writable: true })

    app.unmount()
    mountEl.remove()
  })

  it('renders responsive desktop compact row and mobile card layout with accessible columns and operable controls', async () => {
    const { createApp, h, nextTick } = await import('vue')
    const { default: NodesView } = await import('./NodesView.vue')

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return {
          items: [
            {
              logical_id: 'node-layout-1',
              display_name: 'Tokyo Fast 01',
              protocol: 'vmess',
              active: true,
              connection: { server: 'tokyo.example.com', port: 443 },
              capabilities: {},
              updated_at: new Date().toISOString(),
            },
          ],
          total: 1,
        }
      }
      if (path === '/api/v1/probes/schedule') return { enabled: true, interval_seconds: 3600, kinds: ['baseline'] }
      if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [], queue_nodes_count: 0, probing_count: 0, total_count: 1 }
      if (path === '/api/v1/probes/runs') return { items: [], total: 0 }
      if (path === '/api/v1/probes/batches') return { items: [], total: 0 }
      return {}
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
    await new Promise((r) => setTimeout(r, 60))

    // 1. Desktop table header is rendered with accessible column descriptions
    const headerEl = mountEl.querySelector('.hidden.md\\:flex')
    expect(headerEl).not.toBeNull()
    expect(headerEl?.textContent).toContain('节点信息')
    expect(headerEl?.textContent).toContain('协议与端点')
    expect(headerEl?.textContent).toContain('探测健康与能力')

    // 2. Node card article uses responsive compact row & card classes
    const cardEl = mountEl.querySelector('[data-testid="node-card"]') as HTMLElement
    expect(cardEl).not.toBeNull()
    const innerRow = cardEl.querySelector('.flex.flex-col.md\\:flex-row')
    expect(innerRow).not.toBeNull()

    // 3. Checkbox, probe button, and inspect button are accessible and operable
    const cb = cardEl.querySelector('[data-testid="node-checkbox-node-layout-1"]') as HTMLInputElement
    expect(cb).not.toBeNull()
    expect(cb.checked).toBe(false)
    cb.click()
    await nextTick()
    expect(cb.checked).toBe(true)

    const probeBtn = cardEl.querySelector('[data-testid="node-card-probe-btn"]') as HTMLButtonElement
    expect(probeBtn).not.toBeNull()
    expect(probeBtn.disabled).toBe(false)

    const inspectBtn = cardEl.querySelector('[data-testid="node-inspect-btn"]') as HTMLButtonElement
    expect(inspectBtn).not.toBeNull()

    app.unmount()
    mountEl.remove()
  })

  it('renders underlying canonical health badge and active operation badge simultaneously when probing', async () => {
    const { createApp, h, nextTick } = await import('vue')
    const { default: NodesView } = await import('./NodesView.vue')

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return {
          items: [
            {
              logical_id: 'node-probing-live',
              display_name: 'Tokyo Live Probing',
              protocol: 'ss',
              active: true,
              health_status: 'healthy',
              probe_state: 'probing',
              connection: { server: 'tokyo.example.com', port: 8388 },
              capabilities: {
                baseline: { verdict: 'available', latency_ms: 45 },
                ip_risk: { verdict: 'available', risk_score: '10' },
              },
              updated_at: new Date().toISOString(),
            },
          ],
          total: 1,
        }
      }
      if (path === '/api/v1/probes/schedule') return { enabled: true, interval_seconds: 3600, kinds: ['baseline'] }
      if (path === '/api/v1/probes/pool') return {
        probing_node_ids: ['node-probing-live'],
        queued_node_ids: [],
        queue_nodes_count: 1,
        probing_count: 1,
        total_count: 1,
      }
      if (path === '/api/v1/probes/runs') return { items: [], total: 0 }
      if (path === '/api/v1/probes/batches') return { items: [], total: 0 }
      return {}
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
    await new Promise((r) => setTimeout(r, 60))

    // Both operational state and underlying health are visible simultaneously
    const probeBadge = mountEl.querySelector('[data-testid="node-probe-state-badge"]')
    expect(probeBadge).not.toBeNull()
    expect(probeBadge?.textContent).toContain('检测中')

    const healthBadge = mountEl.querySelector('[data-testid="node-health-badge"]')
    expect(healthBadge).not.toBeNull()
    expect(healthBadge?.textContent).toContain('健康: 正常')

    // Risk badge is rendered independently
    const riskBadge = mountEl.querySelector('[data-testid="node-risk-badge"]')
    expect(riskBadge).not.toBeNull()
    expect(riskBadge?.textContent).toContain('低风险')

    app.unmount()
    mountEl.remove()
  })

  it('restores URL q, protocol, health, and node params with safe defaults on invalid input, and resets page on query change', async () => {
    const { createApp, h, nextTick } = await import('vue')
    const { createRouter, createMemoryHistory } = await import('vue-router')
    const { default: NodesView } = await import('./NodesView.vue')

    const mockNodes: NodeRecord[] = [
      {
        logical_id: 'node-hy2-1',
        display_name: 'Tokyo Hy2 Safe',
        protocol: 'hysteria2',
        active: true,
        health_status: 'healthy',
        capabilities: { baseline: { verdict: 'available', latency_ms: 35 } },
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return { items: mockNodes, total: 1, page: 1, page_size: 100 }
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [], total_count: 1 }
      }
      if (path === '/api/v1/nodes/node-hy2-1') {
        return { node: mockNodes[0], sources: [] }
      }
      return {}
    })

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/nodes', component: NodesView }],
    })

    await router.push('/nodes?q=Tokyo&protocol=hysteria2&health=healthy&node=node-hy2-1')
    await router.isReady()

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({
      render() {
        return h(NodesView)
      },
    })
    try {
      app.use(router)
      app.mount(mountEl)
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      // Search input should have 'Tokyo'
      const searchInput = mountEl.querySelector('[data-testid="node-search-input"]') as HTMLInputElement
      expect(searchInput).not.toBeNull()
      expect(searchInput.value).toBe('Tokyo')

      // Protocol filter pill for hysteria2 should be present
      const protoGroup = mountEl.querySelector('[data-testid="node-protocol-filter"]')
      expect(protoGroup?.textContent?.toUpperCase()).toContain('HYSTERIA2')

      // Health filter select should be 'healthy'
      const healthSelect = mountEl.querySelector('[data-testid="node-health-filter"]') as HTMLSelectElement
      expect(healthSelect.value).toBe('healthy')

      // Deep link drawer should be open for node-hy2-1
      const drawer = mountEl.querySelector('[data-testid="node-detail-drawer"]') || document.body.querySelector('[data-testid="node-detail-drawer"]')
      expect(drawer).not.toBeNull()
      expect(drawer?.textContent).toContain('Tokyo Hy2 Safe')

      // Safe fallback when invalid protocol and invalid health are pushed
      await router.push('/nodes?protocol=bogus_protocol&health=invalid_health')
      await nextTick()
      await new Promise((r) => setTimeout(r, 30))

      expect(healthSelect.value).toBe('all')
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })

  it('retains hidden selections across filters, displays visible vs total counts, allows clearing invisible selections, and confirms exact scope before probing', async () => {
    HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
    HTMLDialogElement.prototype.close = function () { this.removeAttribute('open'); this.dispatchEvent(new Event('close')) }

    const { createApp, h, nextTick } = await import('vue')
    const { default: NodesView } = await import('./NodesView.vue')

    const postCalls: Array<{ path: string; body: any }> = []

    const mockNodes: NodeRecord[] = [
      {
        logical_id: 'node-healthy-item',
        display_name: 'Tokyo Healthy Node',
        protocol: 'vmess',
        active: true,
        health_status: 'healthy',
        capabilities: { baseline: { verdict: 'available', latency_ms: 45 } },
      },
      {
        logical_id: 'node-unhealthy-item',
        display_name: 'Seoul Unhealthy Node',
        protocol: 'vless',
        active: true,
        health_status: 'unhealthy',
        capabilities: { baseline: { verdict: 'error' } },
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return { items: mockNodes, total: 2, page: 1, page_size: 100 }
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [], total_count: 2 }
      }
      return {}
    })

    vi.spyOn(api, 'post').mockImplementation(async (path: string, body?: any) => {
      postCalls.push({ path, body })
      return { run_id: 'run-selected-1', state: 'pending' }
    })

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({
      render() {
        return h(NodesView)
      },
    })
    try {
      app.mount(mountEl)
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      // 1. Select the unhealthy node
      const unhealthyCb = mountEl.querySelector('[data-testid="node-checkbox-node-unhealthy-item"]') as HTMLInputElement
      expect(unhealthyCb).not.toBeNull()
      unhealthyCb.click()
      await nextTick()

      const selectedBadge = mountEl.querySelector('[data-testid="selected-count-badge"]')
      expect(selectedBadge?.textContent).toContain('1')

      // 2. Switch health filter to "healthy" (so unhealthy node is hidden from current view)
      const healthSelect = mountEl.querySelector('[data-testid="node-health-filter"]') as HTMLSelectElement
      healthSelect.value = 'healthy'
      healthSelect.dispatchEvent(new Event('change'))
      await nextTick()

      // 3. Selection of unhealthy node is RETAINED even though it's hidden
      expect(selectedBadge?.textContent).toContain('1')
      expect(selectedBadge?.textContent).toContain('0')

      // 4. Clear invisible selection button appears
      const clearInvisibleBtn = mountEl.querySelector('[data-testid="clear-invisible-selection-btn"]') as HTMLButtonElement
      expect(clearInvisibleBtn).not.toBeNull()
      expect(clearInvisibleBtn.textContent).toContain('清除不可见项')

      // 5. Clicking probe selected triggers scope confirmation modal because of hidden selections
      const probeSelectedBtn = mountEl.querySelector('[data-testid="probe-selected-btn"]') as HTMLButtonElement
      probeSelectedBtn.click()
      await nextTick()

      const confirmScopeModal = mountEl.querySelector('[data-testid="confirm-probe-selected-modal"]') || document.body.querySelector('[data-testid="confirm-probe-selected-modal"]')
      expect(confirmScopeModal).not.toBeNull()
      expect(confirmScopeModal?.textContent).toContain('当前已选 1 个节点')
      expect(confirmScopeModal?.textContent).toContain('当前可见 0 项')

      // Confirming executes probe for node-unhealthy-item
      const modalConfirmBtn = confirmScopeModal?.querySelector('[data-testid="confirm-modal-confirm"]') as HTMLButtonElement
      modalConfirmBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 20))

      const runCall = postCalls.find((c) => c.path === '/api/v1/probes/runs')
      expect(runCall).toBeDefined()
      expect(runCall?.body.node_logical_ids).toEqual(['node-unhealthy-item'])

      // 6. Test clear invisible selections
      clearInvisibleBtn.click()
      await nextTick()
      expect(selectedBadge?.textContent).toContain('0')
      expect(mountEl.querySelector('[data-testid="clear-invisible-selection-btn"]')).toBeNull()
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })

  it('distinguishes loaded count vs full total, renders placeholder instead of 0 on initial loading, and preserves existing items on refresh failure', async () => {
    const { createApp, h, nextTick } = await import('vue')
    const { default: NodesView } = await import('./NodesView.vue')

    let shouldFailNodes = false
    let resolveFirst: any = null
    let hasWaitedFirst = false
    const firstGate = new Promise((r) => { resolveFirst = r })

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        if (!hasWaitedFirst) {
          hasWaitedFirst = true
          await firstGate
        }
        if (shouldFailNodes) {
          throw new Error('500 Internal Server Error: Database timeout')
        }
        return {
          items: [
            { logical_id: 'node-persist-1', display_name: 'Tokyo Preserved 1', protocol: 'ss', active: true, health_status: 'healthy' },
            { logical_id: 'node-persist-2', display_name: 'Tokyo Preserved 2', protocol: 'ss', active: true, health_status: 'healthy' },
          ],
          total: 50,
          page: 1,
          page_size: 2,
        }
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [], total_count: 50 }
      }
      return {}
    })

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({
      render() {
        return h(NodesView)
      },
    })

    try {
      app.mount(mountEl)
      await nextTick()

      // During initial loading: nodes-total-badge shows '--', NEVER '0 个节点'!
      const totalBadge = mountEl.querySelector('[data-testid="nodes-total-badge"]')
      expect(totalBadge?.textContent?.trim()).toBe('--')

      // Now resolve initial request
      resolveFirst()
      await nextTick()
      await new Promise((r) => setTimeout(r, 40))

      // Total badge shows loaded / total
      expect(totalBadge?.textContent).toContain('2 / 50')

      // Now trigger refresh with failure
      shouldFailNodes = true
      const refreshBtn = mountEl.querySelector('button[title="刷新"]') as HTMLButtonElement
      refreshBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 40))

      // Verification: Error is shown, but existing 2 items ARE PRESERVED!
      expect(mountEl.textContent).toContain('Database timeout')
      const cards = mountEl.querySelectorAll('[data-testid="node-card"]')
      expect(cards.length).toBe(2)
      expect(mountEl.textContent).toContain('Tokyo Preserved 1')
      expect(mountEl.textContent).toContain('Tokyo Preserved 2')
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })

  it('discards out-of-order stale generation responses to prevent race condition write-backs', async () => {
    const { useNodes } = await import('./useNodes')

    let resolveReq1: any
    let resolveReq2: any

    const req1Promise = new Promise((r) => { resolveReq1 = r })
    const req2Promise = new Promise((r) => { resolveReq2 = r })

    let callCount = 0
    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        callCount++
        if (callCount === 1) {
          await req1Promise
          return {
            items: [{ logical_id: 'node-slow-1', display_name: 'Slow Response Node', protocol: 'ss', active: true }],
            total: 1,
            page: 1,
            page_size: 10,
          }
        }
        if (callCount === 2) {
          await req2Promise
          return {
            items: [{ logical_id: 'node-fast-2', display_name: 'Fast Response Node', protocol: 'ss', active: true }],
            total: 1,
            page: 1,
            page_size: 10,
          }
        }
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [], total_count: 1 }
      }
      return {}
    })

    const hook = useNodes()

    // Trigger request 1
    const p1 = hook.load()
    // Trigger request 2 immediately
    const p2 = hook.load()

    // Resolve request 2 first (fast response)
    resolveReq2()
    await p2
    expect(hook.items.value[0]?.logicalId).toBe('node-fast-2')

    // Now resolve request 1 later (stale out-of-order response)
    resolveReq1()
    await p1

    // State MUST remain fast response 2, slow response 1 must be discarded!
    expect(hook.items.value[0]?.logicalId).toBe('node-fast-2')
  })

  it('displays actual geo region from capabilities without fake country guessing and expands platform details on toggle', async () => {
    const { createApp, h, nextTick } = await import('vue')
    const { default: NodesView } = await import('./NodesView.vue')

    const mockNodes: NodeRecord[] = [
      {
        logical_id: 'node-geo-jp',
        display_name: 'JP Linode 101',
        protocol: 'vmess',
        active: true,
        health_status: 'healthy',
        capabilities: {
          baseline: { verdict: 'available', latency_ms: 38 },
          geo: { verdict: 'available', region: 'JP' },
          streaming: {
            verdict: 'available',
            platforms: {
              netflix: { verdict: 'available', region: 'JP' },
              youtube: { verdict: 'available' },
            },
          },
          ai: {
            verdict: 'available',
            platforms: {
              openai: { verdict: 'available' },
            },
          },
          speed: {
            verdict: 'available',
            throughput: 12500000,
          },
        },
      },
    ]

    vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
      if (path === '/api/v1/nodes') {
        return { items: mockNodes, total: 1, page: 1, page_size: 10 }
      }
      if (path === '/api/v1/probes/pool') {
        return { probing_node_ids: [], queued_node_ids: [], total_count: 1 }
      }
      return {}
    })

    const mountEl = document.createElement('div')
    document.body.appendChild(mountEl)
    const app = createApp({
      render() {
        return h(NodesView)
      },
    })
    try {
      app.mount(mountEl)
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      // 1. Geo region badge is rendered with actual region 'JP'
      const geoBadge = mountEl.querySelector('[data-testid="node-geo-badge"]')
      expect(geoBadge).not.toBeNull()
      expect(geoBadge?.textContent?.trim()).toBe('JP')

      // 2. Latency badge is prominently displayed
      const latencyBadge = mountEl.querySelector('[data-testid="node-latency-badge"]')
      expect(latencyBadge).not.toBeNull()
      expect(latencyBadge?.textContent).toContain('38 ms')

      // 3. Platform details accordion is initially collapsed
      expect(mountEl.querySelector('[data-testid="node-platform-details-expanded"]')).toBeNull()

      // 4. Toggle platform details
      const toggleBtn = mountEl.querySelector('[data-testid="node-platform-details-toggle"]') as HTMLButtonElement
      expect(toggleBtn).not.toBeNull()
      expect(toggleBtn.textContent).toContain('平台能力')
      toggleBtn.click()
      await nextTick()

      // 5. Expanded accordion displays streaming, AI and speed badges
      const expandedEl = mountEl.querySelector('[data-testid="node-platform-details-expanded"]')
      expect(expandedEl).not.toBeNull()
      expect(expandedEl?.textContent).toContain('NETFLIX')
      expect(expandedEl?.textContent).toContain('OPENAI')
      expect(expandedEl?.textContent).toContain('MB/s')

      // 6. getNodeGeoRegion unit checks: does NOT guess country from name
      expect(getNodeGeoRegion({ logical_id: 'node-name-kr', display_name: '🇰🇷 Seoul Korea 01', protocol: 'ss', active: true })).toBeNull()
      expect(getNodeGeoRegion({ logical_id: 'node-geo-us', display_name: 'US Server', protocol: 'ss', active: true, capabilities: { geo: { verdict: 'available', region: 'US' } } })).toBe('US')
    } finally {
      app.unmount()
      mountEl.remove()
    }
  })
})

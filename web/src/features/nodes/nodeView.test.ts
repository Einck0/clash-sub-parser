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
  type NodeRecord,
} from './nodeView'
import { subscriptionPatchPayload } from '../subscriptions/useSubscriptions'
import { api } from '../../api/client'

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

    // Baseline-authoritative health: auxiliary capabilities alone without baseline must be undetermined, never healthy
    expect(nodeHealthBadge({ logical_id: 'n1', protocol: 'ss', display_name: 'N1', active: true, capabilities: { streaming: 'available' } })).toEqual({
      label: '待复核',
      tone: 'warning',
    })
    expect(nodeHealthBadge({ logical_id: 'n1-baseline', protocol: 'ss', display_name: 'N1', active: true, capabilities: { baseline: { verdict: 'available', latency_ms: 50 } } })).toEqual({
      label: '正常',
      tone: 'success',
    })
    expect(nodeHealthBadge(node)).toEqual({ label: '待复核', tone: 'warning' })
    expect(nodeHealthBadge({ logical_id: 'n2', protocol: 'ss', display_name: 'N2', active: true, capabilities: { baseline: 'error' } })).toEqual({
      label: '异常',
      tone: 'error',
    })
    expect(nodeHealthBadge({ logical_id: 'n3', protocol: 'ss', display_name: 'N3', active: true })).toEqual({
      label: '未探测',
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
    expect(nodeHealthBadge(probingNode)).toEqual({ label: '检测中', tone: 'info' })

    const queuedNode = normalizeNode({
      logical_id: 'n-queued',
      protocol: 'trojan',
      display_name: 'SG Queued 02',
      active: true,
      probe_state: 'queued',
    })
    expect(queuedNode.probeState).toBe('queued')
    expect(nodeHealthBadge(queuedNode)).toEqual({ label: '队列中', tone: 'warning' })

    // Live pool ID sets override static idle node state
    expect(nodeHealthBadge(enrichedNode, new Set(['n-enriched']))).toEqual({
      label: '检测中',
      tone: 'info',
    })
    expect(nodeHealthBadge(enrichedNode, new Set(), new Set(['n-enriched']))).toEqual({
      label: '队列中',
      tone: 'warning',
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

    // Safety policy and probe config/build rejections remain undetermined, never misreported as 'unhealthy'
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
    expect(nodeUnderlyingHealthCategory(unsafeTlsNode)).toBe('undetermined')
    expect(resolveNodeLatencyMs(unsafeTlsNode)).toBeNull()
    expect(nodeHealthBadge(unsafeTlsNode)).toEqual({ label: '未知 · 安全拒绝', tone: 'warning' })
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
    expect(nodeUnderlyingHealthCategory(credsMissingNode)).toBe('undetermined')
    expect(nodeHealthBadge(credsMissingNode)).toEqual({ label: '未知 · 配置待核', tone: 'warning' })
    expect(nodeHealthDiagnostic(credsMissingNode)?.detail).toContain('不代表线路网络瘫痪')

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
    expect(nodeUnderlyingHealthCategory(staleBaselineNode)).toBe('undetermined')
    expect(resolveNodeLatencyMs(staleBaselineNode)).toBeNull()
    expect(nodeHealthBadge(staleBaselineNode)).toEqual({ label: '未知 · 待重测', tone: 'info' })
    expect(nodeHealthDiagnostic(staleBaselineNode)?.code).toBe('probe_stale')

    // AI available with missing baseline: classified as undetermined, does NOT fallback to healthy,
    // and auxiliary probe latency does NOT pollute overall node latency
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
    expect(nodeUnderlyingHealthCategory(aiOnlyNode)).toBe('undetermined')
    expect(resolveNodeLatencyMs(aiOnlyNode)).toBeNull()
    expect(nodeHealthBadge(aiOnlyNode)).toEqual({ label: '待复核', tone: 'warning' })

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
    expect(nodeHealthBadge(unprobedNode)).toEqual({ label: '未探测', tone: 'info' })
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
})

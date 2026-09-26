import type { ToastTone } from '../../ui/toast'
import type { CompilerTarget } from '../publications/publicationTypes'

export type CapabilityStatus = 'available' | 'restricted' | 'unknown' | 'error' | 'stale' | 'missing'

export type NodeProtocol =
  | 'ss'
  | 'vmess'
  | 'vless'
  | 'trojan'
  | 'hysteria2'
  | 'wireguard'
  | 'tuic'

export const SUPPORTED_NODE_PROTOCOLS: readonly NodeProtocol[] = [
  'ss',
  'vmess',
  'vless',
  'trojan',
  'hysteria2',
  'wireguard',
  'tuic',
] as const

export interface NodeConnectionInput {
  server?: string
  port?: number
  local_address?: string[]
  public_key?: string
  private_key?: string
  pre_shared_key?: string
  preshared_key?: string
  reserved?: number[]
  mtu?: number
  dns?: string[]
  uuid?: string
  password?: string
  method?: string
  alter_id?: number
  congestion_control?: string
  udp_relay_mode?: string
  alpn?: string[]
  sni?: string
  disable_sni?: boolean
  flow?: string
  reality_public_key?: string
  reality_short_id?: string
  client_fingerprint?: string
  up?: string
  down?: string
  obfs?: string
  username?: string
  transport?: Record<string, string>
}

export type ApiSafeNodeConnection = NodeConnectionInput
export type NodeCredentialsInput = NodeConnectionInput

export interface NodeConnectionProfile {
  server: string
  port: number
  localAddress: string[]
  publicKey: string
  privateKey: string
  preSharedKey: string
  reserved: number[]
  mtu?: number
  dns: string[]
  uuid: string
  password: string
  method: string
  congestionControl: string
  udpRelayMode: string
  alpn: string[]
  sni: string
  disableSni: boolean
  flow?: string
  realityPublicKey?: string
  realityShortId?: string
  clientFingerprint?: string
  up?: string
  down?: string
  obfs?: string
}

export type SafeNodeConnectionProfile = NodeConnectionProfile

export interface NodeSourceRecord {
  node_logical_id: string
  subscription_id: string
  last_seen_fetch_id: string
}

export interface NodeRecord {
  logical_id: string
  protocol: string
  display_name: string
  active: boolean
  created_at?: string
  updated_at?: string
  capabilities?: Record<string, CapabilityStatus>
  probe_stale?: boolean
  probe_missing?: boolean
  server?: string
  port?: number
  connection?: NodeConnectionInput
  credentials?: NodeCredentialsInput
  sources?: NodeSourceRecord[]
}

export interface NormalizedNode {
  logicalId: string
  protocol: string
  displayName: string
  active: boolean
  createdAt?: string
  updatedAt?: string
  capabilities: Record<string, CapabilityStatus>
  probeStale?: boolean
  probeMissing?: boolean
  connection: NodeConnectionProfile
  sources?: NodeSourceRecord[]
}

export function protocolSupportedTargets(protocol: string): readonly CompilerTarget[] {
  const norm = protocol.trim().toLowerCase()
  switch (norm) {
    case 'ss':
    case 'shadowsocks':
    case 'vmess':
    case 'trojan':
      return ['mihomo', 'singbox', 'surge', 'qx']
    case 'vless':
    case 'hysteria2':
    case 'wireguard':
    case 'tuic':
    default:
      return ['mihomo', 'singbox']
  }
}

export function sanitizeNodeConnection(node: NodeRecord): NodeConnectionProfile {
  const conn = node.connection
  const raw = node.credentials
  const transport = raw?.transport ?? conn?.transport ?? {}

  const server = (node.server ?? conn?.server ?? raw?.server ?? '').trim()
  const port = node.port ?? conn?.port ?? raw?.port ?? 0

  const rawLocal = raw?.local_address ?? conn?.local_address
  const localAddress = Array.isArray(rawLocal)
    ? rawLocal.map((s) => String(s).trim()).filter(Boolean)
    : []

  const publicKey = (raw?.public_key ?? conn?.public_key ?? transport.public_key ?? '').trim()
  const privateKey = (raw?.private_key ?? conn?.private_key ?? '').trim()
  const preSharedKey = (
    raw?.pre_shared_key ??
    raw?.preshared_key ??
    conn?.pre_shared_key ??
    conn?.preshared_key ??
    ''
  ).trim()

  const rawReserved = raw?.reserved ?? conn?.reserved
  const reserved = Array.isArray(rawReserved)
    ? rawReserved.map((n) => Number(n)).filter((n) => !Number.isNaN(n))
    : []

  const mtu = raw?.mtu ?? conn?.mtu
  const rawDns = raw?.dns ?? conn?.dns
  const dns = Array.isArray(rawDns)
    ? rawDns.map((s) => String(s).trim()).filter(Boolean)
    : []

  const uuid = (raw?.uuid ?? conn?.uuid ?? '').trim()
  const password = (raw?.password ?? conn?.password ?? '').trim()
  const method = (raw?.method ?? conn?.method ?? transport.cipher ?? '').trim()
  const congestionControl = (
    raw?.congestion_control ??
    conn?.congestion_control ??
    transport.congestion_control ??
    ''
  ).trim()
  const udpRelayMode = (
    raw?.udp_relay_mode ??
    conn?.udp_relay_mode ??
    transport.udp_relay_mode ??
    ''
  ).trim()

  const rawAlpn = raw?.alpn ?? conn?.alpn
  const alpn = Array.isArray(rawAlpn)
    ? rawAlpn.map((s) => String(s).trim()).filter(Boolean)
    : transport.alpn
    ? transport.alpn.split(',').map((s) => s.trim()).filter(Boolean)
    : []

  const sni = (raw?.sni ?? conn?.sni ?? transport.sni ?? transport.servername ?? '').trim()
  const disableSni =
    Boolean(raw?.disable_sni ?? conn?.disable_sni) ||
    ['true', '1', 'yes', 'on'].includes((transport.disable_sni || transport['disable-sni'] || '').toLowerCase())

  return {
    server,
    port,
    localAddress,
    publicKey,
    privateKey,
    preSharedKey,
    reserved,
    mtu,
    dns,
    uuid,
    password,
    method,
    congestionControl,
    udpRelayMode,
    alpn,
    sni,
    disableSni,
    flow: conn?.flow || raw?.flow || transport.flow || undefined,
    realityPublicKey: conn?.reality_public_key || raw?.reality_public_key || transport.pbk || undefined,
    realityShortId: conn?.reality_short_id || raw?.reality_short_id || transport.sid || undefined,
    clientFingerprint: conn?.client_fingerprint || raw?.client_fingerprint || transport.fp || undefined,
    up: conn?.up || raw?.up || transport.up || undefined,
    down: conn?.down || raw?.down || transport.down || undefined,
    obfs: conn?.obfs || raw?.obfs || transport.obfs || undefined,
  }
}

export function validateNodeConnectionProfile(
  protocol: string,
  draft: Partial<NodeConnectionProfile>
): string | null {
  const proto = protocol.trim().toLowerCase()
  if (!draft.server || !draft.server.trim()) {
    return 'Server address is required'
  }
  if (!draft.port || draft.port < 1 || draft.port > 65535) {
    return 'Port must be between 1 and 65535'
  }

  if (proto === 'wireguard') {
    const addrs = (draft.localAddress ?? []).map((a) => a.trim()).filter(Boolean)
    if (addrs.length === 0) {
      return 'WireGuard requires at least one local_address (IPv4/IPv6 CIDR)'
    }
    if (!draft.publicKey || !draft.publicKey.trim()) {
      return 'WireGuard requires peer public_key'
    }
    if (!draft.privateKey || !draft.privateKey.trim()) {
      return 'WireGuard requires private_key'
    }
    if (draft.mtu !== undefined && draft.mtu !== 0 && (draft.mtu < 576 || draft.mtu > 9000)) {
      return 'WireGuard MTU must be between 576 and 9000'
    }
  }

  if (proto === 'tuic') {
    if (!draft.uuid || !draft.uuid.trim()) {
      return 'TUIC requires uuid'
    }
    if (!draft.password || !draft.password.trim()) {
      return 'TUIC requires password'
    }
  }

  return null
}

export function renderNodePreview(
  node: NormalizedNode,
  target: 'mihomo' | 'singbox' = 'mihomo'
): string {
  const proto = node.protocol.trim().toLowerCase()
  const conn = node.connection

  if (target === 'singbox') {
    if (proto === 'wireguard') {
      const wgEndpoint: Record<string, unknown> = {
        type: 'wireguard',
        tag: node.displayName,
        address: conn.localAddress,
        private_key: conn.privateKey,
        peers: [
          {
            address: conn.server,
            port: conn.port,
            public_key: conn.publicKey,
            ...(conn.preSharedKey ? { pre_shared_key: conn.preSharedKey } : {}),
            ...(conn.reserved.length === 3 ? { reserved: conn.reserved } : {}),
          },
        ],
        ...(conn.mtu ? { mtu: conn.mtu } : {}),
      }
      return JSON.stringify({ endpoints: [wgEndpoint] }, null, 2)
    }

    if (proto === 'tuic') {
      const tuicOutbound: Record<string, unknown> = {
        type: 'tuic',
        tag: node.displayName,
        server: conn.server,
        server_port: conn.port,
        uuid: conn.uuid,
        password: conn.password,
        ...(conn.congestionControl ? { congestion_control: conn.congestionControl } : {}),
        ...(conn.udpRelayMode ? { udp_relay_mode: conn.udpRelayMode } : {}),
        tls: {
          enabled: true,
          ...(conn.sni ? { server_name: conn.sni } : {}),
          ...(conn.alpn.length > 0 ? { alpn: conn.alpn } : {}),
          ...(conn.disableSni ? { disable_sni: true } : {}),
        },
      }
      return JSON.stringify({ outbounds: [tuicOutbound] }, null, 2)
    }

    const genericOutbound: Record<string, unknown> = {
      type: proto,
      tag: node.displayName,
      server: conn.server,
      server_port: conn.port,
      ...(conn.uuid ? { uuid: conn.uuid } : {}),
      ...(conn.method ? { method: conn.method } : {}),
      ...(conn.password ? { password: conn.password } : {}),
      ...(conn.sni || conn.alpn.length > 0 || conn.disableSni
        ? {
            tls: {
              enabled: true,
              ...(conn.sni ? { server_name: conn.sni } : {}),
              ...(conn.alpn.length > 0 ? { alpn: conn.alpn } : {}),
              ...(conn.disableSni ? { disable_sni: true } : {}),
            },
          }
        : {}),
    }
    return JSON.stringify({ outbounds: [genericOutbound] }, null, 2)
  }

  if (proto === 'wireguard') {
    const ipv4 = conn.localAddress.find((a) => !a.includes(':')) || conn.localAddress[0] || ''
    const ipv6 = conn.localAddress.find((a) => a.includes(':'))
    const lines = [
      'proxies:',
      `  - name: ${node.displayName}`,
      '    type: wireguard',
      `    server: ${conn.server}`,
      `    port: ${conn.port}`,
    ]
    if (ipv4) lines.push(`    ip: ${ipv4}`)
    if (ipv6) lines.push(`    ipv6: ${ipv6}`)
    lines.push(`    public-key: ${conn.publicKey}`)
    lines.push(`    private-key: ${conn.privateKey}`)
    if (conn.preSharedKey) {
      lines.push(`    pre-shared-key: ${conn.preSharedKey}`)
    }
    if (conn.mtu) {
      lines.push(`    mtu: ${conn.mtu}`)
    }
    if (conn.dns.length > 0) {
      lines.push(`    dns: [${conn.dns.join(', ')}]`)
    }
    if (conn.reserved.length === 3) {
      lines.push(`    reserved: [${conn.reserved.join(', ')}]`)
    }
    lines.push('    udp: true')
    return lines.join('\n')
  }

  if (proto === 'tuic') {
    const lines = [
      'proxies:',
      `  - name: ${node.displayName}`,
      '    type: tuic',
      `    server: ${conn.server}`,
      `    port: ${conn.port}`,
      `    uuid: ${conn.uuid}`,
      `    password: ${conn.password}`,
    ]
    if (conn.congestionControl) {
      lines.push(`    congestion-controller: ${conn.congestionControl}`)
    }
    if (conn.udpRelayMode) {
      lines.push(`    udp-relay-mode: ${conn.udpRelayMode}`)
    }
    if (conn.alpn.length > 0) {
      lines.push(`    alpn: [${conn.alpn.join(', ')}]`)
    }
    if (conn.sni) {
      lines.push(`    sni: ${conn.sni}`)
    }
    if (conn.disableSni) {
      lines.push('    disable-sni: true')
    }
    lines.push('    udp: true')
    return lines.join('\n')
  }

  const lines = [
    'proxies:',
    `  - name: ${node.displayName}`,
    `    type: ${proto}`,
    `    server: ${conn.server}`,
    `    port: ${conn.port}`,
  ]
  if (conn.uuid) lines.push(`    uuid: ${conn.uuid}`)
  if (conn.method) lines.push(`    cipher: ${conn.method}`)
  if (conn.password) lines.push(`    password: ${conn.password}`)
  if (conn.sni) lines.push(`    sni: ${conn.sni}`)
  if (conn.alpn.length > 0) lines.push(`    alpn: [${conn.alpn.join(', ')}]`)
  if (conn.disableSni) lines.push('    disable-sni: true')
  if (conn.realityPublicKey) {
    lines.push('    reality-opts:')
    lines.push(`      public-key: ${conn.realityPublicKey}`)
    if (conn.realityShortId) lines.push(`      short-id: ${conn.realityShortId}`)
  }
  return lines.join('\n')
}

export const renderSafeNodePreview = renderNodePreview

export function normalizeNode(node: NodeRecord): NormalizedNode {
  const logicalId = node.logical_id || (node as any).logicalId || ''
  const rawDisplayName = node.display_name ?? (node as any).displayName ?? ''
  const normalizedInput: NodeRecord = {
    ...node,
    logical_id: logicalId,
    display_name: rawDisplayName,
  }
  const probeMissing = node.probe_missing ?? (!node.capabilities || Object.keys(node.capabilities).length === 0)
  const probeStale = node.probe_stale ?? Object.values(node.capabilities ?? {}).some((s) => s === 'stale')
  return {
    logicalId,
    protocol: node.protocol,
    displayName: rawDisplayName.trim() || logicalId,
    active: node.active,
    createdAt: node.created_at,
    updatedAt: node.updated_at,
    capabilities: node.capabilities ?? {},
    probeStale,
    probeMissing,
    connection: sanitizeNodeConnection(normalizedInput),
    sources: node.sources,
  }
}

const capabilityLabels: Record<CapabilityStatus, { label: string; tone: ToastTone }> = {
  available: { label: 'Available', tone: 'success' },
  restricted: { label: 'Restricted', tone: 'warning' },
  unknown: { label: 'Unknown', tone: 'info' },
  error: { label: 'Error', tone: 'error' },
  stale: { label: 'Stale', tone: 'warning' },
  missing: { label: 'Missing', tone: 'info' },
}

export function nodeCapabilityLabel(node: NodeRecord | NormalizedNode, capability: string) {
  return capabilityLabels[node.capabilities?.[capability] ?? 'unknown']
}

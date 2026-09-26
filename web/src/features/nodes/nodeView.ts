import type { ToastTone } from '../../ui/toast'
import { redactPreviewSecrets, type CompilerTarget } from '../publications/publicationTypes'

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

export interface ApiSafeNodeConnection {
  available?: boolean
  unavailable_reason?: string
  server?: string
  port?: number
  local_address?: string[]
  public_key?: string
  reserved?: number[]
  mtu?: number
  dns?: string[]
  uuid?: string
  congestion_control?: string
  udp_relay_mode?: string
  alpn?: string[]
  sni?: string
  disable_sni?: boolean
  method?: string
  flow?: string
  reality_public_key?: string
  reality_short_id?: string
  client_fingerprint?: string
  up?: string
  down?: string
  obfs?: string
  has_private_key?: boolean
  has_pre_shared_key?: boolean
  has_password?: boolean
}

export interface NodeCredentialsInput extends ApiSafeNodeConnection {
  password?: string
  alter_id?: number
  private_key?: string
  preshared_key?: string
  pre_shared_key?: string
  transport?: Record<string, string>
}

export interface SafeNodeConnectionProfile {
  available: boolean
  unavailableReason?: string
  server: string
  port: number
  localAddress: string[]
  publicKey: string
  reserved: number[]
  mtu?: number
  dns: string[]
  uuid: string
  congestionControl: string
  udpRelayMode: string
  alpn: string[]
  sni: string
  disableSni: boolean
  method?: string
  flow?: string
  realityPublicKey?: string
  realityShortId?: string
  clientFingerprint?: string
  up?: string
  down?: string
  obfs?: string
  privateKeyMasked: '***'
  preSharedKeyMasked: '***'
  passwordMasked: '***'
  hasPrivateKey: boolean
  hasPreSharedKey: boolean
  hasPassword: boolean
}

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
  credential_version?: number
  created_at?: string
  updated_at?: string
  capabilities?: Record<string, CapabilityStatus>
  probe_stale?: boolean
  probe_missing?: boolean
  credential_mismatch?: boolean
  server?: string
  port?: number
  connection?: ApiSafeNodeConnection
  credentials?: NodeCredentialsInput
  sources?: NodeSourceRecord[]
}

export interface NormalizedNode {
  logicalId: string
  protocol: string
  displayName: string
  active: boolean
  credentialVersion?: number
  createdAt?: string
  updatedAt?: string
  capabilities: Record<string, CapabilityStatus>
  probeStale?: boolean
  probeMissing?: boolean
  credentialMismatch?: boolean
  connection: SafeNodeConnectionProfile
  sources?: NodeSourceRecord[]
}

export function maskSecretReference(_value: string): string {
  return '***'
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

export function sanitizeNodeConnection(node: NodeRecord): SafeNodeConnectionProfile {
  const conn = node.connection
  const raw = node.credentials
  const transport = raw?.transport ?? {}

  const server = (conn?.server ?? node.server ?? raw?.server ?? '').trim()
  const port = conn?.port ?? node.port ?? raw?.port ?? 0

  const explicitAvailable =
    conn && typeof conn.available === 'boolean'
      ? conn.available
      : Boolean(server && port >= 1 && port <= 65535)

  if (!explicitAvailable || !server || port < 1 || port > 65535) {
    return {
      available: false,
      unavailableReason: conn?.unavailable_reason || 'credential_unavailable',
      server: '',
      port: 0,
      localAddress: [],
      publicKey: '',
      reserved: [],
      mtu: undefined,
      dns: [],
      uuid: '',
      congestionControl: '',
      udpRelayMode: '',
      alpn: [],
      sni: '',
      disableSni: false,
      privateKeyMasked: '***',
      preSharedKeyMasked: '***',
      passwordMasked: '***',
      hasPrivateKey: false,
      hasPreSharedKey: false,
      hasPassword: false,
    }
  }

  const rawLocal = conn?.local_address ?? raw?.local_address
  const localAddress = Array.isArray(rawLocal)
    ? rawLocal.map((s) => String(s).trim()).filter(Boolean)
    : []

  const publicKey = (conn?.public_key ?? raw?.public_key ?? transport.public_key ?? '').trim()

  const rawReserved = conn?.reserved ?? raw?.reserved
  const reserved = Array.isArray(rawReserved)
    ? rawReserved.map((n) => Number(n)).filter((n) => !Number.isNaN(n))
    : []

  const mtu = conn?.mtu ?? raw?.mtu
  const rawDns = conn?.dns ?? raw?.dns
  const dns = Array.isArray(rawDns)
    ? rawDns.map((s) => String(s).trim()).filter(Boolean)
    : []

  const uuid = (conn?.uuid ?? raw?.uuid ?? '').trim()
  const congestionControl = (
    conn?.congestion_control ??
    raw?.congestion_control ??
    transport.congestion_control ??
    ''
  ).trim()
  const udpRelayMode = (
    conn?.udp_relay_mode ??
    raw?.udp_relay_mode ??
    transport.udp_relay_mode ??
    ''
  ).trim()

  const rawAlpn = conn?.alpn ?? raw?.alpn
  const alpn = Array.isArray(rawAlpn)
    ? rawAlpn.map((s) => String(s).trim()).filter(Boolean)
    : transport.alpn
    ? transport.alpn.split(',').map((s) => s.trim()).filter(Boolean)
    : []

  const sni = (conn?.sni ?? raw?.sni ?? transport.sni ?? transport.servername ?? '').trim()
  const disableSni =
    Boolean(conn?.disable_sni ?? raw?.disable_sni) ||
    ['true', '1', 'yes', 'on'].includes((transport.disable_sni || transport['disable-sni'] || '').toLowerCase())

  const hasPrivateKey =
    typeof conn?.has_private_key === 'boolean'
      ? conn.has_private_key
      : Boolean(raw?.private_key && raw.private_key.trim())
  const hasPreSharedKey =
    typeof conn?.has_pre_shared_key === 'boolean'
      ? conn.has_pre_shared_key
      : Boolean(
          (raw?.pre_shared_key && raw.pre_shared_key.trim()) ||
            (raw?.preshared_key && raw.preshared_key.trim())
        )
  const hasPassword =
    typeof conn?.has_password === 'boolean'
      ? conn.has_password
      : Boolean(raw?.password && raw.password.trim())

  return {
    available: true,
    server,
    port,
    localAddress,
    publicKey,
    reserved,
    mtu,
    dns,
    uuid,
    congestionControl,
    udpRelayMode,
    alpn,
    sni,
    disableSni,
    method: conn?.method || raw?.method || transport.cipher || undefined,
    flow: conn?.flow || transport.flow || undefined,
    realityPublicKey: conn?.reality_public_key || transport.pbk || undefined,
    realityShortId: conn?.reality_short_id || transport.sid || undefined,
    clientFingerprint: conn?.client_fingerprint || transport.fp || undefined,
    up: conn?.up || transport.up || undefined,
    down: conn?.down || transport.down || undefined,
    obfs: conn?.obfs || transport.obfs || undefined,
    privateKeyMasked: '***',
    preSharedKeyMasked: '***',
    passwordMasked: '***',
    hasPrivateKey,
    hasPreSharedKey,
    hasPassword,
  }
}

export function validateNodeConnectionProfile(
  protocol: string,
  draft: Partial<SafeNodeConnectionProfile> & {
    privateKeyInput?: string
    preSharedKeyInput?: string
    passwordInput?: string
  }
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
    const hasPriv = Boolean(draft.hasPrivateKey || (draft.privateKeyInput && draft.privateKeyInput.trim()))
    if (!hasPriv) {
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
    const hasPass = Boolean(draft.hasPassword || (draft.passwordInput && draft.passwordInput.trim()))
    if (!hasPass) {
      return 'TUIC requires password'
    }
  }

  return null
}

export function renderSafeNodePreview(
  node: NormalizedNode,
  target: 'mihomo' | 'singbox' = 'mihomo'
): string {
  const proto = node.protocol.trim().toLowerCase()
  const conn = node.connection
  if (!conn.available || !conn.server || conn.port < 1) {
    return `# Connection details unavailable (${conn.unavailableReason || 'credential_unavailable'})`
  }

  if (target === 'singbox') {
    if (proto === 'wireguard') {
      const wgEndpoint: Record<string, unknown> = {
        type: 'wireguard',
        tag: node.displayName,
        address: conn.localAddress,
        private_key: '***',
        peers: [
          {
            address: conn.server,
            port: conn.port,
            public_key: conn.publicKey,
            ...(conn.hasPreSharedKey ? { pre_shared_key: '***' } : {}),
            ...(conn.reserved.length === 3 ? { reserved: conn.reserved } : {}),
          },
        ],
        ...(conn.mtu ? { mtu: conn.mtu } : {}),
      }
      return redactPreviewSecrets(JSON.stringify({ endpoints: [wgEndpoint] }, null, 2))
    }

    if (proto === 'tuic') {
      const tuicOutbound: Record<string, unknown> = {
        type: 'tuic',
        tag: node.displayName,
        server: conn.server,
        server_port: conn.port,
        uuid: conn.uuid,
        password: '***',
        ...(conn.congestionControl ? { congestion_control: conn.congestionControl } : {}),
        ...(conn.udpRelayMode ? { udp_relay_mode: conn.udpRelayMode } : {}),
        tls: {
          enabled: true,
          ...(conn.sni ? { server_name: conn.sni } : {}),
          ...(conn.alpn.length > 0 ? { alpn: conn.alpn } : {}),
          ...(conn.disableSni ? { disable_sni: true } : {}),
        },
      }
      return redactPreviewSecrets(JSON.stringify({ outbounds: [tuicOutbound] }, null, 2))
    }

    const genericOutbound: Record<string, unknown> = {
      type: proto,
      tag: node.displayName,
      server: conn.server,
      server_port: conn.port,
      ...(conn.uuid ? { uuid: conn.uuid } : {}),
      ...(conn.hasPassword ? { password: '***' } : {}),
    }
    return redactPreviewSecrets(JSON.stringify({ outbounds: [genericOutbound] }, null, 2))
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
    lines.push('    private-key: ***')
    if (conn.hasPreSharedKey) {
      lines.push('    pre-shared-key: ***')
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
    return redactPreviewSecrets(lines.join('\n'))
  }

  if (proto === 'tuic') {
    const lines = [
      'proxies:',
      `  - name: ${node.displayName}`,
      '    type: tuic',
      `    server: ${conn.server}`,
      `    port: ${conn.port}`,
      `    uuid: ${conn.uuid}`,
      '    password: ***',
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
    return redactPreviewSecrets(lines.join('\n'))
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
  if (conn.hasPassword) lines.push('    password: ***')
  if (conn.realityPublicKey) {
    lines.push('    reality-opts:')
    lines.push(`      public-key: ${conn.realityPublicKey}`)
    if (conn.realityShortId) lines.push(`      short-id: ${conn.realityShortId}`)
  }
  return redactPreviewSecrets(lines.join('\n'))
}

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
    credentialVersion: node.credential_version,
    createdAt: node.created_at,
    updatedAt: node.updated_at,
    capabilities: node.capabilities ?? {},
    probeStale,
    probeMissing,
    credentialMismatch: Boolean(node.credential_mismatch),
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

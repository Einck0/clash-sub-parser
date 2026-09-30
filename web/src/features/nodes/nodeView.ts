import type { ToastTone } from '../../ui/toast'
import type { CompilerTarget } from '../publications/publicationTypes'

export type CapabilityStatus = 'available' | 'restricted' | 'unknown' | 'error' | 'stale' | 'missing'

export interface StructuredCapabilityStatus {
  verdict: CapabilityStatus
  latency_ms?: number
  observed_at?: string
  summary?: string
  stale?: boolean
}

export type CapabilityInput = CapabilityStatus | StructuredCapabilityStatus

export type NodeProbeState = 'probing' | 'queued' | 'idle'

export type NodeHealthStatus =
  | 'probing'
  | 'queued'
  | 'healthy'
  | 'degraded'
  | 'unhealthy'
  | 'unknown'
  | 'missing'

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

export interface IPRiskSummaryRecord {
  decision?: 'allow' | 'review' | 'block' | 'unknown'
  risk_band?: 'low' | 'medium' | 'high' | 'critical' | 'unknown'
  status?: 'fresh' | 'stale' | 'missing' | 'error'
  provider?: string
  reason_code?: string
}

export interface NodeRecord {
  logical_id: string
  protocol: string
  display_name: string
  active: boolean
  connection_revision?: number
  created_at?: string
  updated_at?: string
  latency_ms?: number | null
  last_probed_at?: string | null
  capabilities?: Record<string, CapabilityInput>
  probe_stale?: boolean
  probe_missing?: boolean
  probe_state?: NodeProbeState
  health_status?: NodeHealthStatus
  ip_risk_summary?: IPRiskSummaryRecord
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
  connectionRevision?: number
  createdAt?: string
  updatedAt?: string
  latencyMs?: number | null
  lastProbedAt?: string | null
  capabilities: Record<string, CapabilityStatus>
  capabilityDetails: Record<string, StructuredCapabilityStatus>
  probeStale?: boolean
  probeMissing?: boolean
  probeState: NodeProbeState
  healthStatus?: NodeHealthStatus
  ipRiskSummary?: IPRiskSummaryRecord
  connection: NodeConnectionProfile
  sources?: NodeSourceRecord[]
}

export interface NodeHealthDiagnostic {
  code: string
  shortLabel: string
  detail: string
  isBlockedByPolicyOrConfig: boolean
}

export function extractCapabilityVerdict(val: CapabilityInput | undefined): CapabilityStatus {
  if (!val) return 'unknown'
  if (typeof val === 'string') return val
  if (typeof val === 'object' && typeof val.verdict === 'string') {
    return val.verdict
  }
  return 'unknown'
}

export function isCapabilityStale(val: CapabilityInput | undefined): boolean {
  if (!val) return false
  if (typeof val === 'string') return val === 'stale'
  if (typeof val === 'object') {
    return Boolean(val.stale) || val.verdict === 'stale'
  }
  return false
}

export function extractCapabilityDetail(
  node: NodeRecord | NormalizedNode,
  capability: string
): StructuredCapabilityStatus | null {
  if ('capabilityDetails' in node && node.capabilityDetails?.[capability]) {
    return node.capabilityDetails[capability]
  }
  const raw = node.capabilities?.[capability]
  if (!raw) return null
  if (typeof raw === 'string') {
    return { verdict: raw, stale: raw === 'stale' }
  }
  if (typeof raw === 'object' && raw !== null) {
    return {
      verdict: raw.verdict ?? 'unknown',
      latency_ms: raw.latency_ms,
      observed_at: raw.observed_at,
      summary: raw.summary,
      stale: Boolean(raw.stale) || raw.verdict === 'stale',
    }
  }
  return null
}

export function protocolSupportedTargets(protocol: string): readonly CompilerTarget[] {
  const norm = protocol.trim().toLowerCase()
  switch (norm) {
    case 'ss':
    case 'shadowsocks':
    case 'vmess':
    case 'trojan':
      return ['mihomo', 'singbox', 'surge', 'qx']
    case 'hysteria2':
    case 'wireguard':
    case 'tuic':
      return ['mihomo', 'singbox', 'surge']
    case 'vless':
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
    return '服务器地址不能为空'
  }
  if (!draft.port || draft.port < 1 || draft.port > 65535) {
    return '端口必须在 1 到 65535 之间'
  }

  if (proto === 'wireguard') {
    const addrs = (draft.localAddress ?? []).map((a) => a.trim()).filter(Boolean)
    if (addrs.length === 0) {
      return 'WireGuard 至少需要配置一个内网地址 (local_address，IPv4/IPv6 CIDR)'
    }
    if (!draft.publicKey || !draft.publicKey.trim()) {
      return 'WireGuard 需要配置对端公钥 (public_key)'
    }
    if (!draft.privateKey || !draft.privateKey.trim()) {
      return 'WireGuard 需要配置客户端私钥 (private_key)'
    }
    if (draft.mtu !== undefined && draft.mtu !== 0 && (draft.mtu < 576 || draft.mtu > 9000)) {
      return 'WireGuard MTU 必须在 576 到 9000 之间'
    }
  }

  if (proto === 'tuic') {
    if (!draft.uuid || !draft.uuid.trim()) {
      return 'TUIC 需要配置 UUID (uuid)'
    }
    if (!draft.password || !draft.password.trim()) {
      return 'TUIC 需要配置认证密码 (password)'
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

  const rawCaps = node.capabilities ?? {}
  const capabilities: Record<string, CapabilityStatus> = {}
  const capabilityDetails: Record<string, StructuredCapabilityStatus> = {}

  for (const [k, v] of Object.entries(rawCaps)) {
    const verdict = extractCapabilityVerdict(v)
    capabilities[k] = verdict
    if (typeof v === 'object' && v !== null) {
      capabilityDetails[k] = {
        verdict,
        latency_ms: v.latency_ms,
        observed_at: v.observed_at,
        summary: v.summary,
        stale: Boolean(v.stale) || verdict === 'stale',
      }
    } else {
      capabilityDetails[k] = {
        verdict,
        stale: verdict === 'stale',
      }
    }
  }

  const capEntries = Object.values(rawCaps)
  const probeMissing = node.probe_missing ?? capEntries.length === 0
  const probeStale = node.probe_stale ?? capEntries.some(isCapabilityStale)

  let latencyMs: number | null | undefined = node.latency_ms ?? (node as any).latencyMs
  if (
    latencyMs === undefined &&
    capabilityDetails.baseline?.latency_ms !== undefined &&
    capabilityDetails.baseline.verdict !== 'error' &&
    !capabilityDetails.baseline.stale &&
    capabilityDetails.baseline.latency_ms > 0
  ) {
    latencyMs = capabilityDetails.baseline.latency_ms
  }
  if (
    typeof latencyMs === 'number' &&
    (!Number.isFinite(latencyMs) ||
      latencyMs <= 0 ||
      node.health_status === 'unhealthy' ||
      node.health_status === 'unknown' ||
      node.health_status === 'missing')
  ) {
    latencyMs = null
  }

  const lastProbedAt: string | null | undefined =
    node.last_probed_at ??
    (node as any).lastProbedAt ??
    capabilityDetails.baseline?.observed_at

  const rawProbeState: NodeProbeState | undefined =
    node.probe_state ??
    (node as any).probeState ??
    (node.health_status === 'probing'
      ? 'probing'
      : node.health_status === 'queued'
      ? 'queued'
      : undefined)

  return {
    logicalId,
    protocol: node.protocol,
    displayName: rawDisplayName.trim() || logicalId,
    active: node.active,
    connectionRevision: node.connection_revision ?? (node as any).connectionRevision,
    createdAt: node.created_at,
    updatedAt: node.updated_at,
    latencyMs,
    lastProbedAt,
    capabilities,
    capabilityDetails,
    probeStale,
    probeMissing,
    probeState: rawProbeState ?? 'idle',
    healthStatus: node.health_status,
    ipRiskSummary: node.ip_risk_summary ?? (node as any).ipRiskSummary,
    connection: sanitizeNodeConnection(normalizedInput),
    sources: node.sources,
  }
}

const capabilityLabels: Record<CapabilityStatus, { label: string; tone: ToastTone }> = {
  available: { label: '可用', tone: 'success' },
  restricted: { label: '受限', tone: 'warning' },
  unknown: { label: '未知', tone: 'info' },
  error: { label: '异常', tone: 'error' },
  stale: { label: '已过期', tone: 'warning' },
  missing: { label: '未探测', tone: 'info' },
}

export function nodeCapabilityLabel(node: NodeRecord | NormalizedNode, capability: string) {
  const verdict = extractCapabilityVerdict(node.capabilities?.[capability])
  return capabilityLabels[verdict ?? 'unknown'] ?? capabilityLabels.unknown
}

function hasNodeId(collection: Set<string> | Iterable<string> | undefined, id: string): boolean {
  if (!collection || !id) return false
  if (collection instanceof Set) return collection.has(id)
  for (const item of collection) {
    if (item === id) return true
  }
  return false
}

function extractSummaryReason(summary?: string): string {
  if (!summary) return ''
  const match = summary.match(/(?:^|\s)reason=(\S+)/)
  return match ? match[1] : ''
}

export function resolveNodeProbeState(
  node: NodeRecord | NormalizedNode,
  probingIds?: Set<string> | Iterable<string>,
  queuedIds?: Set<string> | Iterable<string>
): NodeProbeState {
  const logicalId = (node as NormalizedNode).logicalId ?? (node as NodeRecord).logical_id ?? ''
  const probeState = (node as NormalizedNode).probeState ?? (node as NodeRecord).probe_state
  const health = (node as NormalizedNode).healthStatus ?? (node as NodeRecord).health_status
  if (probeState === 'probing' || health === 'probing' || hasNodeId(probingIds, logicalId)) {
    return 'probing'
  }
  if (probeState === 'queued' || health === 'queued' || hasNodeId(queuedIds, logicalId)) {
    return 'queued'
  }
  return 'idle'
}

export function nodeUnderlyingHealthCategory(
  node: NodeRecord | NormalizedNode
): 'healthy' | 'degraded' | 'unhealthy' | 'unprobed' {
  const health = (node as NormalizedNode).healthStatus ?? (node as NodeRecord).health_status
  if (health === 'healthy') return 'healthy'
  if (health === 'degraded') return 'degraded'
  if (health === 'unhealthy') return 'unhealthy'
  if (health === 'missing' || health === 'unknown') {
    return 'unprobed'
  }

  const baseDetail = extractCapabilityDetail(node, 'baseline')
  if (baseDetail) {
    if (baseDetail.stale || baseDetail.verdict === 'stale') {
      return 'unprobed'
    }
    if (baseDetail.verdict === 'available') {
      return 'healthy'
    }
    if (baseDetail.verdict === 'restricted') {
      return 'degraded'
    }
    if (baseDetail.verdict === 'error') {
      const reason = extractSummaryReason(baseDetail.summary)
      if (reason && reason !== 'node_connect_failed') {
        return 'unprobed'
      }
      return 'unhealthy'
    }
    return 'unprobed'
  }

  const caps = node.capabilities ?? {}
  const rawValues = Object.values(caps)
  const values = rawValues.map(extractCapabilityVerdict)
  const probeMissing =
    (node as NormalizedNode).probeMissing ??
    (node as NodeRecord).probe_missing ??
    values.length === 0
  const probeStale =
    (node as NormalizedNode).probeStale ??
    (node as NodeRecord).probe_stale ??
    rawValues.some(isCapabilityStale)

  if (probeMissing || values.length === 0) {
    return 'unprobed'
  }
  if (values.includes('error')) {
    return 'unhealthy'
  }
  if (probeStale || values.includes('restricted') || values.includes('stale')) {
    return 'degraded'
  }
  if (values.includes('available')) {
    return 'healthy'
  }
  return 'unprobed'
}

export function nodeHealthDiagnostic(node: NodeRecord | NormalizedNode): NodeHealthDiagnostic | null {
  const category = nodeUnderlyingHealthCategory(node)
  if (category === 'healthy' || category === 'degraded') {
    return null
  }

  const baseDetail = extractCapabilityDetail(node, 'baseline')
  const reason = extractSummaryReason(baseDetail?.summary)

  if (reason === 'unsafe_tls_rejected') {
    return {
      code: 'unsafe_tls_rejected',
      shortLabel: '安全拒绝：跳过证书校验',
      detail: '探针安全策略拒绝：节点启用了跳过 TLS 证书校验 (skip_cert_verify)，零出站拦截，不代表线路网络瘫痪',
      isBlockedByPolicyOrConfig: true,
    }
  }
  if (reason === 'unsafe_option_rejected') {
    return {
      code: 'unsafe_option_rejected',
      shortLabel: '安全拒绝：不安全协议选项',
      detail: '探针安全策略拒绝：节点包含未验证端口跳跃或禁用 SNI 等不安全参数，零出站拦截，不代表线路故障',
      isBlockedByPolicyOrConfig: true,
    }
  }
  if (reason === 'private_target_rejected') {
    return {
      code: 'private_target_rejected',
      shortLabel: '安全拒绝：私有目标地址',
      detail: '探针安全策略拒绝：目标地址属于内网或保留网段 (private_target_rejected)，零出站拦截，不代表线路故障',
      isBlockedByPolicyOrConfig: true,
    }
  }
  if (reason === 'credentials_unavailable') {
    return {
      code: 'credentials_unavailable',
      shortLabel: '凭据缺失 / 待核验',
      detail: '探针未能读取完整连接凭据或必要协议参数 (credentials_unavailable)，未完成出站检测，不代表线路网络瘫痪',
      isBlockedByPolicyOrConfig: true,
    }
  }
  if (
    reason === 'client_build_failed' ||
    reason === 'probe_dialing_not_configured' ||
    reason === 'request_build_failed'
  ) {
    return {
      code: reason,
      shortLabel: '探针构建失败',
      detail: '探针客户端构建失败 (client_build_failed)：协议或传输参数暂不兼容，未完成出站检测，不代表线路网络瘫痪',
      isBlockedByPolicyOrConfig: true,
    }
  }
  if (reason === 'target_unresolvable') {
    return {
      code: 'target_unresolvable',
      shortLabel: '入口域名无法解析',
      detail: '节点入口域名 DNS 解析失败 (target_unresolvable)，暂无法建立探测连接，状态待复核',
      isBlockedByPolicyOrConfig: false,
    }
  }
  if (reason === 'transport_error' && category === 'unprobed') {
    return {
      code: 'transport_error',
      shortLabel: '传输异常 · 待复核',
      detail: '探测传输异常 (transport_error)，缺乏连接握手阶段失败确权证据，暂列为未知/待核验，不直接断定线路瘫痪',
      isBlockedByPolicyOrConfig: false,
    }
  }
  if ((reason === 'timeout' || reason === 'dial_timeout') && category === 'unprobed') {
    return {
      code: reason,
      shortLabel: '探测超时 · 待复核',
      detail: '探测请求超时，暂列为未知/待核验状态，等待下轮巡检复核',
      isBlockedByPolicyOrConfig: false,
    }
  }
  if ((reason === 'dns_error' || reason === 'dns_resolution_failed') && category === 'unprobed') {
    return {
      code: reason,
      shortLabel: 'DNS 解析异常 · 待复核',
      detail: '探测过程中 DNS 解析异常，暂列为未知/待核验状态',
      isBlockedByPolicyOrConfig: false,
    }
  }
  if (category === 'unhealthy') {
    return {
      code: reason || 'node_connect_failed',
      shortLabel: '连接或握手失败',
      detail: '节点代理连接或握手阶段失败，已证实当前不可达',
      isBlockedByPolicyOrConfig: false,
    }
  }

  const probeStale =
    Boolean(baseDetail?.stale) ||
    Boolean((node as NormalizedNode).probeStale ?? (node as NodeRecord).probe_stale)
  if (probeStale) {
    return {
      code: 'probe_stale',
      shortLabel: '观测已过期 / 配置已更新',
      detail: '基础连通性观测已超保鲜期或节点连接参数已更新，等待 1 分钟增量巡检或手动刷新按需重测',
      isBlockedByPolicyOrConfig: false,
    }
  }

  const caps = node.capabilities ?? {}
  const capKeys = Object.keys(caps)
  if (!baseDetail && capKeys.length > 0) {
    return {
      code: 'baseline_missing',
      shortLabel: '缺少基础连通观测',
      detail: '仅有流媒体/AI 等辅助能力记录，缺少有效基础连通性 (baseline) 观测，整体状态待核验',
      isBlockedByPolicyOrConfig: false,
    }
  }

  return {
    code: 'probe_missing',
    shortLabel: '尚未探测',
    detail: '尚无探测观测记录，等待 1 分钟增量巡检或手动刷新入池检测',
    isBlockedByPolicyOrConfig: false,
  }
}

export function nodeHealthBadge(
  node: NodeRecord | NormalizedNode,
  probingIds?: Set<string> | Iterable<string>,
  queuedIds?: Set<string> | Iterable<string>
): { label: string; tone: ToastTone } {
  const liveProbeState = resolveNodeProbeState(node, probingIds, queuedIds)
  if (liveProbeState === 'probing') {
    return { label: '检测中', tone: 'info' }
  }
  if (liveProbeState === 'queued') {
    return { label: '队列中', tone: 'warning' }
  }

  const category = nodeUnderlyingHealthCategory(node)
  if (category === 'healthy') return { label: '正常', tone: 'success' }
  if (category === 'degraded') return { label: '降级', tone: 'warning' }
  if (category === 'unhealthy') return { label: '异常', tone: 'error' }

  const diag = nodeHealthDiagnostic(node)
  if (diag) {
    if (
      diag.code === 'unsafe_tls_rejected' ||
      diag.code === 'unsafe_option_rejected' ||
      diag.code === 'private_target_rejected'
    ) {
      return { label: '未知 · 安全拒绝', tone: 'warning' }
    }
    if (
      diag.code === 'credentials_unavailable' ||
      diag.code === 'client_build_failed' ||
      diag.code === 'probe_dialing_not_configured' ||
      diag.code === 'request_build_failed'
    ) {
      return { label: '未知 · 配置待核', tone: 'warning' }
    }
    if (diag.code === 'probe_stale') {
      return { label: '未知 · 待重测', tone: 'info' }
    }
    if (diag.code !== 'probe_missing') {
      return { label: '未知 / 待核验', tone: 'info' }
    }
  }
  return { label: '未探测', tone: 'info' }
}

export function nodeRiskBadge(node: NodeRecord | NormalizedNode): { label: string; tone: ToastTone } {
  const summary = (node as NormalizedNode).ipRiskSummary ?? (node as NodeRecord).ip_risk_summary
  const band = summary?.risk_band
  if (band === 'low') return { label: '低风险', tone: 'success' }
  if (band === 'medium') return { label: '中风险', tone: 'warning' }
  if (band === 'high' || band === 'critical') return { label: '高风险', tone: 'error' }

  const capRisk = extractCapabilityVerdict(node.capabilities?.ip_risk)
  if (capRisk === 'available') return { label: '低风险', tone: 'success' }
  if (capRisk === 'restricted' || capRisk === 'stale') return { label: '中风险', tone: 'warning' }
  if (capRisk === 'error') return { label: '高风险', tone: 'error' }

  return { label: '未探测', tone: 'info' }
}

export function resolveNodeLatencyMs(node: NodeRecord | NormalizedNode): number | null {
  const health = (node as NormalizedNode).healthStatus ?? (node as NodeRecord).health_status
  const category = nodeUnderlyingHealthCategory(node)
  if (
    health === 'unhealthy' ||
    health === 'unknown' ||
    health === 'missing' ||
    category === 'unhealthy' ||
    category === 'unprobed'
  ) {
    return null
  }
  const top = (node as NormalizedNode).latencyMs ?? (node as NodeRecord).latency_ms
  if (typeof top === 'number' && Number.isFinite(top) && top > 0) return top
  const baseDetail = extractCapabilityDetail(node, 'baseline')
  if (
    baseDetail &&
    baseDetail.verdict !== 'error' &&
    !baseDetail.stale &&
    typeof baseDetail.latency_ms === 'number' &&
    Number.isFinite(baseDetail.latency_ms) &&
    baseDetail.latency_ms > 0
  ) {
    return baseDetail.latency_ms
  }
  return null
}

export function formatNodeLatency(node: NodeRecord | NormalizedNode): string {
  const ms = resolveNodeLatencyMs(node)
  if (ms === null || ms <= 0) {
    return '--'
  }
  return `${ms} ms`
}

export function nodeLatencyTone(node: NodeRecord | NormalizedNode): ToastTone {
  const ms = resolveNodeLatencyMs(node)
  if (ms === null || ms <= 0) {
    const health = nodeHealthBadge(node)
    if (health.tone === 'error') return 'error'
    return 'info'
  }
  if (ms < 100) return 'success'
  if (ms <= 250) return 'warning'
  return 'error'
}

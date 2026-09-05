/**
 * Node Ledger Domain Logic & Pure Helpers.
 * Provides deterministic filtering, search, sorting, targeting, chunked probe runner,
 * and dialer proxy plan operations.
 */

export interface LedgerNodeItem {
  name: string
  node_key?: string
  subscription_id?: number | null
  subscription_name?: string | null
  type?: string | null
  server?: string | null
  port?: number | null
  uuid?: string | null
  password?: string | null
  udp?: boolean | null
  cipher?: string | null
  network?: string | null
  tls?: boolean | null
  sni?: string | null
  dialer_proxy?: string | null
  chain_source?: string | null
  group_names?: string[]
  [key: string]: any
}

export interface ProbeRecord {
  name?: string
  node_key?: string
  status?: string
  latency_ms?: number
  speed_mbps?: number
  ip?: string
  country?: string
  asn?: number | string
  organization?: string
  error?: string
  checked_at?: number
  media?: Record<string, { status?: string; unlocked?: boolean; [key: string]: any }>
  [key: string]: any
}

export interface FilterState {
  keyword: string
  subscription: string
  protocol: string
  status: string
  country: string
  chain: string
  minSpeed: number
  mediaPlatforms: string[]
  sortBy: string
}

export interface CountryOption {
  code: string
  flag: string
  name: string
  count: number
}

export interface MediaPlatformDef {
  key: string
  name: string
  short: string
  icon: string
}

export const MEDIA_PLATFORMS: MediaPlatformDef[] = [
  { key: 'chatgpt', name: 'ChatGPT', short: 'GPT', icon: '🤖' },
  { key: 'gemini', name: 'Gemini', short: 'Gemini', icon: '🧠' },
  { key: 'youtube', name: 'YouTube', short: 'YT', icon: '📺' },
  { key: 'netflix', name: 'Netflix', short: 'NF', icon: '🍿' },
  { key: 'disney', name: 'Disney+', short: 'Disney', icon: '🏰' },
  { key: 'meta_ai', name: 'Meta AI', short: 'Meta', icon: '🌐' },
  { key: 'bilibili', name: 'Bilibili', short: 'Bili', icon: '⚡' },
]

export const DISQUALIFIED_VERDICTS = new Set([
  'originals_only',
  'unsupported_region',
  'blocked',
  'challenge',
  'rate_limited',
  'unknown',
])

export const DISQUALIFIED_STATUSES = new Set([
  'partial',
  'originals',
  'originals_only',
  'restricted',
  'ip_blocked',
  'challenged',
  'rate_limited',
  'timeout',
  'transport_error',
  'inconclusive',
  'disabled',
  'fail',
  'failed',
  'blocked',
  'unknown',
])

export const DISQUALIFIED_CONFIDENCES = new Set([
  'conflicted',
  'unavailable',
])

/**
 * Pure predicate determining whether a media probe outcome represents verified full unlock.
 * Strictly disqualifies partial originals_only, restricted, ip_blocked, challenged,
 * rate_limited, timeout, transport_error, and inconclusive.
 * Retains legacy full/ok compatibility until superseded.
 */
export function isMediaFullUnlocked(item: any): boolean {
  if (item === null || item === undefined) return false
  if (typeof item !== 'object') return Boolean(item === true)

  const status = String(item.status || '').toLowerCase().trim()
  const verdict = String(item.verdict || '').toLowerCase().trim()
  const confidence = String(item.confidence || '').toLowerCase().trim()
  const unlocked = item.unlocked

  // 1. Explicit disqualifications
  if (verdict && DISQUALIFIED_VERDICTS.has(verdict)) {
    return false
  }
  if (status && DISQUALIFIED_STATUSES.has(status)) {
    return false
  }
  if (confidence && DISQUALIFIED_CONFIDENCES.has(confidence)) {
    return false
  }

  // 2. Evidence-grade verified checks
  if (status === 'verified' && (verdict === 'full' || verdict === 'available')) {
    return true
  }

  // 3. Legacy compatibility (status == 'full' or status == 'ok')
  if (status === 'full' || status === 'ok') {
    return true
  }

  // 4. Fallback unlocked flag when no negative verdict/status
  if (unlocked === true) {
    if (verdict === 'full' || verdict === 'available' || !verdict) {
      return true
    }
  }

  return false
}

/**
 * Sanitizes evidence and produces a concise summary without leaking
 * secrets, cookies, auth tokens, passwords, proxy credentials, or raw bodies.
 */
export function sanitizeEvidenceSummary(evidence: any): string {
  if (!evidence || typeof evidence !== 'object') return ''
  const signals = Array.isArray(evidence.signals) ? evidence.signals : []
  const safeSignals = signals.filter(
    (s: any) =>
      typeof s === 'string' &&
      !/token|cookie|auth|credential|secret|password|bearer/i.test(s)
  )

  const parts: string[] = []
  if (evidence.http_status != null) {
    parts.push(`HTTP ${evidence.http_status}`)
  }
  if (evidence.redirect_class && evidence.redirect_class !== 'none') {
    parts.push(`重定向: ${evidence.redirect_class}`)
  }
  if (safeSignals.length > 0) {
    parts.push(`信号: ${safeSignals.join(', ')}`)
  }
  if (evidence.elapsed_ms != null && evidence.elapsed_ms > 0) {
    parts.push(`${evidence.elapsed_ms}ms`)
  }
  if (evidence.error_code) {
    parts.push(`错误码: ${evidence.error_code}`)
  }
  return parts.join(' | ')
}

export interface MediaSemanticPresentation {
  isFullUnlocked: boolean
  isPartial: boolean
  isInconclusive: boolean
  badgeVariant: 'success' | 'warning' | 'danger' | 'neutral' | 'info'
  badgeClass: string
  label: string
  shortBadgeText: string
  accessibleTitle: string
  region?: string
  verdict?: string
  status?: string
  confidence?: string
  evidenceVersion?: string
  checkedAt?: number
  sanitizedSignalSummary?: string
}

/**
 * Returns structured semantic presentation for UI badges and diagnostics drawers.
 * Guaranteed:
 * - Only verified full / available receives success badge;
 * - partial originals_only displays '仅自制剧' with warning badge, NEVER success;
 * - inconclusive displays '未定结论' with neutral badge;
 * - restricted / challenged / rate_limited / timeout / transport_error never receive success badges;
 * - historical full/originals/ok maintain backward-compatible labels.
 */
export function getMediaSemanticPresentation(
  item: any,
  platformDef?: { key: string; name: string; short: string }
): MediaSemanticPresentation {
  const pName = platformDef?.name || '未知平台'
  const pShort = platformDef?.short || platformDef?.key?.toUpperCase() || 'PROBE'

  if (!item || typeof item !== 'object') {
    return {
      isFullUnlocked: false,
      isPartial: false,
      isInconclusive: false,
      badgeVariant: 'neutral',
      badgeClass: 'bg-surface-active text-text-sub border border-border-subtle',
      label: '未测',
      shortBadgeText: `${pShort}:未测`,
      accessibleTitle: `${pName}: 未检测`,
    }
  }

  const fullUnlocked = isMediaFullUnlocked(item)
  const status = String(item.status || '').toLowerCase().trim()
  const verdict = String(item.verdict || '').toLowerCase().trim()
  const region = item.region ? String(item.region).toUpperCase().trim() : undefined
  const confidence = item.confidence ? String(item.confidence).toLowerCase().trim() : undefined
  const evidenceVersion = item.evidence_version ? String(item.evidence_version).trim() : undefined
  const checkedAt = typeof item.checked_at === 'number' ? item.checked_at : undefined
  const sanitizedSummary = sanitizeEvidenceSummary(item.evidence)

  const isPartial =
    status === 'partial' ||
    verdict === 'originals_only' ||
    status === 'originals'
  const isInconclusive =
    status === 'inconclusive' || (status === 'unknown' && (verdict === 'unknown' || !verdict))

  let badgeVariant: 'success' | 'warning' | 'danger' | 'neutral' | 'info' = 'neutral'
  let label = '未知'
  let shortBadgeText = `${pShort}:--`
  let accessibleTitle = `${pName}: 未知状态`

  if (fullUnlocked) {
    badgeVariant = 'success'
    label = region || (status === 'full' ? '全解' : '解锁')
    shortBadgeText = region ? `${pShort}:${region}` : (status === 'full' ? `${pShort}:全解` : `${pShort}:OK`)
    accessibleTitle = `${pName}: ${region ? region + ' ' : ''}全解锁 (Verified Full Unlock)`
  } else if (isPartial) {
    badgeVariant = 'warning'
    label = '仅自制剧'
    shortBadgeText = `${pShort}:自制`
    accessibleTitle = `${pName}: 仅自制剧 (Partial Originals Only, 未全解)`
  } else if (isInconclusive) {
    badgeVariant = 'neutral'
    label = '未定结论'
    shortBadgeText = `${pShort}:未定`
    accessibleTitle = `${pName}: 未定结论 (Inconclusive, 契约漂移或信号未知)`
  } else if (status === 'restricted' || verdict === 'unsupported_region') {
    badgeVariant = 'danger'
    label = '地区受限'
    shortBadgeText = `${pShort}:受限`
    accessibleTitle = `${pName}: 地区受限 (Restricted / Unsupported Region)`
  } else if (status === 'ip_blocked' || verdict === 'blocked') {
    badgeVariant = 'danger'
    label = 'IP阻断'
    shortBadgeText = `${pShort}:阻断`
    accessibleTitle = `${pName}: IP阻断 (Blocked)`
  } else if (status === 'challenged' || verdict === 'challenge') {
    badgeVariant = 'warning'
    label = '质询拦截'
    shortBadgeText = `${pShort}:质询`
    accessibleTitle = `${pName}: 质询拦截 (Bot Challenge)`
  } else if (status === 'rate_limited' || verdict === 'rate_limited') {
    badgeVariant = 'warning'
    label = '速率限制'
    shortBadgeText = `${pShort}:限流`
    accessibleTitle = `${pName}: 速率限制 (Rate Limited)`
  } else if (status === 'timeout') {
    badgeVariant = 'neutral'
    label = '超时'
    shortBadgeText = `${pShort}:超时`
    accessibleTitle = `${pName}: 请求超时 (Timeout)`
  } else if (status === 'transport_error') {
    badgeVariant = 'danger'
    label = '传输错误'
    shortBadgeText = `${pShort}:错误`
    accessibleTitle = `${pName}: 节点传输错误 (Transport Error)`
  } else if (status === 'disabled') {
    badgeVariant = 'neutral'
    label = '已禁用'
    shortBadgeText = `${pShort}:禁用`
    accessibleTitle = `${pName}: 已禁用 (Disabled)`
  } else if (status === 'fail' || status === 'failed') {
    badgeVariant = 'danger'
    label = '失败'
    shortBadgeText = `${pShort}:失败`
    accessibleTitle = `${pName}: 检测失败 (Failed)`
  }

  let badgeClass = 'bg-surface-active text-text-sub border border-border-subtle'
  if (badgeVariant === 'success') {
    badgeClass = 'bg-status-success/15 text-status-success border border-status-success/30'
  } else if (badgeVariant === 'warning') {
    badgeClass = 'bg-status-warning/15 text-status-warning border border-status-warning/30'
  } else if (badgeVariant === 'danger') {
    badgeClass = 'bg-status-danger/15 text-status-danger border border-status-danger/30'
  }

  return {
    isFullUnlocked: fullUnlocked,
    isPartial,
    isInconclusive,
    badgeVariant,
    badgeClass,
    label,
    shortBadgeText,
    accessibleTitle,
    region,
    verdict: item.verdict || undefined,
    status: item.status || undefined,
    confidence,
    evidenceVersion,
    checkedAt,
    sanitizedSignalSummary: sanitizedSummary || undefined,
  }
}

export const COUNTRY_NAME_MAP: Record<string, string> = {
  HK: '香港',
  TW: '台湾',
  JP: '日本',
  SG: '新加坡',
  US: '美国',
  KR: '韩国',
  DE: '德国',
  GB: '英国',
  FR: '法国',
  CA: '加拿大',
  AU: '澳大利亚',
  CN: '中国大陆',
}

export const COUNTRY_FLAG_MAP: Record<string, string> = {
  HK: '🇭🇰',
  TW: '🇹🇼',
  JP: '🇯🇵',
  SG: '🇸🇬',
  US: '🇺🇸',
  KR: '🇰🇷',
  DE: '🇩🇪',
  GB: '🇬🇧',
  FR: '🇫🇷',
  CA: '🇨🇦',
  AU: '🇦🇺',
  CN: '🇨🇳',
}

/**
 * Normalizes probe results by indexing on node_key (and fallback name if present).
 * Handles paged envelope { results: Record<string, ProbeRecord>, next_cursor, has_more },
 * raw object dictionary, or array of records.
 */
export function normalizeNodeLedgerMap(
  data: any[] | Record<string, any> | undefined | null,
  targetMap?: Record<string, ProbeRecord>
): Record<string, ProbeRecord> {
  const map: Record<string, ProbeRecord> = targetMap ? { ...targetMap } : {}
  if (!data) return map

  const source =
    data && typeof data === 'object' && 'results' in data && typeof data.results === 'object' && data.results !== null
      ? data.results
      : data

  if (Array.isArray(source)) {
    for (const item of source) {
      if (!item || typeof item !== 'object') continue
      if (item.node_key) map[item.node_key] = item
      if (item.name) map[item.name] = item
    }
  } else if (typeof source === 'object') {
    for (const [k, v] of Object.entries(source)) {
      if (!v || typeof v !== 'object') continue
      const record = v as ProbeRecord
      map[k] = record
      if ((v as any).node_key) map[(v as any).node_key] = record
      if ((v as any).name) map[(v as any).name] = record
    }
  }
  return map
}

/**
 * Merges a newly received probe summary page envelope or map into an existing probe map by node_key.
 */
export function mergeNodeLedgerProbePages(
  existingMap: Record<string, ProbeRecord>,
  newPage: any
): Record<string, ProbeRecord> {
  return normalizeNodeLedgerMap(newPage, existingMap)
}

export type DrawerDetailStatus = 'idle' | 'loading' | 'ready' | 'unavailable'

export interface DrawerDetailSession {
  nodeKey: string | null
  status: DrawerDetailStatus
  probe: any | null
  abortController: AbortController | null
}

export function createDrawerDetailSession(): DrawerDetailSession {
  return {
    nodeKey: null,
    status: 'idle',
    probe: null,
    abortController: null,
  }
}

export function startDrawerDetailFetch(
  session: DrawerDetailSession,
  node: { node_key?: string } | null
): { controller: AbortController | null; shouldFetch: boolean } {
  if (session.abortController) {
    session.abortController.abort()
    session.abortController = null
  }

  if (!node) {
    session.nodeKey = null
    session.status = 'idle'
    session.probe = null
    return { controller: null, shouldFetch: false }
  }

  if (!node.node_key) {
    session.nodeKey = null
    session.status = 'unavailable'
    session.probe = null
    return { controller: null, shouldFetch: false }
  }

  session.nodeKey = node.node_key
  session.status = 'loading'
  session.probe = null
  session.abortController = new AbortController()
  return { controller: session.abortController, shouldFetch: true }
}

export function resolveDrawerDetailFetch(
  session: DrawerDetailSession,
  requestedKey: string,
  result: { data?: any } | null,
  error?: any
): void {
  if (session.nodeKey !== requestedKey) {
    return
  }
  if (error) {
    if (error?.name === 'AbortError' || session.abortController?.signal?.aborted) {
      return
    }
    session.status = 'unavailable'
    return
  }
  if (result && result.data) {
    session.probe = result.data
    session.status = 'ready'
  } else {
    session.status = 'unavailable'
  }
}

/**
 * Gets probe record for a given node by node_key or name.
 */
export function getProbeForNode(
  probes: Record<string, ProbeRecord>,
  node: { name?: string; node_key?: string }
): ProbeRecord | undefined {
  if (!probes || !node) return undefined
  if (node.node_key && probes[node.node_key]) return probes[node.node_key]
  if (node.name && probes[node.name]) return probes[node.name]
  return undefined
}

/**
 * Resolves ISO 2-letter country code from probe or node name.
 */
export function resolveNodeCountryCode(node: LedgerNodeItem, probe?: ProbeRecord): string {
  let code = (probe?.country || '').toUpperCase()
  if (!code) {
    const n = (node?.name || '').toUpperCase()
    if (n.includes('香港') || n.includes('HK')) code = 'HK'
    else if (n.includes('日本') || n.includes('JP')) code = 'JP'
    else if (n.includes('美国') || n.includes('US')) code = 'US'
    else if (n.includes('新加坡') || n.includes('SG')) code = 'SG'
    else if (n.includes('台湾') || n.includes('TW')) code = 'TW'
    else if (n.includes('韩国') || n.includes('KR')) code = 'KR'
    else if (n.includes('德国') || n.includes('DE')) code = 'DE'
    else if (n.includes('英国') || n.includes('GB') || n.includes('UK')) code = 'GB'
    else if (n.includes('中国') || n.includes('CN')) code = 'CN'
    else if (n.includes('法国') || n.includes('FR')) code = 'FR'
    else if (n.includes('加拿大') || n.includes('CA')) code = 'CA'
    else if (n.includes('澳大利亚') || n.includes('AU')) code = 'AU'
  }
  return code
}

/**
 * Derives dynamic filter dropdown options from loaded nodes and probe map.
 */
export function computeFilterOptions(
  nodes: LedgerNodeItem[],
  probes: Record<string, ProbeRecord>
): {
  subscriptions: string[]
  protocols: string[]
  countries: CountryOption[]
} {
  const subSet = new Set<string>()
  const protoSet = new Set<string>()
  const countryMap = new Map<string, number>()

  for (const n of nodes) {
    if (n.subscription_name) subSet.add(n.subscription_name)
    if (n.type) protoSet.add(n.type.toLowerCase())

    const p = getProbeForNode(probes, n)
    const c = resolveNodeCountryCode(n, p)
    if (c) {
      countryMap.set(c, (countryMap.get(c) || 0) + 1)
    }
  }

  const countries: CountryOption[] = Array.from(countryMap.entries())
    .map(([code, count]) => ({
      code,
      flag: COUNTRY_FLAG_MAP[code] || getFlagEmoji(code),
      name: COUNTRY_NAME_MAP[code] || code,
      count,
    }))
    .sort((a, b) => b.count - a.count)

  return {
    subscriptions: Array.from(subSet).sort((a, b) => a.localeCompare(b, 'zh')),
    protocols: Array.from(protoSet).sort(),
    countries,
  }
}

/**
 * Simple ISO country code to flag emoji helper.
 */
export function getFlagEmoji(countryCode: string): string {
  if (!countryCode || countryCode.length !== 2) return '🌐'
  const codePoints = countryCode
    .toUpperCase()
    .split('')
    .map(char => 127397 + char.charCodeAt(0))
  return String.fromCodePoint(...codePoints)
}

/**
 * Pure filter and sort algorithm preserving 100% of legacy criteria.
 */
export function filterAndSortNodes(
  nodes: LedgerNodeItem[],
  probes: Record<string, ProbeRecord>,
  filters: FilterState
): LedgerNodeItem[] {
  if (!Array.isArray(nodes)) return []
  const kw = String(filters?.keyword || '').trim().toLowerCase()
  const sub = filters.subscription
  const proto = (filters.protocol || '').toLowerCase()
  const status = filters.status
  const country = (filters.country || '').toUpperCase()
  const chain = filters.chain
  const minSpeed = filters.minSpeed || 0
  const mediaList = filters.mediaPlatforms || []

  const filtered = nodes.filter(item => {
    const probe = getProbeForNode(probes, item)

    // Subscription
    if (sub && item.subscription_name !== sub) return false

    // Protocol
    if (proto && (item.type || '').toLowerCase() !== proto) return false

    // Status
    if (status && status !== 'all') {
      if (status === 'ok' && probe?.status !== 'ok') return false
      if (status === 'fail' && probe?.status !== 'fail' && probe?.status !== 'timeout') return false
      if (status === 'untested' && probe?.status && probe.status !== 'untested') return false
      if (status === 'fast' && (probe?.status !== 'ok' || (probe?.latency_ms || 9999) > 300)) return false
      if (
        status === 'medium' &&
        (probe?.status !== 'ok' ||
          !probe?.latency_ms ||
          probe.latency_ms <= 300 ||
          probe.latency_ms > 800)
      ) {
        return false
      }
    }

    // Country
    if (country) {
      const c = resolveNodeCountryCode(item, probe)
      if (c !== country) return false
    }

    // Chain
    if (chain === 'chained' && !item.dialer_proxy) return false
    if (chain === 'plain' && item.dialer_proxy) return false

    // Min Speed
    if (minSpeed > 0 && (!probe?.speed_mbps || probe.speed_mbps < minSpeed)) return false

    // Media and AI unlocks (AND logic)
    if (mediaList.length > 0) {
      if (!probe?.media) return false
      for (const mKey of mediaList) {
        const m = probe.media[mKey]
        if (!isMediaFullUnlocked(m)) return false
      }
    }

    // Multi-field search
    if (kw) {
      const searchTarget = [
        item.name,
        item.subscription_name,
        item.dialer_proxy,
        item.type,
        item.server,
        item.port != null ? String(item.port) : '',
        item.cipher,
        item.network,
        item.sni,
        probe?.ip,
        probe?.country,
        probe?.asn ? `AS${probe.asn}` : '',
        probe?.organization,
        ...(item.group_names || []),
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()

      if (!searchTarget.includes(kw)) return false
    }

    return true
  })

  // Sort
  return filtered.sort((a, b) => {
    const pA = getProbeForNode(probes, a)
    const pB = getProbeForNode(probes, b)

    switch (filters.sortBy) {
      case 'latency_asc': {
        const latA = pA?.status === 'ok' ? pA.latency_ms ?? 99999 : 999999
        const latB = pB?.status === 'ok' ? pB.latency_ms ?? 99999 : 999999
        return latA - latB
      }
      case 'latency_desc': {
        const latA = pA?.status === 'ok' ? pA.latency_ms ?? 0 : -1
        const latB = pB?.status === 'ok' ? pB.latency_ms ?? 0 : -1
        return latB - latA
      }
      case 'speed_desc': {
        const spA = pA?.speed_mbps ?? 0
        const spB = pB?.speed_mbps ?? 0
        return spB - spA
      }
      case 'name_asc': {
        return (a.name || '').localeCompare(b.name || '', 'zh')
      }
      case 'country': {
        const cA = pA?.country || resolveNodeCountryCode(a, pA) || ''
        const cB = pB?.country || resolveNodeCountryCode(b, pB) || ''
        return cA.localeCompare(cB)
      }
      case 'checked_desc': {
        return (pB?.checked_at ?? 0) - (pA?.checked_at ?? 0)
      }
      default:
        return 0
    }
  })
}

/**
 * Toggles or applies metrics shortcut filters.
 */
export function applyMetricShortcut(
  filters: FilterState,
  metric: 'all' | 'healthy' | 'fast' | 'chained'
): FilterState {
  const next = { ...filters }
  if (metric === 'all') {
    next.status = 'all'
    next.chain = 'all'
    next.minSpeed = 0
    next.country = ''
    next.keyword = ''
    next.subscription = ''
    next.protocol = ''
    next.mediaPlatforms = []
  } else if (metric === 'healthy') {
    next.status = next.status === 'ok' ? 'all' : 'ok'
  } else if (metric === 'fast') {
    next.minSpeed = next.minSpeed > 0 ? 0 : 10
  } else if (metric === 'chained') {
    next.chain = next.chain === 'chained' ? 'all' : 'chained'
  }
  return next
}

/**
 * Yields batch targets: if selection is non-empty, use selection;
 * otherwise target the entire filtered inventory (independent of virtual scroll).
 */
export function buildEffectiveBatchTargets(
  filteredNodes: LedgerNodeItem[],
  selectedNames: Set<string>,
  allNodes?: LedgerNodeItem[]
): LedgerNodeItem[] {
  if (selectedNames && selectedNames.size > 0) {
    const source = allNodes && allNodes.length ? allNodes : filteredNodes
    return source.filter(n => selectedNames.has(n.name))
  }
  return filteredNodes
}

/**
 * Splits array into chunks for progressive dispatch and live UI telemetry.
 */
export function chunkItems<T>(items: T[], chunkSize: number): T[][] {
  if (chunkSize <= 0) return [items]
  const chunks: T[][] = []
  for (let i = 0; i < items.length; i += chunkSize) {
    chunks.push(items.slice(i, i + chunkSize))
  }
  return chunks
}

export interface ProbeBatchChunkOptions {
  chunkSize?: number
  signal?: AbortSignal
  probeChunkFn: (chunk: LedgerNodeItem[]) => Promise<any[]>
  onProgress?: (progress: { done: number; total: number; ok: number; fail: number }) => void
}

/**
 * Runs batch probing in chunks, checking signal before each chunk to allow interruption.
 */
export async function runChunkedBatchProbe(
  targetNodes: LedgerNodeItem[],
  options: ProbeBatchChunkOptions
): Promise<{ results: any[]; stopped: boolean }> {
  const chunkSize = options.chunkSize || 10
  const chunks = chunkItems(targetNodes, chunkSize)
  const results: any[] = []
  let ok = 0
  let fail = 0
  let done = 0
  let stopped = false

  for (const chunk of chunks) {
    if (options.signal?.aborted) {
      stopped = true
      break
    }
    const chunkResults = await options.probeChunkFn(chunk)
    for (const r of chunkResults) {
      results.push(r)
      if (r?.status === 'ok') ok++
      else fail++
      done++
    }
    options.onProgress?.({ done, total: targetNodes.length, ok, fail })
  }

  return { results, stopped: stopped || (options.signal?.aborted ?? false) }
}

/**
 * Calculates plan for replacing a node-level dialer binding.
 */
export function planDialerReplacement(
  bindings: any[],
  targetNodeName: string,
  newDialer: { dialer_type: string; dialer_ref: string }
): {
  bindingsToDelete: number[]
  payloadToCreate: any
} {
  const bindingsToDelete = (bindings || [])
    .filter(b => b.target_type === 'node' && b.target_name === targetNodeName)
    .map(b => b.id)

  const payloadToCreate = {
    target_type: 'node',
    target_name: targetNodeName,
    dialer_type: newDialer.dialer_type,
    dialer_ref: newDialer.dialer_ref,
    enabled: true,
    note: 'configured via node workbench',
  }

  return {
    bindingsToDelete,
    payloadToCreate,
  }
}

export interface ReplaceNodeDialerOptions {
  nodeName: string
  dialerType: string
  dialerRef: string
  existingBindings: any[]
  deleteBindingFn: (id: number) => Promise<any>
  createBindingFn: (data: any) => Promise<any>
  note?: string
}

/**
 * Replaces node dialer proxy by first deleting existing node-level bindings then creating the new binding.
 */
export async function replaceNodeDialerProxy(options: ReplaceNodeDialerOptions): Promise<any> {
  const plan = planDialerReplacement(
    options.existingBindings,
    options.nodeName,
    { dialer_type: options.dialerType, dialer_ref: options.dialerRef }
  )
  if (options.note) {
    plan.payloadToCreate.note = options.note
  }
  await Promise.all(plan.bindingsToDelete.map(id => options.deleteBindingFn(id)))
  return await options.createBindingFn(plan.payloadToCreate)
}

/**
 * Clears node-level dialer proxy by deleting existing node-level bindings.
 */
export async function clearNodeDialerProxy(
  nodeName: string,
  existingBindings: any[],
  deleteBindingFn: (id: number) => Promise<any>
): Promise<void> {
  const toDelete = (existingBindings || [])
    .filter(b => b.target_type === 'node' && b.target_name === nodeName)
    .map(b => b.id)
  await Promise.all(toDelete.map(id => deleteBindingFn(id)))
}

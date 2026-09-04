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
 * Normalizes probe results by dual-indexing on name and node_key.
 */
export function normalizeNodeLedgerMap(
  data: any[] | Record<string, any> | undefined | null
): Record<string, ProbeRecord> {
  const map: Record<string, ProbeRecord> = {}
  if (!data) return map

  if (Array.isArray(data)) {
    for (const item of data) {
      if (!item || typeof item !== 'object') continue
      if (item.name) map[item.name] = item
      if (item.node_key) map[item.node_key] = item
    }
  } else if (typeof data === 'object') {
    for (const [k, v] of Object.entries(data)) {
      if (!v || typeof v !== 'object') continue
      map[k] = v as ProbeRecord
      if ((v as any).name) map[(v as any).name] = v as ProbeRecord
      if ((v as any).node_key) map[(v as any).node_key] = v as ProbeRecord
    }
  }
  return map
}

/**
 * Gets probe record for a given node by name or node_key.
 */
export function getProbeForNode(
  probes: Record<string, ProbeRecord>,
  node: { name?: string; node_key?: string }
): ProbeRecord | undefined {
  if (!probes || !node) return undefined
  if (node.name && probes[node.name]) return probes[node.name]
  if (node.node_key && probes[node.node_key]) return probes[node.node_key]
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
  const kw = String(filters.keyword || '').trim().toLowerCase()
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
        if (!m) return false
        const ok =
          m.status === 'ok' ||
          m.status === 'full' ||
          m.status === 'originals' ||
          m.unlocked === true
        if (!ok) return false
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

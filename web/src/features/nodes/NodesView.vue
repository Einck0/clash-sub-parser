<script setup lang="ts">
import { computed, inject, nextTick, onMounted, onUnmounted, ref, watch, type VNodeRef } from 'vue'
import { matchedRouteKey, onBeforeRouteLeave, onBeforeRouteUpdate, routeLocationKey, routerKey } from 'vue-router'
import {
  ExclamationTriangleIcon,
  EyeIcon,
} from '@heroicons/vue/24/outline'
import { useWindowVirtualizer } from '@tanstack/vue-virtual'
import { useElementSize, useResizeObserver } from '@vueuse/core'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import NodeListToolbar from './NodeListToolbar.vue'
import NodeCompactRow from './NodeCompactRow.vue'
import NodeDetailDrawer from './NodeDetailDrawer.vue'
import NodeProbeHistoryDrawer from './NodeProbeHistoryDrawer.vue'
import { toastStore } from '../../ui/toast'
import type { NodeSourceHistoryData } from './sourceHistoryTypes'
import { ApiError } from '../../api/client'
import {
  SUPPORTED_NODE_PROTOCOLS,
  nodeUnderlyingHealthCategory,
  resolveNodeProbeState,
  type NormalizedNode,
} from './nodeView'
import { parseSpeedMinimum, useNodes, type HealthFilterType } from './useNodes'
import { IP_RISK_BANDS, IP_RISK_BAND_LABELS } from '../policy/policyTypes'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayText } from '../../utils/displayNames'

const GAP = 8
const ESTIMATED_ROW_HEIGHT = 56

const {
  items,
  loading,
  loadingMore,
  loadingDetail,
  nodeDetailError,
  isDetailStale,
  detailErrorMessage,
  savingConnection,
  probingNodeId,
  speedingNodeId,
  measureNodeSpeed,
  probingNodeIds,
  queuedNodeIds,
  probePendingIds,
  probeStates,
  pendingResultRefreshIds,
  resultRefreshFailedIds,
  poolError,
  appendError,
  error,
  total,
  hasMore,
  initialLoaded,
  protocolFilter,
  healthFilter,
  riskBandFilter,
  riskScoreMax,
  speedMinKiBS,
  searchQuery,
  selectedNode,
  load: loadNodes,
  loadMore,
  syncPoolStatus,
  fetchNodeDetail,
  invalidateNodeDetail,
  fetchNodeSourceHistory,
  fetchGlobalScheduleKinds,
  probeSingleNode,
  probeNodesBatch,
  updateNodeConnection,
} = useNodes()

const route = inject(routeLocationKey, undefined)
const router = inject(routerKey, undefined)

function queryNodeId(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

// URL query normalization and safe fallbacks
function parseQueryProtocol(queryProto: unknown): string {
  if (typeof queryProto === 'string') {
    const norm = queryProto.trim().toLowerCase()
    if (norm === 'all' || (SUPPORTED_NODE_PROTOCOLS as readonly string[]).includes(norm)) {
      return norm
    }
  }
  return 'all'
}

function parseQueryHealth(queryHealth: unknown): HealthFilterType {
  if (typeof queryHealth === 'string') {
    const norm = queryHealth.trim().toLowerCase()
    const valid: HealthFilterType[] = [
      'all',
      'probing',
      'healthy',
      'degraded',
      'unhealthy',
      'undetermined',
      'untested',
      'unknown',
    ]
    if (valid.includes(norm as HealthFilterType)) {
      return norm as HealthFilterType
    }
  }
  return 'all'
}

function parseQueryRiskBand(value: unknown): string {
  return typeof value === 'string' && (IP_RISK_BANDS as readonly string[]).includes(value) ? value : 'all'
}

function parseQueryRiskScore(value: unknown): string {
  return typeof value === 'string' && /^(0|[1-9]\d?|100)$/.test(value) ? value : ''
}

function parseQuerySearch(querySearch: unknown): string {
  if (typeof querySearch === 'string') {
    return querySearch.trim()
  }
  return ''
}

function syncUrlFromState() {
  if (!router || !route) return
  const query: Record<string, string> = {}

  if (searchQuery.value.trim()) {
    query.q = searchQuery.value.trim()
  }
  if (protocolFilter.value && protocolFilter.value !== 'all') {
    query.protocol = protocolFilter.value
  }
  if (healthFilter.value && healthFilter.value !== 'all') {
    query.health = healthFilter.value
  }
  if (riskBandFilter.value !== 'all') query.ip_risk_band = riskBandFilter.value
  if (riskScoreMax.value !== '') query.ip_risk_score_max = riskScoreMax.value
  if (speedMinKiBS.value !== '') query.speed_min_kib_s = speedMinKiBS.value
  if (drawerOpen.value && selectedNode.value?.logicalId) {
    query.node = selectedNode.value.logicalId
  }

  const currentQuery = route.query || {}
  const allKeys = new Set([...Object.keys(currentQuery), ...Object.keys(query)])
  let hasDiff = false
  for (const k of allKeys) {
    if ((currentQuery[k] || '') !== (query[k] || '')) {
      hasDiff = true
      break
    }
  }

  if (hasDiff) {
    void Promise.resolve(router.replace({ query })).catch(() => {})
  }
}

const drawerOpen = ref(false)
const nodeDrawer = ref<InstanceType<typeof NodeDetailDrawer> | null>(null)
const routeRecord = inject(matchedRouteKey, null)
if (routeRecord?.value) {
  // Finish the in-flight save before allowing navigation; the drawer owns discard prompts.
  onBeforeRouteLeave(() => !drawerOpen.value || !savingConnection.value)
  onBeforeRouteUpdate((to, from) => {
    const targetId = queryNodeId(to.query.node)
    if (targetId === queryNodeId(from.query.node) || !drawerOpen.value || targetId === selectedNode.value?.logicalId) return true
    return nodeDrawer.value?.confirmNavigation() ?? true
  })
}
const probeFeedbackTarget = ref('')
const speedFeedbackTarget = ref('')
const probeFeedback = ref('')
const selectedProbeFeedback = computed(() => {
  const id = selectedNode.value?.logicalId
  if (!id) return ''
  if (probePendingIds.value.has(id)) return uiText('正在提交普通检测…', 'Submitting probe…')
  const state = probeStates.value[id]
  if (state === 'queued') return uiText('已排队等待普通检测，尚未完成。', 'Queued for probing; not completed yet.')
  if (state === 'running') return uiText('普通检测仍在运行，完成后自动刷新。', 'Probe is still running. Results refresh when it ends.')
  if (state === 'failed') return uiText('普通检测失败，请查看检测历史。', 'Probe failed. See probe history.')
  if (state === 'cancelled') return uiText('普通检测已取消。', 'Probe cancelled.')
  if (state === 'expired') return uiText('普通检测已超时。', 'Probe expired.')
  if (state === 'succeeded') return pendingResultRefreshIds.value.has(id) || resultRefreshFailedIds.value.has(id)
    ? uiText('普通检测已完成，但结果尚未刷新，请重试详情。', 'Probe completed, but results have not refreshed. Retry node details.')
    : uiText('普通检测已完成；带宽需单独测速。', 'Probe completed; bandwidth requires a separate measurement.')
  return probeFeedbackTarget.value === id ? probeFeedback.value : ''
})
const speedTarget = ref<NormalizedNode | null>(null)
const speedFeedback = ref('')
const speedFailedIds = ref<Set<string>>(new Set())

function requestSpeed(node: NormalizedNode) {
  if (speedingNodeId.value || !node.active) return
  speedTarget.value = node
}

async function confirmSpeed() {
  const node = speedTarget.value
  if (!node || speedingNodeId.value) return
  speedTarget.value = null
  speedFailedIds.value = new Set([...speedFailedIds.value].filter((id) => id !== node.logicalId))
  speedFeedbackTarget.value = node.logicalId
  speedFeedback.value = uiText(`正在对「${node.displayName}」进行单次带宽测速…`, `Measuring bandwidth once for “${node.displayName}”…`)
  const result = await measureNodeSpeed(node.logicalId)
  if (speedMinKiBS.value !== '') await load()
  if (!result.ok) speedFailedIds.value = new Set([...speedFailedIds.value, node.logicalId])
  speedFeedback.value = result.ok ? uiText(`「${node.displayName}」测速结果已保存并刷新。`, `Bandwidth result for “${node.displayName}” was saved and refreshed.`) : (result.error || uiText('本次测速失败，请重试。', 'Bandwidth measurement failed. Retry.'))
}
const sourceHistory = ref<NodeSourceHistoryData | null>(null)
const loadingSourceHistory = ref(false)
const sourceHistoryError = ref('')
const currentHistoryTargetId = ref('')
const deepLinkError = ref('')
const lastDeepLinkId = ref('')
const loadingDeepLink = ref(false)
let deepLinkSeq = 0

function invalidateDeepLink() {
  deepLinkSeq++
  invalidateNodeDetail()
  loadingDeepLink.value = false
  deepLinkError.value = ''
  lastDeepLinkId.value = ''
}

const selectedIds = ref<Set<string>>(new Set())
const confirmProbeAllOpen = ref(false)
const confirmProbeSelectedOpen = ref(false)
const historyDrawerOpen = ref(false)

function toggleSelectNode(id: string) {
  const set = new Set(selectedIds.value)
  if (set.has(id)) {
    set.delete(id)
  } else {
    set.add(id)
  }
  selectedIds.value = set
}

const isAllVisibleSelected = computed(() => {
  if (filteredItems.value.length === 0) return false
  return filteredItems.value.every((n) => selectedIds.value.has(n.logicalId))
})

const visibleSelectedCount = computed(() => {
  return filteredItems.value.filter((n) => selectedIds.value.has(n.logicalId)).length
})

const selectedScopeConfirmMessage = computed(() => {
  const total = selectedIds.value.size
  const visible = visibleSelectedCount.value
  const hidden = total - visible
  return uiText(
    `当前已选 ${total} 个节点（其中当前可见 ${visible} 项，其他页面或筛选隐藏 ${hidden} 项）。确认对全部 ${total} 个节点发起普通检测？`,
    `Selected ${total} nodes (${visible} visible, ${hidden} hidden by other pages or filters). Probe all ${total} nodes?`
  )
})

function toggleSelectAllVisible() {
  const set = new Set(selectedIds.value)
  if (isAllVisibleSelected.value) {
    for (const n of filteredItems.value) {
      set.delete(n.logicalId)
    }
  } else {
    for (const n of filteredItems.value) {
      set.add(n.logicalId)
    }
  }
  selectedIds.value = set
}

function clearSelection() {
  selectedIds.value = new Set()
}

function clearInvisibleSelections() {
  const visibleIds = new Set(filteredItems.value.map((n) => n.logicalId))
  const next = new Set<string>()
  for (const id of selectedIds.value) {
    if (visibleIds.has(id)) {
      next.add(id)
    }
  }
  selectedIds.value = next
}

function pruneInvalidSelectedIds() {
  if (selectedIds.value.size === 0) return
  // Only prune if we have loaded the full unfiltered dataset and know the exact total
  if (
    !hasMore.value &&
    !loading.value &&
    items.value.length > 0 &&
    healthFilter.value === 'all' &&
    protocolFilter.value === 'all' &&
    riskBandFilter.value === 'all' &&
    riskScoreMax.value === '' &&
    speedMinKiBS.value === '' &&
    !searchQuery.value.trim()
  ) {
    const validIds = new Set(items.value.map((n) => n.logicalId))
    const next = new Set<string>()
    for (const id of selectedIds.value) {
      if (validIds.has(id)) next.add(id)
    }
    if (next.size !== selectedIds.value.size) {
      selectedIds.value = next
    }
  }
}

watch(items, () => {
  pruneInvalidSelectedIds()
})

function handleProbeSelectedClick() {
  if (selectedIds.value.size === 0) {
    toastStore.push({
      message: t('nodes.noNodeSelected') || uiText('请先勾选需要检测的节点', 'Select nodes to probe first'),
      tone: 'warning',
    })
    return
  }
  // When there are selections outside the current visible filter/page, confirm scope
  if (selectedIds.value.size > visibleSelectedCount.value) {
    confirmProbeSelectedOpen.value = true
    return
  }
  executeProbeSelected()
}

async function executeProbeSelected() {
  confirmProbeSelectedOpen.value = false
  const ids = Array.from(selectedIds.value)
  const res = await probeNodesBatch(ids)
  if (res.ok) {
    toastStore.push({
      message: uiText(
        `已对所选 ${ids.length} 个节点提交普通检测`,
        `Submitted probes for ${ids.length} selected nodes`
      ),
      tone: 'success',
    })
  } else {
    toastStore.push({ message: res.error || uiText('提交普通检测失败', 'Could not submit probes.'), tone: 'error' })
  }
}

function handleProbeAllClick() {
  confirmProbeAllOpen.value = true
}

async function confirmExecuteProbeAll() {
  confirmProbeAllOpen.value = false
  const res = await probeNodesBatch([])
  if (res.ok) {
    toastStore.push({
      message: uiText('已提交全部启用订阅节点的普通检测，不包含带宽测速。', 'Submitted probes for all enabled-subscription nodes. Bandwidth is not measured.'),
      tone: 'success',
    })
  } else {
    toastStore.push({ message: res.error || uiText('全量检测提交失败', 'Could not submit fleet probes.'), tone: 'error' })
  }
}

function getNodeProbeState(node: NormalizedNode): 'probing' | 'queued' | 'idle' {
  return resolveNodeProbeState(node, probingNodeIds.value, queuedNodeIds.value)
}

const healthCounts = computed(() => {
  let probing = 0
  let healthy = 0
  let unhealthy = 0
  let unknown = 0

  for (const node of items.value) {
    const live = getNodeProbeState(node)
    if (live === 'probing' || live === 'queued') {
      probing += 1
    }
    const cat = nodeUnderlyingHealthCategory(node)
    if (cat === 'healthy') healthy += 1
    else if (cat === 'unhealthy') unhealthy += 1
    else unknown += 1
  }

  return {
    all: items.value.length,
    probing,
    healthy,
    unhealthy,
    unknown,
    untested: unknown,
  }
})

// The API applies all active filters before counting and paginating.
const filteredItems = computed(() => items.value)

const appliedSearchQuery = ref('')

function load() {
  appliedSearchQuery.value = searchQuery.value.trim()
  return loadNodes()
}

const activeFilterSummary = computed(() => {
  const conditions: string[] = []
  if (protocolFilter.value !== 'all') {
    conditions.push(`${uiText('协议', 'Protocol')}: ${protocolFilter.value.toUpperCase()}`)
  }
  if (healthFilter.value !== 'all') {
    const labels: Record<string, string> = {
      probing: uiText('检测中', 'Probing'),
      healthy: uiText('正常', 'Healthy'),
      degraded: uiText('降级', 'Degraded'),
      unhealthy: uiText('异常', 'Unhealthy'),
      undetermined: uiText('未知', 'Unknown'),
      untested: uiText('未知', 'Unknown'),
      unknown: uiText('未知', 'Unknown'),
    }
    conditions.push(`${uiText('状态', 'Health')}: ${labels[healthFilter.value] || healthFilter.value}`)
  }
  if (riskBandFilter.value !== 'all') {
    const band = riskBandFilter.value as keyof typeof IP_RISK_BAND_LABELS
    conditions.push(`${uiText('IP 风险', 'IP risk')}: ${IP_RISK_BAND_LABELS[band] || band}`)
  }
  if (riskScoreMax.value !== '') {
    conditions.push(`${uiText('风险分数上限', 'Maximum risk score')}: ${riskScoreMax.value}`)
  }
  if (speedMinKiBS.value !== '') {
    conditions.push(`${uiText('最低测速', 'Minimum speed')}: ${speedMinKiBS.value} KiB/s`)
  }
  if (appliedSearchQuery.value) {
    conditions.push(`${uiText('搜索', 'Search')}: ${appliedSearchQuery.value}`)
  }
  return conditions.join(' · ')
})

async function handleProbeSelectedNode(node: NormalizedNode) {
  probeFeedbackTarget.value = node.logicalId
  probeFeedback.value = uiText(`正在为「${node.displayName}」提交普通检测…`, `Submitting a probe for “${node.displayName}”…`)
  const res = await probeSingleNode(node.logicalId)
  if (selectedNode.value?.logicalId !== node.logicalId) return
  if (!res.ok) {
    probeFeedback.value = res.error || uiText('普通检测提交失败', 'Probe submission failed.')
  } else if (res.state === 'queued') {
    probeFeedback.value = uiText('已排队等待检测，不代表检测已完成。', 'Queued for probing; this does not mean the probe has completed.')
  } else if (res.state === 'running') {
    probeFeedback.value = uiText('普通检测仍在运行，结果完成后会自动刷新。', 'The probe is still running. Results will refresh when it finishes.')
  } else if (res.state === 'succeeded') {
    probeFeedback.value = res.refreshed
      ? uiText('普通检测已完成并刷新结果；带宽需单独测速。', 'Probe completed and results refreshed; bandwidth requires a separate measurement.')
      : uiText('普通检测已完成，但结果刷新失败，请重试详情。', 'Probe completed, but result refresh failed. Retry node details.')
  } else {
    probeFeedback.value = uiText('检测状态尚未确认，请查看检测历史。', 'Probe completion is not confirmed. Check probe history.')
  }
}

let sourceHistorySeq = 0

async function fetchSourceHistory(logicalId: string) {
  const seq = ++sourceHistorySeq
  currentHistoryTargetId.value = logicalId
  sourceHistory.value = null
  sourceHistoryError.value = ''
  loadingSourceHistory.value = true
  try {
    const data = await fetchNodeSourceHistory(logicalId)
    if (!data) throw new Error(uiText('来源历史响应无效，请重试。', 'Invalid source history response. Retry.'))
    if (seq === sourceHistorySeq && currentHistoryTargetId.value === logicalId) {
      sourceHistory.value = data
    }
  } catch (err: any) {
    if (seq === sourceHistorySeq && currentHistoryTargetId.value === logicalId) {
      if (err?.status === 404 || (err instanceof ApiError && err.status === 404)) {
        sourceHistoryError.value = uiText('未找到该节点的来源历史记录 (404)', 'Source history for this node was not found (404)')
      } else if (err?.status === 401 || (err instanceof ApiError && err.status === 401)) {
        sourceHistoryError.value = uiText('鉴权失败，无法获取来源历史', 'Authentication failed. Source history could not be loaded.')
      } else {
        sourceHistoryError.value = displayText(err instanceof Error ? err.message : uiText('获取来源历史失败', 'Failed to load source history'))
      }
    }
  } finally {
    if (seq === sourceHistorySeq && currentHistoryTargetId.value === logicalId) {
      loadingSourceHistory.value = false
    }
  }
}

async function openNodeDetail(node: NormalizedNode, navigationConfirmed = false) {
  if (!navigationConfirmed && drawerOpen.value && selectedNode.value?.logicalId !== node.logicalId && nodeDrawer.value?.confirmNavigation() === false) return
  invalidateDeepLink()
  selectedNode.value = node
  probeFeedback.value = ''
  drawerOpen.value = true
  syncUrlFromState()

  await Promise.all([
    (async () => {
      await fetchNodeDetail(node.logicalId)
    })(),
    fetchSourceHistory(node.logicalId),
  ])
}

async function openNodeByLogicalId(logicalId: string) {
  if (!logicalId) return
  const seq = ++deepLinkSeq
  deepLinkError.value = ''
  lastDeepLinkId.value = logicalId

  const localMatch = items.value.find((n) => n.logicalId === logicalId)
  if (localMatch) {
    await openNodeDetail(localMatch, true)
    return
  }

  loadingDeepLink.value = true
  try {
    const detailed = await fetchNodeDetail(logicalId)
    if (seq !== deepLinkSeq) return
    if (detailed && (!nodeDetailError.value || nodeDetailError.value.logicalId !== logicalId || !nodeDetailError.value.status)) {
      await openNodeDetail(detailed, true)
    } else {
      const err = nodeDetailError.value
      if (err && err.logicalId === logicalId) {
        if (err.status === 404) {
          deepLinkError.value = uiText('未找到该节点（可能已移除），请刷新台账 (404)', 'Node not found (it may have been removed). Refresh the ledger (404)')
        } else if (err.status >= 500) {
          deepLinkError.value = uiText(`获取节点详情失败 (${err.status})`, `Failed to load node details (${err.status})`)
        } else if (err.status === 0) {
          deepLinkError.value = uiText('网络连接失败，请检查网络后重试', 'Network connection failed. Check your connection and retry.')
        } else {
          deepLinkError.value = displayText(err.message) || uiText(`获取节点详情失败 (${err.status})`, `Failed to load node details (${err.status})`)
        }
      } else {
        deepLinkError.value = uiText('未找到该节点（可能已移除），请刷新台账 (404)', 'Node not found (it may have been removed). Refresh the ledger (404)')
      }
    }
  } catch (err: any) {
    if (seq !== deepLinkSeq) return
    const status = err?.status || (err instanceof ApiError ? err.status : 0)
    if (status === 404) {
      deepLinkError.value = uiText('未找到该节点（可能已移除），请刷新台账 (404)', 'Node not found (it may have been removed). Refresh the ledger (404)')
    } else if (status >= 500) {
      deepLinkError.value = uiText(`获取节点详情失败 (${status})`, `Failed to load node details (${status})`)
    } else if (status === 0) {
      deepLinkError.value = uiText('网络连接失败，请检查网络后重试', 'Network connection failed. Check your connection and retry.')
    } else {
      deepLinkError.value = displayText(err?.message) || uiText(`获取节点详情失败 (${status})`, `Failed to load node details (${status})`)
    }
    if (status === 404 && selectedIds.value.has(logicalId)) {
      const next = new Set(selectedIds.value)
      next.delete(logicalId)
      selectedIds.value = next
    }
  } finally {
    if (seq === deepLinkSeq) {
      loadingDeepLink.value = false
    }
  }
}

async function handleSaveConnectionPayload({
  draft,
  onSuccess,
  onError,
}: {
  draft: any
  onSuccess: (node: NormalizedNode) => void
  onError: (err: string) => void
}) {
  if (!selectedNode.value) return
  const res = await updateNodeConnection(selectedNode.value.logicalId, draft)
  if (!res.ok) {
    onError(res.error || uiText('连接参数无效', 'Invalid connection parameters'))
    return
  }
  if (res.node) {
    onSuccess(res.node)
    if (speedMinKiBS.value !== '') await load()
  }
}

function handleProtocolFilterChange(proto: string) {
  protocolFilter.value = proto
  load()
  syncUrlFromState()
}

function handleHealthFilterChange(health: string) {
  healthFilter.value = health as HealthFilterType
  load()
  syncUrlFromState()
}

function handleRiskBandChange(band: string) {
  riskBandFilter.value = parseQueryRiskBand(band)
  load()
  syncUrlFromState()
}

function handleRiskScoreChange(score: string) {
  if (score !== '' && parseQueryRiskScore(score) === '') {
    toastStore.push({ tone: 'error', message: uiText('风险分数上限必须为 0 到 100 的整数', 'Maximum risk score must be an integer from 0 to 100') })
    return
  }
  riskScoreMax.value = score
  load()
  syncUrlFromState()
}

function handleSpeedMinChange(speed: string) {
  speedMinKiBS.value = speed
  load()
  syncUrlFromState()
}

function resetFilters() {
  protocolFilter.value = 'all'
  healthFilter.value = 'all'
  riskBandFilter.value = 'all'
  riskScoreMax.value = ''
  speedMinKiBS.value = ''
  searchQuery.value = ''
  load()
  syncUrlFromState()
}

function handleSearchQueryChange(query: string) {
  searchQuery.value = query
}

function handleSearchSubmit() {
  load()
  syncUrlFromState()
}

const containerRef = ref<HTMLElement | null>(null)
const { width } = useElementSize(containerRef)
const rowCount = computed(() => filteredItems.value.length)

function getRowItems(rowIndex: number) {
  return filteredItems.value.slice(rowIndex, rowIndex + 1)
}

const scrollMargin = ref(0)

function updateScrollMargin() {
  if (containerRef.value && typeof window !== 'undefined') {
    const rect = containerRef.value.getBoundingClientRect()
    scrollMargin.value = Math.max(0, rect.top + window.scrollY)
  }
}

useResizeObserver(containerRef, updateScrollMargin)

const rowVirtualizer = useWindowVirtualizer(
  computed(() => ({
    count: rowCount.value,
    estimateSize: () => ESTIMATED_ROW_HEIGHT + GAP,
    overscan: 4,
    scrollMargin: scrollMargin.value,
    scrollToFn: (offset, options) => {
      if (
        typeof window !== 'undefined' &&
        typeof window.scrollTo === 'function' &&
        !navigator.userAgent.includes('jsdom')
      ) {
        window.scrollTo({ top: offset, behavior: options?.behavior })
      }
    },
  }))
)

const measureRow: VNodeRef = (element) => {
  if (element instanceof HTMLElement) {
    rowVirtualizer.value.measureElement(element)
  }
}

const MAX_AUTO_PAGE_STEPS = 3
const consecutiveAutoPages = ref(0)
const isAutoPaginating = ref(false)

async function checkViewportStarvation() {
  if (typeof window === 'undefined') return
  if (healthFilter.value === 'all') return
  if (loading.value || loadingMore.value || isAutoPaginating.value || appendError.value) return
  if (!hasMore.value) return

  if (filteredItems.value.length < 10 && consecutiveAutoPages.value < MAX_AUTO_PAGE_STEPS) {
    consecutiveAutoPages.value++
    isAutoPaginating.value = true
    try {
      await loadMore()
    } finally {
      isAutoPaginating.value = false
      nextTick(() => {
        checkViewportStarvation()
      })
    }
  }
}

function manualLoadMore() {
  consecutiveAutoPages.value = 0
  loadMore()
}

watch(width, () => {
  nextTick(() => {
    rowVirtualizer.value.measure()
  })
})

watch(healthFilter, () => {
  consecutiveAutoPages.value = 0
  nextTick(() => {
    rowVirtualizer.value.measure()
    checkViewportStarvation()
  })
})

watch(filteredItems, (newItems) => {
  if (newItems.length >= 10) {
    consecutiveAutoPages.value = 0
  } else {
    checkViewportStarvation()
  }
})

function onScroll() {
  if (typeof window === 'undefined') return
  const scrollPosition = window.innerHeight + window.scrollY
  const threshold = document.documentElement.scrollHeight - 420
  if (scrollPosition >= threshold && !appendError.value) {
    consecutiveAutoPages.value = 0
    loadMore()
  }
}

watch(drawerOpen, (isOpen) => {
  if (!isOpen) {
    syncUrlFromState()
    sourceHistorySeq++
    currentHistoryTargetId.value = ''
    invalidateNodeDetail()
    sourceHistory.value = null
    sourceHistoryError.value = ''
    loadingSourceHistory.value = false
  }
})

if (route) {
  watch(
    () => queryNodeId(route.query?.node),
    async (targetId) => {
      if (!targetId) {
        invalidateDeepLink()
        drawerOpen.value = false
        return
      }
      if (selectedNode.value?.logicalId === targetId && drawerOpen.value) {
        return
      }
      await openNodeByLogicalId(targetId)
    }
  )

  watch(
    () => [route.query?.q, route.query?.protocol, route.query?.health, route.query?.ip_risk_band, route.query?.ip_risk_score_max, route.query?.speed_min_kib_s],
    ([newQ, newProto, newHealth, newRiskBand, newRiskScore, newSpeed]) => {
      const safeQ = parseQuerySearch(newQ)
      const safeProto = parseQueryProtocol(newProto)
      const safeHealth = parseQueryHealth(newHealth)

      let needReload = false
      if (safeQ !== searchQuery.value) {
        searchQuery.value = safeQ
        needReload = true
      }
      if (safeProto !== protocolFilter.value) {
        protocolFilter.value = safeProto
        needReload = true
      }
      if (safeHealth !== healthFilter.value) {
        healthFilter.value = safeHealth
        needReload = true
      }
      const safeBand = parseQueryRiskBand(newRiskBand)
      const safeScore = parseQueryRiskScore(newRiskScore)
      if (safeBand !== riskBandFilter.value || safeScore !== riskScoreMax.value) {
        riskBandFilter.value = safeBand
        riskScoreMax.value = safeScore
        needReload = true
      }
      const safeSpeed = parseSpeedMinimum(newSpeed)
      if (safeSpeed !== speedMinKiBS.value) {
        speedMinKiBS.value = safeSpeed
        needReload = true
      }
      if (needReload) {
        load()
      }
    }
  )
}

let poolPollTimer: ReturnType<typeof setInterval> | null = null
let disposed = false

function isPageVisible(): boolean {
  if (typeof document === 'undefined') return true
  return document.visibilityState === 'visible'
}

function hasActiveRuns(): boolean {
  return (
    probingNodeIds.value.size > 0 ||
    queuedNodeIds.value.size > 0 ||
    probePendingIds.value.size > 0 ||
    pendingResultRefreshIds.value.size > 0 ||
    probingNodeId.value !== null
  )
}

function handleVisibilityChange() {
  if (isPageVisible() && hasActiveRuns()) {
    syncPoolStatus()
  }
}

onMounted(async () => {
  // Restore initial URL query params if present
  if (route?.query) {
    const initQ = parseQuerySearch(route.query.q)
    const initProto = parseQueryProtocol(route.query.protocol)
    const initHealth = parseQueryHealth(route.query.health)

    if (initQ) searchQuery.value = initQ
    if (initProto !== 'all') protocolFilter.value = initProto
    if (initHealth !== 'all') healthFilter.value = initHealth
    riskBandFilter.value = parseQueryRiskBand(route.query.ip_risk_band)
    riskScoreMax.value = parseQueryRiskScore(route.query.ip_risk_score_max)
    speedMinKiBS.value = parseSpeedMinimum(route.query.speed_min_kib_s)
  }

  void fetchGlobalScheduleKinds()
  await load()
  if (disposed) return
  pruneInvalidSelectedIds()
  checkViewportStarvation()
  if (typeof window !== 'undefined') {
    window.addEventListener('scroll', onScroll, { passive: true })
    nextTick(updateScrollMargin)
  }
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
  poolPollTimer = setInterval(() => {
    if (isPageVisible() && hasActiveRuns()) {
      syncPoolStatus()
    }
  }, 2000)

  const initialNodeId = queryNodeId(route?.query?.node)
  if (initialNodeId) {
    await openNodeByLogicalId(initialNodeId)
  }
})

onUnmounted(() => {
  disposed = true
  invalidateDeepLink()
  sourceHistorySeq++
  if (poolPollTimer) {
    clearInterval(poolPollTimer)
    poolPollTimer = null
  }
  if (typeof window !== 'undefined') {
    window.removeEventListener('scroll', onScroll)
  }
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="nodes-title">
    <!-- Screen-reader accessible section heading -->
    <h2 id="nodes-title" class="sr-only">{{ uiText('节点列表与检测', 'Node management and probing') }}</h2>

    <!-- Modern Compact Toolbar with Filter Pills & Selection Controls -->
    <NodeListToolbar
      :total="total"
      :items-count="items.length"
      :filtered-count="filteredItems.length"
      :initial-loaded="initialLoaded"
      :loading="loading"
      :protocol-filter="protocolFilter"
      :health-filter="healthFilter"
      :risk-band-filter="riskBandFilter"
      :risk-score-max="riskScoreMax"
      :speed-min="speedMinKiBS"
      :applied-search-query="appliedSearchQuery"
      @update:speed-min="handleSpeedMinChange"
      @update:risk-band-filter="handleRiskBandChange"
      @update:risk-score-max="handleRiskScoreChange"
      @reset-filters="resetFilters"
      :search-query="searchQuery"
      :health-counts="healthCounts"
      :is-all-visible-selected="isAllVisibleSelected"
      :selected-count="selectedIds.size"
      :visible-selected-count="visibleSelectedCount"
      @update:protocol-filter="handleProtocolFilterChange"
      @update:health-filter="handleHealthFilterChange"
      @update:search-query="handleSearchQueryChange"
      @refresh="load"
      @search-submit="handleSearchSubmit"
      @toggle-select-all-visible="toggleSelectAllVisible"
      @clear-selection="clearSelection"
      @clear-invisible-selections="clearInvisibleSelections"
      @probe-selected="handleProbeSelectedClick"
      @probe-all="handleProbeAllClick"
      @open-history="historyDrawerOpen = true"
    />

    <!-- Deep Link Error Alert -->
    <div
      v-if="deepLinkError"
      data-testid="node-deep-link-error"
      class="alert alert-warning shadow-sm flex items-center justify-between"
    >
      <div class="flex items-center gap-2">
        <ExclamationTriangleIcon class="w-5 h-5 shrink-0" />
        <span>{{ deepLinkError }}</span>
      </div>
      <div class="flex items-center gap-2">
        <button
          v-if="lastDeepLinkId && !deepLinkError.includes('(404)')"
          type="button"
          data-testid="node-deep-link-retry-btn"
          class="btn btn-warning btn-xs"
          :disabled="loadingDeepLink"
          @click="openNodeByLogicalId(lastDeepLinkId)"
        >
          {{ uiText('重试', 'Retry') }}
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          @click="deepLinkError = ''"
        >
          {{ uiText('关闭', 'Close') }}
        </button>
      </div>
    </div>

    <p v-if="poolError" data-testid="node-pool-status-error" class="text-sm text-warning break-words [overflow-wrap:anywhere]" role="status">{{ poolError }}</p>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error || appendError"
      :error="appendError || error"
      :retrying="loading || loadingMore"
      @retry="appendError ? loadMore() : load()"
    />

    <!-- Skeleton Loading during initial fetch -->
    <div
      v-if="loading && items.length === 0"
      class="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,288px),1fr))]"
    >
      <div v-for="index in 6" :key="index" class="skeleton h-32 rounded-box" />
    </div>

    <!-- Virtualized Responsive Node List: Desktop Compact Row & Mobile Streamlined Card -->
    <div v-else ref="containerRef" class="w-full min-w-0">
      <!-- Desktop Table Header -->
      <div
        v-if="filteredItems.length > 0"
        class="hidden md:flex items-center justify-between gap-3 px-4 py-2 text-xs font-semibold text-base-content/60 border-b border-base-300 mb-2 select-none"
      >
        <div class="w-1/4">{{ uiText('节点信息', 'Node') }}</div>
        <div class="flex-1">{{ uiText('探测健康与能力', 'Health and capabilities') }}</div>
        <div class="shrink-0 text-right">{{ uiText('检测与详情', 'Probe and details') }}</div>
      </div>

      <div
        v-if="filteredItems.length > 0"
        class="relative w-full"
        :style="{ height: `${rowVirtualizer.getTotalSize()}px` }"
      >
        <div
          v-for="virtualRow in rowVirtualizer.getVirtualItems()"
          :key="String(virtualRow.key)"
          :ref="measureRow"
          :data-index="virtualRow.index"
          class="absolute left-0 top-0 w-full"
          :style="{
            transform: `translateY(${virtualRow.start - scrollMargin}px)`,
            paddingBottom: `${GAP}px`,
          }"
        >
          <div class="w-full min-w-0">
            <NodeCompactRow
              v-for="node in getRowItems(virtualRow.index)"
              :key="node.logicalId"
              :node="node"
              :selected="selectedIds.has(node.logicalId)"
              :probe-state="getNodeProbeState(node)"
              :probe-pending="probePendingIds.has(node.logicalId)"
              :probe-run-state="probeStates[node.logicalId]"
              :speed-busy="speedingNodeId === node.logicalId"
              :speed-disabled="Boolean(speedingNodeId)"
              :speed-failed="speedFailedIds.has(node.logicalId)"
              @speed-node="requestSpeed"
              @toggle-select="toggleSelectNode"
              @open-detail="openNodeDetail"
              @probe-node="handleProbeSelectedNode"
            />
          </div>
        </div>
      </div>

      <!-- Terminal content reserves inherited dynamic dock inset for clean separation -->
      <div
        v-if="filteredItems.length > 0"
        class="py-4 text-center"
        :style="{ paddingBottom: 'var(--content-dock-inset, 32px)' }"
      >
        <div v-if="loadingMore || isAutoPaginating" class="flex justify-center py-2">
          <span class="loading loading-spinner loading-sm text-primary" />
        </div>
        <div v-else-if="hasMore" class="py-2">
          <button
            type="button"
            class="btn btn-xs btn-outline btn-primary"
            data-testid="manual-load-more-btn"
            @click="manualLoadMore"
          >
            {{ appendError ? uiText('重试加载下一页', 'Retry next page') : uiText('继续加载更多节点', 'Load more nodes') }}
          </button>
        </div>
        <p v-if="!hasMore" class="text-xs opacity-60">{{ t('nodes.allLoaded') }}</p>
      </div>

      <!-- Empty State & Starvation Guard if filtered count is 0 -->
      <div v-if="!filteredItems.length && !loading && initialLoaded && !error" aria-live="polite">
        <EmptyState
          :icon="EyeIcon"
          :title="activeFilterSummary ? uiText('没有匹配当前条件的节点', 'No nodes match the current filters') : t('nodes.emptyTitle')"
          :description="activeFilterSummary ? uiText('尝试调整条件，或重置筛选查看节点。', 'Adjust the conditions or reset filters to view nodes.') : t('nodes.emptyDesc')"
          :action-label="activeFilterSummary ? uiText('重置筛选', 'Reset filters') : ''"
          @action="resetFilters"
        >
          <p v-if="activeFilterSummary" data-testid="node-empty-filter-summary" class="mt-3 max-w-lg text-xs text-base-content/70 break-words [overflow-wrap:anywhere]">
            {{ activeFilterSummary }}
          </p>
        </EmptyState>
        <div
          v-if="hasMore && !loadingMore && !isAutoPaginating && consecutiveAutoPages >= MAX_AUTO_PAGE_STEPS"
          class="text-center py-3"
          data-testid="starvation-auto-page-guard"
        >
          <button
            type="button"
            class="btn btn-sm btn-outline btn-primary"
            @click="manualLoadMore"
          >
            {{ uiText('已自动检索 3 页无匹配 · 继续加载更多节点', 'No matches in 3 automatically searched pages · Load more nodes') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Node Detail, Edit & Preview Drawer -->
    <NodeDetailDrawer
      ref="nodeDrawer"
      v-model="drawerOpen"
      :node="selectedNode"
      :source-history="sourceHistory"
      :loading-source-history="loadingSourceHistory"
      :source-history-error="sourceHistoryError"
      :probe-state="selectedNode ? getNodeProbeState(selectedNode) : 'idle'"
      :probe-feedback="selectedProbeFeedback"
      :probe-pending="Boolean(selectedNode && probePendingIds.has(selectedNode.logicalId))"
      :speed-busy="speedingNodeId === selectedNode?.logicalId"
      :speed-disabled="Boolean(speedingNodeId)"
      :speed-failed="Boolean(selectedNode && speedFailedIds.has(selectedNode.logicalId))"
      :speed-feedback="speedFeedbackTarget === selectedNode?.logicalId ? speedFeedback : ''"
      @speed-node="requestSpeed"
      :saving-connection="savingConnection"
      :loading-detail="loadingDetail"
      :detail-stale="isDetailStale"
      :detail-error-message="detailErrorMessage"
      @probe-node="handleProbeSelectedNode"
      @retry-source-history="selectedNode ? fetchSourceHistory(selectedNode.logicalId) : undefined"
      @retry-detail="fetchNodeDetail"
      @save-connection="handleSaveConnectionPayload"
    />

    <p v-if="speedFeedback" data-testid="node-speed-feedback" class="text-sm break-words [overflow-wrap:anywhere]" role="status">{{ speedFeedback }}</p>
    <ConfirmModal
      :model-value="Boolean(speedTarget)"
      :title="uiText('确认单节点带宽测速', 'Confirm single-node speed measurement')"
      :message="uiText(`仅对「${speedTarget?.displayName || ''}」测速一次，将产生真实流量（应用响应体上限 5 MiB，测速请求最长 10 秒；不代表网卡计费上限）。结果保存到节点状态，不创建自动测速计划。`, `Measure bandwidth once for “${speedTarget?.displayName || ''}”. This uses real traffic (application response body limit 5 MiB, request timeout 10 seconds; not a network billing limit). The result is saved without scheduling automatic measurements.`)"
      :confirm-text="uiText('开始测速', 'Measure speed')"
      tone="warning"
      data-testid="confirm-node-speed-modal"
      @confirm="confirmSpeed"
      @update:model-value="!$event && (speedTarget = null)"
    />

    <!-- Full Fleet Probe Confirmation Modal -->
    <ConfirmModal
      :model-value="confirmProbeAllOpen"
      :title="uiText('确认检测全部启用订阅节点', 'Probe all enabled-subscription nodes?')"
      :message="uiText('将对全部启用订阅节点发起普通检测，不受当前筛选和分页限制，不包含带宽测速。', 'Probe all nodes from enabled subscriptions, regardless of filters or pagination. Bandwidth is not measured.')"
      tone="warning"
      data-testid="confirm-probe-all-modal"
      @confirm="confirmExecuteProbeAll"
      @update:model-value="confirmProbeAllOpen = $event"
    />

    <!-- Scope Confirmation Modal for Selected Probes when hidden nodes exist -->
    <ConfirmModal
      :model-value="confirmProbeSelectedOpen"
      :title="uiText('确认发起所选普通检测', 'Confirm selected node probes')"
      :message="selectedScopeConfirmMessage"
      tone="warning"
      data-testid="confirm-probe-selected-modal"
      @confirm="executeProbeSelected"
      @update:model-value="confirmProbeSelectedOpen = $event"
    />

    <!-- Probe Runs & Batches History Drawer -->
    <NodeProbeHistoryDrawer
      v-model="historyDrawerOpen"
    />
  </section>
</template>

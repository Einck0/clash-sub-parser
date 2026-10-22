<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type VNodeRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
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
import { useNodes, type HealthFilterType } from './useNodes'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'

const GAP = 8
const ESTIMATED_ROW_HEIGHT = 56

const {
  items,
  loading,
  loadingMore,
  savingConnection,
  probingNodeId,
  probingNodeIds,
  queuedNodeIds,
  error,
  total,
  hasMore,
  initialLoaded,
  protocolFilter,
  healthFilter,
  searchQuery,
  selectedNode,
  load,
  loadMore,
  syncPoolStatus,
  fetchNodeDetail,
  fetchNodeSourceHistory,
  fetchGlobalScheduleKinds,
  probeSingleNode,
  probeNodesBatch,
  updateNodeConnection,
} = useNodes()

const route = (() => {
  try {
    return useRoute()
  } catch {
    return undefined
  }
})()

const router = (() => {
  try {
    return useRouter()
  } catch {
    return undefined
  }
})()

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
    router.replace({ query }).catch(() => {})
  }
}

const drawerOpen = ref(false)
const probeFeedback = ref('')
const sourceHistory = ref<NodeSourceHistoryData | null>(null)
const loadingSourceHistory = ref(false)
const sourceHistoryError = ref('')
const currentHistoryTargetId = ref('')
const deepLinkError = ref('')
const loadingDeepLink = ref(false)

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
      message: t('nodes.noNodeSelected') || '请先勾选需要检测的节点',
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
        `已对所选 ${ids.length} 个节点发起测速`,
        `Started probe for ${ids.length} selected nodes`
      ),
      tone: 'success',
    })
  } else {
    toastStore.push({ message: res.error || '发起测速失败', tone: 'error' })
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
      message: t('nodes.probingStarted') || '已发起全量节点测速任务',
      tone: 'success',
    })
  } else {
    toastStore.push({ message: res.error || '全量测速失败', tone: 'error' })
  }
}

const effectiveProbingIds = computed(() => {
  const set = new Set<string>(probingNodeIds.value)
  if (probingNodeId.value) set.add(probingNodeId.value)
  return set
})

function getNodeProbeState(node: NormalizedNode): 'probing' | 'queued' | 'idle' {
  return resolveNodeProbeState(node, effectiveProbingIds.value, queuedNodeIds.value)
}

const healthCounts = computed(() => {
  let probing = 0
  let healthy = 0
  let unhealthy = 0
  let untested = 0

  for (const node of items.value) {
    const live = getNodeProbeState(node)
    if (live === 'probing' || live === 'queued') {
      probing += 1
    }
    const cat = nodeUnderlyingHealthCategory(node)
    if (cat === 'healthy') healthy += 1
    else if (cat === 'unhealthy') unhealthy += 1
    else untested += 1
  }

  return {
    all: items.value.length,
    probing,
    healthy,
    unhealthy,
    untested,
  }
})

const filteredItems = computed(() => {
  const filter = healthFilter.value
  if (filter === 'all') return items.value
  return items.value.filter((node) => {
    if (filter === 'probing') {
      const live = getNodeProbeState(node)
      return live === 'probing' || live === 'queued'
    }
    const cat = nodeUnderlyingHealthCategory(node)
    if (filter === 'healthy') return cat === 'healthy'
    if (filter === 'unhealthy') return cat === 'unhealthy'
    if (filter === 'untested' || filter === 'unknown' || filter === 'undetermined')
      return cat === 'untested'
    if (filter === 'degraded') return cat === 'healthy'
    return true
  })
})

async function handleProbeSelectedNode(node: NormalizedNode) {
  probeFeedback.value = `已将节点「${node.displayName}」插队至节点池最前面优先检测...`
  const res = await probeSingleNode(node.logicalId)
  if (res.ok) {
    probeFeedback.value = '已插队至节点池最前面优先检测 · 测速已完成并刷新最新状态'
  } else {
    probeFeedback.value = res.error || '测速请求失败'
  }
}

async function fetchSourceHistory(logicalId: string) {
  currentHistoryTargetId.value = logicalId
  sourceHistory.value = null
  sourceHistoryError.value = ''
  loadingSourceHistory.value = true
  try {
    const data = await fetchNodeSourceHistory(logicalId)
    if (currentHistoryTargetId.value === logicalId) {
      sourceHistory.value = data
    }
  } catch (err: any) {
    if (currentHistoryTargetId.value === logicalId) {
      if (err?.status === 404 || (err instanceof ApiError && err.status === 404)) {
        sourceHistoryError.value = '未找到该节点的来源历史记录 (404)'
      } else if (err?.status === 401 || (err instanceof ApiError && err.status === 401)) {
        sourceHistoryError.value = '鉴权失败，无法获取来源历史'
      } else {
        sourceHistoryError.value = err instanceof Error ? err.message : '获取来源历史失败'
      }
    }
  } finally {
    if (currentHistoryTargetId.value === logicalId) {
      loadingSourceHistory.value = false
    }
  }
}

async function openNodeDetail(node: NormalizedNode) {
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
  deepLinkError.value = ''

  const localMatch = items.value.find((n) => n.logicalId === logicalId)
  if (localMatch) {
    await openNodeDetail(localMatch)
    return
  }

  loadingDeepLink.value = true
  try {
    const detailed = await fetchNodeDetail(logicalId)
    if (detailed) {
      await openNodeDetail(detailed)
    } else {
      deepLinkError.value = `未找到节点「${logicalId}」(404)`
    }
  } catch (err: any) {
    const status = err?.status || (err instanceof ApiError ? err.status : 404)
    deepLinkError.value = `未找到节点「${logicalId}」(${status})`
    if (status === 404 && selectedIds.value.has(logicalId)) {
      const next = new Set(selectedIds.value)
      next.delete(logicalId)
      selectedIds.value = next
    }
  } finally {
    loadingDeepLink.value = false
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
    onError(res.error || '连接参数无效')
    return
  }
  if (res.node) {
    onSuccess(res.node)
  }
}

function handleProtocolFilterChange(proto: string) {
  protocolFilter.value = proto
  load()
  syncUrlFromState()
}

function handleHealthFilterChange(health: string) {
  healthFilter.value = health as HealthFilterType
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
  if (loading.value || loadingMore.value || isAutoPaginating.value) return
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
  if (scrollPosition >= threshold) {
    consecutiveAutoPages.value = 0
    loadMore()
  }
}

watch(drawerOpen, (isOpen) => {
  if (!isOpen) {
    syncUrlFromState()
    sourceHistory.value = null
    sourceHistoryError.value = ''
    loadingSourceHistory.value = false
  }
})

if (route) {
  watch(
    () => route.query?.node as string | undefined,
    async (newNodeId) => {
      const targetId = newNodeId?.trim()
      if (!targetId) {
        if (drawerOpen.value) {
          drawerOpen.value = false
        }
        return
      }
      if (selectedNode.value?.logicalId === targetId && drawerOpen.value) {
        return
      }
      await openNodeByLogicalId(targetId)
    }
  )

  watch(
    () => [route.query?.q, route.query?.protocol, route.query?.health],
    ([newQ, newProto, newHealth]) => {
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
      }
      if (needReload) {
        load()
      }
    }
  )
}

let poolPollTimer: ReturnType<typeof setInterval> | null = null

function isPageVisible(): boolean {
  if (typeof document === 'undefined') return true
  return document.visibilityState === 'visible'
}

function hasActiveRuns(): boolean {
  return (
    probingNodeIds.value.size > 0 ||
    queuedNodeIds.value.size > 0 ||
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
  }

  void fetchGlobalScheduleKinds()
  await load()
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

  const initialNodeId = (route?.query?.node as string | undefined)?.trim()
  if (initialNodeId) {
    await openNodeByLogicalId(initialNodeId)
  }
})

onUnmounted(() => {
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
    <h2 id="nodes-title" class="sr-only">{{ uiText('节点列表与测速', 'Node Management & Probing') }}</h2>

    <!-- Modern Compact Toolbar with Filter Pills & Selection Controls -->
    <NodeListToolbar
      :total="total"
      :items-count="items.length"
      :filtered-count="filteredItems.length"
      :initial-loaded="initialLoaded"
      :loading="loading"
      :protocol-filter="protocolFilter"
      :health-filter="healthFilter"
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
      <button
        type="button"
        class="btn btn-ghost btn-xs"
        @click="deepLinkError = ''"
      >
        关闭
      </button>
    </div>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loading"
      @retry="load"
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
        <div class="w-1/4 flex items-center gap-2">
          <span>节点信息</span>
        </div>
        <div class="w-1/5">
          <span>协议与端点</span>
        </div>
        <div class="flex-1">
          <span>探测健康与能力</span>
        </div>
        <div class="shrink-0 text-right pr-2">
          <span>状态与操作</span>
        </div>
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
        <div v-else-if="hasMore && consecutiveAutoPages >= MAX_AUTO_PAGE_STEPS" class="py-2">
          <button
            type="button"
            class="btn btn-xs btn-outline btn-primary"
            data-testid="manual-load-more-btn"
            @click="manualLoadMore"
          >
            继续加载更多节点
          </button>
        </div>
        <p v-if="!hasMore" class="text-xs opacity-60">{{ t('nodes.allLoaded') }}</p>
      </div>

      <!-- Empty State & Starvation Guard if filtered count is 0 -->
      <div v-if="!filteredItems.length && !loading">
        <EmptyState
          :icon="EyeIcon"
          :title="items.length === 0 ? t('nodes.emptyTitle') : '没有匹配筛选状态的节点'"
          :description="items.length === 0 ? t('nodes.emptyDesc') : '请尝试切换节点状态筛选或协议筛选条件。'"
        />
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
            已自动检索 3 页无匹配 · 继续加载更多节点
          </button>
        </div>
      </div>
    </div>

    <!-- Node Detail, Edit & Preview Drawer -->
    <NodeDetailDrawer
      v-model="drawerOpen"
      :node="selectedNode"
      :source-history="sourceHistory"
      :loading-source-history="loadingSourceHistory"
      :source-history-error="sourceHistoryError"
      :probe-state="selectedNode ? getNodeProbeState(selectedNode) : 'idle'"
      :probe-feedback="probeFeedback"
      :saving-connection="savingConnection"
      @probe-node="handleProbeSelectedNode"
      @retry-source-history="selectedNode ? fetchSourceHistory(selectedNode.logicalId) : undefined"
      @save-connection="handleSaveConnectionPayload"
    />

    <!-- Full Fleet Probe Confirmation Modal -->
    <ConfirmModal
      :model-value="confirmProbeAllOpen"
      :title="t('nodes.confirmProbeAllTitle')"
      :message="t('nodes.confirmProbeAllMessage')"
      tone="warning"
      data-testid="confirm-probe-all-modal"
      @confirm="confirmExecuteProbeAll"
      @cancel="confirmProbeAllOpen = false"
      @update:model-value="confirmProbeAllOpen = $event"
    />

    <!-- Scope Confirmation Modal for Selected Probes when hidden nodes exist -->
    <ConfirmModal
      :model-value="confirmProbeSelectedOpen"
      :title="uiText('确认发起所选测速', 'Confirm Selected Probe')"
      :message="uiText(
        `当前已选 ${selectedIds.size} 个节点（其中当前可见 ${visibleSelectedCount} 项，其他页面/筛选隐藏 ${selectedIds.size - visibleSelectedCount} 项）。确认对全部 ${selectedIds.size} 个节点发起测速？`,
        `Currently selected ${selectedIds.size} nodes (${visibleSelectedCount} visible, ${selectedIds.size - visibleSelectedCount} hidden). Probe all ${selectedIds.size} nodes?`
      )"
      tone="warning"
      data-testid="confirm-probe-selected-modal"
      @confirm="executeProbeSelected"
      @cancel="confirmProbeSelectedOpen = false"
      @update:model-value="confirmProbeSelectedOpen = $event"
    />

    <!-- Probe Runs & Batches History Drawer -->
    <NodeProbeHistoryDrawer
      v-model="historyDrawerOpen"
    />
  </section>
</template>

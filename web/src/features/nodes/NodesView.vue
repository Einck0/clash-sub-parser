<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type VNodeRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowPathIcon,
  BoltIcon,
  CheckCircleIcon,
  ClockIcon,
  ExclamationTriangleIcon,
  EyeIcon,
  ShieldCheckIcon,
} from '@heroicons/vue/24/outline'
import { useWindowVirtualizer } from '@tanstack/vue-virtual'
import { useElementSize, useResizeObserver } from '@vueuse/core'
import DrawerCard from '../../ui/DrawerCard.vue'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import NodeSourceHistoryPanel from './NodeSourceHistoryPanel.vue'
import type { NodeSourceHistoryData } from './sourceHistoryTypes'
import { ApiError } from '../../api/client'
import {
  SUPPORTED_NODE_PROTOCOLS,
  formatNodeLatency,
  nodeCapabilityLabel,
  nodeHealthBadge,
  nodeHealthDiagnostic,
  nodeLatencyTone,
  nodeRiskBadge,
  nodeUnderlyingHealthCategory,
  protocolSupportedTargets,
  renderNodePreview,
  resolveNodeLatencyMs,
  resolveNodeProbeState,
  getNodePlatformBadges,
  getNodeSpeedBadge,
  type NormalizedNode,
} from './nodeView'
import { formatRelativeTime } from '../probes/probeTypes'
import { useNodes } from './useNodes'
import { deriveColumns } from '../../composables/useResponsiveColumns'
import { t } from '../../locales'

const GAP = 16
const MIN_CARD_WIDTH = 288
const ESTIMATED_ROW_HEIGHT = 172

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
  protocolFilter,
  healthFilter,
  searchQuery,
  selectedNode,
  load,
  loadMore,
  syncPoolStatus,
  fetchNodeDetail,
  fetchNodeSourceHistory,
  probeSingleNode,
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

const drawerOpen = ref(false)
const previewTarget = ref<'mihomo' | 'singbox'>('mihomo')
const connectionError = ref('')
const connectionSaved = ref(false)
const probeFeedback = ref('')
const sourceHistory = ref<NodeSourceHistoryData | null>(null)
const loadingSourceHistory = ref(false)
const sourceHistoryError = ref('')
const currentHistoryTargetId = ref('')
const deepLinkError = ref('')
const loadingDeepLink = ref(false)

const effectiveProbingIds = computed(() => {
  const set = new Set<string>(probingNodeIds.value)
  if (probingNodeId.value) set.add(probingNodeId.value)
  return set
})

function getNodeProbeState(node: NormalizedNode): 'probing' | 'queued' | 'idle' {
  return resolveNodeProbeState(node, effectiveProbingIds.value, queuedNodeIds.value)
}

function getNodeHealthBadge(node: NormalizedNode) {
  return nodeHealthBadge(node, effectiveProbingIds.value, queuedNodeIds.value)
}

const healthCounts = computed(() => {
  let probing = 0
  let healthy = 0
  let degraded = 0
  let unhealthy = 0
  let unknown = 0

  for (const node of items.value) {
    const live = getNodeProbeState(node)
    if (live === 'probing' || live === 'queued') {
      probing += 1
    }
    const cat = nodeUnderlyingHealthCategory(node)
    if (cat === 'healthy') healthy += 1
    else if (cat === 'degraded') degraded += 1
    else if (cat === 'unhealthy') unhealthy += 1
    else unknown += 1
  }

  return {
    all: items.value.length,
    probing,
    healthy,
    degraded,
    unhealthy,
    unknown,
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
    if (filter === 'degraded') return cat === 'degraded'
    if (filter === 'unhealthy') return cat === 'unhealthy'
    if (filter === 'unknown') return cat === 'unprobed'
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

// Editable plaintext connection draft fields inside the Node Detail Drawer
const draftDisplayName = ref('')
const draftServer = ref('')
const draftPort = ref<number>(0)
// WireGuard fields
const draftWgLocalAddress = ref('')
const draftWgPublicKey = ref('')
const draftWgPrivateKey = ref('')
const draftWgPreSharedKey = ref('')
const draftWgMtu = ref<number | undefined>(undefined)
const draftWgDns = ref('')
const draftWgReserved = ref('')
// TUIC / Generic protocol fields
const draftUuid = ref('')
const draftPassword = ref('')
const draftMethod = ref('')
const draftCongestionControl = ref('')
const draftUdpRelayMode = ref('')
const draftAlpn = ref('')
const draftSni = ref('')
const draftDisableSni = ref(false)

function syncDraftFromNode(node: NormalizedNode) {
  connectionError.value = ''
  connectionSaved.value = false
  draftDisplayName.value = node.displayName
  draftServer.value = node.connection.server
  draftPort.value = node.connection.port
  draftWgLocalAddress.value = node.connection.localAddress.join(', ')
  draftWgPublicKey.value = node.connection.publicKey
  draftWgPrivateKey.value = node.connection.privateKey
  draftWgPreSharedKey.value = node.connection.preSharedKey
  draftWgMtu.value = node.connection.mtu
  draftWgDns.value = node.connection.dns.join(', ')
  draftWgReserved.value = node.connection.reserved.join(', ')
  draftUuid.value = node.connection.uuid
  draftPassword.value = node.connection.password
  draftMethod.value = node.connection.method || ''
  draftCongestionControl.value = node.connection.congestionControl
  draftUdpRelayMode.value = node.connection.udpRelayMode
  draftAlpn.value = node.connection.alpn.join(', ')
  draftSni.value = node.connection.sni
  draftDisableSni.value = node.connection.disableSni
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
  syncDraftFromNode(node)
  drawerOpen.value = true

  if (router && route && route.query?.node !== node.logicalId) {
    router.replace({ query: { ...route.query, node: node.logicalId } }).catch(() => {})
  }

  await Promise.all([
    (async () => {
      const detailed = await fetchNodeDetail(node.logicalId)
      if (detailed && selectedNode.value?.logicalId === node.logicalId) {
        syncDraftFromNode(detailed)
      }
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
  } finally {
    loadingDeepLink.value = false
  }
}

async function handleApplyConnectionUpdate() {
  if (!selectedNode.value) return
  connectionError.value = ''
  connectionSaved.value = false

  const localAddress = draftWgLocalAddress.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  const dns = draftWgDns.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
  const reserved = draftWgReserved.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => !Number.isNaN(n))
  const alpn = draftAlpn.value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)

  const res = await updateNodeConnection(selectedNode.value.logicalId, {
    displayName: draftDisplayName.value,
    server: draftServer.value,
    port: Number(draftPort.value),
    localAddress,
    publicKey: draftWgPublicKey.value,
    privateKey: draftWgPrivateKey.value,
    preSharedKey: draftWgPreSharedKey.value,
    mtu: draftWgMtu.value ? Number(draftWgMtu.value) : undefined,
    dns,
    reserved,
    uuid: draftUuid.value,
    password: draftPassword.value,
    method: draftMethod.value,
    congestionControl: draftCongestionControl.value,
    udpRelayMode: draftUdpRelayMode.value,
    alpn,
    sni: draftSni.value,
    disableSni: draftDisableSni.value,
  })

  if (!res.ok) {
    connectionError.value = res.error || '连接参数无效'
    return
  }
  if (res.node) {
    syncDraftFromNode(res.node)
  }
  connectionSaved.value = true
}

const previewNodeSnippet = computed(() => {
  if (!selectedNode.value) return ''
  return renderNodePreview(selectedNode.value, previewTarget.value)
})

function selectProtocolFilter(proto: string) {
  protocolFilter.value = proto
  load()
}

const containerRef = ref<HTMLElement | null>(null)
const { width } = useElementSize(containerRef)

const columns = computed(() => deriveColumns(width.value, MIN_CARD_WIDTH, GAP))
const rowCount = computed(() => Math.ceil(filteredItems.value.length / columns.value))

function getRowItems(rowIndex: number) {
  const start = rowIndex * columns.value
  return filteredItems.value.slice(start, start + columns.value)
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
  })),
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

watch(columns, () => {
  nextTick(() => {
    rowVirtualizer.value.measure()
  })
})

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
    if (router && route && route.query?.node) {
      const query = { ...route.query }
      delete query.node
      router.replace({ query }).catch(() => {})
    }
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
}

let poolPollTimer: ReturnType<typeof setInterval> | null = null

onMounted(async () => {
  await load()
  checkViewportStarvation()
  if (typeof window !== 'undefined') {
    window.addEventListener('scroll', onScroll, { passive: true })
    nextTick(updateScrollMargin)
  }
  poolPollTimer = setInterval(() => {
    if (
      probingNodeIds.value.size > 0 ||
      queuedNodeIds.value.size > 0 ||
      healthFilter.value === 'probing'
    ) {
      syncPoolStatus()
    }
  }, 2000)

  const initialNodeId = (route?.query?.node as string | undefined)?.trim()
  if (initialNodeId) {
    await openNodeByLogicalId(initialNodeId)
  }
})

onUnmounted(() => {
  if (poolPollTimer) clearInterval(poolPollTimer)
  if (typeof window !== 'undefined') {
    window.removeEventListener('scroll', onScroll)
  }
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="nodes-title">
    <!-- Header -->
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('nodes.tag') }}</p>
        <h2 id="nodes-title" class="mt-1 text-2xl font-bold">{{ t('nodes.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">{{ t('nodes.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <span class="badge badge-primary badge-outline text-xs" data-testid="nodes-scope-badge">
          {{ t('nodes.scopeEnabledBadge') }}
        </span>
        <span class="badge badge-ghost text-xs" data-testid="nodes-total-badge">
          {{ healthFilter === 'all' ? `${total.toLocaleString()} ${t('nodes.totalNodes')}` : `${filteredItems.length} / ${total.toLocaleString()} ${t('nodes.totalNodes')}` }}
        </span>
        <button class="btn btn-ghost btn-sm btn-square touch-manipulation" type="button" :title="t('common.refresh')" @click="load">
          <ArrowPathIcon class="h-4 w-4" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <!-- Protocol Filter Pills, Health Status Filter & Search Bar -->
    <div class="flex flex-col gap-3">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div
          data-testid="node-protocol-filter"
          class="flex flex-wrap items-center gap-1.5 text-xs"
          role="group"
          aria-label="协议筛选"
        >
          <button
            type="button"
            class="btn btn-xs rounded-lg font-mono"
            :class="protocolFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
            @click="selectProtocolFilter('all')"
          >
            {{ t('nodes.protocolAll') }}
          </button>
          <button
            v-for="proto in SUPPORTED_NODE_PROTOCOLS"
            :key="proto"
            type="button"
            class="btn btn-xs rounded-lg font-mono uppercase"
            :class="protocolFilter === proto ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
            @click="selectProtocolFilter(proto)"
          >
            {{ proto }}
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <select
            v-model="healthFilter"
            data-testid="node-health-filter"
            aria-label="按节点状态筛选"
            class="select select-bordered select-xs sm:select-sm text-xs"
          >
            <option value="all">全部 ({{ healthCounts.all }})</option>
            <option value="probing">检测中 ({{ healthCounts.probing }})</option>
            <option value="healthy">正常 ({{ healthCounts.healthy }})</option>
            <option value="degraded">降级 ({{ healthCounts.degraded }})</option>
            <option value="unhealthy">异常 ({{ healthCounts.unhealthy }})</option>
            <option value="unknown">未探测 / 待核验 ({{ healthCounts.unknown }})</option>
          </select>

          <input
            v-model="searchQuery"
            data-testid="node-search-input"
            type="search"
            :placeholder="t('nodes.searchPlaceholder')"
            class="input input-bordered input-xs sm:input-sm w-full sm:w-60 font-mono text-xs"
            @keydown.enter="load"
          />
        </div>
      </div>

      <!-- Health Status Quick Filter Capsules -->
      <div
        data-testid="node-health-pills"
        class="flex flex-wrap items-center gap-1.5 text-xs"
        role="group"
        aria-label="节点状态筛选"
      >
        <span class="opacity-60 mr-1 font-medium">节点状态：</span>
        <button
          type="button"
          data-testid="node-health-pill-all"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-200/70'"
          @click="healthFilter = 'all'"
        >
          全部 ({{ healthCounts.all }})
        </button>
        <button
          type="button"
          data-testid="node-health-pill-probing"
          class="btn btn-xs rounded-full gap-1"
          :class="healthFilter === 'probing' ? 'btn-info' : 'btn-ghost bg-base-200/70'"
          @click="healthFilter = 'probing'"
        >
          <span
            class="w-1.5 h-1.5 rounded-full bg-current"
            :class="{ 'animate-ping': healthCounts.probing > 0 }"
          />
          <span>检测中 ({{ healthCounts.probing }})</span>
        </button>
        <button
          type="button"
          data-testid="node-health-pill-healthy"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'healthy' ? 'btn-success' : 'btn-ghost bg-base-200/70'"
          @click="healthFilter = 'healthy'"
        >
          正常 ({{ healthCounts.healthy }})
        </button>
        <button
          type="button"
          data-testid="node-health-pill-degraded"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'degraded' ? 'btn-warning' : 'btn-ghost bg-base-200/70'"
          @click="healthFilter = 'degraded'"
        >
          降级 ({{ healthCounts.degraded }})
        </button>
        <button
          type="button"
          data-testid="node-health-pill-unhealthy"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'unhealthy' ? 'btn-error' : 'btn-ghost bg-base-200/70'"
          @click="healthFilter = 'unhealthy'"
        >
          异常 ({{ healthCounts.unhealthy }})
        </button>
        <button
          type="button"
          data-testid="node-health-pill-unknown"
          class="btn btn-xs rounded-full"
          :class="healthFilter === 'unknown' ? 'btn-neutral' : 'btn-ghost bg-base-200/70'"
          @click="healthFilter = 'unknown'"
        >
          未探测 / 待核验 ({{ healthCounts.unknown }})
        </button>
      </div>
    </div>

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

    <!-- Skeleton Loading with identical responsive min-card width -->
    <div
      v-if="loading && items.length === 0"
      class="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,288px),1fr))]"
    >
      <div v-for="index in 6" :key="index" class="skeleton h-32 rounded-box" />
    </div>

    <!-- Virtualized Responsive Node Grid Normal page flow with width-derived columns -->
    <div v-else ref="containerRef" class="w-full min-w-0">
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
          <div
            class="grid min-w-0 max-w-full"
            :style="{
              gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
              gap: `${GAP}px`,
            }"
          >
            <article
              v-for="node in getRowItems(virtualRow.index)"
              :key="node.logicalId"
              data-testid="node-card"
              class="card border border-base-300 bg-base-200 shadow-sm transition hover:border-primary/40 hover:shadow-md min-w-0 max-w-full overflow-hidden cursor-pointer"
              @click="openNodeDetail(node)"
            >
              <div class="card-body gap-3 p-4 sm:p-5 min-w-0 max-w-full overflow-hidden">
                <div class="flex flex-wrap items-start justify-between gap-2 sm:gap-3 min-w-0">
                  <div class="flex min-w-0 items-center gap-2.5 sm:gap-3 flex-1">
                    <div class="rounded-xl bg-primary/10 p-2 text-primary shrink-0">
                      <CheckCircleIcon v-if="node.active" class="h-5 w-5" />
                      <ExclamationTriangleIcon v-else class="h-5 w-5" />
                    </div>
                    <div class="min-w-0 flex-1">
                      <h3 class="truncate font-semibold text-sm sm:text-base">{{ node.displayName }}</h3>
                      <p class="mt-0.5 truncate font-mono text-xs opacity-60">{{ node.logicalId }}</p>
                    </div>
                  </div>
                  <div class="flex items-center gap-1.5 sm:gap-2 shrink-0">
                    <span class="badge badge-outline font-mono text-xs uppercase">{{ node.protocol }}</span>
                    <StatusBadge
                      :label="node.active ? t('common.enabled') : t('common.disabled')"
                      :tone="node.active ? 'success' : 'warning'"
                    />
                  </div>
                </div>

                <!-- Protocol Connection Summary & Target Boundary Badges -->
                <div class="flex flex-wrap items-center gap-1.5 text-[11px] font-mono opacity-80">
                  <span class="badge badge-xs badge-ghost">
                    兼容目标: {{ protocolSupportedTargets(node.protocol).join('/') }}
                  </span>
                  <span v-if="node.connection.server && node.connection.port" class="badge badge-xs badge-ghost">
                    {{ node.connection.server }}:{{ node.connection.port }}
                  </span>
                  <template v-if="node.protocol.toLowerCase() === 'wireguard'">
                    <span v-if="node.connection.localAddress.length" class="badge badge-xs badge-info badge-outline">
                      内网 IP: {{ node.connection.localAddress.join(', ') }}
                    </span>
                    <span v-if="node.connection.mtu" class="badge badge-xs badge-ghost">
                      MTU: {{ node.connection.mtu }}
                    </span>
                  </template>
                  <template v-else-if="node.protocol.toLowerCase() === 'tuic'">
                    <span v-if="node.connection.congestionControl" class="badge badge-xs badge-info badge-outline">
                      拥塞控制: {{ node.connection.congestionControl }}
                    </span>
                    <span v-if="node.connection.udpRelayMode" class="badge badge-xs badge-ghost">
                      UDP 模式: {{ node.connection.udpRelayMode }}
                    </span>
                  </template>
                </div>

                <div class="flex flex-wrap items-center justify-between gap-2 border-t border-base-300 pt-3 text-xs min-w-0">
                  <div class="flex flex-wrap items-center gap-1.5 sm:gap-2 min-w-0">
                    <span class="mr-1 text-xs opacity-60 shrink-0">{{ t('nodes.capabilities') }}</span>
                    <StatusBadge
                      data-testid="node-health-badge"
                      :label="
                        getNodeProbeState(node) === 'probing'
                          ? '⚡ 检测中'
                          : getNodeProbeState(node) === 'queued'
                          ? '⏳ 队列等待'
                          : `健康: ${getNodeHealthBadge(node).label}`
                      "
                      :tone="getNodeHealthBadge(node).tone"
                      :pulse="getNodeProbeState(node) !== 'idle'"
                    />
                    <StatusBadge
                      v-if="resolveNodeLatencyMs(node) !== null"
                      data-testid="node-latency-badge"
                      :label="`延迟: ${formatNodeLatency(node)}`"
                      :tone="nodeLatencyTone(node)"
                    />
                    <StatusBadge
                      :label="`风险: ${nodeRiskBadge(node).label}`"
                      :tone="nodeRiskBadge(node).tone"
                    />
                    <StatusBadge
                      v-if="getNodeSpeedBadge(node)"
                      :label="getNodeSpeedBadge(node)!.label"
                      :tone="getNodeSpeedBadge(node)!.tone"
                    />
                    <!-- Streaming: individual platform badges if available, fallback to single badge -->
                    <template v-if="getNodePlatformBadges(node, 'streaming').length > 0">
                      <StatusBadge
                        v-for="badge in getNodePlatformBadges(node, 'streaming')"
                        :key="`streaming-${badge.platform}`"
                        :label="badge.label"
                        :tone="badge.tone"
                        :title="badge.tooltip"
                      />
                    </template>
                    <StatusBadge
                      v-else
                      :label="`${t('nodes.streaming')}: ${nodeCapabilityLabel(node, 'streaming').label}`"
                      :tone="nodeCapabilityLabel(node, 'streaming').tone"
                    />
                    <!-- AI: individual platform badges if available, fallback to single badge -->
                    <template v-if="getNodePlatformBadges(node, 'ai').length > 0">
                      <StatusBadge
                        v-for="badge in getNodePlatformBadges(node, 'ai')"
                        :key="`ai-${badge.platform}`"
                        :label="badge.label"
                        :tone="badge.tone"
                        :title="badge.tooltip"
                      />
                    </template>
                    <StatusBadge
                      v-else
                      :label="`${t('nodes.ai')}: ${nodeCapabilityLabel(node, 'ai').label}`"
                      :tone="nodeCapabilityLabel(node, 'ai').tone"
                    />
                    <span
                      v-if="node.probeStale"
                      class="badge badge-warning badge-sm gap-1 font-mono text-[11px]"
                      title="探测观测数据已过期（超出保鲜窗口）"
                    >
                      探测已过期
                    </span>
                    <span
                      v-if="node.probeMissing"
                      class="badge badge-ghost badge-sm gap-1 font-mono text-[11px] opacity-75"
                      title="尚无探针观测记录"
                    >
                      未探测
                    </span>
                    <span
                      v-if="getNodeProbeState(node) === 'idle' && nodeHealthDiagnostic(node) && nodeHealthDiagnostic(node)?.code !== 'probe_missing'"
                      data-testid="node-card-diagnostic-badge"
                      class="badge badge-sm gap-1 text-[11px] h-auto py-0.5 whitespace-normal"
                      :class="nodeHealthDiagnostic(node)?.isBlockedByPolicyOrConfig ? 'badge-warning badge-outline' : 'badge-ghost opacity-80'"
                      :title="nodeHealthDiagnostic(node)?.detail"
                    >
                      {{ nodeHealthDiagnostic(node)?.shortLabel }}
                    </span>
                  </div>

                  <div class="flex items-center gap-1.5 shrink-0">
                    <button
                      type="button"
                      data-testid="node-card-probe-btn"
                      class="btn btn-outline btn-primary btn-xs gap-1"
                      :disabled="getNodeProbeState(node) === 'probing'"
                      @click.stop="handleProbeSelectedNode(node)"
                    >
                      <BoltIcon
                        class="w-3.5 h-3.5"
                        :class="{ 'animate-spin': getNodeProbeState(node) === 'probing' }"
                      />
                      <span>{{ getNodeProbeState(node) === 'probing' ? '检测中...' : '立即重测' }}</span>
                    </button>
                    <button
                      type="button"
                      data-testid="node-inspect-btn"
                      class="btn btn-ghost btn-xs font-mono shrink-0"
                      @click.stop="openNodeDetail(node)"
                    >
                      {{ t('nodes.inspectNode') }}
                    </button>
                  </div>
                </div>
              </div>
            </article>
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
    <DrawerCard
      v-model="drawerOpen"
      :title="selectedNode ? `${selectedNode.displayName} (${selectedNode.protocol.toUpperCase()})` : t('nodes.inspectNode')"
      :description="t('nodes.connectionProfile')"
    >
      <div v-if="selectedNode" data-testid="node-detail-drawer" class="space-y-4 text-xs">
        <!-- Real-Time Probe Telemetry & Single-Node Quick Probe Bar -->
        <div
          data-testid="node-drawer-probe-panel"
          class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-2.5"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex items-center gap-1.5 font-semibold">
              <BoltIcon class="w-4 h-4 text-primary shrink-0" />
              <span>实时测速与解锁状态</span>
              <span v-if="selectedNode.lastProbedAt" class="text-[11px] font-normal opacity-65">
                · 最后测速: {{ formatRelativeTime(selectedNode.lastProbedAt) }}
              </span>
            </div>
            <button
              type="button"
              data-testid="node-probe-btn"
              class="btn btn-primary btn-xs gap-1"
              :disabled="getNodeProbeState(selectedNode) === 'probing'"
              @click="handleProbeSelectedNode(selectedNode)"
            >
              <BoltIcon
                class="w-3.5 h-3.5"
                :class="{ 'animate-spin': getNodeProbeState(selectedNode) === 'probing' }"
              />
              <span>{{ getNodeProbeState(selectedNode) === 'probing' ? '检测中...' : '立即重测 (插队)' }}</span>
            </button>
          </div>
          <div class="flex flex-wrap items-center gap-1.5">
            <StatusBadge
              data-testid="node-drawer-health-badge"
              :label="
                getNodeProbeState(selectedNode) === 'probing'
                  ? '⚡ 检测中'
                  : getNodeProbeState(selectedNode) === 'queued'
                  ? '⏳ 队列等待'
                  : `健康: ${getNodeHealthBadge(selectedNode).label}`
              "
              :tone="getNodeHealthBadge(selectedNode).tone"
              :pulse="getNodeProbeState(selectedNode) !== 'idle'"
            />
            <StatusBadge
              data-testid="node-drawer-latency-badge"
              :label="`响应延迟: ${formatNodeLatency(selectedNode)}`"
              :tone="nodeLatencyTone(selectedNode)"
            />
            <StatusBadge
              v-if="getNodeSpeedBadge(selectedNode)"
              :label="getNodeSpeedBadge(selectedNode)!.label"
              :tone="getNodeSpeedBadge(selectedNode)!.tone"
            />
            <template v-if="getNodePlatformBadges(selectedNode, 'streaming').length > 0">
              <StatusBadge
                v-for="badge in getNodePlatformBadges(selectedNode, 'streaming')"
                :key="`drawer-streaming-${badge.platform}`"
                :label="badge.label"
                :tone="badge.tone"
                :title="badge.tooltip"
              />
            </template>
            <StatusBadge
              v-else
              :label="`流媒体: ${nodeCapabilityLabel(selectedNode, 'streaming').label}`"
              :tone="nodeCapabilityLabel(selectedNode, 'streaming').tone"
            />
            <template v-if="getNodePlatformBadges(selectedNode, 'ai').length > 0">
              <StatusBadge
                v-for="badge in getNodePlatformBadges(selectedNode, 'ai')"
                :key="`drawer-ai-${badge.platform}`"
                :label="badge.label"
                :tone="badge.tone"
                :title="badge.tooltip"
              />
            </template>
            <StatusBadge
              v-else
              :label="`AI 解锁: ${nodeCapabilityLabel(selectedNode, 'ai').label}`"
              :tone="nodeCapabilityLabel(selectedNode, 'ai').tone"
            />
            <StatusBadge
              :label="`IP 风险: ${nodeRiskBadge(selectedNode).label}`"
              :tone="nodeRiskBadge(selectedNode).tone"
            />
          </div>
          <div
            v-if="getNodeProbeState(selectedNode) === 'idle' && nodeHealthDiagnostic(selectedNode)"
            data-testid="node-drawer-diagnostic-notice"
            class="rounded-lg px-2.5 py-2 text-[11px] leading-relaxed border"
            :class="
              nodeHealthDiagnostic(selectedNode)?.isBlockedByPolicyOrConfig
                ? 'bg-warning/10 border-warning/30 text-warning'
                : 'bg-base-300/50 border-base-300 text-base-content/80'
            "
          >
            <span class="font-semibold">{{ nodeHealthDiagnostic(selectedNode)?.shortLabel }}：</span>
            <span>{{ nodeHealthDiagnostic(selectedNode)?.detail }}</span>
          </div>
          <p v-if="probeFeedback" data-testid="node-probe-feedback" class="text-[11px] text-primary font-medium">
            {{ probeFeedback }}
          </p>
        </div>

        <!-- Platform Capabilities Breakdown (subcheck matrix) -->
        <div
          v-if="
            getNodePlatformBadges(selectedNode, 'streaming').length > 0 ||
            getNodePlatformBadges(selectedNode, 'ai').length > 0 ||
            getNodeSpeedBadge(selectedNode)
          "
          class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-2"
        >
          <div class="flex items-center justify-between">
            <span class="font-semibold text-xs text-primary">{{ t('nodes.platformMatrix') }}</span>
            <span v-if="getNodeSpeedBadge(selectedNode)" class="font-mono text-[11px] opacity-75">
              {{ getNodeSpeedBadge(selectedNode)!.label }}
            </span>
          </div>
          <div class="grid grid-cols-2 gap-2 text-[11px]">
            <div
              v-for="badge in [
                ...getNodePlatformBadges(selectedNode, 'streaming'),
                ...getNodePlatformBadges(selectedNode, 'ai'),
              ]"
              :key="`matrix-${badge.platform}`"
              class="flex items-center justify-between p-2 rounded-lg bg-base-100 border border-base-300"
            >
              <div class="min-w-0 pr-1">
                <span class="font-medium block truncate">{{ badge.platform.toUpperCase() }}</span>
                <span v-if="badge.subTier" class="text-[10px] opacity-60 block truncate">{{ badge.subTier }}</span>
              </div>
              <StatusBadge
                :label="badge.label"
                :tone="badge.tone"
                :title="badge.tooltip"
              />
            </div>
          </div>
        </div>

        <!-- Subscription Provenance & Reconcile Notice -->
        <div
          data-testid="node-reconcile-overwrite-notice"
          class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-1"
        >
          <p class="font-semibold flex items-center justify-between gap-1.5">
            <span class="flex items-center gap-1.5">
              <ArrowPathIcon class="w-4 h-4 shrink-0 text-primary" />
              <span>订阅溯源与自动同步 (Reconcile)</span>
            </span>
            <span class="font-mono text-[11px] opacity-75">逻辑 ID: {{ selectedNode.logicalId }}</span>
          </p>
          <p class="opacity-80 leading-relaxed">
            直接编辑明文连接参数将原地更新当前节点；后续上游订阅源触发自动同步 (Reconcile) 时将从订阅源刷新节点配置。
          </p>
          <p
            v-if="selectedNode.sources && selectedNode.sources.length > 0"
            data-testid="node-provenance-sources"
            class="font-mono text-[11px] opacity-80"
          >
            来源订阅: {{ selectedNode.sources.map((s) => s.subscription_id).join(', ') }}
          </p>
        </div>

        <!-- Dedicated Source Attribution & Provenance Panel -->
        <NodeSourceHistoryPanel
          :logical-id="selectedNode.logicalId"
          :source-history="sourceHistory"
          :loading="loadingSourceHistory"
          :error="sourceHistoryError"
          @retry="fetchSourceHistory(selectedNode.logicalId)"
        />

        <!-- Compiler Target Compatibility for this Node Protocol -->
        <div
          data-testid="node-target-compatibility"
          class="p-3 rounded-xl bg-base-200 border border-base-300 space-y-1.5"
        >
          <div class="flex items-center gap-1.5 font-semibold">
            <ShieldCheckIcon class="w-4 h-4 text-primary" />
            <span>{{ t('nodes.targetCompatibility') }} ({{ selectedNode.protocol.toUpperCase() }})</span>
          </div>
          <div class="flex flex-wrap items-center gap-1.5 font-mono">
            <span
              v-for="target in protocolSupportedTargets(selectedNode.protocol)"
              :key="target"
              class="badge badge-xs badge-success badge-outline uppercase"
            >
              {{ target }}: 已支持
            </span>
            <span
              v-if="selectedNode.protocol.toLowerCase() === 'vless'"
              class="badge badge-xs badge-warning badge-outline"
            >
              surge / qx: 不支持该协议
            </span>
            <span
              v-else-if="['wireguard', 'tuic', 'hysteria2'].includes(selectedNode.protocol.toLowerCase())"
              class="badge badge-xs badge-warning badge-outline"
            >
              qx: 不支持该协议
            </span>
          </div>
        </div>

        <!-- Connection Parameters Form (Direct Plaintext Edit & Inspect) -->
        <form
          data-testid="node-connection-form"
          class="p-3.5 rounded-xl bg-base-200/70 border border-base-300 space-y-3"
          @submit.prevent="handleApplyConnectionUpdate"
        >
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">节点显示名称</span>
              <input
                v-model="draftDisplayName"
                data-testid="node-display-name-input"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">服务器地址 / IP</span>
              <input
                v-model="draftServer"
                data-testid="node-server-input"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
            <label class="form-control sm:col-span-1">
              <span class="label-text text-xs font-semibold">端口</span>
              <input
                v-model.number="draftPort"
                data-testid="node-port-input"
                type="number"
                min="1"
                max="65535"
                class="input input-bordered input-xs font-mono mt-1"
              />
            </label>
          </div>

          <!-- WireGuard Specific Fields -->
          <div v-if="selectedNode.protocol.toLowerCase() === 'wireguard'" class="space-y-2.5 pt-2 border-t border-base-300">
            <div class="font-bold text-primary uppercase tracking-wider text-[11px]">
              WireGuard 端点与对端配置
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label class="form-control">
                <span class="label-text text-xs font-semibold">内网地址 (CIDR)</span>
                <input
                  v-model="draftWgLocalAddress"
                  data-testid="wg-local-address-input"
                  placeholder="10.0.0.2/32, fd00::2/128"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">对端公钥 (Peer Public Key)</span>
                <input
                  v-model="draftWgPublicKey"
                  data-testid="wg-public-key-input"
                  placeholder="Base64 对端公钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">客户端私钥 (Private Key)</span>
                <input
                  v-model="draftWgPrivateKey"
                  data-testid="wg-private-key-input"
                  placeholder="Base64 客户端私钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">预共享密钥 (可选)</span>
                <input
                  v-model="draftWgPreSharedKey"
                  data-testid="wg-psk-input"
                  placeholder="Base64 预共享密钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">最大传输单元 (MTU)</span>
                <input
                  v-model.number="draftWgMtu"
                  data-testid="wg-mtu-input"
                  type="number"
                  min="576"
                  max="9000"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">DNS 服务器</span>
                <input
                  v-model="draftWgDns"
                  data-testid="wg-dns-input"
                  placeholder="1.1.1.1, 8.8.8.8"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control sm:col-span-2">
                <span class="label-text text-xs font-semibold">保留字节 (可选，3 个 uint8)</span>
                <input
                  v-model="draftWgReserved"
                  data-testid="wg-reserved-input"
                  placeholder="0, 0, 0"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
            </div>
          </div>

          <!-- TUIC Specific Fields -->
          <div v-else-if="selectedNode.protocol.toLowerCase() === 'tuic'" class="space-y-2.5 pt-2 border-t border-base-300">
            <div class="font-bold text-primary uppercase tracking-wider text-[11px]">
              TUIC v5 连接与 QUIC 传输参数
            </div>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label class="form-control">
                <span class="label-text text-xs font-semibold">UUID</span>
                <input
                  v-model="draftUuid"
                  data-testid="tuic-uuid-input"
                  placeholder="00000000-0000-4000-8000-000000000001"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">认证密码</span>
                <input
                  v-model="draftPassword"
                  data-testid="tuic-password-input"
                  placeholder="TUIC 认证密码"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">拥塞控制算法</span>
                <select
                  v-model="draftCongestionControl"
                  data-testid="tuic-cc-select"
                  class="select select-bordered select-xs font-mono mt-1"
                >
                  <option value="">（默认 / 未设置）</option>
                  <option value="bbr">bbr</option>
                  <option value="cubic">cubic</option>
                  <option value="new_reno">new_reno</option>
                </select>
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">UDP 转发模式</span>
                <select
                  v-model="draftUdpRelayMode"
                  data-testid="tuic-udp-mode-select"
                  class="select select-bordered select-xs font-mono mt-1"
                >
                  <option value="">（默认 / 未设置）</option>
                  <option value="native">native</option>
                  <option value="quic">quic</option>
                </select>
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">ALPN 协议协商</span>
                <input
                  v-model="draftAlpn"
                  data-testid="tuic-alpn-input"
                  placeholder="h3"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">SNI 域名</span>
                <input
                  v-model="draftSni"
                  data-testid="tuic-sni-input"
                  placeholder="tuic.example.com"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="flex items-center gap-2 cursor-pointer sm:col-span-2 pt-1">
                <input
                  v-model="draftDisableSni"
                  data-testid="tuic-disable-sni-checkbox"
                  type="checkbox"
                  class="checkbox checkbox-primary checkbox-xs"
                />
                <span class="label-text text-xs font-mono">禁用 SNI (disable_sni)</span>
              </label>
            </div>
          </div>

          <!-- Other Protocols (SS, VMess, VLESS, Trojan, Hysteria2) Plaintext Credential Fields -->
          <div v-else class="pt-2 border-t border-base-300 space-y-2.5">
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <label
                v-if="['vmess', 'vless'].includes(selectedNode.protocol.toLowerCase())"
                class="form-control"
              >
                <span class="label-text text-xs font-semibold">UUID</span>
                <input
                  v-model="draftUuid"
                  data-testid="node-uuid-input"
                  placeholder="UUID"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label
                v-if="['ss', 'shadowsocks', 'vmess'].includes(selectedNode.protocol.toLowerCase())"
                class="form-control"
              >
                <span class="label-text text-xs font-semibold">加密方式 (Cipher)</span>
                <input
                  v-model="draftMethod"
                  data-testid="node-method-input"
                  placeholder="aes-256-gcm, chacha20-ietf-poly1305..."
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control sm:col-span-2">
                <span class="label-text text-xs font-semibold">连接密码 / 密钥</span>
                <input
                  v-model="draftPassword"
                  data-testid="node-password-input"
                  placeholder="输入协议连接密码或密钥"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">SNI 域名</span>
                <input
                  v-model="draftSni"
                  data-testid="node-sni-input"
                  placeholder="sni.example.com"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="form-control">
                <span class="label-text text-xs font-semibold">ALPN 协议协商</span>
                <input
                  v-model="draftAlpn"
                  data-testid="node-alpn-input"
                  placeholder="h2, http/1.1"
                  class="input input-bordered input-xs font-mono mt-1"
                />
              </label>
              <label class="flex items-center gap-2 cursor-pointer sm:col-span-2 pt-1">
                <input
                  v-model="draftDisableSni"
                  data-testid="node-disable-sni-checkbox"
                  type="checkbox"
                  class="checkbox checkbox-primary checkbox-xs"
                />
                <span class="label-text text-xs font-mono">禁用 SNI (disable_sni)</span>
              </label>
            </div>
          </div>

          <div class="flex items-center justify-between pt-2">
            <span
              v-if="connectionError"
              data-testid="node-connection-error"
              class="text-error font-medium"
            >
              {{ connectionError }}
            </span>
            <span
              v-else-if="connectionSaved"
              data-testid="node-connection-saved"
              class="text-success font-medium"
            >
              连接参数已保存
            </span>
            <span v-else />
            <button
              type="submit"
              data-testid="node-save-connection-btn"
              class="btn btn-primary btn-xs"
              :disabled="savingConnection"
            >
              {{ t('common.save') }}
            </button>
          </div>
        </form>

        <!-- Node Config Snippet Preview (Mihomo YAML / sing-box JSON) -->
        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-bold text-xs">节点导出格式预览</span>
            <div class="flex items-center gap-1">
              <button
                type="button"
                data-testid="node-preview-target-mihomo"
                class="btn btn-xs font-mono"
                :class="previewTarget === 'mihomo' ? 'btn-primary' : 'btn-ghost'"
                @click="previewTarget = 'mihomo'"
              >
                Mihomo (YAML)
              </button>
              <button
                type="button"
                data-testid="node-preview-target-singbox"
                class="btn btn-xs font-mono"
                :class="previewTarget === 'singbox' ? 'btn-primary' : 'btn-ghost'"
                @click="previewTarget = 'singbox'"
              >
                sing-box (JSON)
              </button>
            </div>
          </div>
          <pre
            data-testid="node-config-preview"
            class="p-3 rounded-xl bg-base-300/50 border border-base-300 font-mono text-xs overflow-x-auto leading-relaxed"
          >{{ previewNodeSnippet }}</pre>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end w-full">
          <button
            type="button"
            data-testid="node-drawer-close-btn"
            class="btn btn-ghost btn-sm"
            @click="drawerOpen = false"
          >
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </DrawerCard>
  </section>
</template>

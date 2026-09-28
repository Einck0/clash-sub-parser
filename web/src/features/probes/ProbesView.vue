<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  PlayIcon,
  FunnelIcon,
  ClockIcon,
  Cog6ToothIcon,
  StopIcon,
  CheckCircleIcon,
  SparklesIcon,
  MagnifyingGlassIcon,
  ChevronDownIcon,
  ChevronUpIcon,
} from '@heroicons/vue/24/outline'
import { useProbes } from './useProbes'
import {
  ALL_PROBE_KINDS,
  SCHEDULE_PRESETS,
  formatRelativeTime,
  intervalLabel,
  probeBatchStateTone,
  probeKindEmoji,
  probeKindLabel,
  probeStateLabel,
  type ProbeBatch,
  type ProbeKind,
  type ProbeRun,
  type ProbeRunState,
} from './probeTypes'
import {
  SUPPORTED_NODE_PROTOCOLS,
  extractCapabilityVerdict,
  formatNodeLatency,
  nodeCapabilityLabel,
  nodeHealthBadge,
  nodeLatencyTone,
  nodeRiskBadge,
  nodeUnderlyingHealthCategory,
  resolveNodeLatencyMs,
  resolveNodeProbeState,
  type NormalizedNode,
} from '../nodes/nodeView'
import ProbeRunCard from './ProbeRunCard.vue'
import ProbeEvidenceSheet from './ProbeEvidenceSheet.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import ModalDialog from '../../ui/ModalDialog.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import { t } from '../../locales'

const {
  runs,
  activeRun,
  observations,
  nodeObservations,
  schedule,
  batches,
  probeNodes,
  subscriptions,
  nodeMap,
  subscriptionNameMap,
  probingNodeIds,
  queuedNodeIds,
  poolStatus,
  loadingPoolStatus,
  triggeringSchedule,
  loadingRuns,
  loadingObservations,
  loadingNodeObservations,
  loadingSchedule,
  loadingBatches,
  loadingNodes,
  savingSchedule,
  cancellingBatch,
  submitting,
  cancelling,
  error,
  totalRuns,
  totalBatches,
  loadProbeNodes,
  loadSubscriptions,
  loadNodeObservations,
  loadRuns,
  triggerQuickProbe,
  cancelRun,
  loadObservations,
  fetchRunDetail,
  loadSchedule,
  updateSchedule,
  loadBatches,
  cancelBatch,
  loadPoolStatus,
  triggerPeriodicPoolEnqueue,
} = useProbes()

const activeTab = ref<'workbench' | 'runs' | 'schedule'>('workbench')
const scheduleModalOpen = ref(false)
const evidenceSheetOpen = ref(false)
const selectedInspectNode = ref<NormalizedNode | null>(null)
const selectedStateFilter = ref<ProbeRunState | ''>('')
const historyCollapsed = ref(false)
const poolActionFeedback = ref('')

// Probe Dimension Pills state (baseline always included by default)
const selectedKinds = ref<ProbeKind[]>(['baseline', 'streaming', 'ai', 'ip_risk', 'geo'])

// Workbench Filters & Sorting
const searchQuery = ref('')
const protocolFilter = ref('all')
const subscriptionFilter = ref('all')
const healthFilter = ref<
  'all' | 'probing' | 'available' | 'healthy' | 'degraded' | 'unhealthy' | 'unprobed'
>('all')
const sortBy = ref<'latency_asc' | 'latency_desc' | 'name_asc'>('latency_asc')

// Multi-select nodes for targeted probing
const selectedNodeIds = ref<Set<string>>(new Set())

// Periodic Schedule form state (human-friendly presets + minutes)
const scheduleEnabled = ref(false)
const scheduleInterval = ref(3600)
const customMinutes = ref(60)
const scheduleKinds = ref<ProbeKind[]>(['baseline', 'streaming', 'ai'])

function toggleKindPill(kind: ProbeKind) {
  if (kind === 'baseline') return // baseline connectivity & latency is core required dimension
  const idx = selectedKinds.value.indexOf(kind)
  if (idx >= 0) {
    selectedKinds.value = selectedKinds.value.filter((k) => k !== kind)
  } else {
    selectedKinds.value = [...selectedKinds.value, kind]
  }
}

function selectAllKinds() {
  selectedKinds.value = ALL_PROBE_KINDS.map((k) => k.kind)
}

function getNodeProbeState(node: NormalizedNode): 'probing' | 'queued' | 'idle' {
  return resolveNodeProbeState(node, probingNodeIds.value, queuedNodeIds.value)
}

function getNodeHealthBadge(node: NormalizedNode) {
  return nodeHealthBadge(node, probingNodeIds.value, queuedNodeIds.value)
}

// ============================================================================
// Tier 1: Node Pool & Fleet Status Computed Metrics (5 Core Metrics)
// ============================================================================
const kpiStats = computed(() => {
  const nodes = probeNodes.value
  const pool = poolStatus.value

  let latencySum = 0
  let latencyCount = 0
  let fastCount = 0 // < 100ms
  let mediumCount = 0 // 100 - 250ms
  let slowCount = 0 // > 250ms

  let streamingUnlocked = 0
  let aiUnlocked = 0
  let lowRiskCount = 0

  for (const node of nodes) {
    const ms = resolveNodeLatencyMs(node)
    if (ms !== null && ms > 0) {
      latencySum += ms
      latencyCount += 1
      if (ms < 100) fastCount += 1
      else if (ms <= 250) mediumCount += 1
      else slowCount += 1
    }

    if (extractCapabilityVerdict(node.capabilities?.streaming) === 'available') {
      streamingUnlocked += 1
    }
    if (extractCapabilityVerdict(node.capabilities?.ai) === 'available') {
      aiUnlocked += 1
    }
    if (nodeRiskBadge(node).label === '低风险') {
      lowRiskCount += 1
    }
  }

  const total = pool.total_count > 0 || nodes.length === 0 ? pool.total_count : nodes.length
  const availableCount = pool.available_count
  const healthy = pool.healthy_count
  const degraded = pool.degraded_count
  const unhealthy = pool.unavailable_count
  const unprobed = pool.untested_count
  const queueNodesCount = pool.queue_nodes_count
  const probingCount = pool.probing_count
  const queuedWaitingCount = pool.queued_waiting_count

  const onlineRate = total > 0 ? Math.round((availableCount / total) * 100) : 0
  const avgLatency = latencyCount > 0 ? Math.round(latencySum / latencyCount) : null

  // Segmented bar percentages
  const denom = Math.max(1, total)
  const healthyPct = Math.round((healthy / denom) * 100)
  const degradedPct = Math.round((degraded / denom) * 100)
  const unhealthyPct = Math.round((unhealthy / denom) * 100)
  const unprobedPct = Math.max(0, 100 - healthyPct - degradedPct - unhealthyPct)
  const queueProgressPct =
    total > 0 && queueNodesCount > 0
      ? Math.max(8, Math.min(100, Math.round(((total - queueNodesCount) / total) * 100)))
      : 0

  return {
    queueNodesCount,
    probingCount,
    queuedWaitingCount,
    total,
    availableCount,
    onlineRate,
    healthy,
    degraded,
    unhealthy,
    unprobed,
    avgLatency,
    latencyCount,
    fastCount,
    mediumCount,
    slowCount,
    streamingUnlocked,
    aiUnlocked,
    lowRiskCount,
    healthyPct,
    degradedPct,
    unhealthyPct,
    unprobedPct,
    queueProgressPct,
  }
})

function selectPoolMetricFilter(
  target: 'all' | 'probing' | 'available' | 'healthy' | 'degraded' | 'unhealthy' | 'unprobed'
) {
  activeTab.value = 'workbench'
  if (target === 'all') {
    healthFilter.value = 'all'
    return
  }
  healthFilter.value = healthFilter.value === target ? 'all' : target
}

// Available subscription filter options derived from subscriptions + node sources
const subscriptionOptions = computed(() => {
  const map = new Map<string, string>()
  for (const sub of subscriptions.value) {
    map.set(sub.id, sub.name || sub.id)
  }
  for (const node of probeNodes.value) {
    for (const src of node.sources || []) {
      if (src.subscription_id && !map.has(src.subscription_id)) {
        map.set(src.subscription_id, subscriptionNameMap.value[src.subscription_id] || src.subscription_id)
      }
    }
  }
  return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
})

// Filtered & Sorted Nodes for Workbench Table
const filteredNodes = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const proto = protocolFilter.value.toLowerCase()
  const subId = subscriptionFilter.value
  const health = healthFilter.value

  const filtered = probeNodes.value.filter((node) => {
    if (proto !== 'all' && node.protocol.toLowerCase() !== proto) {
      return false
    }
    if (subId !== 'all') {
      const hasSub = (node.sources || []).some((s) => s.subscription_id === subId)
      if (!hasSub) return false
    }
    if (health !== 'all') {
      const liveState = getNodeProbeState(node)
      const category = nodeUnderlyingHealthCategory(node)
      if (health === 'probing' && liveState !== 'probing' && liveState !== 'queued') return false
      if (health === 'available' && category !== 'healthy' && category !== 'degraded') return false
      if (health === 'healthy' && category !== 'healthy') return false
      if (health === 'degraded' && category !== 'degraded') return false
      if (health === 'unhealthy' && category !== 'unhealthy') return false
      if (health === 'unprobed' && category !== 'unprobed') return false
    }
    if (q) {
      const matchName = node.displayName.toLowerCase().includes(q)
      const matchServer = (node.connection.server || '').toLowerCase().includes(q)
      const matchProto = node.protocol.toLowerCase().includes(q)
      if (!matchName && !matchServer && !matchProto) return false
    }
    return true
  })

  return [...filtered].sort((a, b) => {
    if (sortBy.value === 'name_asc') {
      return a.displayName.localeCompare(b.displayName)
    }
    const latA = resolveNodeLatencyMs(a)
    const latB = resolveNodeLatencyMs(b)
    if (latA === null && latB === null) return a.displayName.localeCompare(b.displayName)
    if (latA === null) return 1
    if (latB === null) return -1
    return sortBy.value === 'latency_asc' ? latA - latB : latB - latA
  })
})

const isAllSelected = computed(() => {
  if (filteredNodes.value.length === 0) return false
  return filteredNodes.value.every((n) => selectedNodeIds.value.has(n.logicalId))
})

function toggleSelectAll() {
  const next = new Set(selectedNodeIds.value)
  if (isAllSelected.value) {
    for (const n of filteredNodes.value) {
      next.delete(n.logicalId)
    }
  } else {
    for (const n of filteredNodes.value) {
      next.add(n.logicalId)
    }
  }
  selectedNodeIds.value = next
}

function toggleSelectNode(logicalId: string) {
  const next = new Set(selectedNodeIds.value)
  if (next.has(logicalId)) next.delete(logicalId)
  else next.add(logicalId)
  selectedNodeIds.value = next
}

function formatNodeSources(node: NormalizedNode): string[] {
  if (!node.sources || node.sources.length === 0) return []
  return node.sources.map((s) => subscriptionNameMap.value[s.subscription_id] || s.subscription_id)
}

// Active running/queued run for progress banner
const currentRunningRun = computed(() => {
  return runs.value.find((r) => r.state === 'running' || r.state === 'queued') ?? null
})

// ============================================================================
// One-Click Probing Actions (Front-of-Queue Preemption & Periodic Dedupe)
// ============================================================================
async function handleQuickFullProbe() {
  const count = probeNodes.value.length || kpiStats.value.total
  poolActionFeedback.value =
    count > 0
      ? `已将 ${count} 个活跃节点插队至节点池最前面优先检测`
      : '已发起全量节点插队至节点池最前面优先检测'
  try {
    await triggerQuickProbe({
      kinds: selectedKinds.value,
      nodeLogicalIds: [],
    })
  } catch {
    // error recorded in useProbes
  }
}

async function handleProbeSelectedNodes() {
  if (selectedNodeIds.value.size === 0) return
  const ids = Array.from(selectedNodeIds.value)
  poolActionFeedback.value = `已将 ${ids.length} 个已选节点插队至节点池最前面优先检测`
  try {
    await triggerQuickProbe({
      kinds: selectedKinds.value,
      nodeLogicalIds: ids,
    })
  } catch {
    // error recorded in useProbes
  }
}

async function handleProbeSingleNode(node: NormalizedNode) {
  poolActionFeedback.value = `已将节点「${node.displayName}」插队至节点池最前面优先检测`
  try {
    await triggerQuickProbe({
      kinds: selectedKinds.value,
      nodeLogicalIds: [node.logicalId],
    })
    if (evidenceSheetOpen.value && selectedInspectNode.value?.logicalId === node.logicalId) {
      const refreshed = nodeMap.value[node.logicalId]
      if (refreshed) selectedInspectNode.value = refreshed
      await loadNodeObservations(node.logicalId)
    }
  } catch {
    // error recorded in useProbes
  }
}

async function handleProbeUntestedNodes() {
  const targetIds = probeNodes.value
    .filter((n) => nodeUnderlyingHealthCategory(n) === 'unprobed')
    .map((n) => n.logicalId)
  if (targetIds.length === 0) {
    selectPoolMetricFilter('unprobed')
    return
  }
  healthFilter.value = 'probing'
  poolActionFeedback.value = `已将 ${targetIds.length} 个未测节点插队至节点池最前面优先检测`
  try {
    await triggerQuickProbe({
      kinds: selectedKinds.value,
      nodeLogicalIds: targetIds,
    })
  } catch {
    // error recorded in useProbes
  }
}

async function handleProbeUnavailableNodes() {
  const targetIds = probeNodes.value
    .filter((n) => nodeUnderlyingHealthCategory(n) === 'unhealthy')
    .map((n) => n.logicalId)
  if (targetIds.length === 0) {
    selectPoolMetricFilter('unhealthy')
    return
  }
  healthFilter.value = 'probing'
  poolActionFeedback.value = `已将 ${targetIds.length} 个不可用节点插队至节点池最前面优先检测`
  try {
    await triggerQuickProbe({
      kinds: selectedKinds.value,
      nodeLogicalIds: targetIds,
    })
  } catch {
    // error recorded in useProbes
  }
}

async function handleTriggerPeriodicPool() {
  poolActionFeedback.value = '已触发定时入池巡检（已在节点池中的节点自动去重跳过，不重复添加）'
  await triggerPeriodicPoolEnqueue()
}

// ============================================================================
// Inspect Node or Run Details in Structured Human-Readable Sheet
// ============================================================================
async function handleInspectNode(node: NormalizedNode) {
  selectedInspectNode.value = node
  activeRun.value = null
  evidenceSheetOpen.value = true
  await loadNodeObservations(node.logicalId)
}

async function handleInspectRun(run: ProbeRun) {
  selectedInspectNode.value = null
  activeRun.value = run
  evidenceSheetOpen.value = true
  await loadObservations(run.id)
}

function handleJumpToRun(rId: string) {
  const matched = runs.value.find((r) => r.id === rId)
  if (matched) {
    handleInspectRun(matched)
  } else {
    fetchRunDetail(rId).then((r) => {
      if (r) handleInspectRun(r)
    })
  }
}

const sheetObservations = computed(() => {
  return selectedInspectNode.value ? nodeObservations.value : observations.value
})

const sheetLoading = computed(() => {
  return selectedInspectNode.value ? loadingNodeObservations.value : loadingObservations.value
})

// ============================================================================
// Human-Friendly Periodic Schedule Configuration
// ============================================================================
function openScheduleModal() {
  if (schedule.value) {
    scheduleEnabled.value = schedule.value.enabled
    scheduleInterval.value = schedule.value.interval_seconds
    customMinutes.value = Math.max(1, Math.round(schedule.value.interval_seconds / 60))
    scheduleKinds.value =
      schedule.value.kinds && schedule.value.kinds.length > 0
        ? [...schedule.value.kinds]
        : ['baseline']
  }
  scheduleModalOpen.value = true
}

function selectSchedulePreset(seconds: number) {
  scheduleInterval.value = seconds
  customMinutes.value = Math.round(seconds / 60)
}

function onCustomMinutesInput() {
  const mins = Math.max(1, Math.min(10080, Number(customMinutes.value) || 60))
  scheduleInterval.value = mins * 60
}

async function handleQuickToggleSchedule() {
  const nextEnabled = !(schedule.value?.enabled ?? false)
  const interval = schedule.value?.interval_seconds || 3600
  const kinds =
    schedule.value?.kinds && schedule.value.kinds.length > 0
      ? schedule.value.kinds
      : (['baseline', 'streaming', 'ai'] as ProbeKind[])
  try {
    await updateSchedule({
      enabled: nextEnabled,
      interval_seconds: interval,
      kinds,
    })
  } catch {
    // error recorded in useProbes
  }
}

async function submitSaveSchedule() {
  try {
    await updateSchedule({
      enabled: scheduleEnabled.value,
      interval_seconds: Math.max(60, Number(scheduleInterval.value) || 3600),
      kinds: scheduleKinds.value.length > 0 ? scheduleKinds.value : ['baseline'],
    })
    scheduleModalOpen.value = false
  } catch {
    // error recorded in useProbes
  }
}

function formatBatchTitle(batch: ProbeBatch): string {
  try {
    const d = new Date(batch.window_at)
    if (Number.isNaN(d.getTime())) return '自动定时批次'
    return `自动定时批次 · ${d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}`
  } catch {
    return '自动定时批次'
  }
}

// ============================================================================
// Cancel Run Confirmation
// ============================================================================
const confirmCancelOpen = ref(false)
const pendingCancelRunId = ref<string | null>(null)

function handleCancel(runId: string) {
  pendingCancelRunId.value = runId
  confirmCancelOpen.value = true
}

async function handleConfirmCancel() {
  if (!pendingCancelRunId.value) return
  try {
    await cancelRun(pendingCancelRunId.value)
    confirmCancelOpen.value = false
    pendingCancelRunId.value = null
  } catch {
    // error handled in useProbes
  }
}

function handleFilterChange(state: ProbeRunState | '') {
  selectedStateFilter.value = state
  loadRuns(state || undefined)
}

function refreshAll() {
  loadPoolStatus()
  loadProbeNodes()
  loadSubscriptions()
  loadRuns(selectedStateFilter.value || undefined)
  loadSchedule()
  loadBatches()
}

let pollTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  refreshAll()
  pollTimer = setInterval(() => {
    const hasActiveRun = runs.value.some((r) => r.state === 'running' || r.state === 'queued')
    const hasActivePool =
      poolStatus.value.queue_nodes_count > 0 ||
      probingNodeIds.value.size > 0 ||
      queuedNodeIds.value.size > 0
    if (hasActiveRun || hasActivePool) {
      loadPoolStatus()
      loadRuns(selectedStateFilter.value || undefined)
      loadProbeNodes()
      if (activeRun.value && (activeRun.value.state === 'running' || activeRun.value.state === 'queued')) {
        loadObservations(activeRun.value.id)
      }
    }
  }, 2000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="probes-title">
    <!-- Header -->
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('probes.tag') }}</p>
        <h2 id="probes-title" class="mt-1 text-2xl font-bold">{{ t('probes.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">{{ t('probes.subtitle') }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          data-testid="header-schedule-btn"
          class="btn btn-outline btn-sm gap-2 touch-manipulation"
          @click="openScheduleModal"
        >
          <ClockIcon class="w-4 h-4 text-primary" />
          {{ t('probes.periodicSchedule') }}
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square touch-manipulation"
          :title="t('common.refresh')"
          @click="refreshAll"
        >
          <ArrowPathIcon
            class="w-4 h-4"
            :class="{ 'animate-spin': loadingNodes || loadingRuns || loadingSchedule || loadingBatches || loadingPoolStatus }"
          />
        </button>
        <button
          type="button"
          data-testid="quick-full-probe-btn"
          class="btn btn-primary btn-sm gap-2 touch-manipulation shadow-sm"
          :disabled="submitting"
          @click="handleQuickFullProbe"
        >
          <BoltIcon class="w-4 h-4" :class="{ 'animate-spin': submitting }" />
          {{ t('probes.triggerRun') }}
        </button>
      </div>
    </div>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loadingRuns || loadingNodes || savingSchedule || cancellingBatch"
      @retry="refreshAll"
    />

    <!-- =====================================================================
         Tier 1: Node Pool & Fleet Status Dashboard (节点池与全网检测状态看板 — 5 大核心指标 + 定时去重入池)
         ===================================================================== -->
    <div
      data-testid="probe-kpi-bar"
      class="space-y-3"
    >
      <div
        data-testid="probe-pool-dashboard"
        class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3"
      >
        <!-- Metric 1: 当前队列中的节点数 -->
        <article
          data-testid="pool-metric-queue"
          role="button"
          tabindex="0"
          class="card bg-base-200 border shadow-sm p-4 flex flex-col justify-between gap-2.5 cursor-pointer transition-all hover:border-info/60 hover:shadow-md"
          :class="
            healthFilter === 'probing'
              ? 'border-info ring-2 ring-info/25 bg-info/5'
              : kpiStats.queueNodesCount > 0
              ? 'border-info/40'
              : 'border-base-300'
          "
          @click="selectPoolMetricFilter('probing')"
          @keydown.enter.prevent="selectPoolMetricFilter('probing')"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold opacity-75 whitespace-nowrap">当前队列中的节点数</span>
            <span
              class="inline-flex items-center gap-1 text-[11px] font-semibold px-2 py-0.5 rounded-full whitespace-nowrap shrink-0"
              :class="
                kpiStats.queueNodesCount > 0
                  ? 'bg-info/15 text-info animate-pulse'
                  : 'bg-base-300/70 text-base-content/60'
              "
            >
              <BoltIcon class="w-3.5 h-3.5 shrink-0" :class="{ 'animate-spin': kpiStats.probingCount > 0 }" />
              <span>{{ kpiStats.queueNodesCount > 0 ? '实时检测中' : '队列空闲' }}</span>
            </span>
          </div>

          <div class="flex items-baseline justify-between gap-2">
            <span class="text-3xl font-extrabold font-mono text-info">
              {{ kpiStats.queueNodesCount }}
            </span>
            <span class="text-[11px] opacity-65 font-mono whitespace-nowrap">
              / {{ kpiStats.total }} 节点
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
            <span class="badge badge-xs badge-info gap-1 font-mono h-auto py-0.5 whitespace-nowrap">
              ⚡ 检测中 {{ kpiStats.probingCount }}
            </span>
            <span class="badge badge-xs badge-warning badge-outline gap-1 font-mono h-auto py-0.5 whitespace-nowrap">
              ⏳ 排队等待 {{ kpiStats.queuedWaitingCount }}
            </span>
          </div>

          <progress
            v-if="kpiStats.queueNodesCount > 0"
            class="progress progress-info w-full h-1.5"
            :value="kpiStats.queueProgressPct"
            max="100"
          />
          <p class="text-[11px] opacity-70 leading-tight">
            手动插队最前 · 定时去重入池
          </p>
        </article>

        <!-- Metric 2: 总数 -->
        <article
          data-testid="pool-metric-total"
          role="button"
          tabindex="0"
          class="card bg-base-200 border shadow-sm p-4 flex flex-col justify-between gap-2.5 cursor-pointer transition-all hover:border-primary/50 hover:shadow-md"
          :class="healthFilter === 'all' ? 'border-primary/60 ring-1 ring-primary/20' : 'border-base-300'"
          @click="selectPoolMetricFilter('all')"
          @keydown.enter.prevent="selectPoolMetricFilter('all')"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold opacity-75 whitespace-nowrap">总数</span>
            <span class="badge badge-xs badge-ghost font-mono h-auto py-0.5 whitespace-nowrap">全网活跃</span>
          </div>

          <div class="flex items-baseline justify-between gap-2">
            <span class="text-3xl font-extrabold font-mono">
              {{ kpiStats.total }}
            </span>
            <span class="text-xs opacity-65 whitespace-nowrap">活跃节点</span>
          </div>

          <div class="flex flex-wrap items-center gap-1.5 text-[11px] opacity-85">
            <span class="badge badge-xs badge-primary badge-outline font-mono h-auto py-0.5 whitespace-nowrap">
              平均响应延迟 {{ kpiStats.avgLatency !== null ? `${kpiStats.avgLatency} ms` : '未测速' }}
            </span>
            <span v-if="subscriptionOptions.length > 0" class="badge badge-xs badge-ghost h-auto py-0.5 whitespace-nowrap">
              {{ subscriptionOptions.length }} 个订阅源
            </span>
          </div>

          <p class="text-[11px] opacity-65 leading-tight">
            点击查看全部节点与实时测速状态
          </p>
        </article>

        <!-- Metric 3: 可用数 -->
        <article
          data-testid="pool-metric-available"
          role="button"
          tabindex="0"
          class="card bg-base-200 border shadow-sm p-4 flex flex-col justify-between gap-2.5 cursor-pointer transition-all hover:border-success/60 hover:shadow-md"
          :class="
            healthFilter === 'available' || healthFilter === 'healthy'
              ? 'border-success ring-2 ring-success/25 bg-success/5'
              : 'border-base-300'
          "
          @click="selectPoolMetricFilter('available')"
          @keydown.enter.prevent="selectPoolMetricFilter('available')"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold opacity-75 flex items-center gap-1 whitespace-nowrap">
              <CheckCircleIcon class="w-4 h-4 text-success shrink-0" />
              <span>可用数</span>
            </span>
            <span class="text-xs font-bold text-success font-mono whitespace-nowrap">
              在线可用率 {{ kpiStats.onlineRate }}%
            </span>
          </div>

          <div class="flex items-baseline justify-between gap-2">
            <span class="text-3xl font-extrabold font-mono text-success">
              {{ kpiStats.availableCount }}
            </span>
            <span class="text-xs font-mono opacity-70 whitespace-nowrap">
              {{ kpiStats.avgLatency !== null ? `${kpiStats.avgLatency} ms` : '未测速' }}
            </span>
          </div>

          <progress
            class="progress progress-success w-full h-1.5"
            :value="kpiStats.onlineRate"
            max="100"
          />

          <div class="flex flex-wrap items-center gap-1.5 text-[11px] opacity-85">
            <span class="badge badge-xs badge-success badge-outline font-mono h-auto py-0.5 whitespace-nowrap">
              正常 {{ kpiStats.healthy }}
            </span>
            <span class="badge badge-xs badge-warning badge-outline font-mono h-auto py-0.5 whitespace-nowrap">
              降级 {{ kpiStats.degraded }}
            </span>
          </div>
        </article>

        <!-- Metric 4: 不可用数 -->
        <article
          data-testid="pool-metric-unavailable"
          role="button"
          tabindex="0"
          class="card bg-base-200 border shadow-sm p-4 flex flex-col justify-between gap-2.5 cursor-pointer transition-all hover:border-error/60 hover:shadow-md"
          :class="
            healthFilter === 'unhealthy'
              ? 'border-error ring-2 ring-error/25 bg-error/5'
              : 'border-base-300'
          "
          @click="selectPoolMetricFilter('unhealthy')"
          @keydown.enter.prevent="selectPoolMetricFilter('unhealthy')"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold opacity-75 whitespace-nowrap">不可用数</span>
            <span class="badge badge-xs badge-error badge-outline font-mono h-auto py-0.5 whitespace-nowrap">
              异常 / 不可达
            </span>
          </div>

          <div class="flex items-baseline justify-between gap-2">
            <span class="text-3xl font-extrabold font-mono text-error">
              {{ kpiStats.unhealthy }}
            </span>
            <span class="text-xs opacity-65 font-mono whitespace-nowrap">
              {{ kpiStats.unhealthyPct }}%
            </span>
          </div>

          <div class="flex items-center justify-between gap-2 pt-0.5" @click.stop>
            <span class="text-[11px] opacity-65 truncate">握手失败或连接超时</span>
            <button
              type="button"
              data-testid="pool-probe-unavailable-btn"
              class="btn btn-xs btn-error btn-outline shrink-0 whitespace-nowrap"
              :disabled="submitting || kpiStats.unhealthy === 0"
              @click="handleProbeUnavailableNodes"
            >
              插队重测
            </button>
          </div>
        </article>

        <!-- Metric 5: 未测数 -->
        <article
          data-testid="pool-metric-untested"
          role="button"
          tabindex="0"
          class="card bg-base-200 border shadow-sm p-4 flex flex-col justify-between gap-2.5 cursor-pointer transition-all hover:border-warning/60 hover:shadow-md"
          :class="
            healthFilter === 'unprobed'
              ? 'border-warning ring-2 ring-warning/25 bg-warning/5'
              : 'border-base-300'
          "
          @click="selectPoolMetricFilter('unprobed')"
          @keydown.enter.prevent="selectPoolMetricFilter('unprobed')"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold opacity-75 whitespace-nowrap">未测数</span>
            <span class="badge badge-xs badge-ghost font-mono h-auto py-0.5 whitespace-nowrap">待入池检测</span>
          </div>

          <div class="flex items-baseline justify-between gap-2">
            <span class="text-3xl font-extrabold font-mono text-warning">
              {{ kpiStats.unprobed }}
            </span>
            <span class="text-xs opacity-65 font-mono whitespace-nowrap">
              {{ kpiStats.unprobedPct }}%
            </span>
          </div>

          <div class="flex items-center justify-between gap-2 pt-0.5" @click.stop>
            <span class="text-[11px] opacity-65 truncate">尚无探测观测记录</span>
            <button
              type="button"
              data-testid="pool-probe-untested-btn"
              class="btn btn-xs btn-warning btn-outline shrink-0 whitespace-nowrap"
              :disabled="submitting || kpiStats.unprobed === 0"
              @click="handleProbeUntestedNodes"
            >
              插队检测
            </button>
          </div>
        </article>

        <!-- Card 6: 定时自动入池控制卡 (Periodic Pool Deduplication Control) -->
        <article
          data-testid="pool-schedule-card"
          class="card bg-base-200 border border-base-300 shadow-sm p-4 flex flex-col justify-between gap-2.5"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold opacity-75 whitespace-nowrap">定时自动入池状态</span>
            <StatusBadge
              :label="schedule?.enabled ? '自动巡检开启' : '未启用'"
              :tone="schedule?.enabled ? 'success' : 'info'"
            />
          </div>

          <div>
            <div class="text-sm font-bold flex items-center justify-between gap-2">
              <span class="whitespace-nowrap">{{ schedule ? intervalLabel(schedule.interval_seconds) : '每 1 小时' }}</span>
              <span class="text-[11px] font-normal opacity-65 whitespace-nowrap">
                下次：{{
                  schedule?.enabled && schedule?.next_due_at
                    ? new Date(schedule.next_due_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                    : '等待启用'
                }}
              </span>
            </div>
            <p class="text-[11px] opacity-65 mt-0.5 leading-tight">
              已在节点池中不重复添加 · 手动插队最前
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-1.5 pt-0.5">
            <button
              type="button"
              data-testid="trigger-periodic-pool-btn"
              class="btn btn-xs btn-primary gap-1 whitespace-nowrap"
              :disabled="triggeringSchedule"
              @click="handleTriggerPeriodicPool"
            >
              <ArrowPathIcon class="w-3 h-3 shrink-0" :class="{ 'animate-spin': triggeringSchedule }" />
              <span>立即定时入池（去重）</span>
            </button>
            <button
              type="button"
              data-testid="quick-toggle-schedule-btn"
              class="btn btn-xs whitespace-nowrap"
              :class="schedule?.enabled ? 'btn-ghost text-warning' : 'btn-success btn-outline'"
              :disabled="savingSchedule"
              @click="handleQuickToggleSchedule"
            >
              {{ schedule?.enabled ? '暂停' : '开启' }}
            </button>
            <button
              type="button"
              data-testid="open-schedule-modal-btn"
              class="btn btn-xs btn-ghost whitespace-nowrap"
              @click="openScheduleModal"
            >
              调整策略
            </button>
          </div>
        </article>
      </div>

      <!-- Fleet Health Segmented Bar & Unlock Telemetry Strip -->
      <div
        data-testid="pool-health-segmented-bar"
        class="card bg-base-200/80 border border-base-300 px-4 py-3 flex flex-col gap-2.5"
      >
        <div class="flex flex-wrap items-center justify-between gap-2 text-xs">
          <div class="flex flex-wrap items-center gap-3">
            <span class="font-semibold opacity-80 whitespace-nowrap">节点池与全网健康分布</span>
            <span class="inline-flex items-center gap-1 text-[11px] whitespace-nowrap">
              <span class="w-2 h-2 rounded-full bg-info animate-pulse shrink-0" />
              <span>队列中 {{ kpiStats.queueNodesCount }} (检测中 {{ kpiStats.probingCount }} / 排队 {{ kpiStats.queuedWaitingCount }})</span>
            </span>
            <span class="inline-flex items-center gap-1 text-[11px] whitespace-nowrap">
              <span class="w-2 h-2 rounded-full bg-success shrink-0" />
              <span>可用 {{ kpiStats.availableCount }} (正常 {{ kpiStats.healthy }} / 降级 {{ kpiStats.degraded }})</span>
            </span>
            <span class="inline-flex items-center gap-1 text-[11px] whitespace-nowrap">
              <span class="w-2 h-2 rounded-full bg-error shrink-0" />
              <span>不可用 {{ kpiStats.unhealthy }}</span>
            </span>
            <span class="inline-flex items-center gap-1 text-[11px] whitespace-nowrap">
              <span class="w-2 h-2 rounded-full bg-base-content/30 shrink-0" />
              <span>未测 {{ kpiStats.unprobed }}</span>
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-2 text-[11px]">
            <SparklesIcon class="w-3.5 h-3.5 text-secondary shrink-0" />
            <span class="badge badge-xs badge-success badge-outline h-auto py-0.5 whitespace-nowrap">
              🎬 流媒体解锁 {{ kpiStats.streamingUnlocked }}
            </span>
            <span class="badge badge-xs badge-primary badge-outline h-auto py-0.5 whitespace-nowrap">
              🤖 AI 可用 {{ kpiStats.aiUnlocked }}
            </span>
            <span class="badge badge-xs badge-info badge-outline h-auto py-0.5 whitespace-nowrap">
              🛡️ 纯净 IP {{ kpiStats.lowRiskCount }}
            </span>
            <span class="badge badge-xs badge-ghost font-mono h-auto py-0.5 whitespace-nowrap">
              极速 &lt;100ms: {{ kpiStats.fastCount }}
            </span>
          </div>
        </div>

        <div class="w-full h-2 rounded-full bg-base-300 overflow-hidden flex">
          <div
            v-if="kpiStats.healthyPct > 0"
            class="h-full bg-success transition-all duration-300"
            :style="{ width: `${kpiStats.healthyPct}%` }"
            :title="`正常: ${kpiStats.healthy}`"
          />
          <div
            v-if="kpiStats.degradedPct > 0"
            class="h-full bg-warning transition-all duration-300"
            :style="{ width: `${kpiStats.degradedPct}%` }"
            :title="`降级: ${kpiStats.degraded}`"
          />
          <div
            v-if="kpiStats.unhealthyPct > 0"
            class="h-full bg-error transition-all duration-300"
            :style="{ width: `${kpiStats.unhealthyPct}%` }"
            :title="`不可用: ${kpiStats.unhealthy}`"
          />
          <div
            v-if="kpiStats.unprobedPct > 0"
            class="h-full bg-base-content/20 transition-all duration-300"
            :style="{ width: `${kpiStats.unprobedPct}%` }"
            :title="`未测: ${kpiStats.unprobed}`"
          />
        </div>
      </div>
    </div>

    <!-- Navigation Tabs: Node Workbench vs Manual Runs vs Periodic Schedule & Batches -->
    <div class="tabs tabs-boxed bg-base-200/60 p-1 rounded-xl inline-flex flex-wrap w-fit">
      <button
        type="button"
        data-testid="workbench-tab"
        class="tab tab-sm font-semibold gap-2 transition-all"
        :class="{ 'tab-active': activeTab === 'workbench' }"
        @click="activeTab = 'workbench'"
      >
        <BoltIcon class="w-4 h-4" />
        {{ t('probes.workbenchTab') }} ({{ probeNodes.length }})
      </button>
      <button
        type="button"
        data-testid="runs-tab"
        class="tab tab-sm font-semibold gap-2 transition-all"
        :class="{ 'tab-active': activeTab === 'runs' }"
        @click="activeTab = 'runs'"
      >
        <PlayIcon class="w-4 h-4" />
        {{ t('probes.runsTab') }} ({{ totalRuns }})
      </button>
      <button
        type="button"
        data-testid="schedule-tab"
        class="tab tab-sm font-semibold gap-2 transition-all"
        :class="{ 'tab-active': activeTab === 'schedule' }"
        @click="activeTab = 'schedule'"
      >
        <ClockIcon class="w-4 h-4" />
        {{ t('probes.scheduleTab') }}
        <span
          v-if="schedule"
          class="badge badge-xs"
          :class="schedule.enabled ? 'badge-success' : 'badge-ghost'"
        >
          {{ schedule.enabled ? '已启用' : '已停用' }}
        </span>
      </button>
    </div>

    <!-- =====================================================================
         Tab 1: Node Probe Workbench (以节点为核心的实时测速工作台)
         ===================================================================== -->
    <div v-if="activeTab === 'workbench'" class="space-y-4">
      <!-- Tier 2: Action & Filter Toolbar -->
      <div
        data-testid="probe-action-toolbar"
        class="card bg-base-200 border border-base-300 shadow-sm p-4 space-y-3.5"
      >
        <!-- Top Row: One-Click Action Buttons + Probe Dimension Pills -->
        <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              data-testid="toolbar-full-probe-btn"
              class="btn btn-primary btn-sm gap-1.5 shadow-sm"
              :disabled="submitting"
              @click="handleQuickFullProbe"
            >
              <BoltIcon class="w-4 h-4" :class="{ 'animate-spin': submitting }" />
              <span>一键全量测速</span>
            </button>
            <button
              type="button"
              data-testid="probe-selected-nodes-btn"
              class="btn btn-secondary btn-outline btn-sm gap-1.5"
              :disabled="submitting || selectedNodeIds.size === 0"
              @click="handleProbeSelectedNodes"
            >
              <span>🎯 测速已选节点 ({{ selectedNodeIds.size }})</span>
            </button>
          </div>

          <!-- Probe Dimension Pills -->
          <div
            data-testid="probe-dimension-pills"
            class="flex flex-wrap items-center gap-1.5 text-xs"
            role="group"
            aria-label="探测维度选择"
          >
            <span class="opacity-60 mr-1 font-medium">探测维度：</span>
            <button
              v-for="item in ALL_PROBE_KINDS"
              :key="item.kind"
              type="button"
              :data-testid="`probe-pill-${item.kind}`"
              class="btn btn-xs rounded-full gap-1 transition-all"
              :class="
                selectedKinds.includes(item.kind)
                  ? 'btn-primary shadow-xs'
                  : 'btn-ghost bg-base-300/60 opacity-70'
              "
              :title="item.desc"
              @click="toggleKindPill(item.kind)"
            >
              <span>{{ item.emoji }}</span>
              <span>{{ item.label }}</span>
              <span v-if="item.kind === 'baseline'" class="text-[10px] opacity-75">(必选)</span>
            </button>
            <button
              type="button"
              class="btn btn-ghost btn-xs text-primary"
              @click="selectAllKinds"
            >
              全选
            </button>
          </div>
        </div>

        <!-- Real-Time Active Probe & Pool Preemption Feedback Banner -->
        <div
          v-if="poolActionFeedback || currentRunningRun || submitting || kpiStats.queueNodesCount > 0"
          data-testid="active-probe-progress-banner"
          class="rounded-xl bg-primary/10 border border-primary/30 px-3.5 py-2.5 flex flex-wrap items-center justify-between gap-3 text-xs"
        >
          <div class="flex items-center gap-2.5 min-w-0 flex-1">
            <span
              v-if="currentRunningRun || submitting || kpiStats.queueNodesCount > 0"
              class="loading loading-spinner loading-xs text-primary shrink-0"
            />
            <BoltIcon v-else class="w-4 h-4 text-primary shrink-0" />
            <div class="min-w-0 flex-1">
              <div class="font-semibold text-primary flex flex-wrap items-center gap-2">
                <span data-testid="pool-action-feedback">
                  {{
                    poolActionFeedback ||
                      `节点池实时检测中：当前队列 ${kpiStats.queueNodesCount} 个节点（检测中 ${kpiStats.probingCount} · 排队等待 ${kpiStats.queuedWaitingCount}）`
                  }}
                </span>
                <span v-if="currentRunningRun" class="badge badge-xs badge-primary">
                  {{ probeStateLabel(currentRunningRun.state) }}
                </span>
              </div>
              <progress
                v-if="currentRunningRun || submitting || kpiStats.queueNodesCount > 0"
                class="progress progress-primary w-full max-w-md h-1.5 mt-1"
              />
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <button
              v-if="currentRunningRun"
              type="button"
              class="btn btn-xs btn-ghost"
              @click="handleInspectRun(currentRunningRun)"
            >
              查看实时观测
            </button>
            <button
              v-if="currentRunningRun"
              type="button"
              data-testid="banner-cancel-run-btn"
              class="btn btn-xs btn-error btn-outline"
              :disabled="cancelling"
              @click="handleCancel(currentRunningRun.id)"
            >
              取消测速
            </button>
            <button
              v-if="poolActionFeedback && !currentRunningRun"
              type="button"
              class="btn btn-xs btn-ghost"
              @click="poolActionFeedback = ''"
            >
              知道了
            </button>
          </div>
        </div>

        <!-- Quick Status Filter Capsules -->
        <div
          data-testid="probe-status-quick-pills"
          class="flex flex-wrap items-center gap-1.5 text-xs pt-1"
        >
          <span class="opacity-60 mr-1 font-medium">节点状态筛选：</span>
          <button
            type="button"
            data-testid="status-pill-all"
            class="btn btn-xs rounded-full"
            :class="healthFilter === 'all' ? 'btn-primary' : 'btn-ghost bg-base-300/60'"
            @click="healthFilter = 'all'"
          >
            全部 ({{ kpiStats.total }})
          </button>
          <button
            type="button"
            data-testid="status-pill-probing"
            class="btn btn-xs rounded-full gap-1"
            :class="healthFilter === 'probing' ? 'btn-info' : 'btn-ghost bg-base-300/60'"
            @click="healthFilter = 'probing'"
          >
            <span class="w-1.5 h-1.5 rounded-full bg-current" :class="{ 'animate-ping': kpiStats.queueNodesCount > 0 }" />
            <span>检测中 ({{ kpiStats.queueNodesCount }})</span>
          </button>
          <button
            type="button"
            data-testid="status-pill-available"
            class="btn btn-xs rounded-full"
            :class="healthFilter === 'available' ? 'btn-success' : 'btn-ghost bg-base-300/60'"
            @click="healthFilter = 'available'"
          >
            可用 ({{ kpiStats.availableCount }})
          </button>
          <button
            type="button"
            data-testid="status-pill-unhealthy"
            class="btn btn-xs rounded-full"
            :class="healthFilter === 'unhealthy' ? 'btn-error' : 'btn-ghost bg-base-300/60'"
            @click="healthFilter = 'unhealthy'"
          >
            不可用 ({{ kpiStats.unhealthy }})
          </button>
          <button
            type="button"
            data-testid="status-pill-unprobed"
            class="btn btn-xs rounded-full"
            :class="healthFilter === 'unprobed' ? 'btn-warning' : 'btn-ghost bg-base-300/60'"
            @click="healthFilter = 'unprobed'"
          >
            未测 ({{ kpiStats.unprobed }})
          </button>
        </div>

        <!-- Bottom Row: Multi-dimensional Node Filter & Sort Bar -->
        <div
          data-testid="probe-filter-bar"
          class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-2.5 pt-2 border-t border-base-300"
        >
          <!-- Keyword Search -->
          <label class="input input-bordered input-xs sm:input-sm flex items-center gap-2">
            <MagnifyingGlassIcon class="w-4 h-4 opacity-50 shrink-0" />
            <input
              v-model="searchQuery"
              data-testid="probe-search-input"
              type="search"
              placeholder="搜索节点名称或服务器..."
              class="grow font-mono text-xs"
            />
          </label>

          <!-- Protocol Filter -->
          <select
            v-model="protocolFilter"
            data-testid="probe-protocol-select"
            aria-label="按协议筛选"
            class="select select-bordered select-xs sm:select-sm text-xs font-mono"
          >
            <option value="all">全部协议</option>
            <option v-for="proto in SUPPORTED_NODE_PROTOCOLS" :key="proto" :value="proto">
              {{ proto.toUpperCase() }}
            </option>
          </select>

          <!-- Subscription Source Filter -->
          <select
            v-model="subscriptionFilter"
            data-testid="probe-subscription-select"
            aria-label="按订阅源筛选"
            class="select select-bordered select-xs sm:select-sm text-xs"
          >
            <option value="all">全部订阅源</option>
            <option v-for="sub in subscriptionOptions" :key="sub.id" :value="sub.id">
              {{ sub.name }}
            </option>
          </select>

          <!-- Health Status Filter -->
          <select
            v-model="healthFilter"
            data-testid="probe-health-filter"
            aria-label="按健康状态筛选"
            class="select select-bordered select-xs sm:select-sm text-xs"
          >
            <option value="all">全部状态 ({{ kpiStats.total }})</option>
            <option value="probing">检测中 ({{ kpiStats.queueNodesCount }})</option>
            <option value="available">可用 ({{ kpiStats.availableCount }})</option>
            <option value="healthy">正常 ({{ kpiStats.healthy }})</option>
            <option value="degraded">降级 ({{ kpiStats.degraded }})</option>
            <option value="unhealthy">异常 / 不可用 ({{ kpiStats.unhealthy }})</option>
            <option value="unprobed">未测速 ({{ kpiStats.unprobed }})</option>
          </select>

          <!-- Sort Order -->
          <select
            v-model="sortBy"
            data-testid="probe-sort-select"
            aria-label="排序方式"
            class="select select-bordered select-xs sm:select-sm text-xs"
          >
            <option value="latency_asc">延迟：从快到慢</option>
            <option value="latency_desc">延迟：从慢到快</option>
            <option value="name_asc">按节点名称排序</option>
          </select>
        </div>
      </div>

      <!-- Tier 3: Node Probe Workbench Table / Responsive List -->
      <div
        v-if="loadingNodes && probeNodes.length === 0"
        class="space-y-2.5"
      >
        <div v-for="i in 5" :key="i" class="skeleton h-16 rounded-xl" />
      </div>

      <EmptyState
        v-else-if="filteredNodes.length === 0"
        :icon="BoltIcon"
        :title="probeNodes.length === 0 ? t('probes.emptyTitle') : '没有匹配筛选条件的节点'"
        :description="probeNodes.length === 0 ? t('probes.emptyDesc') : '请尝试清空搜索词或切换协议、订阅源与状态筛选条件。'"
        :action-label="t('probes.triggerRun')"
        @action="handleQuickFullProbe"
      >
        <template #action-icon>
          <BoltIcon class="w-4 h-4" />
        </template>
      </EmptyState>

      <div
        v-else
        data-testid="node-probe-workbench-table"
        class="card bg-base-200 border border-base-300 shadow-sm overflow-hidden"
      >
        <div class="overflow-x-auto probe-node-table-wrap">
          <table class="table table-sm sm:table-md w-full align-middle probe-node-table">
            <thead>
              <tr class="bg-base-300/50 text-xs uppercase tracking-wider">
                <th class="w-10">
                  <input
                    type="checkbox"
                    data-testid="select-all-nodes-checkbox"
                    aria-label="全选当前筛选节点"
                    class="checkbox checkbox-primary checkbox-xs"
                    :checked="isAllSelected"
                    @change="toggleSelectAll"
                  />
                </th>
                <th class="probe-node-identity">节点名称 / 协议与入口</th>
                <th>连通状态与延迟</th>
                <th>流媒体 / AI / 地区 / 风险</th>
                <th>最后测速</th>
                <th class="text-right probe-node-actions">快捷操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="node in filteredNodes"
                :key="node.logicalId"
                data-testid="probe-node-row"
                class="probe-node-row hover:bg-base-300/30 transition-colors"
              >
                <td class="probe-node-select">
                  <input
                    type="checkbox"
                    data-testid="select-node-checkbox"
                    :aria-label="`选择节点 ${node.displayName}`"
                    class="checkbox checkbox-primary checkbox-xs"
                    :checked="selectedNodeIds.has(node.logicalId)"
                    @change="toggleSelectNode(node.logicalId)"
                  />
                </td>

                <!-- Node Name, Protocol, Server:Port & Subscription Source -->
                <td class="min-w-0 probe-node-identity" data-label="节点 / 协议与入口">
                  <div class="flex min-w-0 flex-wrap items-center gap-2">
                    <span class="min-w-0 max-w-full break-all font-semibold text-xs sm:text-sm">
                      {{ node.displayName }}
                    </span>
                    <span class="badge badge-xs badge-outline font-mono uppercase shrink-0">
                      {{ node.protocol }}
                    </span>
                  </div>
                  <div class="flex min-w-0 flex-wrap items-center gap-1.5 mt-1 text-[11px] font-mono opacity-70 break-all">
                    <span v-if="node.connection.server && node.connection.port">
                      {{ node.connection.server }}:{{ node.connection.port }}
                    </span>
                    <span
                      v-for="srcName in formatNodeSources(node)"
                      :key="srcName"
                      data-testid="probe-node-source-badge"
                      class="badge badge-xs badge-ghost font-sans h-auto py-0.5 leading-tight max-w-full whitespace-normal break-all"
                    >
                      {{ srcName }}
                    </span>
                  </div>
                </td>

                <!-- Connectivity Health & Semantic Colored Latency Badge -->
                <td class="whitespace-nowrap" data-label="连通状态与延迟">
                  <div class="flex flex-wrap items-center gap-1.5">
                    <StatusBadge
                      data-testid="probe-node-status-badge"
                      :label="getNodeHealthBadge(node).label"
                      :tone="getNodeHealthBadge(node).tone"
                      :pulse="getNodeProbeState(node) !== 'idle'"
                    />
                    <span
                      v-if="getNodeProbeState(node) === 'queued'"
                      class="badge badge-xs badge-warning badge-outline h-auto py-0.5 whitespace-nowrap"
                    >
                      队列等待
                    </span>
                    <StatusBadge
                      v-if="resolveNodeLatencyMs(node) !== null"
                      data-testid="probe-node-latency-badge"
                      :label="formatNodeLatency(node)"
                      :tone="nodeLatencyTone(node)"
                    />
                    <span
                      v-else
                      data-testid="probe-node-latency-badge"
                      class="font-mono text-xs opacity-60 px-1.5"
                    >
                      --
                    </span>
                  </div>
                </td>

                <!-- Capability Matrix Badges -->
                <td data-label="探测结果">
                  <div class="flex flex-wrap items-center gap-1.5 text-xs">
                    <StatusBadge
                      :label="`🎬 流媒体: ${nodeCapabilityLabel(node, 'streaming').label}`"
                      :tone="nodeCapabilityLabel(node, 'streaming').tone"
                    />
                    <StatusBadge
                      :label="`🤖 AI: ${nodeCapabilityLabel(node, 'ai').label}`"
                      :tone="nodeCapabilityLabel(node, 'ai').tone"
                    />
                    <StatusBadge
                      :label="`🌍 地区: ${nodeCapabilityLabel(node, 'geo').label}`"
                      :tone="nodeCapabilityLabel(node, 'geo').tone"
                    />
                    <StatusBadge
                      :label="`🛡️ 风险: ${nodeRiskBadge(node).label}`"
                      :tone="nodeRiskBadge(node).tone"
                    />
                  </div>
                </td>

                <!-- Relative Last Probed Time -->
                <td class="whitespace-nowrap text-xs font-mono opacity-75" data-label="最后测速">
                  <span>{{ formatRelativeTime(node.lastProbedAt) }}</span>
                  <span
                    v-if="node.probeStale"
                    class="badge badge-xs badge-warning ml-1 font-sans"
                    title="测速数据已超过 1 小时保鲜期"
                  >
                    已过期
                  </span>
                </td>

                <!-- Row-Level Quick Actions -->
                <td class="text-right whitespace-normal probe-node-actions" data-label="快捷操作">
                  <div class="flex min-w-0 flex-wrap items-center gap-1.5">
                    <button
                      type="button"
                      data-testid="row-reprobe-btn"
                      class="btn btn-xs btn-primary btn-outline gap-1 probe-node-action-button"
                      :disabled="getNodeProbeState(node) === 'probing' || submitting"
                      title="手动检测将插在节点池队列最前面优先执行"
                      @click="handleProbeSingleNode(node)"
                    >
                      <BoltIcon
                        class="w-3.5 h-3.5"
                        :class="{ 'animate-spin': getNodeProbeState(node) === 'probing' }"
                      />
                      <span>
                        {{
                          getNodeProbeState(node) === 'probing'
                            ? '检测中'
                            : getNodeProbeState(node) === 'queued'
                            ? '队列等待'
                            : '立即重测'
                        }}
                      </span>
                    </button>
                    <button
                      type="button"
                      data-testid="row-inspect-btn"
                      class="btn btn-xs btn-ghost probe-node-action-button"
                      @click="handleInspectNode(node)"
                    >
                      {{ t('probes.viewEvidence') }}
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Collapsible Recent Manual Tasks Summary in Workbench -->
      <div
        v-if="runs.length > 0"
        class="card bg-base-200/70 border border-base-300 p-4 space-y-3"
      >
        <div class="flex items-center justify-between">
          <button
            type="button"
            class="flex items-center gap-2 text-xs sm:text-sm font-bold"
            @click="historyCollapsed = !historyCollapsed"
          >
            <ClockIcon class="w-4 h-4 text-primary" />
            <span>最近测速任务记录 ({{ runs.length }})</span>
            <ChevronUpIcon v-if="!historyCollapsed" class="w-4 h-4 opacity-60" />
            <ChevronDownIcon v-else class="w-4 h-4 opacity-60" />
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs"
            @click="activeTab = 'runs'"
          >
            查看全部记录
          </button>
        </div>

        <div v-if="!historyCollapsed" class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <ProbeRunCard
            v-for="run in runs.slice(0, 3)"
            :key="run.id"
            :run="run"
            :cancelling="cancelling"
            @inspect="handleInspectRun"
            @cancel="handleCancel"
          />
        </div>
      </div>
    </div>

    <!-- =====================================================================
         Tab 2: Manual Probe Tasks History (手动测速任务记录)
         ===================================================================== -->
    <div v-else-if="activeTab === 'runs'" class="space-y-4">
      <div class="flex items-center gap-2 overflow-x-auto pb-1 text-xs select-none">
        <FunnelIcon class="w-4 h-4 opacity-50 flex-shrink-0" />
        <button
          type="button"
          class="btn btn-xs rounded-lg font-medium touch-manipulation"
          :class="selectedStateFilter === '' ? 'btn-primary' : 'btn-ghost'"
          @click="handleFilterChange('')"
        >
          全部 ({{ totalRuns }})
        </button>
        <button
          v-for="st in (['running', 'queued', 'succeeded', 'failed', 'cancelled'] as ProbeRunState[])"
          :key="st"
          type="button"
          class="btn btn-xs rounded-lg font-medium touch-manipulation"
          :class="selectedStateFilter === st ? 'btn-primary' : 'btn-ghost'"
          @click="handleFilterChange(st)"
        >
          {{ probeStateLabel(st) }}
        </button>
      </div>

      <div v-if="loadingRuns && runs.length === 0" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <div v-for="i in 6" :key="i" class="skeleton h-32 rounded-box" />
      </div>

      <EmptyState
        v-else-if="runs.length === 0"
        :icon="BoltIcon"
        :title="t('probes.noRuns')"
        :description="t('probes.emptyDesc')"
        :action-label="t('probes.triggerRun')"
        @action="handleQuickFullProbe"
      >
        <template #action-icon>
          <PlayIcon class="w-4 h-4" />
        </template>
      </EmptyState>

      <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <ProbeRunCard
          v-for="run in runs"
          :key="run.id"
          :run="run"
          :cancelling="cancelling"
          @inspect="handleInspectRun"
          @cancel="handleCancel"
        />
      </div>
    </div>

    <!-- =====================================================================
         Tab 3: Periodic Schedule & Batches (定时策略与自动批次)
         ===================================================================== -->
    <div v-else class="space-y-5">
      <!-- Schedule Overview Card -->
      <div class="card bg-base-200 border border-base-300 shadow-sm p-4 sm:p-5">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <div class="flex items-center gap-2">
              <h3 class="font-bold text-base sm:text-lg">周期能力探测计划</h3>
              <StatusBadge
                v-if="schedule"
                :label="schedule.enabled ? '已启用' : '已停用'"
                :tone="schedule.enabled ? 'success' : 'info'"
              />
            </div>
            <p class="text-xs opacity-70 mt-1">
              按预设周期自动对所有活跃节点执行连通延迟与解锁能力巡检。
            </p>
          </div>
          <button
            type="button"
            data-testid="configure-schedule-btn"
            class="btn btn-primary btn-sm gap-2"
            @click="openScheduleModal"
          >
            <Cog6ToothIcon class="w-4 h-4" />
            {{ t('probes.configureSchedule') }}
          </button>
        </div>

        <div v-if="schedule" class="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4 pt-3 border-t border-base-300 text-xs">
          <div>
            <span class="opacity-60 block">巡检周期</span>
            <span class="font-semibold">
              {{ intervalLabel(schedule.interval_seconds) }} ({{ Math.round(schedule.interval_seconds / 60) }} 分钟)
            </span>
          </div>
          <div>
            <span class="opacity-60 block">自动检测项目</span>
            <div class="flex flex-wrap gap-1 mt-0.5">
              <span v-for="k in schedule.kinds" :key="k" class="badge badge-xs badge-ghost">
                {{ probeKindEmoji(k) }} {{ probeKindLabel(k) }}
              </span>
            </div>
          </div>
          <div>
            <span class="opacity-60 block">下次自动测速</span>
            <span class="font-mono font-semibold text-primary">
              {{ schedule.next_due_at ? new Date(schedule.next_due_at).toLocaleTimeString() : '暂无' }}
            </span>
          </div>
        </div>
      </div>

      <!-- Periodic Batches List -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <h4 class="font-bold text-sm tracking-wide uppercase opacity-80">
            自动定时测速批次 ({{ totalBatches }})
          </h4>
          <button
            type="button"
            class="btn btn-ghost btn-xs gap-1"
            :disabled="loadingBatches"
            @click="() => loadBatches()"
          >
            <ArrowPathIcon class="w-3.5 h-3.5" :class="{ 'animate-spin': loadingBatches }" />
            {{ t('common.refresh') }}
          </button>
        </div>

        <div v-if="loadingBatches && batches.length === 0" class="space-y-3">
          <div v-for="i in 3" :key="i" class="skeleton h-20 rounded-xl" />
        </div>

        <EmptyState
          v-else-if="batches.length === 0"
          :icon="ClockIcon"
          title="暂无自动定时测速批次"
          description="启用定时自动测速后，系统按周期自动执行的巡检批次将显示在此处。"
        />

        <div v-else class="space-y-3">
          <article
            v-for="batch in batches"
            :key="batch.id"
            data-testid="probe-batch-card"
            class="card bg-base-200 border border-base-300 p-4 rounded-xl flex flex-col gap-2 transition hover:border-primary/40"
          >
            <div class="flex flex-wrap items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <span class="font-bold text-xs sm:text-sm truncate">
                    {{ formatBatchTitle(batch) }}
                  </span>
                  <StatusBadge
                    :label="probeStateLabel(batch.state)"
                    :tone="probeBatchStateTone(batch.state)"
                  />
                </div>
                <p class="text-[11px] opacity-60 mt-0.5">
                  巡检时间窗口：{{ new Date(batch.window_at).toLocaleString() }}
                </p>
              </div>

              <div class="flex items-center gap-2 shrink-0">
                <button
                  v-if="batch.state === 'running' || batch.state === 'pending'"
                  type="button"
                  class="btn btn-ghost btn-xs text-error gap-1"
                  :disabled="cancellingBatch"
                  @click="cancelBatch(batch.id)"
                >
                  <StopIcon class="w-3.5 h-3.5" />
                  取消批次
                </button>
              </div>
            </div>

            <div class="flex flex-wrap items-center gap-4 text-xs pt-2 border-t border-base-300 opacity-80">
              <span>总节点数：<strong class="font-mono">{{ batch.counts.total_nodes }}</strong></span>
              <span>已派发：<strong class="font-mono text-primary">{{ batch.counts.dispatched_runs }}</strong></span>
              <span>已完成：<strong class="font-mono text-success">{{ batch.counts.completed_runs }}</strong></span>
              <span v-if="batch.counts.skipped_nodes > 0" class="text-warning">
                已跳过：<strong class="font-mono">{{ batch.counts.skipped_nodes }}</strong>
              </span>
            </div>

            <div v-if="batch.run_ids && batch.run_ids.length > 0" class="flex flex-wrap items-center gap-1.5 pt-1 text-[11px]">
              <span class="opacity-60">查看批次测速结果：</span>
              <button
                v-for="(rId, idx) in batch.run_ids"
                :key="rId"
                type="button"
                class="badge badge-xs badge-neutral hover:badge-primary cursor-pointer transition-colors"
                title="查看该批次测速结果"
                @click="handleJumpToRun(rId)"
              >
                子任务 #{{ idx + 1 }}
              </button>
            </div>

            <div v-if="batch.counts.total_nodes === 0" class="p-2 bg-info/10 border border-info/20 rounded-lg text-info text-xs">
              当前计划窗口内无可用于探测的活跃节点。
            </div>
            <div v-else-if="batch.counts.skipped_nodes > 0" class="p-2 bg-warning/10 border border-warning/20 rounded-lg text-warning text-xs">
              已跳过 {{ batch.counts.skipped_nodes }} 个凭据缺失或无效的节点（安全闭合保护）。
            </div>
            <div v-if="batch.state === 'expired'" class="p-2 bg-neutral/20 border border-base-300 rounded-lg text-xs opacity-75">
              批次时间窗口已超时结束。
            </div>

            <div v-if="batch.redacted_error" class="p-2 bg-error/10 border border-error/20 rounded-lg text-error text-xs font-mono">
              {{ batch.redacted_error }}
            </div>
          </article>
        </div>
      </div>
    </div>

    <!-- Configure Schedule Modal Dialog (Human-Friendly Preset Intervals) -->
    <ModalDialog
      v-model="scheduleModalOpen"
      title="配置周期探测计划"
      description="启用后台自动测速巡检并选择直观的巡检频率与检测项目"
    >
      <form class="space-y-4" @submit.prevent="submitSaveSchedule">
        <div class="form-control">
          <label class="label cursor-pointer justify-start gap-3">
            <input
              v-model="scheduleEnabled"
              data-testid="schedule-enable-toggle"
              type="checkbox"
              class="toggle toggle-primary"
            />
            <span class="label-text font-semibold">启用后台周期自动探测</span>
          </label>
        </div>

        <div class="space-y-2">
          <span class="label-text font-semibold text-xs block">选择自动测速频率</span>
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
            <button
              v-for="preset in SCHEDULE_PRESETS"
              :key="preset.seconds"
              type="button"
              :data-testid="`schedule-preset-${preset.seconds}`"
              class="btn btn-xs sm:btn-sm rounded-lg"
              :class="scheduleInterval === preset.seconds ? 'btn-primary' : 'btn-outline border-base-300'"
              @click="selectSchedulePreset(preset.seconds)"
            >
              {{ preset.label }}
            </button>
          </div>
          <label class="form-control pt-1">
            <span class="label-text text-xs opacity-75">自定义间隔（分钟）</span>
            <input
              v-model.number="customMinutes"
              data-testid="schedule-custom-minutes-input"
              type="number"
              min="1"
              max="10080"
              required
              class="input input-bordered input-sm font-mono mt-1"
              @input="onCustomMinutesInput"
            />
          </label>
        </div>

        <div>
          <span class="label-text font-semibold text-xs block mb-2">自动测速包含的检测维度</span>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <label
              v-for="item in ALL_PROBE_KINDS"
              :key="item.kind"
              class="flex items-start gap-2.5 p-2 rounded-lg border border-base-300 bg-base-200/50 cursor-pointer hover:border-primary/40 transition-colors"
            >
              <input
                v-model="scheduleKinds"
                type="checkbox"
                :value="item.kind"
                class="checkbox checkbox-primary checkbox-sm mt-0.5"
              />
              <span class="font-medium text-xs">{{ item.emoji }} {{ item.label }}</span>
            </label>
          </div>
        </div>

        <div class="modal-action border-t border-base-300 pt-3">
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            @click="scheduleModalOpen = false"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            data-testid="save-schedule-btn"
            class="btn btn-primary btn-sm gap-2"
            :class="{ loading: savingSchedule }"
            :disabled="savingSchedule || scheduleKinds.length === 0"
          >
            保存周期计划
          </button>
        </div>
      </form>
    </ModalDialog>

    <!-- Structured Human-Readable Evidence Detail Sheet -->
    <ProbeEvidenceSheet
      :open="evidenceSheetOpen"
      :run="activeRun"
      :node="selectedInspectNode"
      :node-map="nodeMap"
      :observations="sheetObservations"
      :loading="sheetLoading"
      :reprobing="selectedInspectNode ? getNodeProbeState(selectedInspectNode) === 'probing' : false"
      @close="evidenceSheetOpen = false"
      @reprobe="handleProbeSingleNode"
    />

    <!-- Cancel Confirmation Modal -->
    <ConfirmModal
      v-model="confirmCancelOpen"
      title="确认取消测速任务"
      message="确定要取消该测速任务吗？正在执行中的节点检测将立即终止。"
      confirm-text="取消测速"
      cancel-text="继续运行"
      tone="danger"
      :loading="cancelling"
      @confirm="handleConfirmCancel"
    />
  </section>
</template>

<style scoped>
.probe-node-identity .badge {
  height: auto;
  min-height: 1rem;
  padding-top: 0.125rem;
  padding-bottom: 0.125rem;
  line-height: 1.25;
}
/* Keep dense desktop tables usable without letting unbroken user data dictate column width. */
@media (min-width: 768px) {
  .probe-node-table { table-layout: fixed; }
  .probe-node-table th:nth-child(1), .probe-node-select { width: 5%; }
  .probe-node-table th:nth-child(2), .probe-node-identity { width: 23%; }
  .probe-node-table th:nth-child(3) { width: 14%; }
  .probe-node-table th:nth-child(4) { width: 26%; }
  .probe-node-table th:nth-child(5) { width: 12%; }
  .probe-node-table th:nth-child(6), .probe-node-actions { width: 20%; }
  .probe-node-actions > div { justify-content: flex-end; }
  .probe-node-action-button { max-width: 100%; white-space: normal; overflow-wrap: anywhere; }
  .probe-node-identity > div { min-width: 0; max-width: 100%; }
  .probe-node-identity span { min-width: 0; max-width: 100%; overflow-wrap: anywhere; }
}
@media (max-width: 767px) {
  .probe-node-table-wrap { overflow-x: clip; }
  .probe-node-table { display: block; width: 100%; min-width: 0; table-layout: fixed; }
  .probe-node-table thead { display: none; }
  .probe-node-table tbody { display: grid; gap: 0.75rem; }
  .probe-node-row {
    position: relative; display: grid; grid-template-columns: minmax(0, 1fr); gap: 0.5rem;
    padding: 0.75rem; border: 1px solid hsl(var(--bc) / 0.18); border-radius: 0.75rem;
    background: hsl(var(--b1));
  }
  .probe-node-row td { display: block; width: 100%; min-width: 0; max-width: 100%; padding: 0; white-space: normal; text-align: left; overflow-wrap: anywhere; }
  .probe-node-row td:not(.probe-node-select)::before {
    content: attr(data-label); display: block; margin-bottom: 0.2rem; font-size: 0.65rem;
    font-weight: 700; opacity: 0.65;
  }
  .probe-node-select { position: absolute; top: 0.75rem; right: 0.75rem; z-index: 1; }
  .probe-node-actions, .probe-node-actions > div { width: 100%; min-width: 0; max-width: 100%; }
  .probe-node-actions button { flex: 1 1 auto; min-width: 0; min-height: 2rem; white-space: normal; overflow-wrap: anywhere; }
  .probe-node-row td > div { min-width: 0; max-width: 100%; }
  .probe-node-row [data-testid="probe-node-status-badge"], .probe-node-row .badge { max-width: 100%; white-space: normal; overflow-wrap: anywhere; }
}
</style>

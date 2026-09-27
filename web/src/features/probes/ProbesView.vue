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
  resolveNodeLatencyMs,
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
} = useProbes()

const activeTab = ref<'workbench' | 'runs' | 'schedule'>('workbench')
const scheduleModalOpen = ref(false)
const evidenceSheetOpen = ref(false)
const selectedInspectNode = ref<NormalizedNode | null>(null)
const selectedStateFilter = ref<ProbeRunState | ''>('')
const historyCollapsed = ref(false)

// Probe Dimension Pills state (baseline always included by default)
const selectedKinds = ref<ProbeKind[]>(['baseline', 'streaming', 'ai', 'ip_risk', 'geo'])

// Workbench Filters & Sorting
const searchQuery = ref('')
const protocolFilter = ref('all')
const subscriptionFilter = ref('all')
const healthFilter = ref<'all' | 'healthy' | 'degraded' | 'unhealthy' | 'unprobed'>('all')
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

// ============================================================================
// Tier 1: KPI Summary Bar Computed Metrics
// ============================================================================
const kpiStats = computed(() => {
  const nodes = probeNodes.value
  const total = nodes.length
  let healthy = 0
  let degraded = 0
  let unhealthy = 0
  let unprobed = 0

  let latencySum = 0
  let latencyCount = 0
  let fastCount = 0 // < 100ms
  let mediumCount = 0 // 100 - 250ms
  let slowCount = 0 // > 250ms

  let streamingUnlocked = 0
  let aiUnlocked = 0
  let lowRiskCount = 0

  for (const node of nodes) {
    const badge = nodeHealthBadge(node)
    if (badge.label === '正常') healthy += 1
    else if (badge.label === '降级') degraded += 1
    else if (badge.label === '异常') unhealthy += 1
    else unprobed += 1

    const ms = resolveNodeLatencyMs(node)
    if (ms !== null && ms >= 0) {
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

  const availableCount = healthy + degraded
  const onlineRate = total > 0 ? Math.round((availableCount / total) * 100) : 0
  const avgLatency = latencyCount > 0 ? Math.round(latencySum / latencyCount) : null

  return {
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
  }
})

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
      const badge = nodeHealthBadge(node)
      if (health === 'healthy' && badge.label !== '正常') return false
      if (health === 'degraded' && badge.label !== '降级') return false
      if (health === 'unhealthy' && badge.label !== '异常') return false
      if (health === 'unprobed' && badge.label !== '未探测') return false
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
// One-Click Probing Actions (No config_revision modal!)
// ============================================================================
async function handleQuickFullProbe() {
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
  try {
    await triggerQuickProbe({
      kinds: selectedKinds.value,
      nodeLogicalIds: Array.from(selectedNodeIds.value),
    })
  } catch {
    // error recorded in useProbes
  }
}

async function handleProbeSingleNode(node: NormalizedNode) {
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
    const hasActive = runs.value.some((r) => r.state === 'running' || r.state === 'queued')
    if (hasActive) {
      loadRuns(selectedStateFilter.value || undefined)
      loadProbeNodes()
      if (activeRun.value && (activeRun.value.state === 'running' || activeRun.value.state === 'queued')) {
        loadObservations(activeRun.value.id)
      }
    }
  }, 4000)
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
            :class="{ 'animate-spin': loadingNodes || loadingRuns || loadingSchedule || loadingBatches }"
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
         Tier 1: KPI Summary Bar (全景节点健康与延迟统计看板)
         ===================================================================== -->
    <div
      data-testid="probe-kpi-bar"
      class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3.5"
    >
      <!-- KPI Card 1: Online Availability Rate -->
      <article
        data-testid="kpi-online-rate"
        class="card bg-base-200 border border-base-300 shadow-sm p-4 flex flex-col justify-between gap-2.5"
      >
        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold opacity-70">在线可用率</span>
          <CheckCircleIcon class="w-4 h-4 text-success" />
        </div>
        <div class="flex items-baseline justify-between gap-2">
          <div class="text-2xl font-extrabold font-mono">
            {{ kpiStats.availableCount }}
            <span class="text-sm font-normal opacity-60">/ {{ kpiStats.total }} 节点</span>
          </div>
          <span class="text-sm font-bold text-success font-mono">{{ kpiStats.onlineRate }}%</span>
        </div>
        <progress
          class="progress progress-success w-full h-1.5"
          :value="kpiStats.onlineRate"
          max="100"
        />
        <div class="flex flex-wrap items-center gap-1.5 text-[11px] opacity-80">
          <span class="badge badge-xs badge-success badge-outline">正常 {{ kpiStats.healthy }}</span>
          <span class="badge badge-xs badge-warning badge-outline">降级 {{ kpiStats.degraded }}</span>
          <span class="badge badge-xs badge-error badge-outline">异常 {{ kpiStats.unhealthy }}</span>
          <span class="badge badge-xs badge-ghost">未测 {{ kpiStats.unprobed }}</span>
        </div>
      </article>

      <!-- KPI Card 2: Average Response Latency -->
      <article
        data-testid="kpi-avg-latency"
        class="card bg-base-200 border border-base-300 shadow-sm p-4 flex flex-col justify-between gap-2.5"
      >
        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold opacity-70">平均响应延迟</span>
          <BoltIcon class="w-4 h-4 text-primary" />
        </div>
        <div class="flex items-baseline gap-2">
          <span class="text-2xl font-extrabold font-mono">
            {{ kpiStats.avgLatency !== null ? `${kpiStats.avgLatency} ms` : '未测速' }}
          </span>
          <span v-if="kpiStats.latencyCount > 0" class="text-xs opacity-60">
            ({{ kpiStats.latencyCount }} 个已测节点)
          </span>
        </div>
        <div class="flex flex-wrap items-center gap-1.5 text-[11px] opacity-85 pt-1">
          <span class="badge badge-xs badge-success badge-outline">
            极速 &lt;100ms: {{ kpiStats.fastCount }}
          </span>
          <span class="badge badge-xs badge-warning badge-outline">
            良好 100-250ms: {{ kpiStats.mediumCount }}
          </span>
          <span class="badge badge-xs badge-error badge-outline">
            较慢 &gt;250ms: {{ kpiStats.slowCount }}
          </span>
        </div>
      </article>

      <!-- KPI Card 3: Streaming & AI Unlock Matrix -->
      <article
        data-testid="kpi-unlock-stats"
        class="card bg-base-200 border border-base-300 shadow-sm p-4 flex flex-col justify-between gap-2.5"
      >
        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold opacity-70">流媒体与 AI 解锁</span>
          <SparklesIcon class="w-4 h-4 text-secondary" />
        </div>
        <div class="grid grid-cols-3 gap-2 pt-0.5">
          <div class="rounded-lg bg-base-300/50 p-2 text-center">
            <span class="text-[11px] opacity-65 block">🎬 流媒体</span>
            <strong class="text-base font-mono font-bold text-success">{{ kpiStats.streamingUnlocked }}</strong>
          </div>
          <div class="rounded-lg bg-base-300/50 p-2 text-center">
            <span class="text-[11px] opacity-65 block">🤖 AI 可用</span>
            <strong class="text-base font-mono font-bold text-primary">{{ kpiStats.aiUnlocked }}</strong>
          </div>
          <div class="rounded-lg bg-base-300/50 p-2 text-center">
            <span class="text-[11px] opacity-65 block">🛡️ 纯净 IP</span>
            <strong class="text-base font-mono font-bold text-info">{{ kpiStats.lowRiskCount }}</strong>
          </div>
        </div>
        <p class="text-[11px] opacity-65 truncate">
          覆盖 Netflix / YouTube / OpenAI / Claude 等核心服务
        </p>
      </article>

      <!-- KPI Card 4: Periodic Auto-Probe Status -->
      <article
        data-testid="kpi-schedule-status"
        class="card bg-base-200 border border-base-300 shadow-sm p-4 flex flex-col justify-between gap-2.5"
      >
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs font-semibold opacity-70">定时自动测速状态</span>
          <StatusBadge
            :label="schedule?.enabled ? '自动巡检开启' : '未启用'"
            :tone="schedule?.enabled ? 'success' : 'info'"
          />
        </div>
        <div class="flex items-center justify-between gap-2">
          <div>
            <div class="text-base font-bold">
              {{ schedule ? intervalLabel(schedule.interval_seconds) : '每 1 小时' }}
            </div>
            <p class="text-[11px] opacity-65">
              下次测速：{{
                schedule?.enabled && schedule?.next_due_at
                  ? new Date(schedule.next_due_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                  : '等待启用'
              }}
            </p>
          </div>
          <div class="flex items-center gap-1.5">
            <button
              type="button"
              data-testid="quick-toggle-schedule-btn"
              class="btn btn-xs"
              :class="schedule?.enabled ? 'btn-ghost text-warning' : 'btn-success btn-outline'"
              :disabled="savingSchedule"
              @click="handleQuickToggleSchedule"
            >
              {{ schedule?.enabled ? '暂停' : '开启' }}
            </button>
            <button
              type="button"
              data-testid="open-schedule-modal-btn"
              class="btn btn-xs btn-primary btn-outline"
              @click="openScheduleModal"
            >
              调整策略
            </button>
          </div>
        </div>
      </article>
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

        <!-- Real-Time Active Probe Progress Banner -->
        <div
          v-if="currentRunningRun || submitting"
          data-testid="active-probe-progress-banner"
          class="rounded-xl bg-primary/10 border border-primary/30 px-3.5 py-2.5 flex flex-wrap items-center justify-between gap-3 text-xs"
        >
          <div class="flex items-center gap-2.5 min-w-0 flex-1">
            <span class="loading loading-spinner loading-xs text-primary shrink-0" />
            <div class="min-w-0 flex-1">
              <div class="font-semibold text-primary flex items-center gap-2">
                <span>正在执行节点实时测速...</span>
                <span v-if="currentRunningRun" class="badge badge-xs badge-primary">
                  {{ probeStateLabel(currentRunningRun.state) }}
                </span>
              </div>
              <progress class="progress progress-primary w-full max-w-md h-1.5 mt-1" />
            </div>
          </div>
          <div v-if="currentRunningRun" class="flex items-center gap-2 shrink-0">
            <button
              type="button"
              class="btn btn-xs btn-ghost"
              @click="handleInspectRun(currentRunningRun)"
            >
              查看实时观测
            </button>
            <button
              type="button"
              data-testid="banner-cancel-run-btn"
              class="btn btn-xs btn-error btn-outline"
              :disabled="cancelling"
              @click="handleCancel(currentRunningRun.id)"
            >
              取消测速
            </button>
          </div>
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
            <option value="all">全部状态 ({{ probeNodes.length }})</option>
            <option value="healthy">正常 ({{ kpiStats.healthy }})</option>
            <option value="degraded">降级 ({{ kpiStats.degraded }})</option>
            <option value="unhealthy">异常 ({{ kpiStats.unhealthy }})</option>
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
        <div class="overflow-x-auto">
          <table class="table table-sm sm:table-md w-full align-middle">
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
                <th>节点名称 / 协议与入口</th>
                <th>连通状态与延迟</th>
                <th>流媒体 / AI / 地区 / 风险</th>
                <th>最后测速</th>
                <th class="text-right">快捷操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="node in filteredNodes"
                :key="node.logicalId"
                data-testid="probe-node-row"
                class="hover:bg-base-300/30 transition-colors"
              >
                <td>
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
                <td class="min-w-[200px]">
                  <div class="flex items-center gap-2">
                    <span class="font-semibold text-xs sm:text-sm truncate max-w-[240px]">
                      {{ node.displayName }}
                    </span>
                    <span class="badge badge-xs badge-outline font-mono uppercase">
                      {{ node.protocol }}
                    </span>
                  </div>
                  <div class="flex flex-wrap items-center gap-1.5 mt-1 text-[11px] font-mono opacity-70">
                    <span v-if="node.connection.server && node.connection.port">
                      {{ node.connection.server }}:{{ node.connection.port }}
                    </span>
                    <span
                      v-for="srcName in formatNodeSources(node)"
                      :key="srcName"
                      class="badge badge-xs badge-ghost font-sans"
                    >
                      {{ srcName }}
                    </span>
                  </div>
                </td>

                <!-- Connectivity Health & Semantic Colored Latency Badge -->
                <td class="whitespace-nowrap">
                  <div class="flex items-center gap-1.5">
                    <StatusBadge
                      :label="nodeHealthBadge(node).label"
                      :tone="nodeHealthBadge(node).tone"
                    />
                    <StatusBadge
                      data-testid="probe-node-latency-badge"
                      :label="formatNodeLatency(node)"
                      :tone="nodeLatencyTone(node)"
                    />
                  </div>
                </td>

                <!-- Capability Matrix Badges -->
                <td>
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
                <td class="whitespace-nowrap text-xs font-mono opacity-75">
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
                <td class="text-right whitespace-nowrap">
                  <div class="inline-flex items-center gap-1.5">
                    <button
                      type="button"
                      data-testid="row-reprobe-btn"
                      class="btn btn-xs btn-primary btn-outline gap-1"
                      :disabled="probingNodeIds.has(node.logicalId) || submitting"
                      @click="handleProbeSingleNode(node)"
                    >
                      <BoltIcon
                        class="w-3.5 h-3.5"
                        :class="{ 'animate-spin': probingNodeIds.has(node.logicalId) }"
                      />
                      <span>{{ probingNodeIds.has(node.logicalId) ? '测速中' : '立即重测' }}</span>
                    </button>
                    <button
                      type="button"
                      data-testid="row-inspect-btn"
                      class="btn btn-xs btn-ghost"
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
      :reprobing="selectedInspectNode ? probingNodeIds.has(selectedInspectNode.logicalId) : false"
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

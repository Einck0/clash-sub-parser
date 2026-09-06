<template>
  <div
    role="region"
    aria-label="定时质检调度状态"
    class="rounded-lg border border-border-subtle bg-surface-base p-3 text-xs font-mono w-full max-w-full overflow-hidden shadow-xs"
  >
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2.5">
      <!-- Left: Status Icon, State Badge, State Description -->
      <div class="flex flex-wrap items-center gap-2 min-w-0 flex-1">
        <component
          :is="stateIcon"
          class="h-4 w-4 shrink-0"
          :class="stateIconClass"
          aria-hidden="true"
        />

        <!-- State Badge -->
        <span
          class="rounded px-1.5 py-0.5 text-[10px] font-bold border shrink-0 uppercase tracking-wider"
          :class="stateBadgeClass"
        >
          {{ stateLabel }}
        </span>

        <!-- Description / Summary text -->
        <span class="text-text-main font-medium truncate" :title="descriptionText">
          {{ descriptionText }}
        </span>

        <!-- Extra metrics tag: Last Finished / Last Summary -->
        <span
          v-if="status?.last_summary && status.state !== 'running'"
          class="hidden md:inline-flex items-center gap-1.5 text-text-muted text-[11px] tabular-nums"
        >
          <span class="text-text-sub">·</span>
          <span>上次完成: {{ status.last_summary.total }} 节点</span>
          <span class="text-status-success">({{ status.last_summary.ok }} 正常)</span>
          <span v-if="status.last_summary.fail > 0" class="text-status-danger">({{ status.last_summary.fail }} 失败)</span>
        </span>
      </div>

      <!-- Right: Countdown / Active Progress & Manual Refresh -->
      <div class="flex items-center justify-between sm:justify-end gap-2.5 w-full sm:w-auto shrink-0 border-t sm:border-t-0 border-border-subtle/40 pt-2 sm:pt-0">
        <!-- Countdown or Running text -->
        <div class="flex items-center gap-1.5 tabular-nums">
          <Clock class="h-3.5 w-3.5 text-text-sub shrink-0" aria-hidden="true" />
          <span class="text-text-sub text-[11px]">下次预计:</span>
          <span
            class="font-semibold"
            :class="countdownClass"
          >
            {{ countdownDisplay }}
          </span>
        </div>

        <!-- Refresh Button -->
        <button
          type="button"
          class="inline-flex items-center justify-center h-8 w-8 sm:h-7 sm:w-7 rounded-md border border-border-subtle bg-surface-active hover:bg-surface-hover text-text-sub hover:text-text-main transition-colors duration-150 cursor-pointer min-h-[44px] min-w-[44px] sm:min-h-0 sm:min-w-0"
          :disabled="loading"
          :aria-label="loading ? '正在刷新调度状态' : '刷新定时调度状态'"
          @click="fetchStatus"
        >
          <RefreshCw
            class="h-3.5 w-3.5"
            :class="{ 'animate-spin': loading }"
            aria-hidden="true"
          />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  Clock,
  Activity,
  AlertCircle,
  PauseCircle,
  RefreshCw,
  Loader2,
} from 'lucide-vue-next'
import { getProbeStatus } from '../../api'
import type { ProbeScheduleStatus } from '../../api/types'
import { calculateRemainingSeconds, formatCountdown } from '../../views/nodeLedgerDomain'

const props = defineProps<{
  initialStatus?: ProbeScheduleStatus | null
}>()

const status = ref<ProbeScheduleStatus | null>(props.initialStatus || null)
const loading = ref(false)
const serverNowAtFetch = ref<number>(0)
const localNowAtFetch = ref<number>(0)
const remainingSeconds = ref<number | null>(null)

let statusTimer: number | null = null
let countdownTimer: number | null = null

const stateLabel = computed(() => {
  const s = status.value?.state
  switch (s) {
    case 'running':
      return '执行中'
    case 'waiting':
      return '就绪等待'
    case 'initializing':
      return '初始化'
    case 'failed':
      return '异常'
    case 'disabled':
    default:
      return '已停用'
  }
})

const stateBadgeClass = computed(() => {
  const s = status.value?.state
  switch (s) {
    case 'running':
      return 'bg-status-warning/15 text-status-warning border-status-warning/30'
    case 'waiting':
      return 'bg-status-success/15 text-status-success border-status-success/30'
    case 'initializing':
      return 'bg-status-info/15 text-status-info border-status-info/30'
    case 'failed':
      return 'bg-status-danger/15 text-status-danger border-status-danger/30'
    case 'disabled':
    default:
      return 'bg-surface-active text-text-sub border-border-subtle'
  }
})

const stateIcon = computed(() => {
  const s = status.value?.state
  switch (s) {
    case 'running':
      return Loader2
    case 'waiting':
      return Clock
    case 'initializing':
      return Activity
    case 'failed':
      return AlertCircle
    case 'disabled':
    default:
      return PauseCircle
  }
})

const stateIconClass = computed(() => {
  const s = status.value?.state
  switch (s) {
    case 'running':
      return 'text-status-warning animate-spin'
    case 'waiting':
      return 'text-status-success'
    case 'initializing':
      return 'text-status-info'
    case 'failed':
      return 'text-status-danger'
    case 'disabled':
    default:
      return 'text-text-sub'
  }
})

const descriptionText = computed(() => {
  if (!status.value) return '正在获取定时质检状态…'
  const s = status.value.state
  if (s === 'running') {
    return '后台定时质检正在执行全量节点深度检测…'
  }
  if (s === 'initializing') {
    return '后台调度器正在建立周期基准线…'
  }
  if (s === 'failed') {
    return `调度执行异常: ${status.value.last_error_code || '未知错误'}`
  }
  if (s === 'waiting') {
    const interval = status.value.interval_minutes
    return interval ? `周期检测: 每 ${interval} 分钟自动执行一次全量质检` : '周期检测就绪'
  }
  return '定时后台质检已停用（可通过系统设置开启）'
})

const countdownDisplay = computed(() => {
  if (!status.value) return '--:--'
  if (status.value.state === 'running') {
    return '进行中'
  }
  if (status.value.state === 'disabled' || status.value.state === 'initializing') {
    return '--'
  }
  if (remainingSeconds.value === null || remainingSeconds.value === undefined) {
    return '--:--'
  }
  return formatCountdown(remainingSeconds.value)
})

const countdownClass = computed(() => {
  if (status.value?.state === 'running') {
    return 'text-status-warning'
  }
  if (remainingSeconds.value !== null && remainingSeconds.value <= 60) {
    return 'text-status-warning'
  }
  return 'text-text-main'
})

function updateCountdown() {
  if (!status.value || status.value.state === 'running' || !status.value.next_expected_at) {
    remainingSeconds.value = null
    return
  }
  remainingSeconds.value = calculateRemainingSeconds(
    status.value.next_expected_at,
    serverNowAtFetch.value,
    localNowAtFetch.value,
    Date.now()
  )
}

async function fetchStatus() {
  if (loading.value) return
  loading.value = true
  try {
    const res = await getProbeStatus()
    const raw = res?.data
    if (raw && typeof raw === 'object' && !Array.isArray(raw) && raw.state) {
      status.value = raw as ProbeScheduleStatus
      serverNowAtFetch.value = Number(raw.server_now) || Math.floor(Date.now() / 1000)
      localNowAtFetch.value = Date.now()
    } else {
      status.value = {
        state: 'disabled',
        server_now: Math.floor(Date.now() / 1000),
        interval_minutes: null,
        next_expected_at: null,
        last_started_at: null,
        last_finished_at: null,
        last_summary: null,
        last_error_code: null,
      }
    }
    updateCountdown()
  } catch (_) {
    // Non-fatal, preserve current status
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchStatus()
  countdownTimer = window.setInterval(updateCountdown, 1000)
  statusTimer = window.setInterval(fetchStatus, 15000)
})

onUnmounted(() => {
  if (countdownTimer) clearInterval(countdownTimer)
  if (statusTimer) clearInterval(statusTimer)
})

defineExpose({
  refresh: fetchStatus,
  status,
})
</script>

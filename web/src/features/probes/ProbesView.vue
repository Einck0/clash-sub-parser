<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  PlayIcon,
  FunnelIcon,
  ClockIcon,
  Cog6ToothIcon,
  StopIcon,
} from '@heroicons/vue/24/outline'
import { useProbes } from './useProbes'
import {
  ALL_PROBE_KINDS,
  probeBatchStateTone,
  probeKindLabel,
  probeStateLabel,
  type ProbeKind,
  type ProbeRun,
  type ProbeRunState,
} from './probeTypes'
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
  schedule,
  batches,
  loadingRuns,
  loadingObservations,
  loadingSchedule,
  loadingBatches,
  savingSchedule,
  cancellingBatch,
  submitting,
  cancelling,
  error,
  totalRuns,
  totalBatches,
  loadRuns,
  createRun,
  cancelRun,
  loadObservations,
  fetchRunDetail,
  loadSchedule,
  updateSchedule,
  loadBatches,
  cancelBatch,
} = useProbes()

const createModalOpen = ref(false)
const scheduleModalOpen = ref(false)
const evidenceSheetOpen = ref(false)
const selectedStateFilter = ref<ProbeRunState | ''>('')
const activeTab = ref<'runs' | 'schedule'>('runs')

const selectedKinds = ref<ProbeKind[]>(['baseline', 'geo', 'streaming', 'ai', 'ip_risk'])
const deadlineMinutes = ref<number>(10)
const configRevision = ref<string>('')

// Periodic Schedule form state
const scheduleEnabled = ref(false)
const scheduleInterval = ref(3600)
const scheduleKinds = ref<ProbeKind[]>(['baseline'])

function openScheduleModal() {
  if (schedule.value) {
    scheduleEnabled.value = schedule.value.enabled
    scheduleInterval.value = schedule.value.interval_seconds
    scheduleKinds.value = [...schedule.value.kinds]
  }
  scheduleModalOpen.value = true
}

async function submitSaveSchedule() {
  try {
    await updateSchedule({
      enabled: scheduleEnabled.value,
      interval_seconds: Number(scheduleInterval.value),
      kinds: scheduleKinds.value,
    })
    scheduleModalOpen.value = false
  } catch {
    // error recorded in useProbes
  }
}

let pollTimer: ReturnType<typeof setInterval> | null = null

function openCreateModal() {
  createModalOpen.value = true
}

async function submitCreate() {
  try {
    await createRun({
      kinds: selectedKinds.value,
      deadline_minutes: deadlineMinutes.value,
      config_revision: configRevision.value,
    })
    createModalOpen.value = false
  } catch {
    // error handled in useProbes
  }
}

async function handleInspect(run: ProbeRun) {
  activeRun.value = run
  evidenceSheetOpen.value = true
  await loadObservations(run.id)
}

function handleJumpToRun(rId: string) {
  activeTab.value = 'runs'
  const matched = runs.value.find((r) => r.id === rId)
  if (matched) {
    handleInspect(matched)
  } else {
    fetchRunDetail(rId).then((r) => {
      if (r) handleInspect(r)
    })
  }
}

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

function selectAllKinds() {
  selectedKinds.value = ALL_PROBE_KINDS.map((k) => k.kind)
}

onMounted(() => {
  loadRuns()
  loadSchedule()
  loadBatches()
  pollTimer = setInterval(() => {
    const hasActive = runs.value.some((r) => r.state === 'running' || r.state === 'queued')
    if (hasActive) {
      loadRuns(selectedStateFilter.value || undefined)
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
      <div class="flex items-center gap-2">
        <button
          type="button"
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
          @click="() => { loadRuns(selectedStateFilter || undefined); loadSchedule(); loadBatches(); }"
        >
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': loadingRuns || loadingSchedule || loadingBatches }" />
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm gap-2 touch-manipulation shadow-sm"
          @click="openCreateModal"
        >
          <PlayIcon class="w-4 h-4" />
          {{ t('probes.triggerRun') }}
        </button>
      </div>
    </div>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loadingRuns || savingSchedule || cancellingBatch"
      @retry="() => { loadRuns(selectedStateFilter || undefined); loadSchedule(); loadBatches(); }"
    />

    <!-- Navigation Tabs: Manual Runs vs Periodic Schedule & Batches -->
    <div class="tabs tabs-boxed bg-base-200/60 p-1 rounded-xl inline-flex w-fit">
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

    <!-- Runs Tab View -->
    <div v-if="activeTab === 'runs'" class="space-y-4">
      <!-- Filter Tabs -->
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

      <!-- Runs Grid -->
      <div v-if="loadingRuns && runs.length === 0" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <div v-for="i in 6" :key="i" class="skeleton h-32 rounded-box" />
      </div>

      <EmptyState
        v-else-if="runs.length === 0"
        :icon="BoltIcon"
        :title="t('probes.noRuns')"
        :description="t('probes.emptyDesc')"
        :action-label="t('probes.triggerRun')"
        @action="openCreateModal"
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
          @inspect="handleInspect"
          @cancel="handleCancel"
        />
      </div>
    </div>

    <!-- Periodic Schedule & Batches Tab View -->
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
              按固定周期在所有活跃节点上自动调度协同探针任务。
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

        <div v-if="schedule" class="grid grid-cols-2 sm:grid-cols-4 gap-3 mt-4 pt-3 border-t border-base-300 text-xs">
          <div>
            <span class="opacity-60 block">执行间隔</span>
            <span class="font-mono font-semibold">{{ schedule.interval_seconds }}s ({{ Math.round(schedule.interval_seconds / 60) }} 分钟)</span>
          </div>
          <div>
            <span class="opacity-60 block">探测类型</span>
            <div class="flex flex-wrap gap-1 mt-0.5">
              <span v-for="k in schedule.kinds" :key="k" class="badge badge-xs badge-ghost">
                {{ probeKindLabel(k) }}
              </span>
            </div>
          </div>
          <div>
            <span class="opacity-60 block">下次执行</span>
            <span class="font-mono font-semibold text-primary">
              {{ schedule.next_due_at ? new Date(schedule.next_due_at).toLocaleTimeString() : '暂无' }}
            </span>
          </div>
          <div>
            <span class="opacity-60 block">计划代数</span>
            <span class="font-mono font-semibold">#{{ schedule.generation }}</span>
          </div>
        </div>
      </div>

      <!-- Periodic Batches List -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <h4 class="font-bold text-sm tracking-wide uppercase opacity-80">
            周期执行批次 ({{ totalBatches }})
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
          title="暂无周期执行批次"
          description="当周期探测计划触发执行时，执行批次将自动显示在此处。"
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
                  <span class="font-mono font-bold text-xs truncate">批次 {{ batch.id }}</span>
                  <StatusBadge
                    :label="probeStateLabel(batch.state)"
                    :tone="probeBatchStateTone(batch.state)"
                  />
                </div>
                <p class="text-[11px] opacity-60 font-mono mt-0.5">
                  时间窗口：{{ new Date(batch.window_at).toLocaleString() }} · 代数 #{{ batch.generation }} · 执行器：{{ batch.owner || 'csp-core' }}
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
              <span class="opacity-60">关联探针任务：</span>
              <button
                v-for="rId in batch.run_ids"
                :key="rId"
                type="button"
                class="badge badge-xs font-mono badge-neutral hover:badge-primary cursor-pointer transition-colors"
                title="跳转到关联探针任务"
                @click="handleJumpToRun(rId)"
              >
                {{ rId }}
              </button>
            </div>

            <div v-if="batch.counts.total_nodes === 0" class="p-2 bg-info/10 border border-info/20 rounded-lg text-info text-xs">
              当前计划窗口内无可用于探测的活跃节点。
            </div>
            <div v-else-if="batch.counts.skipped_nodes > 0" class="p-2 bg-warning/10 border border-warning/20 rounded-lg text-warning text-xs">
              已跳过 {{ batch.counts.skipped_nodes }} 个凭据缺失或无效的节点（安全闭合保护）。
            </div>
            <div v-if="batch.state === 'expired'" class="p-2 bg-neutral/20 border border-base-300 rounded-lg text-xs opacity-75">
              批次时间窗口已过期或租约在完成前失效。
            </div>

            <div v-if="batch.redacted_error" class="p-2 bg-error/10 border border-error/20 rounded-lg text-error text-xs font-mono">
              {{ batch.redacted_error }}
            </div>
          </article>
        </div>
      </div>
    </div>

    <!-- Trigger Modal Dialog -->
    <ModalDialog
      v-model="createModalOpen"
      :title="t('probes.runBatch')"
      description="选择需要执行的探测维度与运行参数（具备 24 小时幂等保护）"
    >
      <form class="space-y-4" @submit.prevent="submitCreate">
        <div>
          <div class="flex items-center justify-between mb-2">
            <span class="label-text font-semibold text-sm">{{ t('probes.kindsTitle') }}</span>
            <button
              type="button"
              class="btn btn-link btn-xs p-0 text-primary"
              @click="selectAllKinds"
            >
              全选
            </button>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <label
              v-for="item in ALL_PROBE_KINDS"
              :key="item.kind"
              class="flex items-start gap-2.5 p-2.5 rounded-lg border border-base-300 bg-base-200/50 cursor-pointer hover:border-primary/40 transition-colors"
            >
              <input
                v-model="selectedKinds"
                type="checkbox"
                :value="item.kind"
                class="checkbox checkbox-primary checkbox-sm mt-0.5"
              />
              <div class="min-w-0">
                <span class="font-medium text-xs block leading-tight">{{ item.label }}</span>
                <span class="text-[11px] opacity-60 block mt-0.5 leading-snug">{{ item.desc }}</span>
              </div>
            </label>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
          <label class="form-control">
            <span class="label-text text-xs font-semibold">{{ t('probes.deadlineTitle') }}</span>
            <input
              v-model.number="deadlineMinutes"
              type="number"
              min="1"
              max="60"
              required
              class="input input-bordered input-sm mt-1 font-mono"
            />
          </label>

          <label class="form-control">
            <span class="label-text text-xs font-semibold">{{ t('probes.configRevision') }}</span>
            <input
              v-model="configRevision"
              placeholder="留空则使用当前活跃版本"
              class="input input-bordered input-sm mt-1 font-mono text-xs"
            />
          </label>
        </div>

        <div class="modal-action border-t border-base-300 pt-3">
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            @click="createModalOpen = false"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            class="btn btn-primary btn-sm gap-2"
            :class="{ loading: submitting }"
            :disabled="submitting || selectedKinds.length === 0"
          >
            <PlayIcon class="w-4 h-4" />
            {{ t('probes.submitProbe') }}
          </button>
        </div>
      </form>
    </ModalDialog>

    <!-- Configure Schedule Modal Dialog -->
    <ModalDialog
      v-model="scheduleModalOpen"
      title="配置周期探测计划"
      description="启用后台自动能力检测并配置调度周期"
    >
      <form class="space-y-4" @submit.prevent="submitSaveSchedule">
        <div class="form-control">
          <label class="label cursor-pointer justify-start gap-3">
            <input
              v-model="scheduleEnabled"
              type="checkbox"
              class="toggle toggle-primary"
            />
            <span class="label-text font-semibold">启用后台周期自动探测</span>
          </label>
        </div>

        <div class="form-control">
          <label class="label">
            <span class="label-text font-semibold text-xs">调度间隔（秒）</span>
          </label>
          <input
            v-model.number="scheduleInterval"
            type="number"
            min="60"
            max="604800"
            required
            class="input input-bordered input-sm font-mono"
            placeholder="例如：3600（1 小时）"
          />
          <span class="text-[11px] opacity-60 mt-1">允许范围：60 秒至 604,800 秒（7 天）。</span>
        </div>

        <div>
          <span class="label-text font-semibold text-xs block mb-2">周期派发的探针类型</span>
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
              <span class="font-medium text-xs">{{ item.label }}</span>
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
            class="btn btn-primary btn-sm gap-2"
            :class="{ loading: savingSchedule }"
            :disabled="savingSchedule || scheduleKinds.length === 0"
          >
            保存周期计划
          </button>
        </div>
      </form>
    </ModalDialog>

    <!-- Evidence Detail Bottom Sheet -->
    <ProbeEvidenceSheet
      :open="evidenceSheetOpen"
      :run="activeRun"
      :observations="observations"
      :loading="loadingObservations"
      @close="evidenceSheetOpen = false"
    />

    <!-- Cancel Confirmation Modal -->
    <ConfirmModal
      v-model="confirmCancelOpen"
      title="确认取消探针任务"
      message="确定要取消该探针任务吗？正在执行中的能力检测将立即终止。"
      confirm-text="取消任务"
      cancel-text="继续运行"
      tone="danger"
      :loading="cancelling"
      @confirm="handleConfirmCancel"
    />
  </section>
</template>

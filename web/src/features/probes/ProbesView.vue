<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  PlayIcon,
  FunnelIcon,
  ClockIcon,
  Cog6ToothIcon,
  ExclamationCircleIcon,
  StopIcon,
} from '@heroicons/vue/24/outline'
import { useProbes } from './useProbes'
import {
  ALL_PROBE_KINDS,
  probeBatchStateTone,
  type ProbeBatch,
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
          {{ schedule.enabled ? 'Active' : 'Disabled' }}
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
          All ({{ totalRuns }})
        </button>
        <button
          v-for="st in (['running', 'queued', 'succeeded', 'failed', 'cancelled'] as ProbeRunState[])"
          :key="st"
          type="button"
          class="btn btn-xs rounded-lg uppercase font-medium touch-manipulation"
          :class="selectedStateFilter === st ? 'btn-primary' : 'btn-ghost'"
          @click="handleFilterChange(st)"
        >
          {{ st }}
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
        description="Trigger your first probe run to evaluate streaming, AI, geo and connectivity capabilities."
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
              <h3 class="font-bold text-base sm:text-lg">Periodic Capability Schedule</h3>
              <StatusBadge
                v-if="schedule"
                :label="schedule.enabled ? 'ENABLED' : 'DISABLED'"
                :tone="schedule.enabled ? 'success' : 'info'"
              />
            </div>
            <p class="text-xs opacity-70 mt-1">
              Automatically dispatches coordinated probe runs across all active nodes at fixed intervals.
            </p>
          </div>
          <button
            type="button"
            data-testid="configure-schedule-btn"
            class="btn btn-primary btn-sm gap-2"
            @click="openScheduleModal"
          >
            <Cog6ToothIcon class="w-4 h-4" />
            Configure Schedule
          </button>
        </div>

        <div v-if="schedule" class="grid grid-cols-2 sm:grid-cols-4 gap-3 mt-4 pt-3 border-t border-base-300 text-xs">
          <div>
            <span class="opacity-60 block">Interval</span>
            <span class="font-mono font-semibold">{{ schedule.interval_seconds }}s ({{ Math.round(schedule.interval_seconds / 60) }}m)</span>
          </div>
          <div>
            <span class="opacity-60 block">Kinds</span>
            <div class="flex flex-wrap gap-1 mt-0.5">
              <span v-for="k in schedule.kinds" :key="k" class="badge badge-xs font-mono uppercase badge-ghost">
                {{ k }}
              </span>
            </div>
          </div>
          <div>
            <span class="opacity-60 block">Next Due</span>
            <span class="font-mono font-semibold text-primary">
              {{ schedule.next_due_at ? new Date(schedule.next_due_at).toLocaleTimeString() : 'N/A' }}
            </span>
          </div>
          <div>
            <span class="opacity-60 block">Generation</span>
            <span class="font-mono font-semibold">#{{ schedule.generation }}</span>
          </div>
        </div>
      </div>

      <!-- Periodic Batches List -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <h4 class="font-bold text-sm tracking-wide uppercase opacity-80">
            Execution Batches ({{ totalBatches }})
          </h4>
          <button
            type="button"
            class="btn btn-ghost btn-xs gap-1"
            :disabled="loadingBatches"
            @click="() => loadBatches()"
          >
            <ArrowPathIcon class="w-3.5 h-3.5" :class="{ 'animate-spin': loadingBatches }" />
            Refresh
          </button>
        </div>

        <div v-if="loadingBatches && batches.length === 0" class="space-y-3">
          <div v-for="i in 3" :key="i" class="skeleton h-20 rounded-xl" />
        </div>

        <EmptyState
          v-else-if="batches.length === 0"
          :icon="ClockIcon"
          title="No execution batches yet"
          description="Batches will appear here automatically when the periodic schedule executes."
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
                  <span class="font-mono font-bold text-xs truncate">Batch {{ batch.id }}</span>
                  <StatusBadge
                    :label="batch.state.toUpperCase()"
                    :tone="probeBatchStateTone(batch.state)"
                  />
                </div>
                <p class="text-[11px] opacity-60 font-mono mt-0.5">
                  Window: {{ new Date(batch.window_at).toLocaleString() }} · Gen #{{ batch.generation }} · Owner: {{ batch.owner || 'csp-core' }}
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
                  Cancel Batch
                </button>
              </div>
            </div>

            <div class="flex flex-wrap items-center gap-4 text-xs pt-2 border-t border-base-300 opacity-80">
              <span>Total Nodes: <strong class="font-mono">{{ batch.counts.total_nodes }}</strong></span>
              <span>Dispatched: <strong class="font-mono text-primary">{{ batch.counts.dispatched_runs }}</strong></span>
              <span>Completed: <strong class="font-mono text-success">{{ batch.counts.completed_runs }}</strong></span>
              <span v-if="batch.counts.skipped_nodes > 0" class="text-warning">
                Skipped: <strong class="font-mono">{{ batch.counts.skipped_nodes }}</strong>
              </span>
            </div>

            <div v-if="batch.run_ids && batch.run_ids.length > 0" class="flex flex-wrap items-center gap-1.5 pt-1 text-[11px]">
              <span class="opacity-60">Associated Runs:</span>
              <button
                v-for="rId in batch.run_ids"
                :key="rId"
                type="button"
                class="badge badge-xs font-mono badge-neutral hover:badge-primary cursor-pointer transition-colors"
                title="Jump to associated run"
                @click="handleJumpToRun(rId)"
              >
                {{ rId }}
              </button>
            </div>

            <div v-if="batch.counts.total_nodes === 0" class="p-2 bg-info/10 border border-info/20 rounded-lg text-info text-xs">
              No active inventory nodes available for probing at scheduled window.
            </div>
            <div v-else-if="batch.counts.skipped_nodes > 0" class="p-2 bg-warning/10 border border-warning/20 rounded-lg text-warning text-xs">
              {{ batch.counts.skipped_nodes }} node(s) skipped due to missing or invalid credentials (fail-closed protection).
            </div>
            <div v-if="batch.state === 'expired'" class="p-2 bg-neutral/20 border border-base-300 rounded-lg text-xs opacity-75">
              Batch window lapsed or lease lost before completion.
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
      title="Trigger New Probe Run"
      description="Select capability kinds and execution parameters with 24h idempotency protection"
    >
      <form class="space-y-4" @submit.prevent="submitCreate">
        <div>
          <div class="flex items-center justify-between mb-2">
            <span class="label-text font-semibold text-sm">Probe Kinds</span>
            <button
              type="button"
              class="btn btn-link btn-xs p-0 text-primary"
              @click="selectAllKinds"
            >
              Select All
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
            <span class="label-text text-xs font-semibold">Deadline (minutes)</span>
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
            <span class="label-text text-xs font-semibold">Config Revision (optional)</span>
            <input
              v-model="configRevision"
              placeholder="Leave empty for active"
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
            Cancel
          </button>
          <button
            type="submit"
            class="btn btn-primary btn-sm gap-2"
            :class="{ loading: submitting }"
            :disabled="submitting || selectedKinds.length === 0"
          >
            <PlayIcon class="w-4 h-4" />
            Dispatch Run
          </button>
        </div>
      </form>
    </ModalDialog>

    <!-- Configure Schedule Modal Dialog -->
    <ModalDialog
      v-model="scheduleModalOpen"
      title="Configure Periodic Probe Schedule"
      description="Enable automated background capability checks and configure dispatch intervals"
    >
      <form class="space-y-4" @submit.prevent="submitSaveSchedule">
        <div class="form-control">
          <label class="label cursor-pointer justify-start gap-3">
            <input
              v-model="scheduleEnabled"
              type="checkbox"
              class="toggle toggle-primary"
            />
            <span class="label-text font-semibold">Enable Periodic Background Probes</span>
          </label>
        </div>

        <div class="form-control">
          <label class="label">
            <span class="label-text font-semibold text-xs">Interval (seconds)</span>
          </label>
          <input
            v-model.number="scheduleInterval"
            type="number"
            min="60"
            max="604800"
            required
            class="input input-bordered input-sm font-mono"
            placeholder="e.g. 3600 (1 hour)"
          />
          <span class="text-[11px] opacity-60 mt-1">Allowed range: 60s to 604,800s (7 days).</span>
        </div>

        <div>
          <span class="label-text font-semibold text-xs block mb-2">Probe Kinds to Dispatch</span>
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
            Cancel
          </button>
          <button
            type="submit"
            class="btn btn-primary btn-sm gap-2"
            :class="{ loading: savingSchedule }"
            :disabled="savingSchedule || scheduleKinds.length === 0"
          >
            Save Schedule
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
      title="Cancel Probe Run"
      message="Are you sure you want to cancel this probe run? In-flight capability checks will be terminated immediately."
      confirm-text="Cancel Run"
      cancel-text="Keep Running"
      tone="danger"
      :loading="cancelling"
      @confirm="handleConfirmCancel"
    />
  </section>
</template>

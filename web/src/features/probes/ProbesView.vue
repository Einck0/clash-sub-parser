<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  PlayIcon,
  FunnelIcon,
} from '@heroicons/vue/24/outline'
import { useProbes } from './useProbes'
import { ALL_PROBE_KINDS, type ProbeKind, type ProbeRun, type ProbeRunState } from './probeTypes'
import ProbeRunCard from './ProbeRunCard.vue'
import ProbeEvidenceSheet from './ProbeEvidenceSheet.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import ModalDialog from '../../ui/ModalDialog.vue'
import { t } from '../../locales'

const {
  runs,
  activeRun,
  observations,
  loadingRuns,
  loadingObservations,
  submitting,
  cancelling,
  error,
  totalRuns,
  loadRuns,
  createRun,
  cancelRun,
  loadObservations,
} = useProbes()

const createModalOpen = ref(false)
const evidenceSheetOpen = ref(false)
const selectedStateFilter = ref<ProbeRunState | ''>('')

const selectedKinds = ref<ProbeKind[]>(['baseline', 'geo', 'streaming', 'ai', 'ip_risk'])
const deadlineMinutes = ref<number>(10)
const configRevision = ref<string>('')

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
          class="btn btn-ghost btn-sm btn-square touch-manipulation"
          :title="t('common.refresh')"
          @click="loadRuns(selectedStateFilter || undefined)"
        >
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': loadingRuns }" />
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
      :retrying="loadingRuns"
      @retry="() => loadRuns(selectedStateFilter || undefined)"
    />

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

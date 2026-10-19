<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  ArrowPathIcon,
  BoltIcon,
  ClockIcon,
  XMarkIcon,
  XCircleIcon,
  CheckCircleIcon,
} from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import ProbeRunCard from '../probes/ProbeRunCard.vue'
import ProbeEvidenceSheet from '../probes/ProbeEvidenceSheet.vue'
import type { ProbeBatch, ProbeObservation, ProbeRun } from '../probes/probeTypes'
import { probeStateLabel, probeStateTone } from '../probes/probeTypes'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

const activeTab = ref<'runs' | 'batches'>('runs')
const runs = ref<ProbeRun[]>([])
const batches = ref<ProbeBatch[]>([])
const loading = ref(false)
const error = ref('')

const inspectingRun = ref<ProbeRun | null>(null)
const evidenceSheetOpen = ref(false)
const runObservations = ref<ProbeObservation[]>([])
const loadingEvidence = ref(false)

async function loadHistory() {
  loading.value = true
  error.value = ''
  try {
    const [runsRes, batchesRes] = await Promise.allSettled([
      api.get<{ items: ProbeRun[]; total: number }>('/api/v1/probes/runs', {
        params: { page: 1, page_size: 100 },
      }),
      api.get<{ items: ProbeBatch[]; total: number }>('/api/v1/probes/batches', {
        params: { page: 1, page_size: 100 },
      }),
    ])

    if (runsRes.status === 'fulfilled') {
      runs.value = Array.isArray(runsRes.value?.items) ? runsRes.value.items : []
    }
    if (batchesRes.status === 'fulfilled') {
      batches.value = Array.isArray(batchesRes.value?.items) ? batchesRes.value.items : []
    }
  } catch (err: any) {
    error.value = err?.message || '加载探测记录失败'
  } finally {
    loading.value = false
  }
}

async function inspectRun(run: ProbeRun) {
  inspectingRun.value = run
  evidenceSheetOpen.value = true
  loadingEvidence.value = true
  try {
    const res = await api.get<{ items: ProbeObservation[] }>(
      `/api/v1/probes/runs/${encodeURIComponent(run.id)}/observations`,
      { params: { page: 1, page_size: 100 } }
    )
    runObservations.value = Array.isArray(res?.items) ? res.items : []
  } catch {
    runObservations.value = []
  } finally {
    loadingEvidence.value = false
  }
}

async function cancelRun(runId: string) {
  try {
    await api.post(`/api/v1/probes/runs/${encodeURIComponent(runId)}/cancel`)
    toastStore.push({ message: '已取消测速任务', tone: 'info' })
    const found = runs.value.find((r) => r.id === runId)
    if (found) {
      found.state = 'cancelled'
    }
  } catch (err: any) {
    toastStore.push({ message: err?.message || '取消任务失败', tone: 'error' })
  }
}

async function cancelBatch(batchId: string) {
  try {
    await api.post(`/api/v1/probes/batches/${encodeURIComponent(batchId)}/cancel`)
    toastStore.push({ message: '已取消执行批次', tone: 'info' })
    const found = batches.value.find((b) => b.id === batchId)
    if (found) {
      found.state = 'cancelled'
    }
  } catch (err: any) {
    toastStore.push({ message: err?.message || '取消批次失败', tone: 'error' })
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      void loadHistory()
    }
  }
)
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    title="探针运行明细与有界批次历史"
    description="呈现最近有界 100 条探针任务执行记录与周期批次状态"
    width-class="max-w-3xl"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div data-testid="node-probe-history-drawer-content" class="space-y-4">
    <!-- Tab Controls & Refresh -->
    <div class="flex items-center justify-between gap-3 mb-4">
      <div class="tabs tabs-boxed bg-base-300/60 p-1 rounded-xl">
        <button
          type="button"
          data-testid="history-tab-runs"
          class="tab tab-sm font-semibold transition-all"
          :class="{ 'tab-active bg-primary text-primary-content shadow': activeTab === 'runs' }"
          @click="activeTab = 'runs'"
        >
          单次运行 ({{ runs.length }})
        </button>
        <button
          type="button"
          data-testid="history-tab-batches"
          class="tab tab-sm font-semibold transition-all"
          :class="{ 'tab-active bg-primary text-primary-content shadow': activeTab === 'batches' }"
          @click="activeTab = 'batches'"
        >
          巡检批次 ({{ batches.length }})
        </button>
      </div>

      <button
        type="button"
        class="btn btn-ghost btn-sm btn-square touch-manipulation"
        :title="t('common.refresh')"
        :disabled="loading"
        @click="loadHistory"
      >
        <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': loading }" />
      </button>
    </div>

    <!-- Error Alert -->
    <div v-if="error" class="alert alert-error text-xs mb-4">
      <span>{{ error }}</span>
    </div>

    <!-- Runs Tab -->
    <div v-if="activeTab === 'runs'" class="space-y-3" data-testid="runs-history-list">
      <div v-if="runs.length === 0 && !loading" class="text-xs text-base-content/50 py-8 text-center border border-dashed border-base-300 rounded-xl">
        暂无测速运行记录
      </div>
      <div v-else class="space-y-3">
        <ProbeRunCard
          v-for="run in runs"
          :key="run.id"
          :run="run"
          @inspect="inspectRun"
          @cancel="cancelRun"
        />
      </div>
    </div>

    <!-- Batches Tab -->
    <div v-else-if="activeTab === 'batches'" class="space-y-3" data-testid="batches-history-list">
      <div v-if="batches.length === 0 && !loading" class="text-xs text-base-content/50 py-8 text-center border border-dashed border-base-300 rounded-xl">
        暂无巡检批次记录
      </div>
      <div v-else class="space-y-3">
        <article
          v-for="batch in batches"
          :key="batch.id"
          class="card border border-base-300 bg-base-200/70 p-4 rounded-xl text-xs space-y-2 hover:border-primary/40 transition"
          data-testid="probe-batch-item"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <span class="font-mono font-semibold">{{ batch.id }}</span>
              <StatusBadge
                :label="batch.state"
                :tone="batch.state === 'running' ? 'info' : batch.state === 'succeeded' ? 'success' : batch.state === 'failed' ? 'error' : undefined"
              />
            </div>
            <button
              v-if="batch.state === 'running' || batch.state === 'pending'"
              type="button"
              data-testid="cancel-batch-btn"
              class="btn btn-ghost btn-xs text-error"
              @click="cancelBatch(batch.id)"
            >
              取消批次
            </button>
          </div>

          <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-[11px] font-mono text-base-content/70">
            <span>总节点: {{ batch.counts?.total_nodes ?? 0 }}</span>
            <span>已派发: {{ batch.counts?.dispatched_runs ?? 0 }}</span>
            <span>已完成: {{ batch.counts?.completed_runs ?? 0 }}</span>
            <span>已跳过: {{ batch.counts?.skipped_nodes ?? 0 }}</span>
          </div>

          <div v-if="batch.redacted_error" class="text-[11px] text-error font-mono">
            {{ batch.redacted_error }}
          </div>
        </article>
      </div>
    </div>

    <!-- Evidence Sheet Modal -->
    <ProbeEvidenceSheet
      :open="evidenceSheetOpen"
      :run="inspectingRun"
      :observations="runObservations"
      :loading="loadingEvidence"
      @close="evidenceSheetOpen = false"
    />
    </div>
  </DrawerCard>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ArrowPathIcon, ChevronLeftIcon, ChevronRightIcon } from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import ProbeRunCard from '../probes/ProbeRunCard.vue'
import ProbeEvidenceSheet from '../probes/ProbeEvidenceSheet.vue'
import { useProbeHistory } from '../probes/useProbeHistory'
import { probeStateLabel, probeBatchStateTone } from '../probes/probeTypes'
import { displayText } from '../../utils/displayNames'
import { uiText } from '../../locales'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
const history = reactive(useProbeHistory())
const activeTab = ref<'runs' | 'batches'>('runs')
const activePage = computed(() => activeTab.value === 'runs' ? history.runs : history.batches)
const maxPage = computed(() => Math.max(1, Math.ceil(activePage.value.total / activePage.value.pageSize)))

watch(() => props.modelValue, (open) => {
  if (open) void history.refreshHistory()
  else history.closeEvidence()
}, { immediate: true })

function loadPage(page: number) {
  void (activeTab.value === 'runs' ? history.loadRuns(page) : history.loadBatches(page))
}
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="uiText('检测历史', 'Probe history')"
    :description="uiText('查看单次检测、周期巡检批次及原始检测证据', 'Review individual probe runs, scheduled batches, and their evidence')"
    width-class="max-w-3xl"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div data-testid="node-probe-history-drawer-content" class="space-y-4">
      <div class="flex items-center justify-between gap-3">
        <div role="group" :aria-label="uiText('历史类型', 'History type')" class="tabs tabs-box bg-base-300/60 p-1">
          <button type="button" data-testid="history-tab-runs" class="tab tab-sm" :class="{ 'tab-active': activeTab === 'runs' }" :aria-pressed="activeTab === 'runs'" @click="activeTab = 'runs'">
            {{ uiText('单次检测', 'Runs') }} <span class="ml-1 tabular-nums">{{ history.runs.loaded ? history.runs.total : '—' }}</span>
          </button>
          <button type="button" data-testid="history-tab-batches" class="tab tab-sm" :class="{ 'tab-active': activeTab === 'batches' }" :aria-pressed="activeTab === 'batches'" @click="activeTab = 'batches'">
            {{ uiText('巡检批次', 'Batches') }} <span class="ml-1 tabular-nums">{{ history.batches.loaded ? history.batches.total : '—' }}</span>
          </button>
        </div>
        <button type="button" class="btn btn-ghost btn-sm btn-square" :aria-label="uiText('刷新历史', 'Refresh history')" :disabled="activePage.loading" @click="loadPage(activePage.requestedPage)">
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': activePage.loading }" />
        </button>
      </div>

      <div v-if="activePage.error" role="alert" class="alert alert-error items-start text-sm">
        <div class="min-w-0 flex-1 whitespace-pre-wrap break-words">{{ displayText(activePage.error) }}</div>
        <button type="button" class="btn btn-xs" :disabled="activePage.loading" @click="loadPage(activePage.requestedPage)">{{ uiText('重试', 'Retry') }}</button>
      </div>
      <p v-if="activePage.stale" class="text-xs text-warning">{{ uiText('刷新失败或仍在刷新；以下为上次成功载入的数据。', 'Refreshing or refresh failed; showing the last successfully loaded data.') }}</p>
      <div v-if="activePage.loading && !activePage.loaded" class="py-10 text-center text-sm opacity-70" role="status">{{ uiText('正在载入…', 'Loading…') }}</div>

      <template v-else-if="activeTab === 'runs'">
        <div v-if="history.runs.loaded && history.runs.items.length === 0 && !history.runs.error" class="py-10 text-center text-sm opacity-60">{{ uiText('暂无检测运行记录', 'No probe runs') }}</div>
        <div class="space-y-3">
          <ProbeRunCard v-for="run in history.runs.items" :key="run.id" :run="run" :cancelling="history.cancellingRuns.has(run.id)" @inspect="history.inspectRun" @cancel="history.cancelRun" />
        </div>
      </template>

      <template v-else>
        <div v-if="history.batches.loaded && history.batches.items.length === 0 && !history.batches.error" class="py-10 text-center text-sm opacity-60">{{ uiText('暂无巡检批次记录', 'No inspection batches') }}</div>
        <div class="space-y-3">
          <article v-for="batch in history.batches.items" :key="batch.id" data-testid="probe-batch-item" class="border border-base-300 bg-base-200/70 p-4 space-y-3">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="flex flex-wrap items-center gap-2 min-w-0">
                <span class="font-semibold">{{ uiText('巡检批次', 'Inspection batch') }} · {{ new Date(batch.window_at).toLocaleString() }}</span>
                <StatusBadge :label="probeStateLabel(batch.state)" :tone="probeBatchStateTone(batch.state)" />
              </div>
              <button v-if="batch.state === 'running' || batch.state === 'pending'" type="button" data-testid="cancel-batch-btn" class="btn btn-ghost btn-xs text-error" :disabled="history.cancellingBatches.has(batch.id)" @click="history.cancelBatch(batch.id)">
                {{ history.cancellingBatches.has(batch.id) ? uiText('正在取消…', 'Cancelling…') : uiText('取消批次', 'Cancel batch') }}
              </button>
            </div>
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs tabular-nums">
              <span>{{ uiText('总节点', 'Nodes') }}: {{ batch.counts?.total_nodes ?? 0 }}</span>
              <span>{{ uiText('已派发', 'Dispatched') }}: {{ batch.counts?.dispatched_runs ?? 0 }}</span>
              <span>{{ uiText('已完成', 'Completed') }}: {{ batch.counts?.completed_runs ?? 0 }}</span>
              <span>{{ uiText('已跳过', 'Skipped') }}: {{ batch.counts?.skipped_nodes ?? 0 }}</span>
            </div>
            <p v-if="batch.redacted_error" class="text-xs text-error whitespace-pre-wrap break-words">{{ displayText(batch.redacted_error) }}</p>
          </article>
        </div>
      </template>

      <nav v-if="activePage.loaded && activePage.total > activePage.pageSize" class="flex items-center justify-between border-t border-base-300 pt-3" :aria-label="uiText('历史分页', 'History pagination')">
        <span class="text-xs tabular-nums">{{ (activePage.page - 1) * activePage.pageSize + 1 }}–{{ Math.min(activePage.page * activePage.pageSize, activePage.total) }} / {{ activePage.total }}</span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-ghost btn-sm btn-square" :aria-label="uiText('上一页', 'Previous page')" :disabled="activePage.page <= 1 || activePage.loading" @click="loadPage(activePage.page - 1)"><ChevronLeftIcon class="w-4 h-4" /></button>
          <span class="text-xs tabular-nums">{{ activePage.page }} / {{ maxPage }}</span>
          <button type="button" class="btn btn-ghost btn-sm btn-square" :aria-label="uiText('下一页', 'Next page')" :disabled="activePage.page >= maxPage || activePage.loading" @click="loadPage(activePage.page + 1)"><ChevronRightIcon class="w-4 h-4" /></button>
        </div>
      </nav>

      <ProbeEvidenceSheet :open="history.evidenceOpen" :run="history.inspectingRun" :observations="history.observations.items" :loading="history.observations.loading" :error="history.observations.error" :stale="history.observations.stale" :total="history.observations.total" :page="history.observations.page" :page-size="history.observations.pageSize" :retry-page="history.observations.requestedPage" @page="history.loadObservations" @close="history.closeEvidence" />
    </div>
  </DrawerCard>
</template>

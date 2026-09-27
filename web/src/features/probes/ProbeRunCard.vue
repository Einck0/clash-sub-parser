<script setup lang="ts">
import { computed } from 'vue'
import {
  BoltIcon,
  ClockIcon,
  XCircleIcon,
  ChevronRightIcon,
} from '@heroicons/vue/24/outline'
import { probeStateLabel, type ProbeRun } from './probeTypes'
import StatusBadge from '../../ui/StatusBadge.vue'
import { t } from '../../locales'

const props = defineProps<{
  run: ProbeRun
  cancelling?: boolean
}>()

const emit = defineEmits<{
  (e: 'inspect', run: ProbeRun): void
  (e: 'cancel', runId: string): void
}>()

const isTerminal = computed(() => {
  return ['succeeded', 'failed', 'cancelled', 'expired'].includes(props.run.state)
})

const formattedCreatedAt = computed(() => {
  try {
    const d = new Date(props.run.created_at)
    if (Number.isNaN(d.getTime())) return props.run.created_at
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return props.run.created_at
  }
})

const durationLabel = computed(() => {
  try {
    const start = new Date(props.run.created_at).getTime()
    const end = new Date(props.run.updated_at || props.run.created_at).getTime()
    if (Number.isNaN(start) || Number.isNaN(end) || end < start) return ''
    const sec = Math.max(1, Math.round((end - start) / 1000))
    if (!isTerminal.value) return '正在执行测速'
    return `耗时 ${sec}s`
  } catch {
    return ''
  }
})
</script>

<template>
  <article
    data-testid="probe-run-card"
    class="card border border-base-300 bg-base-200 shadow-sm transition-all duration-200 ease-out hover:border-primary/50 hover:shadow-md active:scale-[0.99] cursor-pointer"
    @click="emit('inspect', run)"
  >
    <div class="card-body p-4 sm:p-5 gap-3">
      <div class="flex items-start justify-between gap-2">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <div
              class="w-7 h-7 rounded-lg flex items-center justify-center text-xs shrink-0"
              :class="{
                'bg-primary/10 text-primary': run.state === 'running',
                'bg-warning/10 text-warning': run.state === 'queued',
                'bg-success/10 text-success': run.state === 'succeeded',
                'bg-error/10 text-error': run.state === 'failed',
                'bg-base-300 text-base-content/60': isTerminal && run.state !== 'succeeded' && run.state !== 'failed'
              }"
            >
              <BoltIcon class="w-4 h-4" :class="{ 'animate-pulse': run.state === 'running' }" />
            </div>
            <h3 class="text-xs sm:text-sm font-semibold truncate">
              手动测速任务 · {{ formattedCreatedAt }}
            </h3>
          </div>
          <p class="mt-1 text-xs opacity-65 truncate">
            主动探测队列 · {{ durationLabel || probeStateLabel(run.state) }}
          </p>
        </div>
        <StatusBadge
          :label="probeStateLabel(run.state)"
          :tone="run.state === 'succeeded' ? 'success' : run.state === 'failed' ? 'error' : run.state === 'queued' ? 'warning' : 'info'"
        />
      </div>

      <div class="flex items-center justify-between border-t border-base-300 pt-3 text-xs">
        <div class="flex items-center gap-1.5 opacity-60">
          <ClockIcon class="w-3.5 h-3.5" />
          <span>{{ formattedCreatedAt }}</span>
        </div>

        <div class="flex items-center gap-2" @click.stop>
          <button
            v-if="!isTerminal"
            type="button"
            data-testid="cancel-run-btn"
            class="btn btn-xs btn-ghost text-error gap-1"
            :disabled="cancelling"
            @click="emit('cancel', run.id)"
          >
            <XCircleIcon class="w-3.5 h-3.5" />
            {{ t('probes.cancelRun') }}
          </button>
          <button
            type="button"
            data-testid="inspect-run-btn"
            class="btn btn-xs btn-primary btn-outline gap-1"
            @click="emit('inspect', run)"
          >
            <span>{{ t('probes.viewEvidence') }}</span>
            <ChevronRightIcon class="w-3 h-3" />
          </button>
        </div>
      </div>
    </div>
  </article>
</template>

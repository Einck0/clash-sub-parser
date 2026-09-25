<script setup lang="ts">
import { computed } from 'vue'
import {
  BoltIcon,
  ClockIcon,
  XCircleIcon,
  ChevronRightIcon,
} from '@heroicons/vue/24/outline'
import type { ProbeRun } from './probeTypes'
import StatusBadge from '../../ui/StatusBadge.vue'

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
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return props.run.created_at
  }
})
</script>

<template>
  <article
    class="card border border-base-300 bg-base-200 shadow-sm transition-all duration-200 ease-out hover:border-primary/50 hover:shadow-md active:scale-[0.99] cursor-pointer"
    @click="emit('inspect', run)"
  >
    <div class="card-body p-4 sm:p-5 gap-3">
      <div class="flex items-start justify-between gap-2">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <div
              class="w-7 h-7 rounded-lg flex items-center justify-center text-xs"
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
            <h3 class="font-mono text-xs sm:text-sm font-semibold truncate">
              {{ run.id }}
            </h3>
          </div>
          <p class="mt-1 font-mono text-[11px] opacity-60 truncate">
            Scope: {{ run.actor_scope }} · Rev: {{ run.config_revision || 'active' }}
          </p>
        </div>
        <StatusBadge
          :label="run.state.toUpperCase()"
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
            class="btn btn-xs btn-ghost text-error gap-1"
            :disabled="cancelling"
            @click="emit('cancel', run.id)"
          >
            <XCircleIcon class="w-3.5 h-3.5" />
            Cancel
          </button>
          <button
            type="button"
            class="btn btn-xs btn-primary btn-outline gap-1"
            @click="emit('inspect', run)"
          >
            <span>Evidence</span>
            <ChevronRightIcon class="w-3 h-3" />
          </button>
        </div>
      </div>
    </div>
  </article>
</template>

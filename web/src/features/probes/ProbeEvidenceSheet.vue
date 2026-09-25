<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  XMarkIcon,
  CheckCircleIcon,
  ExclamationCircleIcon,
  ClockIcon,
  InformationCircleIcon,
  FingerPrintIcon,
  BoltIcon,
} from '@heroicons/vue/24/outline'
import type { ProbeObservation, ProbeRun } from './probeTypes'
import { formatLatency, probeVerdictTone } from './probeTypes'
import StatusBadge from '../../ui/StatusBadge.vue'

const props = defineProps<{
  open: boolean
  run: ProbeRun | null
  observations: ProbeObservation[]
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const selectedKind = ref<string>('all')

const filteredObservations = computed(() => {
  if (selectedKind.value === 'all') return props.observations
  return props.observations.filter((obs) => obs.kind === selectedKind.value)
})

const kinds = computed(() => {
  const set = new Set<string>()
  props.observations.forEach((obs) => set.add(obs.kind))
  return Array.from(set)
})

function close() {
  emit('close')
}
</script>

<template>
  <!-- 40% Soft Backdrop Blur Overlay -->
  <Transition
    enter-active-class="transition-opacity duration-200 ease-out"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition-opacity duration-150 ease-in"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0"
  >
    <div
      v-if="open"
      class="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
      @click="close"
    />
  </Transition>

  <!-- Bottom Sheet (Mobile) & Modal / Drawer (Desktop) -->
  <Transition
    enter-active-class="transition-all duration-200 ease-out"
    enter-from-class="translate-y-full md:translate-y-0 md:opacity-0 md:scale-95"
    enter-to-class="translate-y-0 md:opacity-100 md:scale-100"
    leave-active-class="transition-all duration-150 ease-in"
    leave-from-class="translate-y-0 md:opacity-100 md:scale-100"
    leave-to-class="translate-y-full md:translate-y-0 md:opacity-0 md:scale-95"
  >
    <section
      v-if="open"
      role="dialog"
      aria-modal="true"
      aria-labelledby="sheet-title"
      class="fixed bottom-0 left-0 right-0 z-50 adaptive-surface-sheet md:max-h-[82vh] md:bottom-auto md:top-1/2 md:left-1/2 md:-translate-x-1/2 md:-translate-y-1/2 md:w-full md:max-w-2xl bg-base-100 rounded-t-2xl md:rounded-2xl border-t md:border border-base-300 shadow-2xl flex flex-col overflow-hidden"
    >
      <!-- Grab Handle for Mobile Touch -->
      <div class="md:hidden pt-3 pb-1 flex justify-center flex-shrink-0 cursor-grab">
        <div class="w-12 h-1.5 rounded-full bg-base-content/20" />
      </div>

      <!-- Sheet Header -->
      <header class="flex items-start justify-between p-4 sm:p-5 border-b border-base-300 flex-shrink-0">
        <div class="min-w-0 pr-2">
          <div class="flex items-center gap-2">
            <span class="text-xs font-semibold uppercase tracking-wider text-primary">Evidence Inspector</span>
            <StatusBadge
              v-if="run"
              :label="run.state.toUpperCase()"
              :tone="run.state === 'succeeded' ? 'success' : run.state === 'failed' ? 'error' : run.state === 'queued' ? 'warning' : 'info'"
            />
          </div>
          <h2 id="sheet-title" class="mt-1 text-lg sm:text-xl font-bold truncate">
            Probe Run Evidence
          </h2>
          <p v-if="run" class="mt-0.5 font-mono text-xs opacity-60 truncate">
            Run ID: {{ run.id }}
          </p>
        </div>
        <button
          type="button"
          class="btn btn-ghost btn-sm btn-circle"
          aria-label="Close sheet"
          @click="close"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </header>

      <!-- Kind Filter Bar -->
      <div v-if="kinds.length > 1" class="px-4 py-2 bg-base-200/50 border-b border-base-300 flex items-center gap-1.5 overflow-x-auto text-xs flex-shrink-0">
        <span class="opacity-60 mr-1">Filter:</span>
        <button
          type="button"
          class="btn btn-xs rounded-lg"
          :class="selectedKind === 'all' ? 'btn-primary' : 'btn-ghost'"
          @click="selectedKind = 'all'"
        >
          All ({{ observations.length }})
        </button>
        <button
          v-for="kind in kinds"
          :key="kind"
          type="button"
          class="btn btn-xs rounded-lg uppercase"
          :class="selectedKind === kind ? 'btn-primary' : 'btn-ghost'"
          @click="selectedKind = kind"
        >
          {{ kind }}
        </button>
      </div>

      <!-- Observations Content -->
      <div class="flex-1 p-4 sm:p-5 overflow-y-auto space-y-3">
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 3" :key="i" class="skeleton h-24 rounded-xl" />
        </div>

        <div
          v-else-if="filteredObservations.length === 0"
          class="rounded-xl border border-dashed border-base-300 p-8 text-center"
        >
          <InformationCircleIcon class="w-8 h-8 mx-auto opacity-40 text-info" />
          <p class="mt-2 font-medium text-sm">No observations recorded</p>
          <p class="mt-1 text-xs opacity-60">Run may still be queued or no nodes were matched.</p>
        </div>

        <article
          v-for="obs in filteredObservations"
          :key="obs.id"
          class="card bg-base-200/80 border border-base-300/80 rounded-xl p-3.5 transition-all duration-200 ease-out hover:border-primary/40"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="badge badge-sm font-semibold uppercase tracking-wider badge-ghost">
                  {{ obs.kind }}
                </span>
                <span class="font-mono text-xs font-medium truncate">
                  {{ obs.node_logical_id }}
                </span>
              </div>
              <p class="mt-1.5 text-xs text-base-content/80 leading-relaxed font-sans">
                {{ obs.redacted_summary || 'Evidence recorded.' }}
              </p>
            </div>
            <StatusBadge
              :label="obs.verdict.toUpperCase()"
              :tone="probeVerdictTone(obs.verdict) === 'error' ? 'error' : probeVerdictTone(obs.verdict) === 'success' ? 'success' : probeVerdictTone(obs.verdict) === 'warning' ? 'warning' : 'info'"
            />
          </div>

          <div class="mt-3 pt-2.5 border-t border-base-300/50 flex flex-wrap items-center justify-between gap-2 text-[11px] opacity-70">
            <span class="flex items-center gap-1">
              <ClockIcon class="w-3.5 h-3.5" />
              Latency: <strong class="font-mono font-semibold">{{ formatLatency(obs.latency_ms) }}</strong>
            </span>
            <span v-if="obs.evidence_digest" class="font-mono truncate max-w-[200px]" title="Evidence Digest">
              {{ obs.evidence_digest }}
            </span>
          </div>
        </article>
      </div>

      <!-- Sheet Footer -->
      <footer class="p-3 border-t border-base-300 bg-base-200/40 flex justify-end flex-shrink-0">
        <button
          type="button"
          class="btn btn-sm btn-ghost"
          @click="close"
        >
          Close
        </button>
      </footer>
    </section>
  </Transition>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  XMarkIcon,
  ClockIcon,
  BoltIcon,
  InformationCircleIcon,
} from '@heroicons/vue/24/outline'
import type { ProbeObservation, ProbeRun } from './probeTypes'
import {
  formatLatency,
  formatRelativeTime,
  parseRedactedSummary,
  probeKindEmoji,
  probeKindLabel,
  probeStateLabel,
  probeVerdictLabel,
  probeVerdictTone,
} from './probeTypes'
import {
  formatNodeLatency,
  nodeHealthBadge,
  nodeLatencyTone,
  resolveNodeLatencyMs,
  type NormalizedNode,
} from '../nodes/nodeView'
import StatusBadge from '../../ui/StatusBadge.vue'
import { t } from '../../locales'

const props = withDefaults(
  defineProps<{
    open: boolean
    run?: ProbeRun | null
    node?: NormalizedNode | null
    nodeMap?: Record<string, NormalizedNode>
    observations: ProbeObservation[]
    loading: boolean
    reprobing?: boolean
  }>(),
  {
    run: null,
    node: null,
    nodeMap: () => ({}),
    reprobing: false,
  }
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'reprobe', node: NormalizedNode): void
}>()

const selectedKind = ref<string>('all')

watch(
  () => [props.open, props.node?.logicalId, props.run?.id],
  () => {
    selectedKind.value = 'all'
  }
)

const filteredObservations = computed(() => {
  if (selectedKind.value === 'all') return props.observations
  return props.observations.filter((obs) => obs.kind === selectedKind.value)
})

const kinds = computed(() => {
  const set = new Set<string>()
  props.observations.forEach((obs) => set.add(obs.kind))
  return Array.from(set)
})

function resolveObsNode(obs: ProbeObservation): NormalizedNode | null {
  if (props.node && props.node.logicalId === obs.node_logical_id) {
    return props.node
  }
  return props.nodeMap?.[obs.node_logical_id] ?? null
}

function resolveObsNodeTitle(obs: ProbeObservation): string {
  const resolved = resolveObsNode(obs)
  if (resolved) return resolved.displayName
  return obs.node_logical_id
}

function resolveObsNodeEndpoint(obs: ProbeObservation): string {
  const resolved = resolveObsNode(obs)
  if (!resolved) return ''
  const proto = resolved.protocol ? resolved.protocol.toUpperCase() : ''
  const addr =
    resolved.connection.server && resolved.connection.port
      ? `${resolved.connection.server}:${resolved.connection.port}`
      : resolved.connection.server || ''
  return [proto, addr].filter(Boolean).join(' · ')
}

function formatHumanSummary(obs: ProbeObservation): {
  statusBadge: string
  explanation: string
} {
  const raw = (obs.redacted_summary || '').trim()
  if (!raw) {
    return {
      statusBadge: '',
      explanation: obs.verdict === 'available' ? '协议握手与连通校验正常' : '已完成该维度探测采样',
    }
  }
  const parsed = parseRedactedSummary(raw)
  const isMachineKeyValue = Boolean(parsed.profile || parsed.version || parsed.reason || parsed.statusCode !== undefined)
  if (!isMachineKeyValue) {
    return {
      statusBadge: '',
      explanation: raw,
    }
  }

  const statusBadge =
    parsed.statusCode !== undefined
      ? `HTTP ${parsed.statusCode} ${parsed.statusCode >= 200 && parsed.statusCode < 400 ? '连通正常' : '响应受限'}`
      : ''
  const parts: string[] = []
  if (parsed.reasonLabel) {
    parts.push(parsed.reasonLabel)
  }
  if (parsed.error) {
    parts.push(`异常详情: ${parsed.error}`)
  }
  if (parts.length === 0) {
    parts.push(obs.verdict === 'available' ? '连接与响应正常' : '探测返回受限或不可达状态')
  }
  return {
    statusBadge,
    explanation: parts.join(' · '),
  }
}

function formatObsTime(iso: string): string {
  if (!iso) return '--'
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso
    return `${d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })} (${formatRelativeTime(iso)})`
  } catch {
    return iso
  }
}

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
      data-testid="probe-evidence-sheet"
      class="fixed bottom-0 left-0 right-0 z-50 adaptive-surface-sheet md:max-h-[82vh] md:bottom-auto md:top-1/2 md:left-1/2 md:-translate-x-1/2 md:-translate-y-1/2 md:w-full md:max-w-2xl bg-base-100 rounded-t-2xl md:rounded-2xl border-t md:border border-base-300 shadow-2xl flex flex-col overflow-hidden"
    >
      <!-- Grab Handle for Mobile Touch -->
      <div class="md:hidden pt-3 pb-1 flex justify-center flex-shrink-0 cursor-grab">
        <div class="w-12 h-1.5 rounded-full bg-base-content/20" />
      </div>

      <!-- Sheet Header -->
      <header class="flex items-start justify-between p-4 sm:p-5 border-b border-base-300 min-h-0 overflow-y-auto gap-3">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-xs font-semibold uppercase tracking-wider text-primary">
              {{ t('probes.evidenceTitle') }}
            </span>
            <StatusBadge
              v-if="node"
              data-testid="evidence-node-health-badge"
              :label="nodeHealthBadge(node, reprobing ? [node.logicalId] : undefined).label"
              :tone="nodeHealthBadge(node, reprobing ? [node.logicalId] : undefined).tone"
            />
            <StatusBadge
              v-if="node && resolveNodeLatencyMs(node) !== null"
              :label="formatNodeLatency(node)"
              :tone="nodeLatencyTone(node)"
            />
            <StatusBadge
              v-else-if="run"
              :label="probeStateLabel(run.state)"
              :tone="run.state === 'succeeded' ? 'success' : run.state === 'failed' ? 'error' : run.state === 'queued' ? 'warning' : 'info'"
            />
          </div>

          <h2 id="sheet-title" class="mt-1 text-lg sm:text-xl font-bold truncate">
            {{ node ? node.displayName : t('probes.evidenceTitle') }}
          </h2>

          <div v-if="node" class="mt-1 flex flex-wrap items-center gap-2 text-xs opacity-75 font-mono">
            <span class="badge badge-xs badge-outline uppercase">{{ node.protocol }}</span>
            <span v-if="node.connection.server && node.connection.port">
              {{ node.connection.server }}:{{ node.connection.port }}
            </span>
          </div>
          <p v-else-if="run" class="mt-0.5 text-xs opacity-65 truncate">
            {{ t('probes.evidenceDesc') }}
          </p>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <button
            v-if="node"
            type="button"
            data-testid="evidence-reprobe-btn"
            class="btn btn-primary btn-xs sm:btn-sm gap-1.5"
            :disabled="reprobing"
            @click="emit('reprobe', node)"
          >
            <BoltIcon class="w-4 h-4" :class="{ 'animate-spin': reprobing }" />
            <span>{{ reprobing ? '正在测速...' : '重新测速此节点' }}</span>
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm btn-circle sticky top-0"
            aria-label="关闭面板"
            @click="close"
          >
            <XMarkIcon class="w-5 h-5" />
          </button>
        </div>
      </header>

      <!-- Kind Filter Bar -->
      <div v-if="kinds.length > 1" class="px-4 py-2 bg-base-200/50 border-b border-base-300 flex items-center gap-1.5 overflow-x-auto text-xs flex-shrink-0">
        <span class="opacity-60 mr-1">检测维度：</span>
        <button
          type="button"
          class="btn btn-xs rounded-lg"
          :class="selectedKind === 'all' ? 'btn-primary' : 'btn-ghost'"
          @click="selectedKind = 'all'"
        >
          全部 ({{ observations.length }})
        </button>
        <button
          v-for="kind in kinds"
          :key="kind"
          type="button"
          class="btn btn-xs rounded-lg"
          :class="selectedKind === kind ? 'btn-primary' : 'btn-ghost'"
          @click="selectedKind = kind"
        >
          {{ probeKindEmoji(kind) }} {{ probeKindLabel(kind) }}
        </button>
      </div>

      <!-- Observations Content -->
      <div class="flex-1 min-h-0 p-4 sm:p-5 overflow-y-auto overscroll-contain space-y-3">
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 3" :key="i" class="skeleton h-24 rounded-xl" />
        </div>

        <div
          v-else-if="filteredObservations.length === 0"
          class="rounded-xl border border-dashed border-base-300 p-8 text-center"
        >
          <InformationCircleIcon class="w-8 h-8 mx-auto opacity-40 text-info" />
          <p class="mt-2 font-medium text-sm">暂无观测记录</p>
          <p class="mt-1 text-xs opacity-60">可点击上方「重新测速此节点」或「一键全量测速」立即采样。</p>
        </div>

        <article
          v-for="obs in filteredObservations"
          :key="obs.id"
          data-testid="observation-card"
          class="card bg-base-200/80 border border-base-300/80 rounded-xl p-3.5 transition-all duration-200 ease-out hover:border-primary/40"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="badge badge-sm font-semibold tracking-wider badge-primary badge-outline">
                  {{ probeKindEmoji(obs.kind) }} {{ probeKindLabel(obs.kind) }}
                </span>
                <span class="text-xs font-semibold truncate">
                  {{ resolveObsNodeTitle(obs) }}
                </span>
                <span
                  v-if="resolveObsNodeEndpoint(obs)"
                  class="text-[11px] font-mono opacity-60 truncate"
                >
                  {{ resolveObsNodeEndpoint(obs) }}
                </span>
              </div>

              <div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
                <span
                  v-if="formatHumanSummary(obs).statusBadge"
                  data-testid="observation-http-badge"
                  class="badge badge-xs badge-ghost font-mono"
                >
                  {{ formatHumanSummary(obs).statusBadge }}
                </span>
                <span class="text-base-content/85 leading-relaxed">
                  {{ formatHumanSummary(obs).explanation }}
                </span>
              </div>
            </div>
            <StatusBadge
              :label="probeVerdictLabel(obs.verdict)"
              :tone="probeVerdictTone(obs.verdict) === 'error' ? 'error' : probeVerdictTone(obs.verdict) === 'success' ? 'success' : probeVerdictTone(obs.verdict) === 'warning' ? 'warning' : 'info'"
            />
          </div>

          <div class="mt-3 pt-2.5 border-t border-base-300/50 flex flex-wrap items-center justify-between gap-2 text-[11px] opacity-75">
            <span class="flex items-center gap-1">
              <ClockIcon class="w-3.5 h-3.5" />
              响应延迟：<strong class="font-mono font-semibold">{{ obs.verdict === 'error' || obs.latency_ms <= 0 ? '--' : formatLatency(obs.latency_ms) }}</strong>
            </span>
            <span class="font-mono">
              检测时间：{{ formatObsTime(obs.observed_at) }}
            </span>
          </div>
        </article>
      </div>

      <!-- Sheet Footer -->
      <footer class="p-3 border-t border-base-300 bg-base-200/40 flex justify-between items-center flex-shrink-0">
        <span class="text-xs opacity-60">
          共 {{ filteredObservations.length }} 条测速记录
        </span>
        <button
          type="button"
          class="btn btn-sm btn-ghost"
          @click="close"
        >
          {{ t('common.close') }}
        </button>
      </footer>
    </section>
  </Transition>
</template>

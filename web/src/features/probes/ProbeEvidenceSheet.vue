<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ClockIcon, BoltIcon, InformationCircleIcon, ChevronLeftIcon, ChevronRightIcon } from '@heroicons/vue/24/outline'
import { displayName, displayText } from '../../utils/displayNames'
import DrawerCard from '../../ui/DrawerCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import { uiText } from '../../locales'
import type { ProbeObservation, ProbeRun } from './probeTypes'
import {
  formatLatency, formatObservationReasonLabel, formatAttemptCategory, formatRelativeTime,
  formatPlatformBadge, formatSpeed, normalizePlatforms, isPolicyOrConfigBlockedReason,
  parseRedactedSummary, probeKindEmoji, probeKindLabel, probeStateLabel,
  probeVerdictLabel, probeVerdictTone, type PlatformBadge,
} from './probeTypes'
import {
  formatNodeLatency, nodeHealthBadge, nodeHealthDiagnostic, nodeLatencyTone,
  resolveNodeLatencyMs, type NormalizedNode,
} from '../nodes/nodeView'

const props = withDefaults(defineProps<{
  open: boolean
  run?: ProbeRun | null
  node?: NormalizedNode | null
  nodeMap?: Record<string, NormalizedNode>
  observations: ProbeObservation[]
  loading: boolean
  reprobing?: boolean
  error?: string
  stale?: boolean
  total?: number
  page?: number
  pageSize?: number
  retryPage?: number
}>(), {
  run: null, node: null, nodeMap: () => ({}), reprobing: false,
  error: '', stale: false, page: 1, pageSize: 25,
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'reprobe', node: NormalizedNode): void
  (e: 'page', page: number): void
}>()
const selectedKind = ref('all')
watch(() => [props.open, props.node?.logicalId, props.run?.id, props.page], () => { selectedKind.value = 'all' })
const filteredObservations = computed(() => selectedKind.value === 'all'
  ? props.observations : props.observations.filter((obs) => obs.kind === selectedKind.value))
const kinds = computed(() => Array.from(new Set(props.observations.map((obs) => obs.kind))))
const page = computed(() => props.page)
const maxPage = computed(() => Math.max(1, Math.ceil((props.total ?? props.observations.length) / props.pageSize)))

const currentNodeDiagnostic = computed(() => props.node ? nodeHealthDiagnostic(props.node) : null)
const title = computed(() => props.node
  ? displayName(props.node.displayName, uiText('未命名节点', 'Unnamed node'), props.node.logicalId)
  : uiText('检测证据', 'Probe evidence'))

function resolveObsNode(obs: ProbeObservation): NormalizedNode | null {
  if (props.node?.logicalId === obs.node_logical_id) return props.node
  return props.nodeMap[obs.node_logical_id] ?? null
}

function resolveObsNodeTitle(obs: ProbeObservation): string {
  return displayName(resolveObsNode(obs)?.displayName, uiText('未知节点（来源不可用）', 'Unknown node (source unavailable)'), obs.node_logical_id)
}

function resolveObsNodeEndpoint(obs: ProbeObservation): string {
  const node = resolveObsNode(obs)
  if (!node) return ''
  const { server, port } = node.connection
  return [node.protocol.toUpperCase(), server && port ? `${server}:${port}` : server].filter(Boolean).join(' · ')
}

function formatHumanSummary(obs: ProbeObservation): { statusBadge: string; explanation: string } {
  const raw = obs.redacted_summary.trim()
  const parsed = parseRedactedSummary(raw)
  const machineSummary = Boolean(parsed.profile || parsed.version || parsed.reason || parsed.statusCode !== undefined)
  if (!machineSummary) return {
    statusBadge: '',
    explanation: displayText(raw || uiText('该检测维度的历史采样结果', 'Historical sample for this probe dimension')),
  }
  const parts: string[] = []
  if (parsed.reasonLabel) parts.push(parsed.reasonLabel)
  if (parsed.error && parsed.error !== parsed.reason) {
    parts.push(`${uiText('诊断详情', 'Details')}: ${formatObservationReasonLabel(parsed.error)}`)
  }
  return {
    statusBadge: parsed.statusCode !== undefined ? `HTTP ${parsed.statusCode}` : '',
    explanation: displayText(parts.join(' · ') || uiText('该检测维度的历史采样结果', 'Historical sample for this probe dimension')),
  }
}

function observationVerdictBadge(obs: ProbeObservation): { label: string; tone: 'success' | 'warning' | 'error' | 'info' } {
  const parsed = parseRedactedSummary(obs.redacted_summary)
  if (obs.verdict === 'error' && isPolicyOrConfigBlockedReason(parsed.reason)) {
    return { label: uiText('未判定 · 安全/配置阻断', 'Unverified · policy/configuration blocked'), tone: 'warning' as const }
  }
  if (obs.verdict === 'error' && parsed.reason === 'transport_error') {
    return { label: uiText('未知 · 待复核', 'Unknown · needs verification'), tone: 'info' as const }
  }
  const tone = probeVerdictTone(obs.verdict)
  return {
    label: probeVerdictLabel(obs.verdict),
    tone: tone === 'success' ? 'success' : tone === 'warning' ? 'warning' : tone === 'error' ? 'error' : 'info',
  }
}

function formatObsTime(iso: string): string {
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? iso : `${date.toLocaleString()} (${formatRelativeTime(iso)})`
}

function resolveObservationPlatforms(obs: ProbeObservation): PlatformBadge[] {
  let platforms = obs.platforms
  if (!platforms && obs.redacted_summary.startsWith('{')) {
    try {
      const summary = JSON.parse(obs.redacted_summary)
      platforms = normalizePlatforms(summary.platforms)
    } catch {
      // 旧记录的非结构化摘要仍在正文展示，不据此伪造平台结果
    }
  }
  if (!platforms) return []
  const order = obs.kind === 'streaming' ? ['netflix', 'youtube', 'disney'] : ['openai', 'claude', 'gemini', 'antigravity']
  const keys = Object.keys(platforms)
  return [...order.filter((key) => keys.includes(key)), ...keys.filter((key) => !order.includes(key))]
    .map((key) => formatPlatformBadge(key, platforms![key]))
}
</script>

<template>
  <DrawerCard :model-value="open" :title="title" :description="uiText('以下为采样当时的证据，不保证当前连接、账号权限或模型可用性。', 'Evidence reflects the time of sampling, not current connectivity, account permissions, or model access.')" width-class="max-w-3xl" @close="emit('close')">
    <section data-testid="probe-evidence-sheet" class="space-y-4 min-w-0">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0 space-y-2">
          <div class="flex flex-wrap items-center gap-2">
            <StatusBadge v-if="node && reprobing" data-testid="evidence-node-probe-state-badge" :label="uiText('检测中', 'Probing')" tone="info" :pulse="true" />
            <StatusBadge v-if="node" data-testid="evidence-node-health-badge" :label="nodeHealthBadge(node).label" :tone="nodeHealthBadge(node).tone" />
            <StatusBadge v-if="node && resolveNodeLatencyMs(node) !== null" :label="formatNodeLatency(node)" :tone="nodeLatencyTone(node)" />
            <StatusBadge v-else-if="run" :label="probeStateLabel(run.state)" :tone="run.state === 'succeeded' ? 'success' : run.state === 'failed' ? 'error' : 'info'" />
          </div>
          <p v-if="node" class="text-xs break-all opacity-70">{{ node.protocol.toUpperCase() }} · {{ node.connection.server }}<template v-if="node.connection.port">:{{ node.connection.port }}</template></p>
          <p v-if="run" class="text-xs opacity-70">{{ uiText('运行时间', 'Run created') }} · {{ new Date(run.created_at).toLocaleString() }}</p>
        </div>
        <button v-if="node" type="button" data-testid="evidence-reprobe-btn" class="btn btn-primary btn-sm" :disabled="reprobing" @click="emit('reprobe', node)">
          <BoltIcon class="w-4 h-4" :class="{ 'animate-pulse': reprobing }" />
          {{ reprobing ? uiText('正在检测…', 'Probing…') : uiText('重新检测此节点', 'Probe this node again') }}
        </button>
      </header>

      <div v-if="node && currentNodeDiagnostic" data-testid="evidence-node-diagnostic-banner" class="rounded-xl border border-base-300 bg-base-200 p-3 text-xs leading-relaxed break-words">
        <strong>{{ currentNodeDiagnostic.shortLabel }}：</strong>{{ displayText(currentNodeDiagnostic.detail) }}
      </div>
      <div v-if="error" role="alert" class="alert alert-error text-sm items-start">
        <p class="min-w-0 flex-1 whitespace-pre-wrap break-words">{{ displayText(error) }}</p>
        <button type="button" class="btn btn-xs" :disabled="loading" @click="emit('page', retryPage ?? page)">{{ uiText('重试', 'Retry') }}</button>
      </div>
      <p v-if="stale" class="text-xs text-warning">{{ uiText('正在刷新或刷新失败；以下为上次成功载入的证据。', 'Refreshing or refresh failed; showing previously loaded evidence.') }}</p>
      <div v-if="kinds.length > 1" class="flex flex-wrap items-center gap-2 text-xs" :aria-label="uiText('筛选本页检测维度', 'Filter this page by probe kind')">
        <span>{{ uiText('本页维度', 'Kinds on this page') }}</span>
        <button type="button" class="btn btn-xs" :class="selectedKind === 'all' ? 'btn-primary' : 'btn-ghost'" :aria-pressed="selectedKind === 'all'" @click="selectedKind = 'all'">{{ uiText('全部', 'All') }} ({{ observations.length }})</button>
        <button v-for="kind in kinds" :key="kind" type="button" class="btn btn-xs" :class="selectedKind === kind ? 'btn-primary' : 'btn-ghost'" :aria-pressed="selectedKind === kind" @click="selectedKind = kind">{{ probeKindEmoji(kind) }} {{ probeKindLabel(kind) }}</button>
      </div>
      <div v-if="loading && observations.length === 0" role="status" class="py-8 text-center text-sm opacity-70">{{ uiText('正在载入证据…', 'Loading evidence…') }}</div>
      <div v-else-if="!error && filteredObservations.length === 0" class="rounded-xl border border-dashed border-base-300 p-8 text-center">
        <InformationCircleIcon class="w-8 h-8 mx-auto opacity-40" />
        <p class="mt-2 font-medium text-sm">{{ uiText('暂无观测记录', 'No observations') }}</p>
        <p class="mt-1 text-xs opacity-60">{{ uiText('本页没有匹配记录；运行完成后可刷新历史重新查看。', 'No matching observations on this page. Refresh history after the run completes.') }}</p>
      </div>

      <article v-for="obs in filteredObservations" :key="obs.id" data-testid="observation-card" class="rounded-xl border border-base-300 bg-base-200/70 p-3.5 space-y-3 min-w-0">
        <div class="flex flex-wrap items-start justify-between gap-2">
          <div class="flex flex-wrap items-center gap-2 min-w-0">
            <span class="badge badge-sm badge-primary badge-outline">{{ probeKindEmoji(obs.kind) }} {{ probeKindLabel(obs.kind) }}</span>
            <span class="font-semibold text-sm break-words">{{ resolveObsNodeTitle(obs) }}</span>
          </div>
          <StatusBadge :label="observationVerdictBadge(obs).label" :tone="observationVerdictBadge(obs).tone" />
        </div>
        <p v-if="resolveObsNodeEndpoint(obs)" class="font-mono text-xs opacity-60 break-all">{{ resolveObsNodeEndpoint(obs) }}</p>
        <div class="text-xs leading-relaxed whitespace-pre-wrap break-words">
          <span v-if="formatHumanSummary(obs).statusBadge" data-testid="observation-http-badge" class="badge badge-xs badge-ghost mr-2">{{ formatHumanSummary(obs).statusBadge }}</span>
          {{ formatHumanSummary(obs).explanation }}
        </div>
        <div v-if="resolveObservationPlatforms(obs).length" data-testid="observation-platforms-grid" class="grid grid-cols-1 gap-2">
          <div v-for="badge in resolveObservationPlatforms(obs)" :key="badge.platform" class="rounded-lg bg-base-100 border border-base-300 p-3 min-w-0 space-y-2">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="text-xs font-semibold">{{ badge.platform.toUpperCase() }}</span>
              <StatusBadge :label="badge.label" :tone="badge.tone" />
            </div>
            <p v-if="badge.tooltip" data-testid="platform-evidence-text" class="text-xs leading-relaxed whitespace-pre-wrap break-words text-base-content/75">{{ displayText(badge.tooltip) }}</p>
          </div>
        </div>
        <div v-if="obs.throughput !== undefined && obs.throughput > 0" data-testid="observation-speed-row" class="text-xs space-y-1">
          <span class="badge badge-sm badge-info badge-outline">{{ uiText('历史带宽速率', 'Historical throughput') }}: {{ formatSpeed(obs.throughput) }}</span>
          <p class="opacity-65">{{ uiText('仅代表该次采样，不代表当前测速成功或当前可用带宽。', 'This sample does not establish a successful current speed test or current bandwidth.') }}</p>
        </div>
        <p v-if="obs.risk_score" data-testid="observation-risk-row" class="text-xs">{{ uiText('风险评分', 'Risk score') }}: {{ displayText(obs.risk_score) }}</p>
        <div v-if="obs.attempt" data-testid="observation-attempt-box" class="rounded-lg border border-base-300 bg-base-100 p-2 text-xs space-y-1 break-words">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span>{{ uiText('执行状态', 'Execution') }}: {{ obs.attempt.executed ? uiText('已执行出站', 'Outbound attempt executed') : uiText('未执行/跳过', 'Not executed / skipped') }}</span>
            <span v-if="obs.attempt.category" class="badge badge-xs badge-ghost">{{ formatAttemptCategory(obs.attempt.category) }}</span>
          </div>
          <p v-if="obs.attempt.reason">{{ uiText('原因', 'Reason') }}: {{ displayText(formatObservationReasonLabel(obs.attempt.reason)) }}</p>
        </div>
        <div class="border-t border-base-300 pt-2 flex flex-wrap items-center justify-between gap-2 text-xs opacity-70">
          <span class="flex items-center gap-1"><ClockIcon class="w-3.5 h-3.5" />{{ uiText('响应延迟', 'Latency') }}: {{ obs.verdict === 'error' || obs.latency_ms <= 0 ? '--' : formatLatency(obs.latency_ms) }}</span>
          <span>{{ uiText('检测时间', 'Observed') }}: {{ formatObsTime(obs.observed_at) }}</span>
        </div>
      </article>
    </section>
    <template #footer>
      <div class="w-full flex flex-wrap items-center justify-between gap-3 text-xs">
        <span>{{ uiText('本页', 'On this page') }} {{ filteredObservations.length }}<template v-if="total !== undefined"> · {{ uiText('共', 'Total') }} {{ total }}</template></span>
        <nav v-if="maxPage > 1" class="flex items-center gap-2" :aria-label="uiText('证据分页', 'Evidence pagination')">
          <button type="button" class="btn btn-ghost btn-sm btn-square" :aria-label="uiText('上一页证据', 'Previous evidence page')" :disabled="page <= 1 || loading" @click="emit('page', page - 1)"><ChevronLeftIcon class="w-4 h-4" /></button>
          <span>{{ page }} / {{ maxPage }}</span>
          <button type="button" class="btn btn-ghost btn-sm btn-square" :aria-label="uiText('下一页证据', 'Next evidence page')" :disabled="page >= maxPage || loading" @click="emit('page', page + 1)"><ChevronRightIcon class="w-4 h-4" /></button>
        </nav>
        <button type="button" class="btn btn-ghost btn-sm" @click="emit('close')">{{ uiText('关闭', 'Close') }}</button>
      </div>
    </template>
  </DrawerCard>
</template>

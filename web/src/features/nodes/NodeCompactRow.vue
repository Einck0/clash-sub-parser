<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { BoltIcon, ChevronDownIcon } from '@heroicons/vue/24/outline'
import StatusBadge from '../../ui/StatusBadge.vue'
import NodeSpeedResult from './NodeSpeedResult.vue'
import NodeTargetCompatibility from './NodeTargetCompatibility.vue'
import { displayText } from '../../utils/displayNames'
import {
  formatExpandedPlatformLabel, formatNodeLatency, getNodeGeoRegion, getNodePlatformBadges,
  nodeCapabilityLabel, nodeHealthBadge, nodeHealthDiagnostic, nodeLatencyTone, nodeRiskBadge,
  resolveNodeLatencyMs, type NormalizedNode,
} from './nodeView'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'

const props = defineProps<{
  node: NormalizedNode
  selected: boolean
  probeState: 'probing' | 'queued' | 'idle'
  probePending?: boolean
  probeRunState?: 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'expired'
  speedBusy?: boolean
  speedFailed?: boolean
  speedDisabled?: boolean
}>()

defineEmits<{
  (e: 'toggle-select', logicalId: string): void
  (e: 'open-detail', node: NormalizedNode): void
  (e: 'probe-node', node: NormalizedNode): void
  (e: 'speed-node', node: NormalizedNode): void
}>()
const detailsExpanded = ref(false)
watch(() => props.node.logicalId, () => { detailsExpanded.value = false })
const healthBadge = computed(() => nodeHealthBadge(props.node))
const riskBadge = computed(() => nodeRiskBadge(props.node))
const diagnostic = computed(() => nodeHealthDiagnostic(props.node))
const geoRegion = computed(() => getNodeGeoRegion(props.node))
const streamingBadges = computed(() => getNodePlatformBadges(props.node, 'streaming'))
const aiBadges = computed(() => getNodePlatformBadges(props.node, 'ai'))
</script>

<template>
  <article data-testid="node-card" class="rounded-xl border border-base-300 bg-base-100 p-3 md:px-4 hover:border-primary/40 transition min-w-0" :class="{ 'ring-2 ring-primary/40': selected }">
    <div class="flex flex-col md:flex-row md:items-start gap-3 text-xs min-w-0">
      <div class="flex items-start gap-2.5 min-w-0 md:w-1/4 shrink-0">
        <input type="checkbox" :checked="selected" :data-testid="`node-checkbox-${node.logicalId}`" :aria-label="uiText(`选择 ${node.displayName}`, `Select ${node.displayName}`)" class="checkbox checkbox-primary checkbox-xs shrink-0 mt-1" @change="$emit('toggle-select', node.logicalId)" />
        <div class="min-w-0 flex-1">
          <h3 class="font-semibold text-sm leading-snug min-w-0">
            <button type="button" class="text-left w-full break-words [overflow-wrap:anywhere] hover:text-primary" @click="$emit('open-detail', node)">{{ node.displayName }}</button>
          </h3>
          <div class="flex flex-wrap items-center gap-1.5 mt-1 text-[11px] text-base-content/65">
            <span class="font-mono uppercase">{{ node.protocol }}</span>
            <span v-if="geoRegion" data-testid="node-geo-badge">{{ geoRegion }}</span>
            <span>{{ node.active ? t('common.enabled') : t('common.disabled') }}</span>
          </div>
        </div>
      </div>

      <div class="min-w-0 md:flex-1 space-y-2">
        <div class="flex flex-wrap items-center gap-1.5" data-testid="node-primary-status">
          <StatusBadge data-testid="node-health-badge" :label="`${uiText('健康', 'Health')}: ${healthBadge.label}`" :tone="healthBadge.tone" />
          <StatusBadge data-testid="node-latency-badge" :label="`${uiText('延迟', 'Latency')}: ${resolveNodeLatencyMs(node) !== null ? formatNodeLatency(node) : '—'}`" :tone="nodeLatencyTone(node)" class="font-mono font-semibold" />
          <StatusBadge v-if="probePending" data-testid="node-probe-pending" :label="uiText('提交中', 'Submitting')" tone="warning" pulse />
          <StatusBadge v-else-if="probeState !== 'idle'" data-testid="node-probe-state-badge" :label="probeState === 'probing' ? uiText('检测中', 'Probing') : uiText('队列等待', 'Queued')" :tone="probeState === 'probing' ? 'info' : 'warning'" pulse />
          <StatusBadge v-else-if="probeRunState === 'failed' || probeRunState === 'cancelled' || probeRunState === 'expired'" data-testid="node-probe-terminal-state-badge" :label="probeRunState === 'failed' ? uiText('上次检测失败', 'Last probe failed') : probeRunState === 'cancelled' ? uiText('上次检测已取消', 'Last probe cancelled') : uiText('上次检测已超时', 'Last probe expired')" :tone="probeRunState === 'failed' ? 'error' : 'warning'" />
        </div>
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-base-content/70" data-testid="node-secondary-status">
          <NodeSpeedResult :node="node" :busy="speedBusy" :failed="speedFailed" />
          <StatusBadge data-testid="node-risk-badge" :label="`${uiText('风险', 'Risk')}: ${riskBadge.label}`" :tone="riskBadge.tone" />
          <span v-if="probeState === 'idle' && diagnostic && diagnostic.code !== 'probe_missing'" data-testid="node-card-diagnostic-badge" class="text-warning break-words [overflow-wrap:anywhere]" :title="displayText(diagnostic.detail)">{{ diagnostic.shortLabel }}</span>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-1.5 md:justify-end md:max-w-64 shrink-0">
        <button type="button" data-testid="node-card-probe-btn" class="btn btn-outline btn-primary btn-xs gap-1" :disabled="probePending || probeState !== 'idle' || !node.active" @click="$emit('probe-node', node)">
          <BoltIcon class="w-3.5 h-3.5" :class="{ 'animate-spin': probeState === 'probing' }" />
          {{ probePending ? uiText('提交中…', 'Submitting…') : uiText('普通检测', 'Probe') }}
        </button>
        <button type="button" data-testid="node-card-speed-btn" class="btn btn-ghost btn-xs" :disabled="speedDisabled || !node.active" @click="$emit('speed-node', node)">{{ speedBusy ? uiText('测速中…', 'Measuring…') : uiText('手动带宽测速', 'Measure bandwidth') }}</button>
        <button type="button" data-testid="node-inspect-btn" class="btn btn-ghost btn-xs" @click="$emit('open-detail', node)">{{ t('nodes.inspectNode') }}</button>
        <button type="button" data-testid="node-platform-details-toggle" class="btn btn-ghost btn-xs text-base-content/65" :aria-expanded="detailsExpanded" :aria-controls="`node-platform-details-${node.logicalId}`" @click="detailsExpanded = !detailsExpanded">
          {{ detailsExpanded ? uiText('收起', 'Collapse') : uiText('能力与端点', 'Capabilities & endpoint') }}
          <ChevronDownIcon class="w-3 h-3 transition-transform" :class="{ 'rotate-180': detailsExpanded }" />
        </button>
      </div>
    </div>

    <div v-if="detailsExpanded" :id="`node-platform-details-${node.logicalId}`" data-testid="node-platform-details-expanded" class="mt-3 pt-3 border-t border-base-300 space-y-3 text-xs min-w-0">
      <p class="text-base-content/65">{{ uiText('平台能力详情（按最近一次检测证据）', 'Platform capability details from the latest probe evidence') }}</p>
      <p class="font-mono break-words [overflow-wrap:anywhere]">{{ node.connection.server }}<template v-if="node.connection.port">:{{ node.connection.port }}</template></p>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <section class="space-y-1.5">
          <h4 class="font-semibold">{{ t('nodes.streaming') }}</h4>
          <div class="flex flex-wrap gap-1.5">
            <StatusBadge v-for="badge in streamingBadges" :key="badge.platform" :label="formatExpandedPlatformLabel(badge)" :tone="badge.tone" :title="badge.tooltip" />
            <StatusBadge v-if="!streamingBadges.length" :label="nodeCapabilityLabel(node, 'streaming').label" :tone="nodeCapabilityLabel(node, 'streaming').tone" />
          </div>
        </section>
        <section class="space-y-1.5">
          <h4 class="font-semibold">{{ t('nodes.ai') }}</h4>
          <div class="flex flex-wrap gap-1.5">
            <StatusBadge v-for="badge in aiBadges" :key="badge.platform" :label="formatExpandedPlatformLabel(badge)" :tone="badge.tone" :title="badge.tooltip" />
            <StatusBadge v-if="!aiBadges.length" :label="nodeCapabilityLabel(node, 'ai').label" :tone="nodeCapabilityLabel(node, 'ai').tone" />
          </div>
        </section>
      </div>
      <NodeTargetCompatibility :node="node" />
    </div>
  </article>
</template>

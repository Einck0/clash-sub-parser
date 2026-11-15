<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  BoltIcon,
  CheckCircleIcon,
  ChevronDownIcon,
  ExclamationTriangleIcon,
} from '@heroicons/vue/24/outline'
import StatusBadge from '../../ui/StatusBadge.vue'
import NodeSpeedResult from './NodeSpeedResult.vue'
import { displayText } from '../../utils/displayNames'
import {
  formatExpandedPlatformLabel,
  formatNodeLatency,
  getNodeGeoRegion,
  getNodePlatformBadges,
  getNodeSpeedBadge,
  nodeCapabilityLabel,
  nodeHealthBadge,
  nodeHealthDiagnostic,
  nodeLatencyTone,
  nodeRiskBadge,
  nodeSupportedTargets,
  nodeTargetDiagnostics,
  resolveNodeLatencyMs,
  type NormalizedNode,
} from './nodeView'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'

const props = defineProps<{
  node: NormalizedNode
  selected: boolean
  probeState: 'probing' | 'queued' | 'idle'
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

const healthBadge = computed(() => nodeHealthBadge(props.node))
const riskBadge = computed(() => nodeRiskBadge(props.node))
const diagnostic = computed(() => nodeHealthDiagnostic(props.node))
const geoRegion = computed(() => getNodeGeoRegion(props.node))

const streamingBadges = computed(() => getNodePlatformBadges(props.node, 'streaming'))
const aiBadges = computed(() => getNodePlatformBadges(props.node, 'ai'))
const speedBadge = computed(() => props.speedBusy || props.speedFailed ? null : getNodeSpeedBadge(props.node))

const hasPlatformDetails = computed(() => {
  return true
})
</script>

<template>
  <article
    data-testid="node-card"
    class="border border-base-300 bg-base-200/70 hover:bg-base-200 shadow-sm transition hover:border-primary/40 rounded-xl overflow-hidden cursor-pointer p-3 md:py-2.5 md:px-4"
    :class="{ 'ring-2 ring-primary/40 bg-primary/5': selected }"
    @click="$emit('open-detail', node)"
  >
    <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-2.5 md:gap-3 text-xs min-w-0">
      <!-- Col 1: Identity & Selection -->
      <div class="flex items-center gap-2.5 min-w-0 md:w-1/4 shrink-0">
        <input
          type="checkbox"
          :checked="selected"
          :data-testid="`node-checkbox-${node.logicalId}`"
          class="checkbox checkbox-primary checkbox-xs shrink-0"
          @click.stop
          @change="$emit('toggle-select', node.logicalId)"
        />
        <div class="rounded-lg bg-primary/10 p-1.5 md:p-2 text-primary shrink-0">
          <CheckCircleIcon v-if="node.active" class="h-4 w-4 md:h-5 md:w-5" />
          <ExclamationTriangleIcon v-else class="h-4 w-4 md:h-5 md:w-5" />
        </div>
        <div class="min-w-0 flex-1">
          <h3 class="truncate font-semibold text-sm">{{ node.displayName }}</h3>
        </div>
        <!-- Mobile-only status badge next to name -->
        <div class="flex md:hidden items-center gap-1.5 shrink-0">
          <span class="badge badge-outline font-mono text-xs uppercase">{{ node.protocol }}</span>
          <StatusBadge
            :label="node.active ? t('common.enabled') : t('common.disabled')"
            :tone="node.active ? 'success' : 'warning'"
          />
        </div>
      </div>

      <!-- Col 2: Protocol Connection, Targets & Geo -->
      <div class="flex flex-wrap items-center gap-1.5 text-[11px] font-mono opacity-80 min-w-0 md:w-1/5 shrink-0 [&>span]:max-w-full [&>span]:min-w-0">
        <span class="hidden md:inline-flex badge badge-outline font-mono text-xs uppercase shrink-0">{{ node.protocol }}</span>
        <span
          v-if="geoRegion"
          data-testid="node-geo-badge"
          class="badge badge-xs badge-neutral font-mono font-semibold uppercase"
          :title="uiText('落地地区', 'Geo Region') + ': ' + geoRegion"
        >
          {{ geoRegion }}
        </span>
        <span
          class="badge badge-xs badge-ghost block truncate"
          :title="`兼容: ${nodeSupportedTargets(node).join('/') || '无'}`"
        >
          兼容: {{ nodeSupportedTargets(node).join('/') || '无' }}
        </span>
        <span
          v-for="message in nodeTargetDiagnostics(node)"
          :key="message"
          data-testid="node-target-diagnostic"
          class="badge badge-xs badge-warning badge-outline h-auto min-w-0 max-w-full whitespace-normal break-words [overflow-wrap:anywhere]"
          :title="displayText(message)"
        >
          {{ displayText(message) }}
        </span>
        <span
          v-if="node.connection.server && node.connection.port"
          class="badge badge-xs badge-ghost block truncate"
          :title="`${node.connection.server}:${node.connection.port}`"
        >
          {{ node.connection.server }}:{{ node.connection.port }}
        </span>
        <template v-if="node.protocol.toLowerCase() === 'wireguard'">
          <span
            v-if="node.connection.localAddress.length"
            class="badge badge-xs badge-info badge-outline block truncate"
            :title="node.connection.localAddress.join(', ')"
          >
            {{ node.connection.localAddress.join(', ') }}
          </span>
          <span v-if="node.connection.mtu" class="badge badge-xs badge-ghost">
            MTU: {{ node.connection.mtu }}
          </span>
        </template>
        <template v-else-if="node.protocol.toLowerCase() === 'tuic'">
          <span
            v-if="node.connection.congestionControl"
            class="badge badge-xs badge-info badge-outline block truncate"
            :title="node.connection.congestionControl"
          >
            {{ node.connection.congestionControl }}
          </span>
          <span
            v-if="node.connection.udpRelayMode"
            class="badge badge-xs badge-ghost block truncate"
            :title="node.connection.udpRelayMode"
          >
            {{ node.connection.udpRelayMode }}
          </span>
        </template>
      </div>

      <!-- Col 3: Prominent Baseline Latency, Health, Task State, Risk & Platform Details Toggle -->
      <div class="flex flex-wrap items-center gap-1.5 min-w-0 md:flex-1">
        <StatusBadge
          v-if="probeState !== 'idle'"
          data-testid="node-probe-state-badge"
          :label="probeState === 'probing' ? '⚡ 检测中' : '⏳ 队列等待'"
          :tone="probeState === 'probing' ? 'info' : 'warning'"
          :pulse="true"
        />
        <StatusBadge
          data-testid="node-health-badge"
          :label="`健康: ${healthBadge.label}`"
          :tone="healthBadge.tone"
        />
        <StatusBadge
          v-if="resolveNodeLatencyMs(node) !== null"
          data-testid="node-latency-badge"
          :label="`延迟: ${formatNodeLatency(node)}`"
          :tone="nodeLatencyTone(node)"
          class="font-mono font-semibold"
        />
        <StatusBadge
          data-testid="node-risk-badge"
          :label="`风险: ${riskBadge.label}`"
          :tone="riskBadge.tone"
        />
        <span
          v-if="probeState === 'idle' && diagnostic && diagnostic.code !== 'probe_missing'"
          data-testid="node-card-diagnostic-badge"
          class="badge badge-sm md:badge-xs gap-1 text-[11px] h-auto py-0.5 whitespace-normal"
          :class="diagnostic.isBlockedByPolicyOrConfig ? 'badge-warning badge-outline' : 'badge-ghost opacity-80'"
          :title="displayText(diagnostic.detail)"
        >
          {{ diagnostic.shortLabel }}
        </span>

        <!-- Platform capabilities expand toggle button -->
        <button
          v-if="hasPlatformDetails"
          type="button"
          data-testid="node-platform-details-toggle"
          class="btn btn-ghost btn-xs text-[11px] gap-1 px-1.5 text-base-content/70 hover:text-base-content"
          :class="{ 'btn-active text-primary': detailsExpanded }"
          :aria-expanded="detailsExpanded"
          :aria-controls="`node-platform-details-${node.logicalId}`"
          :title="detailsExpanded ? uiText('收起流媒体与AI能力详情', 'Collapse platform capability details') : uiText('展开流媒体与AI能力详情', 'Expand platform capability details')"
          @click.stop="detailsExpanded = !detailsExpanded"
        >
          <span>{{ detailsExpanded ? uiText('收起能力', 'Collapse') : uiText('平台能力', 'Platforms') }}</span>
          <ChevronDownIcon class="w-3 h-3 transition-transform" :class="{ 'rotate-180': detailsExpanded }" />
        </button>

        <!-- Compact platform badge previews (abbreviated) -->
        <template v-if="!detailsExpanded">
          <StatusBadge
            v-if="speedBadge"
            :label="speedBadge.label"
            :tone="speedBadge.tone"
          />
          <StatusBadge
            v-for="badge in streamingBadges"
            :key="`compact-streaming-${badge.platform}`"
            :label="badge.label"
            :tone="badge.tone"
            :title="badge.tooltip"
          />
          <StatusBadge
            v-for="badge in aiBadges"
            :key="`compact-ai-${badge.platform}`"
            :label="badge.label"
            :tone="badge.tone"
            :title="badge.tooltip"
          />
        </template>
      </div>

      <!-- Col 4: Status and Action Buttons -->
      <div class="flex items-center justify-between md:justify-end gap-2 border-t md:border-t-0 border-base-300 pt-2 md:pt-0 shrink-0">
        <div class="hidden md:block">
          <StatusBadge
            :label="node.active ? t('common.enabled') : t('common.disabled')"
            :tone="node.active ? 'success' : 'warning'"
          />
        </div>
        <div class="flex flex-wrap items-center gap-1.5 ml-auto md:ml-0">
          <NodeSpeedResult :node="node" :busy="speedBusy" :failed="speedFailed" />
          <button
            type="button"
            data-testid="node-card-speed-btn"
            class="btn btn-outline btn-primary btn-xs"
            :disabled="speedDisabled || !node.active"
            @click.stop="$emit('speed-node', node)"
          >{{ speedBusy ? '测速中…' : '手动测速' }}</button>
          <button
            type="button"
            data-testid="node-card-probe-btn"
            class="btn btn-outline btn-primary btn-xs gap-1"
            :disabled="probeState === 'probing'"
            @click.stop="$emit('probe-node', node)"
          >
            <BoltIcon
              class="w-3.5 h-3.5"
              :class="{ 'animate-spin': probeState === 'probing' }"
            />
            <span>{{ probeState === 'probing' ? '检测中...' : '立即重测' }}</span>
          </button>
          <button
            type="button"
            data-testid="node-inspect-btn"
            class="btn btn-ghost btn-xs font-mono shrink-0"
            @click.stop="$emit('open-detail', node)"
          >
            {{ t('nodes.inspectNode') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Inline Expanded Platform & Speed Details Accordion -->
    <div
      v-if="detailsExpanded"
      :id="`node-platform-details-${node.logicalId}`"
      data-testid="node-platform-details-expanded"
      class="mt-2.5 pt-2.5 border-t border-base-300/80 space-y-2 text-xs"
      @click.stop
    >
      <p class="text-[11px] text-base-content/60">
        {{ uiText('平台能力详情（按最近一次有效检测结果）', 'Platform capability details from the latest valid check') }}
      </p>
      <div class="flex flex-wrap items-start gap-2">
        <div class="rounded-lg bg-base-100 border border-base-300 px-2 py-1.5 space-y-1">
          <p class="font-semibold text-[11px]">{{ t('nodes.streaming') }}</p>
          <div class="flex flex-wrap items-center gap-1.5">
            <template v-if="streamingBadges.length > 0">
              <StatusBadge
                v-for="badge in streamingBadges"
                :key="`streaming-${badge.platform}`"
                :label="formatExpandedPlatformLabel(badge)"
                :tone="badge.tone"
                :title="badge.tooltip"
              />
            </template>
            <StatusBadge
              v-else
              :label="nodeCapabilityLabel(node, 'streaming').label"
              :tone="nodeCapabilityLabel(node, 'streaming').tone"
            />
          </div>
        </div>
        <div class="rounded-lg bg-base-100 border border-base-300 px-2 py-1.5 space-y-1">
          <p class="font-semibold text-[11px]">{{ t('nodes.ai') }}</p>
          <div class="flex flex-wrap items-center gap-1.5">
            <template v-if="aiBadges.length > 0">
              <StatusBadge
                v-for="badge in aiBadges"
                :key="`ai-${badge.platform}`"
                :label="formatExpandedPlatformLabel(badge)"
                :tone="badge.tone"
                :title="badge.tooltip"
              />
            </template>
            <StatusBadge
              v-else
              :label="nodeCapabilityLabel(node, 'ai').label"
              :tone="nodeCapabilityLabel(node, 'ai').tone"
            />
          </div>
        </div>
        <div v-if="speedBadge" class="rounded-lg bg-base-100 border border-base-300 px-2 py-1.5 space-y-1">
          <p class="font-semibold text-[11px]">{{ uiText('测速', 'Speed') }}</p>
          <StatusBadge :label="speedBadge.label" :tone="speedBadge.tone" />
        </div>
      </div>
    </div>
  </article>
</template>

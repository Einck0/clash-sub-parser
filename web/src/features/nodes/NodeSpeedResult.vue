<script setup lang="ts">
import { computed } from 'vue'
import type { NormalizedNode } from './nodeView'
import { formatSpeed } from '../probes/probeTypes'
import { uiText } from '../../locales/redesign'

const props = defineProps<{ node: NormalizedNode; busy?: boolean; failed?: boolean }>()
const speed = computed(() => props.node.capabilityDetails.speed)
const current = computed(() => !props.busy && !props.failed && speed.value?.verdict === 'available' && !speed.value.stale && typeof speed.value.throughput === 'number' && Number.isFinite(speed.value.throughput) && speed.value.throughput > 0)
const label = computed(() => {
  if (props.busy) return uiText('正在测速…', 'Measuring speed…')
  if (props.failed) return uiText('本次测速未取得可用结果', 'No usable result from this attempt')
  if (!speed.value) return uiText('尚未测速', 'Not measured')
  if (speed.value.stale || speed.value.verdict === 'stale') return uiText('测速结果已失效，请重新测速', 'Speed result is stale; measure again')
  if (current.value) return formatSpeed(speed.value.throughput!)
  return uiText('测速结果不可用', 'Speed result unavailable')
})
const observedTime = computed(() => {
  const raw = speed.value?.observed_at
  if (!raw || !Number.isFinite(Date.parse(raw))) return ''
  return new Date(raw).toLocaleString()
})
</script>

<template>
  <div data-testid="node-speed-result" class="min-w-0 text-[11px] break-words [overflow-wrap:anywhere]" :class="current ? 'text-success' : 'text-base-content/70'" role="status">
    <span class="font-semibold">{{ uiText('带宽', 'Speed') }}: {{ label }}</span>
    <span v-if="observedTime && !busy" class="block text-base-content/60">
      {{ failed ? uiText('上次保存', 'Last saved') : uiText('采样时间', 'Observed') }}:
      <time :datetime="speed?.observed_at">{{ observedTime }}</time>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { displayText } from '../../utils/displayNames'
import StatusBadge from '../../ui/StatusBadge.vue'
import { nodeSupportedTargets, nodeTargetDiagnostics, type NormalizedNode } from './nodeView'
import { uiText } from '../../locales/redesign'

const props = defineProps<{ node: NormalizedNode }>()
const targets = computed(() => nodeSupportedTargets(props.node))
const diagnostics = computed(() => nodeTargetDiagnostics(props.node))
const exclusions = computed(() => Object.entries(props.node.targetCompatibility ?? {})
  .filter(([, value]) => value.status !== 'compatible')
  .map(([target, value]) => ({ target, status: value.status })))
</script>

<template>
  <section data-testid="node-target-compatibility" class="space-y-1.5 min-w-0">
    <div class="flex flex-wrap items-center gap-1.5">
      <span class="font-semibold">{{ uiText('目标兼容性', 'Target compatibility') }}</span>
      <StatusBadge v-for="target in targets" :key="target" :label="`${target}: ${node.targetCompatibility ? uiText('参数兼容', 'Parameters compatible') : uiText('协议支持', 'Protocol supported')}`" tone="success" />
      <StatusBadge v-if="!targets.length" :label="uiText('暂无兼容目标', 'No compatible target')" tone="warning" />
    </div>
    <p v-if="!node.targetCompatibility" data-testid="node-target-unverified" class="text-base-content/65">{{ uiText('仅显示协议支持范围，尚无当前连接参数的兼容诊断。', 'Protocol support only; compatibility diagnostics for the current connection are unavailable.') }}</p>
    <div v-if="exclusions.length" class="flex flex-wrap gap-1.5">
      <StatusBadge v-for="item in exclusions" :key="item.target"
        :label="`${displayText(item.target)}: ${item.status === 'format_error' ? uiText('参数格式错误', 'Invalid parameters') : uiText('目标不兼容', 'Target incompatible')}`"
        :tone="item.status === 'format_error' ? 'error' : 'warning'" />
    </div>
    <p class="text-base-content/65">{{ uiText('兼容性独立于检测健康；完整导出仍需核对配置预览。', 'Compatibility is separate from probe health. Review the full configuration before exporting.') }}</p>
    <p v-for="message in diagnostics" :key="message" data-testid="node-target-diagnostic" class="text-warning break-words [overflow-wrap:anywhere]">{{ message }}</p>
  </section>
</template>

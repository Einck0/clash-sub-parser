<template>
  <div class="space-y-2.5 w-full min-w-0" data-testid="ledger-mobile-list">
    <!-- Paged Cards List -->
    <div
      v-for="item in pagedItems"
      :key="item.name"
      class="flex flex-col gap-2 p-3 rounded-lg border bg-surface-base transition-colors cursor-pointer w-full min-w-0 max-w-full overflow-hidden"
      :class="[
        isSelected(item.name)
          ? 'border-accent bg-accent-subtle/30 ring-1 ring-accent/30'
          : 'border-border-subtle hover:border-border-strong hover:bg-surface-hover/60'
      ]"
      data-testid="ledger-mobile-card"
      @click="emit('inspect', item)"
    >
      <!-- Top Row: 44px Checkbox + Country + Name + Protocol Badge -->
      <div class="flex items-center gap-2 min-w-0 w-full">
        <!-- 44px Hit Target Checkbox -->
        <label
          class="flex items-center justify-center min-w-[44px] min-h-[44px] -m-2 cursor-pointer shrink-0 select-none"
          @click.stop
          :aria-label="`选择节点 ${item.name}`"
        >
          <input
            type="checkbox"
            :checked="isSelected(item.name)"
            class="h-4 w-4 rounded-sm border-border-strong bg-canvas text-accent focus:ring-0 cursor-pointer"
            @change="emit('toggleSelect', item.name)"
          />
        </label>

        <!-- Country Code -->
        <span class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-surface-active text-text-muted border border-border-subtle shrink-0 tabular-nums">
          {{ resolveCountry(item) }}
        </span>

        <!-- Node Name (Truncated) -->
        <span class="text-xs font-semibold text-text-main truncate min-w-0 flex-1 font-mono" :title="item.name">
          {{ item.name }}
        </span>

        <!-- Protocol Badge -->
        <StatusBadge type="info" :text="(item.type || 'RAW').toUpperCase()" class="shrink-0" />
      </div>

      <!-- Middle Row: Server:Port + Subscription + Dialer Chain -->
      <div class="flex items-center justify-between text-[11px] font-mono text-text-muted min-w-0 gap-2 pl-9">
        <span class="truncate tabular-nums">{{ item.server }}:{{ item.port }}</span>
        <div class="flex items-center gap-1.5 shrink-0">
          <span
            v-if="item.dialer_proxy"
            class="text-accent text-[10px] px-1 py-0.5 rounded border border-accent/30 bg-accent-subtle truncate max-w-[100px]"
          >
            链: {{ item.dialer_proxy }}
          </span>
          <span v-if="item.subscription_name" class="text-text-sub truncate max-w-[100px]">
            [{{ item.subscription_name }}]
          </span>
        </div>
      </div>

      <!-- Compact Media Tags Row (if probed) -->
      <div
        v-if="getNodeMediaBadges(item).length"
        class="flex items-center gap-1.5 overflow-x-auto py-0.5 pl-9 scrollbar-none min-w-0"
      >
        <span
          v-for="badge in getNodeMediaBadges(item)"
          :key="badge.key"
          class="rounded px-1.5 py-0.5 text-[10px] font-mono font-bold shrink-0 border inline-flex items-center gap-0.5 cursor-pointer"
          :class="badge.badgeClass"
          :title="badge.accessibleTitle"
          @click.stop="emit('inspect', item)"
        >
          {{ badge.shortBadgeText }}
        </span>
      </div>

      <!-- Bottom Row: Status / Latency / Speed + 44px Detail / Probe Actions -->
      <div class="flex items-center justify-between pt-2 border-t border-border-subtle/60 gap-2 min-w-0 w-full pl-9">
        <!-- Status / Latency / Speed -->
        <div class="flex items-center gap-2 text-xs font-mono min-w-0 flex-1 flex-wrap">
          <span
            v-if="getProbe(item)?.status === 'ok'"
            class="text-status-success font-semibold tabular-nums flex items-center gap-1"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-status-success inline-block"></span>
            {{ getProbe(item)?.latency_ms }}ms
          </span>
          <span
            v-else-if="getProbe(item)?.status === 'fail'"
            class="text-status-danger font-medium flex items-center gap-1"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-status-danger inline-block"></span>
            失败
          </span>
          <span
            v-else-if="getProbe(item)?.status === 'timeout'"
            class="text-status-warning font-medium flex items-center gap-1"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-status-warning inline-block"></span>
            超时
          </span>
          <span v-else class="text-text-sub flex items-center gap-1">
            <span class="h-1.5 w-1.5 rounded-full bg-surface-active inline-block"></span>
            未测
          </span>

          <span
            v-if="getProbe(item)?.speed_mbps"
            class="text-status-info font-semibold tabular-nums text-[11px]"
          >
            {{ getProbe(item)?.speed_mbps }}M
          </span>
        </div>

        <!-- 44px Action Targets -->
        <div class="flex items-center gap-2 shrink-0">
          <button
            type="button"
            class="min-h-[44px] min-w-[44px] px-3 py-2 rounded-md border border-border-subtle bg-surface-hover hover:border-border-strong text-text-main text-xs font-mono flex items-center justify-center cursor-pointer transition-colors"
            @click.stop="emit('inspect', item)"
            aria-label="查看节点详情"
          >
            详情
          </button>
          <button
            type="button"
            class="min-h-[44px] min-w-[44px] px-3 py-2 rounded-md border border-accent/40 bg-accent-subtle text-accent hover:bg-accent/20 text-xs font-mono font-medium flex items-center justify-center gap-1 cursor-pointer transition-colors disabled:opacity-50"
            :disabled="probingSingleKey === item.name"
            @click.stop="emit('probeSingle', item)"
            aria-label="测试节点"
          >
            <Zap class="h-3.5 w-3.5" :class="{ 'animate-spin': probingSingleKey === item.name }" aria-hidden="true" />
            <span>测速</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Pagination Controls (Rendered only when items > pageSize) -->
    <div
      v-if="totalPages > 1"
      class="flex items-center justify-between py-2 px-1 text-xs font-mono text-text-muted"
    >
      <button
        type="button"
        class="min-h-[44px] min-w-[44px] px-3 py-2 rounded-md border border-border-subtle bg-surface-hover text-text-main disabled:opacity-40 cursor-pointer transition-colors"
        :disabled="currentPage <= 1"
        @click="currentPage--"
      >
        上一页
      </button>
      <span class="tabular-nums">第 {{ currentPage }} / {{ totalPages }} 页 (共 {{ items.length }} 节点)</span>
      <button
        type="button"
        class="min-h-[44px] min-w-[44px] px-3 py-2 rounded-md border border-border-subtle bg-surface-hover text-text-main disabled:opacity-40 cursor-pointer transition-colors"
        :disabled="currentPage >= totalPages"
        @click="currentPage++"
      >
        下一页
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Zap } from 'lucide-vue-next'
import StatusBadge from '../ui/StatusBadge.vue'
import {
  type LedgerNodeItem,
  type ProbeRecord,
  getProbeForNode,
  resolveNodeCountryCode,
  MEDIA_PLATFORMS,
  getMediaSemanticPresentation,
} from '../../views/nodeLedgerDomain'

const props = withDefaults(
  defineProps<{
    items: LedgerNodeItem[]
    selectedNames?: Set<string>
    probes?: Record<string, ProbeRecord>
    probingSingleKey?: string | null
    pageSize?: number
  }>(),
  {
    selectedNames: () => new Set<string>(),
    probes: () => ({}),
    probingSingleKey: null,
    pageSize: 50,
  }
)

const emit = defineEmits<{
  (e: 'toggleSelect', nodeName: string): void
  (e: 'inspect', node: LedgerNodeItem): void
  (e: 'probeSingle', node: LedgerNodeItem): void
}>()

const currentPage = ref(1)
const totalPages = computed(() => Math.max(1, Math.ceil(props.items.length / props.pageSize)))

watch(
  () => props.items.length,
  () => {
    if (currentPage.value > totalPages.value) {
      currentPage.value = 1
    }
  }
)

const pagedItems = computed(() => {
  if (props.items.length <= props.pageSize) return props.items
  const start = (currentPage.value - 1) * props.pageSize
  return props.items.slice(start, start + props.pageSize)
})

function isSelected(name: string): boolean {
  return props.selectedNames?.has(name) ?? false
}

function getProbe(node: LedgerNodeItem): ProbeRecord | undefined {
  return getProbeForNode(props.probes, node)
}

function resolveCountry(node: LedgerNodeItem): string {
  const p = getProbe(node)
  const code = resolveNodeCountryCode(node, p)
  return code === 'OTHER' ? '--' : code
}

function getNodeMediaBadges(node: LedgerNodeItem) {
  const p = getProbe(node)
  if (!p?.media || typeof p.media !== 'object') return []
  const badges: any[] = []
  for (const plat of MEDIA_PLATFORMS) {
    const item = p.media[plat.key]
    if (!item || typeof item !== 'object') continue
    if (item.status === undefined && item.verdict === undefined && item.unlocked === undefined) continue
    const pres = getMediaSemanticPresentation(item, plat)
    badges.push({ key: plat.key, ...pres })
  }
  return badges
}
</script>

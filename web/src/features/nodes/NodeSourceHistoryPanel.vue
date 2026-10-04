<script setup lang="ts">
import { computed } from 'vue'
import {
  ClockIcon,
  ExclamationTriangleIcon,
  ShieldCheckIcon,
} from '@heroicons/vue/24/outline'
import StatusBadge from '../../ui/StatusBadge.vue'
import {
  attributionStatusBadge,
  attributionCauseLabel,
  relationStateBadge,
  formatObservedTime,
  type NodeSourceHistoryData,
} from './sourceHistoryTypes'

const props = defineProps<{
  logicalId: string
  sourceHistory: NodeSourceHistoryData | null
  loading: boolean
  error?: string
}>()

defineEmits<{
  (e: 'retry'): void
}>()

const verifiedHistory = computed(() => {
  if (!props.sourceHistory?.history) return []
  return props.sourceHistory.history.filter((h) => h.relation_state === 'verified')
})

const conflictOrUnknownHistory = computed(() => {
  if (!props.sourceHistory?.history) return []
  return props.sourceHistory.history.filter((h) => h.relation_state !== 'verified')
})

const isEntirelyEmpty = computed(() => {
  if (!props.sourceHistory) return false
  const noCurrent = !props.sourceHistory.current_sources || props.sourceHistory.current_sources.length === 0
  const noHistory = !props.sourceHistory.history || props.sourceHistory.history.length === 0
  return noCurrent && noHistory
})
</script>

<template>
  <div
    data-testid="node-source-history-panel"
    class="p-3.5 rounded-xl bg-base-200 border border-base-300 space-y-3 min-w-0 max-w-full overflow-hidden text-xs"
  >
    <!-- Panel Header: Title & Overall Status -->
    <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/80 pb-2.5">
      <div class="flex items-center gap-1.5 font-semibold text-primary">
        <ClockIcon class="w-4 h-4 shrink-0" />
        <span>来源归属与历史追溯 (Source Attribution)</span>
      </div>
      <div class="flex items-center gap-1.5 shrink-0">
        <span class="text-[11px] opacity-70">综合归属:</span>
        <StatusBadge
          data-testid="overall-attribution-status-badge"
          :label="attributionStatusBadge(sourceHistory?.attribution_status).label"
          :tone="attributionStatusBadge(sourceHistory?.attribution_status).tone"
        />
      </div>
    </div>

    <!-- Mandatory Audit Disclaimer -->
    <div
      data-testid="source-history-disclaimer"
      class="p-2.5 rounded-lg bg-base-300/50 border border-base-300 text-[11px] leading-relaxed text-base-content/85 flex items-start gap-2"
    >
      <ExclamationTriangleIcon class="w-4 h-4 shrink-0 text-warning mt-0.5" />
      <span>历史归属仅用于追溯，不代表当前订阅成员或启用状态</span>
    </div>

    <!-- Loading State -->
    <div v-if="loading" data-testid="source-history-loading" class="space-y-2 py-2">
      <div class="skeleton h-8 w-full rounded-lg" />
      <div class="skeleton h-12 w-full rounded-lg" />
    </div>

    <!-- Error State -->
    <div
      v-else-if="error"
      data-testid="source-history-error"
      class="p-3 rounded-lg bg-error/10 border border-error/30 text-error flex items-center justify-between gap-2"
    >
      <span class="truncate flex-1">{{ error }}</span>
      <button
        type="button"
        class="btn btn-ghost btn-xs text-error shrink-0"
        @click="$emit('retry')"
      >
        重试
      </button>
    </div>

    <!-- Content Area -->
    <div v-else class="space-y-3.5">
      <!-- Overall Insufficient Evidence Fallback -->
      <div
        v-if="isEntirelyEmpty"
        data-testid="source-history-empty"
        class="p-3 rounded-lg bg-base-300/40 text-center text-xs opacity-75 leading-relaxed"
      >
        来源证据不足，未检索到该节点的历史归属信息
      </div>

      <template v-else>
        <!-- 1. Current Sources Section -->
        <section data-testid="current-sources-section" class="space-y-2">
          <div class="flex items-center justify-between text-xs font-semibold opacity-90">
            <span class="flex items-center gap-1.5">
              <ShieldCheckIcon class="w-3.5 h-3.5 text-primary shrink-0" />
              <span>当前所属订阅 (Current Sources)</span>
            </span>
            <span class="text-[11px] font-mono opacity-65">
              {{ sourceHistory?.current_sources?.length || 0 }} 个
            </span>
          </div>

          <div
            v-if="sourceHistory?.current_sources && sourceHistory.current_sources.length > 0"
            class="space-y-1.5"
          >
            <div
              v-for="src in sourceHistory.current_sources"
              :key="src.subscription_id"
              data-testid="current-source-item"
              class="p-2.5 rounded-lg bg-base-100 border border-base-300 flex flex-wrap items-center justify-between gap-2"
            >
              <div class="min-w-0 flex-1">
                <div class="font-medium truncate">{{ src.name }}</div>
                <div class="font-mono text-[10px] opacity-60 truncate">
                  订阅 ID: {{ src.subscription_id }}
                </div>
              </div>
              <div class="shrink-0">
                <StatusBadge
                  v-if="src.enabled"
                  data-testid="current-source-enabled-badge"
                  label="订阅已启用"
                  tone="success"
                />
                <StatusBadge
                  v-else
                  data-testid="current-source-disabled-badge"
                  label="订阅已停用"
                  tone="warning"
                />
              </div>
            </div>
          </div>
          <div
            v-else
            data-testid="current-sources-empty"
            class="text-[11px] opacity-60 italic py-1 px-1"
          >
            当前无活跃订阅关联 (孤立/失活节点)
          </div>
        </section>

        <!-- 2. Verified Historical Sources Section -->
        <section data-testid="verified-history-section" class="space-y-2 pt-2 border-t border-base-300/60">
          <div class="flex items-center justify-between text-xs font-semibold opacity-90">
            <span>已确认历史来源 (Verified Historical Sources)</span>
            <span class="text-[11px] font-mono opacity-65">
              {{ verifiedHistory.length }} 条
            </span>
          </div>

          <div v-if="verifiedHistory.length > 0" class="space-y-2">
            <div
              v-for="(item, idx) in verifiedHistory"
              :key="`verified-${item.source_label}-${idx}`"
              data-testid="verified-history-item"
              class="p-2.5 rounded-lg bg-base-100 border border-base-300 space-y-1.5"
            >
              <div class="flex flex-wrap items-center justify-between gap-1.5">
                <div class="flex items-center gap-1.5 min-w-0">
                  <span class="font-semibold truncate">{{ item.source_label }}</span>
                  <span
                    v-if="item.source_deleted"
                    data-testid="source-deleted-badge"
                    class="badge badge-xs badge-ghost border-error/40 text-error font-medium"
                  >
                    已删除
                  </span>
                  <span
                    v-else-if="item.source_unmapped"
                    data-testid="source-unmapped-badge"
                    class="badge badge-xs badge-ghost border-warning/40 text-warning font-medium"
                  >
                    未关联当前订阅
                  </span>
                  <span
                    v-else-if="item.subscription_id"
                    class="badge badge-xs badge-ghost font-mono opacity-70 truncate max-w-[140px]"
                  >
                    {{ item.subscription_id }}
                  </span>
                </div>
                <StatusBadge
                  :label="relationStateBadge(item.relation_state).label"
                  :tone="relationStateBadge(item.relation_state).tone"
                />
              </div>

              <div class="flex flex-wrap items-center gap-1.5 text-[10px]">
                <span class="badge badge-xs badge-outline">
                  {{ attributionCauseLabel(item.cause) }}
                </span>
                <span class="badge badge-xs badge-neutral font-mono truncate max-w-[180px]">
                  证据: {{ item.evidence_kind }}
                </span>
                <span
                  v-if="item.connection_revision !== undefined && item.connection_revision !== null"
                  class="badge badge-xs badge-ghost font-mono"
                >
                  rev. {{ item.connection_revision }}
                </span>
              </div>

              <div class="text-[10px] opacity-70 flex flex-wrap gap-x-3 gap-y-0.5 font-mono">
                <template v-if="item.first_observed_at || item.last_observed_at">
                  <span v-if="item.first_observed_at">
                    首次观测: {{ formatObservedTime(item.first_observed_at) }}
                  </span>
                  <span v-if="item.last_observed_at">
                    末次观测: {{ formatObservedTime(item.last_observed_at) }}
                  </span>
                </template>
                <span v-else data-testid="history-timestamp-missing">
                  观测时间: 未留记录
                </span>
              </div>
            </div>
          </div>
          <div
            v-else
            data-testid="verified-history-empty"
            class="text-[11px] opacity-60 italic py-1 px-1"
          >
            暂无已核验的历史来源记录
          </div>
        </section>

        <!-- 3. Conflicts and Unknown Evidence Section -->
        <section data-testid="conflict-history-section" class="space-y-2 pt-2 border-t border-base-300/60">
          <div class="flex items-center justify-between text-xs font-semibold opacity-90">
            <span>冲突与未知来源 (Conflicts & Unknown Evidence)</span>
            <span class="text-[11px] font-mono opacity-65">
              {{ conflictOrUnknownHistory.length }} 条
            </span>
          </div>

          <div v-if="conflictOrUnknownHistory.length > 0" class="space-y-2">
            <div
              v-for="(item, idx) in conflictOrUnknownHistory"
              :key="`conflict-${item.source_label}-${idx}`"
              data-testid="conflict-history-item"
              class="p-2.5 rounded-lg bg-base-100 border border-base-300 space-y-1.5"
            >
              <div class="flex flex-wrap items-center justify-between gap-1.5">
                <div class="flex items-center gap-1.5 min-w-0">
                  <span class="font-semibold truncate">{{ item.source_label }}</span>
                  <span
                    v-if="item.source_deleted"
                    data-testid="source-deleted-badge"
                    class="badge badge-xs badge-ghost border-error/40 text-error font-medium"
                  >
                    已删除
                  </span>
                  <span
                    v-else-if="item.source_unmapped"
                    data-testid="source-unmapped-badge"
                    class="badge badge-xs badge-ghost border-warning/40 text-warning font-medium"
                  >
                    未关联当前订阅
                  </span>
                  <span
                    v-else-if="item.subscription_id"
                    class="badge badge-xs badge-ghost font-mono opacity-70 truncate max-w-[140px]"
                  >
                    {{ item.subscription_id }}
                  </span>
                </div>
                <StatusBadge
                  v-if="item.relation_state === 'conflict'"
                  data-testid="conflict-badge"
                  label="归属冲突"
                  tone="warning"
                />
                <StatusBadge
                  v-else
                  data-testid="unknown-badge"
                  label="未知来源"
                  tone="info"
                />
              </div>

              <div class="flex flex-wrap items-center gap-1.5 text-[10px]">
                <span class="badge badge-xs badge-outline">
                  {{ attributionCauseLabel(item.cause) }}
                </span>
                <span class="badge badge-xs badge-neutral font-mono truncate max-w-[180px]">
                  证据: {{ item.evidence_kind }}
                </span>
                <span
                  v-if="item.connection_revision !== undefined && item.connection_revision !== null"
                  class="badge badge-xs badge-ghost font-mono"
                >
                  rev. {{ item.connection_revision }}
                </span>
              </div>

              <div class="text-[10px] opacity-70 flex flex-wrap gap-x-3 gap-y-0.5 font-mono">
                <template v-if="item.first_observed_at || item.last_observed_at">
                  <span v-if="item.first_observed_at">
                    首次观测: {{ formatObservedTime(item.first_observed_at) }}
                  </span>
                  <span v-if="item.last_observed_at">
                    末次观测: {{ formatObservedTime(item.last_observed_at) }}
                  </span>
                </template>
                <span v-else data-testid="history-timestamp-missing">
                  观测时间: 未留记录
                </span>
              </div>
            </div>
          </div>
          <div
            v-else
            data-testid="conflict-history-empty"
            class="text-[11px] opacity-60 italic py-1 px-1"
          >
            无冲突或未知历史记录
          </div>
        </section>
      </template>
    </div>
  </div>
</template>

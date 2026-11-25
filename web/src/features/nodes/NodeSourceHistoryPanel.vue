<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { uiText } from '../../locales/redesign'
import NodeSourceHistoryPager from './NodeSourceHistoryPager.vue'
import { displayName, displayText } from '../../utils/displayNames'
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

const historyQuery = ref('')
const pageSize = 20
const verifiedPage = ref(1)
const conflictPage = ref(1)
const currentPage = ref(1)
watch(() => [props.logicalId, props.sourceHistory, historyQuery.value], () => {
  verifiedPage.value = 1
  conflictPage.value = 1
  currentPage.value = 1
})
const currentSources = computed(() => props.sourceHistory?.current_sources ?? [])
const visibleCurrentSources = computed(() => currentSources.value.slice((currentPage.value - 1) * pageSize, currentPage.value * pageSize))
function matchesQuery(item: NodeSourceHistoryData['history'][number]) {
  const query = historyQuery.value.trim().toLocaleLowerCase()
  if (!query) return true
  return [item.source_label, item.cause, item.evidence_kind, item.subscription_id, item.relation_state]
    .some((value) => value?.toLocaleLowerCase().includes(query))
}

const verifiedHistory = computed(() => {
  if (!props.sourceHistory?.history) return []
  return props.sourceHistory.history.filter((item) => item.relation_state === 'verified' && matchesQuery(item))
})
const visibleVerifiedHistory = computed(() => verifiedHistory.value.slice((verifiedPage.value - 1) * pageSize, verifiedPage.value * pageSize))

const conflictOrUnknownHistory = computed(() => {
  if (!props.sourceHistory?.history) return []
  return props.sourceHistory.history.filter((item) => item.relation_state !== 'verified' && matchesQuery(item))
})
const visibleConflictOrUnknownHistory = computed(() => conflictOrUnknownHistory.value.slice((conflictPage.value - 1) * pageSize, conflictPage.value * pageSize))

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
        <span>{{ uiText('来源归属与历史追溯 (Source Attribution)', 'Source attribution and history') }}</span>
      </div>
      <div class="flex items-center gap-1.5 shrink-0">
        <span class="text-[11px] opacity-70">{{ uiText('综合归属:', 'Overall attribution:') }}</span>
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
      <span>{{ uiText('历史归属仅用于追溯，不代表当前订阅成员或启用状态', 'Historical attribution is for auditing only; it does not indicate current subscription membership or enabled status.') }}</span>
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
      <span class="truncate flex-1">{{ displayText(error) }}</span>
      <button
        type="button"
        class="btn btn-ghost btn-xs text-error shrink-0"
        @click="$emit('retry')"
      >
        {{ uiText('重试', 'Retry') }}
      </button>
    </div>

    <!-- Content Area -->
    <div v-else class="space-y-3.5">
      <label class="block space-y-1">
        <span>{{ uiText('搜索历史来源', 'Search source history') }}</span>
        <input v-model="historyQuery" type="search" data-testid="source-history-search" class="input input-xs w-full" :placeholder="uiText('来源名称、证据或关联标识', 'Source name, evidence or relation id')" />
      </label>

      <!-- Overall Insufficient Evidence Fallback -->
      <div
        v-if="isEntirelyEmpty"
        data-testid="source-history-empty"
        class="p-3 rounded-lg bg-base-300/40 text-center text-xs opacity-75 leading-relaxed"
      >
        {{ uiText('来源证据不足，未检索到该节点的历史归属信息', 'Insufficient source evidence. No historical attribution was found for this node.') }}
      </div>

      <template v-else>
        <!-- 1. Current Sources Section -->
        <section data-testid="current-sources-section" class="space-y-2">
          <div class="flex items-center justify-between text-xs font-semibold opacity-90">
            <span class="flex items-center gap-1.5">
              <ShieldCheckIcon class="w-3.5 h-3.5 text-primary shrink-0" />
              <span>{{ uiText('当前所属订阅 (Current Sources)', 'Current source subscriptions') }}</span>
            </span>
            <span class="text-[11px] font-mono opacity-65">
              {{ sourceHistory?.current_sources?.length || 0 }} {{ uiText('个', 'sources') }}
            </span>
          </div>

          <div
            v-if="sourceHistory?.current_sources && sourceHistory.current_sources.length > 0"
            class="space-y-1.5"
          >
            <div
              v-for="src in visibleCurrentSources"
              :key="src.subscription_id"
              data-testid="current-source-item"
              class="p-2.5 rounded-lg bg-base-100 border border-base-300 flex flex-wrap items-center justify-between gap-2"
            >
              <div class="min-w-0 flex-1">
                <div class="font-medium truncate">{{ displayName(src.name, uiText('未知订阅', 'Unknown subscription'), src.subscription_id) }}</div>
              </div>
              <div class="shrink-0">
                <StatusBadge
                  v-if="src.enabled"
                  data-testid="current-source-enabled-badge"
                  :label="uiText('订阅已启用', 'Subscription enabled')"
                  tone="success"
                />
                <StatusBadge
                  v-else
                  data-testid="current-source-disabled-badge"
                  :label="uiText('订阅已停用', 'Subscription disabled')"
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
            {{ uiText('当前无活跃订阅关联 (孤立/失活节点)', 'No active subscription association (orphaned or inactive node)') }}
          </div>
          <NodeSourceHistoryPager v-model:page="currentPage" :total="currentSources.length" :page-size="pageSize" />
        </section>

        <!-- 2. Verified Historical Sources Section -->
        <section data-testid="verified-history-section" class="space-y-2 pt-2 border-t border-base-300/60">
          <div class="flex items-center justify-between text-xs font-semibold opacity-90">
            <span>{{ uiText('已确认历史来源 (Verified Historical Sources)', 'Verified historical sources') }}</span>
            <span class="text-[11px] font-mono opacity-65">
              {{ verifiedHistory.length }} {{ uiText('条匹配', 'matching') }}
            </span>
          </div>

          <div v-if="visibleVerifiedHistory.length > 0" class="space-y-2">
            <div
              v-for="(item, idx) in visibleVerifiedHistory"
              :key="`verified-${item.source_label}-${idx}`"
              data-testid="verified-history-item"
              class="p-2.5 rounded-lg bg-base-100 border border-base-300 space-y-1.5"
            >
              <div class="flex flex-wrap items-center justify-between gap-1.5">
                <div class="flex items-center gap-1.5 min-w-0">
                  <span class="font-semibold truncate">{{ displayName(item.source_label, uiText('未知历史来源', 'Unknown historical source'), item.subscription_id) }}</span>
                  <span
                    v-if="item.source_deleted"
                    data-testid="source-deleted-badge"
                    class="badge badge-xs badge-ghost border-error/40 text-error font-medium"
                  >
                    {{ uiText('已删除', 'Deleted') }}
                  </span>
                  <span
                    v-else-if="item.source_unmapped"
                    data-testid="source-unmapped-badge"
                    class="badge badge-xs badge-ghost border-warning/40 text-warning font-medium"
                  >
                    {{ uiText('未关联当前订阅', 'Not linked to a current subscription') }}
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
                  {{ uiText('证据:', 'Evidence:') }} {{ item.evidence_kind }}
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
                    {{ uiText('首次观测:', 'First observed:') }} {{ formatObservedTime(item.first_observed_at) }}
                  </span>
                  <span v-if="item.last_observed_at">
                    {{ uiText('末次观测:', 'Last observed:') }} {{ formatObservedTime(item.last_observed_at) }}
                  </span>
                </template>
                <span v-else data-testid="history-timestamp-missing">
                  {{ uiText('观测时间: 未留记录', 'Observed: No record') }}
                </span>
              </div>
            </div>
          </div>
          <div
            v-else
            data-testid="verified-history-empty"
            class="text-[11px] opacity-60 italic py-1 px-1"
          >
            {{ uiText('暂无已核验的历史来源记录', 'No verified historical source records') }}
          </div>
          <NodeSourceHistoryPager v-model:page="verifiedPage" :total="verifiedHistory.length" :page-size="pageSize" />
        </section>

        <!-- 3. Conflicts and Unknown Evidence Section -->
        <section data-testid="conflict-history-section" class="space-y-2 pt-2 border-t border-base-300/60">
          <div class="flex items-center justify-between text-xs font-semibold opacity-90">
            <span>{{ uiText('冲突与未知来源 (Conflicts & Unknown Evidence)', 'Conflicting and unknown sources') }}</span>
            <span class="text-[11px] font-mono opacity-65">
              {{ conflictOrUnknownHistory.length }} {{ uiText('条匹配', 'matching') }}
            </span>
          </div>

          <div v-if="visibleConflictOrUnknownHistory.length > 0" class="space-y-2">
            <div
              v-for="(item, idx) in visibleConflictOrUnknownHistory"
              :key="`conflict-${item.source_label}-${idx}`"
              data-testid="conflict-history-item"
              class="p-2.5 rounded-lg bg-base-100 border border-base-300 space-y-1.5"
            >
              <div class="flex flex-wrap items-center justify-between gap-1.5">
                <div class="flex items-center gap-1.5 min-w-0">
                  <span class="font-semibold truncate">{{ displayName(item.source_label, uiText('未知历史来源', 'Unknown historical source'), item.subscription_id) }}</span>
                  <span
                    v-if="item.source_deleted"
                    data-testid="source-deleted-badge"
                    class="badge badge-xs badge-ghost border-error/40 text-error font-medium"
                  >
                    {{ uiText('已删除', 'Deleted') }}
                  </span>
                  <span
                    v-else-if="item.source_unmapped"
                    data-testid="source-unmapped-badge"
                    class="badge badge-xs badge-ghost border-warning/40 text-warning font-medium"
                  >
                    {{ uiText('未关联当前订阅', 'Not linked to a current subscription') }}
                  </span>
                </div>
                <StatusBadge
                  v-if="item.relation_state === 'conflict'"
                  data-testid="conflict-badge"
                  :label="uiText('归属冲突', 'Attribution conflict')"
                  tone="warning"
                />
                <StatusBadge
                  v-else
                  data-testid="unknown-badge"
                  :label="uiText('未知来源', 'Unknown source')"
                  tone="info"
                />
              </div>

              <div class="flex flex-wrap items-center gap-1.5 text-[10px]">
                <span class="badge badge-xs badge-outline">
                  {{ attributionCauseLabel(item.cause) }}
                </span>
                <span class="badge badge-xs badge-neutral font-mono truncate max-w-[180px]">
                  {{ uiText('证据:', 'Evidence:') }} {{ item.evidence_kind }}
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
                    {{ uiText('首次观测:', 'First observed:') }} {{ formatObservedTime(item.first_observed_at) }}
                  </span>
                  <span v-if="item.last_observed_at">
                    {{ uiText('末次观测:', 'Last observed:') }} {{ formatObservedTime(item.last_observed_at) }}
                  </span>
                </template>
                <span v-else data-testid="history-timestamp-missing">
                  {{ uiText('观测时间: 未留记录', 'Observed: No record') }}
                </span>
              </div>
            </div>
          </div>
          <div
            v-else
            data-testid="conflict-history-empty"
            class="text-[11px] opacity-60 italic py-1 px-1"
          >
            {{ uiText('无冲突或未知历史记录', 'No conflicting or unknown historical records') }}
          </div>
          <NodeSourceHistoryPager v-model:page="conflictPage" :total="conflictOrUnknownHistory.length" :page-size="pageSize" />
        </section>
      </template>
    </div>
  </div>
</template>

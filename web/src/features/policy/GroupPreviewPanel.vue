<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowPathIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  InformationCircleIcon,
} from '@heroicons/vue/24/outline'
import type {
  GroupDraftPreviewResponse,
  ResolverDiagnostic,
} from './policyTypes'
import { sourceKindLabel } from './policyTypes'
import { uiText } from '../../locales/redesign'

const props = withDefaults(
  defineProps<{
    loading?: boolean
    snapshot: GroupDraftPreviewResponse | null
    error?: string
    diagnostics?: Array<ResolverDiagnostic | string>
    isDraft?: boolean
    sourcesCount?: number
  }>(),
  {
    loading: false,
    error: '',
    diagnostics: () => [],
    isDraft: true,
    sourcesCount: 0,
  }
)

const emit = defineEmits<{
  (e: 'refresh'): void
}>()

const candidateCount = computed(() => {
  if (!props.snapshot) return '-'
  return (
    props.snapshot.candidate_count ??
    ((props.snapshot.node_count ?? 0) + (props.snapshot.excluded_count ?? 0))
  )
})

const memberCount = computed(() => {
  if (!props.snapshot) return '-'
  return (
    props.snapshot.member_count ??
    props.snapshot.members?.length ??
    props.snapshot.node_count ??
    0
  )
})

const excludedCount = computed(() => {
  if (!props.snapshot) return '-'
  return props.snapshot.excluded_count ?? 0
})

const resolvedMembers = computed(() => {
  return props.snapshot?.members || []
})

const exclusionBreakdown = computed(() => {
  if (!props.snapshot || !props.snapshot.excluded_count) return []
  if (props.snapshot.excluded_reasons && props.snapshot.excluded_reasons.length > 0) {
    return props.snapshot.excluded_reasons
  }
  if (props.snapshot.exclusion_summary) {
    return Object.entries(props.snapshot.exclusion_summary).map(
      ([reason, count]) => `${reason}: ${count}`
    )
  }
  return []
})
</script>

<template>
  <div class="space-y-4 text-xs font-sans min-w-0" data-testid="group-preview-panel">
    <!-- Top Header with responsive flex-wrap and shrink-0 badges for narrow screens -->
    <div class="p-3 rounded-xl bg-base-200/70 border border-base-300">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <span
            v-if="isDraft"
            class="badge badge-sm badge-warning font-semibold shrink-0"
          >
            {{ uiText('草稿预览 (Draft Preview)', 'Draft Preview') }}
          </span>
          <span
            v-else
            class="badge badge-sm badge-primary font-semibold shrink-0"
          >
            {{ uiText('实时预览', 'Live Preview') }}
          </span>

          <span
            v-if="snapshot?.revision !== undefined"
            class="badge badge-xs badge-ghost font-mono shrink-0"
          >
            v{{ snapshot.revision }}
          </span>

          <span
            v-if="loading"
            class="badge badge-xs badge-info font-mono shrink-0 animate-pulse"
          >
            {{ uiText('计算中...', 'Calculating...') }}
          </span>
        </div>

        <button
          type="button"
          class="btn btn-ghost btn-xs gap-1 shrink-0 condition-action-btn"
          :class="{ loading }"
          @click="emit('refresh')"
        >
          <ArrowPathIcon class="w-3.5 h-3.5" :class="{ 'animate-spin': loading }" />
          {{ uiText('刷新计算', 'Refresh Calculation') }}
        </button>
      </div>
    </div>

    <!-- Error Alert if preview request failed -->
    <div
      v-if="error"
      role="alert"
      class="p-3 rounded-xl bg-warning/10 border border-warning/30 text-warning text-xs flex items-center justify-between gap-2"
    >
      <div class="flex items-center gap-2 min-w-0">
        <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
        <span class="truncate">{{ error }}</span>
      </div>
      <button
        type="button"
        class="btn btn-warning btn-xs shrink-0"
        :disabled="loading"
        @click="emit('refresh')"
      >
        {{ uiText('重试', 'Retry') }}
      </button>
    </div>

    <!-- Metrics Cards (candidate-count, member-count, excluded-count) -->
    <div
      data-testid="stage-realtime-results"
      class="p-3.5 rounded-xl bg-base-200/50 border border-base-300 space-y-3"
    >
      <div class="flex items-center justify-between">
        <span class="font-bold text-xs">
          {{ uiText('节点与成员统计', 'Nodes & Members Metrics') }}
        </span>
        <button
          type="button"
          class="btn btn-ghost btn-xs opacity-70 condition-action-btn"
          @click="emit('refresh')"
        >
          {{ uiText('刷新计算', 'Refresh Calculation') }}
        </button>
      </div>

      <div class="grid grid-cols-3 gap-2 text-center">
        <!-- Candidate Count -->
        <div
          data-testid="candidate-count"
          class="p-2.5 rounded-lg bg-base-100 border border-base-300"
        >
          <span class="text-[10px] opacity-70 block">
            {{ uiText('候选节点数', 'Candidate Nodes') }}
          </span>
          <div class="text-base font-bold font-mono text-primary mt-0.5">
            {{ candidateCount }}
          </div>
        </div>

        <!-- Member Count (snapshot count includes builtins and subgroups) -->
        <div
          data-testid="member-count"
          class="p-2.5 rounded-lg bg-base-100 border border-base-300"
        >
          <span class="text-[10px] opacity-70 block">
            {{ uiText('生效成员数', 'Effective Members') }}
          </span>
          <div class="text-base font-bold font-mono text-success mt-0.5">
            {{ memberCount }}
          </div>
          <span
            v-if="snapshot && snapshot.node_count !== undefined"
            class="text-[10px] opacity-60 block mt-0.5 font-mono"
          >
            {{ snapshot.node_count }} 节点
          </span>
        </div>

        <!-- Excluded Count -->
        <div
          data-testid="excluded-count"
          class="p-2.5 rounded-lg bg-base-100 border border-base-300"
        >
          <span class="text-[10px] opacity-70 block">
            {{ uiText('排除节点数', 'Excluded Nodes') }}
          </span>
          <div class="text-base font-bold font-mono text-error mt-0.5">
            {{ excludedCount }}
          </div>
        </div>
      </div>
    </div>

    <!-- Exclusion Breakdown Section -->
    <div
      v-if="snapshot && snapshot.excluded_count && snapshot.excluded_count > 0"
      class="p-3 rounded-xl bg-base-200/50 border border-base-300 space-y-1.5"
    >
      <span class="font-bold text-xs block text-error">
        {{ uiText('排除原因明细', 'Exclusion Breakdown') }}
      </span>
      <div v-if="exclusionBreakdown.length > 0" class="space-y-1">
        <div
          v-for="(reason, idx) in exclusionBreakdown"
          :key="idx"
          class="text-xs p-1.5 rounded bg-base-100 border border-base-300/80 font-mono"
        >
          {{ reason }}
        </div>
      </div>
      <div v-else class="text-xs opacity-75 italic">
        已排除 {{ snapshot.excluded_count }} 个节点（未提供细因明细）
      </div>
    </div>

    <!-- Diagnostics List -->
    <div
      data-testid="stage-diagnostics"
      class="space-y-2"
    >
      <!-- Empty sources warning vs No query matches diagnostic -->
      <div
        v-if="sourcesCount === 0"
        role="alert"
        class="alert alert-warning text-xs p-3"
      >
        <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
        <span>空来源项 (Empty Sources)：当前策略组未配置任何成员来源项。请在编排配置页添加来源项。</span>
      </div>
      <div
        v-else-if="snapshot && snapshot.node_count === 0"
        role="alert"
        class="alert alert-info text-xs p-3"
      >
        <InformationCircleIcon class="w-4 h-4 shrink-0" />
        <span>无匹配节点 (No Nodes Matched)：当前规则或筛选条件下未匹配到任何可用节点。</span>
      </div>

      <div
        v-for="(diag, idx) in diagnostics"
        :key="idx"
        class="p-2.5 rounded-lg border text-xs flex items-start gap-2"
        :class="typeof diag === 'object' && diag.severity === 'error' ? 'bg-error/10 border-error/30 text-error' : (typeof diag === 'object' && diag.severity === 'warning' ? 'bg-warning/10 border-warning/30 text-warning' : 'bg-info/10 border-info/30 text-info')"
      >
        <ExclamationTriangleIcon
          v-if="typeof diag === 'object' && (diag.severity === 'error' || diag.severity === 'warning')"
          class="w-4 h-4 shrink-0 mt-0.5"
        />
        <InformationCircleIcon v-else class="w-4 h-4 shrink-0 mt-0.5" />
        <div class="min-w-0">
          <p class="font-semibold">
            {{ typeof diag === 'string' ? diag : diag.message }}
          </p>
          <span
            v-if="typeof diag === 'object' && diag.code"
            class="font-mono text-[10px] opacity-75"
          >
            {{ diag.code }}
          </span>
        </div>
      </div>
    </div>

    <!-- Resolved Members List -->
    <div
      data-testid="stage-resolved-members"
      class="p-3.5 rounded-xl bg-base-200/50 border border-base-300 space-y-2"
    >
      <div class="flex items-center justify-between">
        <span class="font-bold text-xs">
          {{ uiText('解析结果成员列表', 'Resolved Members List') }}
        </span>
        <span class="badge badge-xs badge-ghost font-mono">
          {{ resolvedMembers.length }} 项
        </span>
      </div>

      <div
        v-if="resolvedMembers.length === 0"
        class="text-xs opacity-60 italic py-2 text-center"
      >
        {{ uiText('暂无解析成员（请检查来源条件或刷新）', 'No resolved members. Check source conditions or refresh.') }}
      </div>

      <div v-else class="space-y-1.5 max-h-60 overflow-y-auto pr-1">
        <div
          v-for="(member, idx) in resolvedMembers"
          :key="idx"
          class="flex items-center justify-between p-2 rounded-lg bg-base-100 border border-base-300 text-xs font-mono min-w-0 gap-2"
        >
          <div class="flex items-center gap-1.5 min-w-0 flex-1 truncate">
            <span class="badge badge-xs badge-ghost shrink-0 font-bold">
              #{{ idx + 1 }}
            </span>
            <span class="badge badge-xs badge-outline shrink-0 font-sans">
              {{ sourceKindLabel(member.kind) }}
            </span>
            <span class="truncate font-sans font-medium">
              {{ member.display_name || member.node_logical_id || member.group_id || member.builtin_name || '-' }}
            </span>
          </div>

          <span
            v-if="member.node_logical_id"
            class="text-[11px] opacity-60 font-mono shrink-0 truncate max-w-[120px]"
          >
            {{ member.node_logical_id }}
          </span>
        </div>
      </div>
    </div>

    <!-- Digests Information (Selection Digest & Content Digest) -->
    <div
      v-if="snapshot?.selection_digest"
      class="p-3 rounded-xl bg-base-200/40 border border-base-300/80 space-y-1 text-[11px] font-mono"
    >
      <div class="flex items-center justify-between gap-2">
        <span class="opacity-70 font-sans">{{ uiText('选择集指纹', 'Selection Digest') }}:</span>
        <span class="font-bold truncate text-primary max-w-[200px]" :title="snapshot.selection_digest">
          {{ snapshot.selection_digest }}
        </span>
      </div>
      <div v-if="snapshot.content_digest" class="flex items-center justify-between gap-2">
        <span class="opacity-70 font-sans">{{ uiText('内容指纹', 'Content Digest') }}:</span>
        <span class="font-bold truncate text-secondary max-w-[200px]" :title="snapshot.content_digest">
          {{ snapshot.content_digest }}
        </span>
      </div>
    </div>
  </div>
</template>

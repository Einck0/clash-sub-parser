<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  CpuChipIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  PencilSquareIcon,
  TrashIcon,
  PlusIcon,
} from '@heroicons/vue/24/outline'
import type { PolicyGroup } from './policyTypes'
import {
  groupTypeLabel,
  groupTypeSupportedTargets,
  normalizeGroupSources,
  sourceKindLabel,
  sourceOperationLabel,
  computeSourceSemantics,
} from './policyTypes'
import StatusBadge from '../../ui/StatusBadge.vue'
import { uiText } from '../../locales/redesign'
import { t } from '../../locales'

const props = withDefaults(
  defineProps<{
    group: PolicyGroup
    availableGroups?: PolicyGroup[]
    selected?: boolean
    canMoveUp?: boolean
    canMoveDown?: boolean
  }>(),
  {
    availableGroups: () => [],
    selected: false,
    canMoveUp: false,
    canMoveDown: false,
  }
)

const emit = defineEmits<{
  (e: 'select', group: PolicyGroup): void
  (e: 'edit', group: PolicyGroup): void
  (e: 'delete', id: string): void
  (e: 'manage-edges', group: PolicyGroup): void
  (e: 'move-up', group: PolicyGroup): void
  (e: 'move-down', group: PolicyGroup): void
}>()

const expanded = ref(false)

const normalizedSources = computed(() => {
  return Array.isArray(props.group.sources) && props.group.sources.length > 0
    ? props.group.sources
    : normalizeGroupSources(props.group)
})

const sourceSemantics = computed(() => computeSourceSemantics(normalizedSources.value))
const sourceCount = computed(() => normalizedSources.value.length)
const conditionCount = computed(() => normalizedSources.value.reduce((count, s) => count + (s.filter_spec?.conditions.length || 0), 0))

const membershipMode = computed(() => {
  if (sourceCount.value > 0) {
    const hasGroupRef = normalizedSources.value.some((s) => s.kind === 'group_ref')
    const hasGroupNodes = normalizedSources.value.some((s) => s.kind === 'group_nodes')
    const hasRegex = normalizedSources.value.some((s) => s.kind === 'regex')
    const hasFilter = normalizedSources.value.some((s) => s.kind === 'filter')
    const hasManual = normalizedSources.value.some((s) => s.kind === 'manual_node')

    if (hasGroupRef || hasGroupNodes) {
      return {
        key: 'cascade',
        label: hasGroupNodes ? '子组解包级联' : '子策略组级联',
        fullLabel: '包含子策略组引用或解包展开',
        badgeClass: 'badge-secondary',
        secondaryNotice: hasFilter || hasRegex ? '同级来源顺序增减，后续可重新加入' : undefined,
      }
    }
    if (hasManual) {
      return {
        key: 'explicit',
        label: '手工节点来源',
        fullLabel: '包含显式指定的手工节点来源',
        badgeClass: 'badge-accent',
        secondaryNotice: hasFilter || hasRegex ? '同级来源顺序增减，后续可重新加入' : undefined,
      }
    }
    if (hasFilter || hasRegex) {
      return {
        key: 'dynamic',
        label: '全池动态匹配',
        fullLabel: '全池动态匹配模式（正则与条件动态匹配候选节点）',
        badgeClass: 'badge-info',
      }
    }
  }

  if (sourceCount.value > 0) {
    return { key: 'builtin', label: '内建流控来源', fullLabel: '包含内建流控项来源', badgeClass: 'badge-warning' }
  }
  return {
    key: 'empty',
    label: '无成员来源',
    fullLabel: '未配置成员来源（空策略组）',
    badgeClass: 'badge-ghost',
  }
})

function handleHeaderClick() {
  emit('select', props.group)
  expanded.value = !expanded.value
}
</script>

<template>
  <article
    class="card border bg-base-200 shadow-sm transition-all duration-200 ease-out hover:border-primary/50 hover:shadow-md min-w-0 max-w-full overflow-hidden"
    :class="selected ? 'border-primary ring-2 ring-primary/30 shadow-md bg-primary/[0.03]' : 'border-base-300'"
    data-testid="group-card"
  >
    <div class="card-body p-4 sm:p-5 gap-3 min-w-0 max-w-full overflow-hidden">
      <!-- Card Header with Tap-to-Select and Grow -->
      <div
        class="flex items-start justify-between gap-3 cursor-pointer select-none active:scale-[0.99] transition-transform duration-200 ease-out min-w-0"
        @click="handleHeaderClick"
      >
        <div class="flex items-center gap-3 min-w-0 flex-1">
          <div
            class="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 shrink-0"
            :class="{
              'bg-primary/10 text-primary': group.group_type === 'select',
              'bg-secondary/10 text-secondary': group.group_type === 'urltest',
              'bg-accent/10 text-accent': group.group_type === 'fallback',
              'bg-info/10 text-info': group.group_type === 'loadbalance'
            }"
          >
            <CpuChipIcon class="w-5 h-5" />
          </div>
          <div class="min-w-0 flex-1">
            <h3 class="font-bold text-sm sm:text-base truncate">{{ group.name }}</h3>
            <p class="mt-0.5 font-mono text-[11px] opacity-60 truncate">
              ID: {{ group.id }}
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2 flex-shrink-0 shrink-0">
          <StatusBadge
            :label="groupTypeLabel(group.group_type)"
            :tone="'info'"
          />
          <div
            class="w-7 h-7 rounded-lg flex items-center justify-center bg-base-300/60 transition-transform duration-200 ease-out shrink-0"
            :class="{ 'rotate-180': expanded }"
          >
            <ChevronDownIcon class="w-4 h-4" />
          </div>
        </div>
      </div>

      <!-- Edge / Sources Indicator and Action Bar wrap cleanly on narrow widths -->
      <div class="flex flex-wrap items-center justify-between gap-2 text-xs opacity-70 border-t border-base-300 pt-2.5 min-w-0">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <span class="badge badge-xs badge-neutral font-mono font-bold" title="策略组排序序号">
            #{{ group.position ?? 0 }}
          </span>
          <span v-if="sourceCount > 0" class="badge badge-xs badge-outline font-mono">
            {{ sourceCount }} 项来源
          </span>
          <span
            v-if="sourceCount > 0"
            class="badge badge-xs badge-ghost font-mono opacity-80"
            data-testid="group-preview-status"
            title="策略组解析节点需在编辑抽屉中预览，列表不发额外请求"
          >
            {{ uiText('成员: 待预览', 'Members: Pending preview') }}
          </span>
          <span v-if="group.revision !== undefined" class="badge badge-xs badge-ghost font-mono">
            v{{ group.revision }}
          </span>
          <span
            class="badge badge-xs font-semibold"
            :class="membershipMode.badgeClass"
            :title="membershipMode.fullLabel"
            data-testid="group-membership-mode-badge"
          >
            {{ membershipMode.label }}
          </span>
          <span
            v-if="membershipMode.secondaryNotice"
            class="badge badge-xs badge-outline font-mono"
            :title="membershipMode.secondaryNotice"
          >
            同级顺序增减
          </span>
          <span
            class="badge badge-xs badge-ghost font-mono"
            :title="`支持策略组的编译目标：${groupTypeSupportedTargets(group.group_type).join('/')}`"
          >
            目标：{{ groupTypeSupportedTargets(group.group_type).join('/') }}
          </span>
          <span
            v-if="group.inherit_global_filter === false"
            class="badge badge-xs badge-warning font-semibold"
            data-testid="group-standalone-pool-badge"
            title="关闭继承全局过滤，可从全量接入节点中匹配（如 WARP 或专用线路）"
          >
            全池直通
          </span>
          <span v-if="group.empty_fallback_pass" class="badge badge-xs badge-outline" :title="t('policy.emptyFallbackPassHelp')">{{ t('policy.emptyFallbackPass') }}</span>
          <span
            v-if="conditionCount > 0"
            class="badge badge-xs badge-primary font-mono"
            title="各来源内部的匹配条件，不是整组筛选器"
          >
            {{ conditionCount }} 条来源条件
          </span>
        </div>

        <div class="flex flex-wrap items-center gap-1 shrink-0" @click.stop>
          <button
            v-if="canMoveUp"
            type="button"
            class="btn btn-ghost btn-xs gap-1 touch-manipulation"
            title="上移策略组"
            data-testid="group-move-up-btn"
            @click="emit('move-up', group)"
          >
            <ChevronUpIcon class="w-3.5 h-3.5 shrink-0" />
          </button>
          <button
            v-if="canMoveDown"
            type="button"
            class="btn btn-ghost btn-xs gap-1 touch-manipulation"
            title="下移策略组"
            data-testid="group-move-down-btn"
            @click="emit('move-down', group)"
          >
            <ChevronDownIcon class="w-3.5 h-3.5 shrink-0" />
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs gap-1 touch-manipulation"
            title="管理成员来源"
            @click="emit('manage-edges', group)"
          >
            <PlusIcon class="w-3.5 h-3.5 shrink-0" />
            管理来源
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs gap-1 touch-manipulation"
            title="编辑策略组"
            @click="emit('edit', group)"
          >
            <PencilSquareIcon class="w-3.5 h-3.5 shrink-0" />
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs text-error gap-1 touch-manipulation"
            title="删除策略组"
            @click="emit('delete', group.id)"
          >
            <TrashIcon class="w-3.5 h-3.5 shrink-0" />
          </button>
        </div>
      </div>

      <!-- Expandable Edges / Sources Container (Smooth 200ms ease-out) -->
      <div
        v-if="expanded"
        class="mt-1 pt-3 border-t border-base-300/80 space-y-2 transition-all duration-200 ease-out min-w-0 overflow-hidden"
      >
        <!-- Sources Pipeline breakdown when sources present -->
        <div v-if="sourceCount > 0" class="space-y-1.5">
          <div class="flex items-center justify-between text-[11px] font-sans opacity-70">
            <span>有序成员来源流水线（由上至下执行）：</span>
            <span class="badge badge-xs badge-ghost font-mono">{{ sourceCount }} 步</span>
          </div>
          <div
            v-for="(item, sIdx) in sourceSemantics"
            :key="item.source.id || sIdx"
            class="flex items-center justify-between p-2 rounded-lg bg-base-100 border border-base-300/80 text-xs font-mono min-w-0 overflow-hidden gap-2"
          >
            <div class="flex items-center gap-2 min-w-0 flex-1 truncate">
              <span class="badge badge-xs badge-ghost font-semibold shrink-0">#{{ sIdx + 1 }}</span>
              <span
                class="badge badge-xs shrink-0 font-bold"
                :class="item.source.operation === 'remove' ? 'badge-error text-error-content' : 'badge-primary text-primary-content'"
              >
                {{ item.source.operation === 'remove' ? '-' : '+' }}
              </span>
              <span class="badge badge-xs badge-outline shrink-0 font-sans">
                {{ sourceKindLabel(item.source.kind) }}
              </span>
              <span v-if="item.source.kind === 'manual_node'" class="text-primary truncate">
                节点: {{ item.source.node_logical_id }}
              </span>
              <span v-else-if="item.source.kind === 'group_ref'" class="text-secondary truncate">
                组引用: {{ item.source.target_group_id }}
              </span>
              <span v-else-if="item.source.kind === 'group_nodes'" class="text-secondary truncate">
                展开子组: {{ item.source.target_group_id }}
              </span>
              <span v-else-if="item.source.kind === 'regex'" class="text-accent truncate">
                正则: /{{ item.source.pattern }}/
              </span>
              <span v-else-if="item.source.kind === 'filter'" class="text-info truncate">
                过滤条件: {{ item.source.filter_spec?.conditions?.length || 0 }} 条
              </span>
              <span v-else-if="item.source.kind === 'typed_builtin'" class="text-warning truncate">
                内建: {{ item.source.builtin_name }}
              </span>
            </div>
            <span
              v-if="item.isReEntry"
              class="badge badge-xs badge-secondary shrink-0 font-sans"
              title="先前排除后重新加入"
            >
              重入入选
            </span>
          </div>
        </div>

        <div v-if="sourceCount === 0" class="text-xs opacity-50 italic py-1">
          当前策略组尚未配置成员来源，点击「管理来源」添加节点、子策略组或匹配条件。
        </div>
      </div>
    </div>
  </article>
</template>

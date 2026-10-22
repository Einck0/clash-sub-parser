<script setup lang="ts">
import {
  PlusIcon,
  TrashIcon,
  PencilSquareIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  ArrowUpIcon,
  ArrowDownIcon,
  CpuChipIcon,
  FolderIcon,
  ShieldCheckIcon,
  XMarkIcon,
  MagnifyingGlassIcon,
} from '@heroicons/vue/24/outline'
import {
  type PolicyRuleView,
  type RuleCategory,
  parseRuleExpression,
  ruleTypeBadgeClass,
} from './rulesTypes'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'

const props = defineProps<{
  categories: RuleCategory[]
  rulesByCategory: { map: Map<string, PolicyRuleView[]>; uncategorized: PolicyRuleView[] }
  filteredRules: PolicyRuleView[]
  selectedCategoryId: string | null
  isSearching: boolean
  searchQuery: string
  groupNameMap: Map<string, string>
  collapsedCategories: Record<string, boolean>
}>()

const emit = defineEmits<{
  'toggle-collapse': [catId: string]
  'create-rule-for-category': [catId?: string]
  'edit-rule': [rule: PolicyRuleView]
  'delete-rule': [rule: PolicyRuleView]
  'move-rule': [ruleId: string, direction: 'up' | 'down']
  'toggle-enabled': [rule: PolicyRuleView]
  'move-category': [categoryId: string, direction: 'up' | 'down']
  'clear-search': []
}>()

function getCategoryName(categoryId?: string, categoryName?: string): string {
  if (categoryId) {
    const found = props.categories.find((c) => c.id === categoryId)
    if (found) return found.name
  }
  return categoryName || t('rules.uncategorized')
}

function isMoveUpDisabled(rule: PolicyRuleView, list: PolicyRuleView[], idx: number): boolean {
  if (idx === 0) return true
  const parsed = parseRuleExpression(rule.expression)
  if (parsed.isTerminal) return true
  return false
}

function isMoveDownDisabled(rule: PolicyRuleView, list: PolicyRuleView[], idx: number): boolean {
  if (idx === list.length - 1) return true
  const parsed = parseRuleExpression(rule.expression)
  if (parsed.isTerminal) return true
  const nextRule = list[idx + 1]
  if (nextRule && parseRuleExpression(nextRule.expression).isTerminal) {
    return true
  }
  return false
}

function hasTerminalRule(list: PolicyRuleView[]): boolean {
  return list.some((r) => parseRuleExpression(r.expression).isTerminal)
}
</script>

<template>
  <div class="space-y-4">
    <!-- Search Active Notice Banner -->
    <div
      v-if="isSearching"
      class="flex items-center justify-between p-3 rounded-xl bg-base-200/60 border border-base-300/60 text-xs"
    >
      <div class="flex items-center gap-2 min-w-0">
        <MagnifyingGlassIcon class="w-4 h-4 text-primary shrink-0" />
        <span class="truncate">
          {{ uiText('正在全局搜索：', 'Searching globally: ') }}
          <span class="font-semibold text-primary">"{{ searchQuery }}"</span>
          <span class="opacity-60 ml-1">({{ uiText('跨分类结果已显示归属', 'Cross-category results with attribution') }})</span>
        </span>
      </div>
      <button
        type="button"
        class="btn btn-ghost btn-xs gap-1 shrink-0 text-base-content/75 hover:text-base-content"
        @click="$emit('clear-search')"
      >
        <XMarkIcon class="w-3.5 h-3.5" />
        <span>{{ uiText('清除搜索', 'Clear') }}</span>
      </button>
    </div>

    <!-- Mode 1: Search Active (Flat List with Category Attribution) -->
    <template v-if="isSearching">
      <div v-if="filteredRules.length === 0" class="card bg-base-100 border border-base-300/60 p-8 text-center">
        <div class="max-w-md mx-auto space-y-2">
          <p class="font-semibold text-base">{{ t('rules.emptyTitle') }}</p>
          <p class="text-xs opacity-60">{{ t('rules.emptyDesc') }}</p>
          <div class="pt-2">
            <button type="button" class="btn btn-sm btn-ghost" @click="$emit('clear-search')">
              {{ uiText('返回全部规则', 'Back to all rules') }}
            </button>
          </div>
        </div>
      </div>

      <div v-else class="card bg-base-100 border border-base-300/60 shadow-xs overflow-hidden">
        <!-- Desktop Table -->
        <div class="hidden md:block overflow-x-auto">
          <table class="table table-sm w-full">
            <thead>
              <tr class="text-xs uppercase opacity-70 border-b border-base-300/40">
                <th class="w-12 text-center">#</th>
                <th class="w-36">{{ t('rules.ruleType') }}</th>
                <th>{{ t('rules.expression') }}</th>
                <th class="w-40">{{ uiText('归属分类', 'Category') }}</th>
                <th class="w-44">{{ t('rules.target') }}</th>
                <th class="w-16 text-center">{{ t('common.status') }}</th>
                <th class="w-24 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="rule in filteredRules"
                :key="rule.id"
                class="hover:bg-base-200/30 transition-colors"
                :class="{ 'opacity-50': !rule.enabled }"
              >
                <td class="text-center font-mono text-xs opacity-50">{{ rule.position }}</td>
                <td>
                  <span
                    class="badge badge-sm font-semibold tracking-wide whitespace-nowrap h-auto py-0.5 px-2 shrink-0 leading-tight"
                    :class="ruleTypeBadgeClass(parseRuleExpression(rule.expression).type)"
                  >
                    {{ parseRuleExpression(rule.expression).type }}
                  </span>
                </td>
                <td>
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="font-mono text-xs font-semibold break-all">
                      {{ parseRuleExpression(rule.expression).value || rule.expression }}
                    </span>
                    <span v-if="rule.name" class="badge badge-ghost badge-sm text-[11px] opacity-70">
                      {{ rule.name }}
                    </span>
                    <span
                      v-if="parseRuleExpression(rule.expression).noResolve"
                      class="badge badge-outline badge-warning badge-sm text-[10px]"
                    >
                      no-resolve
                    </span>
                  </div>
                </td>
                <td>
                  <span class="badge badge-outline badge-sm gap-1 max-w-[140px] truncate text-xs">
                    <FolderIcon class="w-3 h-3 shrink-0" />
                    <span class="truncate">{{ getCategoryName(rule.category_id, rule.category) }}</span>
                  </span>
                </td>
                <td>
                  <div v-if="rule.builtin_target" class="flex items-center gap-1.5">
                    <span
                      v-if="rule.builtin_target === 'DIRECT'"
                      class="badge badge-success badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                    >
                      DIRECT
                    </span>
                    <span
                      v-else-if="rule.builtin_target === 'REJECT'"
                      class="badge badge-error badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                    >
                      REJECT
                    </span>
                    <span
                      v-else
                      class="badge badge-warning badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                    >
                      {{ rule.builtin_target }}
                    </span>
                  </div>
                  <div v-else-if="rule.target_group_id" class="flex items-center gap-1.5">
                    <span class="badge badge-info badge-sm font-medium gap-1 max-w-[150px] truncate whitespace-nowrap h-auto py-0.5 px-2 shrink-0">
                      <CpuChipIcon class="w-3.5 h-3.5 shrink-0" />
                      <span class="truncate">{{ groupNameMap.get(rule.target_group_id) || rule.target_group_id }}</span>
                    </span>
                  </div>
                  <span v-else class="badge badge-ghost badge-sm text-xs opacity-60">-</span>
                </td>
                <td class="text-center">
                  <input
                    type="checkbox"
                    class="toggle toggle-xs toggle-primary"
                    :checked="rule.enabled"
                    :aria-label="t('common.status')"
                    @change="$emit('toggle-enabled', rule)"
                  />
                </td>
                <td class="text-right">
                  <div class="flex items-center justify-end gap-1">
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square text-info"
                      :title="t('common.edit')"
                      @click="$emit('edit-rule', rule)"
                    >
                      <PencilSquareIcon class="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square text-error"
                      :title="t('common.delete')"
                      @click="$emit('delete-rule', rule)"
                    >
                      <TrashIcon class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Mobile Readable Cards -->
        <div class="block md:hidden divide-y divide-base-300/40 p-2 space-y-2">
          <div
            v-for="rule in filteredRules"
            :key="rule.id"
            class="p-3 rounded-lg bg-base-200/20 border border-base-300/40 space-y-2"
            :class="{ 'opacity-60': !rule.enabled }"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="flex items-center gap-2 flex-wrap min-w-0">
                <span class="font-mono text-xs opacity-50 font-semibold">#{{ rule.position }}</span>
                <span
                  class="badge badge-sm font-semibold tracking-wide whitespace-nowrap h-auto py-0.5 px-2 shrink-0 leading-tight"
                  :class="ruleTypeBadgeClass(parseRuleExpression(rule.expression).type)"
                >
                  {{ parseRuleExpression(rule.expression).type }}
                </span>
                <span
                  v-if="rule.builtin_target"
                  class="badge badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                  :class="rule.builtin_target === 'DIRECT' ? 'badge-success' : rule.builtin_target === 'REJECT' ? 'badge-error' : 'badge-warning'"
                >
                  {{ rule.builtin_target }}
                </span>
                <span
                  v-else-if="rule.target_group_id"
                  class="badge badge-info badge-sm font-medium gap-1 max-w-[130px] truncate whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                >
                  <CpuChipIcon class="w-3 h-3 shrink-0" />
                  <span class="truncate">{{ groupNameMap.get(rule.target_group_id) || rule.target_group_id }}</span>
                </span>
              </div>
              <input
                type="checkbox"
                class="toggle toggle-xs toggle-primary shrink-0"
                :checked="rule.enabled"
                :aria-label="t('common.status')"
                @change="$emit('toggle-enabled', rule)"
              />
            </div>

            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-mono text-xs font-semibold break-all">
                {{ parseRuleExpression(rule.expression).value || rule.expression }}
              </span>
              <span v-if="rule.name" class="badge badge-ghost badge-sm text-[11px] opacity-75">
                {{ rule.name }}
              </span>
              <span
                v-if="parseRuleExpression(rule.expression).noResolve"
                class="badge badge-outline badge-warning badge-sm text-[10px]"
              >
                no-resolve
              </span>
            </div>

            <div class="flex items-center justify-between pt-2 border-t border-base-300/40 text-xs">
              <span class="badge badge-outline badge-xs gap-1 opacity-70 truncate max-w-[160px]">
                <FolderIcon class="w-3 h-3 shrink-0" />
                <span class="truncate">{{ getCategoryName(rule.category_id, rule.category) }}</span>
              </span>
              <div class="flex items-center gap-1 shrink-0">
                <button
                  type="button"
                  class="btn btn-ghost btn-xs btn-square text-info"
                  :title="t('common.edit')"
                  @click="$emit('edit-rule', rule)"
                >
                  <PencilSquareIcon class="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  class="btn btn-ghost btn-xs btn-square text-error"
                  :title="t('common.delete')"
                  @click="$emit('delete-rule', rule)"
                >
                  <TrashIcon class="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Mode 2: Normal Categories Display (All or Selected Category) -->
    <template v-else>
      <div
        v-for="cat in (selectedCategoryId && selectedCategoryId !== 'all' ? categories.filter((c) => c.id === selectedCategoryId) : categories)"
        :key="cat.id"
        class="card bg-base-100 border border-base-300/60 shadow-xs overflow-hidden"
      >
        <!-- Category Accordion Header -->
        <div
          class="flex items-center justify-between px-4 py-3 bg-base-200/40 hover:bg-base-200/70 cursor-pointer select-none border-b border-base-300/40 transition-colors"
          @click="$emit('toggle-collapse', cat.id)"
        >
          <div class="flex flex-wrap items-center gap-2 min-w-0">
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-circle shrink-0"
              :aria-label="collapsedCategories[cat.id] ? 'Expand' : 'Collapse'"
            >
              <ChevronRightIcon v-if="collapsedCategories[cat.id]" class="w-4 h-4" />
              <ChevronDownIcon v-else class="w-4 h-4" />
            </button>

            <span class="font-bold text-sm md:text-base tracking-tight">{{ cat.name }}</span>

            <span class="badge badge-sm badge-neutral gap-1 font-mono text-xs">
              <span>{{ t('rules.categoryWeight') }}</span>
              <span>{{ cat.sort_order }}</span>
            </span>

            <span class="badge badge-sm badge-ghost text-xs">
              {{ t('rules.rulesCountBadge', { count: (rulesByCategory.map.get(cat.id) || []).length }) }}
            </span>
          </div>

          <div class="flex items-center gap-1.5 shrink-0" @click.stop>
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-square"
              :title="t('rules.moveUp')"
              @click="$emit('move-category', cat.id, 'up')"
            >
              <ArrowUpIcon class="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-square"
              :title="t('rules.moveDown')"
              @click="$emit('move-category', cat.id, 'down')"
            >
              <ArrowDownIcon class="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              class="btn btn-primary btn-xs gap-1 ml-1"
              @click="$emit('create-rule-for-category', cat.id)"
            >
              <PlusIcon class="w-3.5 h-3.5" />
              <span class="hidden sm:inline">{{ t('rules.addRuleToCategory') }}</span>
            </button>
          </div>
        </div>

        <!-- Category Rules Content -->
        <div v-show="!collapsedCategories[cat.id]">
          <div
            v-if="!(rulesByCategory.map.get(cat.id) || []).length"
            class="p-6 text-center text-sm opacity-50"
          >
            {{ t('rules.emptyDesc') }}
          </div>

          <div v-else>
            <!-- Desktop Table View -->
            <div class="hidden md:block overflow-x-auto">
              <table class="table table-sm w-full">
                <thead>
                  <tr class="text-xs uppercase opacity-70 border-b border-base-300/40">
                    <th class="w-12 text-center">#</th>
                    <th class="w-36">{{ t('rules.ruleType') }}</th>
                    <th>{{ t('rules.expression') }}</th>
                    <th class="w-48">{{ t('rules.target') }}</th>
                    <th class="w-20 text-center">{{ t('common.status') }}</th>
                    <th class="w-36 text-right">{{ t('common.actions') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(rule, idx) in (rulesByCategory.map.get(cat.id) || [])"
                    :key="rule.id"
                    class="hover:bg-base-200/30 transition-colors"
                    :class="{
                      'opacity-50': !rule.enabled,
                      'bg-base-200/40 font-medium': parseRuleExpression(rule.expression).isTerminal,
                    }"
                  >
                    <td class="text-center font-mono text-xs opacity-50">{{ rule.position }}</td>
                    <td>
                      <span
                        class="badge badge-sm font-semibold tracking-wide whitespace-nowrap h-auto py-0.5 px-2 shrink-0 leading-tight"
                        :class="ruleTypeBadgeClass(parseRuleExpression(rule.expression).type)"
                      >
                        {{ parseRuleExpression(rule.expression).type }}
                      </span>
                    </td>
                    <td>
                      <div class="flex items-center gap-2 flex-wrap">
                        <span class="font-mono text-xs font-semibold">
                          {{ parseRuleExpression(rule.expression).value || rule.expression }}
                        </span>
                        <span v-if="rule.name" class="badge badge-ghost badge-sm text-[11px] opacity-70">
                          {{ rule.name }}
                        </span>
                        <span
                          v-if="parseRuleExpression(rule.expression).noResolve"
                          class="badge badge-outline badge-warning badge-sm text-[10px]"
                        >
                          no-resolve
                        </span>
                      </div>
                    </td>
                    <td>
                      <div v-if="rule.builtin_target" class="flex items-center gap-1.5">
                        <span
                          v-if="rule.builtin_target === 'DIRECT'"
                          class="badge badge-success badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                        >
                          DIRECT
                        </span>
                        <span
                          v-else-if="rule.builtin_target === 'REJECT'"
                          class="badge badge-error badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                        >
                          REJECT
                        </span>
                        <span
                          v-else
                          class="badge badge-warning badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                        >
                          {{ rule.builtin_target }}
                        </span>
                      </div>
                      <div v-else-if="rule.target_group_id" class="flex items-center gap-1.5">
                        <span class="badge badge-info badge-sm font-medium gap-1 max-w-[160px] truncate whitespace-nowrap h-auto py-0.5 px-2 shrink-0">
                          <CpuChipIcon class="w-3.5 h-3.5 shrink-0" />
                          <span class="truncate">{{ groupNameMap.get(rule.target_group_id) || rule.target_group_id }}</span>
                        </span>
                      </div>
                      <span v-else class="badge badge-ghost badge-sm text-xs opacity-60">-</span>
                    </td>
                    <td class="text-center">
                      <input
                        type="checkbox"
                        class="toggle toggle-xs toggle-primary"
                        :checked="rule.enabled"
                        :aria-label="t('common.status')"
                        @change="$emit('toggle-enabled', rule)"
                      />
                    </td>
                    <td class="text-right">
                      <div class="flex items-center justify-end gap-1">
                        <button
                          type="button"
                          class="btn btn-ghost btn-xs btn-square"
                          :title="parseRuleExpression(rule.expression).isTerminal ? uiText('终结规则不可前移', 'Terminal rule cannot move up') : t('rules.moveUp')"
                          :disabled="isMoveUpDisabled(rule, rulesByCategory.map.get(cat.id) || [], idx)"
                          @click="$emit('move-rule', rule.id, 'up')"
                        >
                          <ArrowUpIcon class="w-3.5 h-3.5" />
                        </button>
                        <button
                          type="button"
                          class="btn btn-ghost btn-xs btn-square"
                          :title="t('rules.moveDown')"
                          :disabled="isMoveDownDisabled(rule, rulesByCategory.map.get(cat.id) || [], idx)"
                          @click="$emit('move-rule', rule.id, 'down')"
                        >
                          <ArrowDownIcon class="w-3.5 h-3.5" />
                        </button>
                        <button
                          type="button"
                          class="btn btn-ghost btn-xs btn-square text-info"
                          :title="t('common.edit')"
                          @click="$emit('edit-rule', rule)"
                        >
                          <PencilSquareIcon class="w-3.5 h-3.5" />
                        </button>
                        <button
                          type="button"
                          class="btn btn-ghost btn-xs btn-square text-error"
                          :title="t('common.delete')"
                          @click="$emit('delete-rule', rule)"
                        >
                          <TrashIcon class="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>

            <!-- Mobile Readable Cards View -->
            <div class="block md:hidden divide-y divide-base-300/40 p-2 space-y-2">
              <div
                v-for="(rule, idx) in (rulesByCategory.map.get(cat.id) || [])"
                :key="rule.id"
                class="p-3 rounded-lg bg-base-200/20 border border-base-300/40 space-y-2"
                :class="{
                  'opacity-60': !rule.enabled,
                  'border-primary/30 bg-primary/5': parseRuleExpression(rule.expression).isTerminal,
                }"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="flex items-center gap-2 flex-wrap min-w-0">
                    <span class="font-mono text-xs opacity-50 font-semibold">#{{ rule.position }}</span>
                    <span
                      class="badge badge-sm font-semibold tracking-wide whitespace-nowrap h-auto py-0.5 px-2 shrink-0 leading-tight"
                      :class="ruleTypeBadgeClass(parseRuleExpression(rule.expression).type)"
                    >
                      {{ parseRuleExpression(rule.expression).type }}
                    </span>
                    <span
                      v-if="rule.builtin_target"
                      class="badge badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                      :class="rule.builtin_target === 'DIRECT' ? 'badge-success' : rule.builtin_target === 'REJECT' ? 'badge-error' : 'badge-warning'"
                    >
                      {{ rule.builtin_target }}
                    </span>
                    <span
                      v-else-if="rule.target_group_id"
                      class="badge badge-info badge-sm font-medium gap-1 max-w-[130px] truncate whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                    >
                      <CpuChipIcon class="w-3 h-3 shrink-0" />
                      <span class="truncate">{{ groupNameMap.get(rule.target_group_id) || rule.target_group_id }}</span>
                    </span>
                  </div>
                  <input
                    type="checkbox"
                    class="toggle toggle-xs toggle-primary shrink-0"
                    :checked="rule.enabled"
                    :aria-label="t('common.status')"
                    @change="$emit('toggle-enabled', rule)"
                  />
                </div>

                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-mono text-xs font-semibold break-all">
                    {{ parseRuleExpression(rule.expression).value || rule.expression }}
                  </span>
                  <span v-if="rule.name" class="badge badge-ghost badge-sm text-[11px] opacity-75">
                    {{ rule.name }}
                  </span>
                  <span
                    v-if="parseRuleExpression(rule.expression).noResolve"
                    class="badge badge-outline badge-warning badge-sm text-[10px]"
                  >
                    no-resolve
                  </span>
                </div>

                <div class="flex items-center justify-end pt-2 border-t border-base-300/40 text-xs">
                  <div class="flex items-center gap-1 shrink-0">
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square"
                      :title="parseRuleExpression(rule.expression).isTerminal ? uiText('终结规则不可前移', 'Terminal rule cannot move up') : t('rules.moveUp')"
                      :disabled="isMoveUpDisabled(rule, rulesByCategory.map.get(cat.id) || [], idx)"
                      @click="$emit('move-rule', rule.id, 'up')"
                    >
                      <ArrowUpIcon class="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square"
                      :title="t('rules.moveDown')"
                      :disabled="isMoveDownDisabled(rule, rulesByCategory.map.get(cat.id) || [], idx)"
                      @click="$emit('move-rule', rule.id, 'down')"
                    >
                      <ArrowDownIcon class="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square text-info"
                      :title="t('common.edit')"
                      @click="$emit('edit-rule', rule)"
                    >
                      <PencilSquareIcon class="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square text-error"
                      :title="t('common.delete')"
                      @click="$emit('delete-rule', rule)"
                    >
                      <TrashIcon class="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- Terminal MATCH Tail Info Section (only inside category containing terminal MATCH) -->
            <div
              v-if="hasTerminalRule(rulesByCategory.map.get(cat.id) || [])"
              class="m-3 p-3 rounded-lg bg-base-200/50 border border-base-300/40 text-xs flex items-center gap-2.5 text-base-content/75"
            >
              <ShieldCheckIcon class="w-4 h-4 text-primary shrink-0" />
              <span>{{ t('rules.matchNotice') }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Uncategorized Rules (if any) -->
      <div
        v-if="rulesByCategory.uncategorized.length && (!selectedCategoryId || selectedCategoryId === 'all' || selectedCategoryId === 'uncategorized')"
        class="card bg-base-100 border border-base-300/60 shadow-xs overflow-hidden"
      >
        <div
          class="flex items-center justify-between px-4 py-3 bg-base-200/40 hover:bg-base-200/70 cursor-pointer select-none border-b border-base-300/40 transition-colors"
          @click="$emit('toggle-collapse', 'uncategorized')"
        >
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-circle"
              :aria-label="collapsedCategories['uncategorized'] ? 'Expand' : 'Collapse'"
            >
              <ChevronRightIcon v-if="collapsedCategories['uncategorized']" class="w-4 h-4" />
              <ChevronDownIcon v-else class="w-4 h-4" />
            </button>
            <span class="font-bold text-sm md:text-base opacity-80">{{ t('rules.uncategorized') }}</span>
            <span class="badge badge-sm badge-ghost">
              {{ t('rules.rulesCountBadge', { count: rulesByCategory.uncategorized.length }) }}
            </span>
          </div>
        </div>

        <div v-show="!collapsedCategories['uncategorized']">
          <!-- Desktop Table -->
          <div class="hidden md:block overflow-x-auto">
            <table class="table table-sm w-full">
              <thead>
                <tr class="text-xs uppercase opacity-70 border-b border-base-300/40">
                  <th class="w-12 text-center">#</th>
                  <th class="w-36">{{ t('rules.ruleType') }}</th>
                  <th>{{ t('rules.expression') }}</th>
                  <th class="w-48">{{ t('rules.target') }}</th>
                  <th class="w-20 text-center">{{ t('common.status') }}</th>
                  <th class="w-36 text-right">{{ t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(rule, idx) in rulesByCategory.uncategorized"
                  :key="rule.id"
                  class="hover:bg-base-200/30 transition-colors"
                  :class="{ 'opacity-50': !rule.enabled }"
                >
                  <td class="text-center font-mono text-xs opacity-50">{{ rule.position }}</td>
                  <td>
                    <span
                      class="badge badge-sm font-semibold tracking-wide whitespace-nowrap h-auto py-0.5 px-2 shrink-0 leading-tight"
                      :class="ruleTypeBadgeClass(parseRuleExpression(rule.expression).type)"
                    >
                      {{ parseRuleExpression(rule.expression).type }}
                    </span>
                  </td>
                  <td>
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-mono text-xs font-semibold">{{ parseRuleExpression(rule.expression).value || rule.expression }}</span>
                      <span v-if="rule.name" class="badge badge-ghost badge-sm text-[11px] opacity-70">
                        {{ rule.name }}
                      </span>
                    </div>
                  </td>
                  <td>
                    <div v-if="rule.builtin_target" class="flex items-center gap-1.5">
                      <span
                        class="badge badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                        :class="rule.builtin_target === 'DIRECT' ? 'badge-success' : rule.builtin_target === 'REJECT' ? 'badge-error' : 'badge-warning'"
                      >
                        {{ rule.builtin_target }}
                      </span>
                    </div>
                    <div v-else-if="rule.target_group_id" class="flex items-center gap-1.5">
                      <span class="badge badge-info badge-sm font-medium gap-1 max-w-[160px] truncate whitespace-nowrap h-auto py-0.5 px-2 shrink-0">
                        <CpuChipIcon class="w-3.5 h-3.5 shrink-0" />
                        <span class="truncate">{{ groupNameMap.get(rule.target_group_id) || rule.target_group_id }}</span>
                      </span>
                    </div>
                  </td>
                  <td class="text-center">
                    <input
                      type="checkbox"
                      class="toggle toggle-xs toggle-primary"
                      :checked="rule.enabled"
                      :aria-label="t('common.status')"
                      @change="$emit('toggle-enabled', rule)"
                    />
                  </td>
                  <td class="text-right">
                    <div class="flex items-center justify-end gap-1">
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square"
                        :title="t('rules.moveUp')"
                        :disabled="isMoveUpDisabled(rule, rulesByCategory.uncategorized, idx)"
                        @click="$emit('move-rule', rule.id, 'up')"
                      >
                        <ArrowUpIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square"
                        :title="t('rules.moveDown')"
                        :disabled="isMoveDownDisabled(rule, rulesByCategory.uncategorized, idx)"
                        @click="$emit('move-rule', rule.id, 'down')"
                      >
                        <ArrowDownIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square text-info"
                        :title="t('common.edit')"
                        @click="$emit('edit-rule', rule)"
                      >
                        <PencilSquareIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square text-error"
                        :title="t('common.delete')"
                        @click="$emit('delete-rule', rule)"
                      >
                        <TrashIcon class="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Mobile Cards -->
          <div class="block md:hidden divide-y divide-base-300/40 p-2 space-y-2">
            <div
              v-for="(rule, idx) in rulesByCategory.uncategorized"
              :key="rule.id"
              class="p-3 rounded-lg bg-base-200/20 border border-base-300/40 space-y-2"
              :class="{ 'opacity-60': !rule.enabled }"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="flex items-center gap-2 flex-wrap min-w-0">
                  <span class="font-mono text-xs opacity-50 font-semibold">#{{ rule.position }}</span>
                  <span
                    class="badge badge-sm font-semibold tracking-wide whitespace-nowrap h-auto py-0.5 px-2 shrink-0 leading-tight"
                    :class="ruleTypeBadgeClass(parseRuleExpression(rule.expression).type)"
                  >
                    {{ parseRuleExpression(rule.expression).type }}
                  </span>
                  <span
                    v-if="rule.builtin_target"
                    class="badge badge-sm font-semibold whitespace-nowrap h-auto py-0.5 px-2 shrink-0"
                    :class="rule.builtin_target === 'DIRECT' ? 'badge-success' : 'badge-warning'"
                  >
                    {{ rule.builtin_target }}
                  </span>
                </div>
                <input
                  type="checkbox"
                  class="toggle toggle-xs toggle-primary shrink-0"
                  :checked="rule.enabled"
                  :aria-label="t('common.status')"
                  @change="$emit('toggle-enabled', rule)"
                />
              </div>

              <div class="flex items-center gap-2 flex-wrap">
                <span class="font-mono text-xs font-semibold break-all">
                  {{ parseRuleExpression(rule.expression).value || rule.expression }}
                </span>
                <span v-if="rule.name" class="badge badge-ghost badge-sm text-[11px] opacity-75">
                  {{ rule.name }}
                </span>
              </div>

              <div class="flex items-center justify-end pt-2 border-t border-base-300/40 text-xs">
                <div class="flex items-center gap-1 shrink-0">
                  <button
                    type="button"
                    class="btn btn-ghost btn-xs btn-square"
                    :title="t('rules.moveUp')"
                    :disabled="isMoveUpDisabled(rule, rulesByCategory.uncategorized, idx)"
                    @click="$emit('move-rule', rule.id, 'up')"
                  >
                    <ArrowUpIcon class="w-3.5 h-3.5" />
                  </button>
                  <button
                    type="button"
                    class="btn btn-ghost btn-xs btn-square"
                    :title="t('rules.moveDown')"
                    :disabled="isMoveDownDisabled(rule, rulesByCategory.uncategorized, idx)"
                    @click="$emit('move-rule', rule.id, 'down')"
                  >
                    <ArrowDownIcon class="w-3.5 h-3.5" />
                  </button>
                  <button
                    type="button"
                    class="btn btn-ghost btn-xs btn-square text-info"
                    :title="t('common.edit')"
                    @click="$emit('edit-rule', rule)"
                  >
                    <PencilSquareIcon class="w-3.5 h-3.5" />
                  </button>
                  <button
                    type="button"
                    class="btn btn-ghost btn-xs btn-square text-error"
                    :title="t('common.delete')"
                    @click="$emit('delete-rule', rule)"
                  >
                    <TrashIcon class="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

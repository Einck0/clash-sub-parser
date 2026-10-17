<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  PlusIcon,
  TrashIcon,
  PencilSquareIcon,
  ArrowPathIcon,
  ShieldCheckIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  ArrowUpIcon,
  ArrowDownIcon,
  MagnifyingGlassIcon,
  CpuChipIcon,
  FolderIcon,
  XMarkIcon,
  Bars3BottomLeftIcon,
} from '@heroicons/vue/24/outline'
import { useRules } from './useRules'
import {
  COMMON_RULE_TYPES,
  type PolicyRuleView,
  type RuleCategory,
  type CommonRuleType,
} from './rulesTypes'
import ModalDialog from '../../ui/ModalDialog.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'

const {
  categories,
  sortedCategories,
  rules,
  filteredRules,
  rulesByCategory,
  groups,
  groupNameMap,
  loading,
  saving,
  validating,
  error,
  searchQuery,
  validationResult,
  totalRulesCount,
  totalCategoriesCount,
  loadData,
  createCategory,
  updateCategory,
  deleteCategory,
  moveCategory,
  createRule,
  updateRule,
  deleteRule,
  toggleRuleEnabled,
  moveRule,
  backfillRules,
  validateRules,
} = useRules()

onMounted(async () => {
  try {
    await loadData()
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
})

// Accordion collapse state per category ID
const collapsedCategories = ref<Record<string, boolean>>({})

function toggleCategoryCollapse(catId: string) {
  collapsedCategories.value[catId] = !collapsedCategories.value[catId]
}

function expandAll() {
  collapsedCategories.value = {}
}

function collapseAll() {
  const map: Record<string, boolean> = {}
  for (const cat of categories.value) {
    map[cat.id] = true
  }
  map['uncategorized'] = true
  collapsedCategories.value = map
}

function isCollapsed(catId: string): boolean {
  return !!collapsedCategories.value[catId]
}

// Helpers for expression parsing
function parseExpression(expr: string) {
  const parts = expr.split(',').map((p) => p.trim())
  const type = (parts[0] || 'MATCH').toUpperCase()
  const value = parts.slice(1).filter((p) => p.toLowerCase() !== 'no-resolve').join(',')
  const noResolve = parts.some((p) => p.toLowerCase() === 'no-resolve')
  return { type, value, noResolve }
}

function ruleTypeBadgeClass(type: string): string {
  const upper = type.toUpperCase()
  if (upper.startsWith('DOMAIN')) {
    return 'badge-primary text-primary-content'
  }
  if (upper.startsWith('IP-CIDR')) {
    return 'badge-secondary text-secondary-content'
  }
  if (upper.startsWith('GEO')) {
    return 'badge-accent text-accent-content'
  }
  if (upper === 'MATCH' || upper === 'FINAL') {
    return 'badge-neutral text-neutral-content'
  }
  if (upper.startsWith('PROCESS')) {
    return 'badge-info text-info-content'
  }
  return 'badge-ghost'
}

// Rule Dialog State
const ruleDialogOpen = ref(false)
const isEditingRule = ref(false)
const editingRuleId = ref<string | null>(null)

const ruleForm = ref({
  category_id: '',
  name: '',
  rule_type: 'DOMAIN-SUFFIX' as CommonRuleType,
  rule_value: '',
  target_kind: 'builtin' as 'builtin' | 'group',
  builtin_target: 'DIRECT',
  target_group_id: '',
  no_resolve: false,
})

function openCreateRuleModal(categoryId?: string) {
  isEditingRule.value = false
  editingRuleId.value = null
  ruleForm.value = {
    category_id: categoryId || (categories.value[0]?.id ?? ''),
    name: '',
    rule_type: 'DOMAIN-SUFFIX',
    rule_value: '',
    target_kind: 'builtin',
    builtin_target: 'DIRECT',
    target_group_id: groups.value[0]?.id ?? '',
    no_resolve: false,
  }
  ruleDialogOpen.value = true
}

function openEditRuleModal(rule: PolicyRuleView) {
  isEditingRule.value = true
  editingRuleId.value = rule.id

  const parsed = parseExpression(rule.expression)
  const isMatch = parsed.type === 'MATCH' || parsed.type === 'FINAL'
  const ruleType = isMatch ? 'MATCH' : ((COMMON_RULE_TYPES.find((t) => t === parsed.type) || 'DOMAIN-SUFFIX') as CommonRuleType)

  const isBuiltin = !!rule.builtin_target || !rule.target_group_id

  ruleForm.value = {
    category_id: rule.category_id || '',
    name: rule.name || '',
    rule_type: ruleType,
    rule_value: parsed.value,
    target_kind: isBuiltin ? 'builtin' : 'group',
    builtin_target: rule.builtin_target || 'DIRECT',
    target_group_id: rule.target_group_id || (groups.value[0]?.id ?? ''),
    no_resolve: parsed.noResolve,
  }
  ruleDialogOpen.value = true
}

async function saveRuleForm() {
  const { rule_type, rule_value, no_resolve, target_kind, builtin_target, target_group_id, category_id, name } = ruleForm.value

  let expression = ''
  if (rule_type === 'MATCH') {
    expression = 'MATCH'
  } else {
    expression = `${rule_type},${rule_value.trim()}`
    if (no_resolve) {
      expression += ',no-resolve'
    }
  }

  const payload: any = {
    category_id: category_id || undefined,
    name: name.trim() || undefined,
    expression,
    builtin_target: target_kind === 'builtin' ? builtin_target : '',
    target_group_id: target_kind === 'group' ? target_group_id : '',
  }

  try {
    if (isEditingRule.value && editingRuleId.value) {
      await updateRule(editingRuleId.value, payload)
      toastStore.push({ message: t('common.success'), tone: 'success' })
    } else {
      await createRule(payload)
      toastStore.push({ message: t('common.success'), tone: 'success' })
    }
    ruleDialogOpen.value = false
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
}

// Delete Rule confirmation
const deleteRuleConfirmOpen = ref(false)
const ruleToDelete = ref<PolicyRuleView | null>(null)

function confirmDeleteRule(rule: PolicyRuleView) {
  ruleToDelete.value = rule
  deleteRuleConfirmOpen.value = true
}

async function handleDeleteRule() {
  if (!ruleToDelete.value) return
  try {
    await deleteRule(ruleToDelete.value.id)
    toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  } finally {
    deleteRuleConfirmOpen.value = false
    ruleToDelete.value = null
  }
}

// Category Management Modal
const categoryModalOpen = ref(false)
const newCatName = ref('')
const newCatSortOrder = ref(100)

const editingCategory = ref<RuleCategory | null>(null)
const editCatName = ref('')
const editCatSortOrder = ref(0)
const editCategoryDialogOpen = ref(false)

function openCategoryModal() {
  categoryModalOpen.value = true
}

function openEditCategory(cat: RuleCategory) {
  editingCategory.value = cat
  editCatName.value = cat.name
  editCatSortOrder.value = cat.sort_order
  editCategoryDialogOpen.value = true
}

async function saveEditCategory() {
  if (!editingCategory.value) return
  try {
    await updateCategory(editingCategory.value.id, {
      name: editCatName.value.trim(),
      sort_order: Number(editCatSortOrder.value),
    })
    toastStore.push({ message: t('common.success'), tone: 'success' })
    editCategoryDialogOpen.value = false
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
}

async function handleCreateCategory() {
  if (!newCatName.value.trim()) return
  try {
    await createCategory({
      name: newCatName.value.trim(),
      sort_order: Number(newCatSortOrder.value) || 100,
    })
    toastStore.push({ message: t('common.success'), tone: 'success' })
    newCatName.value = ''
    newCatSortOrder.value = (sortedCategories.value[sortedCategories.value.length - 1]?.sort_order ?? 100) + 10
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
}

// Delete Category confirmation
const deleteCatConfirmOpen = ref(false)
const categoryToDelete = ref<RuleCategory | null>(null)

function confirmDeleteCategory(cat: RuleCategory) {
  categoryToDelete.value = cat
  deleteCatConfirmOpen.value = true
}

async function handleDeleteCategory() {
  if (!categoryToDelete.value) return
  try {
    await deleteCategory(categoryToDelete.value.id)
    toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  } finally {
    deleteCatConfirmOpen.value = false
    categoryToDelete.value = null
  }
}

// Backfill confirmation
const backfillConfirmOpen = ref(false)

function confirmBackfill() {
  backfillConfirmOpen.value = true
}

async function handleBackfill() {
  try {
    const res = await backfillRules()
    toastStore.push({
      message: t('rules.backfillSuccess', { count: res.count }),
      tone: 'success',
    })
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  } finally {
    backfillConfirmOpen.value = false
  }
}

// Validate Rules Action
async function handleValidate() {
  try {
    const res = await validateRules()
    if (res.valid) {
      toastStore.push({ message: t('rules.topologyValid'), tone: 'success' })
    } else {
      toastStore.push({ message: t('rules.topologyInvalid'), tone: 'warning' })
    }
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
}
</script>

<template>
  <div class="space-y-6 pb-24 md:pb-8">
    <!-- Top Action Bar -->
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2">
          <span class="badge badge-primary badge-sm font-semibold tracking-wide uppercase">{{ t('rules.tag') }}</span>
          <span v-if="loading" class="loading loading-spinner loading-xs text-primary"></span>
        </div>
        <h1 class="text-2xl md:text-3xl font-bold tracking-tight mt-1">{{ t('rules.title') }}</h1>
        <p class="text-sm opacity-70 mt-1 max-w-3xl leading-relaxed">{{ t('rules.subtitle') }}</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="btn btn-sm btn-outline gap-1.5"
          :disabled="saving"
          @click="confirmBackfill"
        >
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': saving }" />
          <span>{{ t('rules.backfillRules') }}</span>
        </button>

        <button
          type="button"
          class="btn btn-sm btn-outline gap-1.5"
          :disabled="validating"
          @click="handleValidate"
        >
          <ShieldCheckIcon class="w-4 h-4 text-success" />
          <span>{{ t('rules.validateRules') }}</span>
        </button>

        <button
          type="button"
          class="btn btn-sm btn-outline gap-1.5"
          @click="openCategoryModal"
        >
          <FolderIcon class="w-4 h-4" />
          <span>{{ t('rules.manageCategories') }}</span>
        </button>

        <button
          type="button"
          class="btn btn-sm btn-primary gap-1.5"
          @click="openCreateRuleModal()"
        >
          <PlusIcon class="w-4 h-4" />
          <span>{{ t('rules.newRule') }}</span>
        </button>
      </div>
    </div>

    <!-- Stats and Search Filter Bar -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      <div class="card bg-base-200/60 border border-base-300/40 p-4 flex items-center justify-between">
        <div>
          <div class="text-xs uppercase tracking-wider opacity-70">{{ t('rules.totalRules') }}</div>
          <div class="text-2xl font-bold mt-1 text-primary">{{ totalRulesCount }}</div>
        </div>
        <Bars3BottomLeftIcon class="w-8 h-8 opacity-20 text-primary" />
      </div>

      <div class="card bg-base-200/60 border border-base-300/40 p-4 flex items-center justify-between">
        <div>
          <div class="text-xs uppercase tracking-wider opacity-70">{{ t('rules.totalCategories') }}</div>
          <div class="text-2xl font-bold mt-1 text-secondary">{{ totalCategoriesCount }}</div>
        </div>
        <FolderIcon class="w-8 h-8 opacity-20 text-secondary" />
      </div>

      <!-- Search Input Container -->
      <div class="card bg-base-200/60 border border-base-300/40 p-3 sm:col-span-2 flex justify-center">
        <div class="relative w-full">
          <MagnifyingGlassIcon class="w-5 h-5 absolute left-3 top-1/2 -translate-y-1/2 opacity-50" />
          <input
            v-model="searchQuery"
            type="text"
            class="input input-sm input-bordered w-full pl-10 pr-9 bg-base-100"
            :placeholder="t('rules.searchPlaceholder')"
          />
          <button
            v-if="searchQuery"
            type="button"
            class="absolute right-2.5 top-1/2 -translate-y-1/2 btn btn-ghost btn-xs btn-circle"
            @click="searchQuery = ''"
          >
            <XMarkIcon class="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>

    <!-- Topology Validation Banner -->
    <div
      v-if="validationResult"
      class="alert p-4 rounded-xl border flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 shadow-sm"
      :class="validationResult.valid ? 'alert-success/15 border-success/30 text-success-content' : 'alert-warning/15 border-warning/30 text-warning-content'"
    >
      <div class="flex items-start gap-3">
        <CheckCircleIcon v-if="validationResult.valid" class="w-6 h-6 text-success shrink-0 mt-0.5" />
        <ExclamationTriangleIcon v-else class="w-6 h-6 text-warning shrink-0 mt-0.5" />
        <div>
          <h4 class="font-semibold text-sm">
            {{ validationResult.valid ? t('rules.topologyValid') : t('rules.topologyInvalid') }}
          </h4>
          <p class="text-xs opacity-80 mt-0.5">
            {{ validationResult.valid ? t('rules.topologyValidDesc') : (validationResult.errors?.join(', ') || t('rules.topologyInvalid')) }}
          </p>
        </div>
      </div>
      <button
        type="button"
        class="btn btn-xs btn-ghost self-end sm:self-center"
        @click="validationResult = null"
      >
        <XMarkIcon class="w-4 h-4" />
      </button>
    </div>

    <!-- Notice about MATCH rule -->
    <div class="flex items-center gap-2 p-3 text-xs rounded-lg bg-base-200/50 border border-base-300/30 opacity-80">
      <ShieldCheckIcon class="w-4 h-4 text-primary shrink-0" />
      <span>{{ t('rules.matchNotice') }}</span>
    </div>

    <!-- Expand / Collapse All Controls -->
    <div class="flex items-center justify-between text-xs opacity-75 px-1">
      <div>{{ t('rules.rulesCountBadge', { count: filteredRules.length }) }}</div>
      <div class="flex items-center gap-2">
        <button type="button" class="btn btn-ghost btn-xs" @click="expandAll">
          {{ t('rules.expandAll') }}
        </button>
        <span>·</span>
        <button type="button" class="btn btn-ghost btn-xs" @click="collapseAll">
          {{ t('rules.collapseAll') }}
        </button>
      </div>
    </div>

    <!-- Categories Accordion List -->
    <div class="space-y-4">
      <div
        v-for="cat in sortedCategories"
        :key="cat.id"
        class="card bg-base-100 border border-base-300/60 shadow-sm overflow-hidden"
      >
        <!-- Category Header -->
        <div
          class="flex items-center justify-between px-4 py-3.5 bg-base-200/40 hover:bg-base-200/70 cursor-pointer select-none border-b border-base-300/40 transition-colors"
          @click="toggleCategoryCollapse(cat.id)"
        >
          <div class="flex flex-wrap items-center gap-2 min-w-0">
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-circle shrink-0"
              :aria-label="isCollapsed(cat.id) ? 'Expand' : 'Collapse'"
            >
              <ChevronRightIcon v-if="isCollapsed(cat.id)" class="w-4 h-4" />
              <ChevronDownIcon v-else class="w-4 h-4" />
            </button>

            <span class="font-bold text-base tracking-tight">{{ cat.name }}</span>

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
              @click="moveCategory(cat.id, 'up')"
            >
              <ArrowUpIcon class="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-square"
              :title="t('rules.moveDown')"
              @click="moveCategory(cat.id, 'down')"
            >
              <ArrowDownIcon class="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              class="btn btn-primary btn-xs gap-1 ml-1"
              @click="openCreateRuleModal(cat.id)"
            >
              <PlusIcon class="w-3.5 h-3.5" />
              <span class="hidden sm:inline">{{ t('rules.addRuleToCategory') }}</span>
            </button>
          </div>
        </div>

        <!-- Category Rules Table / List -->
        <div v-show="!isCollapsed(cat.id)">
          <div
            v-if="!(rulesByCategory.map.get(cat.id) || []).length"
            class="p-6 text-center text-sm opacity-50"
          >
            {{ t('rules.emptyDesc') }}
          </div>

          <div v-else class="overflow-x-auto">
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
                  v-for="rule in (rulesByCategory.map.get(cat.id) || [])"
                  :key="rule.id"
                  class="hover:bg-base-200/30 transition-colors"
                >
                  <td class="text-center font-mono text-xs opacity-50">
                    {{ rule.position }}
                  </td>
                  <td>
                    <span
                      class="badge badge-sm font-semibold tracking-wide"
                      :class="ruleTypeBadgeClass(parseExpression(rule.expression).type)"
                    >
                      {{ parseExpression(rule.expression).type }}
                    </span>
                  </td>
                  <td>
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="font-mono text-xs font-medium">{{ parseExpression(rule.expression).value || rule.expression }}</span>
                      <span
                        v-if="rule.name"
                        class="badge badge-ghost badge-sm text-[11px] opacity-70"
                      >
                        {{ rule.name }}
                      </span>
                      <span
                        v-if="parseExpression(rule.expression).noResolve"
                        class="badge badge-outline badge-warning badge-sm text-[10px]"
                      >
                        no-resolve
                      </span>
                    </div>
                  </td>
                  <td>
                    <!-- Target Badge -->
                    <div v-if="rule.builtin_target" class="flex items-center gap-1.5">
                      <span
                        v-if="rule.builtin_target === 'DIRECT'"
                        class="badge badge-success badge-sm font-semibold gap-1"
                      >
                        DIRECT
                      </span>
                      <span
                        v-else-if="rule.builtin_target === 'REJECT'"
                        class="badge badge-error badge-sm font-semibold gap-1"
                      >
                        REJECT
                      </span>
                      <span
                        v-else
                        class="badge badge-warning badge-sm font-semibold gap-1"
                      >
                        {{ rule.builtin_target }}
                      </span>
                    </div>
                    <div v-else-if="rule.target_group_id" class="flex items-center gap-1.5">
                      <span class="badge badge-info badge-sm font-medium gap-1 max-w-[160px] truncate">
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
                      @change="toggleRuleEnabled(rule)"
                    />
                  </td>
                  <td class="text-right">
                    <div class="flex items-center justify-end gap-1">
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square"
                        :title="t('rules.moveUp')"
                        @click="moveRule(rule.id, 'up')"
                      >
                        <ArrowUpIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square"
                        :title="t('rules.moveDown')"
                        @click="moveRule(rule.id, 'down')"
                      >
                        <ArrowDownIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square text-info"
                        :title="t('common.edit')"
                        @click="openEditRuleModal(rule)"
                      >
                        <PencilSquareIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square text-error"
                        :title="t('common.delete')"
                        @click="confirmDeleteRule(rule)"
                      >
                        <TrashIcon class="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Uncategorized Rules (if any) -->
      <div
        v-if="rulesByCategory.uncategorized.length"
        class="card bg-base-100 border border-base-300/60 shadow-sm overflow-hidden"
      >
        <div
          class="flex items-center justify-between px-4 py-3 bg-base-200/40 hover:bg-base-200/70 cursor-pointer select-none border-b border-base-300/40 transition-colors"
          @click="toggleCategoryCollapse('uncategorized')"
        >
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-ghost btn-xs btn-circle">
              <ChevronRightIcon v-if="isCollapsed('uncategorized')" class="w-4 h-4" />
              <ChevronDownIcon v-else class="w-4 h-4" />
            </button>
            <span class="font-bold text-base opacity-80">{{ t('rules.uncategorized') }}</span>
            <span class="badge badge-sm badge-ghost">
              {{ t('rules.rulesCountBadge', { count: rulesByCategory.uncategorized.length }) }}
            </span>
          </div>
        </div>

        <div v-show="!isCollapsed('uncategorized')">
          <div class="overflow-x-auto">
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
                  v-for="rule in rulesByCategory.uncategorized"
                  :key="rule.id"
                  class="hover:bg-base-200/30 transition-colors"
                >
                  <td class="text-center font-mono text-xs opacity-50">{{ rule.position }}</td>
                  <td>
                    <span
                      class="badge badge-sm font-semibold tracking-wide"
                      :class="ruleTypeBadgeClass(parseExpression(rule.expression).type)"
                    >
                      {{ parseExpression(rule.expression).type }}
                    </span>
                  </td>
                  <td>
                    <div class="flex items-center gap-2">
                      <span class="font-mono text-xs">{{ parseExpression(rule.expression).value || rule.expression }}</span>
                      <span v-if="rule.name" class="badge badge-ghost badge-sm text-[11px] opacity-70">{{ rule.name }}</span>
                    </div>
                  </td>
                  <td>
                    <span v-if="rule.builtin_target" class="badge badge-success badge-sm font-semibold">
                      {{ rule.builtin_target }}
                    </span>
                    <span v-else-if="rule.target_group_id" class="badge badge-info badge-sm font-medium">
                      {{ groupNameMap.get(rule.target_group_id) || rule.target_group_id }}
                    </span>
                  </td>
                  <td class="text-center">
                    <input
                      type="checkbox"
                      class="toggle toggle-xs toggle-primary"
                      :checked="rule.enabled"
                      @change="toggleRuleEnabled(rule)"
                    />
                  </td>
                  <td class="text-right">
                    <div class="flex items-center justify-end gap-1">
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square text-info"
                        @click="openEditRuleModal(rule)"
                      >
                        <PencilSquareIcon class="w-3.5 h-3.5" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-xs btn-square text-error"
                        @click="confirmDeleteRule(rule)"
                      >
                        <TrashIcon class="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Create / Edit Rule Modal -->
    <ModalDialog
      v-model="ruleDialogOpen"
      :title="isEditingRule ? t('rules.editRule') : t('rules.newRule')"
    >
      <form class="space-y-4" @submit.prevent="saveRuleForm">
        <!-- Category selector -->
        <div>
          <label class="label label-text font-semibold text-xs py-1">{{ t('rules.categoryLabel') }}</label>
          <select v-model="ruleForm.category_id" class="select select-bordered select-sm w-full">
            <option
              v-for="cat in sortedCategories"
              :key="cat.id"
              :value="cat.id"
            >
              {{ cat.name }} ({{ t('rules.categoryWeight') }}: {{ cat.sort_order }})
            </option>
          </select>
        </div>

        <!-- Rule Name (display tag) -->
        <div>
          <label class="label label-text font-semibold text-xs py-1">{{ t('rules.ruleName') }}</label>
          <input
            v-model="ruleForm.name"
            type="text"
            class="input input-bordered input-sm w-full"
            :placeholder="t('rules.ruleNamePlaceholder')"
          />
        </div>

        <!-- Rule Type and Value -->
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label class="label label-text font-semibold text-xs py-1">{{ t('rules.ruleType') }}</label>
            <select v-model="ruleForm.rule_type" class="select select-bordered select-sm w-full">
              <option v-for="t in COMMON_RULE_TYPES" :key="t" :value="t">{{ t }}</option>
            </select>
          </div>

          <div class="sm:col-span-2">
            <label class="label label-text font-semibold text-xs py-1">{{ t('rules.expression') }}</label>
            <input
              v-if="ruleForm.rule_type !== 'MATCH'"
              v-model="ruleForm.rule_value"
              type="text"
              class="input input-bordered input-sm w-full font-mono text-xs"
              :placeholder="t('rules.expressionPlaceholder')"
              required
            />
            <div
              v-else
              class="input input-bordered input-sm w-full flex items-center bg-base-200/50 text-xs font-mono opacity-80"
            >
              MATCH (Terminal Catch-all)
            </div>
          </div>
        </div>

        <!-- No Resolve Checkbox -->
        <div v-if="ruleForm.rule_type !== 'MATCH'" class="flex items-center gap-2">
          <input
            id="rule-no-resolve"
            v-model="ruleForm.no_resolve"
            type="checkbox"
            class="checkbox checkbox-xs checkbox-primary"
          />
          <label for="rule-no-resolve" class="text-xs cursor-pointer select-none">
            {{ t('rules.noResolve') }}
          </label>
        </div>

        <!-- Target Selector (Segmented Radio) -->
        <div class="pt-2 border-t border-base-300/40">
          <label class="label label-text font-semibold text-xs py-1">{{ t('rules.target') }}</label>

          <div class="flex items-center gap-4 mb-3">
            <label class="flex items-center gap-2 text-xs cursor-pointer">
              <input
                v-model="ruleForm.target_kind"
                type="radio"
                value="builtin"
                class="radio radio-xs radio-primary"
              />
              <span>{{ t('rules.builtinTarget') }}</span>
            </label>

            <label class="flex items-center gap-2 text-xs cursor-pointer">
              <input
                v-model="ruleForm.target_kind"
                type="radio"
                value="group"
                class="radio radio-xs radio-primary"
              />
              <span>{{ t('rules.policyGroup') }}</span>
            </label>
          </div>

          <!-- Builtin Action Select -->
          <div v-if="ruleForm.target_kind === 'builtin'">
            <select v-model="ruleForm.builtin_target" class="select select-bordered select-sm w-full">
              <option value="DIRECT">{{ t('rules.directDesc') }}</option>
              <option value="REJECT">{{ t('rules.rejectDesc') }}</option>
              <option value="PASS">{{ t('rules.passDesc') }}</option>
            </select>
          </div>

          <!-- Policy Group Select -->
          <div v-else>
            <select v-model="ruleForm.target_group_id" class="select select-bordered select-sm w-full">
              <option v-for="g in groups" :key="g.id" :value="g.id">
                {{ g.name }} ({{ g.group_type || 'group' }})
              </option>
            </select>
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <button type="button" class="btn btn-sm btn-ghost" @click="ruleDialogOpen = false">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-sm btn-primary"
            :disabled="saving"
            @click="saveRuleForm"
          >
            {{ t('common.save') }}
          </button>
        </div>
      </template>
    </ModalDialog>

    <!-- Category Management Modal -->
    <ModalDialog
      v-model="categoryModalOpen"
      :title="t('rules.categoryModalTitle')"
      :description="t('rules.categoryModalDesc')"
    >
      <div class="space-y-4">
        <!-- New Category Input Form -->
        <div class="flex flex-col sm:flex-row gap-2 p-3 rounded-lg bg-base-200/50 border border-base-300/40">
          <input
            v-model="newCatName"
            type="text"
            class="input input-bordered input-sm flex-1 text-xs"
            :placeholder="t('rules.categoryNamePlaceholder')"
          />
          <input
            v-model.number="newCatSortOrder"
            type="number"
            class="input input-bordered input-sm w-24 text-xs font-mono"
            :placeholder="t('rules.categoryWeight')"
          />
          <button
            type="button"
            class="btn btn-sm btn-primary gap-1"
            :disabled="!newCatName.trim() || saving"
            @click="handleCreateCategory"
          >
            <PlusIcon class="w-3.5 h-3.5" />
            <span>{{ t('common.save') }}</span>
          </button>
        </div>

        <!-- Categories List -->
        <div class="divide-y divide-base-300/40 border border-base-300/40 rounded-lg overflow-hidden max-h-96 overflow-y-auto">
          <div
            v-for="cat in sortedCategories"
            :key="cat.id"
            class="flex items-center justify-between p-3 hover:bg-base-200/30 transition-colors"
          >
            <div class="flex items-center gap-2 min-w-0">
              <span class="badge badge-sm badge-neutral font-mono text-xs">{{ cat.sort_order }}</span>
              <span class="font-medium text-sm truncate">{{ cat.name }}</span>
            </div>

            <div class="flex items-center gap-1 shrink-0">
              <button
                type="button"
                class="btn btn-ghost btn-xs btn-square"
                :title="t('rules.moveUp')"
                @click="moveCategory(cat.id, 'up')"
              >
                <ArrowUpIcon class="w-3.5 h-3.5" />
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-xs btn-square"
                :title="t('rules.moveDown')"
                @click="moveCategory(cat.id, 'down')"
              >
                <ArrowDownIcon class="w-3.5 h-3.5" />
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-xs btn-square text-info"
                :title="t('common.edit')"
                @click="openEditCategory(cat)"
              >
                <PencilSquareIcon class="w-3.5 h-3.5" />
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-xs btn-square text-error"
                :title="t('common.delete')"
                @click="confirmDeleteCategory(cat)"
              >
                <TrashIcon class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end w-full">
          <button type="button" class="btn btn-sm btn-ghost" @click="categoryModalOpen = false">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </ModalDialog>

    <!-- Edit Category Modal -->
    <ModalDialog
      v-model="editCategoryDialogOpen"
      :title="t('rules.editCategory')"
    >
      <form class="space-y-3" @submit.prevent="saveEditCategory">
        <div>
          <label class="label label-text font-semibold text-xs py-1">{{ t('rules.categoryName') }}</label>
          <input
            v-model="editCatName"
            type="text"
            class="input input-bordered input-sm w-full text-xs"
            required
          />
        </div>
        <div>
          <label class="label label-text font-semibold text-xs py-1">{{ t('rules.categorySortOrder') }}</label>
          <input
            v-model.number="editCatSortOrder"
            type="number"
            class="input input-bordered input-sm w-full text-xs font-mono"
            required
          />
          <p class="text-[11px] opacity-60 mt-1">{{ t('rules.categorySortOrderHint') }}</p>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <button type="button" class="btn btn-sm btn-ghost" @click="editCategoryDialogOpen = false">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-sm btn-primary" :disabled="saving" @click="saveEditCategory">
            {{ t('common.save') }}
          </button>
        </div>
      </template>
    </ModalDialog>

    <!-- Delete Rule Confirm Modal -->
    <ConfirmModal
      v-model="deleteRuleConfirmOpen"
      :title="t('rules.deleteRuleConfirmTitle')"
      :message="t('rules.deleteRuleConfirmDesc')"
      tone="danger"
      :loading="saving"
      @confirm="handleDeleteRule"
      @cancel="deleteRuleConfirmOpen = false"
    />

    <!-- Delete Category Confirm Modal -->
    <ConfirmModal
      v-model="deleteCatConfirmOpen"
      :title="t('rules.deleteCategoryConfirmTitle')"
      :message="t('rules.deleteCategoryConfirmDesc')"
      tone="danger"
      :loading="saving"
      @confirm="handleDeleteCategory"
      @cancel="deleteCatConfirmOpen = false"
    />

    <!-- Backfill Rules Confirm Modal -->
    <ConfirmModal
      v-model="backfillConfirmOpen"
      :title="t('rules.backfillConfirmTitle')"
      :message="t('rules.backfillConfirmDesc')"
      tone="warning"
      :loading="saving"
      @confirm="handleBackfill"
      @cancel="backfillConfirmOpen = false"
    />
  </div>
</template>

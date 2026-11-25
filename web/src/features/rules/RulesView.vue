<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  ShieldCheckIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  XMarkIcon,
} from '@heroicons/vue/24/outline'
import { useRules } from './useRules'
import type {
  PolicyRuleView,
  RuleCategory,
  CreateRulePayload,
  UpdateRulePayload,
  CreateCategoryPayload,
  UpdateCategoryPayload,
} from './rulesTypes'
import RuleCategoryNav from './RuleCategoryNav.vue'
import RuleToolbar from './RuleToolbar.vue'
import RuleList from './RuleList.vue'
import RuleEditorModal from './RuleEditorModal.vue'
import RuleCategoryModal from './RuleCategoryModal.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import { displayText } from '../../utils/displayNames'
import { uiText } from '../../locales/redesign'

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
  refreshRequired,
  searchQuery,
  selectedCategoryId,
  validationResult,
  totalRulesCount,
  totalCategoriesCount,
  categoryRulesCountMap,
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
  syncToUrl,
} = useRules()

const ruleSaveError = ref('')
const categorySaveError = ref('')
const actionError = ref('')
const actionNotice = ref('')

async function handleToggleRule(rule: PolicyRuleView) {
  if (saving.value) return
  actionError.value = ''
  actionNotice.value = ''
  const enabled = !rule.enabled
  try {
    await toggleRuleEnabled(rule)
    validationResult.value = null
    if (!refreshRequired.value) actionNotice.value = enabled
      ? uiText('规则已启用，后续实时导出会使用它。', 'Rule enabled; live exports will use it.')
      : uiText('规则已停用，仍保留在列表中，但不参与实际导出。', 'Rule disabled; it remains editable here but is excluded from exports.')
  } catch (err) {
    actionError.value = displayText(err instanceof Error ? err.message : t('common.error'))
  }
}
async function handleMoveRule(id: string, direction: 'up' | 'down') {
  try { await moveRule(id, direction); validationResult.value = null }
  catch (err) { actionError.value = displayText(err instanceof Error ? err.message : t('common.error')) }
}
async function handleMoveCategory(id: string, direction: 'up' | 'down') {
  categorySaveError.value = ''
  try { await moveCategory(id, direction) }
  catch (err) {
    categorySaveError.value = displayText(err instanceof Error ? err.message : t('common.error'))
    actionError.value = categorySaveError.value
  }
}

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

// URL Sync on query/category changes
watch([searchQuery, selectedCategoryId], () => {
  syncToUrl()
})

// Cross-category search should not be truncated by a previously active category
watch(searchQuery, (newQ) => {
  if (newQ.trim()) {
    selectedCategoryId.value = null
  }
})

// If user explicitly chooses a category while search is active, clear search query to show that category
watch(selectedCategoryId, (newCat) => {
  if (newCat && newCat !== 'all' && searchQuery.value) {
    searchQuery.value = ''
  }
})

// Category accordion collapse state
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

const isSearching = computed(() => !!searchQuery.value.trim())

// Rule Editor Dialog
const ruleModalOpen = ref(false)
const editingRule = ref<PolicyRuleView | null>(null)
const defaultCategoryIdForNew = ref<string | undefined>(undefined)

function openCreateRuleModal(categoryId?: string) {
  if (refreshRequired.value || loading.value || saving.value) return
  ruleSaveError.value = ''
  editingRule.value = null
  defaultCategoryIdForNew.value = categoryId || selectedCategoryId.value || categories.value[0]?.id
  ruleModalOpen.value = true
}

function openEditRuleModal(rule: PolicyRuleView) {
  if (refreshRequired.value || loading.value || saving.value) return
  ruleSaveError.value = ''
  editingRule.value = rule
  ruleModalOpen.value = true
}

async function handleSaveRule(payload: CreateRulePayload | UpdateRulePayload) {
  if (saving.value) return
  ruleSaveError.value = ''
  try {
    if (editingRule.value) {
      await updateRule(editingRule.value.id, payload)
      if (!refreshRequired.value) toastStore.push({ message: t('common.success'), tone: 'success' })
    } else {
      await createRule(payload as CreateRulePayload)
      if (!refreshRequired.value) toastStore.push({ message: t('common.success'), tone: 'success' })
    }
    ruleModalOpen.value = false
    editingRule.value = null
    validationResult.value = null
  } catch (err) {
    ruleSaveError.value = displayText(err instanceof Error ? err.message : t('common.error'))
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
    deleteRuleConfirmOpen.value = false
    ruleToDelete.value = null
    validationResult.value = null
    if (!refreshRequired.value) toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
}

// Category Management Modal
const categoryModalOpen = ref(false)
const categoryModal = ref<{
  createSucceeded: () => void
  updateSucceeded: () => void
  deleteSucceeded: () => void
} | null>(null)

function openCategoryModal() {
  categorySaveError.value = ''
  categoryModalOpen.value = true
}

async function handleCreateCategory(payload: CreateCategoryPayload) {
  categorySaveError.value = ''
  try {
    await createCategory(payload)
    categoryModal.value?.createSucceeded()
    if (!refreshRequired.value) toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    categorySaveError.value = displayText(err instanceof Error ? err.message : t('common.error'))
  }
}

async function handleUpdateCategory(id: string, payload: UpdateCategoryPayload) {
  categorySaveError.value = ''
  try {
    await updateCategory(id, payload)
    categoryModal.value?.updateSucceeded()
    if (!refreshRequired.value) toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    categorySaveError.value = displayText(err instanceof Error ? err.message : t('common.error'))
  }
}

async function handleDeleteCategory(cat: RuleCategory) {
  categorySaveError.value = ''
  try {
    await deleteCategory(cat.id)
    categoryModal.value?.deleteSucceeded()
    if (!refreshRequired.value) toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    categorySaveError.value = displayText(err instanceof Error ? err.message : t('common.error'))
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
    if (!refreshRequired.value) toastStore.push({
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

// Topology validation
async function handleValidateRules() {
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

async function retryLoad() {
  try { await loadData() } catch { /* The persistent read error stays visible. */ }
}

function handleClearSearch() {
  searchQuery.value = ''
  selectedCategoryId.value = null
}
</script>

<template>
  <div class="space-y-4 pb-24 md:pb-8" data-testid="rules-root">
    <h2 id="rules-title" class="sr-only">{{ t('rules.title') }}</h2>

    <!-- Unified Page Toolbar (Search + Small Counts + Primary New Rule + More Actions) -->
    <RuleToolbar
      v-model:search-query="searchQuery"
      :total-rules-count="totalRulesCount"
      :total-categories-count="totalCategoriesCount"
      :filtered-count="filteredRules.length"
      :is-searching="isSearching"
      :loading="loading"
      :saving="saving || refreshRequired"
      :validating="validating"
      @new-rule="openCreateRuleModal()"
      @manage-categories="openCategoryModal"
      @validate-rules="handleValidateRules"
      @backfill-rules="confirmBackfill"
      @expand-all="expandAll"
      @collapse-all="collapseAll"
      @clear-search="handleClearSearch"
    />

    <p v-if="actionNotice" role="status" class="alert alert-success text-sm break-words" data-testid="rule-action-notice">{{ actionNotice }}</p>
    <p v-if="actionError" role="alert" class="alert alert-error text-sm break-words" data-testid="rule-action-error">{{ actionError }}</p>

    <!-- Topology Validation Banner (if executed) -->
    <div
      v-if="validationResult"
      class="alert p-3.5 rounded-xl border flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 shadow-xs"
      :class="validationResult.valid ? 'alert-success/15 border-success/30 text-success-content' : 'alert-warning/15 border-warning/30 text-warning-content'"
    >
      <div class="flex items-start gap-2.5">
        <CheckCircleIcon v-if="validationResult.valid" class="w-5 h-5 text-success shrink-0 mt-0.5" />
        <ExclamationTriangleIcon v-else class="w-5 h-5 text-warning shrink-0 mt-0.5" />
        <div>
          <h4 class="font-semibold text-xs md:text-sm">
            {{ validationResult.valid ? t('rules.topologyValid') : t('rules.topologyInvalid') }}
          </h4>
          <p class="text-xs opacity-80 mt-0.5">
            {{ validationResult.valid ? t('rules.topologyValidDesc') : displayText(validationResult.errors?.join(', ') || t('rules.topologyInvalid')) }}
          </p>
        </div>
      </div>
      <button
        type="button"
        class="btn btn-xs btn-ghost self-end sm:self-center"
        aria-label="Dismiss banner"
        @click="validationResult = null"
      >
        <XMarkIcon class="w-4 h-4" />
      </button>
    </div>

    <!-- Error Banner (if initial load failed and no cached rules exist) -->
    <div
      v-if="error"
      class="alert alert-error/15 border border-error/30 text-error-content p-4 rounded-xl flex flex-wrap items-center justify-between gap-2"
      role="alert"
    >
      <div class="flex min-w-0 items-center gap-2">
        <ExclamationTriangleIcon class="w-5 h-5 text-error shrink-0" />
        <span class="text-xs whitespace-pre-wrap [overflow-wrap:anywhere]">{{ displayText(error) }}</span>
      </div>
      <button type="button" class="btn btn-xs btn-outline" :disabled="loading || saving" @click="retryLoad">
        {{ t('common.retry') || 'Retry' }}
      </button>
    </div>

    <!-- Main Content Area: Responsive Category Nav + Rules List -->
    <div class="grid grid-cols-1 md:grid-cols-12 gap-4 items-start">
      <!-- Category Navigation (Desktop Sidebar / Mobile Select) -->
      <aside class="md:col-span-4 lg:col-span-3">
        <RuleCategoryNav
          v-model:selected-category-id="selectedCategoryId"
          :categories="sortedCategories"
          :category-counts="categoryRulesCountMap"
          :total-count="totalRulesCount"
        />
      </aside>

      <!-- Rules List & Tail Section -->
      <main class="md:col-span-8 lg:col-span-9 min-w-0">
        <RuleList
          :categories="sortedCategories"
          :rules-by-category="rulesByCategory"
          :filtered-rules="filteredRules"
          :selected-category-id="selectedCategoryId"
          :is-searching="isSearching"
          :search-query="searchQuery"
          :group-name-map="groupNameMap"
          :collapsed-categories="collapsedCategories"
          :busy="saving || loading || refreshRequired"
          @toggle-collapse="toggleCategoryCollapse"
          @create-rule-for-category="openCreateRuleModal"
          @edit-rule="openEditRuleModal"
          @delete-rule="confirmDeleteRule"
          @move-rule="handleMoveRule"
          @toggle-enabled="handleToggleRule"
          @move-category="handleMoveCategory"
          @clear-search="handleClearSearch"
        />
      </main>
    </div>

    <!-- Rule Editor Modal -->
    <RuleEditorModal
      v-model="ruleModalOpen"
      :rule="editingRule"
      :default-category-id="defaultCategoryIdForNew"
      :categories="sortedCategories"
      :groups="groups"
      :saving="saving"
      :error="ruleSaveError"
      @save="handleSaveRule"
    />

    <!-- Category Management Modal -->
    <RuleCategoryModal
      ref="categoryModal"
      v-model="categoryModalOpen"
      :categories="sortedCategories"
      :saving="saving"
      :blocked="loading || refreshRequired"
      :error="categorySaveError || error || ''"
      @retry="retryLoad"
      @create-category="handleCreateCategory"
      @update-category="handleUpdateCategory"
      @delete-category="handleDeleteCategory"
      @move-category="handleMoveCategory"
    />

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

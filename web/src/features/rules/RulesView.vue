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
  editingRule.value = null
  defaultCategoryIdForNew.value = categoryId || selectedCategoryId.value || categories.value[0]?.id
  ruleModalOpen.value = true
}

function openEditRuleModal(rule: PolicyRuleView) {
  editingRule.value = rule
  ruleModalOpen.value = true
}

async function handleSaveRule(payload: CreateRulePayload | UpdateRulePayload) {
  try {
    if (editingRule.value) {
      await updateRule(editingRule.value.id, payload)
      toastStore.push({ message: t('common.success'), tone: 'success' })
    } else {
      await createRule(payload as CreateRulePayload)
      toastStore.push({ message: t('common.success'), tone: 'success' })
    }
    ruleModalOpen.value = false
    editingRule.value = null
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

function openCategoryModal() {
  categoryModalOpen.value = true
}

async function handleCreateCategory(payload: CreateCategoryPayload) {
  try {
    await createCategory(payload)
    toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
}

async function handleUpdateCategory(id: string, payload: UpdateCategoryPayload) {
  try {
    await updateCategory(id, payload)
    toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
  }
}

async function handleDeleteCategory(cat: RuleCategory) {
  try {
    await deleteCategory(cat.id)
    toastStore.push({ message: t('common.success'), tone: 'success' })
  } catch (err) {
    toastStore.push({
      message: err instanceof Error ? err.message : t('common.error'),
      tone: 'error',
    })
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

function handleClearSearch() {
  searchQuery.value = ''
}
</script>

<template>
  <div class="space-y-4 pb-24 md:pb-8">
    <!-- Unified Page Toolbar (Search + Small Counts + Primary New Rule + More Actions) -->
    <RuleToolbar
      v-model:search-query="searchQuery"
      :total-rules-count="totalRulesCount"
      :total-categories-count="totalCategoriesCount"
      :filtered-count="filteredRules.length"
      :is-searching="isSearching"
      :loading="loading"
      :saving="saving"
      :validating="validating"
      @new-rule="openCreateRuleModal()"
      @manage-categories="openCategoryModal"
      @validate-rules="handleValidateRules"
      @backfill-rules="confirmBackfill"
      @expand-all="expandAll"
      @collapse-all="collapseAll"
    />

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
            {{ validationResult.valid ? t('rules.topologyValidDesc') : (validationResult.errors?.join(', ') || t('rules.topologyInvalid')) }}
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
      v-if="error && rules.length === 0"
      class="alert alert-error/15 border border-error/30 text-error-content p-4 rounded-xl flex items-center justify-between"
    >
      <div class="flex items-center gap-2">
        <ExclamationTriangleIcon class="w-5 h-5 text-error shrink-0" />
        <span class="text-xs">{{ error }}</span>
      </div>
      <button type="button" class="btn btn-xs btn-outline" @click="loadData">
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
          @toggle-collapse="toggleCategoryCollapse"
          @create-rule-for-category="openCreateRuleModal"
          @edit-rule="openEditRuleModal"
          @delete-rule="confirmDeleteRule"
          @move-rule="moveRule"
          @toggle-enabled="toggleRuleEnabled"
          @move-category="moveCategory"
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
      @save="handleSaveRule"
    />

    <!-- Category Management Modal -->
    <RuleCategoryModal
      v-model="categoryModalOpen"
      :categories="sortedCategories"
      :saving="saving"
      @create-category="handleCreateCategory"
      @update-category="handleUpdateCategory"
      @delete-category="handleDeleteCategory"
      @move-category="moveCategory"
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

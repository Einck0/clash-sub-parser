<script setup lang="ts">
import { ref } from 'vue'
import {
  PlusIcon,
  TrashIcon,
  PencilSquareIcon,
  ArrowUpIcon,
  ArrowDownIcon,
} from '@heroicons/vue/24/outline'
import ModalDialog from '../../ui/ModalDialog.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import type {
  RuleCategory,
  CreateCategoryPayload,
  UpdateCategoryPayload,
} from './rulesTypes'
import { t } from '../../locales'

const props = defineProps<{
  modelValue: boolean
  categories: RuleCategory[]
  saving: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [open: boolean]
  'create-category': [payload: CreateCategoryPayload]
  'update-category': [id: string, payload: UpdateCategoryPayload]
  'delete-category': [category: RuleCategory]
  'move-category': [categoryId: string, direction: 'up' | 'down']
}>()

// Create state
const newCatName = ref('')
const newCatSortOrder = ref(100)

function handleCreate() {
  if (!newCatName.value.trim()) return
  emit('create-category', {
    name: newCatName.value.trim(),
    sort_order: Number(newCatSortOrder.value) || 100,
  })
  newCatName.value = ''
  newCatSortOrder.value = (props.categories[props.categories.length - 1]?.sort_order ?? 100) + 10
}

// Edit sub-modal state
const editCategoryDialogOpen = ref(false)
const editingCategory = ref<RuleCategory | null>(null)
const editCatName = ref('')
const editCatSortOrder = ref(0)

function openEditCategory(cat: RuleCategory) {
  editingCategory.value = cat
  editCatName.value = cat.name
  editCatSortOrder.value = cat.sort_order
  editCategoryDialogOpen.value = true
}

function saveEditCategory() {
  if (!editingCategory.value || !editCatName.value.trim()) return
  emit('update-category', editingCategory.value.id, {
    name: editCatName.value.trim(),
    sort_order: Number(editCatSortOrder.value) || 0,
  })
  editCategoryDialogOpen.value = false
  editingCategory.value = null
}

// Delete confirm modal state
const deleteCatConfirmOpen = ref(false)
const categoryToDelete = ref<RuleCategory | null>(null)

function confirmDeleteCategory(cat: RuleCategory) {
  categoryToDelete.value = cat
  deleteCatConfirmOpen.value = true
}

function executeDeleteCategory() {
  if (!categoryToDelete.value) return
  emit('delete-category', categoryToDelete.value)
  deleteCatConfirmOpen.value = false
  categoryToDelete.value = null
}
</script>

<template>
  <div>
    <!-- Main Categories Management Dialog -->
    <ModalDialog
      :model-value="modelValue"
      :title="t('rules.categoryModalTitle')"
      :description="t('rules.categoryModalDesc')"
      @update:model-value="$emit('update:modelValue', $event)"
    >
      <div class="space-y-4">
        <!-- New Category Input Form -->
        <div class="flex flex-col sm:flex-row gap-2 p-3 rounded-lg bg-base-200/50 border border-base-300/40">
          <label for="new-category-name" class="sr-only">{{ t('rules.categoryNamePlaceholder') }}</label>
          <input
            id="new-category-name"
            v-model="newCatName"
            type="text"
            class="input input-sm border border-base-300 flex-1 text-xs"
            :placeholder="t('rules.categoryNamePlaceholder')"
          />
          <label for="new-category-sort-order" class="sr-only">{{ t('rules.categoryWeight') }}</label>
          <input
            id="new-category-sort-order"
            v-model.number="newCatSortOrder"
            type="number"
            class="input input-sm border border-base-300 w-24 text-xs font-mono"
            :placeholder="t('rules.categoryWeight')"
          />
          <button
            type="button"
            class="btn btn-sm btn-primary gap-1"
            :disabled="!newCatName.trim() || saving"
            @click="handleCreate"
          >
            <PlusIcon class="w-3.5 h-3.5" />
            <span>{{ t('common.save') }}</span>
          </button>
        </div>

        <!-- Categories List -->
        <div class="divide-y divide-base-300/40 border border-base-300/40 rounded-lg overflow-hidden max-h-96 overflow-y-auto">
          <div
            v-for="cat in categories"
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
          <button
            type="button"
            class="btn btn-sm btn-ghost"
            @click="$emit('update:modelValue', false)"
          >
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </ModalDialog>

    <!-- Edit Category Sub-Dialog -->
    <ModalDialog
      v-model="editCategoryDialogOpen"
      :title="t('rules.editCategory')"
    >
      <form class="space-y-3.5" @submit.prevent="saveEditCategory">
        <div class="space-y-1.5">
          <label for="edit-category-name" class="block text-xs font-semibold text-base-content/80">
            {{ t('rules.categoryName') }}
          </label>
          <input
            id="edit-category-name"
            v-model="editCatName"
            type="text"
            class="input input-sm border border-base-300 w-full text-xs rounded-lg"
            required
          />
        </div>
        <div class="space-y-1.5">
          <label for="edit-category-sort" class="block text-xs font-semibold text-base-content/80">
            {{ t('rules.categorySortOrder') }}
          </label>
          <input
            id="edit-category-sort"
            v-model.number="editCatSortOrder"
            type="number"
            class="input input-sm border border-base-300 w-full text-xs font-mono rounded-lg"
            required
          />
          <p class="text-[11px] text-base-content/60 mt-1">{{ t('rules.categorySortOrderHint') }}</p>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <button
            type="button"
            class="btn btn-sm btn-ghost"
            @click="editCategoryDialogOpen = false"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-sm btn-primary"
            :disabled="saving"
            @click="saveEditCategory"
          >
            {{ t('common.save') }}
          </button>
        </div>
      </template>
    </ModalDialog>

    <!-- Delete Category Confirm Modal -->
    <ConfirmModal
      v-model="deleteCatConfirmOpen"
      :title="t('rules.deleteCategoryConfirmTitle')"
      :message="t('rules.deleteCategoryConfirmDesc')"
      tone="danger"
      :loading="saving"
      @confirm="executeDeleteCategory"
      @cancel="deleteCatConfirmOpen = false"
    />
  </div>
</template>

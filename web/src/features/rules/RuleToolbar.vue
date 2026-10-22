<script setup lang="ts">
import {
  PlusIcon,
  MagnifyingGlassIcon,
  XMarkIcon,
  EllipsisVerticalIcon,
  ShieldCheckIcon,
  ArrowPathIcon,
  FolderIcon,
  ChevronDownIcon,
  ChevronRightIcon,
} from '@heroicons/vue/24/outline'
import PageToolbar from '../../ui/PageToolbar.vue'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'

defineProps<{
  searchQuery: string
  totalRulesCount: number
  totalCategoriesCount: number
  filteredCount: number
  isSearching: boolean
  loading: boolean
  saving: boolean
  validating: boolean
}>()

const emit = defineEmits<{
  'update:searchQuery': [query: string]
  'new-rule': []
  'manage-categories': []
  'validate-rules': []
  'backfill-rules': []
  'expand-all': []
  'collapse-all': []
}>()

function onSearchInput(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:searchQuery', target.value)
}

function clearSearch() {
  emit('update:searchQuery', '')
}
</script>

<template>
  <PageToolbar :title="t('rules.title')" sr-only-title>
    <!-- Default Slot: Search & Small Counts in one cohesive bar -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-3 w-full max-w-xl">
      <!-- Search Input -->
      <div class="relative flex-1 min-w-[200px]">
        <label for="rules-search-input" class="sr-only">{{ t('rules.searchPlaceholder') }}</label>
        <MagnifyingGlassIcon class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-base-content/40 pointer-events-none" />
        <input
          id="rules-search-input"
          :value="searchQuery"
          type="text"
          class="input input-sm border border-base-300 w-full pl-9 pr-8 text-xs bg-base-100 focus:border-primary"
          :placeholder="t('rules.searchPlaceholder')"
          @input="onSearchInput"
        />
        <button
          v-if="searchQuery"
          type="button"
          class="btn btn-ghost btn-xs btn-circle absolute right-1.5 top-1/2 -translate-y-1/2 text-base-content/60 hover:text-base-content"
          aria-label="Clear search"
          @click="clearSearch"
        >
          <XMarkIcon class="w-3.5 h-3.5" />
        </button>
      </div>

      <!-- Small Inline Counts -->
      <div class="flex items-center gap-2 text-xs text-base-content/70 shrink-0 select-none px-1">
        <span v-if="loading" class="loading loading-spinner loading-xs text-primary"></span>
        <template v-else-if="isSearching">
          <span class="badge badge-sm badge-neutral font-mono">
            {{ uiText(`找到 ${filteredCount} 条规则`, `Found ${filteredCount} rules`) }}
          </span>
        </template>
        <template v-else>
          <span class="font-medium">
            {{ uiText(`${totalRulesCount} 规则`, `${totalRulesCount} rules`) }}
          </span>
          <span class="opacity-40">·</span>
          <span class="font-medium">
            {{ uiText(`${totalCategoriesCount} 分类`, `${totalCategoriesCount} categories`) }}
          </span>
        </template>
      </div>
    </div>

    <!-- Actions Slot: Main Action Prominent + More Dropdown -->
    <template #actions>
      <div class="flex items-center gap-2">
        <!-- Prominent Primary Action: New Rule -->
        <button
          type="button"
          class="btn btn-sm btn-primary gap-1.5 shadow-xs"
          @click="$emit('new-rule')"
        >
          <PlusIcon class="w-4 h-4" />
          <span>{{ t('rules.newRule') }}</span>
        </button>

        <!-- More Actions Dropdown -->
        <div class="dropdown dropdown-end">
          <button
            type="button"
            tabindex="0"
            role="button"
            class="btn btn-sm btn-outline btn-square text-base-content/75"
            :title="uiText('更多操作', 'More Actions')"
          >
            <EllipsisVerticalIcon class="w-4 h-4" />
          </button>
          <ul
            tabindex="0"
            class="dropdown-content z-20 menu p-1.5 shadow-lg bg-base-100 rounded-box border border-base-300/60 w-52 text-xs space-y-0.5"
          >
            <li>
              <button
                type="button"
                :disabled="validating"
                class="flex items-center gap-2 py-2"
                @click="$emit('validate-rules')"
              >
                <ShieldCheckIcon class="w-4 h-4 text-success" />
                <span>{{ t('rules.validateRules') }}</span>
              </button>
            </li>
            <li>
              <button
                type="button"
                :disabled="saving"
                class="flex items-center gap-2 py-2"
                @click="$emit('backfill-rules')"
              >
                <ArrowPathIcon class="w-4 h-4 text-warning" :class="{ 'animate-spin': saving }" />
                <span>{{ t('rules.backfillRules') }}</span>
              </button>
            </li>
            <li>
              <button
                type="button"
                class="flex items-center gap-2 py-2"
                @click="$emit('manage-categories')"
              >
                <FolderIcon class="w-4 h-4 text-info" />
                <span>{{ t('rules.manageCategories') }}</span>
              </button>
            </li>
            <div class="divider my-0.5 opacity-40"></div>
            <li>
              <button
                type="button"
                class="flex items-center gap-2 py-1.5"
                @click="$emit('expand-all')"
              >
                <ChevronDownIcon class="w-3.5 h-3.5 opacity-60" />
                <span>{{ t('rules.expandAll') }}</span>
              </button>
            </li>
            <li>
              <button
                type="button"
                class="flex items-center gap-2 py-1.5"
                @click="$emit('collapse-all')"
              >
                <ChevronRightIcon class="w-3.5 h-3.5 opacity-60" />
                <span>{{ t('rules.collapseAll') }}</span>
              </button>
            </li>
          </ul>
        </div>
      </div>
    </template>
  </PageToolbar>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { FolderIcon, Squares2X2Icon } from '@heroicons/vue/24/outline'
import type { RuleCategory } from './rulesTypes'
import { uiText } from '../../locales/redesign'

const props = defineProps<{
  categories: RuleCategory[]
  selectedCategoryId: string | null
  categoryCounts: Map<string, number>
  totalCount: number
}>()

const emit = defineEmits<{
  'update:selectedCategoryId': [id: string | null]
}>()

const isAllSelected = computed(() => {
  return !props.selectedCategoryId || props.selectedCategoryId === 'all'
})

function selectCategory(id: string | null) {
  emit('update:selectedCategoryId', id)
}

function onMobileSelectChange(event: Event) {
  const target = event.target as HTMLSelectElement
  const val = target.value
  emit('update:selectedCategoryId', val === 'all' ? null : val)
}
</script>

<template>
  <div>
    <!-- Mobile Category Select Dropdown -->
    <div class="block md:hidden">
      <label for="rules-mobile-category-select" class="sr-only">{{ uiText('选择分类', 'Select Category') }}</label>
      <div class="relative">
        <select
          id="rules-mobile-category-select"
          class="select select-sm border border-base-300 w-full font-medium bg-base-100 pr-8"
          :value="selectedCategoryId || 'all'"
          @change="onMobileSelectChange"
        >
          <option value="all">
            {{ uiText('全部规则', 'All Rules') }} ({{ totalCount }})
          </option>
          <option
            v-for="cat in categories"
            :key="cat.id"
            :value="cat.id"
          >
            {{ cat.name }} ({{ categoryCounts.get(cat.id) || 0 }})
          </option>
        </select>
      </div>
    </div>

    <!-- Desktop Sidebar Navigation -->
    <div class="hidden md:block">
      <div class="card bg-base-100 border border-base-300/60 shadow-xs overflow-hidden">
        <div class="p-3 border-b border-base-300/40 bg-base-200/30">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold uppercase tracking-wider text-base-content/70">
              {{ uiText('规则分类', 'Categories') }}
            </span>
            <span class="badge badge-sm badge-ghost text-xs">
              {{ categories.length }}
            </span>
          </div>
        </div>

        <nav class="p-1.5 space-y-0.5 max-h-[calc(100vh-14rem)] overflow-y-auto overscroll-contain">
          <!-- All Categories Option -->
          <button
            type="button"
            class="w-full flex items-center justify-between gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-colors text-left"
            :class="isAllSelected ? 'bg-primary text-primary-content font-semibold shadow-xs' : 'hover:bg-base-200/60 text-base-content/85'"
            @click="selectCategory(null)"
          >
            <div class="flex items-center gap-2 min-w-0">
              <Squares2X2Icon class="w-4 h-4 shrink-0" :class="isAllSelected ? 'text-primary-content' : 'text-base-content/60'" />
              <span class="truncate">{{ uiText('全部规则', 'All Rules') }}</span>
            </div>
            <span
              class="badge badge-sm shrink-0"
              :class="isAllSelected ? 'badge-neutral bg-black/20 text-primary-content border-none' : 'badge-ghost text-base-content/70'"
            >
              {{ totalCount }}
            </span>
          </button>

          <!-- Individual Category Items -->
          <button
            v-for="cat in categories"
            :key="cat.id"
            type="button"
            class="w-full flex items-center justify-between gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-colors text-left group"
            :class="selectedCategoryId === cat.id ? 'bg-primary text-primary-content font-semibold shadow-xs' : 'hover:bg-base-200/60 text-base-content/85'"
            @click="selectCategory(cat.id)"
          >
            <div class="flex items-center gap-2 min-w-0">
              <FolderIcon
                class="w-4 h-4 shrink-0"
                :class="selectedCategoryId === cat.id ? 'text-primary-content' : 'text-base-content/60 group-hover:text-base-content'"
              />
              <span class="truncate">{{ cat.name }}</span>
            </div>
            <div class="flex items-center gap-1.5 shrink-0">
              <span
                class="text-[10px] font-mono opacity-50"
                :class="selectedCategoryId === cat.id ? 'text-primary-content' : 'text-base-content/50'"
              >
                #{{ cat.sort_order }}
              </span>
              <span
                class="badge badge-sm"
                :class="selectedCategoryId === cat.id ? 'badge-neutral bg-black/20 text-primary-content border-none' : 'badge-ghost text-base-content/70'"
              >
                {{ categoryCounts.get(cat.id) || 0 }}
              </span>
            </div>
          </button>
        </nav>
      </div>
    </div>
  </div>
</template>

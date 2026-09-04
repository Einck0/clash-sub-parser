<template>
  <div ref="rootRef" class="relative w-full inline-block">
    <!-- Trigger Button -->
    <button
      type="button"
      role="combobox"
      :aria-expanded="isOpen ? 'true' : 'false'"
      aria-haspopup="listbox"
      :aria-controls="listboxId"
      :disabled="disabled"
      :class="[
        'w-full flex items-center justify-between rounded-md border bg-surface px-3 py-2 text-xs sm:text-sm text-text-main transition-colors min-h-[44px] sm:min-h-[36px] cursor-pointer text-left',
        'focus:outline-hidden focus:ring-1',
        mono ? 'font-mono tabular-nums' : '',
        error
          ? 'border-status-danger focus:border-status-danger focus:ring-status-danger'
          : isOpen
            ? 'border-accent focus:border-accent ring-1 ring-accent'
            : 'border-border-subtle hover:border-border-strong focus:border-accent focus:ring-accent',
        disabled ? 'cursor-not-allowed opacity-60 bg-surface-base' : '',
      ]"
      @click="toggleOpen"
      @keydown="handleTriggerKeyDown"
    >
      <span v-if="selectedLabel" class="truncate">{{ selectedLabel }}</span>
      <span v-else class="text-text-muted truncate">{{ placeholder }}</span>
      <ChevronDown
        :class="[
          'h-4 w-4 text-text-muted shrink-0 transition-transform duration-150',
          isOpen ? 'rotate-180 text-accent' : '',
        ]"
        aria-hidden="true"
      />
    </button>

    <!-- Dropdown Listbox -->
    <div
      v-if="isOpen"
      :id="listboxId"
      role="listbox"
      :aria-label="placeholder"
      class="absolute z-50 mt-1 w-full rounded-md border border-border-strong bg-surface-base shadow-md overflow-hidden text-xs sm:text-sm"
    >
      <!-- Optional Search Filter -->
      <div v-if="searchable" class="relative p-2 border-b border-border-subtle bg-surface">
        <Search class="absolute left-4 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-text-muted pointer-events-none" aria-hidden="true" />
        <input
          ref="searchInputRef"
          v-model="searchQuery"
          type="text"
          :placeholder="searchPlaceholder"
          :class="[
            'w-full rounded bg-surface-base border border-border-subtle py-1.5 pl-8 pr-2.5 text-xs text-text-main placeholder-text-muted focus:outline-hidden focus:border-accent focus:ring-1 focus:ring-accent',
            mono ? 'font-mono tabular-nums' : '',
          ]"
          @keydown="handleSearchKeyDown"
        />
      </div>

      <!-- Options Container -->
      <div class="max-h-60 overflow-y-auto p-1 space-y-0.5" role="presentation">
        <div
          v-if="filteredOptions.length === 0"
          class="p-3 text-center text-xs text-text-muted font-mono"
        >
          {{ emptyText }}
        </div>

        <div
          v-for="(option, idx) in filteredOptions"
          :key="String(option.value)"
          role="option"
          :aria-selected="isSelected(option.value) ? 'true' : 'false'"
          :class="[
            'flex items-center justify-between rounded px-2.5 py-2 cursor-pointer transition-colors select-none min-h-[36px]',
            mono ? 'font-mono tabular-nums' : '',
            highlightedIndex === idx
              ? 'bg-surface-hover text-text-main'
              : 'text-text-muted hover:bg-surface-hover hover:text-text-main',
            isSelected(option.value) ? 'text-accent font-medium' : '',
            option.disabled ? 'opacity-40 cursor-not-allowed pointer-events-none' : '',
          ]"
          @click="selectOption(option)"
          @mouseenter="highlightedIndex = idx"
        >
          <div class="flex flex-col truncate">
            <span class="truncate">{{ option.label }}</span>
            <span v-if="option.description" class="text-[11px] text-text-sub truncate">
              {{ option.description }}
            </span>
          </div>
          <Check
            v-if="isSelected(option.value)"
            class="h-4 w-4 text-accent shrink-0 ml-2"
            aria-hidden="true"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { ChevronDown, Search, Check } from 'lucide-vue-next'
import type { ComboboxOption } from './types'

const props = withDefaults(
  defineProps<{
    modelValue: string | number
    options: ComboboxOption[]
    placeholder?: string
    searchPlaceholder?: string
    emptyText?: string
    disabled?: boolean
    error?: string
    mono?: boolean
    searchable?: boolean
  }>(),
  {
    placeholder: '选择...',
    searchPlaceholder: '搜索选项...',
    emptyText: '未找到匹配项',
    disabled: false,
    error: undefined,
    mono: false,
    searchable: true,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
  (e: 'change', value: string | number): void
}>()

const rootRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)
const isOpen = ref(false)
const searchQuery = ref('')
const highlightedIndex = ref(0)
const listboxId = `combobox-listbox-${Math.random().toString(36).slice(2, 9)}`

const filteredOptions = computed(() => {
  if (!props.searchable || !searchQuery.value.trim()) {
    return props.options
  }
  const q = searchQuery.value.toLowerCase().trim()
  return props.options.filter(
    (opt) =>
      opt.label.toLowerCase().includes(q) ||
      (opt.description && opt.description.toLowerCase().includes(q))
  )
})

const selectedLabel = computed(() => {
  const match = props.options.find((opt) => opt.value === props.modelValue)
  return match ? match.label : ''
})

function isSelected(val: string | number) {
  return props.modelValue === val
}

function toggleOpen() {
  if (props.disabled) return
  isOpen.value = !isOpen.value
}

function openDropdown() {
  if (props.disabled) return
  isOpen.value = true
}

function closeDropdown() {
  isOpen.value = false
  searchQuery.value = ''
  highlightedIndex.value = 0
}

function selectOption(option: ComboboxOption) {
  if (option.disabled) return
  emit('update:modelValue', option.value)
  emit('change', option.value)
  closeDropdown()
}

watch(isOpen, (open) => {
  if (open) {
    // Highlight currently selected item if present
    const idx = filteredOptions.value.findIndex((o) => o.value === props.modelValue)
    highlightedIndex.value = idx >= 0 ? idx : 0
    nextTick(() => {
      searchInputRef.value?.focus()
    })
  }
})

function handleTriggerKeyDown(e: KeyboardEvent) {
  if (props.disabled) return
  if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    openDropdown()
  }
}

function handleSearchKeyDown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    closeDropdown()
  } else if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (filteredOptions.value.length > 0) {
      highlightedIndex.value = (highlightedIndex.value + 1) % filteredOptions.value.length
    }
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (filteredOptions.value.length > 0) {
      highlightedIndex.value =
        (highlightedIndex.value - 1 + filteredOptions.value.length) % filteredOptions.value.length
    }
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const target = filteredOptions.value[highlightedIndex.value]
    if (target && !target.disabled) {
      selectOption(target)
    }
  }
}

function handleClickOutside(e: MouseEvent) {
  if (rootRef.value && !rootRef.value.contains(e.target as Node)) {
    closeDropdown()
  }
}

onMounted(() => {
  if (typeof window !== 'undefined') {
    window.addEventListener('click', handleClickOutside)
  }
})

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('click', handleClickOutside)
  }
})
</script>

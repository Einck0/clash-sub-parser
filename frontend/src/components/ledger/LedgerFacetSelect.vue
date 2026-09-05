<template>
  <div class="relative inline-block facet-controller-container">
    <!-- Collapsible Facet Trigger Button -->
    <button
      type="button"
      :aria-expanded="isOpen"
      aria-haspopup="listbox"
      :aria-label="title"
      class="inline-flex min-h-[44px] sm:min-h-[36px] items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-mono transition-colors cursor-pointer select-none focus:outline-hidden focus:ring-2 focus:ring-accent/40"
      :class="[
        isOpen
          ? 'border-accent bg-surface-hover text-text-main shadow-xs ring-1 ring-accent/30'
          : selected.length > 0
            ? 'border-accent/60 bg-accent-subtle text-accent font-semibold'
            : 'border-border-subtle bg-surface-base text-text-main hover:bg-surface-hover hover:border-border-strong'
      ]"
      @click.stop="$emit('toggle')"
      @keydown.enter.prevent="$emit('toggle')"
      @keydown.space.prevent="$emit('toggle')"
      @keydown.esc.prevent="$emit('close')"
    >
      <component
        :is="icon"
        v-if="icon"
        class="h-3.5 w-3.5 shrink-0"
        :class="selected.length > 0 ? 'text-accent' : 'text-text-muted'"
        aria-hidden="true"
      />
      <span>{{ title }}</span>
      <span
        v-if="selected.length > 0"
        class="inline-flex items-center justify-center rounded-full bg-accent text-white px-1.5 py-0.2 text-[10px] font-mono font-semibold tabular-nums min-w-[18px]"
        :aria-label="`已选 ${selected.length} 项`"
      >
        {{ selected.length }}
      </span>
      <ChevronDown
        class="h-3 w-3 text-text-muted shrink-0 transition-transform"
        :class="{ 'rotate-180 text-accent': isOpen }"
        aria-hidden="true"
      />
    </button>

    <!-- Candidate Options Popover: NOT mounted when isOpen is false (v-if) -->
    <div
      v-if="isOpen"
      role="listbox"
      :aria-label="title"
      aria-multiselectable="true"
      class="absolute left-0 top-full mt-1.5 z-40 w-64 max-w-[calc(100vw-32px)] rounded-md border border-border-subtle bg-surface-base shadow-md p-1.5 text-xs font-mono space-y-1 focus:outline-hidden"
      @keydown.esc.prevent="$emit('close')"
    >
      <!-- Header with Quick Action / Count -->
      <div class="flex items-center justify-between px-2 py-1 border-b border-border-subtle text-[11px] text-text-muted select-none">
        <span>{{ title }} ({{ options.length }})</span>
        <button
          v-if="selected.length > 0"
          type="button"
          class="text-accent hover:underline cursor-pointer min-h-[32px] sm:min-h-[24px] inline-flex items-center px-1"
          @click.stop="$emit('clear')"
        >
          清空已选
        </button>
        <span v-else class="text-text-sub">未选择</span>
      </div>

      <!-- Scrollable Candidate Options -->
      <div class="max-h-56 overflow-y-auto space-y-0.5 py-1">
        <div
          v-for="opt in options"
          :key="opt.key"
          role="option"
          :aria-selected="selected.includes(opt.key)"
          class="facet-candidate-option flex items-center justify-between px-2 py-1.5 rounded-sm cursor-pointer select-none transition-colors min-h-[44px] sm:min-h-[32px]"
          :class="[
            selected.includes(opt.key)
              ? 'bg-accent-subtle text-accent font-medium'
              : 'text-text-main hover:bg-surface-hover'
          ]"
          @click.stop="toggleOption(opt.key)"
        >
          <div class="flex items-center gap-2 truncate pr-2">
            <!-- Accessible Checkbox indicator without emoji -->
            <span
              class="inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-xs border transition-colors"
              :class="[
                selected.includes(opt.key)
                  ? 'border-accent bg-accent text-white'
                  : 'border-border-strong bg-canvas'
              ]"
            >
              <Check v-if="selected.includes(opt.key)" class="h-2.5 w-2.5 stroke-[3]" aria-hidden="true" />
            </span>
            <span class="truncate">{{ opt.name }}</span>
          </div>
          <span v-if="opt.count != null" class="text-[10px] tabular-nums text-text-sub shrink-0">
            ({{ opt.count }})
          </span>
        </div>
        <div v-if="options.length === 0" class="px-2 py-3 text-center text-text-sub text-[11px]">
          暂无可选项
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Check, ChevronDown } from 'lucide-vue-next'

export interface FacetOption {
  key: string
  name: string
  count?: number
  short?: string
}

const props = withDefaults(
  defineProps<{
    title: string
    selected: string[]
    options: FacetOption[]
    isOpen: boolean
    icon?: any
  }>(),
  {
    selected: () => [],
    options: () => [],
    isOpen: false,
  }
)

const emit = defineEmits<{
  (e: 'toggle'): void
  (e: 'close'): void
  (e: 'change', keys: string[]): void
  (e: 'clear'): void
}>()

function toggleOption(key: string) {
  const current = [...props.selected]
  const idx = current.indexOf(key)
  if (idx >= 0) {
    current.splice(idx, 1)
  } else {
    current.push(key)
  }
  emit('change', current)
}
</script>

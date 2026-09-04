<template>
  <div class="sticky top-14 sm:top-16 z-10 mb-5 flex flex-wrap items-center gap-3 rounded-xl border border-white/10 bg-slate-900/70 p-3 sm:p-3.5 backdrop-blur-md shadow-sm transition-all">
    <!-- Search Box -->
    <div v-if="showSearch !== false" class="relative flex-1 min-w-[200px] sm:min-w-[240px]">
      <input
        :value="modelValue"
        type="search"
        :placeholder="placeholder || '搜索…'"
        autocomplete="off"
        class="w-full min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3.5 py-2 pl-9 text-xs text-white placeholder-slate-500 focus:border-blue-500 focus:outline-hidden font-mono transition-colors"
        @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
      />
      <svg class="absolute left-3 top-3.5 h-4 w-4 text-slate-500 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
      </svg>
    </div>

    <!-- Custom Filters Slot -->
    <div v-if="$slots.filters" class="flex flex-wrap items-center gap-2">
      <slot name="filters" />
    </div>

    <!-- Meta / Count Info -->
    <div class="flex flex-wrap items-center gap-2 text-xs font-mono text-slate-400">
      <span v-if="countText" class="rounded bg-slate-800/60 border border-white/5 px-2.5 py-1.5">{{ countText }}</span>
      <slot name="meta" />
    </div>

    <!-- Actions Slot -->
    <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2 w-full sm:w-auto sm:ml-auto">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * PageToolbar component providing unified search, filters, count meta, and actions.
 */
withDefaults(
  defineProps<{
    modelValue?: string
    placeholder?: string
    showSearch?: boolean
    countText?: string
  }>(),
  {
    modelValue: '',
    placeholder: '搜索…',
    showSearch: true,
    countText: '',
  }
)

defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()
</script>

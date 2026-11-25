<script setup lang="ts">
import { ExclamationCircleIcon } from '@heroicons/vue/24/outline'
import { displayText } from '../utils/displayNames'
import { uiText } from '../locales'

withDefaults(
  defineProps<{
    loading?: boolean
    error?: string | null
    empty?: boolean
    emptyTitle?: string
    emptyDescription?: string
  }>(),
  {
    loading: false,
    error: null,
    empty: false,
    emptyTitle: '',
    emptyDescription: '',
  }
)

defineEmits<{
  (e: 'retry'): void
}>()
</script>

<template>
  <div>
    <!-- Loading State -->
    <div v-if="loading" role="status" :aria-label="uiText('正在加载', 'Loading')" class="p-8 flex flex-col items-center justify-center space-y-3" data-testid="section-loading">
      <slot name="loading">
        <span class="loading loading-spinner loading-md text-primary" />
        <span class="text-xs text-base-content/60">{{ uiText('正在加载…', 'Loading…') }}</span>
      </slot>
    </div>

    <!-- Error State -->
    <div v-else-if="error" role="alert" class="alert alert-error shadow-sm p-4 rounded-xl flex flex-wrap items-center justify-between [overflow-wrap:anywhere]" data-testid="section-error">
      <slot name="error" :error="displayText(error)">
        <div class="flex items-center gap-3">
          <ExclamationCircleIcon class="w-5 h-5 shrink-0" />
          <span class="text-xs sm:text-sm font-medium">{{ displayText(error) }}</span>
        </div>
        <button
          type="button"
          class="btn btn-xs btn-outline btn-ghost"
          @click="$emit('retry')"
        >
          {{ uiText('重试', 'Retry') }}
        </button>
      </slot>
    </div>

    <!-- Empty State -->
    <div v-else-if="empty" class="p-8 text-center rounded-xl border border-dashed border-base-300" data-testid="section-empty">
      <slot name="empty">
        <h4 class="text-sm font-semibold text-base-content/80">{{ emptyTitle || uiText('暂无内容', 'No items found') }}</h4>
        <p v-if="emptyDescription" class="text-xs text-base-content/50 mt-1">{{ emptyDescription }}</p>
      </slot>
    </div>

    <!-- Ready Content -->
    <template v-else>
      <slot />
    </template>
  </div>
</template>

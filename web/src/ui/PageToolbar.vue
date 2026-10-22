<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string
    description?: string
    srOnlyTitle?: boolean
  }>(),
  {
    title: '',
    description: '',
    srOnlyTitle: false,
  }
)
</script>

<template>
  <div
    class="flex flex-wrap items-center justify-between gap-3 min-w-0 w-full"
    data-testid="page-toolbar"
  >
    <!-- Left: Optional Title / Description (non-redundant heading) -->
    <div v-if="title || description" class="min-w-0 flex-1">
      <h1 v-if="srOnlyTitle && title" class="sr-only">{{ title }}</h1>
      <h2
        v-else-if="title"
        class="text-base sm:text-lg font-semibold tracking-tight text-base-content truncate"
        data-testid="toolbar-title"
      >
        {{ title }}
      </h2>
      <p
        v-if="description"
        class="text-xs text-base-content/60 truncate mt-0.5"
        data-testid="toolbar-description"
      >
        {{ description }}
      </p>
    </div>

    <!-- Center/Default: Page controls, filters, search -->
    <div
      v-if="$slots.default"
      class="flex flex-wrap items-center gap-2 min-w-0 flex-1"
      data-testid="toolbar-controls"
    >
      <slot />
    </div>

    <!-- Right: Page actions -->
    <div
      v-if="$slots.actions"
      class="flex items-center gap-2 shrink-0 ml-auto"
      data-testid="toolbar-actions"
    >
      <slot name="actions" />
    </div>
  </div>
</template>

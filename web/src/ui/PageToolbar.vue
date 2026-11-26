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
    class="csp-page-toolbar flex flex-wrap items-start justify-between gap-3 min-w-0 w-full"
    data-testid="page-toolbar"
  >
    <!-- Left: Optional Title / Description (non-redundant heading) -->
    <div v-if="title || description" class="min-w-0 basis-full sm:flex-[1_1_16rem]">
      <h1 v-if="srOnlyTitle && title" class="sr-only">{{ title }}</h1>
      <h2
        v-else-if="title"
        class="text-lg sm:text-xl font-semibold tracking-tight text-base-content break-words [overflow-wrap:anywhere]"
        data-testid="toolbar-title"
      >
        {{ title }}
      </h2>
      <p
        v-if="description"
        class="csp-page-description text-sm mt-1"
        data-testid="toolbar-description"
      >
        {{ description }}
      </p>
    </div>

    <!-- Center/Default: Page controls, filters, search -->
    <div
      v-if="$slots.default"
      class="flex flex-wrap items-center gap-2 min-w-0 basis-full sm:flex-[1_1_20rem]"
      data-testid="toolbar-controls"
    >
      <slot />
    </div>

    <!-- Right: Page actions -->
    <div
      v-if="$slots.actions"
      class="flex flex-wrap items-center gap-2 max-w-full min-w-0 basis-full ml-0 sm:flex-[0_1_auto] sm:ml-auto"
      data-testid="toolbar-actions"
    >
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  as?: string
  gap?: 'none' | 'sm' | 'md' | 'lg'
  minCardWidth?: string
}>(), { as: 'div', gap: 'md', minCardWidth: 'clamp(16rem, 24vw + 10rem, 20rem)' })
</script>

<template>
  <component
    :is="as"
    class="grid min-w-0 max-w-full"
    :class="{ 'gap-0': gap === 'none', 'gap-2': gap === 'sm', 'gap-4': gap === 'md', 'gap-6': gap === 'lg' }"
    :style="{ gridTemplateColumns: `repeat(auto-fit, minmax(min(${minCardWidth}, 100%), 1fr))` }"
  >
    <slot />
  </component>
</template>


<script setup lang="ts">
import Drawer from './Drawer.vue'

defineProps<{
  modelValue: boolean
  title?: string
  description?: string
  widthClass?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'close'): void
}>()
</script>

<template>
  <Drawer
    :model-value="modelValue"
    :title="title"
    :description="description"
    :width-class="widthClass"
    @update:model-value="emit('update:modelValue', $event)"
    @close="emit('close')"
  >
    <template #default>
      <slot />
    </template>
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </Drawer>
</template>

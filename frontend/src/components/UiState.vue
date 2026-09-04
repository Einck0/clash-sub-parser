<template>
  <div class="ui-state" :class="[`ui-state-${type}`, { compact }]" :role="type === 'error' ? 'alert' : 'status'" aria-live="polite">
    <div class="ui-state-icon" aria-hidden="true">{{ icon }}</div>
    <div class="ui-state-body">
      <strong>{{ title }}</strong>
      <p v-if="description">{{ description }}</p>
      <div v-if="$slots.actions" class="ui-state-actions">
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    type?: string
    title: string
    description?: string
    compact?: boolean
  }>(),
  {
    type: 'info',
    description: '',
    compact: false,
  }
)

defineSlots<{
  default?: () => any
  actions?: () => any
}>()

const icon = computed(() => {
  if (props.type === 'loading') return '…'
  if (props.type === 'empty') return '∅'
  if (props.type === 'error') return '!'
  if (props.type === 'success') return '✓'
  return 'i'
})
</script>

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

<script setup>
import { computed } from 'vue'

const props = defineProps({
  type: { type: String, default: 'info' },
  title: { type: String, required: true },
  description: { type: String, default: '' },
  compact: { type: Boolean, default: false },
})

const icon = computed(() => {
  if (props.type === 'loading') return '…'
  if (props.type === 'empty') return '∅'
  if (props.type === 'error') return '!'
  if (props.type === 'success') return '✓'
  return 'i'
})
</script>

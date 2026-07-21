<template>
  <div class="page-toolbar">
    <div v-if="showSearch" class="toolbar-search">
      <input
        :value="modelValue"
        type="search"
        :placeholder="placeholder"
        autocomplete="off"
        @input="$emit('update:modelValue', $event.target.value)"
      />
    </div>
    <div v-if="$slots.filters" class="toolbar-filters">
      <slot name="filters" />
    </div>
    <div class="toolbar-meta">
      <span v-if="countText" class="muted">{{ countText }}</span>
      <slot name="meta" />
    </div>
    <div v-if="$slots.actions" class="toolbar-actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup>
defineProps({
  modelValue: { type: String, default: '' },
  placeholder: { type: String, default: '搜索…' },
  showSearch: { type: Boolean, default: true },
  countText: { type: String, default: '' },
})
defineEmits(['update:modelValue'])
</script>

<style scoped>
.page-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  margin: 0 0 14px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--surface-2);
  position: sticky;
  top: 64px;
  z-index: 4;
  backdrop-filter: blur(10px);
}
.toolbar-search {
  flex: 1 1 220px;
  min-width: min(100%, 180px);
}
.toolbar-search input {
  width: 100%;
}
.toolbar-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.toolbar-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  font-size: 12px;
}
.toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
}
@media (max-width: 720px) {
  .page-toolbar {
    top: 52px;
    padding: 8px 10px;
  }
  .toolbar-actions {
    width: 100%;
    margin-left: 0;
  }
  .toolbar-actions :deep(button) {
    flex: 1 1 auto;
    min-height: 40px;
  }
}
</style>

<script setup lang="ts">
import { computed } from 'vue'
import { uiText } from '../../locales/redesign'
const props = defineProps<{ page: number; total: number; pageSize: number }>()
defineEmits<{ (e: 'update:page', page: number): void }>()
const pages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
</script>

<template>
  <nav v-if="pages > 1" class="flex flex-wrap items-center justify-end gap-2" :aria-label="uiText('来源记录分页', 'Source records pagination')">
    <button type="button" class="btn btn-ghost btn-xs" :disabled="page <= 1" @click="$emit('update:page', page - 1)">{{ uiText('上一页', 'Previous') }}</button>
    <span class="text-base-content/60">{{ page }} / {{ pages }}</span>
    <button type="button" class="btn btn-ghost btn-xs" :disabled="page >= pages" @click="$emit('update:page', page + 1)">{{ uiText('下一页', 'Next') }}</button>
  </nav>
</template>

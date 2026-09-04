<template>
  <div
    ref="containerRef"
    role="table"
    aria-label="节点质检与路由账本表格"
    :aria-rowcount="items.length"
    tabindex="0"
    class="relative h-[650px] overflow-auto border border-border-subtle rounded-lg bg-surface-base focus:outline-hidden focus-visible:ring-2 focus-visible:ring-accent"
    @keydown="handleKeyDown"
  >
    <div
      role="rowgroup"
      :style="{
        height: `${rowVirtualizer.getTotalSize()}px`,
        width: '100%',
        position: 'relative',
      }"
    >
      <div
        v-for="virtualRow in rowVirtualizer.getVirtualItems()"
        :key="virtualRow.index"
        role="row"
        :aria-rowindex="virtualRow.index + 1"
        :aria-selected="selectedKeys ? selectedKeys.has(getItemKey(items[virtualRow.index])) : undefined"
        :style="{
          position: 'absolute',
          top: 0,
          left: 0,
          width: '100%',
          height: `${virtualRow.size}px`,
          transform: `translateY(${virtualRow.start}px)`,
        }"
        class="flex items-center px-4 border-b border-border-subtle/50 hover:bg-surface-hover transition-colors text-sm"
      >
        <slot
          :item="items[virtualRow.index]"
          :index="virtualRow.index"
          :is-selected="selectedKeys ? selectedKeys.has(getItemKey(items[virtualRow.index])) : false"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" generic="T extends Record<string, any>">
import { ref } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'

const props = withDefaults(
  defineProps<{
    items: T[]
    estimateSize?: number
    overscan?: number
    selectedKeys?: Set<string>
    keyField?: string
  }>(),
  {
    estimateSize: 48,
    overscan: 10,
    keyField: 'name',
  }
)

const containerRef = ref<HTMLElement | null>(null)

const rowVirtualizer = useVirtualizer({
  get count() {
    return props.items.length
  },
  getScrollElement: () => containerRef.value,
  estimateSize: () => props.estimateSize,
  overscan: props.overscan,
})

function getItemKey(item: T): string {
  if (!item) return ''
  return item[props.keyField] || ''
}

function handleKeyDown(e: KeyboardEvent) {
  const el = containerRef.value
  if (!el) return

  const step = props.estimateSize
  const pageStep = el.clientHeight || step * 10

  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      el.scrollTop += step
      break
    case 'ArrowUp':
      e.preventDefault()
      el.scrollTop -= step
      break
    case 'PageDown':
      e.preventDefault()
      el.scrollTop += pageStep
      break
    case 'PageUp':
      e.preventDefault()
      el.scrollTop -= pageStep
      break
    case 'Home':
      e.preventDefault()
      el.scrollTop = 0
      break
    case 'End':
      e.preventDefault()
      el.scrollTop = el.scrollHeight
      break
  }
}
</script>

<template>
  <div ref="containerRef" class="relative h-[650px] overflow-auto border border-white/10 rounded-xl bg-slate-900/60 backdrop-blur-md">
    <div
      :style="{
        height: `${rowVirtualizer.getTotalSize()}px`,
        width: '100%',
        position: 'relative',
      }"
    >
      <div
        v-for="virtualRow in rowVirtualizer.getVirtualItems()"
        :key="virtualRow.index"
        :style="{
          position: 'absolute',
          top: 0,
          left: 0,
          width: '100%',
          height: `${virtualRow.size}px`,
          transform: `translateY(${virtualRow.start}px)`,
        }"
        class="flex items-center px-4 border-b border-white/5 hover:bg-slate-800/40 transition-colors text-sm"
      >
        <slot :item="items[virtualRow.index]" :index="virtualRow.index" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts" generic="T">
import { ref } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'

const props = withDefaults(
  defineProps<{
    items: T[]
    estimateSize?: number
    overscan?: number
  }>(),
  {
    estimateSize: 42,
    overscan: 10,
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
</script>

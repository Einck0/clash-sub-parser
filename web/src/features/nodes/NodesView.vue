<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type VNodeRef } from 'vue'
import { ArrowPathIcon, CheckCircleIcon, ExclamationTriangleIcon, EyeIcon } from '@heroicons/vue/24/outline'
import { useWindowVirtualizer } from '@tanstack/vue-virtual'
import { useElementSize, useResizeObserver } from '@vueuse/core'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import { nodeCapabilityLabel } from './nodeView'
import { useNodes } from './useNodes'
import { deriveColumns } from '../../composables/useResponsiveColumns'
import { t } from '../../locales'

const GAP = 16
const MIN_CARD_WIDTH = 288
const ESTIMATED_ROW_HEIGHT = 140

const { items, loading, loadingMore, error, total, hasMore, load, loadMore } = useNodes()

const containerRef = ref<HTMLElement | null>(null)
const { width } = useElementSize(containerRef)

const columns = computed(() => deriveColumns(width.value, MIN_CARD_WIDTH, GAP))
const rowCount = computed(() => Math.ceil(items.value.length / columns.value))

function getRowItems(rowIndex: number) {
  const start = rowIndex * columns.value
  return items.value.slice(start, start + columns.value)
}

const scrollMargin = ref(0)

function updateScrollMargin() {
  if (containerRef.value && typeof window !== 'undefined') {
    const rect = containerRef.value.getBoundingClientRect()
    scrollMargin.value = Math.max(0, rect.top + window.scrollY)
  }
}

useResizeObserver(containerRef, updateScrollMargin)

const rowVirtualizer = useWindowVirtualizer(
  computed(() => ({
    count: rowCount.value,
    estimateSize: () => ESTIMATED_ROW_HEIGHT + GAP,
    overscan: 4,
    scrollMargin: scrollMargin.value,
    scrollToFn: (offset, options) => {
      if (
        typeof window !== 'undefined' &&
        typeof window.scrollTo === 'function' &&
        !navigator.userAgent.includes('jsdom')
      ) {
        window.scrollTo({ top: offset, behavior: options?.behavior })
      }
    },
  })),
)

// Measure row element dimensions dynamically so card content wrapping adapts accurately
const measureRow: VNodeRef = (element) => {
  if (element instanceof HTMLElement) {
    rowVirtualizer.value.measureElement(element)
  }
}

// Trigger remeasurement after column or width changes so layout stays seamless
watch(columns, () => {
  nextTick(() => {
    rowVirtualizer.value.measure()
  })
})

watch(width, () => {
  nextTick(() => {
    rowVirtualizer.value.measure()
  })
})

function onScroll() {
  if (typeof window === 'undefined') return
  const scrollPosition = window.innerHeight + window.scrollY
  const threshold = document.documentElement.scrollHeight - 420
  if (scrollPosition >= threshold) {
    loadMore()
  }
}

onMounted(() => {
  load()
  if (typeof window !== 'undefined') {
    window.addEventListener('scroll', onScroll, { passive: true })
    nextTick(updateScrollMargin)
  }
})

onUnmounted(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('scroll', onScroll)
  }
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="nodes-title">
    <!-- Header -->
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('nodes.tag') }}</p>
        <h2 id="nodes-title" class="mt-1 text-2xl font-bold">{{ t('nodes.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">{{ t('nodes.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <span class="badge badge-ghost text-xs">{{ total.toLocaleString() }} {{ t('nodes.totalNodes') }}</span>
        <button class="btn btn-ghost btn-sm btn-square touch-manipulation" type="button" :title="t('common.refresh')" @click="load">
          <ArrowPathIcon class="h-4 w-4" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loading"
      @retry="load"
    />

    <!-- Skeleton Loading with identical responsive min-card width -->
    <div
      v-if="loading && items.length === 0"
      class="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,288px),1fr))]"
    >
      <div v-for="index in 6" :key="index" class="skeleton h-32 rounded-box" />
    </div>

    <!-- Virtualized Responsive Node Grid Normal page flow with width-derived columns -->
    <div v-else ref="containerRef" class="w-full min-w-0">
      <div
        v-if="items.length > 0"
        class="relative w-full"
        :style="{ height: `${rowVirtualizer.getTotalSize()}px` }"
      >
        <div
          v-for="virtualRow in rowVirtualizer.getVirtualItems()"
          :key="String(virtualRow.key)"
          :ref="measureRow"
          class="absolute left-0 top-0 w-full"
          :style="{
            transform: `translateY(${virtualRow.start - scrollMargin}px)`,
            paddingBottom: `${GAP}px`,
          }"
        >
          <div
            class="grid min-w-0 max-w-full"
            :style="{
              gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
              gap: `${GAP}px`,
            }"
          >
            <article
              v-for="node in getRowItems(virtualRow.index)"
              :key="node.logicalId"
              class="card border border-base-300 bg-base-200 shadow-sm transition hover:border-primary/40 hover:shadow-md min-w-0 max-w-full overflow-hidden"
            >
              <div class="card-body gap-3 p-4 sm:p-5 min-w-0 max-w-full overflow-hidden">
                <div class="flex flex-wrap items-start justify-between gap-2 sm:gap-3 min-w-0">
                  <div class="flex min-w-0 items-center gap-2.5 sm:gap-3 flex-1">
                    <div class="rounded-xl bg-primary/10 p-2 text-primary shrink-0">
                      <CheckCircleIcon v-if="node.active" class="h-5 w-5" />
                      <ExclamationTriangleIcon v-else class="h-5 w-5" />
                    </div>
                    <div class="min-w-0 flex-1">
                      <h3 class="truncate font-semibold text-sm sm:text-base">{{ node.displayName }}</h3>
                      <p class="mt-0.5 truncate font-mono text-xs opacity-60">{{ node.logicalId }}</p>
                    </div>
                  </div>
                  <div class="flex items-center gap-1.5 sm:gap-2 shrink-0">
                    <span class="badge badge-outline font-mono text-xs">{{ node.protocol }}</span>
                    <StatusBadge
                      :label="node.active ? t('common.enabled') : t('common.disabled')"
                      :tone="node.active ? 'success' : 'warning'"
                    />
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-1.5 sm:gap-2 border-t border-base-300 pt-3 text-xs min-w-0">
                  <span class="mr-1 text-xs opacity-60 shrink-0">Capabilities</span>
                  <StatusBadge
                    :label="`Streaming: ${nodeCapabilityLabel(node, 'streaming').label}`"
                    :tone="nodeCapabilityLabel(node, 'streaming').tone"
                  />
                  <StatusBadge
                    :label="`AI: ${nodeCapabilityLabel(node, 'ai').label}`"
                    :tone="nodeCapabilityLabel(node, 'ai').tone"
                  />
                </div>
              </div>
            </article>
          </div>
        </div>
      </div>

      <!-- Terminal content reserves inherited dynamic dock inset for clean separation -->
      <div
        v-if="items.length > 0"
        class="py-4 text-center"
        :style="{ paddingBottom: 'var(--content-dock-inset, 32px)' }"
      >
        <div v-if="loadingMore" class="flex justify-center py-2">
          <span class="loading loading-spinner loading-sm text-primary" />
        </div>
        <p v-if="!hasMore" class="text-xs opacity-60">All nodes loaded</p>
      </div>

      <!-- Empty State -->
      <EmptyState
        v-if="!items.length && !loading"
        :icon="EyeIcon"
        :title="t('nodes.emptyTitle')"
        :description="t('nodes.emptyDesc')"
      />
    </div>
  </section>
</template>

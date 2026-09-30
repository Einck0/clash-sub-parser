<script setup lang="ts">
import { nextTick, onUnmounted, ref, watch } from 'vue'
import { onClickOutside } from '@vueuse/core'

const props = withDefaults(
  defineProps<{
    placement?: 'bottom-end' | 'bottom-start' | 'top-end' | 'top-start'
    panelClass?: string
  }>(),
  { placement: 'bottom-end', panelClass: 'w-48' }
)

const isOpen = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)
const panelStyle = ref<Record<string, string>>({ visibility: 'hidden' })
let observer: ResizeObserver | undefined

function viewport() {
  const visual = window.visualViewport
  return {
    left: visual?.offsetLeft ?? 0,
    top: visual?.offsetTop ?? 0,
    width: visual?.width ?? window.innerWidth,
    height: visual?.height ?? window.innerHeight,
  }
}

function position() {
  const trigger = triggerRef.value?.getBoundingClientRect()
  const panel = panelRef.value
  if (!trigger || !panel || !isOpen.value) return
  const view = viewport()
  const padding = 8
  const gap = 6
  const leftEdge = view.left + padding
  const rightEdge = view.left + view.width - padding
  const topEdge = view.top + padding
  const bottomEdge = view.top + view.height - padding
  if (trigger.bottom < topEdge || trigger.top > bottomEdge || trigger.right < leftEdge || trigger.left > rightEdge) {
    close(false)
    return
  }
  const width = Math.min(panel.offsetWidth, rightEdge - leftEdge)
  const height = panel.scrollHeight
  const below = Math.max(0, bottomEdge - trigger.bottom - gap)
  const above = Math.max(0, trigger.top - topEdge - gap)
  const preferTop = props.placement.startsWith('top')
  const top = preferTop ? above >= height || above > below : !(below >= height || below >= above)
  const space = top ? above : below
  const x = props.placement.endsWith('end') ? trigger.right - width : trigger.left
  panelStyle.value = {
    position: 'fixed',
    left: `${Math.max(leftEdge, Math.min(x, rightEdge - width))}px`,
    top: `${Math.max(topEdge, top ? trigger.top - gap - Math.min(height, space) : trigger.bottom + gap)}px`,
    maxWidth: `${Math.max(0, rightEdge - leftEdge)}px`,
    maxHeight: `${Math.max(0, space)}px`,
    visibility: 'visible',
  }
}

function stopPositioning() {
  observer?.disconnect()
  observer = undefined
  window.removeEventListener('scroll', position, true)
  window.removeEventListener('resize', position)
  window.visualViewport?.removeEventListener('resize', position)
  window.visualViewport?.removeEventListener('scroll', position)
}

function close(restoreFocus = true) {
  isOpen.value = false
  if (restoreFocus) triggerRef.value?.querySelector<HTMLElement>('button, [tabindex], a')?.focus()
}
function open() { isOpen.value = true }
function toggle() { isOpen.value ? close() : open() }

watch(isOpen, async (openNow) => {
  if (!openNow) {
    stopPositioning()
    panelStyle.value = { visibility: 'hidden' }
    return
  }
  await nextTick()
  if (!isOpen.value || !panelRef.value) return
  position()
  window.addEventListener('scroll', position, true)
  window.addEventListener('resize', position)
  window.visualViewport?.addEventListener('resize', position)
  window.visualViewport?.addEventListener('scroll', position)
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(position)
    observer.observe(panelRef.value)
    if (triggerRef.value) observer.observe(triggerRef.value)
  }
  panelRef.value.querySelector<HTMLElement>('button:not(:disabled), a[href], [tabindex="0"]')?.focus()
})

onClickOutside(panelRef, (event) => {
  if (triggerRef.value?.contains(event.target as Node)) return
  close(false)
})
function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && isOpen.value) {
    event.preventDefault()
    event.stopPropagation()
    close(true)
  }
}
onUnmounted(stopPositioning)

defineExpose({ isOpen, open, close, toggle })
</script>

<template>
  <div class="relative inline-block text-left" @keydown="handleKeydown">
    <div ref="triggerRef" class="inline-flex" @click="toggle">
      <slot name="trigger" :open="isOpen" />
    </div>
    <Teleport to="body">
      <div
        v-if="isOpen"
        ref="panelRef"
        class="z-40 rounded-2xl bg-base-200/95 backdrop-blur-2xl border border-white/10 shadow-2xl p-1.5 outline-none overflow-y-auto overscroll-contain"
        :class="panelClass"
        :style="panelStyle"
        role="menu"
        aria-orientation="vertical"
        tabindex="-1"
        @keydown="handleKeydown"
      >
        <slot :close="close" />
      </div>
    </Teleport>
  </div>
</template>

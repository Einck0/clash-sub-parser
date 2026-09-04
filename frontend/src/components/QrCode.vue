<template>
  <div v-if="url" class="inline-flex items-center justify-center">
    <img
      v-if="!error"
      :src="qrUrl"
      :alt="'QR Code for ' + url"
      class="rounded-md border border-border-subtle"
      @error="error = true"
    />
    <div v-else class="text-xs p-2 text-text-muted">QR 加载失败</div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

const props = defineProps<{
  url?: string
  size?: number
}>()

const error = ref(false)

const qrUrl = computed(() => {
  if (!props.url) return ''
  const size = props.size || 160
  return `https://api.qrserver.com/v1/create-qr-code/?size=${size}x${size}&data=${encodeURIComponent(props.url)}&margin=4`
})
</script>

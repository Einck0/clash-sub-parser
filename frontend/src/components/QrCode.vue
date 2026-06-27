<template>
  <div class="qr-wrapper" v-if="url">
    <img
      v-if="!error"
      :src="qrUrl"
      :alt="'QR Code for ' + url"
      class="qr-image"
      @error="error = true"
    />
    <div v-else class="qr-fallback muted">QR 加载失败</div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'

const props = defineProps({
  url: { type: String, default: '' },
  size: { type: Number, default: 160 },
})

const error = ref(false)

const qrUrl = computed(() => {
  if (!props.url) return ''
  return `https://api.qrserver.com/v1/create-qr-code/?size=${props.size}x${props.size}&data=${encodeURIComponent(props.url)}&margin=4`
})
</script>

<style scoped>
.qr-wrapper {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.qr-image {
  border-radius: 6px;
  border: 1px solid var(--border, #e2e8f0);
}
.qr-fallback {
  font-size: 12px;
  padding: 8px;
}
</style>

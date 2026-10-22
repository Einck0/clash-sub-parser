<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { GlobeAltIcon } from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'

const publicBaseUrl = ref('')
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')

async function fetchPublicBaseUrl() {
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await api.get<{ public_base_url: string }>('/api/v1/settings/public-base-url')
    if (res && typeof res.public_base_url === 'string') {
      publicBaseUrl.value = res.public_base_url
    }
  } catch (err: any) {
    errorMessage.value = err?.message || '获取公网访问基准地址失败'
  } finally {
    loading.value = false
  }
}

function suggestOrigin() {
  if (typeof window !== 'undefined' && window.location?.origin) {
    publicBaseUrl.value = window.location.origin
  }
}

async function savePublicBaseUrl() {
  saving.value = true
  errorMessage.value = ''
  try {
    const res = await api.put<{ public_base_url: string }>('/api/v1/settings/public-base-url', {
      public_base_url: publicBaseUrl.value.trim(),
    })
    publicBaseUrl.value = res?.public_base_url || publicBaseUrl.value.trim()
    toastStore.push({ message: '公网访问基准地址已保存', tone: 'success' })
  } catch (err: any) {
    errorMessage.value = err?.message || '保存失败'
    toastStore.push({ message: errorMessage.value, tone: 'error' })
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void fetchPublicBaseUrl()
})
</script>

<template>
  <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="public-base-url-settings">
    <div class="card-body p-5 space-y-4">
      <div class="flex items-center gap-2.5">
        <GlobeAltIcon class="w-5 h-5 text-primary" />
        <div>
          <h3 class="text-sm sm:text-base font-bold">公网访问基准地址 (Public Base URL)</h3>
          <p class="text-xs text-base-content/60">
            用于组装客户端实时订阅直链的基础 URL。必须配置可信公网地址，禁止自动伪造或推断。
          </p>
        </div>
      </div>

      <div class="space-y-2">
        <div class="flex flex-col sm:flex-row gap-2">
          <input
            v-model="publicBaseUrl"
            type="text"
            data-testid="settings-public-base-url-input"
            class="input input-sm flex-1 font-mono text-xs"
            placeholder="https://sub.example.com"
          />
          <button
            type="button"
            class="btn btn-sm btn-outline"
            data-testid="settings-suggest-origin-btn"
            @click="suggestOrigin"
          >
            填入当前地址建议
          </button>
          <button
            type="button"
            class="btn btn-sm btn-primary"
            data-testid="settings-save-public-base-url-btn"
            :disabled="saving"
            @click="savePublicBaseUrl"
          >
            <span v-if="saving" class="loading loading-spinner loading-xs" />
            <span>保存配置</span>
          </button>
        </div>

        <p v-if="errorMessage" class="text-error text-xs">{{ errorMessage }}</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { GlobeAltIcon, ArrowPathIcon } from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'

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
    errorMessage.value = err?.message || uiText('获取公网访问基准地址失败', 'Failed to fetch public base URL')
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
    toastStore.push({
      message: uiText('公网访问基准地址已保存', 'Public base URL saved'),
      tone: 'success',
    })
  } catch (err: any) {
    errorMessage.value = err?.message || uiText('保存失败', 'Save failed')
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
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/40 pb-3">
        <div class="flex items-center gap-2.5">
          <GlobeAltIcon class="w-5 h-5 text-primary" />
          <div>
            <h3 class="text-sm sm:text-base font-bold">
              {{ uiText('公网访问基准地址 (Public Base URL)', 'Public Base URL') }}
            </h3>
            <p class="text-xs text-base-content/60">
              {{ uiText('用于组装客户端实时订阅直链的基础 URL。必须配置可信公网地址，禁止自动伪造或推断。', 'Base URL for assembling client live subscription links. Must be a trusted public address; auto-guessing is prohibited.') }}
            </p>
          </div>
        </div>
        <span class="badge badge-sm badge-outline font-mono text-[11px] text-base-content/70">
          {{ uiText('服务端配置', 'Server Configuration') }}
        </span>
      </div>

      <div class="space-y-2">
        <div class="flex flex-col sm:flex-row gap-2">
          <input
            v-model="publicBaseUrl"
            type="text"
            data-testid="settings-public-base-url-input"
            class="input input-sm flex-1 font-mono text-xs bg-base-100/80"
            placeholder="https://sub.example.com"
            :disabled="loading || saving"
          />
          <button
            type="button"
            class="btn btn-sm btn-outline gap-1.5"
            data-testid="settings-suggest-origin-btn"
            :disabled="loading || saving"
            @click="suggestOrigin"
          >
            {{ uiText('填入当前地址建议', 'Use Current Origin') }}
          </button>
          <button
            type="button"
            class="btn btn-sm btn-primary gap-1.5"
            data-testid="settings-save-public-base-url-btn"
            :disabled="loading || saving"
            @click="savePublicBaseUrl"
          >
            <span v-if="saving" class="loading loading-spinner loading-xs" />
            <span>{{ uiText('保存配置', 'Save Configuration') }}</span>
          </button>
        </div>

        <div v-if="errorMessage" role="alert" class="alert alert-error text-xs flex items-center justify-between">
          <span>{{ errorMessage }}</span>
          <button type="button" class="btn btn-xs btn-ghost gap-1" @click="fetchPublicBaseUrl">
            <ArrowPathIcon class="w-3.5 h-3.5" />
            <span>{{ t('common.retry') }}</span>
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

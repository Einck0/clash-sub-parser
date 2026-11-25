<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { GlobeAltIcon, ArrowPathIcon } from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayText } from '../../utils/displayNames'
import { useServerSettings } from './useServerSettings'

const publicBaseUrl = ref('')
const state = useServerSettings({
  read: () => api.get<{ public_base_url: string }>('/api/v1/settings/public-base-url'),
  apply: value => {
    if (!value || typeof value.public_base_url !== 'string') throw new Error(uiText('服务端返回的公网访问地址无效', 'The server returned an invalid public base URL.'))
    publicBaseUrl.value = value.public_base_url
  },
  snapshot: () => publicBaseUrl.value,
})
const errorMessage = computed(() => state.error.value)

function suggestOrigin() {
  if (typeof window !== 'undefined' && window.location?.origin) publicBaseUrl.value = window.location.origin
}

async function savePublicBaseUrl() {
  const saved = await state.save(() => api.put<{ public_base_url: string }>('/api/v1/settings/public-base-url', {
    public_base_url: publicBaseUrl.value.trim(),
  }))
  if (saved) toastStore.push({ message: uiText('公网访问基准地址已保存', 'Public base URL saved'), tone: 'success' })
}

onMounted(() => { void state.load() })
</script>

<template>
  <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="public-base-url-settings">
    <div class="card-body p-5 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/40 pb-3">
        <div class="flex items-center gap-2.5">
          <GlobeAltIcon class="w-5 h-5 text-primary" />
          <div>
            <h3 class="text-sm sm:text-base font-bold">{{ uiText('公网访问基准地址 (Public Base URL)', 'Public Base URL') }}</h3>
            <p class="text-xs text-base-content/60">{{ uiText('用于组装客户端实时订阅直链的基础 URL。必须配置可信公网地址，禁止自动伪造或推断。', 'Base URL for assembling client live subscription links. Must be a trusted public address; auto-guessing is prohibited.') }}</p>
          </div>
        </div>
        <span class="badge badge-sm badge-outline font-mono text-[11px] text-base-content/70">{{ uiText('服务端配置', 'Server Configuration') }}</span>
      </div>

      <p v-if="state.loading.value && !state.loaded.value" role="status" class="text-xs text-base-content/60">{{ t('common.loading') }}</p>
      <div v-if="errorMessage" role="alert" class="alert alert-error text-xs flex flex-wrap items-center justify-between gap-2">
        <span class="min-w-0 whitespace-pre-wrap [overflow-wrap:anywhere]">{{ displayText(errorMessage) }}</span>
        <button type="button" class="btn btn-xs btn-ghost gap-1" :disabled="state.busy.value" @click="state.load">
          <ArrowPathIcon class="w-3.5 h-3.5" /><span>{{ t('common.retry') }}</span>
        </button>
      </div>
      <div v-if="!state.loaded.value && !state.busy.value && !errorMessage" class="flex justify-end">
        <button type="button" class="btn btn-sm btn-outline" @click="state.load">{{ t('common.retry') }}</button>
      </div>

      <fieldset v-if="state.loaded.value" :disabled="state.busy.value || state.readFailed.value" class="space-y-2">
        <div class="flex flex-col sm:flex-row gap-2">
          <label for="settings-public-base-url" class="sr-only">{{ uiText('公网访问基准地址', 'Public base URL') }}</label>
          <input id="settings-public-base-url" v-model="publicBaseUrl" type="text" data-testid="settings-public-base-url-input" class="input input-sm min-w-0 flex-1 font-mono text-xs bg-base-100/80" placeholder="https://sub.example.com" :disabled="state.busy.value" />
          <button type="button" class="btn btn-sm btn-outline gap-1.5" data-testid="settings-suggest-origin-btn" :disabled="state.busy.value" @click="suggestOrigin">{{ uiText('填入当前地址建议', 'Use Current Origin') }}</button>
          <button type="button" class="btn btn-sm btn-primary gap-1.5" data-testid="settings-save-public-base-url-btn" :disabled="state.busy.value || !state.dirty.value" @click="savePublicBaseUrl">
            <span v-if="state.saving.value" class="loading loading-spinner loading-xs" /><span>{{ uiText('保存配置', 'Save Configuration') }}</span>
          </button>
        </div>
      </fieldset>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../../api/client'
import { t } from '../../locales'
import { toastStore } from '../../ui/toast'

type Mode = 'environment' | 'direct' | 'proxy'
interface FetchSettings {
  mode: Mode
  effective_mode: 'direct' | 'proxy'
  effective_source: string
  configured_proxy_endpoint: string
  effective_proxy_endpoint: string
  credentials_present: boolean
  effective_credentials_present: boolean
}
const settings = ref<FetchSettings | null>(null)
const mode = ref<Mode>('environment')
const proxyURL = ref('')
const clearProxy = ref(false)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
function apply(value: FetchSettings) {
  if (!value || !['environment', 'direct', 'proxy'].includes(value.mode) ||
      !['direct', 'proxy'].includes(value.effective_mode) || typeof value.credentials_present !== 'boolean') {
    throw new Error(t('settings.fetchInvalidResponse'))
  }
  settings.value = value
  mode.value = value.mode
  proxyURL.value = ''
  clearProxy.value = false
}
async function load() {
  loading.value = true
  error.value = ''
  try { apply(await api.get<FetchSettings>('/api/v1/settings/subscription-fetch')) }
  catch (err: any) { error.value = err?.message || t('settings.fetchLoadFailed') }
  finally { loading.value = false }
}
async function save(reset = false) {
  if (!settings.value || saving.value) return
  saving.value = true
  error.value = ''
  const payload: { mode: Mode; proxy_url?: string; clear_proxy_url?: boolean } = { mode: reset ? 'environment' : mode.value }
  if (!reset) {
    if (clearProxy.value) payload.clear_proxy_url = true
    else if (proxyURL.value.trim()) payload.proxy_url = proxyURL.value.trim()
  }
  try {
    apply(await api.put<FetchSettings>('/api/v1/settings/subscription-fetch', payload))
    toastStore.push({ message: t('settings.fetchSaved'), tone: 'success' })
  } catch (err: any) { error.value = err?.message || t('settings.fetchSaveFailed') }
  finally { saving.value = false }
}
onMounted(load)
</script>

<template>
  <section class="glass-panel p-6 space-y-4 min-w-0" data-testid="subscription-fetch-settings">
    <div>
      <h2 class="text-xl font-semibold">{{ t('settings.fetchTitle') }}</h2>
      <p class="text-sm text-base-content/60 mt-1">{{ t('settings.fetchDescription') }}</p>
    </div>
    <p v-if="loading" role="status">{{ t('common.loading') }}</p>
    <div v-if="error" role="alert" class="alert alert-error break-words" data-testid="fetch-settings-error">{{ error }}</div>
    <button v-if="!loading && !settings" class="btn btn-sm" @click="load">{{ t('common.refresh') }}</button>
    <template v-if="settings">
      <label class="form-control w-full">
        <span class="label-text mb-2">{{ t('settings.fetchMode') }}</span>
        <select v-model="mode" class="select select-bordered w-full" :disabled="saving" data-testid="fetch-mode">
          <option value="environment">{{ t('settings.fetchEnvironment') }}</option>
          <option value="direct">{{ t('settings.fetchDirect') }}</option>
          <option value="proxy">{{ t('settings.fetchProxy') }}</option>
        </select>
      </label>
      <div class="text-sm space-y-1" aria-live="polite">
        <p>{{ t('settings.fetchEffective') }}: <span class="font-mono break-all" data-testid="fetch-effective">{{ settings.effective_proxy_endpoint || t('settings.fetchDirect') }}</span></p>
        <p>{{ t('settings.fetchSource') }}: {{ settings.effective_source === 'environment' ? t('settings.fetchEnvironment') : settings.effective_source === 'direct' ? t('settings.fetchDirect') : t('settings.fetchProxy') }}</p>
        <p v-if="settings.configured_proxy_endpoint" class="break-all">{{ t('settings.fetchConfigured') }}: {{ settings.configured_proxy_endpoint }} · {{ settings.credentials_present ? t('settings.fetchCredentialsPresent') : t('settings.fetchNoCredentials') }}</p>
      </div>
      <label class="form-control w-full">
        <span class="label-text mb-2">{{ t('settings.fetchReplacement') }}</span>
        <input v-model="proxyURL" type="password" autocomplete="new-password" spellcheck="false" class="input input-bordered w-full min-w-0" :disabled="saving || clearProxy" placeholder="http://proxy.example:7890" data-testid="fetch-proxy-url" />
        <span class="text-xs text-base-content/60 mt-2">{{ t('settings.fetchRetainHint') }}</span>
      </label>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="clearProxy" type="checkbox" class="checkbox checkbox-sm" :disabled="saving" data-testid="fetch-clear-proxy" />
        {{ t('settings.fetchClear') }}
      </label>
      <div class="flex flex-wrap gap-3">
        <button class="btn btn-primary" :disabled="saving" @click="save()" data-testid="fetch-save">{{ saving ? t('settings.filterSaving') : t('settings.saveAndApply') }}</button>
        <button class="btn btn-ghost" :disabled="saving" @click="save(true)" data-testid="fetch-reset">{{ t('settings.fetchReset') }}</button>
      </div>
      <p class="text-xs text-base-content/60">{{ t('settings.fetchScope') }}</p>
    </template>
  </section>
</template>

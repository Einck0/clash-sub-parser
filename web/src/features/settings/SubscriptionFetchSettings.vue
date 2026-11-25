<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowDownTrayIcon, ArrowPathIcon } from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayText } from '../../utils/displayNames'
import { toastStore } from '../../ui/toast'
import { useServerSettings } from './useServerSettings'

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
const state = useServerSettings<FetchSettings>({
  read: () => api.get<FetchSettings>('/api/v1/settings/subscription-fetch'),
  apply,
  snapshot: () => ({ mode: mode.value, hasReplacement: proxyURL.value.length > 0, clear: clearProxy.value }),
})
const { loaded, loading, saving, error, readFailed } = state
const busy = computed(() => loading.value || saving.value)
const load = state.load

function apply(value: FetchSettings) {
  if (
    !value ||
    !['environment', 'direct', 'proxy'].includes(value.mode) ||
    !['direct', 'proxy'].includes(value.effective_mode) ||
    typeof value.credentials_present !== 'boolean' ||
    typeof value.effective_credentials_present !== 'boolean' ||
    typeof value.effective_source !== 'string' ||
    typeof value.configured_proxy_endpoint !== 'string' ||
    typeof value.effective_proxy_endpoint !== 'string'
  ) {
    throw new Error(t('settings.fetchInvalidResponse'))
  }
  settings.value = value
  mode.value = value.mode
  proxyURL.value = ''
  clearProxy.value = false
}

async function save(reset = false) {
  if (!settings.value || busy.value) return
  const payload: { mode: Mode; proxy_url?: string; clear_proxy_url?: boolean } = {
    mode: reset ? 'environment' : mode.value,
  }
  if (!reset) {
    if (clearProxy.value) payload.clear_proxy_url = true
    else if (proxyURL.value.trim()) payload.proxy_url = proxyURL.value.trim()
  }
  if (await state.save(() => api.put<FetchSettings>('/api/v1/settings/subscription-fetch', payload))) {
    toastStore.push({ message: t('settings.fetchSaved'), tone: 'success' })
  }
}

onMounted(() => { void load() })
</script>

<template>
  <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10 min-w-0" data-testid="subscription-fetch-settings">
    <div class="card-body p-5 sm:p-6 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/40 pb-3">
        <div class="flex items-center gap-2.5">
          <ArrowDownTrayIcon class="w-5 h-5 text-primary" />
          <div>
            <h3 class="text-sm sm:text-base font-bold">{{ t('settings.fetchTitle') }}</h3>
            <p class="text-xs text-base-content/60">{{ t('settings.fetchDescription') }}</p>
          </div>
        </div>
        <span class="badge badge-sm badge-outline font-mono text-[11px] text-base-content/70">
          {{ uiText('服务端配置', 'Server Configuration') }}
        </span>
      </div>

      <p v-if="loading" role="status" class="text-xs text-base-content/60 flex items-center gap-2">
        <span class="loading loading-spinner loading-xs" />
        <span>{{ t('common.loading') }}</span>
      </p>

      <div v-if="error" role="alert" class="alert alert-error text-xs flex flex-wrap items-center justify-between gap-2" data-testid="fetch-settings-error">
        <span class="min-w-0 whitespace-pre-wrap [overflow-wrap:anywhere]">{{ displayText(error) }}</span>
        <button type="button" class="btn btn-xs btn-ghost gap-1" :disabled="busy" @click="load">
          <ArrowPathIcon class="w-3.5 h-3.5" />
          <span>{{ t('common.refresh') }}</span>
        </button>
      </div>

      <button v-if="!loading && !settings && !error" class="btn btn-sm btn-outline gap-1.5" @click="load">
        <ArrowPathIcon class="w-4 h-4" />
        <span>{{ t('common.refresh') }}</span>
      </button>

      <fieldset v-if="loaded && settings" :disabled="busy || readFailed" class="space-y-4 min-w-0">
        <div class="space-y-1.5 w-full">
          <label for="fetch-mode" class="text-xs font-semibold text-base-content/80 block">{{ t('settings.fetchMode') }}</label>
          <select id="fetch-mode" v-model="mode" class="select select-sm sm:select-md w-full bg-base-100/80" :disabled="saving" data-testid="fetch-mode">
            <option value="environment">{{ t('settings.fetchEnvironment') }}</option>
            <option value="direct">{{ t('settings.fetchDirect') }}</option>
            <option value="proxy">{{ t('settings.fetchProxy') }}</option>
          </select>
        </div>

        <div class="text-xs sm:text-sm space-y-1.5 p-3 rounded-xl bg-base-100/60 border border-base-300/40" aria-live="polite">
          <p>
            {{ t('settings.fetchEffective') }}:
            <span class="font-mono font-semibold text-primary break-all" data-testid="fetch-effective">
              {{ settings.effective_proxy_endpoint || t('settings.fetchDirect') }}
            </span>
          </p>
          <p class="text-base-content/70">
            {{ t('settings.fetchSource') }}:
            <span class="font-medium">
              {{ settings.effective_source === 'environment' ? t('settings.fetchEnvironment') : settings.effective_source === 'direct' ? t('settings.fetchDirect') : t('settings.fetchProxy') }}
            </span>
          </p>
          <p v-if="settings.configured_proxy_endpoint" class="text-base-content/70 break-all">
            {{ t('settings.fetchConfigured') }}: {{ settings.configured_proxy_endpoint }} ·
            <span class="font-medium">{{ settings.credentials_present ? t('settings.fetchCredentialsPresent') : t('settings.fetchNoCredentials') }}</span>
          </p>
        </div>

        <div class="space-y-1.5 w-full">
          <label for="fetch-proxy-url" class="text-xs font-semibold text-base-content/80 block">{{ t('settings.fetchReplacement') }}</label>
          <input
            id="fetch-proxy-url"
            v-model="proxyURL"
            type="password"
            autocomplete="new-password"
            spellcheck="false"
            class="input input-sm sm:input-md w-full min-w-0 font-mono bg-base-100/80"
            :disabled="saving || clearProxy"
            placeholder="http://proxy.example:7890"
            data-testid="fetch-proxy-url"
          />
          <span class="text-[11px] text-base-content/60 mt-1 block">{{ t('settings.fetchRetainHint') }}</span>
        </div>

        <label class="flex items-center gap-2 text-xs sm:text-sm cursor-pointer select-none">
          <input
            v-model="clearProxy"
            type="checkbox"
            class="checkbox checkbox-sm checkbox-primary"
            :disabled="saving"
            data-testid="fetch-clear-proxy"
          />
          <span>{{ t('settings.fetchClear') }}</span>
        </label>

        <div class="flex flex-wrap gap-2.5 pt-2">
          <button
            class="btn btn-primary btn-sm gap-1.5"
            :disabled="saving"
            data-testid="fetch-save"
            @click="save()"
          >
            <span v-if="saving" class="loading loading-spinner loading-xs" />
            <span>{{ saving ? t('settings.filterSaving') : t('settings.saveAndApply') }}</span>
          </button>
          <button
            class="btn btn-ghost btn-sm"
            :disabled="saving"
            data-testid="fetch-reset"
            @click="save(true)"
          >
            {{ t('settings.fetchReset') }}
          </button>
        </div>

        <p class="text-[11px] text-base-content/60 leading-relaxed">{{ t('settings.fetchScope') }}</p>
      </fieldset>
    </div>
  </section>
</template>

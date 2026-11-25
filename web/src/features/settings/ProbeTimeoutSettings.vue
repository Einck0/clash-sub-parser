<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ClockIcon } from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { uiText } from '../../locales/redesign'
import { t } from '../../locales'
import { useServerSettings } from './useServerSettings'
import { displayText } from '../../utils/displayNames'

const probeTimeoutSeconds = ref(5)
const state = useServerSettings<{ probe_timeout_seconds: number }>({
  read: () => api.get('/api/v1/settings/probe-timeout'),
  apply: value => {
    if (!value || !Number.isInteger(value.probe_timeout_seconds) || value.probe_timeout_seconds < 1 || value.probe_timeout_seconds > 30) {
      throw new Error(uiText('服务端返回的超时设置无效', 'The server returned invalid timeout settings.'))
    }
    probeTimeoutSeconds.value = value.probe_timeout_seconds
  },
  snapshot: () => probeTimeoutSeconds.value,
})
const { loading, saving, loaded, readFailed, error: errorMessage } = state
const fetchProbeTimeout = state.load

async function saveProbeTimeout() {
  if (!Number.isInteger(probeTimeoutSeconds.value) || probeTimeoutSeconds.value < 1 || probeTimeoutSeconds.value > 30) {
    errorMessage.value = uiText('探针探测超时必须是 1 到 30 秒之间的整数', 'Probe timeout must be an integer between 1 and 30 seconds.')
    toastStore.push({ message: errorMessage.value, tone: 'error' })
    return
  }
  if (await state.save(() => api.put<{ probe_timeout_seconds: number }>('/api/v1/settings/probe-timeout', {
    probe_timeout_seconds: probeTimeoutSeconds.value,
  }))) {
    toastStore.push({ message: uiText('探针探测超时已保存', 'Probe timeout saved'), tone: 'success' })
  }
}

onMounted(() => {
  void fetchProbeTimeout()
})
</script>

<template>
  <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="probe-timeout-settings">
    <div class="card-body p-5 space-y-4">
      <div class="flex items-center gap-2.5">
        <ClockIcon class="w-5 h-5 text-primary" />
        <div>
          <h3 class="text-sm sm:text-base font-bold">{{ uiText('探针探测超时 (秒)', 'Probe timeout (seconds)') }}</h3>
          <p class="text-xs text-base-content/60">
            {{ uiText('设置单节点基础连通与出站握手的最大等待超时时间（范围 1~30 秒，默认 5 秒）。', 'Maximum wait for node connectivity and outbound handshakes (1–30 seconds, default 5).') }}
          </p>
        </div>
      </div>

      <p v-if="loading && !loaded" role="status" class="text-xs text-base-content/60">{{ t('common.loading') }}</p>
      <div class="space-y-2">
        <fieldset v-if="loaded" :disabled="loading || saving || readFailed" class="flex flex-col sm:flex-row gap-2 min-w-0">
          <label for="probe-timeout-input" class="sr-only">{{ uiText('探针探测超时（秒）', 'Probe timeout (seconds)') }}</label>
          <input
            id="probe-timeout-input"
            v-model.number="probeTimeoutSeconds"
            type="number"
            min="1"
            max="30"
            data-testid="probe-timeout-input"
            placeholder="5"
            class="input input-sm flex-1 font-mono text-xs bg-base-100/80 max-w-xs"
            :disabled="loading || saving"
          />
          <button
            type="button"
            data-testid="save-probe-timeout-btn"
            class="btn btn-primary btn-sm text-xs font-semibold"
            :disabled="loading || saving"
            @click="saveProbeTimeout"
          >
            <span v-if="saving" class="loading loading-spinner loading-xs" />
            <span>{{ uiText('保存超时设置', 'Save timeout') }}</span>
          </button>
        </fieldset>

        <div v-if="errorMessage" role="alert" class="text-xs text-error" data-testid="probe-timeout-error">
          <p class="whitespace-pre-wrap [overflow-wrap:anywhere]">{{ displayText(errorMessage) }}</p>
          <button type="button" class="btn btn-sm btn-ghost" :disabled="loading || saving" @click="fetchProbeTimeout">{{ t('common.retry') }}</button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ClockIcon } from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'

const probeTimeoutSeconds = ref(5)
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')

async function fetchProbeTimeout() {
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await api.get<{ probe_timeout_seconds: number }>('/api/v1/settings/probe-timeout')
    if (res && typeof res.probe_timeout_seconds === 'number' && res.probe_timeout_seconds >= 1 && res.probe_timeout_seconds <= 30) {
      probeTimeoutSeconds.value = res.probe_timeout_seconds
    }
  } catch (err: any) {
    errorMessage.value = err?.message || '获取探针超时设置失败'
  } finally {
    loading.value = false
  }
}

async function saveProbeTimeout() {
  if (probeTimeoutSeconds.value < 1 || probeTimeoutSeconds.value > 30) {
    errorMessage.value = '探针探测超时必须在 1 到 30 秒之间'
    toastStore.push({ message: errorMessage.value, tone: 'error' })
    return
  }
  saving.value = true
  errorMessage.value = ''
  try {
    const res = await api.put<{ probe_timeout_seconds: number }>('/api/v1/settings/probe-timeout', {
      probe_timeout_seconds: probeTimeoutSeconds.value,
    })
    if (res && typeof res.probe_timeout_seconds === 'number') {
      probeTimeoutSeconds.value = res.probe_timeout_seconds
    }
    toastStore.push({ message: '探针探测超时已保存', tone: 'success' })
  } catch (err: any) {
    errorMessage.value = err?.message || '保存失败'
    toastStore.push({ message: errorMessage.value, tone: 'error' })
  } finally {
    saving.value = false
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
          <h3 class="text-sm sm:text-base font-bold">探针探测超时 (秒)</h3>
          <p class="text-xs text-base-content/60">
            设置单节点基础连通与出站握手的最大等待超时时间（范围 1~30 秒，默认 5 秒）。
          </p>
        </div>
      </div>

      <div class="space-y-2">
        <div class="flex flex-col sm:flex-row gap-2">
          <input
            v-model.number="probeTimeoutSeconds"
            type="number"
            min="1"
            max="30"
            data-testid="probe-timeout-input"
            placeholder="5"
            class="input input-bordered input-sm flex-1 font-mono text-xs bg-base-100/80 max-w-xs"
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
            保存超时设置
          </button>
        </div>

        <p v-if="errorMessage" class="text-xs text-error font-mono">
          {{ errorMessage }}
        </p>
      </div>
    </div>
  </section>
</template>

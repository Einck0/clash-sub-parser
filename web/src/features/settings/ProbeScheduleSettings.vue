<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ArrowPathIcon, CalendarDaysIcon, CheckCircleIcon, ExclamationTriangleIcon } from '@heroicons/vue/24/outline'
import { api } from '../../api/client'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import type { ProbeKind, ProbeSchedule } from '../probes/probeTypes'

const ALL_KINDS: readonly { id: ProbeKind; labelKey: string }[] = [
  { id: 'baseline', labelKey: 'settings.probeKindBaseline' },
  { id: 'geo', labelKey: 'settings.probeKindGeo' },
  { id: 'streaming', labelKey: 'settings.probeKindStreaming' },
  { id: 'ai', labelKey: 'settings.probeKindAI' },
  { id: 'speed', labelKey: 'settings.probeKindSpeed' },
  { id: 'ip_risk', labelKey: 'settings.probeKindIPRisk' },
] as const

const enabled = ref(false)
const intervalSeconds = ref(3600)
const selectedKinds = ref<ProbeKind[]>(['baseline'])
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const lastUpdated = ref('')

function toggleKind(kind: ProbeKind) {
  const current = new Set(selectedKinds.value)
  if (current.has(kind)) {
    if (current.size <= 1) {
      errorMessage.value = '必须至少保留一个检测项目'
      return
    }
    current.delete(kind)
  } else {
    current.add(kind)
  }
  selectedKinds.value = Array.from(current)
  errorMessage.value = ''
}

async function fetchSchedule() {
  loading.value = true
  errorMessage.value = ''
  try {
    const res = await api.get<ProbeSchedule>('/api/v1/probes/schedule')
    if (res && typeof res === 'object') {
      enabled.value = !!res.enabled
      if (typeof res.interval_seconds === 'number' && res.interval_seconds >= 60 && res.interval_seconds <= 604800) {
        intervalSeconds.value = res.interval_seconds
      }
      if (Array.isArray(res.kinds) && res.kinds.length > 0) {
        selectedKinds.value = [...res.kinds]
      }
      lastUpdated.value = res.updated_at || ''
    }
  } catch (err: any) {
    errorMessage.value = err?.message || '获取探针巡检配置失败'
  } finally {
    loading.value = false
  }
}

async function saveSchedule() {
  const sec = Number(intervalSeconds.value)
  if (isNaN(sec) || sec < 60 || sec > 604800) {
    errorMessage.value = '默认检测周期必须在 60 到 604800 秒之间'
    toastStore.push({ message: errorMessage.value, tone: 'error' })
    return
  }

  if (selectedKinds.value.length === 0) {
    errorMessage.value = '必须至少选择一个检测项目'
    toastStore.push({ message: errorMessage.value, tone: 'error' })
    return
  }

  saving.value = true
  errorMessage.value = ''
  try {
    const res = await api.put<ProbeSchedule>('/api/v1/probes/schedule', {
      enabled: enabled.value,
      interval_seconds: sec,
      kinds: selectedKinds.value,
    })
    // Read-back verification from server response
    if (res && typeof res === 'object') {
      enabled.value = !!res.enabled
      if (typeof res.interval_seconds === 'number') {
        intervalSeconds.value = res.interval_seconds
      }
      if (Array.isArray(res.kinds) && res.kinds.length > 0) {
        selectedKinds.value = [...res.kinds]
      }
      lastUpdated.value = res.updated_at || ''
    }
    toastStore.push({ message: t('settings.probeScheduleSaved'), tone: 'success' })
  } catch (err: any) {
    errorMessage.value = err?.message || t('settings.probeScheduleSaveFailed')
    toastStore.push({ message: errorMessage.value, tone: 'error' })
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void fetchSchedule()
})
</script>

<template>
  <div class="space-y-4" data-testid="probe-schedule-settings">
    <!-- Auto Patrol Master Switch -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-3.5 rounded-xl bg-base-100/70 border border-base-300">
      <div class="space-y-0.5">
        <label class="text-xs sm:text-sm font-semibold flex items-center gap-1.5 cursor-pointer" for="probe-schedule-toggle">
          <span>{{ t('settings.probeAutoSwitch') }}</span>
          <span
            class="badge badge-xs"
            :class="enabled ? 'badge-success' : 'badge-ghost'"
          >
            {{ enabled ? '运行中' : '已停用' }}
          </span>
        </label>
        <p class="text-[11px] text-base-content/60 leading-relaxed max-w-xl">
          {{ t('settings.probeAutoSwitchHint') }}
        </p>
      </div>

      <input
        id="probe-schedule-toggle"
        v-model="enabled"
        type="checkbox"
        data-testid="probe-schedule-toggle"
        class="toggle toggle-primary toggle-sm sm:toggle-md shrink-0"
        :disabled="loading || saving"
      />
    </div>

    <!-- Default Interval & Kind Selection Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <!-- Default Interval -->
      <div class="space-y-2 p-3.5 rounded-xl bg-base-100/70 border border-base-300">
        <label class="text-xs sm:text-sm font-semibold block">
          {{ t('settings.probeDefaultInterval') }}
        </label>
        <p class="text-[11px] text-base-content/60">
          {{ t('settings.probeDefaultIntervalHint') }}
        </p>
        <div class="flex items-center gap-2 pt-1">
          <input
            v-model.number="intervalSeconds"
            type="number"
            min="60"
            max="604800"
            data-testid="probe-schedule-interval-input"
            class="input input-bordered input-sm font-mono text-xs w-full max-w-xs"
            placeholder="3600"
            :disabled="loading || saving"
          />
          <span class="text-xs font-mono text-base-content/60 shrink-0">
            ≈ {{ Math.round(intervalSeconds / 60) }} 分钟
          </span>
        </div>
      </div>

      <!-- Kinds Selection -->
      <div class="space-y-2 p-3.5 rounded-xl bg-base-100/70 border border-base-300">
        <label class="text-xs sm:text-sm font-semibold block">
          {{ t('settings.probeKinds') }}
        </label>
        <p class="text-[11px] text-base-content/60">
          勾选系统自动巡检时需要执行的能力观测维度：
        </p>
        <div class="grid grid-cols-2 gap-2 pt-1">
          <label
            v-for="item in ALL_KINDS"
            :key="item.id"
            class="flex items-center gap-2 cursor-pointer text-xs"
          >
            <input
              type="checkbox"
              :checked="selectedKinds.includes(item.id)"
              :data-testid="`probe-kind-checkbox-${item.id}`"
              class="checkbox checkbox-primary checkbox-xs"
              :disabled="loading || saving"
              @change="toggleKind(item.id)"
            />
            <span class="truncate">{{ t(item.labelKey) }}</span>
          </label>
        </div>
      </div>
    </div>

    <!-- Action Bar & Visible Error -->
    <div class="flex flex-wrap items-center justify-between gap-3 pt-1">
      <div class="flex items-center gap-2">
        <button
          type="button"
          data-testid="save-probe-schedule-btn"
          class="btn btn-primary btn-sm text-xs font-semibold gap-1.5"
          :disabled="loading || saving"
          @click="saveSchedule"
        >
          <span v-if="saving" class="loading loading-spinner loading-xs" />
          <span>{{ t('settings.saveProbeSchedule') }}</span>
        </button>

        <button
          type="button"
          class="btn btn-ghost btn-sm btn-square"
          :title="t('common.refresh')"
          :disabled="loading || saving"
          @click="fetchSchedule"
        >
          <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        </button>
      </div>

      <p v-if="errorMessage" class="text-xs text-error font-mono flex items-center gap-1" data-testid="probe-schedule-error">
        <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
        <span>{{ errorMessage }}</span>
      </p>
    </div>
  </div>
</template>

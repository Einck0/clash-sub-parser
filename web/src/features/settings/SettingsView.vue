<script setup lang="ts">
import { onMounted, ref } from 'vue'
import PageToolbar from '../../ui/PageToolbar.vue'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import SettingsSections from './SettingsSections.vue'
import PublicBaseUrlSettings from './PublicBaseUrlSettings.vue'
import SubscriptionFetchSettings from './SubscriptionFetchSettings.vue'
import ProbeScheduleSettings from './ProbeScheduleSettings.vue'
import ProbeTimeoutSettings from './ProbeTimeoutSettings.vue'
import GlobalNodeFilterSettings from './GlobalNodeFilterSettings.vue'
import AuthSecuritySettings from './AuthSecuritySettings.vue'
import AppearanceSettings from './AppearanceSettings.vue'
import { useAuthSettings } from './useAuthSettings'

export type SettingsCategory = 'connection' | 'probe' | 'filter' | 'security' | 'display'

const activeCategory = ref<SettingsCategory>('connection')

const categories: { id: SettingsCategory; labelZh: string; labelEn: string }[] = [
  { id: 'connection', labelZh: '连接与订阅', labelEn: 'Connection & Subscription' },
  { id: 'probe', labelZh: '节点检测', labelEn: 'Node Detection' },
  { id: 'filter', labelZh: '全局筛选', labelEn: 'Global Filter' },
  { id: 'security', labelZh: '访问安全', labelEn: 'Access & Security' },
  { id: 'display', labelZh: '外观与偏好', labelEn: 'Display & Preferences' },
]

const auth = useAuthSettings()

useUnsavedChanges(auth.isDirty, () =>
  uiText(
    '当前设置项有未保存的修改，离开将丢失这些草稿。确定要离开吗？',
    'You have unsaved changes in Settings. Leaving will discard drafts. Are you sure?'
  )
)

onMounted(() => {
  auth.syncStoredToken()
  void auth.probeAuthMode()
})
</script>

<template>
  <div class="space-y-6" data-testid="settings-view">
    <h2 class="sr-only">{{ t('settings.title') }}</h2>

    <!-- PageToolbar & Auth Mode Status Badge -->
    <PageToolbar :title="t('settings.title')" :description="t('settings.subtitle')" sr-only-title>
      <template #default>
        <div class="flex items-center gap-2">
          <span class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('settings.tag') }}</span>
          <span class="text-xs text-base-content/60">
            {{ uiText('系统偏好、认证安全与全局配置', 'System preferences, auth security & global configs') }}
          </span>
        </div>
      </template>
      <template #actions>
        <!-- Auth Mode Status Pill -->
        <div class="flex items-center gap-2">
          <span class="text-xs text-base-content/60 font-medium">{{ t('settings.authModeLabel') }}</span>
          <div
            data-testid="auth-mode-badge"
            class="badge gap-1.5 py-2.5 px-3 text-xs font-semibold uppercase tracking-wider"
            :class="{
              'badge-warning badge-outline': auth.authMode.value === 'open',
              'badge-success': auth.authMode.value === 'token',
              'badge-ghost animate-pulse': auth.authMode.value === 'loading',
              'badge-error': auth.authMode.value === 'error',
            }"
          >
            <span
              class="w-2 h-2 rounded-full"
              :class="{
                'bg-warning': auth.authMode.value === 'open',
                'bg-success-content': auth.authMode.value === 'token',
                'bg-base-content/40': auth.authMode.value === 'loading',
                'bg-error': auth.authMode.value === 'error',
              }"
            />
            <span>
              {{
                auth.authMode.value === 'open'
                  ? t('settings.authModeOpen')
                  : auth.authMode.value === 'token'
                  ? t('settings.authModeProtected')
                  : auth.authMode.value === 'loading'
                  ? t('settings.authModeProbing')
                  : t('settings.authModeError')
              }}
            </span>
          </div>
        </div>
      </template>
    </PageToolbar>

    <!-- Top Shared Navigation: CSP / DNS / Chains -->
    <SettingsSections active="csp" />

    <!-- Category Switcher: Mobile Select & Desktop Tabs -->
    <div class="sm:hidden w-full">
      <label class="sr-only" for="settings-category-select">{{ uiText('设置分类', 'Settings Category') }}</label>
      <select
        id="settings-category-select"
        v-model="activeCategory"
        class="select select-sm w-full font-medium bg-base-200/80"
        data-testid="settings-category-select"
      >
        <option v-for="cat in categories" :key="cat.id" :value="cat.id">
          {{ uiText(cat.labelZh, cat.labelEn) }}
        </option>
      </select>
    </div>

    <div class="hidden sm:flex items-center gap-1.5 border-b border-base-300 pb-2">
      <button
        v-for="cat in categories"
        :key="cat.id"
        type="button"
        class="btn btn-sm touch-manipulation transition-all"
        :class="activeCategory === cat.id ? 'btn-primary shadow-sm font-semibold' : 'btn-ghost text-base-content/70 hover:text-base-content'"
        :data-testid="`settings-tab-${cat.id}`"
        @click="activeCategory = cat.id"
      >
        {{ uiText(cat.labelZh, cat.labelEn) }}
      </button>
    </div>

    <!-- 1. Connection & Subscription Panel -->
    <div v-show="activeCategory === 'connection'" class="space-y-6" data-testid="settings-panel-connection">
      <PublicBaseUrlSettings />
      <SubscriptionFetchSettings />
    </div>

    <!-- 2. Node Detection Panel -->
    <div v-show="activeCategory === 'probe'" class="space-y-6" data-testid="settings-panel-probe">
      <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="probe-schedule-card">
        <div class="card-body p-5 sm:p-6 space-y-4">
          <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/40 pb-3">
            <div>
              <h3 class="text-sm sm:text-base font-bold">{{ t('settings.probeScheduleTitle') }}</h3>
              <p class="text-xs text-base-content/60">{{ t('settings.probeScheduleSubtitle') }}</p>
            </div>
            <span class="badge badge-sm badge-outline font-mono text-[11px] text-base-content/70">
              {{ uiText('服务端配置', 'Server Configuration') }}
            </span>
          </div>
          <ProbeScheduleSettings />
        </div>
      </section>
      <ProbeTimeoutSettings />
    </div>

    <!-- 3. Global Filter Panel -->
    <div v-show="activeCategory === 'filter'" class="space-y-6" data-testid="settings-panel-filter">
      <GlobalNodeFilterSettings />
    </div>

    <!-- 4. Access & Security Panel -->
    <div v-show="activeCategory === 'security'" class="space-y-6" data-testid="settings-panel-security">
      <AuthSecuritySettings :auth="auth" />
    </div>

    <!-- 5. Appearance & Preferences Panel -->
    <div v-show="activeCategory === 'display'" class="space-y-6" data-testid="settings-panel-display">
      <AppearanceSettings />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink, useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import PageToolbar from '../../ui/PageToolbar.vue'
import { provideSettingsDrafts } from './useSettingsDraft'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import PublicBaseUrlSettings from './PublicBaseUrlSettings.vue'
import SubscriptionFetchSettings from './SubscriptionFetchSettings.vue'
import ProbeScheduleSettings from './ProbeScheduleSettings.vue'
import ProbeTimeoutSettings from './ProbeTimeoutSettings.vue'
import GlobalNodeFilterSettings from './GlobalNodeFilterSettings.vue'
import AuthSecuritySettings from './AuthSecuritySettings.vue'
import AppearanceSettings from './AppearanceSettings.vue'
import { useAuthSettings } from './useAuthSettings'

export type SettingsCategory = 'connection' | 'probe' | 'filter' | 'security' | 'display'

const categories: { id: SettingsCategory; labelZh: string; labelEn: string }[] = [
  { id: 'connection', labelZh: '连接与订阅', labelEn: 'Connection & Subscription' },
  { id: 'probe', labelZh: '节点检测', labelEn: 'Node Detection' },
  { id: 'filter', labelZh: '全局筛选与空组 PASS', labelEn: 'Global Filter & Empty Group PASS' },
  { id: 'security', labelZh: '访问安全', labelEn: 'Access & Security' },
  { id: 'display', labelZh: '外观与偏好', labelEn: 'Display & Preferences' },
]

const route = useRoute()
const router = useRouter()
const draftRegistry = provideSettingsDrafts()
const draftsDirty = draftRegistry.dirty
const activeCategory = computed<SettingsCategory>(() => {
  const section = route?.query.section
  return categories.some(category => category.id === section) ? section as SettingsCategory : 'connection'
})
function categoryLocation(section: SettingsCategory) {
  return { path: '/settings', query: { ...route?.query, section } as LocationQueryRaw }
}
function selectCategory(event: Event) {
  const select = event.target as HTMLSelectElement
  const category = select.value as SettingsCategory
  // The router is the source of truth, including rejected/cancelled navigation.
  select.value = activeCategory.value
  void router.push(categoryLocation(category))
}

const auth = useAuthSettings((dirty) => !draftRegistry.hasOtherDrafts(dirty) || window.confirm(uiText(
  '认证设置保存成功后将重新连接控制台，其他分区的未保存草稿会丢失。确定继续吗？',
  'Saving authentication reconnects the console and discards unsaved drafts in other sections. Continue?'
)))

onMounted(() => {
  auth.syncStoredToken()
  void auth.probeAuthMode()
})
</script>

<template>
  <div class="space-y-6" data-testid="settings-view" :aria-busy="auth.isSavingSwitches.value || auth.isSavingToken.value || auth.isClearingToken.value">
    <!-- PageToolbar & Auth Mode Status Badge -->
    <PageToolbar :title="t('settings.title')" :description="t('settings.subtitle')">
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

    <!-- Category Switcher: Mobile Select & Desktop Tabs -->
    <div class="sm:hidden w-full">
      <label class="sr-only" for="settings-category-select">{{ uiText('设置分类', 'Settings Category') }}</label>
      <select
        id="settings-category-select"
        :value="activeCategory"
        @change="selectCategory"
        class="select select-sm w-full font-medium bg-base-200/80"
        data-testid="settings-category-select"
      >
        <option v-for="cat in categories" :key="cat.id" :value="cat.id">
          {{ uiText(cat.labelZh, cat.labelEn) }}
        </option>
      </select>
    </div>

    <nav class="hidden sm:flex flex-wrap items-center gap-1.5 border-b border-base-300 pb-2" :aria-label="uiText('设置分类', 'Settings categories')">
      <RouterLink
        v-for="cat in categories"
        :key="cat.id"
        :to="categoryLocation(cat.id)"
        :aria-current="activeCategory === cat.id ? 'page' : undefined"
        :class="activeCategory === cat.id ? 'btn-primary shadow-sm font-semibold' : 'btn-ghost text-base-content/70 hover:text-base-content'"
        class="btn btn-sm touch-manipulation transition-all"
        :data-testid="`settings-tab-${cat.id}`"
      >
        {{ uiText(cat.labelZh, cat.labelEn) }}
      </RouterLink>
    </nav>

    <p v-if="draftsDirty" class="text-xs text-warning" role="status" data-testid="settings-unsaved-summary">{{ uiText('有未保存的设置。切换分区会保留草稿，离开设置或刷新页面前请保存。', 'Unsaved settings. Switching sections keeps drafts; save before leaving Settings or refreshing.') }}</p>

    <fieldset :disabled="auth.isSavingSwitches.value || auth.isSavingToken.value || auth.isClearingToken.value" class="space-y-6 min-w-0">
    <!-- Keep panels mounted: query/back navigation must not discard hidden drafts. -->
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
    </fieldset>
  </div>
</template>

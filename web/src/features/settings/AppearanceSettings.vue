<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  PaintBrushIcon,
  LanguageIcon,
  BoltIcon,
  GlobeAltIcon,
  CheckIcon,
} from '@heroicons/vue/24/outline'
import { t, getLocale, setLocale, type Locale } from '../../locales'
import { uiText } from '../../locales/redesign'
import { applyTheme, getStoredTheme, THEME_NAMES, type ThemeName } from '../../theme'

const router = useRouter()
const currentLocale = ref<Locale>(getLocale())
const currentTheme = ref<ThemeName>(getStoredTheme())

function themeLabel(theme: ThemeName): string {
  switch (theme) {
    case 'light':
      return t('themes.light')
    case 'dark':
      return t('themes.dark')
    case 'dim':
      return t('themes.dim')
    case 'cyberpunk':
      return t('themes.cyberpunk')
    case 'cupcake':
      return t('themes.cupcake')
    case 'dracula':
      return t('themes.dracula')
    case 'nord':
      return t('themes.nord')
    default:
      return theme
  }
}

function handleLocaleChange(loc: Locale) {
  currentLocale.value = loc
  setLocale(loc)
}

function handleThemeChange(theme: ThemeName) {
  currentTheme.value = theme
  applyTheme(theme)
}

function goToDns() {
  if (router) {
    void router.push('/dns')
  } else if (typeof window !== 'undefined') {
    window.location.hash = '#/dns'
  }
}
</script>

<template>
  <div class="space-y-6" data-testid="appearance-settings">
    <!-- Notice distinguishing local preferences from server config -->
    <div class="p-3.5 rounded-xl bg-info/10 border border-info/20 text-xs text-info-content flex items-start gap-2.5">
      <BoltIcon class="w-4 h-4 text-info shrink-0 mt-0.5" />
      <div class="space-y-0.5">
        <span class="font-semibold">{{ uiText('本机显示偏好说明', 'Local Preferences Note') }}</span>
        <p class="text-base-content/70 leading-relaxed">
          {{ uiText('外观主题与界面语言仅保存在当前浏览器的本地存储中，即时生效，无需向服务端提交保存。', 'Appearance theme and UI language are stored locally in the current browser and apply immediately without server calls.') }}
        </p>
      </div>
    </div>

    <!-- 1. Theme Selection -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="theme-settings-card">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex items-center gap-2.5 border-b border-base-300/40 pb-3">
          <PaintBrushIcon class="w-5 h-5 text-primary" />
          <div>
            <h3 class="font-bold text-base">{{ t('settings.themeTitle') }}</h3>
            <p class="text-xs text-base-content/60">{{ t('settings.themeSubtitle') }}</p>
          </div>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-2.5">
          <button
            v-for="theme in THEME_NAMES"
            :key="theme"
            type="button"
            class="flex flex-col items-center justify-center p-3 rounded-xl border text-xs font-medium transition-all gap-1.5 touch-manipulation text-left"
            :class="
              currentTheme === theme
                ? 'border-primary bg-primary/10 text-primary font-bold shadow-xs'
                : 'border-base-300/60 bg-base-100/50 hover:bg-base-100 text-base-content/80'
            "
            @click="handleThemeChange(theme)"
          >
            <div class="flex items-center justify-between w-full">
              <span class="capitalize text-xs">{{ themeLabel(theme) }}</span>
              <CheckIcon v-if="currentTheme === theme" class="w-3.5 h-3.5 text-primary shrink-0" />
            </div>
            <span class="text-[10px] text-base-content/40 font-mono self-start">{{ theme }}</span>
          </button>
        </div>
      </div>
    </section>

    <!-- 2. Language Selection -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="language-settings-card">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex items-center gap-2.5 border-b border-base-300/40 pb-3">
          <LanguageIcon class="w-5 h-5 text-primary" />
          <div>
            <h3 class="font-bold text-base">{{ t('settings.languageTitle') }}</h3>
            <p class="text-xs text-base-content/60">{{ t('settings.languageSubtitle') }}</p>
          </div>
        </div>

        <div class="flex flex-wrap gap-3">
          <button
            type="button"
            class="btn btn-sm gap-2"
            :class="currentLocale === 'zh-CN' ? 'btn-primary font-bold' : 'btn-outline border-base-300'"
            @click="handleLocaleChange('zh-CN')"
          >
            <CheckIcon v-if="currentLocale === 'zh-CN'" class="w-4 h-4" />
            <span>简体中文 (zh-CN)</span>
          </button>
          <button
            type="button"
            class="btn btn-sm gap-2"
            :class="currentLocale === 'en-US' ? 'btn-primary font-bold' : 'btn-outline border-base-300'"
            @click="handleLocaleChange('en-US')"
          >
            <CheckIcon v-if="currentLocale === 'en-US'" class="w-4 h-4" />
            <span>English (en-US)</span>
          </button>
        </div>
      </div>
    </section>

    <!-- 3. Runtime Environment Info -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="runtime-info">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex items-center gap-2.5 border-b border-base-300/40 pb-3">
          <BoltIcon class="w-5 h-5 text-primary" />
          <div>
            <h3 class="font-bold text-base">{{ t('settings.runtimeEnvironment') }}</h3>
            <p class="text-xs text-base-content/60">{{ t('settings.runtimeEnvironmentSubtitle') }}</p>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
          <div class="p-3.5 rounded-xl bg-base-100/60 border border-base-300/40 space-y-1">
            <span class="text-base-content/60 font-medium">{{ t('settings.runtimeArchitecture') }}</span>
            <p class="font-mono text-sm font-semibold text-primary">Single-Binary Go 1.22+</p>
            <p class="text-[11px] text-base-content/50">{{ t('settings.runtimeArchitectureDesc') }}</p>
          </div>
          <div class="p-3.5 rounded-xl bg-base-100/60 border border-base-300/40 space-y-1">
            <span class="text-base-content/60 font-medium">{{ t('settings.runtimeAssetPackaging') }}</span>
            <p class="font-mono text-sm font-semibold text-secondary">internal/webassets (Embedded)</p>
            <p class="text-[11px] text-base-content/50">{{ t('settings.runtimeAssetPackagingDesc') }}</p>
          </div>
        </div>
      </div>
    </section>

    <!-- 4. Quick Jump to DNS -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="settings-dns-card">
      <div class="card-body p-5 sm:p-6 space-y-3">
        <div class="flex items-center gap-2.5">
          <GlobeAltIcon class="w-5 h-5 text-primary" />
          <div>
            <h3 class="font-bold text-base">{{ t('settings.dnsSectionTitle') }}</h3>
            <p class="text-xs text-base-content/60">{{ t('settings.dnsSectionDesc') }}</p>
          </div>
        </div>
        <div>
          <button
            type="button"
            class="btn btn-primary btn-sm gap-2"
            data-testid="jump-to-dns-btn"
            @click="goToDns"
          >
            <GlobeAltIcon class="w-4 h-4" />
            <span>{{ t('settings.dnsJumpBtn') }}</span>
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

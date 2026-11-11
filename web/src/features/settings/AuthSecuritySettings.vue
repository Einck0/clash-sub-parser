<script setup lang="ts">
import {
  ShieldCheckIcon,
  ExclamationTriangleIcon,
  KeyIcon,
  EyeIcon,
  EyeSlashIcon,
  TrashIcon,
  ArrowPathIcon,
  CommandLineIcon,
} from '@heroicons/vue/24/outline'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { useAuthSettings, type UseAuthSettingsReturn } from './useAuthSettings'

const props = defineProps<{
  auth?: UseAuthSettingsReturn
}>()

const auth = props.auth ?? useAuthSettings()
</script>

<template>
  <div class="space-y-6" data-testid="auth-security-settings">
    <div role="note" class="alert alert-warning" data-testid="node-credentials-warning">
      <ExclamationTriangleIcon class="w-5 h-5 shrink-0" aria-hidden="true" />
      <span>{{ t('settings.nodeCredentialsWarning') }}</span>
    </div>

    <!-- 1. Server Protection Policy Switches -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="auth-switches">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/40 pb-3">
          <div class="flex items-center gap-2.5">
            <ShieldCheckIcon class="w-5 h-5 text-primary" />
            <div>
              <h3 class="font-bold text-base">{{ t('settings.protectionTitle') }}</h3>
              <p class="text-xs text-base-content/60">
                {{ uiText('控制控制台管理接口与订阅导出接口的鉴权要求', 'Configure authentication boundaries for management and export endpoints') }}
              </p>
            </div>
          </div>
          <span class="badge badge-sm badge-outline font-mono text-[11px] text-base-content/70">
            {{ uiText('服务端配置', 'Server Configuration') }}
          </span>
        </div>

        <div v-if="auth.settingsError.value" role="alert" class="alert alert-error text-xs flex items-center justify-between">
          <span>{{ auth.settingsError.value }}</span>
          <button type="button" class="btn btn-xs btn-ghost" @click="auth.probeAuthMode">
            {{ t('common.retry') }}
          </button>
        </div>

        <div class="space-y-3 pt-1">
          <label class="flex items-center justify-between gap-4 p-3 rounded-xl bg-base-100/60 border border-base-300/40 cursor-pointer hover:bg-base-100/80 transition-colors">
            <div class="space-y-0.5">
              <span class="text-sm font-semibold text-base-content">{{ t('settings.adminProtection') }}</span>
              <p class="text-xs text-base-content/60">{{ uiText('关闭后无需令牌即可直接进入控制台并调用管理 API', 'When disabled, management APIs are accessible without credentials') }}</p>
            </div>
            <input
              v-model="auth.adminEnabled.value"
              data-testid="admin-auth-switch"
              type="checkbox"
              class="toggle toggle-primary"
              :disabled="!auth.authSettings.value || auth.isSavingSwitches.value"
            />
          </label>

          <label class="flex items-center justify-between gap-4 p-3 rounded-xl bg-base-100/60 border border-base-300/40 cursor-pointer hover:bg-base-100/80 transition-colors">
            <div class="space-y-0.5">
              <span class="text-sm font-semibold text-base-content">{{ t('settings.exportProtection') }}</span>
              <p class="text-xs text-base-content/60">{{ uiText('关闭后直链拉取、节点订阅导出均完全公开', 'When disabled, export links and node subscriptions are fully public') }}</p>
            </div>
            <input
              v-model="auth.exportEnabled.value"
              data-testid="export-auth-switch"
              type="checkbox"
              class="toggle toggle-primary"
              :disabled="!auth.authSettings.value || auth.isSavingSwitches.value"
            />
          </label>
        </div>

        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-2">
          <div class="space-y-1 text-xs">
            <p data-testid="export-mode-badge" class="font-medium text-base-content/80">
              {{ t('settings.exportMode') }}:
              <span class="font-semibold text-primary">
                {{ auth.authSettings.value ? t(auth.authSettings.value.export_mode === 'open' ? 'settings.exportOpen' : 'settings.exportProtected') : '—' }}
              </span>
            </p>
            <p v-if="auth.authSettings.value?.admin_auth_enabled && !auth.authSettings.value.token_configured" class="text-warning flex items-center gap-1 font-medium">
              <span>⚠</span> {{ t('settings.zeroConfigWarning') }}
            </p>
            <p v-if="!auth.exportEnabled.value" class="text-warning flex items-center gap-1 font-medium">
              <span>⚠</span> {{ t('settings.publicWarning') }}
            </p>
          </div>

          <button
            type="button"
            class="btn btn-primary btn-sm self-start sm:self-auto gap-2"
            data-testid="save-auth-switches-btn"
            :disabled="!auth.authSettings.value || auth.isSavingSwitches.value"
            @click="auth.saveSwitches"
          >
            <span v-if="auth.isSavingSwitches.value" class="loading loading-spinner loading-xs" />
            <span>{{ t('settings.saveAndApply') }}</span>
          </button>
        </div>
      </div>
    </section>

    <!-- 2. Shared Access Token Management -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="token-config-card">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/40 pb-3">
          <div class="flex items-center gap-2.5">
            <KeyIcon class="w-5 h-5 text-primary" />
            <div>
              <h3 class="font-bold text-base">{{ t('settings.tokenConfigTitle') }}</h3>
              <p class="text-xs text-base-content/60">{{ t('settings.tokenConfigSubtitle') }}</p>
            </div>
          </div>
          <span class="badge badge-sm badge-outline font-mono text-[11px] text-base-content/70">
            {{ uiText('服务端与本机凭据', 'Server & Local Credential') }}
          </span>
        </div>

        <!-- Token Status & Display -->
        <div class="p-3.5 rounded-xl bg-base-100/60 border border-base-300/40 space-y-2">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-xs font-semibold text-base-content/70">{{ t('settings.currentToken') }}</span>
            <span
              data-testid="token-status-badge"
              class="badge badge-sm text-[11px] font-medium"
              :class="auth.storedToken.value ? 'badge-success badge-outline' : 'badge-ghost'"
            >
              {{ auth.storedToken.value ? t('settings.tokenStatusActive') : t('settings.tokenStatusNotConfigured') }}
            </span>
          </div>

          <div class="flex items-center justify-between gap-2 bg-base-200/80 px-3 py-2 rounded-lg font-mono text-xs">
            <span
              data-testid="stored-token-display"
              class="select-all break-all"
              :class="auth.storedToken.value ? 'text-base-content font-semibold' : 'text-base-content/40 italic'"
            >
              {{ auth.displayToken.value }}
            </span>
            <button
              v-if="auth.storedToken.value"
              type="button"
              class="btn btn-ghost btn-xs btn-square shrink-0"
              data-testid="toggle-token-visibility-btn"
              :title="auth.showStoredToken.value ? t('settings.hideToken') : t('settings.showToken')"
              @click="auth.toggleTokenVisibility"
            >
              <EyeSlashIcon v-if="auth.showStoredToken.value" class="w-4 h-4 text-base-content/70" />
              <EyeIcon v-else class="w-4 h-4 text-base-content/70" />
            </button>
          </div>
        </div>

        <!-- Update / Set Token Form -->
        <div class="space-y-2">
          <label class="text-xs font-semibold text-base-content/80 block" for="new-token-input">
            {{ t('settings.setNewToken') }}
          </label>
          <div class="flex flex-col sm:flex-row gap-2">
            <input
              id="new-token-input"
              v-model="auth.inputToken.value"
              data-testid="settings-token-input"
              type="password"
              class="input input-sm flex-1 font-mono text-xs bg-base-100/80"
              :placeholder="t('settings.tokenPlaceholder')"
              autocomplete="new-password"
              spellcheck="false"
              :disabled="auth.isSavingToken.value || auth.isClearingToken.value"
            />
            <div class="flex gap-2">
              <button
                type="button"
                class="btn btn-primary btn-sm flex-1 sm:flex-initial gap-1.5"
                data-testid="save-token-btn"
                :disabled="!auth.inputToken.value.trim() || auth.isSavingToken.value || auth.isClearingToken.value"
                @click="auth.saveToken"
              >
                <span v-if="auth.isSavingToken.value" class="loading loading-spinner loading-xs" />
                <span>{{ t('settings.saveToken') }}</span>
              </button>
              <button
                type="button"
                class="btn btn-outline btn-error btn-sm gap-1.5"
                data-testid="clear-token-btn"
                :disabled="!auth.authSettings.value?.token_configured && !auth.storedToken.value"
                @click="auth.clearToken"
              >
                <TrashIcon class="w-3.5 h-3.5" />
                <span>{{ t('settings.clearToken') }}</span>
              </button>
            </div>
          </div>
          <p class="text-[11px] text-base-content/50 leading-relaxed">
            {{ uiText('设置新令牌或清空令牌将立即使既有会话 Cookie 与 CSRF 令牌失效，服务端绝不回显任何明文密钥。', 'Setting a new token or clearing it immediately invalidates session cookies and CSRF tokens. The server never returns plaintext secrets.') }}
          </p>
        </div>
      </div>
    </section>

    <!-- 3. Backend Reachability & Diagnostic Test -->
    <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="connection-test-card">
      <div class="card-body p-5 sm:p-6 space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300/40 pb-3">
          <div class="flex items-center gap-2.5">
            <CommandLineIcon class="w-5 h-5 text-primary" />
            <div>
              <h3 class="font-bold text-base">{{ t('settings.testConnectionTitle') }}</h3>
              <p class="text-xs text-base-content/60">{{ t('settings.testConnectionSubtitle') }}</p>
            </div>
          </div>
          <span class="badge badge-sm badge-outline font-mono text-[11px] text-base-content/70">
            {{ uiText('在线探测', 'Diagnostics') }}
          </span>
        </div>

        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <p class="text-xs text-base-content/70">
            {{ uiText('调用控制面 /api/v1/auth/status 探测当前端点是否可达及服务端有效鉴权模式。', 'Calls /api/v1/auth/status to verify backend reachability and active authentication mode.') }}
          </p>
          <button
            type="button"
            class="btn btn-outline btn-sm gap-2 shrink-0 self-start sm:self-auto"
            data-testid="test-connection-btn"
            :disabled="auth.isTesting.value"
            @click="auth.testConnection"
          >
            <span v-if="auth.isTesting.value" class="loading loading-spinner loading-xs" />
            <ArrowPathIcon v-else class="w-4 h-4" />
            <span>{{ t('settings.testConnection') }}</span>
          </button>
        </div>

        <div
          v-if="auth.connectionStatus.value !== 'idle'"
          data-testid="connection-result"
          class="alert text-xs py-2.5 px-4"
          :class="{
            'alert-success': auth.connectionStatus.value === 'success',
            'alert-error': auth.connectionStatus.value === 'error',
          }"
        >
          <span>{{ auth.connectionMessage.value }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

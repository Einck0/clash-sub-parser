<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import {
  LockClosedIcon,
  EyeIcon,
  EyeSlashIcon,
  ExclamationCircleIcon,
  ArrowPathIcon,
  ArrowRightOnRectangleIcon,
} from '@heroicons/vue/24/outline'
import { useAuth } from './useAuth'
import { uiText } from '../../locales'
import { displayText } from '../../utils/displayNames'

const { login, clearStoredToken, errorMessage } = useAuth()

const token = ref('')
const showPassword = ref(false)
const isSubmitting = ref(false)
const isResetting = ref(false)
const isBusy = computed(() => isSubmitting.value || isResetting.value)
const inputRef = ref<HTMLInputElement>()
const safeErrorMessage = computed(() => displayText(errorMessage.value))

onMounted(async () => {
  await nextTick()
  inputRef.value?.focus()
})

function toggleVisibility() {
  showPassword.value = !showPassword.value
}

async function handleSubmit() {
  if (isBusy.value) return
  showPassword.value = false
  isSubmitting.value = true
  try {
    if (await login(token.value)) token.value = ''
  } finally {
    isSubmitting.value = false
  }
}

async function handleReset() {
  if (isBusy.value) return
  token.value = ''
  showPassword.value = false
  isResetting.value = true
  try {
    await clearStoredToken()
  } finally {
    isResetting.value = false
    await nextTick()
    inputRef.value?.focus()
  }
}
</script>

<template>
  <div
    class="max-w-md w-full mx-auto my-8 sm:my-16 p-6 sm:p-8 bg-base-200 border border-base-300 rounded-2xl shadow-xl space-y-6"
    data-testid="auth-gate"
  >
    <!-- Header Icon & Title -->
    <div class="flex items-start gap-4">
      <div class="p-3 rounded-xl bg-primary/10 text-primary border border-primary/20 shrink-0">
        <LockClosedIcon class="w-7 h-7" />
      </div>
      <div>
        <h2 class="text-lg sm:text-xl font-bold tracking-tight">{{ uiText('登录控制台', 'Sign in to the console') }}</h2>
        <p class="text-xs text-base-content/70 mt-1 leading-relaxed">
          {{ uiText('此服务需要管理员令牌才能继续。输入令牌以验证身份并打开控制台。', 'An administrator token is required. Enter it to verify your access and open the console.') }}
        </p>
      </div>
    </div>

    <!-- Error Alert -->
    <div
      v-if="safeErrorMessage"
      class="alert alert-error text-xs py-3 px-3.5 rounded-lg flex items-center gap-2 border border-error/30 shadow-sm"
      data-testid="auth-gate-error"
      id="auth-gate-error-message"
      role="alert"
    >
      <ExclamationCircleIcon class="w-4 h-4 shrink-0 text-error-content" />
      <span class="font-medium text-error-content min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{{ safeErrorMessage }}</span>
    </div>

    <!-- Login Form -->
    <form @submit.prevent="handleSubmit" class="space-y-4" :aria-busy="isBusy">
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between py-0.5">
          <label for="auth-gate-token-input" class="text-xs font-medium text-base-content flex items-center gap-1 cursor-pointer">
            {{ uiText('管理员令牌', 'Administrator token') }}
          </label>
          <span class="text-base-content/50 text-[11px]">{{ uiText('区分大小写', 'Case-sensitive') }}</span>
        </div>
        <div class="relative flex items-center">
          <input
            id="auth-gate-token-input"
            ref="inputRef"
            v-model="token"
            :type="showPassword ? 'text' : 'password'"
            class="input input-sm sm:input-md w-full pr-10 font-mono text-sm focus:input-primary transition-colors"
            :placeholder="uiText('请输入管理员令牌', 'Enter your administrator token')"
            autocomplete="current-password"
            data-testid="auth-gate-token-input"
            :disabled="isBusy"
            :aria-invalid="Boolean(safeErrorMessage)"
            :aria-describedby="safeErrorMessage ? 'auth-gate-error-message' : undefined"
          />
          <button
            type="button"
            class="btn btn-ghost btn-xs sm:btn-sm btn-circle absolute right-1.5 text-base-content/60 hover:text-base-content"
            data-testid="auth-gate-toggle-visibility"
            :aria-label="showPassword ? uiText('隐藏令牌', 'Hide token') : uiText('显示令牌', 'Show token')"
            :aria-pressed="showPassword"
            :disabled="isBusy"
            @click="toggleVisibility"
          >
            <component :is="showPassword ? EyeSlashIcon : EyeIcon" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <div class="pt-2 flex flex-col gap-2">
        <button
          type="submit"
          class="btn btn-primary btn-sm sm:btn-md w-full text-xs sm:text-sm font-semibold shadow-sm gap-2"
          data-testid="auth-gate-submit"
          :disabled="isBusy"
        >
          <ArrowPathIcon v-if="isSubmitting" class="w-4 h-4 animate-spin" />
          <ArrowRightOnRectangleIcon v-else class="w-4 h-4" />
          <span>{{ isSubmitting ? uiText('正在验证…', 'Verifying…') : uiText('验证并登录', 'Verify and sign in') }}</span>
        </button>
      </div>
    </form>

    <div class="divider text-xs text-base-content/40 my-2">{{ uiText('本地恢复', 'Local recovery') }}</div>

    <!-- Escape Hatch -->
    <div class="bg-base-100/70 p-3.5 rounded-xl border border-base-300 space-y-2">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div class="text-[11px] text-base-content/70">
          {{ uiText('无法登录时，可清除此设备保存的令牌并重新检查服务状态。', 'If sign-in is stuck, clear the token saved on this device and check the service again.') }}
        </div>
        <button
          type="button"
          class="btn btn-outline btn-ghost btn-xs shrink-0 text-error hover:bg-error/10 hover:border-error"
          data-testid="auth-gate-reset"
          :disabled="isBusy"
          :aria-busy="isResetting"
          @click="handleReset"
        >
          {{ isResetting ? uiText('正在重新检查…', 'Checking again…') : uiText('清除此设备上的令牌', 'Clear token on this device') }}
        </button>
      </div>
    </div>
  </div>
</template>

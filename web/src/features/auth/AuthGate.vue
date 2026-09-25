<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import {
  LockClosedIcon,
  KeyIcon,
  EyeIcon,
  EyeSlashIcon,
  ExclamationCircleIcon,
  ArrowPathIcon,
  ArrowRightOnRectangleIcon,
} from '@heroicons/vue/24/outline'
import { useAuth } from './useAuth'
import { t } from '../../locales'

const { login, clearStoredToken, errorMessage } = useAuth()

const token = ref('')
const showPassword = ref(false)
const isSubmitting = ref(false)
const inputRef = ref<HTMLInputElement>()

onMounted(async () => {
  await nextTick()
  inputRef.value?.focus()
})

function toggleVisibility() {
  showPassword.value = !showPassword.value
}

async function handleSubmit() {
  if (isSubmitting.value) return
  isSubmitting.value = true
  try {
    await login(token.value)
  } finally {
    isSubmitting.value = false
  }
}

function handleReset() {
  token.value = ''
  clearStoredToken()
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
        <h2 class="text-lg sm:text-xl font-bold tracking-tight">控制面受限访问门禁</h2>
        <p class="text-xs text-base-content/70 mt-1 leading-relaxed">
          当前服务端处于受控保护模式 (Protected Mode)，请输入 Admin Token 以完成身份核验并挂载业务控制台。
        </p>
      </div>
    </div>

    <!-- Error Alert -->
    <div
      v-if="errorMessage"
      class="alert alert-error text-xs py-3 px-3.5 rounded-lg flex items-center gap-2 border border-error/30 shadow-sm"
      data-testid="auth-gate-error"
      role="alert"
    >
      <ExclamationCircleIcon class="w-4 h-4 shrink-0 text-error-content" />
      <span class="font-medium text-error-content">{{ errorMessage }}</span>
    </div>

    <!-- Login Form -->
    <form @submit.prevent="handleSubmit" class="space-y-4">
      <div class="form-control">
        <label class="label py-1" for="auth-gate-token-input">
          <span class="label-text font-medium text-xs flex items-center gap-1">
            <KeyIcon class="w-3.5 h-3.5 text-primary" /> Admin Token 凭据
          </span>
          <span class="label-text-alt text-base-content/50 text-[11px] font-mono">Authorization: Bearer</span>
        </label>
        <div class="relative flex items-center">
          <input
            id="auth-gate-token-input"
            ref="inputRef"
            v-model="token"
            :type="showPassword ? 'text' : 'password'"
            class="input input-bordered input-sm sm:input-md w-full pr-10 font-mono text-sm focus:input-primary transition-colors"
            placeholder="请输入管理员令牌..."
            autocomplete="current-password"
            data-testid="auth-gate-token-input"
            :disabled="isSubmitting"
          />
          <button
            type="button"
            class="btn btn-ghost btn-xs sm:btn-sm btn-circle absolute right-1.5 text-base-content/60 hover:text-base-content"
            data-testid="auth-gate-toggle-visibility"
            :aria-label="showPassword ? '隐藏明文' : '显示明文'"
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
          :disabled="isSubmitting"
        >
          <ArrowPathIcon v-if="isSubmitting" class="w-4 h-4 animate-spin" />
          <ArrowRightOnRectangleIcon v-else class="w-4 h-4" />
          <span>{{ isSubmitting ? '认证验证中...' : '验证令牌并进入' }}</span>
        </button>
      </div>
    </form>

    <div class="divider text-xs text-base-content/40 my-2">逃生通道</div>

    <!-- Escape Hatch -->
    <div class="bg-base-100/70 p-3.5 rounded-xl border border-base-300 space-y-2">
      <div class="flex items-center justify-between gap-2">
        <div class="text-[11px] text-base-content/70">
          若遇到凭据污染死锁，可一键清空本地残留并重新探测。
        </div>
        <button
          type="button"
          class="btn btn-outline btn-ghost btn-xs shrink-0 text-error hover:bg-error/10 hover:border-error"
          data-testid="auth-gate-reset"
          @click="handleReset"
        >
          清空本地凭据
        </button>
      </div>
    </div>
  </div>
</template>

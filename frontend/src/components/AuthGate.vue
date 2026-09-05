<template>
  <section class="auth-gate min-h-[100dvh] flex flex-col justify-start sm:justify-center items-center bg-canvas p-4 sm:p-6 overflow-y-auto pt-12 sm:pt-0">
    <div class="auth-card w-full max-w-md bg-surface border border-border-subtle rounded-lg p-6 sm:p-8 space-y-6 shadow-sm">
      <div class="space-y-1.5 text-center">
        <p class="text-xs font-mono font-semibold text-accent uppercase tracking-wider">Access Token // 身份鉴权</p>
        <h1 class="text-xl font-bold text-text-main tracking-tight">进入管理工作台</h1>
        <p class="text-xs text-text-muted">请输入系统访问 Token 进行身份验证以访问控制台。</p>
      </div>

      <form class="auth-form space-y-4" @submit.prevent="submit">
        <div class="space-y-1.5">
          <label for="auth-token-input" class="block text-xs font-medium text-text-muted">
            访问 Token
          </label>
          <div class="relative flex items-center rounded-md border border-border bg-surface-base focus-within:border-accent focus-within:ring-1 focus-within:ring-accent transition-colors">
            <input
              id="auth-token-input"
              v-model="token"
              :type="showToken ? 'text' : 'password'"
              autocomplete="current-password"
              autofocus
              placeholder="输入访问 Token"
              class="flex-1 min-h-[44px] bg-transparent px-3.5 py-2 text-xs font-mono text-text-main placeholder:text-text-muted focus:outline-hidden min-w-0"
            />
            <div class="flex items-center gap-0.5 pr-1.5 shrink-0">
              <!-- Clear Input Button (Visible when input populated) -->
              <button
                v-if="token"
                type="button"
                class="flex h-8 w-8 items-center justify-center rounded text-text-muted hover:text-text-main hover:bg-surface-hover transition-colors cursor-pointer"
                aria-label="清空 Token"
                title="清空"
                @click="token = ''"
              >
                <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>

              <!-- Show / Hide Password Toggle -->
              <button
                type="button"
                class="flex h-8 w-8 items-center justify-center rounded text-text-muted hover:text-text-main hover:bg-surface-hover transition-colors cursor-pointer"
                :aria-label="showToken ? '隐藏 Token' : '显示 Token'"
                :title="showToken ? '隐藏 Token' : '显示 Token'"
                @click="showToken = !showToken"
              >
                <svg v-if="showToken" class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l18 18" />
                </svg>
                <svg v-else class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              </button>

              <!-- Clipboard Paste Action -->
              <button
                type="button"
                class="flex h-8 w-8 items-center justify-center rounded text-text-muted hover:text-text-main hover:bg-surface-hover transition-colors cursor-pointer"
                aria-label="从剪贴板粘贴"
                title="从剪贴板粘贴"
                @click="handlePaste"
              >
                <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                </svg>
              </button>
            </div>
          </div>
        </div>

        <button
          class="w-full min-h-[44px] flex items-center justify-center gap-2 rounded-md bg-accent px-4 py-2.5 text-xs font-semibold text-white hover:bg-accent-hover transition-colors disabled:opacity-50 cursor-pointer focus-ring"
          type="submit"
          :disabled="loading || !token.trim()"
        >
          <svg v-if="loading" class="animate-spin h-3.5 w-3.5 text-white" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
          </svg>
          <span>{{ loading ? '验证中...' : '进入工作台' }}</span>
        </button>
      </form>

      <div
        v-if="error"
        class="alert error p-3 rounded-md border border-status-danger/30 bg-status-danger/10 text-xs text-status-danger font-mono"
        role="alert"
        aria-live="assertive"
      >
        {{ error }}
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  authenticate: (token: string) => Promise<void>
}>()

const token = ref('')
const showToken = ref(false)
const loading = ref(false)
const error = ref('')

async function handlePaste() {
  error.value = ''
  try {
    if (navigator?.clipboard?.readText) {
      const text = await navigator.clipboard.readText()
      if (text) {
        token.value = text.trim()
      }
    } else {
      error.value = '浏览器不支持或未授权剪贴板读取，请手动粘贴'
    }
  } catch {
    error.value = '无法读取剪贴板内容，请手动输入或粘贴'
  }
}

async function submit() {
  const trimmed = token.value.trim()
  if (!trimmed) return
  loading.value = true
  error.value = ''
  try {
    await props.authenticate(trimmed)
  } catch (err: any) {
    error.value = err?.message || 'Token 验证失败，请检查后重试'
  } finally {
    loading.value = false
  }
}
</script>

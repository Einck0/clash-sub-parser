<template>
  <section class="auth-gate">
    <div class="auth-card">
      <p class="eyebrow">Access Token</p>
      <h1>进入管理界面</h1>
      <p class="page-desc">请输入访问 token。验证成功后后端会设置 HttpOnly cookie；导出/订阅链接仅在本次页面会话中使用 URL token。</p>

      <form class="auth-form" @submit.prevent="submit">
        <label class="field">
          <span>访问 token</span>
          <input v-model="token" type="password" autocomplete="current-password" autofocus placeholder="输入访问 token" />
        </label>
        <button class="primary" type="submit" :disabled="loading || !token.trim()">
          {{ loading ? '验证中...' : '进入' }}
        </button>
      </form>

      <div v-if="error" class="alert error" role="alert" aria-live="assertive">{{ error }}</div>
    </div>
  </section>
</template>

<script setup>
import { ref } from 'vue'

const props = defineProps({
  authenticate: {
    type: Function,
    required: true,
  },
})

const token = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await props.authenticate(token.value.trim())
  } catch (err) {
    error.value = err?.message || 'Token 验证失败'
  } finally {
    loading.value = false
  }
}
</script>

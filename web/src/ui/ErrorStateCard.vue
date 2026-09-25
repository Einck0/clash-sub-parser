<script setup lang="ts">
import { computed } from 'vue'
import {
  ExclamationCircleIcon,
  ShieldExclamationIcon,
  NoSymbolIcon,
  SignalSlashIcon,
  ArrowPathIcon,
  KeyIcon,
} from '@heroicons/vue/24/outline'

export interface ErrorStateCardProps {
  error: unknown
  title?: string
  description?: string
  showRetry?: boolean
  showAuthRecovery?: boolean
  retrying?: boolean
}

export interface ErrorStateCardEmits {
  (e: 'retry'): void
  (e: 'auth-recovery'): void
}

const props = withDefaults(defineProps<ErrorStateCardProps>(), {
  title: '',
  description: '',
  showRetry: true,
  showAuthRecovery: true,
  retrying: false,
})

const emit = defineEmits<ErrorStateCardEmits>()

interface ParsedErrorInfo {
  status?: number
  code?: string
  message: string
  friendlyTitle: string
  friendlyDesc: string
  isAuth: boolean
  isForbidden: boolean
  isConflict: boolean
  isNetwork: boolean
  isServerError: boolean
}

const parsedError = computed<ParsedErrorInfo>(() => {
  const err = props.error
  let status: number | undefined
  let code: string | undefined
  let message = ''

  if (err && typeof err === 'object') {
    if ('status' in err && typeof (err as any).status === 'number') {
      status = (err as any).status
    }
    if ('code' in err && typeof (err as any).code === 'string') {
      code = (err as any).code
    }
    if ('message' in err && typeof (err as any).message === 'string') {
      message = (err as any).message
    }
  } else if (typeof err === 'string') {
    message = err
    const statusMatch = err.match(/\b(401|403|404|409|500|502|503)\b/)
    if (statusMatch) {
      status = parseInt(statusMatch[1], 10)
    }
  }

  const lowerMsg = message.toLowerCase()
  const isAuth = status === 401 || lowerMsg.includes('unauthorized') || lowerMsg.includes('未授权') || lowerMsg.includes('token') || lowerMsg.includes('令牌')
  const isForbidden = status === 403 || lowerMsg.includes('forbidden') || lowerMsg.includes('拒绝访问') || lowerMsg.includes('无权限')
  const isConflict = status === 409 || lowerMsg.includes('conflict') || lowerMsg.includes('冲突')
  const isNetwork = lowerMsg.includes('network') || lowerMsg.includes('failed to fetch') || lowerMsg.includes('networkerror') || lowerMsg.includes('无法连接')
  const isServerError = (status !== undefined && status >= 500) || lowerMsg.includes('internal server error') || lowerMsg.includes('500')

  let friendlyTitle = props.title
  let friendlyDesc = props.description

  if (!friendlyTitle) {
    if (isAuth) {
      friendlyTitle = 'Authentication Required / 鉴权已失效或需要登录'
    } else if (isForbidden) {
      friendlyTitle = 'Access Denied / 访问受限'
    } else if (isConflict) {
      friendlyTitle = 'Resource Conflict / 状态冲突'
    } else if (isNetwork) {
      friendlyTitle = 'Network Connection Failed / 网络连接异常'
    } else if (isServerError) {
      friendlyTitle = 'Service Temporarily Unavailable / 服务暂时不可用'
    } else {
      friendlyTitle = 'Operation Failed / 操作失败'
    }
  }

  if (!friendlyDesc) {
    if (isAuth) {
      friendlyDesc = 'Token is missing, invalid or expired. Please verify your credentials in Settings or re-authenticate.'
    } else if (isForbidden) {
      friendlyDesc = 'You do not have permission to perform this action. Check administrative access permissions.'
    } else if (isConflict) {
      friendlyDesc = 'The resource has been modified or conflicts with another concurrent operation. Please refresh and retry.'
    } else if (isNetwork) {
      friendlyDesc = 'Cannot establish a connection to the CSP backend. Please check network connectivity and backend service status.'
    } else if (isServerError) {
      friendlyDesc = 'The server encountered an error processing your request. Please try again or check backend logs.'
    } else {
      friendlyDesc = message || 'An unexpected error occurred while processing the request.'
    }
  }

  return {
    status,
    code,
    message,
    friendlyTitle,
    friendlyDesc,
    isAuth,
    isForbidden,
    isConflict,
    isNetwork,
    isServerError,
  }
})

function handleRetry() {
  emit('retry')
}

function handleAuthRecovery() {
  emit('auth-recovery')
  if (typeof window !== 'undefined') {
    window.location.hash = '#settings'
  }
}
</script>

<template>
  <div
    role="alert"
    class="rounded-xl border border-error/30 bg-error/10 p-4 sm:p-5 text-base-content shadow-sm transition-all duration-200"
    data-testid="error-state-card"
  >
    <div class="flex items-start gap-3.5">
      <!-- Icon representation by error classification -->
      <div class="p-2 rounded-xl bg-error/20 text-error shrink-0 mt-0.5">
        <KeyIcon v-if="parsedError.isAuth" class="w-5 h-5" />
        <ShieldExclamationIcon v-else-if="parsedError.isForbidden" class="w-5 h-5" />
        <NoSymbolIcon v-else-if="parsedError.isConflict" class="w-5 h-5" />
        <SignalSlashIcon v-else-if="parsedError.isNetwork" class="w-5 h-5" />
        <ExclamationCircleIcon v-else class="w-5 h-5" />
      </div>

      <div class="min-w-0 flex-1 space-y-1">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="font-bold text-sm sm:text-base text-error">
            {{ parsedError.friendlyTitle }}
          </h3>
          <span v-if="parsedError.status" class="badge badge-error badge-sm font-mono text-[11px]">
            HTTP {{ parsedError.status }}
          </span>
          <span v-if="parsedError.code" class="badge badge-outline badge-error badge-sm font-mono text-[11px]">
            {{ parsedError.code }}
          </span>
        </div>

        <p class="text-xs sm:text-sm text-base-content/80 leading-relaxed">
          {{ parsedError.friendlyDesc }}
        </p>

        <p
          v-if="parsedError.message && parsedError.message !== parsedError.friendlyDesc"
          class="font-mono text-[11px] bg-base-100/60 p-2 rounded-lg border border-base-300/40 text-base-content/70 break-all select-text"
        >
          {{ parsedError.message }}
        </p>

        <!-- Recovery / Retry Action Buttons -->
        <div class="pt-2 flex flex-wrap items-center gap-2">
          <button
            v-if="showRetry"
            type="button"
            class="btn btn-sm btn-error btn-outline gap-1.5 touch-manipulation"
            :disabled="retrying"
            :class="{ loading: retrying }"
            @click="handleRetry"
          >
            <ArrowPathIcon class="w-4 h-4" :class="{ 'animate-spin': retrying }" />
            <span>Retry</span>
          </button>

          <button
            v-if="showAuthRecovery && parsedError.isAuth"
            type="button"
            class="btn btn-sm btn-outline gap-1.5 touch-manipulation hover:bg-base-200"
            @click="handleAuthRecovery"
          >
            <KeyIcon class="w-4 h-4" />
            <span>Go to Settings / Auth</span>
          </button>

          <slot name="actions" />
        </div>
      </div>
    </div>
  </div>
</template>

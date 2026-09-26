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
import { t } from '../locales'

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

function translateErrorCode(code?: string): string {
  switch (code) {
    case 'unauthorized':
      return t('errors.unauthorized')
    case 'forbidden':
      return t('errors.forbidden')
    case 'conflict':
      return t('errors.conflict')
    case 'validation_failed':
      return t('errors.validation_failed')
    case 'unsupported_target':
      return t('errors.unsupported_target')
    case 'unsupported_target_capability':
      return t('errors.unsupported_target_capability')
    case 'no_active_revision':
      return t('errors.no_active_revision')
    case 'invalid_json':
      return t('errors.invalid_json')
    case 'cycle_detected':
      return t('errors.cycle_detected')
    case 'self_loop_forbidden':
      return t('errors.self_loop_forbidden')
    case 'publication_preflight_rejected':
      return t('errors.publication_preflight_rejected')
    case 'publication_not_found':
      return t('errors.publication_not_found')
    case 'publication_revoked':
      return t('errors.publication_revoked')
    case 'not_found':
      return t('errors.not_found')
    case 'internal_error':
      return t('errors.internal_error')
    default:
      return ''
  }
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
  const isAuth = status === 401 || code === 'unauthorized' || lowerMsg.includes('unauthorized') || lowerMsg.includes('未授权') || lowerMsg.includes('token') || lowerMsg.includes('令牌')
  const isForbidden = status === 403 || code === 'forbidden' || lowerMsg.includes('forbidden') || lowerMsg.includes('拒绝访问') || lowerMsg.includes('无权限')
  const isConflict = status === 409 || code === 'conflict' || lowerMsg.includes('conflict') || lowerMsg.includes('冲突')
  const isNetwork = lowerMsg.includes('network') || lowerMsg.includes('failed to fetch') || lowerMsg.includes('networkerror') || lowerMsg.includes('无法连接')
  const isServerError = (status !== undefined && status >= 500) || lowerMsg.includes('internal server error') || lowerMsg.includes('500')

  const codeTitle = translateErrorCode(code)
  let friendlyTitle = props.title
  let friendlyDesc = props.description

  if (!friendlyTitle) {
    if (isAuth) {
      friendlyTitle = t('errors.unauthorized')
    } else if (isForbidden) {
      friendlyTitle = t('errors.forbidden')
    } else if (isConflict) {
      friendlyTitle = codeTitle && code !== 'conflict' ? `${t('errors.conflict')}：${codeTitle}` : t('errors.conflict')
    } else if (isNetwork) {
      friendlyTitle = t('errors.networkError')
    } else if (isServerError) {
      friendlyTitle = t('errors.serverError')
    } else if (codeTitle) {
      friendlyTitle = codeTitle
    } else {
      friendlyTitle = t('errors.operationFailed')
    }
  }

  if (!friendlyDesc) {
    if (isAuth) {
      friendlyDesc = t('errors.unauthorizedDesc')
    } else if (isForbidden) {
      friendlyDesc = t('errors.forbiddenDesc')
    } else if (isConflict) {
      friendlyDesc = t('errors.conflictDesc')
    } else if (isNetwork) {
      friendlyDesc = t('errors.networkErrorDesc')
    } else if (isServerError) {
      friendlyDesc = t('errors.serverErrorDesc')
    } else {
      friendlyDesc = message || codeTitle || t('errors.operationFailedDesc')
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
            <span>{{ t('errors.retryBtn') }}</span>
          </button>

          <button
            v-if="showAuthRecovery && parsedError.isAuth"
            type="button"
            class="btn btn-sm btn-outline gap-1.5 touch-manipulation hover:bg-base-200"
            @click="handleAuthRecovery"
          >
            <KeyIcon class="w-4 h-4" />
            <span>{{ t('errors.goToAuth') }}</span>
          </button>

          <slot name="actions" />
        </div>
      </div>
    </div>
  </div>
</template>

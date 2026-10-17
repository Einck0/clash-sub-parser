<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  ClipboardDocumentIcon,
  ClipboardDocumentCheckIcon,
  ArrowPathIcon,
  ExclamationTriangleIcon,
} from '@heroicons/vue/24/outline'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import ModalDialog from '../../ui/ModalDialog.vue'
import { toastStore } from '../../ui/toast'
import { t } from '../../locales'
import {
  COMPILER_TARGETS,
  targetLabel,
  type CompilerTarget,
  type SubscriptionLink,
} from './publicationTypes'
import { usePublications } from './usePublications'

const {
  subscriptionLinks,
  loadingLinks,
  copyingLink,
  copyingTarget,
  resettingTarget,
  copyingTargets,
  resettingTargets,
  targetLinkErrors,
  targetGenerationWarnings,
  manualCopyUrl,
  manualCopyModalOpen,
  linkError,
  generationWarning,
  fetchSubscriptionLinks,
  copyDefaultClientSubscription,
  resetDefaultSubscriptionLink,
} = usePublications()

const resetTarget = ref<CompilerTarget | null>(null)
const resetConfirmModalOpen = ref(false)
const copiedTarget = ref<CompilerTarget | null>(null)

const resetModalTitle = computed(() => {
  if (!resetTarget.value) return t('publications.resetConfirmTitle')
  const label = targetLabel(resetTarget.value)
  return t('publications.resetConfirmTitleTarget', { target: label })
})

const resetModalMessage = computed(() => {
  if (!resetTarget.value) return t('publications.resetConfirmMessage')
  const label = targetLabel(resetTarget.value)
  return t('publications.resetConfirmMessageTarget', { target: label })
})

function defaultLink(target: CompilerTarget): SubscriptionLink | undefined {
  return subscriptionLinks.value.find(
    (link) => link.target === target && link.is_default && link.state === 'active'
  )
}

function openResetModal(target: CompilerTarget) {
  if (copyingTargets.value[target] || resettingTargets.value[target]) return
  resetTarget.value = target
  resetConfirmModalOpen.value = true
}

function cancelReset() {
  resetConfirmModalOpen.value = false
  resetTarget.value = null
}

async function copyLink(target: CompilerTarget) {
  if (copyingTargets.value[target] || resettingTargets.value[target]) return
  if (resetConfirmModalOpen.value && resetTarget.value === target) return
  targetLinkErrors.value[target] = ''
  targetGenerationWarnings.value[target] = ''
  try {
    const result = await copyDefaultClientSubscription(target)
    if (result.url && result.copied) {
      copiedTarget.value = target
      window.setTimeout(() => {
        if (copiedTarget.value === target) copiedTarget.value = null
      }, 2000)
      toastStore.push({
        message: result.warning ? `链接已复制：${result.warning}` : '订阅链接已复制',
        tone: result.warning ? 'warning' : 'success',
      })
    }
  } catch {
    // linkError is populated by the composable
  }
}

async function confirmReset() {
  if (!resetTarget.value) return
  const target = resetTarget.value
  resetConfirmModalOpen.value = false
  try {
    await resetDefaultSubscriptionLink(target)
    await fetchSubscriptionLinks()
    toastStore.push({ message: t('publications.resetSuccess'), tone: 'success' })
  } catch {
    // linkError is populated by the composable
  } finally {
    resetTarget.value = null
  }
}

onMounted(() => {
  // GET-only: loads existing links without creating new links
  void fetchSubscriptionLinks()
})
</script>

<template>
  <section class="mx-auto w-full max-w-5xl space-y-5" aria-labelledby="publications-title" data-testid="client-subscriptions-page">
    <header class="space-y-1">
      <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary">{{ t('publications.tag') }}</p>
      <h2 id="publications-title" class="text-2xl font-bold">{{ t('publications.title') }}</h2>
      <p class="text-sm opacity-70">{{ t('publications.subtitle') }}</p>
    </header>

    <div class="divide-y divide-base-300 rounded-2xl border border-base-300 bg-base-100 shadow-sm" data-testid="client-subscription-list">
      <article
        v-for="item in COMPILER_TARGETS"
        :key="item.target"
        class="flex flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5"
        :data-target="item.target"
        :data-testid="`client-subscription-row-${item.target}`"
      >
        <div class="min-w-0 space-y-1">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="font-semibold text-base">{{ item.label }}</h3>
            <span class="badge badge-sm badge-ghost font-mono">.{{ item.ext }}</span>
            <span
              class="badge badge-sm font-medium"
              :class="item.target === 'mihomo' ? 'badge-primary badge-outline' : 'badge-ghost'"
            >
              {{ item.target === 'mihomo' ? t('publications.fullConfig') : t('publications.nodesOnly') }}
            </span>
          </div>
          <p class="text-sm opacity-70">{{ item.desc }}</p>
          <p v-if="item.target !== 'mihomo'" class="text-xs opacity-60">{{ item.ruleSummary }}</p>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <span v-if="!defaultLink(item.target)" class="hidden text-xs opacity-50 sm:inline">复制时自动创建</span>
          <button
            type="button"
            class="btn btn-primary btn-sm min-w-32 gap-1.5 touch-manipulation"
            data-testid="copy-client-subscription-btn"
            :data-target="item.target"
            :data-testid-target="`copy-client-subscription-${item.target}`"
            :disabled="copyingTargets[item.target] || resettingTargets[item.target]"
            :class="{ loading: copyingTargets[item.target] }"
            @click="copyLink(item.target)"
          >
            <ClipboardDocumentCheckIcon v-if="copiedTarget === item.target" class="h-4 w-4" />
            <ClipboardDocumentIcon v-else class="h-4 w-4" />
            <span>{{ copiedTarget === item.target ? '已复制！' : '复制订阅链接' }}</span>
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-sm gap-1 text-xs text-warning hover:bg-warning/10"
            data-testid="reset-client-subscription-btn"
            :data-target="item.target"
            :data-testid-target="`reset-client-subscription-${item.target}`"
            :disabled="copyingTargets[item.target] || resettingTargets[item.target]"
            :class="{ loading: resettingTargets[item.target] }"
            @click="openResetModal(item.target)"
          >
            <ArrowPathIcon class="h-3.5 w-3.5" />
            <span>{{ t('publications.resetBtn') }}</span>
          </button>
        </div>
      </article>
    </div>

    <!-- Generation Warning Banner -->
    <div
      v-if="generationWarning"
      class="rounded-xl border border-warning/40 bg-warning/10 p-3 sm:p-4 text-xs space-y-1 text-warning"
      role="alert"
      data-testid="generation-warning-banner"
    >
      <div class="font-bold flex items-center gap-1.5 text-sm">
        <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
        <span>链接已创建，当前不可更新</span>
      </div>
      <p class="opacity-90 leading-relaxed">
        {{ generationWarning }}。请修复对应配置后再拉取更新。
      </p>
    </div>

    <div v-if="linkError" class="alert alert-error text-sm" role="alert" data-testid="subscription-link-error">
      {{ linkError }}
    </div>

    <div v-if="loadingLinks" class="text-xs opacity-60" role="status">
      {{ t('common.loading') }}
    </div>

    <!-- Manual Copy Fallback Modal -->
    <ModalDialog
      v-model="manualCopyModalOpen"
      title="手动复制客户端订阅直链"
      description="浏览器剪贴板权限受限或非安全上下文，请在下方全选复制订阅链接："
    >
      <div class="space-y-3">
        <textarea
          readonly
          rows="3"
          :value="manualCopyUrl"
          data-testid="manual-copy-textarea"
          class="textarea textarea-bordered w-full font-mono text-xs select-all bg-base-200"
          @focus="($event.target as HTMLTextAreaElement).select()"
        />
        <p class="text-[11px] opacity-70">
          请复制上方完整地址后粘贴至客户端（Clash / Sing-box / Surge / Quantumult X）中。
        </p>
      </div>
      <template #footer>
        <button
          type="button"
          class="btn btn-primary btn-sm"
          @click="manualCopyModalOpen = false"
        >
          完成
        </button>
      </template>
    </ModalDialog>

    <!-- Reset Link Confirm Modal -->
    <ConfirmModal
      v-model="resetConfirmModalOpen"
      :title="resetModalTitle"
      :message="resetModalMessage"
      confirm-text="确认重置"
      cancel-text="取消"
      tone="warning"
      :loading="resetTarget ? resettingTargets[resetTarget] : false"
      @confirm="confirmReset"
      @cancel="cancelReset"
    />
  </section>
</template>

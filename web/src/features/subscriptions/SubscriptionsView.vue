<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  AdjustmentsHorizontalIcon,
  ArrowPathIcon,
  ClipboardDocumentIcon,
  DocumentDuplicateIcon,
  EllipsisVerticalIcon,
  PlusIcon,
  TrashIcon,
} from '@heroicons/vue/24/outline'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import Popover from '../../ui/Popover.vue'
import { toastStore } from '../../ui/toast'
import SubscriptionConfigDrawer from './SubscriptionConfigDrawer.vue'
import { t } from '../../locales'
import {
  useSubscriptions,
  type SubscriptionDraft,
  type SubscriptionRecord,
} from './useSubscriptions'

const { items, loading, saving, error, total, load, save, remove, refresh, refreshingIDs } = useSubscriptions()
const drawerOpen = ref(false)
const editing = ref<SubscriptionRecord>()
const confirmDeleteOpen = ref(false)
const pendingDeleteSubscription = ref<SubscriptionRecord | null>(null)
const deleting = ref(false)

function openCreate() {
  editing.value = undefined
  drawerOpen.value = true
}

function openEdit(subscription: SubscriptionRecord) {
  editing.value = subscription
  drawerOpen.value = true
}

async function handleDrawerSave(draft: SubscriptionDraft) {
  await save(draft, editing.value)
  drawerOpen.value = false
}

function confirmRemove(subscription: SubscriptionRecord) {
  pendingDeleteSubscription.value = subscription
  confirmDeleteOpen.value = true
}

async function handleConfirmDelete() {
  if (!pendingDeleteSubscription.value) return
  deleting.value = true
  try {
    await remove(pendingDeleteSubscription.value)
    confirmDeleteOpen.value = false
    pendingDeleteSubscription.value = null
  } finally {
    deleting.value = false
  }
}

async function copyReference(subscription: SubscriptionRecord) {
  const refText = subscription.source_url_secret_ref || subscription.name
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(refText)
      toastStore.push({ message: t('common.copySuccess'), tone: 'success' })
    } else {
      throw new Error('Clipboard API unavailable')
    }
  } catch {
    toastStore.push({ message: t('common.copyFailed'), tone: 'error' })
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-5" aria-labelledby="subscriptions-title">
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 w-full min-w-0">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('subscriptions.tag') }}</p>
        <h2 id="subscriptions-title" class="mt-1 text-2xl font-bold truncate">{{ t('subscriptions.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">{{ t('subscriptions.subtitle') }}</p>
      </div>
      <button class="btn btn-primary btn-sm gap-2 touch-manipulation shadow-sm shrink-0 self-start sm:self-auto" type="button" @click="openCreate">
        <PlusIcon class="h-4 w-4" />{{ t('subscriptions.add') }}
      </button>
    </div>

    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loading"
      @retry="load"
    />
    <div v-if="loading" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      <div v-for="index in 3" :key="index" class="skeleton h-44 rounded-box" />
    </div>
    <EmptyState
      v-else-if="items.length === 0"
      :icon="DocumentDuplicateIcon"
      :title="t('subscriptions.emptyTitle')"
      :description="t('subscriptions.emptyDesc')"
      :action-label="t('subscriptions.add')"
      @action="openCreate"
    >
      <template #action-icon>
        <PlusIcon class="h-4 w-4" />
      </template>
    </EmptyState>
    <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3 w-full min-w-0">
      <article
        v-for="subscription in items"
        :key="subscription.id"
        data-testid="subscription-card"
        class="card border border-base-300 bg-base-200 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md min-w-0 w-full overflow-hidden"
      >
        <div class="card-body gap-3 sm:gap-4 p-4 sm:p-5 min-w-0 max-w-full">
          <div class="flex items-start justify-between gap-2 min-w-0">
            <div class="min-w-0 flex-1">
              <h3 class="truncate font-semibold text-sm sm:text-base text-base-content">{{ subscription.name }}</h3>
              <p class="mt-1 truncate font-mono text-xs opacity-60 break-all">{{ subscription.source_url_secret_ref }}</p>
            </div>
            <div class="flex items-center gap-1.5 shrink-0">
              <StatusBadge
                class="shrink-0"
                :label="subscription.enabled ? t('common.enabled') : t('common.disabled')"
                :tone="subscription.enabled ? 'success' : 'warning'"
              />
              <Popover placement="bottom-end" panel-class="w-44 max-w-[calc(100vw-2rem)]">
                <template #trigger="{ open }">
                  <button
                    type="button"
                    class="btn btn-ghost btn-xs btn-circle text-base-content/70 hover:text-base-content touch-manipulation"
                    :class="{ 'btn-active': open }"
                    :title="t('common.actions')"
                    aria-label="Subscription actions"
                    data-testid="subscription-actions-trigger"
                  >
                    <EllipsisVerticalIcon class="h-4 w-4" />
                  </button>
                </template>
                <template #default="{ close }">
                  <div class="flex flex-col gap-0.5 text-xs font-medium" role="menu">
                    <!-- Refresh -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-base-300/80 transition-colors"
                      :disabled="refreshingIDs.has(subscription.id)"
                      @click="close(); refresh(subscription)"
                      data-testid="sub-action-refresh"
                    >
                      <ArrowPathIcon class="h-3.5 w-3.5 shrink-0" :class="{ 'animate-spin': refreshingIDs.has(subscription.id) }" />
                      <span>{{ t('common.refresh') }}</span>
                    </button>

                    <!-- Configure / Open Drawer -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-base-300/80 text-primary transition-colors"
                      @click="close(); openEdit(subscription)"
                      data-testid="sub-action-configure"
                    >
                      <AdjustmentsHorizontalIcon class="h-3.5 w-3.5 shrink-0" />
                      <span>{{ t('subscriptions.configure') }}</span>
                    </button>

                    <!-- Copy Reference -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-base-300/80 transition-colors"
                      @click="close(); copyReference(subscription)"
                      data-testid="sub-action-copy-ref"
                    >
                      <ClipboardDocumentIcon class="h-3.5 w-3.5 shrink-0" />
                      <span>{{ t('subscriptions.copyRef') }}</span>
                    </button>

                    <div class="my-1 border-t border-white/10" />

                    <!-- Dangerous Delete -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-error/20 text-error transition-colors"
                      @click="close(); confirmRemove(subscription)"
                      data-testid="sub-action-delete"
                    >
                      <TrashIcon class="h-3.5 w-3.5 shrink-0" />
                      <span>{{ t('common.delete') }}</span>
                    </button>
                  </div>
                </template>
              </Popover>
            </div>
          </div>

          <!-- Badges for Advanced Config -->
          <div v-if="subscription.config?.cron_schedule || subscription.config?.auto_test || subscription.config?.rename_rules?.length || subscription.config?.filter_rules?.length || subscription.config?.target_groups?.length" class="flex flex-wrap gap-1.5 pt-1">
            <span v-if="subscription.config?.cron_schedule" class="badge badge-neutral badge-xs font-mono text-[10px]">
              {{ subscription.config.cron_schedule }}
            </span>
            <span v-if="subscription.config?.auto_test" class="badge badge-primary badge-outline badge-xs text-[10px]">
              Auto Test
            </span>
            <span v-if="subscription.config?.rename_rules?.length" class="badge badge-info badge-outline badge-xs text-[10px]">
              {{ subscription.config.rename_rules.length }} Renames
            </span>
            <span v-if="subscription.config?.filter_rules?.length" class="badge badge-accent badge-outline badge-xs text-[10px]">
              {{ subscription.config.filter_rules.length }} Filters
            </span>
            <span v-if="subscription.config?.target_groups?.length" class="badge badge-secondary badge-outline badge-xs text-[10px]">
              {{ subscription.config.target_groups.length }} Groups
            </span>
          </div>

          <dl class="grid grid-cols-2 gap-3 text-xs">
            <div>
              <dt class="opacity-60">{{ t('subscriptions.refreshInterval') }}</dt>
              <dd class="mt-1 font-medium">{{ subscription.refresh_policy.interval_seconds }}s</dd>
            </div>
            <div>
              <dt class="opacity-60">{{ t('subscriptions.timeout') }}</dt>
              <dd class="mt-1 font-medium">{{ subscription.refresh_policy.timeout_seconds }}s</dd>
            </div>
          </dl>
        </div>
      </article>
    </div>
    <p v-if="items.length" class="text-right text-xs opacity-60">
      {{ t('subscriptions.totalOf', { count: items.length, total }) }}
    </p>

    <!-- Advanced Configuration Drawer -->
    <SubscriptionConfigDrawer
      v-model="drawerOpen"
      :subscription="editing"
      :saving="saving"
      @save="handleDrawerSave"
    />

    <ConfirmModal
      v-model="confirmDeleteOpen"
      :title="t('subscriptions.deleteConfirmTitle')"
      :message="pendingDeleteSubscription ? t('subscriptions.deleteConfirmDesc', { name: pendingDeleteSubscription.name }) : ''"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      tone="danger"
      :loading="deleting"
      @confirm="handleConfirmDelete"
    />
  </section>
</template>

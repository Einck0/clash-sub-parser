<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  AdjustmentsHorizontalIcon,
  ArrowPathIcon,
  ClipboardDocumentIcon,
  DocumentDuplicateIcon,
  EllipsisVerticalIcon,
  ListBulletIcon,
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
import SubscriptionEntriesDrawer from './SubscriptionEntriesDrawer.vue'
import { t, currentLocale } from '../../locales'
import {
  useSubscriptions,
  formatRefreshTime,
  formatFullDateTime,
  type SubscriptionDraft,
  type SubscriptionRecord,
} from './useSubscriptions'

const { items, loading, saving, error, total, load, save, remove, refresh, refreshingIDs, toggle, togglingIDs } = useSubscriptions()
const drawerOpen = ref(false)
const editing = ref<SubscriptionRecord>()
const confirmDeleteOpen = ref(false)
const pendingDeleteSubscription = ref<SubscriptionRecord | null>(null)
const deleting = ref(false)
const entriesDrawerOpen = ref(false)
const viewingEntriesSub = ref<SubscriptionRecord>()

async function handleToggle(subscription: SubscriptionRecord) {
  try {
    await toggle(subscription)
  } catch {
    // handled with toast
  }
}

function openEntries(subscription: SubscriptionRecord) {
  viewingEntriesSub.value = subscription
  entriesDrawerOpen.value = true
}

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
      throw new Error('剪贴板不可用')
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
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">订阅源管理</p>
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
              <button
                type="button"
                class="btn btn-ghost btn-xs border border-base-300/80 px-2 gap-1 touch-manipulation hover:bg-base-300"
                data-testid="sub-view-entries"
                :title="t('subscriptions.viewEntries')"
                @click="openEntries(subscription)"
              >
                <ListBulletIcon class="h-3.5 w-3.5" />
                <span>{{ t('subscriptions.viewEntries') }}</span>
              </button>
              <button
                type="button"
                data-testid="subscription-toggle-btn"
                class="btn btn-xs gap-1 touch-manipulation transition-all"
                :class="subscription.enabled ? 'btn-success btn-outline' : 'btn-ghost text-base-content/60 border border-base-300'"
                :disabled="togglingIDs.has(subscription.id)"
                :title="subscription.enabled ? '点击停用订阅源' : '点击启用订阅源'"
                @click="handleToggle(subscription)"
              >
                <span
                  class="w-2 h-2 rounded-full"
                  :class="subscription.enabled ? 'bg-success animate-pulse' : 'bg-base-content/40'"
                />
                <span>{{ subscription.enabled ? t('common.enabled') : t('common.disabled') }}</span>
              </button>
              <Popover placement="bottom-end" panel-class="w-44 max-w-[calc(100vw-2rem)]">
                <template #trigger="{ open }">
                  <button
                    type="button"
                    class="btn btn-ghost btn-xs btn-circle text-base-content/70 hover:text-base-content touch-manipulation"
                    :class="{ 'btn-active': open }"
                    :title="t('common.actions')"
                    aria-label="订阅源操作菜单"
                    data-testid="subscription-actions-trigger"
                  >
                    <EllipsisVerticalIcon class="h-4 w-4" />
                  </button>
                </template>
                <template #default="{ close }">
                  <div class="flex flex-col gap-0.5 text-xs font-medium">
                    <!-- Refresh -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-base-300/80 transition-colors"
                      :disabled="refreshingIDs.has(subscription.id)"
                      @click="close(); refresh(subscription)"
                      role="menuitem"
                      data-testid="sub-action-refresh"
                    >
                      <ArrowPathIcon class="h-3.5 w-3.5 shrink-0" :class="{ 'animate-spin': refreshingIDs.has(subscription.id) }" />
                      <span>{{ t('common.refresh') }}</span>
                    </button>

                    <!-- Toggle Enabled -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-base-300/80 transition-colors"
                      :disabled="togglingIDs.has(subscription.id)"
                      @click="close(); handleToggle(subscription)"
                      role="menuitem"
                      data-testid="sub-action-toggle"
                    >
                      <ArrowPathIcon v-if="togglingIDs.has(subscription.id)" class="h-3.5 w-3.5 shrink-0 animate-spin" />
                      <span v-else class="w-3.5 h-3.5 inline-flex items-center justify-center font-bold text-xs">
                        {{ subscription.enabled ? '⏸' : '▶' }}
                      </span>
                      <span>{{ subscription.enabled ? t('common.disable') : t('common.enable') }}</span>
                    </button>

                    <!-- View Source Entries -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-base-300/80 text-info transition-colors"
                      @click="close(); openEntries(subscription)"
                      role="menuitem"
                      data-testid="sub-action-view-entries"
                    >
                      <ListBulletIcon class="h-3.5 w-3.5 shrink-0" />
                      <span>{{ t('subscriptions.viewEntries') }}</span>
                    </button>

                    <!-- Configure / Open Drawer -->
                    <button
                      type="button"
                      class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-left hover:bg-base-300/80 text-primary transition-colors"
                      @click="close(); openEdit(subscription)"
                      role="menuitem"
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
                      role="menuitem"
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
                      role="menuitem"
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

          <!-- Node Count Badge / Retained Source Member Info -->
          <div class="flex items-center justify-between gap-2 p-2 rounded-lg bg-base-300/40 text-xs" data-testid="subscription-node-count-bar">
            <div class="flex items-center gap-1.5 min-w-0">
              <span class="opacity-70 whitespace-nowrap">当前节点:</span>
              <strong
                data-testid="sub-current-node-count"
                class="font-mono text-sm"
                :class="subscription.enabled ? 'text-primary' : 'text-base-content/50'"
              >
                {{ subscription.enabled ? (subscription.node_count ?? 0) : 0 }}
              </strong>
            </div>
            <div v-if="!subscription.enabled && (subscription.source_node_count ?? 0) > 0" class="text-[11px] opacity-65 font-mono truncate" data-testid="sub-retained-members" :title="t('subscriptions.retainedSourceMemberHint')">
              {{ t('subscriptions.retainedSourceMembers', { count: subscription.source_node_count }) }}
            </div>
            <div v-else-if="subscription.enabled && subscription.source_node_count !== undefined && subscription.source_node_count !== subscription.node_count" class="text-[11px] opacity-60 font-mono truncate">
              {{ t('subscriptions.sourceTotal', { count: subscription.source_node_count }) }}
            </div>
          </div>

          <!-- Badges for Advanced Config -->
          <div v-if="subscription.config?.cron_schedule || subscription.config?.auto_test || subscription.config?.rename_rules?.length || subscription.config?.filter_rules?.length || subscription.config?.target_groups?.length" class="flex flex-wrap gap-1.5 pt-1">
            <span v-if="subscription.config?.cron_schedule" class="badge badge-neutral badge-xs font-mono text-[10px]">
              {{ subscription.config.cron_schedule }}
            </span>
            <span v-if="subscription.config?.auto_test" class="badge badge-primary badge-outline badge-xs text-[10px]">
              自动探测
            </span>
            <span v-if="subscription.config?.rename_rules?.length" class="badge badge-info badge-outline badge-xs text-[10px]">
              {{ subscription.config.rename_rules.length }} 条重命名
            </span>
            <span v-if="subscription.config?.filter_rules?.length" class="badge badge-accent badge-outline badge-xs text-[10px]">
              {{ subscription.config.filter_rules.length }} 条过滤
            </span>
            <span v-if="subscription.config?.target_groups?.length" class="badge badge-secondary badge-outline badge-xs text-[10px]">
              {{ subscription.config.target_groups.length }} 个目标组
            </span>
          </div>

          <dl class="grid grid-cols-2 gap-3 text-xs">
            <div>
              <dt class="opacity-60">{{ t('subscriptions.refreshInterval') }}</dt>
              <dd class="mt-1 font-medium">{{ subscription.refresh_policy.interval_seconds }} 秒</dd>
            </div>
            <div>
              <dt class="opacity-60">{{ t('subscriptions.timeout') }}</dt>
              <dd class="mt-1 font-medium">{{ subscription.refresh_policy.timeout_seconds }} 秒</dd>
            </div>
            <div class="col-span-2 min-w-0" data-testid="subscription-last-refresh">
              <dt class="opacity-60">{{ t('subscriptions.lastRefreshed') }}</dt>
              <dd class="mt-1 font-medium min-w-0">
                <span v-if="!subscription.last_refreshed_at" class="opacity-60" data-testid="sub-never-refreshed">
                  {{ t('subscriptions.neverRefreshed') }}
                </span>
                <div v-else-if="subscription.last_refresh_outcome === 'failed'" class="flex items-center gap-2 flex-wrap min-w-0" data-testid="sub-refresh-failed">
                  <span class="badge badge-error badge-sm h-auto min-h-[1.25rem] py-0.5 px-2 font-medium shrink-0 whitespace-nowrap text-error-content">
                    {{ t('subscriptions.refreshFailed') }}
                  </span>
                  <span
                    :title="formatFullDateTime(subscription.last_refreshed_at, currentLocale)"
                    class="truncate text-error/90 font-mono text-[11px] cursor-help"
                    data-testid="sub-refresh-time"
                  >
                    {{ formatRefreshTime(subscription.last_refreshed_at, currentLocale) }}
                  </span>
                </div>
                <div v-else-if="subscription.last_refresh_outcome === 'partial'" class="flex items-center gap-2 flex-wrap min-w-0" data-testid="sub-refresh-partial">
                  <span class="badge badge-warning badge-sm h-auto min-h-[1.25rem] py-0.5 px-2 font-medium shrink-0 whitespace-nowrap text-warning-content">
                    {{ t('subscriptions.refreshPartial') }}
                  </span>
                  <span
                    :title="formatFullDateTime(subscription.last_refreshed_at, currentLocale)"
                    class="truncate opacity-80 font-mono text-[11px] cursor-help"
                    data-testid="sub-refresh-time"
                  >
                    {{ formatRefreshTime(subscription.last_refreshed_at, currentLocale) }}
                  </span>
                </div>
                <div v-else class="flex items-center gap-2 flex-wrap min-w-0" data-testid="sub-refresh-success">
                  <span class="badge badge-success badge-outline badge-sm h-auto min-h-[1.25rem] py-0.5 px-2 font-medium shrink-0 whitespace-nowrap">
                    {{ t('subscriptions.refreshSuccess') }}
                  </span>
                  <span
                    :title="formatFullDateTime(subscription.last_refreshed_at, currentLocale)"
                    class="truncate opacity-80 font-mono text-[11px] cursor-help"
                    data-testid="sub-refresh-time"
                  >
                    {{ formatRefreshTime(subscription.last_refreshed_at, currentLocale) }}
                  </span>
                </div>
              </dd>
            </div>
          </dl>
        </div>
      </article>
    </div>
    <div v-if="items.length" class="flex flex-wrap items-center justify-between gap-2 text-xs opacity-60 pt-1">
      <span class="italic">{{ t('subscriptions.perSubCountNotice') }}</span>
      <p data-testid="subscription-total-count">
        {{ t('subscriptions.totalOf', { count: items.length, total }) }}
      </p>
    </div>

    <!-- Advanced Configuration Drawer -->
    <SubscriptionConfigDrawer
      v-model="drawerOpen"
      :subscription="editing"
      :saving="saving"
      @save="handleDrawerSave"
    />

    <!-- Source Entries Drawer -->
    <SubscriptionEntriesDrawer
      v-model="entriesDrawerOpen"
      :subscription="viewingEntriesSub"
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

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  DocumentDuplicateIcon,
  ArrowPathIcon,
  PlusIcon,
} from '@heroicons/vue/24/outline'
import PageToolbar from '../../ui/PageToolbar.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import EmptyState from '../../ui/EmptyState.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import { toastStore } from '../../ui/toast'
import SubscriptionConfigDrawer from './SubscriptionConfigDrawer.vue'
import SubscriptionEntriesDrawer from './SubscriptionEntriesDrawer.vue'
import SubscriptionCard from './SubscriptionCard.vue'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayName, displayText } from '../../utils/displayNames'
import {
  useSubscriptions,
  type SubscriptionDraft,
  type SubscriptionRecord,
} from './useSubscriptions'

const {
  items,
  loading,
  saving,
  error,
  total,
  page,
  pageCount,
  searchText,
  setPage,
  search,
  load,
  save,
  remove,
  refresh,
  refreshingIDs,
  toggle,
  togglingIDs,
  setPrimary,
  settingPrimaryIDs,
  rowErrors,
  clearRowError,
} = useSubscriptions()

const drawerOpen = ref(false)
const editing = ref<SubscriptionRecord>()
const confirmDeleteOpen = ref(false)
const pendingDeleteSubscription = ref<SubscriptionRecord | null>(null)
const deleting = ref(false)
const entriesDrawerOpen = ref(false)
const viewingEntriesSub = ref<SubscriptionRecord>()

const searchDraft = ref(searchText.value)

function submitSearch() {
  void search(searchDraft.value)
}

async function handleSetPrimary(subscription: SubscriptionRecord) {
  try {
    await setPrimary(subscription)
  } catch {
    // handled with toast & rowErrors
  }
}

async function handleToggle(subscription: SubscriptionRecord) {
  try {
    await toggle(subscription)
  } catch {
    // handled with toast & rowErrors
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

async function handleDrawerSave(draft: SubscriptionDraft, settled: (error?: string) => void) {
  try {
    const saved = await save(draft, editing.value)
    if (!saved) {
      settled(uiText('正在保存，请稍候', 'A save is already in progress'))
      return
    }
    settled()
    drawerOpen.value = false
  } catch (cause) {
    settled(displayText(cause instanceof Error ? cause.message : uiText('保存订阅源失败', 'Failed to save subscription')))
  }
}

function confirmRemove(subscription: SubscriptionRecord) {
  pendingDeleteSubscription.value = subscription
  confirmDeleteOpen.value = true
}

async function handleConfirmDelete() {
  if (!pendingDeleteSubscription.value || deleting.value) return
  deleting.value = true
  try {
    const res = await remove(pendingDeleteSubscription.value)
    if (res.ok) {
      confirmDeleteOpen.value = false
      pendingDeleteSubscription.value = null
    }
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
  <section class="space-y-5" aria-labelledby="subscriptions-title" data-testid="subscriptions-view">
    <!-- Non-redundant Toolbar: renders page title without duplicate visible h1 -->
    <PageToolbar
      id="subscriptions-title"
      :title="t('subscriptions.title')"
      :description="t('subscriptions.subtitle')"
    >
      <!-- Center stats: Compact summary, no fake 0 on initial loading or error -->
      <div class="flex items-center gap-2 text-xs text-base-content/70">
        <span v-if="loading && items.length === 0" class="loading loading-spinner loading-xs text-primary" />
        <template v-else-if="!error">
          <span class="font-medium text-base-content" data-testid="toolbar-sources-count">
            {{ uiText(`共 ${total} 个订阅源`, `${total} sources total`) }}
          </span>
        </template>
      </div>

      <!-- Actions Slot: Add Subscription button -->
      <template #actions>
        <button
          class="btn btn-ghost btn-sm gap-2"
          type="button"
          :disabled="loading"
          data-testid="subscription-list-reload"
          @click="load"
        >
          <ArrowPathIcon class="h-4 w-4" :class="{ 'animate-spin': loading }" />
          {{ uiText('刷新列表', 'Reload list') }}
        </button>
        <button
          class="btn btn-primary btn-sm gap-2 touch-manipulation shadow-xs shrink-0 self-start sm:self-auto"
          type="button"
          @click="openCreate"
        >
          <PlusIcon class="h-4 w-4" />
          <span>{{ t('subscriptions.add') }}</span>
        </button>
      </template>
    </PageToolbar>

    <form class="flex flex-wrap items-end gap-2" @submit.prevent="submitSearch">
      <label for="subscription-search" class="flex flex-1 flex-col gap-1 min-w-48 text-xs text-base-content/70">
        {{ uiText('搜索订阅源', 'Search subscriptions') }}
        <input
          id="subscription-search"
          v-model="searchDraft"
          type="search"
          class="input input-sm w-full"
          :placeholder="uiText('订阅源名称', 'Subscription name')"
          data-testid="subscription-search-input"
        />
      </label>
      <button type="submit" class="btn btn-sm btn-outline" :disabled="loading" data-testid="subscription-search-submit">
        {{ uiText('搜索', 'Search') }}
      </button>
      <button
        v-if="searchText || searchDraft"
        type="button"
        class="btn btn-sm btn-ghost"
        :disabled="loading"
        @click="searchDraft = ''; search('')"
      >
        {{ uiText('清除搜索', 'Clear search') }}
      </button>
    </form>

    <!-- Error State -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loading"
      @retry="load"
    />

    <!-- Initial Loading State (Skeletons) -->
    <div v-if="loading && items.length === 0" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      <div v-for="index in 3" :key="index" class="skeleton h-44 rounded-2xl" />
    </div>

    <!-- Empty State -->
    <EmptyState
      v-else-if="!loading && !error && items.length === 0"
      :icon="DocumentDuplicateIcon"
      :title="searchText ? uiText('没有匹配的订阅源', 'No matching subscriptions') : t('subscriptions.emptyTitle')"
      :description="searchText ? uiText('换个名称搜索，或清除搜索查看全部订阅源', 'Try another name or clear the search to see all sources') : t('subscriptions.emptyDesc')"
      :action-label="t('subscriptions.add')"
      @action="openCreate"
    >
      <template #action-icon>
        <PlusIcon class="h-4 w-4" />
      </template>
    </EmptyState>

    <!-- Subscriptions Cards Grid -->
    <div v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3 w-full min-w-0">
      <SubscriptionCard
        v-for="subscription in items"
        :key="subscription.id"
        :subscription="subscription"
        :refreshing="refreshingIDs.has(subscription.id)"
        :toggling="togglingIDs.has(subscription.id)"
        :setting-primary="settingPrimaryIDs.has(subscription.id)"
        :row-error="rowErrors[subscription.id]"
        @refresh="refresh(subscription)"
        @toggle="handleToggle(subscription)"
        @set-primary="handleSetPrimary(subscription)"
        @open-entries="openEntries(subscription)"
        @edit="openEdit(subscription)"
        @confirm-remove="confirmRemove(subscription)"
        @copy="copyReference(subscription)"
        @clear-error="clearRowError(subscription.id)"
      />
    </div>

    <!-- Total count footer -->
    <div v-if="items.length && !loading && !error" class="flex flex-wrap items-center justify-between gap-2 text-xs opacity-60 pt-1">
      <span class="italic">{{ t('subscriptions.perSubCountNotice') }}</span>
      <p data-testid="subscription-total-count">
        {{ t('subscriptions.totalOf', { count: items.length, total }) }}
      </p>
    </div>

    <nav v-if="total > 0 || page > 1" class="flex flex-wrap items-center justify-between gap-3 text-xs" :aria-label="uiText('订阅源分页', 'Subscription pagination')">
      <span>{{ uiText(`第 ${page} / ${pageCount} 页 · 每页最多 50 个`, `Page ${page} / ${pageCount} · up to 50 per page`) }}</span>
      <div class="flex gap-2">
        <button type="button" class="btn btn-sm btn-ghost" :disabled="loading || page <= 1" data-testid="subscription-page-prev" @click="setPage(page - 1)">
          {{ uiText('上一页', 'Previous') }}
        </button>
        <button type="button" class="btn btn-sm btn-ghost" :disabled="loading || page >= pageCount" data-testid="subscription-page-next" @click="setPage(page + 1)">
          {{ uiText('下一页', 'Next') }}
        </button>
      </div>
    </nav>

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

    <!-- Confirm Remove Modal -->
    <ConfirmModal
      v-model="confirmDeleteOpen"
      :title="t('subscriptions.deleteConfirmTitle')"
      :message="pendingDeleteSubscription ? t('subscriptions.deleteConfirmDesc', { name: displayName(pendingDeleteSubscription.name, uiText('未命名订阅', 'Unnamed subscription'), pendingDeleteSubscription.id) }) : ''"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      tone="danger"
      :loading="deleting"
      @confirm="handleConfirmDelete"
    />
  </section>
</template>

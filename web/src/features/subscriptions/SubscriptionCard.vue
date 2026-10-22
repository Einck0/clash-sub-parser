<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowPathIcon,
  DocumentDuplicateIcon,
  EllipsisVerticalIcon,
  ListBulletIcon,
  PencilSquareIcon,
  StarIcon,
  TrashIcon,
  ExclamationCircleIcon,
  XMarkIcon,
} from '@heroicons/vue/24/outline'
import Popover from '../../ui/Popover.vue'
import { t, currentLocale } from '../../locales'
import {
  formatRefreshTime,
  formatFullDateTime,
  parseUserInfo,
  formatBytes,
  type SubscriptionRecord,
} from './useSubscriptions'

const props = defineProps<{
  subscription: SubscriptionRecord
  refreshing?: boolean
  toggling?: boolean
  settingPrimary?: boolean
  rowError?: string
}>()

const emit = defineEmits<{
  (e: 'refresh', sub: SubscriptionRecord): void
  (e: 'toggle', sub: SubscriptionRecord): void
  (e: 'set-primary', sub: SubscriptionRecord): void
  (e: 'open-entries', sub: SubscriptionRecord): void
  (e: 'edit', sub: SubscriptionRecord): void
  (e: 'confirm-remove', sub: SubscriptionRecord): void
  (e: 'copy', sub: SubscriptionRecord): void
  (e: 'clear-error', id: string): void
}>()

const parsedUserinfo = computed(() => parseUserInfo(props.subscription.userinfo))
</script>

<template>
  <article
    data-testid="subscription-card"
    class="card border border-base-300 bg-base-200/60 shadow-xs transition hover:-translate-y-0.5 hover:shadow-sm min-w-0 w-full overflow-hidden rounded-2xl"
  >
    <div class="card-body gap-3 p-4 sm:p-5 min-w-0 max-w-full">
      <!-- Card Header -->
      <div class="flex flex-wrap items-start justify-between gap-2 min-w-0" data-testid="subscription-card-header">
        <div class="min-w-0 flex-1 basis-full sm:basis-40">
          <div class="flex items-center gap-1.5 min-w-0 flex-wrap">
            <h3 class="truncate font-semibold text-sm sm:text-base text-base-content">{{ subscription.name }}</h3>
            <span
              v-if="subscription.is_primary"
              class="badge badge-warning badge-sm font-semibold shrink-0 gap-1 text-[11px]"
              data-testid="primary-sub-badge"
              title="当前主力订阅，流量与过期时间将透传至客户端"
            >
              <StarIcon class="h-3 w-3 fill-current" />
              主订阅
            </span>
          </div>
          <p class="mt-1 truncate font-mono text-xs opacity-60 break-all">{{ subscription.source_url_secret_ref }}</p>
        </div>

        <div class="flex flex-wrap items-center gap-1.5 min-w-0 max-w-full">
          <!-- Refresh Button -->
          <button
            type="button"
            class="btn btn-ghost btn-xs border border-base-300/80 px-2 gap-1 touch-manipulation hover:bg-base-300 rounded-lg"
            data-testid="subscription-card-refresh-btn"
            :disabled="refreshing"
            :title="refreshing ? t('subscriptions.refreshing') : t('common.refresh')"
            @click="emit('refresh', subscription)"
          >
            <ArrowPathIcon class="h-3.5 w-3.5" :class="{ 'animate-spin': refreshing }" />
            <span>{{ refreshing ? t('subscriptions.refreshing') : t('common.refresh') }}</span>
          </button>

          <!-- View Entries Button -->
          <button
            type="button"
            class="btn btn-ghost btn-xs border border-base-300/80 px-2 gap-1 touch-manipulation hover:bg-base-300 rounded-lg"
            data-testid="sub-view-entries"
            :title="t('subscriptions.viewEntries')"
            @click="emit('open-entries', subscription)"
          >
            <ListBulletIcon class="h-3.5 w-3.5" />
            <span>{{ t('subscriptions.viewEntries') }}</span>
          </button>

          <!-- Toggle Button -->
          <button
            type="button"
            data-testid="subscription-toggle-btn"
            class="btn btn-xs gap-1 touch-manipulation transition-all rounded-lg"
            :class="subscription.enabled ? 'btn-success btn-outline' : 'btn-ghost text-base-content/60 border border-base-300'"
            :disabled="toggling"
            :title="subscription.enabled ? '点击停用订阅源' : '点击启用订阅源'"
            @click="emit('toggle', subscription)"
          >
            <span
              class="w-2 h-2 rounded-full"
              :class="subscription.enabled ? 'bg-success animate-pulse' : 'bg-base-content/40'"
            />
            <span>{{ subscription.enabled ? t('common.enabled') : t('common.disabled') }}</span>
          </button>

          <!-- More Actions Popover -->
          <Popover placement="bottom-end" panel-class="w-44 max-w-[calc(100vw-2rem)]">
            <template #trigger="{ open }">
              <button
                type="button"
                data-testid="subscription-actions-trigger"
                class="btn btn-ghost btn-xs btn-square border border-base-300/80 touch-manipulation rounded-lg"
                :class="{ 'bg-base-300': open }"
                :aria-label="t('common.moreActions')"
              >
                <EllipsisVerticalIcon class="h-4 w-4" />
              </button>
            </template>
            <template #default="{ close }">
              <ul class="menu menu-xs p-1 text-xs gap-0.5">
                <li>
                  <button
                    type="button"
                    data-testid="sub-action-refresh"
                    class="touch-manipulation py-1.5"
                    :disabled="refreshing"
                    @click="close(); emit('refresh', subscription)"
                  >
                    <ArrowPathIcon class="h-3.5 w-3.5" :class="{ 'animate-spin': refreshing }" />
                    <span>{{ refreshing ? t('subscriptions.refreshing') : t('common.refresh') }}</span>
                  </button>
                </li>
                <li>
                  <button
                    type="button"
                    data-testid="sub-action-edit"
                    class="touch-manipulation py-1.5"
                    @click="close(); emit('edit', subscription)"
                  >
                    <PencilSquareIcon class="h-3.5 w-3.5" />
                    <span>{{ t('common.edit') }}</span>
                  </button>
                </li>
                <li>
                  <button
                    type="button"
                    data-testid="sub-action-primary"
                    class="touch-manipulation py-1.5"
                    :disabled="subscription.is_primary || settingPrimary"
                    @click="close(); emit('set-primary', subscription)"
                  >
                    <StarIcon class="h-3.5 w-3.5" :class="{ 'fill-current text-warning': subscription.is_primary }" />
                    <span>{{ subscription.is_primary ? '当前主订阅' : '设为主订阅' }}</span>
                  </button>
                </li>
                <li>
                  <button
                    type="button"
                    data-testid="sub-action-copy"
                    class="touch-manipulation py-1.5"
                    @click="close(); emit('copy', subscription)"
                  >
                    <DocumentDuplicateIcon class="h-3.5 w-3.5" />
                    <span>{{ t('common.copy') }}</span>
                  </button>
                </li>
                <li>
                  <button
                    type="button"
                    data-testid="sub-action-delete"
                    class="text-error touch-manipulation py-1.5"
                    @click="close(); emit('confirm-remove', subscription)"
                  >
                    <TrashIcon class="h-3.5 w-3.5" />
                    <span>{{ t('common.delete') }}</span>
                  </button>
                </li>
              </ul>
            </template>
          </Popover>
        </div>
      </div>

      <!-- Refreshing feedback banner -->
      <div
        v-if="refreshing"
        data-testid="sub-refresh-loading"
        class="alert alert-info py-1.5 px-3 text-xs flex items-center gap-2 rounded-xl"
      >
        <span class="loading loading-spinner loading-xs shrink-0" />
        <span>{{ t('subscriptions.refreshing') }}</span>
      </div>

      <!-- Row-level Error message if present -->
      <div
        v-if="rowError"
        data-testid="sub-row-error"
        class="alert alert-error py-1.5 px-3 text-xs flex items-center justify-between gap-2 rounded-xl"
      >
        <div class="flex items-center gap-2 min-w-0">
          <ExclamationCircleIcon class="h-4 w-4 shrink-0" />
          <span class="truncate">{{ rowError }}</span>
        </div>
        <button
          type="button"
          class="btn btn-ghost btn-xs btn-circle shrink-0"
          aria-label="Dismiss error"
          @click="emit('clear-error', subscription.id)"
        >
          <XMarkIcon class="h-3.5 w-3.5" />
        </button>
      </div>

      <!-- Userinfo Traffic stats if present -->
      <div
        v-if="subscription.userinfo && parsedUserinfo"
        data-testid="subscription-userinfo"
        class="rounded-xl border border-base-300/80 bg-base-100/70 p-2.5 text-xs space-y-1.5"
      >
        <div class="flex items-center justify-between font-medium text-[11px] opacity-75">
          <span>流量与周期</span>
          <span v-if="parsedUserinfo.expireDate" class="font-mono">
            到期: {{ parsedUserinfo.expireDate }}
          </span>
          <span v-else class="opacity-50">长期有效</span>
        </div>
        <div v-if="parsedUserinfo.total > 0" class="space-y-1">
          <progress
            class="progress progress-primary w-full h-1.5"
            :value="parsedUserinfo.used"
            :max="parsedUserinfo.total"
          />
          <div class="flex justify-between text-[11px] font-mono opacity-70">
            <span>已用: {{ formatBytes(parsedUserinfo.used) }}</span>
            <span>总计: {{ formatBytes(parsedUserinfo.total) }}</span>
            <span>剩余: {{ formatBytes(parsedUserinfo.remaining) }}</span>
          </div>
        </div>
      </div>

      <!-- Metadata definition list -->
      <dl class="grid grid-cols-2 gap-2 text-xs">
        <div>
          <dt class="opacity-60">{{ t('subscriptions.interval') }}</dt>
          <dd class="font-mono mt-0.5">
            {{ Math.round(subscription.refresh_policy.interval_seconds / 60) }} {{ t('subscriptions.minutes') }}
          </dd>
        </div>
        <div>
          <dt class="opacity-60">{{ t('subscriptions.timeout') }}</dt>
          <dd class="font-mono mt-0.5">{{ subscription.refresh_policy.timeout_seconds }}s</dd>
        </div>
        <div v-if="subscription.prefix">
          <dt class="opacity-60">{{ t('subscriptions.prefix') }}</dt>
          <dd class="font-mono mt-0.5 truncate">{{ subscription.prefix }}</dd>
        </div>
        <div>
          <dt class="opacity-60">{{ t('subscriptions.nodeCount') }}</dt>
          <dd class="font-mono mt-0.5">{{ subscription.node_count ?? '-' }}</dd>
        </div>

        <!-- Dedicated Last Refresh Row (col-span-2) -->
        <div class="col-span-2 pt-1 border-t border-base-300/50" data-testid="subscription-last-refresh">
          <dt class="opacity-60 text-[11px] mb-1">{{ t('subscriptions.lastRefreshed') }}</dt>
          <dd class="min-w-0">
            <div
              v-if="!subscription.last_refreshed_at"
              data-testid="sub-never-refreshed"
              class="text-xs opacity-50 italic"
            >
              {{ t('subscriptions.neverRefreshed') }}
            </div>
            <div
              v-else-if="subscription.last_refresh_outcome === 'success'"
              data-testid="sub-refresh-success"
              class="flex flex-wrap items-center gap-1.5 min-w-0"
            >
              <span class="badge badge-success badge-sm h-auto whitespace-nowrap text-success-content font-medium py-0.5">
                {{ t('subscriptions.outcomeSuccess') }}
              </span>
              <span
                data-testid="sub-refresh-time"
                class="truncate opacity-80"
                :title="formatFullDateTime(subscription.last_refreshed_at, currentLocale)"
              >
                {{ formatRefreshTime(subscription.last_refreshed_at, currentLocale) }}
              </span>
            </div>
            <div
              v-else-if="subscription.last_refresh_outcome === 'failed'"
              data-testid="sub-refresh-failed"
              class="flex flex-wrap items-center gap-1.5 min-w-0"
            >
              <span class="badge badge-error badge-sm h-auto whitespace-nowrap text-error-content font-medium py-0.5">
                {{ t('subscriptions.outcomeFailed') }}
              </span>
              <span
                data-testid="sub-refresh-time"
                class="truncate text-error font-medium"
                :title="formatFullDateTime(subscription.last_refreshed_at, currentLocale)"
              >
                {{ formatRefreshTime(subscription.last_refreshed_at, currentLocale) }}
              </span>
            </div>
            <div
              v-else-if="subscription.last_refresh_outcome === 'partial'"
              data-testid="sub-refresh-partial"
              class="flex flex-wrap items-center gap-1.5 min-w-0"
            >
              <span class="badge badge-warning badge-sm h-auto whitespace-nowrap text-warning-content font-medium py-0.5">
                {{ t('subscriptions.outcomePartial') }}
              </span>
              <span
                data-testid="sub-refresh-time"
                class="truncate opacity-80"
                :title="formatFullDateTime(subscription.last_refreshed_at, currentLocale)"
              >
                {{ formatRefreshTime(subscription.last_refreshed_at, currentLocale) }}
              </span>
            </div>
            <div
              v-else
              class="flex flex-wrap items-center gap-1.5 min-w-0"
            >
              <span class="badge badge-neutral badge-sm h-auto whitespace-nowrap font-medium py-0.5">
                {{ subscription.last_refresh_outcome || 'Unknown' }}
              </span>
              <span
                data-testid="sub-refresh-time"
                class="truncate opacity-80"
                :title="formatFullDateTime(subscription.last_refreshed_at, currentLocale)"
              >
                {{ formatRefreshTime(subscription.last_refreshed_at, currentLocale) }}
              </span>
            </div>
          </dd>
        </div>
      </dl>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import {
  ArrowPathIcon,
  ExclamationTriangleIcon,
  MegaphoneIcon,
  ShieldCheckIcon,
  SparklesIcon,
} from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import EmptyState from '../../ui/EmptyState.vue'
import { t, currentLocale } from '../../locales'
import {
  useSubscriptionEntries,
  formatFullDateTime,
  type EntryKind,
  type SubscriptionEntryDTO,
  type SubscriptionRecord,
} from './useSubscriptions'

const props = defineProps<{
  modelValue: boolean
  subscription?: SubscriptionRecord
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

const {
  entries,
  loading,
  error,
  total,
  overridingId,
  loadEntries,
  setOverride,
} = useSubscriptionEntries()

watch(
  () => [props.modelValue, props.subscription?.id],
  ([open, subId]) => {
    if (open && subId) {
      void loadEntries(subId as string)
    }
  },
  { immediate: true }
)

const notices = computed(() =>
  entries.value.filter((e) => e.effective_kind === 'notice')
)

const proxyEntries = computed(() =>
  entries.value.filter((e) => e.effective_kind === 'proxy')
)

const unknownEntries = computed(() =>
  entries.value.filter((e) => e.effective_kind === 'unknown')
)

async function handleOverride(
  entry: SubscriptionEntryDTO,
  newKind: EntryKind | null
) {
  if (!props.subscription?.id) return
  if (entry.user_kind_override === newKind) return
  await setOverride(props.subscription.id, entry.entry_id, newKind)
}

function handleRefresh() {
  if (props.subscription?.id) {
    void loadEntries(props.subscription.id)
  }
}
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="subscription ? `${subscription.name} · ${t('subscriptions.entriesTitle')}` : t('subscriptions.entriesTitle')"
    :description="t('subscriptions.entriesSubtitle')"
    width-class="max-w-3xl"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="space-y-4 text-xs">
      <!-- Toolbar / Stats Summary -->
      <div class="flex flex-wrap items-center justify-between gap-2 p-3 rounded-xl bg-base-100 border border-base-300">
        <div class="flex flex-wrap items-center gap-2">
          <span class="font-bold text-sm text-base-content">
            {{ t('subscriptions.entriesTitle') }}
          </span>
          <span class="badge badge-sm badge-neutral font-mono">
            {{ total }} 条
          </span>
          <span class="badge badge-sm badge-success badge-outline font-mono">
            节点: {{ proxyEntries.length }}
          </span>
          <span class="badge badge-sm badge-info badge-outline font-mono">
            公告: {{ notices.length }}
          </span>
          <span
            v-if="unknownEntries.length > 0"
            class="badge badge-sm badge-warning badge-outline font-mono"
          >
            未知: {{ unknownEntries.length }}
          </span>
        </div>

        <button
          type="button"
          class="btn btn-ghost btn-xs gap-1 touch-manipulation"
          :disabled="loading"
          data-testid="entries-refresh-btn"
          @click="handleRefresh"
        >
          <ArrowPathIcon class="w-3.5 h-3.5" :class="{ 'animate-spin': loading }" />
          <span>{{ t('common.refresh') }}</span>
        </button>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="space-y-2">
        <div v-for="i in 3" :key="i" class="skeleton h-20 rounded-xl" />
      </div>

      <!-- Error State -->
      <ErrorStateCard
        v-else-if="error"
        :error="error"
        :retrying="loading"
        @retry="handleRefresh"
      />

      <!-- Empty State -->
      <EmptyState
        v-else-if="entries.length === 0"
        :icon="SparklesIcon"
        :title="t('subscriptions.noEntriesFound')"
        description=""
      />

      <!-- Content Area -->
      <div v-else class="space-y-4">
        <!-- Announcements / Notices Section (Preserved, distinct from proxy nodes) -->
        <section
          v-if="notices.length > 0"
          data-testid="subscription-notices-section"
          class="rounded-xl border border-info/30 bg-info/10 p-3.5 sm:p-4 space-y-3"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2 font-bold text-info text-sm">
              <MegaphoneIcon class="w-4 h-4 shrink-0" />
              <span>{{ t('subscriptions.noticesSectionTitle') }}</span>
            </div>
            <span class="badge badge-xs badge-info font-mono">
              {{ notices.length }} 条
            </span>
          </div>

          <div class="space-y-2">
            <article
              v-for="notice in notices"
              :key="notice.entry_id"
              data-testid="subscription-notice-item"
              class="rounded-lg bg-base-100/90 border border-info/20 p-3 space-y-2 shadow-xs"
            >
              <div class="flex flex-wrap items-start justify-between gap-2">
                <div class="space-y-0.5 min-w-0 flex-1">
                  <div class="flex items-center gap-1.5 flex-wrap">
                    <span class="badge badge-xs font-mono badge-neutral">#{{ notice.ordinal }}</span>
                    <strong class="text-base-content text-xs break-all">{{ notice.name }}</strong>
                    <span class="badge badge-xs badge-info badge-outline font-mono">
                      {{ t('subscriptions.kindNotice') }}
                    </span>
                    <span
                      v-if="notice.user_kind_override"
                      class="badge badge-xs badge-primary font-mono"
                    >
                      手动覆盖: {{ notice.user_kind_override }}
                    </span>
                  </div>
                  <p v-if="notice.classification_reason" class="text-[11px] opacity-75">
                    判定原因: {{ notice.classification_reason }}
                  </p>
                </div>

                <!-- Inline Quick Switcher -->
                <div class="flex items-center gap-1 shrink-0" role="group" aria-label="分类覆盖切换">
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg"
                    :class="notice.user_kind_override === 'proxy' ? 'btn-success' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === notice.entry_id"
                    data-testid="notice-override-proxy-btn"
                    title="纠偏为代理节点"
                    @click="handleOverride(notice, 'proxy')"
                  >
                    节点
                  </button>
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg"
                    :class="notice.user_kind_override === 'notice' ? 'btn-info' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === notice.entry_id"
                    data-testid="notice-override-notice-btn"
                    title="设为公告"
                    @click="handleOverride(notice, 'notice')"
                  >
                    公告
                  </button>
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg"
                    :class="notice.user_kind_override === 'unknown' ? 'btn-warning' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === notice.entry_id"
                    data-testid="notice-override-unknown-btn"
                    title="设为未知"
                    @click="handleOverride(notice, 'unknown')"
                  >
                    未知
                  </button>
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg"
                    :class="notice.user_kind_override === null || notice.user_kind_override === undefined ? 'btn-neutral' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === notice.entry_id"
                    data-testid="notice-override-auto-btn"
                    title="清除覆盖，恢复自动识别"
                    @click="handleOverride(notice, null)"
                  >
                    自动
                  </button>
                </div>
              </div>

              <!-- Conflict Alert if present -->
              <div
                v-if="notice.conflict"
                data-testid="entry-conflict-warning"
                class="p-2 rounded bg-error/15 border border-error/30 text-error text-xs flex items-center gap-1.5"
              >
                <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
                <span>{{ t('subscriptions.conflictWarning') }}: {{ notice.conflict }}</span>
              </div>
            </article>
          </div>
        </section>

        <!-- All Source Entries List -->
        <section class="space-y-2">
          <div class="font-bold text-xs opacity-75 uppercase tracking-wider">
            全部来源条目 ({{ entries.length }})
          </div>

          <div class="space-y-2.5">
            <article
              v-for="entry in entries"
              :key="entry.entry_id"
              data-testid="subscription-entry-item"
              class="rounded-xl border border-base-300 bg-base-200/70 p-3.5 space-y-2 transition-all hover:border-base-content/20"
            >
              <div class="flex flex-col sm:flex-row sm:items-start justify-between gap-2">
                <div class="space-y-1 min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-1.5">
                    <span class="badge badge-xs font-mono badge-neutral">#{{ entry.ordinal }}</span>
                    <strong class="text-base-content text-sm break-all font-semibold">
                      {{ entry.name }}
                    </strong>
                    <!-- Effective Kind Badge -->
                    <span
                      class="badge badge-xs font-mono"
                      :class="{
                        'badge-success': entry.effective_kind === 'proxy',
                        'badge-info': entry.effective_kind === 'notice',
                        'badge-warning': entry.effective_kind === 'unknown',
                      }"
                      data-testid="entry-effective-kind"
                    >
                      生效: {{ entry.effective_kind }}
                    </span>
                    <span class="badge badge-xs badge-ghost font-mono opacity-70">
                      原始: {{ entry.entry_kind }}
                    </span>
                    <span
                      v-if="entry.user_kind_override"
                      class="badge badge-xs badge-primary font-mono"
                    >
                      覆盖为: {{ entry.user_kind_override }}
                    </span>
                  </div>

                  <p v-if="entry.classification_reason" class="text-xs opacity-75 leading-relaxed">
                    判定原因: {{ entry.classification_reason }}
                  </p>

                  <div class="flex flex-wrap items-center gap-3 text-[11px] opacity-60 font-mono">
                    <span v-if="entry.payload_id">
                      Payload ID: {{ entry.payload_id }}
                    </span>
                    <span v-if="entry.node_logical_id">
                      逻辑 ID: {{ entry.node_logical_id }}
                    </span>
                    <span v-if="entry.override_at">
                      覆盖时间: {{ formatFullDateTime(entry.override_at, currentLocale) }}
                    </span>
                    <span v-if="entry.override_reason">
                      理由: {{ entry.override_reason }}
                    </span>
                  </div>
                </div>

                <!-- Inline Quick Switcher (proxy / notice / unknown / auto null) -->
                <div class="flex items-center gap-1 shrink-0 self-start pt-1" role="group" aria-label="分类覆盖切换">
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg transition-all"
                    :class="entry.user_kind_override === 'proxy' ? 'btn-success font-bold' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === entry.entry_id"
                    data-testid="override-proxy-btn"
                    title="纠偏为代理节点"
                    @click="handleOverride(entry, 'proxy')"
                  >
                    节点
                  </button>
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg transition-all"
                    :class="entry.user_kind_override === 'notice' ? 'btn-info font-bold' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === entry.entry_id"
                    data-testid="override-notice-btn"
                    title="标记为公告"
                    @click="handleOverride(entry, 'notice')"
                  >
                    公告
                  </button>
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg transition-all"
                    :class="entry.user_kind_override === 'unknown' ? 'btn-warning font-bold' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === entry.entry_id"
                    data-testid="override-unknown-btn"
                    title="标记为未知"
                    @click="handleOverride(entry, 'unknown')"
                  >
                    未知
                  </button>
                  <button
                    type="button"
                    class="btn btn-xs rounded-lg transition-all"
                    :class="entry.user_kind_override === null || entry.user_kind_override === undefined ? 'btn-neutral font-bold' : 'btn-ghost border-base-300'"
                    :disabled="overridingId === entry.entry_id"
                    data-testid="override-auto-btn"
                    title="清除覆盖，恢复系统自动识别"
                    @click="handleOverride(entry, null)"
                  >
                    自动
                  </button>
                </div>
              </div>

              <!-- Conflict Alert if present -->
              <div
                v-if="entry.conflict"
                data-testid="entry-conflict-warning"
                class="p-2 rounded bg-error/15 border border-error/30 text-error text-xs flex items-center gap-1.5"
              >
                <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
                <span>{{ t('subscriptions.conflictWarning') }}: {{ entry.conflict }}</span>
              </div>
            </article>
          </div>
        </section>
      </div>
    </div>
  </DrawerCard>
</template>

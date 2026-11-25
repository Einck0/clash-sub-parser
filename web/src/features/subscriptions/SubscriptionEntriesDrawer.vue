<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { ArrowPathIcon, ExclamationTriangleIcon, SparklesIcon } from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import EmptyState from '../../ui/EmptyState.vue'
import { t, currentLocale } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayName, displayText } from '../../utils/displayNames'
import {
  useSubscriptionEntries,
  isFormatErrorEntry,
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

const { entries, loading, error, total, entryErrors, isOverriding, resetEntries, loadEntries, setOverride } = useSubscriptionEntries()
const searchText = ref('')
const kindFilter = ref<EntryKind | 'all'>('all')
const page = ref(1)
const pageSize = 50
const kinds: EntryKind[] = ['proxy', 'notice', 'unknown', 'format_error']
const overrideKinds = ['proxy', 'notice', 'unknown', 'auto'] as const

function effectiveKind(entry: SubscriptionEntryDTO): EntryKind {
  return isFormatErrorEntry(entry) ? 'format_error' : entry.effective_kind
}

const counts = computed(() => {
  const result = { proxy: 0, notice: 0, unknown: 0, format_error: 0 }
  for (const entry of entries.value) result[effectiveKind(entry)]++
  return result
})
const filteredEntries = computed(() => {
  const query = searchText.value.trim().toLocaleLowerCase()
  return entries.value.filter((entry) => {
    if (kindFilter.value !== 'all' && effectiveKind(entry) !== kindFilter.value) return false
    if (!query) return true
    return [displayText(entry.name), displayText(entry.classification_reason), displayText(entry.override_reason)]
      .some((text) => text.toLocaleLowerCase().includes(query))
  })
})
const pageCount = computed(() => Math.max(1, Math.ceil(filteredEntries.value.length / pageSize)))
const visibleEntries = computed(() => filteredEntries.value.slice((page.value - 1) * pageSize, page.value * pageSize))
// Each entry is rendered once, including notices and immutable format diagnostics.
const sections = computed(() => [
  { id: 'subscription-format-errors', title: uiText('格式错误 · 仅展示，不参与订阅', 'Format errors · read-only, excluded from subscriptions'), items: visibleEntries.value.filter(isFormatErrorEntry) },
  { id: 'subscription-notices-section', title: t('subscriptions.noticesSectionTitle'), items: visibleEntries.value.filter((entry) => effectiveKind(entry) === 'notice') },
  { id: 'subscription-node-entries', title: uiText('节点与待识别条目', 'Nodes and unclassified entries'), items: visibleEntries.value.filter((entry) => effectiveKind(entry) === 'proxy' || effectiveKind(entry) === 'unknown') },
])
const hasPending = computed(() => entries.value.some((entry) => isOverriding(props.subscription?.id ?? '', entry.entry_id)))

watch(
  () => [props.modelValue, props.subscription?.id] as const,
  ([open, subId]) => {
    searchText.value = ''
    kindFilter.value = 'all'
    page.value = 1
    if (open && subId) void loadEntries(subId)
    else resetEntries()
  },
  { immediate: true }
)
watch([searchText, kindFilter], () => { page.value = 1 })
watch(pageCount, (count) => { page.value = Math.min(page.value, count) })
onUnmounted(resetEntries)

async function handleOverride(entry: SubscriptionEntryDTO, newKind: EntryKind | null) {
  if (!props.subscription?.id || loading.value || isFormatErrorEntry(entry)) return
  if ((entry.user_kind_override ?? null) === newKind) return
  try {
    await setOverride(props.subscription.id, entry.entry_id, newKind)
  } catch {
    // Keep the failed entry and its inline error available for retry.
  }
}

function entryKindLabel(kind: string): string {
  const labels: Record<string, string> = {
    proxy: uiText('节点', 'Node'),
    notice: uiText('公告', 'Notice'),
    unknown: uiText('未知', 'Unknown'),
    format_error: uiText('格式错误', 'Format error'),
    auto: uiText('自动', 'Auto'),
  }
  return labels[kind] || labels.unknown
}

function handleRefresh() {
  if (props.subscription?.id && !hasPending.value) void loadEntries(props.subscription.id)
}
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="subscription ? `${displayName(subscription.name, uiText('未命名订阅', 'Unnamed subscription'), subscription.id)} · ${t('subscriptions.entriesTitle')}` : t('subscriptions.entriesTitle')"
    :description="t('subscriptions.entriesSubtitle')"
    width-class="max-w-3xl"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="space-y-4 text-xs">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-2" aria-live="polite">
          <span class="font-semibold">{{ uiText(`共 ${total} 条`, `${total} entries`) }}</span>
          <span v-for="kind in kinds" :key="kind" class="text-base-content/65">
            {{ entryKindLabel(kind) }}: {{ counts[kind] }}
          </span>
        </div>
        <button type="button" class="btn btn-ghost btn-sm gap-1" :disabled="loading || hasPending" data-testid="entries-refresh-btn" @click="handleRefresh">
          <ArrowPathIcon class="h-4 w-4" :class="{ 'animate-spin': loading }" />
          {{ uiText('刷新条目列表', 'Reload entries') }}
        </button>
      </div>
      <p class="text-base-content/65">
        {{ uiText('这里只读取上次拉取的结果。需要获取新内容时，请在订阅源卡片选择“拉取来源”', 'This shows the last fetched result. Use “Fetch source” on the subscription card to retrieve new content') }}
      </p>
      <div class="flex flex-wrap gap-3">
        <label class="flex min-w-40 flex-1 flex-col gap-1" for="entry-search">
          {{ uiText('搜索条目', 'Search entries') }}
          <input id="entry-search" v-model="searchText" type="search" class="input input-sm w-full" :placeholder="uiText('名称、判定原因或覆盖理由', 'Name, classification or override reason')" data-testid="entries-search" />
        </label>
        <label class="flex flex-col gap-1" for="entry-kind-filter">
          {{ uiText('条目类型', 'Entry type') }}
          <select id="entry-kind-filter" v-model="kindFilter" class="select select-sm w-full" data-testid="entries-kind-filter">
            <option value="all">{{ uiText('全部类型', 'All types') }}</option>
            <option v-for="kind in kinds" :key="kind" :value="kind">{{ entryKindLabel(kind) }} ({{ counts[kind] }})</option>
          </select>
        </label>
      </div>

      <div v-if="loading" class="space-y-2" :aria-label="uiText('正在加载条目', 'Loading entries')">
        <div v-for="i in 3" :key="i" class="skeleton h-20 rounded-xl" />
      </div>
      <ErrorStateCard v-else-if="error" :error="displayText(error)" :retrying="loading" @retry="handleRefresh" />
      <EmptyState v-else-if="entries.length === 0" :icon="SparklesIcon" :title="t('subscriptions.noEntriesFound')" description="" />
      <EmptyState v-else-if="filteredEntries.length === 0" :icon="SparklesIcon" :title="uiText('没有匹配的条目', 'No matching entries')" :description="uiText('试试其他类型或搜索文本', 'Try a different type or search term')" />
      <div v-else class="space-y-4">
        <template v-for="section in sections" :key="section.id">
          <section v-if="section.items.length" :data-testid="section.id" class="space-y-2">
            <h3 class="font-semibold text-base-content/75">{{ section.title }}</h3>
            <p v-if="section.id === 'subscription-format-errors'" class="text-base-content/65">
              {{ uiText('这是最近一次拉取的格式诊断；上次有效节点仍单独展示，格式错误不能手动改成节点', 'These are diagnostics from the latest fetch. Previous valid nodes remain separate; format errors cannot be reclassified as nodes') }}
            </p>
            <article
              v-for="entry in section.items"
              :key="entry.entry_id"
              :data-testid="effectiveKind(entry) === 'notice' ? 'subscription-notice-item' : 'subscription-entry-item'"
              :data-entry-id="entry.entry_id"
              class="rounded-xl border border-base-300 bg-base-100 p-3 space-y-2 min-w-0"
              :class="{ 'border-warning/40': isFormatErrorEntry(entry), 'border-info/30': effectiveKind(entry) === 'notice' }"
              :aria-busy="isOverriding(subscription?.id ?? '', entry.entry_id) ? 'true' : undefined"
            >
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-base-content/50 font-mono">#{{ entry.ordinal }}</span>
                <strong class="min-w-0 max-w-full [overflow-wrap:anywhere]">{{ displayName(entry.name, uiText('未命名来源条目', 'Unnamed source entry'), entry.entry_id) }}</strong>
                <span class="badge badge-sm badge-soft" data-testid="entry-effective-kind">
                  {{ uiText('生效', 'Effective') }}: {{ entryKindLabel(effectiveKind(entry)) }}
                </span>
                <span v-if="entry.entry_kind !== effectiveKind(entry)" class="text-base-content/60">
                  {{ uiText('原始', 'Original') }}: {{ entryKindLabel(entry.entry_kind) }}
                </span>
                <span v-if="entry.user_kind_override" class="badge badge-sm badge-primary badge-outline">
                  {{ uiText('手动覆盖', 'Override') }}: {{ entryKindLabel(entry.user_kind_override) }}
                </span>
              </div>
              <p v-if="entry.classification_reason" class="text-base-content/70 whitespace-pre-wrap [overflow-wrap:anywhere]">
                {{ uiText('判定原因', 'Classification') }}: {{ displayText(entry.classification_reason) }}
              </p>
              <p v-if="entry.override_at || entry.override_reason" class="text-base-content/60 whitespace-pre-wrap [overflow-wrap:anywhere]">
                <span v-if="entry.override_at">{{ uiText('覆盖时间', 'Overridden at') }}: {{ formatFullDateTime(entry.override_at, currentLocale) }}</span>
                <span v-if="entry.override_reason"> · {{ uiText('理由', 'Reason') }}: {{ displayText(entry.override_reason) }}</span>
              </p>
              <div v-if="!isFormatErrorEntry(entry)" class="flex flex-wrap items-center gap-1" role="group" :aria-label="uiText('分类覆盖切换', 'Classification override')">
                <button
                  v-for="kind in overrideKinds"
                  :key="kind"
                  type="button"
                  class="btn btn-xs"
                  :class="(entry.user_kind_override ?? 'auto') === kind ? 'btn-neutral' : 'btn-ghost border-base-300'"
                  :aria-pressed="(entry.user_kind_override ?? 'auto') === kind"
                  :disabled="loading || isOverriding(subscription?.id ?? '', entry.entry_id)"
                  :data-testid="`${effectiveKind(entry) === 'notice' ? 'notice-' : ''}override-${kind}-btn`"
                  :title="kind === 'auto' ? uiText('清除覆盖，恢复自动识别', 'Clear override and restore automatic classification') : uiText(`设为${entryKindLabel(kind)}`, `Classify as ${entryKindLabel(kind)}`)"
                  @click="handleOverride(entry, kind === 'auto' ? null : kind)"
                >
                  {{ entryKindLabel(kind) }}
                </button>
                <span v-if="isOverriding(subscription?.id ?? '', entry.entry_id)" class="loading loading-spinner loading-xs" role="status" :aria-label="uiText('正在保存分类', 'Saving classification')" />
              </div>
              <p v-if="entryErrors[entry.entry_id]" role="alert" class="text-error whitespace-pre-wrap [overflow-wrap:anywhere]" data-testid="entry-override-error">{{ entryErrors[entry.entry_id] }}</p>
              <div v-if="entry.conflict" data-testid="entry-conflict-warning" class="flex items-start gap-2 text-error">
                <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
                <span class="whitespace-pre-wrap [overflow-wrap:anywhere]">{{ t('subscriptions.conflictWarning') }}: {{ displayText(entry.conflict) }}</span>
              </div>
            </article>
          </section>
        </template>
      </div>
      <nav v-if="!loading && !error && filteredEntries.length" class="flex flex-wrap items-center justify-between gap-2" :aria-label="uiText('来源条目分页', 'Entry pagination')">
        <span data-testid="entries-visible-count">{{ uiText(`匹配 ${filteredEntries.length} / ${total} 条 · 第 ${page} / ${pageCount} 页`, `${filteredEntries.length} / ${total} matching · page ${page} / ${pageCount}`) }}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-sm btn-ghost" :disabled="page <= 1" data-testid="entries-page-prev" @click="page--">{{ uiText('上一页', 'Previous') }}</button>
          <button type="button" class="btn btn-sm btn-ghost" :disabled="page >= pageCount" data-testid="entries-page-next" @click="page++">{{ uiText('下一页', 'Next') }}</button>
        </div>
      </nav>
    </div>
  </DrawerCard>
</template>

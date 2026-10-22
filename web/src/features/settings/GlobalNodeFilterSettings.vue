<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { usePolicy } from '../policy/usePolicy'
import StructuredFilterEditor from '../policy/StructuredFilterEditor.vue'
import {
  SUPPORTED_FILTER_FIELDS,
  SUPPORTED_PROTOCOLS,
  validateConditionInput,
  type FilterCondition,
  type FilterField,
  type FilterOp,
} from '../policy/policyTypes'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'

const kinds = ['baseline', 'geo', 'streaming', 'ai', 'speed', 'ip_risk'] as const
const verdicts = ['available', 'restricted', 'unknown', 'error', 'stale'] as const
const {
  globalFilter, error: requestError, loadingGlobalFilter, savingGlobalFilter,
  loadGlobalFilter, updateGlobalFilter,
} = usePolicy()
const conditions = ref<FilterCondition[]>([])
const loaded = ref(false)
const dirty = ref(false)
const validationError = ref('')
const filterInvalid = ref(false)

useUnsavedChanges(dirty, () =>
  uiText(
    '全局筛选条件有未保存的草稿修改，离开将丢失这些修改。确定要离开吗？',
    'Global filter conditions have unsaved changes. Are you sure you want to leave?'
  )
)

const isProbe = (field: FilterField) => field === 'probe_verdict' || field === 'probe_latency_ms'
const operators = (field: FilterField): FilterOp[] => field === 'probe_latency_ms'
  ? ['lte']
  : field === 'display_name' || field === 'source_subscription_ids'
    ? ['contains', 'not_contains']
    : ['equals', 'not_equals']

function changeField(condition: FilterCondition) {
  condition.op = operators(condition.field)[0]!
  condition.value = condition.field === 'probe_verdict' ? 'available' : ''
  if (isProbe(condition.field)) condition.probe_kind = 'baseline'
  else {
    delete condition.probe_kind
    delete condition.freshness_seconds
  }
  edited()
}

function edited() {
  dirty.value = true
  validationError.value = ''
}

function addCondition() {
  if (conditions.value.length >= 32) {
    validationError.value = t('settings.filterMaxConditions')
    return
  }
  conditions.value.push({ field: 'probe_verdict', op: 'equals', value: 'available', probe_kind: 'baseline' })
  edited()
}

function validate(condition: FilterCondition): string | null {
  const standard = validateConditionInput(condition)
  if (standard) return standard
  if (condition.field === 'display_name' && new TextEncoder().encode(condition.value ?? '').length > 255) {
    return t('settings.filterInvalidNameLength')
  }
  if (isProbe(condition.field)) {
    if (!kinds.includes(condition.probe_kind as typeof kinds[number])) return t('settings.filterInvalidKind')
    if (condition.field === 'probe_verdict' && !verdicts.includes(condition.value as typeof verdicts[number])) {
      return t('settings.filterInvalidVerdict')
    }
    if (condition.freshness_seconds !== undefined &&
      (!Number.isInteger(condition.freshness_seconds) || condition.freshness_seconds < 1 || condition.freshness_seconds > 604800)) {
      return t('settings.filterInvalidFreshness')
    }
    if (condition.field === 'probe_latency_ms' && !/^(0|[1-9]\d*)$/.test(condition.value ?? '')) {
      return t('settings.filterInvalidLatency')
    }
  } else if (condition.field !== 'platform_verdict' && (condition.probe_kind !== undefined || condition.freshness_seconds !== undefined)) {
    return t('settings.filterInvalidKind')
  }
  // The server requires a UUIDv7 source id. Reject invalid values before sending.
  if (condition.field === 'source_subscription_ids' &&
    !/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(condition.value ?? '')) {
    return t('settings.filterInvalidSource')
  }
  return null
}

async function refresh() {
  await loadGlobalFilter()
  if (requestError.value) return // Never interpret a failed read as an empty filter.
  conditions.value = (globalFilter.value?.spec?.conditions ?? []).map((condition) => ({ ...condition }))
  loaded.value = true
  dirty.value = false
  validationError.value = ''
}

async function save() {
  if (!loaded.value || savingGlobalFilter.value) return
  validationError.value = ''
  if (conditions.value.length > 32) {
    validationError.value = t('settings.filterMaxConditions')
    return
  }
  for (const condition of conditions.value) {
    const problem = validate(condition)
    if (problem) {
      validationError.value = problem
      return
    }
  }
  if (filterInvalid.value) {
    validationError.value = '存在未完成或无效的筛选条件，请完善或删除后再保存。'
    return
  }
  try {
    await updateGlobalFilter({ conditions: conditions.value.map((condition) => ({ ...condition })) })
    dirty.value = false
  } catch {
    // usePolicy exposes the server error; keep the edited conditions intact.
  }
}

// A failed load must not overwrite an in-progress edit. Refresh is an explicit discard action.
onMounted(refresh)
</script>

<template>
  <section class="card bg-base-200/80 backdrop-blur-xl shadow-sm border border-white/10" data-testid="global-filter-settings">
    <div class="card-body p-5 sm:p-6 space-y-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-base sm:text-lg font-bold">{{ t('settings.filterTitle') }}</h3>
          <p class="text-sm text-base-content/70 mt-1">{{ t('settings.filterDescription') }}</p>
        </div>
        <button type="button" class="btn btn-outline btn-sm" data-testid="filter-refresh" :disabled="loadingGlobalFilter || savingGlobalFilter" @click="refresh">
          {{ t('common.refresh') }}
        </button>
      </div>
      <p class="text-xs text-base-content/70 leading-relaxed">{{ t('settings.filterExplanation') }}</p>
      <p class="text-xs text-base-content/70 leading-relaxed">
        说明：全局软筛选在直接导出与预览时即时生效；WARP 或专用免检线路等策略组可关闭「继承全局筛选」以软豁免并使用全部候选节点(不应用全局筛选)。当前凭据版本门槛、节点停用、来源订阅禁用与显式风险排除始终保留生效，不可绕过。
      </p>
      <p v-if="dirty" class="text-warning text-xs">{{ t('settings.filterUnsaved') }}</p>
      <p v-if="requestError" role="alert" class="text-error text-sm" data-testid="filter-request-error">{{ requestError }}</p>
      <p v-if="validationError" role="alert" class="text-error text-sm" data-testid="filter-validation-error">{{ validationError }}</p>
      <p v-if="loadingGlobalFilter" role="status">{{ t('settings.filterLoading') }}</p>
      <template v-if="loaded">
        <StructuredFilterEditor
          v-model="conditions"
          :max-conditions="32"
          :allow-clear="true"
          :inline="true"
          @change="edited"
          @invalid="filterInvalid = $event"
        />
        <div class="flex flex-wrap gap-2 pt-2 border-t border-base-300/40">
          <button type="button" class="btn btn-primary btn-sm" data-testid="filter-save" :disabled="savingGlobalFilter || loadingGlobalFilter" @click="save">{{ savingGlobalFilter ? t('settings.filterSaving') : t('settings.filterSave') }}</button>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { usePolicy } from '../policy/usePolicy'
import {
  SUPPORTED_FILTER_FIELDS,
  SUPPORTED_PROTOCOLS,
  validateConditionInput,
  type FilterCondition,
  type FilterField,
  type FilterOp,
} from '../policy/policyTypes'
import { t } from '../../locales'

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
  } else if (condition.probe_kind !== undefined || condition.freshness_seconds !== undefined) {
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
      <p class="text-xs text-base-content/70 leading-relaxed">{{ t('settings.filterScope') }}</p>
      <p v-if="dirty" class="text-warning text-xs">{{ t('settings.filterUnsaved') }}</p>
      <p v-if="requestError" role="alert" class="text-error text-sm" data-testid="filter-request-error">{{ requestError }}</p>
      <p v-if="validationError" role="alert" class="text-error text-sm" data-testid="filter-validation-error">{{ validationError }}</p>
      <p v-if="loadingGlobalFilter" role="status">{{ t('settings.filterLoading') }}</p>
      <template v-if="loaded">
        <p v-if="conditions.length === 0" data-testid="filter-empty" class="text-sm text-base-content/60">{{ t('settings.filterEmpty') }}</p>
        <p v-else class="text-xs text-base-content/70">{{ t('settings.filterAnd') }}</p>
        <div v-for="(condition, index) in conditions" :key="index" class="rounded-xl bg-base-100/70 border border-base-300 p-3 space-y-3" data-testid="filter-condition">
          <div class="flex flex-wrap gap-2 items-center">
            <label class="text-xs font-medium">{{ t('settings.filterField') }}
              <select v-model="condition.field" class="select select-bordered select-sm block" :aria-label="t('settings.filterField')" @change="changeField(condition)">
                <option v-for="item in SUPPORTED_FILTER_FIELDS" :key="item.field" :value="item.field">{{ t(`settings.filterField_${item.field}`) }}</option>
              </select>
            </label>
            <label class="text-xs font-medium">{{ t('settings.filterOperator') }}
              <select v-model="condition.op" class="select select-bordered select-sm block" :aria-label="t('settings.filterOperator')" @change="edited">
                <option v-for="op in operators(condition.field)" :key="op" :value="op">{{ t(`settings.filterOp_${op}`) }}</option>
              </select>
            </label>
            <label v-if="isProbe(condition.field)" class="text-xs font-medium">{{ t('settings.filterKind') }}
              <select v-model="condition.probe_kind" class="select select-bordered select-sm block" :aria-label="t('settings.filterKind')" @change="edited">
                <option v-for="kind in kinds" :key="kind" :value="kind">{{ t(`settings.filterKind_${kind}`) }}</option>
              </select>
            </label>
            <label class="text-xs font-medium">{{ t('settings.filterValue') }}
              <select v-if="condition.field === 'probe_verdict'" v-model="condition.value" class="select select-bordered select-sm block" :aria-label="t('settings.filterValue')" @change="edited">
                <option v-for="verdict in verdicts" :key="verdict" :value="verdict">{{ t(`settings.filterVerdict_${verdict}`) }}</option>
              </select>
              <select v-else-if="condition.field === 'protocol'" v-model="condition.value" class="select select-bordered select-sm block" :aria-label="t('settings.filterValue')" @change="edited">
                <option value="" disabled>{{ t('settings.filterSelect') }}</option>
                <option v-for="protocol in SUPPORTED_PROTOCOLS" :key="protocol" :value="protocol">{{ protocol }}</option>
              </select>
              <input v-else v-model="condition.value" class="input input-bordered input-sm block max-w-full" :aria-label="t('settings.filterValue')" @input="edited" />
            </label>
            <label v-if="isProbe(condition.field)" class="text-xs font-medium">{{ t('settings.filterFreshness') }}
              <input :value="condition.freshness_seconds ?? ''" type="number" min="1" max="604800" step="1" class="input input-bordered input-sm block max-w-full" :aria-label="t('settings.filterFreshness')" :placeholder="t('settings.filterDefaultFreshness')" @input="condition.freshness_seconds = ($event.target as HTMLInputElement).value === '' ? undefined : Number(($event.target as HTMLInputElement).value); edited()" />
            </label>
            <button type="button" class="btn btn-ghost btn-sm text-error self-end" :aria-label="t('settings.filterRemove')" @click="conditions.splice(index, 1); edited()">{{ t('settings.filterRemove') }}</button>
          </div>
        </div>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-outline btn-sm" data-testid="filter-add" :disabled="conditions.length >= 32" @click="addCondition">{{ t('settings.filterAdd') }}</button>
          <button type="button" class="btn btn-ghost btn-sm" data-testid="filter-clear" :disabled="conditions.length === 0" @click="conditions = []; edited()">{{ t('settings.filterClear') }}</button>
          <button type="button" class="btn btn-primary btn-sm" data-testid="filter-save" :disabled="savingGlobalFilter || loadingGlobalFilter" @click="save">{{ savingGlobalFilter ? t('settings.filterSaving') : t('settings.filterSave') }}</button>
        </div>
      </template>
    </div>
  </section>
</template>

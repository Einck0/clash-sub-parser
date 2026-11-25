<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, ApiError } from '../../api/client'
import StructuredFilterEditor from '../policy/StructuredFilterEditor.vue'
import {
  type GlobalNodeFilter,
  validateConditionInput,
  SUPPORTED_PROBE_KINDS,
  type FilterCondition,
  type FilterField,
  type FilterOp,
} from '../policy/policyTypes'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { useServerSettings } from './useServerSettings'
import { displayText } from '../../utils/displayNames'

const emit = defineEmits<{ saved: [] }>()
const conditions = ref<FilterCondition[]>([])
const emptyFallbackPass = ref(true)
const validationError = ref('')
const filterInvalid = ref(false)
const state = useServerSettings<GlobalNodeFilter>({
  read: async () => {
    try {
      return await api.get<GlobalNodeFilter>('/api/v1/policies/global-node-filter')
    } catch (err) {
      // A missing optional filter is defined as no conditions, not a failed read.
      if (err instanceof ApiError && err.status === 404) return { spec: { conditions: [] }, empty_fallback_pass: true, updated_at: '' }
      throw err
    }
  },
  apply: value => {
    if (!value || !value.spec || !Array.isArray(value.spec.conditions) ||
      value.spec.conditions.some(condition => !condition || typeof condition.field !== 'string' || typeof condition.op !== 'string') ||
      (value.empty_fallback_pass !== undefined && typeof value.empty_fallback_pass !== 'boolean')) {
      throw new Error(uiText('服务端返回的全局筛选配置无效', 'The server returned invalid global filter settings.'))
    }
    conditions.value = value.spec.conditions.map(condition => ({ ...condition }))
    emptyFallbackPass.value = value.empty_fallback_pass ?? true
  },
  snapshot: () => ({ conditions: conditions.value, emptyFallbackPass: emptyFallbackPass.value }),
})
const { loaded, dirty, loading, saving, readFailed, error } = state
defineExpose({ dirty, busy: state.busy })

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
    // Verdict validation and the supported kind list come from the shared policy contract.
    if (!(SUPPORTED_PROBE_KINDS as readonly string[]).includes(condition.probe_kind || '')) return t('settings.filterInvalidKind')
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
  if (await state.load()) validationError.value = ''
}

async function save() {
  if (!loaded.value || saving.value || loading.value) return
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
    validationError.value = uiText('存在未完成或无效的筛选条件，请完善或删除后再保存。', 'Complete or remove invalid filter conditions before saving.')
    return
  }
  if (await state.save(() => api.put<GlobalNodeFilter>('/api/v1/policies/global-node-filter', {
    spec: { conditions: conditions.value.map(condition => ({ ...condition })) },
    empty_fallback_pass: emptyFallbackPass.value,
  }))) emit('saved')
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
          <p class="text-sm text-base-content/70 mt-1">{{ uiText('与策略页共用一份全局条件，按 AND（全部满足）筛选。空条件不做额外软筛选。', 'Shared with Policy. All conditions must match (AND); empty conditions add no soft filtering.') }}</p>
        </div>
        <button type="button" class="btn btn-outline btn-sm" data-testid="filter-refresh" :disabled="loading || saving" @click="refresh">
          {{ t('common.refresh') }}
        </button>
      </div>
      <p class="text-xs text-base-content/70 leading-relaxed">{{ uiText('执行优先级：节点停用与显式风险排除 → 全局节点软筛选 → 策略组有序来源流水线。留空条件时允许所有符合启用状态与风险限制的候选节点。', 'Order: node disabling and explicit risk exclusions → global soft filtering → ordered group sources. Empty conditions allow all candidates that meet enablement and risk constraints.') }}</p>
      <p class="text-xs text-base-content/70 leading-relaxed">{{ t('settings.filterExplanation') }}</p>
      <p class="text-xs text-base-content/70 leading-relaxed">
        说明：全局软筛选在直接导出与预览时即时生效；WARP 或专用免检线路等策略组可关闭「继承全局筛选」以软豁免并使用全部候选节点(不应用全局筛选)。当前凭据版本门槛、节点停用、来源订阅禁用与显式风险排除始终保留生效，不可绕过。
      </p>
      <p v-if="dirty" class="text-warning text-xs">{{ t('settings.filterUnsaved') }}</p>
      <p v-if="error" role="alert" class="text-error text-sm whitespace-pre-wrap [overflow-wrap:anywhere]" data-testid="filter-request-error">{{ displayText(error) }}</p>
      <p v-if="validationError" role="alert" class="text-error text-sm whitespace-pre-wrap [overflow-wrap:anywhere]" data-testid="filter-validation-error">{{ displayText(validationError) }}</p>
      <p v-if="loading && !loaded" role="status">{{ t('settings.filterLoading') }}</p>
      <fieldset v-if="loaded" :disabled="loading || saving || readFailed" class="space-y-4 min-w-0">
        <label class="flex items-start gap-3 cursor-pointer">
          <input v-model="emptyFallbackPass" type="checkbox" class="checkbox checkbox-primary checkbox-sm" data-testid="global-empty-pass" @change="edited" />
          <span><span class="font-semibold block">{{ uiText("空策略组使用 PASS", "Use PASS for empty groups") }}</span><span class="text-xs opacity-70">{{ uiText("对所有策略组生效；非空组不受影响。空组使用 PASS 继续匹配后续规则，并非直连。", "Applies to all policy groups; non-empty groups are unchanged. PASS continues matching later rules, rather than connecting directly.") }}</span></span>
        </label>
        <StructuredFilterEditor
          v-model="conditions"
          :max-conditions="32"
          :allow-clear="true"
          :inline="true"
          @change="edited"
          @invalid="filterInvalid = $event"
        />
        <div class="flex flex-wrap gap-2 pt-2 border-t border-base-300/40">
          <button type="button" class="btn btn-primary btn-sm" data-testid="filter-save" :disabled="saving || loading" @click="save">{{ saving ? t('settings.filterSaving') : t('settings.filterSave') }}</button>
        </div>
      </fieldset>
    </div>
  </section>
</template>

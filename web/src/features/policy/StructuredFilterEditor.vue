<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  SUPPORTED_FILTER_FIELDS,
  SUPPORTED_PROTOCOLS,
  SUPPORTED_PROBE_KINDS,
  SUPPORTED_PROBE_VERDICTS,
  SUPPORTED_PLATFORM_VERDICTS,
  SUPPORTED_AI_PLATFORMS,
  SUPPORTED_STREAMING_PLATFORMS,
  filterFieldOperators,
  validateConditionInput,
  type FilterCondition,
  type FilterField,
  type FilterOp,
} from './policyTypes'
import { api } from '../../api/client'
import { t } from '../../locales'

interface Props {
  modelValue?: FilterCondition[]
  conditions?: FilterCondition[]
  compact?: boolean
  maxConditions?: number
  allowClear?: boolean
  title?: string
  description?: string
  readOnly?: boolean
  inline?: boolean
  allowedFields?: FilterField[]
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: undefined,
  conditions: undefined,
  compact: false,
  maxConditions: 32,
  allowClear: true,
  title: '',
  description: '',
  readOnly: false,
  inline: false,
  allowedFields: undefined,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: FilterCondition[]): void
  (e: 'update:conditions', value: FilterCondition[]): void
  (e: 'change', value: FilterCondition[]): void
  (e: 'draft-change', hasDraft: boolean): void
  (e: 'invalid', isInvalid: boolean): void
  (e: 'row-invalid', index: number, isInvalid: boolean): void
}>()

const activeConditions = ref<FilterCondition[]>([])

// Subscriptions list fetching for source_subscription_ids
interface SubscriptionSummary {
  id: string
  name: string
}
const subscriptions = ref<SubscriptionSummary[]>([])
const loadingSubscriptions = ref(false)
const subscriptionsError = ref('')

async function fetchSubscriptions() {
  loadingSubscriptions.value = true
  subscriptionsError.value = ''
  try {
    const allSubs: SubscriptionSummary[] = []
    let page = 1
    const pageSize = 100
    while (true) {
      const res = await api.get<{
        items?: Array<{ id: string; name?: string; title?: string }>
        total?: number
      }>('/api/v1/subscriptions', {
        params: { page, page_size: pageSize },
      })
      if (res && Array.isArray(res.items)) {
        res.items.forEach((s) => {
          allSubs.push({
            id: s.id,
            name: s.name?.trim() || (s as any).title?.trim() || s.id,
          })
        })
        if (res.items.length === 0 || allSubs.length >= (res.total || 0) || res.items.length < pageSize) {
          break
        }
        page++
      } else {
        break
      }
    }
    subscriptions.value = allSubs
  } catch (err: any) {
    subscriptionsError.value = '加载订阅失败: ' + (err?.message || '网络异常')
  } finally {
    loadingSubscriptions.value = false
  }
}

function ensureSubscriptions() {
  if (subscriptions.value.length === 0 && !loadingSubscriptions.value && !subscriptionsError.value) {
    fetchSubscriptions()
  }
}

onMounted(() => {
  if (activeConditions.value.some((c) => c.field === 'source_subscription_ids')) {
    ensureSubscriptions()
  }
})

// Sync props -> activeConditions
watch(
  () => props.modelValue ?? props.conditions ?? [],
  (next) => {
    const nextList = next || []
    if (JSON.stringify(nextList) !== JSON.stringify(activeConditions.value)) {
      activeConditions.value = nextList.map((c) => ({ ...c }))
      checkValidityAndEmitDraft()
    }
  },
  { immediate: true, deep: true }
)

const availableFilterFields = computed(() => {
  if (props.allowedFields && props.allowedFields.length > 0) {
    return SUPPORTED_FILTER_FIELDS.filter((f) => props.allowedFields!.includes(f.field))
  }
  const defaultFields: FilterField[] = [
    'health_status',
    'exit_region',
    'platform_verdict',
    'display_name',
    'protocol',
    'source_subscription_ids',
    'probe_latency_ms',
  ]
  return SUPPORTED_FILTER_FIELDS.filter((f) => defaultFields.includes(f.field))
})

function getRowAvailableFields(currentField?: FilterField) {
  if (currentField === 'probe_verdict') {
    const hasIt = availableFilterFields.value.some((f) => f.field === 'probe_verdict')
    if (!hasIt) {
      const probeItem = SUPPORTED_FILTER_FIELDS.find((f) => f.field === 'probe_verdict')
      if (probeItem) {
        return [...availableFilterFields.value, probeItem]
      }
    }
  }
  return availableFilterFields.value
}

function isConditionProbe(field: FilterField) {
  return field === 'probe_verdict' || field === 'probe_latency_ms'
}

function conditionInvalid(cond: FilterCondition): boolean {
  if (!cond.field || !cond.op) return true
  if (cond.value === undefined || (typeof cond.value === 'string' && cond.value.trim() === '')) return true
  return validateConditionInput(cond) !== null
}

function checkValidityAndEmitDraft() {
  const invalidRows = activeConditions.value.map(conditionInvalid)
  invalidRows.forEach((invalid, index) => emit('row-invalid', index, invalid))
  const hasInvalid = invalidRows.some(Boolean)
  emit('draft-change', hasInvalid)
  emit('invalid', hasInvalid)
}

function emitConditions() {
  const cloned = activeConditions.value.map((c) => ({ ...c }))
  emit('update:modelValue', cloned)
  emit('update:conditions', cloned)
  emit('change', cloned)
  checkValidityAndEmitDraft()
}

function onInlineRowFieldChange(cond: FilterCondition) {
  const ops = filterFieldOperators(cond.field)
  if (!ops.includes(cond.op)) {
    cond.op = ops[0]!
  }
  delete cond.platform
  if (!isConditionProbe(cond.field)) {
    delete cond.probe_kind
    delete cond.freshness_seconds
  } else {
    cond.probe_kind = 'baseline'
  }

  if (cond.field === 'source_subscription_ids') {
    ensureSubscriptions()
  }

  if (cond.field === 'health_status') {
    cond.value = 'healthy'
  } else if (cond.field === 'exit_region') {
    cond.value = 'HK'
  } else if (cond.field === 'platform_verdict') {
    cond.probe_kind = 'ai'
    cond.platform = 'openai'
    cond.value = 'available'
  } else if (cond.field === 'probe_verdict') {
    cond.probe_kind = 'baseline'
    cond.value = 'available'
  } else if (cond.field === 'protocol') {
    cond.value = 'ss'
  } else if (cond.field === 'probe_latency_ms') {
    cond.probe_kind = 'baseline'
    cond.op = 'lte'
    cond.value = ''
  } else {
    cond.value = ''
  }
  emitConditions()
}

function addDirectCondition() {
  if (activeConditions.value.length >= props.maxConditions) return
  const first = availableFilterFields.value[0]?.field || 'health_status'
  let defaultVal = 'healthy'
  let defaultOp: FilterOp = 'equals'
  if (first === 'health_status') {
    defaultVal = 'healthy'
    activeConditions.value.push({ field: 'health_status', op: 'equals', value: defaultVal })
  } else if (first === 'exit_region') {
    defaultVal = 'HK'
    activeConditions.value.push({ field: 'exit_region', op: 'equals', value: defaultVal })
  } else if (first === 'platform_verdict') {
    activeConditions.value.push({
      field: 'platform_verdict',
      op: 'equals',
      value: 'available',
      probe_kind: 'ai',
      platform: 'openai',
    })
  } else if (first === 'protocol') {
    activeConditions.value.push({ field: 'protocol', op: 'equals', value: 'ss' })
  } else if (first === 'probe_latency_ms') {
    activeConditions.value.push({ field: 'probe_latency_ms', op: 'lte', value: '200', probe_kind: 'baseline' })
  } else {
    activeConditions.value.push({ field: first, op: defaultOp, value: defaultVal })
  }
  emitConditions()
}

function removeCondition(index: number) {
  activeConditions.value.splice(index, 1)
  emitConditions()
}

function clearAllConditions() {
  activeConditions.value = []
  emitConditions()
}

function opLabel(op: FilterOp): string {
  switch (op) {
    case 'equals': return '等于 (equals)'
    case 'not_equals': return '不等于 (not_equals)'
    case 'contains': return '包含 (contains)'
    case 'not_contains': return '不包含 (not_contains)'
    case 'regex': return '正则匹配 (regex)'
    case 'not_regex': return '不匹配正则 (not_regex)'
    case 'lte': return '小于等于 (<=)'
    default: return op
  }
}
</script>

<template>
  <div class="space-y-3 structured-filter-editor" data-testid="structured-filter-editor">
    <!-- Optional Title and Description for standard mode / named section -->
    <div v-if="title || description" class="flex flex-wrap items-start justify-between gap-2">
      <div>
        <h4 v-if="title" class="font-bold text-xs sm:text-sm">{{ title }}</h4>
        <p v-if="description" class="text-[11px] opacity-70 mt-0.5">{{ description }}</p>
      </div>
      <span class="badge badge-xs badge-info font-mono">{{ activeConditions.length }}/{{ maxConditions }} 条条件</span>
    </div>

    <!-- Empty conditions state -->
    <div v-if="activeConditions.length === 0" class="p-3 rounded-lg bg-base-100/60 border border-base-300/80 text-xs text-base-content/60 space-y-1">
      <div class="flex items-center justify-between">
        <span class="font-medium" data-testid="filter-empty">无额外筛选条件 (匹配全部候选节点)</span>
        <span class="badge badge-xs badge-ghost">清空状态</span>
      </div>
      <p class="text-[11px] opacity-75">
        当前无过滤条件，表示不进行任何额外属性过滤；系统绝不偷偷附加健康或可用性限制条件。
      </p>
    </div>
    <p v-else class="text-xs text-base-content/70">
      {{ t('settings.filterAnd') }}
    </p>

    <!-- Conditions List (Direct Inline Editing) -->
    <div
      v-for="(condition, index) in activeConditions"
      :key="index"
      class="rounded-xl bg-base-100/70 border border-base-300 p-3 space-y-3"
      data-testid="filter-condition"
    >
      <div class="flex flex-wrap gap-2 items-end">
        <label class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterField') }}</span>
          <select
            v-model="condition.field"
            data-testid="condition-field"
            class="select select-sm"
            :aria-label="t('settings.filterField')"
            :disabled="readOnly"
            @change="onInlineRowFieldChange(condition)"
          >
            <option v-for="item in getRowAvailableFields(condition.field)" :key="item.field" :value="item.field">
              {{ t(`settings.filterField_${item.field}`) || item.label }}
            </option>
          </select>
        </label>

        <label class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterOperator') }}</span>
          <select
            v-model="condition.op"
            data-testid="condition-op"
            class="select select-sm font-mono"
            :aria-label="t('settings.filterOperator')"
            :disabled="readOnly"
            @change="emitConditions"
          >
            <option v-for="op in filterFieldOperators(condition.field)" :key="op" :value="op">
              {{ t(`settings.filterOp_${op}`) || opLabel(op) }}
            </option>
          </select>
        </label>

        <!-- Probe kind for probe fields -->
        <label v-if="isConditionProbe(condition.field)" class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterKind') }}</span>
          <select
            v-model="condition.probe_kind"
            class="select select-sm font-mono"
            :aria-label="t('settings.filterKind')"
            :disabled="readOnly"
            @change="emitConditions"
          >
            <option v-for="kind in SUPPORTED_PROBE_KINDS" :key="kind" :value="kind">
              {{ t(`settings.filterKind_${kind}`) || kind }}
            </option>
          </select>
        </label>

        <!-- Health Status Value -->
        <label v-if="condition.field === 'health_status'" class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterValue') }}</span>
          <select
            v-model="condition.value"
            data-testid="condition-value"
            class="select select-sm font-mono"
            :aria-label="t('settings.filterValue')"
            :disabled="readOnly"
            @change="emitConditions"
          >
            <option value="healthy">healthy</option>
            <option value="unhealthy">unhealthy</option>
            <option value="untested">untested</option>
          </select>
        </label>

        <!-- Exit Region Value -->
        <label v-else-if="condition.field === 'exit_region'" class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterValue') }}</span>
          <input
            v-model="condition.value"
            data-testid="condition-value"
            class="input input-sm font-mono uppercase"
            :aria-label="t('settings.filterValue')"
            placeholder="如 HK, JP..."
            maxlength="2"
            :disabled="readOnly"
            @input="condition.value = (condition.value || '').toUpperCase(); emitConditions()"
          />
        </label>

        <!-- Platform Verdict -->
        <template v-else-if="condition.field === 'platform_verdict'">
          <label class="flex flex-col gap-1 text-xs font-medium">
            <span>平台类型</span>
            <select
              v-model="condition.probe_kind"
              class="select select-sm font-mono"
              aria-label="平台类型"
              :disabled="readOnly"
              @change="condition.platform = condition.probe_kind === 'ai' ? 'openai' : 'netflix'; emitConditions()"
            >
              <option value="ai">ai</option>
              <option value="streaming">streaming</option>
            </select>
          </label>
          <label class="flex flex-col gap-1 text-xs font-medium">
            <span>服务平台</span>
            <select
              v-model="condition.platform"
              class="select select-sm font-mono"
              aria-label="服务平台"
              :disabled="readOnly"
              @change="emitConditions"
            >
              <template v-if="condition.probe_kind === 'ai'">
                <option v-for="p in SUPPORTED_AI_PLATFORMS" :key="p" :value="p">{{ p }}</option>
              </template>
              <template v-else>
                <option v-for="p in SUPPORTED_STREAMING_PLATFORMS" :key="p" :value="p">{{ p }}</option>
              </template>
            </select>
          </label>
          <label class="flex flex-col gap-1 text-xs font-medium">
            <span>{{ t('settings.filterValue') }}</span>
            <select
              v-model="condition.value"
              data-testid="condition-value"
              class="select select-sm font-mono"
              :aria-label="t('settings.filterValue')"
              :disabled="readOnly"
              @change="emitConditions"
            >
              <option value="available">available</option>
              <option value="restricted">restricted</option>
              <option value="unknown">unknown</option>
              <option value="error">error</option>
            </select>
          </label>
        </template>

        <!-- Protocol -->
        <label v-else-if="condition.field === 'protocol'" class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterValue') }}</span>
          <select
            v-model="condition.value"
            data-testid="condition-value"
            class="select select-sm font-mono"
            :aria-label="t('settings.filterValue')"
            :disabled="readOnly"
            @change="emitConditions"
          >
            <option value="" disabled>{{ t('settings.filterSelect') }}</option>
            <option v-for="proto in SUPPORTED_PROTOCOLS" :key="proto" :value="proto">{{ proto }}</option>
          </select>
        </label>

        <!-- Probe Verdict (Legacy) -->
        <label v-else-if="condition.field === 'probe_verdict'" class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterValue') }}</span>
          <select
            v-model="condition.value"
            data-testid="condition-value"
            class="select select-sm font-mono"
            :aria-label="t('settings.filterValue')"
            :disabled="readOnly"
            @change="emitConditions"
          >
            <option v-for="v in SUPPORTED_PROBE_VERDICTS" :key="v" :value="v">
              {{ t(`settings.filterVerdict_${v}`) || v }}
            </option>
          </select>
        </label>

        <!-- Probe Latency -->
        <label v-else-if="condition.field === 'probe_latency_ms'" class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterValue') }}</span>
          <input
            v-model="condition.value"
            data-testid="condition-value"
            class="input input-sm font-mono max-w-full"
            :aria-label="t('settings.filterValue')"
            placeholder="毫秒 (例如: 200)"
            :disabled="readOnly"
            @input="emitConditions"
          />
        </label>

        <!-- Source Subscription IDs -->
        <label v-else-if="condition.field === 'source_subscription_ids'" class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterValue') }}</span>
          <input
            v-model="condition.value"
            data-testid="condition-value"
            class="input input-sm font-mono max-w-full"
            :aria-label="t('settings.filterValue')"
            placeholder="订阅 UUIDv7..."
            :disabled="readOnly"
            @input="emitConditions"
          />
        </label>

        <!-- Display Name (Default) -->
        <label v-else class="flex flex-col gap-1 text-xs font-medium">
          <span>{{ t('settings.filterValue') }}</span>
          <input
            v-model="condition.value"
            data-testid="condition-value"
            class="input input-sm font-mono max-w-full"
            :aria-label="t('settings.filterValue')"
            :disabled="readOnly"
            @input="emitConditions"
          />
        </label>

        <button
          v-if="!readOnly"
          type="button"
          class="btn btn-ghost btn-sm text-error self-end"
          data-testid="filter-remove"
          :aria-label="t('settings.filterRemove')"
          @click="removeCondition(index)"
        >
          {{ t('settings.filterRemove') }}
        </button>
      </div>
    </div>

    <!-- Actions: Add and Clear -->
    <div v-if="!readOnly" class="flex flex-wrap gap-2">
      <button
        type="button"
        class="btn btn-outline btn-sm"
        data-testid="filter-add"
        :disabled="activeConditions.length >= maxConditions"
        @click="addDirectCondition"
      >
        {{ t('settings.filterAdd') }}
      </button>
      <button
        v-if="allowClear"
        type="button"
        class="btn btn-ghost btn-sm"
        data-testid="filter-clear"
        :disabled="activeConditions.length === 0"
        @click="clearAllConditions"
      >
        {{ t('settings.filterClear') }}
      </button>
    </div>
  </div>
</template>

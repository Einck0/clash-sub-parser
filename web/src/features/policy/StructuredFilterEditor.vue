<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  PlusIcon,
  TrashIcon,
} from '@heroicons/vue/24/outline'
import {
  SUPPORTED_FILTER_FIELDS,
  SUPPORTED_PROTOCOLS,
  SUPPORTED_PROBE_KINDS,
  SUPPORTED_PROBE_VERDICTS,
  filterFieldOperators,
  validateConditionInput,
  type FilterCondition,
  type FilterField,
  type FilterOp,
  type ProbeKind,
  type ProbeVerdict,
} from './policyTypes'
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
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: FilterCondition[]): void
  (e: 'update:conditions', value: FilterCondition[]): void
  (e: 'change', value: FilterCondition[]): void
  (e: 'draft-change', hasDraft: boolean): void
}>()

const activeConditions = ref<FilterCondition[]>([])

// Draft state for new or editing condition
const newField = ref<FilterField>('display_name')
const newOp = ref<FilterOp>('contains')
const newValue = ref('')
const newProbeKind = ref<ProbeKind>('baseline')
const newFreshnessSeconds = ref<number | undefined>(undefined)
const validationError = ref('')
const editingIndex = ref<number | null>(null)

const isProbe = computed(() => newField.value === 'probe_verdict' || newField.value === 'probe_latency_ms')

watch(newValue, (v) => {
  emit('draft-change', Boolean(v && v.trim()))
})

watch(
  () => props.modelValue ?? props.conditions ?? [],
  (next) => {
    activeConditions.value = (next || []).map((c) => ({ ...c }))
  },
  { immediate: true, deep: true }
)

function emitConditions(list: FilterCondition[]) {
  const cloned = list.map((c) => ({ ...c }))
  emit('update:modelValue', cloned)
  emit('update:conditions', cloned)
  emit('change', cloned)
}

function onFieldChange() {
  const validOps = filterFieldOperators(newField.value)
  if (!validOps.includes(newOp.value)) {
    newOp.value = validOps[0]!
  }
  if (newField.value === 'probe_verdict') {
    newValue.value = 'available'
    newProbeKind.value = 'baseline'
  } else if (newField.value === 'protocol') {
    newValue.value = 'ss'
    delete (newFreshnessSeconds as any).value
  } else {
    newValue.value = ''
  }
  validationError.value = ''
}

function resetDraft() {
  editingIndex.value = null
  newField.value = 'display_name'
  newOp.value = 'contains'
  newValue.value = ''
  newProbeKind.value = 'baseline'
  newFreshnessSeconds.value = undefined
  validationError.value = ''
}

function validate(cond: FilterCondition): string | null {
  const err = validateConditionInput(cond)
  if (err) return err

  if (cond.field === 'display_name' && new TextEncoder().encode(cond.value ?? '').length > (cond.op === 'regex' || cond.op === 'not_regex' ? 1024 : 255)) {
    return t('settings.filterInvalidNameLength') || '显示名称长度超出允许上限'
  }
  if (isProbe.value) {
    if (!cond.probe_kind || !SUPPORTED_PROBE_KINDS.includes(cond.probe_kind)) {
      return t('settings.filterInvalidKind') || '必须选择有效探针类型'
    }
    if (cond.field === 'probe_verdict' && (!cond.value || !SUPPORTED_PROBE_VERDICTS.includes(cond.value as ProbeVerdict))) {
      return t('settings.filterInvalidVerdict') || '必须选择有效探针判定结果'
    }
    if (cond.field === 'probe_latency_ms' && !/^(0|[1-9]\d*)$/.test(cond.value ?? '')) {
      return t('settings.filterInvalidLatency') || '探针延迟必须为有效非负整数毫秒值'
    }
    if (cond.freshness_seconds !== undefined &&
      (!Number.isInteger(cond.freshness_seconds) || cond.freshness_seconds < 1 || cond.freshness_seconds > 604800)) {
      return t('settings.filterInvalidFreshness') || '探针数据有效期必须在 1 到 604800 秒之间'
    }
  }
  if (cond.field === 'source_subscription_ids' &&
    !/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(cond.value ?? '')) {
    return t('settings.filterInvalidSource') || '来源订阅 ID 必须是标准 UUIDv7 格式'
  }
  return null
}

function addOrUpdateCondition() {
  validationError.value = ''
  const cond: FilterCondition = {
    field: newField.value,
    op: newOp.value,
    value: newValue.value.trim(),
  }
  if (isProbe.value) {
    cond.probe_kind = newProbeKind.value
    if (newFreshnessSeconds.value !== undefined && !isNaN(newFreshnessSeconds.value)) {
      cond.freshness_seconds = Number(newFreshnessSeconds.value)
    }
  }

  const err = validate(cond)
  if (err) {
    validationError.value = err
    return
  }

  if (editingIndex.value !== null && editingIndex.value >= 0 && editingIndex.value < activeConditions.value.length) {
    activeConditions.value[editingIndex.value] = cond
  } else {
    if (activeConditions.value.length >= props.maxConditions) {
      validationError.value = `最多允许添加 ${props.maxConditions} 条过滤条件`
      return
    }
    activeConditions.value.push(cond)
  }

  emitConditions(activeConditions.value)
  resetDraft()
}

function startEditCondition(index: number) {
  const cond = activeConditions.value[index]
  if (!cond) return
  editingIndex.value = index
  newField.value = cond.field
  newOp.value = cond.op
  newValue.value = cond.value ?? ''
  newProbeKind.value = cond.probe_kind ?? 'baseline'
  newFreshnessSeconds.value = cond.freshness_seconds
  validationError.value = ''
}

function removeCondition(index: number) {
  if (editingIndex.value === index) {
    resetDraft()
  } else if (editingIndex.value !== null && editingIndex.value > index) {
    editingIndex.value--
  }
  activeConditions.value.splice(index, 1)
  emitConditions(activeConditions.value)
}

function clearAllConditions() {
  activeConditions.value = []
  resetDraft()
  emitConditions(activeConditions.value)
}

function isConditionProbe(field: FilterField) {
  return field === 'probe_verdict' || field === 'probe_latency_ms'
}

function onInlineFieldChange(cond: FilterCondition) {
  const ops = filterFieldOperators(cond.field)
  cond.op = ops[0]!
  cond.value = cond.field === 'probe_verdict' ? 'available' : ''
  if (isConditionProbe(cond.field)) {
    cond.probe_kind = 'baseline'
  } else {
    delete cond.probe_kind
    delete cond.freshness_seconds
  }
  emitConditions(activeConditions.value)
}

function addInlineCondition() {
  if (activeConditions.value.length >= props.maxConditions) return
  activeConditions.value.push({ field: 'probe_verdict', op: 'equals', value: 'available', probe_kind: 'baseline' })
  emitConditions(activeConditions.value)
}
</script>

<template>
  <div class="space-y-3 structured-filter-editor" data-testid="structured-filter-editor">
    <!-- Inline Mode -->
    <template v-if="inline">
      <p v-if="activeConditions.length === 0" data-testid="filter-empty" class="text-sm text-base-content/60">{{ t('settings.filterEmpty') }}</p>
      <p v-else class="text-xs text-base-content/70">{{ t('settings.filterAnd') }}</p>

      <div
        v-for="(condition, index) in activeConditions"
        :key="index"
        class="rounded-xl bg-base-100/70 border border-base-300 p-3 space-y-3"
        data-testid="filter-condition"
      >
        <div class="flex flex-wrap gap-2 items-center">
          <label class="text-xs font-medium">{{ t('settings.filterField') }}
            <select
              v-model="condition.field"
              class="select select-bordered select-sm block"
              :aria-label="t('settings.filterField')"
              @change="onInlineFieldChange(condition)"
            >
              <option v-for="item in SUPPORTED_FILTER_FIELDS" :key="item.field" :value="item.field">
                {{ t(`settings.filterField_${item.field}`) }}
              </option>
            </select>
          </label>
          <label class="text-xs font-medium">{{ t('settings.filterOperator') }}
            <select
              v-model="condition.op"
              class="select select-bordered select-sm block"
              :aria-label="t('settings.filterOperator')"
              @change="emitConditions(activeConditions)"
            >
              <option v-for="op in filterFieldOperators(condition.field)" :key="op" :value="op">
                {{ t(`settings.filterOp_${op}`) }}
              </option>
            </select>
          </label>
          <label v-if="isConditionProbe(condition.field)" class="text-xs font-medium">{{ t('settings.filterKind') }}
            <select
              v-model="condition.probe_kind"
              class="select select-bordered select-sm block"
              :aria-label="t('settings.filterKind')"
              @change="emitConditions(activeConditions)"
            >
              <option v-for="kind in SUPPORTED_PROBE_KINDS" :key="kind" :value="kind">
                {{ t(`settings.filterKind_${kind}`) }}
              </option>
            </select>
          </label>
          <label class="text-xs font-medium">{{ t('settings.filterValue') }}
            <select
              v-if="condition.field === 'probe_verdict'"
              v-model="condition.value"
              class="select select-bordered select-sm block"
              :aria-label="t('settings.filterValue')"
              @change="emitConditions(activeConditions)"
            >
              <option v-for="verdict in SUPPORTED_PROBE_VERDICTS" :key="verdict" :value="verdict">
                {{ t(`settings.filterVerdict_${verdict}`) }}
              </option>
            </select>
            <select
              v-else-if="condition.field === 'protocol'"
              v-model="condition.value"
              class="select select-bordered select-sm block"
              :aria-label="t('settings.filterValue')"
              @change="emitConditions(activeConditions)"
            >
              <option value="" disabled>{{ t('settings.filterSelect') }}</option>
              <option v-for="protocol in SUPPORTED_PROTOCOLS" :key="protocol" :value="protocol">{{ protocol }}</option>
            </select>
            <input
              v-else
              v-model="condition.value"
              class="input input-bordered input-sm block max-w-full"
              :aria-label="t('settings.filterValue')"
              @input="emitConditions(activeConditions)"
            />
          </label>
          <label v-if="isConditionProbe(condition.field)" class="text-xs font-medium">{{ t('settings.filterFreshness') }}
            <input
              :value="condition.freshness_seconds ?? ''"
              type="number"
              min="1"
              max="604800"
              step="1"
              class="input input-bordered input-sm block max-w-full"
              :aria-label="t('settings.filterFreshness')"
              :placeholder="t('settings.filterDefaultFreshness')"
              @input="condition.freshness_seconds = ($event.target as HTMLInputElement).value === '' ? undefined : Number(($event.target as HTMLInputElement).value); emitConditions(activeConditions)"
            />
          </label>
          <button
            type="button"
            class="btn btn-ghost btn-sm text-error self-end"
            :aria-label="t('settings.filterRemove')"
            @click="removeCondition(index)"
          >
            {{ t('settings.filterRemove') }}
          </button>
        </div>
      </div>

      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="btn btn-outline btn-sm"
          data-testid="filter-add"
          :disabled="activeConditions.length >= maxConditions"
          @click="addInlineCondition"
        >
          {{ t('settings.filterAdd') }}
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-sm"
          data-testid="filter-clear"
          :disabled="activeConditions.length === 0"
          @click="clearAllConditions"
        >
          {{ t('settings.filterClear') }}
        </button>
      </div>
    </template>

    <!-- Standard Badge + Draft Mode -->
    <template v-else>
      <div v-if="title || description" class="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h4 v-if="title" class="font-bold text-xs sm:text-sm">{{ title }}</h4>
          <p v-if="description" class="text-[11px] opacity-70 mt-0.5">{{ description }}</p>
        </div>
        <span class="badge badge-xs badge-info font-mono">{{ activeConditions.length }}/{{ maxConditions }} 条条件</span>
      </div>

    <!-- Active conditions list (evaluated as AND) -->
    <div v-if="activeConditions.length > 0" class="space-y-1.5">
      <div class="flex items-center justify-between text-[11px] text-base-content/70">
        <span class="font-semibold text-primary">满足全部条件 (AND 组合，共 {{ activeConditions.length }} 条)：</span>
        <button
          v-if="allowClear && !readOnly"
          type="button"
          class="btn btn-ghost btn-xs text-error font-medium"
          data-testid="filter-clear"
          @click="clearAllConditions"
        >
          清空条件
        </button>
      </div>

      <div
        v-for="(cond, idx) in activeConditions"
        :key="idx"
        class="flex flex-wrap items-center justify-between gap-2 p-2 rounded-lg bg-base-100 border border-base-300 text-xs font-mono"
        data-testid="filter-condition"
      >
        <div class="flex flex-wrap items-center gap-1.5 min-w-0 flex-1 truncate">
          <span class="badge badge-xs badge-neutral font-mono">#{{ idx + 1 }}</span>
          <span class="badge badge-xs badge-outline font-sans">
            {{ t(`settings.filterField_${cond.field}`) || SUPPORTED_FILTER_FIELDS.find(f => f.field === cond.field)?.label || cond.field }}
          </span>
          <span class="text-primary font-bold">{{ t(`settings.filterOp_${cond.op}`) || cond.op }}</span>
          <span class="text-secondary font-mono truncate max-w-[240px]">"{{ cond.value }}"</span>
          <span v-if="cond.probe_kind" class="badge badge-xs badge-ghost font-mono">[{{ cond.probe_kind }}]</span>
          <span v-if="cond.freshness_seconds" class="text-[10px] opacity-60">(&le;{{ cond.freshness_seconds }}s)</span>
        </div>

        <div v-if="!readOnly" class="flex items-center gap-1 shrink-0">
          <button
            type="button"
            class="btn btn-ghost btn-xs text-primary"
            @click="startEditCondition(idx)"
          >
            编辑
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs text-error p-1"
            title="删除此条件"
            data-testid="filter-remove"
            @click="removeCondition(idx)"
          >
            <TrashIcon class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>
    <div v-else class="p-3 rounded-lg bg-base-100/60 border border-base-300/80 text-xs text-base-content/60 space-y-1">
      <div class="flex items-center justify-between">
        <span class="font-medium" data-testid="filter-empty">无额外筛选条件 (匹配全部候选节点)</span>
        <span class="badge badge-xs badge-ghost">清空状态</span>
      </div>
      <p class="text-[11px] opacity-75">
        当前无过滤条件，表示不进行任何额外属性过滤；系统绝不偷偷附加健康或可用性限制条件。
      </p>
    </div>

    <!-- Condition Editor Input Panel -->
    <div v-if="!readOnly" class="p-2.5 rounded-lg bg-base-200/80 border border-base-300 space-y-2 text-xs" data-testid="condition-draft">
      <div class="flex items-center justify-between text-[11px] font-semibold">
        <span>{{ editingIndex !== null ? `编辑条件 #${editingIndex + 1}` : '添加新筛选条件 (AND)' }}</span>
        <button
          v-if="editingIndex !== null"
          type="button"
          class="btn btn-ghost btn-xs text-warning"
          @click="resetDraft"
        >
          取消编辑
        </button>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
        <label class="form-control">
          <span class="label-text text-[10px] opacity-70">{{ t('settings.filterField') || '筛选字段' }}</span>
          <select
            v-model="newField"
            data-testid="condition-field"
            :aria-label="t('settings.filterField')"
            class="select select-bordered select-xs w-full"
            @change="onFieldChange"
          >
            <option v-for="f in SUPPORTED_FILTER_FIELDS" :key="f.field" :value="f.field">
              {{ t(`settings.filterField_${f.field}`) || f.label }}
            </option>
          </select>
        </label>

        <label class="form-control">
          <span class="label-text text-[10px] opacity-70">{{ t('settings.filterOperator') || '匹配运算符' }}</span>
          <select
            v-model="newOp"
            data-testid="condition-op"
            :aria-label="t('settings.filterOperator')"
            class="select select-bordered select-xs w-full font-mono"
          >
            <option v-for="op in filterFieldOperators(newField)" :key="op" :value="op">
              {{ t(`settings.filterOp_${op}`) || op }}
            </option>
          </select>
        </label>

        <label class="form-control">
          <span class="label-text text-[10px] opacity-70">{{ t('settings.filterValue') || '匹配目标值' }}</span>
          <select
            v-if="newField === 'probe_verdict'"
            v-model="newValue"
            data-testid="condition-value"
            :aria-label="t('settings.filterValue')"
            class="select select-bordered select-xs w-full"
          >
            <option v-for="v in SUPPORTED_PROBE_VERDICTS" :key="v" :value="v">
              {{ t(`settings.filterVerdict_${v}`) || v }}
            </option>
          </select>
          <select
            v-else-if="newField === 'protocol'"
            v-model="newValue"
            data-testid="condition-value"
            :aria-label="t('settings.filterValue')"
            class="select select-bordered select-xs w-full font-mono"
          >
            <option v-for="p in SUPPORTED_PROTOCOLS" :key="p" :value="p">
              {{ p }}
            </option>
          </select>
          <input
            v-else
            v-model="newValue"
            data-testid="condition-value"
            :aria-label="t('settings.filterValue')"
            class="input input-bordered input-xs w-full font-mono"
            :placeholder="newField === 'source_subscription_ids' ? '订阅 UUIDv7...' : (newField === 'probe_latency_ms' ? '毫秒 (例如: 200)' : '例如: HK|香港 或 regex...')"
            @keydown.enter.prevent="addOrUpdateCondition"
          />
        </label>
      </div>

      <!-- Probe Specific Kind & Freshness -->
      <div v-if="isProbe" class="grid grid-cols-1 sm:grid-cols-2 gap-2 pt-1 border-t border-base-300/40">
        <label class="form-control">
          <span class="label-text text-[10px] opacity-70">{{ t('settings.filterKind') || '探针类型' }}</span>
          <select
            v-model="newProbeKind"
            :aria-label="t('settings.filterKind')"
            class="select select-bordered select-xs w-full font-mono"
          >
            <option v-for="k in SUPPORTED_PROBE_KINDS" :key="k" :value="k">
              {{ t(`settings.filterKind_${k}`) || k }}
            </option>
          </select>
        </label>

        <label class="form-control">
          <span class="label-text text-[10px] opacity-70">{{ t('settings.filterFreshness') || '最大有效时长' }} (秒，可选 1..604800)</span>
          <input
            :value="newFreshnessSeconds ?? ''"
            type="number"
            min="1"
            max="604800"
            step="1"
            :aria-label="t('settings.filterFreshness')"
            class="input input-bordered input-xs w-full font-mono"
            :placeholder="t('settings.filterDefaultFreshness') || '默认有效期...'"
            @input="newFreshnessSeconds = ($event.target as HTMLInputElement).value === '' ? undefined : Number(($event.target as HTMLInputElement).value)"
          />
        </label>
      </div>

      <div v-if="validationError" role="alert" class="text-error text-xs pt-1">
        {{ validationError }}
      </div>

      <div class="flex items-center justify-between pt-1">
        <span class="text-[10px] opacity-60">条件之间为 AND 约束（必须全部满足）</span>
        <button
          type="button"
          class="btn btn-primary btn-xs gap-1"
          data-testid="filter-add"
          :disabled="activeConditions.length >= maxConditions"
          @click="addOrUpdateCondition"
        >
          <PlusIcon class="w-3.5 h-3.5" />
          添加条件
        </button>
      </div>
    </div>
  </template>
</div>
</template>

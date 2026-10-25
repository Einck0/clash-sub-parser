<script setup lang="ts">
import { computed } from 'vue'
import { uiText } from '../../locales/redesign'
import {
  type DnsFieldDef,
  getBoolDisplayState,
} from './dnsFields'

const props = withDefaults(
  defineProps<{
    field: DnsFieldDef
    draft: Record<string, unknown>
    fieldError?: string
    invalidJsonInput?: string
    disabled?: boolean
  }>(),
  {
    fieldError: '',
    invalidJsonInput: undefined,
    disabled: false,
  }
)

const emit = defineEmits<{
  (e: 'update-list', key: string, text: string): void
  (e: 'update-scalar', key: string, val: string): void
  (e: 'update-bool', key: string, val: boolean): void
  (e: 'update-json', key: string, text: string): void
  (e: 'remove-key', key: string): void
}>()

const isKeyConfigured = computed(() => {
  return props.field.key in props.draft && props.draft[props.field.key] !== undefined
})

const boolDisplayState = computed(() => {
  return getBoolDisplayState(props.draft, props.field.key)
})

function getListText(key: string): string {
  const val = props.draft[key]
  if (Array.isArray(val)) {
    return val.join('\n')
  }
  return ''
}

function onListInput(e: Event) {
  const target = e.target as HTMLTextAreaElement
  emit('update-list', props.field.key, target.value)
}

function getString(key: string): string {
  const val = props.draft[key]
  if (val === undefined || val === null) return ''
  return String(val)
}

const hasCustomSelectOption = computed(() => {
  if (props.field.type !== 'scalar-select') return false
  const currentVal = getString(props.field.key)
  if (!currentVal) return false
  return !props.field.options?.some((opt) => opt.value === currentVal)
})

function onScalarInput(e: Event) {
  const target = e.target as HTMLInputElement
  const val = target.value
  if (val.trim() === '') {
    emit('remove-key', props.field.key)
  } else {
    emit('update-scalar', props.field.key, val)
  }
}

function onSelectChange(e: Event) {
  const target = e.target as HTMLSelectElement
  const val = target.value
  if (val === '') {
    emit('remove-key', props.field.key)
  } else {
    emit('update-scalar', props.field.key, val)
  }
}

function getBoolSelectValue(key: string): string {
  const state = boolDisplayState.value
  if (state === 'true') return 'true'
  if (state === 'false') return 'false'
  if (state === 'custom') return String(props.draft[key])
  return ''
}

function onBoolSelectChange(e: Event) {
  const target = e.target as HTMLSelectElement
  const val = target.value
  if (val === '') {
    emit('remove-key', props.field.key)
  } else if (val === 'true') {
    emit('update-bool', props.field.key, true)
  } else if (val === 'false') {
    emit('update-bool', props.field.key, false)
  }
}

function getJsonText(key: string): string {
  if (props.invalidJsonInput !== undefined) {
    return props.invalidJsonInput
  }
  const val = props.draft[key]
  if (val === undefined || val === null) return ''
  return JSON.stringify(val, null, 2)
}

function onJsonInput(e: Event) {
  const target = e.target as HTMLTextAreaElement
  const val = target.value
  if (val.trim() === '') {
    emit('remove-key', props.field.key)
  } else {
    emit('update-json', props.field.key, val)
  }
}
</script>

<template>
  <div
    class="flex flex-col gap-2 p-3 sm:p-4 rounded-xl bg-base-100/70 border border-base-300 shadow-xs transition-colors"
    :class="{ 'border-error/50 bg-error/5': fieldError }"
  >
    <!-- Label & Header Row -->
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0 flex-1">
        <label
          :for="`field-${field.key}`"
          class="flex flex-wrap items-center gap-1.5 cursor-pointer select-none"
        >
          <span class="text-xs sm:text-sm font-semibold text-base-content/90">
            {{ uiText(field.labelZh, field.labelEn) }}
          </span>
          <code class="font-mono text-[10px] sm:text-[11px] opacity-70 bg-base-300/50 px-1 py-0.5 rounded">
            {{ field.key }}
          </code>
        </label>
        <p class="text-[11px] text-base-content/60 leading-tight mt-0.5">
          {{ uiText(field.descZh, field.descEn) }}
        </p>
      </div>

      <!-- State Badges & Reset Button -->
      <div class="flex items-center gap-1.5 shrink-0">
        <!-- Bool badge -->
        <template v-if="field.type === 'bool'">
          <span
            v-if="boolDisplayState === 'unspecified'"
            class="badge badge-sm badge-ghost opacity-60 text-[10px]"
          >
            {{ uiText('未指定', 'Unspecified') }}
          </span>
          <span
            v-else-if="boolDisplayState === 'true'"
            class="badge badge-sm badge-success text-[10px]"
          >
            {{ uiText('已启用', 'Enabled') }}
          </span>
          <span
            v-else-if="boolDisplayState === 'false'"
            class="badge badge-sm badge-neutral text-[10px]"
          >
            {{ uiText('已禁用', 'Disabled') }}
          </span>
          <span
            v-else
            class="badge badge-sm badge-warning text-[10px]"
          >
            {{ uiText('自定义', 'Custom') }}
          </span>
        </template>

        <!-- List count -->
        <span
          v-else-if="field.type === 'list' && Array.isArray(draft[field.key])"
          class="text-[10px] sm:text-[11px] opacity-60 font-mono"
        >
          {{ (draft[field.key] as unknown[]).length }} {{ uiText('条', 'entries') }}
        </span>

        <!-- Reset / Unset Key Button -->
        <button
          v-if="isKeyConfigured"
          type="button"
          class="btn btn-ghost btn-xs text-[10px] sm:text-[11px] text-error/80 hover:text-error px-1.5"
          :disabled="disabled"
          :data-testid="`dns-reset-${field.key}`"
          :title="uiText('重置为未指定 (删除此字段键)', 'Reset to unspecified (remove this key)')"
          @click="emit('remove-key', field.key)"
        >
          {{ uiText('重置', 'Reset') }}
        </button>
      </div>
    </div>

    <!-- Control Area -->
    <div class="space-y-1.5">
      <!-- 1. List Multiline -->
      <template v-if="field.type === 'list'">
        <textarea
          :id="`field-${field.key}`"
          class="textarea textarea-sm min-h-24 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg focus:border-primary"
          :aria-label="field.key"
          :data-testid="`dns-list-${field.key}`"
          :placeholder="field.placeholder || uiText('每行一个条目', 'One entry per line')"
          :value="getListText(field.key)"
          :disabled="disabled"
          @input="onListInput"
          @change="onListInput"
        />
      </template>

      <!-- 2. Scalar Text -->
      <template v-else-if="field.type === 'scalar-text'">
        <input
          :id="`field-${field.key}`"
          type="text"
          class="input input-sm font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg focus:border-primary"
          :aria-label="field.key"
          :data-testid="`dns-input-${field.key}`"
          :placeholder="field.placeholder"
          :value="getString(field.key)"
          :disabled="disabled"
          @input="onScalarInput"
        />
      </template>

      <!-- 3. Scalar Select -->
      <template v-else-if="field.type === 'scalar-select'">
        <select
          :id="`field-${field.key}`"
          class="select select-sm font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg focus:border-primary"
          :aria-label="field.key"
          :data-testid="`dns-select-${field.key}`"
          :value="getString(field.key)"
          :disabled="disabled"
          @change="onSelectChange"
        >
          <option
            v-for="opt in field.options"
            :key="opt.value"
            :value="opt.value"
          >
            {{ uiText(opt.labelZh, opt.labelEn) }}
          </option>
          <option
            v-if="hasCustomSelectOption"
            :value="getString(field.key)"
            data-testid="dns-select-custom-option"
          >
            {{ uiText('自定义值：', 'Custom value: ') }}{{ getString(field.key) }}
          </option>
        </select>
      </template>

      <!-- 4. Bool Select (Tri-state) -->
      <template v-else-if="field.type === 'bool'">
        <select
          :id="`field-${field.key}`"
          class="select select-sm font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg focus:border-primary"
          :aria-label="field.key"
          :data-testid="`dns-bool-${field.key}`"
          :value="getBoolSelectValue(field.key)"
          :disabled="disabled"
          @change="onBoolSelectChange"
        >
          <option value="">
            {{ uiText('未指定 (遵循内核缺省)', 'Unspecified (core default)') }}
          </option>
          <option value="true">
            {{ uiText('true (开启)', 'true (enabled)') }}
          </option>
          <option value="false">
            {{ uiText('false (关闭)', 'false (disabled)') }}
          </option>
          <option
            v-if="boolDisplayState === 'custom'"
            :value="String(draft[field.key])"
          >
            {{ uiText('自定义值：', 'Custom value: ') }}{{ draft[field.key] }}
          </option>
        </select>

        <!-- Advisory for respect-rules -->
        <div
          v-if="field.key === 'respect-rules' && boolDisplayState === 'true'"
          class="text-[10px] text-warning mt-1"
          data-testid="respect-rules-advisory"
        >
          {{ uiText('提示：启用 respect-rules 必须配置 proxy-server-nameserver，且官方强烈不建议与 prefer-h3 同时开启。', 'Advisory: respect-rules requires proxy-server-nameserver, and is strongly not recommended with prefer-h3.') }}
        </div>

        <!-- Advisory for prefer-h3 conflict -->
        <div
          v-if="field.key === 'prefer-h3' && boolDisplayState === 'true' && getBoolDisplayState(draft, 'respect-rules') === 'true'"
          class="text-[10px] text-warning mt-1"
          data-testid="prefer-h3-advisory"
        >
          {{ uiText('提示：prefer-h3 与已启用的 respect-rules 存在冲突，官方强烈不建议同时启用。', 'Advisory: prefer-h3 conflicts with enabled respect-rules; enabling both is strongly not recommended.') }}
        </div>
      </template>

      <!-- 5. JSON Mapping -->
      <template v-else-if="field.type === 'json'">
        <div v-if="field.example" class="text-[10px] text-base-content/60">
          <span class="font-medium">{{ uiText('格式示例 (必须为 JSON 对象)：', 'Example format (must be a JSON object):') }}</span>
          <pre class="bg-base-200/80 border border-base-300/60 p-1.5 rounded mt-0.5 font-mono overflow-x-auto text-[10px]">{{ field.example }}</pre>
        </div>
        <textarea
          :id="`field-${field.key}`"
          class="textarea textarea-sm min-h-24 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg focus:border-primary"
          :class="{ 'textarea-error border-error': fieldError }"
          :aria-label="field.key"
          :data-testid="`dns-json-${field.key}`"
          :value="getJsonText(field.key)"
          :placeholder="field.example || '{}'"
          :disabled="disabled"
          @input="onJsonInput"
          @change="onJsonInput"
        />
        <div v-if="fieldError" class="text-error text-xs font-medium flex items-center gap-1 mt-1" data-testid="field-error">
          <span>⚠ {{ fieldError }}</span>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import ModalDialog from '../../ui/ModalDialog.vue'
import {
  COMMON_RULE_TYPES,
  type CommonRuleType,
  type CreateRulePayload,
  type PolicyRuleView,
  type RuleCategory,
  type UpdateRulePayload,
  parseRuleExpression,
} from './rulesTypes'
import type { PolicyGroupSummary } from './useRules'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'

const props = defineProps<{
  modelValue: boolean
  rule: PolicyRuleView | null
  defaultCategoryId?: string
  categories: RuleCategory[]
  groups: PolicyGroupSummary[]
  saving: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [open: boolean]
  save: [payload: CreateRulePayload | UpdateRulePayload]
}>()

const isEditing = computed(() => !!props.rule)

interface RuleFormState {
  category_id: string
  name: string
  rule_type: CommonRuleType
  rule_value: string
  target_kind: 'builtin' | 'group'
  builtin_target: string
  target_group_id: string
  no_resolve: boolean
}

const form = ref<RuleFormState>({
  category_id: '',
  name: '',
  rule_type: 'DOMAIN-SUFFIX',
  rule_value: '',
  target_kind: 'builtin',
  builtin_target: 'DIRECT',
  target_group_id: '',
  no_resolve: false,
})

const initialSnapshot = ref<string>('')

function resetForm() {
  if (props.rule) {
    const parsed = parseRuleExpression(props.rule.expression)
    const isMatch = parsed.type === 'MATCH' || parsed.type === 'FINAL'
    const ruleType = (
      isMatch
        ? 'MATCH'
        : COMMON_RULE_TYPES.find((ct) => ct === parsed.type) || 'DOMAIN-SUFFIX'
    ) as CommonRuleType
    const isBuiltin = !!props.rule.builtin_target || !props.rule.target_group_id

    form.value = {
      category_id: props.rule.category_id || '',
      name: props.rule.name || '',
      rule_type: ruleType,
      rule_value: parsed.value,
      target_kind: isBuiltin ? 'builtin' : 'group',
      builtin_target: props.rule.builtin_target || 'DIRECT',
      target_group_id: props.rule.target_group_id || props.groups[0]?.id || '',
      no_resolve: parsed.noResolve,
    }
  } else {
    form.value = {
      category_id: props.defaultCategoryId || props.categories[0]?.id || '',
      name: '',
      rule_type: 'DOMAIN-SUFFIX',
      rule_value: '',
      target_kind: 'builtin',
      builtin_target: 'DIRECT',
      target_group_id: props.groups[0]?.id || '',
      no_resolve: false,
    }
  }
  initialSnapshot.value = JSON.stringify(form.value)
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      resetForm()
    }
  },
  { immediate: true }
)

const isDirty = computed(() => {
  if (!props.modelValue) return false
  return JSON.stringify(form.value) !== initialSnapshot.value
})

// Unsaved changes guard
let unsavedGuard: ReturnType<typeof useUnsavedChanges> | null = null
try {
  unsavedGuard = useUnsavedChanges(
    isDirty,
    () => uiText('未保存的规则修改将会丢失，确定要放弃吗？', 'Unsaved rule changes will be lost. Discard?')
  )
} catch {
  // Graceful fallback if composable unavailable in bare test harness
}

function handleClose() {
  if (unsavedGuard && typeof unsavedGuard.confirmDiscard === 'function') {
    if (!unsavedGuard.confirmDiscard()) {
      return
    }
  } else if (isDirty.value) {
    const ok = window.confirm(
      uiText('未保存的规则修改将会丢失，确定要放弃吗？', 'Unsaved rule changes will be lost. Discard?')
    )
    if (!ok) return
  }
  emit('update:modelValue', false)
}

function handleSubmit() {
  const {
    category_id,
    name,
    rule_type,
    rule_value,
    target_kind,
    builtin_target,
    target_group_id,
    no_resolve,
  } = form.value

  let expression = ''
  if (rule_type === 'MATCH') {
    expression = 'MATCH'
  } else {
    expression = `${rule_type},${rule_value.trim()}`
    if (no_resolve) {
      expression += ',no-resolve'
    }
  }

  const payload: CreateRulePayload | UpdateRulePayload = {
    category_id: category_id || undefined,
    name: name.trim() || undefined,
    expression,
    builtin_target: target_kind === 'builtin' ? builtin_target : '',
    target_group_id: target_kind === 'group' ? target_group_id : '',
  }

  // Update initialSnapshot so dirty guard won't block after saving
  initialSnapshot.value = JSON.stringify(form.value)
  emit('save', payload)
}
</script>

<template>
  <ModalDialog
    :model-value="modelValue"
    :title="isEditing ? t('rules.editRule') : t('rules.newRule')"
    :close-on-backdrop="false"
    @update:model-value="handleClose"
  >
    <form class="space-y-4" @submit.prevent="handleSubmit">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <!-- Category Selection -->
        <div class="sm:col-span-2 space-y-1.5">
          <label for="rule-category-select" class="block text-xs font-semibold text-base-content/80">
            {{ t('rules.categoryLabel') }}
          </label>
          <select
            id="rule-category-select"
            v-model="form.category_id"
            class="select select-sm border border-base-300 w-full text-xs rounded-lg"
          >
            <option
              v-for="cat in categories"
              :key="cat.id"
              :value="cat.id"
            >
              {{ cat.name }} ({{ t('rules.categoryWeight') }} {{ cat.sort_order }})
            </option>
          </select>
        </div>

        <!-- Rule Name (Optional) -->
        <div class="sm:col-span-2 space-y-1.5">
          <label for="rule-name-input" class="block text-xs font-semibold text-base-content/80">
            {{ t('rules.ruleName') }}
          </label>
          <input
            id="rule-name-input"
            v-model="form.name"
            type="text"
            class="input input-sm border border-base-300 w-full text-xs rounded-lg"
            :placeholder="t('rules.ruleNamePlaceholder')"
          />
        </div>

        <!-- Rule Type -->
        <div class="space-y-1.5">
          <label for="rule-type-select" class="block text-xs font-semibold text-base-content/80">
            {{ t('rules.ruleType') }}
          </label>
          <select
            id="rule-type-select"
            v-model="form.rule_type"
            class="select select-sm border border-base-300 w-full text-xs font-mono rounded-lg"
          >
            <option
              v-for="rtype in COMMON_RULE_TYPES"
              :key="rtype"
              :value="rtype"
            >
              {{ rtype }}
            </option>
          </select>
        </div>

        <!-- Expression Value -->
        <div class="sm:col-span-2 space-y-1.5">
          <label for="rule-expression-input" class="block text-xs font-semibold text-base-content/80">
            {{ t('rules.expression') }}
          </label>
          <input
            v-if="form.rule_type !== 'MATCH'"
            id="rule-expression-input"
            v-model="form.rule_value"
            type="text"
            class="input input-sm border border-base-300 w-full font-mono text-xs rounded-lg"
            :placeholder="t('rules.expressionPlaceholder')"
            required
          />
          <div
            v-else
            id="rule-expression-input"
            class="input input-sm border border-base-300 w-full flex items-center bg-base-200/50 text-xs font-mono opacity-80 rounded-lg select-none"
          >
            MATCH (Terminal Catch-all)
          </div>
        </div>
      </div>

      <!-- No Resolve Checkbox -->
      <div v-if="form.rule_type !== 'MATCH'" class="flex items-center gap-2">
        <input
          id="rule-no-resolve-field"
          v-model="form.no_resolve"
          type="checkbox"
          class="checkbox checkbox-xs checkbox-primary"
        />
        <label for="rule-no-resolve-field" class="text-xs cursor-pointer select-none">
          {{ t('rules.noResolve') }}
        </label>
      </div>

      <!-- Target Selector (Segmented Radio) -->
      <div class="pt-2 border-t border-base-300/40 space-y-2">
        <span class="block text-xs font-semibold text-base-content/80">
          {{ t('rules.target') }}
        </span>

        <div class="flex items-center gap-4 mb-2">
          <label class="flex items-center gap-2 text-xs cursor-pointer select-none">
            <input
              v-model="form.target_kind"
              type="radio"
              value="builtin"
              class="radio radio-xs radio-primary"
            />
            <span>{{ t('rules.builtinTarget') }}</span>
          </label>

          <label class="flex items-center gap-2 text-xs cursor-pointer select-none">
            <input
              v-model="form.target_kind"
              type="radio"
              value="group"
              class="radio radio-xs radio-primary"
            />
            <span>{{ t('rules.policyGroup') }}</span>
          </label>
        </div>

        <!-- Builtin Target Select -->
        <div v-if="form.target_kind === 'builtin'">
          <label for="rule-builtin-target-select" class="sr-only">{{ t('rules.builtinTarget') }}</label>
          <select
            id="rule-builtin-target-select"
            v-model="form.builtin_target"
            class="select select-sm border border-base-300 w-full text-xs rounded-lg"
          >
            <option value="DIRECT">DIRECT</option>
            <option value="REJECT">REJECT</option>
            <option value="PASS">PASS</option>
          </select>
        </div>

        <!-- Policy Group Select -->
        <div v-else>
          <label for="rule-target-group-select" class="sr-only">{{ t('rules.policyGroup') }}</label>
          <select
            id="rule-target-group-select"
            v-model="form.target_group_id"
            class="select select-sm border border-base-300 w-full text-xs rounded-lg"
          >
            <option
              v-for="g in groups"
              :key="g.id"
              :value="g.id"
            >
              {{ g.name }} ({{ g.group_type || 'group' }})
            </option>
          </select>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <button
          type="button"
          class="btn btn-sm btn-ghost"
          @click="handleClose"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-sm btn-primary"
          :disabled="saving"
          @click="handleSubmit"
        >
          <span v-if="saving" class="loading loading-spinner loading-xs mr-1"></span>
          {{ t('common.save') }}
        </button>
      </div>
    </template>
  </ModalDialog>
</template>

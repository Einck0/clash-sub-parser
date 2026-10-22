<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { PlusIcon, TrashIcon } from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import {
  defaultPolicy,
  defaultConfig,
  extractSecondLevelDomainName,
  type SubscriptionDraft,
  type SubscriptionRecord,
  type ProbePolicyMode,
} from './useSubscriptions'

const props = defineProps<{
  modelValue: boolean
  subscription?: SubscriptionRecord
  saving?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'save', draft: SubscriptionDraft): void
}>()

const activeTab = ref<'basic' | 'advanced'>('basic')
const draft = ref<SubscriptionDraft>({
  name: '',
  source_url_secret_ref: '',
  enabled: true,
  prefix: '',
  refresh_policy: defaultPolicy(),
  config: defaultConfig(),
})

const targetGroupsInput = ref('')
const nameError = ref('')
const urlError = ref('')
const generalError = ref('')
const probePolicyMode = ref<ProbePolicyMode>('inherit')
const probeIntervalSeconds = ref<number>(3600)

const userManuallyEditedName = ref(false)
const lastAutoName = ref('')

function clearErrors() {
  nameError.value = ''
  urlError.value = ''
  generalError.value = ''
}

function getDraftSnapshot(): string {
  return JSON.stringify({
    name: draft.value.name,
    source_url_secret_ref: draft.value.source_url_secret_ref,
    enabled: draft.value.enabled,
    prefix: draft.value.prefix,
    refresh_policy: draft.value.refresh_policy,
    config: draft.value.config,
    targetGroups: targetGroupsInput.value,
    probeMode: probePolicyMode.value,
    probeInterval: probeIntervalSeconds.value,
  })
}

const initialSnapshot = ref<string>('')

watch(
  () => [props.modelValue, props.subscription],
  ([open]) => {
    clearErrors()
    const sub = props.subscription
    if (sub) {
      draft.value = {
        name: sub.name,
        source_url_secret_ref: sub.source_url_secret_ref || '',
        enabled: sub.enabled,
        prefix: sub.prefix || '',
        refresh_policy: { ...defaultPolicy(), ...sub.refresh_policy },
        config: {
          cron_schedule: sub.config?.cron_schedule || '',
          auto_test: !!sub.config?.auto_test,
          rename_rules: sub.config?.rename_rules ? [...sub.config.rename_rules] : [],
          filter_rules: sub.config?.filter_rules ? [...sub.config.filter_rules] : [],
          target_groups: sub.config?.target_groups ? [...sub.config.target_groups] : [],
          probe_policy: sub.config?.probe_policy
            ? { ...sub.config.probe_policy }
            : { mode: 'inherit' },
        },
      }
      targetGroupsInput.value = (sub.config?.target_groups || []).join(', ')
      if (sub.config?.probe_policy) {
        probePolicyMode.value = sub.config.probe_policy.mode || 'inherit'
        probeIntervalSeconds.value =
          typeof sub.config.probe_policy.interval_seconds === 'number'
            ? sub.config.probe_policy.interval_seconds
            : 3600
      } else {
        probePolicyMode.value = 'inherit'
        probeIntervalSeconds.value = 3600
      }
      userManuallyEditedName.value = true
      lastAutoName.value = ''
    } else {
      draft.value = {
        name: '',
        source_url_secret_ref: '',
        enabled: true,
        prefix: '',
        refresh_policy: defaultPolicy(),
        config: defaultConfig(),
      }
      targetGroupsInput.value = ''
      probePolicyMode.value = 'inherit'
      probeIntervalSeconds.value = 3600
      userManuallyEditedName.value = false
      lastAutoName.value = ''
    }
    if (open) {
      initialSnapshot.value = getDraftSnapshot()
    }
  },
  { immediate: true }
)

const isDirty = computed(() => {
  if (!props.modelValue) return false
  return getDraftSnapshot() !== initialSnapshot.value
})

let unsavedGuard: ReturnType<typeof useUnsavedChanges> | null = null
try {
  unsavedGuard = useUnsavedChanges(
    isDirty,
    () => uiText('当前有未保存的订阅源修改，确定要放弃吗？', 'You have unsaved subscription changes. Are you sure you want to discard them?')
  )
} catch {
  // Graceful fallback for routerless unit tests
}

function confirmBeforeClose(): boolean {
  if (isDirty.value) {
    if (unsavedGuard && typeof unsavedGuard.confirmDiscard === 'function') {
      const allowed = unsavedGuard.confirmDiscard()
      if (!allowed) return false
    } else if (typeof window !== 'undefined' && typeof window.confirm === 'function') {
      const allowed = window.confirm(
        uiText('当前有未保存的订阅源修改，确定要放弃吗？', 'You have unsaved subscription changes. Are you sure you want to discard them?')
      )
      if (!allowed) return false
    }
    initialSnapshot.value = getDraftSnapshot()
  }
  clearErrors()
  return true
}

function handleClose() {
  if (confirmBeforeClose()) {
    emit('update:modelValue', false)
  }
}

function handleDrawerUpdateModelValue(value: boolean) {
  if (!value) {
    if (confirmBeforeClose()) {
      emit('update:modelValue', false)
    }
  } else {
    emit('update:modelValue', true)
  }
}

function onNameInput() {
  nameError.value = ''
  generalError.value = ''
  const trimmed = draft.value.name.trim()
  if (trimmed !== lastAutoName.value) {
    userManuallyEditedName.value = true
  } else if (!trimmed) {
    userManuallyEditedName.value = false
  }
}

function onUrlInput() {
  urlError.value = ''
  generalError.value = ''
  if (!userManuallyEditedName.value || !draft.value.name.trim() || draft.value.name === lastAutoName.value) {
    const auto = extractSecondLevelDomainName(draft.value.source_url_secret_ref)
    if (auto) {
      draft.value.name = auto
      lastAutoName.value = auto
    }
  }
}

function addRenameRule() {
  if (!draft.value.config.rename_rules) {
    draft.value.config.rename_rules = []
  }
  draft.value.config.rename_rules.push({ pattern: '', replace: '' })
}

function removeRenameRule(index: number) {
  draft.value.config.rename_rules?.splice(index, 1)
}

function addFilterRule() {
  if (!draft.value.config.filter_rules) {
    draft.value.config.filter_rules = []
  }
  draft.value.config.filter_rules.push({ type: 'include', pattern: '' })
}

function removeFilterRule(index: number) {
  draft.value.config.filter_rules?.splice(index, 1)
}

function validate(): boolean {
  clearErrors()
  let valid = true

  const trimmedName = draft.value.name.trim()
  if (!trimmedName) {
    nameError.value = '订阅源名称不能为空'
    valid = false
  }

  const trimmedUrl = draft.value.source_url_secret_ref.trim()
  if (!trimmedUrl) {
    urlError.value = '订阅地址不能为空'
    valid = false
  }

  const interval = draft.value.refresh_policy.interval_seconds
  if (!interval || interval < 60) {
    generalError.value = '刷新间隔不能小于 60 秒'
    valid = false
  }

  const timeout = draft.value.refresh_policy.timeout_seconds
  if (!timeout || timeout < 5 || timeout > 120) {
    generalError.value = '超时时间必须在 5 到 120 秒之间'
    valid = false
  }

  if (probePolicyMode.value === 'custom') {
    const pInterval = Number(probeIntervalSeconds.value)
    if (isNaN(pInterval) || pInterval < 60 || pInterval > 604800) {
      generalError.value = '自定义探测周期必须在 60 到 604800 秒之间'
      valid = false
    }
  }

  if (!valid) {
    activeTab.value = 'basic'
    if (!trimmedName) {
      generalError.value = generalError.value || '请填写订阅源名称'
    } else if (!trimmedUrl) {
      generalError.value = generalError.value || '请填写订阅地址'
    }
  }

  return valid
}

function handleSubmit() {
  if (!validate()) {
    return
  }

  // Parse target groups comma-separated string
  const groups = targetGroupsInput.value
    .split(',')
    .map((g) => g.trim())
    .filter((g) => g.length > 0)
  draft.value.config.target_groups = groups

  if (probePolicyMode.value === 'custom') {
    draft.value.config.probe_policy = {
      mode: 'custom',
      interval_seconds: Number(probeIntervalSeconds.value),
    }
  } else {
    draft.value.config.probe_policy = {
      mode: probePolicyMode.value,
    }
  }

  initialSnapshot.value = getDraftSnapshot()
  emit('save', draft.value)
}
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="subscription ? t('subscriptions.configDrawerTitle') : t('subscriptions.add')"
    :description="t('subscriptions.configDrawerDesc')"
    width-class="max-w-2xl"
    @update:model-value="handleDrawerUpdateModelValue"
    @close="handleClose"
  >
    <!-- General Error Banner -->
    <div
      v-if="generalError"
      role="alert"
      class="p-3 rounded-xl bg-error/10 border border-error/30 text-error text-xs font-medium"
      data-testid="subscription-drawer-error"
    >
      {{ generalError }}
    </div>

    <!-- Progressive Disclosure Tabs -->
    <div class="tabs tabs-box bg-base-300/60 p-1 rounded-xl flex">
      <button
        type="button"
        class="tab tab-sm flex-1 font-semibold transition-all rounded-lg"
        :class="{ 'tab-active bg-primary text-primary-content shadow-xs': activeTab === 'basic' }"
        @click="activeTab = 'basic'"
      >
        {{ t('subscriptions.basicTab') }}
      </button>
      <button
        type="button"
        class="tab tab-sm flex-1 font-semibold transition-all rounded-lg"
        :class="{ 'tab-active bg-primary text-primary-content shadow-xs': activeTab === 'advanced' }"
        @click="activeTab = 'advanced'"
      >
        {{ t('subscriptions.advancedTab') }}
      </button>
    </div>

    <!-- Tab 1: Basic Settings (Progressive Disclosure) -->
    <form v-show="activeTab === 'basic'" class="space-y-4" @submit.prevent="handleSubmit">
      <div class="flex flex-col gap-1.5">
        <label for="sub-name-input" class="text-xs font-semibold text-base-content/80">
          {{ t('subscriptions.name') }}
        </label>
        <input
          id="sub-name-input"
          v-model="draft.name"
          required
          class="input input-sm w-full focus:input-primary font-medium"
          :class="{ 'input-error': nameError }"
          placeholder="例如：香港专线订阅源"
          data-testid="subscription-name-input"
          @input="onNameInput"
        />
        <span v-if="nameError" class="text-error text-[11px]" data-testid="subscription-name-error">
          {{ nameError }}
        </span>
      </div>

      <div class="flex flex-col gap-1.5">
        <label for="sub-prefix-input" class="text-xs font-semibold text-base-content/80">
          {{ t('subscriptions.prefixLabel') }}
        </label>
        <input
          id="sub-prefix-input"
          v-model="draft.prefix"
          class="input input-sm w-full focus:input-primary font-mono text-xs"
          :placeholder="t('subscriptions.prefixPlaceholder')"
          data-testid="subscription-prefix-input"
        />
        <span class="text-[11px] text-base-content/60">{{ t('subscriptions.prefixHint') }}</span>
      </div>

      <div class="flex flex-col gap-1.5">
        <label for="sub-url-input" class="text-xs font-semibold text-base-content/80">
          {{ t('subscriptions.sourceUrl') }}
        </label>
        <input
          id="sub-url-input"
          v-model="draft.source_url_secret_ref"
          required
          class="input input-sm w-full font-mono text-xs focus:input-primary"
          :class="{ 'input-error': urlError }"
          :placeholder="t('subscriptions.sourceUrlPlaceholder')"
          data-testid="subscription-url-input"
          @input="onUrlInput"
        />
        <span v-if="urlError" class="text-error text-[11px]" data-testid="subscription-url-error">
          {{ urlError }}
        </span>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div class="flex flex-col gap-1.5">
          <label for="sub-interval-input" class="text-xs font-semibold text-base-content/80">
            {{ t('subscriptions.refreshInterval') }}
          </label>
          <input
            id="sub-interval-input"
            v-model.number="draft.refresh_policy.interval_seconds"
            type="number"
            min="60"
            required
            class="input input-sm w-full font-mono"
          />
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="sub-timeout-input" class="text-xs font-semibold text-base-content/80">
            {{ t('subscriptions.timeout') }}
          </label>
          <input
            id="sub-timeout-input"
            v-model.number="draft.refresh_policy.timeout_seconds"
            type="number"
            min="5"
            max="120"
            required
            class="input input-sm w-full font-mono"
          />
        </div>
      </div>

      <div class="pt-2">
        <label class="flex items-center gap-3 bg-base-200/60 p-3 rounded-xl border border-base-300 cursor-pointer">
          <input v-model="draft.enabled" type="checkbox" class="toggle toggle-primary toggle-sm shrink-0" />
          <div class="flex flex-col">
            <span class="text-xs font-semibold text-base-content">{{ t('subscriptions.enabled') }}</span>
            <span class="text-[11px] text-base-content/60">将该订阅源纳入节点汇聚与自动同步</span>
          </div>
        </label>
      </div>
    </form>

    <!-- Tab 2: Advanced Settings (Fidelity preserved in the same draft) -->
    <div v-show="activeTab === 'advanced'" class="space-y-6">
      <!-- Cron Schedule -->
      <div class="p-4 rounded-xl bg-base-200/50 border border-base-300 space-y-1.5">
        <label for="sub-cron-input" class="block text-xs font-semibold text-base-content/80">
          {{ t('subscriptions.cronSchedule') }}
        </label>
        <input
          id="sub-cron-input"
          v-model="draft.config.cron_schedule"
          class="input input-sm w-full font-mono text-xs focus:input-primary"
          :placeholder="t('subscriptions.cronPlaceholder')"
        />
      </div>

      <!-- Probe Policy Card -->
      <div class="p-4 rounded-xl bg-base-200/50 border border-base-300 space-y-4" data-testid="subscription-probe-policy-section">
        <div>
          <span class="text-xs font-bold uppercase tracking-wider text-base-content/70">
            {{ t('subscriptions.probePolicyTitle') }}
          </span>
          <p class="text-[11px] text-base-content/60 mt-0.5">
            {{ t('subscriptions.probePolicyDesc') }}
          </p>
        </div>

        <div class="space-y-2">
          <label class="flex items-center gap-3 py-1 cursor-pointer">
            <input
              v-model="probePolicyMode"
              type="radio"
              value="inherit"
              name="probe_policy_mode"
              class="radio radio-primary radio-sm"
              data-testid="probe-policy-mode-inherit"
            />
            <div class="flex flex-col">
              <span class="text-xs font-medium">{{ t('subscriptions.probePolicyInherit') }}</span>
            </div>
          </label>

          <label class="flex items-center gap-3 py-1 cursor-pointer">
            <input
              v-model="probePolicyMode"
              type="radio"
              value="disabled"
              name="probe_policy_mode"
              class="radio radio-primary radio-sm"
              data-testid="probe-policy-mode-disabled"
            />
            <div class="flex flex-col">
              <span class="text-xs font-medium">{{ t('subscriptions.probePolicyDisabled') }}</span>
            </div>
          </label>

          <label class="flex items-center gap-3 py-1 cursor-pointer">
            <input
              v-model="probePolicyMode"
              type="radio"
              value="custom"
              name="probe_policy_mode"
              class="radio radio-primary radio-sm"
              data-testid="probe-policy-mode-custom"
            />
            <div class="flex flex-col">
              <span class="text-xs font-medium">{{ t('subscriptions.probePolicyCustom') }}</span>
            </div>
          </label>
        </div>

        <!-- Custom interval input visible only when mode is custom -->
        <div v-if="probePolicyMode === 'custom'" class="pt-2 border-t border-base-300 space-y-1.5" data-testid="probe-policy-custom-interval-block">
          <label for="sub-probe-interval-input" class="block text-xs font-semibold text-base-content/80">
            {{ t('subscriptions.probeIntervalSeconds') }}
          </label>
          <input
            id="sub-probe-interval-input"
            v-model.number="probeIntervalSeconds"
            type="number"
            min="60"
            max="604800"
            class="input input-sm font-mono text-xs focus:input-primary max-w-xs w-full"
            placeholder="3600"
            data-testid="probe-policy-interval-input"
          />
          <span class="text-[11px] text-base-content/60 block">{{ t('subscriptions.probeIntervalHint') }}</span>
        </div>
      </div>

      <!-- Rename Rules -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <span class="text-xs font-bold uppercase tracking-wider text-base-content/70">
            {{ t('subscriptions.renameRules') }}
          </span>
          <button type="button" class="btn btn-ghost btn-xs gap-1 text-primary" @click="addRenameRule">
            <PlusIcon class="w-3.5 h-3.5" /> {{ t('subscriptions.addRenameRule') }}
          </button>
        </div>

        <div v-if="!draft.config.rename_rules?.length" class="text-xs text-base-content/50 py-2 italic text-center border border-dashed border-base-300 rounded-lg">
          暂未配置节点正则重命名规则
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="(rule, idx) in draft.config.rename_rules"
            :key="idx"
            class="flex items-center gap-2 bg-base-200/80 p-2 rounded-lg border border-base-300"
          >
            <input
              v-model="rule.pattern"
              class="input input-xs flex-1 font-mono text-[11px]"
              :placeholder="t('subscriptions.pattern')"
            />
            <span class="text-xs text-base-content/50">&rarr;</span>
            <input
              v-model="rule.replace"
              class="input input-xs flex-1 font-mono text-[11px]"
              :placeholder="t('subscriptions.replace')"
            />
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-circle text-error"
              @click="removeRenameRule(idx)"
            >
              <TrashIcon class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>

      <!-- Filter Rules -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <div>
            <span class="text-xs font-bold uppercase tracking-wider text-base-content/70">
              {{ t('subscriptions.filterRules') }}
            </span>
            <p class="text-[11px] text-base-content/60 mt-0.5">
              仅通过筛选的节点才会存入节点池并参与探针探测。例如保留加拿大节点：选择 include 并填 <code>加拿大</code>。
            </p>
          </div>
          <button type="button" class="btn btn-ghost btn-xs gap-1 text-primary shrink-0" @click="addFilterRule">
            <PlusIcon class="w-3.5 h-3.5" /> {{ t('subscriptions.addFilterRule') }}
          </button>
        </div>

        <div v-if="!draft.config.filter_rules?.length" class="text-xs text-base-content/50 py-2 italic text-center border border-dashed border-base-300 rounded-lg">
          暂未配置节点黑白名单过滤规则
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="(rule, idx) in draft.config.filter_rules"
            :key="idx"
            class="flex items-center gap-2 bg-base-200/80 p-2 rounded-lg border border-base-300"
          >
            <select v-model="rule.type" class="select select-xs text-[11px]">
              <option value="include">{{ t('subscriptions.include') }}</option>
              <option value="exclude">{{ t('subscriptions.exclude') }}</option>
            </select>
            <input
              v-model="rule.pattern"
              class="input input-xs flex-1 font-mono text-[11px]"
              :placeholder="t('subscriptions.pattern')"
            />
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-circle text-error"
              @click="removeFilterRule(idx)"
            >
              <TrashIcon class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>

      <!-- Target Groups -->
      <div class="space-y-1.5">
        <label for="sub-target-groups-input" class="block text-xs font-semibold text-base-content/80">
          {{ t('subscriptions.targetGroups') }}
        </label>
        <input
          id="sub-target-groups-input"
          v-model="targetGroupsInput"
          class="input input-sm w-full font-mono text-xs focus:input-primary"
          :placeholder="t('subscriptions.targetGroupsPlaceholder')"
        />
        <span class="text-[11px] text-base-content/60 block">{{ t('subscriptions.targetGroupsDesc') }}</span>
      </div>
    </div>

    <!-- Sticky Footer Slot -->
    <template #footer>
      <div class="flex items-center justify-end gap-2 w-full">
        <button type="button" class="btn btn-ghost btn-sm touch-manipulation" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm touch-manipulation shadow-xs"
          :class="{ loading: saving }"
          :disabled="saving"
          data-testid="subscription-drawer-save-btn"
          @click="handleSubmit"
        >
          {{ t('common.save') }}
        </button>
      </div>
    </template>
  </DrawerCard>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import { matchedRouteKey, onBeforeRouteLeave } from 'vue-router'
import { PlusIcon, TrashIcon } from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import { t } from '../../locales'
import { uiText } from '../../locales/redesign'
import { displayText } from '../../utils/displayNames'
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
  (e: 'save', draft: SubscriptionDraft, settled: (error?: string) => void): void
}>()

const activeTab = ref<'basic' | 'advanced'>('basic')
const submitting = ref(false)
const busy = computed(() => props.saving || submitting.value)
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
let draftSession = 0

watch(
  () => [props.modelValue, props.subscription?.id] as const,
  ([open, id], previous) => {
    if (!open) { draftSession++; return }
    if (previous?.[0] && previous[1] === id) return
    draftSession++
    submitting.value = false
    activeTab.value = 'basic'
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
          rename_rules: sub.config?.rename_rules?.map((rule) => ({ ...rule })) ?? [],
          filter_rules: sub.config?.filter_rules?.map((rule) => ({ ...rule })) ?? [],
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

const routeRecord = inject(matchedRouteKey, null)
if (routeRecord?.value) onBeforeRouteLeave(() => !props.modelValue || !busy.value)

const unsavedGuard = useUnsavedChanges(
  isDirty,
  () => uiText('当前有未保存的订阅源修改，确定要放弃吗？', 'You have unsaved subscription changes. Are you sure you want to discard them?')
)

function handleClose() {
  if (busy.value || !unsavedGuard.confirmDiscard()) return
  clearErrors()
  emit('update:modelValue', false)
}

function handleDrawerUpdateModelValue(value: boolean) {
  if (!value) handleClose()
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
    nameError.value = uiText('订阅源名称不能为空', 'Subscription name is required')
    valid = false
  }

  const trimmedUrl = draft.value.source_url_secret_ref.trim()
  if (!trimmedUrl && !props.subscription) {
    urlError.value = uiText('订阅地址不能为空', 'Source URL is required')
    valid = false
  }

  const interval = draft.value.refresh_policy.interval_seconds
  if (!interval || interval < 60) {
    generalError.value = uiText('刷新间隔不能小于 60 秒', 'Refresh interval must be at least 60 seconds')
    valid = false
  }

  const timeout = draft.value.refresh_policy.timeout_seconds
  if (!timeout || timeout < 5 || timeout > 120) {
    generalError.value = uiText('超时时间必须在 5 到 120 秒之间', 'Timeout must be between 5 and 120 seconds')
    valid = false
  }

  if (probePolicyMode.value === 'custom') {
    const pInterval = Number(probeIntervalSeconds.value)
    if (isNaN(pInterval) || pInterval < 60 || pInterval > 604800) {
      generalError.value = uiText('自定义探测周期必须在 60 到 604800 秒之间', 'Custom probe interval must be between 60 and 604800 seconds')
      valid = false
    }
  }

  if (!valid) {
    activeTab.value = 'basic'
    if (!trimmedName) {
      generalError.value = generalError.value || uiText('请填写订阅源名称', 'Enter a subscription name')
    } else if (!trimmedUrl && !props.subscription) {
      generalError.value = generalError.value || uiText('请填写订阅地址', 'Enter a source URL')
    }
  }

  return valid
}

function handleSubmit() {
  if (busy.value || !validate()) {
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

  // Keep the request separate from the editable draft until persistence is acknowledged.
  const submitted = JSON.parse(JSON.stringify(draft.value)) as SubscriptionDraft
  submitted.name = submitted.name.trim()
  submitted.source_url_secret_ref = submitted.source_url_secret_ref.trim()
  submitting.value = true
  const session = draftSession
  let acknowledged = false
  emit('save', submitted, (error?: string) => {
    if (acknowledged || session !== draftSession) return
    acknowledged = true
    submitting.value = false
    if (error) {
      generalError.value = displayText(error)
      return
    }
    initialSnapshot.value = getDraftSnapshot()
  })
}
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="subscription ? t('subscriptions.configDrawerTitle') : t('subscriptions.add')"
    :description="t('subscriptions.configDrawerDesc')"
    width-class="max-w-2xl"
    @update:model-value="handleDrawerUpdateModelValue"
  >
    <!-- General Error Banner -->
    <div
      v-if="generalError"
      role="alert"
      class="p-3 rounded-xl bg-error/10 border border-error/30 text-error text-xs font-medium"
      data-testid="subscription-drawer-error"
    >
      {{ displayText(generalError) }}
    </div>

    <p v-if="busy" class="text-xs text-base-content/70" role="status">
      {{ uiText('正在保存，请稍候；输入和关闭已暂时锁定', 'Saving… Editing and closing are temporarily locked') }}
    </p>
    <fieldset :disabled="busy" class="min-w-0 space-y-5" :aria-busy="busy ? 'true' : undefined">
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
          :placeholder="uiText('例如：香港专线订阅源', 'For example: Hong Kong premium')"
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
          :required="!subscription"
          autocomplete="off"
          spellcheck="false"
          class="input input-sm w-full font-mono text-xs focus:input-primary"
          :class="{ 'input-error': urlError }"
          :placeholder="t('subscriptions.sourceUrlPlaceholder')"
          data-testid="subscription-url-input"
          @input="onUrlInput"
        />
        <span v-if="subscription" class="text-[11px] text-base-content/60">
          {{ uiText('留空保留现有订阅来源；只有填写新地址才会替换', 'Leave blank to keep the existing source; enter a new URL only to replace it') }}
        </span>
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
            <span class="text-[11px] text-base-content/60">{{ uiText('将该订阅源纳入节点汇聚与自动同步', 'Include this source in node aggregation and automatic sync') }}</span>
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
          {{ uiText('暂未配置节点正则重命名规则', 'No node rename rules configured') }}
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="(rule, idx) in draft.config.rename_rules"
            :key="idx"
            class="flex items-center gap-2 bg-base-200/80 p-2 rounded-lg border border-base-300"
          >
            <input
              v-model="rule.pattern"
              class="input input-xs min-w-0 flex-1 font-mono text-[11px]"
              :aria-label="uiText('重命名匹配正则', 'Rename match pattern')"
              :placeholder="t('subscriptions.pattern')"
            />
            <span class="text-xs text-base-content/50">&rarr;</span>
            <input
              v-model="rule.replace"
              class="input input-xs min-w-0 flex-1 font-mono text-[11px]"
              :aria-label="uiText('重命名替换文本', 'Rename replacement')"
              :placeholder="t('subscriptions.replace')"
            />
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-circle text-error"
              :aria-label="uiText('删除重命名规则', 'Remove rename rule')"
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
              {{ uiText('仅通过筛选的节点才会存入节点池并参与探测。例如选择 include 并填写 Canada，仅保留匹配的节点', 'Only matching nodes enter inventory and probing. For example, use include with Canada to keep matching nodes') }}
            </p>
          </div>
          <button type="button" class="btn btn-ghost btn-xs gap-1 text-primary shrink-0" @click="addFilterRule">
            <PlusIcon class="w-3.5 h-3.5" /> {{ t('subscriptions.addFilterRule') }}
          </button>
        </div>

        <div v-if="!draft.config.filter_rules?.length" class="text-xs text-base-content/50 py-2 italic text-center border border-dashed border-base-300 rounded-lg">
          {{ uiText('暂未配置节点黑白名单过滤规则', 'No node filter rules configured') }}
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="(rule, idx) in draft.config.filter_rules"
            :key="idx"
            class="flex items-center gap-2 bg-base-200/80 p-2 rounded-lg border border-base-300"
          >
            <select v-model="rule.type" class="select select-xs w-24 shrink-0 text-[11px]" :aria-label="uiText('筛选方式', 'Filter mode')">
              <option value="include">{{ t('subscriptions.include') }}</option>
              <option value="exclude">{{ t('subscriptions.exclude') }}</option>
            </select>
            <input
              v-model="rule.pattern"
              class="input input-xs min-w-0 flex-1 font-mono text-[11px]"
              :aria-label="uiText('筛选匹配正则', 'Filter match pattern')"
              :placeholder="t('subscriptions.pattern')"
            />
            <button
              type="button"
              class="btn btn-ghost btn-xs btn-circle text-error"
              :aria-label="uiText('删除筛选规则', 'Remove filter rule')"
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

    </fieldset>

    <!-- Sticky Footer Slot -->
    <template #footer>
      <div class="flex items-center justify-end gap-2 w-full">
        <button type="button" class="btn btn-ghost btn-sm touch-manipulation" :disabled="busy" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm touch-manipulation shadow-xs"
          :class="{ loading: busy }"
          :disabled="busy"
          data-testid="subscription-drawer-save-btn"
          @click="handleSubmit"
        >
          {{ t('common.save') }}
        </button>
      </div>
    </template>
  </DrawerCard>
</template>

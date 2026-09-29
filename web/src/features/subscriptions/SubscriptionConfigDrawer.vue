<script setup lang="ts">
import { ref, watch } from 'vue'
import { PlusIcon, TrashIcon } from '@heroicons/vue/24/outline'
import DrawerCard from '../../ui/DrawerCard.vue'
import { t } from '../../locales'
import {
  defaultPolicy,
  defaultConfig,
  type SubscriptionDraft,
  type SubscriptionRecord,
  type RenameRule,
  type FilterRule,
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
  refresh_policy: defaultPolicy(),
  config: defaultConfig(),
})

const targetGroupsInput = ref('')
const nameError = ref('')
const urlError = ref('')
const generalError = ref('')

function clearErrors() {
  nameError.value = ''
  urlError.value = ''
  generalError.value = ''
}

watch(
  () => props.subscription,
  (sub) => {
    clearErrors()
    if (sub) {
      draft.value = {
        name: sub.name,
        source_url_secret_ref: sub.source_url_secret_ref || '',
        enabled: sub.enabled,
        refresh_policy: { ...defaultPolicy(), ...sub.refresh_policy },
        config: {
          cron_schedule: sub.config?.cron_schedule || '',
          auto_test: !!sub.config?.auto_test,
          rename_rules: sub.config?.rename_rules ? [...sub.config.rename_rules] : [],
          filter_rules: sub.config?.filter_rules ? [...sub.config.filter_rules] : [],
          target_groups: sub.config?.target_groups ? [...sub.config.target_groups] : [],
        },
      }
      targetGroupsInput.value = (sub.config?.target_groups || []).join(', ')
    } else {
      draft.value = {
        name: '',
        source_url_secret_ref: '',
        enabled: true,
        refresh_policy: defaultPolicy(),
        config: defaultConfig(),
      }
      targetGroupsInput.value = ''
    }
  },
  { immediate: true }
)

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

function handleClose() {
  clearErrors()
  emit('update:modelValue', false)
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

  emit('save', draft.value)
}
</script>

<template>
  <DrawerCard
    :model-value="modelValue"
    :title="subscription ? t('subscriptions.configDrawerTitle') : t('subscriptions.add')"
    :description="t('subscriptions.configDrawerDesc')"
    width-class="max-w-2xl"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <!-- Tab Selector -->
    <div
      v-if="generalError"
      role="alert"
      class="p-3 rounded-xl bg-error/10 border border-error/30 text-error text-xs font-medium"
      data-testid="subscription-drawer-error"
    >
      {{ generalError }}
    </div>

    <div class="tabs tabs-boxed bg-base-300/60 p-1 rounded-xl">
      <button
        type="button"
        class="tab tab-sm flex-1 font-semibold transition-all"
        :class="{ 'tab-active bg-primary text-primary-content shadow': activeTab === 'basic' }"
        @click="activeTab = 'basic'"
      >
        {{ t('subscriptions.basicTab') }}
      </button>
      <button
        type="button"
        class="tab tab-sm flex-1 font-semibold transition-all"
        :class="{ 'tab-active bg-primary text-primary-content shadow': activeTab === 'advanced' }"
        @click="activeTab = 'advanced'"
      >
        {{ t('subscriptions.advancedTab') }}
      </button>
    </div>

    <!-- Tab 1: Basic Settings -->
    <form v-show="activeTab === 'basic'" class="space-y-4" @submit.prevent="handleSubmit">
      <label class="form-control">
        <span class="label-text text-xs font-semibold mb-1">{{ t('subscriptions.name') }}</span>
        <input
          v-model="draft.name"
          required
          class="input input-bordered input-sm focus:input-primary font-medium"
          :class="{ 'input-error': nameError }"
          placeholder="例如：香港专线订阅源"
          data-testid="subscription-name-input"
          @input="nameError = ''; generalError = ''"
        />
        <span v-if="nameError" class="text-error text-[11px] mt-1" data-testid="subscription-name-error">
          {{ nameError }}
        </span>
      </label>

      <label class="form-control">
        <div class="flex justify-between items-center mb-1">
          <span class="label-text text-xs font-semibold">{{ t('subscriptions.sourceUrl') }}</span>
        </div>
        <input
          v-model="draft.source_url_secret_ref"
          required
          class="input input-bordered input-sm font-mono text-xs focus:input-primary"
          :class="{ 'input-error': urlError }"
          :placeholder="t('subscriptions.sourceUrlPlaceholder')"
          data-testid="subscription-url-input"
          @input="urlError = ''; generalError = ''"
        />
        <span v-if="urlError" class="text-error text-[11px] mt-1" data-testid="subscription-url-error">
          {{ urlError }}
        </span>
      </label>

      <div class="grid grid-cols-2 gap-4">
        <label class="form-control">
          <span class="label-text text-xs font-semibold mb-1">{{ t('subscriptions.refreshInterval') }}</span>
          <input
            v-model.number="draft.refresh_policy.interval_seconds"
            type="number"
            min="60"
            required
            class="input input-bordered input-sm font-mono"
          />
        </label>
        <label class="form-control">
          <span class="label-text text-xs font-semibold mb-1">{{ t('subscriptions.timeout') }}</span>
          <input
            v-model.number="draft.refresh_policy.timeout_seconds"
            type="number"
            min="5"
            max="120"
            required
            class="input input-bordered input-sm font-mono"
          />
        </label>
      </div>

      <div class="pt-2">
        <label class="label cursor-pointer justify-start gap-3 bg-base-200/60 p-3 rounded-xl border border-base-300">
          <input v-model="draft.enabled" type="checkbox" class="toggle toggle-primary toggle-sm" />
          <div class="flex flex-col">
            <span class="label-text text-xs font-semibold">{{ t('subscriptions.enabled') }}</span>
            <span class="text-[11px] text-base-content/60">将该订阅源纳入节点汇聚与自动同步</span>
          </div>
        </label>
      </div>
    </form>

    <!-- Tab 2: Advanced Settings -->
    <div v-show="activeTab === 'advanced'" class="space-y-6">
      <!-- Cron Schedule & Auto Test -->
      <div class="p-4 rounded-xl bg-base-200/50 border border-base-300 space-y-4">
        <label class="form-control">
          <span class="label-text text-xs font-semibold mb-1">{{ t('subscriptions.cronSchedule') }}</span>
          <input
            v-model="draft.config.cron_schedule"
            class="input input-bordered input-sm font-mono text-xs focus:input-primary"
            :placeholder="t('subscriptions.cronPlaceholder')"
          />
        </label>

        <label class="label cursor-pointer justify-start gap-3 pt-2">
          <input v-model="draft.config.auto_test" type="checkbox" class="toggle toggle-secondary toggle-sm" />
          <span class="label-text text-xs font-semibold">{{ t('subscriptions.autoTest') }}</span>
        </label>
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
              class="input input-bordered input-xs flex-1 font-mono text-[11px]"
              :placeholder="t('subscriptions.pattern')"
            />
            <span class="text-xs text-base-content/50">&rarr;</span>
            <input
              v-model="rule.replace"
              class="input input-bordered input-xs flex-1 font-mono text-[11px]"
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
          <span class="text-xs font-bold uppercase tracking-wider text-base-content/70">
            {{ t('subscriptions.filterRules') }}
          </span>
          <button type="button" class="btn btn-ghost btn-xs gap-1 text-primary" @click="addFilterRule">
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
            <select v-model="rule.type" class="select select-bordered select-xs text-[11px]">
              <option value="include">{{ t('subscriptions.include') }}</option>
              <option value="exclude">{{ t('subscriptions.exclude') }}</option>
            </select>
            <input
              v-model="rule.pattern"
              class="input input-bordered input-xs flex-1 font-mono text-[11px]"
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
      <div class="space-y-2">
        <label class="form-control">
          <span class="label-text text-xs font-semibold mb-1">{{ t('subscriptions.targetGroups') }}</span>
          <input
            v-model="targetGroupsInput"
            class="input input-bordered input-sm font-mono text-xs focus:input-primary"
            :placeholder="t('subscriptions.targetGroupsPlaceholder')"
          />
          <span class="text-[11px] text-base-content/60 mt-1">{{ t('subscriptions.targetGroupsDesc') }}</span>
        </label>
      </div>
    </div>

    <!-- Sticky Footer Slot -->
    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <button type="button" class="btn btn-ghost btn-sm touch-manipulation" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary btn-sm touch-manipulation shadow-sm"
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

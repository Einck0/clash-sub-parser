<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowPathIcon,
  ExclamationTriangleIcon,
  InformationCircleIcon,
  CheckCircleIcon,
} from '@heroicons/vue/24/outline'
import Drawer from '../../ui/Drawer.vue'
import { uiText } from '../../locales/redesign'
import type { UseChainDraftReturn } from './useChainDraft'
import type { OptionsResult, OptionItem } from './types'

const props = defineProps<{
  draftManager: UseChainDraftReturn
  options: OptionsResult
  currentConfigVersion: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'reload'): void
  (e: 'rebase'): void
}>()

const {
  isOpen,
  isEditing,
  saving,
  saveError,
  conflictState,
  draft,
  isDirty,
  previewLoading,
  previewError,
  previewEffects,
  previewNodes,
  isPreviewStale,
  setTargetType,
  setDialerType,
  setTargetId,
  setDialerId,
  setEnabled,
  setSortOrder,
  setNote,
  rebaseAndKeepDraft,
  triggerPreview,
  save,
} = props.draftManager

const drawerTitle = computed(() => {
  return isEditing.value
    ? uiText('编辑链式代理绑定', 'Edit Proxy Chain Binding')
    : uiText('新建链式代理绑定', 'Create Proxy Chain Binding')
})

const drawerDesc = computed(() => {
  return uiText(
    '配置出口目标（订阅、策略组或单节点）的前置跳板节点或策略组',
    'Configure upstream dialer proxy for an egress target (subscription, group, or node)'
  )
})

// Target options based on target_type
const targetOptions = computed<OptionItem[]>(() => {
  if (draft.value.target_type === 'subscription') {
    return props.options.subscriptions || []
  }
  if (draft.value.target_type === 'node_group') {
    return props.options.groups || []
  }
  if (draft.value.target_type === 'node') {
    return props.options.nodes || []
  }
  return []
})

// Check if selected target_id is missing from options
const isTargetMissing = computed(() => {
  if (!draft.value.target_id) return false
  return !targetOptions.value.some((o) => o.id === draft.value.target_id)
})

// Dialer options based on dialer_type
const dialerOptions = computed<OptionItem[]>(() => {
  if (draft.value.dialer_type === 'node_group') {
    return props.options.groups || []
  }
  if (draft.value.dialer_type === 'node') {
    return props.options.nodes || []
  }
  return []
})

// Check if selected dialer_id is missing from options
const isDialerMissing = computed(() => {
  if (!draft.value.dialer_id) return false
  return !dialerOptions.value.some((o) => o.id === draft.value.dialer_id)
})

const hasMissingReference = computed(() => {
  return isTargetMissing.value || isDialerMissing.value
})

function handleTargetTypeChange(e: Event) {
  const select = e.target as HTMLSelectElement
  setTargetType(select.value as any)
}

function handleDialerTypeChange(e: Event) {
  const select = e.target as HTMLSelectElement
  setDialerType(select.value as any)
}

function handleTargetIdChange(e: Event) {
  const select = e.target as HTMLSelectElement
  setTargetId(select.value)
}

function handleDialerIdChange(e: Event) {
  const select = e.target as HTMLSelectElement
  setDialerId(select.value)
}

function handleSortOrderChange(e: Event) {
  const input = e.target as HTMLInputElement
  setSortOrder(Number(input.value) || 0)
}

function handleNoteChange(e: Event) {
  const input = e.target as HTMLInputElement
  setNote(input.value)
}

function handleToggleEnabled() {
  setEnabled(!draft.value.enabled)
}

function handleRebase() {
  emit('rebase')
}

function handleReloadConfirm() {
  if (isDirty.value) {
    const ok = window.confirm(
      uiText(
        '重新加载将丢失当前所有未保存修改，确定继续吗？',
        'Reloading will discard all unsaved edits. Are you sure?'
      )
    )
    if (!ok) return
  }
  emit('reload')
}

async function handleSaveClick() {
  await save()
}
</script>

<template>
  <Drawer
    :model-value="isOpen"
    :title="drawerTitle"
    :description="drawerDesc"
    width-class="md:max-w-xl"
    @update:model-value="(val: boolean) => { if (!val) $emit('close') }"
    @close="$emit('close')"
  >
    <div class="space-y-5" data-testid="chain-drawer">
      <!-- 409 Conflict Banner -->
      <div
        v-if="conflictState"
        class="alert alert-warning shadow-md rounded-2xl p-4 flex flex-col gap-3"
        data-testid="chain-conflict-alert"
      >
        <div class="flex items-start gap-3">
          <ExclamationTriangleIcon class="w-6 h-6 shrink-0 text-warning" />
          <div class="min-w-0 flex-1">
            <h4 class="text-sm font-bold text-base-content">
              {{ uiText('并发修改冲突 (409 Conflict)', 'Concurrent Conflict (409)') }}
            </h4>
            <p class="text-xs text-base-content/80 mt-1 leading-relaxed">
              {{ conflictState.message }}
            </p>
            <p class="text-xs text-base-content/60 mt-1">
              {{
                uiText(
                  '您的编辑草稿已原样保留。您可以选择保留草稿并基于最新版本重试，或放弃修改重新加载。',
                  'Your draft is preserved. You can rebase and keep draft, or reload from server.'
                )
              }}
            </p>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2 justify-end pt-2 border-t border-base-content/10">
          <button
            type="button"
            class="btn btn-xs btn-ghost"
            data-testid="chain-conflict-reload-btn"
            @click="handleReloadConfirm"
          >
            {{ uiText('放弃修改并重新加载', 'Discard & Reload') }}
          </button>
          <button
            type="button"
            class="btn btn-xs btn-primary font-semibold"
            data-testid="chain-conflict-rebase-btn"
            @click="handleRebase"
          >
            {{ uiText('保留草稿并基于新版本重试', 'Keep Draft & Rebase') }}
          </button>
        </div>
      </div>

      <!-- Save Error Banner -->
      <div
        v-if="saveError"
        class="alert alert-error shadow-sm rounded-xl p-3 text-xs flex items-center gap-2"
        data-testid="chain-save-error"
      >
        <ExclamationTriangleIcon class="w-4 h-4 shrink-0" />
        <span class="flex-1 font-medium">{{ saveError }}</span>
      </div>

      <!-- Priority Information Card -->
      <div class="bg-base-200/60 border border-base-300/50 rounded-2xl p-3.5 space-y-2">
        <div class="flex items-center gap-2 text-xs font-semibold text-primary">
          <InformationCircleIcon class="w-4 h-4 shrink-0" />
          <span>{{ uiText('优先级规则说明', 'Priority Rules') }}</span>
        </div>
        <p class="text-xs text-base-content/70 leading-relaxed">
          {{
            uiText(
              '出口链式代理按范围优先级生效：单节点 (Node) > 策略组 (Group) > 订阅 (Subscription)。同层级范围时，排序值较小 (sort_order 小) 者优先；排序相同时较小 ID 优先。',
              'Egress proxy chains take effect in hierarchical order: Node > Group > Subscription. At the same level, smaller sort_order takes precedence, followed by smaller ID.'
            )
          }}
        </p>
      </div>

      <!-- Missing Reference Repair Notice -->
      <div
        v-if="hasMissingReference"
        class="bg-warning/10 border border-warning/30 rounded-2xl p-3 text-xs text-warning-content space-y-1"
        data-testid="chain-missing-notice"
      >
        <span class="font-bold flex items-center gap-1.5">
          <ExclamationTriangleIcon class="w-4 h-4 text-warning shrink-0" />
          {{ uiText('检测到失效引用', 'Missing Reference Detected') }}
        </span>
        <p class="text-base-content/70">
          {{
            uiText(
              '当前绑定的出口目标或跳板对象已在系统中失效或被删除。失效值已原样保留。您可以更换有效实体，或者将本绑定置为【停用】状态后保存以供后续修复。',
              'The referenced target or dialer entity no longer exists. Value is retained. You can choose a new entity, or disable this binding to repair and save.'
            )
          }}
        </p>
      </div>

      <!-- Form Grid -->
      <div class="space-y-4">
        <!-- Target Type & ID -->
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div class="form-control">
            <label class="label py-1 text-xs font-medium text-base-content/80">
              {{ uiText('目标范围', 'Target Type') }}
            </label>
            <select
              :value="draft.target_type"
              class="select select-bordered select-sm w-full"
              data-testid="chain-target-type"
              @change="handleTargetTypeChange"
            >
              <option value="subscription">{{ uiText('订阅 (Subscription)', 'Subscription') }}</option>
              <option value="node_group">{{ uiText('策略组 (Group)', 'Node Group') }}</option>
              <option value="node">{{ uiText('单节点 (Node)', 'Single Node') }}</option>
            </select>
          </div>

          <div class="form-control sm:col-span-2">
            <label class="label py-1 text-xs font-medium text-base-content/80 flex items-center justify-between">
              <span>{{ uiText('出口目标实体', 'Target Entity') }}</span>
              <span v-if="isTargetMissing" class="badge badge-warning badge-xs">
                {{ uiText('已失效', 'Missing Ref') }}
              </span>
            </label>
            <select
              :value="draft.target_id"
              class="select select-bordered select-sm w-full"
              :class="{ 'select-warning': isTargetMissing }"
              data-testid="chain-target-id"
              @change="handleTargetIdChange"
            >
              <option v-if="!draft.target_id" value="" disabled>
                {{ uiText('-- 请选择出口目标 --', '-- Select Target --') }}
              </option>
              <option v-if="isTargetMissing" :value="draft.target_id" disabled>
                [{{ uiText('已失效引用', 'Missing Ref') }}: {{ draft.target_id }}]
              </option>
              <option
                v-for="opt in targetOptions"
                :key="opt.id"
                :value="opt.id"
              >
                {{ opt.name }} ({{ opt.id }})
              </option>
            </select>
          </div>
        </div>

        <!-- Dialer Type & ID -->
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div class="form-control">
            <label class="label py-1 text-xs font-medium text-base-content/80">
              {{ uiText('跳板类型', 'Dialer Type') }}
            </label>
            <select
              :value="draft.dialer_type"
              class="select select-bordered select-sm w-full"
              data-testid="chain-dialer-type"
              @change="handleDialerTypeChange"
            >
              <option value="node_group">{{ uiText('策略组跳板', 'Group Dialer') }}</option>
              <option value="node">{{ uiText('节点跳板', 'Node Dialer') }}</option>
            </select>
          </div>

          <div class="form-control sm:col-span-2">
            <label class="label py-1 text-xs font-medium text-base-content/80 flex items-center justify-between">
              <span>{{ uiText('前置跳板实体', 'Dialer Entity') }}</span>
              <span v-if="isDialerMissing" class="badge badge-warning badge-xs">
                {{ uiText('已失效', 'Missing Ref') }}
              </span>
            </label>
            <select
              :value="draft.dialer_id"
              class="select select-bordered select-sm w-full"
              :class="{ 'select-warning': isDialerMissing }"
              data-testid="chain-dialer-id"
              @change="handleDialerIdChange"
            >
              <option v-if="!draft.dialer_id" value="" disabled>
                {{ uiText('-- 请选择跳板实体 --', '-- Select Dialer --') }}
              </option>
              <option v-if="isDialerMissing" :value="draft.dialer_id" disabled>
                [{{ uiText('已失效引用', 'Missing Ref') }}: {{ draft.dialer_id }}]
              </option>
              <option
                v-for="opt in dialerOptions"
                :key="opt.id"
                :value="opt.id"
              >
                {{ opt.name }} ({{ opt.id }})
              </option>
            </select>
          </div>
        </div>

        <!-- Sort Order & Enabled -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 items-center">
          <div class="form-control">
            <label class="label py-1 text-xs font-medium text-base-content/80">
              {{ uiText('同级排序优先级 (越小越优先)', 'Sort Order (Lower is higher priority)') }}
            </label>
            <input
              type="number"
              :value="draft.sort_order"
              min="0"
              max="9999"
              step="1"
              class="input input-bordered input-sm w-full"
              data-testid="chain-sort-order"
              @input="handleSortOrderChange"
            />
          </div>

          <div class="form-control pt-4 sm:pt-6">
            <label class="cursor-pointer label justify-start gap-3">
              <input
                type="checkbox"
                class="toggle toggle-primary toggle-sm"
                :checked="draft.enabled"
                data-testid="chain-enabled"
                @change="handleToggleEnabled"
              />
              <span class="label-text text-xs font-semibold">
                {{ draft.enabled ? uiText('启用链式代理', 'Chain Enabled') : uiText('停用链式代理', 'Chain Disabled') }}
              </span>
            </label>
          </div>
        </div>

        <!-- Note -->
        <div class="form-control">
          <label class="label py-1 text-xs font-medium text-base-content/80">
            {{ uiText('备注说明 (可选)', 'Note / Description (Optional)') }}
          </label>
          <input
            type="text"
            :value="draft.note"
            placeholder="例如：专线落地前置跳板"
            maxlength="200"
            class="input input-bordered input-sm w-full"
            data-testid="chain-note"
            @input="handleNoteChange"
          />
        </div>
      </div>

      <!-- Live Preview Section -->
      <div
        class="border border-base-300 rounded-2xl p-4 bg-base-200/40 space-y-3"
        data-testid="chain-preview-card"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <h4 class="text-xs font-bold text-base-content tracking-wide uppercase">
              {{ uiText('实时效果预览 (250ms 防抖)', 'Live Preview (250ms Debounced)') }}
            </h4>
            <span v-if="previewLoading" class="loading loading-spinner loading-xs text-primary" />
          </div>

          <!-- Stale Status / Retry -->
          <div v-if="isPreviewStale" class="flex items-center gap-2" data-testid="chain-preview-stale">
            <span class="badge badge-warning badge-xs">{{ uiText('预览已过期', 'Stale') }}</span>
            <button
              type="button"
              class="btn btn-ghost btn-xs gap-1"
              data-testid="chain-preview-retry"
              @click="triggerPreview(true)"
            >
              <ArrowPathIcon class="w-3 h-3" />
              <span>{{ uiText('重试', 'Retry') }}</span>
            </button>
          </div>
        </div>

        <!-- Preview Error Notice -->
        <div
          v-if="previewError"
          class="alert alert-warning shadow-sm rounded-xl p-2.5 text-xs flex items-start gap-2"
          data-testid="chain-preview-error"
        >
          <ExclamationTriangleIcon class="w-4 h-4 shrink-0 text-warning mt-0.5" />
          <span class="break-words font-medium leading-relaxed">{{ previewError }}</span>
        </div>

        <!-- Preview Effects Stats -->
        <div v-else-if="previewEffects.length > 0" class="space-y-3">
          <div
            v-for="eff in previewEffects"
            :key="eff.binding_id"
            class="bg-base-100 p-3 rounded-xl border border-base-300/50 space-y-2 text-xs"
          >
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-center">
              <div class="bg-base-200/50 p-2 rounded-lg">
                <div class="text-[10px] text-base-content/60">{{ uiText('覆盖目标', 'Targets') }}</div>
                <div class="font-bold text-sm text-base-content">{{ eff.target_count }}</div>
              </div>
              <div class="bg-success/10 p-2 rounded-lg text-success">
                <div class="text-[10px] opacity-80">{{ uiText('成功生效', 'Winning') }}</div>
                <div class="font-bold text-sm">{{ eff.chain_count }}</div>
              </div>
              <div class="bg-warning/10 p-2 rounded-lg text-warning">
                <div class="text-[10px] opacity-80">{{ uiText('重合跳过', 'Skipped') }}</div>
                <div class="font-bold text-sm">{{ eff.skip_count }}</div>
              </div>
              <div class="bg-info/10 p-2 rounded-lg text-info">
                <div class="text-[10px] opacity-80">{{ uiText('被高优覆盖', 'Overridden') }}</div>
                <div class="font-bold text-sm">{{ eff.overridden_count }}</div>
              </div>
            </div>

            <!-- Samples -->
            <div v-if="eff.chain_samples && eff.chain_samples.length > 0" class="space-y-1">
              <div class="text-[11px] font-semibold text-success flex items-center gap-1">
                <CheckCircleIcon class="w-3.5 h-3.5 shrink-0" />
                <span>{{ uiText('生效节点样本', 'Winning Samples') }}:</span>
              </div>
              <div class="flex flex-wrap gap-1">
                <span
                  v-for="s in eff.chain_samples"
                  :key="s"
                  class="badge badge-success badge-sm badge-outline"
                >
                  {{ s }}
                </span>
              </div>
            </div>

            <div v-if="eff.skip_samples && eff.skip_samples.length > 0" class="space-y-1">
              <div class="text-[11px] font-semibold text-warning flex items-center gap-1">
                <ExclamationTriangleIcon class="w-3.5 h-3.5 shrink-0" />
                <span>{{ uiText('跳过节点样本 (重合/自指/环路)', 'Skipped Samples (Overlap/Self)') }}:</span>
              </div>
              <div class="flex flex-wrap gap-1">
                <span
                  v-for="s in eff.skip_samples"
                  :key="s"
                  class="badge badge-warning badge-sm badge-outline"
                >
                  {{ s }}
                </span>
              </div>
            </div>

            <!-- Diagnostics -->
            <div v-if="eff.diagnostics && eff.diagnostics.length > 0" class="space-y-1 pt-1 border-t border-base-200">
              <div
                v-for="(d, i) in eff.diagnostics"
                :key="i"
                class="text-[11px] text-warning flex items-center gap-1.5"
              >
                <span>•</span>
                <span>{{ d.message }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-else class="text-xs text-base-content/50 py-2 text-center">
          {{ uiText('暂无展开节点预览', 'No preview nodes available') }}
        </div>
      </div>
    </div>

    <!-- Drawer Footer Actions -->
    <template #footer>
      <div class="flex items-center justify-between w-full">
        <button
          type="button"
          class="btn btn-ghost btn-sm"
          data-testid="chain-cancel-btn"
          @click="$emit('close')"
        >
          {{ uiText('取消', 'Cancel') }}
        </button>

        <button
          type="button"
          class="btn btn-primary btn-sm px-5 font-semibold gap-2"
          :class="{ loading: saving }"
          :disabled="saving"
          data-testid="chain-save-btn"
          @click="handleSaveClick"
        >
          <span v-if="saving" class="loading loading-spinner loading-xs" />
          <span>{{ uiText('保存绑定', 'Save Binding') }}</span>
        </button>
      </div>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  ArrowPathIcon,
  PlusIcon,
  LinkIcon,
  PencilSquareIcon,
  TrashIcon,
  ExclamationTriangleIcon,
  MagnifyingGlassIcon,
} from '@heroicons/vue/24/outline'
import SettingsSections from '../settings/SettingsSections.vue'
import PageToolbar from '../../ui/PageToolbar.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import SectionState from '../../ui/SectionState.vue'
import { useUnsavedChanges } from '../../composables/useUnsavedChanges'
import { uiText } from '../../locales/redesign'
import { useChains } from './useChains'
import { useChainDraft } from './useChainDraft'
import ChainDrawer from './ChainDrawer.vue'
import ChainEffectLedger from './ChainEffectLedger.vue'
import { displayText } from '../../utils/displayNames'
import {
  getTargetDisplayName,
  getDialerDisplayName,
  getTargetTypeLabel,
  getDialerTypeLabel,
  getPriorityRank,
  type ProxyChainBinding,
} from './types'

const chains = useChains()
const optionsVerified = computed(() => chains.optionsLoaded.value && !chains.optionsLoading.value && !chains.optionsError.value)
const recovering = ref(false)
const recoveryError = ref<string | null>(null)
const bindingSearch = ref('')
const bindingStatus = ref('all')
const bindingLimit = ref(50)
const bindingRows = computed(() => chains.bindings.value.map((binding) => ({
  binding,
  target: getTargetDisplayName(binding.target_type, binding.target_id, chains.options.value, optionsVerified.value),
  dialer: getDialerDisplayName(binding.dialer_type, binding.dialer_id, chains.options.value, optionsVerified.value),
})))
const filteredBindings = computed(() => {
  const query = bindingSearch.value.trim().toLocaleLowerCase()
  return bindingRows.value.filter(({ binding, target, dialer }) => {
    if (bindingStatus.value === 'enabled' && !binding.enabled) return false
    if (bindingStatus.value === 'disabled' && binding.enabled) return false
    return !query || [target.name, dialer.name, binding.note, getTargetTypeLabel(binding.target_type), getDialerTypeLabel(binding.dialer_type)]
      .some((value) => value.toLocaleLowerCase().includes(query))
  })
})
const visibleBindings = computed(() => filteredBindings.value.slice(0, bindingLimit.value))
watch([bindingSearch, bindingStatus], () => { bindingLimit.value = 50 })

function openCreate() {
  recoveryError.value = null
  draftManager.openCreate()
}
function openEdit(binding: ProxyChainBinding) {
  recoveryError.value = null
  draftManager.openEdit(binding)
}
function handleToggle(event: Event, binding: ProxyChainBinding) {
  // Keep the control on the persisted value until the server confirms the change.
  const input = event.target as HTMLInputElement
  input.checked = binding.enabled
  void chains.toggleEnabled(binding)
}

const draftManager = useChainDraft({
  getConfigVersion: () => chains.configVersion.value,
  getOptions: () => chains.options.value,
  onSaved: (binding, newVer) => {
    chains.onBindingSaved(binding, newVer)
  },
})

// Guard against accidental navigation when drawer draft is dirty
useUnsavedChanges(draftManager.isDirty, () =>
  uiText(
    '当前链式代理草稿有未保存的修改，离开将丢失这些草稿。确定要离开吗？',
    'You have unsaved changes in your proxy chain draft. Leaving will discard them. Are you sure?'
  )
)

function handleDrawerClose() {
  if (draftManager.saving.value || recovering.value) return
  if (draftManager.isDirty.value) {
    const ok = window.confirm(
      uiText(
        '当前草稿有未保存的修改，确定关闭并放弃吗？',
        'Discard unsaved draft changes and close?'
      )
    )
    if (!ok) return
  }
  draftManager.close()
}

async function handleDrawerReload() {
  if (recovering.value || draftManager.saving.value) return
  recovering.value = true
  recoveryError.value = null
  try {
    if (await chains.loadAll()) {
      draftManager.close()
    } else {
      recoveryError.value = uiText('重新加载失败，草稿未丢弃。', 'Reload failed; your draft is preserved.') + ' ' +
        (chains.error.value || chains.optionsError.value || chains.effectsError.value || '')
    }
  } finally {
    recovering.value = false
  }
}

async function handleDrawerRebase() {
  if (recovering.value || draftManager.saving.value) return
  recovering.value = true
  recoveryError.value = null
  try {
    if (!await chains.loadBindings()) {
      recoveryError.value = uiText('未能读取最新版本，草稿和原修订号已保留，请重试。', 'Unable to read the latest version. Your draft and original revision are preserved; retry.') + ' ' + (chains.error.value || '')
      return
    }
    const matching = chains.bindings.value.find((b) => b.id === draftManager.draft.value.id)
    if (draftManager.isEditing.value && !matching) {
      recoveryError.value = uiText('此绑定已被删除，草稿仍保留。请复制所需内容后关闭并新建。', 'This binding was deleted. Your draft is preserved; copy any needed values before closing and creating a new binding.')
      return
    }
    draftManager.rebaseAndKeepDraft(chains.configVersion.value, matching?.revision)
  } finally {
    recovering.value = false
  }
}

onMounted(() => {
  void chains.loadAll()
})
</script>

<template>
  <div class="space-y-6" data-testid="chains-view">
    <!-- Top Shared Navigation (3 Settings Domains) -->
    <SettingsSections active="chains" />

    <!-- PageToolbar -->
    <PageToolbar
      :title="uiText('链式代理', 'Proxy Chains')"
      :description="uiText('为出口目标配置前置跳板代理，支持三级范围与同级排序', 'Configure upstream dialer proxies for egress targets')"
      data-testid="chains-toolbar"
    >
      <template #actions>
        <button
          type="button"
          class="btn btn-sm btn-ghost gap-1.5"
          data-testid="refresh-chains-btn"
          :disabled="chains.loading.value || chains.optionsLoading.value || chains.effectsLoading.value || recovering"
          @click="chains.loadAll()"
        >
          <ArrowPathIcon
            class="w-4 h-4 shrink-0"
            :class="{ 'animate-spin': chains.loading.value || chains.effectsLoading.value }"
          />
          <span>{{ uiText('刷新', 'Refresh') }}</span>
        </button>

        <button
          type="button"
          class="btn btn-sm btn-primary gap-1.5 font-semibold shadow-sm"
          data-testid="new-chain-btn"
          :disabled="chains.loading.value || !optionsVerified || !chains.configVersion.value"
          @click="openCreate"
        >
          <PlusIcon class="w-4 h-4 shrink-0" />
          <span>{{ uiText('新建绑定', 'New Binding') }}</span>
        </button>
      </template>
    </PageToolbar>

    <!-- Global Error Alert -->
    <div
      v-if="chains.error.value"
      class="alert alert-error shadow-sm rounded-2xl p-4 text-xs flex flex-wrap items-start justify-between gap-3"
      role="alert"
    >
      <span class="min-w-0 flex-1 break-words">{{ displayText(chains.error.value) }}</span>
      <button
        type="button"
        class="btn btn-xs btn-outline"
        @click="chains.loadAll()"
      >
        {{ uiText('重试', 'Retry') }}
      </button>
    </div>

    <div v-if="chains.optionsError.value" role="alert" class="alert alert-warning text-sm flex flex-wrap gap-3 items-start" data-testid="chain-options-error">
      <span class="min-w-0 flex-1 break-words">{{ chains.optionsError.value }} {{ uiText('候选对象未确认，不应视为引用失效。已有名称可能来自上次读取，请重试。', 'Eligible objects are unverified, not necessarily missing. Names may be cached; retry options.') }}</span>
      <button class="btn btn-sm btn-ghost" :disabled="chains.optionsLoading.value" @click="chains.loadOptions()">{{ uiText('重试候选读取', 'Retry options') }}</button>
    </div>

    <!-- Section 1: Chain Bindings List -->
    <div class="bg-base-100 rounded-3xl border border-base-300/60 shadow-sm p-4 sm:p-6 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="text-base sm:text-lg font-bold text-base-content flex items-center gap-2">
            <LinkIcon class="w-5 h-5 text-primary shrink-0" />
            <span>{{ uiText('链式代理绑定列表', 'Proxy Chain Bindings') }}</span>
          </h3>
          <p class="text-xs text-base-content/60 mt-0.5">
            {{
              uiText(
                '单节点优先于策略组，策略组优先于订阅；同级排序值较小者优先',
                'Node > Group > Subscription; lower sort_order takes precedence at same level'
              )
            }}
          </p>
        </div>

      </div>

      <div class="flex flex-wrap items-center gap-3">
        <label class="relative flex-1 min-w-0 basis-60 max-w-md">
          <MagnifyingGlassIcon class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-base-content/40" />
          <input v-model="bindingSearch" type="search" class="input input-bordered input-sm w-full pl-9" :aria-label="uiText('搜索绑定', 'Search bindings')" :placeholder="uiText('搜索目标、跳板或备注', 'Search target, dialer or note')" data-testid="chain-binding-search" />
        </label>
        <select v-model="bindingStatus" class="select select-bordered select-sm" :aria-label="uiText('绑定状态筛选', 'Filter binding status')" data-testid="chain-binding-status">
          <option value="all">{{ uiText('全部状态', 'All statuses') }}</option>
          <option value="enabled">{{ uiText('已启用', 'Enabled') }}</option>
          <option value="disabled">{{ uiText('已停用', 'Disabled') }}</option>
        </select>
        <span role="status" class="text-xs text-base-content/60">{{ uiText('匹配', 'Matches') }} {{ filteredBindings.length }} / {{ chains.bindings.value.length }}</span>
      </div>

      <!-- Bindings Table or State -->
      <SectionState
        :loading="chains.loading.value"
        :error="displayText(chains.error.value)"
        :empty="chains.bindings.value.length === 0"
        :empty-title="uiText('暂无链式代理绑定', 'No Proxy Chain Bindings')"
        :empty-description="uiText('点击上方“新建绑定”按钮为出口目标配置跳板代理。', 'Click New Binding above to configure upstream dialers.')"
        @retry="chains.loadAll()"
      >
        <div class="overflow-x-auto rounded-2xl border border-base-200">
          <table class="table table-sm table-zebra w-full text-xs max-md:block" data-testid="chains-table">
            <thead class="hidden md:table-header-group bg-base-200/50 text-base-content/70">
              <tr>
                <th class="w-16">{{ uiText('状态', 'Status') }}</th>
                <th>{{ uiText('出口目标 (Target)', 'Egress Target') }}</th>
                <th class="w-8 text-center"></th>
                <th>{{ uiText('前置跳板 (Dialer)', 'Upstream Dialer') }}</th>
                <th>{{ uiText('优先级与排序', 'Priority & Order') }}</th>
                <th>{{ uiText('备注', 'Note') }}</th>
                <th class="w-16 text-center">{{ uiText('版本', 'Rev') }}</th>
                <th class="w-24 text-right">{{ uiText('操作', 'Actions') }}</th>
              </tr>
            </thead>
            <tbody class="block md:table-row-group">
              <tr
                v-for="{ binding: b, target, dialer } in visibleBindings"
                :key="b.id"
                class="block md:table-row max-md:p-2 max-md:border-b max-md:border-base-200"
                :data-testid="`chain-row-${b.id}`"
                :class="{ 'opacity-60': !b.enabled }"
              >
                <!-- Status Toggle -->
                <td class="block md:table-cell">
                  <div class="flex items-center gap-2">
                    <span class="md:hidden text-xs">{{ b.enabled ? uiText('已启用', 'Enabled') : uiText('已停用', 'Disabled') }}</span>
                    <input
                      type="checkbox"
                      class="toggle toggle-primary toggle-xs"
                      :checked="b.enabled"
                      :disabled="chains.rowBusy.value[b.id]"
                      :data-testid="`chain-toggle-${b.id}`"
                      :aria-label="uiText('切换此绑定启用状态', 'Toggle this binding')"
                      @change="handleToggle($event, b)"
                    />
                    <span
                      v-if="chains.rowBusy.value[b.id]"
                      class="loading loading-spinner loading-xs text-primary"
                    />
                  </div>
                </td>

                <!-- Target -->
                <td class="block md:table-cell whitespace-normal md:max-w-xs">
                  <span class="md:hidden text-xs text-base-content/60">{{ uiText('出口目标', 'Target') }}</span>
                  <div class="space-y-0.5">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      <span class="badge badge-sm badge-neutral">
                        {{ getTargetTypeLabel(b.target_type) }}
                      </span>
                      <span class="font-semibold text-base-content min-w-0 [overflow-wrap:anywhere]">
                        {{ target.name }}
                      </span>
                      <span
                        v-if="target.isMissing"
                        class="badge badge-warning badge-xs font-semibold"
                      >
                        {{ uiText('引用失效', 'Missing Ref') }}
                      </span>
                      <span v-if="target.isUnverified" class="badge badge-info badge-xs">{{ uiText('待确认', 'Unverified') }}</span>
                    </div>
                  </div>
                </td>

                <!-- Arrow -->
                <td class="hidden md:table-cell text-center text-primary font-bold">
                  →
                </td>

                <!-- Dialer -->
                <td class="block md:table-cell whitespace-normal md:max-w-xs">
                  <span class="md:hidden text-xs text-base-content/60">{{ uiText('前置跳板', 'Dialer') }}</span>
                  <div class="space-y-0.5">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      <span class="badge badge-sm badge-primary badge-outline font-semibold">
                        {{ getDialerTypeLabel(b.dialer_type) }}
                      </span>
                      <span class="font-semibold text-base-content min-w-0 [overflow-wrap:anywhere]">
                        {{ dialer.name }}
                      </span>
                      <span
                        v-if="dialer.isMissing"
                        class="badge badge-warning badge-xs font-semibold"
                      >
                        {{ uiText('引用失效', 'Missing Ref') }}
                      </span>
                      <span v-if="dialer.isUnverified" class="badge badge-info badge-xs">{{ uiText('待确认', 'Unverified') }}</span>
                    </div>
                  </div>
                </td>

                <!-- Priority & Order -->
                <td class="block md:table-cell">
                  <div class="flex items-center gap-1.5">
                    <span
                      class="badge badge-xs font-semibold"
                      :class="{
                        'badge-secondary': b.target_type === 'node',
                        'badge-accent': b.target_type === 'node_group',
                        'badge-ghost': b.target_type === 'subscription',
                      }"
                    >
                      P{{ getPriorityRank(b.target_type) }}
                    </span>
                    <span class="font-mono text-xs">
                      {{ uiText('排序', 'Order') }}: {{ b.sort_order }}
                    </span>
                  </div>
                </td>

                <!-- Note -->
                <td class="block md:table-cell whitespace-normal break-words md:max-w-xs">
                  <span class="text-base-content/80 text-xs">{{ b.note || '—' }}</span>
                  <div
                    v-if="chains.rowError.value[b.id]"
                    class="text-xs text-error font-medium mt-1 break-words"
                    role="alert"
                  >
                    {{ displayText(chains.rowError.value[b.id]) }}
                  </div>
                </td>

                <!-- Revision -->
                <td class="block md:table-cell md:text-center font-mono text-[11px] text-base-content/50">
                  r{{ b.revision }}
                </td>

                <!-- Actions -->
                <td class="block md:table-cell text-right">
                  <div class="flex items-center justify-end gap-1">
                    <button
                      type="button"
                      class="btn btn-ghost btn-sm md:btn-xs gap-1"
                      :title="uiText('编辑绑定', 'Edit Binding')"
                      :data-testid="`chain-edit-btn-${b.id}`"
                      @click="openEdit(b)"
                    >
                      <PencilSquareIcon class="w-4 h-4 text-base-content/70 hover:text-primary" />
                      <span class="md:sr-only">{{ uiText('编辑', 'Edit') }}</span>
                    </button>
                    <button
                      type="button"
                      class="btn btn-ghost btn-sm md:btn-xs text-error gap-1"
                      :title="uiText('删除绑定', 'Delete Binding')"
                      :data-testid="`chain-delete-btn-${b.id}`"
                      @click="chains.openDeleteConfirm(b)"
                    >
                      <TrashIcon class="w-4 h-4 hover:text-error" />
                      <span class="md:sr-only">{{ uiText('删除', 'Delete') }}</span>
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-if="filteredBindings.length === 0" class="p-6 text-center text-sm text-base-content/60">{{ uiText('没有匹配的绑定，请调整搜索或状态筛选', 'No matching bindings; adjust search or status filter') }}</p>
        </div>
        <button v-if="filteredBindings.length > visibleBindings.length" type="button" class="btn btn-sm btn-ghost" @click="bindingLimit += 50">{{ uiText('显示更多绑定', 'Show more bindings') }} ({{ visibleBindings.length }} / {{ filteredBindings.length }})</button>
      </SectionState>
    </div>

    <!-- Section 2: Chain Effect Ledger (Per-Node Dialer Table) -->
    <ChainEffectLedger
      :nodes="chains.nodes.value"
      :options="chains.options.value"
      :loading="chains.effectsLoading.value"
      :loaded="chains.effectsLoaded.value"
      :stale="chains.effectsVersion.value !== chains.configVersion.value"
      :error="displayText(chains.effectsError.value)"
      @retry="chains.loadEffects()"
    />

    <!-- Create / Edit Drawer -->
    <ChainDrawer
      :draft-manager="draftManager"
      :options="chains.options.value"
      :options-verified="optionsVerified"
      :options-loading="chains.optionsLoading.value"
      :recovering="recovering"
      :recovery-error="recoveryError"
      :current-config-version="chains.configVersion.value"
      @close="handleDrawerClose"
      @reload="handleDrawerReload"
      @rebase="handleDrawerRebase"
      @retry-options="chains.loadOptions()"
    />

    <!-- Delete Confirmation Modal -->
    <ConfirmModal
      :model-value="chains.isDeleteModalOpen.value"
      @update:model-value="(val: boolean) => { if (!val) chains.closeDeleteConfirm() }"
      :title="uiText('确认删除链式代理绑定', 'Confirm Delete Proxy Chain')"
      tone="danger"
      :confirm-text="uiText('确认删除', 'Delete')"
      :cancel-text="uiText('取消', 'Cancel')"
      :loading="chains.deleteLoading.value"
      data-testid="chain-delete-modal"
      @confirm="chains.confirmDelete()"
      @cancel="chains.closeDeleteConfirm()"
    >
      <div v-if="chains.bindingToDelete.value" class="space-y-3 text-xs">
        <p class="text-base-content/80">
          {{
            uiText(
              '确定要删除此链式代理绑定吗？删除后将根据下级绑定规则重新解析跳板。',
              'Are you sure you want to delete this chain binding? Egress nodes will revert to lower-priority bindings or direct connections.'
            )
          }}
        </p>

        <div class="bg-base-200/60 p-3 rounded-xl space-y-1 font-mono text-[11px]">
          <div>
            <span class="text-base-content/50">{{ uiText('目标', 'Target') }}: </span>
            <span class="font-semibold text-base-content">
              {{ getTargetDisplayName(chains.bindingToDelete.value.target_type, chains.bindingToDelete.value.target_id, chains.options.value, optionsVerified).name }}
            </span>
          </div>
          <div>
            <span class="text-base-content/50">{{ uiText('跳板', 'Dialer') }}: </span>
            <span class="font-semibold text-base-content">
              {{ getDialerDisplayName(chains.bindingToDelete.value.dialer_type, chains.bindingToDelete.value.dialer_id, chains.options.value, optionsVerified).name }}
            </span>
          </div>
        </div>

        <!-- 422 Unsafe Lower Binding Error Preservation -->
        <div
          v-if="chains.deleteError.value"
          class="alert alert-error shadow-sm rounded-xl p-3 text-xs flex items-start gap-2"
          data-testid="chain-delete-error"
        >
          <ExclamationTriangleIcon class="w-4 h-4 shrink-0 text-error mt-0.5" />
          <div class="space-y-1 min-w-0 flex-1">
            <span class="font-bold">{{ uiText('无法删除该绑定：', 'Cannot delete binding:') }}</span>
            <p class="leading-relaxed break-words">{{ displayText(chains.deleteError.value) }}</p>
            <p class="text-[11px] text-base-content/60 mt-1">
              {{
                uiText(
                  '删除当前高优绑定暴露了下级绑定中的循环或非法引用，请先修复或停用下级绑定。',
                  'Deleting this higher-priority binding exposes an unsafe lower-priority binding cycle or invalid reference. Please repair or disable the lower binding first.'
                )
              }}
            </p>
          </div>
        </div>
      </div>
    </ConfirmModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import {
  ArrowPathIcon,
  PlusIcon,
  LinkIcon,
  PencilSquareIcon,
  TrashIcon,
  ExclamationTriangleIcon,
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
import {
  getTargetDisplayName,
  getDialerDisplayName,
  getTargetTypeLabel,
  getDialerTypeLabel,
  getPriorityRank,
} from './types'

const chains = useChains()

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
  await chains.loadAll()
  draftManager.close()
}

async function handleDrawerRebase() {
  await chains.loadBindings()
  const matching = chains.bindings.value.find((b) => b.id === draftManager.draft.value.id)
  draftManager.rebaseAndKeepDraft(chains.configVersion.value, matching?.revision)
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
          :disabled="chains.loading.value || chains.effectsLoading.value"
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
          @click="draftManager.openCreate()"
        >
          <PlusIcon class="w-4 h-4 shrink-0" />
          <span>{{ uiText('新建绑定', 'New Binding') }}</span>
        </button>
      </template>
    </PageToolbar>

    <!-- Global Error Alert -->
    <div
      v-if="chains.error.value"
      class="alert alert-error shadow-sm rounded-2xl p-4 text-xs flex items-center justify-between"
    >
      <span>{{ chains.error.value }}</span>
      <button
        type="button"
        class="btn btn-xs btn-outline"
        @click="chains.loadAll()"
      >
        {{ uiText('重试', 'Retry') }}
      </button>
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

        <div v-if="chains.configVersion.value" class="text-xs text-base-content/50 font-mono">
          <span>{{ uiText('配置版本', 'Config Version') }}: </span>
          <span class="font-semibold">{{ chains.configVersion.value.slice(0, 10) }}...</span>
        </div>
      </div>

      <!-- Bindings Table or State -->
      <SectionState
        :loading="chains.loading.value"
        :error="chains.error.value"
        :empty="chains.bindings.value.length === 0"
        :empty-title="uiText('暂无链式代理绑定', 'No Proxy Chain Bindings')"
        :empty-description="uiText('点击上方“新建绑定”按钮为出口目标配置跳板代理。', 'Click New Binding above to configure upstream dialers.')"
        @retry="chains.loadAll()"
      >
        <div class="overflow-x-auto rounded-2xl border border-base-200">
          <table class="table table-sm table-zebra w-full text-xs" data-testid="chains-table">
            <thead class="bg-base-200/50 text-base-content/70">
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
            <tbody>
              <tr
                v-for="b in chains.bindings.value"
                :key="b.id"
                :data-testid="`chain-row-${b.id}`"
                :class="{ 'opacity-60': !b.enabled }"
              >
                <!-- Status Toggle -->
                <td>
                  <div class="flex items-center gap-1.5">
                    <input
                      type="checkbox"
                      class="toggle toggle-primary toggle-xs"
                      :checked="b.enabled"
                      :disabled="chains.rowBusy.value[b.id]"
                      :data-testid="`chain-toggle-${b.id}`"
                      @change="chains.toggleEnabled(b)"
                    />
                    <span
                      v-if="chains.rowBusy.value[b.id]"
                      class="loading loading-spinner loading-xs text-primary"
                    />
                  </div>
                </td>

                <!-- Target -->
                <td>
                  <div class="space-y-0.5">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      <span class="badge badge-sm badge-neutral">
                        {{ getTargetTypeLabel(b.target_type) }}
                      </span>
                      <span class="font-semibold text-base-content">
                        {{ getTargetDisplayName(b.target_type, b.target_id, chains.options.value).name }}
                      </span>
                      <span
                        v-if="getTargetDisplayName(b.target_type, b.target_id, chains.options.value).isMissing"
                        class="badge badge-warning badge-xs font-semibold"
                      >
                        {{ uiText('引用失效', 'Missing Ref') }}
                      </span>
                    </div>
                    <div class="text-[10px] text-base-content/50 font-mono">{{ b.target_id }}</div>
                  </div>
                </td>

                <!-- Arrow -->
                <td class="text-center text-primary font-bold">
                  →
                </td>

                <!-- Dialer -->
                <td>
                  <div class="space-y-0.5">
                    <div class="flex items-center gap-1.5 flex-wrap">
                      <span class="badge badge-sm badge-primary badge-outline font-semibold">
                        {{ getDialerTypeLabel(b.dialer_type) }}
                      </span>
                      <span class="font-semibold text-base-content">
                        {{ getDialerDisplayName(b.dialer_type, b.dialer_id, chains.options.value).name }}
                      </span>
                      <span
                        v-if="getDialerDisplayName(b.dialer_type, b.dialer_id, chains.options.value).isMissing"
                        class="badge badge-warning badge-xs font-semibold"
                      >
                        {{ uiText('引用失效', 'Missing Ref') }}
                      </span>
                    </div>
                    <div class="text-[10px] text-base-content/50 font-mono">{{ b.dialer_id }}</div>
                  </div>
                </td>

                <!-- Priority & Order -->
                <td>
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
                <td>
                  <span class="text-base-content/80 text-xs">{{ b.note || '—' }}</span>
                  <div
                    v-if="chains.rowError.value[b.id]"
                    class="text-[10px] text-error font-medium mt-0.5"
                  >
                    {{ chains.rowError.value[b.id] }}
                  </div>
                </td>

                <!-- Revision -->
                <td class="text-center font-mono text-[11px] text-base-content/50">
                  r{{ b.revision }}
                </td>

                <!-- Actions -->
                <td class="text-right">
                  <div class="flex items-center justify-end gap-1">
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square"
                      :title="uiText('编辑绑定', 'Edit Binding')"
                      :data-testid="`chain-edit-btn-${b.id}`"
                      @click="draftManager.openEdit(b)"
                    >
                      <PencilSquareIcon class="w-4 h-4 text-base-content/70 hover:text-primary" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-ghost btn-xs btn-square text-error"
                      :title="uiText('删除绑定', 'Delete Binding')"
                      :data-testid="`chain-delete-btn-${b.id}`"
                      @click="chains.openDeleteConfirm(b)"
                    >
                      <TrashIcon class="w-4 h-4 hover:text-error" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </SectionState>
    </div>

    <!-- Section 2: Chain Effect Ledger (Per-Node Dialer Table) -->
    <ChainEffectLedger
      :nodes="chains.nodes.value"
      :loading="chains.effectsLoading.value"
      :error="chains.effectsError.value"
    />

    <!-- Create / Edit Drawer -->
    <ChainDrawer
      :draft-manager="draftManager"
      :options="chains.options.value"
      :current-config-version="chains.configVersion.value"
      @close="handleDrawerClose"
      @reload="handleDrawerReload"
      @rebase="handleDrawerRebase"
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
              {{ chains.bindingToDelete.value.target_type }} / {{ chains.bindingToDelete.value.target_id }}
            </span>
          </div>
          <div>
            <span class="text-base-content/50">{{ uiText('跳板', 'Dialer') }}: </span>
            <span class="font-semibold text-base-content">
              {{ chains.bindingToDelete.value.dialer_type }} / {{ chains.bindingToDelete.value.dialer_id }}
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
          <div class="space-y-1">
            <span class="font-bold">{{ uiText('无法删除该绑定：', 'Cannot delete binding:') }}</span>
            <p class="leading-relaxed">{{ chains.deleteError.value }}</p>
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

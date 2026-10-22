<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  PlusIcon,
  CheckBadgeIcon,
  ExclamationTriangleIcon,
  CpuChipIcon,
  Squares2X2Icon,
  GlobeAltIcon,
  ArrowPathIcon,
  MagnifyingGlassIcon,
} from '@heroicons/vue/24/outline'
import { usePolicy } from './usePolicy'
import type {
  FilterCondition,
  MemberSource,
  GroupType,
  PolicyGroup,
} from './policyTypes'
import {
  ALL_GROUP_TYPES,
  MODERN_RULE_CAPABILITY_MATRIX,
} from './policyTypes'
import GroupCard from './GroupCard.vue'
import PolicyEditorSheet from './PolicyEditorSheet.vue'
import StructuredFilterEditor from './StructuredFilterEditor.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import ModalDialog from '../../ui/ModalDialog.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import PageToolbar from '../../ui/PageToolbar.vue'
import { useNodes } from '../nodes/useNodes'
import { uiText } from '../../locales/redesign'
import { t } from '../../locales'

const {
  groups,
  globalFilter,
  loading,
  saving,
  savingGlobalFilter,
  validating,
  validationResult,
  validationState,
  validationStale,
  validationError,
  abortValidation,
  error,
  page,
  pageSize,
  searchQuery,
  totalGroups,
  allTopologyGroups,
  loadGroups,
  loadAllTopologyGroups,
  reload,
  createGroup,
  updateGroup,
  moveGroup,
  deleteGroup,
  validateGraph,
  loadGlobalFilter,
  updateGlobalFilter,
} = usePolicy()

const hasWarningOrErrorIssues = computed(() => {
  return validationResult.value?.issues?.some(
    (issue) => issue.severity === 'warning' || issue.severity === 'error'
  ) ?? false
})

const groupSearchInput = ref('')
let groupSearchDebounceTimer: ReturnType<typeof setTimeout> | null = null

function onGroupSearchInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  groupSearchInput.value = val
  if (groupSearchDebounceTimer) clearTimeout(groupSearchDebounceTimer)
  groupSearchDebounceTimer = setTimeout(() => {
    loadGroups(1, pageSize.value, val)
  }, 300)
}

function handlePrevPage() {
  if (page.value > 1) {
    loadGroups(page.value - 1, pageSize.value, groupSearchInput.value)
  }
}

function handleNextPage() {
  if (page.value * pageSize.value < totalGroups.value) {
    loadGroups(page.value + 1, pageSize.value, groupSearchInput.value)
  }
}

// Global Filter Modal State
const globalFilterModalOpen = ref(false)
const globalConditions = ref<FilterCondition[]>([])
const globalConditionError = ref('')

function openGlobalFilterModal() {
  globalConditions.value = (globalFilter.value?.spec?.conditions || []).map((c) => ({ ...c }))
  globalConditionError.value = ''
  globalFilterModalOpen.value = true
}

async function saveGlobalFilter() {
  try {
    await updateGlobalFilter({ conditions: globalConditions.value })
    await reload()
    globalFilterModalOpen.value = false
  } catch (err: any) {
    globalConditionError.value = err?.message || '保存全局筛选失败'
  }
}

// Group / Edges Sheet State
const sheetOpen = ref(false)
const sheetMode = ref<'group' | 'edges'>('group')
const activeGroup = ref<PolicyGroup | null>(null)
const selectedGroupId = ref<string | null>(null)
const conflictData = ref<{ currentRevision: number; message: string } | null>(null)
const cycleData = ref<{ cyclePath: string[]; message: string } | null>(null)
const sheetServerError = ref<string | null>(null)

function selectGroup(group: PolicyGroup) {
  selectedGroupId.value = group.id
  openEditGroup(group)
}

function handleSheetClose() {
  sheetOpen.value = false
  // Clean up selected group pointer on close so UI doesn't leave stale error state
  selectedGroupId.value = null
  conflictData.value = null
  cycleData.value = null
  sheetServerError.value = null
}

function openCreateGroup() {
  activeGroup.value = null
  selectedGroupId.value = null
  sheetMode.value = 'group'
  conflictData.value = null
  cycleData.value = null
  sheetServerError.value = null
  sheetOpen.value = true
}

function openEditGroup(group: PolicyGroup) {
  activeGroup.value = group
  selectedGroupId.value = group.id
  sheetMode.value = 'group'
  conflictData.value = null
  cycleData.value = null
  sheetServerError.value = null
  sheetOpen.value = true
}

function openManageEdges(group: PolicyGroup) {
  activeGroup.value = group
  selectedGroupId.value = group.id
  sheetMode.value = 'edges'
  conflictData.value = null
  cycleData.value = null
  sheetServerError.value = null
  sheetOpen.value = true
}

async function handleReloadGroup(groupId: string) {
  await loadGroups(page.value, pageSize.value)
  const found = groups.value.find((g) => g.id === groupId)
  if (found) {
    activeGroup.value = found
  }
  conflictData.value = null
  cycleData.value = null
  sheetServerError.value = null
}

async function handleSaveGroup(data: {
  name: string
  type: GroupType
  position?: number
  emptyFallbackPass: boolean
  inheritGlobalFilter?: boolean
  sources: MemberSource[]
  expectedRevision?: number
}) {
  conflictData.value = null
  cycleData.value = null
  sheetServerError.value = null
  try {
    if (activeGroup.value) {
      const updated = await updateGroup(activeGroup.value.id, {
        name: data.name,
        group_type: data.type,
        position: data.position,
        empty_fallback_pass: data.emptyFallbackPass,
        inherit_global_filter: data.inheritGlobalFilter,
        expected_revision: data.expectedRevision ?? activeGroup.value.revision,
        sources: data.sources,
      })
      activeGroup.value = updated
      selectedGroupId.value = updated.id
    } else {
      const created = await createGroup({
        name: data.name,
        group_type: data.type,
        position: data.position,
        empty_fallback_pass: data.emptyFallbackPass,
        inherit_global_filter: data.inheritGlobalFilter,
        sources: data.sources,
      })
      activeGroup.value = created
      selectedGroupId.value = created.id
    }
    sheetOpen.value = false
  } catch (err: any) {
    if (err?.status === 409 || err?.code === 'revision_conflict') {
      const rev = (err.details as any)?.current_revision ?? (err as any).current_revision ?? ((activeGroup.value?.revision || 1) + 1)
      conflictData.value = {
        currentRevision: rev,
        message: err.message || '策略组版本冲突，服务端已更新。',
      }
    } else if (err?.status === 422 && ((err.details as any)?.cycle_path || err?.code === 'cycle_detected')) {
      cycleData.value = {
        cyclePath: (err.details as any)?.cycle_path ?? err.cycle_path ?? [],
        message: err.message || '检测到循环依赖环路',
      }
    } else {
      sheetServerError.value = err instanceof Error ? err.message : '保存策略组失败'
    }
  }
}

const { items: nodeItems, load: loadNodes } = useNodes()
const confirmDeleteGroupOpen = ref(false)
const pendingDeleteGroupId = ref<string | null>(null)
const deletingGroup = ref(false)

async function handleMoveUp(grp: PolicyGroup) {
  await moveGroup(grp.id, 'up')
}

async function handleMoveDown(grp: PolicyGroup) {
  await moveGroup(grp.id, 'down')
}

function handleDeleteGroup(id: string) {
  pendingDeleteGroupId.value = id
  confirmDeleteGroupOpen.value = true
}

async function handleConfirmDeleteGroup() {
  if (!pendingDeleteGroupId.value) return
  deletingGroup.value = true
  try {
    await deleteGroup(pendingDeleteGroupId.value)
    confirmDeleteGroupOpen.value = false
    pendingDeleteGroupId.value = null
  } finally {
    deletingGroup.value = false
  }
}

onMounted(() => {
  void reload()
  loadNodes()
})

onUnmounted(() => {
  abortValidation()
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="policy-title">
    <!-- Header with PageToolbar & Main Actions -->
    <PageToolbar :title="t('policy.title')" :description="t('policy.subtitle')">
      <template #actions>
        <a
          href="#/rules"
          data-testid="rules-nav-link"
          class="btn btn-outline btn-sm gap-2"
        >
          <ArrowPathIcon class="w-4 h-4 text-primary" />
          {{ t('policy.routingRulesTab') }}
        </a>

        <button
          type="button"
          data-testid="global-filter-btn"
          class="btn btn-outline btn-sm gap-2"
          @click="openGlobalFilterModal"
        >
          <GlobeAltIcon class="w-4 h-4 text-primary" />
          {{ t('policy.globalFilter') }}
          <span
            v-if="globalFilter?.spec?.conditions && globalFilter.spec.conditions.length > 0"
            class="badge badge-primary badge-xs"
          >
            {{ globalFilter.spec.conditions.length }}
          </span>
        </button>

        <button
          type="button"
          data-testid="manual-validate-btn"
          class="btn btn-outline btn-sm gap-2"
          :class="{ loading: validating }"
          :disabled="validating || saving"
          @click="validateGraph"
        >
          <CheckBadgeIcon class="w-4 h-4 text-success" />
          {{ t('policy.manualValidate') }}
        </button>

        <button
          type="button"
          data-testid="add-group-btn"
          class="btn btn-primary btn-sm gap-2"
          @click="openCreateGroup"
        >
          <PlusIcon class="w-4 h-4" />
          {{ t('policy.addGroup') }}
        </button>
      </template>
    </PageToolbar>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loading"
      @retry="reload"
    />

    <!-- Single Unified Validation Status Region (never stacks stale + incomplete + result alerts) -->
    <div data-testid="validation-status-region" class="space-y-2">
      <!-- Priority 1: Incomplete validation (request error / timeout / network failure) -->
      <div
        v-if="validationState === 'incomplete'"
        role="alert"
        aria-live="polite"
        class="p-3.5 rounded-xl border border-error/30 bg-error/10 text-error text-xs flex items-center justify-between gap-3"
      >
        <div class="flex items-center gap-2 min-w-0">
          <ExclamationTriangleIcon class="w-5 h-5 shrink-0" />
          <div class="min-w-0">
            <span class="font-bold">{{ t('policy.validationIncomplete') }}</span>
            <p class="opacity-80 mt-0.5 truncate">
              {{ validationError || uiText('校验服务请求失败 (网络/服务异常)，非策略无效，请点击右侧重试', 'Validation request failed (network/service error), draft is not invalid. Please retry.') }}
            </p>
          </div>
        </div>
        <button
          type="button"
          class="btn btn-error btn-xs shrink-0"
          :disabled="validating"
          @click="validateGraph"
        >
          {{ t('policy.retryValidate') }}
        </button>
      </div>

      <!-- Priority 2: Stale validation notification (when not validating and not incomplete) -->
      <div
        v-else-if="validationStale && validationState !== 'validating'"
        class="p-2.5 rounded-lg border border-warning/30 bg-warning/10 text-warning text-xs flex items-center justify-between"
      >
        <div class="flex items-center gap-2">
          <ArrowPathIcon class="w-4 h-4 animate-spin shrink-0" />
          <span>{{ t('policy.validationStaleNotice') }}</span>
        </div>
        <button
          type="button"
          class="btn btn-warning btn-xs btn-outline shrink-0"
          :disabled="validating"
          @click="validateGraph"
        >
          {{ uiText('重新校验', 'Re-validate') }}
        </button>
      </div>

      <!-- Priority 3: Currently validating -->
      <div
        v-else-if="validationState === 'validating'"
        class="p-2.5 rounded-lg border border-info/30 bg-info/10 text-info text-xs flex items-center gap-2"
      >
        <ArrowPathIcon class="w-4 h-4 animate-spin shrink-0" />
        <span>{{ uiText('正在校验策略拓扑与分流规则...', 'Validating policy topology and routing rules...') }}</span>
      </div>

      <!-- Priority 4: Clean validation result banner (when not stale and not incomplete) -->
      <Transition
        enter-active-class="transition-all duration-200 ease-out"
        enter-from-class="opacity-0 -translate-y-2"
        enter-to-class="opacity-100 translate-y-0"
      >
        <div
          v-if="validationResult"
          class="p-4 rounded-xl border text-sm flex items-start justify-between gap-3"
          :class="validationResult.valid ? (hasWarningOrErrorIssues ? 'bg-warning/10 border-warning/30 text-warning' : 'bg-success/10 border-success/30 text-success') : 'bg-error/10 border-error/30 text-error'"
        >
          <div class="flex items-start gap-2.5 min-w-0">
            <CheckBadgeIcon v-if="validationResult.valid && !hasWarningOrErrorIssues" class="w-5 h-5 shrink-0 mt-0.5" />
            <ExclamationTriangleIcon v-else class="w-5 h-5 shrink-0 mt-0.5" />
            <div class="min-w-0 space-y-1">
              <h4 class="font-bold">
                {{ validationResult.valid ? (hasWarningOrErrorIssues ? '拓扑校验通过（存在配置警告）' : '拓扑与分流校验通过') : '拓扑与分流校验存在阻断问题' }}
              </h4>
              <p v-if="validationResult.valid && !hasWarningOrErrorIssues" class="text-xs opacity-90 mt-0.5">
                {{ t('policy.validationSuccess') }}
              </p>
              <div v-else-if="validationResult.issues && validationResult.issues.length > 0" class="space-y-1.5 mt-1.5">
                <div
                  v-for="(iss, idx) in validationResult.issues"
                  :key="idx"
                  class="text-xs flex flex-wrap items-center gap-1.5 p-1.5 rounded bg-base-100/60 border border-base-300/40"
                >
                  <span
                    class="badge badge-xs font-bold"
                    :class="iss.severity === 'error' ? 'badge-error' : 'badge-warning'"
                  >
                    {{ iss.severity === 'error' ? '错误' : '警告' }}
                  </span>
                  <span v-if="iss.position !== undefined" class="badge badge-ghost badge-xs font-mono">
                    #{{ iss.position + 1 }}
                  </span>
                  <span v-if="iss.type" class="badge badge-outline badge-xs font-mono">
                    {{ iss.type }}
                  </span>
                  <span v-if="iss.target_group_name" class="badge badge-neutral badge-xs">
                    {{ iss.target_group_name }}
                  </span>
                  <span class="opacity-90">{{ iss.message }}</span>
                </div>
              </div>
              <ul v-else class="mt-1 text-xs list-disc list-inside space-y-0.5 font-mono">
                <li v-for="(err, idx) in validationResult.errors" :key="idx">{{ err }}</li>
              </ul>
            </div>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-xs btn-circle"
            @click="validationResult = null"
          >
            ✕
          </button>
        </div>
      </Transition>
    </div>

    <!-- Group & Routing Rule Target Capability Boundary (On-Demand Accessible Details, not a permanent giant banner) -->
    <details
      data-testid="policy-capability-boundary"
      class="collapse collapse-arrow rounded-xl border border-base-300 bg-base-200/50 text-xs min-w-0 w-full"
    >
      <summary class="collapse-title text-xs font-semibold py-2.5 px-4 min-h-0 flex items-center justify-between cursor-pointer select-none">
        <span>{{ uiText('编译目标能力边界说明（策略组与分流规则）', 'Compiler Target Capabilities (Policy Groups & Routing Rules)') }}</span>
        <span class="badge badge-xs badge-ghost font-mono ml-2">仅 Mihomo 完整导出</span>
      </summary>
      <div class="collapse-content space-y-2.5 pt-2">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px] font-mono">
          <div class="p-2.5 rounded-lg bg-base-100/70 border border-base-300/60 space-y-1">
            <span class="font-sans font-semibold opacity-75 block">策略组类型支持目标</span>
            <div
              v-for="gt in ALL_GROUP_TYPES"
              :key="gt.type"
              class="flex flex-wrap items-center justify-between gap-1"
            >
              <span class="text-primary font-semibold">{{ gt.label }}</span>
              <span class="opacity-80">{{ gt.supportedTargets.join(', ') }}</span>
            </div>
          </div>
          <div class="p-2.5 rounded-lg bg-base-100/70 border border-base-300/60 space-y-1">
            <span class="font-sans font-semibold opacity-75 block">分流规则子集支持目标</span>
            <div
              v-for="band in MODERN_RULE_CAPABILITY_MATRIX"
              :key="band.category"
              class="leading-snug"
            >
              <span class="font-sans opacity-75 mr-1">{{ band.category }}：</span>
              <span class="text-secondary font-semibold">{{ band.ruleKinds.join(', ') }}</span> →
              <span class="opacity-80">{{ band.supportedTargets.join(', ') }}</span>
            </div>
          </div>
        </div>
      </div>
    </details>

    <!-- Policy Groups View -->
    <div class="space-y-4">
      <!-- Search & Pagination Controls Bar -->
      <div
        data-testid="policy-groups-toolbar"
        class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 p-3 rounded-xl bg-base-200/50 border border-base-300"
      >
        <div class="relative flex-1 max-w-xs sm:max-w-sm">
          <input
            :value="groupSearchInput"
            data-testid="policy-group-search-input"
            type="search"
            placeholder="搜索策略组名称或类型..."
            class="input input-xs sm:input-sm w-full pl-8 font-mono text-xs"
            @input="onGroupSearchInput"
          />
          <MagnifyingGlassIcon class="w-4 h-4 opacity-50 absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
        </div>

        <div class="flex items-center justify-between sm:justify-end gap-3 text-xs font-mono opacity-80">
          <span>第 {{ page }} / {{ Math.max(1, Math.ceil(totalGroups / pageSize)) }} 页 (共 {{ totalGroups }} 组)</span>
          <div class="join">
            <button
              type="button"
              data-testid="policy-prev-page-btn"
              class="join-item btn btn-xs"
              :disabled="loading || page <= 1"
              @click="handlePrevPage"
            >
              上一页
            </button>
            <button
              type="button"
              data-testid="policy-next-page-btn"
              class="join-item btn btn-xs"
              :disabled="loading || page * pageSize >= totalGroups"
              @click="handleNextPage"
            >
              下一页
            </button>
          </div>
        </div>
      </div>

      <div v-if="loading && groups.length === 0" class="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,300px),1fr))]">
        <div v-for="i in 6" :key="i" class="skeleton h-32 rounded-xl" />
      </div>

      <div
        v-else-if="groups.length === 0"
        class="rounded-2xl border border-dashed border-base-300 p-12 text-center"
      >
        <Squares2X2Icon class="w-10 h-10 mx-auto opacity-40 text-primary" />
        <p class="mt-3 font-semibold text-base">{{ t('policy.emptyTitle') }}</p>
        <p class="mt-1 text-sm opacity-60">
          {{ t('policy.emptyDesc') }}
        </p>
        <button
          type="button"
          class="btn btn-primary btn-sm mt-4 gap-2"
          @click="openCreateGroup"
        >
          <PlusIcon class="w-4 h-4" />
          {{ t('policy.createGroup') }}
        </button>
      </div>

      <div v-else class="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,300px),1fr))]">
        <GroupCard
          v-for="(grp, idx) in groups"
          :key="grp.id"
          :group="grp"
          :available-groups="allTopologyGroups.length > 0 ? allTopologyGroups : groups"
          :selected="selectedGroupId === grp.id"
          :can-move-up="idx > 0"
          :can-move-down="idx < groups.length - 1"
          @select="selectGroup"
          @edit="openEditGroup"
          @delete="handleDeleteGroup"
          @manage-edges="openManageEdges"
          @move-up="handleMoveUp"
          @move-down="handleMoveDown"
        />
      </div>
    </div>

    <!-- Global Filter Modal Dialog -->
    <ModalDialog
      v-model="globalFilterModalOpen"
      title="全局节点筛选"
      description="在策略组来源流水线之前评估软筛选；被排除的节点默认不进入继承全局过滤的策略组，关闭继承的策略组（如 WARP 或专用免检线路）享有软豁免并直接匹配全部候选节点(不应用全局筛选)。"
    >
      <div class="space-y-4">
        <div class="p-3 bg-info/10 border border-info/30 rounded-xl text-xs text-info leading-relaxed">
          <p><strong>执行优先级：</strong>节点停用与显式风险排除 → 全局节点软筛选 → 策略组有序来源流水线（逐项加入 / 排除，后续可重新加入）。</p>
          <p class="mt-1">留空筛选条件时将允许所有符合启用状态与风险限制的候选节点，不做额外全局过滤。</p>
          <p class="mt-1 opacity-80">{{ t('settings.filterExplanation') }}</p>
          <p class="mt-1 opacity-80">说明：全局软筛选在直接导出与预览时即时生效；WARP 或专用免检线路等策略组可关闭「继承全局筛选」以软豁免并使用全部候选节点(不应用全局筛选)。当前凭据版本门槛、节点停用、来源订阅禁用与显式风险排除始终保留生效，不可绕过。</p>
        </div>

        <p v-if="globalConditionError" role="alert" class="text-error text-xs" data-testid="filter-request-error">
          {{ globalConditionError }}
        </p>

        <StructuredFilterEditor
          v-model="globalConditions"
          :max-conditions="32"
          :allow-clear="true"
        />

        <div class="modal-action border-t border-base-300 pt-3">
          <button type="button" class="btn btn-ghost btn-sm" @click="globalFilterModalOpen = false">
            取消
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm gap-2"
            :class="{ loading: savingGlobalFilter }"
            :disabled="savingGlobalFilter"
            @click="saveGlobalFilter"
          >
            保存全局筛选
          </button>
        </div>
      </div>
    </ModalDialog>

    <!-- Policy Group / Edges Bottom Sheet -->
    <PolicyEditorSheet
      :open="sheetOpen"
      :group="activeGroup"
      :all-groups="allTopologyGroups.length > 0 ? allTopologyGroups : groups"
      :available-nodes="nodeItems"
      :mode="sheetMode"
      :saving="saving"
      :conflict-error="conflictData"
      :cycle-error="cycleData"
      :server-error="sheetServerError"
      @close="handleSheetClose"
      @save-group="handleSaveGroup"
      @reload-group="handleReloadGroup"
      @clear-conflict="conflictData = null"
    />

    <!-- Confirm Delete Policy Group Modal -->
    <ConfirmModal
      v-model="confirmDeleteGroupOpen"
      title="确认删除策略组"
      message="确定要删除此策略组吗？关联的有向拓扑边和路由引用将一并移除。"
      confirm-text="删除策略组"
      cancel-text="取消"
      tone="danger"
      :loading="deletingGroup"
      @confirm="handleConfirmDeleteGroup"
    />
  </section>
</template>

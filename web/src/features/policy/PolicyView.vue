<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import {
  PlusIcon,
  CheckBadgeIcon,
  ExclamationTriangleIcon,
  ShieldCheckIcon,
  CpuChipIcon,
  Squares2X2Icon,
  GlobeAltIcon,
  TrashIcon,
} from '@heroicons/vue/24/outline'
import { usePolicy } from './usePolicy'
import type {
  FilterCondition,
  FilterField,
  FilterOp,
  GroupEdge,
  GroupType,
  PolicyGroup,
  RuleAction,
} from './policyTypes'
import {
  ALL_GROUP_TYPES,
  MODERN_RULE_CAPABILITY_MATRIX,
  SUPPORTED_FILTER_FIELDS,
  ruleActionLabel,
  ruleActionTone,
  validateConditionInput,
} from './policyTypes'
import GroupCard from './GroupCard.vue'
import PolicyEditorSheet from './PolicyEditorSheet.vue'
import ConfirmModal from '../../ui/ConfirmModal.vue'
import ModalDialog from '../../ui/ModalDialog.vue'
import StatusBadge from '../../ui/StatusBadge.vue'
import ErrorStateCard from '../../ui/ErrorStateCard.vue'
import { useNodes } from '../nodes/useNodes'
import { t } from '../../locales'

const {
  groups,
  globalFilter,
  admissionRules,
  loading,
  saving,
  savingGlobalFilter,
  validating,
  validationResult,
  error,
  loadGroups,
  createGroup,
  updateGroup,
  deleteGroup,
  setGroupEdges,
  loadRules,
  createAdmissionRule,
  validateGraph,
  loadGlobalFilter,
  updateGlobalFilter,
} = usePolicy()

const activeTab = ref<'groups' | 'admission'>('groups')

// Global Filter Modal State
const globalFilterModalOpen = ref(false)
const globalConditions = ref<FilterCondition[]>([])
const newGlobalField = ref<FilterField>('display_name')
const newGlobalOp = ref<FilterOp>('contains')
const newGlobalValue = ref('')
const newGlobalProbeKind = ref<'baseline' | 'geo' | 'streaming' | 'ai' | 'speed' | 'ip_risk'>('baseline')
const newGlobalFreshnessSeconds = ref<number | undefined>(undefined)
const globalConditionError = ref('')

watch(newGlobalField, (f) => {
  globalConditionError.value = ''
  if (f === 'display_name' || f === 'source_subscription_ids') {
    newGlobalOp.value = 'contains'
  } else if (f === 'probe_latency_ms') {
    newGlobalOp.value = 'lte'
  } else {
    newGlobalOp.value = 'equals'
  }
})

function openGlobalFilterModal() {
  globalConditions.value = (globalFilter.value?.spec?.conditions || []).map((c) => ({ ...c }))
  globalConditionError.value = ''
  globalFilterModalOpen.value = true
}

function addGlobalCondition() {
  globalConditionError.value = ''
  const cond: FilterCondition = {
    field: newGlobalField.value,
    op: newGlobalOp.value,
    value: newGlobalValue.value.trim(),
  }
  if (newGlobalField.value === 'probe_verdict' || newGlobalField.value === 'probe_latency_ms') {
    cond.probe_kind = newGlobalProbeKind.value
    if (newGlobalFreshnessSeconds.value !== undefined && newGlobalFreshnessSeconds.value > 0) {
      cond.freshness_seconds = Number(newGlobalFreshnessSeconds.value)
    }
  }

  const err = validateConditionInput(cond)
  if (err) {
    globalConditionError.value = err
    return
  }
  if (globalConditions.value.length >= 32) {
    globalConditionError.value = '最多允许添加 32 条筛选条件'
    return
  }

  globalConditions.value.push(cond)
  newGlobalValue.value = ''
}

function removeGlobalCondition(idx: number) {
  globalConditions.value.splice(idx, 1)
}

async function saveGlobalFilter() {
  try {
    await updateGlobalFilter({ conditions: globalConditions.value })
    globalFilterModalOpen.value = false
  } catch {
    // error handled in usePolicy
  }
}

// Group / Edges Sheet State
const sheetOpen = ref(false)
const sheetMode = ref<'group' | 'edges'>('group')
const activeGroup = ref<PolicyGroup | null>(null)
const selectedGroupId = ref<string | null>(null)

function selectGroup(group: PolicyGroup) {
  selectedGroupId.value = group.id
  openEditGroup(group)
}

function handleSheetClose() {
  sheetOpen.value = false
  // Clean up selected group pointer on close so UI doesn't leave stale error state
  selectedGroupId.value = null
}

// Admission Rule Modal State
const ruleModalOpen = ref(false)
const ruleName = ref('')
const ruleExpression = ref('')
const ruleAction = ref<RuleAction>('allow')

function openCreateGroup() {
  activeGroup.value = null
  selectedGroupId.value = null
  sheetMode.value = 'group'
  sheetOpen.value = true
}

function openEditGroup(group: PolicyGroup) {
  activeGroup.value = group
  selectedGroupId.value = group.id
  sheetMode.value = 'group'
  sheetOpen.value = true
}

function openManageEdges(group: PolicyGroup) {
  activeGroup.value = group
  selectedGroupId.value = group.id
  sheetMode.value = 'edges'
  sheetOpen.value = true
}

async function handleSaveGroup(data: { name: string; type: GroupType; nodeFilter?: any }) {
  try {
    if (activeGroup.value) {
      const updated = await updateGroup(activeGroup.value.id, data.name, data.type, data.nodeFilter)
      activeGroup.value = updated
      selectedGroupId.value = updated.id
    } else {
      const created = await createGroup(data.name, data.type, [], data.nodeFilter)
      activeGroup.value = created
      selectedGroupId.value = created.id
    }
    sheetOpen.value = false
  } catch {
    // error handled in usePolicy
  }
}

async function handleSaveEdges(data: { groupId: string; edges: GroupEdge[] }) {
  try {
    await setGroupEdges(data.groupId, data.edges)
    const refreshed = groups.value.find((g) => g.id === data.groupId)
    if (refreshed) {
      activeGroup.value = refreshed
    }
    sheetOpen.value = false
  } catch {
    // error handled in usePolicy
  }
}

const { items: nodeItems, load: loadNodes } = useNodes()
const confirmDeleteGroupOpen = ref(false)
const pendingDeleteGroupId = ref<string | null>(null)
const deletingGroup = ref(false)

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

async function submitAdmissionRule() {
  try {
    await createAdmissionRule({
      name: ruleName.value,
      expression: ruleExpression.value,
      action: ruleAction.value,
    })
    ruleModalOpen.value = false
    ruleName.value = ''
    ruleExpression.value = ''
    ruleAction.value = 'allow'
  } catch {
    // error handled in usePolicy
  }
}

onMounted(() => {
  loadGroups()
  loadRules()
  loadNodes()
  loadGlobalFilter()
})
</script>

<template>
  <section class="space-y-5" aria-labelledby="policy-title">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row sm:items-end justify-between gap-3 w-full min-w-0">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-primary">{{ t('policy.tag') }}</p>
        <h2 id="policy-title" class="mt-1 text-2xl font-bold truncate">{{ t('policy.title') }}</h2>
        <p class="mt-1 text-sm opacity-70">
          {{ t('policy.subtitle') }}
        </p>
      </div>

      <div class="flex flex-wrap items-center gap-2 shrink-0">
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
          class="btn btn-outline btn-sm gap-2"
          :class="{ loading: validating }"
          :disabled="validating"
          @click="validateGraph"
        >
          <CheckBadgeIcon class="w-4 h-4 text-success" />
          {{ t('policy.validateTopology') }}
        </button>

        <button
          v-if="activeTab === 'groups'"
          type="button"
          class="btn btn-primary btn-sm gap-2"
          @click="openCreateGroup"
        >
          <PlusIcon class="w-4 h-4" />
          {{ t('policy.addGroup') }}
        </button>

        <button
          v-else
          type="button"
          class="btn btn-primary btn-sm gap-2"
          @click="ruleModalOpen = true"
        >
          <PlusIcon class="w-4 h-4" />
          {{ t('policy.addRule') }}
        </button>
      </div>
    </div>

    <!-- Error Alert / Degraded State Card -->
    <ErrorStateCard
      v-if="error"
      :error="error"
      :retrying="loading"
      @retry="() => { loadGroups(); loadRules(); }"
    />

    <!-- Graph Validation Result Banner -->
    <Transition
      enter-active-class="transition-all duration-200 ease-out"
      enter-from-class="opacity-0 -translate-y-2"
      enter-to-class="opacity-100 translate-y-0"
    >
      <div
        v-if="validationResult"
        class="p-4 rounded-xl border text-sm flex items-start justify-between gap-3"
        :class="validationResult.valid ? 'bg-success/10 border-success/30 text-success' : 'bg-error/10 border-error/30 text-error'"
      >
        <div class="flex items-start gap-2.5">
          <CheckBadgeIcon v-if="validationResult.valid" class="w-5 h-5 flex-shrink-0 mt-0.5" />
          <ExclamationTriangleIcon v-else class="w-5 h-5 flex-shrink-0 mt-0.5" />
          <div>
            <h4 class="font-bold">
              {{ validationResult.valid ? '拓扑图校验通过' : '拓扑图校验存在冲突' }}
            </h4>
            <p v-if="validationResult.valid" class="text-xs opacity-90 mt-0.5">
              {{ t('policy.validationSuccess') }}
            </p>
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

    <!-- Group & Routing Rule Target Capability Boundary Banner -->
    <div
      data-testid="policy-capability-boundary"
      class="rounded-xl border border-base-300 bg-base-200/70 p-3.5 sm:p-4 text-xs space-y-2.5 min-w-0 w-full"
    >
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-bold text-base-content">编译目标能力边界说明（策略组与分流规则）</span>
        <span class="badge badge-xs badge-ghost font-mono">仅 Mihomo 导出完整策略组与分流规则；其他目标仅导出节点格式（忽略策略组与规则）</span>
      </div>
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

    <!-- Navigation Tabs -->
    <div class="flex flex-wrap items-center gap-2 border-b border-base-300 pb-2 text-xs w-full min-w-0">
      <button
        type="button"
        class="btn btn-sm gap-2"
        :class="activeTab === 'groups' ? 'btn-primary' : 'btn-ghost'"
        @click="activeTab = 'groups'"
      >
        <CpuChipIcon class="w-4 h-4" />
        {{ t('policy.groupsTab') }} ({{ groups.length }})
      </button>
      <button
        type="button"
        class="btn btn-sm gap-2"
        :class="activeTab === 'admission' ? 'btn-primary' : 'btn-ghost'"
        @click="activeTab = 'admission'"
      >
        <ShieldCheckIcon class="w-4 h-4" />
        {{ t('policy.rulesTab') }} ({{ admissionRules.length }})
      </button>
    </div>

    <!-- Tab 1: Policy Groups View -->
    <div v-if="activeTab === 'groups'" class="space-y-4">
      <div v-if="loading && groups.length === 0" class="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,300px),1fr))]">
        <div v-for="i in 6" :key="i" class="skeleton h-32 rounded-box" />
      </div>

      <div
        v-else-if="groups.length === 0"
        class="rounded-box border border-dashed border-base-300 p-12 text-center"
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
          v-for="grp in groups"
          :key="grp.id"
          :group="grp"
          :available-groups="groups"
          :selected="selectedGroupId === grp.id"
          @select="selectGroup"
          @edit="openEditGroup"
          @delete="handleDeleteGroup"
          @manage-edges="openManageEdges"
        />
      </div>
    </div>

    <!-- Tab 2: Admission Rules View -->
    <div v-else class="space-y-4">
      <div
        v-if="admissionRules.length === 0"
        class="rounded-box border border-dashed border-base-300 p-12 text-center"
      >
        <ShieldCheckIcon class="w-10 h-10 mx-auto opacity-40 text-primary" />
        <p class="mt-3 font-semibold text-base">暂无准入规则</p>
        <p class="mt-1 text-sm opacity-60">
          定义节点准入条件以过滤并分类导入的节点（允许、拒绝、隔离）。
        </p>
        <button
          type="button"
          class="btn btn-primary btn-sm mt-4 gap-2"
          @click="ruleModalOpen = true"
        >
          <PlusIcon class="w-4 h-4" />
          {{ t('policy.addRule') }}
        </button>
      </div>

      <div v-else class="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(min(100%,280px),1fr))] w-full min-w-0">
        <article
          v-for="rule in admissionRules"
          :key="rule.id"
          data-testid="admission-rule-card"
          class="card border border-base-300 bg-base-200 shadow-sm p-4 rounded-xl transition-all duration-200 ease-out hover:border-primary/40 active:scale-[0.99] min-w-0 w-full overflow-hidden"
        >
          <div class="flex items-start justify-between gap-2 min-w-0">
            <div class="min-w-0 flex-1">
              <h4 class="font-bold text-sm truncate">{{ rule.name }}</h4>
              <p class="mt-1 font-mono text-xs bg-base-100 p-2 rounded-lg border border-base-300/60 overflow-x-auto whitespace-pre-wrap break-all max-w-full">
                {{ rule.expression }}
              </p>
            </div>
            <StatusBadge
              class="shrink-0"
              :label="ruleActionLabel(rule.action)"
              :tone="ruleActionTone(rule.action)"
            />
          </div>
          <div class="mt-2.5 pt-2 border-t border-base-300/60 text-[11px] opacity-60 flex justify-between font-mono min-w-0">
            <span class="shrink-0">优先级序号：{{ rule.position }}</span>
            <span class="truncate ml-2 text-right">ID: {{ rule.id }}</span>
          </div>
        </article>
      </div>
    </div>

    <!-- Global Filter Modal Dialog -->
    <ModalDialog
      v-model="globalFilterModalOpen"
      title="全局节点筛选"
      description="在所有策略组条件之前评估，在此处被排除的节点将不会进入任何策略组。"
    >
      <div class="space-y-4">
        <div class="p-3 bg-info/10 border border-info/30 rounded-xl text-xs text-info leading-relaxed">
          <p><strong>执行优先级：</strong>硬性风险/准入拒绝 → 全局节点筛选 → 策略组专属筛选条件。</p>
          <p class="mt-1">留空筛选条件时将允许所有通过准入的节点，不做额外全局过滤。</p>
          <p class="mt-1 opacity-80">探针条件仅评估与节点当前凭据版本匹配的有效期观测记录；缺失或过期的观测将按安全闭合（Fail-Closed）原则排除。</p>
        </div>

        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold uppercase opacity-70">已配置条件 ({{ globalConditions.length }})</span>
          <button
            v-if="globalConditions.length > 0"
            type="button"
            class="btn btn-ghost btn-xs text-error gap-1"
            @click="globalConditions = []"
          >
            <TrashIcon class="w-3.5 h-3.5" />
            清空全部
          </button>
        </div>

        <div class="space-y-2 max-h-48 overflow-y-auto">
          <div
            v-for="(cond, cIdx) in globalConditions"
            :key="cIdx"
            class="flex items-center justify-between p-2 rounded-lg bg-base-200 border border-base-300 text-xs font-mono"
          >
            <div class="flex items-center gap-1.5 truncate min-w-0">
              <span class="badge badge-xs badge-outline">{{ cond.field }}</span>
              <span class="text-primary font-semibold">{{ cond.op }}</span>
              <span class="truncate">"{{ cond.value }}"</span>
              <span v-if="cond.probe_kind" class="badge badge-xs badge-ghost">
                {{ cond.probe_kind }}
              </span>
              <span v-if="cond.freshness_seconds" class="opacity-60 text-[10px]">
                ≤{{ cond.freshness_seconds }}s
              </span>
            </div>
            <button
              type="button"
              class="btn btn-ghost btn-xs text-error p-1"
              @click="removeGlobalCondition(cIdx)"
            >
              <TrashIcon class="w-3.5 h-3.5" />
            </button>
          </div>
          <p v-if="globalConditions.length === 0" class="text-xs opacity-50 italic text-center py-2">
            暂未配置全局筛选条件（默认允许所有节点）。
          </p>
        </div>

        <!-- Add Condition Inline Control -->
        <div class="p-3 rounded-xl bg-base-200/60 border border-base-300 space-y-2">
          <span class="font-semibold text-xs block">添加全局筛选条件</span>
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-2">
            <select
              v-model="newGlobalField"
              class="select select-bordered select-xs"
            >
              <option
                v-for="f in SUPPORTED_FILTER_FIELDS"
                :key="f.field"
                :value="f.field"
              >
                {{ f.label }}
              </option>
            </select>

            <select
              v-model="newGlobalOp"
              class="select select-bordered select-xs font-mono"
            >
              <template v-if="newGlobalField === 'display_name' || newGlobalField === 'source_subscription_ids'">
                <option value="contains">包含 (contains)</option>
                <option value="not_contains">不包含 (not_contains)</option>
              </template>
              <template v-else-if="newGlobalField === 'probe_latency_ms'">
                <option value="lte">小于等于 (lte)</option>
              </template>
              <template v-else>
                <option value="equals">等于 (equals)</option>
                <option value="not_equals">不等于 (not_equals)</option>
              </template>
            </select>

            <select
              v-if="newGlobalField === 'probe_verdict'"
              v-model="newGlobalValue"
              class="select select-bordered select-xs"
            >
              <option value="available">可用 (available)</option>
              <option value="restricted">受限 (restricted)</option>
              <option value="unknown">未知 (unknown)</option>
              <option value="error">错误 (error)</option>
              <option value="stale">已过期 (stale)</option>
            </select>
            <input
              v-else
              v-model="newGlobalValue"
              class="input input-bordered input-xs"
              placeholder="匹配目标值..."
            />
          </div>

          <!-- Extra fields for probe kinds -->
          <div
            v-if="newGlobalField === 'probe_verdict' || newGlobalField === 'probe_latency_ms'"
            class="grid grid-cols-2 gap-2"
          >
            <select
              v-model="newGlobalProbeKind"
              class="select select-bordered select-xs"
            >
              <option value="baseline">基础连通性 (baseline)</option>
              <option value="geo">地域与出口 IP (geo)</option>
              <option value="streaming">流媒体解锁 (streaming)</option>
              <option value="ai">AI 服务可用性 (ai)</option>
              <option value="speed">带宽测速 (speed)</option>
              <option value="ip_risk">IP 风险度 (ip_risk)</option>
            </select>

            <input
              v-model.number="newGlobalFreshnessSeconds"
              type="number"
              placeholder="最大有效期（秒，可选）"
              class="input input-bordered input-xs font-mono"
            />
          </div>

          <div class="flex items-center justify-between pt-1">
            <span v-if="globalConditionError" class="text-error text-xs">
              {{ globalConditionError }}
            </span>
            <span v-else />
            <button
              type="button"
              class="btn btn-outline btn-xs gap-1 ml-auto"
              @click="addGlobalCondition"
            >
              <PlusIcon class="w-3.5 h-3.5" />
              添加条件
            </button>
          </div>
        </div>

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
      :all-groups="groups"
      :available-nodes="nodeItems"
      :mode="sheetMode"
      :saving="saving"
      @close="handleSheetClose"
      @save-group="handleSaveGroup"
      @save-edges="handleSaveEdges"
    />

    <!-- Add Admission Rule Modal -->
    <ModalDialog
      v-model="ruleModalOpen"
      title="新建准入规则"
      description="准入规则在节点入库前评估节点属性并决定放行、拒绝或隔离"
    >
      <form class="space-y-4" @submit.prevent="submitAdmissionRule">
        <label class="form-control">
          <span class="label-text text-xs font-semibold">规则名称</span>
          <input
            v-model="ruleName"
            required
            placeholder="例如：拒绝低速节点 或 允许香港节点"
            class="input input-bordered input-sm mt-1"
          />
        </label>

        <label class="form-control">
          <span class="label-text text-xs font-semibold">规则表达式</span>
          <input
            v-model="ruleExpression"
            required
            placeholder="例如：country in ['HK', 'TW'] && protocol == 'ss'"
            class="input input-bordered input-sm font-mono text-xs mt-1"
          />
        </label>

        <div>
          <span class="label-text text-xs font-semibold block mb-1.5">准入动作</span>
          <div class="flex gap-2">
            <label
              v-for="act in (['allow', 'reject', 'quarantine'] as RuleAction[])"
              :key="act"
              class="flex items-center gap-2 p-2 px-3 rounded-lg border border-base-300 bg-base-200/50 cursor-pointer text-xs font-medium"
              :class="{ 'border-primary bg-primary/10 text-primary': ruleAction === act }"
            >
              <input
                v-model="ruleAction"
                type="radio"
                :value="act"
                class="radio radio-primary radio-xs"
              />
              <span>{{ ruleActionLabel(act) }}</span>
            </label>
          </div>
        </div>

        <div class="modal-action border-t border-base-300 pt-3">
          <button type="button" class="btn btn-ghost btn-sm" @click="ruleModalOpen = false">
            取消
          </button>
          <button
            type="submit"
            class="btn btn-primary btn-sm"
            :disabled="!ruleName.trim() || !ruleExpression.trim()"
          >
            创建规则
          </button>
        </div>
      </form>
    </ModalDialog>

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

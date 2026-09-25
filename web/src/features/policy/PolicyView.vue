<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  ArrowPathIcon,
  PlusIcon,
  CheckBadgeIcon,
  ExclamationTriangleIcon,
  ShieldCheckIcon,
  CpuChipIcon,
  Squares2X2Icon,
  FunnelIcon,
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
  NodeFilterSpec,
  PolicyGroup,
  RuleAction,
} from './policyTypes'
import {
  SUPPORTED_FILTER_FIELDS,
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
  policyRules,
  loading,
  loadingGlobalFilter,
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
    globalConditionError.value = 'Maximum 32 filter conditions allowed'
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
              {{ validationResult.valid ? 'Topology Validated Successfully' : 'Graph Topology Violation' }}
            </h4>
            <p v-if="validationResult.valid" class="text-xs opacity-90 mt-0.5">
              Policy groups and routing rules form an acyclic, well-formed directed acyclic graph.
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
          Create First Group
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
        <p class="mt-3 font-semibold text-base">No admission rules configured</p>
        <p class="mt-1 text-sm opacity-60">
          Define admission criteria to filter and classify imported nodes (allow, reject, quarantine).
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
              :label="rule.action.toUpperCase()"
              :tone="ruleActionTone(rule.action)"
            />
          </div>
          <div class="mt-2.5 pt-2 border-t border-base-300/60 text-[11px] opacity-60 flex justify-between font-mono min-w-0">
            <span class="shrink-0">Position: {{ rule.position }}</span>
            <span class="truncate ml-2 text-right">ID: {{ rule.id }}</span>
          </div>
        </article>
      </div>
    </div>

    <!-- Global Filter Modal Dialog -->
    <ModalDialog
      v-model="globalFilterModalOpen"
      title="Global Node Filter"
      description="Evaluates before any policy group conditions. Nodes rejected here are excluded from all policy groups."
    >
      <div class="space-y-4">
        <div class="p-3 bg-info/10 border border-info/30 rounded-xl text-xs text-info leading-relaxed">
          <p><strong>Order of Precedence:</strong> Hard Risk/Admission Rejection → Global Filter → Group Conditions.</p>
          <p class="mt-1">Empty conditions maintain legacy full compatibility (all nodes allowed).</p>
          <p class="mt-1 opacity-80">Probe conditions evaluate only fresh observations matching the node's credential version; missing or stale observations fail closed.</p>
        </div>

        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold uppercase opacity-70">Configured Conditions ({{ globalConditions.length }})</span>
          <button
            v-if="globalConditions.length > 0"
            type="button"
            class="btn btn-ghost btn-xs text-error gap-1"
            @click="globalConditions = []"
          >
            <TrashIcon class="w-3.5 h-3.5" />
            Clear All
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
            No global conditions configured (all nodes allowed).
          </p>
        </div>

        <!-- Add Condition Inline Control -->
        <div class="p-3 rounded-xl bg-base-200/60 border border-base-300 space-y-2">
          <span class="font-semibold text-xs block">Add Global Filter Condition</span>
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
                <option value="contains">contains</option>
                <option value="not_contains">not_contains</option>
              </template>
              <template v-else-if="newGlobalField === 'probe_latency_ms'">
                <option value="lte">&lt;= (lte)</option>
              </template>
              <template v-else>
                <option value="equals">equals</option>
                <option value="not_equals">not_equals</option>
              </template>
            </select>

            <select
              v-if="newGlobalField === 'probe_verdict'"
              v-model="newGlobalValue"
              class="select select-bordered select-xs"
            >
              <option value="available">available</option>
              <option value="restricted">restricted</option>
              <option value="unknown">unknown</option>
              <option value="error">error</option>
              <option value="stale">stale</option>
            </select>
            <input
              v-else
              v-model="newGlobalValue"
              class="input input-bordered input-xs"
              placeholder="Target value..."
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
              <option value="baseline">baseline</option>
              <option value="geo">geo</option>
              <option value="streaming">streaming</option>
              <option value="ai">ai</option>
              <option value="speed">speed</option>
              <option value="ip_risk">ip_risk</option>
            </select>

            <input
              v-model.number="newGlobalFreshnessSeconds"
              type="number"
              placeholder="Freshness (s, optional)"
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
              Add Condition
            </button>
          </div>
        </div>

        <div class="modal-action border-t border-base-300 pt-3">
          <button type="button" class="btn btn-ghost btn-sm" @click="globalFilterModalOpen = false">
            Cancel
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm gap-2"
            :class="{ loading: savingGlobalFilter }"
            :disabled="savingGlobalFilter"
            @click="saveGlobalFilter"
          >
            Save Global Filter
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
      title="Create Admission Rule"
      description="Rules evaluate incoming node attributes before placing them into inventory"
    >
      <form class="space-y-4" @submit.prevent="submitAdmissionRule">
        <label class="form-control">
          <span class="label-text text-xs font-semibold">Rule Name</span>
          <input
            v-model="ruleName"
            required
            placeholder="e.g. Reject Low Speed or Allow HK"
            class="input input-bordered input-sm mt-1"
          />
        </label>

        <label class="form-control">
          <span class="label-text text-xs font-semibold">Expression</span>
          <input
            v-model="ruleExpression"
            required
            placeholder="e.g. country in ['HK', 'TW'] && protocol == 'ss'"
            class="input input-bordered input-sm font-mono text-xs mt-1"
          />
        </label>

        <div>
          <span class="label-text text-xs font-semibold block mb-1.5">Action</span>
          <div class="flex gap-2">
            <label
              v-for="act in (['allow', 'reject', 'quarantine'] as RuleAction[])"
              :key="act"
              class="flex items-center gap-2 p-2 px-3 rounded-lg border border-base-300 bg-base-200/50 cursor-pointer text-xs uppercase font-medium"
              :class="{ 'border-primary bg-primary/10 text-primary': ruleAction === act }"
            >
              <input
                v-model="ruleAction"
                type="radio"
                :value="act"
                class="radio radio-primary radio-xs"
              />
              <span>{{ act }}</span>
            </label>
          </div>
        </div>

        <div class="modal-action border-t border-base-300 pt-3">
          <button type="button" class="btn btn-ghost btn-sm" @click="ruleModalOpen = false">
            Cancel
          </button>
          <button
            type="submit"
            class="btn btn-primary btn-sm"
            :disabled="!ruleName.trim() || !ruleExpression.trim()"
          >
            Create Rule
          </button>
        </div>
      </form>
    </ModalDialog>

    <!-- Confirm Delete Policy Group Modal -->
    <ConfirmModal
      v-model="confirmDeleteGroupOpen"
      title="Delete Policy Group"
      message="Are you sure you want to delete this policy group? Associated directed edges and routing references will be removed."
      confirm-text="Delete Group"
      cancel-text="Cancel"
      tone="danger"
      :loading="deletingGroup"
      @confirm="handleConfirmDeleteGroup"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  CpuChipIcon,
  ChevronDownIcon,
  PencilSquareIcon,
  TrashIcon,
  PlusIcon,
  ShareIcon,
} from '@heroicons/vue/24/outline'
import type { GroupEdge, PolicyGroup } from './policyTypes'
import { groupTypeLabel } from './policyTypes'
import StatusBadge from '../../ui/StatusBadge.vue'

const props = defineProps<{
  group: PolicyGroup
  availableGroups?: PolicyGroup[]
  selected?: boolean
}>()

const emit = defineEmits<{
  (e: 'select', group: PolicyGroup): void
  (e: 'edit', group: PolicyGroup): void
  (e: 'delete', id: string): void
  (e: 'manage-edges', group: PolicyGroup): void
}>()

const expanded = ref(false)

const edgeCount = computed(() => props.group.edges?.length || 0)

function handleHeaderClick() {
  emit('select', props.group)
  expanded.value = !expanded.value
}
</script>

<template>
  <article
    class="card border bg-base-200 shadow-sm transition-all duration-200 ease-out hover:border-primary/50 hover:shadow-md min-w-0 max-w-full overflow-hidden"
    :class="selected ? 'border-primary ring-2 ring-primary/30 shadow-md bg-primary/[0.03]' : 'border-base-300'"
    data-testid="group-card"
  >
    <div class="card-body p-4 sm:p-5 gap-3 min-w-0 max-w-full overflow-hidden">
      <!-- Card Header with Tap-to-Select and Grow -->
      <div
        class="flex items-start justify-between gap-3 cursor-pointer select-none active:scale-[0.99] transition-transform duration-200 ease-out min-w-0"
        @click="handleHeaderClick"
      >
        <div class="flex items-center gap-3 min-w-0 flex-1">
          <div
            class="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 shrink-0"
            :class="{
              'bg-primary/10 text-primary': group.group_type === 'select',
              'bg-secondary/10 text-secondary': group.group_type === 'urltest',
              'bg-accent/10 text-accent': group.group_type === 'fallback',
              'bg-info/10 text-info': group.group_type === 'loadbalance'
            }"
          >
            <CpuChipIcon class="w-5 h-5" />
          </div>
          <div class="min-w-0 flex-1">
            <h3 class="font-bold text-sm sm:text-base truncate">{{ group.name }}</h3>
            <p class="mt-0.5 font-mono text-[11px] opacity-60 truncate">
              ID: {{ group.id }}
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2 flex-shrink-0 shrink-0">
          <StatusBadge
            :label="groupTypeLabel(group.group_type)"
            :tone="'info'"
          />
          <div
            class="w-7 h-7 rounded-lg flex items-center justify-center bg-base-300/60 transition-transform duration-200 ease-out shrink-0"
            :class="{ 'rotate-180': expanded }"
          >
            <ChevronDownIcon class="w-4 h-4" />
          </div>
        </div>
      </div>

      <!-- Edge Count Indicator and Action Bar wrap cleanly on narrow widths -->
      <div class="flex flex-wrap items-center justify-between gap-2 text-xs opacity-70 border-t border-base-300 pt-2.5 min-w-0">
        <div class="flex flex-wrap items-center gap-2 min-w-0">
          <span class="flex items-center gap-1.5 font-mono min-w-0 truncate shrink-0">
            <ShareIcon class="w-3.5 h-3.5 text-primary shrink-0" />
            <span class="truncate">{{ edgeCount }} connected {{ edgeCount === 1 ? 'edge' : 'edges' }}</span>
          </span>
          <span
            v-if="group.node_filter && group.node_filter.conditions && group.node_filter.conditions.length > 0"
            class="badge badge-xs badge-primary font-mono"
            title="Custom group node filter active"
          >
            {{ group.node_filter.conditions.length }} filter conds
          </span>
        </div>

        <div class="flex flex-wrap items-center gap-1 shrink-0" @click.stop>
          <button
            type="button"
            class="btn btn-ghost btn-xs gap-1 touch-manipulation"
            title="Manage edges"
            @click="emit('manage-edges', group)"
          >
            <PlusIcon class="w-3.5 h-3.5 shrink-0" />
            Edges
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs gap-1 touch-manipulation"
            title="Edit group"
            @click="emit('edit', group)"
          >
            <PencilSquareIcon class="w-3.5 h-3.5 shrink-0" />
          </button>
          <button
            type="button"
            class="btn btn-ghost btn-xs text-error gap-1 touch-manipulation"
            title="Delete group"
            @click="emit('delete', group.id)"
          >
            <TrashIcon class="w-3.5 h-3.5 shrink-0" />
          </button>
        </div>
      </div>

      <!-- Expandable Edges Container (Smooth 200ms ease-out) -->
      <div
        v-if="expanded"
        class="mt-1 pt-3 border-t border-base-300/80 space-y-2 transition-all duration-200 ease-out min-w-0 overflow-hidden"
      >
        <!-- Group Filter Conditions details if configured -->
        <div
          v-if="group.node_filter && group.node_filter.conditions && group.node_filter.conditions.length > 0"
          class="p-2.5 rounded-lg bg-base-100 border border-base-300/80 text-xs font-mono space-y-1"
        >
          <div class="flex items-center justify-between text-[11px] font-sans opacity-70">
            <span>Group Filter (applied after Global Filter):</span>
            <span v-if="!group.edges || group.edges.length === 0" class="badge badge-xs badge-info font-semibold">
              Dynamic Pool
            </span>
          </div>
          <div class="flex flex-wrap gap-1">
            <span
              v-for="(c, cIdx) in group.node_filter.conditions"
              :key="cIdx"
              class="badge badge-xs badge-outline"
            >
              {{ c.field }} {{ c.op }} "{{ c.value }}"
              <span v-if="c.probe_kind" class="opacity-75 ml-1">({{ c.probe_kind }})</span>
            </span>
          </div>
        </div>

        <div v-if="!group.edges || group.edges.length === 0" class="text-xs opacity-50 italic py-1">
          <span v-if="group.node_filter && group.node_filter.conditions && group.node_filter.conditions.length > 0">
            No explicit node edges: group dynamically selects matching candidates from global pool.
          </span>
          <span v-else>
            No edges configured for this group. Click "Edges" to link child groups or nodes.
          </span>
        </div>

        <div
          v-for="(edge, idx) in group.edges"
          :key="edge.id || idx"
          class="flex items-center justify-between p-2 rounded-lg bg-base-100 border border-base-300/80 text-xs font-mono min-w-0 overflow-hidden"
        >
          <div class="flex items-center gap-2 min-w-0 flex-1">
            <span class="badge badge-xs badge-ghost font-semibold shrink-0">{{ edge.position }}</span>
            <span v-if="edge.child_group_id" class="text-secondary truncate min-w-0 flex-1">
              Group: {{ edge.child_group_id }}
            </span>
            <span v-else-if="edge.node_logical_id" class="text-primary truncate min-w-0 flex-1">
              Node: {{ edge.node_logical_id }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </article>
</template>

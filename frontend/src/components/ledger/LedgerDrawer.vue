<template>
  <BaseDrawer :model-value="open" :title="node ? node.name : '节点详情'" @close="$emit('close')">
    <div v-if="node" class="space-y-6 text-xs font-mono">
      <!-- Node Basic Info -->
      <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-2">
        <div class="flex justify-between items-center pb-2 border-b border-white/5">
          <span class="text-slate-400">PROTOCOL</span>
          <StatusBadge type="info" :text="(node.type || 'unknown').toUpperCase()" />
        </div>
        <div class="flex justify-between items-center py-1">
          <span class="text-slate-400">SERVER</span>
          <span class="text-white select-all">{{ node.server }}</span>
        </div>
        <div class="flex justify-between items-center py-1">
          <span class="text-slate-400">PORT</span>
          <span class="text-white">{{ node.port }}</span>
        </div>
      </div>

      <!-- Outbound & Probing Results -->
      <div class="rounded-xl border border-white/10 bg-slate-900/60 p-4 space-y-3">
        <div class="text-slate-300 font-semibold border-b border-white/5 pb-2 flex items-center justify-between">
          <span>PROBE & CAPABILITIES</span>
          <StatusBadge
            :type="node.probe_status === 'success' ? 'success' : (node.probe_status === 'fail' ? 'danger' : 'neutral')"
            :text="node.probe_status || 'untested'"
          />
        </div>
        <div class="flex justify-between items-center py-1">
          <span class="text-slate-400">LATENCY</span>
          <span :class="node.latency ? 'text-emerald-400' : 'text-slate-500'">
            {{ node.latency ? `${node.latency} ms` : 'N/A' }}
          </span>
        </div>
        <div class="flex justify-between items-center py-1">
          <span class="text-slate-400">OUTBOUND IP</span>
          <span class="text-white font-mono">{{ node.outbound_ip || 'N/A' }}</span>
        </div>
        <div class="flex justify-between items-center py-1">
          <span class="text-slate-400">COUNTRY / REGION</span>
          <span class="text-white">{{ node.country || 'N/A' }}</span>
        </div>
        <div class="flex justify-between items-center py-1">
          <span class="text-slate-400">SPEED (DOWN)</span>
          <span :class="node.speed_mbps ? 'text-cyan-400' : 'text-slate-500'">
            {{ node.speed_mbps ? `${node.speed_mbps} Mbps` : 'N/A' }}
          </span>
        </div>
      </div>

      <!-- Action Buttons -->
      <div class="flex gap-3 pt-2">
        <button
          class="flex-1 py-2.5 rounded-lg border border-blue-500/30 bg-blue-600/20 text-blue-400 font-medium hover:bg-blue-600/30 transition-colors cursor-pointer text-center"
          @click="$emit('probe-single', node)"
        >
          ⚡ 单节点测速质检
        </button>
      </div>
    </div>
  </BaseDrawer>
</template>

<script setup lang="ts">
import BaseDrawer from '../ui/BaseDrawer.vue'
import StatusBadge from '../ui/StatusBadge.vue'

defineProps<{
  open: boolean
  node: any
}>()

defineEmits<{
  (e: 'close'): void
  (e: 'probe-single', node: any): void
}>()
</script>

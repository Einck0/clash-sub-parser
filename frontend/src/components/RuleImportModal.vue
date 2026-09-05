<template>
  <AppModal
    :model-value="true"
    size="md"
    title="批量导入规则"
    @update:model-value="(val) => !val && $emit('close')"
    @close="$emit('close')"
  >
    <div class="flex flex-col gap-4">
      <div class="text-xs text-text-muted">
        粘贴 Clash YAML rules 或协议链接，自动解析为规则列表。
      </div>

      <!-- Import Mode Tabs -->
      <div class="grid grid-cols-3 gap-1 rounded-lg bg-surface-base p-1 border border-border-subtle">
        <button
          type="button"
          :class="[
            'py-1.5 px-3 rounded-md text-xs font-medium transition-colors cursor-pointer text-center',
            mode === 'yaml'
              ? 'bg-accent text-white shadow-xs'
              : 'text-text-muted hover:text-text-main hover:bg-surface-hover'
          ]"
          @click="mode = 'yaml'"
        >
          YAML Rules
        </button>
        <button
          type="button"
          :class="[
            'py-1.5 px-3 rounded-md text-xs font-medium transition-colors cursor-pointer text-center',
            mode === 'links'
              ? 'bg-accent text-white shadow-xs'
              : 'text-text-muted hover:text-text-main hover:bg-surface-hover'
          ]"
          @click="mode = 'links'"
        >
          协议链接
        </button>
        <button
          type="button"
          :class="[
            'py-1.5 px-3 rounded-md text-xs font-medium transition-colors cursor-pointer text-center',
            mode === 'text'
              ? 'bg-accent text-white shadow-xs'
              : 'text-text-muted hover:text-text-main hover:bg-surface-hover'
          ]"
          @click="mode = 'text'"
        >
          文本规则
        </button>
      </div>

      <!-- Textarea Input -->
      <textarea
        v-model="input"
        :placeholder="placeholders[mode]"
        rows="8"
        class="w-full rounded-md border border-border-subtle bg-surface-hover p-3 font-mono text-xs text-text-main placeholder:text-text-muted focus:border-accent focus:outline-hidden resize-y"
      ></textarea>

      <!-- Options Row -->
      <div class="flex flex-wrap items-end gap-3">
        <label class="flex flex-col gap-1 text-xs text-text-muted flex-1 min-w-[140px]">
          <span>目标代理</span>
          <select
            v-model="proxy"
            class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          >
            <option value="">选择目标</option>
            <option v-for="p in proxyOptions" :key="p" :value="p">{{ p }}</option>
          </select>
        </label>

        <label class="flex flex-col gap-1 text-xs text-text-muted flex-1 min-w-[140px]">
          <span>目标分类</span>
          <select
            v-model="category"
            class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          >
            <option value="">使用默认</option>
            <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
          </select>
        </label>

        <button
          type="button"
          class="px-4 py-2 rounded-md bg-accent text-xs font-medium text-white hover:bg-accent-hover disabled:opacity-50 transition-colors cursor-pointer shrink-0"
          :disabled="!input.trim() || !proxy"
          @click="parseInput"
        >
          解析
        </button>
      </div>

      <!-- Preview Section -->
      <div v-if="parsed.length" class="flex flex-col gap-2 pt-3 border-t border-border-subtle">
        <div class="flex items-center justify-between">
          <strong class="text-xs text-text-main">解析结果：{{ parsed.length }} 条规则</strong>
          <div class="flex items-center gap-2 text-xs">
            <span v-if="parseSkipped" class="text-text-muted">{{ parseSkipped }} 行无法解析已跳过</span>
            <button
              type="button"
              class="text-accent hover:underline cursor-pointer"
              @click="selectAll"
            >
              全选
            </button>
            <button
              type="button"
              class="text-text-muted hover:text-text-main cursor-pointer"
              @click="deselectAll"
            >
              全不选
            </button>
          </div>
        </div>

        <div class="max-h-60 overflow-y-auto divide-y divide-border-subtle rounded-md border border-border-subtle bg-surface-base p-1">
          <label
            v-for="(rule, idx) in parsed"
            :key="idx"
            :class="[
              'flex items-center gap-2 p-2 text-xs cursor-pointer rounded-sm transition-colors',
              selected.has(idx) ? 'bg-accent/10' : 'hover:bg-surface-hover'
            ]"
          >
            <input
              type="checkbox"
              :checked="selected.has(idx)"
              class="accent-accent cursor-pointer"
              @change="toggle(idx)"
            />
            <span class="rounded bg-surface-hover px-1.5 py-0.5 font-mono text-[10px] text-text-muted border border-border-subtle">{{ rule.type }}</span>
            <code class="flex-1 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-xs text-text-main">{{ rule.value }}</code>
            <span class="text-text-muted text-[11px]">→ {{ rule.proxy }}</span>
          </label>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-between">
        <span class="text-xs text-text-muted">
          {{ selected.size > 0 ? `将添加 ${selected.size} 条规则到当前列表末尾` : '' }}
        </span>
        <div class="flex gap-2">
          <button
            type="button"
            class="px-3 py-1.5 rounded-md border border-border-subtle text-xs text-text-muted hover:text-text-main cursor-pointer"
            @click="$emit('close')"
          >
            取消
          </button>
          <button
            type="button"
            class="px-4 py-1.5 rounded-md bg-accent text-xs font-medium text-white hover:bg-accent-hover disabled:opacity-50 transition-colors cursor-pointer"
            :disabled="selected.size === 0"
            @click="applySelected"
          >
            应用 {{ selected.size }} 条规则
          </button>
        </div>
      </div>
    </template>
  </AppModal>
</template>

<script setup>
import { ref } from 'vue'
import AppModal from './ui/AppModal.vue'

const props = defineProps({
  proxyOptions: { type: Array, default: () => ['DIRECT', 'PROXY', 'REJECT'] },
  categories: { type: Array, default: () => [] },
})

const emit = defineEmits(['apply', 'close'])

const mode = ref('yaml')
const input = ref('')
const proxy = ref('PROXY')
const category = ref('')
const parsed = ref([])
const selected = ref(new Set())
const parseSkipped = ref(0)

const placeholders = {
  yaml: `粘贴 Clash YAML 格式的 rules:\n\nrules:\n  - DOMAIN-SUFFIX,google.com,PROXY\n  - DOMAIN-KEYWORD,facebook,PROXY\n  - GEOIP,CN,DIRECT\n  - MATCH,PROXY`,
  links: `粘贴协议链接（每行一个）:\n\nss://...\ntrojan://...\nvless://...\nvmess://...`,
  text: `粘贴文本规则（每行一条）:\n\nDOMAIN-SUFFIX,google.com\nDOMAIN-KEYWORD,facebook\nIP-CIDR,10.0.0.0/8`,
}

function parseInput() {
  const text = input.value.trim()
  if (!text) return

  const rules = []
  let skipped = 0

  if (mode.value === 'yaml') {
    // Parse Clash YAML rules format
    for (const line of text.split('\n')) {
      const trimmed = line.trim()
      if (!trimmed || trimmed.startsWith('#') || trimmed === 'rules:') continue
      // Remove leading "- " if present
      const clean = trimmed.replace(/^-\s*/, '')
      const parts = clean.split(',').map((s) => s.trim()).filter(Boolean)
      if (parts.length >= 2) {
        const type = parts[0].toUpperCase()
        if (type === 'MATCH') {
          rules.push({ type, value: '', proxy: proxy.value || parts[1], options: parts.slice(2) })
        } else if (parts.length >= 3) {
          rules.push({ type, value: parts[1], proxy: proxy.value || parts[2], options: parts.slice(3) })
        }
      }
    }
  } else if (mode.value === 'text') {
    for (const line of text.split('\n')) {
      const trimmed = line.trim()
      if (!trimmed || trimmed.startsWith('#')) continue
      const parts = trimmed.split(',').map((s) => s.trim()).filter(Boolean)
      if (parts.length >= 2) {
        const type = parts[0].toUpperCase()
        rules.push({
          type,
          value: type === 'MATCH' ? '' : parts[1],
          proxy: proxy.value || (type === 'MATCH' ? parts[1] : parts[2] || ''),
          options: type === 'MATCH' ? parts.slice(2) : parts.slice(3),
        })
      }
    }
  } else if (mode.value === 'links') {
    // For links, we just count them - actual parsing is for subscription nodes, not rules
    // But we can create DOMAIN rules from link hostnames
    for (const line of text.split('\n')) {
      const trimmed = line.trim()
      if (!trimmed) continue
      try {
        const url = new URL(trimmed.split('#')[0])
        if (url.hostname && /^[a-z0-9.-]+$/i.test(url.hostname)) {
          rules.push({
            type: 'DOMAIN-SUFFIX',
            value: url.hostname,
            proxy: proxy.value,
            options: [],
          })
        } else {
          skipped += 1
        }
      } catch {
        skipped += 1
      }
    }
  }

  parsed.value = rules
  parseSkipped.value = skipped
  selected.value = new Set(rules.map((_, i) => i))
}

function toggle(idx) {
  const s = new Set(selected.value)
  if (s.has(idx)) s.delete(idx)
  else s.add(idx)
  selected.value = s
}

function selectAll() {
  selected.value = new Set(parsed.value.map((_, i) => i))
}

function deselectAll() {
  selected.value = new Set()
}

function applySelected() {
  const rules = [...selected.value]
    .sort((a, b) => a - b)
    .map((idx) => {
      const r = parsed.value[idx]
      return {
        type: r.type,
        value: r.value,
        proxy: r.proxy,
        name: '',
        category: category.value || 'default',
        enabled: true,
        options: r.options || [],
      }
    })
  emit('apply', rules)
  emit('close')
}
</script>

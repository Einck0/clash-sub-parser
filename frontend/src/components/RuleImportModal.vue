<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal import-modal">
      <div class="row space">
        <div>
          <p class="eyebrow">Batch Import</p>
          <h3>批量导入规则</h3>
          <p class="section-hint">粘贴 Clash YAML rules 或协议链接，自动解析为规则列表。</p>
        </div>
        <button @click="$emit('close')">关闭</button>
      </div>

      <div class="import-tabs">
        <button :class="{ active: mode === 'yaml' }" @click="mode = 'yaml'">YAML Rules</button>
        <button :class="{ active: mode === 'links' }" @click="mode = 'links'">协议链接</button>
        <button :class="{ active: mode === 'text' }" @click="mode = 'text'">文本规则</button>
      </div>

      <div class="import-body">
        <textarea
          v-model="input"
          :placeholder="placeholders[mode]"
          rows="10"
          class="import-textarea"
        ></textarea>

        <div class="import-options">
          <label class="field">
            <span>目标代理</span>
            <select v-model="proxy">
              <option value="">选择目标</option>
              <option v-for="p in proxyOptions" :key="p" :value="p">{{ p }}</option>
            </select>
          </label>
          <label class="field">
            <span>目标分类</span>
            <select v-model="category">
              <option value="">使用默认</option>
              <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
            </select>
          </label>
          <button class="primary" @click="parseInput" :disabled="!input.trim() || !proxy">解析</button>
        </div>
      </div>

      <div v-if="parsed.length" class="import-preview">
        <div class="row space">
          <strong>解析结果：{{ parsed.length }} 条规则</strong>
          <div class="row" style="gap:6px">
            <button @click="selectAll">全选</button>
            <button @click="deselectAll">全不选</button>
          </div>
        </div>
        <div class="import-rules-list">
          <label
            v-for="(rule, idx) in parsed"
            :key="idx"
            class="import-rule-row"
            :class="{ selected: selected.has(idx) }"
          >
            <input type="checkbox" :checked="selected.has(idx)" @change="toggle(idx)" />
            <span class="badge">{{ rule.type }}</span>
            <code>{{ rule.value }}</code>
            <span class="muted">→ {{ rule.proxy }}</span>
          </label>
        </div>
      </div>

      <div class="row" style="margin-top:12px" v-if="selected.size > 0">
        <button class="primary" @click="applySelected">
          应用 {{ selected.size }} 条规则
        </button>
        <span class="muted">将添加到当前规则列表末尾</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

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

const placeholders = {
  yaml: `粘贴 Clash YAML 格式的 rules:\n\nrules:\n  - DOMAIN-SUFFIX,google.com,PROXY\n  - DOMAIN-KEYWORD,facebook,PROXY\n  - GEOIP,CN,DIRECT\n  - MATCH,PROXY`,
  links: `粘贴协议链接（每行一个）:\n\nss://...\ntrojan://...\nvless://...\nvmess://...`,
  text: `粘贴文本规则（每行一条）:\n\nDOMAIN-SUFFIX,google.com\nDOMAIN-KEYWORD,facebook\nIP-CIDR,10.0.0.0/8`,
}

function parseInput() {
  const text = input.value.trim()
  if (!text) return

  const rules = []

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
        if (url.hostname) {
          rules.push({
            type: 'DOMAIN-SUFFIX',
            value: url.hostname,
            proxy: proxy.value,
            options: [],
          })
        }
      } catch {}
    }
  }

  parsed.value = rules
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

<style scoped>
.import-modal {
  max-width: 700px;
  max-height: 85vh;
  overflow-y: auto;
}
.import-tabs {
  display: flex;
  gap: 4px;
  margin: 12px 0 8px;
}
.import-tabs button {
  padding: 6px 14px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: transparent;
  color: var(--text-0);
  cursor: pointer;
  font-size: 13px;
}
.import-tabs button.active {
  background: var(--accent, #6366f1);
  color: #fff;
  border-color: var(--accent, #6366f1);
}
.import-textarea {
  width: 100%;
  font-family: monospace;
  font-size: 13px;
  resize: vertical;
}
.import-options {
  display: flex;
  gap: 10px;
  align-items: end;
  margin-top: 8px;
  flex-wrap: wrap;
}
.import-preview {
  margin-top: 12px;
  border-top: 1px solid var(--border);
  padding-top: 12px;
}
.import-rules-list {
  max-height: 240px;
  overflow-y: auto;
  margin-top: 8px;
}
.import-rule-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  cursor: pointer;
  font-size: 13px;
  border-bottom: 1px solid var(--border);
}
.import-rule-row.selected {
  background: rgba(99, 102, 241, 0.05);
}
.import-rule-row code {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

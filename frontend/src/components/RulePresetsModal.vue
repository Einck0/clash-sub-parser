<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal preset-modal">
      <div class="row space">
        <div>
          <p class="eyebrow">Rule Presets</p>
          <h3>规则模板</h3>
          <p class="section-hint">选择常用规则模板快速添加，proxy 目标可在应用前修改。</p>
        </div>
        <button @click="$emit('close')">关闭</button>
      </div>

      <div class="preset-controls">
        <input v-model="search" placeholder="搜索模板（如 openai、youtube）" class="preset-search" />
        <label class="field">
          <span>目标代理</span>
          <select v-model="proxyOverride">
            <option value="">使用模板默认值</option>
            <option v-for="p in proxyOptions" :key="p" :value="p">{{ p }}</option>
          </select>
        </label>
        <label class="field">
          <span>分类</span>
          <select v-model="targetCategory">
            <option value="">不指定（使用默认）</option>
            <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
          </select>
        </label>
      </div>

      <div class="preset-grid">
        <div v-for="cat in filteredCategories" :key="cat.id" class="preset-category">
          <div class="preset-category-header">
            <span class="preset-cat-icon">{{ cat.icon }}</span>
            <strong>{{ cat.name }}</strong>
          </div>
          <div class="preset-list">
            <div
              v-for="preset in cat.presets"
              :key="preset.id"
              class="preset-card"
              :class="{ selected: selectedIds.has(preset.id) }"
              @click="togglePreset(preset.id)"
            >
              <div class="preset-card-head">
                <label class="preset-check">
                  <input type="checkbox" :checked="selectedIds.has(preset.id)" @click.stop />
                  <strong>{{ preset.name }}</strong>
                </label>
                <span class="badge">{{ preset.rules.length }} 条</span>
              </div>
              <p class="preset-desc">{{ preset.description }}</p>
              <div class="preset-rules-preview">
                <code v-for="(rule, idx) in preset.rules.slice(0, 3)" :key="idx">
                  {{ rule.type }},{{ rule.value }},{{ proxyOverride || rule.proxy }}
                </code>
                <span v-if="preset.rules.length > 3" class="muted">...等 {{ preset.rules.length }} 条</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div v-if="selectedIds.size > 0" class="preset-footer">
        <span>已选 {{ selectedIds.size }} 个模板，共 {{ totalSelectedRules }} 条规则</span>
        <button class="primary" @click="applySelected">应用到当前规则列表</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { rulePresetCategories } from '../utils/rulePresets'

const props = defineProps({
  proxyOptions: { type: Array, default: () => ['DIRECT', 'PROXY', 'REJECT'] },
  categories: { type: Array, default: () => [] },
})

const emit = defineEmits(['apply', 'close'])

const search = ref('')
const proxyOverride = ref('')
const targetCategory = ref('')
const selectedIds = ref(new Set())

const filteredCategories = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return rulePresetCategories
  return rulePresetCategories
    .map((cat) => ({
      ...cat,
      presets: cat.presets.filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.description.toLowerCase().includes(q) ||
          p.rules.some((r) => r.value.toLowerCase().includes(q))
      ),
    }))
    .filter((cat) => cat.presets.length > 0)
})

const totalSelectedRules = computed(() => {
  let count = 0
  for (const cat of rulePresetCategories) {
    for (const preset of cat.presets) {
      if (selectedIds.value.has(preset.id)) count += preset.rules.length
    }
  }
  return count
})

function togglePreset(id) {
  const s = new Set(selectedIds.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  selectedIds.value = s
}

function applySelected() {
  const rules = []
  for (const cat of rulePresetCategories) {
    for (const preset of cat.presets) {
      if (!selectedIds.value.has(preset.id)) continue
      for (const rule of preset.rules) {
        rules.push({
          type: rule.type,
          value: rule.value,
          proxy: proxyOverride.value || rule.proxy,
          name: preset.name,
          category: targetCategory.value || 'default',
          enabled: true,
          options: [],
        })
      }
    }
  }
  emit('apply', rules)
  emit('close')
}
</script>

<style scoped>
.preset-modal {
  max-width: 860px;
  max-height: 85vh;
  overflow-y: auto;
}
.preset-controls {
  display: flex;
  gap: 10px;
  align-items: end;
  margin: 12px 0;
  flex-wrap: wrap;
}
.preset-search {
  flex: 1;
  min-width: 200px;
}
.preset-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 12px;
}
.preset-category-header {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}
.preset-cat-icon {
  font-size: 18px;
}
.preset-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 8px;
}
.preset-card {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 10px;
  cursor: pointer;
  transition: border-color 0.15s;
}
.preset-card:hover {
  border-color: var(--accent);
}
.preset-card.selected {
  border-color: var(--accent);
  background: var(--accent-bg, rgba(99,102,241,0.08));
}
.preset-card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.preset-check {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}
.preset-desc {
  font-size: 12px;
  color: var(--muted);
  margin: 4px 0;
}
.preset-rules-preview {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 11px;
}
.preset-rules-preview code {
  background: var(--bg-1, #f1f5f9);
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 11px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.preset-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--border);
  position: sticky;
  bottom: 0;
  background: var(--bg-0, #fff);
}
</style>

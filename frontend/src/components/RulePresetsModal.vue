<template>
  <AppModal
    :model-value="true"
    size="lg"
    title="规则模板"
    @update:model-value="(val) => !val && $emit('close')"
    @close="$emit('close')"
  >
    <div class="flex flex-col gap-4">
      <div class="text-xs text-text-muted">
        选择常用规则模板快速添加，proxy 目标可在应用前修改。
      </div>

      <!-- Controls Row -->
      <div class="flex flex-wrap items-end gap-3">
        <label class="flex flex-col gap-1 text-xs text-text-muted flex-1 min-w-[200px]">
          <span>搜索模板</span>
          <Input
            v-model="search"
            placeholder="搜索模板（如 openai、youtube）"
            class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main placeholder:text-text-muted focus:border-accent focus:outline-hidden"
          />
        </label>

        <label class="flex flex-col gap-1 text-xs text-text-muted w-[160px]">
          <span>目标代理</span>
          <Select
            v-model="proxyOverride"
            class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          >
            <option value="">使用模板默认值</option>
            <option v-for="p in proxyOptions" :key="p" :value="p">{{ p }}</option>
          </Select>
        </label>

        <label class="flex flex-col gap-1 text-xs text-text-muted w-[160px]">
          <span>分类</span>
          <Select
            v-model="targetCategory"
            class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          >
            <option value="">不指定（使用默认）</option>
            <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
          </Select>
        </label>
      </div>

      <!-- Preset Grid -->
      <div class="flex flex-col gap-4 pt-2">
        <div
          v-for="cat in filteredCategories"
          :key="cat.id"
          class="flex flex-col gap-2"
        >
          <div class="text-xs font-semibold text-text-main tracking-wide">
            {{ cat.name }}
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2.5">
            <div
              v-for="preset in cat.presets"
              :key="preset.id"
              :class="[
                'flex flex-col gap-2 rounded-lg border p-3 cursor-pointer transition-colors',
                selectedIds.has(preset.id)
                  ? 'border-accent bg-accent/10 shadow-xs'
                  : 'border-border-subtle bg-surface-base hover:border-accent/50 hover:bg-surface-hover'
              ]"
              @click="togglePreset(preset.id)"
            >
              <div class="flex items-center justify-between gap-2">
                <label class="flex items-center gap-2 cursor-pointer text-xs font-semibold text-text-main">
                  <Checkbox
                    :model-value="selectedIds.has(preset.id)"
                    class="accent-accent cursor-pointer"
                    @click.stop
                    @update:model-value="togglePreset(preset.id)"
                  />
                  <span>{{ preset.name }}</span>
                </label>
                <span class="rounded bg-surface-hover px-1.5 py-0.5 font-mono text-[10px] text-text-muted border border-border-subtle shrink-0">
                  {{ preset.rules.length }} 条
                </span>
              </div>

              <p class="text-[11px] text-text-muted line-clamp-2 m-0">
                {{ preset.description }}
              </p>

              <div class="flex flex-col gap-1 text-[11px] font-mono text-text-muted pt-1 border-t border-border-subtle/50">
                <code
                  v-for="(rule, idx) in preset.rules.slice(0, 3)"
                  :key="idx"
                  class="overflow-hidden text-ellipsis whitespace-nowrap rounded bg-surface-hover px-1.5 py-0.5 text-[10px] text-text-main"
                >
                  {{ rule.type }},{{ rule.value }},{{ proxyOverride || rule.proxy }}
                </code>
                <span v-if="preset.rules.length > 3" class="text-[10px] text-text-muted">
                  ...等 {{ preset.rules.length }} 条
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-between">
        <span class="text-xs text-text-muted">
          已选 {{ selectedIds.size }} 个模板，共 {{ totalSelectedRules }} 条规则
        </span>
        <div class="flex gap-2">
          <Button
            type="button"
            class="px-3 py-1.5 rounded-md border border-border-subtle text-xs text-text-muted hover:text-text-main cursor-pointer"
            @click="$emit('close')"
          >
            取消
          </Button>
          <Button
            type="button"
            class="px-4 py-1.5 rounded-md bg-accent text-xs font-medium text-white hover:bg-accent-hover disabled:opacity-50 transition-colors cursor-pointer"
            :disabled="selectedIds.size === 0"
            @click="applySelected"
          >
            应用到当前规则列表
          </Button>
        </div>
      </div>
    </template>
  </AppModal>
</template>

<script setup>
import { computed, ref } from 'vue'
import AppModal from './ui/AppModal.vue'
import { Button, Input, Select, Checkbox } from './ui'
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

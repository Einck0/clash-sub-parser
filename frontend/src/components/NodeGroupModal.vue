<template>
  <AppModal
    :model-value="true"
    size="lg"
    :title="group?.id ? '编辑节点组' : '新增节点组'"
    @update:model-value="(val) => !val && close()"
    @close="close"
  >
    <div class="flex flex-col gap-4">
      <div class="text-xs text-text-muted">
        正则是<strong>虚拟筛选</strong>：加入条目列表后会按最终节点名动态匹配，不会冻结成静态节点。
      </div>

      <div v-if="error" class="rounded-md border border-status-danger/40 bg-status-danger/10 p-3 text-xs text-status-danger">
        {{ error }}
      </div>
      <div v-if="sourcesLoading" class="text-xs text-text-muted">正在加载可选节点与策略组…</div>

      <!-- Basic Info (Name, Type) -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <label class="flex flex-col gap-1 text-xs text-text-muted">
          <span>名称</span>
          <Input
            v-model="form.name"
            placeholder="例如：自动选择"
            class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          />
        </label>
        <label class="flex flex-col gap-1 text-xs text-text-muted">
          <span>类型</span>
          <FormSelect
            v-model="form.group_type"
            class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          >
            <option value="select">select</option>
            <option value="url-test">url-test</option>
            <option value="fallback">fallback</option>
            <option value="load-balance">load-balance</option>
          </FormSelect>
        </label>
      </div>

      <!-- Fallback PASS section -->
      <div class="rounded-lg border border-border-subtle bg-surface-base p-3 flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <strong class="text-xs text-text-main">兜底节点</strong>
          <span class="text-[11px] text-text-muted">仅当策略组最终没有任何节点时才追加 PASS；有节点时不会加。</span>
        </div>
        <label class="flex items-center gap-2 cursor-pointer text-xs text-text-main pt-1">
          <Checkbox v-model="form.add_fallback" />
          <span>空组时追加 PASS <span class="text-text-muted text-[11px]">(默认关闭)</span></span>
        </label>
      </div>

      <!-- URL-Test / Fallback / Load-Balance config -->
      <div
        v-if="['url-test', 'fallback', 'load-balance'].includes(form.group_type)"
        class="rounded-lg border border-border-subtle bg-surface-base p-3 flex flex-col gap-3"
      >
        <div>
          <strong class="text-xs text-text-main">{{ form.group_type }} 参数</strong>
          <p class="text-[11px] text-text-muted m-0">导出到 Clash 时写入对应字段；空值用默认。</p>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <label class="flex flex-col gap-1 text-xs text-text-muted">
            <span>url</span>
            <Input
              v-model="urlTestUrl"
              placeholder="https://www.gstatic.com/generate_204"
              class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
            />
          </label>
          <label class="flex flex-col gap-1 text-xs text-text-muted">
            <span>interval (秒)</span>
            <Input
              v-model.number="urlTestInterval"
              type="number"
              min="1"
              placeholder="300"
              class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
            />
          </label>
          <label class="flex flex-col gap-1 text-xs text-text-muted">
            <span>tolerance (ms)</span>
            <Input
              v-model.number="urlTestTolerance"
              type="number"
              min="0"
              placeholder="50"
              class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
            />
          </label>
        </div>
      </div>

      <!-- Add Source Entries -->
      <div class="rounded-lg border border-border-subtle bg-surface-base p-3 flex flex-col gap-3">
        <div>
          <strong class="text-xs text-text-main">添加来源条目</strong>
          <p class="text-[11px] text-text-muted m-0">
            可添加：静态节点、节点组引用、节点组节点、正则筛选（虚拟）。
            正则只记录规则本身，输出时动态展开匹配到的节点。
          </p>
        </div>

        <!-- Static Node and Builtin row -->
        <div class="flex flex-wrap gap-2 items-center">
          <FormSelect
            v-model="selectedNodeName"
            class="flex-1 min-w-[180px] rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          >
            <option value="">选择节点</option>
            <option v-for="name in selectableNodeNames" :key="name" :value="name">{{ name }}</option>
          </FormSelect>
          <ShadButton
            type="button"
            class="px-3 py-2 rounded-md border border-border-subtle text-xs text-text-main hover:bg-surface-hover disabled:opacity-50 cursor-pointer"
            :disabled="!selectedNodeName"
            @click="addNode"
          >
            加入静态节点
          </ShadButton>
          <ShadButton
            type="button"
            class="px-2.5 py-2 rounded-md border border-border-subtle text-xs text-text-muted hover:text-text-main hover:bg-surface-hover cursor-pointer font-mono"
            @click="addBuiltin('DIRECT')"
          >
            DIRECT
          </ShadButton>
          <ShadButton
            type="button"
            class="px-2.5 py-2 rounded-md border border-border-subtle text-xs text-text-muted hover:text-text-main hover:bg-surface-hover cursor-pointer font-mono"
            @click="addBuiltin('PASS')"
          >
            PASS
          </ShadButton>
          <ShadButton
            type="button"
            class="px-2.5 py-2 rounded-md border border-border-subtle text-xs text-text-muted hover:text-text-main hover:bg-surface-hover cursor-pointer font-mono"
            @click="addBuiltin('REJECT')"
          >
            REJECT
          </ShadButton>
        </div>

        <!-- Node Group reference row -->
        <div class="flex flex-wrap gap-2 items-center">
          <FormSelect
            v-model.number="selectedGroupId"
            class="flex-1 min-w-[180px] rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
          >
            <option :value="null">选择节点组</option>
            <option v-for="g in selectableGroups" :key="g.id" :value="g.id">{{ g.name }}</option>
          </FormSelect>
          <div class="flex gap-1.5 flex-wrap">
            <ShadButton
              type="button"
              class="px-3 py-2 rounded-md border border-border-subtle text-xs text-text-main hover:bg-surface-hover disabled:opacity-50 cursor-pointer"
              :disabled="!selectedGroupId"
              @click="addGroupRef"
            >
              添加组引用
            </ShadButton>
            <ShadButton
              type="button"
              class="px-3 py-2 rounded-md border border-border-subtle text-xs text-text-main hover:bg-surface-hover disabled:opacity-50 cursor-pointer"
              :disabled="!selectedGroupId"
              @click="addGroupNodes"
            >
              添加组节点
            </ShadButton>
            <ShadButton
              type="button"
              class="px-3 py-2 rounded-md border border-border-subtle text-xs text-text-main hover:bg-surface-hover disabled:opacity-50 cursor-pointer"
              :disabled="!selectedGroupId"
              @click="addExcludeGroupNodes"
            >
              减去组节点
            </ShadButton>
          </div>
        </div>

        <!-- Regex filter draft row -->
        <div class="flex flex-col gap-2 pt-2 border-t border-border-subtle/50">
          <div class="text-xs text-text-muted">添加正则筛选（虚拟）</div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <Input
              v-model="regexDraftName"
              placeholder="名称（可选，空则自动 正则1/正则2）"
              class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
            />
            <Input
              v-model="regexDraft"
              placeholder="正则，例如：香港  或  ^(?!.*(官网|套餐)).*$"
              class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
              @keyup.enter="addRegexEntry"
            />
          </div>
          <div class="flex items-center gap-2 flex-wrap">
            <ShadButton
              type="button"
              class="px-3 py-1.5 rounded-md bg-accent text-xs font-medium text-white hover:bg-accent-hover disabled:opacity-50 cursor-pointer"
              :disabled="!regexDraft.trim() || !!regexDraftError"
              @click="addRegexEntry"
            >
              加入正则
            </ShadButton>
            <ShadButton
              type="button"
              class="px-3 py-1.5 rounded-md border border-border-subtle text-xs text-text-main hover:bg-surface-hover disabled:opacity-50 cursor-pointer"
              :disabled="!regexDraft.trim() || !!regexDraftError"
              @click="previewDraftRegex"
            >
              预览该正则匹配
            </ShadButton>
            <span class="text-xs text-text-muted">匹配 {{ draftMatches.length }}</span>
          </div>
          <div v-if="regexDraftError" class="text-xs text-status-danger">
            {{ regexDraftError }}
          </div>
          <div
            v-if="draftMatches.length"
            class="max-h-24 overflow-y-auto rounded bg-surface-hover p-2 font-mono text-[11px] text-text-muted leading-relaxed"
          >
            {{ draftMatches.slice(0, 60).join(' | ') }}
            <span v-if="draftMatches.length > 60" class="text-accent font-medium"> … +{{ draftMatches.length - 60 }}</span>
          </div>
        </div>
      </div>

      <!-- In-Modal Regex Preview Panel (Governed, accessible in-modal region) -->
      <div
        v-if="showRegexPreview"
        class="rounded-lg border border-accent/40 bg-surface-base p-4 shadow-sm"
        role="region"
        aria-label="正则预览"
      >
        <div class="flex items-center justify-between pb-2 border-b border-border-subtle">
          <div class="flex items-center gap-2">
            <strong class="text-xs font-semibold text-text-main">正则预览</strong>
            <span class="rounded bg-accent/15 px-2 py-0.5 text-[11px] font-medium text-accent">
              {{ previewMatches.length }} 个匹配
            </span>
          </div>
          <ShadButton
            type="button"
            class="text-xs text-text-muted hover:text-text-main cursor-pointer"
            @click="closeRegexPreview"
          >
            关闭预览
          </ShadButton>
        </div>
        <div
          v-if="previewMatches.length"
          class="mt-2 max-h-40 overflow-y-auto font-mono text-xs text-text-muted leading-relaxed"
        >
          {{ previewMatches.slice(0, 120).join(' | ') }}
          <span v-if="previewMatches.length > 120" class="text-accent font-medium"> … +{{ previewMatches.length - 120 }}</span>
        </div>
        <div v-else class="mt-2 text-xs text-text-muted">没有匹配节点</div>
      </div>

      <!-- Probe & Media Filtering -->
      <div class="rounded-lg border border-border-subtle bg-surface-base p-3 flex flex-col gap-3">
        <div>
          <strong class="inline-flex items-center gap-1.5 text-xs text-text-main">
            <Zap class="h-4 w-4 text-accent" aria-hidden="true" />
            <span>节点质检与流媒体或 AI 过滤（可选）</span>
          </strong>
          <p class="text-[11px] text-text-muted m-0">
            满足条件的节点才会进入此策略组。测速门槛和流媒体解锁要求需在节点探测中测得有效结果。
          </p>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <label class="flex flex-col gap-1 text-xs text-text-muted">
            <span>测速最低门槛 (Mbps)</span>
            <Input
              type="number"
              min="0"
              step="0.5"
              v-model.number="form.filter_min_speed_mbps"
              placeholder="例如 5.0，留空或 0 为不限制"
              class="rounded-md border border-border-subtle bg-surface-hover px-3 py-2 text-xs text-text-main focus:border-accent focus:outline-hidden"
            />
          </label>
        </div>

        <div class="flex flex-col gap-2">
          <div class="text-xs text-text-muted">必须解锁的流媒体 / AI 平台（多选）</div>
          <div class="flex flex-wrap gap-2">
          <label
            v-for="p in availablePlatforms"
            :key="p.id"
            :class="[
              'min-h-[44px] px-3 py-1.5 rounded-md border text-xs flex items-center gap-1.5 cursor-pointer select-none transition-colors',
                (form.filter_media_unlock || []).includes(p.id)
                  ? 'border-accent bg-accent/15 text-accent font-medium'
                  : 'border-border-subtle bg-surface-hover text-text-muted hover:text-text-main'
              ]"
            >
              <Checkbox
                :model-value="(form.filter_media_unlock || []).includes(p.id)"
                :aria-label="`要求解锁 ${p.name}`"
                @update:model-value="(checked) => togglePlatform(p.id, checked)"
              />
              <span>{{ p.name }}</span>
            </label>
          </div>
        </div>
      </div>

      <!-- Sorted Entry List -->
      <div class="rounded-lg border border-border-subtle bg-surface-base p-3 flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <div>
            <strong class="text-xs text-text-main">统一排序条目</strong>
            <p class="text-[11px] text-text-muted m-0">拖拽/上下调整顺序。正则项显示为虚拟筛选，不是冻结节点列表。</p>
          </div>
          <span class="text-xs text-text-muted">{{ form.include_entries.length }} 项</span>
        </div>

        <div v-if="!form.include_entries.length" class="text-xs text-text-muted py-3 text-center">
          还没有条目。可先加正则筛选或静态节点。
        </div>
        <div v-else class="flex flex-col gap-1.5 max-h-72 overflow-y-auto">
          <div
            v-for="(entry, idx) in form.include_entries"
            :key="`${entry.type}-${entry.value}-${idx}`"
            :class="[
              'flex items-center gap-2 rounded-md border p-2 text-xs transition-colors',
              entry.type === 'regex' ? 'border-accent/30 bg-accent/5' : 'border-border-subtle bg-surface-base',
              draggingIndex === idx ? 'opacity-50' : ''
            ]"
            @dragover.prevent
            @drop="onDrop(idx)"
          >
            <ShadButton
              v-if="!(entry.type === 'regex' && editingRegexIndex === idx)"
              type="button"
              class="drag-handle inline-flex items-center justify-center p-1 text-text-muted hover:text-text-main cursor-grab"
              title="拖拽排序"
              data-drag-handle
              draggable="true"
              @dragstart="onDragStart($event, idx)"
              @dragend="draggingIndex = -1"
              @click.stop
              @mousedown.stop
            >
              <GripVertical class="h-4 w-4" aria-hidden="true" />
            </ShadButton>

            <div class="flex-1 font-mono text-xs overflow-hidden">
              <template v-if="entry.type === 'regex' && editingRegexIndex === idx">
                <div class="flex flex-col gap-2 p-1">
                  <Input
                    v-model="editingRegexName"
                    class="rounded-md border border-border-subtle bg-surface-hover px-2.5 py-1.5 text-xs text-text-main focus:border-accent focus:outline-hidden"
                    placeholder="名称（可选）"
                  />
                  <Input
                    v-model="editingRegexValue"
                    class="rounded-md border border-border-subtle bg-surface-hover px-2.5 py-1.5 text-xs text-text-main focus:border-accent focus:outline-hidden font-mono"
                    placeholder="输入正则，例如 香港 或 ^(?!.*(官网|套餐)).*$"
                    @keyup.enter="saveRegexEdit(idx)"
                    @keyup.escape="cancelRegexEdit"
                  />
                  <div class="flex items-center gap-2 flex-wrap pt-1">
                    <ShadButton
                      type="button"
                      class="px-2.5 py-1 rounded bg-accent text-xs font-medium text-white hover:bg-accent-hover disabled:opacity-50 cursor-pointer"
                      :disabled="!!editingRegexError || !editingRegexValue.trim()"
                      @click="saveRegexEdit(idx)"
                    >
                      确定
                    </ShadButton>
                    <ShadButton
                      type="button"
                      class="px-2.5 py-1 rounded border border-border-subtle text-xs text-text-main hover:bg-surface-hover disabled:opacity-50 cursor-pointer"
                      :disabled="!!editingRegexError || !editingRegexValue.trim()"
                      @click="previewEditingRegex"
                    >
                      预览
                    </ShadButton>
                    <ShadButton
                      type="button"
                      class="px-2.5 py-1 rounded border border-border-subtle text-xs text-text-muted hover:text-text-main cursor-pointer"
                      @click="cancelRegexEdit"
                    >
                      取消
                    </ShadButton>
                    <span v-if="editingRegexError" class="text-xs text-status-danger">
                      {{ editingRegexError }}
                    </span>
                    <span v-else class="text-xs text-text-muted">动态匹配 {{ countRegexMatches(editingRegexValue) }} 个</span>
                  </div>
                </div>
              </template>
              <template v-else>
                <div class="flex items-center gap-1.5 overflow-hidden text-ellipsis whitespace-nowrap">
                  <strong class="text-text-main">{{ idx + 1 }}. {{ formatEntryTitle(entry, idx) }}</strong>
                  <span class="text-text-muted text-[11px]">
                    {{ typeLabel(entry.type) }}
                    <template v-if="entry.type === 'regex'">
                      · {{ truncateText(String(entry.value || ''), 48) }}
                      · 匹配 {{ countRegexMatches(entry.value) }} 个
                    </template>
                  </span>
                </div>
              </template>
            </div>

            <div v-if="!(entry.type === 'regex' && editingRegexIndex === idx)" class="flex items-center gap-1 shrink-0">
              <ShadButton
                v-if="entry.type === 'regex'"
                type="button"
                class="px-2 py-1 rounded border border-accent/40 text-xs text-accent hover:bg-accent/10 cursor-pointer"
                @click="startRegexEdit(idx)"
              >
                编辑
              </ShadButton>
              <ShadButton
                v-if="entry.type === 'regex'"
                type="button"
                class="px-2 py-1 rounded border border-border-subtle text-xs text-text-muted hover:text-text-main hover:bg-surface-hover cursor-pointer"
                @click="previewEntryRegex(entry.value)"
              >
                预览
              </ShadButton>
              <ShadButton
                type="button"
                class="px-2 py-1 rounded border border-border-subtle text-xs text-text-muted hover:text-text-main disabled:opacity-40 cursor-pointer"
                :disabled="idx === 0"
                @click="moveEntry(idx, -1)"
              >
                上
              </ShadButton>
              <ShadButton
                type="button"
                class="px-2 py-1 rounded border border-border-subtle text-xs text-text-muted hover:text-text-main disabled:opacity-40 cursor-pointer"
                :disabled="idx === form.include_entries.length - 1"
                @click="moveEntry(idx, 1)"
              >
                下
              </ShadButton>
              <ShadButton
                type="button"
                class="px-2 py-1 rounded border border-status-danger/40 text-xs text-status-danger hover:bg-status-danger/10 cursor-pointer"
                @click="removeEntry(idx)"
              >
                删
              </ShadButton>
            </div>
          </div>
        </div>
      </div>

      <!-- Raw JSON section -->
      <div v-if="showRaw" class="rounded-lg border border-border-subtle bg-surface-base p-3 flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <strong class="text-xs text-text-main">Raw JSON</strong>
          <ShadButton
            type="button"
            class="px-3 py-1 rounded-md border border-border-subtle text-xs text-text-main hover:bg-surface-hover cursor-pointer"
            @click="syncFromRaw"
          >
            应用 Raw
          </ShadButton>
        </div>
        <textarea
          v-model="rawJson"
          rows="8"
          class="w-full rounded-md border border-border-subtle bg-surface-hover p-2 font-mono text-xs text-text-main focus:border-accent focus:outline-hidden resize-y"
        ></textarea>
      </div>
    </div>

    <template #footer>
      <div class="flex items-center justify-between">
        <ShadButton
          type="button"
          class="px-3 py-1.5 rounded-md border border-border-subtle text-xs text-text-muted hover:text-text-main cursor-pointer"
          @click="showRaw = !showRaw"
        >
          {{ showRaw ? '隐藏 Raw' : '显示 Raw' }}
        </ShadButton>
        <div class="flex gap-2">
          <ShadButton
            type="button"
            class="px-3 py-1.5 rounded-md border border-border-subtle text-xs text-text-muted hover:text-text-main cursor-pointer"
            @click="close"
          >
            取消
          </ShadButton>
          <ShadButton
            type="button"
            class="px-4 py-1.5 rounded-md bg-accent text-xs font-medium text-white hover:bg-accent-hover disabled:opacity-50 transition-colors cursor-pointer"
            :disabled="saving || !form.name.trim()"
            @click="save"
          >
            {{ saving ? '保存中...' : '保存' }}
          </ShadButton>
        </div>
      </div>
    </template>
  </AppModal>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { Button as ShadButton, Input, Select as FormSelect, Checkbox } from './ui'
import { Zap, GripVertical } from 'lucide-vue-next'
import AppModal from './ui/AppModal.vue'
import {
  createNodeGroup,
  getAllSubscriptionNodes,
  getApiErrorMessage,
  getNodeGroups,
  updateNodeGroup,
} from '../api'
import { useAppStore } from '../stores/app'
import { setDragGhost, shouldAllowDragStart } from '../utils/drag'

const props = defineProps({ group: { type: Object, default: null } })
const emit = defineEmits(['saved', 'close'])
const store = useAppStore()

const availablePlatforms = [
  { id: 'youtube', name: 'YouTube' },
  { id: 'netflix', name: 'Netflix' },
  { id: 'disney', name: 'Disney+' },
  { id: 'chatgpt', name: 'ChatGPT' },
  { id: 'gemini', name: 'Gemini' },
  { id: 'meta_ai', name: 'Meta AI' },
  { id: 'bilibili', name: 'Bilibili' },
]

const allGroups = ref([])
const allNodes = ref([])
const selectedGroupId = ref(null)
const selectedNodeName = ref('')
const showRaw = ref(false)
const rawJson = ref('')
const regexDraft = ref('')
const regexDraftName = ref('')
const regexDraftError = ref('')
const draftMatches = ref([])
const previewMatches = ref([])
const showRegexPreview = ref(false)
const editingRegexIndex = ref(-1)
const editingRegexValue = ref('')
const editingRegexName = ref('')
const editingRegexError = ref('')
const draggingIndex = ref(-1)
const saving = ref(false)
const error = ref('')
const sourcesLoading = ref(false)
const form = ref(defaultForm())

const urlTestUrl = computed({
  get: () => form.value.url_test_config?.url || '',
  set: (v) => {
    form.value.url_test_config = { ...(form.value.url_test_config || {}), url: v }
  },
})
const urlTestInterval = computed({
  get: () => form.value.url_test_config?.interval ?? '',
  set: (v) => {
    form.value.url_test_config = { ...(form.value.url_test_config || {}), interval: v }
  },
})
const urlTestTolerance = computed({
  get: () => form.value.url_test_config?.tolerance ?? '',
  set: (v) => {
    form.value.url_test_config = { ...(form.value.url_test_config || {}), tolerance: v }
  },
})

watch(
  () => props.group,
  async (value) => {
    error.value = ''
    draftMatches.value = []
    previewMatches.value = []
    regexDraft.value = ''
    regexDraftName.value = ''
    regexDraftError.value = ''
    cancelRegexEdit()
    await loadSources()
    if (value) {
      const entries = normalizeEntries(value.include_entries || buildEntriesFallback(value))
      form.value = {
        id: value.id,
        name: value.name || '',
        kind: value.kind || 'manual',
        group_type: value.group_type || 'select',
        sort_order: value.sort_order || 0,
        include_entries: entries,
        add_fallback: value.add_fallback === true,
        exclude_nodes: [...(value.exclude_nodes || [])],
        filter_min_speed_mbps: value.filter_min_speed_mbps ?? null,
        filter_media_unlock: [...(value.filter_media_unlock || [])],
        url_test_config: value.url_test_config || {},
        load_balance_config: value.load_balance_config || {},
        fallback_config: value.fallback_config || {},
      }
    } else {
      form.value = defaultForm()
    }
    rawJson.value = JSON.stringify(form.value, null, 2)
  },
  { immediate: true }
)

watch(
  form,
  (value) => {
    if (showRaw.value) rawJson.value = JSON.stringify(value, null, 2)
  },
  { deep: true }
)

watch(regexDraft, () => {
  regexDraftError.value = validateRegex(regexDraft.value.trim())
  draftMatches.value = []
})

watch(editingRegexValue, () => {
  editingRegexError.value = validateRegex(editingRegexValue.value.trim())
})

const selectableGroups = computed(() => allGroups.value.filter((item) => item.id !== form.value.id))
const selectableNodeNames = computed(() =>
  allNodes.value.map((node) => String(node.name || '').trim()).filter(Boolean),
)

function togglePlatform(platformId, checked) {
  const selected = new Set(form.value.filter_media_unlock || [])
  if (checked === true) selected.add(platformId)
  else selected.delete(platformId)
  form.value.filter_media_unlock = [...selected]
}

function validateRegex(rule) {
  if (!rule) return ''
  try {
    new RegExp(rule)
    return ''
  } catch (err) {
    return `正则无效：${err.message}`
  }
}

function collectRegexMatches(rule) {
  const text = String(rule || '').trim()
  if (!text) return []
  let pattern
  try {
    pattern = new RegExp(text, 'i')
  } catch (_) {
    return []
  }
  const matches = []
  for (const node of allNodes.value) {
    const name = String(node.name || '')
    if (name && pattern.test(name)) matches.push(name)
  }
  return uniq(matches)
}

function countRegexMatches(rule) {
  return collectRegexMatches(rule).length
}

function openRegexPreview(matches) {
  previewMatches.value = matches || []
  showRegexPreview.value = true
}

function closeRegexPreview() {
  showRegexPreview.value = false
}

function previewDraftRegex() {
  regexDraftError.value = validateRegex(regexDraft.value.trim())
  if (regexDraftError.value) return
  draftMatches.value = collectRegexMatches(regexDraft.value.trim())
  openRegexPreview(draftMatches.value)
}

function previewEntryRegex(rule) {
  openRegexPreview(collectRegexMatches(rule))
}

function startRegexEdit(index) {
  const entry = form.value.include_entries[index]
  if (!entry || entry.type !== 'regex') return
  editingRegexIndex.value = index
  editingRegexValue.value = String(entry.value || '')
  editingRegexName.value = String(entry.name || entry.label || '')
  editingRegexError.value = validateRegex(editingRegexValue.value.trim())
}

function cancelRegexEdit() {
  editingRegexIndex.value = -1
  editingRegexValue.value = ''
  editingRegexName.value = ''
  editingRegexError.value = ''
}

function previewEditingRegex() {
  editingRegexError.value = validateRegex(editingRegexValue.value.trim())
  if (editingRegexError.value) return
  openRegexPreview(collectRegexMatches(editingRegexValue.value.trim()))
}

function saveRegexEdit(index) {
  const next = editingRegexValue.value.trim()
  editingRegexError.value = validateRegex(next)
  if (!next || editingRegexError.value) return

  const exists = form.value.include_entries.some(
    (item, i) => i !== index && item.type === 'regex' && String(item.value) === next
  )
  if (exists) {
    editingRegexError.value = '已存在相同正则条目'
    return
  }

  const copy = [...form.value.include_entries]
  const label = String(editingRegexName.value || '').trim()
  const item = { type: 'regex', value: next }
  if (label) item.name = label
  copy[index] = item
  form.value.include_entries = copy
  cancelRegexEdit()
}

function addRegexEntry() {
  const rule = regexDraft.value.trim()
  regexDraftError.value = validateRegex(rule)
  if (!rule || regexDraftError.value) return
  const label = String(regexDraftName.value || '').trim() || nextRegexLabel()
  pushEntry({ type: 'regex', value: rule, name: label })
  regexDraft.value = ''
  regexDraftName.value = ''
  draftMatches.value = []
}

function nextRegexLabel() {
  const used = new Set(
    (form.value.include_entries || [])
      .filter((item) => item.type === 'regex')
      .map((item) => String(item.name || '').trim())
      .filter(Boolean),
  )
  let i = 1
  while (used.has(`正则${i}`)) i += 1
  return `正则${i}`
}

function truncateText(text, max = 40) {
  const value = String(text || '')
  if (value.length <= max) return value
  return `${value.slice(0, Math.max(1, max - 1))}…`
}

function formatEntryTitle(entry, idx = 0) {
  if (entry.type === 'regex') {
    return String(entry.name || entry.label || '').trim() || `正则${regexIndex(entry, idx)}`
  }
  return formatEntry(entry)
}

function regexIndex(entry, fallbackIdx = 0) {
  let n = 0
  for (const item of form.value.include_entries || []) {
    if (item.type !== 'regex') continue
    n += 1
    if (item === entry) return n
  }
  return fallbackIdx + 1
}

function addNode() {
  if (!selectedNodeName.value) return
  pushEntry({ type: 'node', value: selectedNodeName.value })
}

function addBuiltin(name) {
  pushEntry({ type: 'node', value: name })
}

function addGroupRef() {
  if (!selectedGroupId.value) return
  pushEntry({ type: 'group', value: Number(selectedGroupId.value) })
}

function addGroupNodes() {
  if (!selectedGroupId.value) return
  pushEntry({ type: 'group_nodes', value: Number(selectedGroupId.value) })
}

function addExcludeGroupNodes() {
  if (!selectedGroupId.value) return
  const gid = Number(selectedGroupId.value)
  if (gid === form.value.id) {
    error.value = '不能减去自己'
    return
  }
  const entry = { type: 'exclude_group_nodes', value: gid }
  const exists = form.value.include_entries.some(
    (item) => item.type === entry.type && String(item.value) === String(entry.value)
  )
  if (!exists) {
    form.value.include_entries.push(entry)
  }
}

function pushEntry(entry) {
  const exists = form.value.include_entries.some(
    (item) => item.type === entry.type && String(item.value) === String(entry.value)
  )
  if (exists) return
  form.value.include_entries.push(entry)
}

function moveEntry(index, direction) {
  const to = index + direction
  if (to < 0 || to >= form.value.include_entries.length) return
  const copy = [...form.value.include_entries]
  ;[copy[index], copy[to]] = [copy[to], copy[index]]
  form.value.include_entries = copy
}

function removeEntry(index) {
  form.value.include_entries.splice(index, 1)
}

function onDragStart(event, index) {
  if (editingRegexIndex.value === index) {
    event.preventDefault()
    return
  }
  if (!shouldAllowDragStart(event, { requireHandle: true })) {
    event.preventDefault()
    draggingIndex.value = -1
    return
  }
  draggingIndex.value = index
  setDragGhost(event, `条目 #${index + 1}`)
}

function onDrop(targetIndex) {
  if (draggingIndex.value < 0 || draggingIndex.value === targetIndex) return
  if (editingRegexIndex.value >= 0) {
    draggingIndex.value = -1
    return
  }
  const copy = [...form.value.include_entries]
  const [moved] = copy.splice(draggingIndex.value, 1)
  copy.splice(targetIndex, 0, moved)
  form.value.include_entries = copy
  draggingIndex.value = -1
}

function typeLabel(type) {
  if (type === 'node') return '静态节点'
  if (type === 'group') return '节点组引用'
  if (type === 'group_nodes') return '节点组节点'
  if (type === 'exclude_group_nodes') return '减去组节点(动态)'
  if (type === 'regex') return '正则筛选(虚拟)'
  return type
}

function formatEntry(entry) {
  if (entry.type === 'node') return `${entry.value}`
  if (entry.type === 'group') return `${groupNameById(Number(entry.value))}`
  if (entry.type === 'group_nodes') return `${groupNameById(Number(entry.value))}(节点)`
  if (entry.type === 'exclude_group_nodes') return `减${groupNameById(Number(entry.value))}节点`
  if (entry.type === 'regex') {
    const label = String(entry.name || entry.label || '').trim() || '正则'
    return `${label} · ${truncateText(String(entry.value || ''), 48)}`
  }
  return JSON.stringify(entry)
}

function groupNameById(id) {
  return allGroups.value.find((item) => item.id === id)?.name || `#${id}`
}

function syncFromRaw() {
  try {
    const parsed = JSON.parse(rawJson.value)
    form.value = {
      ...defaultForm(),
      ...parsed,
      include_entries: normalizeEntries(parsed.include_entries || buildEntriesFallback(parsed)),
      exclude_nodes: uniq(parsed.exclude_nodes || []),
    }
  } catch (err) {
    error.value = `Raw JSON 格式错误: ${err.message}`
  }
}

function detectReferenceCycle(entries) {
  const selfId = form.value.id == null ? null : Number(form.value.id)
  const graph = new Map()
  for (const group of allGroups.value) {
    const edges = []
    const seen = new Set()
    const push = (raw) => {
      const id = Number(raw)
      if (!Number.isInteger(id) || seen.has(id)) return
      seen.add(id)
      edges.push(id)
    }
    const sourceEntries =
      selfId != null && group.id === selfId
        ? entries
        : normalizeEntries(group.include_entries || buildEntriesFallback(group))
    for (const entry of sourceEntries) {
      if (entry.type === 'group' || entry.type === 'group_nodes' || entry.type === 'exclude_group_nodes') push(entry.value)
    }
    graph.set(group.id, edges)
  }
  if (selfId == null) {
    const edges = []
    const seen = new Set()
    const push = (raw) => {
      const id = Number(raw)
      if (!Number.isInteger(id) || seen.has(id)) return
      seen.add(id)
      edges.push(id)
    }
    for (const entry of entries) {
      if (
        entry.type === 'group'
        || entry.type === 'group_nodes'
        || entry.type === 'exclude_group_nodes'
      ) {
        push(entry.value)
      }
    }
    graph.set(0, edges)
  }

  const visiting = new Set()
  const visited = new Set()
  const path = []
  const label = (id) => {
    if (id === 0) return form.value.name || '当前策略组'
    return groupNameById(id)
  }
  const dfs = (node) => {
    if (visited.has(node)) return null
    if (visiting.has(node)) {
      const start = path.indexOf(node)
      const cycle = path.slice(start >= 0 ? start : 0).concat(node)
      return `策略组引用存在循环：${cycle.map(label).join(' → ')}。加/减策略组引用都不能形成环。`
    }
    visiting.add(node)
    path.push(node)
    for (const child of graph.get(node) || []) {
      const hit = dfs(child)
      if (hit) return hit
    }
    path.pop()
    visiting.delete(node)
    visited.add(node)
    return null
  }
  for (const node of graph.keys()) {
    const hit = dfs(node)
    if (hit) return hit
  }
  return ''
}

async function save() {
  if (saving.value) return
  const name = String(form.value.name || '').trim()
  if (!name) {
    error.value = '名称不能为空'
    return
  }

  const entries = normalizeEntries(form.value.include_entries || [])
  for (const [idx, entry] of entries.entries()) {
    if (entry.type !== 'regex') continue
    const err = validateRegex(String(entry.value || ''))
    if (err) {
      error.value = `第 ${idx + 1} 条正则无效：${err}`
      return
    }
  }

  const cycleHint = detectReferenceCycle(entries)
  if (cycleHint) {
    error.value = cycleHint
    store.error(cycleHint)
    return
  }

  const urlCfg = { ...(form.value.url_test_config || {}) }
  if (urlTestUrl.value) urlCfg.url = String(urlTestUrl.value).trim()
  if (urlTestInterval.value !== '' && urlTestInterval.value != null) {
    urlCfg.interval = Number(urlTestInterval.value) || 300
  }
  if (urlTestTolerance.value !== '' && urlTestTolerance.value != null) {
    urlCfg.tolerance = Number(urlTestTolerance.value) || 0
  }
  const payload = {
    name,
    group_type: form.value.group_type || 'select',
    sort_order: form.value.sort_order || 0,
    include_entries: entries,
    add_fallback: form.value.add_fallback === true,
    exclude_nodes: uniq(form.value.exclude_nodes || []),
    filter_min_speed_mbps: form.value.filter_min_speed_mbps || null,
    filter_media_unlock: form.value.filter_media_unlock || [],
    url_test_config: urlCfg,
    load_balance_config: form.value.load_balance_config || {},
    fallback_config: form.value.fallback_config || {},
  }

  saving.value = true
  error.value = ''
  try {
    if (form.value.id) {
      await updateNodeGroup(form.value.id, payload)
    } else {
      await createNodeGroup(payload)
    }
    emit('saved', { created: !form.value.id, name })
    emit('close')
  } catch (err) {
    error.value = getApiErrorMessage(err, '保存节点组失败')
    store.error(error.value)
  } finally {
    saving.value = false
  }
}

async function loadSources() {
  sourcesLoading.value = true
  try {
    const [groupsRes, nodesRes] = await Promise.all([getNodeGroups(), getAllSubscriptionNodes()])
    allGroups.value = groupsRes.data
    allNodes.value = nodesRes.data
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载可选节点与策略组失败，请关闭后重试')
  } finally {
    sourcesLoading.value = false
  }
}

function close() {
  emit('close')
}

function defaultForm() {
  return {
    id: null,
    name: '',
    kind: 'manual',
    group_type: 'select',
    sort_order: 0,
    include_entries: [],
    add_fallback: false,
    exclude_nodes: [],
    filter_min_speed_mbps: null,
    filter_media_unlock: [],
    url_test_config: {},
    load_balance_config: {},
    fallback_config: {},
  }
}

function normalizeEntries(entries) {
  const allowed = new Set(['node', 'group', 'group_nodes', 'exclude_group_nodes', 'regex'])
  const out = []
  for (const item of entries) {
    const type = String(item?.type || '').trim()
    if (!allowed.has(type)) continue
    if (type === 'node') {
      const value = String(item?.value || '').trim()
      if (!value) continue
      out.push({ type, value })
      continue
    }
    if (type === 'regex') {
      const value = String(item?.value || '').trim()
      if (!value) continue
      const name = String(item?.name || item?.label || '').trim()
      const row = { type, value }
      if (name) row.name = name
      out.push(row)
      continue
    }
    const value = Number(item?.value)
    if (!Number.isInteger(value)) continue
    out.push({ type, value })
  }
  return uniqBy(out, (item) => `${item.type}:${item.value}`)
}

function buildEntriesFallback(value) {
  const entries = []
  for (const name of value.include_nodes || []) entries.push({ type: 'node', value: name })
  for (const id of value.include_group_ids || []) entries.push({ type: 'group', value: id })
  for (const id of value.include_group_nodes_ids || []) entries.push({ type: 'group_nodes', value: id })
  for (const id of value.exclude_group_ids || []) entries.push({ type: 'exclude_group_nodes', value: id })
  return entries
}

function uniq(items) {
  return [...new Set(items)]
}

function uniqBy(items, getKey) {
  const seen = new Set()
  const out = []
  for (const item of items) {
    const key = getKey(item)
    if (seen.has(key)) continue
    seen.add(key)
    out.push(item)
  }
  return out
}
</script>

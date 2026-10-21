<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ApiError, api } from '../../api/client'
import { t } from '../../locales'

interface DnsSettings {
  enabled: boolean
  raw_yaml: string
  revision: number
  updated_at: string
  source: 'default' | 'saved'
  config: Record<string, unknown>
}
interface ValidatedDns {
  raw_yaml: string
  config: Record<string, unknown>
}

const settings = ref<DnsSettings | null>(null)
const enabled = ref(false)
const raw = ref('')
const draft = ref<Record<string, unknown>>({})
const mode = ref<'visual' | 'raw'>('visual')
const loading = ref(false)
const saving = ref(false)
const validating = ref(false)
const error = ref('')
const notice = ref('')

const fieldErrors = ref<Record<string, string>>({})
const invalidJsonInputs = ref<Record<string, string>>({})

const hasValidationErrors = computed(() => Object.keys(fieldErrors.value).length > 0)

const visualDirty = computed(() => {
  if (!settings.value) return false
  if (hasValidationErrors.value) return true
  try {
    return JSON.stringify(draft.value) !== JSON.stringify(settings.value.config || {})
  } catch {
    return true
  }
})

const dirty = computed(() => {
  if (!settings.value) return false
  if (enabled.value !== settings.value.enabled) return true
  if (mode.value === 'visual') {
    return visualDirty.value
  }
  return raw.value !== settings.value.raw_yaml
})

function apply(settingsValue: DnsSettings) {
  settings.value = settingsValue
  enabled.value = settingsValue.enabled
  raw.value = settingsValue.raw_yaml
  draft.value = structuredClone(settingsValue.config || {})
  fieldErrors.value = {}
  invalidJsonInputs.value = {}
}

function getListText(key: string): string {
  const val = draft.value[key]
  if (Array.isArray(val)) {
    return val.join('\n')
  }
  return ''
}

function setListText(key: string, text: string) {
  const trimmed = text.trim()
  if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
    try {
      const parsed = JSON.parse(trimmed)
      if (Array.isArray(parsed)) {
        draft.value = { ...draft.value, [key]: parsed.map(String) }
        return
      }
    } catch {
      // Fall through to newline split
    }
  }
  const lines = text
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
  draft.value = { ...draft.value, [key]: lines }
}

function getJsonText(key: string): string {
  if (invalidJsonInputs.value[key] !== undefined) {
    return invalidJsonInputs.value[key]
  }
  const val = draft.value[key]
  return JSON.stringify(val ?? {}, null, 2)
}

function setJsonText(key: string, rawVal: string) {
  try {
    const parsed = JSON.parse(rawVal)
    draft.value = { ...draft.value, [key]: parsed }
    delete invalidJsonInputs.value[key]
    delete fieldErrors.value[key]
    if (Object.keys(fieldErrors.value).length === 0 && error.value.includes('必须是有效 JSON')) {
      error.value = ''
    }
  } catch {
    invalidJsonInputs.value[key] = rawVal
    fieldErrors.value[key] = `${key} 必须是有效 JSON`
    error.value = `${key} 必须是有效 JSON`
  }
}

async function load(force = false) {
  if (dirty.value && !force && !window.confirm('放弃未保存的 DNS 草稿并重新加载？')) return
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    apply(await api.get<DnsSettings>('/api/v1/settings/dns'))
  } catch (e) {
    error.value = e instanceof Error ? e.message : '读取 DNS 设置失败'
  } finally {
    loading.value = false
  }
}

async function validate(input = raw.value): Promise<ValidatedDns> {
  validating.value = true
  try {
    return await api.post<ValidatedDns>('/api/v1/settings/dns/validate', {
      enabled: enabled.value,
      raw_yaml: input,
    })
  } finally {
    validating.value = false
  }
}

async function switchMode(next: 'visual' | 'raw') {
  if (next === mode.value) return
  if (mode.value === 'visual' && hasValidationErrors.value) {
    error.value = '请先修正字段格式错误后再切换模式'
    return
  }
  error.value = ''
  try {
    if (next === 'visual') {
      const parsed = await validate(raw.value)
      draft.value = structuredClone(parsed.config)
      raw.value = parsed.raw_yaml
      fieldErrors.value = {}
      invalidJsonInputs.value = {}
    } else {
      const parsed = await validate(JSON.stringify(draft.value, null, 2))
      raw.value = parsed.raw_yaml
      draft.value = structuredClone(parsed.config)
      fieldErrors.value = {}
      invalidJsonInputs.value = {}
    }
    mode.value = next
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'DNS 配置校验失败'
  }
}

async function save() {
  if (!settings.value || saving.value) return
  if (mode.value === 'visual' && hasValidationErrors.value) {
    error.value = '请先修正字段格式错误后再保存'
    return
  }
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const rawInput = mode.value === 'raw' ? raw.value : JSON.stringify(draft.value, null, 2)
    const validated = await validate(rawInput)
    await api.put<DnsSettings>('/api/v1/settings/dns', {
      enabled: enabled.value,
      raw_yaml: validated.raw_yaml,
      expected_revision: settings.value.revision,
    })
    const confirmed = await api.get<DnsSettings>('/api/v1/settings/dns')
    apply(confirmed)
    notice.value = 'DNS 设置已保存并从服务器重新读取。'
  } catch (e: any) {
    if (e?.status === 409 || (e instanceof ApiError && e.status === 409) || (e?.name === 'ApiError' && e?.status === 409)) {
      error.value = 'DNS 设置已被其他操作更新（revision 冲突）；草稿已保留，请重新加载或复制草稿后再处理。'
    } else if (e?.status === 422 || (e instanceof ApiError && e.status === 422) || (e?.name === 'ApiError' && e?.status === 422)) {
      error.value = e?.message || 'DNS 配置校验失败（422 参数错误），草稿已保留。'
    } else {
      error.value = e instanceof Error ? e.message : '保存 DNS 设置失败'
    }
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void load(true)
})
</script>

<template>
  <section class="space-y-5" data-testid="dns-view" aria-label="客户端 DNS 配置">
    <div v-if="loading" role="status" class="alert">正在读取 DNS 设置…</div>
    <div v-if="error" role="alert" data-testid="dns-error" class="alert alert-error">
      <span>{{ error }}</span>
      <button class="btn btn-sm" @click="load()">重试 / 重载</button>
    </div>
    <div v-if="notice" role="status" class="alert alert-success">{{ notice }}</div>
    <template v-if="settings">
      <section class="card bg-base-200/80 border border-base-300">
        <div class="card-body p-4 sm:p-6 space-y-4">
          <label class="flex items-center justify-between gap-4">
            <span class="font-semibold">启用客户端 DNS</span>
            <input v-model="enabled" data-testid="dns-enabled" type="checkbox" class="toggle toggle-primary" />
          </label>
          <div class="flex gap-2" role="tablist" aria-label="DNS 编辑模式">
            <button
              class="btn btn-sm"
              :class="mode === 'visual' ? 'btn-primary' : 'btn-ghost'"
              data-testid="dns-visual-tab"
              @click="switchMode('visual')"
            >
              可视配置
            </button>
            <button
              class="btn btn-sm"
              :class="mode === 'raw' ? 'btn-primary' : 'btn-ghost'"
              data-testid="dns-raw-tab"
              @click="switchMode('raw')"
            >
              Raw YAML
            </button>
          </div>

          <template v-if="mode === 'visual'">
            <p class="text-xs opacity-70">支持可视化常用字段编辑与多行列表，保留未识别的高级键；复杂策略支持 JSON 编辑。</p>

            <!-- Row 1: Common toggles and scalar inputs -->
            <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-3">
              <label class="form-control flex flex-col justify-between p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                <span class="label-text font-mono text-xs font-semibold">ipv6</span>
                <div class="mt-2 flex items-center justify-between">
                  <span class="text-xs opacity-60">解析 IPv6</span>
                  <input
                    type="checkbox"
                    class="toggle toggle-primary toggle-sm"
                    aria-label="ipv6"
                    :checked="Boolean(draft['ipv6'])"
                    @change="e => { draft['ipv6'] = (e.target as HTMLInputElement).checked }"
                  />
                </div>
              </label>

              <label class="form-control p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                <span class="label-text font-mono text-xs font-semibold">enhanced-mode</span>
                <select
                  class="select select-bordered select-sm mt-1 font-mono text-xs w-full"
                  aria-label="enhanced-mode"
                  :value="String(draft['enhanced-mode'] ?? 'fake-ip')"
                  @change="e => { draft['enhanced-mode'] = (e.target as HTMLSelectElement).value }"
                >
                  <option value="fake-ip">fake-ip</option>
                  <option value="redir-host">redir-host</option>
                </select>
              </label>

              <label class="form-control p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                <span class="label-text font-mono text-xs font-semibold">listen</span>
                <input
                  type="text"
                  class="input input-bordered input-sm mt-1 font-mono text-xs w-full"
                  aria-label="listen"
                  :value="String(draft['listen'] ?? '')"
                  placeholder="0.0.0.0:1053"
                  @input="e => { draft['listen'] = (e.target as HTMLInputElement).value }"
                />
              </label>

              <label class="form-control p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                <span class="label-text font-mono text-xs font-semibold">fake-ip-range</span>
                <input
                  type="text"
                  class="input input-bordered input-sm mt-1 font-mono text-xs w-full"
                  aria-label="fake-ip-range"
                  :value="String(draft['fake-ip-range'] ?? '')"
                  placeholder="198.18.0.1/16"
                  @input="e => { draft['fake-ip-range'] = (e.target as HTMLInputElement).value }"
                />
              </label>
            </div>

            <!-- Multiline lists: nameserver, default-nameserver, fallback, proxy-server-nameserver, direct-nameserver, fake-ip-filter -->
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              <label
                v-for="key in ['nameserver', 'default-nameserver', 'fallback', 'proxy-server-nameserver', 'direct-nameserver', 'fake-ip-filter']"
                :key="key"
                class="form-control"
              >
                <div class="flex items-center justify-between">
                  <span class="label-text font-mono text-xs font-semibold">{{ key }}</span>
                  <span class="text-[10px] opacity-60">每行一条</span>
                </div>
                <textarea
                  class="textarea textarea-bordered min-h-24 font-mono text-xs mt-1"
                  :aria-label="key"
                  :value="getListText(key)"
                  placeholder="每行一个条目"
                  @input="e => setListText(key, (e.target as HTMLTextAreaElement).value)"
                  @change="e => setListText(key, (e.target as HTMLTextAreaElement).value)"
                />
              </label>
            </div>

            <!-- Advanced JSON Fields: nameserver-policy, fallback-filter -->
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <label
                v-for="key in ['nameserver-policy', 'fallback-filter']"
                :key="key"
                class="form-control"
              >
                <div class="flex items-center justify-between">
                  <span class="label-text font-mono text-xs font-semibold">{{ key }} (JSON)</span>
                  <span v-if="fieldErrors[key]" class="text-error text-xs font-semibold">{{ fieldErrors[key] }}</span>
                </div>
                <textarea
                  class="textarea textarea-bordered min-h-28 font-mono text-xs mt-1"
                  :class="{ 'textarea-error': fieldErrors[key] }"
                  :aria-label="key"
                  :value="getJsonText(key)"
                  @input="e => setJsonText(key, (e.target as HTMLTextAreaElement).value)"
                  @change="e => setJsonText(key, (e.target as HTMLTextAreaElement).value)"
                />
              </label>
            </div>
          </template>

          <label v-else class="form-control">
            <span class="label-text">完整 YAML 草稿</span>
            <textarea
              v-model="raw"
              data-testid="dns-raw-editor"
              class="textarea textarea-bordered min-h-96 font-mono text-xs"
              spellcheck="false"
            />
          </label>

          <div class="flex flex-wrap items-center justify-between gap-3">
            <span class="text-xs opacity-60">revision {{ settings.revision }} · {{ settings.updated_at || '—' }}</span>
            <button
              class="btn btn-primary"
              data-testid="dns-save"
              :disabled="saving || validating || !dirty || hasValidationErrors"
              @click="save"
            >
              {{ saving ? '校验并保存…' : '校验并保存' }}
            </button>
          </div>
        </div>
      </section>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import PageToolbar from '../../ui/PageToolbar.vue'
import { uiText } from '../../locales/redesign'
import { useDnsDraft } from './useDnsDraft'
import { DNS_LIST_KEYS, DNS_JSON_KEYS } from './dnsTypes'

const {
  settings,
  enabled,
  raw,
  draft,
  mode,
  loading,
  saving,
  validating,
  busy,
  error,
  notice,
  fieldErrors,
  hasValidationErrors,
  dirty,
  getListText,
  setListText,
  getJsonText,
  setJsonText,
  getString,
  setString,
  getBool,
  setBool,
  onRawInput,
  load,
  switchMode,
  save,
} = useDnsDraft()

const knownKeySet = new Set([
  'enable',
  'ipv6',
  'enhanced-mode',
  'listen',
  'fake-ip-range',
  'cache-algorithm',
  'use-hosts',
  'use-system-hosts',
  'respect-rules',
  'nameserver',
  'default-nameserver',
  'fallback',
  'proxy-server-nameserver',
  'direct-nameserver',
  'fake-ip-filter',
  'nameserver-policy',
  'fallback-filter',
])

const unknownKeys = computed(() => {
  return Object.keys(draft.value).filter((k) => !knownKeySet.has(k))
})

onMounted(() => {
  void load(true)
})
</script>

<template>
  <section class="space-y-4" data-testid="dns-view" :aria-label="uiText('客户端 DNS 配置', 'Client DNS Settings')">
    <!-- Non-redundant Toolbar: Accessible SR-only title, quick mode tabs, actions -->
    <PageToolbar :title="uiText('客户端 DNS 配置', 'Client DNS Settings')" :sr-only-title="true">
      <div class="flex flex-wrap items-center gap-2">
        <label class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-base-200/80 border border-base-300 cursor-pointer select-none">
          <input
            v-model="enabled"
            data-testid="dns-enabled"
            type="checkbox"
            class="toggle toggle-primary toggle-sm"
          />
          <span class="text-xs font-semibold">{{ uiText('启用客户端 DNS', 'Enable Client DNS') }}</span>
        </label>

        <div class="join" role="tablist" :aria-label="uiText('DNS 编辑模式', 'DNS Edit Mode')">
          <button
            class="btn btn-sm join-item"
            :class="mode === 'visual' ? 'btn-primary' : 'btn-ghost'"
            data-testid="dns-visual-tab"
            :disabled="saving || validating"
            @click="switchMode('visual')"
          >
            {{ uiText('可视配置', 'Visual Editor') }}
          </button>
          <button
            class="btn btn-sm join-item"
            :class="mode === 'raw' ? 'btn-primary' : 'btn-ghost'"
            data-testid="dns-raw-tab"
            :disabled="saving || validating"
            @click="switchMode('raw')"
          >
            Raw YAML
          </button>
        </div>
      </div>

      <template #actions>
        <div v-if="settings" class="flex items-center gap-2">
          <span class="badge badge-sm badge-neutral font-mono text-[11px] opacity-75">
            rev {{ settings.revision }}
          </span>
          <button
            class="btn btn-ghost btn-sm text-xs"
            :disabled="busy"
            @click="load(false)"
          >
            {{ uiText('重新载入', 'Reload') }}
          </button>
        </div>
      </template>
    </PageToolbar>

    <!-- Loading / Status Alerts -->
    <div v-if="loading" role="status" class="alert shadow-xs">
      <span class="loading loading-spinner loading-xs text-primary" />
      <span class="text-xs font-medium">{{ uiText('正在读取 DNS 设置…', 'Loading DNS settings…') }}</span>
    </div>

    <div v-if="error" role="alert" data-testid="dns-error" class="alert alert-error shadow-xs">
      <span class="text-xs font-medium flex-1">{{ error }}</span>
      <button class="btn btn-sm btn-ghost border-current/30" @click="load(false)">
        {{ uiText('重试 / 重载', 'Retry / Reload') }}
      </button>
    </div>

    <div v-if="notice" role="status" class="alert alert-success shadow-xs">
      <span class="text-xs font-medium">{{ notice }}</span>
    </div>

    <!-- Main Configuration Card -->
    <template v-if="settings">
      <section class="card bg-base-200/80 border border-base-300 rounded-box shadow-xs">
        <div class="card-body p-4 sm:p-6 space-y-6">
          <!-- Visual Mode: Light Grouping (Basic / Upstreams & Policy / Advanced) in Same Draft -->
          <template v-if="mode === 'visual'">
            <!-- Group 1: Basic Settings (基础设置) -->
            <div class="space-y-3">
              <div class="flex items-center justify-between pb-1 border-b border-base-300/40">
                <span class="text-xs font-bold uppercase tracking-wider text-base-content/70">
                  {{ uiText('基础设置', 'Basic Settings') }}
                </span>
                <span class="text-[11px] opacity-60">
                  {{ uiText('常用基础参数与缓存行为', 'Common scalar parameters and caching behavior') }}
                </span>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-3">
                <!-- ipv6 toggle -->
                <label class="flex flex-col justify-between p-2.5 rounded-lg bg-base-300/40 border border-base-300 cursor-pointer">
                  <span class="text-xs font-mono font-semibold text-base-content/90">ipv6</span>
                  <div class="mt-2 flex items-center justify-between">
                    <span class="text-xs opacity-60">{{ uiText('解析 IPv6', 'Resolve IPv6') }}</span>
                    <input
                      type="checkbox"
                      class="toggle toggle-primary toggle-sm"
                      aria-label="ipv6"
                      :checked="getBool('ipv6')"
                      @change="e => setBool('ipv6', (e.target as HTMLInputElement).checked)"
                    />
                  </div>
                </label>

                <!-- enhanced-mode select -->
                <label class="flex flex-col p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                  <span class="text-xs font-mono font-semibold text-base-content/90">enhanced-mode</span>
                  <select
                    class="select select-sm mt-1 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg"
                    aria-label="enhanced-mode"
                    :value="getString('enhanced-mode', 'fake-ip')"
                    @change="e => setString('enhanced-mode', (e.target as HTMLSelectElement).value)"
                  >
                    <option value="fake-ip">fake-ip</option>
                    <option value="redir-host">redir-host</option>
                  </select>
                </label>

                <!-- listen input -->
                <label class="flex flex-col p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                  <span class="text-xs font-mono font-semibold text-base-content/90">listen</span>
                  <input
                    type="text"
                    class="input input-sm mt-1 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg"
                    aria-label="listen"
                    :value="getString('listen')"
                    placeholder="0.0.0.0:1053"
                    @input="e => setString('listen', (e.target as HTMLInputElement).value)"
                  />
                </label>

                <!-- fake-ip-range input -->
                <label class="flex flex-col p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                  <span class="text-xs font-mono font-semibold text-base-content/90">fake-ip-range</span>
                  <input
                    type="text"
                    class="input input-sm mt-1 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg"
                    aria-label="fake-ip-range"
                    :value="getString('fake-ip-range')"
                    placeholder="198.18.0.1/16"
                    @input="e => setString('fake-ip-range', (e.target as HTMLInputElement).value)"
                  />
                </label>

                <!-- cache-algorithm select -->
                <label class="flex flex-col p-2.5 rounded-lg bg-base-300/40 border border-base-300">
                  <span class="text-xs font-mono font-semibold text-base-content/90">cache-algorithm</span>
                  <select
                    class="select select-sm mt-1 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg"
                    aria-label="cache-algorithm"
                    :value="getString('cache-algorithm')"
                    @change="e => setString('cache-algorithm', (e.target as HTMLSelectElement).value)"
                  >
                    <option value="">{{ uiText('默认 (未指定)', 'Default (unspecified)') }}</option>
                    <option value="arc">arc</option>
                    <option value="bip">bip</option>
                  </select>
                </label>

                <!-- use-hosts toggle -->
                <label class="flex flex-col justify-between p-2.5 rounded-lg bg-base-300/40 border border-base-300 cursor-pointer">
                  <span class="text-xs font-mono font-semibold text-base-content/90">use-hosts</span>
                  <div class="mt-2 flex items-center justify-between">
                    <span class="text-xs opacity-60">{{ uiText('使用 hosts', 'Use hosts') }}</span>
                    <input
                      type="checkbox"
                      class="toggle toggle-primary toggle-sm"
                      aria-label="use-hosts"
                      :checked="getBool('use-hosts')"
                      @change="e => setBool('use-hosts', (e.target as HTMLInputElement).checked)"
                    />
                  </div>
                </label>

                <!-- use-system-hosts toggle -->
                <label class="flex flex-col justify-between p-2.5 rounded-lg bg-base-300/40 border border-base-300 cursor-pointer">
                  <span class="text-xs font-mono font-semibold text-base-content/90">use-system-hosts</span>
                  <div class="mt-2 flex items-center justify-between">
                    <span class="text-xs opacity-60">{{ uiText('使用系统 hosts', 'Use system hosts') }}</span>
                    <input
                      type="checkbox"
                      class="toggle toggle-primary toggle-sm"
                      aria-label="use-system-hosts"
                      :checked="getBool('use-system-hosts')"
                      @change="e => setBool('use-system-hosts', (e.target as HTMLInputElement).checked)"
                    />
                  </div>
                </label>

                <!-- respect-rules toggle -->
                <label class="flex flex-col justify-between p-2.5 rounded-lg bg-base-300/40 border border-base-300 cursor-pointer">
                  <span class="text-xs font-mono font-semibold text-base-content/90">respect-rules</span>
                  <div class="mt-2 flex items-center justify-between">
                    <span class="text-xs opacity-60">{{ uiText('遵循分流规则', 'Respect rules') }}</span>
                    <input
                      type="checkbox"
                      class="toggle toggle-primary toggle-sm"
                      aria-label="respect-rules"
                      :checked="getBool('respect-rules')"
                      @change="e => setBool('respect-rules', (e.target as HTMLInputElement).checked)"
                    />
                  </div>
                </label>
              </div>
            </div>

            <!-- Group 2: Upstreams & Policy (上游与策略) -->
            <div class="space-y-3">
              <div class="flex items-center justify-between pb-1 border-b border-base-300/40">
                <span class="text-xs font-bold uppercase tracking-wider text-base-content/70">
                  {{ uiText('上游与解析策略', 'Upstreams & Resolution Policy') }}
                </span>
                <span class="text-[11px] opacity-60">
                  {{ uiText('多行列表配置，每行一个条目', 'Multiline list entries, one per line') }}
                </span>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                <label
                  v-for="key in DNS_LIST_KEYS"
                  :key="key"
                  class="flex flex-col gap-1"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-mono font-semibold text-base-content/90">{{ key }}</span>
                    <span class="text-[10px] opacity-60">{{ uiText('每行一条', 'One per line') }}</span>
                  </div>
                  <textarea
                    class="textarea textarea-sm min-h-24 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg"
                    :aria-label="key"
                    :value="getListText(key)"
                    :placeholder="uiText('每行一个条目', 'One entry per line')"
                    @input="e => setListText(key, (e.target as HTMLTextAreaElement).value)"
                    @change="e => setListText(key, (e.target as HTMLTextAreaElement).value)"
                  />
                </label>
              </div>
            </div>

            <!-- Group 3: Advanced Configuration (高级配置) -->
            <div class="space-y-3">
              <div class="flex items-center justify-between pb-1 border-b border-base-300/40">
                <span class="text-xs font-bold uppercase tracking-wider text-base-content/70">
                  {{ uiText('高级策略与未知键', 'Advanced Policy & Custom Keys') }}
                </span>
                <span class="text-[11px] opacity-60">
                  {{ uiText('标准 JSON 格式，保留所有未识别扩展键', 'Valid JSON format, preserving all custom extensions') }}
                </span>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <label
                  v-for="key in DNS_JSON_KEYS"
                  :key="key"
                  class="flex flex-col gap-1"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-mono font-semibold text-base-content/90">{{ key }} (JSON)</span>
                    <span v-if="fieldErrors[key]" class="text-error text-xs font-semibold">{{ fieldErrors[key] }}</span>
                  </div>
                  <textarea
                    class="textarea textarea-sm min-h-28 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg"
                    :class="{ 'textarea-error border-error': fieldErrors[key] }"
                    :aria-label="key"
                    :value="getJsonText(key)"
                    @input="e => setJsonText(key, (e.target as HTMLTextAreaElement).value)"
                    @change="e => setJsonText(key, (e.target as HTMLTextAreaElement).value)"
                  />
                </label>
              </div>

              <!-- Indicator if unknown/custom fields are in draft -->
              <div v-if="unknownKeys.length > 0" class="p-3 rounded-lg bg-base-300/30 border border-base-300/60 text-xs flex items-center justify-between">
                <span class="opacity-80">
                  {{ uiText('已保全自定义/扩展字段：', 'Preserved custom/extended fields: ') }}
                  <span class="font-mono font-semibold text-primary">{{ unknownKeys.join(', ') }}</span>
                </span>
                <span class="text-[10px] opacity-60">
                  {{ uiText('在 Raw YAML 模式下可查看并编辑', 'View and edit in Raw YAML mode') }}
                </span>
              </div>
            </div>
          </template>

          <!-- Raw YAML Mode -->
          <div v-else class="flex flex-col gap-1.5">
            <div class="flex items-center justify-between">
              <span class="text-xs font-semibold text-base-content/80">{{ uiText('完整 YAML 草稿', 'Full YAML Draft') }}</span>
              <span class="text-[10px] opacity-60">{{ uiText('保留完整格式与注释', 'Preserves complete format and comments') }}</span>
            </div>
            <textarea
              :value="raw"
              data-testid="dns-raw-editor"
              class="textarea min-h-96 font-mono text-xs w-full bg-base-100 border border-base-300 rounded-lg"
              spellcheck="false"
              @input="e => onRawInput((e.target as HTMLTextAreaElement).value)"
            />
          </div>

          <!-- Bottom Actions & Revision Meta -->
          <div class="flex flex-wrap items-center justify-between gap-3 pt-4 border-t border-base-300/40">
            <span class="text-xs opacity-60 font-mono">
              revision {{ settings.revision }} · {{ settings.updated_at || '—' }}
            </span>
            <button
              class="btn btn-primary"
              data-testid="dns-save"
              :disabled="saving || validating || !dirty || hasValidationErrors"
              @click="save"
            >
              <span v-if="saving" class="loading loading-spinner loading-xs mr-1" />
              {{ saving ? uiText('校验并保存…', 'Validating & saving…') : uiText('校验并保存', 'Validate & Save') }}
            </button>
          </div>
        </div>
      </section>
    </template>
  </section>
</template>

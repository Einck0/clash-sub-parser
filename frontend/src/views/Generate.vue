<template>
  <section class="space-y-6">
    <!-- Top Header -->
    <div class="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 pb-4 border-b border-white/10">
      <div>
        <p class="text-xs font-mono text-blue-400 uppercase tracking-wider">Profile Compiler & Subscriptions</p>
        <h2 class="text-xl font-bold text-white tracking-tight">生成与订阅地址</h2>
        <p class="text-xs text-slate-400 mt-1">
          保存模块开关后，短链接 <code class="text-blue-400">/yaml</code> 会按当前配置实时编译分发。
        </p>
      </div>
      <label class="inline-flex items-center gap-2.5 min-h-[44px] px-3.5 py-2 rounded-lg border border-white/10 bg-slate-900/60 text-xs font-medium text-white cursor-pointer select-none">
        <input
          v-model="switches.enabled"
          type="checkbox"
          class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0 cursor-pointer"
        />
        <span>生成总开关</span>
      </label>
    </div>

    <!-- Alert / Message Banner -->
    <div
      v-if="message"
      class="p-4 rounded-xl border text-xs font-mono"
      :class="messageType === 'error' ? 'border-rose-500/30 bg-rose-500/10 text-rose-400' : 'border-blue-500/30 bg-blue-500/10 text-blue-300'"
      :role="messageType === 'error' ? 'alert' : 'status'"
      aria-live="polite"
    >
      {{ message }}
    </div>

    <!-- Industrial MetricCards Grid for Compile Stats (Adaptive 2 cols mobile, 4 cols md+) -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 sm:gap-4">
      <MetricCard
        label="COMPILED NODES"
        :value="yamlStats?.proxies ?? '-'"
        subtext="生成节点总数"
        :status="yamlStats?.proxies ? 'success' : 'neutral'"
      />
      <MetricCard
        label="POLICY GROUPS"
        :value="yamlStats?.proxy_groups ?? '-'"
        subtext="分流策略组"
        :status="yamlStats?.proxy_groups ? 'info' : 'neutral'"
      />
      <MetricCard
        label="ACTIVE RULES"
        :value="yamlStats?.rules ?? '-'"
        subtext="写入规则条目"
        :status="yamlStats?.rules ? 'info' : 'neutral'"
      />
      <MetricCard
        label="DIALER PROXY"
        :value="yamlStats?.dialer_proxy ?? 0"
        subtext="跳板代理链路"
        :status="yamlStats?.dialer_proxy ? 'warning' : 'neutral'"
      />
    </div>

    <!-- Main Workspace Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-12 gap-6">
      <!-- Left Column: Controls & Single Export -->
      <aside class="lg:col-span-5 space-y-6">
        <!-- Module Toggles Card -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md p-4 sm:p-5 space-y-4">
          <div class="flex items-center justify-between pb-3 border-b border-white/5">
            <div>
              <h3 class="text-sm font-semibold text-white tracking-tight">生成模块</h3>
              <p class="text-xs text-slate-400 mt-0.5">选择要写入最终配置的模块。</p>
            </div>
            <span
              class="px-2.5 py-1 rounded-md text-[10px] font-mono border"
              :class="[
                statusType === 'error'
                  ? 'border-rose-500/30 bg-rose-500/10 text-rose-400'
                  : statusType === 'busy'
                    ? 'border-amber-500/30 bg-amber-500/10 text-amber-300 animate-pulse'
                    : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400'
              ]"
            >
              {{ saveStatus }}
            </span>
          </div>

          <!-- Module Switches Grid (Min-h 44px Touch Targets) -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
            <label class="flex min-h-[44px] items-center gap-2.5 px-3 py-2 rounded-lg border border-white/10 bg-slate-950/60 text-xs text-slate-300 cursor-pointer select-none hover:bg-slate-900 transition-colors">
              <input
                v-model="switches.subscriptions"
                type="checkbox"
                :disabled="!switches.enabled"
                class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
              />
              <span>订阅节点</span>
            </label>
            <label class="flex min-h-[44px] items-center gap-2.5 px-3 py-2 rounded-lg border border-white/10 bg-slate-950/60 text-xs text-slate-300 cursor-pointer select-none hover:bg-slate-900 transition-colors">
              <input
                v-model="switches.node_groups"
                type="checkbox"
                :disabled="!switches.enabled"
                class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
              />
              <span>节点组</span>
            </label>
            <label class="flex min-h-[44px] items-center gap-2.5 px-3 py-2 rounded-lg border border-white/10 bg-slate-950/60 text-xs text-slate-300 cursor-pointer select-none hover:bg-slate-900 transition-colors">
              <input
                v-model="switches.rules"
                type="checkbox"
                :disabled="!switches.enabled"
                class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
              />
              <span>分流规则</span>
            </label>
            <label class="flex min-h-[44px] items-center gap-2.5 px-3 py-2 rounded-lg border border-white/10 bg-slate-950/60 text-xs text-slate-300 cursor-pointer select-none hover:bg-slate-900 transition-colors">
              <input
                v-model="switches.dns"
                type="checkbox"
                :disabled="!switches.enabled"
                class="rounded border-white/20 bg-slate-900 text-blue-500 focus:ring-0"
              />
              <span>DNS 设置</span>
            </label>
          </div>

          <!-- Build Button -->
          <button
            class="w-full min-h-[44px] inline-flex items-center justify-center gap-2 rounded-lg border border-blue-500/40 bg-blue-600 px-4 py-2.5 text-xs font-semibold text-white shadow-md hover:bg-blue-500 transition-all cursor-pointer disabled:opacity-50"
            data-testid="generate-yaml"
            :disabled="!!working"
            @click="buildYaml"
          >
            <span v-if="working === 'yaml'" class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-solid border-current border-r-transparent"></span>
            {{ working === 'yaml' ? '正在编译生成 YAML…' : '⚡ 立即生成 YAML' }}
          </button>

          <!-- Samples hint if present -->
          <div v-if="yamlStats?.dialer_samples?.length" class="text-[11px] font-mono text-slate-400 bg-slate-950/40 rounded-lg p-2.5 border border-white/5 space-y-1">
            <span class="text-blue-400">跳板链路样例:</span>
            <div class="truncate text-slate-300">
              {{ yamlStats.dialer_samples.map((x) => `${x.name}→${x['dialer-proxy']}`).slice(0, 4).join('；') }}
            </div>
          </div>
        </div>

        <!-- Single Subscription Export Card -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md p-4 sm:p-5 space-y-4">
          <div class="pb-3 border-b border-white/5">
            <h3 class="text-sm font-semibold text-white tracking-tight">按订阅单独导出</h3>
            <p class="text-xs text-slate-400 mt-0.5">选择单个订阅导出其专有配置。</p>
          </div>

          <div class="space-y-1.5">
            <label class="text-xs font-mono text-slate-400">选择目标订阅</label>
            <select
              v-model="selectedSubscriptionId"
              class="w-full min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3.5 py-2 text-xs text-white font-mono focus:border-blue-500 focus:outline-hidden cursor-pointer"
            >
              <option :value="null">-- 选择订阅 --</option>
              <option v-for="sub in subscriptions" :key="sub.id" :value="sub.id">
                {{ sub.name }}
              </option>
            </select>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
            <button
              class="min-h-[44px] inline-flex items-center justify-center rounded-lg border border-blue-500/40 bg-blue-600/20 px-3.5 py-2 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer disabled:opacity-50"
              :disabled="!selectedSubscriptionId || working === 'subscription'"
              @click="buildSubscription"
            >
              {{ working === 'subscription' ? '生成中…' : '生成单独订阅' }}
            </button>
            <button
              class="min-h-[44px] inline-flex items-center justify-center rounded-lg border border-white/10 bg-slate-800/40 px-3.5 py-2 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer disabled:opacity-50"
              :disabled="!subscriptionResult"
              @click="download(subscriptionResult, 'subscription.yaml', 'text/yaml')"
            >
              下载单独订阅
            </button>
          </div>
        </div>
      </aside>

      <!-- Right Column: Subscription Links & Output -->
      <main class="lg:col-span-7 space-y-6">
        <!-- Links Grid -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <!-- Short URL Card -->
          <div class="flex flex-col justify-between p-4 sm:p-5 rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md space-y-3">
            <div>
              <strong class="text-sm font-semibold text-white block">短订阅地址</strong>
              <p class="text-xs text-slate-400 mt-0.5">不带 query，使用当前保存配置</p>
            </div>
            <LinkRow label="YAML" :value="yamlCurrentUrl" @copy="copy" />
            <div v-if="yamlCurrentUrl" class="flex justify-center pt-2">
              <div class="p-2 bg-white rounded-xl shadow-md">
                <QrCode :url="yamlCurrentUrl" :size="120" />
              </div>
            </div>
          </div>

          <!-- Full URL Card -->
          <div class="flex flex-col justify-between p-4 sm:p-5 rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md space-y-3">
            <div>
              <strong class="text-sm font-semibold text-white block">完整订阅地址</strong>
              <p class="text-xs text-slate-400 mt-0.5">带 query，适合临时覆盖</p>
            </div>
            <LinkRow label="YAML" :value="yamlSubscribeUrl" @copy="copy" />
            <div class="text-xs font-mono text-slate-500 pt-2 leading-relaxed">
              支持在外部通过 HTTP GET 请求直接拉取并指定模块开关。
            </div>
          </div>
        </div>

        <!-- Compiled Result Output Card -->
        <div class="rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md p-4 sm:p-5 space-y-3">
          <div class="flex items-center justify-between pb-3 border-b border-white/5">
            <strong class="text-sm font-semibold text-white">YAML 预览</strong>
            <div class="flex items-center gap-2">
              <button
                class="min-h-[44px] px-3.5 py-1.5 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer"
                :disabled="!yamlResult"
                @click="copy(yamlResult)"
              >
                复制
              </button>
              <button
                class="min-h-[44px] px-3.5 py-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer"
                :disabled="!yamlResult"
                @click="download(yamlResult, 'generated-config.yaml', 'text/yaml')"
              >
                下载
              </button>
            </div>
          </div>
          <textarea
            data-testid="generated-yaml-output"
            v-model="yamlResult"
            placeholder="点击“⚡ 立即生成 YAML”后显示生成的内容…"
            class="w-full h-80 rounded-lg border border-white/10 bg-slate-950/80 p-3.5 text-xs text-slate-300 font-mono focus:border-blue-500 focus:outline-hidden resize-y"
          ></textarea>
        </div>

        <!-- Separate Subscription Output Card (if generated) -->
        <div v-if="subscriptionResult" class="rounded-xl border border-white/10 bg-slate-900/60 backdrop-blur-md p-4 sm:p-5 space-y-3">
          <div class="flex items-center justify-between pb-3 border-b border-white/5">
            <strong class="text-sm font-semibold text-white">单独订阅内容</strong>
            <div class="flex items-center gap-2">
              <button
                class="min-h-[44px] px-3.5 py-1.5 rounded-lg border border-white/10 bg-slate-800/40 text-xs font-medium text-slate-300 hover:bg-slate-800 transition-colors cursor-pointer"
                @click="copy(subscriptionResult)"
              >
                复制
              </button>
              <button
                class="min-h-[44px] px-3.5 py-1.5 rounded-lg border border-blue-500/40 bg-blue-600/20 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer"
                @click="download(subscriptionResult, 'subscription.yaml', 'text/yaml')"
              >
                下载
              </button>
            </div>
          </div>
          <textarea
            v-model="subscriptionResult"
            class="w-full h-64 rounded-lg border border-white/10 bg-slate-950/80 p-3.5 text-xs text-slate-300 font-mono focus:border-blue-500 focus:outline-hidden resize-y"
          ></textarea>
        </div>
      </main>
    </div>
  </section>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue'
import { useAppStore } from '../stores/app'
import { withAuthToken } from '../auth'
import QrCode from '../components/QrCode.vue'
import MetricCard from '../components/ui/MetricCard.vue'
import {
  generateSubscriptionYaml,
  generateYaml,
  getApiErrorMessage,
  getGenerateSettings,
  getSecuritySettings,
  getSubscriptions,
  updateGenerateSettings,
} from '../api'

const store = useAppStore()

const LinkRow = defineComponent({
  props: { label: String, value: String },
  emits: ['copy'],
  setup(props, { emit }) {
    return () =>
      h('div', { class: 'flex items-center gap-2' }, [
        h('span', { class: 'text-xs font-mono font-semibold text-slate-400 w-12' }, props.label),
        h('input', {
          value: props.value,
          readonly: true,
          class: 'flex-1 min-h-[44px] rounded-lg border border-white/10 bg-slate-950/60 px-3 py-2 text-xs font-mono text-white truncate focus:outline-hidden',
        }),
        h(
          'button',
          {
            onClick: () => emit('copy', props.value),
            class: 'min-h-[44px] px-3.5 py-2 rounded-lg border border-blue-500/40 bg-blue-600/20 text-xs font-medium text-blue-400 hover:bg-blue-600/30 transition-colors cursor-pointer whitespace-nowrap',
          },
          '复制'
        ),
      ])
  },
})

const switches = reactive({
  enabled: true,
  subscriptions: true,
  node_groups: true,
  rules: true,
  dns: true,
})

const yamlResult = ref('')
const yamlStats = ref(null)
const subscriptions = ref([])
const selectedSubscriptionId = ref(null)
const subscriptionResult = ref('')
const saveStatus = ref('加载中')
const statusType = ref('busy')
const message = ref('')
const messageType = ref('')
const working = ref('')
const settingsLoaded = ref(false)
const exportNeedsToken = ref(false)
let saveTimer = null

onMounted(load)

watch(
  switches,
  () => {
    if (!settingsLoaded.value) return
    setStatus('保存中...', 'busy')
    clearTimeout(saveTimer)
    saveTimer = setTimeout(saveSettings, 300)
  },
  { deep: true }
)

const switchQuery = computed(() => {
  const params = new URLSearchParams({
    enabled: String(switches.enabled),
    subscriptions: String(switches.subscriptions),
    node_groups: String(switches.node_groups),
    rules: String(switches.rules),
    dns: String(switches.dns),
    exclude_node_proxies: 'true',
  })
  return params.toString()
})

const yamlCurrentUrl = computed(() => withAuthToken(`${window.location.origin}/yaml`, exportNeedsToken.value))
const yamlSubscribeUrl = computed(() =>
  withAuthToken(`${window.location.origin}/api/generate/yaml/download?${switchQuery.value}`, exportNeedsToken.value)
)

async function load() {
  setStatus('加载中', 'busy')
  setMessage('', '')
  try {
    const [subRes, settingsRes, securityRes] = await Promise.all([
      getSubscriptions(),
      getGenerateSettings(),
      getSecuritySettings(),
    ])
    subscriptions.value = subRes.data
    Object.assign(switches, {
      enabled: settingsRes.data.enabled,
      subscriptions: settingsRes.data.subscriptions,
      node_groups: settingsRes.data.node_groups,
      rules: settingsRes.data.rules,
      dns: settingsRes.data.dns,
    })
    exportNeedsToken.value = Boolean(securityRes.data.auth_enabled && securityRes.data.protect_exports)
    settingsLoaded.value = true
    setStatus('配置已同步', 'success')
  } catch (err) {
    setStatus('加载失败', 'error')
    setMessage(getApiErrorMessage(err, '加载生成配置失败'), 'error')
  }
}

async function saveSettings() {
  try {
    await updateGenerateSettings({ ...switches, exclude_node_proxies: true })
    setStatus('配置已同步', 'success')
    return true
  } catch (err) {
    setStatus('配置保存失败', 'error')
    setMessage(getApiErrorMessage(err, '配置保存失败'), 'error')
    return false
  }
}

async function buildYaml() {
  working.value = 'yaml'
  setMessage('', '')
  try {
    if (!(await saveSettings())) return
    const { data } = await generateYaml({ ...switches })
    yamlResult.value = data.yaml || ''
    yamlStats.value = data.stats || null
    const dialer = data.stats?.dialer_proxy
    store.success(typeof dialer === 'number' ? `YAML 已生成（dialer-proxy ${dialer}）` : 'YAML 已生成')
  } catch (err) {
    store.error(getApiErrorMessage(err, '生成 YAML 失败'))
  } finally {
    working.value = ''
  }
}

async function buildSubscription() {
  working.value = 'subscription'
  setMessage('', '')
  try {
    const { data } = await generateSubscriptionYaml(selectedSubscriptionId.value)
    subscriptionResult.value = data.yaml || ''
    store.success('单独订阅已生成')
  } catch (err) {
    store.error(getApiErrorMessage(err, '生成单独订阅失败'))
  } finally {
    working.value = ''
  }
}

async function copy(text) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    store.success('已复制到剪贴板')
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.cssText = 'position:fixed;opacity:0;left:-9999px'
    document.body.appendChild(textarea)
    textarea.select()
    try {
      document.execCommand('copy')
      store.success('已复制到剪贴板')
    } catch {
      store.error('复制失败，请手动复制')
    } finally {
      document.body.removeChild(textarea)
    }
  }
}

function setStatus(text, type = '') {
  saveStatus.value = text
  statusType.value = type
}

function setMessage(text, type = '') {
  message.value = text
  messageType.value = type
}

function download(content, filename, mimeType) {
  if (!content) return
  const blob = new Blob([content], { type: mimeType })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}
</script>

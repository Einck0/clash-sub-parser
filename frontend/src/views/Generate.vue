<template>
  <section class="page generate-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">Generate</p>
        <h2>生成与订阅地址</h2>
        <p class="page-desc">保存开关后，短链接 `/yaml` 和 `/script` 会按当前配置输出。</p>
      </div>
      <label class="switch-line master-switch"><input type="checkbox" v-model="switches.enabled" /> 总开关</label>
    </div>

    <div class="generate-layout">
      <aside class="generate-side">
        <div class="dns-section generate-control-card">
          <div class="section-title-row">
            <div>
              <h3>生成模块</h3>
              <p class="section-hint">选择要写入最终配置的模块。</p>
            </div>
            <span class="sync-pill" :class="{ error: statusType === 'error', busy: statusType === 'busy' }">{{ saveStatus }}</span>
          </div>
          <div class="toggle-grid generate-toggles">
            <label><input type="checkbox" v-model="switches.subscriptions" :disabled="!switches.enabled" /> 订阅节点</label>
            <label><input type="checkbox" v-model="switches.node_groups" :disabled="!switches.enabled" /> 节点组</label>
            <label><input type="checkbox" v-model="switches.rules" :disabled="!switches.enabled" /> 规则</label>
            <label><input type="checkbox" v-model="switches.dns" :disabled="!switches.enabled" /> DNS</label>
          </div>
          <div class="template-actions" style="margin-top:10px">
            <button class="primary" @click="buildYaml" :disabled="working">{{ working === 'yaml' ? '生成中...' : '生成 YAML' }}</button>
            <button class="primary" @click="buildScript" :disabled="working">{{ working === 'script' ? '生成中...' : '生成 Script.js' }}</button>
            <button @click="buildAll" :disabled="working">{{ working === 'all' ? '生成中...' : '⚡ 全部生成' }}</button>
          </div>
        </div>

        <div class="alert compact-alert" :class="messageType" v-if="message" :role="messageType === 'error' ? 'alert' : 'status'" aria-live="polite">{{ message }}</div>

        <div class="dns-section">
          <h3>按订阅单独导出</h3>
          <label class="field">
            <span>订阅</span>
            <select v-model="selectedSubscriptionId">
              <option :value="null">选择订阅</option>
              <option v-for="sub in subscriptions" :key="sub.id" :value="sub.id">{{ sub.name }}</option>
            </select>
          </label>
          <div class="template-actions" style="margin-top:10px">
            <button class="primary" @click="buildSubscription" :disabled="!selectedSubscriptionId">生成单独订阅</button>
            <button @click="download(subscriptionResult, 'subscription.yaml', 'text/yaml')" :disabled="!subscriptionResult">下载单独订阅</button>
          </div>
        </div>
      </aside>

      <main class="generate-main">
        <div class="link-grid">
          <div class="link-card">
            <div class="row space">
              <div><strong>短订阅地址</strong><p class="section-hint">不带 query，使用当前保存配置</p></div>
            </div>
            <LinkRow label="YAML" :value="yamlCurrentUrl" @copy="copy" />
            <LinkRow label="Script" :value="scriptCurrentUrl" @copy="copy" />
            <div class="qr-row" v-if="yamlCurrentUrl">
              <QrCode :url="yamlCurrentUrl" :size="120" />
            </div>
          </div>

          <div class="link-card">
            <div class="row space">
              <div><strong>完整订阅地址</strong><p class="section-hint">带 query，适合临时覆盖</p></div>
            </div>
            <LinkRow label="YAML" :value="yamlSubscribeUrl" @copy="copy" />
            <LinkRow label="Script" :value="scriptSubscribeUrl" @copy="copy" />
          </div>
        </div>

        <div class="result-grid">
          <div class="result-card">
            <div class="row space result-head">
              <strong>YAML</strong>
              <div class="action-row compact-actions">
                <button @click="copy(yamlResult)">复制</button>
                <button @click="download(yamlResult, 'generated-config.yaml', 'text/yaml')">下载</button>
              </div>
            </div>
            <textarea v-model="yamlResult" placeholder="点击“生成 YAML”后显示"></textarea>
          </div>

          <div class="result-card">
            <div class="row space result-head">
              <strong>Script.js</strong>
              <div class="action-row compact-actions">
                <button @click="copy(scriptResult)">复制</button>
                <button @click="download(scriptResult, 'generated-script.js', 'text/javascript')">下载</button>
              </div>
            </div>
            <textarea v-model="scriptResult" placeholder="点击“生成 Script.js”后显示"></textarea>
          </div>
        </div>

        <div class="result-card" v-if="subscriptionResult">
          <div class="row space result-head">
            <strong>单独订阅内容</strong>
            <div class="action-row compact-actions">
              <button @click="copy(subscriptionResult)">复制</button>
              <button @click="download(subscriptionResult, 'subscription.yaml', 'text/yaml')">下载</button>
            </div>
          </div>
          <textarea v-model="subscriptionResult"></textarea>
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

const store = useAppStore()
import {
  generateScript,
  generateSubscriptionYaml,
  generateYaml,
  getApiErrorMessage,
  getGenerateSettings,
  getSecuritySettings,
  getSubscriptions,
  updateGenerateSettings,
} from '../api'

const LinkRow = defineComponent({
  props: { label: String, value: String },
  emits: ['copy'],
  setup(props, { emit }) {
    return () => h('div', { class: 'link-row' }, [
      h('strong', props.label),
      h('input', { value: props.value, readonly: true }),
      h('button', { onClick: () => emit('copy', props.value) }, '复制'),
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

const scriptResult = ref('')
const yamlResult = ref('')
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
const scriptCurrentUrl = computed(() => withAuthToken(`${window.location.origin}/script`, exportNeedsToken.value))
const yamlSubscribeUrl = computed(() => withAuthToken(`${window.location.origin}/api/generate/yaml/download?${switchQuery.value}`, exportNeedsToken.value))
const scriptSubscribeUrl = computed(() => withAuthToken(`${window.location.origin}/api/generate/script/download?${switchQuery.value}`, exportNeedsToken.value))

async function load() {
  setStatus('加载中', 'busy')
  setMessage('', '')
  try {
    const [subRes, settingsRes, securityRes] = await Promise.all([getSubscriptions(), getGenerateSettings(), getSecuritySettings()])
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

async function buildScript() {
  working.value = 'script'
  setMessage('', '')
  try {
    if (!(await saveSettings())) return
    const { data } = await generateScript({ ...switches, exclude_node_proxies: true })
    scriptResult.value = data.script || ''
    store.success('Script.js 已生成')
  } catch (err) {
    store.error(getApiErrorMessage(err, '生成 Script.js 失败'))
  } finally {
    working.value = ''
  }
}

async function buildYaml() {
  working.value = 'yaml'
  setMessage('', '')
  try {
    if (!(await saveSettings())) return
    const { data } = await generateYaml({ ...switches })
    yamlResult.value = data.yaml || ''
    store.success('YAML 已生成')
  } catch (err) {
    store.error(getApiErrorMessage(err, '生成 YAML 失败'))
  } finally {
    working.value = ''
  }
}

async function buildAll() {
  working.value = 'all'
  setMessage('', '')
  try {
    if (!(await saveSettings())) return
    const [yamlRes, scriptRes] = await Promise.all([
      generateYaml({ ...switches }),
      generateScript({ ...switches, exclude_node_proxies: true }),
    ])
    yamlResult.value = yamlRes.data.yaml || ''
    scriptResult.value = scriptRes.data.script || ''
    store.success('YAML 和 Script.js 已全部生成')
  } catch (err) {
    store.error(getApiErrorMessage(err, '生成失败'))
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
    // Fallback for HTTP environments where clipboard API is unavailable
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

<style scoped>
.qr-row {
  margin-top: 10px;
  display: flex;
  justify-content: center;
}
</style>

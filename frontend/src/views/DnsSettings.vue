<template>
  <section class="page dns-page">
    <div class="page-head">
      <div>
        <p class="eyebrow">DNS</p>
        <h2>DNS 编辑</h2>
        <p class="page-desc">常用项可视化编辑，复杂配置保留 Raw YAML 直接改。保存时会同步成 Clash DNS YAML。</p>
      </div>
      <div class="head-actions">
        <label class="switch-line"><input type="checkbox" v-model="enabled" /> 启用 DNS</label>
        <button @click="load">刷新</button>
        <button class="primary" @click="save" :disabled="saving">{{ saving ? '保存中...' : '保存' }}</button>
      </div>
    </div>

    <div class="alert" :class="messageType" v-if="message" :role="messageType === 'error' ? 'alert' : 'status'" aria-live="polite">{{ message }}</div>

    <div class="dns-tabs">
      <button :class="{ primary: activeTab === 'visual' }" @click="switchTab('visual')">可视化</button>
      <button :class="{ primary: activeTab === 'raw' }" @click="switchTab('raw')">Raw YAML</button>
    </div>

    <template v-if="activeTab === 'visual'">
      <div class="dns-layout">
        <div class="dns-main">
          <div class="dns-section">
            <h3>基础行为</h3>
            <div class="dns-form-grid">
              <label class="field"><span>解析模式</span>
                <select v-model="dns['enhanced-mode']">
                  <option value="fake-ip">fake-ip</option>
                  <option value="redir-host">redir-host</option>
                  <option value="normal">normal</option>
                </select>
              </label>
              <label class="field"><span>Fake IP 段</span><input v-model="dns['fake-ip-range']" placeholder="198.18.0.1/16" /></label>
              <label class="field"><span>缓存算法</span><input v-model="dns['cache-algorithm']" placeholder="arc / lru" /></label>
              <label class="field"><span>默认监听</span><input v-model="dns.listen" placeholder="0.0.0.0:1053（可选）" /></label>
            </div>
            <div class="toggle-grid">
              <label><input type="checkbox" v-model="dns.enable" /> enable</label>
              <label><input type="checkbox" v-model="dns.ipv6" /> ipv6</label>
              <label><input type="checkbox" v-model="dns['prefer-h3']" /> prefer-h3</label>
              <label><input type="checkbox" v-model="dns['respect-rules']" /> respect-rules</label>
              <label><input type="checkbox" v-model="dns['use-system-hosts']" /> use-system-hosts</label>
              <label><input type="checkbox" v-model="dns['direct-nameserver-follow-policy']" /> direct-nameserver-follow-policy</label>
            </div>
          </div>

          <div class="dns-section">
            <h3>DNS 服务器</h3>
            <div class="list-grid">
              <ListEditor title="default-nameserver" hint="解析 DNS 服务器域名用" v-model="dns['default-nameserver']" />
              <ListEditor title="nameserver" hint="主要解析服务器" v-model="dns.nameserver" />
              <ListEditor title="fallback" hint="备用/可信解析服务器" v-model="dns.fallback" />
              <ListEditor title="proxy-server-nameserver" hint="代理服务器域名解析" v-model="dns['proxy-server-nameserver']" />
              <ListEditor title="direct-nameserver" hint="DIRECT 出口解析" v-model="dns['direct-nameserver']" />
              <ListEditor title="fake-ip-filter" hint="这些域名不进 fake-ip" v-model="dns['fake-ip-filter']" />
            </div>
          </div>

          <div class="dns-section">
            <h3>nameserver-policy</h3>
            <p class="section-hint">左边填域名/geosite，右边填 DNS；多个 DNS 用英文逗号分隔。</p>
            <div class="policy-list">
              <div class="policy-row" v-for="(row, idx) in policyRows" :key="idx">
                <input v-model="row.key" placeholder="+.example.com / geosite:cn" />
                <input v-model="row.value" placeholder="223.5.5.5 或 https://..." />
                <button class="danger" @click="policyRows.splice(idx, 1)">删除</button>
              </div>
              <button @click="policyRows.push({ key: '', value: '' })">新增 policy</button>
            </div>
          </div>
        </div>

        <aside class="dns-side">
          <div class="dns-section">
            <h3>fallback-filter</h3>
            <label class="switch-line"><input type="checkbox" v-model="fallbackFilter.geoip" /> geoip</label>
            <label class="field"><span>geoip-code</span><input v-model="fallbackFilter['geoip-code']" placeholder="cn" /></label>
            <ListEditor title="ipcidr" hint="例如 240.0.0.0/4" v-model="fallbackFilter.ipcidr" compact />
            <ListEditor title="domain" hint="例如 +.google.com" v-model="fallbackFilter.domain" compact />
          </div>

          <div class="dns-section">
            <h3>快速模板</h3>
            <div class="template-actions">
              <button @click="applyTemplate('current')">恢复当前推荐</button>
              <button @click="applyTemplate('china')">国内优先</button>
              <button @click="applyTemplate('foreign')">DoH 优先</button>
            </div>
            <p class="section-hint">模板只覆盖常用 DNS 字段，不会动你在 Raw 里新增的其它高级字段。</p>
          </div>
        </aside>
      </div>
    </template>

    <template v-else>
      <div class="raw-dns-card">
        <div class="row space" style="margin-bottom:8px">
          <div>
            <h3 style="margin:0">Raw YAML</h3>
            <p class="section-hint">可以粘贴完整 dns: 片段；切回可视化时会尝试解析。</p>
          </div>
          <button @click="formatRaw">格式化</button>
        </div>
        <textarea v-model="rawYaml" class="dns-raw-textarea" spellcheck="false"></textarea>
      </div>
    </template>
  </section>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from 'vue'
import yaml from 'js-yaml'
import { getApiErrorMessage, getDns, updateDns } from '../api'

const ListEditor = defineComponent({
  name: 'ListEditor',
  props: { title: String, hint: String, modelValue: { type: Array, default: () => [] }, compact: Boolean },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    function update(idx, value) {
      const next = [...props.modelValue]
      next[idx] = value
      emit('update:modelValue', next)
    }
    function add() { emit('update:modelValue', [...props.modelValue, '']) }
    function remove(idx) { emit('update:modelValue', props.modelValue.filter((_, i) => i !== idx)) }
    return () => h('div', { class: ['list-editor', props.compact ? 'compact' : ''] }, [
      h('div', { class: 'list-editor-head' }, [
        h('div', [h('strong', props.title), props.hint ? h('p', { class: 'section-hint' }, props.hint) : null]),
        h('button', { onClick: add }, '新增'),
      ]),
      ...(props.modelValue || []).map((item, idx) => h('div', { class: 'list-row', key: idx }, [
        h('input', { value: item, placeholder: props.title, onInput: (e) => update(idx, e.target.value) }),
        h('button', { class: 'danger', onClick: () => remove(idx) }, '删'),
      ])),
      (!props.modelValue || props.modelValue.length === 0) ? h('div', { class: 'empty-mini' }, '暂无条目') : null,
    ])
  },
})

const rawYaml = ref('')
const enabled = ref(true)
const saving = ref(false)
const message = ref('')
const messageType = ref('')
const activeTab = ref('visual')
const dns = reactive(defaultDns())
const policyRows = ref([])
const fallbackFilter = reactive({ geoip: true, 'geoip-code': 'cn', ipcidr: [], domain: [] })
let syncing = false

onMounted(load)

watch([dns, policyRows, fallbackFilter], () => {
  if (syncing || activeTab.value !== 'visual') return
  rawYaml.value = dumpDns()
}, { deep: true })

async function load() {
  try {
    const { data } = await getDns()
    rawYaml.value = data.raw_yaml || dumpObject(defaultDns())
    enabled.value = !!data.enabled
    parseRawIntoVisual()
    setMessage('', '')
  } catch (err) {
    setMessage(getApiErrorMessage(err, '加载 DNS 配置失败'), 'error')
  }
}

async function save() {
  saving.value = true
  setMessage('', '')
  try {
    if (activeTab.value === 'visual') rawYaml.value = dumpDns()
    else parseRawIntoVisual()
    await updateDns({ raw_yaml: rawYaml.value, enabled: enabled.value })
    setMessage('保存成功', 'success')
  } catch (err) {
    setMessage(getApiErrorMessage(err, '保存失败'), 'error')
  } finally {
    saving.value = false
  }
}

function switchTab(tab) {
  if (tab === activeTab.value) return
  if (tab === 'visual') parseRawIntoVisual()
  else rawYaml.value = dumpDns()
  activeTab.value = tab
}

function parseRawIntoVisual() {
  syncing = true
  try {
    const parsed = yaml.load(rawYaml.value || '{}') || {}
    const obj = parsed.dns && typeof parsed.dns === 'object' ? parsed.dns : parsed
    replaceReactive(dns, { ...defaultDns(), ...obj })
    const policy = obj['nameserver-policy'] || {}
    policyRows.value = Object.entries(policy).map(([key, value]) => ({ key, value: Array.isArray(value) ? value.join(', ') : String(value ?? '') }))
    const ff = obj['fallback-filter'] || {}
    replaceReactive(fallbackFilter, {
      geoip: ff.geoip ?? true,
      'geoip-code': ff['geoip-code'] || 'cn',
      ipcidr: normalizeList(ff.ipcidr),
      domain: normalizeList(ff.domain),
    })
    normalizeDnsLists()
  } finally {
    syncing = false
  }
}

function dumpDns() {
  const obj = cleanObject({ ...dns })
  obj['nameserver-policy'] = buildPolicyObject()
  obj['fallback-filter'] = cleanObject({ ...fallbackFilter })
  return dumpObject(obj)
}

function formatRaw() {
  parseRawIntoVisual()
  rawYaml.value = dumpDns()
}

function applyTemplate(name) {
  const templates = {
    current: {
      'default-nameserver': ['8.8.8.8', '223.5.5.5', '114.114.114.114'],
      nameserver: ['https://1.1.1.1/dns-query', 'https://doh.pub/dns-query'],
      fallback: ['https://1.1.1.1/dns-query', 'https://doh.pub/dns-query'],
      'proxy-server-nameserver': ['https://223.5.5.5/dns-query', 'https://doh.pub/dns-query', 'https://doh.360.cn/dns-query'],
    },
    china: {
      'default-nameserver': ['223.5.5.5', '119.29.29.29', '114.114.114.114'],
      nameserver: ['https://223.5.5.5/dns-query', 'https://doh.pub/dns-query'],
      fallback: ['https://1.1.1.1/dns-query', 'https://8.8.8.8/dns-query'],
      'proxy-server-nameserver': ['https://223.5.5.5/dns-query', 'https://doh.pub/dns-query'],
    },
    foreign: {
      'default-nameserver': ['8.8.8.8', '1.1.1.1', '223.5.5.5'],
      nameserver: ['https://1.1.1.1/dns-query', 'https://8.8.8.8/dns-query', 'https://dns.google/dns-query'],
      fallback: ['https://1.1.1.1/dns-query', 'https://dns.google/dns-query'],
      'proxy-server-nameserver': ['https://1.1.1.1/dns-query', 'https://dns.google/dns-query'],
    },
  }
  Object.assign(dns, templates[name] || templates.current)
}

function buildPolicyObject() {
  const out = {}
  for (const row of policyRows.value) {
    const key = String(row.key || '').trim()
    if (!key) continue
    const parts = String(row.value || '').split(',').map((item) => item.trim()).filter(Boolean)
    out[key] = parts.length > 1 ? parts : (parts[0] || '')
  }
  return out
}

function defaultDns() {
  return {
    enable: true,
    ipv6: false,
    'prefer-h3': true,
    'respect-rules': false,
    'direct-nameserver-follow-policy': false,
    'use-system-hosts': true,
    'enhanced-mode': 'fake-ip',
    'fake-ip-range': '198.18.0.1/16',
    'cache-algorithm': 'arc',
    listen: '',
    'default-nameserver': ['8.8.8.8', '223.5.5.5', '114.114.114.114'],
    nameserver: ['https://1.1.1.1/dns-query', 'https://doh.pub/dns-query'],
    fallback: ['https://1.1.1.1/dns-query', 'https://doh.pub/dns-query'],
    'proxy-server-nameserver': ['https://223.5.5.5/dns-query', 'https://doh.pub/dns-query', 'https://doh.360.cn/dns-query'],
    'direct-nameserver': [],
    'fake-ip-filter': ['*.lan', '*.local', '*.arpa', 'www.msftconnecttest.com'],
  }
}

function normalizeDnsLists() {
  for (const key of ['default-nameserver', 'nameserver', 'fallback', 'proxy-server-nameserver', 'direct-nameserver', 'fake-ip-filter']) {
    dns[key] = normalizeList(dns[key])
  }
}
function normalizeList(value) { return Array.isArray(value) ? value.map(String) : (value ? [String(value)] : []) }
function replaceReactive(target, source) { Object.keys(target).forEach((key) => delete target[key]); Object.assign(target, source) }
function cleanObject(obj) {
  const out = {}
  for (const [key, value] of Object.entries(obj)) {
    if (value === '' || value === null || value === undefined) continue
    if (Array.isArray(value)) out[key] = value.map((item) => String(item).trim()).filter(Boolean)
    else if (typeof value === 'object') out[key] = cleanObject(value)
    else out[key] = value
  }
  return out
}
function dumpObject(obj) { return yaml.dump(cleanObject(obj), { lineWidth: 120, noRefs: true, sortKeys: false }) }
function setMessage(text, type) { message.value = text; messageType.value = type }
</script>

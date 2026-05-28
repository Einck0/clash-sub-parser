<template>
  <section class="page">
    <div class="row space" style="margin-bottom:10px">
      <div class="row">
        <button @click="router.push('/rules')">返回规则</button>
        <h2 style="margin:0">规则编辑：{{ rule.name || `#${ruleId}` }}</h2>
      </div>
      <button class="primary" @click="save">保存</button>
    </div>

    <div class="card" v-if="error" style="border-color: var(--danger); margin-bottom:10px">
      <div style="color: var(--danger)">{{ error }}</div>
    </div>

    <div class="card">
      <div class="row" style="margin-bottom:8px">
        <label><input type="checkbox" v-model="rule.enabled" /> 启用</label>
        <span class="badge">排序：{{ rule.sort_order }}</span>
      </div>
      <div style="display:grid;grid-template-columns: 140px 1fr;gap:10px;align-items:center">
        <label>分类</label><input v-model="rule.category" />
        <label>名称</label><input v-model="rule.name" placeholder="可选" />
        <label>类型</label><input v-model="rule.type" list="rule-type-options" />
        <label>规则值</label><input v-model="rule.value" :disabled="normalizeRuleType(rule.type)==='MATCH'" />
        <label>目标</label>
        <select v-model="rule.proxy">
          <option value="">选择目标</option>
          <option v-for="target in proxyTargets" :key="target" :value="target">{{ target }}</option>
        </select>
        <label>额外参数</label><input v-model="rule.optionsText" placeholder="逗号分隔" />
      </div>
      <datalist id="rule-type-options">
        <option v-for="type in ruleTypes" :key="type" :value="type" />
      </datalist>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getApiErrorMessage, getNodeGroups, getRule, updateRule } from '../api'

const route = useRoute()
const router = useRouter()
const ruleId = computed(() => Number(route.params.id))
const nodeGroups = ref([])
const error = ref('')
const rule = ref({ name: '', category: 'default', type: 'MATCH', value: '', proxy: 'DIRECT', optionsText: '', sort_order: 0, enabled: true })

const builtins = ['DIRECT', 'PASS', 'REJECT']
const ruleTypes = ['DOMAIN', 'DOMAIN-SUFFIX', 'DOMAIN-KEYWORD', 'DOMAIN-REGEX', 'PROCESS-NAME', 'PROCESS-PATH', 'IP-CIDR', 'IP-CIDR6', 'GEOIP', 'GEOSITE', 'DST-PORT', 'SRC-IP-CIDR', 'SRC-PORT', 'RULE-SET', 'MATCH']
const proxyTargets = computed(() => [...builtins, ...nodeGroups.value.map((group) => group.name)])

onMounted(load)

async function load() {
  error.value = ''
  try {
    const [ruleRes, groupsRes] = await Promise.all([getRule(ruleId.value), getNodeGroups()])
    nodeGroups.value = groupsRes.data
    rule.value = { ...ruleRes.data, optionsText: (ruleRes.data.options || []).join(',') }
  } catch (err) {
    error.value = getApiErrorMessage(err, '加载规则失败')
  }
}

async function save() {
  error.value = ''
  const type = normalizeRuleType(rule.value.type)
  const payload = {
    name: String(rule.value.name || '').trim(),
    category: String(rule.value.category || 'default').trim() || 'default',
    type,
    value: type === 'MATCH' ? '' : String(rule.value.value || '').trim(),
    proxy: String(rule.value.proxy || '').trim(),
    options: parseOptions(rule.value.optionsText),
    sort_order: rule.value.sort_order ?? 0,
    enabled: !!rule.value.enabled,
  }
  try {
    await updateRule(ruleId.value, payload)
    await load()
  } catch (err) {
    error.value = getApiErrorMessage(err, '保存规则失败')
  }
}

function normalizeRuleType(type) { return String(type || '').trim().toUpperCase() }
function parseOptions(text) { return String(text || '').split(',').map((item) => item.trim()).filter(Boolean) }
</script>

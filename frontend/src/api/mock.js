const now = new Date().toISOString()

const sampleNodes = [
  { name: '香港 01 · Sample', type: 'ss', server: 'hk.example.net', port: 443 },
  { name: '日本 01 · Sample', type: 'vmess', server: 'jp.example.net', port: 443 },
  { name: '美国 01 · Sample', type: 'trojan', server: 'us.example.net', port: 443 },
  { name: '新加坡 01 · Sample', type: 'ss', server: 'sg.example.net', port: 443 },
]

let subscriptions = [
  {
    id: 1,
    name: 'Primary sample',
    url: 'https://example.com/subscription',
    update_interval: 360,
    is_primary: true,
    node_prefix: '',
    filter_regex: ['香港', '日本', '美国'],
    include_node_names: [],
    exclude_node_names: [],
    source_nodes: sampleNodes,
    raw_nodes: sampleNodes.slice(0, 3),
    last_fetched_at: now,
    last_fetch_error: null,
    fetch_failed_count: 0,
    fetch_comments: ['Static GitHub Pages demo data'],
    subscription_userinfo: 'upload=1073741824; download=21474836480; total=107374182400; expire=1893456000',
    profile_update_interval: '360',
    profile_web_page_url: 'https://example.com',
  },
  {
    id: 2,
    name: 'Manual sample nodes',
    url: 'manual://nodes',
    update_interval: null,
    is_primary: false,
    node_prefix: 'Demo',
    filter_regex: [],
    include_node_names: [],
    exclude_node_names: [],
    source_nodes: [],
    raw_nodes: sampleNodes.slice(2),
    last_fetched_at: now,
    last_fetch_error: null,
    fetch_failed_count: 0,
    fetch_comments: [],
    subscription_userinfo: null,
    profile_update_interval: null,
    profile_web_page_url: null,
  },
]

let nodeGroups = [
  {
    id: 1,
    name: '🚀 节点选择',
    kind: 'manual',
    group_type: 'select',
    sort_order: 1,
    regex_rules: [],
    include_nodes: ['香港 01 · Sample', '日本 01 · Sample', '美国 01 · Sample'],
    include_group_ids: [],
    include_group_nodes_ids: [],
    include_entries: [
      { type: 'builtin', value: 'DIRECT' },
      { type: 'builtin', value: 'REJECT' },
    ],
    add_fallback: true,
    exclude_nodes: [],
    url_test_config: {},
    load_balance_config: {},
    fallback_config: {},
  },
  {
    id: 2,
    name: '香港',
    kind: 'regex',
    group_type: 'url-test',
    sort_order: 2,
    regex_rules: ['香港|HK'],
    include_nodes: [],
    include_group_ids: [],
    include_group_nodes_ids: [],
    include_entries: [],
    add_fallback: false,
    exclude_nodes: [],
    url_test_config: { url: 'http://www.gstatic.com/generate_204', interval: 300 },
    load_balance_config: {},
    fallback_config: {},
  },
  {
    id: 3,
    name: '日本',
    kind: 'regex',
    group_type: 'url-test',
    sort_order: 3,
    regex_rules: ['日本|JP'],
    include_nodes: [],
    include_group_ids: [],
    include_group_nodes_ids: [],
    include_entries: [],
    add_fallback: false,
    exclude_nodes: [],
    url_test_config: { url: 'http://www.gstatic.com/generate_204', interval: 300 },
    load_balance_config: {},
    fallback_config: {},
  },
]

let categories = [
  { id: 1, name: 'AI / Developer', sort_order: 1, rule_count: 3 },
  { id: 2, name: 'Domestic services', sort_order: 2, rule_count: 2 },
  { id: 3, name: 'Final match', sort_order: 3, rule_count: 1 },
]

let rules = [
  { id: 1, name: 'OpenAI', category: 'AI / Developer', type: 'DOMAIN-SUFFIX', value: 'openai.com', proxy: '🚀 节点选择', options: [], sort_order: 1, enabled: true },
  { id: 2, name: 'GitHub', category: 'AI / Developer', type: 'DOMAIN-SUFFIX', value: 'github.com', proxy: '🚀 节点选择', options: [], sort_order: 2, enabled: true },
  { id: 3, name: 'China sites', category: 'Domestic services', type: 'GEOSITE', value: 'cn', proxy: 'DIRECT', options: [], sort_order: 1, enabled: true },
  { id: 4, name: 'LAN', category: 'Domestic services', type: 'GEOIP', value: 'private', proxy: 'DIRECT', options: ['no-resolve'], sort_order: 2, enabled: true },
  { id: 5, name: 'Final', category: 'Final match', type: 'MATCH', value: '', proxy: '🚀 节点选择', options: [], sort_order: 1, enabled: true },
]

let dns = {
  id: 1,
  enabled: true,
  raw_yaml: `enable: true\nipv6: false\nenhanced-mode: fake-ip\nfake-ip-range: 198.18.0.1/16\ndefault-nameserver:\n  - 223.5.5.5\n  - 119.29.29.29\nnameserver:\n  - https://dns.alidns.com/dns-query\n  - https://doh.pub/dns-query\nfallback:\n  - https://1.1.1.1/dns-query\n`,
}

let generateSettings = {
  enabled: true,
  subscriptions: true,
  node_groups: true,
  rules: true,
  dns: true,
  exclude_node_proxies: true,
}

let security = {
  auth_enabled: false,
  protect_frontend: true,
  protect_api: true,
  protect_exports: true,
  has_token: false,
  fetch_proxy_enabled: false,
  fetch_proxy_url: '',
}

function clone(data) {
  return JSON.parse(JSON.stringify(data))
}

function response(data, status = 200, headers = new Headers()) {
  return { data: clone(data), status, headers }
}

function nextId(items) {
  return Math.max(0, ...items.map((item) => Number(item.id) || 0)) + 1
}

function allNodes() {
  return subscriptions.flatMap((sub) => sub.raw_nodes || [])
}

function buildPreview(group) {
  const nodes = group.kind === 'regex'
    ? allNodes().filter((node) => (group.regex_rules || []).some((rule) => new RegExp(rule, 'i').test(node.name)))
    : allNodes().filter((node) => (group.include_nodes || []).includes(node.name))
  return {
    id: group.id,
    name: group.name,
    include_entries: group.include_entries || [],
    include_group_names: [],
    include_group_nodes_names: [],
    resolved_nodes: group.add_fallback === false ? nodes : [...nodes, { name: 'REJECT', type: 'builtin' }],
    resolved_count: group.add_fallback === false ? nodes.length : nodes.length + 1,
  }
}

export async function mockRequest(method, url, data) {
  const cleanUrl = url.split('?')[0]

  if (method === 'GET' && cleanUrl === '/subscriptions') return response(subscriptions)
  if (method === 'GET' && cleanUrl === '/subscriptions/nodes/all') return response(allNodes())
  if (method === 'GET' && cleanUrl.startsWith('/subscriptions/') && cleanUrl.endsWith('/nodes')) {
    const id = Number(cleanUrl.split('/')[2])
    return response(subscriptions.find((sub) => sub.id === id)?.raw_nodes || [])
  }
  if (method === 'POST' && cleanUrl === '/subscriptions') {
    const item = { id: nextId(subscriptions), raw_nodes: [], source_nodes: [], fetch_failed_count: 0, ...data }
    subscriptions.push(item)
    return response(item)
  }
  if (method === 'PATCH' && cleanUrl.startsWith('/subscriptions/')) {
    const id = Number(cleanUrl.split('/')[2])
    subscriptions = subscriptions.map((item) => item.id === id ? { ...item, ...data } : item)
    return response(subscriptions.find((item) => item.id === id))
  }
  if (method === 'POST' && cleanUrl.match(/^\/subscriptions\/\d+\/fetch$/)) return response({ ok: true })
  if (method === 'DELETE' && cleanUrl.startsWith('/subscriptions/')) {
    const id = Number(cleanUrl.split('/')[2])
    subscriptions = subscriptions.filter((item) => item.id !== id)
    return response({ ok: true })
  }

  if (method === 'GET' && cleanUrl === '/node-groups') return response(nodeGroups)
  if (method === 'GET' && cleanUrl === '/node-groups/_preview') return response(nodeGroups.map(buildPreview))
  if (method === 'POST' && cleanUrl === '/node-groups/validate') return response({ ok: true, errors: [] })
  if (method === 'POST' && cleanUrl === '/node-groups/reorder') return response({ ok: true })
  if (method === 'POST' && cleanUrl === '/node-groups') {
    const item = { id: nextId(nodeGroups), sort_order: nextId(nodeGroups), ...data }
    nodeGroups.push(item)
    return response(item)
  }
  if (method === 'PATCH' && cleanUrl.startsWith('/node-groups/')) {
    const id = Number(cleanUrl.split('/')[2])
    nodeGroups = nodeGroups.map((item) => item.id === id ? { ...item, ...data } : item)
    return response(nodeGroups.find((item) => item.id === id))
  }
  if (method === 'DELETE' && cleanUrl.startsWith('/node-groups/')) {
    const id = Number(cleanUrl.split('/')[2])
    nodeGroups = nodeGroups.filter((item) => item.id !== id)
    return response({ ok: true })
  }

  if (method === 'GET' && cleanUrl === '/rule-categories') return response(categories)
  if (method === 'POST' && cleanUrl === '/rule-categories/reorder') return response({ ok: true })
  if (method === 'POST' && cleanUrl === '/rule-categories') {
    const item = { id: nextId(categories), rule_count: 0, ...data }
    categories.push(item)
    return response(item)
  }
  if (method === 'PATCH' && cleanUrl.startsWith('/rule-categories/')) {
    const id = Number(cleanUrl.split('/')[2])
    categories = categories.map((item) => item.id === id ? { ...item, ...data } : item)
    return response(categories.find((item) => item.id === id))
  }
  if (method === 'DELETE' && cleanUrl.startsWith('/rule-categories/')) {
    const id = Number(cleanUrl.split('/')[2])
    categories = categories.filter((item) => item.id !== id)
    return response({ ok: true })
  }

  if (method === 'GET' && cleanUrl === '/rules') return response(rules)
  if (method === 'GET' && cleanUrl.startsWith('/rules/')) return response(rules.find((item) => item.id === Number(cleanUrl.split('/')[2])))
  if (method === 'POST' && cleanUrl === '/rules/reorder') return response({ ok: true })
  if (method === 'POST' && cleanUrl === '/rules') {
    const item = { id: nextId(rules), ...data }
    rules.push(item)
    return response(item)
  }
  if (method === 'PATCH' && cleanUrl.startsWith('/rules/')) {
    const id = Number(cleanUrl.split('/')[2])
    rules = rules.map((item) => item.id === id ? { ...item, ...data } : item)
    return response(rules.find((item) => item.id === id))
  }
  if (method === 'DELETE' && cleanUrl.startsWith('/rules/')) {
    const id = Number(cleanUrl.split('/')[2])
    rules = rules.filter((item) => item.id !== id)
    return response({ ok: true })
  }

  if (method === 'GET' && cleanUrl === '/dns') return response(dns)
  if (method === 'PATCH' && cleanUrl === '/dns') {
    dns = { ...dns, ...data }
    return response(dns)
  }

  if (method === 'GET' && cleanUrl === '/generate/settings') return response(generateSettings)
  if (method === 'PATCH' && cleanUrl === '/generate/settings') {
    generateSettings = { ...generateSettings, ...data }
    return response(generateSettings)
  }
  if (method === 'POST' && cleanUrl === '/generate/yaml') return response({ yaml: sampleYaml() })
  if (method === 'POST' && cleanUrl === '/generate/script') return response({ script: sampleScript() })
  if (method === 'POST' && cleanUrl.startsWith('/generate/subscription/')) return response({ yaml: sampleYaml(true) })

  if (method === 'GET' && cleanUrl === '/settings/security') return response(security)
  if (method === 'PATCH' && cleanUrl === '/settings/security') {
    security = { ...security, ...data, has_token: Boolean(data?.token) || security.has_token }
    delete security.token
    return response(security)
  }
  if (method === 'POST' && cleanUrl.startsWith('/settings/auth/')) return response({ ok: true })
  if (method === 'POST' && cleanUrl === '/settings/reset') return response({ ok: true })
  if (method === 'POST' && cleanUrl === '/settings/import') return response({ ok: true, imported: { subscriptions: subscriptions.length, node_groups: nodeGroups.length, rules: rules.length }, errors: null })

  throw Object.assign(new Error(`Demo mock missing endpoint: ${method} ${url}`), { userMessage: 'Demo mock endpoint missing' })
}

export async function mockFetchExport(includeSubscriptions = true) {
  const tables = {
    node_groups: clone(nodeGroups),
    rules: clone(rules),
    rule_categories: clone(categories),
    dns_config: [clone(dns)],
    generate_config: [clone(generateSettings)],
    security_settings: [clone(security)],
  }
  if (includeSubscriptions) tables.subscriptions = clone(subscriptions)
  const body = JSON.stringify({ version: 1, exported_at: now, include_subscriptions: includeSubscriptions, tables }, null, 2)
  return new Response(body, {
    status: 200,
    headers: {
      'content-type': 'application/json',
      'content-disposition': `attachment; filename="clash-sub-parser-${includeSubscriptions ? 'full' : 'no-subscriptions'}.json"`,
    },
  })
}

function sampleYaml(single = false) {
  const proxies = allNodes().map((node) => `  - { name: ${node.name}, type: ${node.type}, server: ${node.server || 'example.net'}, port: ${node.port || 443} }`).join('\n')
  return `# Static GitHub Pages demo output\nproxies:\n${proxies}\nproxy-groups:\n  - name: 🚀 节点选择\n    type: select\n    proxies:\n      - 香港 01 · Sample\n      - 日本 01 · Sample\n      - 美国 01 · Sample\nrules:\n  - GEOSITE,cn,DIRECT\n  - MATCH,🚀 节点选择\n${single ? '# Single subscription export demo\n' : ''}`
}

function sampleScript() {
  return `// Static GitHub Pages demo output\nfunction main(config) {\n  config.rules = config.rules || []\n  config.rules.push('MATCH,🚀 节点选择')\n  return config\n}\n`
}

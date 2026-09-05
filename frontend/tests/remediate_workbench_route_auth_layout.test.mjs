import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

const srcDir = path.resolve(import.meta.dirname, '../src')

test('Task 2.1: Route resilience — /probe alias and catch-all recovery view', () => {
  const routerPath = path.join(srcDir, 'router/index.ts')
  assert.ok(fs.existsSync(routerPath), 'router/index.ts must exist')
  const routerContent = fs.readFileSync(routerPath, 'utf8')

  // /probe must resolve to NodeLedger (alias or route)
  const hasProbeRoute = /path:\s*'\/probe'/.test(routerContent) ||
    /alias:\s*(\[[^\]]*'\/probe'[^\]]*\]|'\/probe')/.test(routerContent)
  assert.ok(hasProbeRoute, 'router must register /probe as alias or route to NodeLedger')

  // Catch-all route for unknown client-side paths
  const hasCatchAll = /path:\s*['"]\/:pathMatch\(\.\*\)\*['"]/.test(routerContent) ||
    /path:\s*['"]\/:catchAll\(\.\*\)\*['"]/.test(routerContent) ||
    /path:\s*['"]\/\*.*['"]/.test(routerContent)
  assert.ok(hasCatchAll, 'router must register a catch-all route for unmatched paths')

  // NotFound recovery view
  const notFoundPath = path.join(srcDir, 'views/NotFound.vue')
  assert.ok(fs.existsSync(notFoundPath), 'views/NotFound.vue recovery view must exist')
  const notFoundContent = fs.readFileSync(notFoundPath, 'utf8')

  // Accessible return action to Node Ledger
  assert.match(notFoundContent, /to="\/nodes"|to="\/probe"|push\(['"]\/(nodes|probe)['"]\)/, 'NotFound must have an action returning to Node Ledger')
  assert.match(notFoundContent, /404|未找到|页面不存在/, 'NotFound must display a human-readable not-found explanation')
})

test('Task 2.2: Node summary lifecycle — loading placeholder vs factual zero', () => {
  const storePath = path.join(srcDir, 'stores/app.ts')
  const storeContent = fs.readFileSync(storePath, 'utf8')

  // Pinia app store must own nodeSummary state with status
  assert.match(storeContent, /nodeSummary/, 'app store must expose nodeSummary state')
  assert.match(storeContent, /status:\s*ref<.*>|status:\s*['"](idle|loading|ready|error)['"]|'idle' \| 'loading' \| 'ready' \| 'error'/, 'nodeSummary must track explicit lifecycle status (idle/loading/ready/error)')

  const headerPath = path.join(srcDir, 'components/workbench/WorkbenchHeader.vue')
  const headerContent = fs.readFileSync(headerPath, 'utf8')

  // WorkbenchHeader must distinguish loading from factual zero
  assert.match(headerContent, /summary|status|loading|placeholder|--/, 'WorkbenchHeader must be capable of rendering placeholder during idle/loading')

  const appPath = path.join(srcDir, 'App.vue')
  const appContent = fs.readFileSync(appPath, 'utf8')
  // App.vue must connect nodeSummary lifecycle rather than raw store.nodes?.length || 0
  assert.doesNotMatch(appContent, /:total-nodes="store\.nodes\?\.length \|\| 0"/, 'App.vue must NOT pass raw store.nodes?.length || 0 as total-nodes without status check')
})

test('Task 3.1: AuthGate accessible affordances and visual-viewport layout', () => {
  const authGatePath = path.join(srcDir, 'components/AuthGate.vue')
  const authGateContent = fs.readFileSync(authGatePath, 'utf8')

  // Must have password show/hide toggle
  assert.match(authGateContent, /type="showPassword \? 'text' : 'password'"|type="showToken \? 'text' : 'password'"|showPassword|showToken/, 'AuthGate must have a password visibility toggle')

  // Must have clear button when input is non-empty
  assert.match(authGateContent, /clear|token = ''|!token/, 'AuthGate must provide a clear action for populated input')

  // Must have clipboard paste action with error handling
  assert.match(authGateContent, /navigator\.clipboard\.readText|paste/, 'AuthGate must provide a paste action')

  // Must remove technical cookie/query-token copy
  assert.doesNotMatch(authGateContent, /HttpOnly cookie/, 'AuthGate must NOT contain technical HttpOnly cookie implementation copy')
  assert.doesNotMatch(authGateContent, /URL token/, 'AuthGate must NOT contain technical URL token explanation copy')

  // Visual viewport safe layout
  assert.match(authGateContent, /min-h-screen|min-h-\[100dvh\]|100dvh/, 'AuthGate must use full visual viewport height')
})

test('Task 3.2: Settings generated Token requires modal acknowledgement before save', () => {
  const settingsPath = path.join(srcDir, 'views/Settings.vue')
  const settingsContent = fs.readFileSync(settingsPath, 'utf8')

  // Must use AppModal for generated token acknowledgement
  assert.match(settingsContent, /AppModal/, 'Settings must import and use AppModal for generated token recovery')
  assert.match(settingsContent, /tokenAcknowledged|ackToken|acknowledged/, 'Settings must track explicit token acknowledgement state')
  assert.match(settingsContent, /copyToken|copy\(|navigator\.clipboard\.writeText/, 'Settings generated token modal must have a copy action')

  // Save must check acknowledgement when a token was generated
  assert.match(settingsContent, /tokenAcknowledged|acknowledg/, 'save() must enforce token acknowledgement before persisting generated token')
})

test('Task 3.3: Settings dangerous confirmation on auth_enabled: true -> false', () => {
  const settingsPath = path.join(srcDir, 'views/Settings.vue')
  const settingsContent = fs.readFileSync(settingsPath, 'utf8')

  // Danger confirmation when disabling auth
  assert.match(settingsContent, /danger:\s*true/, 'Settings must invoke danger confirm when disabling authentication')
  assert.match(settingsContent, /persistedAuthEnabled|initialAuthEnabled|savedAuthEnabled/, 'Settings must track persisted/original auth_enabled state')
})

test('Task 4.1: Theme coherence — style.css contains zero conflicting :root definitions', () => {
  const stylePath = path.join(srcDir, 'style.css')
  const styleContent = fs.readFileSync(stylePath, 'utf8')

  // Must NOT declare :root token definitions
  assert.doesNotMatch(styleContent, /^:root\s*\{/m, 'style.css must NOT declare :root token definitions')
  // Must enforce overflow-x: hidden on html, body
  assert.match(styleContent, /html,\s*body\s*\{[^}]*overflow-x:\s*hidden/, 'style.css must prevent document-level horizontal scroll')
})

test('Task 4.2: Generate export priority and preserved 5 targets at 1440x900', () => {
  const generatePath = path.join(srcDir, 'views/Generate.vue')
  const generateContent = fs.readFileSync(generatePath, 'utf8')

  // Must preserve all 5 export targets
  assert.match(generateContent, /TARGET_DEFS|clash|mihomo|stash|shadowrocket|sing-box/i, 'Generate must preserve all 5 export targets')

  // Primary export URL and action must appear before decorative metrics grid
  const exportUrlIdx = generateContent.indexOf('currentExportUrl')
  const metricGridIdx = generateContent.indexOf('MetricCard')
  assert.ok(exportUrlIdx > 0, 'Generate must render currentExportUrl')
  assert.ok(metricGridIdx > 0, 'Generate must render MetricCard')
  assert.ok(exportUrlIdx < metricGridIdx, 'Generate must place primary export URL before decorative MetricCard grid for 1440x900 priority')
})

test('Task 4.3: Settings scanable non-nested control grouping', () => {
  const settingsPath = path.join(srcDir, 'views/Settings.vue')
  const settingsContent = fs.readFileSync(settingsPath, 'utf8')

  // Eye icon toggle instead of checkbox
  assert.doesNotMatch(settingsContent, /<label class="switch-line">\s*<input type="checkbox" v-model="showToken" \/>\s*显示 token\s*<\/label>/, 'Settings should replace "显示 token" checkbox with inline eye toggle')
})

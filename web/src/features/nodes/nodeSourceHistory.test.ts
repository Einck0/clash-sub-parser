// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createRouter, createMemoryHistory } from 'vue-router'
import NodesView from './NodesView.vue'
import NodeSourceHistoryPanel from './NodeSourceHistoryPanel.vue'
import { api, ApiError } from '../../api/client'
import {
  attributionStatusBadge,
  attributionCauseLabel,
  relationStateBadge,
  formatObservedTime,
  type NodeSourceHistoryData,
} from './sourceHistoryTypes'
import type { NodeRecord } from './nodeView'

describe('Node Source Attribution & History Specification', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  describe('sourceHistoryTypes formatters & contract invariants', () => {
    it('maps attributionStatusBadge accurately with proper semantic tones', () => {
      expect(attributionStatusBadge('current')).toEqual({
        label: '当前实时归属',
        tone: 'success',
      })
      expect(attributionStatusBadge('historical_verified')).toEqual({
        label: '历史已核验归属',
        tone: 'info',
      })
      expect(attributionStatusBadge('manual_confirmed')).toEqual({
        label: '人工已核验归属',
        tone: 'success',
      })
      expect(attributionStatusBadge('conflict')).toEqual({
        label: '归属存在冲突',
        tone: 'warning',
      })
      expect(attributionStatusBadge('unknown')).toEqual({
        label: '来源证据不足',
        tone: 'info',
      })
      expect(attributionStatusBadge(undefined)).toEqual({
        label: '来源证据不足',
        tone: 'info',
      })
    })

    it('maps attributionCauseLabel to human-readable audit descriptions', () => {
      expect(attributionCauseLabel('legacy_import')).toBe('历史系统导入')
      expect(attributionCauseLabel('refresh_removed')).toBe('刷新下线移除')
      expect(attributionCauseLabel('subscription_deleted')).toBe('订阅源已删除')
      expect(attributionCauseLabel('manual_confirmed')).toBe('人工核验确认')
      expect(attributionCauseLabel('unresolved')).toBe('未决归属')
      expect(attributionCauseLabel(undefined)).toBe('未留记录')
      expect(attributionCauseLabel('custom_cause')).toBe('custom_cause')
    })

    it('maps relationStateBadge accurately', () => {
      expect(relationStateBadge('verified')).toEqual({ label: '已核验', tone: 'success' })
      expect(relationStateBadge('conflict')).toEqual({ label: '冲突', tone: 'warning' })
      expect(relationStateBadge('unknown')).toEqual({ label: '待核验/未知', tone: 'info' })
      expect(relationStateBadge(undefined)).toEqual({ label: '待核验/未知', tone: 'info' })
    })

    it('formats observed timestamps or cleanly displays 未留记录 without fabricating now', () => {
      expect(formatObservedTime(undefined)).toBe('未留记录')
      expect(formatObservedTime('')).toBe('未留记录')
      expect(formatObservedTime('invalid-date')).toBe('未留记录')
      const formatted = formatObservedTime('2026-09-29T10:00:00Z')
      expect(formatted).not.toBe('未留记录')
      expect(formatted).toContain('2026')
    })
  })

  describe('NodeSourceHistoryPanel component rendering', () => {
    it('renders current sources with distinct active vs disabled badges without treating disabled as deleted', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const historyData: NodeSourceHistoryData = {
        attribution_status: 'current',
        current_sources: [
          {
            subscription_id: 'sub-active-1',
            name: 'Active Tokyo Upstream',
            enabled: true,
          },
          {
            subscription_id: 'sub-disabled-2',
            name: 'Paused Osaka Upstream',
            enabled: false,
          },
        ],
        history: [],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-current-1',
            sourceHistory: historyData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      // Mandatory audit disclaimer
      const disclaimer = mountEl.querySelector('[data-testid="source-history-disclaimer"]')
      expect(disclaimer?.textContent).toContain('历史归属仅用于追溯，不代表当前订阅成员或启用状态')

      // Overall status
      const overallBadge = mountEl.querySelector('[data-testid="overall-attribution-status-badge"]')
      expect(overallBadge?.textContent).toContain('当前实时归属')

      // Current sources section
      const activeBadge = mountEl.querySelector('[data-testid="current-source-enabled-badge"]')
      const disabledBadge = mountEl.querySelector('[data-testid="current-source-disabled-badge"]')
      expect(activeBadge).not.toBeNull()
      expect(activeBadge?.textContent).toContain('订阅已启用')
      expect(disabledBadge).not.toBeNull()
      expect(disabledBadge?.textContent).toContain('订阅已停用')
      // Ensure disabled is NOT shown as deleted
      expect(mountEl.querySelectorAll('[data-testid="source-deleted-badge"]').length).toBe(0)

      app.unmount()
      mountEl.remove()
    })

    it('renders multiple verified historical sources without collapsing or discarding edges', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const historyData: NodeSourceHistoryData = {
        attribution_status: 'historical_verified',
        current_sources: [],
        history: [
          {
            source_label: '7li-archive',
            subscription_id: 'sub-7li-old',
            relation_state: 'verified',
            cause: 'legacy_import',
            evidence_kind: 'legacy_archive_link',
            first_observed_at: '2026-01-01T00:00:00Z',
            last_observed_at: '2026-09-29T10:00:00Z',
            connection_revision: 101,
            source_deleted: false,
          },
          {
            source_label: '魔戒-archive',
            subscription_id: 'sub-mojie-old',
            relation_state: 'verified',
            cause: 'legacy_import',
            evidence_kind: 'legacy_archive_link',
            first_observed_at: '2026-02-01T00:00:00Z',
            last_observed_at: '2026-09-29T10:00:00Z',
            connection_revision: 102,
            source_deleted: false,
          },
          {
            source_label: 'einck-qzz-backup',
            subscription_id: 'sub-einck-old',
            relation_state: 'verified',
            cause: 'refresh_removed',
            evidence_kind: 'refresh_pruning_snapshot',
            connection_revision: 103,
            source_deleted: false,
          },
        ],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-multi-verified',
            sourceHistory: historyData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      // Current sources empty notice
      expect(mountEl.querySelector('[data-testid="current-sources-empty"]')?.textContent).toContain(
        '当前无活跃订阅关联 (孤立/失活节点)'
      )

      // Verified history items: all 3 must be rendered individually
      const verifiedItems = mountEl.querySelectorAll('[data-testid="verified-history-item"]')
      expect(verifiedItems.length).toBe(3)
      expect(verifiedItems[0].textContent).toContain('7li-archive')
      expect(verifiedItems[0].textContent).toContain('历史系统导入')
      expect(verifiedItems[0].textContent).toContain('rev. 101')
      expect(verifiedItems[1].textContent).toContain('魔戒-archive')
      expect(verifiedItems[1].textContent).toContain('rev. 102')
      expect(verifiedItems[2].textContent).toContain('einck-qzz-backup')
      expect(verifiedItems[2].textContent).toContain('刷新下线移除')
      expect(verifiedItems[2].textContent).toContain('rev. 103')

      // Timestamp missing for third item handled cleanly as 未留记录
      expect(verifiedItems[2].querySelector('[data-testid="history-timestamp-missing"]')?.textContent).toContain(
        '观测时间: 未留记录'
      )

      app.unmount()
      mountEl.remove()
    })

    it('renders deleted source history with label and Deleted badge without active link', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const historyData: NodeSourceHistoryData = {
        attribution_status: 'historical_verified',
        current_sources: [],
        history: [
          {
            source_label: 'Defunct-Source-Alpha',
            subscription_id: undefined, // Cleared to null via ON DELETE SET NULL
            relation_state: 'verified',
            cause: 'subscription_deleted',
            evidence_kind: 'subscription_deletion_snapshot',
            first_observed_at: '2026-03-01T00:00:00Z',
            last_observed_at: '2026-09-15T00:00:00Z',
            source_deleted: true,
          },
        ],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-deleted-src',
            sourceHistory: historyData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      const deletedBadge = mountEl.querySelector('[data-testid="source-deleted-badge"]')
      expect(deletedBadge).not.toBeNull()
      expect(deletedBadge?.textContent).toContain('已删除')
      expect(mountEl.textContent).toContain('Defunct-Source-Alpha')
      expect(mountEl.textContent).toContain('订阅源已删除')

      app.unmount()
      mountEl.remove()
    })

    it('renders historical unmapped source with 未关联当前订阅 badge instead of 已删除', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const historyData: NodeSourceHistoryData = {
        attribution_status: 'historical_verified',
        current_sources: [],
        history: [
          {
            source_label: 'Legacy-Unmapped-Provider',
            subscription_id: undefined,
            relation_state: 'verified',
            cause: 'legacy_import',
            evidence_kind: 'legacy_cold_archive',
            first_observed_at: '2026-03-01T00:00:00Z',
            last_observed_at: '2026-09-15T00:00:00Z',
            source_deleted: false,
            source_unmapped: true,
          },
        ],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-unmapped-src',
            sourceHistory: historyData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      const unmappedBadge = mountEl.querySelector('[data-testid="source-unmapped-badge"]')
      expect(unmappedBadge).not.toBeNull()
      expect(unmappedBadge?.textContent).toContain('未关联当前订阅')
      const deletedBadge = mountEl.querySelector('[data-testid="source-deleted-badge"]')
      expect(deletedBadge).toBeNull()

      app.unmount()
      mountEl.remove()
    })

    it('preserves conflict fidelity and renders unknown sources as 来源证据不足 instead of junk/garbage', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const historyData: NodeSourceHistoryData = {
        attribution_status: 'conflict',
        current_sources: [],
        history: [
          {
            source_label: 'Conflicting-Peer-Endpoint',
            relation_state: 'conflict',
            cause: 'unresolved',
            evidence_kind: 'dedup_collision_evidence',
            source_deleted: false,
          },
          {
            source_label: 'Unattributed-Candidate',
            relation_state: 'unknown',
            cause: 'unresolved',
            evidence_kind: 'missing_provenance_marker',
            source_deleted: false,
          },
        ],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-conflict-1',
            sourceHistory: historyData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      expect(mountEl.querySelector('[data-testid="conflict-badge"]')?.textContent).toContain('归属冲突')
      expect(mountEl.querySelector('[data-testid="unknown-badge"]')?.textContent).toContain('未知来源')
      expect(mountEl.textContent).not.toContain('垃圾')
      expect(mountEl.textContent).not.toContain('无效节点')

      app.unmount()
      mountEl.remove()
    })

    it('renders 来源证据不足 when both current sources and history are completely empty', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const emptyData: NodeSourceHistoryData = {
        attribution_status: 'unknown',
        current_sources: [],
        history: [],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-empty-orphan',
            sourceHistory: emptyData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      const emptyNotice = mountEl.querySelector('[data-testid="source-history-empty"]')
      expect(emptyNotice).not.toBeNull()
      expect(emptyNotice?.textContent).toContain('来源证据不足，未检索到该节点的历史归属信息')
      expect(emptyNotice?.textContent).not.toContain('垃圾')

      app.unmount()
      mountEl.remove()
    })

    it('renders loading skeleton and error message with retry event', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      let retried = false
      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-err-1',
            sourceHistory: null,
            loading: false,
            error: '未找到该节点的来源历史记录 (404)',
            onRetry: () => {
              retried = true
            },
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      const errEl = mountEl.querySelector('[data-testid="source-history-error"]')
      expect(errEl?.textContent).toContain('404')
      const retryBtn = errEl?.querySelector('button')
      retryBtn?.click()
      expect(retried).toBe(true)

      app.unmount()
      mountEl.remove()
    })

    it('safely escapes untrusted HTML/markup in source labels and evidence kinds', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const xssData: NodeSourceHistoryData = {
        attribution_status: 'historical_verified',
        current_sources: [
          {
            subscription_id: 'sub-xss-1',
            name: '<script>alert("xss-source")</script>',
            enabled: true,
          },
        ],
        history: [
          {
            source_label: '<img src=x onerror=alert(1)>',
            subscription_id: 'sub-legacy',
            relation_state: 'verified',
            cause: 'legacy_import',
            evidence_kind: '<b>fake-evidence</b>',
            source_deleted: false,
          },
        ],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-xss',
            sourceHistory: xssData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      // Scripts and imgs must NOT be injected into the DOM as HTML elements
      expect(mountEl.querySelectorAll('script').length).toBe(0)
      expect(mountEl.querySelectorAll('img').length).toBe(0)
      expect(mountEl.querySelectorAll('b').length).toBe(0)
      // Text content must contain the escaped text literals
      expect(mountEl.textContent).toContain('<script>alert("xss-source")</script>')
      expect(mountEl.textContent).toContain('<img src=x onerror=alert(1)>')
      expect(mountEl.textContent).toContain('<b>fake-evidence</b>')

      app.unmount()
      mountEl.remove()
    })

    it('maintains strict read-only nature with no switch/enable/edit controls in history panel', async () => {
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const historyData: NodeSourceHistoryData = {
        attribution_status: 'historical_verified',
        current_sources: [
          {
            subscription_id: 'sub-1',
            name: 'Upstream 1',
            enabled: true,
          },
        ],
        history: [
          {
            source_label: 'Historic 1',
            relation_state: 'verified',
            cause: 'legacy_import',
            evidence_kind: 'archive',
            source_deleted: false,
          },
        ],
      }

      const app = createApp({
        render() {
          return h(NodeSourceHistoryPanel, {
            logicalId: 'node-ro-1',
            sourceHistory: historyData,
            loading: false,
          })
        },
      })
      app.mount(mountEl)
      await nextTick()

      // No input controls, checkboxes, selects, or mutation buttons
      expect(mountEl.querySelectorAll('input').length).toBe(0)
      expect(mountEl.querySelectorAll('select').length).toBe(0)
      expect(mountEl.querySelectorAll('textarea').length).toBe(0)
      expect(mountEl.querySelectorAll('.toggle, .checkbox').length).toBe(0)
      // Only buttons allowed are retry (when in error state), none present in normal loaded state
      expect(mountEl.querySelectorAll('button').length).toBe(0)

      app.unmount()
      mountEl.remove()
    })
  })

  describe('Deep linking to historic node via router query', () => {
    const scopeENodes: NodeRecord[] = Array.from({ length: 31 }, (_, i) => ({
      logical_id: `scope-e-node-${i + 1}`,
      protocol: 'vmess',
      display_name: `Active Node ${i + 1}`,
      active: true,
      server: `198.51.100.${i + 1}`,
      port: 8443,
      health_status: 'healthy' as const,
      sources: [{ node_logical_id: `scope-e-node-${i + 1}`, subscription_id: 'sub-active', last_seen_fetch_id: 'fetch-1' }],
    }))

    const historicOrphanNode: NodeRecord = {
      logical_id: 'historic-orphan-960',
      protocol: 'ss',
      display_name: 'Historic Orphan Node 960',
      active: false,
      server: '203.0.113.50',
      port: 8388,
      credentials: {
        password: 'secret-ss-password',
        method: 'aes-256-gcm',
      },
    }

    const orphanHistoryResponse: NodeSourceHistoryData = {
      attribution_status: 'historical_verified',
      current_sources: [],
      history: [
        {
          source_label: '7li-legacy',
          subscription_id: 'sub-legacy-7li',
          relation_state: 'verified',
          cause: 'legacy_import',
          evidence_kind: 'legacy_archive_link',
          first_observed_at: '2026-01-15T08:30:00Z',
          last_observed_at: '2026-09-29T10:00:00Z',
          connection_revision: 1,
          source_deleted: false,
        },
      ],
    }

    it('opens drawer for historic node outside Scope E without polluting items or inflating total count', async () => {
      let requestedAllAssets = false
      const apiGetCalls: string[] = []

      vi.spyOn(api, 'get').mockImplementation(async (path: string, options?: any) => {
        apiGetCalls.push(path)
        if (path.includes('all_assets') || path.includes('all-assets')) {
          requestedAllAssets = true
        }

        if (path === '/api/v1/nodes') {
          // Scope E query
          return { items: scopeENodes, total: 31, page: 1, page_size: 100 }
        }
        if (path === '/api/v1/probes/pool') {
          return { probing_node_ids: [], queued_node_ids: [] }
        }
        if (path === '/api/v1/nodes/historic-orphan-960') {
          return { node: historicOrphanNode }
        }
        if (path === '/api/v1/nodes/historic-orphan-960/source-history') {
          return orphanHistoryResponse
        }
        return {}
      })

      const router = createRouter({
        history: createMemoryHistory(),
        routes: [
          { path: '/nodes', component: NodesView },
        ],
      })

      // Push deep link with query parameter
      await router.push('/nodes?node=historic-orphan-960')

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const app = createApp({
        render() {
          return h(NodesView)
        },
      })
      app.use(router)
      app.mount(mountEl)

      await router.isReady()
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      // 1. Verify Scope E total remains strictly 31
      const totalBadge = mountEl.querySelector('[data-testid="nodes-total-badge"]')
      expect(totalBadge?.textContent).toContain('31')

      // 2. Verify all_assets was NEVER called
      expect(requestedAllAssets).toBe(false)

      // 3. Verify exact request was made to /api/v1/nodes/historic-orphan-960/source-history
      expect(apiGetCalls).toContain('/api/v1/nodes/historic-orphan-960')
      expect(apiGetCalls).toContain('/api/v1/nodes/historic-orphan-960/source-history')

      // 4. Verify drawer is open with the historic node's details
      const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')
      expect(drawer).not.toBeNull()
      expect(drawer?.textContent).toContain('Historic Orphan Node 960')

      // 5. Verify source history panel is rendered inside drawer with correct history
      const historyPanel = drawer?.querySelector('[data-testid="node-source-history-panel"]')
      expect(historyPanel).not.toBeNull()
      expect(historyPanel?.textContent).toContain('7li-legacy')
      expect(historyPanel?.textContent).toContain('历史系统导入')
      expect(historyPanel?.textContent).toContain('历史已核验归属')

      // 6. Test closing the drawer updates router query cleanly without infinite loop
      const closeBtn = document.body.querySelector('[data-testid="node-drawer-close-btn"]') as HTMLElement | null
      closeBtn?.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 40))

      expect(router.currentRoute.value.query.node).toBeUndefined()

      app.unmount()
      mountEl.remove()
    })

    it('displays 404 error cleanly on unknown deep-linked node ID without requesting all_assets', async () => {
      let requestedAllAssets = false

      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path.includes('all_assets')) {
          requestedAllAssets = true
        }
        if (path === '/api/v1/nodes') {
          return { items: scopeENodes, total: 31, page: 1, page_size: 100 }
        }
        if (path === '/api/v1/probes/pool') {
          return { probing_node_ids: [], queued_node_ids: [] }
        }
        if (path === '/api/v1/nodes/non-existent-999') {
          throw new ApiError(404, 'node_not_found', 'Node not found')
        }
        return {}
      })

      const router = createRouter({
        history: createMemoryHistory(),
        routes: [{ path: '/nodes', component: NodesView }],
      })

      await router.push('/nodes?node=non-existent-999')

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const app = createApp({
        render() {
          return h(NodesView)
        },
      })
      app.use(router)
      app.mount(mountEl)

      await router.isReady()
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      expect(requestedAllAssets).toBe(false)
      const errAlert = mountEl.querySelector('[data-testid="node-deep-link-error"]')
      expect(errAlert).not.toBeNull()
      expect(errAlert?.textContent).toContain('未找到节点「non-existent-999」(404)')

      // Total still untouched
      const totalBadge = mountEl.querySelector('[data-testid="nodes-total-badge"]')
      expect(totalBadge?.textContent).toContain('31')

      app.unmount()
      mountEl.remove()
    })

    it('switching query node triggers stable watch and loads new node detail and history', async () => {
      const nodeA: NodeRecord = {
        logical_id: 'node-switch-a',
        protocol: 'vmess',
        display_name: 'Node Switch A',
        active: false,
      }
      const nodeB: NodeRecord = {
        logical_id: 'node-switch-b',
        protocol: 'trojan',
        display_name: 'Node Switch B',
        active: false,
      }

      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path === '/api/v1/nodes') {
          return { items: scopeENodes, total: 31, page: 1, page_size: 100 }
        }
        if (path === '/api/v1/probes/pool') {
          return { probing_node_ids: [], queued_node_ids: [] }
        }
        if (path === '/api/v1/nodes/node-switch-a') {
          return { node: nodeA }
        }
        if (path === '/api/v1/nodes/node-switch-a/source-history') {
          return {
            attribution_status: 'historical_verified',
            current_sources: [],
            history: [
              {
                source_label: 'Source-A-Origin',
                relation_state: 'verified',
                cause: 'legacy_import',
                evidence_kind: 'archive',
                source_deleted: false,
              },
            ],
          }
        }
        if (path === '/api/v1/nodes/node-switch-b') {
          return { node: nodeB }
        }
        if (path === '/api/v1/nodes/node-switch-b/source-history') {
          return {
            attribution_status: 'manual_confirmed',
            current_sources: [],
            history: [
              {
                source_label: 'Source-B-Manual',
                relation_state: 'verified',
                cause: 'manual_confirmed',
                evidence_kind: 'manual_audit',
                source_deleted: false,
              },
            ],
          }
        }
        return {}
      })

      const router = createRouter({
        history: createMemoryHistory(),
        routes: [{ path: '/nodes', component: NodesView }],
      })

      await router.push('/nodes?node=node-switch-a')

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const app = createApp({
        render() {
          return h(NodesView)
        },
      })
      app.use(router)
      app.mount(mountEl)

      await router.isReady()
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')
      expect(drawer?.textContent).toContain('Node Switch A')
      expect(drawer?.textContent).toContain('Source-A-Origin')

      // Now switch query to node-switch-b
      await router.push('/nodes?node=node-switch-b')
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      expect(drawer?.textContent).toContain('Node Switch B')
      expect(drawer?.textContent).toContain('Source-B-Manual')
      expect(drawer?.textContent).toContain('人工已核验归属')

      app.unmount()
      mountEl.remove()
    })

    it('safely handles 401 unauthorized errors without leaking auth secrets or bearer tokens', async () => {
      const nodeSecret: NodeRecord = {
        logical_id: 'node-secret-leak-test',
        protocol: 'ss',
        display_name: 'Auth Secret Node',
        active: false,
      }

      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path === '/api/v1/nodes') {
          return { items: scopeENodes, total: 31, page: 1, page_size: 100 }
        }
        if (path === '/api/v1/probes/pool') {
          return { probing_node_ids: [], queued_node_ids: [] }
        }
        if (path === '/api/v1/nodes/node-secret-leak-test') {
          return { node: nodeSecret }
        }
        if (path === '/api/v1/nodes/node-secret-leak-test/source-history') {
          throw new ApiError(401, 'unauthorized', 'Sensitive Bearer Token: secret-token-xyz-12345')
        }
        return {}
      })

      const router = createRouter({
        history: createMemoryHistory(),
        routes: [{ path: '/nodes', component: NodesView }],
      })

      await router.push('/nodes?node=node-secret-leak-test')

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)

      const app = createApp({
        render() {
          return h(NodesView)
        },
      })
      app.use(router)
      app.mount(mountEl)

      await router.isReady()
      await nextTick()
      await new Promise((r) => setTimeout(r, 60))

      const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')
      expect(drawer).not.toBeNull()
      const errEl = drawer?.querySelector('[data-testid="source-history-error"]')
      expect(errEl).not.toBeNull()
      // Verifies friendly Chinese message and strict absence of leaked tokens
      expect(errEl?.textContent).toContain('鉴权失败，无法获取来源历史')
      expect(drawer?.textContent).not.toContain('secret-token-xyz-12345')

      app.unmount()
      mountEl.remove()
    })

    it('verifies that manual confirmation requires explicit backend signal and never fabricates manual for unverified nodes', () => {
      // 1. Without explicit manual_confirmed signal:
      const nonManualData: NodeSourceHistoryData = {
        attribution_status: 'unknown',
        current_sources: [],
        history: [
          {
            source_label: 'Unknown-Origin',
            relation_state: 'unknown',
            cause: 'unresolved',
            evidence_kind: 'none',
            source_deleted: false,
          },
        ],
      }
      expect(attributionStatusBadge(nonManualData.attribution_status).label).not.toContain('人工')
      expect(attributionCauseLabel(nonManualData.history[0].cause)).not.toContain('人工')

      // 2. With explicit manual_confirmed signal:
      const explicitManualData: NodeSourceHistoryData = {
        attribution_status: 'manual_confirmed',
        current_sources: [],
        history: [
          {
            source_label: 'Audited-Source',
            relation_state: 'verified',
            cause: 'manual_confirmed',
            evidence_kind: 'admin_audit_log',
            source_deleted: false,
          },
        ],
      }
      expect(attributionStatusBadge(explicitManualData.attribution_status).label).toBe('人工已核验归属')
      expect(attributionCauseLabel(explicitManualData.history[0].cause)).toBe('人工核验确认')
    })
  })
})

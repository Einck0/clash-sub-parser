// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import {
  formatPlatformBadge,
  formatSpeed,
  computeGroupVerdict,
  normalizePlatforms,
  type PlatformCapability,
  type ProbeObservation,
} from './probeTypes'
import {
  normalizeNode,
  getNodePlatformBadges,
  getNodeSpeedBadge,
  nodeRiskBadge,
  type NodeRecord,
} from '../nodes/nodeView'
import ProbeEvidenceSheet from './ProbeEvidenceSheet.vue'
import NodesView from '../nodes/NodesView.vue'
import ProbesView from './ProbesView.vue'
import { api } from '../../api/client'

export const subcheckFixtureNodes: Record<string, NodeRecord> = {
  aiFull: {
    logical_id: 'node-ai-full',
    protocol: 'vmess',
    display_name: 'US AI Full Dual',
    active: true,
    capabilities: {
      ai: {
        verdict: 'available',
        platforms: {
          openai: { verdict: 'available', sub_tier: 'full', region: 'US' },
          claude: { verdict: 'available', region: 'US' },
        },
      },
    },
  },
  aiWeb: {
    logical_id: 'node-ai-web',
    protocol: 'trojan',
    display_name: 'US AI Web Only',
    active: true,
    capabilities: {
      ai: {
        verdict: 'restricted',
        platforms: {
          openai: { verdict: 'restricted', sub_tier: 'web', region: 'US' },
          claude: { verdict: 'restricted', region: 'HK' },
        },
      },
    },
  },
  aiApp: {
    logical_id: 'node-ai-app',
    protocol: 'ss',
    display_name: 'US AI App Only',
    active: true,
    capabilities: {
      ai: {
        verdict: 'restricted',
        platforms: {
          openai: { verdict: 'restricted', sub_tier: 'app', region: 'US' },
          claude: { verdict: 'error' },
        },
      },
    },
  },
  streamingFull: {
    logical_id: 'node-streaming-full',
    protocol: 'hysteria2',
    display_name: 'HK Streaming Full',
    active: true,
    capabilities: {
      streaming: {
        verdict: 'available',
        platforms: {
          netflix: { verdict: 'available', sub_tier: 'full', region: 'HK' },
          disney: { verdict: 'available', sub_tier: 'unlocked', region: 'HK' },
          youtube: { verdict: 'available', region: 'SG' },
        },
      },
    },
  },
  streamingOriginalsSoon: {
    logical_id: 'node-streaming-originals-soon',
    protocol: 'vless',
    display_name: 'HK Originals & Soon',
    active: true,
    capabilities: {
      streaming: {
        verdict: 'restricted',
        platforms: {
          netflix: { verdict: 'restricted', sub_tier: 'originals', region: 'HK' },
          disney: { verdict: 'restricted', sub_tier: 'soon', region: 'HK' },
          youtube: { verdict: 'restricted', region: 'CN' },
        },
      },
    },
  },
  streamingBanned: {
    logical_id: 'node-streaming-banned',
    protocol: 'ss',
    display_name: 'HK Streaming Banned',
    active: true,
    capabilities: {
      streaming: {
        verdict: 'error',
        platforms: {
          netflix: { verdict: 'error', sub_tier: 'banned', region: 'HK' },
          disney: { verdict: 'error', sub_tier: 'banned', region: 'HK' },
          youtube: { verdict: 'error' },
        },
      },
    },
  },
  missingRegion: {
    logical_id: 'node-missing-region',
    protocol: 'wireguard',
    display_name: 'Global Node Missing Region',
    active: true,
    capabilities: {
      ai: {
        verdict: 'available',
        platforms: {
          openai: { verdict: 'available', sub_tier: 'full', region: '' },
          claude: { verdict: 'available', region: 'unknown' },
        },
      },
      streaming: {
        verdict: 'available',
        platforms: {
          netflix: { verdict: 'available', sub_tier: 'full' },
          disney: { verdict: 'available', sub_tier: 'unlocked' },
        },
      },
    },
  },
  unknownErrorStale: {
    logical_id: 'node-unknown-error-stale',
    protocol: 'tuic',
    display_name: 'Stale & Error Node',
    active: true,
    capabilities: {
      ai: {
        verdict: 'stale',
        platforms: {
          openai: { verdict: 'stale', sub_tier: 'full', region: 'US' },
          claude: { verdict: 'error', reason: 'timeout' },
        },
      },
      streaming: {
        verdict: 'unknown',
        platforms: {
          netflix: { verdict: 'unknown' },
          disney: { verdict: 'error', sub_tier: 'banned' },
        },
      },
    },
  },
  speedKb: {
    logical_id: 'node-speed-kb',
    protocol: 'ss',
    display_name: 'Slow Node KB',
    active: true,
    capabilities: {
      speed: {
        verdict: 'available',
        throughput: 512,
        latency_ms: 120,
      },
    },
  },
  speedMb: {
    logical_id: 'node-speed-mb',
    protocol: 'hysteria2',
    display_name: 'Fast Node MB',
    active: true,
    capabilities: {
      speed: {
        verdict: 'available',
        throughput: 5200,
        latency_ms: 25,
      },
    },
  },
  ipRiskClean: {
    logical_id: 'node-ip-risk-clean',
    protocol: 'trojan',
    display_name: 'Clean IP Node',
    active: true,
    capabilities: {
      ip_risk: {
        verdict: 'available',
        risk_score: '0',
      },
    },
  },
  ipRiskMedium: {
    logical_id: 'node-ip-risk-medium',
    protocol: 'trojan',
    display_name: 'Medium Risk IP Node',
    active: true,
    capabilities: {
      ip_risk: {
        verdict: 'restricted',
        risk_score: '35',
      },
    },
  },
}

describe('Subcheck Platform Capability Matrix & Fixture Tests', () => {
  describe('Speed & Bandwidth formatting', () => {
    it('formats throughput cleanly with reasonable KB/s and MB/s units', () => {
      expect(formatSpeed(null)).toBe('--')
      expect(formatSpeed(undefined)).toBe('--')
      expect(formatSpeed(-1)).toBe('--')
      expect(formatSpeed(0)).toBe('0 KB/s')
      expect(formatSpeed(512)).toBe('512 KB/s')
      expect(formatSpeed(1023)).toBe('1023 KB/s')
      expect(formatSpeed(1024)).toBe('1.0 MB/s')
      expect(formatSpeed(5200)).toBe('5.1 MB/s')
      expect(formatSpeed(10485)).toBe('10.2 MB/s')
    })
  })

  describe('Group Verdict Computation', () => {
    it('returns available if any platform is available', () => {
      expect(
        computeGroupVerdict({
          netflix: { verdict: 'restricted', sub_tier: 'originals' },
          disney: { verdict: 'available', sub_tier: 'unlocked' },
        })
      ).toBe('available')
    })

    it('returns restricted if all platforms are explicitly restricted', () => {
      expect(
        computeGroupVerdict({
          netflix: { verdict: 'restricted', sub_tier: 'originals' },
          disney: { verdict: 'restricted', sub_tier: 'soon' },
        })
      ).toBe('restricted')
    })

    it('returns error if all platforms are error', () => {
      expect(
        computeGroupVerdict({
          netflix: { verdict: 'error', sub_tier: 'banned' },
          disney: { verdict: 'error' },
        })
      ).toBe('error')
    })

    it('returns unknown for mixed or empty states', () => {
      expect(computeGroupVerdict({})).toBe('unknown')
      expect(computeGroupVerdict(undefined)).toBe('unknown')
      expect(
        computeGroupVerdict({
          netflix: { verdict: 'unknown' },
          disney: { verdict: 'restricted' },
        })
      ).toBe('unknown')
    })
  })

  describe('Platform Badge Formatting & Tones', () => {
    it('formats OpenAI sub-tiers accurately', () => {
      // Full: GPT⁺ web + app dual pass
      const full = formatPlatformBadge('openai', {
        verdict: 'available',
        sub_tier: 'full',
        region: 'US',
      })
      expect(full.label).toBe('GPT⁺ (US)')
      expect(full.tone).toBe('success')

      // Web: GPT web pass only (not green!)
      const web = formatPlatformBadge('openai', {
        verdict: 'restricted',
        sub_tier: 'web',
        region: 'US',
      })
      expect(web.label).toBe('GPT (US)')
      expect(web.tone).toBe('warning')
      expect(web.tone).not.toBe('success')

      // App: GPT app pass only (not green!)
      const app = formatPlatformBadge('openai', {
        verdict: 'restricted',
        sub_tier: 'app',
        region: 'US',
      })
      expect(app.label).toBe('GPT App (US)')
      expect(app.tone).toBe('warning')
      expect(app.tone).not.toBe('success')

      // Banned: error
      const banned = formatPlatformBadge('openai', {
        verdict: 'error',
        sub_tier: 'banned',
        region: 'US',
      })
      expect(banned.label).toContain('GPT 封禁')
      expect(banned.tone).toBe('error')

      // Missing region: no dummy value
      const noReg = formatPlatformBadge('openai', {
        verdict: 'available',
        sub_tier: 'full',
        region: '',
      })
      expect(noReg.label).toBe('GPT⁺')
      expect(noReg.label).not.toContain('?')
      expect(noReg.label).not.toContain('unknown')
    })

    it('formats Netflix sub-tiers accurately', () => {
      // Full
      const full = formatPlatformBadge('netflix', {
        verdict: 'available',
        sub_tier: 'full',
        region: 'HK',
      })
      expect(full.label).toBe('NF (HK)')
      expect(full.tone).toBe('success')

      // Originals only: warning (not green!)
      const originals = formatPlatformBadge('netflix', {
        verdict: 'restricted',
        sub_tier: 'originals',
        region: 'HK',
      })
      expect(originals.label).toBe('NF 仅自制 (HK)')
      expect(originals.tone).toBe('warning')
      expect(originals.tone).not.toBe('success')

      // Banned: error
      const banned = formatPlatformBadge('netflix', {
        verdict: 'error',
        sub_tier: 'banned',
        region: 'HK',
      })
      expect(banned.label).toContain('NF 封禁')
      expect(banned.tone).toBe('error')
    })

    it('formats Disney+ sub-tiers accurately, ensuring soon is not green', () => {
      // Unlocked
      const unlocked = formatPlatformBadge('disney', {
        verdict: 'available',
        sub_tier: 'unlocked',
        region: 'HK',
      })
      expect(unlocked.label).toBe('D+ (HK)')
      expect(unlocked.tone).toBe('success')

      // Soon: warning (NOT GREEN!)
      const soon = formatPlatformBadge('disney', {
        verdict: 'restricted',
        sub_tier: 'soon',
        region: 'HK',
      })
      expect(soon.label).toBe('D+ 尚未开放 (HK)')
      expect(soon.tone).toBe('warning')
      expect(soon.tone).not.toBe('success')

      // Banned: error
      const banned = formatPlatformBadge('disney', {
        verdict: 'error',
        sub_tier: 'banned',
        region: 'HK',
      })
      expect(banned.label).toContain('D+ 封禁')
      expect(banned.tone).toBe('error')
    })

    it('formats Claude and YouTube by real region and verdict', () => {
      const ytAvailable = formatPlatformBadge('youtube', {
        verdict: 'available',
        region: 'SG',
      })
      expect(ytAvailable.label).toBe('YT (SG)')
      expect(ytAvailable.tone).toBe('success')

      const ytRestricted = formatPlatformBadge('youtube', {
        verdict: 'restricted',
        region: 'CN',
      })
      expect(ytRestricted.label).toBe('YT 受限 (CN)')
      expect(ytRestricted.tone).toBe('warning')

      const claudeAvailable = formatPlatformBadge('claude', {
        verdict: 'available',
        region: 'US',
      })
      expect(claudeAvailable.label).toBe('Claude (US)')
      expect(claudeAvailable.tone).toBe('success')

      const claudeRestricted = formatPlatformBadge('claude', {
        verdict: 'restricted',
        region: 'HK',
      })
      expect(claudeRestricted.label).toBe('Claude 受限 (HK)')
      expect(claudeRestricted.tone).toBe('warning')
    })

    it('ensures unknown, error, and stale are never green', () => {
      const unk = formatPlatformBadge('openai', { verdict: 'unknown' })
      expect(unk.tone).not.toBe('success')

      const err = formatPlatformBadge('netflix', { verdict: 'error' })
      expect(err.tone).not.toBe('success')

      const stale = formatPlatformBadge('openai', {
        verdict: 'stale',
        sub_tier: 'full',
        region: 'US',
      })
      expect(stale.tone).not.toBe('success')
      expect(stale.label).toContain('已过期')
    })

    it('never displays dummy region values for missing or unknown regions', () => {
      const missing = formatPlatformBadge('openai', {
        verdict: 'available',
        sub_tier: 'full',
        region: 'unknown',
      })
      expect(missing.label).toBe('GPT⁺')
      expect(missing.label).not.toContain('unknown')

      const ws = formatPlatformBadge('netflix', {
        verdict: 'available',
        sub_tier: 'full',
        region: '   ',
      })
      expect(ws.label).toBe('NF')
    })
  })

  describe('Node normalization with subcheck platforms dictionary', () => {
    it('extracts platform badges, throughput, and risk scores into NormalizedNode', () => {
      const normAiFull = normalizeNode(subcheckFixtureNodes.aiFull)
      const aiBadges = getNodePlatformBadges(normAiFull, 'ai')
      expect(aiBadges.length).toBe(2)
      expect(aiBadges[0].label).toBe('GPT⁺ (US)')
      expect(aiBadges[0].tone).toBe('success')
      expect(aiBadges[1].label).toBe('Claude (US)')

      const normStreamingFull = normalizeNode(subcheckFixtureNodes.streamingFull)
      const streamBadges = getNodePlatformBadges(normStreamingFull, 'streaming')
      expect(streamBadges.length).toBe(3)
      expect(streamBadges[0].label).toBe('NF (HK)')
      expect(streamBadges[1].label).toBe('YT (SG)')
      expect(streamBadges[2].label).toBe('D+ (HK)')

      const normSpeed = normalizeNode(subcheckFixtureNodes.speedMb)
      const speedBadge = getNodeSpeedBadge(normSpeed)
      expect(speedBadge).not.toBeNull()
      expect(speedBadge?.label).toBe('🚀 5.1 MB/s')

      const normCleanRisk = normalizeNode(subcheckFixtureNodes.ipRiskClean)
      expect(nodeRiskBadge(normCleanRisk)).toEqual({ label: '低风险 (0)', tone: 'success' })

      const normMedRisk = normalizeNode(subcheckFixtureNodes.ipRiskMedium)
      expect(nodeRiskBadge(normMedRisk)).toEqual({ label: '中风险 (35)', tone: 'warning' })
    })
  })

  describe('Component Rendering with Subcheck Fixtures', () => {
    beforeEach(() => {
      vi.restoreAllMocks()
    })

    it('renders platform badges in NodesView card and inspect drawer', async () => {
      const mockNodes = [
        subcheckFixtureNodes.aiFull,
        subcheckFixtureNodes.streamingOriginalsSoon,
        subcheckFixtureNodes.speedMb,
      ]

      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path.includes('/api/v1/nodes/node-ai-full')) {
          return subcheckFixtureNodes.aiFull
        }
        if (path.includes('/api/v1/nodes')) {
          return { items: mockNodes, total: mockNodes.length, page: 1, page_size: 50 }
        }
        if (path.includes('/api/v1/subscriptions')) {
          return { items: [], total: 0 }
        }
        if (path.includes('/api/v1/probes/pool/status')) {
          return { queue_nodes_count: 0, probing_count: 0, probing_node_ids: [], queued_node_ids: [] }
        }
        return {}
      })

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({
        render: () => h(NodesView),
      })
      app.mount(mountEl)
      await nextTick()
      await new Promise((r) => setTimeout(r, 40))

      const text = mountEl.textContent || ''
      expect(text).toContain('GPT⁺ (US)')
      expect(text).toContain('NF 仅自制 (HK)')
      expect(text).toContain('D+ 尚未开放 (HK)')
      expect(text).toContain('5.1 MB/s')

      // Click first card to inspect
      const card = mountEl.querySelector('[data-testid="node-card"]') as HTMLElement
      expect(card).not.toBeNull()
      card.click()
      await nextTick()
      await new Promise((r) => setTimeout(r, 40))

      const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')
      expect(drawer).not.toBeNull()
      expect(drawer?.textContent).toContain('GPT⁺ (US)')
      expect(drawer?.textContent).toContain('Claude (US)')

      app.unmount()
      mountEl.remove()
    })

    it('renders platform badges in ProbesView workbench table', async () => {
      const mockNodes = [
        subcheckFixtureNodes.streamingFull,
        subcheckFixtureNodes.aiWeb,
      ]

      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path.includes('/api/v1/probes/pool/status')) {
          return { queue_nodes_count: 0, probing_count: 0, probing_node_ids: [], queued_node_ids: [] }
        }
        if (path.includes('/api/v1/probes/runs')) {
          return { items: [], total: 0 }
        }
        if (path.includes('/api/v1/probes/schedule')) {
          return { enabled: false, interval_seconds: 3600, kinds: ['baseline'] }
        }
        if (path.includes('/api/v1/probes/batches')) {
          return { items: [], total: 0 }
        }
        if (path.includes('/api/v1/nodes')) {
          return { items: mockNodes, total: mockNodes.length, page: 1, page_size: 50 }
        }
        if (path.includes('/api/v1/subscriptions')) {
          return { items: [], total: 0 }
        }
        return {}
      })

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({
        render: () => h(ProbesView),
      })
      app.mount(mountEl)
      await nextTick()
      await new Promise((r) => setTimeout(r, 40))

      const text = mountEl.textContent || ''
      expect(text).toContain('NF (HK)')
      expect(text).toContain('YT (SG)')
      expect(text).toContain('D+ (HK)')
      expect(text).toContain('GPT (US)')

      app.unmount()
      mountEl.remove()
    })

    it('renders detailed subcheck platform breakdown in ProbeEvidenceSheet', async () => {
      const obsWithPlatforms: ProbeObservation = {
        id: 'obs-ai-1',
        probe_run_id: 'run-1',
        node_logical_id: 'node-ai-full',
        kind: 'ai',
        verdict: 'available',
        evidence_digest: 'sha256-test',
        observed_at: '2026-10-02T12:00:00Z',
        latency_ms: 150,
        redacted_summary: '',
        platforms: {
          openai: {
            verdict: 'available',
            sub_tier: 'full',
            region: 'US',
            latency_ms: 120,
            summary: 'OpenAI 网页端与移动端网关均可正常通行',
          },
          claude: {
            verdict: 'available',
            region: 'US',
            latency_ms: 160,
            summary: 'Claude Cloudflare Trace 校验通过',
          },
        },
      }

      const obsWithSpeed: ProbeObservation = {
        id: 'obs-speed-1',
        probe_run_id: 'run-1',
        node_logical_id: 'node-ai-full',
        kind: 'speed',
        verdict: 'available',
        evidence_digest: 'sha256-speed',
        observed_at: '2026-10-02T12:01:00Z',
        latency_ms: 45,
        redacted_summary: '',
        throughput: 8192, // 8.0 MB/s
      }

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({
        render: () =>
          h(ProbeEvidenceSheet, {
            open: true,
            observations: [obsWithPlatforms, obsWithSpeed],
            loading: false,
            node: normalizeNode(subcheckFixtureNodes.aiFull),
          }),
      })
      app.mount(mountEl)
      await nextTick()

      const text = mountEl.textContent || ''
      expect(text).toContain('GPT⁺ (US)')
      expect(text).toContain('Claude (US)')
      expect(text).toContain('8.0 MB/s')

      app.unmount()
      mountEl.remove()
    })
  })
})

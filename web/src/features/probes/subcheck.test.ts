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
import { api } from '../../api/client'
import { getLocale, setLocale } from '../../locales'

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
        verdict: 'available',
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

    it('returns skipped for empty states, abnormal if any abnormal, and follows domain precedence', () => {
      expect(computeGroupVerdict({})).toBe('skipped')
      expect(computeGroupVerdict(undefined)).toBe('skipped')
      expect(
        computeGroupVerdict({
          netflix: { verdict: 'skipped' },
          disney: { verdict: 'skipped' },
        })
      ).toBe('skipped')
      expect(
        computeGroupVerdict({
          netflix: { verdict: 'abnormal' },
          disney: { verdict: 'available' },
        })
      ).toBe('abnormal')
      expect(
        computeGroupVerdict({
          netflix: { verdict: 'unknown' },
          disney: { verdict: 'restricted' },
        })
      ).toBe('unknown')
    })
  })

  describe('Platform Badge Formatting & Tones', () => {
    it('normalizes optional anonymous preflight checks without promoting missing/invalid checks', () => {
      const normalized = normalizePlatforms({
        OPENAI: {
          verdict: 'available',
          evidence_scope: 'anonymous_preflight',
          checks: {
            cookie_requirements: { verdict: 'available', status_code: 200, evidence_scope: 'anonymous_preflight' },
            chatgpt_web: { verdict: 'nonsense', statusCode: 403, reason: 'challenge' },
            ignored: null,
          },
        },
      })
      expect(normalized?.openai.checks?.cookie_requirements).toEqual({
        verdict: 'available', reason: undefined, status_code: 200, evidence_scope: 'anonymous_preflight',
      })
      expect(normalized?.openai.checks?.chatgpt_web).toEqual({
        verdict: 'unknown', reason: 'challenge', status_code: 403, evidence_scope: undefined,
      })
      expect(normalized?.openai.checks?.ignored).toBeUndefined()
    })
    it('keeps OpenAI endpoint evidence separate from unverified capability and legacy tiers', () => {
      const current = formatPlatformBadge('openai', {
        verdict: 'unknown',
        reason: 'endpoint_reachable_capability_unverified',
        evidence_scope: 'network_entry',
      })
      expect(current.label).toContain('入口响应')
      expect(current.tone).not.toBe('success')
      expect(current.tooltip).toContain('Codex 能力及 App 能力未核验')
      expect(current.tooltip).toContain('API 鉴权、API 调用和模型推理未检测')

      const legacy = formatPlatformBadge('openai', {
        verdict: 'available',
        sub_tier: 'full',
        region: 'US',
      })
      expect(legacy.label).toContain('旧版探测结果，需重新检测')
      expect(legacy.label).not.toContain('GPT⁺')
      expect(legacy.tone).not.toBe('success')
      expect(legacy.tooltip).toContain('历史记录')

      const rateLimited = formatPlatformBadge('openai', {
        verdict: 'restricted',
        reason: 'rate_limited',
        sub_tier: 'banned',
      })
      expect(rateLimited.label).toBe('OpenAI 入口限流')
      expect(rateLimited.label).not.toContain('封禁')

      const staleLegacy = formatPlatformBadge('openai', {
        verdict: 'stale',
        sub_tier: 'full',
      })
      expect(staleLegacy.label).toBe('OpenAI 结果过期 · 已过期')
      expect(staleLegacy.tone).toBe('warning')
    })

    it('shows success only for anonymous preflight and preserves per-check evidence', () => {
      const current = formatPlatformBadge('openai', {
        verdict: 'available',
        evidence_scope: 'anonymous_preflight',
        sub_tier: 'full',
        checks: {
          cookie_requirements: {
            verdict: 'available',
            status_code: 200,
            evidence_scope: 'anonymous_preflight',
          },
          chatgpt_web: {
            verdict: 'available',
            reason: 'public_web_entry_matched',
            status_code: 200,
            evidence_scope: 'anonymous_preflight',
          },
        },
      })
      expect(current.label).toBe('OpenAI 预检通过')
      expect(current.tone).toBe('success')
      expect(current.tooltip).toContain('账号登录、付费模型、Codex 能力及 App 能力未核验')
      expect(current.tooltip).toContain('API 鉴权、API 调用和模型推理未检测')
      expect(current.tooltip).toContain('合规入口: 已确认')
      expect(current.tooltip).toContain('网页入口: 已确认 · HTTP 200')

      for (const verdict of ['unknown', 'error', 'abnormal', 'restricted', 'stale'] as const) {
        const badge = formatPlatformBadge('openai', { verdict, sub_tier: 'full' })
        expect(badge.tone).not.toBe('success')
        expect(badge.label).not.toContain('GPT⁺')
      }
      expect(formatPlatformBadge('openai', { verdict: 'available', sub_tier: 'full' }).label)
        .toBe('旧版探测结果，需重新检测')
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

    it('shows Antigravity entry reachability only for the complete positive evidence tuple', () => {
      const reachable = formatPlatformBadge('antigravity', {
        verdict: 'available',
        sub_tier: 'reachable',
        evidence_scope: 'network_entry',
        reason: 'antigravity_entry_reachable',
        region: 'US',
        exit_region: 'JP',
        region_source: 'cdn_trace',
      })
      expect(reachable.label).toBe('Antigravity 入口可达 (US)')
      expect(reachable.tone).toBe('success')
      expect(reachable.tooltip).toContain('官网入口可达')
      expect(reachable.tooltip).toContain('个人账号所在国家/地区、年龄(18+)及模型资格未核验')
      expect(reachable.tooltip).toContain('服务观测地区: US')
      expect(reachable.tooltip).toContain('出口参考地区: JP')
      expect(reachable.tooltip).toContain('地区存在差异，非物理路由绝对证明')
      expect(reachable.tooltip).toContain('地区来源: CDN 追踪 (cdn_trace)')
      expect(reachable.tooltip).toContain('观测范围: 网站入口连通性 (network_entry)')

      const abnormal = formatPlatformBadge('antigravity', { verdict: 'abnormal' })
      expect(abnormal.label).toBe('Antigravity 异常')
      expect(abnormal.tone).toBe('warning')

      const skipped = formatPlatformBadge('antigravity', { verdict: 'skipped' })
      expect(skipped.label).toBe('Antigravity 未执行')
      expect(skipped.tone).toBe('info')

      const failed = formatPlatformBadge('antigravity', { verdict: 'error' })
      expect(failed.label).toBe('Antigravity 失败')
      expect(failed.tone).toBe('error')

      const incompletePositiveEvidence: PlatformCapability[] = [
        { verdict: 'available', sub_tier: 'reachable' },
        { verdict: 'available', evidence_scope: 'network_entry' },
        {
          verdict: 'available',
          sub_tier: 'reachable',
          evidence_scope: 'network_entry',
        },
      ]
      for (const capability of incompletePositiveEvidence) {
        const badge = formatPlatformBadge('antigravity', capability)
        expect(badge.tooltip).toContain('检测范围：官网入口连通性')
        expect(badge.tooltip).not.toContain('官网入口可达')
        expect(badge.tooltip).toContain('未核验')
      }

      const nonpositive = [
        { verdict: 'unknown', reason: 'antigravity_status_unknown' },
        { verdict: 'error', reason: 'antigravity_probe_error' },
        { verdict: 'restricted', reason: 'antigravity_entry_restricted' },
        { verdict: 'skipped', reason: 'antigravity_probe_skipped' },
      ] as const
      for (const { verdict, reason } of nonpositive) {
        const badge = formatPlatformBadge('antigravity', {
          verdict,
          reason,
          summary: `历史摘要：${reason}`,
        })
        expect(badge.tooltip).toContain('检测范围：官网入口连通性')
        expect(badge.tooltip).toContain(reason)
        expect(badge.tooltip).toContain(`历史摘要：${reason}`)
        expect(badge.tooltip).toContain('未核验')
        expect(badge.tooltip).not.toContain('官网入口可达')
      }

      const historical = formatPlatformBadge('antigravity', {
        verdict: 'unknown',
        reason: 'antigravity_entry_reachable',
        summary: '历史记录：官网入口可达；模型资格未核验',
      })
      expect(historical.tooltip).toContain('官网入口可达（个人账号及模型资格未核验）')
      expect(historical.tooltip).toContain('历史记录：官网入口可达；模型资格未核验')
      expect(historical.tooltip).toContain('检测范围：官网入口连通性')
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
      expect(stale.label).toBe('OpenAI 结果过期 · 已过期')
    })

    it('never displays dummy region values for missing or unknown regions', () => {
      const missing = formatPlatformBadge('openai', {
        verdict: 'available',
        sub_tier: 'full',
        region: 'unknown',
      })
      expect(missing.label).toBe('旧版探测结果，需重新检测')
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
    it('keeps a restricted risk check unknown even when it carries a numeric score', () => {
      const node: NodeRecord = {
        ...subcheckFixtureNodes.ipRiskMedium,
        capabilities: { ip_risk: { verdict: 'restricted', risk_score: '35' } },
      }
      expect(nodeRiskBadge(node)).toEqual({ label: '风险未知', tone: 'info' })
      expect(nodeRiskBadge(normalizeNode(node))).toEqual({ label: '风险未知', tone: 'info' })
    })

    it('extracts platform badges, throughput, and risk scores into NormalizedNode', () => {
      const normAiFull = normalizeNode(subcheckFixtureNodes.aiFull)
      const aiBadges = getNodePlatformBadges(normAiFull, 'ai')
      expect(aiBadges.length).toBe(2)
      expect(aiBadges[0].label).toBe('旧版探测结果，需重新检测')
      expect(aiBadges[0].tone).not.toBe('success')
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

      const get = vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path === '/api/v1/nodes/node-ai-full') {
          return subcheckFixtureNodes.aiFull
        }
        if (path === '/api/v1/nodes/node-ai-full/source-history') {
          return { attribution_status: 'unknown', current_sources: [], history: [] }
        }
        if (path === '/api/v1/nodes') {
          return { items: mockNodes, total: mockNodes.length, page: 1, page_size: 50 }
        }
        if (path === '/api/v1/probes/pool') {
          return { probing_node_ids: [], queued_node_ids: [] }
        }
        return {}
      })

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({
        render: () => h(NodesView),
      })
      try {
        app.mount(mountEl)
        await nextTick()
        await new Promise((r) => setTimeout(r, 40))

        const cards = mountEl.querySelectorAll<HTMLElement>('[data-testid="node-card"]')
        expect(cards).toHaveLength(3)
        for (const card of [cards[0]!, cards[1]!]) {
          expect(card.querySelector('[data-testid="node-platform-details-expanded"]')).toBeNull()
          const toggle = card.querySelector<HTMLButtonElement>('[data-testid="node-platform-details-toggle"]')!
          expect(toggle.getAttribute('aria-expanded')).toBe('false')
          toggle.click()
        }
        await nextTick()
        const aiDetails = cards[0]!.querySelector<HTMLElement>('[data-testid="node-platform-details-expanded"]')!
        const streamingDetails = cards[1]!.querySelector<HTMLElement>('[data-testid="node-platform-details-expanded"]')!
        expect(cards[0]!.querySelector('[data-testid="node-platform-details-toggle"]')?.getAttribute('aria-expanded')).toBe('true')
        expect(aiDetails.textContent).toContain('旧版探测结果，需重新检测')
        expect(aiDetails.textContent).toContain('CLAUDE (US)')
        const legacyBadge = Array.from(aiDetails.querySelectorAll<HTMLElement>('[title]'))
          .find((element) => element.textContent?.includes('旧版探测结果，需重新检测'))
        expect(legacyBadge?.classList.contains('badge-success')).toBe(false)
        expect(legacyBadge?.title).toContain('匿名公开入口预检')
        expect(streamingDetails.textContent).toContain('NETFLIX 仅自制 (HK)')
        expect(streamingDetails.textContent).toContain('D+ 尚未开放 (HK)')
        expect(cards[2]!.querySelector('[data-testid="node-speed-result"]')?.textContent).toContain('5.1 MB/s')

        // The explicit inspect button, not the non-interactive card surface, opens details.
        cards[0]!.querySelector<HTMLButtonElement>('[data-testid="node-inspect-btn"]')!.click()
        await vi.waitFor(() => {
          expect(document.body.querySelector('[data-testid="node-detail-drawer"]')?.textContent).toContain('旧版探测结果，需重新检测')
        })
        expect(get).toHaveBeenCalledWith('/api/v1/nodes/node-ai-full')
        const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')!
        expect(drawer.textContent).toContain('旧版探测结果，需重新检测')
        expect(drawer.textContent).toContain('Claude (US)')

        cards[0]!.querySelector<HTMLButtonElement>('[data-testid="node-platform-details-toggle"]')!.click()
        await nextTick()
        expect(cards[0]!.querySelector('[data-testid="node-platform-details-expanded"]')).toBeNull()
        expect(cards[0]!.querySelector('[data-testid="node-platform-details-toggle"]')?.getAttribute('aria-expanded')).toBe('false')
      } finally {
        app.unmount()
        mountEl.remove()
      }
    })

    it('qualifies non-current platform evidence in list expansion and both drawer locations', async () => {
      const claude: PlatformCapability = {
        verdict: 'available', sub_tier: 'unlocked', region: 'US', exit_region: 'US',
        reason: 'claude_available', region_source: 'cdn_trace', evidence_scope: 'cdn_trace',
        summary: 'CDN trace region (US); account/chat eligibility unverified',
      }
      const node: NodeRecord = {
        ...subcheckFixtureNodes.aiFull,
        health_status: 'unknown',
        capabilities: {
          ai: { verdict: 'available', stale: true, platforms: { claude } },
          streaming: { verdict: 'available', stale: false, platforms: { netflix: { verdict: 'available', region: 'US' } } },
        },
      }
      const original = JSON.stringify(node)
      const locale = getLocale()
      setLocale('zh-CN')
      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path === '/api/v1/nodes/node-ai-full') return node
        if (path === '/api/v1/nodes/node-ai-full/source-history') return { attribution_status: 'unknown', current_sources: [], history: [] }
        if (path === '/api/v1/nodes') return { items: [node], total: 1, page: 1, page_size: 50 }
        if (path === '/api/v1/probes/pool') return { probing_node_ids: [], queued_node_ids: [] }
        return {}
      })
      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({ render: () => h(NodesView) })
      try {
        app.mount(mountEl)
        await vi.waitFor(() => expect(mountEl.querySelector('[data-testid="node-card"]')).not.toBeNull())
        const card = mountEl.querySelector<HTMLElement>('[data-testid="node-card"]')!
        card.querySelector<HTMLButtonElement>('[data-testid="node-platform-details-toggle"]')!.click()
        await nextTick()
        const expanded = card.querySelector<HTMLElement>('[data-testid="node-platform-details-expanded"]')!
        const platformBadges = (root: Element, name: string) => Array.from(root.querySelectorAll<HTMLElement>('.badge[title]'))
          .filter((el) => el.textContent?.toUpperCase().includes(name))
        const assertQualified = (element: HTMLElement, suffix: string, title: string) => {
          expect(element.textContent).toContain(suffix)
          expect(element.textContent).toContain('(US)')
          expect(element.classList.contains('badge-info')).toBe(true)
          expect(element.classList.contains('badge-success')).toBe(false)
          expect(element.title).toBe(title)
          expect(element.title).toContain('cdn_trace')
          expect(element.title).toContain(claude.summary!)
        }
        const expectedZH = getNodePlatformBadges(node, 'ai')[0].tooltip!
        expect(expectedZH).toMatch(/^仅保留原始观测，不作为当前节点能力依据；请重新检测。 · 原始结论: 可用 \(available\)/)
        const listBadges = platformBadges(expanded, 'CLAUDE')
        expect(listBadges).toHaveLength(1)
        assertQualified(listBadges[0], ' · 非当前记录', expectedZH)
        const currentSibling = platformBadges(expanded, 'NETFLIX')
        expect(currentSibling).toHaveLength(1)
        expect(currentSibling[0].classList.contains('badge-success')).toBe(true)
        expect(currentSibling[0].textContent).not.toContain('非当前记录')

        card.querySelector<HTMLButtonElement>('[data-testid="node-inspect-btn"]')!.click()
        await vi.waitFor(() => expect(document.body.querySelector('[data-testid="node-detail-drawer"]')).not.toBeNull())
        const drawer = document.body.querySelector('[data-testid="node-detail-drawer"]')!
        await vi.waitFor(() => expect(platformBadges(drawer, 'CLAUDE')).toHaveLength(2))
        // Both the top capability row and the platform matrix share the projection.
        for (const badge of platformBadges(drawer, 'CLAUDE')) assertQualified(badge, ' · 非当前记录', expectedZH)

        setLocale('en-US')
        await nextTick()
        const expectedEN = getNodePlatformBadges(node, 'ai')[0].tooltip!
        expect(expectedEN).toMatch(/^Original observation only; not evidence of current node capability\. Probe again\. · Recorded verdict: Available \(available\)/)
        const englishBadges = [...platformBadges(expanded, 'CLAUDE'), ...platformBadges(drawer, 'CLAUDE')]
        expect(englishBadges).toHaveLength(3)
        for (const badge of englishBadges) assertQualified(badge, ' · Recorded (not current)', expectedEN)
        expect(JSON.stringify(node)).toBe(original)
        // History evidence formatting is deliberately not changed by node staleness.
        expect(formatPlatformBadge('claude', claude).tone).toBe('success')
        expect(formatPlatformBadge('claude', claude).label).not.toContain('Recorded (not current)')
      } finally {
        app.unmount()
        mountEl.remove()
        setLocale(locale)
      }
    })

    it('renders platform badges in NodesView ledger', async () => {
      const mockNodes = [
        subcheckFixtureNodes.streamingFull,
        subcheckFixtureNodes.aiWeb,
      ]

      vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
        if (path.includes('/api/v1/probes/pool')) {
          return { queue_nodes_count: 0, probing_count: 0, probing_node_ids: [], queued_node_ids: [], total_count: 2 }
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
          return []
        }
        return {}
      })

      const mountEl = document.createElement('div')
      document.body.appendChild(mountEl)
      const app = createApp({
        render: () => h(NodesView),
      })
      try {
        app.mount(mountEl)
        await nextTick()
        await new Promise((r) => setTimeout(r, 60))

        const cards = mountEl.querySelectorAll<HTMLElement>('[data-testid="node-card"]')
        expect(cards).toHaveLength(2)
        for (const card of cards) {
          card.querySelector<HTMLButtonElement>('[data-testid="node-platform-details-toggle"]')!.click()
        }
        await nextTick()
        const streamingDetails = cards[0]!.querySelector('[data-testid="node-platform-details-expanded"]')!
        const aiDetails = cards[1]!.querySelector('[data-testid="node-platform-details-expanded"]')!
        expect(streamingDetails.textContent).toContain('NETFLIX (HK)')
        expect(streamingDetails.textContent).toContain('YOUTUBE (SG)')
        expect(streamingDetails.textContent).toContain('D+ (HK)')
        expect(aiDetails.textContent).toContain('OpenAI 入口受限')
        expect(aiDetails.textContent).toContain('CLAUDE 受限 (HK)')
        const netflixBadge = Array.from(streamingDetails.querySelectorAll<HTMLElement>('[title]'))
          .find((element) => element.textContent?.trim() === 'NETFLIX (HK)')
        expect(netflixBadge).toBeDefined()
        expect(netflixBadge?.classList.contains('badge-success')).toBe(true)
        expect(netflixBadge?.title).toContain('Netflix 全量剧集解锁')
      } finally {
        app.unmount()
        mountEl.remove()
      }
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
            node: normalizeNode({ ...subcheckFixtureNodes.aiFull, capabilities: { ai: { verdict: 'available', stale: true, platforms: obsWithPlatforms.platforms } } }),
          }),
      })
      app.mount(mountEl)
      await nextTick()

      const text = document.body.querySelector('[data-testid="probe-evidence-sheet"]')?.textContent || ''
      expect(text).toContain('旧版探测结果，需重新检测')
      expect(text).toContain('Claude (US)')
      expect(text).not.toContain('非当前记录')
      expect(text).toContain('8.0 MB/s')

      app.unmount()
      mountEl.remove()
    })
  })
})

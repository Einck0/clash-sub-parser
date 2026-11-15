// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { setLocale } from '../../locales'
import ProbeEvidenceSheet from './ProbeEvidenceSheet.vue'
import ProbeRunCard from './ProbeRunCard.vue'
import type { ProbeRun } from './probeTypes'

let app: ReturnType<typeof createApp> | undefined
let host: HTMLDivElement | undefined
afterEach(() => { app?.unmount(); host?.remove() })

it('shows unknown observation nodes and meaningful evidence without exposing run/node/digest keys', async () => {
  setLocale('zh-CN')
  const run: ProbeRun = {
    id: 'run_private', idempotency_key: 'key_private', actor_scope: 'default', config_revision: 'revision_private',
    state: 'running', deadline_at: '2026-11-15T12:00:00Z', created_at: '2026-11-15T11:00:00Z', updated_at: '2026-11-15T11:00:00Z',
  }
  const onCancel = vi.fn()
  host = document.createElement('div')
  document.body.appendChild(host)
  app = createApp({ render: () => h('div', [
    h(ProbeRunCard, { run, onCancel }),
    h(ProbeEvidenceSheet, { open: true, run, observations: [{
      id: 'obs_private', probe_run_id: run.id, node_logical_id: 'node_private', kind: 'baseline', verdict: 'available',
      evidence_digest: 'sha256:private_digest', latency_ms: 42, observed_at: run.created_at,
      redacted_summary: '节点 node_private 已连接，sha256:private_digest',
    }] }),
  ]) })
  app.mount(host)
  await nextTick()
  expect(host.textContent).toContain('未知节点（来源不可用）')
  expect(host.textContent).toContain('已连接')
  expect(host.textContent).not.toMatch(/run_private|node_private|obs_private|private_digest|revision_private/)
  host.querySelector<HTMLButtonElement>('[data-testid="cancel-run-btn"]')!.click()
  expect(onCancel).toHaveBeenCalledWith('run_private')
})

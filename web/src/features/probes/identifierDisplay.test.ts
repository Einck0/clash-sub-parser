// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { setLocale } from '../../locales'
import ProbeEvidenceSheet from './ProbeEvidenceSheet.vue'
import ProbeRunCard from './ProbeRunCard.vue'
import type { ProbeObservation, ProbeRun } from './probeTypes'

let app: ReturnType<typeof createApp> | undefined
let host: HTMLDivElement | undefined
afterEach(() => { app?.unmount(); host?.remove() })

it('renders anonymous preflight limitations and checks in the visible drawer body', async () => {
  setLocale('zh-CN')
  const onClose = vi.fn()
  const onPage = vi.fn()
  const observation: ProbeObservation = {
    id: 'obs-1', probe_run_id: 'run-1', node_logical_id: 'node-1', kind: 'ai', verdict: 'available',
    evidence_digest: 'digest', observed_at: '2026-11-25T11:00:00Z', latency_ms: 35, redacted_summary: '',
    platforms: {
      openai: {
        verdict: 'available', evidence_scope: 'anonymous_preflight', sub_tier: 'full',
        checks: { chatgpt_web: { verdict: 'available', status_code: 200, evidence_scope: 'anonymous_preflight' } },
      },
    },
  }
  host = document.createElement('div')
  document.body.appendChild(host)
  app = createApp({ render: () => h(ProbeEvidenceSheet, {
    open: true, run: { id: 'run-1', idempotency_key: '', actor_scope: '', config_revision: '', state: 'succeeded', deadline_at: '', created_at: '2026-11-25T10:00:00Z', updated_at: '' },
    observations: [observation], total: 51, page: 1, pageSize: 25, loading: false, onClose, onPage,
  }) })
  app.mount(host)
  await nextTick()
  const dialog = document.body.querySelector('[data-drawer-panel]')!
  expect(dialog.textContent).toContain('匿名公开入口预检')
  expect(dialog.textContent).toContain('账号登录、付费模型')
  expect(dialog.textContent).toContain('API 鉴权、API 调用和模型推理未检测')
  expect(dialog.textContent).toContain('网页入口: 已确认 · HTTP 200')
  expect(document.body.querySelectorAll('[data-drawer-panel]')).toHaveLength(1)
  document.body.querySelector<HTMLButtonElement>('[aria-label="下一页证据"]')!.click()
  expect(onPage).toHaveBeenCalledWith(2)
  Array.from(dialog.querySelectorAll('button')).find((button) => button.textContent?.includes('关闭'))!.click()
  expect(onClose).toHaveBeenCalledOnce()
})

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
    }], loading: false }),
  ]) })
  app.mount(host)
  await nextTick()
  const renderedText = document.body.textContent
  expect(renderedText).toContain('未知节点（来源不可用）')
  expect(renderedText).toContain('已连接')
  expect(renderedText).not.toMatch(/run_private|node_private|obs_private|private_digest|revision_private/)
  host.querySelector<HTMLButtonElement>('[data-testid="cancel-run-btn"]')!.click()
  expect(onCancel).toHaveBeenCalledWith('run_private')
})

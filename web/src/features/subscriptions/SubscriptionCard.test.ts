// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import SubscriptionCard from './SubscriptionCard.vue'
import type { SubscriptionRecord } from './useSubscriptions'

const subscription: SubscriptionRecord = {
  id: 'fixture-subscription',
  name: 'Fixture subscription',
  source_url_secret_ref: 'fixture-source-reference',
  enabled: true,
  prefix: '',
  refresh_policy: {
    interval_seconds: 86400,
    user_agent_policy: 'default',
    timeout_seconds: 30,
    max_response_bytes: 10485760,
  },
  revision: 'fixture-revision',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

let app: ReturnType<typeof createApp> | null = null
let container: HTMLDivElement | null = null

afterEach(() => {
  app?.unmount()
  container?.remove()
  app = null
  container = null
})

describe('SubscriptionCard error feedback', () => {
  it('keeps the complete long error readable and preserves dismissal', async () => {
    const rowError = `Fixture refresh failed: ${'long-unbroken-diagnostic-'.repeat(30)}\nRetry after adjusting the source.`
    const dismissed: string[] = []
    container = document.createElement('div')
    document.body.appendChild(container)
    app = createApp({
      render: () => h(SubscriptionCard, {
        subscription,
        rowError,
        onClearError: (id: string) => dismissed.push(id),
      }),
    })
    app.mount(container)
    await nextTick()

    const error = container.querySelector<HTMLElement>('[data-testid="sub-row-error"]')!
    expect(error.getAttribute('role')).toBe('status')
    const message = error.querySelector('span')!
    expect(message.textContent).toBe(rowError)
    expect(message.classList.contains('truncate')).toBe(false)
    expect(message.classList.contains('whitespace-pre-wrap')).toBe(true)
    expect(message.className).toContain('overflow-wrap:anywhere')
    error.querySelector<HTMLButtonElement>('button')!.click()
    expect(dismissed).toEqual(['fixture-subscription'])
  })
})

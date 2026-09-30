// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import Popover from './Popover.vue'

describe('Popover viewport positioning and keyboard', () => {
  const apps: Array<ReturnType<typeof createApp>> = []
  afterEach(() => {
    apps.forEach((app) => app.unmount())
    apps.length = 0
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  async function mount() {
    const container = document.createElement('div')
    container.style.overflow = 'hidden'
    document.body.append(container)
    const app = createApp({
      render: () => h(Popover, { panelClass: 'w-48' }, {
        trigger: () => h('button', { id: 'trigger' }, 'Actions'),
        default: () => [h('button', { id: 'first' }, 'First'), h('button', { id: 'second' }, 'Second')],
      }),
    })
    apps.push(app)
    app.mount(container)
    const trigger = container.querySelector('#trigger') as HTMLButtonElement
    vi.spyOn(trigger.parentElement!, 'getBoundingClientRect').mockReturnValue({ left: 300, right: 330, top: 240, bottom: 270, width: 30, height: 30, x: 300, y: 240, toJSON: () => ({}) })
    vi.stubGlobal('innerWidth', 340)
    vi.stubGlobal('innerHeight', 300)
    trigger.click()
    await nextTick()
    await nextTick()
    return { container, trigger, panel: document.body.querySelector('[role="menu"]') as HTMLElement }
  }

  it('teleports outside clipped card, positions inside viewport, moves focus and cleans on Escape', async () => {
    const { container, trigger, panel } = await mount()
    expect(panel).toBeTruthy()
    expect(container.contains(panel)).toBe(false)
    expect(panel.style.position).toBe('fixed')
    expect(Number.parseFloat(panel.style.left)).toBeGreaterThanOrEqual(8)
    expect(Number.parseFloat(panel.style.top)).toBeLessThan(300)
    expect(document.activeElement).toBe(panel.querySelector('#first'))
    panel.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(document.body.querySelector('[role="menu"]')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('closes on outside click and repositions on resize without duplicate panels', async () => {
    const { trigger, panel } = await mount()
    window.dispatchEvent(new Event('resize'))
    expect(panel.style.visibility).toBe('visible')
    await new Promise((resolve) => setTimeout(resolve, 1)) // VueUse installs outside listeners on the next task
    document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(document.querySelectorAll('[role="menu"]')).toHaveLength(0)
    trigger.click()
    await nextTick()
    await nextTick()
    expect(document.querySelectorAll('[role="menu"]')).toHaveLength(1)
  })
})

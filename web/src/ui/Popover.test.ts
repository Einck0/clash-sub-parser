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
    vi.unstubAllGlobals()
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

  it('moves through actions with arrow and boundary keys and returns focus on Tab', async () => {
    const { trigger, panel } = await mount()
    const key = (value: string) => document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true }))
    key('ArrowDown')
    expect(document.activeElement).toBe(panel.querySelector('#second'))
    key('ArrowDown')
    expect(document.activeElement).toBe(panel.querySelector('#first'))
    key('End')
    expect(document.activeElement).toBe(panel.querySelector('#second'))
    key('Home')
    expect(document.activeElement).toBe(panel.querySelector('#first'))
    key('Tab')
    await nextTick()
    expect(document.querySelector('[data-popover-panel]')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('keeps menus inside a native dialog top layer instead of the inert page', async () => {
    const dialog = document.createElement('dialog')
    dialog.setAttribute('open', '')
    document.body.appendChild(dialog)
    const app = createApp({ render: () => h(Popover, {}, {
      trigger: () => h('button', { id: 'dialog-trigger' }, 'Actions'),
      default: () => h('button', { id: 'dialog-action' }, 'Edit'),
    }) })
    apps.push(app)
    app.mount(dialog)
    const trigger = dialog.querySelector<HTMLButtonElement>('#dialog-trigger')!
    vi.spyOn(trigger.parentElement!, 'getBoundingClientRect').mockReturnValue({ left: 20, right: 50, top: 20, bottom: 50, width: 30, height: 30, x: 20, y: 20, toJSON: () => ({}) })
    trigger.click()
    await nextTick()
    await nextTick()
    expect(dialog.querySelector('[data-popover-panel]')).toBeTruthy()
    expect(document.activeElement).toBe(dialog.querySelector('#dialog-action'))
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

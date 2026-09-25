// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { createApp, h } from 'vue'
import ResponsiveGrid from './ResponsiveGrid.vue'

describe('ResponsiveGrid Component', () => {
  let container: HTMLDivElement
  let app: ReturnType<typeof createApp> | null = null

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    if (app) {
      app.unmount()
      app = null
    }
    container.remove()
    document.body.innerHTML = ''
  })

  it('renders with default adaptive minmax clamp column layout and md gap', () => {
    app = createApp({
      render() {
        return h(
          ResponsiveGrid,
          {},
          {
            default: () => [
              h('div', { class: 'card-1' }, 'Card 1'),
              h('div', { class: 'card-2' }, 'Card 2'),
            ],
          }
        )
      },
    })

    app.mount(container)
    const grid = container.querySelector('div.grid') as HTMLElement
    expect(grid).toBeTruthy()
    expect(grid.classList.contains('min-w-0')).toBe(true)
    expect(grid.classList.contains('max-w-full')).toBe(true)
    expect(grid.classList.contains('gap-4')).toBe(true)
    expect(grid.style.gridTemplateColumns).toContain('repeat(auto-fit, minmax(min(clamp(16rem, 24vw + 10rem, 20rem), 100%), 1fr))')
  })

  it('accepts custom gap and custom minCardWidth', () => {
    app = createApp({
      render() {
        return h(
          ResponsiveGrid,
          {
            as: 'section',
            gap: 'lg',
            minCardWidth: '320px',
          },
          {
            default: () => h('div', {}, 'Section Card'),
          }
        )
      },
    })

    app.mount(container)
    const grid = container.querySelector('section.grid') as HTMLElement
    expect(grid).toBeTruthy()
    expect(grid.classList.contains('gap-6')).toBe(true)
    expect(grid.style.gridTemplateColumns).toContain('repeat(auto-fit, minmax(min(320px, 100%), 1fr))')
  })
})

import { expect, test } from '@playwright/test'

const viewports = [
  { name: 'mobile (375x812)', width: 375, height: 812 },
  { name: 'tablet (768x900)', width: 768, height: 900 },
  { name: 'desktop (1024x900)', width: 1024, height: 900 },
  { name: 'wide (1440x900)', width: 1440, height: 900 },
]

test.describe('Node Ledger Multi-Viewport Visual & Interaction Gate', () => {
  for (const vp of viewports) {
    test(`Viewport ${vp.name}: zero document horizontal overflow and scannable hierarchy`, async ({ page }) => {
      await page.setViewportSize({ width: vp.width, height: vp.height })

      // Intercept API calls non-destructively with fixture data
      await page.route('/api/**', async (route) => {
        const url = route.request().url()
        if (url.includes('/api/nodes')) {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify([
              {
                id: 'node-1',
                name: 'HK-BGP-01',
                server: 'hk01.example.test',
                port: 443,
                type: 'ss',
                subscription_name: 'Sub-A',
                country: 'HK',
              },
              {
                id: 'node-2',
                name: 'JP-Tokyo-02',
                server: 'jp02.example.test',
                port: 8443,
                type: 'vmess',
                subscription_name: 'Sub-B',
                country: 'JP',
              },
            ]),
          })
        } else {
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify([]),
          })
        }
      })

      await page.goto('/nodes')
      await page.waitForLoadState('domcontentloaded')

      // Assert zero document horizontal overflow
      const overflow = await page.evaluate(() => {
        const docEl = document.documentElement
        const body = document.body
        const scrollWidth = Math.max(docEl.scrollWidth, body.scrollWidth)
        const clientWidth = docEl.clientWidth
        return scrollWidth > clientWidth + 1
      })
      expect(overflow).toBeFalsy()

      // Verify dark canvas color is #08090a
      const bodyBg = await page.evaluate(() => {
        return window.getComputedStyle(document.body).backgroundColor
      })
      expect(bodyBg).toBe('rgb(8, 9, 10)')
    })
  }

  test('Reduced motion disables sustained transitions on overlays', async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 900 })
    await page.emulateMedia({ reducedMotion: 'reduce' })

    await page.route('/api/**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      })
    })

    await page.goto('/nodes')
    await page.waitForLoadState('domcontentloaded')

    const animDuration = await page.evaluate(() => {
      const el = document.createElement('div')
      el.className = 'drawer-slide-right'
      document.body.appendChild(el)
      const duration = parseFloat(window.getComputedStyle(el).animationDuration) || 0
      document.body.removeChild(el)
      return duration
    })

    expect(animDuration).toBeLessThanOrEqual(0.001)
  })
})

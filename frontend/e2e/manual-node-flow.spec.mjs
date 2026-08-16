import { expect, test } from '@playwright/test'

const subscriptionName = 'E2E 手动订阅'
const originalNodeName = 'E2E 原始节点'
const editedNodeName = 'E2E 编辑节点'

async function subscriptionFromApi(page) {
  const response = await page.request.get('/api/subscriptions')
  expect(response.ok()).toBeTruthy()
  const subscriptions = await response.json()
  return subscriptions.find((item) => item.name === subscriptionName)
}

test('通过真实点击保存、编辑、导出并清空手动节点', async ({ page }) => {
  await page.goto('/')

  await page.getByTestId('add-manual-node').click()
  await page.getByTestId('manual-subscription-name').fill(subscriptionName)
  await page.getByTestId('manual-node-name').fill(originalNodeName)
  await page.getByTestId('manual-node-server').fill('e2e.example.test')
  await page.getByTestId('manual-node-port').fill('443')
  await page.getByTestId('manual-node-cipher').fill('aes-128-gcm')
  await page.getByTestId('manual-node-password').fill('e2e-test-password')
  await page.getByTestId('manual-add-draft').click()
  await expect(page.getByText(originalNodeName, { exact: true })).toBeVisible()

  await page.getByTestId('manual-save').click()
  const subscriptionCard = page.getByTestId('subscription-card').filter({ hasText: subscriptionName })
  await expect(subscriptionCard).toBeVisible()
  await expect(subscriptionCard).toContainText('1 节点')

  await subscriptionCard.getByTestId('subscription-edit').click()
  await expect(page.getByTestId('subscription-feature-manual')).toHaveClass(/active/)
  await expect(page.getByTestId('manual-node-preview').locator(':scope > div')).toHaveCount(1)
  await page.getByTestId('manual-node-edit').click()
  await page.getByTestId('manual-node-yaml').fill([
    `name: ${editedNodeName}`,
    'type: ss',
    'server: e2e.example.test',
    'port: 443',
    'cipher: aes-128-gcm',
    'password: e2e-test-password',
    'plugin: v2ray-plugin',
    'plugin-opts:',
    '  mode: websocket',
    '  host: e2e.example.test',
  ].join('\n'))
  await page.getByTestId('manual-node-yaml-apply').click()
  await page.getByTestId('subscription-save').click()
  await expect(subscriptionCard).toContainText('1 节点')

  const saved = await subscriptionFromApi(page)
  expect(saved).toBeTruthy()
  expect(saved.manual_nodes).toHaveLength(1)
  expect(saved.manual_nodes[0]['plugin-opts']).toEqual({
    mode: 'websocket',
    host: 'e2e.example.test',
  })

  await page.getByTestId('nav-generate').click()
  await page.getByTestId('generate-yaml').click()
  await expect(page.getByTestId('generated-yaml-output')).toHaveValue(new RegExp(editedNodeName))

  await page.getByTestId('nav-subscriptions').click()
  await subscriptionCard.getByTestId('subscription-edit').click()
  await expect(page.getByTestId('subscription-feature-manual')).toHaveClass(/active/)
  await page.getByTestId('manual-node-remove').click()
  await page.getByTestId('subscription-save').click()
  await expect(subscriptionCard).toContainText('0 节点')

  const cleared = await subscriptionFromApi(page)
  expect(cleared.manual_nodes).toEqual([])
  expect(cleared.raw_nodes).toEqual([])
})

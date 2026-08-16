import { expect, test } from '@playwright/test'

const subscriptionName = 'E2E 手动订阅'
const originalNodeName = 'E2E 原始节点'
const secondNodeName = 'E2E 第二节点'
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

  await page.getByTestId('manual-node-name').fill(secondNodeName)
  await page.getByTestId('manual-node-server').fill('e2e-second.example.test')
  await page.getByTestId('manual-node-port').fill('443')
  await page.getByTestId('manual-node-cipher').fill('aes-128-gcm')
  await page.getByTestId('manual-node-password').fill('e2e-second-password')
  await page.getByTestId('manual-add-draft').click()
  await expect(page.getByText(secondNodeName, { exact: true })).toBeVisible()

  await page.getByTestId('manual-save').click()
  const subscriptionCard = page.getByTestId('subscription-card').filter({ hasText: subscriptionName })
  await expect(subscriptionCard).toBeVisible()
  await expect(subscriptionCard).toContainText('2 节点')

  await subscriptionCard.getByTestId('subscription-edit').click()
  await expect(page.getByTestId('subscription-feature-manual')).toHaveClass(/active/)
  const savedRows = page.locator('.manual-node-list .node-select-row')
  await expect(savedRows).toHaveCount(2)
  const savedHandles = page.getByTestId('manual-saved-node-drag-handle')
  await expect(savedHandles).toHaveCount(2)
  await savedHandles.nth(1).dragTo(savedRows.nth(0))
  await expect(savedRows.nth(0)).toContainText(secondNodeName)
  await page.getByTestId('subscription-save').click()
  await expect(subscriptionCard).toContainText('2 节点')

  const reordered = await subscriptionFromApi(page)
  expect(reordered.manual_nodes.map((node) => node.name)).toEqual([secondNodeName, originalNodeName])

  await subscriptionCard.getByTestId('subscription-edit').click()
  await expect(page.getByTestId('subscription-feature-manual')).toHaveClass(/active/)
  await expect(page.getByTestId('manual-node-preview').locator(':scope > div')).toHaveCount(2)
  await page.getByTestId('manual-node-edit').first().click()
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
  await expect(subscriptionCard).toContainText('2 节点')

  const saved = await subscriptionFromApi(page)
  expect(saved).toBeTruthy()
  expect(saved.manual_nodes).toHaveLength(2)
  expect(saved.manual_nodes.map((node) => node.name)).toEqual([editedNodeName, originalNodeName])
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
  await page.getByTestId('manual-node-remove').first().click()
  await page.getByTestId('manual-node-remove').first().click()
  await page.getByTestId('subscription-save').click()
  await expect(subscriptionCard).toContainText('0 节点')

  const cleared = await subscriptionFromApi(page)
  expect(cleared.manual_nodes).toEqual([])
  expect(cleared.raw_nodes).toEqual([])
})

test('通过真实拖拽持久化策略组排序', async ({ page }) => {
  const firstName = 'E2E 策略组一'
  const secondName = 'E2E 策略组二'
  for (const [name, sortOrder] of [[firstName, 0], [secondName, 1]]) {
    const response = await page.request.post('/api/node-groups', {
      data: { name, sort_order: sortOrder },
    })
    expect(response.ok()).toBeTruthy()
  }

  await page.goto('/node-groups')
  const firstCard = page.locator('.group-card').filter({ hasText: firstName })
  const secondCard = page.locator('.group-card').filter({ hasText: secondName })
  await expect(firstCard).toBeVisible()
  await expect(secondCard).toBeVisible()
  await secondCard.locator('.drag-handle').dragTo(firstCard)

  await expect.poll(async () => {
    const response = await page.request.get('/api/node-groups')
    const groups = await response.json()
    return groups.filter((group) => [firstName, secondName].includes(group.name)).map((group) => group.name)
  }).toEqual([secondName, firstName])
})

test('通过真实拖拽保存规则分类和规则顺序', async ({ page }) => {
  const categoryOne = 'E2E 分类一'
  const categoryTwo = 'E2E 分类二'
  for (const [name, sort_order] of [[categoryOne, 0], [categoryTwo, 10]]) {
    const response = await page.request.post('/api/rule-categories', { data: { name, sort_order } })
    expect(response.ok()).toBeTruthy()
  }

  await page.goto('/rules')
  const categoryCardOne = page.locator('.category-card').filter({ hasText: categoryOne })
  const categoryCardTwo = page.locator('.category-card').filter({ hasText: categoryTwo })
  await expect(categoryCardOne).toBeVisible()
  await expect(categoryCardTwo).toBeVisible()
  await categoryCardTwo.locator('.drag-handle').dragTo(categoryCardOne)
  await page.getByRole('button', { name: '保存全部', exact: true }).click()
  await expect.poll(async () => {
    const response = await page.request.get('/api/rule-categories')
    const categories = await response.json()
    return categories.filter((category) => [categoryOne, categoryTwo].includes(category.name)).map((category) => category.name)
  }).toEqual([categoryTwo, categoryOne])

  for (const [name, sort_order] of [['E2E 规则一', 0], ['E2E 规则二', 10]]) {
    const response = await page.request.post('/api/rules', {
      data: { name, category: categoryOne, type: 'DOMAIN', value: name, proxy: 'DIRECT', sort_order },
    })
    expect(response.ok()).toBeTruthy()
  }

  await page.goto(`/rules/category/${encodeURIComponent(categoryOne)}`)
  const ruleRows = page.locator('.rules-table tbody tr')
  await expect(ruleRows).toHaveCount(2)
  await ruleRows.nth(1).locator('.drag-handle').dragTo(ruleRows.nth(0))
  await page.locator('.fab-save').click()
  await expect.poll(async () => {
    const response = await page.request.get('/api/rules')
    const rules = await response.json()
    return rules.filter((rule) => rule.category === categoryOne).map((rule) => rule.name)
  }).toEqual(['E2E 规则二', 'E2E 规则一'])
})

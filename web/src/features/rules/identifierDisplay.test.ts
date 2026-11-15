// @vitest-environment jsdom
import { afterEach, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import RuleList from './RuleList.vue'
import type { PolicyRuleView } from './rulesTypes'

let app: ReturnType<typeof createApp> | undefined
let host: HTMLDivElement | undefined
afterEach(() => { app?.unmount(); host?.remove() })

it('uses a business placeholder for deleted target groups in desktop and mobile search results', async () => {
  const rule: PolicyRuleView = {
    id: 'rule_private', revision_id: 'revision_private', target_group_id: 'group_private',
    expression: 'DOMAIN-SUFFIX,example.com', enabled: true, position: 0,
  }
  host = document.createElement('div')
  document.body.appendChild(host)
  app = createApp({ render: () => h(RuleList, {
    categories: [], rulesByCategory: { map: new Map(), uncategorized: [rule] }, filteredRules: [rule],
    selectedCategoryId: 'all', isSearching: true, searchQuery: 'example', groupNameMap: new Map(), collapsedCategories: {},
  }) })
  app.mount(host)
  await nextTick()
  expect(host.textContent).toContain('未知策略组')
  expect(host.textContent).toContain('example.com')
  expect(host.textContent).not.toMatch(/group_private|rule_private|revision_private/)
})

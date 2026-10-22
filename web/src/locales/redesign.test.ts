// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest'
import { computed } from 'vue'
import { setLocale, toggleLocale } from './index'
import { uiText } from './redesign'

describe('uiText locale helper', () => {
  beforeEach(() => {
    setLocale('zh-CN')
  })

  it('returns Chinese by default and English when locale is set to en-US', () => {
    expect(uiText('节点列表', 'Node Ledger')).toBe('节点列表')

    setLocale('en-US')
    expect(uiText('节点列表', 'Node Ledger')).toBe('Node Ledger')

    setLocale('zh-CN')
    expect(uiText('节点列表', 'Node Ledger')).toBe('节点列表')
  })

  it('triggers reactive computed updates upon toggleLocale', () => {
    const title = computed(() => uiText('分流策略', 'Policy Tree'))
    expect(title.value).toBe('分流策略')

    toggleLocale()
    expect(title.value).toBe('Policy Tree')

    toggleLocale()
    expect(title.value).toBe('分流策略')
  })
})

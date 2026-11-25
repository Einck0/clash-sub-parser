// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import NodeTargetCompatibility from './NodeTargetCompatibility.vue'
import { normalizeNode, type NodeRecord } from './nodeView'
import { setLocale } from '../../locales'
let app: ReturnType<typeof createApp> | undefined
let host: HTMLElement | undefined
afterEach(() => { app?.unmount(); host?.remove(); setLocale('zh-CN') })
function mount(compatibility?: NodeRecord['target_compatibility']) {
  setLocale('zh-CN')
  host = document.createElement('div')
  document.body.append(host)
  const node = normalizeNode({ logical_id: 'node-fixture', display_name: 'Fixture', protocol: 'ss', active: true, target_compatibility: compatibility })
  app = createApp({ render: () => h(NodeTargetCompatibility, { node }) })
  app.mount(host)
  return host
}
describe('visible target compatibility contract', () => {
  it('distinguishes invalid parameters from target incompatibility without calling either a probe failure', async () => {
    const element = mount({ mihomo: { status: 'format_error' }, singbox: { status: 'incompatible' }, surge: { status: 'compatible' } })
    expect(element.textContent).toContain('mihomo: 参数格式错误')
    expect(element.textContent).toContain('singbox: 目标不兼容')
    expect(element.textContent).toContain('surge: 参数兼容')
    expect(element.textContent).toContain('兼容性独立于检测健康')
    setLocale('en-US')
    await nextTick()
    expect(element.textContent).toContain('Invalid parameters')
    expect(element.textContent).toContain('Target incompatible')
  })
  it('labels missing diagnostics as protocol support rather than verified parameters', () => {
    const element = mount()
    expect(element.textContent).toContain('协议支持')
    expect(element.textContent).toContain('尚无当前连接参数的兼容诊断')
    expect(element.textContent).not.toContain('参数兼容')
  })
})

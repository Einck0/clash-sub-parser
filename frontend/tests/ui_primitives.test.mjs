import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

test('Tailwind theme and UI primitives exist and conform to design tokens', () => {
  const themePath = path.resolve(import.meta.dirname, '../src/assets/theme.css')
  assert.ok(fs.existsSync(themePath), 'theme.css should exist')
  const themeContent = fs.readFileSync(themePath, 'utf8')
  assert.match(themeContent, /--color-canvas: #090D16/)
  assert.match(themeContent, /--font-mono: 'JetBrains Mono'/)

  const drawerPath = path.resolve(import.meta.dirname, '../src/components/ui/BaseDrawer.vue')
  assert.ok(fs.existsSync(drawerPath), 'BaseDrawer.vue should exist')

  const badgePath = path.resolve(import.meta.dirname, '../src/components/ui/StatusBadge.vue')
  assert.ok(fs.existsSync(badgePath), 'StatusBadge.vue should exist')

  const virtualTablePath = path.resolve(import.meta.dirname, '../src/components/ui/VirtualNodeTable.vue')
  assert.ok(fs.existsSync(virtualTablePath), 'VirtualNodeTable.vue should exist')
})

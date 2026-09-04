import assert from 'node:assert/strict'
import test from 'node:test'
import fs from 'node:fs'
import path from 'node:path'

test('Workbench Header and Sidebar components exist and have standard props/events', () => {
  const headerPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchHeader.vue')
  assert.ok(fs.existsSync(headerPath), 'WorkbenchHeader.vue should exist')
  const headerContent = fs.readFileSync(headerPath, 'utf8')
  assert.match(headerContent, /CSP \/\/ WORKBENCH/)
  assert.match(headerContent, /open-export/)

  const sidebarPath = path.resolve(import.meta.dirname, '../src/components/workbench/WorkbenchSidebar.vue')
  assert.ok(fs.existsSync(sidebarPath), 'WorkbenchSidebar.vue should exist')
  const sidebarContent = fs.readFileSync(sidebarPath, 'utf8')
  assert.match(sidebarContent, /\/nodes/)
  assert.match(sidebarContent, /\/subscriptions/)
  assert.match(sidebarContent, /\/groups/)
})

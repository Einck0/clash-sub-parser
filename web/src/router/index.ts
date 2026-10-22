import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import { ROUTE_STORAGE_KEY } from '../navigation'

export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: () => import('../features/dashboard/DashboardView.vue'),
    meta: { title: 'Dashboard', icon: 'Squares2X2Icon' },
  },
  {
    path: '/subscriptions',
    name: 'subscriptions',
    component: () => import('../features/subscriptions/SubscriptionsView.vue'),
    meta: { title: 'Subscriptions', icon: 'DocumentDuplicateIcon' },
  },
  {
    path: '/nodes',
    name: 'nodes',
    component: () => import('../features/nodes/NodesView.vue'),
    meta: { title: 'Node Ledger', icon: 'ServerStackIcon' },
  },
  {
    path: '/probes',
    redirect: '/nodes',
  },
  {
    path: '/dns',
    name: 'dns',
    component: () => import('../features/dns/DnsView.vue'),
    meta: { title: 'DNS', icon: 'GlobeAltIcon' },
  },
  {
    path: '/policy',
    name: 'policy',
    component: () => import('../features/policy/PolicyView.vue'),
    meta: { title: 'Policy Tree', icon: 'CpuChipIcon' },
  },
  {
    path: '/rules',
    name: 'rules',
    component: () => import('../features/rules/RulesView.vue'),
    meta: { title: 'Rule Desk', icon: 'ShieldCheckIcon' },
  },
  {
    path: '/publications',
    name: 'publications',
    component: () => import('../features/publications/PublicationsView.vue'),
    meta: { title: 'Exports', icon: 'ArrowDownTrayIcon' },
  },
  {
    path: '/settings',
    name: 'settings',
    component: () => import('../features/settings/SettingsView.vue'),
    meta: { title: 'Settings', icon: 'Cog6ToothIcon' },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: (to) => {
      // If someone passed hash like #subscriptions without leading slash, pathMatch is "subscriptions"
      const path = to.params.pathMatch
      const matchedTab = Array.isArray(path) ? path[0] : path
      if (matchedTab === 'probes') {
        return '/nodes'
      }
      if (['dashboard', 'subscriptions', 'nodes', 'dns', 'policy', 'rules', 'publications', 'settings'].includes(matchedTab)) {
        return `/${matchedTab}`
      }
      return '/dashboard'
    },
  },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

// Synchronize route state with navigation storage
router.afterEach((to) => {
  const tabName = to.name ? String(to.name) : 'dashboard'
  if (typeof window !== 'undefined' && typeof window.localStorage !== 'undefined') {
    try {
      window.localStorage.setItem(ROUTE_STORAGE_KEY, tabName)
    } catch {
      // ignore
    }
  }
})

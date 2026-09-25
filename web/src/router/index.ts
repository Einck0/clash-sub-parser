import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'
import DashboardView from '../features/dashboard/DashboardView.vue'
import SubscriptionsView from '../features/subscriptions/SubscriptionsView.vue'
import NodesView from '../features/nodes/NodesView.vue'
import ProbesView from '../features/probes/ProbesView.vue'
import PolicyView from '../features/policy/PolicyView.vue'
import PublicationsView from '../features/publications/PublicationsView.vue'
import SettingsView from '../features/settings/SettingsView.vue'
import { ROUTE_STORAGE_KEY } from '../navigation'

export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/dashboard',
  },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: DashboardView,
    meta: { title: 'Dashboard', icon: 'Squares2X2Icon' },
  },
  {
    path: '/subscriptions',
    name: 'subscriptions',
    component: SubscriptionsView,
    meta: { title: 'Subscriptions', icon: 'DocumentDuplicateIcon' },
  },
  {
    path: '/nodes',
    name: 'nodes',
    component: NodesView,
    meta: { title: 'Node Ledger', icon: 'ServerStackIcon' },
  },
  {
    path: '/probes',
    name: 'probes',
    component: ProbesView,
    meta: { title: 'Probe Engine', icon: 'BoltIcon' },
  },
  {
    path: '/policy',
    name: 'policy',
    component: PolicyView,
    meta: { title: 'Policy Tree', icon: 'CpuChipIcon' },
  },
  {
    path: '/publications',
    name: 'publications',
    component: PublicationsView,
    meta: { title: 'Exports', icon: 'ArrowDownTrayIcon' },
  },
  {
    path: '/settings',
    name: 'settings',
    component: SettingsView,
    meta: { title: 'Settings', icon: 'Cog6ToothIcon' },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: (to) => {
      // If someone passed hash like #subscriptions without leading slash, pathMatch is "subscriptions"
      const path = to.params.pathMatch
      const matchedTab = Array.isArray(path) ? path[0] : path
      if (['dashboard', 'subscriptions', 'nodes', 'probes', 'policy', 'publications', 'settings'].includes(matchedTab)) {
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

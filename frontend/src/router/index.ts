import { createRouter, createWebHistory } from 'vue-router'
import { useAppStore } from '../stores/app'

const routes = [
  { path: '/', name: 'Subscriptions', component: () => import('../views/Subscriptions.vue') },
  { path: '/node-groups', name: 'NodeGroups', component: () => import('../views/NodeGroups.vue') },
  { path: '/rules', name: 'Rules', component: () => import('../views/Rules.vue') },
  { path: '/rules/category/:name', name: 'RuleCategoryDetail', component: () => import('../views/RuleCategoryDetail.vue') },
  { path: '/rules/:id', name: 'RuleDetail', component: () => import('../views/RuleDetail.vue') },
  { path: '/dns', name: 'DnsSettings', component: () => import('../views/DnsSettings.vue') },
  { path: '/generate', name: 'Generate', component: () => import('../views/Generate.vue') },
  { path: '/settings', name: 'Settings', component: () => import('../views/Settings.vue') },
  { path: '/history', name: 'ConfigHistory', component: () => import('../views/ConfigHistory.vue') },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

router.beforeEach((to, from, next) => {
  const store = useAppStore()
  if (store.confirmState.open) {
    store.cancelPendingConfirm()
  }
  next()
})

export default router

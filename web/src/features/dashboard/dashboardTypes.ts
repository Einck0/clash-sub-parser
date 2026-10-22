import type { Component } from 'vue'

export type MetricStatus = 'idle' | 'loading' | 'success' | 'error'

export interface MetricItem {
  value: number | null
  status: MetricStatus
  error: string | null
  isStale?: boolean
}

export interface DashboardMetrics {
  subscriptions: MetricItem
  nodes: MetricItem
  activeNodes: MetricItem
  policies: MetricItem
  rules: MetricItem
  probes: MetricItem
}

export type HealthStatus = 'checking' | 'healthy' | 'unhealthy'

export interface QuickActionItem {
  id: string
  title: string
  description: string
  routeName: string
  icon: Component
}

import { defineStore } from 'pinia'

export const useAppStore = defineStore('app', {
  state: () => ({
    subscriptions: [],
    nodeGroups: [],
    rules: [],
  }),
  actions: {
    setSubscriptions(data) { this.subscriptions = data },
    setNodeGroups(data) { this.nodeGroups = data },
    setRules(data) { this.rules = data },
  },
})

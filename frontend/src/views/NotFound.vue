<template>
  <section class="not-found-view flex flex-col items-center justify-center min-h-[60vh] text-center p-6 space-y-4" data-testid="not-found-view">
    <div class="inline-flex items-center justify-center w-12 h-12 rounded-lg bg-surface-hover text-text-muted border border-border-subtle">
      <svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
    </div>

    <div class="space-y-1">
      <h1 class="text-2xl font-bold font-mono text-text-main tracking-tight">404 // 页面未找到</h1>
      <p class="text-sm text-text-muted max-w-md">
        您访问的客户端路径不存在或已被调整。请检查 URL 或返回节点质检工作台。
      </p>
    </div>

    <div class="pt-2">
      <router-link
        to="/nodes"
        class="inline-flex min-h-[44px] items-center justify-center gap-2 rounded-md bg-accent px-4 py-2 text-xs font-semibold text-white hover:bg-accent-hover transition-colors focus-ring"
      >
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18" />
        </svg>
        <span>返回节点质检中心</span>
      </router-link>
    </div>
  </section>
</template>

<script setup lang="ts">
// Accessible recovery view for unmatched client routes
</script>

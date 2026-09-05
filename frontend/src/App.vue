<template>
  <!-- Full-screen Skip Link for Keyboard Accessibility -->
  <a
    href="#main-content"
    class="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-accent focus:px-4 focus:py-2 focus:text-xs focus:font-medium focus:text-white focus:shadow-xs focus:outline-hidden focus-ring"
  >
    跳转至主工作区
  </a>

  <AuthGate v-if="showAuthGate" :authenticate="handleAuthSubmit" />

  <div v-else class="flex min-h-screen max-w-full overflow-x-hidden flex-col bg-canvas text-text-main min-w-0">
    <!-- Top Workbench Header (48px) -->
    <WorkbenchHeader
      :summary="store.nodeSummary"
      @open-export="showQuickExport = true"
      @toggle-sidebar="mobileSidebarOpen = !mobileSidebarOpen"
    />

    <!-- Main Workspace Body -->
    <div class="flex flex-1 overflow-hidden relative min-w-0">
      <!-- Desktop Sidebar Nav (Fixed on md+ >=768px) -->
      <div class="hidden md:flex shrink-0">
        <WorkbenchSidebar />
      </div>

      <!-- Mobile Sidebar Drawer (Narrow / Mobile Screens <768px) -->
      <BaseDrawer
        v-model="mobileSidebarOpen"
        title="CSP // WORKBENCH"
        placement="left"
        @close="mobileSidebarOpen = false"
      >
        <WorkbenchSidebar :mobile="true" @navigate="mobileSidebarOpen = false" />
      </BaseDrawer>

      <!-- Content Area -->
      <main id="main-content" class="flex-1 overflow-y-auto overflow-x-hidden p-4 md:p-6 min-w-0" aria-live="polite" tabindex="-1">
        <router-view v-slot="{ Component, route }">
          <Transition name="page-fade" mode="out-in">
            <component :is="Component" :key="route.path" />
          </Transition>
        </router-view>
      </main>
    </div>
  </div>

  <ToastContainer />
  <ConfirmDialog />
  <QuickExportModal :open="showQuickExport" :needs-token="exportNeedsToken" @close="showQuickExport = false" />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { checkAuthSession, getSecuritySettings, loginAuthToken } from './api'
import { setAuthToken, syncTokenFromUrl } from './auth'
import AuthGate from './components/AuthGate.vue'
import ToastContainer from './components/ToastContainer.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import QuickExportModal from './components/QuickExportModal.vue'
import WorkbenchHeader from './components/workbench/WorkbenchHeader.vue'
import WorkbenchSidebar from './components/workbench/WorkbenchSidebar.vue'
import BaseDrawer from './components/ui/BaseDrawer.vue'
import { useAppStore } from './stores/app'

const store = useAppStore() as any
const showQuickExport = ref(false)
const mobileSidebarOpen = ref(false)

const probedCount = computed(() => {
  return store.nodes?.filter((n: any) => n.probe_status === 'success')?.length || 0
})

const showAuthGate = ref(false)
const exportNeedsToken = ref(false)

onMounted(() => {
  syncTokenFromUrl()
  window.addEventListener('auth:unauthorized', handleUnauthorized)
  checkFrontendAccess()
  store.refreshNodeSummary()
})

async function checkFrontendAccess() {
  try {
    const { data } = await getSecuritySettings()
    exportNeedsToken.value = Boolean(data.auth_enabled && data.protect_exports)
    if (data.auth_enabled && data.protect_frontend) {
      try {
        await checkAuthSession()
      } catch {
        setAuthToken('')
        showAuthGate.value = true
        return
      }
    }
    showAuthGate.value = false
  } catch (err: any) {
    if (err?.response?.status === 401) {
      setAuthToken('')
      exportNeedsToken.value = true
      showAuthGate.value = true
      return
    }
    showAuthGate.value = false
  }
}

async function handleAuthSubmit(token: string) {
  const { data } = await loginAuthToken(token)
  if (!data?.ok) throw new Error('Token 无效')
  await checkFrontendAccess()
}

function handleUnauthorized() {
  setAuthToken('')
  exportNeedsToken.value = true
  showAuthGate.value = true
}
</script>

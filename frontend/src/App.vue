<template>
  <AuthGate v-if="showAuthGate" :authenticate="handleAuthSubmit" />

  <div v-else class="flex min-h-screen flex-col bg-[#090D16] text-[#F8FAFC]">
    <!-- Top Workbench Header -->
    <WorkbenchHeader
      :total-nodes="store.nodes?.length || 0"
      :probed-count="probedCount"
      @open-export="showQuickExport = true"
      @toggle-sidebar="mobileSidebarOpen = !mobileSidebarOpen"
    />

    <!-- Main Workspace Body -->
    <div class="flex flex-1 overflow-hidden relative">
      <!-- Desktop Sidebar Nav (Fixed on md+) -->
      <div class="hidden md:flex">
        <WorkbenchSidebar />
      </div>

      <!-- Mobile Sidebar Drawer (Narrow / Mobile Screens) -->
      <BaseDrawer
        v-model="mobileSidebarOpen"
        title="CSP // WORKBENCH"
        placement="left"
        @close="mobileSidebarOpen = false"
      >
        <WorkbenchSidebar :mobile="true" @navigate="mobileSidebarOpen = false" />
      </BaseDrawer>

      <!-- Content Area -->
      <main class="flex-1 overflow-y-auto p-4 md:p-6" aria-live="polite">
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

<script setup>
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

const store = useAppStore()
const showQuickExport = ref(false)
const mobileSidebarOpen = ref(false)

const probedCount = computed(() => {
  return store.nodes?.filter(n => n.probe_status === 'success')?.length || 0
})

const showAuthGate = ref(false)
const exportNeedsToken = ref(false)

onMounted(() => {
  syncTokenFromUrl()
  window.addEventListener('auth:unauthorized', handleUnauthorized)
  checkFrontendAccess()
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
  } catch (err) {
    if (err?.response?.status === 401) {
      setAuthToken('')
      exportNeedsToken.value = true
      showAuthGate.value = true
      return
    }
    showAuthGate.value = false
  }
}

async function handleAuthSubmit(token) {
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

<style scoped>
/* Page transition */
.page-fade-enter-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.page-fade-leave-active {
  transition: opacity 0.12s ease;
}
.page-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.page-fade-leave-to {
  opacity: 0;
}
</style>

<script setup lang="ts">
import {RouterLink, RouterView, useRoute} from 'vue-router'
import {ref} from 'vue'
import AppHeader from '@/components/AppHeader.vue'
import {useAppVersion} from '@/composables/useAppVersion'
import {
  Activity,
  ArrowUpCircle,
  Bell,
  Box,
  Globe,
  Heart,
  LayoutGrid,
  Link,
  Menu,
  Shield,
  X,
} from 'lucide-vue-next'

const route = useRoute()
const {version} = useAppVersion()

const mobileMenuOpen = ref(false)

function closeMobileMenu() {
  mobileMenuOpen.value = false
}

const mainNav = [
  {to: '/dashboard', label: 'Dashboard', icon: LayoutGrid},
  {to: '/containers', label: 'Containers', icon: Box},
  {to: '/endpoints', label: 'HTTP Endpoints', icon: Globe},
  {to: '/heartbeats', label: 'Heartbeats', icon: Heart},
  {to: '/certificates', label: 'SSL Certificates', icon: Shield},
  {to: '/updates', label: 'Updates', icon: ArrowUpCircle},
  {to: '/alerts', label: 'Alerts', icon: Bell},
  {to: '/webhooks', label: 'Webhooks', icon: Link},
  {to: '/status-admin', label: 'Status Pages', icon: Activity},
]
</script>

<template>
  <div class="flex h-screen bg-[#0f1115] text-slate-200 antialiased overflow-hidden">
    <!-- Desktop sidebar -->
    <aside
      class="hidden md:flex md:w-64 md:flex-col md:shrink-0 bg-[#151923] border-r border-slate-800"
    >
      <div class="flex flex-col flex-1 overflow-y-auto">
        <!-- Logo -->
        <div class="p-6 flex items-center gap-3 shrink-0">
          <img src="/logo.svg" alt="PulseBoard"
               class="w-8 h-8 rounded-lg shadow-lg shadow-blue-500/25"/>
          <h1 class="text-xl font-bold tracking-tight text-white">PulseBoard</h1>
        </div>

        <!-- Main nav -->
        <nav class="flex-1 px-4 space-y-0.5 overflow-y-auto pb-4">
          <RouterLink
            v-for="item in mainNav"
            :key="item.to"
            :to="item.to"
            class="w-full flex items-center justify-between px-3 py-2 rounded-lg transition-all border group"
            :class="[
              route.path.startsWith(item.to)
                ? 'bg-blue-500/10 text-blue-400 border-blue-500/20'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50 border-transparent',
            ]"
          >
            <div class="flex items-center gap-3">
              <component
                :is="item.icon"
                :size="16"
                class="shrink-0 transition-colors"
                :class="
                  route.path.startsWith(item.to)
                    ? 'text-blue-400'
                    : 'text-slate-500 group-hover:text-slate-300'
                "
              />
              <span class="text-sm font-medium">{{ item.label }}</span>
            </div>
          </RouterLink>
        </nav>

        <!-- Bottom section: Edition Community -->
        <div class="p-4 border-t border-slate-800 space-y-3 shrink-0">
          <div class="bg-slate-800/40 rounded-xl p-3 border border-slate-700/40">
            <div class="flex justify-between items-center mb-2.5">
              <span class="text-[10px] font-bold text-slate-400 uppercase tracking-tighter"
              >Community Edition</span
              >
              <span
                class="text-[10px] bg-blue-500/20 text-blue-400 px-1.5 py-0.5 rounded border border-blue-500/30 font-bold"
              >{{ version }}</span
              >
            </div>
            <RouterLink
              to="/pro"
              class="block w-full py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-xs font-semibold transition-all shadow-lg shadow-blue-500/20 text-center"
            >
              Upgrade to Pro
            </RouterLink>
          </div>
        </div>
      </div>
    </aside>

    <!-- Mobile top bar -->
    <div
      class="md:hidden fixed top-0 left-0 right-0 z-30 flex items-center h-14 px-4 bg-[#151923]/90 backdrop-blur-md border-b border-slate-800"
    >
      <button
        @click="mobileMenuOpen = !mobileMenuOpen"
        class="p-1.5 rounded-md text-slate-400 hover:text-white transition-colors"
        aria-label="Toggle navigation"
      >
        <Menu v-if="!mobileMenuOpen" :size="20"/>
        <X v-else :size="20"/>
      </button>
      <div class="ml-3 flex items-center gap-2">
        <img src="/logo.svg" alt="PulseBoard" class="w-6 h-6 rounded-md"/>
        <span class="text-sm font-bold text-white">PulseBoard</span>
      </div>
      <div class="flex-1"/>
    </div>

    <!-- Mobile overlay -->
    <Transition name="fade">
      <div
        v-if="mobileMenuOpen"
        class="md:hidden fixed inset-0 z-40 bg-black/60 backdrop-blur-sm"
        @click="closeMobileMenu"
      />
    </Transition>

    <!-- Mobile slide-out nav -->
    <Transition name="slide-left">
      <div
        v-if="mobileMenuOpen"
        class="md:hidden fixed inset-y-0 left-0 z-50 w-64 bg-[#151923] border-r border-slate-800 flex flex-col"
      >
        <div class="p-6 flex items-center gap-3">
          <img src="/logo.svg" alt="PulseBoard" class="w-8 h-8 rounded-lg"/>
          <h1 class="text-xl font-bold tracking-tight text-white">PulseBoard</h1>
        </div>
        <nav class="flex-1 px-4 space-y-0.5 overflow-y-auto pb-4">
          <RouterLink
            v-for="item in mainNav"
            :key="item.to"
            :to="item.to"
            class="w-full flex items-center justify-between px-3 py-2 rounded-lg transition-all border"
            :class="[
              route.path.startsWith(item.to)
                ? 'bg-blue-500/10 text-blue-400 border-blue-500/20'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50 border-transparent',
            ]"
            @click="closeMobileMenu"
          >
            <div class="flex items-center gap-3">
              <component :is="item.icon" :size="16" class="shrink-0"/>
              <span class="text-sm font-medium">{{ item.label }}</span>
            </div>
          </RouterLink>
        </nav>
      </div>
    </Transition>

    <!-- Main content -->
    <main class="flex-1 flex flex-col overflow-hidden">
      <AppHeader/>
      <div class="flex-1 overflow-y-auto pt-14 md:pt-0">
        <RouterView v-slot="{ Component }">
          <Suspense>
            <component :is="Component"/>
          </Suspense>
        </RouterView>
      </div>
    </main>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.slide-left-enter-active,
.slide-left-leave-active {
  transition: transform 0.3s ease-out;
}

.slide-left-enter-from,
.slide-left-leave-to {
  transform: translateX(-100%);
}
</style>

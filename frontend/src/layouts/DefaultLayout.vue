<!-- Copyright 2026 Benjamin Touchard (kOlapsis) Licensed under the GNU Affero General Public
License v3.0 (AGPL-3.0) or a commercial license. You may not use this file except in compliance with
one of these licenses. AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html Commercial: See
COMMERCIAL-LICENSE.md Source: https://github.com/kolapsis/maintenant -->

<script setup lang="ts">
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { computed, onMounted, provide, ref } from 'vue'
import AppHeader from '@/components/AppHeader.vue'
import DetailSlideOver from '@/components/DetailSlideOver.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import { useAppVersion } from '@/composables/useAppVersion'
import { useDetailSlideOver, detailSlideOverKey, parseSelectedParam } from '@/composables/useDetailSlideOver'
import { provideConfirm } from '@/composables/useConfirm'
import { useEdition } from '@/composables/useEdition'
import {
  Activity,
  AlertTriangle,
  ArrowUpCircle,
  Bell,
  Box,
  Globe,
  Heart,
  Layers,
  LayoutGrid,
  Link,
  Menu,
  Shield,
  ShieldCheck,
  X,
} from 'lucide-vue-next'

import { useSwarmStore } from '@/stores/swarm'

const route = useRoute()
const router = useRouter()
const { version } = useAppVersion()
const { isEnterprise, hasFeature, licenseMessage, licenseStatusValue, loadLicenseStatus } = useEdition()
const swarmStore = useSwarmStore()

const detailSlideOver = useDetailSlideOver()
provide(detailSlideOverKey, detailSlideOver)

const { state: confirmState } = provideConfirm()

onMounted(() => {
  loadLicenseStatus()
  swarmStore.loadInfo()
  // Parse ?selected=<type>-<id> on initial load
  const parsed = parseSelectedParam(route.query.selected)
  if (parsed) {
    detailSlideOver.openDetail(parsed.type, parsed.id)
  } else if (route.query.selected) {
    // Invalid format — silently remove
    const { selected: _, ...rest } = route.query
    router.replace({ query: rest })
  }
})

const mobileMenuOpen = ref(false)

function closeMobileMenu() {
  mobileMenuOpen.value = false
}

const allNav = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutGrid },
  { to: '/containers', label: 'Containers', icon: Box },
  { to: '/swarm', label: 'Swarm Cluster', icon: Layers, swarmOnly: true },
  { to: '/endpoints', label: 'HTTP Endpoints', icon: Globe },
  { to: '/heartbeats', label: 'Heartbeats', icon: Heart },
  { to: '/certificates', label: 'SSL Certificates', icon: Shield },
  { to: '/updates', label: 'Updates', icon: ArrowUpCircle },
  { to: '/security', label: 'Security Posture', icon: ShieldCheck, feature: 'security_posture' },
  { to: '/alerts', label: 'Alerts', icon: Bell },
  { to: '/webhooks', label: 'Webhooks', icon: Link },
  { to: '/status-admin', label: 'Status Pages', icon: Activity },
]

const mainNav = computed(() => allNav.filter(item => {
  if (item.feature && !hasFeature(item.feature)) return false
  if (item.swarmOnly && !swarmStore.isActive) return false
  return true
}))
</script>

<template>
  <div class="flex h-screen bg-[#0B0E13] text-slate-200 antialiased overflow-hidden">
    <!-- Desktop sidebar -->
    <aside
      class="hidden md:flex md:w-64 md:flex-col md:shrink-0 bg-[#12151C] border-r border-slate-800"
    >
      <div class="flex flex-col flex-1 overflow-y-auto">
        <!-- Logo -->
        <div class="p-6 flex items-center gap-3 shrink-0">
          <img src="/icon.svg" alt="maintenant" />
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
                ? 'bg-pb-green-500/10 text-pb-green-400 border-pb-green-500/20'
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
                    ? 'text-pb-green-400'
                    : 'text-slate-500 group-hover:text-slate-300'
                "
              />
              <span class="text-sm font-medium">{{ item.label }}</span>
            </div>
          </RouterLink>
        </nav>

        <!-- Bottom section: Edition -->
        <div class="p-4 border-t border-slate-800 space-y-3 shrink-0">
          <router-link :to="{ name: 'pro-edition' }">
            <div class="bg-slate-800/40 rounded-xl p-3 border border-slate-700/40">
              <div class="flex justify-between items-center" :class="{ 'mb-2.5': !isEnterprise }">
                <span
                  class="text-[10px] font-bold uppercase tracking-tighter"
                  :class="isEnterprise ? 'text-emerald-400' : 'text-slate-400'"
                  >{{ isEnterprise ? 'Pro Edition' : 'Community Edition' }}</span
                >
                <span
                  class="text-[10px] px-1.5 py-0.5 rounded font-bold"
                  :class="
                    isEnterprise
                      ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                      : 'bg-pb-green-500/20 text-pb-green-400 border border-pb-green-500/30'
                  "
                  >{{ version }}</span
                >
              </div>
              <button
                v-if="!isEnterprise"
                class="cursor-pointer block w-full py-1.5 bg-slate-700/50 text-slate-300 hover:text-slate-200 rounded-lg text-xs font-semibold text-center transition-colors"
              >
                Pro Edition
              </button>
            </div>
          </router-link>
        </div>
      </div>
    </aside>

    <!-- Mobile top bar -->
    <div
      class="md:hidden fixed top-0 left-0 right-0 z-30 flex items-center h-14 px-4 bg-[#12151C]/90 backdrop-blur-md border-b border-slate-800"
    >
      <button
        @click="mobileMenuOpen = !mobileMenuOpen"
        class="p-3 rounded-md text-slate-400 hover:text-white transition-colors"
        aria-label="Toggle navigation"
      >
        <Menu v-if="!mobileMenuOpen" :size="20" />
        <X v-else :size="20" />
      </button>
      <div class="ml-3 flex items-center gap-2">
        <img src="/icon.svg" alt="maintenant" class="w-6 h-6 rounded-md" />
        <span class="text-sm font-bold text-white">maintenant</span>
      </div>
      <div class="flex-1" />
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
        class="md:hidden fixed inset-y-0 left-0 z-50 w-64 bg-[#12151C] border-r border-slate-800 flex flex-col"
      >
        <div class="p-6 flex items-center gap-3">
          <img src="/icon.svg" alt="maintenant" class="w-8 h-8 rounded-lg" />
          <h1 class="text-xl font-bold tracking-tight text-white">maintenant</h1>
        </div>
        <nav class="flex-1 px-4 space-y-0.5 overflow-y-auto pb-4">
          <RouterLink
            v-for="item in mainNav"
            :key="item.to"
            :to="item.to"
            class="w-full flex items-center justify-between px-3 py-2 rounded-lg transition-all border"
            :class="[
              route.path.startsWith(item.to)
                ? 'bg-pb-green-500/10 text-pb-green-400 border-pb-green-500/20'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50 border-transparent',
            ]"
            @click="closeMobileMenu"
          >
            <div class="flex items-center gap-3">
              <component :is="item.icon" :size="16" class="shrink-0" />
              <span class="text-sm font-medium">{{ item.label }}</span>
            </div>
          </RouterLink>
        </nav>
      </div>
    </Transition>

    <!-- Main content -->
    <main class="flex-1 flex flex-col overflow-hidden">
      <!-- License warning banner -->
      <div
        v-if="licenseMessage"
        class="flex items-center gap-2 px-4 py-2 text-xs font-medium shrink-0"
        :class="{
          'bg-amber-500/10 text-amber-400 border-b border-amber-500/20':
            licenseStatusValue === 'grace' || licenseStatusValue === 'unreachable',
          'bg-red-500/10 text-red-400 border-b border-red-500/20':
            licenseStatusValue === 'expired' ||
            licenseStatusValue === 'revoked' ||
            licenseStatusValue === 'unknown',
        }"
      >
        <AlertTriangle :size="14" class="shrink-0" />
        <span>{{ licenseMessage }}</span>
      </div>
      <AppHeader />
      <div class="flex-1 overflow-y-auto pt-14 md:pt-0">
        <RouterView v-slot="{ Component }">
          <Suspense>
            <component :is="Component" />
          </Suspense>
        </RouterView>
      </div>
    </main>

    <!-- Global detail slide-over -->
    <DetailSlideOver />

    <!-- Global confirm dialog -->
    <ConfirmDialog :state="confirmState" />
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

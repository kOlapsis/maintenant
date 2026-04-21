<!-- Copyright 2026 Benjamin Touchard (kOlapsis) Licensed under the GNU Affero General Public
License v3.0 (AGPL-3.0) or a commercial license. You may not use this file except in compliance with
one of these licenses. AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html Commercial: See
COMMERCIAL-LICENSE.md Source: https://github.com/kolapsis/maintenant -->

<script setup lang="ts">
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { computed, onMounted, provide, ref } from 'vue'
import AppHeader from '@/components/AppHeader.vue'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import DetailSlideOver from '@/components/DetailSlideOver.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import ToastContainer from '@/components/ToastContainer.vue'
import { useAppVersion } from '@/composables/useAppVersion'
import {
  useDetailSlideOver,
  detailSlideOverKey,
  parseSelectedParam,
} from '@/composables/useDetailSlideOver'
import { provideConfirm } from '@/composables/useConfirm'
import { useEdition } from '@/composables/useEdition'
import {
  Activity,
  ArrowRight,
  ArrowUpCircle,
  Bell,
  Box,
  Cloud,
  Globe,
  Heart,
  Layers,
  LayoutGrid,
  Link,
  ListChecks,
  Menu,
  Network,
  Server,
  Shield,
  ShieldCheck,
  X,
} from 'lucide-vue-next'

import { useSwarmStore } from '@/stores/swarm'
import { useRuntime } from '@/composables/useRuntime'
import { useRuntimeStore } from '@/stores/runtime'

const route = useRoute()
const router = useRouter()
const { version } = useAppVersion()
const { isEnterprise, hasFeature, licenseMessage, licenseStatusValue, loadLicenseStatus } =
  useEdition()
const swarmStore = useSwarmStore()
const { runtimeContext } = useRuntime()
const runtimeStore = useRuntimeStore()

const detailSlideOver = useDetailSlideOver()
provide(detailSlideOverKey, detailSlideOver)

const { state: confirmState } = provideConfirm()

onMounted(() => {
  loadLicenseStatus()
  swarmStore.loadInfo()
  runtimeStore.fetchStatus()
  runtimeStore.startListening()
  // Parse ?selected=<type>-<id> on initial load
  const parsed = parseSelectedParam(route.query.selected)
  if (parsed) {
    detailSlideOver.openDetail(parsed.type, parsed.id)
  } else if (route.query.selected) {
    // Invalid format — silently remove
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { selected: _, ...rest } = route.query
    router.replace({ query: rest })
  }
})

const licenseMessageParts = computed(() => {
  const msg = licenseMessage.value
  if (!msg) return null
  const match = msg.match(/^(.*?)\b(renew|resubscribe)\b(.*)$/i)
  if (!match || !match[2]) return { before: msg, word: null, after: '' }
  const word = match[2]
  const capitalized = word.charAt(0).toUpperCase() + word.slice(1).toLowerCase()
  return { before: match[1] ?? '', word: capitalized, after: match[3] ?? '' }
})

const licenseSeverity = computed<'warning' | 'critical'>(() => {
  const s = licenseStatusValue.value
  return s === 'grace' || s === 'unreachable' ? 'warning' : 'critical'
})

const licenseLabel = computed(() => {
  switch (licenseStatusValue.value) {
    case 'grace': return 'GRACE PERIOD'
    case 'unreachable': return 'LICENSE UNREACHABLE'
    case 'expired': return 'LICENSE EXPIRED'
    case 'canceled': return 'LICENSE CANCELED'
    case 'revoked': return 'LICENSE REVOKED'
    case 'unknown': return 'LICENSE INVALID'
    default: return 'LICENSE'
  }
})

const mobileMenuOpen = ref(false)

function closeMobileMenu() {
  mobileMenuOpen.value = false
}

interface NavItem {
  to: string
  label: string
  icon: typeof LayoutGrid
  feature?: string
  runtime?: string[]
}

const allNav: NavItem[] = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutGrid },
  // Docker-only
  { to: '/containers', label: 'Containers', icon: Box, runtime: ['docker'] },
  // Swarm Community
  { to: '/services', label: 'Services', icon: Layers, runtime: ['swarm'] },
  { to: '/tasks', label: 'Tasks', icon: ListChecks, runtime: ['swarm'] },
  // K8s Community
  { to: '/workloads', label: 'Workloads', icon: Cloud, runtime: ['kubernetes'] },
  { to: '/pods', label: 'Pods', icon: Box, runtime: ['kubernetes'] },
  // Enterprise: Cluster + Nodes (Swarm & K8s)
  {
    to: '/cluster',
    label: 'Cluster Overview',
    icon: Network,
    runtime: ['swarm', 'kubernetes'],
    feature: 'swarm_dashboard',
  },
  {
    to: '/nodes',
    label: 'Nodes',
    icon: Server,
    runtime: ['swarm', 'kubernetes'],
    feature: 'swarm_dashboard',
  },
  // Always visible
  { to: '/endpoints', label: 'HTTP Endpoints', icon: Globe },
  { to: '/certificates', label: 'SSL Certificates', icon: Shield },
  { to: '/heartbeats', label: 'Heartbeats', icon: Heart },
  { to: '/updates', label: 'Updates', icon: ArrowUpCircle },
  { to: '/security', label: 'Security Posture', icon: ShieldCheck, feature: 'security_posture' },
  { to: '/alerts', label: 'Alerts', icon: Bell },
  { to: '/webhooks', label: 'Webhooks', icon: Link },
  { to: '/status-admin', label: 'Status Pages', icon: Activity },
]

const mainNav = computed(() =>
  allNav.filter((item) => {
    if (item.feature && !hasFeature(item.feature)) return false
    if (item.runtime && !item.runtime.includes(runtimeContext.value)) return false
    return true
  }),
)
</script>

<template>
  <div class="flex h-screen bg-pb-primary text-pb-primary antialiased overflow-hidden">
    <!-- Desktop sidebar -->
    <aside
      class="hidden md:flex md:w-64 md:flex-col md:shrink-0 bg-pb-surface border-r border-slate-800"
    >
      <div class="flex flex-col flex-1 overflow-y-auto">
        <!-- Logo -->
        <div class="p-6 flex items-center gap-3 shrink-0">
          <img src="/icon.svg" alt="" width="32" height="32" />
          <span class="text-sm font-bold text-pb-primary">maintenant</span>
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
                ? 'bg-pb-green-500/10 text-pb-nav-active border-pb-green-500/20'
                : 'text-slate-400 hover:text-pb-primary hover:bg-slate-800/50 border-transparent',
            ]"
          >
            <div class="flex items-center gap-3">
              <component
                :is="item.icon"
                :size="16"
                class="shrink-0 transition-colors"
                :class="
                  route.path.startsWith(item.to)
                    ? 'text-pb-nav-active'
                    : 'text-slate-500 group-hover:text-pb-secondary'
                "
              />
              <span class="text-sm font-medium">{{ item.label }}</span>
            </div>
          </RouterLink>
        </nav>

        <!-- Bottom section: Edition -->
        <div class="p-4 border-t space-y-3 shrink-0" style="border-color: var(--pb-border-default)">
          <router-link :to="{ name: 'pro-edition' }">
            <div class="rounded-xl p-3 border" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default)">
              <div class="flex justify-between items-center" :class="{ 'mb-2.5': !isEnterprise }">
                <span
                  class="text-[10px] font-bold uppercase tracking-tighter"
                  :class="isEnterprise ? 'text-pb-accent' : 'text-pb-secondary'"
                  >{{ isEnterprise ? 'Get Pro Edition' : 'Community Edition' }}</span
                >
                <span
                  class="text-[10px] px-1.5 py-0.5 rounded font-bold"
                  :class="
                    isEnterprise
                      ? 'bg-emerald-500/20 border border-emerald-500/30'
                      : 'border'
                  "
                  :style="isEnterprise
                    ? { color: 'var(--pb-accent)' }
                    : { background: 'var(--pb-bg-surface)', color: 'var(--pb-accent)', borderColor: 'color-mix(in srgb, var(--pb-accent) 40%, transparent)' }"
                  >{{ version }}</span
                >
              </div>
              <button
                v-if="!isEnterprise"
                class="cursor-pointer block w-full py-1.5 rounded-lg text-xs font-semibold text-center transition-colors"
                style="background: var(--pb-bg-surface); color: var(--pb-text-secondary)"
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
      class="md:hidden fixed top-0 left-0 right-0 z-30 flex items-center h-14 px-4 bg-pb-surface/90 backdrop-blur-md border-b border-slate-800"
    >
      <button
        @click="mobileMenuOpen = !mobileMenuOpen"
        class="p-3 rounded-md text-slate-400 hover:text-pb-primary transition-colors"
        aria-label="Toggle navigation"
      >
        <Menu v-if="!mobileMenuOpen" :size="20" />
        <X v-else :size="20" />
      </button>
      <div class="ml-3 flex items-center gap-2">
        <img src="/icon.svg" alt="maintenant" class="w-6 h-6 rounded-md" />
        <span class="text-sm font-bold text-pb-primary">maintenant</span>
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
        class="md:hidden fixed inset-y-0 left-0 z-50 w-64 bg-pb-surface border-r border-slate-800 flex flex-col"
      >
        <div class="p-6 flex items-center gap-3">
          <img src="/icon.svg" alt="maintenant" class="w-8 h-8 rounded-lg" />
          <h1 class="text-xl font-bold tracking-tight text-pb-primary">maintenant</h1>
        </div>
        <nav class="flex-1 px-4 space-y-0.5 overflow-y-auto pb-4">
          <RouterLink
            v-for="item in mainNav"
            :key="item.to"
            :to="item.to"
            class="w-full flex items-center justify-between px-3 py-2 rounded-lg transition-all border"
            :class="[
              route.path.startsWith(item.to)
                ? 'bg-pb-green-500/10 text-pb-nav-active border-pb-green-500/20'
                : 'text-slate-400 hover:text-pb-primary hover:bg-slate-800/50 border-transparent',
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
      <AlertBanner
        v-if="licenseMessageParts"
        :severity="licenseSeverity"
        :label="licenseLabel"
        class="shrink-0"
      >
        {{ licenseMessage }}
        <template v-if="licenseMessageParts.word" #action>
          <RouterLink
            to="/pro-edition"
            class="license-action inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[11px] font-semibold transition-colors"
            :class="`license-action--${licenseSeverity}`"
          >
            {{ licenseMessageParts.word }}
            <ArrowRight :size="12" />
          </RouterLink>
        </template>
      </AlertBanner>
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

    <!-- Toast notifications -->
    <ToastContainer />
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

.license-action--warning {
  background: var(--pb-alert-warn-action-bg);
  border-color: var(--pb-alert-warn-action-border);
  color: var(--pb-alert-warn-action-text);
}
.license-action--warning:hover {
  background: var(--pb-alert-warn-action-hover);
}
.license-action--critical {
  background: var(--pb-alert-critical-action-bg);
  border-color: var(--pb-alert-critical-action-border);
  color: var(--pb-alert-critical-action-text);
}
.license-action--critical:hover {
  background: var(--pb-alert-critical-action-hover);
}
</style>

<!-- Copyright 2026 Benjamin Touchard (kOlapsis) Licensed under the GNU Affero General Public
License v3.0 (AGPL-3.0) or a commercial license. You may not use this file except in compliance with
one of these licenses. AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html Commercial: See
COMMERCIAL-LICENSE.md Source: https://github.com/kolapsis/maintenant -->

<script setup lang="ts">
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { computed, onMounted, provide, ref, watch } from 'vue'
import AppHeader from '@/components/AppHeader.vue'
import EditionBadge from '@/components/EditionBadge.vue'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import DetailSlideOver from '@/components/DetailSlideOver.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import ToastContainer from '@/components/ToastContainer.vue'
import { useAppVersion } from '@/composables/useAppVersion'
import { useStorageStore } from '@/stores/storage'
import {
  detailSlideOverKey,
  parseSelectedParam,
  useDetailSlideOver,
} from '@/composables/useDetailSlideOver'
import { provideConfirm } from '@/composables/useConfirm'
import { useEdition } from '@/composables/useEdition'
import { useEditionBanner } from '@/composables/useEditionBanner'
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
  MonitorDot,
  Network,
  Send,
  Server,
  Shield,
  ShieldCheck,
  Workflow,
  X,
} from 'lucide-vue-next'

import { useSwarmStore } from '@/stores/swarm'
import { useRuntimeStore } from '@/stores/runtime'
import { useResourcesStore } from '@/stores/resources'
import { useFleetRuntimes } from '@/composables/useFleetRuntimes'
import HostFilterDropdown from '@/components/HostFilterDropdown.vue'

const route = useRoute()
const router = useRouter()
const { version } = useAppVersion()
const {
  isCommunity,
  editionName,
  hasFeature,
  licenseMessage,
  licenseSeverity,
  licenseLabel,
  loadLicenseStatus,
} = useEdition()
const swarmStore = useSwarmStore()
const runtimeStore = useRuntimeStore()
const storageStore = useStorageStore()
const resources = useResourcesStore()
const { availableRuntimes } = useFleetRuntimes()

const detailSlideOver = useDetailSlideOver()
provide(detailSlideOverKey, detailSlideOver)

const { state: confirmState } = provideConfirm()

onMounted(() => {
  loadLicenseStatus()
  swarmStore.loadInfo()
  runtimeStore.fetchStatus()
  runtimeStore.startListening()
  storageStore.fetchStatus()
  storageStore.startListening()
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

const editionBanner = useEditionBanner()

const mobileMenuOpen = ref(false)

function closeMobileMenu() {
  mobileMenuOpen.value = false
}

interface NavItem {
  type: string
  to?: string
  label?: string
  icon?: typeof LayoutGrid
  feature?: string
  runtime?: string[]
}

const allNav: NavItem[] = [
  { type: 'item', to: '/dashboard', label: 'Dashboard', icon: LayoutGrid },
  // Pro: Cluster (Swarm & K8s)
  {
    type: 'item',
    to: '/cluster',
    label: 'Cluster Overview',
    icon: Network,
    runtime: ['swarm', 'kubernetes'],
    feature: 'swarm_dashboard',
  },
  { type: 'separator' },
  // Docker-only
  { type: 'item', to: '/containers', label: 'Containers', icon: Box, runtime: ['docker'] },
  // Swarm Community
  { type: 'item', to: '/services', label: 'Services', icon: Layers, runtime: ['swarm'] },
  { type: 'item', to: '/tasks', label: 'Tasks', icon: ListChecks, runtime: ['swarm'] },
  // K8s Community
  { type: 'item', to: '/workloads', label: 'Workloads', icon: Cloud, runtime: ['kubernetes'] },
  { type: 'item', to: '/pods', label: 'Pods', icon: Box, runtime: ['kubernetes'] },
  // Pro: Nodes (Swarm & K8s)
  {
    type: 'item',
    to: '/nodes',
    label: 'Nodes',
    icon: Server,
    runtime: ['swarm', 'kubernetes'],
    feature: 'swarm_dashboard',
  },
  // Always visible
  { type: 'item', to: '/endpoints', label: 'HTTP Endpoints', icon: Globe },
  { type: 'item', to: '/certificates', label: 'SSL Certificates', icon: Shield },
  { type: 'item', to: '/heartbeats', label: 'Heartbeats', icon: Heart },
  { type: 'separator' },
  { type: 'item', to: '/updates', label: 'Updates', icon: ArrowUpCircle },
  { type: 'item', to: '/security', label: 'Security Posture', icon: ShieldCheck },
  { type: 'separator' },
  { type: 'item', to: '/channels', label: 'Channels', icon: Send },
  { type: 'item', to: '/alerts', label: 'Alerts', icon: Bell },
  { type: 'item', to: '/escalation', label: 'Escalation', icon: Workflow },
  { type: 'item', to: '/webhooks', label: 'Webhooks', icon: Link },
  { type: 'separator' },
  { type: 'item', to: '/status-admin', label: 'Status Pages', icon: Activity },
  { type: 'item', to: '/agents', label: 'Agents', icon: MonitorDot },
]

const mainNav = computed(() =>
  allNav.filter((item) => {
    if (item.feature && !hasFeature(item.feature)) return false
    // Runtime-specific items show when the selected host scope offers that
    // runtime: in "all" the union of every agent + local runtime, otherwise the
    // single selected scope's runtime.
    if (item.runtime && !item.runtime.some((rt) => availableRuntimes.value.includes(rt))) {
      return false
    }
    return true
  }),
)

// When the user switches host scope, a runtime-specific page they are on may no
// longer apply (e.g. selecting a Docker agent while viewing /workloads). Send
// them back to the dashboard rather than leaving a stale, empty view. Watches
// the explicit selection (not availableRuntimes) so initial deep-links are not
// redirected before the fleet has loaded.
watch(
  () => resources.selected,
  () => {
    const current = allNav.find((i) => i.type === 'item' && i.to && route.path.startsWith(i.to))
    if (current?.runtime && !current.runtime.some((rt) => availableRuntimes.value.includes(rt))) {
      router.replace('/dashboard')
    }
  },
)
</script>

<template>
  <div class="flex h-screen bg-mnt-primary text-mnt-primary antialiased overflow-hidden">
    <!-- Desktop sidebar -->
    <aside
      class="hidden md:flex md:w-64 md:flex-col md:shrink-0 bg-mnt-surface border-r border-mnt-default"
    >
      <div class="flex flex-col flex-1 overflow-y-auto">
        <!-- Logo -->
        <div class="p-6 flex items-center gap-3 shrink-0">
          <img src="/icon.svg" alt="" width="32" height="32" />
          <span class="text-sm font-bold text-mnt-primary">maintenant</span>
        </div>

        <!-- Main nav -->
        <nav class="flex-1 px-4 space-y-0.5 overflow-y-auto pb-4">
          <template v-for="item in mainNav" :key="item.to">
            <hr v-if="item.type === 'separator'" class="my-1.5 border-mnt-subtle" />
            <RouterLink
              v-else-if="item.type === 'item'"
              :to="item.to!"
              class="w-full flex items-center justify-between px-3 py-1.5 rounded-lg transition-all border group"
              :class="[
                route.path.startsWith(item.to!)
                  ? 'bg-mnt-green-500/10 text-mnt-nav-active border-mnt-green-500/20'
                  : 'text-mnt-muted hover:text-mnt-primary hover:bg-mnt-elevated border-transparent',
              ]"
            >
              <div class="flex items-center gap-3">
                <component
                  :is="item.icon"
                  :size="16"
                  class="shrink-0 transition-colors"
                  :class="
                    route.path.startsWith(item.to!)
                      ? 'text-mnt-nav-active'
                      : 'text-mnt-muted group-hover:text-mnt-secondary'
                  "
                />
                <span class="text-sm font-medium">{{ item.label }}</span>
              </div>
            </RouterLink>
          </template>
        </nav>

        <!-- Bottom section: Edition -->
        <div class="p-4 border-t space-y-3 shrink-0" style="border-color: var(--mnt-border-default)">
          <router-link :to="{ name: 'editions' }">
            <div
              class="rounded-xl p-3 border"
              style="background: var(--mnt-bg-elevated); border-color: var(--mnt-border-default)"
            >
              <div class="flex justify-between items-center" :class="{ 'mb-2.5': isCommunity }">
                <!-- Three visually distinct states; Personal is never shown as
                     a free tier, and an unknown edition still renders (FR-041). -->
                <EditionBadge :edition="editionName" />
                <span
                  class="text-[10px] px-1.5 py-0.5 rounded font-bold border"
                  :style="{
                    background: 'var(--mnt-bg-surface)',
                    color: 'var(--mnt-accent)',
                    borderColor: 'color-mix(in srgb, var(--mnt-accent) 40%, transparent)',
                  }"
                  >{{ version }}</span
                >
              </div>
              <button
                v-if="isCommunity"
                class="cursor-pointer block w-full py-1.5 rounded-lg text-xs font-semibold text-center transition-colors"
                style="background: var(--mnt-bg-surface); color: var(--mnt-text-secondary)"
              >
                Compare editions
              </button>
            </div>
          </router-link>
        </div>
      </div>
    </aside>

    <!-- Mobile top bar -->
    <div
      class="mobile-bar md:hidden fixed top-0 left-0 right-0 z-30 flex items-center h-14 px-4 backdrop-blur-md border-b border-mnt-default"
    >
      <button
        @click="mobileMenuOpen = !mobileMenuOpen"
        class="p-3 rounded-md text-mnt-muted hover:text-mnt-primary transition-colors"
        aria-label="Toggle navigation"
      >
        <Menu v-if="!mobileMenuOpen" :size="20" />
        <X v-else :size="20" />
      </button>
      <div class="ml-3 flex items-center gap-2">
        <img src="/icon.svg" alt="maintenant" class="w-6 h-6 rounded-md" />
        <span class="text-sm font-bold text-mnt-primary">maintenant</span>
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
        class="md:hidden fixed inset-y-0 left-0 z-50 w-64 bg-mnt-surface border-r border-mnt-default flex flex-col"
      >
        <div class="p-6 flex items-center gap-3">
          <img src="/icon.svg" alt="maintenant" class="w-8 h-8 rounded-lg" />
          <h1 class="text-xl font-bold tracking-tight text-mnt-primary">maintenant</h1>
        </div>
        <div class="px-4 pb-2 shrink-0">
          <HostFilterDropdown />
        </div>
        <nav class="flex-1 px-4 space-y-0.5 overflow-y-auto pb-4">
          <template v-for="item in mainNav" :key="item.to">
            <hr v-if="item.type === 'separator'" class="my-1.5 border-mnt-subtle" />
            <RouterLink
              v-else-if="item.type === 'item'"
              :to="item.to!"
              class="w-full flex items-center justify-between px-3 py-2 rounded-lg transition-all border"
              :class="[
                route.path.startsWith(item.to!)
                  ? 'bg-mnt-green-500/10 text-mnt-nav-active border-mnt-green-500/20'
                  : 'text-mnt-muted hover:text-mnt-primary hover:bg-mnt-elevated border-transparent',
              ]"
              @click="closeMobileMenu"
            >
              <div class="flex items-center gap-3">
                <component :is="item.icon" :size="16" class="shrink-0" />
                <span class="text-sm font-medium">{{ item.label }}</span>
              </div>
            </RouterLink>
          </template>
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
            to="/editions"
            class="license-action inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[11px] font-semibold transition-colors"
            :class="`license-action--${licenseSeverity}`"
          >
            {{ licenseMessageParts.word }}
            <ArrowRight :size="12" />
          </RouterLink>
        </template>
      </AlertBanner>
      <!-- Upsell banner: Community only, segmented by container count -->
      <AlertBanner
        v-if="editionBanner.visible.value"
        severity="info"
        label="EDITIONS"
        dismissible
        class="shrink-0"
        @dismiss="editionBanner.dismiss()"
      >
        <!-- Tier 1 leads with Personal: at this size it is most likely a homelab,
             and Personal is what removes the friction they just hit. -->
        <template v-if="editionBanner.tier.value === 1">
          Hitting the Community limits? Personal lifts them all and monitors up to 20 machines,
          for €149 once, for life.
        </template>
        <!-- Tier 2 names both: the size no longer tells you which one fits. -->
        <template v-else-if="editionBanner.tier.value === 2">
          You're monitoring {{ editionBanner.count.value }} containers. Personal removes every limit
          for €149 once. Pro adds the team layer (Slack, escalation, subscribers) at €29/mo.
        </template>
        <!-- Tier 3 stays Pro-only: at this scale it is a team, or someone
             else's infrastructure, which Personal does not cover. -->
        <template v-else-if="editionBanner.tier.value === 3">
          Running 50+ containers in production. Pro adds incident management, alert escalation and
          Slack routing. Want to discuss your setup with the founder?
        </template>
        <template #action>
          <template v-if="editionBanner.tier.value === 1 || editionBanner.tier.value === 2">
            <RouterLink
              to="/editions"
              class="license-action license-action--info inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[11px] font-semibold transition-colors"
            >
              Compare editions →
            </RouterLink>
          </template>
          <template v-else-if="editionBanner.tier.value === 3">
            <a
              href="mailto:benjamin@kolapsis.com?subject=Maintenant%20-%2050%2B%20containers%20setup"
              class="license-action license-action--info inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[11px] font-semibold transition-colors"
            >
              Reply →
            </a>
          </template>
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
  background: var(--mnt-alert-warn-action-bg);
  border-color: var(--mnt-alert-warn-action-border);
  color: var(--mnt-alert-warn-action-text);
}
.license-action--warning:hover {
  background: var(--mnt-alert-warn-action-hover);
}
.license-action--critical {
  background: var(--mnt-alert-critical-action-bg);
  border-color: var(--mnt-alert-critical-action-border);
  color: var(--mnt-alert-critical-action-text);
}
.license-action--critical:hover {
  background: var(--mnt-alert-critical-action-hover);
}
.license-action--info {
  background: var(--mnt-alert-info-action-bg);
  border-color: var(--mnt-alert-info-action-border);
  color: var(--mnt-alert-info-action-text);
}
.license-action--info:hover {
  background: var(--mnt-alert-info-action-hover);
}

/* `bg-mnt-*` are hand-written utilities: Tailwind's `/90` opacity modifier
   does not apply to them, so mix the token here. */
.mobile-bar {
  background-color: color-mix(in srgb, var(--mnt-bg-surface) 90%, transparent);
}
</style>

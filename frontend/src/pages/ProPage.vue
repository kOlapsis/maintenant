<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import {
  AlertTriangle,
  Bell,
  Check,
  Clock,
  Crown,
  Heart,
  Layers,
  Mail,
  MessageSquare,
  RefreshCw,
  Shield,
  TrendingUp,
  Zap,
  ExternalLink,
} from 'lucide-vue-next'
import { useEdition } from '@/composables/useEdition'
import InlineAlert from '@/components/ui/InlineAlert.vue'

const { isEnterprise, licenseStatusValue, licenseMessage, loadLicenseStatus } = useEdition()

onMounted(() => {
  loadLicenseStatus()
})

const isLicenseActive = computed(
  () => isEnterprise.value && licenseStatusValue.value === 'active',
)

const hasLicenseIssue = computed(
  () => isEnterprise.value && licenseStatusValue.value !== '' && licenseStatusValue.value !== 'active',
)

const isProDisabled = computed(() => {
  const s = licenseStatusValue.value
  return s === 'expired' || s === 'canceled' || s === 'revoked' || s === 'unknown'
})

const issueSeverity = computed<'warning' | 'critical'>(() => {
  const s = licenseStatusValue.value
  return s === 'grace' || s === 'unreachable' ? 'warning' : 'critical'
})

const issueTag = computed(() => {
  switch (licenseStatusValue.value) {
    case 'grace': return 'GRACE PERIOD'
    case 'unreachable': return 'UNREACHABLE'
    case 'expired': return 'EXPIRED'
    case 'canceled': return 'CANCELED'
    case 'revoked': return 'REVOKED'
    case 'unknown': return 'INVALID'
    default: return 'LICENSE'
  }
})

const issueTitle = computed(() => {
  switch (licenseStatusValue.value) {
    case 'grace': return 'Your license needs renewal'
    case 'unreachable': return 'License server unreachable'
    case 'expired': return 'Your license has expired'
    case 'canceled': return 'Your subscription was canceled'
    case 'revoked': return 'Your license has been revoked'
    case 'unknown': return 'License could not be validated'
    default: return 'License requires attention'
  }
})

const heroTitle = computed(() => {
  if (isLicenseActive.value) return "You're on the Pro Edition"
  if (hasLicenseIssue.value) {
    return isProDisabled.value
      ? 'Restore your Pro Edition'
      : 'Action required on your Pro license'
  }
  return 'Unlock the full power of maintenant'
})

const heroSubtitle = computed(() => {
  if (isLicenseActive.value || !hasLicenseIssue.value) {
    return 'Everything in Community, plus incident management, advanced notifications, vulnerability intelligence, Docker Swarm cluster intelligence, and extended resource history.'
  }
  if (isProDisabled.value) {
    return 'Pro features are currently disabled. Resubscribe below to regain access, or contact support if this looks wrong.'
  }
  return 'Pro features are still available for now. Renew or update your billing details to avoid any interruption.'
})

const CHECKOUT_URL = 'https://maintenant.dev/checkout/'
const ACCOUNT_URL = 'https://maintenant.dev/account'

const features = [
  {
    icon: AlertTriangle,
    title: 'Incident Management',
    description:
      'Real-time incident tracking with timeline, severity levels, and coordinated communication.',
  },
  {
    icon: Clock,
    title: 'Maintenance Windows',
    description:
      'Schedule maintenance periods with automatic notifications and status page updates.',
  },
  {
    icon: Bell,
    title: 'Subscriber Notifications',
    description: 'Let users subscribe to status updates and receive alerts when incidents occur.',
  },
  {
    icon: MessageSquare,
    title: 'Slack & Teams',
    description: 'Push alerts to Slack channels and Microsoft Teams for instant team awareness.',
  },
  {
    icon: Shield,
    title: 'Risk Scoring & CVE Enrichment',
    description:
      'Vulnerability intelligence for container updates with severity scoring and CVE details.',
  },
  {
    icon: TrendingUp,
    title: 'Resource History',
    description:
      'Track CPU, memory, and disk trends over 24h, 7d, and 30d to spot resource drift early.',
  },
  {
    icon: Layers,
    title: 'Swarm Cluster Dashboard',
    description:
      'Cluster-wide view with node topology, service health, task distribution, and replica status.',
  },
  {
    icon: Zap,
    title: 'Swarm Intelligence',
    description:
      'Crash-loop detection, rolling update tracking, node health alerting, and replica health alerts.',
  },
]
</script>

<template>
  <div class="min-h-full">
    <div class="max-w-5xl mx-auto px-6 py-12">
      <!-- Hero (always shown) -->
      <div class="text-center mb-16">
        <div
          class="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-pb-green-500/10 border border-pb-green-500/20 text-pb-green-400 text-xs font-semibold mb-6"
        >
          <Crown :size="14" />
          Pro Edition
        </div>
        <h1 class="text-4xl font-bold text-pb-primary tracking-tight mb-4">{{ heroTitle }}</h1>
        <p class="text-lg text-slate-400 max-w-2xl mx-auto leading-relaxed">{{ heroSubtitle }}</p>
      </div>

      <!-- Active license: thank you + feature reminder -->
      <template v-if="isLicenseActive">
        <div class="max-w-2xl mx-auto mb-10">
          <div
            class="flex items-start gap-4 bg-pb-green-500/5 border border-pb-green-500/20 rounded-xl px-6 py-5"
          >
            <Heart :size="20" class="text-pb-green-400 shrink-0 mt-0.5" />
            <div>
              <h2 class="text-base font-semibold text-pb-primary mb-1">Thank you for your support</h2>
              <p class="text-sm text-slate-400 leading-relaxed">
                Your Pro license is active. Thank you for supporting the development of maintenant —
                it makes a real difference.
              </p>
              <p class="mt-2 text-sm text-slate-400">
                Manage your subscription from your
                <a
                  :href="ACCOUNT_URL"
                  target="_blank"
                  rel="noopener"
                  class="text-pb-green-400 hover:underline"
                  >maintenant.dev account</a
                >.
              </p>
            </div>
          </div>
        </div>

        <div class="max-w-2xl mx-auto">
          <h3 class="text-xs font-medium text-slate-500 uppercase tracking-wider mb-3">
            Included in your plan
          </h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <div v-for="feature in features" :key="feature.title" class="flex items-center gap-2.5">
              <Check :size="14" class="text-pb-green-500/60 shrink-0" />
              <span class="text-sm text-slate-400">{{ feature.title }}</span>
            </div>
            <div class="flex items-center gap-2.5">
              <Check :size="14" class="text-pb-green-500/60 shrink-0" />
              <span class="text-sm text-slate-400">All Community features</span>
            </div>
          </div>
        </div>
      </template>

      <!-- Enterprise with license issue -->
      <template v-else-if="hasLicenseIssue">
        <div class="max-w-2xl mx-auto mb-10 space-y-4">
          <InlineAlert :severity="issueSeverity" :tag="issueTag">
            <template #title>{{ issueTitle }}</template>
            <p v-if="licenseMessage" class="leading-relaxed">{{ licenseMessage }}</p>
            <p v-else class="leading-relaxed">
              Your license is no longer in a fully active state. Review the options below to restore
              it.
            </p>
          </InlineAlert>

          <div class="flex flex-col sm:flex-row sm:items-center gap-3">
            <a
              :href="CHECKOUT_URL"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center justify-center gap-2 px-5 py-2.5 rounded-lg text-sm font-semibold bg-pb-green-600 hover:bg-pb-green-500 text-slate-950 shadow-lg shadow-pb-green-500/20 transition-colors"
            >
              <RefreshCw :size="15" />
              {{ isProDisabled ? 'Resubscribe to Pro' : 'Renew subscription' }}
            </a>
            <a
              :href="ACCOUNT_URL"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center gap-1.5 px-2 py-1 text-sm text-slate-400 hover:text-pb-primary transition-colors"
            >
              Manage subscription
              <ExternalLink :size="13" class="opacity-70" />
            </a>
          </div>

          <p class="text-xs text-slate-500 leading-relaxed">
            Questions or think this is a mistake? Reach out at
            <a
              href="mailto:support@maintenant.dev"
              class="text-slate-400 hover:text-pb-primary underline underline-offset-2"
              >support@maintenant.dev</a
            >.
          </p>
        </div>

        <div class="max-w-2xl mx-auto">
          <h3 class="text-xs font-medium text-slate-500 uppercase tracking-wider mb-3">
            {{ isProDisabled ? 'Features unlocked by Pro' : 'Included in your plan' }}
          </h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <div v-for="feature in features" :key="feature.title" class="flex items-center gap-2.5">
              <Check
                :size="14"
                class="shrink-0"
                :class="isProDisabled ? 'text-slate-600' : 'text-pb-green-500/60'"
              />
              <span
                class="text-sm"
                :class="isProDisabled ? 'text-slate-500' : 'text-slate-400'"
                >{{ feature.title }}</span
              >
            </div>
            <div class="flex items-center gap-2.5">
              <Check
                :size="14"
                class="shrink-0"
                :class="isProDisabled ? 'text-slate-600' : 'text-pb-green-500/60'"
              />
              <span
                class="text-sm"
                :class="isProDisabled ? 'text-slate-500' : 'text-slate-400'"
                >All Community features</span
              >
            </div>
          </div>
        </div>
      </template>

      <!-- No license: feature grid + pricing -->
      <template v-else>
        <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-16">
          <div
            v-for="feature in features"
            :key="feature.title"
            class="group bg-pb-surface border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors"
          >
            <div
              class="w-9 h-9 rounded-lg bg-pb-green-500/10 border border-pb-green-500/20 flex items-center justify-center mb-3"
            >
              <component :is="feature.icon" :size="18" class="text-pb-green-400" />
            </div>
            <h3 class="text-sm font-semibold text-pb-primary mb-1.5">{{ feature.title }}</h3>
            <p class="text-xs text-slate-400 leading-relaxed">{{ feature.description }}</p>
          </div>
        </div>

        <div class="max-w-2xl mx-auto">
          <h2 class="text-2xl font-bold text-pb-primary text-center mb-2">Simple pricing</h2>
          <p class="text-sm text-slate-400 text-center mb-8">
            One plan, all Pro features. No per-seat or per-host charges. From 9€/month.
          </p>

          <div class="flex flex-col items-center gap-3 mb-6">
            <a
              :href="CHECKOUT_URL"
              target="_blank"
              rel="noopener"
              class="inline-flex items-center gap-2 px-8 py-3 rounded-lg text-sm font-semibold bg-pb-green-600 hover:bg-pb-green-500 text-slate-950 shadow-lg shadow-pb-green-500/20 transition-colors"
            >
              <Crown :size="16" />
              Upgrade to Pro
            </a>
            <p class="text-xs text-slate-500">Choose monthly or yearly on the next screen.</p>
          </div>

          <div
            class="mt-6 flex items-start gap-3 bg-blue-500/5 border border-blue-500/20 rounded-xl px-5 py-4"
          >
            <Mail :size="18" class="text-blue-400 shrink-0 mt-0.5" />
            <p class="text-sm text-pb-secondary leading-relaxed">
              After purchase, your license key will be sent to the email address provided during
              checkout.
            </p>
          </div>

          <div class="mt-8 bg-pb-surface border border-slate-800 rounded-xl p-6">
            <h3 class="text-sm font-semibold text-pb-primary mb-4">Everything in Pro includes:</h3>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
              <div
                v-for="feature in features"
                :key="feature.title"
                class="flex items-center gap-2.5"
              >
                <Check :size="14" class="text-pb-green-400 shrink-0" />
                <span class="text-sm text-pb-secondary">{{ feature.title }}</span>
              </div>
              <div class="flex items-center gap-2.5">
                <Check :size="14" class="text-pb-green-400 shrink-0" />
                <span class="text-sm text-pb-secondary">All Community features</span>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

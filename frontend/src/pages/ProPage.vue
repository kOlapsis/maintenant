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
import { computed, onMounted, ref } from 'vue'
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
  Shield,
  TrendingUp,
  Zap,
} from 'lucide-vue-next'
import { useEdition } from '@/composables/useEdition'

const { isEnterprise, licenseStatusValue, loadLicenseStatus } = useEdition()

onMounted(() => {
  loadLicenseStatus()
})

const isLicenseActive = computed(
  () =>
    isEnterprise.value &&
    (licenseStatusValue.value === 'active' || licenseStatusValue.value === 'grace'),
)

const billingPeriod = ref<'monthly' | 'annual'>('annual')

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
  /*{
    icon: Mail,
    title: 'SMTP Configuration',
    description: 'Use your own mail server for branded notifications and full delivery control.',
  },*/
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
        <h1 class="text-4xl font-bold text-pb-primary tracking-tight mb-4">
          <template v-if="isLicenseActive"> You're on the Pro Edition </template>
          <template v-else> Unlock the full power of maintenant </template>
        </h1>
        <p class="text-lg text-slate-400 max-w-2xl mx-auto leading-relaxed">
          Everything in Community, plus incident management, advanced notifications, vulnerability
          intelligence, Docker Swarm cluster intelligence, and extended resource history.
        </p>
      </div>

      <!-- Active license: thank you + feature reminder -->
      <template v-if="isLicenseActive">
        <!-- Thank you message -->
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
              <p>
                Manager your license directly on
                <a href="https://billing.stripe.com/p/login/8x28wO6dVdVd9UggHX33W00" target="_blank"
                  >stripe website</a
                >
              </p>
            </div>
          </div>
        </div>

        <!-- Discreet feature reminder -->
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

      <!-- No license: feature grid + pricing -->
      <template v-else>
        <!-- Feature grid -->
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

        <!-- Pricing -->
        <div class="max-w-2xl mx-auto">
          <h2 class="text-2xl font-bold text-pb-primary text-center mb-2">Simple pricing</h2>
          <p class="text-sm text-slate-400 text-center mb-8">
            One plan, all Pro features. No per-seat or per-host charges.
          </p>

          <!-- Toggle -->
          <div class="flex items-center justify-center gap-3 mb-8">
            <button
              class="text-sm font-medium transition-colors"
              :class="
                billingPeriod === 'monthly' ? 'text-pb-primary' : 'text-slate-500 hover:text-pb-secondary'
              "
              @click="billingPeriod = 'monthly'"
            >
              Monthly
            </button>
            <button
              class="relative w-11 h-6 rounded-full transition-colors"
              :class="billingPeriod === 'annual' ? 'bg-pb-green-600' : 'bg-slate-700'"
              @click="billingPeriod = billingPeriod === 'monthly' ? 'annual' : 'monthly'"
            >
              <span
                class="absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full transition-transform"
                :class="billingPeriod === 'annual' ? 'translate-x-5' : ''"
              />
            </button>
            <button
              class="text-sm font-medium transition-colors flex items-center gap-1.5"
              :class="
                billingPeriod === 'annual' ? 'text-pb-primary' : 'text-slate-500 hover:text-pb-secondary'
              "
              @click="billingPeriod = 'annual'"
            >
              Annual
              <span
                class="text-[10px] font-bold bg-emerald-500/15 text-emerald-400 px-1.5 py-0.5 rounded border border-emerald-500/25"
              >
                Save 18€
              </span>
            </button>
          </div>

          <!-- Pricing cards -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <!-- Monthly -->
            <div
              class="relative bg-pb-surface border rounded-xl p-6 transition-colors"
              :class="
                billingPeriod === 'monthly'
                  ? 'border-pb-green-500/40'
                  : 'border-slate-800 opacity-60'
              "
            >
              <h3 class="text-sm font-semibold text-pb-secondary mb-3">Monthly</h3>
              <div class="flex items-baseline gap-1 mb-4">
                <span class="text-3xl font-bold text-pb-primary">9€</span>
                <span class="text-sm text-slate-500">/month</span>
              </div>
              <a
                href="https://buy.stripe.com/8x28wO6dVdVd9UggHX33W00"
                target="_blank"
                class="block w-full py-2 text-center rounded-lg text-sm font-semibold transition-colors"
                :class="
                  billingPeriod === 'monthly'
                    ? 'bg-pb-green-600 hover:bg-pb-green-500 text-slate-950 shadow-lg shadow-pb-green-500/20'
                    : 'bg-slate-800 text-slate-400'
                "
              >
                Get started
              </a>
            </div>

            <!-- Annual -->
            <div
              class="relative bg-pb-surface border rounded-xl p-6 transition-colors"
              :class="
                billingPeriod === 'annual'
                  ? 'border-pb-green-500/40'
                  : 'border-slate-800 opacity-60'
              "
            >
              <span
                v-if="billingPeriod === 'annual'"
                class="absolute -top-2.5 right-4 text-[10px] font-bold bg-pb-green-600 text-slate-950 px-2 py-0.5 rounded"
              >
                Recommended
              </span>
              <h3 class="text-sm font-semibold text-pb-secondary mb-3">Annual</h3>
              <div class="flex items-baseline gap-1 mb-1">
                <span class="text-3xl font-bold text-pb-primary">90€</span>
                <span class="text-sm text-slate-500">/year</span>
              </div>
              <p class="text-xs text-emerald-400 mb-3">7.50€/month — save 18€</p>
              <a
                href="https://buy.stripe.com/3cI28qgSz6sL9Ug63j33W01"
                target="_blank"
                class="block w-full py-2 text-center rounded-lg text-sm font-semibold transition-colors"
                :class="
                  billingPeriod === 'annual'
                    ? 'bg-pb-green-600 hover:bg-pb-green-500 text-slate-950 shadow-lg shadow-pb-green-500/20'
                    : 'bg-slate-800 text-slate-400'
                "
              >
                Get started
              </a>
            </div>
          </div>

          <!-- License delivery notice -->
          <div
            class="mt-6 flex items-start gap-3 bg-blue-500/5 border border-blue-500/20 rounded-xl px-5 py-4"
          >
            <Mail :size="18" class="text-blue-400 shrink-0 mt-0.5" />
            <p class="text-sm text-pb-secondary leading-relaxed">
              After purchase, your license key will be sent to the email address provided during
              checkout.
            </p>
          </div>

          <!-- Feature checklist -->
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

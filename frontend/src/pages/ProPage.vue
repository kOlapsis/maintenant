<script setup lang="ts">
import {ref} from 'vue'
import {
  Sparkles,
  Shield,
  Bell,
  Mail,
  MessageSquare,
  TrendingUp,
  Clock,
  AlertTriangle,
  Check,
  Crown,
} from 'lucide-vue-next'

const billingPeriod = ref<'monthly' | 'annual'>('annual')

const features = [
  {
    icon: AlertTriangle,
    title: 'Incident Management',
    description: 'Real-time incident tracking with timeline, severity levels, and coordinated communication.',
  },
  {
    icon: Clock,
    title: 'Maintenance Windows',
    description: 'Schedule maintenance periods with automatic notifications and status page updates.',
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
    description: 'Vulnerability intelligence for container updates with severity scoring and CVE details.',
  },
  {
    icon: TrendingUp,
    title: 'Resource History',
    description: 'Track CPU, memory, and disk trends over 24h, 7d, and 30d to spot resource drift early.',
  },
]
</script>

<template>
  <div class="min-h-full">
    <div class="max-w-5xl mx-auto px-6 py-12">
      <!-- Hero -->
      <div class="text-center mb-16">
        <div
          class="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-blue-500/10 border border-blue-500/20 text-blue-400 text-xs font-semibold mb-6"
        >
          <Crown :size="14" />
          Pro Edition
        </div>
        <h1 class="text-4xl font-bold text-white tracking-tight mb-4">
          Unlock the full power of PulseBoard
        </h1>
        <p class="text-lg text-slate-400 max-w-2xl mx-auto leading-relaxed">
          Everything in Community, plus incident management, advanced notifications,
          vulnerability intelligence, and extended resource history.
        </p>
      </div>

      <!-- Feature grid -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-16">
        <div
          v-for="feature in features"
          :key="feature.title"
          class="group bg-[#151923] border border-slate-800 rounded-xl p-5 hover:border-slate-700 transition-colors"
        >
          <div
            class="w-9 h-9 rounded-lg bg-blue-500/10 border border-blue-500/20 flex items-center justify-center mb-3"
          >
            <component :is="feature.icon" :size="18" class="text-blue-400" />
          </div>
          <h3 class="text-sm font-semibold text-white mb-1.5">{{ feature.title }}</h3>
          <p class="text-xs text-slate-400 leading-relaxed">{{ feature.description }}</p>
        </div>
      </div>

      <!-- Pricing -->
      <div class="max-w-2xl mx-auto">
        <h2 class="text-2xl font-bold text-white text-center mb-2">Simple pricing</h2>
        <p class="text-sm text-slate-400 text-center mb-8">
          One plan, all Pro features. No per-seat or per-host charges.
        </p>

        <!-- Toggle -->
        <div class="flex items-center justify-center gap-3 mb-8">
          <button
            class="text-sm font-medium transition-colors"
            :class="billingPeriod === 'monthly' ? 'text-white' : 'text-slate-500 hover:text-slate-300'"
            @click="billingPeriod = 'monthly'"
          >
            Monthly
          </button>
          <button
            class="relative w-11 h-6 rounded-full transition-colors"
            :class="billingPeriod === 'annual' ? 'bg-blue-600' : 'bg-slate-700'"
            @click="billingPeriod = billingPeriod === 'monthly' ? 'annual' : 'monthly'"
          >
            <span
              class="absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full transition-transform"
              :class="billingPeriod === 'annual' ? 'translate-x-5' : ''"
            />
          </button>
          <button
            class="text-sm font-medium transition-colors flex items-center gap-1.5"
            :class="billingPeriod === 'annual' ? 'text-white' : 'text-slate-500 hover:text-slate-300'"
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
            class="relative bg-[#151923] border rounded-xl p-6 transition-colors"
            :class="
              billingPeriod === 'monthly'
                ? 'border-blue-500/40'
                : 'border-slate-800 opacity-60'
            "
          >
            <h3 class="text-sm font-semibold text-slate-300 mb-3">Monthly</h3>
            <div class="flex items-baseline gap-1 mb-4">
              <span class="text-3xl font-bold text-white">9€</span>
              <span class="text-sm text-slate-500">/month</span>
            </div>
            <a
              href="#"
              class="block w-full py-2 text-center rounded-lg text-sm font-semibold transition-colors"
              :class="
                billingPeriod === 'monthly'
                  ? 'bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-500/20'
                  : 'bg-slate-800 text-slate-400'
              "
            >
              Get started
            </a>
          </div>

          <!-- Annual -->
          <div
            class="relative bg-[#151923] border rounded-xl p-6 transition-colors"
            :class="
              billingPeriod === 'annual'
                ? 'border-blue-500/40'
                : 'border-slate-800 opacity-60'
            "
          >
            <span
              v-if="billingPeriod === 'annual'"
              class="absolute -top-2.5 right-4 text-[10px] font-bold bg-blue-600 text-white px-2 py-0.5 rounded"
            >
              Recommended
            </span>
            <h3 class="text-sm font-semibold text-slate-300 mb-3">Annual</h3>
            <div class="flex items-baseline gap-1 mb-1">
              <span class="text-3xl font-bold text-white">90€</span>
              <span class="text-sm text-slate-500">/year</span>
            </div>
            <p class="text-xs text-emerald-400 mb-3">7.50€/month — save 18€</p>
            <a
              href="#"
              class="block w-full py-2 text-center rounded-lg text-sm font-semibold transition-colors"
              :class="
                billingPeriod === 'annual'
                  ? 'bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-500/20'
                  : 'bg-slate-800 text-slate-400'
              "
            >
              Get started
            </a>
          </div>
        </div>

        <!-- Feature checklist -->
        <div class="mt-8 bg-[#151923] border border-slate-800 rounded-xl p-6">
          <h3 class="text-sm font-semibold text-white mb-4">Everything in Pro includes:</h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
            <div
              v-for="feature in features"
              :key="feature.title"
              class="flex items-center gap-2.5"
            >
              <Check :size="14" class="text-blue-400 shrink-0" />
              <span class="text-sm text-slate-300">{{ feature.title }}</span>
            </div>
            <div class="flex items-center gap-2.5">
              <Check :size="14" class="text-blue-400 shrink-0" />
              <span class="text-sm text-slate-300">All Community features</span>
            </div>
            <div class="flex items-center gap-2.5">
              <Check :size="14" class="text-blue-400 shrink-0" />
              <span class="text-sm text-slate-300">Priority support</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

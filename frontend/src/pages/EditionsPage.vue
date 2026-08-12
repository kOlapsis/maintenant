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
import { Check, Minus, ExternalLink } from 'lucide-vue-next'
import { useEdition } from '@/composables/useEdition'
import type { Edition, QuotaResource } from '@/services/editionApi'
import EditionBadge from '@/components/EditionBadge.vue'
import InlineAlert from '@/components/ui/InlineAlert.vue'

const {
  edition,
  editionName,
  editionRank,
  licenseMessage,
  hasLicenseIssue,
  licenseSeverity,
  licenseLabel,
  loadLicenseStatus,
} = useEdition()

onMounted(loadLicenseStatus)

const TIERS = ['community', 'personal', 'pro'] as const
type Tier = (typeof TIERS)[number]
const RANK: Record<Tier, number> = { community: 0, personal: 1, pro: 2 }

/**
 * Prices are compiled in. Changing one needs a new binary — an accepted
 * trade-off while they are not settled (spec Assumptions).
 */
const PRICING: Record<Tier, { price: string; period: string; note: string }> = {
  community: { price: 'Free', period: '', note: 'Self-hosted, no account required' },
  personal: {
    price: '€149',
    period: 'once, for life',
    note: 'One person, your own infrastructure. One year of updates, then €59/year',
  },
  pro: { price: '€29', period: 'per month', note: 'Commercial use and support included' },
}

/** Human labels for the capability identifiers the engine reports. */
const CAPABILITY_LABELS: Record<string, string> = {
  alert_routing: 'Alert routing (webhook, Discord, ntfy…)',
  swarm_dashboard: 'Docker Swarm: nodes and cluster',
  k8s_cluster: 'Kubernetes: nodes and cluster',
  multihost: 'Multi-host: remote agents',
  smtp: 'Email / SMTP channel',
  cve_enrichment: 'CVE enrichment',
  risk_scoring: 'Risk scoring',
  security_posture: 'Unified security posture',
  incidents: 'Status page incidents',
  changelog: 'Update changelog',
  resource_history: 'Container resource history',
  alert_advanced_filters: 'Advanced trigger filters',
  ocsp_stapling: 'OCSP stapling',
  slack: 'Slack channel',
  teams: 'Microsoft Teams channel',
  alert_escalation: 'Escalation policies',
  alert_entity_routing: 'Per-entity alert routing',
  maintenance_windows: 'Maintenance windows',
  subscribers: 'Status page subscribers',
  personalization: 'Status page personalization',
}

const RESOURCE_LABELS: Record<QuotaResource, string> = {
  endpoints: 'Endpoints',
  heartbeats: 'Heartbeats',
  certificates: 'Certificate monitors',
  status_components: 'Status page components',
  agent_hosts: 'Machines monitored',
}

/**
 * The caps per tier. They mirror extension.Limit — the backend only reports the
 * running edition's own limits, so the other two columns cannot be read from
 * the API and are stated here.
 */
const QUOTAS: Record<QuotaResource, Record<Tier, number>> = {
  endpoints: { community: 10, personal: -1, pro: -1 },
  heartbeats: { community: 5, personal: -1, pro: -1 },
  certificates: { community: 5, personal: -1, pro: -1 },
  status_components: { community: 3, personal: -1, pro: -1 },
  agent_hosts: { community: 1, personal: 20, pro: -1 },
}

/**
 * The capability rows, grouped by the tier that opens them and ordered
 * Community → Personal → Pro. Built from the engine's registry so this page can
 * never claim something the middleware would refuse.
 */
const rows = computed(() => {
  const registry = edition.value?.feature_editions ?? {}
  const entries = Object.entries(registry) as [string, Edition][]

  return entries
    .map(([capability, min]) => ({
      capability,
      label: CAPABILITY_LABELS[capability] ?? capability,
      min,
      rank: RANK[min as Tier] ?? 99,
    }))
    .sort((a, b) => a.rank - b.rank || a.label.localeCompare(b.label))
})

function includedIn(minEdition: Edition, tier: Tier): boolean {
  const min = RANK[minEdition as Tier]
  return min !== undefined && RANK[tier] >= min
}

function quotaLabel(value: number): string {
  if (value < 0) return 'Unlimited'
  if (value === 1) return 'This machine only'
  return String(value)
}

const isActive = (tier: Tier) => editionName.value === tier

function tierLabel(tier: Tier): string {
  return tier.charAt(0).toUpperCase() + tier.slice(1)
}

/**
 * Where to send someone who wants to upgrade. The plan is preselected, because
 * the comparison they would land on is the one they are already reading.
 *
 * `pricing` is an anchor on the home page, not a route, so a link to /pricing
 * lands on a 404.
 */
const SITE = 'https://maintenant.dev'

function checkoutUrl(tier: Tier): string {
  const plan = tier === 'personal' ? 'personal' : 'monthly'
  return `${SITE}/checkout/?plan=${plan}`
}

/** Only tiers above the current one are worth proposing (FR-036). */
function isUpgrade(tier: Tier): boolean {
  // An unknown edition has no rank: propose nothing rather than guess.
  return editionRank.value !== null && RANK[tier] > editionRank.value
}

</script>

<template>
  <div class="mx-auto w-full max-w-6xl px-4 py-8 sm:px-6">
    <header class="mb-8">
      <h1 class="text-2xl font-semibold text-mnt-primary">Editions</h1>
      <p class="mt-2 max-w-3xl text-sm leading-relaxed text-mnt-muted">
        Maintenant comes in three editions. You are currently running
        <EditionBadge :edition="editionName" size="md" class="mx-1 align-middle" />
        . Every feature below reflects what this instance actually reports, not a
        marketing table.
      </p>
    </header>

    <InlineAlert
      v-if="hasLicenseIssue"
      :severity="licenseSeverity"
      class="mb-6"
      :title="licenseLabel"
    >
      {{ licenseMessage }}
    </InlineAlert>

    <!-- Pricing header -->
    <div class="mb-6 grid gap-4 sm:grid-cols-3">
      <div
        v-for="tier in TIERS"
        :key="tier"
        class="rounded-xl border bg-mnt-surface p-5"
        :class="isActive(tier) ? 'edition-card-active' : 'border-mnt-default'"
      >
        <div class="mb-3 flex items-center justify-between gap-2">
          <EditionBadge :edition="tier" size="md" />
          <span v-if="isActive(tier)" class="text-[11px] font-semibold text-mnt-accent">
            Current
          </span>
        </div>
        <p class="text-2xl font-semibold text-mnt-primary">
          {{ PRICING[tier].price }}
          <span v-if="PRICING[tier].period" class="text-sm font-normal text-mnt-muted">
            {{ PRICING[tier].period }}
          </span>
        </p>
        <p class="mt-2 text-xs leading-relaxed text-mnt-muted">{{ PRICING[tier].note }}</p>

        <a
          v-if="isUpgrade(tier)"
          :href="checkoutUrl(tier)"
          target="_blank"
          rel="noopener noreferrer"
          class="mt-4 inline-flex items-center gap-1.5 rounded-lg border border-mnt-default px-3 py-1.5 text-xs font-semibold text-mnt-primary hover:bg-mnt-hover"
        >
          Upgrade to {{ tierLabel(tier) }}
          <ExternalLink class="h-3 w-3" />
        </a>
      </div>
    </div>

    <!-- Comparison table -->
    <div class="overflow-x-auto rounded-xl border border-mnt-default bg-mnt-surface">
      <table class="w-full min-w-[36rem] text-sm">
        <thead>
          <tr class="border-b border-mnt-default text-left">
            <th class="px-4 py-3 font-semibold text-mnt-primary">Limits</th>
            <th
              v-for="tier in TIERS"
              :key="tier"
              class="px-4 py-3 text-center font-semibold"
              :class="isActive(tier) ? 'text-mnt-primary' : 'text-mnt-muted'"
            >
              {{ tierLabel(tier) }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(label, resource) in RESOURCE_LABELS"
            :key="resource"
            class="border-b border-mnt-subtle last:border-0"
          >
            <td class="px-4 py-2.5 text-mnt-secondary">{{ label }}</td>
            <td
              v-for="tier in TIERS"
              :key="tier"
              class="px-4 py-2.5 text-center"
              :class="isActive(tier) ? 'text-mnt-primary font-medium' : 'text-mnt-muted'"
            >
              {{ quotaLabel(QUOTAS[resource][tier]) }}
            </td>
          </tr>

          <tr class="border-b border-mnt-default bg-mnt-elevated">
            <th colspan="4" class="px-4 py-3 text-left font-semibold text-mnt-primary">
              Features
            </th>
          </tr>

          <tr
            v-for="row in rows"
            :key="row.capability"
            class="border-b border-mnt-subtle last:border-0"
          >
            <td class="px-4 py-2.5 text-mnt-secondary">{{ row.label }}</td>
            <td v-for="tier in TIERS" :key="tier" class="px-4 py-2.5 text-center">
              <Check
                v-if="includedIn(row.min, tier)"
                class="mx-auto h-4 w-4 text-mnt-accent"
                :aria-label="`Included in ${tier}`"
              />
              <Minus v-else class="mx-auto h-4 w-4 text-mnt-muted opacity-40" aria-label="Not included" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <p class="mt-6 max-w-3xl text-xs leading-relaxed text-mnt-muted">
      The Personal edition is a one-time purchase, valid for life, for
      <strong class="text-mnt-secondary">one person on infrastructure they own or run
      for themselves</strong>, freelancers included. It does not cover monitoring
      someone else's infrastructure or reselling Maintenant as a service, and carries no
      support commitment. It includes one year of product updates; every version released
      in that year stays yours for life, and a further year costs €59. Pro adds the right
      to use Maintenant on behalf of others, support, and the features a team needs to be
      paged and to address third parties.
    </p>
  </div>
</template>

<style scoped>
.edition-card-active {
  border: 1px solid var(--mnt-accent);
  box-shadow: 0 0 0 1px var(--mnt-accent);
}
</style>

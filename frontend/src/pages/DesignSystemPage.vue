<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.

  Dev-only design system gallery (route /_ds). Renders every shared component in
  its variants and states so tokens + a11y can be reviewed in light and dark.
-->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { Box, Globe, Shield, Heart, Sun, Moon, ServerOff, Plus, Trash2 } from 'lucide-vue-next'
import { useTheme } from '@/composables/useTheme'
import { severityVar, type Severity } from '@/composables/useSeverity'
import StatusDot from '@/components/ui/StatusDot.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import SeverityRow from '@/components/ui/SeverityRow.vue'
import StatusTile from '@/components/ui/StatusTile.vue'
import StatusGrid from '@/components/ui/StatusGrid.vue'
import KpiStrip from '@/components/ui/KpiStrip.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'
import DensityToggle from '@/components/ui/DensityToggle.vue'
import UiTooltip from '@/components/ui/UiTooltip.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import ListToolbar from '@/components/ui/ListToolbar.vue'
import ListRow from '@/components/ui/ListRow.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import CollapsiblePanel from '@/components/ui/CollapsiblePanel.vue'
import type { ChipTone, StatusChip } from '@/components/ui/listFilters'
import { usePreferencesStore } from '@/stores/preferences'
import TabNav, { type TabNavItem } from '@/components/ui/TabNav.vue'
import CountBadge from '@/components/ui/CountBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import TextareaInput from '@/components/ui/TextareaInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import RadioGroup from '@/components/ui/RadioGroup.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import UiModal from '@/components/ui/UiModal.vue'
import EndpointStatusBadge from '@/components/EndpointStatusBadge.vue'
import HeartbeatStatusBadge from '@/components/HeartbeatStatusBadge.vue'
import CertificateStatusBadge from '@/components/CertificateStatusBadge.vue'
import ClusterHealthBadge from '@/components/ClusterHealthBadge.vue'
import OCSPStatusBadge from '@/components/OCSPStatusBadge.vue'
import type { GridGroup } from '@/components/ui/statusGrid'

const { resolvedTheme, setTheme } = useTheme()
function flipTheme() {
  setTheme(resolvedTheme.value === 'dark' ? 'light' : 'dark')
}

const severities: Severity[] = ['ok', 'warning', 'incident', 'unknown', 'neutral']

const gridView = ref<'grid' | 'list'>('grid')

// List primitives demo. Scope 'demo' keeps the real pages' view choice untouched.
const prefs = usePreferencesStore()

interface DemoRow {
  id: string
  name: string
  target: string
  status: 'up' | 'down' | 'degraded'
  responseMs: number
}

const demoRows: DemoRow[] = [
  { id: 'r1', name: 'traefik', target: 'https://maintenant.dev/health', status: 'up', responseMs: 42 },
  { id: 'r2', name: 'shm-app', target: 'https://metrics.kolapsis.com/healthcheck', status: 'down', responseMs: 0 },
  { id: 'r3', name: 'authentik', target: 'https://auth.kolapsis.com/-/health/live/', status: 'degraded', responseMs: 1840 },
  { id: 'r4', name: 'registry', target: 'registry.kolapsis.com:5000', status: 'up', responseMs: 18 },
]

const demoSearch = ref('')
const demoStatus = ref('')

const demoFiltered = computed(() =>
  demoRows.filter((r) => {
    if (demoStatus.value && r.status !== demoStatus.value) return false
    const q = demoSearch.value.trim().toLowerCase()
    if (!q) return true
    return r.name.toLowerCase().includes(q) || r.target.toLowerCase().includes(q)
  }),
)

const demoChips = computed<StatusChip[]>(() => [
  { value: 'up', label: 'up', count: demoRows.filter((r) => r.status === 'up').length, tone: 'ok' },
  { value: 'degraded', label: 'degraded', count: demoRows.filter((r) => r.status === 'degraded').length, tone: 'warn' },
  { value: 'down', label: 'down', count: demoRows.filter((r) => r.status === 'down').length, tone: 'down' },
])

const demoTone = (r: DemoRow): ChipTone =>
  r.status === 'up' ? 'ok' : r.status === 'down' ? 'down' : 'warn'

const demoColumns: Column[] = [
  { key: 'name', label: 'Name', sortable: true, width: 'minmax(0, 1fr)' },
  { key: 'target', label: 'Target', priority: 'md' },
  { key: 'status', label: 'Status', sortable: true, width: '110px' },
  { key: 'response', label: 'Response', sortable: true, align: 'right', width: '90px' },
]

function demoSortValue(row: DemoRow, key: string) {
  if (key === 'response') return row.responseMs
  if (key === 'status') return row.status
  return row.name
}

function okItems(prefix: string, names: string[]) {
  return names.map((n, i) => ({
    id: `${prefix}-${i}`,
    severity: 'ok' as Severity,
    name: n,
    meta: `${(i % 3) + 0.3}% cpu`,
    kind: 'Container',
    description: 'Running',
  }))
}

const sampleGroups: GridGroup[] = [
  {
    key: 'containers',
    label: 'Containers',
    icon: Box,
    items: [
      { id: 'c-crit', severity: 'incident', name: 'matrix-coturn', meta: '42 issues', kind: 'Container', description: '42 security issues — ports exposed on all interfaces' },
      { id: 'c-warn', severity: 'warning', name: 'bitwarden-postgres', meta: 'update 18.4', kind: 'Container', description: 'Update available — postgres 18.4' },
      ...okItems('c', ['traefik', 'redis', 'minio', 'gitea', 'grafana', 'loki', 'n8n', 'authentik', 'vaultwarden', 'registry']),
    ],
  },
  {
    key: 'endpoints',
    label: 'HTTP Endpoints',
    icon: Globe,
    items: [
      { id: 'e-crit', severity: 'incident', name: 'shm-app', meta: '3 checks KO', kind: 'Endpoint', description: 'metrics.kolapsis.com/healthcheck — 3 consecutive failures' },
      ...okItems('e', ['maintenant.dev', 'app.ackify.io', 'api.ackify.io', 'status.kolapsis.com', 'auth.kolapsis.com']).map((it) => ({ ...it, kind: 'Endpoint', meta: `${30 + (Number(it.id.split('-')[1]) || 0) * 7}ms`, description: 'Up' })),
    ],
  },
  {
    key: 'ssl',
    label: 'SSL Certificates',
    icon: Shield,
    collapsedByDefault: true,
    items: okItems('s', ['kolapsis.com', 'ackify.io', 'maintenant.dev', 'restoreproof.com', 'superkloud.eu', 'umami.kolapsis.com']).map((it) => ({ ...it, kind: 'SSL', meta: 'valid 42d', description: 'Valid' })),
  },
]

const sampleKpis = [
  { label: 'Global Uptime', value: '92.96%', sub: '66 / 71 monitors' },
  { label: 'Response Time', value: '55ms', sub: 'avg. endpoints' },
  { label: 'Host CPU', value: '2%', sub: '7.4 / 62.8 GB' },
  { label: 'SSL Certificates', value: '8 OK', sub: 'all valid', tone: 'ok' as Severity },
  { label: 'Security Posture', value: '96', sub: '56 scored', tone: 'ok' as Severity },
]

const lastSelected = ref('')

// Form controls demo state
const demoTab = ref('history')
const tabItems: TabNavItem[] = [
  { value: 'history', label: 'History' },
  { value: 'triggers', label: 'Triggers', count: 4 },
  { value: 'silence', label: 'Silence', count: 2, countTone: 'warn' },
]

const demoName = ref('')
const demoAge = ref<number | null>(null)
const demoEmail = ref('')
const demoEmailError = ref('This field is required')
const demoRuntime = ref('docker')
const demoNotes = ref('')
const demoAccept = ref(false)
const demoTags = ref<string[]>(['alerts'])
const demoPlan = ref('pro')
const demoNotifications = ref(true)
const demoNotificationsSm = ref(false)

const demoButtonLoading = ref(false)
function triggerButtonLoading() {
  demoButtonLoading.value = true
  setTimeout(() => (demoButtonLoading.value = false), 1200)
}

const demoModalOpen = ref(false)
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-10 p-6 pb-24">
    <header class="flex items-center gap-3">
      <h1 class="text-lg font-bold text-mnt-primary">Design system</h1>
      <span class="rounded bg-mnt-elevated px-2 py-0.5 font-mono text-[11px] text-mnt-muted">/_ds</span>
      <div class="ml-auto flex items-center gap-2">
        <DensityToggle />
        <UiButton variant="secondary" size="sm" :icon="resolvedTheme === 'dark' ? Sun : Moon" @click="flipTheme">
          {{ resolvedTheme === 'dark' ? 'Light' : 'Dark' }}
        </UiButton>
      </div>
    </header>

    <!-- Severity scale -->
    <section class="space-y-3">
      <SectionHeader title="Severity scale" :count="severities.length" />
      <div class="flex flex-wrap gap-3">
        <div
          v-for="s in severities"
          :key="s"
          class="flex min-w-[140px] flex-col gap-2 rounded-xl border border-mnt-default bg-mnt-surface p-3"
        >
          <div class="flex items-center gap-2">
            <span class="h-4 w-4 rounded-full" :style="{ backgroundColor: severityVar(s) }" />
            <span class="text-xs font-semibold capitalize text-mnt-primary">{{ s }}</span>
          </div>
          <div class="rounded px-2 py-1 text-[11px] font-medium" :class="[`bg-mnt-sev-${s}`, `text-mnt-sev-${s}`]">
            muted wash + AA text
          </div>
        </div>
      </div>
    </section>

    <!-- Status primitives -->
    <section class="space-y-3">
      <SectionHeader title="StatusDot / StatusBadge" />
      <div class="flex flex-wrap items-center gap-5 rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <StatusDot v-for="s in severities" :key="s" :severity="s" size="lg" :pulse="s === 'incident'" />
      </div>
      <div class="flex flex-wrap items-center gap-4 rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <StatusBadge v-for="s in severities" :key="s" :severity="s" show-label />
      </div>
      <div class="flex flex-wrap items-center gap-4 rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <StatusBadge :status="'critical'" show-label label="legacy critical → incident" />
        <StatusBadge :status="'down'" show-label label="legacy down → incident" />
        <StatusBadge :status="'paused'" show-label label="legacy paused → neutral" />
      </div>
    </section>

    <!-- Rallied badges -->
    <section class="space-y-3">
      <SectionHeader title="Domain badges (rallied to StatusBadge)" />
      <div class="flex flex-wrap items-center gap-4 rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <EndpointStatusBadge status="up" />
        <EndpointStatusBadge status="down" />
        <HeartbeatStatusBadge status="started" />
        <HeartbeatStatusBadge status="down" />
        <CertificateStatusBadge status="valid" />
        <CertificateStatusBadge status="expiring" />
        <CertificateStatusBadge status="expired" />
        <ClusterHealthBadge health="healthy" />
        <ClusterHealthBadge health="degraded" />
        <OCSPStatusBadge status="good" />
        <OCSPStatusBadge status="revoked" />
      </div>
    </section>

    <!-- Rows & tiles -->
    <section class="space-y-3">
      <SectionHeader title="SeverityRow" />
      <div class="rounded-xl border border-mnt-default bg-mnt-surface p-2">
        <SeverityRow severity="incident" name="71b219ddb31b" kind="Agent" description="Agent disconnected (stream_ended)" timestamp="27 min" @select="lastSelected = 'row:incident'" />
        <SeverityRow severity="warning" name="bitwarden-postgres" kind="Update" description="Update available — postgres 18.4" timestamp="7 h" @select="lastSelected = 'row:warning'" />
        <SeverityRow severity="ok" name="maintenant-api" kind="Container" description="Running" metric="0.4% cpu" :interactive="false" />
      </div>
      <p v-if="lastSelected" class="text-xs text-mnt-muted">selected: {{ lastSelected }}</p>
    </section>

    <section class="space-y-3">
      <SectionHeader title="StatusTile" />
      <div class="grid gap-1.5 rounded-xl border border-mnt-default bg-mnt-surface p-4" style="grid-template-columns: repeat(auto-fill, minmax(118px, 1fr))">
        <StatusTile severity="incident" name="matrix-coturn" meta="42 issues" />
        <StatusTile severity="warning" name="bitwarden-pg" meta="update 18.4" />
        <StatusTile severity="ok" name="traefik" meta="0.3% cpu" />
        <StatusTile severity="unknown" name="probe-eu" meta="—" />
        <StatusTile severity="neutral" name="seed-job" meta="paused" />
      </div>
    </section>

    <!-- StatusGrid -->
    <section class="space-y-3">
      <SectionHeader title="StatusGrid (collapsible groups · grid/list toggle)" />
      <StatusGrid v-model:view="gridView" :groups="sampleGroups" @select="lastSelected = $event.name" />
    </section>

    <!-- KPI -->
    <section class="space-y-3">
      <SectionHeader title="KpiStrip" />
      <KpiStrip :stats="sampleKpis" />
    </section>

    <!-- Controls -->
    <section class="space-y-3">
      <SectionHeader title="SegmentedToggle + Tooltip" />
      <div class="flex flex-wrap items-center gap-6 rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <SegmentedToggle v-model="gridView" :options="[{ value: 'grid', label: 'Grid' }, { value: 'list', label: 'List' }]" ariaLabel="Demo view" />
        <UiTooltip text="Last check 12s ago · 0.4% cpu" placement="top">
          <template #trigger>
            <span class="cursor-help rounded border border-mnt-default px-2.5 py-1 text-xs text-mnt-secondary">Hover me</span>
          </template>
        </UiTooltip>
      </div>
    </section>

    <!-- List primitives -->
    <section class="space-y-3">
      <SectionHeader title="ListToolbar + ListRow + DataTable (search · status chips · cards/rows/table)" />
      <div class="rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <ListToolbar
          v-model:search="demoSearch"
          v-model:status="demoStatus"
          scope="demo"
          :chips="demoChips"
          :result-count="demoFiltered.length"
          search-placeholder="Search monitors"
          :active-filter-count="0"
        >
          <template #filters>
            <p class="text-xs text-mnt-muted">Secondary filters live here, one per page.</p>
          </template>
        </ListToolbar>

        <div
          v-if="prefs.listView('demo') === 'rows'"
          class="overflow-hidden rounded-xl border border-mnt-default"
        >
          <ListRow
            v-for="row in demoFiltered"
            :key="row.id"
            :tone="demoTone(row)"
            @select="lastSelected = row.name"
          >
            <span class="min-w-0 flex-1 truncate text-sm font-semibold text-mnt-primary">{{ row.name }}</span>
            <span class="hidden min-w-0 flex-1 truncate text-xs text-mnt-muted sm:block">{{ row.target }}</span>
            <EndpointStatusBadge :status="row.status" />
            <span class="w-14 text-right font-mono text-xs text-mnt-muted">
              {{ row.responseMs ? `${row.responseMs}ms` : '-' }}
            </span>
          </ListRow>
        </div>

        <DataTable
          v-else-if="prefs.listView('demo') === 'table'"
          :columns="demoColumns"
          :rows="demoFiltered"
          :row-key="(r) => r.id"
          :sort-value="demoSortValue"
          :tone="demoTone"
          default-sort="name"
          caption="Demo monitors"
          @select="lastSelected = $event.name"
        >
          <template #cell-name="{ row }">
            <span class="truncate font-semibold text-mnt-primary">{{ row.name }}</span>
          </template>
          <template #cell-target="{ row }">
            <span class="truncate font-mono text-xs text-mnt-muted">{{ row.target }}</span>
          </template>
          <template #cell-status="{ row }">
            <EndpointStatusBadge :status="row.status" />
          </template>
          <template #cell-response="{ row }">
            <span class="font-mono text-xs">{{ row.responseMs ? `${row.responseMs}ms` : '-' }}</span>
          </template>
        </DataTable>

        <div v-else class="grid gap-3 sm:grid-cols-2">
          <div
            v-for="row in demoFiltered"
            :key="row.id"
            class="rounded-xl border border-mnt-default bg-mnt-elevated p-3"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="truncate text-sm font-semibold text-mnt-primary">{{ row.name }}</span>
              <EndpointStatusBadge :status="row.status" />
            </div>
            <p class="mt-1 truncate font-mono text-xs text-mnt-muted">{{ row.target }}</p>
          </div>
        </div>
      </div>
    </section>

    <!-- CollapsiblePanel -->
    <section class="space-y-3">
      <SectionHeader title="CollapsiblePanel (remembers its state)" />
      <CollapsiblePanel storage-key="ds-demo" title="Top consumers">
        <template #summary>4.2 GB / 16 GB RAM · 24 containers · matrix-coturn 38%</template>
        <p class="text-sm text-mnt-secondary">
          Panel contents. Collapsed, the summary above stands in for it.
        </p>
      </CollapsiblePanel>
    </section>

    <!-- Data states -->
    <section class="space-y-3">
      <SectionHeader title="Data states" />
      <div class="grid gap-4 lg:grid-cols-3">
        <div class="rounded-xl border border-mnt-default bg-mnt-surface">
          <LoadingSkeleton variant="list" :count="4" />
        </div>
        <div class="rounded-xl border border-mnt-default bg-mnt-surface">
          <EmptyState :icon="ServerOff" title="Nothing needs your attention" description="All monitors are operational." />
        </div>
        <div class="rounded-xl border border-mnt-default bg-mnt-surface">
          <ErrorState message="Couldn't reach the monitor API. Check the agent connection and retry." retryable />
        </div>
      </div>
      <div class="rounded-xl border border-mnt-default bg-mnt-surface p-3">
        <LoadingSkeleton variant="grid" :count="12" />
      </div>
    </section>

    <!-- TabNav + CountBadge -->
    <section class="space-y-3">
      <SectionHeader title="TabNav + CountBadge" />
      <div class="rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <TabNav v-model="demoTab" :items="tabItems" ariaLabel="Demo section tabs" />
        <p class="mt-3 text-xs text-mnt-muted">Active tab: {{ demoTab }}</p>
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <CountBadge :value="3" />
          <CountBadge :value="5" tone="warn" />
          <CountBadge :value="2" tone="danger" />
          <CountBadge :value="9" tone="ok" />
        </div>
      </div>
    </section>

    <!-- UiButton -->
    <section class="space-y-3">
      <SectionHeader title="UiButton" />
      <div class="flex flex-wrap items-center gap-3 rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <UiButton variant="primary" :icon="Plus">Primary</UiButton>
        <UiButton variant="secondary">Secondary</UiButton>
        <UiButton variant="danger" :icon="Trash2">Danger</UiButton>
        <UiButton variant="ghost">Ghost</UiButton>
        <UiButton variant="primary" size="sm">Small</UiButton>
        <UiButton variant="secondary" disabled>Disabled</UiButton>
        <UiButton variant="primary" :loading="demoButtonLoading" @click="triggerButtonLoading">
          {{ demoButtonLoading ? 'Saving…' : 'Click to load' }}
        </UiButton>
      </div>
    </section>

    <!-- Form fields -->
    <section class="space-y-3">
      <SectionHeader title="FormField + inputs" />
      <div class="grid gap-4 rounded-xl border border-mnt-default bg-mnt-surface p-4 sm:grid-cols-2">
        <FormField label="Name" hint="As shown on the dashboard" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="demoName" :aria-describedby="describedBy" :invalid="invalid" placeholder="matrix-coturn" />
          </template>
        </FormField>

        <FormField label="Age" hint="Numeric field — empty clears to null">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="demoAge" type="number" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>

        <FormField label="Email" :error="demoEmailError">
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="demoEmail"
              type="email"
              :aria-describedby="describedBy"
              :invalid="invalid"
              placeholder="you@example.com"
              @input="demoEmailError = demoEmail ? '' : 'This field is required'"
            />
          </template>
        </FormField>

        <FormField label="Runtime" hint="Select input">
          <template #default="{ id, describedBy, invalid }">
            <SelectInput
              :id="id"
              v-model="demoRuntime"
              :aria-describedby="describedBy"
              :invalid="invalid"
              :options="[
                { value: 'docker', label: 'Docker' },
                { value: 'k8s', label: 'Kubernetes' },
                { value: 'swarm', label: 'Swarm', disabled: true },
              ]"
            />
          </template>
        </FormField>

        <FormField label="Notes" hint="Textarea, mono" class="sm:col-span-2">
          <template #default="{ id, describedBy, invalid }">
            <TextareaInput :id="id" v-model="demoNotes" mono :aria-describedby="describedBy" :invalid="invalid" :rows="3" />
          </template>
        </FormField>
      </div>

      <div class="flex flex-wrap items-center gap-3 rounded-xl border border-mnt-default bg-mnt-surface p-4 text-xs text-mnt-muted">
        <span>md ·</span>
        <TextInput model-value="" size="md" placeholder="md" class="max-w-[140px]" />
        <span>sm ·</span>
        <TextInput model-value="" size="sm" placeholder="sm" class="max-w-[140px]" />
      </div>
    </section>

    <!-- Checkbox / radio / toggle -->
    <section class="space-y-3">
      <SectionHeader title="CheckboxInput + RadioGroup + ToggleSwitch" />
      <div class="grid gap-4 rounded-xl border border-mnt-default bg-mnt-surface p-4 sm:grid-cols-3">
        <div class="space-y-2">
          <CheckboxInput v-model="demoAccept" label="I accept the terms" />
          <CheckboxInput value="alerts" :model-value="demoTags" label="Alerts channel" @update:model-value="(v) => (demoTags = v as string[])" />
          <CheckboxInput value="updates" :model-value="demoTags" label="Updates channel" @update:model-value="(v) => (demoTags = v as string[])" />
          <p class="text-xs text-mnt-muted">Selected: {{ demoTags.join(', ') || 'none' }}</p>
        </div>
        <RadioGroup
          v-model="demoPlan"
          ariaLabel="Plan"
          :options="[
            { value: 'community', label: 'Community', hint: 'Free forever' },
            { value: 'pro', label: 'Pro', hint: '29€/month' },
          ]"
        />
        <div class="flex flex-col gap-3">
          <ToggleSwitch v-model="demoNotifications" label="Email notifications" show-label />
          <ToggleSwitch v-model="demoNotificationsSm" label="Compact toggle" show-label size="sm" />
          <ToggleSwitch :model-value="true" label="Disabled toggle" show-label disabled />
        </div>
      </div>
    </section>

    <!-- UiModal -->
    <section class="space-y-3">
      <SectionHeader title="UiModal (rebuilt ConfirmDialog uses this)" />
      <div class="rounded-xl border border-mnt-default bg-mnt-surface p-4">
        <UiButton variant="secondary" @click="demoModalOpen = true">Open modal</UiButton>
        <UiModal v-model:open="demoModalOpen" title="Delete monitor" size="sm">
          <p class="text-sm text-mnt-secondary">This removes the monitor and its history. This cannot be undone.</p>
          <template #footer>
            <UiButton variant="secondary" @click="demoModalOpen = false">Cancel</UiButton>
            <UiButton variant="danger" @click="demoModalOpen = false">Delete</UiButton>
          </template>
        </UiModal>
      </div>
    </section>

    <p class="flex items-center gap-2 text-xs text-mnt-muted">
      <Heart :size="13" aria-hidden="true" />
      Tokens-only · dual-theme · keyboard-navigable · respects reduced-motion
    </p>
  </div>
</template>

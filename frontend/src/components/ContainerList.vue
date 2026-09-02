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
import { onMounted, ref, computed } from 'vue'
import { useContainersStore } from '@/stores/containers'
import { useResourcesStore } from '@/stores/resources'
import { useHostLabel } from '@/composables/useHostLabel'
import { useListFilter } from '@/composables/useListFilter'
import { timeAgo } from '@/utils/time'
import { getStateStyle } from '@/utils/containerState'
import type { Container } from '@/services/containerApi'
import ContainerCard from './ContainerCard.vue'
import ContainerRow from './ContainerRow.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import ListToolbar from '@/components/ui/ListToolbar.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import type { ChipTone, StatusChip } from '@/components/ui/listFilters'
import { usePreferencesStore } from '@/stores/preferences'
import { ChevronDown, Box, SearchX } from 'lucide-vue-next'

const store = useContainersStore()
const resources = useResourcesStore()
const prefs = usePreferencesStore()
const { showHost, hostLabel } = useHostLabel()

const collapsedGroups = ref<Set<string>>(new Set())
const showArchived = ref(false)
const hostFilter = ref('')
const groupFilter = ref('')

const emit = defineEmits<{
  select: [container: Container]
}>()

interface ControllerGroup {
  kind: string
  name: string
  containers: Container[]
  readyCount: number
  podCount: number
}

const view = computed(() => prefs.listView('containers'))
const hasControllerHierarchy = computed(() => store.isKubernetesMode || store.isSwarmMode)

const stateTones: Record<string, ChipTone> = {
  running: 'ok',
  restarting: 'critical',
  exited: 'down',
  dead: 'down',
  paused: 'warn',
  created: 'warn',
}

function toneOf(container: Container): ChipTone {
  if (container.stale) return 'unknown'
  return stateTones[container.state] ?? 'neutral'
}

// The rows every filter works from: archived containers only join in once asked for.
const sourceContainers = computed(() =>
  store.allContainers.filter((c) => showArchived.value || !c.archived),
)

const groupOfContainer = computed(() => {
  const map = new Map<string, string>()
  for (const g of store.groups) {
    for (const c of g.containers) map.set(c.id, g.name)
  }
  return map
})

const hostOptions = computed(() => {
  const map = new Map<string, string>()
  for (const c of sourceContainers.value) {
    const key = c.agent_id ?? ''
    if (!map.has(key)) map.set(key, hostLabel(c.agent_id, c.agent_hostname, c.agent_label))
  }
  return Array.from(map, ([value, label]) => ({ value, label })).sort((a, b) =>
    a.label.localeCompare(b.label),
  )
})

const groupOptions = computed(() => store.groups.map((g) => g.name))

const filter = useListFilter<Container>(sourceContainers, {
  searchFields: (c) => [
    c.name,
    c.image,
    c.external_id,
    c.orchestration_unit,
    c.agent_hostname,
    c.agent_label,
  ],
  status: (c) => c.state,
  extra: {
    host: computed(() =>
      hostFilter.value ? (c: Container) => (c.agent_id ?? '') === hostFilter.value : null,
    ),
    group: computed(() =>
      groupFilter.value
        ? (c: Container) => groupOfContainer.value.get(c.id) === groupFilter.value
        : null,
    ),
  },
})

// Counts come from the list the other filters already narrowed, so a chip
// promising "3 exited" really leaves 3 rows standing once it is clicked.
const chips = computed<StatusChip[]>(() =>
  Array.from(filter.statusCounts.value, ([value, count]) => ({
    value,
    label: value,
    count,
    tone: stateTones[value] ?? 'neutral',
  })).sort((a, b) => b.count - a.count),
)

// Include-archived is itself a deviation from the default view, so it counts.
const activeFilterCount = computed(
  () => filter.activeFilterCount.value + (showArchived.value ? 1 : 0),
)

const isFiltered = computed(
  () => filter.search.value !== '' || filter.status.value !== '' || activeFilterCount.value > 0,
)

const filteredIds = computed(() => new Set(filter.filtered.value.map((c) => c.id)))

const visibleGroups = computed(() =>
  store.groups
    .map((g) => ({ ...g, containers: g.containers.filter((c) => filteredIds.value.has(c.id)) }))
    .filter((g) => g.containers.length > 0),
)

function getControllerGroups(containers: Container[]): ControllerGroup[] {
  if (!hasControllerHierarchy.value) return []

  const map = new Map<string, ControllerGroup>()
  for (const c of containers) {
    if (!c.controller_kind) continue
    const key = `${c.controller_kind}/${c.orchestration_unit || c.name}`
    if (!map.has(key)) {
      const isSwarmService = c.controller_kind === 'swarm-service'
      map.set(key, {
        kind: c.controller_kind,
        name: c.orchestration_unit || c.name,
        containers: [],
        readyCount: isSwarmService ? 0 : (c.ready_count ?? 0),
        podCount: isSwarmService ? (c.swarm_desired_replicas ?? 0) : (c.pod_count ?? 0),
      })
    }
    const group = map.get(key)!
    group.containers.push(c)
    if (c.controller_kind === 'swarm-service' && c.state === 'running') {
      group.readyCount++
    }
  }
  return Array.from(map.values())
}

function getUngroupedContainers(containers: Container[]): Container[] {
  if (!hasControllerHierarchy.value) return containers
  return containers.filter((c) => !c.controller_kind)
}

function setArchived(value: boolean) {
  showArchived.value = value
  store.fetchContainers({ archived: value })
}

function resetFilters() {
  filter.reset()
  hostFilter.value = ''
  groupFilter.value = ''
  if (showArchived.value) setArchived(false)
}

function toggleGroup(name: string) {
  if (collapsedGroups.value.has(name)) {
    collapsedGroups.value.delete(name)
  } else {
    collapsedGroups.value.add(name)
  }
}

const columns = computed<Column[]>(() => {
  const cols: Column[] = [
    { key: 'name', label: 'Name', sortable: true },
    { key: 'group', label: 'Group', sortable: true, priority: 'sm', width: '132px' },
    { key: 'image', label: 'Image', priority: 'md' },
  ]
  if (showHost.value) cols.push({ key: 'host', label: 'Host', priority: 'lg', width: '140px' })
  cols.push(
    { key: 'state', label: 'State', sortable: true, width: '104px' },
    { key: 'cpu', label: 'CPU', sortable: true, align: 'right', priority: 'sm', width: '76px' },
    { key: 'memory', label: 'Memory', sortable: true, align: 'right', priority: 'sm', width: '84px' },
    { key: 'changed', label: 'Last change', sortable: true, align: 'right', priority: 'md', width: '104px' },
  )
  return cols
})

function sortValue(c: Container, key: string): string | number | undefined {
  switch (key) {
    case 'name':
      return c.name
    case 'group':
      return groupOfContainer.value.get(c.id)
    case 'image':
      return c.image
    case 'host':
      return hostLabel(c.agent_id, c.agent_hostname, c.agent_label)
    case 'state':
      return c.state
    case 'cpu':
      return resources.getSnapshot(c.id)?.cpu_percent ?? -1
    case 'memory':
      return resources.getSnapshot(c.id)?.mem_percent ?? -1
    case 'changed':
      return Date.parse(c.last_state_change_at) || 0
    default:
      return undefined
  }
}

function cellMetrics(c: Container) {
  return resources.formattedSnapshot(c.id)
}

function stateStyle(c: Container) {
  const s = getStateStyle(c.state)
  return { backgroundColor: s.bg, color: s.color }
}

onMounted(() => {
  store.fetchContainers()
})
</script>

<template>
  <div>
    <!-- Loading state -->
    <LoadingSkeleton v-if="store.loading" variant="cards" :count="8" />

    <!-- Error state -->
    <ErrorState v-else-if="store.error" :message="store.error" />

    <!-- Empty state -->
    <EmptyState
      v-else-if="store.groups.length === 0"
      :icon="Box"
      title="No containers detected"
      description="maintenant will automatically discover containers when they start. Make sure your container runtime is accessible."
    />

    <template v-else>
      <ListToolbar
        scope="containers"
        :search="filter.search.value"
        :status="filter.status.value"
        :chips="chips"
        :result-count="filter.filtered.value.length"
        :active-filter-count="activeFilterCount"
        search-placeholder="Search containers"
        @update:search="filter.search.value = $event"
        @update:status="filter.status.value = $event"
        @reset="resetFilters"
      >
        <template #filters>
          <label v-if="hostOptions.length > 1" class="flex flex-col gap-1">
            <span class="text-[11px] font-semibold uppercase tracking-wide text-mnt-muted">Host</span>
            <select
              v-model="hostFilter"
              class="focus-ring min-h-[38px] rounded-lg border border-mnt-default bg-mnt-primary px-2 text-xs text-mnt-secondary"
            >
              <option value="">All hosts</option>
              <option v-for="h in hostOptions" :key="h.value" :value="h.value">{{ h.label }}</option>
            </select>
          </label>

          <label v-if="groupOptions.length > 1" class="flex flex-col gap-1">
            <span class="text-[11px] font-semibold uppercase tracking-wide text-mnt-muted">Group</span>
            <select
              v-model="groupFilter"
              class="focus-ring min-h-[38px] rounded-lg border border-mnt-default bg-mnt-primary px-2 text-xs text-mnt-secondary"
            >
              <option value="">All groups</option>
              <option v-for="g in groupOptions" :key="g" :value="g">{{ g }}</option>
            </select>
          </label>

          <label v-if="store.archivedCount > 0" class="flex flex-col gap-1">
            <span class="text-[11px] font-semibold uppercase tracking-wide text-mnt-muted">Archived</span>
            <select
              class="focus-ring min-h-[38px] rounded-lg border border-mnt-default bg-mnt-primary px-2 text-xs text-mnt-secondary"
              :value="showArchived ? 'all' : 'live'"
              @change="setArchived(($event.target as HTMLSelectElement).value === 'all')"
            >
              <option value="live">Live only</option>
              <option value="all">Include archived ({{ store.archivedCount }})</option>
            </select>
          </label>
        </template>
      </ListToolbar>

      <!-- Filters left nothing standing: distinct from having no containers at all. -->
      <EmptyState
        v-if="filter.filtered.value.length === 0"
        :icon="SearchX"
        title="No container matches your filters"
        description="Try a broader search term, or clear the status and secondary filters to see the whole fleet again."
      >
        <template #action>
          <button
            type="button"
            class="focus-ring min-h-[38px] rounded-lg border border-mnt-default px-3 text-xs font-semibold text-mnt-secondary hover:text-mnt-primary"
            @click="resetFilters"
          >
            Clear filters
          </button>
        </template>
      </EmptyState>

      <!-- Dense views ignore groups: a flat list is what a sortable table is for. -->
      <div
        v-else-if="view === 'rows'"
        class="overflow-hidden rounded-xl border border-mnt-default"
      >
        <ContainerRow
          v-for="container in filter.filtered.value"
          :key="container.id"
          :container="container"
          :tone="toneOf(container)"
          :group="groupOfContainer.get(container.id)"
          @select="emit('select', $event)"
        />
      </div>

      <DataTable
        v-else-if="view === 'table'"
        :columns="columns"
        :rows="filter.filtered.value"
        :row-key="(c: Container) => c.id"
        :sort-value="sortValue"
        :tone="toneOf"
        default-sort="name"
        caption="Containers"
        @select="emit('select', $event)"
      >
        <template #cell-name="{ row }">
          <span class="font-medium text-mnt-primary">{{ row.name }}</span>
        </template>
        <template #cell-group="{ row }">
          <span class="text-mnt-muted">{{ groupOfContainer.get(row.id) }}</span>
        </template>
        <template #cell-image="{ row }">
          <span class="text-mnt-muted">{{ row.image }}</span>
        </template>
        <template #cell-host="{ row }">
          {{ hostLabel(row.agent_id, row.agent_hostname, row.agent_label) }}
        </template>
        <template #cell-state="{ row }">
          <span
            v-if="row.stale"
            class="inline-flex items-center rounded-full bg-mnt-sev-unknown px-2 py-0.5 text-[10px] font-bold text-mnt-sev-unknown"
          >offline</span>
          <span
            v-else
            class="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold"
            :style="stateStyle(row)"
          >{{ row.state }}</span>
        </template>
        <template #cell-cpu="{ row }">
          <span class="font-mono">{{ cellMetrics(row)?.cpu ?? '-' }}</span>
        </template>
        <template #cell-memory="{ row }">
          <span class="font-mono">{{ cellMetrics(row)?.memPercent ?? '-' }}</span>
        </template>
        <template #cell-changed="{ row }">
          {{ timeAgo(row.last_state_change_at) }}
        </template>
      </DataTable>

      <!-- Cards keep the groups, and the K8s/Swarm controller hierarchy inside them. -->
      <div v-else class="space-y-6">
        <div v-for="group in visibleGroups" :key="group.name">
          <button
            class="flex min-h-[44px] w-full items-center gap-2 text-left"
            :aria-expanded="!collapsedGroups.has(group.name)"
            @click="toggleGroup(group.name)"
          >
            <ChevronDown
              :size="14"
              class="shrink-0 text-mnt-muted transition-transform"
              :class="{ '-rotate-90': collapsedGroups.has(group.name) }"
              aria-hidden="true"
            />
            <h2 class="text-sm font-semibold text-mnt-secondary">{{ group.name }}</h2>
            <span class="rounded-full bg-mnt-elevated px-2 py-0.5 text-xs text-mnt-muted">
              {{ group.containers.length }}
            </span>
            <span class="text-xs text-mnt-muted">{{ group.source }}</span>
          </button>

          <template v-if="!collapsedGroups.has(group.name)">
            <div v-if="hasControllerHierarchy" class="mt-2 space-y-3">
              <div
                v-for="ctrl in getControllerGroups(group.containers)"
                :key="`${ctrl.kind}/${ctrl.name}`"
              >
                <button
                  class="flex w-full items-center gap-2 rounded bg-mnt-elevated px-2 py-1.5 text-left"
                  @click="store.toggleController(`${group.name}/${ctrl.kind}/${ctrl.name}`)"
                >
                  <ChevronDown
                    :size="13"
                    class="shrink-0 text-mnt-muted transition-transform"
                    :class="{ '-rotate-90': !store.isControllerExpanded(`${group.name}/${ctrl.kind}/${ctrl.name}`) }"
                    aria-hidden="true"
                  />
                  <span class="rounded bg-mnt-surface px-1.5 py-0.5 text-xs text-mnt-secondary">
                    {{ ctrl.kind }}
                  </span>
                  <span class="text-sm font-medium text-mnt-primary">{{ ctrl.name }}</span>
                  <span
                    class="text-xs"
                    :style="{
                      color: ctrl.readyCount === ctrl.podCount ? 'var(--mnt-status-ok)' : 'var(--mnt-status-warn)',
                    }"
                  >{{ ctrl.readyCount }}/{{ ctrl.podCount }} ready</span>
                </button>
                <div
                  v-if="store.isControllerExpanded(`${group.name}/${ctrl.kind}/${ctrl.name}`)"
                  class="mt-2 grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
                >
                  <ContainerCard
                    v-for="container in ctrl.containers"
                    :key="container.id"
                    :container="container"
                    @select="emit('select', $event)"
                  />
                </div>
              </div>

              <div
                v-if="getUngroupedContainers(group.containers).length > 0"
                class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
              >
                <ContainerCard
                  v-for="container in getUngroupedContainers(group.containers)"
                  :key="container.id"
                  :container="container"
                  @select="emit('select', $event)"
                />
              </div>
            </div>

            <div v-else class="mt-2 grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              <ContainerCard
                v-for="container in group.containers"
                :key="container.id"
                :container="container"
                @select="emit('select', $event)"
              />
            </div>
          </template>
        </div>
      </div>

    </template>

    <!-- Connection status -->
    <div v-if="!store.loading" class="mt-4 flex items-center gap-2 text-xs text-mnt-muted">
      <span
        class="inline-block h-2 w-2 rounded-full"
        :style="{ backgroundColor: store.sseConnected ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)' }"
      />
      {{ store.sseConnected ? 'Live' : 'Disconnected' }}
      <span class="ml-auto">
        <template v-if="isFiltered">
          {{ filter.filtered.value.length }} of {{ store.containerCount }} containers
        </template>
        <template v-else>{{ store.containerCount }} containers</template>
      </span>
    </div>
  </div>
</template>

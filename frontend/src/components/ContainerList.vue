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
import type { Container } from '@/services/containerApi'
import ContainerCard from './ContainerCard.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import { ChevronDown, Box } from 'lucide-vue-next'

const store = useContainersStore()
const collapsedGroups = ref<Set<string>>(new Set())
const showArchived = ref(false)

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

const hasControllerHierarchy = computed(() => store.isKubernetesMode || store.isSwarmMode)

function getControllerGroups(containers: Container[]): ControllerGroup[] {
  const active = containers.filter(c => !c.archived)
  if (!hasControllerHierarchy.value) return []

  const map = new Map<string, ControllerGroup>()
  for (const c of active) {
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
  const active = containers.filter(c => !c.archived)
  if (!hasControllerHierarchy.value) return active
  return active.filter(c => !c.controller_kind)
}

function toggleArchived() {
  showArchived.value = !showArchived.value
  store.fetchContainers({ archived: showArchived.value })
}

function toggleGroup(name: string) {
  if (collapsedGroups.value.has(name)) {
    collapsedGroups.value.delete(name)
  } else {
    collapsedGroups.value.add(name)
  }
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

    <!-- Grouped container display -->
    <div v-else class="space-y-6">
      <div v-for="group in store.groups" :key="group.name">
        <!-- Group header -->
        <button
          class="flex w-full items-center gap-2 text-left min-h-[44px]"
          @click="toggleGroup(group.name)"
        >
          <ChevronDown
            :size="14"
            class="shrink-0 text-mnt-muted transition-transform"
            :class="{ '-rotate-90': collapsedGroups.has(group.name) }"
            aria-hidden="true"
          />
          <h2 class="text-sm font-semibold" :style="{ color: 'var(--mnt-text-secondary)' }">
            {{ group.name }}
          </h2>
          <span
            class="rounded-full px-2 py-0.5 text-xs"
            :style="{
              backgroundColor: 'var(--mnt-bg-elevated)',
              color: 'var(--mnt-text-muted)',
            }"
          >
            {{ group.containers.filter(c => !c.archived).length }}
          </span>
          <span class="text-xs" :style="{ color: 'var(--mnt-text-muted)' }">
            {{ group.source }}
          </span>
        </button>

        <!-- K8s mode: controller hierarchy within namespace -->
        <div v-if="!collapsedGroups.has(group.name) && hasControllerHierarchy" class="mt-2 space-y-3">
          <!-- Controller groups -->
          <div
            v-for="ctrl in getControllerGroups(group.containers)"
            :key="`${ctrl.kind}/${ctrl.name}`"
          >
            <button
              class="flex w-full items-center gap-2 text-left px-2 py-1.5 rounded"
              :style="{ backgroundColor: 'var(--mnt-bg-elevated)' }"
              @click="store.toggleController(`${group.name}/${ctrl.kind}/${ctrl.name}`)"
            >
              <ChevronDown
                :size="13"
                class="shrink-0 text-mnt-muted transition-transform"
                :class="{ '-rotate-90': !store.isControllerExpanded(`${group.name}/${ctrl.kind}/${ctrl.name}`) }"
                aria-hidden="true"
              />
              <span
                class="rounded px-1.5 py-0.5 text-xs"
                :style="{ backgroundColor: 'var(--mnt-bg-surface)', color: 'var(--mnt-text-secondary)' }"
              >{{ ctrl.kind }}</span>
              <span class="text-sm font-medium" :style="{ color: 'var(--mnt-text-primary)' }">{{ ctrl.name }}</span>
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

          <!-- Bare pods (no controller) -->
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

        <!-- Docker mode: flat grid -->
        <div
          v-else-if="!collapsedGroups.has(group.name)"
          class="mt-2 grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
        >
          <ContainerCard
            v-for="container in group.containers.filter(c => !c.archived)"
            :key="container.id"
            :container="container"
            @select="emit('select', $event)"
          />
        </div>
      </div>
    </div>

    <!-- Archived section -->
    <div v-if="!store.loading && store.archivedCount > 0" class="mt-6">
      <button
        class="text-sm"
        :style="{ color: 'var(--mnt-text-muted)' }"
        @click="toggleArchived"
      >
        {{ showArchived ? 'Hide' : 'Show' }} archived ({{ store.archivedCount }})
      </button>
    </div>

    <!-- Connection status -->
    <div
      v-if="!store.loading"
      class="mt-4 flex items-center gap-2 text-xs"
      :style="{ color: 'var(--mnt-text-muted)' }"
    >
      <span
        class="inline-block h-2 w-2 rounded-full"
        :style="{ backgroundColor: store.sseConnected ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)' }"
      />
      {{ store.sseConnected ? 'Live' : 'Disconnected' }}
      <span class="ml-auto">{{ store.containerCount }} containers</span>
    </div>
  </div>
</template>

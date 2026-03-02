<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useContainersStore } from '@/stores/containers'
import type { Container } from '@/services/containerApi'
import ContainerCard from './ContainerCard.vue'

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

function getControllerGroups(containers: Container[]): ControllerGroup[] {
  const active = containers.filter(c => !c.archived)
  if (!store.isKubernetesMode) return []

  const map = new Map<string, ControllerGroup>()
  for (const c of active) {
    if (!c.controller_kind) continue
    const key = `${c.controller_kind}/${c.orchestration_unit || c.name}`
    if (!map.has(key)) {
      map.set(key, {
        kind: c.controller_kind,
        name: c.orchestration_unit || c.name,
        containers: [],
        readyCount: c.ready_count ?? 0,
        podCount: c.pod_count ?? 0,
      })
    }
    map.get(key)!.containers.push(c)
  }
  return Array.from(map.values())
}

function getUngroupedContainers(containers: Container[]): Container[] {
  const active = containers.filter(c => !c.archived)
  if (!store.isKubernetesMode) return active
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
    <div v-if="store.loading" class="flex items-center justify-center py-12">
      <div
        class="h-8 w-8 animate-spin rounded-full border-4"
        :style="{ borderColor: 'var(--pb-border-default)', borderTopColor: 'var(--pb-accent)' }"
      />
    </div>

    <!-- Error state -->
    <div
      v-else-if="store.error"
      class="rounded-lg p-4 text-center text-sm"
      :style="{
        backgroundColor: 'var(--pb-status-down-bg)',
        border: '1px solid var(--pb-status-down)',
        color: 'var(--pb-status-down)',
        borderRadius: 'var(--pb-radius-lg)',
      }"
    >
      {{ store.error }}
    </div>

    <!-- Empty state -->
    <div
      v-else-if="store.groups.length === 0"
      class="flex flex-col items-center justify-center py-16 text-center"
    >
      <svg width="56" height="56" viewBox="0 0 56 56" fill="none" stroke="currentColor" stroke-width="1.5" class="mb-4" style="color: var(--pb-text-muted)">
        <rect x="12" y="8" width="32" height="24" rx="4" />
        <rect x="8" y="24" width="40" height="24" rx="4" />
        <circle cx="18" cy="20" r="2" fill="currentColor" stroke="none" />
        <circle cx="24" cy="20" r="2" fill="currentColor" stroke="none" />
        <circle cx="18" cy="36" r="2" fill="currentColor" stroke="none" />
        <circle cx="24" cy="36" r="2" fill="currentColor" stroke="none" />
      </svg>
      <h3 class="text-lg font-medium mb-1" style="color: var(--pb-text-primary)">No containers detected</h3>
      <p class="text-sm max-w-sm" style="color: var(--pb-text-muted)">
        maintenant will automatically discover containers when they start. Make sure your container runtime is accessible.
      </p>
    </div>

    <!-- Grouped container display -->
    <div v-else class="space-y-6">
      <div v-for="group in store.groups" :key="group.name">
        <!-- Group header -->
        <button
          class="flex w-full items-center gap-2 text-left min-h-[44px]"
          @click="toggleGroup(group.name)"
        >
          <span
            class="text-xs transition-transform"
            :style="{ color: 'var(--pb-text-muted)' }"
            :class="{ '-rotate-90': collapsedGroups.has(group.name) }"
          >
            v
          </span>
          <h2 class="text-sm font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">
            {{ group.name }}
          </h2>
          <span
            class="rounded-full px-2 py-0.5 text-xs"
            :style="{
              backgroundColor: 'var(--pb-bg-elevated)',
              color: 'var(--pb-text-muted)',
            }"
          >
            {{ group.containers.filter(c => !c.archived).length }}
          </span>
          <span class="text-xs" :style="{ color: 'var(--pb-text-muted)' }">
            {{ group.source }}
          </span>
        </button>

        <!-- K8s mode: controller hierarchy within namespace -->
        <div v-if="!collapsedGroups.has(group.name) && store.isKubernetesMode" class="mt-2 space-y-3">
          <!-- Controller groups -->
          <div
            v-for="ctrl in getControllerGroups(group.containers)"
            :key="`${ctrl.kind}/${ctrl.name}`"
          >
            <button
              class="flex w-full items-center gap-2 text-left px-2 py-1.5 rounded"
              :style="{ backgroundColor: 'var(--pb-bg-elevated)' }"
              @click="store.toggleController(`${group.name}/${ctrl.kind}/${ctrl.name}`)"
            >
              <span
                class="text-xs transition-transform"
                :style="{ color: 'var(--pb-text-muted)' }"
                :class="{ '-rotate-90': !store.isControllerExpanded(`${group.name}/${ctrl.kind}/${ctrl.name}`) }"
              >v</span>
              <span
                class="rounded px-1.5 py-0.5 text-xs"
                :style="{ backgroundColor: 'var(--pb-bg-surface)', color: 'var(--pb-text-secondary)' }"
              >{{ ctrl.kind }}</span>
              <span class="text-sm font-medium" :style="{ color: 'var(--pb-text-primary)' }">{{ ctrl.name }}</span>
              <span
                class="text-xs"
                :style="{
                  color: ctrl.readyCount === ctrl.podCount ? 'var(--pb-status-ok)' : 'var(--pb-status-warn)',
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
        :style="{ color: 'var(--pb-text-muted)' }"
        @click="toggleArchived"
      >
        {{ showArchived ? 'Hide' : 'Show' }} archived ({{ store.archivedCount }})
      </button>
    </div>

    <!-- Connection status -->
    <div
      v-if="!store.loading"
      class="mt-4 flex items-center gap-2 text-xs"
      :style="{ color: 'var(--pb-text-muted)' }"
    >
      <span
        class="inline-block h-2 w-2 rounded-full"
        :style="{ backgroundColor: store.sseConnected ? 'var(--pb-status-ok)' : 'var(--pb-status-down)' }"
      />
      {{ store.sseConnected ? 'Live' : 'Disconnected' }}
      <span class="ml-auto">{{ store.containerCount }} containers</span>
    </div>
  </div>
</template>

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
import { ref, computed } from 'vue'
import { Server, ChevronDown } from 'lucide-vue-next'
import { useAgentsStore } from '@/stores/agents'
import { useResourcesStore } from '@/stores/resources'

// Global host/resource scope selector living at the top of the sidebar nav. It
// is the single control that scopes every list (containers, endpoints,
// certificates, heartbeats, workloads, pods, services, tasks, nodes) and the
// dashboard to a host, and drives which runtime views the nav shows. Singleton
// bound to the resources store — no props.
const agentsStore = useAgentsStore()
const resources = useResourcesStore()
const open = ref(false)

const activeAgents = computed(() => agentsStore.agents.filter((a) => a.status === 'active'))

const selectedLabel = computed(() => {
  const s = resources.selected
  if (s === null) return 'All resources'
  if (s === 'local') return 'Local'
  const found = activeAgents.value.find((a) => a.agent_id === s)
  return found ? found.label || found.hostname : s
})

function itemClass(active: boolean): string {
  return active ? 'text-mnt-primary font-medium' : 'text-mnt-secondary'
}

function select(value: string | null) {
  resources.setFilter(value)
  open.value = false
}
</script>

<template>
  <!-- Only meaningful once at least one agent is enrolled; single-host installs hide it. -->
  <div v-if="activeAgents.length > 0" class="relative">
    <button
      class="flex w-full items-center justify-between gap-1.5 rounded-lg border border-mnt-default bg-mnt-primary px-3 py-1.5 text-xs text-mnt-secondary transition-colors hover:text-mnt-primary"
      @click="open = !open"
    >
      <span class="flex items-center gap-1.5 min-w-0">
        <Server :size="13" class="text-mnt-muted shrink-0" />
        <span class="truncate">{{ selectedLabel }}</span>
      </span>
      <ChevronDown :size="13" class="text-mnt-muted shrink-0" />
    </button>

    <div
      v-if="open"
      class="absolute left-0 right-0 z-50 mt-1 min-w-[200px] rounded-xl border border-mnt-default bg-mnt-surface py-1 shadow-2xl"
    >
      <button
        class="hf-item"
        :class="itemClass(resources.selected === null)"
        @click="select(null)"
      >
        All resources
      </button>
      <button
        class="hf-item"
        :class="itemClass(resources.selected === 'local')"
        @click="select('local')"
      >
        Local
      </button>

      <div class="my-1 border-t border-mnt-subtle" />

      <button
        v-for="agent in activeAgents"
        :key="agent.agent_id"
        class="hf-item flex items-center gap-2"
        :class="itemClass(resources.selected === agent.agent_id)"
        @click="select(agent.agent_id)"
      >
        <span
          class="h-1.5 w-1.5 shrink-0 rounded-full"
          :style="{
            backgroundColor:
              agent.connection_state === 'connected'
                ? 'var(--mnt-status-ok-text)'
                : 'var(--mnt-text-muted)',
          }"
        />
        {{ agent.label || agent.hostname }}
      </button>
    </div>

    <div v-if="open" class="fixed inset-0 z-40" @click="open = false" />
  </div>
</template>

<style scoped>
.hf-item {
  width: 100%;
  padding: 0.5rem 0.75rem;
  text-align: left;
  font-size: 0.75rem;
  transition: background-color 0.12s ease;
}
.hf-item:hover {
  background-color: var(--mnt-bg-hover);
}
</style>

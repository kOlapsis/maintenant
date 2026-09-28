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
import PopoverMenu from '@/components/ui/PopoverMenu.vue'
import UiButton from '@/components/ui/UiButton.vue'

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
  <PopoverMenu
    v-if="activeAgents.length > 0"
    v-model:open="open"
    ariaLabel="Host scope"
    align="stretch"
    panel-class="min-w-[200px] py-1"
  >
    <template #trigger="{ toggle }">
      <UiButton
        variant="secondary"
        size="sm"
        block
        align="between"
        class="bg-mnt-primary text-mnt-secondary hover:text-mnt-primary"
        @click="toggle"
      >
        <span class="flex items-center gap-1.5 min-w-0">
          <Server :size="13" class="text-mnt-muted shrink-0" />
          <span class="truncate">{{ selectedLabel }}</span>
        </span>
        <ChevronDown :size="13" class="text-mnt-muted shrink-0" />
      </UiButton>
    </template>

    <UiButton
      variant="ghost"
      size="sm"
      block
      align="start"
      role="menuitem"
      class="rounded-none"
      :class="itemClass(resources.selected === null)"
      @click="select(null)"
    >
      All resources
    </UiButton>
    <UiButton
      variant="ghost"
      size="sm"
      block
      align="start"
      role="menuitem"
      class="rounded-none"
      :class="itemClass(resources.selected === 'local')"
      @click="select('local')"
    >
      Local
    </UiButton>

    <div class="my-1 border-t border-mnt-subtle" />

    <UiButton
      v-for="agent in activeAgents"
      :key="agent.agent_id"
      variant="ghost"
      size="sm"
      block
      align="start"
      role="menuitem"
      class="rounded-none"
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
    </UiButton>
  </PopoverMenu>
</template>

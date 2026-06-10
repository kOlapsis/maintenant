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
import { computed } from 'vue'
import { Server } from 'lucide-vue-next'
import { useResourcesStore } from '@/stores/resources'

// Small chip showing which agent a row was reported by. Renders only in the
// "all resources" scope and only for remote rows (those carrying agent fields),
// so single-host views and agent-scoped views stay uncluttered.
const props = defineProps<{
  agentId?: string | null
  hostname?: string | null
  label?: string | null
}>()

const resources = useResourcesStore()
const visible = computed(() => resources.selected === null && !!props.agentId)
const display = computed(() => props.label || props.hostname || 'agent')
</script>

<template>
  <span
    v-if="visible"
    class="inline-flex items-center gap-1 rounded border border-pb-default bg-pb-surface px-1.5 py-0.5 text-[10px] text-pb-muted"
    :title="hostname || agentId || ''"
  >
    <Server :size="10" class="shrink-0" />
    <span class="truncate max-w-[120px]">{{ display }}</span>
  </span>
</template>

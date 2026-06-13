<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { Inbox } from 'lucide-vue-next'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import SeverityRow from '@/components/ui/SeverityRow.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import type { AttentionItem } from '@/composables/useAttentionItems'

const props = withDefaults(defineProps<{ items: AttentionItem[]; max?: number }>(), { max: 8 })
const emit = defineEmits<{ select: [item: AttentionItem] }>()

const shown = computed(() => props.items.slice(0, props.max))
const overflow = computed(() => Math.max(0, props.items.length - props.max))
</script>

<template>
  <section>
    <SectionHeader title="Needs attention" :count="items.length" class="mb-3">
      <template #action>
        <RouterLink
          v-if="overflow > 0"
          to="/alerts"
          class="focus-ring rounded text-xs font-semibold text-mnt-accent hover:underline"
        >
          {{ overflow }} more →
        </RouterLink>
      </template>
    </SectionHeader>

    <div class="overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface p-1.5">
      <template v-if="items.length > 0">
        <SeverityRow
          v-for="it in shown"
          :key="it.id"
          :severity="it.severity"
          :name="it.name"
          :kind="it.kind"
          :description="it.description"
          :timestamp="it.timestamp"
          @select="emit('select', it)"
        />
      </template>
      <EmptyState
        v-else
        :icon="Inbox"
        title="Nothing needs your attention"
        description="All monitors are operational."
      />
    </div>
  </section>
</template>

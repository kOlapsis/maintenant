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
import { useConfirm } from '@/composables/useConfirm'
import type { EscalationPolicy } from '@/types/escalation'
import { timeAgo } from '@/utils/time'
import { Pencil, Trash2, Layers, CheckCircle2, CircleDashed } from 'lucide-vue-next'
import UiButton from '@/components/ui/UiButton.vue'
import ChipToggle from '@/components/ui/ChipToggle.vue'

defineProps<{
  policies: EscalationPolicy[]
  loading: boolean
}>()

const emit = defineEmits<{
  create: []
  edit: [policy: EscalationPolicy]
  delete: [id: string]
  toggleActive: [id: string, active: boolean]
}>()

const confirm = useConfirm()

async function handleDelete(policy: EscalationPolicy) {
  const ok = await confirm({
    title: 'Delete escalation policy',
    message: `Delete "${policy.name}"? This cannot be undone and will stop any running escalation using this policy.`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  emit('delete', policy.id)
}

function severityLabel(severities: string[]): string {
  if (!severities || severities.length === 0) return 'All'
  return severities.join(', ')
}
</script>

<template>
  <div class="bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
    <!-- Table header -->
    <div class="px-5 py-3 border-b border-mnt-default grid grid-cols-[1fr_auto_auto_auto_auto] gap-4 items-center">
      <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Name</span>
      <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest w-20 text-center">Status</span>
      <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest w-28 text-center">Severities</span>
      <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest w-20 text-center">Levels</span>
      <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest w-28 text-right">Modified</span>
    </div>

    <!-- Skeleton loading -->
    <template v-if="loading">
      <div
        v-for="i in 3"
        :key="i"
        class="px-5 py-4 border-b border-mnt-default/40 grid grid-cols-[1fr_auto_auto_auto_auto] gap-4 items-center"
      >
        <div class="h-4 rounded bg-mnt-elevated/60 animate-pulse w-48" />
        <div class="h-5 rounded-full bg-mnt-elevated/60 animate-pulse w-20" />
        <div class="h-4 rounded bg-mnt-elevated/60 animate-pulse w-28" />
        <div class="h-4 rounded bg-mnt-elevated/60 animate-pulse w-10 mx-auto" />
        <div class="h-4 rounded bg-mnt-elevated/60 animate-pulse w-24" />
      </div>
    </template>

    <!-- Empty state -->
    <template v-else-if="policies.length === 0">
      <div class="flex flex-col items-center justify-center py-16">
        <Layers :size="36" class="text-mnt-muted mb-3" />
        <p class="text-sm text-mnt-muted font-medium">No escalation policies yet</p>
        <p class="text-[10px] text-mnt-muted mt-1">Create a policy to start routing alerts through escalation chains.</p>
        <UiButton variant="primary" size="sm" class="mt-5" @click="emit('create')">
          Create first policy
        </UiButton>
      </div>
    </template>

    <!-- Rows -->
    <template v-else>
      <div
        v-for="policy in policies"
        :key="policy.id"
        class="px-5 py-3.5 border-b border-mnt-default/40 last:border-0 grid grid-cols-[1fr_auto_auto_auto_auto] gap-4 items-center hover:bg-mnt-elevated transition-all cursor-pointer group"
        @click="emit('edit', policy)"
      >
        <!-- Name -->
        <span class="text-sm font-semibold text-mnt-primary group-hover:text-mnt-green-400 transition-colors truncate">
          {{ policy.name }}
        </span>

        <!-- Status badge — clickable to toggle -->
        <div class="w-20 flex justify-center">
          <ChipToggle
            :pressed="policy.active"
            tone="accent"
            size="sm"
            :title="policy.active ? 'Click to deactivate' : 'Click to activate'"
            @click.stop
            @toggle="emit('toggleActive', policy.id, !policy.active)"
          >
            <CheckCircle2 v-if="policy.active" :size="10" />
            <CircleDashed v-else :size="10" />
            {{ policy.active ? 'Active' : 'Inactive' }}
          </ChipToggle>
        </div>

        <!-- Severities -->
        <div class="w-28 text-center">
          <span class="text-xs text-mnt-muted">{{ severityLabel(policy.filters.severities) }}</span>
        </div>

        <!-- Level count -->
        <div class="w-20 text-center">
          <span class="text-xs font-bold text-mnt-secondary">{{ policy.levels.length }}</span>
        </div>

        <!-- Last modified + actions -->
        <div class="w-28 flex items-center justify-end gap-2">
          <span class="text-[10px] text-mnt-muted">{{ timeAgo(policy.updated_at) }}</span>
          <UiButton
            variant="ghost"
            size="sm"
            :icon="Pencil"
            title="Edit"
            aria-label="Edit"
            class="opacity-0 group-hover:opacity-100"
            @click.stop="emit('edit', policy)"
          />
          <UiButton
            variant="danger-ghost"
            size="sm"
            :icon="Trash2"
            title="Delete"
            aria-label="Delete"
            class="opacity-0 group-hover:opacity-100"
            @click.stop="handleDelete(policy)"
          />
        </div>
      </div>
    </template>
  </div>
</template>

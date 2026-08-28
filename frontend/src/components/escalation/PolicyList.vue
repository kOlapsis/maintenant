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
        <button
          class="mt-5 px-4 py-2 bg-mnt-green-600 hover:bg-mnt-green-500 text-mnt-inverted rounded-lg text-xs font-bold transition-all shadow-lg shadow-mnt-green-500/20"
          @click="emit('create')"
        >
          Create first policy
        </button>
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
          <button
            class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold border transition-all"
            :class="policy.active
              ? 'bg-mnt-green-500/10 text-mnt-green-400 border-mnt-green-500/20 hover:bg-mnt-green-500/20'
              : 'bg-mnt-elevated text-mnt-muted border-mnt-default hover:bg-mnt-elevated'"
            :title="policy.active ? 'Click to deactivate' : 'Click to activate'"
            @click.stop="emit('toggleActive', policy.id, !policy.active)"
          >
            <CheckCircle2 v-if="policy.active" :size="10" />
            <CircleDashed v-else :size="10" />
            {{ policy.active ? 'Active' : 'Inactive' }}
          </button>
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
          <button
            class="p-1 rounded text-mnt-muted hover:text-mnt-secondary hover:bg-mnt-elevated transition-all opacity-0 group-hover:opacity-100"
            title="Edit"
            @click.stop="emit('edit', policy)"
          >
            <Pencil :size="13" />
          </button>
          <button
            class="p-1 rounded text-mnt-muted hover:text-mnt-status-down hover:bg-mnt-status-down/10 transition-all opacity-0 group-hover:opacity-100"
            title="Delete"
            @click.stop="handleDelete(policy)"
          >
            <Trash2 :size="13" />
          </button>
        </div>
      </div>
    </template>
  </div>
</template>

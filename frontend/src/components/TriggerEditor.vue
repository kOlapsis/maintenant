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
import { computed, ref, watch } from 'vue'
import { X, Plus, Loader2, ArrowRight, Lock } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { useTriggersStore } from '@/stores/triggers'
import { useChannelsStore } from '@/stores/channels'
import { useEdition } from '@/composables/useEdition'
import type { AlertTrigger, TriggerRequest } from '@/types/triggers'
import EditionBadge from '@/components/EditionBadge.vue'

const props = defineProps<{
  trigger?: AlertTrigger | null
}>()

const emit = defineEmits<{
  saved: []
  cancel: []
}>()

const store = useTriggersStore()
const channelsStore = useChannelsStore()
const { hasFeature, requiredEditionFor } = useEdition()

// Advanced filters are a capability, not a tier: gating on the flag means the
// UI cannot claim something the backend would refuse, or the reverse.
const canUseAdvancedFilters = computed(() => hasFeature('alert_advanced_filters'))
const advancedFiltersEdition = computed(() => requiredEditionFor('alert_advanced_filters'))

const SEVERITY_OPTIONS = ['critical', 'warning']
const SOURCE_OPTIONS = ['container', 'endpoint', 'heartbeat', 'certificate', 'monitor', 'resource', 'update']

const name = ref(props.trigger?.name ?? '')
const enabled = ref(props.trigger?.enabled ?? true)
const severities = ref<string[]>(toCsvArray(props.trigger?.filter_severities ?? ''))
const sources = ref<string[]>(toCsvArray(props.trigger?.filter_sources ?? ''))
const scopesCsv = ref(props.trigger?.filter_scopes ?? '')
const tagsCsv = ref(props.trigger?.filter_tags ?? '')
const selectedChannelIds = ref<string[]>(props.trigger?.channel_ids ?? [])

const saving = ref(false)
const saveError = ref<string | null>(null)

watch(
  () => props.trigger,
  (t) => {
    name.value = t?.name ?? ''
    enabled.value = t?.enabled ?? true
    severities.value = toCsvArray(t?.filter_severities ?? '')
    sources.value = toCsvArray(t?.filter_sources ?? '')
    scopesCsv.value = t?.filter_scopes ?? ''
    tagsCsv.value = t?.filter_tags ?? ''
    selectedChannelIds.value = t?.channel_ids ?? []
  },
)

function toCsvArray(s: string): string[] {
  return s.split(',').map((x) => x.trim()).filter((x) => x.length > 0)
}

function toggleArray(arr: string[], value: string): string[] {
  const idx = arr.indexOf(value)
  if (idx >= 0) return arr.filter((x) => x !== value)
  return [...arr, value]
}

function toggleSeverity(s: string) {
  severities.value = toggleArray(severities.value, s)
}

function toggleSource(s: string) {
  sources.value = toggleArray(sources.value, s)
}

function toggleChannel(id: string) {
  if (selectedChannelIds.value.includes(id)) {
    selectedChannelIds.value = selectedChannelIds.value.filter((x) => x !== id)
  } else {
    selectedChannelIds.value = [...selectedChannelIds.value, id]
  }
}

const matchAll = computed(
  () =>
    severities.value.length === 0 &&
    sources.value.length === 0 &&
    scopesCsv.value.trim() === '' &&
    tagsCsv.value.trim() === '',
)

async function handleSave() {
  if (!name.value.trim()) {
    saveError.value = 'Trigger name is required.'
    return
  }
  if (selectedChannelIds.value.length === 0) {
    saveError.value = 'Select at least one channel.'
    return
  }
  saveError.value = null
  saving.value = true
  try {
    const req: TriggerRequest = {
      name: name.value.trim(),
      filter_severities: severities.value.join(','),
      filter_sources: sources.value.join(','),
      filter_scopes: scopesCsv.value.trim(),
      filter_tags: tagsCsv.value.trim(),
      enabled: enabled.value,
      channel_ids: selectedChannelIds.value,
    }
    if (props.trigger) {
      await store.update(props.trigger.id, req)
    } else {
      await store.create(req)
    }
    emit('saved')
  } catch (e) {
    saveError.value = e instanceof Error ? e.message : 'Failed to save trigger.'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="bg-mnt-surface rounded-2xl border border-mnt-default">
    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-4 border-b border-mnt-default">
      <h3 class="text-sm font-bold text-mnt-primary">
        {{ trigger ? 'Edit trigger' : 'New alert trigger' }}
      </h3>
      <button
        class="p-1 rounded text-mnt-muted hover:text-mnt-secondary hover:bg-mnt-elevated transition-all"
        @click="emit('cancel')"
      >
        <X :size="16" />
      </button>
    </div>

    <div class="p-5 space-y-6">
      <!-- Error -->
      <div
        v-if="saveError"
        class="px-4 py-3 rounded-lg bg-mnt-status-down/10 border border-mnt-status-down/30 text-xs text-mnt-status-down"
      >
        {{ saveError }}
      </div>

      <!-- No channels warning -->
      <div
        v-if="channelsStore.channels.length === 0"
        class="px-4 py-3 rounded-lg bg-mnt-status-warn border border-amber-500/30 text-xs text-mnt-status-warn flex items-start justify-between gap-3"
      >
        <span>
          No notification channels exist yet. Create one before defining a trigger.
        </span>
        <RouterLink
          to="/channels"
          class="inline-flex items-center gap-1.5 rounded-lg border border-amber-500/40 bg-mnt-status-warn px-3 py-1 text-[11px] font-bold text-mnt-status-warn hover:bg-mnt-sev-warning-solid/20 transition-colors"
        >
          Configure
          <ArrowRight :size="12" />
        </RouterLink>
      </div>

      <!-- Name -->
      <div class="space-y-1.5">
        <label class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
          Trigger name
        </label>
        <input
          v-model="name"
          type="text"
          placeholder="e.g. Critical containers"
          class="w-full bg-mnt-primary border border-mnt-default rounded-lg px-3 py-2 text-sm text-mnt-primary placeholder:text-mnt-muted focus:outline-none focus:border-mnt-default transition-colors"
        />
      </div>

      <!-- Active toggle -->
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm font-medium text-mnt-secondary">Enabled</p>
          <p class="text-[10px] text-mnt-muted mt-0.5">
            Disabled triggers stay configured but never dispatch.
          </p>
        </div>
        <button
          class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none"
          :class="enabled ? 'bg-mnt-green-600' : 'bg-mnt-elevated'"
          @click="enabled = !enabled"
        >
          <span
            class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform"
            :class="enabled ? 'translate-x-4' : 'translate-x-0'"
          />
        </button>
      </div>

      <!-- Channels -->
      <div class="space-y-2">
        <label class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
          Notify channels
          <span class="text-mnt-status-down/70 normal-case font-normal">*</span>
        </label>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="ch in channelsStore.channels"
            :key="ch.id"
            class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border transition-all"
            :class="
              selectedChannelIds.includes(ch.id)
                ? 'bg-mnt-green-500/10 border-mnt-green-500/30 text-mnt-green-400'
                : 'bg-transparent border-mnt-default text-mnt-muted hover:border-mnt-default hover:text-mnt-secondary'
            "
            @click="toggleChannel(ch.id)"
          >
            <Plus v-if="!selectedChannelIds.includes(ch.id)" :size="11" />
            {{ ch.name }}
          </button>
        </div>
      </div>

      <!-- Severities (CE) -->
      <div class="space-y-2">
        <label class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
          Severities <span class="text-mnt-muted normal-case font-normal">(empty = match all)</span>
        </label>
        <div class="flex gap-2">
          <button
            v-for="sev in SEVERITY_OPTIONS"
            :key="sev"
            class="px-3 py-1.5 rounded-lg text-xs font-bold border transition-all"
            :class="
              severities.includes(sev)
                ? sev === 'critical'
                  ? 'bg-mnt-status-down/15 border-mnt-status-down/40 text-mnt-status-down'
                  : 'bg-mnt-status-warn border-amber-500/40 text-mnt-status-warn'
                : 'bg-transparent border-mnt-default text-mnt-muted hover:border-mnt-default hover:text-mnt-muted'
            "
            @click="toggleSeverity(sev)"
          >
            {{ sev.charAt(0).toUpperCase() + sev.slice(1) }}
          </button>
        </div>
      </div>

      <!-- Sources (CE) -->
      <div class="space-y-2">
        <label class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
          Sources <span class="text-mnt-muted normal-case font-normal">(empty = match all)</span>
        </label>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="src in SOURCE_OPTIONS"
            :key="src"
            class="px-3 py-1.5 rounded-lg text-xs font-bold border transition-all capitalize"
            :class="
              sources.includes(src)
                ? 'bg-mnt-elevated border-mnt-default text-mnt-secondary'
                : 'bg-transparent border-mnt-default text-mnt-muted hover:border-mnt-default hover:text-mnt-muted'
            "
            @click="toggleSource(src)"
          >
            {{ src }}
          </button>
        </div>
      </div>

      <!-- Scopes / Tags — gated by the alert_advanced_filters capability -->
      <div class="space-y-3 rounded-xl border border-mnt-default bg-mnt-primary p-4">
        <div class="flex items-center justify-between">
          <label class="text-[10px] font-bold uppercase tracking-widest text-mnt-muted">
            Advanced filters
          </label>
          <span
            v-if="!canUseAdvancedFilters && advancedFiltersEdition"
            class="inline-flex items-center gap-1.5"
          >
            <Lock :size="10" class="text-mnt-muted" />
            <EditionBadge :edition="advancedFiltersEdition" />
          </span>
        </div>
        <p v-if="!canUseAdvancedFilters" class="text-xs text-mnt-muted">
          Filter triggers by per-entity scope (e.g. <code class="rounded bg-mnt-elevated px-1.5 py-0.5 text-[10px]">container:42</code>) or by tags.
        </p>

        <div class="space-y-1.5">
          <label class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
            Scopes (CSV)
          </label>
          <input
            v-model="scopesCsv"
            type="text"
            :disabled="!canUseAdvancedFilters"
            placeholder="container:42, endpoint:7"
            class="w-full bg-mnt-surface border border-mnt-default rounded-lg px-3 py-2 text-sm text-mnt-primary placeholder:text-mnt-muted focus:outline-none focus:border-mnt-default transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          />
        </div>

        <div class="space-y-1.5">
          <label class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
            Tags (CSV)
          </label>
          <input
            v-model="tagsCsv"
            type="text"
            :disabled="!canUseAdvancedFilters"
            placeholder="prod, payments"
            class="w-full bg-mnt-surface border border-mnt-default rounded-lg px-3 py-2 text-sm text-mnt-primary placeholder:text-mnt-muted focus:outline-none focus:border-mnt-default transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          />
        </div>
      </div>

      <!-- Match-all hint -->
      <div
        v-if="matchAll"
        class="px-3 py-2 rounded-lg bg-mnt-elevated/50 border border-mnt-default text-[11px] text-mnt-muted"
      >
        Without any filter, this trigger matches <strong>every alert</strong>.
      </div>

      <!-- Actions -->
      <div class="flex items-center justify-end gap-3 pt-2">
        <button
          class="px-4 py-2 text-xs font-medium text-mnt-muted hover:text-mnt-secondary transition-colors"
          @click="emit('cancel')"
        >
          Cancel
        </button>
        <button
          class="inline-flex items-center gap-2 px-4 py-2 bg-mnt-green-600 hover:bg-mnt-green-500 disabled:bg-mnt-elevated disabled:text-mnt-muted text-mnt-inverted rounded-lg text-xs font-bold transition-all shadow-lg shadow-mnt-green-500/20"
          :disabled="saving"
          @click="handleSave"
        >
          <Loader2 v-if="saving" :size="13" class="animate-spin" />
          {{ saving ? 'Saving...' : trigger ? 'Save changes' : 'Create trigger' }}
        </button>
      </div>
    </div>
  </div>
</template>

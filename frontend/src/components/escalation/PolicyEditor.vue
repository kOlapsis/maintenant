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
import { ref, computed, watch, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useEscalationStore } from '@/stores/escalation'
import { useTriggersStore } from '@/stores/triggers'
import { apiFetch } from '@/services/apiFetch'
import type { EscalationPolicy, OverlapWarning as OverlapWarningType } from '@/types/escalation'
import { X, Plus, Loader2, ArrowRight, Shield } from 'lucide-vue-next'
import LevelEditor from './LevelEditor.vue'
import OverlapWarningComponent from './OverlapWarning.vue'
import InlineAlert from '@/components/ui/InlineAlert.vue'
import { useEscalationApi } from '@/composables/useEscalationApi'

interface Channel {
  id: string
  name: string
  type: string
  enabled: boolean
}

const props = defineProps<{
  policy?: EscalationPolicy | null
  maxLevels?: number
}>()

const emit = defineEmits<{
  saved: []
  cancel: []
}>()

const store = useEscalationStore()
const triggersStore = useTriggersStore()
const escalationApi = useEscalationApi()

const name = ref(props.policy?.name ?? '')
const active = ref(props.policy?.active ?? true)
const severities = ref<string[]>(props.policy?.filters.severities ?? [])

const maxLevels = computed(() => props.maxLevels ?? 5)
const levels = ref<Array<{ delay_seconds: number; channel_ids: string[] }>>(
  props.policy?.levels.map((l) => ({ delay_seconds: l.delay_seconds, channel_ids: [...l.channel_ids] })) ??
    [{ delay_seconds: 300, channel_ids: [] }],
)

const channels = ref<Channel[]>([])
const channelsLoading = ref(false)
const saving = ref(false)
const saveError = ref<string | null>(null)
const overlapWarnings = ref<OverlapWarningType[]>([])

let debounceTimer: ReturnType<typeof setTimeout> | null = null

function buildCurrentPayload() {
  return {
    name: name.value.trim(),
    active: active.value,
    filters: {
      severities: severities.value,
      scopes: [],
      tags: [],
    },
    levels: levels.value.map((l) => ({
      delay_seconds: l.delay_seconds,
      channel_ids: l.channel_ids,
    })),
  }
}

function checkOverlap() {
  if (debounceTimer !== null) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(async () => {
    try {
      const res = await escalationApi.overlapProbe(buildCurrentPayload())
      overlapWarnings.value = res.overlapping
    } catch {
      // 403 in CE or other errors — ignore silently
    }
  }, 500)
}

watch([name, severities, levels], () => checkOverlap(), { deep: true })

const SEVERITY_OPTIONS = ['warning', 'critical']

function toggleSeverity(sev: string) {
  if (severities.value.includes(sev)) {
    severities.value = severities.value.filter((s) => s !== sev)
  } else {
    severities.value = [...severities.value, sev]
  }
}

function addLevel() {
  if (levels.value.length >= maxLevels.value) return
  const last = levels.value[levels.value.length - 1]
  const prevDelay = last?.delay_seconds ?? 300
  levels.value = [...levels.value, { delay_seconds: prevDelay + 300, channel_ids: [] }]
}

function removeLevel(index: number) {
  if (levels.value.length <= 1) return
  levels.value = levels.value.filter((_, i) => i !== index)
}

async function loadChannels() {
  channelsLoading.value = true
  try {
    const res = await apiFetch<{ channels: Channel[] }>('/api/v1/channels')
    channels.value = res.channels.filter((c) => c.enabled)
  } catch {
    channels.value = []
  } finally {
    channelsLoading.value = false
  }
}

async function handleSave() {
  if (!name.value.trim()) {
    saveError.value = 'Policy name is required.'
    return
  }
  for (const level of levels.value) {
    if (level.channel_ids.length === 0) {
      saveError.value = 'Each escalation level requires at least one notification channel.'
      return
    }
  }
  saveError.value = null
  saving.value = true
  try {
    const payload = {
      name: name.value.trim(),
      active: active.value,
      filters: {
        severities: severities.value,
        scopes: [],
        tags: [],
      },
      levels: levels.value.map((l) => ({
        delay_seconds: l.delay_seconds,
        channel_ids: l.channel_ids,
      })),
    }
    if (props.policy) {
      await store.updatePolicy(props.policy.id, payload)
    } else {
      await store.createPolicy(payload)
    }
    emit('saved')
  } catch (e) {
    saveError.value = e instanceof Error ? e.message : 'Failed to save policy.'
  } finally {
    saving.value = false
  }
}

// Channels referenced ONLY by escalation (no trigger serves them) → "reserved escalation"
const reservedChannelIds = computed(() => {
  const ids = new Set<string>()
  for (const lvl of levels.value) {
    for (const id of lvl.channel_ids) ids.add(id)
  }
  return [...ids].filter((id) => triggersStore.triggersForChannel(id).length === 0)
})

onMounted(() => {
  loadChannels()
  triggersStore.fetchTriggers()
})
</script>

<template>
  <div class="bg-[#12151C] rounded-2xl border border-slate-800">
    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-4 border-b border-slate-800">
      <h3 class="text-sm font-bold text-white">
        {{ policy ? 'Edit policy' : 'New escalation policy' }}
      </h3>
      <button
        class="p-1 rounded text-slate-500 hover:text-slate-300 hover:bg-slate-700/50 transition-all"
        @click="emit('cancel')"
      >
        <X :size="16" />
      </button>
    </div>

    <div class="p-5 space-y-6">
      <!-- Error -->
      <div
        v-if="saveError"
        class="px-4 py-3 rounded-lg bg-pb-status-down/10 border border-pb-status-down/30 text-xs text-pb-status-down"
      >
        {{ saveError }}
      </div>

      <!-- Name -->
      <div class="space-y-1.5">
        <label class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">
          Policy name
        </label>
        <input
          v-model="name"
          type="text"
          placeholder="e.g. Critical alerts on-call"
          class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-sm text-white placeholder:text-slate-600 focus:outline-none focus:border-slate-600 transition-colors"
        />
      </div>

      <!-- Active toggle -->
      <div class="flex items-center justify-between">
        <div>
          <p class="text-sm font-medium text-slate-200">Active</p>
          <p class="text-[10px] text-slate-500 mt-0.5">
            Inactive policies are saved but never triggered.
          </p>
        </div>
        <button
          class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none"
          :class="active ? 'bg-pb-green-600' : 'bg-slate-700'"
          @click="active = !active"
        >
          <span
            class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform"
            :class="active ? 'translate-x-4' : 'translate-x-0'"
          />
        </button>
      </div>

      <!-- Severities -->
      <div class="space-y-2">
        <label class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">
          Severities <span class="text-slate-600 normal-case font-normal">(empty = all)</span>
        </label>
        <div class="flex gap-2">
          <button
            v-for="sev in SEVERITY_OPTIONS"
            :key="sev"
            class="px-3 py-1.5 rounded-lg text-xs font-bold border transition-all"
            :class="
              severities.includes(sev)
                ? sev === 'critical'
                  ? 'bg-pb-status-down/15 border-pb-status-down/40 text-pb-status-down'
                  : 'bg-amber-500/15 border-amber-500/40 text-amber-400'
                : 'bg-transparent border-slate-700 text-slate-500 hover:border-slate-600 hover:text-slate-400'
            "
            @click="toggleSeverity(sev)"
          >
            {{ sev.charAt(0).toUpperCase() + sev.slice(1) }}
          </button>
        </div>
      </div>

      <!-- No channels warning -->
      <InlineAlert
        v-if="!channelsLoading && channels.length === 0"
        severity="warning"
        tag="SETUP REQUIRED"
      >
        <template #title>No notification channels configured</template>
        Escalation policies need at least one notification channel (Webhook, Slack, Teams, Email)
        to deliver alerts. Create one before defining your levels.
        <template #action>
          <RouterLink
            to="/channels"
            class="inline-flex items-center gap-1.5 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-1.5 text-[11px] font-bold text-amber-300 hover:bg-amber-500/20 transition-colors"
          >
            Configure a channel
            <ArrowRight :size="12" />
          </RouterLink>
        </template>
      </InlineAlert>

      <!-- Reserved-escalation channels notice -->
      <div
        v-if="reservedChannelIds.length > 0"
        class="rounded-xl border border-pb-green-500/30 bg-pb-green-500/5 px-4 py-3 flex items-start gap-3"
      >
        <Shield :size="14" class="text-pb-green-400 mt-0.5 shrink-0" />
        <div class="text-xs text-slate-300 space-y-1">
          <p class="font-medium text-pb-green-400">Reserved-escalation channels detected</p>
          <p class="text-slate-400 leading-relaxed">
            <template v-for="(id, idx) in reservedChannelIds" :key="id">
              <span class="text-slate-200 font-medium">
                {{ channels.find((c) => c.id === id)?.name ?? `Channel #${id}` }}
              </span>
              <span v-if="idx < reservedChannelIds.length - 1">, </span>
            </template>
            do not appear in any active trigger and will only be notified by this escalation —
            this is the intended pattern for last-resort destinations (manager, on-call, CTO, etc).
          </p>
        </div>
      </div>

      <!-- Levels -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <label class="text-[10px] text-slate-500 font-bold uppercase tracking-widest block">
            Escalation levels
          </label>
          <span class="text-[10px] text-slate-600">{{ levels.length }}/{{ maxLevels }} levels</span>
        </div>

        <div v-if="channelsLoading" class="flex items-center gap-2 text-xs text-slate-500 py-2">
          <Loader2 :size="13" class="animate-spin" />
          Loading channels...
        </div>

        <template v-else>
          <LevelEditor
            v-for="(level, i) in levels"
            :key="i"
            :model-value="level"
            :channels="channels"
            :index="i"
            :can-remove="levels.length > 1"
            @update:model-value="(v) => { levels[i] = v }"
            @remove="removeLevel(i)"
          />

          <button
            v-if="levels.length < maxLevels"
            class="w-full py-2.5 rounded-xl border border-dashed border-slate-700 text-xs text-slate-500 hover:text-slate-300 hover:border-slate-600 transition-all flex items-center justify-center gap-1.5"
            @click="addLevel"
          >
            <Plus :size="13" />
            Add level
          </button>
        </template>
      </div>

      <!-- Overlap warnings -->
      <OverlapWarningComponent :warnings="overlapWarnings" />

      <!-- Actions -->
      <div class="flex items-center justify-end gap-3 pt-2">
        <button
          class="px-4 py-2 text-xs font-medium text-slate-400 hover:text-slate-200 transition-colors"
          @click="emit('cancel')"
        >
          Cancel
        </button>
        <button
          class="inline-flex items-center gap-2 px-4 py-2 bg-pb-green-600 hover:bg-pb-green-500 disabled:bg-slate-700 disabled:text-slate-500 text-slate-950 rounded-lg text-xs font-bold transition-all shadow-lg shadow-pb-green-500/20"
          :disabled="saving"
          @click="handleSave"
        >
          <Loader2 v-if="saving" :size="13" class="animate-spin" />
          {{ saving ? 'Saving...' : 'Save policy' }}
        </button>
      </div>
    </div>
  </div>
</template>

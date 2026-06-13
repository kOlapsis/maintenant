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
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import FeatureGate from '@/components/FeatureGate.vue'
import PolicyList from '@/components/escalation/PolicyList.vue'
import PolicyEditor from '@/components/escalation/PolicyEditor.vue'
import { useEscalationStore } from '@/stores/escalation'
import type { EscalationPolicy } from '@/types/escalation'
import { Plus, BellRing, CheckCircle2, ShieldAlert } from 'lucide-vue-next'

const store = useEscalationStore()

const showEditor = ref(false)
const editingPolicy = ref<EscalationPolicy | null>(null)

function openCreate() {
  editingPolicy.value = null
  showEditor.value = true
}

function openEdit(policy: EscalationPolicy) {
  editingPolicy.value = policy
  showEditor.value = true
}

function closeEditor() {
  showEditor.value = false
  editingPolicy.value = null
}

async function handleSaved() {
  closeEditor()
  await store.fetchPolicies()
}

async function handleDelete(id: string) {
  await store.deletePolicy(id)
}

async function handleToggleActive(id: string, active: boolean) {
  await store.setPolicyActive(id, active)
}

onMounted(() => {
  store.fetchPolicies()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">

      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-pb-primary">Escalation Policies</h1>
          <p class="mt-1 text-sm text-pb-muted">
            Multi-level alert routing with automatic escalation chains
          </p>
        </div>
        <div v-if="store.limits" class="flex items-center gap-3">
          <span class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">
            <template v-if="store.limits.max_active === -1">
              {{ store.limits.current_active }} active
            </template>
            <template v-else>
              {{ store.limits.current_active }}/{{ store.limits.max_active }} active
            </template>
          </span>
        </div>
      </div>

      <FeatureGate feature="alert_escalation">
        <!-- Active slot (Pro) -->
        <div class="space-y-4">
          <!-- Action bar -->
          <div class="flex items-center justify-between">
            <p class="text-xs text-pb-muted">
              <template v-if="store.policies.length > 0">
                {{ store.policies.length }} {{ store.policies.length === 1 ? 'policy' : 'policies' }}
              </template>
            </p>
            <button
              v-if="!showEditor"
              class="inline-flex items-center gap-2 px-4 py-2 bg-pb-green-600 hover:bg-pb-green-500 text-pb-inverted rounded-lg text-xs font-bold transition-all shadow-lg shadow-pb-green-500/20"
              @click="openCreate"
            >
              <Plus :size="13" />
              New policy
            </button>
          </div>

          <!-- Editor -->
          <PolicyEditor
            v-if="showEditor"
            :policy="editingPolicy"
            @saved="handleSaved"
            @cancel="closeEditor"
          />

          <!-- List -->
          <PolicyList
            :policies="store.policies"
            :loading="store.loading"
            @create="openCreate"
            @edit="openEdit"
            @delete="handleDelete"
            @toggle-active="handleToggleActive"
          />

          <!-- Error -->
          <div
            v-if="store.error"
            class="px-4 py-3 rounded-lg bg-pb-status-down/10 border border-pb-status-down/30 text-xs text-pb-status-down"
          >
            {{ store.error }}
          </div>
        </div>

        <!-- Placeholder slot (Community Edition) -->
        <template #placeholder>
          <div class="bg-pb-surface rounded-2xl border border-pb-default overflow-hidden">
            <div class="px-6 py-10 flex flex-col items-center text-center">
              <div class="w-12 h-12 rounded-xl bg-pb-green-500/10 border border-pb-green-500/20 flex items-center justify-center mb-4">
                <BellRing :size="22" class="text-pb-green-400" />
              </div>
              <h2 class="text-base font-bold text-pb-primary mb-1">Escalation policies</h2>
              <p class="text-sm text-pb-muted max-w-md mb-6 leading-relaxed">
                Automatically escalate unacknowledged alerts through a chain of notification levels — each with a configurable delay and a distinct set of channels.
              </p>

              <ul class="text-left space-y-3 mb-8 w-full max-w-sm">
                <li class="flex items-start gap-3">
                  <CheckCircle2 :size="15" class="text-pb-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-pb-secondary">
                    Multi-level chains: page different teams at escalating delays
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <CheckCircle2 :size="15" class="text-pb-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-pb-secondary">
                    Acknowledgment or resolution automatically stops the escalation
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <ShieldAlert :size="15" class="text-pb-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-pb-secondary">
                    Full audit trail: every delivery attempt logged with status and error
                  </span>
                </li>
              </ul>

              <RouterLink
                to="/pro-edition"
                class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg text-sm font-semibold bg-pb-green-600 hover:bg-pb-green-500 text-pb-inverted shadow-lg shadow-pb-green-500/20 transition-colors"
              >
                Unlock with Pro
              </RouterLink>
            </div>
          </div>
        </template>
      </FeatureGate>

    </div>
  </div>
</template>

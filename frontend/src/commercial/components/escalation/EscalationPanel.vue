<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { ref } from 'vue'

import PolicyList from './PolicyList.vue'
import PolicyEditor from './PolicyEditor.vue'
import { useEscalationStore } from '@/commercial/stores/escalation'
import type { EscalationPolicy } from '@/commercial/types/escalation'
import UiButton from '@/components/ui/UiButton.vue'
import { Plus } from 'lucide-vue-next'

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
</script>

<template>
  <div class="space-y-4">
    <!-- Action bar -->
    <div class="flex items-center justify-between">
      <p class="text-xs text-mnt-muted">
        <template v-if="store.policies.length > 0">
          {{ store.policies.length }} {{ store.policies.length === 1 ? 'policy' : 'policies' }}
        </template>
      </p>
      <UiButton v-if="!showEditor" variant="primary" size="sm" :icon="Plus" @click="openCreate">
        New policy
      </UiButton>
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
      class="px-4 py-3 rounded-lg bg-mnt-status-down/10 border border-mnt-status-down/30 text-xs text-mnt-status-down"
    >
      {{ store.error }}
    </div>
  </div>
</template>

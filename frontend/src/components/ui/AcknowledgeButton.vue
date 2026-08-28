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
import { ref } from 'vue'
import { useAlertsStore } from '@/stores/alerts'
import type { Alert } from '@/services/alertApi'

const props = defineProps<{
  alert: Alert
}>()

const store = useAlertsStore()
const pending = ref(false)

async function acknowledge() {
  if (pending.value) return
  pending.value = true
  try {
    await store.acknowledgeAlert(props.alert.id)
  } catch (e) {
    console.error('Failed to acknowledge alert:', e)
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <!-- Seule une alerte active peut être acquittée ; sinon on ne rend rien. -->
  <template v-if="alert.status === 'active'">
    <!-- Confirmation passive (pas un bouton) -->
    <span
      v-if="alert.acknowledged_at"
      class="ack-done"
      :title="alert.acknowledged_by ? `Acquittée par ${alert.acknowledged_by}` : 'Acquittée'"
    >
      <svg width="13" height="13" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="2.5 7.5 5.5 10.5 11.5 4" />
      </svg>
      Acquittée
    </span>

    <!-- Action -->
    <button
      v-else
      type="button"
      class="ack-btn"
      :disabled="pending"
      @click.stop="acknowledge"
    >
      <svg width="13" height="13" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="2.5 7.5 5.5 10.5 11.5 4" />
      </svg>
      {{ pending ? 'Acquittement…' : 'Acquitter' }}
    </button>
  </template>
</template>

<style scoped>
/* Action : chrome de bouton explicite — fond opaque surélevé qui ressort sur
   les cartes d'alerte translucides, bordure, et hover qui « s'allume » en accent. */
.ack-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  flex-shrink: 0;
  border-radius: var(--mnt-radius-md);
  border: 1px solid var(--mnt-border-default);
  background: var(--mnt-bg-elevated);
  color: var(--mnt-text-primary);
  padding: 0.25rem 0.625rem;
  font-size: 0.75rem;
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  box-shadow: var(--mnt-shadow-card);
  transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}
.ack-btn svg {
  color: var(--mnt-accent);
  transition: color 0.15s ease;
}
.ack-btn:hover {
  background: var(--mnt-bg-hover);
  border-color: var(--mnt-accent);
  color: var(--mnt-accent);
}
.ack-btn:active {
  transform: translateY(1px);
}
.ack-btn:disabled {
  opacity: 0.55;
  cursor: progress;
  box-shadow: none;
}

/* État acquitté : confirmation plate et muette, sans affordance d'action. */
.ack-done {
  display: inline-flex;
  align-items: center;
  gap: 0.3125rem;
  flex-shrink: 0;
  font-size: 0.75rem;
  font-weight: 500;
  white-space: nowrap;
  color: var(--mnt-text-muted);
}
.ack-done svg {
  color: var(--mnt-accent);
}
</style>

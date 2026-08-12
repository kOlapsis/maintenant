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
import type { Edition } from '@/services/editionApi'

/**
 * The edition pill, in three visually distinct states (FR-041). Used for the
 * current edition in the header and for the edition a locked feature requires,
 * so the two can never disagree on how an edition looks.
 *
 * An edition this build does not know still renders — capitalised, in the
 * neutral style — rather than falling back to "Community", which would be a lie.
 */
const props = withDefaults(
  defineProps<{
    edition: Edition
    size?: 'sm' | 'md'
  }>(),
  { size: 'sm' },
)

const label = computed(() => {
  const raw = String(props.edition || '')
  if (!raw) return 'Community'
  return raw.charAt(0).toUpperCase() + raw.slice(1)
})

const tone = computed(() => {
  switch (props.edition) {
    case 'pro':
      return 'edition-pro'
    case 'personal':
      return 'edition-personal'
    case 'community':
      return 'edition-community'
    default:
      return 'edition-unknown'
  }
})
</script>

<template>
  <span
    class="inline-flex items-center rounded-full font-semibold whitespace-nowrap"
    :class="[tone, size === 'md' ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-[10px]']"
  >
    {{ label }}
  </span>
</template>

<style scoped>
/* Community reads as neutral, Personal and Pro as two distinct paid tiers. */
.edition-community {
  background: var(--mnt-bg-elevated);
  color: var(--mnt-text-muted);
  border: 1px solid var(--mnt-border-default);
}

.edition-personal {
  background: var(--mnt-edition-personal-bg);
  color: var(--mnt-edition-personal-text);
  border: 1px solid var(--mnt-edition-personal-border);
}

.edition-pro {
  background: var(--mnt-sev-ok-bg);
  color: var(--mnt-sev-ok-text);
  border: 1px solid var(--mnt-sev-ok-border);
}

.edition-unknown {
  background: var(--mnt-bg-elevated);
  color: var(--mnt-text-secondary);
  border: 1px dashed var(--mnt-border-default);
}
</style>

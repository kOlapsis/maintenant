<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import TextInput from './TextInput.vue'

defineProps<{ label?: string }>()

const model = defineModel<string>({ default: '' })
</script>

<template>
  <div class="flex items-center gap-3">
    <input
      type="color"
      :value="model"
      :aria-label="label ? `${label} swatch` : 'Color swatch'"
      class="h-8 w-8 cursor-pointer rounded border border-mnt-default bg-transparent"
      @input="model = ($event.target as HTMLInputElement).value"
    />
    <TextInput
      :model-value="model"
      maxlength="9"
      mono
      size="sm"
      class="w-28"
      @change="(e: Event) => (model = (e.target as HTMLInputElement).value)"
    />
  </div>
</template>

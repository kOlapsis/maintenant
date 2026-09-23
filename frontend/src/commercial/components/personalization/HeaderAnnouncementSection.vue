<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import TextareaInput from '@/components/ui/TextareaInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'

const enabled = defineModel<boolean>('enabled', { required: true })
const messageMD = defineModel<string>('messageMD', { required: true })
const url = defineModel<string>('url', { required: true })
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Header Announcement</h3>

    <CheckboxInput v-model="enabled" label="Show announcement banner" />

    <div v-if="enabled" class="space-y-3">
      <FormField label="Message (Markdown)" :hint="`${messageMD.length}/1000 — Bold, italic, and links allowed.`">
        <template #default="{ id, describedBy, invalid }">
          <TextareaInput :id="id" v-model="messageMD" maxlength="1000" :rows="3" mono :aria-describedby="describedBy" :invalid="invalid" placeholder="**Scheduled maintenance** on 12 May at 22:00 UTC." />
        </template>
      </FormField>

      <FormField label="Link URL (optional)" hint="Must start with https:// — clicking the banner opens this URL.">
        <template #default="{ id, describedBy, invalid }">
          <TextInput :id="id" v-model="url" type="url" :aria-describedby="describedBy" :invalid="invalid" placeholder="https://acme.example/maintenance" />
        </template>
      </FormField>
    </div>
  </div>
</template>

<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import TextInput from '@/components/ui/TextInput.vue'
import RadioGroup from '@/components/ui/RadioGroup.vue'

const locale = defineModel<string>('locale', { required: true })
const timezone = defineModel<string>('timezone', { required: true })
const dateFormat = defineModel<string>('dateFormat', { required: true })

const localeOptions = [
  { value: 'en', label: 'English (en)' },
  { value: 'fr', label: 'Français (fr)' },
]

const dateFormatOptions = [
  { value: 'relative', label: 'Relative', hint: '2 minutes ago' },
  { value: 'absolute', label: 'Absolute', hint: '5 mai 2026 14:32' },
]
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Localization</h3>

    <FormField label="Language">
      <template #default="{ id, describedBy, invalid }">
        <SelectInput :id="id" v-model="locale" :options="localeOptions" :aria-describedby="describedBy" :invalid="invalid" />
      </template>
    </FormField>

    <div>
      <FormField label="Timezone">
        <template #default="{ id, describedBy, invalid }">
          <TextInput :id="id" v-model="timezone" :aria-describedby="describedBy" :invalid="invalid" placeholder="Europe/Paris (leave empty for browser timezone)" />
        </template>
      </FormField>
      <p class="text-[11px] text-mnt-muted mt-1">IANA timezone identifier, e.g. <code class="text-mnt-muted">America/New_York</code></p>
    </div>

    <div>
      <p class="block text-xs text-mnt-muted mb-2">Date Format</p>
      <RadioGroup v-model="dateFormat" :options="dateFormatOptions" ariaLabel="Date Format" inline />
    </div>
  </div>
</template>

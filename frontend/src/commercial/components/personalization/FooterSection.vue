<script setup lang="ts">
import { ref } from 'vue'
import { ArrowUp, ArrowDown, X } from 'lucide-vue-next'
import { usePersonalizationStore } from '@/commercial/stores/personalization'
import FormField from '@/components/ui/FormField.vue'
import TextareaInput from '@/components/ui/TextareaInput.vue'
import TextInput from '@/components/ui/TextInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

const store = usePersonalizationStore()
const footerTextMD = defineModel<string>('footerTextMD', { required: true })
const linkError = ref('')

async function addLink() {
  try {
    await store.createFooterLink('New link', 'https://')
    await store.fetchFooterLinks()
  } catch (e) {
    linkError.value = e instanceof Error ? e.message : 'Failed to add link'
  }
}

async function removeLink(id: string) {
  await store.deleteFooterLink(id)
}

async function moveLink(from: number, to: number) {
  const ids = store.footerLinks.map((l) => l.id)
  const [item] = ids.splice(from, 1)
  ids.splice(to, 0, item!)
  await store.reorderFooterLinks(ids)
}
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Footer</h3>

    <FormField label="Footer Text (Markdown)" :hint="`${footerTextMD.length}/500`">
      <template #default="{ id, describedBy, invalid }">
        <TextareaInput :id="id" v-model="footerTextMD" maxlength="500" :rows="3" mono :aria-describedby="describedBy" :invalid="invalid" placeholder="© 2026 Acme — [Privacy](https://acme.example/privacy)" />
      </template>
    </FormField>

    <div class="space-y-2">
      <p class="text-xs text-mnt-muted">External Links</p>
      <div
        v-for="(link, idx) in store.footerLinks"
        :key="link.id"
        class="flex items-center gap-2 bg-mnt-primary border border-mnt-default rounded-lg px-3 py-2"
      >
        <span class="text-mnt-muted text-xs w-4">{{ idx + 1 }}</span>
        <TextInput
          :model-value="link.label"
          class="flex-1"
          placeholder="Label"
          @change="(e: Event) => store.updateFooterLink(link.id, (e.target as HTMLInputElement).value, link.url)"
        />
        <TextInput
          :model-value="link.url"
          class="flex-1"
          placeholder="https://"
          @change="(e: Event) => store.updateFooterLink(link.id, link.label, (e.target as HTMLInputElement).value)"
        />
        <div class="flex gap-1">
          <UiButton
            v-if="idx > 0"
            variant="ghost"
            size="sm"
            :icon="ArrowUp"
            aria-label="Move link up"
            @click="moveLink(idx, idx - 1)"
          />
          <UiButton
            v-if="idx < store.footerLinks.length - 1"
            variant="ghost"
            size="sm"
            :icon="ArrowDown"
            aria-label="Move link down"
            @click="moveLink(idx, idx + 1)"
          />
          <UiButton variant="ghost" size="sm" :icon="X" aria-label="Remove link" @click="removeLink(link.id)" />
        </div>
      </div>
      <UiButton variant="secondary" size="sm" @click="addLink">+ Add link</UiButton>
      <p v-if="linkError" class="text-xs text-mnt-status-down">{{ linkError }}</p>
    </div>
  </div>
</template>

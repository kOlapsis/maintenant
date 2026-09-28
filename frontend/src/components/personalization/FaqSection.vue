<script setup lang="ts">
import { ref } from 'vue'
import { ArrowUp, ArrowDown, X } from 'lucide-vue-next'
import { usePersonalizationStore } from '@/stores/personalization'
import TextInput from '@/components/ui/TextInput.vue'
import TextareaInput from '@/components/ui/TextareaInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

const store = usePersonalizationStore()
const faqError = ref('')

async function addItem() {
  try {
    await store.createFAQItem('New question', 'Answer here.')
    await store.fetchFAQ()
  } catch (e) {
    faqError.value = e instanceof Error ? e.message : 'Failed to add item'
  }
}

async function removeItem(id: string) {
  await store.deleteFAQItem(id)
}

async function moveItem(from: number, to: number) {
  const ids = store.faqItems.map((f) => f.id)
  const [item] = ids.splice(from, 1)
  ids.splice(to, 0, item!)
  await store.reorderFAQ(ids)
}
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">FAQ</h3>

    <div class="space-y-3">
      <div
        v-for="(item, idx) in store.faqItems"
        :key="item.id"
        class="bg-mnt-primary border border-mnt-default rounded-xl p-4 space-y-2"
      >
        <div class="flex items-center gap-2">
          <span class="text-mnt-muted text-xs w-4">{{ idx + 1 }}</span>
          <TextInput
            :model-value="item.question"
            maxlength="200"
            class="flex-1 font-medium"
            placeholder="Question"
            @change="(e: Event) => store.updateFAQItem(item.id, (e.target as HTMLInputElement).value, item.answer_md)"
          />
          <div class="flex gap-1">
            <UiButton
              v-if="idx > 0"
              variant="ghost"
              size="sm"
              :icon="ArrowUp"
              aria-label="Move item up"
              @click="moveItem(idx, idx - 1)"
            />
            <UiButton
              v-if="idx < store.faqItems.length - 1"
              variant="ghost"
              size="sm"
              :icon="ArrowDown"
              aria-label="Move item down"
              @click="moveItem(idx, idx + 1)"
            />
            <UiButton variant="ghost" size="sm" :icon="X" aria-label="Remove item" @click="removeItem(item.id)" />
          </div>
        </div>
        <TextareaInput
          :model-value="item.answer_md"
          maxlength="4000"
          :rows="3"
          mono
          placeholder="Answer in Markdown…"
          @change="(e: Event) => store.updateFAQItem(item.id, item.question, (e.target as HTMLTextAreaElement).value)"
        />
      </div>
    </div>

    <UiButton variant="secondary" size="sm" @click="addItem">+ Add FAQ item</UiButton>
    <p v-if="faqError" class="text-xs text-mnt-status-down">{{ faqError }}</p>
  </div>
</template>

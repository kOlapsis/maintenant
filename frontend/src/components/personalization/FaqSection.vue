<script setup lang="ts">
import { ref } from 'vue'
import { usePersonalizationStore } from '@/stores/personalization'

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
          <input
            :value="item.question"
            maxlength="200"
            class="flex-1 bg-transparent text-mnt-primary text-sm focus:outline-none font-medium"
            placeholder="Question"
            @change="(e: Event) => store.updateFAQItem(item.id, (e.target as HTMLInputElement).value, item.answer_md)"
          />
          <div class="flex gap-1">
            <button
              v-if="idx > 0"
              class="text-mnt-muted hover:text-mnt-secondary text-xs px-1"
              @click="moveItem(idx, idx - 1)"
            >↑</button>
            <button
              v-if="idx < store.faqItems.length - 1"
              class="text-mnt-muted hover:text-mnt-secondary text-xs px-1"
              @click="moveItem(idx, idx + 1)"
            >↓</button>
            <button
              class="text-mnt-muted hover:text-mnt-status-down text-xs px-1"
              @click="removeItem(item.id)"
            >✕</button>
          </div>
        </div>
        <textarea
          :value="item.answer_md"
          maxlength="4000"
          rows="3"
          class="w-full bg-mnt-surface border border-mnt-default rounded-lg px-3 py-2 text-mnt-secondary text-sm font-mono focus:outline-none focus:border-mnt-default resize-none"
          placeholder="Answer in Markdown…"
          @change="(e: Event) => store.updateFAQItem(item.id, item.question, (e.target as HTMLTextAreaElement).value)"
        />
      </div>
    </div>

    <button
      class="text-xs px-3 py-1.5 border border-mnt-default rounded text-mnt-muted hover:text-mnt-primary hover:border-mnt-default"
      @click="addItem"
    >
      + Add FAQ item
    </button>
    <p v-if="faqError" class="text-xs text-mnt-status-down">{{ faqError }}</p>
  </div>
</template>

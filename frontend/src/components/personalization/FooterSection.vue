<script setup lang="ts">
import { ref } from 'vue'
import { usePersonalizationStore } from '@/stores/personalization'

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

async function removeLink(id: number) {
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
    <h3 class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Footer</h3>

    <div>
      <label class="block text-xs text-slate-400 mb-1">Footer Text (Markdown)</label>
      <textarea
        v-model="footerTextMD"
        maxlength="500"
        rows="3"
        class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm font-mono focus:outline-none focus:border-slate-600 resize-none"
        placeholder="© 2026 Acme — [Privacy](https://acme.example/privacy)"
      />
      <p class="text-[11px] text-slate-600 mt-1">{{ footerTextMD.length }}/500</p>
    </div>

    <div class="space-y-2">
      <p class="text-xs text-slate-400">External Links</p>
      <div
        v-for="(link, idx) in store.footerLinks"
        :key="link.id"
        class="flex items-center gap-2 bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2"
      >
        <span class="text-slate-600 text-xs w-4">{{ idx + 1 }}</span>
        <input
          :value="link.label"
          class="flex-1 bg-transparent text-white text-sm focus:outline-none"
          placeholder="Label"
          @change="(e: Event) => store.updateFooterLink(link.id, (e.target as HTMLInputElement).value, link.url)"
        />
        <input
          :value="link.url"
          class="flex-1 bg-transparent text-slate-400 text-sm focus:outline-none"
          placeholder="https://"
          @change="(e: Event) => store.updateFooterLink(link.id, link.label, (e.target as HTMLInputElement).value)"
        />
        <div class="flex gap-1">
          <button
            v-if="idx > 0"
            class="text-slate-600 hover:text-slate-300 text-xs px-1"
            @click="moveLink(idx, idx - 1)"
          >↑</button>
          <button
            v-if="idx < store.footerLinks.length - 1"
            class="text-slate-600 hover:text-slate-300 text-xs px-1"
            @click="moveLink(idx, idx + 1)"
          >↓</button>
          <button
            class="text-slate-600 hover:text-red-400 text-xs px-1"
            @click="removeLink(link.id)"
          >✕</button>
        </div>
      </div>
      <button
        class="text-xs px-3 py-1.5 border border-slate-700 rounded text-slate-400 hover:text-white hover:border-slate-500"
        @click="addLink"
      >
        + Add link
      </button>
      <p v-if="linkError" class="text-xs text-red-400">{{ linkError }}</p>
    </div>
  </div>
</template>

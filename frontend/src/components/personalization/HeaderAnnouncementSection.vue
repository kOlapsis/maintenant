<script setup lang="ts">
const enabled = defineModel<boolean>('enabled', { required: true })
const messageMD = defineModel<string>('messageMD', { required: true })
const url = defineModel<string>('url', { required: true })
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Header Announcement</h3>

    <label class="flex items-center gap-2 cursor-pointer">
      <input v-model="enabled" type="checkbox" class="w-4 h-4 accent-pb-green-500" />
      <span class="text-sm text-slate-300">Show announcement banner</span>
    </label>

    <div v-if="enabled" class="space-y-3">
      <div>
        <label class="block text-xs text-slate-400 mb-1">Message (Markdown)</label>
        <textarea
          v-model="messageMD"
          maxlength="1000"
          rows="3"
          class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm font-mono focus:outline-none focus:border-slate-600 resize-none"
          placeholder="**Scheduled maintenance** on 12 May at 22:00 UTC."
        />
        <p class="text-[11px] text-slate-600 mt-1">{{ messageMD.length }}/1000 — Bold, italic, and links allowed.</p>
      </div>

      <div>
        <label class="block text-xs text-slate-400 mb-1">Link URL (optional)</label>
        <input
          v-model="url"
          type="url"
          class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-slate-600"
          placeholder="https://acme.example/maintenance"
        />
        <p class="text-[11px] text-slate-600 mt-1">Must start with https:// — clicking the banner opens this URL.</p>
      </div>
    </div>
  </div>
</template>

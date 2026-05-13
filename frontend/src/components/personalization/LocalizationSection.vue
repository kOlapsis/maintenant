<script setup lang="ts">
const locale = defineModel<string>('locale', { required: true })
const timezone = defineModel<string>('timezone', { required: true })
const dateFormat = defineModel<string>('dateFormat', { required: true })
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Localization</h3>

    <div>
      <label class="block text-xs text-slate-400 mb-1">Language</label>
      <select
        v-model="locale"
        class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-slate-600"
      >
        <option value="en">English (en)</option>
        <option value="fr">Français (fr)</option>
      </select>
    </div>

    <div>
      <label class="block text-xs text-slate-400 mb-1">Timezone</label>
      <input
        v-model="timezone"
        type="text"
        class="w-full bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-slate-600"
        placeholder="Europe/Paris (leave empty for browser timezone)"
      />
      <p class="text-[11px] text-slate-600 mt-1">IANA timezone identifier, e.g. <code class="text-slate-500">America/New_York</code></p>
    </div>

    <div>
      <label class="block text-xs text-slate-400 mb-2">Date Format</label>
      <div class="flex gap-4">
        <label class="flex items-center gap-2 cursor-pointer">
          <input v-model="dateFormat" type="radio" value="relative" class="accent-pb-green-500" />
          <span class="text-sm text-slate-300">Relative <span class="text-slate-500 text-xs">(2 minutes ago)</span></span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input v-model="dateFormat" type="radio" value="absolute" class="accent-pb-green-500" />
          <span class="text-sm text-slate-300">Absolute <span class="text-slate-500 text-xs">(5 mai 2026 14:32)</span></span>
        </label>
      </div>
    </div>
  </div>
</template>

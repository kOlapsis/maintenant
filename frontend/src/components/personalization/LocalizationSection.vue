<script setup lang="ts">
const locale = defineModel<string>('locale', { required: true })
const timezone = defineModel<string>('timezone', { required: true })
const dateFormat = defineModel<string>('dateFormat', { required: true })
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Localization</h3>

    <div>
      <label class="block text-xs text-pb-muted mb-1">Language</label>
      <select
        v-model="locale"
        class="w-full bg-pb-primary border border-pb-default rounded-lg px-3 py-2 text-pb-primary text-sm focus:outline-none focus:border-pb-default"
      >
        <option value="en">English (en)</option>
        <option value="fr">Français (fr)</option>
      </select>
    </div>

    <div>
      <label class="block text-xs text-pb-muted mb-1">Timezone</label>
      <input
        v-model="timezone"
        type="text"
        class="w-full bg-pb-primary border border-pb-default rounded-lg px-3 py-2 text-pb-primary text-sm focus:outline-none focus:border-pb-default"
        placeholder="Europe/Paris (leave empty for browser timezone)"
      />
      <p class="text-[11px] text-pb-muted mt-1">IANA timezone identifier, e.g. <code class="text-pb-muted">America/New_York</code></p>
    </div>

    <div>
      <label class="block text-xs text-pb-muted mb-2">Date Format</label>
      <div class="flex gap-4">
        <label class="flex items-center gap-2 cursor-pointer">
          <input v-model="dateFormat" type="radio" value="relative" class="accent-pb-green-500" />
          <span class="text-sm text-pb-secondary">Relative <span class="text-pb-muted text-xs">(2 minutes ago)</span></span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input v-model="dateFormat" type="radio" value="absolute" class="accent-pb-green-500" />
          <span class="text-sm text-pb-secondary">Absolute <span class="text-pb-muted text-xs">(5 mai 2026 14:32)</span></span>
        </label>
      </div>
    </div>
  </div>
</template>

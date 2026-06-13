<script setup lang="ts">
const locale = defineModel<string>('locale', { required: true })
const timezone = defineModel<string>('timezone', { required: true })
const dateFormat = defineModel<string>('dateFormat', { required: true })
</script>

<template>
  <div class="space-y-4">
    <h3 class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Localization</h3>

    <div>
      <label class="block text-xs text-mnt-muted mb-1">Language</label>
      <select
        v-model="locale"
        class="w-full bg-mnt-primary border border-mnt-default rounded-lg px-3 py-2 text-mnt-primary text-sm focus:outline-none focus:border-mnt-default"
      >
        <option value="en">English (en)</option>
        <option value="fr">Français (fr)</option>
      </select>
    </div>

    <div>
      <label class="block text-xs text-mnt-muted mb-1">Timezone</label>
      <input
        v-model="timezone"
        type="text"
        class="w-full bg-mnt-primary border border-mnt-default rounded-lg px-3 py-2 text-mnt-primary text-sm focus:outline-none focus:border-mnt-default"
        placeholder="Europe/Paris (leave empty for browser timezone)"
      />
      <p class="text-[11px] text-mnt-muted mt-1">IANA timezone identifier, e.g. <code class="text-mnt-muted">America/New_York</code></p>
    </div>

    <div>
      <label class="block text-xs text-mnt-muted mb-2">Date Format</label>
      <div class="flex gap-4">
        <label class="flex items-center gap-2 cursor-pointer">
          <input v-model="dateFormat" type="radio" value="relative" class="accent-mnt-green-500" />
          <span class="text-sm text-mnt-secondary">Relative <span class="text-mnt-muted text-xs">(2 minutes ago)</span></span>
        </label>
        <label class="flex items-center gap-2 cursor-pointer">
          <input v-model="dateFormat" type="radio" value="absolute" class="accent-mnt-green-500" />
          <span class="text-sm text-mnt-secondary">Absolute <span class="text-mnt-muted text-xs">(5 mai 2026 14:32)</span></span>
        </label>
      </div>
    </div>
  </div>
</template>

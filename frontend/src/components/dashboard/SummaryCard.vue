<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { RouterLink, type RouteLocationRaw } from 'vue-router'
import { Box, Globe, Heart, Shield } from 'lucide-vue-next'

defineProps<{
  title: string
  icon: string
  total: number
  counts: { ok: number; warning: number; down: number }
  link: RouteLocationRaw
}>()

const iconComponents: Record<string, typeof Box> = {
  box: Box,
  globe: Globe,
  heart: Heart,
  shield: Shield,
}

const iconColorClasses: Record<string, string> = {
  box: 'text-pb-green-400',
  globe: 'text-indigo-400',
  heart: 'text-pink-400',
  shield: 'text-emerald-400',
}
</script>

<template>
  <RouterLink
    :to="link"
    class="block bg-pb-surface p-5 rounded-2xl border border-slate-800 hover:border-slate-700 transition-all shadow-lg"
  >
    <div class="flex items-center justify-between mb-3">
      <div class="flex items-center gap-3">
        <!-- Icon in square container -->
        <div class="p-2.5 rounded-xl" style="background: var(--pb-bg-elevated)">
          <component
            :is="iconComponents[icon] || Box"
            :size="20"
            :class="iconColorClasses[icon] || 'text-pb-green-400'"
          />
        </div>
        <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">{{ title }}</span>
      </div>
    </div>
    <div class="text-2xl font-black text-pb-primary mb-3">{{ total }}</div>
    <div class="flex items-center gap-2">
      <span
        v-if="counts.ok > 0"
        class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
        style="background: var(--pb-status-ok-bg); color: var(--pb-status-ok)"
      >
        <span class="inline-block h-1.5 w-1.5 rounded-full bg-emerald-500" />
        {{ counts.ok }} ok
      </span>
      <span
        v-if="counts.warning > 0"
        class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
        style="background: var(--pb-status-warn-bg); color: var(--pb-status-warn)"
      >
        <span class="inline-block h-1.5 w-1.5 rounded-full bg-amber-500" />
        {{ counts.warning }} warn
      </span>
      <span
        v-if="counts.down > 0"
        class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
        style="background: var(--pb-status-down-bg); color: var(--pb-status-down)"
      >
        <span class="inline-block h-1.5 w-1.5 rounded-full bg-rose-500" />
        {{ counts.down }} down
      </span>
    </div>
  </RouterLink>
</template>

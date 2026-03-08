<script setup lang="ts">
import type { ContainerRisk } from '@/services/postureApi'
import PostureScoreBadge from './PostureScoreBadge.vue'
import { ChevronRight } from 'lucide-vue-next'

defineProps<{
  risks: ContainerRisk[]
}>()

const emit = defineEmits<{
  select: [containerId: number]
}>()
</script>

<template>
  <div class="bg-[#12151C] rounded-2xl border border-slate-800 overflow-hidden">
    <div class="hidden md:block overflow-x-auto">
      <table class="w-full text-left border-collapse">
        <thead>
          <tr class="bg-[#0B0E13]/60 text-slate-500 text-[10px] uppercase tracking-widest font-bold border-b border-slate-800/60">
            <th class="px-6 py-3.5">Score</th>
            <th class="px-6 py-3.5">Container</th>
            <th class="px-6 py-3.5">Top Issue</th>
            <th class="px-6 py-3.5 text-right" />
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-800/40">
          <tr
            v-for="risk in risks"
            :key="risk.container_id"
            class="group hover:bg-slate-800/25 transition-all cursor-pointer"
            @click="emit('select', risk.container_id)"
          >
            <td class="px-6 py-3">
              <PostureScoreBadge :score="risk.score" :color="risk.color" size="sm" />
            </td>
            <td class="px-6 py-3 text-sm font-semibold text-slate-100 group-hover:text-pb-green-400 transition-colors">
              {{ risk.container_name }}
            </td>
            <td class="px-6 py-3 text-xs text-slate-500">
              {{ risk.top_issue || '—' }}
            </td>
            <td class="px-6 py-3 text-right">
              <ChevronRight :size="14" class="text-slate-700 group-hover:text-slate-400 transition-colors" />
            </td>
          </tr>
          <tr v-if="risks.length === 0">
            <td colspan="4" class="px-6 py-12 text-center text-slate-600 text-sm font-medium">
              No container risk data available
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Mobile card list -->
    <div class="md:hidden divide-y divide-slate-800/40">
      <div
        v-for="risk in risks"
        :key="'m-' + risk.container_id"
        class="px-4 py-3 active:bg-slate-800/25 transition-colors cursor-pointer flex items-center gap-3"
        @click="emit('select', risk.container_id)"
      >
        <PostureScoreBadge :score="risk.score" :color="risk.color" size="sm" />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-semibold text-slate-100 truncate">{{ risk.container_name }}</p>
          <p class="text-[10px] text-slate-600 mt-0.5 truncate">{{ risk.top_issue || '—' }}</p>
        </div>
        <ChevronRight :size="14" class="text-slate-700 shrink-0" />
      </div>
      <div v-if="risks.length === 0" class="px-4 py-12 text-center">
        <p class="text-sm text-slate-600 font-medium">No container risk data available</p>
      </div>
    </div>
  </div>
</template>

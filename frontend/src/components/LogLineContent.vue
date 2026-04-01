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
import { computed, ref } from 'vue'
import type { LogLine } from '@/composables/useLogStream'
import type { AnsiStyle } from '@/composables/useAnsiParser'
import type { SearchMatch } from '@/composables/useLogSearch'
import LogJsonLine from './LogJsonLine.vue'

const props = defineProps<{
  line: LogLine
  lineIndex: number
  hasTimestamps: boolean
  searchMatches: SearchMatch[]
  activeMatchOffset: number | null
  expanded: boolean
}>()

const emit = defineEmits<{
  'toggle-expand': []
}>()

// ── Level border class ───────────────────────────────────────────────

const levelBorderClass = computed(() => {
  switch (props.line.level) {
    case 'fatal':
    case 'error':
      return 'border-l-2 border-red-500'
    case 'warn':
      return 'border-l-2 border-amber-500'
    case 'info':
      return 'border-l-2 border-blue-500'
    case 'debug':
    case 'trace':
      return 'border-l-2 border-slate-600'
    default:
      return 'border-l-2 border-transparent'
  }
})

// ── Timestamp formatting ─────────────────────────────────────────────

const shortTimestamp = computed(() => {
  if (!props.line.parsedTimestamp) return ''
  const d = props.line.parsedTimestamp
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
})

const showTimestampTooltip = ref(false)

const fullTimestamp = computed(() => {
  if (!props.line.parsedTimestamp) return ''
  return props.line.parsedTimestamp.toISOString()
})

const relativeTime = computed(() => {
  if (!props.line.parsedTimestamp) return ''
  const now = Date.now()
  const diff = now - props.line.parsedTimestamp.getTime()
  if (diff < 1000) return 'just now'
  if (diff < 60000) return `${Math.floor(diff / 1000)} seconds ago`
  if (diff < 3600000) return `${Math.floor(diff / 60000)} minutes ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} hours ago`
  return `${Math.floor(diff / 86400000)} days ago`
})

// ── Token style (ANSI) ──────────────────────────────────────────────

function tokenStyle(style: AnsiStyle): Record<string, string> {
  const s: Record<string, string> = {}
  if (style.fg) s.color = style.fg
  if (style.bg) s.backgroundColor = style.bg
  if (style.bold) s.fontWeight = 'bold'
  if (style.dim) s.opacity = '0.6'
  if (style.italic) s.fontStyle = 'italic'
  const decorations: string[] = []
  if (style.underline) decorations.push('underline')
  if (style.strikethrough) decorations.push('line-through')
  if (decorations.length > 0) s.textDecoration = decorations.join(' ')
  return s
}

// ── Search highlight rendering ───────────────────────────────────────

interface RenderSegment {
  text: string
  style: Record<string, string>
  highlight: 'none' | 'match' | 'current'
}

const renderSegments = computed<RenderSegment[]>(() => {
  const { tokens } = props.line
  const lineMatches = props.searchMatches

  if (lineMatches.length === 0) {
    return tokens.map(t => ({
      text: t.text,
      style: tokenStyle(t.style),
      highlight: 'none' as const,
    }))
  }

  // Build character-level highlight map from matches (on plain text positions)
  const textLen = props.line.text.length
  const highlightMap = new Uint8Array(textLen) // 0=none, 1=match, 2=current
  for (const m of lineMatches) {
    const isCurrent = m.startOffset === props.activeMatchOffset
    for (let i = m.startOffset; i < m.endOffset && i < textLen; i++) {
      highlightMap[i] = isCurrent ? 2 : Math.max(highlightMap[i]!, 1)
    }
  }

  // Walk tokens and split at highlight boundaries
  const segments: RenderSegment[] = []
  let charPos = 0

  for (const token of tokens) {
    const style = tokenStyle(token.style)
    let segStart = 0

    for (let i = 0; i <= token.text.length; i++) {
      const globalPos = charPos + i
      const curHighlight = i < token.text.length ? (highlightMap[globalPos] ?? 0) : -1
      const prevHighlight = i > 0 ? (highlightMap[globalPos - 1] ?? 0) : -1

      if (i > 0 && (i === token.text.length || curHighlight !== prevHighlight)) {
        const text = token.text.slice(segStart, i)
        if (text) {
          const h = highlightMap[charPos + segStart] ?? 0
          segments.push({
            text,
            style,
            highlight: h === 2 ? 'current' : h === 1 ? 'match' : 'none',
          })
        }
        segStart = i
      }
    }

    charPos += token.text.length
  }

  return segments
})
</script>

<template>
  <div class="flex" :class="levelBorderClass">
    <!-- Line number -->
    <span
      class="mr-3 inline-block min-w-[3rem] select-none border-r border-slate-800 pr-3 text-right text-slate-600"
    >{{ line.id }}</span>

    <!-- Timestamp column -->
    <span
      v-if="hasTimestamps"
      class="relative mr-2 inline-block min-w-[5rem] select-none text-right text-slate-500"
      style="font-size: 0.65rem;"
      @mouseenter="showTimestampTooltip = true"
      @mouseleave="showTimestampTooltip = false"
    >
      {{ shortTimestamp }}
      <span
        v-if="showTimestampTooltip && line.parsedTimestamp"
        class="absolute bottom-full left-1/2 z-50 mb-1 -translate-x-1/2 whitespace-nowrap rounded border border-slate-700 bg-pb-surface px-2 py-1 text-[10px] text-pb-secondary shadow-lg"
      >{{ fullTimestamp }} &mdash; {{ relativeTime }}</span>
    </span>

    <!-- Content -->
    <span class="flex-1">
      <!-- JSON rendering -->
      <LogJsonLine
        v-if="line.parsedJson && searchMatches.length === 0"
        :json="line.parsedJson"
        :prefix="line.jsonPrefix"
        :expanded="expanded"
        @toggle-expand="emit('toggle-expand')"
      />
      <!-- ANSI token rendering with search highlights -->
      <template v-else>
        <span
          v-for="(seg, si) in renderSegments"
          :key="si"
          :style="seg.style"
        ><mark
          v-if="seg.highlight !== 'none'"
          class="rounded-sm"
          :class="seg.highlight === 'current'
            ? 'bg-yellow-500/50 text-yellow-100'
            : 'bg-yellow-500/30 text-yellow-200'"
        >{{ seg.text }}</mark><template v-else>{{ seg.text }}</template></span>
      </template>
    </span>
  </div>
</template>

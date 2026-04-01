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
import { computed } from 'vue'

const props = defineProps<{
  json: Record<string, unknown>
  prefix: string | null
  expanded: boolean
}>()

defineEmits<{
  'toggle-expand': []
}>()

interface JsonSpan {
  text: string
  cls: string
}

function tokenizeValue(value: unknown, indent: string, expanded: boolean): JsonSpan[] {
  if (value === null) return [{ text: 'null', cls: 'text-slate-500' }]
  if (typeof value === 'boolean') return [{ text: String(value), cls: 'text-purple-400' }]
  if (typeof value === 'number') return [{ text: String(value), cls: 'text-yellow-400' }]
  if (typeof value === 'string') return [{ text: JSON.stringify(value), cls: 'text-green-400' }]

  if (Array.isArray(value)) {
    if (!expanded || value.length === 0) {
      return [{ text: JSON.stringify(value), cls: 'text-pb-secondary' }]
    }
    const spans: JsonSpan[] = [{ text: '[\n', cls: 'text-pb-secondary' }]
    const childIndent = indent + '  '
    for (let i = 0; i < value.length; i++) {
      spans.push({ text: childIndent, cls: '' })
      spans.push(...tokenizeValue(value[i], childIndent, true))
      if (i < value.length - 1) spans.push({ text: ',', cls: 'text-pb-secondary' })
      spans.push({ text: '\n', cls: '' })
    }
    spans.push({ text: indent + ']', cls: 'text-pb-secondary' })
    return spans
  }

  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>
    const keys = Object.keys(obj)
    if (!expanded || keys.length === 0) {
      return [{ text: JSON.stringify(value), cls: 'text-pb-secondary' }]
    }
    const spans: JsonSpan[] = [{ text: '{\n', cls: 'text-pb-secondary' }]
    const childIndent = indent + '  '
    for (let i = 0; i < keys.length; i++) {
      const k = keys[i]!
      spans.push({ text: childIndent, cls: '' })
      spans.push({ text: JSON.stringify(k), cls: 'text-cyan-400' })
      spans.push({ text: ': ', cls: 'text-pb-secondary' })
      spans.push(...tokenizeValue(obj[k], childIndent, true))
      if (i < keys.length - 1) spans.push({ text: ',', cls: 'text-pb-secondary' })
      spans.push({ text: '\n', cls: '' })
    }
    spans.push({ text: indent + '}', cls: 'text-pb-secondary' })
    return spans
  }

  return [{ text: String(value), cls: 'text-pb-secondary' }]
}

function tokenizeCompact(obj: Record<string, unknown>): JsonSpan[] {
  const keys = Object.keys(obj)
  const spans: JsonSpan[] = [{ text: '{', cls: 'text-pb-secondary' }]
  for (let i = 0; i < keys.length; i++) {
    const k = keys[i]!
    if (i > 0) spans.push({ text: ', ', cls: 'text-pb-secondary' })
    spans.push({ text: JSON.stringify(k), cls: 'text-cyan-400' })
    spans.push({ text: ': ', cls: 'text-pb-secondary' })
    spans.push(...tokenizeValue(obj[k], '', false))
  }
  spans.push({ text: '}', cls: 'text-pb-secondary' })
  return spans
}

const spans = computed<JsonSpan[]>(() => {
  if (props.expanded) {
    return tokenizeValue(props.json, '', true)
  }
  return tokenizeCompact(props.json)
})
</script>

<template>
  <span
    class="cursor-pointer"
    :class="expanded ? 'block max-h-[400px] overflow-y-auto whitespace-pre' : 'inline'"
    @click.stop="$emit('toggle-expand')"
  >
    <span v-if="prefix" class="text-slate-400">{{ prefix }}</span>
    <span v-for="(span, i) in spans" :key="i" :class="span.cls">{{ span.text }}</span>
  </span>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

const props = defineProps<{
  containerId: number
  containerName?: string
}>()

const lines = ref<string[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const streaming = ref(false)
const logContainer = ref<HTMLPreElement | null>(null)
const autoScroll = ref(true)

let eventSource: EventSource | null = null

function scrollToBottom() {
  if (!autoScroll.value || !logContainer.value) return
  nextTick(() => {
    if (logContainer.value) {
      logContainer.value.scrollTop = logContainer.value.scrollHeight
    }
  })
}

function startStreaming() {
  if (eventSource) {
    eventSource.close()
  }

  loading.value = true
  error.value = null
  streaming.value = true

  let streamUrl = `${API_BASE}/containers/${props.containerId}/logs/stream?lines=100`
  if (props.containerName) {
    streamUrl += `&container=${encodeURIComponent(props.containerName)}`
  }
  const sseUrl = new URL(streamUrl, window.location.origin)
  eventSource = new EventSource(sseUrl.toString())

  eventSource.addEventListener('container.log_line', (event: MessageEvent) => {
    loading.value = false
    try {
      const data = JSON.parse(event.data)
      if (data.line) {
        lines.value.push(data.line)
        // Cap at 1000 lines to prevent memory issues
        if (lines.value.length > 1000) {
          lines.value = lines.value.slice(-800)
        }
        scrollToBottom()
      }
    } catch {
      // If not JSON, treat as raw line
      lines.value.push(event.data)
      scrollToBottom()
    }
  })

  eventSource.addEventListener('container.log_backlog', (event: MessageEvent) => {
    loading.value = false
    try {
      const data = JSON.parse(event.data)
      if (data.lines && Array.isArray(data.lines)) {
        lines.value = data.lines
        scrollToBottom()
      }
    } catch {
      // ignore
    }
  })

  eventSource.addEventListener('container.log_error', (event: MessageEvent) => {
    loading.value = false
    try {
      const data = JSON.parse(event.data)
      error.value = data.error || 'Log stream ended'
    } catch {
      error.value = 'Log stream ended'
    }
    streaming.value = false
  })

  eventSource.onopen = () => {
    loading.value = false
  }

  eventSource.onerror = () => {
    loading.value = false
    // Close to prevent EventSource auto-reconnect loop
    // (especially for stopped containers where the stream closes immediately)
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    streaming.value = false
    // Fallback to static fetch if we got nothing from SSE
    if (lines.value.length === 0) {
      fetchLogsStatic()
    }
  }
}

async function fetchLogsStatic() {
  loading.value = true
  error.value = null
  try {
    const res = await fetch(
      `${API_BASE}/containers/${props.containerId}/logs?lines=100&timestamps=true`,
    )
    if (!res.ok) {
      const body = await res.json().catch(() => ({}))
      throw new Error(body?.error?.message || `HTTP ${res.status}`)
    }
    const data = await res.json()
    lines.value = data.lines || []
    scrollToBottom()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to fetch logs'
  } finally {
    loading.value = false
  }
}

function handleScroll() {
  if (!logContainer.value) return
  const el = logContainer.value
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30
  autoScroll.value = atBottom
}

onMounted(() => {
  startStreaming()
})

onUnmounted(() => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
})

watch([() => props.containerId, () => props.containerName], () => {
  lines.value = []
  if (eventSource) {
    eventSource.close()
  }
  startStreaming()
})
</script>

<template>
  <div class="flex flex-col">
    <div class="mb-2 flex items-center justify-between">
      <h3 class="text-sm font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">Logs</h3>
      <div class="flex items-center gap-2">
        <span
          v-if="streaming"
          class="flex items-center gap-1 text-xs"
          :style="{ color: 'var(--pb-status-ok)' }"
        >
          <span
            :style="{
              display: 'inline-block',
              width: '6px',
              height: '6px',
              borderRadius: '50%',
              backgroundColor: 'var(--pb-status-ok)',
            }"
          />
          Streaming
        </span>
        <button
          v-if="!streaming"
          class="rounded px-2 py-1 text-xs"
          :style="{
            backgroundColor: 'var(--pb-bg-elevated)',
            color: 'var(--pb-text-secondary)',
            borderRadius: 'var(--pb-radius-sm)',
          }"
          @click="startStreaming"
        >
          Reconnect
        </button>
      </div>
    </div>

    <div v-if="loading && lines.length === 0" class="flex justify-center py-4">
      <div
        class="h-5 w-5 animate-spin rounded-full border-2"
        :style="{ borderColor: 'var(--pb-border-default)', borderTopColor: 'var(--pb-accent)' }"
      />
    </div>

    <div
      v-else-if="error && lines.length === 0"
      class="rounded p-3 text-sm"
      :style="{
        backgroundColor: 'var(--pb-status-down-bg)',
        color: 'var(--pb-status-down)',
        borderRadius: 'var(--pb-radius-sm)',
      }"
    >
      {{ error }}
    </div>

    <div
      v-else-if="lines.length === 0"
      class="py-4 text-center text-sm"
      :style="{ color: 'var(--pb-text-muted)' }"
    >
      No logs available
    </div>

    <pre
      v-else
      ref="logContainer"
      :style="{
        flex: '1',
        minHeight: '12rem',
        overflow: 'auto',
        backgroundColor: 'var(--pb-bg-primary)',
        padding: '0.75rem',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
        fontSize: '0.7rem',
        lineHeight: '1.6',
        color: 'var(--pb-text-primary)',
        borderRadius: 'var(--pb-radius-md)',
        border: '1px solid var(--pb-border-subtle)',
      }"
      @scroll="handleScroll"
    ><template v-for="(line, i) in lines" :key="i">{{ line }}
</template></pre>
  </div>
</template>

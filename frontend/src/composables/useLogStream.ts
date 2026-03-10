/*
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
*/

import { ref, watch, nextTick, type Ref } from 'vue'
import { parseAnsi, type AnsiToken } from './useAnsiParser'
import { detectLogLevel, parseJsonLine, parseTimestamp } from './useLogParser'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

export type LogStreamStatus = 'connecting' | 'streaming' | 'error' | 'closed'

export type LogLevel = 'fatal' | 'error' | 'warn' | 'info' | 'debug' | 'trace' | 'unknown'

export interface LogLine {
  id: number
  raw: string
  tokens: AnsiToken[]
  stream: string
  timestamp: string | null
  level: LogLevel
  parsedJson: Record<string, unknown> | null
  jsonPrefix: string | null
  parsedTimestamp: Date | null
}

export interface UseLogStreamOptions {
  containerId: Ref<number>
  containerName?: Ref<string | undefined>
}

export interface UseLogStreamReturn {
  lines: Ref<LogLine[]>
  status: Ref<LogStreamStatus>
  autoScroll: Ref<boolean>
  unseenCount: Ref<number>
  wordWrap: Ref<boolean>
  error: Ref<string | null>
  scrollToBottom: () => void
  handleScroll: (event: Event) => void
  connect: () => void
  disconnect: () => void
}

const MAX_LINES = 1000
const TRIM_TO = 800

// Strip non-printable characters except common whitespace
function stripBinary(s: string): string {
  return s.replace(/[\x00-\x08\x0e-\x1a\x7f]/g, '\ufffd')
}

export function useLogStream(options: UseLogStreamOptions): UseLogStreamReturn {
  const lines = ref<LogLine[]>([])
  const status = ref<LogStreamStatus>('closed')
  const autoScroll = ref(true)
  const unseenCount = ref(0)
  const wordWrap = ref(true)
  const error = ref<string | null>(null)

  let nextLineId = 1
  let eventSource: EventSource | null = null
  let scrollContainer: HTMLElement | null = null

  function buildLine(raw: string, stream = 'stdout', timestamp: string | null = null): LogLine {
    const cleaned = stripBinary(raw)
    const jsonResult = parseJsonLine(cleaned)
    return {
      id: nextLineId++,
      raw: cleaned,
      tokens: parseAnsi(cleaned),
      stream,
      timestamp,
      level: detectLogLevel(cleaned),
      parsedJson: jsonResult?.json ?? null,
      jsonPrefix: jsonResult?.prefix ?? null,
      parsedTimestamp: parseTimestamp(cleaned),
    }
  }

  function addLine(raw: string, stream = 'stdout', timestamp: string | null = null) {
    const line = buildLine(raw, stream, timestamp)
    lines.value.push(line)

    if (lines.value.length > MAX_LINES) {
      lines.value = lines.value.slice(-TRIM_TO)
    }

    if (autoScroll.value) {
      doScrollToBottom()
    } else {
      unseenCount.value++
    }
  }

  function doScrollToBottom() {
    nextTick(() => {
      if (scrollContainer) {
        scrollContainer.scrollTop = scrollContainer.scrollHeight
      }
    })
  }

  function scrollToBottom() {
    autoScroll.value = true
    unseenCount.value = 0
    doScrollToBottom()
  }

  function handleScroll(event: Event) {
    const el = event.target as HTMLElement
    scrollContainer = el
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30
    if (atBottom && !autoScroll.value) {
      autoScroll.value = true
      unseenCount.value = 0
    } else if (!atBottom) {
      autoScroll.value = false
    }
  }

  function connect() {
    disconnect()
    status.value = 'connecting'
    error.value = null

    let streamUrl = `${API_BASE}/containers/${options.containerId.value}/logs/stream?lines=100`
    if (options.containerName?.value) {
      streamUrl += `&container=${encodeURIComponent(options.containerName.value)}`
    }
    const sseUrl = new URL(streamUrl, window.location.origin)
    eventSource = new EventSource(sseUrl.toString())

    eventSource.addEventListener('container.log_line', (event: MessageEvent) => {
      status.value = 'streaming'
      try {
        const data = JSON.parse(event.data)
        if (data.line) {
          addLine(data.line, data.stream, data.timestamp)
        }
      } catch {
        addLine(event.data)
      }
    })

    eventSource.addEventListener('container.log_backlog', (event: MessageEvent) => {
      status.value = 'streaming'
      try {
        const data = JSON.parse(event.data)
        if (data.lines && Array.isArray(data.lines)) {
          lines.value = []
          for (const raw of data.lines) {
            lines.value.push(buildLine(raw))
          }
          if (autoScroll.value) {
            doScrollToBottom()
          }
        }
      } catch {
        // ignore malformed backlog
      }
    })

    eventSource.addEventListener('container.log_error', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data)
        error.value = data.error || 'Log stream ended'
      } catch {
        error.value = 'Log stream ended'
      }
      status.value = 'error'
    })

    eventSource.onopen = () => {
      status.value = 'streaming'
    }

    eventSource.onerror = () => {
      if (eventSource) {
        eventSource.close()
        eventSource = null
      }
      if (lines.value.length === 0) {
        fetchLogsStatic()
      } else {
        status.value = 'closed'
      }
    }
  }

  async function fetchLogsStatic() {
    status.value = 'connecting'
    try {
      const res = await fetch(
        `${API_BASE}/containers/${options.containerId.value}/logs?lines=100&timestamps=true`,
      )
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body?.error?.message || `HTTP ${res.status}`)
      }
      const data = await res.json()
      const rawLines: string[] = data.lines || []
      lines.value = []
      for (const raw of rawLines) {
        lines.value.push(buildLine(raw))
      }
      status.value = 'closed'
      if (autoScroll.value) {
        doScrollToBottom()
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch logs'
      status.value = 'error'
    }
  }

  function disconnect() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    status.value = 'closed'
  }

  // Reconnect when containerId or containerName changes
  watch([options.containerId, ...(options.containerName ? [options.containerName] : [])], () => {
    lines.value = []
    nextLineId = 1
    unseenCount.value = 0
    autoScroll.value = true
    connect()
  })

  return {
    lines,
    status,
    autoScroll,
    unseenCount,
    wordWrap,
    error,
    scrollToBottom,
    handleScroll,
    connect,
    disconnect,
  }
}

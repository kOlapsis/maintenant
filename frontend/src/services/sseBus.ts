import { ref } from 'vue'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

type EventHandler = (event: MessageEvent) => void

const listeners = new Map<string, Set<EventHandler>>()
// Track which event names have been registered on the current EventSource
const registeredEvents = new Set<string>()
let eventSource: EventSource | null = null
let refCount = 0
let retryCount = 0
let retryTimer: ReturnType<typeof setTimeout> | null = null

export const connected = ref(false)

function dispatch(eventName: string, e: MessageEvent) {
  const handlers = listeners.get(eventName)
  if (handlers) {
    for (const handler of handlers) {
      handler(e)
    }
  }
}

function registerEvent(es: EventSource, eventName: string) {
  if (registeredEvents.has(eventName)) return
  registeredEvents.add(eventName)
  es.addEventListener(eventName, ((e: MessageEvent) => {
    dispatch(eventName, e)
  }) as EventListener)
}

function openConnection() {
  if (retryTimer) {
    clearTimeout(retryTimer)
    retryTimer = null
  }

  const url = new URL(`${API_BASE}/containers/events`, window.location.origin)
  eventSource = new EventSource(url.toString())
  registeredEvents.clear()

  eventSource.onopen = () => {
    connected.value = true
    retryCount = 0
  }

  eventSource.onerror = () => {
    connected.value = false
    if (eventSource && eventSource.readyState === EventSource.CLOSED) {
      eventSource = null
      const delay = Math.min(30000, 1000 * Math.pow(2, retryCount))
      retryCount++
      retryTimer = setTimeout(() => {
        if (refCount > 0) openConnection()
      }, delay)
    }
  }

  // Register all currently known event names on the new EventSource
  for (const eventName of listeners.keys()) {
    registerEvent(eventSource, eventName)
  }
}

function closeConnection() {
  if (retryTimer) {
    clearTimeout(retryTimer)
    retryTimer = null
  }
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  registeredEvents.clear()
  connected.value = false
  retryCount = 0
}

export function connect() {
  refCount++
  if (refCount === 1) openConnection()
}

export function disconnect() {
  refCount = Math.max(0, refCount - 1)
  if (refCount === 0) closeConnection()
}

export function on(eventName: string, handler: EventHandler) {
  let handlers = listeners.get(eventName)
  if (!handlers) {
    handlers = new Set()
    listeners.set(eventName, handlers)
  }
  handlers.add(handler)

  // If already connected, ensure this event name is registered on the native EventSource
  if (eventSource) {
    registerEvent(eventSource, eventName)
  }
}

export function off(eventName: string, handler: EventHandler) {
  const handlers = listeners.get(eventName)
  if (handlers) {
    handlers.delete(handler)
    if (handlers.size === 0) {
      listeners.delete(eventName)
    }
  }
}

export const sseBus = { on, off, connect, disconnect, connected }

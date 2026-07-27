// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { ref } from 'vue'

import { probeAuth } from './authGuard'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'

type EventHandler = (event: MessageEvent) => void

const listeners = new Map<string, Set<EventHandler>>()
const registeredEvents = new Set<string>()
let eventSource: EventSource | null = null
let refCount = 0
let retryCount = 0
let retryTimer: ReturnType<typeof setTimeout> | null = null
/** true once the first successful connection has been established */
let hasConnectedOnce = false

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

function scheduleRetry() {
  if (retryTimer) return
  const delay = Math.min(30000, 1000 * Math.pow(2, retryCount))
  retryCount++
  retryTimer = setTimeout(() => {
    retryTimer = null
    if (refCount > 0) openConnection()
  }, delay)
}

function openConnection() {
  if (retryTimer) {
    clearTimeout(retryTimer)
    retryTimer = null
  }
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  registeredEvents.clear()

  const url = new URL(`${API_BASE}/containers/events`, window.location.origin)
  const es = new EventSource(url.toString())
  eventSource = es

  es.onopen = () => {
    if (es !== eventSource) return // stale instance
    const isReconnect = hasConnectedOnce
    connected.value = true
    hasConnectedOnce = true
    retryCount = 0
    if (isReconnect) {
      dispatch('sse.reconnected', new MessageEvent('sse.reconnected'))
    }
  }

  es.onerror = () => {
    if (es !== eventSource) return // stale instance
    connected.value = false
    // Close the native EventSource to prevent its built-in retry
    // (we manage reconnection ourselves with backoff).
    es.close()
    eventSource = null
    // EventSource never tells us why it failed: ask the API whether the proxy
    // session died, otherwise we would retry a dead stream forever.
    void probeAuth()
    scheduleRetry()
  }

  // Register all currently known event names on the new EventSource
  for (const eventName of listeners.keys()) {
    registerEvent(es, eventName)
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
  hasConnectedOnce = false
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

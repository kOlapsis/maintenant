// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { ref } from 'vue'

export interface Toast {
  id: number
  title?: string
  message: string
  type: 'info' | 'success' | 'warning'
  duration: number
  dedupeKey?: string
}

export interface ShowToastOptions {
  title?: string
  dedupeKey?: string
}

const toasts = ref<Toast[]>([])
let nextId = 0

export function showToast(
  message: string,
  type: Toast['type'] = 'info',
  duration = 5000,
  options?: ShowToastOptions,
) {
  if (options?.dedupeKey && toasts.value.some((t) => t.dedupeKey === options.dedupeKey)) return
  const id = nextId++
  toasts.value.push({ id, title: options?.title, message, type, duration, dedupeKey: options?.dedupeKey })
  setTimeout(() => {
    toasts.value = toasts.value.filter(t => t.id !== id)
  }, duration)
}

export function useToast() {
  return { toasts, showToast }
}

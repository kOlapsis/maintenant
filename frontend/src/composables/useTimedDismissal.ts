import { ref, onMounted } from 'vue'

export interface TimedDismissalOptions {
  storageKey: string
  cooldownMs: number
}

export function useTimedDismissal(opts: TimedDismissalOptions) {
  const visible = ref(false)

  onMounted(() => {
    const raw = localStorage.getItem(opts.storageKey)
    if (!raw) {
      visible.value = true
      return
    }
    const dismissedAt = Number(raw)
    if (!Number.isFinite(dismissedAt)) {
      visible.value = true
      return
    }
    visible.value = Date.now() - dismissedAt >= opts.cooldownMs
  })

  function dismiss() {
    visible.value = false
    localStorage.setItem(opts.storageKey, String(Date.now()))
  }

  return { visible, dismiss }
}

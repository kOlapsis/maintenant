import { ref, watch, onUnmounted, type Ref } from 'vue'

const FOCUSABLE_SELECTORS = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

export function useFocusTrap(containerRef: Ref<HTMLElement | null>, active: Ref<boolean>) {
  const previouslyFocused = ref<HTMLElement | null>(null)

  function getFocusableElements(): HTMLElement[] {
    if (!containerRef.value) return []
    return Array.from(containerRef.value.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS))
  }

  function handleKeyDown(event: KeyboardEvent) {
    if (event.key !== 'Tab') return

    const focusable = getFocusableElements()
    if (focusable.length === 0) {
      event.preventDefault()
      return
    }

    const first = focusable[0]!
    const last = focusable[focusable.length - 1]!

    if (event.shiftKey) {
      if (document.activeElement === first) {
        event.preventDefault()
        last.focus()
      }
    } else {
      if (document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
  }

  function activate() {
    previouslyFocused.value = document.activeElement as HTMLElement | null
    document.addEventListener('keydown', handleKeyDown)

    // Focus first focusable element in container
    const focusable = getFocusableElements()
    if (focusable.length > 0) {
      requestAnimationFrame(() => focusable[0]!.focus())
    }
  }

  function deactivate() {
    document.removeEventListener('keydown', handleKeyDown)
    if (previouslyFocused.value && typeof previouslyFocused.value.focus === 'function') {
      previouslyFocused.value.focus()
    }
    previouslyFocused.value = null
  }

  watch(active, (isActive) => {
    if (isActive) {
      activate()
    } else {
      deactivate()
    }
  })

  onUnmounted(() => {
    deactivate()
  })

  return { activate, deactivate }
}

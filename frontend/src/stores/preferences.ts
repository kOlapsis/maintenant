import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export type Density = 'compact' | 'comfortable'

export const usePreferencesStore = defineStore('preferences', () => {
  function getInitialDensity(): Density {
    const stored = localStorage.getItem('pb-density')
    if (stored === 'compact' || stored === 'comfortable') return stored
    return 'comfortable'
  }

  const density = ref<Density>(getInitialDensity())

  function applyDensity(d: Density) {
    if (d === 'comfortable') {
      document.documentElement.removeAttribute('data-density')
    } else {
      document.documentElement.setAttribute('data-density', d)
    }
    localStorage.setItem('pb-density', d)
  }

  function toggleDensity() {
    density.value = density.value === 'comfortable' ? 'compact' : 'comfortable'
  }

  watch(density, applyDensity, { immediate: true })

  return {
    density,
    toggleDensity,
  }
})

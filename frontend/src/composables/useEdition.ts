import { ref, computed } from 'vue'
import { fetchEdition, type EditionResponse } from '@/services/editionApi'

const edition = ref<EditionResponse | null>(null)
const loaded = ref(false)

async function load() {
  if (loaded.value) return
  try {
    edition.value = await fetchEdition()
  } catch {
    edition.value = { edition: 'community', features: {} }
  }
  loaded.value = true
}

// Start loading immediately on first import
load()

export function useEdition() {
  const isPro = computed(() => edition.value?.edition === 'pro')
  const isCE = computed(() => !isPro.value)

  function hasFeature(name: string): boolean {
    return edition.value?.features[name] === true
  }

  return { edition, isPro, isCE, hasFeature, load }
}

import { ref, computed } from 'vue'
import { fetchEdition, type EditionResponse } from '@/services/editionApi'

const edition = ref<EditionResponse | null>(null)
const loaded = ref(false)

async function load() {
  if (loaded.value) return
  try {
    edition.value = await fetchEdition()
  } catch {
    edition.value = { edition: 'community', organisation_name: '', features: {} }
  }
  loaded.value = true
}

// Start loading immediately on first import
load()

export function useEdition() {
  const isEnterprise = computed(() => edition.value?.edition === 'enterprise')
  const isCommunity = computed(() => !isEnterprise.value)
  const organisationName = computed(() => edition.value?.organisation_name || '')

  function hasFeature(name: string): boolean {
    return edition.value?.features[name] === true
  }

  return { edition, isEnterprise, isCommunity, organisationName, hasFeature, load }
}

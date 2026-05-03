import { computed, type Ref } from 'vue'
import enDict from '@/locales/status-page/en'
import frDict from '@/locales/status-page/fr'
import type { StatusPageDictKey } from '@/locales/status-page/en'

type Dict = Record<StatusPageDictKey, string>

const dicts: Record<string, Dict> = {
  en: enDict as Dict,
  fr: frDict as Dict,
}

export function useStatusPageI18n(locale: Ref<string>) {
  const dict = computed<Dict>(() => dicts[locale.value] ?? (dicts['en'] as Dict))

  function t(key: StatusPageDictKey): string {
    return dict.value[key] ?? (enDict as Dict)[key]
  }

  return { t }
}

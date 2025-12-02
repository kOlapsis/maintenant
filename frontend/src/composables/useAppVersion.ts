import { ref } from 'vue'

const version = ref('...')

let fetched = false

export function useAppVersion() {
  if (!fetched) {
    fetched = true
    fetch('/api/v1/health')
      .then((r) => r.json())
      .then((data) => {
        if (data.version) version.value = data.version
      })
      .catch(() => {})
  }
  return { version }
}

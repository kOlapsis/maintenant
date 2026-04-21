export const DOCS_BASE_URL = 'https://docs.maintenant.dev'

export function docUrl(path: string): string {
  return `${DOCS_BASE_URL}/${path.replace(/^\//, '')}`
}

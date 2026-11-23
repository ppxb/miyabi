import { parseSearchPage } from '@/lib/search-schema'

export type ActorsSearch = { q?: string; page?: number }

export function validateActorsSearch(search: Record<string, unknown>): ActorsSearch {
  const q = typeof search.q === 'string' && search.q.trim() !== '' ? search.q : undefined
  return { q, page: parseSearchPage(search.page) }
}

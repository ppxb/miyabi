import { parseSearchPage } from '@/lib/search-schema'

export type HistorySearch = { page?: number }

export function validateHistorySearch(search: Record<string, unknown>): HistorySearch {
  const page = parseSearchPage(search.page)
  return page ? { page } : {}
}

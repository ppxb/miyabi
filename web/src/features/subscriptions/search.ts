import { parseSearchPage } from '@/lib/search-schema'

export type SubscriptionsSearch = {
  actorID?: number
  page?: number
}

export function validateSubscriptionsSearch(search: Record<string, unknown>): SubscriptionsSearch {
  if (
    search.actorID !== undefined &&
    typeof search.actorID !== 'number' &&
    typeof search.actorID !== 'string'
  ) {
    throw new Error('演员编号无效')
  }
  const actorID = search.actorID === undefined ? undefined : Number(search.actorID)
  if (actorID !== undefined && (!Number.isSafeInteger(actorID) || actorID < 1)) {
    throw new Error('演员编号无效')
  }
  return { actorID, page: parseSearchPage(search.page) }
}

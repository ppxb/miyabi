import { parseSearchPage } from '@/lib/search-schema'

export type SubscriptionsView = 'movies' | 'actors'
export type SubscriptionsSearch = {
  view?: SubscriptionsView
  actorID?: number
  page?: number
  actorPage?: number
}

export function validateSubscriptionsSearch(search: Record<string, unknown>): SubscriptionsSearch {
  if (search.view !== undefined && search.view !== 'movies' && search.view !== 'actors') {
    throw new Error('订阅类型无效')
  }
  if (
    search.actorID !== undefined &&
    typeof search.actorID !== 'number' &&
    typeof search.actorID !== 'string'
  ) {
    throw new Error('演员订阅编号无效')
  }
  const actorID = search.actorID === undefined ? undefined : Number(search.actorID)
  if (actorID !== undefined && (!Number.isSafeInteger(actorID) || actorID < 1)) {
    throw new Error('演员订阅编号无效')
  }
  return {
    view: search.view === 'actors' ? 'actors' : undefined,
    actorID: search.view === 'actors' ? actorID : undefined,
    page: parseSearchPage(search.page),
    actorPage: parseSearchPage(search.actorPage)
  }
}

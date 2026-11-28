import type { ActorFollowFilter } from '@/api/actors'
import { parseSearchPage } from '@/lib/search-schema'

export type ActorsSearch = { q?: string; follow?: ActorFollowFilter; page?: number }

export function validateActorsSearch(search: Record<string, unknown>): ActorsSearch {
  const q = typeof search.q === 'string' && search.q.trim() !== '' ? search.q : undefined
  const follow = search.follow
  if (follow !== undefined && follow !== 'followed' && follow !== 'unfollowed') {
    throw new Error('关注筛选无效')
  }
  return {
    q,
    follow: follow === 'followed' || follow === 'unfollowed' ? follow : undefined,
    page: parseSearchPage(search.page)
  }
}

import type { QueryClient } from '@tanstack/react-query'

import type { DiscoverMovie, DiscoverMovieDetail } from './discover'

export const discoverKeys = {
  all: ['discover'] as const,
  movies: (params?: unknown) => ['discover', 'movies', params] as const,
  movie: (id: string) => ['discover', 'movie', id] as const,
  magnets: (id: string, code?: string) => ['discover', 'movie', id, 'magnets', code ?? ''] as const,
  tags: (zone: string) => ['discover', 'tags', zone] as const
}

const detailStaleTime = 5 * 60_000

function containsMovieCards(key: readonly unknown[], id: string) {
  return (
    key[0] === 'discover' &&
    (key[1] === 'movies' ||
      key[1] === 'search' ||
      (key[1] === 'movie' && key.length === 3 && key[2] === id))
  )
}

// QueryCache is the only source of card data. Keep list items out of the
// full-detail query, which also contains the upstream recommendation lists.
export function findCachedMovieCard(client: QueryClient, id: string, freshOnly = false) {
  let newest: { card: DiscoverMovie; updatedAt: number; invalidated: boolean } | undefined
  for (const query of client.getQueryCache().findAll({ queryKey: discoverKeys.all })) {
    if (!containsMovieCards(query.queryKey, id)) continue
    const { data, dataUpdatedAt, isInvalidated } = query.state
    const card =
      query.queryKey[1] === 'movie'
        ? (data as DiscoverMovieDetail | undefined)
        : (data as DiscoverMovie[] | undefined)?.find(item => item.id === id)
    if (card && (!newest || dataUpdatedAt >= newest.updatedAt)) {
      newest = { card, updatedAt: dataUpdatedAt, invalidated: isInvalidated }
    }
  }
  if (
    !newest ||
    (freshOnly && (newest.invalidated || Date.now() - newest.updatedAt > detailStaleTime))
  )
    return undefined
  return newest.card
}

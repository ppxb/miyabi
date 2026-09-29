import { queryOptions, type QueryClient } from '@tanstack/react-query'

import type { DiscoverMovie, DiscoverMovieDetail } from './discover'

export const discoverKeys = {
  all: ['discover'] as const,
  movies: (params?: unknown) => ['discover', 'movies', params] as const,
  movie: (id: string) => ['discover', 'movie', id] as const,
  magnets: (id: string) => ['discover', 'movie', id, 'magnets'] as const,
  search: (params?: unknown) => ['discover', 'search', params] as const,
  tags: (zone: string) => ['discover', 'tags', zone] as const,
  route: ['javdb', 'route'] as const
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

// List/search updates can supply a recommendation without its own detail fetch.
// Subscribe only while the card is mounted, and ignore unrelated cache removals.
export function subscribeMovieCard(client: QueryClient, id: string, notify: () => void) {
  return client.getQueryCache().subscribe(event => {
    if (
      containsMovieCards(event.query.queryKey, id) &&
      (event.type === 'removed' || (event.type === 'updated' && event.action.type === 'success'))
    )
      notify()
  })
}

type DetailRequest = { id: string; consumers: number; started: boolean }
type DetailQueue = { requests: Map<string, DetailRequest>; running: number }

export function createMovieDetailLoader(
  fetchDetail: (id: string, signal?: AbortSignal) => Promise<DiscoverMovieDetail>
) {
  const queues = new WeakMap<QueryClient, DetailQueue>()

  function options(id: string) {
    return queryOptions({
      queryKey: discoverKeys.movie(id),
      queryFn: ({ signal }) => fetchDetail(id, signal),
      staleTime: detailStaleTime
    })
  }

  function queueFor(client: QueryClient) {
    let queue = queues.get(client)
    if (!queue) {
      queue = { requests: new Map(), running: 0 }
      queues.set(client, queue)
    }
    return queue
  }

  function prefetchOptions(id: string) {
    // A started prefetch survives the brief observer gap during navigation.
    // Normal foreground queries still consume their signal and can be canceled.
    return { ...options(id), queryFn: () => fetchDetail(id) }
  }

  function start(
    client: QueryClient,
    queue: DetailQueue,
    request: DetailRequest,
    background: boolean
  ) {
    request.started = true
    if (background) queue.running++
    void client.prefetchQuery(prefetchOptions(request.id)).finally(() => {
      if (queue.requests.get(request.id) === request) queue.requests.delete(request.id)
      if (background) queue.running--
      pump(client, queue)
    })
  }

  function pump(client: QueryClient, queue: DetailQueue) {
    for (const request of queue.requests.values()) {
      if (request.started) continue
      if (findCachedMovieCard(client, request.id, true)) {
        queue.requests.delete(request.id)
        continue
      }
      if (queue.running >= 2) break
      start(client, queue, request, true)
    }
  }

  function request(client: QueryClient, id: string) {
    if (
      findCachedMovieCard(client, id, true) ||
      client.getQueryState(discoverKeys.movie(id))?.status === 'error'
    )
      return () => {}
    const queue = queueFor(client)
    let pending = queue.requests.get(id)
    if (!pending) {
      pending = { id, consumers: 0, started: false }
      queue.requests.set(id, pending)
    }
    const current = pending
    current.consumers++
    queueMicrotask(() => pump(client, queue))
    let released = false
    return () => {
      if (released) return
      released = true
      current.consumers--
      if (!current.started && current.consumers === 0 && queue.requests.get(id) === current) {
        queue.requests.delete(id)
      }
    }
  }

  function prioritize(client: QueryClient, id: string) {
    const queue = queues.get(client)
    const pending = queue?.requests.get(id)
    if (!queue || !pending) return false
    if (!pending.started) start(client, queue, pending, false)
    return true
  }

  function prefetch(client: QueryClient, id: string) {
    if (!prioritize(client, id)) void client.prefetchQuery(prefetchOptions(id))
  }

  return { options, request, prioritize, prefetch }
}

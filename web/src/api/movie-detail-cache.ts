import { queryOptions, type QueryClient } from '@tanstack/react-query'

import type {
  BrowseMoviesParams,
  DiscoverMovie,
  DiscoverMovieDetail,
  JavDBZone,
  SearchMoviesParams
} from '@/api/discover'

export const discoverKeys = {
  all: ['discover'] as const,
  movies: (params: BrowseMoviesParams) => ['discover', 'movies', params] as const,
  movie: (id: string) => ['discover', 'movie', id] as const,
  magnets: (id: string) => ['discover', 'movie', id, 'magnets'] as const,
  search: (params: SearchMoviesParams) => ['discover', 'search', params] as const,
  tags: (zone: JavDBZone) => ['discover', 'tags', zone] as const,
  route: ['javdb', 'route'] as const
}

const detailStaleTime = 5 * 60_000

// List results already contain everything a card needs. Read them in place;
// never seed an incomplete list item into the full-detail query.
export function findCachedMovieCard(client: QueryClient, id: string, freshOnly = false) {
  let movie: DiscoverMovie | undefined
  let updatedAt = -1
  const oldest = freshOnly ? Date.now() - detailStaleTime : 0
  for (const query of client.getQueryCache().findAll({ queryKey: discoverKeys.all })) {
    const { data, dataUpdatedAt, isInvalidated } = query.state
    if (
      !data ||
      dataUpdatedAt < oldest ||
      dataUpdatedAt < updatedAt ||
      (freshOnly && isInvalidated)
    )
      continue
    const [, kind, key] = query.queryKey
    let candidate: DiscoverMovie | undefined
    if (kind === 'movie' && query.queryKey.length === 3 && key === id) {
      candidate = data as DiscoverMovieDetail
    } else if (kind === 'movies' || kind === 'search') {
      candidate = (data as DiscoverMovie[]).find(item => item.id === id)
    }
    if (candidate) {
      movie = candidate
      updatedAt = dataUpdatedAt
    }
  }
  return movie
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
      staleTime: detailStaleTime,
      retry: false,
      refetchOnWindowFocus: false
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

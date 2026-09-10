import { queryOptions, type QueryClient } from '@tanstack/react-query'

import type { DiscoverMovie } from '@/api/discover'

export type MovieIdentity = Pick<DiscoverMovie, 'id' | 'code'>
export type MovieLocalState = Pick<DiscoverMovie, 'state' | 'library_id'>
export type MovieStateResult = MovieLocalState & { id: string }
type StateLoader = (movie: MovieIdentity, signal: AbortSignal) => Promise<MovieLocalState>
type PendingState = {
  movie: MovieIdentity
  signal: AbortSignal
  resolve: (state: MovieLocalState) => void
  reject: (error: unknown) => void
}

export const movieStateKeys = {
  all: ['movie-states'] as const,
  movie: (id: string) => ['movie-states', id] as const
}

const emptyState: MovieLocalState = { state: 'not_in_library' }

// Cards and detail views share a query per catalogue ID. Batch subscriptions
// from the same render so a grid does not make a request for every badge.
export function createMovieStateLoader(
  fetchStates: (movies: MovieIdentity[]) => Promise<MovieStateResult[]>
): StateLoader {
  let pending: PendingState[] = []

  async function fetchBatch(requests: PendingState[]) {
    try {
      const identities = new Map(requests.map(({ movie }) => [movie.id, movie]))
      const states = await fetchStates([...identities.values()])
      const byID = new Map(states.map(({ id, ...state }) => [id, state]))
      for (const request of requests) {
        const state = byID.get(request.movie.id)
        if (request.signal.aborted) request.reject(request.signal.reason)
        else if (state) request.resolve(state)
        else request.reject(new Error('影片状态响应不完整'))
      }
    } catch (error) {
      for (const request of requests) request.reject(error)
    }
  }

  function flush() {
    const requests = pending
    pending = []
    const active = requests.filter(request => {
      if (!request.signal.aborted) return true
      request.reject(request.signal.reason)
      return false
    })
    for (let start = 0; start < active.length; start += 100) {
      void fetchBatch(active.slice(start, start + 100))
    }
  }

  return (movie, signal) =>
    new Promise((resolve, reject) => {
      pending.push({ movie: { id: movie.id, code: movie.code }, signal, resolve, reject })
      if (pending.length === 1) queueMicrotask(flush)
    })
}

export function movieStateOptions(load: StateLoader, movie?: MovieIdentity) {
  return queryOptions({
    queryKey: movieStateKeys.movie(movie?.id ?? ''),
    queryFn: ({ signal }) => (movie ? load(movie, signal) : Promise.resolve(emptyState)),
    enabled: Boolean(movie?.id),
    // Catalogue responses may predate a scan or account switch. They never
    // seed this cache or overwrite a more recent local-state response.
    initialData: emptyState,
    staleTime: Infinity,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
    retry: false
  })
}

export function invalidateMovieStates(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: movieStateKeys.all })
}

export async function resetMovieStates(queryClient: QueryClient) {
  await queryClient.cancelQueries({ queryKey: movieStateKeys.all })
  queryClient.setQueriesData<MovieLocalState>({ queryKey: movieStateKeys.all }, emptyState)
  return invalidateMovieStates(queryClient)
}

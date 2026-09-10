import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useSyncExternalStore } from 'react'

import { apiGet, apiPost, apiPut } from '@/api/client'
import {
  createMovieDetailLoader,
  discoverKeys,
  findCachedMovieCard
} from '@/api/movie-detail-cache'

export type MovieState = 'not_in_library' | 'saving' | 'processing' | 'in_library'
export type ReleaseStatus = 'unknown' | 'released' | 'upcoming'
export type JavDBZone = 'censored' | 'uncensored' | 'western' | 'fc2' | 'anime'
export type JavDBEntityType = 'actor' | 'series' | 'maker' | 'director'

export type MovieReference = {
  id: string
  code: string
  thumbnail: string
}

export type PreviewImage = {
  thumbnail: string
  original: string
}

export type Actor = {
  id: string
  name: string
  name_zht: string
  gender: string
  avatar: string
}

export type Tag = {
  id: string
  name: string
  name_zht: string
  category_id: string
}

export type NamedEntity = {
  id: string
  name: string
}

export type DiscoverMovie = {
  id: string
  library_id?: number
  code: string
  title: string
  origin_title: string
  release_date: string
  duration: number
  rating: number
  thumbnail: string
  cover: string
  preview_images: PreviewImage[]
  preview_video: string
  magnets_count: number
  has_subtitle: boolean
  has_preview: boolean
  actors: Actor[]
  tags: Tag[]
  series?: NamedEntity
  maker?: NamedEntity
  director?: NamedEntity
  state: MovieState
  release_status: ReleaseStatus
}

export type DiscoverMovieDetail = DiscoverMovie & {
  zone: JavDBZone
  actor_movies: MovieReference[]
  related_movies: MovieReference[]
}

export type DiscoverMagnet = {
  hash: string
  name: string
  size: number
  has_subtitle: boolean
  hd: boolean
  files_count: number
  created_at: string
  uri: string
}

export type TagCategory = {
  id: string
  name: string
  tags: NamedEntity[]
}

export type JavDBRouteStatus = {
  host: string
  latency_ms: number
  active: boolean
  manual: boolean
  candidates: JavDBRouteCandidate[]
}

export type JavDBRouteCandidate = {
  host: string
  latency_ms: number
  status: 'untested' | 'available' | 'unavailable'
}

export type BrowseMoviesParams = {
  zone?: JavDBZone
  entityType?: JavDBEntityType
  entityID?: string
  main?: string[]
  tagIds?: string[]
  year?: string
  month?: string
  sort?: string
  order?: 'asc' | 'desc'
  page?: number
  limit?: number
}

export type SearchMoviesParams = {
  query: string
  page?: number
  limit?: number
}

const discoverQueryDefaults = {
  retry: false,
  refetchOnWindowFocus: false
} as const

const movieDetails = createMovieDetailLoader((id, signal) =>
  apiGet<DiscoverMovieDetail>(`/api/discover/movies/${encodeURIComponent(id)}`, undefined, signal)
)

export function useDiscoverMovies(params: BrowseMoviesParams, enabled = true) {
  return useQuery({
    ...discoverQueryDefaults,
    queryKey: discoverKeys.movies(params),
    enabled,
    queryFn: ({ signal }) =>
      apiGet<DiscoverMovie[]>(
        '/api/discover/movies',
        {
          zone: params.zone,
          entity_type: params.entityType,
          entity_id: params.entityID,
          main: params.main,
          tag_id: params.tagIds,
          year: params.year,
          month: params.month,
          sort: params.sort,
          order: params.order,
          page: params.page,
          limit: params.limit
        },
        signal
      )
  })
}

export function useDiscoverMovie(id: string, enabled = true) {
  const queryClient = useQueryClient()
  const query = useQuery({ ...movieDetails.options(id), enabled })
  useEffect(() => {
    if (enabled) movieDetails.prioritize(queryClient, id)
  }, [queryClient, id, enabled])
  return query
}

export function useRecommendationMovie(id: string) {
  const queryClient = useQueryClient()
  const query = useQuery({ ...movieDetails.options(id), enabled: false })
  const subscribe = useCallback(
    (notify: () => void) =>
      queryClient.getQueryCache().subscribe(event => {
        if (
          event.type !== 'removed' &&
          !(event.type === 'updated' && event.action.type === 'success')
        )
          return
        const [namespace, kind, key] = event.query.queryKey
        if (
          namespace === 'discover' &&
          (kind === 'movies' ||
            kind === 'search' ||
            (kind === 'movie' && event.query.queryKey.length === 3 && key === id))
        )
          notify()
      }),
    [queryClient, id]
  )
  const snapshot = useCallback(() => findCachedMovieCard(queryClient, id), [queryClient, id])
  const movie = useSyncExternalStore(subscribe, snapshot, snapshot)
  const request = useCallback(() => movieDetails.request(queryClient, id), [queryClient, id])
  const prioritize = useCallback(() => movieDetails.prefetch(queryClient, id), [queryClient, id])
  return {
    movie,
    isError: query.isError,
    isFetching: query.isFetching,
    request,
    prioritize
  }
}

export function useDiscoverMagnets(id: string) {
  return useQuery({
    ...discoverQueryDefaults,
    queryKey: discoverKeys.magnets(id),
    queryFn: ({ signal }) =>
      apiGet<DiscoverMagnet[]>(
        `/api/discover/movies/${encodeURIComponent(id)}/magnets`,
        undefined,
        signal
      ),
    staleTime: 60_000
  })
}

export function useSearchMovies(params: SearchMoviesParams) {
  const query = params.query.trim()
  return useQuery({
    ...discoverQueryDefaults,
    queryKey: discoverKeys.search({ ...params, query }),
    queryFn: ({ signal }) =>
      apiGet<DiscoverMovie[]>(
        '/api/discover/search',
        {
          q: query,
          page: params.page,
          limit: params.limit
        },
        signal
      ),
    enabled: query.length > 0
  })
}

export function useDiscoverTags(zone: JavDBZone, enabled = true) {
  return useQuery({
    ...discoverQueryDefaults,
    queryKey: discoverKeys.tags(zone),
    queryFn: ({ signal }) => apiGet<TagCategory[]>('/api/discover/tags', { zone }, signal),
    enabled,
    staleTime: 24 * 60 * 60_000
  })
}

export function useJavDBRoute() {
  return useQuery({
    ...discoverQueryDefaults,
    queryKey: discoverKeys.route,
    queryFn: ({ signal }) => apiGet<JavDBRouteStatus>('/api/javdb/route', undefined, signal)
  })
}

export function useReselectJavDBRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<JavDBRouteStatus>('/api/javdb/reselect'),
    onSuccess: status => queryClient.setQueryData(discoverKeys.route, status),
    onError: () => queryClient.invalidateQueries({ queryKey: discoverKeys.route })
  })
}

export function useSelectJavDBRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (host: string) => apiPut<JavDBRouteStatus>('/api/javdb/route', { host }),
    onSuccess: status => queryClient.setQueryData(discoverKeys.route, status),
    onError: () => queryClient.invalidateQueries({ queryKey: discoverKeys.route })
  })
}

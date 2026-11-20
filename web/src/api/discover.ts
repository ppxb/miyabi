import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useSyncExternalStore } from 'react'

import { apiGet } from './client'
import {
  createMovieDetailLoader,
  discoverKeys,
  findCachedMovieCard,
  subscribeMovieCard
} from './movie-detail-cache'

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
  provider?: string
  id: string
  name: string
  name_zht: string
  gender: string
  avatar: string
}

export type Tag = {
  provider?: string
  id: string
  name: string
  name_zht: string
  category_id: string
}

export type NamedEntity = {
  provider?: string
  id: string
  name: string
}

export type DiscoverMovie = {
  readonly rating_source?: string
  readonly rating_max?: number
  id: string
  code: string
  title: string
  origin_title: string
  plot?: string
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
  release_status: ReleaseStatus
}

export type DiscoverMovieDetail = DiscoverMovie & {
  zone: JavDBZone | 'unknown'
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
  sources?: string[]
  tags?: string[]
  inferred?: boolean
}

export type TagCategory = {
  id: string
  name: string
  tags: NamedEntity[]
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

const movieDetails = createMovieDetailLoader((id, signal) =>
  apiGet<DiscoverMovieDetail>(`/api/discover/movies/${encodeURIComponent(id)}`, undefined, signal)
)

export function useDiscoverMovies(params: BrowseMoviesParams) {
  return useQuery({
    queryKey: discoverKeys.movies(params),
    placeholderData: keepPreviousData,
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

export function useDiscoverMovie(id: string, code?: string) {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: discoverKeys.movie(id),
    queryFn: ({ signal }) =>
      apiGet<DiscoverMovieDetail>(
        `/api/discover/movies/${encodeURIComponent(id)}`,
        code ? { code } : undefined,
        signal
      ),
    staleTime: 5 * 60_000
  })
  useEffect(() => {
    movieDetails.prioritize(queryClient, id)
  }, [queryClient, id])
  // Fall back to the card already rendered in the list only when the
  // enrichment request fails, so the dialog still opens instead of erroring.
  const card = query.isError ? findCachedMovieCard(queryClient, id) : undefined
  const data = query.data ?? (card ? cardToDetail(card) : undefined)
  return { ...query, data }
}

function cardToDetail(movie: DiscoverMovie): DiscoverMovieDetail {
  const partial = movie as Partial<DiscoverMovieDetail>
  return {
    ...movie,
    zone: partial.zone ?? 'unknown',
    actor_movies: partial.actor_movies ?? [],
    related_movies: partial.related_movies ?? []
  }
}

export function useResolveDiscoverMovie(code: string) {
  return useQuery({
    queryKey: discoverKeys.resolve(code),
    queryFn: ({ signal }) =>
      apiGet<{ id: string }>('/api/discover/movies/resolve', { code }, signal),
    staleTime: 5 * 60_000
  })
}

export function useRecommendationMovie(id: string) {
  const queryClient = useQueryClient()
  const query = useQuery({ ...movieDetails.options(id), enabled: false })
  const subscribe = useCallback(
    (notify: () => void) => subscribeMovieCard(queryClient, id, notify),
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

export function useDiscoverMagnets(id: string, code?: string) {
  return useQuery({
    queryKey: discoverKeys.magnets(id, code),
    queryFn: ({ signal }) =>
      apiGet<DiscoverMagnet[]>(
        `/api/discover/movies/${encodeURIComponent(id)}/magnets`,
        code ? { code } : undefined,
        signal
      ),
    staleTime: 60_000,
    enabled: Boolean(code)
  })
}

export function useDiscoverTags(zone: JavDBZone) {
  return useQuery({
    queryKey: discoverKeys.tags(zone),
    queryFn: ({ signal }) => apiGet<TagCategory[]>('/api/discover/tags', { zone }, signal),
    staleTime: 24 * 60 * 60_000
  })
}

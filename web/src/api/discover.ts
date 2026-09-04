import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'

export type MovieState = 'not_in_library' | 'saving' | 'in_library'
export type ReleaseStatus = 'unknown' | 'released' | 'upcoming'
export type JavDBZone = 'censored' | 'uncensored' | 'western' | 'fc2'

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

export type TagCategory = {
  id: string
  name: string
  name_zht: string
  tags: Tag[]
}

export type JavDBRouteStatus = {
  host: string
  latency_ms: number
  active: boolean
}

export type BrowseMoviesParams = {
  zone?: JavDBZone
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
  zone?: JavDBZone | 'all'
  sort?: string
  filterBy?: string
  page?: number
  limit?: number
}

const discoverKeys = {
  all: ['discover'] as const,
  movies: (params: BrowseMoviesParams) => [...discoverKeys.all, 'movies', params] as const,
  search: (params: SearchMoviesParams) => [...discoverKeys.all, 'search', params] as const,
  tags: (zone: JavDBZone) => [...discoverKeys.all, 'tags', zone] as const,
  route: ['javdb', 'route'] as const
}

export function useDiscoverMovies(params: BrowseMoviesParams) {
  return useQuery({
    queryKey: discoverKeys.movies(params),
    queryFn: () =>
      apiGet<DiscoverMovie[]>('/api/discover/movies', {
        zone: params.zone,
        main: params.main,
        tag_id: params.tagIds,
        year: params.year,
        month: params.month,
        sort: params.sort,
        order: params.order,
        page: params.page,
        limit: params.limit
      }),
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useSearchMovies(params: SearchMoviesParams) {
  const query = params.query.trim()
  return useQuery({
    queryKey: discoverKeys.search({ ...params, query }),
    queryFn: () =>
      apiGet<DiscoverMovie[]>('/api/discover/search', {
        q: query,
        zone: params.zone,
        sort: params.sort,
        filter_by: params.filterBy,
        page: params.page,
        limit: params.limit
      }),
    enabled: query.length > 0,
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useDiscoverTags(zone: JavDBZone, enabled = true) {
  return useQuery({
    queryKey: discoverKeys.tags(zone),
    queryFn: () => apiGet<TagCategory[]>('/api/discover/tags', { zone }),
    enabled,
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useJavDBRoute() {
  return useQuery({
    queryKey: discoverKeys.route,
    queryFn: () => apiGet<JavDBRouteStatus>('/api/javdb/route'),
    refetchOnWindowFocus: false
  })
}

export function useReselectJavDBRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<JavDBRouteStatus>('/api/javdb/reselect'),
    onSuccess: status => queryClient.setQueryData(discoverKeys.route, status)
  })
}

import { keepPreviousData, useQuery } from '@tanstack/react-query'

import { apiGet } from '@/api/client'
import type { LibraryPage } from '@/api/library'

export type ActorSource = {
  provider: string
  source_id: string
  name: string
  avatar?: string
}

export type ActorSummary = {
  key: string
  name: string
  name_zht?: string
  avatar?: string
  movie_count: number
  sources: ActorSource[]
}

export type ActorPage = {
  actors: ActorSummary[]
  total: number
  page: number
  has_more: boolean
}

export const ACTOR_PAGE_SIZE = 40

export type ActorFollowFilter = 'followed' | 'unfollowed'

export type ActorDetail = ActorSummary & {
  aliases: string[]
  names: string[]
  image_count: number
  films: LibraryPage
  summary?: string
  birthday?: string
  height?: number
  cup_size?: string
  measurements?: string
  blood_type?: string
  nationality?: string
  hobby?: string
  skill?: string
  debut_date?: string
  source_url?: string
  enriching?: boolean
  source_links?: { provider: string; url: string }[]
}

export const actorKeys = {
  all: ['actors'] as const,
  catalog: (q: string, follow: string, page: number) =>
    ['actors', 'catalog', q, follow, page] as const,
  detail: (key: string) => ['actors', 'detail', key] as const,
  // Actor pages embed library cards, so playback and subtitle changes refresh them.
  details: ['actors', 'detail'] as const
}

export function useActorCatalog(params: { q?: string; follow?: ActorFollowFilter; page: number }) {
  return useQuery({
    queryKey: actorKeys.catalog(params.q ?? '', params.follow ?? '', params.page),
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) =>
      apiGet<ActorPage>(
        '/api/actors',
        {
          q: params.q,
          follow: params.follow,
          page: params.page,
          limit: ACTOR_PAGE_SIZE
        },
        signal
      )
  })
}

export function useActorDetail(key: string) {
  return useQuery({
    queryKey: actorKeys.detail(key),
    queryFn: ({ signal }) => apiGet<ActorDetail>(`/api/actors/${key}`, undefined, signal),
    // Provider lookups keep running after the first response; poll until the
    // background enrichment finishes (the backend then stops setting enriching).
    refetchInterval: query => (query.state.data?.enriching ? 3000 : false)
  })
}

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
}

export const actorKeys = {
  all: ['actors'] as const,
  catalog: (q: string, page: number, limit: number) =>
    ['actors', 'catalog', q, page, limit] as const,
  detail: (key: string) => ['actors', 'detail', key] as const
}

export function useActorCatalog(params: { q?: string; page: number; limit: number }) {
  return useQuery({
    queryKey: actorKeys.catalog(params.q ?? '', params.page, params.limit),
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) =>
      apiGet<ActorPage>(
        '/api/actors',
        { q: params.q, page: params.page, limit: params.limit },
        signal
      )
  })
}

export function useActorDetail(key: string) {
  return useQuery({
    queryKey: actorKeys.detail(key),
    queryFn: ({ signal }) => apiGet<ActorDetail>(`/api/actors/${key}`, undefined, signal)
  })
}

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPost } from '@/api/client'
import { libraryKeys } from '@/api/library'
import { refreshQueries } from '@/api/query-refresh'

export type SubtitleTrack = {
  id: number
  name: string
  language: string
  format: string
  version: string
  source: string
  removable: boolean
}

export type SubtitleOffer = {
  provider: string
  name: string
  url: string
  format: string
  language: string
  version: string
}

export type SubtitleCue = { start: number; text: string }

export type SubtitlePreview = {
  language: string
  cues: number
  end: number
  runtime: number
  lines: SubtitleCue[]
}

export const subtitleKeys = {
  all: ['subtitles'] as const,
  tracks: (movieID: number) => ['subtitles', movieID, 'tracks'] as const,
  search: (movieID: number) => ['subtitles', movieID, 'search'] as const
}

const base = (movieID: number) => `/api/library/movies/${movieID}/subtitles`

export function useMovieSubtitles(movieID: number, enabled: boolean) {
  return useQuery({
    queryKey: subtitleKeys.tracks(movieID),
    queryFn: ({ signal }) => apiGet<SubtitleTrack[]>(base(movieID), undefined, signal),
    enabled
  })
}

// Online search is slow and hits external sites, so a result is kept for a while.
export function useSubtitleSearch(movieID: number, enabled: boolean) {
  return useQuery({
    queryKey: subtitleKeys.search(movieID),
    queryFn: ({ signal }) => apiGet<SubtitleOffer[]>(`${base(movieID)}/search`, undefined, signal),
    enabled,
    staleTime: 10 * 60_000
  })
}

export function usePreviewSubtitle(movieID: number) {
  return useMutation({
    mutationFn: (offer: SubtitleOffer) =>
      apiPost<SubtitlePreview>(`${base(movieID)}/preview`, offer)
  })
}

export function useSaveSubtitle(movieID: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (offer: SubtitleOffer) => apiPost<SubtitleTrack>(base(movieID), offer),
    // Library cards show a Chinese-subtitle badge derived from the tracks.
    onSuccess: () => refreshQueries(queryClient, subtitleKeys.tracks(movieID), libraryKeys.all),
    meta: { errorTitle: '下载字幕失败' }
  })
}

export function useDeleteSubtitle(movieID: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (trackID: number) => apiDelete<{ deleted: boolean }>(`${base(movieID)}/${trackID}`),
    onSuccess: () => refreshQueries(queryClient, subtitleKeys.tracks(movieID), libraryKeys.all),
    meta: { errorTitle: '删除字幕失败' }
  })
}

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

/** A cue as the player reads it, in seconds. */
export type SubtitlePlayerCue = { startTime: number; endTime: number; text: string }

export const subtitleKeys = {
  all: ['subtitles'] as const,
  tracks: (movieID: number) => ['subtitles', movieID, 'tracks'] as const,
  search: (movieID: number) => ['subtitles', movieID, 'search'] as const,
  cues: (movieID: number, trackID: number) => ['subtitles', movieID, 'cues', trackID] as const
}

const base = (movieID: number) => `/api/library/movies/${movieID}/subtitles`

const languageLabels: Record<string, string> = {
  'zh-CN': '简体中文',
  'zh-TW': '繁体中文',
  en: '英文',
  ja: '日文'
}

export const subtitleVersionLabels: Record<string, string> = {
  uncensored: '无码',
  leaked: '流出',
  extended: '加长'
}

export function subtitleLanguageLabel(language: string) {
  return languageLabels[language] ?? (language ? language : '语言待识别')
}

// A track's file only changes by deleting and saving it again, which gives it a new id.
export function subtitleCuesQuery(movieID: number, trackID: number) {
  return {
    queryKey: subtitleKeys.cues(movieID, trackID),
    queryFn: ({ signal }: { signal: AbortSignal }) =>
      apiGet<SubtitlePlayerCue[]>(`${base(movieID)}/${trackID}/cues`, undefined, signal),
    staleTime: Infinity
  }
}

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

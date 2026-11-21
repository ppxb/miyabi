import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'

import { apiDelete, apiGet, apiPost } from '@/api/client'

export type PlayFile = { id: string; name: string; path: string; size: number }

export type PlaySource = { src: string; type: string; label: string }

// Unified, extensible per-movie playback preferences. Future preferences
// (subtitles, audio tracks, ...) are just new fields here.
export type PlaybackSettings = {
  rate?: number
  quality?: string
}

export type PlaybackResume = {
  file_id: string
  position: number
  duration: number
  settings?: PlaybackSettings
}

export type Playback = {
  id: string
  file_id: string
  sources: PlaySource[]
  source: { account_id: string; directory: { id: string; name: string; path: string } }
  resume?: PlaybackResume
}

export function savePlaybackProgress(
  movieID: number,
  fileID: string,
  position: number,
  duration: number,
  settings?: PlaybackSettings
) {
  return apiPost('/api/play/progress', {
    movie_id: movieID,
    file_id: fileID,
    position,
    duration,
    settings
  })
}

const options = {
  gcTime: 0,
  staleTime: Infinity,
  refetchOnReconnect: false
} as const

export function usePlayQualities(fileID: string | undefined, enabled: boolean) {
  const query = useQuery({
    ...options,
    staleTime: 0,
    queryKey: ['play', 'qualities', fileID],
    queryFn: ({ signal }) =>
      apiGet<Playback>('/api/play/qualities', { file_id: fileID ?? undefined }, signal),
    enabled: enabled && !!fileID
  })
  const id = query.data?.id

  useEffect(() => {
    if (!id) return
    return () => {
      void apiDelete(`/api/play/${id}`).catch(() => {})
    }
  }, [id])

  return query
}

export function useStartMoviePlayback(movieID: number | null) {
  const query = useQuery({
    ...options,
    staleTime: 0,
    queryKey: ['play', 'start', movieID],
    queryFn: ({ signal }) =>
      apiGet<Playback>('/api/play/start', { movie_id: movieID ?? undefined }, signal),
    enabled: movieID !== null
  })
  const id = query.data?.id

  useEffect(() => {
    if (!id) return
    return () => {
      void apiDelete(`/api/play/${id}`).catch(() => {})
    }
  }, [id])

  return query
}

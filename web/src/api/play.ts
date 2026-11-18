import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'

import { apiDelete, apiGet } from '@/api/client'

export type PlayFile = { id: string; name: string; path: string; size: number }

export type PlayFiles = {
  code: string
  title: string
  files: PlayFile[]
  source: { account_id: string; directory: { id: string; name: string; path: string } }
}

export type PlaySource = { src: string; type: string; label: string }

export type Playback = { id: string; sources: PlaySource[] }

const options = {
  gcTime: 0,
  staleTime: Infinity,
  refetchOnReconnect: false
} as const

export function usePlayFiles(movieID: number | null) {
  return useQuery({
    ...options,
    queryKey: ['play', 'files', movieID],
    queryFn: ({ signal }) =>
      apiGet<PlayFiles>('/api/play/files', { movie_id: movieID ?? undefined }, signal),
    enabled: movieID !== null
  })
}

export function usePlayback(fileID?: string) {
  const query = useQuery({
    ...options,
    queryKey: ['play', 'source', fileID],
    queryFn: ({ signal }) =>
      apiGet<Playback>(`/api/play/${encodeURIComponent(fileID ?? '')}`, undefined, signal),
    enabled: !!fileID
  })
  const id = query.data?.id

  useEffect(() => {
    if (!id) return
    return () => {
      // The player aborts media requests on unmount; also release the server session.
      void apiDelete(`/api/play/${id}`).catch(() => {})
    }
  }, [id])

  return query
}

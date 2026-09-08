import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'

import { apiDelete, apiGet } from '@/api/client'
import type { LibraryFile } from '@/api/library'

export type PlayMode = 'original' | 'hls'

export type PlayFiles = {
  code: string
  title: string
  files: LibraryFile[]
}

export type PlaySource = {
  src: string
  type: 'video/object' | 'application/x-mpegurl'
  label: string
}

type Playback = {
  id: string
  sources: PlaySource[]
}

const playQueryOptions = {
  retry: false,
  gcTime: 0,
  staleTime: Infinity,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false
} as const

export function usePlayFiles(code: string) {
  return useQuery({
    ...playQueryOptions,
    queryKey: ['play', 'files', code],
    queryFn: ({ signal }) => apiGet<PlayFiles>('/api/play/files', { code }, signal)
  })
}

export function usePlayback(fileID: string, mode: PlayMode) {
  const query = useQuery({
    ...playQueryOptions,
    queryKey: ['play', 'source', fileID, mode],
    queryFn: ({ signal }) =>
      apiGet<Playback>(`/api/play/${encodeURIComponent(fileID)}`, { mode }, signal)
  })
  const id = query.data?.id

  useEffect(() => {
    if (!id) return
    return () => {
      // The player aborts media requests on unmount; also release the server's URL registry.
      // Abandoned tabs and failed cleanup requests expire on the server.
      void apiDelete(`/api/play/${id}`).catch(() => {})
    }
  }, [id])

  return query
}

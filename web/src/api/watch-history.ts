import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPost, apiPut } from '@/api/client'
import type { LibrarySource } from '@/api/tasks'

export const WATCH_HISTORY_PAGE_SIZE = 20

export type WatchSession = {
  id: number
  session_id: string
  file_id: string
  position: number
  duration: number
}

export type WatchProgress = {
  session_id: string
  file_id: string
  position: number
  duration: number
  version: number
}

export type WatchHistoryItem = {
  id: number
  movie_id: number
  code: string
  title: string
  cover?: string
  poster?: string
  watched_at: string
  position: number
  duration: number
}

export type WatchHistoryPage = {
  source?: LibrarySource
  items: WatchHistoryItem[]
  total: number
  page: number
  has_more: boolean
}

export const watchHistoryKeys = {
  all: ['library', 'history'] as const,
  page: (page: number) => ['library', 'history', page] as const
}

export function useWatchHistory(page: number) {
  return useQuery({
    queryKey: watchHistoryKeys.page(page),
    queryFn: ({ signal }) => apiGet<WatchHistoryPage>('/api/library/history', { page }, signal),
    staleTime: 0,
    retry: false,
    refetchOnWindowFocus: true
  })
}

export function saveWatchProgress(id: number, progress: WatchProgress, keepalive: boolean) {
  return apiPut<null>(`/api/library/history/${id}/progress`, progress, { keepalive })
}

type HistoryRemoval = { source: LibrarySource } & (
  | { type: 'selected'; ids: number[] }
  | { type: 'all' }
)

export function useRemoveWatchHistory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (removal: HistoryRemoval) => {
      const scope = {
        account_id: removal.source.account_id,
        directory_id: removal.source.directory.id
      }
      return removal.type === 'selected'
        ? apiPost<{ removed: number }>('/api/library/history/remove', {
            ...scope,
            ids: removal.ids
          })
        : apiDelete<{ removed: number }>(`/api/library/history?${new URLSearchParams(scope)}`)
    },
    onSuccess: async () => {
      await queryClient.cancelQueries({ queryKey: watchHistoryKeys.all })
      return queryClient.invalidateQueries({ queryKey: watchHistoryKeys.all })
    }
  })
}

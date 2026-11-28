import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet } from '@/api/client'
import { libraryKeys } from '@/api/library'

export const HISTORY_PAGE_SIZE = 20

export type WatchHistoryItem = {
  movie_id: number
  code: string
  title: string
  cover?: string
  poster?: string
  rating?: number
  actors?: Array<{ provider?: string; id?: string; name: string }>
  source?: string
  has_subtitle?: boolean
  watched_at: string
  position: number
  duration: number
}

export type WatchHistoryPage = {
  items: WatchHistoryItem[]
  total: number
  page: number
  has_more: boolean
}

export const historyKeys = {
  all: ['history'] as const,
  page: (page: number) => ['history', page] as const
}

export function useHistory(page: number) {
  return useQuery({
    queryKey: historyKeys.page(page),
    queryFn: ({ signal }) =>
      apiGet<WatchHistoryPage>('/api/history', { page, limit: HISTORY_PAGE_SIZE }, signal),
    placeholderData: keepPreviousData
  })
}

export function useClearHistory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiDelete<{ removed: number }>('/api/history'),
    meta: { errorTitle: '清空观看历史失败' },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: historyKeys.all })
      // The finished badge on library cards is derived from saved history.
      for (const queryKey of libraryKeys.cards) {
        void queryClient.invalidateQueries({ queryKey })
      }
    }
  })
}

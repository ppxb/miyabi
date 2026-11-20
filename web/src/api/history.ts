import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiDelete, apiGet, apiPost } from '@/api/client'

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
    onSuccess: () => queryClient.invalidateQueries({ queryKey: historyKeys.all })
  })
}

export function useRemoveHistory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (ids: number[]) => apiPost<{ removed: number }>('/api/history/remove', { ids }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: historyKeys.all })
  })
}

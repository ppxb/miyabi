import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { ApiError, apiGet, apiPost } from '@/api/client'
import { panKeys } from '@/api/pan'

export type OfflineSubmission = {
  task_id: number
  hash: string
  status: 'queued' | 'running' | 'done' | 'failed'
  phase: 'available' | 'downloading' | 'processing' | 'in_library' | 'downloaded'
  progress: number
  error?: string
}

export const offlineKeys = {
  all: ['offline'] as const,
  movie: (movieID: string, accountID: string) => ['offline', accountID, movieID] as const
}

export function useOfflineTasks(movieID: string, accountID: string) {
  return useQuery({
    queryKey: offlineKeys.movie(movieID, accountID),
    queryFn: ({ signal }) =>
      apiGet<OfflineSubmission[]>(
        `/api/discover/movies/${encodeURIComponent(movieID)}/offline`,
        { account_id: accountID },
        signal
      ),
    enabled: accountID !== '',
    staleTime: 0,
    retry: false,
    refetchInterval: query => {
      if (query.state.status === 'error') return false
      return query.state.data?.some(
        task => task.phase === 'downloading' || task.phase === 'processing'
      )
        ? 5000
        : false
    }
  })
}

export function useAddOffline(movieID: string, accountID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (hash: string) =>
      apiPost<OfflineSubmission>(`/api/discover/movies/${encodeURIComponent(movieID)}/offline`, {
        hash
      }),
    retry: false,
    onSuccess: submission => {
      queryClient.setQueryData<OfflineSubmission[]>(
        offlineKeys.movie(movieID, accountID),
        tasks => [submission, ...(tasks ?? []).filter(task => task.hash !== submission.hash)]
      )
    },
    onError: error => {
      if (error instanceof ApiError && (error.status === 401 || error.status === 400)) {
        void queryClient.invalidateQueries({ queryKey: panKeys.account })
      }
    }
  })
}

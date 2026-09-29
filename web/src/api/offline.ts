import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'
import { invalidateMovieStates } from '@/api/movie-states'
import { panKeys, type PanAccountStatus } from '@/api/pan'
import type { LibrarySource } from '@/api/tasks'

export function isOfflineTaskActive(task: OfflineSubmission) {
  return task.phase === 'downloading' || task.processing
}

export type OfflineSubmission = {
  task_id: number
  code: string
  javdb_id: string
  library_id?: number
  account_id: string
  directory_id: string
  scan_task_id?: number
  hash: string
  status: 'queued' | 'running' | 'done' | 'failed'
  phase: 'available' | 'downloading' | 'processing' | 'in_library' | 'downloaded'
  processing: boolean
  progress: number
  error?: string
}

export type OfflineActivity = { source?: LibrarySource; tasks: OfflineSubmission[] }

export const offlineKeys = {
  all: ['offline'] as const,
  activity: ['offline', 'activity'] as const
}

const activityOptions = queryOptions({
  queryKey: offlineKeys.activity,
  queryFn: ({ signal }) => apiGet<OfflineActivity>('/api/offline/tasks', undefined, signal),
  staleTime: Infinity,
  refetchOnMount: 'always'
})

export function useOfflineActivity() {
  return useQuery({
    ...activityOptions,
    refetchInterval: query => (query.state.data?.tasks.some(isOfflineTaskActive) ? 5000 : false)
  })
}

export function useOfflineTasks(movieID: string, accountID: string, directoryID: string) {
  return useQuery({
    ...activityOptions,
    enabled: accountID !== '' && directoryID !== '',
    select: activity =>
      activity.source?.account_id === accountID && activity.source.directory.id === directoryID
        ? activity.tasks.filter(task => task.javdb_id === movieID)
        : []
  })
}

export function useAddOffline(movieID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (hash: string) =>
      apiPost<OfflineSubmission>(`/api/discover/movies/${encodeURIComponent(movieID)}/offline`, {
        hash
      }),
    onSuccess: async submission => {
      await queryClient.cancelQueries({ queryKey: offlineKeys.activity, exact: true })
      const account = queryClient.getQueryData<PanAccountStatus>(panKeys.account)
      if (
        account &&
        (!account.connected ||
          account.account?.id !== submission.account_id ||
          account.directory?.id !== submission.directory_id)
      ) {
        void queryClient.invalidateQueries({ queryKey: offlineKeys.all })
        return
      }
      queryClient.setQueryData<OfflineActivity>(offlineKeys.activity, activity =>
        activity?.source?.account_id === submission.account_id &&
        activity.source.directory.id === submission.directory_id
          ? {
              ...activity,
              tasks: [submission, ...activity.tasks.filter(task => task.hash !== submission.hash)]
            }
          : activity
      )
      void invalidateMovieStates(queryClient)
      void queryClient.invalidateQueries({ queryKey: offlineKeys.all })
    }
  })
}

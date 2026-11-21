import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'
import { movieStateKeys } from '@/api/movie-states'
import { refreshQueries, updateIfCurrent } from '@/api/query-refresh'
import type { LibrarySource } from '@/api/tasks'
import { sameSource } from '@/lib/source'

export function isOfflineTaskActive(task: OfflineSubmission) {
  return task.phase === 'downloading' || task.processing
}

export type OfflineSubmission = {
  task_id: number
  code: string
  library_id?: number
  account_id: string
  directory_id: string
  scan_task_id?: number
  hash: string
  status: 'queued' | 'running' | 'done' | 'failed' | 'cancelled'
  phase: 'available' | 'downloading' | 'processing' | 'in_library' | 'downloaded'
  processing: boolean
  progress: number
  error?: string
  download_state?:
    | 'queued'
    | 'stalled'
    | 'exhausted'
    | 'switching'
    | 'submitting'
    | 'cancelling'
    | 'waiting'
  attempt_count?: number
  switch_reason?: string
  can_cancel?: boolean
  can_switch?: boolean
  retry_at?: string
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

export function useMovieOfflineTasks(movieID: string) {
  return useQuery({
    ...activityOptions,
    select: activity => activity.tasks.filter(task => task.code === movieID)
  })
}

export function useAddOffline(movieID: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ accountID, hash }: { accountID: string; hash: string }) =>
      apiPost<OfflineSubmission>(`/api/discover/movies/${encodeURIComponent(movieID)}/offline`, {
        account_id: accountID,
        hash
      }),
    onMutate: () => queryClient.getQueryData<OfflineActivity>(offlineKeys.activity),
    onSuccess: (submission, _vars, previous) => {
      // Merge the new task unless a read/SSE refresh already superseded it.
      updateIfCurrent(queryClient, offlineKeys.activity, previous, activity =>
        activity
          ? {
              ...activity,
              tasks: [
                submission,
                ...activity.tasks.filter(
                  task => task.task_id !== submission.task_id && task.hash !== submission.hash
                )
              ]
            }
          : activity
      )
      void refreshQueries(queryClient, movieStateKeys.all, offlineKeys.all)
    }
  })
}

export function useOfflineControl(action: 'cancel' | 'next') {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (taskID: number) =>
      apiPost<OfflineSubmission>(`/api/offline/tasks/${taskID}/${action}`),
    onMutate: () => queryClient.getQueryData<OfflineActivity>(offlineKeys.activity),
    onSuccess: (submission, _taskID, previous) => {
      updateIfCurrent(queryClient, offlineKeys.activity, previous, activity =>
        activity && sameSource(activity.source, submission)
          ? {
              ...activity,
              tasks: activity.tasks.map(task =>
                task.task_id === submission.task_id ? submission : task
              )
            }
          : activity
      )
      void refreshQueries(queryClient, movieStateKeys.all)
    },
    onSettled: () => refreshQueries(queryClient, offlineKeys.all),
    meta: { errorTitle: action === 'cancel' ? '取消下载失败' : '切换磁力失败' }
  })
}

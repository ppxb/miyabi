import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { apiGet, apiPost, apiPut } from '@/api/client'
import { offlineKeys } from '@/api/offline'
import type { PanDirectory } from '@/api/pan'

export type LibrarySource = {
  account_id: string
  directory: PanDirectory
}

export type TaskRevisions = { library: number; offline: number; monitor: number }

type TaskBase = {
  id: number
  status: 'queued' | 'running' | 'done' | 'failed'
  progress: number
  error?: string
  can_retry?: boolean
  retry_at?: string
  retry_count?: number
  created_at: string
  updated_at: string
}

export type ScanTask = TaskBase & {
  type: 'scan'
  rebuild?: boolean
  source: LibrarySource
  offline_task_id?: number
  paused?: boolean
  scan: {
    stage: 'queued' | 'scanning' | 'reconciling' | 'scraping' | 'artwork' | 'done'
    current_path: string
    directories_discovered: number
    directories_scanned: number
    files_scanned: number
    video_files: number
    matched_files: number
    unmatched_files: number
    movies: number
    removed_files: number
    removed_movies: number
    metadata_total: number
    metadata_completed: number
    metadata_retrying?: number
    metadata_failed?: number
  }
}

// A queued subscription batch: each movie is submitted to 115, left waiting
// for a qualifying magnet, or failed.
export type BatchTask = TaskBase & {
  type: 'subscription_batch'
  batch: {
    total: number
    processed: number
    submitted: number
    waiting: number
    failed: number
    failures?: { code: string; error: string }[]
  }
}

export type Task = ScanTask | BatchTask

export const taskKeys = { all: ['tasks'] as const }

export function useTasks() {
  return useQuery({
    queryKey: taskKeys.all,
    queryFn: ({ signal }) => apiGet<Task[]>('/api/tasks', undefined, signal),
    staleTime: Infinity,
    refetchOnMount: 'always'
  })
}

export function useRetryTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => apiPost<Task>(`/api/tasks/${id}/retry`),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: taskKeys.all }),
        queryClient.invalidateQueries({ queryKey: offlineKeys.all })
      ])
    },
    onError: error => toast.error('重试失败', { description: error.message })
  })
}

export function useSetLibraryPaused() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (paused: boolean) => apiPut('/api/tasks/library-control', { paused }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: taskKeys.all }),
    onError: error => toast.error('操作失败', { description: error.message })
  })
}

export function isScanTask(task: Task): task is ScanTask {
  return task.type === 'scan'
}

export function isTaskActive(task: Task) {
  return task.status === 'queued' || task.status === 'running'
}

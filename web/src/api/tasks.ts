import { useQuery } from '@tanstack/react-query'

import { apiGet } from '@/api/client'
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
  created_at: string
  updated_at: string
}

export type ScanTask = TaskBase & {
  type: 'scan'
  source: LibrarySource
  offline_task_id?: number
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

export function isScanTask(task: Task): task is ScanTask {
  return task.type === 'scan'
}

export function isBatchTask(task: Task): task is BatchTask {
  return task.type === 'subscription_batch'
}

export function isTaskActive(task: Task) {
  return task.status === 'queued' || task.status === 'running'
}

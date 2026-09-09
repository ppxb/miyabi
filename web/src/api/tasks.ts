import { useQuery } from '@tanstack/react-query'

import { apiGet } from '@/api/client'
import type { PanDirectory } from '@/api/pan'

export type LibrarySource = {
  account_id: string
  directory: PanDirectory
}

export type TaskRevisions = { library: number; offline: number }

export type ScanTask = {
  id: number
  type: 'scan'
  status: 'queued' | 'running' | 'done' | 'failed'
  progress: number
  error?: string
  created_at: string
  updated_at: string
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

export const taskKeys = { all: ['tasks'] as const }

export function useTasks() {
  return useQuery({
    queryKey: taskKeys.all,
    queryFn: ({ signal }) => apiGet<ScanTask[]>('/api/tasks', undefined, signal),
    staleTime: Infinity,
    refetchOnMount: 'always',
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function isTaskActive(task: ScanTask) {
  return task.status === 'queued' || task.status === 'running'
}

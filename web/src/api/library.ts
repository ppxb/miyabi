import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ApiError, apiGet, apiPost, apiPut } from '@/api/client'
import { panKeys, type PanAccountStatus } from '@/api/pan'
import { taskKeys, type LibrarySource, type ScanTask } from '@/api/tasks'
import type { WatchSession } from '@/api/watch-history'
import { notifyScanTask, notifyTaskError } from '@/features/tasks/task-toast'

export const LIBRARY_PAGE_SIZE = 20

export type LibraryEntity = { id?: string; name: string }

export type LibraryMovie = {
  id: number
  code: string
  title: string
  javdb_id?: string
  cover?: string
  poster?: string
  fanart?: string
  release_date?: string
  duration: number
  rating: number
  maker?: LibraryEntity
  series?: LibraryEntity
  director?: LibraryEntity
  actors: LibraryEntity[]
  tags: Array<{ id: number; javdb_id: string; name: string }>
  scrape_status: 'pending' | 'done' | 'failed'
  watched: boolean
}

type LibraryPage = {
  source?: LibrarySource
  movies: LibraryMovie[]
  total: number
  page: number
  has_more: boolean
}

export type LibraryFile = { id: string; name: string; path: string; size: number }

export const libraryKeys = {
  all: ['library'] as const,
  movieLists: ['library', 'movies'] as const,
  movies: (page: number) => ['library', 'movies', page] as const
}

export function useLibraryMovies(page: number) {
  return useQuery({
    queryKey: libraryKeys.movies(page),
    queryFn: ({ signal }) =>
      apiGet<LibraryPage>('/api/library/movies', { page, limit: LIBRARY_PAGE_SIZE }, signal),
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useMarkMovieWatched() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (movieID: number) =>
      apiPut<{ id: number; watched: boolean; history: WatchSession }>(
        `/api/library/movies/${movieID}/watched`,
        {}
      ),
    retry: (failures, error) =>
      failures < 2 && (!(error instanceof ApiError) || error.status >= 500),
    onSuccess: async ({ id, watched }) => {
      // An older list response must not restore "unwatched" after the write succeeds.
      await queryClient.cancelQueries({ queryKey: libraryKeys.movieLists })
      queryClient.setQueriesData<LibraryPage>({ queryKey: libraryKeys.movieLists }, page =>
        page
          ? {
              ...page,
              movies: page.movies.map(movie => (movie.id === id ? { ...movie, watched } : movie))
            }
          : page
      )
      // Refresh lists in the background so playback can start with the watch session.
      void queryClient.invalidateQueries({ queryKey: libraryKeys.all })
    },
    onError: () => {
      toast.error('观看状态保存失败', {
        id: 'library:watched-error',
        description: '请检查后端连接，稍后重新打开影片即可重试。'
      })
    }
  })
}

export function useStartLibraryScan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<ScanTask>('/api/library/scan'),
    onSuccess: task => {
      const account = queryClient.getQueryData<PanAccountStatus>(panKeys.account)
      if (
        account &&
        (!account.connected ||
          account.account?.id !== task.source.account_id ||
          account.directory?.id !== task.source.directory.id)
      )
        return queryClient.invalidateQueries({ queryKey: taskKeys.all })
      notifyScanTask(task)
      queryClient.setQueryData<ScanTask[]>(taskKeys.all, tasks => [
        task,
        ...(tasks ?? []).filter(item => item.id !== task.id)
      ])
      return queryClient.invalidateQueries({ queryKey: taskKeys.all })
    },
    onError: error => {
      notifyTaskError(
        'scan:submit-error',
        '无法创建扫描任务',
        error instanceof ApiError
          ? error.status === 401
            ? '115 登录已失效，请前往设置重新登录。'
            : error.message
          : '请检查后端服务和 115 连接后重试。'
      )
      if (error instanceof ApiError && (error.status === 401 || error.status === 400)) {
        void queryClient.invalidateQueries({ queryKey: panKeys.account })
        void queryClient.invalidateQueries({ queryKey: libraryKeys.all })
      }
    }
  })
}

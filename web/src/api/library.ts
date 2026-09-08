import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { ApiError, apiGet, apiPost } from '@/api/client'
import { panKeys } from '@/api/pan'
import { taskKeys, type LibrarySource, type ScanTask } from '@/api/tasks'

export type LibraryMovie = {
  id: number
  code: string
  title: string
  javdb_id?: string
  cover?: string
  poster?: string
  scrape_status: 'pending' | 'done' | 'failed'
  file_count: number
  size: number
}

type LibraryPage = {
  source?: LibrarySource
  movies: LibraryMovie[]
  total: number
  file_count: number
  unmatched_files: number
  page: number
  has_more: boolean
}

type LibraryFilePage = {
  files: { id: string; name: string; path: string; size: number }[]
  total: number
  page: number
  has_more: boolean
}

export const libraryKeys = {
  all: ['library'] as const,
  movies: (page: number) => ['library', 'movies', page] as const,
  files: (movieID: number | undefined, unmatched: boolean, page: number) =>
    ['library', 'files', { movieID, unmatched, page }] as const
}

export function useLibraryMovies(page: number) {
  return useQuery({
    queryKey: libraryKeys.movies(page),
    queryFn: ({ signal }) => apiGet<LibraryPage>('/api/library/movies', { page }, signal),
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useLibraryFiles(movieID: number | undefined, unmatched: boolean, page: number) {
  return useQuery({
    queryKey: libraryKeys.files(movieID, unmatched, page),
    queryFn: ({ signal }) =>
      apiGet<LibraryFilePage>(
        '/api/library/files',
        { movie_id: movieID, unmatched: unmatched ? 'true' : undefined, page },
        signal
      ),
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false
  })
}

export function useStartLibraryScan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<ScanTask>('/api/library/scan'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: taskKeys.all }),
    onError: error => {
      if (error instanceof ApiError && (error.status === 401 || error.status === 400)) {
        void queryClient.invalidateQueries({ queryKey: panKeys.account })
        void queryClient.invalidateQueries({ queryKey: libraryKeys.all })
      }
    }
  })
}

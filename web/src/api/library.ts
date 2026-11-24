import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient
} from '@tanstack/react-query'

import { apiGet, apiPost } from '@/api/client'
import type { DiscoverMovieDetail } from '@/api/discover'
import { movieStateKeys } from '@/api/movie-states'
import { refreshQueries, updateIfCurrent } from '@/api/query-refresh'
import { panKeys, type PanAccountStatus } from '@/api/pan'
import { taskKeys, type LibrarySource, type ScanTask, type Task } from '@/api/tasks'

export const LIBRARY_PAGE_SIZE = 20

export type LibraryEntity = { provider?: string; id?: string; name: string }

export type LibraryMovie = {
  id: number
  code: string
  title: string
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
  tags: Array<{ id: number; provider: string; source_id: string; name: string }>
  scrape_status: 'pending' | 'done' | 'failed'
  source?: string
  has_subtitle?: boolean
  has_uncensored?: boolean
  has_cracked?: boolean
  finished?: boolean
}

export type LibraryPage = {
  source?: LibrarySource
  movies: LibraryMovie[]
  total: number
  page: number
  has_more: boolean
}

export type LibraryFilterOption = { id: string; name: string }

export type LibraryFilterOptions = {
  tags: LibraryFilterOption[]
  actors: LibraryFilterOption[]
  makers: LibraryFilterOption[]
  series: LibraryFilterOption[]
}

export type BrowseLibraryParams = {
  q?: string
  sort?: 'recent' | 'rating' | 'release'
  source?: '115' | 'local'
  tagNames?: string[]
  actorNames?: string[]
  makerNames?: string[]
  seriesNames?: string[]
  page?: number
  limit?: number
}

export const libraryKeys = {
  all: ['library'] as const,
  detail: (id: number) => ['library', 'detail', id] as const,
  movies: (page: number) => ['library', 'movies', page] as const,
  browse: (params: BrowseLibraryParams) => ['library', 'browse', params] as const,
  filterOptions: ['library', 'filter-options'] as const
}

export type LibraryMovieDetail = Omit<DiscoverMovieDetail, 'release_status'> & {
  library_id: number
  scrape_status: LibraryMovie['scrape_status']
  source?: string
}

export function useLibraryMovie(id: number) {
  return useQuery({
    queryKey: libraryKeys.detail(id),
    queryFn: ({ signal }) =>
      apiGet<LibraryMovieDetail>(`/api/library/movies/${id}`, undefined, signal),
    staleTime: 5 * 60_000,
    gcTime: 30 * 60_000
  })
}

function libraryMoviesOptions(page: number) {
  return queryOptions({
    queryKey: libraryKeys.movies(page),
    queryFn: ({ signal }) =>
      apiGet<LibraryPage>('/api/library/movies', { page, limit: LIBRARY_PAGE_SIZE }, signal)
  })
}

export function useLibraryMovies(page: number) {
  return useQuery(libraryMoviesOptions(page))
}

export function useTopRatedMovies(limit = 12) {
  return useQuery({
    queryKey: ['library', 'top-rated', limit] as const,
    queryFn: ({ signal }) => apiGet<LibraryMovie[]>('/api/library/top-rated', { limit }, signal)
  })
}

// Reuses the first library page (same cache entry as useLibraryMovies(1)) and
// slices the newest few locally, so the library page issues the page-1 request
// only once instead of a second time with a smaller limit.
const recentMovieLimit = 12

const selectRecentMovies = (page: LibraryPage): LibraryPage => ({
  ...page,
  movies: page.movies.slice(0, recentMovieLimit)
})

export function useRecentMovies() {
  return useQuery({ ...libraryMoviesOptions(1), select: selectRecentMovies })
}

export function useBrowseLibraryMovies(params: BrowseLibraryParams) {
  return useQuery({
    queryKey: libraryKeys.browse(params),
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) =>
      apiGet<LibraryPage>(
        '/api/library/browse',
        {
          q: params.q,
          sort: params.sort,
          source: params.source,
          tag: params.tagNames,
          actor: params.actorNames,
          maker: params.makerNames,
          series: params.seriesNames,
          page: params.page,
          limit: params.limit
        },
        signal
      )
  })
}

export function useLibraryFilterOptions() {
  return useQuery({
    queryKey: libraryKeys.filterOptions,
    queryFn: ({ signal }) =>
      apiGet<LibraryFilterOptions>('/api/library/filter-options', undefined, signal),
    staleTime: 5 * 60_000
  })
}

export function useStartLibraryScan() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (rebuild: boolean) =>
      apiPost<ScanTask>(rebuild ? '/api/library/rebuild' : '/api/library/scan'),
    onMutate: () => queryClient.getQueryData<Task[]>(taskKeys.all),
    onSuccess: (task, _rebuild, previous) => {
      cacheScanTask(queryClient, task, previous)
      return refreshQueries(queryClient, taskKeys.all)
    }
  })
}

export function useSyncLibrary() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<{ started: number }>('/api/library/sync'),
    onSuccess: () => refreshQueries(queryClient, taskKeys.all, libraryKeys.all)
  })
}

export function useLibraryPending() {
  return useQuery({
    queryKey: ['library', 'pending'] as const,
    queryFn: ({ signal }) => apiGet<{ count: number }>('/api/library/pending', undefined, signal),
    staleTime: 15_000
  })
}

export function useRescrapeLibraryMovie(id: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (code?: string) => apiPost<ScanTask>(`/api/library/movies/${id}/scrape`, { code }),
    onMutate: () => queryClient.getQueryData<Task[]>(taskKeys.all),
    onSuccess: (task, _code, previous) => {
      cacheScanTask(queryClient, task, previous)
      return refreshQueries(queryClient, libraryKeys.all, taskKeys.all, movieStateKeys.all)
    }
  })
}

function cacheScanTask(queryClient: QueryClient, task: ScanTask, previous: Task[] | undefined) {
  const account = queryClient.getQueryData<PanAccountStatus>(panKeys.account)
  if (
    account &&
    (!account.connected ||
      account.account?.id !== task.source.account_id ||
      account.directory?.id !== task.source.directory.id)
  )
    return
  updateIfCurrent(queryClient, taskKeys.all, previous, tasks => [
    task,
    ...(tasks ?? []).filter(item => item.id !== task.id)
  ])
}

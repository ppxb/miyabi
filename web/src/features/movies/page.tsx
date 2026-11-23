import { useCallback, useMemo, useState } from 'react'

import { useBrowseLibraryMovies, useLibraryFilterOptions } from '@/api/library'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { LazyLibraryPlayerDialog } from '@/features/library/player-dialog-lazy'
import { MovieFilterBar } from '@/features/movies/filter-bar'
import {
  moviesFilterFromSearch,
  moviesSearchFromFilter,
  type MoviesSearch
} from '@/features/movies/search'
import type { MoviesFilter } from '@/features/movies/types'

const PAGE_SIZE = 20

export function MoviesPage({
  search,
  onSearchChange
}: {
  search: MoviesSearch
  onSearchChange: (next: MoviesSearch) => void
}) {
  const [playingId, setPlayingId] = useState<number | null>(null)
  const options = useLibraryFilterOptions()
  const filters = useMemo(() => moviesFilterFromSearch(search), [search])
  const page = search.page ?? 1
  const movies = useBrowseLibraryMovies({ ...filters, page, limit: PAGE_SIZE })
  const items = movies.data?.movies ?? []
  const totalPages = Math.max(1, Math.ceil((movies.data?.total ?? 0) / PAGE_SIZE))

  const handleFiltersChange = useCallback(
    (next: MoviesFilter) => {
      onSearchChange({ ...moviesSearchFromFilter(next), page: undefined })
    },
    [onSearchChange]
  )

  const handlePageChange = useCallback(
    (next: number) => {
      onSearchChange({ ...moviesSearchFromFilter(filters), page: next > 1 ? next : undefined })
    },
    [filters, onSearchChange]
  )

  return (
    <AppPage>
      <PageHeader title="影片" description="在此聚合了所有来源的全部影片" />

      <MovieFilterBar options={options.data} value={filters} onChange={handleFiltersChange} />

      {movies.isPending ? (
        <MovieGridSkeleton count={PAGE_SIZE} />
      ) : movies.isError ? (
        <ErrorState
          message="无法读取影片，请检查后端服务后重试"
          onRetry={() => void movies.refetch()}
          retrying={movies.isFetching}
        />
      ) : items.length === 0 ? (
        <EmptyState title="没有符合条件的影片" />
      ) : (
        <>
          <MovieGridLayout>
            {items.map(movie => (
              <LibraryMovieCard
                key={movie.id}
                movie={movie}
                onPlay={() => setPlayingId(movie.id)}
              />
            ))}
          </MovieGridLayout>
          <ListPagination
            page={page}
            totalPages={totalPages}
            hasMore={movies.data?.has_more ?? false}
            disabled={movies.isFetching}
            onPageChange={handlePageChange}
          />
        </>
      )}

      <LazyLibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
    </AppPage>
  )
}

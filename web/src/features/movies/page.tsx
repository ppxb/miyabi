import { lazy, Suspense, useCallback, useState } from 'react'

import { useBrowseLibraryMovies, useLibraryFilterOptions } from '@/api/library'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { MovieFilterBar } from '@/features/movies/filter-bar'
import type { MoviesFilter } from '@/features/movies/types'

const PAGE_SIZE = 20

const LibraryPlayerDialog = lazy(() =>
  import('@/features/library/player-dialog').then(module => ({
    default: module.LibraryPlayerDialog
  }))
)

export function MoviesPage() {
  const [filters, setFilters] = useState<MoviesFilter>({ sort: 'recent' })
  const [page, setPage] = useState(1)
  const [playingId, setPlayingId] = useState<number | null>(null)
  const options = useLibraryFilterOptions()
  const movies = useBrowseLibraryMovies({ ...filters, page, limit: PAGE_SIZE })
  const items = movies.data?.movies ?? []
  const totalPages = Math.max(1, Math.ceil((movies.data?.total ?? 0) / PAGE_SIZE))

  const handleFiltersChange = useCallback((next: MoviesFilter) => {
    setFilters(next)
    setPage(1)
  }, [])

  return (
    <AppPage>
      <PageHeader title="影片" description="在此聚合了所有来源的全部影片" />

      <MovieFilterBar options={options.data} value={filters} onChange={handleFiltersChange} />

      {movies.isPending ? (
        <MovieGridSkeleton count={PAGE_SIZE} compact />
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
            onPageChange={setPage}
          />
        </>
      )}

      {playingId !== null ? (
        <Suspense fallback={null}>
          <LibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
        </Suspense>
      ) : null}
    </AppPage>
  )
}

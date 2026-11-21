import type { DiscoverMovie } from '@/api/discover'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridSkeleton } from '@/components/movie'
import { DISCOVER_PAGE_SIZE } from './constants'
import { MovieGrid } from './movie-grid'

export function DiscoverResults({
  movies,
  loading,
  fetching,
  error,
  page,
  onPageChange,
  onRetry
}: {
  movies: DiscoverMovie[] | undefined
  loading: boolean
  fetching: boolean
  error: boolean
  page: number
  onPageChange: (page: number) => void
  onRetry: () => void
}) {
  if (error) {
    return (
      <ErrorState
        message="数据加载失败，请检查网络或代理设置"
        onRetry={onRetry}
        retrying={fetching}
      />
    )
  }
  if (loading || !movies) return <MovieGridSkeleton count={DISCOVER_PAGE_SIZE} />
  return (
    <div className="flex flex-col gap-6">
      {movies.length === 0 && page === 1 ? (
        <EmptyState title="暂无内容" />
      ) : (
        <>
          <MovieGrid movies={movies} />
          <ListPagination
            page={page}
            hasMore={movies.length === DISCOVER_PAGE_SIZE}
            disabled={fetching}
            onPageChange={onPageChange}
          />
        </>
      )}
    </div>
  )
}

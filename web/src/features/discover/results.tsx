import type { DiscoverMovie } from '@/api/discover'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGrid, MovieGridSkeleton } from '@/components/movie'
import { DISCOVER_PAGE_SIZE } from './constants'

export function DiscoverResults({
  movies,
  loading,
  fetching,
  error,
  searching,
  page,
  onPageChange,
  onRetry
}: {
  movies: DiscoverMovie[] | undefined
  loading: boolean
  fetching: boolean
  error: boolean
  searching: boolean
  page: number
  onPageChange: (page: number) => void
  onRetry: () => void
}) {
  if (error) {
    return <ErrorState message="数据加载失败" onRetry={onRetry} retrying={fetching} />
  }
  if (loading || !movies) return <MovieGridSkeleton count={DISCOVER_PAGE_SIZE} />
  return (
    <div className="flex flex-col gap-6" aria-busy={fetching}>
      {movies.length === 0 && page === 1 ? (
        <EmptyState title={searching ? '没有搜索结果' : '暂无内容'} />
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

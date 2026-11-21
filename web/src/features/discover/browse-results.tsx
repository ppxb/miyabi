import type { BrowseMoviesParams } from '@/api/discover'
import { useDiscoverMovies } from '@/api/discover'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridSkeleton } from '@/components/movie'
import { DISCOVER_PAGE_SIZE } from './constants'
import { MovieGrid } from './movie-grid'

export function BrowseResults({
  params,
  onPageChange
}: {
  params: BrowseMoviesParams & { page: number }
  onPageChange: (page: number) => void
}) {
  const movies = useDiscoverMovies(params)
  if (movies.isError) {
    return (
      <ErrorState
        message="数据加载失败，请检查网络或代理设置"
        onRetry={() => movies.refetch()}
        retrying={movies.isFetching}
      />
    )
  }
  if (movies.isPending) {
    return <MovieGridSkeleton count={DISCOVER_PAGE_SIZE} />
  }
  const items = movies.data
  return (
    <div className="flex flex-col gap-6">
      {items.length === 0 && params.page === 1 ? (
        <EmptyState title="暂无内容" />
      ) : (
        <>
          <MovieGrid movies={items} />
          <ListPagination
            page={params.page}
            hasMore={items.length === DISCOVER_PAGE_SIZE}
            disabled={movies.isFetching}
            onPageChange={onPageChange}
          />
        </>
      )}
    </div>
  )
}

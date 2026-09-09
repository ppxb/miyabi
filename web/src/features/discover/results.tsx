import type { DiscoverMovie } from '@/api/discover'
import { EmptyState } from '@/components/empty-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGrid, MovieGridSkeleton } from '@/components/movie'
import { Button } from '@/components/ui/button'
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
    return (
      <EmptyState
        emoji="Ò︵Ó"
        title="数据加载失败"
        actions={
          <Button type="button" variant="outline" size="sm" onClick={onRetry}>
            重试
          </Button>
        }
      />
    )
  }
  if (loading || !movies) return <MovieGridSkeleton count={DISCOVER_PAGE_SIZE} />
  if (movies.length === 0 && page === 1) {
    return <EmptyState emoji="(･o･;)" title={searching ? '没有搜索结果' : '暂无内容'} />
  }
  return (
    <>
      <MovieGrid movies={movies} />
      <ListPagination
        page={page}
        hasMore={movies.length === DISCOVER_PAGE_SIZE}
        disabled={fetching}
        onPageChange={onPageChange}
      />
    </>
  )
}

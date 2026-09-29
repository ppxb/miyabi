import { Link } from '@tanstack/react-router'

import { LIBRARY_PAGE_SIZE, useLibraryMovies } from '@/api/library'
import { isScanTask, isTaskActive, useTasks } from '@/api/tasks'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState, InlineError } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { LibraryScanButton } from '@/features/library/scan-button'
import { useTaskConnection } from '@/features/tasks/task-events'
import { sameSource } from '@/lib/source'

export function LibraryPage({
  page,
  onPageChange
}: {
  page: number
  onPageChange: (page: number) => void
}) {
  const library = useLibraryMovies(page)
  const tasks = useTasks()
  const connection = useTaskConnection()
  const source = library.data?.source
  const latest = tasks.data?.filter(isScanTask).find(task => sameSource(task.source, source))
  const scanning = latest !== undefined && isTaskActive(latest)

  return (
    <AppPage>
      <PageHeader title="媒体库" description="来自 115 网盘的影片索引" inlineActions>
        <LibraryScanButton
          loading={library.isPending}
          available={!!source}
          scanning={scanning}
          connected={connection.status === 'connected'}
          failed={latest?.status === 'failed'}
          onStarted={() => onPageChange(1)}
        />
        {library.isSuccess && !source ? (
          <Button asChild>
            <Link to="/settings">挂载媒体目录</Link>
          </Button>
        ) : null}
      </PageHeader>

      {tasks.isError ? (
        <InlineError
          onRetry={connection.reconnect}
          retrying={connection.status === 'connecting'}
          retryLabel="重新连接"
        >
          暂时无法获取任务状态，请检查后端服务后重试。
        </InlineError>
      ) : null}

      {library.isPending ? (
        <MovieGridSkeleton count={LIBRARY_PAGE_SIZE} compact />
      ) : library.isError ? (
        <ErrorState
          message="无法读取媒体库，请检查后端服务后重试"
          onRetry={() => void library.refetch()}
          retrying={library.isFetching}
        />
      ) : (
        <>
          {library.data.movies.length > 0 ? (
            <MovieGridLayout>
              {library.data.movies.map(movie => (
                <LibraryMovieCard key={movie.id} movie={movie} />
              ))}
            </MovieGridLayout>
          ) : (
            <EmptyState
              className="min-h-0 flex-1 py-12"
              title={
                !source
                  ? '登录 115 并挂载媒体目录后，将自动扫描入库'
                  : scanning
                    ? '正在扫描，识别到的影片会陆续显示'
                    : '未识别到影片'
              }
            />
          )}
          <ListPagination
            page={page}
            totalPages={Math.max(1, Math.ceil(library.data.total / LIBRARY_PAGE_SIZE))}
            hasMore={library.data.has_more}
            disabled={library.isFetching}
            onPageChange={onPageChange}
          />
        </>
      )}
    </AppPage>
  )
}

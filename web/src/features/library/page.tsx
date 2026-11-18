import { lazy, Suspense, useState } from 'react'

import { LIBRARY_PAGE_SIZE, useLibraryMovies } from '@/api/library'
import { isScanTask, isTaskActive, useTasks } from '@/api/tasks'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState, InlineError } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { ContinueWatching } from '@/features/library/continue-watching'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { LibraryProgress } from '@/features/library/library-progress'
import { TopRatedShelf } from '@/features/library/top-rated-shelf'
import { useTaskConnection } from '@/features/tasks/task-events'

const LibraryPlayerDialog = lazy(() =>
  import('@/features/library/player-dialog').then(module => ({
    default: module.LibraryPlayerDialog
  }))
)

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
  const [playingId, setPlayingId] = useState<number | null>(null)
  const source = library.data?.source
  const libraryTasks = tasks.data?.filter(isScanTask)
  const latestScan = libraryTasks?.find(task => !task.movie_id && !task.offline_task_id)
  const scanning = latestScan !== undefined && isTaskActive(latestScan)
  // Show progress for any active library task, including a single-movie rescrape.
  const progressTask = libraryTasks?.find(isTaskActive)
  const processingMovies = new Set(libraryTasks?.filter(isTaskActive).map(task => task.movie_id))

  return (
    <AppPage>
      <PageHeader title="媒体库" description="最新、高分或分类影片" inlineActions>
        <LibraryProgress task={progressTask} />
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
          <ContinueWatching onPlay={setPlayingId} />
          <TopRatedShelf onPlay={setPlayingId} />
          {library.data.movies.length > 0 ? (
            <MovieGridLayout>
              {library.data.movies.map(movie => (
                <LibraryMovieCard
                  key={movie.id}
                  movie={movie}
                  busy={scanning || processingMovies.has(movie.id)}
                  onPlay={() => setPlayingId(movie.id)}
                />
              ))}
            </MovieGridLayout>
          ) : (
            <EmptyState
              className="min-h-0 flex-1 py-12"
              title={
                !source
                  ? '在设置 → 源管理中添加并登录存储源后，扫描入库'
                  : scanning
                    ? connection.status === 'connected'
                      ? `${latestScan?.rebuild ? '正在重建' : '正在同步'}，识别到的影片会陆续显示`
                      : '连接中，等待更新媒体库进度'
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
      {playingId !== null ? (
        <Suspense fallback={null}>
          <LibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
        </Suspense>
      ) : null}
    </AppPage>
  )
}

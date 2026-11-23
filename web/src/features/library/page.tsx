import { useState } from 'react'

import { useLibraryMovies, useRecentMovies, useTopRatedMovies } from '@/api/library'
import { isTaskActive } from '@/api/tasks'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState, InlineError } from '@/components/error-state'
import { PageHeader } from '@/components/page-header'
import { ContinueWatching } from '@/features/library/continue-watching'
import { LibraryProgress } from '@/features/library/library-progress'
import { MovieShelfSection } from '@/features/library/movie-shelf-section'
import { LazyLibraryPlayerDialog } from '@/features/library/player-dialog-lazy'
import { useTaskConnection } from '@/features/tasks/task-events'
import { useActiveScan } from '@/features/tasks/use-active-scan'

export function LibraryPage() {
  const library = useLibraryMovies(1)
  const recent = useRecentMovies()
  const topRated = useTopRatedMovies(12)
  // `active` covers any active library task, including a single-movie rescrape.
  const { tasks, scans, active: progressTask } = useActiveScan()
  const connection = useTaskConnection()
  const [playingId, setPlayingId] = useState<number | null>(null)
  const source = library.data?.source
  const latestScan = scans?.find(task => !task.movie_id && !task.offline_task_id)
  const scanning = latestScan !== undefined && isTaskActive(latestScan)

  return (
    <AppPage>
      <PageHeader title="媒体库" description="最近加入、正在观看与高分影片" inlineActions>
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

      {library.isError ? (
        <ErrorState
          message="无法读取媒体库，请检查后端服务后重试"
          onRetry={() => void library.refetch()}
          retrying={library.isFetching}
        />
      ) : (
        <>
          <ContinueWatching onPlay={setPlayingId} />
          <MovieShelfSection
            title="最近加入"
            movies={recent.data?.movies ?? []}
            onPlay={setPlayingId}
          />
          <MovieShelfSection title="高分影片" movies={topRated.data ?? []} onPlay={setPlayingId} />
          {!library.isPending && library.data.total === 0 ? (
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
          ) : null}
        </>
      )}

      <LazyLibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
    </AppPage>
  )
}

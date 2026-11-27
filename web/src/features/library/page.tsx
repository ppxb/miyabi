import { useState } from 'react'

import { useLibraryMovies, useRecentMovies, useTopRatedMovies } from '@/api/library'
import { isTaskActive, useTasks, type LibrarySource } from '@/api/tasks'
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
  const [playingId, setPlayingId] = useState<number | null>(null)

  return (
    <AppPage>
      <PageHeader title="媒体库" description="最近加入、正在观看与高分影片" inlineActions>
        <LibraryProgress />
      </PageHeader>

      {library.isError ? null : <LibraryTaskError />}

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
          {!library.isPending && library.data?.total === 0 ? (
            <LibraryEmptyState source={library.data?.source} />
          ) : null}
        </>
      )}

      <LazyLibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
    </AppPage>
  )
}

// Task state lives in these small components so the page and its movie cards
// do not re-render on every 300ms task snapshot during a scan.
function LibraryTaskError() {
  const tasks = useTasks()

  if (!tasks.isError) return null
  return <InlineError>暂时无法获取任务状态，请检查后端服务后重试。</InlineError>
}

function LibraryEmptyState({ source }: { source?: LibrarySource }) {
  const { scans } = useActiveScan()
  const connection = useTaskConnection()
  const latestScan = scans?.find(task => !task.movie_id && !task.offline_task_id)
  const scanning = latestScan !== undefined && isTaskActive(latestScan)

  return (
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
  )
}

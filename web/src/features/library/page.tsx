import { Link } from '@tanstack/react-router'
import { LoaderCircleIcon, RefreshCwIcon, ScanLineIcon } from 'lucide-react'

import { useLibraryMovies, useStartLibraryScan } from '@/api/library'
import { isTaskActive, useTasks } from '@/api/tasks'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { LibraryFilesDialog } from '@/features/library/files-dialog'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { useTaskConnection } from '@/features/tasks/task-events'

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
  const startScan = useStartLibraryScan()
  const source = library.data?.source
  const latest = tasks.data?.find(
    task =>
      source !== undefined &&
      task.source.account_id === source.account_id &&
      task.source.directory.id === source.directory.id
  )
  const scanning = latest !== undefined && isTaskActive(latest)
  const processing = scanning && connection.status === 'connected'
  const scanLabel = scanning
    ? processing
      ? '正在处理'
      : '等待同步'
    : latest?.status === 'failed'
      ? '重新扫描'
      : '扫描媒体库'

  return (
    <AppPage>
      <PageHeader title="媒体库" description="来自 115 网盘的影片索引" inlineActions>
        {library.isPending ? (
          <Skeleton className="h-9 w-9 rounded-4xl sm:w-30" />
        ) : source ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                className="w-9 cursor-pointer px-0 sm:w-auto sm:px-3"
                disabled={scanning || startScan.isPending}
                onClick={() => startScan.mutate(undefined, { onSuccess: () => onPageChange(1) })}
              >
                {processing || startScan.isPending ? (
                  <LoaderCircleIcon className="size-4 animate-spin" />
                ) : scanning ? (
                  <RefreshCwIcon className="size-4" />
                ) : (
                  <ScanLineIcon className="size-4" />
                )}
                <span className="hidden sm:inline">{scanLabel}</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent className="sm:hidden">{scanLabel}</TooltipContent>
          </Tooltip>
        ) : library.isSuccess ? (
          <Button asChild>
            <Link to="/settings">挂载媒体目录</Link>
          </Button>
        ) : null}
      </PageHeader>

      {tasks.isError ? (
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm text-muted-foreground">
            暂时无法获取任务状态，请检查后端服务后重试。
          </p>
          <Button
            variant="link"
            size="xs"
            disabled={connection.status === 'connecting'}
            onClick={connection.reconnect}
          >
            重新连接
          </Button>
        </div>
      ) : null}

      {library.isPending ? (
        <MovieGridSkeleton />
      ) : library.isError ? (
        <EmptyState
          emoji="(･o･;)"
          title="无法读取媒体库，请检查后端服务后重试"
          actions={
            <Button variant="outline" onClick={() => void library.refetch()}>
              重试
            </Button>
          }
        />
      ) : (
        <>
          {source ? (
            <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
              <div className="min-w-0 flex-1 space-y-1">
                <p className="text-sm">发现 {library.data.total} 部影片</p>
              </div>
              {library.data.unmatched_files > 0 ? (
                <LibraryFilesDialog>
                  <Button variant="outline" size="sm">
                    查看 {library.data.unmatched_files} 个未识别文件
                  </Button>
                </LibraryFilesDialog>
              ) : null}
            </div>
          ) : null}

          {library.data.movies.length > 0 ? (
            <>
              <MovieGridLayout>
                {library.data.movies.map(movie => (
                  <LibraryMovieCard key={movie.id} movie={movie} />
                ))}
              </MovieGridLayout>
            </>
          ) : (
            <EmptyState
              className="min-h-0 flex-1 py-12"
              emoji="(˙ᯅ˙)"
              title={
                !source
                  ? '登录 115 并挂载媒体目录后，将自动扫描入库'
                  : scanning
                    ? '正在扫描，识别到的影片会陆续显示'
                    : '未识别到影片'
              }
            />
          )}
          {page > 1 || library.data.has_more ? (
            <ListPagination
              page={page}
              hasMore={library.data.has_more}
              disabled={library.isFetching}
              onPageChange={onPageChange}
            />
          ) : null}
        </>
      )}
    </AppPage>
  )
}

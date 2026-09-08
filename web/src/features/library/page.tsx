import { Link } from '@tanstack/react-router'
import { LoaderCircleIcon, ScanLineIcon } from 'lucide-react'

import { ApiError } from '@/api/client'
import { useLibraryMovies, useStartLibraryScan } from '@/api/library'
import { isTaskActive, useTasks } from '@/api/tasks'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { LibraryFilesDialog } from '@/features/library/files-dialog'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { ScanProgressView } from '@/features/tasks/scan-progress'

export function LibraryPage({
  page,
  onPageChange
}: {
  page: number
  onPageChange: (page: number) => void
}) {
  const library = useLibraryMovies(page)
  const tasks = useTasks()
  const startScan = useStartLibraryScan()
  const source = library.data?.source
  const latest = tasks.data?.find(
    task =>
      source !== undefined &&
      task.source.account_id === source.account_id &&
      task.source.directory.id === source.directory.id
  )
  const scanning = latest !== undefined && isTaskActive(latest)
  const scanLabel = scanning ? '正在处理' : latest?.status === 'failed' ? '重新扫描' : '扫描媒体库'

  return (
    <AppPage>
      <PageHeader title="媒体库" description="来自 115 网盘的影片索引" inlineActions>
        {latest ? (
          <div className="max-w-full rounded-lg bg-muted px-1.5 sm:w-60 sm:px-3 sm:py-2">
            <ScanProgressView task={latest} compactOnMobile />
          </div>
        ) : null}
        {source ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                className="w-9 px-0 sm:w-auto sm:px-3"

                disabled={scanning || startScan.isPending}
                onClick={() => startScan.mutate(undefined, { onSuccess: () => onPageChange(1) })}
              >
                {scanning || startScan.isPending ? (
                  <LoaderCircleIcon className="size-4 animate-spin" />
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

      {startScan.isError ? (
        <p className="text-sm text-destructive">
          {startScan.error instanceof ApiError && startScan.error.status === 401
            ? '115 登录已失效，请前往设置重新登录。'
            : startScan.error instanceof ApiError && startScan.error.status === 400
              ? startScan.error.message
              : '无法创建扫描任务，请检查后端服务和 115 连接后重试。'}{' '}
          <Link to="/settings" className="underline underline-offset-4">
            前往设置
          </Link>
        </p>
      ) : null}

      {tasks.isError ? (
        <p className="text-sm text-muted-foreground">
          暂时无法获取任务状态，实时连接恢复后会自动同步。
        </p>
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
                  ? '登录 115 并挂载媒体目录后，即可扫描入库'
                  : scanning
                    ? '正在扫描，识别到的影片会陆续显示'
                    : '尚未识别到影片，可扫描媒体目录或整理文件名后重试'
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

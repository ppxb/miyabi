import { useEffect, useState } from 'react'

import { HISTORY_PAGE_SIZE, useClearHistory, useHistory } from '@/api/history'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { historyToCard } from '@/features/library/history-card'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { LazyLibraryPlayerDialog } from '@/features/library/player-dialog-lazy'
import { TrashIcon } from 'lucide-react'

export function HistoryPage({
  page,
  onPageChange
}: {
  page: number
  onPageChange: (page: number, options?: { replace?: boolean }) => void
}) {
  const history = useHistory(page)
  const clear = useClearHistory()
  const [confirming, setConfirming] = useState(false)
  const [playingId, setPlayingId] = useState<number | null>(null)
  const totalPages = Math.max(1, Math.ceil((history.data?.total ?? 0) / HISTORY_PAGE_SIZE))
  const dataPage = history.data?.page
  // Only the current page's own result may trigger a redirect; placeholder
  // data from the previous page can carry an outdated total.
  const pageOutOfRange = dataPage === page && page > totalPages
  // keepPreviousData keeps the out-of-range page while the redirect target
  // loads; keep showing the redirect state instead of an empty history.
  const redirecting =
    pageOutOfRange || (history.isPlaceholderData && dataPage !== undefined && dataPage > totalPages)

  useEffect(() => {
    if (pageOutOfRange) onPageChange(totalPages, { replace: true })
  }, [pageOutOfRange, totalPages, onPageChange])

  return (
    <AppPage>
      <PageHeader title="观看历史" description="继续观看与最近播放" inlineActions>
        {history.data && history.data.total > 0 && (
          <Button type="button" variant="destructive" onClick={() => setConfirming(true)}>
            <TrashIcon data-icon="inline-start" />
            清空历史
          </Button>
        )}
      </PageHeader>

      {history.isPending ? (
        <MovieGridSkeleton count={HISTORY_PAGE_SIZE} />
      ) : history.isError ? (
        <ErrorState
          message="无法读取观看历史，请检查后端服务后重试"
          onRetry={() => void history.refetch()}
          retrying={history.isFetching}
        />
      ) : redirecting ? (
        <EmptyState className="min-h-0 flex-1 py-12" title="正在返回最后一页…" />
      ) : history.data.items.length === 0 ? (
        <EmptyState className="min-h-0 flex-1 py-12" title="还没有观看记录" />
      ) : (
        <>
          <MovieGridLayout>
            {history.data.items.map(item => {
              const { movie, progress } = historyToCard(item)
              return (
                <LibraryMovieCard
                  key={item.movie_id}
                  movie={movie}
                  progress={progress}
                  onPlay={() => setPlayingId(item.movie_id)}
                />
              )
            })}
          </MovieGridLayout>
          <ListPagination
            page={page}
            totalPages={totalPages}
            hasMore={history.data.has_more}
            disabled={history.isFetching}
            onPageChange={onPageChange}
          />
        </>
      )}

      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>清空观看历史</DialogTitle>
            <DialogDescription>将删除所有影片的续播进度，无法撤销。</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setConfirming(false)}>
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={clear.isPending}
              onClick={() =>
                clear.mutate(undefined, {
                  onSuccess: () => setConfirming(false)
                })
              }
            >
              清空
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <LazyLibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
    </AppPage>
  )
}

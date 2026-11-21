import { useState } from 'react'

import {
  HISTORY_PAGE_SIZE,
  useClearHistory,
  useHistory,
  type WatchHistoryItem
} from '@/api/history'
import type { LibraryMovie } from '@/api/library'
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
import { LibraryMovieCard } from '@/features/library/movie-card'
import { LazyLibraryPlayerDialog } from '@/features/library/player-dialog-lazy'

export function HistoryPage({
  page,
  onPageChange
}: {
  page: number
  onPageChange: (page: number) => void
}) {
  const history = useHistory(page)
  const clear = useClearHistory()
  const [confirming, setConfirming] = useState(false)
  const [playingId, setPlayingId] = useState<number | null>(null)

  return (
    <AppPage>
      <PageHeader title="观看历史" description="继续观看与最近播放" inlineActions>
        {history.data && history.data.total > 0 ? (
          <Button type="button" variant="outline" size="sm" onClick={() => setConfirming(true)}>
            清空历史
          </Button>
        ) : null}
      </PageHeader>

      {history.isPending ? (
        <MovieGridSkeleton count={HISTORY_PAGE_SIZE} compact />
      ) : history.isError ? (
        <ErrorState
          message="无法读取观看历史，请检查后端服务后重试"
          onRetry={() => void history.refetch()}
          retrying={history.isFetching}
        />
      ) : history.data.items.length === 0 ? (
        <EmptyState className="min-h-0 flex-1 py-12" title="还没有观看记录，去媒体库播放一部吧" />
      ) : (
        <>
          <MovieGridLayout>
            {history.data.items.map(item => (
              <HistoryCard
                key={item.movie_id}
                item={item}
                onPlay={() => setPlayingId(item.movie_id)}
              />
            ))}
          </MovieGridLayout>
          <ListPagination
            page={page}
            totalPages={Math.max(1, Math.ceil(history.data.total / HISTORY_PAGE_SIZE))}
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

function HistoryCard({ item, onPlay }: { item: WatchHistoryItem; onPlay: () => void }) {
  const percent =
    item.duration > 0
      ? Math.max(0, Math.min(100, Math.round((item.position / item.duration) * 100)))
      : 0
  const movie: LibraryMovie = {
    id: item.movie_id,
    code: item.code,
    title: item.title,
    cover: item.cover,
    poster: item.poster,
    duration: item.duration,
    rating: item.rating ?? 0,
    actors: item.actors ?? [],
    source: item.source,
    tags: [],
    scrape_status: 'done'
  }
  return <LibraryMovieCard movie={movie} progress={percent} onPlay={onPlay} />
}

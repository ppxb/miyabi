import { useState } from 'react'

import { HISTORY_PAGE_SIZE, useClearHistory, useHistory } from '@/api/history'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieCard } from '@/components/movie'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { LibraryPlayerDialog } from '@/features/library/player-dialog'

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

      {playingId !== null ? (
        <LibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
      ) : null}
    </AppPage>
  )
}

function HistoryCard({
  item,
  onPlay
}: {
  item: {
    movie_id: number
    code: string
    title: string
    cover?: string
    poster?: string
    position: number
    duration: number
  }
  onPlay: () => void
}) {
  const percent =
    item.duration > 0
      ? Math.max(0, Math.min(100, Math.round((item.position / item.duration) * 100)))
      : 0
  return (
    <div
      role="button"
      tabIndex={0}
      className="block h-full min-w-0 cursor-pointer rounded-2xl outline-ring"
      onClick={onPlay}
      onKeyDown={event => {
        if (event.key !== 'Enter' && event.key !== ' ') return
        event.preventDefault()
        if (!event.repeat) onPlay()
      }}
    >
      <MovieCard
        movie={{ code: item.code, title: item.title, cover: item.cover ?? item.poster }}
        titleTooltip={false}
        hideContent
        state={<Badge variant="secondary">{percent}%</Badge>}
        coverOverlay={
          <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent p-3 pt-8">
            <h3 className="line-clamp-2 text-sm leading-5 font-semibold text-white">
              {item.title || item.code}
            </h3>
            <div className="mt-1 h-1 overflow-hidden rounded-full bg-white/25">
              <div className="h-full bg-success" style={{ width: `${percent}%` }} />
            </div>
          </div>
        }
      />
    </div>
  )
}

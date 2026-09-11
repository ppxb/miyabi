import { Link } from '@tanstack/react-router'
import { LoaderCircleIcon, Trash2Icon, XIcon } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'

import {
  useRemoveWatchHistory,
  useWatchHistory,
  WATCH_HISTORY_PAGE_SIZE
} from '@/api/watch-history'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState, InlineError } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout, MovieGridSkeleton } from '@/components/movie'
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
import { HistoryCard } from './history-card'

type HistoryPageProps = { page: number; onPageChange: (page: number) => void }

export function WatchHistoryPage(props: HistoryPageProps) {
  const history = useWatchHistory(props.page)
  const source = history.data?.source
  return (
    <HistoryContent
      key={JSON.stringify([props.page, source?.account_id, source?.directory.id])}
      {...props}
      history={history}
    />
  )
}

function HistoryContent({
  page,
  onPageChange,
  history
}: HistoryPageProps & {
  history: ReturnType<typeof useWatchHistory>
}) {
  const remove = useRemoveWatchHistory()
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<number>>(() => new Set())
  const [clearMode, setClearMode] = useState<'all' | 'selected' | null>(null)
  const items = history.data?.items ?? []
  const source = history.data?.source
  const total = history.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / WATCH_HISTORY_PAGE_SIZE))
  const selectedIDs = items.filter(item => selected.has(item.id)).map(item => item.id)
  const allSelected = items.length > 0 && selectedIDs.length === items.length

  useEffect(() => {
    if (history.isSuccess && !history.isFetching && page > pageCount) onPageChange(pageCount)
  }, [history.isSuccess, history.isFetching, page, pageCount, onPageChange])

  function leaveSelection() {
    setSelecting(false)
    setSelected(new Set())
  }

  function openClearDialog(mode: 'all' | 'selected') {
    remove.reset()
    setClearMode(mode)
  }

  function clearHistory() {
    if (!source || !clearMode || (clearMode === 'selected' && selectedIDs.length === 0)) return
    remove.mutate(
      clearMode === 'all'
        ? { type: 'all', source }
        : { type: 'selected', source, ids: selectedIDs },
      {
        onSuccess: () => {
          setClearMode(null)
          leaveSelection()
          toast.success(clearMode === 'all' ? '观看历史已清除' : '已清除选中的观看记录')
        }
      }
    )
  }

  return (
    <AppPage>
      <PageHeader title="观看历史" description="回顾最近观看的影片">
        {selecting ? (
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={items.length === 0 || remove.isPending}
              onClick={() =>
                setSelected(allSelected ? new Set() : new Set(items.map(item => item.id)))
              }
            >
              {allSelected ? '取消全选' : '全选本页'}
            </Button>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              disabled={selectedIDs.length === 0 || remove.isPending}
              onClick={() => openClearDialog('selected')}
            >
              <Trash2Icon />
              清除选中{selectedIDs.length > 0 ? ` (${selectedIDs.length})` : ''}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={remove.isPending}
              onClick={leaveSelection}
            >
              <XIcon />
              退出
            </Button>
          </>
        ) : (
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={total === 0}
              onClick={() => setSelecting(true)}
            >
              选择清除
            </Button>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              disabled={total === 0}
              onClick={() => openClearDialog('all')}
            >
              <Trash2Icon />
              清除全部
            </Button>
          </>
        )}
      </PageHeader>

      {history.isPending ? (
        <MovieGridSkeleton />
      ) : history.isError ? (
        <ErrorState
          message="观看历史加载失败"
          onRetry={() => void history.refetch()}
          retrying={history.isFetching}
        />
      ) : items.length === 0 ? (
        <EmptyState
          className="min-h-0 flex-1"
          title={source ? '还没有观看记录' : '挂载媒体目录后查看观看历史'}
          actions={
            !source ? (
              <Button asChild>
                <Link to="/settings">挂载媒体目录</Link>
              </Button>
            ) : undefined
          }
        />
      ) : (
        <MovieGridLayout>
          {items.map(item => (
            <HistoryCard
              key={item.id}
              item={item}
              selecting={selecting}
              selected={selecting && selected.has(item.id)}
              disabled={remove.isPending}
              onSelect={() =>
                setSelected(current => {
                  const next = new Set(current)
                  if (next.has(item.id)) next.delete(item.id)
                  else next.add(item.id)
                  return next
                })
              }
            />
          ))}
        </MovieGridLayout>
      )}

      {pageCount > 1 ? (
        <ListPagination
          page={page}
          hasMore={page < pageCount}
          disabled={history.isFetching || remove.isPending}
          onPageChange={onPageChange}
        />
      ) : null}

      <Dialog
        open={clearMode !== null}
        onOpenChange={open => {
          if (!open && !remove.isPending) setClearMode(null)
        }}
      >
        <DialogContent showCloseButton={!remove.isPending}>
          <DialogHeader>
            <DialogTitle>清除观看历史</DialogTitle>
            <DialogDescription>
              {clearMode === 'all'
                ? '清除当前媒体目录的全部观看记录和播放进度？'
                : `清除选中的 ${selectedIDs.length} 条观看记录和播放进度？`}
              影片文件和已观看状态会保留。
            </DialogDescription>
          </DialogHeader>
          {remove.error ? <InlineError>{remove.error.message}</InlineError> : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={remove.isPending}
              onClick={() => setClearMode(null)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={
                remove.isPending ||
                !source ||
                (clearMode === 'selected' && selectedIDs.length === 0)
              }
              onClick={clearHistory}
            >
              {remove.isPending ? <LoaderCircleIcon className="animate-spin" /> : <Trash2Icon />}
              确认清除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </AppPage>
  )
}

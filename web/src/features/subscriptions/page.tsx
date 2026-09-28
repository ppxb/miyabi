import { CloudDownloadIcon, LoaderCircleIcon, XIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import {
  SUBSCRIPTION_PAGE_SIZE,
  useActorFeed,
  useBatchEnqueueSubscriptions,
  useSubscriptions,
  useSubscriptionTargets
} from '@/api/subscriptions'
import { AppPage } from '@/components/app-page'
import { InlineError } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
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
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ActorSubscriptions } from './actor-subscriptions'
import { MovieSubscriptions } from './movie-subscriptions'
import { isPendingSubscription } from './subscription-card'

type SubscriptionsView = 'movies' | 'actors'

export function SubscriptionsPage() {
  const [view, setView] = useState<SubscriptionsView>('movies')
  const [selectedActorID, setSelectedActorID] = useState<number | null>(null)
  const [page, setPage] = useState(1)

  const targets = useSubscriptionTargets()
  const movies = useSubscriptions('movie', page, SUBSCRIPTION_PAGE_SIZE, view === 'movies')
  const actors = useSubscriptions('actor', 1, 100, view === 'actors')
  const feed = useActorFeed(selectedActorID, page, SUBSCRIPTION_PAGE_SIZE, view === 'actors')
  const batch = useBatchEnqueueSubscriptions()

  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<number>>(() => new Set())
  const [confirmMode, setConfirmMode] = useState<'all' | 'selected' | null>(null)

  const allSpawnedTargets = (targets.data?.list ?? []).filter(
    item => item.kind === 'movie' && item.origin_id != null
  )

  const currentItems = view === 'movies' ? (movies.data ?? []) : (feed.data ?? [])
  const currentQuery = view === 'movies' ? movies : feed

  const pending = currentItems.filter(isPendingSubscription)
  const selectedIDs = pending.filter(item => selected.has(item.id)).map(item => item.id)
  const allSelected = pending.length > 0 && selectedIDs.length === pending.length

  const totalMovieWaiting = (targets.data?.list ?? []).filter(
    item => item.kind === 'movie' && item.status === 'waiting'
  ).length
  const canBatchAll = view === 'movies' ? totalMovieWaiting > 0 : pending.length > 0
  const pendingCount =
    view === 'movies' && confirmMode === 'all' ? totalMovieWaiting : pending.length

  function leaveSelection() {
    setSelecting(false)
    setSelected(new Set())
  }

  function handleTabChange(nextView: SubscriptionsView) {
    setView(nextView)
    setPage(1)
    leaveSelection()
    setSelectedActorID(null)
  }

  function handleSelectActor(id: number | null) {
    setSelectedActorID(id)
    setPage(1)
    leaveSelection()
  }

  function handleSelect(id: number) {
    setSelected(current => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function handleEnqueue() {
    if (!confirmMode) return
    const isAll = confirmMode === 'all'
    const count = isAll ? pendingCount : selectedIDs.length
    if (count === 0) return

    const payload =
      isAll && view === 'movies'
        ? { all: true }
        : { ids: isAll ? pending.map(item => item.id) : selectedIDs }

    batch.mutate(payload, {
      onSuccess: () => {
        setConfirmMode(null)
        leaveSelection()
        toast.success(`已创建 ${count} 部影片的入库任务`, {
          description: '按磁力偏好逐部加入 115，进度见任务通知。'
        })
      }
    })
  }

  return (
    <AppPage>
      <PageHeader title="订阅" description="追踪影片磁力与演员新作，出现资源后加入 115">
        {selecting ? (
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={pendingCount === 0 || batch.isPending}
              onClick={() =>
                setSelected(allSelected ? new Set() : new Set(pending.map(item => item.id)))
              }
            >
              {allSelected ? '取消全选' : '全选待入库'}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={selectedIDs.length === 0 || batch.isPending}
              onClick={() => {
                batch.reset()
                setConfirmMode('selected')
              }}
            >
              <CloudDownloadIcon />
              入库选中{selectedIDs.length > 0 ? ` (${selectedIDs.length})` : ''}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={batch.isPending}
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
              disabled={pending.length === 0}
              onClick={() => setSelecting(true)}
            >
              选择入库
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={!canBatchAll}
              onClick={() => {
                batch.reset()
                setConfirmMode('all')
              }}
            >
              <CloudDownloadIcon />
              一键入库
            </Button>
          </>
        )}
      </PageHeader>

      <div className="flex min-w-0 flex-1 flex-col gap-6">
        <Tabs value={view} onValueChange={value => handleTabChange(value as SubscriptionsView)}>
          <TabsList>
            <TabsTrigger value="movies">影片订阅</TabsTrigger>
            <TabsTrigger value="actors">演员订阅</TabsTrigger>
          </TabsList>
        </Tabs>

        {view === 'actors' ? (
          <ActorSubscriptions
            actors={actors}
            selectedID={selectedActorID}
            onSelectID={handleSelectActor}
            allSpawned={allSpawnedTargets}
            displayItems={currentItems}
            displayQuery={currentQuery}
            selecting={selecting}
            selected={selected}
            onSelect={handleSelect}
            disabled={batch.isPending}
          />
        ) : (
          <MovieSubscriptions
            items={currentItems}
            isPending={movies.isPending}
            isError={movies.isError}
            isFetching={movies.isFetching}
            onRetry={() => void movies.refetch()}
            emptyTitle="还没有订阅影片，在「即将发行」卡片右上角或详情页点击铃铛订阅"
            selecting={selecting}
            selected={selected}
            onSelect={handleSelect}
            disabled={batch.isPending}
          />
        )}

        <ListPagination
          page={page}
          hasMore={currentItems.length === SUBSCRIPTION_PAGE_SIZE}
          disabled={currentQuery.isFetching}
          onPageChange={setPage}
        />
      </div>

      <Dialog
        open={confirmMode !== null}
        onOpenChange={open => {
          if (!open && !batch.isPending) setConfirmMode(null)
        }}
      >
        <DialogContent showCloseButton={!batch.isPending}>
          <DialogHeader>
            <DialogTitle>{confirmMode === 'all' ? '一键入库' : '入库选中影片'}</DialogTitle>
            <DialogDescription>
              {confirmMode === 'all'
                ? `为 ${pendingCount} 部等待磁力的影片创建入库任务？任务按磁力偏好逐部加入 115，每部之间随机间隔 1.5 到 3 秒；没有合适磁力的影片保持等待并开启自动入库。`
                : `为选中的 ${selectedIDs.length} 部影片创建入库任务？任务按磁力偏好逐部加入 115，每部之间随机间隔 1.5 到 3 秒。`}
            </DialogDescription>
          </DialogHeader>
          {batch.error ? <InlineError>{batch.error.message}</InlineError> : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={batch.isPending}
              onClick={() => setConfirmMode(null)}
            >
              取消
            </Button>
            <Button
              type="button"
              disabled={
                batch.isPending ||
                (confirmMode === 'selected' && selectedIDs.length === 0) ||
                (confirmMode === 'all' && pendingCount === 0)
              }
              onClick={handleEnqueue}
            >
              {batch.isPending ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <CloudDownloadIcon />
              )}
              确认入库
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </AppPage>
  )
}

import { useSubscriptions } from '@/api/subscriptions'
import type { SubscriptionItem } from '@/api/subscriptions'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ActorChip } from './actor-chip'
import { MovieSubscriptions } from './movie-subscriptions'

type ActorSubscriptionsProps = {
  actors: ReturnType<typeof useSubscriptions>
  selectedID: number | null
  onSelectID: (id: number | null) => void
  allSpawned: Array<{ origin_id?: number }>
  displayItems: SubscriptionItem[]
  displayQuery: {
    isPending: boolean
    isError: boolean
    isFetching: boolean
    refetch: () => void | Promise<unknown>
  }
  selecting?: boolean
  selected?: Set<number>
  onSelect?: (id: number) => void
  disabled?: boolean
}

// Subscribed actors above the grid of works they spawned. Selecting an actor
// narrows the grid to that actor's feed.
export function ActorSubscriptions({
  actors,
  selectedID,
  onSelectID,
  allSpawned,
  displayItems,
  displayQuery,
  selecting = false,
  selected = new Set(),
  onSelect,
  disabled = false
}: ActorSubscriptionsProps) {
  if (actors.isPending) {
    return (
      <div className="flex gap-3 overflow-hidden">
        {Array.from({ length: 3 }, (_, index) => (
          <Skeleton key={index} className="h-16 w-64 shrink-0 rounded-2xl" />
        ))}
      </div>
    )
  }
  if (actors.isError) {
    return (
      <ErrorState
        message="演员订阅加载失败"
        onRetry={() => void actors.refetch()}
        retrying={actors.isFetching}
      />
    )
  }
  if (!actors.data || actors.data.length === 0) {
    return <EmptyState title="还没有订阅演员，在演员作品页点击「订阅演员」" />
  }

  const counts = new Map<number, number>()
  for (const item of allSpawned) {
    if (item.origin_id != null) {
      counts.set(item.origin_id, (counts.get(item.origin_id) ?? 0) + 1)
    }
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-6">
      <div className="flex gap-3 overflow-x-auto pb-2">
        <Button
          type="button"
          variant={selectedID === null ? 'secondary' : 'outline'}
          className="h-auto shrink-0 rounded-2xl px-4 py-3"
          onClick={() => onSelectID(null)}
        >
          全部新作
          <Badge variant="outline">{allSpawned.length}</Badge>
        </Button>
        {actors.data.map(actor => (
          <ActorChip
            key={actor.id}
            actor={actor}
            count={counts.get(actor.id) ?? 0}
            selected={selectedID === actor.id}
            onSelect={() => onSelectID(selectedID === actor.id ? null : actor.id)}
            onRemoved={() => {
              if (selectedID === actor.id) onSelectID(null)
            }}
          />
        ))}
      </div>

      <MovieSubscriptions
        items={displayItems}
        isPending={displayQuery.isPending}
        isError={displayQuery.isError}
        isFetching={displayQuery.isFetching}
        onRetry={() => void displayQuery.refetch()}
        emptyTitle={selectedID === null ? '订阅的演员暂时没有新作' : '这位演员暂时没有新作'}
        selecting={selecting}
        selected={selected}
        onSelect={onSelect}
        disabled={disabled}
      />
    </div>
  )
}

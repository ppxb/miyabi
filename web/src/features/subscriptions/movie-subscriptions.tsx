import type { SubscriptionItem } from '@/api/subscriptions'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { MovieGridLayout, MovieGridSkeleton } from '@/components/movie'
import { SubscriptionCard } from './subscription-card'
import { isPendingSubscription } from './subscription-status'

export type MovieSubscriptionsProps = {
  items: SubscriptionItem[]
  isPending: boolean
  isError: boolean
  isFetching: boolean
  onRetry: () => void
  emptyTitle: string
  selecting?: boolean
  selected?: Set<number>
  onSelect?: (id: number) => void
  disabled?: boolean
}

export function MovieSubscriptions({
  items,
  isPending,
  isError,
  isFetching,
  onRetry,
  emptyTitle,
  selecting = false,
  selected = new Set(),
  onSelect,
  disabled = false
}: MovieSubscriptionsProps) {
  if (isPending) {
    return <MovieGridSkeleton withContent />
  }
  if (isError) {
    return <ErrorState message="订阅列表加载失败" onRetry={onRetry} retrying={isFetching} />
  }
  if (items.length === 0) {
    return <EmptyState title={emptyTitle} />
  }
  return (
    <MovieGridLayout>
      {items.map(item => (
        <SubscriptionCard
          key={item.id}
          item={item}
          selecting={selecting}
          selected={selecting && selected.has(item.id)}
          disabled={disabled || (selecting && !isPendingSubscription(item))}
          onSelect={() => onSelect?.(item.id)}
        />
      ))}
    </MovieGridLayout>
  )
}

import { Link } from '@tanstack/react-router'
import { CloudDownloadIcon, LoaderCircleIcon, Trash2Icon } from 'lucide-react'

import { useMovieState } from '@/api/movie-states'
import {
  type SubscriptionItem,
  useEnqueueSubscription,
  useRemoveSubscription
} from '@/api/subscriptions'
import { MovieCard } from '@/components/movie'
import { MovieStateBadge } from '@/components/movie/movie-badges'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'

const dateTimeFormat = new Intl.DateTimeFormat('zh-CN', {
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit'
})

const statusLabels: Record<SubscriptionItem['status'], string> = {
  waiting: '等待磁力',
  added: '已加入 115',
  stale: '长期无源',
  active: '生效中',
  paused: '已暂停'
}

export function isPendingSubscription(item: SubscriptionItem) {
  return item.status === 'waiting' || item.status === 'stale'
}

// One movie subscription in the grid. Selection mode mirrors the history page:
// the whole card toggles the checkbox and stops navigating.
export function SubscriptionCard({
  item,
  selecting,
  selected,
  disabled,
  onSelect
}: {
  item: SubscriptionItem
  selecting: boolean
  selected: boolean
  disabled: boolean
  onSelect: () => void
}) {
  const enqueue = useEnqueueSubscription()
  const remove = useRemoveSubscription()
  const state = useMovieState({ id: item.target_id, code: item.code })
  const busy = disabled || enqueue.isPending || remove.isPending
  const canEnqueue =
    isPendingSubscription(item) && !state.isPlaceholderData && state.state === 'not_in_library'

  const card = (
    <MovieCard
      movie={item}
      description={
        <div className="space-y-1">
          <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
            <span>{item.release_date ? `发行 ${item.release_date}` : '发行日期未知'}</span>
            <span className="tabular-nums">{describeSchedule(item)}</span>
          </div>
          {item.error ? (
            <p className="line-clamp-2 text-destructive" title={item.error}>
              {item.error}
            </p>
          ) : null}
        </div>
      }
      state={
        <>
          <MovieStateBadge movie={{ id: item.target_id, code: item.code }} hideViewed />
          <Badge variant={statusVariant(item.status)}>{statusLabels[item.status]}</Badge>
        </>
      }
    >
      {item.auto_download ? <Badge variant="outline">自动入库</Badge> : null}
      {item.origin_id ? <Badge variant="outline">演员新作</Badge> : null}
      {!selecting ? (
        <div className="flex w-full items-center justify-end gap-2">
          {canEnqueue ? (
            <Button
              type="button"
              variant="outline"
              size="xs"
              disabled={busy}
              onClick={event => {
                event.preventDefault()
                enqueue.mutate(item.id)
              }}
            >
              {enqueue.isPending ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <CloudDownloadIcon />
              )}
              入库
            </Button>
          ) : null}
          <Button
            type="button"
            variant="ghost"
            size="xs"
            disabled={busy}
            onClick={event => {
              event.preventDefault()
              remove.mutate(item)
            }}
          >
            {remove.isPending ? <LoaderCircleIcon className="animate-spin" /> : <Trash2Icon />}
            取消订阅
          </Button>
        </div>
      ) : null}
    </MovieCard>
  )

  return (
    <div className="relative h-full min-w-0">
      {selecting ? (
        <Button
          type="button"
          variant="ghost"
          className={cn(
            'block h-full w-full min-w-0 rounded-2xl p-0 text-left whitespace-normal hover:bg-transparent hover:text-current dark:hover:bg-transparent',
            selected && 'ring-2 ring-success'
          )}
          disabled={disabled}
          onClick={onSelect}
        >
          {card}
        </Button>
      ) : (
        <Link
          to="/discover/$movieId"
          search={previous => ({ main: previous.main || undefined })}
          params={{ movieId: item.target_id }}
          className="block h-full rounded-2xl outline-ring"
        >
          {card}
        </Link>
      )}
      {selecting ? (
        <Checkbox
          checked={selected}
          disabled={disabled}
          onCheckedChange={onSelect}
          className="absolute top-3 right-3 size-5 data-checked:border-success data-checked:bg-success data-checked:text-white"
        />
      ) : null}
    </div>
  )
}

function statusVariant(status: SubscriptionItem['status']) {
  if (status === 'added') return 'success' as const
  if (status === 'stale' || status === 'paused') return 'secondary' as const
  return 'default' as const
}

function describeSchedule(item: SubscriptionItem) {
  if (item.status === 'added') return '已自动加入 115'
  if (item.status === 'stale') return '发行 30 天后仍无磁力'
  if (item.hash && !item.auto_download) return '已有磁力，等待入库'
  if (item.next_check_at) {
    const next = new Date(item.next_check_at)
    if (next.getTime() <= Date.now()) return '即将检查'
    return `下次检查 ${dateTimeFormat.format(next)}`
  }
  return '等待检查'
}

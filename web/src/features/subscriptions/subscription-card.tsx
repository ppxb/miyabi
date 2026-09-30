import { LoaderCircleIcon, Trash2Icon } from 'lucide-react'

import { type SubscriptionItem, useRemoveSubscription } from '@/api/subscriptions'
import { MovieCard } from '@/components/movie'
import { MovieStateBadge } from '@/components/movie/movie-badges'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { MovieDetailLink } from '@/features/movie-detail/detail-link'
import { cn } from 'cn'

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

// Keep the card mounted when switching between browsing and selection.
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
  const remove = useRemoveSubscription()
  const busy = disabled || remove.isPending

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
      coverOverlay={
        !selecting ? (
          <div className="absolute top-2 right-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="secondary"
                  size="icon-sm"
                  disabled={busy}
                  onClick={event => {
                    event.preventDefault()
                    event.stopPropagation()
                    remove.mutate(item)
                  }}
                >
                  {remove.isPending ? (
                    <LoaderCircleIcon className="animate-spin" />
                  ) : (
                    <Trash2Icon />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent side="left">取消订阅</TooltipContent>
            </Tooltip>
          </div>
        ) : undefined
      }
    />
  )

  return (
    <div className="relative h-full min-w-0">
      <MovieDetailLink
        movieId={item.target_id}
        className={cn(
          'block h-full rounded-2xl outline-ring',
          selected && 'ring-2 ring-success',
          selecting && disabled && 'cursor-not-allowed opacity-50'
        )}
        role={selecting ? 'checkbox' : undefined}
        aria-checked={selecting ? selected : undefined}
        aria-disabled={selecting ? disabled : undefined}
        aria-haspopup={selecting ? false : 'dialog'}
        tabIndex={selecting && disabled ? -1 : undefined}
        onClick={event => {
          if (!selecting) return
          event.preventDefault()
          if (!disabled) onSelect()
        }}
        onAuxClick={event => {
          if (selecting) event.preventDefault()
        }}
        onKeyDown={event => {
          if (!selecting || event.key !== ' ') return
          event.preventDefault()
          if (!disabled && !event.repeat) onSelect()
        }}
      >
        {card}
      </MovieDetailLink>
      {selecting ? (
        <Checkbox
          checked={selected}
          disabled={disabled}
          onCheckedChange={onSelect}
          aria-label={`选择 ${item.code}`}
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

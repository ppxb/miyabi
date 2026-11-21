import { PauseIcon, PlayIcon, Trash2Icon, UserRoundIcon } from 'lucide-react'

import {
  type SubscriptionItem,
  useRemoveSubscription,
  useUpdateSubscription
} from '@/api/subscriptions'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

// One subscribed actor: avatar, name and new-work count as a toggle, with the
// auto-download switch and pause / remove controls beside it.
export function ActorChip({
  actor,
  count,
  selected,
  onSelect,
  onRemoved
}: {
  actor: SubscriptionItem
  count: number
  selected: boolean
  onSelect: () => void
  onRemoved: () => void
}) {
  const update = useUpdateSubscription()
  const remove = useRemoveSubscription()
  const paused = actor.status === 'paused'
  const busy = update.isPending || remove.isPending

  return (
    <div className="flex shrink-0 items-center gap-2 rounded-2xl border p-2">
      <Button
        type="button"
        variant={selected ? 'secondary' : 'ghost'}
        className="h-auto rounded-xl px-2 py-1"
        onClick={onSelect}
      >
        <div className="flex min-w-0 items-center gap-3">
          <Avatar key={actor.cover} size="lg">
            {actor.cover ? <AvatarImage src={actor.cover} alt={actor.title} /> : null}
            <AvatarFallback>
              <UserRoundIcon className="size-5" />
            </AvatarFallback>
          </Avatar>
          <div className="min-w-0 space-y-1 text-left">
            <div className="flex items-center gap-1.5">
              <span className="truncate text-sm font-semibold">{actor.title}</span>
              {paused ? <Badge variant="secondary">已暂停</Badge> : null}
              {actor.error ? <Badge variant="destructive">检查失败</Badge> : null}
            </div>
            <div className="truncate text-xs text-muted-foreground">新作 {count} 部</div>
          </div>
        </div>
      </Button>
      <Tooltip>
        <TooltipTrigger render={<span className="flex items-center" />}>
          <Switch
            size="sm"
            checked={actor.auto_download}
            disabled={busy}
            onCheckedChange={checked => update.mutate({ id: actor.id, auto_download: checked })}
          />
        </TooltipTrigger>
        <TooltipContent>{actor.auto_download ? '新作自动入库' : '新作仅提醒'}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              disabled={busy}
              onClick={() => update.mutate({ id: actor.id, status: paused ? 'active' : 'paused' })}
            />
          }
        >
          {paused ? <PlayIcon /> : <PauseIcon />}
        </TooltipTrigger>
        <TooltipContent>{paused ? '恢复检查' : '暂停检查'}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              disabled={busy}
              onClick={() => remove.mutate(actor, { onSuccess: onRemoved })}
            />
          }
        >
          <Trash2Icon />
        </TooltipTrigger>
        <TooltipContent>取消订阅</TooltipContent>
      </Tooltip>
    </div>
  )
}

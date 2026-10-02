import { BellPlusIcon, BellRingIcon, CircleCheckIcon, LoaderCircleIcon } from 'lucide-react'
import type { MouseEvent } from 'react'

import type { DiscoverMovie } from '@/api/discover'
import { useAddSubscription, useSubscription } from '@/api/subscriptions'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

// Shown on unreleased cards without a magnet; clicks must not open the detail dialog.
export function MovieSubscribeButton({ movie }: { movie: DiscoverMovie }) {
  const { subscription, isPending } = useSubscription('movie', movie.id)
  const add = useAddSubscription()
  const waiting = subscription?.status === 'waiting'
  const added = subscription?.status === 'added'
  const canSubscribe = !waiting && !added

  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault()
    event.stopPropagation()
    if (!canSubscribe || add.isPending) return
    add.mutate({ kind: 'movie', target_id: movie.id })
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant={canSubscribe ? 'outline' : 'default'}
          size="icon-sm"
          disabled={isPending || add.isPending}
          className={canSubscribe ? 'bg-background/85 backdrop-blur' : undefined}
          onClick={handleClick}
        >
          {add.isPending ? (
            <LoaderCircleIcon className="animate-spin" />
          ) : added ? (
            <CircleCheckIcon />
          ) : waiting ? (
            <BellRingIcon />
          ) : (
            <BellPlusIcon />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">
        {waiting
          ? '已订阅，将持续检查符合偏好的磁力'
          : added
            ? '已加入 115 离线下载'
            : subscription
              ? '重新订阅'
              : '订阅影片'}
      </TooltipContent>
    </Tooltip>
  )
}

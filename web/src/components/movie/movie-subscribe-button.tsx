import { BellPlusIcon, BellRingIcon, LoaderCircleIcon } from 'lucide-react'
import type { MouseEvent } from 'react'

import type { DiscoverMovie } from '@/api/discover'
import { useAddSubscription, useSubscription } from '@/api/subscriptions'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

// Shown on unreleased cards without a magnet; clicks must not open the detail dialog.
export function MovieSubscribeButton({ movie }: { movie: DiscoverMovie }) {
  const { subscription, isPending } = useSubscription('movie', movie.id)
  const add = useAddSubscription()
  const subscribed = subscription?.status === 'waiting' || subscription?.status === 'added'

  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    event.preventDefault()
    event.stopPropagation()
    if (subscribed || add.isPending) return
    add.mutate({ kind: 'movie', target_id: movie.id })
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant={subscribed ? 'default' : 'outline'}
          size="icon-sm"
          aria-label={subscribed ? '已订阅' : '订阅影片'}
          aria-pressed={subscribed}
          disabled={isPending || add.isPending}
          className={subscribed ? undefined : 'bg-background/85 backdrop-blur'}
          onClick={handleClick}
        >
          {add.isPending ? (
            <LoaderCircleIcon className="animate-spin" />
          ) : subscribed ? (
            <BellRingIcon />
          ) : (
            <BellPlusIcon />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">
        {subscribed ? '已订阅，出现磁力后将自动处理' : '订阅影片'}
      </TooltipContent>
    </Tooltip>
  )
}

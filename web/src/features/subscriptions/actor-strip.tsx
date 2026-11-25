import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'
import { UserRoundIcon } from 'lucide-react'
import { useEffect, useRef } from 'react'

import type { SubscriptionItem } from '@/api/subscriptions'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { cn } from 'cn'

// One row of followed actors. A vertical wheel scrolls it sideways until an
// edge, then the page takes over.
export function ActorStrip({
  actors,
  selectedID,
  onSelect
}: {
  actors: SubscriptionItem[]
  selectedID: number | null
  onSelect: (id: number | null) => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  useScrollFade(ref, { orientation: 'horizontal' })

  useEffect(() => {
    const element = ref.current
    if (!element) return
    // React wheel handlers are passive and cannot prevent the page scroll.
    const onWheel = (event: WheelEvent) => {
      if (Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return
      // scrollLeft can be fractional under zoom, so leave a pixel of slack.
      const max = element.scrollWidth - element.clientWidth - 1
      if (
        (event.deltaY < 0 && element.scrollLeft <= 0) ||
        (event.deltaY > 0 && element.scrollLeft >= max)
      ) {
        return
      }
      event.preventDefault()
      element.scrollLeft += event.deltaY
    }
    element.addEventListener('wheel', onWheel, { passive: false })
    return () => element.removeEventListener('wheel', onWheel)
  }, [])

  return (
    <div ref={ref} className="flex gap-4 overflow-x-auto pb-2">
      {actors.map(actor => {
        const selected = actor.id === selectedID
        return (
          <button
            key={actor.id}
            type="button"
            title={actor.title}
            aria-pressed={selected}
            className="flex w-20 shrink-0 flex-col items-center gap-2 rounded-xl p-1 outline-ring"
            onClick={() => onSelect(selected ? null : actor.id)}
          >
            <Avatar
              key={actor.cover}
              className={cn(
                'size-16 ring-2 ring-offset-2 ring-offset-background transition',
                selected ? 'ring-primary' : 'ring-transparent'
              )}
            >
              {actor.cover ? <AvatarImage src={actor.cover} alt={actor.title} /> : null}
              <AvatarFallback>
                <UserRoundIcon className="size-6" />
              </AvatarFallback>
            </Avatar>
            <span
              className={cn(
                'w-full truncate text-center text-xs',
                selected ? 'font-semibold text-foreground' : 'text-muted-foreground'
              )}
            >
              {actor.title}
            </span>
          </button>
        )
      })}
    </div>
  )
}
